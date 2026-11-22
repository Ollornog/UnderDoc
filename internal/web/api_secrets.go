// SPDX-License-Identifier: MIT

package web

import (
	"errors"
	"html"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/markdown"
	"github.com/Ollornog/UnderDoc/internal/vault"
)

// Passwords in the text (ADR-7, docs/secrets.md). The text holds ::secret{id="…"}; a page shows a mask in its place, and
// the value comes from the server on a click only, through reveal, logged before it is answered. Nothing else ever
// carries a value: not the page, not an embedding, not an export, not a frozen link.

// The limit of reveals per account (db.RevealsPerWindow) is held twice: in the memory of this instance, which turns a
// burst away before it reaches the database and covers what lies in something private, and in the log, which every
// instance shares and a start does not empty.
const (
	// maxSecret is the longest value: a password, a key or a short certificate, not a document.
	maxSecret = 16 << 10
)

// errNoVault answers when the instance has no key: secrets are neither made nor revealed, and nothing falls back to
// plain text.
const errNoVault = "Passwords need a key on the server: " + vault.KeyEnv + " is not set."

// --- Rendering

// passwords puts each secret of a rendered block in place of its sentinel, as the viewer may see it: a mask with Reveal and
// Copy for whoever may reveal it, "hidden" for a reader of the page who may not, and "not available here" for a reference
// outside the page of its secret or one that names nothing, which look alike on purpose. A page frozen for a link shows
// "not shared", without the ID.
func (h *handler) passwords(r *http.Request, rendered, page string) string {
	if !strings.Contains(rendered, "data-secret=") {
		return rendered
	}
	if _, frozen := r.Context().Value(releaseKey{}).([]string); frozen {
		return markdown.SecretSentinels.ReplaceAllLiteralString(rendered,
			`<span class="badge" data-variant="muted" data-secret-state="unshared">Password not shared</span>`)
	}
	var ids []string
	for _, m := range markdown.SecretSentinels.FindAllStringSubmatch(rendered, -1) {
		if !slices.Contains(ids, m[1]) {
			ids = append(ids, m[1])
		}
	}
	states := map[string]db.SecretState{}
	if h.options.DB != nil && len(ids) > 0 {
		user, _ := account.From(r.Context())
		var err error
		if states, err = h.options.DB.SecretStates(r.Context(), user.ID, ids); err != nil {
			h.log.Error("secrets not read", slog.String("error", err.Error()))
			states = map[string]db.SecretState{}
		}
	}
	return markdown.SecretSentinels.ReplaceAllStringFunc(rendered, func(sentinel string) string {
		secretID := markdown.SecretSentinels.FindStringSubmatch(sentinel)[1]
		s, ok := states[secretID]
		switch {
		case !ok || s.Page != page:
			return `<span class="badge" data-variant="muted" data-secret-state="none">Password not available here</span>`
		case !s.May:
			return `<span class="badge" data-variant="outline" data-secret-state="hidden" title="You may not see this password">Password hidden</span>`
		}
		quoted := html.EscapeString(secretID)
		return `<span class="inline-flex items-center gap-1" data-secret="` + quoted + `" data-secret-state="ok">` +
			`<span class="secret-key" data-secret-value aria-label="Password, hidden">••••••••</span>` +
			`<button type="button" class="btn" data-variant="outline" data-size="xs" data-pwact="show" data-secret="` + quoted + `">Reveal</button>` +
			`<button type="button" class="btn" data-variant="outline" data-size="xs" data-pwact="copy" data-secret="` + quoted + `">Copy</button></span>`
	})
}

// pageOf is the page of the block a trail of rendering ends with ("<page>#<block>").
func pageOf(trail []string) string {
	if len(trail) == 0 {
		return ""
	}
	page, _, _ := strings.Cut(trail[len(trail)-1], "#")
	return page
}

// --- The API

type apiSecret struct {
	ID          string          `json:"id"`
	Page        string          `json:"page"`
	Ref         string          `json:"ref"`
	Restricted  bool            `json:"restricted"`
	May         bool            `json:"may"`
	MayChange   bool            `json:"mayChange"`
	MayRestrict bool            `json:"mayRestrict"`
	Referenced  bool            `json:"referenced"`
	Version     int             `json:"version,omitempty"`
	Changed     string          `json:"changed,omitempty"`
	ChangedBy   string          `json:"changedBy,omitempty"`
	Readers     []apiReader     `json:"readers,omitempty"`
	Versions    []apiSecretVers `json:"versions,omitempty"`
}

