// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/transfer"
)

// The tools that write (ADR-34): a page or a section against the version it was read at, a passage, a dry run of
// either, the title, tags and kind, a new page, moving and the trash. Every answer carries the check of the page
// (errors block, warnings come with their fix) and the diff of what changed; every write is logged (ADR-11) and its
// versions are an agent's (ADR-33), so changes of human documentation carry the hint "written by AI, not checked".

// mcpMaxDiff is how many lines of diff an answer shows at most.
const mcpMaxDiff = 80

type writeIn struct {
	Page     string `json:"page" jsonschema:"the page, or underdoc:<id>#b3 to write only the section that block opens (or the block)"`
	Base     *int64 `json:"base" jsonschema:"the version you read; the write fails if the page changed since"`
	Markdown string `json:"markdown" jsonschema:"the new Markdown, with the block markers you read; a block without a marker is new, a block left out is deleted"`
	DryRun   bool   `json:"dry_run,omitempty" jsonschema:"check and show the diff without saving"`
}

// write writes a page or one section of it against its version.
func (t *mcpTools) write(_ context.Context, _ *mcp.CallToolRequest, in writeIn) (*mcp.CallToolResult, any, error) {
	a, failed := t.pageRef(in.Page)
	if failed != nil {
		return failed, nil, nil
	}
	if in.Base == nil {
		return refusal("base: give the version you read (read names it); a write without it could overwrite another's."), nil, nil
	}
	if a.Sub == db.Section {
		a.Sub = ""
	}
	if a.Sub != "" {
		return refusal("page: write a part with replace, or its block, " + uri(db.Address{Object: a.Object, Block: a.Block}) + "."), nil, nil
	}
	front, blocks := transfer.ParseBlocks([]byte(in.Markdown))
	note := ""
	if front.Found {
		if a.Block != "" {
			return refusal("markdown: a section has no front matter; set title, tags and kind with set_front_matter."), nil, nil
		}
		note = "The front matter was not written: title, tags and kind are set with set_front_matter.\n"
	}
	ctx, readers := t.beforeWrite(a.Object)
	written, err := t.h.options.DB.WriteBlocks(ctx, a.Object, t.user().ID, *in.Base, a.Block, blocks, in.DryRun)
	if err != nil {
		return t.writeFailure(err), nil, nil
	}
	return text(withReaders(note+t.writtenText(a.Object, written), readers)), nil, nil
}

type replaceIn struct {
	Page   string `json:"page" jsonschema:"the page"`
	Base   *int64 `json:"base" jsonschema:"the version you read"`
	Old    string `json:"old" jsonschema:"the passage as it stands, once in the page and inside one block"`
	New    string `json:"new" jsonschema:"what it becomes; empty removes it"`
	DryRun bool   `json:"dry_run,omitempty" jsonschema:"check and show the diff without saving"`
}

// replace replaces one passage of a page, which must stand there once.
func (t *mcpTools) replace(_ context.Context, _ *mcp.CallToolRequest, in replaceIn) (*mcp.CallToolResult, any, error) {
	a, failed := t.pageRef(in.Page)
	if failed != nil {
		return failed, nil, nil
	}
	if in.Base == nil {
		return refusal("base: give the version you read (read names it)."), nil, nil
	}
	if in.Old == "" {
		return refusal("old: name the passage to replace."), nil, nil
	}
	ctx, database, user := t.ctx(), t.h.options.DB, t.user()
	blocks, err := database.Blocks(ctx, a.Object, user.ID)
	if err != nil {
		return t.failure(err, "replace"), nil, nil
	}
	at, count := -1, 0
	for i, b := range blocks {
		if n := strings.Count(b.Body, in.Old); n > 0 {
			at, count = i, count+n
		}
	}
	switch {
	case count == 0:
		return refusal("old: the passage is not in the page as written; read the page again and copy it exactly."), nil, nil
	case count > 1:
		return refusal(fmt.Sprintf("old: the passage stands %d times; take more of the text around it, so it is found once.", count)), nil, nil
	}
	blocks[at].Body = strings.Replace(blocks[at].Body, in.Old, in.New, 1)
	ctx, readers := t.beforeWrite(a.Object)
	written, err := database.WriteBlocks(ctx, a.Object, user.ID, *in.Base, "", blocks, in.DryRun)
	if err != nil {
		return t.writeFailure(err), nil, nil
	}
	return text(withReaders(t.writtenText(a.Object, written), readers)), nil, nil
}

