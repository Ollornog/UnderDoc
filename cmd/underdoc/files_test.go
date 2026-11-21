// SPDX-License-Identifier: MIT

package main

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/store"
)

func TestParseSize(t *testing.T) {
	for in, want := range map[string]int64{"0": 0, "512": 512, "4K": 4096, "10m": 10 << 20, " 2G ": 2 << 30, "1T": 1 << 40} {
		if got, err := parseSize(in); err != nil || got != want {
			t.Errorf("%q: %d %v", in, got, err)
		}
	}
	for _, in := range []string{"", "G", "-1", "1.5G", "10X", "99999999999T"} {
		if _, err := parseSize(in); err == nil {
			t.Errorf("%q was taken", in)
		}
	}
}

func TestFilesConfig(t *testing.T) {
	t.Setenv("UNDERDOC_FILES_USER_QUOTA", "1G")
	t.Setenv("UNDERDOC_FILES_KEEP_ALL_DAYS", "7")
	c, err := readFilesConfig()
	if err != nil || c.Limits.User != 1<<30 || c.Limits.MaxFile != 2<<30 || c.Retention.KeepAll != 7*24*time.Hour || c.Retention.KeepDaily != 90*24*time.Hour {
		t.Fatalf("%+v %v", c, err)
	}
	t.Setenv("UNDERDOC_FILES_KEEP_DAILY_DAYS", "soon")
	if _, err := readFilesConfig(); err == nil || !strings.Contains(err.Error(), "UNDERDOC_FILES_KEEP_DAILY_DAYS") {
		t.Fatalf("a bad number of days: %v", err)
	}
}