type apiReader struct {
	Kind string `json:"kind"`
	Who  string `json:"who"`
	Name string `json:"name,omitempty"`
}

type apiSecretVers struct {
	Number int    `json:"number"`
	At     string `json:"at"`
	Who    string `json:"who"`
	Origin string `json:"origin,omitempty"`
}

// SecretRef is the reference to a secret as the text holds it.
func SecretRef(secretID string) string { return `::secret{id="` + secretID + `"}` }

func toAPISecret(s db.PageSecret) apiSecret {
	out := apiSecret{ID: s.ID, Page: s.Page, Ref: SecretRef(s.ID), Restricted: s.Restricted, May: s.May, MayChange: s.MayChange,
		MayRestrict: s.MayRestrict, Referenced: s.Referenced, Version: s.Version, ChangedBy: s.ChangedBy}
	if !s.Changed.IsZero() {
		out.Changed = s.Changed.UTC().Format(time.RFC3339)
	}
	for _, r := range s.Readers {
		out.Readers = append(out.Readers, apiReader{Kind: r.Kind, Who: r.Who, Name: r.Name})
	}
	for _, v := range s.Versions {
		out.Versions = append(out.Versions, apiSecretVers{Number: v.Number, At: v.At.UTC().Format(time.RFC3339), Who: v.Name, Origin: v.Origin})
	}
	return out
}

// vaultReady answers the request itself when there is no key.
func (h *handler) vaultReady(w http.ResponseWriter) bool {
	if !h.options.Vault.Ready() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": errNoVault})
		return false
	}
	return true
}

// sealer seals a value for the tenant of the request under the name the database gives.
func (h *handler) sealer(r *http.Request, value vault.Secret) db.Seal {
	t := h.tenant(r)
	return func(name string) ([]byte, error) { return h.options.Vault.Seal(t, name, value) }
}

// secretValue reads the value of a new secret or version: text, 1 to maxSecret bytes, kept as a vault.Secret so it
// never prints.
func secretValue(w http.ResponseWriter, value string) (vault.Secret, bool) {
	if value == "" || len(value) > maxSecret || strings.ContainsRune(value, 0) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a password has 1 to 16384 characters"})
		return nil, false
	}
	return vault.Secret(value), true
}

// secretFailed adds the errors of secrets to failed.
func (h *handler) secretFailed(w http.ResponseWriter, err error, what string) bool {
	switch {
	case errors.Is(err, db.ErrSecretHeld):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
		return true
	case errors.Is(err, db.ErrTooManyReveals):
		w.Header().Set("Retry-After", strconv.Itoa(int(db.RevealWindow.Seconds())))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Too many passwords revealed in a short time. Wait a moment."})
		return true
	case errors.Is(err, db.ErrReader):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return true
	case errors.Is(err, vault.ErrSealed):
		h.log.Error(what + " failed: a sealed value does not open")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "the password does not open with the key of the server"})
		return true
	}
	return h.failed(w, err, what)
}

// apiCreateSecret makes a secret of a page: POST /api/pages/{id}/secrets {"value"}. It answers its ID and the reference
// for the text; the value never comes back.
func (h *handler) apiCreateSecret(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Value string `json:"value"`
	}
	user, ok := h.change(w, r, maxSecret+1024, &in)
	if !ok || !h.vaultReady(w) {
		return
	}
	value, ok := secretValue(w, in.Value)
	if !ok {
		return
	}
	defer clear(value)
	secretID, err := h.options.DB.CreatePageSecret(r.Context(), r.PathValue("id"), user.ID, h.sealer(r, value))
	if h.secretFailed(w, err, "secret") {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": secretID, "ref": SecretRef(secretID)})
}

// apiPageSecrets lists the secrets of a page: GET /api/pages/{id}/secrets.
func (h *handler) apiPageSecrets(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	user, _ := account.From(r.Context())
	list, err := h.options.DB.PageSecrets(r.Context(), r.PathValue("id"), user.ID)
	if h.secretFailed(w, err, "secrets") {
		return
	}
	out := []apiSecret{}
	for _, s := range list {
		out = append(out, toAPISecret(s))
	}
	writeJSON(w, http.StatusOK, map[string]any{"secrets": out, "vault": h.options.Vault.Ready()})
}

