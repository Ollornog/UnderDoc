// SPDX-License-Identifier: MIT

package web

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// The end of something in use (ADR-22): deleting asks with every place of use, and each of the three ways does what it
// says: everywhere leaves a placeholder that restoring takes away, copies writes copies where the viewer may and notices
// elsewhere, a new master takes the content in place of its embedding and every other reference follows, with a redirect.
func TestTheEndOfSomethingInUse(t *testing.T) {
	s := withDatabase(t)
	asBob := merged(bob)
	asBob["Sec-Fetch-Site"] = "same-origin"
	s.getWith("/api/state", asBob) // bob has an account
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
	bobs := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Bob's"}`, asBob))
	s.send(http.MethodPost, "/api/objects/"+bobs.ID+"/access", `{"access":"read"}`, asBob)
	s.send(http.MethodPost, "/api/objects/"+team.ID+"/access", `{"access":"read"}`, sameSite)
	newPage := func(parent string, as map[string]string, title string, blocks ...string) string {
		t.Helper()
		p := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"`+title+`","parent":"`+parent+`"}`, as))
		after := ""
		for _, md := range blocks {
			body, _ := json.Marshal(map[string]string{"md": md, "after": after})
			after = decode[apiBlock](t, s.send(http.MethodPost, "/api/pages/"+p.ID+"/blocks", string(body), as)).ID
		}
		return p.ID
	}
	blocks := func(id string) []apiBlock { return decode[apiDocument](t, s.get("/api/doc/"+id)).Blocks }
	html := func(id string) string {
		var out strings.Builder
		for _, b := range blocks(id) {
			out.WriteString(b.HTML)
		}
		return out.String()
	}
	md := func(id string) []string {
		var out []string
		for _, b := range blocks(id) {
			out = append(out, b.ID+":"+b.Markdown)
		}
		return out
	}

	src := newPage(team.ID, sameSite, "Errors", "Error 42: <!-- part t1 -->the disk is full<!-- /part t1 -->.", "Error 43: the network is down.", "## Disk", "Check it.", "## Other")
	runbook := newPage(team.ID, sameSite, "Runbook", "If :embed{ref=\""+src+"#b1.t1\"}, free space.", "::embed{ref=\""+src+"#b2\"}")
	other := newPage(team.ID, sameSite, "Other", "::embed{ref=\""+src+"#b2\"}", "Also: :embed{ref=\""+src+"#b1.t1\"}.")
	foreign := newPage(bobs.ID, asBob, "Bob's notes", "::embed{ref=\""+src+"#b2\"}")

	// Without a way, deleting asks: 409, with every place of use and whether the viewer may write there.
	res := s.send(http.MethodDelete, "/api/pages/"+src+"/blocks/b2", "", sameSite)
	var asked struct {
		InUse []apiInUse `json:"inUse"`
	}
	_ = json.Unmarshal(res.Body.Bytes(), &asked)
	if res.Code != http.StatusConflict || len(asked.InUse) != 1 || asked.InUse[0].Ref != src+"#b2" || len(asked.InUse[0].Uses) != 3 {
		t.Fatalf("deleting a block in use without a way = %d %s", res.Code, res.Body.String())
	}
	var mayWrite []string
	for _, u := range asked.InUse[0].Uses {
		if u.CanEdit {
			mayWrite = append(mayWrite, u.Title)
		}
	}
	if !slices.Equal(mayWrite, []string{"Other", "Runbook"}) {
		t.Errorf("the places Anna may write to: %q", mayWrite)
	}
	if got := md(src); len(got) != 5 {
		t.Errorf("nothing changed while asking: %q", got)
	}
	if got := s.send(http.MethodDelete, "/api/pages/"+src+"/blocks/b2", `{"ways":{"*":{"way":"sideways"}}}`, sameSite).Code; got != http.StatusConflict {
		t.Errorf("an unknown way = %d", got)
	}

	// Copies: every use Anna may change gets the text, in its place; Bob's page keeps a placeholder, Bob gets a notice and
	// may keep a copy himself.
	if got := s.send(http.MethodDelete, "/api/pages/"+src+"/blocks/b2", `{"ways":{"*":{"way":"copies"}}}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("copies = %d", got)
	}
	if got := md(runbook)[1]; got != "b2:Error 43: the network is down." {
		t.Errorf("the copy in Runbook: %q", got)
	}
	if got := md(other)[0]; got != "b1:Error 43: the network is down." {
		t.Errorf("the copy in Other: %q", got)
	}
	var versions []apiVersion
	_ = json.Unmarshal(s.get("/api/pages/"+runbook+"/versions").Body.Bytes(), &versions)
	if len(versions) == 0 || versions[0].Origin != "copy of "+src+"#b2" {
		t.Errorf("the version history says where the copy came from: %+v", versions)
	}
	if got := decode[apiDocument](t, s.getWith("/api/doc/"+foreign, asBob)).Blocks[0].HTML; !strings.Contains(got, "Removed by Anna Berg on") || !strings.Contains(got, `data-act="keepcopy"`) || strings.Contains(got, `data-act="restoreremoved"`) {
		t.Errorf("Bob's page shows a placeholder, and Bob may keep a copy but not restore: %s", got)
	}
	var notices []apiNotice
	_ = json.Unmarshal(s.getWith("/api/notices", asBob).Body.Bytes(), &notices)
	if len(notices) != 1 || notices[0].Kind != "copy" || notices[0].Page != foreign || notices[0].Ref != src+"#b2" {
		t.Errorf("Bob's notice: %+v", notices)
	}
	removed := ""
	if got := decode[apiDocument](t, s.getWith("/api/doc/"+foreign, asBob)).Blocks[0].HTML; strings.Contains(got, `data-id="`) {
		removed = strings.SplitN(strings.SplitN(got, `data-act="keepcopy" data-id="`, 2)[1], `"`, 2)[0]
	}
	if got := s.send(http.MethodPost, "/api/removed/"+removed+"/copy", `{"page":"`+foreign+`"}`, asBob).Code; got != http.StatusNoContent {
		t.Fatalf("Bob keeps a copy = %d", got)
	}
	if got := decode[apiDocument](t, s.getWith("/api/doc/"+foreign, asBob)).Blocks[0].Markdown; got != "Error 43: the network is down." {
		t.Errorf("the copy in Bob's page: %q", got)
	}

	// Everywhere: the part goes, every use shows who removed it, and restoring brings every use back.
	res = s.send(http.MethodPut, "/api/pages/"+src+"/blocks/b1", `{"md":"Error 42."}`, sameSite)
	_ = json.Unmarshal(res.Body.Bytes(), &asked)
	if res.Code != http.StatusConflict || len(asked.InUse) != 1 || asked.InUse[0].Ref != src+"#b1.t1" || len(asked.InUse[0].Uses) != 2 {
		t.Fatalf("taking the text of a part in use without a way = %d %s", res.Code, res.Body.String())
	}
	if got := s.send(http.MethodPut, "/api/pages/"+src+"/blocks/b1", `{"md":"Error 42.","ways":{"b1.t1":{"way":"everywhere"}}}`, sameSite).Code; got != http.StatusOK {
		t.Fatalf("everywhere = %d", got)
	}
	got := html(runbook)
	if !strings.Contains(got, "Removed by Anna Berg on") || !strings.Contains(got, "from Errors") || !strings.Contains(got, `data-act="restoreremoved"`) {
		t.Fatalf("the placeholder in Runbook: %s", got)
	}
	id := strings.SplitN(strings.SplitN(got, `data-act="restoreremoved" data-id="`, 2)[1], `"`, 2)[0]
	if got := s.send(http.MethodPost, "/api/removed/"+id+"/restore", "", asBob).Code; got != http.StatusForbidden && got != http.StatusNotFound {
		t.Errorf("Bob restores in Anna's page = %d", got)
	}
	if got := s.send(http.MethodPost, "/api/removed/"+id+"/restore", "", sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("restore = %d", got)
	}
	if got := s.send(http.MethodPost, "/api/removed/"+id+"/restore", "", sameSite).Code; got != http.StatusNotFound {
		t.Errorf("restoring twice = %d", got)
	}
	if got := html(runbook); !strings.Contains(got, `title="Errors · #b1.t1">the disk is full</span>`) {
		t.Errorf("restoring brings every use back: %s", got)
	}

	// A new master: the part moves into Runbook in place of its embedding; Other follows it, and the old address redirects.
	if got := s.send(http.MethodPut, "/api/pages/"+src+"/blocks/b1", `{"md":"Error 42.","ways":{"*":{"way":"master","master":"`+other+`#b1"}}}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("a master that is no use of the part = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/pages/"+src+"/blocks/b1", `{"md":"Error 42.","ways":{"*":{"way":"master","master":"`+runbook+`#b1"}}}`, sameSite).Code; got != http.StatusOK {
		t.Fatalf("new master = %d", got)
	}
	if got := md(runbook)[0]; got != "b1:If <!-- part t1 -->the disk is full<!-- /part t1 -->, free space." {
		t.Errorf("the part lives in Runbook now: %q", got)
	}
	if got := md(other)[1]; got != `b2:Also: :embed{ref="`+runbook+`#b1.t1"}.` {
		t.Errorf("Other follows it: %q", got)
	}
	if got := html(other); !strings.Contains(got, `title="Runbook · #b1.t1">the disk is full</span>`) {
		t.Errorf("Other shows it from its new place: %s", got)
	}
	var redirect struct{ Ref, URL string }
	_ = json.Unmarshal(s.get("/api/redirect?ref="+src+"%23b1.t1").Body.Bytes(), &redirect)
	if redirect.Ref != runbook+"#b1.t1" || redirect.URL != "/o/"+runbook+"#b1.t1" {
		t.Errorf("the old address redirects: %+v", redirect)
	}
	stale := newPage(team.ID, sameSite, "Stale", "::embed{ref=\""+src+"#b1.t1\"}")
	if got := html(stale); !strings.Contains(got, "the disk is full") {
		t.Errorf("an embedding written later with the old address follows the redirect: %s", got)
	}

	// A section with a new master: the heading and its blocks move into the page that embeds it.
	sectioned := newPage(team.ID, sameSite, "Sectioned", "Before.", "::embed{ref=\""+src+"#b3.section\"}")
	if got := s.send(http.MethodDelete, "/api/pages/"+src+"/blocks/b3", `{"ways":{"*":{"way":"master","master":"`+sectioned+`#b2"}}}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("section master = %d", got)
	}
	if got := md(sectioned); !slices.Equal(got, []string{"b1:Before.", "b2:## Disk", "b3:Check it."}) {
		t.Errorf("the section moved into Sectioned: %q", got)
	}
	if got := md(src); !slices.Equal(got, []string{"b1:Error 42.", "b5:## Other"}) {
		t.Errorf("and left Errors: %q", got)
	}

	// Deleting a whole page asks for what of it is used outside; everywhere puts it into the trash with a placeholder.
	res = s.send(http.MethodDelete, "/api/objects/"+runbook, "", sameSite)
	_ = json.Unmarshal(res.Body.Bytes(), &asked)
	if res.Code != http.StatusConflict || len(asked.InUse) != 1 || asked.InUse[0].Ref != runbook+"#b1.t1" {
		t.Fatalf("deleting a page with a part in use = %d %s", res.Code, res.Body.String())
	}
	if got := s.send(http.MethodDelete, "/api/objects/"+runbook, `{"ways":{"*":{"way":"everywhere"}}}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("delete everywhere = %d", got)
	}
	if got := html(other); !strings.Contains(got, "Removed by Anna Berg on") || !strings.Contains(got, "in the trash") {
		t.Errorf("the placeholder of a page in the trash: %s", got)
	}
	if got := decode[apiDocument](t, s.getWith("/api/doc/"+foreign, asBob)).Blocks[0].HTML; strings.Contains(got, "Removed") {
		t.Errorf("Bob's page no longer embeds anything: %s", got)
	}

	// Of a page Bob may not see, he learns nothing when it goes into the trash: not who, not that it was there.
	notes := ""
	for _, c := range decode[apiState](t, s.get("/api/state")).Collections {
		if c.Private {
			notes = c.ID
		}
	}
	secret := newPage(notes, sameSite, "Secret", "4711")
	peek := newPage(bobs.ID, asBob, "Peek", "::embed{ref=\""+secret+"\"}")
	if got := s.send(http.MethodDelete, "/api/objects/"+secret, `{"ways":{"*":{"way":"everywhere"}}}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("delete the secret = %d", got)
	}
	if got := decode[apiDocument](t, s.getWith("/api/doc/"+peek, asBob)).Blocks[0].HTML; strings.Contains(got, "Removed") || !strings.Contains(got, "Not to be found") {
		t.Errorf("Bob sees of Anna's private page in the trash: %s", got)
	}
}
