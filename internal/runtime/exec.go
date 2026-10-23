// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package runtime

import (
	"strings"

	"github.com/handlebars-lang/handlebars.js/internal/ast"
	"github.com/handlebars-lang/handlebars.js/internal/compiler"
	"github.com/handlebars-lang/handlebars.js/internal/js"
)

// scope is one call of a program function: fn(container, depth0, helpers, partials, data, blockParams, depths).
type scope struct {
	c           *container
	prog        *compiler.Program
	ctx         js.Value
	helpers     *js.Object
	partials    *js.Object
	data        js.Value
	blockParams []js.Value
	depths      []js.Value
}

// run executes a program body and returns its value. It follows the code the original generates: the values
// of consecutive appending statements are joined with JavaScript's + operator (so two numbers add up); the first
// such group initialises the buffer, a later one is added to it as a whole, and a group at the end is appended
// to it one by one. An ambiguous block is a statement of its own. A body of one statement returns that
// statement's value itself.
func (c *container) run(p *compiler.Program, ctx, data js.Value, blockParams, depths []js.Value) (js.Value, error) {
	s := &scope{c: c, prog: p, ctx: ctx, helpers: c.helpers, partials: c.partials, data: data,
		blockParams: blockParams, depths: depths}
	if p.IsSimple {
		v, _, err := s.statement(p.Node.Body[0])
		if err != nil {
			return nil, err
		}
		if v == nil {
			return "", nil
		}
		return v, nil
	}
	// The last ambiguous block splits the body: appends after it form the trailing group.
	lastStatement := -1
	for i, stmt := range p.Node.Body {
		if s.isStatement(stmt) {
			lastStatement = i
		}
	}
	var buffer js.Value = ""
	var group js.Value
	inGroup, leading := false, true
	closeGroup := func() error {
		if !inGroup {
			return nil
		}
		inGroup = false
		if leading {
			buffer = group
			return nil
		}
		v, err := js.Add(buffer, group)
		buffer = v
		return err
	}
	for i, stmt := range p.Node.Body {
		v, isStatement, err := s.statement(stmt)
		if err != nil {
			return nil, err
		}
		switch {
		case v == nil:
		case isStatement:
			if err := closeGroup(); err != nil {
				return nil, err
			}
			leading = false
			if !js.IsNullish(v) {
				if buffer, err = js.Add(buffer, v); err != nil {
					return nil, err
				}
			}
		case lastStatement >= 0 && i > lastStatement:
			// return buffer + a + b + ...
			if buffer, err = js.Add(buffer, v); err != nil {
				return nil, err
			}
		case !inGroup:
			group, inGroup = v, true
		default:
			if group, err = js.Add(group, v); err != nil {
				return nil, err
			}
		}
	}
	if err := closeGroup(); err != nil {
		return nil, err
	}
	return buffer, nil
}

// isStatement reports whether the generated code appends a statement's value with a statement of its own (an
// ambiguous block) rather than inside an expression.
func (s *scope) isStatement(stmt ast.Statement) bool {
	b, ok := stmt.(*ast.BlockStatement)
	return ok && !b.Decorator && s.c.t.compiled.Kinds[b] == compiler.Ambiguous
}

func (s *scope) options() *compiler.Options { return s.c.t.compiled.Options }

// statement evaluates one statement. It returns the value the generated code appends (nil when the statement
// appends nothing) and whether the statement is an ambiguous block, which the generated code appends with a
// statement of its own rather than inside an expression.
func (s *scope) statement(stmt ast.Statement) (js.Value, bool, error) {
	switch n := stmt.(type) {
	case *ast.ContentStatement:
		if n.Value == "" {
			return nil, false, nil
		}
		return n.Value, false, nil
	case *ast.MustacheStatement:
		if n.Decorator {
			return nil, false, nil
		}
		v, err := s.sexpr(n, n.Path, n.Params, n.Hash, nil, nil)
		if err != nil {
			return nil, false, err
		}
		if n.Escaped && !s.options().NoEscape {
			str, err := escapeExpression(v)
			return str, false, err
		}
		if js.IsNullish(v) {
			return "", false, nil
		}
		return v, false, nil
	case *ast.BlockStatement:
		if n.Decorator {
			return nil, false, nil
		}
		v, err := s.block(n)
		if err != nil {
			return nil, false, err
		}
		if s.c.t.compiled.Kinds[n] == compiler.Ambiguous {
			if js.IsNullish(v) && s.prog.IsSimple {
				return "", true, nil
			}
			return v, true, nil
		}
		if js.IsNullish(v) {
			return "", false, nil
		}
		return v, false, nil
	case *ast.PartialStatement:
		v, err := s.partial(n, n.Name, n.Params, n.Hash, nil, n.Indent)
		return v, false, err
	case *ast.PartialBlockStatement:
		v, err := s.partial(n, n.Name, n.Params, n.Hash, n.Program, "")
		return v, false, err
	}
	return nil, false, nil
}

