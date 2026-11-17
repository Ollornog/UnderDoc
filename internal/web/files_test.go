// SPDX-License-Identifier: MIT

package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/store"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

type filesSite struct {
	*site
	db    *db.DB
	store *store.Store
}

func withFiles(t *testing.T) *filesSite {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	database.SetLimits(db.Limits{MaxFile: 1 << 20, User: 2 << 20})
	return &filesSite{site: newSite(t, Options{DB: database, Store: s, AdminGroup: "underdoc-admins"}, sample), db: database, store: s}
}

type rootsAnswer struct {
	Roots []apiRoot `json:"roots"`
	Admin bool      `json:"admin"`
}

func (s *filesSite) root(headers map[string]string, role string) apiRoot {
	s.t.Helper()
	got := decode[rootsAnswer](s.t, s.getWith("/api/files", merged(headers)))
	for _, r := range got.Roots {
		if r.Role == role {
			return r
		}
	}
	s.t.Fatalf("no root %s in %+v", role, got.Roots)
	return apiRoot{}
}

// raw sends a request with a body of bytes, as the browser sends a chunk.
func (s *filesSite) raw(method, target string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	s.t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	for name, value := range merged(sameSite) {
		request.Header.Set(name, value)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	if host := request.Header.Get("Host"); host != "" {
		request.Host = host
	}
	s.http.ServeHTTP(recorder, request)
	return recorder
}

// put uploads content in chunks of the given size and returns the answer of the finish.
func (s *filesSite) put(headers map[string]string, target, name string, content []byte, chunk int) *httptest.ResponseRecorder {
	s.t.Helper()
	start := s.send(http.MethodPost, "/api/uploads", fmt.Sprintf(`{%s,"name":%q,"size":%d}`, target, name, len(content)), merged2(sameSite, headers))
	if start.Code != http.StatusCreated {
		return start
	}
	u := decode[apiUpload](s.t, start)
	for off := 0; off < len(content); off += chunk {
		end := min(off+chunk, len(content))
		got := s.raw(http.MethodPatch, "/api/uploads/"+u.ID, content[off:end], merged2(headers, map[string]string{"Upload-Offset": fmt.Sprint(off)}))
		if got.Code != http.StatusOK {
			return got
		}
	}
	return s.raw(http.MethodPost, "/api/uploads/"+u.ID+"/finish", nil, headers)
}

func merged2(a, b map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

type finished struct {
	ID      string `json:"id"`
	Title   string `json:"n"`
	Version int    `json:"version"`
	Type    string `json:"type"`
	URL     string `json:"url"`
	Attach  string `json:"attachment"`
}

func TestUploadInChunksAndResume(t *testing.T) {
	s := withFiles(t)
	mine := s.root(nil, db.RootUser)
	if mine.Title != db.MyFiles || !mine.Private || mine.Limit != 2<<20 {
		t.Fatalf("%+v", mine)
	}
	content := []byte(strings.Repeat("0123456789", 3))
	start := s.send(http.MethodPost, "/api/uploads", fmt.Sprintf(`{"parent":%q,"name":"notes.txt","size":30}`, mine.ID), sameSite)
	u := decode[apiUpload](t, start)
	if start.Code != http.StatusCreated || u.Received != 0 || u.Chunk != MaxChunk {
		t.Fatalf("%d %+v", start.Code, u)
	}
	if got := s.raw(http.MethodPatch, "/api/uploads/"+u.ID, content[:12], map[string]string{"Upload-Offset": "0"}); got.Code != http.StatusOK {
		t.Fatalf("first chunk: %d %s", got.Code, got.Body)
	}
	// A chunk at the wrong place is refused with where the upload stands, and the browser resumes from there.
	got := s.raw(http.MethodPatch, "/api/uploads/"+u.ID, content[5:], map[string]string{"Upload-Offset": "5"})
	if got.Code != http.StatusConflict || decode[apiUpload](t, got).Received != 12 {
		t.Fatalf("a chunk at the wrong offset: %d %s", got.Code, got.Body)
	}
	if state := decode[apiUpload](t, s.get("/api/uploads/"+u.ID)); state.Received != 12 || state.Size != 30 {
		t.Fatalf("state %+v", state)
	}
	if got := s.raw(http.MethodPost, "/api/uploads/"+u.ID+"/finish", nil, nil); got.Code != http.StatusBadRequest {
		t.Fatalf("an incomplete upload finished: %d", got.Code)
	}
	if got := s.raw(http.MethodPatch, "/api/uploads/"+u.ID, content[12:], map[string]string{"Upload-Offset": "12"}); got.Code != http.StatusOK {
		t.Fatalf("second chunk: %d %s", got.Code, got.Body)
	}
	done := decode[finished](t, s.raw(http.MethodPost, "/api/uploads/"+u.ID+"/finish", nil, nil))
	if done.Version != 1 || done.Type != "text/plain" || done.Title != "notes.txt" {
		t.Fatalf("%+v", done)
	}
	body := s.get(db.FilePath(done.ID))
	if body.Body.String() != string(content) || body.Header().Get("Content-Type") != "text/plain" ||
		!strings.Contains(body.Header().Get("Content-Security-Policy"), "sandbox") || body.Header().Get("X-Content-Type-Options") != "nosniff" ||
		!strings.HasPrefix(body.Header().Get("Content-Disposition"), "inline") {
		t.Fatalf("content: %q %v", body.Body.String(), body.Header())
	}
	// The data of the upload is gone from the store, the blob is there once.
	if entries, _ := os.ReadDir(filepath.Join(s.store.Root(), store.DefaultTenant, "uploads")); len(entries) != 0 {
		t.Fatalf("uploads left: %v", entries)
	}
	// Uploaded again under the same name, in small chunks: a new version.
	second := decode[finished](t, s.put(nil, fmt.Sprintf(`"parent":%q`, mine.ID), "Notes.TXT", []byte("changed"), 3))
	if second.ID != done.ID || second.Version != 2 {
		t.Fatalf("%+v", second)
	}
	if got := s.get(db.FilePath(done.ID)).Body.String(); got != "changed" {
		t.Fatalf("current: %q", got)
	}
	if got := s.get(db.FilePath(done.ID) + "?v=1").Body.String(); got != string(content) {
		t.Fatalf("version 1: %q", got)
	}
	if got := s.raw(http.MethodPost, "/api/files/"+done.ID+"/versions/1/restore", nil, nil); got.Code != http.StatusOK {
		t.Fatalf("restore: %d %s", got.Code, got.Body)
	}
	file := decode[apiFile](t, s.get("/api/files/"+done.ID))
	if file.Versions != 3 || len(file.History) != 3 || file.Size != 30 || !file.CanEdit || file.Content != db.FilePath(done.ID) {
		t.Fatalf("%+v", file)
	}
	list := decode[struct {
		Entries []apiFileEntry `json:"entries"`
	}](t, s.get("/api/files/"+mine.ID+"/list"))
	if len(list.Entries) != 1 || list.Entries[0].ID != done.ID {
		t.Fatalf("%+v", list)
	}
	// An empty file is a file too.
	if empty := decode[finished](t, s.put(nil, fmt.Sprintf(`"parent":%q`, mine.ID), "empty.txt", nil, 1)); empty.Version != 1 {
		t.Fatalf("%+v", empty)
	}
	// Cancelled, an upload leaves nothing.
	u = decode[apiUpload](t, s.send(http.MethodPost, "/api/uploads", fmt.Sprintf(`{"parent":%q,"name":"x","size":3}`, mine.ID), sameSite))
	s.raw(http.MethodPatch, "/api/uploads/"+u.ID, []byte("ab"), map[string]string{"Upload-Offset": "0"})
	if got := s.raw(http.MethodDelete, "/api/uploads/"+u.ID, nil, nil); got.Code != http.StatusNoContent {
		t.Fatalf("cancel: %d", got.Code)
	}
	if entries, _ := os.ReadDir(filepath.Join(s.store.Root(), store.DefaultTenant, "uploads")); len(entries) != 0 {
		t.Fatalf("uploads left after cancel: %v", entries)
	}
}

// TestUploadAttacks is the attack round of the uploads (2026-11-17): every way in that a hostile user or page might try.
func TestUploadAttacks(t *testing.T) {
	s := withFiles(t)
	mine := s.root(nil, db.RootUser)
	parent := fmt.Sprintf(`"parent":%q`, mine.ID)

	t.Run("path traversal in names and IDs", func(t *testing.T) {
		for _, name := range []string{"../../etc/passwd", `..\..\boot.ini`, "..", "a/../../b", "x\x00.png"} {
			if got := s.send(http.MethodPost, "/api/uploads", fmt.Sprintf(`{%s,"name":%q,"size":1}`, parent, name), sameSite); got.Code != http.StatusBadRequest {
				t.Errorf("name %q: %d", name, got.Code)
			}
		}
		for _, id := range []string{"..%2F..%2Fetc", "0123456789abcdef0123456789abcdeg", "../uploads"} {
			if got := s.raw(http.MethodPatch, "/api/uploads/"+id, []byte("x"), map[string]string{"Upload-Offset": "0"}); got.Code != http.StatusNotFound && got.Code != http.StatusTemporaryRedirect {
				t.Errorf("upload ID %q: %d", id, got.Code)
			}
		}
		// Whatever the name, nothing lands outside the store, and nothing in it carries a name a user gave.
		s.put(nil, parent, "...", []byte("x"), 1)
		_ = filepath.WalkDir(s.store.Root(), func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && !store.ValidHash(d.Name()) {
				t.Errorf("a file of another name in the store: %s", p)
			}
			return nil
		})
	})

	t.Run("content type spoofing", func(t *testing.T) {
		png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
		html := []byte("<!DOCTYPE html><script>alert(1)</script>")
		cases := []struct {
			name    string
			content []byte
			stored  string
			served  string
			inline  bool
		}{
			{"photo.html", png, "image/png", "image/png", true},
			{"photo.png", html, "text/html", "application/octet-stream", false},
			{"photo.txt", html, "text/html", "application/octet-stream", false},
			{"logo.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), "image/svg+xml", "application/octet-stream", false},
			{"logo.png", []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`), "text/xml", "application/octet-stream", false},
			{"report.pdf", []byte("%PDF-1.7\n"), "application/pdf", "application/octet-stream", false},
		}
		for _, c := range cases {
			got := s.put(map[string]string{"Content-Type": "image/png"}, parent, c.name, c.content, 4)
			done := decode[finished](t, got)
			if done.Type != c.stored {
				t.Errorf("%s stored as %s, want %s", c.name, done.Type, c.stored)
			}
			served := s.get(db.FilePath(done.ID))
			h := served.Header()
			if h.Get("Content-Type") != c.served || strings.HasPrefix(h.Get("Content-Disposition"), "inline") != c.inline ||
				!strings.Contains(h.Get("Content-Security-Policy"), "sandbox") || h.Get("X-Content-Type-Options") != "nosniff" {
				t.Errorf("%s served as %v", c.name, h)
			}
			if forced := s.get(db.FilePath(done.ID) + "?download=1"); !strings.HasPrefix(forced.Header().Get("Content-Disposition"), "attachment") {
				t.Errorf("%s: ?download is not an attachment", c.name)
			}
		}
	})

	t.Run("an archive is never unpacked", func(t *testing.T) {
		// A zip bomb is only a danger to what unpacks it; storing and serving keep its bytes as they are.
		zip := append([]byte("PK\x03\x04"), bytes.Repeat([]byte{0}, 100)...)
		done := decode[finished](t, s.put(nil, parent, "bomb.zip", zip, 50))
		if done.Type != "application/zip" {
			t.Errorf("stored as %s", done.Type)
		}
		if got := s.get(db.FilePath(done.ID)); got.Body.Len() != len(zip) || got.Header().Get("Content-Type") != "application/octet-stream" {
			t.Errorf("served %d bytes as %s", got.Body.Len(), got.Header().Get("Content-Type"))
		}
	})

	t.Run("quota and size", func(t *testing.T) {
		if got := s.send(http.MethodPost, "/api/uploads", fmt.Sprintf(`{%s,"name":"big","size":%d}`, parent, 1<<20+1), sameSite); got.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("larger than a file may be: %d", got.Code)
		}
		u := decode[apiUpload](t, s.send(http.MethodPost, "/api/uploads", fmt.Sprintf(`{%s,"name":"small","size":4}`, parent), sameSite))
		// More than declared is refused and leaves nothing behind.
		if got := s.raw(http.MethodPatch, "/api/uploads/"+u.ID, []byte("123456789"), map[string]string{"Upload-Offset": "0"}); got.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("more than declared: %d", got.Code)
		}
		if state := decode[apiUpload](t, s.get("/api/uploads/"+u.ID)); state.Received != 0 {
			t.Errorf("received %d after a refused chunk", state.Received)
		}
		if info, err := os.Stat(filepath.Join(s.store.Root(), store.DefaultTenant, "uploads", u.ID)); err == nil && info.Size() != 0 {
			t.Errorf("%d bytes stayed of a refused chunk", info.Size())
		}
		// Running uploads hold their room, so many at once do not pass the quota.
		var codes []int
		for range 3 {
			codes = append(codes, s.send(http.MethodPost, "/api/uploads", fmt.Sprintf(`{%s,"name":"part","size":%d}`, parent, 1<<20), sameSite).Code)
		}
		if codes[0] != http.StatusCreated || codes[2] != http.StatusRequestEntityTooLarge {
			t.Errorf("three uploads of 1 MiB against 2 MiB: %v", codes)
		}
		if got := s.send(http.MethodPost, "/api/uploads", fmt.Sprintf(`{%s,"name":"neg","size":-1}`, parent), sameSite); got.Code != http.StatusBadRequest {
			t.Errorf("a negative size: %d", got.Code)
		}
	})

	t.Run("other users and other sites", func(t *testing.T) {
		// Bob without the groups of anna, who is an administrator.
		bob := merged2(bob, map[string]string{"Remote-Groups": "staff"})
		u := decode[apiUpload](t, s.send(http.MethodPost, "/api/uploads", fmt.Sprintf(`{%s,"name":"secret.txt","size":6}`, parent), sameSite))
		for _, got := range []*httptest.ResponseRecorder{
			s.getWith("/api/uploads/"+u.ID, merged(bob)),
			s.raw(http.MethodPatch, "/api/uploads/"+u.ID, []byte("hijack"), merged2(bob, map[string]string{"Upload-Offset": "0"})),
			s.raw(http.MethodPost, "/api/uploads/"+u.ID+"/finish", nil, bob),
			s.raw(http.MethodDelete, "/api/uploads/"+u.ID, nil, bob),
		} {
			if got.Code != http.StatusNotFound {
				t.Errorf("bob reached anna's upload: %d %s", got.Code, got.Body)
			}
		}
		if got := s.raw(http.MethodPatch, "/api/uploads/"+u.ID, []byte("secret"), map[string]string{"Upload-Offset": "0", "Sec-Fetch-Site": "cross-site"}); got.Code != http.StatusForbidden {
			t.Errorf("a chunk from another site: %d", got.Code)
		}
		done := decode[finished](t, s.put(nil, parent, "private.txt", []byte("only anna"), 4))
		for _, target := range []string{db.FilePath(done.ID), "/api/files/" + done.ID, "/api/files/" + mine.ID + "/list", "/o/" + done.ID} {
			if got := s.getWith(target, merged(bob)); got.Code != http.StatusNotFound {
				t.Errorf("bob reads %s: %d", target, got.Code)
			}
		}
		if got := s.send(http.MethodPost, "/api/uploads", fmt.Sprintf(`{"file":%q,"name":"x","size":1}`, done.ID), merged2(sameSite, bob)); got.Code != http.StatusNotFound {
			t.Errorf("bob uploads a version of anna's file: %d", got.Code)
		}
		tenant := s.root(nil, db.RootTenant)
		if got := s.send(http.MethodPost, "/api/uploads", fmt.Sprintf(`{"parent":%q,"name":"x","size":1}`, tenant.ID), merged2(sameSite, bob)); got.Code != http.StatusForbidden {
			t.Errorf("bob writes into the folder of the tenant: %d", got.Code)
		}
		if got := s.send(http.MethodPut, "/api/files/roots/"+mine.ID+"/quota", `{"quota":0}`, merged2(sameSite, bob)); got.Code != http.StatusForbidden {
			t.Errorf("bob sets a quota: %d", got.Code)
		}
		if got := s.send(http.MethodPut, "/api/files/writers", `{"group":"bob"}`, merged2(sameSite, bob)); got.Code != http.StatusForbidden {
			t.Errorf("bob names the writers: %d", got.Code)
		}
	})
}

func TestAttachmentsAndSpaces(t *testing.T) {
	s := withFiles(t)
	mine := s.root(nil, db.RootUser)
	space := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", fmt.Sprintf(`{"title":"Plan","parent":%q}`, space.ID), sameSite))
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	done := decode[finished](t, s.put(nil, fmt.Sprintf(`"page":%q`, page.ID), "shot.png", png, 5))
	if done.Attach != "a1" || done.URL != db.AttachmentPath(page.ID, "a1") {
		t.Fatalf("%+v", done)
	}
	if got := s.get(done.URL); got.Body.String() != string(png) || got.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("attachment: %d %v", got.Code, got.Header())
	}
	if got := s.getWith(done.URL, merged(bob)); got.Code != http.StatusOK {
		t.Fatalf("bob sees the page, so its attachment: %d", got.Code)
	}
	moved := s.send(http.MethodPost, done.URL+"/move", fmt.Sprintf(`{"parent":%q}`, mine.ID), sameSite)
	file := decode[finished](t, moved)
	if moved.Code != http.StatusOK || file.URL != db.FilePath(file.ID) {
		t.Fatalf("move: %d %s", moved.Code, moved.Body)
	}
	if got := s.get(done.URL); got.Code != http.StatusNotFound {
		t.Fatalf("the attachment stayed: %d", got.Code)
	}
	if got := s.get(file.URL); got.Body.String() != string(png) {
		t.Fatal("the file lost its content")
	}
	// The space names its files.
	if got := s.send(http.MethodPut, "/api/spaces/"+space.ID+"/files", fmt.Sprintf(`{"root":%q}`, mine.ID), sameSite); got.Code != http.StatusNoContent {
		t.Fatalf("link: %d %s", got.Code, got.Body)
	}
	if got := s.get("/api/spaces/" + space.ID + "/files"); !strings.Contains(got.Body.String(), mine.ID) {
		t.Fatalf("space files: %s", got.Body)
	}
	if got := s.getWith("/api/spaces/"+space.ID+"/files", merged(bob)); strings.Contains(got.Body.String(), mine.ID) {
		t.Fatalf("bob sees anna's folder through the space: %s", got.Body)
	}
}

func TestFilesNeedTheStore(t *testing.T) {
	s := withDatabase(t)
	for _, target := range []string{"/api/files", "/api/uploads/0123456789abcdef0123456789abcdef"} {
		if got := s.get(target); got.Code != http.StatusNotImplemented {
			t.Errorf("%s without a store: %d", target, got.Code)
		}
	}
}

// The files of one tenant are not to be found from another, through any door of the files, and their content lies in
// a directory of the store of their own (ADR-23, the attack round of 2026-11-17).
func TestTenantsShareNoFiles(t *testing.T) {
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if _, err := database.CreateTenant(context.Background(), "acme", "Acme Ltd"); err != nil {
		t.Fatal(err)
	}
	st, _ := store.Open(t.TempDir())
	s := &filesSite{site: newSite(t, Options{DB: database, Store: st, AdminGroup: "underdoc-admins", Tenants: tenant.ModeMany, TenantDomain: tenantDomain}, sample),
		db: database, store: st}
	type made struct{ root, file, folder, upload, page, attachment string }
	build := func(host string) made {
		h := at(host)
		var m made
		got := decode[rootsAnswer](t, s.getWith("/api/files", h))
		for _, r := range got.Roots {
			if r.Role == db.RootUser {
				m.root = r.ID
			}
		}
		m.file = decode[finished](t, s.put(h, fmt.Sprintf(`"parent":%q`, m.root), "contract.txt", []byte("the same content"), 5)).ID
		m.folder = decode[apiNode](t, s.send(http.MethodPost, "/api/folders", fmt.Sprintf(`{"title":"Scans","parent":%q}`, m.root), h)).ID
		m.upload = decode[apiUpload](t, s.send(http.MethodPost, "/api/uploads", fmt.Sprintf(`{"parent":%q,"name":"half","size":10}`, m.root), h)).ID
		team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, h))
		m.page = decode[apiNode](t, s.send(http.MethodPost, "/api/pages", fmt.Sprintf(`{"title":"Plan","parent":%q}`, team.ID), h)).ID
		m.attachment = decode[finished](t, s.put(h, fmt.Sprintf(`"page":%q`, m.page), "shot.png", []byte("\x89PNG\r\n\x1a\n"), 4)).URL
		return m
	}
	inMain, inAcme := build(mainHost), build(acmeHost)
	for _, c := range []struct {
		name  string
		host  string
		other made
	}{{"acme looks at Main", acmeHost, inMain}, {"Main looks at acme", mainHost, inAcme}} {
		t.Run(c.name, func(t *testing.T) {
			h := at(c.host)
			if body := s.getWith("/api/files", h).Body.String(); strings.Contains(body, c.other.root) {
				t.Errorf("the roots show the other tenant's: %s", body)
			}
			for _, target := range []string{"/api/files/" + c.other.file, db.FilePath(c.other.file), "/api/files/" + c.other.root + "/list",
				"/api/files/" + c.other.folder + "/list", "/api/uploads/" + c.other.upload, c.other.attachment, "/api/pages/" + c.other.page + "/attachments",
				"/api/spaces/" + c.other.page + "/files", "/o/" + c.other.file} {
				if got := s.getWith(target, h); got.Code != http.StatusNotFound {
					t.Errorf("%s: %d %s", target, got.Code, got.Body)
				}
			}
			for _, w := range []struct{ method, target, body string }{
				{http.MethodPost, "/api/uploads", fmt.Sprintf(`{"parent":%q,"name":"x","size":1}`, c.other.root)},
				{http.MethodPost, "/api/uploads", fmt.Sprintf(`{"file":%q,"name":"x","size":1}`, c.other.file)},
				{http.MethodPost, "/api/uploads", fmt.Sprintf(`{"page":%q,"name":"x","size":1}`, c.other.page)},
				{http.MethodPost, "/api/uploads/" + c.other.upload + "/finish", ""},
				{http.MethodDelete, "/api/uploads/" + c.other.upload, ""},
				{http.MethodPost, "/api/files/" + c.other.file + "/versions/1/restore", ""},
				{http.MethodDelete, "/api/objects/" + c.other.file, ""},
			} {
				if got := s.send(w.method, w.target, w.body, h); got.Code != http.StatusNotFound {
					t.Errorf("%s %s: %d %s", w.method, w.target, got.Code, got.Body)
				}
			}
			if got := s.raw(http.MethodPatch, "/api/uploads/"+c.other.upload, []byte("hijack"), merged2(h, map[string]string{"Upload-Offset": "0"})); got.Code != http.StatusNotFound {
				t.Errorf("a chunk into the other tenant's upload: %d", got.Code)
			}
		})
	}
	// The same content in two tenants is two blobs, each in the directory of its tenant.
	sum := sha256.Sum256([]byte("the same content"))
	for _, tn := range []string{"main", "acme"} {
		if !st.Has(tn, hex.EncodeToString(sum[:])) {
			t.Errorf("the blob is not in the store of %s", tn)
		}
	}
}
