// SPDX-License-Identifier: MIT

// Package web serves the workspace and the API it reads. Everyone who gets past the sign-in gate in front of it can read
// what is not private.
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
	maxSearchResults = 50
	// homeLabel names the root of the folder source in the workspace.
	homeLabel = "Home"
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
	// DevUI turns on /dev/ui, the workspace with only the made-up content of the mockup, every component in use.
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
	pages        *template.Template
	assetVersion string
	options      Options
	// seen keeps when an account was last written, so not every request writes it.
	seenMu sync.Mutex
	seen   map[string]time.Time
}

// New returns the complete handler with logging and security headers.
func New(store *library.Store, log *slog.Logger, options Options) (http.Handler, error) {
	h := &handler{store: store, log: log, options: options, seen: map[string]time.Time{}}
	var err error
	if h.pages, err = template.ParseFS(templateFiles, "templates/*.html"); err != nil {
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
	// The workspace renders itself in the browser, as the approved mockup does, and reads its data from the API (ADR-16).
	for _, pattern := range []string{"GET /{$}", "GET /o/{id...}", "GET /f/{path...}", "GET /profile"} {
		mux.HandleFunc(pattern, h.app)
	}
	mux.HandleFunc("GET /api/state", h.apiState)
	mux.HandleFunc("GET /api/doc/{id...}", h.apiDocument)
	mux.HandleFunc("GET /api/search", h.apiSearch)
	mux.HandleFunc("POST /api/collections", h.apiCreateCollection)
	mux.HandleFunc("PATCH /api/objects/{id}", h.apiRename)
	// Links from before the object addresses of ADR-18 keep working.
	mux.HandleFunc("GET /d/{id...}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, route.Object(r.PathValue("id")), http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /files/{path...}", h.file)
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", staticAssets(http.FileServerFS(assets))))
	mux.HandleFunc("GET /assets/logo.svg", serveBytes("image/svg+xml", docs.Logo))
	mux.HandleFunc("GET /favicon.ico", serveBytes("image/vnd.microsoft.icon", docs.Favicon))
	if options.DevUI {
		mux.HandleFunc("GET /dev/ui", h.app)
	}
	mux.HandleFunc("GET /", h.notFound)
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
			h.message(w, http.StatusUnauthorized, message{
				Title: "Not signed in",
				Text:  "UnderDoc is reached through its sign-in gate, which tells it who you are. This request came without it.",
				Link:  "Try again",
			})
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

// sameOrigin lets a change through only from a page of this application. The gate signs in with a cookie, so a page on
// another site could otherwise send changes here in someone's name.
func sameOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site == "same-origin"
	}
	origin := r.Header.Get("Origin")
	return origin != "" && strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://") == r.Host
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

// appView fills the page of the workspace. Demo shows only the made-up content of the mockup, for /dev/ui.
type appView struct {
	Version string
	Demo    bool
}

// app answers every address of the workspace with the same page; the browser shows the object the address names.
func (h *handler) app(w http.ResponseWriter, r *http.Request) {
	if !h.exists(r) {
		h.notFound(w, r)
		return
	}
	var out bytes.Buffer
	if err := h.pages.ExecuteTemplate(&out, "app", appView{Version: h.assetVersion, Demo: r.URL.Path == "/dev/ui"}); err != nil {
		h.log.Error("template failed", slog.String("template", "app"), slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(out.Bytes())
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

// message is a page of its own for what the workspace cannot show: no sign-in, nothing at this address.
type message struct {
	Title, Text, Link string
	Version           string
}

func (h *handler) message(w http.ResponseWriter, status int, m message) {
	m.Version = h.assetVersion
	var out bytes.Buffer
	if err := h.pages.ExecuteTemplate(&out, "message", m); err != nil {
		h.log.Error("template failed", slog.String("template", "message"), slog.String("error", err.Error()))
		http.Error(w, m.Title, status)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(out.Bytes())
}

func (h *handler) notFound(w http.ResponseWriter, _ *http.Request) {
	h.message(w, http.StatusNotFound, message{
		Title: "Page not found",
		Text:  "It may have moved. Pages without an id in their front matter get a new address when they move.",
		Link:  "To the start",
	})
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
