// SPDX-License-Identifier: MIT

package web

import (
	"encoding/json"
	"errors"
	"html"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/document"
	"github.com/Ollornog/UnderDoc/internal/id"
	"github.com/Ollornog/UnderDoc/internal/module"
	"github.com/Ollornog/UnderDoc/internal/pagekind"
	"github.com/Ollornog/UnderDoc/internal/route"
	"github.com/Ollornog/UnderDoc/internal/tree"
)

// The JSON the workspace in the browser reads (ADR-16). The workspace renders everything itself, as the approved mockup
// does; these answers only carry data.

type apiUser struct {
	ID       string   `json:"id"`
	Login    string   `json:"login"`
	Name     string   `json:"name"`
	Email    string   `json:"email"`
	Groups   []string `json:"groups"`
	Initials string   `json:"initials"`
	Admin    bool     `json:"admin"`
	// Agent tells that this request did not sign in as a session (ADR-36).
	Agent bool `json:"agent"`
}

// apiNode is one object in a list: a collection, folder or page, with what lies inside it.
type apiNode struct {
	ID      string `json:"id"`
	Title   string `json:"n"`
	Kind    string `json:"t"`
	Path    string `json:"path,omitempty"`
	Private bool   `json:"private,omitempty"`
	Owner   string `json:"owner,omitempty"`
	// Access is what the whole tenant is granted on this object itself; empty when nothing is granted here (ADR-29).
	Access string `json:"access,omitempty"`
	// Breaks tells that the object takes no roles from above (ADR-29).
	Breaks bool `json:"breaks,omitempty"`
	// Mine tells the workspace that the viewer owns it, so it offers what only the owner may do. Each change checks again.
	Mine bool `json:"mine,omitempty"`
	// Role is the viewer's role on it (ADR-29): read, comment, edit, manage or owner. The workspace greys out what it
	// does not allow; each change checks again.
	Role string   `json:"role,omitempty"`
	Tags []string `json:"tags,omitempty"`
	// Width and Contents say how a document of the database is shown, where they are set (ADR-16).
	Width    string `json:"width,omitempty"`
	Contents string `json:"contents,omitempty"`
	// Template tells that a page is offered as a template for new pages (M-8).
	Template bool      `json:"template,omitempty"`
	Kids     []apiNode `json:"kids"`
}

type apiState struct {
	User        apiUser   `json:"user"`
	Collections []apiNode `json:"collections"`
	CanCreate   bool      `json:"canCreate"`
	Problems    int       `json:"problems"`
	// Modules are all modules with their state for this user (ADR-25).
	Modules []apiModule `json:"modules"`
	// Settings are the account's settings of the workspace, as the browser saved them; absent without the database.
	Settings json.RawMessage `json:"settings,omitempty"`
	// Files tells that the view Files has the database and the content store behind it (ADR-28).
	Files bool `json:"files,omitempty"`
	// ShareRefused are the saves of this account over SMB that a quota or the largest size turned away (ADR-30).
	ShareRefused []apiRefused `json:"shareRefused,omitempty"`
}

type apiDocument struct {
	ID          string     `json:"id"`
	Title       string     `json:"n"`
	Kind        string     `json:"t"`
	Description string     `json:"desc"`
	Tags        []string   `json:"tags"`
	Place       string     `json:"place"`
	Path        string     `json:"path"`
	Changed     string     `json:"ch"`
	Intro       string     `json:"intro"`
	Sections    [][]string `json:"secs"`
	UsedIn      []string   `json:"usedIn"`
	Links       []string   `json:"links"`
	Stable      bool       `json:"stable"`
	// Blocks, CanEdit and Owner belong to pages of the database.
	Blocks []apiBlock `json:"blocks,omitempty"`
	// Embeds lists what a page of the database embeds, for its facts panel (M-3).
	Embeds  []apiEmbed `json:"embeds,omitempty"`
	CanEdit bool       `json:"canEdit,omitempty"`
	Owner   string     `json:"owner,omitempty"`
	Created string     `json:"created,omitempty"`
	// Comments counts the open comments of a page of the database, on the whole page and on its blocks, also on blocks
	// that are gone; CanComment tells whether the viewer may add some (ADR-20).
	Comments   int  `json:"comments,omitempty"`
	CanComment bool `json:"canComment,omitempty"`
	// SeenBy names who sees an object of the database and with which role, for its facts panel (ADR-29).
	SeenBy []string `json:"seenBy,omitempty"`
	// PageKind is the kind of a page of the database and Audience the side it sets (T-20, ADR-33); Findings are the
	// writing rules it breaks, for whoever may change it.
	PageKind string             `json:"kind,omitempty"`
	Audience string             `json:"audience,omitempty"`
	Findings []pagekind.Finding `json:"findings,omitempty"`
}

