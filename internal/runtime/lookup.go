// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package runtime

import (
	"strconv"

	"github.com/handlebars-lang/handlebars.js/internal/ast"
	"github.com/handlebars-lang/handlebars.js/internal/js"
)

func itoa(n int) string { return strconv.Itoa(n) }

// deniedSilently are the inherited methods the prototype-access rules refuse without logging (their whitelist
// entry is false rather than missing).
var deniedSilently = map[string]bool{"constructor": true, "__defineGetter__": true, "__defineSetter__": true,
	"__lookupGetter__": true, "__lookupSetter__": true}

// lookupProperty is container.lookupProperty: own properties are returned; inherited ones are refused, and the
// refusal is logged once per property name unless the whitelist names it.
func (c *container) lookupProperty(parent js.Value, name string) (js.Value, error) {
	result, own, err := js.Get(parent, name)
	if err != nil {
		return nil, err
	}
	if js.IsNullish(result) {
		return result, nil
	}
	if own {
		return result, nil
	}
	inherited, ok := result.(*js.Inherited)
	if !ok {
		return js.Undefined, nil
	}
	if inherited.Function && !deniedSilently[name] {
		if err := c.t.env.logDenied(name); err != nil {
			return nil, err
		}
	}
	return js.Undefined, nil
}

func (e *Env) logDenied(name string) error {
	if e.loggedProperties[name] {
		return nil
	}
	e.loggedProperties[name] = true
	return e.Logger.Log("error", "Handlebars: Access has been denied to resolve the property \""+name+
		"\" because it is not an \"own property\" of its parent.\n"+
		"You can add a runtime option to disable the check or this warning:\n"+
		"See https://handlebarsjs.com/api-reference/runtime-options.html#options-to-control-prototype-access for details")
}

// strict is container.strict: the last segment of a strict-mode path must exist.
func (c *container) strict(obj js.Value, name string, loc *ast.SourceLocation) (js.Value, error) {
	missing := !js.ToBoolean(obj)
	if !missing {
		in, err := js.In(name, obj)
		if err != nil {
			return nil, err
		}
		missing = !in
	}
	if missing {
		text, err := js.ConcatString(obj)
		if err != nil {
			return nil, err
		}
		return nil, exception(`"`+name+`" not defined in `+text, loc)
	}
	return c.lookupProperty(obj, name)
}

// exception is Handlebars' Exception(message, node).
func exception(message string, loc *ast.SourceLocation) error {
	return &js.Error{Name: "Error", Message: ast.ExceptionMessage(message, loc)}
}

// contextAt is the generated contextName(depth).
func (s *scope) contextAt(depth int) js.Value {
	if depth == 0 {
		return s.ctx
	}
	if depth < len(s.depths) {
		return s.depths[depth]
	}
	return js.Undefined
}

// path evaluates a path expression like the code the original generates for it.
func (s *scope) path(p *ast.PathExpression) (js.Value, error) {
	opts := s.options()
	if ref, ok := s.c.t.compiled.BlockParamRefs[p]; ok {
		if s.blockParams == nil {
			return nil, js.TypeError("Cannot read properties of undefined (reading '" + itoa(ref.Depth) + "')")
		}
		var frame js.Value = js.Undefined
		if ref.Depth < len(s.blockParams) {
			frame = s.blockParams[ref.Depth]
		}
		v, err := get(frame, itoa(ref.Index))
		if err != nil {
			return nil, err
		}
		if inh, ok := v.(*js.Inherited); ok {
			v = inheritedValue(inh)
		}
		return s.resolve(v, p.Parts, 1, false, false, p.Loc)
	}
	if len(p.Parts) == 0 || p.Parts[0] == "" {
		// A path whose first segment is empty (`this`, `.`, `[]`) is the context itself.
		return s.contextAt(p.Depth), nil
	}
	if p.Data {
		var base js.Value = s.data
		for d := p.Depth; js.ToBoolean(base) && d > 0; d-- {
			parent, err := get(base, "_parent")
			if err != nil {
				return nil, err
			}
			base = parent
		}
		return s.resolve(base, p.Parts, 0, true, p.Strict, p.Loc)
	}
	start := 0
	var base js.Value
	if !ast.ScopedID(p) && opts.Compat && p.Depth == 0 {
		v, err := s.depthedLookup(p.Parts[0])
		if err != nil {
			return nil, err
		}
		base = v
		start = 1
	} else {
		base = s.contextAt(p.Depth)
	}
	return s.resolve(base, p.Parts, start, p.Falsy, p.Strict, p.Loc)
}

