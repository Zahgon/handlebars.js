// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package parser

import (
	"strings"
	"unicode/utf16"
)

// The lexer is a port of the jison lexer generated from src/handlebars.l. It works on UTF-16 code units, as
// JavaScript strings do, because token lengths, columns and the context shown in error messages are all counted
// in code units. Its bookkeeping (match, matched, yytext, the location update rules and unput) follows the
// generated lexer exactly, quirks included, since error messages expose it.

// Lexer start conditions.
const (
	condInitial = "INITIAL"
	condMu      = "mu"
	condEmu     = "emu"
	condCom     = "com"
	condRaw     = "raw"
)

// Token numbers returned by the lexer (jison symbol numbers; see tables.go).
const (
	tokEnd              = 1 // the lexer's own end of input, which the grammar does not name
	tokEOF              = 5
	tokComment          = 14
	tokContent          = 15
	tokEndRawBlock      = 18
	tokOpenRawBlock     = 19
	tokCloseRawBlock    = 23
	tokOpenBlock        = 29
	tokClose            = 33
	tokOpenInverse      = 34
	tokOpenInverseChain = 39
	tokInverse          = 44
	tokOpenEndBlock     = 47
	tokOpen             = 48
	tokOpenUnescaped    = 51
	tokCloseUnescaped   = 54
	tokOpenPartial      = 55
	tokOpenPartialBlock = 60
	tokOpenSexpr        = 65
	tokCloseSexpr       = 68
	tokID               = 72
	tokEquals           = 73
	tokOpenBlockParams  = 75
	tokCloseBlockParams = 77
	tokString           = 80
	tokNumber           = 81
	tokBoolean          = 82
	tokUndefined        = 83
	tokNull             = 84
	tokData             = 85
	tokSep              = 87
	// tokInvalid is the 'INVALID' token of the catch-all rule; the grammar has no symbol for it.
	tokInvalid = -1
	// tokNone means a rule matched without producing a token (whitespace, an opening comment tag).
	tokNone = 0
)

// conditionRules lists, per start condition, the rules tried in order (jison's lexer.conditions).
var conditionRules = map[string][]int{
	condMu:      {7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44},
	condEmu:     {2},
	condCom:     {6},
	condRaw:     {3, 4, 5},
	condInitial: {0, 1, 44},
}

// yylloc is jison's location record: lines are 1-based, columns 0-based in code units.
type yylloc struct {
	firstLine, lastLine, firstColumn, lastColumn int
}

type lexer struct {
	// input is the text not yet consumed; it is always a suffix of src, so unput can step back cheaply.
	src        []uint16
	input      []uint16
	yytext     []uint16
	match      []uint16
	matched    []uint16
	yyleng     int
	yylineno   int
	loc        yylloc
	conditions []string
	done       bool
	more       bool
}

func newLexer(input []uint16) *lexer {
	return &lexer{
		src:        input,
		input:      input,
		conditions: []string{condInitial},
		loc:        yylloc{firstLine: 1, lastLine: 1},
	}
}

func (l *lexer) begin(condition string) { l.conditions = append(l.conditions, condition) }

func (l *lexer) popState() {
	if len(l.conditions) > 0 {
		l.conditions = l.conditions[:len(l.conditions)-1]
	}
}

func (l *lexer) top() string {
	if len(l.conditions) == 0 {
		return ""
	}
	return l.conditions[len(l.conditions)-1]
}

// lex returns the next token, skipping rules that produce none.
func (l *lexer) lex() (int, error) {
	for {
		tok, err := l.next()
		if err != nil || tok != tokNone {
			return tok, err
		}
	}
}

func (l *lexer) next() (int, error) {
	if l.done {
		return tokEnd, nil
	}
	if len(l.input) == 0 {
		l.done = true
	}
	if !l.more {
		l.yytext = nil
		l.match = nil
	}
	rules, ok := conditionRules[l.top()]
	if !ok {
		// The generated lexer dereferences the missing condition.
		return 0, &Error{Name: "TypeError", Message: "Cannot read properties of undefined (reading 'rules')"}
	}
	rule, n := -1, 0
	for _, r := range rules {
		if m := matchRule(r, l.input); m >= 0 {
			rule, n = r, m
			break
		}
	}
	if rule < 0 {
		if len(l.input) == 0 {
			return tokEnd, nil
		}
		return 0, &Error{Name: "Error", Message: "Lexical error on line " + itoa(l.yylineno+1) +
			". Unrecognized text.\n" + l.showPosition()}
	}
	text := l.input[:n]
	count, lastLen := lineBreaks(text)
	l.yylineno += count
	next := yylloc{firstLine: l.loc.lastLine, lastLine: l.yylineno + 1, firstColumn: l.loc.lastColumn}
	if count > 0 {
		next.lastColumn = lastLen
	} else {
		next.lastColumn = l.loc.lastColumn + n
	}
	l.loc = next
	l.yytext = concat(l.yytext, text)
	l.match = concat(l.match, text)
	l.yyleng = len(l.yytext)
	l.more = false
	l.input = l.input[n:]
	l.matched = append(l.matched, text...)
	tok := l.action(rule)
	if l.done && len(l.input) > 0 {
		l.done = false
	}
	return tok, nil
}

