// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
)

// setLevel sets what the whole tenant may do with an object, as an administrator: the level of ADR-20 in the roles of
// ADR-29.
func setLevel(db *DB, objectID, level string) error {
	_, err := db.SetAccess(inMain(), objectID, Actor{ID: anna.ID, Admin: true}, level)
	return err
}

// carl is a third account with no group, for whoever holds nothing special.
var carl = account.User{ID: "u-carl", Login: "carl", Name: "Carl"}

func role(t *testing.T, db *DB, objectID, viewer string) string {
	t.Helper()
	got, err := db.Role(inMain(), objectID, viewer)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// TestRolesForPersonsGroupsAndTheTenant: only allowing, the highest role counts, the owner of something above manages,
// groups of both sources count, and the role reaches everything below (ADR-29).
func TestRolesForPersonsGroupsAndTheTenant(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.SeeAccount(ctx, carl); err != nil {
		t.Fatal(err)
	}
	admin := Actor{ID: anna.ID}
	team, _ := db.createShared(ctx, anna, "Team")
	folder, _ := db.CreateFolder(ctx, anna, "Plans", team.ID)
	page, _ := db.CreatePage(ctx, bob, "Plan", folder.ID)
	if got := role(t, db, page.ID, carl.ID); got != RoleEdit {
		t.Errorf("a collection shared with the tenant to edit, below too: %q", got)
	}
	if got := role(t, db, page.ID, bob.ID); got != RoleOwner {
		t.Errorf("bob owns the page: %q", got)
	}
	if got := role(t, db, page.ID, anna.ID); got != RoleManage {
		t.Errorf("anna owns Team above it and manages: %q", got)
	}
	if err := db.SetGrant(ctx, team.ID, admin, ToTenant, "", RoleRead); err != nil {
		t.Fatal(err)
	}
	if got := role(t, db, page.ID, carl.ID); got != RoleRead {
		t.Errorf("the tenant reads now: %q", got)
	}
	// A person and a group taken over from the sign-in (anna's group ops).
	if err := db.SetGrant(ctx, folder.ID, admin, ToPerson, carl.ID, RoleComment); err != nil {
		t.Fatal(err)
	}
	if got := role(t, db, page.ID, carl.ID); got != RoleComment {
		t.Errorf("carl comments below the folder: %q", got)
	}
	groups, err := db.Groups(ctx)
	if err != nil || len(groups) != 1 || groups[0].Name != "ops" || groups[0].Source != GroupSynced || groups[0].Members != 1 {
		t.Fatalf("the group of anna's sign-in is taken over: %+v %v", groups, err)
	}
	ops := groups[0].ID
	if err := db.SetMember(ctx, Actor{ID: anna.ID, Admin: true}, ops, carl.ID, true); !errors.Is(err, ErrGroup) {
		t.Errorf("a group taken over can only be read: %v", err)
	}
	// A local group.
	editors, err := db.CreateGroup(ctx, Actor{ID: anna.ID, Admin: true}, "Editors", "They write the plans", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateGroup(ctx, Actor{ID: anna.ID, Admin: true}, " Editors ", "", ""); !errors.Is(err, ErrGroupName) {
		t.Errorf("two local groups of one name: %v", err)
	}
	if err := db.SetGrant(ctx, folder.ID, admin, ToGroup, editors.ID, RoleEdit); err != nil {
		t.Fatal(err)
	}
	if got := role(t, db, page.ID, carl.ID); got != RoleComment {
		t.Errorf("not a member yet: %q", got)
	}
	if err := db.SetMember(ctx, Actor{ID: anna.ID, Admin: true}, editors.ID, carl.ID, true); err != nil {
		t.Fatal(err)
	}
	if got := role(t, db, page.ID, carl.ID); got != RoleEdit {
		t.Errorf("as a member of Editors carl edits, the highest counts: %q", got)
	}
	if members, err := db.GroupMembers(ctx, editors.ID); err != nil || len(members) != 1 || members[0].ID != carl.ID {
		t.Errorf("members: %+v %v", members, err)
	}
	if mine, _ := db.GroupsOf(ctx, carl.ID); len(mine) != 1 || mine[0] != editors.ID {
		t.Errorf("carl's groups: %v", mine)
	}
	if err := db.DeleteGroup(ctx, Actor{ID: anna.ID, Admin: true}, editors.ID); err != nil {
		t.Fatal(err)
	}
	if got := role(t, db, page.ID, carl.ID); got != RoleComment {
		t.Errorf("a deleted group takes its roles along: %q", got)
	}
	var left int
	if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM grants WHERE kind = 'group' AND who = $1", editors.ID).Scan(&left); err != nil || left != 0 {
		t.Errorf("grants of the deleted group: %d %v", left, err)
	}
	if err := db.SetGrant(ctx, page.ID, admin, ToPerson, carl.ID, RoleOwner); !errors.Is(err, ErrRole) {
		t.Errorf("nobody grants owner: %v", err)
	}
	if err := db.SetGrant(ctx, page.ID, admin, ToPerson, "nobody", RoleRead); !errors.Is(err, ErrNotFound) {
		t.Errorf("an account that is not there: %v", err)
	}
	if err := db.SetGrant(ctx, page.ID, Actor{ID: carl.ID}, ToPerson, carl.ID, RoleEdit); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("carl comments, he does not manage: %v", err)
	}
	if err := db.SetGrant(ctx, folder.ID, admin, ToPerson, carl.ID, RoleNone); err != nil {
		t.Fatal(err)
	}
	if got := role(t, db, page.ID, carl.ID); got != RoleRead {
		t.Errorf("taken back, the tenant's read is left: %q", got)
	}
}

