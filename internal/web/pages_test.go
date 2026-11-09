// SPDX-License-Identifier: MIT

package web

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestPagesThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
	if got := s.send(http.MethodPost, "/api/pages", `{"title":"Plan","parent":"`+team.ID+`"}`, map[string]string{"Sec-Fetch-Site": "cross-site"}).Code; got != http.StatusForbidden {
		t.Errorf("a page from another site = %d", got)
	}
	made := s.send(http.MethodPost, "/api/pages", `{"title":"Plan","parent":"`+team.ID+`"}`, sameSite)
	if made.Code != http.StatusCreated {
		t.Fatalf("page %d %s", made.Code, made.Body.String())
	}
	plan := decode[apiNode](t, made)
	if found := findNode(decode[apiState](t, s.get("/api/state")).Collections, plan.ID); found == nil || found.Kind != "page" {
		t.Errorf("the page is in the tree: %+v", found)
	}
	if got := s.get("/o/" + plan.ID); got.Code != http.StatusOK {
		t.Errorf("the page has its address: %d", got.Code)
	}
	empty := page(t, s, plan.ID)
	if !empty.CanEdit || len(empty.Blocks) != 0 || empty.Place != "Team" || empty.Owner != "Anna Berg" {
		t.Errorf("a new page: %+v", empty)
	}

	blocks := "/api/pages/" + plan.ID + "/blocks"
	first := decode[apiBlock](t, s.send(http.MethodPost, blocks, `{"md":"## Switches\n\nTwo in the rack."}`, sameSite))
	if first.ID != "b1" || !strings.Contains(first.HTML, "<h2") || !strings.Contains(first.HTML, "Two in the rack.") {
		t.Errorf("a block answers rendered: %+v", first)
	}
	second := decode[apiBlock](t, s.send(http.MethodPost, blocks, `{"after":"b1","md":"| Port | VLAN |\n|---|---|\n| 1 | 10 |"}`, sameSite))
	if second.ID != "b2" || !strings.Contains(second.HTML, "<table") {
		t.Errorf("a table block: %+v", second)
	}
	edited := s.send(http.MethodPut, blocks+"/b1", `{"md":"Three in the rack.<script>alert(1)</script>"}`, sameSite)
	if b := decode[apiBlock](t, edited); strings.Contains(b.HTML, "<script") || !strings.Contains(b.Markdown, "<script>") {
		t.Errorf("raw HTML is dropped from the view and kept in the source: %+v", b)
	}
	if got := s.send(http.MethodDelete, blocks+"/b2", "", sameSite).Code; got != http.StatusNoContent {
		t.Errorf("delete = %d", got)
	}
	if got := s.send(http.MethodDelete, blocks+"/b2", "", sameSite).Code; got != http.StatusNotFound {
		t.Errorf("delete twice = %d", got)
	}
	doc := page(t, s, plan.ID)
	if len(doc.Blocks) != 1 || doc.Blocks[0].ID != "b1" || !strings.Contains(doc.Blocks[0].HTML, "Three in the rack.") {
		t.Errorf("the page after the changes: %+v", doc.Blocks)
	}
	versions := decode[[]apiVersion](t, s.get("/api/pages/"+plan.ID+"/versions"))
	if len(versions) != 4 || !versions[0].Deleted || versions[0].Block != "b2" || versions[3].Block != "b1" || versions[3].Author != "Anna Berg" {
		t.Errorf("versions, newest first: %+v", versions)
	}
	// Moments go out in UTC, RFC 3339; the browser shows them in its own time zone.
	if at, err := time.Parse(time.RFC3339, versions[0].At); err != nil || !strings.HasSuffix(versions[0].At, "Z") || time.Since(at) > time.Minute {
		t.Errorf("a version is stamped in UTC: %q %v", versions[0].At, err)
	}
	if doc := decode[apiDocument](t, s.get("/api/doc/"+plan.ID)); !strings.HasSuffix(doc.Changed, "Z") || !strings.HasSuffix(doc.Created, "Z") {
		t.Errorf("changed and created in UTC: %q %q", doc.Changed, doc.Created)
	}
	if got := s.send(http.MethodPost, blocks, `{"md":"`+strings.Repeat("x", 100001)+`"}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("a block too long = %d", got)
	}
	if got := s.send(http.MethodPost, "/api/pages/"+team.ID+"/blocks", `{"md":"x"}`, sameSite).Code; got != http.StatusNotFound {
		t.Errorf("a block in a collection = %d", got)
	}
}

func TestPagesFollowAccess(t *testing.T) {
	s := withDatabase(t)
	state := decode[apiState](t, s.get("/api/state"))
	notes := state.Collections[0].ID
	diary := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Diary","parent":"`+notes+`"}`, sameSite))
	s.send(http.MethodPost, "/api/pages/"+diary.ID+"/blocks", `{"md":"Secret."}`, sameSite)
	asBob := merged(bob)
	asBob["Sec-Fetch-Site"] = "same-origin"
	for _, target := range []string{"/api/doc/" + diary.ID, "/api/pages/" + diary.ID + "/versions", "/o/" + diary.ID} {
		if got := s.getWith(target, asBob); got.Code != http.StatusNotFound || strings.Contains(got.Body.String(), "Secret") {
			t.Errorf("bob reads %s: %d", target, got.Code)
		}
	}
	if got := s.send(http.MethodPut, "/api/pages/"+diary.ID+"/blocks/b1", `{"md":"Mine."}`, asBob).Code; got != http.StatusNotFound {
		t.Errorf("bob writes into Anna's private page: %d", got)
	}
	if got := s.send(http.MethodPost, "/api/pages", `{"title":"Mine","parent":"`+notes+`"}`, asBob).Code; got != http.StatusNotFound {
		t.Errorf("bob makes a page in Anna's notes: %d", got)
	}

	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
	plan := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Plan","parent":"`+team.ID+`"}`, sameSite))
	s.send(http.MethodPost, "/api/pages/"+plan.ID+"/blocks", `{"md":"Text."}`, sameSite)
	if doc := decode[apiDocument](t, s.getWith("/api/doc/"+plan.ID, asBob)); !doc.CanEdit {
		t.Errorf("others may edit by default: %+v", doc)
	}
	if got := s.send(http.MethodPut, "/api/pages/"+plan.ID+"/blocks/b1", `{"md":"Bob was here."}`, asBob).Code; got != http.StatusOK {
		t.Errorf("bob edits a shared page: %d", got)
	}
}

