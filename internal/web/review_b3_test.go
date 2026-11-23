// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// The attack round of the review of rights and sign-in (B-3) through the API: deleting for good, groups archived
// instead of deleted, the role to create spaces, service accounts and Remote-Kind.

// Attack: deleting for good by an agent, with a key, with an app password, without the name, with a wrong one, and by an
// editor with delete who deleted it but does not own it.
func TestPurgeThroughTheAPIOnlyInTheBrowser(t *testing.T) {
	s := withDatabase(t)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Minutes","parent":"`+team.ID+`"}`, sameSite))
	other := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Agenda","parent":"`+team.ID+`"}`, sameSite))
	s.send(http.MethodDelete, "/api/objects/"+page.ID, "", sameSite)
	for name, auth := range map[string]string{"oauth": "oauth", "api key": "apikey", "app password": "app-password"} {
		headers := merged2(sameSite, map[string]string{"Remote-Auth": auth})
		if got := s.send(http.MethodDelete, "/api/trash/"+page.ID, `{"confirm":"Minutes"}`, headers).Code; got != http.StatusForbidden {
			t.Errorf("%s deletes for good = %d", name, got)
		}
		if got := s.send(http.MethodDelete, "/api/trash", `{"confirm":"Trash"}`, headers).Code; got != http.StatusForbidden {
			t.Errorf("%s empties the trash = %d", name, got)
		}
	}
	if trash := decode[[]apiTrashed](t, s.get("/api/trash")); len(trash) != 1 || !trash[0].Purge {
		t.Fatalf("Minutes waits, and its owner may delete it for good: %+v", trash)
	}
	// Bob may delete in Team, so he moves Agenda to the trash; deleting it for good is its owner's.
	bobHere := plain
	if got := s.send(http.MethodDelete, "/api/objects/"+other.ID, "", bobHere).Code; got >= 400 {
		t.Fatalf("bob moves Agenda to the trash = %d", got)
	}
	if trash := decode[[]apiTrashed](t, s.getWith("/api/trash", bobHere)); len(trash) != 1 || trash[0].Purge {
		t.Errorf("bob's trash offers to delete Agenda for good: %+v", trash)
	}
	if got := s.send(http.MethodDelete, "/api/trash/"+other.ID, `{"confirm":"Agenda"}`, bobHere).Code; got != http.StatusForbidden {
		t.Errorf("an editor with delete deletes for good what he deleted = %d", got)
	}
	if got := s.send(http.MethodDelete, "/api/trash", `{"confirm":"Trash"}`, bobHere).Code; got != http.StatusNoContent {
		t.Errorf("bob empties his trash = %d", got)
	}
	if trash := decode[[]apiTrashed](t, s.get("/api/trash")); len(trash) != 2 {
		t.Errorf("Agenda stays for its owner: %+v", trash)
	}
	if got := s.send(http.MethodDelete, "/api/trash/"+page.ID, `{"confirm":"Minutes"}`, sameSite).Code; got != http.StatusNoContent {
		t.Errorf("the owner in the browser with the name = %d", got)
	}
}

