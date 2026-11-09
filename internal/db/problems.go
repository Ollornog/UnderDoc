// SPDX-License-Identifier: MIT

package db

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// PageBlock is a block with the page it belongs to, for the list of problems.
type PageBlock struct {
	Page, Title, Block, Body string
}

// AllBlocks returns the blocks of the pages the viewer sees and that are not in the trash, page by page, at most limit.
func (db *DB) AllBlocks(ctx context.Context, viewer string, limit int) ([]PageBlock, error) {
	rows, err := db.pool.Query(ctx, `SELECT b.page, o.title, b.id, b.body FROM blocks b JOIN objects o ON o.id = b.page
		WHERE `+alive+` AND `+visible+` ORDER BY lower(o.title), o.id, b.position LIMIT $2`, viewer, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (PageBlock, error) {
		var p PageBlock
		err := row.Scan(&p.Page, &p.Title, &p.Block, &p.Body)
		return p, err
	})
}
