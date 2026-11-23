// SPDX-License-Identifier: MIT

package web

import (
	"log/slog"
	"net/http"

	"github.com/Ollornog/UnderDoc/internal/db"
)

// The archive of spaces (T-34, docs/rights.md#archived-spaces): archiving and restoring a space, and what the workspace
// shows of it.

// apiSetArchived archives a space ({"archived": true}) or restores it ({"archived": false}): its owner or an
// administrator, in the browser, never an agent, a service account, a key or an app password.
func (h *handler) apiSetArchived(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Archived *bool `json:"archived"`
	}
	if _, ok := h.change(w, r, 1<<10, &in); !ok {
		return
	}
	if in.Archived == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": `say {"archived": true} or {"archived": false}`})
		return
	}
	if h.failed(w, h.options.DB.SetArchived(r.Context(), r.PathValue("id"), h.actor(r), *in.Archived), "archive") {
		return
	}
	detail := "restored"
	if *in.Archived {
		detail = "archived"
	}
	h.note(r, db.LogArchive, r.PathValue("id"), detail)
	w.WriteHeader(http.StatusNoContent)
}

// mayArchive tells whether the viewer may archive or restore a space, for the workspace to offer it; each change checks
// again.
func (h *handler) mayArchive(r *http.Request, o db.Object) bool {
	if o.Parent != "" || o.Kind != db.KindCollection || o.Private {
		return false
	}
	ok, err := h.options.DB.MayArchive(r.Context(), o.ID, h.actor(r))
	if err != nil {
		h.log.Error("archive check failed", slog.String("error", err.Error()))
	}
	return ok
}

// inArchive tells whether an object lies in an archived space, or is one.
func (h *handler) inArchive(r *http.Request, o db.Object) bool {
	if o.Archived {
		return true
	}
	in, err := h.options.DB.InArchive(r.Context(), o.ID)
	if err != nil {
		h.log.Error("archive check failed", slog.String("error", err.Error()))
	}
	return in
}
