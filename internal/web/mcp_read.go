// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/library"
	"github.com/Ollornog/UnderDoc/internal/markdown"
	"github.com/Ollornog/UnderDoc/internal/module"
	"github.com/Ollornog/UnderDoc/internal/pagekind"
	"github.com/Ollornog/UnderDoc/internal/search"
	"github.com/Ollornog/UnderDoc/internal/transfer"
)

// The tools that read (ADR-34): the start context, reading pages and sections, "used in" and the search.

type contextIn struct {
	Depth int    `json:"depth,omitempty" jsonschema:"how deep the tree goes, 1 to 4; 2 unless given"`
	Under string `json:"under,omitempty" jsonschema:"an address to show the tree below instead of from the top"`
}

// context answers who the agent acts for, what it may, the rules page, open hints and the tree.
func (t *mcpTools) context(_ context.Context, _ *mcp.CallToolRequest, in contextIn) (*mcp.CallToolResult, any, error) {
	ctx, database, user := t.ctx(), t.h.options.DB, t.user()
	depth := in.Depth
	if depth <= 0 {
		depth = mcpDepth
	}
	depth = min(depth, mcpMaxDepth)
	var b strings.Builder
	fmt.Fprintf(&b, "You act for %s (%s)", user.Name, user.Login)
	if len(user.Groups) > 0 {
		fmt.Fprintf(&b, ", groups of the gate: %s", strings.Join(user.Groups, ", "))
	}
	b.WriteString(".\nAs an agent you never manage rights, make share links, delete for good or read passwords")
	if user.Admin || user.Spaces {
		b.WriteString("; you may make spaces at the top")
	} else {
		b.WriteString(", and make no space at the top")
	}
	b.WriteString(".\n")
	if limit, err := database.AgentLimit(ctx, user.ID); err == nil {
		fmt.Fprintf(&b, "Your person lets agents: %s (and only where they may themselves).\n", strings.Join(limit, ", "))
	}
	if rules, err := database.RulesPage(ctx, user.ID); err == nil {
		fmt.Fprintf(&b, "Rules page: %s %q. Read it before you write.\n", uri(db.Address{Object: rules.ID}), rules.Title)
	} else {
		fmt.Fprintf(&b, "Rules page: none yet. It is a page of the kind agent-notes tagged %q; ask a person to write one.\n", db.RulesTag)
	}
	if pages, err := database.PagesToCheck(ctx, user.ID); err == nil && len(pages) > 0 {
		fmt.Fprintf(&b, "Open hints: %d pages of human documentation have changes by agents a person has not checked yet.\n", len(pages))
	}
	if notices, err := database.Notices(ctx, user.ID, mcpMaxRows); err == nil {
		unread := 0
		for _, n := range notices {
			if !n.Read {
				unread++
			}
		}
		if unread > 0 {
			fmt.Fprintf(&b, "Open hints: %d unread notices of your person about embedded content that moved or went.\n", unread)
		}
	}
	tree := &treeWriter{t: t, b: &b, depth: depth}
	if in.Under != "" {
		ref, err := parseRef(in.Under)
		if err != nil || ref.Folder != "" || ref.Address.Block != "" || ref.FilePage != 0 {
			return refusal("under: " + errRef.Error() + ", without a block"), nil, nil
		}
		o, err := database.Object(ctx, ref.Address.Object, user.ID)
		if err != nil {
			return t.failure(err, "read the tree"), nil, nil
		}
		fmt.Fprintf(&b, "\nTree below %s (depth %d):\n", uri(db.Address{Object: o.ID}), depth)
		tree.node(o, 0)
	} else {
		fmt.Fprintf(&b, "\nTree (depth %d):\n", depth)
		collections, err := database.Collections(ctx, user.ID)
		if err != nil {
			return t.failure(err, "read the tree"), nil, nil
		}
		for _, c := range collections {
			tree.node(c, 0)
		}
		if t.h.moduleOn(ctx, module.Files) && t.h.options.Store != nil {
			if roots, err := database.FileRoots(ctx, user.ID); err == nil && len(roots) > 0 {
				b.WriteString("Files:\n")
				for _, root := range roots {
					tree.node(root.Object, 0)
				}
			}
		}
		if lib := t.h.library(ctx); len(lib.Tree().Root().Documents) > 0 || len(lib.Tree().Root().Folders) > 0 {
			b.WriteString("The folder source (read only) is found by search, its pages as underdoc:folder/<id>.\n")
		}
	}
	if tree.cut > 0 {
		fmt.Fprintf(&b, "… %d more entries; ask with under for a part of the tree.\n", tree.cut)
	}
	return text(clip(b.String())), nil, nil
}

