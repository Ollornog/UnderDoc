// SPDX-License-Identifier: MIT

package account

import (
	"crypto/ed25519"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"testing"

	tinysesam "github.com/Ollornog/TinySesam/clients/go"

	"github.com/Ollornog/UnderDoc/internal/account/gatetest"
)

func newGate(t *testing.T, cfg GateConfig) *Gate {
	t.Helper()
	g, err := NewGate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// request carries the headers a proxy of the gate sets, the way it reaches UnderDoc: from the proxy's address.
func request(headers map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "127.0.0.1:40000"
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func person(extra map[string]string) map[string]string {
	h := map[string]string{"Remote-User": "anna", "Remote-Id": "7", "Remote-Name": "Anna Berg", "Remote-Email": "anna@example.com",
		"Remote-Groups": "editors, ops,editors", "Remote-Auth": "session", "Remote-Kind": "user"}
	for k, v := range extra {
		if v == "-" {
			delete(h, k)
			continue
		}
		h[k] = v
	}
	return h
}

func TestGateReadsThePerson(t *testing.T) {
	g := newGate(t, GateConfig{})
	user, err := g.Identify(request(person(nil)))
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != "7" || user.RemoteID != "7" || user.Login != "anna" || user.Name != "Anna Berg" || user.Email != "anna@example.com" ||
		!slices.Equal(user.Groups, []string{"editors", "ops"}) || user.AuthKind != AuthSession || user.Kind != KindPerson ||
		user.GateKind != "user" || user.Admin || user.Agent() {
		t.Fatalf("%+v", user)
	}
	if user, _ := g.Identify(request(person(map[string]string{"Remote-Name": "-"}))); user.Name != "anna" {
		t.Errorf("without a name the login stands in: %+v", user)
	}
	if _, err := g.Identify(request(nil)); !errors.Is(err, tinysesam.ErrUnauthenticated) {
		t.Errorf("no headers: %v", err)
	}
}

// A person is bound by the lasting ID of the gate alone (ADR-35): without one nobody signs in, and the ID is a number.
func TestGateNeedsTheLastingID(t *testing.T) {
	g := newGate(t, GateConfig{})
	if _, err := g.Identify(request(person(map[string]string{"Remote-Id": "-"}))); !errors.Is(err, ErrNoID) {
		t.Errorf("without Remote-Id: %v", err)
	}
	if _, err := g.Identify(request(person(map[string]string{"Remote-Id": "u-7"}))); !errors.Is(err, tinysesam.ErrHeader) {
		t.Errorf("an ID that is no number: %v", err)
	}
}

// The way of signing in comes only from the client's result: exactly "session" is a session, anything else an agent.
func TestGateAuthKind(t *testing.T) {
	g := newGate(t, GateConfig{})
	for value, agent := range map[string]bool{"session": false, "oauth": true, "apikey": true, "app-password": true, "Session": true,
		" session": true, "-": true, "magic": true} {
		user, err := g.Identify(request(person(map[string]string{"Remote-Auth": value})))
		if err != nil {
			t.Fatalf("%q: %v", value, err)
		}
		if user.Agent() != agent {
			t.Errorf("Remote-Auth %q: agent %v, want %v (%q)", value, user.Agent(), agent, user.AuthKind)
		}
	}
	// A browser that adds its own Remote-Auth to the one of the proxy sends two: refused, never the better one.
	r := request(person(map[string]string{"Remote-Auth": "oauth"}))
	r.Header.Add("Remote-Auth", "session")
	if _, err := g.Identify(r); !errors.Is(err, tinysesam.ErrHeader) {
		t.Errorf("two Remote-Auth: %v", err)
	}
}

// Remote-Kind is required (ADR-32, B-3): the gate's proxies set it on every request and remove what the browser sent.
// A missing, empty or unknown kind is refused. KindLegacy (UNDERDOC_GATE_KIND=legacy) is the explicit way back for a
// proxy that does not set it yet: a missing kind is filled in as user and kept as unknown; an unknown one stays refused.
func TestGateKind(t *testing.T) {
	g := newGate(t, GateConfig{})
	if !g.KindRequired() {
		t.Error("required by default")
	}
	for name, kind := range map[string]string{"missing": "-", "empty": "", "unknown": "robot", "in capitals": "USER", "with a space": "user "} {
		if _, err := g.Identify(request(person(map[string]string{"Remote-Kind": kind}))); !errors.Is(err, tinysesam.ErrHeader) {
			t.Errorf("a %s Remote-Kind is refused: %v", name, err)
		}
	}
	if user, err := g.Identify(request(person(nil))); err != nil || user.GateKind != "user" || user.Kind != KindPerson || user.Agent() {
		t.Errorf("a person in a session is one: %+v %v", user, err)
	}
	if _, err := g.Identify(request(person(map[string]string{"Remote-Kind": "service", "Remote-Auth": "session"}))); err == nil {
		t.Error("a service account has no session")
	}

	legacy := newGate(t, GateConfig{KindLegacy: true})
	if legacy.KindRequired() {
		t.Error("the way back is the setting")
	}
	user, err := legacy.Identify(request(person(map[string]string{"Remote-Kind": "-", "Remote-Auth": "oauth"})))
	if err != nil || user.GateKind != "" || user.Kind != KindPerson || !user.Agent() {
		t.Fatalf("the way back, without Remote-Kind: %+v %v", user, err)
	}
	if user, err := legacy.Identify(request(person(map[string]string{"Remote-Kind": ""}))); err != nil || user.GateKind != "" || user.Agent() {
		t.Errorf("the way back, an empty Remote-Kind is none: %+v %v", user, err)
	}
	if _, err := legacy.Identify(request(person(map[string]string{"Remote-Kind": "robot"}))); !errors.Is(err, tinysesam.ErrHeader) {
		t.Errorf("the way back, an unknown kind: %v", err)
	}
	if _, err := NewGate(GateConfig{Mode: GateToken, KindLegacy: true, Issuer: gatetest.Issuer, Audience: gatetest.Audience,
		Keys: []string{gatetest.PublicKey()}}); err == nil {
		t.Error("the way back in token mode")
	}
}

// A service account comes in only with a lasting ID an administrator allowed, and is then always one: an agent (B-3).
// With the way back or without, refusing one can only make a request weaker; an allowed ID is a service account whatever
// kind a proxy passes on.
func TestGateServiceAccounts(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		required := !legacy
		g := newGate(t, GateConfig{KindLegacy: legacy, ServiceAccounts: []string{"42"}})
		if _, err := g.Identify(request(person(map[string]string{"Remote-Kind": "service", "Remote-Auth": "apikey"}))); !errors.Is(err, ErrService) {
			t.Errorf("required=%v: a service account nobody allowed: %v", required, err)
		}
		user, err := g.Identify(request(person(map[string]string{"Remote-Id": "42", "Remote-Kind": "service", "Remote-Auth": "apikey"})))
		if err != nil || user.Kind != KindService || !user.Agent() || user.GateKind != "service" {
			t.Errorf("required=%v: an allowed service account is one, and an agent: %+v %v", required, user, err)
		}
		user, err = g.Identify(request(person(map[string]string{"Remote-Id": "42", "Remote-Kind": "user", "Remote-Auth": "session",
			"Remote-Groups": "underdoc-admin,underdoc-spaces"})))
		if err != nil || user.Kind != KindService || !user.Agent() {
			t.Errorf("required=%v: an allowed ID passed on as a person in a session is still a service account: %+v %v", required, user, err)
		}
		// The roles of the app make no administrator and no maker of spaces of a service account (B-4).
		if user.Admin || user.Spaces {
			t.Errorf("required=%v: a service account with the roles of the app administers or makes spaces: %+v", required, user)
		}
	}
	// The way back takes a request without Remote-Kind as a person's: a service account nobody allowed comes in as a
	// person there, as the warning at the start says; an allowed one stays a service account (B-4).
	legacy := newGate(t, GateConfig{KindLegacy: true, ServiceAccounts: []string{"42"}})
	unlisted := person(map[string]string{"Remote-Id": "43", "Remote-Auth": "apikey"})
	delete(unlisted, "Remote-Kind")
	if user, err := legacy.Identify(request(unlisted)); err != nil || user.Kind != KindPerson || user.GateKind != "" {
		t.Errorf("the way back, an unlisted ID without Remote-Kind: %+v %v", user, err)
	}
	listed := person(map[string]string{"Remote-Id": "42", "Remote-Auth": "apikey", "Remote-Groups": "underdoc-admin"})
	delete(listed, "Remote-Kind")
	if user, err := legacy.Identify(request(listed)); err != nil || user.Kind != KindService || user.Admin {
		t.Errorf("the way back, an allowed ID without Remote-Kind: %+v %v", user, err)
	}
	for _, bad := range []string{"svc", "4 2", "-1"} {
		if _, err := NewGate(GateConfig{ServiceAccounts: []string{bad}}); err == nil {
			t.Errorf("a service account named %q", bad)
		}
	}
}

// The role of the app to create spaces is read like the administrator's, compared exactly (ADR-35, B-3).
func TestGateSpacesRole(t *testing.T) {
	g := newGate(t, GateConfig{})
	if user, _ := g.Identify(request(person(map[string]string{"Remote-Groups": "underdoc-spaces"}))); !user.Spaces || user.Admin {
		t.Errorf("the default role: %+v", user)
	}
	if user, _ := g.Identify(request(person(map[string]string{"Remote-Groups": "Underdoc-Spaces, underdoc-spaces-x"}))); user.Spaces {
		t.Errorf("compared exactly: %+v", user)
	}
	own := newGate(t, GateConfig{SpacesRole: "wiki-spaces"})
	if user, _ := own.Identify(request(person(map[string]string{"Remote-Groups": "wiki-spaces"}))); !user.Spaces || own.SpacesRole() != "wiki-spaces" {
		t.Errorf("a role of its own: %+v", user)
	}
	if _, err := NewGate(GateConfig{SpacesRole: "admin"}); err == nil {
		t.Error("the gate's own administrator creates spaces")
	}
}

// Only the role of the app makes an administrator, compared exactly; the gate's own administrator never does (ADR-35).
func TestGateAdminRole(t *testing.T) {
	if _, err := NewGate(GateConfig{AdminRole: "admin"}); err == nil {
		t.Error("the gate's own administrator as the role of the app")
	}
	g := newGate(t, GateConfig{})
	for groups, admin := range map[string]bool{"underdoc-admin": true, "ops, underdoc-admin": true, "admin": false, "Underdoc-Admin": false,
		"underdoc-admins": false, "underdoc-admin ": true, "": false} {
		user, err := g.Identify(request(person(map[string]string{"Remote-Groups": groups})))
		if err != nil {
			t.Fatal(err)
		}
		if user.Admin != admin {
			t.Errorf("groups %q: admin %v", groups, user.Admin)
		}
	}
	other := newGate(t, GateConfig{AdminRole: "wiki-admins"})
	if user, _ := other.Identify(request(person(map[string]string{"Remote-Groups": "wiki-admins"}))); !user.Admin {
		t.Error("a role of its own")
	}
}

// Only the proxy of the gate may set the headers: from any other address they are refused, whatever they say.
func TestGateTrustsTheProxyAlone(t *testing.T) {
	g := newGate(t, GateConfig{Proxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}})
	if _, err := g.Identify(request(person(nil))); err != nil {
		t.Fatal(err)
	}
	past := request(person(map[string]string{"Remote-Groups": "underdoc-admin"}))
	past.RemoteAddr = "198.51.100.9:5555"
	if _, err := g.Identify(past); !errors.Is(err, tinysesam.ErrUntrustedPeer) {
		t.Errorf("headers from past the gate: %v", err)
	}
}

