// SPDX-License-Identifier: MIT

package web

import (
	"html/template"
	"net/http"
	"slices"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/document"
	"github.com/Ollornog/UnderDoc/internal/library"
	"github.com/Ollornog/UnderDoc/internal/route"
	"github.com/Ollornog/UnderDoc/internal/tree"
)

// The view models carry finished values. The templates only render them and calculate nothing.

// shellView is what every page shares: the top row, the three columns and the head of the page (ADR-16).
type shellView struct {
	Title        string
	Description  string
	Tags         []tagLink
	Query        string
	Crumbs       []crumb
	ProblemCount int
	AssetVersion string
	User         account.User
	// View is the entry of nav1 that is active: "workspace", "search" or "profile".
	View string
	// Collections are listed in nav2. Until collections live in the database, each folder at the top is one (M-28).
	Collections []collectionLink
	// Open is the collection nav3 shows, or nil.
	Open *openCollection
}

type tagLink struct {
	Label string
	Href  string
}

type collectionLink struct {
	Label   string
	Href    string
	Current bool
}

type openCollection struct {
	Label string
	Href  string
	Items []treeItem
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
	Body      template.HTML
	ID        string
	Address   string
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
	Tag      string
	Tagged   []linkItem
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

type profileView struct {
	shellView
	Admin bool
}

// makeShell builds what every page shares. current is the open document, or empty, and at is the open folder, or nil.
func (h *handler) makeShell(r *http.Request, lib *library.Library, title string, current document.ID, at *tree.Folder) shellView {
	shell := shellView{Title: title, ProblemCount: len(lib.Problems()), AssetVersion: h.assetVersion, View: "workspace"}
	shell.User, _ = account.From(r.Context())
	if current != "" {
		at, _ = lib.Tree().Location(current)
	}
	var open []*tree.Folder
	if at != nil {
		open = tree.Ancestors(at)
	}
	root := lib.Tree().Root()
	// The collection of a page is the folder at the top it lies in. Pages directly in the root form "Home".
	var collection *tree.Folder
	if len(open) > 1 {
		collection = open[1]
	} else if at != nil {
		collection = root
	}
	if len(root.Documents) > 0 {
		shell.Collections = append(shell.Collections, collectionLink{Label: homeLabel, Href: route.Folder(""), Current: collection == root})
	}
	for _, sub := range root.Folders {
		shell.Collections = append(shell.Collections, collectionLink{Label: sub.Name, Href: route.Folder(sub.Path), Current: collection == sub})
	}
	switch {
	case collection == root:
		shell.Open = &openCollection{Label: homeLabel, Href: route.Folder(""), Items: documentItems(lib, root, current)}
	case collection != nil:
		shell.Open = &openCollection{Label: collection.Name, Href: route.Folder(collection.Path), Items: makeTree(lib, collection, current, open)}
	}
	return shell
}

const homeLabel = "Home"

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
	return append(items, documentItems(lib, folder, current)...)
}

func documentItems(lib *library.Library, folder *tree.Folder, current document.ID) []treeItem {
	items := make([]treeItem, 0, len(folder.Documents))
	for _, id := range folder.Documents {
		doc, _ := lib.Document(id)
		items = append(items, treeItem{Label: doc.Title, Href: route.Document(string(id)), Current: id == current})
	}
	return items
}

// makeCrumbs returns the way from the root down to a folder.
func makeCrumbs(folder *tree.Folder) []crumb {
	crumbs := []crumb{{Label: homeLabel, Href: route.Folder("")}}
	if folder == nil {
		return crumbs
	}
	for _, f := range tree.Ancestors(folder)[1:] {
		crumbs = append(crumbs, crumb{Label: f.Name, Href: route.Folder(f.Path)})
	}
	return crumbs
}

func makeTags(tags []string) []tagLink {
	links := make([]tagLink, 0, len(tags))
	for _, tag := range tags {
		links = append(links, tagLink{Label: "#" + tag, Href: route.Tag(tag)})
	}
	return links
}
