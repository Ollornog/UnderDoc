// SPDX-License-Identifier: MIT

package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/auth"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
)

// Sign-in built in (ADR-23). Each tenant chooses its ways in: local accounts with a passkey, with a password and TOTP or
// with a password alone, OIDC, and the gate in front. A session is a cookie with a random token, HttpOnly, Secure and
// SameSite=Lax, whose hash the database keeps; it holds only as long as the way it came in is switched on.

const (
	sessionCookie  = "__Host-underdoc"
	ceremonyCookie = "__Host-underdoc-step"
	// A session ends after a week without use, and after a month in any case.
	sessionIdle  = 7 * 24 * time.Hour
	sessionLife  = 30 * 24 * time.Hour
	ceremonyLife = 5 * time.Minute
	inviteLife   = 7 * 24 * time.Hour
	formLimit    = 16 << 10
)

// Gate settings of the instance (Options.Gate). The headers of a gate can be forged by anyone who reaches UnderDoc
// without it, so only whoever runs the instance knows whether they can be trusted, never the administrator of a tenant.
const (
	// GateKeep trusts the gate only for a tenant that has it already, as Main of an instance that ran behind one.
	GateKeep = ""
	// GateOn lets administrators switch the gate on for their tenant.
	GateOn = "on"
	// GateOff never trusts the headers of a gate.
	GateOff = "off"
)

// signInSettings returns how the tenant of the request lets people in. Without the database there is only the gate.
func (h *handler) signInSettings(ctx context.Context) db.SignIn {
	if h.options.DB == nil {
		return db.SignIn{Methods: []string{db.MethodGate}}
	}
	s, err := h.options.DB.SignIn(ctx)
	if err != nil {
		h.log.Error("sign-in settings failed", slog.String("error", err.Error()))
		return db.SignIn{}
	}
	return s
}

// gateTrusted tells whether the headers of the gate count for this tenant.
func (h *handler) gateTrusted(s db.SignIn) bool {
	return h.options.Gate != GateOff && s.Has(db.MethodGate)
}

// adminGroup is the group whose members administer the tenant.
func (h *handler) adminGroup(s db.SignIn) string {
	if s.AdminGroup != "" {
		return s.AdminGroup
	}
	return h.options.AdminGroup
}

// identify finds who sent a request: the gate where the tenant trusts it, else the session cookie, else the user that
// stands in for development.
func (h *handler) identify(r *http.Request, s db.SignIn) (account.User, bool) {
	ctx := r.Context()
	group := h.adminGroup(s)
	if h.gateTrusted(s) {
		if user, ok := account.FromRequest(r); ok {
			user.Admin = group != "" && slices.Contains(user.Groups, group)
			user.Method = db.MethodGate
			return user, true
		}
	}
	if h.options.DB != nil {
		if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
			accountID, method, err := h.options.DB.Session(ctx, auth.HashToken(c.Value), sessionIdle)
			if err == nil && s.Has(method) {
				if a, err := h.options.DB.AccountByID(ctx, accountID); err == nil {
					user := a.User
					user.Admin = a.Admin || group != "" && slices.Contains(a.Groups, group)
					user.Method = method
					return user, true
				}
			} else if err != nil && !errors.Is(err, db.ErrNotFound) {
				h.log.Error("session failed", slog.String("error", err.Error()))
			}
		}
	}
	if h.options.DevUser != nil {
		user := *h.options.DevUser
		user.Admin = group != "" && slices.Contains(user.Groups, group)
		user.Method = db.MethodGate
		return user, true
	}
	return account.User{}, false
}

// public tells the paths that answer without a sign-in: the pages to sign in, to accept an invitation and to register.
func public(r *http.Request) bool {
	p := r.URL.Path
	return p == "/sign-in" || strings.HasPrefix(p, "/sign-in/") || strings.HasPrefix(p, "/invite/") || p == "/register"
}

// onlyGate tells whether the gate is the only way in, as before ADR-23: then a request without it gets the page that
// says so, unchanged.
func onlyGate(s db.SignIn) bool { return len(s.Methods) == 1 && s.Has(db.MethodGate) }

// notSignedIn answers a request without a user: the page of the gate where that is the only way in, else the way to
// the sign-in page for a page, and 401 for the API.
func (h *handler) notSignedIn(w http.ResponseWriter, r *http.Request, s db.SignIn) {
	if onlyGate(s) || len(s.Methods) == 0 {
		h.message(w, http.StatusUnauthorized, message{
			Title: "Not signed in",
			Text:  "UnderDoc is reached through its sign-in gate, which tells it who you are. This request came without it.",
			Link:  "Try again",
		})
		return
	}
	if r.Method == http.MethodGet && !strings.HasPrefix(r.URL.Path, "/api/") {
		http.Redirect(w, r, "/sign-in?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		return
	}
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not signed in"})
}

// quotaText says that a tenant has as many accounts as its hoster allows.
const quotaText = "This workspace has as many accounts as its hoster allows. Ask its administrator."

// safeNext keeps an address to go on to after signing in only if it stays on this host.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") || strings.ContainsAny(next, "\r\n") {
		return "/"
	}
	if public(&http.Request{URL: &url.URL{Path: strings.SplitN(next, "?", 2)[0]}}) {
		return "/"
	}
	return next
}

