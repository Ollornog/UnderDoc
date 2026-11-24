// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/pagekind"
)

// The review of B-6, T-33 and T-38 (B-7). Each test is the attack round on the door it closes.

// Finding 1: a copy keeps of its original only the role it grants the whole tenant, not its denies. A page readable for
// the tenant with a deny for dora, copied by an agent into a collection only Sales reads, became readable for dora:
// the check compared the readers of the target with the original, and dora reads neither.
func TestAgentsCopyKeepsNoDeniedReaderOut(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "Salaries", team.ID)
	folder, _ := db.CreateFolder(ctx, anna, "HR", team.ID)
	inner, _ := db.CreatePage(ctx, anna, "Reviews", folder.ID)
	sales, err := db.CreateGroup(ctx, admin, "Sales", "", "")
	if err != nil {
		t.Fatal(err)
	}
	room, _ := db.CreateCollection(ctx, anna, "Sales room", "")
	for _, step := range []error{
		db.SetMember(ctx, admin, sales.ID, bob.ID, true),
		db.SetEntries(ctx, room.ID, admin, ToGroup, sales.ID, entries(ActRead, Allow, ActCreate, Allow), true),
		// The page and the folder grant the tenant read themselves, so their copies keep it; dora is denied.
		db.SetGrant(ctx, page.ID, admin, ToTenant, "", RoleRead),
		db.SetEntries(ctx, page.ID, admin, ToPerson, dora.ID, entries(ActRead, Deny), true),
		db.SetGrant(ctx, folder.ID, admin, ToTenant, "", RoleRead),
		db.SetGrant(ctx, inner.ID, admin, ToTenant, "", RoleRead),
		db.SetEntries(ctx, inner.ID, admin, ToPerson, dora.ID, entries(ActRead, Deny), true),
	} {
		if step != nil {
			t.Fatal(step)
		}
	}
	if may(t, db, ctx, page.ID, dora.ID, ActRead) || may(t, db, ctx, room.ID, dora.ID, ActRead) {
		t.Fatal("dora reads the page or the room")
	}
	if _, err := db.Copy(asAgent(anna), page.ID, anna.ID, room.ID, ""); !errors.Is(err, ErrPublish) {
		t.Errorf("anna's agent copies the page with the deny for dora into the room of Sales: %v", err)
	}
	if _, err := db.Copy(asAgent(anna), folder.ID, anna.ID, room.ID, ""); !errors.Is(err, ErrPublish) {
		t.Errorf("anna's agent copies the folder whose page denies dora into the room of Sales: %v", err)
	}
	// Nothing of a refused copy stays behind.
	if kids, _ := db.Children(ctx, room.ID, anna.ID); len(kids) != 0 {
		t.Errorf("a refused copy left %v in the room", titles(kids))
	}
	// Without the deny the copy is readable by no one who does not read the original, and it goes; a person copies all.
	open, _ := db.CreatePage(ctx, anna, "Minutes", team.ID)
	if err := db.SetGrant(ctx, open.ID, admin, ToTenant, "", RoleRead); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Copy(asAgent(anna), open.ID, anna.ID, room.ID, ""); err != nil {
		t.Errorf("an agent copies a page every account reads: %v", err)
	}
	if _, err := db.Copy(asPerson(anna), page.ID, anna.ID, room.ID, ""); err != nil {
		t.Errorf("anna copies the page herself: %v", err)
	}
}

