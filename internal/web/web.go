// SPDX-License-Identifier: MIT

// Package web is the read-only web interface. Everyone who gets past the sign-in gate in front of it can read everything.
package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Ollornog/UnderDoc/docs"
	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/document"
	"github.com/Ollornog/UnderDoc/internal/id"
	"github.com/Ollornog/UnderDoc/internal/library"
	"github.com/Ollornog/UnderDoc/internal/route"
)

//go:embed templates/*.html
var templateFiles embed.FS

//go:embed all:assets
var assetFiles embed.FS

const (
	maxSearchResults   = 50
	maxRecentDocuments = 10
	// A table of contents with a single entry would only repeat the heading.
	minContentsEntries = 2
)

var imageTypes = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".avif": "image/avif",
	".svg":  "image/svg+xml",
}

// Options configures the handler.
type Options struct {
	// DevUI turns on /dev/ui, which shows every component in every state.
	DevUI bool
	// DevUser stands in for the gate when the application runs on its own, for development only.
	DevUser *account.User
	// AdminGroup is the group of the gate whose members see the administration.
	AdminGroup string
	// DB keeps accounts and the objects made in the web (ADR-19). Without it, the folder source is all there is.
	DB *db.DB
}

type handler struct {
	store        *library.Store
	log          *slog.Logger
	pages        map[string]*template.Template
	assetVersion string
	options      Options
	// seen keeps when an account was last written, so not every request writes it.
	seenMu sync.Mutex
	seen   map[string]time.Time
}

// New returns the complete handler with logging and security headers.
func New(store *library.Store, log *slog.Logger, options Options) (http.Handler, error) {
	h := &handler{store: store, log: log, pages: map[string]*template.Template{}, options: options, seen: map[string]time.Time{}}
	base, err := template.ParseFS(templateFiles, "templates/layout.html", "templates/components.html")
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"document", "folder", "search", "problems", "status", "gallery", "profile", "object"} {
		page, err := template.Must(base.Clone()).ParseFS(templateFiles, "templates/"+name+".html")
		if err != nil {
			return nil, err
		}
		h.pages[name] = page
	}
	if h.pages["signedout"], err = template.ParseFS(templateFiles, "templates/signedout.html"); err != nil {
		return nil, err
	}
	if h.pages["app"], err = template.ParseFS(templateFiles, "templates/app.html"); err != nil {
		return nil, err
	}
	if h.assetVersion, err = hashAssets(); err != nil {
		return nil, err
	}
	assets, err := fs.Sub(assetFiles, "assets")
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	// The workspace of the mockup renders itself in the browser and reads its data from the API (sprint 2026-11-08).
	// The pages rendered on the server stay under /read/ for reading without JavaScript.
	mux.HandleFunc("GET /{$}", h.app)
	mux.HandleFunc("GET /o/{id...}", h.app)
	mux.HandleFunc("GET /f/{path...}", h.app)
	mux.HandleFunc("GET /profile", h.app)
	mux.HandleFunc("GET /api/state", h.apiState)
	mux.HandleFunc("GET /api/doc/{id...}", h.apiDocument)
	mux.HandleFunc("GET /api/search", h.apiSearch)
	mux.HandleFunc("POST /api/collections", h.apiCreateCollection)
	mux.HandleFunc("PATCH /api/objects/{id}", h.apiRename)
	mux.HandleFunc("GET /read/{$}", h.home)
	mux.HandleFunc("GET /read/o/{id...}", h.document)
	// Links from before the object addresses of ADR-18 keep working.
	mux.HandleFunc("GET /d/{id...}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, route.Object(r.PathValue("id")), http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /read/profile", h.profile)
	mux.HandleFunc("POST /collections", h.createCollection)
	mux.HandleFunc("GET /read/f/{path...}", h.folder)
	mux.HandleFunc("GET /files/{path...}", h.file)
	mux.HandleFunc("GET /search", h.search)
	mux.HandleFunc("GET /problems", h.problems)
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", staticAssets(http.FileServerFS(assets))))
	mux.HandleFunc("GET /assets/logo.svg", serveBytes("image/svg+xml", docs.Logo))
	mux.HandleFunc("GET /favicon.ico", serveBytes("image/vnd.microsoft.icon", docs.Favicon))
	if options.DevUI {
		mux.HandleFunc("GET /dev/ui", h.gallery)
	}
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) { h.notFound(w, r) })
	return accessLog(log, secure(h.signedIn(mux))), nil
}

