// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/samba"
)

// Guard, green on 2f1b7ab: a peer of the socket whose UID is not UnderDoc's gets no answer at all, also not to ping,
// and an acl request from it changes nothing (SO_PEERCRED). The test plays the foreign peer by naming another UID as
// UnderDoc's.
func TestGuardForeignPeerIsTurnedAway(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "share")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	h := &helper{cfg: config{root: root, owner: os.Getuid() + 1, ownerGID: os.Getgid() + 1, uidMin: 20000, uidMax: 29999},
		log: slog.New(slog.NewTextHandler(io.Discard, nil)), audit: newRing(8), boot: "x"}
	sock := filepath.Join(dir, "helper.sock")
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.AcceptUnix()
			if err != nil {
				return
			}
			go h.serve(c)
		}
	}()
	for _, req := range []samba.Request{{Op: samba.OpPing}, {Op: samba.OpACL, ACL: []samba.PathACL{{Path: root, Access: map[int]samba.Access{20000: samba.Write}}}}} {
		c, err := net.Dial("unix", sock)
		if err != nil {
			t.Fatal(err)
		}
		_ = c.SetDeadline(time.Now().Add(5 * time.Second))
		if err := json.NewEncoder(c).Encode(req); err != nil {
			t.Fatal(err)
		}
		answer, _ := io.ReadAll(c)
		_ = c.Close()
		if len(answer) != 0 {
			t.Errorf("a foreign peer got an answer to %s: %s", req.Op, answer)
		}
	}
	if st, err := os.Stat(root); err != nil || st.Mode().Perm() != 0o700 {
		t.Errorf("the acl of a foreign peer changed the directory: %v %v", st.Mode(), err)
	}
}
