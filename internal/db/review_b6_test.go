// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/pagekind"
)

// The review of T-34 and B-5 (B-6). Each test is the attack round on the door it closes.

// Finding 1: a copy keeps of what lies below its top none of its entries, so an agent's copy of a folder with a page
// that broke its inheritance for one person would make that page readable for everyone around the copy.
func TestAgentsCopyNoRestrictedChildIntoTheOpen(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.createShared(ctx, anna, "Team")
	other, _ := db.createShared(ctx, anna, "Other team")
	folder, _ := db.CreateFolder(ctx, anna, "Projects", team.ID)
	inner, _ := db.CreatePage(ctx, anna, "Salaries", folder.ID)
	if err := db.SetInherit(asPerson(anna), inner.ID, admin, false); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGrant(ctx, inner.ID, admin, ToTenant, "", RoleNone); err != nil {
		t.Fatal(err)
	}
	if may(t, db, ctx, inner.ID, bob.ID, ActRead) {
		t.Fatal("bob reads the page only anna may read")
	}
	if _, err := db.Copy(asAgent(anna), folder.ID, anna.ID, other.ID, ""); !errors.Is(err, ErrPublish) {
		t.Errorf("anna's agent copies the folder with the closed page into another team: %v", err)
	}
	// Without the closed page the copy goes, and the person may copy it all.
	open, _ := db.CreateFolder(ctx, anna, "Open", team.ID)
	if _, err := db.CreatePage(ctx, anna, "Minutes", open.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Copy(asAgent(anna), open.ID, anna.ID, other.ID, ""); err != nil {
		t.Errorf("an agent copies an open folder: %v", err)
	}
	if _, err := db.Copy(asPerson(anna), folder.ID, anna.ID, other.ID, ""); err != nil {
		t.Errorf("anna copies it herself: %v", err)
	}
}

// Finding 2: a link lends an action only where its maker holds it on the object asked for, not only where the link is.
func TestLinksLendOnlyWhatTheMakerHoldsThere(t *testing.T) {
	db, ctx := setup(t)
	space, _ := db.CreateCollection(ctx, anna, "Space", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", space.ID)
	if err := db.SetGrant(ctx, space.ID, admin, ToPerson, carl.ID, RoleManage); err != nil {
		t.Fatal(err)
	}
	link, _, err := db.CreateLink(ctx, space.ID, Actor{ID: carl.ID}, NewLink{Mode: LinkEdit})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UseLink(ctx, link.ID, dora.ID); err != nil {
		t.Fatal(err)
	}
	if !may(t, db, ctx, page.ID, dora.ID, ActDelete) {
		t.Fatal("dora does not delete through the link at all")
	}
	if err := db.SetEntries(ctx, page.ID, admin, ToPerson, carl.ID, entries(ActDelete, Deny), true); err != nil {
		t.Fatal(err)
	}
	if may(t, db, ctx, page.ID, dora.ID, ActDelete) {
		t.Error("dora deletes the page through carl's link, where carl may not delete")
	}
	if err := db.Delete(ctx, page.ID, dora.ID); err == nil {
		t.Error("dora deleted the page through the link")
	}
	if !may(t, db, ctx, page.ID, dora.ID, ActWrite) {
		t.Error("dora no longer writes through the link where carl still may")
	}
}

// Finding 3: a live link on the target serves it to anyone who has the link, so an agent moves nothing there that such
// a link does not serve already, even when every account reads both places.
func TestAgentsMoveNothingUnderAPublicLink(t *testing.T) {
	db, ctx := setup(t)
	library, _ := db.createSharedFiles(ctx, anna, "Library")
	public, _ := db.CreateFolder(ctx, anna, "Handouts", library.ID)
	price := upload(t, db, anna.ID, UploadTarget{Parent: library.ID}, "prices.pdf", 4, "c"+strings.Repeat("0", 63)).File
	notes := upload(t, db, anna.ID, UploadTarget{Parent: library.ID}, "notes.pdf", 4, "d"+strings.Repeat("0", 63)).File
	if _, err := db.Move(asAgent(anna), price.ID, anna.ID, public.ID, ""); err != nil {
		t.Fatalf("before the link, a move within the same readers: %v", err)
	}
	for _, mode := range []string{LinkRead, LinkComment} {
		link, _, err := db.CreateLink(ctx, public.ID, admin, NewLink{Mode: mode})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Move(asAgent(anna), notes.ID, anna.ID, public.ID, ""); !errors.Is(err, ErrPublish) {
			t.Errorf("an agent moves a file under a live %s link: %v", mode, err)
		}
		if _, err := db.Copy(asAgent(anna), notes.ID, anna.ID, public.ID, ""); !errors.Is(err, ErrPublish) {
			t.Errorf("an agent copies a file under a live %s link: %v", mode, err)
		}
		if _, err := db.RevokeLink(ctx, link.ID, admin); err != nil {
			t.Fatal(err)
		}
	}
	// A link to upload only shows nothing, so it publishes nothing.
	if _, _, err := db.CreateLink(ctx, public.ID, admin, NewLink{Mode: LinkUpload}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Move(asAgent(anna), notes.ID, anna.ID, public.ID, ""); err != nil {
		t.Errorf("under a link to upload only, the move goes: %v", err)
	}
	// A block goes no more to a page that a live link shows.
	team, _ := db.createShared(ctx, anna, "Team")
	shown, _ := db.CreatePage(ctx, anna, "Opening hours", team.ID)
	draft, _ := db.CreatePage(ctx, anna, "Draft", team.ID)
	block, _ := db.AddBlock(ctx, draft.ID, anna.ID, "", "Die Kasse schließt früher.")
	if _, _, err := db.CreateLink(ctx, shown.ID, admin, NewLink{Mode: LinkRead}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MoveBlockTo(asAgent(anna), draft.ID, block.ID, anna.ID, shown.ID, ""); !errors.Is(err, ErrPublish) {
		t.Errorf("an agent moves a block to a page a live link shows: %v", err)
	}
	// Out from under the link into a place with the same readers and no link: fewer readers, allowed.
	if _, err := db.Move(asAgent(anna), price.ID, anna.ID, library.ID, ""); err != nil {
		t.Errorf("an agent moves out from under a link: %v", err)
	}
}

// Finding 4: moving a block rewrites the references to it, but not in an archived space or on the rules page; the
// redirect leads those to the new place.
func TestBlockMovesLeaveArchiveAndRulesPageAlone(t *testing.T) {
	db, ctx := setup(t)
	team, _ := db.createShared(ctx, anna, "Team")
	src, _ := db.CreatePage(ctx, anna, "Source", team.ID)
	dst, _ := db.CreatePage(ctx, anna, "Target", team.ID)
	block, _ := db.AddBlock(ctx, src.ID, anna.ID, "", "Die Kasse schließt um 20 Uhr.")
	embed := `::embed{ref="` + src.ID + `#` + block.ID + `"}`
	live, _ := db.CreatePage(ctx, anna, "Uses it", team.ID)
	liveBlock, _ := db.AddBlock(ctx, live.ID, anna.ID, "", embed)
	rules, _ := db.CreatePageOf(ctx, anna, "Rules", team.ID, "", pagekind.AgentNotes)
	rulesBlock, _ := db.AddBlock(ctx, rules.ID, anna.ID, "", embed)
	if err := db.SetRulesPage(asPerson(anna), admin, rules.ID); err != nil {
		t.Fatal(err)
	}
	old, _ := db.CreateCollection(ctx, anna, "Old", "")
	archived, _ := db.CreatePage(ctx, anna, "Kept", old.ID)
	archivedBlock, _ := db.AddBlock(ctx, archived.ID, anna.ID, "", embed)
	if err := db.SetArchived(asPerson(anna), old.ID, Actor{ID: anna.ID}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MoveBlockTo(asPerson(anna), src.ID, block.ID, anna.ID, dst.ID, ""); err != nil {
		t.Fatal(err)
	}
	body := func(page, id string) string {
		var b string
		if err := db.pool.QueryRow(ctx, "SELECT body FROM blocks WHERE page = $1 AND id = $2", page, id).Scan(&b); err != nil {
			t.Fatal(err)
		}
		return b
	}
	if got := body(live.ID, liveBlock.ID); got == embed || !strings.Contains(got, dst.ID) {
		t.Errorf("the reference of a live page is not rewritten: %q", got)
	}
	if got := body(rules.ID, rulesBlock.ID); got != embed {
		t.Errorf("the rules page was rewritten: %q", got)
	}
	if got := body(archived.ID, archivedBlock.ID); got != embed {
		t.Errorf("a page in an archived space was rewritten: %q", got)
	}
}

// Finding 5: the entries of a holder are read in the transaction that replaces them, under a lock of the object, so a
// deny set at the same moment is seen: whoever may not open it up does not take it back.
func TestEntriesAreReadUnderTheLock(t *testing.T) {
	db, ctx := setup(t)
	space, _ := db.CreateCollection(ctx, anna, "Space", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", space.ID)
	if err := db.SetGrant(ctx, space.ID, admin, ToPerson, carl.ID, RoleMembers); err != nil {
		t.Fatal(err)
	}
	if err := db.SetEntries(ctx, page.ID, admin, ToPerson, dora.ID, entries(ActRead, Allow), true); err != nil {
		t.Fatal(err)
	}
	// Another change of the page's entries is under way: it holds the lock and denies dora delete, not committed yet.
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 FOR UPDATE", page.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO grants (object, kind, who, action, value, subgroups, granted_by)
		VALUES ($1, 'person', $2, 'delete', 'deny', true, $3)`, page.ID, dora.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		// carl (Members: no delete) sets dora's entries to read only, which takes the deny of delete back.
		done <- db.SetEntries(ctx, page.ID, Actor{ID: carl.ID}, ToPerson, dora.ID, entries(ActRead, Allow, ActComment, Allow), true)
	}()
	select {
	case err := <-done:
		t.Fatalf("carl's change did not wait for the other one: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, ErrWiden) {
		t.Errorf("carl takes back a deny of delete he does not hold: %v", err)
	}
}

// Finding 6: an entry holds below the object where the inheritance is not broken, so whoever gives an action must hold
// it there too.
func TestOpeningUpCountsWhatInheritsBelow(t *testing.T) {
	db, ctx := setup(t)
	space, _ := db.CreateCollection(ctx, anna, "Space", "")
	folder, _ := db.CreateFolder(ctx, anna, "Folder", space.ID)
	child, _ := db.CreatePage(ctx, anna, "Child", folder.ID)
	if err := db.SetGrant(ctx, space.ID, admin, ToPerson, carl.ID, RoleManage); err != nil {
		t.Fatal(err)
	}
	if err := db.SetEntries(ctx, child.ID, admin, ToPerson, carl.ID, entries(ActDelete, Deny), true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetEntries(ctx, folder.ID, Actor{ID: carl.ID}, ToPerson, dora.ID, entries(ActRead, Allow, ActDelete, Allow), true); !errors.Is(err, ErrWiden) {
		t.Errorf("carl gives dora delete over a child he may not delete: %v", err)
	}
	if may(t, db, ctx, child.ID, dora.ID, ActDelete) {
		t.Error("dora deletes the child")
	}
	if err := db.SetEntries(ctx, folder.ID, Actor{ID: carl.ID}, ToPerson, dora.ID, entries(ActRead, Allow), true); err != nil {
		t.Errorf("carl gives what he holds below too: %v", err)
	}
	// A child that broke its inheritance takes nothing from the folder, so it does not count.
	if err := db.SetInherit(asPerson(anna), child.ID, admin, false); err != nil {
		t.Fatal(err)
	}
	if err := db.SetEntries(ctx, folder.ID, Actor{ID: carl.ID}, ToPerson, dora.ID, entries(ActRead, Allow, ActDelete, Allow), true); err != nil {
		t.Errorf("a child that breaks the inheritance still counts: %v", err)
	}
}

// Finding 7: a write that checked its rights before a space was archived does not land in it: the write holds the space,
// and the archiving waits for it or the write finds it archived.
func TestWritesDoNotLandInAnArchive(t *testing.T) {
	db, ctx := setup(t)
	space, _ := db.CreateCollection(ctx, anna, "Space", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", space.ID)
	files, _ := db.createSharedFiles(ctx, anna, "Library")
	file := upload(t, db, anna.ID, UploadTarget{Parent: files.ID}, "plan.pdf", 4, "a"+strings.Repeat("0", 63))
	archive := func(id string) func() error {
		return func() error { return db.SetArchived(asPerson(anna), id, Actor{ID: anna.ID}, true) }
	}
	restore := func(id string) {
		if err := db.SetArchived(asPerson(anna), id, Actor{ID: anna.ID}, false); err != nil {
			t.Fatal(err)
		}
	}
	// The space is archived while the upload is finished, after its rights were checked.
	finish := func(target UploadTarget, name string, n int) error {
		u, err := db.StartUpload(ctx, anna.ID, target, name, 4)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Received(ctx, u.ID, anna.ID, 0, 4); err != nil {
			t.Fatal(err)
		}
		root := files.ID
		if target.Page != "" {
			root = space.ID
		}
		_, err = db.FinishUpload(ctx, u.ID, anna.ID, strconv.Itoa(n)+strings.Repeat("b", 63), 4, "application/pdf", archive(root))
		return err
	}
	for name, try := range map[string]func() error{
		"a new file":    func() error { return finish(UploadTarget{Parent: files.ID}, "new.pdf", 1) },
		"a new version": func() error { return finish(UploadTarget{File: file.File.ID}, "plan.pdf", 2) },
		"an attachment": func() error { return finish(UploadTarget{Page: page.ID}, "scan.pdf", 3) },
	} {
		if err := try(); !errors.Is(err, ErrArchived) {
			t.Errorf("%s lands in a space archived meanwhile: %v", name, err)
		}
		restore(files.ID)
		restore(space.ID)
	}
	// A block and a new folder: the archiving is under way when they are written, and commits after their check.
	for name, write := range map[string]func() error{
		"a block": func() error {
			_, err := db.AddBlock(ctx, page.ID, anna.ID, "", "Written while it was archived.")
			return err
		},
		"a folder": func() error {
			_, err := db.CreateFolder(ctx, anna, "Made while it was archived", space.ID)
			return err
		},
	} {
		tx, err := db.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		// As setArchived does: the row for update first (B-7), then the mark.
		if _, err := tx.Exec(ctx, "SELECT 1 FROM objects WHERE id = $1 FOR UPDATE", space.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, "UPDATE objects SET archived = now(), archived_by = $2 WHERE id = $1", space.ID, anna.ID); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- write() }()
		select {
		case err := <-done:
			_ = tx.Rollback(ctx)
			t.Fatalf("%s did not wait for the archiving: %v", name, err)
		case <-time.After(300 * time.Millisecond):
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if err := <-done; !errors.Is(err, ErrArchived) {
			t.Errorf("%s lands in a space archived meanwhile: %v", name, err)
		}
		restore(space.ID)
	}
}

// Finding 8: a link on what an agent moves that is dead where it lies, because its maker may not share it there, would
// come alive where its maker may share: such a move publishes as well.
func TestAgentsWakeNoDeadLink(t *testing.T) {
	db, ctx := setup(t)
	here, _ := db.createShared(ctx, anna, "Here")
	there, _ := db.createShared(ctx, anna, "There")
	page, _ := db.CreatePage(ctx, anna, "Plan", here.ID)
	if err := db.SetGrant(ctx, page.ID, admin, ToPerson, carl.ID, RoleManage); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.CreateLink(ctx, page.ID, Actor{ID: carl.ID}, NewLink{Mode: LinkRead}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGrant(ctx, page.ID, admin, ToPerson, carl.ID, RoleNone); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGrant(ctx, there.ID, admin, ToPerson, carl.ID, RoleManage); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Move(asAgent(anna), page.ID, anna.ID, there.ID, ""); !errors.Is(err, ErrPublish) {
		t.Errorf("an agent moves a page whose dead link comes alive there: %v", err)
	}
	if _, err := db.Move(asPerson(anna), page.ID, anna.ID, there.ID, ""); err != nil {
		t.Errorf("anna moves it herself: %v", err)
	}
}

// Finding 10: emptying the trash checks and locks everything first, asks for one approval bound to a cutoff, and takes
// only what lay in the trash at that moment.
func TestEmptyingTheTrashIsBoundToItsCutoff(t *testing.T) {
	db, ctx := setup(t)
	space, _ := db.CreateCollection(ctx, anna, "Space", "")
	early, _ := db.CreatePage(ctx, anna, "Early", space.ID)
	late, _ := db.CreatePage(ctx, anna, "Late", space.ID)
	if err := db.Delete(ctx, early.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.pool.Exec(ctx, "UPDATE objects SET deleted_at = now() - interval '1 hour' WHERE id = $1", early.ID); err != nil {
		t.Fatal(err)
	}
	cutoff := time.Now().Add(-time.Minute).Truncate(time.Second)
	if err := db.Delete(ctx, late.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if ids, err := db.PurgeableTrash(asPerson(anna), Actor{ID: anna.ID}, TrashName, cutoff); err != nil || len(ids) != 1 || ids[0] != early.ID {
		t.Errorf("what emptying up to the cutoff takes: %v %v", ids, err)
	}
	recorder := &recordingApprover{}
	db.SetApprover(recorder)
	defer db.SetApprover(nil)
	gone, err := db.EmptyTrashBefore(asPerson(anna), Actor{ID: anna.ID}, TrashName, cutoff)
	if err != nil || gone != 1 {
		t.Fatalf("emptying up to the cutoff: %d %v", gone, err)
	}
	if len(recorder.asked) != 1 || recorder.asked[0] != ApproveEmptyTrash+" "+strconv.FormatInt(cutoff.Unix(), 10) {
		t.Errorf("the approval is not bound to the cutoff: %v", recorder.asked)
	}
	var left []string
	rows, _ := db.pool.Query(ctx, "SELECT id FROM objects WHERE deleted_at IS NOT NULL ORDER BY id")
	left, _ = pgx.CollectRows(rows, pgx.RowTo[string])
	if len(left) != 1 || left[0] != late.ID {
		t.Errorf("what was deleted after the cutoff went too: %v", left)
	}
	// A refused approval takes nothing, and nothing to take asks for none.
	db.SetApprover(refuseAll{})
	if _, err := db.EmptyTrash(asPerson(anna), Actor{ID: anna.ID}, TrashName); !errors.Is(err, errNoApproval) {
		t.Errorf("emptying without the approval: %v", err)
	}
	var n int
	if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM objects WHERE deleted_at IS NOT NULL").Scan(&n); err != nil || n != 1 {
		t.Errorf("a refused approval took something: %d left (%v)", n, err)
	}
	db.SetApprover(recorder)
	recorder.asked = nil
	if n, err := db.EmptyTrashBefore(asPerson(anna), Actor{ID: anna.ID}, TrashName, cutoff); err != nil || n != 0 || len(recorder.asked) != 0 {
		t.Errorf("nothing up to the cutoff: %d %v, asked %v", n, err, recorder.asked)
	}
}

type recordingApprover struct{ asked []string }

func (r *recordingApprover) Approve(_ context.Context, action, subject string) error {
	r.asked = append(r.asked, action+" "+subject)
	return nil
}

// Finding 11: the rules page is no private page and none in an archived space; naming it confirms one taken over from
// the tag.
func TestTheRulesPageIsNeitherPrivateNorArchived(t *testing.T) {
	db, ctx := setup(t)
	notes, _ := db.Collections(ctx, anna.ID)
	private, _ := db.CreatePageOf(ctx, anna, "My rules", notes[0].ID, "", pagekind.AgentNotes)
	old, _ := db.CreateCollection(ctx, anna, "Old", "")
	archived, _ := db.CreatePageOf(ctx, anna, "Old rules", old.ID, "", pagekind.AgentNotes)
	if err := db.SetArchived(asPerson(anna), old.ID, Actor{ID: anna.ID}, true); err != nil {
		t.Fatal(err)
	}
	for name, page := range map[string]string{"a private page": private.ID, "an archived page": archived.ID} {
		if err := db.SetRulesPage(asPerson(anna), admin, page); !errors.Is(err, ErrRulesPage) {
			t.Errorf("%s as the rules page: %v", name, err)
		}
	}
	team, _ := db.createShared(ctx, anna, "Team")
	rules, _ := db.CreatePageOf(ctx, anna, "Rules", team.ID, "", pagekind.AgentNotes)
	if _, err := db.pool.Exec(ctx, "INSERT INTO agent_settings (rules_page, from_tag) VALUES ($1, true)", rules.ID); err != nil {
		t.Fatal(err)
	}
	if fromTag, _ := db.RulesPageFromTag(ctx); !fromTag {
		t.Fatal("a page taken over from the tag is not marked")
	}
	if err := db.SetRulesPage(asPerson(anna), admin, rules.ID); err != nil {
		t.Fatal(err)
	}
	if fromTag, _ := db.RulesPageFromTag(ctx); fromTag {
		t.Error("naming the page does not confirm it")
	}
}

// Finding 13: the trash of an archived space waits while the space is archived, and gets the time back after.
func TestTheTrashOfAnArchiveWaits(t *testing.T) {
	db, ctx := setup(t)
	space, _ := db.CreateCollection(ctx, anna, "Space", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", space.ID)
	if err := db.Delete(ctx, page.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.pool.Exec(ctx, "UPDATE objects SET deleted_at = now() - interval '20 days' WHERE id = $1", page.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.SetArchived(asPerson(anna), space.ID, Actor{ID: anna.ID}, true); err != nil {
		t.Fatal(err)
	}
	// Archived for 15 days: past the 30 days, but the trash waited.
	if _, err := db.pool.Exec(ctx, "UPDATE objects SET archived = now() - interval '15 days' WHERE id = $1", space.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := db.ExpireTrash(ctx, time.Now().Add(-TrashDays*24*time.Hour+time.Hour*24*10)); err != nil || n != 0 {
		t.Errorf("the trash of an archived space expired: %d %v", n, err)
	}
	if err := db.SetArchived(asPerson(anna), space.ID, Actor{ID: anna.ID}, false); err != nil {
		t.Fatal(err)
	}
	var age time.Duration
	var deleted time.Time
	if err := db.pool.QueryRow(ctx, "SELECT deleted_at FROM objects WHERE id = $1", page.ID).Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	if age = time.Since(deleted); age < 4*24*time.Hour || age > 6*24*time.Hour {
		t.Errorf("after the restore the page is %v in the trash, want about 5 days", age)
	}
	if n, _ := db.ExpireTrash(ctx, time.Now().Add(-TrashDays*24*time.Hour)); n != 0 {
		t.Errorf("the page expired at once after the restore: %d", n)
	}
}

// Finding 9 stays proven in the web (TestServiceCountsWithAnAllowAndADenyOnOneLevel). The context of agents names the
// rules page as written (finding 12), and an empty page offers its kind (finding 14): both in the web.
var _ = account.AuthSession
