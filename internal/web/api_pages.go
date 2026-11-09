// SPDX-License-Identifier: MIT

package web

import (
	"encoding/json"
	"errors"
	"html"
	"log/slog"
	"net/http"
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
	case errors.Is(err, db.ErrTitle), errors.Is(err, db.ErrBlock), errors.Is(err, db.ErrPlace), errors.Is(err, db.ErrTooMany):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
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
func (h *handler) block(b db.Block) apiBlock {
	out := apiBlock{ID: b.ID, Markdown: b.Body}
	rendered, err := h.store.Library().RenderMarkdown(b.Body)
	if err != nil {
		out.HTML = "<pre>" + html.EscapeString(b.Body) + "</pre>"
		return out
	}
	out.HTML = rendered.HTML
	return out
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
		Place: h.place(r, o), Path: "not a file, lives in the database", Changed: o.Changed.Format("Jan 02 15:04"),
		Sections: [][]string{}, UsedIn: []string{}, Links: []string{}, Stable: true, Blocks: []apiBlock{},
		Owner: h.options.DB.AccountName(ctx, o.Owner), Created: o.Created.Format("Jan 02")}
	for _, b := range blocks {
		answer.Blocks = append(answer.Blocks, h.block(b))
	}
	answer.CanEdit = h.options.DB.MayEdit(ctx, o, user.ID)
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
	writeJSON(w, http.StatusCreated, h.block(b))
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
	writeJSON(w, http.StatusOK, h.block(b))
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
		out = append(out, apiVersion{Block: v.Block, Author: v.Author, At: v.At.Format("Jan 02 15:04"), Deleted: v.Deleted, Excerpt: string(excerpt)})
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

// apiDeleteObject takes an object away. For now only a line or a space: deleting what may be in use comes with its
// question (M-28).
func (h *handler) apiDeleteObject(w http.ResponseWriter, r *http.Request) {
	user, ok := h.change(w, r, 0, nil)
	if !ok {
		return
	}
	if h.failed(w, h.options.DB.DeleteDivider(r.Context(), r.PathValue("id"), user.ID), "delete") {
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
