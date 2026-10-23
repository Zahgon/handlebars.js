// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package cli

import (
	"github.com/handlebars-lang/handlebars.js/internal/js"
	"github.com/handlebars-lang/handlebars.js/internal/runtime"
)

// The helpers and decorators of the reference command line, with the JavaScript semantics of their
// definitions: String(), Number(), ===, <, JSON.stringify and the string methods they call.

type helper = func(this js.Value, args []js.Value) (js.Value, error)

func argAt(args []js.Value, i int) js.Value {
	if i < len(args) {
		return args[i]
	}
	return js.Undefined
}

func str(v js.Value) (string, error) { return js.ToString(v) }

func num(v js.Value) (float64, error) { return js.ToNumber(v) }

func registerHelpers(env *runtime.Env) {
	add := func(name string, fn helper) { env.RegisterHelper(name, fn) }
	add("upper", func(_ js.Value, a []js.Value) (js.Value, error) {
		s, err := str(argAt(a, 0))
		if err != nil {
			return nil, err
		}
		return js.ToUpperCase(s), nil
	})
	add("lower", func(_ js.Value, a []js.Value) (js.Value, error) {
		s, err := str(argAt(a, 0))
		if err != nil {
			return nil, err
		}
		return js.ToLowerCase(s), nil
	})
	add("concat", func(_ js.Value, a []js.Value) (js.Value, error) {
		out := ""
		for i := 0; i < len(a)-1; i++ {
			s, err := str(a[i])
			if err != nil {
				return nil, err
			}
			out += s
		}
		return out, nil
	})
	add("join", func(_ js.Value, a []js.Value) (js.Value, error) {
		list, separator := argAt(a, 0), argAt(a, 1)
		arr, ok := list.(*js.Array)
		if !ok {
			return str(list)
		}
		sep := ","
		if separator != js.Undefined {
			s, err := str(separator)
			if err != nil {
				return nil, err
			}
			sep = s
		}
		return js.JoinArray(arr, sep)
	})
	add("split", func(_ js.Value, a []js.Value) (js.Value, error) {
		text, err := str(argAt(a, 0))
		if err != nil {
			return nil, err
		}
		sep, err := str(argAt(a, 1))
		if err != nil {
			return nil, err
		}
		parts := js.Split(text, sep)
		elems := make([]js.Value, len(parts))
		for i, p := range parts {
			elems[i] = p
		}
		return js.NewArray(elems...), nil
	})
	add("json", func(_ js.Value, a []js.Value) (js.Value, error) {
		s, ok, err := js.Stringify(argAt(a, 0))
		if err != nil {
			return nil, err
		}
		if !ok {
			return js.Undefined, nil
		}
		return s, nil
	})
	add("safe", func(_ js.Value, a []js.Value) (js.Value, error) {
		return &js.SafeString{S: argAt(a, 0)}, nil
	})
	add("link", func(_ js.Value, a []js.Value) (js.Value, error) {
		hash, err := runtime.Prop(argAt(a, 1), "hash")
		if err != nil {
			return nil, err
		}
		href, err := runtime.Prop(hash, "href")
		if err != nil {
			return nil, err
		}
		h, err := runtime.Escape(href)
		if err != nil {
			return nil, err
		}
		t, err := runtime.Escape(argAt(a, 0))
		if err != nil {
			return nil, err
		}
		return &js.SafeString{S: `<a href="` + h + `">` + t + `</a>`}, nil
	})
	add("length", func(_ js.Value, a []js.Value) (js.Value, error) {
		v := argAt(a, 0)
		if js.IsNullish(v) {
			return float64(0), nil
		}
		l, err := runtime.Prop(v, "length")
		if err != nil {
			return nil, err
		}
		if l != js.Undefined {
			return l, nil
		}
		return float64(len(js.OwnKeys(v))), nil
	})
	add("typeof", func(_ js.Value, a []js.Value) (js.Value, error) {
		v := argAt(a, 0)
		switch {
		case v == js.Null:
			return "null", nil
		case isArray(v):
			return "array", nil
		}
		return js.TypeOf(v), nil
	})
	add("default", func(_ js.Value, a []js.Value) (js.Value, error) {
		v := argAt(a, 0)
		if js.IsNullish(v) || v == "" {
			return argAt(a, 1), nil
		}
		return v, nil
	})
	arith := func(op func(x, y float64) float64) helper {
		return func(_ js.Value, a []js.Value) (js.Value, error) {
			x, err := num(argAt(a, 0))
			if err != nil {
				return nil, err
			}
			y, err := num(argAt(a, 1))
			if err != nil {
				return nil, err
			}
			return op(x, y), nil
		}
	}
	add("add", arith(func(x, y float64) float64 { return x + y }))
	add("mul", arith(func(x, y float64) float64 { return x * y }))
	add("div", arith(func(x, y float64) float64 { return x / y }))
	add("fixed", func(_ js.Value, a []js.Value) (js.Value, error) {
		x, err := num(argAt(a, 0))
		if err != nil {
			return nil, err
		}
		return js.ToFixed(x, argAt(a, 1))
	})
	add("range", func(_ js.Value, a []js.Value) (js.Value, error) {
		var list []js.Value
		for i := 0; ; i++ {
			n, err := num(argAt(a, 0))
			if err != nil {
				return nil, err
			}
			if !(float64(i) < n) {
				break
			}
			list = append(list, float64(i))
		}
		return js.NewArray(list...), nil
	})
	add("eq", func(_ js.Value, a []js.Value) (js.Value, error) {
		return js.StrictEquals(argAt(a, 0), argAt(a, 1)), nil
	})
	add("ne", func(_ js.Value, a []js.Value) (js.Value, error) {
		return !js.StrictEquals(argAt(a, 0), argAt(a, 1)), nil
	})
	add("lt", func(_ js.Value, a []js.Value) (js.Value, error) {
		r, err := js.LessThan(argAt(a, 0), argAt(a, 1))
		if err != nil {
			return nil, err
		}
		return r == js.True, nil
	})
	add("and", func(_ js.Value, a []js.Value) (js.Value, error) {
		if !js.ToBoolean(argAt(a, 0)) {
			return argAt(a, 0), nil
		}
		return argAt(a, 1), nil
	})
	add("or", func(_ js.Value, a []js.Value) (js.Value, error) {
		if js.ToBoolean(argAt(a, 0)) {
			return argAt(a, 0), nil
		}
		return argAt(a, 1), nil
	})
	add("not", func(_ js.Value, a []js.Value) (js.Value, error) {
		return !js.ToBoolean(argAt(a, 0)), nil
	})
	add("obj", func(_ js.Value, a []js.Value) (js.Value, error) {
		return runtime.Prop(argAt(a, 0), "hash")
	})
	add("dumphash", func(_ js.Value, a []js.Value) (js.Value, error) {
		hash, err := runtime.Prop(argAt(a, 0), "hash")
		if err != nil {
			return nil, err
		}
		s, ok, err := js.Stringify(hash)
		if err != nil || !ok {
			return js.Undefined, err
		}
		return s, nil
	})
	add("context", func(this js.Value, _ []js.Value) (js.Value, error) {
		s, ok, err := js.Stringify(this)
		if err != nil || !ok {
			return js.Undefined, err
		}
		return s, nil
	})
	add("optinfo", optinfo)
	add("repeat", repeat(env))
	add("list", func(this js.Value, a []js.Value) (js.Value, error) {
		items, options := argAt(a, 0), argAt(a, 1)
		arr, ok := items.(*js.Array)
		if !ok || len(arr.Elems) == 0 {
			return runtime.CallMethod(options, "inverse", options, this)
		}
		out := "<ul>"
		for _, item := range arr.Elems {
			r, err := runtime.CallMethod(options, "fn", options, item)
			if err != nil {
				return nil, err
			}
			s, err := js.ConcatString(r)
			if err != nil {
				return nil, err
			}
			out += "<li>" + s + "</li>"
		}
		return out + "</ul>", nil
	})
	add("bold", func(this js.Value, a []js.Value) (js.Value, error) {
		r, err := runtime.CallMethod(argAt(a, 0), "fn", argAt(a, 0), this)
		if err != nil {
			return nil, err
		}
		s, err := js.ConcatString(r)
		if err != nil {
			return nil, err
		}
		return &js.SafeString{S: "<b>" + s + "</b>"}, nil
	})
	add("fail", func(_ js.Value, a []js.Value) (js.Value, error) {
		message, err := str(argAt(a, 0))
		if err != nil {
			return nil, err
		}
		return nil, runtime.NewException(message, argAt(a, 1))
	})
	add("crash", func(_ js.Value, a []js.Value) (js.Value, error) {
		message, err := str(argAt(a, 0))
		if err != nil {
			return nil, err
		}
		return nil, &js.Error{Name: "Error", Message: message}
	})
}

