// SPDX-License-Identifier: MIT

package web

import (
	"encoding/base64"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/auth"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/id"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
)

// Local accounts (ADR-23): an administrator invites, or, where the tenant allows it, people register themselves. Either
// way an invitation link leads to the page where the new account gets its first passkey or its password, and the
// security page of the account adds and removes them later.

// me returns who is signed in.
func (h *handler) me(r *http.Request) account.User {
	user, _ := account.From(r.Context())
	return user
}

// welcomeView fills the page of an invitation.
type welcomeView struct {
	signInView
	Token     string
	Name      string
	TOTPSetup *totpView
	// PasswordOnly tells that a password alone is allowed, so TOTP is a choice and not a must.
	PasswordOnly bool
}

// totpReady tells whether the tenant uses TOTP and the instance can seal its secrets.
func (h *handler) totpReady(s db.SignIn) bool {
	return s.Has(db.MethodPasswordTOTP) && h.options.Vault.Ready()
}

func (h *handler) welcomeView(r *http.Request, token string, inv db.Invite, problem string) (welcomeView, error) {
	ctx := r.Context()
	s := h.signInSettings(ctx)
	v := welcomeView{signInView: signInView{Version: h.assetVersion, Title: "Welcome", Tenant: h.tenantName(ctx), Error: problem,
		Passkey: s.Has(db.MethodPasskey), Password: s.Has(db.MethodPassword) || h.totpReady(s), TOTP: h.totpReady(s), Login: inv.Login},
		Token: token, Name: inv.Name, PasswordOnly: s.Has(db.MethodPassword)}
	if v.TOTP {
		setup, err := newTOTPView(v.Tenant, inv.Login)
		if err != nil {
			return v, err
		}
		v.TOTPSetup = setup
	}
	return v, nil
}

// invite shows the page of an invitation: who it is for, and the ways to set up the account.
func (h *handler) invite(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		h.notFound(w, r)
		return
	}
	token := r.PathValue("token")
	inv, err := h.options.DB.OpenInvite(r.Context(), auth.HashToken(token))
	if err != nil {
		h.message(w, http.StatusNotFound, message{Title: "Invitation not valid", Text: "This invitation is used or has run out. Ask for a new one.", Link: "To the start"})
		return
	}
	v, err := h.welcomeView(r, token, inv, "")
	if err != nil {
		h.passkeyFailed(w, err, "invitation")
		return
	}
	h.page(w, http.StatusOK, "welcome", v)
}

// invitePassword sets up an invited account with a password, and with TOTP where the tenant asks for it.
func (h *handler) invitePassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s := h.signInSettings(ctx)
	if h.options.DB == nil || !s.Has(db.MethodPassword) && !h.totpReady(s) {
		h.notFound(w, r)
		return
	}
	if !h.formOK(w, r) {
		return
	}
	token := r.PathValue("token")
	hash := auth.HashToken(token)
	inv, err := h.options.DB.OpenInvite(ctx, hash)
	if err != nil {
		h.message(w, http.StatusNotFound, message{Title: "Invitation not valid", Text: "This invitation is used or has run out. Ask for a new one.", Link: "To the start"})
		return
	}
	again := func(problem string) {
		v, err := h.welcomeView(r, token, inv, problem)
		if err != nil {
			h.passkeyFailed(w, err, "invitation")
			return
		}
		h.page(w, http.StatusBadRequest, "welcome", v)
	}
	password := r.PostFormValue("password")
	if err := auth.CheckPassword(password); err != nil {
		again(err.Error() + ".")
		return
	}
	if password != r.PostFormValue("repeat") {
		again("The two passwords differ.")
		return
	}
	in := db.NewLocal{ID: id.New()}
	if in.PasswordHash, err = auth.HashPassword(password); err != nil {
		h.passkeyFailed(w, err, "invitation")
		return
	}
	secret, code := r.PostFormValue("totp_secret"), r.PostFormValue("code")
	if h.totpReady(s) && (!s.Has(db.MethodPassword) || strings.TrimSpace(code) != "") {
		step, ok := auth.CheckTOTP(secret, code, time.Now(), 0)
		if !ok {
			again("The code from the app is wrong. Scan the new picture and try again.")
			return
		}
		t, _ := tenant.From(ctx)
		if in.TOTPSealed, err = h.options.Vault.Seal(t, totpName(in.ID), vault.Secret(secret)); err != nil {
			h.passkeyFailed(w, err, "invitation")
			return
		}
		in.TOTPStep = step
	}
	a, err := h.options.DB.AcceptInvite(ctx, hash, in)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrLoginTaken):
			again("This login is taken by now. Ask for a new invitation.")
		case errors.Is(err, db.ErrNotFound):
			again("This invitation is used or has run out.")
		default:
			h.passkeyFailed(w, err, "invitation")
		}
		return
	}
	method := db.MethodPassword
	if in.TOTPSealed != nil && s.Has(db.MethodPasswordTOTP) {
		method = db.MethodPasswordTOTP
	}
	if err := h.startSession(w, r, a.ID, method); err != nil {
		h.passkeyFailed(w, err, "invitation")
		return
	}
	h.logSignIn(ctx, a.Login, method, true, "invitation accepted")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// registrationsPerHour bounds how many people can register in a tenant within an hour.
