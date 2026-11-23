// SPDX-License-Identifier: MIT

//go:build browser

package browsertest

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"regexp"
	"sync"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/samba"
	"github.com/Ollornog/UnderDoc/internal/share"
	"github.com/Ollornog/UnderDoc/internal/store"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
	"github.com/Ollornog/UnderDoc/internal/web"
)

// companion stands in for the Samba companion of the tenant: the browser test is about the workspace, the tests of
// package share run against real Samba.
type companion struct {
	mu    sync.Mutex
	state samba.State
}

func (c *companion) Do(_ context.Context, r samba.Request) (samba.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r.Op == samba.OpApply {
		c.state = *r.State
	}
	return samba.Response{OK: true}, nil
}

// In the profile an account makes an app password for SMB, shown once, and then sees its Samba user and the shares it
// reaches; the facts of a file in the folder of the tenant name its path on the share; the rights dialog of that file
// says that Samba knows rights per folder only and offers no breaking of the inheritance; the rights dialog of a
// collection of the files switches its share (ADR-30).
func TestSambaInTheWorkspace(t *testing.T) {
	ctx := tenant.With(context.Background(), tenant.Main)
	database, err := db.Open(ctx, pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	v, err := vault.New(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	c := &companion{}
	m, err := share.New(database, st, v, share.Config{Dir: t.TempDir(), Helpers: map[string]share.Helper{tenant.Main: c}, Server: "files"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, base := setupWith(t, web.Options{DevUI: true, DB: database, Store: st, Vault: v, Share: m, GateClient: gateWithRole(t, "ops")}, nil)
	b.open(base + "/")
	b.ready("")

	// The profile: make an app password.
	b.clickOn(`[data-act="view"][data-id="profile"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-samba] [data-act="sambanew"]')`) {
		t.Fatalf("no SMB part in the profile: %s", b.eval(`document.querySelector('[aria-label=Nav3]')?.innerText.slice(0, 400)`))
	}
	if got := b.eval(`document.querySelector('[data-samba-name]').value`); got != "anna" {
		t.Errorf("the suggested name: %q", got)
	}
	b.clickOn(`[data-samba] [data-act="sambanew"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-samba-password]') && !!document.querySelector('[data-samba-user]')`) {
		t.Fatalf("no password shown: %s", b.eval(`document.querySelector('[data-samba]')?.innerText`))
	}
	shown := b.eval(`document.querySelector('[data-samba-password] code:last-of-type').textContent`)
	if !regexp.MustCompile(`^[a-z2-9]{5}(-[a-z2-9]{5}){3}$`).MatchString(shown) {
		t.Errorf("the app password shown: %q", shown)
	}
	if got := b.eval(`document.querySelector('[data-samba-user]').textContent`); got != "anna" {
		t.Errorf("the Samba user: %q", got)
	}
	b.shot("samba-profile")

	// A file in the folder of the tenant, uploaded in the web, and one pass of the working copy.
	b.eval(`window.fileID = null; (async () => {
		const json = {'Content-Type': 'application/json'};
		const roots = (await (await fetch('/api/files')).json()).roots;
		const company = roots.find(r => r.role === 'tenant');
		const body = new TextEncoder().encode('the plan');
		const u = await (await fetch('/api/uploads', {method: 'POST', headers: json, body: JSON.stringify({parent: company.id, name: 'plan.txt', size: body.length})})).json();
		await fetch('/api/uploads/' + u.id, {method: 'PATCH', headers: {'Upload-Offset': '0'}, body});
		const done = await (await fetch('/api/uploads/' + u.id + '/finish', {method: 'POST'})).json();
		const team = await (await fetch('/api/files/collections', {method: 'POST', headers: json, body: JSON.stringify({title: 'Team'})})).json();
		window.teamID = team.id;
		window.fileID = done.id;
	})()`)
	if !b.waitFor(`!!window.fileID && !!window.teamID`) {
		t.Fatal("the file was not uploaded")
	}
	if err := m.Sync(context.Background(), tenant.Main); err != nil {
		t.Fatal(err)
	}
	file := b.eval(`window.fileID`)
	team := b.eval(`window.teamID`)
	b.open(base + "/o/" + file)
	if !b.waitFor(`Object.values(D).some(d => d.sid === '` + file + `' && d.share)`) {
		t.Fatalf("the file did not open with its place on the share: %s", b.eval(`document.querySelector('[data-win]')?.innerText.slice(0, 300)`))
	}
	b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
	if !b.waitFor(`(document.querySelector('[data-share-path]')?.textContent || '') === '\\\\files\\Company\\plan.txt'`) {
		t.Errorf("the facts name the path on the share: %q", b.eval(`document.querySelector('[data-share-path]')?.textContent || ''`))
	}
	b.shot("samba-facts")

	// The rights of the file: per folder only on the share.
	b.eval(`UDRights.open('` + file + `', 'plan.txt'); 1`)
	if !b.waitFor(`!!document.querySelector('#udrights [data-samba-per-folder]')`) {
		t.Fatalf("the rights dialog of the file says nothing of Samba: %s", b.eval(`document.getElementById('udrights')?.innerText`))
	}
	if got := b.eval(`!!document.querySelector('#udrights [data-ract="inherit"]')`); got != "false" {
		t.Error("the rights dialog offers to break the inheritance of a file in a shared library")
	}
	if got := b.eval(`document.querySelector('#udrights [data-samba-path] code')?.textContent`); got != `\\files\Company\plan.txt` {
		t.Errorf("the path in the rights dialog: %q", got)
	}
	b.clickOn(`#udrights [data-ract="close"]`, 1)

	// The rights of a collection of the files switch its share.
	b.eval(`UDRights.open('` + team + `', 'Team'); 1`)
	if !b.waitFor(`!!document.querySelector('#udrights [data-samba-root] [data-ract="samba"]')`) {
		t.Fatalf("no switch of the share: %s", b.eval(`document.getElementById('udrights')?.innerText`))
	}
	b.clickOn(`#udrights [data-samba-root] [data-ract="samba"]`, 1)
	if !b.waitFor(`(document.querySelector('#udrights [data-samba-root]')?.innerText || '').includes('Shared over Samba')`) {
		t.Errorf("the share was not switched on: %s", b.eval(`document.querySelector('#udrights [data-samba-root]')?.innerText`))
	}
	roots, err := database.SharedRoots(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range roots {
		found = found || r.ID == team
	}
	if !found {
		t.Error("the collection is not shared in the database")
	}
	b.shot("samba-collection")
}
