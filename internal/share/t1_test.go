// SPDX-License-Identifier: MIT

//go:build t1

package share

// T-1, the part of LibreOffice (scripts/t1-libreoffice.sh): LibreOffice headless saves over a real Samba share while
// smbd's audit records every operation. The test checks what the working copy makes of each save, measures whether a
// right on a single file survives the save and whether a file without read access stays hidden, and with
// UNDERDOC_T1_FIXTURES set writes the recording as a fixture (testdata/t1) that TestRecordedSaves replays.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Ollornog/UnderDoc/internal/samba"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// lo runs LibreOffice as an account against the share Company of main.
func (f *sambaFixture) lo(t *testing.T, password string, args ...string) {
	t.Helper()
	cmd := exec.Command(os.Getenv("UNDERDOC_T1_LO"), append([]string{"anna", password}, args...)...) // #nosec G204 -- a test
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("LibreOffice %v: %v\n%s", args, err, out)
	}
}

func inodeOf(t *testing.T, path string) uint64 {
	t.Helper()
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		t.Fatal(err)
	}
	return st.Ino
}

func TestT1LibreOffice(t *testing.T) {
	if os.Getenv("UNDERDOC_T1_LO") == "" {
		t.Fatal("UNDERDOC_T1_LO is not set: run scripts/t1-libreoffice.sh")
	}
	f := setupSamba(t)
	ctx := inMain()
	helper := f.m.tenants[tenant.Main].helper
	password := f.password["main/"+anna.ID]
	server := os.Getenv("UNDERDOC_T1_SERVER") + "/Company"
	dir := filepath.Join(f.dir, f.company.ID)
	for _, name := range []string{"budget.xlsx", "budget.ods", "letter.docx", "letter.odt"} {
		t.Run(name, func(t *testing.T) {
			f.lo(t, password, "--make", server, name)
			f.sync(tenant.Main)
			file := f.fileNamed(f.company.ID, name)
			full := filepath.Join(dir, name)
			// A right on the single file: Bob, who reads the folder, is taken off the file's own ACL.
			a, b := 20000, 20001
			if _, err := helper.Do(ctx, samba.Request{Op: samba.OpACL, ACL: []samba.PathACL{{Path: full, Access: map[int]samba.Access{a: samba.Write}}}}); err != nil {
				t.Fatal(err)
			}
			listBefore, _ := f.smb(tenant.Main, bob, "", "Company", "ls", "")
			before := inodeOf(t, full)
			start := time.Now()
			f.lo(t, password, server, name)
			time.Sleep(time.Second) // the last audit lines of the save
			r, err := helper.Do(ctx, samba.Request{Op: samba.OpWho})
			if err != nil {
				t.Fatal(err)
			}
			f.sync(tenant.Main)
			versions, err := f.db.FileVersions(ctx, file.ID, anna.ID)
			if err != nil || len(versions) != 2 || versions[0].Author != anna.ID {
				t.Fatalf("versions after the save of LibreOffice: %+v %v", versions, err)
			}
			if readID(full) != file.ID {
				t.Error("the saved file does not carry its ID")
			}
			if trash, _ := f.db.Trash(ctx, anna.ID); len(trash) != 0 {
				t.Errorf("a save put something into the trash: %+v", trash)
			}
			if list, _ := f.db.FilesIn(ctx, f.company.ID, anna.ID); len(list) != 1+indexOf(name) {
				t.Errorf("files after the save: %d, a temporary file became an object", len(list))
			}
			after := inodeOf(t, full)
			acl := map[int]int{}
			buf := make([]byte, 4096)
			if n, err := unix.Getxattr(full, samba.XattrAccess, buf); err == nil {
				acl = samba.ParseACL(buf[:n])
			}
			_, bobBack := acl[b]
			listAfter, _ := f.smb(tenant.Main, bob, "", "Company", "ls", "")
			rec := recording{
				Client: os.Getenv("UNDERDOC_T1_VERSIONS"), File: name, Measured: time.Now().UTC().Format("2006-01-02"),
				NewInode:          before != after,
				FileRightSurvived: !bobBack,
				HiddenWithoutRead: !strings.Contains(listBefore, name),
				ShownAfterTheSave: strings.Contains(listAfter, name),
			}
			for _, e := range r.Who {
				if e.At.Before(start) || e.User != "anna" {
					continue
				}
				p, ok := strings.CutPrefix(e.Path, dir+"/")
				if !ok || !changing[e.Op] {
					continue
				}
				step := step{Op: e.Op, Path: p}
				if e.To != "" {
					step.To = strings.TrimPrefix(e.To, dir+"/")
				}
				if len(rec.Steps) > 0 && rec.Steps[len(rec.Steps)-1] == step {
					continue
				}
				rec.Steps = append(rec.Steps, step)
			}
			t.Logf("%s: new inode %v, right on the file survived %v, hidden without read %v, shown to Bob after the save %v, %d steps",
				name, rec.NewInode, rec.FileRightSurvived, rec.HiddenWithoutRead, rec.ShownAfterTheSave, len(rec.Steps))
			if out := os.Getenv("UNDERDOC_T1_FIXTURES"); out != "" {
				b, _ := json.MarshalIndent(rec, "", "  ")
				if err := os.WriteFile(filepath.Join(out, "libreoffice-"+strings.ReplaceAll(name, ".", "-")+".json"), append(b, '\n'), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// changing are the operations of the audit that can change what lies in a folder; reads, stats and the rest are left
// out of a recording.
var changing = map[string]bool{"create_file": true, "openat": true, "pwrite_send": true, "pwrite": true, "ftruncate": true,
	"fallocate": true, "renameat": true, "unlinkat": true, "mkdirat": true, "linkat": true, "symlinkat": true}

func indexOf(name string) int {
	for i, n := range []string{"budget.xlsx", "budget.ods", "letter.docx", "letter.odt"} {
		if n == name {
			return i
		}
	}
	return 0
}