// TestBreakingTheInheritance narrows by not taking from above: breaking copies what held, so nobody loses access by
// surprise, and the manager then takes back what shall not hold.
func TestBreakingTheInheritance(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.SeeAccount(ctx, carl); err != nil {
		t.Fatal(err)
	}
	team, _ := db.createShared(ctx, anna, "Team")
	page, _ := db.CreatePage(ctx, bob, "Salaries", team.ID)
	if err := db.SetGrant(ctx, team.ID, Actor{ID: anna.ID}, ToPerson, carl.ID, RoleComment); err != nil {
		t.Fatal(err)
	}
	if err := db.SetInherit(ctx, team.ID, Actor{ID: anna.ID}, false); !errors.Is(err, ErrPlace) {
		t.Errorf("a collection at the top has nothing above it: %v", err)
	}
	if err := db.SetInherit(ctx, page.ID, Actor{ID: carl.ID}, false); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("only who manages breaks it: %v", err)
	}
	if err := db.SetInherit(ctx, page.ID, Actor{ID: bob.ID}, false); err != nil {
		t.Fatal(err)
	}
	r, err := db.RightsOf(ctx, page.ID, Actor{ID: bob.ID})
	if err != nil || r.Inherit || len(r.Inherited) != 0 {
		t.Fatalf("the page takes nothing from above now: %+v %v", r, err)
	}
	held := map[string]string{}
	for _, g := range r.Own {
		held[g.Kind+":"+g.Who] = g.Role
	}
	if held["tenant:"] != RoleEdit || held["person:"+carl.ID] != RoleComment || held["person:"+anna.ID] != RoleManage || len(held) != 3 {
		t.Errorf("what held from above is copied, anna's managing as the owner of Team: %v", held)
	}
	for _, take := range [][2]string{{ToTenant, ""}, {ToPerson, anna.ID}} {
		if err := db.SetGrant(ctx, page.ID, Actor{ID: bob.ID}, take[0], take[1], RoleNone); err != nil {
			t.Fatal(err)
		}
	}
	if got := role(t, db, page.ID, anna.ID); got != RoleNone {
		t.Errorf("anna owns Team, but the page broke away: %q", got)
	}
	if _, err := db.Object(ctx, page.ID, anna.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("anna does not find it: %v", err)
	}
	if kids, _ := db.Children(ctx, team.ID, anna.ID); len(kids) != 0 {
		t.Errorf("nor in its collection: %v", titles(kids))
	}
	if got := role(t, db, page.ID, carl.ID); got != RoleComment {
		t.Errorf("carl keeps what was copied: %q", got)
	}
	// An administrator manages the rights without reading the page.
	boss := Actor{ID: anna.ID, Admin: true}
	if r, err := db.RightsOf(ctx, page.ID, boss); err != nil || r.Owner != bob.ID {
		t.Errorf("an administrator reads the rights: %+v %v", r, err)
	}
	if err := db.SetInherit(ctx, page.ID, boss, true); err != nil {
		t.Fatal(err)
	}
	if got := role(t, db, page.ID, anna.ID); got != RoleManage {
		t.Errorf("inheriting again, anna manages as the owner of Team: %q", got)
	}
}

