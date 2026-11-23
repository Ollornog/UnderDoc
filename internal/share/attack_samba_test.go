// SPDX-License-Identifier: MIT

//go:build samba

package share

// Attack round on the Samba share (ADR-30), against real companions that scripts/check-samba.sh starts. Each test is
// red while the weakness it names is open and turns green with a correct fix; none of them only checks a form.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// bobEditsCompany gives Bob edit on the folder of the tenant and makes two folders in it, Inbox and Archive.
func (f *sambaFixture) bobEditsCompany() (inbox, archive db.Object) {
	f.t.Helper()
	ctx := inMain()
	admin := db.Actor{ID: anna.ID, Admin: true}
	if err := f.db.SetGrant(ctx, f.company.ID, admin, db.ToPerson, bob.ID, db.RoleEdit); err != nil {
		f.t.Fatal(err)
	}
	var err error
	if inbox, err = f.db.CreateFolder(ctx, anna, "Inbox", f.company.ID); err != nil {
		f.t.Fatal(err)
	}
	if archive, err = f.db.CreateFolder(ctx, anna, "Archive", f.company.ID); err != nil {
		f.t.Fatal(err)
	}
	f.sync(tenant.Main)
	return inbox, archive
}

// An editor may not move a file into a folder of their own, where they would come to share it and manage its rights as
// the owner above (ADR-36; ADR-29, 2026-11-19, asked manage for every move). Samba lets the rename through, since Bob
// writes in both folders; over SMB the move is taken as whoever the audit names. Once Bob pushes his rename out of the
// audit's ring with harmless listings, the pass falls back to the owner of the file and takes the move as Anna, who may.
func TestSambaAttackAuditFloodMovesAsOwner(t *testing.T) {
	f := setupSamba(t)
	ctx := inMain()
	inbox, _ := f.bobEditsCompany()
	mine, err := f.db.CreateFolder(ctx, bob, "Mine", f.company.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.put(anna, "Company", "Inbox/contract.txt", "signed")
	f.sync(tenant.Main)
	file := f.fileNamed(inbox.ID, "contract.txt")
	f.sync(tenant.Main) // the helper hands the new file over to UnderDoc's UID

	// Without the flood the audit names Bob, the database refuses, and the file goes back: the baseline.
	if out, err := f.smb(tenant.Main, bob, "", "Company", `rename Inbox\contract.txt Mine\contract.txt`, ""); err != nil || strings.Contains(out, "NT_STATUS") {
		t.Fatalf("Bob's rename on the share: %v %s", err, out)
	}
	f.sync(tenant.Main)
	if o, err := f.db.Object(ctx, file.ID, anna.ID); err != nil || o.Parent != inbox.ID {
		t.Fatalf("baseline: an editor's move into a folder of his own was taken: %+v %v", o, err)
	}

	// Now the same move, followed by enough listings to push it out of the ring of 4096 events.
	commands := `rename Inbox\contract.txt Mine\contract.txt` + strings.Repeat("; ls", 5000)
	if out, err := f.smb(tenant.Main, bob, "", "Company", commands, ""); err != nil {
		t.Fatalf("Bob's rename and listings: %v %s", err, lastLines(out))
	}
	f.sync(tenant.Main)
	o, err := f.db.Object(ctx, file.ID, anna.ID)
	if err != nil {
		t.Fatal(err)
	}
	if o.Parent == mine.ID {
		t.Errorf("an editor moved Anna's file into his own folder over SMB: the move was taken as its owner once the audit forgot Bob")
	}
}

// A hard link over SMB2 (FileLinkInformation) gives a second path the same inode, and with it the same ID in the
// extended attribute. The pass must not lose the original object over it: it stays alive, at its path, with its ID.
func TestSambaAttackHardlinkStealsID(t *testing.T) {
	f := setupSamba(t)
	ctx := inMain()
	f.bobEditsCompany()
	f.put(anna, "Company", "budget.txt", "v1")
	f.sync(tenant.Main)
	file := f.fileNamed(f.company.ID, "budget.txt")
	f.sync(tenant.Main)
	if out, err := f.smb(tenant.Main, bob, "", "Company", "hardlink budget.txt copy.txt", ""); err != nil || strings.Contains(out, "NT_STATUS") {
		t.Fatalf("a hard link over SMB: %v %s", err, out)
	}
	for range 3 {
		f.sync(tenant.Main)
	}
	if _, err := f.db.Object(ctx, file.ID, anna.ID); errors.Is(err, db.ErrNotFound) {
		trash, _ := f.db.Trash(ctx, anna.ID)
		t.Fatalf("a hard link made by Bob put Anna's file into the trash: %+v", trash)
	} else if err != nil {
		t.Fatal(err)
	}
	if got := readID(filepath.Join(f.dir, f.company.ID, "budget.txt")); got != file.ID {
		t.Errorf("budget.txt carries the ID %q, not its own %q", got, file.ID)
	}
	// A new version of budget.txt in the web reaches the share.
	f.uploadTo(f.company.ID, "budget.txt", "v2 from the web")
	f.sync(tenant.Main)
	if out, _ := f.smb(tenant.Main, anna, "", "Company", "get budget.txt /dev/stdout", ""); !strings.Contains(out, "v2 from the web") {
		t.Errorf("a new version of budget.txt in the web no longer reaches the share: %s", lastLines(out))
	}
}

// Guard, green on 2f1b7ab: a new app password replaces the old one "and closes its sessions" (ADR-30), so a session
// opened with the old password does not go on writing after the change. Seen on disk, since smbclient buffers what it
// prints into a pipe.
func TestSambaGuardPasswordChangeEndsSession(t *testing.T) {
	f := setupSamba(t)
	f.bobEditsCompany()
	cmd := exec.Command(f.clients[tenant.Main], "//localhost/Company", "-U", "bob%"+f.password["main/"+bob.ID]) // #nosec G204
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	onDisk := func(name string) bool {
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(filepath.Join(f.dir, f.company.ID, name)); err == nil {
				return true
			}
			time.Sleep(100 * time.Millisecond)
		}
		return false
	}
	_, _ = stdin.Write([]byte("put /etc/hostname before-change.txt\n"))
	if !onDisk("before-change.txt") {
		t.Fatal("the session with the old password does not write to begin with")
	}
	// Bob's app password changes (a new one in the profile, or after the old one leaked).
	f.appPassword(inMain(), bob)
	f.sync(tenant.Main)
	_, _ = stdin.Write([]byte("put /etc/hostname after-change.txt\n"))
	if onDisk("after-change.txt") {
		t.Error("the session of the old app password goes on writing after the password changed")
	}
}

