//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import "testing"

// Quick notes are a document type (ADR-16, M-8): New with Quick notes chosen puts a card on a board in the private
// notes, without choosing a place first; the board has a field on top for the next card; and New keeps the last choice
// of where it lands.
func TestQuickNotes(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makePage(b, "Somewhere", "x") // makes a collection Team … for the last choice
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.watch()
	b.clickOn(`[data-act="newmenu"]`, 1)
	if got := b.eval(`document.querySelector('.newpop [data-act="into"][data-id="qn"]')?.getAttribute('aria-pressed') + ' ' + document.querySelector('.newpop [data-act="into"][data-id="qn"]')?.disabled`); got != "true false" {
		t.Fatalf("New lands in Quick notes at first: %q", got)
	}
	b.typeText("Call the crane company")
	b.press("Enter", "Enter", 13, 0)
	if !b.waitFor(`D[at(0)?.d]?.t === 'notes' && D[at(0).d].real && [...document.querySelectorAll('article.typeset .keep .kard')].map(k => k.textContent.trim()).join(',') === 'Call the crane company'`) {
		t.Fatalf("the card is on the board of quick notes: %s", b.eval(`document.querySelector('article.typeset')?.innerHTML.slice(0, 300)`))
	}
	board := b.eval(`D[at(0).d].sid`)
	b.clickOn(`article.typeset .noteadd`, 1)
	b.typeText("Spare antenna: check the shelf")
	b.press("Enter", "Enter", 13, 0)
	if !b.waitFor(`[...document.querySelectorAll('article.typeset .keep .kard')].map(k => k.textContent.trim()).join(',') === 'Spare antenna: check the shelf,Call the crane company' && document.activeElement?.matches('.noteadd') && !document.activeElement.value`) {
		t.Errorf("the field on top adds a card, newest first, and stays ready: %s", b.eval(`[...document.querySelectorAll('article.typeset .kard')].map(k => k.textContent.trim()).join(',')`))
	}
	b.shot("quick-notes")
	if got := b.stored(board); got != "b2:Spare antenna: check the shelf | b1:Call the crane company" {
		t.Errorf("the cards are blocks of the board: %q", got)
	}

	// The last choice stays: a collection chosen in New is chosen again after a reload.
	coll := b.eval(`(() => { const p = Object.keys(D).find(k => D[k].sid === '` + team + `'); let c = p; while (PARENT[c]) c = PARENT[c]; return COLL[p] ? COLL[p].id : c; })()`)
	b.clickOn(`[data-act="newmenu"]`, 1)
	b.clickOn(`.newpop [data-act="into"][data-id="`+coll+`"]`, 1)
	b.press("Escape", "Escape", 27, 0)
	b.settingsSent()
	b.open(base + "/o/" + board)
	b.ready("Quick notes")
	if !b.waitFor(`document.querySelectorAll('article.typeset .keep .kard').length === 2`) {
		t.Errorf("after a reload the board shows its cards: %s", b.eval(`document.querySelector('article.typeset')?.innerHTML.slice(0, 300)`))
	}
	b.clickOn(`[data-act="newmenu"]`, 1)
	if got := b.eval(`document.querySelector('.newpop [aria-pressed="true"][data-act="into"]')?.dataset.id`); got != coll {
		t.Errorf("New keeps the last choice: %q, want %q", got, coll)
	}
	b.clean("quick notes")
}
