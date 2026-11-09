// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Deleting puts an object into the trash with everything below it, so a slip of the hand costs nothing (2026-11-09).
// Restore puts it back where it lay; Purge takes it away for good.

// ErrParentDeleted is a restore of something whose place is in the trash itself: that has to come back first.
var ErrParentDeleted = fmt.Errorf("%w: what it lay in is in the trash, restore that first", ErrPlace)

// Trashed is an object in the trash, as its list shows it.
type Trashed struct {
	Object
	DeletedAt time.Time
	// DeletedBy is the name of whoever deleted it.
	DeletedBy string
	// Place names the collections and folders it lay in, from the top.
	Place string
	// PlaceDeleted tells that what it lay in is in the trash too, so it cannot be restored before that.
	PlaceDeleted bool
}

// Delete puts an object and everything below it into the trash, marked with the same moment. Whoever may change the
// object may delete it (ADR-20), a collection at the top only its owner, since it holds what others made. Private notes
// are never deleted, also not with something they were moved into. A line or a space has nothing to restore, so it goes
// for good, as DeleteDivider does.
func (db *DB) Delete(ctx context.Context, objectID, viewer string) error {
	o, err := db.Object(ctx, objectID, viewer)
	if err != nil {
		return err
	}
	if IsDivider(o.Kind) {
		return db.DeleteDivider(ctx, objectID, viewer)
	}
	if err := db.mayEdit(ctx, o, viewer); err != nil {
		return err
	}
	if o.Parent == "" && o.Owner != viewer {
		return ErrNotAllowed
	}
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		var deleted bool
		if err := tx.QueryRow(ctx, "SELECT deleted_at IS NOT NULL FROM objects WHERE id = $1 FOR UPDATE", objectID).Scan(&deleted); err != nil {
			return err
		}
		if deleted {
			return ErrNotFound
		}
		var private bool
		if err := tx.QueryRow(ctx, `WITH RECURSIVE down AS (
				SELECT id, private FROM objects WHERE id = $1
				UNION ALL SELECT c.id, c.private FROM objects c JOIN down ON c.parent = down.id WHERE c.deleted_at IS NULL
			) SELECT coalesce(bool_or(private), false) FROM down`, objectID).Scan(&private); err != nil {
			return err
		}
		if private {
			return ErrNotAllowed
		}
		_, err := tx.Exec(ctx, `WITH RECURSIVE down AS (
				SELECT id FROM objects WHERE id = $1
				UNION ALL SELECT c.id FROM objects c JOIN down ON c.parent = down.id WHERE c.deleted_at IS NULL
			) UPDATE objects SET deleted_at = now(), deleted_by = $2 WHERE id IN (SELECT id FROM down)`, objectID, viewer)
		return err
	})
}

// trashed is a condition on o: it lies in the trash and the viewer deleted it or owns it, and it is the top of what was
// deleted at that moment, not something that went with what lay above it.
const trashed = `o.deleted_at IS NOT NULL AND (o.owner = $1 OR o.deleted_by = $1)`

const trashColumns = columns + `, o.deleted_at, coalesce((SELECT a.name FROM accounts a WHERE a.id = o.deleted_by), o.deleted_by),
	coalesce((WITH RECURSIVE up AS (
		SELECT parent AS id, 1 AS depth FROM objects WHERE id = o.id
		UNION ALL SELECT p.parent, up.depth + 1 FROM objects p JOIN up ON p.id = up.id WHERE up.depth < 50
	) SELECT string_agg(p.title, ' › ' ORDER BY up.depth DESC) FROM up JOIN objects p ON p.id = up.id), ''),
	coalesce((SELECT p.deleted_at IS NOT NULL FROM objects p WHERE p.id = o.parent), false)`

func scanTrashed(row pgx.Row) (Trashed, error) {
	var t Trashed
	o := &t.Object
	err := row.Scan(&o.ID, &o.Kind, &o.Title, &o.Description, &o.Owner, &o.Parent, &o.Position, &o.Access, &o.Private, &o.Created, &o.Changed,
		&t.DeletedAt, &t.DeletedBy, &t.Place, &t.PlaceDeleted)
	return t, err
}

