// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/module"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

const (
	hosterHost = "admin.hoster.example"
	betaHost   = "beta.docs.example"
)

// hosterAdmin is an administrator of the hoster: on the hoster host, the gate gives them the role of the app.
var hosterAdmin = map[string]string{"Remote-Auth": "session", "Remote-User": "host", "Remote-Id": "9001", "Remote-Name": "Hoster Admin",
	"Remote-Groups": "underdoc-admin"}

// hosterSite is an instance with many tenants, Main and acme, and the hoster administration at its own host. Everyone
// comes through the gate (ADR-32). An instance does not start with many tenants yet; the code stays for when the gate
// names the tenant.
func hosterSite(t *testing.T) (*site, *db.DB, *client) {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if _, err := database.CreateTenant(context.Background(), "acme", "Acme Ltd"); err != nil {
		t.Fatal(err)
	}
	s := newSite(t, Options{DB: database, Tenants: tenant.ModeMany, TenantDomain: tenantDomain, HosterHost: hosterHost,
		Vault: testVault(t)}, sample)
	h := newClient(t, s, hosterHost)
	h.headers = hosterAdmin
	return s, database, h
}

// TestTheHosterManagesTenants: on its own host, the hoster makes a tenant, locks it, archives it, restores it, sets its
// limits, modules and domains. Its administrators come from the gate; there is no invitation.
func TestTheHosterManagesTenants(t *testing.T) {
	s, database, h := hosterSite(t)
	if got := h.get("/"); got.Code != http.StatusSeeOther || got.Header().Get("Location") != "/hoster" {
		t.Errorf("the start of the hoster host: %d", got.Code)
	}
	list := h.get("/hoster")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `data-id="acme"`) || !strings.Contains(list.Body.String(), `data-id="main"`) ||
		strings.Contains(list.Body.String(), `data-id="hoster"`) {
		t.Fatalf("the list: %d", list.Code)
	}
	for _, bad := range []url.Values{
		{"id": {"Bad"}, "name": {"x"}, "admin_login": {"a"}, "admin_name": {"A"}},
		{"id": {"acme"}, "name": {"x"}, "admin_login": {"a"}, "admin_name": {"A"}},
		{"id": {"hoster"}, "name": {"x"}, "admin_login": {"a"}, "admin_name": {"A"}},
	} {
		if got := h.form("/hoster/tenants", bad); got.Code != http.StatusBadRequest {
			t.Errorf("%v: %d", bad, got.Code)
		}
	}
	made := h.form("/hoster/tenants", url.Values{"id": {"beta"}, "name": {"Beta GmbH"}})
	if made.Code != http.StatusOK || !strings.Contains(made.Body.String(), "Beta GmbH is there") || strings.Contains(made.Body.String(), "/invite/") {
		t.Fatalf("a new tenant, without an invitation: %d", made.Code)
	}
	beta := newClient(t, s, betaHost)
	beta.headers = gateHeaders
	if got := beta.get("/api/state"); got.Code != http.StatusOK || !decode[apiState](t, got).User.Admin {
		t.Errorf("the gate's administrator on the tenant's host: %d", got.Code)
	}
	change := func(id string, values url.Values) int {
		return h.form("/hoster/tenants/"+id, values).Code
	}
	if got := change("beta", url.Values{"do": {"lock"}}); got != http.StatusOK {
		t.Fatalf("lock: %d", got)
	}
	if got := beta.get("/api/state"); got.Code != http.StatusForbidden {
		t.Errorf("a locked tenant: %d", got.Code)
	}
	change("beta", url.Values{"do": {"archive"}})
	if got := beta.get("/api/state"); got.Code != http.StatusNotFound {
		t.Errorf("an archived tenant: %d", got.Code)
	}
	if !strings.Contains(h.get("/hoster").Body.String(), "✗ archived") {
		t.Error("the list shows it archived, and keeps it")
	}
	change("beta", url.Values{"do": {"open"}})
	if got := beta.get("/api/state"); got.Code != http.StatusOK {
		t.Errorf("a restored tenant: %d", got.Code)
	}
	if got := change("beta", url.Values{"do": {"limits"}, "max_users": {"-1"}}); got != http.StatusBadRequest {
		t.Errorf("a negative limit: %d", got)
	}
	if got := change("beta", url.Values{"do": {"limits"}, "max_users": {"5"}, "max_storage_mb": {"2"}}); got != http.StatusOK {
		t.Fatalf("limits: %d", got)
	}
	// What is used of a limit shows as C22's bar, used / limit beside it; one user of five, nothing stored.
	listed := h.get("/hoster").Body.String()
	if !strings.Contains(listed, `<progress class="progress mt-1" value="1" max="5" aria-label="Users of the limit" data-test="meter">`) ||
		!strings.Contains(listed, "1 / 5") || !strings.Contains(listed, `max="2000" aria-label="Storage of the limit"`) {
		t.Errorf("the bars of the limits: %s", between(listed, `data-id="beta"`, "</tr>"))
	}
	if page := h.get("/hoster/tenants/beta").Body.String(); !strings.Contains(page, `data-test="usage"`) || strings.Count(page, `data-test="meter"`) != 2 {
		t.Error("the page of the tenant shows its use against its limits")
	}
	l, _ := database.TenantLimits(context.Background(), "beta")
	if l.MaxUsers == nil || *l.MaxUsers != 5 || l.MaxStorage == nil || *l.MaxStorage != 2_000_000 || l.Modules == nil || len(l.Modules) != 0 {
		t.Errorf("%+v", l)
	}
	change("beta", url.Values{"do": {"limits"}, "all_modules": {"on"}})
	if l, _ := database.TenantLimits(context.Background(), "beta"); l.Modules != nil || l.MaxUsers != nil {
		t.Errorf("all modules, no limits: %+v", l)
	}
	change("beta", url.Values{"do": {"limits"}, "module": {module.Documents}})
	if l, _ := database.TenantLimits(context.Background(), "beta"); len(l.Modules) != 1 {
		t.Errorf("one module: %+v", l)
	}
	if got := change("beta", url.Values{"do": {"add-host"}, "host": {"wiki.beta.example"}}); got != http.StatusOK {
		t.Fatalf("a domain: %d", got)
	}
	if got := newClientOn(t, s, "wiki.beta.example").get("/api/state"); got.Code != http.StatusOK {
		t.Errorf("the own domain: %d", got.Code)
	}
	if got := change("acme", url.Values{"do": {"add-host"}, "host": {"wiki.beta.example"}}); got != http.StatusBadRequest {
		t.Errorf("a domain twice: %d", got)
	}
	change("beta", url.Values{"do": {"remove-host"}, "host": {"wiki.beta.example"}})
	if got := newClientOn(t, s, "wiki.beta.example").get("/api/state"); got.Code != http.StatusNotFound {
		t.Errorf("a removed domain: %d", got.Code)
	}
	if got := change("main", url.Values{"do": {"lock"}}); got != http.StatusBadRequest {
		t.Errorf("Main cannot be locked: %d", got)
	}
	for _, id := range []string{"hoster", "nobody"} {
		if got := change(id, url.Values{"do": {"lock"}}); got != http.StatusNotFound {
			t.Errorf("%s: %d", id, got)
		}
	}
	if got := change("beta", url.Values{"do": {"invite"}, "admin_login": {"bea"}, "admin_name": {"Bea"}}); got != http.StatusNotFound {
		t.Errorf("an invitation: %d", got)
	}
	if got := change("beta", url.Values{"do": {"explode"}}); got != http.StatusNotFound {
		t.Errorf("an unknown action: %d", got)
	}

	// Renaming keeps the ID; deleting for good needs the tenant archived and its ID typed.
	if got := change("beta", url.Values{"do": {"rename"}, "name": {"Beta AG"}}); got != http.StatusOK || !strings.Contains(h.get("/hoster").Body.String(), "Beta AG") {
		t.Errorf("rename: %d", got)
	}
	if got := change("beta", url.Values{"do": {"rename"}, "name": {" "}}); got != http.StatusBadRequest {
		t.Errorf("an empty name: %d", got)
	}
	if got := change("beta", url.Values{"do": {"purge"}, "confirm": {"beta"}}); got != http.StatusBadRequest {
		t.Errorf("deleting an active tenant: %d", got)
	}
	if page := h.get("/hoster/tenants/beta").Body.String(); strings.Contains(page, `value="purge"`) {
		t.Error("an active tenant offers no delete for good")
	}
	change("beta", url.Values{"do": {"archive"}})
	if page := h.get("/hoster/tenants/beta").Body.String(); !strings.Contains(page, `value="purge"`) {
		t.Error("an archived tenant offers the delete for good")
	}
	if got := change("beta", url.Values{"do": {"purge"}, "confirm": {"Beta AG"}}); got != http.StatusBadRequest {
		t.Errorf("a confirmation that is not the ID: %d", got)
	}
	if got := change("main", url.Values{"do": {"purge"}, "confirm": {"main"}}); got != http.StatusBadRequest {
		t.Errorf("Main cannot be deleted: %d", got)
	}
	if got := h.form("/hoster/tenants/beta", url.Values{"do": {"purge"}, "confirm": {" beta "}}); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "Deleted for good: beta") {
		t.Fatalf("delete for good: %d", got.Code)
	}
	if _, err := database.Tenant(context.Background(), "beta"); !errors.Is(err, db.ErrNotFound) {
		t.Errorf("beta is still there: %v", err)
	}
	if got := h.get("/hoster/tenants/beta").Code; got != http.StatusNotFound {
		t.Errorf("its page: %d", got)
	}
}