// treeWriter writes the tree, one line per entry, indented, at most mcpMaxTree entries.
type treeWriter struct {
	t      *mcpTools
	b      *strings.Builder
	depth  int
	lines  int
	cut    int
	failed bool
}

func (w *treeWriter) node(o db.Object, level int) {
	if db.IsDivider(o.Kind) {
		return
	}
	if w.lines >= mcpMaxTree {
		w.cut++
		return
	}
	w.lines++
	line := fmt.Sprintf("%s- [%s] %s %s", strings.Repeat("  ", level), o.Kind, oneLine(o.Title, 120), uri(db.Address{Object: o.ID}))
	if o.PageKind != "" {
		line += " (" + o.PageKind + ")"
	}
	if len(o.Tags) > 0 {
		line += " #" + strings.Join(o.Tags, " #")
	}
	kids, err := w.t.h.options.DB.Children(w.t.ctx(), o.ID, w.t.user().ID)
	if err != nil {
		w.failed = true
		return
	}
	kids = slices.DeleteFunc(kids, func(k db.Object) bool { return db.IsDivider(k.Kind) })
	if level+1 >= w.depth {
		if len(kids) > 0 {
			line += fmt.Sprintf(" · %d inside", len(kids))
		}
		w.b.WriteString(line + "\n")
		return
	}
	w.b.WriteString(line + "\n")
	for _, k := range kids {
		w.node(k, level+1)
	}
}

type readIn struct {
	Pages []string `json:"pages" jsonschema:"addresses of 1 to 5 pages; one with #b3 reads only that block, or the section it opens"`
}

// read answers pages, a section or a block as Markdown with front matter and markers, with the version a write names.
func (t *mcpTools) read(_ context.Context, _ *mcp.CallToolRequest, in readIn) (*mcp.CallToolResult, any, error) {
	if len(in.Pages) == 0 || len(in.Pages) > mcpMaxPages {
		return refusal(fmt.Sprintf("pages: name 1 to %d addresses.", mcpMaxPages)), nil, nil
	}
	var b strings.Builder
	for i, address := range in.Pages {
		if i > 0 {
			b.WriteString("\n\n")
		}
		ref, err := parseRef(address)
		if err != nil {
			return refusal(address + ": " + err.Error()), nil, nil
		}
		var part string
		var failed *mcp.CallToolResult
		switch {
		case ref.Folder != "":
			part, failed = t.readFolder(ref)
		case ref.FilePage != 0:
			return refusal(address + ": a page of a file is read with read_file_page."), nil, nil
		default:
			part, failed = t.readPage(ref.Address, len(in.Pages))
		}
		if failed != nil {
			return failed, nil, nil
		}
		b.WriteString(part)
	}
	return text(clip(b.String())), nil, nil
}

