// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/markdown"
)

// The end of something in use is chosen (ADR-22). Deleting a block, or the text of a part, that something embeds asks,
// and the viewer picks one of three ways, with nothing chosen in advance:
//
//   - everywhere: it goes, and every embedding shows who removed it and when; restoring it brings every use back,
//   - copies: every embedding becomes a copy of it, plain text in its page, in its place,
//   - master: the content moves into one page that embeds it, in place of its embedding, and every other reference is
//     rewritten to the new place; the old address answers with a redirect.
//
// Copies and a new master are written only into pages the viewer may change; an embedding in any other page shows a
// placeholder, and its owner gets a notice.

// The ways.
const (
	WayEverywhere = "everywhere"
	WayCopies     = "copies"
	WayMaster     = "master"
)

// Way is the choice for one thing in use. Master is the place for the new master, "<page>#<block>", one of its uses.
type Way struct {
	Way    string `json:"way"`
	Master string `json:"master,omitempty"`
}

// InUse is the answer to deleting something in use without a way for it: the addresses that need one.
type InUse struct{ Refs []string }

func (e *InUse) Error() string { return "in use elsewhere: " + strings.Join(e.Refs, ", ") }

// ErrWay is a way that cannot be taken: unknown, or a master that is no use the viewer may change.
var ErrWay = errors.New("choose delete everywhere, a copy for every use, or a new master among the uses you may change")

// Removed is what was deleted everywhere while in use.
type Removed struct {
	ID       int64
	Page     string
	Target   string
	Blocks   []Block
	After    string
	By       string
	At       time.Time
	ByName   string
	PageName string
}

// lost is something in use that a change takes away, with what it was.
type lost struct {
	target string
	// blocks are the blocks it took, as they were; for a part the one block holding the part's Markdown with its markers.
	blocks []Block
	after  string
	name   string
}

