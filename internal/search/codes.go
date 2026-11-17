// SPDX-License-Identifier: MIT

package search

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// Codes are tokens such as E-1042, 0x80070005, ORA-00942, HTTP 503, KB5034441, CVE-2024-3094 or E_ACCESSDENIED. They are
// found as written, never through a typing error (ADR-24): every one goes into fields of its own, in a canonical form, and
// a query that contains one looks it up there exactly.
//
// The canonical form is lower case without separators, so "ORA-00942", "ora00942" and "Ora 00942" are the same code. Each
// code also brings its parts with a digit in them, so "00942" and "942" find ORA-00942 as well, but only after the pages
// that carry the whole code.

const (
	minCodeLength = 3
	maxCodeLength = 64
)

// candidate is a run of letters and digits, joined by the separators codes use.
var candidate = regexp.MustCompile(`[\p{L}\p{N}]+(?:[-_.:/][\p{L}\p{N}]+)*`)

// pairWords may stand before a number with a space and form one code with it: "HTTP 503", "Port 443", "Event ID 4625".
// A word in capitals always may.
var pairWords = map[string]bool{
	"http": true, "https": true, "port": true, "error": true, "fehler": true, "fehlercode": true, "code": true,
	"status": true, "event": true, "ereignis": true, "id": true, "sqlstate": true, "kb": true, "rfc": true, "iso": true,
	"din": true, "en": true, "cve": true, "ticket": true, "ora": true, "smtp": true, "exit": true,
}

// Token is one code as it stands in a text, with its forms.
type Token struct {
	// Text is the code as written.
	Text string
	// Code is the canonical form.
	Code string
	// Parts are the pieces with a digit, canonical too, also without leading zeros.
	Parts []string
	// Start and End are the byte offsets of the code in the text it was found in.
	Start, End int
}

// FindCodes returns the codes of a text in their order. A code that occurs twice comes twice.
func FindCodes(text string) []Token {
	spans := candidate.FindAllStringIndex(text, -1)
	var tokens []Token
	for i := 0; i < len(spans); i++ {
		start, end := spans[i][0], spans[i][1]
		word := text[start:end]
		// A pair: a word that may stand before a number, one space or tab, then the number.
		if i+1 < len(spans) {
			next := text[spans[i+1][0]:spans[i+1][1]]
			gap := text[end:spans[i+1][0]]
			if stem, at, ok := pairStem(word); ok && (gap == " " || gap == "\t" || gap == "\u00a0") && isNumber(next) {
				// What stands before the part that pairs may be a code of its own.
				if head := strings.TrimRight(word[:at], "-_.:/"); isCode(head) {
					tokens = append(tokens, makeToken(head, start, start+len(head)))
				}
				token := makeToken(stem+" "+next, start+at, spans[i+1][1])
				token.Text = text[start+at : spans[i+1][1]]
				tokens = append(tokens, token)
				i++
				continue
			}
		}
		if isCode(word) {
			tokens = append(tokens, makeToken(word, start, end))
		}
	}
	return tokens
}

// CodeForms returns every canonical form the codes of a text bring, each once: the codes and their parts.
func CodeForms(text string) []string {
	var forms []string
	for _, token := range FindCodes(text) {
		forms = appendNew(forms, token.Code)
		for _, part := range token.Parts {
			forms = appendNew(forms, part)
		}
	}
	return forms
}

func appendNew(list []string, s string) []string {
	if s == "" || slices.Contains(list, s) {
		return list
	}
	return append(list, s)
}

// pairSuffixes end German compounds that name a code: Druckerfehler 79, Statuscode 404.
var pairSuffixes = []string{"fehlercode", "fehler", "code", "error", "status"}

