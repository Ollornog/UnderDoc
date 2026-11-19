//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"context"
	"strconv"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/web"
)

// signInWith sets the headers of the gate for another person with groups.
func (b *browser) signInWith(login, name, groups string) {
	b.t.Helper()
	b.call("Network.setExtraHTTPHeaders", map[string]any{"headers": map[string]string{"Remote-User": login, "Remote-Name": name, "Remote-Groups": groups}}, nil)
}

// TestTheRoleDecidesWhatIsGreyed: what the viewer may do comes from their role (ADR-29), not from the level for all
// others. Team is read only for the tenant, but the group ops edits it: carl, in ops, may add to it and sees the lock
// that tells what others may do; bob, in no group, may not add.
func TestTheRoleDecidesWhatIsGreyed(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.eval(`window.done = null; (async () => {
		const json = {'Content-Type': 'application/json'};
		await fetch('/api/objects/` + team + `/access', {method: 'POST', headers: json, body: JSON.stringify({access: 'read'})});
		const people = await (await fetch('/api/people')).json();
		const ops = people.groups.find(g => g.name === 'ops');
		const r = await fetch('/api/objects/` + team + `/grants', {method: 'PUT', headers: json, body: JSON.stringify({kind: 'group', who: ops.id, role: 'edit'})});
		window.done = r.status;
	})()`)
	if !b.waitFor(`window.done === 204`) {
		t.Fatalf("the group ops was not granted edit: %s", b.eval(`String(window.done)`))
	}
	newHere := `[document.querySelector('.cheadrow[data-cdrop="` + team + `"] .newplus') !== null, !!document.querySelector('.cheadrow[data-cdrop="` + team + `"] .acc')].join(' ')`
	b.signInWith("carl", "Carl", "ops")
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := b.eval(newHere); got != "true true" {
		t.Errorf("carl edits through ops, and the lock says what others may do: %q", got)
	}
	b.signInWith("bob", "Bob", "")
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := b.eval(newHere); got != "false true" {
		t.Errorf("bob only reads: %q", got)
	}
	b.signIn(true)
}

// setupAsAdmin is setupWithDatabase where the group ops administers the tenant, as Anna's sign-in names it.
func setupAsAdmin(t *testing.T) (*browser, string) {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if err := database.SetSignIn(tenant.With(context.Background(), tenant.Main), db.SignIn{Methods: []string{db.MethodGate}}); err != nil {
		t.Fatal(err)
	}
	return setupWith(t, web.Options{DevUI: true, DB: database, AdminGroup: "ops"}, nil)
}

// submit fills a form of the rights by script, as a person would by hand, and sends it.
func (b *browser) submit(form string, values map[string]string) {
	b.t.Helper()
	b.exists(form)
	script := `(() => { const f = document.querySelector(` + strconv.Quote(form) + `);`
	for name, value := range values {
		script += `f.elements[` + strconv.Quote(name) + `].value = ` + strconv.Quote(value) + `;`
	}
	b.eval(script + ` f.requestSubmit(); })()`)
}

// TestTheRightsDialog: from the settings of a page, its owner grants a person a role, breaks the inheritance and makes a
// link; the facts panel names who sees the page; the link shows the page to anyone (ADR-29).
func TestTheRightsDialog(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.signInWith("bob", "Bob Kern", "")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.signIn(true)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	b.clickOn(rowNamed(b, "A")+` [data-act="open"]`, 1)
	b.ready("A")
	b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
	b.clickOn(`[data-act="rights"]`, 1)
	if !b.waitFor(`document.getElementById('udrights')?.open && !!document.querySelector('#udrights [data-rform="grant"]')`) {
		t.Fatalf("the dialog opens with the means to grant: %s", b.eval(`document.getElementById('udrights')?.innerText`))
	}
	b.submit(`#udrights [data-rform="grant"]`, map[string]string{"who": "person:bob", "role": "comment"})
	if !b.waitFor(`!!document.querySelector('#udrights [data-grant="person:bob"]')`) {
		t.Fatalf("bob is granted: %s", b.eval(`document.getElementById('udrights').innerText`))
	}
	b.clickOn(`#udrights [data-ract="inherit"]`, 1)
	if !b.waitFor(`document.getElementById('udrights').innerText.includes('Takes no roles from above')`) {
		t.Errorf("the inheritance breaks: %s", b.eval(`document.getElementById('udrights').innerText`))
	}
	b.submit(`#udrights [data-rform="link"]`, map[string]string{"mode": "read"})
	if !b.waitFor(`!!document.querySelector('#udrights [data-made-link] input')?.value.includes('/s/')`) {
		t.Fatalf("a link is made and shown once: %s", b.eval(`document.getElementById('udrights').innerText`))
	}
	link := b.eval(`document.querySelector('#udrights [data-made-link] input').value`)
	b.shot("rights-dialog")
	if got := b.eval(`document.querySelectorAll('#udrights [data-links] tr[data-link]').length`); got != "1" {
		t.Errorf("the link is listed: %s", got)
	}
	b.clickOn(`#udrights [data-ract="close"]`, 1)
	// The facts panel names who sees the page.
	b.open(base + "/o/" + team)
	b.ready("Team")
	b.clickOn(rowNamed(b, "A")+` [data-act="open"]`, 1)
	b.ready("A")
	b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
	if !b.waitFor(`(document.querySelector('[data-seen-by]')?.textContent || '').includes('Bob Kern (comment)')`) {
		t.Errorf("who sees it: %s", b.eval(`document.querySelector('[data-seen-by]')?.textContent`))
	}
	b.open(link)
	if !b.waitFor(`document.querySelector('[data-test="share-title"]')?.textContent === 'A'`) {
		t.Errorf("the link shows the page: %s", b.eval(`document.body.innerText.slice(0, 300)`))
	}
	b.shot("share-page")
}

