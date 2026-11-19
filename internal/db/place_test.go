// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"slices"
	"testing"
)

func order(t *testing.T, db *DB, parent string) []string {
	t.Helper()
	kids, err := db.Children(inMain(), parent, anna.ID)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, k := range kids {
		out = append(out, k.Title)
	}
	return out
}

func TestDividersAndMoving(t *testing.T) {
	db := open(t)
	ctx := inMain()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	a, _ := db.CreatePage(ctx, anna, "A", team.ID)
	b, _ := db.CreatePage(ctx, anna, "B", team.ID)
	c, _ := db.CreatePage(ctx, anna, "C", team.ID)
	line, err := db.CreateDivider(ctx, anna, KindLine, team.ID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDivider(ctx, anna, KindSpace, team.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got := order(t, db, team.ID); !slices.Equal(got, []string{"space", "A", "line", "B", "C"}) {
		t.Errorf("dividers where they were put: %q", got)
	}
	if _, err := db.Move(ctx, c.ID, anna.ID, team.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Move(ctx, line.ID, anna.ID, team.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if got := order(t, db, team.ID); !slices.Equal(got, []string{"space", "A", "C", "B", "line"}) {
		t.Errorf("after moving inside the collection: %q", got)
	}
	if _, err := db.Move(ctx, b.ID, anna.ID, a.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got := order(t, db, a.ID); !slices.Equal(got, []string{"B"}) {
		t.Errorf("B below A: %q", got)
	}
	if _, err := db.Move(ctx, a.ID, anna.ID, b.ID, ""); !errors.Is(err, ErrPlace) {
		t.Errorf("A below its own child: %v", err)
	}
	if _, err := db.Move(ctx, a.ID, anna.ID, a.ID, ""); !errors.Is(err, ErrPlace) {
		t.Errorf("A into itself: %v", err)
	}
	if _, err := db.Move(ctx, a.ID, anna.ID, "", ""); !errors.Is(err, ErrPlace) {
		t.Errorf("a page at the top: %v", err)
	}
	if _, err := db.Move(ctx, a.ID, anna.ID, line.ID, ""); !errors.Is(err, ErrPlace) {
		t.Errorf("into a line: %v", err)
	}
	if _, err := db.Move(ctx, a.ID, anna.ID, team.ID, "aaaaaaaaaa"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after something that is not there: %v", err)
	}
	if _, err := db.CreateDivider(ctx, anna, KindPage, team.ID, ""); !errors.Is(err, ErrPlace) {
		t.Errorf("a page is no divider: %v", err)
	}
	if err := db.DeleteDivider(ctx, a.ID, anna.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a page is not deleted as a divider: %v", err)
	}
	if err := db.DeleteDivider(ctx, line.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if got := order(t, db, team.ID); !slices.Equal(got, []string{"space", "A", "C"}) {
		t.Errorf("after deleting the line: %q", got)
	}
}

func TestMovingFollowsAccess(t *testing.T) {
	db := open(t)
	ctx := inMain()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	archive, _ := db.CreateCollection(ctx, anna, "Archive", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	if err := setLevel(db, archive.ID, "read"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Move(ctx, page.ID, bob.ID, archive.ID, ""); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob moves into a collection he may only read: %v", err)
	}
	if _, err := db.CreateDivider(ctx, bob, KindLine, archive.ID, ""); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob adds a line there: %v", err)
	}
	if _, err := db.Move(ctx, page.ID, anna.ID, archive.ID, ""); err != nil {
		t.Errorf("the owner may: %v", err)
	}
	if _, err := db.Move(ctx, page.ID, bob.ID, team.ID, ""); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob moves out of a collection he may only read: %v", err)
	}
	other, _ := db.CreatePage(ctx, anna, "Shared", team.ID)
	bobsNotes, _ := db.Collections(ctx, bob.ID)
	if _, err := db.Move(ctx, other.ID, bob.ID, bobsNotes[0].ID, ""); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob hides Anna's page in his private notes: %v", err)
	}
	annasNotes, _ := db.Collections(ctx, anna.ID)
	bobs, _ := db.CreatePage(ctx, bob, "Bob's", team.ID)
	if _, err := db.Move(ctx, bobs.ID, bob.ID, annasNotes[0].ID, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob moves into Anna's private notes: %v", err)
	}
	if _, err := db.Move(ctx, annasNotes[0].ID, anna.ID, team.ID, ""); !errors.Is(err, ErrPlace) {
		t.Errorf("Anna moves her private notes into a shared collection: %v", err)
	}
	line, _ := db.CreateDivider(ctx, anna, KindLine, team.ID, "")
	if err := db.DeleteDivider(ctx, line.ID, bob.ID); err != nil {
		t.Errorf("others may edit the team, so also remove its line: %v", err)
	}
}

// myNotes returns the private collection of an account.
func myNotes(t *testing.T, db *DB, viewer string) Object {
	t.Helper()
	cols, err := db.Collections(inMain(), viewer)
	if err != nil || len(cols) == 0 || !cols[0].Private {
		t.Fatalf("no private notes for %s: %+v %v", viewer, cols, err)
	}
	return cols[0]
}

// TestMovingElsewhereNeedsManage: whoever may only edit a page reorders it in its place, but does not move it into a
// collection of their own, where they would manage it as the owner above and could share it (ADR-29, 2026-11-19).
func TestMovingElsewhereNeedsManage(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.SeeAccount(ctx, carl); err != nil {
		t.Fatal(err)
	}
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, err := db.CreatePage(ctx, anna, "Salaries", team.ID)
	if err != nil {
		t.Fatal(err)
	}
	budget, _ := db.CreatePage(ctx, anna, "Budget", team.ID)
	if got := role(t, db, page.ID, carl.ID); got != RoleEdit {
		t.Fatalf("carl edits through the tenant: %q", got)
	}
	mine, err := db.CreateCollection(ctx, carl, "Mine", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Move(ctx, page.ID, carl.ID, mine.ID, ""); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("carl, who only edits, moved anna's page into his collection: %v", err)
	}
	if got := role(t, db, page.ID, carl.ID); got != RoleEdit {
		t.Errorf("carl holds %q on anna's page, not edit", got)
	}
	if _, _, err := db.CreateLink(ctx, page.ID, Actor{ID: carl.ID}, NewLink{Mode: LinkRead}); err == nil {
		t.Errorf("carl, who only edits, made a public link to anna's page")
	}
	if err := db.SetGrant(ctx, page.ID, Actor{ID: carl.ID}, ToPerson, bob.ID, RoleManage); err == nil {
		t.Errorf("carl, who only edits, granted manage to bob")
	}
	// Within its place, edit reorders it.
	if _, err := db.Move(ctx, page.ID, carl.ID, team.ID, budget.ID); err != nil {
		t.Errorf("carl reorders the page in its place: %v", err)
	}
	if got := order(t, db, team.ID); !slices.Equal(got, []string{"Budget", "Salaries"}) {
		t.Errorf("order = %v", got)
	}
	// Whoever manages it moves it.
	if err := db.SetGrant(ctx, page.ID, Actor{ID: anna.ID}, ToPerson, carl.ID, RoleManage); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Move(ctx, page.ID, carl.ID, mine.ID, ""); err != nil {
		t.Errorf("carl, who manages the page now, moves it: %v", err)
	}
}

// TestOnlyWhatIsAllYoursGoesPrivate: moving one's own folder into My notes would take a page of someone else along, which
// its owner would lose and nobody else would see. That is refused; a folder with only one's own things in it goes.
func TestOnlyWhatIsAllYoursGoesPrivate(t *testing.T) {
	db := open(t)
	ctx := inMain()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	folder, _ := db.CreateFolder(ctx, anna, "Drafts", team.ID)
	page, err := db.CreatePage(ctx, bob, "Bob's contract", folder.ID)
	if err != nil {
		t.Fatal(err)
	}
	notes := myNotes(t, db, anna.ID)
	if _, err := db.Move(ctx, folder.ID, anna.ID, notes.ID, ""); !errors.Is(err, ErrForeignBelow) {
		t.Errorf("anna took a folder with bob's page into her notes: %v", err)
	}
	if got := role(t, db, page.ID, bob.ID); got != RoleOwner {
		t.Errorf("bob holds %q on his own page", got)
	}
	if o, err := db.Object(ctx, folder.ID, anna.ID); err != nil || o.Parent != team.ID {
		t.Errorf("the folder left the team: %+v %v", o, err)
	}
	own, _ := db.CreateFolder(ctx, anna, "Own", team.ID)
	if _, err := db.CreatePage(ctx, anna, "Mine", own.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Move(ctx, own.ID, anna.ID, notes.ID, ""); err != nil {
		t.Errorf("anna takes a folder of her own into her notes: %v", err)
	}
}

// TestGoingPrivateEndsGrantsAndLinks: an object that goes into something private loses its grants and links, and the log
// keeps that for its owner. "What may X do" names nothing private, also not from grants left from before.
func TestGoingPrivateEndsGrantsAndLinks(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.SeeAccount(ctx, carl); err != nil {
		t.Fatal(err)
	}
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, bob, "Bob resigns on Friday", team.ID)
	below, _ := db.CreatePage(ctx, bob, "Handover", page.ID)
	bobActs := Actor{ID: bob.ID}
	if _, err := db.SetAccess(ctx, page.ID, bobActs, RoleRead); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGrant(ctx, below.ID, bobActs, ToPerson, carl.ID, RoleEdit); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.CreateLink(ctx, page.ID, bobActs, NewLink{Mode: LinkRead}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Move(ctx, page.ID, bob.ID, myNotes(t, db, bob.ID).ID, ""); err != nil {
		t.Fatalf("bob takes his page into his notes: %v", err)
	}
	var grants, links int
	if err := db.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM grants WHERE object IN ($1, $2)), (SELECT count(*) FROM share_links WHERE object IN ($1, $2))`,
		page.ID, below.ID).Scan(&grants, &links); err != nil {
		t.Fatal(err)
	}
	if grants != 0 || links != 0 {
		t.Errorf("in private, %d grants and %d links stay", grants, links)
	}
	logged, err := db.LogEntries(ctx, bobActs, LogFilter{Actor: bob.ID})
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, e := range logged {
		actions = append(actions, e.Action+" "+e.Object)
	}
	for _, want := range []string{LogGrant + " " + page.ID, LogGrant + " " + below.ID, LogLinkRevoked + " " + page.ID} {
		if !slices.Contains(actions, want) {
			t.Errorf("the log of bob lacks %q: %v", want, actions)
		}
	}
	// A grant left on something private from before names it to nobody.
	if _, err := db.pool.Exec(ctx, "INSERT INTO grants (object, kind, who, role) VALUES ($1, 'tenant', '', 'read')", page.ID); err != nil {
		t.Fatal(err)
	}
	for _, asker := range []Actor{{ID: carl.ID}, {ID: anna.ID, Admin: true}} {
		held, err := db.HeldBy(ctx, asker, carl.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range held {
			if h.Object == page.ID || h.Object == below.ID {
				t.Errorf("%s (admin %v) reads the private page in held: %+v", asker.ID, asker.Admin, h)
			}
		}
	}
}
