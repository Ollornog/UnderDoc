// SPDX-License-Identifier: MIT

package share

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/samba"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// manager makes another manager over the database of the fixture, with a setup of its own (T-42).
func (f *fixture) manager(cfg Config, h *fakeHelper) *Manager {
	f.t.Helper()
	cfg.Dir, cfg.Helpers, cfg.Server = f.t.TempDir(), map[string]Helper{tenant.Main: h}, "files"
	m, err := New(f.db, f.store, f.m.vault, cfg, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	return m
}

var domainSetup = Config{Domain: map[string]bool{tenant.Main: true}}

// A tenant set up for a domain gives its companion the state of a domain: its users are domain accounts with a UID and
// no hash, and nobody makes an app password there.
func TestDomainStateHasNoPasswords(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	if _, err := f.db.RemoveAllSambaAccounts(ctx); err != nil {
		t.Fatal(err)
	}
	h := &fakeHelper{mode: samba.ModeDomain}
	m := f.manager(domainSetup, h)
	if err := m.CheckModes(ctx); err != nil {
		t.Fatalf("a tenant without Samba users: %v", err)
	}
	if _, err := f.db.SetSambaPassword(ctx, bob.ID, "bob", []byte("sealed"), 20000, 29999); !errors.Is(err, db.ErrSambaMode) {
		t.Fatalf("an app password with a domain: %v", err)
	}
	a, err := f.db.SetSambaDomainAccount(ctx, bob.ID, "bob", 20000, 29999)
	if err != nil || a.UID < 20000 {
		t.Fatalf("the domain account: %+v %v", a, err)
	}
	// A number from the gate is a name a domain account can have.
	if _, err := f.db.SetSambaDomainAccount(ctx, anna.ID, "4711", 20000, 29999); err != nil {
		t.Fatalf("a number: %v", err)
	}
	if err := m.Sync(context.Background(), tenant.Main); err != nil {
		t.Fatal(err)
	}
	st := h.state
	if st.Mode != samba.ModeDomain || len(st.Users) != 2 {
		t.Fatalf("state: %+v", st)
	}
	for _, u := range st.Users {
		if u.NTHash != "" || u.UID < 20000 {
			t.Errorf("user: %#v %q", u, u.NTHash)
		}
	}
	if err := (samba.Request{Op: samba.OpApply, State: &st}).Check(m.Dir(tenant.Main), 20000, 29999); err != nil {
		t.Errorf("the state does not pass the rules of the channel: %v", err)
	}
	if err := m.CheckModes(ctx); err != nil {
		t.Errorf("only domain accounts: %v", err)
	}
	// The same tenant set up for app passwords refuses to start, and a pass leaves the domain accounts out.
	local := f.manager(Config{}, &fakeHelper{})
	if err := local.CheckModes(ctx); err == nil || !strings.Contains(err.Error(), "never mixed") {
		t.Errorf("domain accounts where app passwords are set up: %v", err)
	}
}

// A tenant set up for a domain that still holds app passwords refuses to start, and a pass never hands them to the
// companion (T-42): app passwords and a domain are never mixed.
func TestDomainRefusesAppPasswordsAtTheStart(t *testing.T) {
	f := setup(t) // Anna and Bob have app passwords
	ctx := inMain()
	h := &fakeHelper{mode: samba.ModeDomain}
	m := f.manager(domainSetup, h)
	err := m.CheckModes(ctx)
	if err == nil || !strings.Contains(err.Error(), "2 of its accounts have app passwords") || !strings.Contains(err.Error(), "samba-users --tenant main remove") {
		t.Fatalf("app passwords where a domain is set up: %v", err)
	}
	if err := m.Sync(context.Background(), tenant.Main); err != nil {
		t.Fatal(err)
	}
	if h.state.Mode != samba.ModeDomain || len(h.state.Users) != 0 {
		t.Fatalf("the companion got %+v", h.state)
	}
	// The migration: the operator removes them, and the start goes on.
	if n, err := f.db.RemoveAllSambaAccounts(ctx); err != nil || n != 2 {
		t.Fatalf("removed %d: %v", n, err)
	}
	if err := m.CheckModes(ctx); err != nil {
		t.Fatalf("after the migration: %v", err)
	}
}

// A companion set up the other way than UnderDoc gets nothing: no users, no ACLs; the pass says so (T-42).
func TestACompanionOfTheOtherModeGetsNothing(t *testing.T) {
	f := setup(t)
	// UnderDoc with app passwords, the companion a member of a domain.
	f.h.mode = samba.ModeDomain
	if err := f.m.Sync(context.Background(), tenant.Main); err == nil || !strings.Contains(err.Error(), "signs in with domain") {
		t.Fatalf("a companion of a domain: %v", err)
	}
	if f.h.calls[samba.OpApply] != 0 || f.h.calls[samba.OpACL] != 0 {
		t.Fatalf("the companion of the other mode got %v", f.h.calls)
	}
	// UnderDoc set up for a domain, the companion with app passwords (or from before T-42, which names no mode).
	ctx := inMain()
	if _, err := f.db.RemoveAllSambaAccounts(ctx); err != nil {
		t.Fatal(err)
	}
	h := &fakeHelper{}
	m := f.manager(domainSetup, h)
	if err := m.Sync(context.Background(), tenant.Main); err == nil || !strings.Contains(err.Error(), "signs in with local") {
		t.Fatalf("a companion with app passwords: %v", err)
	}
	if h.calls[samba.OpApply] != 0 {
		t.Fatalf("the companion of the other mode got %v", h.calls)
	}
}

// The name of the domain account: the login in lower case, or the gate's Remote-Id; a name of the system never maps.
func TestDomainNames(t *testing.T) {
	f := setup(t)
	byLogin := f.manager(domainSetup, &fakeHelper{mode: samba.ModeDomain})
	byID := f.manager(Config{Domain: domainSetup.Domain, DomainNames: DomainByRemoteID}, &fakeHelper{mode: samba.ModeDomain})
	for _, c := range []struct {
		m             *Manager
		login, remote string
		want          string
		ok            bool
	}{
		{byLogin, "Anna.Berg", "1007", "anna.berg", true},
		{byLogin, "root", "1", "root", false},
		{byLogin, "Administrator", "1", "administrator", false},
		{byLogin, "anna berg", "1", "anna berg", false},
		{byID, "anna", "1007", "1007", true},
		{byID, "anna", "", "", false},
	} {
		if got, ok := c.m.DomainName(c.login, c.remote); got != c.want || ok != c.ok {
			t.Errorf("%s/%s: %q %v", c.login, c.remote, got, ok)
		}
	}
	if _, err := New(f.db, f.store, f.m.vault, Config{Dir: t.TempDir(), DomainNames: "mail"}, nil); err == nil {
		t.Error("a way of naming that does not exist")
	}
	if _, err := New(f.db, f.store, f.m.vault, Config{Dir: t.TempDir(), Domain: map[string]bool{"acme": true}}, nil); err == nil {
		t.Error("a domain for a tenant without a companion")
	}
}
