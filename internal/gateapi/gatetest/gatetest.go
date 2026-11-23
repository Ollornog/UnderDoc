// SPDX-License-Identifier: MIT

// Package gatetest is a stand-in for the account API and the approvals of the gate, TinySesam, for the tests of the
// web. It answers as spec/provisioning-api.md and spec/approval-token.md of the gate say, signs confirmations with a key
// of its own, and lets a test decide what a person in the gate would do (confirm, sign in). It stands for the gate only
// in unit tests: the contract test of package gateapi runs against a real TinySesam (scripts/check-services.sh), so the
// assumptions written here are checked there.
package gatetest

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tinysesam "github.com/Ollornog/TinySesam/clients/go"
)

// Call is one call the stand-in received, with its On-Behalf-Of.
type Call struct {
	Method, Path, Actor string
}

// Account is an account of the stand-in.
type Account struct {
	ID, Username, Name, Email string
	Roles                     []string
	State                     string
	FirstLogin                int64
	// Managed tells that UnderDoc manages it: the account API sees only those.
	Managed bool
}

// Approval is an approval the stand-in was asked for.
type Approval struct {
	ID, Type, Subject, State, ReturnURL, Nonce string
	Fields                                     map[string]any
	// Confirmed by whom (the lasting ID), once a test confirmed it.
	By string
	// Picked tells that the confirmation or the output was handed out once.
	Picked bool
}

// Gate is the stand-in.
type Gate struct {
	*httptest.Server
	t       testing.TB
	mu      sync.Mutex
	app     string
	roles   []string
	scopes  []string
	approve string
	next    int
	byID    map[string]*Account
	asked   map[string]*Approval
	calls   []Call
	priv    ed25519.PrivateKey
	// key is the automation key the stand-in takes, made for this stand-in alone; any other is 401.
	key string
	kid string
	// Refuse makes every call answer this status and code, until it is cleared.
	refuseStatus int
	refuseCode   string
}

// New starts a stand-in for the application app with the allowlist roles and the approver role approver.
func New(t testing.TB, app string, roles []string, approver string) *Gate {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secret := make([]byte, 24)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(pub)
	g := &Gate{t: t, app: app, roles: roles, approve: approver, next: 100, byID: map[string]*Account{}, asked: map[string]*Approval{},
		scopes: []string{"users:read", "users:invite", "users:update", "users:disable", "users:reset"}, priv: priv,
		kid: hex.EncodeToString(sum[:])[:16], key: "tsk_" + hex.EncodeToString(secret)}
	g.Server = httptest.NewServer(http.HandlerFunc(g.serve))
	t.Cleanup(g.Close)
	return g
}

// Key is the automation key this stand-in takes, made fresh for it.
func (g *Gate) Key() string { return g.key }

// KeyFile writes the key into a file of the test's own folder and returns its path.
func (g *Gate) KeyFile() string {
	path := filepath.Join(g.t.TempDir(), "gate.key")
	if err := os.WriteFile(path, []byte(g.key+"\n"), 0o600); err != nil {
		g.t.Fatal(err)
	}
	return path
}

// SetScopes changes the scopes the key holds.
func (g *Gate) SetScopes(scopes ...string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.scopes = scopes
}

// Refuse makes every call answer status with code; Refuse(0, "") ends it.
func (g *Gate) Refuse(status int, code string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.refuseStatus, g.refuseCode = status, code
}

// Add makes an account in the stand-in and returns its lasting ID.
func (g *Gate) Add(a Account) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	a.ID = strconv.Itoa(g.next)
	if a.State == "" {
		a.State = "active"
	}
	g.byID[a.ID] = &a
	return a.ID
}

// Get returns an account of the stand-in.
func (g *Gate) Get(id string) (Account, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	a, ok := g.byID[id]
	if !ok {
		return Account{}, false
	}
	return *a, true
}

// SignIn records the first sign-in of an account.
func (g *Gate) SignIn(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if a, ok := g.byID[id]; ok && a.FirstLogin == 0 {
		a.FirstLogin = time.Now().Unix()
	}
}

// Calls returns the calls so far.
func (g *Gate) Calls() []Call {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.calls)
}

// Asked returns an approval the stand-in was asked for.
func (g *Gate) Asked(id string) (Approval, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	a, ok := g.asked[id]
	if !ok {
		return Approval{}, false
	}
	return *a, true
}

// Last returns the approval asked for last.
func (g *Gate) Last() Approval {
	g.mu.Lock()
	defer g.mu.Unlock()
	var last *Approval
	for _, a := range g.asked {
		if last == nil || a.ID > last.ID {
			last = a
		}
	}
	if last == nil {
		g.t.Fatal("the gate was asked for no approval")
	}
	return *last
}

// Confirm lets the person by confirm an approval in the gate, as on its page with a step-up. Only the subject may; a
// reset_access needs the approver role, which by holds when approver is true.
func (g *Gate) Confirm(id, by string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	a, ok := g.asked[id]
	if !ok || a.State != "pending" {
		g.t.Fatalf("approval %s is not pending", id)
	}
	if a.Subject != by {
		g.t.Fatalf("approval %s asks %s to confirm, not %s", id, a.Subject, by)
	}
	a.State, a.By = "approved", by
}

