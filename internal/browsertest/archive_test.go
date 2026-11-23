//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"testing"
)

// The archive of spaces in the workspace (T-34): the owner archives a space in its overview, it leaves the tree, comes
// back under "Show the archive" read only, opens by its address, and its owner restores it there; bob, who edits it,
// is offered neither.
func TestArchiveASpace(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	if !b.exists(`[data-act="archive"][data-id="` + team + `"]`) {
		t.Fatalf("the owner is not offered to archive: %s", b.eval(`document.querySelector('[data-art]')?.innerText`))
	}
	b.eval(`window.confirm = () => true`)
	b.clickOn(`[data-act="archive"][data-id="`+team+`"]`, 1)
	if !b.waitFor(`![...document.querySelectorAll('#clist [data-favid]')].some(n => n.textContent.trim() === 'Team')`) {
		t.Fatalf("the archived space is still in nav2: %s", b.eval(`document.getElementById('clist').innerText`))
	}
	b.shot("archive-done")

	// Shown on request, marked, read only.
	b.clickOn(`[data-archive-toggle]`, 1)
	if !b.waitFor(`document.querySelector('[data-archive-list] [data-archived="1"]')?.textContent.trim() === 'Team'`) {
		t.Fatalf("the archive does not list Team: %s", b.eval(`document.querySelector('[aria-label=Nav2]').innerText`))
	}
	if got := b.eval(`document.querySelector('[data-archive-toggle]').getAttribute('aria-pressed')`); got != "true" {
		t.Errorf("the toggle does not say it is on: %q", got)
	}
	b.shot("archive-shown")
	b.open(base + "/o/" + team)
	b.ready("Team")
	if !b.exists(`[data-archived-note]`) || !b.exists(`[data-act="unarchive"][data-id="`+team+`"]`) || b.exists(`[data-cname]`) {
		t.Errorf("the overview of the archived space: %s", b.eval(`document.querySelector('[data-art]')?.innerText`))
	}
	if b.exists(`[aria-label=Nav3] header[data-cdrop="` + team + `"] [data-act="addmenu"]`) {
		t.Error("nav3 offers New in the archive")
	}

	// bob edits Team, but archives and restores nothing.
	b.signInWith("bob", "Bob", "ops")
	b.open(base + "/o/" + team)
	b.ready("Team")
	if b.exists(`[data-act="unarchive"]`) || b.exists(`[data-act="archive"]`) {
		t.Error("bob is offered to archive or restore")
	}
	if !b.exists(`[data-archived-note]`) {
		t.Error("bob does not see that it is archived")
	}

	// The owner restores it; it comes back into nav2.
	b.signIn(true)
	b.open(base + "/o/" + team)
	b.ready("Team")
	b.clickOn(`[data-act="unarchive"][data-id="`+team+`"]`, 1)
	if !b.waitFor(`[...document.querySelectorAll('#clist [data-favid]')].some(n => n.textContent.trim() === 'Team') && !document.querySelector('[data-archived-note]')`) {
		t.Fatalf("restored, Team is not back in nav2: %s", b.eval(`document.getElementById('clist')?.innerText`))
	}
}

// The search leaves the archive out until the filter asks for it (T-34).
func TestSearchTheArchive(t *testing.T) {
	b, base := setupWithSearch(t)
	b.open(base + "/")
	b.ready("")
	page := makePage(b, "Kartenzahlung", "Wie man mit Karte zahlt.")
	b.indexed("Kartenzahlung")
	b.eval(`window.done = false; (async () => {
		const st = await (await fetch('/api/state')).json();
		const team = st.collections.find(c => (c.kids || []).some(k => k.id === '` + page + `'));
		await fetch('/api/objects/' + team.id + '/archive', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({archived: true})});
		window.done = true;
	})()`)
	if !b.waitFor(`window.done`) {
		t.Fatal("the space was not archived")
	}
	b.open(base + "/")
	b.ready("")
	b.press("/", "Slash", 191, 0)
	b.typeText("Kartenzahlung")
	if !b.waitFor(`document.getElementById('sres')?.textContent.includes('Nothing found')`) {
		t.Fatalf("the archive is found unasked: %s", b.eval(`document.getElementById('sres')?.innerText`))
	}
	b.clickOn(`[aria-label=Nav3] [data-act="filters"]`, 1)
	b.clickOn(`[aria-label=Nav3] [data-sact="archive"]`, 1)
	if !b.waitFor(`document.querySelectorAll('#sres [data-sact="go"]').length === 1`) {
		t.Fatalf("the filter does not find the archive: %s", b.eval(`document.getElementById('sres')?.innerText`))
	}
	if got := b.eval(`document.querySelector('[data-sact="archive"]').getAttribute('aria-pressed')`); got != "true" {
		t.Errorf("the filter does not say it is on: %q", got)
	}
	b.shot("search-archive")
}
