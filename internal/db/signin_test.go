// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
)

// TestAnInstanceBehindAGateKeepsIt: an instance that has accounts when sign-in arrives came in through its gate, so
// Main keeps the gate as its only way in; a new instance starts with passkeys (ADR-23).
func TestAnInstanceBehindAGateKeepsIt(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name     string
		accounts bool
		want     []string
	}{{"behind a gate", true, []string{MethodGate}}, {"new", false, []string{MethodPasskey}}} {
		url := pgtest.URL(t)
		conn, err := pgx.Connect(ctx, url)
		if err != nil {
			t.Fatal(err)
		}
		if err := migrate(ctx, conn, 14); err != nil {
			t.Fatal(err)
		}
		if c.accounts {
			if _, err := conn.Exec(ctx, "SELECT set_config('underdoc.tenant', 'main', false); INSERT INTO accounts (id, login, name) VALUES ('u-1', 'anna', 'Anna')"); err != nil {
				t.Fatal(err)
			}
		}
		_ = conn.Close(ctx)
		db, err := Open(ctx, url)
		if err != nil {
			t.Fatal(err)
		}
		s, err := db.SignIn(inMain())
		if err != nil || !slices.Equal(s.Methods, c.want) || s.Registration {
			t.Errorf("%s: %+v %v", c.name, s, err)
		}
		if c.accounts {
			a, err := db.AccountByID(inMain(), "u-1")
			if err != nil || a.Source != SourceGate || a.Admin {
				t.Errorf("the account of the gate: %+v %v", a, err)
			}
		}
		db.Close()
	}
}

