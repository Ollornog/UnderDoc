// SPDX-License-Identifier: MIT

package web

import (
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/files"
	"github.com/Ollornog/UnderDoc/internal/store"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// The view Files (ADR-28): roots, folders and files, uploads in chunks that can resume, versions and attachments of
// pages. Content leaves only through these handlers, after the database has said the viewer may see it.

// MaxChunk is the most one request of an upload carries.
const MaxChunk = 8 << 20

// fileCSP is the policy for content served from the store: nothing in it runs, whatever it is.
const fileCSP = "default-src 'none'; img-src 'self'; media-src 'self'; style-src 'unsafe-inline'; sandbox"

type apiRoot struct {
	ID      string `json:"id"`
	Title   string `json:"n"`
	Role    string `json:"role"`
	Private bool   `json:"private,omitempty"`
	Mine    bool   `json:"mine,omitempty"`
	Access  string `json:"access,omitempty"`
	Writers string `json:"writers,omitempty"`
	Quota   int64  `json:"quota"`
	Limit   int64  `json:"limit"`
	Used    int64  `json:"used"`
	CanEdit bool   `json:"canEdit"`
}

type apiFileEntry struct {
	ID        string   `json:"id"`
	Title     string   `json:"n"`
	Kind      string   `json:"t"`
	Size      int64    `json:"size,omitempty"`
	Type      string   `json:"type,omitempty"`
	Versions  int      `json:"versions,omitempty"`
	Changed   string   `json:"ch"`
	ChangedBy string   `json:"by,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

type apiFileVersion struct {
	Number int    `json:"n"`
	Size   int64  `json:"size"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	Author string `json:"who"`
	At     string `json:"at"`
}

type apiFile struct {
	apiFileEntry
	Parent     string           `json:"parent"`
	Place      string           `json:"place"`
	Owner      string           `json:"owner"`
	Created    string           `json:"created"`
	CanEdit    bool             `json:"canEdit"`
	CanComment bool             `json:"canComment"`
	Inline     bool             `json:"inline"`
	Content    string           `json:"content"`
	History    []apiFileVersion `json:"history"`
}

type apiUpload struct {
	ID       string `json:"id"`
	Size     int64  `json:"size"`
	Received int64  `json:"received"`
	Chunk    int    `json:"chunk"`
}

// filesReady answers that the view Files needs the database and the content store, and reports whether both are there.
func (h *handler) filesReady(w http.ResponseWriter) bool {
	if h.options.DB == nil || h.options.Store == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "files need the database and the content store"})
		return false
	}
	return true
}

// tenant is the tenant of a request (ADR-23), whose directory of the content store holds its blobs.
func (h *handler) tenant(r *http.Request) string {
	t, _ := tenant.From(r.Context())
	return t
}

// filesFailed answers the errors of files, and the others as failed does.
func (h *handler) filesFailed(w http.ResponseWriter, err error, what string) bool {
	switch {
	case errors.Is(err, db.ErrFileName), errors.Is(err, db.ErrUpload), errors.Is(err, db.ErrNotFile):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, db.ErrTooLarge), errors.Is(err, db.ErrQuota):
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": err.Error()})
	default:
		return h.failed(w, err, what)
	}
	return true
}

