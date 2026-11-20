// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Ollornog/UnderDoc/internal/samba"
)

// What smbstatus --json says about open files and processes, as far as the helper reads it.
type smbStatus struct {
	Sessions  map[string]smbSession `json:"sessions"`
	OpenFiles map[string]smbFile    `json:"open_files"`
}

type smbSession struct {
	ServerID struct {
		PID string `json:"pid"`
	} `json:"server_id"`
	Username string `json:"username"`
}

type smbFile struct {
	ServicePath string             `json:"service_path"`
	Filename    string             `json:"filename"`
	Opens       map[string]smbOpen `json:"opens"`
}

type smbOpen struct {
	UID        int    `json:"uid"`
	OpenedAt   string `json:"opened_at"`
	AccessMask struct {
		Hex string `json:"hex"`
	} `json:"access_mask"`
}

// sessionPIDs returns the processes that serve the named users.
func sessionPIDs(raw []byte, users []string) []int {
	var s smbStatus
	if json.Unmarshal(raw, &s) != nil {
		return nil
	}
	want := map[string]bool{}
	for _, u := range users {
		want[u] = true
	}
	var pids []int
	for _, session := range s.Sessions {
		if !want[session.Username] {
			continue
		}
		if pid, err := strconv.Atoi(session.ServerID.PID); err == nil && pid > 1 {
			pids = append(pids, pid)
		}
	}
	return pids
}

// The bits of an access mask that change a file: write data, append, write attributes and extended attributes, delete.
const writeMask = 0x2 | 0x4 | 0x10 | 0x100 | 0x10000

// parseOpen reads the open files of smbstatus, below the tenant's directory, limited to paths when given.
func parseOpen(raw []byte, root string, paths []string, names map[int]string) []samba.Open {
	var s smbStatus
	if json.Unmarshal(raw, &s) != nil {
		return nil
	}
	want := map[string]bool{}
	for _, p := range paths {
		want[p] = true
	}
	var out []samba.Open
	for _, f := range s.OpenFiles {
		full := f.Filename
		if !strings.HasPrefix(full, "/") {
			full = path.Join(f.ServicePath, f.Filename)
		}
		full = path.Clean(full)
		if !samba.Below(root, full) || (len(want) > 0 && !want[full]) {
			continue
		}
		for _, o := range f.Opens {
			mask, _ := strconv.ParseUint(strings.TrimPrefix(o.AccessMask.Hex, "0x"), 16, 32)
			since, _ := time.Parse(time.RFC3339Nano, o.OpenedAt)
			out = append(out, samba.Open{Path: full, User: names[o.UID], Write: mask&writeMask != 0, Since: since})
		}
	}
	return out
}

func (h *helper) openFiles(paths []string) ([]samba.Open, error) {
	out, err := exec.Command("smbstatus", "--locks", "--json", "--configfile="+h.confPath()).Output() // #nosec G204
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	names := map[int]string{}
	for _, u := range h.state.Users {
		names[u.UID] = u.Name
	}
	h.mu.Unlock()
	return parseOpen(out, h.cfg.root, paths, names), nil
}
