// SPDX-License-Identifier: MIT

package share

// What the attack round on the Samba share (ADR-30, 2026-11-20) settled, beyond the attacker's own tests
// (attack_test.go, attack_samba_test.go), with the stand-in companion.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/samba"
)

// logged tells whether the log holds an entry of the working copy whose detail contains part.
func (f *fixture) logged(part string) bool {
	f.t.Helper()
	entries, err := f.db.LogEntries(inMain(), db.Actor{ID: anna.ID, Admin: true}, db.LogFilter{Limit: 500})
	if err != nil {
		f.t.Fatal(err)
	}
	for _, e := range entries {
		if e.Action == LogShare && strings.Contains(e.Detail, part) {
			return true
		}
	}
	return false
}

// A change on disk whose writer neither the audit nor the owner of the file names is never taken as the owner of the
// object: a move, a rename, a save and a deletion are put back, and the log says so. Something new stays on disk,
// untaken.
func TestUnknownWriterIsPutBack(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	if _, err := f.db.CreateFolder(ctx, anna, "Archive", f.company.ID); err != nil {
		t.Fatal(err)
	}
	moved := f.upload(anna.ID, f.company.ID, "moved.txt", "m")
	saved := f.upload(anna.ID, f.company.ID, "saved.txt", "s")
	deleted := f.upload(anna.ID, f.company.ID, "deleted.txt", "d")
	f.sync()
	if err := os.Rename(f.path("moved.txt"), f.path("Archive/moved.txt")); err != nil {
		t.Fatal(err)
	}
	f.write("saved.txt", "changed by nobody known")
	if err := os.Remove(f.path("deleted.txt")); err != nil {
		t.Fatal(err)
	}
	f.write("new.txt", "made by nobody known")
	f.sync()
	if o, err := f.db.Object(ctx, moved.ID, anna.ID); err != nil || o.Parent != f.company.ID {
		t.Errorf("the move of an unknown writer was taken: %+v %v", o, err)
	}
	if _, err := os.Stat(f.path("moved.txt")); err != nil {
		t.Errorf("the move was not put back on disk: %v", err)
	}
	if _, err := os.Stat(f.path("Archive/moved.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the moved file stays in Archive on disk")
	}
	if len(f.versions(saved.ID)) != 1 || f.read("saved.txt") != "s" {
		t.Errorf("the save of an unknown writer: %d versions, %q on disk", len(f.versions(saved.ID)), f.read("saved.txt"))
	}
	if _, err := f.db.Object(ctx, deleted.ID, anna.ID); err != nil || f.read("deleted.txt") != "d" {
		t.Errorf("the deletion of an unknown writer was taken: %v", err)
	}
	list, _ := f.db.FilesIn(ctx, f.company.ID, anna.ID)
	for _, x := range list {
		if x.Title == "new.txt" {
			t.Errorf("something new of an unknown writer was taken: %+v", x)
		}
	}
	if f.read("new.txt") != "made by nobody known" {
		t.Error("something new of an unknown writer left the disk")
	}
	for _, part := range []string{"a move or rename over Samba was put back", "a save over Samba was put back",
		"a deletion over Samba was put back", "new.txt is new on the share and was not taken"} {
		if !f.logged(part) {
			t.Errorf("the log does not say %q", part)
		}
	}
}

// The audit numbers its events; a pass that finds the oldest one it keeps beyond the newest it saw before knows events
// were pushed out, and says so in the log.
func TestLostAuditIsLogged(t *testing.T) {
	f := setup(t)
	f.h.first, f.h.last = 1, 10
	f.sync()
	f.h.first, f.h.last = 11, 20
	f.sync()
	if f.logged("lost") {
		t.Fatal("a pass that missed nothing logged a loss")
	}
	f.h.first, f.h.last = 50, 60
	f.sync()
	if !f.logged("the audit of the share lost 29 events") {
		t.Error("the loss of 29 events of the audit is not in the log")
	}
}

