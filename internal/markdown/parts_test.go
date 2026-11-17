// SPDX-License-Identifier: MIT

package markdown

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// A part cut out of its paragraph reads as it reads there: formatting open at an edge is closed and opened again, and
// only the markers of the part go.
func TestPartMarkdown(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"plain", `Error 42: <!-- part t1 -->the disk is full<!-- /part t1 -->.`, `the disk is full`},
		{"the example of ADR-22", `**very <!-- part t2 -->important to all** customers<!-- /part t2 -->`, `**important to all** customers`},
		{"ends inside bold", `Say <!-- part t1 -->it **loud<!-- /part t1 --> and** clear`, `it **loud**`},
		{"inside bold", `**a <!-- part t1 -->b<!-- /part t1 --> c**`, `**b**`},
		{"inside italic in bold", `***a <!-- part t1 -->b<!-- /part t1 --> c***`, `***b***`},
		{"underscores", `__a <!-- part t1 -->b__ c<!-- /part t1 -->`, `__b__ c`},
		{"italic underscore", `_one <!-- part t1 -->two_ three<!-- /part t1 -->`, `_two_ three`},
		{"strikethrough", `~~gone <!-- part t1 -->for~~ good<!-- /part t1 -->`, `~~for~~ good`},
		{"through a link", `see [the <!-- part t1 -->manual](/o/abc) now<!-- /part t1 -->`, `[manual](/o/abc) now`},
		{"link with title", `[a <!-- part t1 -->b](/x "T") c<!-- /part t1 -->`, `[b](/x "T") c`},
		{"link starts inside", `x <!-- part t1 -->y [z<!-- /part t1 --> w](/q)`, `y [z](/q)`},
		{"inline code whole", "run <!-- part t1 -->`make all`<!-- /part t1 --> now", "`make all`"},
		{"nested parts", `a <!-- part t1 -->b <!-- part t2 -->c<!-- /part t2 --> d<!-- /part t1 --> e`, `b c d`},
		{"overlapping", `a <!-- part t1 -->b <!-- part t2 -->c<!-- /part t1 --> d<!-- /part t2 --> e`, `b c`},
		{"at the start of the paragraph", `<!-- part t1 -->First sentence.<!-- /part t1 --> Second.`, `First sentence.`},
		{"in a heading", `## The <!-- part t1 -->big<!-- /part t1 --> one`, `big`},
		{"in a list item", "- one\n- two <!-- part t1 -->three<!-- /part t1 -->\n- four", `three`},
		{"in a table cell", "| a | b |\n|---|---|\n| x <!-- part t1 -->y<!-- /part t1 --> | z |", `y`},
		{"in a quote", "> quoted <!-- part t1 -->words<!-- /part t1 -->", `words`},
		{"looks like a heading", `a <!-- part t1 --># not a heading<!-- /part t1 -->`, `\# not a heading`},
		{"looks like a list", `a <!-- part t1 -->- not a list<!-- /part t1 -->`, `\- not a list`},
		{"looks like an ordered list", `a <!-- part t1 -->2. not a list<!-- /part t1 -->`, `2\. not a list`},
		{"looks like a list, with bold", `a <!-- part t1 -->- **not** a list<!-- /part t1 -->`, `\- **not** a list`},
		{"over a line break", "one <!-- part t1 -->two\nthree<!-- /part t1 --> four", "two\nthree"},
		{"escapes stay", `a <!-- part t1 -->\*not bold\*<!-- /part t1 -->`, `\*not bold\*`},
		{"value with a name", `VAT is <!-- part t3 name="vat" -->19 %<!-- /part t3 -->.`, `19 %`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id := "t1"
			if p := Parts(c.src); len(p) > 0 {
				id = p[0].ID
			}
			got, err := PartMarkdown(c.src, id)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// Markers that are no part: inside code, without a partner, in another paragraph, or an ID taken already.
func TestNoPart(t *testing.T) {
	for name, src := range map[string]string{
		"inside inline code": "`<!-- part t1 -->x<!-- /part t1 -->`",
		"inside fenced code": "```\n<!-- part t1 -->x<!-- /part t1 -->\n```",
		"no closing marker":  "a <!-- part t1 -->b",
		"closing first":      "a <!-- /part t1 -->b<!-- part t1 -->",
		"other paragraph":    "a <!-- part t1 -->b\n\nc<!-- /part t1 --> d",
		"other list item":    "- a <!-- part t1 -->b\n- c<!-- /part t1 --> d",
		"other table cell":   "| a <!-- part t1 -->b | c<!-- /part t1 --> |\n|---|---|",
	} {
		if _, ok := FindPart(src, "t1"); ok {
			t.Errorf("%s: found a part", name)
		}
		if _, err := PartMarkdown(src, "t1"); err == nil {
			t.Errorf("%s: wrote a part", name)
		}
	}
	src := "a <!-- part t1 -->b<!-- /part t1 --> <!-- part t1 -->c<!-- /part t1 -->"
	if got, _ := PartMarkdown(src, "t1"); got != "b" {
		t.Errorf("a second t1 counts: %q", got)
	}
}

// Cut out of rendered HTML, a part stays in its paragraph: markers in two paragraphs make no part.
func TestCutPartStaysInItsParagraph(t *testing.T) {
	for name, src := range map[string]string{
		"two paragraphs": "a <!-- part t1 -->b\n\nc<!-- /part t1 --> d",
		"two list items": "- a <!-- part t1 -->b\n- c<!-- /part t1 --> d",
		"two cells":      "| a <!-- part t1 -->b | c<!-- /part t1 --> |\n|---|---|",
	} {
		p := parseBlock(src)
		if got, ok := CutPart(renderPlain(p.source, p.root), "t1"); ok {
			t.Errorf("%s: cut %q", name, got)
		}
	}
	p := parseBlock("**a <!-- part t1 -->b *c<!-- /part t1 --> d* e**")
	if got, ok := CutPart(renderPlain(p.source, p.root), "t1"); !ok || got != "<strong>b <em>c</em></strong>" {
		t.Errorf("cut %q %v", got, ok)
	}
}

// A paragraph that starts with a marker stays a paragraph, and markers show as empty spans in the HTML, nothing else.
func TestPartMarkersRender(t *testing.T) {
	page := Parse([]byte("<!-- part t1 -->Start<!-- /part t1 --> of it.\n\n<!-- other --> comment\n"))
	out, err := page.Render(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.HTML, `<p class="leading-relaxed"><span data-part-open="t1"></span>Start<span data-part-close="t1"></span> of it.</p>`) {
		t.Errorf("the marked paragraph: %s", out.HTML)
	}
	if strings.Contains(out.HTML, "other") {
		t.Errorf("raw HTML that is no marker is dropped: %s", out.HTML)
	}
}

func select2(t *testing.T, src, selected string) (int, int) {
	t.Helper()
	i := strings.Index(src, selected)
	if i < 0 {
		t.Fatalf("%q not in %q", selected, src)
	}
	return i, i + len(selected)
}

// Marking puts the markers where the paragraph reads as before, snapping outwards at atomic elements.
func TestMarkPart(t *testing.T) {
	cases := []struct{ name, src, selected, want string }{
		{"words", "Error 42: the disk is full.", "the disk", "Error 42: <!-- part t1 -->the disk<!-- /part t1 --> is full."},
		{"through bold", "**very important to all** customers", "important to all** customers", "**very <!-- part t1 -->important to all** customers<!-- /part t1 -->"},
		{"into inline code", "run `make all` now", "ke al", "run <!-- part t1 -->`make all`<!-- /part t1 --> now"},
		{"from inside code onwards", "run `make all` now", "all` now", "run <!-- part t1 -->`make all` now<!-- /part t1 -->"},
		{"into a link address", "see [manual](/o/abcdefghjk) now", "manual](/o/ab", "see [<!-- part t1 -->manual](/o/abcdefghjk)<!-- /part t1 --> now"},
		{"into an autolink", "at <https://example.org/x> it is", "example", "at <!-- part t1 --><https://example.org/x><!-- /part t1 --> it is"},
		{"into a wiki link", "see [[Network plan]] there", "work", "see <!-- part t1 -->[[Network plan]]<!-- /part t1 --> there"},
		{"text in raw html", "a <b>bold</b> c", "ol", "a <b>b<!-- part t1 -->ol<!-- /part t1 -->d</b> c"},
		{"into raw html", "a <b>bold</b> c", "b>bo", "a <!-- part t1 --><b>bo<!-- /part t1 -->ld</b> c"},
		{"into an escape", `a \*b\* c`, `*b`, `a <!-- part t1 -->\*b<!-- /part t1 -->\* c`},
		{"into an entity", "a &amp; b", "amp", "a <!-- part t1 -->&amp;<!-- /part t1 --> b"},
		{"the whole paragraph", "All of it.", "All of it.", "<!-- part t1 -->All of it.<!-- /part t1 -->"},
		{"in a heading", "## Big title", "Big", "## <!-- part t1 -->Big<!-- /part t1 --> title"},
		{"with a name", "VAT is 19 %.", "19 %", `VAT is <!-- part t1 name="vat" -->19 %<!-- /part t1 -->.`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end := select2(t, c.src, c.selected)
			name := ""
			if strings.Contains(c.want, "name=") {
				name = "vat"
			}
			got, part, err := MarkPart(c.src, start, end, "t1", name)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
			if part.ID != "t1" || part.Name != name {
				t.Errorf("part %+v", part)
			}
		})
	}
}

