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

// Remote-Kind is not yet the only barrier (ADR-32): a proxy of the gate may still pass on what the browser sent. So by
// default a missing kind is let through as unknown, and "service" makes no service account; only the switch does.
func TestGateKind(t *testing.T) {
	g := newGate(t, GateConfig{})
	user, err := g.Identify(request(person(map[string]string{"Remote-Kind": "-", "Remote-Auth": "oauth"})))
	if err != nil || user.GateKind != "" || user.Kind != KindPerson || !user.Agent() {
		t.Fatalf("without Remote-Kind: %+v %v", user, err)
	}
	if user, err := g.Identify(request(person(map[string]string{"Remote-Kind": ""}))); err != nil || user.GateKind != "" || user.Agent() {
		t.Errorf("an empty Remote-Kind is none: %+v %v", user, err)
	}
	user, err = g.Identify(request(person(map[string]string{"Remote-Kind": "service", "Remote-Auth": "apikey"})))
	if err != nil || user.GateKind != "service" || user.Kind != KindPerson || !user.Agent() {
		t.Errorf("a service account the gate cannot vouch for yet is an agent: %+v %v", user, err)
	}
	if _, err := g.Identify(request(person(map[string]string{"Remote-Kind": "robot"}))); !errors.Is(err, tinysesam.ErrHeader) {
		t.Errorf("an unknown kind: %v", err)
	}

	strict := newGate(t, GateConfig{KindRequired: true})
	if _, err := strict.Identify(request(person(map[string]string{"Remote-Kind": "-"}))); !errors.Is(err, tinysesam.ErrHeader) {
		t.Errorf("switched on, a request without Remote-Kind is refused: %v", err)
	}
	user, err = strict.Identify(request(person(map[string]string{"Remote-Kind": "service", "Remote-Auth": "apikey"})))
	if err != nil || user.Kind != KindService || user.Agent() {
		t.Errorf("switched on, a service account is one: %+v %v", user, err)
	}
	if _, err := strict.Identify(request(person(map[string]string{"Remote-Kind": "service", "Remote-Auth": "session"}))); err == nil {
		t.Error("a service account has no session")
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
	r := request(map[string]string{"X-Gate-User": "anna", "X-Gate-Id": "7", "X-Gate-Auth": "session"})
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
	if user, err := g.Identify(withToken(gatetest.Token("8", map[string]any{"ts_kind": "service", "ts_auth": "apikey"}), nil)); err != nil ||
		user.Kind != KindService || user.Agent() {
		t.Errorf("a signed service account is one: %+v %v", user, err)
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
