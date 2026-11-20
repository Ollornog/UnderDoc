// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
)

// rekey seals every sealed value of every tenant again under the key of UNDERDOC_SECRET_KEY, opening what the key in
// UNDERDOC_SECRET_KEY_OLD sealed (ADR-7): the passwords of pages with their versions, TOTP, the secrets of other systems
// and the NT hashes of Samba. A value that does not open with either key stops it, and the tenant it was in keeps
// everything as it was. It can run again: what the new key opens is left alone.
func rekey(ctx context.Context, stdout io.Writer) error {
	keys, err := vault.NewRotating(os.Getenv(vault.KeyEnv), os.Getenv(vault.OldKeyEnv))
	if err != nil {
		return err
	}
	if !keys.Ready() {
		return vault.ErrNoKey
	}
	if !keys.Rotating() {
		return errors.New("no old key: set " + vault.OldKeyEnv + " to the key before the rotation")
	}
	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()
	tenants, err := database.Tenants(ctx)
	if err != nil {
		return err
	}
	ids := []string{tenant.Hoster}
	for _, t := range tenants {
		ids = append(ids, t.ID)
	}
	total := 0
	for _, t := range ids {
		n, err := database.Rekey(ctx, t, func(name string, sealed []byte) ([]byte, error) { return keys.Reseal(t, name, sealed) })
		if err != nil {
			return fmt.Errorf("tenant %s: %w", t, err)
		}
		total += n
		if _, err := fmt.Fprintf(stdout, "%s: %d sealed again\n", t, n); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(stdout, "%d values sealed again under the new key; %s can go now\n", total, vault.OldKeyEnv)
	return err
}
