// SPDX-License-Identifier: MIT

//go:build samba || t1

package share

// Tests against real Samba (ADR-30): scripts/check-samba.sh builds the companion and starts one for the tenant main and
// one for acme, each on the directory of its tenant only, and names them in the environment. A client talks SMB to them
// with smbclient. Without the environment these tests fail: a skipped test is not a green one.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/samba"
	"github.com/Ollornog/UnderDoc/internal/store"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
)

// sambaEnv reads a list name=value,name=value of the environment.
func sambaEnv(t *testing.T, name string) map[string]string {
	t.Helper()
	raw := os.Getenv(name)
	if raw == "" {
		t.Fatalf("%s is not set: these tests run against companions that scripts/check-samba.sh starts", name)
	}
	out := map[string]string{}
	for _, part := range strings.Split(raw, ",") {
		k, v, _ := strings.Cut(part, "=")
		out[k] = v
	}
	return out
}

type sambaFixture struct {
	t        *testing.T
	db       *db.DB
	store    *store.Store
	m        *Manager
	clients  map[string]string
	company  db.Root
	dir      string
	password map[string]string // account → app password, made here for the test
}

func setupSamba(t *testing.T) *sambaFixture {
	t.Helper()
	dir := os.Getenv("UNDERDOC_TEST_SAMBA_DIR")
	if dir == "" {
		t.Fatal("UNDERDOC_TEST_SAMBA_DIR is not set: run scripts/check-samba.sh")
	}
	sockets := sambaEnv(t, "UNDERDOC_TEST_SAMBA_SOCKETS")
	clients := sambaEnv(t, "UNDERDOC_TEST_SAMBA_CLIENTS")
	ctx := inMain()
	database, err := db.Open(ctx, pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if _, err := database.CreateTenant(ctx, "acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	for _, u := range []account.User{anna, bob, carl} {
		if err := database.SeeAccount(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	acme := tenant.With(context.Background(), "acme")
	if err := database.SeeAccount(acme, carl); err != nil {
		t.Fatal(err)
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
	helpers := map[string]Helper{}
	for tn, s := range sockets {
		helpers[tn] = samba.Client{Socket: s, Timeout: time.Minute}
	}
	// The directory of each tenant is shared with its companion: what a test before left there goes first.
	for tn := range sockets {
		entries, _ := os.ReadDir(filepath.Join(dir, tn))
		for _, e := range entries {
			_ = os.RemoveAll(filepath.Join(dir, tn, e.Name()))
		}
	}
	m, err := New(database, st, v, Config{Dir: dir, Helpers: helpers, Server: "files"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.EnsureRoots(ctx, anna, true, "admins"); err != nil {
		t.Fatal(err)
	}
	if err := database.EnsureRoots(acme, carl, true, "admins"); err != nil {
		t.Fatal(err)
	}
	f := &sambaFixture{t: t, db: database, store: st, m: m, clients: clients, dir: filepath.Join(dir, tenant.Main), password: map[string]string{}}
	roots, _ := database.FileRoots(ctx, anna.ID)
	for _, r := range roots {
		if r.Role == db.RootTenant {
			f.company = r
		}
	}
	f.appPassword(ctx, anna)
	f.appPassword(ctx, bob)
	f.appPassword(acme, carl)
	f.sync(tenant.Main)
	f.sync("acme")
	return f
}

// appPassword makes an app password the way the web does: random, its NT hash sealed.
func (f *sambaFixture) appPassword(ctx context.Context, u account.User) {
	f.t.Helper()
	raw := make([]byte, 15)
	_, _ = rand.Read(raw)
	password := base64.RawURLEncoding.EncodeToString(raw)
	tn, _ := tenant.From(ctx)
	sealed, err := f.m.vault.Seal(tn, db.SambaSecretName(u.ID), vault.Secret(samba.NTHash(password)))
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.db.SetSambaPassword(ctx, u.ID, u.Login, sealed, 20000, 29999); err != nil {
		f.t.Fatal(err)
	}
	f.password[tn+"/"+u.ID] = password
}

func (f *sambaFixture) sync(tn string) {
	f.t.Helper()
	if err := f.m.Sync(context.Background(), tn); err != nil {
		f.t.Fatal(err)
	}
}

// smb runs smbclient as an account against a share of a tenant's companion, with stdin, and returns what it said.
func (f *sambaFixture) smb(tn string, u account.User, password, share, commands, stdin string) (string, error) {
	f.t.Helper()
	if password == "" {
		password = f.password[tn+"/"+u.ID]
	}
	cmd := exec.Command(f.clients[tn], "//localhost/"+share, "-U", u.Login+"%"+password, "-c", commands) // #nosec G204 -- a test
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// put writes content to a path of a share as an account.
func (f *sambaFixture) put(u account.User, share, name, content string) {
	f.t.Helper()
	if out, err := f.smb(tenant.Main, u, "", share, "put /dev/stdin "+name, content); err != nil || strings.Contains(out, "NT_STATUS") {
		f.t.Fatalf("put %s as %s: %v %s", name, u.Login, err, out)
	}
}

func (f *sambaFixture) current(fileID string) string {
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

func (f *sambaFixture) fileNamed(parent, name string) db.FileInfo {
	f.t.Helper()
	list, err := f.db.FilesIn(inMain(), parent, anna.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, x := range list {
		if x.Title == name {
			return x
		}
	}
	f.t.Fatalf("no file %s in %s: %+v", name, parent, list)
	return db.FileInfo{}
}

// A file saved over SMB becomes a file of whoever saved it; a save through a temporary file and a rename, as Office
// does it, becomes a version of the same object; deleting moves it to the trash.
func TestSambaSaveRenameDelete(t *testing.T) {
	f := setupSamba(t)
	f.put(anna, "Company", "report.txt", "first")
	f.sync(tenant.Main)
	file := f.fileNamed(f.company.ID, "report.txt")
	if file.Owner != anna.ID || f.current(file.ID) != "first" {
		t.Fatalf("the new file: %+v", file)
	}
	if readID(filepath.Join(f.dir, f.company.ID, "report.txt")) != file.ID {
		t.Error("the file on the share carries no ID")
	}
	// Excel's way: a temporary file, the original to a backup name, the temporary one to the original, the backup gone.
	out, err := f.smb(tenant.Main, anna, "", "Company",
		"put /dev/stdin ~WRD0001.tmp; rename report.txt ~WRL0002.tmp; rename ~WRD0001.tmp report.txt; del ~WRL0002.tmp", "second, saved")
	if err != nil || strings.Contains(out, "NT_STATUS") {
		t.Fatalf("the save: %v %s", err, out)
	}
	f.sync(tenant.Main)
	versions, err := f.db.FileVersions(inMain(), file.ID, anna.ID)
	if err != nil || len(versions) != 2 || versions[0].Author != anna.ID {
		t.Fatalf("versions after the save: %+v %v", versions, err)
	}
	if f.current(file.ID) != "second, saved" {
		t.Error("the save is not the current version")
	}
	if readID(filepath.Join(f.dir, f.company.ID, "report.txt")) != file.ID {
		t.Error("the saved file does not carry the ID again")
	}
	// The ACL of the folder holds on the saved file: Bob reads it.
	if out, err := f.smb(tenant.Main, bob, "", "Company", "get report.txt /dev/stdout", ""); err != nil || !strings.Contains(out, "second, saved") {
		t.Errorf("Bob reads the saved file: %v %s", err, out)
	}
	// A rename over SMB renames the object.
	if out, err := f.smb(tenant.Main, anna, "", "Company", "rename report.txt final.txt", ""); err != nil || strings.Contains(out, "NT_STATUS") {
		t.Fatalf("rename: %v %s", err, out)
	}
	f.sync(tenant.Main)
	if o, err := f.db.Object(inMain(), file.ID, anna.ID); err != nil || o.Title != "final.txt" {
		t.Fatalf("after the rename: %+v %v", o, err)
	}
	// Deleting over SMB moves it to the trash, by whoever deleted it.
	if out, err := f.smb(tenant.Main, anna, "", "Company", "del final.txt", ""); err != nil || strings.Contains(out, "NT_STATUS") {
		t.Fatalf("delete: %v %s", err, out)
	}
	f.sync(tenant.Main)
	trash, _ := f.db.Trash(inMain(), anna.ID)
	if len(trash) != 1 || trash[0].ID != file.ID || trash[0].DeletedBy != anna.Name {
		t.Fatalf("trash: %+v", trash)
	}
	// A change in the web reaches the share: restored, it is there again.
	if _, err := f.db.Restore(inMain(), file.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	f.sync(tenant.Main)
	if out, err := f.smb(tenant.Main, anna, "", "Company", "get final.txt /dev/stdout", ""); err != nil || !strings.Contains(out, "second, saved") {
		t.Errorf("the restored file over SMB: %v %s", err, out)
	}
}

// Rights per folder hold on the share: Bob reads the folder of the tenant and writes nothing there, a folder that shuts
// him out does not even show, a folder in which he may edit is a share of his own; a wrong app password, and an account
// without one, are turned away.
func TestSambaRightsPerFolder(t *testing.T) {
	f := setupSamba(t)
	ctx := inMain()
	admin := db.Actor{ID: anna.ID, Admin: true}
	board, err := f.db.CreateFolder(ctx, anna, "Board", f.company.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetInherit(ctx, board.ID, admin, false); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetGrant(ctx, board.ID, admin, db.ToTenant, "", db.RoleNone); err != nil {
		t.Fatal(err)
	}
	f.sync(tenant.Main)
	f.put(anna, "Company", "Board/minutes.txt", "secret")
	f.put(anna, "Company", "open.txt", "for all")
	team, _ := f.db.CreateFileCollection(ctx, anna, "Team")
	if err := f.db.SetRootSamba(ctx, team.ID, anna.ID, true); err != nil {
		t.Fatal(err)
	}
	inner, _ := f.db.CreateFolder(ctx, anna, "Shared", team.ID)
	if err := f.db.SetGrant(ctx, inner.ID, db.Actor{ID: anna.ID}, db.ToPerson, bob.ID, db.RoleEdit); err != nil {
		t.Fatal(err)
	}
	f.sync(tenant.Main)
	f.sync(tenant.Main)

	list, err := f.smb(tenant.Main, bob, "", "Company", "ls", "")
	if err != nil || !strings.Contains(list, "open.txt") || strings.Contains(list, "Board") {
		t.Errorf("Bob's listing of Company: %v %s", err, list)
	}
	if out, _ := f.smb(tenant.Main, bob, "", "Company", "get Board/minutes.txt /dev/stdout", ""); strings.Contains(out, "secret") {
		t.Error("Bob reads what lies in Board")
	}
	if out, _ := f.smb(tenant.Main, bob, "", "Company", "put /dev/stdin bobs.txt", "nope"); !strings.Contains(out, "NT_STATUS_ACCESS_DENIED") {
		t.Errorf("Bob writes into Company, where he reads only: %s", out)
	}
	enum, err := exec.Command(f.clients[tenant.Main], "-L", "//localhost", "-U", "bob%"+f.password["main/"+bob.ID]).CombinedOutput() // #nosec G204
	if err != nil || strings.Contains(string(enum), "Team") || !strings.Contains(string(enum), "Shared") {
		t.Errorf("the shares Bob sees: %v %s", err, enum)
	}
	if out, err := f.smb(tenant.Main, bob, "", "Shared", "put /dev/stdin from-bob.txt", "bob's"); err != nil || strings.Contains(out, "NT_STATUS") {
		t.Errorf("Bob writes in his share Shared: %v %s", err, out)
	}
	if out, _ := f.smb(tenant.Main, bob, "", "Team", "ls", ""); !strings.Contains(out, "NT_STATUS") {
		t.Errorf("Bob reaches Team: %s", out)
	}
	f.sync(tenant.Main)
	got := f.fileNamed(inner.ID, "from-bob.txt")
	if got.Owner != bob.ID {
		t.Errorf("the file from Bob belongs to %s", got.Owner)
	}
	// A wrong app password and an account without one are turned away.
	if out, _ := f.smb(tenant.Main, anna, "wrong-password", "Company", "ls", ""); !strings.Contains(out, "NT_STATUS_LOGON_FAILURE") {
		t.Errorf("a wrong app password: %s", out)
	}
	if out, _ := f.smb(tenant.Main, carl, "anything", "Company", "ls", ""); !strings.Contains(out, "NT_STATUS_LOGON_FAILURE") {
		t.Errorf("Carl without an app password in main: %s", out)
	}
	// Taking the app password away ends it at once.
	if err := f.db.RemoveSambaAccount(ctx, db.Actor{ID: bob.ID}, bob.ID); err != nil {
		t.Fatal(err)
	}
	f.sync(tenant.Main)
	if out, _ := f.smb(tenant.Main, bob, "", "Company", "ls", ""); !strings.Contains(out, "NT_STATUS_LOGON_FAILURE") {
		t.Errorf("Bob after his app password was removed: %s", out)
	}
}

// The tenant acme has a companion of its own on its own directory: Carl of acme signs in there and sees acme's files,
// and is a stranger to the companion of main, which never sees the directory of acme.
func TestSambaTenantsApart(t *testing.T) {
	f := setupSamba(t)
	acme := tenant.With(context.Background(), "acme")
	roots, err := f.db.SharedRoots(acme)
	if err != nil || len(roots) == 0 {
		t.Fatalf("acme shares %+v %v", roots, err)
	}
	if out, err := f.smb("acme", carl, "", "Company", "put /dev/stdin acme.txt", "acme only"); err != nil || strings.Contains(out, "NT_STATUS") {
		t.Fatalf("Carl in acme: %v %s", err, out)
	}
	f.sync("acme")
	if out, _ := f.smb(tenant.Main, carl, f.password["acme/"+carl.ID], "Company", "ls", ""); !strings.Contains(out, "NT_STATUS_LOGON_FAILURE") {
		t.Errorf("Carl of acme signs in to main: %s", out)
	}
	if out, _ := f.smb("acme", anna, f.password["main/"+anna.ID], "Company", "ls", ""); !strings.Contains(out, "NT_STATUS_LOGON_FAILURE") {
		t.Errorf("Anna of main signs in to acme: %s", out)
	}
	list, err := f.smb(tenant.Main, anna, "", "Company", "ls", "")
	if err != nil || strings.Contains(list, "acme.txt") {
		t.Errorf("main shows acme's file: %v %s", err, list)
	}
	if _, err := os.Stat(filepath.Join(f.dir, "..", "acme")); err != nil {
		t.Fatal(err)
	}
}

// While a file is open over SMB, the web does not write it and names who has it open.
func TestSambaOpenHoldsTheWebBack(t *testing.T) {
	f := setupSamba(t)
	f.put(anna, "Company", "busy.txt", "one")
	f.sync(tenant.Main)
	file := f.fileNamed(f.company.ID, "busy.txt")
	// smbclient keeps the file open while it waits for its next command.
	cmd := exec.Command(f.clients[tenant.Main], "//localhost/Company", "-U", "anna%"+f.password["main/"+anna.ID]) // #nosec G204
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()
	if _, err := stdin.Write([]byte("open busy.txt\n")); err != nil {
		t.Fatal(err)
	}
	var open *db.OpenError
	deadline := time.Now().Add(20 * time.Second)
	for {
		_, err = f.db.Rename(inMain(), file.ID, anna.ID, "moved.txt")
		if errors.As(err, &open) || time.Now().After(deadline) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !errors.As(err, &open) || !slices.Equal(open.Users, []string{"anna"}) {
		t.Fatalf("rename of a file open over SMB: %v", err)
	}
	place, ok := f.m.PlaceOf(inMain(), file.ID, anna.ID)
	if !ok || place.Path != `\\files\Company\busy.txt` || !slices.Contains(place.OpenBy, "anna") {
		t.Errorf("place: %+v %v", place, ok)
	}
}

// The quota of a library reaches the client through dfree command (ADR-30, 2026-11-20): smbclient's du shows the room
// the quota leaves, also in a folder below; a save past it stays on the share without a version, and its writer is told.
func TestSambaQuotaReachesTheClient(t *testing.T) {
	f := setupSamba(t)
	ctx := inMain()
	if err := f.db.SetQuota(ctx, f.company.ID, 1<<20); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.CreateFolder(ctx, anna, "Reports", f.company.ID); err != nil {
		t.Fatal(err)
	}
	f.put(anna, "Company", "first.txt", strings.Repeat("a", 100<<10))
	f.sync(tenant.Main)
	du := regexp.MustCompile(`(\d+) blocks of size (\d+)\. (\d+) blocks available`)
	room := func(dir string) (total, free int64) {
		t.Helper()
		out, err := f.smb(tenant.Main, anna, "", "Company", "cd "+dir+"; du", "")
		m := du.FindStringSubmatch(out)
		if err != nil || m == nil {
			t.Fatalf("du in %s: %v %s", dir, err, out)
		}
		n, _ := strconv.ParseInt(m[1], 10, 64)
		size, _ := strconv.ParseInt(m[2], 10, 64)
		left, _ := strconv.ParseInt(m[3], 10, 64)
		return n * size, left * size
	}
	for _, dir := range []string{"/", "Reports"} {
		total, free := room(dir)
		if total != 1<<20 || free > 1<<20-100<<10 || free < 1<<20-100<<10-8<<10 {
			t.Errorf("du in %s: %d free of %d, want about %d of %d", dir, free, total, 1<<20-100<<10, 1<<20)
		}
	}
	// Past the quota: the file stays, without becoming an object, and anna is told.
	f.put(anna, "Company", "big.txt", strings.Repeat("b", 2<<20))
	f.sync(tenant.Main)
	if _, err := os.Stat(filepath.Join(f.dir, f.company.ID, "big.txt")); err != nil {
		t.Errorf("the save past the quota left the share: %v", err)
	}
	if got := f.m.Refused(ctx, anna.ID); len(got) != 1 || got[0].Path != "big.txt" || got[0].Why != "the quota is full" {
		t.Errorf("anna is not told: %+v", got)
	}
	if _, free := room("/"); free > 1<<20-100<<10 {
		t.Errorf("du after the refused save: %d free", free)
	}
}
