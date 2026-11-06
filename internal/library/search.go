// SPDX-License-Identifier: MIT

package library

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Ollornog/UnderDoc/internal/document"
)

const (
	maxQueryBytes = 256
	maxQueryTerms = 8
	snippetBefore = 60
	snippetAfter  = 160
	titleWeight   = 20
)

type indexEntry struct {
	doc   *document.Document
	title string
	text  string
}

// Hit is one search result.
type Hit struct {
	Document *document.Document
	Snippet  []Fragment
}

// Fragment is a piece of a snippet. Match marks the pieces that fit the query.
type Fragment struct {
	Text  string
	Match bool
}

func (lib *Library) buildIndex() {
	lib.index = make([]indexEntry, 0, len(lib.documents))
	for _, doc := range lib.documents {
		lib.index = append(lib.index, indexEntry{doc: doc, title: fold(doc.Title), text: fold(doc.Text)})
	}
}

// fold lowers a text byte-compatibly: a letter whose lower form has another length stays as it is, so offsets in the folded
// text are also valid in the original.
func fold(s string) string {
	return strings.Map(func(r rune) rune {
		lower := unicode.ToLower(r)
		if utf8.RuneLen(lower) != utf8.RuneLen(r) {
			return r
		}
		return lower
	}, s)
}

// Search finds documents containing all words of the query, also inside longer words: German compounds would otherwise
// hide “netz” in “Netzwerkplan”. Matches in the title count more.
func (lib *Library) Search(query string, limit int) []Hit {
	terms := queryTerms(query)
	if len(terms) == 0 {
		return nil
	}
	type scored struct {
		entry *indexEntry
		score int
	}
	var found []scored
	for i := range lib.index {
		entry := &lib.index[i]
		score := 0
		for _, term := range terms {
			inTitle, inText := strings.Count(entry.title, term), strings.Count(entry.text, term)
			if inTitle+inText == 0 {
				score = 0
				break
			}
			score += inTitle*titleWeight + min(inText, titleWeight-1) + 1
		}
		if score > 0 {
			found = append(found, scored{entry, score})
		}
	}
	slices.SortFunc(found, func(a, b scored) int {
		return cmp.Or(cmp.Compare(b.score, a.score), cmp.Compare(a.entry.title, b.entry.title), cmp.Compare(a.entry.doc.ID, b.entry.doc.ID))
	})
	if len(found) > limit {
		found = found[:limit]
	}
	hits := make([]Hit, len(found))
	for i, match := range found {
		hits[i] = Hit{Document: match.entry.doc, Snippet: snippet(match.entry.doc.Text, match.entry.text, terms)}
	}
	return hits
}

func queryTerms(query string) []string {
	if len(query) > maxQueryBytes {
		query = strings.ToValidUTF8(query[:maxQueryBytes], "")
	}
	var terms []string
	for _, term := range strings.Fields(fold(query)) {
		if !slices.Contains(terms, term) && len(terms) < maxQueryTerms {
			terms = append(terms, term)
		}
	}
	return terms
}

// snippet cuts the text around the first match and splits it into marked and unmarked fragments.
func snippet(original, folded string, terms []string) []Fragment {
	first := -1
	for _, term := range terms {
		if at := strings.Index(folded, term); at >= 0 && (first < 0 || at < first) {
			first = at
		}
	}
	start, end := 0, min(snippetAfter, len(original))
	if first >= 0 {
		start, end = max(first-snippetBefore, 0), min(first+snippetAfter, len(original))
	}
	for start > 0 && !utf8.RuneStart(original[start]) {
		start--
	}
	for end < len(original) && !utf8.RuneStart(original[end]) {
		end++
	}
	window, foldedWindow := original[start:end], folded[start:end]
	var fragments []Fragment
	if start > 0 {
		fragments = append(fragments, Fragment{Text: "… "})
	}
	for position := 0; position < len(window); {
		at, length := nextMatch(foldedWindow[position:], terms)
		if at < 0 {
			fragments = append(fragments, Fragment{Text: window[position:]})
			break
		}
		if at > 0 {
			fragments = append(fragments, Fragment{Text: window[position : position+at]})
		}
		fragments = append(fragments, Fragment{Text: window[position+at : position+at+length], Match: true})
		position += at + length
	}
	if end < len(original) {
		fragments = append(fragments, Fragment{Text: " …"})
	}
	return fragments
}

func nextMatch(s string, terms []string) (at, length int) {
	at = -1
	for _, term := range terms {
		if i := strings.Index(s, term); i >= 0 && (at < 0 || i < at || (i == at && len(term) > length)) {
			at, length = i, len(term)
		}
	}
	return at, length
}
