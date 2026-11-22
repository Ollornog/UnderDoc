//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

// The gear of a page of the database keeps its width and where its contents show (ADR-16, M-8): chosen with the pointer,
// stored on the server, and still so after a reload. The contents of such a page are its blocks that start with a heading.
func TestTheGearKeepsWidthAndContents(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := makePage(b, "Viewed", "## Switches", "Two of them.", "## Routers", "One.")
	b.open(base + "/o/" + page)
	b.ready("Viewed")
	b.watch()
	b.clickOn(`[data-win="0"] [data-act="gear"][aria-label="Document settings"]`, 1)
	if !b.waitFor(`!!document.querySelector('.fdrawer [data-act="setw"]')`) {
		t.Fatal("the gear did not open")
	}
	if got := b.eval(`[...document.querySelectorAll('.fdrawer [data-act="settoc"]')].map(x => x.textContent).join(',') + ' | ' + document.querySelector('.fdrawer .hint').textContent`); got != "marker,side,hidden | Both are kept with this document, for everyone who opens it." {
		t.Errorf("the gear of a page of the database: %q", got)
	}
	// The second choice is held on its way to the server: the page must not be read again before it is stored, which
	// under load made this test red (2026-11-17).
	b.call("Fetch.enable", map[string]any{"patterns": []any{map[string]any{"urlPattern": "*/view", "requestStage": "Request"}}}, nil)
	b.clickOn(`.fdrawer [data-act="setw"][data-id="full"]`, 1)
	var paused struct{ RequestID string }
	_ = json.Unmarshal(b.waitEvent("Fetch.requestPaused"), &paused)
	b.call("Fetch.continueRequest", map[string]any{"requestId": paused.RequestID}, nil)
	b.clickOn(`.fdrawer [data-act="settoc"][data-id="side"]`, 1)
	_ = json.Unmarshal(b.waitEvent("Fetch.requestPaused"), &paused)
	if !b.waitFor(`window.written.filter(w => w.endsWith('/view')).length === 1`) {
		t.Errorf("the first choice is stored: %s", b.eval(`window.written.join(', ')`))
	}
	time.Sleep(300 * time.Millisecond)
	b.call("Fetch.continueRequest", map[string]any{"requestId": paused.RequestID}, nil)
	b.call("Fetch.disable", map[string]any{}, nil)
	if !b.waitFor(`document.querySelector('article.typeset').dataset.width === 'full' && [...document.querySelectorAll('nav.toc a')].map(x => x.textContent).join(',') === 'Switches,Routers'`) {
		t.Errorf("full width and the contents at the side: %s", b.eval(`document.querySelector('article.typeset').dataset.width + ' ' + document.querySelector('nav.toc')?.textContent`))
	}
	b.shot("gear-view")
	if !b.waitFor(`window.written.filter(w => w.endsWith('/view')).length === 2`) {
		t.Errorf("each choice is stored: %s", b.eval(`window.written.join(', ')`))
	}
	b.open(base + "/o/" + page)
	b.ready("Viewed")
	if !b.waitFor(`document.querySelector('article.typeset')?.dataset.width === 'full' && !!document.querySelector('nav.toc')`) {
		t.Errorf("after a reload the page keeps its view: %s", b.eval(`document.querySelector('article.typeset')?.dataset.width`))
	}
	b.clean("gear view")
}

// A block is aligned left, center or right from the menu of its handle (ADR-16), in reading and in the editor; the
// alignment is stored with the block, not in its Markdown, and makes no version.
func TestBlocksAreAligned(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := makePage(b, "Aligned", "Title", "Text")
	versions := b.versions(page)
	b.open(base + "/o/" + page)
	b.ready("Aligned")
	b.watch()
	b.clickOn(`article.typeset [data-b="b1"] > .bhandle`, 1)
	b.clickOn(`.bmenu [data-act="balign"][data-k="center"]`, 1)
	if !b.waitFor(`getComputedStyle(document.querySelector('article.typeset [data-b="b1"] p')).textAlign === 'center'`) {
		t.Errorf("reading shows the block centered: %s", b.eval(`document.querySelector('article.typeset [data-b="b1"]')?.outerHTML`))
	}
	b.clickOn(`[data-win="0"] [data-mode]`, 1)
	if !b.waitFor(`!!document.querySelector('.cm-editor .cm-content')`) {
		t.Fatal("no editor")
	}
	line := func(text string) string {
		return `getComputedStyle([...document.querySelectorAll('.cm-editor .cm-line')].find(l => l.textContent.trim() === ` + strconv.Quote(text) + `)).textAlign`
	}
	if got := b.eval(line("Title") + ` + ' ' + ` + line("Text")); got != "center start" {
		t.Errorf("the editor shows the alignment: %q", got)
	}
	b.lineWith("Text")
	b.clickOn(`.cm-ud-marker[data-edb="b2"] .bhandle`, 1)
	b.clickOn(`.cm-ud-marker[data-edb="b2"] .bmenu [data-act="balign"][data-k="right"]`, 1)
	if !b.waitFor(line("Text") + ` === 'right'`) {
		t.Errorf("aligned right in the editor: %q", b.eval(line("Text")))
	}
	b.shot("blocks-aligned")
	b.clickOn(`[data-win="0"] [data-mode]`, 1)
	b.open(base + "/o/" + page)
	b.ready("Aligned")
	if !b.waitFor(`getComputedStyle(document.querySelector('article.typeset [data-b="b2"] p')).textAlign === 'right' && getComputedStyle(document.querySelector('article.typeset [data-b="b1"] p')).textAlign === 'center'`) {
		t.Errorf("after a reload: %s", b.eval(`[...document.querySelectorAll('article.typeset [data-b]')].map(x => x.className).join(',')`))
	}
	if got := b.stored(page); got != "b1:Title | b2:Text" {
		t.Errorf("the Markdown stays as written: %q", got)
	}
	if got := b.versions(page); got != versions {
		t.Errorf("aligning made versions: %d, before %d", got, versions)
	}
	b.clean("aligned")
}
