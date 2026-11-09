// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"slices"
	"testing"
)

func TestReferencesFollowTheBlocks(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	if got := RefsIn("text\n::embed{ref=\"7k2m9q4x1c#b3\"}\n  ::embed{ref=\"7k2m9q4x1c\"}  \n::embed{ref=\"7k2m9q4x1c#b3\"}\nnot ::embed{ref=\"7k2m9q4x1c\"} inline\nsee ::embed{ref=\"7k2m9q4x1c#b9\"}\n::embed{ref=\"bad\"}"); !slices.Equal(got, [][2]string{{"7k2m9q4x1c", "b3"}, {"7k2m9q4x1c", ""}}) {
		t.Errorf("each embedding once, on a line of its own: %q", got)
	}
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	plan, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	rack, _ := db.CreatePage(ctx, anna, "Rack", team.ID)
	b, _ := db.AddBlock(ctx, rack.ID, anna.ID, "", "::embed{ref=\""+plan.ID+"#b1\"}")
	used := func() []string {
		refs, _ := db.UsedIn(ctx, plan.ID, bob.ID)
		var out []string
		for _, r := range refs {
			out = append(out, r.Title+":"+r.Block+">"+r.TargetBlock)
		}
		return out
	}
	if got := used(); !slices.Equal(got, []string{"Rack:" + b.ID + ">b1"}) {
		t.Errorf("a new block's embedding is kept: %q", got)
	}
	if _, err := db.EditBlock(ctx, rack.ID, b.ID, anna.ID, "::embed{ref=\""+plan.ID+"\"}"); err != nil {
		t.Fatal(err)
	}
	if got := used(); !slices.Equal(got, []string{"Rack:" + b.ID + ">"}) {
		t.Errorf("an edit replaces it: %q", got)
	}
	if _, err := db.Copy(ctx, rack.ID, anna.ID, team.ID, rack.ID); err != nil {
		t.Fatal(err)
	}
	if got := used(); len(got) != 2 {
		t.Errorf("a copy embeds too: %q", got)
	}
	copies, _ := db.Children(ctx, team.ID, anna.ID)
	if err := db.Delete(ctx, copies[2].ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if got := used(); len(got) != 1 {
		t.Errorf("a page in the trash uses nothing: %q", got)
	}
	if err := db.DeleteBlock(ctx, rack.ID, b.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if got := used(); len(got) != 0 {
		t.Errorf("a deleted block embeds nothing: %q", got)
	}
	notes, _ := db.Collections(ctx, anna.ID)
	secret, _ := db.CreatePage(ctx, anna, "Secret", notes[0].ID)
	if _, err := db.AddBlock(ctx, secret.ID, anna.ID, "", "::embed{ref=\""+plan.ID+"\"}"); err != nil {
		t.Fatal(err)
	}
	if got := used(); len(got) != 0 {
		t.Errorf("bob does not learn of Anna's private page: %q", got)
	}
	if refs, _ := db.UsedIn(ctx, plan.ID, anna.ID); len(refs) != 1 || refs[0].Title != "Secret" {
		t.Errorf("Anna does: %+v", refs)
	}
}
