// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

// Package js models the JavaScript values a Handlebars template sees: JSON data, helper results, the options
// objects passed to helpers and the program functions of a template. It reproduces the parts of the ECMAScript
// semantics Handlebars depends on: property lookup along the prototype chain, own-key order, type conversion,
// equality, JSON and the string and number methods the command-line helpers call.
//
// Strings are Go strings holding WTF-8: valid UTF-8 plus three-byte encodings of lone UTF-16 surrogates, so that
// every JavaScript string (a sequence of UTF-16 code units) has a representation.
package js

import (
	"strconv"
)

// Value is any JavaScript value: Undefined, Null, bool, float64, string, *Array, *Object, *SafeString or *Func.
type Value interface{}

type undefinedType struct{}
type nullType struct{}

// Undefined and Null are the two JavaScript values without properties.
var (
	Undefined Value = undefinedType{}
	Null      Value = nullType{}
)

// Array is a JavaScript array without holes.
type Array struct {
	Elems []Value
}

// NewArray returns an array holding elems.
func NewArray(elems ...Value) *Array {
	return &Array{Elems: elems}
}

// Object is an ordinary JavaScript object. Its own keys keep the order JavaScript gives them: array-index keys in
// ascending numeric order first, then the other keys in insertion order.
type Object struct {
	keys   []string
	values map[string]Value
	// Sealed marks Handlebars' nullContext, which is frozen against additions.
	Sealed bool
}

// NewObject returns an empty object.
func NewObject() *Object {
	return &Object{values: map[string]Value{}}
}

// Get returns an own property.
func (o *Object) Get(key string) (Value, bool) {
	v, ok := o.values[key]
	return v, ok
}

// Value returns an own property or Undefined.
func (o *Object) Value(key string) Value {
	if v, ok := o.values[key]; ok {
		return v
	}
	return Undefined
}

// Has reports whether key is an own property.
func (o *Object) Has(key string) bool {
	_, ok := o.values[key]
	return ok
}

// Set assigns an own property, adding the key in JavaScript order when it is new.
func (o *Object) Set(key string, v Value) {
	if _, ok := o.values[key]; ok {
		o.values[key] = v
		return
	}
	if o.Sealed {
		return
	}
	o.values[key] = v
	if idx, ok := ArrayIndex(key); ok {
		// Index keys come first, ascending.
		pos := 0
		for pos < len(o.keys) {
			other, isIdx := ArrayIndex(o.keys[pos])
			if !isIdx || other > idx {
				break
			}
			pos++
		}
		o.keys = append(o.keys, "")
		copy(o.keys[pos+1:], o.keys[pos:])
		o.keys[pos] = key
		return
	}
	o.keys = append(o.keys, key)
}

// Keys returns the own keys in order (the result of Object.keys).
func (o *Object) Keys() []string {
	return o.keys
}

// Len returns the number of own properties.
func (o *Object) Len() int {
	return len(o.keys)
}

// Extend copies the own properties of each source onto o, as Handlebars' Utils.extend does.
func (o *Object) Extend(sources ...Value) *Object {
	for _, src := range sources {
		switch s := src.(type) {
		case *Object:
			for _, k := range s.keys {
				o.Set(k, s.values[k])
			}
		case *Array:
			for i, e := range s.Elems {
				o.Set(strconv.Itoa(i), e)
			}
		case string:
			units := ToUnits(s)
			for i := range units {
				o.Set(strconv.Itoa(i), FromUnits(units[i:i+1]))
			}
		case *SafeString:
			o.Set("string", s.S)
		case *Func:
			if s.Props != nil {
				o.Extend(s.Props)
			}
		}
	}
	return o
}

// ArrayIndex reports whether key is a canonical array index ("0" to "4294967294").
func ArrayIndex(key string) (uint32, bool) {
	if key == "" || len(key) > 10 || (len(key) > 1 && key[0] == '0') {
		return 0, false
	}
	n, err := strconv.ParseUint(key, 10, 64)
	if err != nil || n >= 4294967295 {
		return 0, false
	}
	return uint32(n), true
}

// SafeString is Handlebars.SafeString: an object whose own property "string" is output without escaping.
type SafeString struct {
	S Value
}

// Func is a JavaScript function: a helper, a decorator, a compiled program or partial, or a built-in method.
type Func struct {
	Name string
	Call func(this Value, args []Value) (Value, error)
	// Props holds properties assigned to the function object (blockParams, partials, ...).
	Props *Object
}

