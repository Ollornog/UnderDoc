//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// Blocks of a page of the database: their lines, dragging them by the handle, and the bar of the editor as a row of the
// window. Pointer input is real: pressed, moved and released as a hand does.

// makeShared makes a collection that others may comment on, and in it a page with three blocks; it returns the page.
func makeShared(b *browser, title string) string {
	b.t.Helper()
	b.eval(`window.made = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		const team = await post('/api/collections', {title: 'Team ' + Math.random().toString(36).slice(2, 6)});
		const page = await post('/api/pages', {title: ` + strconv.Quote(title) + `, parent: team.id});
		let after = '';
		for (const md of ['One', 'Two', 'Three']) after = (await post('/api/pages/' + page.id + '/blocks', {md, after})).id;
		await post('/api/objects/' + team.id + '/access', {access: 'comment'});
		window.made = page.id;
	})()`)
	if !b.waitFor(`!!window.made`) {
		b.t.Fatal("the page for the test was not made")
	}
	return b.eval(`window.made`)
}

// hold presses the handle, lifts it a little, moves in steps to y and stays there, pressed.
func (b *browser) hold(handle string, y float64) {
	b.t.Helper()
	a := b.rect(handle)
	b.mouse("mouseMoved", a.X, a.Y, 0, 0)
	b.mouse("mousePressed", a.X, a.Y, 1, 0)
	b.mouse("mouseMoved", a.X, a.Y+8, 0, 0)
	for step := 1; step <= 8; step++ {
		b.mouse("mouseMoved", a.X, a.Y+8+(y-a.Y-8)*float64(step)/8, 0, 0)
	}
	time.Sleep(100 * time.Millisecond)
}

// release lets go of the mouse at y.
func (b *browser) release(handle string, y float64) {
	b.t.Helper()
	b.mouse("mouseReleased", b.rect(handle).X, y, 1, 0)
}

const readOrder = `[...document.querySelectorAll('article.doc [data-b]')].map(x => x.dataset.b).join(' ')`

// In a block of the database a single newline is a line break: two quote lines, or two lines of a paragraph, read as
// two lines, and the editor shows them so too. The Markdown stays as written.
func TestTwoLinesOfABlockReadAsTwoLines(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := makePage(b, "Lines", "> a\n> b", "one\ntwo", "- item\n  more")
	b.open(base + "/o/" + page)
	b.ready("Lines")
	// The top of the line each word stands on.
	lines := `(sel, words) => words.map(w => { const root = document.querySelector(sel), walk = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
		for (let n = walk.nextNode(); n; n = walk.nextNode()) { const at = n.data.indexOf(w); if (at < 0) continue; const r = document.createRange(); r.setStart(n, at); r.setEnd(n, at + w.length); return Math.round(r.getBoundingClientRect().top); }
		return 'none'; })`
	for _, c := range []struct{ sel, a, b string }{
		{`article.doc [data-b="b1"] blockquote`, "a", "b"},
		{`article.doc [data-b="b2"] p`, "one", "two"},
		{`article.doc [data-b="b3"] li`, "item", "more"},
	} {
		got := b.eval(`(() => { const [x, y] = (` + lines + `)(` + strconv.Quote(c.sel) + `, [` + strconv.Quote(c.a) + `, ` + strconv.Quote(c.b) + `]); return x !== 'none' && y !== 'none' && y > x; })()`)
		if got != "true" {
			t.Errorf("%s: %q and %q stand on one line: %s", c.sel, c.a, c.b, b.eval(`document.querySelector(`+strconv.Quote(c.sel)+`)?.outerHTML`))
		}
	}
	b.shot("block-lines")
	b.clickOn(`[data-win="0"] .dstatus`, 1)
	if !b.waitFor(`!!document.querySelector('article.doc .cm-editor .cm-content')`) {
		t.Fatal("no editor")
	}
	if got := b.eval(`[...document.querySelectorAll('.cm-editor .cm-line')].filter(l => l.textContent.trim()).map(l => l.textContent.trim() + (l.classList.contains('cm-lp-quote') ? '(q)' : '')).join(' / ')`); got != "a(q) / b(q) / one / two / • item / more" {
		t.Errorf("the editor shows the lines as lines: %q", got)
	}
	if got := b.stored(page); got != "b1:> a\n> b | b2:one\ntwo | b3:- item\n  more" {
		t.Errorf("the Markdown stays as written: %q", got)
	}
}

