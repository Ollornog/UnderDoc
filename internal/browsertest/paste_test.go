//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"encoding/json"
	"strconv"
	"testing"
)

// clipboard is what a copy put on the clipboard: its text and the origin beside it (T-41).
type clipboard struct {
	Text   string `json:"text"`
	Origin string `json:"origin"`
}

// copyFrom marks the text of an element and copies it as a hand with Ctrl+C would, and reads the clipboard.
func copyFrom(b *browser, selector string) clipboard {
	b.t.Helper()
	raw := b.eval(`(() => { const el = document.querySelector(` + strconv.Quote(selector) + `), r = document.createRange();
		r.selectNodeContents(el); getSelection().removeAllRanges(); getSelection().addRange(r);
		const dt = new DataTransfer();
		el.dispatchEvent(new ClipboardEvent('copy', {clipboardData: dt, bubbles: true, cancelable: true}));
		return JSON.stringify({text: dt.getData('text/plain'), origin: dt.getData('application/x-underdoc-origin')}); })()`)
	var c clipboard
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		b.t.Fatalf("%s: %s", selector, raw)
	}
	return c
}

// pasteAtEnd puts the cursor at the end of the open editor and pastes what a clipboard holds.
func pasteAtEnd(b *browser, c clipboard) {
	b.t.Helper()
	data, _ := json.Marshal(c)
	b.eval(`(() => { const c = ` + string(data) + `, v = EDS[at(0).d].ed.view;
		v.dispatch({selection: {anchor: v.state.doc.length}});
		const dt = new DataTransfer(); dt.setData('text/plain', c.text); if (c.origin) dt.setData('application/x-underdoc-origin', c.origin);
		document.querySelector('.cm-content').dispatchEvent(new ClipboardEvent('paste', {clipboardData: dt, bubbles: true, cancelable: true})); })()`)
}

// The origin of the newest version of a page, through the API.
func newestOrigin(b *browser, page string) string {
	b.t.Helper()
	b.eval(`window.origin_ = null; fetch('/api/pages/` + page + `/versions').then(r => r.json()).then(v => { window.origin_ = v[0].origin || '-'; })`)
	if !b.waitFor(`window.origin_ !== null`) {
		b.t.Fatal("the versions do not come")
	}
	return b.eval(`window.origin_`)
}

// A text copied from a page and pasted into another notes where it came from in the version of the block it lands in
// (T-41): from the reading view and from the editor, the page and block with its version, shown as a link in the
// versions. Pasting text from outside UnderDoc notes nothing, and the copy stays a copy.
func TestAPastedTextNotesWhereItCameFrom(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	source := makePage(b, "Source", "The core switch sits in rack B.", "The router sits in rack A.")
	target := makePage(b, "Target", "Notes.")

	// Copying while reading: the page and its block go beside the text.
	b.open(base + "/o/" + source)
	b.ready("Source")
	read := copyFrom(b, `[data-block="b1"][data-page] p`)
	if read.Text != "The core switch sits in rack B." || read.Origin == "" {
		t.Fatalf("copying while reading: %+v", read)
	}
	var origin struct {
		Page   string
		Blocks []string
	}
	if err := json.Unmarshal([]byte(read.Origin), &origin); err != nil || origin.Page != source || len(origin.Blocks) != 1 || origin.Blocks[0] != "b1" {
		t.Fatalf("the origin of a copy while reading: %s", read.Origin)
	}
	// Copying in the editor.
	b.clickOn(`[data-act="edit"]`, 1)
	if !b.waitFor(`document.activeElement?.classList.contains('cm-content')`) {
		t.Fatal("no editor in Source")
	}
	// The reading view knows the format of the origin without the editor, which loads only when the page is idle: a copy
	// while reading before that carried none (PR #205). Both name the same.
	if got := b.eval(`String(UnderDocEditor.ORIGIN === CLIP_ORIGIN)`); got != "true" {
		t.Errorf("the reading view and the editor name the origin differently: %s", b.eval(`UnderDocEditor.ORIGIN + ' ' + CLIP_ORIGIN`))
	}
	b.eval(`(() => { const v = EDS[at(0).d].ed.view, t = v.state.doc.toString(), from = t.indexOf('The router');
		v.dispatch({selection: {anchor: from, head: from + 'The router sits in rack A.'.length}}); })()`)
	edited := b.eval(`(() => { const dt = new DataTransfer();
		document.querySelector('.cm-content').dispatchEvent(new ClipboardEvent('copy', {clipboardData: dt, bubbles: true, cancelable: true}));
		return JSON.stringify({text: dt.getData('text/plain'), origin: dt.getData('application/x-underdoc-origin')}); })()`)
	var fromEditor clipboard
	_ = json.Unmarshal([]byte(edited), &fromEditor)
	if fromEditor.Text != "The router sits in rack A." || fromEditor.Origin == "" {
		t.Fatalf("copying in the editor: %s", edited)
	}
	b.clickOn(`[data-act="edit"]`, 1)

	// Pasting both into Target notes both in its version.
	b.open(base + "/o/" + target)
	b.ready("Target")
	b.clickOn(`[data-act="edit"]`, 1)
	if !b.waitFor(`document.activeElement?.classList.contains('cm-content')`) {
		t.Fatal("no editor in Target")
	}
	// An origin beside another text names nothing.
	pasteAtEnd(b, clipboard{Text: " Other text.", Origin: read.Origin})
	b.clickOn(`[data-act="edit"]`, 1)
	b.settled()
	if got := newestOrigin(b, target); got != "-" {
		t.Errorf("an origin beside another text notes %q", got)
	}
	b.clickOn(`[data-act="edit"]`, 1)
	if !b.waitFor(`document.activeElement?.classList.contains('cm-content')`) {
		t.Fatal("no editor in Target")
	}
	pasteAtEnd(b, read)
	pasteAtEnd(b, fromEditor)
	b.clickOn(`[data-act="edit"]`, 1) // leaving the editor saves it
	b.settled()
	if got, want := newestOrigin(b, target), "copy of "+source+"#b1, version 1; "+source+"#b2, version 1"; got != want {
		t.Fatalf("the origin in the version:\n%s\nwant\n%s", got, want)
	}
	b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
	if !b.waitFor(`[...document.querySelectorAll('aside[data-drawer] [data-origin] a')].map(a => a.textContent).join(', ') === 'Source #b1, Source #b2'`) {
		t.Errorf("the versions show where the text came from: %s", b.eval(`document.querySelector('aside[data-drawer]')?.innerText`))
	}
	b.shot("paste-origin")

	// Text from outside UnderDoc notes nothing.
	b.clickOn(`[data-act="edit"]`, 1)
	if !b.waitFor(`document.activeElement?.classList.contains('cm-content')`) {
		t.Fatal("no editor in Target")
	}
	pasteAtEnd(b, clipboard{Text: " From a mail."})
	b.clickOn(`[data-act="edit"]`, 1)
	b.settled()
	if got := newestOrigin(b, target); got != "-" {
		t.Errorf("a paste from outside notes %q", got)
	}
	b.eval(`window.used_ = null; fetch('/api/doc/` + source + `').then(r => r.json()).then(d => { window.used_ = d.blocks.map(x => (x.usedIn || []).length).join(','); })`)
	if !b.waitFor(`window.used_ !== null`) || b.eval(`window.used_`) != "0,0" {
		t.Errorf("the copy ties the source to it: %s", b.eval(`window.used_`))
	}
}
