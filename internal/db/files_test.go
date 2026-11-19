// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
)

// sha gives a made-up hash for test content: the database only checks its form.
func sha(n int) string { return fmt.Sprintf("%064x", n) }

func rootOfRole(t *testing.T, db *DB, viewer, role string) Root {
	t.Helper()
	roots, err := db.FileRoots(inMain(), viewer)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range roots {
		if r.Role == role {
			return r
		}
	}
	t.Fatalf("%s sees no root %s", viewer, role)
	return Root{}
}

// upload sends a whole file in one go, as the web does in chunks.
func upload(t *testing.T, db *DB, viewer string, target UploadTarget, name string, size int64, hash string) Finished {
	t.Helper()
	ctx := inMain()
	u, err := db.StartUpload(ctx, viewer, target, name, size)
	if err != nil {
		t.Fatalf("start %s: %v", name, err)
	}
	if err := db.Received(ctx, u.ID, viewer, 0, size); err != nil {
		t.Fatal(err)
	}
	done, err := db.FinishUpload(ctx, u.ID, viewer, hash, size, "application/pdf", func() error { return nil })
	if err != nil {
		t.Fatalf("finish %s: %v", name, err)
	}
	return done
}

func TestFileRoots(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.EnsureRoots(ctx, bob, false, "admins"); err != nil {
		t.Fatal(err)
	}
	if roots, _ := db.FileRoots(ctx, bob.ID); len(roots) != 1 || roots[0].Role != RootUser || roots[0].Title != MyFiles || !roots[0].Private {
		t.Fatalf("before an administrator came, bob sees %+v", roots)
	}
	for range 2 {
		if err := db.EnsureRoots(ctx, anna, true, "admins"); err != nil {
			t.Fatal(err)
		}
	}
	tenant := rootOfRole(t, db, bob.ID, RootTenant)
	if tenant.Owner != anna.ID || tenant.Access != "read" || tenant.Writers != "admins" || tenant.Title != TenantFolder {
		t.Fatalf("the folder of the tenant: %+v", tenant)
	}
	roots, _ := db.FileRoots(ctx, bob.ID)
	if len(roots) != 2 {
		t.Fatalf("bob sees %d roots, want his own and the tenant's: %+v", len(roots), roots)
	}
	team, err := db.CreateFileCollection(ctx, bob, "Team")
	if err != nil {
		t.Fatal(err)
	}
	if roots, _ := db.FileRoots(ctx, anna.ID); len(roots) != 3 || roots[2].ID != team.ID || roots[2].Role != RootCollection {
		t.Fatalf("anna sees %+v", roots)
	}
	// The documents never show the roots of the files.
	collections, _ := db.Collections(ctx, anna.ID)
	for _, c := range collections {
		if c.Area != AreaDocs {
			t.Fatalf("documents show %+v", c)
		}
	}
	// The folders of the tenant and of a user stay.
	mine := rootOfRole(t, db, anna.ID, RootUser)
	for _, id := range []string{tenant.ID, mine.ID} {
		if err := db.Delete(ctx, id, anna.ID); !errors.Is(err, ErrNotAllowed) {
			t.Fatalf("delete %s: %v", id, err)
		}
	}
	if _, err := db.Move(ctx, team.ID, bob.ID, mine.ID, ""); !errors.Is(err, ErrNotAllowed) && !errors.Is(err, ErrPlace) && !errors.Is(err, ErrNotFound) {
		t.Fatalf("a root moved below another: %v", err)
	}
	if err := db.Delete(ctx, team.ID, bob.ID); err != nil {
		t.Fatalf("a collection is deleted by its owner: %v", err)
	}
}

func TestTheTenantFolderIsWrittenByItsGroup(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.EnsureRoots(ctx, anna, true, "admins"); err != nil {
		t.Fatal(err)
	}
	tenant := rootOfRole(t, db, anna.ID, RootTenant)
	if _, err := db.StartUpload(ctx, bob.ID, UploadTarget{Parent: tenant.ID}, "plan.pdf", 10); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("bob wrote into the folder of the tenant: %v", err)
	}
	if _, err := db.CreateFolder(ctx, bob, "Mine", tenant.ID); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("bob made a folder there: %v", err)
	}
	writer := account.User{ID: bob.ID, Login: bob.Login, Name: bob.Name, Groups: []string{"office"}}
	if err := db.SeeAccount(ctx, writer); err != nil {
		t.Fatal(err)
	}
	if err := db.SetWriters(ctx, "office"); err != nil {
		t.Fatal(err)
	}
	folder, err := db.CreateFolder(ctx, writer, "Contracts", tenant.ID)
	if err != nil {
		t.Fatalf("a writer of the tenant folder: %v", err)
	}
	if folder.Area != AreaFiles {
		t.Fatalf("a folder below a root of the files is in %q", folder.Area)
	}
	done := upload(t, db, bob.ID, UploadTarget{Parent: folder.ID}, "plan.pdf", 10, sha(1))
	if done.File.Area != AreaFiles || done.Version != 1 {
		t.Fatalf("%+v", done)
	}
}

