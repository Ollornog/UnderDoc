// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/files"
	"github.com/Ollornog/UnderDoc/internal/module"
)

// The tools for files (ADR-34): list them and read a page of their text. A file reaches an agent only as text, page by
// page, never as its bytes in a message.

type listFilesIn struct {
	In string `json:"in,omitempty" jsonschema:"a folder or root of the view Files, or a page for its attachments; the roots if empty"`
}

// listFiles lists the roots of the view Files, what lies in a folder, or the attachments of a page.
func (t *mcpTools) listFiles(_ context.Context, _ *mcp.CallToolRequest, in listFilesIn) (*mcp.CallToolResult, any, error) {
	ctx, database, user := t.ctx(), t.h.options.DB, t.user()
	var rows []string
	if in.In == "" {
		roots, err := database.FileRoots(ctx, user.ID)
		if err != nil {
			return t.failure(err, "list the files"), nil, nil
		}
		for _, r := range roots {
			rows = append(rows, fmt.Sprintf("[%s] %s %s", r.Role, oneLine(r.Title, 80), uri(db.Address{Object: r.ID})))
		}
		return text(listText("Roots of the view Files", rows)), nil, nil
	}
	ref, err := parseRef(in.In)
	if err != nil || ref.Folder != "" || ref.FilePage != 0 || ref.Address.Block != "" {
		return refusal("in: the address of a folder, a root or a page."), nil, nil
	}
	o, err := database.Object(ctx, ref.Address.Object, user.ID)
	if err != nil {
		return t.failure(err, "list the files"), nil, nil
	}
	if o.Kind == db.KindPage {
		list, err := database.Attachments(ctx, o.ID, user.ID)
		if err != nil {
			return t.failure(err, "list the attachments"), nil, nil
		}
		for _, a := range list {
			rows = append(rows, fmt.Sprintf("%s · %s · %s · %s", oneLine(a.Name, 80), a.ContentType, size(a.Size), db.AttachmentPath(o.ID, a.ID)))
		}
		return text(listText(fmt.Sprintf("Attachments of %q", o.Title), rows)), nil, nil
	}
	list, err := database.FilesIn(ctx, o.ID, user.ID)
	if err != nil {
		return t.failure(err, "list the files"), nil, nil
	}
	for _, f := range list {
		if f.Kind == db.KindFile {
			rows = append(rows, fmt.Sprintf("[file] %s %s · %s · %s · %d versions", oneLine(f.Title, 80), uri(db.Address{Object: f.ID}),
				f.ContentType, size(f.Size), f.Versions))
			continue
		}
		rows = append(rows, fmt.Sprintf("[%s] %s %s", f.Kind, oneLine(f.Title, 80), uri(db.Address{Object: f.ID})))
	}
	return text(listText(fmt.Sprintf("In %q", o.Title), rows)), nil, nil
}

// listText writes at most mcpMaxRows rows under a heading, with the total.
func listText(heading string, rows []string) string {
	var b strings.Builder
	b.WriteString(heading + ":\n")
	for i, row := range rows {
		if i == mcpMaxRows {
			break
		}
		b.WriteString("- " + row + "\n")
	}
	fmt.Fprintf(&b, "%d of %d shown.", min(len(rows), mcpMaxRows), len(rows))
	return b.String()
}

// size is a number of bytes for people.
func size(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d bytes", n)
}

type readFilePageIn struct {
	File string `json:"file" jsonschema:"the file, with #page=<n> for a page other than the first"`
}

// readFilePage answers the text of one page of a file: of a PDF the page as it is, of any other file its text as one
// page. The best text there is comes first: what the content says itself before what was recognised in pixels.
func (t *mcpTools) readFilePage(_ context.Context, _ *mcp.CallToolRequest, in readFilePageIn) (*mcp.CallToolResult, any, error) {
	ref, err := parseRef(in.File)
	if err != nil || ref.Folder != "" || ref.Address.Block != "" {
		return refusal("file: underdoc:<id> or underdoc:<id>#page=<n>."), nil, nil
	}
	n := max(ref.FilePage, 1)
	ctx, database, user := t.ctx(), t.h.options.DB, t.user()
	f, err := database.File(ctx, ref.Address.Object, user.ID)
	if err != nil {
		return t.failure(err, "read the file"), nil, nil
	}
	texts, err := database.FileTexts(ctx, f.ID, user.ID)
	if err != nil {
		return t.failure(err, "read the file"), nil, nil
	}
	if len(texts) == 0 || strings.TrimSpace(texts[0].Text) == "" {
		return refusal(fmt.Sprintf("%q has no text yet (%s): it may be a scan or of a type whose text is not read.", f.Title, f.ContentType)), nil, nil
	}
	// A PDF by its pages, a long text without them in pseudo pages of about 20 KB (B-5).
	pages := files.Pages(texts[0].Text)
	if n > len(pages) {
		return refusal(fmt.Sprintf("%q has %d pages of text.", f.Title, len(pages))), nil, nil
	}
	t.h.noteOpen(t.r, f.Object)
	body := strings.TrimSpace(pages[n-1])
	if body == "" {
		body = "(no text on this page)"
	}
	return text(clip(fmt.Sprintf("file: %s · %s · page %d of %d · text from %s\n\n%s", uri(db.Address{Object: f.ID}), oneLine(f.Title, 120),
		n, len(pages), texts[0].Source, body))), nil, nil
}

