// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

// Package ast defines the Handlebars abstract syntax tree produced by the parser. The node types mirror the
// objects built by the original's grammar actions and compiler helpers (lib/handlebars/compiler/helpers.js),
// field for field. Source positions count UTF-16 code units, as JavaScript string indices do.
package ast

import (
	"regexp"
	"strconv"
)

// Position is a line (1-based) and a column (0-based, in UTF-16 code units).
type Position struct {
	Line   int
	Column int
}

// SourceLocation is the span a node covers in the template.
type SourceLocation struct {
	Start Position
	End   Position
}

// StripFlags records the `~` whitespace-control markers of a tag. Set is false for the empty flags object the
// original attaches to programs and raw blocks (`{}`), which has neither key.
type StripFlags struct {
	Open  bool
	Close bool
	Set   bool
}

// Node is any syntax tree node.
type Node interface {
	// Type is the original's node type name, e.g. "MustacheStatement".
	Type() string
	// Location is the node's source span; nil when the original has none (an empty program).
	Location() *SourceLocation
}

// Statement is a node that can appear in a program body.
type Statement interface {
	Node
	statementNode()
}

// Expression is a path, a subexpression or a literal: anything usable as a helper name, a parameter or a hash
// value.
type Expression interface {
	Node
	expressionNode()
}

// Program is a sequence of statements: the template itself or the body of a block.
type Program struct {
	Body []Statement
	// BlockParams are the `as |a b|` names of the block that owns this program; nil when there are none.
	BlockParams []string
	Strip       StripFlags
	// Chained marks the synthetic program that wraps an `{{else if}}` chain link.
	Chained bool
	Loc     *SourceLocation
}

// MustacheStatement is `{{expr}}`, `{{{expr}}}` or `{{&expr}}`; with Decorator set it is `{{*expr}}`.
type MustacheStatement struct {
	Decorator bool
	Path      Expression
	Params    []Expression
	Hash      *Hash
	Escaped   bool
	Strip     StripFlags
	Loc       *SourceLocation
}

// BlockStatement is `{{#expr}}...{{/expr}}`, an inverted section `{{^expr}}`, or a raw block
// `{{{{expr}}}}...{{{{/expr}}}}`; with Decorator set it is `{{#*expr}}...{{/expr}}`.
type BlockStatement struct {
	Decorator bool
	Path      Expression
	Params    []Expression
	Hash      *Hash
	// Program is nil for an inverted section without an `{{else}}`.
	Program *Program
	Inverse *Program
	// The strip flags of the opening, `{{else}}` and closing tags; nil where the original leaves them
	// undefined.
	OpenStrip    *StripFlags
	InverseStrip *StripFlags
	CloseStrip   *StripFlags
	Loc          *SourceLocation
}

// PartialStatement is `{{> name}}`.
type PartialStatement struct {
	Name   Expression
	Params []Expression
	Hash   *Hash
	// Indent is the whitespace before a standalone partial tag, applied to every line of the partial.
	Indent string
	Strip  StripFlags
	Loc    *SourceLocation
}

// PartialBlockStatement is `{{#> name}}...{{/name}}`.
type PartialBlockStatement struct {
	Name       Expression
	Params     []Expression
	Hash       *Hash
	Program    *Program
	OpenStrip  *StripFlags
	CloseStrip *StripFlags
	Loc        *SourceLocation
}

// ContentStatement is literal template text.
type ContentStatement struct {
	// Original is the text as written; Value is what remains after whitespace control.
	Original string
	Value    string
	// RightStripped and LeftStripped are set (true or false) once whitespace control has looked at the
	// node's start or end; nil when it never did.
	RightStripped *bool
	LeftStripped  *bool
	Loc           *SourceLocation
}

// CommentStatement is `{{! ... }}` or `{{!-- ... --}}`.
type CommentStatement struct {
	Value string
	Strip StripFlags
	Loc   *SourceLocation
}

// SubExpression is `(helper params... hash...)`.
type SubExpression struct {
	Path   Expression
	Params []Expression
	Hash   *Hash
	Loc    *SourceLocation
}

// PathExpression is an identifier path such as `foo.bar`, `../x`, `this.[a b]` or `@index`.
type PathExpression struct {
	// Data is true for `@` paths.
	Data bool
	// Depth counts the leading `..` segments.
	Depth int
	// Parts are the segments after `this`, `.` and `..` have been removed (literal segments unwrapped).
	Parts []string
	// Original is the path as written, with literal segments unwrapped and `@` kept.
	Original string
	Loc      *SourceLocation
	// Strict and Falsy are compile-time annotations the compiler sets on the node.
	Strict bool
	Falsy  bool
}

// StringLiteral is `"text"` or `'text'`.
type StringLiteral struct {
	Value string
	Loc   *SourceLocation
}

// NumberLiteral is a number such as `-1.5`; the original keeps it as a JavaScript number.
type NumberLiteral struct {
	Value float64
	Loc   *SourceLocation
}

// BooleanLiteral is `true` or `false`.
type BooleanLiteral struct {
	Value bool
	Loc   *SourceLocation
}

// UndefinedLiteral is `undefined`.
type UndefinedLiteral struct {
	Loc *SourceLocation
}