// Reject lets the subject reject an approval.
func (g *Gate) Reject(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if a, ok := g.asked[id]; ok {
		a.State = "rejected"
	}
}

// Sign makes a confirmation as the gate signs it, for a test that needs one the stand-in did not hand out.
func (g *Gate) Sign(subject, typ string, fields map[string]any, nonce, jti string) string {
	now := time.Now().Unix()
	entry := map[string]any{"type": typ}
	for k, v := range fields {
		entry[k] = v
	}
	claims := map[string]any{"iss": g.URL, "aud": g.app, "sub": subject, "iat": now, "nbf": now, "exp": now + 120,
		"jti": jti, "auth_time": now, "amr": []string{"pwd"}, "act": map[string]string{"sub": "app:" + g.app},
		"authorization_details": []any{entry}, "ts_payload_sha256": tinysesam.PayloadDigest(entry)}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	header, _ := json.Marshal(map[string]string{"alg": "EdDSA", "typ": tinysesam.ApprovalTokenType, "kid": g.kid})
	payload, _ := json.Marshal(claims)
	input := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	return input + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(g.priv, []byte(input)))
}

func (g *Gate) serve(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if r.URL.Path == "/auth/approvals/jwks.json" {
		pub := g.priv.Public().(ed25519.PublicKey)
		writeJSON(w, 200, map[string]any{"keys": []any{map[string]string{"kty": "OKP", "crv": "Ed25519", "alg": "EdDSA",
			"use": "sig", "kid": g.kid, "x": base64.RawURLEncoding.EncodeToString(pub)}}})
		return
	}
	g.calls = append(g.calls, Call{Method: r.Method, Path: r.URL.Path, Actor: r.Header.Get("TinySesam-On-Behalf-Of")})
	if r.Header.Get("X-Api-Key") != g.key {
		refuse(w, 401, "unauthenticated")
		return
	}
	if g.refuseStatus != 0 {
		refuse(w, g.refuseStatus, g.refuseCode)
		return
	}
	write := r.Method != http.MethodGet
	actor := r.Header.Get("TinySesam-On-Behalf-Of")
	if write && !strings.HasPrefix(r.URL.Path, "/auth/api/approvals") {
		if actor == "" {
			refuse(w, 400, "actor_required")
			return
		}
		if _, err := strconv.Atoi(actor); err != nil && actor != "-" {
			refuse(w, 400, "actor_invalid")
			return
		}
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/auth/api/"), "/")
	switch {
	case r.URL.Path == "/auth/api/provisioning":
		writeJSON(w, 200, map[string]any{"app": g.app, "scopes": g.scopes, "roles": g.roles, "approver_role": g.approve,
			"link_ttl_min": 60, "max_accounts": 1000, "accounts": len(g.byID)})
	case parts[0] == "users" && len(parts) == 1 && r.Method == http.MethodPost:
		g.create(w, r)
	case parts[0] == "users" && len(parts) == 1 && r.Method == http.MethodGet:
		var list []any
		for _, a := range g.byID {
			if a.Managed {
				list = append(list, wire(a))
			}
		}
		slices.SortFunc(list, func(x, y any) int {
			return strings.Compare(fmt.Sprint(x.(map[string]any)["id"]), fmt.Sprint(y.(map[string]any)["id"]))
		})
		writeJSON(w, 200, map[string]any{"users": list, "next": nil})
	case parts[0] == "users" && len(parts) >= 2:
		a, ok := g.byID[parts[1]]
		if !ok || !a.Managed {
			refuse(w, 404, "not_found")
			return
		}
		g.account(w, r, a, parts[2:])
	case parts[0] == "approvals" && len(parts) == 1 && r.Method == http.MethodPost:
		g.ask(w, r)
	case parts[0] == "approvals" && len(parts) == 2 && r.Method == http.MethodGet:
		g.pickup(w, parts[1])
	default:
		refuse(w, 404, "not_found")
	}
}

func (g *Gate) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username    string   `json:"username"`
		DisplayName string   `json:"display_name"`
		Email       string   `json:"email"`
		Roles       []string `json:"roles"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil || in.Username == "" {
		refuse(w, 400, "invalid")
		return
	}
	if len(in.Roles) == 0 || slices.ContainsFunc(in.Roles, func(role string) bool { return !slices.Contains(g.roles, role) }) {
		refuse(w, 400, "role_not_allowed")
		return
	}
	for _, a := range g.byID {
		if a.Username == in.Username {
			refuse(w, 409, "username_taken")
			return
		}
	}
	g.next++
	a := &Account{ID: strconv.Itoa(g.next), Username: in.Username, Name: in.DisplayName, Email: in.Email, Roles: in.Roles,
		State: "active", Managed: true}
	g.byID[a.ID] = a
	writeJSON(w, 201, map[string]any{"id": g.next, "username": a.Username, "signin_url": g.URL + "/auth/magic/link-" + a.ID,
		"expires_at": time.Now().Add(time.Hour).Unix(), "qr": nil, "user": wire(a)})
}

func (g *Gate) account(w http.ResponseWriter, r *http.Request, a *Account, rest []string) {
	switch {
	case len(rest) == 0 && r.Method == http.MethodGet:
		writeJSON(w, 200, wire(a))
	case len(rest) == 1 && rest[0] == "disable" && r.Method == http.MethodPost:
		if a.State == "active" {
			a.State = "disabled"
		}
		writeJSON(w, 200, wire(a))
	case len(rest) == 1 && rest[0] == "enable" && r.Method == http.MethodPost:
		if a.State == "disabled_operator" {
			refuse(w, 409, "disabled_by_operator")
			return
		}
		a.State = "active"
		writeJSON(w, 200, wire(a))
	case len(rest) == 1 && rest[0] == "signin-link" && r.Method == http.MethodPost:
		switch {
		case a.FirstLogin != 0:
			refuse(w, 409, "already_signed_in")
		case a.State != "active":
			refuse(w, 409, "account_disabled")
		default:
			writeJSON(w, 200, map[string]any{"id": a.ID, "signin_url": g.URL + "/auth/magic/new-" + a.ID,
				"expires_at": time.Now().Add(time.Hour).Unix(), "qr": nil})
		}
	default:
		refuse(w, 404, "not_found")
	}
}

func (g *Gate) ask(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Type      string         `json:"type"`
		Subject   string         `json:"subject"`
		Fields    map[string]any `json:"fields"`
		ReturnURL string         `json:"return_url"`
		State     string         `json:"state"`
	}
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil {
		refuse(w, 400, "invalid_request")
		return
	}
	switch {
	case in.Type == "tinysesam.account.reset_access":
		if !slices.Contains(g.scopes, "users:reset") {
			refuse(w, 403, "scope_missing")
			return
		}
		target, _ := in.Fields["account"].(string)
		if a, ok := g.byID[target]; !ok || !a.Managed {
			refuse(w, 404, "not_found")
			return
		}
		if target == in.Subject {
			refuse(w, 403, "forbidden")
			return
		}
	case !strings.HasPrefix(in.Type, g.app+"."):
		refuse(w, 403, "forbidden_type")
		return
	}
	for k, v := range in.Fields {
		if n, ok := v.(json.Number); ok {
			i, err := n.Int64()
			if err != nil {
				refuse(w, 400, "invalid_request")
				return
			}
			in.Fields[k] = i
		}
	}
	id := fmt.Sprintf("apv_%06d%s", len(g.asked)+1, strings.Repeat("x", 8))
	g.asked[id] = &Approval{ID: id, Type: in.Type, Subject: in.Subject, State: "pending", Fields: in.Fields,
		ReturnURL: in.ReturnURL, Nonce: in.State}
	writeJSON(w, 201, map[string]any{"id": id, "state": "pending", "match_code": "418 207",
		"approve_url": g.URL + "/auth/approvals/" + id, "expires_at": time.Now().Add(10 * time.Minute).Unix()})
}

func (g *Gate) pickup(w http.ResponseWriter, id string) {
	a, ok := g.asked[id]
	if !ok {
		refuse(w, 404, "not_found")
		return
	}
	switch {
	case a.State == "approved" && !a.Picked:
		a.Picked, a.State = true, "consumed"
		if a.Type == "tinysesam.account.reset_access" {
			target := g.byID[a.Fields["account"].(string)]
			if target == nil || !target.Managed {
				writeJSON(w, 200, map[string]any{"id": id, "state": "consumed", "result": "not_found"})
				return
			}
			writeJSON(w, 200, map[string]any{"id": id, "state": "consumed", "result": "ok",
				"output": map[string]any{"account": target.ID, "signin_url": g.URL + "/auth/magic/reset-" + target.ID,
					"expires_at": time.Now().Add(time.Hour).Unix(), "qr": nil}})
			return
		}
		token := g.Sign(a.Subject, a.Type, a.Fields, a.Nonce, a.ID)
		writeJSON(w, 200, map[string]any{"id": id, "state": "consumed", "type": a.Type, "confirmation": token})
	default:
		writeJSON(w, 200, map[string]any{"id": id, "state": a.State, "type": a.Type})
	}
}

func wire(a *Account) map[string]any {
	id, _ := strconv.Atoi(a.ID)
	var first any
	if a.FirstLogin != 0 {
		first = a.FirstLogin
	}
	var email any
	if a.Email != "" {
		email = a.Email
	}
	return map[string]any{"id": id, "username": a.Username, "display_name": a.Name, "email": email, "roles": a.Roles,
		"other_roles": false, "state": a.State, "created_at": 1, "first_login_at": first, "last_login_at": first,
		"delete_after": nil}
}

func refuse(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": "made up by gatetest"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
