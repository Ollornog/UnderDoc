// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	tinysesam "github.com/Ollornog/TinySesam/clients/go"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/account/gatetest"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// Sign-in through the gate (ADR-32) and the attack round on it: who someone is, how they signed in and what kind of
// account they have come only from what the gate's client checked.

func gateOf(t *testing.T, cfg account.GateConfig) *account.Gate {
	t.Helper()
	g, err := account.NewGate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// gateSite is a site with the database behind the gate cfg describes.
func gateSite(t *testing.T, cfg account.GateConfig) (*site, *db.DB) {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	return newSite(t, Options{DB: database, GateClient: gateOf(t, cfg)}, sample), database
}

// raw sends a request with nothing added: the headers given, from the address given, with the gate cookie given.
func (s *site) raw(method, target, body, from, token string, headers map[string]string) *httptest.ResponseRecorder {
	s.t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	if from != "" {
		request.RemoteAddr = from
	}
	if token != "" {
		request.AddCookie(&http.Cookie{Name: tinysesam.DefaultGateCookie, Value: token})
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	s.http.ServeHTTP(recorder, request)
	return recorder
}

const fromProxy = "127.0.0.1:40000"

func tokenConfig() account.GateConfig {
	return account.GateConfig{Mode: account.GateToken, Issuer: gatetest.Issuer, Audience: gatetest.Audience, Keys: []string{gatetest.PublicKey()}}
}

// Attack: Remote headers without a valid gate token. In token mode no header signs anyone in, an administrator's
// included; a token of another key, for another app, or past its time does not either.
func TestForgedHeadersWithoutAGateToken(t *testing.T) {
	s, _ := gateSite(t, tokenConfig())
	if got := s.raw(http.MethodGet, "/api/state", "", "", "", gateHeaders).Code; got != http.StatusUnauthorized {
		t.Errorf("the headers of an administrator without a token = %d", got)
	}
	for name, token := range map[string]string{
		"another app":   gatetest.Token("1007", map[string]any{"aud": "other.example.com"}),
		"expired":       gatetest.Token("1007", map[string]any{"nbf": 1000, "iat": 1000, "exp": 1100}),
		"not a token":   "anna",
		"no kind":       gatetest.Token("1007", map[string]any{"ts_kind": nil}),
		"no lasting ID": gatetest.Token("1007", map[string]any{"ts_id": nil, "sub": ""}),
	} {
		if got := s.raw(http.MethodGet, "/api/state", "", "", token, gateHeaders).Code; got != http.StatusUnauthorized {
			t.Errorf("%s = %d", name, got)
		}
	}
	state := decode[apiState](s.t, s.raw(http.MethodGet, "/api/state", "", "", gatetest.Token("1007", map[string]any{"ts_user": "anna",
		"ts_groups": "underdoc-admin"}), nil))
	if state.User.Login != "anna" || !state.User.Admin {
		t.Errorf("a valid token signs in: %+v", state.User)
	}
}

// Attack: the headers over a way past the gate. With the proxies named, a request from any other address is refused,
// whatever it says; from the proxy it counts.
func TestHeadersPastTheGate(t *testing.T) {
	s, _ := gateSite(t, account.GateConfig{Proxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}})
	if got := s.raw(http.MethodGet, "/api/state", "", "198.51.100.7:5555", "", gateHeaders).Code; got != http.StatusUnauthorized {
		t.Errorf("the headers of an administrator from past the gate = %d", got)
	}
	if got := s.raw(http.MethodGet, "/api/state", "", fromProxy, "", gateHeaders).Code; got != http.StatusOK {
		t.Errorf("from the proxy = %d", got)
	}
}

// Attack: Remote-Auth=session in a header with an agent's token. The token says oauth, and only the token counts: the
// request is an agent, which never changes rights and administers nothing, even with the role of the app.
func TestSessionHeaderWithAnAgentToken(t *testing.T) {
	s, _ := gateSite(t, tokenConfig())
	anna := gatetest.Token("1007", map[string]any{"ts_user": "anna", "ts_groups": "underdoc-admin"})
	top := decode[apiNode](t, s.raw(http.MethodPost, "/api/collections", `{"title":"Team"}`, "", anna, nil))
	agent := gatetest.Token("1007", map[string]any{"ts_user": "anna", "ts_groups": "underdoc-admin", "ts_auth": "oauth"})
	session := map[string]string{"Remote-Auth": "session", "Remote-Kind": "service"}
	if got := s.raw(http.MethodPut, "/api/objects/"+top.ID+"/grants", `{"kind":"tenant","role":"read"}`, "", agent, session).Code; got != http.StatusForbidden {
		t.Errorf("the agent changes rights with Remote-Auth: session in a header = %d", got)
	}
	if state := decode[apiState](t, s.raw(http.MethodGet, "/api/state", "", "", agent, session)); state.User.Admin {
		t.Errorf("the agent of an administrator administers: %+v", state.User)
	}
	if got := s.raw(http.MethodGet, "/api/users", "", "", agent, session).Code; got != http.StatusForbidden {
		t.Errorf("the agent lists the users = %d", got)
	}
	// A token of a gate before 0.26.3 has no ts_auth: an agent until it expires.
	old := gatetest.Token("1007", map[string]any{"ts_user": "anna", "ts_groups": "underdoc-admin", "ts_auth": nil})
	if got := s.raw(http.MethodPut, "/api/objects/"+top.ID+"/grants", `{"kind":"tenant","role":"read"}`, "", old, session).Code; got != http.StatusForbidden {
		t.Errorf("a token without ts_auth changes rights = %d", got)
	}
}

// Attack: an agent with the role of the app, in header mode. Two Remote-Auth (the proxy's and one the browser added)
// are refused, and a Remote-Kind the browser sent makes no service account out of an agent while the kind cannot be
// trusted.
func TestAgentWithTheAdminRole(t *testing.T) {
	s, _ := gateSite(t, account.GateConfig{})
	agent := merged2(gateHeaders, map[string]string{"Remote-Auth": "apikey"})
	if state := decode[apiState](t, s.raw(http.MethodGet, "/api/state", "", "", "", agent)); state.User.Admin || state.User.Login != "anna" {
		t.Errorf("an agent with the role of the app administers: %+v", state.User)
	}
	if got := s.raw(http.MethodGet, "/api/users", "", "", "", agent).Code; got != http.StatusForbidden {
		t.Errorf("an agent with the role of the app lists the users = %d", got)
	}
	service := merged2(agent, map[string]string{"Remote-Kind": "service"})
	if got := s.raw(http.MethodPost, "/api/groups", `{"name":"By a service"}`, "", "", service).Code; got != http.StatusForbidden {
		t.Errorf("an agent that calls itself a service account makes a group = %d", got)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	for k, v := range agent {
		request.Header.Set(k, v)
	}
	request.Header.Add("Remote-Auth", "session")
	s.http.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("two Remote-Auth = %d", recorder.Code)
	}
	// The gate's own administrator is no administrator here.
	if state := decode[apiState](t, s.raw(http.MethodGet, "/api/state", "", "", "", merged2(gateHeaders, map[string]string{"Remote-Groups": "admin"}))); state.User.Admin {
		t.Errorf("the gate's administrator administers UnderDoc: %+v", state.User)
	}
}

// A person is bound by the lasting ID of the gate, never by the name: a new login keeps the account, the same login with
// another ID is another account, and an account of the gate keyed by its ID before ADR-32 is bound to it.
func TestBoundByTheLastingID(t *testing.T) {
	s, database := gateSite(t, account.GateConfig{})
	anna := decode[apiState](t, s.raw(http.MethodGet, "/api/state", "", "", "", gateHeaders))
	if anna.User.ID != "1007" || len(anna.Collections) == 0 {
		t.Fatalf("the first request makes the account under the lasting ID: %+v", anna.User)
	}
	notes := anna.Collections[0].ID
	renamed := decode[apiState](t, s.raw(http.MethodGet, "/api/state", "", "", "", merged2(gateHeaders, map[string]string{"Remote-User": "anna.berg"})))
	if renamed.User.ID != "1007" || findNode(renamed.Collections, notes) == nil {
		t.Errorf("a new login keeps the account and its notes: %+v", renamed.User)
	}
	other := decode[apiState](t, s.raw(http.MethodGet, "/api/state", "", "", "", merged2(gateHeaders, map[string]string{"Remote-Id": "1999"})))
	if other.User.ID == "1007" || findNode(other.Collections, notes) != nil {
		t.Errorf("the login of anna with another ID is someone else: %+v", other.User)
	}
	ctx := tenant.With(context.Background(), tenant.Main)
	// An account under a login, as the gate made one before ADR-32 without an ID: someone of the gate with that ID as
	// their lasting ID does not come into it.
	if err := database.SeeAccount(ctx, account.User{ID: "1500", Login: "1500", Name: "Old"}); err != nil {
		t.Fatal(err)
	}
	newcomer := decode[apiState](t, s.raw(http.MethodGet, "/api/state", "", "", "", merged2(gateHeaders, map[string]string{"Remote-Id": "1500", "Remote-User": "eve"})))
	if newcomer.User.ID == "1500" {
		t.Errorf("an account not bound to the ID was taken: %+v", newcomer.User)
	}
}

// What the gate passes on of the account is kept, and a request that is no session leaves the groups as they are: a
// token carries a part of the roles only.
func TestAccountKeepsWhatTheGateSays(t *testing.T) {
	s, database := gateSite(t, account.GateConfig{})
	s.raw(http.MethodGet, "/api/state", "", "", "", merged2(gateHeaders, map[string]string{"Remote-Kind": "user"}))
	ctx := tenant.With(context.Background(), tenant.Main)
	a, err := database.AccountByID(ctx, "1007")
	if err != nil || a.RemoteID != "1007" || a.GateKind != "user" || len(a.Groups) != 2 {
		t.Fatalf("%+v %v", a, err)
	}
	// After the minutes the account is kept in memory, an agent with fewer roles comes.
	fresh := newSite(t, Options{DB: database, GateClient: gateOf(t, account.GateConfig{})}, sample)
	fresh.raw(http.MethodGet, "/api/state", "", "", "", merged2(gateHeaders, map[string]string{"Remote-Auth": "oauth", "Remote-Groups": ""}))
	if a, _ := database.AccountByID(ctx, "1007"); len(a.Groups) != 2 || a.GateKind != "" {
		t.Errorf("an agent changed the groups, or the kind was not kept as sent: %+v", a)
	}
}

// The sign-in of UnderDoc's own is gone (ADR-32): none of its pages answers, through the gate or past it, and a cookie
// of a session from before signs nobody in.
func TestNoSignInOfItsOwn(t *testing.T) {
	s, _ := gateSite(t, account.GateConfig{})
	for _, target := range []string{"GET /sign-in", "POST /sign-in/password", "POST /sign-in/passkey/options", "GET /sign-in/oidc",
		"GET /invite/abc", "GET /register", "POST /register", "GET /account/security", "POST /account/password", "GET /admin/sign-in",
		"POST /admin/invites", "POST /sign-out"} {
		method, path, _ := strings.Cut(target, " ")
		if got := s.raw(method, path, "login=anna&password=x", "", "", nil).Code; got != http.StatusUnauthorized {
			t.Errorf("%s without the gate = %d", target, got)
		}
		if got := s.raw(method, path, "", "", "", gateHeaders).Code; got != http.StatusNotFound && got != http.StatusMethodNotAllowed {
			t.Errorf("%s through the gate = %d", target, got)
		}
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	request.AddCookie(&http.Cookie{Name: "__Host-underdoc", Value: "a-session-from-before"})
	s.http.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("a session cookie from before = %d", recorder.Code)
	}
}

// Attack: the app password of Samba as a sign-in to the web. It is made for the share alone (ADR-30): neither as Basic
// authentication nor as a form does it sign anyone in.
func TestSambaPasswordIsNoWebSignIn(t *testing.T) {
	s := withSamba(t)
	made := decode[map[string]string](t, s.send(http.MethodPost, "/api/samba/password", `{}`, merged2(bobOnly, sameSite)))
	if made["password"] == "" {
		t.Fatalf("no app password: %+v", made)
	}
	basic := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	basic.SetBasicAuth(made["name"], made["password"])
	recorder := httptest.NewRecorder()
	s.http.ServeHTTP(recorder, basic)
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("the app password as Basic authentication = %d", recorder.Code)
	}
	form := "login=" + made["name"] + "&password=" + made["password"]
	for _, path := range []string{"/sign-in/password", "/sign-in"} {
		if got := s.raw(http.MethodPost, path, form, "", "", map[string]string{"Content-Type": "application/x-www-form-urlencoded"}); got.Code != http.StatusUnauthorized ||
			len(got.Result().Cookies()) != 0 {
			t.Errorf("the app password as a form to %s = %d %v", path, got.Code, got.Result().Cookies())
		}
	}
}

