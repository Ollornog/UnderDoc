// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/auth"
	"github.com/Ollornog/UnderDoc/internal/id"
)

// Modes of a share link (ADR-29).
const (
	LinkRead    = "read"
	LinkComment = "comment"
	LinkEdit    = "edit"
	LinkUpload  = "upload"
)

// ErrLink is a link that cannot be made like this: a mode that does not fit the object, or an expiry in the past.
var ErrLink = errors.New("a link reads a page or the files, comments or edits, or uploads into a folder of the files, and ends in the future")

// Link is a share link, without its token: only the hash of the token is kept.
type Link struct {
	ID, Object, ObjectTitle, ObjectKind, Area string
	Mode                                      string
	// Expires is zero for a link without an end.
	Expires     time.Time
	HasPassword bool
	// PasswordHash is the Argon2id hash of the password, for checking it; never shown.
	PasswordHash        string
	CreatedBy, Creator  string
	Created             time.Time
	Frozen              string
	FrozenAt            time.Time
	Released            []string
	Users               int
	ObjectOwner, Parent string
}

const linkColumns = `l.id, l.object, o.title, o.kind, o.area, l.mode, coalesce(l.expires, 'epoch'::timestamptz), l.password <> '', l.password,
	l.created_by, coalesce(a.name, l.created_by), l.created, coalesce(l.frozen, ''), coalesce(l.frozen_at, 'epoch'::timestamptz), l.released,
	(SELECT count(*) FROM share_link_users u WHERE u.link = l.id), o.owner, coalesce(o.parent, '')`

const linkJoins = ` FROM share_links l JOIN objects o ON o.id = l.object LEFT JOIN accounts a ON a.id = l.created_by`

func scanLink(row pgx.Row) (Link, error) {
	var l Link
	err := row.Scan(&l.ID, &l.Object, &l.ObjectTitle, &l.ObjectKind, &l.Area, &l.Mode, &l.Expires, &l.HasPassword, &l.PasswordHash,
		&l.CreatedBy, &l.Creator, &l.Created, &l.Frozen, &l.FrozenAt, &l.Released, &l.Users, &l.ObjectOwner, &l.Parent)
	if l.Expires.Equal(time.Unix(0, 0)) {
		l.Expires = time.Time{}
	}
	if l.FrozenAt.Equal(time.Unix(0, 0)) {
		l.FrozenAt = time.Time{}
	}
	return l, err
}

// NewLink is what a link is made with. Frozen is the page as the link shows it, rendered by the caller with only the
// Released embeddings; empty for anything but a page.
type NewLink struct {
	Mode         string
	Expires      time.Time
	PasswordHash string
	Released     []string
	Frozen       string
}

// CreateLink makes a share link on an object the actor manages, and returns it with its token, which is shown this once.
// Nothing private gets a link, and upload only fits a folder or collection of the files.
func (db *DB) CreateLink(ctx context.Context, objectID string, actor Actor, in NewLink) (Link, string, error) {
	o, err := db.Object(ctx, objectID, actor.ID)
	if err != nil {
		return Link{}, "", err
	}
	if private, err := isPrivate(ctx, db.pool, o.ID); err != nil || private {
		if err == nil {
			err = ErrPrivate
		}
		return Link{}, "", err
	}
	if err := db.may(ctx, o, actor.ID, RoleManage); err != nil {
		return Link{}, "", err
	}
	switch in.Mode {
	case LinkRead:
		// Anyone reads a page frozen, or a file or folder of the files; a collection or folder of the documents is
		// shared with accounts only, to comment or edit.
		if o.Area == AreaDocs && !HasBlocks(o.Kind) {
			return Link{}, "", ErrLink
		}
	case LinkComment, LinkEdit:
	case LinkUpload:
		if o.Area != AreaFiles || (o.Kind != KindCollection && o.Kind != KindFolder) {
			return Link{}, "", ErrLink
		}
	default:
		return Link{}, "", ErrLink
	}
	if IsDivider(o.Kind) || (!in.Expires.IsZero() && !in.Expires.After(time.Now())) {
		return Link{}, "", ErrLink
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		return Link{}, "", err
	}
	var expires *time.Time
	if !in.Expires.IsZero() {
		expires = &in.Expires
	}
	var frozen *string
	var frozenAt *time.Time
	if o.Kind == KindPage || o.Kind == KindNotes {
		now := time.Now()
		frozen, frozenAt = &in.Frozen, &now
	}
	linkID := id.New()
	_, err = db.pool.Exec(ctx, `INSERT INTO share_links (id, object, token, mode, password, expires, created_by, frozen, frozen_at, released)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`, linkID, objectID, hash, in.Mode, in.PasswordHash, expires, actor.ID, frozen, frozenAt,
		nonNil(in.Released))
	if err != nil {
		return Link{}, "", err
	}
	l, err := db.link(ctx, linkID)
	return l, token, err
}

