// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

// Package parser turns Handlebars template text into the syntax tree of package ast, as the original's
// jison-generated parser (src/handlebars.l, src/handlebars.yy) and its whitespace control do. Error messages,
// including the parser's expectation lists and source context, are the original's byte for byte.
package parser

import (
	"sort"
	"strings"

	"github.com/handlebars-lang/handlebars.js/internal/ast"
)

// Error is a failure to parse a template. Name is the JavaScript error class the original throws ("Error", or
// "TypeError" where the original fails inside its own code); Message is its message.
type Error struct {
	Name    string
	Message string
	// Loc is the node location an Exception carries, when it has one.
	Loc *ast.SourceLocation
}

func (e *Error) Error() string { return e.Message }

// exception mirrors the original's Exception(message, node): a node location appends " - line:column".
func exception(message string, loc *ast.SourceLocation) *Error {
	return &Error{Name: "Error", Message: ast.ExceptionMessage(message, loc), Loc: loc}
}

// Options are the parse options the original honours.
type Options struct {
	// IgnoreStandalone disables the removal of standalone lines around block tags, partials and comments.
	IgnoreStandalone bool
}

// Parse parses a template and applies whitespace control, like Handlebars.parse.
func Parse(template string, opts Options) (*ast.Program, error) {
	program, err := ParseWithoutProcessing(template)
	if err != nil {
		return nil, err
	}
	if err := applyWhitespaceControl(program, opts); err != nil {
		return nil, err
	}
	return program, nil
}

// ParseWithoutProcessing parses a template without whitespace control, like Handlebars.parseWithoutProcessing.
func ParseWithoutProcessing(template string) (program *ast.Program, err error) {
	p := &parser{lex: newLexer(encode(template))}
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*Error)
			if !ok {
				panic(r)
			}
			program, err = nil, e
		}
	}()
	return p.parse()
}

// Parser actions (jison's table entries).
const (
	actShift = iota + 1
	actReduce
	actAccept
	actGoto
)

type action struct {
	kind int
	arg  int
}

type parser struct {
	lex *lexer
}

// parse is jison's LALR driver. The parser's own line counter (for "Parse error on line N") is the lexer's line
// count after the last shifted token.
func (p *parser) parse() (*ast.Program, error) {
	stack := []int{0}
	var vstack []any
	vstack = append(vstack, nil)
	lstack := []yylloc{p.lex.loc}
	yylineno := 0
	symbol, haveSymbol := 0, false
	for {
		state := stack[len(stack)-1]
		act, isDefault := defaultActions[state]
		if !isDefault {
			if !haveSymbol {
				tok, err := p.lex.lex()
				if err != nil {
					return nil, err
				}
				symbol, haveSymbol = tok, true
			}
			var found bool
			act, found = parseTable[state][symbol]
			if !found || act.kind == actGoto {
				return nil, p.parseError(state, symbol, yylineno)
			}
		}
		switch act.kind {
		case actShift:
			stack = append(stack, symbol, act.arg)
			vstack = append(vstack, decode(p.lex.yytext))
			lstack = append(lstack, p.lex.loc)
			haveSymbol = false
			yylineno = p.lex.yylineno
		case actReduce:
			lhs, n := productions[act.arg][0], productions[act.arg][1]
			first := lstack[len(lstack)-max(n, 1)]
			last := lstack[len(lstack)-1]
			loc := yylloc{firstLine: first.firstLine, lastLine: last.lastLine, firstColumn: first.firstColumn,
				lastColumn: last.lastColumn}
			var value any
			if len(vstack)-n < len(vstack) {
				value = vstack[len(vstack)-n]
			}
			result, done := p.reduce(act.arg, vstack[len(vstack)-n:], value, loc)
			if done {
				program, _ := result.(*ast.Program)
				return program, nil
			}
			stack = stack[:len(stack)-2*n]
			vstack = vstack[:len(vstack)-n]
			lstack = lstack[:len(lstack)-n]
			stack = append(stack, lhs)
			vstack = append(vstack, result)
			lstack = append(lstack, loc)
			next := parseTable[stack[len(stack)-2]][lhs]
			stack = append(stack, next.arg)
		case actAccept:
			return nil, nil
		}
	}
}

