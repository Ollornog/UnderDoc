// SPDX-License-Identifier: MIT

// Package vault seals what a tenant enters for other systems, such as the client secret of its identity provider or,
// later, the password of a source or a mailbox (ADR-23). It uses AES-256-GCM under a key from the environment, and binds
// every sealed value to its tenant and its name, so a value copied to another row does not open. What is sealed can be
// written and used, never shown again: no API returns it, and a Secret never prints itself into a log.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
)

// KeyEnv is the variable that holds the key: 32 random bytes in base64, for example from `openssl rand -base64 32`.
const KeyEnv = "UNDERDOC_SECRET_KEY"

// Errors of the vault.
var (
	ErrNoKey  = errors.New("no key for secrets: set " + KeyEnv + " to 32 random bytes in base64")
	ErrKey    = errors.New(KeyEnv + " must be 32 bytes in base64")
	ErrSealed = errors.New("a sealed value does not open: wrong key, tenant or name")
)

// Vault seals and opens values. The zero value and nil have no key: they seal nothing and say so.
type Vault struct {
	aead cipher.AEAD
}

// New returns a vault with the key in base64. An empty key gives a vault without a key, so the rest of UnderDoc runs.
func New(key string) (*Vault, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return &Vault{}, nil
	}
	raw, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		raw, err = base64.RawURLEncoding.DecodeString(key)
	}
	if err != nil || len(raw) != 32 {
		return nil, ErrKey
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Vault{aead: aead}, nil
}

// Ready tells whether the vault has a key.
func (v *Vault) Ready() bool { return v != nil && v.aead != nil }

func label(tenant, name string) []byte { return []byte("underdoc/v1\x00" + tenant + "\x00" + name) }

// Seal encrypts a value of a tenant under its name, with a fresh nonce in front.
func (v *Vault) Seal(tenant, name string, value Secret) ([]byte, error) {
	if !v.Ready() {
		return nil, ErrNoKey
	}
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return v.aead.Seal(nonce, nonce, value, label(tenant, name)), nil
}

// Open decrypts what Seal made for the same tenant and name.
func (v *Vault) Open(tenant, name string, sealed []byte) (Secret, error) {
	if !v.Ready() {
		return nil, ErrNoKey
	}
	n := v.aead.NonceSize()
	if len(sealed) < n {
		return nil, ErrSealed
	}
	plain, err := v.aead.Open(nil, sealed[:n], sealed[n:], label(tenant, name))
	if err != nil {
		return nil, ErrSealed
	}
	return Secret(plain), nil
}

// Secret is a value that never shows itself: printed, logged or turned into JSON, it is a mark.
type Secret []byte

const hidden = "[hidden]"

// String hides the value.
func (Secret) String() string { return hidden }

// GoString hides the value from %#v.
func (Secret) GoString() string { return hidden }

// LogValue hides the value from slog.
func (Secret) LogValue() slog.Value { return slog.StringValue(hidden) }

// MarshalJSON hides the value from JSON.
func (Secret) MarshalJSON() ([]byte, error) { return []byte(`"` + hidden + `"`), nil }

// MarshalText hides the value from text encodings.
func (Secret) MarshalText() ([]byte, error) { return []byte(hidden), nil }
