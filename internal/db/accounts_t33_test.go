// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// Accounts through the gate (T-33) in the database: accounts bound before their first sign-in, the lock of UnderDoc,
// who manages whose account, invitations into groups, the delegated administrator, and the approvals of the gate.

// An account added for an import is bound by its lasting ID alone, never by a login or a name, and gets no mark.
func TestAccountsAreAddedByTheirLastingID(t *testing.T) {
	db, ctx := setup(t)
	a, created, err := db.AddAccount(ctx, "4711", "lena", "Lena M.", "lena@example.org")
	if err != nil || !created || a.RemoteID != "4711" || a.Admin || a.Login != "lena" || a.Name != "Lena M." {
		t.Fatalf("add: %+v %v %v", a, created, err)
	}
	again, created, err := db.AddAccount(ctx, "4711", "other", "Other", "")
	if err != nil || created || again.ID != a.ID || again.Login != "lena" {
		t.Errorf("the same ID again: %+v %v %v", again, created, err)
	}
	// bob's login under another ID is another account, and bob stays as he is.
	same, created, err := db.AddAccount(ctx, "4712", bob.Login, bob.Name, "")
	if err != nil || !created || same.ID == bob.ID {
		t.Errorf("a login of another account: %+v %v", same, err)
	}
	if b, _ := db.AccountByID(ctx, bob.ID); b.RemoteID != "" {
		t.Errorf("bob was bound by his login: %+v", b)
	}
	for name, try := range map[string][3]string{
		"no number":         {"lena", "lena", "Lena"},
		"a control in name": {"4713", "lena", "Le\nna"},
		"an empty login":    {"4714", " ", ""},
	} {
		if _, _, err := db.AddAccount(ctx, try[0], try[1], try[2], ""); !errors.Is(err, ErrRemoteID) && !errors.Is(err, ErrLogin) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// The person comes into it at the first request.
	bound, err := db.BindGate(ctx, account.User{RemoteID: "4711", Login: "lena", Name: "Lena M.", Kind: account.KindPerson, AuthKind: account.AuthSession})
	if err != nil || bound.ID != a.ID {
		t.Errorf("the first sign-in finds another account: %+v %v", bound, err)
	}
}

// A locked account may do nothing through any way, not through ownership, entries or links, and its links lend nothing.
func TestALockedAccountMayDoNothing(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.CreateCollection(ctx, bob, "Bob's space", "")
	page, _ := db.CreatePage(ctx, bob, "Bob's page", team.ID)
	notes, _ := db.Collections(ctx, bob.ID)
	link, token, err := db.CreateLink(ctx, page.ID, Actor{ID: bob.ID}, NewLink{Mode: LinkEdit})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UseLink(ctx, link.ID, dora.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetLocked(ctx, bob.ID, true, anna.ID); err != nil {
		t.Fatal(err)
	}
	for name, object := range map[string]string{"his own page": page.ID, "his private notes": notes[0].ID, "his space": team.ID} {
		for _, action := range []string{ActRead, ActWrite, ActDelete, ActManage} {
			if may(t, db, ctx, object, bob.ID, action) {
				t.Errorf("locked, bob may %s %s", action, name)
			}
		}
	}
	if d, _ := db.Decide(ctx, page.ID, bob.ID, ActRead, false); d.Via != "locked" {
		t.Errorf("the access check names the lock: %+v", d)
	}
	if _, err := db.LinkByToken(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Errorf("a link of a locked account still answers: %v", err)
	}
	if may(t, db, ctx, page.ID, dora.ID, ActWrite) {
		t.Error("dora writes through the link of a locked account")
	}
	// Unlocked, everything is as before.
	if a, err := db.SetLocked(ctx, bob.ID, false, ""); err != nil || a.Locked {
		t.Fatalf("unlock: %+v %v", a, err)
	}
	if !may(t, db, ctx, page.ID, bob.ID, ActWrite) || !may(t, db, ctx, page.ID, dora.ID, ActWrite) {
		t.Error("unlocked, bob or the link does not come back")
	}
}

// The share leaves a locked account's Samba user out.
func TestALockedAccountHasNoSambaUser(t *testing.T) {
	db, ctx := setup(t)
	if _, err := db.pool.Exec(ctx, "INSERT INTO samba_accounts (account, name, uid, nt_hash) VALUES ($1, 'bob', 30001, '\\x00')", bob.ID); err != nil {
		t.Fatal(err)
	}
	locked := func() bool {
		list, err := db.SambaAccounts(ctx)
		if err != nil || len(list) != 1 {
			t.Fatalf("samba accounts: %+v %v", list, err)
		}
		return list[0].Locked
	}
	if locked() {
		t.Fatal("bob is locked before anyone locked him")
	}
	if _, err := db.SetLocked(ctx, bob.ID, true, anna.ID); err != nil {
		t.Fatal(err)
	}
	if !locked() {
		t.Error("the share does not learn that bob is locked")
	}
}

// A group admin manages the accounts of the persons in the groups they manage, inside them too, and no others.
func TestGroupAdminsManageTheirPeople(t *testing.T) {
	db, ctx := setup(t)
	outer, _ := db.CreateGroup(asPerson(anna), admin, "Branch", "", "")
	inner, _ := db.CreateGroup(asPerson(anna), admin, "Till", "", outer.ID)
	other, _ := db.CreateGroup(asPerson(anna), admin, "Office", "", "")
	for _, step := range []error{
		db.SetGroupAdmin(asPerson(anna), admin, outer.ID, carl.ID, true),
		db.SetMember(asPerson(anna), admin, inner.ID, dora.ID, true),
		db.SetMember(asPerson(anna), admin, other.ID, bob.ID, true),
	} {
		if step != nil {
			t.Fatal(step)
		}
	}
	if m, err := db.MayManagePerson(ctx, carl.ID, dora.ID); err != nil || !m.Manages {
		t.Errorf("carl manages dora in a group inside his: %v %v", m, err)
	}
	if m, _ := db.MayManagePerson(ctx, carl.ID, bob.ID); m.Manages {
		t.Error("carl manages bob in a group he does not manage")
	}
	if m, _ := db.MayManagePerson(ctx, dora.ID, carl.ID); m.Manages {
		t.Error("dora manages her group admin")
	}
	if err := db.ArchiveGroup(asPerson(anna), admin, inner.ID); err != nil {
		t.Fatal(err)
	}
	if m, _ := db.MayManagePerson(ctx, carl.ID, dora.ID); m.Manages {
		t.Error("carl manages dora through an archived group")
	}
}

// Who may invite into a group: its admins, administrators and the delegated administrator, into a local group that is
// not archived, never an agent.
func TestInvitationsIntoGroups(t *testing.T) {
	db, ctx := setup(t)
	group, _ := db.CreateGroup(asPerson(anna), admin, "Branch", "", "")
	if err := db.SetGroupAdmin(asPerson(anna), admin, group.ID, carl.ID, true); err != nil {
		t.Fatal(err)
	}
	newcomer, _, _ := db.AddAccount(ctx, "4720", "lena", "Lena", "")
	delegate := Actor{ID: dora.ID, Delegate: true}
	for name, try := range map[string]struct {
		ctx   func() error
		allow bool
	}{
		"its admin":                   {func() error { return db.InviteMember(asPerson(carl), Actor{ID: carl.ID}, group.ID, newcomer.ID) }, true},
		"the delegated administrator": {func() error { return db.InviteMember(asPerson(dora), delegate, group.ID, newcomer.ID) }, true},
		"bob":                         {func() error { return db.InviteMember(asPerson(bob), Actor{ID: bob.ID}, group.ID, newcomer.ID) }, false},
		"an agent of its admin":       {func() error { return db.InviteMember(asAgent(carl), Actor{ID: carl.ID}, group.ID, newcomer.ID) }, false},
		"an agent of the delegate":    {func() error { return db.InviteMember(asAgent(dora), delegate, group.ID, newcomer.ID) }, false},
	} {
		err := try.ctx()
		if try.allow && err != nil || !try.allow && !isNotAllowed(err) {
			t.Errorf("%s invites: %v", name, err)
		}
	}
	if err := db.ArchiveGroup(asPerson(dora), delegate, group.ID); err != nil {
		t.Fatalf("the delegated administrator archives a group: %v", err)
	}
	if err := db.InviteMember(asPerson(anna), admin, group.ID, newcomer.ID); !errors.Is(err, ErrGroupArchived) {
		t.Errorf("into an archived group: %v", err)
	}
	if err := db.RestoreGroup(asPerson(dora), delegate, group.ID); err != nil {
		t.Errorf("the delegated administrator restores a group: %v", err)
	}
	if err := db.ArchiveGroup(asPerson(carl), Actor{ID: carl.ID}, group.ID); !isNotAllowed(err) {
		t.Errorf("its group admin archives it: %v", err)
	}
	if err := db.ArchiveGroup(asAgent(dora), delegate, group.ID); !isNotAllowed(err) {
		t.Errorf("an agent of the delegated administrator archives it: %v", err)
	}
}

// An approval UnderDoc waits for is taken once, by the account that asked; a confirmation's jti holds once.
func TestApprovalsAreTakenOnce(t *testing.T) {
	db, ctx := setup(t)
	p := PendingApproval{ID: "apv_test0001", Account: anna.ID, Kind: ApprovalPurge, Object: "7k2m9q4x1c",
		Fields: map[string]any{"object": "7k2m9q4x1c", "title": "Plan", "n": int64(3)}, Nonce: "state"}
	if err := db.PutApproval(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := db.TakeApproval(ctx, p.ID, bob.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob takes anna's approval: %v", err)
	}
	got, err := db.TakeApproval(ctx, p.ID, anna.ID)
	if err != nil || got.Object != p.Object || got.Nonce != "state" || got.Fields["title"] != "Plan" {
		t.Fatalf("take: %+v %v", got, err)
	}
	if n, ok := got.Fields["n"].(interface{ Int64() (int64, error) }); !ok {
		t.Errorf("a number comes back as %T", got.Fields["n"])
	} else if v, _ := n.Int64(); v != 3 {
		t.Errorf("the number: %d", v)
	}
	if _, err := db.TakeApproval(ctx, p.ID, anna.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("taken twice: %v", err)
	}
	// One older than the wait is gone.
	old := p
	old.ID = "apv_test0002"
	if err := db.PutApproval(ctx, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.pool.Exec(ctx, "UPDATE gate_approvals SET created = now() - interval '1 hour' WHERE id = $1", old.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.PendingApprovalOf(ctx, old.ID, anna.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("an approval past its wait: %v", err)
	}
	until := time.Now().Add(time.Minute).Unix()
	if first, err := db.RememberJTI(ctx, "apv_jti1", until); err != nil || !first {
		t.Errorf("a new jti: %v %v", first, err)
	}
	if first, err := db.RememberJTI(ctx, "apv_jti1", until); err != nil || first {
		t.Errorf("a jti twice: %v %v", first, err)
	}
}
