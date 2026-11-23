// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"slices"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// The attack round of the review of B-3 (B-4) in the database: service accounts and spaces, archiving one's own group
// through a parent, what HeldBy lists, archived groups as readers of passwords, and allow groups of a service account.

// serviceAccount binds an allowed service account and marks it as one.
func serviceAccount(t *testing.T, db *DB) account.User {
	t.Helper()
	ctx := inMain()
	svc := account.User{ID: "42", RemoteID: "42", Login: "export-bot", Name: "Export", Groups: []string{"ops"},
		AuthKind: account.AuthAPIKey, Kind: account.KindService}
	bound, err := db.BindGate(ctx, svc)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetServiceAccounts(ctx, []string{"42"}); err != nil {
		t.Fatal(err)
	}
	svc.ID = bound.ID
	return svc
}

// Attack: a service account that carries the administrator's mark and the role to create spaces still makes no space
// (B-4, finding 1): the database refuses it whatever reached it.
func TestServiceAccountsMakeNoSpace(t *testing.T) {
	db, ctx := setup(t)
	svc := serviceAccount(t, db)
	svc.Admin, svc.Spaces = true, true
	asSvc := account.With(ctx, svc)
	if _, err := db.CreateCollection(asSvc, svc, "By the bot", ""); !errors.Is(err, ErrSpaces) {
		t.Errorf("a service account with the roles makes a space: %v", err)
	}
	if _, err := db.CreateFileCollection(asSvc, svc, "Files of the bot"); !errors.Is(err, ErrSpaces) {
		t.Errorf("a service account with the roles makes a space of files: %v", err)
	}
}

// Attack: the admin of a group who also administers another puts the group inside it, archives it as the admin of the
// group above, and takes it out again (B-4, finding 2). The admin of a group never archives or restores it.
func TestOwnAdminsNeverArchiveThroughAParent(t *testing.T) {
	db, ctx := setup(t)
	g, _ := db.CreateGroup(ctx, admin, "Support", "", "")
	k, _ := db.CreateGroup(ctx, admin, "Helpers", "", "")
	for _, step := range []error{
		db.SetGroupAdmin(ctx, admin, g.ID, carl.ID, true),
		db.SetGroupAdmin(ctx, admin, k.ID, carl.ID, true),
	} {
		if step != nil {
			t.Fatal(step)
		}
	}
	if err := db.SetSubgroup(ctx, Actor{ID: carl.ID}, k.ID, g.ID, true); err != nil {
		t.Fatalf("carl puts Support inside Helpers: %v", err)
	}
	if err := db.ArchiveGroup(ctx, Actor{ID: carl.ID}, g.ID); !errors.Is(err, ErrGroupArchiver) {
		t.Errorf("carl archives his own Support as the admin of Helpers: %v", err)
	}
	if ok, err := db.MayArchiveGroup(ctx, Actor{ID: carl.ID}, g.ID); ok || err != nil {
		t.Errorf("Support is offered to carl to archive: %v %v", ok, err)
	}
	// Dora administers only the group above: she may.
	if err := db.SetGroupAdmin(ctx, admin, k.ID, dora.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := db.ArchiveGroup(ctx, Actor{ID: dora.ID}, g.ID); err != nil {
		t.Errorf("the admin of the group above alone: %v", err)
	}
	if err := db.RestoreGroup(ctx, Actor{ID: carl.ID}, g.ID); !errors.Is(err, ErrGroupArchiver) {
		t.Errorf("carl restores his own Support: %v", err)
	}
}

// Attack: what may X do lists entries that do not hold for X (B-4, finding 6): an allow through an archived group, and
// for a service account an allow for the tenant or a role of the gate. Their denies hold and stay listed.
func TestHeldByListsWhatHolds(t *testing.T) {
	db, ctx := setup(t)
	svc := serviceAccount(t, db)
	ops, err := db.GroupsOf(ctx, anna.ID)
	if err != nil || len(ops) != 1 {
		t.Fatalf("the role ops: %v %v", ops, err)
	}
	sales, _ := db.CreateGroup(ctx, admin, "Sales", "", "")
	if err := db.SetMember(ctx, admin, sales.ID, bob.ID, true); err != nil {
		t.Fatal(err)
	}
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	allowed, _ := db.CreatePage(ctx, anna, "For sales", team.ID)
	mixed, _ := db.CreatePage(ctx, anna, "Mixed", team.ID)
	byRole, _ := db.CreatePage(ctx, anna, "For ops", team.ID)
	for _, step := range []error{
		db.SetGrant(ctx, team.ID, admin, ToTenant, "", RoleRead),
		db.SetEntries(ctx, allowed.ID, admin, ToGroup, sales.ID, entries(ActWrite, Allow), true),
		db.SetEntries(ctx, mixed.ID, admin, ToGroup, sales.ID, entries(ActWrite, Allow, ActComment, Deny), true),
		db.SetEntries(ctx, byRole.ID, admin, ToGroup, ops[0], entries(ActWrite, Allow, ActComment, Deny), true),
		db.ArchiveGroup(ctx, admin, sales.ID),
	} {
		if step != nil {
			t.Fatal(step)
		}
	}
	heldOf := func(accountID string) map[string]Held {
		t.Helper()
		list, err := db.HeldBy(ctx, admin, accountID)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]Held{}
		for _, h := range list {
			out[h.Object+" "+h.Via] = h
		}
		return out
	}
	bobs := heldOf(bob.ID)
	if _, ok := bobs[allowed.ID+" "+ToGroup]; ok {
		t.Errorf("an allow through archived Sales is listed for bob: %+v", bobs)
	}
	if h, ok := bobs[mixed.ID+" "+ToGroup]; !ok || h.Actions[ActComment] != Deny || h.Actions[ActWrite] != "" {
		t.Errorf("the deny of archived Sales stays, its allow goes: %+v", h)
	}
	if _, ok := bobs[team.ID+" "+ToTenant]; !ok {
		t.Errorf("bob reads Team as everyone: %+v", bobs)
	}
	svcs := heldOf(svc.ID)
	if _, ok := svcs[team.ID+" "+ToTenant]; ok {
		t.Errorf("the allow of the tenant is listed for the service account: %+v", svcs)
	}
	if h, ok := svcs[byRole.ID+" "+ToGroup]; !ok || h.Actions[ActComment] != Deny || h.Actions[ActWrite] != "" {
		t.Errorf("for the service account the deny of the role stays, its allow goes: %+v", h)
	}
	if h, ok := heldOf(anna.ID)[byRole.ID+" "+ToGroup]; !ok || h.Actions[ActWrite] != Allow {
		t.Errorf("for anna the allow of the role holds: %+v", h)
	}
}