func (s *scope) callContext() js.Value {
	if js.IsNullish(s.ctx) {
		return s.c.nullContext
	}
	return s.ctx
}

func (s *scope) program(p *ast.Program) (js.Value, error) {
	if p == nil {
		return nil, nil
	}
	child := s.c.t.compiled.Programs[p]
	var bp, depths []js.Value
	if s.prog.UseBlockParams || s.prog.UseDepths {
		bp = s.blockParams
	}
	depths = s.depths
	return s.c.makeProgram(child, s.data, bp, depths)
}

// setupParams builds the options object of a helper, block helper, decorator or partial call. Parameters are
// evaluated first, left to right, then the hash values.
func (s *scope) setupParamsWith(node ast.Node, name js.Value, params []ast.Expression, hash *ast.Hash,
	program, inverse *ast.Program, objectArgs, omitEmptyHash bool) (*js.Object, []js.Value, error) {
	values := make([]js.Value, len(params))
	for i, p := range params {
		v, err := s.expression(p)
		if err != nil {
			return nil, nil, err
		}
		values[i] = v
	}
	options := js.NewObject()
	if name != nil {
		options.Set("name", name)
	}
	hashValue, err := s.hash(hash, omitEmptyHash)
	if err != nil {
		return nil, nil, err
	}
	options.Set("hash", hashValue)
	if program != nil || inverse != nil {
		fn, err := s.program(program)
		if err != nil {
			return nil, nil, err
		}
		inv, err := s.program(inverse)
		if err != nil {
			return nil, nil, err
		}
		if fn == nil {
			fn = s.c.noop
		}
		if inv == nil {
			inv = s.c.noop
		}
		options.Set("fn", fn)
		options.Set("inverse", inv)
	}
	if objectArgs {
		options.Set("args", js.NewArray(values...))
	}
	if s.options().Data {
		options.Set("data", s.data)
	}
	if s.prog.UseBlockParams {
		options.Set("blockParams", s.blockParamsValue())
	}
	return options, values, nil
}

func (s *scope) blockParamsValue() js.Value {
	if s.blockParams == nil {
		return js.Undefined
	}
	return js.NewArray(s.blockParams...)
}

func (s *scope) setupParams(node ast.Node, name string, params []ast.Expression, hash *ast.Hash,
	program, inverse *ast.Program, objectArgs bool) (*js.Object, error) {
	options, _, err := s.setupParamsWith(node, name, params, hash, program, inverse, objectArgs, false)
	if err != nil {
		return nil, err
	}
	options.Set("loc", locValue(node.Location()))
	return options, nil
}

func locValue(loc *ast.SourceLocation) js.Value {
	if loc == nil {
		return js.Undefined
	}
	pos := func(p ast.Position) *js.Object {
		o := js.NewObject()
		o.Set("line", float64(p.Line))
		o.Set("column", float64(p.Column))
		return o
	}
	o := js.NewObject()
	o.Set("start", pos(loc.Start))
	o.Set("end", pos(loc.End))
	return o
}

// hash evaluates a hash. The generated code assigns the pairs in reverse order into an object literal, so a
// repeated key keeps the first occurrence's value and keys appear last-to-first (array-index keys first, as in
// every JavaScript object); `__proto__` in an object literal sets the prototype instead of adding a key.
func (s *scope) hash(h *ast.Hash, omitEmpty bool) (js.Value, error) {
	if h == nil {
		if omitEmpty {
			return js.Undefined, nil
		}
		return js.NewObject(), nil
	}
	exprs := map[string]ast.Expression{}
	order := js.NewObject()
	for i := len(h.Pairs) - 1; i >= 0; i-- {
		pair := h.Pairs[i]
		exprs[pair.Key] = pair.Value
		order.Set(pair.Key, js.Undefined)
	}
	result := js.NewObject()
	for _, key := range order.Keys() {
		v, err := s.expression(exprs[key])
		if err != nil {
			return nil, err
		}
		if key == "__proto__" {
			continue
		}
		result.Set(key, v)
	}
	return result, nil
}

