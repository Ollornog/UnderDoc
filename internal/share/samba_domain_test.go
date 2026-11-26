// SPDX-License-Identifier: MIT

//go:build samba

package share

// Tests against a real member of a domain (T-42): scripts/check-samba.sh starts a domain controller of Samba with made-up
// accounts (dora, eve, daemon, 4711; Administrator comes with the domain), all with one password made for the run, and a
// companion for the tenant corp that joined it. Without the environment these tests fail.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
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

const corp = "corp"

var (
	dora  = account.User{ID: "2001", RemoteID: "2001", Login: "dora", Name: "Dora", Groups: []string{"admins"}}
	eve   = account.User{ID: "2002", RemoteID: "2002", Login: "eve", Name: "Eve"}
	numbr = account.User{ID: "2003", RemoteID: "4711", Login: "nummer", Name: "Nummer"}
)

type domainFixture struct {
	t        *testing.T
	db       *db.DB
	m        *Manager
	helper   samba.Client
	client   string
	password string
	dir      string
	company  db.Root
}

func inCorp() context.Context { return tenant.With(context.Background(), corp) }

func setupDomain(t *testing.T) *domainFixture {
	t.Helper()
	dir := os.Getenv("UNDERDOC_TEST_SAMBA_DIR")
	client := os.Getenv("UNDERDOC_TEST_SAMBA_DOMAIN_CLIENT")
	secret := os.Getenv("UNDERDOC_TEST_SAMBA_DOMAIN_PASSWORD_FILE")
	sockets := sambaEnv(t, "UNDERDOC_TEST_SAMBA_DOMAIN")
	if dir == "" || client == "" || secret == "" || sockets[corp] == "" {
		t.Fatal("the domain is not set up: run scripts/check-samba.sh")
	}
	raw, err := os.ReadFile(secret) // #nosec G304 -- a file of the test run
	if err != nil {
		t.Fatal(err)
	}
	ctx := inCorp()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if _, err := database.CreateTenant(context.Background(), corp, "Corp"); err != nil {
		t.Fatal(err)
	}
	for _, u := range []account.User{dora, eve, numbr} {
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
	entries, _ := os.ReadDir(filepath.Join(dir, corp))
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(dir, corp, e.Name()))
	}
	h := samba.Client{Socket: sockets[corp], Timeout: time.Minute}
	m, err := New(database, st, v, Config{Dir: dir, Helpers: map[string]Helper{corp: h}, Server: "files",
		Domain: map[string]bool{corp: true}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.EnsureRoots(ctx, dora, true, "admins"); err != nil {
		t.Fatal(err)
	}
	f := &domainFixture{t: t, db: database, m: m, helper: h, client: client, password: strings.TrimSpace(string(raw)), dir: filepath.Join(dir, corp)}
	roots, _ := database.FileRoots(ctx, dora.ID)
	for _, r := range roots {
		if r.Role == db.RootTenant {
			f.company = r
		}
	}
	// Dora is mapped by her login, the account "nummer" by the number of the gate; Eve has a domain account, but no
	// Samba user in UnderDoc.
	f.domainAccount(dora, "dora")
	f.domainAccount(numbr, "4711")
	if err := m.CheckModes(ctx); err != nil {
		t.Fatal(err)
	}
	f.sync()
	return f
}

func (f *domainFixture) domainAccount(u account.User, name string) db.SambaAccount {
	f.t.Helper()
	a, err := f.db.SetSambaDomainAccount(inCorp(), u.ID, name, 20000, 29999)
	if err != nil {
		f.t.Fatal(err)
	}
	return a
}

func (f *domainFixture) sync() {
	f.t.Helper()
	if err := f.m.Sync(context.Background(), corp); err != nil {
		f.t.Fatal(err)
	}
}

// smb signs in as user (DOMAIN\name, or name alone) with password, and runs the commands against a share.
func (f *domainFixture) smb(user, password, share, commands string, extra ...string) string {
	f.t.Helper()
	args := append([]string{"//localhost/" + share, "-U", user + "%" + password, "-c", commands}, extra...)
	cmd := exec.Command(f.client, args...) // #nosec G204 -- a test
	cmd.Stdin = strings.NewReader("written over SMB")
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func signedIn(out string) bool { return !strings.Contains(out, "NT_STATUS_") }

// A domain account with a Samba user signs in with the domain's password, by NTLM and by Kerberos, and what it saves
// becomes a file of its UnderDoc account; the number of the gate maps as well.
func TestSambaDomainSignsIn(t *testing.T) {
	f := setupDomain(t)
	if out := f.smb(`EXAMPLE\dora`, f.password, "Company", "put /dev/stdin plan.txt; ls"); !signedIn(out) || !strings.Contains(out, "plan.txt") {
		t.Fatalf("Dora: %s", out)
	}
	f.sync()
	list, err := f.db.FilesIn(inCorp(), f.company.ID, dora.ID)
	if err != nil || len(list) != 1 || list[0].Owner != dora.ID {
		t.Fatalf("the file of Dora: %+v %v", list, err)
	}
	// Kerberos: a ticket of the domain for the companion's name.
	if out := f.smb("dora", f.password, "Company", "ls", "--use-kerberos=required", "--realm=EXAMPLE.TEST"); !strings.Contains(out, "plan.txt") {
		// smbclient takes the server from the path; Kerberos needs the name the domain knows.
		out = kerberos(f, "Company", "ls")
		if !strings.Contains(out, "plan.txt") {
			t.Errorf("Kerberos: %s", out)
		}
	}
	// Upper case, as Windows may send it, is the same account.
	if out := f.smb(`EXAMPLE\DORA`, f.password, "dora", "ls"); !signedIn(out) {
		t.Errorf("DORA: %s", out)
	}
	if out := f.smb(`EXAMPLE\4711`, f.password, "Company", "ls"); !signedIn(out) {
		t.Errorf("the number of the gate: %s", out)
	}
}

// kerberos runs smbclient with Kerberos against the companion by the name the domain knows (files.example.test).
func kerberos(f *domainFixture, share, commands string) string {
	f.t.Helper()
	out, _ := exec.Command(f.client, "//files.example.test/"+share, "-U", "dora@EXAMPLE.TEST%"+f.password, "--use-kerberos=required", // #nosec G204
		"-c", commands).CombinedOutput()
	return string(out)
}

// The attack round of T-42: whoever the domain knows but UnderDoc does not map is turned away; so are the users of the
// system (daemon) and the administrator of the domain, both with the right password; a wrong password; and a sign-in
// as a local user of the companion, which has none with a domain.
func TestSambaDomainTurnsAway(t *testing.T) {
	f := setupDomain(t)
	for name, user := range map[string]string{
		"a domain account without a Samba user": `EXAMPLE\eve`,
		"a user of the system":                  `EXAMPLE\daemon`,
		"the administrator of the domain":       `EXAMPLE\Administrator`,
		"a local user of the companion":         `FILES\dora`,
	} {
		if out := f.smb(user, f.password, "Company", "ls"); signedIn(out) {
			t.Errorf("%s signed in: %s", name, out)
		}
	}
	if out := f.smb(`EXAMPLE\dora`, f.password+"x", "Company", "ls"); signedIn(out) {
		t.Errorf("a wrong password: %s", out)
	}
	// Nobody named root, even if the domain had one: the companion refuses the name, and the range leaves out UID 0.
	st := samba.State{Mode: samba.ModeDomain, Users: []samba.User{{Name: "root", UID: 20005}}}
	if r, err := f.helper.Do(context.Background(), samba.Request{Op: samba.OpApply, State: &st}); err == nil && r.OK {
		t.Error("the companion took a user named root")
	}
}

// App passwords and a domain are never mixed: the member refuses a state with an app password, also one named like a
// domain account, and keeps the state it had (T-42).
func TestSambaDomainRefusesAppPasswords(t *testing.T) {
	f := setupDomain(t)
	for _, st := range []samba.State{
		{Users: []samba.User{{Name: "dora", UID: 20001, NTHash: samba.NTHash("local")}}},
		{Mode: samba.ModeDomain, Users: []samba.User{{Name: "dora", UID: 20001, NTHash: samba.NTHash("local")}}},
	} {
		r, err := f.helper.Do(context.Background(), samba.Request{Op: samba.OpApply, State: &st})
		if err == nil && r.OK {
			t.Fatalf("the member took %#v", st)
		}
	}
	if out := f.smb(`FILES\dora`, "local", "Company", "ls"); signedIn(out) {
		t.Errorf("the app password signs in: %s", out)
	}
	if out := f.smb(`EXAMPLE\dora`, f.password, "Company", "ls"); !signedIn(out) {
		t.Errorf("Dora after the refused states: %s", out)
	}
}

// A removed Samba user stops at once, and a UID that passes to another account is never reached by the domain account
// that had it before: the companion forgets what it mapped whenever its users change (T-42).
func TestSambaDomainUIDChangesHands(t *testing.T) {
	f := setupDomain(t)
	ctx := inCorp()
	before, err := f.db.SambaAccountOf(ctx, dora.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out := f.smb(`EXAMPLE\dora`, f.password, "Company", "ls"); !signedIn(out) {
		t.Fatalf("Dora: %s", out)
	}
	// Eve tries before she is mapped; that answer must not stick either.
	if out := f.smb(`EXAMPLE\eve`, f.password, "Company", "ls"); signedIn(out) {
		t.Fatalf("Eve before: %s", out)
	}
	if err := f.db.RemoveSambaAccount(ctx, db.Actor{ID: dora.ID}, dora.ID); err != nil {
		t.Fatal(err)
	}
	after := f.domainAccount(eve, "eve")
	if after.UID != before.UID {
		t.Fatalf("Eve has UID %d, Dora had %d: the test needs the UID to pass", after.UID, before.UID)
	}
	f.sync()
	if out := f.smb(`EXAMPLE\dora`, f.password, "Company", "ls"); signedIn(out) {
		t.Errorf("Dora reaches the UID that is Eve's now: %s", out)
	}
	// Eve reaches the share with the UID that was Dora's, and with her own rights, not Dora's: she reads the folder of the
	// tenant, and writing there, which Dora could, is refused.
	if out := f.smb(`EXAMPLE\eve`, f.password, "Company", "ls"); !signedIn(out) {
		t.Fatalf("Eve after: %s", out)
	}
	if out := f.smb(`EXAMPLE\eve`, f.password, "Company", "put /dev/stdin eve.txt"); !strings.Contains(out, "NT_STATUS_ACCESS_DENIED") {
		t.Errorf("Eve writes with the rights that were Dora's: %s", out)
	}
}
