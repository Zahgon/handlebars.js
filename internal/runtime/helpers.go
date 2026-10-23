// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package runtime

import (
	"github.com/handlebars-lang/handlebars.js/internal/ast"
	"github.com/handlebars-lang/handlebars.js/internal/js"
)

// Prop reads v[key] like JavaScript, failing on null and undefined.
func Prop(v js.Value, key string) (js.Value, error) {
	r, err := get(v, key)
	if err != nil {
		return nil, err
	}
	if inh, ok := r.(*js.Inherited); ok {
		return inheritedValue(inh), nil
	}
	return r, nil
}

// Call calls fn with this and args; expr is the source text V8 names in "... is not a function".
func Call(fn js.Value, expr string, this js.Value, args ...js.Value) (js.Value, error) {
	f, ok := fn.(*js.Func)
	if !ok {
		return nil, js.TypeError(expr + " is not a function")
	}
	return f.Call(this, args)
}

// CallMethod is options.<name>(this, args...) for a helper's options object.
func CallMethod(options js.Value, name string, this js.Value, args ...js.Value) (js.Value, error) {
	fn, err := Prop(options, name)
	if err != nil {
		return nil, err
	}
	return Call(fn, "options."+name, this, args...)
}

func isEmpty(v js.Value) bool {
	if !js.ToBoolean(v) {
		if f, ok := v.(float64); ok && f == 0 {
			return false
		}
		return true
	}
	if a, ok := v.(*js.Array); ok && len(a.Elems) == 0 {
		return true
	}
	return false
}

func registerDefaultHelpers(e *Env) {
	e.RegisterHelper("blockHelperMissing", func(this js.Value, args []js.Value) (js.Value, error) {
		context, options := arg(args, 0), arg(args, 1)
		fn, err := Prop(options, "fn")
		if err != nil {
			return nil, err
		}
		inverse, err := Prop(options, "inverse")
		if err != nil {
			return nil, err
		}
		switch {
		case context == true:
			return Call(fn, "fn", this, this)
		case context == false || js.IsNullish(context):
			return Call(inverse, "inverse", this, this)
		}
		if a, ok := context.(*js.Array); ok {
			if len(a.Elems) > 0 {
				return Call(e.Helpers.Value("each"), "instance.helpers.each", js.Undefined, context, options)
			}
			return Call(inverse, "inverse", this, this)
		}
		return Call(fn, "fn", this, context, options)
	})
	e.RegisterHelper("each", func(this js.Value, args []js.Value) (js.Value, error) {
		return eachHelper(this, args)
	})
	e.RegisterHelper("helperMissing", func(this js.Value, args []js.Value) (js.Value, error) {
		if len(args) == 1 {
			return js.Undefined, nil
		}
		name, err := Prop(arg(args, len(args)-1), "name")
		if err != nil {
			return nil, err
		}
		return nil, &js.Error{Name: "Error", Message: `Missing helper: "` + nameText(name) + `"`}
	})
	ifHelper := func(this js.Value, args []js.Value) (js.Value, error) {
		if len(args) != 2 {
			return nil, &js.Error{Name: "Error", Message: "#if requires exactly one argument"}
		}
		conditional, options := args[0], args[1]
		if f, ok := conditional.(*js.Func); ok {
			v, err := f.Call(this, nil)
			if err != nil {
				return nil, err
			}
			conditional = v
		}
		hash, err := Prop(options, "hash")
		if err != nil {
			return nil, err
		}
		includeZero, err := Prop(hash, "includeZero")
		if err != nil {
			return nil, err
		}
		if (!js.ToBoolean(includeZero) && !js.ToBoolean(conditional)) || isEmpty(conditional) {
			return CallMethod(options, "inverse", this, this)
		}
		return CallMethod(options, "fn", this, this)
	}
	e.RegisterHelper("if", ifHelper)
	e.RegisterHelper("unless", func(this js.Value, args []js.Value) (js.Value, error) {
		if len(args) != 2 {
			return nil, &js.Error{Name: "Error", Message: "#unless requires exactly one argument"}
		}
		options := args[1]
		swapped := js.NewObject()
		for _, pair := range [][2]string{{"fn", "inverse"}, {"inverse", "fn"}, {"hash", "hash"}} {
			v, err := Prop(options, pair[1])
			if err != nil {
				return nil, err
			}
			swapped.Set(pair[0], v)
		}
		return Call(e.Helpers.Value("if"), "instance.helpers['if'].call", this, args[0], swapped)
	})
	e.RegisterHelper("log", func(this js.Value, args []js.Value) (js.Value, error) {
		options := arg(args, len(args)-1)
		message := append([]js.Value(nil), args[:len(args)-1]...)
		var level js.Value = float64(1)
		hash, err := Prop(options, "hash")
		if err != nil {
			return nil, err
		}
		hashLevel, err := Prop(hash, "level")
		if err != nil {
			return nil, err
		}
		data, err := Prop(options, "data")
		if err != nil {
			return nil, err
		}
		if !js.IsNullish(hashLevel) {
			level = hashLevel
		} else if js.ToBoolean(data) {
			dataLevel, err := Prop(data, "level")
			if err != nil {
				return nil, err
			}
			if !js.IsNullish(dataLevel) {
				level = dataLevel
			}
		}
		return js.Undefined, e.Logger.Log(level, message...)
	})
	e.RegisterHelper("lookup", func(this js.Value, args []js.Value) (js.Value, error) {
		obj, field, options := arg(args, 0), arg(args, 1), arg(args, 2)
		if !js.ToBoolean(obj) {
			return obj, nil
		}
		fn, err := Prop(options, "lookupProperty")
		if err != nil {
			return nil, err
		}
		return Call(fn, "options.lookupProperty", options, obj, field)
	})
	e.RegisterHelper("with", func(this js.Value, args []js.Value) (js.Value, error) {
		if len(args) != 2 {
			return nil, &js.Error{Name: "Error", Message: "#with requires exactly one argument"}
		}
		context, options := args[0], args[1]
		if f, ok := context.(*js.Func); ok {
			v, err := f.Call(this, nil)
			if err != nil {
				return nil, err
			}
			context = v
		}
		fn, err := Prop(options, "fn")
		if err != nil {
			return nil, err
		}
		if !isEmpty(context) {
			data, err := Prop(options, "data")
			if err != nil {
				return nil, err
			}
			runOptions := js.NewObject()
			runOptions.Set("data", data)
			runOptions.Set("blockParams", js.NewArray(context))
			return Call(fn, "fn", js.Undefined, context, runOptions)
		}
		return CallMethod(options, "inverse", this, this)
	})
}

