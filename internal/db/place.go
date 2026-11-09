// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// Kinds that only set entries apart: a line or a space between them.
const (
	KindLine  = "line"
	KindSpace = "space"
)

// ErrPlace is a move that would put an object where it cannot be: into itself, below itself, or at the top without being
// a collection.
var ErrPlace = errors.New("it cannot go there")

// IsDivider tells whether a kind only sets entries apart.
func IsDivider(kind string) bool { return kind == KindLine || kind == KindSpace }

// CreateDivider puts a line or a space into parent, after the entry named by after or first. Whoever may change the
// parent may add one (ADR-20).
func (db *DB) CreateDivider(ctx context.Context, owner account.User, kind, parent, after string) (Object, error) {
	if !IsDivider(kind) {
		return Object{}, ErrPlace
	}
	p, err := db.Object(ctx, parent, owner.ID)
	if err != nil {
		return Object{}, err
	}
	if err := db.mayEdit(ctx, p, owner.ID); err != nil {
		return Object{}, err
	}
	var made Object
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		position, err := place(ctx, tx, parent, after)
		if err != nil {
			return err
		}
		made, err = insertObject(ctx, tx, Object{Kind: kind, Title: kind, Owner: owner.ID, Parent: parent})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "UPDATE objects SET position = $2 WHERE id = $1", made.ID, position)
		made.Position = position
		return err
	})
	return made, err
}

// DeleteDivider takes a line or a space away. Other objects are not deleted this way.
func (db *DB) DeleteDivider(ctx context.Context, objectID, viewer string) error {
	o, err := db.Object(ctx, objectID, viewer)
	if err != nil {
		return err
	}
	if !IsDivider(o.Kind) {
		return ErrNotFound
	}
	if err := db.mayEdit(ctx, o, viewer); err != nil {
		return err
	}
	_, err = db.pool.Exec(ctx, "DELETE FROM objects WHERE id = $1", objectID)
	return err
}

// place makes room in parent after the entry named by after, or first, and returns the position that is free now. The
// parent is locked, so two moves into it do not take the same place.
func place(ctx context.Context, tx pgx.Tx, parent, after string) (int, error) {
	if parent != "" {
		if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 FOR UPDATE", parent); err != nil {
			return 0, err
		}
	}
	position := 0
	if after != "" {
		err := tx.QueryRow(ctx, "SELECT position + 1 FROM objects WHERE id = $1 AND parent IS NOT DISTINCT FROM nullif($2, '') AND deleted_at IS NULL", after, parent).Scan(&position)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		if err != nil {
			return 0, err
		}
	}
	_, err := tx.Exec(ctx, "UPDATE objects SET position = position + 1 WHERE parent IS NOT DISTINCT FROM nullif($1, '') AND position >= $2", parent, position)
	return position, err
}

// Move puts an object into parent, after the entry named by after or first. The viewer must be allowed to change the
// object where it is and the parent it goes to (ADR-20). Only a collection stands at the top, nothing goes into itself
// or below itself, and only its owner takes an object into a private collection, where no one else would see it.
func (db *DB) Move(ctx context.Context, objectID, viewer, parent, after string) (Object, error) {
	o, err := db.Object(ctx, objectID, viewer)
	if err != nil {
		return Object{}, err
	}
	if err := db.mayEdit(ctx, o, viewer); err != nil {
		return Object{}, err
	}
	if parent == "" && o.Kind != KindCollection {
		return Object{}, ErrPlace
	}
	if parent != "" {
		p, err := db.Object(ctx, parent, viewer)
		if err != nil {
			return Object{}, err
		}
		if err := db.mayEdit(ctx, p, viewer); err != nil {
			return Object{}, err
		}
		if IsDivider(p.Kind) {
			return Object{}, ErrPlace
		}
		top := p
		for x := p; ; {
			if x.ID == objectID {
				return Object{}, ErrPlace
			}
			top = x
			if x.Parent == "" {
				break
			}
			if x, err = db.Object(ctx, x.Parent, viewer); err != nil {
				return Object{}, err
			}
		}
		if top.Private && o.Owner != viewer {
			return Object{}, ErrNotAllowed
		}
	}
	var moved Object
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if after == objectID {
			return ErrPlace
		}
		position, err := place(ctx, tx, parent, after)
		if err != nil {
			return err
		}
		moved, err = scan(tx.QueryRow(ctx, `UPDATE objects SET parent = nullif($2, ''), position = $3, changed = now() WHERE id = $1 RETURNING `+columns,
			objectID, parent, position))
		return err
	})
	return moved, err
}
