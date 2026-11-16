// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestTemplates(t *testing.T) {
	db := open(t)
	ctx := context.Background()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	list, err := db.Templates(ctx, anna.ID)
	if err != nil || len(list) != len(BuiltInTemplates) || list[0].ID != "builtin:meeting" {
		t.Fatalf("UnderDoc's own come first: %+v %v", list, err)
	}
	meeting, err := db.CreatePageFrom(ctx, anna, "Monday", team.ID, "builtin:meeting")
	if err != nil {
		t.Fatal(err)
	}
	blocks, _ := db.Blocks(ctx, meeting.ID, anna.ID)
	if len(blocks) != 5 || blocks[0].ID != "b1" || blocks[4].ID != "b5" || !strings.HasPrefix(blocks[1].Body, "## Agenda") {
		t.Errorf("the page starts with the template's blocks: %+v", blocks)
	}
	if b, _ := db.AddBlock(ctx, meeting.ID, anna.ID, "", "next"); b.ID != "b6" {
		t.Errorf("the next block after them: %s", b.ID)
	}
	if v, _ := db.Versions(ctx, meeting.ID, anna.ID, 100); len(v) != 6 {
		t.Errorf("each block has its version: %d", len(v))
	}

	// A page of the team offered as a template, with an embedding.
	own, _ := db.CreatePage(ctx, anna, "Site visit", team.ID)
	b1, _ := db.AddBlock(ctx, own.ID, anna.ID, "", "## Site")
	db.AddBlock(ctx, own.ID, anna.ID, b1.ID, "::embed{ref=\""+meeting.ID+"#b1\"}")
	if _, err := db.CreatePageFrom(ctx, anna, "Visit", team.ID, own.ID); !errors.Is(err, ErrTemplate) {
		t.Errorf("a page not offered as a template: %v", err)
	}
	if o, err := db.SetTemplate(ctx, own.ID, anna.ID, true); err != nil || !o.Template {
		t.Fatalf("offer: %+v %v", o, err)
	}
	list, _ = db.Templates(ctx, bob.ID)
	if last := list[len(list)-1]; last.ID != own.ID || last.Place != "Team" || last.Title != "Site visit" {
		t.Errorf("bob sees the template of the team: %+v", last)
	}
	visit, err := db.CreatePageFrom(ctx, bob, "Visit Tuesday", team.ID, own.ID)
	if err != nil {
		t.Fatal(err)
	}
	if blocks, _ = db.Blocks(ctx, visit.ID, bob.ID); len(blocks) != 2 || blocks[1].Body != "::embed{ref=\""+meeting.ID+"#b1\"}" || visit.Template || visit.Owner != bob.ID {
		t.Errorf("a page from a template: %+v %+v", visit, blocks)
	}
	if used, _ := db.UsedIn(ctx, meeting.ID, anna.ID); !slices.ContainsFunc(used, func(r Ref) bool { return r.Page == visit.ID }) {
		t.Errorf("the embedding of the new page is known: %+v", used)
	}
	for _, bad := range []string{"builtin:nothing", "zzzzzzzzzz"} {
		if _, err := db.CreatePageFrom(ctx, anna, "x", team.ID, bad); !errors.Is(err, ErrTemplate) {
			t.Errorf("%s: %v", bad, err)
		}
	}
	if _, err := db.SetTemplate(ctx, team.ID, anna.ID, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("only a page is a template: %v", err)
	}
	// A private template stays with its owner.
	notes, _ := db.Collections(ctx, anna.ID)
	mine, _ := db.CreatePage(ctx, anna, "Diary", notes[0].ID)
	db.SetTemplate(ctx, mine.ID, anna.ID, true)
	if list, _ = db.Templates(ctx, bob.ID); slices.ContainsFunc(list, func(t Template) bool { return t.ID == mine.ID }) {
		t.Errorf("bob sees anna's private template")
	}
	if _, err := db.CreatePageFrom(ctx, bob, "x", team.ID, mine.ID); !errors.Is(err, ErrTemplate) {
		t.Errorf("bob starts from anna's private template: %v", err)
	}
	if _, err := db.SetTemplate(ctx, own.ID, anna.ID, false); err != nil {
		t.Fatal(err)
	}
	if list, _ = db.Templates(ctx, anna.ID); slices.ContainsFunc(list, func(t Template) bool { return t.ID == own.ID }) {
		t.Errorf("a page no longer offered is no template")
	}
}
