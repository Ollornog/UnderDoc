//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The web editor of M-8 (ADR-21): one CodeMirror editor per page, the blocks joined with hidden markers, saved block by
// block. Everything is typed with real keys; what the editor writes is read back from the server.

// makePage makes a collection and in it a page with these blocks, through the API, and returns the page's ID.
func makePage(b *browser, title string, blocks ...string) string {
	b.t.Helper()
	list, _ := jsonList(blocks)
	b.eval(`window.made = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		const team = await post('/api/collections', {title: 'Team ' + Math.random().toString(36).slice(2, 6)});
		const page = await post('/api/pages', {title: ` + strconv.Quote(title) + `, parent: team.id});
		let after = '';
		for (const md of ` + list + `) after = (await post('/api/pages/' + page.id + '/blocks', {md, after})).id;
		window.made = page.id;
	})()`)
	if !b.waitFor(`!!window.made`) {
		b.t.Fatal("the page for the test was not made")
	}
	return b.eval(`window.made`)
}

func jsonList(items []string) (string, error) {
	quoted := make([]string, len(items))
	for k, s := range items {
		quoted[k] = strconv.Quote(s)
	}
	return "[" + strings.Join(quoted, ",") + "]", nil
}

// watch records every request of the page that writes, except the settings of the account, and every CSP violation and
// exception, in the page itself.
func (b *browser) watch() {
	b.t.Helper()
	b.eval(`(() => {
		window.writes = []; window.written = []; window.problems = [];
		const original = window.fetch;
		// writes are the changes sent, written those the server has answered.
		window.fetch = (url, init) => { const m = (init && init.method) || 'GET', w = m !== 'GET' && !String(url).startsWith('/api/settings');
			if (w) window.writes.push(m + ' ' + url);
			const answer = original(url, init); if (w) answer.then(() => window.written.push(m + ' ' + url), () => {}); return answer; };
		document.addEventListener('securitypolicyviolation', e => window.problems.push('CSP ' + e.effectiveDirective + ' ' + e.blockedURI + ' ' + e.sourceFile + ':' + e.lineNumber));
		addEventListener('error', e => window.problems.push('error ' + e.message));
		addEventListener('unhandledrejection', e => window.problems.push('rejection ' + (e.reason && e.reason.message || e.reason)));
	})()`)
}

// clean fails the test for every CSP violation and exception seen in the page or reported by Chrome.
func (b *browser) clean(where string) {
	b.t.Helper()
	if got := b.eval(`(window.problems || []).join(' | ')`); got != "" {
		b.t.Errorf("%s: problems in the page: %s", where, got)
	}
	csp, errors := b.drain()
	for _, list := range [][]string{csp, errors} {
		for _, entry := range list {
			b.t.Errorf("%s: %s", where, entry)
		}
	}
}

// editorAt is true when the editor has the focus and the cursor stands at the start of the text of a block.
func editorAt(block string) string {
	return `(() => { const e = EDS[at(0)?.d]; if (!e || !document.activeElement?.classList.contains('cm-content')) return false;
		const s = e.ed.view.state; return s.doc.sliceString(0, s.selection.main.head).endsWith('<!-- block ` + block + ` -->\n'); })()`
}

const edText = `(() => { const k = at(0) && at(0).d; return EDS[k] ? EDS[k].ed.text() : 'no editor'; })()`

// edWait waits until the editor's text satisfies a condition on t.
func (b *browser) edWait(condition string) bool {
	b.t.Helper()
	return b.waitFor(`(() => { const t = ` + edText + `; return ` + condition + `; })()`)
}

// openEditing opens a page and turns it to editing with the status button.
func (b *browser) openEditing(base, page, title string) {
	b.t.Helper()
	b.open(base + "/o/" + page)
	b.ready(title)
	b.watch()
	b.clickOn(`[data-win="0"] .dstatus`, 1)
	if !b.waitFor(`!!document.querySelector('article.doc .cm-editor .cm-content') && document.activeElement?.classList.contains('cm-content')`) {
		b.t.Fatalf("the page did not open in the editor: %s", b.eval(`document.querySelector('article.doc')?.innerHTML.slice(0, 300)`))
	}
}

// lineWith clicks into the line of the editor that shows this text, and puts the cursor at its end. CodeMirror draws its
// lines itself, so the line is found and measured in one go, without marking it.
func (b *browser) lineWith(text string) {
	b.t.Helper()
	raw := b.eval(`JSON.stringify((() => { const l = [...document.querySelectorAll('.cm-editor .cm-line')].find(n => n.textContent.trim() === ` + strconv.Quote(text) + `);
		if (!l) return null; const r = l.getBoundingClientRect(); return {X: r.left + Math.min(r.width / 2, 20), Y: r.top + r.height / 2}; })())`)
	var r box
	if err := json.Unmarshal([]byte(raw), &r); err != nil || raw == "null" {
		b.t.Fatalf("no line %q in the editor: %s", text, b.eval(`[...document.querySelectorAll('.cm-editor .cm-line')].map(n => n.textContent).join(' / ')`))
	}
	b.mouse("mouseMoved", r.X, r.Y, 0, 0)
	b.mouse("mousePressed", r.X, r.Y, 1, 0)
	b.mouse("mouseReleased", r.X, r.Y, 1, 0)
	b.press("End", "End", 35, 0)
}

