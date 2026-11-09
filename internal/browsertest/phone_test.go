//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// phone makes the page a phone of this size: a narrow viewport, touch instead of a mouse, and no hover.
func (b *browser) phone(width, height int) {
	b.t.Helper()
	b.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": width, "height": height, "deviceScaleFactor": 1, "mobile": true}, nil)
	b.call("Emulation.setTouchEmulationEnabled", map[string]any{"enabled": true, "maxTouchPoints": 5}, nil)
	b.call("Emulation.setEmulatedMedia", map[string]any{"features": []map[string]string{{"name": "hover", "value": "none"}, {"name": "pointer", "value": "coarse"}}}, nil)
}

// tap touches the middle of an element with a finger, briefly.
func (b *browser) tap(selector string) {
	b.t.Helper()
	r := b.rect(selector)
	b.pressAt(r.X, r.Y, 60*time.Millisecond)
}

// pressAt holds a finger still on a point for a while and lifts it.
func (b *browser) pressAt(x, y float64, hold time.Duration) {
	b.t.Helper()
	b.call("Input.dispatchTouchEvent", map[string]any{"type": "touchStart", "touchPoints": []map[string]any{{"x": x, "y": y}}}, nil)
	time.Sleep(hold)
	b.call("Input.dispatchTouchEvent", map[string]any{"type": "touchEnd", "touchPoints": []map[string]any{}}, nil)
}

// longPress holds a finger on the middle of an element for most of a second.
func (b *browser) longPress(selector string) {
	b.t.Helper()
	r := b.rect(selector)
	b.pressAt(r.X, r.Y, 800*time.Millisecond)
}

// longPressFreeNav2 holds a finger on the free part of nav2, below its entries.
func (b *browser) longPressFreeNav2() {
	b.t.Helper()
	r := b.rect(".nav2")
	b.pressAt(r.X, r.Bottom-6, 800*time.Millisecond)
}