// usedTargets returns which of the given targets of a page something embeds, from pages not in the trash, leaving out
// embeddings from the block itself.
func usedTargets(ctx context.Context, tx pgx.Tx, page, block string, targets []string) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT r.target_block FROM refs r JOIN objects o ON o.id = r.page
		WHERE r.target = $1 AND r.target_block = ANY($2) AND o.deleted_at IS NULL AND NOT (r.page = $1 AND r.block = $3)
		ORDER BY 1`, page, targets, block)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// lostByEdit finds the parts in use that a new text of a block takes away.
func lostByEdit(ctx context.Context, tx pgx.Tx, page, block, old, body string) ([]lost, error) {
	var targets []string
	parts := map[string]markdown.Part{}
	for _, p := range markdown.Parts(old) {
		if _, still := markdown.FindPart(body, p.ID); !still {
			targets = append(targets, block+"."+p.ID)
			parts[block+"."+p.ID] = p
		}
	}
	if len(targets) == 0 {
		return nil, nil
	}
	used, err := usedTargets(ctx, tx, page, block, targets)
	if err != nil {
		return nil, err
	}
	var out []lost
	for _, t := range used {
		p := parts[t]
		md, err := markdown.PartMarkdown(old, p.ID)
		if err != nil {
			md = markdown.StripParts(p.Text(old))
		}
		out = append(out, lost{target: t, name: p.Name, blocks: []Block{{ID: block, Body: markedPart(p.ID, p.Name, md)}}})
	}
	return out, nil
}

func markedPart(id, name, md string) string {
	open := "<!-- part " + id
	if name != "" {
		open += ` name="` + name + `"`
	}
	return open + " -->" + md + "<!-- /part " + id + " -->"
}

// lostByDelete finds what deleting a block takes away while in use: the block, its parts and the section it opens. They
// go one way together.
func lostByDelete(ctx context.Context, tx pgx.Tx, page, block string) ([]lost, error) {
	all, err := pageBlocks(ctx, tx, page)
	if err != nil {
		return nil, err
	}
	at := slices.IndexFunc(all, func(b Block) bool { return b.ID == block })
	if at < 0 {
		return nil, ErrNotFound
	}
	targets := []string{block, block + "." + Section}
	for _, p := range markdown.Parts(all[at].Body) {
		targets = append(targets, block+"."+p.ID)
	}
	used, err := usedTargets(ctx, tx, page, block, targets)
	if err != nil || len(used) == 0 {
		return nil, err
	}
	after := ""
	if at > 0 {
		after = all[at-1].ID
	}
	var out []lost
	for _, t := range used {
		l := lost{target: t, blocks: []Block{all[at]}, after: after}
		if strings.HasSuffix(t, "."+Section) {
			bodies := make([]string, len(all)-at)
			for i, b := range all[at:] {
				bodies[i] = b.Body
			}
			if n := markdown.SectionEnd(bodies); n > 0 {
				l.blocks = slices.Clone(all[at : at+n])
			}
		} else if t != block {
			id := t[len(block)+1:]
			p, _ := markdown.FindPart(all[at].Body, id)
			md, err := markdown.PartMarkdown(all[at].Body, id)
			if err != nil {
				md = markdown.StripParts(p.Text(all[at].Body))
			}
			l.name = p.Name
			l.blocks = []Block{{ID: block, Body: markedPart(id, p.Name, md)}}
		}
		out = append(out, l)
	}
	return out, nil
}

func pageBlocks(ctx context.Context, tx pgx.Tx, page string) ([]Block, error) {
	rows, err := tx.Query(ctx, "SELECT id, body, coalesce(align, '') FROM blocks WHERE page = $1 ORDER BY position", page)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Block, error) {
		var b Block
		err := row.Scan(&b.ID, &b.Body, &b.Align)
		return b, err
	})
}

// copyOf is what an embedding of a lost thing becomes as a copy: plain Markdown without markers.
func (l lost) copyOf() string {
	if !strings.Contains(l.target, ".") || strings.HasSuffix(l.target, "."+Section) {
		bodies := make([]string, len(l.blocks))
		for i, b := range l.blocks {
			bodies[i] = markdown.StripParts(b.Body)
		}
		return strings.Join(bodies, "\n\n")
	}
	return markdown.StripParts(l.blocks[0].Body)
}

func (l lost) isPart() bool {
	return strings.Contains(l.target, ".") && !strings.HasSuffix(l.target, "."+Section)
}

// embedsOf finds the embeddings of one address in a block's Markdown: lines of ::embed and parts inside paragraphs.
func embedsOf(ref string) (line, inline *regexp.Regexp) {
	q := regexp.QuoteMeta(`{ref="` + ref + `"}`)
	return regexp.MustCompile(`(?m)^[ \t]*::embed` + q + `[ \t]*$`), regexp.MustCompile(`(^|[^:]):embed` + q)
}

// settle takes the chosen way for each thing in use that a change of page takes away, inside the change's transaction.
func (db *DB) settle(ctx context.Context, tx pgx.Tx, page, viewer string, gone []lost, ways map[string]Way) error {
	var missing []string
	for _, l := range gone {
		w, ok := ways[l.target]
		if !ok {
			w, ok = ways["*"]
		}
		if !ok || (w.Way != WayEverywhere && w.Way != WayCopies && w.Way != WayMaster) {
			missing = append(missing, Address{Object: page, Block: BlockOf(l.target), Sub: subOf(l.target)}.String())
		}
	}
	if len(missing) > 0 {
		return &InUse{Refs: missing}
	}
	moved := map[string]Address{}
	mastered := false
	for _, l := range gone {
		w, ok := ways[l.target]
		if !ok {
			w = ways["*"]
		}
		if _, done := moved[l.target]; done {
			continue
		}
		// One choice for all of a block (ways["*"]): its new master is the place of the first thing in use; whatever
		// else of it is in use and did not move with it becomes a copy where it is used.
		if w.Way == WayMaster && mastered && !ok {
			w.Way = WayCopies
		}
		switch w.Way {
		case WayEverywhere:
			if err := keepRemoved(ctx, tx, page, viewer, l); err != nil {
				return err
			}
		case WayCopies:
			if err := db.copyEverywhere(ctx, tx, page, viewer, l); err != nil {
				return err
			}
		case WayMaster:
			to, err := db.newMaster(ctx, tx, page, viewer, l, w.Master)
			if err != nil {
				return err
			}
			mastered = true
			for k, v := range to {
				moved[k] = v
			}
		}
	}
	return nil
}

func subOf(target string) string {
	_, sub, _ := strings.Cut(target, ".")
	return sub
}

func keepRemoved(ctx context.Context, tx pgx.Tx, page, viewer string, l lost) error {
	blocks, err := json.Marshal(l.blocks)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO removed (page, target, blocks, after, removed_by) VALUES ($1, $2, $3, $4, $5)", page, l.target, blocks, l.after, viewer)
	return err
}

// use is one block that embeds a lost thing.
type use struct{ page, block, owner string }

func usesIn(ctx context.Context, tx pgx.Tx, page, target string) ([]use, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT r.page, r.block, o.owner FROM refs r JOIN objects o ON o.id = r.page
		WHERE r.target = $1 AND r.target_block = $2 AND o.deleted_at IS NULL ORDER BY 1, 2`, page, target)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (use, error) {
		var u use
		err := row.Scan(&u.page, &u.block, &u.owner)
		return u, err
	})
}

