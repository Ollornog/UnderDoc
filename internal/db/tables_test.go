// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// TestTablesOfAFileAreBounded (attack round on ADR-31): whoever reads a file chooses its tables, so a reader must not be
// able to fill the database: a file keeps at most MaxFileTables, a table chosen again keeps its ID beyond the limit, and an
// anchor is at most as wide and as long as the reader makes one.
func TestTablesOfAFileAreBounded(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.EnsureRoots(ctx, anna, false, ""); err != nil {
		t.Fatal(err)
	}
	f := upload(t, db, anna.ID, UploadTarget{Parent: rootOfRole(t, db, anna.ID, RootUser).ID}, "plan.xlsx", 10, sha(41))
	first := FileTable{Kind: TableSheet, Sheet: "S1", FoundBlob: sha(41)}
	if _, fresh, err := db.ChooseTable(ctx, f.File.ID, anna.ID, first); err != nil || !fresh {
		t.Fatalf("first: %v", err)
	}
	if _, err := db.pool.Exec(ctx, `INSERT INTO file_tables (file, id, kind, sheet, created_by)
		SELECT $1, 'x' || n, 'sheet', 'S' || n, $2 FROM generate_series(2, $3) n`, f.File.ID, anna.ID, MaxFileTables); err != nil {
		t.Fatal(err)
	}
	another := FileTable{Kind: TableSheet, Sheet: "another", FoundBlob: sha(41)}
	if _, _, err := db.ChooseTable(ctx, f.File.ID, anna.ID, another); !errors.Is(err, ErrTable) || !strings.Contains(err.Error(), "at most") {
		t.Errorf("a table past the limit: %v", err)
	}
	if got, fresh, err := db.ChooseTable(ctx, f.File.ID, anna.ID, first); err != nil || fresh || got.ID != "x1" {
		t.Errorf("the first again: %+v %v %v", got, fresh, err)
	}
	// On a file of few tables, an anchor wider or longer than the reader makes one is refused.
	other := upload(t, db, anna.ID, UploadTarget{Parent: rootOfRole(t, db, anna.ID, RootUser).ID}, "other.xlsx", 10, sha(42))
	wide := FileTable{Kind: TableRange, Sheet: "S1", Range: "A1:B1", Anchor: make([]string, 257), FoundBlob: sha(42)}
	long := FileTable{Kind: TableRange, Sheet: "S1", Range: "A1:B1", Anchor: []string{strings.Repeat("y", 5000)}, FoundBlob: sha(42)}
	for name, rec := range map[string]FileTable{"wide": wide, "long": long} {
		if _, _, err := db.ChooseTable(ctx, other.File.ID, anna.ID, rec); !errors.Is(err, ErrTable) {
			t.Errorf("%s anchor: %v", name, err)
		}
	}
	ok := FileTable{Kind: TableRange, Sheet: "S1", Range: "A1:B1", Anchor: []string{strings.Repeat("ä", 256), "b"}, FoundBlob: sha(42)}
	if _, _, err := db.ChooseTable(ctx, other.File.ID, anna.ID, ok); err != nil {
		t.Errorf("an anchor of 256 characters: %v", err)
	}
	// An export made before anchors were short imports, its anchor cut as one is made now.
	err := pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		return importTable(ctx, tx, other.File.ID, anna.ID, FileTable{ID: "x7", Kind: TableRange, Sheet: "S1", Range: "A1:B1",
			Anchor: []string{strings.Repeat("z", 5000), "b"}})
	})
	if err != nil {
		t.Fatalf("an old export: %v", err)
	}
	if got, err := db.FileTableOf(ctx, other.File.ID, "x7", anna.ID); err != nil || len(got.Anchor) != 2 || len(got.Anchor[0]) != 256 {
		t.Errorf("imported: %v %d", err, len(got.Anchor))
	}
}
