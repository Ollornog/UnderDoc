// SPDX-License-Identifier: MIT

package markdown

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

type fakeResolver struct{}

func (fakeResolver) Wiki(target string) (Target, error) {
	switch target {
	case "known":
		return Target{Href: "/d/known", Title: "Known <Title>", Document: "known"}, nil
	case "twice":
		return Target{}, errors.Join(ErrAmbiguous, errors.New("fits a, b"))
	}
	return Target{}, errors.New("not found")
}

func (fakeResolver) Link(path string) (Target, bool) {
	if path == "other.md" {
		return Target{Href: "/d/other", Document: "other"}, true
	}
	if path == "my page.md" {
		return Target{Href: "/d/my", Document: "my"}, true
	}
	return Target{}, false
}

func (fakeResolver) Image(path string) (Target, bool) {
	if path == "pic.png" {
		return Target{Href: "/files/pic.png"}, true
	}
	return Target{}, false
}

func render(t *testing.T, source string) Rendered {
	t.Helper()
	result, err := Parse([]byte(source)).Render(fakeResolver{})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestFrontMatter(t *testing.T) {
	cases := []struct {
		name, source, id, title, heading string
		frontError                       bool
	}{
		{name: "plain", source: "---\nid: abc\ntitle: T\n---\n# H\n", id: "abc", title: "T", heading: "H"},
		{name: "byte order mark from Notepad", source: "\xef\xbb\xbf---\nid: abc\n---\n# H\n", id: "abc", heading: "H"},
		{name: "CRLF line ends", source: "---\r\nid: abc\r\n---\r\n# H\r\n", id: "abc", heading: "H"},
		{name: "a number as id", source: "---\nid: 4711\n---\ntext\n", id: "4711"},
		{name: "other keys are ignored", source: "---\ntags: [a, b]\nid: abc\n---\ntext\n", id: "abc"},
		{name: "invalid YAML keeps the page", source: "---\nid: [\n---\n# H\n", heading: "H", frontError: true},
		{name: "no closing line means no front matter", source: "---\nid: abc\n\n# H\n", heading: "H"},
		{name: "a rule later on is no front matter", source: "# H\n\n---\n\nid: abc\n\n---\n", heading: "H"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			page := Parse([]byte(c.source))
			if page.Front.ID != c.id || page.Front.Title != c.title || page.Heading != c.heading {
				t.Errorf("got id %q title %q heading %q", page.Front.ID, page.Front.Title, page.Heading)
			}
			if (page.FrontError != nil) != c.frontError {
				t.Errorf("front error: %v", page.FrontError)
			}
		})
	}
}

func TestFrontMatterStaysOutOfThePage(t *testing.T) {
	page := Parse([]byte("---\nid: secret-looking\n---\nbody\n"))
	if text := page.Text(); text != "body" {
		t.Errorf("text = %q", text)
	}
	result, err := page.Render(fakeResolver{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.HTML, "secret-looking") {
		t.Errorf("front matter rendered: %s", result.HTML)
	}
}

func TestStartsWithHeading(t *testing.T) {
	if !Parse([]byte("# A\n\ntext")).StartsWithHeading {
		t.Error("a page opening with a heading was not recognized")
	}
	page := Parse([]byte("text\n\n# A\n"))
	if page.StartsWithHeading || page.Heading != "A" {
		t.Errorf("heading later in the page: %+v", page)
	}
	if got := Parse([]byte("# A **bold** `code`\n")).Heading; got != "A bold code" {
		t.Errorf("heading text = %q", got)
	}
}

func TestNothingExecutableSurvives(t *testing.T) {
	cases := map[string]string{
		"raw script":            "<script>alert(1)</script>",
		"raw inline":            "text <img src=x onerror=alert(1)> text",
		"raw iframe":            "<iframe src=\"https://example.com\"></iframe>",
		"javascript link":       "[x](javascript:alert(1))",
		"mixed case":            "[x](JaVaScRiPt:alert(1))",
		"tab inside the scheme": "[x](<java\tscript:alert(1)>)",
		"leading space":         "[x](< javascript:alert(1)>)",
		"vbscript":              "[x](vbscript:alert(1))",
		"data html":             "[x](data:text/html,<script>alert(1)</script>)",
		"javascript image":      "![x](javascript:alert(1))",
		"svg data image":        "![x](data:image/svg+xml,<svg/onload=alert(1)>)",
		"reference link":        "[x][r]\n\n[r]: javascript:alert(1)",
		"wiki label":            "[[missing|<script>alert(1)</script>]]",
		"wiki label, resolved":  "[[known|<script>alert(1)</script>]]",
		"wiki target":           "[[<img src=x onerror=alert(1)>]]",
		"wiki anchor":           `[[known#"><script>alert(1)</script>]]`,
		"link title":            `[x](other.md "\"><script>alert(1)</script>")`,
		"image alt":             `![<script>alert(1)</script>](pic.png)`,
		"code block":            "```\n<script>alert(1)</script>\n```",
		"code language":         "```\"><script>alert(1)</script>\nx\n```",
		"autolink":              "<javascript:alert(1)>",
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			got := strings.ToLower(render(t, source).HTML)
			for _, forbidden := range []string{"<script", "<iframe", "<img src=x", "onerror=\"", `href="javascript`, `href="vbscript`, `href="data:`, `src="javascript`, `src="data:image/svg`, "\tscript:"} {
				if strings.Contains(got, forbidden) {
					t.Errorf("%q survived in: %s", forbidden, got)
				}
			}
		})
	}
}

