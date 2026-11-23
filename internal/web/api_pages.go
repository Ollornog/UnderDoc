// SPDX-License-Identifier: MIT

package web

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/markdown"
	"github.com/Ollornog/UnderDoc/internal/pagekind"
	"github.com/Ollornog/UnderDoc/internal/route"
	"github.com/Ollornog/UnderDoc/internal/transfer"
)

// Pages of the database, edited block by block (ADR-19). Every change answers with what the workspace shows next.

// maxVersions is how many changes the workspace lists for a page.
const maxVersions = 50

type apiBlock struct {
	ID       string `json:"id"`
	HTML     string `json:"html"`
	Markdown string `json:"md"`
	// UsedIn names the pages that embed this block.
	UsedIn []string `json:"usedIn"`
	// Align is center or right, absent for the left (ADR-16).
	Align string `json:"align,omitempty"`
	// Comments counts the comments on this block in threads not resolved.
	Comments int `json:"comments,omitempty"`
	// Parts are the parts inside its paragraphs (ADR-22), with where each is used.
	Parts []apiPart `json:"parts,omitempty"`
	// SectionUsedIn names the pages that embed the section this block opens.
	SectionUsedIn []string `json:"sectionUsedIn,omitempty"`
	// Findings are the writing rules the page breaks after a write, with how to fix each (T-20): the answer of a write
	// carries them, so every door hears of them at once.
	Findings []pagekind.Finding `json:"findings,omitempty"`
	// AI is the change of agents of this block that nobody checked yet, on human documentation (ADR-33): readers see it.
	AI *apiAIMark `json:"ai,omitempty"`
}

// apiAIMark is the hint "written by AI, not checked" of a block: the change a check names, whom the agent works for,
// when, and whether an agent made the block.
type apiAIMark struct {
	Version int64  `json:"version"`
	By      string `json:"by"`
	At      string `json:"at"`
	Added   bool   `json:"added,omitempty"`
}

// apiRemoved is a block an agent took out of human documentation that nobody checked yet; Moved names where it went.
type apiRemoved struct {
	Block   string `json:"block"`
	Version int64  `json:"version"`
	By      string `json:"by"`
	At      string `json:"at"`
	Moved   string `json:"moved,omitempty"`
}

// apiChecked is the last check of a page: who and when.
type apiChecked struct {
	By string `json:"by"`
	At string `json:"at"`
}

type apiVersion struct {
	Block   string `json:"block"`
	Author  string `json:"who"`
	At      string `json:"at"`
	Deleted bool   `json:"deleted,omitempty"`
	Excerpt string `json:"excerpt"`
	// Origin says where a copy came from or where moved text went (ADR-22).
	Origin string `json:"origin,omitempty"`
	// AI tells that an agent wrote the change (ADR-33).
	AI bool `json:"ai,omitempty"`
}

// change reads the body of a request that changes something: only with the database, only from this application, and
// only a small body. It answers the request itself when one of these fails.
func (h *handler) change(w http.ResponseWriter, r *http.Request, limit int64, into any) (account.User, bool) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return account.User{}, false
	}
	if !sameOrigin(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only from this application"})
		return account.User{}, false
	}
	if into != nil {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit)).Decode(into); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
			return account.User{}, false
		}
	}
	user, _ := account.From(r.Context())
	return user, true
}

// failed answers an error of the database in the words a person can act on. It reports whether there was one.
func (h *handler) failed(w http.ResponseWriter, err error, what string) bool {
	var rule *db.RuleError
	switch {
	case err == nil:
		return false
	case errors.As(err, &rule):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "findings": rule.Findings})
	case errors.Is(err, db.ErrTitle), errors.Is(err, db.ErrKind), errors.Is(err, db.ErrBlock), errors.Is(err, db.ErrPlace), errors.Is(err, db.ErrTooMany), errors.Is(err, db.ErrSettings), errors.Is(err, db.ErrComment), errors.Is(err, db.ErrTag), errors.Is(err, db.ErrView), errors.Is(err, db.ErrAlign), errors.Is(err, db.ErrTemplate), errors.Is(err, db.ErrRole), errors.Is(err, db.ErrGroupName),
		errors.Is(err, db.ErrLogKeep), errors.Is(err, db.ErrFileName), errors.Is(err, db.ErrSambaName), errors.Is(err, db.ErrNoSamba),
		errors.Is(err, db.ErrConfirmName), errors.Is(err, db.ErrArchivePlace):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.As(err, new(*db.OpenError)), errors.Is(err, db.ErrPerFolder):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, db.ErrQuota):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, db.ErrWay), errors.Is(err, db.ErrEntry):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, db.ErrGroupCycle), errors.Is(err, db.ErrGroupArchived), errors.Is(err, db.ErrGroupHasGroups):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, db.ErrPart):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": strings.TrimPrefix(err.Error(), db.ErrPart.Error()+"\n")})
	case errors.Is(err, db.ErrStale), errors.Is(err, db.ErrMovedAway):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, db.ErrHasReplies):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, db.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, db.ErrNotAllowed), errors.Is(err, db.ErrPrivate), errors.Is(err, db.ErrGroup), errors.Is(err, db.ErrSecretHeld):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
	default:
		h.log.Error(what+" failed", slog.String("error", err.Error()))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
	return true
}

