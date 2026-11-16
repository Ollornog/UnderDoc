//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"strconv"
	"testing"
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
	b.clickOn(`.fdrawer [data-act="setw"][data-id="full"]`, 1)
	b.clickOn(`.fdrawer [data-act="settoc"][data-id="side"]`, 1)
	if !b.waitFor(`document.querySelector('article.doc').classList.contains('full') && [...document.querySelectorAll('nav.toc button')].map(x => x.textContent).join(',') === 'Switches,Routers'`) {
		t.Errorf("full width and the contents at the side: %s", b.eval(`document.querySelector('article.doc').className + ' ' + document.querySelector('nav.toc')?.textContent`))
	}
	b.shot("gear-view")
	if !b.waitFor(`window.writes.filter(w => w.endsWith('/view')).length === 2`) {
		t.Errorf("each choice is stored: %s", b.eval(`window.writes.join(', ')`))
	}
	b.open(base + "/o/" + page)
	b.ready("Viewed")
	if !b.waitFor(`document.querySelector('article.doc')?.classList.contains('full') && !!document.querySelector('nav.toc')`) {
		t.Errorf("after a reload the page keeps its view: %s", b.eval(`document.querySelector('article.doc')?.className`))
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
	b.clickOn(`article.doc [data-b="b1"] > .bhandle`, 1)
	b.clickOn(`.bmenu [data-act="balign"][data-k="center"]`, 1)
	if !b.waitFor(`getComputedStyle(document.querySelector('article.doc [data-b="b1"] p')).textAlign === 'center'`) {
		t.Errorf("reading shows the block centered: %s", b.eval(`document.querySelector('article.doc [data-b="b1"]')?.outerHTML`))
	}
	b.clickOn(`[data-win="0"] .dstatus`, 1)
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
	b.clickOn(`[data-win="0"] .dstatus`, 1)
	b.open(base + "/o/" + page)
	b.ready("Aligned")
	if !b.waitFor(`getComputedStyle(document.querySelector('article.doc [data-b="b2"] p')).textAlign === 'right' && getComputedStyle(document.querySelector('article.doc [data-b="b1"] p')).textAlign === 'center'`) {
		t.Errorf("after a reload: %s", b.eval(`[...document.querySelectorAll('article.doc [data-b]')].map(x => x.className).join(',')`))
	}
	if got := b.stored(page); got != "b1:Title | b2:Text" {
		t.Errorf("the Markdown stays as written: %q", got)
	}
	if got := b.versions(page); got != versions {
		t.Errorf("aligning made versions: %d, before %d", got, versions)
	}
	b.clean("aligned")
}