// unreachable lists everything of the workspace a hand cannot get to on this screen: a control outside the window and
// not in a list that scrolls to it, cut off by something that does not scroll, covered by something that is not a menu
// or drawer opened over it, or not to be seen at all (shown only to a pointer that hovers); text cut off without the
// dots that say there is more; a column, window, drawer or menu that scrolls sideways; and the page itself scrolling
// sideways. Lists that scroll are scrolled to each control in turn, as a finger would.
const unreachable = `(() => {
	const W = innerWidth, H = innerHeight, bad = [];
	if (document.documentElement.scrollWidth > W || document.body.scrollWidth > W) bad.push('the page scrolls sideways: ' + document.documentElement.scrollWidth + ' > ' + W);
	const name = e => (e.dataset.act ? e.dataset.act + (e.dataset.id ? ':' + e.dataset.id : '') + ' ' : '') + JSON.stringify((e.getAttribute('aria-label') || e.title || e.textContent || e.placeholder || '').trim().replace(/\s+/g, ' ').slice(0, 30)) + ' ' + e.tagName.toLowerCase() + (typeof e.className === 'string' && e.className.trim() ? '.' + e.className.trim().split(/\s+/).join('.') : '');
	const scrolls = s => /(auto|scroll)/.test(s);
	// Columns, windows, drawers and menus do not scroll sideways: what is wider than they are is cut off for a hand.
	for (const p of document.querySelectorAll('#root :is(.strip, .nav2, .nav3, .tree, .scroll, .page, .fdrawer, .spanel, .pop, .ctxpop, .bmenu, .bslash, .work, .side)')) {
		if (p.getClientRects().length && getComputedStyle(p).overflowX !== 'visible' && p.scrollWidth > p.clientWidth + 1) bad.push('scrolls sideways (' + p.scrollWidth + ' > ' + p.clientWidth + '): ' + name(p));
	}
	// Text cut by an edge that does not scroll, without the dots that say there is more.
	const walk = document.createTreeWalker(document.getElementById('root'), NodeFilter.SHOW_TEXT);
	for (let n = walk.nextNode(); n; n = walk.nextNode()) {
		const el = n.parentElement;
		if (!n.textContent.trim() || !el.getClientRects().length || getComputedStyle(el).visibility === 'hidden') continue;
		const range = document.createRange(); range.selectNodeContents(n);
		const t = range.getBoundingClientRect();
		if (t.width < 1) continue;
		for (let p = el; p && p !== document.documentElement; p = p.parentElement) { const s = getComputedStyle(p);
			if (s.textOverflow === 'ellipsis' || scrolls(s.overflowX)) break;
			if (s.overflowX === 'visible') continue;
			const q = p.getBoundingClientRect();
			// Two pixels: the letter spacing after the last letter of a label in capitals is no ink.
			if (t.left < q.left - 2 || t.right > q.right + 2) bad.push('text cut off by ' + name(p) + ' (' + Math.round(t.left) + '-' + Math.round(t.right) + ' in ' + Math.round(q.left) + '-' + Math.round(q.right) + '): ' + JSON.stringify(n.textContent.trim().slice(0, 30)));
			break; }
	}
	const layer = '.fly, .pop, .ctxpop, .cmdpop, .fdrawer, .bmenu, .bslash, .btool, .aidrawer, .toast, .rail';
	for (const e of document.querySelectorAll('#root :is(button, input, textarea, select, a[href], [role=menuitem]):not([data-grip])')) {
		if (!e.getClientRects().length || getComputedStyle(e).visibility === 'hidden') continue;
		let r = e.getBoundingClientRect();
		if (r.width < 1 || r.height < 1) continue;
		let opacity = 1;
		for (let p = e; p; p = p.parentElement) opacity *= +getComputedStyle(p).opacity;
		if (opacity < 0.2) { bad.push('not to be seen: ' + name(e)); continue; }
		// A finger scrolls what scrolls, nothing else.
		const fixed = [];
		for (let p = e.parentElement; p; p = p.parentElement) { const s = getComputedStyle(p); if (!scrolls(s.overflowX) || !scrolls(s.overflowY)) fixed.push([p, p.scrollLeft, p.scrollTop, scrolls(s.overflowX), scrolls(s.overflowY)]); }
		e.scrollIntoView({block: 'nearest', inline: 'nearest'});
		let hidden = false;
		for (const [p, x, y, sx, sy] of fixed) { if ((!sx && p.scrollLeft !== x) || (!sy && p.scrollTop !== y)) hidden = true; if (!sx) p.scrollLeft = x; if (!sy) p.scrollTop = y; }
		r = e.getBoundingClientRect();
		if (hidden) { bad.push('only behind something that does not scroll: ' + name(e)); continue; }
		if (r.left < -1 || r.right > W + 1 || r.top < -1 || r.bottom > H + 1) { bad.push('outside the screen (' + Math.round(r.left) + ',' + Math.round(r.top) + ' ' + Math.round(r.width) + 'x' + Math.round(r.height) + '): ' + name(e)); continue; }
		let cut = false;
		for (let p = e.parentElement; p && p !== document.documentElement; p = p.parentElement) { const s = getComputedStyle(p); if (s.overflowX === 'visible' && s.overflowY === 'visible') continue;
			const q = p.getBoundingClientRect(); if (r.left < q.left - 1 || r.right > q.right + 1 || r.top < q.top - 1 || r.bottom > q.bottom + 1) { cut = true; bad.push('cut off by ' + name(p) + ': ' + name(e)); break; } }
		if (cut) continue;
		const top = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
		if (!top || top === e || e.contains(top) || top.contains(e)) continue;
		// A menu, a drawer, or nav2 and nav3 opened over the window cover what lies below them on purpose.
		const over = top.closest(layer);
		if (!over || over.contains(e)) bad.push('covered by ' + name(top) + ': ' + name(e));
	}
	return JSON.stringify(bad);
})()`

// reach fails the test for everything of this state a hand cannot get to, and keeps a picture of it.
func (b *browser) reach(state string) {
	b.t.Helper()
	time.Sleep(250 * time.Millisecond) // the bar of a block fades in
	var bad []string
	if err := json.Unmarshal([]byte(b.eval(unreachable)), &bad); err != nil {
		b.t.Fatal(err)
	}
	if len(bad) > 0 {
		b.t.Errorf("%s: %d things out of reach:\n\t%s", state, len(bad), strings.Join(bad, "\n\t"))
	}
	b.shot(fmt.Sprintf("phone-%d-%s", int(b.rect("html").Width), state))
}

// rackPage makes a collection with a page of two blocks through the API, as a setup, and returns the ID of the page.
func rackPage(b *browser) string {
	b.t.Helper()
	b.eval(`window.made = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		const team = await post('/api/collections', {title: 'Team'});
		const page = await post('/api/pages', {title: 'Rack', parent: team.id});
		const b1 = await post('/api/pages/' + page.id + '/blocks', {md: '## Switches', after: ''});
		await post('/api/pages/' + page.id + '/blocks', {md: 'Two in the rack, with a line of text long enough to wrap on a phone.', after: b1.id});
		window.made = page.id;
	})()`)
	if !b.waitFor(`!!window.made`) {
		b.t.Fatal("the page for the test was not made")
	}
	return b.eval(`window.made`)
}

