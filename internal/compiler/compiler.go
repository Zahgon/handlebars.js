// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

// Package compiler is the compile step of Handlebars (lib/handlebars/compiler/compiler.js together with the
// decisions javascript-compiler.js takes while generating code): it classifies every mustache, block and
// subexpression as a helper call, a simple lookup or an ambiguous one, resolves block parameter references,
// records which programs need depths, block parameters and decorators, and raises the compile-time errors.
// Package runtime executes the annotated tree with the semantics of the code the original generates.
package compiler

import (
	"strconv"

	"github.com/handlebars-lang/handlebars.js/internal/ast"
	"github.com/handlebars-lang/handlebars.js/internal/js"
)

// Options are the compile options. The structure is shared with the partials a template compiles at run time,
// which copy it, as the original passes its (mutated) options object on.
type Options struct {
	// KnownHelpers holds the knownHelpers option as given; the built-in helpers are added at compile time.
	KnownHelpers           map[string]bool
	KnownHelpersOnly       bool
	Strict                 bool
	AssumeObjects          bool
	PreventIndent          bool
	NoEscape               bool
	ExplicitPartialContext bool
	Compat                 bool
	IgnoreStandalone       bool
	// Data is the data option. It defaults to true and is switched on by any @-path in the template.
	Data bool
}

// Clone returns a shallow copy, as the original's extend({}, options) makes.
func (o *Options) Clone() *Options {
	c := *o
	return &c
}

// Kind is how a mustache, block or subexpression is evaluated.
type Kind int

// The kinds classifySexpr distinguishes.
const (
	Simple Kind = iota
	Helper
	Ambiguous
)

// Program is the compile-time information about one program (the template or a block body).
type Program struct {
	Node *ast.Program
	// UseBlockParams is set when the program or a program inside it reads a block parameter.
	UseBlockParams bool
	// UseDepths is set when the program or a program inside it uses `..`, a decorator, or compat mode is on.
	UseDepths bool
	// Decorators are the program's {{*decorator}} and {{#*decorator}} statements in source order.
	Decorators []ast.Statement
	// IsSimple marks a body of exactly one statement.
	IsSimple bool
}

// BlockParamRef locates a block parameter: the program depth counting outward and the index of the name.
type BlockParamRef struct {
	Depth, Index int
}

// Compiled is a template after the compile step.
type Compiled struct {
	Options        *Options
	Main           *Program
	Programs       map[*ast.Program]*Program
	Kinds          map[ast.Node]Kind
	BlockParamRefs map[*ast.PathExpression]BlockParamRef
	// KnownHelpers is the effective knownHelpers table (the built-ins plus the option).
	KnownHelpers map[string]bool
}

// Exception is a Handlebars Exception raised while compiling.
type Exception struct {
	Message string
}

func (e *Exception) Error() string { return e.Message }

// NewException builds the original's Exception(message, node): a location appends " - line:column".
func NewException(message string, loc *ast.SourceLocation) *Exception {
	return &Exception{Message: ast.ExceptionMessage(message, loc)}
}

type compiler struct {
	out         *Compiled
	stack       []*Program
	blockParams [][]string
}

// Compile runs the compile step over a parsed template. opts is updated in place as the original updates its
// options object (an @-path turns data on).
func Compile(program *ast.Program, opts *Options) (*Compiled, error) {
	known := map[string]bool{"helperMissing": true, "blockHelperMissing": true, "each": true, "if": true,
		"unless": true, "with": true, "log": true, "lookup": true}
	for name, v := range opts.KnownHelpers {
		known[name] = v
	}
	c := &compiler{out: &Compiled{
		Options:        opts,
		Programs:       map[*ast.Program]*Program{},
		Kinds:          map[ast.Node]Kind{},
		BlockParamRefs: map[*ast.PathExpression]BlockParamRef{},
		KnownHelpers:   known,
	}}
	main, err := c.compileProgram(program)
	if err != nil {
		return nil, err
	}
	c.out.Main = main
	return c.out, nil
}

func (c *compiler) current() *Program {
	return c.stack[len(c.stack)-1]
}

func (c *compiler) compileProgram(node *ast.Program) (*Program, error) {
	p := &Program{Node: node, UseDepths: c.out.Options.Compat}
	c.out.Programs[node] = p
	c.stack = append(c.stack, p)
	c.blockParams = append([][]string{node.BlockParams}, c.blockParams...)
	for _, stmt := range node.Body {
		if err := c.statement(stmt); err != nil {
			return nil, err
		}
	}
	c.blockParams = c.blockParams[1:]
	c.stack = c.stack[:len(c.stack)-1]
	p.IsSimple = len(node.Body) == 1
	if len(p.Decorators) > 0 {
		p.UseDepths = true
	}
	if len(c.stack) > 0 {
		parent := c.current()
		parent.UseDepths = parent.UseDepths || p.UseDepths
		parent.UseBlockParams = parent.UseBlockParams || p.UseBlockParams
	}
	return p, nil
}

func (c *compiler) statement(stmt ast.Statement) error {
	switch n := stmt.(type) {
	case *ast.BlockStatement:
		if n.Decorator {
			return c.decoratorBlock(n)
		}
		return c.block(n)
	case *ast.MustacheStatement:
		if n.Decorator {
			if err := c.fullParams(n.Params, n.Hash); err != nil {
				return err
			}
			c.current().Decorators = append(c.current().Decorators, n)
			return nil
		}
		return c.sexpr(n, n.Path, n.Params, n.Hash, false)
	case *ast.PartialStatement:
		return c.partial(n, n.Name, n.Params, n.Hash, nil)
	case *ast.PartialBlockStatement:
		return c.partial(n, n.Name, n.Params, n.Hash, n.Program)
	}
	return nil
}

