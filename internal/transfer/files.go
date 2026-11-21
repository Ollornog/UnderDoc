// SPDX-License-Identifier: MIT

package transfer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"time"

	"go.yaml.in/yaml/v4"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/store"
)

// The files of a backup (docs/files.md): the view Files as real files, each file in its current version under its
// folders, and a manifest that keeps what files cannot say: IDs, roots, owners, every version, and the attachments of
// pages. Content the real files do not hold, older versions and attachments, lies under BlobsDir by its hash.

// FilesDir is the directory of a full export that holds the files.
const FilesDir = "files"

// FilesManifest is the manifest at the root of FilesDir.
const FilesManifest = ".underdoc-files.yaml"

// BlobsDir holds, below FilesDir, the content that no real file holds, each by its SHA-256.
const BlobsDir = ".blobs"

type filesManifest struct {
	Format      int              `yaml:"format"`
	Roots       []fileNode       `yaml:"roots"`
	Attachments []attachmentMeta `yaml:"attachments,omitempty"`
}

type fileNode struct {
	ID          string        `yaml:"id"`
	Kind        string        `yaml:"kind"`
	Title       string        `yaml:"title"`
	Description string        `yaml:"description,omitempty"`
	Owner       string        `yaml:"owner,omitempty"`
	Access      string        `yaml:"access,omitempty"`
	Private     bool          `yaml:"private,omitempty"`
	Tags        []string      `yaml:"tags,omitempty,flow"`
	Role        string        `yaml:"role,omitempty"`
	Login       string        `yaml:"login,omitempty"`
	Writers     string        `yaml:"writers,omitempty"`
	Quota       *int64        `yaml:"quota,omitempty"`
	Path        string        `yaml:"path,omitempty"`
	Versions    []versionMeta `yaml:"versions,omitempty"`
	Tables      []tableMeta   `yaml:"tables,omitempty"`
	Kids        []fileNode    `yaml:"kids,omitempty"`
}

// tableMeta is a table of a spreadsheet with its ID (ADR-31); where it was last found is looked for anew after import.
type tableMeta struct {
	ID          string   `yaml:"id"`
	Kind        string   `yaml:"kind"`
	Sheet       string   `yaml:"sheet"`
	SheetNumber int      `yaml:"sheet_number,omitempty"`
	Range       string   `yaml:"range,omitempty"`
	Name        string   `yaml:"name,omitempty"`
	Head        bool     `yaml:"head,omitempty"`
	Anchor      []string `yaml:"anchor,omitempty,flow"`
}

type versionMeta struct {
	Number int       `yaml:"number"`
	SHA256 string    `yaml:"sha256"`
	Size   int64     `yaml:"size"`
	Name   string    `yaml:"name"`
	Type   string    `yaml:"type"`
	Author string    `yaml:"author,omitempty"`
	At     time.Time `yaml:"at"`
}

type attachmentMeta struct {
	Page   string    `yaml:"page"`
	ID     string    `yaml:"id"`
	Name   string    `yaml:"name"`
	SHA256 string    `yaml:"sha256"`
	Size   int64     `yaml:"size"`
	Type   string    `yaml:"type"`
	Author string    `yaml:"author,omitempty"`
	At     time.Time `yaml:"at"`
}

// OpenBlob opens a blob of the content store by its hash.
type OpenBlob func(sha string) (io.ReadCloser, error)

// WrittenFiles says what WriteFiles wrote.
type WrittenFiles struct {
	Roots, Files, Blobs int
}

// WriteFiles writes the files into dir, below which nothing may be yet: each root as a directory, each file in its
// current version, the manifest, and every other version and attachment under BlobsDir, each blob once.
func WriteFiles(dir *os.Root, roots []*db.FileNode, attachments []db.AttachmentRecord, open OpenBlob) (WrittenFiles, error) {
	var done WrittenFiles
	written := map[string]bool{}
	writeBlob := func(sha string) error {
		if written[sha] {
			return nil
		}
		written[sha] = true
		done.Blobs++
		return copyBlob(dir, path.Join(BlobsDir, sha), sha, open)
	}
	var walk func(n *db.FileNode, at string, names *namer) (fileNode, error)
	walk = func(n *db.FileNode, at string, names *namer) (fileNode, error) {
		out := fileNode{ID: n.ID, Kind: n.Kind, Title: n.Title, Description: n.Description, Owner: n.OwnerLogin, Access: n.Access, Private: n.Private,
			Tags: n.Tags, Role: n.Role, Login: n.Login, Writers: n.Writers}
		if n.Role != "" && n.Quota >= 0 {
			quota := n.Quota
			out.Quota = &quota
		}
		name := names.take(n.Title)
		here := path.Join(at, name)
		if n.Kind == db.KindFile {
			out.Path = here
			for _, t := range n.Tables {
				out.Tables = append(out.Tables, tableMeta{ID: t.ID, Kind: t.Kind, Sheet: t.Sheet, SheetNumber: t.SheetNumber, Range: t.Range,
					Name: t.Name, Head: t.Head, Anchor: t.Anchor})
			}
			for i, v := range n.Versions {
				out.Versions = append(out.Versions, versionMeta{Number: v.Number, SHA256: v.Blob, Size: v.Size, Name: v.Name, Type: v.ContentType,
					Author: v.Author, At: v.At.UTC()})
				if i == len(n.Versions)-1 {
					if err := copyBlob(dir, here, v.Blob, open); err != nil {
						return out, err
					}
					done.Files++
				} else if err := writeBlob(v.Blob); err != nil {
					return out, err
				}
			}
			return out, nil
		}
		if err := dir.MkdirAll(here, 0o755); err != nil {
			return out, err
		}
		kids := newNamer()
		for _, k := range n.Kids {
			child, err := walk(k, here, kids)
			if err != nil {
				return out, err
			}
			out.Kids = append(out.Kids, child)
		}
		return out, nil
	}
	m := filesManifest{Format: Format}
	top := newNamer(BlobsDir)
	for _, r := range roots {
		node, err := walk(r, "", top)
		if err != nil {
			return done, err
		}
		m.Roots = append(m.Roots, node)
		done.Roots++
	}
	for _, a := range attachments {
		m.Attachments = append(m.Attachments, attachmentMeta{Page: a.Page, ID: a.ID, Name: a.Name, SHA256: a.Blob, Size: a.Size, Type: a.ContentType,
			Author: a.AuthorLogin, At: a.At.UTC()})
		if err := writeBlob(a.Blob); err != nil {
			return done, err
		}
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		return done, err
	}
	return done, dir.WriteFile(FilesManifest, data, 0o644)
}