// expression evaluates a parameter or hash value.
func (s *scope) expression(e ast.Expression) (js.Value, error) {
	switch n := e.(type) {
	case *ast.PathExpression:
		return s.path(n)
	case *ast.SubExpression:
		return s.sexpr(n, n.Path, n.Params, n.Hash, nil, nil)
	case *ast.StringLiteral:
		return n.Value, nil
	case *ast.NumberLiteral:
		return n.Value, nil
	case *ast.BooleanLiteral:
		return n.Value, nil
	case *ast.UndefinedLiteral:
		return js.Undefined, nil
	case *ast.NullLiteral:
		return js.Null, nil
	}
	return js.Undefined, nil
}

// sexpr evaluates a mustache or subexpression according to its compile-time kind.
func (s *scope) sexpr(node ast.Node, pathExpr ast.Expression, params []ast.Expression, hash *ast.Hash,
	program, inverse *ast.Program) (js.Value, error) {
	path := pathExpr.(*ast.PathExpression)
	switch s.c.t.compiled.Kinds[node] {
	case compiler.Helper:
		return s.helperCall(node, path, params, hash, program, inverse)
	case compiler.Ambiguous:
		v, _, err := s.ambiguous(node, path, program, inverse)
		return v, err
	}
	v, err := s.path(path)
	if err != nil {
		return nil, err
	}
	return s.lambda(v)
}

// lambda is container.lambda: a function value is called with the context as `this`.
func (s *scope) lambda(v js.Value) (js.Value, error) {
	if f, ok := v.(*js.Func); ok {
		return f.Call(s.ctx, nil)
	}
	return v, nil
}

func (s *scope) block(b *ast.BlockStatement) (js.Value, error) {
	path := b.Path.(*ast.PathExpression)
	switch s.c.t.compiled.Kinds[b] {
	case compiler.Helper:
		return s.helperCall(b, path, b.Params, b.Hash, b.Program, b.Inverse)
	case compiler.Ambiguous:
		v, options, err := s.ambiguous(b, path, b.Program, b.Inverse)
		if err != nil {
			return nil, err
		}
		found, err := s.c.lookupProperty(s.helpers, firstPart(path))
		if err != nil {
			return nil, err
		}
		if !js.ToBoolean(found) {
			return s.c.callHook("blockHelperMissing", s.ctx, []js.Value{v, options})
		}
		return v, nil
	}
	v, err := s.path(path)
	if err != nil {
		return nil, err
	}
	if v, err = s.lambda(v); err != nil {
		return nil, err
	}
	options, err := s.setupParams(b, path.Original, nil, nil, b.Program, b.Inverse, false)
	if err != nil {
		return nil, err
	}
	return s.c.callHook("blockHelperMissing", s.ctx, []js.Value{v, options})
}

func (c *container) callHook(name string, this js.Value, args []js.Value) (js.Value, error) {
	h, ok := c.hooks[name].(*js.Func)
	if !ok {
		return nil, js.TypeError("Cannot read properties of undefined (reading 'call')")
	}
	return h.Call(this, args)
}

func firstPart(p *ast.PathExpression) string {
	if len(p.Parts) > 0 {
		return p.Parts[0]
	}
	return "undefined"
}

// ambiguous is invokeAmbiguous: a helper of that name, else the value, which is called when it is a function.
// It returns the options object too, which an ambiguous block hands on to blockHelperMissing.
func (s *scope) ambiguous(node ast.Node, path *ast.PathExpression, program, inverse *ast.Program) (js.Value, js.Value, error) {
	name := firstPart(path)
	if s.helpers == nil {
		return nil, nil, errNoHelpers
	}
	helper, err := s.c.lookupProperty(s.helpers, name)
	if err != nil {
		return nil, nil, err
	}
	if !js.ToBoolean(helper) {
		if helper, err = s.path(path); err != nil {
			return nil, nil, err
		}
	}
	if !s.options().Strict && js.IsNullish(helper) {
		helper = s.c.hooks["helperMissing"]
		if helper == nil {
			helper = js.Undefined
		}
	}
	options, err := s.setupParams(node, name, nil, nil, program, inverse, false)
	if err != nil {
		return nil, nil, err
	}
	if f, ok := helper.(*js.Func); ok {
		v, err := f.Call(s.callContext(), []js.Value{options})
		return v, options, err
	}
	return helper, options, nil
}

