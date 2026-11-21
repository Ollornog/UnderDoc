// SPDX-License-Identifier: MIT

package sheet

import (
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Number formats of SpreadsheetML (ECMA-376, part 1, 18.8.30 and 18.8.31), rendered by UnderDoc in the reader's
// language (ADR-31). The format code in the file is always written with the invariant separators ("#,##0.00"); the
// language decides which characters show. Colours are left out, and a negative number that a format would mark only by
// colour keeps its minus sign, so nothing depends on colour.

// Lang is how numbers, dates and booleans read in a language.
type Lang struct {
	Code           string
	Decimal, Group string
	Months         [12]string
	MonthsShort    [12]string
	Days           [7]string // Sunday first
	DaysShort      [7]string
	ShortDate      string // the built-in date format 14
	ShortDateTime  string // the built-in format 22
	True, False    string
	// Currency is the code of the built-in currency formats 5 to 8.
	Currency [4]string
}

// English reads as Excel does in English (United States).
var English = Lang{
	Code: "en", Decimal: ".", Group: ",",
	Months:        [12]string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
	MonthsShort:   [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
	Days:          [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
	DaysShort:     [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
	ShortDate:     "m/d/yyyy",
	ShortDateTime: "m/d/yyyy h:mm",
	True:          "TRUE", False: "FALSE",
	Currency: [4]string{`"$"#,##0_);\("$"#,##0\)`, `"$"#,##0_);[Red]\("$"#,##0\)`, `"$"#,##0.00_);\("$"#,##0.00\)`,
		`"$"#,##0.00_);[Red]\("$"#,##0.00\)`},
}

// German reads as Excel does in German (Germany).
var German = Lang{
	Code: "de", Decimal: ",", Group: ".",
	Months:        [12]string{"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"},
	MonthsShort:   [12]string{"Jan", "Feb", "Mär", "Apr", "Mai", "Jun", "Jul", "Aug", "Sep", "Okt", "Nov", "Dez"},
	Days:          [7]string{"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"},
	DaysShort:     [7]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"},
	ShortDate:     "dd.mm.yyyy",
	ShortDateTime: "dd.mm.yyyy hh:mm",
	True:          "WAHR", False: "FALSCH",
	Currency: [4]string{`#,##0 "€";\-#,##0 "€"`, `#,##0 "€";[Red]\-#,##0 "€"`, `#,##0.00 "€";\-#,##0.00 "€"`,
		`#,##0.00 "€";[Red]\-#,##0.00 "€"`},
}

// LangOf picks the language of a reader from the header Accept-Language: German when the first language is German,
// otherwise English.
func LangOf(acceptLanguage string) Lang {
	first, _, _ := strings.Cut(acceptLanguage, ",")
	first, _, _ = strings.Cut(strings.TrimSpace(first), ";")
	if tag := strings.ToLower(first); tag == "de" || strings.HasPrefix(tag, "de-") {
		return German
	}
	return English
}

// builtIn are the format codes of the built-in formats that do not depend on the language (ECMA-376, 18.8.30).
var builtIn = map[int]string{
	0: "General", 1: "0", 2: "0.00", 3: "#,##0", 4: "#,##0.00", 9: "0%", 10: "0.00%", 11: "0.00E+00", 12: "# ?/?",
	13: "# ??/??", 15: "d-mmm-yy", 16: "d-mmm", 17: "mmm-yy", 18: "h:mm AM/PM", 19: "h:mm:ss AM/PM", 20: "h:mm",
	21: "h:mm:ss", 37: "#,##0 ;(#,##0)", 38: "#,##0 ;[Red](#,##0)", 39: "#,##0.00;(#,##0.00)",
	40: "#,##0.00;[Red](#,##0.00)", 45: "mm:ss", 46: "[h]:mm:ss", 47: "mmss.0", 48: "##0.0E+0", 49: "@",
}

// BuiltInCode returns the code of a built-in format for a language, and false for an ID that is no built-in format.
func BuiltInCode(id int, l Lang) (string, bool) {
	switch {
	case id == 14:
		return l.ShortDate, true
	case id == 22:
		return l.ShortDateTime, true
	case id >= 5 && id <= 8:
		return l.Currency[id-5], true
	}
	code, ok := builtIn[id]
	return code, ok
}

// --- Parsing a code into sections of tokens

type tokenKind byte

const (
	tLit tokenKind = iota
	tZero
	tHash
	tQuestion
	tPoint
	tComma
	tPercent
	tExp
	tSlash
	tAt
	tGeneral
	tYear
	tMonth
	tDay
	tHour
	tMinute
	tSecond
	tSubSecond
	tAMPM
	tElapsedH
	tElapsedM
	tElapsedS
)

type token struct {
	kind tokenKind
	text string // the literal, the sign of an exponent, the written form of AM/PM
	n    int    // how many letters of a date part, digits of a fraction of a second
}

type section struct {
	tokens  []token
	cond    string // "<", "<=", ">", ">=", "=", "<>"; "" without a condition
	operand float64
	date    bool
	text    bool // holds @
	colour  bool
}

func (s section) matches(v float64) bool {
	switch s.cond {
	case "<":
		return v < s.operand
	case "<=":
		return v <= s.operand
	case ">":
		return v > s.operand
	case ">=":
		return v >= s.operand
	case "=":
		return v == s.operand
	case "<>":
		return v != s.operand
	}
	return true
}

// splitSections cuts a code at its semicolons, outside quotes, brackets and escapes.
func splitSections(code string) []string {
	var out []string
	start, quoted, bracket := 0, false, false
	for i := 0; i < len(code); i++ {
		switch c := code[i]; {
		case c == '"' && !bracket:
			quoted = !quoted
		case quoted:
		case c == '\\':
			i++
		case c == '[':
			bracket = true
		case c == ']':
			bracket = false
		case c == ';' && !bracket:
			out = append(out, code[start:i])
			start = i + 1
		}
	}
	return append(out, code[start:])
}

func parseSection(code string) section {
	var s section
	lit := func(text string) {
		if n := len(s.tokens); n > 0 && s.tokens[n-1].kind == tLit {
			s.tokens[n-1].text += text
			return
		}
		s.tokens = append(s.tokens, token{kind: tLit, text: text})
	}
	run := func(i int, lower byte) int {
		n := 0
		for i+n < len(code) && (code[i+n]|0x20) == lower {
			n++
		}
		return n
	}
	for i := 0; i < len(code); {
		c := code[i]
		switch {
		case c == '"':
			end := strings.IndexByte(code[i+1:], '"')
			if end < 0 {
				lit(code[i+1:])
				i = len(code)
				continue
			}
			lit(code[i+1 : i+1+end])
			i += end + 2
		case c == '\\' && i+1 < len(code):
			_, size := utf8.DecodeRuneInString(code[i+1:])
			lit(code[i+1 : i+1+size])
			i += 1 + size
		case c == '_' && i+1 < len(code):
			_, size := utf8.DecodeRuneInString(code[i+1:])
			lit(" ")
			i += 1 + size
		case c == '*' && i+1 < len(code):
			_, size := utf8.DecodeRuneInString(code[i+1:])
			i += 1 + size
		case c == '[':
			end := strings.IndexByte(code[i:], ']')
			if end < 0 {
				i = len(code)
				continue
			}
			s.bracket(code[i+1:i+end], lit)
			i += end + 1
		case c == '0':
			s.tokens = append(s.tokens, token{kind: tZero})
			i++
		case c == '#':
			s.tokens = append(s.tokens, token{kind: tHash})
			i++
		case c == '?':
			s.tokens = append(s.tokens, token{kind: tQuestion})
			i++
		case c == '.':
			s.tokens = append(s.tokens, token{kind: tPoint})
			i++
		case c == ',':
			s.tokens = append(s.tokens, token{kind: tComma})
			i++
		case c == '%':
			s.tokens = append(s.tokens, token{kind: tPercent})
			i++
		case (c == 'E' || c == 'e') && i+1 < len(code) && (code[i+1] == '+' || code[i+1] == '-'):
			s.tokens = append(s.tokens, token{kind: tExp, text: code[i+1 : i+2]})
			i += 2
		case c == '/':
			s.tokens = append(s.tokens, token{kind: tSlash})
			i++
		case c == '@':
			s.tokens = append(s.tokens, token{kind: tAt})
			s.text = true
			i++
		case (c == 'G' || c == 'g') && strings.EqualFold(safeSlice(code, i, i+7), "general"):
			s.tokens = append(s.tokens, token{kind: tGeneral})
			i += 7
		case (c == 'A' || c == 'a') && strings.EqualFold(safeSlice(code, i, i+5), "am/pm"):
			s.tokens = append(s.tokens, token{kind: tAMPM, text: code[i : i+5]})
			s.date = true
			i += 5
		case (c == 'A' || c == 'a') && strings.EqualFold(safeSlice(code, i, i+3), "a/p"):
			s.tokens = append(s.tokens, token{kind: tAMPM, text: code[i : i+3]})
			s.date = true
			i += 3
		case c|0x20 == 'y' || (c|0x20 == 'e' && s.dateLike()):
			n := run(i, c|0x20)
			s.tokens = append(s.tokens, token{kind: tYear, n: n})
			s.date = true
			i += n
		case c|0x20 == 'm':
			n := run(i, 'm')
			s.tokens = append(s.tokens, token{kind: tMonth, n: n})
			s.date = true
			i += n
		case c|0x20 == 'd':
			n := run(i, 'd')
			s.tokens = append(s.tokens, token{kind: tDay, n: n})
			s.date = true
			i += n
		case c|0x20 == 'h':
			n := run(i, 'h')
			s.tokens = append(s.tokens, token{kind: tHour, n: n})
			s.date = true
			i += n
		case c|0x20 == 's':
			n := run(i, 's')
			s.tokens = append(s.tokens, token{kind: tSecond, n: n})
			s.date = true
			i += n
		default:
			r, size := utf8.DecodeRuneInString(code[i:])
			if r == utf8.RuneError && size <= 1 {
				size = 1
			} else {
				lit(string(r))
			}
			i += size
		}
	}
	if s.date {
		s.resolveDate()
	}
	return s
}

func safeSlice(s string, from, to int) string {
	if to > len(s) {
		return ""
	}
	return s[from:to]
}

func (s *section) dateLike() bool {
	for _, t := range s.tokens {
		if t.kind >= tYear {
			return true
		}
	}
	return false
}

// bracket reads what stands in square brackets: a colour, a condition, a currency with its locale, or elapsed time.
func (s *section) bracket(inner string, lit func(string)) {
	lower := strings.ToLower(inner)
	switch {
	case inner == "":
	case inner[0] == '$':
		symbol, _, _ := strings.Cut(inner[1:], "-")
		if symbol != "" {
			lit(symbol)
		}
	case strings.Trim(lower, "h") == "":
		s.tokens = append(s.tokens, token{kind: tElapsedH, n: len(inner)})
		s.date = true
	case strings.Trim(lower, "m") == "":
		s.tokens = append(s.tokens, token{kind: tElapsedM, n: len(inner)})
		s.date = true
	case strings.Trim(lower, "s") == "":
		s.tokens = append(s.tokens, token{kind: tElapsedS, n: len(inner)})
		s.date = true
	case strings.ContainsAny(inner[:1], "<>="):
		op := inner[:1]
		if len(inner) > 1 && strings.ContainsAny(inner[1:2], "<>=") {
			op = inner[:2]
		}
		if v, err := strconv.ParseFloat(strings.TrimSpace(inner[len(op):]), 64); err == nil {
			s.cond, s.operand = op, v
		}
	default:
		// A colour ([Red], [Color10]) or something unknown, such as a calendar: left out.
		s.colour = true
	}
}

// resolveDate tells minutes from months: an m right after an hour, or right before a second, is a minute. A dot with
// zeros after a second is a fraction of the second.
func (s *section) resolveDate() {
	prev := func(i int) tokenKind {
		for j := i - 1; j >= 0; j-- {
			if k := s.tokens[j].kind; k != tLit {
				return k
			}
		}
		return tLit
	}
	next := func(i int) tokenKind {
		for j := i + 1; j < len(s.tokens); j++ {
			if k := s.tokens[j].kind; k != tLit {
				return k
			}
		}
		return tLit
	}
	for i, t := range s.tokens {
		if t.kind == tMonth && t.n <= 2 {
			if p := prev(i); p == tHour || p == tElapsedH || next(i) == tSecond || next(i) == tElapsedS {
				s.tokens[i].kind = tMinute
			}
		}
	}
	var out []token
	for i := 0; i < len(s.tokens); i++ {
		t := s.tokens[i]
		if t.kind == tPoint && len(out) > 0 && (out[len(out)-1].kind == tSecond || out[len(out)-1].kind == tElapsedS) {
			n := 0
			for i+1 < len(s.tokens) && s.tokens[i+1].kind == tZero {
				n++
				i++
			}
			if n > 0 {
				out = append(out, token{kind: tSubSecond, n: min(n, 3)})
				continue
			}
		}
		out = append(out, t)
	}
	s.tokens = out
}

// --- Formatting a number

// FormatNumber renders a number with a format code in a language. date1904 is the date system of the workbook.
func FormatNumber(v float64, code string, l Lang, date1904 bool) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "#NUM!"
	}
	if strings.TrimSpace(code) == "" || strings.EqualFold(code, "general") {
		return general(v, l)
	}
	raw := splitSections(code)
	sections := make([]section, 0, len(raw))
	for _, r := range raw {
		sections = append(sections, parseSection(r))
	}
	s, abs := pick(sections, v)
	if abs {
		v = math.Abs(v)
	}
	if s.date {
		return formatDate(v, s, l, date1904)
	}
	return formatPlain(v, s, l)
}

// pick chooses the section for a number, and tells whether the section shows its absolute value (it carries its own
// sign, as the second section of "0;(0)" does).
func pick(sections []section, v float64) (section, bool) {
	numeric := sections
	if n := len(numeric); n == 4 || (n > 1 && numeric[n-1].text) {
		numeric = numeric[:n-1]
	}
	if len(numeric) == 0 {
		return section{tokens: []token{{kind: tGeneral}}}, false
	}
	conditional := false
	for _, s := range numeric {
		if s.cond != "" {
			conditional = true
		}
	}
	if conditional {
		for i, s := range numeric {
			if s.cond != "" && s.matches(v) {
				// A section for negatives, such as [<0], shows the number without its sign.
				return s, i > 0 || (s.cond != "" && s.operand <= 0 && strings.HasPrefix(s.cond, "<"))
			}
			if s.cond == "" {
				return s, v < 0 && i > 0
			}
		}
		return numeric[len(numeric)-1], false
	}
	switch {
	case v > 0 || len(numeric) == 1:
		return numeric[0], false
	case v < 0:
		s := numeric[1]
		// Nothing depends on colour: a section for negatives that would show the number only in colour keeps its sign.
		if s.colour && !hasSign(s) {
			return s, false
		}
		return s, true
	case len(numeric) >= 3:
		return numeric[2], false
	}
	return numeric[0], false
}

// hasSign tells whether a section marks a negative number itself: with a minus or brackets in a literal.
func hasSign(s section) bool {
	for _, t := range s.tokens {
		if t.kind == tLit && strings.ContainsAny(t.text, "-()") {
			return true
		}
	}
	return false
}

// general renders as Excel's General: at most eleven characters, scientific beyond.
func general(v float64, l Lang) string {
	if v == 0 {
		return "0"
	}
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	if v >= 1e11 || v < 1e-9 {
		return sign + scientificGeneral(v, l)
	}
	whole, _ := fixed(v, 0)
	intLen := len(whole)
	if whole == "0" {
		intLen = 1
	}
	decimals := max(10-intLen, 0)
	i, f := fixed(v, decimals)
	f = strings.TrimRight(f, "0")
	if i == "0" && f == "" {
		return sign + scientificGeneral(v, l)
	}
	if f == "" {
		return sign + i
	}
	return sign + i + l.Decimal + f
}

func scientificGeneral(v float64, l Lang) string {
	e := int(math.Floor(math.Log10(v)))
	m := v / math.Pow(10, float64(e))
	i, f := fixed(m, 5)
	if i == "10" {
		e++
		i, f = fixed(v/math.Pow(10, float64(e)), 5)
	}
	f = strings.TrimRight(f, "0")
	out := i
	if f != "" {
		out += l.Decimal + f
	}
	sign := "+"
	if e < 0 {
		sign, e = "-", -e
	}
	return out + "E" + sign + pad(strconv.Itoa(e), 2)
}

func pad(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return strings.Repeat("0", n-len(s)) + s
}

// fixed returns the decimal digits of |v|, rounded first to 15 significant digits as Excel keeps them and then half away
// from zero to n decimals: the whole part and n digits of the fraction.
func fixed(v float64, n int) (string, string) {
	v = math.Abs(v)
	if v == 0 {
		return "0", strings.Repeat("0", n)
	}
	e := strconv.FormatFloat(v, 'e', 14, 64) // d.dddddddddddddde±XX
	mantissa, exp, _ := strings.Cut(e, "e")
	digits := strings.Replace(mantissa, ".", "", 1)
	x, _ := strconv.Atoi(exp)
	point := x + 1 // digits before the decimal point
	if point <= 0 {
		digits = strings.Repeat("0", 1-point) + digits
		point = 1
	}
	if len(digits) < point+n+1 {
		digits += strings.Repeat("0", point+n+1-len(digits))
	}
	keep := []byte(digits[:point+n])
	if digits[point+n] >= '5' {
		i := len(keep) - 1
		for ; i >= 0; i-- {
			if keep[i] == '9' {
				keep[i] = '0'
				continue
			}
			keep[i]++
			break
		}
		if i < 0 {
			keep = append([]byte{'1'}, keep...)
			point++
		}
	}
	whole := strings.TrimLeft(string(keep[:point]), "0")
	if whole == "" {
		whole = "0"
	}
	return whole, string(keep[point:])
}

// formatPlain renders a number with a section that is no date: digits, fractions, percent, scientific, literals.
func formatPlain(v float64, s section, l Lang) string {
	negative := v < 0
	v = math.Abs(v)
	// The structure of the section.
	var intPH, fracPH, expPH []token
	afterPoint, afterExp, thousands, hasGeneral := false, false, false, false
	percent, scale := 0, 0
	slash := -1
	for i, t := range s.tokens {
		switch t.kind {
		case tZero, tHash, tQuestion:
			switch {
			case afterExp:
				expPH = append(expPH, t)
			case afterPoint:
				fracPH = append(fracPH, t)
			default:
				intPH = append(intPH, t)
			}
		case tPoint:
			if !afterExp {
				afterPoint = true
			}
		case tComma:
			switch {
			case afterExp:
			case !afterPoint && nextDigit(s.tokens, i) && len(intPH) > 0:
				thousands = true
			case !nextDigit(s.tokens, i) && len(intPH)+len(fracPH) > 0:
				scale++ // a comma after the digits divides by a thousand
			}
		case tPercent:
			percent++
		case tExp:
			afterExp = true
		case tSlash:
			slash = i
		case tGeneral:
			hasGeneral = true
		}
	}
	for range percent {
		v *= 100
	}
	for range scale {
		v /= 1000
	}
	sign := ""
	if negative {
		sign = "-"
	}
	if hasGeneral {
		return sign + replaceTokens(s, func(t token) string {
			if t.kind == tGeneral {
				return general(v, l)
			}
			return ""
		})
	}
	if slash >= 0 && !afterExp {
		return sign + formatFraction(v, s, slash, l)
	}
	if afterExp {
		return sign + formatScientific(v, s, intPH, fracPH, expPH, l)
	}
	whole, frac := fixed(v, len(fracPH))
	if sign != "" && strings.Trim(whole+frac, "0") == "" {
		sign = "" // what rounds to zero shows no minus
	}
	intText := placeInteger(whole, intPH, thousands, l)
	fracText := placeFraction(frac, fracPH)
	var out strings.Builder
	out.WriteString(sign)
	ip, fp := 0, 0
	for _, t := range s.tokens {
		switch t.kind {
		case tLit:
			out.WriteString(t.text)
		case tPercent:
			out.WriteString("%")
		case tPoint:
			if !afterExp {
				out.WriteString(l.Decimal)
			}
		case tZero, tHash, tQuestion:
			if fp < len(fracPH) && ip >= len(intPH) {
				out.WriteString(fracText[fp])
				fp++
			} else if ip < len(intPH) {
				out.WriteString(intText[ip])
				ip++
			}
		}
	}
	return out.String()
}

// replaceTokens writes a section with literals as they are and other tokens through fn.
func replaceTokens(s section, fn func(token) string) string {
	var out strings.Builder
	for _, t := range s.tokens {
		if t.kind == tLit {
			out.WriteString(t.text)
			continue
		}
		out.WriteString(fn(t))
	}
	return out.String()
}

func nextDigit(tokens []token, i int) bool {
	for j := i + 1; j < len(tokens); j++ {
		switch tokens[j].kind {
		case tZero, tHash, tQuestion:
			return true
		case tComma:
			continue
		default:
			return false
		}
	}
	return false
}

// placeInteger gives each placeholder of the whole part its text: digits from the right, the leftmost placeholder takes
// every digit that is left, zeros where a 0 stands, spaces where a ? stands. With thousands, the first placeholder takes
// the whole grouped number.
func placeInteger(whole string, ph []token, thousands bool, l Lang) []string {
	out := make([]string, len(ph))
	if len(ph) == 0 {
		return out
	}
	if whole == "0" {
		whole = ""
	}
	if thousands {
		minDigits := 0
		for i, t := range ph {
			if t.kind != tHash {
				minDigits = len(ph) - i
				break
			}
		}
		digits := whole
		if len(digits) < minDigits {
			digits = strings.Repeat("0", minDigits-len(digits)) + digits
		}
		out[0] = group(digits, l.Group)
		return out
	}
	d := len(whole) - 1
	for i := len(ph) - 1; i >= 0; i-- {
		switch {
		case d >= 0 && i == 0:
			out[i] = whole[:d+1]
		case d >= 0:
			out[i] = whole[d : d+1]
		case ph[i].kind == tZero:
			out[i] = "0"
		case ph[i].kind == tQuestion:
			out[i] = " "
		}
		d--
	}
	return out
}

func group(digits, sep string) string {
	if len(digits) <= 3 {
		return digits
	}
	var out strings.Builder
	first := len(digits) % 3
	if first > 0 {
		out.WriteString(digits[:first])
	}
	for i := first; i < len(digits); i += 3 {
		if out.Len() > 0 {
			out.WriteString(sep)
		}
		out.WriteString(digits[i : i+3])
	}
	return out.String()
}

// placeFraction gives each placeholder after the point its digit; trailing zeros go where # or ? stands.
func placeFraction(frac string, ph []token) []string {
	out := make([]string, len(ph))
	trailing := true
	for i := len(ph) - 1; i >= 0; i-- {
		d := frac[i : i+1]
		if trailing && d == "0" && ph[i].kind != tZero {
			if ph[i].kind == tQuestion {
				out[i] = " "
			}
			continue
		}
		trailing = false
		out[i] = d
	}
	return out
}

// formatScientific renders 0.00E+00 and the engineering form ##0.0E+0.
func formatScientific(v float64, s section, intPH, fracPH, expPH []token, l Lang) string {
	k := max(len(intPH), 1)
	e := 0
	if v != 0 {
		e = int(math.Floor(math.Log10(v)))
		if k > 1 && intPH[0].kind == tHash {
			e = int(math.Floor(float64(e)/float64(k))) * k
		} else {
			e -= countKind(intPH, tZero) - 1
			if countKind(intPH, tZero) == 0 {
				e = int(math.Floor(math.Log10(v)))
			}
		}
	}
	m := v / math.Pow(10, float64(e))
	whole, frac := fixed(m, len(fracPH))
	if v != 0 && len(whole) > k {
		e += len(whole) - k
		whole, frac = fixed(v/math.Pow(10, float64(e)), len(fracPH))
	}
	intText := placeInteger(whole, intPH, false, l)
	fracText := placeFraction(frac, fracPH)
	expSign := ""
	var exp string
	ip, fp, afterExp := 0, 0, false
	var out strings.Builder
	for _, t := range s.tokens {
		switch t.kind {
		case tLit:
			out.WriteString(t.text)
		case tPercent:
			out.WriteString("%")
		case tPoint:
			if !afterExp {
				out.WriteString(l.Decimal)
			}
		case tExp:
			afterExp = true
			switch {
			case e < 0:
				expSign = "-"
			case t.text == "+":
				expSign = "+"
			}
			digits := strconv.Itoa(abs(e))
			exp = "E" + expSign + pad(digits, max(countKind(expPH, tZero), 1))
			out.WriteString(exp)
		case tZero, tHash, tQuestion:
			switch {
			case afterExp:
			case ip < len(intPH):
				out.WriteString(intText[ip])
				ip++
			case fp < len(fracPH):
				out.WriteString(fracText[fp])
				fp++
			}
		}
	}
	return out.String()
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func countKind(ts []token, k tokenKind) int {
	n := 0
	for _, t := range ts {
		if t.kind == k {
			n++
		}
	}
	return n
}

// formatFraction renders # ?/?, # ??/??, ?/8 and the like: an optional whole part, a numerator and a denominator that
// is either fixed or the best one with as many digits as the placeholders allow.
func formatFraction(v float64, s section, slash int, l Lang) string {
	// The placeholders before the slash: the numerator is the last run of them, a whole part the runs before it.
	var runs [][2]int // start, end (exclusive) of each run of digit placeholders before the slash
	for i := 0; i < slash; i++ {
		if isDigit(s.tokens[i].kind) {
			start := i
			for i < slash && isDigit(s.tokens[i].kind) {
				i++
			}
			runs = append(runs, [2]int{start, i})
		}
	}
	if len(runs) == 0 {
		return general(v, l)
	}
	num := runs[len(runs)-1]
	var whole *[2]int
	if len(runs) > 1 {
		whole = &runs[0]
	}
	// The denominator: digit placeholders, or a number written out, after the slash.
	denEnd := slash + 1
	fixedDen := 0
	var denPH []token
	for denEnd < len(s.tokens) && (isDigit(s.tokens[denEnd].kind) || (s.tokens[denEnd].kind == tLit && isNumber(s.tokens[denEnd].text))) {
		t := s.tokens[denEnd]
		if t.kind == tLit {
			fixedDen, _ = strconv.Atoi(strings.TrimSpace(t.text))
		} else {
			denPH = append(denPH, t)
		}
		denEnd++
	}
	var w, n, d int
	if whole != nil {
		w = int(math.Floor(v))
		v -= float64(w)
	}
	if fixedDen > 0 {
		d = fixedDen
		n = int(math.Round(v * float64(d)))
	} else {
		maxDen := int(math.Pow(10, float64(min(max(len(denPH), 1), 9)))) - 1
		n, d = bestFraction(v, maxDen)
	}
	if whole != nil && n == d && d != 0 {
		w++
		n = 0
	}
	var out strings.Builder
	for i := 0; i < len(s.tokens); i++ {
		t := s.tokens[i]
		switch {
		case whole != nil && i == whole[0]:
			if w != 0 || n == 0 || hasZero(s.tokens[whole[0]:whole[1]]) {
				out.WriteString(strconv.Itoa(w))
			}
			i = whole[1] - 1
		case i == num[0]:
			if whole != nil && n == 0 && w != 0 {
				// A whole number shows no fraction, only the space it would take.
				width := (num[1] - num[0]) + 1 + max(len(denPH), len(strconv.Itoa(fixedDen)))
				out.WriteString(strings.Repeat(" ", width))
				i = denEnd - 1
				continue
			}
			out.WriteString(padLeft(strconv.Itoa(n), countKind(s.tokens[num[0]:num[1]], tQuestion)+countKind(s.tokens[num[0]:num[1]], tZero)))
			out.WriteString("/")
			if fixedDen > 0 {
				out.WriteString(strconv.Itoa(fixedDen))
			} else {
				out.WriteString(padRight(strconv.Itoa(d), countKind(denPH, tQuestion)+countKind(denPH, tZero)))
			}
			i = denEnd - 1
		case t.kind == tLit:
			out.WriteString(t.text)
		}
	}
	return strings.TrimRight(out.String(), " ")
}

func hasZero(ts []token) bool { return countKind(ts, tZero) > 0 }

func isDigit(k tokenKind) bool { return k == tZero || k == tHash || k == tQuestion }

func isNumber(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func padLeft(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return strings.Repeat(" ", n-len(s)) + s
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

// bestFraction finds the fraction closest to v >= 0 with a denominator up to maxDen, through its continued fraction and
// the semiconvergents of its last step, so its cost does not grow with maxDen.
func bestFraction(v float64, maxDen int) (int, int) {
	if v > 1e15 {
		return int(math.Round(v)), 1
	}
	p0, q0, p1, q1 := 0, 1, 1, 0 // the convergents before the current one
	x := v
	for range 64 {
		a := math.Floor(x)
		if a > 1e15 {
			break
		}
		ai := int(a)
		p2, q2 := ai*p1+p0, ai*q1+q0
		if q2 > maxDen {
			// The best semiconvergent within the bound, if it beats the last convergent.
			k := (maxDen - q0) / q1
			ps, qs := k*p1+p0, k*q1+q0
			if qs > 0 && math.Abs(v-float64(ps)/float64(qs)) < math.Abs(v-float64(p1)/float64(q1)) {
				return ps, qs
			}
			break
		}
		p0, q0, p1, q1 = p1, q1, p2, q2
		if x-a < 1e-12 {
			break
		}
		x = 1 / (x - a)
	}
	if q1 == 0 {
		return int(math.Round(v)), 1
	}
	return p1, q1
}

// --- Dates and times

// serialDate turns the whole days of a serial number into a date: 1900 with its leap day of 1900 (serial 60), or 1904.
func serialDate(days int, date1904 bool) (year int, month time.Month, day int, weekday time.Weekday) {
	if date1904 {
		t := time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, days)
		return t.Year(), t.Month(), t.Day(), t.Weekday()
	}
	switch {
	case days == 0:
		return 1900, time.January, 0, time.Saturday
	case days == 60:
		return 1900, time.February, 29, time.Wednesday
	case days < 60:
		t := time.Date(1899, 12, 31, 0, 0, 0, 0, time.UTC).AddDate(0, 0, days)
		return t.Year(), t.Month(), t.Day(), time.Weekday((days + 6) % 7)
	}
	t := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, days)
	return t.Year(), t.Month(), t.Day(), t.Weekday()
}

// formatDate renders a serial number with a date or time section.
func formatDate(v float64, s section, l Lang, date1904 bool) string {
	if v < 0 || v > 2958465.99999999 { // after 31 Dec 9999 Excel shows only hashes too
		return "#######"
	}
	sub := 0
	ampm := false
	for _, t := range s.tokens {
		if t.kind == tSubSecond {
			sub = max(sub, t.n)
		}
		if t.kind == tAMPM {
			ampm = true
		}
	}
	unit := math.Pow(10, float64(sub))
	total := int64(math.Round(v * 86400 * unit)) // in units of the shown precision
	perDay := int64(86400 * unit)
	days := int(total / perDay)
	rest := total % perDay
	secondsOfDay := rest / int64(unit)
	fraction := rest % int64(unit)
	hour, minute, second := int(secondsOfDay/3600), int(secondsOfDay/60%60), int(secondsOfDay%60)
	year, month, day, weekday := serialDate(days, date1904)
	totalSeconds := total / int64(unit)
	var out strings.Builder
	for _, t := range s.tokens {
		switch t.kind {
		case tLit:
			out.WriteString(t.text)
		case tYear:
			if t.n <= 2 {
				out.WriteString(pad(strconv.Itoa(year%100), 2))
			} else {
				out.WriteString(strconv.Itoa(year))
			}
		case tMonth:
			switch {
			case t.n == 1:
				out.WriteString(strconv.Itoa(int(month)))
			case t.n == 2:
				out.WriteString(pad(strconv.Itoa(int(month)), 2))
			case t.n == 3:
				out.WriteString(l.MonthsShort[month-1])
			case t.n == 5:
				r, _ := utf8.DecodeRuneInString(l.Months[month-1])
				out.WriteRune(r)
			default:
				out.WriteString(l.Months[month-1])
			}
		case tDay:
			switch {
			case t.n == 1:
				out.WriteString(strconv.Itoa(day))
			case t.n == 2:
				out.WriteString(pad(strconv.Itoa(day), 2))
			case t.n == 3:
				out.WriteString(l.DaysShort[weekday])
			default:
				out.WriteString(l.Days[weekday])
			}
		case tHour:
			h := hour
			if ampm {
				h = hour % 12
				if h == 0 {
					h = 12
				}
			}
			out.WriteString(padN(h, t.n))
		case tMinute:
			out.WriteString(padN(minute, t.n))
		case tSecond:
			out.WriteString(padN(second, t.n))
		case tSubSecond:
			digits := pad(strconv.FormatInt(fraction, 10), sub)
			out.WriteString(l.Decimal + digits[:t.n])
		case tAMPM:
			out.WriteString(ampmText(t.text, hour))
		case tElapsedH:
			out.WriteString(padN(int(totalSeconds/3600), t.n))
		case tElapsedM:
			out.WriteString(padN(int(totalSeconds/60), t.n))
		case tElapsedS:
			out.WriteString(padN(int(totalSeconds), t.n))
		case tPoint:
			out.WriteString(".")
		case tZero:
			out.WriteString("0")
		case tPercent:
			out.WriteString("%")
		case tSlash:
			out.WriteString("/")
		case tComma:
			out.WriteString(",")
		}
	}
	return out.String()
}

func padN(v, n int) string {
	if n >= 2 {
		return pad(strconv.Itoa(v), 2)
	}
	return strconv.Itoa(v)
}

func ampmText(written string, hour int) string {
	am := hour < 12
	if len(written) == 3 { // A/P
		if am {
			return written[:1]
		}
		return written[2:3]
	}
	text := "PM"
	if am {
		text = "AM"
	}
	if written[0] >= 'a' {
		return strings.ToLower(text)
	}
	return text
}

// FormatText renders a text with a format code: its text section (the fourth, or one that holds @), or the text as it is.
func FormatText(text, code string) string {
	if !strings.Contains(code, "@") {
		return text
	}
	raw := splitSections(code)
	var s section
	found := false
	for i, r := range raw {
		p := parseSection(r)
		if p.text || i == 3 {
			s, found = p, true
		}
	}
	if !found {
		return text
	}
	return replaceTokens(s, func(t token) string {
		if t.kind == tAt {
			return text
		}
		return ""
	})
}
