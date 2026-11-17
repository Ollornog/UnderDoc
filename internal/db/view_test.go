// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"testing"
)

func TestView(t *testing.T) {
	db := open(t)
	ctx := inMain()
	text := func(s string) *string { return &s }
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	plan, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	if plan.Width != "" || plan.Contents != "" {
		t.Errorf("a new page has the defaults of the workspace: %q %q", plan.Width, plan.Contents)
	}
	o, err := db.SetView(ctx, plan.ID, anna.ID, text("full"), nil)
	if err != nil || o.Width != "full" || o.Contents != "" {
		t.Fatalf("width: %+v %v", o, err)
	}
	if o, err = db.SetView(ctx, plan.ID, anna.ID, nil, text("side")); err != nil || o.Width != "full" || o.Contents != "side" {
		t.Errorf("contents keep the width: %+v %v", o, err)
	}
	if again, _ := db.Object(ctx, plan.ID, bob.ID); again.Width != "full" || again.Contents != "side" {
		t.Errorf("kept: %q %q", again.Width, again.Contents)
	}
	for _, bad := range [][2]*string{{text("huge"), nil}, {nil, text("top")}, {text("FULL"), nil}} {
		if _, err := db.SetView(ctx, plan.ID, anna.ID, bad[0], bad[1]); !errors.Is(err, ErrView) {
			t.Errorf("%v: %v", bad, err)
		}
	}
	if o, err = db.SetView(ctx, plan.ID, anna.ID, text(""), nil); err != nil || o.Width != "" || o.Contents != "side" {
		t.Errorf("empty goes back to the default: %+v %v", o, err)
	}
	line, _ := db.CreateDivider(ctx, anna, KindLine, team.ID, "")
	if _, err := db.SetView(ctx, line.ID, anna.ID, text("wide"), nil); !errors.Is(err, ErrPlace) {
		t.Errorf("a line has no view: %v", err)
	}
	if _, err := db.SetView(ctx, plan.ID, anna.ID, text("slim"), nil); err != nil {
		t.Fatal(err)
	}
	copied, err := db.Copy(ctx, plan.ID, anna.ID, team.ID, plan.ID)
	if err != nil || copied.Width != "slim" || copied.Contents != "side" {
		t.Errorf("a copy keeps the view: %+v %v", copied, err)
	}
	if _, err := db.SetAccess(ctx, team.ID, anna.ID, "comment"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetView(ctx, plan.ID, bob.ID, text("wide"), nil); err == nil {
		t.Errorf("bob may only comment, so he sets no view")
	}
}

func TestAlignBlock(t *testing.T) {
	db := open(t)
	ctx := inMain()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	plan, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	b1, _ := db.AddBlock(ctx, plan.ID, anna.ID, "", "Title")
	b2, _ := db.AddBlock(ctx, plan.ID, anna.ID, b1.ID, "Text")
	before, _ := db.Versions(ctx, plan.ID, anna.ID, 100)
	if err := db.AlignBlock(ctx, plan.ID, b1.ID, anna.ID, "center"); err != nil {
		t.Fatal(err)
	}
	if err := db.AlignBlock(ctx, plan.ID, b2.ID, bob.ID, "right"); err != nil {
		t.Fatalf("others may edit here: %v", err)
	}
	blocks, _ := db.Blocks(ctx, plan.ID, anna.ID)
	if blocks[0].Align != "center" || blocks[1].Align != "right" || blocks[0].Body != "Title" {
		t.Errorf("aligned: %+v", blocks)
	}
	if after, _ := db.Versions(ctx, plan.ID, anna.ID, 100); len(after) != len(before) {
		t.Errorf("aligning makes no version: %d, before %d", len(after), len(before))
	}
	if err := db.AlignBlock(ctx, plan.ID, b2.ID, anna.ID, "left"); err != nil {
		t.Fatal(err)
	}
	if blocks, _ = db.Blocks(ctx, plan.ID, anna.ID); blocks[1].Align != "" {
		t.Errorf("left is the default: %q", blocks[1].Align)
	}
	if err := db.AlignBlock(ctx, plan.ID, b2.ID, anna.ID, "justify"); !errors.Is(err, ErrAlign) {
		t.Errorf("justify: %v", err)
	}
	if err := db.AlignBlock(ctx, plan.ID, "b99", anna.ID, "center"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a block that is not there: %v", err)
	}
	copied, err := db.Copy(ctx, plan.ID, anna.ID, team.ID, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if blocks, _ = db.Blocks(ctx, copied.ID, anna.ID); blocks[0].Align != "center" {
		t.Errorf("a copy keeps the alignment: %+v", blocks)
	}
	if _, err := db.SetAccess(ctx, team.ID, anna.ID, "read"); err != nil {
		t.Fatal(err)
	}
	if err := db.AlignBlock(ctx, plan.ID, b1.ID, bob.ID, "right"); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob may only read: %v", err)
	}
}
