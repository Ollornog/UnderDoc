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

func TestUsesBeforeDeleting(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	folder, _ := db.CreateFolder(ctx, anna, "Folder", team.ID)
	plan, _ := db.CreatePage(ctx, anna, "Plan", folder.ID)
	inside, _ := db.CreatePage(ctx, anna, "Inside", folder.ID)
	rack, _ := db.CreatePage(ctx, anna, "Rack", team.ID)
	gone, _ := db.CreatePage(ctx, anna, "Gone", team.ID)
	notes, _ := db.Collections(ctx, anna.ID)
	secret, _ := db.CreatePage(ctx, anna, "Secret", notes[0].ID)
	for _, p := range []Object{inside, rack, gone, secret} {
		if _, err := db.AddBlock(ctx, p.ID, anna.ID, "", "::embed{ref=\""+plan.ID+"\"}"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Delete(ctx, gone.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	titles, hidden, err := db.Uses(ctx, folder.ID, anna.ID)
	if err != nil || !slices.Equal(titles, []string{"Rack", "Secret"}) || hidden != 0 {
		t.Errorf("for Anna, the folder is used in Rack and Secret, not in Inside or the trash: %q %d %v", titles, hidden, err)
	}
	titles, hidden, _ = db.Uses(ctx, folder.ID, bob.ID)
	if !slices.Equal(titles, []string{"Rack"}) || hidden != 1 {
		t.Errorf("for bob, Secret is one page he cannot see: %q %d", titles, hidden)
	}
	if titles, _, _ := db.Uses(ctx, rack.ID, anna.ID); len(titles) != 0 {
		t.Errorf("Rack is used nowhere: %q", titles)
	}
}