func (c *compiler) block(b *ast.BlockStatement) error {
	transformLiteralToPath(&b.Path)
	if b.Program != nil {
		if _, err := c.compileProgram(b.Program); err != nil {
			return err
		}
	}
	if b.Inverse != nil {
		if _, err := c.compileProgram(b.Inverse); err != nil {
			return err
		}
	}
	return c.sexpr(b, b.Path, b.Params, b.Hash, true)
}

func (c *compiler) decoratorBlock(b *ast.BlockStatement) error {
	if b.Program != nil {
		if _, err := c.compileProgram(b.Program); err != nil {
			return err
		}
	}
	if err := c.fullParams(b.Params, b.Hash); err != nil {
		return err
	}
	c.current().Decorators = append(c.current().Decorators, b)
	return nil
}

func (c *compiler) partial(node ast.Node, name ast.Expression, params []ast.Expression, hash *ast.Hash,
	program *ast.Program) error {
	if program != nil {
		if _, err := c.compileProgram(program); err != nil {
			return err
		}
	}
	if len(params) > 1 {
		return NewException("Unsupported number of partial arguments: "+strconv.Itoa(len(params)), node.Location())
	}
	if sub, ok := name.(*ast.SubExpression); ok {
		if err := c.subExpression(sub); err != nil {
			return err
		}
	}
	return c.fullParams(params, hash)
}

// sexpr is the original's SubExpression / BlockStatement classification.
func (c *compiler) sexpr(node ast.Node, path ast.Expression, params []ast.Expression, hash *ast.Hash, block bool) error {
	if m, ok := node.(*ast.MustacheStatement); ok {
		transformLiteralToPath(&m.Path)
		path = m.Path
	}
	p := path.(*ast.PathExpression)
	kind := c.classify(node, p)
	c.out.Kinds[node] = kind
	switch kind {
	case Helper:
		if err := c.fullParams(params, hash); err != nil {
			return err
		}
		name := ""
		if len(p.Parts) > 0 {
			name = p.Parts[0]
		}
		if c.out.KnownHelpers[name] {
			return nil
		}
		if c.out.Options.KnownHelpersOnly {
			return NewException("You specified knownHelpersOnly, but used the unknown helper "+name, node.Location())
		}
		p.Strict = true
		p.Falsy = true
		c.path(p)
	case Simple:
		p.Strict = true
		c.path(p)
	default:
		p.Strict = true
		c.path(p)
	}
	return nil
}

func (c *compiler) subExpression(s *ast.SubExpression) error {
	transformLiteralToPath(&s.Path)
	return c.sexpr(s, s.Path, s.Params, s.Hash, false)
}

func (c *compiler) classify(node ast.Node, p *ast.PathExpression) Kind {
	isSimple := ast.SimpleID(p)
	isBlockParam := isSimple && c.blockParamIndex(p.Parts[0]) != nil
	isHelper := !isBlockParam && ast.HelperExpression(node)
	isEligible := !isBlockParam && (isHelper || isSimple)
	if isEligible && !isHelper {
		if c.out.KnownHelpers[p.Parts[0]] {
			isHelper = true
		} else if c.out.Options.KnownHelpersOnly {
			isEligible = false
		}
	}
	switch {
	case isHelper:
		return Helper
	case isEligible:
		return Ambiguous
	}
	return Simple
}

func (c *compiler) fullParams(params []ast.Expression, hash *ast.Hash) error {
	for _, param := range params {
		if err := c.expression(param); err != nil {
			return err
		}
	}
	if hash != nil {
		for _, pair := range hash.Pairs {
			if err := c.expression(pair.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *compiler) expression(e ast.Expression) error {
	switch n := e.(type) {
	case *ast.SubExpression:
		return c.subExpression(n)
	case *ast.PathExpression:
		c.path(n)
	}
	return nil
}

// path records what the original's PathExpression visitor does: depths, block parameters and data.
func (c *compiler) path(p *ast.PathExpression) {
	prog := c.current()
	if p.Depth > 0 {
		prog.UseDepths = true
	}
	name := ""
	if len(p.Parts) > 0 {
		name = p.Parts[0]
	}
	if p.Depth == 0 && !ast.ScopedID(p) {
		if ref := c.blockParamIndex(name); ref != nil {
			c.out.BlockParamRefs[p] = *ref
			prog.UseBlockParams = true
			return
		}
	}
	if name != "" && p.Data {
		c.out.Options.Data = true
	}
}

func (c *compiler) blockParamIndex(name string) *BlockParamRef {
	for depth, params := range c.blockParams {
		for i, p := range params {
			if p == name {
				return &BlockParamRef{Depth: depth, Index: i}
			}
		}
	}
	return nil
}

// transformLiteralToPath turns a literal used as a helper name ({{"foo"}}, {{1}}, {{true}}) into a path whose
// single part is the literal's text.
func transformLiteralToPath(e *ast.Expression) {
	var text string
	var loc *ast.SourceLocation
	switch lit := (*e).(type) {
	case *ast.PathExpression:
		return
	case *ast.StringLiteral:
		text, loc = lit.Value, lit.Loc
	case *ast.NumberLiteral:
		text, loc = js.NumberToString(lit.Value), lit.Loc
	case *ast.BooleanLiteral:
		text, loc = strconv.FormatBool(lit.Value), lit.Loc
	case *ast.UndefinedLiteral:
		text, loc = "undefined", lit.Loc
	case *ast.NullLiteral:
		text, loc = "null", lit.Loc
	default:
		return
	}
	*e = &ast.PathExpression{Parts: []string{text}, Original: text, Loc: loc}
}
