// SPDX-License-Identifier: MIT

package share

// Attack round on the working copy (ADR-30), with the stand-in companion. Each test is red while the weakness it names
// is open and turns green with a correct fix.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
)

// A file saved over SMB a moment before the share of its collection is switched off is lost: the pass that follows
// takes the directory of a library that is no longer shared off the disk (dropUnshared) without first taking in what
// changed there since the last pass. "A library that stops being shared leaves the disk; the store keeps everything"
// holds only for what reached the store.
func TestAttackUnshareDropsUnsyncedFile(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	team, err := f.db.CreateFileCollection(ctx, anna, "Team")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetRootSamba(ctx, team.ID, anna.ID, true); err != nil {
		t.Fatal(err)
	}
	f.sync()
	dir := filepath.Join(f.dir, team.ID)
	// Anna saves a new file over SMB; before the quiet time is over, the share is switched off.
	if err := os.WriteFile(filepath.Join(dir, "minutes.txt"), []byte("what was decided"), 0o660); err != nil {
		t.Fatal(err)
	}
	f.h.wrote("anna", filepath.Join(dir, "minutes.txt"))
	if err := f.db.SetRootSamba(ctx, team.ID, anna.ID, false); err != nil {
		t.Fatal(err)
	}
	f.sync()
	list, err := f.db.FilesIn(ctx, team.ID, anna.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range list {
		if x.Title == "minutes.txt" {
			return
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "minutes.txt")); err == nil {
		return // still on disk: not lost
	}
	t.Errorf("minutes.txt, saved over SMB, is neither in the database nor on disk after the share went off: %+v", list)
}

// The same for the folder of a user: an administrator removes the app password of an account (or the account does),
// and whatever it saved since the last pass is gone with its [homes].
func TestAttackRemovedAppPasswordDropsUnsyncedFile(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	if err := f.db.EnsureRoots(ctx, bob, false, "admins"); err != nil {
		t.Fatal(err)
	}
	f.sync()
	roots, err := f.db.FileRoots(ctx, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	var home db.Root
	for _, r := range roots {
		if r.Role == db.RootUser {
			home = r
		}
	}
	if home.ID == "" {
		t.Fatal("bob has no folder of his own")
	}
	dir := filepath.Join(f.dir, home.ID)
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("bob's folder is not on the share: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "thesis.txt"), []byte("chapter one"), 0o660); err != nil {
		t.Fatal(err)
	}
	f.h.wrote("bob", filepath.Join(dir, "thesis.txt"))
	if err := f.db.RemoveSambaAccount(ctx, db.Actor{ID: anna.ID, Admin: true}, bob.ID); err != nil {
		t.Fatal(err)
	}
	f.sync()
	list, err := f.db.FilesIn(ctx, home.ID, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range list {
		if x.Title == "thesis.txt" {
			return
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "thesis.txt")); err == nil {
		return
	}
	t.Errorf("thesis.txt, saved over SMB into bob's folder, is gone after his app password was removed: %+v", list)
}