// The gate may rename its headers; then the default names count for nothing.
func TestGateRenamedHeaders(t *testing.T) {
	g := newGate(t, GateConfig{Headers: tinysesam.HeaderNames{User: "X-Gate-User", ID: "X-Gate-Id", Auth: "X-Gate-Auth", Kind: "X-Gate-Kind"}})
	r := request(map[string]string{"X-Gate-User": "anna", "X-Gate-Id": "7", "X-Gate-Auth": "session", "X-Gate-Kind": "user"})
	user, err := g.Identify(r)
	if err != nil || user.RemoteID != "7" || user.Agent() {
		t.Fatalf("%+v %v", user, err)
	}
	if _, err := g.Identify(request(person(nil))); !errors.Is(err, tinysesam.ErrUnauthenticated) {
		t.Errorf("the default names after a rename: %v", err)
	}
}

func tokenGate(t *testing.T) *Gate {
	return newGate(t, GateConfig{Mode: GateToken, Issuer: gatetest.Issuer, Audience: gatetest.Audience, Keys: []string{gatetest.PublicKey()}})
}

func withToken(token string, headers map[string]string) *http.Request {
	r := request(headers)
	r.AddCookie(&http.Cookie{Name: tinysesam.DefaultGateCookie, Value: token})
	return r
}