// What marking refuses: more than a paragraph, or nothing.
func TestMarkPartRefuses(t *testing.T) {
	for name, c := range map[string]struct{ src, selected string }{
		"two paragraphs": {"one two\n\nthree four", "two\n\nthree"},
		"two list items": {"- one\n- two", "one\n- two"},
		"two cells":      {"| a | b |\n|---|---|\n| c | d |", "c | d"},
		"only spaces":    {"a    b", "  "},
		"into a fence":   {"```\ncode\n```", "code"},
	} {
		start, end := select2(t, c.src, c.selected)
		if got, _, err := MarkPart(c.src, start, end, "t1", ""); err == nil {
			t.Errorf("%s: marked %q", name, got)
		}
	}
	if _, _, err := MarkPart("abc", 0, 2, "t1", `a"b`); err != ErrPartName {
		t.Errorf("a name with a quote: %v", err)
	}
	if _, _, err := MarkPart("abc", 0, 2, "x1", ""); err == nil {
		t.Error("an ID of the wrong form")
	}
}

func TestNextPartIDAndStrip(t *testing.T) {
	src := `a <!-- part t2 -->b<!-- /part t2 --> <!-- part t7 name="x" -->c<!-- /part t7 --> ` + "`<!-- part t1 -->`"
	if got := NextPartID(src, 0); got != "t8" {
		t.Errorf("next = %s", got)
	}
	if got := NextPartID("plain", 5); got != "t5" {
		t.Errorf("next from 5 = %s", got)
	}
	if got := StripParts(src); got != "a b c `<!-- part t1 -->`" {
		t.Errorf("strip = %q", got)
	}
}