// TestTheTabRights: an administrator makes a group in the tab Rights, takes a member in, sees the users, and asks who
// sees a page (ADR-29).
func TestTheTabRights(t *testing.T) {
	b, base := setupAsAdmin(t)
	b.signInWith("bob", "Bob Kern", "")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.signIn(true)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.clickOn(`[data-act="view"][data-id="rights"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-rights="groups"] [data-group="ops"]')`) {
		t.Fatalf("the groups, with ops from the sign-in: %s", b.eval(`document.querySelector('.spanel')?.innerText`))
	}
	b.submit(`[data-rights="groups"] [data-rform="group"]`, map[string]string{"name": "Board", "description": "The board"})
	if !b.waitFor(`!!document.querySelector('[data-rights="groups"] [data-group="Board"]')`) {
		t.Fatalf("the group is made: %s", b.eval(`document.querySelector('.spanel')?.innerText`))
	}
	b.clickOn(`[data-group="Board"] [data-ract="members"]`, 1)
	b.submit(`[data-rights="groups"] [data-rform="member"]`, map[string]string{"account": "bob"})
	if !b.waitFor(`(document.querySelector('[data-rights="groups"]')?.innerText || '').includes('Bob Kern')`) {
		t.Errorf("bob is a member: %s", b.eval(`document.querySelector('.spanel')?.innerText`))
	}
	b.clickOn(`[data-act="sec"][data-id="users"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-rights="users"] [data-user="bob"]')`) {
		t.Errorf("the users: %s", b.eval(`document.querySelector('.spanel')?.innerText`))
	}
	b.clickOn(`[data-act="sec"][data-id="check"]`, 1)
	b.submit(`[data-rights="check"] [data-rform="who"]`, map[string]string{"object": "/o/" + team, "account": "bob"})
	if !b.waitFor(`(document.querySelector('[data-who]')?.innerText || '').includes('Everyone') && (document.querySelector('[data-check]')?.innerText || '').includes('Role: edit')`) {
		t.Errorf("who sees Team, and what bob may do: %s", b.eval(`document.querySelector('.spanel')?.innerText`))
	}
	b.shot("rights-tab")
	// What bob left behind: a collection of his own, which the administrator transfers without seeing what is in it.
	b.signInWith("bob", "Bob Kern", "")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.eval(`window.left = null; fetch('/api/collections', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: '{"title":"Left behind"}'}).then(r => r.json()).then(j => { window.left = j.id; })`)
	if !b.waitFor(`!!window.left`) {
		t.Fatal("bob's collection was not made")
	}
	left := b.eval(`window.left`)
	b.signIn(true)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.clickOn(`[data-act="view"][data-id="rights"]`, 1)
	b.clickOn(`[data-act="sec"][data-id="users"]`, 1)
	b.clickOn(`[data-rights="users"] [data-user="bob"] [data-ract="heldby"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-held-row="` + left + `"] [data-rform="transfer"]')`) {
		t.Fatalf("what bob owns, with Transfer: %s", b.eval(`document.querySelector('.spanel')?.innerText`))
	}
	b.eval(`window.confirm = () => true`)
	b.submit(`[data-held-row="`+left+`"] [data-rform="transfer"]`, map[string]string{"to": "anna"})
	// Bob keeps edit, as a former owner does.
	if !b.waitFor(`!document.querySelector('[data-held-row="` + left + `"] [data-rform="transfer"]') && (document.querySelector('[data-held-row="` + left + `"]')?.innerText || '').includes('edit')`) {
		t.Errorf("transferred, bob only edits it: %s", b.eval(`document.querySelector('.spanel')?.innerText`))
	}
	// The log names what was done, the group made among it.
	b.clickOn(`[data-act="sec"][data-id="log"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-rights="log"] [data-log] [data-entry="group"]') && (document.querySelector('[data-rights="log"]')?.innerText || '').includes('Board')`) {
		t.Errorf("the log: %s", b.eval(`document.querySelector('.spanel')?.innerText`))
	}
	b.shot("rights-log")
}

