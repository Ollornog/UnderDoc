// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// thread writes the comments of a page as "block author: body", replies indented, resolved threads marked, blocks that are
// gone marked.
func thread(comments []Comment) []string {
	var out []string
	for _, c := range comments {
		line := fmt.Sprintf("%s %s: %s", c.Block, c.AuthorName, c.Body)
		if c.Parent != 0 {
			line = "  " + line
		}
		if !c.Resolved.IsZero() {
			line += " (resolved by " + c.ResolvedBy + ")"
		}
		if c.Gone {
			line += " (gone)"
		}
		out = append(out, line)
	}
	return out
}

func setAccess(t *testing.T, db *DB, objectID, access string) {
	t.Helper()
	if _, err := db.pool.Exec(context.Background(), "UPDATE objects SET access = nullif($2, '') WHERE id = $1", objectID, access); err != nil {
		t.Fatal(err)
	}
}

func TestCommentsOnAPageAndItsBlocks(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	b1, _ := db.AddBlock(ctx, page.ID, anna.ID, "", "# Plan")
	b2, _ := db.AddBlock(ctx, page.ID, anna.ID, b1.ID, "Text")

	whole, err := db.AddComment(ctx, page.ID, anna.ID, "", "  Looks good.  ", 0)
	if err != nil || whole.Block != "" || whole.Body != "Looks good." || whole.AuthorName != "Anna Berg" || whole.Created.IsZero() {
		t.Fatalf("a comment on the whole page, trimmed: %+v %v", whole, err)
	}
	onText, err := db.AddComment(ctx, page.ID, bob.ID, b2.ID, "Which rack?", 0)
	if err != nil || onText.Block != b2.ID {
		t.Fatalf("a comment on a block: %+v %v", onText, err)
	}
	reply, err := db.AddComment(ctx, page.ID, anna.ID, "b1", "Rack 2.", onText.ID)
	if err != nil || reply.Block != b2.ID || reply.Parent != onText.ID {
		t.Fatalf("a reply lands on the block of what it answers, whatever block is sent: %+v %v", reply, err)
	}
	if _, err := db.AddComment(ctx, page.ID, bob.ID, "", "Deeper", reply.ID); !errors.Is(err, ErrPlace) {
		t.Errorf("replies go one level deep: %v", err)
	}
	if _, err := db.AddComment(ctx, page.ID, bob.ID, "b9", "Nowhere", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("a comment on a block that is not there: %v", err)
	}
	if _, err := db.AddComment(ctx, page.ID, bob.ID, "", "x", 999999); !errors.Is(err, ErrNotFound) {
		t.Errorf("a reply to nothing: %v", err)
	}
	other, _ := db.CreatePage(ctx, anna, "Other", team.ID)
	if _, err := db.AddComment(ctx, other.ID, bob.ID, "", "Cross", onText.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a reply to a comment of another page: %v", err)
	}
	for _, body := range []string{"", "   ", strings.Repeat("ä", MaxComment+1)} {
		if _, err := db.AddComment(ctx, page.ID, bob.ID, "", body, 0); !errors.Is(err, ErrComment) {
			t.Errorf("a comment of %d characters: %v", len([]rune(body)), err)
		}
	}
	if _, err := db.AddComment(ctx, page.ID, bob.ID, "", strings.Repeat("ä", MaxComment), 0); err != nil {
		t.Errorf("just long enough: %v", err)
	}
	if _, err := db.AddComment(ctx, team.ID, anna.ID, "", "On a collection", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("comments belong to pages: %v", err)
	}

	if _, err := db.AddComment(ctx, page.ID, anna.ID, b1.ID, "Heading ok?", 0); err != nil {
		t.Fatal(err)
	}
	all, err := db.Comments(ctx, page.ID, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{" Anna Berg: Looks good.", "b2 Bob: Which rack?", "  b2 Anna Berg: Rack 2.", " Bob: " + strings.Repeat("ä", MaxComment), "b1 Anna Berg: Heading ok?"}
	if got := thread(all); !slices.Equal(got, want) {
		t.Errorf("threads in the order they began, replies right after:\n%q", got)
	}

	// A deleted block keeps its comments; they say that the block is gone. Its ID is never given out again.
	if err := db.DeleteBlock(ctx, page.ID, b2.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	all, _ = db.Comments(ctx, page.ID, anna.ID)
	if got := thread(all); got[1] != "b2 Bob: Which rack? (gone)" || got[2] != "  b2 Anna Berg: Rack 2. (gone)" || strings.Contains(got[4], "gone") {
		t.Errorf("comments on a block that is gone: %q", got)
	}
	if _, err := db.AddComment(ctx, page.ID, bob.ID, b2.ID, "New on gone", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("no new thread on a block that is gone: %v", err)
	}
	if _, err := db.AddComment(ctx, page.ID, bob.ID, "", "Still answered", onText.ID); err != nil {
		t.Errorf("a thread on a block that is gone can still be answered: %v", err)
	}

	open, err := db.OpenComments(ctx, page.ID, bob.ID)
	if err != nil || open[""] != 2 || open[b2.ID] != 3 || open[b1.ID] != 1 {
		t.Errorf("open comments by block: %v %v", open, err)
	}
	if _, err := db.ResolveComment(ctx, onText.ID, bob.ID, true); err != nil {
		t.Fatal(err)
	}
	if open, _ := db.OpenComments(ctx, page.ID, bob.ID); open[b2.ID] != 0 || open[""] != 2 {
		t.Errorf("a resolved thread counts no more, its replies neither: %v", open)
	}
	if _, err := db.ResolveComment(ctx, reply.ID, anna.ID, true); !errors.Is(err, ErrPlace) {
		t.Errorf("a reply is resolved with its thread: %v", err)
	}
	if c, err := db.ResolveComment(ctx, onText.ID, anna.ID, false); err != nil || !c.Resolved.IsZero() || c.ResolvedBy != "" {
		t.Errorf("opened again: %+v %v", c, err)
	}

	// The page in the trash: its comments are not to be found, and back again they are there.
	if err := db.Delete(ctx, page.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Comments(ctx, page.ID, anna.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("comments of a page in the trash: %v", err)
	}
	if _, err := db.AddComment(ctx, page.ID, anna.ID, "", "x", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("a comment on a page in the trash: %v", err)
	}
	if _, err := db.EditComment(ctx, whole.ID, anna.ID, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("editing a comment of a page in the trash: %v", err)
	}
	if _, err := db.Restore(ctx, page.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if all, _ := db.Comments(ctx, page.ID, anna.ID); len(all) != 6 {
		t.Errorf("restored with its comments: %q", thread(all))
	}
	if err := db.Delete(ctx, page.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Purge(ctx, page.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM comments WHERE page = $1", page.ID).Scan(&left); err != nil || left != 0 {
		t.Errorf("deleted for good, its comments go with it: %d %v", left, err)
	}
}

// Who may do what with comments follows ADR-20: read sees them, comment adds, edit resolves those of others; the owner
// always may; a private page of someone else has none to show.
func TestCommentsFollowAccess(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	block, _ := db.AddBlock(ctx, page.ID, anna.ID, "", "Text")
	first, _ := db.AddComment(ctx, page.ID, anna.ID, block.ID, "Anna's", 0)

	setAccess(t, db, team.ID, "read")
	if all, err := db.Comments(ctx, page.ID, bob.ID); err != nil || len(all) != 1 {
		t.Errorf("read only still sees the comments: %v %v", all, err)
	}
	if _, err := db.AddComment(ctx, page.ID, bob.ID, "", "Bob's", 0); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("read only means no comment: %v", err)
	}
	if _, err := db.AddComment(ctx, page.ID, bob.ID, "", "Bob's reply", first.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("and no reply: %v", err)
	}
	if db.MayComment(ctx, page, bob.ID) || !db.MayComment(ctx, page, anna.ID) {
		t.Error("MayComment: bob may not, the owner may")
	}
	if _, err := db.AddComment(ctx, page.ID, anna.ID, "", "The owner always may", 0); err != nil {
		t.Errorf("the owner comments on a page set to read only: %v", err)
	}

	setAccess(t, db, team.ID, "comment")
	bobs, err := db.AddComment(ctx, page.ID, bob.ID, block.ID, "Bob's", 0)
	if err != nil {
		t.Fatalf("comment adds comments to reading: %v", err)
	}
	if _, err := db.AddComment(ctx, page.ID, bob.ID, "", "Bob answers", first.ID); err != nil {
		t.Errorf("and replies: %v", err)
	}
	if _, err := db.ResolveComment(ctx, first.ID, bob.ID, true); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("one who may only comment does not resolve the thread of another: %v", err)
	}
	if c, err := db.ResolveComment(ctx, bobs.ID, bob.ID, true); err != nil || c.Resolved.IsZero() || c.ResolvedBy != "Bob" {
		t.Errorf("but resolves his own: %+v %v", c, err)
	}
	if c, err := db.ResolveComment(ctx, bobs.ID, anna.ID, false); err != nil || !c.Resolved.IsZero() {
		t.Errorf("the owner of the page opens it again: %+v %v", c, err)
	}
	if _, err := db.EditComment(ctx, first.ID, bob.ID, "Changed by Bob"); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob edits Anna's comment: %v", err)
	}
	if err := db.DeleteComment(ctx, first.ID, bob.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob deletes Anna's comment: %v", err)
	}
	if c, err := db.EditComment(ctx, bobs.ID, bob.ID, "Bob's, better"); err != nil || c.Body != "Bob's, better" || c.Edited.IsZero() {
		t.Errorf("bob edits his own: %+v %v", c, err)
	}
	if !MayResolve(bobs, bob.ID, true, false) || MayResolve(first, bob.ID, true, false) || !MayResolve(first, bob.ID, true, true) || MayResolve(bobs, bob.ID, false, false) {
		t.Error("MayResolve: the author while commenting, or whoever may edit")
	}

	setAccess(t, db, team.ID, "read")
	if _, err := db.EditComment(ctx, bobs.ID, bob.ID, "Bob's, again"); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("read only: not even his own comment changes: %v", err)
	}
	if err := db.DeleteComment(ctx, bobs.ID, bob.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("read only: nor is it deleted: %v", err)
	}
	if _, err := db.ResolveComment(ctx, bobs.ID, bob.ID, true); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("read only: nor resolved: %v", err)
	}

	setAccess(t, db, team.ID, "edit")
	if c, err := db.ResolveComment(ctx, first.ID, bob.ID, true); err != nil || c.ResolvedBy != "Bob" {
		t.Errorf("whoever may edit the page resolves the thread of another: %+v %v", c, err)
	}

	notes, _ := db.Collections(ctx, anna.ID)
	diary, _ := db.CreatePage(ctx, anna, "Diary", notes[0].ID)
	secret, _ := db.AddComment(ctx, diary.ID, anna.ID, "", "Secret", 0)
	if _, err := db.Comments(ctx, diary.ID, bob.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob reads the comments of Anna's private page: %v", err)
	}
	if _, err := db.OpenComments(ctx, diary.ID, bob.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("nor counts them: %v", err)
	}
	if _, err := db.AddComment(ctx, diary.ID, bob.ID, "", "Mine", 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("nor adds one: %v", err)
	}
	if _, err := db.ResolveComment(ctx, secret.ID, bob.ID, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("nor resolves one by its ID: %v", err)
	}
	if _, err := db.EditComment(ctx, 999999, anna.ID, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a comment that is not there: %v", err)
	}
}

