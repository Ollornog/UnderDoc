// SPDX-License-Identifier: MIT

package db

import "testing"

// TestTheLog: opening is kept once an hour, changes of rights each time; administrators read all of it, everyone else
// what they did and what happened to what they manage; what lies in private notes shows to its owner alone (ADR-29).
func TestTheLog(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.SeeAccount(ctx, carl); err != nil {
		t.Fatal(err)
	}
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
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
