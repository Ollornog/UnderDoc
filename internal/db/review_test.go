// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/pagekind"
)

func changeOf(t *testing.T, changes []AIChange, block string) *AIChange {
	t.Helper()
	for i := range changes {
		if changes[i].Block == block {
			return &changes[i]
		}
	}
	return nil
}

// TestAIChangesAreVisible: every version knows whether an agent wrote it; a change by an agent of human documentation
// carries a hint until a person changes the block, and further changes by agents extend it; the agent side carries none
// (ADR-33).
func TestAIChangesAreVisible(t *testing.T) {
	db := open(t)
	ctx := inMain()
	agent, person := asAgent(anna), asPerson(anna)
	team, _ := db.createShared(ctx, anna, "Team")
	guide, _ := db.CreatePageOf(person, anna, "Restore", team.ID, "", pagekind.HowTo)
	first, _ := db.AddBlock(person, guide.ID, anna.ID, "", "This guide brings files back.")
	second, _ := db.AddBlock(person, guide.ID, anna.ID, first.ID, "1. Open the backup.")
	if changes, _ := db.AIChanges(ctx, guide.ID, anna.ID); len(changes) != 0 {
		t.Fatalf("a person's text carries no hint: %+v", changes)
	}
	if _, err := db.EditBlock(agent, guide.ID, second.ID, anna.ID, "1. Open the backup of last night."); err != nil {
		t.Fatal(err)
	}
	added, _ := db.AddBlock(agent, guide.ID, anna.ID, second.ID, "2. Copy the files.")
	versions, _ := db.Versions(ctx, guide.ID, anna.ID, 10)
	if len(versions) != 4 || !versions[0].Agent || !versions[1].Agent || versions[2].Agent || versions[3].Agent {
		t.Errorf("the versions know who wrote them: %+v", versions)
	}
	changes, _ := db.AIChanges(ctx, guide.ID, bob.ID)
	edited, made := changeOf(t, changes, second.ID), changeOf(t, changes, added.ID)
	if len(changes) != 2 || edited == nil || made == nil || edited.Base != versions[2].Number || edited.Head != versions[1].Number ||
		made.Base != 0 || made.By != "Anna Berg" {
		t.Fatalf("readers see the hints of both blocks: %+v", changes)
	}
	// A second change of the agent extends the hint: the base stays, the head moves.
	if _, err := db.EditBlock(agent, guide.ID, second.ID, anna.ID, "1. Open the backup of tonight."); err != nil {
		t.Fatal(err)
	}
	changes, _ = db.AIChanges(ctx, guide.ID, anna.ID)
	if again := changeOf(t, changes, second.ID); again == nil || again.Base != edited.Base || again.Head <= edited.Head {
		t.Errorf("a further change by an agent extends the hint: %+v", again)
	}
	// A person changes the block: its hint goes, the other stays.
	if _, err := db.EditBlock(asPerson(bob), guide.ID, second.ID, bob.ID, "1. Open the backup of tonight, as root."); err != nil {
		t.Fatal(err)
	}
	changes, _ = db.AIChanges(ctx, guide.ID, anna.ID)
	if len(changes) != 1 || changes[0].Block != added.ID {
		t.Errorf("a person's own change ends the hint of that block: %+v", changes)
	}
	// The agent side carries no hint per change.
	ref, _ := db.CreatePageOf(person, anna, "Network", team.ID, "", pagekind.Reference)
	b, _ := db.AddBlock(agent, ref.ID, anna.ID, "", "## Hosts")
	if _, err := db.EditBlock(agent, ref.ID, b.ID, anna.ID, "## Hosts\n\nbackup.example.com"); err != nil {
		t.Fatal(err)
	}
	if changes, _ := db.AIChanges(ctx, ref.ID, anna.ID); len(changes) != 0 {
		t.Errorf("a reference carries no hints: %+v", changes)
	}
	// What agents wrote on the agent side becomes no hint when a person makes the page human documentation later.
	if _, err := db.SetPageKind(person, ref.ID, anna.ID, pagekind.Explanation); err != nil {
		t.Fatal(err)
	}
	if changes, _ := db.AIChanges(ctx, ref.ID, anna.ID); len(changes) != 0 {
		t.Errorf("no hints from the time as a reference: %+v", changes)
	}
	// A page without a kind is human documentation.
	plain, _ := db.CreatePage(person, anna, "Notes", team.ID)
	_, _ = db.AddBlock(agent, plain.ID, anna.ID, "", "Some text.")
	if changes, _ := db.AIChanges(ctx, plain.ID, anna.ID); len(changes) != 1 {
		t.Errorf("a page without a kind carries hints: %+v", changes)
	}
	// A person moves it to the agent side: no hints show there, and they come back with the page.
	if _, err := db.SetPageKind(person, plain.ID, anna.ID, pagekind.AgentNotes); err != nil {
		t.Fatal(err)
	}
	if changes, _ := db.AIChanges(ctx, plain.ID, anna.ID); len(changes) != 0 {
		t.Errorf("agent notes show no hints: %+v", changes)
	}
	var head int64
	if err := db.pool.QueryRow(ctx, "SELECT head FROM ai_changes WHERE page = $1", plain.ID).Scan(&head); err != nil {
		t.Fatal(err)
	}
	if err := db.CheckChange(person, plain.ID, "b1", anna.ID, head); !errors.Is(err, ErrNotFound) {
		t.Errorf("nothing to check on the agent side: %v", err)
	}
	_, _ = db.SetPageKind(person, plain.ID, anna.ID, pagekind.Explanation)
	if changes, _ := db.AIChanges(ctx, plain.ID, anna.ID); len(changes) != 1 {
		t.Errorf("the hint is kept for when the page comes back: %+v", changes)
	}
	// Nothing changes the mark of a version afterwards.
	if _, err := db.pool.Exec(asAgent(anna), "UPDATE block_versions SET agent = false WHERE page = $1", plain.ID); err != nil {
		t.Fatal(err)
	}
	if v, _ := db.Versions(ctx, plain.ID, anna.ID, 1); len(v) != 1 || !v[0].Agent {
		t.Errorf("a version keeps who wrote it: %+v", v)
	}
}