// versions counts the changes the server keeps for a page.
func (b *browser) versions(page string) int {
	b.t.Helper()
	b.eval(`window.nv = null; fetch('/api/pages/` + page + `/versions').then(r => r.json()).then(v => { window.nv = v.length; })`)
	if !b.waitFor(`window.nv !== null`) {
		b.t.Fatal("no versions")
	}
	n, _ := strconv.Atoi(b.eval(`window.nv`))
	return n
}

// stored reads the blocks of a page from the server, as "id:md" joined by " | ".
func (b *browser) stored(page string) string {
	b.t.Helper()
	b.eval(`window.got = null; fetch('/api/doc/` + page + `').then(r => r.json()).then(d => { window.got = d.blocks.map(x => x.id + ':' + x.md).join(' | '); })`)
	if !b.waitFor(`window.got !== null`) {
		b.t.Fatal("the page could not be read")
	}
	return b.eval(`window.got`)
}

// Opening a page in the editor and leaving it without typing changes nothing: no write, no new version.
func TestTheEditorRoundTripChangesNothing(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	tricky := []string{"## Switches\n\nTwo of them, in the tower.", "| Port | VLAN |\n|---|---:|\n| 1 | 10 |", "- [x] done\n- [ ] open",
		"```text\n<!-- block b1 -->\n\nrouter\n```", "::embed{ref=\"8p3n5v2w6h#b1\"}", "[[plan]] and [a link](/o/plan)", "", "\n\nBlank lines around\n\n"}
	page := makePage(b, "Round trip", tricky...)
	before, versions := b.stored(page), b.versions(page)
	b.openEditing(base, page, "Round trip")
	if got := b.eval(`document.querySelectorAll('.cm-editor .cm-ud-marker').length + ' ' + document.querySelectorAll('.cm-editor .cm-ud-marker .bhandle').length + ' ' + !document.querySelector('textarea')`); got != "8 8 true" {
		t.Errorf("one hidden marker with a handle per block, no field: %q", got)
	}
	if got := b.eval(`[...document.querySelectorAll('.cm-editor .cm-line')].some(l => /<!-- block b\d -->/.test(l.textContent) && !l.closest('.cm-lp-code'))`); got != "false" {
		t.Errorf("a marker shows as text")
	}
	b.shot("editor-round-trip")
	b.press("ArrowDown", "ArrowDown", 40, 0)
	b.press("ArrowDown", "ArrowDown", 40, 0)
	time.Sleep(1500 * time.Millisecond)
	b.clickOn(`[data-win="0"] .dstatus`, 1) // back to reading
	if !b.waitFor(`!document.querySelector('.cm-editor') && !!document.querySelector('article.doc [data-b="b8"]')`) {
		t.Fatalf("reading again: %s", b.eval(`document.querySelector('article.doc')?.innerHTML.slice(0, 200)`))
	}
	time.Sleep(500 * time.Millisecond)
	if got := b.eval(`window.writes.join(', ')`); got != "" {
		t.Errorf("opening and leaving wrote: %s", got)
	}
	if got := b.stored(page); got != before {
		t.Errorf("the page changed:\n%s\n%s", before, got)
	}
	if got := b.versions(page); got != versions {
		t.Errorf("versions %d, before %d", got, versions)
	}
	b.clean("round trip")
}

