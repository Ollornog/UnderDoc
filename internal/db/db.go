// SPDX-License-Identifier: MIT

// Package db keeps what lives in PostgreSQL (ADR-19): accounts, and objects with their place, owner and access. Every
// query that returns objects takes the viewer, so what a viewer may not see never leaves this package (ADR-29). Every
// query runs as the tenant its context carries (package tenant), and row-level security keeps it to the rows of that
// tenant (ADR-23): a context without a tenant sees nothing.
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

	"github.com/Ollornog/EightPerDay/core/pgstore"

	"github.com/Ollornog/UnderDoc/internal/sheet"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/id"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

//go:embed migrations/*.sql
var migrations embed.FS

// DB is the connection pool with the schema brought up to date.
type DB struct {
	pool *pgxpool.Pool
	// time is EightPerDay's store on the pool (TimeStore).
	time *pgstore.Store
	// limits are the sizes files may have (SetLimits); set once before the first request.
	limits Limits
	// samba names the tenants with a Samba companion, guard holds the web back from files open over SMB (ADR-30); both
	// set once before the first request.
	samba map[string]bool
	guard func(ctx context.Context, objectID string) error
	// sheets reads spreadsheets as tables for the search (SetSheets, ADR-31); set once before the indexers start.
	sheets SheetReader
	// approver confirms deleting for good with an approval of the gate (SetApprover, T-33); nil asks for none.
	approver Approver
}

// SheetReader reads the workbook of a blob of a tenant (sheet.Cache).
type SheetReader interface {
	Workbook(ctx context.Context, tenant, sha, contentType, name string) (*sheet.Workbook, error)
}

// SetSheets lets the search find spreadsheets by their cells (ADR-31); without it they are found by their text, as other
// files are. Set once before the indexers start.
func (db *DB) SetSheets(r SheetReader) { db.sheets = r }

// Errors a caller can tell apart.
var (
	ErrNotFound   = errors.New("not found")
	ErrTitle      = errors.New("a title has 1 to 200 characters")
	ErrNotAllowed = errors.New("others may not change this")
)

// AppRole is the role the application works as when it connects as a superuser: row-level security does not hold for a
// superuser, so every connection of the pool steps down to this role, which has no way around the rules (ADR-23).
const AppRole = "underdoc_app"

// ErrBypass is a connection that row-level security does not hold for, so tenants would not be kept apart.
var ErrBypass = errors.New("the database role passes row-level security (superuser or BYPASSRLS) and cannot step down to " + AppRole)

// Open connects and brings the schema up to date. The schema is migrated as the role of the URL, which owns the tables;
// the pool then works as a role that row-level security holds for, and Open refuses to go on if it does not.
func Open(ctx context.Context, url string) (*DB, error) {
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	stepDown, err := prepare(ctx, config.ConnConfig)
	if err != nil {
		return nil, err
	}
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		if !stepDown {
			return nil
		}
		_, err := conn.Exec(ctx, "SET ROLE "+AppRole)
		return err
	}
	// Every connection taken from the pool is set to the tenant of the context it is taken for, and to none without one.
	// It also carries whether the request is one of an agent (ADR-36), which the check of rights reads (request_agent).
	config.PrepareConn = func(ctx context.Context, conn *pgx.Conn) (bool, error) {
		t, _ := tenant.From(ctx)
		if _, err := conn.Exec(ctx, "SELECT set_config('underdoc.tenant', $1, false), set_config('underdoc.agent', $2, false)",
			t, agentSetting(ctx)); err != nil {
			return false, err
		}
		return true, nil
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}
	db := &DB{pool: pool}
	if db.time, err = pgstore.New(pool, TimeOptions); err != nil {
		pool.Close()
		return nil, err
	}
	if err := db.checkRules(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return db, nil
}

// prepare migrates the schema on a connection of its own and, if the role passes row-level security, sets up the role
// to step down to. It reports whether the pool must step down.
func prepare(ctx context.Context, config *pgx.ConnConfig) (bool, error) {
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return false, err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if err := migrate(ctx, conn, 0); err != nil {
		return false, err
	}
	var super, bypass bool
	if err := conn.QueryRow(ctx, "SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user").Scan(&super, &bypass); err != nil {
		return false, err
	}
	if !super && !bypass {
		// The owner of the tables: FORCE ROW LEVEL SECURITY holds the rules for it.
		return false, nil
	}
	if !super {
		return false, ErrBypass
	}
	_, err = conn.Exec(ctx, `DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = '`+AppRole+`') THEN
			CREATE ROLE `+AppRole+` NOLOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE NOINHERIT;
		END IF;
	END $$;
	GRANT USAGE ON SCHEMA public TO `+AppRole+`;
	GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO `+AppRole+`;
	GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO `+AppRole)
	return true, err
}

