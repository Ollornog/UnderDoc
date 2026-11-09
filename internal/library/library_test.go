// SPDX-License-Identifier: MIT

package library

import (
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/document"
)

func page(source, content string) Page {
	return Page{Source: source, Content: []byte(content), Modified: time.Unix(1_700_000_000, 0)}
}

func problemsOf(lib *Library) string {
	var lines []string
	for _, problem := range lib.Problems() {
		lines = append(lines, problem.Source+": "+problem.Message)
	}
	return strings.Join(lines, "\n")
}

func mustDoc(t *testing.T, lib *Library, id string) *document.Document {
	t.Helper()
	doc, ok := lib.Document(document.ID(id))
	if !ok {
		t.Fatalf("document %q missing, problems:\n%s", id, problemsOf(lib))
	}
	return doc
}

func TestIdentityAndTitle(t *testing.T) {
	lib := Build(Input{Pages: []Page{
		page("a/stable.md", "---\nid: abc\ntitle: From front matter\n---\n# Heading\n"),
		page("a/heading.md", "text\n\n# From heading\n"),
		page("a/plain name.md", "just text\n"),
	}})
	stable := mustDoc(t, lib, "abc")
	// The heading differs from the title, so it is content and stays.
	if !stable.Stable || stable.Title != "From front matter" || stable.StartsWithTitle {
		t.Errorf("%+v", stable)
	}
	if same := mustDoc(t, Build(Input{Pages: []Page{page("s.md", "# Same\n\ntext\n")}}), "~s.md"); !same.StartsWithTitle {
		t.Errorf("a page that opens with its title: %+v", same)
	}
	heading := mustDoc(t, lib, "~a/heading.md")
	if heading.Stable || heading.Title != "From heading" || heading.StartsWithTitle {
		t.Errorf("%+v", heading)
	}
	if plain := mustDoc(t, lib, "~a/plain name.md"); plain.Title != "plain name" {
		t.Errorf("%+v", plain)
	}
}

// The core promise of ADR-13: a move changes where a document appears, not what it is.
func TestMovingKeepsIdentityLinksAndBacklinks(t *testing.T) {
	linking := page("start.md", "See [[abc]] and [[Target page]].\n")
	before := Build(Input{Pages: []Page{linking, page("old/place/target.md", "---\nid: abc\n---\n# Target page\n")}})
	after := Build(Input{Pages: []Page{linking, page("new/target.md", "---\nid: abc\n---\n# Target page\n")}})
	for name, lib := range map[string]*Library{"before": before, "after": after} {
		if problems := problemsOf(lib); problems != "" {
			t.Errorf("%s: %s", name, problems)
		}
		target := mustDoc(t, lib, "abc")
		if len(target.Backlinks) != 1 || target.Backlinks[0] != "~start.md" {
			t.Errorf("%s: backlinks %v", name, target.Backlinks)
		}
		if start := mustDoc(t, lib, "~start.md"); strings.Count(start.HTML, `href="/o/abc"`) != 2 {
			t.Errorf("%s: %s", name, start.HTML)
		}
	}
	if folder, _ := after.Tree().Location("abc"); folder.Path != "new" {
		t.Errorf("location after the move: %q", folder.Path)
	}
	if _, stillThere := after.Tree().Folder("old/place"); stillThere {
		t.Error("the old folder should be gone from the tree")
	}
}

func TestBadIDs(t *testing.T) {
	lib := Build(Input{Pages: []Page{
		page("a.md", "---\nid: same\n---\nfirst\n"),
		page("b.md", "---\nid: same\n---\nsecond\n"),
		page("c.md", "---\nid: ../../evil\n---\nthird\n"),
		page("d.md", "---\nid: [\n---\nfourth\n"),
	}})
	if first := mustDoc(t, lib, "same"); first.Source != "a.md" {
		t.Errorf("the first file by path should keep the id, got %s", first.Source)
	}
	if second := mustDoc(t, lib, "~b.md"); second.Stable {
		t.Errorf("%+v", second)
	}
	mustDoc(t, lib, "~c.md")
	mustDoc(t, lib, "~d.md")
	problems := problemsOf(lib)
	for _, want := range []string{"b.md: id \"same\" is already used by a.md", "c.md: id \"../../evil\" is not usable", "d.md: front matter is not valid YAML"} {
		if !strings.Contains(problems, want) {
			t.Errorf("%q missing in:\n%s", want, problems)
		}
	}
}

