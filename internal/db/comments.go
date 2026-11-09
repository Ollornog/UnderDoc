// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Comments belong to a page, to the whole of it or to one of its blocks through the block's ID (ADR-18). Who may do what
// follows ADR-20: whoever sees the page reads its comments; the owner, and all others while the page lets them comment or
// edit, add comments and replies; each author changes and deletes their own; a thread is resolved by its author or by
// whoever may edit the page.

// MaxComment is the longest a comment may be, in characters.
const MaxComment = 10000

// Errors of comments a caller can tell apart.
var (
	ErrComment = errors.New("a comment has 1 to 10000 characters")
	// ErrHasReplies is the deletion of a comment that others have answered: their replies stay, so it stays too.
	ErrHasReplies = errors.New("others have replied to this comment; resolve it instead")
)

// Comment is one comment, or one reply to a comment.
type Comment struct {
	ID int64
	// Page and Block say where it belongs; Block is empty for the whole page.
	Page, Block string
	// Gone tells that its block is no longer in the page. The comment stays and says so.
	Gone bool
	// Parent is the comment it answers, 0 for the first comment of a thread.
	Parent int64
	// Author is the account, AuthorName its name as people see it.
	Author, AuthorName string
	// Body is plain text.
	Body    string
	Created time.Time
	// Edited is when the author last changed it, zero if never.
	Edited time.Time
	// Resolved is when its thread was resolved, zero while it is open; ResolvedBy names who did it.
	Resolved   time.Time
	ResolvedBy string
}

const commentColumns = `c.id, c.page, c.block,
	c.block <> '' AND NOT EXISTS (SELECT 1 FROM blocks b WHERE b.page = c.page AND b.id = c.block),
	coalesce(c.parent, 0), c.author, coalesce(a.name, c.author), c.body, c.created, c.edited,
	c.resolved_at, coalesce((SELECT r.name FROM accounts r WHERE r.id = c.resolved_by), coalesce(c.resolved_by, ''))`

const commentFrom = ` FROM comments c LEFT JOIN accounts a ON a.id = c.author`

func scanComment(row pgx.Row) (Comment, error) {
	var c Comment
	var edited, resolved *time.Time
	err := row.Scan(&c.ID, &c.Page, &c.Block, &c.Gone, &c.Parent, &c.Author, &c.AuthorName, &c.Body, &c.Created, &edited, &resolved, &c.ResolvedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return Comment{}, ErrNotFound
	}
	if edited != nil {
		c.Edited = *edited
	}
	if resolved != nil {
		c.Resolved = *resolved
	}
	return c, err
}

// mayComment tells whether the viewer may add comments to an object: its owner always, anyone else while others may
// comment or edit (ADR-20). Read only means no comment.
func (db *DB) mayComment(ctx context.Context, o Object, viewer string) error {
	if o.Owner == viewer {
		return nil
	}
	access, err := db.Access(ctx, o.ID)
	if err != nil {
		return err
	}
	if access != "edit" && access != "comment" {
		return ErrNotAllowed
	}
	return nil
}

// MayComment tells whether the viewer may add comments to an object, for showing the means to do so. Each change checks
// again.
func (db *DB) MayComment(ctx context.Context, o Object, viewer string) bool {
	return db.mayComment(ctx, o, viewer) == nil
}

func checkComment(body string) (string, error) {
	body = strings.TrimSpace(body)
	if n := len([]rune(body)); n < 1 || n > MaxComment {
		return "", ErrComment
	}
	return body, nil
}

// Comments returns the comments of a page the viewer sees, each thread in the order it began, its replies right after
// it. A page in the trash, or a private page of someone else, has none to show: it is not found.
func (db *DB) Comments(ctx context.Context, pageID, viewer string) ([]Comment, error) {
	if _, err := db.page(ctx, pageID, viewer); err != nil {
		return nil, err
	}
	rows, err := db.pool.Query(ctx, `SELECT `+commentColumns+commentFrom+` WHERE c.page = $1
		ORDER BY coalesce(c.parent, c.id), c.parent IS NOT NULL, c.id`, pageID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Comment, error) { return scanComment(row) })
}

// OpenComments counts the comments of a page the viewer sees that belong to a thread not resolved, by block; the whole
// page counts under "". Comments on a block that is gone count under its ID too.
func (db *DB) OpenComments(ctx context.Context, pageID, viewer string) (map[string]int, error) {
	if _, err := db.page(ctx, pageID, viewer); err != nil {
		return nil, err
	}
	rows, err := db.pool.Query(ctx, `SELECT c.block, count(*) FROM comments c LEFT JOIN comments t ON t.id = c.parent
		WHERE c.page = $1 AND coalesce(t.resolved_at, c.resolved_at) IS NULL GROUP BY c.block`, pageID)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	defer rows.Close()
	for rows.Next() {
		var block string
		var n int
		if err := rows.Scan(&block, &n); err != nil {
			return nil, err
		}
		out[block] = n
	}
	return out, rows.Err()
}

