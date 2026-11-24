// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/pagekind"
)

// Changes by agents are visible (ADR-33, T-20). Every version knows whether an agent wrote it (migration 034, from the
// sign-in of the request, never from what a client sends). On human documentation every block an agent changed keeps a
// hint until a person with the right to write checks it, one change at a time with the diff on screen, or takes it back;
// a person's own change of the block ends it as well. An agent never checks, not even for its person.

// ErrStale is a check or a taking back that names a change which is not the open one of the block: an older one, one of
// another block, or one that a newer change of an agent followed.
var ErrStale = errors.New("this is not the open change of the block: open it again to see what changed since")

// ErrMovedAway is the taking back of a block an agent moved to another page: it is moved back from there.
var ErrMovedAway = errors.New("an agent moved this block to another page: move it back from there")

// noteChange keeps the hint of a change of a block that was just written, with its version: a change by an agent of
// human documentation opens or extends the hint, a person's change ends it. near are the blocks before and after one that
// was deleted.
func noteChange(ctx context.Context, tx pgx.Tx, pageID, blockID string, near [2]string) error {
	if !requestAgent(ctx) {
		_, err := tx.Exec(ctx, "DELETE FROM ai_changes WHERE page = $1 AND block = $2", pageID, blockID)
		return err
	}
	// A change of an agent that read narrower content before is marked on every kind of page (T-38); the mark stays
	// until a person checks the block, also when a later change of an agent comes without it.
	wider := requestWider(ctx)
	_, err := tx.Exec(ctx, `
		INSERT INTO ai_changes (page, block, base, head, follows, precedes, wider)
		SELECT $1, $2, (SELECT max(version) FROM block_versions WHERE page = $1 AND block = $2 AND version < h.v), h.v, $3, $4, $6
		FROM (SELECT max(version) AS v FROM block_versions WHERE page = $1 AND block = $2) h
		WHERE $6 OR EXISTS (SELECT 1 FROM objects WHERE id = $1 AND kind = 'page' AND page_kind = ANY($5))
		ON CONFLICT (tenant, page, block) DO UPDATE SET head = EXCLUDED.head, follows = EXCLUDED.follows, precedes = EXCLUDED.precedes,
			at = now(), wider = ai_changes.wider OR EXCLUDED.wider`,
		pageID, blockID, near[0], near[1], pagekind.PeopleKinds, wider)
	if err != nil {
		return err
	}
	// A block an agent made and took out again before anyone checked it changed nothing people wrote.
	_, err = tx.Exec(ctx, `DELETE FROM ai_changes c WHERE c.page = $1 AND c.block = $2 AND c.base IS NULL
		AND EXISTS (SELECT 1 FROM block_versions v WHERE v.version = c.head AND v.body IS NULL)`, pageID, blockID)
	return err
}

// AIChange is a change of a block by agents that nobody checked yet.
type AIChange struct {
	Block string
	// Head is the latest change, which a check names; Base the version before the first, 0 when an agent made the block.
	Head, Base int64
	// Deleted tells that the block is gone; Moved names where to, when it was moved to another page.
	Deleted bool
	Moved   string
	// By is the name of whom the agent works for, At when it changed the block last.
	By string
	At time.Time
	// Wider tells that an agent wrote it after reading what fewer may read than may read here (T-38).
	Wider bool
	// Near are the blocks before and after a deleted one, where it comes back.
	Near [2]string
}

// Checked is the last check of a page: who and when.
type Checked struct {
	By string
	At time.Time
}

// AIChanges returns the open changes of agents in a page the viewer sees, in the order of the page, the deleted ones
// last; none on a page that is not human documentation, since the agent side carries no hint per change (ADR-33).
func (db *DB) AIChanges(ctx context.Context, pageID, viewer string) ([]AIChange, error) {
	o, err := db.page(ctx, pageID, viewer)
	if err != nil || o.Kind != KindPage {
		return nil, err
	}
	// The agent side carries a hint only for a change marked as written after reading narrower content (T-38).
	onlyWider := !pagekind.ForPeople(o.PageKind)
	rows, err := db.pool.Query(ctx, `
		SELECT c.block, c.head, coalesce(c.base, 0), h.body IS NULL, CASE WHEN h.origin LIKE 'moved to %' THEN substr(h.origin, 10) ELSE '' END,
			coalesce(a.name, h.author), h.at, c.follows, c.precedes, c.wider
		FROM ai_changes c JOIN block_versions h ON h.version = c.head LEFT JOIN accounts a ON a.id = h.author
		LEFT JOIN blocks b ON b.page = c.page AND b.id = c.block
		WHERE c.page = $1 AND (c.wider OR NOT $2) ORDER BY b.position IS NULL, b.position, c.head`, pageID, onlyWider)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AIChange, error) {
		var c AIChange
		err := row.Scan(&c.Block, &c.Head, &c.Base, &c.Deleted, &c.Moved, &c.By, &c.At, &c.Near[0], &c.Near[1], &c.Wider)
		return c, err
	})
}

