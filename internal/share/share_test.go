// SPDX-License-Identifier: MIT

package share

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/samba"
	"github.com/Ollornog/UnderDoc/internal/store"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Main(m)) }

func inMain() context.Context { return tenant.With(context.Background(), tenant.Main) }

var (
	anna = account.User{ID: "u-anna", Login: "anna", Name: "Anna"}
	bob  = account.User{ID: "u-bob", Login: "bob", Name: "Bob"}
	carl = account.User{ID: "u-carl", Login: "carl", Name: "Carl"}
)

// fakeHelper stands in for the companion: it keeps what it was given and answers what the test says is open and who
// changed what.
type fakeHelper struct {
	mu    sync.Mutex
	state samba.State
	acl   map[string]map[int]samba.Access
	open  []samba.Open
	who   []samba.Event
	first uint64 // the numbers of the audit's oldest and newest event, as the companion answers them
	last  uint64
	calls map[string]int
	boot  string
}

func (f *fakeHelper) Do(_ context.Context, r samba.Request) (samba.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[r.Op]++
	switch r.Op {
	case samba.OpApply:
		f.state = *r.State
	case samba.OpACL:
		if f.acl == nil {
			f.acl = map[string]map[int]samba.Access{}
		}
		for _, a := range r.ACL {
			f.acl[a.Path] = a.Access
		}
	case samba.OpOpen:
		return samba.Response{OK: true, Open: f.open}, nil
	case samba.OpWho:
		return samba.Response{OK: true, Who: f.who, First: f.first, Last: f.last}, nil
	case samba.OpPing:
		return samba.Response{OK: true, Boot: f.boot}, nil
	}
	return samba.Response{OK: true}, nil
}

// wrote tells the fake audit that a Samba user changed a path.
func (f *fakeHelper) wrote(user, full string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.who = append(f.who, samba.Event{User: user, Op: "renameat", Path: full})
}

