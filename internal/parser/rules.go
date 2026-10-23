// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package parser

// The lexer rules of src/handlebars.l, numbered as in the generated lexer. JavaScript regular expressions with
// lookahead and lazy or backtracking repetition are matched by hand here; each matcher returns the length of the
// match at the start of the input, or -1. A rule may match the empty string.

func matchRule(rule int, s []uint16) int {
	switch rule {
	case 0: // [^\x00]*?/("{{")
		for i := 0; i < len(s); i++ {
			if hasPrefix(s[i:], "{{") {
				return i
			}
			if s[i] == 0 {
				return -1
			}
		}
		return -1
	case 1: // [^\x00]+
		n := 0
		for n < len(s) && s[n] != 0 {
			n++
		}
		return positive(n)
	case 2: // <emu>[^\x00]{2,}?/("{{"|"\\{{"|"\\\\{{"|<<EOF>>)
		for i := 1; i <= len(s); i++ {
			if s[i-1] == 0 {
				return -1
			}
			if i < 2 {
				continue
			}
			rest := s[i:]
			if len(rest) == 0 || hasPrefix(rest, "{{") || hasPrefix(rest, `\{{`) || hasPrefix(rest, `\\{{`) {
				return i
			}
		}
		return -1
	case 3: // <raw>"{{{{"/[^/]
		if hasPrefix(s, "{{{{") && len(s) > 4 && s[4] != '/' {
			return 4
		}
		return -1
	case 4: // <raw>"{{{{/"{ID-chars}+/[=}\s\/.]"}}}}"
		if !hasPrefix(s, "{{{{/") {
			return -1
		}
		k := 5
		for k < len(s) && isIDChar(s[k]) {
			k++
		}
		// The lookahead character must also start the literal "}}}}".
		if k > 5 && hasPrefix(s[k:], "}}}}") {
			return k + 4
		}
		return -1
	case 5: // <raw>[^\x00]+?/("{{{{")
		for i := 1; i <= len(s); i++ {
			if s[i-1] == 0 {
				return -1
			}
			if hasPrefix(s[i:], "{{{{") {
				return i
			}
		}
		return -1
	case 6: // <com>[\s\S]*?"--"{RIGHT_STRIP}?"}}"
		for i := 0; i+4 <= len(s); i++ {
			if hasPrefix(s[i:], "--~}}") {
				return i + 5
			}
			if hasPrefix(s[i:], "--}}") {
				return i + 4
			}
		}
		return -1
	case 7:
		return literal(s, "(")
	case 8:
		return literal(s, ")")
	case 9:
		return literal(s, "{{{{")
	case 10:
		return literal(s, "}}}}")
	case 11:
		return openTag(s, func(r []uint16) int { return literal(r, ">") })
	case 12:
		return openTag(s, func(r []uint16) int { return literal(r, "#>") })
	case 13:
		return openTag(s, func(r []uint16) int {
			if literal(r, "#") < 0 {
				return -1
			}
			return 1 + optional(r[1:], '*')
		})
	case 14:
		return openTag(s, func(r []uint16) int { return literal(r, "/") })
	case 15: // "{{"~?"^"\s*~?"}}"
		return openTag(s, func(r []uint16) int {
			if literal(r, "^") < 0 {
				return -1
			}
			return seq(r, 1, closeAfterSpace)
		})
	case 16: // "{{"~?\s*"else"\s*~?"}}"
		return openTag(s, func(r []uint16) int {
			n := spaces(r)
			if literal(r[n:], "else") < 0 {
				return -1
			}
			return seq(r, n+4, closeAfterSpace)
		})
	case 17:
		return openTag(s, func(r []uint16) int { return literal(r, "^") })
	case 18: // "{{"~?\s*"else"\b
		return openTag(s, func(r []uint16) int {
			n := spaces(r)
			if literal(r[n:], "else") < 0 {
				return -1
			}
			n += 4
			if n < len(r) && isWordChar(r[n]) {
				return -1
			}
			return n
		})
	case 19:
		return openTag(s, func(r []uint16) int { return literal(r, "{") })
	case 20:
		return openTag(s, func(r []uint16) int { return literal(r, "&") })
	case 21:
		return openTag(s, func(r []uint16) int { return literal(r, "!--") })
	case 22: // "{{"~?"!"[\s\S]*?"}}"
		return openTag(s, func(r []uint16) int {
			if literal(r, "!") < 0 {
				return -1
			}
			for i := 1; i+2 <= len(r); i++ {
				if hasPrefix(r[i:], "}}") {
					return i + 2
				}
			}
			return -1
		})
	case 23:
		return openTag(s, func(r []uint16) int { return optional(r, '*') })
	case 24:
		return literal(s, "=")
	case 25:
		return literal(s, "..")
	case 26: // "."/{LOOKAHEAD}
		if literal(s, ".") == 1 && len(s) > 1 && isLookahead(s[1]) {
			return 1
		}
		return -1
	case 27: // [\/.]
		if len(s) > 0 && (s[0] == '/' || s[0] == '.') {
			return 1
		}
		return -1
	case 28: // \s+
		return positive(spaces(s))
	case 29: // "}"~?"}}"
		if literal(s, "}~}}") > 0 {
			return 4
		}
		return literal(s, "}}}")
	case 30: // ~?"}}"
		if literal(s, "~}}") > 0 {
			return 3
		}
		return literal(s, "}}")
	case 31: // '"'("\\"["]|[^"])*'"'
		return quoted(s, '"', '"')
	case 32:
		return quoted(s, '\'', '\'')
	case 33:
		return literal(s, "@")
	case 34:
		return keyword(s, "true")
	case 35:
		return keyword(s, "false")
	case 36:
		return keyword(s, "undefined")
	case 37:
		return keyword(s, "null")
	case 38:
		return number(s)
	case 39: // "as"\s+"|"
		if literal(s, "as") < 0 {
			return -1
		}
		n := spaces(s[2:])
		if n == 0 || literal(s[2+n:], "|") < 0 {
			return -1
		}
		return 3 + n
	case 40:
		return literal(s, "|")
	case 41: // {ID}, i.e. ID-chars+/{LOOKAHEAD}
		n := 0
		for n < len(s) && isIDChar(s[n]) {
			n++
		}
		// Every lookahead character is outside the ID class, so only the longest run can be followed by one.
		if n > 0 && n < len(s) && isLookahead(s[n]) {
			return n
		}
		return -1
	case 42: // '['('\\]'|[^\]])*']'
		return quoted(s, '[', ']')
	case 43: // .
		if len(s) > 0 && !isLineTerminator(s[0]) {
			return 1
		}
		return -1
	case 44: // <<EOF>>
		if len(s) == 0 {
			return 0
		}
		return -1
	}
	return -1
}

