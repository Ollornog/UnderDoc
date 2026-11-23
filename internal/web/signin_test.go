// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/Ollornog/UnderDoc/internal/auth"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
)

// testVaultKey is a key for tests only.
const testVaultKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

const signInHost = "docs.example"

func testVault(t *testing.T) *vault.Vault {
	t.Helper()
	v, err := vault.New(testVaultKey)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func inMain() context.Context { return tenant.With(context.Background(), tenant.Main) }

// signInSite is a site whose tenant Main lets people in as s says; nil leaves it as a new instance starts.
func signInSite(t *testing.T, s *db.SignIn, options Options) (*site, *db.DB) {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if s != nil {
		if err := database.SetSignIn(inMain(), *s); err != nil {
			t.Fatal(err)
		}
	}
	options.DB = database
	if options.Vault == nil {
		options.Vault = testVault(t)
	}
	return newSite(t, options, sample), database
}

// client is a browser for tests: it keeps cookies, sends the host and says that its requests come from the same site.
type client struct {
	t         *testing.T
	s         *site
	host      string
	cookies   map[string]string
	crossSite bool
	headers   map[string]string
}

func newClient(t *testing.T, s *site, host string) *client {
	return &client{t: t, s: s, host: host, cookies: map[string]string{}, headers: map[string]string{}}
}

func (c *client) do(method, target, contentType, body string) *httptest.ResponseRecorder {
	c.t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Host = c.host
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if c.crossSite {
		request.Header.Set("Sec-Fetch-Site", "cross-site")
	} else {
		request.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	for name, value := range c.headers {
		request.Header.Set(name, value)
	}
	for name, value := range c.cookies {
		request.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	recorder := httptest.NewRecorder()
	c.s.http.ServeHTTP(recorder, request)
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.MaxAge < 0 {
			delete(c.cookies, cookie.Name)
		} else {
			c.cookies[cookie.Name] = cookie.Value
		}
	}
	return recorder
}

func (c *client) get(target string) *httptest.ResponseRecorder {
	return c.do(http.MethodGet, target, "", "")
}

func (c *client) form(target string, values url.Values) *httptest.ResponseRecorder {
	return c.do(http.MethodPost, target, "application/x-www-form-urlencoded", values.Encode())
}

func (c *client) post(target, body string) *httptest.ResponseRecorder {
	return c.do(http.MethodPost, target, "application/json", body)
}

// invite makes an invitation in the tenant of ctx and returns its token.
func invite(t *testing.T, database *db.DB, ctx context.Context, login, name string, admin bool) string {
	t.Helper()
	token, hash, err := auth.NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.CreateInvite(ctx, hash, db.Invite{Login: login, Name: name, Admin: admin, Expires: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	return token
}

func login(t *testing.T, c *client) string {
	t.Helper()
	got := c.get("/api/state")
	if got.Code != http.StatusOK {
		return ""
	}
	return decode[apiState](t, got).User.Login
}

var gateOnly = &db.SignIn{Methods: []string{db.MethodGate}}

// TestGateModeIsUnchanged is the promise to the instance that runs behind a gate (ADR-23, the test instance): with the
// gate as its only way in, the headers sign in as before, a request without them gets the same page as before, and none
// of the new pages answers. A session cookie does not count there, and an instance that turns the gate off trusts no
// header.
func TestGateModeIsUnchanged(t *testing.T) {
	s, database := signInSite(t, gateOnly, Options{})
	c := newClient(t, s, "wiki.example")
	c.headers = gateHeaders
	state := decode[apiState](t, c.get("/api/state"))
	if state.User.Login != "anna" || state.User.Method != "gate" || !state.User.Admin || len(state.Collections) == 0 {
		t.Fatalf("through the gate: %+v", state.User)
	}
	anonymous := newClient(t, s, "wiki.example")
	for _, target := range []string{"/", "/api/state", "/o/start"} {
		got := anonymous.get(target)
		if got.Code != http.StatusUnauthorized || !strings.Contains(got.Body.String(), "UnderDoc is reached through its sign-in gate") {
			t.Errorf("%s without the gate: %d", target, got.Code)
		}
	}
	for _, target := range []string{"/sign-in", "/register", "/invite/x"} {
		if got := anonymous.get(target); got.Code != http.StatusNotFound {
			t.Errorf("%s answers with the gate alone: %d", target, got.Code)
		}
	}
	// A session of a local account does not let in where the gate is the only way.
	token, hash, _ := auth.NewToken()
	if _, err := database.AcceptInvite(inMain(), mustInviteHash(t, database, "carl"), db.NewLocal{PasswordHash: "x"}); err != nil {
		t.Fatal(err)
	}
	carl, _ := database.LocalAccount(inMain(), "carl")
	if err := database.CreateSession(inMain(), hash, carl.ID, db.MethodPassword, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	anonymous.cookies[sessionCookie] = token
	if got := anonymous.get("/api/state"); got.Code != http.StatusUnauthorized {
		t.Errorf("a password session where only the gate counts: %d", got.Code)
	}
	// With the gate switched off for the instance, the same headers count for nothing.
	off := newSite(t, Options{DB: database, Gate: GateOff}, sample)
	if got := newClientWith(t, off, gateHeaders).get("/api/state"); got.Code != http.StatusUnauthorized {
		t.Errorf("headers count with the gate off: %d", got.Code)
	}
}

func newClientWith(t *testing.T, s *site, headers map[string]string) *client {
	c := newClient(t, s, "wiki.example")
	c.headers = headers
	return c
}

func mustInviteHash(t *testing.T, database *db.DB, login string) []byte {
	t.Helper()
	return auth.HashToken(invite(t, database, inMain(), login, strings.ToUpper(login[:1])+login[1:], false))
}

// TestANewInstanceAsksForAPasskey: without settings, a tenant signs in with passkeys only, and the headers of a gate are
// forged headers there.
func TestANewInstanceAsksForAPasskey(t *testing.T) {
	s, database := signInSite(t, nil, Options{})
	settings, err := database.SignIn(inMain())
	if err != nil || len(settings.Methods) != 1 || settings.Methods[0] != db.MethodPasskey || settings.Registration {
		t.Fatalf("%+v %v", settings, err)
	}
	c := newClientWith(t, s, gateHeaders)
	if got := c.get("/api/state"); got.Code != http.StatusUnauthorized {
		t.Errorf("forged gate headers: %d %s", got.Code, got.Body.String())
	}
	got := c.get("/o/start?x=1")
	if got.Code != http.StatusSeeOther || got.Header().Get("Location") != "/sign-in?next=%2Fo%2Fstart%3Fx%3D1" {
		t.Errorf("a page leads to the sign-in: %d %q", got.Code, got.Header().Get("Location"))
	}
	page := c.get("/sign-in").Body.String()
	if !strings.Contains(page, "data-passkey-sign-in") || strings.Contains(page, `action="/sign-in/password"`) || strings.Contains(page, "/register") {
		t.Errorf("the sign-in page offers passkeys only: %s", page)
	}
	if !strings.Contains(page, `<div class="card w-full max-w-sm" data-test="signin-card">`) || strings.Contains(page, "field-separator") {
		t.Error("with passkeys only, the sign-in is a card of buttons, without an \"or\"")
	}
	if got := c.get("/register"); got.Code != http.StatusNotFound {
		t.Errorf("registration is off: %d", got.Code)
	}
	if got := c.form("/sign-in/password", url.Values{"login": {"x"}, "password": {"y"}}); got.Code != http.StatusNotFound {
		t.Errorf("passwords are off: %d", got.Code)
	}
}

func TestPasswordsAndSessions(t *testing.T) {
	s, database := signInSite(t, &db.SignIn{Methods: []string{db.MethodPassword}}, Options{})
	c := newClient(t, s, signInHost)
	token := invite(t, database, inMain(), "dora", "Dora Kim", false)
	page := c.get("/invite/" + token)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Welcome, Dora Kim") || strings.Contains(page.Body.String(), "data-passkey-create") {
		t.Fatalf("invitation page: %d %s", page.Code, page.Body.String())
	}
	const password = "a long enough password"
	for _, bad := range []url.Values{
		{"password": {"short"}, "repeat": {"short"}},
		{"password": {password}, "repeat": {password + "!"}},
	} {
		if got := c.form("/invite/"+token+"/password", bad); got.Code != http.StatusBadRequest {
			t.Errorf("%v: %d", bad, got.Code)
		}
	}
	cross := newClient(t, s, signInHost)
	cross.crossSite = true
	if got := cross.form("/invite/"+token+"/password", url.Values{"password": {password}, "repeat": {password}}); got.Code != http.StatusForbidden {
		t.Errorf("from another site: %d", got.Code)
	}
	got := c.form("/invite/"+token+"/password", url.Values{"password": {password}, "repeat": {password}})
	if got.Code != http.StatusSeeOther {
		t.Fatalf("accepting: %d %s", got.Code, got.Body.String())
	}
	cookie := got.Header().Get("Set-Cookie")
	for _, part := range []string{sessionCookie + "=", "Path=/", "HttpOnly", "Secure", "SameSite=Lax"} {
		if !strings.Contains(cookie, part) {
			t.Errorf("the session cookie lacks %s: %s", part, cookie)
		}
	}
	if strings.Contains(cookie, "Domain=") {
		t.Errorf("the session cookie names a domain: %s", cookie)
	}
	if who := login(t, c); who != "dora" {
		t.Fatalf("signed in as %q", who)
	}
	if got := newClient(t, s, signInHost).get("/invite/" + token); got.Code != http.StatusNotFound {
		t.Errorf("an invitation works once: %d", got.Code)
	}
	// Sign out, then in again; a wrong password does not.
	first := c.cookies[sessionCookie]
	if got := c.form("/sign-out", nil); got.Code != http.StatusSeeOther || c.cookies[sessionCookie] != "" {
		t.Fatalf("sign-out: %d %v", got.Code, c.cookies)
	}
	c.cookies[sessionCookie] = first
	if who := login(t, c); who != "" {
		t.Error("a session works after signing out")
	}
	if got := c.form("/sign-in/password", url.Values{"login": {"dora"}, "password": {"wrong password!!"}}); got.Code != http.StatusUnauthorized ||
		!strings.Contains(got.Body.String(), "The login, the password or the code is wrong.") {
		t.Errorf("wrong password: %d", got.Code)
	}
	if got := c.form("/sign-in/password", url.Values{"login": {"nobody"}, "password": {password}}); got.Code != http.StatusUnauthorized ||
		!strings.Contains(got.Body.String(), "The login, the password or the code is wrong.") {
		t.Errorf("unknown login, same answer: %d", got.Code)
	}
	// A token planted before the sign-in is never the session after it.
	planted, plantedHash, _ := auth.NewToken()
	dora, _ := database.LocalAccount(inMain(), "dora")
	if err := database.CreateSession(inMain(), plantedHash, dora.ID, db.MethodPassword, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	c.cookies[sessionCookie] = planted
	got = c.form("/sign-in/password", url.Values{"login": {"Dora"}, "password": {password}, "next": {"/o/start"}})
	if got.Code != http.StatusSeeOther || got.Header().Get("Location") != "/o/start" {
		t.Fatalf("sign-in: %d %q", got.Code, got.Header().Get("Location"))
	}
	if c.cookies[sessionCookie] == planted || c.cookies[sessionCookie] == "" {
		t.Error("the planted token stays the session")
	}
	attacker := newClient(t, s, signInHost)
	attacker.cookies[sessionCookie] = planted
	if who := login(t, attacker); who != "" {
		t.Error("the planted token is signed in")
	}
	for next, want := range map[string]string{"//evil.example/x": "/", "https://evil.example": "/", "/\\evil.example": "/", "/sign-in": "/", "/o/a?b=c": "/o/a?b=c"} {
		got := newClient(t, s, signInHost).form("/sign-in/password", url.Values{"login": {"dora"}, "password": {password}, "next": {next}})
		if got.Header().Get("Location") != want {
			t.Errorf("next %q leads to %q", next, got.Header().Get("Location"))
		}
	}
	// Switching passwords off ends their sessions.
	if err := database.SetSignIn(inMain(), db.SignIn{Methods: []string{db.MethodPasskey}}); err != nil {
		t.Fatal(err)
	}
	if who := login(t, c); who != "" {
		t.Error("a password session outlives passwords")
	}
	events, err := database.SignInLog(inMain(), 50)
	if err != nil {
		t.Fatal(err)
	}
	var ok, failed int
	for _, e := range events {
		if e.OK {
			ok++
		} else {
			failed++
		}
	}
	if ok < 6 || failed != 2 {
		t.Errorf("the log has %d sign-ins and %d failures: %+v", ok, failed, events)
	}
}

// TestGuessingIsSlowedDown: after five wrong passwords, the right one waits too, and the waiting is logged.
func TestGuessingIsSlowedDown(t *testing.T) {
	s, database := signInSite(t, &db.SignIn{Methods: []string{db.MethodPassword}}, Options{})
	token := invite(t, database, inMain(), "erik", "Erik", false)
	const password = "a long enough password"
	c := newClient(t, s, signInHost)
	c.form("/invite/"+token+"/password", url.Values{"password": {password}, "repeat": {password}})
	for i := range 6 {
		got := newClient(t, s, signInHost).form("/sign-in/password", url.Values{"login": {"erik"}, "password": {"guess number " + string(rune('a'+i))}})
		if got.Code != http.StatusUnauthorized {
			t.Fatalf("guess %d: %d", i, got.Code)
		}
	}
	got := newClient(t, s, signInHost).form("/sign-in/password", url.Values{"login": {"ERIK"}, "password": {password}})
	if got.Code != http.StatusTooManyRequests || !strings.Contains(got.Body.String(), "Too many tries") {
		t.Errorf("the right password right after the guesses: %d", got.Code)
	}
	if got := newClient(t, s, signInHost).form("/sign-in/password", url.Values{"login": {"someone"}, "password": {"x"}}); got.Code != http.StatusUnauthorized {
		t.Errorf("another login waits too: %d", got.Code)
	}
	events, _ := database.SignInLog(inMain(), 1)
	if len(events) != 1 || events[0].Reason != "wrong login or password" {
		t.Errorf("%+v", events)
	}
	events, _ = database.SignInLog(inMain(), 3)
	if events[1].Reason != "too many tries" {
		t.Errorf("the waiting is logged: %+v", events)
	}
}

// TestPasswordAndCode: with TOTP, the code is asked for at the invitation and at every sign-in, and a code works once.
func TestPasswordAndCode(t *testing.T) {
	s, database := signInSite(t, &db.SignIn{Methods: []string{db.MethodPasswordTOTP}}, Options{})
	token := invite(t, database, inMain(), "fay", "Fay", false)
	c := newClient(t, s, signInHost)
	page := c.get("/invite/" + token).Body.String()
	secret := between(page, `name="totp_secret" value="`, `"`)
	if secret == "" || !strings.Contains(page, `src="data:image/png;base64,`) {
		t.Fatalf("no TOTP setup: %s", page)
	}
	const password = "a long enough password"
	if got := c.form("/invite/"+token+"/password", url.Values{"password": {password}, "repeat": {password}, "totp_secret": {secret}}); got.Code != http.StatusBadRequest {
		t.Errorf("without a code: %d", got.Code)
	}
	now := time.Now()
	code, _ := totp.GenerateCode(secret, now)
	if got := c.form("/invite/"+token+"/password", url.Values{"password": {password}, "repeat": {password}, "totp_secret": {secret}, "code": {code}}); got.Code != http.StatusSeeOther {
		t.Fatalf("with the code: %d %s", got.Code, got.Body.String())
	}
	fay, _ := database.LocalAccount(inMain(), "fay")
	if fay.TOTPSealed == nil || strings.Contains(string(fay.TOTPSealed), secret) {
		t.Fatal("the TOTP secret is not sealed")
	}
	signIn := func(code string) int {
		return newClient(t, s, signInHost).form("/sign-in/password", url.Values{"login": {"fay"}, "password": {password}, "code": {code}}).Code
	}
	if got := signIn(""); got != http.StatusUnauthorized {
		t.Errorf("without a code: %d", got)
	}
	if got := signIn(code); got != http.StatusUnauthorized {
		t.Errorf("the code of the invitation again: %d", got)
	}
	next, _ := totp.GenerateCode(secret, now.Add(30*time.Second))
	if got := signIn(next); got != http.StatusSeeOther {
		t.Errorf("the next code: %d", got)
	}
	if got := signIn(next); got != http.StatusUnauthorized {
		t.Errorf("the same code twice: %d", got)
	}
}

// inviteLink is the link that works once, from C22's field to copy it in (template "invite-link").
func inviteLink(body string) string {
	return html.UnescapeString(between(body, `id="invite-link" type="text" value="`, `"`))
}

func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	s = s[i+len(start):]
	j := strings.Index(s, end)
	if j < 0 {
		return ""
	}
	return s[:j]
}

// TestPasskeys: an invited account sets up a passkey, signs in with it, and an answer of the authenticator works once,
// on this host only, and not with a counter that goes back.
func TestPasskeys(t *testing.T) {
	s, database := signInSite(t, &db.SignIn{Methods: []string{db.MethodPasskey}}, Options{})
	key := newAuthenticator(t)
	origin := "https://" + signInHost
	token := invite(t, database, inMain(), "gus", "Gus", true)
	c := newClient(t, s, signInHost)
	options := c.post("/invite/"+token+"/passkey/options", "")
	if options.Code != http.StatusOK {
		t.Fatalf("options: %d %s", options.Code, options.Body.String())
	}
	credID, body := key.create(options.Body.Bytes(), origin)
	if got := c.post("/invite/"+token+"/passkey?name=Laptop", body); got.Code != http.StatusOK {
		t.Fatalf("finish: %d %s", got.Code, got.Body.String())
	}
	if who := login(t, c); who != "gus" {
		t.Fatalf("after the invitation: %q", who)
	}
	if state := decode[apiState](t, c.get("/api/state")); !state.User.Admin || state.User.Method != db.MethodPasskey {
		t.Errorf("an invited administrator: %+v", state.User)
	}
	signIn := func(c *client, answer func(options []byte) string) int {
		options := c.post("/sign-in/passkey/options", "")
		if options.Code != http.StatusOK {
			t.Fatalf("options: %d", options.Code)
		}
		return c.post("/sign-in/passkey?next=/o/start", answer(options.Body.Bytes())).Code
	}
	fresh := newClient(t, s, signInHost)
	var used string
	if got := signIn(fresh, func(o []byte) string { used = key.get(o, credID, origin); return used }); got != http.StatusOK || login(t, fresh) != "gus" {
		t.Fatalf("sign-in with the passkey: %d", got)
	}
	// The same answer again, on a new ceremony: its challenge is gone.
	again := newClient(t, s, signInHost)
	if got := signIn(again, func([]byte) string { return used }); got != http.StatusUnauthorized || login(t, again) != "" {
		t.Errorf("a replayed answer: %d", got)
	}
	// The same ceremony twice: it works once.
	twice := newClient(t, s, signInHost)
	o := twice.post("/sign-in/passkey/options", "").Body.Bytes()
	answer := key.get(o, credID, origin)
	ceremony := twice.cookies[ceremonyCookie]
	if got := twice.post("/sign-in/passkey", answer); got.Code != http.StatusOK {
		t.Fatalf("first use: %d", got.Code)
	}
	replay := newClient(t, s, signInHost)
	replay.cookies[ceremonyCookie] = ceremony
	if got := replay.post("/sign-in/passkey", answer); got.Code != http.StatusUnauthorized || login(t, replay) != "" {
		t.Errorf("the same ceremony twice: %d", got.Code)
	}
	// Another origin, as a page of another site would send.
	evil := newClient(t, s, signInHost)
	if got := signIn(evil, func(o []byte) string { return key.get(o, credID, "https://evil.example") }); got != http.StatusUnauthorized {
		t.Errorf("another origin: %d", got)
	}
	// A counter that goes back marks a copied authenticator.
	key.counter = 1
	copied := newClient(t, s, signInHost)
	if got := signIn(copied, func(o []byte) string { return key.get(o, credID, origin) }); got != http.StatusUnauthorized {
		t.Errorf("a counter that went back: %d", got)
	}
	events, _ := database.SignInLog(inMain(), 1)
	if len(events) != 1 || !strings.Contains(events[0].Reason, "copied") {
		t.Errorf("%+v", events)
	}
	// A synced passkey has no counter, so only the ceremony stops the same answer from working twice.
	cloud := newAuthenticator(t)
	cloud.synced = true
	una := invite(t, database, inMain(), "una", "Una", false)
	u := newClient(t, s, signInHost)
	unaKey, body := cloud.create(u.post("/invite/"+una+"/passkey/options", "").Body.Bytes(), origin)
	if got := u.post("/invite/"+una+"/passkey", body); got.Code != http.StatusOK {
		t.Fatalf("a synced passkey: %d %s", got.Code, got.Body.String())
	}
	once := newClient(t, s, signInHost)
	answer = cloud.get(once.post("/sign-in/passkey/options", "").Body.Bytes(), unaKey, origin)
	ceremony = once.cookies[ceremonyCookie]
	if got := once.post("/sign-in/passkey", answer); got.Code != http.StatusOK {
		t.Fatalf("a synced passkey signs in: %d", got.Code)
	}
	replay = newClient(t, s, signInHost)
	replay.cookies[ceremonyCookie] = ceremony
	if got := replay.post("/sign-in/passkey", answer); got.Code != http.StatusUnauthorized || login(t, replay) != "" {
		t.Errorf("a synced passkey's answer twice: %d", got.Code)
	}
	// Another passkey, added on the security page, and the last one cannot go.
	key.counter = 100
	options = fresh.post("/account/passkeys/options", "")
	second, body := key.create(options.Body.Bytes(), origin)
	if got := fresh.post("/account/passkeys?name=Phone", body); got.Code != http.StatusOK {
		t.Fatalf("a second passkey: %d %s", got.Code, got.Body.String())
	}
	keys, _ := database.Passkeys(inMain(), (func() string { a, _ := database.LocalAccount(inMain(), "gus"); return a.ID })())
	if len(keys) != 2 || keys[1].Name != "Phone" {
		t.Fatalf("%+v", keys)
	}
	if got := fresh.form("/account/passkeys/"+second+"/remove", nil); got.Code != http.StatusOK {
		t.Errorf("remove one of two: %d", got.Code)
	}
	if got := fresh.form("/account/passkeys/"+credID+"/remove", nil); got.Code != http.StatusBadRequest || !strings.Contains(got.Body.String(), "last way in") {
		t.Errorf("remove the last: %d", got.Code)
	}
}

// TestPasskeysStayWithTheirTenant: a passkey made on one tenant's host does not sign in on another's.
func TestPasskeysStayWithTheirTenant(t *testing.T) {
	s, database := withTenants(t)
	for _, id := range []string{tenant.Main, "acme"} {
		if err := database.SetSignIn(tenant.With(context.Background(), id), db.SignIn{Methods: []string{db.MethodPasskey}}); err != nil {
			t.Fatal(err)
		}
	}
	key := newAuthenticator(t)
	token := invite(t, database, tenant.With(context.Background(), "acme"), "hal", "Hal", false)
	c := newClient(t, s, acmeHost)
	credID, body := key.create(c.post("/invite/"+token+"/passkey/options", "").Body.Bytes(), "https://"+acmeHost)
	if got := c.post("/invite/"+token+"/passkey", body); got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body.String())
	}
	other := newClient(t, s, mainHost)
	options := other.post("/sign-in/passkey/options", "").Body.Bytes()
	// The browser would not even offer it; a forged answer with acme's origin, or with Main's, is refused.
	for _, origin := range []string{"https://" + acmeHost, "https://" + mainHost} {
		if got := other.post("/sign-in/passkey", key.get(options, credID, origin)); got.Code != http.StatusUnauthorized {
			t.Errorf("acme's passkey on Main with origin %s: %d", origin, got.Code)
		}
		options = other.post("/sign-in/passkey/options", "").Body.Bytes()
	}
}

// TestAdministratorsChooseTheWaysIn: only an administrator sees and changes the sign-in of the tenant; the gate can only
// be switched on where the instance allows it; an invitation link shows once; registration is off until switched on.
func TestAdministratorsChooseTheWaysIn(t *testing.T) {
	s, database := signInSite(t, gateOnly, Options{})
	admin := newClientWith(t, s, gateHeaders)
	plain := newClientWith(t, s, bob)
	for _, c := range []*client{plain, newClient(t, s, signInHost)} {
		if got := c.get("/admin/sign-in"); got.Code == http.StatusOK {
			t.Errorf("not an administrator sees the page: %d", got.Code)
		}
		if got := c.form("/admin/sign-in", url.Values{"method": {db.MethodPassword}}); got.Code == http.StatusOK {
			t.Errorf("not an administrator changes it: %d", got.Code)
		}
	}
	page := admin.get("/admin/sign-in")
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `value="gate" checked`) {
		t.Fatalf("%d %s", page.Code, page.Body.String())
	}
	cross := newClientWith(t, s, gateHeaders)
	cross.crossSite = true
	if got := cross.form("/admin/sign-in", url.Values{"method": {db.MethodPassword}}); got.Code != http.StatusForbidden {
		t.Errorf("from another site: %d", got.Code)
	}
	for _, bad := range []url.Values{{}, {"method": {"magic"}}, {"method": {db.MethodOIDC}, "oidc_issuer": {"http://id.example"}, "oidc_client_id": {"x"}}} {
		if got := admin.form("/admin/sign-in", bad); got.Code != http.StatusBadRequest {
			t.Errorf("%v: %d", bad, got.Code)
		}
	}
	got := admin.form("/admin/sign-in", url.Values{"method": {db.MethodGate, db.MethodPassword, db.MethodPasswordTOTP}, "registration": {"on"}})
	if got.Code != http.StatusOK {
		t.Fatalf("save: %d %s", got.Code, got.Body.String())
	}
	if settings, _ := database.SignIn(inMain()); !settings.Registration || len(settings.Methods) != 3 {
		t.Errorf("%+v", settings)
	}
	// Without the gate, it cannot come back but by whoever runs the instance.
	admin.form("/admin/sign-in", url.Values{"method": {db.MethodPassword}, "registration": {"on"}})
	local := newClient(t, s, signInHost)
	reg := local.form("/register", url.Values{"login": {"lena"}, "name": {"Lena"}})
	if reg.Code != http.StatusSeeOther || !strings.HasPrefix(reg.Header().Get("Location"), "/invite/") {
		t.Fatalf("register: %d", reg.Code)
	}
	const password = "a long enough password"
	local.form(reg.Header().Get("Location")+"/password", url.Values{"password": {password}, "repeat": {password}})
	if who := login(t, local); who != "lena" {
		t.Fatalf("registered: %q", who)
	}
	if state := decode[apiState](t, local.get("/api/state")); state.User.Admin {
		t.Error("someone who registers is no administrator")
	}
	if got := local.get("/admin/sign-in"); got.Code != http.StatusNotFound {
		t.Errorf("a registered account administers: %d", got.Code)
	}
	if got := local.form("/register", url.Values{"login": {"lena"}, "name": {"Again"}}); got.Code != http.StatusBadRequest {
		t.Errorf("a taken login: %d", got.Code)
	}
	// An invited administrator does.
	invited := newClient(t, s, signInHost)
	carl := invite(t, database, inMain(), "carl", "Carl", true)
	invited.form("/invite/"+carl+"/password", url.Values{"password": {password}, "repeat": {password}})
	page = invited.get("/admin/sign-in")
	if page.Code != http.StatusOK || strings.Contains(page.Body.String(), `value="gate" checked`) || !strings.Contains(page.Body.String(), "Only whoever runs the instance") {
		t.Fatalf("%d", page.Code)
	}
	if got := invited.form("/admin/sign-in", url.Values{"method": {db.MethodGate, db.MethodPassword}}); got.Code != http.StatusBadRequest {
		t.Errorf("an administrator switches the gate on: %d", got.Code)
	}
	link := invited.form("/admin/invites", url.Values{"login": {"mia"}, "name": {"Mia"}})
	shown := inviteLink(link.Body.String())
	if !strings.HasPrefix(shown, "https://"+signInHost+"/invite/") {
		t.Fatalf("the link: %q", shown)
	}
	if strings.Contains(invited.get("/admin/sign-in").Body.String(), shown) {
		t.Error("the link shows again")
	}
	if got := newClient(t, s, signInHost).get(strings.TrimPrefix(shown, "https://"+signInHost)); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "Welcome, Mia") {
		t.Errorf("the link: %d", got.Code)
	}
	if !strings.Contains(invited.get("/admin/sign-in").Body.String(), `data-test="log"`) {
		t.Error("the log of sign-ins is not shown")
	}
	// Where the instance allows the gate, an administrator may switch it on.
	allowed := newSite(t, Options{DB: database, Gate: GateOn, Vault: testVault(t)}, sample)
	again := newClient(t, allowed, signInHost)
	again.cookies = invited.cookies
	if got := again.form("/admin/sign-in", url.Values{"method": {db.MethodGate, db.MethodPassword}}); got.Code != http.StatusOK {
		t.Errorf("with the gate allowed: %d", got.Code)
	}
}