// A hard link made over SMB (two names, one inode, one ID) ends at the next pass: the name the record knows keeps the
// object and its ID, the other becomes a copy of its own, a new file of whoever made the link.
func TestHardLinkBecomesACopy(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	if err := f.db.SetGrant(ctx, f.company.ID, db.Actor{ID: anna.ID}, db.ToPerson, bob.ID, db.RoleEdit); err != nil {
		t.Fatal(err)
	}
	file := f.upload(anna.ID, f.company.ID, "budget.txt", "v1")
	f.sync()
	if err := os.Link(f.path("budget.txt"), f.path("link.txt")); err != nil {
		t.Fatal(err)
	}
	f.h.mu.Lock()
	f.h.who = append(f.h.who, samba.Event{User: "bob", Op: "linkat", Path: f.path("budget.txt"), To: f.path("link.txt")})
	f.h.mu.Unlock()
	f.sync()
	if got := readID(f.path("budget.txt")); got != file.ID {
		t.Errorf("budget.txt carries %q, not %q", got, file.ID)
	}
	var a, b os.FileInfo
	var err error
	if a, err = os.Stat(f.path("budget.txt")); err != nil {
		t.Fatal(err)
	}
	if b, err = os.Stat(f.path("link.txt")); err != nil {
		t.Fatal(err)
	}
	if os.SameFile(a, b) {
		t.Error("the hard link is still there")
	}
	list, _ := f.db.FilesIn(ctx, f.company.ID, anna.ID)
	var copyID string
	for _, x := range list {
		if x.Title == "link.txt" {
			copyID = x.ID
			if x.Owner != bob.ID || f.current(x.ID) != "v1" {
				t.Errorf("the copy: %+v", x)
			}
		}
	}
	if copyID == "" || readID(f.path("link.txt")) != copyID || copyID == file.ID {
		t.Errorf("link.txt became no object of its own: %q, files %+v", copyID, list)
	}
	if _, err := f.db.Object(ctx, file.ID, anna.ID); err != nil {
		t.Errorf("the original object: %v", err)
	}
}

// A link from another library (two shares on one file system) is ended there too, and never takes the ID.
func TestHardLinkFromAnotherLibrary(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	team, err := f.db.CreateFileCollection(ctx, anna, "Team")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetRootSamba(ctx, team.ID, anna.ID, true); err != nil {
		t.Fatal(err)
	}
	file := f.upload(anna.ID, f.company.ID, "budget.txt", "v1")
	f.sync()
	other := filepath.Join(f.dir, team.ID, "budget.txt")
	if err := os.Link(f.path("budget.txt"), other); err != nil {
		t.Fatal(err)
	}
	f.h.wrote("anna", other)
	f.sync()
	f.sync()
	if got := readID(f.path("budget.txt")); got != file.ID {
		t.Errorf("budget.txt carries %q, not %q", got, file.ID)
	}
	if got := readID(other); got == file.ID || got == "" {
		t.Errorf("the link in Team carries %q", got)
	}
}

