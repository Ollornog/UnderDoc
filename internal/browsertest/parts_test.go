//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// Taking away a part in use asks in a dialog (ADR-22): nothing chosen in advance, delete everywhere leaves a placeholder
// that says who removed it, and Restore on it brings the part back in every use. On a phone the dialog fits the screen and
// a finger chooses.
func TestTheWaysOfAPartInUse(t *testing.T) {
	for _, phone := range []bool{false, true} {
		name := "desktop"
		if phone {
			name = "phone"
		}
		t.Run(name, func(t *testing.T) {
			b, base := setupWithDatabase(t)
			if phone {
				b.phone(390, 844)
			}
			press := func(selector string) {
				if phone {
					b.tap(selector)
				} else {
					b.clickOn(selector, 1)
				}
			}
			b.open(base + "/o/plan")
			b.ready("Network plan")
			errors := makePage(b, "Errors", "Error 42: <!-- part t1 -->the disk is full<!-- /part t1 -->. Free space.")
			runbook := makePage(b, "Runbook", "If :embed{ref=\""+errors+"#b1.t1\"}, act.")
			b.openEditing(base, errors, "Errors")
			// The text of the part goes; saving asks, since Runbook embeds it.
			b.eval(`(() => { const v = EDS[at(0).d].ed.view, t = v.state.doc.toString(), from = t.indexOf('<!-- part t1 -->'), to = t.indexOf('<!-- /part t1 -->') + '<!-- /part t1 -->'.length; v.dispatch({changes: {from, to, insert: 'it'}, userEvent: 'input'}); })()`)
			if !b.waitFor(`!!document.querySelector('[data-ways]')`) {
				t.Fatalf("no dialog: %s", b.eval(edText))
			}
			if got := b.eval(`document.querySelector('[data-ways]').innerText.replace(/\s+/g, ' ')`); !strings.Contains(got, "Part #b1.t1 in Errors") || !strings.Contains(got, "Used in Runbook") {
				t.Errorf("the dialog names the part and its uses: %q", got)
			}
			if got := b.eval(`document.querySelector('[data-ways] [data-act="waysgo"]').disabled + ' ' + document.querySelectorAll('[data-ways] [aria-pressed="true"]').length`); got != "true 0" {
				t.Errorf("nothing is chosen in advance: %q", got)
			}
			if got := b.eval(`(() => { const r = document.querySelector('[data-ways]').getBoundingClientRect(); return r.left >= 0 && r.right <= innerWidth && document.documentElement.scrollWidth <= innerWidth; })()`); got != "true" {
				t.Errorf("the dialog fits the screen")
			}
			b.shot("ways-" + name)
			if got := b.stored(errors); !strings.Contains(got, "<!-- part t1 -->") {
				t.Errorf("nothing changed while it asks: %q", got)
			}
			press(`[data-ways] [data-act="way"][data-way="everywhere"]`)
			if !b.waitFor(`document.querySelector('[data-ways] [data-way="everywhere"]').getAttribute('aria-pressed') === 'true' && !document.querySelector('[data-ways] [data-act="waysgo"]').disabled`) {
				t.Fatalf("the choice does not take")
			}
			press(`[data-ways] [data-act="waysgo"]`)
			if !b.waitFor(`!document.querySelector('[data-ways]') && document.querySelector('[data-edstatus]')?.textContent === 'Saved'`) {
				t.Fatalf("the change is not saved: %s", b.eval(`document.querySelector('[data-edstatus]')?.textContent`))
			}
			if got := b.stored(errors); got != "b1:Error 42: it. Free space." {
				t.Errorf("the server: %q", got)
			}
			// Runbook shows who removed it, and Restore brings it back.
			b.open(base + "/o/" + runbook)
			b.ready("Runbook")
			if !b.waitFor(`/Removed by .* from Errors/.test(document.querySelector('article.typeset')?.innerText || '')`) {
				t.Fatalf("no placeholder: %s", b.eval(`document.querySelector('article.typeset')?.innerText`))
			}
			press(`article.typeset [data-act="restoreremoved"]`)
			if !b.waitFor(`(document.querySelector('article.typeset [data-embed-part]')?.textContent || '') === 'the disk is full'`) {
				t.Errorf("restored, the part shows again: %s", b.eval(`document.querySelector('article.typeset')?.innerText`))
			}
			b.clean("ways " + name)
		})
	}
}

