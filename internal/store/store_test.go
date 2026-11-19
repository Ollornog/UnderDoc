// SPDX-License-Identifier: MIT

package store

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestPutKeepsEqualContentOnce(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	a, n, err := s.Put(DefaultTenant, strings.NewReader("hello"), 100)
	if err != nil || n != 5 || a != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("%s %d %v", a, n, err)
	}
	b, _, err := s.Put(DefaultTenant, strings.NewReader("hello"), 100)
	if err != nil || b != a {
		t.Fatalf("%s %v", b, err)
	}
	var all []string
	if err := s.Blobs(DefaultTenant, func(sha string, _ time.Time) error { all = append(all, sha); return nil }); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(all, []string{a}) {
		t.Fatalf("blobs %v", all)
	}
	f, err := s.Open(DefaultTenant, a)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(f)
	_ = f.Close()
	if string(content) != "hello" {
		t.Fatalf("%q", content)
	}
	// Too much leaves nothing behind.
	if _, _, err := s.Put(DefaultTenant, strings.NewReader("too long"), 3); err == nil {
		t.Fatal("more than the limit was taken")
	}
	if entries, _ := os.ReadDir(filepath.Join(s.Root(), DefaultTenant, "uploads")); len(entries) != 0 {
		t.Fatalf("left behind: %v", entries)
	}
	// Tenants are apart: a blob of one is not in the other.
	if s.Has("other", a) {
		t.Fatal("another tenant has the blob")
	}
}

func TestUploadsAreAdopted(t *testing.T) {
	s, _ := Open(t.TempDir())
	id := strings.Repeat("ab", 16)
	f, err := s.OpenUpload(DefaultTenant, id)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("data")
	_ = f.Close()
	path, _ := s.UploadPath(DefaultTenant, id)
	sha, n, err := HashFile(path)
	if err != nil || n != 4 {
		t.Fatal(n, err)
	}
	for range 2 {
		if err := s.Adopt(DefaultTenant, id, sha); err != nil {
			t.Fatal(err)
		}
	}
	if !s.Has(DefaultTenant, sha) {
		t.Fatal("not adopted")
	}
	if err := s.RemoveUpload(DefaultTenant, id); err != nil {
		t.Fatal(err)
	}
	if !s.Has(DefaultTenant, sha) {
		t.Fatal("the blob went with the upload")
	}
	if err := s.WriteDerived(DefaultTenant, sha+"-thumb-256.jpg", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(DefaultTenant, sha); err != nil {
		t.Fatal(err)
	}
	derived, _ := s.DerivedPath(DefaultTenant, sha+"-thumb-256.jpg")
	if _, err := os.Stat(derived); !errors.Is(err, os.ErrNotExist) || s.Has(DefaultTenant, sha) {
		t.Fatal("the blob or what was made from it stayed")
	}
}

func TestNamesAreNeverPaths(t *testing.T) {
	s, _ := Open(t.TempDir())
	sha := strings.Repeat("a", 64)
	for _, tenant := range []string{"", "..", "../x", "A", "a/b", strings.Repeat("a", 64)} {
		if _, err := s.Open(tenant, sha); !errors.Is(err, ErrName) {
			t.Errorf("tenant %q: %v", tenant, err)
		}
	}
	for _, hash := range []string{"", "../../etc/passwd", strings.Repeat("A", 64), strings.Repeat("a", 63)} {
		if _, err := s.Open(DefaultTenant, hash); !errors.Is(err, ErrName) {
			t.Errorf("hash %q: %v", hash, err)
		}
	}
	for _, id := range []string{"", "..", "../" + strings.Repeat("a", 29), strings.Repeat("g", 32)} {
		if _, err := s.OpenUpload(DefaultTenant, id); !errors.Is(err, ErrName) {
			t.Errorf("upload %q: %v", id, err)
		}
	}
	for _, name := range []string{"../x", sha + "/../../x", sha + "_X"} {
		if err := s.WriteDerived(DefaultTenant, name, nil); !errors.Is(err, ErrName) {
			t.Errorf("derived %q: %v", name, err)
		}
	}
}

func TestRemoveTenant(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tenant := range []string{"acme", "beta"} {
		if _, _, err := s.Put(tenant, strings.NewReader("content of "+tenant), 1<<20); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RemoveTenant("acme"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Root(), "acme")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the directory of acme is still there: %v", err)
	}
	if entries, err := os.ReadDir(filepath.Join(s.Root(), "beta")); err != nil || len(entries) == 0 {
		t.Errorf("beta keeps its files: %v %v", entries, err)
	}
	if err := s.RemoveTenant("acme"); err != nil {
		t.Errorf("a tenant without a directory: %v", err)
	}
	for _, bad := range []string{"", "..", "../beta", "a/b", "Beta"} {
		if err := s.RemoveTenant(bad); !errors.Is(err, ErrName) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}
