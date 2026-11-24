// SPDX-License-Identifier: MIT

package gateapi_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/gateapi"
	"github.com/Ollornog/UnderDoc/internal/gateapi/gatetest"
)

func client(t *testing.T, g *gatetest.Gate) *gateapi.Client {
	t.Helper()
	c, err := gateapi.New(gateapi.Config{URL: g.URL, KeyFile: g.KeyFile()})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// The configuration: https, or http to the own machine only, and a file that holds one key.
func TestTheClientTakesOnlyASafeConfiguration(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "key")
	if err := os.WriteFile(key, []byte("tsk_made_up\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	two := filepath.Join(dir, "two")
	if err := os.WriteFile(two, []byte("tsk_one tsk_two"), 0o600); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, cfg := range map[string]gateapi.Config{
		"plain http to another host": {URL: "http://auth.example.com", KeyFile: key},
		"another scheme":             {URL: "ftp://auth.example.com", KeyFile: key},
		"a user in the address":      {URL: "https://me@auth.example.com", KeyFile: key},
		"a query":                    {URL: "https://auth.example.com/?x=1", KeyFile: key},
		"no key file":                {URL: "https://auth.example.com"},
		"a missing file":             {URL: "https://auth.example.com", KeyFile: filepath.Join(dir, "none")},
		"two keys":                   {URL: "https://auth.example.com", KeyFile: two},
		"an empty file":              {URL: "https://auth.example.com", KeyFile: empty},
	} {
		if _, err := gateapi.New(cfg); err == nil {
			t.Errorf("%s is taken", name)
		} else if strings.Contains(err.Error(), "tsk_") {
			t.Errorf("%s: the error names the key: %v", name, err)
		}
	}
	for _, url := range []string{"https://auth.example.com/", "http://127.0.0.1:8000", "http://localhost:8000", "http://[::1]:8000"} {
		c, err := gateapi.New(gateapi.Config{URL: url, KeyFile: key})
		if err != nil {
			t.Errorf("%s: %v", url, err)
		} else if strings.HasSuffix(c.URL(), "/") {
			t.Errorf("%s keeps its slash: %s", url, c.URL())
		}
	}
}

// Invite, read, lock, unlock and a new link, with the On-Behalf-Of on every write and none on a read.
func TestTheClientSpeaksTheAccountAPI(t *testing.T) {
	g := gatetest.New(t, "underdoc", []string{"underdoc-user"}, "underdoc-admin")
	c := client(t, g)
	ctx := context.Background()
	p, err := c.Provisioning(ctx)
	if err != nil || p.App != "underdoc" || !p.Has(gateapi.ScopeInvite) || p.ApproverRole != "underdoc-admin" {
		t.Fatalf("provisioning: %+v %v", p, err)
	}
	made, err := c.Create(ctx, "17", gateapi.NewAccount{Username: "lena", DisplayName: "Lena M.", Roles: []string{"underdoc-user"}})
	if err != nil || !gateapi.ID(made.Account.ID) || made.Link.URL == "" || made.Account.SignedIn() {
		t.Fatalf("create: %+v %v", made, err)
	}
	if _, err := c.Create(ctx, "17", gateapi.NewAccount{Username: "otto", Roles: []string{"underdoc-admin"}}); !errors.Is(err, gateapi.ErrRoleNotAllowed) {
		t.Errorf("a role off the allowlist: %v", err)
	}
	if _, err := c.Create(ctx, "17", gateapi.NewAccount{Username: "lena", Roles: []string{"underdoc-user"}}); !errors.Is(err, gateapi.ErrNameTaken) {
		t.Errorf("a login taken: %v", err)
	}
	if a, err := c.Disable(ctx, "17", made.Account.ID); err != nil || a.State != gateapi.StateDisabled {
		t.Errorf("disable: %+v %v", a, err)
	}
	if _, err := c.SigninLink(ctx, "17", made.Account.ID); !errors.Is(err, gateapi.ErrDisabled) {
		t.Errorf("a link for a locked account: %v", err)
	}
	if a, err := c.Enable(ctx, "-", made.Account.ID); err != nil || a.State != gateapi.StateActive {
		t.Errorf("enable as a job: %+v %v", a, err)
	}
	if l, err := c.SigninLink(ctx, "17", made.Account.ID); err != nil || l.URL == "" {
		t.Errorf("a new link: %+v %v", l, err)
	}
	g.SignIn(made.Account.ID)
	if _, err := c.SigninLink(ctx, "17", made.Account.ID); !errors.Is(err, gateapi.ErrSignedIn) {
		t.Errorf("a link after the first sign-in: %v", err)
	}
	foreign := g.Add(gatetest.Account{Username: "boss", Roles: []string{"staff"}})
	if _, err := c.Get(ctx, foreign); !errors.Is(err, gateapi.ErrNotFound) {
		t.Errorf("an account UnderDoc does not manage: %v", err)
	}
	list, next, err := c.List(ctx, "")
	if err != nil || next != "" || len(list) != 1 || list[0].ID != made.Account.ID {
		t.Errorf("the list: %+v %q %v", list, next, err)
	}
	// Every write names the person, every read none.
	for _, call := range g.Calls() {
		write := call.Method != http.MethodGet
		if write && call.Actor == "" || !write && call.Actor != "" {
			t.Errorf("%s %s with On-Behalf-Of %q", call.Method, call.Path, call.Actor)
		}
	}
	// A write without a person's lasting ID never leaves UnderDoc.
	for _, actor := range []string{"", "anna", "1 2", "17\n"} {
		before := len(g.Calls())
		if _, err := c.Disable(ctx, actor, made.Account.ID); err == nil {
			t.Errorf("On-Behalf-Of %q is sent", actor)
		}
		if len(g.Calls()) != before {
			t.Errorf("On-Behalf-Of %q reached the gate", actor)
		}
	}
	// An ID that is no number is no account of the gate, and is not asked for.
	if _, err := c.Get(ctx, "../provisioning"); !errors.Is(err, gateapi.ErrNotFound) {
		t.Errorf("a path as an ID: %v", err)
	}
}

// A refusal of the gate comes as its status and code, never its text, and the key is in no error.
func TestRefusalsNameNoSecret(t *testing.T) {
	g := gatetest.New(t, "underdoc", []string{"underdoc-user"}, "underdoc-admin")
	c := client(t, g)
	g.Refuse(http.StatusTooManyRequests, "limit")
	_, err := c.Provisioning(context.Background())
	var e *gateapi.Error
	if !errors.As(err, &e) || e.Status != http.StatusTooManyRequests || !errors.Is(err, gateapi.ErrLimit) {
		t.Errorf("a limit: %v", err)
	}
	if strings.Contains(err.Error(), "made up by gatetest") || strings.Contains(err.Error(), g.Key()) {
		t.Errorf("the error carries the gate's text or the key: %v", err)
	}
	g.Refuse(0, "")
	// Another key: the gate says 401, the error does not name the key.
	other := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(other, []byte("tsk_wrong_key"), 0o600); err != nil {
		t.Fatal(err)
	}
	wrong, _ := gateapi.New(gateapi.Config{URL: g.URL, KeyFile: other})
	if _, err := wrong.Provisioning(context.Background()); !errors.Is(err, &gateapi.Error{Code: "unauthenticated"}) || strings.Contains(err.Error(), "tsk_") {
		t.Errorf("a wrong key: %v", err)
	}
	// A gate that is not there: no address, no key.
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	gone, _ := gateapi.New(gateapi.Config{URL: dead.URL, KeyFile: g.KeyFile()})
	if _, err := gone.Provisioning(context.Background()); err == nil || strings.Contains(err.Error(), "tsk_") || strings.Contains(err.Error(), "provisioning") {
		t.Errorf("a gate not reached: %v", err)
	}
	// A redirect is not followed: the key would go along.
	moved := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.example.com/", http.StatusFound)
	}))
	defer moved.Close()
	redirected, _ := gateapi.New(gateapi.Config{URL: moved.URL, KeyFile: g.KeyFile()})
	if _, err := redirected.Provisioning(context.Background()); !errors.Is(err, &gateapi.Error{Status: http.StatusFound}) {
		t.Errorf("a redirect: %v", err)
	}
	// An answer that is no account API.
	odd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) }))
	defer odd.Close()
	html, _ := gateapi.New(gateapi.Config{URL: odd.URL, KeyFile: g.KeyFile()})
	if _, err := html.Provisioning(context.Background()); err == nil {
		t.Error("an HTML page is taken as an answer")
	}
}

