// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// TestAccessAndChangesAreLogged: opening and changing a page, a role, a link opened by someone without an account and a
// wrong password of a link land in the log; bob reads what he did, the owner what happened to her page (ADR-29).
func TestAccessAndChangesAreLogged(t *testing.T) {
	s := withDatabase(t)
	s.send(http.MethodGet, "/api/state", "", plain)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Plan","parent":"`+team.ID+`"}`, sameSite))
	s.send(http.MethodPost, "/api/pages/"+page.ID+"/blocks", `{"md":"one"}`, sameSite)
	s.send(http.MethodPost, "/api/pages/"+page.ID+"/blocks", `{"md":"two"}`, sameSite)
	for range 3 {
		s.send(http.MethodGet, "/api/doc/"+page.ID, "", plain)
	}
	s.send(http.MethodPut, "/api/objects/"+team.ID+"/grants", `{"kind":"person","who":"1002","role":"manage"}`, sameSite)
	if got := s.send(http.MethodPut, "/api/objects/"+team.ID+"/grants", `{"kind":"tenant","role":"read"}`, plain).Code; got != http.StatusNoContent {
		t.Fatalf("bob, who manages Team, grants = %d", got)
	}
	s.send(http.MethodPut, "/api/objects/"+page.ID+"/grants", `{"kind":"person","who":"1002","role":"comment"}`, sameSite)
	if got := s.send(http.MethodPost, "/api/pages/"+page.ID+"/comments", `{"body":"Looks good"}`, plain).Code; got != http.StatusCreated {
		t.Fatalf("bob comments = %d", got)
	}
	link := decode[apiLink](t, s.send(http.MethodPost, "/api/objects/"+page.ID+"/links", `{"mode":"read","password":"correct horse battery"}`, sameSite))
	path := pathOf(t, link.URL)
	s.anonymous(http.MethodPost, path+"/password", []byte("password=wrong"), map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	// Something private is not logged.
	notes := decode[apiState](t, s.get("/api/state")).Collections[0]
	secret := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Secret","parent":"`+notes.ID+`"}`, sameSite))
	s.get("/api/doc/" + secret.ID)

	entries := decode[[]apiEntry](t, s.get("/api/log"))
	count := map[string]int{}
	for _, e := range entries {
		if e.Object == page.ID {
			count[e.Action]++
		}
		if e.Object == secret.ID {
			t.Errorf("a private page is logged: %+v", e)
		}
	}
	// Anna and bob changed it (each once an hour), bob opened it (once), anna granted, made a link, and a visitor typed a wrong password.
	if count["change"] != 2 || count["open"] != 1 || count["grant"] != 1 || count["link made"] != 1 || count["link wrong password"] != 1 {
		t.Errorf("the log of the page: %v\n%+v", count, entries)
	}
	if !slices.ContainsFunc(entries, func(e apiEntry) bool { return e.Action == "change" && e.Actor == "1002" && e.Object == page.ID }) {
		t.Errorf("bob's comment is a change of the page: %+v", entries)
	}
	if slices.ContainsFunc(entries, func(e apiEntry) bool { return e.Action == "change" && e.Object == team.ID && e.Actor == "1002" }) {
		t.Errorf("a role is logged as what it is, not as a change: %+v", entries)
	}
	if i := slices.IndexFunc(entries, func(e apiEntry) bool { return e.Action == "link wrong password" }); i < 0 || entries[i].Actor != "" {
		t.Errorf("the visitor has no account: %+v", entries)
	}
	if i := slices.IndexFunc(entries, func(e apiEntry) bool { return e.Action == "grant" }); i < 0 || !strings.Contains(entries[i].Detail, "1002 comment") {
		t.Errorf("the grant says what: %+v", entries)
	}
	// Bob manages Team now, so he reads what happened there; carl, who manages nothing, reads what he did alone.
	bobs := decode[[]apiEntry](t, s.send(http.MethodGet, "/api/log", "", plain))
	if len(bobs) != len(entries) || !slices.ContainsFunc(bobs, func(e apiEntry) bool { return e.Action == "open" && e.Actor == "1002" }) {
		t.Errorf("bob reads what happened to what he manages: %d of %d", len(bobs), len(entries))
	}
	s.send(http.MethodGet, "/api/doc/"+page.ID, "", carlOf)
	carls := decode[[]apiEntry](t, s.send(http.MethodGet, "/api/log", "", carlOf))
	if len(carls) != 1 || carls[0].Actor != "1003" || carls[0].ObjectTitle != "Plan" {
		t.Errorf("carl reads what he did: %+v", carls)
	}
}
