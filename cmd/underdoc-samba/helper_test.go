// SPDX-License-Identifier: MIT

package main

import (
	"slices"
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