// signedIn lets a request through only with a user from the gate. Assets stay open, so the page that says so has its
// styles. Without the gate in front, nothing else answers (M-28).
func (h *handler) signedIn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") || r.URL.Path == "/favicon.ico" {
			next.ServeHTTP(w, r)
			return
		}
		user, ok := account.FromRequest(r)
		if !ok && h.options.DevUser != nil {
			user, ok = *h.options.DevUser, true
		}
		if !ok {
			var out bytes.Buffer
			if err := h.pages["signedout"].ExecuteTemplate(&out, "signedout", h.assetVersion); err != nil {
				http.Error(w, "not signed in", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write(out.Bytes())
			return
		}
		h.seeAccount(r.Context(), user)
		next.ServeHTTP(w, r.WithContext(account.With(r.Context(), user)))
	})
}

// seeAccount writes the account at its first request and then at most every few minutes (ADR-20: the owner of what
// someone makes must exist). A database that does not answer is logged; reading goes on.
func (h *handler) seeAccount(ctx context.Context, user account.User) {
	if h.options.DB == nil {
		return
	}
	h.seenMu.Lock()
	last, ok := h.seen[user.ID]
	if ok && time.Since(last) < 5*time.Minute {
		h.seenMu.Unlock()
		return
	}
	h.seen[user.ID] = time.Now()
	h.seenMu.Unlock()
	if err := h.options.DB.SeeAccount(ctx, user); err != nil {
		h.log.Error("account not written", slog.String("error", err.Error()))
		h.seenMu.Lock()
		delete(h.seen, user.ID)
		h.seenMu.Unlock()
	}
}

// sameOrigin lets a change through only from a page of this application. The gate signs in with a cookie, so a form on
// another site could otherwise post here in someone's name.
func sameOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site == "same-origin"
	}
	origin := r.Header.Get("Origin")
	return origin != "" && strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://") == r.Host
}

// createCollection makes a collection from the form in nav2, at the top or inside another, and opens it.
func (h *handler) createCollection(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		http.Error(w, "collections need the database", http.StatusNotImplemented)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "only from this application", http.StatusForbidden)
		return
	}
	user, _ := account.From(r.Context())
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	created, err := h.options.DB.CreateCollection(r.Context(), user, r.PostForm.Get("title"), r.PostForm.Get("parent"))
	switch {
	case errors.Is(err, db.ErrTitle):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case errors.Is(err, db.ErrNotFound):
		h.notFound(w, r)
		return
	case err != nil:
		h.log.Error("collection not made", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, route.Object(created.ID), http.StatusSeeOther)
}

func hashAssets() (string, error) {
	hash := sha256.New()
	err := fs.WalkDir(assetFiles, "assets", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := assetFiles.ReadFile(p)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(hash, "%s %d\n", p, len(content))
		_, _ = hash.Write(content)
		return nil
	})
	return hex.EncodeToString(hash.Sum(nil))[:12], err
}

func staticAssets(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400") // URLs carry a version
		next.ServeHTTP(w, r)
	})
}

func serveBytes(contentType string, content []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		if contentType == "image/svg+xml" {
			w.Header().Set("Content-Security-Policy", imageCSP)
		}
		_, _ = w.Write(content)
	}
}

