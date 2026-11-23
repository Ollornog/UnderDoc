// SPDX-License-Identifier: MIT

// Package gatetest signs gate tokens as the gate does, for tests of token mode (ADR-32). The key is made up and
// fixed, so it signs for tests only.
package gatetest

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"time"

	tinysesam "github.com/Ollornog/TinySesam/clients/go"
)

// Issuer and Audience are what the tokens of this package name.
const (
	Issuer   = "https://auth.example.com"
	Audience = "docs.example.com"
)

var private = ed25519.NewKeyFromSeed(seed("underdoc gatetest"))

func seed(text string) []byte {
	sum := sha256.Sum256([]byte(text))
	return sum[:]
}

// PublicKey is the key a gate in token mode checks the tokens of this package with, as `tinysesam gate-key` prints it.
func PublicKey() string {
	return base64.StdEncoding.EncodeToString(private.Public().(ed25519.PublicKey))
}

// Token is a gate token for the account with the ID sub, valid for five minutes. Claims add to or, with nil, take
// away from the identity claims (ts_user, ts_auth, ts_kind, ...).
func Token(sub string, claims map[string]any) string {
	now := time.Now().Unix()
	payload := map[string]any{
		"iss": Issuer, "aud": Audience, "sub": sub, "iat": now - 5, "nbf": now - 5, "exp": now + 300, "jti": "t-" + sub,
		"ts_id": sub, "ts_user": "user" + sub, "ts_name": "User " + sub, "ts_auth": "session", "ts_kind": "user",
	}
	for k, v := range claims {
		if v == nil {
			delete(payload, k)
			continue
		}
		payload[k] = v
	}
	return SignWith(private, payload)
}

// SignWith signs any payload with any key, for tokens a gate would never issue.
func SignWith(key ed25519.PrivateKey, payload map[string]any) string {
	head, _ := json.Marshal(map[string]string{"alg": "EdDSA", "typ": "tinysesam-gate+jwt",
		"kid": tinysesam.KeyID(key.Public().(ed25519.PublicKey))})
	body, _ := json.Marshal(payload)
	input := base64.RawURLEncoding.EncodeToString(head) + "." + base64.RawURLEncoding.EncodeToString(body)
	return input + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, []byte(input)))
}
