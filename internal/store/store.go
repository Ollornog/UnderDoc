// SPDX-License-Identifier: MIT

// Package store is the content store of ADR-28: every version of a file and every attachment of a page as an immutable
// blob named by the SHA-256 of its content, so equal content is kept once. Each tenant has a directory of its own, so no
// path of one tenant ever reaches the blobs of another (ADR-23).
//
// Layout below the root, with <t> the tenant:
//
//	<t>/blobs/ab/cd/<sha256>   the blobs, read-only once written
//	<t>/uploads/<id>           uploads in progress, removed when finished or expired
//	<t>/derived/<name>         what is made from blobs and can be made again, such as previews
//
// No name a user gives ever becomes a path here: blobs go by their hash, uploads by a random ID, derived files by the
// hash and a fixed suffix. The database knows which blob belongs to what; a backup needs both (ADR-13).
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// DefaultTenant is the tenant Main (package tenant), the one of an instance with one tenant (ADR-23).
const DefaultTenant = "main"

var (
	shaPattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
	uploadPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	tenantPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	// derivedPattern is a hash with a fixed suffix of letters, digits, dots and dashes, such as <sha>-thumb-256.jpg.
	derivedPattern = regexp.MustCompile(`^[0-9a-f]{64}[a-z0-9.-]{0,40}$`)
)

// ErrName is a hash, upload ID or tenant that does not have its form: never a path.
var ErrName = errors.New("store: not a valid name")

// Store is the content store below one directory.
type Store struct {
	root string
}

// Open returns the store below dir, making it if needed. Only the owner may read it.
func Open(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("store: no directory")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, err
	}
	return &Store{root: abs}, nil
}

// Root is the directory of the store.
func (s *Store) Root() string { return s.root }

func (s *Store) tenantDir(tenant string) (string, error) {
	if !tenantPattern.MatchString(tenant) {
		return "", ErrName
	}
	return filepath.Join(s.root, tenant), nil
}

// ValidHash tells whether s is a SHA-256 in lower-case hex.
func ValidHash(s string) bool { return shaPattern.MatchString(s) }

// ValidUpload tells whether s is the form of an upload ID.
func ValidUpload(s string) bool { return uploadPattern.MatchString(s) }

func (s *Store) blobPath(tenant, sha string) (string, error) {
	dir, err := s.tenantDir(tenant)
	if err != nil {
		return "", err
	}
	if !ValidHash(sha) {
		return "", ErrName
	}
	return filepath.Join(dir, "blobs", sha[0:2], sha[2:4], sha), nil
}

// UploadPath is the file that collects the chunks of an upload.
func (s *Store) UploadPath(tenant, id string) (string, error) {
	dir, err := s.tenantDir(tenant)
	if err != nil {
		return "", err
	}
	if !ValidUpload(id) {
		return "", ErrName
	}
	return filepath.Join(dir, "uploads", id), nil
}

// OpenUpload opens the file of an upload for writing, made if it is not there.
func (s *Store) OpenUpload(tenant, id string) (*os.File, error) {
	p, err := s.UploadPath(tenant, id)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return nil, err
	}
	return os.OpenFile(p, os.O_RDWR|os.O_CREATE, 0o600)
}

// RemoveUpload removes the file of an upload; one that is gone already is no error.
func (s *Store) RemoveUpload(tenant, id string) error {
	p, err := s.UploadPath(tenant, id)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// HashFile returns the SHA-256 and size of a file.
func HashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// Adopt puts a finished upload into the store as the blob with the given hash, as a second name of the same file, so the
// upload stays until the caller removes it when everything else has worked. If that blob is there already, nothing is
// done: equal content is kept once. The hash must be the one of the content (HashFile).
func (s *Store) Adopt(tenant, uploadID, sha string) error {
	from, err := s.UploadPath(tenant, uploadID)
	if err != nil {
		return err
	}
	to, err := s.blobPath(tenant, sha)
	if err != nil {
		return err
	}
	if _, err := os.Stat(to); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
		return err
	}
	if err := os.Link(from, to); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	return os.Chmod(to, 0o400)
}

// adopt moves a file of the store's own into place as the blob with the given hash, or drops it if that is there.
func (s *Store) adopt(from, tenant, sha string) error {
	to, err := s.blobPath(tenant, sha)
	if err != nil {
		return err
	}
	if _, err := os.Stat(to); err == nil {
		return os.Remove(from)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
		return err
	}
	if err := os.Chmod(from, 0o400); err != nil {
		return err
	}
	return os.Rename(from, to)
}

// Put writes content from r into the store and returns its hash and size, reading at most limit bytes; more is an error
// and leaves nothing behind.
func (s *Store) Put(tenant string, r io.Reader, limit int64) (string, int64, error) {
	dir, err := s.tenantDir(tenant)
	if err != nil {
		return "", 0, err
	}
	tmpDir := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		return "", 0, err
	}
	tmp, err := os.CreateTemp(tmpDir, "put-")
	if err != nil {
		return "", 0, err
	}
	done := false
	defer func() {
		_ = tmp.Close()
		if !done {
			_ = os.Remove(tmp.Name())
		}
	}()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(r, limit+1))
	if err != nil {
		return "", 0, err
	}
	if n > limit {
		return "", 0, fmt.Errorf("store: more than %d bytes", limit)
	}
	if err := tmp.Close(); err != nil {
		return "", 0, err
	}
	sha := hex.EncodeToString(h.Sum(nil))
	if err := s.adopt(tmp.Name(), tenant, sha); err != nil {
		return "", 0, err
	}
	done = true
	return sha, n, nil
}

// Open opens a blob for reading.
func (s *Store) Open(tenant, sha string) (*os.File, error) {
	p, err := s.blobPath(tenant, sha)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

// Has tells whether a blob is in the store.
func (s *Store) Has(tenant, sha string) bool {
	p, err := s.blobPath(tenant, sha)
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Remove takes a blob and everything derived from it out of the store; one that is gone already is no error.
func (s *Store) Remove(tenant, sha string) error {
	p, err := s.blobPath(tenant, sha)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	dir, _ := s.tenantDir(tenant)
	derived, _ := filepath.Glob(filepath.Join(dir, "derived", sha+"*"))
	for _, d := range derived {
		if err := os.Remove(d); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// DerivedPath is where something made from a blob is kept, such as a preview: name is the hash with a suffix.
func (s *Store) DerivedPath(tenant, name string) (string, error) {
	dir, err := s.tenantDir(tenant)
	if err != nil {
		return "", err
	}
	if !derivedPattern.MatchString(name) {
		return "", ErrName
	}
	return filepath.Join(dir, "derived", name), nil
}

// WriteDerived keeps content made from a blob under name, written whole or not at all.
func (s *Store) WriteDerived(tenant, name string, content []byte) error {
	p, err := s.DerivedPath(tenant, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".tmp-")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// Blobs calls fn with the hash and time of every blob of a tenant, for a check of the store against the database.
func (s *Store) Blobs(tenant string, fn func(sha string, modified time.Time) error) error {
	dir, err := s.tenantDir(tenant)
	if err != nil {
		return err
	}
	root := filepath.Join(dir, "blobs")
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == root {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() || !ValidHash(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return fn(d.Name(), info.ModTime())
	})
	return err
}