// TestDeletedAndMovedByAnAgent: what an agent takes out of human documentation stays visible, with where it stood or
// went.
func TestDeletedAndMovedByAnAgent(t *testing.T) {
	db := open(t)
	ctx := inMain()
	agent, person := asAgent(anna), asPerson(anna)
	team, _ := db.createShared(ctx, anna, "Team")
	guide, _ := db.CreatePageOf(person, anna, "Restore", team.ID, "", pagekind.Explanation)
	first, _ := db.AddBlock(person, guide.ID, anna.ID, "", "Why backups matter.")
	second, _ := db.AddBlock(person, guide.ID, anna.ID, first.ID, "They keep the work of years.")
	third, _ := db.AddBlock(person, guide.ID, anna.ID, second.ID, "And they are cheap.")
	other, _ := db.CreatePage(person, anna, "Elsewhere", team.ID)
	if err := db.DeleteBlock(agent, guide.ID, second.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MoveBlockTo(agent, guide.ID, third.ID, anna.ID, other.ID, ""); err != nil {
		t.Fatal(err)
	}
	changes, _ := db.AIChanges(ctx, guide.ID, anna.ID)
	deleted, moved := changeOf(t, changes, second.ID), changeOf(t, changes, third.ID)
	if deleted == nil || !deleted.Deleted || deleted.Near != [2]string{first.ID, third.ID} || deleted.Moved != "" || moved == nil || !moved.Deleted ||
		moved.Moved != other.ID+"#b1" {
		t.Fatalf("a deletion and a move by an agent: %+v", changes)
	}
}

// TestCheckingAChange: a person who may change the page checks the open change they saw, and the block keeps who and
// when; an agent never checks, nor a reader, nor a change that is not the open one (ADR-33, the attack round of T-20).
func TestCheckingAChange(t *testing.T) {
	db := open(t)
	ctx := inMain()
	agent, person := asAgent(anna), asPerson(anna)
	team, _ := db.createShared(ctx, anna, "Team")
	guide, _ := db.CreatePageOf(person, anna, "Restore", team.ID, "", pagekind.HowTo)
	first, _ := db.AddBlock(person, guide.ID, anna.ID, "", "This guide brings files back.")
	other, _ := db.AddBlock(agent, guide.ID, anna.ID, first.ID, "1. Ask first.")
	if _, err := db.EditBlock(agent, guide.ID, first.ID, anna.ID, "This guide brings lost files back."); err != nil {
		t.Fatal(err)
	}
	changes, _ := db.AIChanges(ctx, guide.ID, anna.ID)
	open, otherChange := changeOf(t, changes, first.ID), changeOf(t, changes, other.ID)

	diff, err := db.ChangeOf(person, guide.ID, first.ID, anna.ID)
	if err != nil || diff.Before != "This guide brings files back." || diff.Now != "This guide brings lost files back." || diff.Head != open.Head {
		t.Fatalf("the diff of the change: %+v %v", diff, err)
	}
	if err := db.CheckChange(agent, guide.ID, first.ID, anna.ID, open.Head); !errors.Is(err, ErrAgent) {
		t.Errorf("an agent checks: %v", err)
	}
	if err := db.CheckChange(asAgent(bob), guide.ID, first.ID, bob.ID, open.Head); !errors.Is(err, ErrAgent) {
		t.Errorf("another agent checks: %v", err)
	}
	if err := db.TakeBack(agent, guide.ID, first.ID, anna.ID, open.Head, nil); !errors.Is(err, ErrAgent) {
		t.Errorf("an agent takes back: %v", err)
	}
	if err := db.MarkPageChecked(agent, guide.ID, anna.ID, "AB", time.Now()); !errors.Is(err, ErrAgent) {
		t.Errorf("an agent marks a page checked: %v", err)
	}
	if err := db.CheckChange(person, guide.ID, first.ID, anna.ID, open.Base); !errors.Is(err, ErrStale) {
		t.Errorf("an older version: %v", err)
	}
	if err := db.CheckChange(person, guide.ID, first.ID, anna.ID, otherChange.Head); !errors.Is(err, ErrStale) {
		t.Errorf("the change of another block: %v", err)
	}
	if err := db.CheckChange(person, guide.ID, "b99", anna.ID, open.Head); !errors.Is(err, ErrStale) {
		t.Errorf("a block with nothing open: %v", err)
	}
	// A reader may not check, nor see the diff.
	shown, _ := db.CreateCollection(ctx, anna, "Shown", "")
	if err := db.SetGrant(ctx, shown.ID, Actor{ID: anna.ID}, ToTenant, "", RoleRead); err != nil {
		t.Fatal(err)
	}
	read, _ := db.CreatePageOf(person, anna, "Read only", shown.ID, "", pagekind.HowTo)
	readBlock, _ := db.AddBlock(agent, read.ID, anna.ID, "", "Text by an agent.")
	readChanges, _ := db.AIChanges(ctx, read.ID, bob.ID)
	if len(readChanges) != 1 {
		t.Fatalf("the reader sees the hint: %+v", readChanges)
	}
	if err := db.CheckChange(asPerson(bob), read.ID, readBlock.ID, bob.ID, readChanges[0].Head); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("a reader checks: %v", err)
	}
	if _, err := db.ChangeOf(asPerson(bob), read.ID, readBlock.ID, bob.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("a reader reads the diff: %v", err)
	}
	// The check of another page's block with its own version.
	if err := db.CheckChange(person, guide.ID, readBlock.ID, anna.ID, readChanges[0].Head); !errors.Is(err, ErrStale) {
		t.Errorf("a version of another page: %v", err)
	}
	// A newer change of the agent after the diff was opened: the check of the older head is refused.
	if _, err := db.EditBlock(agent, guide.ID, first.ID, anna.ID, "This guide brings lost files back, fast."); err != nil {
		t.Fatal(err)
	}
	if err := db.CheckChange(person, guide.ID, first.ID, anna.ID, open.Head); !errors.Is(err, ErrStale) {
		t.Errorf("a check of what was not seen: %v", err)
	}
	changes, _ = db.AIChanges(ctx, guide.ID, anna.ID)
	open = changeOf(t, changes, first.ID)
	if err := db.CheckChange(asPerson(bob), guide.ID, first.ID, bob.ID, open.Head); err != nil {
		t.Fatalf("bob, who may change the page, checks: %v", err)
	}
	changes, _ = db.AIChanges(ctx, guide.ID, anna.ID)
	if changeOf(t, changes, first.ID) != nil || changeOf(t, changes, other.ID) == nil {
		t.Errorf("only the checked hint goes: %+v", changes)
	}
	last, err := db.LastCheck(ctx, guide.ID, anna.ID)
	if err != nil || last == nil || last.By != "Bob" || time.Since(last.At) > time.Minute {
		t.Errorf("the page says who checked and when: %+v %v", last, err)
	}
	if err := db.CheckChange(asPerson(bob), guide.ID, first.ID, bob.ID, open.Head); !errors.Is(err, ErrStale) {
		t.Errorf("a check twice: %v", err)
	}
	// The import marks every block checked and ends every hint (T-28).
	day := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	if err := db.MarkPageChecked(person, guide.ID, anna.ID, "AB", day); err != nil {
		t.Fatal(err)
	}
	if changes, _ := db.AIChanges(ctx, guide.ID, anna.ID); len(changes) != 0 {
		t.Errorf("no hint after the mark of an import: %+v", changes)
	}
	if last, _ := db.LastCheck(ctx, guide.ID, anna.ID); last == nil || last.By != "AB" || !last.At.Equal(day) {
		t.Errorf("the mark of the import: %+v", last)
	}
	if err := db.MarkPageChecked(asPerson(bob), read.ID, bob.ID, "BB", day); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("a reader marks a page checked: %v", err)
	}
}