func isArray(v js.Value) bool {
	_, ok := v.(*js.Array)
	return ok
}

func optinfo(_ js.Value, a []js.Value) (js.Value, error) {
	options := argAt(a, len(a)-1)
	info := js.NewObject()
	name, err := runtime.Prop(options, "name")
	if err != nil {
		return nil, err
	}
	info.Set("name", name)
	info.Set("args", float64(len(a)-1))
	for _, key := range []string{"fn", "inverse"} {
		v, err := runtime.Prop(options, key)
		if err != nil {
			return nil, err
		}
		info.Set(key, js.TypeOf(v))
	}
	for _, key := range []string{"hash", "loc"} {
		v, err := runtime.Prop(options, key)
		if err != nil {
			return nil, err
		}
		info.Set(key, v)
	}
	data, err := runtime.Prop(options, "data")
	if err != nil {
		return nil, err
	}
	if js.ToBoolean(data) {
		var keys []js.Value
		for _, k := range js.OwnKeys(data) {
			if k != "_parent" {
				keys = append(keys, k)
			}
		}
		info.Set("data", js.NewArray(keys...))
	} else {
		info.Set("data", js.Null)
	}
	fn, err := runtime.Prop(options, "fn")
	if err != nil {
		return nil, err
	}
	if js.ToBoolean(fn) {
		bp, err := runtime.Prop(fn, "blockParams")
		if err != nil {
			return nil, err
		}
		info.Set("blockParams", bp)
	} else {
		info.Set("blockParams", js.Undefined)
	}
	s, _, err := js.Stringify(info)
	return s, err
}

