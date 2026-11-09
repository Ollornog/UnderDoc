// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSettingsOfAnAccount(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	if got, err := db.Settings(ctx, anna.ID); err != nil || string(got) != "{}" {
		t.Fatalf("an account starts with no settings: %s %v", got, err)
	}
	if err := db.SaveSettings(ctx, anna.ID, []byte(`{"s2": 2, "n2": ["abc", {"k": "line", "id": "d1234"}]}`)); err != nil {
		t.Fatal(err)
	}
	got, _ := db.Settings(ctx, anna.ID)
	if !strings.Contains(string(got), `"s2": 2`) || !strings.Contains(string(got), `"k": "line"`) {
		t.Errorf("the settings come back: %s", got)
	}
	if other, _ := db.Settings(ctx, bob.ID); string(other) != "{}" {
		t.Errorf("they are Anna's alone: %s", other)
	}
	for _, bad := range []string{`[1, 2]`, `"text"`, `{"open": `, `null`, `{"x": "` + strings.Repeat("a", MaxSettings) + `"}`} {
		if err := db.SaveSettings(ctx, anna.ID, []byte(bad)); !errors.Is(err, ErrSettings) {
			t.Errorf("%.30q is no settings: %v", bad, err)
		}
	}
	if err := db.SaveSettings(ctx, "nobody", []byte(`{}`)); !errors.Is(err, ErrNotFound) {
		t.Errorf("an account that is not there: %v", err)
	}
	if _, err := db.Settings(ctx, "nobody"); !errors.Is(err, ErrNotFound) {
		t.Errorf("settings of an account that is not there: %v", err)
	}
}
