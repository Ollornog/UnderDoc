// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func bodies(blocks []Block) []string {
	var out []string
	for _, b := range blocks {
		out = append(out, b.ID+":"+b.Body)
	}
	return out
}

func TestPagesAreBlocks(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, err := db.CreatePage(ctx, anna, "Plan", team.ID)
	if err != nil || page.Kind != KindPage || page.Parent != team.ID {
		t.Fatalf("page not made: %+v %v", page, err)
	}
	if blocks, err := db.Blocks(ctx, page.ID, anna.ID); err != nil || len(blocks) != 0 {
		t.Fatalf("a new page is empty: %v %v", blocks, err)
	}
	first, err := db.AddBlock(ctx, page.ID, anna.ID, "", "# Plan")
	if err != nil || first.ID != "b1" {
		t.Fatalf("first block: %+v %v", first, err)
	}
	third, _ := db.AddBlock(ctx, page.ID, anna.ID, first.ID, "| a | b |\n|---|---|\n| 1 | 2 |")
	second, _ := db.AddBlock(ctx, page.ID, anna.ID, first.ID, "Some text.")
	blocks, _ := db.Blocks(ctx, page.ID, anna.ID)
	if want := []string{"b1:# Plan", "b3:Some text.", "b2:| a | b |\n|---|---|\n| 1 | 2 |"}; !slices.Equal(bodies(blocks), want) {
		t.Errorf("order after inserting in the middle: %q", bodies(blocks))
	}
	if _, err := db.EditBlock(ctx, page.ID, second.ID, bob.ID, "Better text."); err != nil {
		t.Fatalf("others may edit by default: %v", err)
	}
	if _, err := db.EditBlock(ctx, page.ID, second.ID, bob.ID, "Better text."); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteBlock(ctx, page.ID, third.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	fourth, _ := db.AddBlock(ctx, page.ID, anna.ID, second.ID, "New.")
	if fourth.ID != "b4" {
		t.Errorf("a deleted block's ID is not given out again: %s", fourth.ID)
	}
	blocks, _ = db.Blocks(ctx, page.ID, anna.ID)
	if want := []string{"b1:# Plan", "b3:Better text.", "b4:New."}; !slices.Equal(bodies(blocks), want) {
		t.Errorf("after edit and delete: %q", bodies(blocks))
	}
	versions, err := db.Versions(ctx, page.ID, anna.ID, 50)
	if err != nil {
		t.Fatal(err)
	}
	var log []string
	for _, v := range versions {
		entry := v.Block + " " + v.Author + " " + v.Body
		if v.Deleted {
			entry = v.Block + " " + v.Author + " deleted"
		}
		log = append(log, entry)
	}
	want := []string{"b4 Anna Berg New.", "b2 Anna Berg deleted", "b3 Bob Better text.", "b3 Anna Berg Some text.", "b2 Anna Berg | a | b |\n|---|---|\n| 1 | 2 |", "b1 Anna Berg # Plan"}
	if !slices.Equal(log, want) {
		t.Errorf("versions, newest first, the same text twice only once:\n%q", log)
	}
}

func TestBlocksFollowAccess(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	block, _ := db.AddBlock(ctx, page.ID, anna.ID, "", "Text")
	if _, err := db.pool.Exec(ctx, "UPDATE objects SET access = 'comment' WHERE id = $1", team.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.EditBlock(ctx, page.ID, block.ID, bob.ID, "Mine"); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob may only comment, inherited from the collection: %v", err)
	}
	if _, err := db.AddBlock(ctx, page.ID, bob.ID, "", "Mine"); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("no new block either: %v", err)
	}
	if err := db.DeleteBlock(ctx, page.ID, block.ID, bob.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("no deleting either: %v", err)
	}
	if _, err := db.CreatePage(ctx, bob, "Mine", team.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("no new page in it: %v", err)
	}
	if _, err := db.CreateCollection(ctx, bob, "Mine", team.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("no new collection in it: %v", err)
	}
	if _, err := db.EditBlock(ctx, page.ID, block.ID, anna.ID, "Still mine"); err != nil {
		t.Errorf("the owner may always: %v", err)
	}
	if blocks, err := db.Blocks(ctx, page.ID, bob.ID); err != nil || len(blocks) != 1 || blocks[0].Body != "Still mine" {
		t.Errorf("bob still reads it: %v %v", blocks, err)
	}

	notes, _ := db.Collections(ctx, anna.ID)
	private, _ := db.CreatePage(ctx, anna, "Diary", notes[0].ID)
	if _, err := db.Blocks(ctx, private.ID, bob.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob does not see a page in Anna's private notes: %v", err)
	}
	if _, err := db.AddBlock(ctx, private.ID, bob.ID, "", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("nor write into it: %v", err)
	}
	if _, err := db.Versions(ctx, private.ID, bob.ID, 10); !errors.Is(err, ErrNotFound) {
		t.Errorf("nor read its versions: %v", err)
	}
	if _, err := db.CreatePage(ctx, bob, "Mine", notes[0].ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("nor make a page in it: %v", err)
	}
	if _, err := db.Blocks(ctx, team.ID, anna.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a collection has no blocks: %v", err)
	}
}

