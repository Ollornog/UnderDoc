//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import "testing"

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
