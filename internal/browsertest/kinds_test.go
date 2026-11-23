//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"strings"
	"testing"
)

// A page shows its kind and side under the title, the settings choose the kind, and whoever may change the page sees
// the writing rules it breaks, with how to fix each (T-20).
func TestPageKindsAndWritingRules(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.eval(`window.made = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		const ref = await post('/api/pages', {title: 'Network', parent: '` + team + `', kind: 'reference'});
		await post('/api/pages/' + ref.id + '/blocks', {md: '## Backup\n\nThe backup host is backup.example.com.'});
		const guide = await post('/api/pages', {title: 'Restore', parent: '` + team + `'});
		await post('/api/pages/' + guide.id + '/blocks', {md: '- Files come back from backup.example.com.'});
		window.made = guide.id;
	})()`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the pages for the test were not made")
	}
	guide := b.eval(`window.made`)
	b.open(base + "/o/" + guide)
	b.ready("Restore")
	if got := b.eval(`[...document.querySelectorAll('[data-kind-head] > .badge')].map(x => x.textContent).join(',')`); got != "No kind,For people" {
		t.Errorf("a page without a kind is for people: %q", got)
	}
	if got := b.eval(`document.querySelector('[data-findings] summary')?.textContent`); got != "1 hint on writing" {
		t.Errorf("the hints of a page without a kind: %q", got)
	}
	b.watch()
	b.clickOn(`[data-win="0"] [data-act="gear"][aria-label="Document settings"]`, 1)
	if !b.waitFor(`!!document.querySelector('aside[data-drawer] select[data-setkind]')`) {
		t.Fatal("the settings choose the kind")
	}
	b.eval(`(() => { const s = document.querySelector('aside[data-drawer] select[data-setkind]'); s.value = 'how-to'; s.dispatchEvent(new Event('change', {bubbles: true})); })()`)
	if !b.waitFor(`window.writes.some(w => w.endsWith('/kind')) && [...document.querySelectorAll('[data-kind-head] > .badge')].map(x => x.textContent).join(',') === 'How-to,For people'`) {
		t.Fatalf("the kind is kept and shown: %s | %s", b.eval(`window.writes.join(', ')`), b.eval(`document.querySelector('[data-kind-head]')?.textContent`))
	}
	if got := b.eval(`[...document.querySelectorAll('[data-findings] li')].map(x => x.dataset.finding).join(',')`); got != "START,STEPS,COPY" {
		t.Errorf("a how-to that starts with a list, has no steps and copies a value: %q", got)
	}
	b.clickOn(`[data-findings] summary`, 1)
	if got := b.eval(`document.querySelector('[data-findings] li[data-finding="COPY"]')?.textContent`); !strings.Contains(got, "backup.example.com") || !strings.Contains(got, "Network") || !strings.Contains(got, "Embed instead of copying") {
		t.Errorf("COPY names the value, its source and the fix: %q", got)
	}
	// The opened list settles before the picture (C22 lets details grow).
	b.waitFor(`(() => { window.shotAt = window.shotAt || Date.now(); return Date.now() - window.shotAt > 600; })()`)
	b.shot("page-kind-findings")

	// The list of problems has the writing rules.
	b.eval(`openDoc('problems:'); loadProblems();`)
	if !b.waitFor(`!!document.querySelector('[data-rule="` + guide + `:COPY"]')`) {
		t.Errorf("the problems list the rules: %s", b.eval(`document.querySelector('article.typeset')?.innerText.slice(0, 300)`))
	}
	b.clean("page-kinds")
}
