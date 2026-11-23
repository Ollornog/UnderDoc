// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// The review of rights and sign-in (B-3): archived groups, service accounts and the role to create spaces. Each test
// is also the attack round on the door it changes.

// TestArchivingAGroup: a group is archived instead of deleted, only by an administrator or the admin of a group it is
// inside, only without groups inside that are not archived; its allow entries hold for nobody while it is archived, its
// deny entries keep holding, and restoring brings everything back.
func TestArchivingAGroup(t *testing.T) {
	db, ctx := setup(t)
	company, _ := db.CreateGroup(ctx, admin, "Company", "", "")
	sales, _ := db.CreateGroup(ctx, admin, "Sales", "", company.ID)
	inner, _ := db.CreateGroup(ctx, admin, "Key accounts", "", sales.ID)
	for _, step := range []error{
		db.SetGroupAdmin(ctx, admin, company.ID, dora.ID, true),
		db.SetGroupAdmin(ctx, admin, sales.ID, carl.ID, true),
		db.SetMember(ctx, admin, sales.ID, bob.ID, true),
	} {
		if step != nil {
			t.Fatal(step)
		}
	}
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	allowed, _ := db.CreatePage(ctx, anna, "For sales", team.ID)
	denied, _ := db.CreatePage(ctx, anna, "Not for sales", team.ID)
	above, _ := db.CreatePage(ctx, anna, "For the company", team.ID)
	for _, step := range []error{
		db.SetGrant(ctx, team.ID, admin, ToTenant, "", RoleRead),
		db.SetEntries(ctx, allowed.ID, admin, ToGroup, sales.ID, entries(ActRead, Allow, ActWrite, Allow), true),
		db.SetEntries(ctx, denied.ID, admin, ToGroup, sales.ID, entries(ActRead, Deny), true),
		db.SetEntries(ctx, above.ID, admin, ToGroup, company.ID, entries(ActWrite, Allow, ActComment, Deny), true),
	} {
		if step != nil {
			t.Fatal(step)
		}
	}
	check := func(when string, wantWrite, wantAbove bool) {
		t.Helper()
		if got := may(t, db, ctx, allowed.ID, bob.ID, ActWrite); got != wantWrite {
			t.Errorf("%s: bob writes through Sales: %v", when, got)
		}
		if may(t, db, ctx, denied.ID, bob.ID, ActRead) {
			t.Errorf("%s: the deny of Sales lets bob read", when)
		}
		if got := may(t, db, ctx, above.ID, bob.ID, ActWrite); got != wantAbove {
			t.Errorf("%s: bob writes through Company, which Sales is inside: %v", when, got)
		}
		if may(t, db, ctx, above.ID, bob.ID, ActComment) {
			t.Errorf("%s: the deny of Company lets bob comment", when)
		}
		if !may(t, db, ctx, allowed.ID, bob.ID, ActRead) {
			t.Errorf("%s: bob reads as everyone does", when)
		}
	}
	check("before", true, true)

	// Attacks: the admin of Sales archives their own group or the group above it; an agent of the admin above; a group
	// from the sign-in; a group with a group inside that is not archived.
	if err := db.ArchiveGroup(ctx, Actor{ID: carl.ID}, sales.ID); !errors.Is(err, ErrGroupArchiver) {
		t.Errorf("the admin of Sales archives Sales: %v", err)
	}
	if err := db.ArchiveGroup(ctx, Actor{ID: carl.ID}, company.ID); !errors.Is(err, ErrGroupArchiver) {
		t.Errorf("the admin of Sales archives Company: %v", err)
	}
	if err := db.ArchiveGroup(ctx, Actor{ID: dora.ID}, company.ID); !errors.Is(err, ErrGroupArchiver) {
		t.Errorf("the admin of Company archives Company: %v", err)
	}
	if err := db.ArchiveGroup(asAgent(dora), Actor{ID: dora.ID}, inner.ID); !errors.Is(err, ErrAgent) {
		t.Errorf("an agent archives: %v", err)
	}
	if err := db.ArchiveGroup(ctx, Actor{ID: dora.ID}, sales.ID); !errors.Is(err, ErrGroupHasGroups) {
		t.Errorf("Sales is archived with Key accounts inside: %v", err)
	}
	synced, err := db.GroupsOf(ctx, anna.ID)
	if err != nil || len(synced) != 1 {
		t.Fatalf("anna's group from the sign-in: %v %v", synced, err)
	}
	if err := db.ArchiveGroup(ctx, admin, synced[0]); !errors.Is(err, ErrGroup) {
		t.Errorf("a group from the sign-in is archived: %v", err)
	}
	if ok, _ := db.MayArchiveGroup(ctx, Actor{ID: carl.ID}, sales.ID); ok {
		t.Error("the list offers the admin of Sales to archive it")
	}

	// The admin of Company archives what lies inside it, the inner group first.
	if err := db.ArchiveGroup(ctx, Actor{ID: dora.ID}, inner.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.ArchiveGroup(ctx, Actor{ID: dora.ID}, sales.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.ArchiveGroup(ctx, Actor{ID: dora.ID}, sales.ID); !errors.Is(err, ErrGroupArchived) {
		t.Errorf("archived twice: %v", err)
	}
	check("archived", false, false)
	if g, _ := db.GroupOf(ctx, sales.ID); !g.Archived || g.Members != 1 {
		t.Errorf("Sales keeps its member: %+v", g)
	}
	// Archived, the group is not changed and gets no new entries; it holds no admin's rights.
	if err := db.SetMember(ctx, admin, sales.ID, dora.ID, true); !errors.Is(err, ErrGroupArchived) {
		t.Errorf("a member is added to an archived group: %v", err)
	}
	if err := db.SetEntries(ctx, team.ID, admin, ToGroup, sales.ID, entries(ActRead, Allow), true); !errors.Is(err, ErrNotFound) {
		t.Errorf("an archived group gets an entry: %v", err)
	}
	if err := db.SetSubgroup(ctx, admin, company.ID, inner.ID, true); !errors.Is(err, ErrGroupArchived) {
		t.Errorf("an archived group is put inside another: %v", err)
	}
	if _, err := db.CreateGroup(ctx, admin, "Inside archived", "", sales.ID); !errors.Is(err, ErrGroupArchived) {
		t.Errorf("a group is made inside an archived one: %v", err)
	}
	if mine, _ := db.AdministeredGroups(ctx, carl.ID); len(mine) != 0 {
		t.Errorf("the admin of an archived group manages %v", mine)
	}
	if got, _ := db.AllowGroupsOf(ctx, bob.ID); len(got) != 0 {
		t.Errorf("bob's groups whose allow holds: %v", got)
	}
	if got, _ := db.GroupsOf(ctx, bob.ID); !slices.Contains(got, sales.ID) || !slices.Contains(got, company.ID) {
		t.Errorf("bob's groups for a deny keep the archived one and those above it: %v", got)
	}

	// Restoring: not inside an archived group, and only by the same people.
	if err := db.RestoreGroup(ctx, admin, inner.ID); !errors.Is(err, ErrGroupArchived) {
		t.Errorf("Key accounts comes back inside archived Sales: %v", err)
	}
	if err := db.RestoreGroup(ctx, Actor{ID: dora.ID}, inner.ID); !errors.Is(err, ErrGroupArchiver) {
		t.Errorf("the admin of Company reaches Key accounts through archived Sales: %v", err)
	}
	if err := db.RestoreGroup(ctx, Actor{ID: carl.ID}, sales.ID); !errors.Is(err, ErrGroupArchiver) {
		t.Errorf("the admin of Sales restores it: %v", err)
	}
	if err := db.RestoreGroup(ctx, admin, company.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("restoring a group that is not archived: %v", err)
	}
	if err := db.RestoreGroup(ctx, Actor{ID: dora.ID}, sales.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.RestoreGroup(ctx, Actor{ID: dora.ID}, inner.ID); err != nil {
		t.Fatal(err)
	}
	check("restored", true, true)
	if err := db.SetMember(ctx, Actor{ID: carl.ID}, sales.ID, dora.ID, true); err != nil {
		t.Errorf("restored, its admin manages it again: %v", err)
	}
}

// TestServiceAccounts: an allowed service account is an agent whatever the caller says, and never belongs to everyone
// signed in or to a role of the gate, which may mean everyone too; a deny for those still holds for it. It gets what is
// granted to it and to a local group it is put in (B-3).
func TestServiceAccounts(t *testing.T) {
	db, ctx := setup(t)
	svc := account.User{ID: "42", RemoteID: "42", Login: "export-bot", Name: "Export", Groups: []string{"ops"},
		AuthKind: account.AuthAPIKey, Kind: account.KindService}
	bound, err := db.BindGate(ctx, svc)
	if err != nil {
		t.Fatal(err)
	}
	svc.ID = bound.ID
	asSvc := account.With(ctx, svc)
	ops, err := db.GroupsOf(ctx, anna.ID)
	if err != nil || len(ops) != 1 {
		t.Fatalf("the role ops: %v %v", ops, err)
	}
	bots, _ := db.CreateGroup(ctx, admin, "Bots", "", "")
	staff, _ := db.CreateGroup(ctx, admin, "Staff", "", "")
	for _, step := range []error{
		db.SetMember(ctx, admin, bots.ID, svc.ID, true),
		db.SetSubgroup(ctx, admin, staff.ID, ops[0], true),
	} {
		if step != nil {
			t.Fatal(step)
		}
	}
	everyone, _ := db.CreateCollection(ctx, anna, "Everyone", "")
	byRole, _ := db.CreateCollection(ctx, anna, "By role", "")
	byStaff, _ := db.CreateCollection(ctx, anna, "By staff", "")
	byBots, _ := db.CreateCollection(ctx, anna, "By bots", "")
	mine, _ := db.CreateCollection(ctx, anna, "Granted to it", "")
	noTenant, _ := db.CreatePage(ctx, anna, "Denied to everyone", mine.ID)
	noRole, _ := db.CreatePage(ctx, anna, "Denied to the role", mine.ID)
	for _, step := range []error{
		db.SetGrant(ctx, everyone.ID, admin, ToTenant, "", RoleEdit),
		db.SetGrant(ctx, byRole.ID, admin, ToGroup, ops[0], RoleEdit),
		db.SetGrant(ctx, byStaff.ID, admin, ToGroup, staff.ID, RoleEdit),
		db.SetGrant(ctx, byBots.ID, admin, ToGroup, bots.ID, RoleRead),
		db.SetGrant(ctx, mine.ID, admin, ToPerson, svc.ID, RoleManage),
		db.SetEntries(ctx, noTenant.ID, admin, ToTenant, "", entries(ActRead, Deny), true),
		db.SetEntries(ctx, noRole.ID, admin, ToGroup, ops[0], entries(ActRead, Deny), true),
	} {
		if step != nil {
			t.Fatal(step)
		}
	}
	for _, c := range []struct {
		object string
		want   bool
	}{
		{everyone.ID, false}, {byRole.ID, false}, {byStaff.ID, false}, {byBots.ID, true}, {mine.ID, true},
		{noTenant.ID, false}, {noRole.ID, false},
	} {
		if got := may(t, db, asSvc, c.object, svc.ID, ActRead); got != c.want {
			t.Errorf("the service account reads %s: %v, want %v", c.object, got, c.want)
		}
		// The same as a person would ask it, or without a request at all, as the share does: it is an agent anyway.
		if d, err := db.Decide(ctx, c.object, svc.ID, ActRead, false); err != nil || d.Allowed != c.want {
			t.Errorf("decided as no agent, %s: %+v %v", c.object, d, err)
		}
		if got := may(t, db, ctx, c.object, anna.ID, ActRead); !got {
			t.Errorf("anna reads %s", c.object)
		}
	}
	// An agent never manages or shares, also where it is granted manage, and also where a caller asks as if it were none.
	if d, err := db.Decide(ctx, mine.ID, svc.ID, ActManage, false); err != nil || d.Allowed || d.Via != ViaAgentsDefault {
		t.Errorf("asked as no agent, the service account manages: %+v %v", d, err)
	}
	if may(t, db, asSvc, mine.ID, svc.ID, ActManage) || may(t, db, asSvc, mine.ID, svc.ID, ActShare) {
		t.Error("the service account manages or shares")
	}
	var held []string
	if err := db.pool.QueryRow(ctx, "SELECT object_actions($1, $2, false, true)", everyone.ID, svc.ID).Scan(&held); err != nil || len(held) != 0 {
		t.Errorf("the share gives the service account %v %v", held, err)
	}
	// Every door that lists or reads asks the same check: the tree, export.
	tops, _ := db.Collections(asSvc, svc.ID)
	for _, o := range tops {
		if o.ID == everyone.ID || o.ID == byRole.ID {
			t.Errorf("the tree shows %s", o.Title)
		}
	}
	if _, err := db.Subtree(asSvc, everyone.ID, svc.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the export of what everyone reads: %v", err)
	}
	if _, err := db.Subtree(asSvc, mine.ID, svc.ID); err != nil {
		t.Errorf("the export of what is granted to it: %v", err)
	}
	// The mark follows the allowed IDs; without it the account is an ordinary one again.
	if err := db.SetServiceAccounts(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if !may(t, db, ctx, everyone.ID, svc.ID, ActRead) {
		t.Error("unmarked, the account reads as everyone")
	}
	if err := db.SetServiceAccounts(ctx, []string{"42"}); err != nil {
		t.Fatal(err)
	}
	if may(t, db, ctx, everyone.ID, svc.ID, ActRead) {
		t.Error("marked again, the account reads as everyone")
	}
}

// TestOnlyHoldersOfTheRoleMakeSpaces: a collection at the top, made, copied or moved there, needs an administrator or
// the role of the gate to create spaces; inside, the rights decide (ADR-35, B-3).
func TestOnlyHoldersOfTheRoleMakeSpaces(t *testing.T) {
	db, ctx := setup(t)
	person := func(u account.User) context.Context {
		u.AuthKind = account.AuthSession
		return account.With(ctx, u)
	}
	withRole := bob
	withRole.Spaces = true
	asAdmin := carl
	asAdmin.Admin = true
	team, _ := db.createShared(ctx, anna, "Team")
	sub, _ := db.CreateCollection(ctx, anna, "Projects", team.ID)
	if _, err := db.CreateCollection(person(bob), bob, "Mine", ""); !errors.Is(err, ErrSpaces) {
		t.Errorf("bob makes a space without the role: %v", err)
	}
	if _, err := db.CreateCollection(person(bob), anna, "In anna's name", ""); !errors.Is(err, ErrSpaces) {
		t.Errorf("bob makes a space in the name of another: %v", err)
	}
	if _, err := db.CreateFileCollection(person(bob), bob, "Files"); !errors.Is(err, ErrSpaces) {
		t.Errorf("bob makes a space of files without the role: %v", err)
	}
	if _, err := db.Copy(person(bob), team.ID, bob.ID, "", ""); !errors.Is(err, ErrSpaces) {
		t.Errorf("bob copies a space to the top without the role: %v", err)
	}
	if _, err := db.Move(person(bob), sub.ID, bob.ID, "", ""); !errors.Is(err, ErrSpaces) {
		t.Errorf("bob moves a collection to the top without the role: %v", err)
	}
	if _, err := db.CreateCollection(person(bob), bob, "Inside", team.ID); err != nil {
		t.Errorf("inside, bob's rights decide: %v", err)
	}
	if _, err := db.CreateCollection(person(withRole), withRole, "Bob's space", ""); err != nil {
		t.Errorf("with the role: %v", err)
	}
	if _, err := db.Move(person(withRole), sub.ID, withRole.ID, "", ""); err != nil {
		t.Errorf("with the role, moved to the top: %v", err)
	}
	if _, err := db.CreateCollection(person(asAdmin), asAdmin, "Carl's space", ""); err != nil {
		t.Errorf("an administrator: %v", err)
	}
	if _, err := db.CreateFileCollection(asAgent(withRole), withRole, "Agent's files"); err != nil {
		t.Errorf("an agent of a holder of the role: %v", err)
	}
	// Reordering at the top moves no collection up.
	if _, err := db.Move(person(anna), team.ID, anna.ID, "", ""); err != nil {
		t.Errorf("anna reorders her space at the top: %v", err)
	}
	// Without a request, whoever runs the instance acts: the command line, an import.
	if _, err := db.CreateCollection(ctx, bob, "From the command line", ""); err != nil {
		t.Errorf("without a request: %v", err)
	}
}

// TestArchivedGroupTrigger: the database itself keeps archived groups out of groups inside groups, so a group put inside
// while another request archives cannot slip past the check of the application (B-3).
func TestArchivedGroupTrigger(t *testing.T) {
	db, ctx := setup(t)
	outer, _ := db.CreateGroup(ctx, admin, "Outer", "", "")
	inner, _ := db.CreateGroup(ctx, admin, "Inner", "", "")
	if err := db.ArchiveGroup(ctx, admin, inner.ID); err != nil {
		t.Fatal(err)
	}
	for name, pair := range map[string][2]string{"an archived group inside": {outer.ID, inner.ID}, "inside an archived group": {inner.ID, outer.ID}} {
		if _, err := db.pool.Exec(ctx, "INSERT INTO group_groups (grp, member) VALUES ($1, $2)", pair[0], pair[1]); err == nil {
			t.Errorf("%s: the database took it", name)
		}
	}
}

// TestNothingPassesUpThroughAnArchivedGroup: even where a group inside an archived one is active (which archiving and
// restoring do not let happen, but a database written by hand could), its members gain no allow through the archived
// group and the groups it is inside, and keep their denies (B-3).
func TestNothingPassesUpThroughAnArchivedGroup(t *testing.T) {
	db, ctx := setup(t)
	company, _ := db.CreateGroup(ctx, admin, "Company", "", "")
	sales, _ := db.CreateGroup(ctx, admin, "Sales", "", company.ID)
	inner, _ := db.CreateGroup(ctx, admin, "Key accounts", "", sales.ID)
	if err := db.SetMember(ctx, admin, inner.ID, bob.ID, true); err != nil {
		t.Fatal(err)
	}
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "For the company", team.ID)
	if err := db.SetEntries(ctx, page.ID, admin, ToGroup, company.ID, entries(ActRead, Allow, ActComment, Deny), true); err != nil {
		t.Fatal(err)
	}
	if !may(t, db, ctx, page.ID, bob.ID, ActRead) {
		t.Fatal("bob reads through Key accounts, Sales and Company")
	}
	if _, err := db.pool.Exec(ctx, "UPDATE groups SET archived = now() WHERE id = $1", sales.ID); err != nil {
		t.Fatal(err)
	}
	if may(t, db, ctx, page.ID, bob.ID, ActRead) {
		t.Error("bob reads through archived Sales")
	}
	if may(t, db, ctx, page.ID, bob.ID, ActComment) {
		t.Error("the deny of Company no longer holds for bob")
	}
}