const registrationsPerHour = 20

// register lets someone make an account where the tenant allows it: it makes an invitation for itself and goes on to it.
func (h *handler) register(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s := h.signInSettings(ctx)
	if h.options.DB == nil || !s.Registration || !s.Has(db.MethodPasskey) && !s.Has(db.MethodPassword) && !h.totpReady(s) {
		h.notFound(w, r)
		return
	}
	v := signInView{Version: h.assetVersion, Title: "Register", Tenant: h.tenantName(ctx)}
	if r.Method == http.MethodGet {
		h.page(w, http.StatusOK, "register", v)
		return
	}
	if !h.formOK(w, r) {
		return
	}
	v.Login = strings.ToLower(strings.TrimSpace(r.PostFormValue("login")))
	if n, err := h.options.DB.RecentRegistrations(ctx, time.Hour); err != nil || n >= registrationsPerHour {
		v.Error = "Too many registrations just now. Try again later."
		h.page(w, http.StatusTooManyRequests, "register", v)
		return
	}
	token, hash, err := auth.NewToken()
	if err == nil {
		err = h.options.DB.CreateInvite(ctx, hash, db.Invite{Login: v.Login, Name: r.PostFormValue("name"), Expires: time.Now().Add(time.Hour)})
	}
	switch {
	case errors.Is(err, db.ErrLogin), errors.Is(err, db.ErrTitle), errors.Is(err, db.ErrLoginTaken):
		v.Error = registerProblem(err)
		h.page(w, http.StatusBadRequest, "register", v)
		return
	case err != nil:
		h.passkeyFailed(w, err, "registration")
		return
	}
	http.Redirect(w, r, "/invite/"+token, http.StatusSeeOther)
}

func registerProblem(err error) string {
	switch {
	case errors.Is(err, db.ErrLoginTaken):
		return "This login is taken."
	case errors.Is(err, db.ErrTitle):
		return "A name has 1 to 200 characters."
	}
	return "A login has up to 63 lower case letters, digits, dots, hyphens and underscores."
}

// --- The security page of an account

type securityView struct {
	Version   string
	Title     string
	Tenant    string
	Error     string
	Notice    string
	User      account.User
	Local     bool
	Source    string
	Passkeys  []passkeyRow
	Passkey   bool
	Password  bool
	HasPass   bool
	TOTP      bool
	HasTOTP   bool
	TOTPSetup *totpView
}

// totpView is the setup of an authenticator app on a page. The picture is a data: URL made here, so it is safe.
type totpView struct {
	Secret string
	QR     template.URL
}