// uploadLinkPath is where a one-time upload link takes its file: below /s/, which the gate leaves open as it does for
// share links, since the program that sends the file is not signed in (ADR-34). The token alone says what it may do.
const uploadLinkPath = "/s/u/"

type uploadLinkIn struct {
	In   string `json:"in" jsonschema:"where the file goes: a folder or root of the view Files, a file for its next version, or a page to attach it to"`
	Name string `json:"name" jsonschema:"the name of the file"`
	Size int64  `json:"size" jsonschema:"its size in bytes, exactly"`
}

// uploadLink starts an upload with the person's rights and answers a link a program sends the file to once, within
// db.UploadLinkTime: a file never travels inside a message (ADR-34).
func (t *mcpTools) uploadLink(_ context.Context, _ *mcp.CallToolRequest, in uploadLinkIn) (*mcp.CallToolResult, any, error) {
	ref, err := parseRef(in.In)
	if err != nil || ref.Folder != "" || ref.FilePage != 0 || ref.Address.Block != "" {
		return refusal("in: the address of a folder, a root, a file or a page."), nil, nil
	}
	ctx, database, user := t.ctx(), t.h.options.DB, t.user()
	o, err := database.Object(ctx, ref.Address.Object, user.ID)
	if err != nil {
		return t.failure(err, "start the upload"), nil, nil
	}
	target := db.UploadTarget{Parent: o.ID}
	switch o.Kind {
	case db.KindPage:
		target = db.UploadTarget{Page: o.ID}
	case db.KindFile:
		target = db.UploadTarget{File: o.ID}
	}
	u, err := database.StartUpload(ctx, user.ID, target, in.Name, in.Size)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrFileName), errors.Is(err, db.ErrUpload), errors.Is(err, db.ErrNotFile), errors.Is(err, db.ErrTooLarge):
			return refusal(err.Error()), nil, nil
		}
		return t.failure(err, "start the upload"), nil, nil
	}
	token, err := database.LinkUpload(ctx, u.ID, user.ID)
	if err != nil {
		return t.failure(err, "make the link"), nil, nil
	}
	link := requestOrigin(t.r) + uploadLinkPath + token
	// The line for the shell names the local file by a placeholder and quotes the link in single quotes: the name of the
	// file is the agent's, and may hold $( ) or backticks that double quotes would run (B-5). The link is a bearer
	// address: whoever holds it sends the file.
	return text(fmt.Sprintf("Send the file once, within %d minutes, as the body of a PUT of exactly %d bytes; put the path of the local file in place of <local-file>:\n\ncurl -fsS -T <local-file> %s\n\n"+
		"The link works once and only for %q into %s, and whoever holds it may use it, so pass it on to nobody. It answers what was made.",
		int(db.UploadLinkTime.Minutes()), in.Size, shellQuote(link), u.Name, uri(db.Address{Object: o.ID}))), nil, nil
}

// shellQuote quotes a word for a POSIX shell in single quotes, where nothing is expanded; a single quote inside ends the
// quote, adds an escaped one and opens it again.
func shellQuote(word string) string {
	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}

// uploadThroughOneTimeLink takes the file of a one-time upload link (PUT /s/u/<token>), in the name of the agent's person
// who asked for it, and finishes it as the browser does; the rights are checked again when it is made.
func (h *handler) uploadThroughOneTimeLink(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil || h.options.Store == nil || !h.moduleOn(r.Context(), module.Files) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	// A program sends the file; a page of a browser has no business here, from another site least of all.
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "none" && site != "same-origin" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "only a program sends to an upload link"})
		return
	}
	u, err := h.options.DB.TakeUploadLink(r.Context(), r.PathValue("token"))
	if errors.Is(err, db.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such link, or it was used or ran out"})
		return
	}
	if h.failed(w, err, "upload link") {
		return
	}
	// The upload is the agent's: its connection says so, and its rights are those of its person, as an agent's.
	user := account.User{ID: u.Owner, AuthKind: account.AuthOAuth, MCP: true}
	r = r.WithContext(account.With(r.Context(), user))
	ctx := r.Context()
	cancel := func() {
		_ = h.options.DB.CancelUpload(ctx, u.ID, user.ID)
		_ = h.options.Store.RemoveUpload(h.tenant(r), u.ID)
	}
	if !h.uploads.take(u.ID) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "another request writes this upload"})
		return
	}
	defer h.uploads.give(u.ID)
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(30 * time.Minute))
	f, err := h.options.Store.OpenUpload(h.tenant(r), u.ID)
	if err != nil {
		cancel()
		h.failed(w, err, "upload link")
		return
	}
	n, err := io.Copy(f, io.LimitReader(r.Body, u.Size+1))
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		cancel()
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "the file did not arrive whole; ask for a new link"})
		return
	}
	if n != u.Size {
		cancel()
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("the link is for %d bytes, %d came; ask for a new link", u.Size, n)})
		return
	}
	if err := h.options.DB.Received(ctx, u.ID, user.ID, 0, n); err != nil {
		cancel()
		h.failed(w, err, "upload link")
		return
	}
	done, contentType, err := h.completeUpload(r, u, user)
	if err != nil {
		cancel()
		h.filesFailed(w, err, "upload link")
		return
	}
	if done.Page != "" {
		writeJSON(w, http.StatusCreated, map[string]string{"page": done.Page, "attachment": done.Attachment,
			"url": db.AttachmentPath(done.Page, done.Attachment), "type": contentType})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"file": uri(db.Address{Object: done.File.ID}), "name": done.File.Title, "version": done.Version, "type": contentType})
}
