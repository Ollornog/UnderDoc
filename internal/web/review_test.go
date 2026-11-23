// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"strconv"
	"testing"
)

func blockOfDoc(doc apiDocument, id string) *apiBlock {
	for i := range doc.Blocks {
		if doc.Blocks[i].ID == id {
			return &doc.Blocks[i]
		}
	}
	return nil
}

// TestAIHintsThroughTheAPI: an agent's change of human documentation carries a hint every reader sees; a person who may
// change the page opens the diff and checks it or takes it back, and the page says who checked and when (ADR-33). The
// attack round of T-20: an agent never checks or takes back, neither directly nor through the API; what a client sends
// never makes a version an agent's or a person's; a reader neither reads the diff nor checks; a check names the open
// change of its block and nothing else; an agent may not hide its hints by changing the kind of a page people wrote.
func TestAIHintsThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	s.send(http.MethodGet, "/api/state", "", plain)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	guide := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Restore","parent":"`+team.ID+`","kind":"how-to"}`, sameSite))
	ids := addBlocks(t, s, guide.ID, "This guide brings files back.", "1. Open the backup.", "2. Copy the files.")
	blocks := "/api/pages/" + guide.ID + "/blocks/"

	// A client claims to be no agent: the version is the agent's all the same, and it carries the hint.
	if got := s.send(http.MethodPut, blocks+ids[1], `{"md":"1. Open the backup of last night.","agent":false,"ai":false,"authKind":"session"}`, asAgentOf); got.Code != http.StatusOK {
		t.Fatalf("the agent edits: %d %s", got.Code, got.Body.String())
	}
	// A person claims to be an agent: the version is the person's, and no hint comes.
	if got := s.send(http.MethodPut, blocks+ids[2], `{"md":"2. Copy the files back.","agent":true,"ai":true}`, sameSite); got.Code != http.StatusOK {
		t.Fatalf("anna edits: %d", got.Code)
	}
	versions := decode[[]apiVersion](t, s.get("/api/pages/"+guide.ID+"/versions"))
	if !versions[1].AI || versions[1].Block != ids[1] || versions[0].AI || versions[0].Block != ids[2] {
		t.Errorf("the versions say who wrote them, whatever the client sent: %+v", versions[:2])
	}
	read := decode[apiDocument](t, s.getWith("/api/doc/"+guide.ID, plain))
	hint := blockOfDoc(read, ids[1])
	if hint == nil || hint.AI == nil || hint.AI.By != "Anna Berg" || hint.AI.Added || blockOfDoc(read, ids[2]).AI != nil || blockOfDoc(read, ids[0]).AI != nil {
		t.Fatalf("a reader sees the hint on the block the agent changed, and only there: %+v", read.Blocks)
	}
	version := strconv.FormatInt(hint.AI.Version, 10)

	// The agent tries to clear its hint: by checking, taking back or changing the block again.
	if got := s.send(http.MethodPost, blocks+ids[1]+"/check", `{"version":`+version+`}`, asAgentOf).Code; got != http.StatusForbidden {
		t.Errorf("an agent checks = %d", got)
	}
	if got := s.send(http.MethodPost, blocks+ids[1]+"/takeback", `{"version":`+version+`}`, asAgentOf).Code; got != http.StatusForbidden {
		t.Errorf("an agent takes back = %d", got)
	}
	s.send(http.MethodPut, blocks+ids[1], `{"md":"1. Open the backup of tonight."}`, asAgentOf)
	again := blockOfDoc(decode[apiDocument](t, s.get("/api/doc/"+guide.ID)), ids[1])
	if again.AI == nil || again.AI.Version <= hint.AI.Version {
		t.Fatalf("a further change of the agent keeps the hint, with a newer version: %+v", again.AI)
	}
	if got := s.send(http.MethodPut, "/api/pages/"+guide.ID+"/kind", `{"kind":"reference"}`, asAgentOf).Code; got != http.StatusForbidden {
		t.Errorf("an agent moves a page of people to its side = %d", got)
	}
	if got := s.send(http.MethodDelete, blocks+ids[0], "", asAgentOf).Code; got != http.StatusNoContent {
		t.Fatalf("the agent deletes = %d", got)
	}
	doc := decode[apiDocument](t, s.getWith("/api/doc/"+guide.ID, plain))
	if len(doc.Removed) != 1 || doc.Removed[0].Block != ids[0] || doc.Removed[0].By != "Anna Berg" {
		t.Errorf("a reader sees what the agent took out: %+v", doc.Removed)
	}

	// A reader neither reads the diff nor checks.
	s.send(http.MethodPost, "/api/objects/"+guide.ID+"/access", `{"access":"read"}`, sameSite)
	s.send(http.MethodPut, "/api/objects/"+guide.ID+"/inherit", `{"inherit":false}`, sameSite)
	s.send(http.MethodPut, "/api/objects/"+guide.ID+"/grants", `{"kind":"tenant","role":"read"}`, sameSite)
	if got := s.getWith(blocks+ids[1]+"/change", plain).Code; got != http.StatusForbidden {
		t.Errorf("a reader reads the diff = %d", got)
	}
	newest := strconv.FormatInt(again.AI.Version, 10)
	if got := s.send(http.MethodPost, blocks+ids[1]+"/check", `{"version":`+newest+`}`, plain).Code; got != http.StatusForbidden {
		t.Errorf("a reader checks = %d", got)
	}

	// anna opens the diff and checks what she saw: an older version and the version of another block are refused.
	diff := decode[apiChange](t, s.get(blocks+ids[1]+"/change"))
	if diff.Before != "1. Open the backup." || diff.Now != "1. Open the backup of tonight." || diff.Version != again.AI.Version {
		t.Errorf("the diff: %+v", diff)
	}
	if got := s.send(http.MethodPost, blocks+ids[1]+"/check", `{"version":`+version+`}`, sameSite).Code; got != http.StatusConflict {
		t.Errorf("a check of an older version = %d", got)
	}
	removed := strconv.FormatInt(doc.Removed[0].Version, 10)
	if got := s.send(http.MethodPost, blocks+ids[1]+"/check", `{"version":`+removed+`}`, sameSite).Code; got != http.StatusConflict {
		t.Errorf("a check with the version of another block = %d", got)
	}
	if got := s.send(http.MethodPost, blocks+ids[1]+"/check", `{"version":`+newest+`}`, map[string]string{"Sec-Fetch-Site": "cross-site"}).Code; got != http.StatusForbidden {
		t.Errorf("a check from another site = %d", got)
	}
	if got := s.send(http.MethodPost, blocks+ids[1]+"/check", `{"version":`+newest+`}`, sameSite); got.Code != http.StatusNoContent {
		t.Fatalf("anna checks: %d %s", got.Code, got.Body.String())
	}
	doc = decode[apiDocument](t, s.getWith("/api/doc/"+guide.ID, plain))
	if blockOfDoc(doc, ids[1]).AI != nil || doc.Checked == nil || doc.Checked.By != "Anna Berg" || doc.Checked.At == "" {
		t.Errorf("the hint goes, and the page says who checked it: %+v %+v", blockOfDoc(doc, ids[1]).AI, doc.Checked)
	}
	// The deletion is taken back: the block comes back where it stood.
	if got := s.send(http.MethodPost, blocks+ids[0]+"/takeback", `{"version":`+removed+`}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("anna takes the deletion back = %d", got)
	}
	doc = decode[apiDocument](t, s.get("/api/doc/"+guide.ID))
	if len(doc.Removed) != 0 || doc.Blocks[0].ID != ids[0] || doc.Blocks[0].Markdown != "This guide brings files back." {
		t.Errorf("the block is back at the top: %+v %+v", doc.Removed, doc.Blocks)
	}
	entries := decode[[]apiEntry](t, s.get("/api/log"))
	checked, taken := false, false
	for _, e := range entries {
		checked = checked || (e.Action == "checked" && e.Object == guide.ID)
		taken = taken || (e.Action == "taken back" && e.Object == guide.ID)
	}
	if !checked || !taken {
		t.Errorf("the log keeps the check and the taking back: %+v", entries)
	}

	// The agent side shows once that AI maintains it, and no hint per change.
	ref := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Network","parent":"`+team.ID+`","kind":"reference"}`, sameSite))
	refBlocks := addBlocks(t, s, ref.ID, "## Hosts")
	s.send(http.MethodPut, "/api/pages/"+ref.ID+"/blocks/"+refBlocks[0], `{"md":"## Hosts\n\nbackup.example.com"}`, asAgentOf)
	refDoc := decode[apiDocument](t, s.get("/api/doc/"+ref.ID))
	if !refDoc.MaintainedByAI || refDoc.Blocks[0].AI != nil || decode[apiDocument](t, s.get("/api/doc/"+guide.ID)).MaintainedByAI {
		t.Errorf("maintained by AI on the agent side only, without hints: %v %+v", refDoc.MaintainedByAI, refDoc.Blocks[0].AI)
	}
}