// In reading, who may change the page drags a block by its handle: the block lifts, a gap opens between the blocks where
// it would land, and the drop moves it through the API, without a new version of any text. Escape gives up, a click
// still opens the menu, and someone who may only comment cannot drag.
func TestDraggingABlockInReading(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := makeShared(b, "Drag")
	versions := b.versions(page)
	b.open(base + "/o/" + page)
	b.ready("Drag")
	b.watch()
	if got := b.eval(`[...document.querySelectorAll('article.doc [data-b] > .bhandle')].map(h => h.hasAttribute('data-drag') + ' ' + getComputedStyle(h).touchAction).join(', ')`); got != "true none, true none, true none" {
		t.Errorf("the handles drag, also with a finger: %q", got)
	}
	// Escape gives up: One held below Three goes back, nothing is written.
	b.hold(`article.doc [data-b="b1"] > .bhandle`, b.rect(`article.doc [data-b="b3"]`).Bottom-2)
	if got := b.eval(`(() => { const g = document.querySelector('article.doc .dropgap.held'); return !!g && g.previousElementSibling?.dataset.b + ' ' + g.textContent.trim() + ' ' + getComputedStyle(document.querySelector('article.doc [data-b="b1"]')).display; })()`); got != "b3 One none" {
		t.Errorf("held below Three, the gap holds One and One is lifted: %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
	if got := b.eval(`!document.querySelector('.dropgap') + ' ' + ` + readOrder); got != "true b1 b2 b3" {
		t.Errorf("Escape puts it back: %q", got)
	}
	b.release(`article.doc [data-b="b1"] > .bhandle`, b.rect(`article.doc [data-b="b3"]`).Bottom-2)
	time.Sleep(200 * time.Millisecond)
	if got := b.eval(`window.writes.join(', ') + ' | ' + !!document.querySelector('.bmenu')`); got != " | false" {
		t.Errorf("after Escape nothing is written and no menu opens: %q", got)
	}
	// Three above One.
	top := b.rect(`article.doc [data-b="b1"]`).Top + 3
	b.hold(`article.doc [data-b="b3"] > .bhandle`, top)
	if got := b.eval(`(() => { const g = document.querySelector('article.doc .dropgap.held'); return !!g && (g.nextElementSibling?.dataset.b || 'end') + ' ' + g.textContent.trim(); })()`); got != "b1 Three" {
		t.Errorf("the gap opens before One, between blocks: %q", got)
	}
	b.shot("block-drag-reading")
	b.release(`article.doc [data-b="b3"] > .bhandle`, top)
	if !b.waitFor(readOrder + ` === 'b3 b1 b2' && !document.querySelector('.dropgap')`) {
		t.Errorf("the view shows the new order: %q", b.eval(readOrder))
	}
	time.Sleep(300 * time.Millisecond)
	if got := b.eval(`window.writes.join(', ') + ' | ' + !!document.querySelector('.bmenu')`); got != "POST /api/pages/"+page+"/blocks/b3/move | false" {
		t.Errorf("one move, and the drop opens no menu: %q", got)
	}
	if got := b.stored(page); got != "b3:Three | b1:One | b2:Two" {
		t.Errorf("the server moved it: %q", got)
	}
	if got := b.versions(page); got != versions {
		t.Errorf("no new version of the texts: %d, before %d", got, versions)
	}
	b.open(base + "/o/" + page)
	b.ready("Drag")
	if got := b.eval(readOrder); got != "b3 b1 b2" {
		t.Errorf("after a reload: %q", got)
	}
	// A click without moving opens the menu.
	b.clickOn(`article.doc [data-b="b2"] > .bhandle`, 1)
	if !b.waitFor(`!!document.querySelector('article.doc [data-b="b2"] .bmenu')`) {
		t.Errorf("a click opens the menu of the handle")
	}
	b.clean("drag in reading")

	// Bob may comment, not change: his handles do not drag.
	b.signInAs("bob", "Bob")
	b.open(base + "/o/" + page)
	b.ready("Drag")
	b.watch()
	if got := b.eval(`document.querySelectorAll('article.doc .bhandle[data-drag]').length + ' ' + document.querySelectorAll('article.doc .bhandle').length`); got != "0 3" {
		t.Errorf("a reader's handles do not drag: %q", got)
	}
	top = b.rect(`article.doc [data-b="b3"]`).Top + 3
	b.hold(`article.doc [data-b="b2"] > .bhandle`, top)
	if got := b.eval(`!!document.querySelector('.dropgap')`); got != "false" {
		t.Errorf("no gap for a reader")
	}
	b.release(`article.doc [data-b="b2"] > .bhandle`, top)
	time.Sleep(200 * time.Millisecond)
	if got := b.eval(readOrder + ` + ' | ' + window.writes.join(', ')`); got != "b3 b1 b2 | " {
		t.Errorf("nothing moves for a reader: %q", got)
	}
	b.clean("reader")
}

// A finger drags the handle too: touch input becomes pointer events, and the page does not scroll instead.
func TestDraggingABlockWithAFinger(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := makeShared(b, "Finger")
	b.open(base + "/o/" + page)
	b.ready("Finger")
	// fingerDrag drags a handle with a finger to y.
	fingerDrag := func(handle string, y float64, check func()) {
		h := b.rect(handle)
		touch := func(kind string, y float64) {
			points := []map[string]any{{"x": h.X, "y": y}}
			if kind == "touchEnd" {
				points = []map[string]any{}
			}
			b.call("Input.dispatchTouchEvent", map[string]any{"type": kind, "touchPoints": points}, nil)
		}
		touch("touchStart", h.Y)
		for step := 1; step <= 8; step++ {
			touch("touchMove", h.Y+(y-h.Y)*float64(step)/8)
		}
		time.Sleep(150 * time.Millisecond)
		check()
		touch("touchEnd", y)
	}
	fingerDrag(`article.doc [data-b="b3"] > .bhandle`, b.rect(`article.doc [data-b="b1"]`).Top+3, func() {
		if got := b.eval(`document.querySelector('article.doc .dropgap.held')?.nextElementSibling?.dataset.b`); got != "b1" {
			t.Errorf("the finger opens the gap before One: %q", got)
		}
	})
	if !b.waitFor(readOrder + ` === 'b3 b1 b2'`) {
		t.Errorf("the finger moved it: %q", b.eval(readOrder))
	}
	time.Sleep(300 * time.Millisecond)
	if got := b.stored(page); got != "b3:Three | b1:One | b2:Two" {
		t.Errorf("the server: %q", got)
	}
	// In the editor the held block's handle is gone from the page while the finger still moves.
	b.clickOn(`[data-win="0"] .dstatus`, 1)
	if !b.waitFor(`!!document.querySelector('.cm-ud-marker[data-edb="b2"] .bhandle')`) {
		t.Fatal("no editor")
	}
	fingerDrag(`.cm-ud-marker[data-edb="b2"] .bhandle`, b.rect(`.cm-ud-marker[data-edb="b3"]`).Top+3, func() {
		if got := b.eval(`document.querySelector('.cm-editor .cm-ud-gap')?.nextElementSibling?.dataset.edb`); got != "b3" {
			t.Errorf("in the editor the finger opens the gap before Three: %q", got)
		}
	})
	if !b.edWait(`t === '<!-- block b2 -->\nTwo\n\n<!-- block b3 -->\nThree\n\n<!-- block b1 -->\nOne'`) {
		t.Errorf("in the editor the finger moved it: %q", b.eval(edText))
	}
	if !b.waitFor(`document.querySelector('[data-edstatus]')?.textContent === 'Saved'`) {
		t.Errorf("saved")
	}
	if got := b.stored(page); got != "b2:Two | b3:Three | b1:One" {
		t.Errorf("the server: %q", got)
	}
}

// In the editor the handle drags the block's text with its marker: the gap is a widget of the editor, the drop moves the
// text, and saving moves the block, with the same ID and text; the cursor stays where it was in its block.
func TestDraggingABlockInTheEditor(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := makePage(b, "Drag edit", "One", "Two", "Three")
	versions := b.versions(page)
	b.openEditing(base, page, "Drag edit")
	b.lineWith("Two")
	scroll := `document.querySelector('[data-win="0"] .scroll').scrollTop`
	scrolled := b.eval(scroll)
	b.eval(`window.writes = []`)
	// Escape gives up.
	bottom := b.rect(`.cm-ud-marker[data-edb="b3"]`).Top + 30
	b.hold(`.cm-ud-marker[data-edb="b1"] .bhandle`, bottom)
	if got := b.eval(`(() => { const g = document.querySelector('.cm-editor .cm-ud-gap.dropgap.held'); return !!g && (g.nextElementSibling?.dataset.edb || 'end') + ' ' + g.textContent.trim() + ' ' + !document.querySelector('.cm-ud-marker[data-edb="b1"]') + ' ' + [...document.querySelectorAll('.cm-content > .cm-line')].filter(l => /One|block b1/.test(l.textContent)).length; })()`); got != "end One true 0" {
		t.Errorf("held at the end, the gap holds One and One is lifted: %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
	if !b.waitFor(`!document.querySelector('.cm-ud-gap') && !!document.querySelector('.cm-ud-marker[data-edb="b1"]')`) {
		t.Errorf("Escape puts it back")
	}
	b.release(`.cm-ud-marker[data-edb="b1"] .bhandle`, bottom)
	if !b.edWait(`t === '<!-- block b1 -->\nOne\n\n<!-- block b2 -->\nTwo\n\n<!-- block b3 -->\nThree'`) {
		t.Errorf("after Escape the text is as it was: %q", b.eval(edText))
	}
	// Three above One.
	top := b.rect(`.cm-ud-marker[data-edb="b1"]`).Top + 3
	b.hold(`.cm-ud-marker[data-edb="b3"] .bhandle`, top)
	if got := b.eval(`(() => { const g = document.querySelector('.cm-editor .cm-ud-gap'); return !!g && (g.nextElementSibling?.dataset.edb || 'end') + ' ' + g.textContent.trim(); })()`); got != "b1 Three" {
		t.Errorf("the gap opens before One: %q", got)
	}
	b.shot("block-drag-editor")
	b.release(`.cm-ud-marker[data-edb="b1"] .bhandle`, top)
	if !b.edWait(`t === '<!-- block b3 -->\nThree\n\n<!-- block b1 -->\nOne\n\n<!-- block b2 -->\nTwo'`) {
		t.Errorf("the text moved with its marker: %q", b.eval(edText))
	}
	if got := b.eval(`(() => { const v = EDS[at(0).d].ed.view; return v.state.doc.sliceString(0, v.state.selection.main.head).endsWith('<!-- block b2 -->\nTwo') + ' ' + !document.querySelector('.bmenu'); })()`); got != "true true" {
		t.Errorf("the cursor stays after Two, and no menu opens: %q", got)
	}
	if got := b.eval(scroll); got != scrolled {
		t.Errorf("the view stays: %s, before %s", got, scrolled)
	}
	if !b.waitFor(`document.querySelector('[data-edstatus]')?.textContent === 'Saved'`) {
		t.Errorf("the move is saved")
	}
	if got := b.eval(`window.writes.filter(w => !w.startsWith('GET')).join(', ')`); got != "POST /api/pages/"+page+"/blocks/b3/move" {
		t.Errorf("one move, no new block, no text: %q", got)
	}
	if got := b.stored(page); got != "b3:Three | b1:One | b2:Two" {
		t.Errorf("the server moved it: %q", got)
	}
	if got := b.versions(page); got != versions {
		t.Errorf("no new version of the texts: %d, before %d", got, versions)
	}
	b.clickOn(`.cm-ud-marker[data-edb="b2"] .bhandle`, 1)
	if !b.waitFor(`!!document.querySelector('.cm-ud-marker[data-edb="b2"] .bmenu')`) {
		t.Errorf("a click opens the menu")
	}
	b.open(base + "/o/" + page)
	b.ready("Drag edit")
	if got := b.eval(readOrder); got != "b3 b1 b2" {
		t.Errorf("after a reload: %q", got)
	}
	b.clean("drag in the editor")
}

// The bar of the editor is a row of the window, between the head and the content, outside what scrolls: it stays while
// the page scrolls, is flat, and shows only while the page is written.
func TestTheBarIsARowOfTheWindow(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	blocks := make([]string, 40)
	for k := range blocks {
		blocks[k] = "Paragraph " + strconv.Itoa(k+1) + " with some words in it."
	}
	page := makePage(b, "Long", blocks...)
	b.open(base + "/o/" + page)
	b.ready("Long")
	if got := b.eval(`document.querySelectorAll('.edbar').length`); got != "0" {
		t.Errorf("no bar while reading: %s", got)
	}
	b.openEditing(base, page, "Long")
	place := `(() => { const w = document.querySelector('[data-win="0"]'), bar = w.querySelector(':scope > .edbar'), tools = w.querySelector(':scope > .tools'), sc = w.querySelector('.scroll');
		if (!bar) return 'no bar'; const r = bar.getBoundingClientRect(), t = tools.getBoundingClientRect(), s = sc.getBoundingClientRect(), wr = w.getBoundingClientRect(), cs = getComputedStyle(bar);
		return [r.top >= t.bottom - 1, r.bottom <= s.top + 1, Math.abs(r.width - wr.width) < 2, !sc.contains(bar), cs.boxShadow, cs.borderTopLeftRadius, cs.borderBottomStyle, Math.round(r.top)].join(' '); })()`
	before := b.eval(place)
	if !strings.HasPrefix(before, "true true true true none 0px solid ") {
		t.Errorf("between the head and the content, full width, flat, with a line below: %q", before)
	}
	b.eval(`document.querySelector('[data-win="0"] .scroll').scrollTop = 100000`)
	time.Sleep(150 * time.Millisecond)
	if got := b.eval(`document.querySelector('[data-win="0"] .scroll').scrollTop > 200`); got != "true" {
		t.Fatalf("the page did not scroll")
	}
	if got := b.eval(place); got != before {
		t.Errorf("the bar stays in place while the page scrolls: %q, before %q", got, before)
	}
	b.lineWith("Paragraph 40 with some words in it.")
	b.press("ArrowLeft", "ArrowLeft", 37, 8)
	for range 5 {
		b.press("ArrowLeft", "ArrowLeft", 37, 8)
	}
	b.clickOn(`[data-win="0"] > .edbar [data-act="edfmt"][data-k="bold"]`, 1)
	if !b.edWait(`t.endsWith('Paragraph 40 with some words **in it.**')`) {
		t.Errorf("the bar works on the selection: %q", b.eval(edText)[len(b.eval(edText))-60:])
	}
	if got := b.eval(`document.activeElement?.classList.contains('cm-content')`); got != "true" {
		t.Errorf("the editor keeps the focus")
	}
	b.shot("editor-bar-row")
	b.clickOn(`[data-win="0"] .dstatus`, 1)
	if !b.waitFor(`!document.querySelector('.edbar') && !document.querySelector('.cm-editor')`) {
		t.Errorf("back to reading, the bar goes")
	}
}
