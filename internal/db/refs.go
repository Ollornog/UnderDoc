// SPDX-License-Identifier: MIT

package db

import (
	"cmp"
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/markdown"
)

// EmbedLine is a line of a block that embeds something of another page by its address (ADR-18, ADR-22):
// ::embed{ref="<object id>#<block id>"} a block, without "#<block id>" the whole page, with ".<part id>" after the block a
// part inside a paragraph, with ".section" the section the block opens. It stands on a line of its own.
var EmbedLine = regexp.MustCompile(`(?m)^[ \t]*::embed\{ref="([0-9a-hjkmnp-tv-z]{10})(?:#(b[1-9][0-9]{0,8})(?:\.(t[1-9][0-9]{0,8}|section))?)?"\}[ \t]*$`)

// Section is what follows a block ID in the address of the section it opens.
const Section = "section"

// Address is what an embedding refers to: an object, a block of it, or a part or the section of a block.
type Address struct {
	Object, Block string
	// Sub is a part ID (tN), Section, or empty.
	Sub string
}

var addressForm = regexp.MustCompile(`^([0-9a-hjkmnp-tv-z]{10})(?:#(b[1-9][0-9]{0,8})(?:\.(t[1-9][0-9]{0,8}|section))?)?$`)

// ParseAddress reads an address such as 7k2m9q4x1c#b3.t2.
func ParseAddress(s string) (Address, bool) {
	m := addressForm.FindStringSubmatch(s)
	if m == nil {
		return Address{}, false
	}
	return Address{Object: m[1], Block: m[2], Sub: m[3]}, true
}

// Target is the address within its object, as refs keeps it: "", "b3", "b3.t2" or "b3.section".
func (a Address) Target() string {
	if a.Sub != "" {
		return a.Block + "." + a.Sub
	}
	return a.Block
}

func (a Address) String() string {
	if a.Block == "" {
		return a.Object
	}
	return a.Object + "#" + a.Target()
}

// BlockOf is the block of a target as refs keeps it: "b3" for "b3.t2".
func BlockOf(target string) string {
	block, _, _ := strings.Cut(target, ".")
	return block
}

// Ref is one embedding: what a block of a page embeds, or where something is embedded.
type Ref struct {
	Page, Block, Target, TargetBlock string
	// Title is the title of the page on the other side, as the viewer sees it.
	Title string
}

var (
	fencedCode = regexp.MustCompile("(?s)(?m)^[ \t]*(```|~~~).*?^[ \t]*(```|~~~)[ \t]*$")
	inlineCode = regexp.MustCompile("`+[^`]*`+")
)

// RefsIn returns what a block's Markdown embeds, each once, in order: the lines of ::embed, and the parts embedded inside
// a paragraph with :embed (markdown.InlineEmbed), not in code.
func RefsIn(body string) [][2]string {
	var out [][2]string
	seen := map[[2]string]bool{}
	add := func(r [2]string) {
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	for _, m := range EmbedLine.FindAllStringSubmatch(body, -1) {
		add([2]string{m[1], Address{Block: m[2], Sub: m[3]}.Target()})
	}
	if strings.Contains(body, ":embed{") {
		text := inlineCode.ReplaceAllString(fencedCode.ReplaceAllString(body, ""), "")
		for _, m := range markdown.InlineEmbed.FindAllStringSubmatch(text, -1) {
			add([2]string{m[1], m[2] + "." + m[3]})
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

// Embed is one thing a page embeds, for its facts panel (M-3): the address, the title of the page it lives in, and for a
// part the name it carries if it is a value.
type Embed struct {
	Address Address
	Title   string
	// Name is the name of a part that is a value; empty for everything else.
	Name string
}

// Embeds returns what a page embeds, each once, in the order of its blocks, from pages the viewer sees that are not in the
// trash. A part that is gone, or a block, has no name.
func (db *DB) Embeds(ctx context.Context, pageID, viewer string) ([]Embed, error) {
	rows, err := db.pool.Query(ctx, `SELECT DISTINCT ON (r.target, r.target_block) r.target, r.target_block, o.title, coalesce(b.body, ''), p.position
		FROM refs r JOIN objects o ON o.id = r.target JOIN blocks p ON p.page = r.page AND p.id = r.block
		LEFT JOIN blocks b ON b.page = r.target AND b.id = split_part(r.target_block, '.', 1)
		WHERE r.page = $2 AND o.deleted_at IS NULL AND `+visible+`
		ORDER BY r.target, r.target_block, p.position`, viewer, pageID)
	if err != nil {
		return nil, err
	}
	type row struct {
		embed    Embed
		position int64
	}
	found, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var x row
		var target, body string
		err := r.Scan(&x.embed.Address.Object, &target, &x.embed.Title, &body, &x.position)
		x.embed.Address.Block, x.embed.Address.Sub = BlockOf(target), subOf(target)
		if sub := x.embed.Address.Sub; sub != "" && sub != Section {
			if p, ok := markdown.FindPart(body, sub); ok {
				x.embed.Name = p.Name
			}
		}
		return x, err
	})
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(found, func(a, b row) int { return cmp.Compare(a.position, b.position) })
	out := make([]Embed, len(found))
	for i, x := range found {
		out[i] = x.embed
	}
	return out, nil
}

// Reused is something of a page that other pages embed, for the Resources tab (M-3): its address, the page it lives in,
// how many places use it in pages the viewer sees, and the Markdown of its block.
type Reused struct {
	Address Address
	Title   string
	Uses    int
	Body    string
}

// Reused lists the blocks, sections and parts the viewer sees that pages the viewer sees embed, by page title and address.
// Pages the viewer may not see are not counted, nor told.
func (db *DB) Reused(ctx context.Context, viewer string, limit int) ([]Reused, error) {
	rows, err := db.pool.Query(ctx, `SELECT r.target, r.target_block, min(o.title), count(*), coalesce(min(b.body), '')
		FROM refs r JOIN objects o ON o.id = r.target
		LEFT JOIN blocks b ON b.page = r.target AND b.id = split_part(r.target_block, '.', 1)
		WHERE r.target_block <> '' AND `+alive+` AND `+visible+`
		  AND r.page IN (SELECT o.id FROM objects o WHERE `+alive+` AND `+visible+`)
		GROUP BY r.target, r.target_block ORDER BY lower(min(o.title)), r.target, r.target_block LIMIT $2`, viewer, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Reused, error) {
		var x Reused
		var target string
		err := row.Scan(&x.Address.Object, &target, &x.Title, &x.Uses, &x.Body)
		x.Address.Block, x.Address.Sub = BlockOf(target), subOf(target)
		return x, err
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