func TestNoInlineStyles(t *testing.T) {
	got := render(t, "| a | b |\n|:--|--:|\n| 1 | 2 |\n\n~~x~~ `c`\n\n- [x] t\n\n[^1]\n\n[^1]: f\n").HTML
	if strings.Contains(got, "style=") {
		t.Errorf("an inline style would break the strict CSP: %s", got)
	}
	if !strings.Contains(got, `align="right"`) {
		t.Errorf("column alignment lost: %s", got)
	}
}

func TestWikiLinks(t *testing.T) {
	cases := []struct{ name, source, want string }{
		{"target only uses the title, escaped", "[[known]]", `href="/d/known">Known &lt;Title&gt;</a>`},
		{"label", "[[known|Label]]", `href="/d/known">Label</a>`},
		{"anchor", "[[known#Über uns]]", `href="/d/known#h-über-uns"`},
		{"same page", "[[#Über uns]]", `href="#h-über-uns"`},
		{"inside a table", "| a |\n|---|\n| [[known\\|L]] |\n", `href="/d/known">L</a>`},
		{"missing is visible", "[[gone]]", `data-variant="destructive"`},
		{"ambiguous is visible", "[[twice]]", `data-variant="destructive"`},
		{"code stays code", "`[[known]]`", `[[known]]`},
		{"normal link next to it", "[a](other.md)", `href="/d/other"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := render(t, c.source).HTML; !strings.Contains(got, c.want) {
				t.Errorf("want %q in %s", c.want, got)
			}
		})
	}
}

func TestBrokenReferencesAreReported(t *testing.T) {
	result := render(t, "[[gone]] [[twice]] [a](none.md) ![b](none.png) [c](javascript:x)")
	if len(result.Broken) != 5 {
		t.Fatalf("broken = %+v", result.Broken)
	}
	if !strings.Contains(result.Broken[1].Reason, "ambiguous") {
		t.Errorf("ambiguous reason lost: %+v", result.Broken[1])
	}
	if got := strings.Count(result.HTML, `data-variant="destructive"`); got != 5 {
		t.Errorf("%d of 5 visible: %s", got, result.HTML)
	}
}

func TestOnlyKnownLinkTypesPass(t *testing.T) {
	for _, source := range []string{"[x](ftp://example.com/a)", "[x](blob:abc)", "[x](file:///etc/passwd)", "[x](foo:bar)", `[x](\\\\host\\share)`} {
		result := render(t, source)
		if len(result.Broken) != 1 || strings.Contains(result.HTML, "<a ") {
			t.Errorf("%s: broken %+v, html %s", source, result.Broken, result.HTML)
		}
	}
}

func TestLinksAreCollected(t *testing.T) {
	result := render(t, "[[known]] [a](other.md) [b](https://example.com) ![c](pic.png)")
	if strings.Join(result.Links, ",") != "known,other" {
		t.Errorf("links = %v", result.Links)
	}
}

func TestRelativeLinks(t *testing.T) {
	cases := []struct{ name, source, want string }{
		{"keeps the anchor", "[a](other.md#part)", `href="/d/other#part"`},
		{"decodes the path", "[a](my%20page.md)", `href="/d/my"`},
		{"anchor only", "[a](#part)", `href="#part"`},
		{"external stays", "[a](https://example.com/x)", `href="https://example.com/x"`},
		{"mail stays", "[a](mailto:a@example.com)", `href="mailto:a@example.com"`},
		{"image", "![a](pic.png)", `src="/files/pic.png"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := render(t, c.source).HTML; !strings.Contains(got, c.want) {
				t.Errorf("want %q in %s", c.want, got)
			}
		})
	}
}

