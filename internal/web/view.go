// SPDX-License-Identifier: MIT

package web

import (
	"html/template"
	"slices"

	"github.com/Ollornog/UnderDoc/internal/document"
	"github.com/Ollornog/UnderDoc/internal/library"
	"github.com/Ollornog/UnderDoc/internal/route"
	"github.com/Ollornog/UnderDoc/internal/tree"
)

// The view models carry finished values. The templates only render them and calculate nothing.

type shellView struct {
	Title        string
	Query        string
	Tree         []treeItem
	Crumbs       []crumb
	ProblemCount int
	AssetVersion string
}

type treeItem struct {
	Label    string
	Href     string
	Current  bool
	Open     bool
	IsFolder bool
	Children []treeItem
}

type crumb struct {
	Label string
	// Href is empty for the current page.
	Href string
}

type linkItem struct {
	Label  string
	Href   string
	Detail string
}

type documentView struct {
	shellView
	ShowTitle bool
	Body      template.HTML
	Source    string
	Modified  string
	PathBased bool
	Backlinks []linkItem
	Contents  []contentsItem
}

type contentsItem struct {
	Label string
	Href  string
	Level int
}

type folderView struct {
	shellView
	Recent    []linkItem
	Folders   []linkItem
	Documents []linkItem
	Empty     bool
}

type searchView struct {
	shellView
	Searched bool
	Results  []searchResult
}

type searchResult struct {
	linkItem
	Snippet []library.Fragment
}

type problemsView struct {
	shellView
	Problems []library.Problem
}

type statusView struct {
	shellView
	Heading string
	Message string
}

// makeShell builds what every page shares. current is the open document, or empty, and at is the open folder, or nil.
func (h *handler) makeShell(lib *library.Library, title string, current document.ID, at *tree.Folder) shellView {
	shell := shellView{Title: title, ProblemCount: len(lib.Problems()), AssetVersion: h.assetVersion}
	var open []*tree.Folder
	if current != "" {
		at, _ = lib.Tree().Location(current)
	}
	if at != nil {
		open = tree.Ancestors(at)
	}
	shell.Tree = makeTree(lib, lib.Tree().Root(), current, open)
	return shell
}

func makeTree(lib *library.Library, folder *tree.Folder, current document.ID, open []*tree.Folder) []treeItem {
	items := make([]treeItem, 0, len(folder.Folders)+len(folder.Documents))
	for _, sub := range folder.Folders {
		items = append(items, treeItem{
			Label:    sub.Name,
			Href:     route.Folder(sub.Path),
			IsFolder: true,
			Open:     slices.Contains(open, sub),
			Children: makeTree(lib, sub, current, open),
		})
	}
	for _, id := range folder.Documents {
		doc, _ := lib.Document(id)
		items = append(items, treeItem{Label: doc.Title, Href: route.Document(string(id)), Current: id == current})
	}
	return items
}

// makeCrumbs returns the way from the root down to a folder.
func makeCrumbs(folder *tree.Folder) []crumb {
	crumbs := []crumb{{Label: "Home", Href: route.Folder("")}}
	if folder == nil {
		return crumbs
	}
	for _, f := range tree.Ancestors(folder)[1:] {
		crumbs = append(crumbs, crumb{Label: f.Name, Href: route.Folder(f.Path)})
	}
	return crumbs
}
