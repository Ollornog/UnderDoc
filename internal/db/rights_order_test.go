// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"testing"
	"time"
)

// Changes of rights on deep levels go one after the other (T-15). Breaking the inheritance deep down copies what holds
// from above: a deny given above while the break is under way must be copied, not lost to a copy read before it. A
// change of entries deep down decides whether its actor still manages the object after a change above finished.
func TestRightsChangesOnDeepLevelsGoOneAfterTheOther(t *testing.T) {
	db, ctx := setup(t)
	if err := db.SeeAccount(ctx, bob); err != nil {
		t.Fatal(err)
	}
	space, _ := db.createShared(ctx, anna, "Space")
	x, _ := db.CreateFolder(ctx, anna, "X", space.ID)
	y, _ := db.CreateFolder(ctx, anna, "Y", x.ID)
	deep, _ := db.CreatePage(ctx, anna, "Deep", y.ID)
	if ok, _ := db.May(ctx, deep.ID, bob.ID, ActRead); !ok {
		t.Fatal("bob reads Deep through the tenant")
	}
	// A change above under way: a deny of read for bob on X, written and not committed yet.
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := lockRights(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := setEntries(ctx, tx, x.ID, ToPerson, bob.ID, []string{ActRead}, map[string]string{ActRead: Deny}, true, anna.ID); err != nil {
		t.Fatal(err)
	}
	// Breaking the inheritance of Deep meanwhile waits for it.
	done := make(chan error, 1)
	go func() { done <- db.SetInherit(asPerson(anna), deep.ID, Actor{ID: anna.ID}, false) }()
	select {
	case err := <-done:
		t.Fatalf("breaking went on while a change above was under way: %v", err)
	case <-time.After(400 * time.Millisecond):
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if ok, _ := db.May(ctx, deep.ID, bob.ID, ActRead); ok {
		t.Error("the deny given above while Deep broke its inheritance was lost: bob reads Deep")
	}
	d, _ := db.Decide(ctx, deep.ID, bob.ID, ActRead, false)
	if d.Object != deep.ID {
		t.Errorf("the deny is not copied onto Deep: decided on %s", d.Object)
	}

	// Carl manages X; while a change above takes that away, his change on Y waits for it and is then refused.
	if err := db.SetGrant(ctx, x.ID, Actor{ID: anna.ID}, ToPerson, carl.ID, RoleManage); err != nil {
		t.Fatal(err)
	}
	tx, err = db.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := lockRights(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err := setEntries(ctx, tx, x.ID, ToPerson, carl.ID, nil, nil, true, anna.ID); err != nil {
		t.Fatal(err)
	}
	go func() {
		done <- db.SetEntries(asPerson(carl), y.ID, Actor{ID: carl.ID}, ToPerson, bob.ID, map[string]string{ActRead: Allow}, true)
	}()
	select {
	case err := <-done:
		t.Fatalf("a change deep down went on while a change above was under way: %v", err)
	case <-time.After(400 * time.Millisecond):
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, ErrNotAllowed) && !errors.Is(err, ErrNotFound) {
		t.Errorf("carl changed Y after he stopped managing it: %v", err)
	}
	if ok, _ := db.May(ctx, y.ID, bob.ID, ActRead); ok {
		t.Error("bob reads Y through carl's entry")
	}
}
