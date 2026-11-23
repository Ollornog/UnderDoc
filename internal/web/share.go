// SPDX-License-Identifier: MIT

package web

import (
	"bytes"
	"context"
	"errors"
	"html"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/auth"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/id"
	"github.com/Ollornog/UnderDoc/internal/markdown"
	"github.com/Ollornog/UnderDoc/internal/store"
)

// Share links (ADR-29). The manager of an object makes a link with a mode, an optional expiry and an optional password;
// the token is shown once and only its hash is kept. Under /s/<token> anyone with the link reads a page as it was frozen,
// or the current files of a file or folder, or uploads into a folder; an account of the tenant that opens a link to
// comment or edit holds that role from then on.

// shareCookie names the cookie of a browser that gave the password of a link; its path is the link's.
const shareCookie = "underdoc_share"

// MaxLinkUpload is the most one file sent through a link may have.
const MaxLinkUpload = 100 << 20

// sharePath is the address of a link.
func sharePath(token string) string { return "/s/" + token }

// releaseKey carries the embedded objects released for a link while a page is frozen: what is not in it shows "not
// shared" instead of its content.
type releaseKey struct{}

func withRelease(ctx context.Context, released []string) context.Context {
	return context.WithValue(ctx, releaseKey{}, released)
}

// unreleased tells whether an embedded object may not be shown, because a link is being frozen without it.
func unreleased(ctx context.Context, objectID string) bool {
	released, ok := ctx.Value(releaseKey{}).([]string)
	return ok && !slices.Contains(released, objectID)
}

// --- Making and listing links

type apiLink struct {
	ID          string   `json:"id"`
	Object      string   `json:"object"`
	Title       string   `json:"title"`
	Mode        string   `json:"mode"`
	Expires     string   `json:"expires,omitempty"`
	Password    bool     `json:"password"`
	CreatedBy   string   `json:"createdBy"`
	Creator     string   `json:"creator"`
	Created     string   `json:"created"`
	FrozenAt    string   `json:"frozenAt,omitempty"`
	Released    []string `json:"released"`
	Users       int      `json:"users"`
	URL         string   `json:"url,omitempty"`
	ObjectKind  string   `json:"kind"`
	ObjectWhere string   `json:"area"`
}

func linkOut(l db.Link) apiLink {
	out := apiLink{ID: l.ID, Object: l.Object, Title: l.ObjectTitle, Mode: l.Mode, Password: l.HasPassword, CreatedBy: l.CreatedBy,
		Creator: l.Creator, Created: stamp(l.Created), Released: l.Released, Users: l.Users, ObjectKind: l.ObjectKind, ObjectWhere: l.Area}
	if !l.Expires.IsZero() {
		out.Expires = stamp(l.Expires)
	}
	if !l.FrozenAt.IsZero() {
		out.FrozenAt = stamp(l.FrozenAt)
	}
	if out.Released == nil {
		out.Released = []string{}
	}
	return out
}

type apiShareable struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Depth is how deep it is embedded: 1 directly in the page.
	Depth int `json:"depth"`
}

// shareables lists the pages a page embeds, directly and through what it embeds, as far as embedding goes: each must be
// released one by one for a link (ADR-29). Only what the viewer sees is listed; the rest shows "not to be found" anyway.
func (h *handler) shareables(r *http.Request, pageID string) ([]apiShareable, error) {
	user, _ := account.From(r.Context())
	out := []apiShareable{}
	seen := map[string]bool{pageID: true}
	level := []string{pageID}
	for depth := 1; depth <= maxEmbed && len(level) > 0; depth++ {
		var next []string
		for _, p := range level {
			embeds, err := h.options.DB.Embeds(r.Context(), p, user.ID)
			if err != nil {
				return nil, err
			}
			for _, e := range embeds {
				if seen[e.Address.Object] {
					continue
				}
				seen[e.Address.Object] = true
				out = append(out, apiShareable{ID: e.Address.Object, Title: e.Title, Depth: depth})
				next = append(next, e.Address.Object)
			}
		}
		level = next
	}
	return out, nil
}

