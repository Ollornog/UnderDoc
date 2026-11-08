// SPDX-License-Identifier: MIT

// Package db keeps what lives in PostgreSQL (ADR-19): accounts, and objects with their place, owner and access. Every
// query that returns objects takes the viewer, so what a viewer may not see never leaves this package (ADR-20).
package db

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/id"
)

//go:embed migrations/*.sql
var migrations embed.FS

// DB is the connection pool with the schema brought up to date.
type DB struct {
	pool *pgxpool.Pool
}

// Errors a caller can tell apart.
var (
	ErrNotFound   = errors.New("not found")
	ErrTitle      = errors.New("a title has 1 to 200 characters")
	ErrNotAllowed = errors.New("others may not change this")
)

// Open connects and brings the schema up to date.
func Open(ctx context.Context, url string) (*DB, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	db := &DB{pool: pool}
	if err := db.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return db, nil
}

// Close ends all connections.
func (db *DB) Close() { db.pool.Close() }

// Healthy reports whether the database answers.
func (db *DB) Healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return db.pool.Ping(ctx) == nil
}

// migrate applies every migration not applied yet, in the order of their numbers, each in its own transaction. An
// advisory lock keeps two starting instances from doing it at once.
func (db *DB) migrate(ctx context.Context) error {
	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	const lock = 7272822 // any number, the same for every instance
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", lock); err != nil {
		return err
	}
	defer func() { _, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", lock) }()
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version integer PRIMARY KEY, applied timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	names, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	slices.Sort(names)
	for _, name := range names {
		version, err := strconv.Atoi(strings.SplitN(strings.TrimPrefix(name, "migrations/"), "_", 2)[0])
		if err != nil {
			return fmt.Errorf("%s: a migration starts with its number", name)
		}
		var done bool
		if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", version).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		sql, err := migrations.ReadFile(name)
		if err != nil {
			return err
		}
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			_, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// --- Accounts

// MyNotes is the title of the private collection every account starts with (ADR-20).
const MyNotes = "My notes"

// SeeAccount records a user from the gate: new on the first sign-in, with a private collection, and up to date after.
func (db *DB) SeeAccount(ctx context.Context, user account.User) error {
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		var inserted bool
		err := tx.QueryRow(ctx, `
			INSERT INTO accounts (id, login, name, email, groups) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (id) DO UPDATE SET login = EXCLUDED.login, name = EXCLUDED.name, email = EXCLUDED.email,
				groups = EXCLUDED.groups, last_seen = now()
			RETURNING (xmax = 0)`, user.ID, user.Login, user.Name, user.Email, nonNil(user.Groups)).Scan(&inserted)
		if err != nil || !inserted {
			return err
		}
		_, err = insertObject(ctx, tx, Object{Kind: KindCollection, Title: MyNotes, Owner: user.ID, Private: true})
		return err
	})
}