// Marking text in the editor makes it a part (ADR-22): its markers are hidden and kept whole, the cursor in it shows its
// boundary, its address pasted into a paragraph of another page embeds it there, through its formatting, and while
// reading the pointer, or a finger, shows its boundary and where it is used. Copy gives plain text with no tie.
func TestMarkingAPartInTheEditor(t *testing.T) {
	for _, phone := range []bool{false, true} {
		name := map[bool]string{false: "desktop", true: "phone"}[phone]
		t.Run(name, func(t *testing.T) {
			b, base := setupWithDatabase(t)
			if phone {
				b.phone(390, 844)
			}
			b.open(base + "/o/plan")
			b.ready("Network plan")
			errors := makePage(b, "Errors", "Error 42: the disk is **very full** now.")
			runbook := makePage(b, "Runbook", "When it says so, act.")
			b.openEditing(base, errors, "Errors")
			b.eval(`window.navigator.clipboard.writeText = async (t) => { window.clip = t; };`)
			// Mark "disk is **very" and choose Embed in the bar.
			b.eval(`(() => { const v = EDS[at(0).d].ed.view, t = v.state.doc.toString(), from = t.indexOf('disk'), to = t.indexOf('very') + 4; v.dispatch({selection: {anchor: from, head: to}}); v.focus(); })()`)
			if !b.waitFor(`!document.querySelector('[data-edsel]').hidden`) {
				t.Fatalf("the bar offers nothing for marked text")
			}
			if phone {
				b.tap(`[data-edbar] [data-act="edselembed"]`)
			} else {
				b.clickOn(`[data-edbar] [data-act="edselembed"]`, 1)
			}
			if !b.waitFor(`!!window.lastMarked && !!window.clip`) {
				t.Fatalf("no part was made: %s", b.eval(`(() => { const x = document.querySelector('[data-edbar] [data-act="edselembed"]'), r = x.getBoundingClientRect(), hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2); return JSON.stringify({toast: document.querySelector('.toast')?.textContent, r, hit: hit && hit.outerHTML.slice(0, 80), sel: EDS[at(0).d].ed.view.state.selection.main, w: innerWidth}); })()`))
			}
			ref := b.eval(`window.lastMarked.ref`)
			if got := b.eval(`window.clip`); !strings.HasSuffix(got, "/o/"+errors+"#b1.t1") {
				t.Errorf("the address on the clipboard: %q", got)
			}
			if got := b.stored(errors); got != "b1:Error 42: the <!-- part t1 -->disk is **very<!-- /part t1 --> full** now." {
				t.Errorf("the server: %q", got)
			}
			if got := b.eval(`document.querySelector('.cm-content').innerText`); strings.Contains(got, "<!--") {
				t.Errorf("the markers are hidden: %q", got)
			}
			// A click into the text of the part, as a hand puts the cursor there.
			at := b.eval(`(() => { const w = document.createTreeWalker(document.querySelector('.cm-content'), NodeFilter.SHOW_TEXT); let n; while ((n = w.nextNode())) { const k = n.data.indexOf('disk'); if (k >= 0) { const r = document.createRange(); r.setStart(n, k + 1); r.setEnd(n, k + 2); const x = r.getBoundingClientRect(); return x.left + ',' + (x.top + x.height / 2); } } return ''; })()`)
			var x, y float64
			if _, err := fmt.Sscanf(at, "%f,%f", &x, &y); err != nil {
				t.Fatalf("no text of the part on the screen: %q", at)
			}
			if phone {
				b.pressAt(x, y, 60*time.Millisecond)
			} else {
				b.mouse("mouseMoved", x, y, 0, 0)
				b.mouse("mousePressed", x, y, 1, 0)
				b.mouse("mouseReleased", x, y, 1, 0)
			}
			if !b.waitFor(`!!document.querySelector('.cm-editor [data-part][data-framed]') && document.querySelector('.cm-editor [data-part][data-framed]')?.dataset.part === 'Not used elsewhere'`) {
				t.Errorf("the cursor in the part shows its boundary: %s", b.eval(`JSON.stringify({label: document.querySelector('.cm-editor [data-part][data-framed]')?.dataset.part, mark: !!document.querySelector('.cm-editor [data-part]'), focus: EDS[at(0).d].ed.view.hasFocus, head: EDS[at(0).d].ed.view.state.selection.main.head, text: EDS[at(0).d].ed.text()})`))
			}
			// A marker does not go on its own: Backspace right after the part leaves it whole.
			b.eval(`(() => { const v = EDS[at(0).d].ed.view, t = v.state.doc.toString(), p = t.indexOf('<!-- /part t1 -->') + '<!-- /part t1 -->'.length; v.dispatch({selection: {anchor: p}}); v.focus(); })()`)
			b.press("Backspace", "Backspace", 8, 0)
			if !b.edWait(`t.includes('<!-- part t1 -->disk is **ver<!-- /part t1 -->')`) {
				t.Errorf("the part keeps both markers, and Backspace takes its last character: %q", b.eval(edText))
			}
			b.press("Backspace", "Backspace", 8, 0)
			if !b.edWait(`t.includes('<!-- part t1 -->disk is **ve<!-- /part t1 -->')`) {
				t.Errorf("and the next one: %q", b.eval(edText))
			}
			// Copy is plain text with the formatting closed.
			b.eval(`(() => { const v = EDS[at(0).d].ed.view, t = v.state.doc.toString(), from = t.indexOf('ve'), to = t.indexOf('now'); v.dispatch({selection: {anchor: from, head: to}}); v.focus(); window.clip = null; })()`)
			b.waitFor(`!document.querySelector('[data-edsel]').hidden`)
			b.clickOn(`[data-edbar] [data-act="edselcopy"]`, 1)
			if !b.waitFor(`window.clip === '**ve full**'`) {
				t.Errorf("the copy: %q", b.eval(`window.clip`))
			}
			// Pasted into a paragraph of Runbook, the address embeds the part there.
			b.openEditing(base, runbook, "Runbook")
			b.eval(`(() => { const v = EDS[at(0).d].ed.view, t = v.state.doc.toString(), p = t.indexOf('so') + 2; v.dispatch({selection: {anchor: p}}); v.focus(); })()`)
			b.eval(`(() => { const dt = new DataTransfer(); dt.setData('text/plain', location.origin + '/o/` + errors + `#b1.t1');
				document.querySelector('.cm-content').dispatchEvent(new ClipboardEvent('paste', {clipboardData: dt, bubbles: true, cancelable: true})); })()`)
			if !b.edWait(`t.includes('When it says so:embed{ref="` + ref + `"}, act.')`) {
				t.Fatalf("pasted inside a paragraph: %q", b.eval(edText))
			}
			b.clickOn(`[data-win="0"] [data-mode]`, 1) // back to reading
			if !b.waitFor(`(document.querySelector('article.typeset [data-embed-part]')?.innerHTML || '') === 'disk is <strong>ve</strong>'`) {
				t.Errorf("Runbook reads the part through its formatting: %s", b.eval(`document.querySelector('article.typeset')?.innerHTML`))
			}
			// Reading Errors, the pointer on the part shows its boundary and where it is used.
			b.open(base + "/o/" + errors)
			b.ready("Errors")
			r := b.rect(`article.typeset [data-block="b1"] strong`)
			if phone {
				b.pressAt(r.X-30, r.Y, 60*time.Millisecond)
			} else {
				b.mouse("mouseMoved", r.X-30, r.Y, 0, 0)
			}
			if !b.waitFor(`document.querySelectorAll('#root > [data-part-id]').length > 0 && document.querySelector('#root > [data-part-id]')?.dataset.part === 'Used in 1 place'`) {
				t.Errorf("the boundary while reading: %s", b.eval(`document.querySelector('#root > [data-part-id]')?.dataset.part`))
			}
			// The part runs into the bold text on one line: one frame round it, one name, not a frame for each piece.
			if got := b.eval(`(() => { const x = [...document.querySelectorAll('#root > [data-part-id]')], s = document.querySelector('article.typeset [data-block="b1"] strong').getBoundingClientRect(); return x.length + ' ' + (x.length === 1 && x[0].getBoundingClientRect().right >= s.left + 1); })()`); got != "1 true" {
				t.Errorf("one frame round the part on its line: %q", got)
			}
			b.shot("part-boundary-" + name)
			b.clean("marking " + name)
		})
	}
}