type frontIn struct {
	Page  string    `json:"page" jsonschema:"the page"`
	Title *string   `json:"title,omitempty" jsonschema:"a new title, 1 to 200 characters"`
	Tags  *[]string `json:"tags,omitempty" jsonschema:"the tags as a whole list; an empty list removes them"`
	Kind  *string   `json:"kind,omitempty" jsonschema:"overview, register, record, plan, source, reference, agent-notes, how-to, explanation, tutorial, or empty for none"`
}

// setFrontMatter sets what the front matter of a page holds: title, tags and kind. The description has no way in the
// browser yet, so none here either (docs/mcp.md).
func (t *mcpTools) setFrontMatter(_ context.Context, _ *mcp.CallToolRequest, in frontIn) (*mcp.CallToolResult, any, error) {
	a, failed := t.pageRef(in.Page)
	if failed != nil {
		return failed, nil, nil
	}
	if a.Block != "" {
		return refusal("page: front matter belongs to the page, not to a block."), nil, nil
	}
	if in.Title == nil && in.Tags == nil && in.Kind == nil {
		return refusal("Name title, tags or kind."), nil, nil
	}
	ctx, database, user := t.ctx(), t.h.options.DB, t.user()
	o, err := database.Object(ctx, a.Object, user.ID)
	if err == nil && o.Kind != db.KindPage {
		err = db.ErrNotFound
	}
	if err != nil {
		return t.failure(err, "set the front matter"), nil, nil
	}
	var done []string
	if in.Title != nil {
		if o, err = database.Rename(ctx, a.Object, user.ID, *in.Title); err != nil {
			return t.failure(err, "set the title"), nil, nil
		}
		done = append(done, "title")
	}
	if in.Tags != nil {
		if o, err = database.SetTags(ctx, a.Object, user.ID, *in.Tags); err != nil {
			return t.failure(err, "set the tags"), nil, nil
		}
		done = append(done, "tags")
	}
	if in.Kind != nil {
		if o, err = database.SetPageKind(ctx, a.Object, user.ID, *in.Kind); err != nil {
			return t.failure(err, "set the kind"), nil, nil
		}
		done = append(done, "kind")
	}
	t.h.noteChange(t.r, o.ID, "mcp front matter: "+strings.Join(done, ", "))
	found, _ := database.Findings(ctx, o.ID, user.ID)
	return text(fmt.Sprintf("Set %s of %s: title %q, kind %q, tags %v.\n%s", strings.Join(done, ", "), uri(db.Address{Object: o.ID}),
		o.Title, o.PageKind, o.Tags, findingLines(found))), nil, nil
}

type createIn struct {
	Parent   string `json:"parent" jsonschema:"the collection, folder or page to make it in"`
	Title    string `json:"title" jsonschema:"its title, 1 to 200 characters"`
	Kind     string `json:"kind,omitempty" jsonschema:"its kind (see set_front_matter); choose one for prose"`
	Markdown string `json:"markdown,omitempty" jsonschema:"its text; headings split it into blocks"`
}

// createPage makes a page, empty or with text. A space at the top is never made through MCP (B-4).
func (t *mcpTools) createPage(_ context.Context, _ *mcp.CallToolRequest, in createIn) (*mcp.CallToolResult, any, error) {
	ref, err := parseRef(in.Parent)
	if err != nil || ref.Folder != "" || ref.FilePage != 0 || ref.Address.Block != "" {
		return refusal("parent: the address of a collection, folder or page."), nil, nil
	}
	database, user := t.h.options.DB, t.user()
	// The page and its text in one transaction (B-5): a text the checks refuse makes no page.
	var blocks []db.Block
	if strings.TrimSpace(in.Markdown) != "" {
		_, blocks = transfer.ParseBlocks([]byte(in.Markdown))
	}
	// The new page has the readers of its parent.
	ctx, readers := t.beforeWrite(ref.Address.Object)
	made, written, err := database.CreatePageWithBlocks(ctx, user, in.Title, ref.Address.Object, in.Kind, blocks)
	if err != nil {
		return t.writeFailure(err), nil, nil
	}
	t.h.noteChange(t.r, made.ID, "made through mcp")
	return text(fmt.Sprintf("Made %s %q in %s.\n", uri(db.Address{Object: made.ID}), made.Title, uri(db.Address{Object: ref.Address.Object})) +
		withReaders(t.writtenText(made.ID, written), readers)), nil, nil
}

