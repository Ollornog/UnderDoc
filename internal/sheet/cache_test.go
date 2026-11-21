// SPDX-License-Identifier: MIT

package sheet

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// dirBlobs is a content store in a folder: blobs by their name, derived files beside them.
type dirBlobs struct{ dir string }

func (b dirBlobs) Open(_, sha string) (*os.File, error) { return os.Open(filepath.Join(b.dir, sha)) }
func (b dirBlobs) DerivedPath(_, name string) (string, error) {
	return filepath.Join(b.dir, "derived-"+name), nil
}
func (b dirBlobs) WriteDerived(_, name string, content []byte) error {
	return os.WriteFile(filepath.Join(b.dir, "derived-"+name), content, 0o600)
}

// TestTheSheetOfACSVIsNamedAfterItsFile: two files with the same bytes share their blob. The one sheet of a CSV carries
// the name of the file it is read for, never the name of the other file, and the same bytes read as TSV are split at
// tabs, not taken from what was read as CSV.
func TestTheSheetOfACSVIsNamedAfterItsFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "blob"), []byte("a,b\tc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := NewCache(dirBlobs{dir}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()
	first, err := c.Workbook(ctx, "t", "blob", TypeCSV, "salaries of the board.csv")
	if err != nil || first.Sheets[0].Name != "salaries of the board" {
		t.Fatalf("first: %v %+v", err, first)
	}
	other, err := c.Workbook(ctx, "t", "blob", TypeCSV, "plain.csv")
	if err != nil || other.Sheets[0].Name != "plain" {
		t.Errorf("the other file of the same bytes: %v %+v", err, other.Sheets)
	}
	if first.Sheets[0].Name != "salaries of the board" {
		t.Errorf("the first file renamed: %+v", first.Sheets)
	}
	// From what was kept on disk, too.
	fresh := NewCache(dirBlobs{dir}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if again, err := fresh.Workbook(ctx, "t", "blob", TypeCSV, "plain.csv"); err != nil || again.Sheets[0].Name != "plain" {
		t.Errorf("from disk: %v %+v", err, again.Sheets)
	}
	tsv, err := c.Workbook(ctx, "t", "blob", TypeTSV, "plain.tsv")
	if err != nil || len(tsv.Sheets[0].Rows) != 1 || len(tsv.Sheets[0].Rows[0].Cells) != 2 || tsv.Sheets[0].Rows[0].Cells[0].Value != "a,b" {
		t.Errorf("as TSV: %v %+v", err, tsv.Sheets)
	}
}
