// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/Ollornog/EightPerDay/core"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/module"
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
	s, database := withTenants(t)
	type made struct {
		ids     []string
		comment string
		trashed string
		// trashedTitle is the name of what lies in the trash, typed to delete it for good.
		trashedTitle string
		secret       string
		embedded     string
		// employer is the private employer booked in the time tracking, its name the word of the tenant.
		employer string
		// group is a local group of the tenant, link the address of a link to the plan.
		group, link string
	}
	// The door "time tracking" (EightPerDay) is open in both tenants.
	for _, id := range []string{tenant.Main, "acme"} {
		if err := database.SetTenantModules(tenant.With(context.Background(), id), []string{module.Documents, module.Files, module.Time}); err != nil {
			t.Fatal(err)
		}
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
		m.employer = bookIn(t, s, h, word+" hours")
		// A local group with a role on the plan: the door "rights" (ADR-29).
		tid := tenant.Main
		if host == acmeHost {
			tid = "acme"
		}
		board, err := database.CreateGroup(tenant.With(context.Background(), tid), db.Actor{Admin: true}, word+" board", "", "")
		if err != nil {
			t.Fatal(err)
		}
		if err := database.SetGrant(tenant.With(context.Background(), tid), plan.ID, db.Actor{ID: "1007"}, db.ToGroup, board.ID, db.RoleRead); err != nil {
			t.Fatal(err)
		}
		m.group = board.ID
		link := decode[apiLink](t, s.send(http.MethodPost, "/api/objects/"+plan.ID+"/links", `{"mode":"read"}`, h))
		if link.URL == "" {
			t.Fatalf("a link: %+v", link)
		}
		u, _ := url.Parse(link.URL)
		m.link = u.Path
		m.ids = []string{team.ID, plan.ID, uses.ID, gone.ID}
		m.comment = strconv.FormatInt(c.ID, 10)
		m.trashed, m.trashedTitle = gone.ID, gone.Title
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
			reads := []string{"/api/state", "/api/trash", "/api/problems", "/api/search?q=secret", "/api/search?q=plan", "/api/people", "/api/groups", "/api/log"}
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
			// The time tracking shows nothing of the other tenant, and nothing of it can be changed from here.
			for _, target := range []string{"/time/", "/time/week", "/time/weeks", "/time/leave", "/time/reports", "/time/all",
				"/time/access", "/time/orgs", "/time/export/json", "/time/?e=" + c.other.employer,
				"/api/time/absences?from=2026-01-01&to=2026-12-31"} {
				got := s.getWith(target, h)
				if strings.Contains(got.Body.String(), c.other.employer) || strings.Contains(got.Body.String(), c.otherWords[0]+" hours") {
					t.Errorf("%s shows the time tracking of the other tenant: %d", target, got.Code)
				}
			}
			otherTenant := tenant.Main
			if c.otherHost == acmeHost {
				otherTenant = "acme"
			}
			for _, target := range []string{"/time/employers/" + c.other.employer, "/time/orgs/" + otherTenant} {
				if got := s.getWith(target, h); got.Code != http.StatusNotFound {
					t.Errorf("GET %s = %d", target, got.Code)
				}
			}
			for _, change := range []struct {
				target string
				values url.Values
			}{
				{"/time/book/week", url.Values{"employer": {c.other.employer}, "week": {"2026-11-16"}, "hours": {"1"}, "back": {"/"}}},
				{"/time/book/absence", url.Values{"employer": {c.other.employer}, "type": {"sick"}, "from": {"2026-11-17"}, "back": {"/"}}},
				{"/time/employers/" + c.other.employer + "/archive", url.Values{"back": {"/"}}},
				{"/time/employers/" + c.other.employer + "/name", url.Values{"name": {"taken"}, "back": {"/"}}},
				{"/time/orgs/" + otherTenant + "/member", url.Values{"person": {"1007"}, "role": {"admin"}, "back": {"/"}}},
			} {
				if got := sendForm(s, change.target, change.values, h); got.Code < 400 || got.Code == http.StatusInternalServerError {
					t.Errorf("POST %s = %d", change.target, got.Code)
				}
			}
			for _, id := range c.other.ids {
				for _, target := range []string{"/o/" + id, "/api/doc/" + id, "/api/objects/" + id + "/uses", "/api/pages/" + id + "/comments",
					"/api/pages/" + id + "/versions", "/api/objects/" + id + "/export", "/api/objects/" + id + "/rights"} {
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
					{http.MethodPut, "/api/objects/" + id + "/grants", `{"kind":"tenant","role":"edit"}`},
					{http.MethodPut, "/api/objects/" + id + "/inherit", `{"inherit":false}`},
					{http.MethodPut, "/api/objects/" + id + "/owner", `{"to":"1007"}`},
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
				{http.MethodDelete, "/api/trash/" + c.other.trashed, `{"confirm":"` + c.other.trashedTitle + `"}`},
			} {
				if got := s.send(change.method, change.target, change.body, h); got.Code != http.StatusNotFound {
					t.Errorf("%s %s = %d", change.method, change.target, got.Code)
				}
			}
			// The link of the other tenant leads nowhere here, also signed out.
			for _, headers := range []map[string]string{h, {"Host": c.host}} {
				if got := s.getWith(c.other.link, headers); got.Code != http.StatusNotFound || strings.Contains(got.Body.String(), c.other.secret) {
					t.Errorf("the link of the other tenant: %d", got.Code)
				}
			}
			if got := s.getWith(c.other.link, at(c.otherHost)); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), c.other.secret) {
				t.Errorf("at its own host the link works: %d", got.Code)
			}
			if body := s.getWith("/api/links", h).Body.String(); strings.Contains(body, c.other.ids[1]) {
				t.Errorf("the links of the other tenant are listed: %s", body)
			}
			// The groups of the other tenant are not to be found, also for an administrator of this one.
			here := tenant.Main
			if c.host == acmeHost {
				here = "acme"
			}
			hctx := tenant.With(context.Background(), here)
			for name, err := range map[string]error{
				"member":  database.SetMember(hctx, db.Actor{Admin: true}, c.other.group, "1007", true),
				"rename":  database.RenameGroup(hctx, db.Actor{Admin: true}, c.other.group, "taken", ""),
				"archive": database.ArchiveGroup(hctx, db.Actor{Admin: true}, c.other.group),
				"restore": database.RestoreGroup(hctx, db.Actor{Admin: true}, c.other.group),
			} {
				if !errors.Is(err, db.ErrNotFound) {
					t.Errorf("%s of the other tenant's group: %v", name, err)
				}
			}
			if _, err := database.GroupMembers(hctx, c.other.group); !errors.Is(err, db.ErrNotFound) {
				t.Errorf("members of the other tenant's group: %v", err)
			}
			// An embedding of the other tenant's block shows "not to be found", as for something that never was.
			team := s.sharedCollection(`{"title":"Probe"}`, h)
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
			if got := s.send(http.MethodDelete, "/api/trash", `{"confirm":"Trash"}`, h).Code; got >= 400 {
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
	// Both tenants still have what they made, in the time tracking as well.
	for _, c := range []struct {
		host, tenant string
		m            made
	}{{mainHost, tenant.Main, inMain}, {acmeHost, "acme", inAcme}} {
		ctx := tenant.With(context.Background(), c.tenant)
		entries, err := database.TimeStore().Entries(ctx, core.EntryFilter{Owner: db.Person("1007"), IncludeDeleted: true})
		if err != nil || len(entries) != 1 || entries[0].EmployerID != c.m.employer || entries[0].Minutes != 38*60+30 || entries[0].Version != 1 {
			t.Errorf("%s: its time tracking changed: %+v %v", c.tenant, entries, err)
		}
		cat, err := database.TimeStore().Catalog(ctx, db.Person("1007"))
		if err != nil || len(cat.Employers) != 1 || cat.Employers[0].Archived {
			t.Errorf("%s: its employers changed: %+v %v", c.tenant, cat.Employers, err)
		}
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
	if hits := decode[apiSearchResult](t, s.getWith("/api/search?q=network", at(acmeHost))).Hits; len(hits) != 0 {
		t.Errorf("acme finds the folder: %+v", hits)
	}
	if hits := decode[apiSearchResult](t, s.getWith("/api/search?q=network", at(mainHost))).Hits; len(hits) == 0 {
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
	team := s.sharedCollection(`{"title":"Own domain"}`, at("wiki.acme.example"))
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
	team := s.sharedCollection(`{"title":"Team"}`, at("anything.example"))
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
