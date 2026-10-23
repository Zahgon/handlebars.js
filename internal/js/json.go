// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package js

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

var errJSON = errors.New("invalid JSON")

// ParseJSON is JSON.parse: it accepts exactly the JSON grammar (whitespace is tab, newline, carriage return and
// space only, so a byte order mark is an error) and builds objects with JavaScript key order; a repeated key keeps
// its first position and takes the last value.
func ParseJSON(text string) (Value, error) {
	p := &jsonParser{s: text}
	p.space()
	v, err := p.value()
	if err != nil {
		return nil, err
	}
	p.space()
	if p.i != len(p.s) {
		return nil, errJSON
	}
	return v, nil
}

type jsonParser struct {
	s string
	i int
}

func (p *jsonParser) space() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *jsonParser) literal(word string, v Value) (Value, error) {
	if strings.HasPrefix(p.s[p.i:], word) {
		p.i += len(word)
		return v, nil
	}
	return nil, errJSON
}

func (p *jsonParser) value() (Value, error) {
	if p.i >= len(p.s) {
		return nil, errJSON
	}
	switch c := p.s[p.i]; {
	case c == '{':
		p.i++
		obj := NewObject()
		p.space()
		if p.i < len(p.s) && p.s[p.i] == '}' {
			p.i++
			return obj, nil
		}
		for {
			p.space()
			if p.i >= len(p.s) || p.s[p.i] != '"' {
				return nil, errJSON
			}
			key, err := p.str()
			if err != nil {
				return nil, err
			}
			p.space()
			if p.i >= len(p.s) || p.s[p.i] != ':' {
				return nil, errJSON
			}
			p.i++
			p.space()
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			obj.Set(key, v)
			p.space()
			if p.i >= len(p.s) {
				return nil, errJSON
			}
			if p.s[p.i] == ',' {
				p.i++
				continue
			}
			if p.s[p.i] == '}' {
				p.i++
				return obj, nil
			}
			return nil, errJSON
		}
	case c == '[':
		p.i++
		arr := &Array{}
		p.space()
		if p.i < len(p.s) && p.s[p.i] == ']' {
			p.i++
			return arr, nil
		}
		for {
			p.space()
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			arr.Elems = append(arr.Elems, v)
			p.space()
			if p.i >= len(p.s) {
				return nil, errJSON
			}
			if p.s[p.i] == ',' {
				p.i++
				continue
			}
			if p.s[p.i] == ']' {
				p.i++
				return arr, nil
			}
			return nil, errJSON
		}
	case c == '"':
		return p.str()
	case c == 't':
		return p.literal("true", true)
	case c == 'f':
		return p.literal("false", false)
	case c == 'n':
		return p.literal("null", Null)
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	}
	return nil, errJSON
}

func (p *jsonParser) number() (Value, error) {
	start := p.i
	if p.s[p.i] == '-' {
		p.i++
	}
	digits := func() int {
		n := 0
		for p.i < len(p.s) && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
			p.i++
			n++
		}
		return n
	}
	if p.i < len(p.s) && p.s[p.i] == '0' {
		p.i++
	} else if digits() == 0 {
		return nil, errJSON
	}
	if p.i < len(p.s) && p.s[p.i] == '.' {
		p.i++
		if digits() == 0 {
			return nil, errJSON
		}
	}
	if p.i < len(p.s) && (p.s[p.i] == 'e' || p.s[p.i] == 'E') {
		p.i++
		if p.i < len(p.s) && (p.s[p.i] == '+' || p.s[p.i] == '-') {
			p.i++
		}
		if digits() == 0 {
			return nil, errJSON
		}
	}
	f, err := strconv.ParseFloat(p.s[start:p.i], 64)
	if err != nil && !math.IsInf(f, 0) {
		return nil, errJSON
	}
	return f, nil
}

