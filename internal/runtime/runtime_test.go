// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package runtime

import (
	"bytes"
	"testing"

	"github.com/handlebars-lang/handlebars.js/internal/compiler"
	"github.com/handlebars-lang/handlebars.js/internal/js"
)

func TestEscapeExpression(t *testing.T) {
	for _, tc := range []struct {
		in   js.Value
		want string
	}{{`&<>"'` + "`=", "&amp;&lt;&gt;&quot;&#x27;&#x60;&#x3D;"}, {js.Null, ""}, {false, "false"}, {float64(0), "0"},
		{&js.SafeString{S: "<b>"}, "<b>"}, {js.NewArray(float64(1), js.Null, "<"), "1,,&lt;"}} {
		if got, err := escapeExpression(tc.in); err != nil || got != tc.want {
			t.Errorf("escapeExpression(%v) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}

func render(t *testing.T, source string, context js.Value, opts *compiler.Options) string {
	t.Helper()
	var out bytes.Buffer
	env := NewEnv(&out, &out)
	v, err := env.Compile(source, opts).Call(js.Undefined, []js.Value{context, js.NewObject()})
	if err != nil {
		return "error: " + err.Error()
	}
	return out.String() + v.(string)
}

func TestRender(t *testing.T) {
	ctx, _ := js.ParseJSON(`{"a":[1,"1",2],"one":1,"t":true,"m":{"2":"b","1":"a"}}`)
	for _, tc := range []struct {
		source string
		opts   compiler.Options
		want   string
	}{
		{"{{#each m}}{{@key}}={{this}}{{#unless @last}},{{/unless}}{{/each}}", compiler.Options{Data: true}, "1=a,2=b"},
		{"{{{one}}}{{{t}}}", compiler.Options{Data: true}, "2"},
		{"{{#each a}}{{#each ../a}}{{../this}}{{/each}};{{/each}}", compiler.Options{Data: true},
			"[object Object][object Object]1;[object Object][object Object]1;22[object Object];"},
		{"{{x.y}}", compiler.Options{Data: true, Strict: true}, `error: Error: "y" not defined in undefined - 1:2`},
		{"{{foo 1}}", compiler.Options{Data: true}, `error: Error: Missing helper: "foo"`},
	} {
		opts := tc.opts
		if got := render(t, tc.source, ctx, &opts); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.source, got, tc.want)
		}
	}
}