// apiFileRoots returns the roots of the view Files, making the viewer's folder, and the tenant's for an administrator,
// when they are not there yet.
func (h *handler) apiFileRoots(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	ctx := r.Context()
	user, _ := account.From(ctx)
	admin := h.isAdmin(r)
	if err := h.options.DB.EnsureRoots(ctx, user, admin, h.options.AdminGroup); h.failed(w, err, "roots") {
		return
	}
	roots, err := h.options.DB.FileRoots(ctx, user.ID)
	if h.failed(w, err, "roots") {
		return
	}
	out := make([]apiRoot, 0, len(roots))
	for _, root := range roots {
		out = append(out, apiRoot{ID: root.ID, Title: root.Title, Role: root.Role, Private: root.Private, Mine: root.Owner == user.ID,
			Access: root.Access, Writers: root.Writers, Quota: root.Quota, Limit: root.Limit, Used: root.Used,
			CanEdit: h.options.DB.MayEdit(ctx, root.Object, user.ID)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"roots": out, "admin": admin, "chunk": MaxChunk})
}

func (h *handler) apiCreateFileCollection(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	user, ok := h.change(w, r, 4<<10, &body)
	if !ok {
		return
	}
	o, err := h.options.DB.CreateFileCollection(r.Context(), user, body.Title)
	if h.failed(w, err, "file collection") {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": o.ID})
}

// apiSetQuota sets the quota of a root; only administrators do (ADR-28).
func (h *handler) apiSetQuota(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	var body struct {
		Quota int64 `json:"quota"`
	}
	if _, ok := h.change(w, r, 1<<10, &body); !ok {
		return
	}
	if !h.isAdmin(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only administrators set quotas"})
		return
	}
	if h.filesFailed(w, h.options.DB.SetQuota(r.Context(), r.PathValue("id"), body.Quota), "quota") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// apiSetWriters names the group that writes in the folder of the tenant; only administrators do.
func (h *handler) apiSetWriters(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	var body struct {
		Group string `json:"group"`
	}
	if _, ok := h.change(w, r, 1<<10, &body); !ok {
		return
	}
	if !h.isAdmin(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only administrators name the writers"})
		return
	}
	if h.failed(w, h.options.DB.SetWriters(r.Context(), body.Group), "writers") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func fileEntry(f db.FileInfo) apiFileEntry {
	return apiFileEntry{ID: f.ID, Title: f.Title, Kind: f.Kind, Size: f.Size, Type: f.ContentType, Versions: f.Versions, Changed: stamp(f.Changed),
		ChangedBy: f.ChangedBy, Tags: f.Tags}
}

// apiFileList returns what lies in a folder or collection of the view Files: folders first, then files, by name.
func (h *handler) apiFileList(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	user, _ := account.From(r.Context())
	list, err := h.options.DB.FilesIn(r.Context(), r.PathValue("id"), user.ID)
	if h.failed(w, err, "file list") {
		return
	}
	parent, err := h.options.DB.Object(r.Context(), r.PathValue("id"), user.ID)
	if h.failed(w, err, "file list") {
		return
	}
	out := make([]apiFileEntry, 0, len(list))
	for _, f := range list {
		if db.IsDivider(f.Kind) {
			continue
		}
		out = append(out, fileEntry(f))
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": parent.ID, "n": parent.Title, "parent": parent.Parent, "place": h.place(r, parent),
		"canEdit": h.options.DB.MayEdit(r.Context(), parent, user.ID), "entries": out})
}

// apiFile returns a file with its versions.
func (h *handler) apiFile(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	ctx := r.Context()
	user, _ := account.From(ctx)
	f, err := h.options.DB.File(ctx, r.PathValue("id"), user.ID)
	if h.failed(w, err, "file") {
		return
	}
	versions, err := h.options.DB.FileVersions(ctx, f.ID, user.ID)
	if h.failed(w, err, "file") {
		return
	}
	out := apiFile{apiFileEntry: fileEntry(f), Parent: f.Parent, Place: h.place(r, f.Object), Owner: h.options.DB.AccountName(ctx, f.Owner),
		Created: stamp(f.Created), CanEdit: h.options.DB.MayEdit(ctx, f.Object, user.ID), CanComment: h.options.DB.MayComment(ctx, f.Object, user.ID),
		Inline: files.Inline(f.ContentType), Content: db.FilePath(f.ID), History: []apiFileVersion{}}
	for _, v := range versions {
		out.History = append(out.History, apiFileVersion{Number: v.Number, Size: v.Size, Type: v.ContentType, Name: v.Name, Author: v.AuthorName, At: stamp(v.At)})
	}
	writeJSON(w, http.StatusOK, out)
}

// apiFileContent sends a version of a file, the current one without ?v=.
func (h *handler) apiFileContent(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	user, _ := account.From(r.Context())
	number := 0
	if v := r.URL.Query().Get("v"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "v is a version number"})
			return
		}
		number = n
	}
	v, err := h.options.DB.FileVersion(r.Context(), r.PathValue("id"), user.ID, number)
	if h.failed(w, err, "file content") {
		return
	}
	h.serveBlob(w, r, v.Blob, v.Name, v.ContentType)
}