// Typed Markdown converts live, autosave keeps it, and a paragraph written after a block becomes a block with its ID and
// marker, without moving the cursor.
func TestTheEditorSavesAsYouType(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := makePage(b, "Typing", "First", "Second")
	b.openEditing(base, page, "Typing")
	b.lineWith("First")
	b.typeKeys("\n\n## Heading")
	if !b.waitFor(`!!document.querySelector('.cm-editor .cm-lp-h2')`) {
		t.Errorf("a heading typed shows as a heading")
	}
	if !b.edWait(`t.includes('<!-- block b3 -->\n## Heading')`) {
		t.Fatalf("the new paragraph did not become block b3: %q", b.eval(edText))
	}
	if got := b.eval(`(() => { const v = EDS[at(0).d].ed.view; return v.state.doc.sliceString(v.state.selection.main.head - 7, v.state.selection.main.head); })()`); got != "Heading" {
		t.Errorf("the cursor stays after what was typed: %q", got)
	}
	// Live shortcuts, each typed with real keys into a paragraph of its own at the end of the page. The blank line before
	// it is put there directly: Enter after a list item would continue the list, as it should.
	for _, c := range []struct{ typed, selector string }{
		{"**bold** and *it* and `code`", ".cm-lp-strong"},
		{"- item", ".cm-lp-bullet"},
		{"1. one", ".cm-lp-ordered"},
		{"> quoted", ".cm-lp-quote"},
		{"[ ] todo", ".cm-lp-task"},
		{"[a link](/o/x)", ".cm-lp-link"},
		{"---", ".cm-lp-hr"},
	} {
		b.eval(`(() => { const v = EDS[at(0).d].ed.view, n = v.state.doc.length; v.dispatch({changes: {from: n, insert: '\n\n'}, selection: {anchor: n + 2}}); })()`)
		b.typeKeys(c.typed)
		if !b.waitFor(`!!document.querySelector('.cm-editor ` + c.selector + `')`) {
			t.Errorf("%q does not convert: %s", c.typed, c.selector)
		}
	}
	if !b.edWait(`t.includes('- [ ] todo')`) {
		t.Errorf("[ ] becomes a to-do item: %q", b.eval(edText))
	}
	b.shot("editor-typing")
	if !b.waitFor(`document.querySelector('[data-edstatus]')?.textContent === 'Saved'`) {
		t.Errorf("the bar says it saved: %q", b.eval(`document.querySelector('[data-edstatus]')?.textContent`))
	}
	time.Sleep(1500 * time.Millisecond)
	b.open(base + "/o/" + page)
	b.ready("Typing")
	if got := b.eval(`[...document.querySelectorAll('article.doc [data-b]')].map(x => x.dataset.b).join(' ') + ' | ' + document.querySelector('article.doc [data-b="b3"] h2')?.textContent + ' | ' + !!document.querySelector('article.doc [data-b="b10"] hr')`); got != "b1 b3 b2 b4 b5 b6 b7 b8 b9 b10 | Heading | true" {
		t.Errorf("after a reload the blocks are kept: %q", got)
	}
	// Each paragraph written after a block became a block of its own, in the order of the text.
	if got := b.stored(page); got != "b1:First | b3:## Heading | b2:Second | b4:**bold** and *it* and `code` | b5:- item | b6:1. one | b7:> quoted | b8:- [ ] todo | b9:[a link](/o/x) | b10:---" {
		t.Errorf("the server has: %q", got)
	}
	b.clean("typing")
}

