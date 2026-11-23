// SPDX-License-Identifier: MIT

package account

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"

	tinysesam "github.com/Ollornog/TinySesam/clients/go"
)

// Sign-in through the gate (ADR-32): TinySesam signs people in, and its Go client checks what reaches UnderDoc, either
// the Remote headers that the proxy of the gate sets (header mode) or the gate token itself (token mode). UnderDoc never
// reads who someone is, how they signed in or what kind of account they have from a header on its own.

// Modes of the gate (GateConfig.Mode).
const (
	// GateHeader reads the Remote headers the proxy of the gate sets after asking it. The default.
	GateHeader = "header"
	// GateToken checks the gate token, signed by the gate, and ignores every Remote header.
	GateToken = "token"
)

// DefaultAdminRole is the role of the app in the gate whose holders administer UnderDoc (ADR-35).
const DefaultAdminRole = "underdoc-admin"

// gateAdmin is the name under which the gate passes on its own administrators. It is never a right in UnderDoc (ADR-35).
const gateAdmin = "admin"

// GateConfig says how the gate reaches UnderDoc. The names of the headers and of the claims are those of the gate unless
// its configuration renames them.
type GateConfig struct {
	// Mode is GateHeader (empty) or GateToken.
	Mode string
	// Proxies are the addresses the proxy of the gate connects from (header mode). A request from any other address is
	// refused. Empty: any address, so UnderDoc must be reachable through the proxy alone.
	Proxies []netip.Prefix
	// Headers renames the Remote headers where the gate's forward_headers does (header mode).
	Headers tinysesam.HeaderNames
	// Token mode: the issuer (the gate's base address), the audience the proxy names, the public keys of the gate or
	// the address of its key set, and where the token comes from (the gate cookie by default, or a header).
	Issuer, Audience string
	Keys             []string
	JWKSURL          string
	Cookie           string
	TokenHeader      string
	// AdminRole is the role of the app that makes an administrator; DefaultAdminRole when empty.
	AdminRole string
	// KindRequired refuses a request without Remote-Kind and lets a service account be one (header mode). Until the
	// gate says that its proxy removes a Remote-Kind sent by the browser, it stays off: then a missing Remote-Kind is
	// let through as unknown, and a service account counts as a person signed in with a token, an agent (ADR-36).
	KindRequired bool
	// Logger receives one line for each refused request, with the reason's class only.
	Logger *slog.Logger
}

// Gate identifies the person behind a request through the gate.
type Gate struct {
	client       *tinysesam.Gate
	mode         string
	kindHeader   string
	adminRole    string
	kindRequired bool
}

// ErrNoID is a request whose sign-in names no lasting ID. A person is bound by that ID alone (ADR-35), never by a name.
var ErrNoID = errors.New("the gate passed on no Remote-Id")

// NewGate checks the configuration. Anything contradictory or incomplete stops the start.
func NewGate(cfg GateConfig) (*Gate, error) {
	g := &Gate{mode: cfg.Mode, adminRole: strings.TrimSpace(cfg.AdminRole), kindRequired: cfg.KindRequired}
	if g.mode == "" {
		g.mode = GateHeader
	}
	if g.adminRole == "" {
		g.adminRole = DefaultAdminRole
	}
	if g.adminRole == gateAdmin {
		return nil, fmt.Errorf("the role %q is the gate's own administrator, which is no right in UnderDoc: name a role of the app", gateAdmin)
	}
	// Every setting goes to the client, which refuses one that does not belong to the mode.
	client := tinysesam.Config{Logger: cfg.Logger, TrustedProxies: cfg.Proxies, HeaderNames: cfg.Headers, Issuer: cfg.Issuer,
		Audience: cfg.Audience, GateKeys: cfg.Keys, JWKSURL: cfg.JWKSURL, CookieName: cfg.Cookie, TokenHeader: cfg.TokenHeader}
	switch g.mode {
	case GateHeader:
		client.Mode = tinysesam.HeaderMode
		g.kindHeader = cfg.Headers.Kind
		if g.kindHeader == "" {
			g.kindHeader = tinysesam.DefaultHeaderNames.Kind
		}
	case GateToken:
		client.Mode = tinysesam.TokenMode
		// A signed token always carries the kind of the account (ts_kind), so the kind can be trusted.
		g.kindRequired = true
	default:
		return nil, fmt.Errorf("unknown mode of the gate %q: %s or %s", cfg.Mode, GateHeader, GateToken)
	}
	var err error
	if g.client, err = tinysesam.New(client); err != nil {
		return nil, err
	}
	return g, nil
}

// Mode is GateHeader or GateToken.
func (g *Gate) Mode() string { return g.mode }

// KindTrusted tells whether a service account is taken for one: in token mode, and in header mode once KindRequired is
// on.
func (g *Gate) KindTrusted() bool { return g.kindRequired }

// AdminRole is the role of the app that makes an administrator.
func (g *Gate) AdminRole() string { return g.adminRole }

// Identify returns the person behind a request as the gate's client checked it, or an error that means "not signed
// in". ID and RemoteID are the lasting ID of the account in the gate; the web binds the account of UnderDoc to it.
func (g *Gate) Identify(r *http.Request) (User, error) {
	checked := r
	kindSent := true
	if g.mode == GateHeader && !g.kindRequired && !sent(r.Header, g.kindHeader) {
		// The client of the gate refuses a request without Remote-Kind. Until the gate says that its proxy sets it on
		// every request, its absence is no reason to refuse: the client checks everything else as if a person had come,
		// and the kind stays unknown. This can only make the request weaker, never a service account.
		kindSent = false
		checked = r.Clone(r.Context())
		for name := range checked.Header {
			if strings.EqualFold(name, g.kindHeader) {
				delete(checked.Header, name)
			}
		}
		checked.Header.Set(g.kindHeader, string(tinysesam.AccountUser))
	}
	id, err := g.client.Identify(checked)
	if err != nil {
		return User{}, err
	}
	if id.ID == "" {
		return User{}, ErrNoID
	}
	user := User{ID: id.ID, RemoteID: id.ID, Login: id.User, Name: id.Name, Email: id.Email, Groups: id.Groups,
		AuthKind: id.Auth, Kind: KindPerson}
	if user.Name == "" {
		user.Name = user.Login
	}
	if kindSent {
		user.GateKind = string(id.AccountKind)
	}
	if id.IsService() && g.kindRequired {
		user.Kind = KindService
	}
	// The role of the app makes an administrator, compared exactly; the gate's own administrator is no right here.
	user.Admin = id.HasRole(g.adminRole)
	return user, nil
}

// sent tells whether a request carries a header with a value, in any spelling of its name.
func sent(header http.Header, name string) bool {
	for key, values := range header {
		if !strings.EqualFold(key, name) {
			continue
		}
		for _, v := range values {
			if v != "" {
				return true
			}
		}
	}
	return false
}