// TestTransferringOwnership: the owner or an administrator gives an object away; the former owner keeps edit.
func TestTransferringOwnership(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.SeeAccount(ctx, carl); err != nil {
		t.Fatal(err)
	}
	team, _ := db.createShared(ctx, anna, "Team")
	if err := db.TransferOwner(ctx, team.ID, Actor{ID: bob.ID}, bob.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob may not take it: %v", err)
	}
	if err := db.TransferOwner(ctx, team.ID, Actor{ID: anna.ID}, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Errorf("to nobody: %v", err)
	}
	if err := db.TransferOwner(ctx, team.ID, Actor{ID: anna.ID}, bob.ID); err != nil {
		t.Fatal(err)
	}
	if got := role(t, db, team.ID, bob.ID); got != RoleOwner {
		t.Errorf("bob owns Team: %q", got)
	}
	if got := role(t, db, team.ID, anna.ID); got != RoleEdit {
		t.Errorf("anna keeps edit: %q", got)
	}
	if err := db.TransferOwner(ctx, team.ID, Actor{ID: carl.ID, Admin: true}, carl.ID); err != nil {
		t.Errorf("an administrator transfers: %v", err)
	}
	if got := role(t, db, team.ID, carl.ID); got != RoleOwner {
		t.Errorf("carl owns Team: %q", got)
	}
	notes, _ := db.Collections(ctx, anna.ID)
	if err := db.TransferOwner(ctx, notes[0].ID, Actor{ID: anna.ID}, bob.ID); !errors.Is(err, ErrPrivate) {
		t.Errorf("private notes are not given away: %v", err)
	}
	if err := db.SetGrant(ctx, notes[0].ID, Actor{ID: anna.ID}, ToPerson, bob.ID, RoleRead); !errors.Is(err, ErrPrivate) {
		t.Errorf("nor shared: %v", err)
	}
	if got := role(t, db, notes[0].ID, bob.ID); got != RoleNone {
		t.Errorf("notes stay private: %q", got)
	}
}

