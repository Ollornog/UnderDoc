// SPDX-License-Identifier: MIT

// Package folder reads a library from a folder on disk. It only ever reads. Symbolic links are skipped and all access goes
// through os.Root, so nothing outside the folder can be reached.
package folder

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Ollornog/UnderDoc/internal/library"
)

// MaxPageBytes is the largest page that gets read.
const MaxPageBytes = 8 << 20

var (
	pageExtensions  = map[string]bool{".md": true, ".markdown": true}
	imageExtensions = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".avif": true, ".svg": true}
)

// IsImage reports whether a path has the extension of a served image type.
func IsImage(p string) bool { return imageExtensions[strings.ToLower(path.Ext(p))] }

// Source is a folder on disk.
type Source struct {
	dir string
}

// New returns a source for a folder. The first scan shows whether it can be read.
func New(dir string) *Source { return &Source{dir: dir} }

type entry struct {
	path     string
	size     int64
	modified time.Time
}

// Scan implements library.Source. A changed size or modification time anywhere counts as a change.
func (s *Source) Scan(since string) (*library.Input, string, error) {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = root.Close() }()

	var pages, images []entry
	var problems []library.Problem
	hash := sha256.New()
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if p == "." {
				return walkErr
			}
			problems = append(problems, library.Problem{Source: p, Message: "cannot be read: " + walkErr.Error()})
			return nil
		}
		if p == "." {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		isPage, isImage := pageExtensions[strings.ToLower(path.Ext(name))], IsImage(name)
		if !isPage && !isImage {
			return nil
		}
		if !d.Type().IsRegular() {
			problems = append(problems, library.Problem{Source: p, Message: "skipped, only regular files are served"})
			return nil
		}
		info, err := d.Info()
		if err != nil {
			problems = append(problems, library.Problem{Source: p, Message: "cannot be read: " + err.Error()})
			return nil
		}
		found := entry{path: p, size: info.Size(), modified: info.ModTime()}
		fmt.Fprintf(hash, "%q %d %s\n", p, found.size, strconv.FormatInt(found.modified.UnixNano(), 10))
		if isPage {
			pages = append(pages, found)
		} else {
			images = append(images, found)
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	for _, problem := range problems {
		fmt.Fprintf(hash, "%q %q\n", problem.Source, problem.Message)
	}
	state := hex.EncodeToString(hash.Sum(nil))
	if state == since {
		return nil, state, nil
	}

	in := &library.Input{Problems: problems}
	for _, image := range images {
		in.Images = append(in.Images, image.path)
	}
	for _, page := range pages {
		if page.size > MaxPageBytes {
			in.Problems = append(in.Problems, library.Problem{Source: page.path, Message: fmt.Sprintf("skipped, larger than %d MiB", MaxPageBytes>>20)})
			continue
		}
		content, err := readLimited(root, page.path)
		if err != nil {
			// Vanished or replaced between listing and reading. Its new state shows in the next scan.
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			in.Problems = append(in.Problems, library.Problem{Source: page.path, Message: "cannot be read: " + err.Error()})
			continue
		}
		in.Pages = append(in.Pages, library.Page{Source: page.path, Content: content, Modified: page.modified})
	}
	return in, state, nil
}

func readLimited(root *os.Root, name string) ([]byte, error) {
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return io.ReadAll(io.LimitReader(file, MaxPageBytes))
}

// OpenImage implements library.Source.
func (s *Source) OpenImage(imagePath string) (io.ReadSeekCloser, time.Time, error) {
	if !utf8.ValidString(imagePath) {
		return nil, time.Time{}, fs.ErrNotExist
	}
	file, err := os.OpenInRoot(s.dir, imagePath)
	if err != nil {
		return nil, time.Time{}, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, time.Time{}, fs.ErrNotExist
	}
	return file, info.ModTime(), nil
}
