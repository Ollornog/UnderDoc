// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// The review of B-4 and the MCP core (B-5) through the API and MCP. Each test is the attack round on the door it closes.

// Attack (finding 5): a key, an app password or OAuth of a person with the role to create spaces makes a space, or the
// workspace offers one to it.
func TestAgentsMakeNoSpaceThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	withRole := merged2(plain, map[string]string{"Remote-Groups": "underdoc-spaces"})
	for _, auth := range []string{"apikey", "app-password", "oauth"} {
		agent := merged2(withRole, map[string]string{"Remote-Auth": auth})
		if state := decode[apiState](t, s.getWith("/api/state", agent)); state.CanSpaces {
			t.Errorf("%s: the workspace offers a space", auth)
		}
		for name, call := range map[string][2]string{
			"make":       {"/api/collections", `{"title":"By ` + auth + `"}`},
			"make files": {"/api/files/collections", `{"title":"Files by ` + auth + `"}`},
		} {
			if got := s.send(http.MethodPost, call[0], call[1], agent).Code; got != http.StatusForbidden && got != http.StatusNotImplemented {
				t.Errorf("%s with the role: %s = %d", auth, name, got)
			}
		}
	}
	if got := s.send(http.MethodPost, "/api/collections", `{"title":"In the browser"}`, withRole).Code; got != http.StatusCreated {
		t.Errorf("the person with the role in the browser = %d", got)
	}
}

// Finding 3: the counts beside the hits come from the index, so they count beyond the hits shown; only for a reader whose
// tokens drop out (a service account, someone in an archived group) they come from the checked hits, and then they tell
// nothing that reader may not find.
func TestSearchCountsBeyondTheFirstPage(t *testing.T) {
	s := withSearch(t, meiliEngine(t))
	s.send(http.MethodGet, "/api/state", "", plain)
	team := s.sharedCollection(`{"title":"Filiale"}`, sameSite)
	for i := range 25 {
		page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kasse `+string(rune('A'+i))+`","parent":"`+team.ID+`"}`, sameSite))
		addBlocks(t, s.site, page.ID, "Die Kartenzahlung beginnt mit der Taste Karte.")
	}
	secret := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Abschluss","parent":"`+team.ID+`"}`, sameSite))
	addBlocks(t, s.site, secret.ID, "Die Kartenzahlung des Tages wird abgerechnet.")
	group := decode[apiGroup](t, s.send(http.MethodPost, "/api/groups", `{"name":"Aushilfen"}`, sameSite))
	for _, step := range [][3]string{
		{http.MethodPut, "/api/groups/" + group.ID + "/members/1002", `{"member":true}`},
		{http.MethodPut, "/api/objects/" + secret.ID + "/grants", `{"kind":"group","who":"` + group.ID + `","actions":{"read":"deny"}}`},
	} {
		if got := s.send(step[0], step[1], step[2], sameSite).Code; got != http.StatusNoContent {
			t.Fatalf("%s %s = %d", step[0], step[1], got)
		}
	}
	s.drain()
	count := func(result apiSearchResult) int {
		for _, f := range result.Facets["type"] {
			if f.Value == "page" {
				return f.Count
			}
		}
		return 0
	}
	got := s.find("q=Kartenzahlung&limit=5", plain)
	if len(got.Hits) != 5 || count(got) != 25 {
		t.Errorf("bob sees 5 hits and counts 25 pages: %d hits, %d counted", len(got.Hits), count(got))
	}
	// A page of a collection only the group reads: archived, the group's allow holds for nobody, but the index still
	// takes bob's token of the group, so it would count the page; the counts come from the checked hits.
	inner := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Intern"}`, sameSite))
	only := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Nur Aushilfen","parent":"`+inner.ID+`"}`, sameSite))
	addBlocks(t, s.site, only.ID, "Die Kartenzahlung der Aushilfen.")
	if code := s.send(http.MethodPut, "/api/objects/"+inner.ID+"/grants", `{"kind":"group","who":"`+group.ID+`","role":"read"}`, sameSite).Code; code != http.StatusNoContent {
		t.Fatalf("read for the group = %d", code)
	}
	s.drain()
	if got := s.find("q=Kartenzahlung&limit=50", plain); len(got.Hits) != 26 || count(got) != 26 {
		t.Errorf("bob in the group: %d hits, %d counted, want 26", len(got.Hits), count(got))
	}
	if code := s.send(http.MethodDelete, "/api/groups/"+group.ID, "", sameSite).Code; code != http.StatusNoContent {
		t.Fatalf("archive = %d", code)
	}
	got = s.find("q=Kartenzahlung&limit=50", plain)
	if len(got.Hits) != 25 || count(got) != 25 {
		t.Errorf("bob in an archived group: %d hits, %d counted, want 25 and 25", len(got.Hits), count(got))
	}
}

