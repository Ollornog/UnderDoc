// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// MaxCopy is how many objects one copy makes at most, so copying a large tree stays a deliberate act.
const MaxCopy = 500

// copyLimit is MaxCopy, lowered by a test.
var copyLimit = MaxCopy

// ErrTooMany is a copy of more than MaxCopy objects.
var ErrTooMany = errors.New("too much to copy at once")

// copyTitle is the title of the top copy: the original's, marked as a copy, within the length a title may have.
func copyTitle(title string) string {
	const mark = " (copy)"
	r := []rune(title)
	if len(r)+len([]rune(mark)) > 200 {
		r = r[:200-len([]rune(mark))]
	}
	return string(r) + mark
}

// Copy duplicates an object with everything below it that the viewer sees into parent, after the entry named by after
// or first. The viewer must be allowed to change the parent (ADR-20); the copies belong to the viewer, pages keep their
// blocks with their IDs, and the top copy is marked as a copy in its title. Only a collection stands at the top, and
// nothing is copied into itself.
func (db *DB) Copy(ctx context.Context, objectID, viewer, parent, after string) (Object, error) {
	o, err := db.Object(ctx, objectID, viewer)
	if err != nil {
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
		for x := p; ; {
			if x.ID == objectID {
				return Object{}, ErrPlace
			}
			if x.Parent == "" {
				break
			}
			if x, err = db.Object(ctx, x.Parent, viewer); err != nil {
				return Object{}, err
			}
		}
	}
	// What is copied is read first, as the viewer sees it, so the copy is finite and leaves out what they may not see.
	type node struct {
		o    Object
		kids []*node
	}
	count := 0
	var read func(o Object) (*node, error)
	read = func(o Object) (*node, error) {
		if count++; count > copyLimit {
			return nil, ErrTooMany
		}
		n := &node{o: o}
		kids, err := db.Children(ctx, o.ID, viewer)
		if err != nil {
			return nil, err
		}
		for _, k := range kids {
			c, err := read(k)
			if err != nil {
				return nil, err
			}
			n.kids = append(n.kids, c)
		}
		return n, nil
	}
	tree, err := read(o)
	if err != nil {
		return Object{}, err
	}
	var top Object
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		position, err := place(ctx, tx, parent, after)
		if err != nil {
			return err
		}
		var write func(n *node, parent, title string) (Object, error)
		write = func(n *node, parent, title string) (Object, error) {
			made, err := insertObject(ctx, tx, Object{Kind: n.o.Kind, Title: title, Description: n.o.Description, Owner: viewer,
				Parent: parent, Access: n.o.Access, Private: n.o.Private && parent == ""})
			if err != nil {
				return Object{}, err
			}
			if _, err := tx.Exec(ctx, "UPDATE objects SET next_block = (SELECT next_block FROM objects WHERE id = $2) WHERE id = $1", made.ID, n.o.ID); err != nil {
				return Object{}, err
			}
			if _, err := tx.Exec(ctx, "INSERT INTO blocks (page, id, position, body) SELECT $1, id, position, body FROM blocks WHERE page = $2", made.ID, n.o.ID); err != nil {
				return Object{}, err
			}
			if _, err := tx.Exec(ctx, "INSERT INTO block_versions (page, block, body, author) SELECT page, id, body, $2 FROM blocks WHERE page = $1 ORDER BY position", made.ID, viewer); err != nil {
				return Object{}, err
			}
			if _, err := tx.Exec(ctx, "INSERT INTO refs (page, block, target, target_block) SELECT $1, block, target, target_block FROM refs WHERE page = $2", made.ID, n.o.ID); err != nil {
				return Object{}, err
			}
			for _, k := range n.kids {
				if _, err := write(k, made.ID, k.o.Title); err != nil {
					return Object{}, err
				}
			}
			return made, nil
		}
		if top, err = write(tree, parent, copyTitle(o.Title)); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "UPDATE objects SET position = $2 WHERE id = $1", top.ID, position)
		top.Position = position
		return err
	})
	return top, err
}