// serveBlob sends a blob of the store under a name. Only types the browser may show stay inline, and only without
// ?download; everything else is an attachment. Nothing served runs: the policy sandboxes it and the type is never sniffed.
func (h *handler) serveBlob(w http.ResponseWriter, r *http.Request, sha, name, contentType string) {
	f, err := h.options.Store.Open(h.tenant(r), sha)
	if err != nil {
		h.log.Error("blob missing", slog.String("blob", sha), slog.String("error", err.Error()))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "the content is missing from the store"})
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	disposition := "attachment"
	if files.Inline(contentType) && r.URL.Query().Get("download") == "" {
		disposition = "inline"
	} else {
		contentType = "application/octet-stream"
	}
	header := w.Header()
	header.Set("Content-Type", contentType)
	header.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	header.Set("Content-Security-Policy", fileCSP)
	// A blob never changes, so its hash is a strong validator.
	header.Set("ETag", `"`+sha+`"`)
	header.Set("Cache-Control", "private, no-cache")
	// Large files take longer than the write timeout of the server.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(time.Hour))
	http.ServeContent(w, r, "", info.ModTime(), f)
}

func (h *handler) apiRestoreFileVersion(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	user, ok := h.change(w, r, 0, nil)
	if !ok {
		return
	}
	n, err := strconv.Atoi(r.PathValue("n"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not a version"})
		return
	}
	made, err := h.options.DB.RestoreVersion(r.Context(), r.PathValue("id"), user.ID, n)
	if h.filesFailed(w, err, "restore version") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"version": made})
}

// --- Uploads in chunks

// uploadBusy keeps the uploads a request is writing right now, so two requests never write the same upload at once.
type uploadBusy struct {
	mu   sync.Mutex
	busy map[string]bool
}

func (u *uploadBusy) take(id string) bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.busy == nil {
		u.busy = map[string]bool{}
	}
	if u.busy[id] {
		return false
	}
	u.busy[id] = true
	return true
}

func (u *uploadBusy) give(id string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	delete(u.busy, id)
}

func uploadOut(u db.Upload) apiUpload {
	return apiUpload{ID: u.ID, Size: u.Size, Received: u.Received, Chunk: MaxChunk}
}

// apiStartUpload begins an upload: the name, the size and where it goes are checked now, the quota included.
func (h *handler) apiStartUpload(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	var body struct {
		Parent string `json:"parent"`
		File   string `json:"file"`
		Page   string `json:"page"`
		Name   string `json:"name"`
		Size   int64  `json:"size"`
	}
	user, ok := h.change(w, r, 4<<10, &body)
	if !ok {
		return
	}
	u, err := h.options.DB.StartUpload(r.Context(), user.ID, db.UploadTarget{Parent: body.Parent, File: body.File, Page: body.Page}, body.Name, body.Size)
	if h.filesFailed(w, err, "start upload") {
		return
	}
	writeJSON(w, http.StatusCreated, uploadOut(u))
}

// upload returns an upload of the viewer named in the path, or answers that there is none.
func (h *handler) upload(w http.ResponseWriter, r *http.Request, user account.User) (db.Upload, bool) {
	uploadID := r.PathValue("id")
	if !store.ValidUpload(uploadID) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return db.Upload{}, false
	}
	u, err := h.options.DB.GetUpload(r.Context(), uploadID, user.ID)
	if h.failed(w, err, "upload") {
		return db.Upload{}, false
	}
	return u, true
}

