// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestEverythingReadsAllButTheTrash(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	ops, _ := db.CreateCollection(ctx, bob, "Ops", "")
	a, _ := db.CreatePage(ctx, anna, "A", team.ID)
	gone, _ := db.CreatePage(ctx, anna, "Gone", team.ID)
	first, _ := db.AddBlock(ctx, a.ID, anna.ID, "", "one")
	second, _ := db.AddBlock(ctx, a.ID, anna.ID, first.ID, "two")
	if err := db.DeleteBlock(ctx, a.ID, second.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(ctx, gone.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	shared, private, err := db.Everything(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := titles([]Object{shared[0].Object, shared[1].Object}); len(shared) != 2 || !slices.Equal(got, []string{"Team", "Ops"}) || shared[1].ID != ops.ID {
		t.Fatalf("shared: %q", got)
	}
	if kids := shared[0].Kids; len(kids) != 1 || kids[0].ID != a.ID {
		t.Fatalf("the trash stays out: %+v", kids)
	}
	page := shared[0].Kids[0]
	if len(page.Blocks) != 1 || page.Blocks[0] != first || page.NextBlock != 3 {
		t.Errorf("the page with its blocks and the next number: %+v", page)
	}
	logins := map[string]string{}
	for _, o := range private {
		logins[o.Login] = o.Node.Title
	}
	if len(private) != 2 || logins["anna"] != MyNotes || logins["bob"] != MyNotes {
		t.Errorf("the private notes of every account, with its login: %+v", logins)
	}
}

func TestSubtreeIsWhatTheViewerSees(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	a, _ := db.CreatePage(ctx, anna, "A", team.ID)
	if _, err := db.CreatePage(ctx, anna, "Kid", a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddBlock(ctx, a.ID, anna.ID, "", "text"); err != nil {
		t.Fatal(err)
	}
	tree, err := db.Subtree(ctx, team.ID, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Kids) != 1 || tree.Kids[0].Blocks[0].Body != "text" || tree.Kids[0].NextBlock != 2 || tree.Kids[0].Kids[0].Title != "Kid" {
		t.Errorf("the whole tree: %+v", tree.Kids)
	}
	notes, _ := db.Collections(ctx, anna.ID)
	if _, err := db.Subtree(ctx, notes[0].ID, bob.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Anna's notes for Bob: %v", err)
	}
	exportLimit = 2
	t.Cleanup(func() { exportLimit = MaxExport })
	if _, err := db.Subtree(ctx, team.ID, anna.ID); !errors.Is(err, ErrTooMany) {
		t.Errorf("more than the limit: %v", err)
	}
}

func TestImportKeepsFreeIDsAndTouchesNothing(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	tree := &Node{Object: Object{ID: team.ID, Kind: KindFolder, Description: "about", Access: "read", Private: true}, Kids: []*Node{
		{Object: Object{ID: "7k2m9q4x1c", Kind: KindPage, Title: "Kept", Access: "comment"}, NextBlock: 9, Blocks: []Block{{ID: "b4", Body: "four"}, {ID: "b2", Body: "two"}}},
		{Object: Object{ID: "7k2m9q4x1c", Kind: KindPage, Title: "Twice"}, Blocks: []Block{{ID: "b20", Body: "x"}}},
		{Object: Object{ID: "ops.plan", Kind: KindLine, Title: "anything", Description: "dropped"}},
		{Object: Object{Kind: KindFolder, Title: "Folder"}, Kids: []*Node{{Object: Object{Kind: KindPage, Title: "Deep", Private: true}}}},
	}}
	done, err := db.Import(ctx, "bob", "Imported", tree)
	if err != nil {
		t.Fatal(err)
	}
	top := done.Top
	if top.Title != "Imported" || top.Kind != KindCollection || top.Owner != bob.ID || top.ID == team.ID || !top.Private || top.Access != "read" || top.Description != "about" {
		t.Errorf("the top: %+v", top)
	}
	if done.Objects != 6 || len(done.Renamed) != 3 || done.Renamed[team.ID] != top.ID || done.Renamed["ops.plan"] == "" {
		t.Errorf("what it did: %+v", done)
	}
	if again, _ := db.Object(ctx, team.ID, anna.ID); again.Title != "Team" || again.Owner != anna.ID {
		t.Errorf("the collection whose ID was taken is untouched: %+v", again)
	}
	kids, _ := db.Children(ctx, top.ID, bob.ID)
	var got []string
	for _, k := range kids {
		got = append(got, k.Kind+":"+k.Title)
	}
	if !slices.Equal(got, []string{"page:Kept", "page:Twice", "line:line", "folder:Folder"}) || kids[0].ID != "7k2m9q4x1c" || kids[1].ID == "7k2m9q4x1c" {
		t.Fatalf("children in their order, IDs kept where free: %q %+v", got, kids)
	}
	if kids[0].Access != "comment" || kids[2].Description != "" {
		t.Errorf("access kept, a line is only a line: %+v", kids)
	}
	if blocks, _ := db.Blocks(ctx, kids[0].ID, bob.ID); len(blocks) != 2 || blocks[0].ID != "b4" || blocks[1].ID != "b2" {
		t.Errorf("the blocks in their order with their IDs: %+v", blocks)
	}
	if b, _ := db.AddBlock(ctx, kids[0].ID, bob.ID, "", "new"); b.ID != "b9" {
		t.Errorf("the next block keeps to next_block: %q", b.ID)
	}
	if b, _ := db.AddBlock(ctx, kids[1].ID, bob.ID, "", "new"); b.ID != "b21" {
		t.Errorf("the next block comes after the highest: %q", b.ID)
	}
	if versions, _ := db.Versions(ctx, kids[1].ID, bob.ID, 10); len(versions) != 2 || versions[1].Author != "Bob" {
		t.Errorf("every block starts its history: %+v", versions)
	}
	deep, _ := db.Children(ctx, kids[3].ID, bob.ID)
	if len(deep) != 1 || deep[0].Private {
		t.Errorf("only the top is private: %+v", deep)
	}
	if _, err := db.Import(ctx, "nobody", "X", &Node{}); !errors.Is(err, ErrNoAccount) {
		t.Errorf("an unknown owner: %v", err)
	}
	bad := &Node{Kids: []*Node{{Object: Object{Kind: KindPage, Title: "P"}, Blocks: []Block{{ID: "x1", Body: "x"}}}}}
	if _, err := db.Import(ctx, "bob", "Bad", bad); err == nil {
		t.Error("a block without a valid ID is refused")
	}
	if all, _ := db.Collections(ctx, bob.ID); slices.Contains(titles(all), "Bad") {
		t.Error("a failed import leaves nothing behind")
	}
}

// An imported page embeds what its blocks name, so the source knows where it is used.
func TestImportKeepsTheReferences(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	plan, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	tree := &Node{Object: Object{Kind: KindCollection}, Kids: []*Node{
		{Object: Object{Kind: KindPage, Title: "Rack"}, Blocks: []Block{{ID: "b1", Body: "Ports:\n\n::embed{ref=\"" + plan.ID + "#b1\"}"}}},
	}}
	if _, err := db.Import(ctx, "anna", "Imported", tree); err != nil {
		t.Fatal(err)
	}
	if refs, _ := db.UsedIn(ctx, plan.ID, anna.ID); len(refs) != 1 || refs[0].Title != "Rack" || refs[0].TargetBlock != "b1" {
		t.Errorf("Plan is used in the imported page: %+v", refs)
	}
}
