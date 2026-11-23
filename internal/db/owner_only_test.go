// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/search"
)

// TestANewCollectionIsItsOwnersAlone: whoever makes a collection at the top owns it and is the only one who holds a role
// on it, until they share it (ADR-29, 2026-11-19). It is not private: it is shared like any other, and administrators
// manage its rights without seeing its content. The same holds for a collection of the view Files and for a copy made at
// the top.
func TestANewCollectionIsItsOwnersAlone(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.SeeAccount(ctx, carl); err != nil {
		t.Fatal(err)
	}
	board, err := db.CreateCollection(ctx, anna, "Board", "")
	if err != nil || board.Access != "" || board.Private || board.Breaks {
		t.Fatalf("a new collection grants nobody anything and is not private: %+v %v", board, err)
	}
	page, err := db.CreatePage(ctx, anna, "Salaries", board.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, viewer := range []string{bob.ID, carl.ID} {
		for _, o := range []Object{board, page} {
			if got := role(t, db, o.ID, viewer); got != RoleNone {
				t.Errorf("%s holds %q on %s", viewer, got, o.Title)
			}
		}
		if got, _ := db.Collections(ctx, viewer); slices.Contains(titles(got), "Board") {
			t.Errorf("%s sees Board among the collections: %q", viewer, titles(got))
		}
	}
	if got := role(t, db, board.ID, anna.ID); got != RoleOwner {
		t.Errorf("anna owns Board: %q", got)
	}
	// An administrator manages its rights and transfers it, but sees nothing of its content.
	admin := Actor{ID: carl.ID, Admin: true}
	rights, err := db.RightsOf(ctx, board.ID, admin)
	if err != nil || rights.Owner != anna.ID || rights.Private || len(rights.Own) != 0 {
		t.Errorf("an administrator reads the rights, and nothing is granted: %+v %v", rights, err)
	}
	if _, err := db.Object(ctx, page.ID, carl.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("an administrator does not open the content: %v", err)
	}
	// Shared step by step: a person, then the tenant.
	if err := db.SetGrant(ctx, board.ID, Actor{ID: anna.ID}, ToPerson, bob.ID, RoleRead); err != nil {
		t.Fatalf("a collection of one's own is shared like any other: %v", err)
	}
	if got := role(t, db, page.ID, bob.ID); got != RoleRead {
		t.Errorf("bob reads the page now: %q", got)
	}
	if got := role(t, db, page.ID, carl.ID); got != RoleNone {
		t.Errorf("carl still holds nothing: %q", got)
	}
	if err := db.SetGrant(ctx, board.ID, Actor{ID: anna.ID}, ToTenant, "", RoleComment); err != nil {
		t.Fatal(err)
	}
	if got := role(t, db, page.ID, carl.ID); got != RoleComment {
		t.Errorf("the tenant comments now: %q", got)
	}
	// Inside a collection, a new one inherits what holds there.
	inner, err := db.CreateCollection(ctx, anna, "Minutes", board.ID)
	if err != nil || inner.Access != "" {
		t.Fatalf("%+v %v", inner, err)
	}
	if got := role(t, db, inner.ID, carl.ID); got != RoleComment {
		t.Errorf("a collection inside inherits: %q", got)
	}
	// A collection of the view Files.
	inbox, err := db.CreateFileCollection(ctx, anna, "Inbox")
	if err != nil || inbox.Access != "" {
		t.Fatalf("%+v %v", inbox, err)
	}
	if got := role(t, db, inbox.ID, bob.ID); got != RoleNone {
		t.Errorf("a collection of the files is its owner's alone too: %q", got)
	}
	// A copy at the top is a new collection of whoever copies, theirs alone, whatever its original granted.
	team, err := db.createShared(ctx, anna, "Team")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreatePage(ctx, anna, "Plan", team.ID); err != nil {
		t.Fatal(err)
	}
	copied, err := db.Copy(ctx, team.ID, bob.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if copied.Access != "" || copied.Owner != bob.ID {
		t.Errorf("the copy at the top grants the tenant nothing: %+v", copied)
	}
	if got := role(t, db, copied.ID, carl.ID); got != RoleNone {
		t.Errorf("carl holds nothing on bob's copy: %q", got)
	}
}

// TestCollectionsMadeBeforeKeepTheirGrants: a collection at the top made before 2026-11-19 granted the tenant edit, and
// keeps it: no migration takes a right away that was given (ADR-29).
func TestCollectionsMadeBeforeKeepTheirGrants(t *testing.T) {
	ctx := context.Background()
	url := pgtest.URL(t)
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, conn, 25); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `SELECT set_config('underdoc.tenant', 'main', false);
		INSERT INTO accounts (id, login, name, groups) VALUES ('u-anna', 'anna', 'Anna', '{}'), ('u-bob', 'bob', 'Bob', '{}');
		INSERT INTO objects (id, kind, title, owner, parent, private, area) VALUES
			('ccccccccc1', 'collection', 'Team', 'u-anna', NULL, false, 'docs'),
			('ccccccccc2', 'page', 'Plan', 'u-anna', 'ccccccccc1', false, 'docs'),
			('ccccccccc3', 'collection', 'Projects', 'u-anna', NULL, false, 'files');
		INSERT INTO file_roots (object, role) VALUES ('ccccccccc3', 'collection');
		INSERT INTO grants (object, kind, who, role, granted_by) VALUES
			('ccccccccc1', 'tenant', '', 'edit', 'u-anna'), ('ccccccccc3', 'tenant', '', 'edit', 'u-anna')`); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close(ctx)
	db, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, objectID := range []string{"ccccccccc1", "ccccccccc2", "ccccccccc3"} {
		if got := role(t, db, objectID, "u-bob"); got != RoleEdit {
			t.Errorf("bob edits %s as before: %q", objectID, got)
		}
	}
}

