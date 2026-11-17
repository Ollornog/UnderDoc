// SPDX-License-Identifier: MIT

package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/folder"
	"github.com/Ollornog/UnderDoc/internal/library"
)

// provider is an OIDC identity provider for tests: discovery, keys, and a token endpoint that checks PKCE and the client
// secret and hands out the ID token the test asks for.
type provider struct {
	t      *testing.T
	server *httptest.Server
	key    *rsa.PrivateKey
	mu     sync.Mutex
	codes  map[string]grant
	// secret is the client secret the token endpoint wants, if any; seen is the last one it got.
	secret, seen string
}

type grant struct {
	challenge string
	claims    map[string]any
}

const clientID = "underdoc-test"

func newProvider(t *testing.T) *provider {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &provider{t: t, key: key, codes: map[string]grant{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": p.server.URL, "authorization_endpoint": p.server.URL + "/authorize",
			"token_endpoint": p.server.URL + "/token", "jwks_uri": p.server.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"},
			"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		p.mu.Lock()
		g, ok := p.codes[r.PostFormValue("code")]
		delete(p.codes, r.PostFormValue("code"))
		_, p.seen, _ = r.BasicAuth()
		if p.seen == "" {
			p.seen = r.PostFormValue("client_secret")
		}
		want := p.secret
		p.mu.Unlock()
		sum := sha256.Sum256([]byte(r.PostFormValue("code_verifier")))
		if !ok || b64.EncodeToString(sum[:]) != g.challenge || want != "" && p.seen != want {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 60, "id_token": p.sign(g.claims)})
	})
	p.server = httptest.NewTLSServer(mux)
	t.Cleanup(p.server.Close)
	return p
}

func (p *provider) sign(claims map[string]any) string {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: p.key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
	if err != nil {
		p.t.Fatal(err)
	}
	payload, _ := json.Marshal(claims)
	signed, err := signer.Sign(payload)
	if err != nil {
		p.t.Fatal(err)
	}
	out, err := signed.CompactSerialize()
	if err != nil {
		p.t.Fatal(err)
	}
	return out
}

