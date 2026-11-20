// SPDX-License-Identifier: MIT

package main

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
)

func freshKey(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// TestRekey: underdoc rekey seals a password of a page again under the new key, needs the old key to do it, and can run
// again (ADR-7).
func TestRekey(t *testing.T) {
	ctx := inMain()
	first, url := database(t)
	anna := account.User{ID: "u-anna", Login: "anna"}
	team, _ := first.CreateCollection(ctx, anna, "Team", "")
	page, _ := first.CreatePage(ctx, anna, "Router", team.ID)
	oldKey, newKey := freshKey(t), freshKey(t)
	before, _ := vault.New(oldKey)
	value := freshKey(t)
	secretID, err := first.CreatePageSecret(ctx, page.ID, anna.ID, func(name string) ([]byte, error) {
		return before.Seal(tenant.Main, name, vault.Secret(value))
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("UNDERDOC_DATABASE_URL", url)
	t.Setenv(vault.KeyEnv, newKey)
	t.Setenv(vault.OldKeyEnv, "")
	if code, _, errOut := runCommand(t, "rekey"); code != 1 || !strings.Contains(errOut, vault.OldKeyEnv) {
		t.Errorf("rekey without the old key = %d %s", code, errOut)
	}
	t.Setenv(vault.OldKeyEnv, oldKey)
	code, stdout, errOut := runCommand(t, "rekey")
	if code != 0 || !strings.Contains(stdout, "main: 1 sealed again") || strings.Contains(stdout+errOut, value) {
		t.Fatalf("rekey = %d %s %s", code, stdout, errOut)
	}
	if code, stdout, _ := runCommand(t, "rekey"); code != 0 || !strings.Contains(stdout, "0 values sealed again") {
		t.Errorf("a second rekey = %d %s", code, stdout)
	}
	sealed, name, err := first.RevealPageSecret(ctx, secretID, anna.ID, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := vault.New(newKey)
	if got, err := after.Open(tenant.Main, name, sealed); err != nil || string(got) != value {
		t.Errorf("the new key does not open it: %v", err)
	}
}
