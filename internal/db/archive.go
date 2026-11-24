// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// The archive of spaces (T-34). A space, a collection at the top of the documents or of Files, is archived instead of
// left in the way or deleted: it keeps everything inside it without a limit, the tree and the search hide it unless
// asked for, and it is read only for everyone until it is restored. The one check decides that (object_decide, migration
// 037), so every door holds it: the web, the API, MCP, Samba, share links and upload links. Only the space itself is
// archived, never a collection inside one: a space has one owner who answers for it, and what lies inside a space is
// moved to a space of its own first, or to the trash. Groups have their archive since B-3 (ArchiveGroup).

// notArchived is a condition on o: it is no archived space. Only the lists of the tree leave archived spaces out; an
// object inside one is still found by its address.
const notArchived = `o.archived IS NULL`

// Errors of the archive.
var (
	// ErrArchivePlace is archiving what is no space: a collection inside another, a page, something private, or the
	// folder of the tenant or of a user among Files.
	ErrArchivePlace = errors.New("only a space is archived: a collection at the top, not private, not the folder of the workspace or of a user")
	// ErrArchiver is archiving, restoring or transferring a space by someone who is neither its owner nor an
	// administrator.
	ErrArchiver = fmt.Errorf("%w: only its owner and administrators archive or restore a space", ErrNotAllowed)
	// ErrArchived is a change inside an archived space: it is read only until its owner or an administrator restores it.
	ErrArchived = fmt.Errorf("%w: this space is archived and read only until its owner or an administrator restores it", ErrNotAllowed)
	// ErrWiden is opening up an action one does not hold oneself: allowing it, or taking back a deny of it (T-34).
	ErrWiden = fmt.Errorf("%w: you give or open up only the actions you hold here yourself", ErrNotAllowed)
)

// LogArchive is archiving or restoring a space; its detail says which.
const LogArchive = "archive"

// holdSpace takes the space an object lies in, inside a write transaction, and refuses when it is archived (B-6). Every
// write of content and of the tree goes through it after it locked what it changes (B-7): the blocks of a page (also
// written by an agent, taken back or checked), versions of files, attachments, new objects, a move (where it lies and
// where it goes), a deletion and a new name. Archiving locks that row for update first (setArchived), so it waits for
// such a write, and a write that checked its rights before archiving came finds the archive here instead of landing in
// it.
//
// The lock is FOR KEY SHARE (B-7): it conflicts with the update lock of archiving, not with the lock place takes on a
// parent to put something at a position (FOR NO KEY UPDATE), so a write that holds a page directly in a space and then
// holds the space does not deadlock with a move or a new entry into that space, which holds the space and then shifts
// the page. Spaces of several objects are taken in the order of their IDs.
func holdSpace(ctx context.Context, q execer, objectIDs ...string) error {
	var archived bool
	err := q.QueryRow(ctx, `WITH RECURSIVE up AS (
			SELECT id, parent, 0 AS depth FROM objects WHERE id = ANY ($1::text[])
			UNION ALL SELECT o.id, o.parent, up.depth + 1 FROM objects o JOIN up ON o.id = up.parent WHERE up.depth < 200
		), held AS (
			SELECT archived FROM objects WHERE id IN (SELECT id FROM up WHERE parent IS NULL) ORDER BY id FOR KEY SHARE
		) SELECT coalesce(bool_or(archived IS NOT NULL), false) FROM held`, objectIDs).Scan(&archived)
	if err != nil {
		return err
	}
	if archived {
		return ErrArchived
	}
	return nil
}

// ArchivedSpaces returns the archived spaces of the documents and of Files the viewer may read, by area and title, for
// the tree when the archive is shown.
func (db *DB) ArchivedSpaces(ctx context.Context, viewer string) ([]Object, error) {
	return db.list(ctx, `SELECT `+columns+` FROM objects o WHERE parent IS NULL AND kind = 'collection' AND o.archived IS NOT NULL AND `+alive+` AND `+visible+`
		ORDER BY area, lower(title), created`, viewer)
}

// ArchivedSpaceIDs returns the IDs of every archived space of the tenant. The search leaves their records out unless the
// archive is asked for; it is a filter, never shown to anyone.
func (db *DB) ArchivedSpaceIDs(ctx context.Context) ([]string, error) {
	var ids []string
	err := db.pool.QueryRow(ctx, `SELECT coalesce(array_agg(id ORDER BY id), '{}') FROM objects
		WHERE parent IS NULL AND archived IS NOT NULL AND deleted_at IS NULL`).Scan(&ids)
	return nonNil(ids), err
}

// InArchive tells whether an object lies in an archived space, or is one.
func (db *DB) InArchive(ctx context.Context, objectID string) (bool, error) {
	var in bool
	err := db.pool.QueryRow(ctx, "SELECT object_archived($1)", objectID).Scan(&in)
	return in, err
}

