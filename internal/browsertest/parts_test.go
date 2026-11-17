//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"strings"
	"testing"
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
