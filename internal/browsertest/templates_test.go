//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"strings"
	"testing"
)

// Templates start new pages (M-8): one of UnderDoc's own from New at a collection, a page offered from its gear, and that
// page from New in the top bar with the title typed there.
func TestTemplatesStartNewPages(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.eval(`window.made = null; fetch('/api/collections', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: '{"title":"Team"}'}).then(r => r.json()).then(j => { window.made = j.id; })`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the collection for the test was not made")
	}
	team := b.eval(`window.made`)
	b.open(base + "/o/" + team)
	b.ready("Team")
	b.watch()

	b.clickOn(`.cheadrow[data-cdrop="`+team+`"] .newplus`, 1)
	if got := b.eval(`[...document.querySelectorAll('.cmdpop [data-act="addtpl"]')].map(x => x.textContent).join(',')`); got != "Meeting notes,How-to,Runbook,Decision" {
		t.Errorf("New at a collection offers UnderDoc's templates: %q", got)
	}
	b.shot("templates-new-at-collection")
	b.clickOn(`.cmdpop [data-act="addtpl"][data-id="builtin:meeting"]`, 1)
	if !b.waitFor(`(EDS[at(0)?.d]?.ed.text() || '').includes('## Agenda') && document.activeElement?.classList.contains('cm-content')`) {
		t.Fatalf("the page starts with the template, in the editor: %q", b.eval(`JSON.stringify({d: at(0)?.d, blocks: D[at(0)?.d]?.blocks?.length, loaded: D[at(0)?.d]?.loaded, can: D[at(0)?.d]?.canEdit, eds: Object.keys(EDS), path: location.pathname, art: document.querySelector("article.doc")?.innerHTML.slice(0,200)})`))
	}
	meeting := b.eval(`location.pathname.slice(3)`)
	if got := b.eval(`document.querySelector('[data-win="0"] .wtitle, [data-win="0"] h1')?.textContent || document.title`); !strings.Contains(got, "Meeting notes") {
		t.Errorf("the page is named after the template: %q", got)
	}

	b.clickOn(`[data-win="0"] [data-act="gear"][aria-label="Document settings"]`, 1)
	b.clickOn(`.fdrawer [data-act="settpl"][data-id="offered"]`, 1)
	if !b.waitFor(`window.writes.some(w => w.endsWith('/template')) && S.tpls.some(x => x.id === '` + meeting + `')`) {
		t.Fatalf("the page is offered as a template: %s", b.eval(`window.writes.join(', ')`))
	}
	b.clickOn(`[data-act="newmenu"]`, 1)
	b.clickOn(`.newpop [data-act="into"][data-id="`+team+`"]`, 1)
	b.typeText("Weekly")
	b.clickOn(`.newpop [data-act="newtpl"][data-id="`+meeting+`"]`, 1)
	if !b.waitFor(`location.pathname.slice(3) !== '` + meeting + `' && (EDS[at(0)?.d]?.ed.text() || '').includes('## Decisions')`) {
		t.Fatalf("New in the top bar starts from the offered page: %q", b.eval(`EDS[at(0)?.d]?.ed.text()`))
	}
	weekly := b.eval(`location.pathname.slice(3)`)
	if got := b.stored(weekly); !strings.HasPrefix(got, "b1:**Date:**") || !strings.Contains(got, "b5:## To-dos") {
		t.Errorf("the server has the blocks: %q", got)
	}
	b.eval(`window.got = null; fetch('/api/doc/` + weekly + `').then(r => r.json()).then(d => { window.got = d.n; })`)
	if !b.waitFor(`window.got === 'Weekly'`) {
		t.Errorf("the title typed in New: %q", b.eval(`window.got`))
	}
	b.clean("templates")
}
