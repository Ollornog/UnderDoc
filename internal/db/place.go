// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"

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

// ErrForeignBelow is a move into something private of an object with something of someone else below it: private
// stays private, so it would take that away from its owner (ADR-29).
var ErrForeignBelow = fmt.Errorf("%w: something in it belongs to someone else, and only what is all yours goes into something private", ErrNotAllowed)

// Move puts an object into parent, after the entry named by after or first. Within the parent it lies in, whoever may
// edit the object and the parent reorders it; to another place it goes only with the role manage on the object and edit
// on where it goes, since the owner of what is above manages it there (ADR-29, 2026-11-19). Only a collection stands at
// the top, private notes stay there, and nothing goes into itself or below itself. Only its owner takes an object into
// something private, and only when everything below it is theirs too; its grants and links end then, and the log keeps
// that.
func (db *DB) Move(ctx context.Context, objectID, viewer, parent, after string) (Object, error) {
	o, err := db.Object(ctx, objectID, viewer)
	if err != nil {
		return Object{}, err
	}
	need := RoleManage
	if parent == o.Parent {
		need = RoleEdit
	}
	if err := db.may(ctx, o, viewer, need); err != nil {
		return Object{}, err
	}
	if err := db.guardFile(ctx, o); err != nil {
		return Object{}, err
	}
	if parent == "" && o.Kind != KindCollection {
		return Object{}, ErrPlace
	}
	// The roots of the view Files stand at the top, and nothing else of that area does: a collection there without a
	// root would show nowhere.
	isRoot, err := isFileRoot(ctx, db.pool, objectID)
	if err != nil {
		return Object{}, err
	}
	if o.Area == AreaFiles && (parent == "") != isRoot {
		return Object{}, ErrPlace
	}
	// Private notes stand at the top: inside another collection they would be an odd private island in a shared tree.
	if o.Private && parent != "" {
		return Object{}, ErrPlace
	}
	wasPrivate, err := isPrivate(ctx, db.pool, objectID)
	if err != nil {
		return Object{}, err
	}
	intoPrivate := false
	if parent != "" {
		p, err := db.Object(ctx, parent, viewer)
		if err != nil {
			return Object{}, err
		}
		if err := db.mayEdit(ctx, p, viewer); err != nil {
			return Object{}, err
		}
		if IsDivider(p.Kind) || p.Area != o.Area || p.Kind == KindFile {
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
		intoPrivate = top.Private && !wasPrivate
	}
	var moved Object
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if after == objectID {
			return ErrPlace
		}
		if intoPrivate {
			// The object is locked, so nothing new lands directly in it while this is checked.
			if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 FOR UPDATE", objectID); err != nil {
				return err
			}
			var foreign bool
			if err := tx.QueryRow(ctx, `WITH RECURSIVE down AS (
					SELECT id, owner FROM objects WHERE id = $1
					UNION ALL SELECT c.id, c.owner FROM objects c JOIN down ON c.parent = down.id
				) SELECT EXISTS (SELECT 1 FROM down WHERE owner <> $2)`, objectID, viewer).Scan(&foreign); err != nil {
				return err
			}
			if foreign {
				return ErrForeignBelow
			}
		}
		position, err := place(ctx, tx, parent, after)
		if err != nil {
			return err
		}
		moved, err = scan(tx.QueryRow(ctx, `UPDATE objects SET parent = nullif($2, ''), position = $3, changed = now() WHERE id = $1 RETURNING `+columns,
			objectID, parent, position))
		if err != nil || !intoPrivate {
			return err
		}
		return endShares(ctx, tx, objectID, viewer)
	})
	return moved, err
}

// endShares takes back every grant and revokes every link on an object and everything below it, as it goes into
// something private, and logs each (ADR-29). The log of something private shows to its owner alone.
func endShares(ctx context.Context, tx pgx.Tx, objectID, actor string) error {
	const down = `WITH RECURSIVE down AS (
			SELECT id FROM objects WHERE id = $1
			UNION ALL SELECT c.id FROM objects c JOIN down ON c.parent = down.id
		) `
	if _, err := tx.Exec(ctx, down+`, gone AS (DELETE FROM grants WHERE object IN (SELECT id FROM down) RETURNING object, kind, who, role)
		INSERT INTO audit_log (actor, action, object, detail)
		SELECT $2, '`+LogGrant+`', object, btrim(kind || ' ' || who) || ' ' || role || ' taken back: moved into something private' FROM gone`,
		objectID, actor); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, down+`, gone AS (DELETE FROM share_links WHERE object IN (SELECT id FROM down) RETURNING object, id, mode)
		INSERT INTO audit_log (actor, action, object, detail)
		SELECT $2, '`+LogLinkRevoked+`', object, id || ' ' || mode || ': moved into something private' FROM gone`, objectID, actor)
	return err
}
