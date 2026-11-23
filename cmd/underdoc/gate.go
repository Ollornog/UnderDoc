// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strings"

	tinysesam "github.com/Ollornog/TinySesam/clients/go"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// The sign-in gate (ADR-32, docs/sign-in.md), from the environment:
//
//	UNDERDOC_GATE_MODE          "header" (the Remote headers of the gate's proxy) or "token" (the gate token) ("header")
//	UNDERDOC_GATE_PROXIES       header mode: the addresses or networks the proxy connects from, separated by commas;
//	                            every other peer is refused (none: any, so only the proxy may reach UnderDoc)
//	UNDERDOC_GATE_HEADER_USER, _NAME, _EMAIL, _GROUPS, _ID, _AUTH, _KIND
//	                            header mode: the names of the Remote headers, where the gate's forward_headers renames them
//	UNDERDOC_GATE_ISSUER, UNDERDOC_GATE_AUDIENCE, UNDERDOC_GATE_KEYS (comma separated) or UNDERDOC_GATE_JWKS_URL,
//	UNDERDOC_GATE_COOKIE or UNDERDOC_GATE_TOKEN_HEADER
//	                            token mode: what the token is checked against, and where it comes from
//	UNDERDOC_GATE_KIND          a request without Remote-Kind, or with one the gate's client does not know, is refused
//	                            (none or "required"); "legacy" is the way back for a proxy that does not set it yet:
//	                            a missing one is filled in as user, with a warning at the start (header mode only)
//	UNDERDOC_ADMIN_ROLE         the role of the app in the gate that makes an administrator ("underdoc-admin")
//	UNDERDOC_SPACES_ROLE        the role of the app in the gate that lets a person create spaces ("underdoc-spaces")
//	UNDERDOC_SERVICE_ACCOUNTS   the lasting IDs (Remote-Id) of the service accounts allowed in, separated by commas;
//	                            any other service account is refused (none)
func readGateConfig(log *slog.Logger) (account.GateConfig, error) {
	cfg := account.GateConfig{
		Mode:        os.Getenv("UNDERDOC_GATE_MODE"),
		Issuer:      os.Getenv("UNDERDOC_GATE_ISSUER"),
		Audience:    os.Getenv("UNDERDOC_GATE_AUDIENCE"),
		Keys:        list(os.Getenv("UNDERDOC_GATE_KEYS")),
		JWKSURL:     os.Getenv("UNDERDOC_GATE_JWKS_URL"),
		Cookie:      os.Getenv("UNDERDOC_GATE_COOKIE"),
		TokenHeader: os.Getenv("UNDERDOC_GATE_TOKEN_HEADER"),
		AdminRole:   os.Getenv("UNDERDOC_ADMIN_ROLE"),
		SpacesRole:  os.Getenv("UNDERDOC_SPACES_ROLE"),
		// account.NewGate checks that each is a lasting ID.
		ServiceAccounts: list(os.Getenv("UNDERDOC_SERVICE_ACCOUNTS")),
		Headers: tinysesam.HeaderNames{
			User: os.Getenv("UNDERDOC_GATE_HEADER_USER"), Name: os.Getenv("UNDERDOC_GATE_HEADER_NAME"),
			Email: os.Getenv("UNDERDOC_GATE_HEADER_EMAIL"), Groups: os.Getenv("UNDERDOC_GATE_HEADER_GROUPS"),
			ID: os.Getenv("UNDERDOC_GATE_HEADER_ID"), Auth: os.Getenv("UNDERDOC_GATE_HEADER_AUTH"),
			Kind: os.Getenv("UNDERDOC_GATE_HEADER_KIND"),
		},
		Logger: log,
	}
	if cfg.AdminRole == "" {
		if group := os.Getenv("UNDERDOC_ADMIN_GROUP"); group != "" {
			// The name before ADR-32, still read so that an instance keeps its administrators until it is renamed.
			cfg.AdminRole = group
			log.Warn("UNDERDOC_ADMIN_GROUP is the old name of UNDERDOC_ADMIN_ROLE: rename it")
		}
	}
	switch kind := os.Getenv("UNDERDOC_GATE_KIND"); kind {
	case "":
	case "required":
	case "legacy":
		cfg.KindLegacy = true
		log.Warn("UNDERDOC_GATE_KIND=legacy: a request without Remote-Kind is let through as a person's; only for a proxy of the gate that does not set Remote-Kind yet, remove it once it does")
	default:
		return cfg, fmt.Errorf("UNDERDOC_GATE_KIND: %q is neither empty, required nor legacy", kind)
	}
	for _, item := range list(os.Getenv("UNDERDOC_GATE_PROXIES")) {
		prefix, err := netip.ParsePrefix(item)
		if err != nil {
			addr, aerr := netip.ParseAddr(item)
			if aerr != nil {
				return cfg, fmt.Errorf("UNDERDOC_GATE_PROXIES: %q is no address or network", item)
			}
			prefix = netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen())
		}
		cfg.Proxies = append(cfg.Proxies, prefix.Masked())
	}
	if (cfg.Mode == "" || cfg.Mode == account.GateHeader) && len(cfg.Proxies) == 0 {
		log.Warn("UNDERDOC_GATE_PROXIES is not set: the headers of the gate count from any address, so UnderDoc must be reachable through the gate's proxy alone")
	}
	return cfg, nil
}

// list splits a value at commas and drops empty items.
func list(value string) []string {
	var out []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
