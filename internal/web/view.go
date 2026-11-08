// SPDX-License-Identifier: MIT

package web

import (
	"html/template"
	"log/slog"
	"net/http"
	"slices"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
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
	// Profile fills nav2 and nav3 in the profile view, and is nil in every other view (ADR-16).
	Profile *profileState
	// CanCreate is true when there is a database to keep what New makes (M-28).
	CanCreate bool
}

// profileState is the profile view: its sections in nav2, the chosen one in nav3.
type profileState struct {
	Section  string
	Settings []profileSection
	// Admin lists the administration, set apart, for administrators only. It is empty for everyone else.
	Admin []profileSection
}

type profileSection struct {
	ID      string
	Label   string
	Icon    string
	Href    string
	Current bool
	// Later marks a section that is not built yet (ADR-17).
	Later string
}

type tagLink struct {
	Label string
	Href  string
}

type collectionLink struct {
	Label   string
	Href    string
	Current bool
	Private bool
}

type openCollection struct {
	Label string
	Href  string
	// ID is set for a collection of the database, so New in its heading can make something inside it.
	ID    string
	Items []treeItem
}

// objectView is the page of an object of the database, so far a collection.
type objectView struct {
	shellView
	ID        string
	Kind      string
	Owner     string
	Access    string
	Inherited bool
	Visible   string
	Entries   []linkItem
}

func (objectView) kindOf(kind string) string {
	return map[string]string{db.KindCollection: "Collection", db.KindFolder: "Folder", db.KindPage: "Page"}[kind]
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
	// Folder shows a folder icon in the list of entries instead of a page.
	Folder bool
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
	// Status is the line at the bottom of the window: words, characters and reading time.
	Status string
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
}

// makeShell builds what every page shares. current is the open document, or empty, and at is the open folder, or nil.
func (h *handler) makeShell(r *http.Request, lib *library.Library, title string, current document.ID, at *tree.Folder) shellView {
	shell := shellView{Title: title, ProblemCount: len(lib.Problems()), AssetVersion: h.assetVersion, View: "workspace"}
	shell.User, _ = account.From(r.Context())
	if h.options.DB != nil {
		shell.CanCreate = true
		collections, err := h.options.DB.Collections(r.Context(), shell.User.ID)
		if err != nil {
			h.log.Error("collections failed", slog.String("error", err.Error()))
		}
		for _, c := range collections {
			shell.Collections = append(shell.Collections, collectionLink{Label: c.Title, Href: route.Object(c.ID), Private: c.Private})
		}
	}
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
	// Collections of the folder source follow those of the database, until the import moves them in (M-28).
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

// openFromDatabase puts the collection an object of the database belongs to into nav2 and nav3, marks the object and
// fills the breadcrumbs with the way down to it.
func (h *handler) openFromDatabase(r *http.Request, shell *shellView, objectID string) {
	ctx, viewer := r.Context(), shell.User.ID
	var way []db.Object
	for o, err := h.options.DB.Object(ctx, objectID, viewer); err == nil; o, err = h.options.DB.Object(ctx, o.Parent, viewer) {
		way = append([]db.Object{o}, way...)
		if o.Parent == "" {
			break
		}
	}
	if len(way) == 0 {
		return
	}
	top := way[0]
	for i := range shell.Collections {
		shell.Collections[i].Current = shell.Collections[i].Href == route.Object(top.ID)
	}
	open := map[string]bool{}
	for _, o := range way {
		open[o.ID] = true
	}
	shell.Open = &openCollection{Label: top.Title, Href: route.Object(top.ID), ID: top.ID, Items: h.databaseTree(r, top.ID, objectID, open, 0)}
	shell.Crumbs = nil
	for _, o := range way[:len(way)-1] {
		shell.Crumbs = append(shell.Crumbs, crumb{Label: o.Title, Href: route.Object(o.ID)})
	}
}

// databaseTree lists what is inside an object, nested, as deep as it is open or a few levels.
func (h *handler) databaseTree(r *http.Request, parent, current string, open map[string]bool, depth int) []treeItem {
	if depth > 8 {
		return nil
	}
	user, _ := account.From(r.Context())
	children, err := h.options.DB.Children(r.Context(), parent, user.ID)
	if err != nil {
		h.log.Error("children failed", slog.String("error", err.Error()))
		return nil
	}
	items := make([]treeItem, 0, len(children))
	for _, c := range children {
		items = append(items, treeItem{
			Label:    c.Title,
			Href:     route.Object(c.ID),
			Current:  c.ID == current,
			IsFolder: c.Kind != db.KindPage,
			Open:     open[c.ID] || depth == 0,
			Children: h.databaseTree(r, c.ID, current, open, depth+1),
		})
	}
	return items
}
