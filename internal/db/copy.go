// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"strings"

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

// fileCopyTitle marks a file as a copy before its extension, so it keeps its type: "plan (copy).pdf".
func fileCopyTitle(name string) string {
	base, ext := name, ""
	if i := strings.LastIndex(name, "."); i > 0 && len([]rune(name[i:])) <= 16 {
		base, ext = name[:i], name[i:]
	}
	return copyTitle(base) + ext
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
	if parent == "" && (o.Kind != KindCollection || o.Area == AreaFiles) {
		return Object{}, ErrPlace
	}
	// A copy at the top is a new space (B-3).
	if parent == "" {
		if err := maySpace(ctx, viewer); err != nil {
			return Object{}, err
		}
	}
	if parent != "" {
		p, err := db.Object(ctx, parent, viewer)
		if err != nil {
			return Object{}, err
		}
		if err := db.mayDo(ctx, p, viewer, ActCreate); err != nil {
			return Object{}, err
		}
		if IsDivider(p.Kind) || p.Area != o.Area || p.Kind == KindFile {
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
	// size is what the copies of files and attachments add to the store's count, checked against the quotas.
	var size int64
	var read func(o Object) (*node, error)
	read = func(o Object) (*node, error) {
		if count++; count > copyLimit {
			return nil, ErrTooMany
		}
		n := &node{o: o}
		var more int64
		if err := db.pool.QueryRow(ctx, `SELECT coalesce((SELECT size FROM file_versions WHERE file = $1 ORDER BY number DESC LIMIT 1), 0)
			+ coalesce((SELECT sum(size) FROM attachments WHERE page = $1), 0)`, o.ID).Scan(&more); err != nil {
			return nil, err
		}
		size += more
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
	// An agent publishes nothing by copying it either (B-5): every copy is checked, since one below the top keeps none
	// of the entries of its original (B-6).
	var ids []string
	var walk func(n *node)
	walk = func(n *node) {
		ids = append(ids, n.o.ID)
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(tree)
	if err := db.agentNoWider(ctx, ids, parent, false); err != nil {
		return Object{}, err
	}
	var top Object
	root := ""
	if o.Area == AreaFiles {
		if root, err = rootOf(ctx, db.pool, parent); err != nil {
			return Object{}, err
		}
	}
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if size > 0 {
			if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockQuota); err != nil {
				return err
			}
			if err := db.checkRoom(ctx, tx, root, size); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared($1)", lockBlobs); err != nil {
				return err
			}
		}
		position, err := place(ctx, tx, parent, after)
		if err != nil {
			return err
		}
		var write func(n *node, parent, title string) (Object, error)
		write = func(n *node, parent, title string) (Object, error) {
			// A copy at the top is a new collection of the viewer, theirs alone until they share it (ADR-29, 2026-11-19);
			// below it, each copy keeps the role its original grants the whole tenant.
			access := n.o.Access
			if parent == "" {
				access = ""
			}
			made, err := insertObject(ctx, tx, Object{Kind: n.o.Kind, Title: title, Description: n.o.Description, Tags: n.o.Tags, Owner: viewer,
				Parent: parent, Access: access, Private: n.o.Private && parent == "", Width: n.o.Width, Contents: n.o.Contents, PageKind: n.o.PageKind})
			if err != nil {
				return Object{}, err
			}
			if _, err := tx.Exec(ctx, `UPDATE objects SET (next_block, next_attachment) = (SELECT next_block, next_attachment FROM objects WHERE id = $2)
				WHERE id = $1`, made.ID, n.o.ID); err != nil {
				return Object{}, err
			}
			// Attachments go with their page, and its blocks show the copies (ADR-28).
			if _, err := tx.Exec(ctx, `INSERT INTO blocks (page, id, position, body, align)
				SELECT $1, id, position, replace(body, $3, $4), align FROM blocks WHERE page = $2`,
				made.ID, n.o.ID, AttachmentPath(n.o.ID, ""), AttachmentPath(made.ID, "")); err != nil {
				return Object{}, err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO attachments (page, id, blob, size, name, content_type, author)
				SELECT $1, id, blob, size, name, content_type, $3 FROM attachments WHERE page = $2`, made.ID, n.o.ID, viewer); err != nil {
				return Object{}, err
			}
			// A copy of a file starts with the content it has now; its history stays with the original.
			if _, err := tx.Exec(ctx, `INSERT INTO file_versions (file, number, blob, size, name, content_type, author)
				SELECT $1, 1, blob, size, $3, content_type, $4 FROM file_versions WHERE file = $2 ORDER BY number DESC LIMIT 1`,
				made.ID, n.o.ID, title, viewer); err != nil {
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
		title := copyTitle(o.Title)
		if o.Kind == KindFile {
			title = fileCopyTitle(o.Title)
		}
		if top, err = write(tree, parent, title); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE objects SET position = $2 WHERE id = $1", top.ID, position); err != nil {
			return err
		}
		top.Position = position
		return withinStorage(ctx, tx)
	})
	return top, err
}