// TestTheLevelsOfADR20BecomeGrants: the migration turns each level for all others into grants, so nobody sees more or
// less than before; the writers of the folder of the tenant become a group with edit (ADR-29).
func TestTheLevelsOfADR20BecomeGrants(t *testing.T) {
	ctx := context.Background()
	url := pgtest.URL(t)
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, conn, 21); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `SELECT set_config('underdoc.tenant', 'main', false);
		INSERT INTO accounts (id, login, name, groups) VALUES ('u-anna', 'anna', 'Anna', '{}'), ('u-bob', 'bob', 'Bob', '{writers,ops}');
		INSERT INTO objects (id, kind, title, owner, parent, access, private, area) VALUES
			('aaaaaaaaa1', 'collection', 'Open', 'u-anna', NULL, NULL, false, 'docs'),
			('aaaaaaaaa2', 'page', 'Closed', 'u-anna', 'aaaaaaaaa1', 'read', false, 'docs'),
			('aaaaaaaaa3', 'page', 'Below closed', 'u-anna', 'aaaaaaaaa2', NULL, false, 'docs'),
			('aaaaaaaaa4', 'collection', 'Archive', 'u-anna', NULL, 'read', false, 'docs'),
			('aaaaaaaaa5', 'page', 'Comments welcome', 'u-anna', 'aaaaaaaaa4', 'comment', false, 'docs'),
			('aaaaaaaaa6', 'page', 'Same', 'u-anna', 'aaaaaaaaa4', 'read', false, 'docs'),
			('aaaaaaaaa7', 'collection', 'My notes', 'u-anna', NULL, 'read', true, 'docs'),
			('aaaaaaaaa8', 'line', 'line', 'u-anna', 'aaaaaaaaa1', NULL, false, 'docs'),
			('aaaaaaaaa9', 'collection', 'Company', 'u-anna', NULL, 'read', false, 'files');
		INSERT INTO file_roots (object, role, writers) VALUES ('aaaaaaaaa9', 'tenant', 'writers')`); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close(ctx)
	db, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	main := inMain()
	for objectID, want := range map[string]string{"aaaaaaaaa1": RoleEdit, "aaaaaaaaa2": RoleRead, "aaaaaaaaa3": RoleRead, "aaaaaaaaa4": RoleRead,
		"aaaaaaaaa5": RoleComment, "aaaaaaaaa6": RoleRead, "aaaaaaaaa7": RoleNone, "aaaaaaaaa9": RoleEdit} {
		if got := role(t, db, objectID, "u-bob"); got != want {
			t.Errorf("%s: bob holds %q, want %q", objectID, got, want)
		}
	}
	if got := role(t, db, "aaaaaaaaa7", "u-anna"); got != RoleOwner {
		t.Errorf("anna's notes: %q", got)
	}
	var grants, broken int
	if err := db.pool.QueryRow(main, "SELECT count(DISTINCT (object, kind, who)) FROM grants").Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if err := db.pool.QueryRow(main, "SELECT count(*) FROM objects WHERE NOT inherit").Scan(&broken); err != nil {
		t.Fatal(err)
	}
	// Open (edit), Closed (read), Archive (read), Comments welcome (comment), Company (read) and its writers (edit).
	if grants != 6 || broken != 1 {
		t.Errorf("%d grants and %d objects that break the inheritance, want 6 and 1", grants, broken)
	}
	groups, err := db.Groups(main)
	if err != nil || len(groups) != 2 {
		t.Fatalf("writers and ops are groups taken over: %+v %v", groups, err)
	}
	if roots, err := db.FileRoots(main, "u-bob"); err != nil || len(roots) != 1 || roots[0].Writers != "writers" {
		t.Errorf("the folder of the tenant names its writers: %+v %v", roots, err)
	}
}

// TestReadersFollowTheRoles: the search index keeps who may read as tokens of persons, groups and the tenant, the same
// rule as object_role.
func TestReadersFollowTheRoles(t *testing.T) {
	db := open(t)
	ctx := inMain()
	team, _ := db.createShared(ctx, anna, "Team")
	page, _ := db.CreatePage(ctx, bob, "Plan", team.ID)
	readers := func() []string { return flatReaders(t, db, page.ID) }
	if got := readers(); !slices.Equal(got, []string{"account:" + anna.ID, "account:" + bob.ID, "all"}) {
		t.Errorf("the owners and the tenant: %v", got)
	}
	board, _ := db.CreateGroup(ctx, Actor{ID: anna.ID, Admin: true}, "Board", "", "")
	if err := db.SetInherit(ctx, page.ID, Actor{ID: bob.ID}, false); err != nil {
		t.Fatal(err)
	}
	for _, take := range [][2]string{{ToTenant, ""}, {ToPerson, anna.ID}} {
		if err := db.SetGrant(ctx, page.ID, Actor{ID: bob.ID}, take[0], take[1], RoleNone); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.pool.Exec(ctx, "DELETE FROM search_queue"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGrant(ctx, page.ID, Actor{ID: bob.ID}, ToGroup, board.ID, RoleRead); err != nil {
		t.Fatal(err)
	}
	if got := readers(); !slices.Equal(got, []string{"account:" + bob.ID, "group:" + board.ID}) {
		t.Errorf("broken away, only bob and the board: %v", got)
	}
	var queued bool
	if err := db.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM search_queue WHERE key = $1)", page.ID).Scan(&queued); err != nil || !queued {
		t.Errorf("a change of roles queues the page for the index: %v %v", queued, err)
	}
	notes, _ := db.Collections(ctx, anna.ID)
	if got := flatReaders(t, db, notes[0].ID); !slices.Equal(got, []string{"account:" + anna.ID}) {
		t.Errorf("private notes are read by their owner alone: %v", got)
	}
}