// copyBlob writes a blob to name below dir, checking on the way that it is what its hash says.
func copyBlob(dir *os.Root, name, sha string, open OpenBlob) error {
	if parent := path.Dir(name); parent != "." {
		if err := dir.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	in, err := open(sha)
	if err != nil {
		return fmt.Errorf("blob %s: %w", sha, err)
	}
	defer func() { _ = in.Close() }()
	out, err := dir.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, h), in)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != sha {
		return fmt.Errorf("blob %s: the store holds content with hash %s", sha, got)
	}
	return nil
}

// FilesToImport is what ReadFiles found.
type FilesToImport struct {
	Roots       []*db.FileNode
	Attachments []db.AttachmentRecord
	// Sources names, for each hash, the files below the directory that hold that content: an older version or attachment
	// under BlobsDir, and each file whose current version it is.
	Sources map[string][]string
}

// ReadFiles reads the manifest of a directory written by WriteFiles, and where the content of each version and
// attachment lies. Paths come only from the manifest and are opened below dir, so none leads outside it.
func ReadFiles(dir *os.Root) (FilesToImport, error) {
	var in FilesToImport
	data, err := fs.ReadFile(dir.FS(), FilesManifest)
	if err != nil {
		return in, fmt.Errorf("%s: %w", FilesManifest, err)
	}
	var m filesManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return in, fmt.Errorf("%s: %w", FilesManifest, err)
	}
	if m.Format != Format {
		return in, fmt.Errorf("%s: format %d, this version reads %d", FilesManifest, m.Format, Format)
	}
	in.Sources = map[string][]string{}
	source := func(sha, current string) {
		if _, ok := in.Sources[sha]; !ok {
			in.Sources[sha] = nil
			if _, err := dir.Stat(path.Join(BlobsDir, sha)); err == nil {
				in.Sources[sha] = []string{path.Join(BlobsDir, sha)}
			}
		}
		if current != "" {
			in.Sources[sha] = append(in.Sources[sha], current)
		}
	}
	var walk func(f fileNode) (*db.FileNode, error)
	walk = func(f fileNode) (*db.FileNode, error) {
		n := &db.FileNode{Object: db.Object{ID: f.ID, Kind: f.Kind, Title: f.Title, Description: f.Description, Access: f.Access, Private: f.Private,
			Tags: f.Tags}, OwnerLogin: f.Owner, Role: f.Role, Login: f.Login, Writers: f.Writers, Quota: -1}
		if f.Quota != nil {
			n.Quota = *f.Quota
		}
		for i, v := range f.Versions {
			if !store.ValidHash(v.SHA256) {
				return nil, fmt.Errorf("%s: version %d has no valid hash", f.ID, v.Number)
			}
			current := ""
			if i == len(f.Versions)-1 {
				current = f.Path
			}
			source(v.SHA256, current)
			n.Versions = append(n.Versions, db.FileVersion{Number: v.Number, Blob: v.SHA256, Size: v.Size, Name: v.Name, ContentType: v.Type,
				Author: v.Author, At: v.At})
		}
		for _, t := range f.Tables {
			n.Tables = append(n.Tables, db.FileTable{ID: t.ID, Kind: t.Kind, Sheet: t.Sheet, SheetNumber: t.SheetNumber, Range: t.Range,
				Name: t.Name, Head: t.Head, Anchor: t.Anchor})
		}
		for _, k := range f.Kids {
			kid, err := walk(k)
			if err != nil {
				return nil, err
			}
			n.Kids = append(n.Kids, kid)
		}
		return n, nil
	}
	for _, r := range m.Roots {
		root, err := walk(r)
		if err != nil {
			return in, err
		}
		in.Roots = append(in.Roots, root)
	}
	for _, a := range m.Attachments {
		if !store.ValidHash(a.SHA256) {
			return in, fmt.Errorf("attachment %s of page %s has no valid hash", a.ID, a.Page)
		}
		source(a.SHA256, "")
		in.Attachments = append(in.Attachments, db.AttachmentRecord{Attachment: db.Attachment{Page: a.Page, ID: a.ID, Blob: a.SHA256, Size: a.Size,
			Name: a.Name, ContentType: a.Type, At: a.At}, AuthorLogin: a.Author})
	}
	for sha, paths := range in.Sources {
		if len(paths) == 0 {
			return in, fmt.Errorf("the content of %s is missing", sha)
		}
		for _, p := range paths {
			if _, err := dir.Stat(p); err != nil {
				return in, fmt.Errorf("the content of %s is missing: %w", sha, err)
			}
		}
	}
	return in, nil
}