// The facts panel of a page names what it embeds (M-3): a value by its name and the page it lives in, a block by its kind.
func TestTheFactsNameWhatAPageEmbeds(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	rates := makePage(b, "Rates", `VAT is <!-- part t1 name="vat" -->19 %<!-- /part t1 -->.`, "Shipping is free.")
	invoice := makePage(b, "Invoice", "Net plus :embed{ref=\""+rates+"#b1.t1\"}.", "::embed{ref=\""+rates+"#b2\"}")
	b.open(base + "/o/" + invoice)
	b.ready("Invoice")
	b.clickOn(`[data-act="gear"]`, 1)
	uses := `[...document.querySelectorAll('.facts li')].map(li => li.innerText.replace(/\s+/g, ' ').trim()).join(' | ')`
	if !b.waitFor(uses + `.includes('Value vat Rates #b1.t1') && ` + uses + `.includes('Block Rates #b2')`) {
		t.Fatalf("the facts do not name what Invoice embeds: %s", b.eval(uses))
	}
	b.shot("facts-embeds")
}

// The Resources tab lists the real values and snippets (M-3): a value with its text, its page and how often it is used,
// and opening it names the page it lives in. Databases and external sources are the mock-up still, and say so.
func TestTheResourcesTabIsReal(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	rates := makePage(b, "Rates", `VAT is <!-- part t1 name="vat" -->19 %<!-- /part t1 -->.`, "Shipping is free.")
	makePage(b, "Invoice", "Net plus :embed{ref=\""+rates+"#b1.t1\"}.", "::embed{ref=\""+rates+"#b2\"}")
	b.open(base + "/")
	b.ready("")
	b.clickOn(`[aria-label=Nav1] [data-act="view"][data-id="resources"]`, 1)
	if !b.waitFor(`[...document.querySelectorAll('[aria-label=Nav3] [data-act="open"]')].some(x => x.textContent.trim() === 'vat')`) {
		t.Fatalf("the value is not in the list: %s", b.eval(`document.querySelector('[aria-label=Nav3]')?.innerText`))
	}
	b.clickOn(probe(b, `[aria-label=Nav3] [data-act="open"]`, "vat"), 1)
	row := `[...document.querySelectorAll('article.typeset tr')].map(r => r.innerText.replace(/\s+/g, ' ').trim()).join(' | ')`
	if !b.waitFor(row + `.includes('Value 19 %') && ` + row + `.includes('Used in 1 place') && document.querySelector('article.typeset a[href^="/o/` + rates + `#b1.t1"]')?.textContent === 'Rates'`) {
		t.Fatalf("the value does not show: %s", b.eval(row))
	}
	if got := b.eval(`[...document.querySelectorAll('[aria-label=Nav2] [data-act="sec"]')].map(x => x.textContent.trim()).join(', ')`); !strings.Contains(got, "Databases and views (demo)") || strings.Contains(got, "Values (demo)") {
		t.Errorf("what is real and what is the mock-up: %s", got)
	}
	b.clickOn(`[data-act="sec"][data-id="snippets"]`, 1)
	if !b.waitFor(`[...document.querySelectorAll('[aria-label=Nav3] [data-act="open"]')].some(x => x.textContent.trim() === 'Shipping is free.')`) {
		t.Fatalf("the snippet is not in the list: %s", b.eval(`document.querySelector('[aria-label=Nav3]')?.innerText`))
	}
	b.shot("resources-real")
}

