// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package js

import (
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

func nan() float64 { return math.NaN() }

// NumberToString is Number.prototype.toString(): the shortest digits that round-trip, written in fixed notation
// for exponents from -7 to 20 and in exponential notation (1e+21, 1.5e-7) outside that range.
func NumberToString(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		return "0"
	}
	sign := ""
	if f < 0 {
		sign = "-"
		f = -f
	}
	e := strconv.FormatFloat(f, 'e', -1, 64)
	mant, expPart, _ := strings.Cut(e, "e")
	digits := strings.Replace(mant, ".", "", 1)
	exp, _ := strconv.Atoi(expPart)
	k := len(digits)
	n := exp + 1
	switch {
	case k <= n && n <= 21:
		return sign + digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return sign + digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		return sign + "0." + strings.Repeat("0", -n) + digits
	}
	expSign := "+"
	if n-1 < 0 {
		expSign = "-"
	}
	expStr := strconv.Itoa(absInt(n - 1))
	if k == 1 {
		return sign + digits + "e" + expSign + expStr
	}
	return sign + digits[:1] + "." + digits[1:] + "e" + expSign + expStr
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

var decimalLiteral = regexp.MustCompile(`^[+-]?(?:[0-9]+\.?[0-9]*|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

// isJSWhitespace reports whether r is a WhiteSpace or LineTerminator code point, the set String.prototype.trim
// removes.
func isJSWhitespace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xA0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}

// TrimJS is String.prototype.trim.
func TrimJS(s string) string {
	return strings.TrimFunc(s, isJSWhitespace)
}

// StringToNumber is the ToNumber conversion of a string.
func StringToNumber(s string) float64 {
	s = TrimJS(s)
	if s == "" {
		return 0
	}
	switch s {
	case "Infinity", "+Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}
	if len(s) > 2 && s[0] == '0' {
		base := 0
		switch s[1] {
		case 'x', 'X':
			base = 16
		case 'o', 'O':
			base = 8
		case 'b', 'B':
			base = 2
		}
		if base != 0 {
			n, ok := new(big.Int).SetString(s[2:], base)
			if !ok || strings.ContainsAny(s[2:], "_+-") {
				return nan()
			}
			f, _ := new(big.Float).SetInt(n).Float64()
			return f
		}
	}
	if !decimalLiteral.MatchString(s) {
		return nan()
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil && !math.IsInf(f, 0) {
		return nan()
	}
	return f
}

// ToIntegerOrInfinity truncates a number toward zero; NaN becomes 0.
func ToIntegerOrInfinity(f float64) float64 {
	if math.IsNaN(f) {
		return 0
	}
	return math.Trunc(f)
}

// ToFixed is Number.prototype.toFixed: exact decimal rounding of the binary value, ties toward the larger
// magnitude.
func ToFixed(x float64, digits Value) (string, error) {
	fd, err := ToNumber(digits)
	if err != nil {
		return "", err
	}
	f := ToIntegerOrInfinity(fd)
	if f < 0 || f > 100 {
		return "", &Error{Name: "RangeError", Message: "toFixed() digits argument must be between 0 and 100"}
	}
	if math.IsNaN(x) {
		return "NaN", nil
	}
	if math.Abs(x) >= 1e21 || math.IsInf(x, 0) {
		return NumberToString(x), nil
	}
	sign := ""
	if x < 0 {
		sign = "-"
		x = -x
	}
	prec := int(f)
	r := new(big.Rat).SetFloat64(x)
	r.Mul(r, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(prec)), nil)))
	r.Add(r, big.NewRat(1, 2))
	n := new(big.Int).Quo(r.Num(), r.Denom())
	m := n.String()
	if prec == 0 {
		return sign + m, nil
	}
	if len(m) <= prec {
		m = strings.Repeat("0", prec+1-len(m)) + m
	}
	return sign + m[:len(m)-prec] + "." + m[len(m)-prec:], nil
}