// apiUploadState tells how much of an upload has come, so a browser can resume after a break.
func (h *handler) apiUploadState(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	user, _ := account.From(r.Context())
	if u, ok := h.upload(w, r, user); ok {
		writeJSON(w, http.StatusOK, uploadOut(u))
	}
}

// apiUploadChunk adds a chunk at the offset in the header Upload-Offset, which must be what has come so far; a chunk at
// any other offset is refused with what has come, so the browser sends from there.
func (h *handler) apiUploadChunk(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	user, ok := h.change(w, r, 0, nil)
	if !ok {
		return
	}
	u, ok := h.upload(w, r, user)
	if !ok {
		return
	}
	if !h.uploads.take(u.ID) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "another request writes this upload", "received": u.Received})
		return
	}
	defer h.uploads.give(u.ID)
	// Read again under the lock: a request that just ended may have moved on.
	if u, ok = h.upload(w, r, user); !ok {
		return
	}
	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil || offset != u.Received {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "the chunk does not start where the upload stands", "received": u.Received})
		return
	}
	limit := min(int64(MaxChunk), u.Size-u.Received)
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(10 * time.Minute))
	f, err := h.options.Store.OpenUpload(h.tenant(r), u.ID)
	if err != nil {
		h.failed(w, err, "upload chunk")
		return
	}
	defer func() { _ = f.Close() }()
	// What a broken request wrote after the last recorded byte is dropped.
	if err := f.Truncate(offset); err != nil {
		h.failed(w, err, "upload chunk")
		return
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		h.failed(w, err, "upload chunk")
		return
	}
	n, err := io.Copy(f, io.LimitReader(r.Body, limit+1))
	if err == nil && n > limit {
		err = db.ErrTooLarge
	}
	if err == nil {
		err = f.Sync()
	}
	if err != nil {
		_ = f.Truncate(offset)
		if errors.Is(err, db.ErrTooLarge) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "more than the chunk or the declared size", "received": u.Received})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "the chunk did not arrive whole", "received": u.Received})
		return
	}
	if h.filesFailed(w, h.options.DB.Received(r.Context(), u.ID, user.ID, offset, offset+n), "upload chunk") {
		return
	}
	u.Received = offset + n
	writeJSON(w, http.StatusOK, uploadOut(u))
}

// apiFinishUpload turns a complete upload into a file, a version or an attachment: the store hashes what came, the
// content tells its type, and the database records it in one transaction.
func (h *handler) apiFinishUpload(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	user, ok := h.change(w, r, 0, nil)
	if !ok {
		return
	}
	u, ok := h.upload(w, r, user)
	if !ok {
		return
	}
	if !h.uploads.take(u.ID) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "another request writes this upload"})
		return
	}
	defer h.uploads.give(u.ID)
	tenant := h.tenant(r)
	path, err := h.options.Store.UploadPath(tenant, u.ID)
	if err != nil {
		h.failed(w, err, "finish upload")
		return
	}
	if u.Size == 0 {
		// An empty file has no chunk that would have made its data.
		f, err := h.options.Store.OpenUpload(tenant, u.ID)
		if err != nil {
			h.failed(w, err, "finish upload")
			return
		}
		_ = f.Close()
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Minute))
	sha, size, err := store.HashFile(path)
	if err != nil {
		h.failed(w, err, "finish upload")
		return
	}
	contentType, err := detect(path, u.Name)
	if err != nil {
		h.failed(w, err, "finish upload")
		return
	}
	done, err := h.options.DB.FinishUpload(r.Context(), u.ID, user.ID, sha, size, contentType, func() error {
		return h.options.Store.Adopt(tenant, u.ID, sha)
	})
	if h.filesFailed(w, err, "finish upload") {
		return
	}
	if err := h.options.Store.RemoveUpload(tenant, u.ID); err != nil {
		h.log.Error("upload not removed", slog.String("error", err.Error()))
	}
	h.afterUpload(r, done, sha, contentType)
	if done.Page != "" {
		writeJSON(w, http.StatusCreated, map[string]string{"page": done.Page, "attachment": done.Attachment,
			"url": db.AttachmentPath(done.Page, done.Attachment), "type": contentType})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": done.File.ID, "n": done.File.Title, "version": done.Version, "type": contentType})
}

