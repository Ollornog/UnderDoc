// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// Files (ADR-28): a file is an object of the area Files with its versions; each version names a blob of the content
// store by its SHA-256. The roots of the view Files are collections too (addendum of 2026-11-17): the folder of the
// tenant, one folder per user and collections. Who sees and changes what follows ADR-20 until ADR-29: a user's folder is
// private, the folder of the tenant is read only for all but its owner and the group named as its writers.

// Roles of the roots of the view Files.
const (
	RootTenant     = "tenant"
	RootUser       = "user"
	RootCollection = "collection"
)

// Titles the roots get when they are made; their owner may rename them.
const (
	TenantFolder = "Company"
	MyFiles      = "My files"
)

// Errors of files a caller can tell apart.
var (
	ErrFileName = errors.New("a file name is not . or .., and has 1 to 200 characters without / \\ or control characters")
	ErrTooLarge = errors.New("the file is larger than allowed")
	ErrQuota    = errors.New("the quota does not leave room for this")
	ErrUpload   = errors.New("this upload is not complete or does not match")
	ErrNotFile  = errors.New("this is not a file")
	// ErrNoLink is a space the viewer sees without files of its own, or with files the viewer does not see.
	ErrNoLink = errors.New("this space names no files you see")
)

// Limits are the sizes the files may have, in bytes; 0 means no limit. The tenant limit covers all files, versions and
// attachments of the tenant; the others are defaults for roots that have no quota of their own.
type Limits struct {
	MaxFile    int64
	Tenant     int64
	User       int64
	Collection int64
}

// SetLimits sets the limits for files. Without it nothing is limited.
func (db *DB) SetLimits(l Limits) { db.limits = l }

// Root is a root of the view Files.
type Root struct {
	Object
	Role string
	// Writers is the group that writes in the folder of the tenant.
	Writers string
	// Quota is the quota of its own, -1 for none.
	Quota int64
	// Limit is the quota that holds: its own, else the default of its role, 0 for none.
	Limit int64
	// Used is what its files take, every version and every running upload counted.
	Used int64
}

// FileInfo is a file as a list shows it: the object and its current version.
type FileInfo struct {
	Object
	Size        int64
	ContentType string
	Versions    int
	Blob        string
	ChangedBy   string
}

// FileVersion is one version of a file.
type FileVersion struct {
	Number      int
	Blob        string
	Size        int64
	Name        string
	ContentType string
	Author      string
	AuthorName  string
	At          time.Time
}

// CleanFileName checks a name a file is given and returns it trimmed. It is a label only: no name ever becomes a path in
// the store (package store), but it does become one on a share and in an export, so what no file system takes is refused.
func CleanFileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := len([]rune(name)); n < 1 || n > 200 || name == "." || name == ".." {
		return "", ErrFileName
	}
	for _, r := range name {
		if r == '/' || r == '\\' || r == 0 || unicode.IsControl(r) {
			return "", ErrFileName
		}
	}
	return name, nil
}

// --- Roots

// rootOf returns the object at the top an object belongs to, seen by anyone: for quotas and writers, not for showing.
func rootOf(ctx context.Context, q execer, objectID string) (string, error) {
	var root string
	err := q.QueryRow(ctx, `WITH RECURSIVE up AS (
			SELECT id, parent, 0 AS depth FROM objects WHERE id = $1
			UNION ALL SELECT p.id, p.parent, up.depth + 1 FROM objects p JOIN up ON p.id = up.parent WHERE up.depth < 100
		) SELECT id FROM up WHERE parent IS NULL`, objectID).Scan(&root)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return root, err
}

// EnsureRoots makes the folder of the user, and the folder of the tenant when an administrator comes first: its owner
// may change everything in it, so it must not be someone else. adminGroup becomes the group that writes there.
func (db *DB) EnsureRoots(ctx context.Context, user account.User, admin bool, adminGroup string) error {
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		// One at a time, so two first requests do not make two folders.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockRoots); err != nil {
			return err
		}
		var mine bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM file_roots WHERE role = 'user' AND account = $1)", user.ID).Scan(&mine); err != nil {
			return err
		}
		if !mine {
			o, err := insertObject(ctx, tx, Object{Kind: KindCollection, Title: MyFiles, Owner: user.ID, Private: true, Area: AreaFiles})
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "INSERT INTO file_roots (object, role, account) VALUES ($1, 'user', $2)", o.ID, user.ID); err != nil {
				return err
			}
		}
		if !admin {
			return nil
		}
		var tenant bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM file_roots WHERE role = 'tenant')").Scan(&tenant); err != nil {
			return err
		}
		if tenant {
			return nil
		}
		o, err := insertObject(ctx, tx, Object{Kind: KindCollection, Title: TenantFolder, Owner: user.ID, Access: "read", Area: AreaFiles})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO file_roots (object, role, writers) VALUES ($1, 'tenant', $2)", o.ID, adminGroup)
		return err
	})
}

