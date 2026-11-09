// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// MaxBlock is the longest a block may be, in characters.
const MaxBlock = 100000

// ErrBlock is a block that is too long.
var ErrBlock = errors.New("a block has at most 100000 characters")

// Block is one piece of Markdown in a page, with an ID that stays with it (ADR-18, ADR-19).
type Block struct {
	ID   string
	Body string
}

// Version is one change of a block: its new text, or its deletion.
type Version struct {
	Number  int64
	Block   string
	Body    string
	Deleted bool
	Author  string
	At      time.Time
}

// mayEdit tells whether the viewer may change an object: its owner always, anyone else while others may edit (ADR-20).
func (db *DB) mayEdit(ctx context.Context, o Object, viewer string) error {
	if o.Owner == viewer {
		return nil
	}
	access, err := db.Access(ctx, o.ID)
	if err != nil {
		return err
	}
	if access != "edit" {
		return ErrNotAllowed
	}
	return nil
}

// CreatePage makes an empty page inside parent, which the owner must see and may change.
func (db *DB) CreatePage(ctx context.Context, owner account.User, title, parent string) (Object, error) {
	p, err := db.Object(ctx, parent, owner.ID)
	if err != nil {
		return Object{}, err
	}
	if err := db.mayEdit(ctx, p, owner.ID); err != nil {
		return Object{}, err
	}
	return insertObject(ctx, db.pool, Object{Kind: KindPage, Title: title, Owner: owner.ID, Parent: parent})
}

// Blocks returns the blocks of a page the viewer may see, in their order.
func (db *DB) Blocks(ctx context.Context, pageID, viewer string) ([]Block, error) {
	if _, err := db.page(ctx, pageID, viewer); err != nil {
		return nil, err
	}
	rows, err := db.pool.Query(ctx, "SELECT id, body FROM blocks WHERE page = $1 ORDER BY position", pageID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Block, error) {
		var b Block
		err := row.Scan(&b.ID, &b.Body)
		return b, err
	})
}

// page returns an object only if it is a page the viewer may see.
func (db *DB) page(ctx context.Context, pageID, viewer string) (Object, error) {
	o, err := db.Object(ctx, pageID, viewer)
	if err == nil && o.Kind != KindPage {
		err = ErrNotFound
	}
	return o, err
}

// change runs a change of the blocks of a page in one transaction, with the page locked, after checking that the viewer
// sees the page and may change it.
func (db *DB) change(ctx context.Context, pageID, viewer string, do func(tx pgx.Tx) error) error {
	o, err := db.page(ctx, pageID, viewer)
	if err != nil {
		return err
	}
	if err := db.mayEdit(ctx, o, viewer); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 FOR UPDATE", pageID); err != nil {
			return err
		}
		if err := do(tx); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "UPDATE objects SET changed = now() WHERE id = $1", pageID)
		return err
	})
}

func checkBody(body string) error {
	if len([]rune(body)) > MaxBlock {
		return ErrBlock
	}
	return nil
}

// AddBlock puts a new block after the block named by after, or first if after is empty. The block has its ID at once.
func (db *DB) AddBlock(ctx context.Context, pageID, viewer, after, body string) (Block, error) {
	if err := checkBody(body); err != nil {
		return Block{}, err
	}
	var b Block
	err := db.change(ctx, pageID, viewer, func(tx pgx.Tx) error {
		position := 0
		if after != "" {
			if err := tx.QueryRow(ctx, "SELECT position + 1 FROM blocks WHERE page = $1 AND id = $2", pageID, after).Scan(&position); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrNotFound
				}
				return err
			}
		}
		var next int
		if err := tx.QueryRow(ctx, "UPDATE objects SET next_block = next_block + 1 WHERE id = $1 RETURNING next_block - 1", pageID).Scan(&next); err != nil {
			return err
		}
		b = Block{ID: "b" + strconv.Itoa(next), Body: body}
		if _, err := tx.Exec(ctx, "UPDATE blocks SET position = position + 1 WHERE page = $1 AND position >= $2", pageID, position); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO blocks (page, id, position, body) VALUES ($1, $2, $3, $4)", pageID, b.ID, position, body); err != nil {
			return err
		}
		if err := syncRefs(ctx, tx, pageID, b.ID, body); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "INSERT INTO block_versions (page, block, body, author) VALUES ($1, $2, $3, $4)", pageID, b.ID, body, viewer)
		return err
	})
	return b, err
}

// EditBlock gives a block new text and keeps the change as a version. The same text again makes no version.
func (db *DB) EditBlock(ctx context.Context, pageID, blockID, viewer, body string) (Block, error) {
	if err := checkBody(body); err != nil {
		return Block{}, err
	}
	err := db.change(ctx, pageID, viewer, func(tx pgx.Tx) error {
		var old string
		if err := tx.QueryRow(ctx, "SELECT body FROM blocks WHERE page = $1 AND id = $2", pageID, blockID).Scan(&old); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if old == body {
			return nil
		}
		if _, err := tx.Exec(ctx, "UPDATE blocks SET body = $3 WHERE page = $1 AND id = $2", pageID, blockID, body); err != nil {
			return err
		}
		if err := syncRefs(ctx, tx, pageID, blockID, body); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "INSERT INTO block_versions (page, block, body, author) VALUES ($1, $2, $3, $4)", pageID, blockID, body, viewer)
		return err
	})
	return Block{ID: blockID, Body: body}, err
}

// DeleteBlock takes a block out of its page. Its versions stay, and its ID is not given out again.
func (db *DB) DeleteBlock(ctx context.Context, pageID, blockID, viewer string) error {
	return db.change(ctx, pageID, viewer, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "DELETE FROM blocks WHERE page = $1 AND id = $2", pageID, blockID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		_, err = tx.Exec(ctx, "INSERT INTO block_versions (page, block, body, author) VALUES ($1, $2, NULL, $3)", pageID, blockID, viewer)
		return err
	})
}

// Versions returns the latest changes of a page the viewer may see, newest first.
func (db *DB) Versions(ctx context.Context, pageID, viewer string, limit int) ([]Version, error) {
	if _, err := db.page(ctx, pageID, viewer); err != nil {
		return nil, err
	}
	rows, err := db.pool.Query(ctx, `
		SELECT v.version, v.block, coalesce(v.body, ''), v.body IS NULL, coalesce(a.name, v.author), v.at
		FROM block_versions v LEFT JOIN accounts a ON a.id = v.author
		WHERE v.page = $1 ORDER BY v.version DESC LIMIT $2`, pageID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Version, error) {
		var v Version
		err := row.Scan(&v.Number, &v.Block, &v.Body, &v.Deleted, &v.Author, &v.At)
		return v, err
	})
}

// MayEdit tells whether the viewer may change an object, for showing the means to do so. Each change checks again.
func (db *DB) MayEdit(ctx context.Context, o Object, viewer string) bool {
	return db.mayEdit(ctx, o, viewer) == nil
}
