// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// Tenants (ADR-23). The tables tenants and tenant_hosts are the only ones without row-level security: they say which
// tenants exist and which host names lead to them, and hold no content.

// States of a tenant.
const (
	TenantActive   = "active"
	TenantLocked   = "locked"
	TenantArchived = "archived"
)

// Tenant is one tenant, without anything of its content.
type Tenant struct {
	ID      string
	Name    string
	State   string
	Hosts   []string
	Created time.Time
}

// Errors about tenants.
var (
	ErrTenantTaken = errors.New("this tenant ID is taken")
	ErrHostTaken   = errors.New("this host name leads to another tenant already")
	ErrHost        = errors.New("not a host name")
	ErrReserved    = errors.New("this tenant belongs to the instance and cannot be changed")
	// ErrNotArchived is a purge of a tenant that is not archived: only what was archived first is deleted for good.
	ErrNotArchived = errors.New("only an archived tenant can be deleted for good")
	// ErrConfirm is a purge whose confirmation is not the ID of the tenant.
	ErrConfirm = errors.New("type the ID of the tenant to delete it for good")
)

const tenantColumns = `t.id, t.name, t.state, t.created, coalesce((SELECT array_agg(h.host ORDER BY h.host) FROM tenant_hosts h WHERE h.tenant = t.id), '{}')`

func scanTenant(row pgx.Row) (Tenant, error) {
	var t Tenant
	err := row.Scan(&t.ID, &t.Name, &t.State, &t.Created, &t.Hosts)
	if errors.Is(err, pgx.ErrNoRows) {
		return Tenant{}, ErrNotFound
	}
	return t, err
}

// TenantByHost finds the tenant a host name leads to: a subdomain of the hoster's domain, or a domain of its own. The
// tenant of the hoster administrators is never found this way, and neither is an archived one.
func (db *DB) TenantByHost(ctx context.Context, host, domain string) (Tenant, error) {
	host = tenant.Host(host)
	if label, ok := tenant.Subdomain(host, domain); ok {
		t, err := db.Tenant(ctx, label)
		if err == nil && t.ID != tenant.Hoster && t.State != TenantArchived {
			return t, nil
		}
		if err != nil && !errors.Is(err, ErrNotFound) {
			return Tenant{}, err
		}
	}
	t, err := scanTenant(db.pool.QueryRow(ctx, `SELECT `+tenantColumns+` FROM tenants t JOIN tenant_hosts x ON x.tenant = t.id
		WHERE x.host = $1 AND t.id <> $2 AND t.state <> 'archived'`, host, tenant.Hoster))
	return t, err
}

// Tenant returns one tenant by its ID.
func (db *DB) Tenant(ctx context.Context, tenantID string) (Tenant, error) {
	return scanTenant(db.pool.QueryRow(ctx, `SELECT `+tenantColumns+` FROM tenants t WHERE t.id = $1`, tenantID))
}

// Tenants returns every tenant but the one of the hoster administrators, in the order of their IDs.
func (db *DB) Tenants(ctx context.Context) ([]Tenant, error) {
	rows, err := db.pool.Query(ctx, `SELECT `+tenantColumns+` FROM tenants t WHERE t.id <> $1 ORDER BY t.id`, tenant.Hoster)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Tenant, error) { return scanTenant(row) })
}

// CreateTenant makes a new, empty tenant.
func (db *DB) CreateTenant(ctx context.Context, tenantID, name string) (Tenant, error) {
	if !tenant.Valid(tenantID) {
		return Tenant{}, tenant.ErrID
	}
	name = strings.TrimSpace(name)
	if n := len([]rune(name)); n < 1 || n > 200 {
		return Tenant{}, ErrTitle
	}
	_, err := db.pool.Exec(ctx, "INSERT INTO tenants (id, name) VALUES ($1, $2)", tenantID, name)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return Tenant{}, ErrTenantTaken
	}
	if err != nil {
		return Tenant{}, err
	}
	return db.Tenant(ctx, tenantID)
}

// SetTenantState locks, opens, archives or restores a tenant. Nothing of its content changes: archiving is the delete
// of the hoster, and an archived tenant comes back whole. Main and the hoster tenant cannot be changed.
func (db *DB) SetTenantState(ctx context.Context, tenantID, state string) (Tenant, error) {
	switch state {
	case TenantActive, TenantLocked, TenantArchived:
	default:
		return Tenant{}, errors.New("unknown state " + state)
	}
	if tenantID == tenant.Main || tenantID == tenant.Hoster {
		return Tenant{}, ErrReserved
	}
	tag, err := db.pool.Exec(ctx, "UPDATE tenants SET state = $2, changed = now() WHERE id = $1", tenantID, state)
	if err != nil {
		return Tenant{}, err
	}
	if tag.RowsAffected() == 0 {
		return Tenant{}, ErrNotFound
	}
	return db.Tenant(ctx, tenantID)
}