// Trash lists what the viewer deleted or owns and was deleted, newest first: each deletion once, by the object it began
// with, and where that lay.
func (db *DB) Trash(ctx context.Context, viewer string) ([]Trashed, error) {
	rows, err := db.pool.Query(ctx, `SELECT `+trashColumns+` FROM objects o
		WHERE `+trashed+` AND `+visible+`
			AND NOT EXISTS (SELECT 1 FROM objects p WHERE p.id = o.parent AND p.deleted_at = o.deleted_at)
		ORDER BY o.deleted_at DESC, o.position`, viewer)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Trashed, error) { return scanTrashed(row) })
}

// inTrash returns an object of the trash that the viewer may restore or purge: one they deleted or own.
func (db *DB) inTrash(ctx context.Context, objectID, viewer string) (Trashed, error) {
	t, err := scanTrashed(db.pool.QueryRow(ctx, `SELECT `+trashColumns+` FROM objects o
		WHERE o.id = $2 AND `+trashed+` AND `+visible, viewer, objectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Trashed{}, ErrNotFound
	}
	return t, err
}

// Restore puts an object back where it lay, at the end, with everything that was deleted with it; what was deleted on
// its own before stays in the trash. Its place must be there and open for the viewer to change (ADR-20): if that is in
// the trash itself, it has to be restored first.
func (db *DB) Restore(ctx context.Context, objectID, viewer string) (Object, error) {
	t, err := db.inTrash(ctx, objectID, viewer)
	if err != nil {
		return Object{}, err
	}
	if t.Parent != "" {
		if t.PlaceDeleted {
			return Object{}, ErrParentDeleted
		}
		p, err := db.Object(ctx, t.Parent, viewer)
		if err != nil {
			return Object{}, err
		}
		if err := db.mayEdit(ctx, p, viewer); err != nil {
			return Object{}, err
		}
	}
	var back Object
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if t.Parent != "" {
			// The place is locked, so it cannot go into the trash while this comes back into it.
			var gone bool
			if err := tx.QueryRow(ctx, "SELECT deleted_at IS NOT NULL FROM objects WHERE id = $1 FOR UPDATE", t.Parent).Scan(&gone); err != nil {
				return err
			}
			if gone {
				return ErrParentDeleted
			}
		}
		tag, err := tx.Exec(ctx, `WITH RECURSIVE down AS (
				SELECT id FROM objects WHERE id = $1 AND deleted_at = $2
				UNION ALL SELECT c.id FROM objects c JOIN down ON c.parent = down.id WHERE c.deleted_at = $2
			) UPDATE objects SET deleted_at = NULL, deleted_by = NULL WHERE id IN (SELECT id FROM down)`, objectID, t.DeletedAt)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		back, err = scan(tx.QueryRow(ctx, `UPDATE objects SET position = (SELECT coalesce(max(position) + 1, 0) FROM objects
				WHERE parent IS NOT DISTINCT FROM nullif($2, '') AND deleted_at IS NULL AND id <> $1)
			WHERE id = $1 RETURNING `+columns, objectID, t.Parent))
		return err
	})
	return back, err
}

// Purge takes an object of the trash away for good, with everything below it, its blocks and their versions. Whoever
// owns it or deleted it may.
func (db *DB) Purge(ctx context.Context, objectID, viewer string) error {
	if _, err := db.inTrash(ctx, objectID, viewer); err != nil {
		return err
	}
	// One statement takes the whole tree, so the parent of each is gone in the same moment as it is.
	tag, err := db.pool.Exec(ctx, `WITH RECURSIVE down AS (
			SELECT id FROM objects WHERE id = $1 AND deleted_at IS NOT NULL
			UNION ALL SELECT c.id FROM objects c JOIN down ON c.parent = down.id
		) DELETE FROM objects WHERE id IN (SELECT id FROM down)`, objectID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