// TestTakingBackAChange: the text before comes back, a block an agent made goes, a block an agent deleted comes back
// where it stood with its ID, and a block moved away is moved back from where it went.
func TestTakingBackAChange(t *testing.T) {
	db := open(t)
	ctx := inMain()
	agent, person := asAgent(anna), asPerson(anna)
	team, _ := db.createShared(ctx, anna, "Team")
	guide, _ := db.CreatePageOf(person, anna, "Restore", team.ID, "", pagekind.HowTo)
	first, _ := db.AddBlock(person, guide.ID, anna.ID, "", "This guide brings files back.")
	second, _ := db.AddBlock(person, guide.ID, anna.ID, first.ID, "1. Open the backup.")
	third, _ := db.AddBlock(person, guide.ID, anna.ID, second.ID, "2. Copy the files.")
	other, _ := db.CreatePage(person, anna, "Elsewhere", team.ID)
	_, _ = db.EditBlock(agent, guide.ID, first.ID, anna.ID, "Changed by an agent.")
	made, _ := db.AddBlock(agent, guide.ID, anna.ID, first.ID, "Made by an agent.")
	_ = db.DeleteBlock(agent, guide.ID, second.ID, anna.ID)
	_, _ = db.MoveBlockTo(agent, guide.ID, third.ID, anna.ID, other.ID, "")
	changes, _ := db.AIChanges(ctx, guide.ID, anna.ID)
	for i, block := range []string{second.ID, first.ID, made.ID} {
		c := changeOf(t, changes, block)
		if err := db.TakeBack(person, guide.ID, block, anna.ID, c.Head, nil); err != nil {
			t.Fatalf("take back %s: %v", block, err)
		}
		if i == 0 {
			blocks, _ := db.Blocks(ctx, guide.ID, anna.ID)
			if want := []string{first.ID + ":Changed by an agent.", made.ID + ":Made by an agent.", second.ID + ":1. Open the backup."}; !slices.Equal(bodies(blocks), want) {
				t.Errorf("the deleted block is back after the block before it: %q", bodies(blocks))
			}
		}
	}
	if err := db.TakeBack(person, guide.ID, third.ID, anna.ID, changeOf(t, changes, third.ID).Head, nil); !errors.Is(err, ErrMovedAway) {
		t.Errorf("a block moved away: %v", err)
	}
	blocks, _ := db.Blocks(ctx, guide.ID, anna.ID)
	if want := []string{first.ID + ":This guide brings files back.", second.ID + ":1. Open the backup."}; !slices.Equal(bodies(blocks), want) {
		t.Errorf("after taking back: %q", bodies(blocks))
	}
	changes, _ = db.AIChanges(ctx, guide.ID, anna.ID)
	if len(changes) != 1 || changes[0].Block != third.ID {
		t.Errorf("only the moved block keeps its hint: %+v", changes)
	}
	if err := db.CheckChange(person, guide.ID, third.ID, anna.ID, changes[0].Head); err != nil {
		t.Errorf("a move away is checked: %v", err)
	}
	versions, _ := db.Versions(ctx, guide.ID, anna.ID, 3)
	if versions[0].Agent || versions[1].Agent || versions[2].Agent || versions[2].Origin != "taken back" {
		t.Errorf("taking back is the person's own change: %+v", versions)
	}
}