// action runs a rule's action and returns its token.
func (l *lexer) action(rule int) int {
	switch rule {
	case 0:
		// Content before a mustache. A backslash escapes the mustache (`\{{`), a doubled one escapes the
		// backslash (`\\{{`); either way the last backslash is dropped from the content.
		switch {
		case hasSuffix(l.yytext, `\\`):
			l.strip(0, 1)
			l.begin(condMu)
		case hasSuffix(l.yytext, `\`):
			l.strip(0, 1)
			l.begin(condEmu)
		default:
			l.begin(condMu)
		}
		if len(l.yytext) > 0 {
			return tokContent
		}
		return tokNone
	case 1:
		return tokContent
	case 2:
		l.popState()
		return tokContent
	case 3:
		l.begin(condRaw)
		return tokContent
	case 4:
		l.popState()
		// The original checks the new top of the stack: a nested raw block's end is content.
		if l.top() == condRaw {
			return tokContent
		}
		l.strip(5, 9)
		return tokEndRawBlock
	case 5:
		return tokContent
	case 6:
		l.popState()
		return tokComment
	case 7:
		return tokOpenSexpr
	case 8:
		return tokCloseSexpr
	case 9:
		return tokOpenRawBlock
	case 10:
		l.popState()
		l.begin(condRaw)
		return tokCloseRawBlock
	case 11:
		return tokOpenPartial
	case 12:
		return tokOpenPartialBlock
	case 13:
		return tokOpenBlock
	case 14:
		return tokOpenEndBlock
	case 15, 16:
		l.popState()
		return tokInverse
	case 17:
		return tokOpenInverse
	case 18:
		return tokOpenInverseChain
	case 19:
		return tokOpenUnescaped
	case 20:
		return tokOpen
	case 21:
		// A long comment: put the opening tag back and lex the whole comment in the com condition.
		l.unput(l.yytext)
		l.popState()
		l.begin(condCom)
		return tokNone
	case 22:
		l.popState()
		return tokComment
	case 23:
		return tokOpen
	case 24:
		return tokEquals
	case 25, 26:
		return tokID
	case 27:
		return tokSep
	case 28:
		return tokNone
	case 29:
		l.popState()
		return tokCloseUnescaped
	case 30:
		l.popState()
		return tokClose
	case 31:
		l.strip(1, 2)
		l.yytext = replaceEscapes(l.yytext, func(c uint16) bool { return c == '"' })
		return tokString
	case 32:
		l.strip(1, 2)
		l.yytext = replaceEscapes(l.yytext, func(c uint16) bool { return c == '\'' })
		return tokString
	case 33:
		return tokData
	case 34, 35:
		return tokBoolean
	case 36:
		return tokUndefined
	case 37:
		return tokNull
	case 38:
		return tokNumber
	case 39:
		return tokOpenBlockParams
	case 40:
		return tokCloseBlockParams
	case 41:
		return tokID
	case 42:
		l.yytext = replaceEscapes(l.yytext, func(c uint16) bool { return c == '\\' || c == ']' })
		return tokID
	case 43:
		return tokInvalid
	case 44:
		return tokEOF
	}
	return tokNone
}

// strip is the grammar's strip(start, end): yytext.substring(start, yyleng - end + start).
func (l *lexer) strip(start, end int) {
	l.yytext = substring(l.yytext, start, l.yyleng-end+start)
}

// unput pushes text back onto the input, with the generated lexer's bookkeeping: it shortens match and matched
// by one code unit only, which later shows in the context of error messages.
func (l *lexer) unput(ch []uint16) {
	ch = concat(nil, ch)
	n := len(ch)
	lines := splitLines(ch)
	if consumed := len(l.src) - len(l.input); consumed >= n && equalUnits(l.src[consumed-n:consumed], ch) {
		l.input = l.src[consumed-n:]
	} else {
		l.src = concat(ch, l.input)
		l.input = l.src
	}
	l.yytext = substr(l.yytext, 0, len(l.yytext)-n-1)
	oldLines := splitLines(l.match)
	l.match = substr(l.match, 0, len(l.match)-1)
	l.matched = substr(l.matched, 0, len(l.matched)-1)
	if len(lines)-1 > 0 {
		l.yylineno -= len(lines) - 1
	}
	base := 0
	if len(lines) == len(oldLines) {
		base = l.loc.firstColumn
	}
	l.loc = yylloc{
		firstLine:   l.loc.firstLine,
		lastLine:    l.yylineno + 1,
		firstColumn: l.loc.firstColumn,
		lastColumn:  base + len(oldLines[len(oldLines)-len(lines)]) - len(lines[0]),
	}
}

// pastInput is the up to 20 code units consumed before the current match, newlines removed.
func (l *lexer) pastInput() string {
	past := substr(l.matched, 0, len(l.matched)-len(l.match))
	prefix := ""
	if len(past) > 20 {
		prefix = "..."
		past = past[len(past)-20:]
	}
	return prefix + removeNewlines(past)
}

// upcomingInput is the current match and following input, cut to 20 code units, newlines removed.
func (l *lexer) upcomingInput() string {
	next := concat(nil, l.match)
	if len(next) < 20 {
		rest := l.input
		if len(rest) > 20-len(next) {
			rest = rest[:20-len(next)]
		}
		next = concat(next, rest)
	}
	suffix := ""
	if len(next) > 20 {
		suffix = "..."
	}
	if len(next) > 20 {
		next = next[:20]
	}
	return removeNewlines(next) + suffix
}

func (l *lexer) showPosition() string {
	pre := l.pastInput()
	return pre + l.upcomingInput() + "\n" + strings.Repeat("-", len(utf16.Encode([]rune(pre)))) + "^"
}

// Code unit helpers.

func concat(a, b []uint16) []uint16 {
	out := make([]uint16, 0, len(a)+len(b))
	return append(append(out, a...), b...)
}

func equalUnits(a, b []uint16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasSuffix(s []uint16, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	for i := 0; i < len(suffix); i++ {
		if s[len(s)-len(suffix)+i] != uint16(suffix[i]) {
			return false
		}
	}
	return true
}

// substring is String.prototype.substring: arguments are clamped and swapped when reversed.
func substring(s []uint16, start, end int) []uint16 {
	start = clamp(start, 0, len(s))
	end = clamp(end, 0, len(s))
	if start > end {
		start, end = end, start
	}
	return s[start:end]
}

// substr is String.prototype.substr with a non-negative start: a negative length gives the empty string.
func substr(s []uint16, start, length int) []uint16 {
	start = clamp(start, 0, len(s))
	end := clamp(start+length, start, len(s))
	if length <= 0 {
		return nil
	}
	return s[start:end]
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// replaceEscapes drops a backslash before any code unit accepted by escaped, left to right.
func replaceEscapes(s []uint16, escaped func(uint16) bool) []uint16 {
	out := make([]uint16, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && escaped(s[i+1]) {
			out = append(out, s[i+1])
			i++
			continue
		}
		out = append(out, s[i])
	}
	return out
}

func removeNewlines(s []uint16) string {
	out := make([]uint16, 0, len(s))
	for _, c := range s {
		if c != '\n' {
			out = append(out, c)
		}
	}
	return decode(out)
}

// lineBreaks applies the generated lexer's /(?:\r\n?|\n).*/g to a match: it returns the number of line breaks
// and the length of the last line after its break, where `.` stops at \n, \r, U+2028 and U+2029.
func lineBreaks(s []uint16) (count, lastLen int) {
	for i := 0; i < len(s); {
		if s[i] != '\r' && s[i] != '\n' {
			i++
			continue
		}
		if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
			i += 2
		} else {
			i++
		}
		start := i
		for i < len(s) && !isLineTerminator(s[i]) {
			i++
		}
		count++
		lastLen = i - start
	}
	return count, lastLen
}

// splitLines is s.split(/(?:\r\n?|\n)/g).
func splitLines(s []uint16) [][]uint16 {
	var parts [][]uint16
	start := 0
	for i := 0; i < len(s); {
		switch {
		case s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n':
			parts = append(parts, s[start:i])
			i += 2
			start = i
		case s[i] == '\r' || s[i] == '\n':
			parts = append(parts, s[start:i])
			i++
			start = i
		default:
			i++
		}
	}
	return append(parts, s[start:])
}

func isLineTerminator(c uint16) bool {
	return c == '\n' || c == '\r' || c == 0x2028 || c == 0x2029
}

// isSpace is JavaScript's \s.
func isSpace(c uint16) bool {
	switch c {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return c >= 0x2000 && c <= 0x200a
}

// isIDChar is the grammar's ID class [^\s!"#%-,\.\/;->@\[-\^`\{-~].
func isIDChar(c uint16) bool {
	if isSpace(c) {
		return false
	}
	switch {
	case c == '!' || c == '"' || c == '#':
		return false
	case c >= '%' && c <= ',':
		return false
	case c == '.' || c == '/':
		return false
	case c >= ';' && c <= '>':
		return false
	case c == '@':
		return false
	case c >= '[' && c <= '^':
		return false
	case c == '`':
		return false
	case c >= '{' && c <= '~':
		return false
	}
	return true
}

func encode(s string) []uint16 { return utf16.Encode([]rune(s)) }

// decode turns code units back into a Go string; a lone surrogate becomes U+FFFD, as it does when Node writes
// the string out.
func decode(s []uint16) string { return string(utf16.Decode(s)) }
