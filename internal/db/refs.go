// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"regexp"

	"github.com/jackc/pgx/v5"
)

// EmbedLine is a line of a block that embeds a block or a whole page of another page by its address (ADR-18):
// ::embed{ref="<object id>#<block id>"}, or without "#<block id>" for the whole page. It stands on a line of its own.
var EmbedLine = regexp.MustCompile(`(?m)^[ \t]*::embed\{ref="([0-9a-hjkmnp-tv-z]{10})(?:#(b[1-9][0-9]{0,8}))?"\}[ \t]*$`)

// Ref is one embedding: what a block of a page embeds, or where something is embedded.
type Ref struct {
	Page, Block, Target, TargetBlock string
	// Title is the title of the page on the other side, as the viewer sees it.
	Title string
}

// RefsIn returns what a block's Markdown embeds, each once, in order.
func RefsIn(body string) [][2]string {
	var out [][2]string
	seen := map[[2]string]bool{}
	for _, m := range EmbedLine.FindAllStringSubmatch(body, -1) {
		r := [2]string{m[1], m[2]}
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out
}

// syncRefs writes down what a block embeds now, replacing what it embedded before.
func syncRefs(ctx context.Context, tx pgx.Tx, page, block, body string) error {
	if _, err := tx.Exec(ctx, "DELETE FROM refs WHERE page = $1 AND block = $2", page, block); err != nil {
		return err
	}
	for _, r := range RefsIn(body) {
		if _, err := tx.Exec(ctx, "INSERT INTO refs (page, block, target, target_block) VALUES ($1, $2, $3, $4)", page, block, r[0], r[1]); err != nil {
			return err
		}
	}
	return nil
}

// UsedIn returns where an object, or one of its blocks, is embedded, in pages the viewer sees and that are not in the
// trash: whole-page embeddings and embeddings of single blocks alike.
func (db *DB) UsedIn(ctx context.Context, objectID, viewer string) ([]Ref, error) {
	rows, err := db.pool.Query(ctx, `SELECT r.page, r.block, r.target, r.target_block, o.title FROM refs r JOIN objects o ON o.id = r.page
		WHERE r.target = $2 AND o.deleted_at IS NULL AND `+visible+` ORDER BY o.title, r.page, r.block, r.target_block`, viewer, objectID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Ref, error) {
		var r Ref
		err := row.Scan(&r.Page, &r.Block, &r.Target, &r.TargetBlock, &r.Title)
		return r, err
	})
}

// Uses says where an object, or anything below it, is embedded from outside of it, before it is deleted: the titles of
// the pages the viewer sees, and how many more there are that the viewer may not see, without naming them. Pages in the
// trash embed nothing.
func (db *DB) Uses(ctx context.Context, objectID, viewer string) (titles []string, hidden int, err error) {
	rows, err := db.pool.Query(ctx, `
		WITH RECURSIVE tree AS (
			SELECT id FROM objects WHERE id = $2
			UNION ALL SELECT o.id FROM objects o JOIN tree ON o.parent = tree.id
		)
		SELECT DISTINCT o.id, o.title, `+visible+` AS seen FROM refs r JOIN objects o ON o.id = r.page
		WHERE r.target IN (SELECT id FROM tree) AND r.page NOT IN (SELECT id FROM tree) AND o.deleted_at IS NULL
		ORDER BY o.title, o.id`, viewer, objectID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	titles = []string{}
	for rows.Next() {
		var id, title string
		var seen bool
		if err := rows.Scan(&id, &title, &seen); err != nil {
			return nil, 0, err
		}
		if seen {
			titles = append(titles, title)
		} else {
			hidden++
		}
	}
	return titles, hidden, rows.Err()
}