// TestAnAgentKeepsPagesOfPeopleForPeople: an agent may not take a page people wrote out of the human documentation, which
// would hide the hints of its changes; a page only agents wrote it may class, and a person always.
func TestAnAgentKeepsPagesOfPeopleForPeople(t *testing.T) {
	db := open(t)
	ctx := inMain()
	agent, person := asAgent(anna), asPerson(anna)
	team, _ := db.createShared(ctx, anna, "Team")
	guide, _ := db.CreatePageOf(person, anna, "Restore", team.ID, "", pagekind.HowTo)
	_, _ = db.AddBlock(person, guide.ID, anna.ID, "", "Written by a person.")
	if _, err := db.SetPageKind(agent, guide.ID, anna.ID, pagekind.Reference); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("an agent takes a page of people to its side: %v", err)
	}
	if _, err := db.SetPageKind(agent, guide.ID, anna.ID, pagekind.Tutorial); err != nil {
		t.Errorf("an agent changes the kind within the human documentation: %v", err)
	}
	notes, _ := db.CreatePage(agent, anna, "Agent notes", team.ID)
	_, _ = db.AddBlock(agent, notes.ID, anna.ID, "", "Written by an agent.")
	if _, err := db.SetPageKind(agent, notes.ID, anna.ID, pagekind.AgentNotes); err != nil {
		t.Errorf("an agent classes a page only agents wrote: %v", err)
	}
	if _, err := db.SetPageKind(person, guide.ID, anna.ID, pagekind.Reference); err != nil {
		t.Errorf("a person classes any page: %v", err)
	}
}

