// SPDX-License-Identifier: MIT

package web

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/id"
)

// The log of access and changes (ADR-29, M-5): opening and changing objects, and every change of rights, groups,
// administrators and links. A log that cannot be written is reported, and the request goes on.

// note writes an entry of the log in the name of whoever made the request, or of nobody for a visitor without an account.
func (h *handler) note(r *http.Request, action, objectID, detail string) {
	if h.options.DB == nil {
		return
	}
	user, _ := account.From(r.Context())
	if err := h.options.DB.Log(r.Context(), user.ID, action, objectID, detail); err != nil {
		h.log.Error("log not written", slog.String("action", action), slog.String("error", err.Error()))
	}
}

// noteOpen logs that an object was opened, unless it is private: what lies in private notes is nobody's business.
func (h *handler) noteOpen(r *http.Request, o db.Object) {
	if o.Private {
		return
	}
	if private, err := h.options.DB.Private(r.Context(), o.ID); err != nil || private {
		return
	}
	h.note(r, db.LogOpen, o.ID, "")
}

// noteMade logs that an object was made, unless it is private.
func (h *handler) noteMade(r *http.Request, objectID string) {
	if private, err := h.options.DB.Private(r.Context(), objectID); err != nil || private {
		return
	}
	h.note(r, db.LogChange, objectID, "made")
}

// changeRoutes are the paths of the API under which a successful change changes the object named after their prefix.
// The rights have entries of their own (note), with what was changed.
var changeRoutes = []string{"/api/objects/", "/api/pages/", "/api/files/"}

// rightsPaths end the paths whose changes are logged by their handlers, with what was granted or taken back.
var rightsPaths = []string{"/grants", "/inherit", "/owner", "/links", "/access"}

// logged writes "change" for every change of the API that went through, once per account, object and hour.
func (h *handler) logged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || h.options.DB == nil {
			next.ServeHTTP(w, r)
			return
		}
		writer := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(writer, r)
		if writer.status >= 400 {
			return
		}
		objectID := changedObject(r.URL.Path)
		if objectID == "" {
			return
		}
		if private, err := h.options.DB.Private(r.Context(), objectID); err != nil || private {
			return
		}
		h.note(r, db.LogChange, objectID, r.Method+" "+strings.TrimPrefix(r.URL.Path, "/api/"))
	})
}

// changedObject is the object a change of the API names, or "" for a change the log keeps otherwise or none at all.
func changedObject(path string) string {
	for _, suffix := range rightsPaths {
		if strings.HasSuffix(path, suffix) {
			return ""
		}
	}
	for _, prefix := range changeRoutes {
		rest, ok := strings.CutPrefix(path, prefix)
		if !ok {
			continue
		}
		objectID, _, _ := strings.Cut(rest, "/")
		if id.Valid(objectID) {
			return objectID
		}
	}
	return ""
}

type apiEntry struct {
	ID          int64  `json:"id"`
	At          string `json:"at"`
	Actor       string `json:"actor"`
	ActorName   string `json:"actorName"`
	Action      string `json:"action"`
	Object      string `json:"object,omitempty"`
	ObjectTitle string `json:"objectTitle,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

// apiLog answers the log: administrators read all of it, everyone else what they did and what happened to what they
// manage. ?object=, ?actor= and ?before= narrow it.
func (h *handler) apiLog(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	q := r.URL.Query()
	before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	entries, err := h.options.DB.LogEntries(r.Context(), h.actor(r), db.LogFilter{Object: q.Get("object"), Actor: q.Get("actor"), Before: before, Limit: limit})
	if h.failed(w, err, "log") {
		return
	}
	out := []apiEntry{}
	for _, e := range entries {
		out = append(out, apiEntry{ID: e.ID, At: stamp(e.At), Actor: e.Actor, ActorName: e.ActorName, Action: e.Action, Object: e.Object,
			ObjectTitle: e.ObjectTitle, Detail: e.Detail})
	}
	writeJSON(w, http.StatusOK, out)
}
