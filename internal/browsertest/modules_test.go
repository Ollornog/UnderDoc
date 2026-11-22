//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
	"github.com/Ollornog/UnderDoc/internal/web"
)

const testPassword = "a long enough password"

// signUp accepts an invitation with a password, by mouse and keyboard, and waits until the browser has gone on.
func (b *browser) signUp(address string) {
	b.t.Helper()
	b.open(address)
	b.typeInto(`#password`, testPassword)
	b.typeInto(`#repeat`, testPassword)
	b.pressOn(`form[action$="/password"] button[type="submit"]`)
	if !b.waitFor(`!location.pathname.startsWith('/invite/')`) {
		b.t.Fatalf("the invitation did not go on: %s", b.eval(`document.querySelector('[data-test="error"]')?.textContent`))
	}
}

// An administrator switches Documents off on the page of the modules, and the workspace shows none of it; switched on
// again, everything is back. The profile leads there (ADR-25).
func TestSwitchingModulesWithTheMouse(t *testing.T) {
	b, base, database := setupSignIn(t, db.MethodPassword)
	b.signUp(base + invitation(t, database, "tom", "Tom", true))
	b.open(base + "/")
	team := makeTeam(b)
	b.clickOn(`[data-act="view"][data-id="profile"]`, 1)
	b.clickOn(`[data-act="sec"][data-id="mods"]`, 1)
	if !b.waitFor(`(document.querySelector('.spanel')?.textContent || '').includes('✓ On')`) {
		t.Fatalf("the profile lists the modules: %s", b.eval(`document.querySelector('.spanel')?.textContent`))
	}
	b.clickOn(`a[href="/admin/modules"]`, 1)
	if !b.waitFor(`location.pathname === '/admin/modules'`) {
		t.Fatal("the profile leads to the modules")
	}
	clean(t, "modules", b.collect())
	b.pressOn(`#mod-documents`)
	b.pressOn(`form[action="/admin/modules"] button[type="submit"]`)
	if !b.waitFor(`(document.querySelector('[data-test="notice"]')?.textContent || '').includes('Nothing was deleted')`) {
		t.Fatal("saved")
	}
	b.shot("modules-admin")
	b.open(base + "/")
	if !b.waitFor(`!!document.querySelector('#root [aria-label=Nav1] > div')`) {
		t.Fatal("the workspace without Documents")
	}
	if got := b.eval(`fetch('/api/state').then(r => r.json()).then(s => { window.cols = s.collections.length; }); 'asked'`); got != "asked" {
		t.Fatal(got)
	}
	if !b.waitFor(`window.cols === 0`) {
		t.Errorf("collections while Documents is off: %s", b.eval(`String(window.cols)`))
	}
	b.open(base + "/admin/modules")
	b.pressOn(`#mod-documents`)
	b.pressOn(`form[action="/admin/modules"] button[type="submit"]`)
	b.waitFor(`!!document.querySelector('[data-test="notice"]')`)
	b.open(base + "/o/" + team)
	b.ready("Team")
}

// The hoster makes a tenant and locks it, by mouse and keyboard, on its own host; tenants live at their subdomains of
// localhost, which Chrome resolves by itself.
func TestTheHosterWithTheMouse(t *testing.T) {
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	hoster := tenant.With(context.Background(), tenant.Hoster)
	if err := database.SetSignIn(hoster, db.SignIn{Methods: []string{db.MethodPassword}}); err != nil {
		t.Fatal(err)
	}
	keys, _ := vault.New("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=") // a key for tests only
	b, base := setupWith(t, web.Options{DB: database, Vault: keys, Tenants: tenant.ModeMany, TenantDomain: "localhost", HosterHost: "localhost"}, nil)
	b.signIn(false)
	base = strings.Replace(base, "127.0.0.1", "localhost", 1)
	token, hash, _ := newInviteToken(t)
	if err := database.CreateInvite(hoster, hash, db.Invite{Login: "host", Name: "Hoster", Admin: true, Expires: inAnHour()}); err != nil {
		t.Fatal(err)
	}
	b.signUp(base + "/invite/" + token)
	if !b.waitFor(`location.pathname === '/hoster'`) {
		t.Fatalf("the hoster lands on its page: %s", b.eval(`location.href`))
	}
	clean(t, "hoster", b.collect())
	b.typeInto(`#new-id`, "beta")
	b.typeInto(`#new-name`, "Beta GmbH")
	b.typeInto(`#new-admin-login`, "bea")
	b.typeInto(`#new-admin-name`, "Bea")
	b.pressOn(`form[action="/hoster/tenants"] button[type="submit"]`)
	if !b.waitFor(`(document.querySelector('[data-test="invite-link"]')?.value || '').includes('beta.localhost')`) {
		t.Fatalf("the link for the first administrator: %s", b.eval(`document.querySelector('[data-test="error"]')?.textContent`))
	}
	link := b.eval(`document.querySelector('[data-test="invite-link"]').value`)
	b.pressOn(`form input[value="lock"] + button`)
	if !b.waitFor(`(document.querySelector('.card-description')?.textContent || '').includes('✗ locked')`) {
		t.Fatal("locked")
	}
	b.shot("hoster-tenant")
	// Chrome reaches subdomains of localhost over IPv6 only, where the test server does not listen; the request goes
	// from here, with the host name of the tenant.
	request, err := http.NewRequest(http.MethodGet, strings.Replace(base, "localhost", "127.0.0.1", 1)+"/sign-in", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "beta.localhost"
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusForbidden || !strings.Contains(link, "beta.localhost/invite/") {
		t.Errorf("a locked tenant: %d, link %s", response.StatusCode, link)
	}
}