// LastCheck returns the latest check of a block of a page the viewer sees, if any.
func (db *DB) LastCheck(ctx context.Context, pageID, viewer string) (*Checked, error) {
	if _, err := db.page(ctx, pageID, viewer); err != nil {
		return nil, err
	}
	var c Checked
	err := db.pool.QueryRow(ctx, `SELECT checked_name, checked_at FROM blocks WHERE page = $1 AND checked_at IS NOT NULL
		ORDER BY checked_at DESC LIMIT 1`, pageID).Scan(&c.By, &c.At)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &c, err
}

// Diff is one open change of agents with the text before and now, for whoever checks it.
type Diff struct {
	AIChange
	Before, Now string
}

// ChangeOf returns the open change of agents of one block with its text before and now. It shows what was taken out, so
// only whoever may change the page reads it, as the versions (ADR-29).
func (db *DB) ChangeOf(ctx context.Context, pageID, blockID, viewer string) (Diff, error) {
	o, err := db.page(ctx, pageID, viewer)
	if err != nil {
		return Diff{}, err
	}
	if err := db.mayDo(ctx, o, viewer, ActWrite); err != nil {
		return Diff{}, err
	}
	changes, err := db.AIChanges(ctx, pageID, viewer)
	if err != nil {
		return Diff{}, err
	}
	for _, c := range changes {
		if c.Block != blockID {
			continue
		}
		d := Diff{AIChange: c}
		err := db.pool.QueryRow(ctx, `SELECT coalesce((SELECT body FROM block_versions WHERE version = $3 AND page = $1 AND block = $2), ''),
			coalesce((SELECT body FROM blocks WHERE page = $1 AND id = $2), '')`, pageID, blockID, c.Base).Scan(&d.Before, &d.Now)
		return d, err
	}
	return Diff{}, ErrNotFound
}

// review runs a check or a taking back of the open change of a block: only a person who may change the page, only the
// change the person saw (head), with the page locked.
func (db *DB) review(ctx context.Context, pageID, blockID, viewer string, head int64, do func(tx pgx.Tx, c AIChange, base *string) error) error {
	if err := noAgent(ctx); err != nil {
		return err
	}
	o, err := db.page(ctx, pageID, viewer)
	if err != nil {
		return err
	}
	if err := db.mayDo(ctx, o, viewer, ActWrite); err != nil {
		return err
	}
	if o.Kind != KindPage {
		return ErrNotFound
	}
	// On the agent side only a change marked as written after reading narrower content is checked (T-38).
	onlyWider := !pagekind.ForPeople(o.PageKind)
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 FOR UPDATE", pageID); err != nil {
			return err
		}
		if err := holdSpace(ctx, tx, pageID); err != nil {
			return err
		}
		var c AIChange
		var origin string
		var base *string
		err := tx.QueryRow(ctx, `SELECT c.head, coalesce(c.base, 0), c.follows, c.precedes, h.body IS NULL, h.origin, v.body
			FROM ai_changes c JOIN block_versions h ON h.version = c.head LEFT JOIN block_versions v ON v.version = c.base
			WHERE c.page = $1 AND c.block = $2 AND (c.wider OR NOT $3) FOR UPDATE OF c`, pageID, blockID, onlyWider).Scan(&c.Head, &c.Base, &c.Near[0], &c.Near[1], &c.Deleted, &origin, &base)
		if errors.Is(err, pgx.ErrNoRows) && onlyWider {
			// The agent side has nothing to check but a marked change.
			return ErrNotFound
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrStale
		}
		if err != nil {
			return err
		}
		if c.Head != head {
			return ErrStale
		}
		c.Block = blockID
		if strings.HasPrefix(origin, "moved to ") {
			c.Moved = strings.TrimPrefix(origin, "moved to ")
		}
		return do(tx, c, base)
	})
}

