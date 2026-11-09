// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"testing"
)

func TestSettingAccess(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	if o, err := db.SetAccess(ctx, team.ID, anna.ID, "read"); err != nil || o.Access != "read" {
		t.Fatalf("the owner makes Team read only: %+v %v", o, err)
	}
	if got, _ := db.Access(ctx, page.ID); got != "read" {
		t.Errorf("the page inherits it: %q", got)
	}
	if _, err := db.CreatePage(ctx, bob, "Mine", team.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("others may not add to it now: %v", err)
	}
	if _, err := db.SetAccess(ctx, page.ID, anna.ID, "comment"); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.Access(ctx, page.ID); got != "comment" {
		t.Errorf("the page sets its own: %q", got)
	}
	if o, err := db.SetAccess(ctx, page.ID, anna.ID, ""); err != nil || o.Access != "" {
		t.Errorf("back to what is above: %+v %v", o, err)
	}
	if got, _ := db.Access(ctx, page.ID); got != "read" {
		t.Errorf("inherited again: %q", got)
	}
	if _, err := db.SetAccess(ctx, team.ID, anna.ID, "edit"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetAccess(ctx, team.ID, bob.ID, "read"); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob may edit Team, but not set what others may do: %v", err)
	}
	if _, err := db.SetAccess(ctx, team.ID, anna.ID, "admin"); !errors.Is(err, ErrPlace) {
		t.Errorf("no such level: %v", err)
	}
	line, _ := db.CreateDivider(ctx, anna, KindLine, team.ID, "")
	if _, err := db.SetAccess(ctx, line.ID, anna.ID, "read"); !errors.Is(err, ErrPlace) {
		t.Errorf("a line has no level: %v", err)
	}
	notes, _ := db.Collections(ctx, anna.ID)
	if _, err := db.SetAccess(ctx, notes[0].ID, bob.ID, "read"); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob does not see Anna's private notes: %v", err)
	}
}