// apiSecret returns one secret, with its readers and versions as far as the viewer may know them: GET /api/secrets/{id}.
func (h *handler) apiSecret(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	user, _ := account.From(r.Context())
	s, err := h.options.DB.PageSecretOf(r.Context(), r.PathValue("id"), user.ID)
	if h.secretFailed(w, err, "secret") {
		return
	}
	writeJSON(w, http.StatusOK, toAPISecret(s))
}

// apiRevealSecret answers the value of a secret: POST /api/secrets/{id}/reveal {"purpose": "show|copy", "version": n}.
// Only from this application, for a signed-in account that may reveal it, within the limit per minute, never kept by a
// cache, and only once the log holds it.
func (h *handler) apiRevealSecret(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Purpose string `json:"purpose"`
		Version int    `json:"version"`
	}
	user, ok := h.change(w, r, 1024, &in)
	if !ok {
		return
	}
	if user.ID == "" || in.Version < 0 || in.Purpose != "show" && in.Purpose != "copy" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	if !h.vaultReady(w) {
		return
	}
	if allowed, wait := h.reveals.Take(h.tenant(r) + "/" + user.ID); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Too many passwords revealed in a short time. Wait a moment."})
		return
	}
	sealed, name, err := h.options.DB.RevealPageSecret(r.Context(), r.PathValue("id"), user.ID, in.Version, in.Purpose == "copy")
	if h.secretFailed(w, err, "reveal") {
		return
	}
	value, err := h.options.Vault.Open(h.tenant(r), name, sealed)
	if h.secretFailed(w, err, "reveal") {
		return
	}
	defer clear(value)
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusOK, map[string]string{"value": string(value)})
}

// apiChangeSecret gives a secret a new value: PUT /api/secrets/{id} {"value"}.
func (h *handler) apiChangeSecret(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Value string `json:"value"`
	}
	user, ok := h.change(w, r, maxSecret+1024, &in)
	if !ok || !h.vaultReady(w) {
		return
	}
	value, ok := secretValue(w, in.Value)
	if !ok {
		return
	}
	defer clear(value)
	if h.secretFailed(w, h.options.DB.ChangePageSecret(r.Context(), r.PathValue("id"), user.ID, h.sealer(r, value)), "secret") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// apiRestoreSecret makes an older value the newest again: POST /api/secrets/{id}/versions/{n}/restore.
func (h *handler) apiRestoreSecret(w http.ResponseWriter, r *http.Request) {
	user, ok := h.change(w, r, 0, nil)
	if !ok || !h.vaultReady(w) {
		return
	}
	number, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || number < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	t := h.tenant(r)
	err = h.options.DB.RestorePageSecret(r.Context(), r.PathValue("id"), user.ID, number, func(oldName string, old []byte, newName string) ([]byte, error) {
		value, err := h.options.Vault.Open(t, oldName, old)
		if err != nil {
			return nil, err
		}
		defer clear(value)
		return h.options.Vault.Seal(t, newName, value)
	})
	if h.secretFailed(w, err, "secret") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// apiDeleteSecret deletes a secret with every version: DELETE /api/secrets/{id}.
func (h *handler) apiDeleteSecret(w http.ResponseWriter, r *http.Request) {
	user, ok := h.change(w, r, 0, nil)
	if !ok {
		return
	}
	if h.secretFailed(w, h.options.DB.DeletePageSecret(r.Context(), r.PathValue("id"), user.ID), "secret") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// apiSecretReaders restricts a secret to persons and groups, or lets whoever reads the page reveal it again:
// PUT /api/secrets/{id}/readers {"restricted": true, "readers": [{"kind": "person|group", "who": "<id>"}]}.
func (h *handler) apiSecretReaders(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Restricted bool        `json:"restricted"`
		Readers    []apiReader `json:"readers"`
	}
	user, ok := h.change(w, r, 64<<10, &in)
	if !ok {
		return
	}
	readers := make([]db.SecretReader, 0, len(in.Readers))
	for _, x := range in.Readers {
		if x.Kind != "person" && x.Kind != "group" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": db.ErrReader.Error()})
			return
		}
		readers = append(readers, db.SecretReader{Kind: x.Kind, Who: x.Who})
	}
	if h.secretFailed(w, h.options.DB.SetPageSecretReaders(r.Context(), r.PathValue("id"), user.ID, in.Restricted, readers), "readers") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