// block renders a block for the workspace. Raw HTML in it is dropped, as in every page (TestUserTextStaysText).
func (h *handler) block(r *http.Request, page string, b db.Block, used []db.Ref) apiBlock {
	out := apiBlock{ID: b.ID, Markdown: b.Body, HTML: h.render(r, b.Body, []string{page, page + "#" + b.ID}), UsedIn: []string{}, Align: b.Align}
	for _, u := range used {
		if u.TargetBlock == b.ID && !slices.Contains(out.UsedIn, u.Title) {
			out.UsedIn = append(out.UsedIn, u.Title)
		}
		if u.TargetBlock == b.ID+"."+db.Section && !slices.Contains(out.SectionUsedIn, u.Title) {
			out.SectionUsedIn = append(out.SectionUsedIn, u.Title)
		}
	}
	if ps := parts(page, b, used); len(ps) > 0 {
		out.Parts = ps
	}
	return out
}

// maxEmbed is how deep embeddings may nest: a block that embeds a block that embeds a block, and so on.
const maxEmbed = 5

// render turns the Markdown of a block into HTML. A line ::embed{ref="…"} becomes what it refers to, rendered for this
// viewer each time, so it is live, with a link back to where it comes from (ADR-18); so does a part embedded inside a
// paragraph with :embed{ref="…"} (ADR-22). What cannot be shown says so, visibly: a missing source is an error to see,
// never a silent gap. trail holds what is being rendered, to stop at a loop.
func (h *handler) render(r *http.Request, body string, trail []string) string {
	// Passwords show only in a block of their own page (ADR-7); the trail ends with that block.
	page := pageOf(trail)
	var out strings.Builder
	last := 0
	for _, m := range db.EmbedLine.FindAllStringSubmatchIndex(body, -1) {
		out.WriteString(h.inlineEmbeds(r, h.passwords(r, h.markdown(r, body[last:m[0]]), page), trail))
		ref := body[m[2]:m[3]]
		if m[4] >= 0 {
			ref += "#" + body[m[4]:m[5]]
		}
		a, _ := db.ParseAddress(ref)
		out.WriteString(h.embed(r, a, trail))
		last = m[1]
	}
	out.WriteString(h.inlineEmbeds(r, h.passwords(r, h.markdown(r, body[last:]), page), trail))
	return out.String()
}

func (h *handler) markdown(r *http.Request, md string) string {
	if strings.TrimSpace(md) == "" {
		return ""
	}
	rendered, err := h.library(r.Context()).RenderMarkdown(md)
	if err != nil {
		return "<pre>" + html.EscapeString(md) + "</pre>"
	}
	return rendered.HTML
}

// inlineEmbeds puts the parts embedded inside paragraphs into rendered HTML, in place of their sentinels.
func (h *handler) inlineEmbeds(r *http.Request, rendered string, trail []string) string {
	if !strings.Contains(rendered, "data-embed-inline") {
		return rendered
	}
	return markdown.EmbedSentinels.ReplaceAllStringFunc(rendered, func(sentinel string) string {
		a, _ := db.ParseAddress(markdown.EmbedSentinels.FindStringSubmatch(sentinel)[1])
		var inner, title, why string
		if db.IsTableID(a.Block) {
			inner, title, why = h.inlineTable(r, a)
		} else {
			inner, title, why = h.embedded(r, a, trail)
		}
		if g := h.goneAt(r, a); why != "" && g != nil {
			return h.placeholder(r, a, g, trail[0], true)
		}
		if why != "" {
			return `<span class="badge" data-variant="destructive" data-ref="` + html.EscapeString(a.String()) + `">` + html.EscapeString(why) + `</span>`
		}
		return `<span data-embed-part="` + html.EscapeString(a.String()) + `" title="` + html.EscapeString(title) + `">` + inner + `</span>`
	})
}

// embed renders one embedded page, block, section or part of the database, as a block with a link to where it comes from:
// C22's figure.editor-embed, with data-state="missing" and the reason when it cannot be shown.
func (h *handler) embed(r *http.Request, a db.Address, trail []string) string {
	ref := a.String()
	inner, label, why := h.embedded(r, a, trail)
	if g := h.goneAt(r, a); why != "" && g != nil {
		return h.placeholder(r, a, g, trail[0], false)
	}
	if why != "" {
		return `<figure class="editor-embed" data-state="missing" data-ref="` + html.EscapeString(ref) + `"><p>` + html.EscapeString(why) + `</p></figure>`
	}
	if a.Sub != "" && a.Sub != db.Section {
		inner = `<p class="leading-relaxed">` + inner + `</p>`
	}
	link := route.Object(a.Object)
	if a.Block != "" {
		link += "#" + a.Target()
	}
	return `<figure class="editor-embed" data-ref="` + html.EscapeString(ref) + `">` + inner +
		`<figcaption><a href="` + html.EscapeString(link) + `">` + html.EscapeString(label) + `</a></figcaption></figure>`
}

