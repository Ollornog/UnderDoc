// SPDX-License-Identifier: MIT

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/Ollornog/UnderDoc/internal/samba"
)

// The room of the libraries (ADR-30, 2026-11-20). UnderDoc counts quotas per library and tenant, which no file system
// knows; smbd asks `dfree command` how much room a directory has and tells the client. The helper keeps one file per
// library that a limit binds, named by a hash of its path in a directory that can be passed but not listed, so whoever
// runs the command (smbd, as the connected user) reads the room of a library only by knowing its path.

// roomDir is where the files of the room lie.
func roomDir() string { return env("UNDERDOC_SAMBA_RUN", "/run/underdoc-samba/state") + "/room" }

// roomFile is the file of a library.
func roomFile(dir, library string) string {
	sum := sha256.Sum256([]byte(library))
	return filepath.Join(dir, hex.EncodeToString(sum[:16]))
}

// applyRoom writes the room of each library of the list and removes the files of every library not in it.
func (h *helper) applyRoom(list []samba.Room) error {
	dir := roomDir()
	if err := os.MkdirAll(dir, 0o711); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o711); err != nil { // #nosec G302 -- passed by name, never listed
		return err
	}
	keep := map[string]bool{}
	for _, r := range list {
		name := roomFile(dir, r.Path)
		keep[filepath.Base(name)] = true
		if err := writeFile(name, fmt.Appendf(nil, "%d %d\n", r.Total, r.Free), 0o644); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !keep[e.Name()] && !strings.HasPrefix(e.Name(), ".tmp-") {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

// dfree answers smbd's question how much room a directory has, as "total free blocksize" with blocks of 1024 bytes:
// the room of the nearest library above it that a limit binds, never more than the disk has; without one, the disk.
// It never fails: smbd would take a broken answer for a full disk.
func dfree(out io.Writer, dir string) {
	if !filepath.IsAbs(dir) {
		if wd, err := os.Getwd(); err == nil {
			dir = filepath.Join(wd, dir)
		}
	}
	dir = filepath.Clean(dir)
	var st unix.Statfs_t
	total, free := uint64(0), uint64(0)
	if unix.Statfs(dir, &st) == nil {
		total, free = st.Blocks*uint64(st.Bsize), st.Bavail*uint64(st.Bsize) // #nosec G115 -- sizes of a file system
	}
	if t, f, ok := roomOf(roomDir(), dir); ok {
		total, free = t, min(f, free)
		if total < free {
			total = free
		}
	}
	fmt.Fprintf(out, "%d %d 1024\n", total/1024, free/1024)
}

// roomOf reads the room of the nearest library at or above dir.
func roomOf(roomDir, dir string) (total, free uint64, ok bool) {
	for p := dir; ; p = filepath.Dir(p) {
		raw, err := os.ReadFile(roomFile(roomDir, p)) // #nosec G304 -- named by a hash in the helper's own directory
		if err == nil {
			fields := strings.Fields(string(raw))
			if len(fields) == 2 {
				t, err1 := strconv.ParseUint(fields[0], 10, 64)
				f, err2 := strconv.ParseUint(fields[1], 10, 64)
				if err1 == nil && err2 == nil {
					return t, f, true
				}
			}
		}
		if p == "/" || p == "." {
			return 0, 0, false
		}
	}
}