// At phone width nothing is cut off and everything can be reached, even before the phone has a layout of its own (M-28,
// T-10): the page never scrolls sideways, and every control of the workspace in its main states lies on the screen or in
// a list that scrolls to it, can be seen without hovering, and is not covered, at the width of a common phone and of a
// small one.
func TestNothingIsCutOffOnAPhone(t *testing.T) {
	for _, size := range [][2]int{{390, 844}, {360, 740}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			b, base := setupWithDatabase(t)
			b.phone(size[0], size[1])
			b.open(base + "/o/plan")
			b.ready("Network plan")
			page := rackPage(b)
			b.open(base + "/o/plan")
			b.ready("Network plan")
			b.reach("plan")
			b.clickOn(`.brandrow .winbtn`, 1)
			b.reach("window-menu")
			b.clickOn(probe(b, `.ctxpop button`, "Keep as a tab of its own"), 1)
			b.reach("tabs")
			for _, act := range []string{"tablist", "menu", "fly", "find", "aid"} {
				open := `[data-win="0"] [data-act="` + act + `"]`
				b.clickOn(open, 1)
				b.reach(act)
				switch act {
				case "fly", "find":
					b.clickOn(open, 1)
				case "aid":
					b.clickOn(`.aidrawer [data-act="aid"]`, 1)
				}
				b.press("Escape", "Escape", 27, 0)
			}
			b.longPress(`.dtab`)
			b.reach("tab-menu")
			b.press("Escape", "Escape", 27, 0)
			b.clickOn(`[data-win="0"] [data-act="split"]`, 1)
			b.reach("two-windows")
			b.clickOn(`.win.focus [data-act="closewin"]`, 1)
			if got := b.eval(`document.querySelectorAll('.win').length`); got != "1" {
				t.Fatalf("the second window closes: %s windows", got)
			}

			b.clickOn(`.strip [data-id="workspace"]`, 1)
			b.reach("nav2-nav3")
			b.clickOn(`.newbtn[data-act="newmenu"]`, 1)
			b.reach("new")
			b.press("Escape", "Escape", 27, 0)
			b.clickOn(`.nav2 .newplus`, 1)
			b.reach("new-in-nav2")
			b.press("Escape", "Escape", 27, 0)
			b.clickOn(probe(b, `.nav2 .navitem`, "Team"), 1)
			b.clickOn(`.nav3 .cheadrow .newplus:not(:disabled)`, 1)
			b.reach("new-in-a-collection")
			b.press("Escape", "Escape", 27, 0)
			b.longPress(`.nav3 .row [data-act="open"]`)
			b.reach("entry-menu")
			b.press("Escape", "Escape", 27, 0)
			b.longPressFreeNav2()
			b.reach("nav2-menu")
			b.clickOn(probe(b, `.ctxpop button`, "Problems"), 1)
			b.ready("Problems")
			b.reach("problems")
			b.clickOn(`.strip [data-id="workspace"]`, 1)
			b.longPressFreeNav2()
			b.clickOn(probe(b, `.ctxpop button`, "Trash"), 1)
			b.ready("Trash")
			b.reach("trash")
			b.clickOn(`.strip [data-id="search"]`, 1)
			b.reach("search")
			b.clickOn(`.strip [data-id="profile"]`, 1)
			b.reach("profile")

			b.open(base + "/o/" + page)
			b.ready("Rack")
			b.reach("page")
			b.clickOn(`[data-win="0"] .tools [data-act="cmt"]`, 1)
			b.reach("comments")
			b.clickOn(`[data-win="0"] .fdrawer [data-act="cmt"]`, 1)
			b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
			b.reach("gear")
			b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
			b.tap(`article.doc [data-b="b2"] > .bhandle`)
			b.reach("block-menu")
			b.clickOn(`.dtitle h1`, 1)
			b.clickOn(`[data-act="edit"]`, 1)
			b.waitFor(`!!document.querySelector('textarea[data-blk="b2"]')`)
			b.tap(`textarea[data-blk="b2"]`)
			b.reach("block-bar")
			b.tap(`.bplus[data-id="b2"]`)
			b.waitFor(`!!document.querySelector('.bslash')`)
			b.reach("kinds")
		})
	}
}

