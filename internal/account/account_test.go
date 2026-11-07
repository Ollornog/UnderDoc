// SPDX-License-Identifier: MIT

package account

import (
	"context"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestFromRequest(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if _, ok := FromRequest(r); ok {
		t.Fatal("a request without the gate headers has no user")
	}
	r.Header.Set(HeaderUser, "  ")
	if _, ok := FromRequest(r); ok {
		t.Fatal("a blank user is no user")
	}
	r.Header.Set(HeaderUser, "anna")
	r.Header.Set(HeaderID, "7f3c")
	r.Header.Set(HeaderName, "Anna Berg")
	r.Header.Set(HeaderEmail, "anna@example.com")
	r.Header.Set(HeaderGroups, "editors, ops editors")
	user, ok := FromRequest(r)
	if !ok || user.ID != "7f3c" || user.Login != "anna" || user.Name != "Anna Berg" || user.Email != "anna@example.com" {
		t.Fatalf("%+v %v", user, ok)
	}
	if !slices.Equal(user.Groups, []string{"editors", "ops"}) {
		t.Fatalf("groups %q", user.Groups)
	}
}

func TestFallbacks(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set(HeaderUser, "bob")
	user, _ := FromRequest(r)
	if user.ID != "bob" || user.Name != "bob" || user.Groups != nil {
		t.Fatalf("%+v", user)
	}
}

func TestInitials(t *testing.T) {
	for name, want := range map[string]string{"Anna Berg": "AB", "anna": "A", "Anna Maria von Berg": "AB", "": "?", "élise roux": "ÉR"} {
		if got := (User{Name: name}).Initials(); got != want {
			t.Errorf("%q: got %q, want %q", name, got, want)
		}
	}
}

func TestContext(t *testing.T) {
	if _, ok := From(context.Background()); ok {
		t.Fatal("an empty context has no user")
	}
	user, ok := From(With(context.Background(), User{Login: "anna"}))
	if !ok || user.Login != "anna" {
		t.Fatal(user, ok)
	}
}
