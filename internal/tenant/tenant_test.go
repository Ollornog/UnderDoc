// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"testing"
)

func TestValid(t *testing.T) {
	for _, s := range []string{"main", "a", "acme-2", "x1", "abcdefghijabcdefghijabcdefghijabcdefghij"} {
		if !Valid(s) {
			t.Errorf("%q is valid", s)
		}
	}
	for _, s := range []string{"", "-a", "a-", "Acme", "a.b", "a_b", "abcdefghijabcdefghijabcdefghijabcdefghijk", "ä"} {
		if Valid(s) {
			t.Errorf("%q is not valid", s)
		}
	}
}

func TestContext(t *testing.T) {
	if _, ok := From(context.Background()); ok {
		t.Error("no tenant without one")
	}
	if _, ok := From(With(context.Background(), "")); ok {
		t.Error("an empty tenant is none")
	}
	if got, ok := From(With(context.Background(), "acme")); !ok || got != "acme" {
		t.Errorf("%q %v", got, ok)
	}
}

func TestHost(t *testing.T) {
	for in, want := range map[string]string{
		"Acme.Docs.Example:8443": "acme.docs.example",
		"acme.docs.example.":     "acme.docs.example",
		"[::1]:80":               "::1",
		"localhost":              "localhost",
	} {
		if got := Host(in); got != want {
			t.Errorf("Host(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSubdomain(t *testing.T) {
	if got, ok := Subdomain("acme.docs.example", "Docs.Example"); !ok || got != "acme" {
		t.Errorf("%q %v", got, ok)
	}
	for _, host := range []string{"docs.example", "a.b.docs.example", "acmedocs.example", "-x.docs.example", "acme.other.example"} {
		if got, ok := Subdomain(host, "docs.example"); ok {
			t.Errorf("%s names %q", host, got)
		}
	}
	if _, ok := Subdomain("acme.docs.example", ""); ok {
		t.Error("no domain, no subdomain")
	}
}
