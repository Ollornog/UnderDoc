// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// The rights of ADR-36 through every door: the API, embedding, search and the sign-in of agents.

// asAgentOf is a request of anna's agent: the gate says it signed in with OAuth, not in a session.
var asAgentOf = map[string]string{"Remote-Auth": "oauth", "Sec-Fetch-Site": "same-origin"}

// TestEntriesThroughTheAPI: entries per action are set and read back with their preset, a deny holds in the tree and
// the page, and an embedding of something denied is "not to be found" (ADR-36, ADR-18).
func TestEntriesThroughTheAPI(t *testing.T) {
	s, _ := adminSite(t)
	s.send(http.MethodGet, "/api/state", "", plain)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	secret := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Salaries","parent":"`+team.ID+`"}`, sameSite))
	addBlocks(t, s, secret.ID, "Anna 100, Bob 90.")
	rack := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Rack","parent":"`+team.ID+`"}`, sameSite))
	addBlocks(t, s, rack.ID, "Before.\n\n::embed{ref=\""+secret.ID+"\"}\n\nAfter.")
	if got := s.send(http.MethodPut, "/api/objects/"+secret.ID+"/grants", `{"kind":"person","who":"1002","actions":{"read":"deny","write":"deny"}}`, sameSite); got.Code != http.StatusNoContent {
		t.Fatalf("deny for bob: %d %s", got.Code, got.Body.String())
	}
	if got := s.send(http.MethodPut, "/api/objects/"+secret.ID+"/grants", `{"kind":"person","who":"1002","actions":{"read":"perhaps"}}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("an unknown value = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/objects/"+secret.ID+"/grants", `{"kind":"agents","actions":{"read":"allow"}}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("an allow for Agents = %d", got)
	}
	rights := decode[apiRights](t, s.get("/api/objects/"+secret.ID+"/rights"))
	if len(rights.Own) != 1 || rights.Own[0].Role != db.PresetCustom || rights.Own[0].Actions["read"] != "deny" || rights.Own[0].Actions["write"] != "deny" {
		t.Errorf("the entries of bob read back as custom: %+v", rights.Own)
	}
	if got := s.send(http.MethodGet, "/api/doc/"+secret.ID, "", plain).Code; got != http.StatusNotFound {
		t.Errorf("bob opens the page denied to him = %d", got)
	}
	state := decode[apiState](t, s.getWith("/api/state", plain))
	if findNode(state.Collections, team.ID) == nil || findNode(state.Collections, secret.ID) != nil {
		t.Errorf("bob sees Team in the tree, not the page denied to him: %+v", state.Collections)
	}
	var html strings.Builder
	for _, b := range decode[apiDocument](t, s.getWith("/api/doc/"+rack.ID, plain)).Blocks {
		html.WriteString(b.HTML)
	}
	if got := html.String(); strings.Contains(got, "Bob 90") || !strings.Contains(got, "Not to be found") || !strings.Contains(got, "After.") {
		t.Errorf("the embedding of the denied page is not to be found for bob: %s", got)
	}
	html.Reset()
	for _, b := range decode[apiDocument](t, s.get("/api/doc/"+rack.ID)).Blocks {
		html.WriteString(b.HTML)
	}
	if !strings.Contains(html.String(), "Bob 90") {
		t.Errorf("anna sees what she embedded: %s", html.String())
	}
	// The access check names the deciding entry.
	check := decode[struct {
		Actions []apiDecision `json:"actions"`
	}](t, s.get("/api/rights/check?object="+secret.ID+"&account=1002"))
	if len(check.Actions) == 0 || check.Actions[0].Action != "read" || check.Actions[0].Allowed || check.Actions[0].Value != "deny" ||
		check.Actions[0].Object != secret.ID || check.Actions[0].Name != "Bob" {
		t.Errorf("the check names bob's deny on the page: %+v", check.Actions)
	}
	if check.Actions[2].Action != "create" || !check.Actions[2].Allowed || check.Actions[2].Kind != "tenant" || check.Actions[2].Object != team.ID {
		t.Errorf("create comes from everyone on Team: %+v", check.Actions[2])
	}
}