// Finding 2 (the database's part): a group admin can put any account into their group, so that alone gives them nothing
// over the person. They manage the persons in none but their groups, and whom they invited.
func TestGroupAdminsManageOnlyTheirOwnPeople(t *testing.T) {
	db, ctx := setup(t)
	mine, _ := db.CreateGroup(asPerson(anna), admin, "Till", "", "")
	other, _ := db.CreateGroup(asPerson(anna), admin, "Office", "", "")
	for _, step := range []error{
		db.SetGroupAdmin(asPerson(anna), admin, mine.ID, carl.ID, true),
		db.SetMember(asPerson(anna), admin, other.ID, bob.ID, true),
	} {
		if step != nil {
			t.Fatal(step)
		}
	}
	// carl takes bob, who is in Office too, into his group by his key.
	carlAsActor := Actor{ID: carl.ID}
	if err := db.SetMember(asPerson(carl), carlAsActor, mine.ID, bob.ID, true); err != nil {
		t.Fatal(err)
	}
	if m, err := db.MayManagePerson(ctx, carl.ID, bob.ID); err != nil || m.Manages || m.Invited {
		t.Errorf("carl manages bob, whom he took into his group, though bob is in Office too: %+v %v", m, err)
	}
	// dora is in no group: taken into carl's, she is in none but his, and he manages her, without being her inviter.
	if err := db.SetMember(asPerson(carl), carlAsActor, mine.ID, dora.ID, true); err != nil {
		t.Fatal(err)
	}
	if m, _ := db.MayManagePerson(ctx, carl.ID, dora.ID); !m.Manages || m.Invited {
		t.Errorf("carl and dora, in none but his group: %+v", m)
	}
	// Invited by carl, bob is his to manage, also in Office.
	if err := db.MarkInvited(ctx, bob.ID, carl.ID); err != nil {
		t.Fatal(err)
	}
	if m, _ := db.MayManagePerson(ctx, carl.ID, bob.ID); !m.Manages || !m.Invited {
		t.Errorf("carl invited bob: %+v", m)
	}
	// The inviter is kept once.
	if err := db.MarkInvited(ctx, bob.ID, dora.ID); err != nil {
		t.Fatal(err)
	}
	if m, _ := db.MayManagePerson(ctx, carl.ID, bob.ID); !m.Invited {
		t.Error("a second invitation took the inviter over")
	}
	// The lock of a group admin takes the person out of his groups, not out of Office.
	n, err := db.RemoveFromMyGroups(asPerson(carl), carlAsActor, bob.ID)
	if err != nil || n != 1 {
		t.Fatalf("carl takes bob out of his groups: %d %v", n, err)
	}
	if groups, _ := db.DirectGroupsOf(ctx, bob.ID); len(groups) != 1 || groups[0] != other.ID {
		t.Errorf("bob is in %v after carl took him out of his groups", groups)
	}
	if _, err := db.RemoveFromMyGroups(asAgent(carl), carlAsActor, dora.ID); err == nil {
		t.Error("an agent takes a person out of groups")
	}
}

// Finding 7: a link of a moved object that was dead where it lay wakes up where its maker may share; every mode that
// shows the object counts, not only read.
func TestAgentsWakeNoDeadLinkOfAnyMode(t *testing.T) {
	db, ctx := setup(t)
	here, _ := db.createShared(ctx, anna, "Here")
	there, _ := db.createShared(ctx, anna, "There")
	for _, mode := range []string{LinkComment, LinkEdit} {
		page, _ := db.CreatePage(ctx, anna, "Plan "+mode, here.ID)
		if err := db.SetGrant(ctx, page.ID, admin, ToPerson, carl.ID, RoleManage); err != nil {
			t.Fatal(err)
		}
		if _, _, err := db.CreateLink(ctx, page.ID, Actor{ID: carl.ID}, NewLink{Mode: mode}); err != nil {
			t.Fatal(err)
		}
		if err := db.SetGrant(ctx, page.ID, admin, ToPerson, carl.ID, RoleNone); err != nil {
			t.Fatal(err)
		}
		if err := db.SetGrant(ctx, there.ID, admin, ToPerson, carl.ID, RoleManage); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Move(asAgent(anna), page.ID, anna.ID, there.ID, ""); !errors.Is(err, ErrPublish) {
			t.Errorf("an agent moves a page whose dead %s link wakes up there: %v", mode, err)
		}
		if err := db.SetGrant(ctx, there.ID, admin, ToPerson, carl.ID, RoleNone); err != nil {
			t.Fatal(err)
		}
	}
}

// archiving locks a space for update and marks it archived in a transaction left open, as setArchived does, so a write
// that checked its rights before can be started against it. The caller commits.
func archiving(t *testing.T, db *DB, spaceID string) pgx.Tx {
	t.Helper()
	ctx := inMain()
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 FOR UPDATE", spaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "UPDATE objects SET archived = now(), archived_by = $2 WHERE id = $1", spaceID, anna.ID); err != nil {
		t.Fatal(err)
	}
	return tx
}

