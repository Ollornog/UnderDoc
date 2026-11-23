// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/markdown"
	"github.com/Ollornog/UnderDoc/internal/pagekind"
)

// Writing a page, or a section of it, as a whole against the version it was read at (ADR-34, T-21): the way agents
// write. The blocks are compared with what stands there, by the IDs of their markers: a block with the ID of one in the
// part written keeps it and gets its new text as a version, one without, or with an ID from elsewhere, is made new, and
// a block of the part that is left out is deleted. Every change goes through the same steps as a write in the editor
// (versions, references, the hints of AI changes, the ways of what is in use), in one transaction.

// ErrChanged is a write against a version the page has left: someone wrote it since it was read.
type ErrChanged struct{ Base, Now int64 }

func (e *ErrChanged) Error() string {
	return fmt.Sprintf("page changed since you read it: you read version %d, it is at %d now; read it again and write your change into that", e.Base, e.Now)
}

// errDryRun ends the transaction of a dry run, so nothing of it is kept.
var errDryRun = errors.New("dry run")

// BlockChange is what a write did, or would do, to one block.
type BlockChange struct {
	// Block is the ID of the block; a block made anew in a dry run has the ID it would get.
	Block string
	// Kind is added, changed, deleted or kept.
	Kind string
	// Before and After are its text before and after.
	Before, After string
}

// Changes of a block in a write.
const (
	ChangeAdded   = "added"
	ChangeChanged = "changed"
	ChangeDeleted = "deleted"
	ChangeKept    = "kept"
)

// Written is the result of a write: what changed, the version the page is at after it, its blocks, and what the check
// finds.
type Written struct {
	Changes  []BlockChange
	Version  int64
	Blocks   []Block
	Findings []pagekind.Finding
	DryRun   bool
}