// rewriteBlock changes the Markdown of a block outside the change at hand, with a version and its references.
func rewriteBlock(ctx context.Context, tx pgx.Tx, page, block, viewer, origin string, change func(string) string) error {
	var body string
	if err := tx.QueryRow(ctx, "SELECT body FROM blocks WHERE page = $1 AND id = $2 FOR UPDATE", page, block).Scan(&body); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	next := change(body)
	if next == body {
		return nil
	}
	if err := checkBody(next); err != nil {
		return err
	}
	if err := roomForBytes(ctx, tx, int64(len(next)-len(body))); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE blocks SET body = $3 WHERE page = $1 AND id = $2", page, block, next); err != nil {
		return err
	}
	if err := syncRefs(ctx, tx, page, block, next); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE objects SET changed = now() WHERE id = $1", page); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "INSERT INTO block_versions (page, block, body, author, origin) VALUES ($1, $2, $3, $4, $5)", page, block, next, viewer, origin)
	return err
}

// copyEverywhere turns every embedding the viewer may change into a copy; every other one keeps a placeholder, and the
// owner of its page gets a notice.
func (db *DB) copyEverywhere(ctx context.Context, tx pgx.Tx, page, viewer string, l lost) error {
	ref := Address{Object: page, Block: BlockOf(l.target), Sub: subOf(l.target)}.String()
	uses, err := usesIn(ctx, tx, page, l.target)
	if err != nil {
		return err
	}
	text := l.copyOf()
	line, inline := embedsOf(ref)
	kept := false
	for _, u := range uses {
		if u.page == page && u.block == BlockOf(l.target) {
			continue
		}
		o, err := db.Object(ctx, u.page, viewer)
		if err != nil || !db.MayEdit(ctx, o, viewer) {
			kept = true
			if err := notify(ctx, tx, u.owner, "copy", u.page, u.block, ref, viewer); err != nil {
				return err
			}
			continue
		}
		if err := rewriteBlock(ctx, tx, u.page, u.block, viewer, "copy of "+ref, func(body string) string {
			body = line.ReplaceAllLiteralString(body, text)
			return inline.ReplaceAllString(body, "${1}"+strings.ReplaceAll(strings.ReplaceAll(text, "$", "$$"), "\n", " "))
		}); err != nil {
			return err
		}
	}
	if kept && l.target != "" {
		return keepRemoved(ctx, tx, page, viewer, l)
	}
	return nil
}

func notify(ctx context.Context, tx pgx.Tx, account, kind, page, block, ref, by string) error {
	if account == by {
		return nil
	}
	_, err := tx.Exec(ctx, "INSERT INTO notices (account, kind, page, block, ref, by) VALUES ($1, $2, $3, $4, $5, $6)", account, kind, page, block, ref, by)
	return err
}