// TestPasswordAndCodeNeedAKey: without a key for secrets, TOTP is not offered, since its secret would lie open.
func TestPasswordAndCodeNeedAKey(t *testing.T) {
	keyless, err := vault.New("")
	if err != nil {
		t.Fatal(err)
	}
	s, database := signInSite(t, &db.SignIn{Methods: []string{db.MethodPassword, db.MethodPasswordTOTP}}, Options{Vault: keyless})
	token := invite(t, database, inMain(), "nina", "Nina", true)
	c := newClient(t, s, signInHost)
	if page := c.get("/invite/" + token).Body.String(); strings.Contains(page, "totp_secret") {
		t.Error("TOTP is offered without a key")
	}
	const password = "a long enough password"
	c.form("/invite/"+token+"/password", url.Values{"password": {password}, "repeat": {password}})
	if got := c.form("/admin/sign-in", url.Values{"method": {db.MethodPasswordTOTP}}); got.Code != http.StatusBadRequest {
		t.Errorf("TOTP switched on without a key: %d", got.Code)
	}
	if got := c.form("/admin/sign-in", url.Values{"method": {db.MethodPassword}, "oidc_client_secret": {"x"}}); got.Code != http.StatusBadRequest {
		t.Errorf("a client secret kept without a key: %d", got.Code)
	}
}

