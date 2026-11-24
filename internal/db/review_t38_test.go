// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/pagekind"
)

// The attack round T-38 in the database: who reads what an agent writes, and the paths that do not go through MCP.

// Path 1: the readers of an object in one short line, never with names of persons.
func TestTheReadersOfAnObject(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.createShared(ctx, anna, "Team")
	space, _ := db.CreateCollection(ctx, anna, "Sales", "")
	group, _ := db.CreateGroup(asPerson(anna), admin, "Sales", "", "")
	if err := db.SetMember(asPerson(anna), admin, group.ID, bob.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGrant(ctx, space.ID, admin, ToGroup, group.ID, RoleRead); err != nil {
		t.Fatal(err)
	}
	notes, _ := db.Collections(ctx, anna.ID)
	for name, try := range map[string]struct{ object, want string }{
		"everyone": {team.ID, "readers: everyone"},
		"a group":  {space.ID, "readers: Sales (1), 1 more person"},
		"private":  {notes[0].ID, "readers: only you"},
	} {
		r, err := db.ReadersOf(ctx, try.object)
		if err != nil {
			t.Fatal(err)
		}
		if got := r.Line(anna.ID); got != try.want || strings.Contains(got, "Anna") || strings.Contains(got, "Bob") {
			t.Errorf("%s: %q, want %q", name, got, try.want)
		}
	}
	// A public link counts, and a long line stays short.
	page, _ := db.CreatePage(ctx, anna, "Prices", space.ID)
	if _, _, err := db.CreateLink(ctx, page.ID, admin, NewLink{Mode: LinkRead}); err != nil {
		t.Fatal(err)
	}
	r, _ := db.ReadersOf(ctx, page.ID)
	if got := r.Line(anna.ID); got != "readers: Sales (1), 1 more person, 1 public link" {
		t.Errorf("with a link: %q", got)
	}
	long := Readers{IDs: []string{"a", "b"}, Groups: []GroupReaders{{"Customer service Europe", 12}, {"Sales and marketing", 9}, {"Board", 3}}, Links: 2}
	if got := long.Line("x"); len([]rune(got)) > 70 || !strings.Contains(got, "more") || !strings.HasSuffix(got, "2 public links") {
		t.Errorf("a long line: %q", got)
	}
	if !Wider(r, Readers{IDs: r.IDs}) || Wider(Readers{IDs: []string{anna.ID}}, Readers{IDs: []string{anna.ID, bob.ID}}) {
		t.Error("wider does not count links or accounts")
	}
}

// Path 1: a change of an agent that read narrower content is marked on every kind of page, shows to whoever checks,
// and is among the pages to check, until a person checks it.
func TestAWiderChangeIsMarked(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.createShared(ctx, anna, "Team")
	page, _ := db.CreatePageOf(ctx, anna, "Notes for agents", team.ID, "", pagekind.AgentNotes)
	plain, _ := db.AddBlock(asAgent(anna), page.ID, anna.ID, "", "Written without reading anything narrower.")
	marked, err := db.AddBlock(WithWider(asAgent(anna)), page.ID, anna.ID, "", "Written after reading the salaries.")
	if err != nil {
		t.Fatal(err)
	}
	changes, err := db.AIChanges(ctx, page.ID, anna.ID)
	if err != nil || len(changes) != 1 || changes[0].Block != marked.ID || !changes[0].Wider {
		t.Fatalf("the agent side shows only the marked change: %+v %v", changes, err)
	}
	_ = plain
	if pages, _ := db.PagesToCheck(ctx, anna.ID); !slices.Contains(pages, page.ID) {
		t.Errorf("the marked page is not among the pages to check: %v", pages)
	}
	if err := db.CheckChange(asPerson(anna), page.ID, marked.ID, anna.ID, changes[0].Head); err != nil {
		t.Fatal(err)
	}
	if changes, _ := db.AIChanges(ctx, page.ID, anna.ID); len(changes) != 0 {
		t.Errorf("after the check: %+v", changes)
	}
	// A later change of an agent without the mark keeps the mark of an unchecked one.
	b, _ := db.AddBlock(WithWider(asAgent(anna)), page.ID, anna.ID, "", "Again after reading.")
	if _, err := db.EditBlock(asAgent(anna), page.ID, b.ID, anna.ID, "Changed without reading."); err != nil {
		t.Fatal(err)
	}
	if changes, _ := db.AIChanges(ctx, page.ID, anna.ID); len(changes) != 1 || !changes[0].Wider {
		t.Errorf("a later change lost the mark: %+v", changes)
	}
	// On human documentation a later change of an agent without the mark keeps it too.
	howto, _ := db.CreatePageOf(ctx, anna, "How to pay", team.ID, "", pagekind.HowTo)
	h, _ := db.AddBlock(WithWider(asAgent(anna)), howto.ID, anna.ID, "", "After reading the salaries.")
	if _, err := db.EditBlock(asAgent(anna), howto.ID, h.ID, anna.ID, "Changed without reading."); err != nil {
		t.Fatal(err)
	}
	if changes, _ := db.AIChanges(ctx, howto.ID, anna.ID); len(changes) != 1 || !changes[0].Wider {
		t.Errorf("a later change on human documentation lost the mark: %+v", changes)
	}
}

// Path 6: an agent makes no page from a template fewer may read than the page's readers.
func TestAgentsMakeNoWiderPageFromATemplate(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.createShared(ctx, anna, "Team")
	notes, _ := db.Collections(ctx, anna.ID)
	private, _ := db.CreatePage(ctx, anna, "My offer", notes[0].ID)
	if _, err := db.AddBlock(ctx, private.ID, anna.ID, "", "Discount 40 percent."); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetTemplate(ctx, private.ID, anna.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreatePageOf(asAgent(anna), anna, "Offer", team.ID, private.ID, ""); !errors.Is(err, ErrPublish) {
		t.Errorf("an agent makes a team page from a private template: %v", err)
	}
	if _, err := db.CreatePageOf(asAgent(anna), anna, "Offer", notes[0].ID, private.ID, ""); err != nil {
		t.Errorf("an agent makes a private page from it: %v", err)
	}
	if _, err := db.CreatePageOf(asPerson(anna), anna, "Offer", team.ID, private.ID, ""); err != nil {
		t.Errorf("anna makes the team page herself: %v", err)
	}
	if _, err := db.CreatePageOf(asAgent(anna), anna, "Meeting", team.ID, BuiltIn+"meeting", ""); err != nil {
		t.Errorf("an agent makes a page from a template of UnderDoc's own: %v", err)
	}
}

// Path 9: rights hold by the object, not by its name or place: renamed or moved, a deny still holds, and nothing that
// lists or searches finds it for whom it is denied.
func TestRightsHoldThroughRenameAndMove(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.createShared(ctx, anna, "Team")
	other, _ := db.createShared(ctx, anna, "Other")
	folder, _ := db.CreateFolder(ctx, anna, "Board", team.ID)
	page, _ := db.CreatePage(ctx, anna, "Minutes", folder.ID)
	if err := db.SetEntries(ctx, folder.ID, admin, ToPerson, bob.ID, entries(ActRead, Deny), true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Rename(ctx, folder.ID, anna.ID, "Open board"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Move(ctx, folder.ID, anna.ID, other.ID, ""); err != nil {
		t.Fatal(err)
	}
	for _, object := range []string{folder.ID, page.ID} {
		if may(t, db, ctx, object, bob.ID, ActRead) {
			t.Errorf("after rename and move bob reads %s", object)
		}
	}
	kids, _ := db.Children(ctx, other.ID, bob.ID)
	for _, k := range kids {
		if k.ID == folder.ID {
			t.Error("bob lists the folder denied to him")
		}
	}
}
