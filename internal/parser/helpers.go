// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package parser

import (
	"math"
	"strconv"
	"strings"

	"github.com/handlebars-lang/handlebars.js/internal/ast"
	"github.com/handlebars-lang/handlebars.js/internal/js"
)

// The grammar's helper functions, ported from lib/handlebars/compiler/helpers.js.

// original is a node's JavaScript `original` property as the original compares and prints it in
// "x doesn't match y": paths and strings give text, the other literals their values, and a subexpression has
// none (undefined).
type original struct {
	kind  string // "string", "number", "boolean", "undefined" or "null"
	text  string
	num   float64
	truth bool
}

func originalOf(e ast.Expression) original {
	switch n := e.(type) {
	case *ast.PathExpression:
		return original{kind: "string", text: n.Original}
	case *ast.StringLiteral:
		return original{kind: "string", text: n.Value}
	case *ast.NumberLiteral:
		return original{kind: "number", num: n.Value}
	case *ast.BooleanLiteral:
		return original{kind: "boolean", truth: n.Value}
	case *ast.NullLiteral:
		return original{kind: "null"}
	}
	return original{kind: "undefined"}
}

// equals is JavaScript's ===.
func (o original) equals(other original) bool {
	if o.kind != other.kind {
		return false
	}
	switch o.kind {
	case "string":
		return o.text == other.text
	case "number":
		return o.num == other.num
	case "boolean":
		return o.truth == other.truth
	}
	return true
}

// String is JavaScript's ToString of the value.
func (o original) String() string {
	switch o.kind {
	case "string":
		return o.text
	case "number":
		return js.NumberToString(o.num)
	case "boolean":
		return strconv.FormatBool(o.truth)
	}
	return o.kind
}

// validateClose checks that a block's closing name matches its opening one.
func validateClose(open ast.Expression, close original) {
	opened := originalOf(open)
	if !opened.equals(close) {
		panic(exception(opened.String()+" doesn't match "+close.String(), open.Location()))
	}
}

// id unwraps a literal segment: `[a b]` names the property "a b".
func id(token string) string {
	if isBracketed(token) {
		return token[1 : len(token)-1]
	}
	return token
}

// isBracketed is /^\[.*\]$/.test(token); `.` does not match line terminators.
func isBracketed(token string) bool {
	if len(token) < 2 || token[0] != '[' || token[len(token)-1] != ']' {
		return false
	}
	return !strings.ContainsAny(token[1:len(token)-1], "\n\r  ")
}

// blockParamsID applies the grammar's yy.id to the list of block parameter names. The original passes the
// whole array, which its regular expression tests as the comma-joined names: when that looks bracketed it calls
// substring on the array and fails; otherwise the names are kept as written, brackets included.
func blockParamsID(names []string) []string {
	if isBracketed(strings.Join(names, ",")) {
		panic(&Error{Name: "TypeError", Message: "token.substring is not a function"})
	}
	return names
}

// stripFlags reads the `~` markers: the third character of the opening tag, the third from last of the closing.
func stripFlags(open, close string) *ast.StripFlags {
	return &ast.StripFlags{
		Open:  len(open) > 2 && open[2] == '~',
		Close: len(close) >= 3 && close[len(close)-3] == '~',
		Set:   true,
	}
}

// stripComment removes the comment delimiters: /^\{\{~?!-?-?/ and /-?-?~?\}\}$/.
func stripComment(comment string) string {
	s := strings.TrimPrefix(comment, "{{")
	if len(s) != len(comment) {
		t := strings.TrimPrefix(s, "~")
		if strings.HasPrefix(t, "!") {
			t = t[1:]
			for i := 0; i < 2 && strings.HasPrefix(t, "-"); i++ {
				t = t[1:]
			}
			comment = t
		}
	}
	if strings.HasSuffix(comment, "}}") {
		s := comment[:len(comment)-2]
		s = strings.TrimSuffix(s, "~")
		for i := 0; i < 2 && strings.HasSuffix(s, "-"); i++ {
			s = s[:len(s)-1]
		}
		comment = s
	}
	return comment
}

// preparePath builds a path from its segments. `this`, `.` and `..` are only allowed at the start (unless
// written as literal segments); `..` counts depth.
func preparePath(data bool, parts []segment, l yylloc) *ast.PathExpression {
	loc := locInfo(l)
	orig := ""
	if data {
		orig = "@"
	}
	dig := []string{}
	depth := 0
	for _, seg := range parts {
		part := seg.part
		isLiteral := seg.original != part
		orig += seg.separator + part
		if !isLiteral && (part == ".." || part == "." || part == "this") {
			if len(dig) > 0 {
				panic(exception("Invalid path: "+orig, loc))
			} else if part == ".." {
				depth++
			}
		} else {
			dig = append(dig, part)
		}
	}
	return &ast.PathExpression{Data: data, Depth: depth, Parts: dig, Original: orig, Loc: loc}
}

