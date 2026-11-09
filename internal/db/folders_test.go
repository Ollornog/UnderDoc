// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"testing"
)

func TestFoldersAndConverting(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	archive, _ := db.CreateCollection(ctx, anna, "Archive", "")
	folder, err := db.CreateFolder(ctx, anna, "Drawings", team.ID)
	if err != nil || folder.Kind != KindFolder || folder.Parent != team.ID {
		t.Fatalf("a folder in the team: %+v %v", folder, err)
	}
	if _, err := db.CreateFolder(ctx, anna, "Loose", ""); !errors.Is(err, ErrPlace) {
		t.Errorf("a folder at the top: %v", err)
	}
	// A folder may hold collections.
	inner, err := db.CreateCollection(ctx, anna, "Inner", folder.ID)
	if err != nil || inner.Parent != folder.ID {
		t.Errorf("a collection in a folder: %+v %v", inner, err)
	}
	if _, err := db.pool.Exec(ctx, "UPDATE objects SET access = 'read' WHERE id = $1", archive.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateFolder(ctx, bob, "Mine", archive.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob adds a folder where he may only read: %v", err)
	}
	made, err := db.Convert(ctx, folder.ID, anna.ID, KindCollection)
	if err != nil || made.Kind != KindCollection || made.ID != folder.ID {
		t.Fatalf("the folder turns into a collection, keeping its ID: %+v %v", made, err)
	}
	if back, err := db.Convert(ctx, folder.ID, anna.ID, KindFolder); err != nil || back.Kind != KindFolder {
		t.Errorf("and back: %+v %v", back, err)
	}
	if _, err := db.Convert(ctx, team.ID, anna.ID, KindFolder); !errors.Is(err, ErrPlace) {
		t.Errorf("a collection at the top stays one: %v", err)
	}
	notes, _ := db.Collections(ctx, anna.ID)
	if _, err := db.Convert(ctx, notes[0].ID, anna.ID, KindCollection); err != nil {
		t.Errorf("the private notes are a collection already: %v", err)
	}
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	if _, err := db.Convert(ctx, page.ID, anna.ID, KindFolder); !errors.Is(err, ErrPlace) {
		t.Errorf("a page is no folder: %v", err)
	}
	if _, err := db.Convert(ctx, folder.ID, anna.ID, KindPage); !errors.Is(err, ErrPlace) {
		t.Errorf("a folder into a page: %v", err)
	}
	if _, err := db.Convert(ctx, inner.ID, bob.ID, KindFolder); err != nil {
		t.Errorf("others may edit the team, so also turn its collections into folders: %v", err)
	}
	ro, _ := db.CreateCollection(ctx, anna, "Old", archive.ID)
	if _, err := db.Convert(ctx, ro.ID, bob.ID, KindFolder); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob converts where he may only read: %v", err)
	}
}
