// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func order(t *testing.T, db *DB, parent string) []string {
	t.Helper()
	kids, err := db.Children(context.Background(), parent, anna.ID)
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
	ctx := context.Background()
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
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	archive, _ := db.CreateCollection(ctx, anna, "Archive", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	if _, err := db.pool.Exec(ctx, "UPDATE objects SET access = 'read' WHERE id = $1", archive.ID); err != nil {
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
	if _, err := db.Move(ctx, other.ID, bob.ID, annasNotes[0].ID, ""); !errors.Is(err, ErrNotFound) {
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