// TestAgentsThroughTheGate: a request whose sign-in at the gate is not a session is an agent: it reads and writes as its
// person, never changes rights or makes links, and stays out of a space with no agents. A request without Remote-Auth
// counts as an agent too (ADR-36).
func TestAgentsThroughTheGate(t *testing.T) {
	s, _ := adminSite(t)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Plan","parent":"`+team.ID+`"}`, sameSite))
	if got := s.send(http.MethodGet, "/api/doc/"+page.ID, "", asAgentOf).Code; got != http.StatusOK {
		t.Errorf("anna's agent reads = %d", got)
	}
	if got := s.send(http.MethodPost, "/api/pages/"+page.ID+"/blocks", `{"md":"from the agent"}`, asAgentOf).Code; got != http.StatusCreated && got != http.StatusOK {
		t.Errorf("anna's agent writes = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/objects/"+page.ID+"/grants", `{"kind":"tenant","role":"read"}`, asAgentOf).Code; got != http.StatusForbidden {
		t.Errorf("anna's agent changes rights = %d", got)
	}
	if got := s.send(http.MethodPost, "/api/objects/"+page.ID+"/links", `{"mode":"read"}`, asAgentOf).Code; got != http.StatusForbidden {
		t.Errorf("anna's agent makes a link = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/me/agents", `{"actions":["read","write","create","comment","delete"]}`, asAgentOf).Code; got != http.StatusForbidden {
		t.Errorf("an agent widens its own limit = %d", got)
	}
	noAuth := map[string]string{"Remote-Auth": "", "Sec-Fetch-Site": "same-origin"}
	if got := s.send(http.MethodPut, "/api/objects/"+page.ID+"/grants", `{"kind":"tenant","role":"read"}`, noAuth).Code; got != http.StatusForbidden {
		t.Errorf("without Remote-Auth the request is an agent = %d", got)
	}
	rights := decode[apiRights](t, s.getWith("/api/objects/"+page.ID+"/rights", merged(asAgentOf)))
	if rights.Manage || slices.Contains(rights.Actions, "share") || !slices.Contains(rights.Actions, "write") {
		t.Errorf("the agent's rights: %+v", rights)
	}
	// An agent of an administrator administers nothing.
	if got := s.send(http.MethodGet, "/api/users", "", asAgentOf).Code; got != http.StatusForbidden {
		t.Errorf("an administrator's agent lists the users = %d", got)
	}
	if got := s.send(http.MethodPost, "/api/groups", `{"name":"By an agent"}`, asAgentOf).Code; got != http.StatusForbidden {
		t.Errorf("an administrator's agent makes a group = %d", got)
	}
	// An agent never takes a link to edit for its person: bob's agent opens it, and bob still only reads.
	s.send(http.MethodGet, "/api/state", "", plain)
	s.send(http.MethodPost, "/api/objects/"+team.ID+"/access", `{"access":"read"}`, sameSite)
	link := decode[apiLink](t, s.send(http.MethodPost, "/api/objects/"+team.ID+"/links", `{"mode":"edit"}`, sameSite))
	bobsAgent := map[string]string{"Remote-Auth": "apikey", "Remote-User": "bob", "Remote-Id": "1002", "Remote-Name": "Bob", "Remote-Groups": "", "Sec-Fetch-Site": "same-origin"}
	if got := s.send(http.MethodGet, pathOf(t, link.URL), "", bobsAgent).Code; got == http.StatusSeeOther {
		t.Errorf("bob's agent took the link to edit")
	}
	if got := s.send(http.MethodPost, "/api/pages/"+page.ID+"/blocks", `{"md":"bob was here"}`, plain).Code; got != http.StatusForbidden {
		t.Errorf("bob edits through a link his agent opened = %d", got)
	}
	// No agents in Team.
	if got := s.send(http.MethodPut, "/api/objects/"+team.ID+"/grants", `{"kind":"agents","actions":{"read":"deny"}}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("no agents = %d", got)
	}
	if got := s.send(http.MethodGet, "/api/doc/"+page.ID, "", asAgentOf).Code; got != http.StatusNotFound {
		t.Errorf("the owner's agent in a space with no agents = %d", got)
	}
	if got := s.send(http.MethodGet, "/api/doc/"+page.ID, "", sameSite).Code; got != http.StatusOK {
		t.Errorf("anna herself = %d", got)
	}
	// Anna limits her agents to reading, in a session.
	if got := s.send(http.MethodPut, "/api/me/agents", `{"actions":["read"]}`, sameSite).Code; got != http.StatusNoContent {
		t.Errorf("anna limits her agents = %d", got)
	}
	limit := decode[map[string][]string](t, s.get("/api/me/agents"))
	if !slices.Equal(limit["actions"], []string{"read"}) {
		t.Errorf("the limit: %v", limit)
	}
}

// TestNestedGroupsThroughTheAPI: a group admin manages their group and those inside it, a cycle answers 409, and a grant
// to a group reaches its subgroups unless it says otherwise (ADR-36).
func TestNestedGroupsThroughTheAPI(t *testing.T) {
	s, _ := adminSite(t)
	s.send(http.MethodGet, "/api/state", "", plain)
	s.send(http.MethodGet, "/api/state", "", carlOf)
	make := func(body string, headers map[string]string) apiGroup {
		t.Helper()
		got := s.send(http.MethodPost, "/api/groups", body, headers)
		if got.Code != http.StatusCreated {
			t.Fatalf("make %s: %d %s", body, got.Code, got.Body.String())
		}
		var g apiGroup
		if err := json.Unmarshal(got.Body.Bytes(), &g); err != nil {
			t.Fatal(err)
		}
		return g
	}
	company := make(`{"name":"Company"}`, sameSite)
	sales := make(`{"name":"Sales","parent":"`+company.ID+`"}`, sameSite)
	if got := s.send(http.MethodPut, "/api/groups/"+sales.ID+"/admins/1002", `{"admin":true}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("bob admin of Sales = %d", got)
	}
	north := make(`{"name":"North","parent":"`+sales.ID+`"}`, plain)
	if got := s.send(http.MethodPut, "/api/groups/"+north.ID+"/members/1003", `{"member":true}`, plain).Code; got != http.StatusNoContent {
		t.Errorf("bob takes carl into North = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/groups/"+company.ID+"/members/1002", `{"member":true}`, plain).Code; got != http.StatusForbidden {
		t.Errorf("bob changes Company = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/groups/"+north.ID+"/groups/"+company.ID, `{"member":true}`, sameSite).Code; got != http.StatusConflict {
		t.Errorf("a cycle = %d", got)
	}
	detail := decode[struct {
		Group   apiGroup    `json:"group"`
		Groups  []apiGroup  `json:"groups"`
		Admins  []apiPerson `json:"admins"`
		Parents []apiGroup  `json:"parents"`
	}](t, s.getWith("/api/groups/"+sales.ID, merged(plain)))
	if !detail.Group.Admin || len(detail.Groups) != 1 || detail.Groups[0].ID != north.ID || len(detail.Admins) != 1 || len(detail.Parents) != 1 {
		t.Errorf("Sales as bob sees it: %+v", detail)
	}
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Handbook","parent":"`+team.ID+`"}`, sameSite))
	s.send(http.MethodPut, "/api/objects/"+page.ID+"/inherit", `{"inherit":false}`, sameSite)
	s.send(http.MethodPut, "/api/objects/"+page.ID+"/grants", `{"kind":"tenant","role":""}`, sameSite)
	s.send(http.MethodPut, "/api/objects/"+page.ID+"/grants", `{"kind":"person","who":"1002","role":""}`, sameSite)
	if got := s.send(http.MethodPut, "/api/objects/"+page.ID+"/grants", `{"kind":"group","who":"`+company.ID+`","actions":{"read":"allow"}}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("grant to Company = %d", got)
	}
	if got := s.send(http.MethodGet, "/api/doc/"+page.ID, "", carlOf).Code; got != http.StatusOK {
		t.Errorf("carl, in North inside Sales inside Company, reads = %d", got)
	}
	s.send(http.MethodPut, "/api/objects/"+page.ID+"/grants", `{"kind":"group","who":"`+company.ID+`","actions":{"read":"allow"},"subgroups":false}`, sameSite)
	if got := s.send(http.MethodGet, "/api/doc/"+page.ID, "", carlOf).Code; got != http.StatusNotFound {
		t.Errorf("without subgroups carl does not read = %d", got)
	}
}

// TestDenyInTheSearch: a deny on read holds in the search service, for persons and for agents, through the levels of
// the index and once more through the check of each hit (ADR-36, ADR-24).
func TestDenyInTheSearch(t *testing.T) {
	s := withSearch(t, meiliEngine(t))
	s.send(http.MethodGet, "/api/state", "", bob)
	team := s.sharedCollection(`{"title":"Filiale"}`, sameSite)
	open := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kassenhandbuch","parent":"`+team.ID+`"}`, sameSite))
	addBlocks(t, s.site, open.ID, "Die Kartenzahlung beginnt mit der Taste Karte.")
	closed := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Gehälter zur Kartenzahlung","parent":"`+team.ID+`"}`, sameSite))
	addBlocks(t, s.site, closed.ID, "Kartenzahlung der Gehälter.")
	if got := s.send(http.MethodPut, "/api/objects/"+closed.ID+"/grants", `{"kind":"person","who":"1002","actions":{"read":"deny"}}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("deny = %d", got)
	}
	s.drain()
	ids := func(r apiSearchResult) []string {
		var out []string
		for _, h := range r.Hits {
			out = append(out, h.ID)
		}
		return out
	}
	if got := ids(s.find("q=Kartenzahlung", nil)); !slices.Contains(got, closed.ID) || !slices.Contains(got, open.ID) {
		t.Errorf("anna finds both: %v", got)
	}
	if got := ids(s.find("q=Kartenzahlung", bob)); !slices.Equal(got, []string{open.ID}) {
		t.Errorf("bob never finds the page denied to him: %v", got)
	}
	// The index alone keeps it from bob: its record lists the deny.
	docs, err := s.db.SearchSource().Docs(tenant.With(context.Background(), tenant.Main), closed.ID)
	if err != nil || len(docs) == 0 || docs[0].MayRead([]string{"all", "account:1002"}, false) || !docs[0].MayRead([]string{"all", "account:1003"}, false) {
		t.Errorf("the record of the page denies bob and lets carl read: %+v %v", docs, err)
	}
	// A deny the index has not heard of yet holds at once: each hit is checked again.
	s.send(http.MethodPut, "/api/objects/"+open.ID+"/grants", `{"kind":"person","who":"1002","actions":{"read":"deny"}}`, sameSite)
	if got := ids(s.find("q=Kartenzahlung", bob)); len(got) != 0 {
		t.Errorf("before the index catches up bob finds %v", got)
	}
	// No agents in Filiale: anna's agent finds nothing there, anna herself everything.
	s.send(http.MethodPut, "/api/objects/"+team.ID+"/grants", `{"kind":"agents","actions":{"read":"deny"}}`, sameSite)
	s.drain()
	if got := ids(s.find("q=Kartenzahlung", asAgentOf)); len(got) != 0 {
		t.Errorf("anna's agent finds %v in a space with no agents", got)
	}
	if got := ids(s.find("q=Kartenzahlung", nil)); len(got) != 2 {
		t.Errorf("anna finds both still: %v", got)
	}
}
