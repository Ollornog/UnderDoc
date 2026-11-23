// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/markdown"
	"github.com/Ollornog/UnderDoc/internal/pagekind"
)

// Page kinds and their writing rules (T-20, ADR-33). The rules are package pagekind's; here they meet the pages: the
// errors block a write of every door (AddBlock, EditBlock), and Findings gathers what a page breaks, with the values of
// references and records the viewer reads, for the answer of a write, the page and the list of problems.

// ErrKind is a kind that is not one, or a kind on something that is not a page.
var ErrKind = errors.New("this is not a kind of page: choose one of " + strings.Join(pagekind.Kinds, ", ") + ", or none")

// ErrRule is the error every write that breaks a blocking rule wraps: a *RuleError holds the findings.
var ErrRule = errors.New("the text breaks a writing rule")

// RuleError is a write refused by the rules: the errors with their fixes.
type RuleError struct{ Findings []pagekind.Finding }

func (e *RuleError) Error() string {
	var words []string
	for _, f := range e.Findings {
		words = append(words, f.Message+" "+f.Fix)
	}
	return strings.Join(words, " ")
}

func (e *RuleError) Unwrap() error { return ErrRule }

// refuse checks the text of a block against the rules that block a write.
func refuse(blockID, body string) error {
	if found := pagekind.Refuse(blockID, body); len(found) > 0 {
		return &RuleError{Findings: found}
	}
	return nil
}

// cleanKind reads a kind as given (pagekind.Parse) for a page.
func cleanKind(kind string) (string, error) {
	k, ok := pagekind.Parse(kind)
	if !ok {
		return "", ErrKind
	}
	return k, nil
}

// SetPageKind gives a page its kind, or none; whoever may change the page may.
func (db *DB) SetPageKind(ctx context.Context, pageID, viewer, kind string) (Object, error) {
	k, err := cleanKind(kind)
	if err != nil {
		return Object{}, err
	}
	o, err := db.Object(ctx, pageID, viewer)
	if err != nil {
		return Object{}, err
	}
	if o.Kind != KindPage {
		return Object{}, ErrKind
	}
	if err := db.mayDo(ctx, o, viewer, ActWrite); err != nil {
		return Object{}, err
	}
	return scan(db.pool.QueryRow(ctx, "UPDATE objects SET page_kind = $2, changed = now() WHERE id = $1 RETURNING "+columns, pageID, k))
}

// maxValueBlocks is how many blocks of references and records the check reads for copied values.
const maxValueBlocks = 5000

// values returns what the references and records the viewer reads hold, as COPY compares it: each value with the
// address of its block, or of the part it stands in. The pages it checks are how-tos and explanations, never among them.
func (db *DB) values(ctx context.Context, viewer string) ([]pagekind.Value, error) {
	rows, err := db.pool.Query(ctx, `SELECT b.page, o.title, b.id, b.body FROM blocks b JOIN objects o ON o.id = b.page
		WHERE o.kind = 'page' AND o.page_kind IN ('reference', 'record') AND `+alive+` AND `+visible+`
		ORDER BY o.id, b.position LIMIT $2`, viewer, maxValueBlocks)
	if err != nil {
		return nil, err
	}
	blocks, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (PageBlock, error) {
		var p PageBlock
		err := row.Scan(&p.Page, &p.Title, &p.Block, &p.Body)
		return p, err
	})
	if err != nil {
		return nil, err
	}
	var out []pagekind.Value
	for _, b := range blocks {
		parts := markdown.Parts(b.Body)
		for _, v := range pagekind.Values(b.Body) {
			address := Address{Object: b.Page, Block: b.Block}
			for _, p := range parts {
				if strings.Contains(p.Text(b.Body), v) {
					address.Sub = p.ID
					break
				}
			}
			out = append(out, pagekind.Value{Text: v, Address: address.String(), Title: b.Title})
		}
	}
	return out, nil
}

// Findings runs the rules of its kind on a page the viewer sees: what it breaks, with the copied values of the
// references and records the viewer reads, and the embeddings that find nothing for the viewer. A board of quick notes
// has no kind and no rules.
func (db *DB) Findings(ctx context.Context, pageID, viewer string) ([]pagekind.Finding, error) {
	o, err := db.page(ctx, pageID, viewer)
	if err != nil || o.Kind != KindPage {
		return nil, err
	}
	blocks, err := db.Blocks(ctx, pageID, viewer)
	if err != nil {
		return nil, err
	}
	var values []pagekind.Value
	if o.PageKind == pagekind.HowTo || o.PageKind == pagekind.Explanation {
		if values, err = db.values(ctx, viewer); err != nil {
			return nil, err
		}
	}
	return db.findingsOf(ctx, o, blocks, values, viewer, map[string]*embedTarget{}), nil
}

