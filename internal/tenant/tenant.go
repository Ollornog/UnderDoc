// SPDX-License-Identifier: MIT

// Package tenant knows which tenant a request belongs to (ADR-23). An instance runs in one of two modes: one tenant, the
// default, where nothing about tenants shows and every request belongs to Main, or many tenants, where the host name of
// a request names its tenant. The database separates tenants by row-level security; this package only carries the
// tenant from the request to the database.
package tenant

import (
	"context"
	"errors"
	"net"
	"regexp"
	"strings"
)

// Main is the tenant of an instance in the mode one tenant, and the one that existing data moves into.
const Main = "main"

// Hoster is the reserved tenant of the hoster administrators. It holds their accounts and nothing else.
const Hoster = "hoster"

// Modes of an instance.
const (
	ModeOne  = "one"
	ModeMany = "many"
)

// ErrID is a tenant ID that is not a valid DNS label.
var ErrID = errors.New("a tenant ID has 1 to 40 lower case letters, digits and inner hyphens")

var idPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)

// Valid tells whether s can be the ID of a tenant. It is one DNS label, so it can be the subdomain of the hoster.
func Valid(s string) bool { return idPattern.MatchString(s) }

type contextKey struct{}

// With returns a context that carries the tenant. Every query of the database runs as that tenant.
func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKey{}, id)
}

// From returns the tenant a context carries, or "" and false. A query without a tenant sees nothing.
func From(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(contextKey{}).(string)
	return id, ok && id != ""
}

// Host returns the host name of a request in the form tenants are found by: lower case, without port and final dot.
func Host(hostport string) string {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

// Subdomain returns the tenant a host names below the domain of the hoster, such as acme for acme.docs.example.
func Subdomain(host, domain string) (string, bool) {
	domain = Host(domain)
	if domain == "" {
		return "", false
	}
	label, ok := strings.CutSuffix(host, "."+domain)
	if !ok || !Valid(label) {
		return "", false
	}
	return label, true
}