// parseError builds jison's message: the expected terminals of the state in symbol order, and the token found.
func (p *parser) parseError(state, symbol, yylineno int) error {
	var ids []int
	for id := range parseTable[state] {
		if _, ok := terminalNames[id]; ok && id > 2 {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	expected := make([]string, len(ids))
	for i, id := range ids {
		expected[i] = "'" + terminalNames[id] + "'"
	}
	got := tokenName(symbol)
	msg := "Parse error on line " + itoa(yylineno+1) + ":\n" + p.lex.showPosition() + "\nExpecting " +
		strings.Join(expected, ", ") + ", got '" + got + "'"
	return &Error{Name: "Error", Message: msg}
}

func tokenName(symbol int) string {
	if symbol == tokInvalid {
		return "INVALID"
	}
	if name, ok := terminalNames[symbol]; ok {
		return name
	}
	return itoa(symbol)
}

func locInfo(l yylloc) *ast.SourceLocation {
	return &ast.SourceLocation{
		Start: ast.Position{Line: l.firstLine, Column: l.firstColumn},
		End:   ast.Position{Line: l.lastLine, Column: l.lastColumn},
	}
}

// Intermediate grammar values.

type segment struct {
	part, original, separator string
}

type openRaw struct {
	path   ast.Expression
	params []ast.Expression
	hash   *ast.Hash
}

type openBlockInfo struct {
	open        string
	path        ast.Expression
	params      []ast.Expression
	hash        *ast.Hash
	blockParams []string
	strip       *ast.StripFlags
}

type inverseInfo struct {
	strip   *ast.StripFlags
	program *ast.Program
	chain   bool
}

type closeInfo struct {
	path  ast.Expression
	strip *ast.StripFlags
}

// reduce runs the semantic action of a production. value is jison's default $$ ($1, or undefined for an empty
// production). It returns done for the accepting production.
func (p *parser) reduce(production int, rhs []any, value any, loc yylloc) (any, bool) {
	arg := func(i int) any { return rhs[i] }
	str := func(i int) string { s, _ := rhs[i].(string); return s }
	exprs := func(i int) []ast.Expression { e, _ := rhs[i].([]ast.Expression); return e }
	hash := func(i int) *ast.Hash { h, _ := rhs[i].(*ast.Hash); return h }
	params := func(i int) []string { b, _ := rhs[i].([]string); return b }
	switch production {
	case 1:
		return arg(0), true
	case 2:
		stmts, _ := rhs[0].([]ast.Statement)
		return prepareProgram(stmts, nil), false
	case 9:
		return &ast.CommentStatement{Value: stripComment(str(0)), Strip: *stripFlags(str(0), str(0)),
			Loc: locInfo(loc)}, false
	case 10:
		return &ast.ContentStatement{Original: str(0), Value: str(0), Loc: locInfo(loc)}, false
	case 11:
		contents, _ := rhs[1].([]ast.Statement)
		return prepareRawBlock(rhs[0].(*openRaw), contents, str(2), loc), false
	case 12:
		return &openRaw{path: rhs[1].(ast.Expression), params: exprs(2), hash: hash(3)}, false
	case 13, 14:
		var inverse *inverseInfo
		if v, ok := rhs[2].(*inverseInfo); ok {
			inverse = v
		}
		closer := rhs[3].(*closeInfo)
		return prepareBlock(rhs[0].(*openBlockInfo), rhs[1].(*ast.Program), inverse, closer, closer.strip,
			production == 14, loc), false
	case 15:
		return &openBlockInfo{open: str(0), path: rhs[1].(ast.Expression), params: exprs(2), hash: hash(3),
			blockParams: params(4), strip: stripFlags(str(0), str(5))}, false
	case 16, 17:
		return &openBlockInfo{path: rhs[1].(ast.Expression), params: exprs(2), hash: hash(3),
			blockParams: params(4), strip: stripFlags(str(0), str(5))}, false
	case 18:
		return &inverseInfo{strip: stripFlags(str(0), str(0)), program: rhs[1].(*ast.Program)}, false
	case 19:
		var next *inverseInfo
		var nextStrip *ast.StripFlags
		if v, ok := rhs[2].(*inverseInfo); ok {
			next, nextStrip = v, v.strip
		}
		open := rhs[0].(*openBlockInfo)
		body := rhs[1].(*ast.Program)
		inverse := prepareBlock(open, body, next, nil, nextStrip, false, loc)
		program := prepareProgram([]ast.Statement{inverse}, body.Loc)
		program.Chained = true
		return &inverseInfo{strip: open.strip, program: program, chain: true}, false
	case 21:
		return &closeInfo{path: rhs[1].(ast.Expression), strip: stripFlags(str(0), str(2))}, false
	case 22, 23:
		return prepareMustache(rhs[1].(ast.Expression), exprs(2), hash(3), str(0), stripFlags(str(0), str(4)),
			loc), false
	case 24:
		return &ast.PartialStatement{Name: rhs[1].(ast.Expression), Params: exprs(2), Hash: hash(3),
			Strip: *stripFlags(str(0), str(4)), Loc: locInfo(loc)}, false
	case 25:
		return preparePartialBlock(rhs[0].(*openBlockInfo), rhs[1].(*ast.Program), rhs[2].(*closeInfo), loc), false
	case 26:
		return &openBlockInfo{path: rhs[1].(ast.Expression), params: exprs(2), hash: hash(3),
			strip: stripFlags(str(0), str(4))}, false
	case 29:
		return &ast.SubExpression{Path: rhs[1].(ast.Expression), Params: exprs(2), Hash: hash(3),
			Loc: locInfo(loc)}, false
	case 30:
		pairs, _ := rhs[0].([]*ast.HashPair)
		return &ast.Hash{Pairs: pairs, Loc: locInfo(loc)}, false
	case 31:
		return &ast.HashPair{Key: id(str(0)), Value: rhs[2].(ast.Expression), Loc: locInfo(loc)}, false
	case 32:
		ids, _ := rhs[1].([]string)
		return blockParamsID(ids), false
	case 35:
		return &ast.StringLiteral{Value: str(0), Loc: locInfo(loc)}, false
	case 36:
		return &ast.NumberLiteral{Value: jsNumber(str(0)), Loc: locInfo(loc)}, false
	case 37:
		return &ast.BooleanLiteral{Value: str(0) == "true", Loc: locInfo(loc)}, false
	case 38:
		return &ast.UndefinedLiteral{Loc: locInfo(loc)}, false
	case 39:
		return &ast.NullLiteral{Loc: locInfo(loc)}, false
	case 42:
		segs, _ := rhs[1].([]segment)
		return preparePath(true, segs, loc), false
	case 43:
		segs, _ := rhs[0].([]segment)
		return preparePath(false, segs, loc), false
	case 44:
		segs, _ := rhs[0].([]segment)
		return append(segs, segment{part: id(str(2)), original: str(2), separator: str(1)}), false
	case 45:
		return []segment{{part: id(str(0)), original: str(0)}}, false
	}
	return p.repetition(production, rhs, value), false
}

// repetition implements the list productions jison generates for `x*` and `x+` (start a list, append to it);
// every other production keeps jison's default value.
func (p *parser) repetition(production int, rhs []any, value any) any {
	switch lhsName(productions[production][0]) {
	case "program_repetition0", "rawBlock_repetition0":
		if len(rhs) == 0 {
			return []ast.Statement{}
		}
		return append(rhs[0].([]ast.Statement), rhs[1].(ast.Statement))
	case "hash_repetition_plus0":
		if len(rhs) == 1 {
			return []*ast.HashPair{rhs[0].(*ast.HashPair)}
		}
		return append(rhs[0].([]*ast.HashPair), rhs[1].(*ast.HashPair))
	case "blockParams_repetition_plus0":
		if len(rhs) == 1 {
			return []string{rhs[0].(string)}
		}
		return append(rhs[0].([]string), rhs[1].(string))
	}
	if strings.Contains(lhsName(productions[production][0]), "_repetition") {
		if len(rhs) == 0 {
			return []ast.Expression{}
		}
		return append(rhs[0].([]ast.Expression), rhs[1].(ast.Expression))
	}
	return value
}

var symbolNames = func() map[int]string {
	names := make(map[int]string, len(symbolNumbers))
	for name, n := range symbolNumbers {
		names[n] = name
	}
	return names
}()

func lhsName(symbol int) string { return symbolNames[symbol] }
