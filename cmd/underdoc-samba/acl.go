// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/Ollornog/UnderDoc/internal/samba"
)

// applyACL sets the ACL of each directory, its default ACL, and the ACL of the files directly in it; every path and every
// file in it is handed to UnderDoc's UID, so the bits of an owner never give a Samba user more than its folder. Paths are
// opened below the tenant's directory without following any link (openat2 with RESOLVE_BENEATH and RESOLVE_NO_SYMLINKS),
// so a link made in the share cannot lead the helper elsewhere.
func (h *helper) applyACL(list []samba.PathACL) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	root, err := unix.Open(h.cfg.root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(root) }()
	var failed []string
	for _, a := range list {
		if err := h.aclOne(root, a); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", a.Path, err))
		}
	}
	if len(failed) > 0 {
		return errors.New(strings.Join(failed, "; "))
	}
	return nil
}

func (h *helper) aclOne(root int, a samba.PathACL) error {
	rel := strings.TrimPrefix(strings.TrimPrefix(a.Path, h.cfg.root), "/")
	if rel == "" {
		rel = "."
	}
	how := &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS}
	fd, err := unix.Openat2(root, rel, how)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(fd) }()
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return h.aclFile(fd, a.Access)
	}
	if err := unix.Fchown(fd, h.cfg.owner, h.cfg.ownerGID); err != nil {
		return err
	}
	if err := unix.Fchmod(fd, 0o770); err != nil {
		return err
	}
	if err := unix.Fsetxattr(fd, samba.XattrAccess, samba.ACL(a.Access, true, h.cfg.owner), 0); err != nil {
		return err
	}
	if err := unix.Fsetxattr(fd, samba.XattrDefault, samba.ACL(fileDefault(a.Access), true, h.cfg.owner), 0); err != nil {
		return err
	}
	// The files directly in it; folders below have ACLs of their own.
	names, err := readNames(fd)
	if err != nil {
		return err
	}
	for _, name := range names {
		child, err := unix.Openat2(fd, name, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS})
		if err != nil {
			continue // a link, a socket, or gone meanwhile
		}
		var cs unix.Stat_t
		if unix.Fstat(child, &cs) == nil && cs.Mode&unix.S_IFMT == unix.S_IFREG {
			if err := h.aclFile(child, a.Access); err != nil {
				_ = unix.Close(child)
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		_ = unix.Close(child)
	}
	return nil
}

// fileDefault is the default ACL of a directory: what is made in it inherits the access of the folder, except passage,
// which is for the way to a share below and never for what lies in this folder.
func fileDefault(access map[int]samba.Access) map[int]samba.Access {
	out := map[int]samba.Access{}
	for uid, a := range access {
		if a != samba.Pass {
			out[uid] = a
		}
	}
	return out
}

func (h *helper) aclFile(fd int, access map[int]samba.Access) error {
	if err := unix.Fchown(fd, h.cfg.owner, h.cfg.ownerGID); err != nil {
		return err
	}
	if err := unix.Fchmod(fd, 0o660); err != nil {
		return err
	}
	return unix.Fsetxattr(fd, samba.XattrAccess, samba.ACL(access, false, h.cfg.owner), 0)
}

// readNames lists the names in an open directory.
func readNames(fd int) ([]string, error) {
	dup, err := unix.Openat(fd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = unix.Close(dup) }()
	var names []string
	buf := make([]byte, 64<<10)
	for {
		n, err := unix.Getdents(dup, buf)
		if err != nil {
			return nil, err
		}
		if n <= 0 {
			break
		}
		_, _, names = unix.ParseDirent(buf[:n], -1, names)
	}
	return names, nil
}
