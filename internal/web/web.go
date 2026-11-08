// SPDX-License-Identifier: MIT

// Package web is the read-only web interface. Everyone who gets past the sign-in gate in front of it can read everything.
package web

import (
	"bytes"
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

	"github.com/Ollornog/UnderDoc/docs"
	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/document"
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
}

type handler struct {
	store        *library.Store
	log          *slog.Logger
	pages        map[string]*template.Template
	assetVersion string
	options      Options
}

// New returns the complete handler with logging and security headers.
func New(store *library.Store, log *slog.Logger, options Options) (http.Handler, error) {
	h := &handler{store: store, log: log, pages: map[string]*template.Template{}, options: options}
	base, err := template.ParseFS(templateFiles, "templates/layout.html", "templates/components.html")
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"document", "folder", "search", "problems", "status", "gallery", "profile"} {
		page, err := template.Must(base.Clone()).ParseFS(templateFiles, "templates/"+name+".html")
		if err != nil {
			return nil, err
		}
		h.pages[name] = page
	}
	if h.pages["signedout"], err = template.ParseFS(templateFiles, "templates/signedout.html"); err != nil {
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
	mux.HandleFunc("GET /{$}", h.home)
	mux.HandleFunc("GET /o/{id...}", h.document)
	// Links from before the object addresses of ADR-18 keep working.
	mux.HandleFunc("GET /d/{id...}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, route.Object(r.PathValue("id")), http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /profile", h.profile)
	mux.HandleFunc("GET /f/{path...}", h.folder)
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
		next.ServeHTTP(w, r.WithContext(account.With(r.Context(), user)))
	})
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

func (h *handler) home(w http.ResponseWriter, r *http.Request) {
	if doc, ok := h.store.Library().Home(); ok {
		http.Redirect(w, r, route.Document(string(doc.ID)), http.StatusFound)
		return
	}
	http.Redirect(w, r, route.Folder(""), http.StatusFound)
}

func (h *handler) document(w http.ResponseWriter, r *http.Request) {
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
	view.Crumbs = makeCrumbs(folder)
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
	if crumbs := makeCrumbs(folder); folder != lib.Tree().Root() {
		view.Crumbs = crumbs[:len(crumbs)-1]
	}
	if folder == lib.Tree().Root() {
		for _, doc := range lib.Recent(maxRecentDocuments) {
			view.Recent = append(view.Recent, linkItem{Label: doc.Title, Href: route.Document(string(doc.ID)), Detail: doc.Modified.Format("2006-01-02 15:04")})
		}
	}
	for _, sub := range folder.Folders {
		view.Folders = append(view.Folders, linkItem{Label: sub.Name, Href: route.Folder(sub.Path)})
	}
	for _, id := range folder.Documents {
		doc, _ := lib.Document(id)
		view.Documents = append(view.Documents, linkItem{Label: doc.Title, Href: route.Document(string(id)), Detail: doc.Modified.Format("2006-01-02 15:04")})
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