// flatReaders is every token the index allows to read an object, without its levels: for objects without a deny.
func flatReaders(t *testing.T, db *DB, objectID string) []string {
	t.Helper()
	r, err := db.readAccess(inMain(), objectID)
	if err != nil {
		t.Fatal(err)
	}
	got := slices.Clone(r.always)
	for _, l := range r.levels {
		if len(l.Deny) > 0 {
			t.Fatalf("%s denies: %+v", objectID, r.levels)
		}
		got = append(got, l.Allow...)
	}
	slices.Sort(got)
	return slices.Compact(got)
}

// TestTakingBackAnAdministrator: an administrator makes a local account an administrator and takes that back again; the
// nobody takes their own away, so a tenant keeps one, and accounts of the gate are administrators by their group.
func TestTakingBackAnAdministrator(t *testing.T) {
	db := open(t)
	ctx := inMain()
	local := func(login string, admin bool) Account {
		hash := []byte(login + strings.Repeat("x", 32-len(login)))
		if err := db.CreateInvite(ctx, hash, Invite{Login: login, Name: login, Admin: admin, Expires: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
		a, err := db.AcceptInvite(ctx, hash, NewLocal{PasswordHash: "x"})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	dora, eve := local("dora", true), local("eve", false)
	boss := Actor{ID: dora.ID, Admin: true}
	if err := db.SetAdmin(ctx, Actor{ID: eve.ID}, dora.ID, false); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("eve does not administer: %v", err)
	}
	if err := db.SetAdmin(ctx, boss, dora.ID, false); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("dora takes her own away: %v", err)
	}
	if err := db.SetAdmin(ctx, boss, anna.ID, true); !errors.Is(err, ErrSource) {
		t.Errorf("anna comes through the gate: %v", err)
	}
	if err := db.SetAdmin(ctx, boss, eve.ID, true); err != nil {
		t.Fatal(err)
	}
	if a, _ := db.AccountByID(ctx, eve.ID); !a.Admin {
		t.Error("eve administers now")
	}
	if err := db.SetAdmin(ctx, Actor{ID: eve.ID, Admin: true}, dora.ID, false); err != nil {
		t.Fatal(err)
	}
	if a, _ := db.AccountByID(ctx, dora.ID); a.Admin {
		t.Error("dora no longer administers")
	}
	if err := db.SetAdmin(ctx, Actor{ID: eve.ID, Admin: true}, eve.ID, false); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("eve takes her own away: %v", err)
	}
	people, err := db.Everyone(ctx)
	if err != nil || len(people) != 4 {
		t.Errorf("everyone: %+v %v", people, err)
	}
}

