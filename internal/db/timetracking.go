// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/EightPerDay/core"
	"github.com/Ollornog/EightPerDay/core/pgstore"

	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// The time tracking is EightPerDay (docs/time-tracking.md), embedded as a library. Its tables live in this database
// with the prefix time_ and keep their tenant by the same setting as every other table, so the guard of ADR-23 reads
// them like the rest (TestEveryTableKeepsTenantsApart).

// TimeOptions are the options of EightPerDay's tables here. They are part of the schema: never change them.
var TimeOptions = pgstore.Options{Prefix: "time_", TenantSetting: "underdoc.tenant"}

// timeVersions is where the versions of EightPerDay's migrations start in schema_migrations: its migration n is
// version timeVersions+n. UnderDoc's own migrations stay below it and never touch the tables of the time tracking,
// which come after them.
const timeVersions = 1000

// migrateTime runs the migrations of EightPerDay not run yet, each in its own transaction, and then ties every table of
// the time tracking to its tenant (tenantReferences), as every table here is tied.
func migrateTime(ctx context.Context, conn *pgx.Conn) error {
	steps, err := pgstore.Migrations(TimeOptions)
	if err != nil {
		return err
	}
	for _, m := range steps {
		version := timeVersions + m.Version
		var done bool
		if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", version).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, m.SQL); err != nil {
				return fmt.Errorf("time tracking %03d_%s: %w", m.Version, m.Name, err)
			}
			if _, err := tx.Exec(ctx, tenantReferences); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// tenantReferences lets every table of the time tracking refer to tenants, so a tenant cannot go while a row of it is
// left (PurgeTenant). EightPerDay knows no table tenants, so UnderDoc adds the reference after its migrations.
const tenantReferences = `DO $$ DECLARE t text; BEGIN
	FOR t IN SELECT c.relname FROM pg_class c JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'tenant'
		WHERE c.relnamespace = 'public'::regnamespace AND c.relkind = 'r' AND c.relname LIKE 'time\_%'
		AND NOT EXISTS (SELECT 1 FROM pg_constraint k WHERE k.conrelid = c.oid AND k.contype = 'f' AND k.confrelid = 'tenants'::regclass)
	LOOP
		EXECUTE format('ALTER TABLE %I ADD FOREIGN KEY (tenant) REFERENCES tenants (id)', t);
	END LOOP;
END $$`

// TimeStore is EightPerDay's store on this database: every query runs as the tenant of its context, as all of this
// package does.
func (db *DB) TimeStore() *pgstore.Store { return db.time }

// timeEraseSetting lets PurgeTenant reach the access log of the time tracking, which otherwise only lets go of rows
// older than two years.
var timeEraseSetting = func() string {
	s, err := TimeOptions.EraseSetting()
	if err != nil {
		panic(err)
	}
	return s
}()

// personPattern is what EightPerDay takes as the ID of a person.
var personPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Person is the person in the time tracking of an account (M-3 of EightPerDay): its ID where EightPerDay takes it,
// otherwise a name drawn from it that stays the same.
func Person(accountID string) string {
	if personPattern.MatchString(accountID) {
		return accountID
	}
	sum := sha256.Sum256([]byte(accountID))
	return "a-" + hex.EncodeToString(sum[:16])
}

// TimeAccount is an account as the time tracking names it.
type TimeAccount struct {
	Person, Login, Name, Language string
}

// TimeAccounts returns every account of the tenant with its person, by name.
func (db *DB) TimeAccounts(ctx context.Context) ([]TimeAccount, error) {
	rows, err := db.pool.Query(ctx, "SELECT id, login, name FROM accounts ORDER BY lower(name), login")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (TimeAccount, error) {
		var id string
		var a TimeAccount
		err := row.Scan(&id, &a.Login, &a.Name)
		a.Person = Person(id)
		return a, err
	})
}

// TimeAccount returns the account of a person of the tenant.
func (db *DB) TimeAccount(ctx context.Context, person string) (TimeAccount, error) {
	all, err := db.TimeAccounts(ctx)
	if err != nil {
		return TimeAccount{}, err
	}
	i := slices.IndexFunc(all, func(a TimeAccount) bool { return a.Person == person })
	if i < 0 {
		return TimeAccount{}, ErrNotFound
	}
	return all[i], nil
}

// Organization is the organization of the tenant in the time tracking: one per tenant, its ID the tenant's.
func Organization(ctx context.Context) string {
	t, _ := tenant.From(ctx)
	return t
}

// MapTenant keeps the organization of the tenant in the time tracking (M-3 of EightPerDay): it is there with the
// tenant's name, and an account that administers the tenant administers it. An account that no longer administers the
// tenant stops administering the organization where this mapping made it do so; the administrators of the organization
// change every other role in the time tracking itself.
func (db *DB) MapTenant(ctx context.Context, accountID string, admin bool) error {
	t, ok := tenant.From(ctx)
	if !ok {
		return errors.New("no tenant")
	}
	info, err := db.Tenant(ctx, t)
	if err != nil {
		return err
	}
	org, err := db.time.Organization(ctx, t)
	switch {
	case errors.Is(err, core.ErrNotFound):
		org = core.Organization{ID: t, Name: info.Name, TimeZone: core.DefaultTimeZone}
		if err := db.time.PutOrganization(ctx, org); err != nil {
			return err
		}
	case err != nil:
		return err
	case org.Name != info.Name && strings.TrimSpace(info.Name) != "":
		org.Name = info.Name
		if err := db.time.PutOrganization(ctx, org); err != nil {
			return err
		}
	}
	person := Person(accountID)
	members, err := db.time.Memberships(ctx, t, person)
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation(core.DefaultTimeZone)
	if err != nil {
		return err
	}
	today := core.DateIn(time.Now(), loc)
	if !admin {
		// Whoever stops administering the tenant stops administering the organization where UnderDoc made them do so
		// (ADR-29); what the organization's own administrators gave them stays.
		for _, m := range members {
			if m.Role != core.RoleAdmin || !m.Active(today) || !strings.HasPrefix(m.ID, mappedAdmin(person)) {
				continue
			}
			if m.From.Before(today) {
				m.Until = today.AddDays(-1)
			} else {
				m.Role = core.RoleMember
			}
			if err := db.time.PutMembership(ctx, m); err != nil {
				return err
			}
		}
		return nil
	}
	if core.ActorFor(person, t, members, today).Admin {
		return nil
	}
	return db.time.PutMembership(ctx, core.Membership{ID: mappedAdmin(person) + strings.ReplaceAll(today.String(), "-", ""),
		Organization: t, Person: person, Role: core.RoleAdmin, From: today})
}

// mappedAdmin is the start of the ID of an administrator's membership that UnderDoc made, so it can end it again.
func mappedAdmin(person string) string {
	sum := sha256.Sum256([]byte(person))
	return "ud-admin-" + hex.EncodeToString(sum[:6]) + "-"
}