func TestUploadsMakeFilesAndVersions(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.EnsureRoots(ctx, anna, false, ""); err != nil {
		t.Fatal(err)
	}
	mine := rootOfRole(t, db, anna.ID, RootUser)
	first := upload(t, db, anna.ID, UploadTarget{Parent: mine.ID}, "Report.pdf", 100, sha(1))
	if first.Version != 1 || first.File.Kind != KindFile || first.File.Title != "Report.pdf" {
		t.Fatalf("%+v", first)
	}
	// The same name again, also in other case, is a new version of the same file.
	second := upload(t, db, anna.ID, UploadTarget{Parent: mine.ID}, "report.PDF", 150, sha(2))
	if second.File.ID != first.File.ID || second.Version != 2 {
		t.Fatalf("second upload: %+v", second)
	}
	third := upload(t, db, anna.ID, UploadTarget{File: first.File.ID}, "anything.bin", 120, sha(3))
	if third.Version != 3 {
		t.Fatalf("third: %+v", third)
	}
	f, err := db.File(ctx, first.File.ID, anna.ID)
	if err != nil || f.Size != 120 || f.Versions != 3 || f.Blob != sha(3) || f.Title != "Report.pdf" {
		t.Fatalf("%+v %v", f, err)
	}
	if n, err := db.RestoreVersion(ctx, f.ID, anna.ID, 1); err != nil || n != 4 {
		t.Fatalf("restore: %d %v", n, err)
	}
	versions, _ := db.FileVersions(ctx, f.ID, anna.ID)
	var blobs []string
	for _, v := range versions {
		blobs = append(blobs, v.Blob)
	}
	if !slices.Equal(blobs, []string{sha(1), sha(3), sha(2), sha(1)}) || versions[0].AuthorName != anna.Name {
		t.Fatalf("versions: %+v", versions)
	}
	list, err := db.FilesIn(ctx, mine.ID, anna.ID)
	if err != nil || len(list) != 1 || list[0].Size != 100 || list[0].Versions != 4 {
		t.Fatalf("list: %+v %v", list, err)
	}
	// A folder of that name is not a file.
	if _, err := db.CreateFolder(ctx, anna, "Archive", mine.ID); err != nil {
		t.Fatal(err)
	}
	u, _ := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: mine.ID}, "archive", 1)
	_ = db.Received(ctx, u.ID, anna.ID, 0, 1)
	if _, err := db.FinishUpload(ctx, u.ID, anna.ID, sha(9), 1, "text/plain", func() error { return nil }); !errors.Is(err, ErrPlace) {
		t.Fatalf("a file over a folder: %v", err)
	}
	// Others see nothing of a private folder.
	if _, err := db.File(ctx, f.ID, bob.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob sees anna's file: %v", err)
	}
	if _, err := db.StartUpload(ctx, bob.ID, UploadTarget{File: f.ID}, "x", 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob uploads to anna's file: %v", err)
	}
}