// putFile stores content and records it as a new file, or a new version, through the same steps as an upload.
func putFile(t *testing.T, d *db.DB, s *store.Store, viewer string, target db.UploadTarget, name, content string) db.Finished {
	t.Helper()
	ctx := inMain()
	u, err := d.StartUpload(ctx, viewer, target, name, int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Received(ctx, u.ID, viewer, 0, u.Size); err != nil {
		t.Fatal(err)
	}
	sha, _, err := s.Put(store.DefaultTenant, strings.NewReader(content), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	done, err := d.FinishUpload(ctx, u.ID, viewer, sha, u.Size, "text/plain", func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return done
}

func content(t *testing.T, d *db.DB, s *store.Store, viewer, fileID string, number int) string {
	t.Helper()
	v, err := d.FileVersion(inMain(), fileID, viewer, number)
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Open(store.DefaultTenant, v.Blob)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	data, _ := io.ReadAll(f)
	return string(data)
}

// TestBackupOfTheFiles proves the way back of a backup (ADR-28): the database and the store are exported together, and
// imported into an empty database and an empty store they come back with their IDs, roots, versions and attachments.
func TestBackupOfTheFiles(t *testing.T) {
	ctx := inMain()
	first, url := database(t)
	anna := account.User{ID: "u-anna", Login: "anna", Name: "Anna", Groups: []string{"admins"}}
	bob := account.User{ID: "u-bob", Login: "bob", Name: "Bob"}
	for _, u := range []account.User{anna, bob} {
		if err := first.SeeAccount(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	firstStore := t.TempDir()
	s, _ := store.Open(firstStore)
	if err := first.EnsureRoots(ctx, anna, true, "admins"); err != nil {
		t.Fatal(err)
	}
	if err := first.EnsureRoots(ctx, bob, false, ""); err != nil {
		t.Fatal(err)
	}
	roots, _ := first.FileRoots(ctx, anna.ID)
	tenant, mine := roots[0], roots[1]
	contracts, _ := first.CreateFolder(ctx, anna, "Contracts", tenant.ID)
	plan := putFile(t, first, s, anna.ID, db.UploadTarget{Parent: contracts.ID}, "plan.txt", "first").File
	putFile(t, first, s, anna.ID, db.UploadTarget{File: plan.ID}, "plan.txt", "second")
	putFile(t, first, s, anna.ID, db.UploadTarget{Parent: mine.ID}, "same.txt", "second") // equal content, kept once
	bobs, _ := first.FileRoots(ctx, bob.ID)
	secret := putFile(t, first, s, bob.ID, db.UploadTarget{Parent: bobs[1].ID}, "secret.txt", "bob only").File
	team, _ := first.CreateFileCollection(ctx, anna, "Team")
	if err := first.SetQuota(ctx, team.ID, 12345); err != nil {
		t.Fatal(err)
	}
	space, _ := first.CreateCollection(ctx, anna, "Space", "")
	page, _ := first.CreatePage(ctx, anna, "Page", space.ID)
	shot := putFile(t, first, s, anna.ID, db.UploadTarget{Page: page.ID}, "shot.png", "pixels")
	if _, err := first.AddBlock(ctx, page.ID, anna.ID, "", "![shot]("+db.AttachmentPath(page.ID, shot.Attachment)+")"); err != nil {
		t.Fatal(err)
	}
	// A table of a spreadsheet with its ID (ADR-31) travels with its file.
	if _, _, err := first.ChooseTable(ctx, plan.ID, anna.ID, db.FileTable{Kind: db.TableRange, Sheet: "Budget", Range: "A1:B2",
		Anchor: []string{"Item", "Amount"}, FoundBlob: "seen"}); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "backup")
	t.Setenv("UNDERDOC_DATABASE_URL", url)
	t.Setenv("UNDERDOC_STORE_DIR", firstStore)
	code, stdout, errOut := runCommand(t, "export", out)
	if code != 0 || !strings.Contains(stdout, "exported 4 roots of the files with 3 files and 2 further versions and attachments to files/") {
		t.Fatalf("export: %d %q %q", code, stdout, errOut)
	}
	written := files(t, out)
	if written["files/Company/Contracts/plan.txt"] != "second" || written["files/My files/same.txt"] != "second" {
		t.Fatalf("the current files: %v", slices.Sorted(func(yield func(string) bool) {
			for k := range written {
				if !yield(k) {
					return
				}
			}
		}))
	}

	// An empty database and an empty store, where anna and bob have signed in once.
	second, other := database(t)
	if err := second.SeeAccount(ctx, bob); err != nil {
		t.Fatal(err)
	}
	secondStore := t.TempDir()
	t.Setenv("UNDERDOC_DATABASE_URL", other)
	t.Setenv("UNDERDOC_STORE_DIR", secondStore)
	if code, _, errOut := runCommand(t, "import", "--owner", "anna", filepath.Join(out, "Space"), "Space"); code != 0 {
		t.Fatalf("import of the documents: %q", errOut)
	}
	code, stdout, errOut = runCommand(t, "import", "--files", "--owner", "anna", filepath.Join(out, "files"))
	if code != 0 || !strings.Contains(stdout, "imported 4 roots with 8 objects, 4 versions and 1 attachments, 0 got a new id") {
		t.Fatalf("import of the files: %d %q %q", code, stdout, errOut)
	}
	s2, _ := store.Open(secondStore)
	if got := content(t, second, s2, "u-anna", plan.ID, 1); got != "first" {
		t.Errorf("version 1: %q", got)
	}
	if got := content(t, second, s2, "u-anna", plan.ID, 0); got != "second" {
		t.Errorf("version 2: %q", got)
	}
	back, _ := second.FileRoots(ctx, "u-anna")
	var roles []string
	for _, r := range back {
		roles = append(roles, r.Role+":"+r.Title)
	}
	if !slices.Equal(roles, []string{"tenant:Company", "user:My files", "collection:Team"}) || back[2].Quota != 12345 || back[0].Writers != "admins" {
		t.Errorf("roots: %v %+v", roles, back)
	}
	if table, err := second.FileTableOf(ctx, plan.ID, "x1", "u-anna"); err != nil || table.Kind != db.TableRange || table.Sheet != "Budget" ||
		table.Range != "A1:B2" || !slices.Equal(table.Anchor, []string{"Item", "Amount"}) || table.FoundBlob != "" {
		t.Errorf("the table of the file: %+v %v", table, err)
	}
	if got := content(t, second, s2, "u-bob", secret.ID, 0); got != "bob only" {
		t.Errorf("bob's file: %q", got)
	}
	if _, err := second.File(ctx, secret.ID, "u-anna"); err == nil {
		t.Error("bob's folder came back open to anna")
	}
	if a, err := second.GetAttachment(ctx, page.ID, shot.Attachment, "u-anna"); err != nil || a.Name != "shot.png" {
		t.Errorf("the attachment: %+v %v", a, err)
	}
	if again := putFile(t, second, s2, "u-anna", db.UploadTarget{Page: page.ID}, "x.png", "x"); again.Attachment != "a2" {
		t.Errorf("a new attachment after the import: %+v", again)
	}

	// Content that does not match its hash is refused, and nothing comes in.
	third, url3 := database(t)
	t.Setenv("UNDERDOC_DATABASE_URL", url3)
	t.Setenv("UNDERDOC_STORE_DIR", t.TempDir())
	if err := os.WriteFile(filepath.Join(out, "files", "Company", "Contracts", "plan.txt"), []byte("forged"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := runCommand(t, "import", "--files", "--owner", "anna", filepath.Join(out, "files")); code != 1 || !strings.Contains(errOut, "does not match its hash") {
		t.Errorf("forged content: %d %q", code, errOut)
	}
	if roots, _ := third.FileRoots(ctx, "u-anna"); len(roots) != 0 {
		t.Errorf("a refused import left %+v", roots)
	}
}