func hasPrefix(s []uint16, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if s[i] != uint16(prefix[i]) {
			return false
		}
	}
	return true
}

func literal(s []uint16, text string) int {
	if hasPrefix(s, text) {
		return len(text)
	}
	return -1
}

func positive(n int) int {
	if n > 0 {
		return n
	}
	return -1
}

func optional(s []uint16, c uint16) int {
	if len(s) > 0 && s[0] == c {
		return 1
	}
	return 0
}

func spaces(s []uint16) int {
	n := 0
	for n < len(s) && isSpace(s[n]) {
		n++
	}
	return n
}

// seq matches rest after the first n code units of s, returning the total length.
func seq(s []uint16, n int, rest func([]uint16) int) int {
	m := rest(s[n:])
	if m < 0 {
		return -1
	}
	return n + m
}

// closeAfterSpace is \s*~?"}}".
func closeAfterSpace(s []uint16) int {
	n := spaces(s)
	if literal(s[n:], "~}}") > 0 {
		return n + 3
	}
	if literal(s[n:], "}}") > 0 {
		return n + 2
	}
	return -1
}

// openTag matches "{{" and an optional "~" before rest. No rest pattern can start with "~", so taking the "~"
// whenever it is present is what the backtracking original does.
func openTag(s []uint16, rest func([]uint16) int) int {
	if !hasPrefix(s, "{{") {
		return -1
	}
	n := 2 + optional(s[2:], '~')
	return seq(s, n, rest)
}

// isLookahead is the grammar's LOOKAHEAD class [=~}\s\/.)|].
func isLookahead(c uint16) bool {
	switch c {
	case '=', '~', '}', '/', '.', ')', '|':
		return true
	}
	return isSpace(c)
}

// isLiteralLookahead is the grammar's LITERAL_LOOKAHEAD class [~}\s)].
func isLiteralLookahead(c uint16) bool {
	return c == '~' || c == '}' || c == ')' || isSpace(c)
}

// isWordChar is the ASCII word class used by \b.
func isWordChar(c uint16) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// keyword matches a literal keyword that must be followed by a LITERAL_LOOKAHEAD character.
func keyword(s []uint16, word string) int {
	if hasPrefix(s, word) && len(s) > len(word) && isLiteralLookahead(s[len(word)]) {
		return len(word)
	}
	return -1
}

// number matches -?[0-9]+(?:\.[0-9]+)?/{LITERAL_LOOKAHEAD} in the order a backtracking matcher tries: the
// most digits first, with the fraction before without it.
func number(s []uint16) int {
	sign := 0
	if len(s) > 0 && s[0] == '-' {
		sign = 1
	}
	digits := 0
	for sign+digits < len(s) && isDigit(s[sign+digits]) {
		digits++
	}
	followed := func(n int) bool { return n < len(s) && isLiteralLookahead(s[n]) }
	for d := digits; d >= 1; d-- {
		end := sign + d
		if d == digits && end < len(s) && s[end] == '.' {
			frac := 0
			for end+1+frac < len(s) && isDigit(s[end+1+frac]) {
				frac++
			}
			for f := frac; f >= 1; f-- {
				if followed(end + 1 + f) {
					return end + 1 + f
				}
			}
		}
		if followed(end) {
			return end
		}
	}
	return -1
}

func isDigit(c uint16) bool { return c >= '0' && c <= '9' }

// quoted matches open ( "\\" close | [^close] )* close with the original's backtracking order: at each step the
// escape alternative is tried first, then any other code unit, and only then the end of the repetition.
func quoted(s []uint16, open, close uint16) int {
	if len(s) == 0 || s[0] != open {
		return -1
	}
	failed := make(map[int]bool)
	var from func(pos int) int
	from = func(pos int) int {
		if failed[pos] {
			return -1
		}
		if pos < len(s) {
			if s[pos] == '\\' && pos+1 < len(s) && s[pos+1] == close {
				if end := from(pos + 2); end >= 0 {
					return end
				}
			}
			if s[pos] != close {
				if end := from(pos + 1); end >= 0 {
					return end
				}
			}
			if s[pos] == close {
				return pos + 1
			}
		}
		failed[pos] = true
		return -1
	}
	return from(1)
}