func TestBlockLimits(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	if _, err := db.AddBlock(ctx, page.ID, anna.ID, "", strings.Repeat("ä", MaxBlock+1)); !errors.Is(err, ErrBlock) {
		t.Errorf("too long: %v", err)
	}
	if _, err := db.AddBlock(ctx, page.ID, anna.ID, "", strings.Repeat("ä", MaxBlock)); err != nil {
		t.Errorf("just long enough: %v", err)
	}
	if _, err := db.AddBlock(ctx, page.ID, anna.ID, "b99", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after a block that is not there: %v", err)
	}
	if _, err := db.EditBlock(ctx, page.ID, "b99", anna.ID, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("editing a block that is not there: %v", err)
	}
	if _, err := db.CreatePage(ctx, anna, " ", team.ID); !errors.Is(err, ErrTitle) {
		t.Errorf("a page needs a title: %v", err)
	}
}

func TestMovingBlocks(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	after := ""
	for _, body := range []string{"one", "two", "three"} {
		b, _ := db.AddBlock(ctx, page.ID, anna.ID, after, body)
		after = b.ID
	}
	order := func() string {
		blocks, _ := db.Blocks(ctx, page.ID, anna.ID)
		var out []string
		for _, b := range blocks {
			out = append(out, b.ID)
		}
		return strings.Join(out, " ")
	}
	for _, step := range []struct{ block, after, want string }{
		{"b3", "", "b3 b1 b2"},
		{"b3", "b1", "b1 b3 b2"},
		{"b1", "b2", "b3 b2 b1"},
		{"b2", "b1", "b3 b1 b2"},
	} {
		if err := db.MoveBlock(ctx, page.ID, step.block, anna.ID, step.after); err != nil {
			t.Fatal(err)
		}
		if got := order(); got != step.want {
			t.Errorf("%s after %q: %s, want %s", step.block, step.after, got, step.want)
		}
	}
	versions, _ := db.Versions(ctx, page.ID, anna.ID, 100)
	if len(versions) != 3 {
		t.Errorf("moving makes no version: %d", len(versions))
	}
	if err := db.MoveBlock(ctx, page.ID, "b9", anna.ID, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("a block that is not there: %v", err)
	}
	if err := db.MoveBlock(ctx, page.ID, "b1", anna.ID, "b9"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after a block that is not there: %v", err)
	}
	if err := db.MoveBlock(ctx, page.ID, "b1", anna.ID, "b1"); !errors.Is(err, ErrPlace) {
		t.Errorf("after itself: %v", err)
	}
	if _, err := db.SetAccess(ctx, team.ID, anna.ID, "read"); err != nil {
		t.Fatal(err)
	}
	if err := db.MoveBlock(ctx, page.ID, "b1", bob.ID, ""); err == nil || order() != "b3 b1 b2" {
		t.Errorf("only who may edit moves blocks: %v %s", err, order())
	}
}