// newMaster moves the content into the page of one of its uses, in place of its embedding, rewrites every other reference
// and keeps redirects. It returns the new address of each target that moved.
func (db *DB) newMaster(ctx context.Context, tx pgx.Tx, page, viewer string, l lost, master string) (map[string]Address, error) {
	at, ok := ParseAddress(master)
	if !ok || at.Block == "" || at.Sub != "" {
		return nil, ErrWay
	}
	uses, err := usesIn(ctx, tx, page, l.target)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(uses, func(u use) bool { return u.page == at.Object && u.block == at.Block })
	if i < 0 || (at.Object == page && at.Block == BlockOf(l.target)) {
		return nil, ErrWay
	}
	o, err := db.Object(ctx, at.Object, viewer)
	if err != nil || !db.MayEdit(ctx, o, viewer) {
		return nil, ErrWay
	}
	if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 FOR UPDATE", at.Object); err != nil {
		return nil, err
	}
	old := Address{Object: page, Block: BlockOf(l.target), Sub: subOf(l.target)}
	line, inline := embedsOf(old.String())
	moved := map[string]Address{}
	origin := "moved from " + old.String()
	if l.isPart() {
		floor, err := nextPartFloor(ctx, tx, at.Object, at.Block)
		if err != nil {
			return nil, err
		}
		var body string
		if err := tx.QueryRow(ctx, "SELECT body FROM blocks WHERE page = $1 AND id = $2", at.Object, at.Block).Scan(&body); err != nil {
			return nil, err
		}
		id := markdown.NextPartID(body, floor)
		p, _ := markdown.FindPart(l.blocks[0].Body, old.Sub)
		marked := markedPart(id, l.name, markdown.StripParts(p.Text(l.blocks[0].Body)))
		moved[l.target] = Address{Object: at.Object, Block: at.Block, Sub: id}
		placed := false
		if err := rewriteBlock(ctx, tx, at.Object, at.Block, viewer, origin, func(body string) string {
			if loc := inline.FindStringSubmatchIndex(body); loc != nil {
				placed = true
				return body[:loc[3]] + marked + body[loc[1]:]
			}
			if loc := line.FindStringIndex(body); loc != nil {
				placed = true
				return body[:loc[0]] + marked + body[loc[1]:]
			}
			return body
		}); err != nil {
			return nil, err
		}
		if !placed {
			return nil, ErrWay
		}
	} else {
		var body string
		if err := tx.QueryRow(ctx, "SELECT body FROM blocks WHERE page = $1 AND id = $2", at.Object, at.Block).Scan(&body); err != nil {
			return nil, err
		}
		loc := line.FindStringIndex(body)
		if loc == nil {
			return nil, ErrWay
		}
		blocks := l.blocks
		after := at.Block
		var ids []string
		if strings.TrimSpace(body[:loc[0]]+body[loc[1]:]) == "" {
			// The embedding is the whole block: the first block of the content takes its place and its ID.
			if err := rewriteBlock(ctx, tx, at.Object, at.Block, viewer, origin, func(string) string { return blocks[0].Body }); err != nil {
				return nil, err
			}
			ids = append(ids, at.Block)
			blocks = blocks[1:]
		} else if err := rewriteBlock(ctx, tx, at.Object, at.Block, viewer, origin, func(body string) string {
			return strings.TrimSpace(body[:loc[0]]) + "\n\n" + strings.TrimSpace(body[loc[1]:])
		}); err != nil {
			return nil, err
		}
		for _, b := range blocks {
			if len(ids) > 0 {
				after = ids[len(ids)-1]
			}
			id, err := insertBlock(ctx, tx, at.Object, after, b.Body, viewer, origin)
			if err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		for i, b := range l.blocks {
			moved[b.ID] = Address{Object: at.Object, Block: ids[i]}
			moved[b.ID+"."+Section] = Address{Object: at.Object, Block: ids[i], Sub: Section}
			for _, p := range markdown.Parts(b.Body) {
				moved[b.ID+"."+p.ID] = Address{Object: at.Object, Block: ids[i], Sub: p.ID}
			}
		}
		// The rest of a section goes from its old page with it.
		for _, b := range l.blocks[1:] {
			if _, err := tx.Exec(ctx, "DELETE FROM blocks WHERE page = $1 AND id = $2", page, b.ID); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx, "INSERT INTO block_versions (page, block, body, author, origin) VALUES ($1, $2, NULL, $3, $4)", page, b.ID, viewer, "moved to "+moved[b.ID].String()); err != nil {
				return nil, err
			}
		}
	}
	if err := redirectAll(ctx, tx, page, viewer, moved); err != nil {
		return nil, err
	}
	if err := notify(ctx, tx, uses[i].owner, "master", at.Object, at.Block, moved[l.target].String(), viewer); err != nil {
		return nil, err
	}
	return moved, nil
}

