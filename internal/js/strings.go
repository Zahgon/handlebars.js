// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package js

import (
	"unicode/utf8"
)

// ToUnits returns the UTF-16 code units of a WTF-8 string.
func ToUnits(s string) []uint16 {
	units := make([]uint16, 0, len(s))
	for i := 0; i < len(s); {
		b := s[i]
		if b < 0x80 {
			units = append(units, uint16(b))
			i++
			continue
		}
		// A WTF-8 surrogate: ED A0..BF 80..BF.
		if b == 0xED && i+2 < len(s) && s[i+1] >= 0xA0 && s[i+1] <= 0xBF {
			cu := uint16(0xD000) | uint16(s[i+1]&0x3F)<<6 | uint16(s[i+2]&0x3F)
			units = append(units, cu)
			i += 3
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if r >= 0x10000 {
			r -= 0x10000
			units = append(units, uint16(0xD800+(r>>10)), uint16(0xDC00+(r&0x3FF)))
		} else {
			units = append(units, uint16(r))
		}
	}
	return units
}

// FromUnits encodes UTF-16 code units as WTF-8: pairs become their character, lone surrogates their own
// three-byte sequence.
func FromUnits(units []uint16) string {
	buf := make([]byte, 0, len(units))
	for i := 0; i < len(units); i++ {
		u := units[i]
		switch {
		case u < 0x80:
			buf = append(buf, byte(u))
		case u >= 0xD800 && u <= 0xDBFF && i+1 < len(units) && units[i+1] >= 0xDC00 && units[i+1] <= 0xDFFF:
			r := 0x10000 + (rune(u)-0xD800)<<10 + rune(units[i+1]) - 0xDC00
			buf = utf8.AppendRune(buf, r)
			i++
		case u >= 0xD800 && u <= 0xDFFF:
			buf = append(buf, 0xED, byte(0x80|(u>>6)&0x3F), byte(0x80|u&0x3F))
		default:
			buf = utf8.AppendRune(buf, rune(u))
		}
	}
	return string(buf)
}

// Canonical re-encodes s so that surrogate halves joined by concatenation form their character.
func Canonical(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == 0xED {
			return FromUnits(ToUnits(s))
		}
	}
	return s
}

// SameUnits reports whether two strings hold the same code units.
func SameUnits(a, b string) bool {
	if a == b {
		return true
	}
	return Canonical(a) == Canonical(b)
}

// UTF8 encodes a string as Node.js writes it to a stream: lone surrogates become U+FFFD.
func UTF8(s string) []byte {
	s = Canonical(s)
	buf := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		if s[i] == 0xED && i+2 < len(s) && s[i+1] >= 0xA0 {
			buf = append(buf, "�"...)
			i += 3
			continue
		}
		buf = append(buf, s[i])
		i++
	}
	return buf
}

// DecodeUTF8 decodes bytes like Node.js's TextDecoder / Buffer.toString('utf8'): every maximal ill-formed
// subsequence becomes one U+FFFD (the WHATWG decoder). A byte order mark is kept.
func DecodeUTF8(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}
	buf := make([]byte, 0, len(data)+8)
	for i := 0; i < len(data); {
		b := data[i]
		if b < 0x80 {
			buf = append(buf, b)
			i++
			continue
		}
		need, lower, upper := 0, byte(0x80), byte(0xBF)
		switch {
		case b >= 0xC2 && b <= 0xDF:
			need = 1
		case b == 0xE0:
			need, lower = 2, 0xA0
		case b == 0xED:
			need, upper = 2, 0x9F
		case b >= 0xE1 && b <= 0xEF:
			need = 2
		case b == 0xF0:
			need, lower = 3, 0x90
		case b == 0xF4:
			need, upper = 3, 0x8F
		case b >= 0xF1 && b <= 0xF3:
			need = 3
		default:
			buf = append(buf, "�"...)
			i++
			continue
		}
		j := i + 1
		ok := true
		for k := 0; k < need; k++ {
			if j >= len(data) || data[j] < lower || data[j] > upper {
				ok = false
				break
			}
			lower, upper = 0x80, 0xBF
			j++
		}
		if !ok {
			buf = append(buf, "�"...)
			i = j
			continue
		}
		buf = append(buf, data[i:j]...)
		i = j
	}
	return string(buf)
}

// Length is String.prototype.length.
func Length(s string) int {
	n := 0
	for i := 0; i < len(s); {
		b := s[i]
		switch {
		case b < 0x80:
			i++
			n++
		case b == 0xED && i+2 < len(s) && s[i+1] >= 0xA0:
			i += 3
			n++
		default:
			r, size := utf8.DecodeRuneInString(s[i:])
			i += size
			n++
			if r >= 0x10000 {
				n++
			}
		}
	}
	return n
}

// Split is String.prototype.split with a string separator.
func Split(s, sep string) []string {
	units := ToUnits(s)
	if sep == "" {
		out := make([]string, len(units))
		for i := range units {
			out[i] = FromUnits(units[i : i+1])
		}
		return out
	}
	sepUnits := ToUnits(sep)
	var out []string
	start := 0
	for i := 0; i+len(sepUnits) <= len(units); {
		if equalUnits(units[i:i+len(sepUnits)], sepUnits) {
			out = append(out, FromUnits(units[start:i]))
			i += len(sepUnits)
			start = i
			continue
		}
		i++
	}
	return append(out, FromUnits(units[start:]))
}

func equalUnits(a, b []uint16) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Less compares two strings by UTF-16 code units, like the < operator.
func Less(a, b string) bool {
	ua, ub := ToUnits(a), ToUnits(b)
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}