// TestTheHosterSeesNoContent: the hoster sees counts, never content; its host has no way into a workspace; a tenant's
// administrator gets nowhere on the hoster's pages, nor does someone without the role of the app on the hoster host.
func TestTheHosterSeesNoContent(t *testing.T) {
	s, _, h := hosterSite(t)
	acme := newClient(t, s, acmeHost)
	acme.headers = gateHeaders
	team := acme.sharedCollection(`{"title":"Secret plans of acme"}`)
	plan := decode[apiNode](t, acme.post("/api/pages", `{"title":"Merger","parent":"`+team.ID+`"}`))
	acme.post("/api/pages/"+plan.ID+"/blocks", `{"md":"We buy the competitor."}`)
	for _, target := range []string{"/hoster", "/hoster/tenants/acme"} {
		body := h.get(target).Body.String()
		for _, secret := range []string{"Secret plans", "Merger", "competitor", team.ID} {
			if strings.Contains(body, secret) {
				t.Errorf("%s shows %q", target, secret)
			}
		}
	}
	if body := h.get("/hoster").Body.String(); !strings.Contains(body, "0.0 MB") && !strings.Contains(body, " MB") {
		t.Error("the hoster sees the usage")
	}
	for _, target := range []string{"/api/state", "/o/" + plan.ID, "/api/doc/" + plan.ID, "/api/trash", "/profile", "/admin/modules", "/api/search?q=competitor"} {
		if got := h.get(target); got.Code != http.StatusNotFound {
			t.Errorf("%s on the hoster host: %d", target, got.Code)
		}
	}
	if got := acme.get("/hoster"); got.Code != http.StatusNotFound {
		t.Errorf("a tenant's administrator on the hoster pages: %d", got.Code)
	}
	someone := newClient(t, s, hosterHost)
	someone.headers = bob
	if got := someone.get("/hoster"); got.Code != http.StatusNotFound {
		t.Errorf("someone without the role on the hoster host: %d", got.Code)
	}
}