// Attack on the emergency access: the mark of the command line belongs to the lasting ID it names. Someone of the gate
// with the same login and another ID is no administrator, the agent of the marked person administers nothing, and the
// web gives the mark to nobody.
func TestEmergencyAccessThroughTheWeb(t *testing.T) {
	s, database := gateSite(t, account.GateConfig{})
	if _, err := database.MarkAdmin(inMain(), "1300", true); err != nil {
		t.Fatal(err)
	}
	marked := map[string]string{"Remote-Auth": "session", "Remote-User": "rescue", "Remote-Id": "1300", "Remote-Name": "Rescue"}
	if state := decode[apiState](t, s.raw(http.MethodGet, "/api/state", "", "", "", marked)); !state.User.Admin || state.User.ID != "1300" {
		t.Errorf("the marked account without the role: %+v", state.User)
	}
	namesake := merged2(marked, map[string]string{"Remote-Id": "1301"})
	if state := decode[apiState](t, s.raw(http.MethodGet, "/api/state", "", "", "", namesake)); state.User.Admin {
		t.Errorf("the same login with another ID administers: %+v", state.User)
	}
	agent := merged2(marked, map[string]string{"Remote-Auth": "apikey"})
	if got := s.raw(http.MethodGet, "/api/users", "", "", "", agent).Code; got != http.StatusForbidden {
		t.Errorf("the agent of the marked account lists the users = %d", got)
	}
	if got := s.raw(http.MethodPut, "/api/users/1301/admin", `{"admin":true}`, "", "", marked).Code; got != http.StatusConflict {
		t.Errorf("the marked account marks another = %d", got)
	}
	if a, _ := database.AccountByID(inMain(), "1301"); a.Admin {
		t.Error("the namesake was marked")
	}
}