func TestWikiResolution(t *testing.T) {
	lib := Build(Input{Pages: []Page{
		page("x/from.md", strings.Join([]string{
			"[[byid]]", "[[Some Title]]", "[[some title]]", "[[filename]]", "[[sibling]]", "[[sibling.md]]", "[[../y/filename]]",
			"[[/y/filename.md]]", "[[Twin]]", "[[gone]]", "[[../../../etc/passwd]]",
		}, " ")),
		page("x/sibling.md", "s"),
		page("y/filename.md", "---\nid: byid\ntitle: Some Title\n---\n"),
		page("a/one.md", "# Twin\n"),
		page("b/two.md", "# Twin\n"),
	}})
	from := mustDoc(t, lib, "~x/from.md")
	if got := strings.Count(from.HTML, `href="/o/byid"`); got != 6 {
		t.Errorf("%d of 6 links to the same page: %s", got, from.HTML)
	}
	if got := strings.Count(from.HTML, `href="/o/~x/sibling.md"`); got != 2 {
		t.Errorf("%d of 2 links to the sibling: %s", got, from.HTML)
	}
	problems := problemsOf(lib)
	if !strings.Contains(problems, "[[Twin]]: ambiguous, fits a/one.md, b/two.md") {
		t.Errorf("ambiguous link not named:\n%s", problems)
	}
	if !strings.Contains(problems, "[[gone]]: no such page") || !strings.Contains(problems, "[[../../../etc/passwd]]: no such page") {
		t.Errorf("missing links not named:\n%s", problems)
	}
}

func TestRelativeLinksAndImages(t *testing.T) {
	lib := Build(Input{
		Pages: []Page{
			page("a/b/page.md", "[up](../other.md) [root](/other2.md) ![i](pic.png) ![j](../../root.png) [file](pic.png) [out](../../../x.md) ![out](../../../x.png) ![none](no.png)"),
			page("a/other.md", "o"),
			page("other2.md", "o"),
		},
		Images: []string{"a/b/pic.png", "root.png"},
	})
	doc := mustDoc(t, lib, "~a/b/page.md")
	for _, want := range []string{`href="/o/~a/other.md"`, `href="/o/~other2.md"`, `src="/files/a/b/pic.png"`, `src="/files/root.png"`, `href="/files/a/b/pic.png"`} {
		if !strings.Contains(doc.HTML, want) {
			t.Errorf("%q missing in %s", want, doc.HTML)
		}
	}
	if got := strings.Count(doc.HTML, `data-variant="destructive"`); got != 3 {
		t.Errorf("%d of 3 broken references visible: %s", got, doc.HTML)
	}
}

func TestLinksToFolders(t *testing.T) {
	lib := Build(Input{
		Pages: []Page{
			page("a/page.md", "[sub](sub/) [plain](sub) [space](my%20docs/) [up](../) [root](/) [other](/b/) [none](nope/) [out](../../) [query](?x=1) [dot](/.sub)"),
			page("a/sub/inner.md", "i"),
			page("a/my docs/inner.md", "i"),
			page("b/inner.md", "i"),
			page("sub/inner.md", "i"),
		},
	})
	doc := mustDoc(t, lib, "~a/page.md")
	for _, want := range []string{`href="/f/a/sub"`, `href="/f/a/my%20docs"`, `href="/f/"`, `href="/f/b"`} {
		if !strings.Contains(doc.HTML, want) {
			t.Errorf("%q missing in %s", want, doc.HTML)
		}
	}
	if got := strings.Count(doc.HTML, `data-variant="destructive"`); got != 4 {
		t.Errorf("%d of 4 broken references visible: %s", got, doc.HTML)
	}
	problems := problemsOf(lib)
	for _, want := range []string{"nope/", "../../", "?x=1", "/.sub"} {
		if !strings.Contains(problems, want) {
			t.Errorf("%q missing in the problems:\n%s", want, problems)
		}
	}
	if strings.Contains(problems, "sub/") || strings.Contains(problems, "docs") {
		t.Errorf("a link to a folder counts as broken:\n%s", problems)
	}
}

func TestBacklinksSkipSelfAndDuplicates(t *testing.T) {
	lib := Build(Input{Pages: []Page{
		page("a.md", "---\nid: a\n---\n[[a]] [[b]] [[b]]\n"),
		page("b.md", "---\nid: b\n---\n"),
	}})
	if a := mustDoc(t, lib, "a"); len(a.Backlinks) != 0 || len(a.Links) != 1 {
		t.Errorf("%+v", a)
	}
	if b := mustDoc(t, lib, "b"); len(b.Backlinks) != 1 {
		t.Errorf("%+v", b)
	}
}

