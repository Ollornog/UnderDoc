// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
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
	LogKept         = "log kept"
	// LogRefused is an action a deny refused, with the deciding entry (ADR-36).
	LogRefused = "refused"
	// LogAgents is a change of what a person lets their agents do.
	LogAgents = "agents"
	// LogChecked and LogTakenBack are a check of a change by agents and its taking back (ADR-33), each kept.
	LogChecked   = "checked"
	LogTakenBack = "taken back"
	// LogPurge is deleting for good from the trash (B-3), each kept.
	LogPurge = "deleted for good"
)

// hourly are the actions kept at most once per account, object and hour.
var hourly = map[string]bool{LogOpen: true, LogChange: true, LogLinkOpened: true, LogRefused: true}

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
		  AND ($5 OR l.actor = $1 OR (l.object <> '' AND object_may(l.object, $1, 'manage')))
		  AND (l.object = '' OR o.id IS NULL OR NOT object_private(l.object) OR object_may(l.object, $1, 'read'))
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

// Retention of the log (ADR-29, 2026-11-19): each tenant keeps an entry LogKeepDays days unless its administrators set
// between LogKeepMin and LogKeepMax.
const (
	LogKeepDays = 365
	LogKeepMin  = 30
	LogKeepMax  = 3650
)

// ErrLogKeep is a retention of the log outside LogKeepMin to LogKeepMax days.
var ErrLogKeep = errors.New("the log is kept for 30 to 3650 days")

// LogKeep returns how many days the tenant of ctx keeps an entry of the log.
func (db *DB) LogKeep(ctx context.Context) (int, error) {
	days := LogKeepDays
	err := db.pool.QueryRow(ctx, "SELECT keep_days FROM log_settings").Scan(&days)
	if errors.Is(err, pgx.ErrNoRows) {
		return LogKeepDays, nil
	}
	return days, err
}

// SetLogKeep sets how many days the tenant of ctx keeps an entry of the log. Only administrators do; the caller checks
// and logs it.
func (db *DB) SetLogKeep(ctx context.Context, days int) error {
	if days < LogKeepMin || days > LogKeepMax {
		return ErrLogKeep
	}
	_, err := db.pool.Exec(ctx, `INSERT INTO log_settings (keep_days) VALUES ($1)
		ON CONFLICT (tenant) DO UPDATE SET keep_days = EXCLUDED.keep_days, changed = now()`, days)
	return err
}

// ExpireLog deletes the entries of the log of the tenant of ctx that are older than it keeps them, as of now, and
// returns how many went.
func (db *DB) ExpireLog(ctx context.Context, now time.Time) (int64, error) {
	days, err := db.LogKeep(ctx)
	if err != nil {
		return 0, err
	}
	tag, err := db.pool.Exec(ctx, "DELETE FROM audit_log WHERE at < $1", now.AddDate(0, 0, -days))
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Private tells whether an object lies in something private.
func (db *DB) Private(ctx context.Context, objectID string) (bool, error) {
	return isPrivate(ctx, db.pool, objectID)
}
