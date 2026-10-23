// Ported from Handlebars.js 4.7.10 (https://github.com/handlebars-lang/handlebars.js).
// Copyright (C) 2011-2019 by Yehuda Katz. Licensed under the MIT License; see LICENSE.

package js

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// The console of Node.js 22 formats logged values with util.formatWithOptions and util.inspect's defaults:
// depth 2, breakLength 80, compact 3, at most 100 array items.
const (
	inspectDepth     = 2
	breakLength      = 80
	compactLevels    = 3
	maxArrayLength   = 100
	minLineWidth     = 16
	inspectKeyPlain  = `^[a-zA-Z_][a-zA-Z_0-9]*$`
	groupSeparatorSp = 2
)

var plainKey = regexp.MustCompile(inspectKeyPlain)

type inspectCtx struct {
	indentation  int
	currentDepth int
	depth        int
	// showHidden adds non-enumerable properties (an array's [length]), as %o does.
	showHidden bool
}

// Format is util.format as console.log applies it to its arguments.
func Format(args []Value) string {
	if len(args) == 0 {
		return ""
	}
	first, isString := args[0].(string)
	if isString && len(args) == 1 {
		// A lone string is printed as it is: Node only interprets %% and the other
		// directives when there are arguments after the format string.
		return first
	}
	var b strings.Builder
	rest := args
	if isString {
		rest = args[1:]
		a := 0
		last := 0
		for i := 0; i < len(first)-1; i++ {
			if first[i] != '%' {
				continue
			}
			c := first[i+1]
			if c == '%' {
				b.WriteString(first[last:i])
				b.WriteByte('%')
				last = i + 2
				i++
				continue
			}
			if a >= len(rest) || !strings.ContainsRune("sdifjoOc", rune(c)) {
				continue
			}
			b.WriteString(first[last:i])
			arg := rest[a]
			a++
			switch c {
			case 's':
				switch x := arg.(type) {
				case float64:
					b.WriteString(formatNumber(x))
				case *Object, *Array:
					b.WriteString(inspectWith(arg, 0))
				default:
					s, err := ToString(x)
					if err != nil {
						s = inspectWith(arg, 0)
					}
					b.WriteString(s)
				}
			case 'd':
				n, err := ToNumber(arg)
				if err != nil {
					n = nan()
				}
				b.WriteString(formatNumber(n))
			case 'i':
				b.WriteString(formatNumber(parseIntLike(arg)))
			case 'f':
				b.WriteString(formatNumber(parseFloatLike(arg)))
			case 'j':
				s, ok, _ := Stringify(arg)
				if !ok {
					s = "undefined"
				}
				b.WriteString(s)
			case 'o':
				ctx := &inspectCtx{depth: 4, showHidden: true}
				b.WriteString(ctx.format(arg, 0, true))
			case 'O':
				b.WriteString(Inspect(arg))
			case 'c':
			}
			last = i + 2
			i++
		}
		b.WriteString(first[last:])
		rest = rest[a:]
		for _, v := range rest {
			b.WriteByte(' ')
			if s, ok := v.(string); ok {
				b.WriteString(s)
			} else {
				b.WriteString(Inspect(v))
			}
		}
		return b.String()
	}
	for i, v := range rest {
		if i > 0 {
			b.WriteByte(' ')
		}
		if s, ok := v.(string); ok {
			b.WriteString(s)
		} else {
			b.WriteString(Inspect(v))
		}
	}
	return b.String()
}

func parseIntLike(v Value) float64 {
	s, err := ToString(v)
	if err != nil {
		return nan()
	}
	s = strings.TrimLeftFunc(s, isJSWhitespace)
	m := regexp.MustCompile(`^[+-]?[0-9]+`).FindString(s)
	if m == "" {
		return nan()
	}
	f, _ := strconv.ParseFloat(m, 64)
	return f
}

func parseFloatLike(v Value) float64 {
	s, err := ToString(v)
	if err != nil {
		return nan()
	}
	s = strings.TrimLeftFunc(s, isJSWhitespace)
	if strings.HasPrefix(s, "Infinity") || strings.HasPrefix(s, "+Infinity") {
		return math.Inf(1)
	}
	if strings.HasPrefix(s, "-Infinity") {
		return math.Inf(-1)
	}
	m := regexp.MustCompile(`^[+-]?(?:[0-9]+\.?[0-9]*|\.[0-9]+)(?:[eE][+-]?[0-9]+)?`).FindString(s)
	if m == "" {
		return nan()
	}
	f, _ := strconv.ParseFloat(m, 64)
	return f
}

func formatNumber(f float64) string {
	if f == 0 && math.Signbit(f) {
		return "-0"
	}
	return NumberToString(f)
}