// AddComment puts a comment on a page, on the block it names or on the whole page if block is empty. With parent set it
// is a reply: it lands on the block of the comment it answers, and replies go one level deep. A comment needs a block
// that is in the page now; a reply may still answer a comment on a block that is gone.
func (db *DB) AddComment(ctx context.Context, pageID, viewer, block, body string, parent int64) (Comment, error) {
	body, err := checkComment(body)
	if err != nil {
		return Comment{}, err
	}
	o, err := db.page(ctx, pageID, viewer)
	if err != nil {
		return Comment{}, err
	}
	if err := db.mayComment(ctx, o, viewer); err != nil {
		return Comment{}, err
	}
	if parent != 0 {
		var grand *int64
		if err := db.pool.QueryRow(ctx, "SELECT block, parent FROM comments WHERE id = $1 AND page = $2", parent, pageID).Scan(&block, &grand); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Comment{}, ErrNotFound
			}
			return Comment{}, err
		}
		if grand != nil {
			return Comment{}, ErrPlace
		}
	} else if block != "" {
		var found bool
		if err := db.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM blocks WHERE page = $1 AND id = $2)", pageID, block).Scan(&found); err != nil {
			return Comment{}, err
		}
		if !found {
			return Comment{}, ErrNotFound
		}
	}
	var id int64
	if err := db.pool.QueryRow(ctx, "INSERT INTO comments (page, block, parent, author, body) VALUES ($1, $2, nullif($3, 0), $4, $5) RETURNING id",
		pageID, block, parent, viewer, body).Scan(&id); err != nil {
		return Comment{}, err
	}
	return db.comment(ctx, id)
}

func (db *DB) comment(ctx context.Context, id int64) (Comment, error) {
	return scanComment(db.pool.QueryRow(ctx, `SELECT `+commentColumns+commentFrom+` WHERE c.id = $1`, id))
}

// ownComment finds a comment on a page the viewer sees, and the page; a comment on a page the viewer does not see is not
// found, as the page itself.
func (db *DB) ownComment(ctx context.Context, id int64, viewer string) (Comment, Object, error) {
	c, err := db.comment(ctx, id)
	if err != nil {
		return Comment{}, Object{}, err
	}
	o, err := db.page(ctx, c.Page, viewer)
	if err != nil {
		return Comment{}, Object{}, err
	}
	return c, o, nil
}

// EditComment gives the viewer's own comment new text. Only its author may, and only while they may comment on the page.
func (db *DB) EditComment(ctx context.Context, id int64, viewer, body string) (Comment, error) {
	body, err := checkComment(body)
	if err != nil {
		return Comment{}, err
	}
	c, o, err := db.ownComment(ctx, id, viewer)
	if err != nil {
		return Comment{}, err
	}
	if c.Author != viewer {
		return Comment{}, ErrNotAllowed
	}
	if err := db.mayComment(ctx, o, viewer); err != nil {
		return Comment{}, err
	}
	if c.Body == body {
		return c, nil
	}
	if _, err := db.pool.Exec(ctx, "UPDATE comments SET body = $2, edited = now() WHERE id = $1", id, body); err != nil {
		return Comment{}, err
	}
	return db.comment(ctx, id)
}

// DeleteComment takes the viewer's own comment away, with their own replies to it. A comment others have replied to
// stays, so their replies do not lose what they answer: it can be resolved instead.
func (db *DB) DeleteComment(ctx context.Context, id int64, viewer string) error {
	c, o, err := db.ownComment(ctx, id, viewer)
	if err != nil {
		return err
	}
	if c.Author != viewer {
		return ErrNotAllowed
	}
	if err := db.mayComment(ctx, o, viewer); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		var others bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM comments WHERE parent = $1 AND author <> $2)", id, viewer).Scan(&others); err != nil {
			return err
		}
		if others {
			return ErrHasReplies
		}
		_, err := tx.Exec(ctx, "DELETE FROM comments WHERE id = $1", id)
		return err
	})
}

// ResolveComment resolves a thread, or opens it again. Its author may, and whoever may edit the page. A reply is
// resolved with its thread, through the first comment.
func (db *DB) ResolveComment(ctx context.Context, id int64, viewer string, resolved bool) (Comment, error) {
	c, o, err := db.ownComment(ctx, id, viewer)
	if err != nil {
		return Comment{}, err
	}
	if c.Parent != 0 {
		return Comment{}, ErrPlace
	}
	if err := db.mayComment(ctx, o, viewer); err != nil {
		return Comment{}, err
	}
	if c.Author != viewer {
		if err := db.mayEdit(ctx, o, viewer); err != nil {
			return Comment{}, err
		}
	}
	if resolved {
		_, err = db.pool.Exec(ctx, "UPDATE comments SET resolved_at = coalesce(resolved_at, now()), resolved_by = coalesce(resolved_by, $2) WHERE id = $1", id, viewer)
	} else {
		_, err = db.pool.Exec(ctx, "UPDATE comments SET resolved_at = NULL, resolved_by = NULL WHERE id = $1", id)
	}
	if err != nil {
		return Comment{}, err
	}
	return db.comment(ctx, id)
}

// MayResolve tells whether the viewer may resolve a comment, for showing the means to do so: the first comment of a
// thread, by its author while they may comment, or by whoever may edit the page. Each change checks again.
func MayResolve(c Comment, viewer string, mayComment, mayEdit bool) bool {
	return c.Parent == 0 && (mayEdit || (mayComment && c.Author == viewer))
}
