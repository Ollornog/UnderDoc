//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"strings"
	"testing"
)

// signInAs sets the headers of the gate for another person, on every request of the browser.
func (b *browser) signInAs(login, name string) {
	b.t.Helper()
	b.call("Network.setExtraHTTPHeaders", map[string]any{"headers": map[string]string{"Remote-User": login, "Remote-Name": name}}, nil)
}

// drawer returns the text of the comments drawer in one line, or "none".
const drawer = `(document.querySelector('aside[data-drawer][aria-label="Comments"]')?.innerText.replace(/\s+/g, ' ').trim() || 'none')`

// Comments on a page of the database, on the whole page and on its blocks: written, answered, resolved, changed and
// deleted in the drawer, counted at the head and beside each block, kept by the server, opened from the bar while editing
// and from commentOn; a block that is gone keeps its comments; the text stays text; read only shows them without a field.
func TestCommentsOnAPageAndItsBlocks(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	if got := b.eval(`String(cmtCount(D.plan) === 3 && cmtHtml({d: 'plan', cb: 'h3'}, 0).includes('Is the radio link backed up'))`); got != "true" {
		t.Errorf("the made-up document keeps its made-up comments: %s", got)
	}
	b.eval(`window.made = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		const team = await post('/api/collections', {title: 'Team'});
		const page = await post('/api/pages', {title: 'Notes', parent: team.id});
		const b1 = await post('/api/pages/' + page.id + '/blocks', {md: '## Switches', after: ''});
		await post('/api/pages/' + page.id + '/blocks', {md: 'Two in the rack.', after: b1.id});
		window.made = [team.id, page.id];
	})()`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the page for the test was not made")
	}
	team, notes := b.eval(`window.made[0]`), b.eval(`window.made[1]`)
	b.open(base + "/o/" + notes)
	b.ready("Notes")
	if got := b.eval(`[...document.querySelectorAll('article.typeset [data-comment]')].map(x => x.dataset.id + ':' + (x.dataset.count || '')).join(' ')`); got != "b1: b2:" {
		t.Errorf("a speech bubble beside each block, none counted yet: %q", got)
	}

	// The head opens the comments on the whole page.
	b.clickOn(`[data-win="0"] [role=toolbar] [data-act="cmt"]`, 1)
	if !b.waitFor(`document.activeElement?.dataset?.addcmt === '0'`) {
		t.Fatalf("the drawer opens with its field: %s", b.eval(drawer))
	}
	if got := b.eval(drawer); !strings.Contains(strings.ToLower(got), "the whole page") || !strings.Contains(got, "No comments on this yet.") {
		t.Errorf("the whole page, nothing yet: %q", got)
	}
	b.typeText(`Looks good. <img src=x onerror="window.hacked=1">`)
	b.press("Enter", "Enter", 13, 0)
	if !b.waitFor(`!!document.querySelector('aside[data-drawer] [data-cmt]') && document.querySelector('[data-win="0"] [role=toolbar] [data-act="cmt"]')?.dataset.count === '1'`) {
		t.Fatalf("the comment shows and the head counts it: %s", b.eval(drawer))
	}
	if got := b.eval(`document.querySelector('aside[data-drawer] [data-cmt] > p').textContent + ' | ' + !!document.querySelector('aside[data-drawer] [data-cmt] img') + ' ' + !!window.hacked + ' | ' + document.querySelector('aside[data-drawer] [data-cmt] > header > b').textContent`); got != `Looks good. <img src=x onerror="window.hacked=1"> | false false | Anna Berg` {
		t.Errorf("the text stays text: %q", got)
	}

	// A speech bubble beside a block opens the comments on that block.
	b.clickOn(`[data-win="0"] aside[data-drawer] [data-act="cmt"]`, 1)
	b.clickOn(`article.typeset [data-comment][data-id="b2"]`, 1)
	if !b.waitFor(`document.querySelector('aside[data-drawer] [data-cfor] small')?.textContent === '#b2' && document.activeElement?.dataset?.addcmt === '0'`) {
		t.Fatalf("the drawer on block b2: %s", b.eval(drawer))
	}
	if got := b.eval(`document.querySelector('aside[data-drawer] [data-cfor] span').textContent + ' | ' + !!document.querySelector('article.typeset [data-block="b2"][data-comment-focus]')`); got != "Two in the rack. | true" {
		t.Errorf("it names the block and marks it: %q", got)
	}
	b.typeText("Which rack?")
	b.press("Enter", "Enter", 13, 0)
	if !b.waitFor(`document.querySelector('article.typeset [data-comment][data-id="b2"]')?.dataset.count === '1' && document.querySelector('[data-win="0"] [role=toolbar] [data-act="cmt"]')?.dataset.count === '2'`) {
		t.Fatalf("the block and the head count it: %s", b.eval(drawer))
	}
	if got := b.eval(drawer); !strings.Contains(got, "Which rack?") || !strings.Contains(strings.ToLower(got), "elsewhere on this page") || strings.Contains(got, "Looks good.") {
		t.Errorf("only the comments on b2, the rest elsewhere: %q", got)
	}

	// Reply, resolve, open again.
	b.clickOn(`aside[data-drawer] [data-act="cmtreply"]`, 1)
	if !b.waitFor(`document.activeElement?.dataset?.replycmt === '0'`) {
		t.Fatal("Reply opens a field")
	}
	b.typeText("Rack 2.")
	b.press("Enter", "Enter", 13, 0)
	if !b.waitFor(`document.querySelector('.timeline .timeline > [data-cmt] > p')?.textContent === 'Rack 2.'`) {
		t.Fatalf("the reply shows under its thread: %s", b.eval(drawer))
	}
	if got := b.eval(`document.querySelector('article.typeset [data-comment][data-id="b2"]')?.dataset.count + ' ' + document.querySelectorAll('.timeline .timeline > [data-cmt] [data-act="cmtreply"], .timeline .timeline > [data-cmt] [data-act="cmtres"]').length`); got != "2 0" {
		t.Errorf("the reply counts, and is neither answered nor resolved on its own: %q", got)
	}
	b.shot("comments")
	b.clickOn(`aside[data-drawer] [data-act="cmtres"]`, 1)
	if !b.waitFor(`!!document.querySelector('aside[data-drawer] [data-cmt][data-state="done"]') && !document.querySelector('article.typeset [data-comment][data-id="b2"][data-count]')`) {
		t.Fatalf("resolved, it counts no more: %s", b.eval(drawer))
	}
	if got := b.eval(drawer + ` + ' | ' + document.querySelector('[data-win="0"] [role=toolbar] [data-act="cmt"]')?.dataset.count`); !strings.Contains(got, "Resolved by Anna Berg") || !strings.Contains(got, "Reopen") || !strings.HasSuffix(got, "| 1") {
		t.Errorf("who resolved it, and the head counts only what is open: %q", got)
	}
	b.clickOn(`aside[data-drawer] [data-act="cmtres"]`, 1)
	if !b.waitFor(`!document.querySelector('aside[data-drawer] [data-cmt][data-state="done"]') && document.querySelector('article.typeset [data-comment][data-id="b2"]')?.dataset.count === '2'`) {
		t.Fatalf("open again: %s", b.eval(drawer))
	}

	// Change one's own comment, then delete it with its reply.
	b.clickOn(`aside[data-drawer] > .timeline > [data-cmt] [data-act="cmtedit"]`, 1)
	if !b.waitFor(`document.activeElement?.dataset?.editcmt === '0'`) {
		t.Fatal("Edit opens a field")
	}
	b.typeText("Which rack, exactly?")
	b.press("Enter", "Enter", 13, 0)
	if !b.waitFor(`document.querySelector('aside[data-drawer] [data-cmt] > p')?.textContent === 'Which rack, exactly?'`) {
		t.Fatalf("changed: %s", b.eval(drawer))
	}
	if got := b.eval(`document.querySelector('aside[data-drawer] [data-cmt] > header > time').textContent`); !strings.HasSuffix(got, "· edited") {
		t.Errorf("it says it was changed: %q", got)
	}
	b.clickOn(`aside[data-drawer] > .timeline > [data-cmt] [data-act="cmtdel"]`, 1)
	if got := b.eval(`document.querySelector('aside[data-drawer] > .timeline > [data-cmt] [data-act="cmtdel"]').textContent + ' ' + document.querySelectorAll('aside[data-drawer] [data-cmt]').length`); got != "Delete? Yes 2" {
		t.Errorf("deleting asks first: %q", got)
	}
	b.clickOn(`aside[data-drawer] > .timeline > [data-cmt] [data-act="cmtdel"]`, 1)
	if !b.waitFor(`!document.querySelector('aside[data-drawer] [data-cmt]') && !document.querySelector('article.typeset [data-comment][data-id="b2"][data-count]')`) {
		t.Fatalf("deleted with its reply: %s", b.eval(drawer))
	}

	// The server keeps them: after a reload, the head counts the comment on the whole page.
	b.open(base + "/o/" + notes)
	b.ready("Notes")
	if got := b.eval(`document.querySelector('[data-win="0"] [role=toolbar] [data-act="cmt"]')?.dataset.count`); got != "1" {
		t.Errorf("after a reload: %q", got)
	}

	// While editing, the handle of a block opens its comments; commentOn does it from elsewhere.
	b.clickOn(`[data-win="0"] [data-mode]`, 1)
	if !b.waitFor(`!!document.querySelector('.cm-ud-marker[data-edb="b1"] .editor-handle > button')`) {
		t.Fatal("no handle in the editor")
	}
	// The bubbles belong to reading: none lands in the editor, whose marker lines are wrappers of blocks as well.
	if got := b.eval(`document.querySelectorAll('.cm-editor [data-comment]').length`); got != "0" {
		t.Errorf("comment bubbles inside the editor: %s", got)
	}
	b.clickOn(`.cm-ud-marker[data-edb="b1"] .editor-handle > button`, 1)
	if !b.waitFor(`!!document.querySelector('.editor-handle [role=menu] [data-act="bcmt"]:not([disabled])')`) {
		t.Fatalf("Comment in the menu of the handle is on: %s", b.eval(`document.querySelector('.editor-handle [role=menu]')?.innerHTML.slice(0, 200)`))
	}
	b.clickOn(`.editor-handle [role=menu] [data-act="bcmt"]`, 1)
	if !b.waitFor(`document.querySelector('aside[data-drawer] [data-cfor] small')?.textContent === '#b1' && document.activeElement?.dataset?.addcmt === '0'`) {
		t.Fatalf("the handle opens the comments on its block: %s %s", b.eval(drawer), b.eval(`document.activeElement?.outerHTML.slice(0, 120)`))
	}
	b.typeText("Is this the right heading?")
	b.press("Enter", "Enter", 13, 0)
	b.clickOn(`.cm-ud-marker[data-edb="b1"] .editor-handle > button`, 1)
	if !b.waitFor(`document.querySelector('.editor-handle [role=menu] [data-act="bcmt"] > span:nth-of-type(2)')?.textContent === '1'`) {
		t.Errorf("the menu counts it: %s", b.eval(`document.querySelector('.editor-handle [role=menu]')?.textContent`))
	}
	b.clickOn(`[data-win="0"] [data-mode]`, 1)
	b.eval(`commentOn(at(S.focus).d, 'b2')`)
	if !b.waitFor(`document.querySelector('aside[data-drawer] [data-cfor] small')?.textContent === '#b2' && document.activeElement?.dataset?.addcmt === '0'`) {
		t.Errorf("commentOn opens the drawer on a block: %s", b.eval(drawer))
	}

	// A deleted block keeps its comments: they show as on a block that is gone, still to be answered.
	b.eval(`window.gone = null; fetch('/api/pages/` + notes + `/blocks/b1', {method: 'DELETE'}).then(r => { window.gone = r.status; })`)
	if !b.waitFor(`window.gone === 204`) {
		t.Fatal("block b1 was not deleted")
	}
	b.open(base + "/o/" + notes)
	b.ready("Notes")
	b.clickOn(`[data-win="0"] [role=toolbar] [data-act="cmt"]`, 1)
	if !b.waitFor(`!!document.querySelector('aside[data-drawer] [data-act="cblk"][data-id="b1"]')`) {
		t.Fatalf("the block that is gone is listed: %s", b.eval(drawer))
	}
	if got := b.eval(`document.querySelector('aside[data-drawer] [data-act="cblk"][data-id="b1"]').textContent`); got != "A block that is gone#b1 · 1" {
		t.Errorf("named as gone: %q", got)
	}
	b.clickOn(`aside[data-drawer] [data-act="cblk"][data-id="b1"]`, 1)
	if !b.waitFor(`document.querySelector('aside[data-drawer] [data-cfor] small')?.textContent === '#b1'`) {
		t.Fatalf("its comments open: %s", b.eval(drawer))
	}
	if got := b.eval(drawer + ` + ' | ' + !!document.querySelector('aside[data-drawer] [data-addcmt]') + ' ' + !!document.querySelector('aside[data-drawer] [data-act="cmtreply"]')`); !strings.Contains(got, "Is this the right heading?") || !strings.Contains(got, "This block is gone") || !strings.HasSuffix(got, "| false true") {
		t.Errorf("no new thread on it, but its thread can be answered: %q", got)
	}
	b.shot("comments")

	// Read only: Bob reads the comments, adds none, and sees a bubble only where there are some.
	b.eval(`window.done = null; fetch('/api/objects/` + team + `/access', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: '{"access":"read"}'}).then(r => { window.done = r.status; })`)
	if !b.waitFor(`window.done === 200`) {
		t.Fatal("access not set")
	}
	b.eval(`window.done = null; fetch('/api/pages/` + notes + `/comments', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: '{"block":"b2","body":"On the rack"}'}).then(r => { window.done = r.status; })`)
	if !b.waitFor(`window.done === 201`) {
		t.Fatal("the owner comments on a page set to read only")
	}
	b.signInAs("bob", "Bob")
	b.open(base + "/o/" + notes)
	b.ready("Notes")
	if got := b.eval(`[...document.querySelectorAll('article.typeset [data-comment]')].map(x => x.dataset.id + ':' + (x.dataset.count || '')).join(' ')`); got != "b2:1" {
		t.Errorf("read only, a bubble only where there are comments: %q", got)
	}
	b.clickOn(`[data-win="0"] [role=toolbar] [data-act="cmt"]`, 1)
	if !b.waitFor(`!!document.querySelector('aside[data-drawer] [data-cmt]')`) {
		t.Fatalf("Bob reads the comments: %s", b.eval(drawer))
	}
	if got := b.eval(drawer + ` + ' | ' + document.querySelectorAll('aside[data-drawer] input, aside[data-drawer] [data-act^="cmt"]:not([data-act="cmt"])').length`); !strings.Contains(got, "Looks good.") || !strings.Contains(got, "comments can be read, not added") || !strings.HasSuffix(got, "| 0") {
		t.Errorf("no field and nothing to change: %q", got)
	}
}
