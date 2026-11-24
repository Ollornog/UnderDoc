// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/gateapi/gatetest"
)

// The start checks what UnderDoc may do in the gate against the gate itself, and stops on any contradiction (T-33).
func TestTheStartChecksTheAccountAPI(t *testing.T) {
	gate, err := account.NewGate(account.GateConfig{})
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	g := gatetest.New(t, "underdoc", []string{"underdoc-user", "underdoc-reader"}, "underdoc-admin")
	key := g.KeyFile()
	for _, name := range []string{"UNDERDOC_GATE_URL", "UNDERDOC_GATE_SERVICE_KEY_FILE", "UNDERDOC_ACCOUNTS_ROLES", "UNDERDOC_APPROVALS", "UNDERDOC_GATE_APP"} {
		t.Setenv(name, "")
	}
	if api, err := readGateAPI(context.Background(), gate, log); err != nil || api != nil {
		t.Fatalf("nothing configured: %+v %v", api, err)
	}
	t.Setenv("UNDERDOC_GATE_URL", g.URL)
	t.Setenv("UNDERDOC_GATE_SERVICE_KEY_FILE", key)
	for name, c := range map[string]struct{ roles, appID, approvals, want string }{
		"the administrator's role":     {"underdoc-user,underdoc-admin", "", "", "administrators"},
		"the delegate's role":          {"underdoc-delegate", "", "", "administrators"},
		"the role to create spaces":    {"underdoc-spaces", "", "", "administrators"},
		"the gate's own admin":         {"admin", "", "", "administrators"},
		"a role the gate refuses":      {"staff", "", "", "does not let UnderDoc give"},
		"another app":                  {"underdoc-user", "wiki", "", "belongs to the app"},
		"approvals neither on nor off": {"", "", "yes", "neither on nor off"},
	} {
		t.Setenv("UNDERDOC_ACCOUNTS_ROLES", c.roles)
		t.Setenv("UNDERDOC_GATE_APP", c.appID)
		t.Setenv("UNDERDOC_APPROVALS", c.approvals)
		if _, err := readGateAPI(context.Background(), gate, log); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	t.Setenv("UNDERDOC_GATE_APP", "")
	t.Setenv("UNDERDOC_APPROVALS", "")
	// The approver's role on the allowlist of the gate is still never UnderDoc's to give.
	g2 := gatetest.New(t, "underdoc", []string{"underdoc-user", "teamadmin"}, "teamadmin")
	t.Setenv("UNDERDOC_GATE_URL", g2.URL)
	t.Setenv("UNDERDOC_GATE_SERVICE_KEY_FILE", g2.KeyFile())
	t.Setenv("UNDERDOC_ACCOUNTS_ROLES", "teamadmin")
	if _, err := readGateAPI(context.Background(), gate, log); err == nil || !strings.Contains(err.Error(), "approver role") {
		t.Errorf("the approver's role: %v", err)
	}
	t.Setenv("UNDERDOC_GATE_URL", g.URL)
	t.Setenv("UNDERDOC_GATE_SERVICE_KEY_FILE", key)
	g.SetScopes("users:read")
	t.Setenv("UNDERDOC_ACCOUNTS_ROLES", "underdoc-user")
	if _, err := readGateAPI(context.Background(), gate, log); err == nil || !strings.Contains(err.Error(), "needs at least") {
		t.Errorf("too few scopes: %v", err)
	}
	g.SetScopes("users:read", "users:invite", "users:disable")
	api, err := readGateAPI(context.Background(), gate, log)
	if err != nil || len(api.Roles) != 1 || api.ApproverRole != "" || api.App != "underdoc" || api.Approvals {
		t.Fatalf("without users:reset: %+v %v", api, err)
	}
	g.SetScopes("users:read", "users:invite", "users:disable", "users:reset")
	t.Setenv("UNDERDOC_APPROVALS", "on")
	if api, err = readGateAPI(context.Background(), gate, log); err != nil || api.ApproverRole != "underdoc-admin" || !api.Approvals {
		t.Errorf("with users:reset and approvals: %+v %v", api, err)
	}
	// Approvals alone need the gate's address and key, not its account API.
	t.Setenv("UNDERDOC_ACCOUNTS_ROLES", "")
	t.Setenv("UNDERDOC_GATE_SERVICE_KEY_FILE", "")
	if _, err := readGateAPI(context.Background(), gate, log); err == nil {
		t.Error("approvals without a key file")
	}
}

// underdoc account add binds an account to a lasting ID before its first sign-in, without a mark; adopt makes one for
// every account the gate's account API shows UnderDoc, by its lasting ID, and keeps those bound already.
func TestAccountAddAndAdopt(t *testing.T) {
	url := pgtest.URL(t)
	t.Setenv("UNDERDOC_DATABASE_URL", url)
	code, out, errOut := runCommand(t, "account", "add", "4711", "lena", "Lena M.")
	if code != 0 || !strings.Contains(out, "without an administrator's mark") {
		t.Fatalf("add: %d %q %q", code, out, errOut)
	}
	if code, out, _ = runCommand(t, "account", "add", "4711", "other", "Other"); code != 0 || !strings.Contains(out, "bound to account") {
		t.Errorf("add again: %d %q", code, out)
	}
	if code, _, _ = runCommand(t, "account", "add", "lena", "lena", "Lena"); code == 0 {
		t.Error("an ID that is no number is taken")
	}
	g := gatetest.New(t, "underdoc", []string{"underdoc-user"}, "underdoc-admin")
	g.Add(gatetest.Account{Username: "mira", Name: "Mira", Managed: true})
	g.Add(gatetest.Account{Username: "boss", Managed: false})
	t.Setenv("UNDERDOC_GATE_URL", g.URL)
	t.Setenv("UNDERDOC_GATE_SERVICE_KEY_FILE", g.KeyFile())
	if code, out, errOut = runCommand(t, "account", "adopt", "--dry-run"); code != 0 || !strings.Contains(out, "would  mira") || !strings.Contains(out, "1 would be made") {
		t.Fatalf("dry run: %d %q %q", code, out, errOut)
	}
	if code, out, errOut = runCommand(t, "account", "adopt"); code != 0 || !strings.Contains(out, "made   mira") || strings.Contains(out, "boss") {
		t.Fatalf("adopt: %d %q %q", code, out, errOut)
	}
	if code, out, _ = runCommand(t, "account", "adopt"); code != 0 || !strings.Contains(out, "kept   mira") || !strings.Contains(out, "0 made") {
		t.Errorf("adopt again: %d %q", code, out)
	}
	code, out, _ = runCommand(t, "account", "list")
	if code != 0 || !strings.Contains(out, "4711") || !strings.Contains(out, "mira") || strings.Contains(out, "✓") {
		t.Errorf("the list: %q", out)
	}
}
