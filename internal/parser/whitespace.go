// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package parser

import (
	"strings"
	"unicode/utf8"

	"github.com/handlebars-lang/handlebars.js/internal/ast"
)

// Whitespace control, ported from lib/handlebars/compiler/whitespace-control.js: `~` markers strip all
// whitespace next to a tag, and block tags, partials and comments alone on a line ("standalone") take their line
// with them. Whitespace here is JavaScript's \s.

type stripInfo struct {
	open, close                                       bool
	openStandalone, closeStandalone, inlineStandalone bool
}

type whitespaceControl struct {
	ignoreStandalone bool
	rootSeen         bool
}

func applyWhitespaceControl(program *ast.Program, opts Options) (err error) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*Error)
			if !ok {
				panic(r)
			}
			err = e
		}
	}()
	w := &whitespaceControl{ignoreStandalone: opts.IgnoreStandalone}
	w.program(program)
	return nil
}

func (w *whitespaceControl) program(program *ast.Program) {
	doStandalone := !w.ignoreStandalone
	isRoot := !w.rootSeen
	w.rootSeen = true
	body := program.Body
	for i, current := range body {
		strip := w.accept(current)
		if strip == nil {
			continue
		}
		prevWhitespace := isPrevWhitespace(body, i, isRoot)
		nextWhitespace := isNextWhitespace(body, i, isRoot)
		openStandalone := strip.openStandalone && prevWhitespace
		closeStandalone := strip.closeStandalone && nextWhitespace
		inlineStandalone := strip.inlineStandalone && prevWhitespace && nextWhitespace

		if strip.close {
			omitRight(body, i, true)
		}
		if strip.open {
			omitLeft(body, i, true)
		}
		if doStandalone && inlineStandalone {
			omitRight(body, i, false)
			if omitLeft(body, i, false) {
				// A standalone partial keeps its line's indentation, to indent every line it renders.
				if partial, ok := current.(*ast.PartialStatement); ok {
					partial.Indent = trailingIndent(body[i-1].(*ast.ContentStatement).Original)
				}
			}
		}
		if doStandalone && openStandalone {
			omitRight(firstProgram(current, false).Body, -1, false)
			// Strip out the previous content node if it's whitespace only.
			omitLeft(body, i, false)
		}
		if doStandalone && closeStandalone {
			// Always strip the next node.
			omitRight(body, i, false)
			inner := firstProgram(current, true).Body
			omitLeft(inner, len(inner), false)
		}
	}
}

// firstProgram is `current.program || current.inverse`, or with inverseFirst `current.inverse ||
// current.program`.
func firstProgram(node ast.Statement, inverseFirst bool) *ast.Program {
	var program, inverse *ast.Program
	switch n := node.(type) {
	case *ast.BlockStatement:
		program, inverse = n.Program, n.Inverse
	case *ast.PartialBlockStatement:
		program = n.Program
	}
	if inverseFirst {
		program, inverse = inverse, program
	}
	if program != nil {
		return program
	}
	return inverse
}

func (w *whitespaceControl) accept(node ast.Statement) *stripInfo {
	switch n := node.(type) {
	case *ast.MustacheStatement:
		return &stripInfo{open: n.Strip.Open, close: n.Strip.Close}
	case *ast.PartialStatement:
		return &stripInfo{inlineStandalone: true, open: n.Strip.Open, close: n.Strip.Close}
	case *ast.CommentStatement:
		return &stripInfo{inlineStandalone: true, open: n.Strip.Open, close: n.Strip.Close}
	case *ast.BlockStatement:
		return w.block(n.Program, n.Inverse, n.OpenStrip, n.InverseStrip, n.CloseStrip)
	case *ast.PartialBlockStatement:
		return w.block(n.Program, nil, n.OpenStrip, nil, n.CloseStrip)
	}
	return nil
}

func flags(s *ast.StripFlags) ast.StripFlags {
	if s == nil {
		return ast.StripFlags{}
	}
	return *s
}