// embedded renders what an address names for this viewer: its HTML and a label that says where it comes from, or why it
// cannot be shown. A part comes as inline HTML, cut out of its paragraph with its formatting (markdown.CutPart).
func (h *handler) embedded(r *http.Request, a db.Address, trail []string) (inner, label, why string) {
	ref := a.String()
	loop := slices.Contains(trail, ref) || slices.Contains(trail, a.Object)
	if a.Block == "" {
		loop = loop || slices.ContainsFunc(trail, func(x string) bool { return strings.HasPrefix(x, a.Object+"#") })
	} else {
		loop = loop || slices.Contains(trail, a.Object+"#"+a.Block)
	}
	if loop {
		return "", "", "This embeds itself: " + ref
	}
	if len(trail) > maxEmbed {
		return "", "", "Embedded too deep: " + ref
	}
	// A page frozen for a link shows only what its maker released (ADR-29).
	if unreleased(r.Context(), a.Object) {
		return "", "", "Not shared: " + ref
	}
	user, _ := account.From(r.Context())
	o, err := h.options.DB.Object(r.Context(), a.Object, user.ID)
	if err == nil && o.Kind == db.KindFile && db.IsTableID(a.Block) && h.options.Store != nil {
		return h.embeddedTable(r, o, a)
	}
	if err != nil || o.Kind != db.KindPage || db.IsTableID(a.Block) {
		return "", "", "Not to be found: " + ref
	}
	blocks, err := h.options.DB.Blocks(r.Context(), a.Object, user.ID)
	if err != nil {
		return "", "", "Not to be found: " + ref
	}
	at := slices.IndexFunc(blocks, func(b db.Block) bool { return b.ID == a.Block })
	if a.Block != "" && at < 0 {
		if to, moved := h.options.DB.Redirect(r.Context(), a); moved {
			return h.embedded(r, to, trail)
		}
		return "", "", "Block #" + a.Block + " is no longer in " + o.Title
	}
	inside := func(b db.Block) []string { return append(slices.Clone(trail), ref, a.Object+"#"+b.ID) }
	var out strings.Builder
	label = o.Title
	switch {
	case a.Block == "":
		for _, b := range blocks {
			out.WriteString(h.render(r, b.Body, inside(b)))
		}
	case a.Sub == "":
		out.WriteString(h.render(r, blocks[at].Body, inside(blocks[at])))
		label += " · #" + a.Block
	case a.Sub == db.Section:
		bodies := make([]string, len(blocks)-at)
		for i, b := range blocks[at:] {
			bodies[i] = b.Body
		}
		n := markdown.SectionEnd(bodies)
		if n == 0 {
			if to, moved := h.options.DB.Redirect(r.Context(), a); moved {
				return h.embedded(r, to, trail)
			}
			return "", "", "Block #" + a.Block + " opens no section in " + o.Title
		}
		for _, b := range blocks[at : at+n] {
			out.WriteString(h.render(r, b.Body, inside(b)))
		}
		_, heading := markdown.OpeningHeading(blocks[at].Body)
		label += " · " + heading
	default:
		part, ok := markdown.CutPart(h.render(r, blocks[at].Body, inside(blocks[at])), a.Sub)
		if !ok {
			if to, moved := h.options.DB.Redirect(r.Context(), a); moved {
				return h.embedded(r, to, trail)
			}
			return "", "", "Part #" + a.Target() + " is no longer in " + o.Title
		}
		return part, o.Title + " · #" + a.Target(), ""
	}
	return markdown.StripPartSentinels(out.String()), label, ""
}

