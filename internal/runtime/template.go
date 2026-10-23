// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package runtime

import (
	"github.com/handlebars-lang/handlebars.js/internal/ast"
	"github.com/handlebars-lang/handlebars.js/internal/compiler"
	"github.com/handlebars-lang/handlebars.js/internal/js"
	"github.com/handlebars-lang/handlebars.js/internal/parser"
)

// template is one compiled template with its container (the runtime state the generated code closes over).
type template struct {
	env      *Env
	source   string
	opts     *compiler.Options
	compiled *compiler.Compiled
	c        *container
}

// container mirrors the original's container object.
type container struct {
	t           *template
	helpers     *js.Object
	partials    *js.Object
	decorators  *js.Object
	hooks       hookSet
	nullContext *js.Object
	lookupFn    *js.Func
	noop        *js.Func
}

func (t *template) compile() error {
	if t.compiled != nil {
		return nil
	}
	program, err := parser.Parse(t.source, parser.Options{IgnoreStandalone: t.opts.IgnoreStandalone})
	if err != nil {
		if pe, ok := err.(*parser.Error); ok {
			return &js.Error{Name: pe.Name, Message: pe.Message}
		}
		return err
	}
	compiled, err := compiler.Compile(program, t.opts)
	if err != nil {
		if ce, ok := err.(*compiler.Exception); ok {
			return &js.Error{Name: "Error", Message: ce.Message}
		}
		return err
	}
	t.compiled = compiled
	nullContext := js.NewObject()
	nullContext.Sealed = true
	c := &container{t: t, nullContext: nullContext}
	c.lookupFn = &js.Func{Name: "lookupProperty", Call: func(_ js.Value, args []js.Value) (js.Value, error) {
		key, err := js.ToString(arg(args, 1))
		if err != nil {
			return nil, err
		}
		return c.lookupProperty(arg(args, 0), key)
	}}
	c.noop = &js.Func{Name: "noop", Call: func(js.Value, []js.Value) (js.Value, error) { return "", nil }}
	t.c = c
	return nil
}

