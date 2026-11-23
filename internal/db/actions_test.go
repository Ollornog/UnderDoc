// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
)

// The rights of ADR-36: actions with inherit, allow and deny, nested groups and the group Agents.

var dora = account.User{ID: "u-dora", Login: "dora", Name: "Dora"}

// admin is an administrator of the tenant who changes rights in the tests.
var admin = Actor{ID: anna.ID, Admin: true}

func may(t *testing.T, db *DB, ctx context.Context, objectID, viewer, action string) bool {
	t.Helper()
	ok, err := db.May(ctx, objectID, viewer, action)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

func entries(pairs ...string) map[string]string {
	m := map[string]string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	return m
}

// asAgent is a context of a request of an agent of the user: signed in through OAuth, not in a session.
func asAgent(user account.User) context.Context {
	user.AuthKind = account.AuthOAuth
	return account.With(inMain(), user)
}

// asPerson is a context of a request of the user in a session.
func asPerson(user account.User) context.Context {
	user.AuthKind = account.AuthSession
	return account.With(inMain(), user)
}

// purgeAs deletes an object of the trash for good as the account in a browser session, with its name typed in (B-3),
// in the tenant of ctx.
func purgeAs(ctx context.Context, db *DB, accountID, objectID string) error {
	var title string
	if err := db.pool.QueryRow(ctx, "SELECT title FROM objects WHERE id = $1", objectID).Scan(&title); err != nil {
		return err
	}
	return db.Purge(account.With(ctx, account.User{ID: accountID, AuthKind: account.AuthSession}), objectID, Actor{ID: accountID}, title)
}

func setup(t *testing.T) (*DB, context.Context) {
	t.Helper()
	db := open(t)
	ctx := inMain()
	for _, u := range []account.User{carl, dora} {
		if err := db.SeeAccount(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	return db, ctx
}

// TestTheNearestEntryWins: for each action the nearest object with an entry for the account or a group of it decides
// (ADR-36), and nothing else counts below it.
func TestTheNearestEntryWins(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	folder, _ := db.CreateFolder(ctx, anna, "HR", team.ID)
	page, _ := db.CreatePage(ctx, anna, "Salaries", folder.ID)
	sales, err := db.CreateGroup(ctx, admin, "Sales", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{bob.ID, carl.ID} {
		if err := db.SetMember(ctx, admin, sales.ID, u, true); err != nil {
			t.Fatal(err)
		}
	}
	// Everyone reads Team, Sales may not read HR, and bob reads the page itself.
	if err := db.SetGrant(ctx, team.ID, admin, ToTenant, "", RoleRead); err != nil {
		t.Fatal(err)
	}
	if err := db.SetEntries(ctx, folder.ID, admin, ToGroup, sales.ID, entries(ActRead, Deny), true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetEntries(ctx, page.ID, admin, ToPerson, bob.ID, entries(ActRead, Allow), true); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		object, viewer string
		want           bool
	}{
		{team.ID, carl.ID, true}, {folder.ID, carl.ID, false}, {page.ID, carl.ID, false},
		{folder.ID, bob.ID, false}, {page.ID, bob.ID, true},
		{folder.ID, dora.ID, true}, {page.ID, dora.ID, true},
	} {
		if got := may(t, db, ctx, c.object, c.viewer, ActRead); got != c.want {
			t.Errorf("%s reads %s: %v, want %v", c.viewer, c.object, got, c.want)
		}
	}
	d, err := db.Decide(ctx, page.ID, bob.ID, ActRead, false)
	if err != nil || !d.Allowed || d.Via != ViaEntry || d.Object != page.ID || d.Kind != ToPerson || d.Who != bob.ID {
		t.Errorf("bob's read is decided on the page by his own entry: %+v %v", d, err)
	}
	d, err = db.Decide(ctx, page.ID, carl.ID, ActRead, false)
	if err != nil || d.Allowed || d.Via != ViaEntry || d.Object != folder.ID || d.Kind != ToGroup || d.Name != "Sales" || d.Value != Deny {
		t.Errorf("carl's read is refused on HR by the deny of Sales: %+v %v", d, err)
	}
	// Bob sees the page in a list too, and carl does not see HR among the children of Team.
	if kids, _ := db.Children(ctx, team.ID, carl.ID); len(kids) != 0 {
		t.Errorf("carl lists %v in Team", titles(kids))
	}
	if kids, _ := db.Children(ctx, folder.ID, bob.ID); len(kids) != 1 {
		t.Errorf("bob lists the page he reads: %v", titles(kids))
	}
	// No entry up to the top: denied.
	alone, _ := db.CreateCollection(ctx, anna, "Alone", "")
	if d, _ := db.Decide(ctx, alone.ID, carl.ID, ActRead, false); d.Allowed || d.Via != ViaNone {
		t.Errorf("without an entry nothing is allowed: %+v", d)
	}
}

// TestDenyBeatsAllowOnTheSameLevel: on the object that decides, a deny beats an allow (ADR-36).
func TestDenyBeatsAllowOnTheSameLevel(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	interns, _ := db.CreateGroup(ctx, admin, "Interns", "", "")
	if err := db.SetMember(ctx, admin, interns.ID, carl.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGrant(ctx, team.ID, admin, ToTenant, "", RoleEdit); err != nil {
		t.Fatal(err)
	}
	if err := db.SetEntries(ctx, team.ID, admin, ToGroup, interns.ID, entries(ActWrite, Deny, ActDelete, Deny), true); err != nil {
		t.Fatal(err)
	}
	if !may(t, db, ctx, page.ID, carl.ID, ActRead) || !may(t, db, ctx, page.ID, carl.ID, ActCreate) {
		t.Error("carl still reads and creates as everyone")
	}
	if may(t, db, ctx, page.ID, carl.ID, ActWrite) || may(t, db, ctx, page.ID, carl.ID, ActDelete) {
		t.Error("the deny of Interns on the same level beats the allow of everyone")
	}
	if !may(t, db, ctx, page.ID, bob.ID, ActWrite) {
		t.Error("bob is no intern and writes")
	}
	if _, err := db.AddBlock(ctx, page.ID, carl.ID, "", "carl was here"); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("carl writes a block: %v", err)
	}
	if role := role(t, db, page.ID, carl.ID); role != RoleComment {
		t.Errorf("carl's preset is comment now: %q", role)
	}
	// The refusal is logged with the deciding entry.
	var detail string
	if err := db.pool.QueryRow(ctx, "SELECT detail FROM audit_log WHERE action = $1 AND actor = $2 AND object = $3", LogRefused, carl.ID, page.ID).
		Scan(&detail); err != nil || !strings.Contains(detail, "write: deny for group Interns ("+interns.ID+") on "+team.ID) {
		t.Errorf("the log names the deny that refused: %q %v", detail, err)
	}
}

// TestTheOwnerKeepsEveryAction: a deny for a group the owner belongs to does not lock them out of their own object; on
// what they own above, their implicit allow is beaten by a deny on that level like any other (ADR-36).
func TestTheOwnerKeepsEveryAction(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	all, _ := db.CreateGroup(ctx, admin, "All", "", "")
	if err := db.SetMember(ctx, admin, all.ID, anna.ID, true); err != nil {
		t.Fatal(err)
	}
	deny := entries()
	for _, a := range Actions {
		deny[a] = Deny
	}
	if err := db.SetEntries(ctx, team.ID, admin, ToGroup, all.ID, deny, true); err != nil {
		t.Fatal(err)
	}
	for _, a := range Actions {
		if !may(t, db, ctx, team.ID, anna.ID, a) {
			t.Errorf("anna owns Team and may %s", a)
		}
	}
	if role := role(t, db, team.ID, anna.ID); role != RoleOwner {
		t.Errorf("anna's role: %q", role)
	}
	if err := db.SetEntries(ctx, team.ID, Actor{ID: anna.ID}, ToPerson, bob.ID, entries(ActRead, Allow), true); err != nil {
		t.Errorf("anna manages the rights of her Team: %v", err)
	}
}

// TestAdministratorsManageRightsNotContent: a deny does not keep an administrator from managing rights, and their
// access to content follows the entries like anyone's (ADR-36).
func TestAdministratorsManageRightsNotContent(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.CreateCollection(ctx, bob, "Bob's", "")
	if err := db.SetEntries(ctx, team.ID, Actor{ID: bob.ID}, ToPerson, carl.ID, entries(ActRead, Deny, ActManage, Deny), true); err != nil {
		t.Fatal(err)
	}
	boss := Actor{ID: carl.ID, Admin: true}
	if _, err := db.Object(ctx, team.ID, carl.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("carl reads Bob's: %v", err)
	}
	if err := db.SetEntries(ctx, team.ID, boss, ToPerson, dora.ID, entries(ActRead, Allow), true); err != nil {
		t.Errorf("as an administrator carl manages the rights all the same: %v", err)
	}
	if r, err := db.RightsOf(ctx, team.ID, boss); err != nil || len(r.Own) != 2 {
		t.Errorf("and sees them: %+v %v", r, err)
	}
}

// TestPrivateStaysPrivate: nothing is granted or denied on something private (ADR-29, ADR-36).
func TestPrivateStaysPrivateUnderActions(t *testing.T) {
	db, ctx := setup(t)
	var notes Object
	for _, c := range must(db.Collections(ctx, bob.ID)) {
		if c.Private {
			notes = c
		}
	}
	if notes.ID == "" {
		t.Fatal("bob has no notes")
	}
	if err := db.SetEntries(ctx, notes.ID, Actor{ID: bob.ID}, ToPerson, carl.ID, entries(ActRead, Allow), true); !errors.Is(err, ErrPrivate) {
		t.Errorf("an entry on private notes: %v", err)
	}
	if err := db.SetEntries(ctx, notes.ID, Actor{ID: bob.ID}, ToAgents, "", entries(ActRead, Deny), true); !errors.Is(err, ErrPrivate) {
		t.Errorf("a deny for Agents on private notes: %v", err)
	}
	if !may(t, db, ctx, notes.ID, bob.ID, ActManage) || may(t, db, ctx, notes.ID, carl.ID, ActRead) {
		t.Error("bob's notes are bob's alone")
	}
}

// TestAgentsAreACeiling: every request that did not sign in in a session belongs to Agents. A deny on Agents holds after
// the evaluation, for the owner too; agents never manage or share; each person limits their own agents (ADR-36).
func TestAgentsAreACeiling(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	agent, person := asAgent(anna), asPerson(anna)
	for _, a := range AgentActions {
		if !may(t, db, agent, page.ID, anna.ID, a) {
			t.Errorf("anna's agent may %s on her page", a)
		}
	}
	for _, a := range []string{ActShare, ActManage} {
		if may(t, db, agent, page.ID, anna.ID, a) {
			t.Errorf("an agent may never %s", a)
		}
		if !may(t, db, person, page.ID, anna.ID, a) {
			t.Errorf("anna herself may %s", a)
		}
	}
	if err := db.SetGrant(agent, page.ID, Actor{ID: anna.ID}, ToTenant, "", RoleRead); !errors.Is(err, ErrAgent) {
		t.Errorf("an agent changes rights: %v", err)
	}
	if err := db.SetGrant(asAgent(account.User{ID: anna.ID, Admin: true}), page.ID, admin, ToTenant, "", RoleRead); !errors.Is(err, ErrAgent) {
		t.Errorf("an administrator's agent changes rights: %v", err)
	}
	// No agents on Team: the owner's agent is refused too.
	if err := db.SetEntries(ctx, team.ID, admin, ToAgents, "", entries(ActRead, Deny), true); err != nil {
		t.Fatal(err)
	}
	if may(t, db, agent, page.ID, anna.ID, ActRead) {
		t.Error("owner + oauth + deny on Agents: refused")
	}
	if !may(t, db, person, page.ID, anna.ID, ActRead) {
		t.Error("anna herself reads")
	}
	if _, err := db.Object(agent, page.ID, anna.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the agent opens the page: %v", err)
	}
	d, err := db.Decide(ctx, page.ID, anna.ID, ActRead, true)
	if err != nil || d.Allowed || d.Via != ViaAgents || d.Object != team.ID {
		t.Errorf("the check names the deny on Agents on Team: %+v %v", d, err)
	}
	// The ceiling holds whatever breaks the inheritance below it.
	if err := db.SetInherit(ctx, page.ID, admin, false); err != nil {
		t.Fatal(err)
	}
	if may(t, db, agent, page.ID, anna.ID, ActRead) {
		t.Error("breaking the inheritance lets agents in")
	}
	if err := db.SetEntries(ctx, team.ID, admin, ToAgents, "", entries(ActRead, Allow), true); !errors.Is(err, ErrEntry) {
		t.Errorf("an allow for Agents: %v", err)
	}
	// A service account is an agent (B-3) and gets what is granted to it, within what agents may.
	svc := account.User{ID: carl.ID, AuthKind: account.AuthAPIKey, Kind: account.KindService}
	if err := db.SetGrant(ctx, team.ID, admin, ToPerson, carl.ID, RoleEdit); err != nil {
		t.Fatal(err)
	}
	if err := db.SetInherit(ctx, page.ID, admin, true); err != nil {
		t.Fatal(err)
	}
	if !may(t, db, account.With(inMain(), svc), page.ID, carl.ID, ActWrite) {
		t.Error("a service account writes where it is granted")
	}
	// An unknown or missing way of signing in counts as an agent (fail closed).
	for _, kind := range []string{"", "magic"} {
		u := anna
		u.AuthKind = kind
		if may(t, db, account.With(inMain(), u), page.ID, anna.ID, ActRead) {
			t.Errorf("sign-in %q passes the ceiling", kind)
		}
	}
	// Anna limits her own agents to reading.
	other, _ := db.CreateCollection(ctx, anna, "Other", "")
	if err := db.SetAgentLimit(agent, anna.ID, []string{ActRead}); !errors.Is(err, ErrAgent) {
		t.Errorf("an agent changes its own limit: %v", err)
	}
	if err := db.SetAgentLimit(person, anna.ID, []string{ActRead}); err != nil {
		t.Fatal(err)
	}
	if !may(t, db, agent, other.ID, anna.ID, ActRead) || may(t, db, agent, other.ID, anna.ID, ActWrite) {
		t.Error("anna's agents only read now")
	}
	if err := db.SetAgentLimit(person, anna.ID, []string{ActManage}); !errors.Is(err, ErrEntry) {
		t.Errorf("agents never manage, not even when allowed: %v", err)
	}
}

// TestAgentsNeverDeleteForGoodNorReadPasswords: the trash empties and passwords show only to the person (ADR-36, ADR-7).
func TestAgentsNeverDeleteForGood(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "Old", team.ID)
	agent := asAgent(anna)
	// An agent moves no space to the trash (B-5), a page in it it does.
	if err := db.Delete(agent, team.ID, anna.ID); !errors.Is(err, ErrAgent) {
		t.Errorf("an agent moves a space to the trash: %v", err)
	}
	if err := db.Delete(agent, page.ID, anna.ID); err != nil {
		t.Fatalf("an agent moves to the trash: %v", err)
	}
	if err := db.Purge(agent, page.ID, Actor{ID: anna.ID}, "Old"); !errors.Is(err, ErrAgent) {
		t.Errorf("an agent deletes for good: %v", err)
	}
	if _, _, err := db.RevealPageSecret(agent, "nothing", anna.ID, 0, false); !errors.Is(err, ErrAgent) {
		t.Errorf("an agent reveals a password: %v", err)
	}
	if err := db.Purge(asPerson(anna), page.ID, Actor{ID: anna.ID}, "Old"); err != nil {
		t.Errorf("anna empties it herself: %v", err)
	}
}

// TestGroupsNest: membership is passed down, a grant holds for subgroups unless it says otherwise, a cycle is refused,
// roles of the gate do not contain groups, and group admins manage their group and those inside it (ADR-36).
func TestGroupsNest(t *testing.T) {
	db, ctx := setup(t)
	company, _ := db.CreateGroup(ctx, admin, "Company", "", "")
	sales, err := db.CreateGroup(ctx, admin, "Sales", "", company.ID)
	if err != nil {
		t.Fatal(err)
	}
	north, _ := db.CreateGroup(ctx, admin, "North", "", sales.ID)
	if err := db.SetMember(ctx, admin, north.ID, carl.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetMember(ctx, admin, company.ID, dora.ID, true); err != nil {
		t.Fatal(err)
	}
	if mine, _ := db.GroupsOf(ctx, carl.ID); !slices.Equal(mine, sortedIDs(company.ID, sales.ID, north.ID)) {
		t.Errorf("carl is in North, so in Sales and Company: %v", mine)
	}
	// The group ops of anna's sign-in can be inside a local group.
	var ops string
	for _, g := range must(db.Groups(ctx)) {
		if g.Name == "ops" {
			ops = g.ID
		}
	}
	if err := db.SetSubgroup(ctx, admin, sales.ID, ops, true); err != nil {
		t.Fatal(err)
	}
	if mine, _ := db.GroupsOf(ctx, anna.ID); !slices.Contains(mine, sales.ID) || !slices.Contains(mine, company.ID) {
		t.Errorf("anna, of ops, is in Sales and Company: %v", mine)
	}
	if err := db.SetSubgroup(ctx, admin, ops, sales.ID, true); !errors.Is(err, ErrGroup) {
		t.Errorf("a role of the gate contains a group: %v", err)
	}
	// Cycles, direct and through others.
	for _, c := range [][2]string{{north.ID, company.ID}, {north.ID, north.ID}, {sales.ID, company.ID}} {
		if err := db.SetSubgroup(ctx, admin, c[0], c[1], true); !errors.Is(err, ErrGroupCycle) {
			t.Errorf("%s inside %s: %v", c[1], c[0], err)
		}
	}
	// A cycle made past the check is refused by the database itself.
	if _, err := db.pool.Exec(ctx, "INSERT INTO group_groups (grp, member) VALUES ($1, $2)", north.ID, company.ID); err == nil {
		t.Error("the database takes a cycle")
	}
	// With subgroups (the default) a grant to Company reaches carl; without, only dora, who is in it directly.
	page, _ := db.CreateCollection(ctx, anna, "Handbook", "")
	if err := db.SetEntries(ctx, page.ID, admin, ToGroup, company.ID, entries(ActRead, Allow), true); err != nil {
		t.Fatal(err)
	}
	if !may(t, db, ctx, page.ID, carl.ID, ActRead) || !may(t, db, ctx, page.ID, dora.ID, ActRead) {
		t.Error("Company with its subgroups reads")
	}
	if err := db.SetEntries(ctx, page.ID, admin, ToGroup, company.ID, entries(ActRead, Allow), false); err != nil {
		t.Fatal(err)
	}
	if may(t, db, ctx, page.ID, carl.ID, ActRead) || !may(t, db, ctx, page.ID, dora.ID, ActRead) {
		t.Error("Company without its subgroups: dora reads, carl does not")
	}
	// Group admins: bob administers Sales, so Sales and North, not Company.
	if err := db.SetGroupAdmin(ctx, admin, sales.ID, bob.ID, true); err != nil {
		t.Fatal(err)
	}
	lead := Actor{ID: bob.ID}
	if err := db.SetMember(ctx, lead, north.ID, dora.ID, true); err != nil {
		t.Errorf("bob takes dora into North: %v", err)
	}
	if err := db.SetMember(ctx, lead, company.ID, bob.ID, true); !errors.Is(err, ErrGroupAdmin) {
		t.Errorf("bob changes Company: %v", err)
	}
	if _, err := db.CreateGroup(ctx, lead, "South", "", sales.ID); err != nil {
		t.Errorf("bob makes a group inside Sales: %v", err)
	}
	if _, err := db.CreateGroup(ctx, lead, "Elsewhere", "", ""); !errors.Is(err, ErrGroupAdmin) {
		t.Errorf("bob makes a group at the top: %v", err)
	}
	board, _ := db.CreateGroup(ctx, admin, "Board", "", "")
	if err := db.SetSubgroup(ctx, lead, sales.ID, board.ID, true); !errors.Is(err, ErrGroupAdmin) {
		t.Errorf("bob puts Board, which he does not manage, inside Sales and so comes to manage it: %v", err)
	}
	if err := db.SetSubgroup(ctx, lead, sales.ID, ops, true); err != nil {
		t.Errorf("bob puts a role of the gate inside Sales: %v", err)
	}
	if err := db.SetAdmin(asAgent(account.User{ID: anna.ID, Admin: true}), admin, carl.ID, true); !errors.Is(err, ErrAgent) {
		t.Errorf("an administrator's agent makes an administrator: %v", err)
	}
	if _, err := db.SetSambaPassword(asAgent(anna), anna.ID, "anna", nil, 20000, 29999); !errors.Is(err, ErrAgent) {
		t.Errorf("an agent makes an app password for the share: %v", err)
	}
	if err := db.SetGroupAdmin(asAgent(account.User{ID: anna.ID, Admin: true}), admin, sales.ID, carl.ID, true); !errors.Is(err, ErrAgent) {
		t.Errorf("an agent manages a group: %v", err)
	}
}

func sortedIDs(ids ...string) []string {
	slices.Sort(ids)
	return ids
}

// TestMovingNeedsDeleteAndCreate: to another place an object goes with delete where it is and create where it goes, and
// never under something of the mover's own that would let them share it or manage its rights (ADR-36, ADR-29).
func TestMovingNeedsDeleteAndCreate(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	archive, _ := db.CreateCollection(ctx, anna, "Archive", "")
	mine, _ := db.CreateCollection(ctx, carl, "Carl's", "")
	if err := db.SetEntries(ctx, team.ID, admin, ToPerson, carl.ID, entries(ActRead, Allow, ActDelete, Allow), true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetEntries(ctx, archive.ID, admin, ToPerson, carl.ID, entries(ActRead, Allow), true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Move(ctx, page.ID, carl.ID, archive.ID, ""); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("without create on Archive: %v", err)
	}
	if err := db.SetEntries(ctx, archive.ID, admin, ToPerson, carl.ID, entries(ActRead, Allow, ActCreate, Allow), true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Move(ctx, page.ID, carl.ID, mine.ID, ""); !errors.Is(err, ErrMoveWidens) {
		t.Errorf("into his own collection, where he would manage it: %v", err)
	}
	// Something below that shuts carl out does not go with a move of his, as it would not go with a deletion.
	hidden, _ := db.CreatePage(ctx, anna, "Hidden", page.ID)
	if err := db.SetInherit(ctx, hidden.ID, admin, false); err != nil {
		t.Fatal(err)
	}
	if err := db.SetEntries(ctx, hidden.ID, admin, ToPerson, carl.ID, entries(), true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Move(ctx, page.ID, carl.ID, archive.ID, ""); !errors.Is(err, ErrHeldBelow) {
		t.Errorf("moving what holds something carl may not delete: %v", err)
	}
	if err := db.SetEntries(ctx, hidden.ID, admin, ToPerson, carl.ID, entries(ActDelete, Allow), true); err != nil {
		t.Fatal(err)
	}
	if moved, err := db.Move(ctx, page.ID, carl.ID, archive.ID, ""); err != nil || moved.Parent != archive.ID {
		t.Errorf("delete on Team and create on Archive move it: %+v %v", moved, err)
	}
	if _, err := db.Move(ctx, page.ID, bob.ID, team.ID, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob sees nothing of it: %v", err)
	}
}

// TestBreakingCopiesTheNearestEntries: breaking the inheritance copies, for each holder and action, the nearest entry,
// allow or deny, so nobody gains anything (ADR-36).
func TestBreakingCopiesTheNearestEntries(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	folder, _ := db.CreateFolder(ctx, anna, "HR", team.ID)
	page, _ := db.CreatePage(ctx, anna, "Plan", folder.ID)
	if err := db.SetGrant(ctx, team.ID, admin, ToTenant, "", RoleEdit); err != nil {
		t.Fatal(err)
	}
	if err := db.SetEntries(ctx, folder.ID, admin, ToPerson, carl.ID, entries(ActWrite, Deny), true); err != nil {
		t.Fatal(err)
	}
	before := map[string][]string{}
	for _, u := range []string{bob.ID, carl.ID} {
		before[u] = must(db.ActionsOf(ctx, page.ID, u))
	}
	if err := db.SetInherit(ctx, page.ID, admin, false); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{bob.ID, carl.ID} {
		if after := must(db.ActionsOf(ctx, page.ID, u)); !slices.Equal(after, before[u]) {
			t.Errorf("%s: %v before breaking, %v after", u, before[u], after)
		}
	}
	if err := db.SetEntries(ctx, team.ID, admin, ToPerson, carl.ID, entries(ActWrite, Allow), true); err != nil {
		t.Fatal(err)
	}
	if may(t, db, ctx, page.ID, carl.ID, ActWrite) {
		t.Error("nothing comes from above once broken")
	}
}

// TestGrantsBecomeAllowEntries: the migration turns each grant of a role into allow entries of its preset and keeps a
// broken inheritance broken, so everyone holds what they held (ADR-36).
func TestGrantsBecomeAllowEntries(t *testing.T) {
	ctx := context.Background()
	url := pgtest.URL(t)
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, conn, 29); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `SELECT set_config('underdoc.tenant', 'main', false);
		INSERT INTO accounts (id, login, name, groups) VALUES ('u-anna', 'anna', 'Anna', '{}'), ('u-bob', 'bob', 'Bob', '{ops}'),
			('u-carl', 'carl', 'Carl', '{}');
		INSERT INTO groups (id, name, source) VALUES ('gggggggg01', 'ops', 'synced'), ('gggggggg02', 'Editors', 'local');
		INSERT INTO group_members (grp, account) VALUES ('gggggggg02', 'u-carl');
		INSERT INTO objects (id, kind, title, owner, parent, private, area, inherit) VALUES
			('ccccccccc1', 'collection', 'Team', 'u-anna', NULL, false, 'docs', true),
			('ccccccccc2', 'folder', 'HR', 'u-anna', 'ccccccccc1', false, 'docs', false),
			('ccccccccc3', 'page', 'Plan', 'u-anna', 'ccccccccc2', false, 'docs', true),
			('ccccccccc4', 'page', 'Open', 'u-anna', 'ccccccccc1', false, 'docs', true);
		INSERT INTO grants (object, kind, who, role, granted_by) VALUES
			('ccccccccc1', 'tenant', '', 'edit', 'u-anna'),
			('ccccccccc1', 'group', 'gggggggg01', 'manage', 'u-anna'),
			('ccccccccc2', 'person', 'u-bob', 'comment', 'u-anna'),
			('ccccccccc4', 'group', 'gggggggg02', 'read', '')`); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close(ctx)
	db, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	main := inMain()
	for _, c := range []struct{ object, viewer, want string }{
		{"ccccccccc1", "u-bob", RoleManage}, {"ccccccccc1", "u-carl", RoleEdit},
		{"ccccccccc2", "u-bob", RoleComment}, {"ccccccccc2", "u-carl", RoleNone}, {"ccccccccc3", "u-bob", RoleComment},
		{"ccccccccc4", "u-carl", RoleEdit}, {"ccccccccc4", "u-bob", RoleManage}, {"ccccccccc3", "u-anna", RoleOwner},
	} {
		if got := role(t, db, c.object, c.viewer); got != c.want {
			t.Errorf("%s: %s holds %q, want %q", c.object, c.viewer, got, c.want)
		}
	}
	var rows int
	var by string
	if err := db.pool.QueryRow(main, `SELECT count(*), min(granted_by) FROM grants WHERE object = 'ccccccccc1' AND kind = 'group' AND value = 'allow'`).
		Scan(&rows, &by); err != nil || rows != 7 || by != "u-anna" {
		t.Errorf("manage became 7 allow entries, kept who granted it: %d %q %v", rows, by, err)
	}
	r, err := db.RightsOf(main, "ccccccccc2", admin)
	if err != nil || r.Inherit || len(r.Own) != 1 || r.Own[0].Role != RoleComment || !r.Own[0].Subgroups {
		t.Errorf("HR still breaks the inheritance and grants bob comment: %+v %v", r, err)
	}
	if grants, err := db.Groups(main); err != nil || len(grants) != 2 {
		t.Errorf("the groups stay: %+v %v", grants, err)
	}
}

// TestDenyHoldsThroughEveryDoor: the attack round of T-26. A deny holds for a share link its maker no longer may make, in
// a copy, in an export, in the path of a search hit and for the role a link lends (ADR-36).
func TestDenyHoldsThroughEveryDoor(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	folder, _ := db.CreateFolder(ctx, anna, "Projects", team.ID)
	open, _ := db.CreatePage(ctx, anna, "Open", folder.ID)
	closed, _ := db.CreatePage(ctx, anna, "Closed", folder.ID)
	if err := db.SetGrant(ctx, team.ID, admin, ToTenant, "", RoleEdit); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGrant(ctx, folder.ID, admin, ToPerson, bob.ID, RoleManage); err != nil {
		t.Fatal(err)
	}
	if err := db.SetEntries(ctx, closed.ID, admin, ToPerson, carl.ID, entries(ActRead, Deny), true); err != nil {
		t.Fatal(err)
	}
	// A link of bob's lives while bob may share.
	link, token, err := db.CreateLink(ctx, open.ID, Actor{ID: bob.ID}, NewLink{Mode: LinkRead, Frozen: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.LinkByToken(ctx, token); err != nil {
		t.Fatalf("the link lives: %v", err)
	}
	if err := db.SetEntries(ctx, folder.ID, admin, ToPerson, bob.ID, entries(ActRead, Allow, ActShare, Deny), true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.LinkByToken(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Errorf("a link whose maker may no longer share answers like none: %+v %v", link, err)
	}
	if _, _, err := db.CreateLink(ctx, open.ID, Actor{ID: bob.ID}, NewLink{Mode: LinkRead, Frozen: "x"}); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob makes a link without share: %v", err)
	}
	// Carl copies Projects into Team: Closed does not come along.
	cp, err := db.Copy(ctx, folder.ID, carl.ID, team.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if kids, _ := db.Children(ctx, cp.ID, carl.ID); len(kids) != 1 || kids[0].Title != "Open" {
		t.Errorf("the copy of carl holds only what he reads: %v", titles(kids))
	}
	// Carl exports Projects: without Closed.
	tree, err := db.Subtree(ctx, folder.ID, carl.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range tree.Kids {
		if k.ID == closed.ID {
			t.Error("the export of carl holds the page denied to him")
		}
	}
	// The path of a hit: a level that denies someone is never named to the readers of a page below it.
	below, _ := db.CreatePage(ctx, anna, "Below", closed.ID)
	if err := db.SetEntries(ctx, below.ID, admin, ToPerson, carl.ID, entries(ActRead, Allow), true); err != nil {
		t.Fatal(err)
	}
	docs, err := db.SearchSource().Docs(ctx, below.ID)
	if err != nil || len(docs) == 0 {
		t.Fatalf("docs: %v %v", docs, err)
	}
	if !strings.HasSuffix(docs[0].Path, "…") || !docs[0].MayRead([]string{"all", "account:" + carl.ID}, false) {
		t.Errorf("carl finds Below without the name of Closed, which is denied to him: %q", docs[0].Path)
	}
	// A link to edit lends nothing past a nearer deny.
	edit, etoken, err := db.CreateLink(ctx, team.ID, Actor{ID: anna.ID}, NewLink{Mode: LinkEdit})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UseLink(ctx, edit.ID, carl.ID); err != nil {
		t.Fatal(err)
	}
	_ = etoken
	if may(t, db, ctx, closed.ID, carl.ID, ActRead) {
		t.Error("a link to edit Team lets carl read the page denied to him")
	}
}
