// SPDX-License-Identifier: MIT

// Package gateapi speaks to the account API and the approvals of the gate in front of UnderDoc, TinySesam (ADR-35,
// T-33), with the automation key of UnderDoc's service account there. The gate's Go client checks only what reaches
// UnderDoc; these calls go the other way, so this package follows the gate's specification of the account API
// (spec/provisioning-api.md) and of approvals.
//
// The key is read from a file and never written anywhere: not into an error, not into a log. A sign-in link the gate
// answers lives only in the answer and goes to the person who asked; it is never logged or stored either.
package gateapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	tinysesam "github.com/Ollornog/TinySesam/clients/go"
)

// Scopes of the account API (spec/provisioning-api.md section 2).
const (
	ScopeRead    = "users:read"
	ScopeInvite  = "users:invite"
	ScopeDisable = "users:disable"
	ScopeReset   = "users:reset"
)

// States of an account in the gate (spec section 3).
const (
	StateActive           = "active"
	StateDisabled         = "disabled"
	StateDisabledOperator = "disabled_operator"
	StateDeleting         = "deleting"
)

// ResetAccess is the approval of the gate that resets the access of an account (spec section 5).
const ResetAccess = "tinysesam.account.reset_access"

// NoPerson is the On-Behalf-Of of a job without a person, such as an import on the command line.
const NoPerson = "-"

// maxAnswer is the most an answer of the gate may carry; an account page with 200 accounts is far below it.
const maxAnswer = 1 << 20

// Error is a refusal of the gate: its status and its code, never the text it sent, which may name more than UnderDoc
// needs to show.
type Error struct {
	Status int
	Code   string
	// RetryAfter is the wait the gate asked for with 429, in seconds.
	RetryAfter int
}

func (e *Error) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("the gate answered %d", e.Status)
	}
	return fmt.Sprintf("the gate answered %d %s", e.Status, e.Code)
}

// Is matches errors.Is(err, &Error{Code: "…"}) by code, and by status where the code is empty.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return (t.Code == "" || t.Code == e.Code) && (t.Status == 0 || t.Status == e.Status)
}

// Sentinels for errors.Is.
var (
	ErrNotFound       = &Error{Code: "not_found"}
	ErrSignedIn       = &Error{Code: "already_signed_in"}
	ErrDisabled       = &Error{Code: "account_disabled"}
	ErrShared         = &Error{Code: "account_shared"}
	ErrOperator       = &Error{Code: "disabled_by_operator"}
	ErrRoleNotAllowed = &Error{Code: "role_not_allowed"}
	ErrNameTaken      = &Error{Code: "username_taken"}
	ErrEmailTaken     = &Error{Code: "email_taken"}
	ErrLimit          = &Error{Status: http.StatusTooManyRequests}
)

// Client calls the gate with the automation key of UnderDoc's service account.
type Client struct {
	base string
	key  string
	http *http.Client
}

// Config names the gate and the file with the key.
type Config struct {
	// URL is the gate's own address, its base_url: the calls go to the gate directly, not through the proxy of the app.
	URL string
	// KeyFile holds the automation key of UnderDoc's service account in the gate, and nothing else.
	KeyFile string
	// HTTP is the client for the calls; nil makes one with a timeout that never follows a redirect.
	HTTP *http.Client
}

// New checks the configuration and reads the key. The address is https, or http to the own machine only, since the key
// travels in every call.
func New(cfg Config) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(cfg.URL, "/"))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("the address of the gate is no plain http(s) address")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !loopback(u.Hostname()) {
			return nil, errors.New("the address of the gate is https; plain http only to the own machine")
		}
	default:
		return nil, errors.New("the address of the gate is https; plain http only to the own machine")
	}
	if cfg.KeyFile == "" {
		return nil, errors.New("no file with the key of the service account")
	}
	raw, err := os.ReadFile(cfg.KeyFile)
	if err != nil {
		// The path may be named, never what is in the file.
		return nil, fmt.Errorf("the key of the service account cannot be read from %s", cfg.KeyFile)
	}
	key := strings.TrimSpace(string(raw))
	if key == "" || strings.ContainsFunc(key, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return nil, fmt.Errorf("the file %s does not hold one key of the service account", cfg.KeyFile)
	}
	client := cfg.HTTP
	if client == nil {
		client = &http.Client{Timeout: 40 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}
	}
	return &Client{base: u.String(), key: key, http: client}, nil
}

