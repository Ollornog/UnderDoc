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
	"strings"

	"github.com/Ollornog/UnderDoc/docs"
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
}

type handler struct {
	store        *library.Store
	log          *slog.Logger
	pages        map[string]*template.Template
	assetVersion string
}

// New returns the complete handler with logging and security headers.
func New(store *library.Store, log *slog.Logger, options Options) (http.Handler, error) {
	h := &handler{store: store, log: log, pages: map[string]*template.Template{}}
	base, err := template.ParseFS(templateFiles, "templates/layout.html", "templates/components.html")
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"document", "folder", "search", "problems", "status", "gallery"} {
		page, err := template.Must(base.Clone()).ParseFS(templateFiles, "templates/"+name+".html")
		if err != nil {
			return nil, err
		}
		h.pages[name] = page
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
	mux.HandleFunc("GET /d/{id...}", h.document)
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
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) { h.notFound(w) })
	return accessLog(log, secure(mux)), nil
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
		h.notFound(w)
		return
	}
	folder, _ := lib.Tree().Location(doc.ID)
	view := documentView{
		shellView: h.makeShell(lib, doc.Title, doc.ID, nil),
		ShowTitle: !doc.StartsWithTitle,
		// The renderer drops raw HTML and unsafe links, which the tests of package markdown prove.
		Body:      template.HTML(doc.HTML), //nolint:gosec // see above
		Source:    doc.Source,
		Modified:  doc.Modified.Format("2006-01-02 15:04"),
		PathBased: !doc.Stable,
	}
	view.Crumbs = append(makeCrumbs(folder), crumb{Label: doc.Title})
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

func (h *handler) folder(w http.ResponseWriter, r *http.Request) {
	lib := h.store.Library()
	folder, ok := lib.Tree().Folder(r.PathValue("path"))
	if !ok {
		h.notFound(w)
		return
	}
	title := folder.Name
	if title == "" {
		title = "Home"
	}
	view := folderView{shellView: h.makeShell(lib, title, "", folder), Empty: lib.Len() == 0}
	view.Crumbs = makeCrumbs(folder)
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
		h.notFound(w)
		return
	}
	file, modified, err := h.store.OpenImage(imagePath)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			h.log.Error("image failed", slog.String("path", imagePath), slog.String("error", err.Error()))
		}
		h.notFound(w)
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
	view := searchView{shellView: h.makeShell(lib, "Search", "", nil), Searched: strings.TrimSpace(query) != ""}
	view.Query = query
	for _, hit := range lib.Search(query, maxSearchResults) {
		view.Results = append(view.Results, searchResult{
			linkItem: linkItem{Label: hit.Document.Title, Href: route.Document(string(hit.Document.ID)), Detail: hit.Document.Source},
			Snippet:  hit.Snippet,
		})
	}
	h.render(w, "search", http.StatusOK, view)
}

func (h *handler) problems(w http.ResponseWriter, _ *http.Request) {
	lib := h.store.Library()
	h.render(w, "problems", http.StatusOK, problemsView{shellView: h.makeShell(lib, "Problems", "", nil), Problems: lib.Problems()})
}

func (h *handler) notFound(w http.ResponseWriter) {
	lib := h.store.Library()
	h.render(w, "status", http.StatusNotFound, statusView{
		shellView: h.makeShell(lib, "Not found", "", nil),
		Heading:   "Page not found",
		Message:   "It may have moved. Pages without an id in their front matter get a new address when they move.",
	})
}