func TestTreeHoldsOnlyArrangement(t *testing.T) {
	lib := Build(Input{Pages: []Page{
		page("z.md", "# Zebra\n"),
		page("a.md", "# apple\n"),
		page("sub/deep/x.md", "x"),
		page("Other/y.md", "y"),
	}})
	root := lib.Tree().Root()
	if len(root.Documents) != 2 || root.Documents[0] != "~a.md" {
		t.Errorf("documents = %v", root.Documents)
	}
	if len(root.Folders) != 2 || root.Folders[0].Name != "Other" || root.Folders[1].Name != "sub" {
		t.Errorf("folders = %+v", root.Folders)
	}
	if deep, ok := lib.Tree().Folder("sub/deep"); !ok || len(deep.Documents) != 1 {
		t.Errorf("deep = %+v", deep)
	}
}

func TestHome(t *testing.T) {
	for _, c := range []struct {
		pages []Page
		want  string
	}{
		{[]Page{page("README.md", "r"), page("index.md", "i")}, "~index.md"},
		{[]Page{page("readme.MD", "r"), page("other.md", "o")}, "~readme.MD"},
		{[]Page{page("sub/index.md", "s"), page("other.md", "o")}, ""},
	} {
		doc, ok := Build(Input{Pages: c.pages}).Home()
		if c.want == "" {
			if ok {
				t.Errorf("unexpected home %s", doc.ID)
			}
			continue
		}
		if !ok || string(doc.ID) != c.want {
			t.Errorf("home = %+v, want %s", doc, c.want)
		}
	}
}

func hitsOf(lib *Library, query string) []string {
	var titles []string
	for _, hit := range lib.Search(query, 50) {
		titles = append(titles, hit.Document.Title)
	}
	return titles
}

func TestSearch(t *testing.T) {
	lib := Build(Input{Pages: []Page{
		page("a.md", "# Netzwerkplan\n\nDer Router steht im Turm.\n"),
		page("b.md", "# Kontakte\n\nFür das netz zuständig: Größe egal. Router, Router, Router.\n"),
		page("c.md", "# Sonstiges\n\nNichts davon.\n"),
	}})
	cases := []struct {
		query string
		want  string
	}{
		{"netz", "Netzwerkplan,Kontakte"},
		{"NETZ", "Netzwerkplan,Kontakte"},
		{"router turm", "Netzwerkplan"},
		{"turm router", "Netzwerkplan"},
		{"router kein-treffer", ""},
		{"GRÖSSE", ""},
		{"größe", "Kontakte"},
		{"", ""},
		{"   ", ""},
	}
	for _, c := range cases {
		if got := strings.Join(hitsOf(lib, c.query), ","); got != c.want {
			t.Errorf("search %q = %q, want %q", c.query, got, c.want)
		}
	}
}

func TestSearchRanksTitlesFirstAndLimits(t *testing.T) {
	var pages []Page
	for i := range 20 {
		pages = append(pages, page("p"+string(rune('a'+i))+".md", "word word word word word\n"))
	}
	pages = append(pages, page("t.md", "# Word\n"))
	lib := Build(Input{Pages: pages})
	hits := lib.Search("word", 5)
	if len(hits) != 5 || hits[0].Document.Title != "Word" {
		t.Errorf("hits = %d, first %q", len(hits), hits[0].Document.Title)
	}
}

func TestSnippetMarksMatchesWithoutBreakingLetters(t *testing.T) {
	long := strings.Repeat("äöü ", 200)
	lib := Build(Input{Pages: []Page{page("a.md", long+"ROUTER im Turm "+long)}})
	hits := lib.Search("router", 5)
	if len(hits) != 1 {
		t.Fatal("no hit")
	}
	var marked, whole string
	for _, fragment := range hits[0].Snippet {
		if fragment.Match {
			marked += fragment.Text
		}
		whole += fragment.Text
	}
	if marked != "ROUTER" {
		t.Errorf("marked %q, original spelling expected", marked)
	}
	if strings.ContainsRune(whole, '�') || !strings.HasPrefix(whole, "… ") || !strings.HasSuffix(whole, " …") {
		t.Errorf("snippet = %q", whole)
	}
	if len(whole) > 400 {
		t.Errorf("snippet too long: %d", len(whole))
	}
}