// apiShareables answers what a link to a page would have to release.
func (h *handler) apiShareables(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	user, _ := account.From(r.Context())
	o, err := h.options.DB.Object(r.Context(), r.PathValue("id"), user.ID)
	if h.failed(w, err, "shareables") {
		return
	}
	out := []apiShareable{}
	if db.HasBlocks(o.Kind) {
		if out, err = h.shareables(r, o.ID); h.failed(w, err, "shareables") {
			return
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// freeze renders a page as a link shows it: every block, with only the released embeddings, as the maker sees it now.
func (h *handler) freeze(r *http.Request, pageID string, released []string) (string, error) {
	user, _ := account.From(r.Context())
	blocks, err := h.options.DB.Blocks(r.Context(), pageID, user.ID)
	if err != nil {
		return "", err
	}
	frozen := r.WithContext(withRelease(r.Context(), released))
	var out strings.Builder
	for _, b := range blocks {
		out.WriteString(`<section data-share-block>`)
		out.WriteString(h.render(frozen, b.Body, []string{pageID, pageID + "#" + b.ID}))
		out.WriteString("</section>")
	}
	return out.String(), nil
}

// releasedOf keeps of the released IDs those the page really embeds.
func (h *handler) releasedOf(r *http.Request, pageID string, wanted []string) ([]string, error) {
	can, err := h.shareables(r, pageID)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, s := range can {
		if slices.Contains(wanted, s.ID) {
			out = append(out, s.ID)
		}
	}
	return out, nil
}

// apiCreateLink makes a share link and answers it with its address, shown this once.
func (h *handler) apiCreateLink(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode     string   `json:"mode"`
		Expires  string   `json:"expires"`
		Password string   `json:"password"`
		Released []string `json:"released"`
	}
	user, ok := h.change(w, r, 16<<10, &in)
	if !ok {
		return
	}
	link := db.NewLink{Mode: in.Mode}
	if in.Expires != "" {
		t, err := parseExpiry(in.Expires)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "the expiry is a date (2026-12-31) or a moment (RFC 3339)"})
			return
		}
		link.Expires = t
	}
	if in.Password != "" {
		if err := auth.CheckPassword(in.Password); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		hash, err := auth.HashPassword(in.Password)
		if h.failed(w, err, "link") {
			return
		}
		link.PasswordHash = hash
	}
	o, err := h.options.DB.Object(r.Context(), r.PathValue("id"), user.ID)
	if h.failed(w, err, "link") {
		return
	}
	if db.HasBlocks(o.Kind) {
		if link.Released, err = h.releasedOf(r, o.ID, in.Released); h.failed(w, err, "link") {
			return
		}
		if link.Frozen, err = h.freeze(r, o.ID, link.Released); h.failed(w, err, "link") {
			return
		}
	}
	made, token, err := h.options.DB.CreateLink(r.Context(), o.ID, h.actor(r), link)
	if h.linkFailed(w, err) {
		return
	}
	h.note(r, db.LogLinkMade, o.ID, made.ID+" "+made.Mode)
	out := linkOut(made)
	out.URL = requestOrigin(r) + sharePath(token)
	writeJSON(w, http.StatusCreated, out)
}

// parseExpiry reads an expiry: a day, which ends at its close in UTC, or a moment.
func parseExpiry(s string) (time.Time, error) {
	if t, err := time.Parse(time.DateOnly, s); err == nil {
		return t.Add(24*time.Hour - time.Second), nil
	}
	return time.Parse(time.RFC3339, s)
}

func (h *handler) linkFailed(w http.ResponseWriter, err error) bool {
	if errors.Is(err, db.ErrLink) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return true
	}
	return h.failed(w, err, "link")
}

// apiLinks lists links: of one object with ?object=, else all the viewer may see (ADR-29).
func (h *handler) apiLinks(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	links, err := h.options.DB.Links(r.Context(), h.actor(r), r.URL.Query().Get("object"))
	if h.failed(w, err, "links") {
		return
	}
	out := []apiLink{}
	for _, l := range links {
		out = append(out, linkOut(l))
	}
	writeJSON(w, http.StatusOK, out)
}

// apiRevokeLink ends a link.
func (h *handler) apiRevokeLink(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.change(w, r, 0, nil); !ok {
		return
	}
	revoked, err := h.options.DB.RevokeLink(r.Context(), r.PathValue("id"), h.actor(r))
	if h.failed(w, err, "revoke") {
		return
	}
	h.note(r, db.LogLinkRevoked, revoked.Object, revoked.ID)
	w.WriteHeader(http.StatusNoContent)
}