// pairStem tells whether a word may stand before a number as one code with it, and what of it belongs to the code: the
// whole word ("HTTP 503"), the last part after a separator ("VPN-Fehler 809" is "Fehler 809") or the end of a compound
// ("Druckerfehler 79" is "Fehler 79"). at is where that part starts in the word.
func pairStem(word string) (stem string, at int, ok bool) {
	if i := strings.LastIndexAny(word, "-_.:/"); i >= 0 {
		stem, at, ok = pairStem(word[i+1:])
		return stem, i + 1 + at, ok
	}
	if len(word) < 2 || len(word) > 24 {
		return "", 0, false
	}
	upper := true
	for _, r := range word {
		if !unicode.IsLetter(r) {
			return "", 0, false
		}
		if !unicode.IsUpper(r) {
			upper = false
		}
	}
	lower := strings.ToLower(word)
	if (upper && len(word) <= 12) || pairWords[lower] {
		return lower, 0, true
	}
	for _, suffix := range pairSuffixes {
		if len(lower) > len(suffix) && strings.HasSuffix(lower, suffix) {
			return suffix, len(word) - len(suffix), true
		}
	}
	return "", 0, false
}

func isNumber(word string) bool {
	if len(word) < 2 {
		return false
	}
	for _, r := range word {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isCode tells a code from a word: it has a digit and is at least three long (a bare number too), or it joins letters
// with an underscore as identifiers do (E_ACCESSDENIED, max_connections).
func isCode(word string) bool {
	if len(word) < minCodeLength || len(word) > maxCodeLength {
		return false
	}
	digits, letters := 0, 0
	for _, r := range word {
		switch {
		case unicode.IsDigit(r):
			digits++
		case unicode.IsLetter(r):
			letters++
		}
	}
	if digits > 0 {
		return digits+letters >= minCodeLength
	}
	return letters > 0 && strings.Contains(strings.Trim(word, "_"), "_")
}

func makeToken(text string, start, end int) Token {
	token := Token{Text: text, Code: canonical(text), Start: start, End: end}
	for _, part := range parts(text) {
		if part != token.Code {
			token.Parts = appendNew(token.Parts, part)
		}
	}
	// ORA-00942 is also written ORA-942.
	if stripped := withoutLeadingZeros(text); stripped != token.Code {
		token.Parts = appendNew(token.Parts, stripped)
	}
	return token
}

// canonical lowers a code and drops its separators and spaces.
func canonical(code string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(code) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// parts splits a code at separators and where letters turn into digits, and keeps the pieces with a digit that are long
// enough to mean something: KB5034441 gives 5034441, 0x80070005 gives 80070005, ORA-00942 gives 00942 and 942.
func parts(code string) []string {
	lower := strings.ToLower(code)
	var out []string
	add := func(piece string) {
		if len(piece) < minCodeLength || !strings.ContainsAny(piece, "0123456789") {
			return
		}
		out = appendNew(out, piece)
		if trimmed := strings.TrimLeft(piece, "0"); len(trimmed) >= minCodeLength && isNumber(trimmed) {
			out = appendNew(out, trimmed)
		}
	}
	for _, piece := range strings.FieldsFunc(lower, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		add(piece)
		if strings.HasPrefix(piece, "0x") {
			add(piece[2:])
			continue
		}
		// Where letters turn into digits: kb5034441 → 5034441.
		var run []rune
		digit := false
		for _, r := range piece {
			isDigit := unicode.IsDigit(r)
			if len(run) > 0 && isDigit != digit {
				add(string(run))
				run = run[:0]
			}
			digit = isDigit
			run = append(run, r)
		}
		if string(run) != piece {
			add(string(run))
		}
	}
	return out
}

// withoutLeadingZeros returns the canonical form with the zeros at the start of every number dropped: ORA-00942 → ora942.
func withoutLeadingZeros(code string) string {
	var b strings.Builder
	for _, piece := range strings.FieldsFunc(strings.ToLower(code), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if isNumber(piece) || (len(piece) == 1 && piece[0] >= '0' && piece[0] <= '9') {
			if trimmed := strings.TrimLeft(piece, "0"); trimmed != "" {
				piece = trimmed
			} else {
				piece = "0"
			}
		}
		b.WriteString(piece)
	}
	return b.String()
}