func (h *handler) render(w http.ResponseWriter, name string, status int, view any) {
	var out bytes.Buffer
	if err := h.pages[name].ExecuteTemplate(&out, "layout", view); err != nil {
		h.log.Error("template failed", slog.String("template", name), slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache") // a change on disk has to show on the next view
	w.WriteHeader(status)
	_, _ = w.Write(out.Bytes())
}

// app answers every address of the workspace with the same page; the browser shows the object the address names.
func (h *handler) app(w http.ResponseWriter, r *http.Request) {
	if !h.exists(r) {
		h.notFound(w, r)
		return
	}
	var out bytes.Buffer
	if err := h.pages["app"].ExecuteTemplate(&out, "app", h.assetVersion); err != nil {
		h.log.Error("template failed", slog.String("template", "app"), slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(out.Bytes())
}

func (h *handler) home(w http.ResponseWriter, r *http.Request) {
	if doc, ok := h.store.Library().Home(); ok {
		http.Redirect(w, r, route.Document(string(doc.ID)), http.StatusFound)
		return
	}
	http.Redirect(w, r, route.Folder(""), http.StatusFound)
}

func (h *handler) document(w http.ResponseWriter, r *http.Request) {
	if h.options.DB != nil && id.Valid(r.PathValue("id")) {
		if h.object(w, r) {
			return
		}
	}
	lib := h.store.Library()
	doc, ok := lib.Document(document.ID(r.PathValue("id")))
	if !ok {
		h.notFound(w, r)
		return
	}
	folder, _ := lib.Tree().Location(doc.ID)
	view := documentView{
		shellView: h.makeShell(r, lib, doc.Title, doc.ID, nil),
		// The renderer drops raw HTML and unsafe links, which the tests of package markdown prove.
		Body:      template.HTML(withoutFirstHeading(doc.HTML, doc.StartsWithTitle)), //nolint:gosec // see above
		ID:        string(doc.ID),
		Address:   route.Object(string(doc.ID)),
		Source:    doc.Source,
		Modified:  doc.Modified.Format("2006-01-02 15:04"),
		PathBased: !doc.Stable,
	}
	view.Description = doc.Description
	view.Tags = makeTags(doc.Tags)
	// The way starts at the collection, as in the mockup: the root is no place of its own.
	if view.Crumbs = makeCrumbs(folder); len(view.Crumbs) > 1 {
		view.Crumbs = view.Crumbs[1:]
	}
	words := len(strings.Fields(doc.Text))
	view.Status = fmt.Sprintf("%d words · %d characters · %d min read", words, len([]rune(doc.Text)), max(1, (words+199)/200))
	if len(doc.Contents) >= minContentsEntries {
		for _, heading := range doc.Contents {
			view.Contents = append(view.Contents, contentsItem{Label: heading.Text, Href: "#" + heading.Anchor, Level: heading.Level})
		}
	}
	for _, id := range doc.Backlinks {
		other, _ := lib.Document(id)
		view.Backlinks = append(view.Backlinks, linkItem{Label: other.Title, Href: route.Document(string(id)), Detail: other.Source})
	}
	h.render(w, "document", http.StatusOK, view)
}

// withoutFirstHeading drops the heading a page opens with: the title stands in the head of the window already (ADR-16).
func withoutFirstHeading(body string, startsWithTitle bool) string {
	if !startsWithTitle {
		return body
	}
	trimmed := strings.TrimLeft(body, " \t\r\n")
	if !strings.HasPrefix(trimmed, "<h1") {
		return body
	}
	end := strings.Index(trimmed, "</h1>")
	if end < 0 {
		return body
	}
	return trimmed[end+len("</h1>"):]
}

func (h *handler) folder(w http.ResponseWriter, r *http.Request) {
	lib := h.store.Library()
	folder, ok := lib.Tree().Folder(r.PathValue("path"))
	if !ok {
		h.notFound(w, r)
		return
	}
	title := folder.Name
	if title == "" {
		title = "Home"
	}
	view := folderView{shellView: h.makeShell(r, lib, title, "", folder), Empty: lib.Len() == 0}
	// The way to a folder, without the root and without the folder itself, which is the title (ADR-16).
	if crumbs := makeCrumbs(folder); len(crumbs) > 2 {
		view.Crumbs = crumbs[1 : len(crumbs)-1]
	}
	if folder == lib.Tree().Root() {
		for _, doc := range lib.Recent(maxRecentDocuments) {
			view.Recent = append(view.Recent, linkItem{Label: doc.Title, Href: route.Document(string(doc.ID)), Detail: doc.Modified.Format("2006-01-02 15:04")})
		}
	}
	for _, sub := range folder.Folders {
		view.Folders = append(view.Folders, linkItem{Label: sub.Name, Href: route.Folder(sub.Path), Detail: "Folder", Folder: true})
	}
	for _, id := range folder.Documents {
		doc, _ := lib.Document(id)
		view.Documents = append(view.Documents, linkItem{Label: doc.Title, Href: route.Document(string(id)), Detail: "Page"})
	}
	h.render(w, "folder", http.StatusOK, view)
}

func (h *handler) file(w http.ResponseWriter, r *http.Request) {
	imagePath := r.PathValue("path")
	contentType, known := imageTypes[strings.ToLower(path.Ext(imagePath))]
	if !known || !h.store.Library().HasImage(imagePath) {
		h.notFound(w, r)
		return
	}
	file, modified, err := h.store.OpenImage(imagePath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			h.log.Error("image failed", slog.String("path", imagePath), slog.String("error", err.Error()))
		}
		h.notFound(w, r)
		return
	}
	defer func() { _ = file.Close() }()
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Security-Policy", imageCSP)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "", modified, file)
}

func (h *handler) search(w http.ResponseWriter, r *http.Request) {
	lib := h.store.Library()
	query := r.URL.Query().Get("q")
	view := searchView{shellView: h.makeShell(r, lib, "Search", "", nil), Searched: strings.TrimSpace(query) != ""}
	view.Query = query
	view.View = "search"
	if tag := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(r.URL.Query().Get("tag")), "#")); tag != "" {
		view.Title, view.Tag = "#"+tag, tag
		for _, doc := range lib.Tagged(tag) {
			view.Tagged = append(view.Tagged, linkItem{Label: doc.Title, Href: route.Document(string(doc.ID)), Detail: doc.Source})
		}
	}
	for _, hit := range lib.Search(query, maxSearchResults) {
		view.Results = append(view.Results, searchResult{
			linkItem: linkItem{Label: hit.Document.Title, Href: route.Document(string(hit.Document.ID)), Detail: hit.Document.Source},
			Snippet:  hit.Snippet,
		})
	}
	h.render(w, "search", http.StatusOK, view)
}

func (h *handler) problems(w http.ResponseWriter, r *http.Request) {
	lib := h.store.Library()
	h.render(w, "problems", http.StatusOK, problemsView{shellView: h.makeShell(r, lib, "Problems", "", nil), Problems: lib.Problems()})
}

// profile shows the account from the gate and the settings, and the administration to administrators (ADR-16).
// The sections of the profile: settings for everyone, the administration set apart for administrators (ADR-16).
var (
	profileSettings = []profileSection{
		{ID: "account", Label: "Account", Icon: "user"}, {ID: "appearance", Label: "Appearance", Icon: "sun"},
		{ID: "navigation", Label: "Navigation", Icon: "panel"}, {ID: "language", Label: "Language", Icon: "globe", Later: "comes with the translation"},
		{ID: "notifications", Label: "Notifications", Icon: "mail", Later: "comes with M-16"},
	}
	profileAdmin = []profileSection{
		{ID: "modules", Label: "Modules", Icon: "list", Later: "comes with the module switches"}, {ID: "users", Label: "Users and groups", Icon: "user", Later: "comes with M-5"},
		{ID: "sources", Label: "Sources", Icon: "db", Later: "comes with M-4"}, {ID: "storage", Label: "Storage and backup", Icon: "drive", Later: "comes with M-2"},
		{ID: "signin", Label: "Sign-in", Icon: "lock", Later: "the gate in front decides it"}, {ID: "audit", Label: "Audit log", Icon: "list", Later: "comes with M-2 and M-5"},
	}
)

// profile opens the profile view: its sections in nav2, the chosen one in nav3. The window keeps what it showed, since
// the shell swaps only the columns; opened directly, the window says where things are.
func (h *handler) profile(w http.ResponseWriter, r *http.Request) {
	view := profileView{shellView: h.makeShell(r, h.store.Library(), "Profile", "", nil)}
	view.View = "profile"
	view.Open = nil
	section := r.URL.Query().Get("s")
	state := &profileState{}
	pick := func(list []profileSection) []profileSection {
		out := make([]profileSection, len(list))
		for i, s := range list {
			s.Href = route.Profile() + "?s=" + s.ID
			s.Current = s.ID == section
			out[i] = s
		}
		return out
	}
	admin := h.options.AdminGroup != "" && slices.Contains(view.User.Groups, h.options.AdminGroup)
	// Only a built section can be chosen. The administration is not built yet, so it never is.
	known := slices.ContainsFunc(profileSettings, func(s profileSection) bool { return s.ID == section && s.Later == "" })
	if !known {
		section = "account"
	}
	state.Section = section
	state.Settings = pick(profileSettings)
	if admin {
		state.Admin = pick(profileAdmin)
	}
	view.Profile = state
	h.render(w, "profile", http.StatusOK, view)
}

func (h *handler) notFound(w http.ResponseWriter, r *http.Request) {
	lib := h.store.Library()
	h.render(w, "status", http.StatusNotFound, statusView{
		shellView: h.makeShell(r, lib, "Page not found", "", nil),
		Heading:   "Page not found",
		Message:   "It may have moved. Pages without an id in their front matter get a new address when they move.",
	})
}

var accessLabels = map[string]string{"edit": "can edit", "comment": "can comment", "read": "can only read"}

// object shows an object of the database, so far a collection: its name and description in the head, its owner, what
// others may do, who sees it and its entries (ADR-16). It reports false if there is no such object for the viewer, so
// the folder source gets its turn.
func (h *handler) object(w http.ResponseWriter, r *http.Request) bool {
	ctx := r.Context()
	user, _ := account.From(ctx)
	o, err := h.options.DB.Object(ctx, r.PathValue("id"), user.ID)
	if errors.Is(err, db.ErrNotFound) {
		return false
	}
	if err != nil {
		h.log.Error("object failed", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return true
	}
	view := objectView{shellView: h.makeShell(r, h.store.Library(), o.Title, "", nil)}
	view.Description = o.Description
	view.ID = o.ID
	view.Kind = map[string]string{db.KindCollection: "Collection", db.KindFolder: "Folder", db.KindPage: "Page"}[o.Kind]
	view.Owner = h.options.DB.AccountName(ctx, o.Owner)
	access, err := h.options.DB.Access(ctx, o.ID)
	if err != nil {
		h.log.Error("access failed", slog.String("error", err.Error()))
	}
	view.Access = accessLabels[access]
	view.Inherited = o.Access == ""
	view.Visible = "Everyone signed in"
	if top, err := h.options.DB.Top(ctx, o.ID, user.ID); err == nil && top.Private {
		view.Visible = "Only you"
	}
	children, err := h.options.DB.Children(ctx, o.ID, user.ID)
	if err != nil {
		h.log.Error("children failed", slog.String("error", err.Error()))
	}
	for _, child := range children {
		view.Entries = append(view.Entries, linkItem{Label: child.Title, Href: route.Object(child.ID), Detail: view.kindOf(child.Kind), Folder: child.Kind != db.KindPage})
	}
	h.openFromDatabase(r, &view.shellView, o.ID)
	h.render(w, "object", http.StatusOK, view)
	return true
}

// exists tells whether the address of the workspace names something the viewer may see, so an unknown address is a 404
// and not an empty workspace.
func (h *handler) exists(r *http.Request) bool {
	switch {
	case strings.HasPrefix(r.URL.Path, "/o/"):
		key := r.PathValue("id")
		if _, ok := h.store.Library().Document(document.ID(key)); ok {
			return true
		}
		if h.options.DB == nil || !id.Valid(key) {
			return false
		}
		user, _ := account.From(r.Context())
		_, err := h.options.DB.Object(r.Context(), key, user.ID)
		return err == nil
	case strings.HasPrefix(r.URL.Path, "/f/"):
		_, ok := h.store.Library().Tree().Folder(r.PathValue("path"))
		return ok
	}
	return true
}