// bareBlock is a block named without its page, such as b3.
var bareBlock = regexp.MustCompile(`^b[1-9][0-9]{0,8}$`)

type moveIn struct {
	Object string `json:"object" jsonschema:"what to move: an object, or a block (underdoc:<id>#b3) to move into another page"`
	To     string `json:"to" jsonschema:"where it goes: a collection, folder or page; for a block the page"`
	After  string `json:"after,omitempty" jsonschema:"the entry or block it comes after (for a block: b3 of the page it goes to); first if empty"`
}

// move puts an object somewhere else, or a block into another page. The address of an object never changes, and a moved
// block leaves a redirect, so what links or embeds it follows (ADR-18, ADR-22).
func (t *mcpTools) move(_ context.Context, _ *mcp.CallToolRequest, in moveIn) (*mcp.CallToolResult, any, error) {
	what, err := parseRef(in.Object)
	if err != nil || what.Folder != "" || what.FilePage != 0 || what.Address.Sub != "" {
		return refusal("object: the address of an object or a block."), nil, nil
	}
	to, err := parseRef(in.To)
	if err != nil || to.Folder != "" || to.FilePage != 0 || to.Address.Block != "" {
		return refusal("to: the address of a collection, folder or page; a space at the top is made in the browser."), nil, nil
	}
	after := ""
	switch bare := strings.TrimPrefix(in.After, "#"); {
	case in.After == "":
	case what.Address.Block != "" && bareBlock.MatchString(bare):
		// A block of the page it goes to, named alone (b3 or #b3), as the tool says (B-5).
		after = bare
	default:
		ref, err := parseRef(in.After)
		if err != nil || ref.Folder != "" {
			return refusal("after: the address of an entry, or a block such as b3."), nil, nil
		}
		after = ref.Address.Object
		if what.Address.Block != "" {
			after = ref.Address.Block
		}
	}
	database, user := t.h.options.DB, t.user()
	ctx, readers := t.beforeWrite(to.Address.Object)
	if what.Address.Block != "" {
		if after == "" && in.After != "" {
			return refusal("after: name a block of the page it goes to."), nil, nil
		}
		block, err := database.MoveBlockTo(ctx, what.Address.Object, what.Address.Block, user.ID, to.Address.Object, after)
		if err != nil {
			return t.writeFailure(err), nil, nil
		}
		t.h.noteChange(t.r, what.Address.Object, "mcp moved #"+what.Address.Block+" to "+to.Address.Object)
		moved := db.Address{Object: to.Address.Object, Block: block}
		return text(fmt.Sprintf("Moved %s to %s; the old address leads there, and what embedded it follows.\n", uri(what.Address), uri(moved)) + readers), nil, nil
	}
	o, err := database.Move(ctx, what.Address.Object, user.ID, to.Address.Object, after)
	if err != nil {
		return t.failure(err, "move"), nil, nil
	}
	t.h.noteChange(t.r, o.ID, "mcp moved to "+to.Address.Object)
	return text(fmt.Sprintf("Moved %q to %s. Its address %s stays, so every link to it still leads there.\n", o.Title,
		t.h.place(t.r, o), uri(db.Address{Object: o.ID})) + readers), nil, nil
}

type trashIn struct {
	Object string `json:"object" jsonschema:"what to move to the trash, with everything below it"`
}

// trash moves an object to the trash: a person can restore it there, and only a person deletes it for good (ADR-36).
func (t *mcpTools) trash(_ context.Context, _ *mcp.CallToolRequest, in trashIn) (*mcp.CallToolResult, any, error) {
	ref, err := parseRef(in.Object)
	if err != nil || ref.Folder != "" || ref.FilePage != 0 || ref.Address.Block != "" {
		return refusal("object: the address of an object; a block is taken out with write."), nil, nil
	}
	ctx, database, user := t.ctx(), t.h.options.DB, t.user()
	o, err := database.Object(ctx, ref.Address.Object, user.ID)
	if err != nil {
		return t.failure(err, "move to the trash"), nil, nil
	}
	if err := database.DeleteWays(ctx, o.ID, user.ID, nil); err != nil {
		return t.writeFailure(err), nil, nil
	}
	t.h.noteChange(t.r, o.ID, "mcp moved to the trash")
	return text(fmt.Sprintf("Moved %q to the trash. A person can restore it there for %d days; nothing is deleted for good by an agent.",
		o.Title, db.TrashDays)), nil, nil
}

