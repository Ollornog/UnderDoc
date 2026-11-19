// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/files"
)

// The text of files for the search (ADR-24): what the browser of the uploader reads comes in here, the server reads in a
// queue, and each way is switched on by the hoster (Options.TextWays), the tenant and the user.

type apiWays struct {
	Browser bool `json:"browser"`
	Server  bool `json:"server"`
	AI      bool `json:"ai"`
}

type apiWaySettings struct {
	Browser *bool `json:"browser"`
	Server  *bool `json:"server"`
	AI      *bool `json:"ai"`
}

func waysOut(w db.Ways) apiWays { return apiWays{Browser: w.Browser, Server: w.Server, AI: w.AI} }

// queueText puts the reading of a new version on the queue of the server, the ways the uploader has on.
func (h *handler) queueText(ctx context.Context, user account.User, done db.Finished, sha, contentType string) {
	if done.File.ID == "" {
		return
	}
	ways, err := h.options.DB.TextWays(ctx, h.options.TextWays, user.ID)
	if err != nil {
		h.log.Error("text ways", slog.String("error", err.Error()))
		return
	}
	var jobs []string
	if ways.Server && files.HasServerText(contentType) {
		jobs = append(jobs, files.TextServer)
	}
	if ways.Server && h.options.OCR != nil && files.IsImage(contentType) {
		jobs = append(jobs, files.TextOCR)
	}
	if ways.AI && h.options.AIOCR != nil && files.IsImage(contentType) {
		jobs = append(jobs, files.TextAI)
	}
	if err := h.options.DB.QueueText(ctx, sha, contentType, user.ID, jobs...); err != nil {
		h.log.Error("text queue", slog.String("error", err.Error()))
	}
}

// apiFileText returns the texts of a file with where each came from.
func (h *handler) apiFileText(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	user, _ := account.From(r.Context())
	texts, err := h.options.DB.FileTexts(r.Context(), r.PathValue("id"), user.ID)
	if h.failed(w, err, "file text") {
		return
	}
	out := make([]map[string]string, 0, len(texts))
	for _, t := range texts {
		out = append(out, map[string]string{"source": t.Source, "text": t.Text, "at": stamp(t.At)})
	}
	writeJSON(w, http.StatusOK, out)
}

// apiPutFileText takes the text the browser of a user read from a file, while that way is on for them.
func (h *handler) apiPutFileText(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	user, ok := h.change(w, r, db.MaxFileText+(64<<10), &body)
	if !ok {
		return
	}
	ways, err := h.options.DB.TextWays(r.Context(), h.options.TextWays, user.ID)
	if h.failed(w, err, "file text") {
		return
	}
	if !ways.Browser {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "reading text in the browser is off"})
		return
	}
	if err := h.options.DB.SaveFileText(r.Context(), r.PathValue("id"), user.ID, files.TextBrowser, body.Text); err != nil {
		if err == db.ErrText {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		h.failed(w, err, "file text")
		return
	}
	// The search learns of the text from the database itself (migration 020): the file goes into its queue.
	w.WriteHeader(http.StatusNoContent)
}

// apiTextSettings tells which ways are on: what the hoster allows, what the tenant and the user set, and what holds.
func (h *handler) apiTextSettings(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	user, _ := account.From(r.Context())
	tenant, err := h.options.DB.TextSettings(r.Context(), "")
	if h.failed(w, err, "text settings") {
		return
	}
	mine, err := h.options.DB.TextSettings(r.Context(), user.ID)
	if h.failed(w, err, "text settings") {
		return
	}
	ways, err := h.options.DB.TextWays(r.Context(), h.options.TextWays, user.ID)
	if h.failed(w, err, "text settings") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hoster": waysOut(h.options.TextWays), "tenant": apiWaySettings(tenant), "mine": apiWaySettings(mine),
		"ways": waysOut(ways), "admin": h.isAdmin(r)})
}

// apiSetTextSettings sets the ways of the tenant, which only administrators do, or the user's own.
func (h *handler) apiSetTextSettings(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	var body struct {
		Scope string `json:"scope"`
		apiWaySettings
	}
	user, ok := h.change(w, r, 1<<10, &body)
	if !ok {
		return
	}
	scope := user.ID
	switch body.Scope {
	case "tenant":
		if !h.isAdmin(r) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "only administrators set the ways of the tenant"})
			return
		}
		scope = ""
	case "mine":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope is tenant or mine"})
		return
	}
	if h.failed(w, h.options.DB.SetTextSettings(r.Context(), scope, db.WaySettings(body.apiWaySettings)), "text settings") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