// Inspect is util.inspect with the console's options.
func Inspect(v Value) string {
	return inspectWith(v, inspectDepth)
}

func inspectWith(v Value, depth int) string {
	ctx := &inspectCtx{depth: depth}
	return ctx.format(v, 0, true)
}

func (ctx *inspectCtx) format(v Value, recurse int, top bool) string {
	switch x := v.(type) {
	case undefinedType, nil:
		return "undefined"
	case nullType:
		return "null"
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return formatNumber(x)
	case string:
		if Length(x) > minLineWidth && Length(x) > breakLength-ctx.indentation-4 {
			lines := splitNewLines(x)
			parts := make([]string, len(lines))
			for i, line := range lines {
				parts[i] = quoteInspect(line)
			}
			return strings.Join(parts, " +\n"+strings.Repeat(" ", ctx.indentation+2))
		}
		return quoteInspect(x)
	case *Inherited:
		if x.Function {
			return "[Function: " + x.Name + "]"
		}
		return "[Object: null prototype] {}"
	case *Func:
		name := x.Name
		if name == "" {
			return "[Function (anonymous)]"
		}
		return "[Function: " + name + "]"
	}
	return ctx.formatObject(v, recurse)
}

// splitNewLines splits after each newline, keeping it with its line.
func splitNewLines(s string) []string {
	var out []string
	for {
		i := strings.IndexByte(s, '\n')
		if i < 0 || i == len(s)-1 {
			break
		}
		out = append(out, s[:i+1])
		s = s[i+1:]
	}
	return append(out, s)
}

func (ctx *inspectCtx) formatObject(v Value, recurse int) string {
	var keys []string
	var get func(string) Value
	prefix := ""
	isArray := false
	var arr *Array
	switch x := v.(type) {
	case *Object:
		keys = x.Keys()
		get = x.Value
	case *SafeString:
		keys = []string{"string"}
		get = func(string) Value { return x.S }
		prefix = "SafeString "
	case *Array:
		isArray = true
		arr = x
	}
	if isArray && len(arr.Elems) == 0 {
		if ctx.showHidden {
			return "[ [length]: 0 ]"
		}
		return "[]"
	}
	if !isArray && len(keys) == 0 {
		return prefix + "{}"
	}
	if recurse > ctx.depth {
		if isArray {
			return "[Array]"
		}
		if prefix != "" {
			return "[" + strings.TrimSpace(prefix) + "]"
		}
		return "[Object]"
	}
	recurse++
	ctx.currentDepth = recurse
	var output []string
	ctx.indentation += 2
	if isArray {
		n := len(arr.Elems)
		if n > maxArrayLength {
			n = maxArrayLength
		}
		for i := 0; i < n; i++ {
			output = append(output, ctx.format(arr.Elems[i], recurse, false))
		}
		if len(arr.Elems) > maxArrayLength {
			more := len(arr.Elems) - maxArrayLength
			s := "... " + strconv.Itoa(more) + " more item"
			if more > 1 {
				s += "s"
			}
			output = append(output, s)
		}
		if ctx.showHidden {
			output = append(output, "[length]: "+formatNumber(float64(len(arr.Elems))))
		}
	} else {
		for _, k := range keys {
			name := k
			if !plainKey.MatchString(k) {
				name = quoteInspect(k)
			}
			output = append(output, name+": "+ctx.format(get(k), recurse, false))
		}
	}
	ctx.indentation -= 2
	open, close := "{", "}"
	if isArray {
		open, close = "[", "]"
	}
	open = prefix + open
	return ctx.reduce(output, open, close, isArray, recurse, arr)
}

func (ctx *inspectCtx) reduce(output []string, open, close string, isArray bool, recurse int, arr *Array) string {
	entries := len(output)
	if isArray && entries > 6 {
		output = ctx.groupArrayElements(output, arr)
	}
	if ctx.currentDepth-recurse < compactLevels && entries == len(output) {
		start := len(output) + ctx.indentation + len(open) + 10
		if ctx.belowBreakLength(output, start) {
			joined := strings.Join(output, ", ")
			if !strings.Contains(joined, "\n") {
				return open + " " + joined + " " + close
			}
		}
	}
	indentation := "\n" + strings.Repeat(" ", ctx.indentation)
	return open + indentation + "  " + strings.Join(output, ","+indentation+"  ") + indentation + close
}

func (ctx *inspectCtx) belowBreakLength(output []string, start int) bool {
	total := len(output) + start
	if total+len(output) > breakLength {
		return false
	}
	for _, s := range output {
		total += Length(s)
		if total > breakLength {
			return false
		}
	}
	return true
}