// forArchive returns a space the actor may archive or restore: its owner or an administrator, in a browser session,
// never an agent, a service account, a key or an app password (as deleting for good, B-3). An administrator finds every
// space that is not private, also one whose content they may not read (administrators manage rights, not content).
func (db *DB) forArchive(ctx context.Context, objectID string, actor Actor) (Object, error) {
	if err := browserSession(ctx, actor.ID); err != nil {
		return Object{}, err
	}
	o, err := db.Object(ctx, objectID, actor.ID)
	if errors.Is(err, ErrNotFound) && actor.Admin {
		o, err = scan(db.pool.QueryRow(ctx, `SELECT `+columns+` FROM objects o WHERE id = $1 AND `+alive+` AND NOT private`, objectID))
		if errors.Is(err, pgx.ErrNoRows) {
			err = ErrNotFound
		}
	}
	if err != nil {
		return Object{}, err
	}
	if o.Parent != "" || o.Kind != KindCollection || o.Private {
		return Object{}, ErrArchivePlace
	}
	if fixed, err := isFixedRoot(ctx, db.pool, o.ID); err != nil || fixed {
		if err == nil {
			err = ErrArchivePlace
		}
		return Object{}, err
	}
	if o.Owner != actor.ID && !actor.Admin {
		return Object{}, ErrArchiver
	}
	return o, nil
}

// MayArchive tells whether the actor of the request may archive or restore a space, for the workspace to offer it.
func (db *DB) MayArchive(ctx context.Context, objectID string, actor Actor) (bool, error) {
	_, err := db.forArchive(ctx, objectID, actor)
	if isNotAllowed(err) || errors.Is(err, ErrArchivePlace) || errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// SetArchived archives a space, or restores it with archived false (T-34). Only its owner or an administrator does, in
// a browser session. Archived, it keeps everything inside it, its entries and its links, without a limit; it is hidden
// from the tree and the search unless asked for, and read only for everyone: the one check refuses comment, create,
// write, delete and share inside it, also to its owner, and a link lends nothing while it is archived. Restoring brings
// all of it back. Archiving what is archived, or restoring what is not, changes nothing.
func (db *DB) SetArchived(ctx context.Context, objectID string, actor Actor, archived bool) error {
	o, err := db.forArchive(ctx, objectID, actor)
	if err != nil {
		return err
	}
	if o.Archived == archived {
		return nil
	}
	return db.setArchived(ctx, o.ID, actor.ID, archived)
}

func (db *DB) setArchived(ctx context.Context, objectID, by string, archived bool) error {
	if archived {
		// The lock for update waits for every write that holds the space (holdSpace, FOR KEY SHARE), and those after it
		// find it archived.
		return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 AND parent IS NULL FOR UPDATE", objectID); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE objects SET archived = coalesce(archived, now()), archived_by = $2, changed = now()
				WHERE id = $1 AND parent IS NULL`, objectID, by)
			return err
		})
	}
	// What lay in its trash gets the time back that the space was archived, so its 30 days pause while archived (B-6).
	// Everything deleted together moves by the same span, so it stays together.
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		var since *time.Time
		if err := tx.QueryRow(ctx, "SELECT archived FROM objects WHERE id = $1 FOR UPDATE", objectID).Scan(&since); err != nil {
			return err
		}
		if since != nil {
			if _, err := tx.Exec(ctx, `WITH RECURSIVE down AS (
					SELECT id FROM objects WHERE id = $1
					UNION ALL SELECT c.id FROM objects c JOIN down ON c.parent = down.id
				) UPDATE objects SET deleted_at = deleted_at + (now() - $2::timestamptz)
				WHERE id IN (SELECT id FROM down) AND deleted_at IS NOT NULL`, objectID, *since); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `UPDATE objects SET archived = NULL, archived_by = '', changed = now() WHERE id = $1`, objectID)
		return err
	})
}

// ArchiveAsOperator archives a space without a request, as whoever runs the instance: for an import that brings an
// archived space (T-28). A request of anyone is refused; in the web, SetArchived is the way.
func (db *DB) ArchiveAsOperator(ctx context.Context, objectID string) error {
	if _, ok := account.From(ctx); ok {
		return ErrSession
	}
	var place bool
	if err := db.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM objects WHERE id = $1 AND parent IS NULL AND kind = 'collection'
		AND NOT private AND deleted_at IS NULL)`, objectID).Scan(&place); err != nil {
		return err
	}
	if !place {
		return ErrArchivePlace
	}
	if fixed, err := isFixedRoot(ctx, db.pool, objectID); err != nil || fixed {
		if err == nil {
			err = ErrArchivePlace
		}
		return err
	}
	return db.setArchived(ctx, objectID, "", true)
}

// ImportGroup makes a local group without a request, as whoever runs the instance, archived where the import says so
// (T-28): the team wiki keeps archived groups, and they arrive archived. Inside, parent names a local group that exists;
// an archived group goes inside nothing (B-3), so it comes without a parent. A request of anyone is refused; in the web,
// CreateGroup and ArchiveGroup are the way.
func (db *DB) ImportGroup(ctx context.Context, name, description, parent string, archived bool) (Group, error) {
	if _, ok := account.From(ctx); ok {
		return Group{}, ErrSession
	}
	name, description = strings.TrimSpace(name), strings.TrimSpace(description)
	if n := len([]rune(name)); n < 1 || n > 200 || len([]rune(description)) > 1000 {
		return Group{}, ErrGroupName
	}
	if archived && parent != "" {
		return Group{}, ErrGroupArchived
	}
	groupID := ""
	err := pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if err := lockGroups(ctx, tx); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `INSERT INTO groups (id, name, source, description, archived) VALUES (new_id(), $1, 'local', $2,
			CASE WHEN $3 THEN now() END) RETURNING id`, name, description, archived).Scan(&groupID); err != nil {
			return err
		}
		if parent == "" {
			return nil
		}
		_, err := tx.Exec(ctx, "INSERT INTO group_groups (grp, member) VALUES ($1, $2)", parent, groupID)
		return err
	})
	if isUnique(err, "groups_tenant_source_name_key") {
		return Group{}, ErrGroupName
	}
	if err != nil {
		return Group{}, err
	}
	return db.GroupOf(ctx, groupID)
}