func TestUploadsAreChecked(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.EnsureRoots(ctx, anna, false, ""); err != nil {
		t.Fatal(err)
	}
	mine := rootOfRole(t, db, anna.ID, RootUser)
	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "a\x00b", "line\nbreak", strings.Repeat("x", 201)} {
		if _, err := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: mine.ID}, name, 1); !errors.Is(err, ErrFileName) {
			t.Errorf("name %q: %v", name, err)
		}
	}
	if _, err := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: mine.ID, Page: mine.ID}, "x", 1); !errors.Is(err, ErrPlace) {
		t.Fatalf("two targets: %v", err)
	}
	if _, err := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: mine.ID}, "x", -1); !errors.Is(err, ErrUpload) {
		t.Fatalf("a negative size: %v", err)
	}
	notes, _ := db.Collections(ctx, anna.ID)
	if _, err := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: notes[0].ID}, "x", 1); !errors.Is(err, ErrPlace) {
		t.Fatalf("a file among the documents: %v", err)
	}
	u, err := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: mine.ID}, "x.txt", 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetUpload(ctx, u.ID, bob.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob sees anna's upload: %v", err)
	}
	if err := db.Received(ctx, u.ID, bob.ID, 0, 5); !errors.Is(err, ErrUpload) {
		t.Fatalf("bob writes to anna's upload: %v", err)
	}
	if err := db.Received(ctx, u.ID, anna.ID, 3, 5); !errors.Is(err, ErrUpload) {
		t.Fatalf("a chunk at the wrong offset: %v", err)
	}
	if err := db.Received(ctx, u.ID, anna.ID, 0, 11); !errors.Is(err, ErrUpload) {
		t.Fatalf("more than declared: %v", err)
	}
	if err := db.Received(ctx, u.ID, anna.ID, 0, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := db.FinishUpload(ctx, u.ID, anna.ID, sha(1), 5, "text/plain", func() error { return nil }); !errors.Is(err, ErrUpload) {
		t.Fatalf("an incomplete upload finished: %v", err)
	}
	if err := db.Received(ctx, u.ID, anna.ID, 5, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := db.FinishUpload(ctx, u.ID, bob.ID, sha(1), 10, "text/plain", func() error { return nil }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob finished anna's upload: %v", err)
	}
	failed := errors.New("store failed")
	if _, err := db.FinishUpload(ctx, u.ID, anna.ID, sha(1), 10, "text/plain", func() error { return failed }); !errors.Is(err, failed) {
		t.Fatalf("a failing store: %v", err)
	}
	if _, err := db.GetUpload(ctx, u.ID, anna.ID); err != nil {
		t.Fatalf("a failed finish keeps the upload for another try: %v", err)
	}
	if _, err := db.FinishUpload(ctx, u.ID, anna.ID, sha(1), 10, "text/plain", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := db.FinishUpload(ctx, u.ID, anna.ID, sha(1), 10, "text/plain", func() error { return nil }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("finished twice: %v", err)
	}
}

func TestQuotas(t *testing.T) {
	db := open(t)
	ctx := inMain()
	db.SetLimits(Limits{MaxFile: 80, User: 100, Collection: 50, Tenant: 300})
	if err := db.EnsureRoots(ctx, anna, true, ""); err != nil {
		t.Fatal(err)
	}
	mine := rootOfRole(t, db, anna.ID, RootUser)
	if _, err := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: mine.ID}, "big", 81); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("larger than a file may be: %v", err)
	}
	first, err := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: mine.ID}, "a", 60)
	if err != nil {
		t.Fatal(err)
	}
	// The running upload holds its room: a second one at the same time does not pass the quota.
	if _, err := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: mine.ID}, "b", 60); !errors.Is(err, ErrQuota) {
		t.Fatalf("two uploads passed the quota: %v", err)
	}
	if r := rootOfRole(t, db, anna.ID, RootUser); r.Used != 60 || r.Limit != 100 {
		t.Fatalf("used %d of %d", r.Used, r.Limit)
	}
	if err := db.CancelUpload(ctx, first.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	done := upload(t, db, anna.ID, UploadTarget{Parent: mine.ID}, "a", 60, sha(1))
	// Versions count too.
	if _, err := db.StartUpload(ctx, anna.ID, UploadTarget{File: done.File.ID}, "a", 41); !errors.Is(err, ErrQuota) {
		t.Fatalf("a version passed the quota: %v", err)
	}
	if _, err := db.RestoreVersion(ctx, done.File.ID, anna.ID, 1); !errors.Is(err, ErrQuota) {
		t.Fatalf("a restored version passed the quota: %v", err)
	}
	// An administrator gives the folder more room; the tenant still holds.
	if err := db.SetQuota(ctx, mine.ID, 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RestoreVersion(ctx, done.File.ID, anna.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: mine.ID}, "c", 80); err != nil {
		t.Fatal(err)
	}
	if _, err := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: mine.ID}, "d", 80); err != nil {
		t.Fatal(err)
	}
	if _, err := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: mine.ID}, "e", 30); !errors.Is(err, ErrQuota) {
		t.Fatalf("the tenant's quota passed: %v", err)
	}
	// A collection has its default.
	team, _ := db.CreateFileCollection(ctx, anna, "Team")
	if _, err := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: team.ID}, "f", 51); !errors.Is(err, ErrQuota) {
		t.Fatalf("the collection's quota passed: %v", err)
	}
	// Uploads that got nothing for long go, and give their room back.
	n, err := db.ExpireUploads(ctx, time.Now().Add(time.Minute), func(string) error { return nil })
	if err != nil || n != 2 {
		t.Fatalf("expired %d, %v", n, err)
	}
	if _, err := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: mine.ID}, "e", 30); err != nil {
		t.Fatal(err)
	}
	if err := db.SetQuota(ctx, mine.ID, -2); !errors.Is(err, ErrQuota) {
		t.Fatalf("a quota below -1: %v", err)
	}
}