type fixture struct {
	t       *testing.T
	db      *db.DB
	store   *store.Store
	m       *Manager
	h       *fakeHelper
	company db.Root
	dir     string
	url     string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := inMain()
	url := pgtest.URL(t)
	database, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	for _, u := range []account.User{anna, bob, carl} {
		if err := database.SeeAccount(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	v, err := vault.New(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	h := &fakeHelper{}
	shareDir := t.TempDir()
	m, err := New(database, st, v, Config{Dir: shareDir, Helpers: map[string]Helper{tenant.Main: h}, Server: "files"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.EnsureRoots(ctx, anna, true, "admins"); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, db: database, store: st, m: m, h: h, dir: filepath.Join(shareDir, tenant.Main), url: url}
	roots, err := database.FileRoots(ctx, anna.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range roots {
		if r.Role == db.RootTenant {
			f.company = r
		}
	}
	// Anna and Bob have app passwords; Carl has none.
	for _, u := range []account.User{anna, bob} {
		f.password(u)
	}
	return f
}

func (f *fixture) password(u account.User) {
	f.t.Helper()
	ctx := inMain()
	sealed, err := f.m.vault.Seal(tenant.Main, db.SambaSecretName(u.ID), vault.Secret(samba.NTHash("pw-"+u.Login)))
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.db.SetSambaPassword(ctx, u.ID, u.Login, sealed, 20000, 29999); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) sync() {
	f.t.Helper()
	if err := f.m.Sync(context.Background(), tenant.Main); err != nil {
		f.t.Fatal(err)
	}
}

// upload puts a file into a folder through the web's way of uploading.
func (f *fixture) upload(viewer, parent, name, content string) db.Object {
	f.t.Helper()
	ctx := inMain()
	sha, size, err := f.store.Put(tenant.Main, strings.NewReader(content), 1<<20)
	if err != nil {
		f.t.Fatal(err)
	}
	u, err := f.db.StartUpload(ctx, viewer, db.UploadTarget{Parent: parent}, name, size)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := f.db.Received(ctx, u.ID, viewer, 0, size); err != nil {
		f.t.Fatal(err)
	}
	done, err := f.db.FinishUpload(ctx, u.ID, viewer, sha, size, "text/plain", func() error { return nil })
	if err != nil {
		f.t.Fatal(err)
	}
	return done.File
}

// path is the absolute path of a path in the folder of the tenant.
func (f *fixture) path(rel string) string {
	return filepath.Join(f.dir, f.company.ID, filepath.FromSlash(rel))
}

func (f *fixture) read(rel string) string {
	f.t.Helper()
	b, err := os.ReadFile(f.path(rel))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

func (f *fixture) write(rel, content string) {
	f.t.Helper()
	if err := os.WriteFile(f.path(rel), []byte(content), 0o660); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) versions(fileID string) []db.FileVersion {
	f.t.Helper()
	v, err := f.db.FileVersions(inMain(), fileID, anna.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

func (f *fixture) current(fileID string) string {
	f.t.Helper()
	info, err := f.db.File(inMain(), fileID, anna.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	r, err := f.store.Open(tenant.Main, info.Blob)
	if err != nil {
		f.t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	b := make([]byte, 1<<16)
	n, _ := r.Read(b)
	return string(b[:n])
}

// What lies in a shared library in the web is on disk after a pass, with its ID, and the companion knows the users and
// the share.
func TestTheWebShowsOnDisk(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	folder, err := f.db.CreateFolder(ctx, anna, "Contracts", f.company.ID)
	if err != nil {
		t.Fatal(err)
	}
	file := f.upload(anna.ID, folder.ID, "plan.txt", "first")
	f.sync()
	if got := f.read("Contracts/plan.txt"); got != "first" {
		t.Fatalf("on disk: %q", got)
	}
	if id := readID(f.path("Contracts/plan.txt")); id != file.ID {
		t.Errorf("the file carries %q, want %q", id, file.ID)
	}
	if id := readID(f.path("Contracts")); id != folder.ID {
		t.Errorf("the folder carries %q", id)
	}
	if len(f.h.state.Users) != 2 || len(f.h.state.Shares) != 1 || f.h.state.Shares[0].Name != "Company" {
		t.Fatalf("the companion has %+v", f.h.state)
	}
	if f.h.state.Users[0].NTHash != samba.NTHash("pw-anna") {
		t.Error("the companion has not the NT hash of the app password")
	}
	// On disk: the folder of the tenant and Anna's own folder (she has an app password and opened Files); Bob never
	// opened Files, so he has no folder yet, and Carl has no app password.
	entries, _ := os.ReadDir(f.dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	mine := ""
	if roots, err := f.db.FileRoots(inMain(), anna.ID); err == nil {
		for _, r := range roots {
			if r.Role == db.RootUser {
				mine = r.ID
			}
		}
	}
	want := []string{f.company.ID, mine}
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Errorf("libraries on disk: %v, want %v", names, want)
	}
	if f.h.state.Users[0].Home != filepath.Join(f.dir, mine) || f.h.state.Users[1].Home != "" {
		t.Errorf("homes: %+v", f.h.state.Users)
	}
	// A second pass changes nothing: the companion is not asked again.
	applied := f.h.calls[samba.OpApply]
	acls := f.h.calls[samba.OpACL]
	f.sync()
	if f.h.calls[samba.OpApply] != applied || f.h.calls[samba.OpACL] != acls {
		t.Errorf("a pass without a change applied again: %v", f.h.calls)
	}
	// A companion that started again gets its users and shares again.
	f.h.mu.Lock()
	f.h.boot = "again"
	f.h.state = samba.State{}
	f.h.mu.Unlock()
	f.sync()
	if len(f.h.state.Users) != 2 {
		t.Errorf("after the companion started again it has %+v", f.h.state.Users)
	}
}

// Office saves through a temporary file and a rename (T-1): the file at the old path has no ID, the old one went to a
// backup name and is deleted. It is the same object with a new version, by whoever saved it, and carries the ID again.
func TestOfficeSaveBecomesAVersion(t *testing.T) {
	f := setup(t)
	file := f.upload(anna.ID, f.company.ID, "budget.xlsx", "v1")
	f.sync()
	// Excel: write ~ABC.tmp, rename budget.xlsx → ~XYZ.tmp, rename ~ABC.tmp → budget.xlsx, delete ~XYZ.tmp.
	f.write("~ABC.tmp", "v2 by bob")
	if err := os.Rename(f.path("budget.xlsx"), f.path("~XYZ.tmp")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(f.path("~ABC.tmp"), f.path("budget.xlsx")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(f.path("~XYZ.tmp")); err != nil {
		t.Fatal(err)
	}
	f.h.wrote("anna", f.path("budget.xlsx"))
	f.sync()
	v := f.versions(file.ID)
	if len(v) != 2 || v[0].Author != anna.ID {
		t.Fatalf("versions: %+v", v)
	}
	if got := f.current(file.ID); got != "v2 by bob" {
		t.Errorf("current: %q", got)
	}
	if id := readID(f.path("budget.xlsx")); id != file.ID {
		t.Errorf("the saved file carries %q, want %q", id, file.ID)
	}
	trash, _ := f.db.Trash(inMain(), anna.ID)
	if len(trash) != 0 {
		t.Errorf("a save put something into the trash: %+v", trash)
	}
	// Temporary files never become objects.
	f.write("~$budget.xlsx", "lock")
	f.write(".~lock.budget.xlsx#", "lock")
	f.sync()
	list, _ := f.db.FilesIn(inMain(), f.company.ID, anna.ID)
	if len(list) != 1 {
		t.Errorf("files: %d, temporary files became objects", len(list))
	}
}

// A file saved in place becomes a version by the account the audit names, or by the Samba user that owns the file on
// disk; never by the owner of the object (TestUnknownWriterIsPutBack).
func TestSavedInPlace(t *testing.T) {
	f := setup(t)
	file := f.upload(anna.ID, f.company.ID, "notes.txt", "one")
	if err := f.db.SetGrant(inMain(), f.company.ID, db.Actor{ID: anna.ID}, db.ToPerson, bob.ID, db.RoleEdit); err != nil {
		t.Fatal(err)
	}
	f.sync()
	f.write("notes.txt", "two, longer")
	f.h.wrote("bob", f.path("notes.txt"))
	f.sync()
	v := f.versions(file.ID)
	if len(v) != 2 || v[0].Author != bob.ID || f.current(file.ID) != "two, longer" {
		t.Fatalf("versions: %+v", v)
	}
}

// Renaming and moving on disk renames and moves the object; deleting moves it to the trash.
func TestRenameMoveDeleteOnDisk(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	folder, _ := f.db.CreateFolder(ctx, anna, "Archive", f.company.ID)
	file := f.upload(anna.ID, f.company.ID, "a.txt", "a")
	other := f.upload(anna.ID, f.company.ID, "b.txt", "b")
	f.sync()
	if err := os.Rename(f.path("a.txt"), f.path("Archive/renamed.txt")); err != nil {
		t.Fatal(err)
	}
	f.h.wrote("anna", f.path("Archive/renamed.txt"))
	if err := os.Remove(f.path("b.txt")); err != nil {
		t.Fatal(err)
	}
	f.h.wrote("anna", f.path("b.txt"))
	f.sync()
	o, err := f.db.Object(ctx, file.ID, anna.ID)
	if err != nil || o.Parent != folder.ID || o.Title != "renamed.txt" {
		t.Fatalf("after the move: %+v %v", o, err)
	}
	if _, err := f.db.Object(ctx, other.ID, anna.ID); !errors.Is(err, db.ErrNotFound) {
		t.Errorf("the deleted file is still there: %v", err)
	}
	trash, _ := f.db.Trash(ctx, anna.ID)
	if len(trash) != 1 || trash[0].ID != other.ID {
		t.Fatalf("trash: %+v", trash)
	}
	// Restored in the web, it comes back on disk.
	if _, err := f.db.Restore(ctx, other.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	f.sync()
	if f.read("b.txt") != "b" {
		t.Error("the restored file is not on disk")
	}
}

// New files and folders on disk become objects of the account that made them.
func TestNewOnDisk(t *testing.T) {
	f := setup(t)
	f.sync()
	if err := os.Mkdir(f.path("Offers"), 0o770); err != nil {
		t.Fatal(err)
	}
	f.write("Offers/offer.txt", "hello")
	f.h.wrote("anna", f.path("Offers"))
	f.h.wrote("anna", f.path("Offers/offer.txt"))
	f.sync()
	ctx := inMain()
	kids, err := f.db.Children(ctx, f.company.ID, anna.ID)
	if err != nil || len(kids) != 1 || kids[0].Title != "Offers" || kids[0].Owner != anna.ID {
		t.Fatalf("children: %+v %v", kids, err)
	}
	files, _ := f.db.FilesIn(ctx, kids[0].ID, anna.ID)
	if len(files) != 1 || files[0].Title != "offer.txt" || files[0].Owner != anna.ID || f.current(files[0].ID) != "hello" {
		t.Fatalf("files: %+v", files)
	}
	if readID(f.path("Offers/offer.txt")) != files[0].ID {
		t.Error("the new file carries no ID")
	}
	// Bob, who may edit the folder of the tenant, makes a file: it is his, not the owner's of the folder.
	if err := f.db.SetGrant(ctx, f.company.ID, db.Actor{ID: anna.ID}, db.ToPerson, bob.ID, db.RoleEdit); err != nil {
		t.Fatal(err)
	}
	f.write("bobs.txt", "from bob")
	f.h.wrote("bob", f.path("bobs.txt"))
	f.sync()
	list, _ := f.db.FilesIn(ctx, f.company.ID, anna.ID)
	owner := ""
	for _, x := range list {
		if x.Title == "bobs.txt" {
			owner = x.Owner
		}
	}
	if owner != bob.ID {
		t.Errorf("Bob's file belongs to %q", owner)
	}
}

// A change in the web writes to disk: a new version, a rename, a deletion.
func TestTheWebWritesToDisk(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	file := f.upload(anna.ID, f.company.ID, "a.txt", "one")
	f.sync()
	f.upload(anna.ID, f.company.ID, "a.txt", "two") // the same name: a new version
	if _, err := f.db.Rename(ctx, file.ID, anna.ID, "b.txt"); err != nil {
		t.Fatal(err)
	}
	f.sync()
	if _, err := os.Stat(f.path("a.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the old name is still on disk")
	}
	if f.read("b.txt") != "two" || readID(f.path("b.txt")) != file.ID {
		t.Fatal("the new name does not hold the new version with its ID")
	}
	// A name of the files becomes a path on the share: one with a slash is refused, also in a rename.
	if _, err := f.db.Rename(ctx, file.ID, anna.ID, "x/y.txt"); !errors.Is(err, db.ErrFileName) {
		t.Errorf("a rename to a path: %v", err)
	}
	if err := f.db.Delete(ctx, file.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	f.sync()
	if _, err := os.Stat(f.path("b.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the deleted file is still on disk")
	}
}

// While a file is open over SMB, the web does not write it, and says who has it open.
func TestOpenOverSambaHoldsTheWebBack(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	file := f.upload(anna.ID, f.company.ID, "open.txt", "one")
	f.sync()
	f.h.open = []samba.Open{{Path: f.path("open.txt"), User: "bob", Write: true}}
	_, err := f.db.Rename(ctx, file.ID, anna.ID, "other.txt")
	var open *db.OpenError
	if !errors.As(err, &open) || !slices.Equal(open.Users, []string{"bob"}) {
		t.Fatalf("rename of an open file: %v", err)
	}
	if err := f.db.Delete(ctx, file.ID, anna.ID); !errors.As(err, &open) {
		t.Errorf("delete of an open file: %v", err)
	}
	if _, err := f.db.RestoreVersion(ctx, file.ID, anna.ID, 1); !errors.As(err, &open) {
		t.Errorf("restore of an open file: %v", err)
	}
	place, ok := f.m.PlaceOf(ctx, file.ID, anna.ID)
	if !ok || place.Path != `\\files\Company\open.txt` || !slices.Equal(place.OpenBy, []string{"bob"}) {
		t.Errorf("place: %+v %v", place, ok)
	}
	// A change on disk while it is open for writing waits.
	f.write("open.txt", "half")
	f.h.wrote("anna", f.path("open.txt"))
	f.sync()
	if len(f.versions(file.ID)) != 1 {
		t.Error("a file open for writing became a version")
	}
	f.h.open = nil
	f.sync()
	if len(f.versions(file.ID)) != 2 {
		t.Error("the file closed did not become a version")
	}
	if _, err := f.db.Rename(ctx, file.ID, anna.ID, "other.txt"); err != nil {
		t.Errorf("rename after it was closed: %v", err)
	}
}

// What the database refuses goes back on disk: Bob reads the folder of the tenant only, so his rename is put back.
func TestRefusedChangesGoBack(t *testing.T) {
	f := setup(t)
	file := f.upload(anna.ID, f.company.ID, "keep.txt", "anna's")
	f.sync()
	if err := os.Rename(f.path("keep.txt"), f.path("mine.txt")); err != nil {
		t.Fatal(err)
	}
	f.h.wrote("bob", f.path("mine.txt"))
	f.sync()
	o, err := f.db.Object(inMain(), file.ID, anna.ID)
	if err != nil || o.Title != "keep.txt" {
		t.Fatalf("the object: %+v %v", o, err)
	}
	if f.read("keep.txt") != "anna's" {
		t.Error("the rename was not put back")
	}
	if _, err := os.Stat(f.path("mine.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Error("the refused name stays on disk")
	}
}

// Roles become ACLs per folder: read and comment read, edit writes, nothing else. A folder that breaks the inheritance
// shuts out who it no longer grants; a folder in which someone holds a role without reading the folder above is a
// share of its own for them, with passage above it.
func TestRolesBecomeACLs(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	admin := db.Actor{ID: anna.ID, Admin: true}
	secret, _ := f.db.CreateFolder(ctx, anna, "Board", f.company.ID)
	if err := f.db.SetInherit(ctx, secret.ID, admin, false); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetGrant(ctx, secret.ID, admin, db.ToTenant, "", db.RoleNone); err != nil {
		t.Fatal(err)
	}
	team, err := f.db.CreateFileCollection(ctx, anna, "Team")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetRootSamba(ctx, team.ID, anna.ID, true); err != nil {
		t.Fatal(err)
	}
	inner, _ := f.db.CreateFolder(ctx, anna, "For Bob", team.ID)
	if err := f.db.SetGrant(ctx, inner.ID, db.Actor{ID: anna.ID}, db.ToPerson, bob.ID, db.RoleEdit); err != nil {
		t.Fatal(err)
	}
	f.sync()
	a, b := 20000, 20001
	company := f.h.acl[f.path("")]
	if company[a] != samba.Write || company[b] != samba.Read {
		t.Errorf("Company: %v", company)
	}
	board := f.h.acl[f.path("Board")]
	if board[a] != samba.Write {
		t.Errorf("Board: %v", board)
	}
	if _, ok := board[b]; ok {
		t.Errorf("Bob reaches Board on the share: %v", board)
	}
	teamDir := filepath.Join(f.dir, team.ID)
	if got := f.h.acl[teamDir][b]; got != samba.Pass {
		t.Errorf("Bob's passage through Team: %q", got)
	}
	if got := f.h.acl[filepath.Join(teamDir, "For Bob")][b]; got != samba.Write {
		t.Errorf("Bob on For Bob: %q", got)
	}
	shares := map[string][]string{}
	for _, s := range f.h.state.Shares {
		shares[s.Name] = s.Users
	}
	if !slices.Equal(shares["Team"], []string{"anna"}) || !slices.Equal(shares["For Bob"], []string{"bob"}) ||
		!slices.Equal(shares["Company"], []string{"anna", "bob"}) {
		t.Errorf("shares: %+v", shares)
	}
	if got := f.m.Reaches(ctx, bob.ID); !slices.Equal(got, []string{`\\files\Company`, `\\files\For Bob`}) {
		t.Errorf("Bob reaches %v", got)
	}
	// A file in a shared library cannot break the inheritance; a folder can.
	file := f.upload(anna.ID, f.company.ID, "one.txt", "x")
	if err := f.db.SetInherit(ctx, file.ID, admin, false); !errors.Is(err, db.ErrPerFolder) {
		t.Errorf("breaking a single file: %v", err)
	}
	// Taking Bob's role away takes it off the share.
	if err := f.db.SetGrant(ctx, inner.ID, db.Actor{ID: anna.ID}, db.ToPerson, bob.ID, db.RoleNone); err != nil {
		t.Fatal(err)
	}
	f.sync()
	if _, ok := f.h.acl[filepath.Join(teamDir, "For Bob")][b]; ok {
		t.Error("Bob keeps For Bob on the share")
	}
	for _, s := range f.h.state.Shares {
		if s.Name == "For Bob" {
			t.Error("the share of its own stays without anyone in it")
		}
	}
}

// A library whose share is switched off leaves the disk; the store keeps everything.
func TestUnsharedLeavesTheDisk(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	team, _ := f.db.CreateFileCollection(ctx, anna, "Team")
	if err := f.db.SetRootSamba(ctx, team.ID, anna.ID, true); err != nil {
		t.Fatal(err)
	}
	file := f.upload(anna.ID, team.ID, "t.txt", "team")
	f.sync()
	if _, err := os.Stat(filepath.Join(f.dir, team.ID, "t.txt")); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetRootSamba(ctx, team.ID, anna.ID, false); err != nil {
		t.Fatal(err)
	}
	f.sync()
	if _, err := os.Stat(filepath.Join(f.dir, team.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Error("the unshared library stays on disk")
	}
	if f.current(file.ID) != "team" {
		t.Error("the file is gone from the store")
	}
	// Only its managers switch it: Bob edits it, and may not.
	if err := f.db.SetGrant(ctx, team.ID, db.Actor{ID: anna.ID}, db.ToPerson, bob.ID, db.RoleEdit); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetRootSamba(ctx, team.ID, bob.ID, true); err == nil {
		t.Error("Bob switched a share he does not manage")
	}
}

// A name that is no file name never becomes a path: the web refuses a folder named ".." or with a line break, and one
// made before names were checked stays off the disk with what lies in it, and never reaches smb.conf.
func TestBadNamesStayOffTheDisk(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	for _, title := range []string{"..", "a\n[evil]"} {
		if _, err := f.db.CreateFolder(ctx, anna, title, f.company.ID); !errors.Is(err, db.ErrFileName) {
			t.Errorf("a folder named %q: %v", title, err)
		}
		o, err := f.db.CreateFolder(ctx, anna, "placeholder", f.company.ID)
		if err != nil {
			t.Fatal(err)
		}
		f.upload(anna.ID, o.ID, "inside.txt", "x")
		// One made before: written straight into the database.
		conn, err := pgx.Connect(context.Background(), f.url)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(context.Background(), "UPDATE objects SET title = $2 WHERE id = $1", o.ID, title); err != nil {
			t.Fatal(err)
		}
		_ = conn.Close(context.Background())
	}
	f.sync()
	if entries, _ := os.ReadDir(f.path("")); len(entries) != 0 {
		t.Errorf("on disk: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "inside.txt")); err == nil {
		t.Error("a folder named .. put its file above the library")
	}
	for _, s := range f.h.state.Shares {
		if strings.ContainsAny(s.Path+s.Comment, "\n") {
			t.Errorf("a share with a line break: %+v", s)
		}
	}
}

// A tenant without a companion shares nothing, and the working copy of Main holds only Main.
func TestOtherTenantsShareNothing(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	if _, err := f.db.CreateTenant(ctx, "acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	acme := tenant.With(context.Background(), "acme")
	if err := f.db.SeeAccount(acme, anna); err != nil {
		t.Fatal(err)
	}
	if err := f.db.EnsureRoots(acme, anna, true, "admins"); err != nil {
		t.Fatal(err)
	}
	roots, err := f.db.SharedRoots(acme)
	if err != nil || len(roots) != 0 {
		t.Fatalf("acme shares %+v %v", roots, err)
	}
	if err := f.m.Sync(context.Background(), "acme"); !errors.Is(err, db.ErrNoSamba) {
		t.Errorf("a pass for acme: %v", err)
	}
	f.sync()
	entries, _ := os.ReadDir(filepath.Dir(f.dir))
	if len(entries) != 1 || entries[0].Name() != tenant.Main {
		t.Errorf("the share directory holds %v", entries)
	}
}
