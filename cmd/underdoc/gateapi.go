// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/gateapi"
	"github.com/Ollornog/UnderDoc/internal/web"
)

// What UnderDoc may do in the gate itself (T-33, docs/accounts.md), from the environment. All of it is off unless named:
//
//	UNDERDOC_GATE_URL               the gate's own address (its base_url), which the calls go to directly
//	UNDERDOC_GATE_SERVICE_KEY_FILE  a file that holds the automation key of UnderDoc's service account in the gate, and
//	                                nothing else (never the key itself in the environment)
//	UNDERDOC_GATE_APP               the id of UnderDoc in the gate's apps ("underdoc")
//	UNDERDOC_ACCOUNTS_ROLES         the roles of the app UnderDoc gives a person it invites, separated by commas: its
//	                                allowlist, a part of the gate's; set, it turns the accounts on (none: off)
//	UNDERDOC_APPROVALS              "on": deleting for good waits for a signed approval of the gate (none: off)
//	UNDERDOC_DELEGATE_ROLE          the role of the app that makes a delegated administrator ("underdoc-delegate")

// readGateAPI reads what UnderDoc may do in the gate, and checks it against the gate where the accounts are on. nil when
// none of it is configured. A contradiction stops the start.
func readGateAPI(ctx context.Context, gate *account.Gate, log *slog.Logger) (*web.GateAPI, error) {
	roles := list(os.Getenv("UNDERDOC_ACCOUNTS_ROLES"))
	approvals := false
	switch v := os.Getenv("UNDERDOC_APPROVALS"); v {
	case "", "off":
	case "on":
		approvals = true
	default:
		return nil, fmt.Errorf("UNDERDOC_APPROVALS: %q is neither on nor off", v)
	}
	url, keyFile := os.Getenv("UNDERDOC_GATE_URL"), os.Getenv("UNDERDOC_GATE_SERVICE_KEY_FILE")
	if len(roles) == 0 && !approvals {
		if url != "" || keyFile != "" {
			log.Warn("UNDERDOC_GATE_URL or UNDERDOC_GATE_SERVICE_KEY_FILE is set, but neither UNDERDOC_ACCOUNTS_ROLES nor UNDERDOC_APPROVALS: nothing is asked of the gate")
		}
		return nil, nil
	}
	if url == "" || keyFile == "" {
		return nil, errors.New("the accounts and approvals of the gate need UNDERDOC_GATE_URL and UNDERDOC_GATE_SERVICE_KEY_FILE")
	}
	client, err := gateapi.New(gateapi.Config{URL: url, KeyFile: keyFile})
	if err != nil {
		return nil, fmt.Errorf("the gate's account API: %w", err)
	}
	app := env("UNDERDOC_GATE_APP", "underdoc")
	g := &web.GateAPI{Client: client, App: app, Approvals: approvals}
	if len(roles) == 0 {
		return g, nil
	}
	// Never a role that makes an administrator of any kind: a group admin would hand it out with an invitation.
	for _, role := range roles {
		if role == "admin" || role == gate.AdminRole() || role == gate.DelegateRole() || role == gate.SpacesRole() {
			return nil, fmt.Errorf("UNDERDOC_ACCOUNTS_ROLES: %q is a role of administrators or of who creates spaces, which an invitation never gives", role)
		}
	}
	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	p, err := client.Provisioning(checkCtx)
	if err != nil {
		return nil, fmt.Errorf("the gate's account API does not answer as expected: %w", err)
	}
	switch {
	case p.App != app:
		return nil, fmt.Errorf("the key of the service account belongs to the app %q of the gate, not to UNDERDOC_GATE_APP %q", p.App, app)
	case !p.Has(gateapi.ScopeRead) || !p.Has(gateapi.ScopeInvite) || !p.Has(gateapi.ScopeDisable):
		return nil, fmt.Errorf("the gate gives UnderDoc %s; it needs at least %s, %s and %s", strings.Join(p.Scopes, ", "),
			gateapi.ScopeRead, gateapi.ScopeInvite, gateapi.ScopeDisable)
	}
	for _, role := range roles {
		if !slices.Contains(p.Roles, role) {
			return nil, fmt.Errorf("UNDERDOC_ACCOUNTS_ROLES: the gate does not let UnderDoc give %q (its allowlist: %s)", role, strings.Join(p.Roles, ", "))
		}
		if role == p.ApproverRole {
			return nil, fmt.Errorf("UNDERDOC_ACCOUNTS_ROLES: %q is the gate's approver role, which is never on the allowlist", role)
		}
	}
	g.Roles = roles
	if p.Has(gateapi.ScopeReset) && p.ApproverRole != "" {
		g.ApproverRole = p.ApproverRole
		if p.ApproverRole != gate.AdminRole() {
			log.Warn("the gate's approver role for resetting access is not UnderDoc's administrator role (ADR-35)", slog.String("approver", p.ApproverRole))
		}
	}
	log.Info("accounts through the gate", slog.String("app", app), slog.String("roles", strings.Join(roles, ",")),
		slog.Bool("reset", g.ApproverRole != ""), slog.Bool("approvals", approvals))
	return g, nil
}
