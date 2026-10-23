// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

// Package cli is the handlebars command: it renders one template with registered partials, the command's
// helpers and decorators and JSON data.
//
//	handlebars [options] TEMPLATE [DATA]
//
// Options may appear anywhere before "--": --partial NAME=FILE, --data-var NAME=JSON, --known-helper NAME,
// --unknown-helper NAME, --known-helpers-only, --strict, --assume-objects, --prevent-indent, --no-escape,
// --explicit-partial-context, --compat, --ignore-standalone, --no-data and --log-level LEVEL. Exit status 0
// after rendering, 1 when compiling or rendering throws, 2 for usage, file and JSON errors.
package cli

import (
	"io"
	"os"
	"strings"

	"github.com/handlebars-lang/handlebars.js/internal/compiler"
	"github.com/handlebars-lang/handlebars.js/internal/js"
	"github.com/handlebars-lang/handlebars.js/internal/runtime"
)

// usageError ends the command with status 2 and one "handlebars: ..." line.
type usageError struct{ message string }

func (e *usageError) Error() string { return e.message }

var flags = map[string]func(*compiler.Options){
	"--known-helpers-only":       func(o *compiler.Options) { o.KnownHelpersOnly = true },
	"--strict":                   func(o *compiler.Options) { o.Strict = true },
	"--assume-objects":           func(o *compiler.Options) { o.AssumeObjects = true },
	"--prevent-indent":           func(o *compiler.Options) { o.PreventIndent = true },
	"--no-escape":                func(o *compiler.Options) { o.NoEscape = true },
	"--explicit-partial-context": func(o *compiler.Options) { o.ExplicitPartialContext = true },
	"--compat":                   func(o *compiler.Options) { o.Compat = true },
	"--ignore-standalone":        func(o *compiler.Options) { o.IgnoreStandalone = true },
	"--no-data":                  func(o *compiler.Options) { o.Data = false },
}

var valueOptions = map[string]bool{"--partial": true, "--data-var": true, "--known-helper": true,
	"--unknown-helper": true, "--log-level": true}

type invocation struct {
	opts     *compiler.Options
	partials [][2]string
	dataVars [][2]string
	logLevel string
	operands []string
}

func splitAssignment(option, value string) ([2]string, error) {
	at := strings.IndexByte(value, '=')
	if at <= 0 {
		return [2]string{}, &usageError{option + " needs NAME=VALUE"}
	}
	return [2]string{value[:at], value[at+1:]}, nil
}

// parseArgs reads the command line as the reference command does: unknown options and missing values are
// usage errors at the point they are met.
func parseArgs(args []string) (*invocation, error) {
	inv := &invocation{opts: &compiler.Options{Data: true}, logLevel: "info"}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			inv.operands = append(inv.operands, args[i+1:]...)
			break
		}
		if js.Length(a) < 2 || a[0] != '-' || a == "-" {
			inv.operands = append(inv.operands, a)
			continue
		}
		if set, ok := flags[a]; ok {
			set(inv.opts)
			continue
		}
		if !valueOptions[a] {
			return nil, &usageError{"unknown option " + a}
		}
		if i+1 >= len(args) {
			return nil, &usageError{a + " needs a value"}
		}
		i++
		value := args[i]
		switch a {
		case "--partial", "--data-var":
			pair, err := splitAssignment(a, value)
			if err != nil {
				return nil, err
			}
			if a == "--partial" {
				inv.partials = append(inv.partials, pair)
			} else {
				inv.dataVars = append(inv.dataVars, pair)
			}
		case "--known-helper", "--unknown-helper":
			if inv.opts.KnownHelpers == nil {
				inv.opts.KnownHelpers = map[string]bool{}
			}
			inv.opts.KnownHelpers[value] = a == "--known-helper"
		default:
			inv.logLevel = value
		}
	}
	if len(inv.operands) < 1 || len(inv.operands) > 2 {
		return nil, &usageError{"expected TEMPLATE [DATA]"}
	}
	return inv, nil
}

func readText(file string, stdin io.Reader) (string, error) {
	var data []byte
	var err error
	if file == "-" {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(file)
	}
	if err != nil {
		return "", &usageError{"cannot read " + file}
	}
	return js.DecodeUTF8(data), nil
}

func parseJSON(text, what string) (js.Value, error) {
	v, err := js.ParseJSON(text)
	if err != nil {
		return nil, &usageError{"invalid JSON in " + what}
	}
	return v, nil
}

// Run executes the command and returns its exit status.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	output, err := render(args, stdin, stdout, stderr)
	if err != nil {
		if u, ok := err.(*usageError); ok {
			_, _ = stderr.Write(js.UTF8("handlebars: " + u.message + "\n"))
			return 2
		}
		name, message := "Error", err.Error()
		if e, ok := err.(*js.Error); ok {
			name, message = e.Name, e.Message
		}
		_, _ = stderr.Write(js.UTF8(name + ": " + message + "\n"))
		return 1
	}
	if _, err := stdout.Write(js.UTF8(output)); err != nil {
		return 1
	}
	return 0
}

func render(args []string, stdin io.Reader, stdout, stderr io.Writer) (string, error) {
	inv, err := parseArgs(args)
	if err != nil {
		return "", err
	}
	template, err := readText(inv.operands[0], stdin)
	if err != nil {
		return "", err
	}
	var context js.Value = js.NewObject()
	if len(inv.operands) > 1 {
		text, err := readText(inv.operands[1], stdin)
		if err != nil {
			return "", err
		}
		if context, err = parseJSON(text, inv.operands[1]); err != nil {
			return "", err
		}
	}
	env := runtime.NewEnv(stdout, stderr)
	for _, p := range inv.partials {
		text, err := readText(p[1], stdin)
		if err != nil {
			return "", err
		}
		env.RegisterPartial(p[0], text)
	}
	runtimeData := js.NewObject()
	for _, d := range inv.dataVars {
		v, err := parseJSON(d[1], "--data-var "+d[0])
		if err != nil {
			return "", err
		}
		runtimeData.Set(d[0], v)
	}
	env.Logger.Level = inv.logLevel
	registerHelpers(env)
	registerDecorators(env)
	runOptions := js.NewObject()
	runOptions.Set("data", runtimeData)
	out, err := env.Compile(template, inv.opts).Call(js.Undefined, []js.Value{context, runOptions})
	if err != nil {
		return "", err
	}
	s, _ := out.(string)
	return s, nil
}