// RenameTenant gives a tenant another name; its ID, and with it its addresses, stays. The hoster tenant cannot be renamed.
func (db *DB) RenameTenant(ctx context.Context, tenantID, name string) (Tenant, error) {
	if tenantID == tenant.Hoster {
		return Tenant{}, ErrReserved
	}
	name = strings.TrimSpace(name)
	if n := len([]rune(name)); n < 1 || n > 200 {
		return Tenant{}, ErrTitle
	}
	tag, err := db.pool.Exec(ctx, "UPDATE tenants SET name = $2, changed = now() WHERE id = $1", tenantID, name)
	if err != nil {
		return Tenant{}, err
	}
	if tag.RowsAffected() == 0 {
		return Tenant{}, ErrNotFound
	}
	return db.Tenant(ctx, tenantID)
}

// PurgeTenant deletes an archived tenant for good: every row it has in every table, its addresses and the tenant itself,
// in one transaction that works as that tenant, so row-level security keeps every other tenant out of reach. confirm
// must be the ID of the tenant. The files of its content store and its search index are the caller's (store, search).
// Main and the hoster tenant cannot be deleted.
func (db *DB) PurgeTenant(ctx context.Context, tenantID, confirm string) error {
	if tenantID == tenant.Main || tenantID == tenant.Hoster {
		return ErrReserved
	}
	if confirm != tenantID {
		return ErrConfirm
	}
	t, err := db.Tenant(ctx, tenantID)
	if err != nil {
		return err
	}
	if t.State != TenantArchived {
		return ErrNotArchived
	}
	rows, err := db.pool.Query(ctx, `SELECT c.relname FROM pg_class c JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'tenant'
		WHERE c.relnamespace = 'public'::regnamespace AND c.relkind IN ('r', 'p') AND c.relname NOT IN ('tenants', 'tenant_hosts')
		ORDER BY c.relname`)
	if err != nil {
		return err
	}
	tables, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	return pgx.BeginFunc(tenant.With(ctx, tenantID), db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('underdoc.tenant', $1, true)", tenantID); err != nil {
			return err
		}
		// The access log of the time tracking lets go of its rows only for a tenant deleted for good (EightPerDay).
		if _, err := tx.Exec(ctx, "SELECT set_config($1, $2, true)", timeEraseSetting, tenantID); err != nil {
			return err
		}
		// The tables refer to each other; a table whose rows are still referred to waits for the next round.
		for len(tables) > 0 {
			var left []string
			var last error
			for _, name := range tables {
				if _, err := tx.Exec(ctx, "SAVEPOINT purge"); err != nil {
					return err
				}
				_, err := tx.Exec(ctx, "DELETE FROM "+pgx.Identifier{name}.Sanitize()+" WHERE tenant = $1", tenantID)
				var pg *pgconn.PgError
				if errors.As(err, &pg) && pg.Code == "23503" {
					if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT purge"); err != nil {
						return err
					}
					left, last = append(left, name), err
					continue
				}
				if err != nil {
					return err
				}
			}
			if len(left) == len(tables) {
				return last
			}
			tables = left
		}
		if _, err := tx.Exec(ctx, "DELETE FROM tenant_hosts WHERE tenant = $1", tenantID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "DELETE FROM tenants WHERE id = $1", tenantID)
		return err
	})
}

// AddTenantHost lets a domain of its own lead to a tenant. TLS for it is the job of the proxy in front.
func (db *DB) AddTenantHost(ctx context.Context, tenantID, host string) error {
	host = tenant.Host(host)
	if !validHost(host) {
		return ErrHost
	}
	if tenantID == tenant.Hoster {
		return ErrReserved
	}
	_, err := db.pool.Exec(ctx, "INSERT INTO tenant_hosts (host, tenant) VALUES ($1, $2)", host, tenantID)
	var pg *pgconn.PgError
	switch {
	case errors.As(err, &pg) && pg.Code == "23505":
		return ErrHostTaken
	case errors.As(err, &pg) && pg.Code == "23503":
		return ErrNotFound
	}
	return err
}

// RemoveTenantHost takes a domain away from a tenant.
func (db *DB) RemoveTenantHost(ctx context.Context, tenantID, host string) error {
	tag, err := db.pool.Exec(ctx, "DELETE FROM tenant_hosts WHERE host = $1 AND tenant = $2", tenant.Host(host), tenantID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func validHost(host string) bool {
	if len(host) < 1 || len(host) > 253 || !strings.Contains(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || !hostLabel(label) {
			return false
		}
	}
	return true
}

func hostLabel(label string) bool {
	for i, r := range label {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		case r == '-' && i > 0 && i < len(label)-1:
		default:
			return false
		}
	}
	return true
}

// TenantIDs returns the IDs of every tenant that is not archived, for the jobs that run for each of them.
func (db *DB) TenantIDs(ctx context.Context) ([]string, error) {
	rows, err := db.pool.Query(ctx, "SELECT id FROM tenants WHERE state <> 'archived' ORDER BY id")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// ForEachTenant runs a job for every tenant that is not archived, each in a context of that tenant, and goes on after a
// failure; it returns all failures together.
func (db *DB) ForEachTenant(ctx context.Context, job func(context.Context) error) error {
	ids, err := db.TenantIDs(ctx)
	if err != nil {
		return err
	}
	var failed error
	for _, t := range ids {
		failed = errors.Join(failed, job(tenant.With(ctx, t)))
	}
	return failed
}
