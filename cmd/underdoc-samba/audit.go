// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"net"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Ollornog/UnderDoc/internal/samba"
)

// The audit of smbd (vfs_full_audit) goes to syslog. The helper is that syslog: it binds the socket smbd writes to and
// keeps the last operations in a ring, so UnderDoc can ask who changed a path. Nothing is written to disk.

// ring keeps the newest events, numbered from 1 on since the start of the helper, so UnderDoc tells when events were
// pushed out before it asked.
type ring struct {
	mu     sync.Mutex
	events []samba.Event
	next   int
	full   bool
	seq    uint64 // the number of the newest event
}

func newRing(n int) *ring { return &ring{events: make([]samba.Event, n)} }

func (r *ring) add(e samba.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	r.events[r.next] = e
	r.next = (r.next + 1) % len(r.events)
	if r.next == 0 {
		r.full = true
	}
}

// find returns the events about the paths, oldest first, all events without paths; and the numbers of the oldest and
// the newest event the ring still keeps.
func (r *ring) find(paths []string) (out []samba.Event, first, last uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	want := map[string]bool{}
	for _, p := range paths {
		want[p] = true
	}
	n := r.next
	start := 0
	if r.full {
		n = len(r.events)
		start = r.next
	}
	for i := 0; i < n; i++ {
		e := r.events[(start+i)%len(r.events)]
		if len(want) == 0 || want[e.Path] || (e.To != "" && want[e.To]) {
			out = append(out, e)
		}
	}
	return out, r.seq - uint64(n) + 1, r.seq // #nosec G115 -- n is a length
}

// listenAudit binds the syslog socket and reads smbd's audit lines into the ring.
func (h *helper) listenAudit(ctx context.Context) error {
	_ = os.Remove(h.cfg.audit)
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: h.cfg.audit, Net: "unixgram"})
	if err != nil {
		return err
	}
	// smbd writes its audit after it switched to the UID of the user it serves, so every process of the container may
	// write here; only smbd runs in it besides the helper.
	if err := os.Chmod(h.cfg.audit, 0o666); err != nil { // #nosec G302 -- see above
		return err
	}
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	go func() {
		buf := make([]byte, 16<<10)
		for {
			n, _, err := conn.ReadFromUnix(buf)
			if err != nil {
				return
			}
			if e, ok := parseAudit(string(buf[:n]), h.cfg.root); ok {
				e.At = time.Now()
				h.audit.add(e)
				if h.cfg.verbose {
					h.log.Info("audit", "user", e.User, "op", e.Op, "path", e.Path, "to", e.To)
				}
			}
		}
	}()
	return nil
}

// parseAudit reads one syslog message of vfs_full_audit with the prefix %u:
//
//	<173>Nov 20 10:00:00 smbd_audit: anna|renameat|ok|/share/dir/a.tmp|/share/dir/a.xlsx
//
// Samba names absolute paths; a path outside the tenant's directory is dropped. So is a create_file that opens what is
// there without asking to change it (a listing, a read): it changes nothing, and kept it would push the events that do
// out of the ring and name a reader as the last one to touch a path.
func parseAudit(msg, root string) (samba.Event, bool) {
	i := strings.Index(msg, "smbd_audit:")
	if i < 0 {
		return samba.Event{}, false
	}
	fields := strings.Split(strings.TrimSpace(msg[i+len("smbd_audit:"):]), "|")
	if len(fields) < 4 || fields[2] != "ok" {
		return samba.Event{}, false
	}
	if fields[1] == "create_file" && len(fields) >= 6 && fields[5] == "open" && !changing(fields[3]) {
		return samba.Event{}, false
	}
	// The paths are the fields that start with '/': create_file names its access mask and kind first.
	e := samba.Event{User: fields[0], Op: fields[1]}
	for _, f := range fields[3:] {
		if !strings.HasPrefix(f, "/") {
			continue
		}
		if e.Path == "" {
			e.Path = clean(f, root)
			if e.Path == "" {
				return samba.Event{}, false
			}
		} else if e.To == "" {
			e.To = clean(f, root)
		}
	}
	return e, e.User != "" && e.Path != ""
}

// changing tells whether the access mask of create_file (vfs_full_audit: "0x%x") asks for more than reading: writing data,
// attributes or extended attributes, deleting, changing ACL or owner, or the generic and maximal rights. An unknown mask
// counts as changing.
func changing(mask string) bool {
	m, err := strconv.ParseUint(strings.TrimPrefix(mask, "0x"), 16, 32)
	if err != nil {
		return true
	}
	const write = 0x2 | 0x4 | 0x10 | 0x40 | 0x100 | 0x10000 | 0x40000 | 0x80000 | 0x2000000 | 0x10000000 | 0x40000000
	return m&write != 0
}

func clean(p, root string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = path.Clean(p)
	if !samba.Below(root, p) {
		return ""
	}
	return p
}
