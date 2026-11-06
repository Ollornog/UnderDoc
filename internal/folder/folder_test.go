// SPDX-License-Identifier: MIT

package folder

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/library"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func scan(t *testing.T, s *Source) (*library.Input, string) {
	t.Helper()
	in, state, err := s.Scan("")
	if err != nil {
		t.Fatal(err)
	}
	return in, state
}

func sources(in *library.Input) []string {
	var list []string
	for _, page := range in.Pages {
		list = append(list, page.Source)
	}
	return list
}

func TestScanFindsPagesAndImages(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.md", "a")
	write(t, dir, "sub/b.MD", "b")
	write(t, dir, "sub/c.markdown", "c")
	write(t, dir, "sub/pic.PNG", "p")
	write(t, dir, "other.txt", "no")
	write(t, dir, "config.yaml", "no")
	write(t, dir, ".hidden.md", "no")
	write(t, dir, ".git/HEAD.md", "no")
	write(t, dir, ".obsidian/x.png", "no")
	in, _ := scan(t, New(dir))
	if got := strings.Join(sources(in), ","); got != "a.md,sub/b.MD,sub/c.markdown" {
		t.Errorf("pages = %s", got)
	}
	if !slices.Equal(in.Images, []string{"sub/pic.PNG"}) {
		t.Errorf("images = %v", in.Images)
	}
	if string(in.Pages[0].Content) != "a" || in.Pages[0].Modified.IsZero() {
		t.Errorf("page = %+v", in.Pages[0])
	}
}

func TestSymlinksAreNotFollowed(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	write(t, outside, "secret.md", "secret")
	write(t, outside, "secret.png", "secret")
	write(t, outside, "sub/deep.md", "secret")
	write(t, dir, "real.md", "ok")
	for link, target := range map[string]string{
		"link.md":  filepath.Join(outside, "secret.md"),
		"link.png": filepath.Join(outside, "secret.png"),
		"linkdir":  filepath.Join(outside, "sub"),
		"inner.md": filepath.Join(dir, "real.md"),
	} {
		if err := os.Symlink(target, filepath.Join(dir, link)); err != nil {
			t.Fatal(err)
		}
	}
	s := New(dir)
	in, _ := scan(t, s)
	if got := strings.Join(sources(in), ","); got != "real.md" {
		t.Errorf("pages = %s", got)
	}
	if len(in.Images) != 0 {
		t.Errorf("images = %v", in.Images)
	}
	for _, page := range in.Pages {
		if strings.Contains(string(page.Content), "secret") {
			t.Error("content from outside the folder")
		}
	}
	if len(in.Problems) != 3 {
		t.Errorf("the skipped links should be named: %+v", in.Problems)
	}
	for _, name := range []string{"link.png", "../" + filepath.Base(outside) + "/secret.png", "/etc/passwd"} {
		if file, _, err := s.OpenImage(name); err == nil {
			_ = file.Close()
			t.Errorf("%s could be opened", name)
		}
	}
}

func TestStateDetectsEveryKindOfChange(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "sub/a.md", "a")
	s := New(dir)
	_, state := scan(t, s)

	if in, same, err := s.Scan(state); err != nil || in != nil || same != state {
		t.Fatalf("no change: in=%v state=%v err=%v", in, same != state, err)
	}
	changes := []struct {
		name   string
		change func()
	}{
		{"content with a new size", func() { write(t, dir, "sub/a.md", "longer") }},
		{"same size, new time", func() {
			write(t, dir, "sub/a.md", "LONGER")
			later := time.Now().Add(time.Hour)
			if err := os.Chtimes(filepath.Join(dir, "sub/a.md"), later, later); err != nil {
				t.Fatal(err)
			}
		}},
		{"new page", func() { write(t, dir, "b.md", "b") }},
		{"new image", func() { write(t, dir, "p.png", "p") }},
		{"move", func() {
			if err := os.Rename(filepath.Join(dir, "b.md"), filepath.Join(dir, "sub/b.md")); err != nil {
				t.Fatal(err)
			}
		}},
		{"delete", func() {
			if err := os.Remove(filepath.Join(dir, "sub/b.md")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, c := range changes {
		c.change()
		in, next, err := s.Scan(state)
		if err != nil {
			t.Fatal(err)
		}
		if in == nil || next == state {
			t.Errorf("%s was not noticed", c.name)
		}
		state = next
	}
}

func TestTooLargePagesAreNamed(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "big.md", strings.Repeat("x", MaxPageBytes+1))
	write(t, dir, "ok.md", "ok")
	in, _ := scan(t, New(dir))
	if got := strings.Join(sources(in), ","); got != "ok.md" {
		t.Errorf("pages = %s", got)
	}
	if len(in.Problems) != 1 || in.Problems[0].Source != "big.md" {
		t.Errorf("problems = %+v", in.Problems)
	}
}

func TestMissingFolderIsAnError(t *testing.T) {
	if _, _, err := New(filepath.Join(t.TempDir(), "gone")).Scan(""); err == nil {
		t.Error("a missing folder must fail")
	}
}

func TestReadOnlyFolderWorksAndStaysUntouched(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "sub/a.md", "a")
	write(t, dir, "p.png", "p")
	before := listing(t, dir)
	for _, p := range []string{filepath.Join(dir, "sub/a.md"), filepath.Join(dir, "p.png")} {
		if err := os.Chmod(p, 0o444); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{filepath.Join(dir, "sub"), dir} {
		if err := os.Chmod(p, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(p, 0o755) })
	}
	s := New(dir)
	if in, _ := scan(t, s); len(in.Pages) != 1 {
		t.Errorf("pages = %v", sources(in))
	}
	file, _, err := s.OpenImage("p.png")
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if after := listing(t, dir); !slices.Equal(before, after) {
		t.Errorf("the folder changed:\n%v\n%v", before, after)
	}
}

func listing(t *testing.T, dir string) []string {
	t.Helper()
	var list []string
	if err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		list = append(list, p+" "+info.ModTime().String())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return list
}

func TestOpenImageChecksTheFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "folder.png/x.txt", "x")
	s := New(dir)
	for _, name := range []string{"folder.png", "missing.png", "\xff.png"} {
		if file, _, err := s.OpenImage(name); err == nil {
			_ = file.Close()
			t.Errorf("%q could be opened", name)
		} else if name != "\xff.png" && !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%q: %v", name, err)
		}
	}
}
