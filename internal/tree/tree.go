// SPDX-License-Identifier: MIT

// Package tree is the folder tree. It only arranges documents by ID and knows nothing about their content or roles,
// so moving a document changes where it appears, not what it is (ADR-13).
package tree

import (
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/Ollornog/UnderDoc/internal/document"
)

// Folder is one node of the tree.
type Folder struct {
	Name      string
	Path      string
	Folders   []*Folder
	Documents []document.ID
	parent    *Folder
}

// Tree holds the folders and the location of every document. A document has exactly one main location.
type Tree struct {
	root    *Folder
	folders map[string]*Folder
	placed  map[document.ID]*Folder
}

// New returns an empty tree.
func New() *Tree {
	root := &Folder{}
	return &Tree{root: root, folders: map[string]*Folder{"": root}, placed: map[document.ID]*Folder{}}
}

// Place puts a document into a folder and creates the folder if needed. Placing it again moves it.
func (t *Tree) Place(id document.ID, folderPath string) {
	if old, ok := t.placed[id]; ok {
		old.Documents = slices.DeleteFunc(old.Documents, func(d document.ID) bool { return d == id })
	}
	folder := t.ensure(folderPath)
	folder.Documents = append(folder.Documents, id)
	t.placed[id] = folder
}

func (t *Tree) ensure(folderPath string) *Folder {
	if folderPath == "." {
		folderPath = ""
	}
	if folder, ok := t.folders[folderPath]; ok {
		return folder
	}
	parent := t.ensure(path.Dir(folderPath))
	folder := &Folder{Name: path.Base(folderPath), Path: folderPath, parent: parent}
	parent.Folders = append(parent.Folders, folder)
	t.folders[folderPath] = folder
	return folder
}

// Root returns the root folder.
func (t *Tree) Root() *Folder { return t.root }

// Folder returns the folder at a path.
func (t *Tree) Folder(folderPath string) (*Folder, bool) {
	folder, ok := t.folders[folderPath]
	return folder, ok
}

// Location returns the folder that holds a document.
func (t *Tree) Location(id document.ID) (*Folder, bool) {
	folder, ok := t.placed[id]
	return folder, ok
}

// Ancestors returns the folders from the root down to and including folder.
func Ancestors(folder *Folder) []*Folder {
	var chain []*Folder
	for f := folder; f != nil; f = f.parent {
		chain = append(chain, f)
	}
	slices.Reverse(chain)
	return chain
}

// Sort orders every folder: subfolders and documents each by name, ignoring case.
func (t *Tree) Sort(title func(document.ID) string) {
	for _, folder := range t.folders {
		slices.SortFunc(folder.Folders, func(a, b *Folder) int {
			return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		})
		slices.SortFunc(folder.Documents, func(a, b document.ID) int {
			return cmp.Or(cmp.Compare(strings.ToLower(title(a)), strings.ToLower(title(b))), cmp.Compare(a, b))
		})
	}
}