func (p *jsonParser) str() (string, error) {
	p.i++ // opening quote
	var units []uint16
	plain := p.i
	simple := true
	for {
		if p.i >= len(p.s) {
			return "", errJSON
		}
		c := p.s[p.i]
		switch {
		case c == '"':
			if simple {
				s := p.s[plain:p.i]
				p.i++
				return s, nil
			}
			p.i++
			return FromUnits(units), nil
		case c < 0x20:
			return "", errJSON
		case c == '\\':
			if simple {
				units = ToUnits(p.s[plain:p.i])
				simple = false
			}
			p.i++
			if p.i >= len(p.s) {
				return "", errJSON
			}
			e := p.s[p.i]
			p.i++
			switch e {
			case '"', '\\', '/':
				units = append(units, uint16(e))
			case 'b':
				units = append(units, '\b')
			case 'f':
				units = append(units, '\f')
			case 'n':
				units = append(units, '\n')
			case 'r':
				units = append(units, '\r')
			case 't':
				units = append(units, '\t')
			case 'u':
				if p.i+4 > len(p.s) {
					return "", errJSON
				}
				n, err := strconv.ParseUint(p.s[p.i:p.i+4], 16, 16)
				if err != nil || strings.ContainsAny(p.s[p.i:p.i+4], "+-_") {
					return "", errJSON
				}
				units = append(units, uint16(n))
				p.i += 4
			default:
				return "", errJSON
			}
		default:
			if simple {
				p.i++
				continue
			}
			start := p.i
			p.i++
			for p.i < len(p.s) && p.s[p.i] != '"' && p.s[p.i] != '\\' && p.s[p.i] >= 0x20 {
				p.i++
			}
			units = append(units, ToUnits(p.s[start:p.i])...)
		}
	}
}

// Stringify is JSON.stringify(v) without replacer or indentation; ok is false when the result is undefined.
func Stringify(v Value) (string, bool, error) {
	var b strings.Builder
	ok, err := stringifyTo(&b, v)
	return b.String(), ok, err
}

func stringifyTo(b *strings.Builder, v Value) (bool, error) {
	switch x := v.(type) {
	case undefinedType, nil, *Func, *Inherited:
		return false, nil
	case nullType:
		b.WriteString("null")
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			b.WriteString("null")
		} else {
			b.WriteString(NumberToString(x))
		}
	case string:
		QuoteJSON(b, x)
	case *Array:
		b.WriteByte('[')
		for i, e := range x.Elems {
			if i > 0 {
				b.WriteByte(',')
			}
			ok, err := stringifyTo(b, e)
			if err != nil {
				return false, err
			}
			if !ok {
				b.WriteString("null")
			}
		}
		b.WriteByte(']')
	case *Object:
		writeMembers(b, x.Keys(), x.Value)
	case *SafeString:
		writeMembers(b, []string{"string"}, func(string) Value { return x.S })
	}
	return true, nil
}

func writeMembers(b *strings.Builder, keys []string, get func(string) Value) {
	b.WriteByte('{')
	first := true
	for _, k := range keys {
		var part strings.Builder
		if ok, _ := stringifyTo(&part, get(k)); !ok {
			continue
		}
		if !first {
			b.WriteByte(',')
		}
		first = false
		QuoteJSON(b, k)
		b.WriteByte(':')
		b.WriteString(part.String())
	}
	b.WriteByte('}')
}

const hexDigits = "0123456789abcdef"

// QuoteJSON writes s as a JSON string literal the way V8 does: lone surrogates are escaped.
func QuoteJSON(b *strings.Builder, s string) {
	b.WriteByte('"')
	units := ToUnits(s)
	for i := 0; i < len(units); i++ {
		u := units[i]
		switch {
		case u == '"':
			b.WriteString(`\"`)
		case u == '\\':
			b.WriteString(`\\`)
		case u == '\b':
			b.WriteString(`\b`)
		case u == '\f':
			b.WriteString(`\f`)
		case u == '\n':
			b.WriteString(`\n`)
		case u == '\r':
			b.WriteString(`\r`)
		case u == '\t':
			b.WriteString(`\t`)
		case u < 0x20:
			b.WriteString(`\u00`)
			b.WriteByte(hexDigits[u>>4])
			b.WriteByte(hexDigits[u&15])
		case u >= 0xD800 && u <= 0xDBFF && i+1 < len(units) && units[i+1] >= 0xDC00 && units[i+1] <= 0xDFFF:
			b.WriteString(FromUnits(units[i : i+2]))
			i++
		case u >= 0xD800 && u <= 0xDFFF:
			b.WriteString(`\u`)
			for shift := 12; shift >= 0; shift -= 4 {
				b.WriteByte(hexDigits[(u>>uint(shift))&15])
			}
		default:
			b.WriteString(FromUnits(units[i : i+1]))
		}
	}
	b.WriteByte('"')
}
