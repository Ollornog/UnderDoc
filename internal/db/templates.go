// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// Template is what a new page can start from (M-8): one of UnderDoc's own, or a page offered as a template.
type Template struct {
	ID          string
	Title       string
	Description string
	// Place names where a page offered as a template lies; empty for UnderDoc's own.
	Place string
	// Blocks are the Markdown of UnderDoc's own templates, one block each.
	Blocks []string
}

// BuiltIn is the prefix of the IDs of UnderDoc's own templates.
const BuiltIn = "builtin:"

// BuiltInTemplates are the templates UnderDoc brings, after Confluence and Docmost: a page starts with their blocks.
var BuiltInTemplates = []Template{
	{ID: BuiltIn + "meeting", Title: "Meeting notes", Description: "Date, who came, agenda, decisions and to-dos.",
		Blocks: []string{"**Date:** \n**Present:** ", "## Agenda\n\n1. ", "## Notes", "## Decisions\n\n- ", "## To-dos\n\n- [ ] "}},
	{ID: BuiltIn + "howto", Title: "How-to", Description: "A task in steps, with what is needed first.",
		Blocks: []string{"What this does, in one sentence.", "## Before you start\n\n- ", "## Steps\n\n1. \n2. \n3. ", "## Check that it worked"}},
	{ID: BuiltIn + "runbook", Title: "Runbook", Description: "What to do when something fails, and whom to call.",
		Blocks: []string{"## When to use this", "## Check first\n\n- [ ] ", "## Fix\n\n1. ", "## Escalate\n\n| Who | How | When |\n|---|---|---|\n|  |  |  |"}},
	{ID: BuiltIn + "decision", Title: "Decision", Description: "Context, options, the decision and what follows.",
		Blocks: []string{"## Context", "## Options\n\n1. ", "## Decision", "## Consequences"}},
}

// ErrTemplate is a template that is not there or not offered as one.
var ErrTemplate = errors.New("no such template")

// Templates lists what a new page can start from, for the viewer: UnderDoc's own first, then the pages offered as
// templates that the viewer sees, by title.
func (db *DB) Templates(ctx context.Context, viewer string) ([]Template, error) {
	out := append([]Template{}, BuiltInTemplates...)
	pages, err := db.list(ctx, `SELECT `+columns+` FROM objects o WHERE template AND kind = 'page' AND `+alive+` AND `+visible+`
		ORDER BY lower(title), id`, viewer)
	if err != nil {
		return nil, err
	}
	for _, p := range pages {
		out = append(out, Template{ID: p.ID, Title: p.Title, Description: p.Description, Place: db.placeOf(ctx, p, viewer)})
	}
	return out, nil
}

// placeOf names the collections and folders above an object, outermost first.
func (db *DB) placeOf(ctx context.Context, o Object, viewer string) string {
	var names []string
	for parent := o.Parent; parent != "" && len(names) < 20; {
		p, err := db.Object(ctx, parent, viewer)
		if err != nil {
			break
		}
		names = append([]string{p.Title}, names...)
		parent = p.Parent
	}
	return strings.Join(names, " › ")
}

// SetTemplate offers a page as a template, or stops offering it. Whoever may change the page decides (ADR-20).
func (db *DB) SetTemplate(ctx context.Context, pageID, viewer string, on bool) (Object, error) {
	o, err := db.page(ctx, pageID, viewer)
	if err == nil && o.Kind != KindPage {
		err = ErrNotFound
	}
	if err != nil {
		return Object{}, err
	}
	if err := db.mayDo(ctx, o, viewer, ActWrite); err != nil {
		return Object{}, err
	}
	return scan(db.pool.QueryRow(ctx, "UPDATE objects SET template = $2 WHERE id = $1 RETURNING "+columns, pageID, on))
}

// CreatePageFrom makes a page inside parent that starts with the blocks of a template: one of UnderDoc's own, or a page
// the owner sees that is offered as a template. The blocks are new, numbered from b1, each with a version by the owner;
// the page is not a template itself. An empty template makes an empty page, as CreatePage does.
func (db *DB) CreatePageFrom(ctx context.Context, owner account.User, title, parent, template string) (Object, error) {
	if template == "" {
		return db.CreatePage(ctx, owner, title, parent)
	}
	var bodies []string
	if strings.HasPrefix(template, BuiltIn) {
		for _, t := range BuiltInTemplates {
			if t.ID == template {
				bodies = t.Blocks
			}
		}
		if bodies == nil {
			return Object{}, ErrTemplate
		}
	} else {
		from, err := db.page(ctx, template, owner.ID)
		if errors.Is(err, ErrNotFound) || err == nil && (!from.Template || from.Kind != KindPage) {
			return Object{}, ErrTemplate
		}
		if err != nil {
			return Object{}, err
		}
		blocks, err := db.Blocks(ctx, from.ID, owner.ID)
		if err != nil {
			return Object{}, err
		}
		for _, b := range blocks {
			bodies = append(bodies, b.Body)
		}
	}
	p, err := db.Object(ctx, parent, owner.ID)
	if err != nil {
		return Object{}, err
	}
	if err := db.mayDo(ctx, p, owner.ID, ActCreate); err != nil {
		return Object{}, err
	}
	var made Object
	err = pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		var err error
		if made, err = insertObject(ctx, tx, Object{Kind: KindPage, Title: title, Owner: owner.ID, Parent: parent}); err != nil {
			return err
		}
		for i, body := range bodies {
			if err := checkBody(body); err != nil {
				return err
			}
			id := "b" + strconv.Itoa(i+1)
			if _, err := tx.Exec(ctx, "INSERT INTO blocks (page, id, position, body) VALUES ($1, $2, $3, $4)", made.ID, id, i, body); err != nil {
				return err
			}
			if err := syncRefs(ctx, tx, made.ID, id, body); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "INSERT INTO block_versions (page, block, body, author) VALUES ($1, $2, $3, $4)", made.ID, id, body, owner.ID); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, "UPDATE objects SET next_block = $2 WHERE id = $1", made.ID, len(bodies)+1); err != nil {
			return err
		}
		return withinStorage(ctx, tx)
	})
	return made, err
}
