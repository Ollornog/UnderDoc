//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/auth"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
	"github.com/Ollornog/UnderDoc/internal/web"
)

// setupSignIn starts the handler with a database whose tenant lets people in the given ways, no gate in front, and a
// browser that sends no headers of a gate. The address uses localhost: passkeys need a host name, not an address.
func setupSignIn(t *testing.T, methods ...string) (*browser, string, *db.DB) {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if err := database.SetSignIn(tenant.With(context.Background(), tenant.Main), db.SignIn{Methods: methods}); err != nil {
		t.Fatal(err)
	}
	keys, err := vault.New("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=") // a key for tests only
	if err != nil {
		t.Fatal(err)
	}
	b, base := setupWith(t, web.Options{DB: database, Vault: keys}, nil)
	b.signIn(false)
	return b, strings.Replace(base, "127.0.0.1", "localhost", 1), database
}

// invitation makes an invitation in Main and returns its path.
func invitation(t *testing.T, database *db.DB, login, name string, admin bool) string {
	t.Helper()
	token, hash, err := auth.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.CreateInvite(tenant.With(context.Background(), tenant.Main), hash,
		db.Invite{Login: login, Name: name, Admin: admin, Expires: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	return "/invite/" + token
}

// authenticator gives the browser a passkey device of its own, which says yes by itself, in place of the one before.
func (b *browser) authenticator(previous string) string {
	b.t.Helper()
	if previous == "" {
		b.call("WebAuthn.enable", map[string]any{"enableUI": false}, nil)
	} else {
		b.call("WebAuthn.removeVirtualAuthenticator", map[string]any{"authenticatorId": previous}, nil)
	}
	var made struct{ AuthenticatorID string }
	b.call("WebAuthn.addVirtualAuthenticator", map[string]any{"options": map[string]any{
		"protocol": "ctap2", "transport": "internal", "hasResidentKey": true, "hasUserVerification": true,
		"isUserVerified": true, "automaticPresenceSimulation": true,
	}}, &made)
	return made.AuthenticatorID
}

// whoAmI asks the server who the browser is signed in as.
func (b *browser) whoAmI() string {
	b.t.Helper()
	b.eval(`window.who = null; fetch('/api/state').then(r => r.ok ? r.json() : {user: {login: 'nobody', method: String(r.status)}}).then(s => { window.who = s.user.login + ' ' + s.user.method; })`)
	b.waitFor(`window.who !== null`)
	return b.eval(`window.who`)
}

// reachFor scrolls an element into the middle of the window, so the mouse can reach it.
func (b *browser) reachFor(selector string) {
	b.t.Helper()
	b.exists(selector)
	b.eval(`document.querySelector(` + strconv.Quote(selector) + `).scrollIntoView({block: 'center', behavior: 'instant'})`)
}

// pressOn clicks an element with the mouse, after scrolling to it. A passkey button works only once signin.js has
// taken over the clicks, which a page that has just loaded may not have done yet, so it waits for that first.
func (b *browser) pressOn(selector string) {
	b.t.Helper()
	if strings.Contains(selector, "data-passkey") && !b.waitFor(`document.documentElement.dataset.passkeys === 'ready'`) {
		b.t.Fatal("signin.js did not take over the passkey buttons")
	}
	b.reachFor(selector)
	b.clickOn(selector, 1)
}

// clean fails on what went wrong while a page loaded: a blocked resource, an error in the console, or a break of the
// content security policy.
func clean(t *testing.T, name string, f findings) {
	t.Helper()
	if len(f.csp) > 0 || len(f.errors) > 0 {
		t.Errorf("%s: csp %v, errors %v", name, f.csp, f.errors)
	}
}

// typeInto clicks a field with the mouse and types into it.
func (b *browser) typeInto(selector, text string) {
	b.t.Helper()
	b.pressOn(selector)
	b.typeText(text)
}

// An invited person sets up a passkey with the mouse, lands in the workspace, signs out from the profile, and signs in
// again with the passkey alone (ADR-23).
func TestSignInWithAPasskey(t *testing.T) {
	b, base, database := setupSignIn(t, db.MethodPasskey)
	laptop := b.authenticator("")
	f := b.open(base + "/")
	if !b.waitFor(`location.pathname === '/sign-in'`) {
		t.Fatalf("a page without a sign-in leads to it: %s", b.eval(`location.href`))
	}
	clean(t, "sign-in", f)
	clean(t, "invitation", b.open(base+invitation(t, database, "pia", "Pia Holm", false)))
	b.shot("signin-welcome")
	if got := b.eval(`document.querySelector('[data-test="login"]').textContent`); got != "pia" {
		t.Errorf("the login of the invitation: %q", got)
	}
	b.typeInto(`#passkey-name`, "Laptop")
	b.pressOn(`[data-passkey-create]`)
	if !b.waitFor(`location.pathname === '/' && !!document.querySelector('#root [aria-label=Nav1] > div')`) {
		t.Fatalf("after the passkey: %s %s", b.eval(`location.href`), b.eval(`document.querySelector('[data-passkey-error]')?.textContent`))
	}
	b.clickOn(`[data-act="view"][data-id="profile"]`, 1)
	b.clickOn(`form[action="/sign-out"] button`, 1)
	if !b.waitFor(`location.pathname === '/sign-in' && !!document.querySelector('[data-test="notice"]')`) {
		t.Fatalf("signed out: %s", b.eval(`location.href`))
	}
	b.shot("signin-passkey")
	b.pressOn(`[data-passkey-sign-in]`)
	if !b.waitFor(`location.pathname === '/' && !!document.querySelector('#root [aria-label=Nav1] > div')`) {
		t.Fatalf("signed in with the passkey: %s %s", b.eval(`location.href`), b.eval(`document.querySelector('[data-passkey-error]')?.textContent`))
	}
	if got := b.whoAmI(); got != "pia passkey" {
		t.Errorf("signed in as %q", got)
	}
	// The security page adds a passkey of a second device, with the mouse as well.
	clean(t, "security", b.open(base+"/account/security"))
	b.authenticator(laptop)
	b.typeInto(`#passkey-name`, "Phone")
	b.pressOn(`[data-passkey-create]`)
	if !b.waitFor(`document.querySelectorAll('[data-test="passkey"]').length === 2`) {
		t.Fatalf("a second passkey: %s", b.eval(`document.querySelector('[data-passkey-error]')?.textContent`))
	}
	if got := b.eval(`[...document.querySelectorAll('[data-test="passkey"] td:first-child')].map(n => n.textContent).join(',')`); got != "Laptop,Phone" {
		t.Errorf("the passkeys: %q", got)
	}
	b.shot("signin-security")
}

// An invited person sets up a password by typing, and signs in with it; a wrong one says so on the page.
func TestSignInWithAPassword(t *testing.T) {
	b, base, database := setupSignIn(t, db.MethodPassword)
	const password = "a long enough password"
	clean(t, "invitation", b.open(base+invitation(t, database, "quinn", "Quinn", true)))
	b.typeInto(`#password`, password)
	b.typeInto(`#repeat`, password)
	b.pressOn(`form[action$="/password"] button[type="submit"]`)
	if !b.waitFor(`location.pathname === '/' && !!document.querySelector('#root [aria-label=Nav1] > div')`) {
		t.Fatalf("after the password: %s", b.eval(`location.href`))
	}
	b.clickOn(`[data-act="view"][data-id="profile"]`, 1)
	b.clickOn(`form[action="/sign-out"] button`, 1)
	if !b.waitFor(`location.pathname === '/sign-in'`) {
		t.Fatal("signed out")
	}
	b.typeInto(`#login`, "quinn")
	b.typeInto(`#password`, "not the password")
	b.pressOn(`form[action="/sign-in/password"] button`)
	if !b.waitFor(`!!document.querySelector('[data-test="error"]')`) {
		t.Fatal("a wrong password says so")
	}
	if got := b.eval(`document.querySelector('#login').value`); got != "quinn" {
		t.Errorf("the login stays filled in: %q", got)
	}
	b.shot("signin-wrong")
	b.typeInto(`#password`, password)
	b.pressOn(`form[action="/sign-in/password"] button`)
	if !b.waitFor(`location.pathname === '/' && !!document.querySelector('#root [aria-label=Nav1] > div')`) {
		t.Fatalf("signed in: %s", b.eval(`location.href`))
	}
}

// An administrator turns registration on and invites someone, with the mouse and the keyboard, on the sign-in page of
// the administration; the profile leads there.
func TestAdministratorsSetUpTheSignIn(t *testing.T) {
	b, base, database := setupSignIn(t, db.MethodPassword)
	const password = "a long enough password"
	b.open(base + invitation(t, database, "rosa", "Rosa", true))
	b.typeInto(`#password`, password)
	b.typeInto(`#repeat`, password)
	b.pressOn(`form[action$="/password"] button[type="submit"]`)
	if !b.waitFor(`location.pathname === '/' && !!document.querySelector('#root [aria-label=Nav1] > div')`) {
		t.Fatal("signed in")
	}
	b.clickOn(`[data-act="view"][data-id="profile"]`, 1)
	b.clickOn(`[data-act="sec"][data-id="signin"]`, 1)
	b.clickOn(`a[href="/admin/sign-in"]`, 1)
	if !b.waitFor(`location.pathname === '/admin/sign-in'`) {
		t.Fatalf("the profile leads to the sign-in settings: %s", b.eval(`location.href`))
	}
	clean(t, "admin", b.collect())
	b.pressOn(`#registration`)
	b.pressOn(`form[action="/admin/sign-in"] button[type="submit"]`)
	if !b.waitFor(`document.querySelector('[data-test="notice"]')?.textContent.trim() === 'Saved.'`) {
		t.Fatalf("saved: %s", b.eval(`document.querySelector('[data-test="error"]')?.textContent`))
	}
	if s, _ := database.SignIn(tenant.With(context.Background(), tenant.Main)); !s.Registration {
		t.Error("registration is not on")
	}
	b.typeInto(`#invite-login`, "sam")
	b.typeInto(`#invite-name`, "Sam Berg")
	b.pressOn(`form[action="/admin/invites"] button`)
	if !b.waitFor(`(document.querySelector('[data-test="invite-link"]')?.value || '').includes('/invite/')`) {
		t.Fatalf("no invitation link: %s | %s | %s", b.eval(`location.href`), b.eval(`document.querySelector('[data-test="error"]')?.textContent`),
			b.eval(`document.querySelector('#invite-login').value + '/' + document.querySelector('#invite-name').value`))
	}
	b.shot("signin-admin")
	link := b.eval(`document.querySelector('[data-test="invite-link"]').value`)
	if !strings.HasPrefix(link, base+"/invite/") {
		t.Errorf("the link: %q", link)
	}
	// C22 takes over the button that copies it (input-group with data-copy).
	if !b.waitFor(`document.querySelector('#invite-link + [data-copy]')?.dataset.c22CopyWired === '1'`) {
		t.Error("the button to copy the link is not C22's")
	}
}

func newInviteToken(t *testing.T) (string, []byte, error) {
	t.Helper()
	return auth.NewToken()
}

func inAnHour() time.Time { return time.Now().Add(time.Hour) }