// apiRenewLink freezes the page of a link again, with the embeddings released now.
func (h *handler) apiRenewLink(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Released []string `json:"released"`
	}
	if _, ok := h.change(w, r, 16<<10, &in); !ok {
		return
	}
	links, err := h.options.DB.Links(r.Context(), h.actor(r), "")
	if h.failed(w, err, "renew") {
		return
	}
	i := slices.IndexFunc(links, func(l db.Link) bool { return l.ID == r.PathValue("id") })
	if i < 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	released, err := h.releasedOf(r, links[i].Object, in.Released)
	if h.failed(w, err, "renew") {
		return
	}
	frozen, err := h.freeze(r, links[i].Object, released)
	if h.failed(w, err, "renew") {
		return
	}
	if h.linkFailed(w, h.options.DB.RenewLink(r.Context(), links[i].ID, h.actor(r), frozen, released)) {
		return
	}
	h.note(r, db.LogLinkRenewed, links[i].Object, links[i].ID)
	w.WriteHeader(http.StatusNoContent)
}

// --- The address of a link

// shareView fills the page of a link.
type shareView struct {
	Version  string
	Title    string
	Token    string
	Mode     string
	Kind     string
	Frozen   template.HTML
	FrozenAt string
	Files    []sharedEntry
	Up       string
	Problem  string
	Notice   string
	Password bool
	SignIn   string
	Expires  string
}

type sharedEntry struct {
	ID, Title, Size, Href string
	Folder                bool
}

// isShare tells whether a request is for the address of a link, which needs no sign-in.
func isShare(r *http.Request) bool { return strings.HasPrefix(r.URL.Path, "/s/") }

// sharedLink finds the link of a request, or answers as for an address that never was.
func (h *handler) sharedLink(w http.ResponseWriter, r *http.Request) (db.Link, bool) {
	if h.options.DB == nil {
		h.notFound(w, r)
		return db.Link{}, false
	}
	l, err := h.options.DB.LinkByToken(r.Context(), r.PathValue("token"))
	if err != nil {
		if !errors.Is(err, db.ErrNotFound) {
			h.log.Error("link failed", slog.String("error", err.Error()))
		}
		h.message(w, http.StatusNotFound, message{Title: "Nothing here", Text: "This link does not lead anywhere: it may have expired or been revoked.", Link: "To the start"})
		return db.Link{}, false
	}
	return l, true
}

// passed tells whether the browser may open a link: it has no password, or the browser gave it.
func (h *handler) passed(r *http.Request, l db.Link) bool {
	if !l.HasPassword {
		return true
	}
	c, err := r.Cookie(shareCookie)
	return err == nil && h.options.DB.HasPass(r.Context(), l.ID, c.Value)
}