// readPage is one page, block or section of the database for read.
func (t *mcpTools) readPage(a db.Address, of int) (string, *mcp.CallToolResult) {
	ctx, database, user := t.ctx(), t.h.options.DB, t.user()
	o, err := database.Object(ctx, a.Object, user.ID)
	if err == nil && !db.HasBlocks(o.Kind) {
		return "", refusal(uri(a) + " is a " + o.Kind + ", not a page: context with under lists what is inside it.")
	}
	if err != nil {
		return "", t.failure(err, "read")
	}
	blocks, err := database.Blocks(ctx, o.ID, user.ID)
	if err != nil {
		return "", t.failure(err, "read")
	}
	version, err := database.PageVersion(ctx, o.ID, user.ID)
	if err != nil {
		return "", t.failure(err, "read")
	}
	t.h.noteOpen(t.r, o)
	var b strings.Builder
	fmt.Fprintf(&b, "page: %s · version: %d · audience: %s", uri(db.Address{Object: o.ID}), version, pagekind.Audience(o.PageKind))
	if place := t.h.place(t.r, o); place != "" {
		fmt.Fprintf(&b, " · in: %s", place)
	}
	b.WriteString("\n")
	if used := t.usedLines(o.ID, ""); used != "" {
		b.WriteString("used in: " + used + "\n")
	}
	if a.Sub != "" && a.Sub != db.Section {
		return "", refusal(uri(a) + ": a part is read with its block, " + uri(db.Address{Object: a.Object, Block: a.Block}) + ".")
	}
	if a.Block != "" {
		at := slices.IndexFunc(blocks, func(x db.Block) bool { return x.ID == a.Block })
		if at < 0 {
			if to, moved := database.Redirect(ctx, a); moved {
				return "", refusal(fmt.Sprintf("%s moved to %s: read that.", uri(a), uri(to)))
			}
			return "", refusal(uri(a) + ": no such block in the page.")
		}
		n := 1
		bodies := make([]string, 0, len(blocks)-at)
		for _, x := range blocks[at:] {
			bodies = append(bodies, x.Body)
		}
		if end := markdown.SectionEnd(bodies); end > 0 {
			n = end
		}
		what := "block"
		if isHeading(bodies[0]) {
			what = "section"
		}
		fmt.Fprintf(&b, "%s %s of %q\n", what, uri(a), o.Title)
		for _, x := range blocks[at : at+n] {
			b.WriteString(blockText(x))
		}
		return b.String(), nil
	}
	raw, err := transfer.PageMarkdown(&db.Node{Object: o, Blocks: blocks})
	if err != nil {
		return "", t.failure(err, "read")
	}
	// A page too long for one answer, or for a share of one when several are read, comes as its outline.
	if len(raw) > mcpMaxAnswer/of {
		fmt.Fprintf(&b, "The page has %d KB, more than one answer takes. Read it by section:\n", len(raw)>>10)
		for _, x := range blocks {
			if level, heading := markdown.OpeningHeading(x.Body); level > 0 {
				fmt.Fprintf(&b, "%s- %s %s\n", strings.Repeat("  ", level-1), oneLine(heading, 120), uri(db.Address{Object: o.ID, Block: x.ID}))
			}
		}
		return b.String(), nil
	}
	b.Write(raw)
	return b.String(), nil
}

// isHeading tells whether a block opens with a heading.
func isHeading(body string) bool {
	level, _ := markdown.OpeningHeading(body)
	return level > 0
}

// blockText is a block as a page file writes it: its marker, then its Markdown.
func blockText(x db.Block) string {
	marker := "<!-- block " + x.ID + " -->"
	if x.Align != "" {
		marker = "<!-- block " + x.ID + " align=" + x.Align + " -->"
	}
	return "\n" + marker + "\n" + strings.Trim(x.Body, "\r\n") + "\n"
}

// readFolder is a page of the folder source, read only: its outline as text.
func (t *mcpTools) readFolder(ref mcpRef) (string, *mcp.CallToolResult) {
	lib := t.h.library(t.ctx())
	doc, ok := lib.Document(ref.Folder)
	if !ok {
		return "", refusal(errNotFound)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "page: %s · in the folder source, read only · in: %s\n# %s\n", folderURI(string(doc.ID)), lib.Place(doc.ID), doc.Title)
	if doc.Description != "" {
		b.WriteString(doc.Description + "\n")
	}
	for _, piece := range lib.Outline(doc.ID) {
		if piece.Level > 0 {
			fmt.Fprintf(&b, "\n%s %s\n", strings.Repeat("#", piece.Level), piece.Text)
			continue
		}
		b.WriteString(piece.Text + "\n")
	}
	return b.String(), nil
}

type usedInIn struct {
	Address string `json:"address" jsonschema:"the page, block, section or part, such as underdoc:<id>#b3"`
}

// usedIn answers where something is embedded.
func (t *mcpTools) usedIn(_ context.Context, _ *mcp.CallToolRequest, in usedInIn) (*mcp.CallToolResult, any, error) {
	ref, err := parseRef(in.Address)
	if err != nil || ref.Folder != "" || ref.FilePage != 0 {
		return refusal("address: " + errRef.Error()), nil, nil
	}
	if _, err := t.h.options.DB.Object(t.ctx(), ref.Address.Object, t.user().ID); err != nil {
		return t.failure(err, "find where it is used"), nil, nil
	}
	lines := t.usedLines(ref.Address.Object, ref.Address.Target())
	if lines == "" {
		return text(uri(ref.Address) + " is embedded nowhere the person you act for sees."), nil, nil
	}
	return text(uri(ref.Address) + " is used in: " + lines), nil, nil
}