// insertBlock puts a new block after another inside a change, with its version, and returns its ID.
func insertBlock(ctx context.Context, tx pgx.Tx, page, after, body, viewer, origin string) (string, error) {
	if err := checkBody(body); err != nil {
		return "", err
	}
	if err := roomForBytes(ctx, tx, int64(len(body))); err != nil {
		return "", err
	}
	position := 0
	if after != "" {
		if err := tx.QueryRow(ctx, "SELECT position + 1 FROM blocks WHERE page = $1 AND id = $2", page, after).Scan(&position); err != nil {
			return "", err
		}
	}
	var next int
	if err := tx.QueryRow(ctx, "UPDATE objects SET next_block = next_block + 1, changed = now() WHERE id = $1 RETURNING next_block - 1", page).Scan(&next); err != nil {
		return "", err
	}
	id := "b" + strconv.Itoa(next)
	if _, err := tx.Exec(ctx, "UPDATE blocks SET position = position + 1 WHERE page = $1 AND position >= $2", page, position); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO blocks (page, id, position, body) VALUES ($1, $2, $3, $4)", page, id, position, body); err != nil {
		return "", err
	}
	if err := syncRefs(ctx, tx, page, id, body); err != nil {
		return "", err
	}
	_, err := tx.Exec(ctx, "INSERT INTO block_versions (page, block, body, author, origin) VALUES ($1, $2, $3, $4, $5)", page, id, body, viewer, origin)
	return id, err
}

// redirectAll rewrites every reference to the old addresses, in every page, and keeps a redirect for each, so links and
// bookmarks to the old place keep working (ADR-22). It runs in the transaction of the move.
func redirectAll(ctx context.Context, tx pgx.Tx, page, viewer string, moved map[string]Address) error {
	if len(moved) == 0 {
		return nil
	}
	var targets []string
	for t := range moved {
		targets = append(targets, t)
	}
	slices.Sort(targets)
	rows, err := tx.Query(ctx, "SELECT DISTINCT page, block FROM refs WHERE target = $1 AND target_block = ANY($2) ORDER BY 1, 2", page, targets)
	if err != nil {
		return err
	}
	type at struct{ page, block string }
	places, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (at, error) {
		var x at
		err := row.Scan(&x.page, &x.block)
		return x, err
	})
	if err != nil {
		return err
	}
	replacer := make([]string, 0, 2*len(moved))
	for _, t := range targets {
		old := Address{Object: page, Block: BlockOf(t), Sub: subOf(t)}
		replacer = append(replacer, `ref="`+old.String()+`"`, `ref="`+moved[t].String()+`"`)
	}
	r := strings.NewReplacer(replacer...)
	for _, x := range places {
		if err := rewriteBlock(ctx, tx, x.page, x.block, viewer, "", r.Replace); err != nil {
			return err
		}
	}
	for _, t := range targets {
		to := moved[t]
		if _, err := tx.Exec(ctx, `INSERT INTO redirects (from_page, from_target, to_page, to_target, moved_by) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (tenant, from_page, from_target) DO UPDATE SET to_page = excluded.to_page, to_target = excluded.to_target, moved_by = excluded.moved_by, moved_at = now()`,
			page, t, to.Object, to.Target(), viewer); err != nil {
			return err
		}
	}
	return nil
}

// Redirect follows the redirects of an address to where its content lives now, or reports false.
func (db *DB) Redirect(ctx context.Context, a Address) (Address, bool) {
	moved := false
	for range 10 {
		var to Address
		var target string
		err := db.pool.QueryRow(ctx, "SELECT to_page, to_target FROM redirects WHERE from_page = $1 AND from_target = $2", a.Object, a.Target()).Scan(&to.Object, &target)
		if err != nil {
			break
		}
		to.Block, to.Sub = BlockOf(target), subOf(target)
		a, moved = to, true
	}
	return a, moved
}

// RemovedAt returns what was deleted everywhere at an address, the latest, if it is not back: for a part or the section
// of a block also what went with the whole block. Only in a page the viewer sees.
func (db *DB) RemovedAt(ctx context.Context, a Address, viewer string) (Removed, bool) {
	var r Removed
	var blocks []byte
	err := db.pool.QueryRow(ctx, `SELECT r.id, r.page, r.target, r.blocks, r.after, r.removed_by, r.removed_at, coalesce(a.name, r.removed_by), o.title
		FROM removed r JOIN accounts a ON a.id = r.removed_by JOIN objects o ON o.id = r.page
		WHERE r.page = $1 AND (r.target = $2 OR r.target = $3) AND o.deleted_at IS NULL AND `+strings.ReplaceAll(visible, "$1", "$4")+`
		ORDER BY r.removed_at DESC, r.id DESC LIMIT 1`,
		a.Object, a.Target(), a.Block, viewer).Scan(&r.ID, &r.Page, &r.Target, &blocks, &r.After, &r.By, &r.At, &r.ByName, &r.PageName)
	if err != nil {
		return Removed{}, false
	}
	if err := json.Unmarshal(blocks, &r.Blocks); err != nil {
		return Removed{}, false
	}
	return r, true
}