// URL is the gate's address, the issuer of its approvals.
func (c *Client) URL() string { return c.base }

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// do sends one call. actor is the On-Behalf-Of of a write ("" for a read); out receives the answer.
func (c *Client) do(ctx context.Context, method, path, actor string, in, out any) (int, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("X-Api-Key", c.key)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if actor != "" {
		req.Header.Set("TinySesam-On-Behalf-Of", actor)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// The error of the transport never names the key; the address of the call is left out all the same.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return 0, fmt.Errorf("the gate is not reached: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswer+1))
	if err != nil {
		return resp.StatusCode, fmt.Errorf("the answer of the gate broke off: %w", err)
	}
	if len(data) > maxAnswer {
		return resp.StatusCode, errors.New("the answer of the gate is too long")
	}
	if resp.StatusCode >= 300 {
		e := &Error{Status: resp.StatusCode}
		var refusal struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &refusal) == nil && code(refusal.Error) {
			e.Code = refusal.Error
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			e.RetryAfter, _ = strconv.Atoi(resp.Header.Get("Retry-After"))
		}
		return resp.StatusCode, e
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, errors.New("the gate answered something that is no account API")
		}
	}
	return resp.StatusCode, nil
}

// code tells whether s looks like an error code of the gate: short, lower case, digits and underscores.
func code(s string) bool {
	if s == "" || len(s) > 40 {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

// ID checks a lasting ID of the gate: a decimal number, as Remote-Id carries it.
func ID(s string) bool {
	if s == "" || len(s) > 19 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// actorOK checks an On-Behalf-Of: the lasting ID of a person, or NoPerson.
func actorOK(actor string) error {
	if actor == NoPerson || ID(actor) {
		return nil
	}
	return errors.New("a call that changes something names the person it acts for by their lasting ID in the gate")
}

// Provisioning is what the key may do (GET /auth/api/provisioning).
type Provisioning struct {
	App          string   `json:"app"`
	Scopes       []string `json:"scopes"`
	Roles        []string `json:"roles"`
	ApproverRole string   `json:"approver_role"`
	LinkTTLMin   int      `json:"link_ttl_min"`
	MaxAccounts  int      `json:"max_accounts"`
	Accounts     int      `json:"accounts"`
}

// Has tells whether the key holds a scope.
func (p Provisioning) Has(scope string) bool {
	for _, s := range p.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// Provisioning asks the gate what UnderDoc's key may do.
func (c *Client) Provisioning(ctx context.Context) (Provisioning, error) {
	var p Provisioning
	_, err := c.do(ctx, http.MethodGet, "/auth/api/provisioning", "", nil, &p)
	return p, err
}

// Account is an account of the gate as the account API shows it (spec section 3). Times are Unix seconds, 0 for none.
type Account struct {
	ID           string
	Username     string
	DisplayName  string
	Email        string
	Roles        []string
	OtherRoles   bool
	State        string
	FirstLoginAt int64
	LastLoginAt  int64
	DeleteAfter  int64
}

// SignedIn tells whether the account has signed in once: then no new sign-in link is issued.
func (a Account) SignedIn() bool { return a.FirstLoginAt != 0 }

type wireAccount struct {
	ID           json.Number `json:"id"`
	Username     string      `json:"username"`
	DisplayName  *string     `json:"display_name"`
	Email        *string     `json:"email"`
	Roles        []string    `json:"roles"`
	OtherRoles   bool        `json:"other_roles"`
	State        string      `json:"state"`
	FirstLoginAt *int64      `json:"first_login_at"`
	LastLoginAt  *int64      `json:"last_login_at"`
	DeleteAfter  *int64      `json:"delete_after"`
}

func (w wireAccount) account() (Account, error) {
	a := Account{ID: w.ID.String(), Username: w.Username, Roles: w.Roles, OtherRoles: w.OtherRoles, State: w.State}
	if !ID(a.ID) {
		return Account{}, errors.New("the gate answered an account without a lasting ID")
	}
	if w.DisplayName != nil {
		a.DisplayName = *w.DisplayName
	}
	if w.Email != nil {
		a.Email = *w.Email
	}
	if w.FirstLoginAt != nil {
		a.FirstLoginAt = *w.FirstLoginAt
	}
	if w.LastLoginAt != nil {
		a.LastLoginAt = *w.LastLoginAt
	}
	if w.DeleteAfter != nil {
		a.DeleteAfter = *w.DeleteAfter
	}
	if a.Roles == nil {
		a.Roles = []string{}
	}
	return a, nil
}

// NewAccount is a person to invite (POST /auth/api/users).
type NewAccount struct {
	Username    string   `json:"username"`
	DisplayName string   `json:"display_name,omitempty"`
	Email       string   `json:"email,omitempty"`
	Roles       []string `json:"roles"`
	Next        string   `json:"next,omitempty"`
}

// Link is a one-time sign-in link. Hand it to the person; never log or keep it.
type Link struct {
	URL       string
	ExpiresAt int64
}

// Created is an invited account with its first sign-in link.
type Created struct {
	Account Account
	Link    Link
}

// Create invites a person: an account of the gate that UnderDoc manages, with roles of its allowlist, and its first
// sign-in link. actor is the lasting ID of the person who invites.
func (c *Client) Create(ctx context.Context, actor string, in NewAccount) (Created, error) {
	if err := actorOK(actor); err != nil {
		return Created{}, err
	}
	var out struct {
		SigninURL string      `json:"signin_url"`
		ExpiresAt int64       `json:"expires_at"`
		User      wireAccount `json:"user"`
	}
	if _, err := c.do(ctx, http.MethodPost, "/auth/api/users", actor, in, &out); err != nil {
		return Created{}, err
	}
	a, err := out.User.account()
	if err != nil {
		return Created{}, err
	}
	return Created{Account: a, Link: Link{URL: out.SigninURL, ExpiresAt: out.ExpiresAt}}, nil
}

// Get returns an account UnderDoc manages; ErrNotFound for any other, as the gate answers for those.
func (c *Client) Get(ctx context.Context, id string) (Account, error) {
	if !ID(id) {
		return Account{}, ErrNotFound
	}
	var w wireAccount
	if _, err := c.do(ctx, http.MethodGet, "/auth/api/users/"+id, "", nil, &w); err != nil {
		return Account{}, err
	}
	return w.account()
}

// List returns one page of the accounts UnderDoc manages, after the lasting ID after ("" for the first page), and the
// ID to ask after for the next page ("" at the end).
func (c *Client) List(ctx context.Context, after string) ([]Account, string, error) {
	q := url.Values{"limit": {"200"}}
	if after != "" {
		if !ID(after) {
			return nil, "", errors.New("a page of the account API starts after a lasting ID")
		}
		q.Set("after", after)
	}
	var out struct {
		Users []wireAccount `json:"users"`
		Next  *json.Number  `json:"next"`
	}
	if _, err := c.do(ctx, http.MethodGet, "/auth/api/users?"+q.Encode(), "", nil, &out); err != nil {
		return nil, "", err
	}
	list := make([]Account, 0, len(out.Users))
	for _, w := range out.Users {
		a, err := w.account()
		if err != nil {
			return nil, "", err
		}
		list = append(list, a)
	}
	next := ""
	if out.Next != nil {
		next = out.Next.String()
	}
	return list, next, nil
}

// Disable locks an account in the gate, with the lock of the application; Enable lifts that lock.
func (c *Client) Disable(ctx context.Context, actor, id string) (Account, error) {
	return c.lock(ctx, actor, id, "disable")
}

// Enable lifts the lock of the application; a lock of the operator stays (ErrOperator).
func (c *Client) Enable(ctx context.Context, actor, id string) (Account, error) {
	return c.lock(ctx, actor, id, "enable")
}

func (c *Client) lock(ctx context.Context, actor, id, way string) (Account, error) {
	if err := actorOK(actor); err != nil {
		return Account{}, err
	}
	if !ID(id) {
		return Account{}, ErrNotFound
	}
	var w wireAccount
	if _, err := c.do(ctx, http.MethodPost, "/auth/api/users/"+id+"/"+way, actor, nil, &w); err != nil {
		return Account{}, err
	}
	return w.account()
}

// SigninLink issues a new one-time sign-in link, only before the first sign-in (ErrSignedIn after it); older links of
// the account die.
func (c *Client) SigninLink(ctx context.Context, actor, id string) (Link, error) {
	if err := actorOK(actor); err != nil {
		return Link{}, err
	}
	if !ID(id) {
		return Link{}, ErrNotFound
	}
	var out struct {
		SigninURL string `json:"signin_url"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if _, err := c.do(ctx, http.MethodPost, "/auth/api/users/"+id+"/signin-link", actor, struct{}{}, &out); err != nil {
		return Link{}, err
	}
	return Link{URL: out.SigninURL, ExpiresAt: out.ExpiresAt}, nil
}

// Ask is a request for an approval (POST /auth/api/approvals): the gate's reset_access, or an action of the app.
type Ask struct {
	Type    string         `json:"type"`
	Subject string         `json:"subject"`
	Fields  map[string]any `json:"fields"`
	// ReturnURL and State send the browser back to UnderDoc after the decision (actions of the app only).
	ReturnURL string `json:"return_url,omitempty"`
	State     string `json:"state,omitempty"`
}

// Asked is an approval waiting for a person: the page to send them to and the code they compare there.
type Asked struct {
	ID         string `json:"id"`
	ApproveURL string `json:"approve_url"`
	MatchCode  string `json:"match_code"`
	ExpiresAt  int64  `json:"expires_at"`
}

// Ask asks the gate for an approval.
func (c *Client) Ask(ctx context.Context, in Ask) (Asked, error) {
	if !ID(in.Subject) {
		return Asked{}, errors.New("an approval names the person who confirms by their lasting ID in the gate")
	}
	var out Asked
	if _, err := c.do(ctx, http.MethodPost, "/auth/api/approvals", "", in, &out); err != nil {
		return Asked{}, err
	}
	if !approvalID(out.ID) || out.ApproveURL == "" {
		return Asked{}, errors.New("the gate answered an approval without its id or its page")
	}
	return out, nil
}

// States of an approval.
const (
	ApprovalPending  = "pending"
	ApprovalConsumed = "consumed"
)

// Approval is the state of an approval. Confirmation (actions of the app) and Output (reset_access) come only once, in
// the first answer after the confirmation; never log either.
type Approval struct {
	State        string
	Result       string
	Confirmation string
	Link         Link
}

// Approval reads the state of an approval UnderDoc asked for, waiting up to wait seconds (0 to 25) for a change.
func (c *Client) Approval(ctx context.Context, id string, wait int) (Approval, error) {
	if !approvalID(id) {
		return Approval{}, ErrNotFound
	}
	path := "/auth/api/approvals/" + id
	if wait > 0 {
		path += "?wait=" + strconv.Itoa(min(wait, 25))
	}
	var out struct {
		State        string `json:"state"`
		Result       string `json:"result"`
		Confirmation string `json:"confirmation"`
		Output       *struct {
			SigninURL string `json:"signin_url"`
			ExpiresAt int64  `json:"expires_at"`
		} `json:"output"`
	}
	if _, err := c.do(ctx, http.MethodGet, path, "", nil, &out); err != nil {
		return Approval{}, err
	}
	a := Approval{State: out.State, Result: out.Result, Confirmation: out.Confirmation}
	if out.Output != nil {
		a.Link = Link{URL: out.Output.SigninURL, ExpiresAt: out.Output.ExpiresAt}
	}
	return a, nil
}

// approvalID checks the id of an approval: apv_ and letters or digits, as the gate makes them.
func approvalID(s string) bool {
	if !strings.HasPrefix(s, "apv_") || len(s) > 64 || len(s) < 8 {
		return false
	}
	for _, r := range s[4:] {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

// ApprovalID tells whether s has the form of the id of an approval of the gate.
func ApprovalID(s string) bool { return approvalID(s) }

// NewVerifier checks the confirmations of the gate for the actions of the app app (apps.<app> in the gate's
// configuration) with the gate's Go client: the issuer is the gate's address, the keys its approval key set
// (/auth/approvals/jwks.json), never the gate key.
func NewVerifier(c *Client, app string) (*tinysesam.ApprovalVerifier, error) {
	return tinysesam.NewApprovalVerifier(tinysesam.ApprovalConfig{Issuer: c.base, AppID: app, JWKSURL: c.base + "/auth/approvals/jwks.json"})
}
