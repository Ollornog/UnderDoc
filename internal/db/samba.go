// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// Samba (ADR-30): the Samba users of a tenant, the libraries shared over SMB, and what the working copy on disk last
// agreed on with the database. Package share keeps the working copy; it reads and changes the database only through
// these functions and the ordinary ones, the latter always as the account that made the change on the share.

// Errors of Samba a caller can tell apart.
var (
	ErrSambaName = errors.New("a Samba user name has 1 to 32 characters a-z, 0-9, '.', '_' or '-', starts with a letter, and is free")
	ErrNoSamba   = errors.New("samba is not set up for this tenant")
	// ErrPerFolder is breaking the inheritance of a single file in a library shared over Samba (ADR-29, ADR-30).
	ErrPerFolder = errors.New("samba knows rights per folder only: a single file in a shared library cannot break the inheritance")
)

// OpenError is a change of a file that is open over SMB: the web does not write it then (ADR-30).
type OpenError struct{ Users []string }

func (e *OpenError) Error() string {
	if len(e.Users) == 0 {
		return "the file is open over Samba"
	}
	return "the file is open over Samba by " + strings.Join(e.Users, ", ")
}

// SambaAccount is an account with an app password for SMB.
type SambaAccount struct {
	Account string
	Name    string
	UID     int
	// Sealed is the NT hash of the app password, sealed by the vault for the tenant and samba:<account>.
	Sealed []byte
	Login  string
}

// SambaSecretName is the name the NT hash of an account is sealed under.
func SambaSecretName(account string) string { return "samba:" + account }

// SetSamba names the tenants that have a Samba companion. Without one, nothing of a tenant is shared.
func (db *DB) SetSamba(tenants map[string]bool) { db.samba = tenants }

// SambaOn tells whether the tenant of the context has a Samba companion.
func (db *DB) SambaOn(ctx context.Context) bool {
	t, _ := tenant.From(ctx)
	return db.samba[t]
}

// SetFileGuard sets what is asked before the web changes a file or folder of the view Files: package share answers an
// OpenError while the file is open over SMB. Changes the share makes itself pass (FromShare).
func (db *DB) SetFileGuard(guard func(ctx context.Context, objectID string) error) { db.guard = guard }

type fromShareKey struct{}

// FromShare marks a context of the working copy: its changes are not held back by the guard, since they come from SMB.
func FromShare(ctx context.Context) context.Context {
	return context.WithValue(ctx, fromShareKey{}, true)
}

// guardFile asks the guard about an object of the view Files.
func (db *DB) guardFile(ctx context.Context, o Object) error {
	if db.guard == nil || o.Area != AreaFiles || ctx.Value(fromShareKey{}) != nil {
		return nil
	}
	return db.guard(ctx, o.ID)
}

// SambaAccountOf returns the Samba user of an account, ErrNotFound for none.
func (db *DB) SambaAccountOf(ctx context.Context, accountID string) (SambaAccount, error) {
	var a SambaAccount
	err := db.pool.QueryRow(ctx, `SELECT s.account, s.name, s.uid, s.nt_hash, a.login FROM samba_accounts s JOIN accounts a ON a.id = s.account
		WHERE s.account = $1`, accountID).Scan(&a.Account, &a.Name, &a.UID, &a.Sealed, &a.Login)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// SambaAccounts returns every Samba user of the tenant.
func (db *DB) SambaAccounts(ctx context.Context) ([]SambaAccount, error) {
	rows, err := db.pool.Query(ctx, `SELECT s.account, s.name, s.uid, s.nt_hash, a.login FROM samba_accounts s JOIN accounts a ON a.id = s.account
		ORDER BY s.name`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (SambaAccount, error) {
		var a SambaAccount
		err := row.Scan(&a.Account, &a.Name, &a.UID, &a.Sealed, &a.Login)
		return a, err
	})
}

// SetSambaPassword keeps the sealed NT hash of a new app password for an account, making its Samba user with the name and
// a free UID of the range when it has none; the name of an existing one stays. The log names the change, never a value.
func (db *DB) SetSambaPassword(ctx context.Context, accountID, name string, sealed []byte, uidMin, uidMax int) (SambaAccount, error) {
	if !db.SambaOn(ctx) {
		return SambaAccount{}, ErrNoSamba
	}
	var a SambaAccount
	err := pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		// One new Samba user at a time per tenant: the free UID and name are chosen under this lock.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext(current_setting('underdoc.tenant', true) || ':samba'))"); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, "UPDATE samba_accounts SET nt_hash = $2, changed = now() WHERE account = $1 RETURNING name, uid",
			accountID, sealed).Scan(&a.Name, &a.UID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var uid int
		if err := tx.QueryRow(ctx, `SELECT min(u) FROM generate_series($1::int, $2::int) u
			WHERE NOT EXISTS (SELECT 1 FROM samba_accounts WHERE uid = u)`, uidMin, uidMax).Scan(&uid); err != nil {
			return fmt.Errorf("no free UID for a Samba user: %w", err)
		}
		_, err = tx.Exec(ctx, "INSERT INTO samba_accounts (account, name, uid, nt_hash) VALUES ($1, $2, $3, $4)", accountID, name, uid, sealed)
		if isUnique(err, "samba_accounts_tenant_name_key") || isCheck(err) {
			return ErrSambaName
		}
		a.Name, a.UID = name, uid
		return err
	})
	if err != nil {
		return SambaAccount{}, err
	}
	a.Account, a.Sealed = accountID, sealed
	return a, db.Log(ctx, accountID, "app password", "", "Samba user "+a.Name)
}