// The bar and Ctrl+B/I/K write Markdown around the selection; Turn into rewrites the lines; "/" opens the kinds.
func TestTheBarAndTheSlashMenu(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := makePage(b, "Bar", "First words", "Second")
	b.openEditing(base, page, "Bar")
	if got := b.eval(`(() => { const bar = document.querySelector('[data-win="0"] > .edbar'); const s = getComputedStyle(bar); return [s.visibility, [...bar.querySelectorAll('[data-act="edfmt"]')].map(x => x.dataset.k).join(',')].join(' '); })()`); got != "visible bold,italic,strike,code,link" {
		t.Errorf("the bar is a row of the window: %q", got)
	}
	b.lineWith("First words")
	b.press("ArrowLeft", "ArrowLeft", 37, 8) // Shift+Left x5: "words"
	for range 4 {
		b.press("ArrowLeft", "ArrowLeft", 37, 8)
	}
	b.press("b", "KeyB", 66, 2)
	if !b.edWait(`t.includes('First **words**')`) {
		t.Errorf("Ctrl+B: %q", b.eval(edText))
	}
	b.press("i", "KeyI", 73, 2)
	if !b.edWait(`t.includes('First **_words_**')`) {
		t.Errorf("Ctrl+I: %q", b.eval(edText))
	}
	b.clickOn(`.edbar [data-act="edfmt"][data-k="italic"]`, 1)
	if !b.edWait(`t.includes('First **words**')`) {
		t.Errorf("the bar takes italic away again: %q", b.eval(edText))
	}
	if got := b.eval(`document.activeElement?.classList.contains('cm-content')`); got != "true" {
		t.Errorf("the editor keeps the focus after the bar")
	}
	b.press("k", "KeyK", 75, 2)
	if !b.edWait(`t.includes('First **[words](https://)**')`) {
		t.Errorf("Ctrl+K: %q", b.eval(edText))
	}
	b.eval(`(() => { const s = document.querySelector('[data-edtype]'); s.value = 'h2'; s.dispatchEvent(new Event('change', {bubbles: true})); })()`)
	if !b.edWait(`t.includes('\n## First **[words](https://)**')`) {
		t.Errorf("Turn into a heading: %q", b.eval(edText))
	}
	if got := b.eval(`document.querySelector('[data-edtype]').value`); got != "h2" {
		t.Errorf("the bar names the kind of the line: %q", got)
	}
	// "/" at the start of an empty line opens the kinds; typing filters, Enter takes one.
	b.press("End", "End", 35, 0)
	b.typeKeys("\n\n/")
	if !b.waitFor(`!!document.querySelector('.cm-editor .bslash [role=option]')`) {
		t.Fatalf("the slash menu did not open")
	}
	b.shot("editor-slash")
	if got := b.eval(`[...document.querySelectorAll('.bslash [role=option]')].map(o => o.textContent).join(',')`); !strings.HasPrefix(got, "Text,Heading 1,") || !strings.Contains(got, "Embed a block or page") {
		t.Errorf("the kinds: %q", got)
	}
	b.typeKeys("tab")
	if got := b.eval(`[...document.querySelectorAll('.bslash [role=option]')].map(o => o.textContent).join(',')`); got != "Table" {
		t.Errorf("typing filters the kinds: %q", got)
	}
	b.press("Enter", "Enter", 13, 0)
	if !b.edWait(`t.includes('\n| Column | Column |\n|---|---|\n|  |  |') && !document.querySelector('.bslash')`) {
		t.Errorf("Enter makes a table: %q", b.eval(edText))
	}
	b.typeKeys("Port")
	b.press("Escape", "Escape", 27, 0)
	b.typeKeys("\n")
	b.press("End", "End", 35, 2)
	b.typeKeys("\n\n/zzz")
	if got := b.eval(`document.querySelector('.bslash')?.textContent`); got != "Nothing fits." {
		t.Errorf("nothing fits: %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
	if !b.waitFor(`!document.querySelector('.bslash')`) {
		t.Errorf("Escape closes the slash menu")
	}
	b.typeKeys(" not a menu")
	time.Sleep(1500 * time.Millisecond)
	b.clickOn(`[data-win="0"] .dstatus`, 1)
	if !b.waitFor(`!!document.querySelector('article.doc table') && !!document.querySelector('article.doc h2 strong a')`) {
		t.Errorf("reading shows what the bar wrote: %s", b.eval(`document.querySelector('article.doc')?.innerHTML.slice(0, 400)`))
	}
	b.clean("bar")
}

// The handle of a block in the editor: its menu moves and deletes the block; a block used elsewhere asks first, in
// place, and comes back when kept.
func TestTheHandleInTheEditor(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := makePage(b, "Handles", "One", "Two", "Three")
	other := makePage(b, "Uses it", "::embed{ref=\""+page+"#b2\"}")
	_ = other
	b.openEditing(base, page, "Handles")
	b.lineWith("Three")
	if got := b.eval(`(() => { const h = document.querySelector('.cm-ud-marker[data-edb="b3"] .bhandle'); return getComputedStyle(h).opacity + ' ' + h.title; })()`); got != "1 Block #b3: address, moving, comments" {
		t.Errorf("the handle of the block with the cursor shows: %q", got)
	}
	b.clickOn(`.cm-ud-marker[data-edb="b3"] .bhandle`, 1)
	if !b.waitFor(`!!document.querySelector('.cm-ud-marker[data-edb="b3"] .bmenu')`) {
		t.Fatal("the menu of the handle did not open")
	}
	if got := b.eval(`[...document.querySelectorAll('.bmenu [role=menuitem]')].map(x => x.textContent.trim() + (x.disabled ? '(off)' : '')).join(', ') + ' | ' + document.querySelector('.bmenu .bused').textContent`); got != "Copy address#b3, Move up, Move down(off), Delete, Move to page, Comment | Not used elsewhere" {
		t.Errorf("the menu in the editor: %q", got)
	}
	b.shot("editor-handle")
	b.clickOn(`.bmenu [data-act="edmove"][data-k="up"]`, 1)
	if !b.edWait(`t.indexOf('<!-- block b3 -->') < t.indexOf('<!-- block b2 -->')`) {
		t.Errorf("moved up in the text: %q", b.eval(edText))
	}
	if !b.waitFor(`document.querySelector('[data-edstatus]')?.textContent === 'Saved'`) {
		t.Errorf("the move is saved")
	}
	if got := b.stored(page); got != "b1:One | b3:Three | b2:Two" {
		t.Errorf("the server moved it: %q", got)
	}
	// Two is used on the other page: deleting it asks first, in place.
	b.clickOn(`.cm-ud-marker[data-edb="b2"] .bhandle`, 1)
	b.waitFor(`!!document.querySelector('.bmenu')`)
	if got := b.eval(`document.querySelector('.bmenu .bused').textContent`); got != "Used in Uses it" {
		t.Errorf("the menu says where it is used: %q", got)
	}
	b.clickOn(`.bmenu [data-act="eddel"]`, 1)
	if !b.waitFor(`!!document.querySelector('.cm-editor .cm-ud-ask[data-ask="b2"]')`) {
		t.Fatalf("a block used elsewhere asks: %q", b.eval(edText))
	}
	b.shot("editor-ask")
	if got := b.eval(`document.querySelector('.cm-ud-ask').textContent`); !strings.Contains(got, "Block #b2 is used in Uses it.") {
		t.Errorf("the question: %q", got)
	}
	if got := b.stored(page); got != "b1:One | b3:Three | b2:Two" {
		t.Errorf("nothing is deleted while it asks: %q", got)
	}
	b.clickOn(`.cm-ud-ask [data-answer="keep"]`, 1)
	if !b.edWait(`t.endsWith('<!-- block b2 -->\nTwo') && !document.querySelector('.cm-ud-ask')`) {
		t.Errorf("kept, the block comes back: %q", b.eval(edText))
	}
	time.Sleep(1500 * time.Millisecond)
	if got := b.stored(page); got != "b1:One | b3:Three | b2:Two" {
		t.Errorf("kept on the server: %q", got)
	}
	// Now delete it for good, and One, which is used nowhere, without a question.
	b.clickOn(`.cm-ud-marker[data-edb="b2"] .bhandle`, 1)
	b.waitFor(`!!document.querySelector('.bmenu')`)
	b.clickOn(`.bmenu [data-act="eddel"]`, 1)
	b.waitFor(`!!document.querySelector('.cm-ud-ask')`)
	b.clickOn(`.cm-ud-ask [data-answer="delete"]`, 1)
	// What happens in the page that uses it is chosen in a dialog, with nothing chosen in advance (ADR-22).
	if !b.waitFor(`!!document.querySelector('.wayspop')`) {
		t.Fatalf("the ways to choose from do not show")
	}
	if got := b.eval(`document.querySelector('.wayspop [data-act="waysgo"]').disabled + ' ' + document.querySelectorAll('.wayspop [aria-pressed="true"]').length`); got != "true 0" {
		t.Errorf("nothing is chosen in advance, and nothing goes before: %q", got)
	}
	b.clickOn(`.wayspop [data-act="way"][data-way="copies"]`, 1)
	b.clickOn(`.wayspop [data-act="waysgo"]`, 1)
	if !b.waitFor(`!document.querySelector('.cm-ud-ask') && !document.querySelector('.wayspop')`) {
		t.Errorf("the question goes")
	}
	time.Sleep(1200 * time.Millisecond)
	if got := b.stored(other); got != "b1:Two" {
		t.Errorf("the page that used it has a copy now: %q", got)
	}
	b.lineWith("One")
	b.press("Home", "Home", 36, 0)
	b.press("Backspace", "Backspace", 8, 0) // nothing before the first block
	b.lineWith("Three")
	b.press("Home", "Home", 36, 0)
	b.press("Backspace", "Backspace", 8, 0) // Three joins One
	if !b.edWait(`t === '<!-- block b1 -->\nOne\n\nThree'`) {
		t.Errorf("Backspace at the start of a block joins it to the one before: %q", b.eval(edText))
	}
	time.Sleep(1500 * time.Millisecond)
	if got := b.stored(page); got != "b1:One\n\nThree" {
		t.Errorf("the server: %q", got)
	}
	b.clean("handle")
}

// Pasting an address alone on an empty line embeds it; away from the cursor the embedding shows as a box with a link.
func TestPastingAnAddressEmbedsInTheEditor(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	source := makePage(b, "Source", "A table here")
	page := makePage(b, "Target", "Intro")
	b.openEditing(base, page, "Target")
	b.lineWith("Intro")
	b.typeKeys("\n\n")
	b.eval(`(() => { const dt = new DataTransfer(); dt.setData('text/plain', location.origin + '/o/` + source + `#b1');
		document.querySelector('.cm-content').dispatchEvent(new ClipboardEvent('paste', {clipboardData: dt, bubbles: true, cancelable: true})); })()`)
	if !b.edWait(`t.endsWith('::embed{ref="` + source + `#b1"}')`) {
		t.Errorf("the address became an embedding: %q", b.eval(edText))
	}
	b.lineWith("Intro")
	if !b.waitFor(`document.querySelector('.cm-editor .cm-ud-embed a')?.textContent === 'Source · #b1'`) {
		t.Errorf("the embedding shows as a box: %s", b.eval(`document.querySelector('.cm-editor .cm-ud-embed')?.outerHTML`))
	}
	if got := b.eval(`document.querySelector('.cm-editor .cm-ud-embed a').getAttribute('href')`); got != "/o/"+source+"#b1" {
		t.Errorf("with a link: %q", got)
	}
	b.shot("editor-embed")
	time.Sleep(1500 * time.Millisecond)
	if got := b.stored(page); got != "b1:Intro | b2:::embed{ref=\""+source+"#b1\"}" {
		t.Errorf("the server: %q", got)
	}
	b.clean("embed")
}

// A page changed elsewhere meanwhile is not overwritten: the editor loads it again and says so.
func TestTheEditorDoesNotOverwriteAChangeElsewhere(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := makePage(b, "Shared", "Mine")
	b.openEditing(base, page, "Shared")
	b.eval(`fetch('/api/pages/` + page + `/blocks/b1', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: '{"md":"Theirs"}'})`)
	time.Sleep(300 * time.Millisecond)
	b.lineWith("Mine")
	b.typeKeys(" and more")
	if !b.waitFor(`!!document.querySelector('[data-ednote]')`) {
		t.Fatalf("no notice: %q", b.eval(edText))
	}
	if !b.edWait(`t === '<!-- block b1 -->\nTheirs'`) {
		t.Errorf("the editor loaded the page again: %q", b.eval(edText))
	}
	if got := b.stored(page); got != "b1:Theirs" {
		t.Errorf("the other change stays: %q", got)
	}
	b.clean("conflict")
}

// The editor loads when it is needed: not on a page one only reads, but on the way into editing, and in an idle moment
// when editing is likely.
func TestTheEditorLoadsWhenNeeded(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	loaded := `String(!!window.UnderDocEditor) + ' ' + document.querySelectorAll('script[data-editor]').length`
	b.eval(`new Promise(r => setTimeout(r, 2000))`)
	if got := b.eval(loaded); got != "false 0" {
		t.Errorf("a page of the folder source loads no editor: %q", got)
	}
	b.eval(`window.made = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		const team = await post('/api/collections', {title: 'Team'});
		const page = await post('/api/pages', {title: 'Notes', parent: team.id});
		await post('/api/pages/' + page.id + '/blocks', {md: 'First', after: ''});
		window.made = page.id;
	})()`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the page for the test was not made")
	}
	page := b.eval(`window.made`)
	b.open(base + "/o/" + page)
	b.ready("Notes")
	if !b.waitFor(`!!window.UnderDocEditor && document.querySelectorAll('script[data-editor]').length === 1`) {
		t.Errorf("an editable page loads the editor in an idle moment: %q", b.eval(loaded))
	}
	b.clickOn(`[data-win="0"] .dstatus`, 1)
	if !b.waitFor(`!!document.querySelector('.cm-editor') && document.querySelectorAll('script[data-editor]').length === 1`) {
		t.Errorf("editing mounts it, loaded once: %q", b.eval(loaded))
	}
}

// Without an idle moment, the way into editing loads the editor itself.
func TestEditingLoadsTheEditorItself(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.call("Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": "window.requestIdleCallback = () => 0;"}, nil)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.eval(`window.made = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		const team = await post('/api/collections', {title: 'Team'});
		const page = await post('/api/pages', {title: 'Notes', parent: team.id});
		await post('/api/pages/' + page.id + '/blocks', {md: 'First', after: ''});
		window.made = page.id;
	})()`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the page for the test was not made")
	}
	b.open(base + "/o/" + b.eval(`window.made`))
	b.ready("Notes")
	if got := b.eval(`String(!!window.UnderDocEditor)`); got != "false" {
		t.Fatalf("no idle moment, nothing loaded yet: %s", got)
	}
	b.eval(`needEditor(); needEditor(); document.querySelector('[data-win="0"] .dstatus').click()`)
	if !b.waitFor(`!!document.querySelector('.cm-editor')`) {
		t.Errorf("editing loads and mounts the editor: %s", b.eval(`document.querySelector('[data-cmhost]')?.textContent`))
	}
	if got := b.eval(`document.querySelectorAll('script[data-editor]').length`); got != "1" {
		t.Errorf("asked three times while loading, it loads once: %s", got)
	}
}

// The editor keeps the rhythm of the page as it is read (2026-11-10): a blank line between paragraphs, and so between
// blocks with the hidden marker, is as high as the space after a paragraph when read, not a whole line, as in Typora or
// Obsidian. On the cursor it is a whole line again, and blank lines in code stay lines. Measured from baseline to
// baseline of consecutive lines; before, two paragraphs stood 48 px apart in the editor and 34.3 px when read.
func TestTheEditorKeepsTheRhythmOfThePage(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := makePage(b, "Rhythm", "Alpha one.", "Bravo two.", "Charlie three.\n\nDelta four.", "## Echo head", "Foxtrot five.", "- Golf item\n- Hotel item", "India six.", "```text\nJuliet\n\nKilo\n```")
	b.open(base + "/o/" + page)
	b.ready("Rhythm")
	// The bottom of each word, line by line, as distances from the one before.
	steps := `(root => { const at = ["Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Golf", "Hotel", "India", "Juliet", "Kilo"].map(w => { const walk = document.createTreeWalker(document.querySelector(root), NodeFilter.SHOW_TEXT);
		for (let n = walk.nextNode(); n; n = walk.nextNode()) { const k = n.data.indexOf(w); if (k < 0) continue; const r = document.createRange(); r.setStart(n, k); r.setEnd(n, k + w.length); return r.getBoundingClientRect().bottom; } return NaN; });
		return JSON.stringify(at.slice(1).map((y, k) => Math.round((y - at[k]) * 10) / 10)); })`
	var read, edit []float64
	if err := json.Unmarshal([]byte(b.eval(steps+`('article.doc')`)), &read); err != nil || len(read) != 10 {
		t.Fatalf("reading: %v %v", read, err)
	}
	b.clickOn(`[data-win="0"] .dstatus`, 1)
	if !b.waitFor(`!!document.querySelector('.cm-ud-marker .bhandle')`) {
		t.Fatal("no editor")
	}
	// Away from the cursor: the editor loses the focus.
	b.clickOn(`.dtitle h1`, 1)
	time.Sleep(150 * time.Millisecond)
	if err := json.Unmarshal([]byte(b.eval(steps+`('.cm-content')`)), &edit); err != nil || len(edit) != 10 {
		t.Fatalf("editor: %v %v", edit, err)
	}
	t.Logf("baseline to baseline, reading %v, editor %v", read, edit)
	// Paragraph to paragraph, between blocks and within one: as when read.
	for _, k := range []int{0, 1, 2} {
		if d := edit[k] - read[k]; d > 0.25 || d < -0.25 {
			t.Errorf("paragraph %d to the next: %.1f px in the editor, %.1f px when read", k+1, edit[k], read[k])
		}
	}
	// Headings and lists: close to it.
	for _, k := range []int{3, 4, 5, 6, 7} {
		if d := edit[k] - read[k]; d > 3 || d < -3 {
			t.Errorf("line %d to the next: %.1f px in the editor, %.1f px when read", k+1, edit[k], read[k])
		}
	}
	// In code a blank line is a line: two lines of code apart.
	if got := b.eval(`(() => { const l = [...document.querySelectorAll('.cm-line.cm-lp-code')].find(x => !x.textContent), j = [...document.querySelectorAll('.cm-line.cm-lp-code')].find(x => x.textContent === 'Juliet');
		return !!l && Math.abs(l.getBoundingClientRect().height - j.getBoundingClientRect().height) < 1; })()`); got != "true" {
		t.Errorf("a blank line in code stays a line: %.1f px from Juliet to Kilo", edit[9])
	}
	// On the cursor the blank line is a whole line again.
	b.eval(`(() => { const v = EDS[at(0).d].ed.view, t = v.state.doc.toString(), at0 = t.indexOf('Charlie three.\n') + 'Charlie three.\n'.length; v.focus(); v.dispatch({selection: {anchor: at0}}); })()`)
	time.Sleep(150 * time.Millisecond)
	if err := json.Unmarshal([]byte(b.eval(steps+`('.cm-content')`)), &edit); err != nil {
		t.Fatal(err)
	}
	if line := edit[6]; edit[2] < 2*line-1 {
		t.Errorf("with the cursor on the blank line it is a whole line: %.1f px, a line is %.1f px", edit[2], line)
	}
	b.shot("editor-rhythm")
}

// The table tools of the bar (M-8): shown while the cursor stands in a table, each writes the table again as plain
// Markdown, and the cursor goes to the cell the tool names.
func TestTheTableTools(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := makePage(b, "Tables", "Before", "| Port | VLAN |\n|---|---:|\n| 1 | 10 |")
	b.openEditing(base, page, "Tables")
	if got := b.eval(`document.querySelector('[data-edtable]').hidden`); got != "true" {
		t.Errorf("the table tools hide while the cursor is in no table: %q", got)
	}
	b.clickOn(`.cm-editor .cm-lp-table-widget td`, 1)
	if !b.waitFor(`!document.querySelector('[data-edtable]').hidden`) {
		t.Fatalf("the table tools did not show in the table: %s", b.eval(edText))
	}
	b.shot("editor-table-tools")
	b.lineWith("| 1 | 10 |")
	b.clickOn(`.edbar [data-act="edtable"][data-k="rowBelow"]`, 1)
	if !b.edWait(`t.includes('| 1 | 10 |\n|  |  |')`) {
		t.Fatalf("Row below: %q", b.eval(edText))
	}
	b.typeKeys("x")
	if !b.edWait(`t.includes('| 1 | 10 |\n|  | x |')`) {
		t.Errorf("the cursor stands in the new row, in the column it was in: %q", b.eval(edText))
	}
	b.clickOn(`.edbar [data-act="edtable"][data-k="colLeft"]`, 1)
	b.typeKeys("y")
	if !b.edWait(`t.includes('| Port |  | VLAN |\n|---|---|---:|\n| 1 |  | 10 |\n|  | y | x |')`) {
		t.Errorf("Column left: %q", b.eval(edText))
	}
	b.clickOn(`.edbar [data-act="edtable"][data-k="alignCenter"]`, 1)
	if !b.edWait(`t.includes('|---|:---:|---:|')`) {
		t.Errorf("Center column: %q", b.eval(edText))
	}
	b.clickOn(`.edbar [data-act="edtable"][data-k="rowAbove"]`, 1)
	b.clickOn(`.edbar [data-act="edtable"][data-k="rowDelete"]`, 1)
	b.clickOn(`.edbar [data-act="edtable"][data-k="colDelete"]`, 1)
	want := "| Port | VLAN |\n|---|---:|\n| 1 | 10 |\n|  | x |"
	if !b.edWait(`t.includes(` + strconv.Quote(want) + `)`) {
		t.Errorf("Row above, then Delete row and Delete column: %q", b.eval(edText))
	}
	b.lineWith("| Port | VLAN |")
	b.clickOn(`.edbar [data-act="edtable"][data-k="rowDelete"]`, 1)
	if !b.waitFor(`/header row stays/.test(document.body.textContent)`) {
		t.Errorf("the header row does not go, and the workspace says why")
	}
	if !b.edWait(`t.includes(` + strconv.Quote(want) + `)`) {
		t.Errorf("the header row stays: %q", b.eval(edText))
	}
	b.lineWith("Before")
	if !b.waitFor(`document.querySelector('[data-edtable]').hidden`) {
		t.Errorf("the table tools hide again outside the table")
	}
	time.Sleep(1500 * time.Millisecond)
	if got := b.stored(page); got != "b1:Before | b2:"+want {
		t.Errorf("the server has: %q", got)
	}
	b.clean("table tools")
}

// On a phone the bar wraps its table tools instead of scrolling them out of sight: they are C22's .bgroup (C22 #123), and
// every tool lies inside the window.
func TestTheTableToolsOnAPhone(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.phone(390, 844)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := makePage(b, "Tables", "Before", "| Port | VLAN |\n|---|---:|\n| 1 | 10 |")
	b.openEditing(base, page, "Tables")
	b.tap(`.cm-editor .cm-lp-table-widget tr:nth-child(2) td:nth-child(2)`)
	if !b.waitFor(`!document.querySelector('[data-edtable]').hidden`) {
		// A finger on the drawn table may land beside its text; the line of the table takes the cursor as well.
		t.Fatalf("a finger on the table puts the cursor into it, and the table tools show: %s", b.eval(edText))
	}
	if got := b.eval(`(() => { const s = EDS[at(0).d].ed.view.state, h = s.selection.main.head; return s.doc.sliceString(h, h + 2); })()`); got != "10" {
		t.Errorf("the cursor stands in the cell the finger touched: %q", got)
	}
	if got := b.eval(`document.querySelector('.edbar [data-edtable]').classList.contains('bgroup')`); got != "true" {
		t.Errorf("the table tools are a group of the bar")
	}
	out := b.eval(`(() => { const bar = document.querySelector('.edbar'), w = innerWidth;
		const outside = [...bar.querySelectorAll('[data-act="edtable"]')].filter(x => { const r = x.getBoundingClientRect(); return r.width === 0 || r.left < 0 || r.right > w + 0.5; }).map(x => x.dataset.k);
		return outside.join(',') + '|' + (bar.scrollWidth > bar.clientWidth + 1); })()`)
	if out != "|false" {
		t.Errorf("on a phone the table tools wrap inside the window: outside %q", out)
	}
	b.shot("editor-table-tools-phone")
	b.clean("table tools on a phone")
}

// A block used elsewhere shows its boundary and where it is used while the cursor is in it (ADR-22), so whoever writes
// there knows that other pages change too; other blocks show nothing of the kind.
func TestAUsedBlockShowsItsBoundary(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := makePage(b, "Used", "One", "Two\nand more", "Three")
	makePage(b, "Uses it", "::embed{ref=\""+page+"#b2\"}")
	makePage(b, "Uses it too", "::embed{ref=\""+page+"#b2\"}")
	b.openEditing(base, page, "Used")
	b.lineWith("One")
	if got := b.eval(`document.querySelectorAll('.cm-editor .cm-ud-used, .cm-editor .cm-ud-usedlabel').length`); got != "0" {
		t.Errorf("a block used nowhere shows no boundary: %q", got)
	}
	b.lineWith("and more")
	if !b.waitFor(`document.querySelectorAll('.cm-editor .cm-line.cm-ud-used').length === 2`) {
		t.Fatalf("the lines of the used block are outlined: %q", b.eval(`[...document.querySelectorAll('.cm-editor .cm-ud-used')].map(l => l.textContent).join(' / ')`))
	}
	if got := b.eval(`(() => { const f = document.querySelector('.cm-ud-used-first'), l = document.querySelector('.cm-ud-used-last'), x = f.querySelector('.cm-ud-usedlabel');
		return [f.textContent.replace(x.textContent, ''), l.textContent, x.textContent, getComputedStyle(f).boxShadow !== 'none', x.getBoundingClientRect().right <= innerWidth].join(' | '); })()`); got != "Two | and more | Used in 2 places | true | true" {
		t.Errorf("the boundary and its label: %q", got)
	}
	b.shot("editor-used-block")
	b.press("ArrowDown", "ArrowDown", 40, 0)
	b.press("ArrowDown", "ArrowDown", 40, 0)
	b.press("ArrowDown", "ArrowDown", 40, 0)
	if !b.waitFor(`!document.querySelector('.cm-editor .cm-ud-used, .cm-editor .cm-ud-usedlabel')`) {
		t.Errorf("leaving the block takes the boundary away: %s", b.eval(edText))
	}
	time.Sleep(1200 * time.Millisecond)
	if got := b.eval(`window.writes.join(', ')`); got != "" {
		t.Errorf("showing the boundary wrote: %s", got)
	}
	b.clean("used block")
}
