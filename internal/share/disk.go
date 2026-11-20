// SPDX-License-Identifier: MIT

package share

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// XattrID is the extended attribute that carries the ID of an object on its file or folder (ADR-30).
const XattrID = "user.underdoc.id"

// tempPrefix starts the names of the temporary files UnderDoc writes before it renames them into place; smbd hides them.
const tempPrefix = ".~underdoc-"

// entry is a file or folder of the working copy as a scan saw it. Path is relative to the directory of its library,
// with '/' and without a leading one; the library itself is "".
type entry struct {
	path    string
	dir     bool
	inode   uint64
	size    int64
	mtimeNS int64
	uid     int
	nlink   uint64
	id      string
}

// ignored tells whether a name never becomes an object: temporary files of Office, LibreOffice, smbd, the systems and
// UnderDoc itself (ADR-30, backed by the recordings of T-1). The patterns of files hold for files only: a folder named
// like a temporary file is an ordinary folder, else it would keep a whole tree out of the database, the quota and the
// versions. Of folders only the temporary names of smbd and UnderDoc are left out.
func ignored(name string, dir bool) bool {
	if strings.HasPrefix(name, tempPrefix) || strings.HasPrefix(name, ".::TMPNAME:") {
		return true
	}
	if dir {
		return false
	}
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(name, "~$"), strings.HasPrefix(name, ".~lock.") && strings.HasSuffix(name, "#"),
		strings.HasSuffix(lower, ".tmp"), strings.HasPrefix(name, "._"),
		lower == "thumbs.db", lower == "desktop.ini", name == ".DS_Store":
		return true
	}
	return false
}

// readID reads the ID an entry carries, "" for none or one that is not an ID of an object.
func readID(full string) string {
	buf := make([]byte, 64)
	n, err := unix.Lgetxattr(full, XattrID, buf)
	if err != nil || n <= 0 {
		return ""
	}
	id := string(buf[:n])
	if !validID(id) {
		return ""
	}
	return id
}

// validID matches the IDs of objects (ADR-18).
func validID(id string) bool {
	if len(id) != 10 {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z') || r == 'i' || r == 'l' || r == 'o' || r == 'u' {
			return false
		}
	}
	return true
}

// writeID sets the ID on an entry.
func writeID(full, id string) error { return unix.Lsetxattr(full, XattrID, []byte(id), 0) }

// scan walks the directory of a library: every folder and regular file below it with its ID, without following links.
// Ignored names are left out, with everything below them.
func scan(root string) (map[string]*entry, error) {
	out := map[string]*entry{}
	var walk func(rel string) error
	walk = func(rel string) error {
		full := filepath.Join(root, filepath.FromSlash(rel))
		var st unix.Stat_t
		if err := unix.Lstat(full, &st); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		kind := st.Mode & unix.S_IFMT
		if kind != unix.S_IFDIR && kind != unix.S_IFREG {
			return nil
		}
		e := &entry{path: rel, dir: kind == unix.S_IFDIR, inode: st.Ino, size: st.Size,
			mtimeNS: st.Mtim.Nano(), uid: int(st.Uid), nlink: uint64(st.Nlink), id: readID(full)} // #nosec G115
		out[rel] = e
		if !e.dir {
			return nil
		}
		names, err := os.ReadDir(full)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
				return nil
			}
			return err
		}
		for _, d := range names {
			if ignored(d.Name(), d.IsDir()) {
				continue
			}
			if err := walk(path.Join(rel, d.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	return out, walk("")
}

// stat returns the entry of one path, nil when it is gone.
func stat(root, rel string) *entry {
	full := filepath.Join(root, filepath.FromSlash(rel))
	var st unix.Stat_t
	if err := unix.Lstat(full, &st); err != nil {
		return nil
	}
	kind := st.Mode & unix.S_IFMT
	if kind != unix.S_IFDIR && kind != unix.S_IFREG {
		return nil
	}
	return &entry{path: rel, dir: kind == unix.S_IFDIR, inode: st.Ino, size: st.Size, mtimeNS: st.Mtim.Nano(), uid: int(st.Uid),
		nlink: uint64(st.Nlink), id: readID(full)} // #nosec G115
}

// hashFile returns the SHA-256 and size of a file, opened without following a link.
func hashFile(full string) (string, int64, error) {
	f, err := os.OpenFile(full, os.O_RDONLY|syscall.O_NOFOLLOW, 0) // #nosec G304 -- a path of the working copy
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), n, err
}

// copyOver replaces a file by a copy of itself: a new inode with the same content and no ID, renamed over the old name.
// It ends a hard link at this name; the other names keep the old inode.
func copyOver(full string) error {
	f, err := os.OpenFile(full, os.O_RDONLY|syscall.O_NOFOLLOW, 0) // #nosec G304 -- a path of the working copy
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var suffix [8]byte
	_, _ = rand.Read(suffix[:])
	tmp := filepath.Join(filepath.Dir(full), tempPrefix+hex.EncodeToString(suffix[:]))
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o660) // #nosec G304
	if err != nil {
		return err
	}
	_, err = io.Copy(out, f)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, full)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

// writeAtomic writes content to a path through a temporary file in the same directory, which carries the ID before it
// is renamed into place, so neither a reader nor the watcher ever sees half a file or a file without its ID.
func writeAtomic(full, id string, content io.Reader) error {
	var suffix [8]byte
	_, _ = rand.Read(suffix[:])
	tmp := filepath.Join(filepath.Dir(full), tempPrefix+hex.EncodeToString(suffix[:]))
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o660) // #nosec G304
	if err != nil {
		return err
	}
	_, err = io.Copy(f, content)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = writeID(tmp, id)
	}
	if err == nil {
		err = os.Rename(tmp, full)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}
