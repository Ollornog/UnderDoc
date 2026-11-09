// SPDX-License-Identifier: MIT

// Package transfer turns trees of the database into normal files and back (ADR-13, ADR-19): collections and folders become
// directories, pages become Markdown files with front matter, and a short HTML comment before each block keeps its ID
// without showing in any Markdown viewer. The layout is described in docs/export.md.
package transfer

import (
	"bytes"
	"fmt"
	"github.com/Ollornog/UnderDoc/internal/markdown"
	"path"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/Ollornog/UnderDoc/internal/db"
)

// MetaFile describes a directory: the collection or folder it is, and the order of what lies in it, lines and spaces
// included. It starts with a dot, so the folder source and most file browsers leave it out.
const MetaFile = ".underdoc.yaml"

// PrivateDir is the directory at the root of a full export that holds the private notes, one directory per login.
const PrivateDir = "private"

// Format is the version of the layout, written at the root of an export.
const Format = 1

// Put writes one file at a slash-separated path. Directories come with their files.
type Put func(name string, data []byte) error

// Tree is what an export writes: shared collections, and private ones with the login of their account.
type Tree struct {
	Shared  []*db.Node
	Private []db.Owned
}

type dirMeta struct {
	Format      int           `yaml:"format,omitempty"`
	ID          string        `yaml:"id,omitempty"`
	Kind        string        `yaml:"kind,omitempty"`
	Title       string        `yaml:"title,omitempty"`
	Description string        `yaml:"description,omitempty"`
	Access      string        `yaml:"access,omitempty"`
	Private     bool          `yaml:"private,omitempty"`
	Tags        markdown.Tags `yaml:"tags,omitempty,flow"`
	Order       []entry       `yaml:"order,omitempty"`
}

// entry is one place in the order of a directory: a file or directory by its name, or a line or a space by its ID.
type entry struct {
	Name  string `yaml:"name,omitempty"`
	Line  string `yaml:"line,omitempty"`
	Space string `yaml:"space,omitempty"`
}

type pageMeta struct {
	ID          string        `yaml:"id,omitempty"`
	Title       string        `yaml:"title,omitempty"`
	Description string        `yaml:"description,omitempty"`
	Access      string        `yaml:"access,omitempty"`
	Tags        markdown.Tags `yaml:"tags,omitempty,flow"`
	// NextBlock is written only when it is higher than the block after the highest, so that an ID deleted at the end is
	// not given out again after an import.
	NextBlock int `yaml:"next_block,omitempty"`
}

// marker is the line before each block. An HTML comment on a line of its own is dropped by every Markdown viewer and by
// UnderDoc itself.
func marker(blockID string) string { return "<!-- block " + blockID + " -->" }

type writer struct {
	put      Put
	warnings []string
}

// Write writes a tree through put and returns warnings about what will not come back exactly as it is. The root gets a
// MetaFile with the format and the order of the shared collections; private notes go to private/<login>/.
func Write(t Tree, put Put) ([]string, error) {
	w := &writer{put: put}
	names := newNamer(PrivateDir)
	root := dirMeta{Format: Format}
	for _, n := range t.Shared {
		e, err := w.node("", names, n)
		if err != nil {
			return nil, err
		}
		root.Order = append(root.Order, e)
	}
	if err := w.meta("", root); err != nil {
		return nil, err
	}
	if len(t.Private) == 0 {
		return w.warnings, nil
	}
	byLogin := map[string][]*db.Node{}
	var logins []string
	for _, o := range t.Private {
		if _, seen := byLogin[o.Login]; !seen {
			logins = append(logins, o.Login)
		}
		byLogin[o.Login] = append(byLogin[o.Login], o.Node)
	}
	slices.Sort(logins)
	accounts := newNamer()
	for _, login := range logins {
		dir := path.Join(PrivateDir, accounts.take(login))
		order, err := w.children(dir, byLogin[login])
		if err != nil {
			return nil, err
		}
		if err := w.meta(dir, dirMeta{Order: order}); err != nil {
			return nil, err
		}
	}
	return w.warnings, nil
}

func (w *writer) meta(dir string, m dirMeta) error {
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return w.put(path.Join(dir, MetaFile), data)
}

// children writes the objects of one directory and returns their order.
func (w *writer) children(dir string, kids []*db.Node) ([]entry, error) {
	names := newNamer()
	order := make([]entry, 0, len(kids))
	for _, k := range kids {
		e, err := w.node(dir, names, k)
		if err != nil {
			return nil, err
		}
		order = append(order, e)
	}
	return order, nil
}

// node writes one object into dir and returns its entry in the order there.
func (w *writer) node(dir string, names *namer, n *db.Node) (entry, error) {
	switch n.Kind {
	case db.KindLine:
		return entry{Line: n.ID}, nil
	case db.KindSpace:
		return entry{Space: n.ID}, nil
	case db.KindPage:
		stem := names.take(n.Title)
		file := path.Join(dir, stem+".md")
		text, err := pageText(n)
		if err != nil {
			return entry{}, err
		}
		w.check(file, n, text)
		if err := w.put(file, text); err != nil {
			return entry{}, err
		}
		if len(n.Kids) > 0 {
			kids := path.Join(dir, stem)
			order, err := w.children(kids, n.Kids)
			if err != nil {
				return entry{}, err
			}
			if err := w.meta(kids, dirMeta{Order: order}); err != nil {
				return entry{}, err
			}
		}
		return entry{Name: stem + ".md"}, nil
	default:
		stem := names.take(n.Title)
		sub := path.Join(dir, stem)
		order, err := w.children(sub, n.Kids)
		if err != nil {
			return entry{}, err
		}
		m := dirMeta{ID: n.ID, Kind: n.Kind, Title: n.Title, Description: n.Description, Access: n.Access, Private: n.Private, Tags: n.Tags, Order: order}
		return entry{Name: stem}, w.meta(sub, m)
	}
}

// trimBody takes the blank lines off both ends of a block: they mean nothing in Markdown, and the file needs one blank
// line between blocks.
func trimBody(body string) string { return strings.Trim(body, "\r\n") }

// pageText is the Markdown file of a page: front matter, then each block after its marker, one blank line apart.
func pageText(n *db.Node) ([]byte, error) {
	m := pageMeta{ID: n.ID, Title: n.Title, Description: n.Description, Access: n.Access, Tags: n.Tags}
	if next := nextAfter(n.Blocks); n.NextBlock > next {
		m.NextBlock = n.NextBlock
	}
	front, err := yaml.Marshal(m)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	b.Write(front)
	b.WriteString("---\n")
	for _, block := range n.Blocks {
		b.WriteString("\n" + marker(block.ID) + "\n")
		if body := trimBody(block.Body); body != "" {
			b.WriteString(body + "\n")
		}
	}
	return b.Bytes(), nil
}

// check reads a written page back and warns if its blocks would not come back as they are: a block that holds a line
// like a marker outside fenced code, or a fence it never closes, swallows or splits what follows.
func (w *writer) check(file string, n *db.Node, text []byte) {
	back := parsePage(text, path.Base(file))
	same := len(back.Blocks) == len(n.Blocks)
	for i := 0; same && i < len(n.Blocks); i++ {
		same = back.Blocks[i].ID == n.Blocks[i].ID && back.Blocks[i].Body == trimBody(n.Blocks[i].Body)
	}
	if !same {
		w.warnings = append(w.warnings, fmt.Sprintf("%s: the blocks will not come back exactly, a block holds a marker line or an open code fence", file))
	}
}
