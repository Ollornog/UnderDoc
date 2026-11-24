//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"context"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/gateapi"
	"github.com/Ollornog/UnderDoc/internal/gateapi/gatetest"
	"github.com/Ollornog/UnderDoc/internal/web"
)

// Accounts through the gate (T-33) in the tab Rights: an administrator invites a colleague into a group and gets the
// sign-in link once, opens the account and locks it. The gate is the stand-in of package gatetest.
func TestInvitingAndLockingInTheTabRights(t *testing.T) {
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	g := gatetest.New(t, "underdoc", []string{"underdoc-user"}, "underdoc-admin")
	client, err := gateapi.New(gateapi.Config{URL: g.URL, KeyFile: g.KeyFile()})
	if err != nil {
		t.Fatal(err)
	}
	b, base := setupWith(t, web.Options{DevUI: true, DB: database,
		Gate: &web.GateAPI{Client: client, Roles: []string{"underdoc-user"}, ApproverRole: "underdoc-admin", App: "underdoc"}}, nil)
	b.signInWith("anna", "Anna Berg", "ops,underdoc-admin")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.clickOn(`[data-act="view"][data-id="rights"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-rights="groups"] [data-rform="group"]')`) {
		t.Fatalf("the groups: %s", b.eval(`document.querySelector('[aria-label=Nav3] aside.card')?.innerText`))
	}
	b.submit(`[data-rights="groups"] [data-rform="group"]`, map[string]string{"name": "Board", "description": ""})
	if !b.waitFor(`!!document.querySelector('[data-rights="groups"] [data-group="Board"]')`) {
		t.Fatal("the group was not made")
	}
	b.clickOn(`[data-group="Board"] [data-ract="members"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-rform="invite"]')`) {
		t.Fatalf("no invitation offered: %s", b.eval(`document.querySelector('[data-rights="groups"]')?.innerText`))
	}
	b.submit(`[data-rform="invite"]`, map[string]string{"login": "lena", "name": "Lena Moor", "email": ""})
	if !b.waitFor(`(document.querySelector('[data-signin-link] input')?.value || '').includes('/auth/magic/')`) {
		t.Fatalf("the sign-in link is not shown: %s", b.eval(`document.querySelector('[data-rights="groups"]')?.innerText`))
	}
	if !b.waitFor(`(document.querySelector('[data-group-detail="Board"]')?.innerText || '').includes('Lena Moor')`) {
		t.Fatalf("lena is not in the group: %s", b.eval(`document.querySelector('[data-group-detail="Board"]')?.innerText`))
	}
	b.shot("invited")
	b.clickOn(`[data-group-detail="Board"] [data-ract="account"]`, 1)
	if !b.waitFor(`(document.querySelector('[data-account-state]')?.innerText || '').includes('active in the gate')`) {
		t.Fatalf("the account is not shown: %s", b.eval(`document.querySelector('[data-group-detail="Board"]')?.innerText`))
	}
	b.eval(`window.confirm = () => true`)
	b.clickOn(`[data-account] [data-ract="lock"]`, 1)
	if !b.waitFor(`(document.querySelector('[data-account-state]')?.innerText || '').includes('❌ locked in UnderDoc') && (document.querySelector('[data-account-state]')?.innerText || '').includes('locked in the gate')`) {
		t.Fatalf("locked, it does not say so: %s", b.eval(`document.querySelector('[data-account]')?.innerText`))
	}
	b.shot("locked")
	// A group admin locks nobody (B-7): carl, admin of Board, takes lena out of his groups, and she stays as she is.
	b.signInWith("carl", "Carl", "")
	b.eval(`window.done = null; fetch('/api/state').then(r => { window.done = r.status; })`)
	if !b.waitFor(`window.done === 200`) {
		t.Fatalf("carl does not come in: %s", b.eval(`String(window.done)`))
	}
	b.signInWith("anna", "Anna Berg", "ops,underdoc-admin")
	b.eval(`window.done = null; (async () => {
		const json = {'Content-Type': 'application/json'};
		await fetch('/api/accounts/' + document.querySelector('[data-account]').dataset.account + '/lock', {method: 'PUT', headers: json, body: JSON.stringify({locked: false})});
		const groups = await (await fetch('/api/groups')).json();
		const board = (groups.groups || groups).find(g => g.name === 'Board');
		const r = await fetch('/api/groups/' + board.id + '/admins/` + remoteID("carl") + `', {method: 'PUT', headers: json, body: JSON.stringify({admin: true})});
		window.done = r.status;
	})()`)
	if !b.waitFor(`window.done === 204`) {
		t.Fatalf("carl was not made admin of Board: %s", b.eval(`String(window.done)`))
	}
	b.signInWith("carl", "Carl", "")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.clickOn(`[data-act="view"][data-id="rights"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-group="Board"] [data-ract="members"]')`) {
		t.Fatalf("carl does not see Board: %s", b.eval(`document.querySelector('[aria-label=Nav3] aside.card')?.innerText`))
	}
	b.clickOn(`[data-group="Board"] [data-ract="members"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-group-detail="Board"] [data-ract="account"]')`) {
		t.Fatalf("no account offered to carl: %s", b.eval(`document.querySelector('[data-rights="groups"]')?.innerText`))
	}
	b.clickOn(`[data-group-detail="Board"] [data-ract="account"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-account] [data-ract="removeacct"]') && !document.querySelector('[data-account] [data-ract="lock"]') && !document.querySelector('[data-account] [data-ract="signinlink"]')`) {
		t.Fatalf("carl is not offered only to remove lena: %s", b.eval(`document.querySelector('[data-group-detail="Board"]')?.innerText`))
	}
	b.eval(`window.confirm = () => true`)
	b.clickOn(`[data-account] [data-ract="removeacct"]`, 1)
	if !b.waitFor(`!(document.querySelector('[data-group-detail="Board"]')?.innerText || '').includes('Lena Moor')`) {
		t.Fatalf("lena is still in Board: %s", b.eval(`document.querySelector('[data-group-detail="Board"]')?.innerText`))
	}
	b.shot("removed by a group admin")
}