func TestSignInSettings(t *testing.T) {
	db := open(t)
	ctx := inMain()
	for _, bad := range []SignIn{{}, {Methods: []string{"magic"}}, {Methods: []string{MethodOIDC}, OIDCIssuer: "http://id.example", OIDCClientID: "x"},
		{Methods: []string{MethodOIDC}, OIDCIssuer: "https://id.example"}} {
		if err := db.SetSignIn(ctx, bad); err == nil {
			t.Errorf("%+v saved", bad)
		}
	}
	if err := db.SetSignIn(ctx, SignIn{Methods: []string{MethodGate, MethodPasskey, MethodPasskey}, Registration: true}); err != nil {
		t.Fatal(err)
	}
	s, _ := db.SignIn(ctx)
	if !slices.Equal(s.Methods, []string{MethodPasskey, MethodGate}) || !s.Registration || s.OIDCGroupsClaim != "groups" {
		t.Errorf("%+v", s)
	}
	if _, err := db.CreateTenant(ctx, "acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	other, _ := db.SignIn(tenant.With(context.Background(), "acme"))
	if !slices.Equal(other.Methods, []string{MethodPasskey}) || other.Registration {
		t.Errorf("another tenant starts with its own defaults: %+v", other)
	}
}

func TestInvitationsAndLocalAccounts(t *testing.T) {
	db := open(t)
	ctx := inMain()
	invite := func(login string, admin bool, expires time.Duration) []byte {
		hash := make([]byte, 32)
		copy(hash, login)
		if err := db.CreateInvite(ctx, hash, Invite{Login: login, Name: "Name of " + login, Admin: admin, Expires: time.Now().Add(expires)}); err != nil {
			t.Fatal(err)
		}
		return hash
	}
	for login, want := range map[string]error{"anna": ErrLoginTaken, "Bad Login": ErrLogin, "": ErrLogin, "-x": ErrLogin} {
		if err := db.CreateInvite(ctx, make([]byte, 32), Invite{Login: login, Name: "x", Expires: time.Now().Add(time.Hour)}); !errors.Is(err, want) {
			t.Errorf("%q: %v", login, err)
		}
	}
	carl := invite("carl", true, time.Hour)
	if err := db.CreateInvite(ctx, make([]byte, 32), Invite{Login: "CARL", Name: "x", Expires: time.Now().Add(time.Hour)}); !errors.Is(err, ErrLoginTaken) {
		t.Errorf("a login with an open invitation: %v", err)
	}
	if _, err := db.AcceptInvite(ctx, carl, NewLocal{}); !errors.Is(err, ErrNoWayIn) {
		t.Errorf("no way in: %v", err)
	}
	a, err := db.AcceptInvite(ctx, carl, NewLocal{PasswordHash: "$argon2id$x"})
	if err != nil || a.Login != "carl" || !a.Admin || a.Source != SourceLocal || !validID(a.ID) {
		t.Fatalf("%+v %v", a, err)
	}
	if _, err := db.AcceptInvite(ctx, carl, NewLocal{PasswordHash: "x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("used twice: %v", err)
	}
	list, _ := db.Collections(ctx, a.ID)
	if len(list) != 1 || list[0].Title != MyNotes {
		t.Errorf("the private notes: %+v", list)
	}
	if found, err := db.LocalAccount(ctx, "CARL"); err != nil || found.ID != a.ID {
		t.Errorf("by login: %v", err)
	}
	if _, err := db.AcceptInvite(ctx, invite("late", false, -time.Minute), NewLocal{PasswordHash: "x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("an expired invitation: %v", err)
	}
	// Nobody from outside comes in as a local account, by its login or its ID.
	for _, user := range []account.User{{ID: "carl", Login: "carl", Name: "Carl"}, {ID: a.ID, Login: "other", Name: "x"}} {
		if err := db.SeeExternal(ctx, user, SourceOIDC); !errors.Is(err, ErrSource) {
			t.Errorf("%+v: %v", user, err)
		}
	}
	// Gate and OIDC share their accounts.
	if err := db.SeeExternal(ctx, account.User{ID: "u-anna", Login: "anna", Name: "Anna"}, SourceOIDC); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.AccountByID(ctx, "u-anna"); got.Source != SourceOIDC {
		t.Errorf("%+v", got)
	}
	// A step of TOTP is used once, also by two requests at the same moment: only one of them advances it.
	if ok, err := db.UseTOTPStep(ctx, a.ID, 100); !ok || err != nil {
		t.Errorf("a new step: %v %v", ok, err)
	}
	for _, step := range []int64{100, 99} {
		if ok, _ := db.UseTOTPStep(ctx, a.ID, step); ok {
			t.Errorf("step %d after 100", step)
		}
	}
	if n, err := db.RecentRegistrations(ctx, time.Hour); err != nil || n != 2 {
		t.Errorf("registrations: %d %v", n, err)
	}
}

func TestSessionsAndCeremonies(t *testing.T) {
	db := open(t)
	ctx := inMain()
	hash := make([]byte, 32)
	hash[0] = 1
	if err := db.CreateSession(ctx, hash, anna.ID, MethodPasskey, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, method, err := db.Session(ctx, hash, time.Hour); err != nil || got != anna.ID || method != MethodPasskey {
		t.Errorf("%q %q %v", got, method, err)
	}
	if _, _, err := db.Session(tenant.With(context.Background(), "hoster"), hash, time.Hour); !errors.Is(err, ErrNotFound) {
		t.Errorf("a session of Main in another tenant: %v", err)
	}
	if _, err := db.pool.Exec(ctx, "UPDATE sessions SET seen = now() - interval '2 hours'"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Session(ctx, hash, time.Hour); !errors.Is(err, ErrNotFound) {
		t.Errorf("an idle session: %v", err)
	}
	expired := make([]byte, 32)
	expired[0] = 2
	if err := db.CreateSession(ctx, expired, anna.ID, MethodPasskey, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.Session(ctx, expired, time.Hour); !errors.Is(err, ErrNotFound) {
		t.Errorf("an expired session: %v", err)
	}
	if err := db.PutCeremony(ctx, hash, "oidc", "", []byte(`{"state":"x"}`), time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.TakeCeremony(ctx, hash, "passkey_add"); !errors.Is(err, ErrNotFound) {
		t.Errorf("another kind: %v", err)
	}
	if _, data, err := db.TakeCeremony(ctx, hash, "oidc"); err != nil || string(data) != `{"state": "x"}` {
		t.Errorf("%s %v", data, err)
	}
	if _, _, err := db.TakeCeremony(ctx, hash, "oidc"); !errors.Is(err, ErrNotFound) {
		t.Errorf("twice: %v", err)
	}
	if err := db.PutCeremony(ctx, expired, "oidc", "", []byte(`{}`), time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.TakeCeremony(ctx, expired, "oidc"); !errors.Is(err, ErrNotFound) {
		t.Errorf("an expired ceremony: %v", err)
	}
	if err := db.LogSignIn(ctx, SignInEvent{Login: "anna", Method: MethodPasskey, OK: true}); err != nil {
		t.Fatal(err)
	}
	if err := db.ExpireSignIns(ctx); err != nil {
		t.Fatal(err)
	}
	var sessions int
	if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM sessions").Scan(&sessions); err != nil || sessions != 1 {
		t.Errorf("only the expired session goes: %d %v", sessions, err)
	}
	if events, _ := db.SignInLog(ctx, 10); len(events) != 1 {
		t.Errorf("the log keeps a fresh entry: %+v", events)
	}
}

// TestSecretsStayWithTheirTenant: a sealed secret opens only in its tenant and under its name, also if its row is
// copied to another tenant.
func TestSecretsStayWithTheirTenant(t *testing.T) {
	db := open(t)
	v, err := vault.New("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	if err != nil {
		t.Fatal(err)
	}
	store := vault.Store{Vault: v, Rows: db}
	main := inMain()
	if _, err := db.CreateTenant(main, "acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	acme := tenant.With(context.Background(), "acme")
	if err := store.Put(main, "mail.password", vault.Secret("hunter2-but-longer")); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Use(main, "mail.password"); err != nil || string(got) != "hunter2-but-longer" {
		t.Errorf("%v", err)
	}
	if store.Has(acme, "mail.password") {
		t.Error("acme sees Main's secret")
	}
	sealed, _ := db.SealedSecret(main, "mail.password")
	if err := db.PutSealedSecret(acme, "mail.password", sealed); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Use(acme, "mail.password"); !errors.Is(err, vault.ErrSealed) {
		t.Errorf("a copied row opens in another tenant: %v", err)
	}
	if err := db.PutSealedSecret(main, "other", sealed); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Use(main, "other"); !errors.Is(err, vault.ErrSealed) {
		t.Errorf("a copied row opens under another name: %v", err)
	}
	if err := store.Put(context.Background(), "x", vault.Secret("y")); !errors.Is(err, vault.ErrNoTenant) {
		t.Errorf("without a tenant: %v", err)
	}
	if err := db.PutSealedSecret(main, "Bad Name", sealed); err == nil {
		t.Error("a bad name")
	}
	if err := store.Delete(main, "mail.password"); err != nil || store.Has(main, "mail.password") {
		t.Errorf("delete: %v", err)
	}
}

func validID(s string) bool { return len(s) == 10 }
