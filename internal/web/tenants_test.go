// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

const tenantDomain = "docs.example"

// withTenants is a site in the mode many tenants with Main and acme, each reached at its subdomain.
func withTenants(t *testing.T) (*site, *db.DB) {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if _, err := database.CreateTenant(context.Background(), "acme", "Acme Ltd"); err != nil {
		t.Fatal(err)
	}
	behindGate(t, database, tenant.Main, "acme")
	return newSite(t, Options{DB: database, Tenants: tenant.ModeMany, TenantDomain: tenantDomain}, sample), database
}

// at is a set of headers for a request to a host, as Anna through the gate, from a page of the same origin.
func at(host string, more ...map[string]string) map[string]string {
	headers := map[string]string{"Host": host, "Sec-Fetch-Site": "same-origin"}
	for k, v := range gateHeaders {
		headers[k] = v
	}
	for _, m := range more {
		for k, v := range m {
			headers[k] = v
		}
	}
	return headers
}

var (
	mainHost = "main." + tenantDomain
	acmeHost = "acme." + tenantDomain
)

// TestTenantsSeeNothingOfEachOther is the separation test of ADR-23: Anna works in Main, and the same Anna in acme finds
// nothing of it through any door: the tree, the pages, search, embeddings, comments, uses, versions, the trash, the
// export, the folder source and every change. And the other way round.
func TestTenantsSeeNothingOfEachOther(t *testing.T) {
	s, _ := withTenants(t)
	type made struct {
		ids      []string
		comment  string
		trashed  string
		secret   string
		embedded string
	}
	build := func(host, word string) made {
		h := at(host)
		var m made
		m.secret = word + " secret text"
		team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"`+word+` team"}`, h))
		plan := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"`+word+` plan","parent":"`+team.ID+`"}`, h))
		block := decode[apiBlock](t, s.send(http.MethodPost, "/api/pages/"+plan.ID+"/blocks", `{"md":"`+m.secret+`"}`, h))
		uses := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"`+word+` uses","parent":"`+team.ID+`"}`, h))
		s.send(http.MethodPost, "/api/pages/"+uses.ID+"/blocks", `{"md":"::embed{ref=\"`+plan.ID+`#`+block.ID+`\"}"}`, h)
		c := decode[apiComment](t, s.send(http.MethodPost, "/api/pages/"+plan.ID+"/comments", `{"body":"`+word+` comment"}`, h))
		gone := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"`+word+` gone","parent":"`+team.ID+`"}`, h))
		if got := s.send(http.MethodDelete, "/api/objects/"+gone.ID, "", h).Code; got != http.StatusNoContent && got != http.StatusOK {
			t.Fatalf("delete: %d", got)
		}
		if got := s.send(http.MethodPut, "/api/objects/"+plan.ID+"/tags", `{"tags":["`+strings.ToLower(word)+`tag"]}`, h).Code; got != http.StatusOK {
			t.Fatalf("tags: %d", got)
		}
		if doc := decode[apiDocument](t, s.getWith("/api/doc/"+uses.ID, h)); !strings.Contains(doc.Blocks[0].HTML, m.secret) {
			t.Fatalf("the embedding works within its tenant: %+v", doc.Blocks)
		}
		m.ids = []string{team.ID, plan.ID, uses.ID, gone.ID}
		m.comment = strconv.FormatInt(c.ID, 10)
		m.trashed = gone.ID
		m.embedded = plan.ID + "#" + block.ID
		return m
	}
	inMain := build(mainHost, "Main")
	inAcme := build(acmeHost, "Acme")
	if trash := s.getWith("/api/trash", at(mainHost)).Body.String(); !strings.Contains(trash, inMain.trashed) {
		t.Fatalf("Main has its trash: %s", trash)
	}

	for _, c := range []struct {
		name       string
		host       string
		otherHost  string
		other      made
		otherWords []string
	}{
		{"acme looks at Main", acmeHost, mainHost, inMain, []string{"Main", "Start page", "Network plan", "maintag"}},
		{"Main looks at acme", mainHost, acmeHost, inAcme, []string{"Acme", "acmetag"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := at(c.host)
			reads := []string{"/api/state", "/api/trash", "/api/problems", "/api/search?q=secret", "/api/search?q=plan"}
			for _, target := range reads {
				body := s.getWith(target, h).Body.String()
				for _, id := range c.other.ids {
					if strings.Contains(body, id) {
						t.Errorf("%s shows the object %s of the other tenant", target, id)
					}
				}
				for _, word := range append(c.otherWords, c.other.secret) {
					if strings.Contains(body, word) {
						t.Errorf("%s shows %q of the other tenant: %s", target, word, body)
					}
				}
			}
			for _, id := range c.other.ids {
				for _, target := range []string{"/o/" + id, "/api/doc/" + id, "/api/objects/" + id + "/uses", "/api/pages/" + id + "/comments",
					"/api/pages/" + id + "/versions", "/api/objects/" + id + "/export"} {
					if got := s.getWith(target, h); got.Code != http.StatusNotFound {
						t.Errorf("GET %s = %d", target, got.Code)
					}
				}
				for _, change := range []struct{ method, target, body string }{
					{http.MethodPatch, "/api/objects/" + id, `{"title":"taken"}`},
					{http.MethodPut, "/api/objects/" + id + "/tags", `{"tags":["x"]}`},
					{http.MethodPost, "/api/objects/" + id + "/access", `{"access":"read"}`},
					{http.MethodPost, "/api/objects/" + id + "/kind", `{"kind":"folder"}`},
					{http.MethodPost, "/api/objects/" + id + "/move", `{"parent":""}`},
					{http.MethodPost, "/api/objects/" + id + "/copy", `{"parent":""}`},
					{http.MethodDelete, "/api/objects/" + id, ""},
					{http.MethodPost, "/api/pages", `{"title":"in","parent":"` + id + `"}`},
					{http.MethodPost, "/api/collections", `{"title":"in","parent":"` + id + `"}`},
					{http.MethodPost, "/api/pages/" + id + "/blocks", `{"md":"in"}`},
					{http.MethodPut, "/api/pages/" + id + "/blocks/b1", `{"md":"in"}`},
					{http.MethodDelete, "/api/pages/" + id + "/blocks/b1", ""},
					{http.MethodPost, "/api/pages/" + id + "/comments", `{"body":"in"}`},
				} {
					got := s.send(change.method, change.target, change.body, h)
					if got.Code < 400 || got.Code == http.StatusInternalServerError {
						t.Errorf("%s %s = %d", change.method, change.target, got.Code)
					}
				}
			}
			for _, change := range []struct{ method, target, body string }{
				{http.MethodPatch, "/api/comments/" + c.other.comment, `{"body":"taken"}`},
				{http.MethodPatch, "/api/comments/" + c.other.comment, `{"resolved":true}`},
				{http.MethodDelete, "/api/comments/" + c.other.comment, ""},
				{http.MethodPost, "/api/trash/" + c.other.trashed + "/restore", ""},
				{http.MethodDelete, "/api/trash/" + c.other.trashed, ""},
			} {
				if got := s.send(change.method, change.target, change.body, h); got.Code != http.StatusNotFound {
					t.Errorf("%s %s = %d", change.method, change.target, got.Code)
				}
			}
			// An embedding of the other tenant's block shows "not to be found", as for something that never was.
			team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Probe"}`, h))
			probe := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Probe","parent":"`+team.ID+`"}`, h))
			s.send(http.MethodPost, "/api/pages/"+probe.ID+"/blocks", `{"md":"::embed{ref=\"`+c.other.embedded+`\"}"}`, h)
			s.send(http.MethodPost, "/api/pages/"+probe.ID+"/blocks", `{"md":"[[plan]] and [[start]]"}`, h)
			body := s.getWith("/api/doc/"+probe.ID, h).Body.String()
			if strings.Contains(body, c.other.secret) || !strings.Contains(body, "Not to be found") {
				t.Errorf("an embedding reaches into the other tenant: %s", body)
			}
			if c.host == acmeHost && (strings.Contains(body, "/o/plan") || strings.Contains(body, "/o/start")) {
				t.Errorf("a link reaches into the folder of Main: %s", body)
			}
			// Emptying the trash empties this tenant's, and what the other tenant has in its trash stays.
			before := s.getWith("/api/trash", at(c.otherHost)).Body.String()
			if got := s.send(http.MethodDelete, "/api/trash", "", h).Code; got >= 400 {
				t.Errorf("empty the trash = %d", got)
			}
			if mine := decode[[]apiTrashed](t, s.getWith("/api/trash", h)); len(mine) != 0 {
				t.Errorf("the own trash stays full: %+v", mine)
			}
			if after := s.getWith("/api/trash", at(c.otherHost)).Body.String(); after != before {
				t.Errorf("the other tenant's trash changed: %s, was %s", after, before)
			}
		})
	}
	// Both tenants still have what they made.
	for _, c := range []struct {
		host string
		m    made
	}{{mainHost, inMain}, {acmeHost, inAcme}} {
		if doc := decode[apiDocument](t, s.getWith("/api/doc/"+c.m.ids[1], at(c.host))); !strings.Contains(doc.Blocks[0].HTML, c.m.secret) {
			t.Errorf("%s lost its page: %+v", c.host, doc)
		}
	}
}