// Attack: a group admin archives their own group or the group above; a group with a group inside; restoring.
func TestArchivingGroupsThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	s.send(http.MethodGet, "/api/state", "", bob)
	s.send(http.MethodGet, "/api/state", "", carlOf)
	company := decode[apiGroup](t, s.send(http.MethodPost, "/api/groups", `{"name":"Company"}`, sameSite))
	sales := decode[apiGroup](t, s.send(http.MethodPost, "/api/groups", `{"name":"Sales","parent":"`+company.ID+`"}`, sameSite))
	inner := decode[apiGroup](t, s.send(http.MethodPost, "/api/groups", `{"name":"Key accounts","parent":"`+sales.ID+`"}`, sameSite))
	if got := s.send(http.MethodPut, "/api/groups/"+sales.ID+"/admins/1003", `{"admin":true}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("carl admin of Sales = %d", got)
	}
	carl := merged2(carlOf, sameSite)
	for name, groupID := range map[string]string{"his own group": sales.ID, "the group above": company.ID} {
		if got := s.send(http.MethodDelete, "/api/groups/"+groupID, "", carl).Code; got != http.StatusForbidden {
			t.Errorf("the admin of Sales archives %s = %d", name, got)
		}
	}
	for _, g := range decode[[]apiGroup](t, s.getWith("/api/groups", carl)) {
		if g.ID == sales.ID && g.Archive {
			t.Error("the list offers the admin of Sales to archive it")
		}
		if g.ID == inner.ID && g.Archive {
			t.Error("the list offers the admin of Sales the group inside it (only administrators archive, B-5)")
		}
	}
	if got := s.send(http.MethodDelete, "/api/groups/"+sales.ID, "", sameSite).Code; got != http.StatusConflict {
		t.Errorf("Sales with Key accounts inside = %d", got)
	}
	if got := s.send(http.MethodDelete, "/api/groups/"+inner.ID, "", carl).Code; got != http.StatusForbidden {
		t.Errorf("the admin of Sales archives the group inside it = %d", got)
	}
	if got := s.send(http.MethodDelete, "/api/groups/"+inner.ID, "", sameSite).Code; got != http.StatusNoContent {
		t.Errorf("an administrator archives Key accounts = %d", got)
	}
	if got := s.send(http.MethodDelete, "/api/groups/"+sales.ID, "", sameSite).Code; got != http.StatusNoContent {
		t.Errorf("an administrator archives Sales = %d", got)
	}
	people := decode[struct {
		Groups []apiGroup `json:"groups"`
	}](t, s.get("/api/people"))
	if slices.ContainsFunc(people.Groups, func(g apiGroup) bool { return g.ID == sales.ID }) {
		t.Error("an archived group is offered for new entries")
	}
	listed := decode[[]apiGroup](t, s.get("/api/groups"))
	if i := slices.IndexFunc(listed, func(g apiGroup) bool { return g.ID == sales.ID }); i < 0 || !listed[i].Archived || !listed[i].Archive {
		t.Errorf("the administrator sees Sales archived, to restore: %+v", listed)
	}
	if got := s.send(http.MethodPost, "/api/groups/"+sales.ID+"/restore", "", carl).Code; got != http.StatusForbidden {
		t.Errorf("the admin of Sales restores it = %d", got)
	}
	if got := s.send(http.MethodPost, "/api/groups/"+sales.ID+"/restore", "", sameSite).Code; got != http.StatusNoContent {
		t.Errorf("an administrator restores Sales = %d", got)
	}
}

// Attack: a space without the role, made, copied, moved there or among the files.
func TestSpacesThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	sub := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Projects","parent":"`+team.ID+`"}`, sameSite))
	bobHere := plain
	for name, call := range map[string][3]string{
		"make":       {http.MethodPost, "/api/collections", `{"title":"Mine"}`},
		"copy":       {http.MethodPost, "/api/objects/" + team.ID + "/copy", `{"parent":""}`},
		"move":       {http.MethodPost, "/api/objects/" + sub.ID + "/move", `{"parent":""}`},
		"make files": {http.MethodPost, "/api/files/collections", `{"title":"Files"}`},
	} {
		if got := s.send(call[0], call[1], call[2], bobHere); got.Code != http.StatusForbidden && got.Code != http.StatusNotImplemented {
			t.Errorf("bob without the role: %s = %d %s", name, got.Code, got.Body.String())
		}
	}
	if state := decode[apiState](t, s.getWith("/api/state", bobHere)); state.CanSpaces {
		t.Error("the workspace offers bob a space")
	}
	if got := s.send(http.MethodPost, "/api/collections", `{"title":"Inside","parent":"`+team.ID+`"}`, bobHere).Code; got != http.StatusCreated {
		t.Errorf("bob makes a collection inside Team = %d", got)
	}
	withRole := merged2(bobHere, map[string]string{"Remote-Groups": "underdoc-spaces"})
	if state := decode[apiState](t, s.getWith("/api/state", withRole)); !state.CanSpaces {
		t.Error("the role offers a space")
	}
	if got := s.send(http.MethodPost, "/api/collections", `{"title":"Bob's"}`, withRole).Code; got != http.StatusCreated {
		t.Errorf("bob with the role = %d", got)
	}
	if got := s.send(http.MethodPost, "/api/collections", `{"title":"Anna's"}`, sameSite).Code; got != http.StatusCreated {
		t.Errorf("an administrator = %d", got)
	}
}

