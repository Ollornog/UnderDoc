// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/auth"
	"github.com/Ollornog/UnderDoc/internal/db"
)

// OIDC with any provider (ADR-23): the authorization code flow with PKCE, a state bound to the browser by the ceremony
// cookie, and a nonce in the ID token. The account is named by preferred_username, so a tenant that came through a gate
// keeps its accounts, and its groups come from a claim the tenant names. The provider must not let people choose their
// preferred_username freely, or one could take the name of another.

// oidcProviders keeps what the discovery of an issuer found, for an hour.
type oidcProviders struct {
	mu    sync.Mutex
	known map[string]oidcProvider
}

type oidcProvider struct {
	provider *oidc.Provider
	fetched  time.Time
}

func (h *handler) oidcContext(ctx context.Context) context.Context {
	client := h.options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return oidc.ClientContext(ctx, client)
}

func (h *handler) provider(ctx context.Context, issuer string) (*oidc.Provider, error) {
	h.oidc.mu.Lock()
	defer h.oidc.mu.Unlock()
	if p, ok := h.oidc.known[issuer]; ok && time.Since(p.fetched) < time.Hour {
		return p.provider, nil
	}
	p, err := oidc.NewProvider(h.oidcContext(ctx), issuer)
	if err != nil {
		return nil, err
	}
	if h.oidc.known == nil {
		h.oidc.known = map[string]oidcProvider{}
	}
	h.oidc.known[issuer] = oidcProvider{provider: p, fetched: time.Now()}
	return p, nil
}

// oidcStep is the ceremony between leaving for the provider and coming back.
type oidcStep struct {
	State    string `json:"state"`
	Nonce    string `json:"nonce"`
	Verifier string `json:"verifier"`
	Next     string `json:"next"`
}

func (h *handler) oauthConfig(ctx context.Context, r *http.Request, s db.SignIn, p *oidc.Provider) (oauth2.Config, error) {
	conf := oauth2.Config{ClientID: s.OIDCClientID, Endpoint: p.Endpoint(), RedirectURL: requestOrigin(r) + "/sign-in/oidc/callback",
		Scopes: []string{oidc.ScopeOpenID, "profile", "email"}}
	if s.OIDCGroupsClaim == "groups" {
		conf.Scopes = append(conf.Scopes, "groups")
	}
	if h.options.Vault.Ready() && h.secrets().Has(ctx, oidcSecret) {
		secret, err := h.secrets().Use(ctx, oidcSecret)
		if err != nil {
			return conf, err
		}
		conf.ClientSecret = string(secret)
	}
	return conf, nil
}

// oidcStart leaves for the identity provider of the tenant.
func (h *handler) oidcStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s := h.signInSettings(ctx)
	if h.options.DB == nil || !s.Has(db.MethodOIDC) {
		h.notFound(w, r)
		return
	}
	p, err := h.provider(ctx, s.OIDCIssuer)
	if err != nil {
		h.log.Error("OIDC discovery failed", slog.String("issuer", s.OIDCIssuer), slog.String("error", err.Error()))
		h.signInPage(w, r, http.StatusBadGateway, "The identity provider does not answer. Try again later.")
		return
	}
	conf, err := h.oauthConfig(ctx, r, s, p)
	if err != nil {
		h.passkeyFailed(w, err, "OIDC")
		return
	}
	state, _, err := auth.NewToken()
	if err != nil {
		h.passkeyFailed(w, err, "OIDC")
		return
	}
	nonce, _, err := auth.NewToken()
	if err != nil {
		h.passkeyFailed(w, err, "OIDC")
		return
	}
	step := oidcStep{State: state, Nonce: nonce, Verifier: oauth2.GenerateVerifier(), Next: safeNext(r.URL.Query().Get("next"))}
	if err := h.putCeremony(w, r, "oidc", "", step); err != nil {
		h.passkeyFailed(w, err, "OIDC")
		return
	}
	http.Redirect(w, r, conf.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(step.Verifier)), http.StatusSeeOther)
}

