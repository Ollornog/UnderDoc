// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/EightPerDay/core"

	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// timeTableNames are the tables of the time tracking that fillTime fills.
var timeTableNames = []string{"time_absences", "time_access_log", "time_entries", "time_records", "time_revisions", "time_timers"}

// fillTime writes something into every table of the time tracking, in the tenant of ctx: the organization with a
// member, a private and a managed employer, an entry and an absence with their history, a line of the access log and a
// running stopwatch.
func fillTime(t *testing.T, db *DB, ctx context.Context) {
	t.Helper()
	s := db.TimeStore()
	org := Organization(ctx)
	old := core.MustDate("2020-01-06")
	for _, err := range []error{
		s.PutOrganization(ctx, core.Organization{ID: org, Name: "Org"}),
		s.Put(ctx, core.Employer{ID: "e1", Owner: "u-anna", Name: "Own", Kind: core.KindPrivate, Mode: core.ModeExact}),
		s.Put(ctx, core.Employer{ID: "m1", Owner: "u-anna", Name: "Org", Kind: core.KindEmployer, Holder: core.HolderManaged,
			Organization: org, Mode: core.ModeSchedule}),
		s.PutMembership(ctx, core.Membership{ID: "ms1", Organization: org, Person: "u-anna", Role: core.RoleMember,
			EmployerID: "m1", From: old}),
		s.StartTimer(ctx, "u-anna", core.Timer{EmployerID: "e1", Started: time.Date(2026, 11, 18, 7, 0, 0, 0, time.UTC)}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateEntry(ctx, core.Entry{ID: "x1", Owner: "u-anna", EmployerID: "e1", Kind: core.EntryWork, Date: old,
		Minutes: 60}, core.Change{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateAbsence(ctx, core.Absence{ID: "y1", Owner: "u-anna", EmployerID: "m1", Type: core.AbsenceVacation,
		From: old, To: old, Status: core.StatusApproved}, core.Change{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LogAccess(ctx, core.Access{Actor: "u-boss", Organization: org, Subject: "u-anna", Action: core.AccessRead,
		What: "entries"}); err != nil {
		t.Fatal(err)
	}
}

// TestTimeTrackingMigrates: EightPerDay's migrations run in UnderDoc's flow, after its own, each once, and every table of
// the time tracking refers to tenants.
func TestTimeTrackingMigrates(t *testing.T) {
	db := open(t)
	ctx := inMain()
	versions, err := pgx.CollectRows(must(db.pool.Query(ctx, "SELECT version FROM schema_migrations WHERE version > $1 ORDER BY version",
		timeVersions)), pgx.RowTo[int])
	if err != nil || len(versions) < 3 || versions[0] != timeVersions+1 {
		t.Fatalf("versions of the time tracking: %v %v", versions, err)
	}
	tables, err := pgx.CollectRows(must(db.pool.Query(ctx, `SELECT c.relname FROM pg_class c WHERE c.relnamespace = 'public'::regnamespace
		AND c.relkind = 'r' AND c.relname LIKE 'time\_%' AND EXISTS (SELECT 1 FROM pg_constraint k WHERE k.conrelid = c.oid
		AND k.contype = 'f' AND k.confrelid = 'tenants'::regclass) ORDER BY c.relname`)), pgx.RowTo[string])
	slices.Sort(tables)
	if err != nil || !slices.Equal(tables, timeTableNames) {
		t.Fatalf("tables of the time tracking that refer to tenants: %v %v", tables, err)
	}
	// A second run changes nothing.
	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "RESET ROLE"); err != nil {
		t.Fatal(err)
	}
	if err := migrateTime(ctx, conn.Conn()); err != nil {
		t.Fatalf("second run: %v", err)
	}
	// A row of a tenant that does not exist is refused.
	if _, err := db.pool.Exec(tenant.With(context.Background(), "nobody"), "INSERT INTO time_timers (owner, employer_id, started) VALUES ('a', 'e', now())"); err == nil {
		t.Error("a row of a tenant that does not exist")
	}
}

func TestPerson(t *testing.T) {
	for id, want := range map[string]string{"u-anna": "u-anna", "3f2a_9": "3f2a_9"} {
		if got := Person(id); got != want {
			t.Errorf("%s: %s", id, got)
		}
	}
	mail := Person("anna@example.com")
	if mail == "anna@example.com" || len(mail) != 34 || mail != Person("anna@example.com") || mail == Person("ben@example.com") {
		t.Errorf("an ID EightPerDay does not take: %q", mail)
	}
	if !personPattern.MatchString(mail) {
		t.Errorf("the drawn person is no ID of EightPerDay: %q", mail)
	}
}

// TestMapTenant: the tenant is an organization in the time tracking, with its name; its administrators administer it,
// once, and nobody else is made a member by the mapping.
func TestMapTenant(t *testing.T) {
	db := open(t)
	main := inMain()
	if err := db.SeeAccount(main, anna); err != nil {
		t.Fatal(err)
	}
	if err := db.MapTenant(main, anna.ID, false); err != nil {
		t.Fatal(err)
	}
	s := db.TimeStore()
	org, err := s.Organization(main, tenant.Main)
	if err != nil || org.Name != "Main" {
		t.Fatalf("organization: %+v %v", org, err)
	}
	if ms, _ := s.Memberships(main, tenant.Main, ""); len(ms) != 0 {
		t.Fatalf("a user who is no administrator became a member: %+v", ms)
	}
	for range 2 {
		if err := db.MapTenant(main, "anna@example.com", true); err != nil {
			t.Fatal(err)
		}
	}
	ms, err := s.Memberships(main, tenant.Main, "")
	if err != nil || len(ms) != 1 || ms[0].Role != core.RoleAdmin || ms[0].Person != Person("anna@example.com") {
		t.Fatalf("the administrator's membership, once: %+v %v", ms, err)
	}
	if _, err := db.RenameTenant(main, tenant.Main, "Main office"); err != nil {
		t.Fatal(err)
	}
	if err := db.MapTenant(main, anna.ID, false); err != nil {
		t.Fatal(err)
	}
	if org, _ := s.Organization(main, tenant.Main); org.Name != "Main office" {
		t.Errorf("the organization keeps the old name: %+v", org)
	}
	// Another tenant has an organization of its own.
	if _, err := db.CreateTenant(main, "acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	acme := tenant.With(context.Background(), "acme")
	if err := db.MapTenant(acme, anna.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Organization(acme, tenant.Main); err == nil {
		t.Error("acme sees the organization of Main")
	}
	if ms, _ := s.Memberships(acme, "", ""); len(ms) != 1 || ms[0].Organization != "acme" {
		t.Errorf("acme's memberships: %+v", ms)
	}
	if err := db.MapTenant(context.Background(), anna.ID, true); err == nil {
		t.Error("mapped without a tenant")
	}
	accounts, err := db.TimeAccounts(main)
	if err != nil || !slices.ContainsFunc(accounts, func(a TimeAccount) bool { return a.Person == "u-anna" && a.Login == "anna" }) {
		t.Fatalf("accounts: %+v %v", accounts, err)
	}
	if a, err := db.TimeAccount(main, "u-anna"); err != nil || a.Name != "Anna Berg" {
		t.Errorf("account of a person: %+v %v", a, err)
	}
	if _, err := db.TimeAccount(acme, "u-anna"); err == nil {
		t.Error("acme finds an account of Main")
	}
}