// NullLiteral is `null`.
type NullLiteral struct {
	Loc *SourceLocation
}

// Hash is the `key=value` arguments of a call, in source order.
type Hash struct {
	Pairs []*HashPair
	Loc   *SourceLocation
}

// HashPair is one `key=value` argument.
type HashPair struct {
	Key   string
	Value Expression
	Loc   *SourceLocation
}

// Type implementations.

func (*Program) Type() string { return "Program" }

func (m *MustacheStatement) Type() string {
	if m.Decorator {
		return "Decorator"
	}
	return "MustacheStatement"
}

func (b *BlockStatement) Type() string {
	if b.Decorator {
		return "DecoratorBlock"
	}
	return "BlockStatement"
}

func (*PartialStatement) Type() string      { return "PartialStatement" }
func (*PartialBlockStatement) Type() string { return "PartialBlockStatement" }
func (*ContentStatement) Type() string      { return "ContentStatement" }
func (*CommentStatement) Type() string      { return "CommentStatement" }
func (*SubExpression) Type() string         { return "SubExpression" }
func (*PathExpression) Type() string        { return "PathExpression" }
func (*StringLiteral) Type() string         { return "StringLiteral" }
func (*NumberLiteral) Type() string         { return "NumberLiteral" }
func (*BooleanLiteral) Type() string        { return "BooleanLiteral" }
func (*UndefinedLiteral) Type() string      { return "UndefinedLiteral" }
func (*NullLiteral) Type() string           { return "NullLiteral" }
func (*Hash) Type() string                  { return "Hash" }
func (*HashPair) Type() string              { return "HashPair" }

// Location implementations.

func (n *Program) Location() *SourceLocation               { return n.Loc }
func (n *MustacheStatement) Location() *SourceLocation     { return n.Loc }
func (n *BlockStatement) Location() *SourceLocation        { return n.Loc }
func (n *PartialStatement) Location() *SourceLocation      { return n.Loc }
func (n *PartialBlockStatement) Location() *SourceLocation { return n.Loc }
func (n *ContentStatement) Location() *SourceLocation      { return n.Loc }
func (n *CommentStatement) Location() *SourceLocation      { return n.Loc }
func (n *SubExpression) Location() *SourceLocation         { return n.Loc }
func (n *PathExpression) Location() *SourceLocation        { return n.Loc }
func (n *StringLiteral) Location() *SourceLocation         { return n.Loc }
func (n *NumberLiteral) Location() *SourceLocation         { return n.Loc }
func (n *BooleanLiteral) Location() *SourceLocation        { return n.Loc }
func (n *UndefinedLiteral) Location() *SourceLocation      { return n.Loc }
func (n *NullLiteral) Location() *SourceLocation           { return n.Loc }
func (n *Hash) Location() *SourceLocation                  { return n.Loc }
func (n *HashPair) Location() *SourceLocation              { return n.Loc }

func (*MustacheStatement) statementNode()     {}
func (*BlockStatement) statementNode()        {}
func (*PartialStatement) statementNode()      {}
func (*PartialBlockStatement) statementNode() {}
func (*ContentStatement) statementNode()      {}
func (*CommentStatement) statementNode()      {}

func (*SubExpression) expressionNode()    {}
func (*PathExpression) expressionNode()   {}
func (*StringLiteral) expressionNode()    {}
func (*NumberLiteral) expressionNode()    {}
func (*BooleanLiteral) expressionNode()   {}
func (*UndefinedLiteral) expressionNode() {}
func (*NullLiteral) expressionNode()      {}

// HelperExpression reports whether a node is definitely a helper call: a subexpression, or a mustache or block
// with at least one parameter or a hash.
func HelperExpression(node Node) bool {
	switch n := node.(type) {
	case *SubExpression:
		return true
	case *MustacheStatement:
		return !n.Decorator && (len(n.Params) > 0 || n.Hash != nil)
	case *BlockStatement:
		return !n.Decorator && (len(n.Params) > 0 || n.Hash != nil)
	}
	return false
}

// scopedPattern is the original's /^\.|this\b/: note that `this` may appear anywhere in the path, so `athis`
// and `foo/[this]` are scoped too.
var scopedPattern = regexp.MustCompile(`^\.|this\b`)

// ScopedID reports whether a path is explicitly scoped to the current context (`.`, `./x`, `this.x`).
func ScopedID(path *PathExpression) bool {
	return scopedPattern.MatchString(path.Original)
}

// SimpleID reports whether a path is a single plain segment: not scoped and without `..`.
func SimpleID(path *PathExpression) bool {
	return len(path.Parts) == 1 && !ScopedID(path) && path.Depth == 0
}

// ExceptionMessage is the message of the original's Exception(message, node): when the node has a location, its
// start is appended as " - line:column". Every compile-time and runtime error with a location is built with it.
func ExceptionMessage(message string, loc *SourceLocation) string {
	if loc == nil {
		return message
	}
	return AppendLocation(message, strconv.Itoa(loc.Start.Line), strconv.Itoa(loc.Start.Column))
}

// AppendLocation adds the " - line:column" suffix of the original's Exception to a message; line and column are
// already converted to text (helpers may pass any JavaScript values as a location).
func AppendLocation(message, line, column string) string {
	return message + " - " + line + ":" + column
}