func (w *whitespaceControl) block(blockProgram, blockInverse *ast.Program, openStrip, inverseStrip,
	closeStrip *ast.StripFlags) *stripInfo {
	if blockProgram != nil {
		w.program(blockProgram)
	}
	if blockInverse != nil {
		w.program(blockInverse)
	}
	// Find the inverse program that is involved with whitespace stripping.
	program := blockProgram
	if program == nil {
		program = blockInverse
	}
	var inverse *ast.Program
	if blockProgram != nil {
		inverse = blockInverse
	}
	firstInverse, lastInverse := inverse, inverse
	if inverse != nil && inverse.Chained {
		firstInverse = inverse.Body[0].(*ast.BlockStatement).Program
		// Walk the inverse chain to find the last inverse that is actually in the chain.
		for lastInverse.Chained {
			lastInverse = lastInverse.Body[len(lastInverse.Body)-1].(*ast.BlockStatement).Program
		}
	}
	standaloneBody := program
	if firstInverse != nil {
		standaloneBody = firstInverse
	}
	strip := &stripInfo{
		open:  flags(openStrip).Open,
		close: flags(closeStrip).Close,
		// Our parent decides whether we really are standalone.
		openStandalone:  isNextWhitespace(program.Body, -1, false),
		closeStandalone: isPrevWhitespace(standaloneBody.Body, len(standaloneBody.Body), false),
	}
	if flags(openStrip).Close {
		omitRight(program.Body, -1, true)
	}
	if inverse != nil {
		if flags(inverseStrip).Open {
			omitLeft(program.Body, len(program.Body), true)
		}
		if flags(inverseStrip).Close {
			omitRight(firstInverse.Body, -1, true)
		}
		if flags(closeStrip).Open {
			omitLeft(lastInverse.Body, len(lastInverse.Body), true)
		}
		// Find standalone else statements.
		if !w.ignoreStandalone && isPrevWhitespace(program.Body, len(program.Body), false) &&
			isNextWhitespace(firstInverse.Body, -1, false) {
			omitLeft(program.Body, len(program.Body), false)
			omitRight(firstInverse.Body, -1, false)
		}
	} else if flags(closeStrip).Open {
		omitLeft(program.Body, len(program.Body), true)
	}
	return strip
}

// isPrevWhitespace reports whether the node before index i is content whose trailing whitespace contains a
// newline (for the root's first node, whitespace only is enough); with nothing before it, isRoot.
func isPrevWhitespace(body []ast.Statement, i int, isRoot bool) bool {
	if i-1 < 0 || i-1 >= len(body) {
		return isRoot
	}
	prev, ok := body[i-1].(*ast.ContentStatement)
	if !ok {
		return false
	}
	hasSibling := i-2 >= 0
	run := prev.Original[len(trimRightSpace(prev.Original)):]
	if strings.Contains(run, "\n") {
		return true
	}
	return !hasSibling && isRoot && run == prev.Original
}

// isNextWhitespace reports whether the node after index i is content whose leading whitespace contains a
// newline (for the root's last node, whitespace only is enough); with nothing after it, isRoot.
func isNextWhitespace(body []ast.Statement, i int, isRoot bool) bool {
	if i+1 >= len(body) {
		return isRoot
	}
	next, ok := body[i+1].(*ast.ContentStatement)
	if !ok {
		return false
	}
	hasSibling := i+2 < len(body)
	run := next.Original[:len(next.Original)-len(trimLeftSpace(next.Original))]
	if strings.Contains(run, "\n") {
		return true
	}
	return !hasSibling && isRoot && run == next.Original
}

// omitRight strips the start of the content node after index i (-1: the first node): with multiple all
// whitespace, otherwise spaces and tabs up to and including one line break.
func omitRight(body []ast.Statement, i int, multiple bool) {
	if i+1 < 0 || i+1 >= len(body) {
		return
	}
	current, ok := body[i+1].(*ast.ContentStatement)
	if !ok || (!multiple && current.RightStripped != nil && *current.RightStripped) {
		return
	}
	before := current.Value
	if multiple {
		current.Value = trimLeftSpace(current.Value)
	} else {
		// /^[ \t]*\r?\n?/
		v := strings.TrimLeft(current.Value, " \t")
		v = strings.TrimPrefix(v, "\r")
		current.Value = strings.TrimPrefix(v, "\n")
	}
	changed := current.Value != before
	current.RightStripped = &changed
}

// omitLeft strips the end of the content node before index i (len(body): the last node): with multiple all
// trailing whitespace, otherwise trailing spaces and tabs. It reports whether anything was removed.
func omitLeft(body []ast.Statement, i int, multiple bool) bool {
	if i-1 < 0 || i-1 >= len(body) {
		return false
	}
	current, ok := body[i-1].(*ast.ContentStatement)
	if !ok || (!multiple && current.LeftStripped != nil && *current.LeftStripped) {
		return false
	}
	before := current.Value
	if multiple {
		current.Value = trimRightSpace(current.Value)
	} else {
		current.Value = strings.TrimRight(current.Value, " \t")
	}
	changed := current.Value != before
	current.LeftStripped = &changed
	return changed
}

// trailingIndent is the run of spaces and tabs ending the text.
func trailingIndent(text string) string {
	return text[len(strings.TrimRight(text, " \t")):]
}

func isJSSpace(r rune) bool {
	return r < 0x10000 && isSpace(uint16(r))
}

func trimLeftSpace(s string) string {
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		if !isJSSpace(r) {
			break
		}
		s = s[n:]
	}
	return s
}

func trimRightSpace(s string) string {
	for len(s) > 0 {
		r, n := utf8.DecodeLastRuneInString(s)
		if !isJSSpace(r) {
			break
		}
		s = s[:len(s)-n]
	}
	return s
}
