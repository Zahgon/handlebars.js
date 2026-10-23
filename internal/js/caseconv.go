// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package js

import (
	"strings"
	"unicode"
)

// ToUpperCase is String.prototype.toUpperCase: full Unicode case mapping, including the mappings that change
// the length (ß becomes SS).
func ToUpperCase(s string) string {
	return mapCase(s, upperCase, false)
}

// ToLowerCase is String.prototype.toLowerCase, with the final-sigma rule: a capital sigma that ends a word
// becomes ς.
func ToLowerCase(s string) string {
	return mapCase(s, lowerCase, true)
}

func mapCase(s string, table map[rune]string, lower bool) string {
	units := ToUnits(s)
	runes := decodeUnits(units)
	var b strings.Builder
	for i, r := range runes {
		if r < 0 {
			b.WriteString(FromUnits([]uint16{uint16(-r)}))
			continue
		}
		if lower && r == 0x3A3 && finalSigma(runes, i) {
			b.WriteString("ς")
			continue
		}
		if m, ok := table[r]; ok {
			b.WriteString(m)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// decodeUnits returns the code points of units; a lone surrogate u is returned as -u.
func decodeUnits(units []uint16) []rune {
	out := make([]rune, 0, len(units))
	for i := 0; i < len(units); i++ {
		u := units[i]
		if u >= 0xD800 && u <= 0xDBFF && i+1 < len(units) && units[i+1] >= 0xDC00 && units[i+1] <= 0xDFFF {
			out = append(out, 0x10000+(rune(u)-0xD800)<<10+rune(units[i+1])-0xDC00)
			i++
			continue
		}
		if u >= 0xD800 && u <= 0xDFFF {
			out = append(out, -rune(u))
			continue
		}
		out = append(out, rune(u))
	}
	return out
}

func isCased(r rune) bool {
	return r > 0 && (unicode.IsUpper(r) || unicode.IsLower(r) || unicode.IsTitle(r) ||
		unicode.Is(unicode.Other_Lowercase, r) || unicode.Is(unicode.Other_Uppercase, r))
}

func isCaseIgnorable(r rune) bool {
	if r <= 0 {
		return false
	}
	switch r {
	case '\'', '.', ':', 0xB7, 0x387, 0x55F, 0x5F4, 0x2018, 0x2019, 0x2024, 0x2027, 0xFE13, 0xFE52, 0xFE55,
		0xFF07, 0xFF0E, 0xFF1A:
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf, unicode.Lm, unicode.Sk)
}

// finalSigma applies the Final_Sigma condition of the Unicode special casing rules at position i.
func finalSigma(runes []rune, i int) bool {
	before := false
	for j := i - 1; j >= 0; j-- {
		if isCaseIgnorable(runes[j]) {
			continue
		}
		before = isCased(runes[j])
		break
	}
	if !before {
		return false
	}
	for j := i + 1; j < len(runes); j++ {
		if isCaseIgnorable(runes[j]) {
			continue
		}
		return !isCased(runes[j])
	}
	return true
}
