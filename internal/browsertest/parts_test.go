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
			if !b.waitFor(`!!document.querySelector('.wayspop')`) {
				t.Fatalf("no dialog: %s", b.eval(edText))
			}
			if got := b.eval(`document.querySelector('.wayspop').innerText.replace(/\s+/g, ' ')`); !strings.Contains(got, "Part #b1.t1 in Errors") || !strings.Contains(got, "Used in Runbook") {
				t.Errorf("the dialog names the part and its uses: %q", got)
			}
			if got := b.eval(`document.querySelector('.wayspop [data-act="waysgo"]').disabled + ' ' + document.querySelectorAll('.wayspop [aria-pressed="true"]').length`); got != "true 0" {
				t.Errorf("nothing is chosen in advance: %q", got)
			}
			if got := b.eval(`(() => { const r = document.querySelector('.wayspop').getBoundingClientRect(); return r.left >= 0 && r.right <= innerWidth && document.documentElement.scrollWidth <= innerWidth; })()`); got != "true" {
				t.Errorf("the dialog fits the screen")
			}
			b.shot("ways-" + name)
			if got := b.stored(errors); !strings.Contains(got, "<!-- part t1 -->") {
				t.Errorf("nothing changed while it asks: %q", got)
			}
			press(`.wayspop [data-act="way"][data-way="everywhere"]`)
			if !b.waitFor(`document.querySelector('.wayspop [data-way="everywhere"]').getAttribute('aria-pressed') === 'true' && !document.querySelector('.wayspop [data-act="waysgo"]').disabled`) {
				t.Fatalf("the choice does not take")
			}
			press(`.wayspop [data-act="waysgo"]`)
			if !b.waitFor(`!document.querySelector('.wayspop') && document.querySelector('[data-edstatus]')?.textContent === 'Saved'`) {
				t.Fatalf("the change is not saved: %s", b.eval(`document.querySelector('[data-edstatus]')?.textContent`))
			}
			if got := b.stored(errors); got != "b1:Error 42: it. Free space." {
				t.Errorf("the server: %q", got)
			}
			// Runbook shows who removed it, and Restore brings it back.
			b.open(base + "/o/" + runbook)
			b.ready("Runbook")
			if !b.waitFor(`/Removed by .* from Errors/.test(document.querySelector('article.doc')?.innerText || '')`) {
				t.Fatalf("no placeholder: %s", b.eval(`document.querySelector('article.doc')?.innerText`))
			}
			press(`article.doc [data-act="restoreremoved"]`)
			if !b.waitFor(`(document.querySelector('article.doc [data-embed-part]')?.textContent || '') === 'the disk is full'`) {
				t.Errorf("restored, the part shows again: %s", b.eval(`document.querySelector('article.doc')?.innerText`))
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
				b.tap(`.edbar [data-act="edselembed"]`)
			} else {
				b.clickOn(`.edbar [data-act="edselembed"]`, 1)
			}
			if !b.waitFor(`!!window.lastMarked && !!window.clip`) {
				t.Fatalf("no part was made: %s", b.eval(`(() => { const x = document.querySelector('.edbar [data-act="edselembed"]'), r = x.getBoundingClientRect(), hit = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2); return JSON.stringify({toast: document.querySelector('.toast')?.textContent, r, hit: hit && hit.outerHTML.slice(0, 80), sel: EDS[at(0).d].ed.view.state.selection.main, w: innerWidth}); })()`))
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
			if !b.waitFor(`!!document.querySelector('.cm-editor .cm-ud-part') && document.querySelector('.cm-ud-partlabel')?.textContent === 'Not used elsewhere'`) {
				t.Errorf("the cursor in the part shows its boundary: %s", b.eval(`JSON.stringify({label: document.querySelector('.cm-ud-partlabel')?.textContent, mark: !!document.querySelector('.cm-ud-part'), focus: EDS[at(0).d].ed.view.hasFocus, head: EDS[at(0).d].ed.view.state.selection.main.head, text: EDS[at(0).d].ed.text()})`))
			}
			// A marker does not go on its own: Backspace right after the part leaves it whole.
			b.eval(`(() => { const v = EDS[at(0).d].ed.view, t = v.state.doc.toString(), p = t.indexOf('<!-- /part t1 -->') + '<!-- /part t1 -->'.length; v.dispatch({selection: {anchor: p}}); v.focus(); })()`)
			b.press("Backspace", "Backspace", 8, 0)
			if !b.edWait(`t.includes('<!-- part t1 -->') && t.includes('<!-- /part t1 -->')`) {
				t.Errorf("the part keeps both markers: %q", b.eval(edText))
			}
			// Copy is plain text with the formatting closed.
			b.eval(`(() => { const v = EDS[at(0).d].ed.view, t = v.state.doc.toString(), from = t.indexOf('very'), to = t.indexOf('now'); v.dispatch({selection: {anchor: from, head: to}}); v.focus(); window.clip = null; })()`)
			b.waitFor(`!document.querySelector('[data-edsel]').hidden`)
			b.clickOn(`.edbar [data-act="edselcopy"]`, 1)
			if !b.waitFor(`window.clip === '**very full**'`) {
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
			b.clickOn(`[data-win="0"] .dstatus`, 1) // back to reading
			if !b.waitFor(`(document.querySelector('article.doc [data-embed-part]')?.innerHTML || '') === 'disk is <strong>very</strong>'`) {
				t.Errorf("Runbook reads the part through its formatting: %s", b.eval(`document.querySelector('article.doc')?.innerHTML`))
			}
			// Reading Errors, the pointer on the part shows its boundary and where it is used.
			b.open(base + "/o/" + errors)
			b.ready("Errors")
			r := b.rect(`article.doc [data-b="b1"] strong`)
			if phone {
				b.pressAt(r.X-30, r.Y, 60*time.Millisecond)
			} else {
				b.mouse("mouseMoved", r.X-30, r.Y, 0, 0)
			}
			if !b.waitFor(`document.querySelectorAll('.partedge').length > 0 && document.querySelector('.partlabel')?.textContent === 'Used in 1 place'`) {
				t.Errorf("the boundary while reading: %s", b.eval(`document.querySelector('.partlabel')?.textContent`))
			}
			b.shot("part-boundary-" + name)
			b.clean("marking " + name)
		})
	}
}
