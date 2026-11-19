// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// sharedTables are the only tables without a tenant (ADR-23): which tenants exist, which host names lead to them, and
// which migrations ran. Nothing else may be added here without a decision: every other table holds what a tenant made.
var sharedTables = []string{"schema_migrations", "tenants", "tenant_hosts"}

// rule is the one condition a permissive policy may have: the row belongs to the tenant of the connection.
const rule = "(tenant = current_setting('underdoc.tenant'::text, true))"

// TestEveryTableKeepsTenantsApart is the guard of ADR-23: it fails for any table, also one added later, that does not
// carry its tenant and keep it by row-level security. Each table needs a tenant column that may not be empty and fills
// itself from the setting, row-level security turned on and forced, every permissive policy restricted to the tenant
// for reading and writing, a primary key with the tenant, and the tenant in every reference to another such table.
func TestEveryTableKeepsTenantsApart(t *testing.T) {
	db := open(t)
	ctx := inMain()
	rows, err := db.pool.Query(ctx, `
		SELECT c.relname, c.relrowsecurity, c.relforcerowsecurity,
			coalesce((SELECT a.attnotnull FROM pg_attribute a WHERE a.attrelid = c.oid AND a.attname = 'tenant' AND NOT a.attisdropped), false),
			coalesce((SELECT pg_get_expr(d.adbin, d.adrelid) FROM pg_attrdef d JOIN pg_attribute a ON a.attrelid = d.adrelid AND a.attnum = d.adnum
				WHERE d.adrelid = c.oid AND a.attname = 'tenant'), ''),
			(SELECT count(*) FROM pg_policy p WHERE p.polrelid = c.oid AND p.polpermissive),
			(SELECT count(*) FROM pg_policy p WHERE p.polrelid = c.oid AND p.polpermissive AND p.polcmd = '*'
				AND pg_get_expr(p.polqual, p.polrelid) = $1 AND pg_get_expr(p.polwithcheck, p.polrelid) = $1),
			coalesce((SELECT a.attname = 'tenant' FROM pg_index i JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = i.indkey[0]
				WHERE i.indrelid = c.oid AND i.indisprimary), false)
		FROM pg_class c
		WHERE c.relnamespace = 'public'::regnamespace AND c.relkind IN ('r', 'p')
		ORDER BY c.relname`, rule)
	if err != nil {
		t.Fatal(err)
	}
	type table struct {
		name                     string
		enabled, forced, notNull bool
		fill                     string
		policies, tenantOnly     int
		keyFirst                 bool
	}
	tables, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (table, error) {
		var x table
		err := row.Scan(&x.name, &x.enabled, &x.forced, &x.notNull, &x.fill, &x.policies, &x.tenantOnly, &x.keyFirst)
		return x, err
	})
	if err != nil {
		t.Fatal(err)
	}
	checked, timeTables := 0, 0
	// The tables of the rights (ADR-29) must be among them.
	rights := map[string]bool{"groups": false, "group_members": false, "grants": false, "share_links": false, "share_link_users": false,
		"share_link_passes": false, "audit_log": false, "log_settings": false}
	for _, x := range tables {
		if slices.Contains(sharedTables, x.name) {
			continue
		}
		checked++
		if _, ok := rights[x.name]; ok {
			rights[x.name] = true
		}
		if strings.HasPrefix(x.name, TimeOptions.Prefix) {
			timeTables++
		}
		switch {
		case !x.notNull:
			t.Errorf("%s: no column tenant, or one that may be empty", x.name)
		case x.fill != "current_setting('underdoc.tenant'::text, true)":
			t.Errorf("%s: the tenant does not fill itself from the setting: %q", x.name, x.fill)
		case !x.enabled || !x.forced:
			t.Errorf("%s: row-level security is not on and forced", x.name)
		case x.policies == 0 || x.policies != x.tenantOnly:
			t.Errorf("%s: %d permissive policies, %d of them keep to the tenant for all commands; every one must", x.name, x.policies, x.tenantOnly)
		case !x.keyFirst:
			t.Errorf("%s: the primary key does not start with the tenant", x.name)
		}
	}
	if checked < 6 {
		t.Fatalf("only %d tables checked, the query finds the tables no more", checked)
	}
	// The tables of the time tracking (EightPerDay) come from its own migrations and are held to the same rules.
	if timeTables < 6 {
		t.Fatalf("only %d tables of the time tracking checked", timeTables)
	}
	for name, seen := range rights {
		if !seen {
			t.Errorf("the table %s of the rights was not checked", name)
		}
	}

	// Every reference between tables of tenants carries the tenant on both sides, at the same place.
	refs, err := db.pool.Query(ctx, `
		SELECT c.conname, c.conrelid::regclass::text, c.confrelid::regclass::text,
			array_position(c.conkey, (SELECT attnum FROM pg_attribute WHERE attrelid = c.conrelid AND attname = 'tenant')),
			array_position(c.confkey, (SELECT attnum FROM pg_attribute WHERE attrelid = c.confrelid AND attname = 'tenant'))
		FROM pg_constraint c WHERE c.contype = 'f' AND c.connamespace = 'public'::regnamespace`)
	if err != nil {
		t.Fatal(err)
	}
	type reference struct {
		name, from, to string
		at, atTarget   *int32
	}
	all, err := pgx.CollectRows(refs, func(row pgx.CollectableRow) (reference, error) {
		var r reference
		err := row.Scan(&r.name, &r.from, &r.to, &r.at, &r.atTarget)
		return r, err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range all {
		if slices.Contains(sharedTables, r.from) || slices.Contains(sharedTables, r.to) {
			continue
		}
		if r.at == nil || r.atTarget == nil || *r.at != *r.atTarget {
			t.Errorf("%s: the reference from %s to %s does not carry the tenant on both sides", r.name, r.from, r.to)
		}
	}
}

// TestNoTableShowsAnotherTenant fills the tenant Main through the functions of this package, and then reads every
// table as another tenant and as no tenant: nothing shows. It goes over the tables of the database, so a table added
// later is read too.
func TestNoTableShowsAnotherTenant(t *testing.T) {
	db := open(t)
	main := inMain()
	if _, err := db.CreateTenant(main, "acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	shared, err := db.createShared(main, anna, "Operations")
	if err != nil {
		t.Fatal(err)
	}
	p, err := db.CreatePage(main, anna, "Plan", shared.ID)
	if err != nil {
		t.Fatal(err)
	}
	source, err := db.CreatePage(main, anna, "Source", shared.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddBlock(main, p.ID, anna.ID, "", "::embed{ref=\""+source.ID+"\"}"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddComment(main, p.ID, anna.ID, "", "Looks good", 0); err != nil {
		t.Fatal(err)
	}
	gone, err := db.CreatePage(main, anna, "Old", shared.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(main, gone.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	editors, err := db.CreateGroup(main, "Editors", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetMember(main, editors.ID, bob.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGrant(main, p.ID, Actor{ID: anna.ID}, ToGroup, editors.ID, RoleComment); err != nil {
		t.Fatal(err)
	}
	link, _, err := db.CreateLink(main, shared.ID, Actor{ID: anna.ID}, NewLink{Mode: LinkEdit})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UseLink(main, link.ID, bob.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GivePass(main, link.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Log(main, anna.ID, LogOpen, p.ID, ""); err != nil {
		t.Fatal(err)
	}
	fillTime(t, db, main)
	names, err := pgx.CollectRows(must(db.pool.Query(main, `SELECT relname FROM pg_class WHERE relnamespace = 'public'::regnamespace
		AND relkind IN ('r', 'p') ORDER BY relname`)), pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	filled := 0
	for _, name := range names {
		if slices.Contains(sharedTables, name) {
			continue
		}
		count := func(ctx context.Context) int {
			var n int
			if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{name}.Sanitize()).Scan(&n); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			return n
		}
		if count(main) > 0 {
			filled++
		} else {
			t.Logf("%s: no rows in Main, read as another tenant all the same", name)
		}
		if n := count(tenant.With(context.Background(), "acme")); n != 0 {
			t.Errorf("%s: the tenant acme sees %d rows of Main", name, n)
		}
		if n := count(context.Background()); n != 0 {
			t.Errorf("%s: a query without a tenant sees %d rows", name, n)
		}
	}
	if filled < 6 {
		t.Errorf("only %d tables have rows in Main, the test proves too little", filled)
	}
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

// TestTenantsWorkApart: two tenants with the same account and the same object IDs live side by side, each sees only its
// own, and nothing can be written into another tenant.
func TestTenantsWorkApart(t *testing.T) {
	db := open(t)
	main := inMain()
	if _, err := db.CreateTenant(main, "acme", "Acme Ltd"); err != nil {
		t.Fatal(err)
	}
	acme := tenant.With(context.Background(), "acme")
	if err := db.SeeAccount(acme, anna); err != nil {
		t.Fatal(err)
	}
	mine, err := db.createShared(main, anna, "Main things")
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := db.createShared(acme, anna, "Acme things")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		ctx       context.Context
		see, miss string
	}{{main, mine.ID, theirs.ID}, {acme, theirs.ID, mine.ID}} {
		list, err := db.Collections(c.ctx, anna.ID)
		if err != nil {
			t.Fatal(err)
		}
		ids := []string{}
		for _, o := range list {
			ids = append(ids, o.ID)
		}
		if !slices.Contains(ids, c.see) || slices.Contains(ids, c.miss) {
			t.Errorf("collections %v: want %s, not %s", ids, c.see, c.miss)
		}
		if _, err := db.Object(c.ctx, c.miss, anna.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("the other tenant's collection by its ID: %v", err)
		}
		if _, err := db.Rename(c.ctx, c.miss, anna.ID, "taken over"); !errors.Is(err, ErrNotFound) {
			t.Errorf("renaming the other tenant's collection: %v", err)
		}
	}
	// An import into acme with the IDs of Main keeps them: an ID is unique within its tenant only.
	imported, err := db.Import(acme, anna.Login, "Copy", &Node{Object: Object{ID: mine.ID, Kind: KindCollection, Title: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if imported.Top.ID != mine.ID || len(imported.Renamed) != 0 {
		t.Errorf("the ID of Main is free in acme: %+v", imported)
	}
	// A row written for another tenant is refused by the rules, whatever the statement says.
	_, err = db.pool.Exec(acme, "INSERT INTO objects (tenant, id, kind, title, owner) VALUES ('main', 'aaaaaaaaaa', 'collection', 'x', 'u-anna')")
	if err == nil || !strings.Contains(err.Error(), "row-level security") {
		t.Errorf("writing into Main as acme: %v", err)
	}
	_, err = db.pool.Exec(acme, "UPDATE objects SET tenant = 'main' WHERE id = $1", theirs.ID)
	if err == nil || !strings.Contains(err.Error(), "row-level security") {
		t.Errorf("moving a row to Main as acme: %v", err)
	}
	// Without a tenant, nothing can be written at all.
	if err := db.SeeAccount(context.Background(), bob); err == nil {
		t.Error("an account was written without a tenant")
	}
	// The way around the rules is closed for the connections of the pool.
	if _, err := db.pool.Exec(acme, "SET row_security = off"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.pool.QueryRow(acme, "SELECT count(*) FROM objects").Scan(&n); err == nil {
		t.Errorf("with row_security off, the pool read %d objects instead of failing", n)
	}
}

func TestTenantsAndHosts(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if _, err := db.CreateTenant(ctx, "Acme", "x"); !errors.Is(err, tenant.ErrID) {
		t.Errorf("upper case: %v", err)
	}
	if _, err := db.CreateTenant(ctx, "acme", " "); !errors.Is(err, ErrTitle) {
		t.Errorf("no name: %v", err)
	}
	acme, err := db.CreateTenant(ctx, "acme", " Acme Ltd ")
	if err != nil || acme.Name != "Acme Ltd" || acme.State != TenantActive {
		t.Fatalf("%+v %v", acme, err)
	}
	if _, err := db.CreateTenant(ctx, "acme", "again"); !errors.Is(err, ErrTenantTaken) {
		t.Errorf("twice: %v", err)
	}
	if err := db.AddTenantHost(ctx, "acme", "Docs.Acme.Example:443"); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"docs.acme.example", "x", "a..b", "-a.b", "a_b.example"} {
		err := db.AddTenantHost(ctx, "main", host)
		want := ErrHost
		if host == "docs.acme.example" {
			want = ErrHostTaken
		}
		if !errors.Is(err, want) {
			t.Errorf("%s: %v", host, err)
		}
	}
	if err := db.AddTenantHost(ctx, "nobody", "x.example"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown tenant: %v", err)
	}
	for host, want := range map[string]string{
		"acme.docs.example":    "acme",
		"MAIN.docs.example.":   "main",
		"docs.acme.example":    "acme",
		"hoster.docs.example":  "",
		"nobody.docs.example":  "",
		"a.acme.docs.example":  "",
		"docs.example":         "",
		"acme.other.example":   "",
		"docs.acme.example:80": "acme",
	} {
		got, err := db.TenantByHost(ctx, host, "docs.example")
		if want == "" {
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("%s leads to %q, %v", host, got.ID, err)
			}
			continue
		}
		if err != nil || got.ID != want {
			t.Errorf("%s: %q %v, want %s", host, got.ID, err, want)
		}
	}
	if _, err := db.SetTenantState(ctx, "acme", TenantArchived); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"acme.docs.example", "docs.acme.example"} {
		if _, err := db.TenantByHost(ctx, host, "docs.example"); !errors.Is(err, ErrNotFound) {
			t.Errorf("an archived tenant is found by %s: %v", host, err)
		}
	}
	ids, err := db.TenantIDs(ctx)
	if err != nil || !slices.Equal(ids, []string{"hoster", "main"}) {
		t.Errorf("jobs run for %v, %v", ids, err)
	}
	if _, err := db.SetTenantState(ctx, "acme", TenantLocked); err != nil {
		t.Fatal(err)
	}
	if got, err := db.TenantByHost(ctx, "acme.docs.example", "docs.example"); err != nil || got.State != TenantLocked {
		t.Errorf("a locked tenant is found, to say it is locked: %+v %v", got, err)
	}
	for _, id := range []string{"main", "hoster"} {
		if _, err := db.SetTenantState(ctx, id, TenantLocked); !errors.Is(err, ErrReserved) {
			t.Errorf("%s locked: %v", id, err)
		}
	}
	if err := db.AddTenantHost(ctx, "hoster", "admin.example"); !errors.Is(err, ErrReserved) {
		t.Errorf("a host for the hoster tenant: %v", err)
	}
	if err := db.RemoveTenantHost(ctx, "acme", "docs.acme.example"); err != nil {
		t.Fatal(err)
	}
	if err := db.RemoveTenantHost(ctx, "acme", "docs.acme.example"); !errors.Is(err, ErrNotFound) {
		t.Errorf("twice: %v", err)
	}
	list, err := db.Tenants(ctx)
	if err != nil || len(list) != 2 || list[0].ID != "acme" || list[1].ID != "main" {
		t.Errorf("%+v %v", list, err)
	}
}

// TestTrashExpiresForEveryTenant: the job that empties the trash runs for each tenant, as that tenant.
func TestTrashExpiresForEveryTenant(t *testing.T) {
	db := open(t)
	main := inMain()
	if _, err := db.CreateTenant(main, "acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	acme := tenant.With(context.Background(), "acme")
	if err := db.SeeAccount(acme, anna); err != nil {
		t.Fatal(err)
	}
	for _, ctx := range []context.Context{main, acme} {
		c, err := db.createShared(ctx, anna, "Gone")
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Delete(ctx, c.ID, anna.ID); err != nil {
			t.Fatal(err)
		}
	}
	// A tenant with the trash switched off keeps it (ADR-25).
	acmeOff := func(ctx context.Context) bool { t, _ := tenant.From(ctx); return t != "acme" }
	gone, err := db.ExpireAllTrash(context.Background(), time.Now().Add(time.Minute), acmeOff)
	if err != nil || gone != 1 {
		t.Errorf("%d gone, %v", gone, err)
	}
	gone, err = db.ExpireAllTrash(context.Background(), time.Now().Add(time.Minute), nil)
	if err != nil || gone != 1 {
		t.Errorf("%d gone, %v", gone, err)
	}
}

// TestOpenRefusesARoleThatPassesTheRules: a role with BYPASSRLS that cannot step down stops the start, and the owner of
// the tables without it works under FORCE ROW LEVEL SECURITY.
func TestOpenRefusesARoleThatPassesTheRules(t *testing.T) {
	url := pgtest.URL(t)
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(ctx) }()
	suffix := strings.TrimPrefix(url[strings.LastIndex(url, "/")+1:strings.Index(url, "?")], "t")
	bypasser, owner := "bypass_"+suffix, "owner_"+suffix
	for _, sql := range []string{
		"CREATE ROLE " + bypasser + " LOGIN BYPASSRLS",
		"CREATE ROLE " + owner + " LOGIN",
		"GRANT ALL ON SCHEMA public TO " + bypasser + ", " + owner,
	} {
		if _, err := admin.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	as := func(role string) string { return strings.Replace(url, "//underdoc@", "//"+role+"@", 1) }
	if _, err := Open(ctx, as(bypasser)); !errors.Is(err, ErrBypass) {
		t.Errorf("a role with BYPASSRLS: %v", err)
	}
	if _, err := admin.Exec(ctx, "DROP OWNED BY "+bypasser); err != nil {
		t.Fatal(err)
	}
	db, err := Open(ctx, as(owner))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	main := inMain()
	if err := db.SeeAccount(main, anna); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.pool.QueryRow(context.Background(), "SELECT count(*) FROM accounts").Scan(&n); err != nil || n != 0 {
		t.Errorf("the owner without a tenant sees %d accounts, %v", n, err)
	}
	if err := db.pool.QueryRow(main, "SELECT count(*) FROM accounts").Scan(&n); err != nil || n != 1 {
		t.Errorf("the owner in Main sees %d accounts, %v", n, err)
	}
}

// TestRenamingAndPurgingATenant: a tenant gets another name and keeps its ID; deleting it for good needs it archived and
// its ID typed, takes every row it has in every table and its addresses, and leaves Main as it was.
func TestRenamingAndPurgingATenant(t *testing.T) {
	db := open(t)
	main := inMain()
	if _, err := db.CreateTenant(main, "acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	if err := db.AddTenantHost(main, "acme", "wiki.acme.example"); err != nil {
		t.Fatal(err)
	}
	if got, err := db.RenameTenant(main, "acme", "  Acme Ltd "); err != nil || got.Name != "Acme Ltd" || got.ID != "acme" {
		t.Errorf("rename: %+v %v", got, err)
	}
	if _, err := db.RenameTenant(main, "acme", " "); !errors.Is(err, ErrTitle) {
		t.Errorf("an empty name: %v", err)
	}
	if _, err := db.RenameTenant(main, tenant.Hoster, "Mine"); !errors.Is(err, ErrReserved) {
		t.Errorf("the hoster tenant: %v", err)
	}
	if _, err := db.RenameTenant(main, "nobody", "X"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a tenant that is not there: %v", err)
	}
	// Content in both tenants: pages, blocks, references, comments, the trash, settings.
	fill := func(ctx context.Context) {
		t.Helper()
		if err := db.SeeAccount(ctx, anna); err != nil {
			t.Fatal(err)
		}
		c, err := db.createShared(ctx, anna, "Things")
		if err != nil {
			t.Fatal(err)
		}
		p, _ := db.CreatePage(ctx, anna, "Plan", c.ID)
		src, _ := db.CreatePage(ctx, anna, "Source", c.ID)
		if _, err := db.AddBlock(ctx, p.ID, anna.ID, "", "::embed{ref=\""+src.ID+"\"}"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.AddComment(ctx, p.ID, anna.ID, "", "Looks good", 0); err != nil {
			t.Fatal(err)
		}
		gone, _ := db.CreatePage(ctx, anna, "Old", c.ID)
		if err := db.Delete(ctx, gone.ID, anna.ID); err != nil {
			t.Fatal(err)
		}
		fillTime(t, db, ctx)
	}
	acme := tenant.With(context.Background(), "acme")
	fill(main)
	fill(acme)
	names, err := pgx.CollectRows(must(db.pool.Query(main, `SELECT c.relname FROM pg_class c JOIN pg_attribute a ON a.attrelid = c.oid
		AND a.attname = 'tenant' WHERE c.relnamespace = 'public'::regnamespace AND c.relkind IN ('r', 'p') AND c.relname NOT IN ('tenants', 'tenant_hosts')
		ORDER BY c.relname`)), pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	counts := func(ctx context.Context) map[string]int {
		out := map[string]int{}
		for _, name := range names {
			var n int
			if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{name}.Sanitize()).Scan(&n); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if n > 0 {
				out[name] = n
			}
		}
		return out
	}
	before := counts(main)
	if len(counts(acme)) < 6 {
		t.Fatalf("acme has too little to prove anything: %v", counts(acme))
	}
	for _, name := range timeTableNames {
		if counts(acme)[name] == 0 {
			t.Fatalf("acme has no rows in %s", name)
		}
	}
	if err := db.PurgeTenant(main, "acme", "acme"); !errors.Is(err, ErrNotArchived) {
		t.Errorf("an active tenant: %v", err)
	}
	if _, err := db.SetTenantState(main, "acme", TenantArchived); err != nil {
		t.Fatal(err)
	}
	if err := db.PurgeTenant(main, "acme", "Acme"); !errors.Is(err, ErrConfirm) {
		t.Errorf("a confirmation that is not the ID: %v", err)
	}
	for _, reserved := range []string{tenant.Main, tenant.Hoster} {
		if err := db.PurgeTenant(main, reserved, reserved); !errors.Is(err, ErrReserved) {
			t.Errorf("%s: %v", reserved, err)
		}
	}
	if err := db.PurgeTenant(main, "acme", "acme"); err != nil {
		t.Fatal(err)
	}
	if got := counts(acme); len(got) != 0 {
		t.Errorf("rows of acme are left: %v", got)
	}
	if _, err := db.Tenant(main, "acme"); !errors.Is(err, ErrNotFound) {
		t.Errorf("the tenant is still there: %v", err)
	}
	if _, err := db.TenantByHost(main, "wiki.acme.example", ""); err == nil {
		t.Error("its address still leads somewhere")
	}
	if got := counts(main); !maps.Equal(got, before) {
		t.Errorf("Main changed:\n got %v\nwant %v", got, before)
	}
	// The ID is free again.
	if _, err := db.CreateTenant(main, "acme", "Acme again"); err != nil {
		t.Errorf("a new tenant with the ID: %v", err)
	}
}
