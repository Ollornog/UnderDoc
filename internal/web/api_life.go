// SPDX-License-Identifier: MIT

package web

import (
	"errors"
	"html"
	"net/http"
	"strconv"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/route"
)

// The end of something in use (ADR-22): the question with every place of use, the placeholder where it was deleted
// everywhere, restoring it, keeping a copy, redirects and notices.

type apiInUse struct {
	Ref    string   `json:"ref"`
	Uses   []apiUse `json:"uses"`
	Hidden int      `json:"hidden"`
}

// apiLost answers, without changing anything, what in use a new text of a block, or deleting it, would take away, with
// every place of use: {"md": …} for a new text, {"delete": true} for deleting.
func (h *handler) apiLost(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Body   *string `json:"md"`
		Delete bool    `json:"delete"`
	}
	user, ok := h.change(w, r, 1<<20, &in)
	if !ok {
		return
	}
	if in.Delete {
		in.Body = nil
	} else if in.Body == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "md or delete"})
		return
	}
	refs, err := h.options.DB.Lost(r.Context(), r.PathValue("id"), r.PathValue("block"), user.ID, in.Body)
	if h.failed(w, err, "lost") {
		return
	}
	out, ok := h.usesOfRefs(w, r, refs)
	if ok {
		writeJSON(w, http.StatusOK, map[string]any{"inUse": out})
	}
}

// usesOfRefs lists each address with its places of use.
func (h *handler) usesOfRefs(w http.ResponseWriter, r *http.Request, refs []string) ([]apiInUse, bool) {
	user, _ := account.From(r.Context())
	out := []apiInUse{}
	for _, ref := range refs {
		a, _ := db.ParseAddress(ref)
		uses, hidden, err := h.options.DB.UsesOf(r.Context(), a, user.ID)
		if err != nil {
			h.failed(w, err, "uses")
			return nil, false
		}
		item := apiInUse{Ref: ref, Uses: []apiUse{}, Hidden: hidden}
		for _, u := range uses {
			item.Uses = append(item.Uses, apiUse{Page: u.Page, Title: u.Title, Block: u.Block, CanEdit: u.CanEdit})
		}
		out = append(out, item)
	}
	return out, true
}

// inUse answers a change that takes away something in use without a way for it: 409, with each thing and its uses, so
// the workspace can ask. It reports whether it answered.
func (h *handler) inUse(w http.ResponseWriter, r *http.Request, err error) bool {
	var used *db.InUse
	if !errors.As(err, &used) {
		return false
	}
	out, ok := h.usesOfRefs(w, r, used.Refs)
	if !ok {
		return true
	}
	writeJSON(w, http.StatusConflict, map[string]any{"error": "in use elsewhere: choose a way", "inUse": out})
	return true
}

// gone is why an embedding shows a placeholder instead of its content.
type gone struct {
	removed   *db.Removed
	trashedBy string
	at        time.Time
}

// goneAt finds why an address shows nothing: deleted everywhere, or its page in the trash.
func (h *handler) goneAt(r *http.Request, a db.Address) *gone {
	user, _ := account.From(r.Context())
	if by, at, ok := h.options.DB.TrashedAt(r.Context(), a.Object, user.ID); ok {
		return &gone{trashedBy: by, at: at}
	}
	if a.Block == "" {
		return nil
	}
	if removed, ok := h.options.DB.RemovedAt(r.Context(), a, user.ID); ok {
		return &gone{removed: &removed, at: removed.At}
	}
	return nil
}

// placeholder says who removed an embedded thing and when, instead of letting it vanish quietly (ADR-22). Who may change
// the page it came from can restore it; who may change the page it is embedded in can keep a copy there.
func (h *handler) placeholder(r *http.Request, a db.Address, g *gone, page string, inline bool) string {
	user, _ := account.From(r.Context())
	day := g.at.UTC().Format("2006-01-02")
	text := "Removed by " + g.trashedBy + " on " + day + ", in the trash"
	actions := ""
	if g.removed != nil {
		text = "Removed by " + g.removed.ByName + " on " + day + " from " + g.removed.PageName
		id := strconv.FormatInt(g.removed.ID, 10)
		if o, err := h.options.DB.Object(r.Context(), a.Object, user.ID); err == nil && h.options.DB.MayEdit(r.Context(), o, user.ID) {
			actions += `<button type="button" class="btn" data-variant="outline" data-size="xs" data-act="restoreremoved" data-id="` + id + `">Restore</button>`
		}
		if o, err := h.options.DB.Object(r.Context(), page, user.ID); err == nil && page != a.Object && h.options.DB.MayEdit(r.Context(), o, user.ID) {
			actions += `<button type="button" class="btn" data-variant="outline" data-size="xs" data-act="keepcopy" data-id="` + id + `" data-page="` + html.EscapeString(page) + `">Keep a copy here</button>`
		}
	}
	if inline {
		return `<span class="badge" data-variant="destructive" data-ref="` + html.EscapeString(a.String()) + `" data-removed>` + html.EscapeString(text) + `</span>` + actions
	}
	return `<figure class="editor-embed" data-state="missing" data-ref="` + html.EscapeString(a.String()) + `" data-removed><div class="flex flex-wrap items-center gap-2"><p>` +
		html.EscapeString(text) + `</p>` + actions + `</div></figure>`
}

// apiRestoreRemoved puts what was deleted everywhere back into its page.
func (h *handler) apiRestoreRemoved(w http.ResponseWriter, r *http.Request) {
	user, ok := h.change(w, r, 0, nil)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.failed(w, db.ErrNotFound, "restore")
		return
	}
	if h.failed(w, h.options.DB.RestoreRemoved(r.Context(), id, user.ID), "restore") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// apiKeepCopy writes a copy of what was deleted everywhere in place of its embeddings in a page the viewer may change.
func (h *handler) apiKeepCopy(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Page string `json:"page"`
	}
	user, ok := h.change(w, r, 1<<10, &in)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.failed(w, db.ErrNotFound, "keep copy")
		return
	}
	if h.failed(w, h.options.DB.KeepCopy(r.Context(), id, in.Page, user.ID), "keep copy") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// apiRedirect answers where the content of an old address lives now, for a link or bookmark that names it.
func (h *handler) apiRedirect(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	a, ok := db.ParseAddress(r.URL.Query().Get("ref"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not an address"})
		return
	}
	to, moved := h.options.DB.Redirect(r.Context(), a)
	if !moved {
		h.failed(w, db.ErrNotFound, "redirect")
		return
	}
	user, _ := account.From(r.Context())
	if _, err := h.options.DB.Object(r.Context(), to.Object, user.ID); h.failed(w, err, "redirect") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ref": to.String(), "url": route.Object(to.Object) + "#" + to.Target()})
}

type apiNotice struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"`
	Page string `json:"page"`
	Ref  string `json:"ref"`
	Text string `json:"text"`
	At   string `json:"at"`
	Read bool   `json:"read,omitempty"`
}

// apiNotices lists what the viewer was told about pages of theirs; the inbox of M-16 shows them.
func (h *handler) apiNotices(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	user, _ := account.From(r.Context())
	notices, err := h.options.DB.Notices(r.Context(), user.ID, 100)
	if h.failed(w, err, "notices") {
		return
	}
	out := []apiNotice{}
	for _, n := range notices {
		out = append(out, apiNotice{ID: n.ID, Kind: n.Kind, Page: n.Page, Ref: n.Ref, Text: n.String(), At: stamp(n.At), Read: n.Read})
	}
	writeJSON(w, http.StatusOK, out)
}
