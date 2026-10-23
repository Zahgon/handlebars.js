// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package js

import (
	"strconv"
)

// Inherited property names of the built-in prototypes (Node.js 22), with whether each holds a function. Handlebars
// refuses inherited properties, so their values never reach a template; only their existence and kind matter.
var (
	objectProto = []string{"constructor", "__defineGetter__", "__defineSetter__", "hasOwnProperty",
		"__lookupGetter__", "__lookupSetter__", "isPrototypeOf", "propertyIsEnumerable", "toString", "valueOf",
		"__proto__", "toLocaleString"}
	arrayProto = []string{"constructor", "at", "concat", "copyWithin", "fill", "find", "findIndex", "findLast",
		"findLastIndex", "lastIndexOf", "pop", "push", "reverse", "shift", "unshift", "slice", "sort", "splice",
		"includes", "indexOf", "join", "keys", "entries", "values", "forEach", "filter", "flat", "flatMap", "map",
		"every", "some", "reduce", "reduceRight", "toReversed", "toSorted", "toSpliced", "with", "toLocaleString",
		"toString"}
	stringProto = []string{"constructor", "anchor", "at", "big", "blink", "bold", "charAt", "charCodeAt",
		"codePointAt", "concat", "endsWith", "fontcolor", "fontsize", "fixed", "includes", "indexOf", "isWellFormed",
		"italics", "lastIndexOf", "link", "localeCompare", "match", "matchAll", "normalize", "padEnd", "padStart",
		"repeat", "replace", "replaceAll", "search", "slice", "small", "split", "strike", "sub", "substr",
		"substring", "sup", "startsWith", "toString", "toWellFormed", "trim", "trimStart", "trimLeft", "trimEnd",
		"trimRight", "toLocaleLowerCase", "toLocaleUpperCase", "toLowerCase", "toUpperCase", "valueOf"}
	numberProto     = []string{"constructor", "toExponential", "toFixed", "toPrecision", "toString", "valueOf", "toLocaleString"}
	booleanProto    = []string{"constructor", "toString", "valueOf"}
	functionProto   = []string{"apply", "bind", "call", "toString", "constructor"}
	safeStringProto = []string{"toString", "toHTML", "constructor"}
)

var protoSets = map[string]map[string]bool{}

func init() {
	for name, list := range map[string][]string{"object": objectProto, "array": arrayProto, "string": stringProto,
		"number": numberProto, "boolean": booleanProto, "function": functionProto, "safestring": safeStringProto} {
		set := map[string]bool{}
		for _, k := range list {
			set[k] = true
		}
		protoSets[name] = set
	}
}

// protoChain names the prototypes of v, nearest first.
func protoChain(v Value) []string {
	switch v.(type) {
	case *Array:
		return []string{"array", "object"}
	case string:
		return []string{"string", "object"}
	case float64:
		return []string{"number", "object"}
	case bool:
		return []string{"boolean", "object"}
	case *Func:
		return []string{"function", "object"}
	case *SafeString:
		return []string{"safestring", "object"}
	case *Object:
		return []string{"object"}
	}
	return nil
}

// Inherited is a placeholder for a value found on a built-in prototype.
type Inherited struct {
	Name     string
	Function bool
}

// GetOwn returns v's own property key.
func GetOwn(v Value, key string) (Value, bool) {
	switch x := v.(type) {
	case *Object:
		return x.Get(key)
	case *Array:
		if key == "length" {
			return float64(len(x.Elems)), true
		}
		if i, ok := ArrayIndex(key); ok && int(i) < len(x.Elems) {
			return x.Elems[i], true
		}
	case string:
		units := ToUnits(x)
		if key == "length" {
			return float64(len(units)), true
		}
		if i, ok := ArrayIndex(key); ok && int(i) < len(units) {
			return FromUnits(units[i : i+1]), true
		}
	case *SafeString:
		if key == "string" {
			return x.S, true
		}
	case *Func:
		if x.Props != nil {
			if p, ok := x.Props.Get(key); ok {
				return p, true
			}
		}
		switch key {
		case "length":
			return float64(0), true
		case "name":
			return x.Name, true
		}
	}
	return nil, false
}

// Get is the property access v[key]. It fails like JavaScript when v is null or undefined. A property found on
// a built-in prototype is returned as *Inherited; own reports whether the property is v's own.
func Get(v Value, key string) (result Value, own bool, err error) {
	if IsNullish(v) {
		return nil, false, TypeError("Cannot read properties of " + nullishName(v) + " (reading '" + key + "')")
	}
	if r, ok := GetOwn(v, key); ok {
		return r, true, nil
	}
	for _, proto := range protoChain(v) {
		if protoSets[proto][key] {
			return &Inherited{Name: key, Function: key != "__proto__"}, false, nil
		}
	}
	return Undefined, false, nil
}

// In is the `key in v` operator.
func In(key string, v Value) (bool, error) {
	if !IsObject(v) {
		s, err := ToString(v)
		if err != nil {
			return false, err
		}
		return false, TypeError("Cannot use 'in' operator to search for '" + key + "' in " + s)
	}
	r, own, err := Get(v, key)
	if err != nil {
		return false, err
	}
	if own {
		return true, nil
	}
	_, inherited := r.(*Inherited)
	return inherited, nil
}

func nullishName(v Value) string {
	if v == Null {
		return "null"
	}
	return "undefined"
}

// OwnKeys returns Object.keys(v).
func OwnKeys(v Value) []string {
	switch x := v.(type) {
	case *Object:
		return x.Keys()
	case *Array:
		keys := make([]string, len(x.Elems))
		for i := range keys {
			keys[i] = strconv.Itoa(i)
		}
		return keys
	case string:
		units := ToUnits(x)
		keys := make([]string, len(units))
		for i := range keys {
			keys[i] = strconv.Itoa(i)
		}
		return keys
	case *SafeString:
		return []string{"string"}
	case *Func:
		if x.Props != nil {
			return x.Props.Keys()
		}
	}
	return nil
}
