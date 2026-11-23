// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// The attack round of the review of B-3 (B-4) through the API: a service account with the roles of the app, the counts
// beside the hits of a search, and deleting for good with a wrong name or from another site.

// serviceWithRoles is an allowed service account to which the gate gives the roles of the administrator and of spaces.
var serviceWithRoles = map[string]string{"Remote-Auth": "apikey", "Remote-Kind": "service", "Remote-User": "export-bot",
	"Remote-Id": "4242", "Remote-Name": "Export", "Remote-Groups": "ops,underdoc-admin,underdoc-spaces", "Sec-Fetch-Site": "same-origin"}

// Attack: a service account to which the gate gives the roles of the app administers nothing and makes no space (B-4,
// finding 1).
func TestServiceAccountWithTheRoles(t *testing.T) {
	s := withSearch(t, nil, gateOf(t, account.GateConfig{ServiceAccounts: []string{"4242"}}))
	state := decode[apiState](t, s.send(http.MethodGet, "/api/state", "", serviceWithRoles))
	if state.User.Admin || state.CanSpaces || !state.User.Agent {
		t.Errorf("the service account with the roles: %+v, spaces %v", state.User, state.CanSpaces)
	}
	for name, call := range map[string][3]string{
		"list the users":   {http.MethodGet, "/api/users", ""},
		"take a mark":      {http.MethodPut, "/api/users/1007/admin", `{"admin":false}`},
		"make a group":     {http.MethodPost, "/api/groups", `{"name":"Bots"}`},
		"make a space":     {http.MethodPost, "/api/collections", `{"title":"Exports"}`},
		"make a file root": {http.MethodPost, "/api/files/collections", `{"title":"Exports"}`},
	} {
		if got := s.send(call[0], call[1], call[2], serviceWithRoles); got.Code < 400 {
			t.Errorf("the service account with the roles may %s: %d %s", name, got.Code, got.Body.String())
		}
	}
	if got := s.send(http.MethodGet, "/admin/modules", "", serviceWithRoles).Code; got != http.StatusNotFound {
		t.Errorf("the service account with the roles opens the administration of modules = %d", got)
	}
}

// testHandler is a bare handler with the pages of the application, to ask its checks directly.
func testHandler(t *testing.T) *handler {
	t.Helper()
	pages, err := template.New("").Funcs(template.FuncMap{"meterOf": func(m *meter, label string) map[string]any { return nil }}).
		ParseFS(templateFiles, "templates/*.html")
	if err != nil {
		t.Fatal(err)
	}
	_, database := adminSite(t)
	return &handler{pages: pages, options: Options{DB: database}}
}

// TestOnlyPersonsAdminister: the checks of the administration ask administers, so a service account or an agent that
// reaches them with the mark of an administrator is still refused (B-4, finding 1). The sign-in takes the mark away
// before; this holds if it did not.
func TestOnlyPersonsAdminister(t *testing.T) {
	h := testHandler(t)
	for name, user := range map[string]account.User{
		"service account": {ID: "4242", Admin: true, AdminRole: true, AuthKind: account.AuthSession, Kind: account.KindService},
		"agent":           {ID: "1007", Admin: true, AdminRole: true, AuthKind: account.AuthOAuth, Kind: account.KindPerson},
	} {
		r := httptest.NewRequest(http.MethodGet, "/api/users", nil).WithContext(account.With(context.Background(), user))
		if w := httptest.NewRecorder(); h.adminAPI(w, r) || w.Code != http.StatusForbidden {
			t.Errorf("adminAPI lets the %s through: %d", name, w.Code)
		}
		if w := httptest.NewRecorder(); h.adminOnly(w, r) || w.Code != http.StatusNotFound {
			t.Errorf("adminOnly lets the %s through: %d", name, w.Code)
		}
	}
	person := account.User{ID: "1007", Admin: true, AuthKind: account.AuthSession, Kind: account.KindPerson}
	r := httptest.NewRequest(http.MethodGet, "/api/users", nil).WithContext(account.With(context.Background(), person))
	if !h.adminAPI(httptest.NewRecorder(), r) || !h.adminOnly(httptest.NewRecorder(), r) {
		t.Error("an administrator in a session is refused")
	}
}

