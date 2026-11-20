// SPDX-License-Identifier: MIT

package share

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/samba"
)

// The room of a library that a quota binds reaches the companion after a pass, and again only when it changes; a
// library without a limit is not in it, so the client sees the disk (ADR-30, 2026-11-20).
func TestTheRoomReachesTheCompanion(t *testing.T) {
	f := setup(t)
	f.sync()
	if len(f.h.room) != 0 || f.h.calls[samba.OpRoom] != 0 {
		t.Fatalf("no limit, no room: %+v (%d requests)", f.h.room, f.h.calls[samba.OpRoom])
	}
	if err := f.db.SetQuota(inMain(), f.company.ID, 1000); err != nil {
		t.Fatal(err)
	}
	f.upload(anna.ID, f.company.ID, "notes.txt", strings.Repeat("x", 300))
	f.sync()
	want := []samba.Room{{Path: filepath.Join(f.dir, f.company.ID), Free: 700, Total: 1000}}
	if len(f.h.room) != 1 || f.h.room[0] != want[0] {
		t.Fatalf("the room of the folder of the tenant: %+v, want %+v", f.h.room, want)
	}
	f.sync()
	if got := f.h.calls[samba.OpRoom]; got != 1 {
		t.Errorf("the same room was given %d times", got)
	}
	// A companion from before does not know the request: the pass goes on.
	f.h.old = true
	if err := f.db.SetQuota(inMain(), f.company.ID, 2000); err != nil {
		t.Fatal(err)
	}
	f.sync()
}

// A save over SMB past the quota stays on disk without a version, and the account that saved it is told: once in the
// log as theirs, and in the web until the file is taken (ADR-30, 2026-11-20).
func TestASaveOverTheQuotaIsTold(t *testing.T) {
	f := setup(t)
	if err := f.db.SetGrant(inMain(), f.company.ID, db.Actor{ID: anna.ID}, db.ToPerson, bob.ID, db.RoleEdit); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetQuota(inMain(), f.company.ID, 100); err != nil {
		t.Fatal(err)
	}
	file := f.upload(anna.ID, f.company.ID, "notes.txt", "one")
	f.sync()
	f.write("notes.txt", strings.Repeat("y", 200))
	f.h.wrote("bob", f.path("notes.txt"))
	f.sync()
	if v := f.versions(file.ID); len(v) != 1 {
		t.Fatalf("the save past the quota became a version: %+v", v)
	}
	if got := f.read("notes.txt"); got != strings.Repeat("y", 200) {
		t.Fatalf("the save turned away was written over: %q", got)
	}
	got := f.m.Refused(inMain(), bob.ID)
	if len(got) != 1 || got[0].Path != "notes.txt" || got[0].Why != "the quota is full" || got[0].Library == "" {
		t.Fatalf("bob is not told: %+v", got)
	}
	if other := f.m.Refused(inMain(), anna.ID); len(other) != 0 {
		t.Errorf("anna is told of bob's save: %+v", other)
	}
	entries, err := f.db.LogEntries(inMain(), db.Actor{ID: bob.ID}, db.LogFilter{Actor: bob.ID, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	said := 0
	for _, e := range entries {
		if e.Action == LogShare && strings.Contains(e.Detail, "notes.txt over Samba was not taken: the quota is full") {
			said++
		}
	}
	if said != 1 {
		t.Errorf("bob's part of the log says it %d times: %+v", said, entries)
	}
	// Another pass says it in the web still, and not again in the log.
	f.sync()
	if got := f.read("notes.txt"); got != strings.Repeat("y", 200) {
		t.Errorf("the save turned away did not stay on disk: %q", got)
	}
	if len(f.m.Refused(inMain(), bob.ID)) != 1 || countRefusals(t, f) != 1 {
		t.Errorf("a second pass: %+v, %d in the log", f.m.Refused(inMain(), bob.ID), countRefusals(t, f))
	}
	// With room again, the next save is taken and nothing is told any more.
	if err := f.db.SetQuota(inMain(), f.company.ID, 1000); err != nil {
		t.Fatal(err)
	}
	f.write("notes.txt", strings.Repeat("z", 200))
	f.h.wrote("bob", f.path("notes.txt"))
	f.sync()
	if v := f.versions(file.ID); len(v) != 2 || v[0].Author != bob.ID {
		t.Fatalf("with room, the save is a version: %+v", v)
	}
	if got := f.m.Refused(inMain(), bob.ID); len(got) != 0 {
		t.Errorf("still told: %+v", got)
	}
}

func countRefusals(t *testing.T, f *fixture) int {
	t.Helper()
	entries, err := f.db.LogEntries(inMain(), db.Actor{ID: anna.ID, Admin: true}, db.LogFilter{Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if e.Action == LogShare && strings.Contains(e.Detail, "over Samba was not taken") {
			n++
		}
	}
	return n
}