func TestFoldKeepsOffsets(t *testing.T) {
	for _, s := range []string{"İstanbul", "ẞ", "ÄÖÜ", "Kelvin K", "Ⱥ"} {
		if len(fold(s)) != len(s) {
			t.Errorf("fold(%q) changes the length: %q", s, fold(s))
		}
	}
}

func TestSearchQueryLimits(t *testing.T) {
	terms := queryTerms(strings.Repeat("ä", 500))
	if len(terms) != 1 || len(terms[0]) > maxQueryBytes || strings.ContainsRune(terms[0], '�') {
		t.Errorf("long query: %d terms, %d bytes", len(terms), len(terms[0]))
	}
	if got := queryTerms("a b c d e f g h i j a"); len(got) != maxQueryTerms {
		t.Errorf("terms = %v", got)
	}
}

func BenchmarkBuild5000Pages(b *testing.B) {
	pages := make([]Page, 5000)
	for i := range pages {
		pages[i] = page("folder"+string(rune('a'+i%26))+"/page"+strings.Repeat("x", i%7)+string(rune('a'+i%26))+string(rune('a'+(i/26)%26))+string(rune('a'+(i/676)%26))+".md",
			"# Title\n\n"+strings.Repeat("Some ordinary text with [[other page]] and more words.\n\n", 40))
	}
	for b.Loop() {
		Build(Input{Pages: pages})
	}
}

func TestRecent(t *testing.T) {
	older, newer := page("a.md", "a"), page("b.md", "b")
	newer.Modified = older.Modified.Add(time.Hour)
	third := page("c.md", "c")
	third.Modified = older.Modified.Add(-time.Hour)
	recent := Build(Input{Pages: []Page{older, newer, third}}).Recent(2)
	if len(recent) != 2 || recent[0].Source != "b.md" || recent[1].Source != "a.md" {
		t.Errorf("recent = %+v", recent)
	}
}

func TestDescriptionAndTagsReachTheDocument(t *testing.T) {
	lib := Build(Input{Pages: []Page{page("a.md", "---\ndescription: \" About a. \"\ntags: [Network, '#network', main-site]\n---\nText")}})
	doc := mustDoc(t, lib, "~a.md")
	if doc.Description != "About a." || strings.Join(doc.Tags, ",") != "network,main-site" {
		t.Fatalf("%q %q", doc.Description, doc.Tags)
	}
}

func TestTagged(t *testing.T) {
	lib := Build(Input{Pages: []Page{page("b.md", "---\ntags: [net]\n---\n# Beta"), page("a.md", "---\ntags: net, ops\n---\n# Alpha"), page("c.md", "# Gamma")}})
	var titles []string
	for _, doc := range lib.Tagged("net") {
		titles = append(titles, doc.Title)
	}
	if strings.Join(titles, ",") != "Alpha,Beta" || len(lib.Tagged("none")) != 0 {
		t.Fatal(titles)
	}
}

// A block of the database may link to an object of the database by its address; the folder source cannot know it, so the
// link stays as written and is checked by the list of problems. Pages of the folder source keep their checks.
func TestBlocksLinkToTheDatabase(t *testing.T) {
	lib := Build(Input{Pages: []Page{page("plan.md", "---\nid: plan\n---\n# Plan\n"), page("other.md", "# Other\n\n[x](/o/7k2m9q4x1c)\n")}})
	got, err := lib.RenderMarkdown("[Rack](/o/7k2m9q4x1c) and [named](/o/7k2m9q4x1c/rack) and [plan](/o/plan) and [bad](/o/NOT-AN-ID)")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.HTML, `href="/o/7k2m9q4x1c"`) || !strings.Contains(got.HTML, `href="/o/7k2m9q4x1c/rack"`) || !strings.Contains(got.HTML, `href="/o/plan"`) {
		t.Errorf("links to the database stay: %s", got.HTML)
	}
	if len(got.Broken) != 1 || got.Broken[0].Reference != "/o/NOT-AN-ID" {
		t.Errorf("what is not an address is still broken: %+v", got.Broken)
	}
	if len(lib.Problems()) != 1 || !strings.Contains(lib.Problems()[0].Message, "/o/7k2m9q4x1c") {
		t.Errorf("a page of the folder source does not link into the database unchecked: %+v", lib.Problems())
	}
}
