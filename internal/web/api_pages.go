// SPDX-License-Identifier: MIT

package web

import (
	"encoding/json"
	"errors"
	"html"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/route"
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
	// Comments counts the comments on this block in threads not resolved.
	Comments int `json:"comments,omitempty"`
}

type apiVersion struct {
	Block   string `json:"block"`
	Author  string `json:"who"`
	At      string `json:"at"`
	Deleted bool   `json:"deleted,omitempty"`
	Excerpt string `json:"excerpt"`
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
	switch {
	case err == nil:
		return false
	case errors.Is(err, db.ErrTitle), errors.Is(err, db.ErrBlock), errors.Is(err, db.ErrPlace), errors.Is(err, db.ErrTooMany), errors.Is(err, db.ErrSettings), errors.Is(err, db.ErrComment):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, db.ErrHasReplies):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, db.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case errors.Is(err, db.ErrNotAllowed):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
	default:
		h.log.Error(what+" failed", slog.String("error", err.Error()))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
	return true
}

// block renders a block for the workspace. Raw HTML in it is dropped, as in every page (TestUserTextStaysText).
func (h *handler) block(r *http.Request, page string, b db.Block, used []db.Ref) apiBlock {
	out := apiBlock{ID: b.ID, Markdown: b.Body, HTML: h.render(r, b.Body, []string{page, page + "#" + b.ID}), UsedIn: []string{}}
	for _, u := range used {
		if u.TargetBlock == b.ID && !slices.Contains(out.UsedIn, u.Title) {
			out.UsedIn = append(out.UsedIn, u.Title)
		}
	}
	return out
}

// maxEmbed is how deep embeddings may nest: a block that embeds a block that embeds a block, and so on.
const maxEmbed = 5

// render turns the Markdown of a block into HTML. A line ::embed{ref="…"} becomes what it refers to, rendered for this
// viewer each time, so it is live, with a link back to where it comes from (ADR-18). What cannot be shown says so, visibly:
// a missing source is an error to see, never a silent gap. trail holds what is being rendered, to stop at a loop.
func (h *handler) render(r *http.Request, body string, trail []string) string {
	var out strings.Builder
	last := 0
	for _, m := range db.EmbedLine.FindAllStringSubmatchIndex(body, -1) {
		out.WriteString(h.markdown(body[last:m[0]]))
		block := ""
		if m[4] >= 0 {
			block = body[m[4]:m[5]]
		}
		out.WriteString(h.embed(r, body[m[2]:m[3]], block, trail))
		last = m[1]
	}
	out.WriteString(h.markdown(body[last:]))
	return out.String()
}

func (h *handler) markdown(md string) string {
	if strings.TrimSpace(md) == "" {
		return ""
	}
	rendered, err := h.store.Library().RenderMarkdown(md)
	if err != nil {
		return "<pre>" + html.EscapeString(md) + "</pre>"
	}
	return rendered.HTML
}

// embed renders one embedded block, or a whole page, of the database.
func (h *handler) embed(r *http.Request, target, block string, trail []string) string {
	ref := target
	if block != "" {
		ref += "#" + block
	}
	broken := func(why string) string {
		return `<div class="embed broken" data-ref="` + html.EscapeString(ref) + `">` + html.EscapeString(why) + `</div>`
	}
	loop := slices.Contains(trail, ref) || slices.Contains(trail, target)
	if block == "" {
		loop = loop || slices.ContainsFunc(trail, func(x string) bool { return strings.HasPrefix(x, target+"#") })
	}
	if loop {
		return broken("This embeds itself: " + ref)
	}
	if len(trail) > maxEmbed {
		return broken("Embedded too deep: " + ref)
	}
	user, _ := account.From(r.Context())
	o, err := h.options.DB.Object(r.Context(), target, user.ID)
	if err != nil || o.Kind != db.KindPage {
		return broken("Not to be found: " + ref)
	}
	blocks, err := h.options.DB.Blocks(r.Context(), target, user.ID)
	if err != nil {
		return broken("Not to be found: " + ref)
	}
	var inner strings.Builder
	found := false
	for _, b := range blocks {
		if block == "" || b.ID == block {
			found = true
			inner.WriteString(h.render(r, b.Body, append(slices.Clone(trail), ref, target+"#"+b.ID)))
		}
	}
	if block != "" && !found {
		return broken("Block #" + block + " is no longer in " + o.Title)
	}
	label := o.Title
	if block != "" {
		label += " · #" + block
	}
	link := route.Object(target)
	if block != "" {
		link += "#" + block
	}
	return `<figure class="embed" data-ref="` + html.EscapeString(ref) + `"><div class="embedbody">` + inner.String() +
		`</div><figcaption><a href="` + html.EscapeString(link) + `">` + html.EscapeString(label) + `</a></figcaption></figure>`
}

