// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// TestTheLog: opening is kept once an hour, changes of rights each time; administrators read all of it, everyone else
// what they did and what happened to what they manage; what lies in private notes shows to its owner alone (ADR-29).
func TestTheLog(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.SeeAccount(ctx, carl); err != nil {
		t.Fatal(err)
	}
	team, _ := db.createShared(ctx, anna, "Team")
	notes, _ := db.Collections(ctx, anna.ID)
	for range 3 {
		if err := db.Log(ctx, bob.ID, LogOpen, team.ID, ""); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := db.Log(ctx, anna.ID, LogGrant, team.ID, "person u-bob read"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Log(ctx, anna.ID, LogOpen, notes[0].ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.Log(ctx, carl.ID, LogOpen, "", ""); err != nil {
		t.Fatal(err)
	}
	all, err := db.LogEntries(ctx, Actor{ID: carl.ID, Admin: true}, LogFilter{})
	if err != nil {
		t.Fatal(err)
	}
	// bob's three openings are one, anna's two grants are two, carl's own line; anna's notes stay hers.
	if len(all) != 4 || all[0].Actor != carl.ID || all[len(all)-1].ActorName != "Bob" || all[len(all)-1].ObjectTitle != "Team" {
		t.Fatalf("an administrator reads the log without anna's notes: %+v", all)
	}
	mine, _ := db.LogEntries(ctx, Actor{ID: anna.ID}, LogFilter{})
	if len(mine) != 4 {
		t.Errorf("anna reads what she did, her notes, and what happened to Team, which she owns: %d", len(mine))
	}
	bobs, _ := db.LogEntries(ctx, Actor{ID: bob.ID}, LogFilter{})
	if len(bobs) != 1 || bobs[0].Action != LogOpen {
		t.Errorf("bob reads what he did: %+v", bobs)
	}
	if only, _ := db.LogEntries(ctx, Actor{ID: anna.ID}, LogFilter{Object: team.ID, Actor: anna.ID}); len(only) != 2 {
		t.Errorf("narrowed to anna on Team: %d", len(only))
	}
	if page, _ := db.LogEntries(ctx, Actor{ID: carl.ID, Admin: true}, LogFilter{Before: all[1].ID, Limit: 1}); len(page) != 1 || page[0].ID != all[2].ID {
		t.Errorf("the next page: %+v", page)
	}
}

// TestTheLogKeepsItsDays: an entry of the log stays 365 days unless the tenant sets 30 to 3650, each tenant its own; the
// job that empties the trash deletes what is older, and only in the tenant it is older for (ADR-29, 2026-11-19). The
// time is staged: entries are dated back, and the moment of the expiry is given.
func TestTheLogKeepsItsDays(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if _, err := db.CreateTenant(ctx, "acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	acme := tenant.With(context.Background(), "acme")
	if days, err := db.LogKeep(ctx); err != nil || days != LogKeepDays || LogKeepDays != 365 {
		t.Fatalf("a year by default: %d %v", days, err)
	}
	for _, bad := range []int{0, 29, 3651} {
		if err := db.SetLogKeep(ctx, bad); !errors.Is(err, ErrLogKeep) {
			t.Errorf("%d days: %v", bad, err)
		}
	}
	// One entry in each tenant at each age, in days.
	now := time.Date(2027, 3, 1, 12, 0, 0, 0, time.UTC)
	ages := []int{10, 100, 364, 366, 4000}
	for _, c := range []context.Context{ctx, acme} {
		for _, age := range ages {
			if _, err := db.pool.Exec(c, "INSERT INTO audit_log (actor, action, detail, at) VALUES ('', 'open', $1, $2)",
				strconv.Itoa(age), now.AddDate(0, 0, -age)); err != nil {
				t.Fatal(err)
			}
		}
	}
	left := func(c context.Context) string {
		t.Helper()
		var kept []string
		rows, err := db.pool.Query(c, "SELECT detail FROM audit_log WHERE action = 'open' ORDER BY at DESC")
		if err != nil {
			t.Fatal(err)
		}
		kept, err = pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join(kept, " ")
	}
	if gone, err := db.ExpireLog(ctx, now); err != nil || gone != 2 || left(ctx) != "10 100 364" {
		t.Errorf("a year: %d gone, %q left, %v", gone, left(ctx), err)
	}
	if left(acme) != "10 100 364 366 4000" {
		t.Errorf("acme lost entries through main: %q", left(acme))
	}
	if err := db.SetLogKeep(acme, 30); err != nil {
		t.Fatal(err)
	}
	if days, _ := db.LogKeep(ctx); days != 365 {
		t.Errorf("main keeps its year when acme sets its own: %d", days)
	}
	if gone, err := db.ExpireLog(acme, now); err != nil || gone != 4 || left(acme) != "10" {
		t.Errorf("acme keeps 30 days: %d gone, %q left, %v", gone, left(acme), err)
	}
	if err := db.SetLogKeep(ctx, 3650); err != nil {
		t.Fatal(err)
	}
	if days, _ := db.LogKeep(ctx); days != 3650 {
		t.Errorf("ten years: %d", days)
	}
	// The job of the trash does it by itself: an entry dated back more than a year goes, one of today stays.
	if _, err := db.pool.Exec(acme, "UPDATE log_settings SET keep_days = 365"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.pool.Exec(acme, "INSERT INTO audit_log (actor, action, detail, at) VALUES ('', 'open', 'old', now() - interval '400 days'), ('', 'open', 'new', now())"); err != nil {
		t.Fatal(err)
	}
	keep, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { db.KeepTrash(keep, 10*time.Millisecond, nil, func(err error) { t.Error(err) }); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && strings.Contains(left(acme), "old") {
		time.Sleep(10 * time.Millisecond)
	}
	stop()
	<-done
	if got := left(acme); got != "10 new" {
		t.Errorf("the job deletes what is older than a year: %q", got)
	}
}
