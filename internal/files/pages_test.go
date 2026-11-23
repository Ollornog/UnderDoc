// SPDX-License-Identifier: MIT

package files

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestPseudoPages (B-5, finding 11): a long text without page breaks is cut into pages of about PseudoPage, at a
// paragraph or a line where there is one, and never inside a character; a PDF keeps its pages, a short text is one page.
func TestPseudoPages(t *testing.T) {
	if got := Pages("eins\fzwei\fdrei"); len(got) != 3 || got[1] != "zwei" {
		t.Errorf("the pages of a PDF: %q", got)
	}
	if got := Pages("kurz"); len(got) != 1 || got[0] != "kurz" {
		t.Errorf("a short text: %q", got)
	}
	paragraph := strings.Repeat("Kassenabschluss am Freitag. ", 30) + "\n\n"
	long := strings.Repeat(paragraph, 100)
	pages := Pages(long)
	if len(pages) < 4 {
		t.Fatalf("%d bytes make %d pages", len(long), len(pages))
	}
	total := 0
	for i, p := range pages {
		total += len(p)
		if len(p) > PseudoPage || (i < len(pages)-1 && len(p) < PseudoPage/2) {
			t.Errorf("page %d has %d bytes", i+1, len(p))
		}
		if i < len(pages)-1 && !strings.HasSuffix(p, "Freitag. ") {
			t.Errorf("page %d is not cut at a paragraph: …%q", i+1, p[len(p)-20:])
		}
	}
	if total < len(long)-len(pages)*2 {
		t.Errorf("text went missing: %d of %d bytes", total, len(long))
	}
	// One line without breaks, of characters of two bytes: cut at whole characters.
	umlauts := "x" + strings.Repeat("ä", PseudoPage)
	for i, p := range Pages(umlauts) {
		if !utf8.ValidString(p) || len(p) > PseudoPage {
			t.Errorf("page %d of one long line: %d bytes, valid %v", i+1, len(p), utf8.ValidString(p))
		}
	}
}
