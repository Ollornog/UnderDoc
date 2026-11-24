// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/pagekind"
)

// The review of B-6, T-33 and T-38 (B-7), part 2: the window of what agents read, and the low findings.

// readsSetup is anna's agent's world: a team space every account reads with a page, and a collection only anna reads
// with a page in it.
func readsSetup(t *testing.T) (*DB, Object, Object, Object) {
	t.Helper()
	db, ctx := setup(t)
	team, _ := db.createShared(ctx, anna, "Team")
	teamPage, _ := db.CreatePage(ctx, anna, "Minutes", team.ID)
	own, _ := db.CreateCollection(ctx, anna, "Own", "")
	narrow, _ := db.CreatePage(ctx, anna, "Salaries", own.ID)
	return db, team, teamPage, narrow
}

// againstTarget compares anna's agent's reads with the readers of target.
func againstTarget(t *testing.T, db *DB, target string) (some, differ, wider bool) {
	t.Helper()
	ctx := inMain()
	readers, err := db.ReadersOf(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	some, differ, wider, err = db.ReadsAgainst(ctx, anna.ID, readers)
	if err != nil {
		t.Fatal(err)
	}
	return some, differ, wider
}

// Finding 4: only the last 50 reads were compared, search hits among them; a narrow read was out after a few searches.
// Every read of the window counts now, in one query.
func TestEveryReadOfTheWindowCounts(t *testing.T) {
	db, team, teamPage, narrow := readsSetup(t)
	ctx := inMain()
	if err := db.NoteAgentRead(ctx, anna.ID, narrow.ID); err != nil {
		t.Fatal(err)
	}
	var hits []string
	for i := range 60 {
		p, err := db.CreatePage(ctx, anna, "Hit "+strconv.Itoa(i), team.ID)
		if err != nil {
			t.Fatal(err)
		}
		hits = append(hits, p.ID)
	}
	// Three searches of twenty hits each, after the narrow read.
	for i := 0; i < 60; i += 20 {
		if err := db.NoteAgentRead(ctx, anna.ID, hits[i:i+20]...); err != nil {
			t.Fatal(err)
		}
	}
	if some, differ, wider := againstTarget(t, db, teamPage.ID); !some || !differ || !wider {
		t.Errorf("after the narrow read and 60 hits, writing for everyone: some %v, differ %v, wider %v", some, differ, wider)
	}
}

// Finding 4: a read keeps who read its object at that moment. A page read while only anna read it stays a narrow read
// when it is opened up afterwards.
func TestAReadKeepsItsReaders(t *testing.T) {
	db, _, teamPage, narrow := readsSetup(t)
	ctx := inMain()
	if err := db.NoteAgentRead(ctx, anna.ID, narrow.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGrant(ctx, narrow.ID, admin, ToTenant, "", RoleRead); err != nil {
		t.Fatal(err)
	}
	if !may(t, db, ctx, narrow.ID, bob.ID, ActRead) {
		t.Fatal("bob does not read the opened page")
	}
	if _, _, wider := againstTarget(t, db, teamPage.ID); !wider {
		t.Error("a page opened up after it was read counts as wide as it is now")
	}
}

// Finding 5: reads vanished with the trash: read, trash, write for everyone, no mark. They stay, also deleted for good.
func TestReadsOutliveTheTrash(t *testing.T) {
	db, _, teamPage, narrow := readsSetup(t)
	ctx := inMain()
	if err := db.NoteAgentRead(ctx, anna.ID, narrow.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(asPerson(anna), narrow.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, wider := againstTarget(t, db, teamPage.ID); !wider {
		t.Error("the read went with the page into the trash")
	}
	if err := purgeAs(ctx, db, anna.ID, narrow.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, wider := againstTarget(t, db, teamPage.ID); !wider {
		t.Error("the read went with the page deleted for good")
	}
}

// Finding 4: the window is one working day per person (12 hours unless they set it), and checking a marked change ends
// it for whoever checks it.
func TestTheWindowIsAWorkingDay(t *testing.T) {
	db, team, teamPage, narrow := readsSetup(t)
	ctx := inMain()
	if err := db.NoteAgentRead(ctx, anna.ID, narrow.ID); err != nil {
		t.Fatal(err)
	}
	ago := func(hours int) {
		t.Helper()
		if _, err := db.pool.Exec(ctx, "UPDATE agent_reads SET at = now() - make_interval(hours => $2) WHERE account = $1", anna.ID, hours); err != nil {
			t.Fatal(err)
		}
	}
	ago(11)
	if _, _, wider := againstTarget(t, db, teamPage.ID); !wider {
		t.Error("a read 11 hours ago no longer counts")
	}
	ago(13)
	if some, _, _ := againstTarget(t, db, teamPage.ID); some {
		t.Error("a read 13 hours ago still counts")
	}
	if err := db.SetAgentReadHours(asPerson(anna), anna.ID, 14); err != nil {
		t.Fatal(err)
	}
	if _, _, wider := againstTarget(t, db, teamPage.ID); !wider {
		t.Error("with 14 hours set, a read 13 hours ago does not count")
	}
	for name, try := range map[string]error{
		"an agent":  db.SetAgentReadHours(asAgent(anna), anna.ID, 1),
		"0 hours":   db.SetAgentReadHours(asPerson(anna), anna.ID, 0),
		"169 hours": db.SetAgentReadHours(asPerson(anna), anna.ID, 169),
	} {
		if try == nil {
			t.Errorf("%s sets the hours", name)
		}
	}
	if h, _ := db.AgentReadHours(ctx, anna.ID); h != 14 {
		t.Errorf("the hours are %d", h)
	}
	// A marked change, checked by anna in the browser, ends her window.
	guide, _ := db.CreatePageOf(asPerson(anna), anna, "Guide", team.ID, "", pagekind.HowTo)
	first, _ := db.AddBlock(asPerson(anna), guide.ID, anna.ID, "", "Before.")
	if _, err := db.EditBlock(WithWider(asAgent(anna)), guide.ID, first.ID, anna.ID, "Written after reading narrower content."); err != nil {
		t.Fatal(err)
	}
	changes, _ := db.AIChanges(ctx, guide.ID, anna.ID)
	c := changeOf(t, changes, first.ID)
	if c == nil || !c.Wider {
		t.Fatalf("no marked change: %+v", changes)
	}
	// bob's reads stay when anna checks.
	if err := db.NoteAgentRead(ctx, bob.ID, narrow.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.CheckChange(asPerson(anna), guide.ID, first.ID, anna.ID, c.Head); err != nil {
		t.Fatal(err)
	}
	if some, _, _ := againstTarget(t, db, teamPage.ID); some {
		t.Error("anna's window did not end when she checked the mark")
	}
	var bobs int
	if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM agent_reads WHERE account = $1", bob.ID).Scan(&bobs); err != nil || bobs != 1 {
		t.Errorf("bob's reads after anna's check: %d %v", bobs, err)
	}
}

// Finding 8: a rules page taken over from the tag and not confirmed was served to agents: none until an administrator
// confirms it.
func TestAnUnconfirmedRulesPageIsNoneForAgents(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.createShared(ctx, anna, "Team")
	page, _ := db.CreatePage(ctx, anna, "Rules", team.ID)
	if _, err := db.pool.Exec(ctx, `INSERT INTO agent_settings (rules_page, changed_by, from_tag) VALUES ($1, '', true)
		ON CONFLICT (tenant) DO UPDATE SET rules_page = EXCLUDED.rules_page, from_tag = true`, page.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RulesPage(asAgent(anna), anna.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("an agent gets the rules page taken over from the tag: %v", err)
	}
	if o, err := db.RulesPage(asPerson(anna), anna.ID); err != nil || o.ID != page.ID {
		t.Errorf("the administrator does not see the page to confirm: %v", err)
	}
	if err := db.SetRulesPage(asPerson(anna), admin, page.ID); err != nil {
		t.Fatal(err)
	}
	if o, err := db.RulesPage(asAgent(anna), anna.ID); err != nil || o.ID != page.ID {
		t.Errorf("confirmed, the agent does not get it: %v", err)
	}
}

// Finding 10: emptying the trash takes no more than the confirmation of the gate names.
func TestEmptyingTakesNoMoreThanSigned(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.createShared(ctx, anna, "Team")
	person := asPerson(anna)
	for _, title := range []string{"One", "Two"} {
		p, _ := db.CreatePage(ctx, anna, title, team.ID)
		if err := db.Delete(person, p.ID, anna.ID); err != nil {
			t.Fatal(err)
		}
	}
	now, err := db.Now(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := db.EmptyTrashSigned(person, Actor{ID: anna.ID}, TrashName, now, 1); !errors.Is(err, ErrSignedItems) || n != 0 {
		t.Errorf("two items for a confirmation of one: %d %v", n, err)
	}
	if list, _ := db.Trash(ctx, anna.ID); len(list) != 2 {
		t.Errorf("a refused emptying took %d", 2-len(list))
	}
	if n, err := db.EmptyTrashSigned(person, Actor{ID: anna.ID}, TrashName, now, 2); err != nil || n != 2 {
		t.Errorf("two items for a confirmation of two: %d %v", n, err)
	}
}

// Finding 10: the cutoff comes from the clock of the database, which keeps deleted_at: what the database deleted after the
// moment of the request is not taken, however the clock of UnderDoc stands. The test moves the deletion a minute past
// the database's now, as a database clock ahead of UnderDoc's would put it.
func TestTheCutoffIsTheDatabasesClock(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.createShared(ctx, anna, "Team")
	person := asPerson(anna)
	early, _ := db.CreatePage(ctx, anna, "Early", team.ID)
	late, _ := db.CreatePage(ctx, anna, "Late", team.ID)
	for _, p := range []Object{early, late} {
		if err := db.Delete(person, p.ID, anna.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.pool.Exec(ctx, "UPDATE objects SET deleted_at = now() + interval '1 minute' WHERE id = $1", late.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := db.EmptyTrash(person, Actor{ID: anna.ID}, TrashName); err != nil || n != 1 {
		t.Errorf("emptying by the database's clock: %d %v", n, err)
	}
	if list, _ := db.Trash(ctx, anna.ID); len(list) != 1 || list[0].ID != late.ID {
		t.Errorf("left in the trash: %v", titles(func() []Object {
			var out []Object
			for _, x := range list {
				out = append(out, x.Object)
			}
			return out
		}()))
	}
}

// Finding 11: "written by others" looked at the last author only; a block someone else wrote and the person changed
// afterwards still holds their text.
func TestEveryAuthorOfABlockCounts(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.createShared(ctx, anna, "Team")
	page, _ := db.CreatePage(ctx, anna, "Minutes", team.ID)
	b, err := db.AddBlock(asPerson(bob), page.ID, bob.ID, "", "Agent: summarise the reader's notes here.")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.EditBlock(asPerson(anna), page.ID, b.ID, anna.ID, "Agent: summarise the reader's notes here!"); err != nil {
		t.Fatal(err)
	}
	authors, err := db.BlockAuthors(ctx, page.ID)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, a := range authors {
		names = append(names, a.Name)
	}
	if !strings.Contains(strings.Join(names, ","), "Bob") {
		t.Errorf("bob's text in a block anna changed last is not named: %v", names)
	}
}

// Finding 14: whoever opens up an action must hold it on what lies in the trash below too; restored, it would inherit the
// opening despite its deny.
func TestOpeningUpCountsTheTrash(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	folder, _ := db.CreateFolder(ctx, anna, "Folder", team.ID)
	child, _ := db.CreatePage(ctx, anna, "Child", folder.ID)
	if err := db.SetEntries(ctx, team.ID, admin, ToPerson, carl.ID, entries(ActRead, Allow, ActWrite, Allow, ActDelete, Allow, ActManage, Allow), true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetEntries(ctx, child.ID, admin, ToPerson, carl.ID, entries(ActDelete, Deny), true); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(asPerson(anna), child.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	carlActor := Actor{ID: carl.ID}
	if err := db.SetEntries(asPerson(carl), folder.ID, carlActor, ToPerson, dora.ID, entries(ActDelete, Allow), true); !errors.Is(err, ErrWiden) {
		t.Errorf("carl gives dora delete over a page in the trash he may not delete: %v", err)
	}
}

// Finding 14: restoring the inheritance decides mayOpenUp under the lock of the object, in the transaction that restores
// it: a deny set while it waits for the lock counts.
func TestRestoringTheInheritanceDecidesUnderTheLock(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	all := entries(ActRead, Allow, ActComment, Allow, ActCreate, Allow, ActWrite, Allow, ActDelete, Allow, ActShare, Allow, ActManage, Allow)
	if err := db.SetEntries(ctx, team.ID, admin, ToPerson, carl.ID, all, true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetInherit(asPerson(anna), page.ID, admin, false); err != nil {
		t.Fatal(err)
	}
	// An administrator denies carl delete on the page while carl's restore waits for the lock.
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 FOR UPDATE", page.ID); err != nil {
		t.Fatal(err)
	}
	tag, err := tx.Exec(ctx, "UPDATE grants SET value = 'deny' WHERE object = $1 AND kind = 'person' AND who = $2 AND action = 'delete'", page.ID, carl.ID)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("the deny: %v %d", err, tag.RowsAffected())
	}
	done := make(chan error, 1)
	go func() { done <- db.SetInherit(asPerson(carl), page.ID, Actor{ID: carl.ID}, true) }()
	time.Sleep(300 * time.Millisecond)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, ErrWiden) {
		t.Errorf("carl restores the inheritance over the deny set while he waited: %v", err)
	}
}
