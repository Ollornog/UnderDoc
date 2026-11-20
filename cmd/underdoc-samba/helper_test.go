// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/samba"
)

// The audit line of vfs_full_audit: the user, the operation, and its paths; create_file names its access mask and kind
// before the path; a path outside the tenant's directory is dropped.
func TestParseAudit(t *testing.T) {
	root := "/srv/share/main"
	e, ok := parseAudit("<173>Nov 20 10:00:00 smbd_audit: anna|renameat|ok|/srv/share/main/c/a.tmp|/srv/share/main/c/a.xlsx", root)
	if !ok || e.User != "anna" || e.Op != "renameat" || e.Path != root+"/c/a.tmp" || e.To != root+"/c/a.xlsx" {
		t.Errorf("rename: %+v %v", e, ok)
	}
	e, ok = parseAudit("smbd_audit: bob|create_file|ok|0x12019f|file|open|/srv/share/main/c/b.txt", root)
	if !ok || e.Path != root+"/c/b.txt" || e.To != "" {
		t.Errorf("create_file: %+v %v", e, ok)
	}
	for _, bad := range []string{
		"smbd_audit: anna|unlinkat|fail (NT_STATUS_ACCESS_DENIED)|/srv/share/main/x",
		"smbd_audit: anna|unlinkat|ok|/srv/share/acme/x",
		"smbd_audit: anna|unlinkat|ok|/srv/share/main/../acme/x",
		"something else",
	} {
		if e, ok := parseAudit(bad, root); ok {
			t.Errorf("%q gave %+v", bad, e)
		}
	}
}

// The open files of smbstatus: by the UID of the user, writing when the access mask writes, only below the tenant.
func TestParseOpen(t *testing.T) {
	raw := []byte(`{"open_files": {
		"/srv/share/main/c/a.txt": {"service_path": "/srv/share/main/c", "filename": "a.txt",
			"opens": {"1/2": {"uid": 20001, "access_mask": {"hex": "0x00000003"}, "opened_at": "2026-11-20T04:18:09.681843+01:00"}}},
		"/srv/share/main/c/b.txt": {"service_path": "/srv/share/main/c", "filename": "b.txt",
			"opens": {"1/3": {"uid": 20002, "access_mask": {"hex": "0x00120089"}, "opened_at": "2026-11-20T04:18:09+01:00"}}},
		"/srv/share/acme/x.txt": {"service_path": "/srv/share/acme", "filename": "x.txt",
			"opens": {"1/4": {"uid": 20001, "access_mask": {"hex": "0x2"}}}}}}`)
	open := parseOpen(raw, "/srv/share/main", nil, map[int]string{20001: "anna", 20002: "bob"})
	slices.SortFunc(open, func(a, b samba.Open) int {
		if a.Path < b.Path {
			return -1
		}
		return 1
	})
	if len(open) != 2 || open[0].User != "anna" || !open[0].Write || open[1].User != "bob" || open[1].Write {
		t.Errorf("open: %+v", open)
	}
	if got := parseOpen(raw, "/srv/share/main", []string{"/srv/share/main/c/b.txt"}, nil); len(got) != 1 {
		t.Errorf("limited to one path: %+v", got)
	}
}

func TestSessionPIDs(t *testing.T) {
	raw := []byte(`{"sessions": {"1": {"server_id": {"pid": "117"}, "username": "anna"}, "2": {"server_id": {"pid": "118"}, "username": "bob"}}}`)
	if got := sessionPIDs(raw, []string{"bob"}); !slices.Equal(got, []int{118}) {
		t.Errorf("pids: %v", got)
	}
}

func TestNetbiosName(t *testing.T) {
	for name, want := range map[string]bool{"FILES": true, "ud-main": true, "": false, "a_b": false, "sixteen-chars-xx": false} {
		if regexpNetbios(name) != want {
			t.Errorf("%q: %v", name, !want)
		}
	}
}

// A listing or a read opens with create_file and changes nothing: it stays out of the ring, so a flood of listings
// pushes no change out and a reader is never named as the one who changed a path. What asks to write, delete or make
// something stays in, as do linkat with both its paths.
func TestAuditKeepsOnlyChanges(t *testing.T) {
	root := "/srv/share/main"
	for _, line := range []string{
		"smbd_audit: bob|create_file|ok|0x100081|dir|open|/srv/share/main/c",
		"smbd_audit: bob|create_file|ok|0x120089|file|open|/srv/share/main/c/a.txt",
	} {
		if e, ok := parseAudit(line, root); ok {
			t.Errorf("a read went into the ring: %+v", e)
		}
	}
	for _, line := range []string{
		"smbd_audit: bob|create_file|ok|0x12019f|file|open|/srv/share/main/c/a.txt",
		"smbd_audit: bob|create_file|ok|0x110080|file|open|/srv/share/main/c/a.txt",
		"smbd_audit: bob|create_file|ok|0x120089|file|create|/srv/share/main/c/new.txt",
		"smbd_audit: bob|create_file|ok|0x120089|file|overwrite_if|/srv/share/main/c/a.txt",
		"smbd_audit: bob|create_file|ok|nonsense|file|open|/srv/share/main/c/a.txt",
	} {
		if _, ok := parseAudit(line, root); !ok {
			t.Errorf("a change stayed out of the ring: %s", line)
		}
	}
	e, ok := parseAudit("smbd_audit: bob|linkat|ok|/srv/share/main/c/a.txt|/srv/share/main/c/b.txt", root)
	if !ok || e.Path != root+"/c/a.txt" || e.To != root+"/c/b.txt" {
		t.Errorf("linkat: %+v %v", e, ok)
	}
}

