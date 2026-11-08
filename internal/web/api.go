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

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/document"
	"github.com/Ollornog/UnderDoc/internal/id"
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
}

// apiNode is one object in a list: a collection, folder or page, with what lies inside it.
type apiNode struct {
	ID      string    `json:"id"`
	Title   string    `json:"n"`
	Kind    string    `json:"t"`
	Path    string    `json:"path,omitempty"`
	Private bool      `json:"private,omitempty"`
	Kids    []apiNode `json:"kids"`
}

type apiState struct {
	User        apiUser   `json:"user"`
	Collections []apiNode `json:"collections"`
	CanCreate   bool      `json:"canCreate"`
	Problems    int       `json:"problems"`
}

type apiSection struct {
	Title string `json:"0"`
	HTML  string `json:"1"`
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
}

type apiHit struct {
	ID      string `json:"id"`
	Title   string `json:"n"`
	Place   string `json:"place"`
	Snippet string `json:"snippet"`
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
	lib := h.store.Library()
	state := apiState{
		User: apiUser{ID: user.ID, Login: user.Login, Name: user.Name, Email: user.Email, Groups: user.Groups, Initials: user.Initials(),
			Admin: h.options.AdminGroup != "" && slices.Contains(user.Groups, h.options.AdminGroup)},
		CanCreate: h.options.DB != nil,
		Problems:  len(lib.Problems()),
	}
	if state.User.Groups == nil {
		state.User.Groups = []string{}
	}
	if h.options.DB != nil {
		collections, err := h.options.DB.Collections(ctx, user.ID)
		if err != nil {
			h.log.Error("collections failed", slog.String("error", err.Error()))
		}
		for _, c := range collections {
			state.Collections = append(state.Collections, h.databaseNode(r, c, 0))
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
	node := apiNode{ID: o.ID, Title: o.Title, Kind: o.Kind, Private: o.Private, Kids: []apiNode{}}
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
			writeJSON(w, http.StatusOK, apiDocument{ID: o.ID, Title: o.Title, Kind: o.Kind, Description: o.Description, Tags: []string{},
				Path: "not a file, lives in the database", Changed: o.Changed.Format("Jan 02"), Intro: "", Sections: [][]string{},
				UsedIn: []string{}, Links: []string{}, Stable: true})
			return
		}
		if !errors.Is(err, db.ErrNotFound) {
			h.log.Error("object failed", slog.String("error", err.Error()))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
	}
	lib := h.store.Library()
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
		Place: strings.Join(place, " › "), Path: doc.Source, Changed: doc.Modified.Format("Jan 02 15:04"), Intro: intro, Sections: sections,
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

// apiSearch searches the full text of the folder source.
func (h *handler) apiSearch(w http.ResponseWriter, r *http.Request) {
	lib := h.store.Library()
	hits := []apiHit{}
	for _, hit := range lib.Search(r.URL.Query().Get("q"), maxSearchResults) {
		var snippet strings.Builder
		for _, f := range hit.Snippet {
			if f.Match {
				snippet.WriteString("<mark>" + html.EscapeString(f.Text) + "</mark>")
			} else {
				snippet.WriteString(html.EscapeString(f.Text))
			}
		}
		place := homeLabel
		if folder, ok := lib.Tree().Location(hit.Document.ID); ok && folder != lib.Tree().Root() {
			var names []string
			for _, f := range tree.Ancestors(folder)[1:] {
				names = append(names, f.Name)
			}
			place = strings.Join(names, " › ")
		}
		hits = append(hits, apiHit{ID: string(hit.Document.ID), Title: hit.Document.Title, Place: place, Snippet: snippet.String()})
	}
	writeJSON(w, http.StatusOK, hits)
}

// apiCreateCollection makes a collection, from the workspace, and answers with it.
func (h *handler) apiCreateCollection(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "collections need the database"})
		return
	}
	if !sameOrigin(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only from this application"})
		return
	}
	var in struct {
		Title  string `json:"title"`
		Parent string `json:"parent"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	user, _ := account.From(r.Context())
	created, err := h.options.DB.CreateCollection(r.Context(), user, in.Title, in.Parent)
	switch {
	case errors.Is(err, db.ErrTitle):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, db.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such collection"})
	case err != nil:
		h.log.Error("collection not made", slog.String("error", err.Error()))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	default:
		writeJSON(w, http.StatusCreated, apiNode{ID: created.ID, Title: created.Title, Kind: created.Kind, Kids: []apiNode{}, Path: route.Object(created.ID)})
	}
}
