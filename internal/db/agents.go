// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/pagekind"
)

// What the MCP access needs of the database (ADR-34). Every function decides with the one check of rights, as the
// browser does; the connection of a request through MCP carries that it is an agent's (account.User.MCP).

// RulesTag marks the rules page: the page of the kind agent notes with this tag holds the conventions agents follow
// (ADR-33, ADR-34). It is a page like any other, written in the browser and checked at every write.
const RulesTag = "rules"

// PageVersion is the newest change of the blocks of a page the viewer sees, 0 for a page nobody wrote yet. A write names
// it as its base, so that it fails when the page changed since it was read (ADR-34). Moving a block inside the page makes
// no version and changes nothing of its text, so it does not count.
func (db *DB) PageVersion(ctx context.Context, pageID, viewer string) (int64, error) {
	if _, err := db.page(ctx, pageID, viewer); err != nil {
		return 0, err
	}
	var version int64
	err := db.pool.QueryRow(ctx, "SELECT coalesce(max(version), 0) FROM block_versions WHERE page = $1", pageID).Scan(&version)
	return version, err
}

// RulesPage returns the rules page the viewer reads: the oldest page of the kind agent notes tagged RulesTag. ErrNotFound
// when there is none, or none the viewer may read.
func (db *DB) RulesPage(ctx context.Context, viewer string) (Object, error) {
	o, err := scan(db.pool.QueryRow(ctx, `SELECT `+columns+` FROM objects o
		WHERE o.kind = 'page' AND o.page_kind = $2 AND $3 = ANY(o.tags) AND `+alive+` AND `+visible+`
		ORDER BY o.created, o.id LIMIT 1`, viewer, pagekind.AgentNotes, RulesTag))
	if errors.Is(err, pgx.ErrNoRows) {
		return Object{}, ErrNotFound
	}
	return o, err
}

// UploadLinkTime is how long a one-time upload link holds (ADR-34).
const UploadLinkTime = 15 * time.Minute

// LinkUpload gives an upload of the viewer, which has received nothing yet, a one-time link and returns its token. Only
// the hash of the token is kept.
func (db *DB) LinkUpload(ctx context.Context, uploadID, viewer string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	tag, err := db.pool.Exec(ctx, `UPDATE uploads SET link_hash = $3, link_until = now() + make_interval(secs => $4)
		WHERE id = $1 AND owner = $2 AND received = 0`, uploadID, viewer, sum[:], UploadLinkTime.Seconds())
	if err == nil && tag.RowsAffected() == 0 {
		err = ErrNotFound
	}
	return token, err
}

// TakeUploadLink finds the upload of a one-time link and takes the link away, in one step, so it works once; a link
// past its time is not found.
func (db *DB) TakeUploadLink(ctx context.Context, token string) (Upload, error) {
	sum := sha256.Sum256([]byte(token))
	var u Upload
	err := db.pool.QueryRow(ctx, `UPDATE uploads SET link_hash = NULL, link_until = NULL, touched = now()
		WHERE link_hash = $1 AND link_until > now() AND received = 0
		RETURNING id, owner, coalesce(parent, ''), coalesce(file, ''), coalesce(page, ''), coalesce(root, ''), name, size, received, touched`, sum[:]).
		Scan(&u.ID, &u.Owner, &u.Parent, &u.File, &u.Page, &u.Root, &u.Name, &u.Size, &u.Received, &u.Touched)
	if errors.Is(err, pgx.ErrNoRows) {
		return Upload{}, ErrNotFound
	}
	return u, err
}