func repeat(env *runtime.Env) helper {
	return func(this js.Value, a []js.Value) (js.Value, error) {
		count, options := argAt(a, 0), argAt(a, 1)
		n, err := num(count)
		if err != nil {
			return nil, err
		}
		if !(n > 0) {
			return runtime.CallMethod(options, "inverse", options, this)
		}
		out := ""
		for i := 0; float64(i) < n; i++ {
			optData, err := runtime.Prop(options, "data")
			if err != nil {
				return nil, err
			}
			var data js.Value = js.Undefined
			if js.ToBoolean(optData) {
				frame := runtime.CreateFrame(optData)
				frame.Set("index", float64(i))
				frame.Set("first", i == 0)
				frame.Set("last", float64(i) == n-1)
				data = frame
			}
			run := js.NewObject()
			run.Set("data", data)
			run.Set("blockParams", js.NewArray(float64(i), n))
			r, err := runtime.CallMethod(options, "fn", options, this, run)
			if err != nil {
				return nil, err
			}
			s, err := js.ConcatString(r)
			if err != nil {
				return nil, err
			}
			out += s
		}
		return out, nil
	}
}

func registerDecorators(env *runtime.Env) {
	env.RegisterDecorator("wrap", func(_ js.Value, a []js.Value) (js.Value, error) {
		program := argAt(a, 0)
		args, err := runtime.Prop(argAt(a, 3), "args")
		if err != nil {
			return nil, err
		}
		before, err := runtime.Prop(args, "0")
		if err != nil {
			return nil, err
		}
		after, err := runtime.Prop(args, "1")
		if err != nil {
			return nil, err
		}
		return &js.Func{Name: "", Call: func(_ js.Value, b []js.Value) (js.Value, error) {
			bs, err := str(before)
			if err != nil {
				return nil, err
			}
			r, err := runtime.Call(program, "program", js.Undefined, argAt(b, 0), argAt(b, 1))
			if err != nil {
				return nil, err
			}
			rs, err := js.ConcatString(r)
			if err != nil {
				return nil, err
			}
			as, err := str(after)
			if err != nil {
				return nil, err
			}
			return bs + rs + as, nil
		}}, nil
	})
	env.RegisterDecorator("setdata", func(_ js.Value, a []js.Value) (js.Value, error) {
		program := argAt(a, 0)
		options := argAt(a, 3)
		return &js.Func{Name: "", Call: func(_ js.Value, b []js.Value) (js.Value, error) {
			run := js.NewObject().Extend(argAt(b, 1))
			d := run.Value("data")
			if !js.ToBoolean(d) {
				d = js.NewObject()
			}
			frame := runtime.CreateFrame(d)
			run.Set("data", frame)
			hash, err := runtime.Prop(options, "hash")
			if err != nil {
				return nil, err
			}
			for _, key := range js.OwnKeys(hash) {
				v, _ := js.GetOwn(hash, key)
				frame.Set(key, v)
			}
			return runtime.Call(program, "program", js.Undefined, argAt(b, 0), run)
		}}, nil
	})
}