func eachHelper(this js.Value, args []js.Value) (js.Value, error) {
	if len(args) < 2 || js.IsNullish(args[1]) || !js.ToBoolean(args[1]) {
		return nil, &js.Error{Name: "Error", Message: "Must pass iterator to #each"}
	}
	context, options := args[0], args[1]
	fn, err := Prop(options, "fn")
	if err != nil {
		return nil, err
	}
	inverse, err := Prop(options, "inverse")
	if err != nil {
		return nil, err
	}
	if f, ok := context.(*js.Func); ok {
		if context, err = f.Call(this, nil); err != nil {
			return nil, err
		}
	}
	optData, err := Prop(options, "data")
	if err != nil {
		return nil, err
	}
	var data *js.Object
	if js.ToBoolean(optData) {
		data = createFrame(optData)
	}
	ret := ""
	i := 0
	iteration := func(field js.Value, index int, last bool, value js.Value) error {
		if data != nil {
			data.Set("key", field)
			data.Set("index", float64(index))
			data.Set("first", index == 0)
			data.Set("last", last)
		}
		runOptions := js.NewObject()
		runOptions.Set("data", dataValue(data))
		runOptions.Set("blockParams", js.NewArray(value, field))
		out, err := Call(fn, "fn", js.Undefined, value, runOptions)
		if err != nil {
			return err
		}
		s, err := js.ConcatString(out)
		if err != nil {
			return err
		}
		ret += s
		return nil
	}
	switch c := context.(type) {
	case *js.Array:
		for ; i < len(c.Elems); i++ {
			if err := iteration(float64(i), i, i == len(c.Elems)-1, c.Elems[i]); err != nil {
				return nil, err
			}
		}
	case *js.Object, *js.SafeString:
		keys := js.OwnKeys(c)
		for _, key := range keys {
			i++
			v, _ := js.GetOwn(c, key)
			if err := iteration(key, i-1, i == len(keys), v); err != nil {
				return nil, err
			}
		}
	}
	if i == 0 {
		out, err := Call(inverse, "inverse", this, this)
		if err != nil {
			return nil, err
		}
		return out, nil
	}
	return ret, nil
}

func dataValue(data *js.Object) js.Value {
	if data == nil {
		return js.Undefined
	}
	return data
}

// inlineDecorator is the built-in {{#*inline "name"}} decorator.
func inlineDecorator(_ js.Value, args []js.Value) (js.Value, error) {
	fn, _ := arg(args, 0).(*js.Func)
	props, _ := arg(args, 1).(*js.Object)
	c, _ := arg(args, 2).(*container)
	options := arg(args, 3)
	ret := fn
	partials, _ := props.Value("partials").(*js.Object)
	if partials == nil {
		partials = js.NewObject()
		props.Set("partials", partials)
		ret = &js.Func{Name: "inline", Call: func(this js.Value, a []js.Value) (js.Value, error) {
			original := c.partials
			c.partials = js.NewObject().Extend(original, partials)
			out, err := fn.Call(this, a)
			c.partials = original
			return out, err
		}}
	}
	argList, err := Prop(options, "args")
	if err != nil {
		return nil, err
	}
	name, err := Prop(argList, "0")
	if err != nil {
		return nil, err
	}
	key, err := js.ToString(name)
	if err != nil {
		return nil, err
	}
	optFn, err := Prop(options, "fn")
	if err != nil {
		return nil, err
	}
	partials.Set(key, optFn)
	return ret, nil
}

// Escape is Handlebars.Utils.escapeExpression.
func Escape(v js.Value) (string, error) { return escapeExpression(v) }

// CreateFrame is Handlebars.createFrame.
func CreateFrame(v js.Value) *js.Object { return createFrame(v) }

// NewException is `new Handlebars.Exception(message, node)`: the node's loc.start adds " - line:column".
func NewException(message string, node js.Value) error {
	if js.ToBoolean(node) {
		loc, err := Prop(node, "loc")
		if err != nil {
			return err
		}
		if js.ToBoolean(loc) {
			start, err := Prop(loc, "start")
			if err != nil {
				return err
			}
			if js.ToBoolean(start) {
				line, err := Prop(start, "line")
				if err != nil {
					return err
				}
				column, err := Prop(start, "column")
				if err != nil {
					return err
				}
				l, _ := js.ConcatString(line)
				c, _ := js.ConcatString(column)
				message = ast.AppendLocation(message, l, c)
			}
		}
	}
	return &js.Error{Name: "Error", Message: message}
}
