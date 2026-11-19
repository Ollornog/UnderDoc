// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Actions of the log (ADR-29). Opening and changing an object are kept once per account, object and hour; everything
// else each time.
const (
	LogOpen         = "open"
	LogChange       = "change"
	LogGrant        = "grant"
	LogInherit      = "inherit"
	LogOwner        = "owner"
	LogGroup        = "group"
	LogMember       = "member"
	LogAdmin        = "admin"
	LogLinkMade     = "link made"
	LogLinkRevoked  = "link revoked"
	LogLinkRenewed  = "link renewed"
	LogLinkOpened   = "link opened"
	LogLinkUsed     = "link used"
	LogLinkPassword = "link wrong password"
	LogLinkUpload   = "link upload"
)

// hourly are the actions kept at most once per account, object and hour.
var hourly = map[string]bool{LogOpen: true, LogChange: true, LogLinkOpened: true}

// Log writes an entry. Opening and changing are kept once per account, object and hour, so reading a page all day is one
// line an hour, not a thousand.
func (db *DB) Log(ctx context.Context, actor, action, objectID, detail string) error {
	if len([]rune(detail)) > 1000 {
		detail = string([]rune(detail)[:1000])
	}
	if hourly[action] {
		_, err := db.pool.Exec(ctx, `INSERT INTO audit_log (actor, action, object, detail)
			SELECT $1, $2, $3, $4 WHERE NOT EXISTS (SELECT 1 FROM audit_log WHERE actor = $1 AND action = $2 AND object = $3
				AND at > now() - interval '1 hour')`, actor, action, objectID, detail)
		return err
	}
	_, err := db.pool.Exec(ctx, "INSERT INTO audit_log (actor, action, object, detail) VALUES ($1, $2, $3, $4)", actor, action, objectID, detail)
	return err
}

// Entry is one line of the log, with the names of who and what where they are known.
type Entry struct {
	ID                  int64
	At                  time.Time
	Actor, ActorName    string
	Action              string
	Object, ObjectTitle string
	Detail              string
}

// LogFilter narrows the log: to one object, to one actor, and to before an entry, for the next page.
type LogFilter struct {
	Object, Actor string
	Before        int64
	Limit         int
}

// LogEntries returns the newest entries first. Administrators read the whole log; everyone else what they did themselves
// and what happened to the objects they manage (ADR-29).
func (db *DB) LogEntries(ctx context.Context, actor Actor, f LogFilter) ([]Entry, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	rows, err := db.pool.Query(ctx, `SELECT l.id, l.at, l.actor, coalesce(a.name, l.actor), l.action, l.object, coalesce(o.title, ''), l.detail
		FROM audit_log l LEFT JOIN accounts a ON a.id = l.actor LEFT JOIN objects o ON o.id = l.object
		WHERE ($2 = '' OR l.object = $2) AND ($3 = '' OR l.actor = $3) AND ($4 = 0 OR l.id < $4)
		  AND ($5 OR l.actor = $1 OR (l.object <> '' AND object_role(l.object, $1) >= 4))
		  AND (l.object = '' OR o.id IS NULL OR NOT object_private(l.object) OR object_role(l.object, $1) > 0)
		ORDER BY l.id DESC LIMIT $6`, actor.ID, f.Object, f.Actor, f.Before, actor.Admin, f.Limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Entry, error) {
		var e Entry
		err := row.Scan(&e.ID, &e.At, &e.Actor, &e.ActorName, &e.Action, &e.Object, &e.ObjectTitle, &e.Detail)
		return e, err
	})
}

// Private tells whether an object lies in something private.
func (db *DB) Private(ctx context.Context, objectID string) (bool, error) {
	return isPrivate(ctx, db.pool, objectID)
}
