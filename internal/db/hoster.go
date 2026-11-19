// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// What the hoster sets per tenant (ADR-23) and the modules of a tenant (ADR-25). The hoster sees counts, never content.

// HosterLimits are what the hoster allows a tenant; nil means no limit, and Modules nil means all the instance allows.
// A change past one fails with ErrQuota, as one past a quota of the files does.
type HosterLimits struct {
	MaxUsers   *int
	MaxStorage *int64
	Modules    []string
}

// Usage is how much a tenant uses: its accounts, and its storage: the bytes of the current blocks of its pages and of
// its files, their versions, attachments and running uploads (ADR-28).
type Usage struct {
	Users   int
	Storage int64
}

// TenantLimits returns the limits of a tenant.
func (db *DB) TenantLimits(ctx context.Context, tenantID string) (HosterLimits, error) {
	var l HosterLimits
	err := db.pool.QueryRow(ctx, "SELECT max_users, max_storage, modules FROM tenants WHERE id = $1", tenantID).Scan(&l.MaxUsers, &l.MaxStorage, &l.Modules)
	if errors.Is(err, pgx.ErrNoRows) {
		return HosterLimits{}, ErrNotFound
	}
	return l, err
}

// SetTenantLimits sets the limits of a tenant. Main has none in the mode one tenant, but the hoster may set them.
func (db *DB) SetTenantLimits(ctx context.Context, tenantID string, l HosterLimits) error {
	if l.MaxUsers != nil && *l.MaxUsers < 0 || l.MaxStorage != nil && *l.MaxStorage < 0 {
		return errors.New("a limit is not negative")
	}
	if tenantID == tenant.Hoster {
		return ErrReserved
	}
	tag, err := db.pool.Exec(ctx, "UPDATE tenants SET max_users = $2, max_storage = $3, modules = $4, changed = now() WHERE id = $1",
		tenantID, l.MaxUsers, l.MaxStorage, l.Modules)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// TenantUsage counts what a tenant uses, as the hoster may see it: numbers only.
func (db *DB) TenantUsage(ctx context.Context, tenantID string) (Usage, error) {
	var u Usage
	err := db.pool.QueryRow(tenant.With(ctx, tenantID), `SELECT (SELECT count(*) FROM accounts), `+storageUsed).Scan(&u.Users, &u.Storage)
	return u, err
}

// storageUsed is the storage of the tenant of the connection, as its hoster limits it: blocks and files. Triggers keep it
// in tenant_storage (migration 021); the tests hold it to the same counted the long way.
const storageUsed = `(SELECT coalesce((SELECT bytes FROM tenant_storage WHERE tenant = current_setting('underdoc.tenant', true)), 0))`

type queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// roomForUser fails with ErrQuota when the tenant of ctx has as many accounts as its hoster allows.
func roomForUser(ctx context.Context, q queryer) error {
	var full bool
	err := q.QueryRow(ctx, `SELECT coalesce((SELECT max_users FROM tenants WHERE id = current_setting('underdoc.tenant', true))
		<= (SELECT count(*) FROM accounts), false)`).Scan(&full)
	if err == nil && full {
		return ErrQuota
	}
	return err
}

// roomForBytes fails with ErrQuota when growing the blocks of the tenant of ctx by grow bytes would pass its limit.
// Shrinking always works.
func roomForBytes(ctx context.Context, q queryer, grow int64) error {
	if grow <= 0 {
		return nil
	}
	var full bool
	err := q.QueryRow(ctx, `SELECT coalesce((SELECT max_storage FROM tenants WHERE id = current_setting('underdoc.tenant', true))
		< `+storageUsed+` + $1, false)`, grow).Scan(&full)
	if err == nil && full {
		return ErrQuota
	}
	return err
}

// withinStorage fails with ErrQuota when the blocks of the tenant of ctx, as written so far in a transaction, pass its
// limit; the transaction then writes nothing.
func withinStorage(ctx context.Context, q queryer) error {
	var over bool
	err := q.QueryRow(ctx, `SELECT coalesce((SELECT max_storage FROM tenants WHERE id = current_setting('underdoc.tenant', true))
		< `+storageUsed+`, false)`).Scan(&over)
	if err == nil && over {
		return ErrQuota
	}
	return err
}

// --- Modules

// TenantModules returns the modules the hoster allows the tenant of ctx (nil: all of the instance) and those its
// administrator switched on (nil: the defaults).
func (db *DB) TenantModules(ctx context.Context) (allowed, on []string, err error) {
	t, _ := tenant.From(ctx)
	if err := db.pool.QueryRow(ctx, "SELECT modules FROM tenants WHERE id = $1", t).Scan(&allowed); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, err
	}
	err = db.pool.QueryRow(ctx, "SELECT enabled FROM module_settings").Scan(&on)
	if errors.Is(err, pgx.ErrNoRows) {
		return allowed, nil, nil
	}
	return allowed, on, err
}

// SetTenantModules keeps the modules the administrator switched on. Nothing of their data changes.
func (db *DB) SetTenantModules(ctx context.Context, on []string) error {
	on = nonNil(slices.Compact(slices.Sorted(slices.Values(on))))
	_, err := db.pool.Exec(ctx, `INSERT INTO module_settings (enabled) VALUES ($1)
		ON CONFLICT (tenant) DO UPDATE SET enabled = EXCLUDED.enabled, changed = now()`, on)
	return err
}

// HiddenModules returns the modules a user hides.
func (db *DB) HiddenModules(ctx context.Context, accountID string) ([]string, error) {
	var hidden []string
	err := db.pool.QueryRow(ctx, "SELECT hidden_modules FROM accounts WHERE id = $1", accountID).Scan(&hidden)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return hidden, err
}

// SetHiddenModules keeps the modules a user hides.
func (db *DB) SetHiddenModules(ctx context.Context, accountID string, hidden []string) error {
	hidden = nonNil(slices.Compact(slices.Sorted(slices.Values(hidden))))
	if len(hidden) > 100 {
		return fmt.Errorf("too many modules")
	}
	return db.updateLocalOrNot(ctx, "UPDATE accounts SET hidden_modules = $2 WHERE id = $1", accountID, hidden)
}

func (db *DB) updateLocalOrNot(ctx context.Context, sql string, args ...any) error {
	tag, err := db.pool.Exec(ctx, sql, args...)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
