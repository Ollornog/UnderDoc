// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/markdown"
)

// Parts inside a paragraph (ADR-22). The markers are part of the block's Markdown, so a part comes and goes with the
// text of its block and its versions; marking one is a change of the block like any other.

// ErrPart is a marking that cannot be made: more than one paragraph, no text, or a name that does not fit.
var ErrPart = errors.New("this cannot be marked as a part")

// MarkPart marks the Markdown between start and end, byte offsets into the block, as a part with a new ID (ADR-22), and
// keeps the change as a version. A name makes it a value (M-3). The ID is new also against parts that are gone but still
// embedded somewhere, so an old embedding never shows something else.
func (db *DB) MarkPart(ctx context.Context, pageID, blockID, viewer string, start, end int, name string) (Block, markdown.Part, error) {
	var out Block
	var part markdown.Part
	err := db.change(ctx, pageID, viewer, func(tx pgx.Tx) error {
		var body string
		if err := tx.QueryRow(ctx, "SELECT body FROM blocks WHERE page = $1 AND id = $2", pageID, blockID).Scan(&body); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		at, err := nextPartFloor(ctx, tx, pageID, blockID)
		if err != nil {
			return err
		}
		marked, p, err := markdown.MarkPart(body, start, end, markdown.NextPartID(body, at), name)
		if err != nil {
			return errors.Join(ErrPart, err)
		}
		if err := checkBody(marked); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE blocks SET body = $3 WHERE page = $1 AND id = $2", pageID, blockID, marked); err != nil {
			return err
		}
		if err := syncRefs(ctx, tx, pageID, blockID, marked); err != nil {
			return err
		}
		out, part = Block{ID: blockID, Body: marked}, p
		_, err = tx.Exec(ctx, "INSERT INTO block_versions (page, block, body, author) VALUES ($1, $2, $3, $4)", pageID, blockID, marked, viewer)
		return err
	})
	return out, part, err
}

// nextPartFloor is the lowest number a new part of a block may have: one more than any part of it that something embeds.
func nextPartFloor(ctx context.Context, tx pgx.Tx, pageID, blockID string) (int, error) {
	rows, err := tx.Query(ctx, "SELECT target_block FROM refs WHERE target = $1 AND target_block LIKE $2", pageID, blockID+".t%")
	if err != nil {
		return 0, err
	}
	targets, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, err
	}
	floor := 1
	for _, t := range targets {
		if n, err := strconv.Atoi(strings.TrimPrefix(t[strings.IndexByte(t, '.')+1:], "t")); err == nil && n >= floor {
			floor = n + 1
		}
	}
	return floor, nil
}

// Use is one place where something is embedded, for the choices before it goes (ADR-22).
type Use struct {
	Page, Block, Title string
	// CanEdit tells whether the viewer may change that page, and so write a copy or a new master into it.
	CanEdit bool
}

// UsesOf returns where an address of a page is embedded, in pages that are not in the trash: the places the viewer sees,
// and how many more there are that the viewer may not see, without naming them. For a block, embeddings of its parts and
// of the section it opens count too; for a page, everything of it.
func (db *DB) UsesOf(ctx context.Context, a Address, viewer string) ([]Use, int, error) {
	query := `SELECT r.page, r.block, o.title, ` + visible + ` AS seen, o.owner, o.id FROM refs r JOIN objects o ON o.id = r.page
		WHERE r.target = $2 AND o.deleted_at IS NULL`
	args := []any{viewer, a.Object}
	switch {
	case a.Block == "":
	case a.Sub == "":
		query += ` AND (r.target_block = $3 OR r.target_block LIKE $3 || '.%')`
		args = append(args, a.Block)
	default:
		query += ` AND r.target_block = $3`
		args = append(args, a.Target())
	}
	rows, err := db.pool.Query(ctx, query+` ORDER BY o.title, r.page, r.block`, args...)
	if err != nil {
		return nil, 0, err
	}
	type row struct {
		page, block, title, owner, id string
		seen                          bool
	}
	found, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var x row
		err := r.Scan(&x.page, &x.block, &x.title, &x.seen, &x.owner, &x.id)
		return x, err
	})
	if err != nil {
		return nil, 0, err
	}
	uses := []Use{}
	hidden := 0
	seen := map[[2]string]bool{}
	for _, x := range found {
		if seen[[2]string{x.page, x.block}] {
			continue
		}
		seen[[2]string{x.page, x.block}] = true
		if !x.seen {
			hidden++
			continue
		}
		o, err := db.Object(ctx, x.page, viewer)
		uses = append(uses, Use{Page: x.page, Block: x.block, Title: x.title, CanEdit: err == nil && db.MayEdit(ctx, o, viewer)})
	}
	return uses, hidden, nil
}