// startSession signs the browser in. The session it came with ends first, so a token set before the sign-in never
// becomes a signed-in one.
func (h *handler) startSession(w http.ResponseWriter, r *http.Request, accountID, method string) error {
	ctx := r.Context()
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		if err := h.options.DB.EndSession(ctx, auth.HashToken(c.Value)); err != nil {
			return err
		}
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		return err
	}
	if err := h.options.DB.CreateSession(ctx, hash, accountID, method, time.Now().Add(sessionLife)); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", MaxAge: int(sessionLife.Seconds()),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	return nil
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
}

// logSignIn keeps a sign-in or a failed try in the log of the tenant, and in the log of the instance without anything
// secret.
func (h *handler) logSignIn(ctx context.Context, login, method string, ok bool, reason string) {
	t, _ := tenant.From(ctx)
	h.log.LogAttrs(ctx, slog.LevelInfo, "sign-in", slog.String("tenant", t), slog.String("login", login), slog.String("method", method),
		slog.Bool("ok", ok), slog.String("reason", reason))
	if err := h.options.DB.LogSignIn(ctx, db.SignInEvent{Login: login, Method: method, OK: ok, Reason: reason}); err != nil {
		h.log.Error("sign-in log failed", slog.String("error", err.Error()))
	}
}

// secrets is the store of sealed secrets of the tenant of a request.
func (h *handler) secrets() vault.Store {
	return vault.Store{Vault: h.options.Vault, Rows: h.options.DB}
}

// --- Ceremonies: what the second request of a passkey or OIDC sign-in needs, kept in the database, found by a cookie.