// RemoveSambaAccount removes the Samba user of an account: the account itself or an administrator. The log keeps who
// did it.
func (db *DB) RemoveSambaAccount(ctx context.Context, actor Actor, accountID string) error {
	if actor.ID != accountID && !actor.Admin {
		return ErrNotAllowed
	}
	var name string
	err := db.pool.QueryRow(ctx, "DELETE FROM samba_accounts WHERE account = $1 RETURNING name", accountID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return db.Log(ctx, actor.ID, "app password removed", "", "Samba user "+name+" of "+accountID)
}

func isCheck(err error) bool {
	var pg interface{ SQLState() string }
	return errors.As(err, &pg) && pg.SQLState() == "23514"
}

// SharedRoot is a library shared over Samba: the folder of the tenant, the folder of an account with an app password, or
// a collection whose manager switched its share on.
type SharedRoot struct {
	ID    string
	Role  string
	Title string
	Owner string
	// Account is the account whose folder it is, for a root of the role user.
	Account string
}

// SharedRoots returns the libraries of the tenant shared over Samba; none without a companion.
func (db *DB) SharedRoots(ctx context.Context) ([]SharedRoot, error) {
	if !db.SambaOn(ctx) {
		return nil, nil
	}
	rows, err := db.pool.Query(ctx, `SELECT o.id, r.role, o.title, o.owner, coalesce(r.account, '') FROM file_roots r JOIN objects o ON o.id = r.object
		WHERE o.deleted_at IS NULL AND (r.role = 'tenant' OR (r.role = 'collection' AND r.samba)
			OR (r.role = 'user' AND EXISTS (SELECT 1 FROM samba_accounts s WHERE s.account = r.account)))
		ORDER BY r.role, o.title, o.id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (SharedRoot, error) {
		var s SharedRoot
		err := row.Scan(&s.ID, &s.Role, &s.Title, &s.Owner, &s.Account)
		return s, err
	})
}

// sharedRootOf returns the shared library an object lies in, "" when it lies in none.
func (db *DB) sharedRootOf(ctx context.Context, q execer, objectID string) (string, error) {
	if !db.SambaOn(ctx) {
		return "", nil
	}
	root, err := rootOf(ctx, q, objectID)
	if err != nil {
		return "", err
	}
	var shared bool
	err = q.QueryRow(ctx, `SELECT r.role = 'tenant' OR (r.role = 'collection' AND r.samba)
			OR (r.role = 'user' AND EXISTS (SELECT 1 FROM samba_accounts s WHERE s.account = r.account))
		FROM file_roots r WHERE r.object = $1`, root).Scan(&shared)
	if errors.Is(err, pgx.ErrNoRows) || !shared {
		return "", nil
	}
	return root, err
}

// SharedRootOf returns the library shared over Samba an object lies in, "" for none.
func (db *DB) SharedRootOf(ctx context.Context, objectID string) (string, error) {
	return db.sharedRootOf(ctx, db.pool, objectID)
}

// SetRootSamba switches the share of a collection of the view Files on or off: whoever manages it. The folder of the
// tenant and the folders of users are shared by the setup and the app passwords, not by this.
func (db *DB) SetRootSamba(ctx context.Context, rootID, viewer string, on bool) error {
	if !db.SambaOn(ctx) {
		return ErrNoSamba
	}
	o, err := db.Object(ctx, rootID, viewer)
	if err != nil {
		return err
	}
	if err := db.may(ctx, o, viewer, RoleManage); err != nil {
		return err
	}
	tag, err := db.pool.Exec(ctx, "UPDATE file_roots SET samba = $2 WHERE object = $1 AND role = 'collection'", rootID, on)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrPlace
	}
	detail := "off"
	if on {
		detail = "on"
	}
	return db.Log(ctx, viewer, "samba share", rootID, detail)
}

// ShareNode is an object of a shared library as the working copy needs it: where it lies, what it is, and the current
// version of a file.
type ShareNode struct {
	ID     string
	Parent string
	Kind   string
	Name   string
	Owner  string
	SHA    string
	Size   int64
}

// ShareTree returns every object of a library that is not in the trash, from the top down, for the working copy. It is
// not for showing: it asks no viewer.
func (db *DB) ShareTree(ctx context.Context, rootID string) ([]ShareNode, error) {
	rows, err := db.pool.Query(ctx, `WITH RECURSIVE down AS (
			SELECT o.id, '' AS parent, o.kind, o.title, o.owner, 0 AS depth FROM objects o WHERE o.id = $1 AND o.deleted_at IS NULL
			UNION ALL SELECT c.id, c.parent, c.kind, c.title, c.owner, down.depth + 1 FROM objects c JOIN down ON c.parent = down.id
				WHERE c.deleted_at IS NULL AND c.kind IN ('folder', 'file') AND down.depth < 100
		) SELECT d.id, d.parent, d.kind, d.title, d.owner, coalesce(v.blob, ''), coalesce(v.size, 0) FROM down d
			LEFT JOIN LATERAL (SELECT blob, size FROM file_versions WHERE file = d.id ORDER BY number DESC LIMIT 1) v ON true
		ORDER BY d.depth, d.title, d.id`, rootID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ShareNode, error) {
		var n ShareNode
		err := row.Scan(&n.ID, &n.Parent, &n.Kind, &n.Name, &n.Owner, &n.SHA, &n.Size)
		return n, err
	})
}

// ShareRecord is what the working copy and the database last agreed on for one object.
type ShareRecord struct {
	Object  string
	Root    string
	Path    string
	Kind    string
	Inode   uint64
	Size    int64
	MtimeNS int64
	SHA     string
}

// ShareRecords returns the records of a library; with root "" those of every library of the tenant.
func (db *DB) ShareRecords(ctx context.Context, rootID string) ([]ShareRecord, error) {
	rows, err := db.pool.Query(ctx, `SELECT object, root, path, kind, inode, size, mtime_ns, sha256 FROM share_files
		WHERE $1 = '' OR root = $1 ORDER BY path`, rootID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ShareRecord, error) {
		var r ShareRecord
		var inode int64
		err := row.Scan(&r.Object, &r.Root, &r.Path, &r.Kind, &inode, &r.Size, &r.MtimeNS, &r.SHA)
		r.Inode = uint64(inode) // #nosec G115 -- an inode number kept as it came
		return r, err
	})
}

// PutShareRecord keeps the record of an object.
func (db *DB) PutShareRecord(ctx context.Context, r ShareRecord) error {
	_, err := db.pool.Exec(ctx, `INSERT INTO share_files (object, root, path, kind, inode, size, mtime_ns, sha256) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (tenant, object) DO UPDATE SET root = $2, path = $3, kind = $4, inode = $5, size = $6, mtime_ns = $7, sha256 = $8`,
		r.Object, r.Root, r.Path, r.Kind, int64(r.Inode), r.Size, r.MtimeNS, r.SHA) // #nosec G115
	return err
}

// DeleteShareRecords removes records: the named objects, or with root and no objects every record of that library.
func (db *DB) DeleteShareRecords(ctx context.Context, rootID string, objects ...string) error {
	var err error
	if len(objects) == 0 {
		_, err = db.pool.Exec(ctx, "DELETE FROM share_files WHERE root = $1", rootID)
	} else {
		_, err = db.pool.Exec(ctx, "DELETE FROM share_files WHERE object = ANY($1)", objects)
	}
	return err
}

// FolderAccess returns, for the root of a library and every folder in it, the role of each named account as a rank
// (0 none, 1 read, 2 comment, 3 edit, 4 manage, 5 owner): what the ACLs of the working copy are made of.
func (db *DB) FolderAccess(ctx context.Context, rootID string, accounts []string) (map[string]map[string]int, error) {
	out := map[string]map[string]int{}
	if len(accounts) == 0 {
		return out, nil
	}
	rows, err := db.pool.Query(ctx, `WITH RECURSIVE down AS (
			SELECT id, 0 AS depth FROM objects WHERE id = $1 AND deleted_at IS NULL
			UNION ALL SELECT c.id, down.depth + 1 FROM objects c JOIN down ON c.parent = down.id
				WHERE c.deleted_at IS NULL AND c.kind = 'folder' AND down.depth < 100
		) SELECT d.id, a, object_role(d.id, a) FROM down d CROSS JOIN unnest($2::text[]) a`, rootID, accounts)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var folder, account string
		var rank int
		if err := rows.Scan(&folder, &account, &rank); err != nil {
			return nil, err
		}
		if out[folder] == nil {
			out[folder] = map[string]int{}
		}
		out[folder][account] = rank
	}
	return out, rows.Err()
}

// ShareNewFile makes a file that came over Samba, as the account that made it: it needs edit on the folder, like an
// upload. The blob is in the store already; sha and size are its own.
func (db *DB) ShareNewFile(ctx context.Context, viewer, parentID, name, sha string, size int64, contentType string) (Object, error) {
	name, err := CleanFileName(name)
	if err != nil {
		return Object{}, err
	}
	p, err := db.Object(ctx, parentID, viewer)
	if err != nil {
		return Object{}, err
	}
	if err := db.mayEdit(ctx, p, viewer); err != nil {
		return Object{}, err
	}
	if p.Area != AreaFiles || (p.Kind != KindFolder && p.Kind != KindCollection) {
		return Object{}, ErrPlace
	}
	var made Object
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if err := db.shareBlob(ctx, tx, p.ID, sha, size); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 FOR UPDATE", p.ID); err != nil {
			return err
		}
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM objects o WHERE parent = $1 AND lower(title) = lower($2) AND `+alive+`)`,
			p.ID, name).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return fmt.Errorf("%w: something in the folder has this name", ErrPlace)
		}
		if made, err = insertObject(ctx, tx, Object{Kind: KindFile, Title: name, Owner: viewer, Parent: p.ID}); err != nil {
			return err
		}
		_, err = addVersion(ctx, tx, made.ID, viewer, sha, size, name, contentType)
		return err
	})
	if err != nil {
		return Object{}, err
	}
	return made, db.Log(ctx, viewer, "change", made.ID, "made over Samba")
}