// Prop returns a property assigned to the function or Undefined.
func (f *Func) Prop(key string) Value {
	if f.Props == nil {
		return Undefined
	}
	return f.Props.Value(key)
}

// SetProp assigns a property on the function object.
func (f *Func) SetProp(key string, v Value) {
	if f.Props == nil {
		f.Props = NewObject()
	}
	f.Props.Set(key, v)
}

// Invoke calls f with this and args.
func (f *Func) Invoke(this Value, args ...Value) (Value, error) {
	return f.Call(this, args)
}

// Error is a thrown JavaScript error: Name is "Error", "TypeError" or "RangeError".
type Error struct {
	Name    string
	Message string
}

func (e *Error) Error() string {
	return e.Name + ": " + e.Message
}

// TypeError returns a thrown TypeError.
func TypeError(message string) error {
	return &Error{Name: "TypeError", Message: message}
}

// IsNullish reports whether v is null or undefined (v == null in JavaScript).
func IsNullish(v Value) bool {
	return v == Undefined || v == Null || v == nil
}

// TypeOf returns the result of the typeof operator.
func TypeOf(v Value) string {
	switch v.(type) {
	case undefinedType, nil:
		return "undefined"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case *Func:
		return "function"
	}
	return "object"
}

// IsObject reports whether v is an object (including arrays and functions).
func IsObject(v Value) bool {
	switch v.(type) {
	case *Object, *Array, *SafeString, *Func:
		return true
	}
	return false
}

// ToBoolean converts like Boolean(v).
func ToBoolean(v Value) bool {
	switch x := v.(type) {
	case undefinedType, nullType, nil:
		return false
	case bool:
		return x
	case float64:
		return x != 0 && x == x
	case string:
		return x != ""
	}
	return true
}

// StrictEquals is the === operator.
func StrictEquals(a, b Value) bool {
	if a == nil {
		a = Undefined
	}
	if b == nil {
		b = Undefined
	}
	switch x := a.(type) {
	case float64:
		y, ok := b.(float64)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && SameUnits(x, y)
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	}
	return a == b
}

// LooseEquals is the == operator.
func LooseEquals(a, b Value) (bool, error) {
	if a == nil {
		a = Undefined
	}
	if b == nil {
		b = Undefined
	}
	if sameType(a, b) {
		return StrictEquals(a, b), nil
	}
	if IsNullish(a) && IsNullish(b) {
		return true, nil
	}
	if IsNullish(a) || IsNullish(b) {
		return false, nil
	}
	switch x := a.(type) {
	case float64:
		if s, ok := b.(string); ok {
			return x == StringToNumber(s), nil
		}
	case string:
		if y, ok := b.(float64); ok {
			return StringToNumber(x) == y, nil
		}
	}
	if x, ok := a.(bool); ok {
		return LooseEquals(boolNumber(x), b)
	}
	if y, ok := b.(bool); ok {
		return LooseEquals(a, boolNumber(y))
	}
	if IsObject(a) && !IsObject(b) {
		p, err := ToPrimitive(a, "default")
		if err != nil {
			return false, err
		}
		return LooseEquals(p, b)
	}
	if IsObject(b) && !IsObject(a) {
		p, err := ToPrimitive(b, "default")
		if err != nil {
			return false, err
		}
		return LooseEquals(a, p)
	}
	return false, nil
}