// apiEmbed is one thing a page embeds: a value (a part with a name), a part, a section, a block or a whole page.
type apiEmbed struct {
	Kind  string `json:"kind"`
	Name  string `json:"name,omitempty"`
	Title string `json:"title"`
	Ref   string `json:"ref"`
	URL   string `json:"url"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// folderID marks a folder of the folder source, so it can never clash with the ID of a document.
func folderID(path string) string { return "f:" + path }

// apiState returns who is signed in and every collection with its tree: those of the database first, then the folders
// at the top of the folder source, and the pages directly in its root as "Home".
func (h *handler) apiState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := account.From(ctx)
	lib := h.library(r.Context())
	modules := h.moduleStates(r)
	documents := slices.ContainsFunc(modules, func(m module.State) bool { return m.ID == module.Documents && m.On && !m.Hidden })
	if !documents {
		// Switched off or hidden: no entry of Documents in the workspace (ADR-25).
		lib = h.noFolder
	}
	state := apiState{
		User: apiUser{ID: user.ID, Login: user.Login, Name: user.Name, Email: user.Email, Groups: user.Groups, Initials: user.Initials(),
			Admin: user.Admin, Agent: user.Agent()},
		CanCreate: h.options.DB != nil && documents,
		Problems:  len(lib.Problems()),
		Files: h.options.DB != nil && h.options.Store != nil &&
			slices.ContainsFunc(modules, func(m module.State) bool { return m.ID == module.Files && m.On && !m.Hidden }),
		Modules: apiModules(modules),
	}
	if h.options.Share != nil && h.options.DB != nil {
		if refused := h.refusedOf(r.Context(), user.ID); len(refused) > 0 {
			state.ShareRefused = refused
		}
	}
	if state.User.Groups == nil {
		state.User.Groups = []string{}
	}
	if h.options.DB != nil {
		var collections []db.Object
		var err error
		if documents {
			collections, err = h.options.DB.Collections(ctx, user.ID)
		}
		if err != nil {
			h.log.Error("collections failed", slog.String("error", err.Error()))
		}
		for _, c := range collections {
			state.Collections = append(state.Collections, h.databaseNode(r, c, 0))
		}
		if state.Settings, err = h.options.DB.Settings(ctx, user.ID); err != nil {
			h.log.Error("settings failed", slog.String("error", err.Error()))
		}
	}
	root := lib.Tree().Root()
	if len(root.Documents) > 0 {
		home := apiNode{ID: folderID(""), Title: homeLabel, Kind: "collection", Path: "the folder"}
		for _, docID := range root.Documents {
			home.Kids = append(home.Kids, documentNode(lib.Document, docID))
		}
		state.Collections = append(state.Collections, home)
	}
	for _, sub := range root.Folders {
		node := folderNode(lib.Document, sub)
		node.Kind = "collection"
		state.Collections = append(state.Collections, node)
	}
	if state.Collections == nil {
		state.Collections = []apiNode{}
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *handler) databaseNode(r *http.Request, o db.Object, depth int) apiNode {
	user, _ := account.From(r.Context())
	node := apiNode{ID: o.ID, Title: o.Title, Kind: o.Kind, Private: o.Private, Owner: h.options.DB.AccountName(r.Context(), o.Owner),
		Access: o.Access, Breaks: o.Breaks, Mine: o.Owner == user.ID, Tags: o.Tags, Width: o.Width, Contents: o.Contents, Template: o.Template, Kids: []apiNode{}}
	role, err := h.options.DB.Role(r.Context(), o.ID, user.ID)
	if err != nil {
		h.log.Error("role failed", slog.String("error", err.Error()))
	}
	node.Role = role
	if depth > 8 {
		return node
	}
	children, err := h.options.DB.Children(r.Context(), o.ID, user.ID)
	if err != nil {
		h.log.Error("children failed", slog.String("error", err.Error()))
	}
	for _, c := range children {
		node.Kids = append(node.Kids, h.databaseNode(r, c, depth+1))
	}
	return node
}

func folderNode(get func(document.ID) (*document.Document, bool), folder *tree.Folder) apiNode {
	node := apiNode{ID: folderID(folder.Path), Title: folder.Name, Kind: "folder", Path: folder.Path + "/", Kids: []apiNode{}}
	for _, sub := range folder.Folders {
		node.Kids = append(node.Kids, folderNode(get, sub))
	}
	for _, docID := range folder.Documents {
		node.Kids = append(node.Kids, documentNode(get, docID))
	}
	return node
}

func documentNode(get func(document.ID) (*document.Document, bool), docID document.ID) apiNode {
	doc, _ := get(docID)
	return apiNode{ID: string(docID), Title: doc.Title, Kind: "page", Path: doc.Source, Kids: []apiNode{}}
}

var (
	h2Start = regexp.MustCompile(`<h2[\s>]`)
	tag     = regexp.MustCompile(`<[^>]*>`)
)

// splitSections cuts a rendered page at its second-level headings: the text before the first one, and each heading with
// what follows it. The workspace builds its contents, its source view and its counts from these parts, as the mockup does.
func splitSections(body string) (string, [][]string) {
	starts := h2Start.FindAllStringIndex(body, -1)
	if len(starts) == 0 {
		return body, [][]string{}
	}
	intro := body[:starts[0][0]]
	sections := make([][]string, 0, len(starts))
	for i, start := range starts {
		end := len(body)
		if i+1 < len(starts) {
			end = starts[i+1][0]
		}
		part := body[start[0]:end]
		close := strings.Index(part, "</h2>")
		if close < 0 {
			continue
		}
		title := html.UnescapeString(tag.ReplaceAllString(part[:close], ""))
		sections = append(sections, []string{strings.TrimSpace(title), part[close+len("</h2>"):]})
	}
	return intro, sections
}

// apiDocument returns one page of the folder source with its parts, place and links, or a collection of the database.
func (h *handler) apiDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, _ := account.From(ctx)
	objectID := r.PathValue("id")
	if h.options.DB != nil && id.Valid(objectID) {
		o, err := h.options.DB.Object(ctx, objectID, user.ID)
		if err == nil {
			h.noteOpen(r, o)
		}
		if err == nil && db.HasBlocks(o.Kind) {
			h.pageDocument(w, r, o)
			return
		}
		if err == nil {
			writeJSON(w, http.StatusOK, apiDocument{ID: o.ID, Title: o.Title, Kind: o.Kind, Description: o.Description, Tags: tagList(o.Tags),
				Path: "not a file, lives in the database", Changed: stamp(o.Changed), Intro: "", Sections: [][]string{},
				UsedIn: []string{}, Links: []string{}, Stable: true, Owner: h.options.DB.AccountName(ctx, o.Owner), SeenBy: h.seenBy(r, o)})
			return
		}
		if !errors.Is(err, db.ErrNotFound) {
			h.log.Error("object failed", slog.String("error", err.Error()))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
	}
	lib := h.library(r.Context())
	doc, ok := lib.Document(document.ID(objectID))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such page"})
		return
	}
	folder, _ := lib.Tree().Location(doc.ID)
	var place []string
	if folder != nil {
		for _, f := range tree.Ancestors(folder)[1:] {
			place = append(place, f.Name)
		}
	}
	if len(place) == 0 {
		place = []string{homeLabel}
	}
	intro, sections := splitSections(withoutFirstHeading(doc.HTML, doc.StartsWithTitle))
	answer := apiDocument{ID: string(doc.ID), Title: doc.Title, Kind: "page", Description: doc.Description, Tags: doc.Tags,
		Place: strings.Join(place, " › "), Path: doc.Source, Changed: stamp(doc.Modified), Intro: intro, Sections: sections,
		UsedIn: []string{}, Links: []string{}, Stable: doc.Stable}
	if answer.Tags == nil {
		answer.Tags = []string{}
	}
	for _, other := range doc.Backlinks {
		if d, ok := lib.Document(other); ok {
			answer.UsedIn = append(answer.UsedIn, d.Title)
		}
	}
	for _, other := range doc.Links {
		if d, ok := lib.Document(other); ok {
			answer.Links = append(answer.Links, d.Title)
		}
	}
	writeJSON(w, http.StatusOK, answer)
}

// apiCreateCollection makes a collection, from the workspace, and answers with it.
func (h *handler) apiCreateCollection(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title  string `json:"title"`
		Parent string `json:"parent"`
	}
	user, ok := h.change(w, r, 8<<10, &in)
	if !ok {
		return
	}
	created, err := h.options.DB.CreateCollection(r.Context(), user, in.Title, in.Parent)
	if h.failed(w, err, "collection") {
		return
	}
	h.noteMade(r, created.ID)
	writeJSON(w, http.StatusCreated, apiNode{ID: created.ID, Title: created.Title, Kind: created.Kind, Kids: []apiNode{}, Path: route.Object(created.ID)})
}

// apiRename gives an object of the database a new title.
func (h *handler) apiRename(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title string `json:"title"`
	}
	user, ok := h.change(w, r, 8<<10, &in)
	if !ok {
		return
	}
	o, err := h.options.DB.Rename(r.Context(), r.PathValue("id"), user.ID, in.Title)
	if h.failed(w, err, "rename") {
		return
	}
	writeJSON(w, http.StatusOK, apiNode{ID: o.ID, Title: o.Title, Kind: o.Kind, Kids: []apiNode{}})
}

// tagList is never null in JSON.
func tagList(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	return tags
}

// apiTags gives an object of the database its tags, as a whole list.
func (h *handler) apiTags(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Tags []string `json:"tags"`
	}
	user, ok := h.change(w, r, 8<<10, &in)
	if !ok {
		return
	}
	o, err := h.options.DB.SetTags(r.Context(), r.PathValue("id"), user.ID, in.Tags)
	if h.failed(w, err, "tags") {
		return
	}
	writeJSON(w, http.StatusOK, map[string][]string{"tags": tagList(o.Tags)})
}

// apiView keeps how a document of the database is shown: its width, where its contents show, or both (ADR-16).
func (h *handler) apiView(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Width    *string `json:"width"`
		Contents *string `json:"contents"`
	}
	user, ok := h.change(w, r, 1<<10, &in)
	if !ok {
		return
	}
	o, err := h.options.DB.SetView(r.Context(), r.PathValue("id"), user.ID, in.Width, in.Contents)
	if h.failed(w, err, "view") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"width": o.Width, "contents": o.Contents})
}

// apiUses says, before deleting, where an object or anything below it is embedded from outside of it.
func (h *handler) apiUses(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	user, _ := account.From(r.Context())
	if _, err := h.options.DB.Object(r.Context(), r.PathValue("id"), user.ID); h.failed(w, err, "uses") {
		return
	}
	if target := r.URL.Query().Get("ref"); target != "" {
		h.usesOf(w, r, r.PathValue("id"), target)
		return
	}
	titles, hidden, err := h.options.DB.Uses(r.Context(), r.PathValue("id"), user.ID)
	if h.failed(w, err, "uses") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"usedIn": titles, "hidden": hidden})
}

// stamp writes a moment for the browser: in UTC, as RFC 3339. The browser shows it in the time zone of its device, so the
// server needs to know none (2026-11-09).
func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }
