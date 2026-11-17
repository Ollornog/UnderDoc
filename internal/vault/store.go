// SPDX-License-Identifier: MIT

package vault

import (
	"context"
	"errors"

	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// Rows is where sealed values live, per tenant of the context: the table secrets of the database.
type Rows interface {
	PutSealedSecret(ctx context.Context, name string, sealed []byte) error
	SealedSecret(ctx context.Context, name string) ([]byte, error)
	DeleteSecret(ctx context.Context, name string) error
}

// Store keeps the secrets of the tenant of a context: written, used by UnderDoc itself, never read back by anyone. It
// is the building block for credentials of sources, mailboxes and identity providers.
type Store struct {
	Vault *Vault
	Rows  Rows
}

// ErrNoTenant is a secret asked for without a tenant.
var ErrNoTenant = errors.New("a secret belongs to a tenant")

// Put seals a value and keeps it under its name, replacing what was there.
func (s Store) Put(ctx context.Context, name string, value Secret) error {
	t, ok := tenant.From(ctx)
	if !ok {
		return ErrNoTenant
	}
	sealed, err := s.Vault.Seal(t, name, value)
	if err != nil {
		return err
	}
	return s.Rows.PutSealedSecret(ctx, name, sealed)
}

// Has tells whether a secret is kept under the name, without opening it.
func (s Store) Has(ctx context.Context, name string) bool {
	sealed, err := s.Rows.SealedSecret(ctx, name)
	return err == nil && len(sealed) > 0
}

// Use opens a secret for UnderDoc to use, never to show.
func (s Store) Use(ctx context.Context, name string) (Secret, error) {
	t, ok := tenant.From(ctx)
	if !ok {
		return nil, ErrNoTenant
	}
	sealed, err := s.Rows.SealedSecret(ctx, name)
	if err != nil {
		return nil, err
	}
	return s.Vault.Open(t, name, sealed)
}

// Delete removes a secret.
func (s Store) Delete(ctx context.Context, name string) error { return s.Rows.DeleteSecret(ctx, name) }