// Attack: a service account nobody allowed; an allowed one that reads what everyone signed in reads, through the tree,
// a page, the export, the folder and the search; Remote-Kind missing or unknown (B-3).
func TestServiceAccountsThroughTheAPI(t *testing.T) {
	s := withSearch(t, meiliEngine(t), gateOf(t, account.GateConfig{ServiceAccounts: []string{"4242"}}))
	team := s.sharedCollection(`{"title":"Filiale"}`, sameSite)
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kassenhandbuch","parent":"`+team.ID+`"}`, sameSite))
	addBlocks(t, s.site, page.ID, "Die Kartenzahlung beginnt mit der Taste Karte.")
	s.drain()
	service := map[string]string{"Remote-Auth": "apikey", "Remote-Kind": "service", "Remote-User": "export-bot", "Remote-Id": "4242",
		"Remote-Name": "Export", "Remote-Groups": "ops", "Sec-Fetch-Site": "same-origin"}
	other := merged2(service, map[string]string{"Remote-Id": "4243"})
	if got := s.send(http.MethodGet, "/api/state", "", other); got.Code != http.StatusForbidden || !strings.Contains(got.Body.String(), "service account") {
		t.Errorf("a service account nobody allowed = %d", got.Code)
	}
	state := decode[apiState](t, s.send(http.MethodGet, "/api/state", "", service))
	if !state.User.Agent || state.User.Admin || state.CanSpaces {
		t.Errorf("an allowed service account is an agent: %+v", state.User)
	}
	if slices.ContainsFunc(state.Collections, func(n apiNode) bool { return n.ID == team.ID || n.Kind != "collection" }) {
		t.Errorf("the tree of a service account shows what everyone reads, or the folder: %+v", state.Collections)
	}
	for _, target := range []string{"/api/doc/" + page.ID, "/api/objects/" + team.ID + "/export", "/api/doc/plan"} {
		if got := s.send(http.MethodGet, target, "", service).Code; got != http.StatusNotFound {
			t.Errorf("the service account reads %s = %d", target, got)
		}
	}
	// Not even the counts beside the hits, which come from the index before each hit is checked, tell of it.
	if result := s.find("q=Kartenzahlung", service); len(result.Hits) != 0 || len(result.Facets["type"]) != 0 || len(result.Facets["author"]) != 0 {
		t.Errorf("the search finds for a service account what everyone reads: %+v %+v", result.Hits, result.Facets)
	}
	// Granted to it, it reads, finds and exports.
	if got := s.send(http.MethodPut, "/api/objects/"+page.ID+"/grants", `{"kind":"person","who":"4242","role":"read"}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("grant = %d", got)
	}
	s.drain()
	if got := s.send(http.MethodGet, "/api/doc/"+page.ID, "", service).Code; got != http.StatusOK {
		t.Errorf("the service account reads what is granted to it = %d", got)
	}
	if result := s.find("q=Kartenzahlung", service); len(result.Hits) != 1 {
		t.Errorf("the service account finds what is granted to it: %+v", result.Hits)
	}
	// A person signed in with a key is an agent of a person: everyone's audience holds.
	if result := s.find("q=Kartenzahlung", map[string]string{"Remote-Auth": "apikey"}); len(result.Hits) != 1 {
		t.Errorf("anna's agent: %+v", result.Hits)
	}

	// Remote-Kind is required: missing, empty or unknown is refused; legacy is the way back for a missing one.
	for name, kind := range map[string]string{"missing": "", "unknown": "robot", "forged": "admin"} {
		headers := map[string]string{"Remote-Kind": kind}
		if got := s.raw(http.MethodGet, "/api/state", "", "", "", merged2(gateHeaders, headers)).Code; got != http.StatusForbidden {
			t.Errorf("Remote-Kind %s = %d", name, got)
		}
	}
	legacy, _ := gateSite(t, account.GateConfig{KindLegacy: true})
	withoutKind := map[string]string{}
	for k, v := range gateHeaders {
		if k != "Remote-Kind" {
			withoutKind[k] = v
		}
	}
	if got := legacy.raw(http.MethodGet, "/api/state", "", "", "", withoutKind).Code; got != http.StatusOK {
		t.Errorf("the way back, without Remote-Kind = %d", got)
	}
	if got := legacy.raw(http.MethodGet, "/api/state", "", "", "", merged2(gateHeaders, map[string]string{"Remote-Kind": "robot"})).Code; got != http.StatusForbidden {
		t.Errorf("the way back, an unknown Remote-Kind = %d", got)
	}
}
