// SPDX-License-Identifier: MIT

package transfer

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Ollornog/UnderDoc/internal/db"
)

func TestSafeStem(t *testing.T) {
	for title, want := range map[string]string{
		"Network plan":           "Network plan",
		`a/b\c:d*e?f"g<h>i|j`:    "a_b_c_d_e_f_g_h_i_j",
		"  many   spaces\there":  "many spaces here",
		".hidden":                "_hidden",
		"ends with dots...":      "ends with dots",
		"CON":                    "_CON",
		"lpt1.txt":               "_lpt1.txt",
		"console":                "console",
		"...":                    "untitled",
		"Über Straße":            "Über Straße",
		strings.Repeat("x", 300): strings.Repeat("x", maxStem),
	} {
		if got := safeStem(title); got != want {
			t.Errorf("safeStem(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestNamerKeepsNamesApartWithoutCase(t *testing.T) {
	n := newNamer("private")
	got := []string{n.take("Plan"), n.take("plan"), n.take("PLAN"), n.take("Private"), n.take("Plan (2)"), n.take("x.md"), n.take("x"), n.take("y"), n.take("y.md")}
	// A page "y" is y.md (and y/ for its children), so a folder "y.md" must not take that name.
	want := []string{"Plan", "plan (2)", "PLAN (3)", "Private (2)", "Plan (2) (2)", "x.md", "x (2)", "y", "y.md (2)"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func pageNode(id, title string, blocks ...db.Block) *db.Node {
	return &db.Node{Object: db.Object{ID: id, Kind: db.KindPage, Title: title}, Blocks: blocks, NextBlock: nextAfter(blocks)}
}

func collect(t *testing.T, tree Tree) (map[string]string, []string) {
	t.Helper()
	files := map[string]string{}
	warnings, err := Write(tree, func(name string, data []byte) error {
		if _, twice := files[name]; twice {
			t.Errorf("%s is written twice", name)
		}
		files[name] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files, warnings
}

func TestWriteLayout(t *testing.T) {
	plan := pageNode("7k2m9q4x1c", "Plan: network", db.Block{ID: "b2", Body: "## Switches\n\nTwo of them.\n"}, db.Block{ID: "b5", Body: "::embed{ref=\"aaaaaaaaaa#b1\"}"})
	plan.Description = "What goes where"
	plan.NextBlock = 9
	plan.Kids = []*db.Node{pageNode("1111111111", "Kid")}
	team := &db.Node{Object: db.Object{ID: "2222222222", Kind: db.KindCollection, Title: "Team", Access: "read"}, Kids: []*db.Node{
		plan,
		{Object: db.Object{ID: "3333333333", Kind: db.KindLine, Title: "line"}},
		{Object: db.Object{ID: "4444444444", Kind: db.KindFolder, Title: "Plan: network"}},
		{Object: db.Object{ID: "5555555555", Kind: db.KindSpace, Title: "space"}},
	}}
	notes := &db.Node{Object: db.Object{ID: "6666666666", Kind: db.KindCollection, Title: "My notes", Private: true}}
	files, warnings := collect(t, Tree{
		Shared:  []*db.Node{team, {Object: db.Object{ID: "7777777777", Kind: db.KindCollection, Title: "private"}}},
		Private: []db.Owned{{Login: "anna", Node: notes}},
	})
	if len(warnings) != 0 {
		t.Errorf("warnings: %q", warnings)
	}
	names := slices.Sorted(maps.Keys(files))
	wantNames := []string{".underdoc.yaml", "Team/.underdoc.yaml", "Team/Plan_ network (2)/.underdoc.yaml", "Team/Plan_ network.md",
		"Team/Plan_ network/.underdoc.yaml", "Team/Plan_ network/Kid.md", "private (2)/.underdoc.yaml",
		"private/anna/.underdoc.yaml", "private/anna/My notes/.underdoc.yaml"}
	if !slices.Equal(names, wantNames) {
		t.Errorf("files:\n%q\nwant\n%q", names, wantNames)
	}
	if got, want := files["Team/Plan_ network.md"], "---\nid: 7k2m9q4x1c\ntitle: 'Plan: network'\ndescription: What goes where\nnext_block: 9\n---\n\n<!-- block b2 -->\n## Switches\n\nTwo of them.\n\n<!-- block b5 -->\n::embed{ref=\"aaaaaaaaaa#b1\"}\n"; got != want {
		t.Errorf("page:\n%s\nwant\n%s", got, want)
	}
	if got, want := files["Team/.underdoc.yaml"], "id: \"2222222222\"\nkind: collection\ntitle: Team\naccess: read\norder:\n    - name: Plan_ network.md\n    - line: \"3333333333\"\n    - name: Plan_ network (2)\n    - space: \"5555555555\"\n"; got != want {
		t.Errorf("collection:\n%s\nwant\n%s", got, want)
	}
	if got := files["private/anna/My notes/.underdoc.yaml"]; !strings.Contains(got, "private: true") {
		t.Errorf("the private notes say they are private:\n%s", got)
	}
	if got := files[".underdoc.yaml"]; !strings.HasPrefix(got, "format: 1\n") || !strings.Contains(got, "name: private (2)") {
		t.Errorf("root:\n%s", got)
	}
	if got := files["Team/Plan_ network/Kid.md"]; strings.Contains(got, "next_block") || strings.Contains(got, "<!--") {
		t.Errorf("an empty page has neither next_block nor markers:\n%s", got)
	}
}

func TestWriteWarnsAboutBlocksThatWillNotComeBack(t *testing.T) {
	_, warnings := collect(t, Tree{Shared: []*db.Node{{Object: db.Object{ID: "2222222222", Kind: db.KindCollection, Title: "Team"}, Kids: []*db.Node{
		pageNode("1111111111", "Fine", db.Block{ID: "b1", Body: "```\n<!-- block b7 -->\n```"}),
		pageNode("3333333333", "Marker", db.Block{ID: "b1", Body: "a\n<!-- block b7 -->\nb"}),
		pageNode("4444444444", "Open fence", db.Block{ID: "b1", Body: "```go\nx"}, db.Block{ID: "b2", Body: "y"}),
	}}}})
	if len(warnings) != 2 || !strings.HasPrefix(warnings[0], "Team/Marker.md:") || !strings.HasPrefix(warnings[1], "Team/Open fence.md:") {
		t.Errorf("warnings: %q", warnings)
	}
}

func ids(blocks []db.Block) []string {
	var out []string
	for _, b := range blocks {
		out = append(out, b.ID+"="+b.Body)
	}
	return out
}

func TestParsePage(t *testing.T) {
	for name, c := range map[string]struct {
		content string
		title   string
		blocks  []string
		next    int
	}{
		"plain page split at headings, its title heading left out": {
			"\xef\xbb\xbf# Network\r\n\r\nIntro.\r\n\r\n## Switches\r\n\r\nTwo.\r\n\r\n```\r\n# not a heading\r\n```\r\n#### small\r\n\r\n### Cables\r\n",
			"Network", []string{"b1=Intro.", "b2=## Switches\n\nTwo.\n\n```\n# not a heading\n```\n#### small", "b3=### Cables"}, 4,
		},
		"a first heading that is not the title stays": {
			"---\ntitle: Other\n---\n# Network\n\ntext\n", "Other", []string{"b1=# Network\n\ntext"}, 2,
		},
		"title from the file name": {"just text", "page", []string{"b1=just text"}, 2},
		"markers keep their IDs, text before them comes first": {
			"---\nid: x\nnext_block: 12\n---\nloose\n<!-- block b4 -->\nfour\n\n<!-- block b2 -->\n\n<!-- block b2 -->\ntwice\n<!-- block b0 -->\nzero\n```\n<!-- block b9 -->\n```\n",
			"page", []string{"b12=loose", "b4=four", "b2=", "b13=twice", "b14=zero\n```\n<!-- block b9 -->\n```"}, 15,
		},
		"a silly next_block is left alone": {"---\nnext_block: 99999999999\n---\n<!-- block b3 -->\nx\n", "page", []string{"b3=x"}, 4},
		"bad front matter":                 {"---\ntitle: [\n---\ntext\n", "page", []string{"b1=text"}, 2},
		"empty":                            {"", "page", nil, 1},
	} {
		p := parsePage([]byte(c.content), "page.md")
		if p.Title != c.title || !slices.Equal(ids(p.Blocks), c.blocks) || p.Next != c.next {
			t.Errorf("%s: title %q, blocks %q, next %d", name, p.Title, ids(p.Blocks), p.Next)
		}
	}
}

func TestFitCutsLongBlocks(t *testing.T) {
	para := strings.Repeat("a", 40)
	got := fit(para+"\n\n"+para+"\n\n"+para, 90)
	if len(got) != 2 || got[0] != para+"\n\n"+para || got[1] != para {
		t.Errorf("at blank lines: %q", got)
	}
	got = fit(strings.Repeat("line\n", 30), 22)
	for _, piece := range got {
		if len(piece) > 22 {
			t.Errorf("a piece is too long: %q", piece)
		}
	}
	if strings.Join(got, "\n") != strings.TrimSuffix(strings.Repeat("line\n", 30), "\n")+"\n" && strings.Join(got, "\n") != strings.Repeat("line\n", 30) {
		t.Errorf("lines are lost: %q", got)
	}
	got = fit(strings.Repeat("x", 25), 10)
	if !slices.Equal(got, []string{strings.Repeat("x", 10), strings.Repeat("x", 10), strings.Repeat("x", 5)}) {
		t.Errorf("anywhere: %q", got)
	}
	if got := fit("short", 10); !slices.Equal(got, []string{"short"}) {
		t.Errorf("short: %q", got)
	}
}

func TestReadAFolder(t *testing.T) {
	fsys := fstest.MapFS{
		".underdoc.yaml":        {Data: []byte("id: \"2222222222\"\nkind: collection\ndescription: About us\naccess: comment\norder:\n  - name: b.md\n  - line: \"3333333333\"\n  - name: gone.md\n  - name: Sub\n")},
		"a.md":                  {Data: []byte("# A\n\nfirst")},
		"b.md":                  {Data: []byte("---\nid: \"1111111111\"\ntitle: Bee\naccess: nonsense\n---\n<!-- block b3 -->\nbee\n")},
		"b/c.md":                {Data: []byte("child of b")},
		"Sub/.underdoc.yaml":    {Data: []byte("kind: collection\ntitle: The sub\n")},
		"Sub/d.markdown":        {Data: []byte("d")},
		"plain/e.md":            {Data: []byte("e")},
		"plain/picture.png":     {Data: []byte("png")},
		".git/config":           {Data: []byte("x")},
		".hidden.md":            {Data: []byte("x")},
		"broken/.underdoc.yaml": {Data: []byte("order: [")},
		"broken/f.md":           {Data: []byte("f")},
		"page/.underdoc.yaml":   {Data: []byte("kind: folder\n")},
		"page.md":               {Data: []byte("a page beside a folder of the same name")},
		"page/inside.md":        {Data: []byte("inside")},
	}
	root, warnings, err := Read(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if root.Kind != db.KindCollection || root.ID != "2222222222" || root.Description != "About us" || root.Access != "comment" {
		t.Errorf("root: %+v", root.Object)
	}
	var got []string
	var walk func(prefix string, nodes []*db.Node)
	walk = func(prefix string, nodes []*db.Node) {
		for _, n := range nodes {
			got = append(got, prefix+n.Kind+":"+n.Title)
			walk(prefix+"  ", n.Kids)
		}
	}
	walk("", root.Kids)
	want := []string{"page:Bee", "  page:c", "line:line", "collection:The sub", "  page:d", "page:A", "folder:broken", "  page:f",
		"folder:page", "  page:inside", "page:page", "folder:plain", "  page:e"}
	if !slices.Equal(got, want) {
		t.Errorf("tree:\n%q\nwant\n%q", got, want)
	}
	if b := root.Kids[0]; b.ID != "1111111111" || b.Access != "" || len(b.Blocks) != 1 || b.Blocks[0].ID != "b3" || b.NextBlock != 4 {
		t.Errorf("b: %+v", b)
	}
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{"b.md: access \"nonsense\"", "plain/picture.png: left out", "broken/.underdoc.yaml: not valid YAML"} {
		if !strings.Contains(joined, want) {
			t.Errorf("no warning %q in\n%s", want, joined)
		}
	}
	if strings.Contains(joined, ".git") || strings.Contains(joined, "hidden") {
		t.Errorf("hidden entries are left out without a word:\n%s", joined)
	}
}
