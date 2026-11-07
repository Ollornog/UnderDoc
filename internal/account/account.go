// SPDX-License-Identifier: MIT

// Package account knows who is signed in. The sign-in gate in front of the application checks the user and passes them
// on in request headers (ADR-9). The proxy that runs the gate removes these headers from every request before it sets
// them, so the application only trusts them because it is reachable through that proxy alone.
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
)

// User is the person behind a request.
type User struct {
	// ID is stable for a person, even when the login changes. Without one from the gate, it is the login.
	ID     string
	Login  string
	Name   string
	Email  string
	Groups []string
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