// Locks taken with pg_advisory_xact_lock: any numbers, the same for every instance.
const (
	lockRoots = 7272823
	lockQuota = 7272824
	// lockBlobs is taken shared by whoever adds a reference to a blob, and alone by the cleaning of the store, so a blob
	// is never removed between being found in the store and being referred to.
	lockBlobs = 7272825
)

const rootColumns = `, r.role, r.writers, coalesce(r.quota, -1)`

func (db *DB) scanRoot(row pgx.Row) (Root, error) {
	var r Root
	o := &r.Object
	err := row.Scan(append(fields(o), &r.Role, &r.Writers, &r.Quota)...)
	return r, err
}

// limitOf is the quota that holds for a root: its own, else the default of its role.
func (db *DB) limitOf(role string, quota int64) int64 {
	if quota >= 0 {
		return quota
	}
	switch role {
	case RootUser:
		return db.limits.User
	case RootCollection:
		return db.limits.Collection
	}
	return 0
}

// FileRoots returns the roots of the view Files the viewer sees: the folder of the tenant, their own folder, then the
// collections, each with what it uses.
func (db *DB) FileRoots(ctx context.Context, viewer string) ([]Root, error) {
	rows, err := db.pool.Query(ctx, `SELECT `+columns+rootColumns+` FROM objects o JOIN file_roots r ON r.object = o.id
		WHERE `+alive+` AND `+visible+`
		ORDER BY CASE r.role WHEN 'tenant' THEN 0 WHEN 'user' THEN 1 ELSE 2 END, o.position, o.created`, viewer)
	if err != nil {
		return nil, err
	}
	roots, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Root, error) { return db.scanRoot(row) })
	if err != nil {
		return nil, err
	}
	for i := range roots {
		roots[i].Limit = db.limitOf(roots[i].Role, roots[i].Quota)
		if roots[i].Used, err = used(ctx, db.pool, roots[i].ID); err != nil {
			return nil, err
		}
	}
	return roots, nil
}

// FileRoot returns a root the viewer sees.
func (db *DB) FileRoot(ctx context.Context, rootID, viewer string) (Root, error) {
	r, err := db.scanRoot(db.pool.QueryRow(ctx, `SELECT `+columns+rootColumns+` FROM objects o JOIN file_roots r ON r.object = o.id
		WHERE o.id = $2 AND `+alive+` AND `+visible, viewer, rootID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Root{}, ErrNotFound
	}
	if err != nil {
		return Root{}, err
	}
	r.Limit = db.limitOf(r.Role, r.Quota)
	r.Used, err = used(ctx, db.pool, r.ID)
	return r, err
}

// CreateFileCollection makes a collection among the roots of the view Files, owned by whoever makes it (ADR-20).
func (db *DB) CreateFileCollection(ctx context.Context, owner account.User, title string) (Object, error) {
	var made Object
	err := pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		var err error
		made, err = insertObject(ctx, tx, Object{Kind: KindCollection, Title: title, Owner: owner.ID, Area: AreaFiles})
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO file_roots (object, role) VALUES ($1, 'collection')", made.ID)
		return err
	})
	return made, err
}

// SetQuota sets the quota of a root in bytes, -1 for none of its own. Only administrators set quotas; the caller checks.
func (db *DB) SetQuota(ctx context.Context, rootID string, quota int64) error {
	if quota < -1 {
		return ErrQuota
	}
	tag, err := db.pool.Exec(ctx, "UPDATE file_roots SET quota = CASE WHEN $2::bigint < 0 THEN NULL ELSE $2::bigint END WHERE object = $1", rootID, quota)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}

// SetWriters names the group that writes in the folder of the tenant. Only administrators do; the caller checks.
func (db *DB) SetWriters(ctx context.Context, group string) error {
	group = strings.TrimSpace(group)
	if len(group) > 200 {
		return ErrPlace
	}
	tag, err := db.pool.Exec(ctx, "UPDATE file_roots SET writers = $1 WHERE role = 'tenant'", group)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}