// pageRef reads the address of a page of the database a tool writes.
func (t *mcpTools) pageRef(s string) (db.Address, *mcp.CallToolResult) {
	ref, err := parseRef(s)
	if err != nil {
		return db.Address{}, refusal("page: " + err.Error())
	}
	if ref.Folder != "" {
		return db.Address{}, refusal("page: the folder source is read only.")
	}
	if ref.FilePage != 0 {
		return db.Address{}, refusal("page: a file is not written here; upload a new version.")
	}
	return ref.Address, nil
}

// writeFailure answers what a write refuses: a conflict, something in use, an error of the check, or what failure says.
func (t *mcpTools) writeFailure(err error) *mcp.CallToolResult {
	var changed *db.ErrChanged
	var used *db.InUse
	switch {
	case errors.As(err, &changed):
		return refusal("Conflict: " + changed.Error() + ".")
	case errors.As(err, &used):
		var where []string
		for _, ref := range used.Refs {
			if a, ok := db.ParseAddress(ref); ok {
				where = append(where, uri(a))
			}
		}
		return refusal("In use elsewhere: " + strings.Join(where, ", ") + ". Keep it, or move it with move so what embeds it follows; a person decides in the browser what becomes of the uses.")
	}
	return t.failure(err, "write")
}

// writtenText is the answer of a write: saved or not, the version, the changes with their diff, and the check.
func (t *mcpTools) writtenText(pageID string, w db.Written) string {
	var b strings.Builder
	if w.DryRun {
		fmt.Fprintf(&b, "Dry run, nothing saved: %s stays at version %d.\n", uri(db.Address{Object: pageID}), w.Version)
	} else if len(w.Changes) > 0 {
		fmt.Fprintf(&b, "Saved %s: version %d. Write against it next.\n", uri(db.Address{Object: pageID}), w.Version)
		t.h.noteChange(t.r, pageID, "mcp write")
	}
	var counts []string
	for _, kind := range []string{db.ChangeAdded, db.ChangeChanged, db.ChangeDeleted} {
		if n := countChanges(w.Changes, kind); n > 0 {
			counts = append(counts, fmt.Sprintf("%d %s", n, kind))
		}
	}
	if len(counts) == 0 {
		b.WriteString("Nothing changed.\n")
	} else {
		b.WriteString("Blocks: " + strings.Join(counts, ", ") + ".\n")
		b.WriteString(changeDiff(w.Changes))
	}
	b.WriteString(findingLines(w.Findings))
	return clip(b.String())
}

func countChanges(changes []db.BlockChange, kind string) int {
	n := 0
	for _, c := range changes {
		if c.Kind == kind {
			n++
		}
	}
	return n
}

// changeDiff writes the changed blocks as a diff, line by line, at most mcpMaxDiff lines.
func changeDiff(changes []db.BlockChange) string {
	var lines []string
	for _, c := range changes {
		if c.Kind == db.ChangeKept {
			continue
		}
		lines = append(lines, fmt.Sprintf("@@ %s %s", c.Block, c.Kind))
		lines = append(lines, lineDiff(splitLines(c.Before), splitLines(c.After))...)
	}
	if len(lines) > mcpMaxDiff {
		lines = append(lines[:mcpMaxDiff], fmt.Sprintf("… %d more lines of diff", len(lines)-mcpMaxDiff))
	}
	return strings.Join(lines, "\n") + "\n"
}

func splitLines(s string) []string {
	s = strings.Trim(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// lineDiff is the shortest edit between two texts line by line: "-" for a line taken out, "+" for one put in, the
// lines both share left out. Blocks are short, so the quadratic table is cheap; a pair too large is shown whole.
func lineDiff(a, b []string) []string {
	if len(a)*len(b) > 1_000_000 {
		var out []string
		for _, l := range a {
			out = append(out, "-"+l)
		}
		for _, l := range b {
			out = append(out, "+"+l)
		}
		return out
	}
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []string
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			i, j = i+1, j+1
		case j < len(b) && (i == len(a) || lcs[i][j+1] >= lcs[i+1][j]):
			out = append(out, "+"+b[j])
			j++
		default:
			out = append(out, "-"+a[i])
			i++
		}
	}
	return slices.Clip(out)
}