// TestLimitsShowInTheWeb: past its limit of users, a tenant lets no new account in, also through the gate; past its
// limit of storage, a block is refused with a reason.
func TestLimitsShowInTheWeb(t *testing.T) {
	s, database := dbSite(t, Options{})
	anna := newClientWith(t, s, gateHeaders)
	team := anna.sharedCollection(`{"title":"Team"}`)
	plan := decode[apiNode](t, anna.post("/api/pages", `{"title":"Plan","parent":"`+team.ID+`"}`))
	max := 1
	storage := int64(20)
	if err := database.SetTenantLimits(context.Background(), tenant.Main, db.HosterLimits{MaxUsers: &max, MaxStorage: &storage}); err != nil {
		t.Fatal(err)
	}
	if got := newClientWith(t, s, bob).get("/api/state"); got.Code != http.StatusForbidden || !strings.Contains(got.Body.String(), "as many accounts") {
		t.Errorf("a new account past the limit: %d", got.Code)
	}
	if got := anna.get("/api/state"); got.Code != http.StatusOK {
		t.Errorf("a known account: %d", got.Code)
	}
	if got := anna.post("/api/pages/"+plan.ID+"/blocks", `{"md":"`+strings.Repeat("x", 21)+`"}`); got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), "quota") {
		t.Errorf("a block past the limit: %d %s", got.Code, got.Body.String())
	}
	if got := anna.post("/api/pages/"+plan.ID+"/blocks", `{"md":"fits"}`); got.Code != http.StatusCreated && got.Code != http.StatusOK {
		t.Errorf("a block that fits: %d", got.Code)
	}
}

// newClientOn is a client with anna's headers of the gate on a host.
func newClientOn(t *testing.T, s *site, host string) *client {
	c := newClient(t, s, host)
	c.headers = gateHeaders
	return c
}

func TestMeters(t *testing.T) {
	for _, c := range []struct {
		used, limit int64
		value, max  int64
		variant     string
	}{{1, 5, 1, 5, ""}, {9, 10, 9, 10, "warning"}, {10, 10, 1, 1, "destructive"}, {12, 10, 1, 1, "destructive"}, {0, 0, 1, 1, "destructive"}} {
		m := newMeter(c.used, c.limit)
		if m.Value != c.value || m.Max != c.max || m.Variant != c.variant {
			t.Errorf("%d of %d: %+v", c.used, c.limit, m)
		}
	}
}