func prepareMustache(path ast.Expression, params []ast.Expression, hash *ast.Hash, open string,
	strip *ast.StripFlags, l yylloc) *ast.MustacheStatement {
	// The escape flag is the fourth character of the opening tag, or the third when there is no `~`.
	flag := byte(0)
	if len(open) > 3 {
		flag = open[3]
	} else if len(open) > 2 {
		flag = open[2]
	}
	return &ast.MustacheStatement{
		Decorator: strings.Contains(open, "*"),
		Path:      path,
		Params:    params,
		Hash:      hash,
		Escaped:   flag != '{' && flag != '&',
		Strip:     *strip,
		Loc:       locInfo(l),
	}
}

func prepareRawBlock(open *openRaw, contents []ast.Statement, close string, l yylloc) *ast.BlockStatement {
	validateClose(open.path, original{kind: "string", text: close})
	loc := locInfo(l)
	program := &ast.Program{Body: contents, Loc: loc}
	return &ast.BlockStatement{
		Path:         open.path,
		Params:       open.params,
		Hash:         open.hash,
		Program:      program,
		OpenStrip:    &ast.StripFlags{},
		InverseStrip: &ast.StripFlags{},
		CloseStrip:   &ast.StripFlags{},
		Loc:          loc,
	}
}

// prepareBlock builds a block. closer is the closing tag when there is one with a name to check; closeStrip is
// the strip flags of whatever the grammar passes as the close (for an `{{else if}}` link, the next link).
func prepareBlock(open *openBlockInfo, program *ast.Program, inverseAndProgram *inverseInfo, closer *closeInfo,
	closeStrip *ast.StripFlags, inverted bool, l yylloc) *ast.BlockStatement {
	if closer != nil && closer.path != nil {
		validateClose(open.path, originalOf(closer.path))
	}
	decorator := strings.Contains(open.open, "*")
	program.BlockParams = open.blockParams
	var inverse *ast.Program
	var inverseStrip *ast.StripFlags
	if inverseAndProgram != nil {
		if decorator {
			panic(exception("Unexpected inverse block on decorator", nil))
		}
		if inverseAndProgram.chain {
			inverseAndProgram.program.Body[0].(*ast.BlockStatement).CloseStrip = closeStrip
		}
		inverseStrip = inverseAndProgram.strip
		inverse = inverseAndProgram.program
	}
	if inverted {
		program, inverse = inverse, program
	}
	return &ast.BlockStatement{
		Decorator:    decorator,
		Path:         open.path,
		Params:       open.params,
		Hash:         open.hash,
		Program:      program,
		Inverse:      inverse,
		OpenStrip:    open.strip,
		InverseStrip: inverseStrip,
		CloseStrip:   closeStrip,
		Loc:          locInfo(l),
	}
}

// prepareProgram wraps statements in a program; without a location it spans its first to last statement.
func prepareProgram(statements []ast.Statement, loc *ast.SourceLocation) *ast.Program {
	if loc == nil && len(statements) > 0 {
		first, last := statements[0].Location(), statements[len(statements)-1].Location()
		if first != nil && last != nil {
			loc = &ast.SourceLocation{Start: first.Start, End: last.End}
		}
	}
	if statements == nil {
		statements = []ast.Statement{}
	}
	return &ast.Program{Body: statements, Loc: loc}
}

func preparePartialBlock(open *openBlockInfo, program *ast.Program, close *closeInfo,
	l yylloc) *ast.PartialBlockStatement {
	validateClose(open.path, originalOf(close.path))
	return &ast.PartialBlockStatement{
		Name:       open.path,
		Params:     open.params,
		Hash:       open.hash,
		Program:    program,
		OpenStrip:  open.strip,
		CloseStrip: close.strip,
		Loc:        locInfo(l),
	}
}

// jsNumber is Number(text) for the grammar's number tokens.
func jsNumber(text string) float64 {
	f, err := strconv.ParseFloat(text, 64)
	if err != nil && !math.IsInf(f, 0) {
		return math.NaN()
	}
	return f
}

func itoa(n int) string { return strconv.Itoa(n) }
