// SPDX-License-Identifier: MIT

//go:build gate

package gateapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tinysesam "github.com/Ollornog/TinySesam/clients/go"

	"github.com/Ollornog/UnderDoc/internal/gateapi"
)

// The contract test against a real TinySesam (scripts/check-services.sh starts the published image, pinned by digest,
// with scripts/gate/gate.py, which configures the app underdoc as docs/accounts.md tells an operator to). It proves what
// the stand-in of package gatetest only assumes: the account API, the reset of access and the confirmations of the
// app's actions, checked by the same verifier UnderDoc runs. Without the gate this test fails: a skipped test is not a
// green one.

// facts are what scripts/gate/gate.py made for this run, all made up.
type facts struct {
	Key           string            `json:"key"`
	Password      string            `json:"password"`
	Lead          int               `json:"lead"`
	Boss          int               `json:"boss"`
	Outsider      int               `json:"outsider"`
	Adopted       int               `json:"adopted"`
	SessionCookie string            `json:"session_cookie"`
	Sessions      map[string]string `json:"sessions"`
}

func realGate(t *testing.T) (*gateapi.Client, facts, string) {
	t.Helper()
	base, path := os.Getenv("UNDERDOC_TEST_GATE_URL"), os.Getenv("UNDERDOC_TEST_GATE_FACTS")
	if base == "" || path == "" {
		t.Fatal("UNDERDOC_TEST_GATE_URL and UNDERDOC_TEST_GATE_FACTS are not set: run scripts/check-services.sh")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f facts
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(t.TempDir(), "service.key")
	if err := os.WriteFile(keyFile, []byte(f.Key+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := gateapi.New(gateapi.Config{URL: base, KeyFile: keyFile})
	if err != nil {
		t.Fatal(err)
	}
	return c, f, base
}

// browser is a person's session in the gate, for what only a person does there: confirming, the first sign-in.
type browser struct {
	t    *testing.T
	http *http.Client
	base string
}

func personIn(t *testing.T, base string, f facts, who string) *browser {
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse(base)
	if who != "" {
		jar.SetCookies(u, []*http.Cookie{{Name: f.SessionCookie, Value: f.Sessions[who], Path: "/"}})
	}
	return &browser{t: t, base: base, http: &http.Client{Jar: jar, Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (b *browser) get(target string) string {
	b.t.Helper()
	resp, err := b.http.Get(target)
	if err != nil {
		b.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		b.t.Fatalf("GET %s = %d", target, resp.StatusCode)
	}
	return string(body)
}

func (b *browser) post(target string, form url.Values) *http.Response {
	b.t.Helper()
	resp, err := b.http.PostForm(target, form)
	if err != nil {
		b.t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp
}

var (
	csrfField    = regexp.MustCompile(`name=_csrf value='([^']+)'`)
	payloadField = regexp.MustCompile(`name=payload_hash value='([^']+)'`)
	formAction   = regexp.MustCompile(`action='([^']+)'`)
)

// confirm confirms an approval on the gate's page as the person of the session, with the step-up of their password.
func (b *browser) confirm(approveURL, password string) *http.Response {
	b.t.Helper()
	page := b.get(approveURL)
	csrf, hash := csrfField.FindStringSubmatch(page), payloadField.FindStringSubmatch(page)
	if csrf == nil || hash == nil {
		b.t.Fatalf("the approval page has no form: %.300s", page)
	}
	return b.post(approveURL, url.Values{"_csrf": {csrf[1]}, "decision": {"approve"}, "payload_hash": {hash[1]}, "password": {password}})
}

// signIn takes a sign-in link as its person does, the first sign-in of the account.
func (b *browser) signIn(link string) {
	b.t.Helper()
	page := b.get(link)
	csrf, action := csrfField.FindStringSubmatch(page), formAction.FindStringSubmatch(page)
	if csrf == nil || action == nil {
		b.t.Fatalf("the sign-in link has no form: %.300s", page)
	}
	if resp := b.post(b.base+action[1], url.Values{"_csrf": {csrf[1]}}); resp.StatusCode != http.StatusSeeOther {
		b.t.Fatalf("the sign-in = %d", resp.StatusCode)
	}
}

// TestTheAccountAPIOfARealGate: what UnderDoc asks of the account API, as TinySesam answers it.
func TestTheAccountAPIOfARealGate(t *testing.T) {
	c, f, base := realGate(t)
	ctx := context.Background()
	lead := strconv.Itoa(f.Lead)
	p, err := c.Provisioning(ctx)
	if err != nil || p.App != "underdoc" || p.ApproverRole != "underdoc-admin" || !p.Has(gateapi.ScopeReset) ||
		len(p.Roles) != 1 || p.Roles[0] != "underdoc-user" {
		t.Fatalf("provisioning: %+v %v", p, err)
	}
	made, err := c.Create(ctx, lead, gateapi.NewAccount{Username: "lena", DisplayName: "Lena M.", Email: "lena@example.org", Roles: []string{"underdoc-user"}})
	if err != nil || !gateapi.ID(made.Account.ID) || !strings.HasPrefix(made.Link.URL, base+"/") || made.Link.ExpiresAt <= time.Now().Unix() ||
		made.Account.SignedIn() || made.Account.State != gateapi.StateActive || made.Account.DisplayName != "Lena M." {
		t.Fatalf("invite: %+v %v", made, err)
	}
	for name, try := range map[string]gateapi.NewAccount{
		"the administrator's role": {Username: "otto", Roles: []string{"underdoc-admin"}},
		"the delegate's role":      {Username: "otto", Roles: []string{"underdoc-delegate"}},
		"no role":                  {Username: "otto", Roles: []string{}},
	} {
		if _, err := c.Create(ctx, lead, try); !errors.Is(err, gateapi.ErrRoleNotAllowed) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := c.Create(ctx, lead, gateapi.NewAccount{Username: "lena", Roles: []string{"underdoc-user"}}); !errors.Is(err, gateapi.ErrNameTaken) {
		t.Errorf("a login taken: %v", err)
	}
	if a, err := c.Disable(ctx, lead, made.Account.ID); err != nil || a.State != gateapi.StateDisabled {
		t.Errorf("lock: %+v %v", a, err)
	}
	if _, err := c.SigninLink(ctx, lead, made.Account.ID); !errors.Is(err, gateapi.ErrDisabled) {
		t.Errorf("a link for a locked account: %v", err)
	}
	if a, err := c.Enable(ctx, gateapi.NoPerson, made.Account.ID); err != nil || a.State != gateapi.StateActive {
		t.Errorf("unlock as a job: %+v %v", a, err)
	}
	link, err := c.SigninLink(ctx, lead, made.Account.ID)
	if err != nil || link.URL == "" || link.URL == made.Link.URL {
		t.Fatalf("a new link: %+v %v", link, err)
	}
	// The person takes the new link: the first sign-in. After it there is no new link, only a reset.
	personIn(t, base, f, "").signIn(link.URL)
	if a, err := c.Get(ctx, made.Account.ID); err != nil || !a.SignedIn() {
		t.Errorf("after the first sign-in: %+v %v", a, err)
	}
	if _, err := c.SigninLink(ctx, lead, made.Account.ID); !errors.Is(err, gateapi.ErrSignedIn) {
		t.Errorf("a link after the first sign-in: %v", err)
	}
	// Only what UnderDoc manages: an account of the operator is as if it were not there; one the operator handed over is.
	for name, id := range map[string]int{"another account": f.Outsider, "the approver": f.Boss} {
		if _, err := c.Get(ctx, strconv.Itoa(id)); !errors.Is(err, gateapi.ErrNotFound) {
			t.Errorf("%s: %v", name, err)
		}
		if _, err := c.Disable(ctx, lead, strconv.Itoa(id)); !errors.Is(err, gateapi.ErrNotFound) {
			t.Errorf("locking %s: %v", name, err)
		}
	}
	var ids []string
	for after := ""; ; {
		page, next, err := c.List(ctx, after)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range page {
			ids = append(ids, a.ID)
		}
		if next == "" {
			break
		}
		after = next
	}
	if !contains(ids, strconv.Itoa(f.Adopted)) || !contains(ids, made.Account.ID) || contains(ids, strconv.Itoa(f.Outsider)) {
		t.Errorf("the accounts UnderDoc manages: %v", ids)
	}
	// A write that names no person's ID is refused by the gate as well as by the client.
	if _, err := c.Create(ctx, strconv.Itoa(f.Lead+100000), gateapi.NewAccount{Username: "ghost", Roles: []string{"underdoc-user"}}); !errors.Is(err, &gateapi.Error{Code: "actor_invalid"}) {
		t.Errorf("On-Behalf-Of of nobody: %v", err)
	}
}

// TestResettingAccessThroughARealGate: the reset is confirmed by a holder of the approver role in the gate, and the
// new link comes once; someone without the role confirms in vain.
func TestResettingAccessThroughARealGate(t *testing.T) {
	c, f, base := realGate(t)
	ctx := context.Background()
	made, err := c.Create(ctx, strconv.Itoa(f.Lead), gateapi.NewAccount{Username: "mira", Roles: []string{"underdoc-user"}})
	if err != nil {
		t.Fatal(err)
	}
	personIn(t, base, f, "").signIn(made.Link.URL)
	reset := func(subject string) gateapi.Asked {
		asked, err := c.Ask(ctx, gateapi.Ask{Type: gateapi.ResetAccess, Subject: subject, Fields: map[string]any{"account": made.Account.ID}})
		if err != nil || asked.MatchCode == "" || !strings.HasPrefix(asked.ApproveURL, base+"/") {
			t.Fatalf("ask: %+v %v", asked, err)
		}
		return asked
	}
	asked := reset(strconv.Itoa(f.Boss))
	if a, err := c.Approval(ctx, asked.ID, 0); err != nil || a.State != gateapi.ApprovalPending {
		t.Fatalf("before: %+v %v", a, err)
	}
	personIn(t, base, f, "boss").confirm(asked.ApproveURL, f.Password)
	a, err := c.Approval(ctx, asked.ID, 5)
	if err != nil || a.State != gateapi.ApprovalConsumed || a.Result != "ok" || !strings.HasPrefix(a.Link.URL, base+"/") {
		t.Fatalf("after the confirmation: %+v %v", a, err)
	}
	if again, err := c.Approval(ctx, asked.ID, 0); err != nil || again.Link.URL != "" {
		t.Errorf("the link a second time: %+v %v", again, err)
	}
	// Without the approver role the confirmation changes nothing.
	asked = reset(strconv.Itoa(f.Outsider))
	personIn(t, base, f, "outsider").confirm(asked.ApproveURL, f.Password)
	if a, err := c.Approval(ctx, asked.ID, 5); err != nil || a.Result != "forbidden" || a.Link.URL != "" {
		t.Errorf("confirmed without the approver role: %+v %v", a, err)
	}
	if _, err := c.Ask(ctx, gateapi.Ask{Type: gateapi.ResetAccess, Subject: made.Account.ID, Fields: map[string]any{"account": made.Account.ID}}); err == nil {
		t.Error("an account asked to confirm its own reset")
	}
}

// TestConfirmationsOfARealGate: deleting for good and emptying the trash, confirmed in the gate with the browser entry
// and checked by the verifier UnderDoc runs: once, for this action, these fields, this person and this browser only.
func TestConfirmationsOfARealGate(t *testing.T) {
	c, f, base := realGate(t)
	ctx := context.Background()
	verifier, err := gateapi.NewVerifier(c, "underdoc")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var mu sync.Mutex
	jti := tinysesam.JTIStoreFunc(func(_ context.Context, id string, _ int64) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		if seen[id] {
			return false, nil
		}
		seen[id] = true
		return true, nil
	})
	boss := strconv.Itoa(f.Boss)
	returnURL := os.Getenv("UNDERDOC_TEST_GATE_RETURN")
	confirmed := func(typ string, fields map[string]any, state string) string {
		asked, err := c.Ask(ctx, gateapi.Ask{Type: typ, Subject: boss, Fields: fields, ReturnURL: returnURL, State: state})
		if err != nil {
			t.Fatalf("ask %s: %v", typ, err)
		}
		resp := personIn(t, base, f, "boss").confirm(asked.ApproveURL, f.Password)
		back, _ := url.Parse(resp.Header.Get("Location"))
		if resp.StatusCode != http.StatusSeeOther || back == nil || back.Query().Get("approval") != asked.ID || back.Query().Get("state") != state ||
			strings.Contains(resp.Header.Get("Location"), "confirmation") {
			t.Fatalf("the way back: %d %q", resp.StatusCode, resp.Header.Get("Location"))
		}
		a, err := c.Approval(ctx, asked.ID, 5)
		if err != nil || a.Confirmation == "" {
			t.Fatalf("pick up: %+v %v", a, err)
		}
		if again, _ := c.Approval(ctx, asked.ID, 0); again.Confirmation != "" {
			t.Error("the confirmation came a second time")
		}
		return a.Confirmation
	}
	purge := map[string]any{"object": "7k2m9q4x1c", "title": "Minutes 2026"}
	token := confirmed("underdoc.object.purge", purge, "state-of-the-browser")
	want := tinysesam.ApprovalExpect{Type: "underdoc.object.purge", Fields: purge, Subject: boss, Nonce: "state-of-the-browser"}
	// Every other expectation is refused first, without using the confirmation up.
	for name, other := range map[string]tinysesam.ApprovalExpect{
		"another action":  {Type: "underdoc.trash.empty", Fields: purge, Subject: boss, Nonce: want.Nonce},
		"another object":  {Type: want.Type, Fields: map[string]any{"object": "0000000000", "title": "Minutes 2026"}, Subject: boss, Nonce: want.Nonce},
		"another person":  {Type: want.Type, Fields: purge, Subject: strconv.Itoa(f.Lead), Nonce: want.Nonce},
		"another browser": {Type: want.Type, Fields: purge, Subject: boss, Nonce: "another-state"},
	} {
		if _, err := verifier.Verify(ctx, token, other, jti); err == nil {
			t.Errorf("%s takes the confirmation", name)
		}
	}
	if _, err := verifier.Verify(ctx, token, want, jti); err != nil {
		t.Fatalf("the confirmation does not hold: %v (%s)", err, tinysesam.ClassOf(err))
	}
	if _, err := verifier.Verify(ctx, token, want, jti); !errors.Is(err, tinysesam.ErrReplay) {
		t.Errorf("the confirmation holds twice: %v", err)
	}
	// Emptying the trash: integers stay integers, bound to the cutoff.
	cutoff := time.Now().Unix()
	empty := map[string]any{"items": int64(3), "cutoff": cutoff}
	token = confirmed("underdoc.trash.empty", empty, "state-2")
	if _, err := verifier.Verify(ctx, token, tinysesam.ApprovalExpect{Type: "underdoc.trash.empty", Fields: map[string]any{"items": int64(3), "cutoff": cutoff + 1}, Subject: boss, Nonce: "state-2"}, jti); err == nil {
		t.Error("another cutoff takes the confirmation")
	}
	if _, err := verifier.Verify(ctx, token, tinysesam.ApprovalExpect{Type: "underdoc.trash.empty", Fields: empty, Subject: boss, Nonce: "state-2"}, jti); err != nil {
		t.Errorf("emptying the trash: %v (%s)", err, tinysesam.ClassOf(err))
	}
	// A type of another app is never UnderDoc's to ask for.
	if _, err := c.Ask(ctx, gateapi.Ask{Type: "other.object.purge", Subject: boss, Fields: purge}); !errors.Is(err, &gateapi.Error{Code: "forbidden_type"}) {
		t.Errorf("another app's type: %v", err)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