// pageDocument answers /api/doc for a page of the database: its blocks, its place and whether the viewer may change it.
func (h *handler) pageDocument(w http.ResponseWriter, r *http.Request, o db.Object) {
	ctx := r.Context()
	user, _ := account.From(ctx)
	blocks, err := h.options.DB.Blocks(ctx, o.ID, user.ID)
	if h.failed(w, err, "blocks") {
		return
	}
	answer := apiDocument{ID: o.ID, Title: o.Title, Kind: o.Kind, Description: o.Description, Tags: tagList(o.Tags),
		Place: h.place(r, o), Path: "not a file, lives in the database", Changed: stamp(o.Changed),
		Sections: [][]string{}, UsedIn: []string{}, Links: []string{}, Stable: true, Blocks: []apiBlock{},
		Owner: h.options.DB.AccountName(ctx, o.Owner), Created: stamp(o.Created), SeenBy: h.seenBy(r, o), Archived: h.inArchive(r, o)}
	used, err := h.options.DB.UsedIn(ctx, o.ID, user.ID)
	if err != nil {
		h.log.Error("used in failed", slog.String("error", err.Error()))
	}
	open, err := h.options.DB.OpenComments(ctx, o.ID, user.ID)
	if err != nil {
		h.log.Error("open comments failed", slog.String("error", err.Error()))
	}
	for _, b := range blocks {
		out := h.block(r, o.ID, b, used)
		out.Comments = open[b.ID]
		answer.Blocks = append(answer.Blocks, out)
	}
	for _, n := range open {
		answer.Comments += n
	}
	embeds, err := h.options.DB.Embeds(ctx, o.ID, user.ID)
	if err != nil {
		h.log.Error("embeds failed", slog.String("error", err.Error()))
	}
	for _, e := range embeds {
		kind := "page"
		switch {
		case db.IsTableID(e.Address.Block):
			kind, e.Name = "table", e.Table
		case e.Name != "":
			kind = "value"
		case e.Address.Sub == db.Section:
			kind = "section"
		case e.Address.Sub != "":
			kind = "part"
		case e.Address.Block != "":
			kind = "block"
		}
		answer.Embeds = append(answer.Embeds, apiEmbed{Kind: kind, Name: e.Name, Title: e.Title, Ref: e.Address.String(),
			URL: route.Object(e.Address.Object) + anchorOf(e.Address)})
	}
	for _, u := range used {
		where := u.Title
		if u.TargetBlock != "" {
			where += " (block #" + u.TargetBlock + ")"
		}
		if !slices.Contains(answer.UsedIn, where) {
			answer.UsedIn = append(answer.UsedIn, where)
		}
	}
	answer.CanEdit = h.options.DB.MayEdit(ctx, o, user.ID)
	answer.CanComment = h.options.DB.MayComment(ctx, o, user.ID)
	if o.Kind == db.KindPage {
		answer.PageKind, answer.Audience = o.PageKind, pagekind.Audience(o.PageKind)
		if answer.CanEdit {
			answer.Findings = h.findings(r, o.ID)
		}
		h.aiMarks(r, o, &answer)
	}
	writeJSON(w, http.StatusOK, answer)
}

// aiMarks puts the hints of changes by agents into a page (ADR-33): on human documentation each block an agent changed
// and nobody checked carries one, and what agents took out is listed; the agent side says once that AI maintains it.
// The last check shows as "checked by … on …".
func (h *handler) aiMarks(r *http.Request, o db.Object, answer *apiDocument) {
	ctx := r.Context()
	user, _ := account.From(ctx)
	answer.MaintainedByAI = pagekind.Audience(o.PageKind) == pagekind.AudienceAgents
	changes, err := h.options.DB.AIChanges(ctx, o.ID, user.ID)
	if err != nil {
		h.log.Error("ai changes failed", slog.String("error", err.Error()))
	}
	for _, c := range changes {
		if c.Deleted {
			answer.Removed = append(answer.Removed, apiRemoved{Block: c.Block, Version: c.Head, By: c.By, At: stamp(c.At), Moved: c.Moved})
			continue
		}
		for i := range answer.Blocks {
			if answer.Blocks[i].ID == c.Block {
				answer.Blocks[i].AI = &apiAIMark{Version: c.Head, By: c.By, At: stamp(c.At), Added: c.Base == 0}
			}
		}
	}
	if checked, err := h.options.DB.LastCheck(ctx, o.ID, user.ID); err == nil && checked != nil {
		answer.Checked = &apiChecked{By: checked.By, At: stamp(checked.At)}
	}
}

