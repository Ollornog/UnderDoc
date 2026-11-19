// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"testing"
)

// TestSettingAccess sets the level of ADR-20 for all others, which ADR-29 keeps as the role of the whole tenant: a
// narrower level breaks the inheritance, "" takes it from above again.
func TestSettingAccess(t *testing.T) {
	db := open(t)
	ctx := inMain()
	team, _ := db.createShared(ctx, anna, "Team")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	if o, err := db.SetAccess(ctx, team.ID, Actor{ID: anna.ID}, "read"); err != nil || o.Access != "read" {
		t.Fatalf("the owner makes Team read only: %+v %v", o, err)
	}
	if got, _ := db.Role(ctx, page.ID, bob.ID); got != RoleRead {
		t.Errorf("the page inherits it: %q", got)
	}
	if _, err := db.CreatePage(ctx, bob, "Mine", team.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("others may not add to it now: %v", err)
	}
	if _, err := db.SetAccess(ctx, page.ID, Actor{ID: anna.ID}, "comment"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.Role(ctx, page.ID, bob.ID); got != RoleComment {
		t.Errorf("the page sets its own: %q", got)
	}
	if o, err := db.SetAccess(ctx, page.ID, Actor{ID: anna.ID}, ""); err != nil || o.Access != "" || o.Breaks {
		t.Errorf("back to what is above: %+v %v", o, err)
	}
	if got, _ := db.Role(ctx, page.ID, bob.ID); got != RoleRead {
		t.Errorf("inherited again: %q", got)
	}
	// At the top, "" is the default of a new collection since 2026-11-19: its owner's alone (ADR-29).
	if o, err := db.SetAccess(ctx, team.ID, Actor{ID: anna.ID}, ""); err != nil || o.Access != "" {
		t.Fatalf("at the top, \"\" takes the tenant's grant back: %+v %v", o, err)
	}
	if got, _ := db.Role(ctx, page.ID, bob.ID); got != RoleNone {
		t.Errorf("Team is anna's alone now: %q", got)
	}
	if o, err := db.SetAccess(ctx, team.ID, Actor{ID: anna.ID}, RoleEdit); err != nil || o.Access != RoleEdit {
		t.Fatalf("edit for the tenant again: %+v %v", o, err)
	}
	// Narrower below: the page breaks the inheritance, and only Anna, who owns it, keeps more than read.
	if o, err := db.SetAccess(ctx, page.ID, Actor{ID: anna.ID}, "read"); err != nil || !o.Breaks || o.Access != "read" {
		t.Fatalf("a narrower level breaks the inheritance: %+v %v", o, err)
	}
	if got, _ := db.Role(ctx, page.ID, bob.ID); got != RoleRead {
		t.Errorf("bob only reads the page now: %q", got)
	}
	if got, _ := db.Role(ctx, page.ID, anna.ID); got != RoleOwner {
		t.Errorf("anna owns it: %q", got)
	}
	if _, err := db.SetAccess(ctx, team.ID, Actor{ID: bob.ID}, "read"); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob may edit Team, but not set what others may do: %v", err)
	}
	if _, err := db.SetAccess(ctx, team.ID, Actor{ID: bob.ID, Admin: true}, "comment"); err != nil {
		t.Errorf("an administrator manages the rights: %v", err)
	}
	if _, err := db.SetAccess(ctx, team.ID, Actor{ID: anna.ID}, "admin"); !errors.Is(err, ErrPlace) {
		t.Errorf("no such level: %v", err)
	}
	line, _ := db.CreateDivider(ctx, anna, KindLine, team.ID, "")
	if _, err := db.SetAccess(ctx, line.ID, Actor{ID: anna.ID}, "read"); !errors.Is(err, ErrPlace) {
		t.Errorf("a line has no level: %v", err)
	}
	notes, _ := db.Collections(ctx, anna.ID)
	if _, err := db.SetAccess(ctx, notes[0].ID, Actor{ID: bob.ID}, "read"); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob does not see Anna's private notes: %v", err)
	}
	if _, err := db.SetAccess(ctx, notes[0].ID, Actor{ID: bob.ID, Admin: true}, "read"); !errors.Is(err, ErrNotFound) {
		t.Errorf("nor does an administrator: %v", err)
	}
	if _, err := db.SetAccess(ctx, notes[0].ID, Actor{ID: anna.ID}, "read"); !errors.Is(err, ErrPrivate) {
		t.Errorf("private stays private, also for its owner: %v", err)
	}
}