// TestSecurityPage: a local account sets up TOTP, changes its password with the current one, and signs out elsewhere.
func TestSecurityPage(t *testing.T) {
	s, database := signInSite(t, &db.SignIn{Methods: []string{db.MethodPassword, db.MethodPasswordTOTP}}, Options{})
	token := invite(t, database, inMain(), "olga", "Olga", false)
	c := newClient(t, s, signInHost)
	const password = "a long enough password"
	c.form("/invite/"+token+"/password", url.Values{"password": {password}, "repeat": {password}})
	other := newClient(t, s, signInHost)
	other.form("/sign-in/password", url.Values{"login": {"olga"}, "password": {password}})
	if login(t, other) != "olga" {
		t.Fatal("a second session")
	}
	page := c.get("/account/security?totp=new").Body.String()
	secret := between(page, `name="totp_secret" value="`, `"`)
	if secret == "" {
		t.Fatalf("no setup: %s", page)
	}
	if got := c.form("/account/totp", url.Values{"totp_secret": {secret}, "code": {"000000"}}); got.Code != http.StatusBadRequest {
		t.Errorf("a wrong code: %d", got.Code)
	}
	code, _ := totp.GenerateCode(secret, time.Now())
	if got := c.form("/account/totp", url.Values{"totp_secret": {secret}, "code": {code}}); got.Code != http.StatusOK {
		t.Fatalf("set up: %d", got.Code)
	}
	if got := newClient(t, s, signInHost).form("/sign-in/password", url.Values{"login": {"olga"}, "password": {password}}); got.Code != http.StatusUnauthorized {
		t.Errorf("without the code now: %d", got.Code)
	}
	if got := c.form("/account/password", url.Values{"current": {"not the password"}, "password": {"another long password"}, "repeat": {"another long password"}}); got.Code != http.StatusBadRequest {
		t.Errorf("without the current password: %d", got.Code)
	}
	if got := c.form("/account/password", url.Values{"current": {password}, "password": {"another long password"}, "repeat": {"another long password"}}); got.Code != http.StatusOK {
		t.Errorf("change: %d", got.Code)
	}
	if got := c.form("/account/sessions/end", nil); got.Code != http.StatusOK {
		t.Errorf("sign out elsewhere: %d", got.Code)
	}
	if login(t, other) != "" || login(t, c) != "olga" {
		t.Error("only the other session ends")
	}
	if got := c.form("/account/totp/remove", nil); got.Code != http.StatusOK {
		t.Errorf("remove TOTP where a password alone is allowed: %d", got.Code)
	}
	gate := newClientWith(t, newSite(t, Options{DB: database, Gate: GateOn, Vault: testVault(t)}, sample), gateHeaders)
	if err := database.SetSignIn(inMain(), db.SignIn{Methods: []string{db.MethodGate, db.MethodPassword}}); err != nil {
		t.Fatal(err)
	}
	if page := gate.get("/account/security").Body.String(); !strings.Contains(page, `data-test="managed"`) {
		t.Errorf("an account of the gate manages its sign-in there: %s", page)
	}
	if got := gate.form("/account/password", url.Values{"password": {"another long password"}, "repeat": {"another long password"}}); got.Code != http.StatusForbidden {
		t.Errorf("an account of the gate sets a password: %d", got.Code)
	}
}