// TestPagesToCheck: the filter "AI changes to check" names the pages of human documentation with open hints that the
// viewer sees.
func TestPagesToCheck(t *testing.T) {
	db := open(t)
	ctx := inMain()
	agent, person := asAgent(anna), asPerson(anna)
	team, _ := db.createShared(ctx, anna, "Team")
	guide, _ := db.CreatePageOf(person, anna, "Restore", team.ID, "", pagekind.HowTo)
	_, _ = db.AddBlock(agent, guide.ID, anna.ID, "", "Text by an agent.")
	checked, _ := db.CreatePageOf(person, anna, "Done", team.ID, "", pagekind.HowTo)
	_, _ = db.AddBlock(person, checked.ID, anna.ID, "", "Text by a person.")
	mine, _ := db.CreateCollection(ctx, anna, "Mine", "")
	hidden, _ := db.CreatePage(person, anna, "Hidden", mine.ID)
	_, _ = db.AddBlock(agent, hidden.ID, anna.ID, "", "Text by an agent.")
	if got, _ := db.PagesToCheck(ctx, bob.ID); !slices.Equal(got, []string{guide.ID}) {
		t.Errorf("bob: %v", got)
	}
	if got, _ := db.PagesToCheck(ctx, anna.ID); !slices.Equal(got, []string{hidden.ID, guide.ID}) {
		t.Errorf("anna, newest first: %v", got)
	}
	if _, err := db.SetPageKind(person, guide.ID, anna.ID, pagekind.Reference); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.PagesToCheck(ctx, bob.ID); len(got) != 0 {
		t.Errorf("the agent side has nothing to check: %v", got)
	}
}