// usedLines names the pages that embed an object, or one target in it ("b3", "b3.t2", "b3.section"), at most
// mcpMaxRows, with the total.
func (t *mcpTools) usedLines(objectID, target string) string {
	refs, err := t.h.options.DB.UsedIn(t.ctx(), objectID, t.user().ID)
	if err != nil {
		return ""
	}
	var out []string
	seen := map[string]bool{}
	for _, u := range refs {
		if target != "" && u.TargetBlock != target && !strings.HasPrefix(u.TargetBlock, target+".") {
			continue
		}
		key := u.Page + "#" + u.Block
		if seen[key] {
			continue
		}
		seen[key] = true
		where := fmt.Sprintf("%q %s", oneLine(u.Title, 80), uri(db.Address{Object: u.Page, Block: u.Block}))
		if target == "" && u.TargetBlock != "" {
			where += " (embeds #" + u.TargetBlock + ")"
		}
		out = append(out, where)
	}
	if len(out) > mcpMaxRows {
		return strings.Join(out[:mcpMaxRows], "; ") + fmt.Sprintf("; … %d in all", len(out))
	}
	return strings.Join(out, "; ")
}

type searchIn struct {
	Query string   `json:"query" jsonschema:"the words to find; an error code such as E-1042 is found exactly"`
	Exact bool     `json:"exact,omitempty" jsonschema:"find the words only as written and in this order"`
	Types []string `json:"types,omitempty" jsonschema:"only these: page, file, folder, collection"`
	Tags  []string `json:"tags,omitempty" jsonschema:"only what carries every one of these tags"`
	In    string   `json:"in,omitempty" jsonschema:"only inside this collection at the top, by its address"`
}

// search answers one line per hit with its address and section and a short excerpt, the total at the end.
func (t *mcpTools) search(_ context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Query) == "" && len(in.Tags) == 0 {
		return refusal("query: say what to find, or name tags."), nil, nil
	}
	for _, typ := range in.Types {
		if !slices.Contains(searchTypes, typ) {
			return refusal("types: one of " + strings.Join(searchTypes, ", ") + "."), nil, nil
		}
	}
	request := search.Request{Text: in.Query, Exact: in.Exact, Limit: mcpSearchDepth, Filters: search.Filters{Types: in.Types, Tags: in.Tags}}
	if in.In != "" {
		ref, err := parseRef(in.In)
		if err != nil || ref.Folder != "" || ref.Address.Block != "" {
			return refusal("in: the address of a collection at the top."), nil, nil
		}
		request.Filters.Collections = []string{ref.Address.Object}
	}
	result, err := t.h.searchAs(t.ctx(), request)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return refusal("The search was cancelled."), nil, nil
		}
		return t.failure(err, "search"), nil, nil
	}
	var b strings.Builder
	if result.Notice != "" {
		b.WriteString(result.Notice + "\n")
	}
	for i, hit := range result.Hits {
		if i == mcpMaxRows {
			break
		}
		address := uri(db.Address{Object: hit.Object})
		if hit.Anchor != "" {
			address = uriScheme + hit.Object + "#" + hit.Anchor
		}
		if hit.Collection == library.FolderCollection {
			address = folderURI(hit.Object)
		}
		line := fmt.Sprintf("%d. [%s] %s", i+1, hit.Type, oneLine(hit.Title, 80))
		if hit.Heading != "" && hit.Heading != hit.Title {
			line += " › " + oneLine(hit.Heading, 80)
		}
		line += " " + address
		if excerpt := oneLine(plainSnippet(hit.Snippet), 160); excerpt != "" {
			line += " — " + excerpt
		}
		b.WriteString(line + "\n")
	}
	total := fmt.Sprint(len(result.Hits))
	if len(result.Hits) >= mcpSearchDepth {
		total = fmt.Sprintf("%d or more", mcpSearchDepth)
	}
	shown := min(len(result.Hits), mcpMaxRows)
	fmt.Fprintf(&b, "%d of %s found.", shown, total)
	return text(b.String()), nil, nil
}

// plainSnippet is a snippet without the marks of its matches.
func plainSnippet(fragments []search.Fragment) string {
	var b strings.Builder
	for _, f := range fragments {
		b.WriteString(f.Text)
	}
	return b.String()
}