// Finding 3 for a service account: its token for everyone drops out of the index, so a deny for everyone does not reach
// the index; its counts come from the checked hits.
func TestServiceCountsLeaveOutWhatEveryoneIsDenied(t *testing.T) {
	s := withSearch(t, meiliEngine(t), gateOf(t, account.GateConfig{ServiceAccounts: []string{"4242"}}))
	service := merged2(serviceWithRoles, map[string]string{"Remote-Groups": ""})
	s.send(http.MethodGet, "/api/state", "", service)
	team := s.sharedCollection(`{"title":"Filiale"}`, sameSite)
	if got := s.send(http.MethodPut, "/api/objects/"+team.ID+"/grants", `{"kind":"person","who":"4242","role":"read"}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("read for the service account = %d", got)
	}
	open := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kassenhandbuch","parent":"`+team.ID+`"}`, sameSite))
	closed := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kassenabschluss","parent":"`+team.ID+`"}`, sameSite))
	addBlocks(t, s.site, open.ID, "Die Kartenzahlung beginnt mit der Taste Karte.")
	addBlocks(t, s.site, closed.ID, "Die Kartenzahlung des Tages wird abgerechnet.")
	if got := s.send(http.MethodPut, "/api/objects/"+closed.ID+"/grants", `{"kind":"tenant","actions":{"read":"deny"}}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("deny = %d", got)
	}
	s.drain()
	got := s.find("q=Kartenzahlung", service)
	if len(got.Hits) != 1 || got.Hits[0].ID != open.ID {
		t.Fatalf("the service account finds: %+v", got.Hits)
	}
	for _, f := range got.Facets["type"] {
		if f.Count != 1 {
			t.Errorf("the count tells of the page denied to everyone: %+v", f)
		}
	}
}

// Finding 10: the total says "100 or more" when the search reached its limit, also where the check left hits out; the
// check is one query. The index lags behind a deny until its queue is drained, so it still gives bob the pages denied to
// him, and the check takes them out again.
func TestMCPSearchTotalAfterTheCheck(t *testing.T) {
	s := withSearch(t, meiliEngine(t))
	s.send(http.MethodGet, "/api/state", "", plain)
	team := s.sharedCollection(`{"title":"Filiale"}`, sameSite)
	var pages []string
	for i := range 100 {
		pages = append(pages, decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kartenzahlung `+strconv.Itoa(i)+`","parent":"`+team.ID+`"}`, sameSite)).ID)
	}
	s.drain()
	for _, page := range pages[:3] {
		if got := s.send(http.MethodPut, "/api/objects/"+page+"/grants", `{"kind":"person","who":"1002","actions":{"read":"deny"}}`, sameSite).Code; got != http.StatusNoContent {
			t.Fatalf("deny = %d", got)
		}
	}
	got, failed := call(t, mcpSession(t, s.http, bobsAgent), "search", map[string]any{"query": "Kartenzahlung"})
	if failed || !strings.HasSuffix(got, "20 of 100 or more found.") {
		t.Errorf("the total after the check: %s", got[max(0, len(got)-80):])
	}
	for _, page := range pages[:3] {
		if strings.Contains(got, page) {
			t.Errorf("a page denied to bob is listed: %s", page)
		}
	}
}

