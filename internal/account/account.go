// SPDX-License-Identifier: MIT

// Package account knows who is signed in. One way in is the sign-in gate in front of the application, which checks the
// user and passes them on in request headers (ADR-9). The proxy that runs the gate removes these headers from every
// request before it sets them, so the application only trusts them because it is reachable through that proxy alone,
// and only where a tenant uses the gate (ADR-23).
package account

import (
	"context"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The headers the gate sets. Remote-User is the one that must be there.
const (
	HeaderUser   = "Remote-User"
	HeaderID     = "Remote-Id"
	HeaderName   = "Remote-Name"
	HeaderEmail  = "Remote-Email"
	HeaderGroups = "Remote-Groups"
	// HeaderAuth says how the request signed in at the gate (session, oauth, apikey, app-password), HeaderKind whether
	// the account is a person or a service account (ADR-35).
	HeaderAuth = "Remote-Auth"
	HeaderKind = "Remote-Kind"
)

// How a request signed in (ADR-35, ADR-36). Only an interactive session is a person at work; every other way is an
// agent acting for them.
const (
	AuthSession     = "session"
	AuthOAuth       = "oauth"
	AuthAPIKey      = "apikey"
	AuthAppPassword = "app-password"
)

// Kinds of accounts: a person, or a service account that gets what is granted to it (ADR-35).
const (
	KindPerson  = "person"
	KindService = "service"
)

// AuthKindOf reads the way a request signed in. A value it does not know is kept as it is, and counts as an agent.
func AuthKindOf(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

// KindOf reads the kind of an account. Only "service" is a service account; anything else is a person.
func KindOf(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), KindService) {
		return KindService
	}
	return KindPerson
}

// User is the person behind a request.
type User struct {
	// ID is stable for a person, even when the login changes. Without one from the gate, it is the login.
	ID     string
	Login  string
	Name   string
	Email  string
	Groups []string
	// Admin tells that the user administers the tenant: a local account marked so, or a member of its admin group.
	Admin bool
	// Method is the way the user signed in (db.Method*), set by the web.
	Method string
	// AuthKind is how this request signed in (AuthSession, AuthOAuth, …). The web sets it from the session or from what
	// the gate checked (T-27 will take it from the gate's checked result). Empty or unknown counts as an agent.
	AuthKind string
	// Kind is KindService for a service account; anything else is a person.
	Kind string
}

// Agent tells whether the request belongs to the built-in group Agents (ADR-36): every sign-in that is not an
// interactive session, and a missing or unknown one too (fail closed). A service account is a case of its own: it
// gets what is granted to it, not this group.
func (u User) Agent() bool {
	if u.Kind == KindService {
		return false
	}
	return u.AuthKind != AuthSession
}

// FromRequest reads the user from the headers of the gate. It reports false if there is none.
func FromRequest(r *http.Request) (User, bool) {
	login := strings.TrimSpace(r.Header.Get(HeaderUser))
	if login == "" {
		return User{}, false
	}
	user := User{
		ID:    strings.TrimSpace(r.Header.Get(HeaderID)),
		Login: login,
		Name:  strings.TrimSpace(r.Header.Get(HeaderName)),
		Email: strings.TrimSpace(r.Header.Get(HeaderEmail)),
	}
	if user.ID == "" {
		user.ID = login
	}
	if user.Name == "" {
		user.Name = login
	}
	user.Groups = splitGroups(r.Header.Get(HeaderGroups))
	user.AuthKind = AuthKindOf(r.Header.Get(HeaderAuth))
	user.Kind = KindOf(r.Header.Get(HeaderKind))
	return user, true
}

// splitGroups accepts commas and spaces between groups, each group once.
func splitGroups(value string) []string {
	var groups []string
	for _, group := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
		if !contains(groups, group) {
			groups = append(groups, group)
		}
	}
	return groups
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// Initials are up to two letters for the avatar: the first letters of the first and the last word of the name.
func (u User) Initials() string {
	words := strings.Fields(u.Name)
	if len(words) == 0 {
		return "?"
	}
	first := func(word string) string {
		r, _ := utf8.DecodeRuneInString(word)
		return strings.ToUpper(string(r))
	}
	if len(words) == 1 {
		return first(words[0])
	}
	return first(words[0]) + first(words[len(words)-1])
}

type contextKey struct{}

// With returns a context that carries the user.
func With(ctx context.Context, user User) context.Context {
	return context.WithValue(ctx, contextKey{}, user)
}

// From returns the user a context carries.
func From(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(contextKey{}).(User)
	return user, ok
}
