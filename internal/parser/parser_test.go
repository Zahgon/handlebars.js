// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package parser

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/handlebars-lang/handlebars.js/internal/ast"
	"github.com/handlebars-lang/handlebars.js/internal/js"
)

// testdata/cases.golden holds templates with the original's result, recorded with Handlebars 4.7.10 under Node:
// either the JSON of Handlebars.parse (parseWithoutProcessing when raw is set) or "<error name>: <message>".
type recordedCase struct {
	Template         string         `json:"t"`
	IgnoreStandalone bool           `json:"is"`
	Raw              bool           `json:"raw"`
	Want             map[string]any `json:"want"`
}

func TestRecordedCases(t *testing.T) {
	data, err := os.ReadFile("testdata/cases.golden")
	if err != nil {
		t.Fatal(err)
	}
	var cases []recordedCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	failures := 0
	for i, c := range cases {
		var program *ast.Program
		if c.Raw {
			program, err = ParseWithoutProcessing(c.Template)
		} else {
			program, err = Parse(c.Template, Options{IgnoreStandalone: c.IgnoreStandalone})
		}
		got := map[string]any{}
		var perr *Error
		switch {
		case errors.As(err, &perr):
			got["err"] = perr.Name + ": " + perr.Message
		case err != nil:
			t.Fatalf("case %d: unexpected error type %T", i, err)
		default:
			got["ok"] = prog(program)
		}
		// Round-trip through JSON so both sides use the same value types.
		encoded, _ := json.Marshal(got)
		var normalized map[string]any
		if err := json.Unmarshal(encoded, &normalized); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(normalized, c.Want) {
			failures++
			if failures <= 3 {
				t.Errorf("case %d %q:\n got %s\nwant %s", i, c.Template, encoded, mustJSON(c.Want))
			}
		}
	}
	if failures > 0 {
		t.Errorf("%d of %d recorded cases differ", failures, len(cases))
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// The JSON shape of the original's syntax tree, for comparing with the recordings.

type obj = map[string]any

func loc(l *ast.SourceLocation) any {
	if l == nil {
		return nil
	}
	return obj{"start": obj{"line": l.Start.Line, "column": l.Start.Column}, "end": obj{"line": l.End.Line, "column": l.End.Column}}
}

func strip(s ast.StripFlags) any {
	if !s.Set {
		return obj{}
	}
	return obj{"open": s.Open, "close": s.Close}
}

func stripP(s *ast.StripFlags) any {
	if s == nil {
		return nil
	}
	return strip(*s)
}

type num float64

func (n num) MarshalJSON() ([]byte, error) {
	s := js.NumberToString(float64(n))
	if s == "NaN" || strings.Contains(s, "Infinity") {
		return []byte("null"), nil
	}
	return []byte(s), nil
}

// set adds a key unless the original leaves it undefined.
func set(o obj, k string, v any) {
	switch x := v.(type) {
	case nil:
		return
	case obj:
		if x == nil {
			return
		}
	case *ast.SourceLocation:
		if x == nil {
			return
		}
	}
	o[k] = v
}

func exprs(es []ast.Expression) []any {
	out := []any{}
	for _, e := range es {
		out = append(out, node(e))
	}
	return out
}

func prog(p *ast.Program) any {
	if p == nil {
		return nil
	}
	body := []any{}
	for _, s := range p.Body {
		body = append(body, node(s))
	}
	o := obj{"type": "Program", "body": body, "strip": strip(p.Strip)}
	set(o, "loc", loc(p.Loc))
	if p.BlockParams != nil {
		o["blockParams"] = p.BlockParams
	}
	if p.Chained {
		o["chained"] = true
	}
	return o
}

func hash(h *ast.Hash) any {
	if h == nil {
		return nil
	}
	pairs := []any{}
	for _, p := range h.Pairs {
		pairs = append(pairs, obj{"type": "HashPair", "key": p.Key, "value": node(p.Value), "loc": loc(p.Loc)})
	}
	return obj{"type": "Hash", "pairs": pairs, "loc": loc(h.Loc)}
}

func node(n ast.Node) any {
	switch x := n.(type) {
	case *ast.Program:
		return prog(x)
	case *ast.MustacheStatement:
		o := obj{"type": x.Type(), "path": node(x.Path), "params": exprs(x.Params), "escaped": x.Escaped, "strip": strip(x.Strip), "loc": loc(x.Loc)}
		set(o, "hash", hash(x.Hash))
		return o
	case *ast.BlockStatement:
		o := obj{"type": x.Type(), "path": node(x.Path), "params": exprs(x.Params), "loc": loc(x.Loc)}
		set(o, "hash", hash(x.Hash))
		set(o, "program", prog(x.Program))
		set(o, "inverse", prog(x.Inverse))
		set(o, "openStrip", stripP(x.OpenStrip))
		set(o, "inverseStrip", stripP(x.InverseStrip))
		set(o, "closeStrip", stripP(x.CloseStrip))
		return o
	case *ast.PartialStatement:
		o := obj{"type": x.Type(), "name": node(x.Name), "params": exprs(x.Params), "indent": x.Indent, "strip": strip(x.Strip), "loc": loc(x.Loc)}
		set(o, "hash", hash(x.Hash))
		return o
	case *ast.PartialBlockStatement:
		o := obj{"type": x.Type(), "name": node(x.Name), "params": exprs(x.Params), "program": prog(x.Program), "loc": loc(x.Loc)}
		set(o, "hash", hash(x.Hash))
		set(o, "openStrip", stripP(x.OpenStrip))
		set(o, "closeStrip", stripP(x.CloseStrip))
		return o
	case *ast.ContentStatement:
		o := obj{"type": x.Type(), "original": x.Original, "value": x.Value, "loc": loc(x.Loc)}
		if x.RightStripped != nil {
			o["rightStripped"] = *x.RightStripped
		}
		if x.LeftStripped != nil {
			o["leftStripped"] = *x.LeftStripped
		}
		return o
	case *ast.CommentStatement:
		return obj{"type": x.Type(), "value": x.Value, "strip": strip(x.Strip), "loc": loc(x.Loc)}
	case *ast.SubExpression:
		o := obj{"type": x.Type(), "path": node(x.Path), "params": exprs(x.Params), "loc": loc(x.Loc)}
		set(o, "hash", hash(x.Hash))
		return o
	case *ast.PathExpression:
		return obj{"type": x.Type(), "data": x.Data, "depth": x.Depth, "parts": x.Parts, "original": x.Original, "loc": loc(x.Loc)}
	case *ast.StringLiteral:
		return obj{"type": x.Type(), "value": x.Value, "original": x.Value, "loc": loc(x.Loc)}
	case *ast.NumberLiteral:
		return obj{"type": x.Type(), "value": num(x.Value), "original": num(x.Value), "loc": loc(x.Loc)}
	case *ast.BooleanLiteral:
		return obj{"type": x.Type(), "value": x.Value, "original": x.Value, "loc": loc(x.Loc)}
	case *ast.UndefinedLiteral:
		return obj{"type": x.Type(), "loc": loc(x.Loc)}
	case *ast.NullLiteral:
		return obj{"type": x.Type(), "value": nil, "original": nil, "loc": loc(x.Loc)}
	}
	panic(fmt.Sprintf("unknown node %T", n))
}