// mayWriteAsGroup tells whether the viewer belongs to the group that writes in the root above an object.
func (db *DB) mayWriteAsGroup(ctx context.Context, objectID, viewer string) (bool, error) {
	root, err := rootOf(ctx, db.pool, objectID)
	if err != nil {
		return false, err
	}
	var ok bool
	err = db.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM file_roots r JOIN accounts a ON a.id = $2
		WHERE r.object = $1 AND r.writers <> '' AND r.writers = ANY (a.groups))`, root, viewer).Scan(&ok)
	return ok, err
}

// isFileRoot tells whether an object is a root of the view Files.
func isFileRoot(ctx context.Context, q execer, objectID string) (bool, error) {
	var root bool
	err := q.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM file_roots WHERE object = $1)", objectID).Scan(&root)
	return root, err
}

// isFixedRoot tells whether an object is the folder of the tenant or of a user: those are never deleted or moved.
func isFixedRoot(ctx context.Context, q execer, objectID string) (bool, error) {
	var fixed bool
	err := q.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM file_roots WHERE object = $1 AND role IN ('tenant', 'user'))", objectID).Scan(&fixed)
	return fixed, err
}

// --- Spaces and their files

// LinkSpaceFiles names a root of the view Files as the files of a collection of the documents ("files of this space"),
// or takes the link away with rootID empty. The viewer must own the collection and see the root.
func (db *DB) LinkSpaceFiles(ctx context.Context, spaceID, viewer, rootID string) error {
	space, err := db.Object(ctx, spaceID, viewer)
	if err != nil {
		return err
	}
	if space.Kind != KindCollection || space.Area != AreaDocs {
		return ErrPlace
	}
	if space.Owner != viewer {
		return ErrNotAllowed
	}
	if rootID == "" {
		_, err := db.pool.Exec(ctx, "DELETE FROM space_files WHERE space = $1", spaceID)
		return err
	}
	if _, err := db.FileRoot(ctx, rootID, viewer); err != nil {
		return err
	}
	_, err = db.pool.Exec(ctx, `INSERT INTO space_files (space, root) VALUES ($1, $2)
		ON CONFLICT (tenant, space) DO UPDATE SET root = EXCLUDED.root`, spaceID, rootID)
	return err
}

// SpaceFiles returns the root linked to a collection of the documents: ErrNotFound if the viewer does not see the
// collection, ErrNoLink if it names no root the viewer sees.
func (db *DB) SpaceFiles(ctx context.Context, spaceID, viewer string) (Root, error) {
	if _, err := db.Object(ctx, spaceID, viewer); err != nil {
		return Root{}, err
	}
	var rootID string
	err := db.pool.QueryRow(ctx, "SELECT root FROM space_files WHERE space = $1", spaceID).Scan(&rootID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Root{}, ErrNoLink
	}
	if err != nil {
		return Root{}, err
	}
	r, err := db.FileRoot(ctx, rootID, viewer)
	if errors.Is(err, ErrNotFound) {
		return Root{}, ErrNoLink
	}
	return r, err
}

// --- Quotas

// used is what the files below a root take: every version of every file, the trash included, and the declared size of
// every upload into it that runs.
func used(ctx context.Context, q execer, rootID string) (int64, error) {
	var n int64
	err := q.QueryRow(ctx, `WITH RECURSIVE down AS (
			SELECT id FROM objects WHERE id = $1
			UNION ALL SELECT c.id FROM objects c JOIN down ON c.parent = down.id
		) SELECT coalesce((SELECT sum(v.size) FROM file_versions v WHERE v.file IN (SELECT id FROM down)), 0)
			+ coalesce((SELECT sum(u.size) FROM uploads u WHERE u.root = $1), 0)`, rootID).Scan(&n)
	return n, err
}

// tenantUsed is what all files, versions, attachments and running uploads of the tenant take.
func tenantUsed(ctx context.Context, q execer) (int64, error) {
	var n int64
	err := q.QueryRow(ctx, `SELECT coalesce((SELECT sum(size) FROM file_versions), 0) + coalesce((SELECT sum(size) FROM attachments), 0)
		+ coalesce((SELECT sum(size) FROM uploads), 0)`).Scan(&n)
	return n, err
}

// checkRoom tells whether size more bytes fit below a root (empty for an attachment) and into the tenant. It must run in
// a transaction holding lockQuota, so two checks never both see the same room.
func (db *DB) checkRoom(ctx context.Context, tx pgx.Tx, rootID string, size int64) error {
	if db.limits.MaxFile > 0 && size > db.limits.MaxFile {
		return ErrTooLarge
	}
	if err := db.checkRootRoom(ctx, tx, rootID, size); err != nil {
		return err
	}
	if db.limits.Tenant > 0 {
		n, err := tenantUsed(ctx, tx)
		if err != nil {
			return err
		}
		if n+size > db.limits.Tenant {
			return ErrQuota
		}
	}
	return nil
}

// checkRootRoom tells whether size more bytes fit below a root; an empty root has room for anything.
func (db *DB) checkRootRoom(ctx context.Context, tx pgx.Tx, rootID string, size int64) error {
	if rootID == "" {
		return nil
	}
	var role string
	var quota int64
	err := tx.QueryRow(ctx, "SELECT role, coalesce(quota, -1) FROM file_roots WHERE object = $1", rootID).Scan(&role, &quota)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	limit := db.limitOf(role, quota)
	if limit <= 0 {
		return nil
	}
	n, err := used(ctx, tx, rootID)
	if err != nil {
		return err
	}
	if n+size > limit {
		return ErrQuota
	}
	return nil
}

// --- Uploads

// Upload is an upload in chunks.
type Upload struct {
	ID       string
	Owner    string
	Parent   string
	File     string
	Page     string
	Root     string
	Name     string
	Size     int64
	Received int64
	Touched  time.Time
}

// UploadTarget says where an upload goes: a new file into a folder or collection of the view Files (Parent), a new
// version of a file (File), or an attachment of a page (Page). Exactly one is set.
type UploadTarget struct {
	Parent, File, Page string
}

func newUploadID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("db: no randomness: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// StartUpload begins an upload of size bytes under name. The viewer must be allowed to change where it goes, and the
// declared size must fit the limits now: it counts against them until the upload ends.
func (db *DB) StartUpload(ctx context.Context, viewer string, target UploadTarget, name string, size int64) (Upload, error) {
	name, err := CleanFileName(name)
	if err != nil {
		return Upload{}, err
	}
	if size < 0 {
		return Upload{}, ErrUpload
	}
	set := 0
	for _, s := range []string{target.Parent, target.File, target.Page} {
		if s != "" {
			set++
		}
	}
	if set != 1 {
		return Upload{}, ErrPlace
	}
	u := Upload{Owner: viewer, Parent: target.Parent, File: target.File, Page: target.Page, Name: name, Size: size}
	switch {
	case target.Parent != "":
		p, err := db.Object(ctx, target.Parent, viewer)
		if err != nil {
			return Upload{}, err
		}
		if p.Area != AreaFiles || (p.Kind != KindCollection && p.Kind != KindFolder) {
			return Upload{}, ErrPlace
		}
		if err := db.mayEdit(ctx, p, viewer); err != nil {
			return Upload{}, err
		}
	case target.File != "":
		f, err := db.Object(ctx, target.File, viewer)
		if err != nil {
			return Upload{}, err
		}
		if f.Kind != KindFile {
			return Upload{}, ErrNotFile
		}
		if err := db.mayEdit(ctx, f, viewer); err != nil {
			return Upload{}, err
		}
	default:
		p, err := db.page(ctx, target.Page, viewer)
		if err != nil {
			return Upload{}, err
		}
		if err := db.mayEdit(ctx, p, viewer); err != nil {
			return Upload{}, err
		}
	}
	if target.Page == "" {
		if u.Root, err = rootOf(ctx, db.pool, target.Parent+target.File); err != nil {
			return Upload{}, err
		}
	}
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockQuota); err != nil {
			return err
		}
		if err := db.checkRoom(ctx, tx, u.Root, size); err != nil {
			return err
		}
		u.ID = newUploadID()
		return tx.QueryRow(ctx, `INSERT INTO uploads (id, owner, parent, file, page, root, name, size)
			VALUES ($1, $2, nullif($3, ''), nullif($4, ''), nullif($5, ''), nullif($6, ''), $7, $8) RETURNING touched`,
			u.ID, viewer, u.Parent, u.File, u.Page, u.Root, name, size).Scan(&u.Touched)
	})
	return u, err
}

// GetUpload returns an upload of the viewer; the uploads of others are not found.
func (db *DB) GetUpload(ctx context.Context, uploadID, viewer string) (Upload, error) {
	var u Upload
	err := db.pool.QueryRow(ctx, `SELECT id, owner, coalesce(parent, ''), coalesce(file, ''), coalesce(page, ''), coalesce(root, ''),
			name, size, received, touched FROM uploads WHERE id = $1 AND owner = $2`, uploadID, viewer).
		Scan(&u.ID, &u.Owner, &u.Parent, &u.File, &u.Page, &u.Root, &u.Name, &u.Size, &u.Received, &u.Touched)
	if errors.Is(err, pgx.ErrNoRows) {
		return Upload{}, ErrNotFound
	}
	return u, err
}

// Received records that an upload has its first n bytes. It only grows from the offset the caller wrote at.
func (db *DB) Received(ctx context.Context, uploadID, viewer string, from, n int64) error {
	tag, err := db.pool.Exec(ctx, "UPDATE uploads SET received = $4, touched = now() WHERE id = $1 AND owner = $2 AND received = $3 AND $4 <= size",
		uploadID, viewer, from, n)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrUpload
	}
	return err
}

// CancelUpload ends an upload without a file; the caller removes its data from the store.
func (db *DB) CancelUpload(ctx context.Context, uploadID, viewer string) error {
	tag, err := db.pool.Exec(ctx, "DELETE FROM uploads WHERE id = $1 AND owner = $2", uploadID, viewer)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return err
}

// ExpireUploads ends uploads that got nothing since before, and calls remove for each so its data leaves the store.
func (db *DB) ExpireUploads(ctx context.Context, before time.Time, remove func(id string) error) (int, error) {
	rows, err := db.pool.Query(ctx, "DELETE FROM uploads WHERE touched < $1 RETURNING id", before)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if err := remove(id); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

// Finished is what a finished upload became: a file, with the version it now has, or an attachment of a page.
type Finished struct {
	File       Object
	Version    int
	Page       string
	Attachment string
}

// FinishUpload turns a complete upload into a file, a new version or an attachment, in one transaction. sha and size are
// those of the data received, contentType what the content shows itself to be; adopt moves the data into the store and
// runs inside the transaction, so the cleaning of the store never sees the blob without its reference. A new file with
// the name of a file already in its folder, ignoring case, becomes a new version of that file.
func (db *DB) FinishUpload(ctx context.Context, uploadID, viewer, sha string, size int64, contentType string, adopt func() error) (Finished, error) {
	u, err := db.GetUpload(ctx, uploadID, viewer)
	if err != nil {
		return Finished{}, err
	}
	if u.Received != u.Size || size != u.Size {
		return Finished{}, ErrUpload
	}
	// Rights are checked again: they may have changed while the data came.
	var target Object
	switch {
	case u.Parent != "":
		if target, err = db.Object(ctx, u.Parent, viewer); err == nil {
			err = db.mayEdit(ctx, target, viewer)
		}
	case u.File != "":
		if target, err = db.Object(ctx, u.File, viewer); err == nil {
			err = db.mayEdit(ctx, target, viewer)
		}
	default:
		if target, err = db.page(ctx, u.Page, viewer); err == nil {
			err = db.mayEdit(ctx, target, viewer)
		}
	}
	if err != nil {
		return Finished{}, err
	}
	var done Finished
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared($1)", lockBlobs); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "DELETE FROM uploads WHERE id = $1 AND owner = $2 AND received = size", uploadID, viewer)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrUpload
		}
		if _, err := tx.Exec(ctx, "INSERT INTO blobs (sha256, size) VALUES ($1, $2) ON CONFLICT (tenant, sha256) DO NOTHING", sha, size); err != nil {
			return err
		}
		if err := adopt(); err != nil {
			return err
		}
		switch {
		case u.Page != "":
			done.Page = u.Page
			done.Attachment, err = addAttachment(ctx, tx, u.Page, viewer, sha, size, u.Name, contentType)
			return err
		case u.File != "":
			done.File = target
			done.Version, err = addVersion(ctx, tx, u.File, viewer, sha, size, target.Title, contentType)
			return err
		}
		var existing Object
		existing, err = scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM objects o WHERE parent = $1 AND lower(title) = lower($2) AND `+alive+`
			ORDER BY kind = 'file' DESC LIMIT 1 FOR UPDATE`, u.Parent, u.Name))
		switch {
		case err == nil && existing.Kind != KindFile:
			return fmt.Errorf("%w: a %s has this name", ErrPlace, existing.Kind)
		case err == nil:
			if err := db.mayEdit(ctx, existing, viewer); err != nil {
				return err
			}
			done.File = existing
			done.Version, err = addVersion(ctx, tx, existing.ID, viewer, sha, size, existing.Title, contentType)
			return err
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		if done.File, err = insertObject(ctx, tx, Object{Kind: KindFile, Title: u.Name, Owner: viewer, Parent: u.Parent}); err != nil {
			return err
		}
		done.Version, err = addVersion(ctx, tx, done.File.ID, viewer, sha, size, u.Name, contentType)
		return err
	})
	return done, err
}