// Attack: the counts beside the hits of a search (B-4, finding 3). A deny for a role of the gate holds for a service
// account, but the role's token is not among the service account's, so the index lets the page through; the check of
// each hit drops it, and the counts, made from the checked hits, do not tell of it either.
func TestSearchCountsOnlyWhatIsFound(t *testing.T) {
	s := withSearch(t, meiliEngine(t), gateOf(t, account.GateConfig{ServiceAccounts: []string{"4242"}}))
	service := merged2(serviceWithRoles, map[string]string{"Remote-Groups": "ops"})
	s.send(http.MethodGet, "/api/state", "", service)
	team := s.sharedCollection(`{"title":"Filiale"}`, sameSite)
	open := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kassenhandbuch","parent":"`+team.ID+`"}`, sameSite))
	closed := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kassenabschluss","parent":"`+team.ID+`"}`, sameSite))
	addBlocks(t, s.site, open.ID, "Die Kartenzahlung beginnt mit der Taste Karte.")
	addBlocks(t, s.site, closed.ID, "Die Kartenzahlung des Tages wird abgerechnet.")
	people := decode[struct {
		Groups []apiGroup `json:"groups"`
	}](t, s.get("/api/people"))
	ops := ""
	for _, g := range people.Groups {
		if g.Name == "ops" {
			ops = g.ID
		}
	}
	if ops == "" {
		t.Fatalf("the role ops: %+v", people.Groups)
	}
	for _, grant := range [][2]string{
		{team.ID, `{"kind":"person","who":"4242","role":"read"}`},
		{closed.ID, `{"kind":"group","who":"` + ops + `","actions":{"read":"deny"}}`},
	} {
		if got := s.send(http.MethodPut, "/api/objects/"+grant[0]+"/grants", grant[1], sameSite).Code; got != http.StatusNoContent {
			t.Fatalf("grant %s = %d", grant[1], got)
		}
	}
	s.drain()
	result := s.find("q=Kartenzahlung", service)
	if len(result.Hits) != 1 || result.Hits[0].ID != open.ID {
		t.Fatalf("the service account finds what is granted to it, not what everyone is denied: %+v", result.Hits)
	}
	for _, field := range []string{"type", "author", "collection"} {
		for _, f := range result.Facets[field] {
			if f.Count != 1 {
				t.Errorf("the count of %s %s tells of the page denied: %+v", field, f.Value, f)
			}
		}
	}
	if len(result.Facets["type"]) != 1 {
		t.Errorf("the counts by type: %+v", result.Facets["type"])
	}
}

// Attack: deleting for good with a wrong name, and from a page of another site or without saying where it comes from
// (B-4, missing tests of B-3).
func TestPurgeNeedsTheNameAndTheBrowser(t *testing.T) {
	s := withDatabase(t)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Minutes","parent":"`+team.ID+`"}`, sameSite))
	s.send(http.MethodDelete, "/api/objects/"+page.ID, "", sameSite)
	if got := s.send(http.MethodDelete, "/api/trash/"+page.ID, `{"confirm":"Minute"}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("a wrong name = %d", got)
	}
	if got := s.send(http.MethodDelete, "/api/trash", `{"confirm":"Trash can"}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("emptying with a wrong word = %d", got)
	}
	for name, headers := range map[string]map[string]string{
		"without Sec-Fetch-Site or Origin": {},
		"from another site":                {"Sec-Fetch-Site": "cross-site"},
		"from another origin":              {"Origin": "https://evil.example"},
	} {
		if got := s.send(http.MethodDelete, "/api/trash/"+page.ID, `{"confirm":"Minutes"}`, headers).Code; got != http.StatusForbidden {
			t.Errorf("%s = %d", name, got)
		}
		if got := s.send(http.MethodDelete, "/api/trash", `{"confirm":"Trash"}`, headers).Code; got != http.StatusForbidden {
			t.Errorf("emptying %s = %d", name, got)
		}
	}
	if trash := decode[[]apiTrashed](t, s.get("/api/trash")); len(trash) != 1 {
		t.Errorf("Minutes is still in the trash: %+v", trash)
	}
}
