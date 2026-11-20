// SPDX-License-Identifier: MIT

package db

// Attack round on passwords in the text (ADR-7). Each test is red while the weakness it names is open and turns green
// with a correct fix.

import (
	"errors"
	"strings"
	"testing"
)

// An editor of a page who may reveal its secret moves the block that holds it into a page of his private notes. The
// secret goes along (moveSecrets), so the team loses it; and since the entry "secret moved" is written on the page it
// goes to, and nothing of a private page is logged, the page it came from keeps no trace of where its secret went.
// ADR-7: "The log names … moving … a secret, each time, on its page".
func TestAttackSecretMovedIntoPrivateLeavesNoTrace(t *testing.T) {
	db := open(t)
	ctx := inMain()
	ops, page := secretPage(t, db)
	if err := db.SetGrant(ctx, ops.ID, Actor{ID: anna.ID}, ToPerson, bob.ID, RoleEdit); err != nil {
		t.Fatal(err)
	}
	secretID, err := db.CreatePageSecret(ctx, page.ID, anna.ID, fakeSeal(randomValue(t)))
	if err != nil {
		t.Fatal(err)
	}
	b, err := db.AddBlock(ctx, page.ID, anna.ID, "", "Switch admin: ::secret{id=\""+secretID+"\"}")
	if err != nil {
		t.Fatal(err)
	}
	collections, err := db.Collections(ctx, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	var mine Object
	for _, c := range collections {
		if c.Private && c.Owner == bob.ID {
			mine = c
		}
	}
	if mine.ID == "" {
		t.Fatal("bob has no private notes")
	}
	stash, err := db.CreatePage(ctx, bob, "Stash", mine.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.MoveBlockTo(ctx, page.ID, b.ID, bob.ID, stash.ID, ""); err != nil {
		return // refusing to take a shared secret into something private is a correct answer
	}
	if _, err := reveal(db, secretID, carl.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the secret did not leave the team: %v", err)
	}
	entries, err := db.LogEntries(ctx, Actor{ID: anna.ID, Admin: true}, LogFilter{Object: page.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Action == LogSecretMoved && strings.Contains(e.Detail, secretID) {
			return
		}
	}
	t.Errorf("bob took the secret %s of %q into his private notes, and its page logs nothing of it: %+v", secretID, page.Title, entries)
}