func TestOffsets(t *testing.T) {
	s := "aé😀b"
	for units, want := range map[int]int{0: 0, 1: 1, 2: 3, 4: 7, 5: 8, 9: 8} {
		if got := ByteOffset(s, units); got != want {
			t.Errorf("ByteOffset(%d) = %d, want %d", units, got, want)
		}
	}
	if got := UnitOffset(s, 7); got != 4 {
		t.Errorf("UnitOffset = %d", got)
	}
}

// The fuzz test marks any stretch of any Markdown: marking either refuses or keeps the paragraph as it reads, and the part
// written on its own reads exactly as it reads in its paragraph, or as its plain text.
func FuzzMarkPart(f *testing.F) {
	for _, seed := range []string{
		"**very important to all** customers",
		"a *b **c** d* e",
		"[link *text*](/o/x \"t\") and `code` and <https://example.org>",
		"~~struck~~ __strong__ _em_ ***both***",
		"## Heading with **bold**",
		"- item *one*\n- item **two**",
		"| a | *b* |\n|---|---|\n| **c** | d |",
		"> quote with [link](/x)\n> second line",
		"text with \\*escapes\\* and &amp; entities",
		"x <!-- part t3 -->y<!-- /part t3 --> z",
		"foo**bar**baz and a_b_c",
	} {
		f.Add(seed, 2, 12)
	}
	f.Fuzz(func(t *testing.T, src string, start, end int) {
		if !utf8.ValidString(src) || len(src) > 400 || start < 0 || end > len(src) || start >= end {
			return
		}
		id := NextPartID(src, 1)
		marked, part, err := MarkPart(src, start, end, id, "")
		if err != nil {
			return
		}
		p, q := parseBlock(src), parseBlock(marked)
		if StripPartSentinels(renderPlain(p.source, p.root)) != StripPartSentinels(renderPlain(q.source, q.root)) {
			t.Fatalf("marking changed how %q reads: %q", src, marked)
		}
		if StripParts(marked) != StripParts(src) {
			t.Fatalf("marking changed the text: %q", marked)
		}
		md, err := PartMarkdown(marked, part.ID)
		if err != nil {
			t.Fatalf("no Markdown for the part just marked in %q: %v", marked, err)
		}
		want, ok := CutPart(renderPlain(q.source, q.root), part.ID)
		if !ok {
			t.Fatalf("the part of %q is not in its HTML", marked)
		}
		got := renderInline(md)
		// Exactly as in its paragraph; or, where the formatting cannot be written so (`*!*0*0*`, runs of the same delimiter
		// that would merge at the edge), as its plain text, which must then read as the text of the part.
		fallback := md == plainPart(want) && strings.Join(strings.Fields(htmlText(got)), " ") == strings.Join(strings.Fields(htmlText(want)), " ")
		if !sameInline(got, want) && !fallback {
			t.Fatalf("%q: the part %q reads %q, in its paragraph %q", marked, md, got, want)
		}
	})
}