// TestANewCollectionAsksWithWhomToShare: the dialog of a new collection at the top has the step Share with, through
// the same roles as the rights dialog. Anna shares hers with bob to comment and makes it; bob comments, carl sees
// nothing, and its overview says that nobody else may do anything with it (ADR-29, 2026-11-19).
func TestANewCollectionAsksWithWhomToShare(t *testing.T) {
	b, base := setupWithDatabase(t)
	for _, who := range [][2]string{{"bob", "Bob Kern"}, {"carl", "Carl"}} {
		b.signInWith(who[0], who[1], "")
		b.open(base + "/o/plan")
		b.ready("Network plan")
	}
	b.signIn(true)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.clickOn(`.nav2 [data-act="addmenu"][data-id="n2"]`, 1)
	b.clickOn(probe(b, `.cmdpop [role="menuitem"]`, "Collection"), 1)
	if !b.waitFor(`document.getElementById('udrights')?.open && !!document.querySelector('#udrights [data-share-step] option[value="person:bob"]')`) {
		t.Fatalf("the step Share with lists the people: %s", b.eval(`document.getElementById('udrights')?.innerText`))
	}
	b.eval(`(() => { const f = document.querySelector('#udrights [data-share-step]'); f.querySelector('select[name="who"]').value = 'person:bob';
		f.querySelector('select[name="role"]').value = 'comment'; })()`)
	b.clickOn(`#udrights [data-ract="newshare"]`, 1)
	if !b.waitFor(`!!document.querySelector('#udrights [data-new-share="person:bob"]') && !document.querySelector('#udrights [data-only-me]')`) {
		t.Fatalf("bob is in the list: %s", b.eval(`document.getElementById('udrights').innerText`))
	}
	// Added twice, the same one is listed once, with the role chosen last.
	b.eval(`document.querySelector('#udrights [data-share-step] select[name="who"]').value = 'tenant:'`)
	b.clickOn(`#udrights [data-ract="newshare"]`, 1)
	if !b.waitFor(`document.querySelectorAll('#udrights [data-new-shares] tr').length === 2`) {
		t.Fatalf("everyone is in the list: %s", b.eval(`document.getElementById('udrights').innerText`))
	}
	b.clickOn(`#udrights [data-new-share="tenant:"] [data-ract="newunshare"]`, 1)
	if !b.waitFor(`document.querySelectorAll('#udrights [data-new-shares] tr').length === 1`) {
		t.Fatalf("everyone is taken out: %s", b.eval(`document.getElementById('udrights').innerText`))
	}
	b.eval(`(() => { const f = document.querySelector('#udrights [data-new-title]'); f.value = 'Board'; f.dispatchEvent(new Event('input', {bubbles: true})); })()`)
	b.shot("new-collection-share")
	b.clickOn(`#udrights footer button[type="submit"]`, 1)
	if !b.waitFor(`!document.getElementById('udrights').open && [...document.querySelectorAll('.nav2 .lbl')].some(n => n.textContent === 'Board')`) {
		t.Fatalf("Board is made: %s", b.eval(`document.getElementById('udrights')?.innerText + ' / ' + document.querySelector('.nav2').innerText.replace(/\s+/g, ' ')`))
	}
	board := b.eval(`(() => { const n = [...document.querySelectorAll('.nav2 [data-act="coll"]')].find(n => n.textContent.includes('Board')); return D[n.dataset.id].sid; })()`)
	b.eval(`window.r = null; fetch('/api/objects/` + board + `/rights').then(r => r.json()).then(j => { window.r = JSON.stringify(j.own.map(g => g.kind + ':' + g.who + ':' + g.role)); }); ''`)
	if !b.waitFor(`window.r === '["person:bob:comment"]'`) {
		t.Errorf("only bob is granted, to comment: %s", b.eval(`String(window.r)`))
	}
	// Its overview: nobody else, that is, nobody in the workspace without a role of their own.
	b.open(base + "/o/" + board)
	b.ready("Board")
	if got := b.eval(`document.querySelector('[data-act="cacc"][data-id="none"]')?.getAttribute('aria-pressed')`); got != "true" {
		t.Errorf("the overview says nobody else: %q", got)
	}
	b.signInWith("bob", "Bob Kern", "")
	b.open(base + "/o/" + board)
	b.ready("Board")
	b.signInWith("carl", "Carl", "")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	if got := b.eval(`[...document.querySelectorAll('.nav2 .lbl')].some(n => n.textContent === 'Board')`); got != "false" {
		t.Errorf("carl sees Board")
	}
	b.eval(`window.c = null; fetch('/api/objects/` + board + `/rights').then(r => { window.c = r.status; }); ''`)
	if !b.waitFor(`window.c === 404`) {
		t.Errorf("carl reads the rights of Board: %s", b.eval(`String(window.c)`))
	}
	b.signIn(true)
}
