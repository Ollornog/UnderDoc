// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestCopyTakesTheTreeAndTheBlocks(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	a, _ := db.CreatePage(ctx, anna, "A", team.ID)
	if _, err := db.CreatePage(ctx, anna, "B", team.ID); err != nil {
		t.Fatal(err)
	}
	kid, _ := db.CreatePage(ctx, anna, "Kid", a.ID)
	first, _ := db.AddBlock(ctx, a.ID, anna.ID, "", "## Plan")
	if _, err := db.AddBlock(ctx, a.ID, anna.ID, first.ID, "Two switches."); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteBlock(ctx, a.ID, first.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddBlock(ctx, kid.ID, anna.ID, "", "Below."); err != nil {
		t.Fatal(err)
	}
	made, err := db.Copy(ctx, a.ID, bob.ID, team.ID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if made.Title != "A (copy)" || made.Owner != bob.ID || made.Kind != KindPage {
		t.Errorf("the copy: %+v", made)
	}
	if got := order(t, db, team.ID); !slices.Equal(got, []string{"A", "A (copy)", "B"}) {
		t.Errorf("the copy lies right after A: %q", got)
	}
	blocks, _ := db.Blocks(ctx, made.ID, bob.ID)
	if len(blocks) != 1 || blocks[0].ID != "b2" || blocks[0].Body != "Two switches." {
		t.Errorf("the blocks keep their IDs: %+v", blocks)
	}
	if versions, _ := db.Versions(ctx, made.ID, bob.ID, 10); len(versions) != 1 || versions[0].Author != "Bob" || versions[0].Block != "b2" {
		t.Errorf("the copy starts its own history, by the one who copied: %+v", versions)
	}
	// The next block of the copy does not take an ID the original has given out.
	if added, _ := db.AddBlock(ctx, made.ID, bob.ID, "", "New."); added.ID != "b3" {
		t.Errorf("the next block of the copy is %q", added.ID)
	}
	kids, _ := db.Children(ctx, made.ID, bob.ID)
	if len(kids) != 1 || kids[0].Title != "Kid" || kids[0].Owner != bob.ID {
		t.Fatalf("the child is copied as it is: %+v", kids)
	}
	if got, _ := db.Blocks(ctx, kids[0].ID, bob.ID); len(got) != 1 || got[0].Body != "Below." {
		t.Errorf("the blocks of the child: %+v", got)
	}
	if blocks, _ := db.Blocks(ctx, a.ID, anna.ID); len(blocks) != 1 {
		t.Errorf("the original is untouched: %+v", blocks)
	}
	if _, err := db.Copy(ctx, made.ID, bob.ID, team.ID, ""); err != nil {
		t.Errorf("a copy of a copy: %v", err)
	}
	if got := order(t, db, team.ID); got[0] != "A (copy) (copy)" {
		t.Errorf("the copy of the copy is first: %q", got)
	}
	long := strings.Repeat("x", 200)
	if got := copyTitle(long); len([]rune(got)) != 200 || !strings.HasSuffix(got, " (copy)") {
		t.Errorf("a long title stays within 200: %d", len([]rune(got)))
	}
}

func TestCopyFollowsAccessAndLimits(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	archive, _ := db.CreateCollection(ctx, anna, "Archive", "")
	a, _ := db.CreatePage(ctx, anna, "A", team.ID)
	kid, _ := db.CreatePage(ctx, anna, "Kid", a.ID)
	if _, err := db.pool.Exec(ctx, "UPDATE objects SET access = 'read' WHERE id = $1", archive.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Copy(ctx, a.ID, bob.ID, archive.ID, ""); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob copies into a collection he may only read: %v", err)
	}
	if _, err := db.Copy(ctx, a.ID, anna.ID, kid.ID, ""); !errors.Is(err, ErrPlace) {
		t.Errorf("A into its own child: %v", err)
	}
	if _, err := db.Copy(ctx, a.ID, anna.ID, "", ""); !errors.Is(err, ErrPlace) {
		t.Errorf("a page at the top: %v", err)
	}
	annasNotes, _ := db.Collections(ctx, anna.ID)
	secret, _ := db.CreatePage(ctx, anna, "Secret", annasNotes[0].ID)
	if _, err := db.Copy(ctx, secret.ID, bob.ID, team.ID, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob copies a page of Anna's private notes: %v", err)
	}
	if _, err := db.Copy(ctx, team.ID, anna.ID, "", ""); err != nil {
		t.Errorf("a collection at the top: %v", err)
	}
	copyLimit = 1
	defer func() { copyLimit = MaxCopy }()
	if _, err := db.Copy(ctx, a.ID, anna.ID, team.ID, ""); !errors.Is(err, ErrTooMany) {
		t.Errorf("more than the limit: %v", err)
	}
}