// Without the account API the tab Rights offers neither invitations nor accounts.
func TestNoAccountsWithoutTheGatesAPI(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.signInWith("anna", "Anna Berg", "ops,underdoc-admin")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.clickOn(`[data-act="view"][data-id="rights"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-rights="groups"] [data-rform="group"]')`) {
		t.Fatal("the groups do not load")
	}
	b.submit(`[data-rights="groups"] [data-rform="group"]`, map[string]string{"name": "Board", "description": ""})
	if !b.waitFor(`!!document.querySelector('[data-rights="groups"] [data-group="Board"]')`) {
		t.Fatal("the group was not made")
	}
	b.clickOn(`[data-group="Board"] [data-ract="members"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-group-detail="Board"]')`) {
		t.Fatal("the members do not open")
	}
	if b.exists(`[data-rform="invite"]`) || b.exists(`[data-ract="account"]`) {
		t.Error("without the account API, accounts are offered")
	}
}

// With approvals of the gate, deleting for good sends the browser to the gate's page to confirm it (T-33).
func TestPurgeGoesToTheGatesConfirmation(t *testing.T) {
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	g := gatetest.New(t, "underdoc", []string{"underdoc-user"}, "underdoc-admin")
	client, err := gateapi.New(gateapi.Config{URL: g.URL, KeyFile: g.KeyFile()})
	if err != nil {
		t.Fatal(err)
	}
	b, base := setupWith(t, web.Options{DevUI: true, DB: database, Gate: &web.GateAPI{Client: client, App: "underdoc", Approvals: true}}, nil)
	b.signInWith("anna", "Anna Berg", "ops,underdoc-admin")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.eval(`window.made = null; (async () => {
		const r = await fetch('/api/pages', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({title: 'Plan', parent: '` + team + `'})});
		const id = (await r.json()).id;
		await fetch('/api/objects/' + id, {method: 'DELETE'});
		window.made = id;
	})()`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the page was not made")
	}
	b.eval(`window.trashConfirm = () => 'Plan'; purgeTrash(window.made)`)
	if !b.waitFor(`location.href.startsWith('` + g.URL + `/auth/approvals/apv_')`) {
		t.Fatalf("the browser is not sent to the gate: %s", b.eval(`location.href`))
	}
}