func (db *DB) link(ctx context.Context, linkID string) (Link, error) {
	l, err := scanLink(db.pool.QueryRow(ctx, `SELECT `+linkColumns+linkJoins+` WHERE l.id = $1`, linkID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Link{}, ErrNotFound
	}
	return l, err
}

// LinkByToken returns the live link of a token: one that is gone, has expired, lies on something in the trash or
// something private, or whose maker no longer manages its object looks like one that never was.
func (db *DB) LinkByToken(ctx context.Context, token string) (Link, error) {
	l, err := scanLink(db.pool.QueryRow(ctx, `SELECT `+linkColumns+linkJoins+` WHERE l.token = $1 AND (l.expires IS NULL OR l.expires > now())
		AND o.deleted_at IS NULL AND object_role(l.object, l.created_by, false) >= 4`, auth.HashToken(token)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Link{}, ErrNotFound
	}
	if err != nil {
		return Link{}, err
	}
	if private, err := isPrivate(ctx, db.pool, l.Object); err != nil || private {
		return Link{}, ErrNotFound
	}
	return l, nil
}

// Links lists share links, newest first: of one object if objectID is set, else all. Administrators see every link;
// everyone else the links they made and those on objects they manage (ADR-29).
func (db *DB) Links(ctx context.Context, actor Actor, objectID string) ([]Link, error) {
	rows, err := db.pool.Query(ctx, `SELECT `+linkColumns+linkJoins+` WHERE ($2 = '' OR l.object = $2) AND o.deleted_at IS NULL
		AND ($3 OR l.created_by = $1 OR object_role(l.object, $1) >= 4) ORDER BY l.created DESC, l.id`, actor.ID, objectID, actor.Admin)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Link, error) { return scanLink(row) })
}

// mayChangeLink tells whether the actor may revoke or renew a link: whoever made it, whoever manages its object, and
// administrators.
func (db *DB) mayChangeLink(ctx context.Context, linkID string, actor Actor) (Link, error) {
	l, err := db.link(ctx, linkID)
	if err != nil {
		return Link{}, err
	}
	if actor.Admin || l.CreatedBy == actor.ID {
		return l, nil
	}
	role, err := db.Role(ctx, l.Object, actor.ID)
	if err != nil {
		return Link{}, err
	}
	if Rank(role) < Rank(RoleManage) {
		if role == RoleNone {
			return Link{}, ErrNotFound
		}
		return Link{}, ErrNotAllowed
	}
	return l, nil
}

// RevokeLink ends a link for good: it answers like one that never was, and whoever opened it to comment or edit loses
// that role.
func (db *DB) RevokeLink(ctx context.Context, linkID string, actor Actor) (Link, error) {
	l, err := db.mayChangeLink(ctx, linkID, actor)
	if err != nil {
		return Link{}, err
	}
	_, err = db.pool.Exec(ctx, "DELETE FROM share_links WHERE id = $1", linkID)
	return l, err
}

// RenewLink freezes a page of a link again, as it is now, with the embeddings released now. Only who manages it does.
func (db *DB) RenewLink(ctx context.Context, linkID string, actor Actor, frozen string, released []string) error {
	l, err := db.mayChangeLink(ctx, linkID, actor)
	if err != nil {
		return err
	}
	if l.ObjectKind != KindPage && l.ObjectKind != KindNotes {
		return ErrLink
	}
	_, err = db.pool.Exec(ctx, "UPDATE share_links SET frozen = $2, frozen_at = now(), released = $3 WHERE id = $1", linkID, frozen, nonNil(released))
	return err
}

// UseLink gives an account the role of a link to comment or edit, from now on and as long as the link lives.
func (db *DB) UseLink(ctx context.Context, linkID, accountID string) error {
	_, err := db.pool.Exec(ctx, `INSERT INTO share_link_users (link, account)
		SELECT id, $2 FROM share_links WHERE id = $1 AND mode IN ('comment', 'edit') ON CONFLICT DO NOTHING`, linkID, accountID)
	return err
}

// passLife is how long a browser that gave the password of a link may read it.
const passLife = 24 * time.Hour

