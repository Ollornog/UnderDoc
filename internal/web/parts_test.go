// SPDX-License-Identifier: MIT

package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"
)

type markedPart struct {
	Part  string `json:"part"`
	Ref   string `json:"ref"`
	URL   string `json:"url"`
	MD    string `json:"md"`
	Start int    `json:"start"`
	End   int    `json:"end"`
	Error string `json:"error"`
}

// units counts as the browser counts: UTF-16 code units.
func units(s string) int { return len(utf16.Encode([]rune(s))) }

// Parts, sections and their embedding through the API (ADR-22): marking a stretch of a block, embedding it inside a
// paragraph or as a block, cutting through formatting, the section that grows, copies, uses and broken references.
func TestPartsThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	newPage := func(title string, blocks ...string) string {
		p := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"`+title+`","parent":"`+team.ID+`"}`, sameSite))
		after := ""
		for _, md := range blocks {
			body, _ := json.Marshal(map[string]string{"md": md, "after": after})
			after = decode[apiBlock](t, s.send(http.MethodPost, "/api/pages/"+p.ID+"/blocks", string(body), sameSite)).ID
		}
		return p.ID
	}
	html := func(id string) string {
		var out strings.Builder
		for _, b := range decode[apiDocument](t, s.get("/api/doc/"+id)).Blocks {
			out.WriteString(b.HTML)
		}
		return out.String()
	}
	mark := func(page, block, src, selected, name string) (markedPart, int) {
		t.Helper()
		i := strings.Index(src, selected)
		if i < 0 {
			t.Fatalf("%q not in %q", selected, src)
		}
		body, _ := json.Marshal(map[string]any{"start": units(src[:i]), "end": units(src[:i+len(selected)]), "name": name})
		res := s.send(http.MethodPost, "/api/pages/"+page+"/blocks/"+block+"/parts", string(body), sameSite)
		var out markedPart
		_ = json.Unmarshal(res.Body.Bytes(), &out)
		return out, res.Code
	}

	first := "😀 Error 42: the disk is full. Free some space."
	catalogue := newPage("Errors", first, "**very important to all** customers", "## Disk", "Check it.", "### Mounts", "Each one.", "## Network", "Cables.")
	disk, code := mark(catalogue, "b1", first, "the disk is full", "")
	if code != http.StatusCreated || disk.Part != "t1" || disk.Ref != catalogue+"#b1.t1" || disk.URL != "/o/"+catalogue+"#b1.t1" {
		t.Fatalf("mark = %d %+v", code, disk)
	}
	if want := "😀 Error 42: <!-- part t1 -->the disk is full<!-- /part t1 -->. Free some space."; disk.MD != want {
		t.Errorf("the markers: %q", disk.MD)
	}
	if got := string(utf16.Decode(utf16.Encode([]rune(disk.MD))[disk.Start:disk.End])); got != "the disk is full" {
		t.Errorf("start and end in UTF-16 name the part's text: %q", got)
	}
	bold, _ := mark(catalogue, "b2", "**very important to all** customers", "important to all** customers", "")
	if bold.MD != "**very <!-- part t1 -->important to all** customers<!-- /part t1 -->" {
		t.Errorf("through formatting: %q", bold.MD)
	}

	runbook := newPage("Runbook",
		"When it says :embed{ref=\""+disk.Ref+"\"}, free space.",
		"::embed{ref=\""+bold.Ref+"\"}",
		"::embed{ref=\""+catalogue+"#b3.section\"}",
		"`:embed{ref=\""+disk.Ref+"\"}` is how it is written.",
		"::embed{ref=\""+catalogue+"#b1\"}")
	got := html(runbook)
	if !strings.Contains(got, `When it says <span data-embed-part="`+disk.Ref+`" title="Errors · #b1.t1">the disk is full</span>, free space.`) {
		t.Errorf("a part inside a paragraph: %s", got)
	}
	if !strings.Contains(got, `<figure class="editor-embed" data-ref="`+bold.Ref+`"><p class="leading-relaxed"><strong>important to all</strong> customers</p>`) {
		t.Errorf("a part as a block, cut through its formatting: %s", got)
	}
	section := got[strings.Index(got, `data-ref="`+catalogue+`#b3.section"`):]
	section = section[:strings.Index(section, "</figure>")]
	for _, want := range []string{"Disk", "Check it.", "Mounts", "Each one.", "Errors · Disk", `href="/o/` + catalogue + `#b3.section"`} {
		if !strings.Contains(section, want) {
			t.Errorf("the section lacks %q: %s", want, section)
		}
	}
	if strings.Contains(section, "Network") || strings.Contains(section, "Cables") {
		t.Errorf("the section ends before the next heading of its level: %s", section)
	}
	if !strings.Contains(got, `<code class="code-inline">:embed{ref=`) {
		t.Errorf("in code it stays text: %s", got)
	}
	if strings.Contains(html(runbook), "data-part-open") {
		t.Error("the markers of the source do not travel into an embedding")
	}

	// The section grows with the original.
	added, _ := json.Marshal(map[string]string{"md": "A note added under Mounts.", "after": "b6"})
	s.send(http.MethodPost, "/api/pages/"+catalogue+"/blocks", string(added), sameSite)
	if !strings.Contains(html(runbook), "A note added under Mounts.") {
		t.Error("a paragraph added under the heading shows where the section is embedded")
	}

	// The source knows where its parts and its section are used, and shows its markers as empty spans.
	doc := decode[apiDocument](t, s.get("/api/doc/"+catalogue))
	if p := doc.Blocks[0].Parts; len(p) != 1 || p[0].ID != "t1" || !slices.Equal(p[0].UsedIn, []string{"Runbook"}) {
		t.Errorf("the parts of b1: %+v", p)
	}
	if !slices.Equal(doc.Blocks[2].SectionUsedIn, []string{"Runbook"}) || len(doc.Blocks[2].UsedIn) != 0 {
		t.Errorf("the section of b3: %+v %+v", doc.Blocks[2].SectionUsedIn, doc.Blocks[2].UsedIn)
	}
	if !strings.Contains(doc.Blocks[0].HTML, `<span data-part-open="t1"></span>the disk is full<span data-part-close="t1"></span>`) {
		t.Errorf("the boundary of the part in its page: %s", doc.Blocks[0].HTML)
	}

	// Where something is used, place by place, with whether the viewer may write there.
	var uses struct {
		Ref    string   `json:"ref"`
		Uses   []apiUse `json:"uses"`
		Hidden int      `json:"hidden"`
	}
	_ = json.Unmarshal(s.get("/api/objects/"+catalogue+"/uses?ref=b1.t1").Body.Bytes(), &uses)
	if uses.Ref != disk.Ref || len(uses.Uses) != 1 || uses.Uses[0].Page != runbook || uses.Uses[0].Block != "b1" || !uses.Uses[0].CanEdit {
		t.Errorf("uses of the part: %+v", uses)
	}
	_ = json.Unmarshal(s.get("/api/objects/"+catalogue+"/uses?ref=b1").Body.Bytes(), &uses)
	if len(uses.Uses) != 2 {
		t.Errorf("a part in use makes its block used, next to the block itself: %+v", uses)
	}
	if got := s.get("/api/objects/" + catalogue + "/uses?ref=nonsense").Code; got != http.StatusBadRequest {
		t.Errorf("uses of a broken address = %d", got)
	}

	// A copy is plain text with formatting closed, without markers.
	cur := doc.Blocks[1].Markdown
	from, to := strings.Index(cur, "important"), strings.Index(cur, "cust")+len("cust")
	body, _ := json.Marshal(map[string]int{"start": units(cur[:from]), "end": units(cur[:to])})
	var excerpt struct{ MD, From string }
	_ = json.Unmarshal(s.send(http.MethodPost, "/api/pages/"+catalogue+"/blocks/b2/excerpt", string(body), sameSite).Body.Bytes(), &excerpt)
	if excerpt.MD != "**important to all** cust" || excerpt.From != catalogue+"#b2" {
		t.Errorf("excerpt = %+v", excerpt)
	}

	// A part with a name is a value, listed by name for the Resources tab.
	vat := newPage("Rates", "VAT is 19 %.")
	if res, code := mark(vat, "b1", "VAT is 19 %.", "19 %", "vat"); code != http.StatusCreated || res.MD != `VAT is <!-- part t1 name="vat" -->19 %<!-- /part t1 -->.` {
		t.Fatalf("a value = %d %+v", code, res)
	}
	if _, code := mark(vat, "b1", `VAT is <!-- part t1 name="vat" -->19 %<!-- /part t1 -->.`, "VAT", ""); code != http.StatusCreated {
		t.Fatalf("a part without a name next to the value = %d", code)
	}
	var values []apiValue
	_ = json.Unmarshal(s.get("/api/values").Body.Bytes(), &values)
	if len(values) != 1 || values[0].Name != "vat" || values[0].Text != "19 %" || values[0].Ref != vat+"#b1.t1" || values[0].Title != "Rates" {
		t.Errorf("the values: %+v", values)
	}

	// The facts panel of a page lists what it embeds, in the order of its blocks: a value by its name, a part, a section,
	// a block and a whole page.
	invoice := newPage("Invoice", "Net plus :embed{ref=\""+vat+"#b1.t1\"}.", "::embed{ref=\""+vat+"#b1.t2\"}",
		"::embed{ref=\""+catalogue+"#b1.section\"}", "::embed{ref=\""+catalogue+"#b2\"}", "::embed{ref=\""+vat+"\"}",
		"Again :embed{ref=\""+vat+"#b1.t1\"}.")
	var embeds []string
	for _, e := range decode[apiDocument](t, s.get("/api/doc/"+invoice)).Embeds {
		embeds = append(embeds, e.Kind+":"+e.Name+":"+e.Title+":"+e.Ref+":"+e.URL)
	}
	want := []string{"value:vat:Rates:" + vat + "#b1.t1:/o/" + vat + "#b1.t1", "part::Rates:" + vat + "#b1.t2:/o/" + vat + "#b1.t2",
		"section::Errors:" + catalogue + "#b1.section:/o/" + catalogue + "#b1.section", "block::Errors:" + catalogue + "#b2:/o/" + catalogue + "#b2",
		"page::Rates:" + vat + ":/o/" + vat}
	if !slices.Equal(embeds, want) {
		t.Errorf("what Invoice embeds:\n got %q\nwant %q", embeds, want)
	}

	// The Resources tab: the value says how often it is used, and the snippets are what pages embed, values aside.
	values = nil
	_ = json.Unmarshal(s.get("/api/values").Body.Bytes(), &values)
	if len(values) != 1 || values[0].Uses != 2 {
		t.Errorf("the value is used in two places of Invoice: %+v", values)
	}
	var snippets []apiSnippet
	_ = json.Unmarshal(s.get("/api/snippets").Body.Bytes(), &snippets)
	var listed []string
	for _, x := range snippets {
		if x.Page == vat || x.Page == catalogue {
			listed = append(listed, fmt.Sprintf("%s:%s:%d:%s", x.Kind, x.Ref, x.Uses, x.Text))
		}
	}
	for _, w := range []string{"part:" + vat + "#b1.t2:1:VAT", "section:" + catalogue + "#b1.section:1:", "block:" + catalogue + "#b2:1:"} {
		if !slices.ContainsFunc(listed, func(g string) bool { return strings.HasPrefix(g, w) }) {
			t.Errorf("snippet %q missing in %q", w, listed)
		}
	}
	if slices.ContainsFunc(listed, func(g string) bool { return strings.Contains(g, vat+"#b1.t1:") }) {
		t.Errorf("a value is no snippet: %q", listed)
	}

	// Marking refuses what is more than a paragraph, and a name that does not fit.
	if _, code := mark(catalogue, "b2", "**very important to all** customers", "**very important to all** customers", `a"b`); code != http.StatusBadRequest {
		t.Errorf("a bad name = %d", code)
	}
	twoParas := newPage("Two", "one two\n\nthree four")
	if res, code := mark(twoParas, "b1", "one two\n\nthree four", "two\n\nthree", ""); code != http.StatusBadRequest || !strings.Contains(res.Error, "paragraph") {
		t.Errorf("over two paragraphs = %d %+v", code, res)
	}
	// Who may not change the page may not mark in it, but copies from it.
	notes := ""
	for _, c := range decode[apiState](t, s.get("/api/state")).Collections {
		if c.Private {
			notes = c.ID
		}
	}
	private := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Mine","parent":"`+notes+`"}`, sameSite)).ID
	s.send(http.MethodPost, "/api/pages/"+private+"/blocks", `{"md":"Only mine."}`, sameSite)
	asBob := merged(bob)
	asBob["Sec-Fetch-Site"] = "same-origin"
	for _, what := range []string{"parts", "excerpt"} {
		if got := s.send(http.MethodPost, "/api/pages/"+private+"/blocks/b1/"+what, `{"start":0,"end":4}`, asBob).Code; got != http.StatusNotFound {
			t.Errorf("bob %s in Anna's private page = %d", what, got)
		}
	}

	// An ID is never given out again while something still embeds it.
	if got := s.send(http.MethodPut, "/api/pages/"+catalogue+"/blocks/b1", `{"md":"Error 42: gone.","ways":{"*":{"way":"everywhere"}}}`, sameSite).Code; got != http.StatusOK {
		t.Fatalf("deleting the text of a part in use, everywhere = %d", got)
	}
	again, _ := mark(catalogue, "b1", "Error 42: gone.", "gone", "")
	if again.Part != "t2" {
		t.Errorf("a new part after t1 that Runbook still embeds: %s", again.Part)
	}
	if got := html(runbook); !strings.Contains(got, `data-ref="`+disk.Ref+`" data-removed>Removed by Anna Berg on `) {
		t.Errorf("an embedding of a part deleted everywhere says who removed it: %s", got)
	}

	// The list of problems names parts and sections that are gone.
	s.send(http.MethodPut, "/api/pages/"+catalogue+"/blocks/b3", `{"md":"Disk, no longer a heading."}`, sameSite)
	var p apiProblems
	_ = json.Unmarshal(s.get("/api/problems").Body.Bytes(), &p)
	var problems []string
	for _, x := range p.Database {
		problems = append(problems, x.Title+" #"+x.Block+" "+x.Ref+": "+x.Why)
	}
	for _, want := range []string{
		"Runbook #b1 " + disk.Ref + ": embeds part #b1.t1, which is no longer in Errors",
		"Runbook #b3 " + catalogue + "#b3.section: embeds the section of block #b3, which opens no section in Errors",
	} {
		if !slices.Contains(problems, want) {
			t.Errorf("problems lack %q:\n%s", want, strings.Join(problems, "\n"))
		}
	}
	if slices.ContainsFunc(problems, func(x string) bool { return strings.HasPrefix(x, "Runbook #b4") || strings.HasPrefix(x, "Runbook #b2") }) {
		t.Errorf("code and parts that are there are no problem:\n%s", strings.Join(problems, "\n"))
	}
}
