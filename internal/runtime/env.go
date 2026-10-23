// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

// Package runtime renders compiled Handlebars templates (lib/handlebars/runtime.js, base.js, helpers/,
// decorators/, logger.js and the semantics of the JavaScript that compiler/javascript-compiler.js generates).
package runtime

import (
	"io"
	"strconv"

	"github.com/handlebars-lang/handlebars.js/internal/compiler"
	"github.com/handlebars-lang/handlebars.js/internal/js"
)

// HelperFunc is the Go form of a JavaScript helper or decorator: it receives `this` and the arguments, the
// options object last.
type HelperFunc func(this js.Value, args []js.Value) (js.Value, error)

// Env is a Handlebars environment (Handlebars.create()): registered helpers, partials and decorators, and the
// logger.
type Env struct {
	Helpers    *js.Object
	Partials   *js.Object
	Decorators *js.Object
	Logger     *Logger
	// loggedProperties remembers the prototype properties whose denied access has been logged.
	loggedProperties map[string]bool
}

// NewEnv returns an environment with the built-in helpers and decorators; log output goes to stdout (debug and
// info) and stderr (warn and error), as Node's console methods write.
func NewEnv(stdout, stderr io.Writer) *Env {
	e := &Env{
		Helpers:          js.NewObject(),
		Partials:         js.NewObject(),
		Decorators:       js.NewObject(),
		Logger:           &Logger{Level: "info", Stdout: stdout, Stderr: stderr},
		loggedProperties: map[string]bool{},
	}
	registerDefaultHelpers(e)
	e.RegisterDecorator("inline", inlineDecorator)
	return e
}

// RegisterHelper adds a helper.
func (e *Env) RegisterHelper(name string, fn HelperFunc) {
	e.Helpers.Set(name, &js.Func{Name: name, Call: fn})
}

// RegisterPartial adds a partial given as template text.
func (e *Env) RegisterPartial(name, template string) {
	e.Partials.Set(name, template)
}

// RegisterDecorator adds a decorator.
func (e *Env) RegisterDecorator(name string, fn HelperFunc) {
	e.Decorators.Set(name, &js.Func{Name: name, Call: fn})
}

// Compile returns the template function of source, compiled lazily on its first call as Handlebars.compile does.
// The function takes (context, options) and returns the rendered string.
func (e *Env) Compile(source string, opts *compiler.Options) *js.Func {
	t := &template{env: e, source: source, opts: opts.Clone()}
	return &js.Func{Name: "ret", Call: t.call}
}

// Logger is Handlebars.logger.
type Logger struct {
	// Level is logger.level: a level name or number.
	Level  js.Value
	Stdout io.Writer
	Stderr io.Writer
}

var methodMap = []string{"debug", "info", "warn", "error"}

// lookupLevel maps a level name to its index; other strings go through parseInt.
func lookupLevel(level js.Value) js.Value {
	if s, ok := level.(string); ok {
		lower := js.ToLowerCase(s)
		for i, m := range methodMap {
			if m == lower {
				return float64(i)
			}
		}
		return parseInt(s)
	}
	return level
}

// parseInt is parseInt(s, 10).
func parseInt(s string) float64 {
	s = js.TrimJS(s)
	end := 0
	if end < len(s) && (s[end] == '+' || s[end] == '-') {
		end++
	}
	start := end
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == start {
		return js.StringToNumber("x")
	}
	f, _ := strconv.ParseFloat(s[:end], 64)
	return f
}

// Log is logger.log(level, ...message).
func (l *Logger) Log(level js.Value, message ...js.Value) error {
	level = lookupLevel(level)
	threshold := lookupLevel(l.Level)
	allowed, err := lessOrEqual(threshold, level)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	key, err := js.ToString(level)
	if err != nil {
		return err
	}
	method := "log"
	if i, ok := js.ArrayIndex(key); ok && int(i) < len(methodMap) {
		method = methodMap[i]
	}
	line := js.UTF8(js.Format(message) + "\n")
	var w io.Writer = l.Stdout
	if method == "warn" || method == "error" {
		w = l.Stderr
	}
	_, err = w.Write(line)
	return err
}

// lessOrEqual is a <= b.
func lessOrEqual(a, b js.Value) (bool, error) {
	r, err := js.LessThan(b, a)
	if err != nil {
		return false, err
	}
	return r == js.False, nil
}