func newTOTPView(issuer, login string) (*totpView, error) {
	setup, err := auth.GenerateTOTP(issuer, login)
	if err != nil {
		return nil, err
	}
	return &totpView{Secret: setup.Secret, QR: template.URL(setup.QR)}, nil // a PNG that auth.GenerateTOTP made, never input
}

type passkeyRow struct {
	ID       string
	Name     string
	Created  string
	LastUsed string
}

func (h *handler) securityPage(w http.ResponseWriter, r *http.Request, status int, problem, notice string, setup *totpView) {
	ctx := r.Context()
	me := h.me(r)
	s := h.signInSettings(ctx)
	v := securityView{Version: h.assetVersion, Title: "Sign-in and security", Tenant: h.tenantName(ctx), Error: problem, Notice: notice, User: me,
		Passkey: s.Has(db.MethodPasskey), Password: s.Has(db.MethodPassword) || h.totpReady(s), TOTP: h.totpReady(s), TOTPSetup: setup}
	if h.options.DB != nil {
		if a, err := h.options.DB.AccountByID(ctx, me.ID); err == nil {
			v.Source, v.Local, v.HasPass, v.HasTOTP = a.Source, a.Source == db.SourceLocal, a.PasswordHash != "", a.TOTPSealed != nil
		}
		keys, err := h.options.DB.Passkeys(ctx, me.ID)
		if err != nil {
			h.log.Error("passkeys failed", slog.String("error", err.Error()))
		}
		for _, k := range keys {
			row := passkeyRow{ID: base64.RawURLEncoding.EncodeToString(k.ID), Name: k.Name, Created: k.Created.UTC().Format("2006-01-02"), LastUsed: "never"}
			if k.LastUsed != nil {
				row.LastUsed = k.LastUsed.UTC().Format("2006-01-02 15:04")
			}
			v.Passkeys = append(v.Passkeys, row)
		}
	}
	h.page(w, status, "security", v)
}

// security shows the sign-in of the account: passkeys, password, TOTP, and signing out everywhere.
func (h *handler) security(w http.ResponseWriter, r *http.Request) {
	var setup *totpView
	if r.URL.Query().Get("totp") == "new" && h.totpReady(h.signInSettings(r.Context())) {
		var err error
		if setup, err = newTOTPView(h.tenantName(r.Context()), h.me(r).Login); err != nil {
			h.passkeyFailed(w, err, "TOTP")
			return
		}
	}
	h.securityPage(w, r, http.StatusOK, "", "", setup)
}

// localAccount returns the signed-in local account, or answers that there is none.
func (h *handler) localAccount(w http.ResponseWriter, r *http.Request) (db.Account, bool) {
	if h.options.DB == nil {
		h.notFound(w, r)
		return db.Account{}, false
	}
	if !h.formOK(w, r) {
		return db.Account{}, false
	}
	a, err := h.options.DB.AccountByID(r.Context(), h.me(r).ID)
	if err != nil || a.Source != db.SourceLocal {
		h.securityPage(w, r, http.StatusForbidden, "Your sign-in is managed elsewhere.", "", nil)
		return db.Account{}, false
	}
	return a, true
}