// TestNothingReachesANewCollectionBeforeItIsShared attacks the seams: nobody but its owner gets at a new collection,
// or what lies in it, by moving, copying, importing, a share link, the search or the access check, before it is
// shared; an administrator neither.
func TestNothingReachesANewCollectionBeforeItIsShared(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.SeeAccount(ctx, carl); err != nil {
		t.Fatal(err)
	}
	board, _ := db.CreateCollection(ctx, anna, "Board", "")
	salaries, _ := db.CreatePage(ctx, anna, "Salaries", board.ID)
	if _, err := db.AddBlock(ctx, salaries.ID, anna.ID, "", "Nine thousand a month."); err != nil {
		t.Fatal(err)
	}
	mine, _ := db.CreateCollection(ctx, bob, "Bob's", "")
	note, _ := db.CreatePage(ctx, bob, "Note", mine.ID)
	admin := Actor{ID: carl.ID, Admin: true}

	// Moving: in, out, or the collection itself.
	if _, err := db.Move(ctx, note.ID, bob.ID, board.ID, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob moves his page into Board: %v", err)
	}
	if _, err := db.Move(ctx, salaries.ID, bob.ID, mine.ID, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob moves the page out of Board: %v", err)
	}
	if _, err := db.Move(ctx, board.ID, bob.ID, mine.ID, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob moves Board into his own: %v", err)
	}
	if _, err := db.Move(ctx, salaries.ID, carl.ID, board.ID, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("an administrator moves the page: %v", err)
	}
	// Copying: the collection, the page, or into it.
	if _, err := db.Copy(ctx, board.ID, bob.ID, "", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob copies Board: %v", err)
	}
	if _, err := db.Copy(ctx, salaries.ID, bob.ID, mine.ID, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob copies the page into his own: %v", err)
	}
	if _, err := db.Copy(ctx, note.ID, bob.ID, board.ID, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob copies into Board: %v", err)
	}
	if _, err := db.Copy(ctx, salaries.ID, carl.ID, "", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("an administrator copies the page: %v", err)
	}
	// Importing under the IDs of Board and its page: they are taken, so the import gets new ones and touches nothing.
	tree := &Node{Object: Object{ID: board.ID, Kind: KindCollection, Title: "x"},
		Kids: []*Node{{Object: Object{ID: salaries.ID, Kind: KindPage, Title: "Imported"}, Blocks: []Block{{ID: "b1", Body: "Mine now."}}}}}
	done, err := db.Import(ctx, "bob", "Taken", tree)
	if err != nil {
		t.Fatal(err)
	}
	if done.Top.ID == board.ID || done.Renamed[board.ID] == "" || done.Renamed[salaries.ID] == "" {
		t.Errorf("the import took the IDs of Board and its page: %+v", done)
	}
	if got := role(t, db, salaries.ID, bob.ID); got != RoleNone {
		t.Errorf("the import gave bob a role on the page: %q", got)
	}
	if o, err := db.Object(ctx, salaries.ID, anna.ID); err != nil || o.Parent != board.ID || o.Owner != anna.ID {
		t.Errorf("the import touched the page: %+v %v", o, err)
	}
	// Share links: only a manager makes one, and an administrator who does not manage makes none.
	for name, actor := range map[string]Actor{"bob": {ID: bob.ID}, "an administrator": admin} {
		for _, o := range []Object{board, salaries} {
			if _, _, err := db.CreateLink(ctx, o.ID, actor, NewLink{Mode: LinkEdit}); err == nil {
				t.Errorf("%s makes a link to %s", name, o.Title)
			}
		}
	}
	if links, err := db.Links(ctx, Actor{ID: bob.ID}, ""); err != nil || len(links) != 0 {
		t.Errorf("bob sees links: %+v %v", links, err)
	}
	// The search: neither the title nor the record of the page reaches anyone but anna.
	for _, viewer := range []string{bob.ID, carl.ID} {
		if hits, err := db.SearchTitles(ctx, "Salaries", viewer, nil, 10); err != nil || len(hits) != 0 {
			t.Errorf("%s finds the page by its title: %+v %v", viewer, hits, err)
		}
	}
	if hits, _ := db.SearchTitles(ctx, "Salaries", anna.ID, nil, 10); len(hits) != 1 {
		t.Errorf("anna finds her page: %+v", hits)
	}
	docs, err := db.SearchSource().Docs(ctx, salaries.ID)
	if err != nil || len(docs) != 1 || !slices.Equal(docs[0].Readers, []string{search.ReaderAccount(anna.ID)}) {
		t.Errorf("only anna reads the record of the page: %+v %v", docs, err)
	}
	// The access check: bob learns nothing of it for himself.
	if got, reasons, err := db.Explain(ctx, salaries.ID, Actor{ID: bob.ID}, bob.ID, false); err != nil || got != RoleNone || len(reasons) != 0 {
		t.Errorf("bob's access check: %q %+v %v", got, reasons, err)
	}
	if held, err := db.HeldBy(ctx, Actor{ID: bob.ID}, bob.ID); err != nil || slices.ContainsFunc(held, func(h Held) bool { return h.Object == board.ID }) {
		t.Errorf("Board among what bob holds: %+v %v", held, err)
	}
}