// A file that breaks the inheritance never reaches the share: a library holding one is not shared and nothing holding
// one moves into a shared library, each answer naming the file; and one that is in a shared library anyway, as in the
// folder of the tenant when the companion came later, stays off the disk (or leaves it), and the log says so.
func TestBrokenFilesStayOffTheShare(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	admin := db.Actor{ID: anna.ID, Admin: true}
	team, err := f.db.CreateFileCollection(ctx, anna, "Team")
	if err != nil {
		t.Fatal(err)
	}
	folder, err := f.db.CreateFolder(ctx, anna, "Reviews", team.ID)
	if err != nil {
		t.Fatal(err)
	}
	review := f.upload(anna.ID, folder.ID, "review.txt", "not good")
	if err := f.db.SetInherit(ctx, review.ID, admin, false); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetRootSamba(ctx, team.ID, anna.ID, true); !errors.Is(err, db.ErrPerFolder) || !strings.Contains(err.Error(), "review.txt") {
		t.Errorf("sharing a library with a broken file: %v", err)
	}
	if _, err := f.db.Move(ctx, folder.ID, anna.ID, f.company.ID, ""); !errors.Is(err, db.ErrPerFolder) || !strings.Contains(err.Error(), "review.txt") {
		t.Errorf("moving a folder with a broken file into a shared library: %v", err)
	}
	// Inheriting again, it goes.
	if err := f.db.SetInherit(ctx, review.ID, admin, true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Move(ctx, folder.ID, anna.ID, f.company.ID, ""); err != nil {
		t.Errorf("the folder after its file inherits again: %v", err)
	}
	// The folder of the tenant holds a file that broke the inheritance while there was no companion: written straight
	// into the database, as SetInherit refuses now.
	f.sync()
	if f.read("Reviews/review.txt") != "not good" {
		t.Fatal("the file is not on disk to begin with")
	}
	hidden := f.upload(anna.ID, f.company.ID, "hidden.txt", "never on disk")
	conn, err := pgx.Connect(context.Background(), f.url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(context.Background(), "UPDATE objects SET inherit = false WHERE id = ANY($1)", []string{review.ID, hidden.ID}); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close(context.Background())
	f.sync()
	for _, p := range []string{"Reviews/review.txt", "hidden.txt"} {
		if _, err := os.Stat(f.path(p)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s, which breaks the inheritance, is on the share", p)
		}
	}
	for _, id := range []string{review.ID, hidden.ID} {
		if _, err := f.db.Object(ctx, id, anna.ID); err != nil {
			t.Errorf("a file kept off the share left the database: %v", err)
		}
	}
	if !f.logged("kept off the share") {
		t.Error("the log does not say a file was kept off the share")
	}
}

// A file moved to another folder, in the web or over SMB, gets the ACL of the folder it went to: the companion sets the
// ACL of the folder's files only when the folder's own changes.
func TestAMovedFileGetsTheACLOfItsFolder(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	admin := db.Actor{ID: anna.ID, Admin: true}
	hr, err := f.db.CreateFolder(ctx, anna, "HR", f.company.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetInherit(ctx, hr.ID, admin, false); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetGrant(ctx, hr.ID, admin, db.ToTenant, "", db.RoleNone); err != nil {
		t.Fatal(err)
	}
	web := f.upload(anna.ID, f.company.ID, "salaries.txt", "draft")
	f.upload(anna.ID, f.company.ID, "bonus.txt", "draft")
	f.sync()
	hrACL := f.h.acl[f.path("HR")]
	if _, ok := hrACL[20001]; ok {
		t.Fatalf("Bob reaches HR: %v", hrACL)
	}
	if _, err := f.db.Move(ctx, web.ID, anna.ID, hr.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(f.path("bonus.txt"), f.path("HR/bonus.txt")); err != nil {
		t.Fatal(err)
	}
	f.h.wrote("anna", f.path("HR/bonus.txt"))
	f.h.acl = nil
	f.sync()
	for _, p := range []string{"HR/salaries.txt", "HR/bonus.txt"} {
		got, ok := f.h.acl[f.path(p)]
		if !ok || len(got) != len(hrACL) {
			t.Errorf("%s after the move: %v (HR: %v)", p, got, hrACL)
		}
		if _, bob := got[20001]; bob {
			t.Errorf("%s keeps Bob from the folder it left: %v", p, got)
		}
	}
}

// The patterns of temporary files hold for files only: a folder named like one is an ordinary folder with everything
// in it in the database; a file made in the web with such a name stays off the disk and in the database.
func TestTemporaryNamesAreForFilesOnly(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	f.sync()
	for _, dir := range []string{"Backup.tmp", "._old", "~$drafts"} {
		if err := os.Mkdir(f.path(dir), 0o770); err != nil {
			t.Fatal(err)
		}
		f.write(dir+"/inside.txt", "kept")
		f.h.wrote("anna", f.path(dir))
		f.h.wrote("anna", f.path(dir+"/inside.txt"))
	}
	f.sync()
	kids, err := f.db.Children(ctx, f.company.ID, anna.ID)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, k := range kids {
		names = append(names, k.Title)
		if files, _ := f.db.FilesIn(ctx, k.ID, anna.ID); len(files) != 1 {
			t.Errorf("%s holds %+v", k.Title, files)
		}
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"._old", "Backup.tmp", "~$drafts"}) {
		t.Errorf("folders: %v", names)
	}
	web := f.upload(anna.ID, f.company.ID, "notes.tmp", "from the web")
	f.sync()
	f.sync()
	if _, err := f.db.Object(ctx, web.ID, anna.ID); err != nil {
		t.Errorf("a file of the web named like a temporary one went: %v", err)
	}
	if _, err := os.Stat(f.path("notes.tmp")); !errors.Is(err, os.ErrNotExist) {
		t.Error("a file of the web named like a temporary one is on disk")
	}
}

// A folder with '%' in its path is no share of its own (smb.conf would read %U in it as the user's name); the folder
// stays reachable through its library.
func TestPercentIsNoShareOfItsOwn(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	team, err := f.db.CreateFileCollection(ctx, anna, "Team")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetRootSamba(ctx, team.ID, anna.ID, true); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"100%U", "Plain"} {
		inner, err := f.db.CreateFolder(ctx, anna, name, team.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.db.SetGrant(ctx, inner.ID, db.Actor{ID: anna.ID}, db.ToPerson, bob.ID, db.RoleRead); err != nil {
			t.Fatal(err)
		}
	}
	f.sync()
	var names []string
	for _, s := range f.h.state.Shares {
		names = append(names, s.Name)
		if strings.ContainsRune(s.Path, '%') {
			t.Errorf("a share with '%%' in its path: %+v", s)
		}
	}
	if !slices.Contains(names, "Plain") {
		t.Errorf("shares: %v", names)
	}
	if err := (samba.Request{Op: samba.OpApply, State: &f.h.state}).Check(f.dir, 20000, 29999); err != nil {
		t.Errorf("the companion refuses the state: %v", err)
	}
}

// A share keeps its name when a Samba user comes with it: the web refuses such a name, and if one is there anyway, the
// share of the pass before keeps its name.
func TestAShareKeepsItsName(t *testing.T) {
	f := setup(t)
	f.sync()
	if !slices.ContainsFunc(f.h.state.Shares, func(s samba.Share) bool { return s.Name == "Company" }) {
		t.Fatalf("shares: %+v", f.h.state.Shares)
	}
	if taken, err := f.m.NameTaken(inMain(), "company"); err != nil || !taken {
		t.Errorf("the name of the share of the tenant: %v %v", taken, err)
	}
	if taken, _ := f.m.NameTaken(inMain(), "carl"); taken {
		t.Error("carl is taken")
	}
	carlNamed := carl
	carlNamed.Login = "company"
	f.password(carlNamed)
	f.sync()
	if !slices.ContainsFunc(f.h.state.Shares, func(s samba.Share) bool { return s.Name == "Company" }) {
		t.Errorf("a Samba user pushed the share of the tenant aside: %+v", f.h.state.Shares)
	}
}

// A save made shortly before an administrator removed the app password is taken as the account that made it: the pass
// remembers the Samba users of the passes before while it runs.
func TestRemovedAppPasswordTakesTheLastChanges(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	if err := f.db.EnsureRoots(ctx, bob, false, "admins"); err != nil {
		t.Fatal(err)
	}
	f.sync()
	roots, err := f.db.FileRoots(ctx, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	var home db.Root
	for _, r := range roots {
		if r.Role == db.RootUser {
			home = r
		}
	}
	dir := filepath.Join(f.dir, home.ID)
	if err := os.WriteFile(filepath.Join(dir, "thesis.txt"), []byte("chapter one"), 0o660); err != nil {
		t.Fatal(err)
	}
	f.h.wrote("bob", filepath.Join(dir, "thesis.txt"))
	if err := f.db.RemoveSambaAccount(ctx, db.Actor{ID: anna.ID, Admin: true}, bob.ID); err != nil {
		t.Fatal(err)
	}
	f.sync()
	list, err := f.db.FilesIn(ctx, home.ID, bob.ID)
	if err != nil || len(list) != 1 || list[0].Title != "thesis.txt" || list[0].Owner != bob.ID {
		t.Fatalf("bob's folder: %+v %v", list, err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Error("bob's folder stays on disk though everything in it was taken")
	}
}

// A library that stops being shared stays on disk while it holds something the database did not take, here a file whose
// writer is unknown, and the log says so; once that is gone, the next pass takes the directory away.
func TestUnsharedKeepsWhatWasNotTaken(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	team, err := f.db.CreateFileCollection(ctx, anna, "Team")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetRootSamba(ctx, team.ID, anna.ID, true); err != nil {
		t.Fatal(err)
	}
	f.upload(anna.ID, team.ID, "known.txt", "in the database")
	f.sync()
	stray := filepath.Join(f.dir, team.ID, "stray.txt")
	if err := os.WriteFile(stray, []byte("nobody known"), 0o660); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetRootSamba(ctx, team.ID, anna.ID, false); err != nil {
		t.Fatal(err)
	}
	f.sync()
	if _, err := os.Stat(stray); err != nil {
		t.Fatalf("a file the database did not take left the disk: %v", err)
	}
	if !f.logged("stays on disk") {
		t.Error("the log does not say the library stays on disk")
	}
	if err := os.Remove(stray); err != nil {
		t.Fatal(err)
	}
	f.sync()
	if _, err := os.Stat(filepath.Join(f.dir, team.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Error("the library stays on disk with nothing untaken in it")
	}
}
