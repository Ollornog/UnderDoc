// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/sheet"
)

// Tables of spreadsheet files (ADR-31): what something uses of a spreadsheet gets an ID local to its file, x1, x2, …,
// kept here with what it points at. The file itself is never changed for it. Owner and rights are the file's: whoever
// reads the file reads and chooses its tables.

// Kinds of a table of a file.
const (
	TableSheet = "sheet"
	TableRange = "range"
	TableName  = "name"
	TableTable = "table"
)

// ErrTable is a table chosen with something missing or wrong.
var ErrTable = errors.New("a table is a sheet, a range of a sheet, a defined name or an Excel table of the file")

// MaxFileTables is the most tables of one file that get an ID. Every reader of a file chooses its tables, so without a
// limit a reader could fill the database.
const MaxFileTables = 10_000

// errTooManyTables is a table chosen past MaxFileTables.
var errTooManyTables = fmt.Errorf("%w: a file has at most %d tables with an ID", ErrTable, MaxFileTables)

// FileTable is a table of a spreadsheet file.
type FileTable struct {
	File, ID, Kind string
	Sheet          string
	SheetNumber    int
	// Range is the range of a range; for the other kinds where it was last found.
	Range, Name string
	Head        bool
	Anchor      []string
	// FoundBlob is the blob the table was last looked for in, FoundApp the program that saved it.
	FoundBlob, FoundApp string
	// MovedFrom is where a followed range or sheet was before; Changed a range whose first row changed and could not be
	// followed; Lost why it is no longer in the file.
	MovedFrom string
	Changed   bool
	Lost      string
	CreatedBy string
	Created   time.Time
}

// Ref is the address of the table, <file>#x3.
func (t FileTable) Ref() string { return t.File + "#" + t.ID }

// Label names the table as a person reads it: the name of a name or table, the sheet, or the sheet and range.
func (t FileTable) Label() string {
	switch t.Kind {
	case TableName, TableTable:
		return t.Name
	case TableRange:
		return t.Sheet + "!" + t.Range
	}
	return t.Sheet
}

const fileTableColumns = `file, id, kind, sheet, sheet_number, range, name, head, anchor, found_blob, found_app, moved_from, changed, lost, created_by, created`

func scanFileTable(row pgx.Row) (FileTable, error) {
	var t FileTable
	err := row.Scan(&t.File, &t.ID, &t.Kind, &t.Sheet, &t.SheetNumber, &t.Range, &t.Name, &t.Head, &t.Anchor, &t.FoundBlob,
		&t.FoundApp, &t.MovedFrom, &t.Changed, &t.Lost, &t.CreatedBy, &t.Created)
	return t, err
}

func validTable(t FileTable) error {
	switch {
	case t.Sheet == "" || len(t.Sheet) > 1024 || len(t.Name) > 1024 || len(t.Range) > 40:
		return ErrTable
	case len(t.Anchor) > sheet.MaxGridCols || slices.ContainsFunc(t.Anchor, func(a string) bool { return len(a) > 4*sheet.MaxAnchorText }):
		// An anchor is never wider or longer than sheet.Anchor makes one.
		return ErrTable
	case t.Kind == TableSheet && t.Range == "" && t.Name == "":
	case t.Kind == TableRange && t.Range != "" && t.Name == "":
	case (t.Kind == TableName || t.Kind == TableTable) && t.Name != "":
	default:
		return ErrTable
	}
	return nil
}

