// SPDX-License-Identifier: MIT

package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/Ollornog/UnderDoc/internal/auth"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/id"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// Passkeys (WebAuthn), the default way in of a local account (ADR-23). The relying party is the host name of the tenant,
// so a passkey of one tenant is never offered on another. Every challenge is used once: it lives in a ceremony that the
// second request takes away. The library checks origin, challenge, signature and the flags; a counter that does not go
// up marks a copied authenticator, and such a sign-in is refused.

const passkeyBodyLimit = 64 << 10

// webAuthn returns the relying party for the host of the request.
func (h *handler) webAuthn(r *http.Request) (*webauthn.WebAuthn, error) {
	return webauthn.New(&webauthn.Config{
		RPID:          tenant.Host(r.Host),
		RPDisplayName: h.tenantName(r.Context()),
		RPOrigins:     []string{requestOrigin(r)},
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationRequired,
		},
	})
}

// passkeyUser is an account as the WebAuthn library sees it. Its handle is the account ID, which is random (ADR-18).
type passkeyUser struct {
	id, login, name string
	credentials     []webauthn.Credential
}

func (u passkeyUser) WebAuthnID() []byte                         { return []byte(u.id) }
func (u passkeyUser) WebAuthnName() string                       { return u.login }
func (u passkeyUser) WebAuthnDisplayName() string                { return u.name }
func (u passkeyUser) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

// loadPasskeyUser reads an account with all its passkeys.
func (h *handler) loadPasskeyUser(r *http.Request, accountID string) (passkeyUser, error) {
	a, err := h.options.DB.AccountByID(r.Context(), accountID)
	if err != nil {
		return passkeyUser{}, err
	}
	if a.Source != db.SourceLocal {
		return passkeyUser{}, db.ErrSource
	}
	keys, err := h.options.DB.Passkeys(r.Context(), accountID)
	if err != nil {
		return passkeyUser{}, err
	}
	u := passkeyUser{id: a.ID, login: a.Login, name: a.Name}
	for _, k := range keys {
		var c webauthn.Credential
		if err := json.Unmarshal(k.Credential, &c); err != nil {
			return passkeyUser{}, err
		}
		u.credentials = append(u.credentials, c)
	}
	return u, nil
}

// passkeyOK checks what every passkey request needs: the database, passkeys switched on, and a request of this site.
func (h *handler) passkeyOK(w http.ResponseWriter, r *http.Request) bool {
	if h.options.DB == nil || !h.signInSettings(r.Context()).Has(db.MethodPasskey) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return false
	}
	if !sameOrigin(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only from this application"})
		return false
	}
	return true
}

func (h *handler) passkeyFailed(w http.ResponseWriter, err error, what string) {
	h.log.Error(what+" failed", slog.String("error", err.Error()))
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
}

// passkeyOptions starts a sign-in with a passkey. No login is asked: the browser offers the passkeys it has for this
// host, and the passkey names its account.
func (h *handler) passkeyOptions(w http.ResponseWriter, r *http.Request) {
	if !h.passkeyOK(w, r) {
		return
	}
	wa, err := h.webAuthn(r)
	if err != nil {
		h.passkeyFailed(w, err, "passkey")
		return
	}
	assertion, session, err := wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		h.passkeyFailed(w, err, "passkey")
		return
	}
	if err := h.putCeremony(w, r, "passkey_sign_in", "", session); err != nil {
		h.passkeyFailed(w, err, "passkey")
		return
	}
	writeJSON(w, http.StatusOK, assertion)
}