// A block embeds a block or a whole page of another page by its address: shown live, with a link back, each side knowing
// where it is used, and what cannot be shown saying why.
func TestEmbeddingThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
	newPage := func(title string, blocks ...string) string {
		p := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"`+title+`","parent":"`+team.ID+`"}`, sameSite))
		after := ""
		for _, md := range blocks {
			body, _ := json.Marshal(map[string]string{"md": md, "after": after})
			b := decode[apiBlock](t, s.send(http.MethodPost, "/api/pages/"+p.ID+"/blocks", string(body), sameSite))
			after = b.ID
		}
		return p.ID
	}
	doc := func(id string) apiDocument { return decode[apiDocument](t, s.get("/api/doc/"+id)) }
	html := func(id string) string {
		var out strings.Builder
		for _, b := range doc(id).Blocks {
			out.WriteString(b.HTML)
		}
		return out.String()
	}
	plan := newPage("Plan", "## Switches", "| Port | VLAN |\n|---|---|\n| 1 | 10 |")
	rack := newPage("Rack", "The ports:\n\n::embed{ref=\""+plan+"#b2\"}\n\nThat is all.")
	got := html(rack)
	if !strings.Contains(got, `<figure class="embed" data-ref="`+plan+`#b2">`) || !strings.Contains(got, "<td>10</td>") ||
		!strings.Contains(got, `href="/o/`+plan+`#b2"`) || !strings.Contains(got, "The ports") || !strings.Contains(got, "That is all") {
		t.Fatalf("the table of Plan shows in Rack, with a link back: %s", got)
	}
	if got := s.send(http.MethodPut, "/api/pages/"+plan+"/blocks/b2", `{"md":"| Port | VLAN |\n|---|---|\n| 1 | 20 |"}`, sameSite).Code; got != http.StatusOK {
		t.Fatalf("edit = %d", got)
	}
	if got := html(rack); !strings.Contains(got, "<td>20</td>") {
		t.Errorf("it is live: %s", got)
	}
	p := doc(plan)
	if !slices.Contains(p.UsedIn, "Rack (block #b2)") || len(p.Blocks) != 2 || !slices.Equal(p.Blocks[1].UsedIn, []string{"Rack"}) || len(p.Blocks[0].UsedIn) != 0 {
		t.Errorf("Plan and its block know where they are used: %+v %+v", p.UsedIn, p.Blocks)
	}
	whole := newPage("Overview", "::embed{ref=\""+plan+"\"}")
	if got := html(whole); !strings.Contains(got, "<h2") || !strings.Contains(got, "<td>20</td>") || !strings.Contains(got, `href="/o/`+plan+`"`) {
		t.Errorf("a whole page embeds: %s", got)
	}
	gone := newPage("Gone", "::embed{ref=\""+plan+"#b9\"}", "::embed{ref=\"zzzzzzzzzz\"}")
	if got := html(gone); !strings.Contains(got, "Block #b9 is no longer in Plan") || !strings.Contains(got, "Not to be found: zzzzzzzzzz") {
		t.Errorf("what is missing says so: %s", got)
	}
	self := newPage("Self", "First", "")
	body, _ := json.Marshal(map[string]string{"md": "::embed{ref=\"" + self + "\"}"})
	s.send(http.MethodPut, "/api/pages/"+self+"/blocks/b2", string(body), sameSite)
	if got := html(self); !strings.Contains(got, "This embeds itself") {
		t.Errorf("a page embedding itself stops: %s", got)
	}
	chain := plan
	for k := 0; k < 7; k++ {
		chain = newPage("Chain", "::embed{ref=\""+chain+"#b"+map[bool]string{true: "2", false: "1"}[k == 0]+"\"}")
	}
	if got := html(chain); !strings.Contains(got, "Embedded too deep") {
		t.Errorf("deep embedding stops: %.300s", got)
	}
	// A private page embedded in a shared one: others see that it is not for them, not what it holds.
	notes := ""
	for _, c := range decode[apiState](t, s.get("/api/state")).Collections {
		if c.Private {
			notes = c.ID
		}
	}
	secret := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Secret","parent":"`+notes+`"}`, sameSite))
	s.send(http.MethodPost, "/api/pages/"+secret.ID+"/blocks", `{"md":"The code is 4711.","after":""}`, sameSite)
	shared := newPage("Shared", "::embed{ref=\""+secret.ID+"\"}")
	if got := decode[apiDocument](t, s.getWith("/api/doc/"+shared, bob)); len(got.Blocks) != 1 || strings.Contains(got.Blocks[0].HTML, "4711") || !strings.Contains(got.Blocks[0].HTML, "Not to be found") {
		t.Errorf("bob does not see the secret: %+v", got.Blocks)
	}
}

func TestMovingABlockThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
	p := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Plan","parent":"`+team.ID+`"}`, sameSite))
	b1 := decode[apiBlock](t, s.send(http.MethodPost, "/api/pages/"+p.ID+"/blocks", `{"md":"one"}`, sameSite))
	s.send(http.MethodPost, "/api/pages/"+p.ID+"/blocks", `{"md":"two","after":"`+b1.ID+`"}`, sameSite)
	if got := s.send(http.MethodPost, "/api/pages/"+p.ID+"/blocks/b2/move", `{"after":""}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("move = %d", got)
	}
	if got := decode[apiDocument](t, s.get("/api/doc/"+p.ID)).Blocks; len(got) != 2 || got[0].ID != "b2" {
		t.Errorf("b2 comes first: %+v", got)
	}
	if got := s.send(http.MethodPost, "/api/pages/"+p.ID+"/blocks/b2/move", `{"after":"b2"}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("after itself = %d", got)
	}
}

// The list of problems names what does not resolve in the pages of the database, for the viewer, and what the folder
// source ran into.
func TestTheListOfProblems(t *testing.T) {
	s := withDatabase(t)
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
	newPage := func(parent, title string, blocks ...string) string {
		p := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"`+title+`","parent":"`+parent+`"}`, sameSite))
		after := ""
		for _, md := range blocks {
			body, _ := json.Marshal(map[string]string{"md": md, "after": after})
			after = decode[apiBlock](t, s.send(http.MethodPost, "/api/pages/"+p.ID+"/blocks", string(body), sameSite)).ID
		}
		return p.ID
	}
	plan := newPage(team.ID, "Plan", "one", "two")
	gone := newPage(team.ID, "Gone", "::embed{ref=\"zzzzzzzzzz\"}")
	notes := ""
	for _, c := range decode[apiState](t, s.get("/api/state")).Collections {
		if c.Private {
			notes = c.ID
		}
	}
	secret := newPage(notes, "Secret", "4711")
	newPage(team.ID, "Rack",
		"::embed{ref=\""+plan+"#b1\"}\n\n[fine](/o/"+plan+")",
		"::embed{ref=\""+plan+"#b9\"}",
		"::embed{ref=\"zzzzzzzzzz\"}\n\n[nowhere](/o/zzzzzzzzzz)",
		"[[No such page]]\n\n```\n[[Inside code]]\n::embed{ref=\"zzzzzzzzzz\"}\n```",
		"::embed{ref=\""+gone+"\"}",
		"::embed{ref=\""+secret+"\"}",
		"::embed{ref=\""+team.ID+"\"}")
	if got := s.send(http.MethodDelete, "/api/objects/"+gone, "", sameSite).Code; got != http.StatusNoContent && got != http.StatusOK {
		t.Fatalf("delete = %d", got)
	}
	list := func(as map[string]string) []string {
		var p apiProblems
		res := s.get("/api/problems")
		if as != nil {
			res = s.getWith("/api/problems", as)
		}
		if err := json.Unmarshal(res.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, x := range p.Database {
			out = append(out, x.Title+" #"+x.Block+" "+x.Ref+": "+x.Why)
		}
		return out
	}
	r := func(b string) string { return "Rack #" + b + " " }
	want := []string{
		r("b2") + plan + "#b9: embeds block #b9, which is no longer in Plan",
		r("b3") + "zzzzzzzzzz: embeds something that is not to be found",
		r("b3") + "/o/zzzzzzzzzz: links to a page that is not to be found",
		r("b4") + "[[No such page]]: no such page",
		r("b5") + gone + ": embeds something that is not to be found",
		r("b7") + team.ID + ": embeds Team, which is not a page",
	}
	if got := list(nil); !slices.Equal(got, want) {
		t.Errorf("for the owner:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	var all apiProblems
	_ = json.Unmarshal(s.get("/api/problems").Body.Bytes(), &all)
	if len(all.Source) != 1 || all.Source[0].Source != "README.md" || all.Source[0].Message != "[[gone]]: no such page" || all.More {
		t.Errorf("what the folder source ran into: %+v", all.Source)
	}
	asBob := merged(bob)
	asBob["Sec-Fetch-Site"] = "same-origin"
	got := list(asBob)
	if len(got) != len(want)+1 || !slices.Contains(got, r("b6")+secret+": embeds something that is not to be found") {
		t.Errorf("for bob, the private page is not to be found:\n%s", strings.Join(got, "\n"))
	}
}

func TestTagsThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
	p := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Plan","parent":"`+team.ID+`"}`, sameSite))
	res := s.send(http.MethodPut, "/api/objects/"+p.ID+"/tags", `{"tags":["Network","#rack","network"]}`, sameSite)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"tags":["network","rack"]`) {
		t.Fatalf("set: %d %s", res.Code, res.Body.String())
	}
	if got := decode[apiDocument](t, s.get("/api/doc/"+p.ID)).Tags; !slices.Equal(got, []string{"network", "rack"}) {
		t.Errorf("the document has them: %q", got)
	}
	var found []string
	for _, c := range decode[apiState](t, s.get("/api/state")).Collections {
		for _, k := range c.Kids {
			if k.ID == p.ID {
				found = k.Tags
			}
		}
	}
	if !slices.Equal(found, []string{"network", "rack"}) {
		t.Errorf("the tree has them, so search finds by tag: %q", found)
	}
	if got := s.send(http.MethodPut, "/api/objects/"+p.ID+"/tags", `{"tags":["two words"]}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("a bad tag = %d", got)
	}
	asBob := merged(bob)
	asBob["Sec-Fetch-Site"] = "same-origin"
	s.send(http.MethodPost, "/api/objects/"+team.ID+"/access", `{"access":"read"}`, sameSite)
	if got := s.send(http.MethodPut, "/api/objects/"+p.ID+"/tags", `{"tags":["x"]}`, asBob).Code; got != http.StatusForbidden && got != http.StatusNotFound {
		t.Errorf("bob may only read = %d", got)
	}
}