// The ring numbers its events: First and Last name the oldest and the newest it still keeps, so a caller that saw
// Last before sees a gap when events were pushed out.
func TestRingNumbersItsEvents(t *testing.T) {
	r := newRing(3)
	if _, first, last := r.find(nil); first != 1 || last != 0 {
		t.Errorf("empty: %d %d", first, last)
	}
	for i := range 5 {
		r.add(samba.Event{User: "bob", Op: "renameat", Path: "/x/" + string(rune('a'+i))})
	}
	got, first, last := r.find(nil)
	if first != 3 || last != 5 || len(got) != 3 || got[0].Path != "/x/c" {
		t.Errorf("after 5 events in a ring of 3: %d %d %+v", first, last, got)
	}
}

// A comment of a share is a name from UnderDoc: '%' in it would be a variable of smb.conf (%I is the client's address),
// so it becomes a space, as a backslash and a line break do.
func TestCommentHasNoVariables(t *testing.T) {
	h := &helper{cfg: config{run: t.TempDir(), owner: 1000, ownerGID: 1000}}
	state := samba.State{Users: []samba.User{{Name: "anna", UID: 20000, NTHash: samba.NTHash("x")}},
		Shares: []samba.Share{{Name: "Plans", Path: "/srv/share/main/x", Comment: "100% of %I", Users: []string{"anna"}}}}
	if err := h.writeState(state); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(h.cfg.run, "shares.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "comment = 100  of  I") {
		t.Errorf("shares.conf: %s", raw)
	}
}

// The room UnderDoc gives reaches dfree command: a directory below a library answers the room of that library in blocks
// of 1024 bytes, never more than the disk has; a library left out of the next list answers the disk again; the
// directory of the room can be passed but not listed (ADR-30, 2026-11-20).
func TestTheRoomAnswersDfree(t *testing.T) {
	run := t.TempDir()
	t.Setenv("UNDERDOC_SAMBA_RUN", run)
	library := t.TempDir()
	below := filepath.Join(library, "Reports")
	if err := os.Mkdir(below, 0o700); err != nil {
		t.Fatal(err)
	}
	answer := func(dir string) (total, free, block uint64) {
		var out strings.Builder
		dfree(&out, dir)
		if _, err := fmt.Sscan(out.String(), &total, &free, &block); err != nil {
			t.Fatalf("dfree answered %q: %v", out.String(), err)
		}
		return
	}
	diskTotal, diskFree, _ := answer(below)
	if diskTotal == 0 || diskFree == 0 {
		t.Fatalf("without a room, the disk: %d %d", diskTotal, diskFree)
	}
	h := &helper{}
	if err := h.applyRoom([]samba.Room{{Path: library, Free: 3 << 20, Total: 10 << 20}}); err != nil {
		t.Fatal(err)
	}
	if total, free, block := answer(below); total != 10<<10 || free != 3<<10 || block != 1024 {
		t.Errorf("below the library: %d %d %d", total, free, block)
	}
	// More room than the disk has: the disk.
	if err := h.applyRoom([]samba.Room{{Path: library, Free: 1 << 60, Total: 1 << 60}}); err != nil {
		t.Fatal(err)
	}
	if _, free, _ := answer(below); free > diskFree+(1<<10) {
		t.Errorf("more free than the disk: %d > %d", free, diskFree)
	}
	if fi, err := os.Stat(roomDir()); err != nil || fi.Mode().Perm() != 0o711 {
		t.Errorf("the directory of the room: %v %v", fi.Mode(), err)
	}
	if err := h.applyRoom(nil); err != nil {
		t.Fatal(err)
	}
	if total, _, _ := answer(below); total != diskTotal {
		t.Errorf("left out, the library answers the disk: %d, want %d", total, diskTotal)
	}
	if left, _ := os.ReadDir(roomDir()); len(left) != 0 {
		t.Errorf("files of the room left: %v", left)
	}
}