// RestoreRemoved puts what was deleted everywhere back into its page, so every embedding shows it again: blocks with
// their IDs where they were, a part as a paragraph of its own at the end of its block.
func (db *DB) RestoreRemoved(ctx context.Context, id int64, viewer string) error {
	var page string
	if err := db.pool.QueryRow(ctx, "SELECT page FROM removed WHERE id = $1", id).Scan(&page); err != nil {
		return ErrNotFound
	}
	return db.change(ctx, page, viewer, func(tx pgx.Tx) error {
		var target, after string
		var raw []byte
		if err := tx.QueryRow(ctx, "DELETE FROM removed WHERE id = $1 RETURNING target, blocks, after", id).Scan(&target, &raw, &after); err != nil {
			return ErrNotFound
		}
		var blocks []Block
		if err := json.Unmarshal(raw, &blocks); err != nil {
			return err
		}
		have, err := pageBlocks(ctx, tx, page)
		if err != nil {
			return err
		}
		exists := func(id string) bool { return slices.ContainsFunc(have, func(b Block) bool { return b.ID == id }) }
		if strings.Contains(target, ".") && !strings.HasSuffix(target, "."+Section) {
			if exists(blocks[0].ID) {
				return rewriteBlock(ctx, tx, page, blocks[0].ID, viewer, "restored", func(body string) string {
					return strings.TrimRight(body, "\n") + "\n\n" + blocks[0].Body
				})
			}
			blocks = blocks[:1]
		}
		if !exists(after) {
			after = ""
			if len(have) > 0 {
				after = have[len(have)-1].ID
			}
		}
		for _, b := range blocks {
			if exists(b.ID) {
				after = b.ID
				continue
			}
			position := 0
			if after != "" {
				if err := tx.QueryRow(ctx, "SELECT position + 1 FROM blocks WHERE page = $1 AND id = $2", page, after).Scan(&position); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx, "UPDATE blocks SET position = position + 1 WHERE page = $1 AND position >= $2", page, position); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "INSERT INTO blocks (page, id, position, body, align) VALUES ($1, $2, $3, $4, nullif($5, ''))", page, b.ID, position, b.Body, b.Align); err != nil {
				return err
			}
			if err := syncRefs(ctx, tx, page, b.ID, b.Body); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "INSERT INTO block_versions (page, block, body, author, origin) VALUES ($1, $2, $3, $4, 'restored')", page, b.ID, b.Body, viewer); err != nil {
				return err
			}
			after = b.ID
		}
		return nil
	})
}