// No document of the made-up content, with its speech bubbles, tables and boards, makes its window scroll sideways on a
// phone: what is wider scrolls in its own frame, as a table does.
func TestNoDocumentScrollsSidewaysOnAPhone(t *testing.T) {
	for _, size := range [][2]int{{390, 844}, {360, 740}} {
		b, base := setup(t)
		b.phone(size[0], size[1])
		b.open(base + "/dev/ui")
		b.ready("")
		b.eval(`window.over = []; window.swept = 0; (async () => {
			for (const id of Object.keys(D).filter(k => !D[k].real && !k.startsWith('dir:'))) {
				openDoc(id); render(); await new Promise(r => setTimeout(r, 20)); window.swept++;
				for (const s of document.querySelectorAll('.win.focus :is(.scroll, .page)')) if (s.scrollWidth > s.clientWidth + 1) window.over.push(id + ' (' + s.scrollWidth + ' > ' + s.clientWidth + ')');
			}
			window.done = true; })()`)
		if !b.waitFor(`window.done === true`) {
			t.Fatal("the sweep did not end")
		}
		if got := b.eval(`window.swept > 40 ? window.over.join(', ') : 'only ' + window.swept + ' documents'`); got != "" {
			t.Errorf("%dx%d: windows scroll sideways: %s", size[0], size[1], got)
		}
	}
}

// inSight tells whether an element can be seen without hovering, lies inside the window of the page and is the one a
// finger on its middle would touch.
const inSight = `(sel => { const e = document.querySelector(sel), r = e.getBoundingClientRect(), w = document.querySelector('[data-win="0"] .scroll').getBoundingClientRect();
	let o = 1; for (let p = e; p; p = p.parentElement) o *= +getComputedStyle(p).opacity;
	return o >= 0.3 && r.left >= w.left && r.right <= w.right && document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2)?.closest(sel) === e; })`

// On a touch screen there is no hover: the handle of a block and its speech bubble are in sight, and a tap on the handle
// opens its menu; while editing, the plus beside each block is in sight, and a tap on it adds a block below with the
// list of kinds.
func TestTouchReachesTheHandleAndThePlus(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.phone(390, 844)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := rackPage(b)
	b.open(base + "/o/" + page)
	b.ready("Rack")
	if got := b.eval(inSight + `('article.doc [data-b="b2"] > .bhandle') + ' ' + ` + inSight + `('article.doc .cblk[data-id="b2"]')`); got != "true true" {
		t.Errorf("the handle and the speech bubble are in sight and inside the window without hovering: %q", got)
	}
	b.tap(`article.doc [data-b="b2"] > .bhandle`)
	if !b.waitFor(`!!document.querySelector('.bmenu [data-act="blkedit"]')`) {
		t.Fatal("a tap on the handle opens the menu of the block")
	}
	b.tap(`.bmenu [data-act="blkedit"]`)
	if !b.waitFor(`document.activeElement?.dataset?.blk === 'b2'`) {
		t.Fatalf("Edit this block opens editing at b2: %s", b.eval(`document.activeElement?.outerHTML.slice(0, 100)`))
	}
	if got := b.eval(inSight + `('.bplus[data-id="b2"]') + ' ' + ` + inSight + `('.bplus[data-id="b1"]')`); got != "true true" {
		t.Errorf("the plus of each block is in sight and inside the window: %q", got)
	}
	b.tap(`.bplus[data-id="b2"]`)
	if !b.waitFor(`document.activeElement?.dataset?.blk === 'b3' && !!document.querySelector('.bslash')`) {
		t.Fatalf("a tap on the plus adds a block below, ready to write, with the list of kinds: %s", b.eval(`document.activeElement?.outerHTML.slice(0, 100)`))
	}
	b.shot("phone-touch-plus")
}

// A long press is the right click (ADR-17): it opens the menu of an entry, and lifting the finger opens nothing; on the
// free part of nav2 it reaches the problems and the trash. A short tap stays a tap.
func TestLongPressIsTheRightClick(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.phone(390, 844)
	b.open(base + "/o/~ops/plain.md")
	b.ready("plain")
	b.clickOn(`.strip [data-id="workspace"]`, 1)
	b.longPress(probe(b, `.nav3 .name`, "Network plan"))
	if !b.waitFor(`document.querySelector('.ctxpop > b')?.textContent === 'Network plan'`) {
		t.Fatalf("a long press opens the menu of the entry: %s", b.eval(`document.querySelector('.ctxpop')?.innerText`))
	}
	time.Sleep(200 * time.Millisecond)
	if got := b.eval(`document.querySelector('.dtitle h1')?.textContent + ' ' + !!document.querySelector('.ctxpop')`); got != "plain true" {
		t.Errorf("lifting the finger opens nothing and keeps the menu: %q", got)
	}
	b.shot("phone-long-press")
	b.press("Escape", "Escape", 27, 0)
	b.longPressFreeNav2()
	if got := b.eval(`[...document.querySelectorAll('.ctxpop button')].map(n => n.textContent.trim()).join(', ')`); !strings.Contains(got, "Problems") || !strings.Contains(got, "Trash") {
		t.Errorf("the free part of nav2 offers the problems and the trash: %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
	b.tap(probe(b, `.nav3 .name`, "Network plan"))
	b.ready("Network plan")
}