// call is the template function ret(context, options).
func (t *template) call(_ js.Value, args []js.Value) (js.Value, error) {
	context := arg(args, 0)
	options := arg(args, 1)
	if js.IsNullish(options) {
		options = js.NewObject()
	}
	if err := t.compile(); err != nil {
		return nil, err
	}
	c := t.c
	if err := c.setup(options); err != nil {
		return nil, err
	}
	main := t.compiled.Main
	data, err := get(options, "data")
	if err != nil {
		return nil, err
	}
	partial, err := get(options, "partial")
	if err != nil {
		return nil, err
	}
	if !js.ToBoolean(partial) && t.compiled.Options.Data {
		if data, err = initData(context, data); err != nil {
			return nil, err
		}
	}
	optDepths, err := get(options, "depths")
	if err != nil {
		return nil, err
	}
	var depths []js.Value
	if main.UseDepths {
		if od, ok := optDepths.(*js.Array); ok && js.ToBoolean(optDepths) {
			first := js.Undefined
			if len(od.Elems) > 0 {
				first = od.Elems[0]
			}
			same, err := js.LooseEquals(context, first)
			if err != nil {
				return nil, err
			}
			if !same {
				depths = append([]js.Value{context}, od.Elems...)
			} else {
				depths = append([]js.Value(nil), od.Elems...)
			}
		} else {
			depths = []js.Value{context}
		}
	}
	var blockParams []js.Value
	if main.UseBlockParams {
		blockParams = []js.Value{}
	}
	run := &js.Func{Name: "main", Call: func(_ js.Value, a []js.Value) (js.Value, error) {
		return c.run(main, arg(a, 0), data, blockParams, depths)
	}}
	decoratorDepths := []js.Value{}
	if od, ok := optDepths.(*js.Array); ok {
		decoratorDepths = od.Elems
	}
	run, err = c.executeDecorators(main, run, decoratorDepths, data, blockParams)
	if err != nil {
		return nil, err
	}
	result, err := run.Call(js.Undefined, []js.Value{context, options})
	if err != nil {
		return nil, err
	}
	s, err := js.ConcatString(result)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// setup is ret._setup: the helpers, partials and decorators of this call.
func (c *container) setup(options js.Value) error {
	partial, err := get(options, "partial")
	if err != nil {
		return err
	}
	if !js.ToBoolean(partial) {
		env := c.t.env
		merged := js.NewObject()
		for _, name := range env.Helpers.Keys() {
			merged.Set(name, c.wrapHelper(env.Helpers.Value(name)))
		}
		c.helpers = merged
		c.partials = env.Partials
		c.decorators = js.NewObject().Extend(env.Decorators)
		c.hooks = hookSet{}
		for _, name := range []string{"helperMissing", "blockHelperMissing"} {
			if h := merged.Value(name); js.ToBoolean(h) {
				c.hooks[name] = h
				merged.Set(name, js.Undefined)
			}
		}
		return nil
	}
	values := map[string]*js.Object{}
	for _, key := range []string{"helpers", "partials", "decorators"} {
		v, err := get(options, key)
		if err != nil {
			return err
		}
		o, _ := v.(*js.Object)
		values[key] = o
	}
	c.helpers, c.partials, c.decorators = values["helpers"], values["partials"], values["decorators"]
	hooks, err := get(options, "hooks")
	if err != nil {
		return err
	}
	if h, ok := hooks.(hookSet); ok {
		c.hooks = h
	}
	return nil
}

// hookSet carries the parent's hooks into a partial (the original's internal options.hooks).
type hookSet map[string]js.Value

// wrapHelper is passLookupPropertyOption: the helper gets options.lookupProperty.
func (c *container) wrapHelper(helper js.Value) js.Value {
	fn, ok := helper.(*js.Func)
	if !ok {
		return helper
	}
	return &js.Func{Name: "wrapper", Call: func(this js.Value, args []js.Value) (js.Value, error) {
		if len(args) > 0 {
			if o, ok := args[len(args)-1].(*js.Object); ok {
				o.Set("lookupProperty", c.lookupFn)
			} else if !js.IsObject(args[len(args)-1]) && !js.IsNullish(args[len(args)-1]) {
				s, _ := js.ToString(args[len(args)-1])
				return nil, js.TypeError("Cannot create property 'lookupProperty' on " + js.TypeOf(args[len(args)-1]) + " '" + s + "'")
			}
		}
		return fn.Call(this, args)
	}}
}

// initData adds @root.
func initData(context, data js.Value) (js.Value, error) {
	if js.ToBoolean(data) {
		if in, err := js.In("root", data); err != nil {
			return nil, err
		} else if in {
			return data, nil
		}
		frame := createFrame(data)
		frame.Set("root", context)
		return frame, nil
	}
	frame := js.NewObject()
	frame.Set("root", context)
	return frame, nil
}

// createFrame is Handlebars.createFrame.
func createFrame(object js.Value) *js.Object {
	frame := js.NewObject().Extend(object)
	frame.Set("_parent", object)
	return frame
}

// makeProgram is container.program / wrapProgram: the function of a block body bound to the data, block
// parameters and depths of the place it is created.
func (c *container) makeProgram(p *compiler.Program, data js.Value, blockParams, depths []js.Value) (*js.Func, error) {
	prog := &js.Func{Name: "prog"}
	prog.Call = func(_ js.Value, args []js.Value) (js.Value, error) {
		context := arg(args, 0)
		options := arg(args, 1)
		if options == js.Undefined || options == nil {
			options = js.NewObject()
		}
		current := depths
		if depths != nil {
			var first js.Value = js.Undefined
			if len(depths) > 0 {
				first = depths[0]
			}
			same, err := js.LooseEquals(context, first)
			if err != nil {
				return nil, err
			}
			if !same && !(context == js.Value(c.nullContext) && first == js.Null) {
				current = append([]js.Value{context}, depths...)
			}
		}
		d, err := get(options, "data")
		if err != nil {
			return nil, err
		}
		if !js.ToBoolean(d) {
			d = data
		}
		var bp []js.Value
		if blockParams != nil {
			frame, err := get(options, "blockParams")
			if err != nil {
				return nil, err
			}
			bp = append([]js.Value{frame}, blockParams...)
		}
		return c.run(p, context, d, bp, current)
	}
	decorated, err := c.executeDecorators(p, prog, depths, data, blockParams)
	if err != nil {
		return nil, err
	}
	decorated.SetProp("blockParams", float64(len(p.Node.BlockParams)))
	return decorated, nil
}

// executeDecorators runs a program's decorators, which may replace its function.
func (c *container) executeDecorators(p *compiler.Program, fn *js.Func, depths []js.Value, data js.Value,
	blockParams []js.Value) (*js.Func, error) {
	if len(p.Decorators) == 0 {
		return fn, nil
	}
	props := js.NewObject()
	var depth0 js.Value = js.Undefined
	if len(depths) > 0 {
		depth0 = depths[0]
	}
	// The decorator function has container, depth0, data, blockParams and depths, but no helpers.
	s := &scope{c: c, prog: p, ctx: depth0, partials: c.partials, data: data, blockParams: blockParams,
		depths: depths}
	for _, stmt := range p.Decorators {
		var path ast.Expression
		var params []ast.Expression
		var hash *ast.Hash
		var program *ast.Program
		switch n := stmt.(type) {
		case *ast.MustacheStatement:
			path, params, hash = n.Path, n.Params, n.Hash
		case *ast.BlockStatement:
			path, params, hash, program = n.Path, n.Params, n.Hash, n.Program
		}
		name := pathOriginal(path)
		decorator, err := c.lookupProperty(c.decorators, name)
		if err != nil {
			return nil, err
		}
		dfn, ok := decorator.(*js.Func)
		if !ok {
			return nil, &js.Error{Name: "Error", Message: `Missing decorator: "` + name + `"`}
		}
		options, err := s.setupParams(stmt, name, params, hash, program, nil, true)
		if err != nil {
			return nil, err
		}
		r, err := dfn.Call(js.Undefined, []js.Value{fn, props, c.containerValue(), options})
		if err != nil {
			return nil, err
		}
		if rf, ok := r.(*js.Func); ok {
			fn = rf
		}
	}
	if props.Len() > 0 {
		for _, k := range props.Keys() {
			fn.SetProp(k, props.Value(k))
		}
	}
	return fn, nil
}

// containerValue passes the container to decorators as an opaque value; the inline decorator swaps its partials.
func (c *container) containerValue() js.Value {
	return c
}

func pathOriginal(e ast.Expression) string {
	if p, ok := e.(*ast.PathExpression); ok {
		return p.Original
	}
	return ""
}

func arg(args []js.Value, i int) js.Value {
	if i < len(args) && args[i] != nil {
		return args[i]
	}
	return js.Undefined
}

// get is the property read v[key] (prototype placeholders read as undefined values of their kind).
func get(v js.Value, key string) (js.Value, error) {
	r, _, err := js.Get(v, key)
	if err != nil {
		return nil, err
	}
	return r, nil
}