// checkRules makes sure that row-level security holds for the connections of the pool, so a mistake in the setup of
// the database stops the start instead of mixing tenants.
func (db *DB) checkRules(ctx context.Context) error {
	var super, bypass bool
	err := db.pool.QueryRow(ctx, "SELECT current_setting('is_superuser') = 'on', rolbypassrls FROM pg_roles WHERE rolname = current_user").Scan(&super, &bypass)
	if err != nil {
		return err
	}
	if super || bypass {
		return ErrBypass
	}
	return nil
}

// Close ends all connections.
func (db *DB) Close() { db.pool.Close() }

// Healthy reports whether the database answers.
func (db *DB) Healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return db.pool.Ping(ctx) == nil
}

// migrate applies every migration not applied yet, in the order of their numbers, each in its own transaction, up to
// the number upTo, or all with 0. An advisory lock keeps two starting instances from doing it at once.
func migrate(ctx context.Context, conn *pgx.Conn, upTo int) error {
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
		if upTo > 0 && version > upTo {
			break
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
	if upTo > 0 {
		return nil
	}
	return migrateTime(ctx, conn)
}

// --- Accounts

// MyNotes is the title of the private collection every account starts with (ADR-20).
const MyNotes = "My notes"

// SeeAccount records a user under the ID it carries, as the gate did before ADR-32: new with a private collection, and
// up to date after. Imports and tests make accounts with it; a sign-in goes through BindGate.
func (db *DB) SeeAccount(ctx context.Context, user account.User) error {
	return db.seeExternal(ctx, user, SourceGate)
}

// BindGate returns the account bound to the lasting ID the gate gave a person (ADR-32, ADR-35), and records what the
// gate says of them. At first contact it makes the account, keyed by that ID where no account has that key, with its
// private collection. A request that is no interactive session brings a part of the roles only (the gate's tokens carry
// fewer), so only a session changes the groups of an account that exists.
func (db *DB) BindGate(ctx context.Context, user account.User) (Account, error) {
	if user.RemoteID == "" {
		return Account{}, ErrNotFound
	}
	var a Account
	var err error
	// Two first requests of a person at the same moment both find no account; the second one finds the first's.
	for range 2 {
		if a, err = db.bindGate(ctx, user); !isUnique(err, "accounts_pkey") && !isUnique(err, "accounts_remote_id") {
			break
		}
	}
	return a, err
}

func (db *DB) bindGate(ctx context.Context, user account.User) (Account, error) {
	var a Account
	err := pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		var accountID string
		err := tx.QueryRow(ctx, "SELECT id FROM accounts WHERE remote_id = $1", user.RemoteID).Scan(&accountID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			if err := roomForUser(ctx, tx); err != nil {
				return err
			}
			// The key is the ID of the gate, as it was for accounts of the gate before; where an account has that key
			// already (an account under its login, made before), the new one gets a key of its own.
			accountID = user.RemoteID
			var taken bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM accounts WHERE id = $1)", accountID).Scan(&taken); err != nil {
				return err
			}
			if taken {
				accountID = id.New()
			}
			if _, err := tx.Exec(ctx, `INSERT INTO accounts (id, remote_id, login, name, email, groups, source, gate_kind, service)
				VALUES ($1, $2, $3, $4, $5, $6, 'gate', $7, $8)`, accountID, user.RemoteID, user.Login, user.Name, user.Email,
				nonNil(user.Groups), user.GateKind, user.Kind == account.KindService); err != nil {
				return err
			}
			if _, err := insertObject(ctx, tx, Object{Kind: KindCollection, Title: MyNotes, Owner: accountID, Private: true}); err != nil {
				return err
			}
		case err != nil:
			return err
		default:
			if _, err := tx.Exec(ctx, `UPDATE accounts SET login = $2, name = $3, email = $4, gate_kind = $5, last_seen = now(),
				groups = CASE WHEN $6 THEN $7 ELSE groups END, service = $8 WHERE id = $1`, accountID, user.Login, user.Name, user.Email,
				user.GateKind, user.AuthKind == account.AuthSession, nonNil(user.Groups), user.Kind == account.KindService); err != nil {
				return err
			}
		}
		// The groups the sign-in names are groups taken over (ADR-29).
		if err := syncGroups(ctx, tx, user.Groups); err != nil {
			return err
		}
		a, err = scanAccount(tx.QueryRow(ctx, `SELECT `+accountColumns+` FROM accounts WHERE id = $1`, accountID))
		return err
	})
	return a, err
}