// authorize plays the provider's sign-in page: it takes the request the app sent the browser to, and returns the code
// and state the browser brings back. change can alter the claims of the ID token.
func (p *provider) authorize(location string, change func(claims map[string]any)) url.Values {
	p.t.Helper()
	u, err := url.Parse(location)
	if err != nil || !strings.HasPrefix(location, p.server.URL+"/authorize?") {
		p.t.Fatalf("not to the provider: %s", location)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("client_id") != clientID || q.Get("nonce") == "" || q.Get("state") == "" {
		p.t.Fatalf("the request lacks PKCE, nonce or state: %s", location)
	}
	claims := map[string]any{"iss": p.server.URL, "aud": clientID, "sub": "s-1", "iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(),
		"nonce": q.Get("nonce"), "preferred_username": "ines", "name": "Ines Ray", "email": "ines@example.com", "groups": []string{"staff", "wiki-admins"}}
	if change != nil {
		change(claims)
	}
	code := "c" + q.Get("state")[:8]
	p.mu.Lock()
	p.codes[code] = grant{challenge: q.Get("code_challenge"), claims: claims}
	p.mu.Unlock()
	return url.Values{"code": {code}, "state": {q.Get("state")}}
}

func oidcSite(t *testing.T) (*site, *db.DB, *provider) {
	t.Helper()
	p := newProvider(t)
	s, database := signInSite(t, &db.SignIn{Methods: []string{db.MethodOIDC}, OIDCName: "Example ID", OIDCIssuer: p.server.URL,
		OIDCClientID: clientID, OIDCGroupsClaim: "groups", AdminGroup: "wiki-admins"}, Options{HTTPClient: p.server.Client()})
	return s, database, p
}

func TestOIDC(t *testing.T) {
	s, database, p := oidcSite(t)
	start := func(c *client) string {
		got := c.get("/sign-in/oidc?next=/o/start")
		if got.Code != http.StatusSeeOther {
			t.Fatalf("start: %d %s", got.Code, got.Body.String())
		}
		return got.Header().Get("Location")
	}
	c := newClient(t, s, signInHost)
	if page := c.get("/sign-in").Body.String(); !strings.Contains(page, "Sign in with Example ID") {
		t.Errorf("no button: %s", page)
	}
	back := p.authorize(start(c), nil)
	got := c.get("/sign-in/oidc/callback?" + back.Encode())
	if got.Code != http.StatusSeeOther || got.Header().Get("Location") != "/o/start" {
		t.Fatalf("callback: %d %q %s", got.Code, got.Header().Get("Location"), got.Body.String())
	}
	state := decode[apiState](t, c.get("/api/state"))
	if state.User.Login != "ines" || state.User.Name != "Ines Ray" || !state.User.Admin || state.User.Method != db.MethodOIDC ||
		strings.Join(state.User.Groups, ",") != "staff,wiki-admins" {
		t.Errorf("the account from the ID token: %+v", state.User)
	}
	refused := func(name string, c *client, query string) {
		t.Helper()
		got := c.get("/sign-in/oidc/callback?" + query)
		if got.Code != http.StatusUnauthorized || login(t, c) != "" {
			t.Errorf("%s: %d", name, got.Code)
		}
	}
	// The state must be the one of this browser.
	other := newClient(t, s, signInHost)
	back = p.authorize(start(other), nil)
	back.Set("state", "forged")
	refused("forged state", other, back.Encode())
	// A callback without the browser's ceremony, as a link someone else sends: login CSRF.
	victim := newClient(t, s, signInHost)
	refused("no ceremony", victim, p.authorize(start(newClient(t, s, signInHost)), nil).Encode())
	// An ID token with another nonce, for another client, or from another issuer.
	for name, change := range map[string]func(map[string]any){
		"nonce":    func(c map[string]any) { c["nonce"] = "other" },
		"audience": func(c map[string]any) { c["aud"] = "someone-else" },
		"issuer":   func(c map[string]any) { c["iss"] = "https://evil.example" },
		"expired":  func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() },
		"no name":  func(c map[string]any) { delete(c, "preferred_username") },
		"spaces":   func(c map[string]any) { c["preferred_username"] = "a b" },
	} {
		c := newClient(t, s, signInHost)
		refused(name, c, p.authorize(start(c), change).Encode())
	}
	// A code works once.
	c2 := newClient(t, s, signInHost)
	back = p.authorize(start(c2), nil)
	c2.get("/sign-in/oidc/callback?" + back.Encode())
	c3 := newClient(t, s, signInHost)
	start(c3)
	c3.cookies[ceremonyCookie] = c2.cookies[ceremonyCookie]
	refused("the same code again", c3, back.Encode())
	// A local account of the same name is never taken over.
	if err := database.SetSignIn(inMain(), db.SignIn{Methods: []string{db.MethodOIDC, db.MethodPassword}, OIDCIssuer: p.server.URL,
		OIDCClientID: clientID, OIDCGroupsClaim: "groups"}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.AcceptInvite(inMain(), mustInviteHash(t, database, "jo"), db.NewLocal{PasswordHash: "x"}); err != nil {
		t.Fatal(err)
	}
	c4 := newClient(t, s, signInHost)
	refused("a local account's name", c4, p.authorize(start(c4), func(c map[string]any) { c["preferred_username"] = "jo" }).Encode())
	events, _ := database.SignInLog(inMain(), 1)
	if len(events) != 1 || events[0].Login != "jo" || events[0].OK {
		t.Errorf("%+v", events)
	}
}

// TestTheClientSecretIsWrittenOnly: the client secret of OIDC is set on the admin page, used at the provider, and never
// shown on a page or written to the log.
func TestTheClientSecretIsWrittenOnly(t *testing.T) {
	p := newProvider(t)
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	behindGate(t, database, "main")
	var logged bytes.Buffer
	dir := t.TempDir()
	store := library.NewStore(folder.New(dir), slog.New(slog.DiscardHandler))
	if err := store.Refresh(); err != nil {
		t.Fatal(err)
	}
	handler, err := New(store, slog.New(slog.NewJSONHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Options{DB: database, Vault: testVault(t), HTTPClient: p.server.Client(), AdminGroup: "underdoc-admins"})
	if err != nil {
		t.Fatal(err)
	}
	s := &site{t: t, dir: dir, store: store, http: handler}
	admin := newClientWith(t, s, gateHeaders)
	const secret = "dummy-client-value-for-tests"
	p.secret = secret
	save := url.Values{"method": {db.MethodGate, db.MethodOIDC}, "oidc_name": {"Example ID"}, "oidc_issuer": {p.server.URL},
		"oidc_client_id": {clientID}, "oidc_groups_claim": {"groups"}, "oidc_client_secret": {secret}}
	if got := admin.form("/admin/sign-in", save); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "Saved.") {
		t.Fatalf("save: %d %s", got.Code, got.Body.String())
	}
	page := admin.get("/admin/sign-in").Body.String()
	if strings.Contains(page, secret) || !strings.Contains(page, "(kept, type to replace)") {
		t.Errorf("the page shows the secret, or not that it is kept")
	}
	sealed, err := database.SealedSecret(inMain(), oidcSecret)
	if err != nil || bytes.Contains(sealed, []byte(secret)) {
		t.Errorf("the secret is not sealed: %v", err)
	}
	c := newClient(t, s, signInHost)
	back := p.authorize(c.get("/sign-in/oidc").Header().Get("Location"), nil)
	if got := c.get("/sign-in/oidc/callback?" + back.Encode()); got.Code != http.StatusSeeOther || p.seen != secret {
		t.Errorf("the provider did not get the secret: %d, got one: %v", got.Code, p.seen != "")
	}
	// Saving again without typing it keeps it; forgetting removes it.
	save.Del("oidc_client_secret")
	admin.form("/admin/sign-in", save)
	if _, err := database.SealedSecret(inMain(), oidcSecret); err != nil {
		t.Errorf("an empty field forgot the secret: %v", err)
	}
	save.Set("oidc_forget_secret", "on")
	admin.form("/admin/sign-in", save)
	if _, err := database.SealedSecret(inMain(), oidcSecret); err == nil {
		t.Error("the secret is still kept")
	}
	if strings.Contains(logged.String(), secret) {
		t.Errorf("the log holds the secret: %s", logged.String())
	}
	if !strings.Contains(logged.String(), `"msg":"sign-in"`) {
		t.Error("the sign-in was not logged at all, so the check above proves nothing")
	}
}