// Finding 3: holdSpace was missing in the write of an agent (write, replace), in taking back, in a move (where it lies
// and where it goes), in deleting and in renaming. An archiving that commits after their check let the change land in
// the archive. Each one now waits for the archiving and finds the archive.
func TestEveryWriteHoldsItsSpace(t *testing.T) {
	db, ctx := setup(t)
	person, agent := asPerson(anna), asAgent(anna)
	space, _ := db.CreateCollection(ctx, anna, "Space", "")
	elsewhere, _ := db.CreateCollection(ctx, anna, "Elsewhere", "")
	restore := func(id string) {
		t.Helper()
		if err := db.SetArchived(person, id, Actor{ID: anna.ID}, false); err != nil {
			t.Fatal(err)
		}
	}
	type try struct {
		space string
		do    func() error
	}
	cases := map[string]func() try{
		"the write of an agent": func() try {
			page, _ := db.CreatePage(person, anna, "Written", space.ID)
			return try{space.ID, func() error {
				_, err := db.WriteBlocks(agent, page.ID, anna.ID, 0, "", []Block{{Body: "Written while it was archived."}}, false)
				return err
			}}
		},
		"taking back": func() try {
			page, _ := db.CreatePageOf(person, anna, "Taken back", space.ID, "", pagekind.HowTo)
			first, _ := db.AddBlock(person, page.ID, anna.ID, "", "Before.")
			if _, err := db.EditBlock(agent, page.ID, first.ID, anna.ID, "Changed by an agent."); err != nil {
				t.Fatal(err)
			}
			changes, _ := db.AIChanges(ctx, page.ID, anna.ID)
			c := changeOf(t, changes, first.ID)
			if c == nil {
				t.Fatal("no change of an agent to take back")
			}
			return try{space.ID, func() error { return db.TakeBack(person, page.ID, first.ID, anna.ID, c.Head, nil) }}
		},
		"a move out": func() try {
			page, _ := db.CreatePage(person, anna, "Moved out", space.ID)
			return try{space.ID, func() error {
				_, err := db.Move(person, page.ID, anna.ID, elsewhere.ID, "")
				return err
			}}
		},
		"a move in": func() try {
			page, _ := db.CreatePage(person, anna, "Moved in", elsewhere.ID)
			return try{space.ID, func() error {
				_, err := db.Move(person, page.ID, anna.ID, space.ID, "")
				return err
			}}
		},
		"a deletion": func() try {
			page, _ := db.CreatePage(person, anna, "Deleted", space.ID)
			return try{space.ID, func() error { return db.Delete(person, page.ID, anna.ID) }}
		},
		"a new name": func() try {
			page, _ := db.CreatePage(person, anna, "Renamed", space.ID)
			return try{space.ID, func() error {
				_, err := db.Rename(person, page.ID, anna.ID, "Renamed while it was archived")
				return err
			}}
		},
	}
	for name, setUp := range cases {
		c := setUp()
		tx := archiving(t, db, c.space)
		done := make(chan error, 1)
		go func() { done <- c.do() }()
		select {
		case err := <-done:
			_ = tx.Rollback(ctx)
			t.Errorf("%s did not wait for the archiving: %v", name, err)
			continue
		case <-time.After(300 * time.Millisecond):
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-done; !errors.Is(err, ErrArchived) {
			t.Errorf("%s lands in a space archived meanwhile: %v", name, err)
		}
		restore(c.space)
	}
}

// Finding 3, the order of the locks: a write of a page directly in a space locks the page and then holds the space; a
// move into the space at its first place locks the space and then shifts the page. With the space held FOR SHARE and
// placed FOR UPDATE, the two deadlocked; held FOR KEY SHARE and placed FOR NO KEY UPDATE, both go through. Two uploads
// and two new pages at the top of a space at the same time go through as well.
func TestHoldingASpaceDeadlocksNothing(t *testing.T) {
	db, ctx := setup(t)
	space, _ := db.CreateCollection(ctx, anna, "Space", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", space.ID)
	other, _ := db.CreateCollection(ctx, anna, "Other", "")
	moving, _ := db.CreatePage(ctx, anna, "Moving", other.ID)
	// The write, step by step: the page is locked.
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 FOR UPDATE", page.ID); err != nil {
		t.Fatal(err)
	}
	// The move into the first place of the space places there and waits to shift the page.
	done := make(chan error, 1)
	go func() {
		_, err := db.Move(asPerson(anna), moving.ID, anna.ID, space.ID, "")
		done <- err
	}()
	time.Sleep(300 * time.Millisecond)
	// Now the write holds the space.
	if err := holdSpace(ctx, tx, page.ID); err != nil {
		t.Errorf("the write does not hold the space while a move places into it: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Errorf("the move into the space: %v", err)
	}
	// Two new pages and two uploads at the top of the space at once.
	files, _ := db.createSharedFiles(ctx, anna, "Library")
	errs := make(chan error, 4)
	for i := range 2 {
		go func() {
			_, err := db.CreatePage(ctx, anna, "Parallel", space.ID)
			errs <- err
		}()
		go func() {
			u, err := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: files.ID}, "parallel.pdf", 4)
			if err == nil {
				err = db.Received(ctx, u.ID, anna.ID, 0, 4)
			}
			if err == nil {
				_, err = db.FinishUpload(ctx, u.ID, anna.ID, string(rune('a'+i))+strings.Repeat("0", 62)+"f", 4, "application/pdf", func() error { return nil })
			}
			errs <- err
		}()
	}
	for range 4 {
		if err := <-errs; err != nil {
			t.Errorf("parallel at the top of a space: %v", err)
		}
	}
}

