// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/id"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Main(m)) }

var (
	anna = account.User{ID: "u-anna", Login: "anna", Name: "Anna Berg", Email: "anna@example.com", Groups: []string{"ops"}}
	bob  = account.User{ID: "u-bob", Login: "bob", Name: "Bob"}
)

func open(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	for _, user := range []account.User{anna, bob} {
		if err := db.SeeAccount(context.Background(), user); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func titles(objects []Object) []string {
	var out []string
	for _, o := range objects {
		out = append(out, o.Title)
	}
	return out
}

func TestMigrationsRunOnce(t *testing.T) {
	url := pgtest.URL(t)
	ctx := context.Background()
	for range 2 {
		db, err := Open(ctx, url)
		if err != nil {
			t.Fatal(err)
		}
		var n int
		if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&n); err != nil || n != 1 {
			t.Fatalf("%d migrations recorded, %v", n, err)
		}
		if !db.Healthy(ctx) {
			t.Fatal("not healthy")
		}
		db.Close()
	}
}

func TestAccountsComeWithPrivateNotes(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	renamed := anna
	renamed.Name = "Anna Maria Berg"
	if err := db.SeeAccount(ctx, renamed); err != nil {
		t.Fatal(err)
	}
	var name string
	var count int
	if err := db.pool.QueryRow(ctx, "SELECT name, (SELECT count(*) FROM objects WHERE owner = 'u-anna') FROM accounts WHERE id = 'u-anna'").Scan(&name, &count); err != nil {
		t.Fatal(err)
	}
	if name != "Anna Maria Berg" || count != 1 {
		t.Fatalf("a second sign-in updates the account and makes no second collection: %q, %d", name, count)
	}
	mine, err := db.Collections(ctx, anna.ID)
	if err != nil || len(mine) != 1 || mine[0].Title != MyNotes || !mine[0].Private || mine[0].Owner != anna.ID {
		t.Fatalf("%+v %v", mine, err)
	}
}

func TestCollections(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	ops, err := db.CreateCollection(ctx, anna, "  Operations ", "")
	if err != nil || !id.Valid(ops.ID) || ops.Title != "Operations" || ops.Private || ops.Access != "" {
		t.Fatalf("%+v %v", ops, err)
	}
	if _, err := db.CreateCollection(ctx, bob, "Projects", ""); err != nil {
		t.Fatal(err)
	}
	inner, err := db.CreateCollection(ctx, bob, "Planning 2027", ops.ID)
	if err != nil || inner.Parent != ops.ID {
		t.Fatalf("%+v %v", inner, err)
	}
	for _, bad := range []string{"", "   ", string(make([]rune, 201))} {
		if _, err := db.CreateCollection(ctx, anna, bad, ""); !errors.Is(err, ErrTitle) {
			t.Errorf("title %q: %v", bad, err)
		}
	}
	// Private ones first, then the shared ones in the order they were made. Nobody sees another's notes.
	if got, _ := db.Collections(ctx, anna.ID); len(got) != 3 || got[0].Title != MyNotes || got[1].Title != "Operations" || got[2].Title != "Projects" {
		t.Errorf("anna sees %q", titles(got))
	}
	if got, _ := db.Collections(ctx, bob.ID); len(got) != 3 || got[0].Owner != bob.ID {
		t.Errorf("bob sees %q", titles(got))
	}
	if got, _ := db.Children(ctx, ops.ID, anna.ID); len(got) != 1 || got[0].Title != "Planning 2027" {
		t.Errorf("children %q", titles(got))
	}
}

// A private collection and everything inside it stay with their owner, also when someone asks for an ID directly.
func TestPrivateStaysPrivate(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	notes, _ := db.Collections(ctx, anna.ID)
	private := notes[0]
	inside, err := db.CreateCollection(ctx, anna, "Ideas", private.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, objectID := range []string{private.ID, inside.ID} {
		if _, err := db.Object(ctx, objectID, bob.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("bob reached %s: %v", objectID, err)
		}
		if _, err := db.Object(ctx, objectID, anna.ID); err != nil {
			t.Errorf("anna lost %s: %v", objectID, err)
		}
	}
	if got, _ := db.Children(ctx, private.ID, bob.ID); len(got) != 0 {
		t.Errorf("bob sees inside: %q", titles(got))
	}
	if _, err := db.CreateCollection(ctx, bob, "Sneaky", private.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob put something into anna's notes: %v", err)
	}
	if _, err := db.Object(ctx, "0000000000", anna.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a missing ID: %v", err)
	}
}

// The schema itself refuses what must never be stored, whatever the code above it does.
func TestSchemaRefuses(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	for name, sql := range map[string]string{
		"page without a place": `INSERT INTO objects (id, kind, title, owner) VALUES ('aaaaaaaaaa', 'page', 'x', 'u-anna')`,
		"unknown kind":         `INSERT INTO objects (id, kind, title, owner) VALUES ('aaaaaaaaab', 'table', 'x', 'u-anna')`,
		"bad ID":               `INSERT INTO objects (id, kind, title, owner) VALUES ('Network', 'collection', 'x', 'u-anna')`,
		"unknown access":       `INSERT INTO objects (id, kind, title, owner, access) VALUES ('aaaaaaaaac', 'collection', 'x', 'u-anna', 'admin')`,
		"unknown owner":        `INSERT INTO objects (id, kind, title, owner) VALUES ('aaaaaaaaad', 'collection', 'x', 'nobody')`,
	} {
		if _, err := db.pool.Exec(ctx, sql); err == nil {
			t.Errorf("%s was stored", name)
		}
	}
}

func TestAccessIsInherited(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	top, _ := db.CreateCollection(ctx, anna, "Archive", "")
	inner, _ := db.CreateCollection(ctx, anna, "2025", top.ID)
	for _, o := range []Object{top, inner} {
		if got, err := db.Access(ctx, o.ID); got != "edit" || err != nil {
			t.Errorf("%s: the default is edit, got %q %v", o.Title, got, err)
		}
	}
	if _, err := db.pool.Exec(ctx, "UPDATE objects SET access = 'read' WHERE id = $1", top.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.Access(ctx, inner.ID); got != "read" {
		t.Errorf("read only should reach what is inside: %q", got)
	}
	if got, _ := db.Top(ctx, inner.ID, bob.ID); got.ID != top.ID {
		t.Errorf("top of 2025: %+v", got)
	}
	if db.AccountName(ctx, anna.ID) != "Anna Berg" || db.AccountName(ctx, "nobody") != "nobody" {
		t.Error("account names")
	}
}