// TestSessionsStayWithTheirTenant: the cookie of a session in one tenant, sent to the host of another (which a browser
// never does, as the cookie belongs to its host), signs nobody in there, also for the same login.
func TestSessionsStayWithTheirTenant(t *testing.T) {
	s, database := withTenants(t)
	acmeCtx := tenant.With(context.Background(), "acme")
	for _, ctx := range []context.Context{inMain(), acmeCtx} {
		if err := database.SetSignIn(ctx, db.SignIn{Methods: []string{db.MethodPassword}}); err != nil {
			t.Fatal(err)
		}
	}
	const password = "a long enough password"
	for _, c := range []struct {
		ctx  context.Context
		host string
	}{{inMain(), mainHost}, {acmeCtx, acmeHost}} {
		token := invite(t, database, c.ctx, "vera", "Vera", true)
		newClient(t, s, c.host).form("/invite/"+token+"/password", url.Values{"password": {password}, "repeat": {password}})
	}
	inAcme := newClient(t, s, acmeHost)
	inAcme.form("/sign-in/password", url.Values{"login": {"vera"}, "password": {password}})
	if login(t, inAcme) != "vera" {
		t.Fatal("signed in at acme")
	}
	carried := newClient(t, s, mainHost)
	carried.cookies[sessionCookie] = inAcme.cookies[sessionCookie]
	if who := login(t, carried); who != "" {
		t.Errorf("acme's session signs in on Main as %q", who)
	}
	if got := carried.get("/admin/sign-in"); got.Code == http.StatusOK {
		t.Error("acme's administrator administers Main")
	}
}

// TestTheSignInIsC22sCard: with a password, the card itself is the form, the passkey above it set apart by "or", the
// button in its footer and registration as the action of its header (C22, blocks/login-signup).
func TestTheSignInIsC22sCard(t *testing.T) {
	s, _ := signInSite(t, &db.SignIn{Methods: []string{db.MethodPasskey, db.MethodPassword}, Registration: true}, Options{})
	page := newClient(t, s, signInHost).get("/sign-in").Body.String()
	for _, want := range []string{
		`<form class="card w-full max-w-sm" method="post" action="/sign-in/password" data-test="signin-card">`,
		`<div class="field-separator"><hr role="separator"><span>or</span></div>`,
		`<div class="card-action"><a class="btn" data-variant="link" href="/register">Register</a></div>`,
		`<footer class="flex-col gap-2">`, `data-passkey-sign-in`, `type="button" data-passkey-sign-in`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the sign-in lacks %s", want)
		}
	}
	if strings.Index(page, "data-passkey-sign-in") > strings.Index(page, `id="login"`) {
		t.Error("the passkey comes after the password")
	}
}
