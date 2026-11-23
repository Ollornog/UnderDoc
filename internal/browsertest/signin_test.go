//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"strconv"
	"strings"
	"testing"
)

// whoAmI asks the server who the browser is signed in as, and whether as an agent.
func (b *browser) whoAmI() string {
	b.t.Helper()
	b.eval(`window.who = null; fetch('/api/state').then(r => r.ok ? r.json() : {user: {login: 'nobody ' + r.status}}).then(s => { window.who = s.user.login + (s.user.agent ? ' agent' : ''); })`)
	b.waitFor(`window.who !== null`)
	return b.eval(`window.who`)
}

// reachFor scrolls an element into the middle of the window, so the mouse can reach it.
func (b *browser) reachFor(selector string) {
	b.t.Helper()
	b.exists(selector)
	b.eval(`document.querySelector(` + strconv.Quote(selector) + `).scrollIntoView({block: 'center', behavior: 'instant'})`)
}

// pressOn clicks an element with the mouse, after scrolling to it.
func (b *browser) pressOn(selector string) {
	b.t.Helper()
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

// TestSignInThroughTheGate: the gate signs in (ADR-32). With its headers the workspace opens and the profile signs out at
// the gate; without them every page says "Not signed in", and none of the pages of the sign-in built in before answers.
func TestSignInThroughTheGate(t *testing.T) {
	b, base := setupWithDatabase(t)
	clean(t, "workspace", b.open(base+"/"))
	if !b.waitFor(`!!document.querySelector('#root [aria-label=Nav1] > div')`) {
		t.Fatal("the workspace opens through the gate")
	}
	if got := b.whoAmI(); got != "anna" {
		t.Errorf("signed in as %q", got)
	}
	b.clickOn(`[data-act="view"][data-id="profile"]`, 1)
	if !b.waitFor(`!!document.querySelector('[aria-label=Nav3] a[href="/.tinysesam/logout"]')`) {
		t.Fatalf("the profile signs out at the gate: %s", b.eval(`document.querySelector('[aria-label=Nav3]')?.innerText`))
	}
	if got := b.eval(`String(!!document.querySelector('a[href="/account/security"], form[action="/sign-out"]'))`); got != "false" {
		t.Error("the profile still leads to a sign-in of UnderDoc's own")
	}
	b.signIn(false)
	for _, path := range []string{"/", "/sign-in", "/register", "/account/security", "/admin/sign-in"} {
		b.open(base + path)
		if !b.waitFor(`document.title.startsWith('Not signed in')`) {
			t.Errorf("%s without the gate: %s", path, b.eval(`document.title`))
		}
	}
	if got := b.whoAmI(); got != "nobody 401" {
		t.Errorf("the API without the gate: %q", got)
	}
}

// TestAnAgentInTheBrowser: a request the gate signed in with a token, not a session, is an agent of its person (ADR-36):
// it reads and writes, but the rights dialog offers it nothing to grant, and the profile says what it is.
func TestAnAgentInTheBrowser(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.call("Network.setExtraHTTPHeaders", map[string]any{"headers": map[string]string{"Remote-Auth": "oauth", "Remote-Kind": "user", "Remote-User": "anna",
		"Remote-Id": "1007", "Remote-Name": "Anna Berg", "Remote-Groups": "ops"}}, nil)
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := b.whoAmI(); got != "anna agent" {
		t.Errorf("signed in as %q", got)
	}
	b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
	b.clickOn(`[data-act="rights"]`, 1)
	if !b.waitFor(`document.getElementById('udrights')?.open`) {
		t.Fatal("the rights dialog opens")
	}
	if !b.waitFor(`(document.getElementById('udrights').innerText || '').length > 0`) || b.eval(`String(!!document.querySelector('#udrights [data-rform="grant"]'))`) != "false" {
		t.Errorf("the agent may grant: %s", b.eval(`document.getElementById('udrights').innerText`))
	}
	b.eval(`document.getElementById('udrights').close()`)
	b.clickOn(`[data-act="view"][data-id="profile"]`, 1)
	if !b.waitFor(`(document.querySelector('[aria-label=Nav3]')?.innerText || '').includes('from an agent')`) {
		t.Errorf("the profile says it is an agent: %s", b.eval(`document.querySelector('[aria-label=Nav3]')?.innerText`))
	}
	if !strings.Contains(b.eval(`document.querySelector('[aria-label=Nav3]')?.innerText`), "Sign out") {
		t.Error("the agent signs out at the gate as well")
	}
}