func boolNumber(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func sameType(a, b Value) bool {
	switch a.(type) {
	case undefinedType:
		_, ok := b.(undefinedType)
		return ok
	case nullType:
		_, ok := b.(nullType)
		return ok
	case bool:
		_, ok := b.(bool)
		return ok
	case float64:
		_, ok := b.(float64)
		return ok
	case string:
		_, ok := b.(string)
		return ok
	}
	return IsObject(a) && IsObject(b)
}

// ToPrimitive converts an object to a primitive with the given hint ("default", "string" or "number"). Objects
// built from JSON may carry own "toString" or "valueOf" keys; those are not callable and are skipped, so an object
// with both fails like it does in JavaScript.
func ToPrimitive(v Value, hint string) (Value, error) {
	if !IsObject(v) {
		return v, nil
	}
	order := []string{"valueOf", "toString"}
	if hint == "string" {
		order = []string{"toString", "valueOf"}
	}
	for _, method := range order {
		if r, ok, err := callConversion(v, method); err != nil {
			return nil, err
		} else if ok {
			return r, nil
		}
	}
	return nil, TypeError("Cannot convert object to primitive value")
}

// callConversion runs v.valueOf() or v.toString() when that property is callable and returns a primitive.
func callConversion(v Value, method string) (Value, bool, error) {
	switch x := v.(type) {
	case *Object:
		if own, ok := x.Get(method); ok {
			if f, isFunc := own.(*Func); isFunc {
				r, err := f.Call(v, nil)
				if err != nil {
					return nil, false, err
				}
				if IsObject(r) {
					return nil, false, nil
				}
				return r, true, nil
			}
			return nil, false, nil
		}
		if method == "toString" {
			return "[object Object]", true, nil
		}
		return nil, false, nil
	case *Array:
		if method == "toString" {
			s, err := JoinArray(x, ",")
			return s, err == nil, err
		}
		return nil, false, nil
	case *SafeString:
		if method == "toString" {
			s, err := ToString(x.S)
			return s, err == nil, err
		}
		return nil, false, nil
	case *Func:
		if method == "toString" {
			return "function " + x.Name + "() { [native code] }", true, nil
		}
		return nil, false, nil
	}
	return v, true, nil
}

// JoinArray is Array.prototype.join.
func JoinArray(a *Array, sep string) (string, error) {
	buf := make([]byte, 0, 16)
	for i, e := range a.Elems {
		if i > 0 {
			buf = append(buf, sep...)
		}
		if IsNullish(e) {
			continue
		}
		s, err := ToString(e)
		if err != nil {
			return "", err
		}
		buf = append(buf, s...)
	}
	return string(buf), nil
}

// ToString converts like String(v).
func ToString(v Value) (string, error) {
	switch x := v.(type) {
	case undefinedType, nil:
		return "undefined", nil
	case nullType:
		return "null", nil
	case bool:
		if x {
			return "true", nil
		}
		return "false", nil
	case float64:
		return NumberToString(x), nil
	case string:
		return x, nil
	}
	p, err := ToPrimitive(v, "string")
	if err != nil {
		return "", err
	}
	return ToString(p)
}

// ConcatString converts like the + operator with a string operand: objects use the default hint.
func ConcatString(v Value) (string, error) {
	if IsObject(v) {
		p, err := ToPrimitive(v, "default")
		if err != nil {
			return "", err
		}
		return ToString(p)
	}
	return ToString(v)
}

// ToNumber converts like Number(v).
func ToNumber(v Value) (float64, error) {
	switch x := v.(type) {
	case undefinedType, nil:
		return nan(), nil
	case nullType:
		return 0, nil
	case bool:
		return boolNumber(x), nil
	case float64:
		return x, nil
	case string:
		return StringToNumber(x), nil
	}
	p, err := ToPrimitive(v, "number")
	if err != nil {
		return 0, err
	}
	return ToNumber(p)
}

// Comparison is the result of the abstract relational comparison: True, False or Undef (a NaN was involved).
type Comparison int

// The outcomes of LessThan.
const (
	False Comparison = iota
	True
	Undef
)

// LessThan is the abstract relational comparison a < b.
func LessThan(a, b Value) (Comparison, error) {
	pa, err := ToPrimitive(a, "number")
	if err != nil {
		return Undef, err
	}
	pb, err := ToPrimitive(b, "number")
	if err != nil {
		return Undef, err
	}
	sa, aStr := pa.(string)
	sb, bStr := pb.(string)
	if aStr && bStr {
		if Less(sa, sb) {
			return True, nil
		}
		return False, nil
	}
	na, err := ToNumber(pa)
	if err != nil {
		return Undef, err
	}
	nb, err := ToNumber(pb)
	if err != nil {
		return Undef, err
	}
	if na != na || nb != nb {
		return Undef, nil
	}
	if na < nb {
		return True, nil
	}
	return False, nil
}

// Add is the + operator: string concatenation when either primitive operand is a string, numeric addition
// otherwise.
func Add(a, b Value) (Value, error) {
	pa, err := ToPrimitive(a, "default")
	if err != nil {
		return nil, err
	}
	pb, err := ToPrimitive(b, "default")
	if err != nil {
		return nil, err
	}
	sa, aStr := pa.(string)
	sb, bStr := pb.(string)
	if aStr || bStr {
		if !aStr {
			if sa, err = ToString(pa); err != nil {
				return nil, err
			}
		}
		if !bStr {
			if sb, err = ToString(pb); err != nil {
				return nil, err
			}
		}
		return sa + sb, nil
	}
	na, err := ToNumber(pa)
	if err != nil {
		return nil, err
	}
	nb, err := ToNumber(pb)
	if err != nil {
		return nil, err
	}
	return na + nb, nil
}