// Findings 6, 7 and 8 through MCP: a moved block names its new page only to whoever sees it; a block goes after a block
// named alone (b3); the line for the shell runs nothing of the file's name and quotes the link.
func TestMCPRedirectsMovesAndTheUploadLine(t *testing.T) {
	s := withFiles(t)
	s.send(http.MethodGet, "/api/state", "", plain)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	from := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kasse","parent":"`+team.ID+`"}`, sameSite))
	to := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Geheim","parent":"`+team.ID+`"}`, sameSite))
	moved := addBlocks(t, s.site, from.ID, "Bleibt.", "Wandert.")
	there := addBlocks(t, s.site, to.ID, "Erster.", "Zweiter.")
	if got := s.send(http.MethodPut, "/api/objects/"+to.ID+"/grants", `{"kind":"person","who":"1002","actions":{"read":"deny"}}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("deny = %d", got)
	}
	annas := mcpSession(t, s.http, annasAgent)
	// Finding 7: after a block named alone.
	if got, failed := call(t, annas, "move", map[string]any{"object": "underdoc:" + from.ID + "#" + moved[1], "to": to.ID, "after": there[0]}); failed {
		t.Fatalf("move after a bare block: %s", got)
	}
	doc := decode[apiDocument](t, s.get("/api/doc/"+to.ID))
	if len(doc.Blocks) != 3 || doc.Blocks[0].Markdown != "Erster." || doc.Blocks[1].Markdown != "Wandert." {
		t.Errorf("the block is not after the first: %+v", doc.Blocks)
	}
	// Finding 6: bob's agent reads the old address and learns nothing of Geheim.
	got, _ := call(t, mcpSession(t, s.http, bobsAgent), "read", map[string]any{"pages": []string{"underdoc:" + from.ID + "#" + moved[1]}})
	if strings.Contains(got, to.ID) || !strings.Contains(got, "no such block") {
		t.Errorf("the redirect names a page bob may not see: %s", got)
	}
	if got := s.getWith("/api/redirect?ref="+from.ID+"%23"+moved[1], merged(plain)); got.Code != http.StatusNotFound || strings.Contains(got.Body.String(), to.ID) {
		t.Errorf("the redirect of the API for bob = %d %s", got.Code, got.Body.String())
	}
	if got, _ := call(t, annas, "read", map[string]any{"pages": []string{"underdoc:" + from.ID + "#" + moved[1]}}); !strings.Contains(got, to.ID) {
		t.Errorf("anna's agent is not led on: %s", got)
	}
	// Finding 8: a name with $( ) and backticks never stands in the line for the shell.
	mine := s.root(nil, "user")
	name := "x$(touch pwned)`id`.txt"
	got, failed := call(t, annas, "upload_link", map[string]any{"in": mine.ID, "name": name, "size": 3})
	link := uploadLink.FindString(got)
	if failed || link == "" {
		t.Fatalf("upload_link: %s", got)
	}
	line := ""
	for _, l := range strings.Split(got, "\n") {
		if strings.HasPrefix(l, "curl ") {
			line = l
		}
	}
	if line != "curl -fsS -T <local-file> '"+link+"'" {
		t.Errorf("the line for the shell: %q", line)
	}
	if got := shellQuote("a'b"); got != `'a'\''b'` {
		t.Errorf("a quote inside: %s", got)
	}
}

// Finding 14: the tree of context stops at about 50 entries and points to under.
func TestMCPContextStopsEarly(t *testing.T) {
	s := withDatabase(t)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	for i := range 60 {
		s.send(http.MethodPost, "/api/pages", `{"title":"Seite `+strconv.Itoa(i)+`","parent":"`+team.ID+`"}`, sameSite)
	}
	got, _ := call(t, mcpSession(t, s.http, annasAgent), "context", nil)
	if n := strings.Count(got, "\n- ") + strings.Count(got, "\n  - "); n > 50 || !strings.Contains(got, "more entries; ask with under") {
		t.Errorf("context lists %d entries:\n%s", n, got[max(0, len(got)-200):])
	}
}

// Finding 1 through the API: only an administrator in the browser names the rules page.
func TestRulesPageThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	s.send(http.MethodGet, "/api/state", "", plain)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Rules","parent":"`+team.ID+`","kind":"agent-notes"}`, sameSite))
	body := `{"page":"` + page.ID + `"}`
	for name, headers := range map[string]map[string]string{
		"bob":              plain,
		"an agent of anna": merged2(sameSite, map[string]string{"Remote-Auth": "oauth"}),
		"a key of anna":    merged2(sameSite, map[string]string{"Remote-Auth": "apikey"}),
		"another site":     {"Sec-Fetch-Site": "cross-site"},
	} {
		if got := s.send(http.MethodPut, "/api/rules-page", body, headers).Code; got != http.StatusForbidden {
			t.Errorf("%s names the rules page = %d", name, got)
		}
	}
	if got := s.send(http.MethodPut, "/api/rules-page", body, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("an administrator names it = %d", got)
	}
	if got := decode[map[string]string](t, s.getWith("/api/rules-page", merged(plain))); got["page"] != page.ID {
		t.Errorf("bob reads the rules page: %v", got)
	}
	if doc := decode[apiDocument](t, s.get("/api/doc/"+page.ID)); !doc.RulesPage {
		t.Error("the page does not say it is the rules page")
	}
}
