// SPDX-License-Identifier: MIT

// Package account knows who is signed in. UnderDoc signs in only through the gate in front of it, TinySesam, whose Go
// client checks what the gate passes on (ADR-32, gate.go). A person is bound by the lasting ID the gate gives the
// account, never by a name.
package account

import (
	"context"
	"strings"
	"unicode/utf8"
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

// User is the person behind a request.
type User struct {
	// ID is the account in UnderDoc. For an account of the gate it is the account bound to RemoteID, mostly that ID.
	ID string
	// RemoteID is the lasting ID of the account in the gate (Remote-Id, ts_id): the one key a person is bound by.
	RemoteID string
	Login    string
	Name     string
	Email    string
	Groups   []string
	// Admin tells that the user administers the tenant: the gate gives the role of the app, or the account is marked so.
	Admin bool
	// AdminRole tells that the gate gives the role of the app, not only the mark of the command line.
	AdminRole bool
	// Spaces tells that the gate gives the role of the app to create spaces, the collections at the top (B-3).
	Spaces bool
	// AuthKind is how this request signed in (AuthSession, AuthOAuth, …), as the gate's client checked it. Empty or
	// unknown counts as an agent.
	AuthKind string
	// Kind is KindService for a service account an administrator allowed by its lasting ID (B-3); anything else is a
	// person.
	Kind string
	// GateKind is the kind of account the gate passed on ("user" or "service"), kept as it came; empty when it sent none.
	GateKind string
}

// Agent tells whether the request belongs to the built-in group Agents (ADR-36): every sign-in that is not an
// interactive session, and a missing or unknown one too (fail closed). A service account is always an agent (B-3).
func (u User) Agent() bool {
	if u.Kind == KindService {
		return true
	}
	return u.AuthKind != AuthSession
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