func addVersion(ctx context.Context, tx pgx.Tx, fileID, author, sha string, size int64, name, contentType string) (int, error) {
	var number int
	err := tx.QueryRow(ctx, `INSERT INTO file_versions (file, number, blob, size, name, content_type, author)
		VALUES ($1, (SELECT coalesce(max(number), 0) + 1 FROM file_versions WHERE file = $1), $2, $3, $4, $5, $6) RETURNING number`,
		fileID, sha, size, name, contentType, author).Scan(&number)
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(ctx, "UPDATE objects SET changed = now() WHERE id = $1", fileID)
	return number, err
}

func addAttachment(ctx context.Context, tx pgx.Tx, pageID, author, sha string, size int64, name, contentType string) (string, error) {
	var number int
	if err := tx.QueryRow(ctx, "UPDATE objects SET next_attachment = next_attachment + 1 WHERE id = $1 RETURNING next_attachment - 1", pageID).Scan(&number); err != nil {
		return "", err
	}
	attachmentID := fmt.Sprintf("a%d", number)
	_, err := tx.Exec(ctx, `INSERT INTO attachments (page, id, blob, size, name, content_type, author) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		pageID, attachmentID, sha, size, name, contentType, author)
	return attachmentID, err
}

// --- Files and versions

const fileInfoColumns = `, v_size, v_type, v_number, v_blob, coalesce((SELECT a.name FROM accounts a WHERE a.id = v_author), v_author)`

// fileInfoJoin adds the current version of each file under names of their own, so the columns of the object stay
// unambiguous.
const fileInfoJoin = ` LEFT JOIN LATERAL (SELECT size AS v_size, content_type AS v_type, number AS v_number, blob AS v_blob, author AS v_author
	FROM file_versions WHERE file = o.id ORDER BY number DESC LIMIT 1) v ON true`

func scanFileInfo(row pgx.Row) (FileInfo, error) {
	var f FileInfo
	o := &f.Object
	var size *int64
	var contentType, blob, by *string
	var versions *int
	err := row.Scan(append(fields(o), &size, &contentType, &versions, &blob, &by)...)
	if size != nil {
		f.Size, f.ContentType, f.Versions, f.Blob, f.ChangedBy = *size, *contentType, *versions, *blob, *by
	}
	return f, err
}

// FilesIn returns what lies directly in a folder or collection of the view Files, with the current version of each file.
func (db *DB) FilesIn(ctx context.Context, parentID, viewer string) ([]FileInfo, error) {
	p, err := db.Object(ctx, parentID, viewer)
	if err != nil {
		return nil, err
	}
	if p.Area != AreaFiles {
		return nil, ErrNotFound
	}
	rows, err := db.pool.Query(ctx, `SELECT `+columns+fileInfoColumns+` FROM objects o`+fileInfoJoin+`
		WHERE parent = $2 AND `+alive+` AND `+visible+` ORDER BY kind = 'file', lower(title), created`, viewer, parentID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (FileInfo, error) { return scanFileInfo(row) })
}

// File returns a file the viewer sees, with its current version.
func (db *DB) File(ctx context.Context, fileID, viewer string) (FileInfo, error) {
	f, err := scanFileInfo(db.pool.QueryRow(ctx, `SELECT `+columns+fileInfoColumns+` FROM objects o`+fileInfoJoin+`
		WHERE id = $2 AND kind = 'file' AND `+alive+` AND `+visible, viewer, fileID))
	if errors.Is(err, pgx.ErrNoRows) {
		return FileInfo{}, ErrNotFound
	}
	return f, err
}

// FileVersions returns the versions of a file the viewer sees, newest first.
func (db *DB) FileVersions(ctx context.Context, fileID, viewer string) ([]FileVersion, error) {
	if _, err := db.File(ctx, fileID, viewer); err != nil {
		return nil, err
	}
	rows, err := db.pool.Query(ctx, `SELECT v.number, v.blob, v.size, v.name, v.content_type, v.author,
			coalesce((SELECT a.name FROM accounts a WHERE a.id = v.author), v.author), v.at
		FROM file_versions v WHERE v.file = $1 ORDER BY v.number DESC`, fileID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (FileVersion, error) {
		var v FileVersion
		err := row.Scan(&v.Number, &v.Blob, &v.Size, &v.Name, &v.ContentType, &v.Author, &v.AuthorName, &v.At)
		return v, err
	})
}

// FileVersion returns one version of a file the viewer sees; number 0 is the current one.
func (db *DB) FileVersion(ctx context.Context, fileID, viewer string, number int) (FileVersion, error) {
	versions, err := db.FileVersions(ctx, fileID, viewer)
	if err != nil {
		return FileVersion{}, err
	}
	for _, v := range versions {
		if number == 0 || v.Number == number {
			return v, nil
		}
	}
	return FileVersion{}, ErrNotFound
}

// RestoreVersion makes an older version of a file the current one, as a new version: nothing is lost (ADR-11).
func (db *DB) RestoreVersion(ctx context.Context, fileID, viewer string, number int) (int, error) {
	f, err := db.File(ctx, fileID, viewer)
	if err != nil {
		return 0, err
	}
	if err := db.mayEdit(ctx, f.Object, viewer); err != nil {
		return 0, err
	}
	v, err := db.FileVersion(ctx, fileID, viewer, number)
	if err != nil || number == 0 {
		return 0, ErrNotFound
	}
	root, err := rootOf(ctx, db.pool, fileID)
	if err != nil {
		return 0, err
	}
	var made int
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockQuota); err != nil {
			return err
		}
		if err := db.checkRoom(ctx, tx, root, v.Size); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared($1)", lockBlobs); err != nil {
			return err
		}
		made, err = addVersion(ctx, tx, fileID, viewer, v.Blob, v.Size, f.Title, v.ContentType)
		return err
	})
	return made, err
}

// --- Attachments of pages

// Attachment is a file that belongs to a page, such as a pasted image (ADR-28).
type Attachment struct {
	Page        string
	ID          string
	Blob        string
	Size        int64
	Name        string
	ContentType string
	Author      string
	At          time.Time
}

const attachmentColumns = `page, id, blob, size, name, content_type, author, at`

func scanAttachment(row pgx.Row) (Attachment, error) {
	var a Attachment
	err := row.Scan(&a.Page, &a.ID, &a.Blob, &a.Size, &a.Name, &a.ContentType, &a.Author, &a.At)
	if errors.Is(err, pgx.ErrNoRows) {
		return Attachment{}, ErrNotFound
	}
	return a, err
}

// Attachments returns the attachments of a page the viewer sees, in the order they came.
func (db *DB) Attachments(ctx context.Context, pageID, viewer string) ([]Attachment, error) {
	if _, err := db.page(ctx, pageID, viewer); err != nil {
		return nil, err
	}
	rows, err := db.pool.Query(ctx, `SELECT `+attachmentColumns+` FROM attachments WHERE page = $1 ORDER BY length(id), id`, pageID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Attachment, error) { return scanAttachment(row) })
}

// GetAttachment returns one attachment of a page the viewer sees.
func (db *DB) GetAttachment(ctx context.Context, pageID, attachmentID, viewer string) (Attachment, error) {
	if _, err := db.page(ctx, pageID, viewer); err != nil {
		return Attachment{}, err
	}
	return scanAttachment(db.pool.QueryRow(ctx, `SELECT `+attachmentColumns+` FROM attachments WHERE page = $1 AND id = $2`, pageID, attachmentID))
}

// AttachmentPath is the address an attachment is shown at inside its page.
func AttachmentPath(pageID, attachmentID string) string {
	return "/api/pages/" + pageID + "/attachments/" + attachmentID
}

// FilePath is the address the current content of a file is shown at.
func FilePath(fileID string) string { return "/api/files/" + fileID + "/content" }

// MoveAttachment makes an attachment of a page a file in a folder or collection of the view Files ("move to the files"):
// the file keeps the content, the attachment goes, and every block of the page that showed it shows the file instead, as a
// new version of the block. The viewer must be allowed to change the page and the folder, and the file must fit there.
func (db *DB) MoveAttachment(ctx context.Context, pageID, attachmentID, viewer, parentID string) (Object, error) {
	a, err := db.GetAttachment(ctx, pageID, attachmentID, viewer)
	if err != nil {
		return Object{}, err
	}
	p, err := db.Object(ctx, parentID, viewer)
	if err != nil {
		return Object{}, err
	}
	if p.Area != AreaFiles || (p.Kind != KindCollection && p.Kind != KindFolder) {
		return Object{}, ErrPlace
	}
	if err := db.mayEdit(ctx, p, viewer); err != nil {
		return Object{}, err
	}
	root, err := rootOf(ctx, db.pool, parentID)
	if err != nil {
		return Object{}, err
	}
	var file Object
	err = db.change(ctx, pageID, viewer, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockQuota); err != nil {
			return err
		}
		// The attachment leaves the count of the tenant as the file comes in, so only the root has to have room.
		if err := db.checkRootRoom(ctx, tx, root, a.Size); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock_shared($1)", lockBlobs); err != nil {
			return err
		}
		name, err := freeName(ctx, tx, parentID, a.Name)
		if err != nil {
			return err
		}
		if file, err = insertObject(ctx, tx, Object{Kind: KindFile, Title: name, Owner: viewer, Parent: parentID}); err != nil {
			return err
		}
		if _, err := addVersion(ctx, tx, file.ID, viewer, a.Blob, a.Size, name, a.ContentType); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, "DELETE FROM attachments WHERE page = $1 AND id = $2", pageID, attachmentID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return relinkBlocks(ctx, tx, pageID, viewer, AttachmentPath(pageID, attachmentID), FilePath(file.ID))
	})
	return file, err
}

// freeName returns name, or name with " (2)", " (3)" … before its extension, whichever no entry of the folder has yet.
func freeName(ctx context.Context, tx pgx.Tx, parentID, name string) (string, error) {
	base, ext := name, ""
	if i := strings.LastIndex(name, "."); i > 0 {
		base, ext = name[:i], name[i:]
	}
	for n := 1; n < 1000; n++ {
		candidate := name
		if n > 1 {
			candidate = fmt.Sprintf("%s (%d)%s", base, n, ext)
		}
		var taken bool
		if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM objects WHERE parent = $1 AND lower(title) = lower($2) AND deleted_at IS NULL)",
			parentID, candidate).Scan(&taken); err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
	}
	return "", ErrPlace
}

// relinkBlocks replaces one address by another in every block of a page, each change kept as a version of its block.
func relinkBlocks(ctx context.Context, tx pgx.Tx, pageID, author, from, to string) error {
	rows, err := tx.Query(ctx, "SELECT id, body FROM blocks WHERE page = $1 AND strpos(body, $2) > 0", pageID, from)
	if err != nil {
		return err
	}
	type change struct{ id, body string }
	changes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (change, error) {
		var c change
		err := row.Scan(&c.id, &c.body)
		return c, err
	})
	if err != nil {
		return err
	}
	for _, c := range changes {
		body := strings.ReplaceAll(c.body, from, to)
		if _, err := tx.Exec(ctx, "UPDATE blocks SET body = $3 WHERE page = $1 AND id = $2", pageID, c.id, body); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO block_versions (page, block, body, author) VALUES ($1, $2, $3, $4)", pageID, c.id, body, author); err != nil {
			return err
		}
	}
	return nil
}

// --- Keeping the store clean

// Retention says how long versions are kept in full and how they are thinned after that (ADR-28): every version for
// KeepAll, then the last of each day until KeepDaily, then the last of each week until KeepWeekly, older ones not at all;
// a KeepWeekly of 0 keeps a version a week for good. The current version of a file is always kept. Days count in UTC.
type Retention struct {
	KeepAll, KeepDaily, KeepWeekly time.Duration
}

// ThinVersions drops the versions the retention does not keep, as of now, and returns how many went. Their blobs stay
// until CollectBlobs finds them unused.
func (db *DB) ThinVersions(ctx context.Context, now time.Time, r Retention) (int64, error) {
	seconds := func(d time.Duration) float64 { return d.Seconds() }
	tag, err := db.pool.Exec(ctx, `WITH ranked AS (
			SELECT v.id, v.number = max(v.number) OVER (PARTITION BY v.file) AS current,
				row_number() OVER (PARTITION BY v.file, date_trunc('day', v.at AT TIME ZONE 'UTC') ORDER BY v.number DESC) AS in_day,
				row_number() OVER (PARTITION BY v.file, date_trunc('week', v.at AT TIME ZONE 'UTC') ORDER BY v.number DESC) AS in_week
			FROM file_versions v
		) DELETE FROM file_versions v USING ranked r
		WHERE v.id = r.id AND NOT r.current AND v.at < $1::timestamptz - $2 * interval '1 second' AND (
			(v.at >= $1::timestamptz - $3 * interval '1 second' AND r.in_day > 1)
			OR (v.at < $1::timestamptz - $3 * interval '1 second' AND r.in_week > 1)
			OR ($4 > 0 AND v.at < $1::timestamptz - greatest($4, $3) * interval '1 second'))`,
		now, seconds(r.KeepAll), seconds(max(r.KeepDaily, r.KeepAll)), seconds(r.KeepWeekly))
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// CollectBlobs takes away every blob nothing refers to any more: remove takes it out of the store, and only then its
// row goes, in one transaction that no new reference can pass. It returns how many went.
func (db *DB) CollectBlobs(ctx context.Context, remove func(sha string) error) (int, error) {
	gone := 0
	err := pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockBlobs); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `DELETE FROM blobs b WHERE NOT EXISTS (SELECT 1 FROM file_versions v WHERE v.blob = b.sha256)
			AND NOT EXISTS (SELECT 1 FROM attachments a WHERE a.blob = b.sha256) RETURNING sha256`)
		if err != nil {
			return err
		}
		shas, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, sha := range shas {
			if err := remove(sha); err != nil {
				return err
			}
		}
		gone = len(shas)
		return nil
	})
	return gone, err
}

// SweepStore calls each with every blob of the store and removes those the database does not know, as long as they are
// older than grace: a blob whose transaction failed after it came into the store. It returns how many went.
func (db *DB) SweepStore(ctx context.Context, each func(fn func(sha string, modified time.Time) error) error, remove func(sha string) error, grace time.Duration) (int, error) {
	gone := 0
	err := pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockBlobs); err != nil {
			return err
		}
		return each(func(sha string, modified time.Time) error {
			if time.Since(modified) < grace {
				return nil
			}
			var known bool
			if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM blobs WHERE sha256 = $1)", sha).Scan(&known); err != nil {
				return err
			}
			if known {
				return nil
			}
			gone++
			return remove(sha)
		})
	})
	return gone, err
}

// BlobInUse tells whether a blob is referred to by a version or an attachment the viewer sees: the only way a blob is
// served, so a hash alone never opens content (concept: no door bypasses rights).
func (db *DB) BlobInUse(ctx context.Context, sha, viewer string) (bool, error) {
	var ok bool
	err := db.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM file_versions v JOIN objects o ON o.id = v.file WHERE v.blob = $2 AND `+alive+` AND `+visible+`)
		OR EXISTS (SELECT 1 FROM attachments a JOIN objects o ON o.id = a.page WHERE a.blob = $2 AND `+alive+` AND `+visible+`)`, viewer, sha).Scan(&ok)
	return ok, err
}
