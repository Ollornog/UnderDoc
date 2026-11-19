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
	// Away from the cursor, as every line looks once the editor has no focus: the focus comes when the editor has loaded,
	// and the line with the cursor shows its Markdown (on a busy machine the focus came before the check, 2026-11-17).
	b.eval(`document.activeElement?.blur()`)
	shown := `[...document.querySelectorAll('.cm-editor .cm-line')].filter(l => l.textContent.trim()).map(l => l.textContent.trim() + (l.classList.contains('cm-lp-quote') ? '(q)' : '')).join(' / ')`
	if !b.waitFor(shown + ` === "a(q) / b(q) / one / two / • item / more"`) {
		t.Errorf("the editor shows the lines as lines: %q", b.eval(shown))
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

// realPage makes a page as an imported file gives one: sections of a heading with paragraphs and a list, each a block of
// several hundred pixels, between short paragraphs, a table and code. It returns the page.
func realPage(b *browser, title string) string {
	b.t.Helper()
	words := "The first build makes pages in the web, and every block keeps its ID, so it can be embedded elsewhere and two writers do not collide on the whole file. "
	var blocks []string
	for k := 1; k <= 8; k++ {
		n := strconv.Itoa(k)
		blocks = append(blocks,
			"## Section "+n+"\n\n"+strings.Repeat(words, 3)+"\n\n"+strings.Repeat(words, 2)+"\n\n- **Point one of "+n+":** "+words+"\n- **Point two:** "+words+"\n- Point three.",
			"A short paragraph "+n+".")
	}
	blocks = append(blocks, "| Port | VLAN | Room |\n|---|---:|---|\n| 1 | 10 | Tower |\n| 2 | 20 | Hall |\n| 3 | 30 | Roof |", "```text\nrouter 192.0.2.1\nswitch 192.0.2.2\n```")
	return makePage(b, title, blocks...)
}

// storedOrder is the order of the blocks on the server, each ID followed by a space.
func storedOrder(b *browser, page string) string {
	b.t.Helper()
	b.eval(`window.got = null; fetch('/api/doc/` + page + `').then(r => r.json()).then(d => { window.got = d.blocks.map(x => x.id + ' ').join(''); })`)
	if !b.waitFor(`window.got !== null`) {
		b.t.Fatal("the page could not be read")
	}
	return b.eval(`window.got`)
}

// heldAt sends a mouse event with the left button held, as a real mouse does while dragging.
func (b *browser) heldAt(kind string, x, y float64) {
	b.t.Helper()
	b.call("Input.dispatchMouseEvent", map[string]any{"type": kind, "x": x, "y": y, "button": "left", "buttons": 1, "clickCount": 1}, nil)
}

// grab moves a real mouse onto a handle's icon, presses there with the button held, and moves in steps to y, the button
// held all the way, as a hand on a mouse does; then it waits there.
func (b *browser) grab(handle string, y float64, wait time.Duration) {
	b.t.Helper()
	a := b.rect(handle + " svg")
	at := b.heldAt
	b.mouse("mouseMoved", a.X+120, a.Y+4, 0, 0)
	b.mouse("mouseMoved", a.X+1, a.Y+1, 0, 0)
	at("mousePressed", a.X, a.Y)
	// A hand starts slowly: the first moves are a few pixels, below the threshold of the module, above the browser's.
	for _, d := range []float64{1, 2, 4} {
		at("mouseMoved", a.X+d/2, a.Y+d)
		time.Sleep(8 * time.Millisecond)
	}
	for step := 1; step <= 24; step++ {
		at("mouseMoved", a.X+float64(step%3), a.Y+(y-a.Y)*float64(step)/24)
		time.Sleep(8 * time.Millisecond)
	}
	time.Sleep(wait)
}

// The maintainer's situation (2026-11-10): a long page whose blocks are whole sections, scrolled, text selected, a real
// mouse. A held block makes a short gap, the page scrolls by itself at the bottom edge, so a block goes down past blocks
// that were not in view, and a selection over the handle does not take the pointer away. In reading and in the editor.
func TestDraggingLongBlocksOfARealPage(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := realPage(b, "Long blocks")
	b.open(base + "/o/" + page)
	b.ready("Long blocks")
	b.watch()
	sc := `document.querySelector('[data-win="0"] .scroll')`
	b.eval(sc + `.scrollTop = 120`)
	time.Sleep(100 * time.Millisecond)
	if got := b.eval(`Math.round(document.querySelector('article.doc [data-b="b3"]').getBoundingClientRect().height) > 300`); got != "true" {
		t.Fatalf("the sections are long blocks")
	}
	// Text selected over the handles, as after marking a passage.
	b.eval(`(() => { const r = document.createRange(); r.selectNodeContents(document.querySelector('article.doc')); getSelection().removeAllRanges(); getSelection().addRange(r); })()`)
	bottom := b.rect(`[data-win="0"] .scroll`).Bottom - 6
	index := `(sel, gap) => [...document.querySelectorAll(sel)].filter(n => n.matches(gap) || !n.matches('[style*="none"]')).findIndex(n => n.matches(gap))`
	// A short way down: once lifted, the next block, a short paragraph, stands right below the short gap, and the pointer
	// on its lower half puts the long block after it.
	b.grab(`article.doc [data-b="b3"] > .bhandle`, b.rect(`article.doc [data-b="b3"] > .bhandle`).Y+12, 50*time.Millisecond)
	next := b.rect(`article.doc [data-b="b4"]`)
	b.heldAt("mouseMoved", next.X, next.Bottom-3)
	time.Sleep(50 * time.Millisecond)
	if got := b.eval(`[...document.querySelectorAll('article.doc > [data-b], article.doc > .dropgap')].filter(n => !n.matches('[style*="none"]')).slice(0, 4).map(n => n.dataset.b || 'gap').join(' ')`); got != "b1 b2 b4 gap" {
		t.Errorf("a short way down over the next block puts it after that block: %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
	b.mouse("mouseReleased", next.X, next.Bottom-3, 1, 0)
	time.Sleep(100 * time.Millisecond)
	// A busy browser draws fewer frames, as the CI does under load: the page scrolls by the time held, not by the frames
	// drawn, so it gets as far (2026-11-19).
	b.eval(`window.busy = true; (function frame() { if (!window.busy) return; const t = performance.now(); while (performance.now() - t < 60); requestAnimationFrame(frame); })()`)
	b.grab(`article.doc [data-b="b3"] > .bhandle`, bottom, 1500*time.Millisecond)
	b.eval(`window.busy = false`)
	got := b.eval(`(() => { const g = document.querySelector('article.doc .dropgap.held'); if (!g) return 'no gap'; return [Math.round(g.getBoundingClientRect().height) <= 130 && getComputedStyle(g).overflowY === 'hidden', (` + index + `)('article.doc > [data-b], article.doc > .dropgap', '.dropgap') >= 6, ` + sc + `.scrollTop > 400].join(' '); })()`)
	if got != "true true true" {
		t.Errorf("held at the bottom edge: a short gap, the page scrolled, and the gap went down past the blocks out of view: %q", got)
	}
	b.shot("block-drag-long-reading")
	b.mouse("mouseReleased", b.rect(`[data-win="0"] .scroll`).X, bottom, 1, 0)
	if !b.waitFor(`!document.querySelector('.dropgap') && ` + readOrder + `.split(' ').indexOf('b3') >= 6`) {
		t.Errorf("the block went down: %q", b.eval(readOrder))
	}
	time.Sleep(300 * time.Millisecond)
	if got := storedOrder(b, page); strings.Index(got, "b3 ") < strings.Index(got, "b8 ") {
		t.Errorf("the server moved it: %q", got)
	}
	b.clean("long blocks in reading")

	// In the editor.
	b.openEditing(base, page, "Long blocks")
	b.eval(`document.querySelector('.cm-ud-marker[data-edb="b5"]').scrollIntoView({block: 'start'}); ` + sc + `.scrollTop -= 120`)
	time.Sleep(150 * time.Millisecond)
	order := `[...` + edText + `.matchAll(/block (b\d+)/g)].map(m => m[1])`
	// A short way down, over the short paragraph after it.
	b.grab(`.cm-ud-marker[data-edb="b5"] .bhandle`, b.rect(`.cm-ud-marker[data-edb="b5"] .bhandle`).Y+12, 50*time.Millisecond)
	edGaps := `[...document.querySelectorAll('.cm-content > .cm-ud-marker, .cm-content > .cm-ud-gap')].map(n => n.dataset.edb || 'gap').join(' ')`
	// Up over the block before it, a short paragraph: on its lower half the gap stays, on its upper half it goes before it.
	above, gapTop := b.rect(`.cm-ud-marker[data-edb="b4"]`), b.rect(`.cm-ud-gap`).Top
	for _, c := range []struct {
		at   float64
		want string
	}{{0.75, "b4 gap b6"}, {0.25, "gap b4 b6"}} {
		b.heldAt("mouseMoved", above.X, above.Top+(gapTop-above.Top)*c.at)
		time.Sleep(80 * time.Millisecond)
		if got := b.eval(edGaps); !strings.Contains(got, c.want) {
			t.Errorf("in the editor, at %.2f of the block before: %q, want %q", c.at, got, c.want)
		}
	}
	next = b.rect(`.cm-ud-marker[data-edb="b7"]`)
	b.heldAt("mouseMoved", next.X, next.Top-3)
	time.Sleep(100 * time.Millisecond)
	if got := b.eval(edGaps); !strings.Contains(got, "b6 gap b7") {
		t.Errorf("in the editor a short way down over the next block puts it after that block: %q", got)
	}
	// Back on the lower half of that block, now before the gap, it stays.
	six := b.rect(`.cm-ud-marker[data-edb="b6"]`)
	b.heldAt("mouseMoved", six.X, six.Top+(b.rect(`.cm-ud-gap`).Top-six.Top)*0.75)
	time.Sleep(80 * time.Millisecond)
	if got := b.eval(edGaps); !strings.Contains(got, "b6 gap b7") {
		t.Errorf("in the editor, on the lower half of the block before the gap, it stays: %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
	b.mouse("mouseReleased", next.X, next.Top-3, 1, 0)
	time.Sleep(100 * time.Millisecond)
	b.grab(`.cm-ud-marker[data-edb="b5"] .bhandle`, bottom, 1500*time.Millisecond)
	got = b.eval(`(() => { const g = document.querySelector('.cm-editor .cm-ud-gap'); if (!g) return 'no gap'; const before = g.nextElementSibling?.dataset.edb; return [Math.round(g.getBoundingClientRect().height) <= 130 && getComputedStyle(g).overflowY === 'hidden', !!before && ` + order + `.indexOf(before) >= 6 || !before, ` + sc + `.scrollTop > 400].join(' '); })()`)
	if got != "true true true" {
		t.Errorf("in the editor, held at the bottom edge: a short gap, scrolled, gone down: %q", got)
	}
	b.shot("block-drag-long-editor")
	b.mouse("mouseReleased", b.rect(`[data-win="0"] .scroll`).X, bottom, 1, 0)
	if !b.waitFor(order + `.indexOf('b5') >= 8`) {
		t.Errorf("in the editor the block went down: %q", b.eval(order+`.join(' ')`))
	}
	if !b.waitFor(`document.querySelector('[data-edstatus]')?.textContent === 'Saved'`) {
		t.Errorf("saved")
	}
	if got := storedOrder(b, page); strings.Index(got, "b5 ") < strings.Index(got, "b10 ") {
		t.Errorf("the server moved it: %q", got)
	}
	b.clean("long blocks in the editor")
}

// The menu of a handle is as compact as the right-click menus, in reading and in the editor (where CodeMirror's text
// keeps white space): their width, item height and font, smaller icons; near the bottom of the window it opens above the
// handle, inside the window.
func TestTheMenuOfAHandleIsCompact(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := realPage(b, "Menu")
	b.open(base + "/o/" + page)
	b.ready("Menu")
	// The right-click menu of the workspace, for comparison.
	b.rightClickOn(`.nav3 [data-row]`)
	if !b.waitFor(`!!document.querySelector('.ctxpop > button')`) {
		t.Fatal("no right-click menu")
	}
	ctx := b.eval(`(() => { const x = document.querySelector('.ctxpop > button'), cs = getComputedStyle(x); return Math.round(x.getBoundingClientRect().height) + ' ' + cs.fontSize + ' ' + cs.borderTopLeftRadius; })()`)
	// frame is how a menu is framed: padding, border, corners, background, shadow.
	frame := `(m => { const cs = getComputedStyle(m); return [cs.padding, cs.borderTopWidth, cs.borderTopColor, cs.borderTopLeftRadius, cs.backgroundColor, cs.boxShadow].join(' / '); })`
	ctxFrame := b.eval(frame + `(document.querySelector('.ctxpop'))`)
	b.press("Escape", "Escape", 27, 0)
	b.waitFor(`!document.querySelector('.ctxpop')`)
	look := `(() => { const m = document.querySelector('.bmenu'); if (!m) return 'no menu'; const r = m.getBoundingClientRect(), items = [...m.querySelectorAll('[role=menuitem]')], cs = getComputedStyle(items[0]);
		return [r.width >= 180 && r.width <= 200 && ` + frame + `(m) === ` + strconv.Quote(ctxFrame) + `, items.every(x => Math.round(x.getBoundingClientRect().height) + ' ' + getComputedStyle(x).fontSize + ' ' + getComputedStyle(x).borderTopLeftRadius === ` + strconv.Quote(ctx) + `),
			items.every(x => x.querySelector('svg').getBoundingClientRect().width <= 14), r.height <= items.length * 30 + 50 + (m.querySelector('.seg')?.getBoundingClientRect().height || 0), r.top >= 0 && r.bottom <= innerHeight && r.right <= innerWidth].join(' '); })()`
	// open opens the menu of block id, its handle at the top of the window or near the bottom.
	open := func(handle string, bottom bool) {
		b.eval(`(() => { const s = document.querySelector('[data-win="0"] .scroll'), h = document.querySelector(` + strconv.Quote(handle) + `), r = s.getBoundingClientRect();
			s.scrollTop += h.getBoundingClientRect().top - (` + strconv.FormatBool(bottom) + ` ? r.bottom - 50 : r.top + 40); })()`)
		time.Sleep(100 * time.Millisecond)
		b.clickOn(handle, 1)
		if !b.waitFor(`!!document.querySelector('.bmenu')`) {
			t.Fatalf("%s: no menu", handle)
		}
	}
	open(`article.doc [data-b="b4"] > .bhandle`, false)
	if got := b.eval(look); got != "true true true true true" {
		t.Errorf("reading: as compact as the right-click menu (%s): %q", ctx, got)
	}
	b.clickOn(`.dtitle h1`, 1)
	open(`article.doc [data-b="b6"] > .bhandle`, true)
	if got := b.eval(look); got != "true true true true true" {
		t.Errorf("reading, near the bottom: inside the window: %q", got)
	}
	b.shot("block-menu-reading")
	b.clickOn(`.dtitle h1`, 1)
	b.clickOn(`[data-win="0"] .dstatus`, 1)
	if !b.waitFor(`!!document.querySelector('.cm-ud-marker[data-edb="b4"] .bhandle')`) {
		t.Fatal("no editor")
	}
	open(`.cm-ud-marker[data-edb="b4"] .bhandle`, false)
	if got := b.eval(look); got != "true true true true true" {
		t.Errorf("editor: as compact as the right-click menu (%s): %q", ctx, got)
	}
	b.shot("block-menu-editor")
	b.clickOn(`.dtitle h1`, 1)
	open(`.cm-ud-marker[data-edb="b6"] .bhandle`, true)
	if got := b.eval(look); got != "true true true true true" {
		t.Errorf("editor, near the bottom: inside the window: %q", got)
	}
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