func (ctx *inspectCtx) groupArrayElements(output []string, arr *Array) []string {
	totalLength := 0
	maxLength := 0
	outputLength := len(output)
	// Node compares maxArrayLength with the number of output entries, hidden ones included.
	if maxArrayLength < len(output) {
		outputLength--
	}
	dataLen := make([]int, outputLength)
	for i := 0; i < outputLength; i++ {
		l := Length(output[i])
		dataLen[i] = l
		totalLength += l + groupSeparatorSp
		if maxLength < l {
			maxLength = l
		}
	}
	actualMax := maxLength + groupSeparatorSp
	if actualMax*3+ctx.indentation < breakLength &&
		(float64(totalLength)/float64(actualMax) > 5 || maxLength <= 6) {
		averageBias := math.Sqrt(float64(actualMax) - float64(totalLength)/float64(len(output)))
		biasedMax := math.Max(float64(actualMax)-3-averageBias, 1)
		columns := minInt(
			int(math.Round(math.Sqrt(2.5*biasedMax*float64(outputLength))/biasedMax)),
			(breakLength-ctx.indentation)/actualMax,
			compactLevels*4,
			15,
		)
		if columns <= 1 {
			return output
		}
		var maxLineLength []int
		for i := 0; i < columns; i++ {
			lineLength := 0
			for j := i; j < len(output); j += columns {
				if j < outputLength && dataLen[j] > lineLength {
					lineLength = dataLen[j]
				}
			}
			maxLineLength = append(maxLineLength, lineLength+groupSeparatorSp)
		}
		padStart := true
		if arr != nil {
			for i := 0; i < len(output); i++ {
				if i >= len(arr.Elems) {
					padStart = false
					break
				}
				if _, ok := arr.Elems[i].(float64); !ok {
					padStart = false
					break
				}
			}
		}
		var tmp []string
		for i := 0; i < outputLength; i += columns {
			max := minInt(i+columns, outputLength)
			var line strings.Builder
			j := i
			for ; j < max-1; j++ {
				cell := output[j] + ", "
				line.WriteString(pad(cell, maxLineLength[j-i], padStart))
			}
			if padStart {
				line.WriteString(pad(output[j], maxLineLength[j-i]-groupSeparatorSp, true))
			} else {
				line.WriteString(output[j])
			}
			tmp = append(tmp, line.String())
		}
		if outputLength < len(output) {
			tmp = append(tmp, output[outputLength])
		}
		return tmp
	}
	return output
}

func pad(s string, width int, start bool) string {
	n := width - Length(s)
	if n <= 0 {
		return s
	}
	if start {
		return strings.Repeat(" ", n) + s
	}
	return s + strings.Repeat(" ", n)
}

func minInt(values ...int) int {
	m := values[0]
	for _, v := range values[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

// quoteInspect quotes a string like util.inspect: single quotes unless the string contains one and no double
// quote (then double quotes), or contains both but no backtick (then backticks).
func quoteInspect(s string) string {
	quote := byte('\'')
	if strings.IndexByte(s, '\'') >= 0 {
		if strings.IndexByte(s, '"') < 0 {
			quote = '"'
		} else if strings.IndexByte(s, '`') < 0 && !strings.Contains(s, "${") {
			quote = '`'
		}
	}
	var b strings.Builder
	b.WriteByte(quote)
	units := ToUnits(s)
	for i := 0; i < len(units); i++ {
		u := units[i]
		switch {
		case u == uint16(quote):
			b.WriteByte('\\')
			b.WriteByte(quote)
		case u == '\\':
			b.WriteString(`\\`)
		case u == '\n':
			b.WriteString(`\n`)
		case u == '\t':
			b.WriteString(`\t`)
		case u == '\b':
			b.WriteString(`\b`)
		case u == '\f':
			b.WriteString(`\f`)
		case u == '\r':
			b.WriteString(`\r`)
		case u < 0x20 || u == 0x7F:
			b.WriteString(`\x`)
			b.WriteString(strings.ToUpper(hex2(u)))
		case u >= 0xD800 && u <= 0xDBFF && i+1 < len(units) && units[i+1] >= 0xDC00 && units[i+1] <= 0xDFFF:
			b.WriteString(FromUnits(units[i : i+2]))
			i++
		case u >= 0xD800 && u <= 0xDFFF:
			b.WriteString(`\u`)
			b.WriteString(strings.ToUpper(strconv.FormatUint(uint64(u), 16)))
		default:
			b.WriteString(FromUnits(units[i : i+1]))
		}
	}
	b.WriteByte(quote)
	return b.String()
}

func hex2(u uint16) string {
	return string([]byte{hexDigits[u>>4], hexDigits[u&15]})
}
