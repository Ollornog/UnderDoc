//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"testing"
)

// The rules page for agents (B-5): an administrator names a page of agent notes in its settings and takes it back;
// someone who is no administrator is offered neither.
func TestTheRulesPageIsNamedInTheSettings(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.signInWith("anna", "Anna Berg", "ops,underdoc-admin")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.eval(`window.made = null; (async () => {
		const r = await fetch('/api/pages', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({title: 'Rules', parent: '` + team + `', kind: 'agent-notes'})});
		const id = (await r.json()).id;
		await fetch('/api/pages/' + id + '/blocks', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({md: 'Write in English.'})});
		window.made = id;
	})()`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the page was not made")
	}
	page := b.eval(`window.made`)
	b.open(base + "/o/" + page)
	b.ready("Rules")
	if !b.waitFor(`!!document.querySelector('[data-kind-head]')`) {
		t.Fatalf("the page did not load: %s", b.eval(`JSON.stringify(Object.entries(D).filter(([k, d]) => d.n === 'Rules').map(([k, d]) => [k, d.kind, !!d.blocks, d.loading, d.t, d.db]))`))
	}
	b.clickOn(`[data-win="0"] [data-act="gear"][aria-label="Document settings"]`, 1)
	if !b.waitFor(`document.querySelector('aside[data-drawer] [data-setrules]')?.getAttribute('aria-pressed') === 'false'`) {
		t.Fatalf("the administrator is not offered the rules page: %s", b.eval(`document.querySelector('aside[data-drawer]')?.innerText`))
	}
	b.clickOn(`aside[data-drawer] [data-setrules]`, 1)
	if !b.waitFor(`!!document.querySelector('aside[data-drawer] [data-rules-page]') && document.querySelector('aside[data-drawer] [data-setrules]').getAttribute('aria-pressed') === 'true'`) {
		t.Fatalf("named, it does not say so: %s", b.eval(`document.querySelector('aside[data-drawer]')?.innerText`))
	}
	b.eval(`window.rp = null; fetch('/api/rules-page').then(r => r.json()).then(a => { window.rp = a.page; })`)
	if !b.waitFor(`window.rp === '` + page + `'`) {
		t.Errorf("the server's rules page: %s", b.eval(`String(window.rp)`))
	}
	b.shot("rules-page")
	// bob is no administrator: no button.
	b.signInWith("bob", "Bob", "ops")
	b.open(base + "/o/" + page)
	b.ready("Rules")
	if !b.waitFor(`!!document.querySelector('[data-kind-head]')`) {
		t.Fatalf("the page did not load: %s", b.eval(`JSON.stringify(Object.entries(D).filter(([k, d]) => d.n === 'Rules').map(([k, d]) => [k, d.kind, !!d.blocks, d.loading, d.t, d.db]))`))
	}
	b.clickOn(`[data-win="0"] [data-act="gear"][aria-label="Document settings"]`, 1)
	if !b.waitFor(`!!document.querySelector('aside[data-drawer] select[data-setkind]')`) {
		t.Fatal("bob's settings do not open")
	}
	if b.exists(`aside[data-drawer] [data-setrules]`) {
		t.Error("bob is offered the rules page")
	}
}

// An empty page of the database offers its kind in the settings, as one with blocks does (B-6, finding 14).
func TestAnEmptyPageOffersItsKind(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.signInWith("anna", "Anna Berg", "ops,underdoc-admin")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.eval(`window.made = null; (async () => {
		const r = await fetch('/api/pages', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({title: 'Empty', parent: '` + team + `'})});
		window.made = (await r.json()).id;
	})()`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the page was not made")
	}
	b.open(base + "/o/" + b.eval(`window.made`))
	b.ready("Empty")
	b.clickOn(`[data-win="0"] [data-act="gear"][aria-label="Document settings"]`, 1)
	if !b.waitFor(`!!document.querySelector('aside[data-drawer] select[data-setkind]')`) {
		t.Fatalf("an empty page offers no kind: %s", b.eval(`document.querySelector('aside[data-drawer]')?.innerText`))
	}
}
