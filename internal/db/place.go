// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"slices"

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
	if err := db.mayDo(ctx, p, owner.ID, ActCreate); err != nil {
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
	if err := db.mayDo(ctx, o, viewer, ActDelete); err != nil {
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

// ErrMoveWidens is a move that would give the mover a right on the object they did not hold: to share it or to manage
// its rights, as the owner of where it goes.
var ErrMoveWidens = fmt.Errorf("%w: there it would be yours to share or to manage, which it is not here; ask whoever manages it to move it", ErrNotAllowed)

// ErrForeignBelow is a move into something private of an object with something of someone else below it: private
// stays private, so it would take that away from its owner (ADR-29).
var ErrForeignBelow = fmt.Errorf("%w: something in it belongs to someone else, and only what is all yours goes into something private", ErrNotAllowed)

// Move puts an object into parent, after the entry named by after or first. Within the parent it lies in, whoever may
// write the object and the parent reorders it; to another place it goes with delete on the object and create on where it
// goes (ADR-36), unless it would give the mover there a right to share or manage it that they do not hold here, since
// the owner of what is above manages it there (ADR-29, 2026-11-19). Only a collection stands at
// the top, private notes stay there, and nothing goes into itself or below itself. Only its owner takes an object into
// something private, and only when everything below it is theirs too; its grants and links end then, and the log keeps
// that.
func (db *DB) Move(ctx context.Context, objectID, viewer, parent, after string) (Object, error) {
	o, err := db.Object(ctx, objectID, viewer)
	if err != nil {
		return Object{}, err
	}
	// Within its parent, write on the object reorders it; to another place it goes with delete where it is and create
	// where it goes (ADR-36), and only if the move gives the mover no right on it they did not hold (below).
	need := ActDelete
	if parent == o.Parent {
		need = ActWrite
	}
	if err := db.mayDo(ctx, o, viewer, need); err != nil {
		return Object{}, err
	}
	widens := map[string]bool{}
	if parent != o.Parent {
		for _, a := range []string{ActShare, ActManage} {
			ok, err := db.May(ctx, o.ID, viewer, a)
			if err != nil {
				return Object{}, err
			}
			widens[a] = !ok
		}
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
		need := ActCreate
		if parent == o.Parent {
			need = ActWrite
		}
		if err := db.mayDo(ctx, p, viewer, need); err != nil {
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
			// The whole tree is locked, so nothing new lands anywhere in it and no owner changes while this is checked.
			if err := lockTree(ctx, tx, objectID); err != nil {
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
		// To another place goes only what the mover may delete, everything below it included, as a deletion would
		// (ADR-36: moving is delete where it is and create where it goes).
		if parent != o.Parent {
			if err := mayTakeAll(ctx, tx, objectID, viewer, "c.deleted_at IS NULL"); err != nil {
				return err
			}
		}
		// Into a library shared over Samba from somewhere else, nothing goes that holds a file breaking the inheritance:
		// Samba knows rights per folder only (ADR-29, ADR-30).
		if parent != o.Parent && o.Area == AreaFiles {
			shared, err := db.sharedRootOf(ctx, tx, parent)
			if err != nil {
				return err
			}
			if shared != "" {
				if err := perFolder(ctx, tx, objectID); err != nil {
					return err
				}
			}
		}
		position, err := place(ctx, tx, parent, after)
		if err != nil {
			return err
		}
		moved, err = scan(tx.QueryRow(ctx, `UPDATE objects SET parent = nullif($2, ''), position = $3, changed = now() WHERE id = $1 RETURNING `+columns,
			objectID, parent, position))
		if err != nil {
			return err
		}
		// Whoever only deletes an object must not move it under something of their own and so come to share it or
		// manage its rights (the attack of ADR-29, 2026-11-19, under the actions of ADR-36).
		if !intoPrivate {
			for _, a := range []string{ActShare, ActManage} {
				if !widens[a] {
					continue
				}
				var now bool
				if err := tx.QueryRow(ctx, "SELECT object_may($1, $2, $3)", objectID, viewer, a).Scan(&now); err != nil {
					return err
				}
				if now {
					return ErrMoveWidens
				}
			}
			return nil
		}
		return endShares(ctx, tx, objectID, viewer)
	})
	return moved, err
}

// lockTree locks an object and everything below it, at every depth, for the rest of the transaction. Making something
// below a locked object, or moving something into it, waits for the lock (the key its parent refers to), and so does
// changing the owner of a locked object. What came in before the lock was granted shows in the next round, which locks it
// too; when a round finds nothing new, nothing can come in any more until the transaction ends.
func lockTree(ctx context.Context, tx pgx.Tx, objectID string) error {
	var held []string
	for {
		var ids []string
		if err := tx.QueryRow(ctx, `WITH locked AS (
				SELECT id FROM objects WHERE id IN (
					WITH RECURSIVE down AS (
						SELECT id FROM objects WHERE id = $1
						UNION ALL SELECT c.id FROM objects c JOIN down ON c.parent = down.id
					) SELECT id FROM down)
				ORDER BY id FOR UPDATE
			) SELECT coalesce(array_agg(id ORDER BY id), '{}') FROM locked`, objectID).Scan(&ids); err != nil {
			return err
		}
		if slices.Equal(ids, held) {
			return nil
		}
		held = ids
	}
}

// endShares takes back every grant and revokes every link on an object and everything below it, as it goes into
// something private, and logs each (ADR-29). The log of something private shows to its owner alone.
func endShares(ctx context.Context, tx pgx.Tx, objectID, actor string) error {
	const down = `WITH RECURSIVE down AS (
			SELECT id FROM objects WHERE id = $1
			UNION ALL SELECT c.id FROM objects c JOIN down ON c.parent = down.id
		) `
	if _, err := tx.Exec(ctx, down+`, gone AS (DELETE FROM grants WHERE object IN (SELECT id FROM down) RETURNING object, kind, who, action, value)
		INSERT INTO audit_log (actor, action, object, detail)
		SELECT $2, '`+LogGrant+`', object, btrim(kind || ' ' || who) || ' ' || action || ' ' || value || ' taken back: moved into something private' FROM gone`,
		objectID, actor); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, down+`, gone AS (DELETE FROM share_links WHERE object IN (SELECT id FROM down) RETURNING object, id, mode)
		INSERT INTO audit_log (actor, action, object, detail)
		SELECT $2, '`+LogLinkRevoked+`', object, id || ' ' || mode || ': moved into something private' FROM gone`, objectID, actor)
	return err
}