// changePassword sets a new password. The current one is asked for, where there is one.
func (h *handler) changePassword(w http.ResponseWriter, r *http.Request) {
	a, ok := h.localAccount(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	s := h.signInSettings(ctx)
	if !s.Has(db.MethodPassword) && !h.totpReady(s) {
		h.notFound(w, r)
		return
	}
	key := h.throttleKey(ctx, a.Login)
	if wait := h.throttle.Wait(key); wait > 0 {
		h.securityPage(w, r, http.StatusTooManyRequests, waitText(wait), "", nil)
		return
	}
	if a.PasswordHash != "" && !auth.VerifyPassword(a.PasswordHash, r.PostFormValue("current")) {
		h.throttle.Failed(key)
		h.logSignIn(ctx, a.Login, db.MethodPassword, false, "wrong password on change")
		h.securityPage(w, r, http.StatusBadRequest, "The current password is wrong.", "", nil)
		return
	}
	password := r.PostFormValue("password")
	if err := auth.CheckPassword(password); err != nil {
		h.securityPage(w, r, http.StatusBadRequest, err.Error()+".", "", nil)
		return
	}
	if password != r.PostFormValue("repeat") {
		h.securityPage(w, r, http.StatusBadRequest, "The two passwords differ.", "", nil)
		return
	}
	hash, err := auth.HashPassword(password)
	if err == nil {
		err = h.options.DB.SetPassword(ctx, a.ID, hash)
	}
	if err != nil {
		h.passkeyFailed(w, err, "password")
		return
	}
	h.securityPage(w, r, http.StatusOK, "", "The new password holds from now on.", nil)
}

// setTOTP keeps a new TOTP secret once a code from the app proves it was scanned.
func (h *handler) setTOTP(w http.ResponseWriter, r *http.Request) {
	a, ok := h.localAccount(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if !h.totpReady(h.signInSettings(ctx)) {
		h.notFound(w, r)
		return
	}
	secret := r.PostFormValue("totp_secret")
	step, good := auth.CheckTOTP(secret, r.PostFormValue("code"), time.Now(), 0)
	if !good {
		setup, _ := newTOTPView(h.tenantName(ctx), a.Login)
		h.securityPage(w, r, http.StatusBadRequest, "The code from the app is wrong. Scan the new picture and try again.", "", setup)
		return
	}
	t, _ := tenant.From(ctx)
	sealed, err := h.options.Vault.Seal(t, totpName(a.ID), vault.Secret(secret))
	if err == nil {
		err = h.options.DB.SetTOTP(ctx, a.ID, sealed)
	}
	if err == nil {
		_, err = h.options.DB.UseTOTPStep(ctx, a.ID, step)
	}
	if err != nil {
		h.passkeyFailed(w, err, "TOTP")
		return
	}
	h.securityPage(w, r, http.StatusOK, "", "The app gives the codes from now on.", nil)
}

// removeTOTP takes TOTP away, where a password alone is allowed.
func (h *handler) removeTOTP(w http.ResponseWriter, r *http.Request) {
	a, ok := h.localAccount(w, r)
	if !ok {
		return
	}
	if !h.signInSettings(r.Context()).Has(db.MethodPassword) {
		h.securityPage(w, r, http.StatusBadRequest, "This workspace asks for a code with every password.", "", nil)
		return
	}
	if err := h.options.DB.SetTOTP(r.Context(), a.ID, nil); err != nil {
		h.passkeyFailed(w, err, "TOTP")
		return
	}
	h.securityPage(w, r, http.StatusOK, "", "The password alone signs in from now on.", nil)
}

// removePasskey takes a passkey away, unless it is the last way in.
func (h *handler) removePasskey(w http.ResponseWriter, r *http.Request) {
	a, ok := h.localAccount(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	credentialID, err := base64.RawURLEncoding.DecodeString(r.PathValue("id"))
	if err != nil {
		h.notFound(w, r)
		return
	}
	keys, err := h.options.DB.Passkeys(ctx, a.ID)
	if err != nil {
		h.passkeyFailed(w, err, "passkey")
		return
	}
	s := h.signInSettings(ctx)
	passwordWorks := a.PasswordHash != "" && (s.Has(db.MethodPassword) || a.TOTPSealed != nil && s.Has(db.MethodPasswordTOTP))
	if len(keys) <= 1 && !passwordWorks {
		h.securityPage(w, r, http.StatusBadRequest, "This is your last way in. Add another passkey first.", "", nil)
		return
	}
	if err := h.options.DB.RemovePasskey(ctx, a.ID, credentialID); errors.Is(err, db.ErrNotFound) {
		h.notFound(w, r)
		return
	} else if err != nil {
		h.passkeyFailed(w, err, "passkey")
		return
	}
	h.securityPage(w, r, http.StatusOK, "", "The passkey is gone.", nil)
}

// endOtherSessions signs the account out everywhere but here.
func (h *handler) endOtherSessions(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil || !h.formOK(w, r) {
		return
	}
	var keep []byte
	if c, err := r.Cookie(sessionCookie); err == nil {
		keep = auth.HashToken(c.Value)
	}
	if err := h.options.DB.EndSessions(r.Context(), h.me(r).ID, keep); err != nil {
		h.passkeyFailed(w, err, "sessions")
		return
	}
	h.securityPage(w, r, http.StatusOK, "", "Signed out everywhere else.", nil)
}

// --- The sign-in of the tenant, for its administrators

type adminSignInView struct {
	Version     string
	Title       string
	Tenant      string
	Error       string
	Notice      string
	Link        string
	Settings    db.SignIn
	Methods     []methodRow
	SecretSet   bool
	VaultReady  bool
	Log         []logRow
	DefaultTeam string
}

type methodRow struct {
	ID, Label, Hint string
	On, Disabled    bool
}

type logRow struct {
	At, Login, Method, Reason string
	OK                        bool
}

// oidcSecret is the name of the client secret of OIDC in the store of secrets.
const oidcSecret = "oidc.client_secret"

var methodLabels = map[string][2]string{
	db.MethodPasskey:      {"Passkey", "Local accounts sign in with a passkey of their device."},
	db.MethodPasswordTOTP: {"Password and code", "Local accounts sign in with a password and a code from an app (TOTP)."},
	db.MethodPassword:     {"Password alone", "Local accounts sign in with a password only. The weakest way."},
	db.MethodOIDC:         {"Single sign-on (OIDC)", "Accounts come from an identity provider, named by preferred_username."},
	db.MethodGate:         {"Gate in front", "A sign-in gate in front of UnderDoc passes the user on in headers."},
}

func (h *handler) adminOnly(w http.ResponseWriter, r *http.Request) bool {
	if h.options.DB == nil || !h.me(r).Admin {
		h.notFound(w, r)
		return false
	}
	return true
}

func (h *handler) adminSignInPage(w http.ResponseWriter, r *http.Request, status int, problem, notice, link string) {
	ctx := r.Context()
	s := h.signInSettings(ctx)
	v := adminSignInView{Version: h.assetVersion, Title: "Sign-in", Tenant: h.tenantName(ctx), Error: problem, Notice: notice, Link: link,
		Settings: s, SecretSet: h.options.Vault.Ready() && h.secrets().Has(ctx, oidcSecret), VaultReady: h.options.Vault.Ready(),
		DefaultTeam: h.options.AdminGroup}
	for _, m := range db.Methods {
		row := methodRow{ID: m, Label: methodLabels[m][0], Hint: methodLabels[m][1], On: s.Has(m)}
		switch {
		case m == db.MethodGate && h.options.Gate == GateOff:
			row.Disabled, row.Hint = true, row.Hint+" Switched off for this instance."
		case m == db.MethodGate && h.options.Gate != GateOn && !row.On:
			row.Disabled, row.Hint = true, row.Hint+" Only whoever runs the instance can allow it."
		case m == db.MethodPasswordTOTP && !h.options.Vault.Ready():
			row.Disabled, row.Hint = true, row.Hint+" Needs a key for secrets on the instance."
		}
		v.Methods = append(v.Methods, row)
	}
	events, err := h.options.DB.SignInLog(ctx, 100)
	if err != nil {
		h.log.Error("sign-in log failed", slog.String("error", err.Error()))
	}
	for _, e := range events {
		v.Log = append(v.Log, logRow{At: e.At.UTC().Format("2006-01-02 15:04:05"), Login: e.Login, Method: e.Method, OK: e.OK, Reason: e.Reason})
	}
	h.page(w, status, "admin-signin", v)
}

// adminSignIn shows how the tenant lets people in, its invitations and its log of sign-ins.
func (h *handler) adminSignIn(w http.ResponseWriter, r *http.Request) {
	if !h.adminOnly(w, r) {
		return
	}
	h.adminSignInPage(w, r, http.StatusOK, "", "", "")
}

// saveSignIn keeps the ways in of the tenant. The client secret of OIDC can be set and removed, never read.
func (h *handler) saveSignIn(w http.ResponseWriter, r *http.Request) {
	if !h.adminOnly(w, r) || !h.formOK(w, r) {
		return
	}
	ctx := r.Context()
	old := h.signInSettings(ctx)
	s := db.SignIn{Methods: r.PostForm["method"], Registration: r.PostFormValue("registration") == "on",
		OIDCName: r.PostFormValue("oidc_name"), OIDCIssuer: r.PostFormValue("oidc_issuer"), OIDCClientID: r.PostFormValue("oidc_client_id"),
		OIDCGroupsClaim: r.PostFormValue("oidc_groups_claim"), AdminGroup: r.PostFormValue("admin_group")}
	gateWanted := slices.Contains(s.Methods, db.MethodGate)
	if gateWanted && !old.Has(db.MethodGate) && h.options.Gate != GateOn || gateWanted && h.options.Gate == GateOff {
		h.adminSignInPage(w, r, http.StatusBadRequest, "The gate can only be switched on by whoever runs the instance.", "", "")
		return
	}
	if slices.Contains(s.Methods, db.MethodPasswordTOTP) && !h.options.Vault.Ready() {
		h.adminSignInPage(w, r, http.StatusBadRequest, "Password and code needs a key for secrets on the instance.", "", "")
		return
	}
	if secret := r.PostFormValue("oidc_client_secret"); secret != "" {
		if err := h.secrets().Put(ctx, oidcSecret, vault.Secret(secret)); err != nil {
			h.adminSignInPage(w, r, http.StatusBadRequest, "The client secret cannot be kept: the instance has no key for secrets.", "", "")
			return
		}
	} else if r.PostFormValue("oidc_forget_secret") == "on" {
		if err := h.secrets().Delete(ctx, oidcSecret); err != nil {
			h.passkeyFailed(w, err, "secret")
			return
		}
	}
	if err := h.options.DB.SetSignIn(ctx, s); err != nil {
		if errors.Is(err, db.ErrNoMethod) || errors.Is(err, db.ErrMethod) || errors.Is(err, db.ErrOIDC) {
			h.adminSignInPage(w, r, http.StatusBadRequest, capital(err.Error())+".", "", "")
			return
		}
		h.passkeyFailed(w, err, "sign-in settings")
		return
	}
	h.log.Info("sign-in settings changed", slog.String("by", h.me(r).Login), slog.Any("methods", s.Methods), slog.Bool("registration", s.Registration))
	h.adminSignInPage(w, r, http.StatusOK, "", "Saved.", "")
}

func capital(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// createInvite makes an invitation and shows its link once.
func (h *handler) createInvite(w http.ResponseWriter, r *http.Request) {
	if !h.adminOnly(w, r) || !h.formOK(w, r) {
		return
	}
	token, hash, err := auth.NewToken()
	if err == nil {
		err = h.options.DB.CreateInvite(r.Context(), hash, db.Invite{Login: r.PostFormValue("login"), Name: r.PostFormValue("name"),
			Admin: r.PostFormValue("admin") == "on", MadeBy: h.me(r).ID, Expires: time.Now().Add(inviteLife)})
	}
	switch {
	case errors.Is(err, db.ErrLogin), errors.Is(err, db.ErrTitle), errors.Is(err, db.ErrLoginTaken):
		h.adminSignInPage(w, r, http.StatusBadRequest, registerProblem(err), "", "")
		return
	case err != nil:
		h.passkeyFailed(w, err, "invitation")
		return
	}
	h.adminSignInPage(w, r, http.StatusOK, "", "Send this link to the person. It works once, for seven days, and is not shown again.",
		requestOrigin(r)+"/invite/"+token)
}