// inheritedValue stands in for a built-in prototype value read directly (outside lookupProperty).
func inheritedValue(inh *js.Inherited) js.Value {
	return &js.Func{Name: inh.Name, Call: func(js.Value, []js.Value) (js.Value, error) { return js.Undefined, nil }}
}

// depthedLookup is container.lookup(depths, name) of compat mode.
func (s *scope) depthedLookup(name string) (js.Value, error) {
	for _, d := range s.depths {
		// depths[i] && lookupProperty(depths[i], name): a falsy context that is not null ends the search.
		if !js.ToBoolean(d) {
			if !js.IsNullish(d) {
				return d, nil
			}
			continue
		}
		r, err := s.c.lookupProperty(d, name)
		if err != nil {
			return nil, err
		}
		if !js.IsNullish(r) {
			return r, nil
		}
	}
	return js.Undefined, nil
}

// resolve is resolvePath: the remaining segments, guarded by `!= null` (or `&&` for helper names and data),
// or in strict and assumeObjects modes read directly, with container.strict checking the last segment.
func (s *scope) resolve(current js.Value, parts []string, start int, falsy, strict bool, loc *ast.SourceLocation) (js.Value, error) {
	opts := s.options()
	if opts.Strict || opts.AssumeObjects {
		requireTerminal := opts.Strict && strict
		n := len(parts)
		if requireTerminal {
			n--
		}
		for i := start; i < n; i++ {
			v, err := s.c.lookupProperty(current, parts[i])
			if err != nil {
				return nil, err
			}
			current = v
		}
		if requireTerminal {
			last := "undefined"
			if n >= 0 {
				last = parts[n]
			}
			return s.c.strict(current, last, loc)
		}
		return current, nil
	}
	for i := start; i < len(parts); i++ {
		if falsy {
			if !js.ToBoolean(current) {
				continue
			}
		} else if js.IsNullish(current) {
			continue
		}
		v, err := s.c.lookupProperty(current, parts[i])
		if err != nil {
			return nil, err
		}
		current = v
	}
	return current, nil
}

var escapeTable = map[byte]string{'&': "&amp;", '<': "&lt;", '>': "&gt;", '"': "&quot;", '\'': "&#x27;",
	'`': "&#x60;", '=': "&#x3D;"}

// escapeExpression is Handlebars.Utils.escapeExpression.
func escapeExpression(v js.Value) (string, error) {
	str, ok := v.(string)
	if !ok {
		if ss, isSafe := v.(*js.SafeString); isSafe {
			return js.ConcatString(ss.S)
		}
		if js.IsNullish(v) {
			return "", nil
		}
		if !js.ToBoolean(v) {
			return js.ConcatString(v)
		}
		s, err := js.ConcatString(v)
		if err != nil {
			return "", err
		}
		str = s
	}
	return EscapeString(str), nil
}

// EscapeString replaces & < > " ' ` = with their HTML entities.
func EscapeString(str string) string {
	var out []byte
	for i := 0; i < len(str); i++ {
		if rep, ok := escapeTable[str[i]]; ok {
			if out == nil {
				out = append(make([]byte, 0, len(str)+16), str[:i]...)
			}
			out = append(out, rep...)
		} else if out != nil {
			out = append(out, str[i])
		}
	}
	if out == nil {
		return str
	}
	return string(out)
}
