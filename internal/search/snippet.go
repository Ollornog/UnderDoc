// SPDX-License-Identifier: MIT

package search

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Engines mark matches with these two characters of the private use area, which never stand in a text: Prepare drops
// them. Fragments turns the marked text into pieces, so nothing an engine returns ever reaches a page as HTML.
const (
	MarkStart = ""
	MarkEnd   = ""
)

// Fragment is a piece of a snippet. Match marks the pieces that fit the query.
type Fragment struct {
	Text  string `json:"text"`
	Match bool   `json:"match,omitempty"`
}

// Fragments splits a text that an engine marked.
func Fragments(marked string) []Fragment {
	var out []Fragment
	match := false
	for marked != "" {
		marker := MarkStart
		if match {
			marker = MarkEnd
		}
		at := strings.Index(marked, marker)
		piece := marked
		if at >= 0 {
			piece = marked[:at]
		}
		piece = strings.NewReplacer(MarkStart, "", MarkEnd, "").Replace(piece)
		if piece != "" {
			out = append(out, Fragment{Text: piece, Match: match})
		}
		if at < 0 {
			break
		}
		marked = marked[at+len(marker):]
		match = !match
	}
	return out
}

func dropMarks(s string) string {
	if !strings.ContainsAny(s, MarkStart+MarkEnd) {
		return s
	}
	return strings.NewReplacer(MarkStart, "", MarkEnd, "").Replace(s)
}

const (
	snippetBefore = 60
	snippetAfter  = 180
)

type span struct{ start, end int }

// Snippet cuts a text around the first place that fits the query and marks every code and word of it there. Codes match
// as a whole or by a part, words at the start of a word, so "druck" marks "Drucker".
func Snippet(text string, codes, words []string) []Fragment {
	var spans []span
	for _, token := range FindCodes(text) {
		for _, code := range codes {
			if token.Code == code || slices.Contains(token.Parts, code) {
				spans = append(spans, span{token.Start, token.End})
				break
			}
		}
	}
	lower := foldSameLength(text)
	for _, word := range words {
		word = foldSameLength(word)
		if word == "" {
			continue
		}
		for from := 0; ; {
			at := strings.Index(lower[from:], word)
			if at < 0 {
				break
			}
			at += from
			if wordStart(text, at) {
				end := at + len(word)
				for end < len(text) {
					r, size := utf8.DecodeRuneInString(text[end:])
					if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
						break
					}
					end += size
				}
				spans = append(spans, span{at, end})
			}
			from = at + len(word)
		}
	}
	spans = mergeSpans(spans)
	start, end := 0, min(len(text), snippetAfter)
	if len(spans) > 0 {
		start, end = max(spans[0].start-snippetBefore, 0), min(spans[0].start+snippetAfter, len(text))
	}
	for start > 0 && !utf8.RuneStart(text[start]) {
		start--
	}
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end++
	}
	var out []Fragment
	if start > 0 {
		out = append(out, Fragment{Text: "… "})
	}
	position := start
	for _, s := range spans {
		if s.end <= start || s.start >= end {
			continue
		}
		s.start, s.end = max(s.start, position), min(s.end, end)
		if s.start > position {
			out = append(out, Fragment{Text: text[position:s.start]})
		}
		out = append(out, Fragment{Text: text[s.start:s.end], Match: true})
		position = s.end
	}
	if position < end {
		out = append(out, Fragment{Text: text[position:end]})
	}
	if end < len(text) {
		out = append(out, Fragment{Text: " …"})
	}
	return out
}

func wordStart(text string, at int) bool {
	if at == 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(text[:at])
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

func mergeSpans(spans []span) []span {
	if len(spans) < 2 {
		return spans
	}
	sorted := slices.SortedFunc(slices.Values(spans), func(a, b span) int { return cmp.Compare(a.start, b.start) })
	out := []span{sorted[0]}
	for _, s := range sorted[1:] {
		last := &out[len(out)-1]
		if s.start <= last.end {
			last.end = max(last.end, s.end)
			continue
		}
		out = append(out, s)
	}
	return out
}

// foldSameLength lowers a text, keeping a letter whose lower form has another length, so offsets stay valid.
func foldSameLength(s string) string {
	return strings.Map(func(r rune) rune {
		lower := unicode.ToLower(r)
		if utf8.RuneLen(lower) != utf8.RuneLen(r) {
			return r
		}
		return lower
	}, s)
}