// pageDocument answers /api/doc for a page of the database: its blocks, its place and whether the viewer may change it.
func (h *handler) pageDocument(w http.ResponseWriter, r *http.Request, o db.Object) {
	ctx := r.Context()
	user, _ := account.From(ctx)
	blocks, err := h.options.DB.Blocks(ctx, o.ID, user.ID)
	if h.failed(w, err, "blocks") {
		return
	}
	answer := apiDocument{ID: o.ID, Title: o.Title, Kind: o.Kind, Description: o.Description, Tags: []string{},
		Place: h.place(r, o), Path: "not a file, lives in the database", Changed: stamp(o.Changed),
		Sections: [][]string{}, UsedIn: []string{}, Links: []string{}, Stable: true, Blocks: []apiBlock{},
		Owner: h.options.DB.AccountName(ctx, o.Owner), Created: stamp(o.Created)}
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
	writeJSON(w, http.StatusOK, answer)
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

// apiCreatePage makes an empty page inside a collection, folder or page.
func (h *handler) apiCreatePage(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title  string `json:"title"`
		Parent string `json:"parent"`
	}
	user, ok := h.change(w, r, 8<<10, &in)
	if !ok {
		return
	}
	created, err := h.options.DB.CreatePage(r.Context(), user, in.Title, in.Parent)
	if h.failed(w, err, "page") {
		return
	}
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
	writeJSON(w, http.StatusCreated, h.block(r, r.PathValue("id"), b, nil))
}

// apiMoveBlock puts a block after another, or first.
func (h *handler) apiMoveBlock(w http.ResponseWriter, r *http.Request) {
	var in struct {
		After string `json:"after"`
	}
	user, ok := h.change(w, r, 1<<10, &in)
	if !ok {
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
		Body string `json:"md"`
	}
	user, ok := h.change(w, r, 1<<20, &in)
	if !ok {
		return
	}
	b, err := h.options.DB.EditBlock(r.Context(), r.PathValue("id"), r.PathValue("block"), user.ID, in.Body)
	if h.failed(w, err, "block") {
		return
	}
	used, _ := h.options.DB.UsedIn(r.Context(), r.PathValue("id"), user.ID)
	writeJSON(w, http.StatusOK, h.block(r, r.PathValue("id"), b, used))
}

// apiDeleteBlock takes a block out of its page.
func (h *handler) apiDeleteBlock(w http.ResponseWriter, r *http.Request) {
	user, ok := h.change(w, r, 0, nil)
	if !ok {
		return
	}
	if h.failed(w, h.options.DB.DeleteBlock(r.Context(), r.PathValue("id"), r.PathValue("block"), user.ID), "delete block") {
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
		out = append(out, apiVersion{Block: v.Block, Author: v.Author, At: stamp(v.At), Deleted: v.Deleted, Excerpt: string(excerpt)})
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
func (h *handler) apiDeleteObject(w http.ResponseWriter, r *http.Request) {
	user, ok := h.change(w, r, 0, nil)
	if !ok {
		return
	}
	if h.failed(w, h.options.DB.Delete(r.Context(), r.PathValue("id"), user.ID), "delete") {
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
	for _, t := range trash {
		out = append(out, apiTrashed{ID: t.ID, Title: t.Title, Kind: t.Kind, Place: t.Place, PlaceDeleted: t.PlaceDeleted,
			At: stamp(t.DeletedAt), Who: t.DeletedBy})
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

// apiPurge takes an object of the trash away for good.
func (h *handler) apiPurge(w http.ResponseWriter, r *http.Request) {
	user, ok := h.change(w, r, 0, nil)
	if !ok {
		return
	}
	if h.failed(w, h.options.DB.Purge(r.Context(), r.PathValue("id"), user.ID), "purge") {
		return
	}
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

// apiAccess sets what all others may do with an object; only its owner may.
func (h *handler) apiAccess(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Access string `json:"access"`
	}
	user, ok := h.change(w, r, 1<<10, &in)
	if !ok {
		return
	}
	o, err := h.options.DB.SetAccess(r.Context(), r.PathValue("id"), user.ID, in.Access)
	if h.failed(w, err, "access") {
		return
	}
	writeJSON(w, http.StatusOK, apiNode{ID: o.ID, Title: o.Title, Kind: o.Kind, Access: o.Access, Kids: []apiNode{}})
}

// apiEmptyTrash takes away for good everything in the trash the account may purge.
func (h *handler) apiEmptyTrash(w http.ResponseWriter, r *http.Request) {
	user, ok := h.change(w, r, 0, nil)
	if !ok {
		return
	}
	if _, err := h.options.DB.EmptyTrash(r.Context(), user.ID); h.failed(w, err, "empty trash") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