// passkeySignIn finishes a sign-in with a passkey.
func (h *handler) passkeySignIn(w http.ResponseWriter, r *http.Request) {
	if !h.passkeyOK(w, r) {
		return
	}
	ctx := r.Context()
	refuse := func(login, reason string) {
		h.logSignIn(ctx, login, db.MethodPasskey, false, reason)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "This passkey does not sign in here."})
	}
	var session webauthn.SessionData
	if _, err := h.takeCeremony(w, r, "passkey_sign_in", &session); err != nil {
		refuse("", "no ceremony")
		return
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(http.MaxBytesReader(w, r.Body, passkeyBodyLimit))
	if err != nil {
		refuse("", "unreadable answer")
		return
	}
	wa, err := h.webAuthn(r)
	if err != nil {
		h.passkeyFailed(w, err, "passkey")
		return
	}
	var user passkeyUser
	credential, err := wa.ValidateDiscoverableLogin(func(rawID, handle []byte) (webauthn.User, error) {
		key, err := h.options.DB.PasskeyByID(ctx, rawID)
		if err != nil {
			return nil, err
		}
		if key.Account != string(handle) {
			return nil, errors.New("the passkey belongs to another account")
		}
		user, err = h.loadPasskeyUser(r, key.Account)
		return user, err
	}, session, parsed)
	if err != nil {
		refuse(user.login, "passkey not valid")
		return
	}
	if credential.Authenticator.CloneWarning {
		refuse(user.login, "counter did not go up: copied passkey")
		return
	}
	raw, err := json.Marshal(credential)
	if err == nil {
		err = h.options.DB.UsedPasskey(ctx, credential.ID, raw)
	}
	if err == nil {
		err = h.startSession(w, r, user.id, db.MethodPasskey)
	}
	if err != nil {
		h.passkeyFailed(w, err, "passkey")
		return
	}
	h.logSignIn(ctx, user.login, db.MethodPasskey, true, "")
	writeJSON(w, http.StatusOK, map[string]string{"next": safeNext(r.URL.Query().Get("next"))})
}

// newPasskey is the ceremony of a passkey being made: for an invitation, the hash of its token and the ID the account
// will get; for an account that adds one, its ID.
type newPasskey struct {
	Session webauthn.SessionData `json:"session"`
	Invite  []byte               `json:"invite,omitempty"`
	Account string               `json:"account"`
}

func registrationOptions(user passkeyUser) []webauthn.RegistrationOption {
	exclude := make([]protocol.CredentialDescriptor, 0, len(user.credentials))
	for _, c := range user.credentials {
		exclude = append(exclude, c.Descriptor())
	}
	return []webauthn.RegistrationOption{
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(exclude),
		webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
	}
}

// invitePasskeyOptions starts the first passkey of an invited account.
func (h *handler) invitePasskeyOptions(w http.ResponseWriter, r *http.Request) {
	if !h.passkeyOK(w, r) {
		return
	}
	hash := auth.HashToken(r.PathValue("token"))
	inv, err := h.options.DB.OpenInvite(r.Context(), hash)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "This invitation is used or has run out."})
		return
	}
	h.beginPasskey(w, r, passkeyUser{id: id.New(), login: inv.Login, name: inv.Name}, hash)
}

// beginPasskey sends the options to make a passkey for the user, and keeps the ceremony.
func (h *handler) beginPasskey(w http.ResponseWriter, r *http.Request, user passkeyUser, invite []byte) {
	wa, err := h.webAuthn(r)
	if err != nil {
		h.passkeyFailed(w, err, "passkey")
		return
	}
	creation, session, err := wa.BeginRegistration(user, registrationOptions(user)...)
	if err != nil {
		h.passkeyFailed(w, err, "passkey")
		return
	}
	accountID := ""
	if invite == nil {
		accountID = user.id
	}
	if err := h.putCeremony(w, r, "passkey_add", accountID, newPasskey{Session: *session, Invite: invite, Account: user.id}); err != nil {
		h.passkeyFailed(w, err, "passkey")
		return
	}
	writeJSON(w, http.StatusOK, creation)
}