// GivePass records that a browser gave the right password of a link, and returns the value of its cookie.
func (db *DB) GivePass(ctx context.Context, linkID string) (string, error) {
	value, hash, err := auth.NewToken()
	if err != nil {
		return "", err
	}
	if _, err := db.pool.Exec(ctx, "DELETE FROM share_link_passes WHERE expires < now()"); err != nil {
		return "", err
	}
	_, err = db.pool.Exec(ctx, "INSERT INTO share_link_passes (pass, link, expires) VALUES ($1, $2, $3)", hash, linkID, time.Now().Add(passLife))
	return value, err
}

// HasPass tells whether the value of a cookie lets a browser read a link with a password.
func (db *DB) HasPass(ctx context.Context, linkID, value string) bool {
	if value == "" {
		return false
	}
	var ok bool
	err := db.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM share_link_passes WHERE pass = $1 AND link = $2 AND expires > now())",
		auth.HashToken(value), linkID).Scan(&ok)
	return err == nil && ok
}

// Below tells whether an object lies at or below another one, outside the trash, and whether the maker of a link sees it
// now: what a link to a folder may reach. A link never shows more than its maker sees, checked for each object (ADR-29),
// so a folder below that breaks the inheritance and shuts the maker out stays out of the link.
func (db *DB) Below(ctx context.Context, objectID, top, maker string) bool {
	var ok bool
	err := db.pool.QueryRow(ctx, `WITH RECURSIVE up AS (
			SELECT id, parent, deleted_at FROM objects WHERE id = $1
			UNION ALL SELECT p.id, p.parent, p.deleted_at FROM objects p JOIN up ON p.id = up.parent WHERE up.id <> $2
		)
		SELECT EXISTS (SELECT 1 FROM up WHERE id = $2) AND NOT EXISTS (SELECT 1 FROM up WHERE deleted_at IS NOT NULL)
			AND object_role($1, $3, false) > 0`, objectID, top, maker).Scan(&ok)
	return err == nil && ok
}

// Shared is an object as a link shows it.
type Shared struct {
	ID, Title, Kind string
	Size            int64
	Changed         time.Time
}

// SharedFiles lists the folders and files directly inside a folder of a link, by name, for whoever holds the link: those
// the maker of the link sees now. The caller has checked that the folder lies below the object of the link.
func (db *DB) SharedFiles(ctx context.Context, folderID, maker string) ([]Shared, error) {
	rows, err := db.pool.Query(ctx, `SELECT o.id, o.title, o.kind, coalesce((SELECT v.size FROM file_versions v WHERE v.file = o.id
			ORDER BY v.number DESC LIMIT 1), 0), o.changed
		FROM objects o WHERE o.parent = $1 AND `+alive+` AND o.kind IN ('folder', 'file') AND object_role(o.id, $2, false) > 0
		ORDER BY o.kind = 'file', lower(o.title), o.id`, folderID, maker)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Shared, error) {
		var s Shared
		err := row.Scan(&s.ID, &s.Title, &s.Kind, &s.Size, &s.Changed)
		return s, err
	})
}

// SharedFile returns the current version of a file for whoever holds a link to it or to a folder above it: the caller
// has checked that.
func (db *DB) SharedFile(ctx context.Context, fileID string) (FileVersion, error) {
	var v FileVersion
	err := db.pool.QueryRow(ctx, `SELECT v.number, v.blob, v.size, v.name, v.content_type FROM file_versions v JOIN objects o ON o.id = v.file
		WHERE v.file = $1 AND o.kind = 'file' AND o.deleted_at IS NULL ORDER BY v.number DESC LIMIT 1`, fileID).
		Scan(&v.Number, &v.Blob, &v.Size, &v.Name, &v.ContentType)
	if errors.Is(err, pgx.ErrNoRows) {
		return FileVersion{}, ErrNotFound
	}
	return v, err
}

// SharedAttachment returns an attachment of a page for a link that shows the page frozen. The caller has checked that
// the maker of the link sees the page now (Below) and that the frozen page shows the attachment.
func (db *DB) SharedAttachment(ctx context.Context, pageID, attachmentID string) (Attachment, error) {
	return scanAttachment(db.pool.QueryRow(ctx, `SELECT `+attachmentColumns+` FROM attachments WHERE page = $1 AND id = $2`, pageID, attachmentID))
}

// SettleUpload ends an upload whose size was only known as an upper bound when it began, such as one through a link: it
// now has exactly n bytes, all received.
func (db *DB) SettleUpload(ctx context.Context, uploadID, viewer string, n int64) error {
	tag, err := db.pool.Exec(ctx, "UPDATE uploads SET size = $3, received = $3, touched = now() WHERE id = $1 AND owner = $2 AND $3 <= size AND $3 >= 0",
		uploadID, viewer, n)
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrUpload
	}
	return err
}
