// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"

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