// finishPasskey checks the new passkey of a ceremony and returns it with the ceremony.
func (h *handler) finishPasskey(w http.ResponseWriter, r *http.Request, user func(newPasskey) (passkeyUser, error)) (*webauthn.Credential, newPasskey, bool) {
	var c newPasskey
	if _, err := h.takeCeremony(w, r, "passkey_add", &c); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Start again: this step ran out."})
		return nil, c, false
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(http.MaxBytesReader(w, r.Body, passkeyBodyLimit))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "The passkey could not be read."})
		return nil, c, false
	}
	u, err := user(c)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Start again: this step does not belong here."})
		return nil, c, false
	}
	wa, err := h.webAuthn(r)
	if err != nil {
		h.passkeyFailed(w, err, "passkey")
		return nil, c, false
	}
	credential, err := wa.CreateCredential(u, c.Session, parsed)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "The passkey was not accepted."})
		return nil, c, false
	}
	return credential, c, true
}

// invitePasskey finishes the first passkey of an invited account, makes the account and signs it in.
func (h *handler) invitePasskey(w http.ResponseWriter, r *http.Request) {
	if !h.passkeyOK(w, r) {
		return
	}
	ctx := r.Context()
	hash := auth.HashToken(r.PathValue("token"))
	var inv db.Invite
	credential, c, ok := h.finishPasskey(w, r, func(c newPasskey) (passkeyUser, error) {
		if string(c.Invite) != string(hash) {
			return passkeyUser{}, errors.New("another invitation")
		}
		var err error
		inv, err = h.options.DB.OpenInvite(ctx, hash)
		return passkeyUser{id: c.Account, login: inv.Login, name: inv.Name}, err
	})
	if !ok {
		return
	}
	raw, err := json.Marshal(credential)
	if err != nil {
		h.passkeyFailed(w, err, "passkey")
		return
	}
	a, err := h.options.DB.AcceptInvite(ctx, hash, db.NewLocal{ID: c.Account, PasskeyID: credential.ID, Passkey: raw,
		PasskeyName: passkeyName(r)})
	if h.inviteFailed(w, err) {
		return
	}
	if err := h.startSession(w, r, a.ID, db.MethodPasskey); err != nil {
		h.passkeyFailed(w, err, "passkey")
		return
	}
	h.logSignIn(ctx, a.Login, db.MethodPasskey, true, "invitation accepted")
	writeJSON(w, http.StatusOK, map[string]string{"next": "/"})
}

// inviteFailed answers what went wrong when accepting an invitation. It reports whether something did.
func (h *handler) inviteFailed(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, db.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "This invitation is used or has run out."})
	case errors.Is(err, db.ErrLoginTaken):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "This login is taken by now. Ask for a new invitation."})
	default:
		h.passkeyFailed(w, err, "invitation")
	}
	return true
}

// passkeyName is the name a person gave the passkey, or one from the browser.
func passkeyName(r *http.Request) string {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		name = "Passkey"
	}
	return name
}

// addPasskeyOptions starts another passkey of the signed-in local account.
func (h *handler) addPasskeyOptions(w http.ResponseWriter, r *http.Request) {
	if !h.passkeyOK(w, r) {
		return
	}
	user, err := h.loadPasskeyUser(r, h.me(r).ID)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Only a local account keeps passkeys here."})
		return
	}
	h.beginPasskey(w, r, user, nil)
}

// addPasskey finishes another passkey of the signed-in local account.
func (h *handler) addPasskey(w http.ResponseWriter, r *http.Request) {
	if !h.passkeyOK(w, r) {
		return
	}
	me := h.me(r).ID
	credential, _, ok := h.finishPasskey(w, r, func(c newPasskey) (passkeyUser, error) {
		if c.Account != me || c.Invite != nil {
			return passkeyUser{}, errors.New("another account")
		}
		return h.loadPasskeyUser(r, me)
	})
	if !ok {
		return
	}
	raw, err := json.Marshal(credential)
	if err == nil {
		err = h.options.DB.AddPasskey(r.Context(), me, passkeyName(r), credential.ID, raw)
	}
	if errors.Is(err, db.ErrPasskeyTaken) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		h.passkeyFailed(w, err, "passkey")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"next": "/account/security"})
}