// WriteBlocks replaces the blocks of a page, or of the section that opens at block from (its block alone if it opens no
// section), by blocks, against the version base the writer read (PageVersion). The writer must see the page and may
// change it. A page changed since gives *ErrChanged; a block taken out that something embeds gives *InUse, as in the
// editor. With dryRun the whole write runs and is undone: the answer says what would change and what the check would
// find, and nothing is kept.
func (db *DB) WriteBlocks(ctx context.Context, pageID, viewer string, base int64, from string, blocks []Block, dryRun bool) (Written, error) {
	for _, b := range blocks {
		if err := checkBody(b.Body); err != nil {
			return Written{}, err
		}
		if !ValidAlign(b.Align) {
			return Written{}, ErrAlign
		}
		if err := refuse(b.ID, b.Body); err != nil {
			return Written{}, err
		}
	}
	o, err := db.page(ctx, pageID, viewer)
	if err != nil {
		return Written{}, err
	}
	if err := db.mayDo(ctx, o, viewer, ActWrite); err != nil {
		return Written{}, err
	}
	var out Written
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 FOR UPDATE", pageID); err != nil {
			return err
		}
		var now int64
		if err := tx.QueryRow(ctx, "SELECT coalesce(max(version), 0) FROM block_versions WHERE page = $1", pageID).Scan(&now); err != nil {
			return err
		}
		if now != base {
			return &ErrChanged{Base: base, Now: now}
		}
		rows, err := tx.Query(ctx, "SELECT id, body, coalesce(align, '') FROM blocks WHERE page = $1 ORDER BY position, id", pageID)
		if err != nil {
			return err
		}
		current, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Block, error) {
			var b Block
			err := row.Scan(&b.ID, &b.Body, &b.Align)
			return b, err
		})
		if err != nil {
			return err
		}
		// The part written: the whole page, or the section at from.
		start, end := 0, len(current)
		if from != "" {
			start = slices.IndexFunc(current, func(b Block) bool { return b.ID == from })
			if start < 0 {
				return ErrNotFound
			}
			bodies := make([]string, 0, len(current)-start)
			for _, b := range current[start:] {
				bodies = append(bodies, b.Body)
			}
			end = start + max(markdown.SectionEnd(bodies), 1)
		}
		inPart := map[string]Block{}
		for _, b := range current[start:end] {
			inPart[b.ID] = b
		}
		var order []string
		kept := map[string]bool{}
		for _, b := range blocks {
			old, ok := inPart[b.ID]
			if ok && !kept[b.ID] {
				kept[b.ID] = true
				order = append(order, b.ID)
				switch {
				case old.Body != b.Body:
					if err := db.editBlock(ctx, tx, pageID, b.ID, viewer, b.Body, nil); err != nil {
						return err
					}
					out.Changes = append(out.Changes, BlockChange{Block: b.ID, Kind: ChangeChanged, Before: old.Body, After: b.Body})
				default:
					out.Changes = append(out.Changes, BlockChange{Block: b.ID, Kind: ChangeKept, Before: old.Body, After: b.Body})
				}
				if old.Align != b.Align {
					if _, err := tx.Exec(ctx, "UPDATE blocks SET align = nullif($3, '') WHERE page = $1 AND id = $2", pageID, b.ID, b.Align); err != nil {
						return err
					}
				}
				continue
			}
			made, err := db.addBlockTx(ctx, tx, pageID, viewer, b.Body, b.Align)
			if err != nil {
				return err
			}
			order = append(order, made)
			out.Changes = append(out.Changes, BlockChange{Block: made, Kind: ChangeAdded, After: b.Body})
		}
		for _, b := range current[start:end] {
			if kept[b.ID] {
				continue
			}
			if err := db.deleteBlock(ctx, tx, pageID, b.ID, viewer, nil); err != nil {
				return err
			}
			out.Changes = append(out.Changes, BlockChange{Block: b.ID, Kind: ChangeDeleted, Before: b.Body})
		}
		var ids []string
		for _, b := range current[:start] {
			ids = append(ids, b.ID)
		}
		ids = append(ids, order...)
		for _, b := range current[end:] {
			ids = append(ids, b.ID)
		}
		if _, err := tx.Exec(ctx, "UPDATE blocks b SET position = o.n - 1 FROM unnest($2::text[]) WITH ORDINALITY o(id, n) WHERE b.page = $1 AND b.id = o.id", pageID, ids); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE objects SET changed = now() WHERE id = $1", pageID); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, "SELECT coalesce(max(version), 0) FROM block_versions WHERE page = $1", pageID).Scan(&out.Version); err != nil {
			return err
		}
		rows, err = tx.Query(ctx, "SELECT id, body, coalesce(align, '') FROM blocks WHERE page = $1 ORDER BY position, id", pageID)
		if err != nil {
			return err
		}
		if out.Blocks, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Block, error) {
			var b Block
			err := row.Scan(&b.ID, &b.Body, &b.Align)
			return b, err
		}); err != nil {
			return err
		}
		if dryRun {
			return errDryRun
		}
		return nil
	})
	if dryRun && errors.Is(err, errDryRun) {
		err = nil
		out.DryRun = true
		out.Version = base
	}
	if err != nil {
		return Written{}, err
	}
	if o.Kind == KindPage {
		var values []pagekind.Value
		if o.PageKind == pagekind.HowTo || o.PageKind == pagekind.Explanation {
			if values, err = db.values(ctx, viewer); err != nil {
				return Written{}, err
			}
		}
		out.Findings = db.findingsOf(ctx, o, out.Blocks, values, viewer, map[string]*embedTarget{})
	}
	return out, nil
}

// addBlockTx makes a block at the end of a page inside the transaction of a change, with the next free ID; the caller
// puts it in its place. It is AddBlock's change without the check of rights, which the caller made.
func (db *DB) addBlockTx(ctx context.Context, tx pgx.Tx, pageID, viewer, body, align string) (string, error) {
	if err := roomForBytes(ctx, tx, int64(len(body))); err != nil {
		return "", err
	}
	var next int
	if err := tx.QueryRow(ctx, "UPDATE objects SET next_block = next_block + 1 WHERE id = $1 RETURNING next_block - 1", pageID).Scan(&next); err != nil {
		return "", err
	}
	id := "b" + strconv.Itoa(next)
	if _, err := tx.Exec(ctx, `INSERT INTO blocks (page, id, position, body, align)
		VALUES ($1, $2, (SELECT coalesce(max(position), -1) + 1 FROM blocks WHERE page = $1), $3, nullif($4, ''))`, pageID, id, body, align); err != nil {
		return "", err
	}
	if err := syncRefs(ctx, tx, pageID, id, body); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO block_versions (page, block, body, author) VALUES ($1, $2, $3, $4)", pageID, id, body, viewer); err != nil {
		return "", err
	}
	return id, noteChange(ctx, tx, pageID, id, [2]string{})
}