// TestMigrationKeepsWritersInNarrowerFolders: under ADR-20 the group of writers edited everywhere below the folder of the
// tenant, whatever level a folder set. Migration 022 broke the inheritance of a narrower folder and copied nothing onto
// it; migration 025 gives the writers edit there again. A break someone made since stays as it is.
func TestMigrationKeepsWritersInNarrowerFolders(t *testing.T) {
	ctx := context.Background()
	url := pgtest.URL(t)
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, conn, 21); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `SELECT set_config('underdoc.tenant', 'main', false);
		INSERT INTO accounts (id, login, name, groups) VALUES ('u-anna', 'anna', 'Anna', '{}'), ('u-bob', 'bob', 'Bob', '{writers}');
		INSERT INTO objects (id, kind, title, owner, parent, access, private, area) VALUES
			('bbbbbbbbb1', 'collection', 'Company', 'u-anna', NULL, 'comment', false, 'files'),
			('bbbbbbbbb2', 'folder', 'Contracts', 'u-anna', 'bbbbbbbbb1', 'read', false, 'files'),
			('bbbbbbbbb3', 'folder', 'Signed', 'u-anna', 'bbbbbbbbb2', NULL, false, 'files');
		INSERT INTO file_roots (object, role, writers) VALUES ('bbbbbbbbb1', 'tenant', 'writers')`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, conn, 24); err != nil {
		t.Fatal(err)
	}
	// After 022, anna shuts the writers out of a folder of her own making: a break with a granter.
	if _, err := conn.Exec(ctx, `INSERT INTO objects (id, kind, title, owner, parent, private, area, inherit) VALUES
			('bbbbbbbbb4', 'folder', 'Board', 'u-anna', 'bbbbbbbbb1', false, 'files', false);
		INSERT INTO grants (object, kind, who, role, granted_by) VALUES ('bbbbbbbbb4', 'tenant', '', 'read', 'u-anna')`); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close(ctx)
	db, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for id, want := range map[string]string{"bbbbbbbbb1": RoleEdit, "bbbbbbbbb2": RoleEdit, "bbbbbbbbb3": RoleEdit, "bbbbbbbbb4": RoleRead} {
		if got := role(t, db, id, "u-bob"); got != want {
			t.Errorf("bob, a writer, holds %q on %s, want %q", got, id, want)
		}
	}
}

// TestTwoAdministratorsCannotTakeEachOtherAway: two requests at the same time each found their actor an administrator
// at the start. The database decides under a lock, so the second, whose actor no longer administers, takes nothing away
// and the tenant keeps a local administrator. The stale actor here stands for that window.
func TestTwoAdministratorsCannotTakeEachOtherAway(t *testing.T) {
	db := open(t)
	ctx := inMain()
	local := func(login string) Account {
		hash := []byte(login + strings.Repeat("x", 32-len(login)))
		if err := db.CreateInvite(ctx, hash, Invite{Login: login, Name: login, Admin: true, Expires: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
		a, err := db.AcceptInvite(ctx, hash, NewLocal{PasswordHash: "x"})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	dora, eve := local("dora"), local("eve")
	if err := db.SetAdmin(ctx, Actor{ID: dora.ID, Admin: true}, eve.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAdmin(ctx, Actor{ID: eve.ID, Admin: true}, dora.ID, false); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("eve, no longer an administrator: %v", err)
	}
	var left int
	if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM accounts WHERE admin").Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 {
		t.Errorf("%d local administrators are left", left)
	}
	// Both at once: the request of dora has taken eve's administration and not yet ended when eve's begins. Eve's
	// waits for it, and then finds eve no longer an administrator.
	if err := db.SetAdmin(ctx, Actor{ID: dora.ID, Admin: true}, eve.ID, true); err != nil {
		t.Fatal(err)
	}
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockAdmins); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "UPDATE accounts SET admin = false WHERE id = $1", eve.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- db.SetAdmin(ctx, Actor{ID: eve.ID, Admin: true}, dora.ID, false) }()
	var eves error
	select {
	case eves = <-done:
		t.Errorf("eve's request did not wait for dora's: %v", eves)
	case <-time.After(300 * time.Millisecond):
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		eves = <-done
	}
	if !errors.Is(eves, ErrNotAllowed) {
		t.Errorf("eve, no longer an administrator once dora's request ended: %v", eves)
	}
	if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM accounts WHERE admin").Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 1 {
		t.Errorf("at once: %d administrators left", left)
	}
}