// In token mode the signed token alone counts: Remote headers say nothing, and the kind of account is signed.
func TestGateToken(t *testing.T) {
	g := tokenGate(t)
	user, err := g.Identify(withToken(gatetest.Token("7", map[string]any{"ts_auth": "oauth", "ts_groups": "underdoc-admin"}),
		map[string]string{"Remote-Auth": "session", "Remote-User": "root", "Remote-Id": "1"}))
	if err != nil {
		t.Fatal(err)
	}
	if user.RemoteID != "7" || user.Login != "user7" || !user.Agent() || user.AuthKind != AuthOAuth {
		t.Errorf("an agent's token with Remote-Auth: session in a header: %+v", user)
	}
	// A token of a gate before 0.26.3 has no ts_auth: an agent until it expires.
	if user, err := g.Identify(withToken(gatetest.Token("7", map[string]any{"ts_auth": nil}), nil)); err != nil || !user.Agent() {
		t.Errorf("without ts_auth: %+v %v", user, err)
	}
	if _, err := g.Identify(withToken(gatetest.Token("8", map[string]any{"ts_kind": "service", "ts_auth": "apikey"}), nil)); !errors.Is(err, ErrService) {
		t.Errorf("a signed service account nobody allowed: %v", err)
	}
	allowed := newGate(t, GateConfig{Mode: GateToken, Issuer: gatetest.Issuer, Audience: gatetest.Audience, Keys: []string{gatetest.PublicKey()},
		ServiceAccounts: []string{"8"}})
	if user, err := allowed.Identify(withToken(gatetest.Token("8", map[string]any{"ts_kind": "service", "ts_auth": "apikey"}), nil)); err != nil ||
		user.Kind != KindService || !user.Agent() {
		t.Errorf("an allowed signed service account is one, an agent: %+v %v", user, err)
	}
	if _, err := g.Identify(withToken(gatetest.Token("7", map[string]any{"ts_kind": nil}), nil)); err == nil {
		t.Error("a token without ts_kind")
	}
	if _, err := g.Identify(request(person(nil))); err == nil {
		t.Error("headers without a token")
	}
	stranger := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	forged := gatetest.SignWith(stranger, map[string]any{"iss": gatetest.Issuer, "aud": gatetest.Audience, "sub": "7"})
	if _, err := g.Identify(withToken(forged, nil)); !errors.Is(err, tinysesam.ErrKid) {
		t.Errorf("a token signed by another key: %v", err)
	}
	if _, err := g.Identify(withToken(gatetest.Token("7", map[string]any{"aud": "other.example.com"}), nil)); !errors.Is(err, tinysesam.ErrAudience) {
		t.Errorf("a token for another app: %v", err)
	}
}