// KeepCopy writes, in a page the viewer may change, a copy of what was deleted everywhere in place of its embeddings:
// the choice the owner of a page has when a copy could not be written there.
func (db *DB) KeepCopy(ctx context.Context, id int64, page, viewer string) error {
	var from, target string
	var raw []byte
	if err := db.pool.QueryRow(ctx, "SELECT page, target, blocks FROM removed WHERE id = $1", id).Scan(&from, &target, &raw); err != nil {
		return ErrNotFound
	}
	l := lost{target: target}
	if err := json.Unmarshal(raw, &l.blocks); err != nil {
		return err
	}
	if _, err := db.Object(ctx, from, viewer); err != nil {
		return err
	}
	ref := Address{Object: from, Block: BlockOf(target), Sub: subOf(target)}.String()
	line, inline := embedsOf(ref)
	text := l.copyOf()
	return db.change(ctx, page, viewer, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT DISTINCT block FROM refs WHERE page = $1 AND target = $2 AND target_block = $3", page, from, target)
		if err != nil {
			return err
		}
		blocks, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		if len(blocks) == 0 {
			return ErrNotFound
		}
		for _, b := range blocks {
			if err := rewriteBlock(ctx, tx, page, b, viewer, "copy of "+ref, func(body string) string {
				body = line.ReplaceAllLiteralString(body, text)
				return inline.ReplaceAllString(body, "${1}"+strings.ReplaceAll(strings.ReplaceAll(text, "$", "$$"), "\n", " "))
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

// Notice is something the owner of a page is told (M-16 shows them in the inbox).
type Notice struct {
	ID                     int64
	Kind, Page, Block, Ref string
	By, PageTitle          string
	At                     time.Time
	Read                   bool
}

// Notices returns the latest notices of an account, newest first.
func (db *DB) Notices(ctx context.Context, account string, limit int) ([]Notice, error) {
	rows, err := db.pool.Query(ctx, `SELECT n.id, n.kind, n.page, n.block, n.ref, coalesce(a.name, n.by), coalesce(o.title, ''), n.at, n.read_at IS NOT NULL
		FROM notices n LEFT JOIN accounts a ON a.id = n.by LEFT JOIN objects o ON o.id = n.page
		WHERE n.account = $1 ORDER BY n.at DESC, n.id DESC LIMIT $2`, account, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Notice, error) {
		var n Notice
		err := row.Scan(&n.ID, &n.Kind, &n.Page, &n.Block, &n.Ref, &n.By, &n.PageTitle, &n.At, &n.Read)
		return n, err
	})
}

// String says what a notice is about, in words.
func (n Notice) String() string {
	switch n.Kind {
	case "master":
		return fmt.Sprintf("%s moved %s into %s, which you own", n.By, n.Ref, n.PageTitle)
	default:
		return fmt.Sprintf("%s deleted %s, which %s embeds; keep a copy there or remove the embedding", n.By, n.Ref, n.PageTitle)
	}
}

// TrashedAt says who put a page into the trash and when, for the placeholder of an embedding of it; only for a page the
// viewer could see: of anything else nothing is told, not even that it was there.
func (db *DB) TrashedAt(ctx context.Context, page, viewer string) (by string, at time.Time, ok bool) {
	err := db.pool.QueryRow(ctx, `SELECT coalesce(a.name, o.deleted_by), o.deleted_at FROM objects o LEFT JOIN accounts a ON a.id = o.deleted_by
		WHERE o.id = $2 AND o.deleted_at IS NOT NULL AND `+visible, viewer, page).Scan(&by, &at)
	return by, at, err == nil
}

// DeleteWays puts an object into the trash, as Delete does, after the way chosen for each thing of it that is embedded
// from outside, keyed by its address or "*" for all (ADR-22): everywhere leaves a placeholder that restoring from the trash
// takes away again, copies writes a copy into each page that embeds it, master moves it into one of them. Without a way
// for something in use it answers *InUse.
func (db *DB) DeleteWays(ctx context.Context, objectID, viewer string, ways map[string]Way) error {
	return db.deleteObject(ctx, objectID, viewer, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `WITH RECURSIVE down AS (
				SELECT id FROM objects WHERE id = $1
				UNION ALL SELECT c.id FROM objects c JOIN down ON c.parent = down.id WHERE c.deleted_at IS NULL
			)
			SELECT DISTINCT r.target, r.target_block FROM refs r JOIN objects o ON o.id = r.page
			WHERE r.target IN (SELECT id FROM down) AND r.page NOT IN (SELECT id FROM down) AND o.deleted_at IS NULL
			ORDER BY 1, 2`, objectID)
		if err != nil {
			return err
		}
		used, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) ([2]string, error) {
			var x [2]string
			err := row.Scan(&x[0], &x[1])
			return x, err
		})
		if err != nil || len(used) == 0 {
			return err
		}
		// A thing without a way makes settle answer *InUse, before anything changed.
		byPage := map[string][]string{}
		var pages []string
		for _, u := range used {
			if _, seen := byPage[u[0]]; !seen {
				pages = append(pages, u[0])
			}
			byPage[u[0]] = append(byPage[u[0]], u[1])
		}
		for _, page := range pages {
			if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 FOR UPDATE", page); err != nil {
				return err
			}
			all, err := pageBlocks(ctx, tx, page)
			if err != nil {
				return err
			}
			local := map[string]Way{}
			var gone []lost
			moving := map[string]bool{}
			for _, t := range byPage[page] {
				ref := Address{Object: page, Block: BlockOf(t), Sub: subOf(t)}.String()
				w, ok := ways[ref]
				if !ok {
					w = ways["*"]
				}
				if w.Way == WayEverywhere {
					continue // the trash keeps it, and the placeholder says so
				}
				if t == "" {
					if w.Way == WayMaster {
						return ErrWay // a whole page has no place inside another
					}
					local[t] = w
					gone = append(gone, lost{target: "", blocks: all})
					continue
				}
				local[t] = w
				found, err := lostByDelete(ctx, tx, page, BlockOf(t))
				if err != nil {
					return err
				}
				for _, l := range found {
					if l.target == t {
						gone = append(gone, l)
						if w.Way == WayMaster && !l.isPart() {
							moving[BlockOf(t)] = true
						}
					}
				}
			}
			if err := db.settle(ctx, tx, page, viewer, gone, local); err != nil {
				return err
			}
			// What moved to a new master leaves the page in the trash, so restoring it does not bring it twice.
			for block := range moving {
				if _, err := tx.Exec(ctx, "DELETE FROM blocks WHERE page = $1 AND id = $2", page, block); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Lost says, without changing anything, what in use a change of a block would take away: a new text (body) or, with body
// nil, deleting it. The workspace asks before it saves, so the question comes before the change and not as an error.
func (db *DB) Lost(ctx context.Context, pageID, blockID, viewer string, body *string) ([]string, error) {
	if _, err := db.page(ctx, pageID, viewer); err != nil {
		return nil, err
	}
	var refs []string
	err := pgx.BeginTxFunc(ctx, db.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var gone []lost
		var err error
		if body == nil {
			gone, err = lostByDelete(ctx, tx, pageID, blockID)
		} else {
			var old string
			if err := tx.QueryRow(ctx, "SELECT body FROM blocks WHERE page = $1 AND id = $2", pageID, blockID).Scan(&old); err != nil {
				return ErrNotFound
			}
			gone, err = lostByEdit(ctx, tx, pageID, blockID, old, *body)
		}
		for _, l := range gone {
			refs = append(refs, Address{Object: pageID, Block: BlockOf(l.target), Sub: subOf(l.target)}.String())
		}
		return err
	})
	return refs, err
}

// MoveBlockTo moves a block into another page, after the block named by after or first (ADR-22). Its text, its parts and
// the section it opens go with it under a new block ID there; every reference to them is rewritten in the same
// transaction, and the old addresses redirect. The viewer must be able to change both pages.
func (db *DB) MoveBlockTo(ctx context.Context, pageID, blockID, viewer, toPage, after string) (string, error) {
	if toPage == pageID {
		return blockID, db.MoveBlock(ctx, pageID, blockID, viewer, after)
	}
	to, err := db.page(ctx, toPage, viewer)
	if err != nil {
		return "", err
	}
	if err := db.mayEdit(ctx, to, viewer); err != nil {
		return "", err
	}
	var id string
	err = db.change(ctx, pageID, viewer, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 FOR UPDATE", toPage); err != nil {
			return err
		}
		var b Block
		if err := tx.QueryRow(ctx, "SELECT body, coalesce(align, '') FROM blocks WHERE page = $1 AND id = $2", pageID, blockID).Scan(&b.Body, &b.Align); err != nil {
			return ErrNotFound
		}
		if after != "" {
			var n int
			if err := tx.QueryRow(ctx, "SELECT count(*) FROM blocks WHERE page = $1 AND id = $2", toPage, after).Scan(&n); err != nil || n == 0 {
				return ErrNotFound
			}
		}
		id, err = insertBlock(ctx, tx, toPage, after, b.Body, viewer, "moved from "+Address{Object: pageID, Block: blockID}.String())
		if err != nil {
			return err
		}
		if b.Align != "" {
			if _, err := tx.Exec(ctx, "UPDATE blocks SET align = $3 WHERE page = $1 AND id = $2", toPage, id, b.Align); err != nil {
				return err
			}
		}
		moved := map[string]Address{
			blockID:                 {Object: toPage, Block: id},
			blockID + "." + Section: {Object: toPage, Block: id, Sub: Section},
		}
		for _, p := range markdown.Parts(b.Body) {
			moved[blockID+"."+p.ID] = Address{Object: toPage, Block: id, Sub: p.ID}
		}
		if _, err := tx.Exec(ctx, "DELETE FROM blocks WHERE page = $1 AND id = $2", pageID, blockID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO block_versions (page, block, body, author, origin) VALUES ($1, $2, NULL, $3, $4)", pageID, blockID, viewer, "moved to "+moved[blockID].String()); err != nil {
			return err
		}
		return redirectAll(ctx, tx, pageID, viewer, moved)
	})
	return id, err
}