// ShareNewVersion makes a new version of a file from what came over Samba, as the account that wrote it: it needs edit
// on the file.
func (db *DB) ShareNewVersion(ctx context.Context, viewer, fileID, sha string, size int64, contentType string) (int, error) {
	f, err := db.Object(ctx, fileID, viewer)
	if err != nil {
		return 0, err
	}
	if f.Kind != KindFile {
		return 0, ErrNotFile
	}
	if err := db.mayEdit(ctx, f, viewer); err != nil {
		return 0, err
	}
	var number int
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if err := db.shareBlob(ctx, tx, f.ID, sha, size); err != nil {
			return err
		}
		number, err = addVersion(ctx, tx, f.ID, viewer, sha, size, f.Title, contentType)
		return err
	})
	if err != nil {
		return 0, err
	}
	return number, db.Log(ctx, viewer, "change", f.ID, "saved over Samba")
}

// shareBlob registers a blob of the store for a file of a library and checks the limits: the largest size and the quota
// of the library and of the tenant, as an upload does.
func (db *DB) shareBlob(ctx context.Context, tx pgx.Tx, objectID, sha string, size int64) error {
	if db.limits.MaxFile > 0 && size > db.limits.MaxFile {
		return ErrTooLarge
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared($1)", lockBlobs); err != nil {
		return err
	}
	root, err := rootOf(ctx, tx, objectID)
	if err != nil {
		return err
	}
	if err := db.checkRoom(ctx, tx, root, size); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO blobs (sha256, size) VALUES ($1, $2) ON CONFLICT (tenant, sha256) DO NOTHING", sha, size)
	return err
}

// ListenShare calls fn with the tenant of every change the working copy must look at (migration 027), until ctx ends or
// the connection fails.
func (db *DB) ListenShare(ctx context.Context, fn func(tenant string)) error {
	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "LISTEN underdoc_share"); err != nil {
		return err
	}
	for {
		note, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		fn(note.Payload)
	}
}

// RootSamba tells whether an object is a collection of the view Files, whose share its managers switch, and whether it
// is on.
func (db *DB) RootSamba(ctx context.Context, objectID string) (collection, on bool, err error) {
	err = db.pool.QueryRow(ctx, "SELECT samba FROM file_roots WHERE object = $1 AND role = 'collection'", objectID).Scan(&on)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	return err == nil, on, err
}