// SetServiceAccounts marks the accounts bound to these lasting IDs of the gate as service accounts, and every other one
// as none, in every tenant (B-3). The start calls it with the IDs an administrator allowed, so a decision about an
// account that has not signed in since holds as well; each sign-in marks its account again (BindGate).
func (db *DB) SetServiceAccounts(ctx context.Context, remoteIDs []string) error {
	return db.ForEachTenant(ctx, func(ctx context.Context) error {
		_, err := db.pool.Exec(ctx, `UPDATE accounts SET service = (remote_id IS NOT NULL AND remote_id = ANY ($1::text[]))
			WHERE service IS DISTINCT FROM (remote_id IS NOT NULL AND remote_id = ANY ($1::text[]))`, nonNil(remoteIDs))
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
	// KindNotes is a board of quick notes: its cards are its blocks (M-8).
	KindNotes = "notes"
	KindFile  = "file"
)

// Areas of objects (ADR-28): the documents, and the view Files with its roots.
const (
	AreaDocs  = "docs"
	AreaFiles = "files"
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
	// Access is the role the whole tenant is granted on this object itself (ADR-29); empty for none here. Only for a new
	// object it is what to grant.
	Access string
	// Breaks tells that the object breaks the inheritance of roles: it takes none from above (ADR-29).
	Breaks  bool
	Private bool
	Created time.Time
	Changed time.Time
	// Tags are kept lower case, each once, in the order given (CleanTags).
	Tags []string
	// Width and Contents say how the document is shown (ADR-16); empty means the default of the workspace.
	Width    string
	Contents string
	// Template tells that a page is offered as a template for new pages (M-8).
	Template bool
	// Area is where the object lives: AreaDocs for the documents, AreaFiles for the view Files (ADR-28). Below the top it
	// is the area of the parent.
	Area string
	// PageKind is the kind of a page (T-20, ADR-33), one of package pagekind; empty for none and for every other object.
	PageKind string
	// Archived tells that the object is an archived space (T-34): kept without a limit, hidden from the tree and the
	// search unless asked for, read only for everyone until it is restored. Only a collection at the top is.
	Archived bool
}

const columns = `id, kind, title, description, owner, coalesce(parent, ''), position, coalesce(tenant_grant(id), ''), NOT inherit, private, created, changed, tags,
	coalesce(width, ''), coalesce(contents, ''), template, area, page_kind, archived IS NOT NULL`

// fields are the places columns scans into, in their order.
func fields(o *Object) []any {
	return []any{&o.ID, &o.Kind, &o.Title, &o.Description, &o.Owner, &o.Parent, &o.Position, &o.Access, &o.Breaks, &o.Private, &o.Created, &o.Changed, &o.Tags,
		&o.Width, &o.Contents, &o.Template, &o.Area, &o.PageKind, &o.Archived}
}

func scan(row pgx.Row) (Object, error) {
	var o Object
	err := row.Scan(fields(&o)...)
	return o, err
}

type execer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// tagsOf gives the tags to store for an object: never NULL.
func tagsOf(o Object) []string {
	if o.Tags == nil {
		return []string{}
	}
	return o.Tags
}

// areaOf gives the area of an object at the top: the documents unless it says otherwise.
func areaOf(o Object) string {
	if o.Area == "" {
		return AreaDocs
	}
	return o.Area
}

// insertObject gives the object a new ID and puts it last at its place. A clash of IDs is drawn again.
func insertObject(ctx context.Context, tx execer, o Object) (Object, error) {
	o.Title = strings.TrimSpace(o.Title)
	if n := len([]rune(o.Title)); n < 1 || n > 200 {
		return Object{}, ErrTitle
	}
	if o.Access != "" && (!grantable(o.Access) || o.Private || IsDivider(o.Kind)) {
		return Object{}, ErrRole
	}
	// Nothing new lands in a space that is archived meanwhile (B-6): the insert holds the space, in a transaction of its
	// own where the caller has none.
	if o.Parent != "" {
		if _, inTx := tx.(pgx.Tx); !inTx {
			if b, ok := tx.(interface {
				Begin(context.Context) (pgx.Tx, error)
			}); ok {
				var made Object
				err := pgx.BeginFunc(ctx, b, func(t pgx.Tx) error {
					var err error
					made, err = insertObject(ctx, t, o)
					return err
				})
				return made, err
			}
		}
		if err := holdSpace(ctx, tx, o.Parent); err != nil {
			return Object{}, err
		}
	}
	for attempt := 0; ; attempt++ {
		o.ID = id.New()
		created, err := scan(tx.QueryRow(ctx, `
			INSERT INTO objects (id, kind, title, description, owner, parent, position, private, tags, width, contents, area, page_kind)
			VALUES ($1, $2, $3, $4, $5, nullif($6, ''),
				(SELECT coalesce(max(position) + 1, 0) FROM objects WHERE parent IS NOT DISTINCT FROM nullif($6, '')),
				$7, $8, nullif($9, ''), nullif($10, ''), coalesce((SELECT area FROM objects WHERE id = nullif($6, '')), $11), $12)
			RETURNING `+columns, o.ID, o.Kind, o.Title, o.Description, o.Owner, o.Parent, o.Private, tagsOf(o), o.Width, o.Contents, areaOf(o), o.PageKind))
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == "objects_pkey" && attempt < 5 {
			continue
		}
		if err != nil || o.Access == "" {
			return created, err
		}
		if err := setGrant(ctx, tx, created.ID, ToTenant, "", o.Access, o.Owner); err != nil {
			return Object{}, err
		}
		created.Access = o.Access
		return created, nil
	}
}

// ErrSpaces is a new collection at the top, a space, made by someone without the role of the gate to create spaces
// (ADR-35, B-3).
var ErrSpaces = fmt.Errorf("%w: only administrators and whoever the gate gives the role to create spaces make a collection at the top", ErrNotAllowed)

// maySpace refuses a new collection at the top, made, copied or moved there, to a request of anyone but an
// administrator or a holder of the role to create spaces (account.User.Spaces), in a session: never an agent (B-5). Without a request in the context (the
// command line, an import, a job), whoever runs the instance acts, and it is allowed.
func maySpace(ctx context.Context, accountID string) error {
	u, ok := account.From(ctx)
	if !ok {
		return nil
	}
	// A service account makes no space, whatever it carries (B-4), and no agent does: a key, an app password or OAuth
	// of a person with the role neither (B-5).
	if u.ID != accountID || u.Kind == account.KindService || u.Agent() || !(u.Admin || u.Spaces) {
		return ErrSpaces
	}
	return nil
}

// CreateCollection makes a collection at the top, or inside parent if that is set. One at the top is a space: only an
// administrator or a holder of the role to create spaces makes one (maySpace), and it belongs to its owner alone until
// they share it: it grants nobody a role, not even the whole tenant (ADR-29, 2026-11-19). It is not private: roles and
// links can be granted on it like on any other. One inside a parent inherits what holds there, and needs create there.
func (db *DB) CreateCollection(ctx context.Context, owner account.User, title, parent string) (Object, error) {
	if parent == "" {
		if err := maySpace(ctx, owner.ID); err != nil {
			return Object{}, err
		}
	} else {
		p, err := db.Object(ctx, parent, owner.ID)
		if err != nil {
			return Object{}, err
		}
		if err := db.mayDo(ctx, p, owner.ID, ActCreate); err != nil {
			return Object{}, err
		}
	}
	return insertObject(ctx, db.pool, Object{Kind: KindCollection, Title: title, Owner: owner.ID, Parent: parent})
}

// alive is a condition on o: it is not in the trash. Deleting marks everything below an object too, so an object that is
// not deleted never lies below one that is.
const alive = `o.deleted_at IS NULL`

// Collections returns the collections of the documents at the top that the viewer may see, in their order, without the
// archived ones (T-34), which ArchivedSpaces lists. The roots of the view Files are collections too, and come from
// FileRoots.
func (db *DB) Collections(ctx context.Context, viewer string) ([]Object, error) {
	return db.list(ctx, `SELECT `+columns+` FROM objects o WHERE parent IS NULL AND kind = 'collection' AND area = 'docs' AND `+alive+` AND `+notArchived+` AND `+visible+`
		ORDER BY private DESC, position, created`, viewer)
}

// Children returns what lies directly inside an object the viewer may see, in its order.
func (db *DB) Children(ctx context.Context, parent, viewer string) ([]Object, error) {
	return db.list(ctx, `SELECT `+columns+` FROM objects o WHERE parent = $2 AND `+alive+` AND `+visible+` ORDER BY position, created`, viewer, parent)
}

// Object returns one object, or ErrNotFound if it does not exist, lies in the trash or the viewer may not see it. These
// look alike on purpose: an ID must not tell whether a private object exists. Every change starts here, so nothing in
// the trash is changed but by Restore and Purge.
func (db *DB) Object(ctx context.Context, objectID, viewer string) (Object, error) {
	o, err := scan(db.pool.QueryRow(ctx, `SELECT `+columns+` FROM objects o WHERE id = $2 AND `+alive+` AND `+visible, viewer, objectID))
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
	if err := db.mayDo(ctx, o, viewer, ActWrite); err != nil {
		return Object{}, err
	}
	// A name of the view Files becomes a path on a share and in an export (ADR-30).
	if o.Area == AreaFiles {
		if title, err = CleanFileName(title); err != nil {
			return Object{}, err
		}
	}
	if err := db.guardFile(ctx, o); err != nil {
		return Object{}, err
	}
	var renamed Object
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 FOR NO KEY UPDATE", objectID); err != nil {
			return err
		}
		if err := holdSpace(ctx, tx, objectID); err != nil {
			return err
		}
		renamed, err = scan(tx.QueryRow(ctx, `UPDATE objects SET title = $2, changed = now() WHERE id = $1 RETURNING `+columns, objectID, title))
		return err
	})
	return renamed, err
}