func TestExternalImagesAreNotLoaded(t *testing.T) {
	for _, source := range []string{"![logo](https://example.com/a.png)", "![logo](//example.com/a.png)"} {
		got := render(t, source).HTML
		if strings.Contains(got, "<img") {
			t.Errorf("a page must not load from third parties: %s", got)
		}
		if !strings.Contains(got, "example.com/a.png") || !strings.Contains(got, ">logo</a>") {
			t.Errorf("the image should become a link: %s", got)
		}
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{"Übersicht": "h-übersicht", "Über uns!": "h-über-uns", "A  B": "h-a-b", "!!": "h-", "Größe 50 m": "h-größe-50-m"}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRepeatedHeadingsGetTheirOwnAnchor(t *testing.T) {
	got := render(t, "## A\n\n## A\n").HTML
	if !strings.Contains(got, `id="h-a"`) || !strings.Contains(got, `id="h-a-2"`) {
		t.Errorf("anchors: %s", got)
	}
}

func TestTextForSearch(t *testing.T) {
	text := Parse([]byte("# Title\n\nSome *bold* text\nnext line [[known|label]] https://example.com\n\n<div>hidden</div>\n\n```\ncode here\n```\n")).Text()
	for _, want := range []string{"Title", "bold", "text next line", "label", "example.com", "code here"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q missing in %q", want, text)
		}
	}
	if strings.Contains(text, "hidden") {
		t.Errorf("raw HTML is not shown, so it must not be found: %q", text)
	}
}

func TestBrokenEncodingStaysUsable(t *testing.T) {
	got := render(t, "# Gr\xfc\xdfe\n").HTML
	if !strings.Contains(got, "Gr�") {
		t.Errorf("%q", got)
	}
}

func TestContents(t *testing.T) {
	got := render(t, "# Title\n\n## One [[known|Linked]]\n\n### Über uns\n\n#### Too deep\n\n## One\n").Contents
	want := []Heading{{2, "h-one-known-linked", "One Linked"}, {3, "h-über-uns", "Über uns"}, {2, "h-one", "One"}}
	if len(got) != len(want) {
		t.Fatalf("contents = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDescriptionAndTags(t *testing.T) {
	for name, c := range map[string]struct {
		source string
		tags   []string
	}{
		"list":   {"---\ndescription: One line.\ntags: [network, main-site]\n---\nText", []string{"network", "main-site"}},
		"string": {"---\ndescription: One line.\ntags: network, main-site\n---\nText", []string{"network", "main-site"}},
		"none":   {"---\ndescription: One line.\n---\nText", nil},
	} {
		t.Run(name, func(t *testing.T) {
			page := Parse([]byte(c.source))
			if page.FrontError != nil || page.Front.Description != "One line." || !slices.Equal([]string(page.Front.Tags), c.tags) {
				t.Errorf("got %q %q %v", page.Front.Description, page.Front.Tags, page.FrontError)
			}
		})
	}
	if page := Parse([]byte("---\ntags: {a: b}\n---\nText")); page.FrontError == nil {
		t.Error("a map is no list of tags and must be reported")
	}
}
