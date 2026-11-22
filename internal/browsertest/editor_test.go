//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"encoding/json"
	"fmt"
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
	b.clickOn(`[data-win="0"] [data-mode]`, 1)
	if !b.waitFor(`!!document.querySelector('article.typeset .cm-editor .cm-content') && document.activeElement?.classList.contains('cm-content')`) {
		b.t.Fatalf("the page did not open in the editor: %s", b.eval(`document.querySelector('article.typeset')?.innerHTML.slice(0, 300)`))
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
	if got := b.eval(`document.querySelectorAll('.cm-editor .cm-ud-marker').length + ' ' + document.querySelectorAll('.cm-editor .cm-ud-marker .editor-handle > button').length + ' ' + !document.querySelector('textarea')`); got != "8 8 true" {
		t.Errorf("one hidden marker with a handle per block, no field: %q", got)
	}
	if got := b.eval(`[...document.querySelectorAll('.cm-editor .cm-line')].some(l => /<!-- block b\d -->/.test(l.textContent) && !l.closest('[data-md~=codeblock]'))`); got != "false" {
		t.Errorf("a marker shows as text")
	}
	b.shot("editor-round-trip")
	b.press("ArrowDown", "ArrowDown", 40, 0)
	b.press("ArrowDown", "ArrowDown", 40, 0)
	time.Sleep(1500 * time.Millisecond)
	b.clickOn(`[data-win="0"] [data-mode]`, 1) // back to reading
	if !b.waitFor(`!document.querySelector('.cm-editor') && !!document.querySelector('article.typeset [data-block="b8"]')`) {
		t.Fatalf("reading again: %s", b.eval(`document.querySelector('article.typeset')?.innerHTML.slice(0, 200)`))
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
	if !b.waitFor(`!!document.querySelector('.cm-editor .cm-line[data-md~=h2]')`) {
		t.Errorf("a heading typed shows as a heading")
	}
	// On the cursor's line its sign stays, muted (data-md="mark"): not in the colour of the heading.
	if got := b.eval(`(() => { const l = document.querySelector('.cm-editor .cm-line[data-md~=h2]'), m = l && [...l.querySelectorAll('[data-md~=mark]')].find(x => x.textContent === '##'); return !!m && getComputedStyle(m).color !== getComputedStyle(l).color; })()`); got != "true" {
		t.Errorf("the sign of a heading on the cursor's line is muted: %q", got)
	}
	if !b.edWait(`t.includes('<!-- block b3 -->\n## Heading')`) {
		t.Fatalf("the new paragraph did not become block b3: %q", b.eval(edText))
	}
	if got := b.eval(`(() => { const v = EDS[at(0).d].ed.view; return v.state.doc.sliceString(v.state.selection.main.head - 7, v.state.selection.main.head); })()`); got != "Heading" {
		t.Errorf("the cursor stays after what was typed: %q", got)
	}
	// Live shortcuts, each typed with real keys into a paragraph of its own at the end of the page. The blank line before
	// it is put there directly: Enter after a list item would continue the list, as it should. The live preview says what
	// a line, a mark or a widget is by C22's data-md; what shows on the cursor's line is looked for at once, a widget that
	// stands in for Markdown (the dot, the box, the rule) once the cursor has left the line.
	shortcuts := []struct {
		typed, shown string
		away         bool
	}{
		{"**bold** and *it* and `code`", `!!q('.cm-line [data-md~=strong]') && !!q('.cm-line [data-md~=em]') && !!q('.cm-line [data-md~=code]')`, false},
		{"- item", `!!q('.cm-line [data-md~=bullet]')`, true},
		// The number of an ordered list is never hidden: it stays as a muted sign.
		{"1. one", `[...document.querySelectorAll('.cm-editor .cm-line [data-md~=mark]')].some(m => m.textContent === '1.')`, false},
		{"> quoted", `!!q('.cm-line[data-md~=quote]')`, false},
		{"[ ] todo", `!!q('.cm-line input[type=checkbox][data-md~=task]')`, true},
		{"[a link](/o/x)", `!!q('.cm-line [data-md~=link]') && !!q('.cm-line [data-md~=url]')`, false},
		{"---", `!!q('.cm-line [data-md~=rule]')`, true},
	}
	shown := func(c string) bool {
		return b.waitFor(`(() => { const q = s => document.querySelector('.cm-editor ' + s); return ` + c + `; })()`)
	}
	for _, c := range shortcuts {
		b.eval(`(() => { const v = EDS[at(0).d].ed.view, n = v.state.doc.length; v.dispatch({changes: {from: n, insert: '\n\n'}, selection: {anchor: n + 2}}); })()`)
		b.typeKeys(c.typed)
		if !c.away && !shown(c.shown) {
			t.Errorf("%q does not convert: %s", c.typed, c.shown)
		}
	}
	// The cursor leaves for the first line: every widget stands in for its Markdown.
	b.eval(`(() => { const v = EDS[at(0).d].ed.view; v.dispatch({selection: {anchor: v.state.doc.toString().indexOf('First')}}); })()`)
	for _, c := range shortcuts {
		if c.away && !shown(c.shown) {
			t.Errorf("%q does not convert away from the cursor: %s", c.typed, c.shown)
		}
	}
	// The look is C22's, not a rule of the editor's own: its rules sit in a layer, CodeMirror's base theme does not, so
	// this proves that nothing of CodeMirror's covers them. A heading takes the pack's size of its level (--editor-h2),
	// bold is bold, a quote stands in with its bar, the rule is a line, the dot of a list is bold.
	look := `(() => { const q = s => document.querySelector('.cm-editor ' + s), cs = s => getComputedStyle(q(s)), ed = getComputedStyle(q('.cm-content').closest('.editor'));
		const px = v => { const d = document.createElement('div'); d.style.width = v; q('.cm-content').closest('.editor').append(d); const w = d.getBoundingClientRect().width; d.remove(); return w; };
		return [Math.abs(parseFloat(cs('.cm-line[data-md~=h2]').fontSize) - px(ed.getPropertyValue('--editor-h2'))) < 0.5, +cs('[data-md~=strong]').fontWeight >= 700,
			parseFloat(cs('.cm-line[data-md~=quote]').marginInlineStart) > 0 && cs('.cm-line[data-md~=quote]').fontStyle === 'italic', cs('[data-md~=rule]').borderTopStyle === 'solid',
			+cs('[data-md~=bullet]').fontWeight >= 700].join(' '); })()`
	if got := b.eval(look); got != "true true true true true" {
		t.Errorf("the live preview has C22's look (heading size, bold, quote, rule, dot): %q", got)
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
	if got := b.eval(`[...document.querySelectorAll('article.typeset [data-block]')].map(x => x.dataset.block).join(' ') + ' | ' + document.querySelector('article.typeset [data-block="b3"] h2')?.textContent + ' | ' + !!document.querySelector('article.typeset [data-block="b10"] hr')`); got != "b1 b3 b2 b4 b5 b6 b7 b8 b9 b10 | Heading | true" {
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
	if got := b.eval(`(() => { const bar = document.querySelector('[data-win="0"] .workspace-main > [data-edbar]'); const s = getComputedStyle(bar); return [s.visibility, [...bar.querySelectorAll('[data-act="edfmt"]')].map(x => x.dataset.k).join(',')].join(' '); })()`); got != "visible bold,italic,strike,code,link" {
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
	b.clickOn(`[data-edbar] [data-act="edfmt"][data-k="italic"]`, 1)
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
	if !b.waitFor(`!!document.querySelector('.cm-editor .command [role=menuitem]')`) {
		t.Fatalf("the slash menu did not open")
	}
	b.shot("editor-slash")
	if got := b.eval(`[...document.querySelectorAll('.command [role=menuitem]')].map(o => o.textContent).join(',')`); !strings.HasPrefix(got, "Text,Heading 1,") || !strings.Contains(got, "Embed a block or page") {
		t.Errorf("the kinds: %q", got)
	}
	b.typeKeys("tab")
	if got := b.eval(`[...document.querySelectorAll('.command [role=menuitem]')].map(o => o.textContent).join(',')`); got != "Table" {
		t.Errorf("typing filters the kinds: %q", got)
	}
	b.press("Enter", "Enter", 13, 0)
	if !b.edWait(`t.includes('\n| Column | Column |\n|---|---|\n|  |  |') && !document.querySelector('.cm-editor .command')`) {
		t.Errorf("Enter makes a table: %q", b.eval(edText))
	}
	b.typeKeys("Port")
	b.press("Escape", "Escape", 27, 0)
	b.typeKeys("\n")
	b.press("End", "End", 35, 2)
	b.typeKeys("\n\n/zzz")
	if got := b.eval(`(() => { const m = document.querySelector('.cm-editor .command [role=menu]'); return m && !m.querySelector('[role=menuitem]') ? getComputedStyle(m, '::before').content : 'none'; })()`); got != `"Nothing fits."` {
		t.Errorf("nothing fits: %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
	if !b.waitFor(`!document.querySelector('.cm-editor .command')`) {
		t.Errorf("Escape closes the slash menu")
	}
	b.typeKeys(" not a menu")
	time.Sleep(1500 * time.Millisecond)
	b.clickOn(`[data-win="0"] [data-mode]`, 1)
	if !b.waitFor(`!!document.querySelector('article.typeset table') && !!document.querySelector('article.typeset h2 strong a')`) {
		t.Errorf("reading shows what the bar wrote: %s", b.eval(`document.querySelector('article.typeset')?.innerHTML.slice(0, 400)`))
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
	if got := b.eval(`(() => { const h = document.querySelector('.cm-ud-marker[data-edb="b3"] .editor-handle'); return getComputedStyle(h).opacity + ' ' + h.querySelector(':scope > button').title; })()`); got != "1 Block #b3: address, moving, comments" {
		t.Errorf("the handle of the block with the cursor shows: %q", got)
	}
	b.clickOn(`.cm-ud-marker[data-edb="b3"] .editor-handle > button`, 1)
	if !b.waitFor(`!!document.querySelector('.cm-ud-marker[data-edb="b3"] .editor-handle [role=menu]')`) {
		t.Fatal("the menu of the handle did not open")
	}
	if got := b.eval(`[...document.querySelectorAll('.editor-handle [role=menu] [role=menuitem]')].map(x => x.textContent.trim() + (x.disabled ? '(off)' : '')).join(', ') + ' | ' + document.querySelector('.editor-handle [role=menu] > [role=heading]').textContent`); got != "Copy address#b3, Move up, Move down(off), Delete, Move to page, Comment | Not used elsewhere" {
		t.Errorf("the menu in the editor: %q", got)
	}
	b.shot("editor-handle")
	b.clickOn(`.editor-handle [role=menu] [data-act="edmove"][data-k="up"]`, 1)
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
	b.clickOn(`.cm-ud-marker[data-edb="b2"] .editor-handle > button`, 1)
	b.waitFor(`!!document.querySelector('.editor-handle [role=menu]')`)
	if got := b.eval(`document.querySelector('.editor-handle [role=menu] > [role=heading]').textContent`); got != "Used in Uses it" {
		t.Errorf("the menu says where it is used: %q", got)
	}
	b.clickOn(`.editor-handle [role=menu] [data-act="eddel"]`, 1)
	if !b.waitFor(`!!document.querySelector('.cm-editor [data-ask="b2"]')`) {
		t.Fatalf("a block used elsewhere asks: %q", b.eval(edText))
	}
	b.shot("editor-ask")
	if got := b.eval(`document.querySelector('[data-ask]').textContent`); !strings.Contains(got, "Block #b2 is used in Uses it.") {
		t.Errorf("the question: %q", got)
	}
	if got := b.stored(page); got != "b1:One | b3:Three | b2:Two" {
		t.Errorf("nothing is deleted while it asks: %q", got)
	}
	b.clickOn(`[data-ask] [data-answer="keep"]`, 1)
	if !b.edWait(`t.endsWith('<!-- block b2 -->\nTwo') && !document.querySelector('[data-ask]')`) {
		t.Errorf("kept, the block comes back: %q", b.eval(edText))
	}
	time.Sleep(1500 * time.Millisecond)
	if got := b.stored(page); got != "b1:One | b3:Three | b2:Two" {
		t.Errorf("kept on the server: %q", got)
	}
	// Now delete it for good, and One, which is used nowhere, without a question.
	b.clickOn(`.cm-ud-marker[data-edb="b2"] .editor-handle > button`, 1)
	b.waitFor(`!!document.querySelector('.editor-handle [role=menu]')`)
	b.clickOn(`.editor-handle [role=menu] [data-act="eddel"]`, 1)
	b.waitFor(`!!document.querySelector('[data-ask]')`)
	b.clickOn(`[data-ask] [data-answer="delete"]`, 1)
	// What happens in the page that uses it is chosen in a dialog, with nothing chosen in advance (ADR-22).
	if !b.waitFor(`!!document.querySelector('[data-ways]')`) {
		t.Fatalf("the ways to choose from do not show")
	}
	if got := b.eval(`document.querySelector('[data-ways] [data-act="waysgo"]').disabled + ' ' + document.querySelectorAll('[data-ways] [aria-pressed="true"]').length`); got != "true 0" {
		t.Errorf("nothing is chosen in advance, and nothing goes before: %q", got)
	}
	b.clickOn(`[data-ways] [data-act="way"][data-way="copies"]`, 1)
	b.clickOn(`[data-ways] [data-act="waysgo"]`, 1)
	if !b.waitFor(`!document.querySelector('[data-ask]') && !document.querySelector('[data-ways]')`) {
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
	if !b.waitFor(`document.querySelector('.cm-editor a.badge[data-ref] > span:last-child')?.textContent === 'Source · #b1'`) {
		t.Errorf("the embedding shows as a box: %s", b.eval(`document.querySelector('.cm-editor a.badge[data-ref]')?.outerHTML`))
	}
	if got := b.eval(`document.querySelector('.cm-editor a.badge[data-ref]').getAttribute('href')`); got != "/o/"+source+"#b1" {
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
	b.clickOn(`[data-win="0"] [data-mode]`, 1)
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
	b.eval(`needEditor(); needEditor(); document.querySelector('[data-win="0"] [data-mode]').click()`)
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
	if err := json.Unmarshal([]byte(b.eval(steps+`('article.typeset')`)), &read); err != nil || len(read) != 10 {
		t.Fatalf("reading: %v %v", read, err)
	}
	b.clickOn(`[data-win="0"] [data-mode]`, 1)
	if !b.waitFor(`!!document.querySelector('.cm-ud-marker .editor-handle > button')`) {
		t.Fatal("no editor")
	}
	// Away from the cursor: the editor loses the focus.
	b.clickOn(`.workspace-main > header > h1`, 1)
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
	if got := b.eval(`(() => { const l = [...document.querySelectorAll('.cm-line[data-md~=codeblock]')].find(x => !x.textContent), j = [...document.querySelectorAll('.cm-line[data-md~=codeblock]')].find(x => x.textContent === 'Juliet');
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

// While reading, the flow of the page runs through the wrappers of the blocks (div[data-block]): two paragraphs of one
// block stand as far apart as two blocks, also in the first block, and a heading stands as far from the text under it in
// its own block as from the next block.
func TestTheFlowRunsThroughABlock(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := makePage(b, "Flow", "Alpha one.\n\nAlpha two.", "Bravo three.", "## Charlie head\n\nCharlie text.", "## Delta head", "Delta text.")
	b.open(base + "/o/" + page)
	b.ready("Flow")
	gaps := b.eval(`(() => { const top = w => { const walk = document.createTreeWalker(document.querySelector('article.typeset'), NodeFilter.SHOW_TEXT);
		for (let n = walk.nextNode(); n; n = walk.nextNode()) { const k = n.data.indexOf(w); if (k >= 0) { const e = n.parentElement.closest('p, h2'); return [e.getBoundingClientRect().top, e.getBoundingClientRect().bottom]; } } return [NaN, NaN]; };
		const gap = (a, z) => Math.round((top(z)[0] - top(a)[1]) * 10) / 10;
		return [gap('Alpha one', 'Alpha two'), gap('Alpha two', 'Bravo three'), gap('Charlie head', 'Charlie text'), gap('Delta head', 'Delta text')].join(' '); })()`)
	var inBlock, between, headIn, headNext float64
	if _, err := fmt.Sscan(gaps, &inBlock, &between, &headIn, &headNext); err != nil {
		t.Fatalf("%q: %v", gaps, err)
	}
	if inBlock < 5 || inBlock-between > 0.25 || between-inBlock > 0.25 {
		t.Errorf("two paragraphs of the first block stand as far apart as two blocks: %.1f px within, %.1f px between", inBlock, between)
	}
	if headIn-headNext > 0.25 || headNext-headIn > 0.25 {
		t.Errorf("a heading stands as far from the text in its block as from the next block: %.1f px within, %.1f px to the next", headIn, headNext)
	}
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
	b.clickOn(`.cm-editor table[data-md~=table] td`, 1)
	if !b.waitFor(`!document.querySelector('[data-edtable]').hidden`) {
		t.Fatalf("the table tools did not show in the table: %s", b.eval(edText))
	}
	b.shot("editor-table-tools")
	b.lineWith("| 1 | 10 |")
	b.clickOn(`[data-edbar] [data-act="edtable"][data-k="rowBelow"]`, 1)
	if !b.edWait(`t.includes('| 1 | 10 |\n|  |  |')`) {
		t.Fatalf("Row below: %q", b.eval(edText))
	}
	b.typeKeys("x")
	if !b.edWait(`t.includes('| 1 | 10 |\n|  | x |')`) {
		t.Errorf("the cursor stands in the new row, in the column it was in: %q", b.eval(edText))
	}
	b.clickOn(`[data-edbar] [data-act="edtable"][data-k="colLeft"]`, 1)
	b.typeKeys("y")
	if !b.edWait(`t.includes('| Port |  | VLAN |\n|---|---|---:|\n| 1 |  | 10 |\n|  | y | x |')`) {
		t.Errorf("Column left: %q", b.eval(edText))
	}
	b.clickOn(`[data-edbar] [data-act="edtable"][data-k="alignCenter"]`, 1)
	if !b.edWait(`t.includes('|---|:---:|---:|')`) {
		t.Errorf("Center column: %q", b.eval(edText))
	}
	b.clickOn(`[data-edbar] [data-act="edtable"][data-k="rowAbove"]`, 1)
	b.clickOn(`[data-edbar] [data-act="edtable"][data-k="rowDelete"]`, 1)
	b.clickOn(`[data-edbar] [data-act="edtable"][data-k="colDelete"]`, 1)
	want := "| Port | VLAN |\n|---|---:|\n| 1 | 10 |\n|  | x |"
	if !b.edWait(`t.includes(` + strconv.Quote(want) + `)`) {
		t.Errorf("Row above, then Delete row and Delete column: %q", b.eval(edText))
	}
	b.lineWith("| Port | VLAN |")
	b.clickOn(`[data-edbar] [data-act="edtable"][data-k="rowDelete"]`, 1)
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
	b.tap(`.cm-editor table[data-md~=table] tr:nth-child(2) td:nth-child(2)`)
	if !b.waitFor(`!document.querySelector('[data-edtable]').hidden`) {
		// A finger on the drawn table may land beside its text; the line of the table takes the cursor as well.
		t.Fatalf("a finger on the table puts the cursor into it, and the table tools show: %s", b.eval(edText))
	}
	if got := b.eval(`(() => { const s = EDS[at(0).d].ed.view.state, h = s.selection.main.head; return s.doc.sliceString(h, h + 2); })()`); got != "10" {
		t.Errorf("the cursor stands in the cell the finger touched: %q", got)
	}
	if got := b.eval(`document.querySelector('[data-edbar] > [data-edtable]')?.getAttribute('role') === 'group'`); got != "true" {
		t.Errorf("the table tools are a group of the bar")
	}
	out := b.eval(`(() => { const bar = document.querySelector('[data-edbar]'), w = innerWidth;
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
	if got := b.eval(`document.querySelectorAll('.cm-editor [data-used], .cm-editor [data-used] > small').length`); got != "0" {
		t.Errorf("a block used nowhere shows no boundary: %q", got)
	}
	b.lineWith("and more")
	if !b.waitFor(`document.querySelectorAll('.cm-editor .cm-line[data-used]').length === 2`) {
		t.Fatalf("the lines of the used block are outlined: %q", b.eval(`[...document.querySelectorAll('.cm-editor [data-used]')].map(l => l.textContent).join(' / ')`))
	}
	if got := b.eval(`(() => { const f = document.querySelector('.cm-line[data-used="first"]'), l = document.querySelector('.cm-line[data-used="last"]'), x = f.querySelector(':scope > small');
		return [f.textContent.replace(x.textContent, ''), l.textContent, x.textContent, parseFloat(getComputedStyle(f, '::before').borderTopWidth) >= 1 && parseFloat(getComputedStyle(l, '::before').borderBottomWidth) >= 1 && getComputedStyle(f, '::before').borderBottomWidth === '0px', x.getBoundingClientRect().right <= innerWidth].join(' | '); })()`); got != "Two | and more | Used in 2 places | true | true" {
		t.Errorf("the boundary and its label: %q", got)
	}
	b.shot("editor-used-block")
	b.press("ArrowDown", "ArrowDown", 40, 0)
	b.press("ArrowDown", "ArrowDown", 40, 0)
	b.press("ArrowDown", "ArrowDown", 40, 0)
	if !b.waitFor(`!document.querySelector('.cm-editor [data-used], .cm-editor [data-used] > small')`) {
		t.Errorf("leaving the block takes the boundary away: %s", b.eval(edText))
	}
	time.Sleep(1200 * time.Millisecond)
	if got := b.eval(`window.writes.join(', ')`); got != "" {
		t.Errorf("showing the boundary wrote: %s", got)
	}
	b.clean("used block")
}
