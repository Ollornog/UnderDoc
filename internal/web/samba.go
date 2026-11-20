// SPDX-License-Identifier: MIT

package web

import (
	"crypto/rand"
	"errors"
	"net/http"
	"strings"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/samba"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
)

// Samba in the web (ADR-30, docs/samba.md): the app password for SMB in the profile, the share of a collection, and
// the Samba users for administrators. The working copy itself is package share.

// sambaOn answers 404 where the tenant of the request has no companion.
func (h *handler) sambaOn(w http.ResponseWriter, r *http.Request) bool {
	if h.options.DB == nil || h.options.Share == nil || !h.options.Share.On(r.Context()) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Samba is not set up here"})
		return false
	}
	return true
}

type apiSamba struct {
	On     bool     `json:"on"`
	Server string   `json:"server,omitempty"`
	Name   string   `json:"name,omitempty"`
	Has    bool     `json:"has"`
	Shares []string `json:"shares"`
	// Suggest is the name a new Samba user gets: the login where it fits the rules.
	Suggest string `json:"suggest,omitempty"`
}

// apiSamba tells the signed-in account whether Samba is there, its Samba user and the shares it reaches.
func (h *handler) apiSamba(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil || h.options.Share == nil || !h.options.Share.On(r.Context()) {
		writeJSON(w, http.StatusOK, apiSamba{Shares: []string{}})
		return
	}
	ctx := r.Context()
	user, _ := account.From(ctx)
	out := apiSamba{On: true, Server: h.options.Share.Server(), Shares: []string{}}
	a, err := h.options.DB.SambaAccountOf(ctx, user.ID)
	switch {
	case err == nil:
		out.Has, out.Name = true, a.Name
		if s := h.options.Share.Reaches(ctx, user.ID); s != nil {
			out.Shares = s
		}
	case errors.Is(err, db.ErrNotFound):
		if name := strings.ToLower(user.Login); samba.ValidName(name) {
			out.Suggest = name
		}
	default:
		h.failed(w, err, "samba")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// appPasswordAlphabet has no letters that look alike (no 0/o, 1/l/i), so a password can be typed from the screen.
const appPasswordAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// newAppPassword makes an app password of 20 characters in groups of five, about 99 bits.
func newAppPassword() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	var b strings.Builder
	for i, c := range raw {
		if i > 0 && i%5 == 0 {
			b.WriteByte('-')
		}
		// 256 is not a multiple of 31: values above the last full round are drawn again.
		for c >= 248 {
			var one [1]byte
			if _, err := rand.Read(one[:]); err != nil {
				return "", err
			}
			c = one[0]
		}
		b.WriteByte(appPasswordAlphabet[int(c)%len(appPasswordAlphabet)])
	}
	return b.String(), nil
}

// apiNewAppPassword makes a new app password for the signed-in account and shows it once; its NT hash is sealed, the
// password itself is kept nowhere. The first one makes the Samba user, with the name given or the login.
func (h *handler) apiNewAppPassword(w http.ResponseWriter, r *http.Request) {
	if !h.sambaOn(w, r) {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	user, ok := h.change(w, r, 1<<10, &body)
	if !ok {
		return
	}
	ctx := r.Context()
	if !h.options.Vault.Ready() {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "app passwords need " + vault.KeyEnv})
		return
	}
	name := strings.ToLower(strings.TrimSpace(body.Name))
	if name == "" {
		name = strings.ToLower(user.Login)
	}
	existing, err := h.options.DB.SambaAccountOf(ctx, user.ID)
	isNew := errors.Is(err, db.ErrNotFound)
	if err == nil {
		name = existing.Name
	} else if !isNew {
		h.failed(w, err, "app password")
		return
	}
	if !samba.ValidName(name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": db.ErrSambaName.Error()})
		return
	}
	// A new Samba user does not take the name of a share: its [homes] would push the share aside, and every drive
	// mapped to it would break (ADR-30, after an attack round).
	if isNew {
		taken, err := h.options.Share.NameTaken(ctx, name)
		if h.failed(w, err, "app password") {
			return
		}
		if taken {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": db.ErrSambaName.Error() + "; a share has this name"})
			return
		}
	}
	password, err := newAppPassword()
	if h.failed(w, err, "app password") {
		return
	}
	t, _ := tenant.From(ctx)
	sealed, err := h.options.Vault.Seal(t, db.SambaSecretName(user.ID), vault.Secret(samba.NTHash(password)))
	if h.failed(w, err, "app password") {
		return
	}
	lo, hi := h.options.Share.UIDs()
	a, err := h.options.DB.SetSambaPassword(ctx, user.ID, name, sealed, lo, hi)
	if h.failed(w, err, "app password") {
		return
	}
	h.options.Share.FilesChanged(ctx)
	// writeJSON answers with Cache-Control: no-store, so the password stays out of every cache.
	writeJSON(w, http.StatusOK, map[string]string{"name": a.Name, "password": password, "server": h.options.Share.Server()})
}

// apiRemoveAppPassword removes the Samba user of the signed-in account, or of another for an administrator
// (DELETE /api/samba/accounts/{id}).
func (h *handler) apiRemoveAppPassword(w http.ResponseWriter, r *http.Request) {
	if !h.sambaOn(w, r) {
		return
	}
	user, ok := h.change(w, r, 0, nil)
	if !ok {
		return
	}
	target := r.PathValue("id")
	if target == "" {
		target = user.ID
	}
	if h.failed(w, h.options.DB.RemoveSambaAccount(r.Context(), h.actor(r), target), "app password") {
		return
	}
	h.options.Share.FilesChanged(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// apiSambaAccounts lists the Samba users of the tenant, for administrators.
func (h *handler) apiSambaAccounts(w http.ResponseWriter, r *http.Request) {
	if !h.sambaOn(w, r) {
		return
	}
	if !h.isAdmin(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only administrators see every Samba user"})
		return
	}
	list, err := h.options.DB.SambaAccounts(r.Context())
	if h.failed(w, err, "samba") {
		return
	}
	type entry struct {
		Account string `json:"account"`
		Login   string `json:"login"`
		Name    string `json:"name"`
	}
	out := make([]entry, 0, len(list))
	for _, a := range list {
		out = append(out, entry{Account: a.Account, Login: a.Login, Name: a.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": out})
}

// apiSetRootSamba switches the share of a collection of the view Files on or off, for its managers.
func (h *handler) apiSetRootSamba(w http.ResponseWriter, r *http.Request) {
	if !h.sambaOn(w, r) {
		return
	}
	var body struct {
		On bool `json:"on"`
	}
	user, ok := h.change(w, r, 1<<10, &body)
	if !ok {
		return
	}
	if h.failed(w, h.options.DB.SetRootSamba(r.Context(), r.PathValue("id"), user.ID, body.On), "samba share") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sambaRights is the part of the rights dialog about the share.
func (h *handler) sambaRights(r *http.Request, objectID, viewer string) *apiSambaRights {
	ctx := r.Context()
	out := &apiSambaRights{}
	if collection, on, err := h.options.DB.RootSamba(ctx, objectID); err == nil && collection {
		out.Root = &on
	}
	root, err := h.options.DB.SharedRootOf(ctx, objectID)
	if err != nil || root == "" {
		return out
	}
	out.Shared = true
	if o, err := h.options.DB.Object(ctx, objectID, viewer); err == nil {
		out.File = o.Kind == db.KindFile
	}
	if place, ok := h.options.Share.PlaceOf(ctx, objectID, viewer); ok {
		out.Path = place.Path
	}
	return out
}