func TestDeletingComments(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	mine, _ := db.AddComment(ctx, page.ID, bob.ID, "", "Bob asks", 0)
	if _, err := db.AddComment(ctx, page.ID, bob.ID, "", "Bob adds", mine.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteComment(ctx, mine.ID, bob.ID); err != nil {
		t.Fatalf("his own, with his own reply: %v", err)
	}
	if all, _ := db.Comments(ctx, page.ID, anna.ID); len(all) != 0 {
		t.Errorf("gone with its reply: %q", thread(all))
	}
	asked, _ := db.AddComment(ctx, page.ID, bob.ID, "", "Bob asks again", 0)
	answer, _ := db.AddComment(ctx, page.ID, anna.ID, "", "Anna answers", asked.ID)
	if err := db.DeleteComment(ctx, asked.ID, bob.ID); !errors.Is(err, ErrHasReplies) {
		t.Errorf("answered by others, it stays: %v", err)
	}
	if err := db.DeleteComment(ctx, answer.ID, anna.ID); err != nil {
		t.Errorf("a reply of one's own goes: %v", err)
	}
	if err := db.DeleteComment(ctx, asked.ID, bob.ID); err != nil {
		t.Errorf("without the reply of another, it goes: %v", err)
	}
	if err := db.DeleteComment(ctx, asked.ID, bob.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted twice: %v", err)
	}
}