// TestAIChangesToCheckInTheSearch: the filter "AI changes to check" finds the pages of human documentation with open
// hints that the reader may read, with and without words, and nothing once they are checked.
func TestAIChangesToCheckInTheSearch(t *testing.T) {
	s := withSearch(t, meiliEngine(t))
	s.send(http.MethodGet, "/api/state", "", bob)
	team := s.sharedCollection(`{"title":"Filiale"}`, sameSite)
	open := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kasse öffnen","parent":"`+team.ID+`","kind":"how-to"}`, sameSite))
	openBlocks := addBlocks(t, s.site, open.ID, "Die Kasse wird morgens geöffnet.")
	s.send(http.MethodPut, "/api/pages/"+open.ID+"/blocks/"+openBlocks[0], `{"md":"Die Kasse wird morgens um acht geöffnet."}`, asAgentOf)
	done := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kasse schließen","parent":"`+team.ID+`","kind":"how-to"}`, sameSite))
	addBlocks(t, s.site, done.ID, "Die Kasse wird abends geschlossen.")
	notes := decode[apiState](t, s.get("/api/state")).Collections[0].ID
	private := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kasse privat","parent":"`+notes+`"}`, sameSite))
	privateBlocks := addBlocks(t, s.site, private.ID, "Nur für mich.")
	s.send(http.MethodPut, "/api/pages/"+private.ID+"/blocks/"+privateBlocks[0], `{"md":"Nur für mich, vom Agenten."}`, asAgentOf)
	s.drain()

	ids := func(r apiSearchResult) []string {
		var out []string
		for _, h := range r.Hits {
			out = append(out, h.ID)
		}
		return out
	}
	if got := ids(s.find("q=Kasse&check=ai", nil)); len(got) != 2 || !contains(got, open.ID) || !contains(got, private.ID) {
		t.Errorf("anna finds both pages with open hints: %v", got)
	}
	if got := ids(s.find("q=&check=ai", bob)); len(got) != 1 || got[0] != open.ID {
		t.Errorf("bob finds only what he reads, without words: %v", got)
	}
	if got := ids(s.find("q=Kasse", bob)); len(got) != 2 {
		t.Errorf("without the filter both pages of the team: %v", got)
	}
	doc := decode[apiDocument](t, s.get("/api/doc/"+open.ID))
	if got := s.send(http.MethodPost, "/api/pages/"+open.ID+"/blocks/"+openBlocks[0]+"/check", `{"version":`+strconv.FormatInt(doc.Blocks[0].AI.Version, 10)+`}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("check = %d", got)
	}
	if got := s.find("q=&check=ai", bob); len(got.Hits) != 0 || got.Hits == nil {
		t.Errorf("nothing left to check for bob: %+v", got)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// TestChecksInPrivateNotesStayOutOfTheLog: what lies in private notes is nobody's business, also not its checks.
func TestChecksInPrivateNotesStayOutOfTheLog(t *testing.T) {
	s := withDatabase(t)
	notes := decode[apiState](t, s.get("/api/state")).Collections[0].ID
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Mine","parent":"`+notes+`"}`, sameSite))
	ids := addBlocks(t, s, page.ID, "Text.")
	s.send(http.MethodPut, "/api/pages/"+page.ID+"/blocks/"+ids[0], `{"md":"Text by the agent."}`, asAgentOf)
	doc := decode[apiDocument](t, s.get("/api/doc/"+page.ID))
	if doc.Blocks[0].AI == nil {
		t.Fatal("the hint in private notes")
	}
	if got := s.send(http.MethodPost, "/api/pages/"+page.ID+"/blocks/"+ids[0]+"/check", `{"version":`+strconv.FormatInt(doc.Blocks[0].AI.Version, 10)+`}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("check = %d", got)
	}
	for _, e := range decode[[]apiEntry](t, s.get("/api/log")) {
		if e.Object == page.ID {
			t.Errorf("a check in private notes is logged: %+v", e)
		}
	}
}