func TestThinningVersions(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.EnsureRoots(ctx, anna, false, ""); err != nil {
		t.Fatal(err)
	}
	mine := rootOfRole(t, db, anna.ID, RootUser)
	file := upload(t, db, anna.ID, UploadTarget{Parent: mine.ID}, "a.txt", 1, sha(0)).File
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	// Versions at these ages, oldest first; the last one is the current version, however old it is.
	ages := []time.Duration{
		400 * 24 * time.Hour, 399 * 24 * time.Hour, // older than a year: gone
		100*24*time.Hour + time.Hour, 100 * 24 * time.Hour, // same week, older than 90 days: one stays
		50*24*time.Hour + time.Hour, 50 * 24 * time.Hour, // same day, between 30 and 90 days: one stays
		10*24*time.Hour + time.Hour, 10 * 24 * time.Hour, // younger than 30 days: both stay
		500 * 24 * time.Hour, // the current version
	}
	if _, err := db.pool.Exec(ctx, "DELETE FROM file_versions WHERE file = $1", file.ID); err != nil {
		t.Fatal(err)
	}
	for i, age := range ages {
		if _, err := db.pool.Exec(ctx, `INSERT INTO blobs (sha256, size) VALUES ($1, 1) ON CONFLICT DO NOTHING`, sha(i+1)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.pool.Exec(ctx, `INSERT INTO file_versions (file, number, blob, size, name, content_type, author, at)
			VALUES ($1, $2, $3, 1, 'a.txt', 'text/plain', $4, $5)`, file.ID, i+1, sha(i+1), anna.ID, now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	day := 24 * time.Hour
	gone, err := db.ThinVersions(ctx, now, Retention{KeepAll: 30 * day, KeepDaily: 90 * day, KeepWeekly: 365 * day})
	if err != nil {
		t.Fatal(err)
	}
	versions, _ := db.FileVersions(ctx, file.ID, anna.ID)
	var kept []int
	for _, v := range versions {
		kept = append(kept, v.Number)
	}
	if want := []int{9, 8, 7, 6, 4}; gone != 4 || !slices.Equal(kept, want) {
		t.Fatalf("%d went, kept %v, want %v", gone, kept, want)
	}
	// Without a weekly limit a version a week stays for good.
	if _, err := db.pool.Exec(ctx, `INSERT INTO file_versions (file, number, blob, size, name, content_type, author, at)
		VALUES ($1, 10, $2, 1, 'a.txt', 'text/plain', $3, $4)`, file.ID, sha(1), anna.ID, now.Add(-2000*day)); err != nil {
		t.Fatal(err)
	}
	if gone, err := db.ThinVersions(ctx, now, Retention{KeepAll: 30 * day, KeepDaily: 90 * day}); err != nil || gone != 0 {
		t.Fatalf("%d went, %v", gone, err)
	}
}

func TestBlobsGoWhenNothingRefersToThem(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.EnsureRoots(ctx, anna, false, ""); err != nil {
		t.Fatal(err)
	}
	mine := rootOfRole(t, db, anna.ID, RootUser)
	a := upload(t, db, anna.ID, UploadTarget{Parent: mine.ID}, "a.txt", 5, sha(1)).File
	b := upload(t, db, anna.ID, UploadTarget{Parent: mine.ID}, "b.txt", 5, sha(1)).File // equal content, kept once
	var removed []string
	collect := func() []string {
		removed = nil
		if _, err := db.CollectBlobs(ctx, func(s string) error { removed = append(removed, s); return nil }); err != nil {
			t.Fatal(err)
		}
		return removed
	}
	if got := collect(); len(got) != 0 {
		t.Fatalf("blobs in use went: %v", got)
	}
	for _, id := range []string{a.ID, b.ID} {
		if err := db.Delete(ctx, id, anna.ID); err != nil {
			t.Fatal(err)
		}
	}
	if got := collect(); len(got) != 0 {
		t.Fatalf("blobs in the trash went: %v", got)
	}
	if err := db.Purge(ctx, a.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if got := collect(); len(got) != 0 {
		t.Fatalf("a blob still used by b went: %v", got)
	}
	if err := db.Purge(ctx, b.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if got := collect(); !slices.Equal(got, []string{sha(1)}) {
		t.Fatalf("removed %v", got)
	}
	// A removal that fails keeps the row, so the next round tries again.
	upload(t, db, anna.ID, UploadTarget{Parent: mine.ID}, "c.txt", 5, sha(2))
	c, _ := db.FilesIn(ctx, mine.ID, anna.ID)
	_ = db.Delete(ctx, c[0].ID, anna.ID)
	_ = db.Purge(ctx, c[0].ID, anna.ID)
	if _, err := db.CollectBlobs(ctx, func(string) error { return errors.New("disk") }); err == nil {
		t.Fatal("a failed removal passed")
	}
	if got := collect(); !slices.Equal(got, []string{sha(2)}) {
		t.Fatalf("removed %v after a failure", got)
	}
	// The sweep removes old blobs of the store the database does not know, and leaves young ones.
	files := map[string]time.Time{sha(7): time.Now().Add(-48 * time.Hour), sha(8): time.Now()}
	each := func(fn func(string, time.Time) error) error {
		for s, m := range files {
			if err := fn(s, m); err != nil {
				return err
			}
		}
		return nil
	}
	var swept []string
	if n, err := db.SweepStore(ctx, each, func(s string) error { swept = append(swept, s); return nil }, 24*time.Hour); err != nil || n != 1 || swept[0] != sha(7) {
		t.Fatalf("swept %v, %d, %v", swept, n, err)
	}
}

func TestAttachmentsBelongToTheirPage(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.EnsureRoots(ctx, anna, false, ""); err != nil {
		t.Fatal(err)
	}
	mine := rootOfRole(t, db, anna.ID, RootUser)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	done := upload(t, db, anna.ID, UploadTarget{Page: page.ID}, "shot.png", 7, sha(1))
	if done.Attachment != "a1" || done.Page != page.ID {
		t.Fatalf("%+v", done)
	}
	second := upload(t, db, anna.ID, UploadTarget{Page: page.ID}, "shot.png", 8, sha(2))
	if second.Attachment != "a2" {
		t.Fatalf("%+v", second)
	}
	block, err := db.AddBlock(ctx, page.ID, anna.ID, "", "![shot]("+AttachmentPath(page.ID, "a1")+")")
	if err != nil {
		t.Fatal(err)
	}
	// Bob may read the page, so he sees its attachments, but may not add one while it is read only.
	if _, err := db.GetAttachment(ctx, page.ID, "a1", bob.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetAccess(ctx, team.ID, Actor{ID: anna.ID}, "read"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.StartUpload(ctx, bob.ID, UploadTarget{Page: page.ID}, "x.png", 1); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("bob attached to a read-only page: %v", err)
	}
	// A copy of the page has its own attachments, and its blocks show them.
	copied, err := db.Copy(ctx, page.ID, anna.ID, team.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := db.Attachments(ctx, copied.ID, anna.ID); len(list) != 2 {
		t.Fatalf("the copy has %d attachments", len(list))
	}
	if blocks, _ := db.Blocks(ctx, copied.ID, anna.ID); !strings.Contains(blocks[0].Body, AttachmentPath(copied.ID, "a1")) {
		t.Fatalf("the copy shows %q", blocks[0].Body)
	}
	// Moved to the files, the attachment becomes a file and the page shows it.
	file, err := db.MoveAttachment(ctx, page.ID, "a1", anna.ID, mine.ID)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := db.File(ctx, file.ID, anna.ID)
	if f.Blob != sha(1) || f.Title != "shot.png" || f.Versions != 1 {
		t.Fatalf("%+v", f)
	}
	blocks, _ := db.Blocks(ctx, page.ID, anna.ID)
	if blocks[0].ID != block.ID || blocks[0].Body != "![shot]("+FilePath(file.ID)+")" {
		t.Fatalf("the page shows %q", blocks[0].Body)
	}
	if _, err := db.GetAttachment(ctx, page.ID, "a1", anna.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the attachment stayed: %v", err)
	}
	// A second one of the same name gets a free name.
	other, err := db.MoveAttachment(ctx, page.ID, "a2", anna.ID, mine.ID)
	if err != nil || other.Title != "shot (2).png" {
		t.Fatalf("%+v %v", other, err)
	}
	// A new attachment never takes an ID given out before.
	if again := upload(t, db, anna.ID, UploadTarget{Page: page.ID}, "x.png", 1, sha(3)); again.Attachment != "a3" {
		t.Fatalf("%+v", again)
	}
	if _, err := db.MoveAttachment(ctx, page.ID, "a3", anna.ID, team.ID); !errors.Is(err, ErrPlace) {
		t.Fatalf("an attachment moved among the documents: %v", err)
	}
	// The blob of an attachment stays in the store while the page has it.
	var removed []string
	if _, err := db.CollectBlobs(ctx, func(s string) error { removed = append(removed, s); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Fatalf("blobs in use went: %v", removed)
	}
}

func TestFilesStayInTheirArea(t *testing.T) {
	db := open(t)
	ctx := inMain()
	db.SetLimits(Limits{User: 15})
	if err := db.EnsureRoots(ctx, anna, false, ""); err != nil {
		t.Fatal(err)
	}
	mine := rootOfRole(t, db, anna.ID, RootUser)
	folder, _ := db.CreateFolder(ctx, anna, "Old", mine.ID)
	file := upload(t, db, anna.ID, UploadTarget{Parent: mine.ID}, "plan.pdf", 6, sha(1)).File
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "Page", team.ID)
	if _, err := db.Move(ctx, file.ID, anna.ID, team.ID, ""); !errors.Is(err, ErrPlace) {
		t.Fatalf("a file among the documents: %v", err)
	}
	if _, err := db.Move(ctx, page.ID, anna.ID, folder.ID, ""); !errors.Is(err, ErrPlace) {
		t.Fatalf("a page among the files: %v", err)
	}
	if _, err := db.Move(ctx, folder.ID, anna.ID, file.ID, ""); !errors.Is(err, ErrPlace) {
		t.Fatalf("a folder into a file: %v", err)
	}
	if _, err := db.Move(ctx, folder.ID, anna.ID, "", ""); !errors.Is(err, ErrPlace) {
		t.Fatalf("a folder of the files at the top: %v", err)
	}
	if _, err := db.Move(ctx, file.ID, anna.ID, folder.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Copy(ctx, file.ID, anna.ID, team.ID, ""); !errors.Is(err, ErrPlace) {
		t.Fatalf("a file copied among the documents: %v", err)
	}
	copied, err := db.Copy(ctx, file.ID, anna.ID, mine.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	f, _ := db.File(ctx, copied.ID, anna.ID)
	if f.Title != "plan (copy).pdf" || f.Blob != sha(1) || f.Versions != 1 {
		t.Fatalf("the copy: %+v", f)
	}
	if _, err := db.Copy(ctx, file.ID, anna.ID, mine.ID, ""); !errors.Is(err, ErrQuota) {
		t.Fatalf("a copy passed the quota: %v", err)
	}
	// Comments go on the whole file.
	if _, err := db.AddComment(ctx, file.ID, anna.ID, "", "Looks right", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddComment(ctx, file.ID, anna.ID, "b1", "on a block", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a comment on a block of a file: %v", err)
	}
	// A space of the documents names its files.
	if err := db.LinkSpaceFiles(ctx, team.ID, anna.ID, mine.ID); err != nil {
		t.Fatal(err)
	}
	if r, err := db.SpaceFiles(ctx, team.ID, anna.ID); err != nil || r.ID != mine.ID {
		t.Fatalf("%+v %v", r, err)
	}
	// Bob sees the space but not anna's folder, so not its link either.
	if _, err := db.SpaceFiles(ctx, team.ID, bob.ID); !errors.Is(err, ErrNoLink) {
		t.Fatalf("bob sees anna's folder through the space: %v", err)
	}
	if err := db.LinkSpaceFiles(ctx, team.ID, bob.ID, mine.ID); !errors.Is(err, ErrNotAllowed) && !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob linked: %v", err)
	}
	if err := db.LinkSpaceFiles(ctx, team.ID, anna.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SpaceFiles(ctx, team.ID, anna.ID); !errors.Is(err, ErrNoLink) {
		t.Fatalf("the link stayed: %v", err)
	}
}
