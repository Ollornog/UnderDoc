//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"regexp"
	"strconv"
	"testing"
)

// trashButton marks a button of the row of the trash page that holds an entry of this name.
func trashButton(b *browser, name, act string) string {
	b.t.Helper()
	b.eval(`document.querySelectorAll('[data-probe]').forEach(n => n.removeAttribute('data-probe'));
		[...document.querySelectorAll('tr[data-trash]')].find(r => r.cells[0].textContent.trim() === ` + strconv.Quote(name) + `)
			?.querySelector('[data-act="` + act + `"]')?.setAttribute('data-probe', '')`)
	return `tr[data-trash] [data-probe]`
}

// The right click deletes an entry after asking in place; it goes from the lists, from the window and from the database,
// and the trash, reached from the free part of nav2, brings it back where it lay or deletes it for good, asking as well.
func TestDeleteAndRestoreFromTheTrash(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	tops := func() string {
		return b.eval(`[...document.querySelectorAll('.nav3 .cheadrow[data-cdrop="` + team + `"] + ul > li > .row')].map(n => n.textContent.trim()).join(', ')`)
	}
	b.clickOn(rowNamed(b, "B"), 1)
	b.ready("B")
	b.rightClickOn(rowNamed(b, "B"))
	if got := b.eval(`(() => { const l = document.querySelector('.ctxpop').lastElementChild; return l.textContent.trim() + '|' + l.previousElementSibling.tagName + '|' + !!l.querySelector('svg'); })()`); got != "Delete|HR|true" {
		t.Errorf("Delete is last, after a line, with its icon: %q", got)
	}
	b.clickOn(probe(b, `.ctxpop button`, "Delete"), 1)
	if got := b.eval(`document.querySelector('.ctxpop')?.lastElementChild.textContent.trim()`); got != "Delete? Yes" {
		t.Fatalf("the first click asks in place: %q", got)
	}
	b.shot("delete-asks")
	if got := tops(); got != "A, B, C" {
		t.Errorf("asking deletes nothing: %q", got)
	}
	b.clickOn(probe(b, `.ctxpop button`, "Delete? Yes"), 1)
	if !b.waitFor(`![...document.querySelectorAll('.nav3 .row')].some(n => n.textContent.trim() === 'B')`) {
		t.Fatalf("B is still listed: %q", tops())
	}
	if got := b.eval(`document.querySelector('.dtitle h1')?.textContent || 'nothing'`); got == "B" {
		t.Error("the window still shows B")
	}
	// C goes into the trash too, to be deleted for good below.
	b.eval(`window.done = false; fetch('/api/objects/' + D[` + strconv.Quote(rowID(b, "C")) + `].sid, {method: 'DELETE'}).then(() => { window.done = true; })`)
	if !b.waitFor(`window.done`) {
		t.Fatal("C was not deleted")
	}
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := tops(); got != "A" {
		t.Errorf("after a reload B and C stay deleted: %q", got)
	}
	// In nav2, the own collection may go, the private notes stay.
	b.rightClickOn(probe(b, `#clist .collrow`, "Team"))
	if got := b.eval(`(() => { const l = document.querySelector('.ctxpop').lastElementChild; return l.textContent.trim() + '|' + l.disabled; })()`); got != "Delete|false" {
		t.Errorf("Delete of a collection in nav2: %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
	b.rightClickOn(probe(b, `#clist .collrow`, "My notes"))
	if got := b.eval(`(() => { const l = document.querySelector('.ctxpop').lastElementChild; return l.textContent.trim() + '|' + l.disabled + '|' + l.title; })()`); got != "Delete|true|Private notes stay" {
		t.Errorf("Delete of the private notes: %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
	h, _ := strconv.ParseFloat(b.eval(`innerHeight`), 64)
	n2 := b.rect(`.nav2`)
	b.call("Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": n2.X, "y": h - 6}, nil)
	for _, kind := range []string{"mousePressed", "mouseReleased"} {
		b.call("Input.dispatchMouseEvent", map[string]any{"type": kind, "x": n2.X, "y": h - 6, "button": "right", "buttons": 2, "clickCount": 1}, nil)
	}
	if !b.waitFor(`!!document.querySelector('.ctxpop')`) {
		t.Fatal("no menu on the free part of nav2")
	}
	b.clickOn(probe(b, `.ctxpop button`, "Trash"), 1)
	if !b.waitFor(`document.querySelectorAll('tr[data-trash]').length === 2`) {
		t.Fatalf("the trash page: %s", b.eval(`document.querySelector('article.doc')?.innerText`))
	}
	if got := b.eval(`document.querySelector('.dtitle h1').textContent + ' | ' + [...document.querySelectorAll('tr[data-trash]')].map(r => [...r.cells].slice(0, 4).map(c => c.textContent.trim()).join(' / ')).join(' | ')`); !regexp.MustCompile(`^Trash \| C / Team / \S+ \d\d \d\d:\d\d / Anna Berg \| B / Team / \S+ \d\d \d\d:\d\d / Anna Berg$`).MatchString(got) {
		t.Errorf("the trash lists name, place, when and who, newest first: %q", got)
	}
	b.shot("trash")
	b.clickOn(trashButton(b, "C", "trpurge"), 1)
	if got := b.eval(`document.querySelector('tr[data-trash] [aria-pressed="true"]')?.textContent.trim()`); got != "Delete for good? Yes" {
		t.Fatalf("deleting for good asks in place: %q", got)
	}
	if got := b.eval(`document.querySelectorAll('tr[data-trash]').length`); got != "2" {
		t.Errorf("asking deletes nothing: %s", got)
	}
	b.clickOn(trashButton(b, "C", "trpurge"), 1)
	if !b.waitFor(`document.querySelectorAll('tr[data-trash]').length === 1`) {
		t.Fatal("C is still in the trash")
	}
	b.clickOn(trashButton(b, "B", "trrestore"), 1)
	if !b.waitFor(`document.querySelector('article.doc')?.textContent.includes('The trash is empty.')`) {
		t.Errorf("the trash after restoring: %s", b.eval(`document.querySelector('article.doc')?.innerText`))
	}
	if !b.waitFor(`[...document.querySelectorAll('.nav3 .row')].some(n => n.textContent.trim() === 'B')`) {
		t.Fatalf("B is not back: %q", tops())
	}
	if got := tops(); got != "A, B" {
		t.Errorf("B is back where it lay, at the end: %q", got)
	}
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := tops(); got != "A, B" {
		t.Errorf("after a reload B is back and C gone: %q", got)
	}
}

// rowID returns the key of the workspace for the entry of nav3 with this name.
func rowID(b *browser, name string) string {
	b.t.Helper()
	return b.eval(`[...document.querySelectorAll('.nav3 .row')].find(n => n.textContent.trim() === ` + strconv.Quote(name) + `)?.dataset.row`)
}

// The trash is emptied as a whole, asking first in place.
func TestEmptyTheTrash(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.eval(`window.gone = false; (async () => { const s = await (await fetch('/api/state')).json(); const t = s.collections.find(c => c.id === '` + team + `');
		for (const k of t.kids.slice(0, 2)) await fetch('/api/objects/' + k.id, {method: 'DELETE'}); window.gone = true; })()`)
	if !b.waitFor(`window.gone`) {
		t.Fatal("nothing deleted")
	}
	h, _ := strconv.ParseFloat(b.eval(`innerHeight`), 64)
	n2 := b.rect(`.nav2`)
	b.call("Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": n2.X, "y": h - 6}, nil)
	for _, kind := range []string{"mousePressed", "mouseReleased"} {
		b.call("Input.dispatchMouseEvent", map[string]any{"type": kind, "x": n2.X, "y": h - 6, "button": "right", "buttons": 2, "clickCount": 1}, nil)
	}
	b.clickOn(probe(b, `.ctxpop button`, "Trash"), 1)
	if !b.waitFor(`document.querySelectorAll('tr[data-trash]').length === 2`) {
		t.Fatalf("two in the trash: %s", b.eval(`document.querySelector('article.doc')?.innerText`))
	}
	b.clickOn(`[data-act="trempty"]`, 1)
	if got := b.eval(`[document.querySelector('[data-act="trempty"]').textContent.trim(), document.querySelectorAll('tr[data-trash]').length].join(' | ')`); got != "Empty the trash? Yes | 2" {
		t.Errorf("it asks first: %q", got)
	}
	b.clickOn(`[data-act="trempty"]`, 1)
	if !b.waitFor(`document.querySelector('article.doc')?.innerText.includes('The trash is empty.')`) {
		t.Errorf("emptied: %s", b.eval(`document.querySelector('article.doc')?.innerText`))
	}
}