// A file moved into a folder keeps the ACL of the folder it came from: the helper sets the ACLs of the files of a
// folder only when the ACL of that folder changes. Bob reads the folder of the tenant and has passage through HR (for
// his share HR/Public); a file Anna moves from Company into HR stays readable to him at HR\salaries.txt, also with
// what she writes into it there afterwards.
func TestSambaAttackStaleACLAfterMove(t *testing.T) {
	f := setupSamba(t)
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
	public, err := f.db.CreateFolder(ctx, anna, "Public", hr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetGrant(ctx, public.ID, admin, db.ToPerson, bob.ID, db.RoleRead); err != nil {
		t.Fatal(err)
	}
	f.put(anna, "Company", "salaries.txt", "draft")
	f.sync(tenant.Main)
	f.sync(tenant.Main)
	file := f.fileNamed(f.company.ID, "salaries.txt")
	if _, err := f.db.Move(ctx, file.ID, anna.ID, hr.ID, ""); err != nil {
		t.Fatal(err)
	}
	f.sync(tenant.Main)
	f.sync(tenant.Main)
	if _, err := f.db.Object(ctx, file.ID, bob.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("in the web Bob should no longer see the moved file: %v", err)
	}
	// Anna writes the real numbers into it over SMB, in place.
	f.put(anna, "Company", `HR\salaries.txt`, "final numbers")
	if out, _ := f.smb(tenant.Main, bob, "", "Company", `get HR\salaries.txt /dev/stdout`, ""); strings.Contains(out, "final numbers") {
		t.Errorf("Bob reads HR\\salaries.txt over SMB, which the web hides from him: %s", lastLines(out))
	}
}

// A file whose inheritance was broken before its library was shared keeps its narrower rights in the web, but the share
// gives it the ACL of its folder: Bob reads over SMB what the web hides from him.
func TestSambaAttackBrokenFileSharedLater(t *testing.T) {
	f := setupSamba(t)
	ctx := inMain()
	admin := db.Actor{ID: anna.ID, Admin: true}
	team, err := f.db.CreateFileCollection(ctx, anna, "Team")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetGrant(ctx, team.ID, admin, db.ToPerson, bob.ID, db.RoleRead); err != nil {
		t.Fatal(err)
	}
	file := f.uploadTo(team.ID, "review.txt", "Bob's review: not good")
	if err := f.db.SetInherit(ctx, file.ID, admin, false); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetGrant(ctx, file.ID, admin, db.ToPerson, bob.ID, db.RoleNone); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Object(ctx, file.ID, bob.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("Bob sees the file in the web: %v", err)
	}
	if err := f.db.SetRootSamba(ctx, team.ID, anna.ID, true); err != nil {
		if errors.Is(err, db.ErrPerFolder) {
			return // refusing the share is a correct answer
		}
		t.Fatal(err)
	}
	f.sync(tenant.Main)
	f.sync(tenant.Main)
	if out, _ := f.smb(tenant.Main, bob, "", "Team", "get review.txt /dev/stdout", ""); strings.Contains(out, "not good") {
		t.Errorf("Bob reads over SMB a file the web hides from him: %s", lastLines(out))
	}
}

// uploadTo puts a file into a folder through the web's way of uploading, as Anna.
func (f *sambaFixture) uploadTo(parent, name, content string) db.Object {
	f.t.Helper()
	ctx := inMain()
	sha, size, err := f.store.Put(tenant.Main, strings.NewReader(content), 1<<20)
	if err != nil {
		f.t.Fatal(err)
	}
	u, err := f.db.StartUpload(ctx, anna.ID, db.UploadTarget{Parent: parent}, name, size)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := f.db.Received(ctx, u.ID, anna.ID, 0, size); err != nil {
		f.t.Fatal(err)
	}
	done, err := f.db.FinishUpload(ctx, u.ID, anna.ID, sha, size, "text/plain", func() error { return nil })
	if err != nil {
		f.t.Fatal(err)
	}
	return done.File
}

func lastLines(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 5 {
		lines = lines[len(lines)-5:]
	}
	return strings.Join(lines, "\n")
}

// The same through a move: a file that broke its inheritance in a collection that is not shared is moved into the
// folder of the tenant, which is. The web keeps it from Bob; the share gives it the ACL of Company, which Bob reads.
func TestSambaAttackBrokenFileMovedIn(t *testing.T) {
	f := setupSamba(t)
	ctx := inMain()
	admin := db.Actor{ID: anna.ID, Admin: true}
	drafts, err := f.db.CreateFileCollection(ctx, anna, "Drafts")
	if err != nil {
		t.Fatal(err)
	}
	file := f.uploadTo(drafts.ID, "layoffs.txt", "the list")
	if err := f.db.SetInherit(ctx, file.ID, admin, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Move(ctx, file.ID, anna.ID, f.company.ID, ""); err != nil {
		if errors.Is(err, db.ErrPerFolder) {
			return // refusing the move is a correct answer
		}
		t.Fatal(err)
	}
	if _, err := f.db.Object(ctx, file.ID, bob.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("Bob sees the file in the web: %v", err)
	}
	f.sync(tenant.Main)
	f.sync(tenant.Main)
	if out, _ := f.smb(tenant.Main, bob, "", "Company", "get layoffs.txt /dev/stdout", ""); strings.Contains(out, "the list") {
		t.Errorf("Bob reads over SMB a file the web hides from him: %s", lastLines(out))
	}
}

// KNOWN LIMIT (ADR-30, docs/samba.md): passage (--x) through the folders above a share of one's own lets the kernel stat
// any name in them. Bob, who may not see anything in HR, can tell a name that exists there from one that does not by the
// answer of the share (NT_STATUS_ACCESS_DENIED against NT_STATUS_OBJECT_NAME_NOT_FOUND): a guessed name is confirmed.
// What stays closed, and what this test holds: listing HR fails and shows no name, and reading a name that exists fails.
// The guess is the known limit; if it ever closes, the last check fails and ADR-30 and docs/samba.md must change.
func TestSambaPassageKnownLimitGuessedNames(t *testing.T) {
	f := setupSamba(t)
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
	public, err := f.db.CreateFolder(ctx, anna, "Public", hr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetGrant(ctx, public.ID, admin, db.ToPerson, bob.ID, db.RoleRead); err != nil {
		t.Fatal(err)
	}
	f.sync(tenant.Main)
	f.put(anna, "Company", `HR\dismissal-carl.txt`, "x")
	f.sync(tenant.Main)
	f.sync(tenant.Main)
	exists, _ := f.smb(tenant.Main, bob, "", "Company", `get HR\dismissal-carl.txt /dev/null`, "")
	missing, _ := f.smb(tenant.Main, bob, "", "Company", `get HR\dismissal-dave.txt /dev/null`, "")
	status := func(out string) string {
		for _, w := range strings.Fields(out) {
			if strings.HasPrefix(w, "NT_STATUS_") {
				return w
			}
		}
		return "no error"
	}
	// Closed: the listing of the passage folder fails and names nothing.
	list, _ := f.smb(tenant.Main, bob, "", "Company", `ls HR\*`, "")
	if status(list) == "no error" || strings.Contains(list, "dismissal") {
		t.Errorf("Bob lists the passage folder HR: %s", lastLines(list))
	}
	// Closed: an existing name cannot be read.
	if status(exists) == "no error" {
		t.Errorf("Bob reads a file in HR: %s", lastLines(exists))
	}
	// Known limit: the answers differ, so a guessed name shows whether it exists (never its content).
	if status(exists) == status(missing) {
		t.Errorf("the guess of a name in HR is closed (%s for both): move the known limit out of ADR-30 and docs/samba.md", status(exists))
	}
}