func (h *handler) putCeremony(w http.ResponseWriter, r *http.Request, kind, accountID string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	token, hash, err := auth.NewToken()
	if err != nil {
		return err
	}
	if err := h.options.DB.PutCeremony(r.Context(), hash, kind, accountID, raw, time.Now().Add(ceremonyLife)); err != nil {
		return err
	}
	// Lax, not Strict: the way back from an identity provider is a navigation from another site.
	http.SetCookie(w, &http.Cookie{Name: ceremonyCookie, Value: token, Path: "/", MaxAge: int(ceremonyLife.Seconds()),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	return nil
}

// takeCeremony returns the ceremony of the cookie and ends it, so it works once.
func (h *handler) takeCeremony(w http.ResponseWriter, r *http.Request, kind string, into any) (string, error) {
	c, err := r.Cookie(ceremonyCookie)
	if err != nil || c.Value == "" {
		return "", db.ErrNotFound
	}
	clearCookie(w, ceremonyCookie)
	accountID, raw, err := h.options.DB.TakeCeremony(r.Context(), auth.HashToken(c.Value), kind)
	if err != nil {
		return "", err
	}
	return accountID, json.Unmarshal(raw, into)
}

// --- Pages

// signInView fills the pages of signing in.
type signInView struct {
	Version  string
	Title    string
	Tenant   string
	Error    string
	Notice   string
	Next     string
	Passkey  bool
	Password bool
	TOTP     bool
	OIDC     string
	Register bool
	Login    string
}

func (h *handler) page(w http.ResponseWriter, status int, name string, view any) {
	var out bytes.Buffer
	if err := h.pages.ExecuteTemplate(&out, name, view); err != nil {
		h.log.Error("template failed", slog.String("template", name), slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(out.Bytes())
}

func (h *handler) tenantName(ctx context.Context) string {
	if h.options.Tenants != tenant.ModeMany {
		return "UnderDoc"
	}
	t, _ := tenant.From(ctx)
	if found, err := h.options.DB.Tenant(ctx, t); err == nil {
		return found.Name
	}
	return "UnderDoc"
}

func (h *handler) signInPage(w http.ResponseWriter, r *http.Request, status int, problem string) {
	ctx := r.Context()
	s := h.signInSettings(ctx)
	if h.options.DB == nil || onlyGate(s) {
		h.notFound(w, r)
		return
	}
	v := signInView{Version: h.assetVersion, Title: "Sign in", Tenant: h.tenantName(ctx), Error: problem, Next: safeNext(r.FormValue("next")),
		Passkey: s.Has(db.MethodPasskey), Password: s.Has(db.MethodPassword) || s.Has(db.MethodPasswordTOTP),
		TOTP: s.Has(db.MethodPasswordTOTP), Register: s.Registration, Login: strings.TrimSpace(r.PostFormValue("login"))}
	if s.Has(db.MethodOIDC) {
		v.OIDC = s.OIDCName
		if v.OIDC == "" {
			v.OIDC = "single sign-on"
		}
	}
	if r.URL.Query().Get("out") == "1" {
		v.Notice = "You are signed out."
	}
	h.page(w, status, "signin", v)
}

// signIn shows the ways in of the tenant. Someone signed in already goes on.
func (h *handler) signIn(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.identify(r, h.signInSettings(r.Context())); ok && r.URL.Query().Get("out") == "" {
		http.Redirect(w, r, safeNext(r.URL.Query().Get("next")), http.StatusSeeOther)
		return
	}
	h.signInPage(w, r, http.StatusOK, "")
}

// formOK reads a form sent from a page of this application, and refuses one from anywhere else.
func (h *handler) formOK(w http.ResponseWriter, r *http.Request) bool {
	if !sameOrigin(r) {
		h.message(w, http.StatusForbidden, message{Title: "Not from here", Text: "This form came from another site.", Link: "To the start"})
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, formLimit)
	if err := r.ParseForm(); err != nil {
		h.message(w, http.StatusBadRequest, message{Title: "Bad request", Text: "The form could not be read.", Link: "To the start"})
		return false
	}
	return true
}

func (h *handler) throttleKey(ctx context.Context, login string) string {
	t, _ := tenant.From(ctx)
	return t + "\x00" + strings.ToLower(login)
}

// waitText says how long a try has to wait.
func waitText(wait time.Duration) string {
	return "Too many tries. Try again in " + strconv.Itoa(int(wait.Round(time.Second).Seconds())+1) + " seconds."
}

// signInPassword signs in a local account with its password and, where it has one, its TOTP code.
func (h *handler) signInPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s := h.signInSettings(ctx)
	if h.options.DB == nil || !s.Has(db.MethodPassword) && !s.Has(db.MethodPasswordTOTP) {
		h.notFound(w, r)
		return
	}
	if !h.formOK(w, r) {
		return
	}
	login := strings.ToLower(strings.TrimSpace(r.PostFormValue("login")))
	key := h.throttleKey(ctx, login)
	if wait := h.throttle.Wait(key); wait > 0 {
		h.logSignIn(ctx, login, db.MethodPassword, false, "too many tries")
		h.signInPage(w, r, http.StatusTooManyRequests, waitText(wait))
		return
	}
	a, err := h.options.DB.LocalAccount(ctx, login)
	if err != nil && !errors.Is(err, db.ErrNotFound) {
		h.log.Error("account failed", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	method, reason := db.MethodPassword, ""
	switch {
	case !auth.VerifyPassword(a.PasswordHash, r.PostFormValue("password")):
		reason = "wrong login or password"
	case a.TOTPSealed != nil:
		method = db.MethodPasswordTOTP
		if !h.checkTOTP(ctx, a, r.PostFormValue("code")) {
			reason = "wrong code"
		}
	case !s.Has(db.MethodPassword):
		reason = "no TOTP set up"
	}
	if reason != "" {
		h.throttle.Failed(key)
		h.logSignIn(ctx, login, method, false, reason)
		h.signInPage(w, r, http.StatusUnauthorized, "The login, the password or the code is wrong.")
		return
	}
	// A session made with TOTP lasts while either password way is on; one without, only while password alone is.
	if method == db.MethodPasswordTOTP && !s.Has(db.MethodPasswordTOTP) {
		method = db.MethodPassword
	}
	h.throttle.Succeeded(key)
	if err := h.startSession(w, r, a.ID, method); err != nil {
		h.log.Error("session failed", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.logSignIn(ctx, a.Login, method, true, "")
	http.Redirect(w, r, safeNext(r.PostFormValue("next")), http.StatusSeeOther)
}

// checkTOTP tells whether a code is right for the account and was not used before, and marks it used.
func (h *handler) checkTOTP(ctx context.Context, a db.Account, code string) bool {
	t, _ := tenant.From(ctx)
	secret, err := h.options.Vault.Open(t, totpName(a.ID), a.TOTPSealed)
	if err != nil {
		h.log.Error("TOTP secret does not open", slog.String("error", err.Error()))
		return false
	}
	last, err := h.options.DB.TOTPStep(ctx, a.ID)
	if err != nil {
		return false
	}
	step, ok := auth.CheckTOTP(string(secret), code, time.Now(), last)
	if !ok {
		return false
	}
	advanced, err := h.options.DB.UseTOTPStep(ctx, a.ID, step)
	return err == nil && advanced
}

// totpName binds a sealed TOTP secret to its account.
func totpName(accountID string) string { return "totp:" + accountID }

// signOut ends the session of the browser.
func (h *handler) signOut(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		h.message(w, http.StatusForbidden, message{Title: "Not from here", Text: "This form came from another site.", Link: "To the start"})
		return
	}
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" && h.options.DB != nil {
		if err := h.options.DB.EndSession(r.Context(), auth.HashToken(c.Value)); err != nil {
			h.log.Error("sign-out failed", slog.String("error", err.Error()))
		}
	}
	clearCookie(w, sessionCookie)
	http.Redirect(w, r, "/sign-in?out=1", http.StatusSeeOther)
}

// requestOrigin is the origin the browser sees: https, but http on the own machine.
func requestOrigin(r *http.Request) string {
	if host := tenant.Host(r.Host); host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return "http://" + r.Host
	}
	return "https://" + r.Host
}

// isAdmin tells whether the viewer administers the tenant.
func (h *handler) isAdmin(r *http.Request) bool { return h.me(r).Admin }