// A block moves into another page from its menu (M-3), while reading and while editing: it goes to the end of the other
// page, leaves its own, and its old address leads to the new place.
func TestMovingABlockToAnotherPage(t *testing.T) {
	for _, editing := range []bool{false, true} {
		name := "reading"
		if editing {
			name = "editing"
		}
		t.Run(name, func(t *testing.T) {
			b, base := setupWithDatabase(t)
			b.open(base + "/o/plan")
			b.ready("Network plan")
			target := makePage(b, "Target page", "First.")
			source := makePage(b, "Source page", "Stays here.", "Goes away <!-- part t1 -->with its part<!-- /part t1 -->.")
			user := makePage(b, "User page", "Uses :embed{ref=\""+source+"#b2.t1\"}.")
			handle := `article.typeset [data-block="b2"] > .editor-handle > button`
			if editing {
				b.openEditing(base, source, "Source page")
				handle = `.cm-ud-marker[data-edb="b2"] .editor-handle > button`
			} else {
				b.open(base + "/o/" + source)
				b.ready("Source page")
			}
			if !b.waitFor(`!!document.querySelector('` + handle + `')`) {
				t.Fatal("no handle on the block")
			}
			b.clickOn(handle, 1)
			b.clickOn(`.editor-handle [role=menu] [data-act="bmoveto"]`, 1)
			if !b.waitFor(`[...document.querySelectorAll('#bmovesel option')].some(o => o.textContent === 'Target page')`) {
				t.Fatalf("the menu offers no page: %s", b.eval(`document.querySelector('.editor-handle [role=menu]')?.innerHTML.slice(0, 400)`))
			}
			// A redraw keeps the choice.
			b.eval(`(() => { const s = document.getElementById('bmovesel'); s.value = [...s.options].find(o => o.textContent === 'Target page').value; s.focus(); })()`)
			b.eval(`render()`)
			b.clickOn(`.editor-handle [role=menu] [data-act="bmovego"]`, 1)
			md := func(id string) string {
				return b.eval(`(() => { const x = new XMLHttpRequest(); x.open('GET', '/api/doc/` + id + `', false); x.send(); return JSON.parse(x.responseText).blocks.map(b => b.id + ':' + b.md).join(' | '); })()`)
			}
			if !b.waitFor(`(() => { const x = new XMLHttpRequest(); x.open('GET', '/api/doc/` + target + `', false); x.send(); return JSON.parse(x.responseText).blocks.length === 2; })()`) {
				t.Fatalf("the block did not move: target %s, source %s", md(target), md(source))
			}
			if got := md(target); got != "b1:First. | b2:Goes away <!-- part t1 -->with its part<!-- /part t1 -->." {
				t.Errorf("Target page: %q", got)
			}
			if got := md(source); got != "b1:Stays here." {
				t.Errorf("Source page: %q", got)
			}
			if got := md(user); got != `b1:Uses :embed{ref="`+target+`#b2.t1"}.` {
				t.Errorf("User page follows: %q", got)
			}
			if !b.waitFor(`![...document.querySelectorAll('article.typeset')].some(a => a.textContent.includes('Goes away')) && !Object.values(EDS).some(e => e.ed.text().includes('Goes away'))`) {
				t.Errorf("the page still shows the block: %s", b.eval(`document.querySelector('article.typeset')?.textContent`))
			}
			// Nothing saves it back later: the editor made again saves nothing of the old page.
			time.Sleep(2 * time.Second)
			if got := md(source); got != "b1:Stays here." {
				t.Errorf("after a while, Source page: %q", got)
			}
		})
	}
}