// findingsOf runs the rules on a page whose blocks are read, values those of the other pages.
func (db *DB) findingsOf(ctx context.Context, o Object, blocks []Block, values []pagekind.Value, viewer string, targets map[string]*embedTarget) []pagekind.Finding {
	p := pagekind.Page{Kind: o.PageKind, Description: o.Description}
	for _, b := range blocks {
		p.Blocks = append(p.Blocks, pagekind.Block{ID: b.ID, Body: b.Body})
	}
	found := pagekind.Check(p, values)
	for _, b := range blocks {
		for _, r := range RefsIn(fencedCode.ReplaceAllString(b.Body, "")) {
			a := Address{Object: r[0], Block: BlockOf(r[1])}
			if a.Block != r[1] {
				a.Sub = r[1][len(a.Block)+1:]
			}
			if IsTableID(a.Block) || db.resolves(ctx, a, viewer, targets) {
				continue
			}
			f := pagekind.New(pagekind.CodeEmbed, b.ID, a.String())
			f.Source = a.String()
			found = append(found, f)
		}
	}
	return found
}

// embedTarget is a page as far as an embedding needs it: whether the viewer sees it, and its blocks.
type embedTarget struct {
	ok     bool
	blocks map[string]string
}

// resolves tells whether an address finds something for the viewer: a page it sees, a block in it, a part in the block;
// a section only needs its block. A moved part or block counts, as its redirect leads on.
func (db *DB) resolves(ctx context.Context, a Address, viewer string, targets map[string]*embedTarget) bool {
	t, ok := targets[a.Object]
	if !ok {
		t = &embedTarget{blocks: map[string]string{}}
		if o, err := db.Object(ctx, a.Object, viewer); err == nil && o.Kind == KindPage {
			t.ok = true
			if bs, err := db.Blocks(ctx, a.Object, viewer); err == nil {
				for _, b := range bs {
					t.blocks[b.ID] = b.Body
				}
			}
		}
		targets[a.Object] = t
	}
	if !t.ok {
		return false
	}
	if a.Block == "" {
		return true
	}
	body, there := t.blocks[a.Block]
	if there && (a.Sub == "" || a.Sub == Section) {
		return true
	}
	if there {
		if _, found := markdown.FindPart(body, a.Sub); found {
			return true
		}
	}
	_, moved := db.Redirect(ctx, a)
	return moved
}

// PageFindings are the findings of one page, for the list of problems.
type PageFindings struct {
	Page, Title, Kind string
	Findings          []pagekind.Finding
}

// maxRulePages is how many pages and folders the list of problems checks at most.
const maxRulePages = 2000

// RuleProblems runs the rules over the pages and folders the viewer sees (M-3, T-20): every page that breaks a rule of
// its kind, and folders without an overview page or with too many pages. more tells that there were more pages than
// were checked.
func (db *DB) RuleProblems(ctx context.Context, viewer string) (out []PageFindings, more bool, err error) {
	pages, err := db.list(ctx, `SELECT `+columns+` FROM objects o WHERE kind = 'page' AND area = 'docs' AND `+alive+` AND `+visible+`
		ORDER BY lower(title), id LIMIT $2`, viewer, maxRulePages+1)
	if err != nil {
		return nil, false, err
	}
	if len(pages) > maxRulePages {
		pages, more = pages[:maxRulePages], true
	}
	values, err := db.values(ctx, viewer)
	if err != nil {
		return nil, false, err
	}
	targets := map[string]*embedTarget{}
	for _, p := range pages {
		blocks, err := db.Blocks(ctx, p.ID, viewer)
		if err != nil {
			return nil, false, err
		}
		if found := db.findingsOf(ctx, p, blocks, values, viewer, targets); len(found) > 0 {
			out = append(out, PageFindings{Page: p.ID, Title: p.Title, Kind: p.PageKind, Findings: found})
		}
	}
	rows, err := db.pool.Query(ctx, `SELECT f.id, f.title, count(p.id), coalesce(bool_or(p.page_kind = 'overview'), false)
		FROM objects f LEFT JOIN objects p ON p.parent = f.id AND p.kind = 'page' AND p.deleted_at IS NULL AND object_may(p.id, $1, 'read')
		WHERE f.kind = 'folder' AND f.area = 'docs' AND f.deleted_at IS NULL AND object_may(f.id, $1, 'read')
		GROUP BY f.id, f.title ORDER BY lower(f.title), f.id LIMIT $2`, viewer, maxRulePages)
	if err != nil {
		return nil, false, err
	}
	type folder struct {
		id, title string
		pages     int
		overview  bool
	}
	folders, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (folder, error) {
		var f folder
		err := row.Scan(&f.id, &f.title, &f.pages, &f.overview)
		return f, err
	})
	if err != nil {
		return nil, false, err
	}
	for _, f := range folders {
		var found []pagekind.Finding
		if f.pages >= 2 && !f.overview {
			found = append(found, pagekind.New(pagekind.CodeOverview, ""))
		}
		if f.pages > pagekind.MaxPagesInFolder {
			found = append(found, pagekind.New(pagekind.CodeFolderSize, "", f.pages))
		}
		if len(found) > 0 {
			out = append(out, PageFindings{Page: f.id, Title: f.title, Kind: KindFolder, Findings: found})
		}
	}
	return out, more, nil
}