func (h *handler) sharePage(w http.ResponseWriter, status int, v shareView) {
	v.Version = h.assetVersion
	var out bytes.Buffer
	if err := h.pages.ExecuteTemplate(&out, "share", v); err != nil {
		h.log.Error("template failed", slog.String("template", "share"), slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(out.Bytes())
}

// takeLink is where an account of the tenant takes the role of a link to comment or edit: behind the gate, so the person
// is signed in. It holds that role on the shared object from now on, as long as the link lives, and goes on to it. A link
// to read, or a password not given yet, goes back to the address of the link.
func (h *handler) takeLink(w http.ResponseWriter, r *http.Request) {
	l, ok := h.sharedLink(w, r)
	if !ok {
		return
	}
	user := h.me(r)
	// An agent never takes a link for its person: that is sharing, which agents never do (ADR-36).
	if (l.Mode != db.LinkComment && l.Mode != db.LinkEdit) || !h.passed(r, l) || user.Agent() {
		http.Redirect(w, r, sharePath(r.PathValue("token")), http.StatusSeeOther)
		return
	}
	if err := h.options.DB.UseLink(r.Context(), l.ID, user.ID); err != nil {
		h.log.Error("link not used", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := h.options.DB.Log(r.Context(), user.ID, db.LogLinkUsed, l.Object, l.ID+" "+l.Mode); err != nil {
		h.log.Error("log not written", slog.String("error", err.Error()))
	}
	http.Redirect(w, r, "/o/"+l.Object, http.StatusSeeOther)
}

// share answers the address of a link.
func (h *handler) share(w http.ResponseWriter, r *http.Request) {
	l, ok := h.sharedLink(w, r)
	if !ok {
		return
	}
	token := r.PathValue("token")
	v := shareView{Title: l.ObjectTitle, Token: token, Mode: l.Mode, Kind: l.ObjectKind}
	if !l.Expires.IsZero() {
		v.Expires = l.Expires.UTC().Format("2006-01-02 15:04 UTC")
	}
	if !h.passed(r, l) {
		v.Password = true
		h.sharePage(w, http.StatusOK, v)
		return
	}
	// To comment or edit, an account of the tenant takes the role through the gate: the address of a link is open at the
	// gate, so nobody is signed in here, and /link/<token> is not.
	if l.Mode == db.LinkComment || l.Mode == db.LinkEdit {
		v.SignIn = "/link/" + token
	}
	h.note(r, db.LogLinkOpened, l.Object, l.ID)
	switch {
	case l.Mode == db.LinkUpload:
		v.Kind = "upload"
		if r.URL.Query().Get("sent") != "" {
			v.Notice = "Thank you, the file arrived."
		}
	case l.ObjectKind == db.KindPage || l.ObjectKind == db.KindNotes:
		v.Kind = "page"
		v.Frozen = template.HTML(frozenForLink(h.frozenTables(r, l.Frozen, l.CreatedBy), sharePath(token), l.Object)) // #nosec G203 -- rendered and sanitized by the same pipeline as every page; only addresses change
		if !l.FrozenAt.IsZero() {
			v.FrozenAt = l.FrozenAt.UTC().Format("2006-01-02 15:04 UTC")
		}
	case l.ObjectKind == db.KindFile:
		v.Kind = "file"
		f, err := h.options.DB.SharedFile(r.Context(), l.Object)
		if err != nil {
			h.notFound(w, r)
			return
		}
		v.Files = []sharedEntry{{ID: l.Object, Title: f.Name, Size: sizeText(f.Size), Href: sharePath(token) + "/f/" + l.Object}}
	case l.Area == db.AreaFiles:
		v.Kind = "folder"
		folder := l.Object
		if at := r.URL.Query().Get("in"); at != "" {
			if !id.Valid(at) || !h.options.DB.Below(r.Context(), at, l.Object, l.CreatedBy) {
				h.notFound(w, r)
				return
			}
			folder = at
			v.Up = sharePath(token)
		}
		entries, err := h.options.DB.SharedFiles(r.Context(), folder, l.CreatedBy)
		if err != nil {
			h.log.Error("shared files failed", slog.String("error", err.Error()))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		for _, e := range entries {
			s := sharedEntry{ID: e.ID, Title: e.Title, Folder: e.Kind == db.KindFolder}
			if s.Folder {
				s.Href = sharePath(token) + "?in=" + e.ID
			} else {
				s.Size, s.Href = sizeText(e.Size), sharePath(token)+"/f/"+e.ID
			}
			v.Files = append(v.Files, s)
		}
	default:
		// A collection or folder of the documents is shared to comment or edit only, through the workspace.
		v.Kind = "workspace"
	}
	h.sharePage(w, http.StatusOK, v)
}

// sharePassword checks the password of a link, with the limit against guessing of the sign-in, and lets the browser in.
func (h *handler) sharePassword(w http.ResponseWriter, r *http.Request) {
	l, ok := h.sharedLink(w, r)
	if !ok {
		return
	}
	token := r.PathValue("token")
	v := shareView{Title: l.ObjectTitle, Token: token, Mode: l.Mode, Kind: l.ObjectKind, Password: true}
	if !sameOrigin(r) {
		v.Problem = "This form came from another site."
		h.sharePage(w, http.StatusForbidden, v)
		return
	}
	key := "link:" + h.tenant(r) + ":" + l.ID
	if wait := h.throttle.Wait(key); wait > 0 {
		v.Problem = "Too many tries. Wait a little and try again."
		h.sharePage(w, http.StatusTooManyRequests, v)
		return
	}
	if !l.HasPassword {
		http.Redirect(w, r, sharePath(token), http.StatusSeeOther)
		return
	}
	if !auth.VerifyPassword(l.PasswordHash, r.PostFormValue("password")) {
		h.throttle.Failed(key)
		h.note(r, db.LogLinkPassword, l.Object, l.ID)
		v.Problem = "That is not the password of this link."
		h.sharePage(w, http.StatusUnauthorized, v)
		return
	}
	h.throttle.Succeeded(key)
	value, err := h.options.DB.GivePass(r.Context(), l.ID)
	if err != nil {
		h.log.Error("pass failed", slog.String("error", err.Error()))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// The pass holds on the address of the link, and where an account takes its role behind the gate.
	for _, path := range []string{sharePath(token), "/link/" + token} {
		http.SetCookie(w, &http.Cookie{Name: shareCookie, Value: value, Path: path, HttpOnly: true, Secure: r.TLS != nil || !isLocal(r),
			SameSite: http.SameSiteStrictMode, MaxAge: 24 * 60 * 60})
	}
	http.Redirect(w, r, sharePath(token), http.StatusSeeOther)
}

// isLocal tells whether a request comes to the own machine, where cookies go over http.
func isLocal(r *http.Request) bool { return strings.HasPrefix(requestOrigin(r), "http://") }

// The addresses of a frozen page as rendered for the workspace: an attachment of a page, and a link to a place of the
// application. The page was rendered and sanitized, so every attribute is quoted and escaped, and no text holds a quote.
var (
	frozenAttachment = regexp.MustCompile(`"/api/pages/([0-9a-hjkmnp-tv-z]{10}/attachments/a[1-9][0-9]{0,8})"`)
	frozenLink       = regexp.MustCompile(`<a [^>]*>`)
	frozenHref       = regexp.MustCompile(` href="(/[^/"][^"]*|/)"`)
)

// frozenForLink shows a frozen page through its link (ADR-29): its attachments, and those of the pages it released,
// come through the address of the link, since whoever has the link has no account; a link to a heading of the page
// itself stays in the page; every other link into the application becomes plain text, as it would lead to a sign-in.
func frozenForLink(frozen, path, pageID string) string {
	out := frozenAttachment.ReplaceAllString(frozen, `"`+path+`/p/${1}"`)
	return frozenLink.ReplaceAllStringFunc(out, func(tag string) string {
		href := frozenHref.FindStringSubmatchIndex(tag)
		if href == nil {
			return tag
		}
		target := html.UnescapeString(tag[href[2]:href[3]])
		if strings.HasPrefix(target, path+"/") {
			return tag
		}
		if page, anchor, ok := strings.Cut(strings.TrimPrefix(target, "/o/"), "#"); ok && strings.HasPrefix(target, "/o/") && page == pageID && anchor != "" {
			if !strings.HasPrefix(anchor, "h-") {
				anchor = markdown.Slug(anchor)
			}
			return tag[:href[0]] + ` href="#` + html.EscapeString(anchor) + `"` + tag[href[1]:]
		}
		return `<a data-not-shared title="Not part of this link">`
	})
}

// shareAttachment sends an attachment that a frozen page of a link shows: of the page itself or of a page it released,
// as the page was frozen. Its maker must still see that page.
func (h *handler) shareAttachment(w http.ResponseWriter, r *http.Request) {
	l, ok := h.sharedLink(w, r)
	if !ok {
		return
	}
	pageID, attachmentID := r.PathValue("page"), r.PathValue("attachment")
	if !h.passed(r, l) || l.Mode != db.LinkRead || (l.ObjectKind != db.KindPage && l.ObjectKind != db.KindNotes) || h.options.Store == nil ||
		!id.Valid(pageID) || !strings.Contains(l.Frozen, `"`+db.AttachmentPath(pageID, attachmentID)+`"`) ||
		!h.options.DB.Below(r.Context(), pageID, pageID, l.CreatedBy) {
		h.notFound(w, r)
		return
	}
	a, err := h.options.DB.SharedAttachment(r.Context(), pageID, attachmentID)
	if err != nil {
		h.notFound(w, r)
		return
	}
	h.serveBlob(w, r, a.Blob, a.Name, a.ContentType)
}

// shareFile sends a file of a link: the file of the link, or a file below its folder, as the current version.
func (h *handler) shareFile(w http.ResponseWriter, r *http.Request) {
	l, ok := h.sharedLink(w, r)
	if !ok {
		return
	}
	fileID := r.PathValue("file")
	if !h.passed(r, l) || l.Mode == db.LinkUpload || h.options.Store == nil || !id.Valid(fileID) || !h.options.DB.Below(r.Context(), fileID, l.Object, l.CreatedBy) {
		h.notFound(w, r)
		return
	}
	v, err := h.options.DB.SharedFile(r.Context(), fileID)
	if err != nil {
		h.notFound(w, r)
		return
	}
	h.serveBlob(w, r, v.Blob, v.Name, v.ContentType)
}

// shareUpload takes one file into the folder of a link to upload only. It becomes a file of whoever made the link, with a
// name nothing in the folder has, so it never replaces what is there; the visitor sees nothing of the folder.
func (h *handler) shareUpload(w http.ResponseWriter, r *http.Request) {
	l, ok := h.sharedLink(w, r)
	if !ok {
		return
	}
	token := r.PathValue("token")
	v := shareView{Title: l.ObjectTitle, Token: token, Mode: l.Mode, Kind: "upload"}
	if l.Mode != db.LinkUpload || !h.passed(r, l) || h.options.Store == nil {
		h.notFound(w, r)
		return
	}
	if !sameOrigin(r) {
		v.Problem = "This form came from another site."
		h.sharePage(w, http.StatusForbidden, v)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxLinkUpload+1<<20)
	reader, err := r.MultipartReader()
	if err != nil {
		v.Problem = "Choose a file to send."
		h.sharePage(w, http.StatusBadRequest, v)
		return
	}
	for {
		part, err := reader.NextPart()
		if err != nil {
			v.Problem = "Choose a file to send."
			h.sharePage(w, http.StatusBadRequest, v)
			return
		}
		if part.FormName() != "file" || part.FileName() == "" {
			_ = part.Close()
			continue
		}
		err = h.uploadThroughLink(r, l, part.FileName(), part)
		_ = part.Close()
		switch {
		case errors.Is(err, db.ErrQuota):
			v.Problem = "There is no room for this file."
			h.sharePage(w, http.StatusConflict, v)
		case errors.Is(err, db.ErrTooLarge):
			v.Problem = "This file is too large for a link."
			h.sharePage(w, http.StatusRequestEntityTooLarge, v)
		case errors.Is(err, db.ErrFileName):
			v.Problem = "A file cannot have this name."
			h.sharePage(w, http.StatusBadRequest, v)
		case err != nil:
			h.log.Error("upload through a link failed", slog.String("error", err.Error()))
			v.Problem = "The file did not arrive. Try again."
			h.sharePage(w, http.StatusInternalServerError, v)
		default:
			h.note(r, db.LogLinkUpload, l.Object, l.ID+" "+part.FileName())
			http.Redirect(w, r, sharePath(token)+"?sent=1", http.StatusSeeOther)
		}
		return
	}
}

// uploadThroughLink stores one file from a link to upload only, in the name of whoever made the link.
func (h *handler) uploadThroughLink(r *http.Request, l db.Link, name string, data io.Reader) error {
	ctx := r.Context()
	name, err := db.CleanFileName(name)
	if err != nil {
		return err
	}
	declared := min(r.ContentLength, MaxLinkUpload)
	if declared < 0 {
		declared = MaxLinkUpload
	}
	u, err := h.options.DB.StartUpload(ctx, l.CreatedBy, db.UploadTarget{Parent: l.Object}, name, declared)
	if err != nil {
		return err
	}
	t := h.tenant(r)
	done := false
	defer func() {
		if !done {
			_ = h.options.DB.CancelUpload(ctx, u.ID, l.CreatedBy)
			_ = h.options.Store.RemoveUpload(t, u.ID)
		}
	}()
	f, err := h.options.Store.OpenUpload(t, u.ID)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(data, MaxLinkUpload+1))
	if err == nil && n > MaxLinkUpload {
		err = db.ErrTooLarge
	}
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err := h.options.DB.SettleUpload(ctx, u.ID, l.CreatedBy, n); err != nil {
		return err
	}
	path, err := h.options.Store.UploadPath(t, u.ID)
	if err != nil {
		return err
	}
	sha, size, err := store.HashFile(path)
	if err != nil {
		return err
	}
	contentType, err := detect(path, name)
	if err != nil {
		return err
	}
	// Only adds: the file takes a name nothing in the folder has when it is made, never a version of what is there.
	finished, err := h.options.DB.FinishUploadAsNew(ctx, u.ID, l.CreatedBy, sha, size, contentType, func() error {
		return h.options.Store.Adopt(t, u.ID, sha)
	})
	if err != nil {
		return err
	}
	done = true
	if err := h.options.Store.RemoveUpload(t, u.ID); err != nil {
		h.log.Error("upload not removed", slog.String("error", err.Error()))
	}
	h.queueText(ctx, account.User{ID: l.CreatedBy}, finished, sha, contentType)
	return nil
}

// sizeText says how large a file is, for people.
func sizeText(n int64) string {
	if n < 1024 {
		return strconv.FormatInt(n, 10) + " B"
	}
	value, suffix := float64(n)/1024, "KB"
	for _, s := range []string{"MB", "GB", "TB"} {
		if value < 1024 {
			break
		}
		value, suffix = value/1024, s
	}
	return strconv.FormatFloat(value, 'f', 1, 64) + " " + suffix
}