// Attack: an archived group made a reader of a password (B-4, finding 7). It gets no new place, as it gets no new
// entry; one that reads already stays while the list is written anew.
func TestArchivedGroupsBecomeNoReaders(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "Router", team.ID)
	secretID, err := db.CreatePageSecret(ctx, page.ID, anna.ID, fakeSeal("s3cret"))
	if err != nil {
		t.Fatal(err)
	}
	old, _ := db.CreateGroup(ctx, admin, "Old", "", "")
	gone, _ := db.CreateGroup(ctx, admin, "Gone", "", "")
	if err := db.SetPageSecretReaders(ctx, secretID, anna.ID, true, []SecretReader{{Kind: "group", Who: old.ID}}); err != nil {
		t.Fatal(err)
	}
	for _, g := range []Group{old, gone} {
		if err := db.ArchiveGroup(ctx, admin, g.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SetPageSecretReaders(ctx, secretID, anna.ID, true, []SecretReader{{Kind: "group", Who: gone.ID}}); !errors.Is(err, ErrReader) {
		t.Errorf("an archived group becomes a reader: %v", err)
	}
	if err := db.SetPageSecretReaders(ctx, secretID, anna.ID, true, []SecretReader{{Kind: "group", Who: old.ID}, {Kind: "person", Who: bob.ID}}); err != nil {
		t.Errorf("an archived reader stays while the list is written anew: %v", err)
	}
}

// A service account gets the allow of a local group it is put in directly and of the local groups that group is inside,
// as anyone; never of a role of the gate, nor of a local group it reaches only through one (B-4, finding 9; ADR-36).
func TestServiceAccountsThroughLocalParents(t *testing.T) {
	db, ctx := setup(t)
	svc := serviceAccount(t, db)
	asSvc := account.With(ctx, svc)
	ops, _ := db.GroupsOf(ctx, anna.ID)
	bots, _ := db.CreateGroup(ctx, admin, "Bots", "", "")
	machines, _ := db.CreateGroup(ctx, admin, "Machines", "", "")
	staff, _ := db.CreateGroup(ctx, admin, "Staff", "", "")
	for _, step := range []error{
		db.SetMember(ctx, admin, bots.ID, svc.ID, true),
		db.SetSubgroup(ctx, admin, machines.ID, bots.ID, true),
		db.SetSubgroup(ctx, admin, staff.ID, ops[0], true),
	} {
		if step != nil {
			t.Fatal(step)
		}
	}
	byMachines, _ := db.CreateCollection(ctx, anna, "For machines", "")
	byStaff, _ := db.CreateCollection(ctx, anna, "For staff", "")
	for _, step := range []error{
		db.SetGrant(ctx, byMachines.ID, admin, ToGroup, machines.ID, RoleRead),
		db.SetGrant(ctx, byStaff.ID, admin, ToGroup, staff.ID, RoleRead),
	} {
		if step != nil {
			t.Fatal(step)
		}
	}
	if !may(t, db, asSvc, byMachines.ID, svc.ID, ActRead) {
		t.Error("the service account reads through Machines, the local group Bots is inside")
	}
	if may(t, db, asSvc, byStaff.ID, svc.ID, ActRead) {
		t.Error("the service account reads through Staff, reached only through the role ops")
	}
	allow, err := db.AllowGroupsOf(ctx, svc.ID)
	if err != nil || !slices.Contains(allow, machines.ID) || slices.Contains(allow, staff.ID) || slices.Contains(allow, ops[0]) {
		t.Errorf("the allow groups of the service account: %v %v", allow, err)
	}
}