// CheckChange marks the open change of agents of a block as checked by the viewer: the hint goes, and the block keeps
// who checked it and when.
func (db *DB) CheckChange(ctx context.Context, pageID, blockID, viewer string, head int64) error {
	return db.review(ctx, pageID, blockID, viewer, head, func(tx pgx.Tx, c AIChange, _ *string) error {
		if _, err := tx.Exec(ctx, "DELETE FROM ai_changes WHERE page = $1 AND block = $2", pageID, blockID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE blocks SET checked_by = $3, checked_name = coalesce((SELECT name FROM accounts WHERE id = $3), $3),
			checked_at = now() WHERE page = $1 AND id = $2`, pageID, blockID, viewer)
		return err
	})
}

// TakeBack undoes the open change of agents of a block as the viewer: the text before comes back, a block an agent made
// goes, and a block an agent deleted comes back where it stood. It is the viewer's own change, so the hint goes.
func (db *DB) TakeBack(ctx context.Context, pageID, blockID, viewer string, head int64, ways map[string]Way) error {
	return db.review(ctx, pageID, blockID, viewer, head, func(tx pgx.Tx, c AIChange, base *string) error {
		switch {
		case c.Moved != "":
			return ErrMovedAway
		case c.Deleted && base != nil:
			return restoreBlock(ctx, tx, pageID, blockID, c.Near, *base, viewer)
		case base == nil:
			return db.deleteBlock(ctx, tx, pageID, blockID, viewer, ways)
		default:
			if err := checkBody(*base); err != nil {
				return err
			}
			return db.editBlock(ctx, tx, pageID, blockID, viewer, *base, ways)
		}
	})
}

// restoreBlock puts a deleted block back with its ID and text where it stood: after the block before it, else before the
// block after it, else at the end, else first when nothing is left.
func restoreBlock(ctx context.Context, tx pgx.Tx, pageID, blockID string, near [2]string, body, viewer string) error {
	if err := roomForBytes(ctx, tx, int64(len(body))); err != nil {
		return err
	}
	var position int
	err := tx.QueryRow(ctx, `SELECT coalesce(
			(SELECT position + 1 FROM blocks WHERE page = $1 AND id = $2 AND $2 <> ''),
			(SELECT position FROM blocks WHERE page = $1 AND id = $3 AND $3 <> ''),
			(SELECT max(position) + 1 FROM blocks WHERE page = $1), 0)`, pageID, near[0], near[1]).Scan(&position)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE blocks SET position = position + 1 WHERE page = $1 AND position >= $2", pageID, position); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO blocks (page, id, position, body) VALUES ($1, $2, $3, $4)", pageID, blockID, position, body); err != nil {
		return err
	}
	if err := syncRefs(ctx, tx, pageID, blockID, body); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE objects SET changed = now() WHERE id = $1", pageID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO block_versions (page, block, body, author, origin) VALUES ($1, $2, $3, $4, 'taken back')", pageID, blockID, body, viewer); err != nil {
		return err
	}
	return noteChange(ctx, tx, pageID, blockID, [2]string{})
}

// MarkPageChecked marks every block of a page as checked by name on a day, and ends every hint of the page: what an
// import (T-28) does with `geprüft: <KÜRZEL> <JJJJ-MM-TT>` of a team wiki (pagekind.ParseChecked). Only a person who
// may change the page does it, never an agent; it has no door of its own in the API.
func (db *DB) MarkPageChecked(ctx context.Context, pageID, viewer, name string, at time.Time) error {
	if err := noAgent(ctx); err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 200 {
		return ErrStale
	}
	o, err := db.page(ctx, pageID, viewer)
	if err != nil {
		return err
	}
	if err := db.mayDo(ctx, o, viewer, ActWrite); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "DELETE FROM ai_changes WHERE page = $1", pageID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE blocks SET checked_by = '', checked_name = $2, checked_at = $3 WHERE page = $1", pageID, name, at)
		return err
	})
}

// maxToCheck is how many pages the filter "AI changes to check" names at most.
const maxToCheck = 1000

// PagesToCheck returns the pages of human documentation the viewer sees that have changes of agents nobody checked, for
// the search filter "AI changes to check": the newest change first.
func (db *DB) PagesToCheck(ctx context.Context, viewer string) ([]string, error) {
	rows, err := db.pool.Query(ctx, `SELECT o.id FROM objects o JOIN ai_changes c ON c.page = o.id
		WHERE o.kind = 'page' AND (o.page_kind = ANY($2) OR c.wider) AND `+alive+` AND `+visible+`
		GROUP BY o.id ORDER BY max(c.at) DESC, o.id LIMIT $3`, viewer, pagekind.PeopleKinds, maxToCheck)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// ErrAgentKind is an agent that would take a page people wrote out of the human documentation, which would hide the
// hints of its changes (ADR-33).
var ErrAgentKind = errors.New("agents may not take a page that people wrote out of the human documentation; ask a person to change its kind: " + ErrNotAllowed.Error())

// mayLeaveHumanDocs refuses an agent that changes the kind of a page from human documentation to another side while a
// person wrote any of it. A page only agents wrote they may class as they like.
func (db *DB) mayLeaveHumanDocs(ctx context.Context, o Object, kind string) error {
	if !requestAgent(ctx) || !pagekind.ForPeople(o.PageKind) || pagekind.ForPeople(kind) {
		return nil
	}
	var byPerson bool
	if err := db.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM block_versions WHERE page = $1 AND NOT agent)", o.ID).Scan(&byPerson); err != nil {
		return err
	}
	if byPerson {
		return errors.Join(ErrNotAllowed, ErrAgentKind)
	}
	return nil
}