// oidcCallback comes back from the identity provider and signs the account in.
func (h *handler) oidcCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s := h.signInSettings(ctx)
	if h.options.DB == nil || !s.Has(db.MethodOIDC) {
		h.notFound(w, r)
		return
	}
	refuse := func(login, reason string) {
		h.logSignIn(ctx, login, db.MethodOIDC, false, reason)
		h.signInPage(w, r, http.StatusUnauthorized, "The sign-in with "+orDefault(s.OIDCName, "single sign-on")+" did not work. Try again.")
	}
	var step oidcStep
	if _, err := h.takeCeremony(w, r, "oidc", &step); err != nil {
		refuse("", "no ceremony")
		return
	}
	q := r.URL.Query()
	if q.Get("state") == "" || q.Get("state") != step.State {
		refuse("", "state does not match")
		return
	}
	if e := q.Get("error"); e != "" {
		refuse("", "provider: "+clipText(e, 100))
		return
	}
	p, err := h.provider(ctx, s.OIDCIssuer)
	if err != nil {
		refuse("", "provider does not answer")
		return
	}
	conf, err := h.oauthConfig(ctx, r, s, p)
	if err != nil {
		refuse("", "client secret does not open")
		return
	}
	octx := h.oidcContext(ctx)
	token, err := conf.Exchange(octx, q.Get("code"), oauth2.VerifierOption(step.Verifier))
	if err != nil {
		refuse("", "code exchange failed")
		return
	}
	raw, _ := token.Extra("id_token").(string)
	idToken, err := p.Verifier(&oidc.Config{ClientID: s.OIDCClientID}).Verify(octx, raw)
	if err != nil {
		refuse("", "ID token not valid")
		return
	}
	if idToken.Nonce == "" || idToken.Nonce != step.Nonce {
		refuse("", "nonce does not match")
		return
	}
	var claims map[string]any
	if err := idToken.Claims(&claims); err != nil {
		refuse("", "claims unreadable")
		return
	}
	user, ok := oidcUser(claims, s.OIDCGroupsClaim)
	if !ok {
		refuse("", "no usable preferred_username")
		return
	}
	if err := h.options.DB.SeeExternal(ctx, user, db.SourceOIDC); err != nil {
		if errors.Is(err, db.ErrSource) {
			refuse(user.Login, "a local account has this name")
			return
		}
		if errors.Is(err, db.ErrQuota) {
			h.logSignIn(ctx, user.Login, db.MethodOIDC, false, "workspace full")
			h.signInPage(w, r, http.StatusForbidden, quotaText)
			return
		}
		h.passkeyFailed(w, err, "OIDC")
		return
	}
	if err := h.startSession(w, r, user.ID, db.MethodOIDC); err != nil {
		h.passkeyFailed(w, err, "OIDC")
		return
	}
	h.logSignIn(ctx, user.Login, db.MethodOIDC, true, "")
	http.Redirect(w, r, safeNext(step.Next), http.StatusSeeOther)
}

// oidcUser reads the account from the claims of an ID token: preferred_username names it, and the groups come from the
// claim the tenant named, as a list or as one string.
func oidcUser(claims map[string]any, groupsClaim string) (account.User, bool) {
	login, _ := claims["preferred_username"].(string)
	login = strings.TrimSpace(login)
	if login == "" || len([]rune(login)) > 200 || strings.ContainsFunc(login, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) {
		return account.User{}, false
	}
	user := account.User{ID: login, Login: login, Name: login, Groups: []string{}}
	if name, ok := claims["name"].(string); ok && strings.TrimSpace(name) != "" {
		user.Name = clipText(strings.TrimSpace(name), 200)
	}
	if email, ok := claims["email"].(string); ok {
		user.Email = clipText(strings.TrimSpace(email), 200)
	}
	switch g := claims[groupsClaim].(type) {
	case []any:
		for _, item := range g {
			if s, ok := item.(string); ok && s != "" && len(user.Groups) < 200 {
				user.Groups = append(user.Groups, clipText(s, 200))
			}
		}
	case string:
		for _, s := range strings.FieldsFunc(g, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
			user.Groups = append(user.Groups, clipText(s, 200))
		}
	}
	return user, true
}

func clipText(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
