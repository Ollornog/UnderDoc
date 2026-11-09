// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestTags(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	if got, err := CleanTags([]string{" Network ", "#vlan", "network", "", "main-site", "v1.2_x"}); err != nil || !slices.Equal(got, []string{"network", "vlan", "main-site", "v1.2_x"}) {
		t.Errorf("clean: %q %v", got, err)
	}
	for _, bad := range [][]string{{"two words"}, {"-lead"}, {"<b>"}, {strings.Repeat("a", 41)}} {
		if _, err := CleanTags(bad); !errors.Is(err, ErrTag) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	many := make([]string, 51)
	for i := range many {
		many[i] = "t" + strconv.Itoa(i)
	}
	if _, err := CleanTags(many); !errors.Is(err, ErrTag) {
		t.Errorf("51 tags: %v", err)
	}
	if got, err := CleanTags(many[:50]); err != nil || len(got) != 50 {
		t.Errorf("50 tags are fine: %d %v", len(got), err)
	}
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	plan, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	o, err := db.SetTags(ctx, plan.ID, anna.ID, []string{"Network", "#Rack"})
	if err != nil || !slices.Equal(o.Tags, []string{"network", "rack"}) {
		t.Fatalf("set: %q %v", o.Tags, err)
	}
	if again, _ := db.Object(ctx, plan.ID, bob.ID); !slices.Equal(again.Tags, []string{"network", "rack"}) {
		t.Errorf("kept: %q", again.Tags)
	}
	if fresh, _ := db.CreatePage(ctx, anna, "Fresh", team.ID); fresh.Tags == nil || len(fresh.Tags) != 0 {
		t.Errorf("a new object has no tags, not null: %#v", fresh.Tags)
	}
	line, _ := db.CreateDivider(ctx, anna, KindLine, team.ID, "")
	if _, err := db.SetTags(ctx, line.ID, anna.ID, []string{"x"}); !errors.Is(err, ErrPlace) {
		t.Errorf("a line takes no tags: %v", err)
	}
	if _, err := db.SetAccess(ctx, team.ID, anna.ID, "comment"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetTags(ctx, plan.ID, bob.ID, []string{"mine"}); err == nil {
		t.Errorf("bob may only comment, so he tags nothing")
	}
	if _, err := db.Copy(ctx, plan.ID, anna.ID, team.ID, plan.ID); err != nil {
		t.Fatal(err)
	}
	kids, _ := db.Children(ctx, team.ID, anna.ID)
	found := false
	for _, k := range kids {
		if k.ID != plan.ID && strings.HasPrefix(k.Title, "Plan") {
			found = slices.Equal(k.Tags, []string{"network", "rack"})
		}
	}
	if !found {
		t.Errorf("a copy keeps the tags: %+v", kids)
	}
}