// Approvals: asked with the person who confirms, read once with what comes after the confirmation.
func TestTheClientAsksForApprovals(t *testing.T) {
	g := gatetest.New(t, "underdoc", []string{"underdoc-user"}, "underdoc-admin")
	c := client(t, g)
	ctx := context.Background()
	made, _ := c.Create(ctx, "17", gateapi.NewAccount{Username: "lena", Roles: []string{"underdoc-user"}})
	if _, err := c.Ask(ctx, gateapi.Ask{Type: gateapi.ResetAccess, Subject: "anna", Fields: map[string]any{"account": made.Account.ID}}); err == nil {
		t.Error("an approval without a person's lasting ID is asked")
	}
	asked, err := c.Ask(ctx, gateapi.Ask{Type: gateapi.ResetAccess, Subject: "17", Fields: map[string]any{"account": made.Account.ID}})
	if err != nil || !gateapi.ApprovalID(asked.ID) || asked.MatchCode == "" {
		t.Fatalf("ask: %+v %v", asked, err)
	}
	if a, err := c.Approval(ctx, asked.ID, 0); err != nil || a.State != gateapi.ApprovalPending || a.Link.URL != "" {
		t.Errorf("before the confirmation: %+v %v", a, err)
	}
	g.Confirm(asked.ID, "17")
	if a, err := c.Approval(ctx, asked.ID, 1); err != nil || a.State != gateapi.ApprovalConsumed || a.Result != "ok" || a.Link.URL == "" {
		t.Errorf("after the confirmation: %+v %v", a, err)
	}
	if a, err := c.Approval(ctx, asked.ID, 0); err != nil || a.Link.URL != "" {
		t.Errorf("the link a second time: %+v %v", a, err)
	}
	if _, err := c.Approval(ctx, "../users", 0); !errors.Is(err, gateapi.ErrNotFound) {
		t.Errorf("a path as an approval: %v", err)
	}
}