// mayOpenUp refuses a change of entries that opens up an action the actor does not hold on the object (T-34): whoever
// manages without delete (the preset Members) gives nobody delete, also not through the whole tenant, and takes back no
// deny of it. Narrowing is always allowed: taking an allow back, or denying. Administrators manage rights whatever they
// hold (ADR-36).
//
// An entry holds below the object too, wherever the inheritance is not broken, so the actor must hold the action on each
// of those as well, as mayTakeAll asks for deleting (B-6): a manager with a deny of delete on a page inside gives nobody
// delete on the folder around it. What is private below gets no entries, so it does not count. What lies in the trash
// below counts too (B-7): restored, it would inherit the opening despite its deny. q is the transaction of the change,
// so that what it reads is what it changes.
func (db *DB) mayOpenUp(ctx context.Context, q execer, o Object, actor Actor, opened []string) error {
	if actor.Admin || len(opened) == 0 {
		return nil
	}
	var shut bool
	if err := q.QueryRow(ctx, `WITH RECURSIVE down AS (
			SELECT id, 0 AS depth FROM objects WHERE id = $1
			UNION ALL SELECT c.id, down.depth + 1 FROM objects c JOIN down ON c.parent = down.id
				WHERE c.inherit AND NOT c.private AND down.depth < 200
		) SELECT EXISTS (SELECT 1 FROM down CROSS JOIN unnest($3::text[]) a WHERE NOT object_may(down.id, $2, a))`,
		o.ID, actor.ID, opened).Scan(&shut); err != nil {
		return err
	}
	if shut {
		return ErrWiden
	}
	return nil
}

// openedBy names the actions that new entries of one holder open up against the old ones: an action now allowed where
// it was not allowed the same way, or one denied before that is no longer denied the same way.
func openedBy(old map[string]Grant, actions map[string]string, subgroups bool) []string {
	var opened []string
	for _, a := range Actions {
		before, had := old[a]
		now := actions[a]
		if had && before.Actions[a] == now && before.Subgroups == subgroups {
			continue
		}
		if !had && now == Inherit {
			continue
		}
		if now == Allow || (had && before.Actions[a] == Deny) {
			opened = append(opened, a)
		}
	}
	return opened
}

// entriesOf reads the entries of one holder on an object, per action.
func entriesOf(ctx context.Context, q execer, objectID, kind, who string) (map[string]Grant, error) {
	var actions, values []string
	var subgroups []bool
	if err := q.QueryRow(ctx, `SELECT coalesce(array_agg(action ORDER BY action), '{}'), coalesce(array_agg(value ORDER BY action), '{}'),
			coalesce(array_agg(subgroups ORDER BY action), '{}') FROM grants WHERE object = $1 AND kind = $2 AND who = $3`,
		objectID, kind, who).Scan(&actions, &values, &subgroups); err != nil {
		return nil, err
	}
	out := map[string]Grant{}
	for i, a := range actions {
		if i < len(values) && i < len(subgroups) {
			out[a] = Grant{Actions: map[string]string{a: values[i]}, Subgroups: subgroups[i]}
		}
	}
	return out, nil
}