// ChooseTable gives a table of a file the viewer reads its ID: the one it has, or the next one. The caller has checked
// it against the current version, found it there and set where (FoundBlob, FoundApp, Range, Anchor).
func (db *DB) ChooseTable(ctx context.Context, fileID, viewer string, t FileTable) (FileTable, bool, error) {
	if err := validTable(t); err != nil {
		return FileTable{}, false, err
	}
	if _, err := db.File(ctx, fileID, viewer); err != nil {
		return FileTable{}, false, err
	}
	var out FileTable
	made := false
	err := pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		// One chooser at a time per file, so two never get the same ID or the same table twice.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('file_tables'), hashtext($1))", fileID); err != nil {
			return err
		}
		var err error
		out, err = scanFileTable(tx.QueryRow(ctx, `SELECT `+fileTableColumns+` FROM file_tables WHERE file = $1 AND kind = $2
			AND sheet = $3 AND (kind <> 'range' OR range = $4) AND name = $5 AND head = $6 AND lost = '' ORDER BY length(id), id LIMIT 1`,
			fileID, t.Kind, t.Sheet, t.Range, t.Name, t.Head))
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var next, count int
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(substr(id, 2)::int), 0) + 1, count(*) FROM file_tables WHERE file = $1`, fileID).Scan(&next, &count); err != nil {
			return err
		}
		if count >= MaxFileTables {
			return errTooManyTables
		}
		made = true
		out, err = scanFileTable(tx.QueryRow(ctx, `INSERT INTO file_tables (file, id, kind, sheet, sheet_number, range, name, head,
			anchor, found_blob, found_app, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING `+fileTableColumns,
			fileID, "x"+strconv.Itoa(next), t.Kind, t.Sheet, t.SheetNumber, t.Range, t.Name, t.Head, nonNil(t.Anchor), t.FoundBlob, t.FoundApp, viewer))
		return err
	})
	return out, made, err
}

// FileTables returns the tables of a file the viewer reads, in the order of their IDs.
func (db *DB) FileTables(ctx context.Context, fileID, viewer string) ([]FileTable, error) {
	if _, err := db.File(ctx, fileID, viewer); err != nil {
		return nil, err
	}
	rows, err := db.pool.Query(ctx, `SELECT `+fileTableColumns+` FROM file_tables WHERE file = $1 ORDER BY length(id), id`, fileID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (FileTable, error) { return scanFileTable(row) })
}

// FileTableOf returns one table of a file the viewer reads.
func (db *DB) FileTableOf(ctx context.Context, fileID, id, viewer string) (FileTable, error) {
	if _, err := db.File(ctx, fileID, viewer); err != nil {
		return FileTable{}, err
	}
	t, err := scanFileTable(db.pool.QueryRow(ctx, `SELECT `+fileTableColumns+` FROM file_tables WHERE file = $1 AND id = $2`, fileID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return FileTable{}, ErrNotFound
	}
	return t, err
}

// TableFound writes down what came of looking for a table in a version of its file: where it is now, whether it moved,
// changed or is lost. Only what UnderDoc found is written, never what a viewer says, so it needs no viewer; the table
// is written only while it was last looked for in the blob given as before, so two lookups never undo each other.
func (db *DB) TableFound(ctx context.Context, before string, t FileTable) error {
	_, err := db.pool.Exec(ctx, `UPDATE file_tables SET sheet = $3, sheet_number = $4, range = $5, anchor = $6, found_blob = $7,
		found_app = $8, moved_from = $9, changed = $10, lost = $11 WHERE file = $1 AND id = $2 AND found_blob = $12`,
		t.File, t.ID, t.Sheet, t.SheetNumber, t.Range, nonNil(t.Anchor), t.FoundBlob, t.FoundApp, t.MovedFrom, t.Changed, t.Lost, before)
	return err
}

// KeepTable keeps a range whose first row changed as it is now: its anchor becomes the first row it has. Whoever may
// edit the file does it, since the table is the file's and every page that embeds it follows.
func (db *DB) KeepTable(ctx context.Context, fileID, id, viewer string, anchor []string) error {
	f, err := db.File(ctx, fileID, viewer)
	if err != nil {
		return err
	}
	if !db.MayEdit(ctx, f.Object, viewer) {
		return ErrNotAllowed
	}
	tag, err := db.pool.Exec(ctx, `UPDATE file_tables SET anchor = $3, changed = false, moved_from = '' WHERE file = $1 AND id = $2 AND kind = 'range'`,
		fileID, id, nonNil(anchor))
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// TableUses counts, for each table of a file, the places that embed it in pages the viewer sees.
func (db *DB) TableUses(ctx context.Context, fileID, viewer string) (map[string]int, error) {
	rows, err := db.pool.Query(ctx, `SELECT r.target_block, count(*) FROM refs r JOIN objects o ON o.id = r.page
		WHERE r.target = $2 AND r.target_block LIKE 'x%' AND o.deleted_at IS NULL AND `+visible+` GROUP BY 1`, viewer, fileID)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// tablesForExport returns every table of the files of a tenant, by file, for the export of the files.
func (db *DB) tablesForExport(ctx context.Context) (map[string][]FileTable, error) {
	rows, err := db.pool.Query(ctx, `SELECT `+fileTableColumns+` FROM file_tables ORDER BY file, length(id), id`)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (FileTable, error) { return scanFileTable(row) })
	if err != nil {
		return nil, err
	}
	out := map[string][]FileTable{}
	for _, t := range list {
		out[t.File] = append(out[t.File], t)
	}
	return out, nil
}

// importTable puts a table of an export back, with its ID, inside the transaction of the import. It is looked for anew
// in the file's current version the first time it shows.
func importTable(ctx context.Context, tx pgx.Tx, fileID, owner string, t FileTable) error {
	t.Anchor = sheet.ClipAnchor(t.Anchor)
	if err := validTable(t); err != nil || !tableID.MatchString(t.ID) {
		return fmt.Errorf("table %s of file %s: %w", t.ID, fileID, ErrTable)
	}
	_, err := tx.Exec(ctx, `INSERT INTO file_tables (file, id, kind, sheet, sheet_number, range, name, head, anchor, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) ON CONFLICT DO NOTHING`,
		fileID, t.ID, t.Kind, t.Sheet, t.SheetNumber, t.Range, t.Name, t.Head, nonNil(t.Anchor), owner)
	return err
}

// IsTableID tells whether the target of an address is a table of a file, x3.
func IsTableID(target string) bool { return tableID.MatchString(target) }
