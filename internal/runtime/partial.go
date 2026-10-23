// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package runtime

import (
	"strings"

	"github.com/handlebars-lang/handlebars.js/internal/ast"
	"github.com/handlebars-lang/handlebars.js/internal/js"
)

// partial executes a partial statement or partial block: the generated invokePartial opcode and the
// runtime's invokePartialWrapper, resolvePartial and invokePartial.
func (s *scope) partial(node ast.Node, nameExpr ast.Expression, params []ast.Expression,
	hash *ast.Hash, program *ast.Program, indent string) (js.Value, error) {
	opts := s.options()
	prefix := ""
	if opts.PreventIndent && indent != "" {
		// The indentation becomes content of its own before the partial.
		prefix, indent = indent, ""
	}
	var partial js.Value
	var name js.Value
	sub, dynamic := nameExpr.(*ast.SubExpression)
	if dynamic {
		v, err := s.sexpr(sub, sub.Path, sub.Params, sub.Hash, nil, nil)
		if err != nil {
			return nil, err
		}
		partial = v
	} else {
		name = partialName(nameExpr)
		v, err := s.c.lookupProperty(s.partials, name.(string))
		if err != nil {
			return nil, err
		}
		partial = v
	}
	var context js.Value
	if dynamic && len(params) == 0 && opts.ExplicitPartialContext {
		// The generated code pushes `undefined` for the missing context before the dynamic name, then pops
		// them the other way round: the name becomes the context and the partial is undefined.
		context, partial = partial, js.Undefined
	} else if len(params) > 0 {
		v, err := s.expression(params[0])
		if err != nil {
			return nil, err
		}
		context = v
	} else if opts.ExplicitPartialContext {
		context = js.Undefined
	} else {
		context = s.ctx
	}
	options, _, err := s.setupParamsWith(node, name, nil, hash, program, nil, false, true)
	if err != nil {
		return nil, err
	}
	if indent != "" {
		options.Set("indent", indent)
	}
	options.Set("helpers", s.helpers)
	options.Set("partials", s.partials)
	options.Set("decorators", s.c.decorators)
	if opts.Compat {
		if s.depths == nil {
			options.Set("depths", js.Undefined)
		} else {
			options.Set("depths", js.NewArray(s.depths...))
		}
	}
	result, err := s.c.invokePartialWrapper(partial, context, options)
	if err != nil {
		return nil, err
	}
	if js.IsNullish(result) {
		result = ""
	}
	if prefix != "" {
		return js.Add(prefix, result)
	}
	return result, nil
}

func partialName(e ast.Expression) js.Value {
	switch n := e.(type) {
	case *ast.PathExpression:
		return n.Original
	case *ast.StringLiteral:
		return n.Value
	case *ast.NumberLiteral:
		return js.NumberToString(n.Value)
	case *ast.BooleanLiteral:
		if n.Value {
			return "true"
		}
		return "false"
	case *ast.UndefinedLiteral:
		return "undefined"
	case *ast.NullLiteral:
		return "null"
	}
	return "undefined"
}

func nameText(v js.Value) string {
	s, err := js.ConcatString(v)
	if err != nil {
		return ""
	}
	return s
}

func (c *container) invokePartialWrapper(partial, context js.Value, options *js.Object) (js.Value, error) {
	if h := options.Value("hash"); js.ToBoolean(h) {
		context = js.NewObject().Extend(context, h)
	}
	partial, err := resolvePartial(partial, options)
	if err != nil {
		return nil, err
	}
	options.Set("hooks", c.hooks)
	result, err := invokePartial(partial, context, options)
	if err != nil {
		return nil, err
	}
	if js.IsNullish(result) {
		if _, isFunc := partial.(*js.Func); isFunc {
			return nil, &js.Error{Name: "Error", Message: "The partial " + nameText(options.Value("name")) +
				" returned no output: partials must return a string"}
		}
		text, isString := partial.(string)
		if !isString {
			return nil, &js.Error{Name: "Error", Message: "The partial " + nameText(options.Value("name")) +
				" could not be compiled: partials must be strings or functions returning strings"}
		}
		compiled := c.t.env.Compile(text, c.t.compiled.Options)
		partials, _ := options.Value("partials").(*js.Object)
		if partials != nil {
			key, err := js.ToString(options.Value("name"))
			if err != nil {
				return nil, err
			}
			partials.Set(key, compiled)
		}
		if result, err = compiled.Call(js.Undefined, []js.Value{context, options}); err != nil {
			return nil, err
		}
	}
	indent, _ := options.Value("indent").(string)
	if indent == "" {
		return result, nil
	}
	str, ok := result.(string)
	if !ok {
		return nil, js.TypeError("result.split is not a function")
	}
	{
		lines := strings.Split(str, "\n")
		for i := range lines {
			if lines[i] == "" && i+1 == len(lines) {
				break
			}
			lines[i] = indent + lines[i]
		}
		str = strings.Join(lines, "\n")
	}
	return str, nil
}

func lookupOwn(obj js.Value, name string) js.Value {
	if js.ToBoolean(obj) {
		if v, ok := js.GetOwn(obj, name); ok {
			return v
		}
	}
	return js.Undefined
}

func resolvePartial(partial js.Value, options *js.Object) (js.Value, error) {
	if !js.ToBoolean(partial) {
		name := options.Value("name")
		if s, ok := name.(string); ok && s == "@partial-block" {
			return lookupOwn(options.Value("data"), "partial-block"), nil
		}
		key, err := js.ToString(name)
		if err != nil {
			return nil, err
		}
		return lookupOwn(options.Value("partials"), key), nil
	}
	if _, isFunc := partial.(*js.Func); !isFunc && !js.ToBoolean(options.Value("name")) {
		options.Set("name", partial)
		key, err := js.ToString(partial)
		if err != nil {
			return nil, err
		}
		return lookupOwn(options.Value("partials"), key), nil
	}
	return partial, nil
}

func invokePartial(partial, context js.Value, options *js.Object) (js.Value, error) {
	currentPartialBlock := lookupOwn(options.Value("data"), "partial-block")
	options.Set("partial", true)
	var partialBlock js.Value = js.Undefined
	if fn, ok := options.Value("fn").(*js.Func); ok && fn.Name != "noop" {
		frame := createFrame(options.Value("data"))
		options.Set("data", frame)
		wrapper := &js.Func{Name: "partialBlockWrapper"}
		wrapper.Call = func(_ js.Value, args []js.Value) (js.Value, error) {
			opts := arg(args, 1)
			if js.IsNullish(opts) {
				opts = js.NewObject()
			}
			o, ok := opts.(*js.Object)
			if !ok {
				return fn.Call(js.Undefined, []js.Value{arg(args, 0), opts})
			}
			d := createFrame(o.Value("data"))
			o.Set("data", d)
			d.Set("partial-block", currentPartialBlock)
			return fn.Call(js.Undefined, []js.Value{arg(args, 0), o})
		}
		frame.Set("partial-block", wrapper)
		partialBlock = wrapper
		if inline := fn.Prop("partials"); js.ToBoolean(inline) {
			options.Set("partials", js.NewObject().Extend(options.Value("partials"), inline))
		}
	}
	if partial == js.Undefined && partialBlock != js.Undefined {
		partial = partialBlock
	}
	if partial == js.Undefined || partial == nil {
		return nil, &js.Error{Name: "Error", Message: "The partial " + nameText(options.Value("name")) + " could not be found"}
	}
	if f, ok := partial.(*js.Func); ok {
		return f.Call(js.Undefined, []js.Value{context, options})
	}
	return js.Undefined, nil
}