// helperCall is invokeKnownHelper / invokeHelper.
func (s *scope) helperCall(node ast.Node, path *ast.PathExpression, params []ast.Expression, hash *ast.Hash,
	program, inverse *ast.Program) (js.Value, error) {
	name := firstPart(path)
	simple := ast.SimpleID(path)
	if s.helpers == nil && (simple || s.c.t.compiled.KnownHelpers[name]) {
		return nil, errNoHelpers
	}
	if s.c.t.compiled.KnownHelpers[name] {
		helper, err := s.c.lookupProperty(s.helpers, name)
		if err != nil {
			return nil, err
		}
		if js.IsNullish(helper) {
			return nil, js.TypeError("Cannot read properties of " + nullishName(helper) + " (reading 'call')")
		}
		options, values, err := s.setupParamsWith(node, name, params, hash, program, inverse, false, false)
		if err != nil {
			return nil, err
		}
		options.Set("loc", locValue(node.Location()))
		f, ok := helper.(*js.Func)
		if !ok {
			return nil, js.TypeError("lookupProperty(...).call is not a function")
		}
		return f.Call(s.callContext(), append(values, options))
	}
	var callee js.Value = js.Undefined
	if simple {
		h, err := s.c.lookupProperty(s.helpers, name)
		if err != nil {
			return nil, err
		}
		callee = h
	}
	if !js.ToBoolean(callee) {
		v, err := s.path(path)
		if err != nil {
			return nil, err
		}
		callee = v
		if !js.ToBoolean(callee) && !s.options().Strict {
			callee = s.c.hooks["helperMissing"]
			if callee == nil {
				callee = js.Undefined
			}
		}
	}
	if js.IsNullish(callee) {
		return nil, js.TypeError("Cannot read properties of " + nullishName(callee) + " (reading 'call')")
	}
	options, values, err := s.setupParamsWith(node, path.Original, params, hash, program, inverse, false, false)
	if err != nil {
		return nil, err
	}
	options.Set("loc", locValue(node.Location()))
	f, ok := callee.(*js.Func)
	if !ok {
		return nil, js.TypeError(s.calleeText(path, simple) + ".call is not a function")
	}
	return f.Call(s.callContext(), append(values, options))
}

// errNoHelpers is what the generated decorator function throws when a parameter calls a helper: it has no
// `helpers` variable.
var errNoHelpers = &js.Error{Name: "ReferenceError", Message: "helpers is not defined"}

func nullishName(v js.Value) string {
	if v == js.Null {
		return "null"
	}
	return "undefined"
}

// calleeText is how V8 prints the callee `(helpers.x || <path lookup> || helperMissing)` of a failed call.
func (s *scope) calleeText(path *ast.PathExpression, simple bool) string {
	var pieces []string
	if simple {
		pieces = append(pieces, "lookupProperty(...)")
	}
	pieces = append(pieces, s.pathText(path))
	if !s.options().Strict {
		pieces = append(pieces, "container.hooks.helperMissing")
	}
	if len(pieces) == 1 {
		return pieces[0]
	}
	return "(" + strings.Join(pieces, " || ") + ")"
}

// pathText is V8's rendering of the code generated for a helper-name lookup.
func (s *scope) pathText(p *ast.PathExpression) string {
	opts := s.options()
	if _, ok := s.c.t.compiled.BlockParamRefs[p]; ok {
		if len(p.Parts) == 1 {
			return "blockParams[...][...]"
		}
		if opts.Strict || opts.AssumeObjects {
			return "lookupProperty(...)"
		}
		return "(intermediate value)(intermediate value)(intermediate value)"
	}
	base := "depth0"
	switch {
	case p.Data && p.Depth > 0:
		base = "container.data(...)"
	case p.Data:
		base = "data"
	case p.Depth > 0:
		base = "depths[" + itoa(p.Depth) + "]"
	}
	parts := len(p.Parts)
	if !p.Data && !ast.ScopedID(p) && opts.Compat && p.Depth == 0 && parts > 0 {
		base = "container.lookup(...)"
		parts--
	}
	if opts.Strict || opts.AssumeObjects {
		if opts.Strict && p.Strict && len(p.Parts) > 0 {
			return "container.strict(...)"
		}
		if parts == 0 {
			return base
		}
		return "lookupProperty(...)"
	}
	switch {
	case parts == 0:
		return base
	case parts == 1 && base != "container.lookup(...)":
		return "(" + base + " && lookupProperty(...))"
	}
	return "(stack1 && lookupProperty(...))"
}
