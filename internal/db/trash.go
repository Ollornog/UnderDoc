// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
// object and everything below it may delete it (ADR-29), a collection at the top only its owner, since it holds what
// others made; something below that breaks the inheritance and shuts the viewer out stops the deletion. Private notes
// are never deleted, also not with something they were moved into. A line or a space has nothing to restore, so it goes
// for good, as DeleteDivider does.
func (db *DB) Delete(ctx context.Context, objectID, viewer string) error {
	return db.deleteObject(ctx, objectID, viewer, nil)
}

// deleteObject is Delete, with a step before the trash in the same transaction.
func (db *DB) deleteObject(ctx context.Context, objectID, viewer string, before func(pgx.Tx) error) error {
	o, err := db.Object(ctx, objectID, viewer)
	if err != nil {
		return err
	}
	if IsDivider(o.Kind) {
		return db.DeleteDivider(ctx, objectID, viewer)
	}
	if err := db.mayDo(ctx, o, viewer, ActDelete); err != nil {
		return err
	}
	if err := db.guardFile(ctx, o); err != nil {
		return err
	}
	if o.Parent == "" && o.Owner != viewer {
		return ErrNotAllowed
	}
	// A space goes to the trash only by a person (B-5): an agent moves pages, never a whole space.
	if o.Parent == "" && requestAgent(ctx) {
		return ErrAgent
	}
	// The folder of the tenant and of each user stay: they are where files land (ADR-28).
	if fixed, err := isFixedRoot(ctx, db.pool, objectID); err != nil || fixed {
		if err == nil {
			err = ErrNotAllowed
		}
		return err
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
		if err := mayTakeAll(ctx, tx, objectID, viewer, "c.deleted_at IS NULL"); err != nil {
			return err
		}
		if before != nil {
			if err := before(tx); err != nil {
				return err
			}
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
	err := row.Scan(append(fields(o), &t.DeletedAt, &t.DeletedBy, &t.Place, &t.PlaceDeleted)...)
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
		if err := db.mayDo(ctx, p, viewer, ActCreate); err != nil {
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

// ErrHeldBelow is a deletion of something with an object below it on which the viewer may not edit: one that breaks the
// inheritance and shuts them out. It stays where it is; the deletion does not happen (ADR-29, 2026-11-19).
var ErrHeldBelow = fmt.Errorf("%w: something below it is not open to you, so it cannot go with it; its owner moves it out first", ErrNotAllowed)

// mayTakeAll tells whether the viewer may edit the object and everything below it that the condition on c keeps, so a
// deletion takes nothing they could not delete on its own.
func mayTakeAll(ctx context.Context, tx pgx.Tx, objectID, viewer, below string) error {
	var shut bool
	if err := tx.QueryRow(ctx, `WITH RECURSIVE down AS (
			SELECT id FROM objects WHERE id = $1
			UNION ALL SELECT c.id FROM objects c JOIN down ON c.parent = down.id WHERE `+below+`
		) SELECT EXISTS (SELECT 1 FROM down WHERE NOT object_may(id, $2, 'delete'))`, objectID, viewer).Scan(&shut); err != nil {
		return err
	}
	if shut {
		return ErrHeldBelow
	}
	return nil
}

// Errors of deleting for good (B-3).
var (
	// ErrPurger is deleting for good by someone who neither owns it nor administers the tenant.
	ErrPurger = fmt.Errorf("%w: only its owner or an administrator deletes it for good", ErrNotAllowed)
	// ErrConfirmName is deleting for good without typing the name of what goes.
	ErrConfirmName = errors.New("type its name exactly to delete it for good")
)

// TrashName is the name to type to empty the whole trash.
const TrashName = "Trash"

// Actions of the app that a signed one-time approval of the gate confirms (ADR-35, T-33). The type the gate signs is
// the id of the app in the gate followed by the action, such as underdoc.object.purge: the operator registers both.
const (
	// ApprovePurge is deleting one object of the trash for good, with what lies below it; the approval names the object.
	ApprovePurge = "object.purge"
	// ApproveEmptyTrash is emptying the trash: one approval for all that goes (B-5), bound to a cutoff, so that only
	// what lay in the trash at that moment goes (B-6).
	ApproveEmptyTrash = "trash.empty"
)

// Approver confirms a dangerous action with a signed one-time approval of the gate, its jti used once (ADR-35, T-33).
// Approve runs after every other check of the action, with what goes locked, right before it goes: a confirmation is
// used up only by an action that goes (B-4 finding 8, B-6). subject is what the approval is bound to: the object for
// ApprovePurge, the cutoff in Unix seconds for ApproveEmptyTrash. Until T-33 wires the gate's VerifyApproval there is
// none, and such actions stay with owners and administrators in the browser.
type Approver interface {
	Approve(ctx context.Context, action, subject string) error
}

// SetApprover makes deleting for good wait for an approval of the gate; nil (the default) asks for none. Set once
// before the first request.
func (db *DB) SetApprover(a Approver) { db.approver = a }

// Purge takes an object of the trash away for good, with everything below it, its blocks and their versions. Only its
// owner or an administrator does, in a browser session, never an agent or a key (ADR-35, B-3), with its name typed as
// confirm, and with the approval of the gate once there is one (SetApprover). It must lie in their trash (they deleted
// it or own it), and they must be allowed to delete everything that goes with it.
func (db *DB) Purge(ctx context.Context, objectID string, actor Actor, confirm string) error {
	t, err := db.forPurge(ctx, objectID, actor)
	if err != nil {
		return err
	}
	if strings.TrimSpace(confirm) != t.Title {
		return ErrConfirmName
	}
	return db.purge(ctx, objectID, actor)
}

// forPurge returns an object of the trash the actor may delete for good, as far as that can be told before the name.
func (db *DB) forPurge(ctx context.Context, objectID string, actor Actor) (Trashed, error) {
	// Agents never delete for good (ADR-36), and neither does anything but a person in the browser (ADR-35).
	if err := browserSession(ctx, actor.ID); err != nil {
		return Trashed{}, err
	}
	t, err := db.inTrash(ctx, objectID, actor.ID)
	if err != nil {
		return Trashed{}, err
	}
	if t.Owner != actor.ID && !actor.Admin {
		return Trashed{}, ErrPurger
	}
	return t, nil
}

// purge deletes for good: it locks what goes, checks, asks the gate for its approval and then deletes, nothing else in
// between (B-5, B-6): an approval is used up only by a purge that goes, never by one the check refuses.
func (db *DB) purge(ctx context.Context, objectID string, actor Actor) error {
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, "SELECT id FROM objects WHERE id = $1 AND deleted_at IS NOT NULL FOR UPDATE", objectID).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := mayTakeAll(ctx, tx, objectID, actor.ID, "true"); err != nil {
			return err
		}
		if db.approver != nil {
			if err := db.approver.Approve(ctx, ApprovePurge, objectID); err != nil {
				return err
			}
		}
		// One statement takes the whole tree, so the parent of each is gone in the same moment as it is.
		tag, err := tx.Exec(ctx, `WITH RECURSIVE down AS (
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
	})
}

// TrashDays is how long something stays in the trash before it goes for good by itself (2026-11-09).
const TrashDays = 30

// EmptyTrash takes away for good everything in the trash the actor may purge: what they own, and for an administrator
// what they deleted too. The rules of Purge hold, with TrashName typed as confirm. What someone deleted but does not own
// stays for its owner until it goes by itself. It returns how many deletions went.
func (db *DB) EmptyTrash(ctx context.Context, actor Actor, confirm string) (int, error) {
	return db.EmptyTrashBefore(ctx, actor, confirm, time.Now())
}

// EmptyTrashBefore empties the trash as EmptyTrash does, of what was deleted at or before cutoff only: the moment an
// approval of the gate was asked for, so that nothing deleted after it goes with it (B-6). Everything is checked and
// locked first, then the gate's approval is asked for once, bound to the cutoff, and then it all goes in one statement.
func (db *DB) EmptyTrashBefore(ctx context.Context, actor Actor, confirm string, cutoff time.Time) (int, error) {
	// The candidates whatever their moment; the lock below takes only those deleted at or before the cutoff.
	purgeable, err := db.purgeableTrash(ctx, actor, confirm, nil)
	if err != nil || len(purgeable) == 0 {
		return 0, err
	}
	gone := 0
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		// What goes is locked, so nothing comes below it and nothing of it is taken out before it goes.
		rows, err := tx.Query(ctx, `SELECT id FROM objects WHERE id = ANY ($1::text[]) AND deleted_at IS NOT NULL AND deleted_at <= $2
			ORDER BY id FOR UPDATE`, purgeable, cutoff)
		if err != nil {
			return err
		}
		locked, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		var take []string
		for _, id := range locked {
			// Something below that is not open to the actor keeps its top in the trash for its owner.
			if err := mayTakeAll(ctx, tx, id, actor.ID, "true"); errors.Is(err, ErrHeldBelow) {
				continue
			} else if err != nil {
				return err
			}
			take = append(take, id)
		}
		if len(take) == 0 {
			return nil
		}
		// One approval of the gate covers emptying the trash, never one per item (B-5); nothing but the deletion follows.
		if db.approver != nil {
			if err := db.approver.Approve(ctx, ApproveEmptyTrash, strconv.FormatInt(cutoff.Unix(), 10)); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `WITH RECURSIVE down AS (
				SELECT id FROM objects WHERE id = ANY ($1::text[]) AND deleted_at IS NOT NULL
				UNION ALL SELECT c.id FROM objects c JOIN down ON c.parent = down.id
			) DELETE FROM objects WHERE id IN (SELECT id FROM down)`, take); err != nil {
			return err
		}
		gone = len(take)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return gone, nil
}

// PurgeableTrash returns what emptying the trash would take for the actor, as far as can be told before anything is
// locked: the objects at the top of the trash deleted at or before cutoff that they may delete for good, with TrashName
// typed as confirm.
func (db *DB) PurgeableTrash(ctx context.Context, actor Actor, confirm string, cutoff time.Time) ([]string, error) {
	return db.purgeableTrash(ctx, actor, confirm, &cutoff)
}

func (db *DB) purgeableTrash(ctx context.Context, actor Actor, confirm string, cutoff *time.Time) ([]string, error) {
	if err := browserSession(ctx, actor.ID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(confirm) != TrashName {
		return nil, ErrConfirmName
	}
	list, err := db.Trash(ctx, actor.ID)
	if err != nil {
		return nil, err
	}
	var purgeable []string
	for _, t := range list {
		if cutoff != nil && t.DeletedAt.After(*cutoff) {
			continue
		}
		if _, err := db.forPurge(ctx, t.ID, actor); errors.Is(err, ErrPurger) || errors.Is(err, ErrNotFound) {
			continue
		} else if err != nil {
			return nil, err
		}
		purgeable = append(purgeable, t.ID)
	}
	return purgeable, nil
}

// ExpireTrash takes away for good what was deleted before the given moment, for every account of the tenant of ctx.
// Something deleted on its own before what lay above it is older still, so it goes in the same statement, and nothing
// is left without its parent.
//
// The trash of an archived space waits: nothing of it can be restored while the space is archived, so its time does not
// run, and restoring the space moves the moment of its deletion on by as long as the space was archived (B-6).
func (db *DB) ExpireTrash(ctx context.Context, before time.Time) (int64, error) {
	tag, err := db.pool.Exec(ctx, "DELETE FROM objects WHERE deleted_at IS NOT NULL AND deleted_at < $1 AND NOT object_archived(id)", before)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// ExpireAllTrash runs ExpireTrash for every tenant, each as itself, where on says the trash is on for that tenant (nil:
// everywhere). An archived tenant keeps its trash as it is, and so does one with Documents switched off (ADR-25). It
// returns how many objects went in all.
func (db *DB) ExpireAllTrash(ctx context.Context, before time.Time, on func(context.Context) bool) (int64, error) {
	var gone int64
	err := db.ForEachTenant(ctx, func(ctx context.Context) error {
		if on != nil && !on(ctx) {
			return nil
		}
		n, err := db.ExpireTrash(ctx, before)
		gone += n
		return err
	})
	return gone, err
}

// KeepTrash empties the trash of every tenant where on says it is on (nil: everywhere) of what lies there longer than
// TrashDays, and deletes the entries of the log of every tenant older than it keeps them (ExpireLog), at once and then every so often, until ctx ends. What fails is told to onError
// and tried again next time.
func (db *DB) KeepTrash(ctx context.Context, every time.Duration, on func(context.Context) bool, onError func(error)) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		if _, err := db.ExpireAllTrash(ctx, time.Now().Add(-TrashDays*24*time.Hour), on); err != nil && ctx.Err() == nil {
			onError(err)
		}
		if err := db.ForEachTenant(ctx, func(ctx context.Context) error {
			_, err := db.ExpireLog(ctx, time.Now())
			return err
		}); err != nil && ctx.Err() == nil {
			onError(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