type apiChange struct {
	Block   string `json:"block"`
	Version int64  `json:"version"`
	By      string `json:"by"`
	At      string `json:"at"`
	Before  string `json:"before"`
	Now     string `json:"now"`
	Added   bool   `json:"added,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
	Moved   string `json:"moved,omitempty"`
}

// apiChange answers the open change of agents of a block with its text before and now, for the diff of a check.
func (h *handler) apiChange(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	user, _ := account.From(r.Context())
	d, err := h.options.DB.ChangeOf(r.Context(), r.PathValue("id"), r.PathValue("block"), user.ID)
	if h.failed(w, err, "change") {
		return
	}
	writeJSON(w, http.StatusOK, apiChange{Block: d.Block, Version: d.Head, By: d.By, At: stamp(d.At), Before: d.Before, Now: d.Now,
		Added: d.Base == 0, Deleted: d.Deleted, Moved: d.Moved})
}

// apiCheckChange marks the open change of agents of a block as checked, the one the person saw ({"version": n}).
func (h *handler) apiCheckChange(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Version int64 `json:"version"`
	}
	user, ok := h.change(w, r, 1<<10, &in)
	if !ok {
		return
	}
	if h.failed(w, h.options.DB.CheckChange(r.Context(), r.PathValue("id"), r.PathValue("block"), user.ID, in.Version), "check") {
		return
	}
	h.noteReview(r, db.LogChecked, in.Version)
	w.WriteHeader(http.StatusNoContent)
}

// noteReview logs a check or a taking back, unless the page is private: what lies in private notes is nobody's business.
func (h *handler) noteReview(r *http.Request, action string, version int64) {
	if private, err := h.options.DB.Private(r.Context(), r.PathValue("id")); err != nil || private {
		return
	}
	h.note(r, action, r.PathValue("id"), "#"+r.PathValue("block")+" version "+strconv.FormatInt(version, 10))
}

// apiTakeBack undoes the open change of agents of a block, the one the person saw ({"version": n}); a block in use that
// it would take away needs a way, as a deletion does (ADR-22).
func (h *handler) apiTakeBack(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Version int64             `json:"version"`
		Ways    map[string]db.Way `json:"ways"`
	}
	user, ok := h.change(w, r, 8<<10, &in)
	if !ok {
		return
	}
	err := h.options.DB.TakeBack(r.Context(), r.PathValue("id"), r.PathValue("block"), user.ID, in.Version, in.Ways)
	if h.inUse(w, r, err) || h.failed(w, err, "take back") {
		return
	}
	h.noteReview(r, db.LogTakenBack, in.Version)
	w.WriteHeader(http.StatusNoContent)
}

// findings are the writing rules a page breaks for the viewer (T-20). A failure to check is logged, never a failed write.
func (h *handler) findings(r *http.Request, page string) []pagekind.Finding {
	user, _ := account.From(r.Context())
	found, err := h.options.DB.Findings(r.Context(), page, user.ID)
	if err != nil {
		h.log.Error("findings failed", slog.String("error", err.Error()))
	}
	return found
}

// place names the collections and folders above an object, as the workspace shows it.
func (h *handler) place(r *http.Request, o db.Object) string {
	user, _ := account.From(r.Context())
	var names []string
	for parent := o.Parent; parent != "" && len(names) < 20; {
		p, err := h.options.DB.Object(r.Context(), parent, user.ID)
		if err != nil {
			break
		}
		names = append([]string{p.Title}, names...)
		parent = p.Parent
	}
	return strings.Join(names, " › ")
}

// apiCreatePage makes a page inside a collection, folder or page: empty, or with the blocks of a template (M-8).
func (h *handler) apiCreatePage(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title    string `json:"title"`
		Parent   string `json:"parent"`
		Template string `json:"template"`
		// Kind is the kind of the page (T-20); without it the page takes the kind of its template.
		Kind string `json:"kind"`
	}
	user, ok := h.change(w, r, 8<<10, &in)
	if !ok {
		return
	}
	created, err := h.options.DB.CreatePageOf(r.Context(), user, in.Title, in.Parent, in.Template, in.Kind)
	if h.failed(w, err, "page") {
		return
	}
	h.noteMade(r, created.ID)
	writeJSON(w, http.StatusCreated, apiNode{ID: created.ID, Title: created.Title, Kind: created.Kind, Kids: []apiNode{}, Path: route.Object(created.ID)})
}

// apiAddBlock puts a new block into a page, after the block it names or first.
func (h *handler) apiAddBlock(w http.ResponseWriter, r *http.Request) {
	var in struct {
		After string `json:"after"`
		Body  string `json:"md"`
	}
	user, ok := h.change(w, r, 1<<20, &in)
	if !ok {
		return
	}
	b, err := h.options.DB.AddBlock(r.Context(), r.PathValue("id"), user.ID, in.After, in.Body)
	if h.failed(w, err, "new block") {
		return
	}
	out := h.block(r, r.PathValue("id"), b, nil)
	out.Findings = h.findings(r, r.PathValue("id"))
	writeJSON(w, http.StatusCreated, out)
}

// apiMoveBlock puts a block after another, or first.
func (h *handler) apiMoveBlock(w http.ResponseWriter, r *http.Request) {
	var in struct {
		After string `json:"after"`
		// Page moves the block into another page (ADR-22): references follow, and the old address redirects.
		Page string `json:"page"`
	}
	user, ok := h.change(w, r, 1<<10, &in)
	if !ok {
		return
	}
	if in.Page != "" && in.Page != r.PathValue("id") {
		id, err := h.options.DB.MoveBlockTo(r.Context(), r.PathValue("id"), r.PathValue("block"), user.ID, in.Page, in.After)
		if h.failed(w, err, "move block") {
			return
		}
		a := db.Address{Object: in.Page, Block: id}
		writeJSON(w, http.StatusOK, map[string]string{"ref": a.String(), "url": route.Object(in.Page) + "#" + id})
		return
	}
	if h.failed(w, h.options.DB.MoveBlock(r.Context(), r.PathValue("id"), r.PathValue("block"), user.ID, in.After), "move block") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// apiEditBlock gives a block new Markdown.
func (h *handler) apiEditBlock(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Body string            `json:"md"`
		Ways map[string]db.Way `json:"ways"`
	}
	user, ok := h.change(w, r, 1<<20, &in)
	if !ok {
		return
	}
	b, err := h.options.DB.EditBlockWays(r.Context(), r.PathValue("id"), r.PathValue("block"), user.ID, in.Body, in.Ways)
	if h.inUse(w, r, err) || h.failed(w, err, "block") {
		return
	}
	used, _ := h.options.DB.UsedIn(r.Context(), r.PathValue("id"), user.ID)
	out := h.block(r, r.PathValue("id"), b, used)
	out.Findings = h.findings(r, r.PathValue("id"))
	writeJSON(w, http.StatusOK, out)
}

// apiPageKind gives a page its kind, or none (T-20), and answers with the side it sets and what the page breaks now.
func (h *handler) apiPageKind(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind string `json:"kind"`
	}
	user, ok := h.change(w, r, 1<<10, &in)
	if !ok {
		return
	}
	o, err := h.options.DB.SetPageKind(r.Context(), r.PathValue("id"), user.ID, in.Kind)
	if h.failed(w, err, "kind") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"kind": o.PageKind, "audience": pagekind.Audience(o.PageKind), "findings": nonNilFindings(h.findings(r, o.ID))})
}

func nonNilFindings(found []pagekind.Finding) []pagekind.Finding {
	if found == nil {
		return []pagekind.Finding{}
	}
	return found
}

// apiRawPage answers a page as its Markdown file, with its front matter and the marker before each block, as an export
// writes it (docs/export.md): what agents read, while people get the page without them (T-20).
func (h *handler) apiRawPage(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	user, _ := account.From(r.Context())
	n, err := h.options.DB.Subtree(r.Context(), r.PathValue("id"), user.ID)
	if err == nil && n.Kind != db.KindPage && n.Kind != db.KindNotes {
		err = db.ErrNotFound
	}
	if h.failed(w, err, "raw page") {
		return
	}
	text, err := transfer.PageMarkdown(n)
	if h.failed(w, err, "raw page") {
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(text)
}

type apiTemplate struct {
	ID          string `json:"id"`
	Title       string `json:"n"`
	Description string `json:"desc,omitempty"`
	Place       string `json:"place,omitempty"`
	// Blocks are the Markdown of UnderDoc's own templates, for a preview.
	Blocks []string `json:"blocks,omitempty"`
	// Kind is the kind a page made from it gets (T-20).
	Kind string `json:"kind,omitempty"`
}

// apiTemplates lists what a new page can start from: UnderDoc's own templates, then the pages offered as one.
func (h *handler) apiTemplates(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusOK, []apiTemplate{})
		return
	}
	user, _ := account.From(r.Context())
	list, err := h.options.DB.Templates(r.Context(), user.ID)
	if h.failed(w, err, "templates") {
		return
	}
	out := []apiTemplate{}
	for _, t := range list {
		out = append(out, apiTemplate{ID: t.ID, Title: t.Title, Description: t.Description, Place: t.Place, Blocks: t.Blocks, Kind: t.Kind})
	}
	writeJSON(w, http.StatusOK, out)
}

// apiTemplate offers a page as a template for new pages, or stops offering it.
func (h *handler) apiTemplate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Template bool `json:"template"`
	}
	user, ok := h.change(w, r, 1<<10, &in)
	if !ok {
		return
	}
	o, err := h.options.DB.SetTemplate(r.Context(), r.PathValue("id"), user.ID, in.Template)
	if h.failed(w, err, "template") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"template": o.Template})
}

// apiAddNote puts a card on top of the viewer's quick notes, made in the private notes the first time (M-8). It answers
// the board, so the workspace can show it, and the card.
func (h *handler) apiAddNote(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Body string `json:"md"`
	}
	user, ok := h.change(w, r, 1<<20, &in)
	if !ok {
		return
	}
	if strings.TrimSpace(in.Body) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a note needs some text"})
		return
	}
	board, card, err := h.options.DB.AddNote(r.Context(), user, in.Body)
	if h.failed(w, err, "note") {
		return
	}
	notes, err := h.options.DB.Object(r.Context(), board.Parent, user.ID)
	if h.failed(w, err, "note") {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"board": apiNode{ID: board.ID, Title: board.Title, Kind: board.Kind, Owner: h.options.DB.AccountName(r.Context(), board.Owner), Mine: true,
			Kids: []apiNode{}, Path: route.Object(board.ID)},
		"in":    notes.ID,
		"block": h.block(r, board.ID, card, nil),
	})
}

// apiAlignBlock aligns a block left, center or right (ADR-16); its text stays as it is.
func (h *handler) apiAlignBlock(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Align string `json:"align"`
	}
	user, ok := h.change(w, r, 1<<10, &in)
	if !ok {
		return
	}
	if h.failed(w, h.options.DB.AlignBlock(r.Context(), r.PathValue("id"), r.PathValue("block"), user.ID, in.Align), "align") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// apiDeleteBlock takes a block out of its page. A block in use needs a way, {"ways": {"*": {"way": …}}} (ADR-22).
func (h *handler) apiDeleteBlock(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Ways map[string]db.Way `json:"ways"`
	}
	var into any
	if r.ContentLength > 0 {
		into = &in
	}
	user, ok := h.change(w, r, 8<<10, into)
	if !ok {
		return
	}
	err := h.options.DB.DeleteBlockWays(r.Context(), r.PathValue("id"), r.PathValue("block"), user.ID, in.Ways)
	if h.inUse(w, r, err) || h.failed(w, err, "delete block") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// apiVersions lists the latest changes of a page, newest first.
func (h *handler) apiVersions(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	user, _ := account.From(r.Context())
	versions, err := h.options.DB.Versions(r.Context(), r.PathValue("id"), user.ID, maxVersions)
	if h.failed(w, err, "versions") {
		return
	}
	out := []apiVersion{}
	for _, v := range versions {
		excerpt := []rune(strings.Join(strings.Fields(v.Body), " "))
		if len(excerpt) > 80 {
			excerpt = append(excerpt[:79], '…')
		}
		out = append(out, apiVersion{Block: v.Block, Author: v.Author, At: stamp(v.At), Deleted: v.Deleted, Excerpt: string(excerpt), Origin: v.Origin, AI: v.Agent})
	}
	writeJSON(w, http.StatusOK, out)
}

// apiCreateDivider puts a line or a space into a collection, folder or page, after the entry it names or first.
func (h *handler) apiCreateDivider(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind   string `json:"kind"`
		Parent string `json:"parent"`
		After  string `json:"after"`
	}
	user, ok := h.change(w, r, 8<<10, &in)
	if !ok {
		return
	}
	made, err := h.options.DB.CreateDivider(r.Context(), user, in.Kind, in.Parent, in.After)
	if h.failed(w, err, "divider") {
		return
	}
	writeJSON(w, http.StatusCreated, apiNode{ID: made.ID, Title: made.Title, Kind: made.Kind, Kids: []apiNode{}})
}

// apiDeleteObject takes an object away: a line or a space for good, anything else into the trash with everything below it.
// What of it is embedded from outside needs a way, {"ways": {"<address>" or "*": {"way": …}}} (ADR-22).
func (h *handler) apiDeleteObject(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Ways map[string]db.Way `json:"ways"`
	}
	var into any
	if r.ContentLength > 0 {
		into = &in
	}
	user, ok := h.change(w, r, 64<<10, into)
	if !ok {
		return
	}
	err := h.options.DB.DeleteWays(r.Context(), r.PathValue("id"), user.ID, in.Ways)
	if h.inUse(w, r, err) || h.failed(w, err, "delete") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type apiTrashed struct {
	ID    string `json:"id"`
	Title string `json:"n"`
	Kind  string `json:"t"`
	Place string `json:"place"`
	// PlaceDeleted tells that what it lay in is in the trash too and has to be restored first.
	PlaceDeleted bool   `json:"placeDeleted,omitempty"`
	At           string `json:"at"`
	Who          string `json:"who"`
	// Purge tells that the viewer may delete it for good: its owner or an administrator, in the browser (B-3).
	Purge bool `json:"purge,omitempty"`
}

// apiTrash lists what the viewer deleted or owns and was deleted, newest first.
func (h *handler) apiTrash(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	user, _ := account.From(r.Context())
	trash, err := h.options.DB.Trash(r.Context(), user.ID)
	if h.failed(w, err, "trash") {
		return
	}
	out := []apiTrashed{}
	purger := !user.Agent() && user.AuthKind == account.AuthSession
	for _, t := range trash {
		out = append(out, apiTrashed{ID: t.ID, Title: t.Title, Kind: t.Kind, Place: t.Place, PlaceDeleted: t.PlaceDeleted,
			At: stamp(t.DeletedAt), Who: t.DeletedBy, Purge: purger && (t.Owner == user.ID || user.Admin)})
	}
	writeJSON(w, http.StatusOK, out)
}

// apiRestore puts an object of the trash back where it lay, and answers with that place and the object with its
// children, as the tree shows them.
func (h *handler) apiRestore(w http.ResponseWriter, r *http.Request) {
	user, ok := h.change(w, r, 0, nil)
	if !ok {
		return
	}
	back, err := h.options.DB.Restore(r.Context(), r.PathValue("id"), user.ID)
	if h.failed(w, err, "restore") {
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Parent string  `json:"parent"`
		Node   apiNode `json:"node"`
	}{back.Parent, h.databaseNode(r, back, 0)})
}

// apiPurge takes an object of the trash away for good: its owner or an administrator, in the browser, with its name
// typed as confirm (B-3).
func (h *handler) apiPurge(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirm string `json:"confirm"`
	}
	if _, ok := h.change(w, r, 1<<10, &in); !ok {
		return
	}
	if h.failed(w, h.options.DB.Purge(r.Context(), r.PathValue("id"), h.actor(r), in.Confirm), "purge") {
		return
	}
	h.note(r, db.LogPurge, "", "deleted for good "+r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

// apiMove puts an object into a parent, after the entry it names or first.
func (h *handler) apiMove(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Parent string `json:"parent"`
		After  string `json:"after"`
	}
	user, ok := h.change(w, r, 8<<10, &in)
	if !ok {
		return
	}
	moved, err := h.options.DB.Move(r.Context(), r.PathValue("id"), user.ID, in.Parent, in.After)
	if h.failed(w, err, "move") {
		return
	}
	writeJSON(w, http.StatusOK, apiNode{ID: moved.ID, Title: moved.Title, Kind: moved.Kind, Kids: []apiNode{}})
}

// apiCopy duplicates an object with everything below it into a parent, after an entry or first, and answers with the
// copy and its children, as the tree shows them.
func (h *handler) apiCopy(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Parent string `json:"parent"`
		After  string `json:"after"`
	}
	user, ok := h.change(w, r, 8<<10, &in)
	if !ok {
		return
	}
	made, err := h.options.DB.Copy(r.Context(), r.PathValue("id"), user.ID, in.Parent, in.After)
	if h.failed(w, err, "copy") {
		return
	}
	writeJSON(w, http.StatusCreated, h.databaseNode(r, made, 0))
}

// apiCreateFolder makes a folder in a parent; a folder never stands at the top.
func (h *handler) apiCreateFolder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title  string `json:"title"`
		Parent string `json:"parent"`
	}
	user, ok := h.change(w, r, 8<<10, &in)
	if !ok {
		return
	}
	created, err := h.options.DB.CreateFolder(r.Context(), user, in.Title, in.Parent)
	if h.failed(w, err, "folder") {
		return
	}
	h.noteMade(r, created.ID)
	writeJSON(w, http.StatusCreated, apiNode{ID: created.ID, Title: created.Title, Kind: created.Kind, Kids: []apiNode{}, Path: route.Object(created.ID)})
}

// apiConvert turns a collection into a folder or back.
func (h *handler) apiConvert(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind string `json:"kind"`
	}
	user, ok := h.change(w, r, 1<<10, &in)
	if !ok {
		return
	}
	made, err := h.options.DB.Convert(r.Context(), r.PathValue("id"), user.ID, in.Kind)
	if h.failed(w, err, "convert") {
		return
	}
	writeJSON(w, http.StatusOK, apiNode{ID: made.ID, Title: made.Title, Kind: made.Kind, Kids: []apiNode{}})
}

// apiSettings replaces the settings of the workspace of the account: the browser sends them whole when they change.
func (h *handler) apiSettings(w http.ResponseWriter, r *http.Request) {
	var in json.RawMessage
	user, ok := h.change(w, r, db.MaxSettings, &in)
	if !ok {
		return
	}
	if h.failed(w, h.options.DB.SaveSettings(r.Context(), user.ID, in), "settings") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// apiAccess sets what the whole tenant may do with an object; whoever manages it may, and administrators (ADR-29).
func (h *handler) apiAccess(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Access string `json:"access"`
	}
	user, ok := h.change(w, r, 1<<10, &in)
	if !ok {
		return
	}
	o, err := h.options.DB.SetAccess(r.Context(), r.PathValue("id"), db.Actor{ID: user.ID, Admin: user.Admin}, in.Access)
	if h.failed(w, err, "access") {
		return
	}
	h.note(r, db.LogGrant, o.ID, "everyone "+cmp.Or(in.Access, "as above"))
	writeJSON(w, http.StatusOK, apiNode{ID: o.ID, Title: o.Title, Kind: o.Kind, Access: o.Access, Kids: []apiNode{}})
}

// apiEmptyTrash takes away for good everything in the trash the account may purge, with db.TrashName typed as
// confirm (B-3).
func (h *handler) apiEmptyTrash(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Confirm string `json:"confirm"`
	}
	if _, ok := h.change(w, r, 1<<10, &in); !ok {
		return
	}
	n, err := h.options.DB.EmptyTrash(r.Context(), h.actor(r), in.Confirm)
	if h.failed(w, err, "empty trash") {
		return
	}
	h.note(r, db.LogPurge, "", fmt.Sprintf("emptied the trash: %d deleted for good", n))
	w.WriteHeader(http.StatusNoContent)
}

// anchorOf is the fragment that opens an address at its place in the page, none for a whole page.
func anchorOf(a db.Address) string {
	if a.Block == "" {
		return ""
	}
	return "#" + a.Target()
}