func nonNil(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

// --- Objects

// Kinds of objects.
const (
	KindCollection = "collection"
	KindFolder     = "folder"
	KindPage       = "page"
)

// Object is a collection, folder or page, without its content.
type Object struct {
	ID          string
	Kind        string
	Title       string
	Description string
	Owner       string
	Parent      string
	Position    int
	// Access is what all others may do, set here; empty means inherited.
	Access  string
	Private bool
	Created time.Time
	Changed time.Time
}

const columns = `id, kind, title, description, owner, coalesce(parent, ''), position, coalesce(access, ''), private, created, changed`

func scan(row pgx.Row) (Object, error) {
	var o Object
	err := row.Scan(&o.ID, &o.Kind, &o.Title, &o.Description, &o.Owner, &o.Parent, &o.Position, &o.Access, &o.Private, &o.Created, &o.Changed)
	return o, err
}

type execer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// insertObject gives the object a new ID and puts it last at its place. A clash of IDs is drawn again.
func insertObject(ctx context.Context, tx execer, o Object) (Object, error) {
	o.Title = strings.TrimSpace(o.Title)
	if n := len([]rune(o.Title)); n < 1 || n > 200 {
		return Object{}, ErrTitle
	}
	for attempt := 0; ; attempt++ {
		o.ID = id.New()
		created, err := scan(tx.QueryRow(ctx, `
			INSERT INTO objects (id, kind, title, description, owner, parent, position, access, private)
			VALUES ($1, $2, $3, $4, $5, nullif($6, ''),
				(SELECT coalesce(max(position) + 1, 0) FROM objects WHERE parent IS NOT DISTINCT FROM nullif($6, '')),
				nullif($7, ''), $8)
			RETURNING `+columns, o.ID, o.Kind, o.Title, o.Description, o.Owner, o.Parent, o.Access, o.Private))
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == "objects_pkey" && attempt < 5 {
			continue
		}
		return created, err
	}
}

// CreateCollection makes a collection at the top, or inside parent if that is set and visible to the owner.
func (db *DB) CreateCollection(ctx context.Context, owner account.User, title, parent string) (Object, error) {
	if parent != "" {
		if _, err := db.Object(ctx, parent, owner.ID); err != nil {
			return Object{}, err
		}
	}
	return insertObject(ctx, db.pool, Object{Kind: KindCollection, Title: title, Owner: owner.ID, Parent: parent})
}

// visible is a condition on o: the object, or the top it belongs to, is not private, or it is the viewer's own.
const visible = `NOT EXISTS (
	WITH RECURSIVE up AS (
		SELECT id, parent, private, owner FROM objects WHERE id = o.id
		UNION ALL SELECT p.id, p.parent, p.private, p.owner FROM objects p JOIN up ON p.id = up.parent
	)
	SELECT 1 FROM up WHERE up.private AND up.owner <> $1)`

// Collections returns the collections at the top that the viewer may see, in their order.
func (db *DB) Collections(ctx context.Context, viewer string) ([]Object, error) {
	return db.list(ctx, `SELECT `+columns+` FROM objects o WHERE parent IS NULL AND kind = 'collection' AND `+visible+`
		ORDER BY private DESC, position, created`, viewer)
}

// Children returns what lies directly inside an object the viewer may see, in its order.
func (db *DB) Children(ctx context.Context, parent, viewer string) ([]Object, error) {
	return db.list(ctx, `SELECT `+columns+` FROM objects o WHERE parent = $2 AND `+visible+` ORDER BY position, created`, viewer, parent)
}

// Object returns one object, or ErrNotFound if it does not exist or the viewer may not see it. The two look alike on
// purpose: an ID must not tell whether a private object exists.
func (db *DB) Object(ctx context.Context, objectID, viewer string) (Object, error) {
	o, err := scan(db.pool.QueryRow(ctx, `SELECT `+columns+` FROM objects o WHERE id = $2 AND `+visible, viewer, objectID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Object{}, ErrNotFound
	}
	return o, err
}

func (db *DB) list(ctx context.Context, sql string, args ...any) ([]Object, error) {
	rows, err := db.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Object, error) { return scan(row) })
}

// AccountName returns the display name of an account, or the ID if it is unknown.
func (db *DB) AccountName(ctx context.Context, accountID string) string {
	var name string
	if err := db.pool.QueryRow(ctx, "SELECT name FROM accounts WHERE id = $1", accountID).Scan(&name); err != nil {
		return accountID
	}
	return name
}

// Access returns what all others may do with an object: the first level set on it or above it, else edit (ADR-20).
func (db *DB) Access(ctx context.Context, objectID string) (string, error) {
	var access string
	err := db.pool.QueryRow(ctx, `
		WITH RECURSIVE up AS (
			SELECT id, parent, access, 0 AS depth FROM objects WHERE id = $1
			UNION ALL SELECT p.id, p.parent, p.access, up.depth + 1 FROM objects p JOIN up ON p.id = up.parent
		)
		SELECT coalesce((SELECT access FROM up WHERE access IS NOT NULL ORDER BY depth LIMIT 1), 'edit')`, objectID).Scan(&access)
	return access, err
}

// Top returns the collection at the top that an object belongs to.
func (db *DB) Top(ctx context.Context, objectID, viewer string) (Object, error) {
	o, err := db.Object(ctx, objectID, viewer)
	for err == nil && o.Parent != "" {
		o, err = db.Object(ctx, o.Parent, viewer)
	}
	return o, err
}

// Rename gives an object a new title. The viewer must see it and may change it: the owner, or anyone while others may
// edit it (ADR-20).
func (db *DB) Rename(ctx context.Context, objectID, viewer, title string) (Object, error) {
	title = strings.TrimSpace(title)
	if n := len([]rune(title)); n < 1 || n > 200 {
		return Object{}, ErrTitle
	}
	o, err := db.Object(ctx, objectID, viewer)
	if err != nil {
		return Object{}, err
	}
	if o.Owner != viewer {
		access, err := db.Access(ctx, objectID)
		if err != nil {
			return Object{}, err
		}
		if access != "edit" {
			return Object{}, ErrNotAllowed
		}
	}
	return scan(db.pool.QueryRow(ctx, `UPDATE objects SET title = $2, changed = now() WHERE id = $1 RETURNING `+columns, objectID, title))
}