func TestGateConfiguration(t *testing.T) {
	for name, cfg := range map[string]GateConfig{
		"unknown mode":           {Mode: "cookie"},
		"token without keys":     {Mode: GateToken, Issuer: gatetest.Issuer, Audience: gatetest.Audience},
		"token without issuer":   {Mode: GateToken, Audience: gatetest.Audience, Keys: []string{gatetest.PublicKey()}},
		"header with a token":    {Issuer: gatetest.Issuer},
		"token with proxies":     {Mode: GateToken, Issuer: gatetest.Issuer, Audience: gatetest.Audience, Keys: []string{gatetest.PublicKey()}, Proxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}},
		"two equal header names": {Headers: tinysesam.HeaderNames{User: "Remote-Id"}},
	} {
		if _, err := NewGate(cfg); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// The delegated administrator (T-33): the role of the app makes one of a person, never of a service account, and its
// permissions hold for a person in a session only, never for an agent.
func TestTheDelegatedAdministratorRole(t *testing.T) {
	g := newGate(t, GateConfig{ServiceAccounts: []string{"42"}})
	user, err := g.Identify(request(person(map[string]string{"Remote-Groups": "underdoc-delegate"})))
	if err != nil || !user.Delegate || user.Admin || !user.Has(PermAccounts) || !user.Has(PermArchiveGroups) || user.Has(PermAdminister) {
		t.Errorf("a person with the delegate's role: %+v %v", user, err)
	}
	user.AuthKind = AuthOAuth
	if user.Has(PermAccounts) || user.Has(PermArchiveGroups) {
		t.Error("the agent of a delegated administrator holds its permissions")
	}
	service, err := g.Identify(request(person(map[string]string{"Remote-Id": "42", "Remote-Kind": "user", "Remote-Auth": "session",
		"Remote-Groups": "underdoc-delegate"})))
	if err != nil || service.Delegate {
		t.Errorf("a service account with the delegate's role is one: %+v %v", service, err)
	}
	admin := User{Admin: true, AuthKind: AuthSession}
	if !admin.Has(PermAccounts) || !admin.Has(PermArchiveGroups) || !admin.Has(PermAdminister) {
		t.Error("an administrator lacks a permission")
	}
	if _, err := NewGate(GateConfig{DelegateRole: "admin"}); err == nil {
		t.Error("the gate's own admin as the delegate's role")
	}
	if _, err := NewGate(GateConfig{DelegateRole: "underdoc-admin"}); err == nil {
		t.Error("the administrator's role as the delegate's role")
	}
}
