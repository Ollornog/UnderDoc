// SPDX-License-Identifier: MIT

package web

import (
	"errors"
	"net/http"

	"github.com/Ollornog/UnderDoc/internal/db"
)

// The rules page for agents (B-5, docs/mcp.md#the-rules-page): an administrator names it in the browser.

// apiRulesPage answers the rules page as the viewer reads it: its ID and title, or none.
func (h *handler) apiRulesPage(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	o, err := h.options.DB.RulesPage(r.Context(), h.me(r).ID)
	if err != nil {
		if h.failed(w, ignoreNotFound(err), "rules page") {
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"page": "", "title": ""})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"page": o.ID, "title": o.Title})
}

// apiSetRulesPage names the rules page, or none with an empty page: administrators, in the browser.
func (h *handler) apiSetRulesPage(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Page string `json:"page"`
	}
	if _, ok := h.change(w, r, 1<<10, &in); !ok {
		return
	}
	if h.failed(w, h.options.DB.SetRulesPage(r.Context(), h.actor(r), in.Page), "rules page") {
		return
	}
	detail := "rules page " + in.Page
	if in.Page == "" {
		detail = "no rules page"
	}
	h.note(r, db.LogAgents, in.Page, detail)
	w.WriteHeader(http.StatusNoContent)
}

// ignoreNotFound is nil for ErrNotFound, the error otherwise.
func ignoreNotFound(err error) error {
	if errors.Is(err, db.ErrNotFound) {
		return nil
	}
	return err
}