// Finding 3: archiving locks the space for update, so it waits for a write that holds the space; an update alone would
// pass a hold FOR KEY SHARE.
func TestArchivingWaitsForAWrite(t *testing.T) {
	db, ctx := setup(t)
	space, _ := db.CreateCollection(ctx, anna, "Space", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", space.ID)
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := holdSpace(ctx, tx, page.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- db.SetArchived(asPerson(anna), space.ID, Actor{ID: anna.ID}, true) }()
	select {
	case err := <-done:
		t.Fatalf("archiving did not wait for the write that holds the space: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Errorf("archiving after the write: %v", err)
	}
}

// Finding 3, a move that waits for the lock of where it goes: meanwhile the target was moved below what moves. The move
// finds it in its transaction instead of making a loop.
func TestAMoveMakesNoLoopWhileItWaits(t *testing.T) {
	db, ctx := setup(t)
	space, _ := db.CreateCollection(ctx, anna, "Space", "")
	a, _ := db.CreateFolder(ctx, anna, "A", space.ID)
	b, _ := db.CreateFolder(ctx, anna, "B", space.ID)
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "UPDATE objects SET parent = $2 WHERE id = $1", b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := db.Move(asPerson(anna), a.ID, anna.ID, b.ID, "")
		done <- err
	}()
	time.Sleep(300 * time.Millisecond)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, ErrPlace) {
		t.Errorf("A moved into B, which was moved into A meanwhile: %v", err)
	}
}

// Finding 3, two uploads into the top of a library at once: one finishing counted its bytes (the counter row), then
// locked the folder FOR UPDATE; one starting held the folder for key share (its row refers to it), then counted its
// bytes. Each waited for the other. The finishing upload locks the folder FOR NO KEY UPDATE now.
func TestTwoUploadsAtTheTopDoNotDeadlock(t *testing.T) {
	db, ctx := setup(t)
	files, _ := db.createSharedFiles(ctx, anna, "Library")
	u, err := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: files.ID}, "first.pdf", 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Received(ctx, u.ID, anna.ID, 0, 4); err != nil {
		t.Fatal(err)
	}
	// The upload starting: its row refers to the folder, and it has not counted its bytes yet.
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 FOR KEY SHARE", files.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := db.FinishUpload(ctx, u.ID, anna.ID, "e"+strings.Repeat("0", 63), 4, "application/pdf", func() error { return nil })
		done <- err
	}()
	time.Sleep(300 * time.Millisecond)
	// Now the starting upload counts its bytes.
	if _, err := tx.Exec(ctx, "SELECT storage_add(current_setting('underdoc.tenant', true), 4)"); err != nil {
		t.Errorf("the starting upload counts its bytes: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Errorf("the finishing upload: %v", err)
	}
}
