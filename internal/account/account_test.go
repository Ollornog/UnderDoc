// SPDX-License-Identifier: MIT

package account

import (
	"context"
	"testing"
)

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