// TestTheFolderBelongsToMain: the folder source is the instance's, so with many tenants only Main sees it.
func TestTheFolderBelongsToMain(t *testing.T) {
	s, _ := withTenants(t)
	for _, target := range []string{"/o/start", "/f/ops/net", "/files/ops/net/map.png", "/api/doc/start"} {
		if got := s.getWith(target, at(mainHost)); got.Code != http.StatusOK {
			t.Errorf("Main: %s = %d", target, got.Code)
		}
		if got := s.getWith(target, at(acmeHost)); got.Code != http.StatusNotFound {
			t.Errorf("acme: %s = %d", target, got.Code)
		}
	}
	if hits := decode[[]apiHit](t, s.getWith("/api/search?q=router", at(acmeHost))); len(hits) != 0 {
		t.Errorf("acme finds the folder: %+v", hits)
	}
	if hits := decode[[]apiHit](t, s.getWith("/api/search?q=router", at(mainHost))); len(hits) == 0 {
		t.Error("Main finds its folder")
	}
}

// TestHostsLeadToTenants: an unknown host finds nothing, the hoster tenant is never a workspace, a locked tenant lets
// nobody in, and a domain of its own leads to its tenant.
func TestHostsLeadToTenants(t *testing.T) {
	s, database := withTenants(t)
	ctx := context.Background()
	for _, host := range []string{"nobody." + tenantDomain, "hoster." + tenantDomain, tenantDomain, "acme.elsewhere.example", "127.0.0.1"} {
		got := s.getWith("/api/state", at(host))
		if got.Code != http.StatusNotFound || !strings.Contains(got.Body.String(), "Nothing here") {
			t.Errorf("%s = %d", host, got.Code)
		}
	}
	if got := s.getWith("/assets/app/demo.js", at("nobody."+tenantDomain)); got.Code != http.StatusOK {
		t.Errorf("assets answer on every host, so the message has its styles: %d", got.Code)
	}
	if err := database.AddTenantHost(ctx, "acme", "wiki.acme.example"); err != nil {
		t.Fatal(err)
	}
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Own domain"}`, at("wiki.acme.example")))
	if state := s.getWith("/api/state", at(acmeHost)).Body.String(); !strings.Contains(state, team.ID) {
		t.Error("the own domain and the subdomain lead to the same tenant")
	}
	if _, err := database.SetTenantState(ctx, "acme", db.TenantLocked); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{acmeHost, "wiki.acme.example"} {
		if got := s.getWith("/api/state", at(host)); got.Code != http.StatusForbidden || strings.Contains(got.Body.String(), team.ID) {
			t.Errorf("locked %s = %d", host, got.Code)
		}
	}
	if _, err := database.SetTenantState(ctx, "acme", db.TenantArchived); err != nil {
		t.Fatal(err)
	}
	if got := s.getWith("/api/state", at(acmeHost)); got.Code != http.StatusNotFound {
		t.Errorf("archived = %d", got.Code)
	}
	if _, err := database.SetTenantState(ctx, "acme", db.TenantActive); err != nil {
		t.Fatal(err)
	}
	if state := s.getWith("/api/state", at(acmeHost)).Body.String(); !strings.Contains(state, team.ID) {
		t.Error("an archived tenant comes back whole")
	}
}

// TestOneTenantIgnoresTheHost: in the default mode every host leads to Main, as before tenants.
func TestOneTenantIgnoresTheHost(t *testing.T) {
	s := withDatabase(t)
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, at("anything.example")))
	if state := s.getWith("/api/state", at("other.example")).Body.String(); !strings.Contains(state, team.ID) {
		t.Error("every host is Main")
	}
	if _, err := New(s.store, nil, Options{Tenants: tenant.ModeMany}); err == nil {
		t.Error("many tenants without a database")
	}
	if _, err := New(s.store, nil, Options{Tenants: "some"}); err == nil {
		t.Error("an unknown mode")
	}
}
