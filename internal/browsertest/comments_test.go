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
const drawer = `(document.querySelector('.fdrawer[aria-label="Comments"]')?.innerText.replace(/\s+/g, ' ').trim() || 'none')`

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
	if got := b.eval(`[...document.querySelectorAll('article.doc .cblk')].map(x => x.dataset.id + ':' + x.textContent).join(' ')`); got != "b1: b2:" {
		t.Errorf("a speech bubble beside each block, none counted yet: %q", got)
	}

	// The head opens the comments on the whole page.
	b.clickOn(`[data-win="0"] .tools [data-act="cmt"]`, 1)
	if !b.waitFor(`document.activeElement?.dataset?.addcmt === '0'`) {
		t.Fatalf("the drawer opens with its field: %s", b.eval(drawer))
	}
	if got := b.eval(drawer); !strings.Contains(got, "The whole page") || !strings.Contains(got, "No comments on this yet.") {
		t.Errorf("the whole page, nothing yet: %q", got)
	}
	b.typeText(`Looks good. <img src=x onerror="window.hacked=1">`)
	b.press("Enter", "Enter", 13, 0)
	if !b.waitFor(`!!document.querySelector('.fdrawer .cmtrow') && document.querySelector('[data-win="0"] .tools [data-act="cmt"] .cnum')?.textContent === '1'`) {
		t.Fatalf("the comment shows and the head counts it: %s", b.eval(drawer))
	}
	if got := b.eval(`document.querySelector('.fdrawer .cmtrow p').textContent + ' | ' + !!document.querySelector('.fdrawer .cmtrow img') + ' ' + !!window.hacked + ' | ' + document.querySelector('.fdrawer .cmtrow b').textContent`); got != `Looks good. <img src=x onerror="window.hacked=1"> | false false | Anna Berg` {
		t.Errorf("the text stays text: %q", got)
	}

	// A speech bubble beside a block opens the comments on that block.
	b.clickOn(`[data-win="0"] .fdrawer [data-act="cmt"]`, 1)
	b.clickOn(`article.doc .cblk[data-id="b2"]`, 1)
	if !b.waitFor(`document.querySelector('.fdrawer .cfor em')?.textContent === '#b2' && document.activeElement?.dataset?.addcmt === '0'`) {
		t.Fatalf("the drawer on block b2: %s", b.eval(drawer))
	}
	if got := b.eval(`document.querySelector('.fdrawer .cfor span').textContent + ' | ' + !!document.querySelector('article.doc [data-b="b2"].cfocus')`); got != "Two in the rack. | true" {
		t.Errorf("it names the block and marks it: %q", got)
	}
	b.typeText("Which rack?")
	b.press("Enter", "Enter", 13, 0)
	if !b.waitFor(`document.querySelector('article.doc .cblk[data-id="b2"] em')?.textContent === '1' && document.querySelector('[data-win="0"] .tools .cnum')?.textContent === '2'`) {
		t.Fatalf("the block and the head count it: %s", b.eval(drawer))
	}
	if got := b.eval(drawer); !strings.Contains(got, "Which rack?") || !strings.Contains(strings.ToLower(got), "elsewhere on this page") || strings.Contains(got, "Looks good.") {
		t.Errorf("only the comments on b2, the rest elsewhere: %q", got)
	}

	// Reply, resolve, open again.
	b.clickOn(`.fdrawer [data-act="cmtreply"]`, 1)
	if !b.waitFor(`document.activeElement?.dataset?.replycmt === '0'`) {
		t.Fatal("Reply opens a field")
	}
	b.typeText("Rack 2.")
	b.press("Enter", "Enter", 13, 0)
	if !b.waitFor(`document.querySelector('.fdrawer .cmtrow.reply p')?.textContent === 'Rack 2.'`) {
		t.Fatalf("the reply shows under its thread: %s", b.eval(drawer))
	}
	if got := b.eval(`document.querySelector('article.doc .cblk[data-id="b2"] em')?.textContent + ' ' + document.querySelectorAll('.fdrawer .cmtrow.reply [data-act="cmtreply"], .fdrawer .cmtrow.reply [data-act="cmtres"]').length`); got != "2 0" {
		t.Errorf("the reply counts, and is neither answered nor resolved on its own: %q", got)
	}
	b.shot("comments")
	b.clickOn(`.fdrawer [data-act="cmtres"]`, 1)
	if !b.waitFor(`!!document.querySelector('.fdrawer .cmtrow.done') && !document.querySelector('article.doc .cblk[data-id="b2"] em')`) {
		t.Fatalf("resolved, it counts no more: %s", b.eval(drawer))
	}
	if got := b.eval(drawer + ` + ' | ' + document.querySelector('[data-win="0"] .tools .cnum')?.textContent`); !strings.Contains(got, "Resolved by Anna Berg") || !strings.Contains(got, "Reopen") || !strings.HasSuffix(got, "| 1") {
		t.Errorf("who resolved it, and the head counts only what is open: %q", got)
	}
	b.clickOn(`.fdrawer [data-act="cmtres"]`, 1)
	if !b.waitFor(`!document.querySelector('.fdrawer .cmtrow.done') && document.querySelector('article.doc .cblk[data-id="b2"] em')?.textContent === '2'`) {
		t.Fatalf("open again: %s", b.eval(drawer))
	}

	// Change one's own comment, then delete it with its reply.
	b.clickOn(`.fdrawer .cmtrow:not(.reply) [data-act="cmtedit"]`, 1)
	if !b.waitFor(`document.activeElement?.dataset?.editcmt === '0'`) {
		t.Fatal("Edit opens a field")
	}
	b.typeText("Which rack, exactly?")
	b.press("Enter", "Enter", 13, 0)
	if !b.waitFor(`document.querySelector('.fdrawer .cmtrow p')?.textContent === 'Which rack, exactly?'`) {
		t.Fatalf("changed: %s", b.eval(drawer))
	}
	if got := b.eval(`document.querySelector('.fdrawer .cmtrow em').textContent`); !strings.HasSuffix(got, "· edited") {
		t.Errorf("it says it was changed: %q", got)
	}
	b.clickOn(`.fdrawer .cmtrow:not(.reply) [data-act="cmtdel"]`, 1)
	if got := b.eval(`document.querySelector('.fdrawer .cmtrow:not(.reply) [data-act="cmtdel"]').textContent + ' ' + document.querySelectorAll('.fdrawer .cmtrow').length`); got != "Delete? Yes 2" {
		t.Errorf("deleting asks first: %q", got)
	}
	b.clickOn(`.fdrawer .cmtrow:not(.reply) [data-act="cmtdel"]`, 1)
	if !b.waitFor(`!document.querySelector('.fdrawer .cmtrow') && !document.querySelector('article.doc .cblk[data-id="b2"] em')`) {
		t.Fatalf("deleted with its reply: %s", b.eval(drawer))
	}

	// The server keeps them: after a reload, the head counts the comment on the whole page.
	b.open(base + "/o/" + notes)
	b.ready("Notes")
	if got := b.eval(`document.querySelector('[data-win="0"] .tools [data-act="cmt"] .cnum')?.textContent`); got != "1" {
		t.Errorf("after a reload: %q", got)
	}

	// While editing, the bar of a block opens its comments; commentOn does it from elsewhere.
	b.clickOn(`[data-win="0"] .dstatus`, 1)
	if !b.waitFor(`!!document.querySelector('.btool [data-act="cblk"][data-id="b1"]:not([disabled])')`) {
		t.Fatalf("the comment button in the bar is on: %s", b.eval(`document.querySelector('.btool')?.innerHTML.slice(0, 200)`))
	}
	b.eval(`document.querySelector('textarea[data-blk="b1"]').focus()`)
	b.clickOn(`.btool [data-act="cblk"][data-id="b1"]`, 1)
	if !b.waitFor(`document.querySelector('.fdrawer .cfor em')?.textContent === '#b1' && document.activeElement?.dataset?.addcmt === '0'`) {
		t.Fatalf("the bar opens the comments on its block: %s %s", b.eval(drawer), b.eval(`document.activeElement?.outerHTML.slice(0, 120)`))
	}
	b.typeText("Is this the right heading?")
	b.press("Enter", "Enter", 13, 0)
	if !b.waitFor(`document.querySelector('.btool [data-act="cblk"][data-id="b1"] .cnum')?.textContent === '1'`) {
		t.Errorf("the bar counts it: %s", b.eval(drawer))
	}
	b.clickOn(`[data-win="0"] .dstatus`, 1)
	b.eval(`commentOn(at(S.focus).d, 'b2')`)
	if !b.waitFor(`document.querySelector('.fdrawer .cfor em')?.textContent === '#b2' && document.activeElement?.dataset?.addcmt === '0'`) {
		t.Errorf("commentOn opens the drawer on a block: %s", b.eval(drawer))
	}

	// A deleted block keeps its comments: they show as on a block that is gone, still to be answered.
	b.eval(`window.gone = null; fetch('/api/pages/` + notes + `/blocks/b1', {method: 'DELETE'}).then(r => { window.gone = r.status; })`)
	if !b.waitFor(`window.gone === 204`) {
		t.Fatal("block b1 was not deleted")
	}
	b.open(base + "/o/" + notes)
	b.ready("Notes")
	b.clickOn(`[data-win="0"] .tools [data-act="cmt"]`, 1)
	if !b.waitFor(`!!document.querySelector('.fdrawer .cgo[data-id="b1"]')`) {
		t.Fatalf("the block that is gone is listed: %s", b.eval(drawer))
	}
	if got := b.eval(`document.querySelector('.fdrawer .cgo[data-id="b1"]').textContent`); got != "A block that is gone#b1 · 1" {
		t.Errorf("named as gone: %q", got)
	}
	b.clickOn(`.fdrawer .cgo[data-id="b1"]`, 1)
	if !b.waitFor(`document.querySelector('.fdrawer .cfor em')?.textContent === '#b1'`) {
		t.Fatalf("its comments open: %s", b.eval(drawer))
	}
	if got := b.eval(drawer + ` + ' | ' + !!document.querySelector('.fdrawer [data-addcmt]') + ' ' + !!document.querySelector('.fdrawer [data-act="cmtreply"]')`); !strings.Contains(got, "Is this the right heading?") || !strings.Contains(got, "This block is gone") || !strings.HasSuffix(got, "| false true") {
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
	if got := b.eval(`[...document.querySelectorAll('article.doc .cblk')].map(x => x.dataset.id + ':' + x.textContent).join(' ')`); got != "b2:1" {
		t.Errorf("read only, a bubble only where there are comments: %q", got)
	}
	b.clickOn(`[data-win="0"] .tools [data-act="cmt"]`, 1)
	if !b.waitFor(`!!document.querySelector('.fdrawer .cmtrow')`) {
		t.Fatalf("Bob reads the comments: %s", b.eval(drawer))
	}
	if got := b.eval(drawer + ` + ' | ' + document.querySelectorAll('.fdrawer input, .fdrawer [data-act^="cmt"]:not([data-act="cmt"])').length`); !strings.Contains(got, "Looks good.") || !strings.Contains(got, "comments can be read, not added") || !strings.HasSuffix(got, "| 0") {
		t.Errorf("no field and nothing to change: %q", got)
	}
}
