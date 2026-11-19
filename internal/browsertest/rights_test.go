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
	// The log names what was done, the group made among it.
	b.clickOn(`[data-act="sec"][data-id="log"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-rights="log"] [data-log] [data-entry="group"]') && (document.querySelector('[data-rights="log"]')?.innerText || '').includes('Board')`) {
		t.Errorf("the log: %s", b.eval(`document.querySelector('.spanel')?.innerText`))
	}
	b.shot("rights-log")
}