// afterUpload is where what follows an upload starts, such as previews and the text for the search.
func (h *handler) afterUpload(*http.Request, db.Finished, string, string) {}

// detect reads the first bytes of a file and tells its type.
func detect(path, name string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, files.Sniff)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	return files.Detect(name, head[:n]), nil
}

func (h *handler) apiCancelUpload(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	user, ok := h.change(w, r, 0, nil)
	if !ok {
		return
	}
	u, ok := h.upload(w, r, user)
	if !ok {
		return
	}
	if h.failed(w, h.options.DB.CancelUpload(r.Context(), u.ID, user.ID), "cancel upload") {
		return
	}
	if err := h.options.Store.RemoveUpload(h.tenant(r), u.ID); err != nil {
		h.log.Error("upload not removed", slog.String("error", err.Error()))
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Attachments of pages

type apiAttachment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	Type string `json:"type"`
	URL  string `json:"url"`
	At   string `json:"at"`
}

func (h *handler) apiAttachments(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	user, _ := account.From(r.Context())
	list, err := h.options.DB.Attachments(r.Context(), r.PathValue("id"), user.ID)
	if h.failed(w, err, "attachments") {
		return
	}
	out := make([]apiAttachment, 0, len(list))
	for _, a := range list {
		out = append(out, apiAttachment{ID: a.ID, Name: a.Name, Size: a.Size, Type: a.ContentType, URL: db.AttachmentPath(a.Page, a.ID), At: stamp(a.At)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handler) apiAttachmentContent(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	user, _ := account.From(r.Context())
	a, err := h.options.DB.GetAttachment(r.Context(), r.PathValue("id"), r.PathValue("attachment"), user.ID)
	if h.failed(w, err, "attachment") {
		return
	}
	h.serveBlob(w, r, a.Blob, a.Name, a.ContentType)
}

// apiMoveAttachment makes an attachment a file in a folder of the view Files.
func (h *handler) apiMoveAttachment(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	var body struct {
		Parent string `json:"parent"`
	}
	user, ok := h.change(w, r, 1<<10, &body)
	if !ok {
		return
	}
	file, err := h.options.DB.MoveAttachment(r.Context(), r.PathValue("id"), r.PathValue("attachment"), user.ID, body.Parent)
	if h.filesFailed(w, err, "move attachment") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": file.ID, "n": file.Title, "url": db.FilePath(file.ID)})
}

// --- Spaces and their files

func (h *handler) apiSpaceFiles(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	user, _ := account.From(r.Context())
	root, err := h.options.DB.SpaceFiles(r.Context(), r.PathValue("id"), user.ID)
	if errors.Is(err, db.ErrNoLink) {
		writeJSON(w, http.StatusOK, map[string]any{"root": nil})
		return
	}
	if h.failed(w, err, "space files") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"root": map[string]string{"id": root.ID, "n": root.Title, "role": root.Role}})
}

func (h *handler) apiLinkSpaceFiles(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	var body struct {
		Root string `json:"root"`
	}
	user, ok := h.change(w, r, 1<<10, &body)
	if !ok {
		return
	}
	if h.failed(w, h.options.DB.LinkSpaceFiles(r.Context(), r.PathValue("id"), user.ID, body.Root), "link space files") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
