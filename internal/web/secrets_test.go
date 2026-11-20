// SPDX-License-Identifier: MIT

package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/folder"
	"github.com/Ollornog/UnderDoc/internal/library"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
)

// lockedBuffer collects what the process logs, for a test that reads it while requests write it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// testKey makes a fresh key for the vault of a test.
func testKey(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// testValue makes a password for a test, so none is written down anywhere.
func testValue(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return "pw-" + base64.RawURLEncoding.EncodeToString(raw)
}

// withVault is a site with the database and a vault under key, whose process log the test reads.
func withVault(t *testing.T, key string) (*site, *lockedBuffer, *db.DB) {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	behindGate(t, database, tenant.Main)
	keys, err := vault.New(key)
	if err != nil {
		t.Fatal(err)
	}
	logged := &lockedBuffer{}
	s := &site{t: t, dir: t.TempDir()}
	log := slog.New(slog.NewJSONHandler(logged, &slog.HandlerOptions{Level: slog.LevelDebug}))
	s.store = library.NewStore(folder.New(s.dir), log)
	s.refresh()
	if s.http, err = New(s.store, log, Options{DB: database, Vault: keys}); err != nil {
		t.Fatal(err)
	}
	return s, logged, database
}

// newSecret makes a secret on a page as anna and returns its ID.
func (s *site) newSecret(page, value string, headers map[string]string) string {
	s.t.Helper()
	body, _ := json.Marshal(map[string]string{"value": value})
	made := s.send(http.MethodPost, "/api/pages/"+page+"/secrets", string(body), headers)
	if made.Code != http.StatusCreated {
		s.t.Fatalf("make a secret = %d %s", made.Code, made.Body.String())
	}
	if strings.Contains(made.Body.String(), value) {
		s.t.Errorf("making a secret answered its value: %s", made.Body.String())
	}
	out := decode[map[string]string](s.t, made)
	if out["ref"] != `::secret{id="`+out["id"]+`"}` {
		s.t.Errorf("the reference: %+v", out)
	}
	return out["id"]
}

func (s *site) reveal(secretID, purpose string, headers map[string]string) (int, string) {
	s.t.Helper()
	got := s.send(http.MethodPost, "/api/secrets/"+secretID+"/reveal", `{"purpose":"`+purpose+`"}`, headers)
	if got.Code != http.StatusOK {
		return got.Code, ""
	}
	if cache := got.Header().Get("Cache-Control"); cache != "no-store" {
		s.t.Errorf("a revealed value may be cached: %q", cache)
	}
	return got.Code, decode[map[string]string](s.t, got)["value"]
}

func (s *site) newPageWith(parent, title string, blocks ...string) string {
	s.t.Helper()
	p := decode[apiNode](s.t, s.send(http.MethodPost, "/api/pages", `{"title":"`+title+`","parent":"`+parent+`"}`, sameSite))
	after := ""
	for _, md := range blocks {
		body, _ := json.Marshal(map[string]string{"md": md, "after": after})
		after = decode[apiBlock](s.t, s.send(http.MethodPost, "/api/pages/"+p.ID+"/blocks", string(body), sameSite)).ID
	}
	return p.ID
}

func (s *site) docHTML(page string, headers map[string]string) string {
	s.t.Helper()
	got := s.getWith("/api/doc/"+page, headers)
	if got.Code != http.StatusOK {
		s.t.Fatalf("doc %s = %d", page, got.Code)
	}
	var html strings.Builder
	for _, b := range decode[apiDocument](s.t, got).Blocks {
		html.WriteString(b.HTML)
	}
	return html.String()
}

// TestAPasswordInTheText: a page shows a mask with Reveal and Copy, the value comes on a click only, from this
// application, with no-store; a reader left out of a restriction sees "hidden", a reference on another page "not
// available here", the same as an ID that names nothing; embedding grants nothing (ADR-7).
func TestAPasswordInTheText(t *testing.T) {
	s, _, _ := withVault(t, testKey(t))
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := s.newPageWith(team.ID, "Router")
	value := testValue(t)
	secretID := s.newSecret(page, value, sameSite)
	body, _ := json.Marshal(map[string]string{"md": "Admin: " + SecretRef(secretID), "after": ""})
	if got := s.send(http.MethodPost, "/api/pages/"+page+"/blocks", string(body), sameSite); got.Code != http.StatusCreated && got.Code != http.StatusOK {
		t.Fatalf("block = %d %s", got.Code, got.Body.String())
	}
	html := s.docHTML(page, gateHeaders)
	if !strings.Contains(html, `data-secret-state="ok"`) || !strings.Contains(html, `data-pwact="show" data-secret="`+secretID+`"`) {
		t.Errorf("the page shows no mask to reveal: %s", html)
	}
	if doc := s.get("/api/doc/" + page).Body.String(); strings.Contains(doc, value) {
		t.Errorf("the document carries the value: %s", doc)
	}
	if code, got := s.reveal(secretID, "show", sameSite); code != http.StatusOK || got != value {
		t.Fatalf("reveal = %d", code)
	}
	if code, got := s.reveal(secretID, "copy", plain); code != http.StatusOK || got != value {
		t.Errorf("bob, who reads the page, copies = %d", code)
	}
	if code, _ := s.reveal(secretID, "show", nil); code != http.StatusForbidden {
		t.Errorf("a reveal from another site = %d", code)
	}
	if code, _ := s.reveal(secretID, "print", sameSite); code != http.StatusBadRequest {
		t.Errorf("an unknown purpose = %d", code)
	}
	if got := s.send(http.MethodGet, "/api/secrets/"+secretID+"/reveal", "", sameSite); got.Code == http.StatusOK || strings.Contains(got.Body.String(), value) {
		t.Errorf("a reveal by GET = %d", got.Code)
	}
	// Restricted to anna: bob reads the page and sees "hidden", and his reveal looks like one of nothing.
	me := decode[map[string]any](t, s.get("/api/state"))["user"].(map[string]any)["id"].(string)
	if got := s.send(http.MethodPut, "/api/secrets/"+secretID+"/readers", `{"restricted":false,"readers":[{"kind":"person","who":"`+me+`"}]}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("readers without a restriction = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/secrets/"+secretID+"/readers", `{"restricted":true,"readers":[{"kind":"person","who":"`+me+`"}]}`, plain).Code; got != http.StatusForbidden {
		t.Errorf("bob, who edits but does not manage, restricts = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/secrets/"+secretID+"/readers", `{"restricted":true,"readers":[{"kind":"person","who":"`+me+`"}]}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("restrict = %d", got)
	}
	if html := s.docHTML(page, plain); !strings.Contains(html, `data-secret-state="hidden"`) || strings.Contains(html, secretID) {
		t.Errorf("bob sees more than hidden: %s", html)
	}
	if code, _ := s.reveal(secretID, "show", plain); code != http.StatusNotFound {
		t.Errorf("bob reveals a restricted secret = %d", code)
	}
	if code, _ := s.reveal("zzzzzzzzzz", "show", plain); code != http.StatusNotFound {
		t.Errorf("an ID of nothing = %d", code)
	}
	if got := s.send(http.MethodPut, "/api/secrets/"+secretID, `{"value":"x"}`, plain).Code; got != http.StatusNotFound {
		t.Errorf("bob changes a restricted secret = %d", got)
	}
	// The same reference on another page shows nothing, also to anna; an embedding of the block shows it to anna.
	other := s.newPageWith(team.ID, "Copy", "Pasted: "+SecretRef(secretID), "Pasted: "+SecretRef("zzzzzzzzzz"), "::embed{ref=\""+page+"#b1\"}")
	html = s.docHTML(other, gateHeaders)
	if strings.Count(html, `data-secret-state="none"`) != 2 {
		t.Errorf("a reference outside its page, and one of nothing, show alike: %s", html)
	}
	if !strings.Contains(html, `data-pwact="show" data-secret="`+secretID+`"`) {
		t.Errorf("the embedded block shows no mask to anna: %s", html)
	}
	if html := s.docHTML(other, plain); strings.Contains(html, `data-secret-state="ok"`) || strings.Contains(html, secretID) {
		t.Errorf("an embedding lent bob the secret: %s", html)
	}
	// Changed, the old value is a version; anna sees the versions, and restores.
	newValue := testValue(t)
	if got := s.send(http.MethodPut, "/api/secrets/"+secretID, `{"value":"`+newValue+`"}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("change = %d", got)
	}
	if _, got := s.reveal(secretID, "show", sameSite); got != newValue {
		t.Error("the change did not take")
	}
	info := s.get("/api/secrets/" + secretID)
	if strings.Contains(info.Body.String(), value) || strings.Contains(info.Body.String(), newValue) || len(decode[apiSecret](t, info).Versions) != 2 {
		t.Errorf("the secret: %s", info.Body.String())
	}
	old := s.send(http.MethodPost, "/api/secrets/"+secretID+"/reveal", `{"purpose":"show","version":1}`, sameSite)
	if old.Code != http.StatusOK || decode[map[string]string](t, old)["value"] != value {
		t.Errorf("version 1 = %d", old.Code)
	}
	if got := s.send(http.MethodPost, "/api/secrets/"+secretID+"/versions/1/restore", "", sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("restore = %d", got)
	}
	if _, got := s.reveal(secretID, "show", sameSite); got != value {
		t.Error("the restore did not take")
	}
	if list := s.get("/api/pages/" + page + "/secrets").Body.String(); strings.Contains(list, value) || !strings.Contains(list, secretID) {
		t.Errorf("the list of secrets: %s", list)
	}
	if got := s.send(http.MethodDelete, "/api/secrets/"+secretID, "", sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("delete = %d", got)
	}
	if code, _ := s.reveal(secretID, "show", sameSite); code != http.StatusNotFound {
		t.Errorf("a deleted secret = %d", code)
	}
}

// TestAPasswordLeavesByNoOtherWay: the value is in no answer but the reveal: not in the document, the versions of the
// page, the export, the search, a frozen link, the log of access, or the log of the process; and no frozen link names its
// ID (ADR-7).
func TestAPasswordLeavesByNoOtherWay(t *testing.T) {
	s, logged, _ := withVault(t, testKey(t))
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := s.newPageWith(team.ID, "Router", "Network: switches.")
	value := testValue(t)
	secretID := s.newSecret(page, value, sameSite)
	if got := s.send(http.MethodPut, "/api/pages/"+page+"/blocks/b1", `{"md":"Admin: `+strings.ReplaceAll(SecretRef(secretID), `"`, `\"`)+`"}`, sameSite).Code; got != http.StatusOK {
		t.Fatalf("edit = %d", got)
	}
	if code, got := s.reveal(secretID, "show", sameSite); code != http.StatusOK || got != value {
		t.Fatalf("reveal = %d", code)
	}
	if got := s.send(http.MethodPut, "/api/secrets/"+secretID, `{"value":"`+testValue(t)+`"}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("change = %d", got)
	}
	made := s.send(http.MethodPost, "/api/objects/"+page+"/links", `{"mode":"read"}`, sameSite)
	if made.Code != http.StatusCreated {
		t.Fatalf("link = %d %s", made.Code, made.Body.String())
	}
	frozen := s.anonymous(http.MethodGet, pathOf(t, decode[apiLink](t, made).URL), nil, nil).Body.String()
	if !strings.Contains(frozen, "Password not shared") || strings.Contains(frozen, secretID) {
		t.Errorf("the frozen page: %s", frozen)
	}
	ways := map[string]string{
		"the document":           s.get("/api/doc/" + page).Body.String(),
		"the versions":           s.get("/api/pages/" + page + "/versions").Body.String(),
		"the export":             strings.Join(mapValues(unzip(t, s.get("/api/objects/"+team.ID+"/export").Body.Bytes())), "\n"),
		"the search":             s.get("/api/search?q=" + url.QueryEscape(value)).Body.String(),
		"the frozen link":        frozen,
		"the log of access":      s.get("/api/log").Body.String(),
		"the secrets":            s.get("/api/pages/" + page + "/secrets").Body.String(),
		"the secret":             s.get("/api/secrets/" + secretID).Body.String(),
		"the list of problems":   s.get("/api/problems").Body.String(),
		"the log of the process": logged.String(),
	}
	for way, text := range ways {
		if strings.Contains(text, value) {
			t.Errorf("the value leaves through %s", way)
		}
	}
	if !strings.Contains(ways["the export"], SecretRef(secretID)) {
		t.Errorf("the export lost the reference: %s", ways["the export"])
	}
	if !strings.Contains(ways["the log of access"], db.LogSecretRevealed) {
		t.Errorf("the reveal is not in the log: %s", ways["the log of access"])
	}
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// TestPasswordsNeedTheKey: without a key nothing is made or revealed, and a page says so; with a wrong key nothing opens
// and nothing falls back to plain text.
func TestPasswordsNeedTheKey(t *testing.T) {
	key := testKey(t)
	s, _, database := withVault(t, key)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := s.newPageWith(team.ID, "Router")
	value := testValue(t)
	secretID := s.newSecret(page, value, sameSite)

	for name, keys := range map[string]string{"no key": "", "another key": testKey(t)} {
		v, err := vault.New(keys)
		if err != nil {
			t.Fatal(err)
		}
		other := &site{t: t, dir: t.TempDir()}
		log := slog.New(slog.DiscardHandler)
		other.store = library.NewStore(folder.New(other.dir), log)
		other.refresh()
		if other.http, err = New(other.store, log, Options{DB: database, Vault: v}); err != nil {
			t.Fatal(err)
		}
		want := http.StatusInternalServerError
		if keys == "" {
			want = http.StatusServiceUnavailable
		}
		got := other.send(http.MethodPost, "/api/secrets/"+secretID+"/reveal", `{"purpose":"show"}`, sameSite)
		if got.Code != want || strings.Contains(got.Body.String(), value) {
			t.Errorf("%s: reveal = %d %s", name, got.Code, got.Body.String())
		}
		if keys == "" {
			if got := other.send(http.MethodPost, "/api/pages/"+page+"/secrets", `{"value":"x"}`, sameSite); got.Code != http.StatusServiceUnavailable {
				t.Errorf("no key: make = %d", got.Code)
			}
			if got := decode[map[string]any](t, other.get("/api/pages/"+page+"/secrets")); got["vault"] != false {
				t.Errorf("no key: the list does not say so: %+v", got)
			}
		}
	}
}

// TestRevealingIsLimited: an account reveals so many passwords a minute, then waits; another account is not held back.
func TestRevealingIsLimited(t *testing.T) {
	s, _, _ := withVault(t, testKey(t))
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := s.newPageWith(team.ID, "Router")
	secretID := s.newSecret(page, testValue(t), sameSite)
	for i := range db.RevealsPerWindow {
		if code, _ := s.reveal(secretID, "copy", sameSite); code != http.StatusOK {
			t.Fatalf("reveal %d = %d", i+1, code)
		}
	}
	got := s.send(http.MethodPost, "/api/secrets/"+secretID+"/reveal", `{"purpose":"show"}`, sameSite)
	if got.Code != http.StatusTooManyRequests || got.Header().Get("Retry-After") == "" {
		t.Errorf("one more = %d %v", got.Code, got.Header())
	}
	if code, _ := s.reveal(secretID, "show", plain); code != http.StatusOK {
		t.Errorf("bob is held back by anna's reveals: %d", code)
	}
}

// TestTheLimitOfRevealsHoldsAcrossInstances: two instances behind one address share the database, and a third starts
// later. Half the reveals of a minute through each fill the limit, and neither of them, nor the new one, gives one more,
// though each has counted only its own in memory (ADR-7).
func TestTheLimitOfRevealsHoldsAcrossInstances(t *testing.T) {
	key := testKey(t)
	s, _, database := withVault(t, key)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := s.newPageWith(team.ID, "Router")
	secretID := s.newSecret(page, testValue(t), sameSite)
	instance := func() *site {
		keys, err := vault.New(key)
		if err != nil {
			t.Fatal(err)
		}
		other := &site{t: t, dir: s.dir, store: s.store}
		if other.http, err = New(s.store, slog.New(slog.DiscardHandler), Options{DB: database, Vault: keys}); err != nil {
			t.Fatal(err)
		}
		return other
	}
	second := instance()
	for i := range db.RevealsPerWindow {
		through := s
		if i%2 == 1 {
			through = second
		}
		if code, _ := through.reveal(secretID, "show", sameSite); code != http.StatusOK {
			t.Fatalf("reveal %d = %d", i+1, code)
		}
	}
	for name, through := range map[string]*site{"the first": s, "the second": second, "one started later": instance()} {
		if got := through.send(http.MethodPost, "/api/secrets/"+secretID+"/reveal", `{"purpose":"copy"}`, sameSite); got.Code != http.StatusTooManyRequests {
			t.Errorf("one more through %s instance = %d", name, got.Code)
		}
	}
	if code, _ := second.reveal(secretID, "show", plain); code != http.StatusOK {
		t.Errorf("bob is held back by anna's reveals: %d", code)
	}
}
