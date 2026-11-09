// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestDeleteHidesTheTree(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	a, _ := db.CreatePage(ctx, anna, "A", team.ID)
	b, _ := db.CreatePage(ctx, anna, "B", team.ID)
	kid, _ := db.CreatePage(ctx, anna, "Kid", a.ID)
	if _, err := db.AddBlock(ctx, kid.ID, anna.ID, "", "Below."); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(ctx, a.ID, bob.ID); err != nil {
		t.Fatal(err)
	}
	if got := order(t, db, team.ID); !slices.Equal(got, []string{"B"}) {
		t.Errorf("A is gone from the list: %q", got)
	}
	for _, id := range []string{a.ID, kid.ID} {
		if _, err := db.Object(ctx, id, anna.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s still opens: %v", id, err)
		}
	}
	if _, err := db.Blocks(ctx, kid.ID, anna.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the blocks of the child still read: %v", err)
	}
	if _, err := db.Rename(ctx, a.ID, anna.ID, "Again"); !errors.Is(err, ErrNotFound) {
		t.Errorf("something in the trash is renamed: %v", err)
	}
	if _, err := db.CreatePage(ctx, anna, "Into the trash", a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a page made in something in the trash: %v", err)
	}
	if _, err := db.Move(ctx, b.ID, anna.ID, team.ID, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a move after something in the trash: %v", err)
	}
	if err := db.Delete(ctx, a.ID, anna.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted twice: %v", err)
	}
	made, err := db.Copy(ctx, team.ID, anna.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := order(t, db, made.ID); !slices.Equal(got, []string{"B"}) {
		t.Errorf("a copy leaves out what is in the trash: %q", got)
	}
	if err := db.Delete(ctx, team.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	tops, _ := db.Collections(ctx, anna.ID)
	if got := titles(tops); slices.Contains(got, "Team") {
		t.Errorf("a collection at the top in the trash is still listed: %q", got)
	}
	trash, err := db.Trash(ctx, anna.ID)
	if err != nil {
		t.Fatal(err)
	}
	var listed []string
	for _, x := range trash {
		listed = append(listed, x.Title+"@"+x.Place)
	}
	// Each deletion once, newest first: Team with B, then A with its child, whose place went later.
	if !slices.Equal(listed, []string{"Team@", "A@Team"}) {
		t.Errorf("the trash: %q", listed)
	}
	if trash[0].DeletedBy != "Anna Berg" || trash[1].DeletedBy != "Bob" || !trash[1].PlaceDeleted || trash[0].PlaceDeleted {
		t.Errorf("who and whether its place is in the trash: %+v", trash)
	}
}

func TestDeleteFollowsAccess(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	inner, _ := db.CreateCollection(ctx, anna, "Inner", team.ID)
	archive, _ := db.CreateCollection(ctx, anna, "Archive", "")
	old, _ := db.CreatePage(ctx, anna, "Old", archive.ID)
	if _, err := db.pool.Exec(ctx, "UPDATE objects SET access = 'read' WHERE id = $1", archive.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(ctx, old.ID, bob.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob deletes in a collection he may only read: %v", err)
	}
	if err := db.Delete(ctx, team.ID, bob.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob deletes a collection at the top that is not his: %v", err)
	}
	if err := db.Delete(ctx, inner.ID, bob.ID); err != nil {
		t.Errorf("bob deletes a collection inside one he may edit: %v", err)
	}
	notes, _ := db.Collections(ctx, anna.ID)
	if err := db.Delete(ctx, notes[0].ID, anna.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("Anna deletes her private notes: %v", err)
	}
	if err := db.Delete(ctx, notes[0].ID, bob.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob deletes Anna's private notes: %v", err)
	}
	secret, _ := db.CreatePage(ctx, anna, "Secret", notes[0].ID)
	if err := db.Delete(ctx, secret.ID, anna.ID); err != nil {
		t.Errorf("a page in her own notes: %v", err)
	}
	// Private notes do not move into a shared collection any more; should they lie in one from before, they do not go with
	// it either.
	if _, err := db.Move(ctx, notes[0].ID, anna.ID, team.ID, ""); !errors.Is(err, ErrPlace) {
		t.Errorf("Anna moves her private notes into Team: %v", err)
	}
	if _, err := db.pool.Exec(ctx, "UPDATE objects SET parent = $2 WHERE id = $1", notes[0].ID, team.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(ctx, team.ID, anna.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("deleting what holds private notes: %v", err)
	}
	if trash, _ := db.Trash(ctx, bob.ID); len(trash) != 1 || trash[0].Title != "Inner" {
		t.Errorf("bob's trash holds what he deleted: %+v", trash)
	}
	if trash, _ := db.Trash(ctx, anna.ID); len(trash) != 2 {
		t.Errorf("Anna's trash holds what she owns and what she deleted: %+v", trash)
	}
	line, _ := db.CreateDivider(ctx, anna, KindLine, archive.ID, "")
	if err := db.Delete(ctx, line.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if trash, _ := db.Trash(ctx, anna.ID); len(trash) != 2 {
		t.Errorf("a line goes for good, not into the trash: %+v", trash)
	}
	var lines int
	if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM objects WHERE id = $1", line.ID).Scan(&lines); err != nil || lines != 0 {
		t.Errorf("the line is still stored: %d %v", lines, err)
	}
}

func TestRestore(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	a, _ := db.CreatePage(ctx, anna, "A", team.ID)
	if _, err := db.CreatePage(ctx, anna, "B", team.ID); err != nil {
		t.Fatal(err)
	}
	kid, _ := db.CreatePage(ctx, anna, "Kid", a.ID)
	early, _ := db.CreatePage(ctx, anna, "Early", a.ID)
	if _, err := db.AddBlock(ctx, kid.ID, anna.ID, "", "Below."); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(ctx, early.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(ctx, a.ID, bob.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Restore(ctx, kid.ID, anna.ID); !errors.Is(err, ErrPlace) || !errors.Is(err, ErrParentDeleted) {
		t.Errorf("something that went with its parent comes back alone: %v", err)
	}
	if _, err := db.Restore(ctx, a.ID, "u-nobody"); !errors.Is(err, ErrNotFound) {
		t.Errorf("someone who neither owns nor deleted it: %v", err)
	}
	back, err := db.Restore(ctx, a.ID, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if back.Parent != team.ID {
		t.Errorf("back where it lay: %+v", back)
	}
	if got := order(t, db, team.ID); !slices.Equal(got, []string{"B", "A"}) {
		t.Errorf("back at the end: %q", got)
	}
	if got := order(t, db, a.ID); !slices.Equal(got, []string{"Kid"}) {
		t.Errorf("what went with it comes back, what was deleted before stays: %q", got)
	}
	if blocks, _ := db.Blocks(ctx, kid.ID, anna.ID); len(blocks) != 1 || blocks[0].Body != "Below." {
		t.Errorf("the blocks come back: %+v", blocks)
	}
	if trash, _ := db.Trash(ctx, anna.ID); len(trash) != 1 || trash[0].Title != "Early" || trash[0].Place != "Team › A" || trash[0].PlaceDeleted {
		t.Errorf("the trash after restoring: %+v", trash)
	}
	if _, err := db.Restore(ctx, a.ID, anna.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("restored twice: %v", err)
	}
	// What lay in a collection that is in the trash waits for it; once that is back, it comes back too.
	if err := db.Delete(ctx, team.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Restore(ctx, early.ID, anna.ID); !errors.Is(err, ErrParentDeleted) {
		t.Errorf("restored under a parent in the trash: %v", err)
	}
	if _, err := db.Restore(ctx, team.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Restore(ctx, early.ID, anna.ID); err != nil {
		t.Errorf("after its place is back: %v", err)
	}
	if got := order(t, db, a.ID); !slices.Equal(got, []string{"Kid", "Early"}) {
		t.Errorf("Early back at the end of A: %q", got)
	}
	// Into a place that is read only by now, only its owner restores.
	if err := db.Delete(ctx, early.ID, bob.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.pool.Exec(ctx, "UPDATE objects SET access = 'read' WHERE id = $1", team.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Restore(ctx, early.ID, bob.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob restores into a place he may only read: %v", err)
	}
	if _, err := db.Restore(ctx, early.ID, anna.ID); err != nil {
		t.Errorf("the owner restores: %v", err)
	}
}

func TestPurge(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	a, _ := db.CreatePage(ctx, anna, "A", team.ID)
	kid, _ := db.CreatePage(ctx, anna, "Kid", a.ID)
	early, _ := db.CreatePage(ctx, anna, "Early", a.ID)
	if _, err := db.AddBlock(ctx, kid.ID, anna.ID, "", "Below."); err != nil {
		t.Fatal(err)
	}
	if err := db.Purge(ctx, a.ID, anna.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("purged without being in the trash: %v", err)
	}
	if err := db.Delete(ctx, early.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(ctx, a.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Purge(ctx, a.ID, bob.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob purges what he neither owns nor deleted: %v", err)
	}
	if err := db.Purge(ctx, a.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"objects WHERE id = ANY($1)", "blocks WHERE page = ANY($1)", "block_versions WHERE page = ANY($1)"} {
		var n int
		if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM "+table, []string{a.ID, kid.ID, early.ID}).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s: %d left, %v", table, n, err)
		}
	}
	if trash, _ := db.Trash(ctx, anna.ID); len(trash) != 0 {
		t.Errorf("the trash is empty, also of what lay below: %+v", trash)
	}
	if got := order(t, db, team.ID); len(got) != 0 {
		t.Errorf("the collection stays, empty: %q", got)
	}
	// Whoever deleted it may purge it, also when it is not theirs.
	b, _ := db.CreatePage(ctx, anna, "B", team.ID)
	if err := db.Delete(ctx, b.ID, bob.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Purge(ctx, b.ID, bob.ID); err != nil {
		t.Errorf("bob purges what he deleted: %v", err)
	}
}

func TestEmptyingAndExpiringTheTrash(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	a, _ := db.CreatePage(ctx, anna, "A", team.ID)
	kid, _ := db.CreatePage(ctx, anna, "Kid", a.ID)
	b, _ := db.CreatePage(ctx, anna, "B", team.ID)
	c, _ := db.CreatePage(ctx, bob, "C", team.ID)
	for _, x := range []string{a.ID, b.ID} {
		if err := db.Delete(ctx, x, anna.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Delete(ctx, c.ID, bob.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := db.EmptyTrash(ctx, anna.ID); err != nil || n != 2 {
		t.Fatalf("Anna empties her trash: %d %v", n, err)
	}
	var left []string
	rows, _ := db.pool.Query(ctx, "SELECT title FROM objects WHERE id IN ($1, $2, $3, $4) ORDER BY title", a.ID, kid.ID, b.ID, c.ID)
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		left = append(left, s)
	}
	if len(left) != 1 || left[0] != "C" {
		t.Errorf("A with its child and B are gone, Bob's C waits in his trash: %v", left)
	}
	// What lies in the trash longer than TrashDays goes by itself, for everyone; what came later stays.
	d, _ := db.CreatePage(ctx, anna, "D", team.ID)
	if err := db.Delete(ctx, d.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	keep, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { db.KeepTrash(keep, 10*time.Millisecond, func(err error) { t.Error(err) }); close(done) }()
	// It keeps at it: C turns old only after the first round.
	time.Sleep(50 * time.Millisecond)
	if _, err := db.pool.Exec(ctx, "UPDATE objects SET deleted_at = now() - interval '31 days' WHERE id = $1", c.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var n int
		_ = db.pool.QueryRow(ctx, "SELECT count(*) FROM objects WHERE id = $1", c.ID).Scan(&n)
		if n == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop()
	<-done
	if list, _ := db.Trash(ctx, bob.ID); len(list) != 0 {
		t.Errorf("C went after 30 days: %+v", list)
	}
	if list, _ := db.Trash(ctx, anna.ID); len(list) != 1 || list[0].Title != "D" {
		t.Errorf("D, deleted just now, stays: %+v", list)
	}
}
