// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"strconv"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
)

// Comments on pages of the database, on the whole page or on one block (ADR-18, ADR-20). The body is plain text: the
// workspace shows it as text, never as HTML.

type apiComment struct {
	ID    int64  `json:"id"`
	Block string `json:"block"`
	// Gone tells that the block it belongs to is no longer in the page.
	Gone   bool   `json:"gone,omitempty"`
	Parent int64  `json:"parent,omitempty"`
	Who    string `json:"who"`
	// Mine tells the workspace to offer editing and deleting; each change checks again.
	Mine       bool   `json:"mine,omitempty"`
	Body       string `json:"body"`
	At         string `json:"at"`
	Edited     string `json:"edited,omitempty"`
	Resolved   string `json:"resolved,omitempty"`
	ResolvedBy string `json:"resolvedBy,omitempty"`
	CanResolve bool   `json:"canResolve,omitempty"`
}

type apiComments struct {
	Comments   []apiComment `json:"comments"`
	CanComment bool         `json:"canComment"`
}

// commentOf turns a comment into what the workspace reads, for this viewer.
func commentOf(c db.Comment, viewer string, mayComment, mayEdit bool) apiComment {
	out := apiComment{ID: c.ID, Block: c.Block, Gone: c.Gone, Parent: c.Parent, Who: c.AuthorName, Mine: c.Author == viewer,
		Body: c.Body, At: stamp(c.Created), ResolvedBy: c.ResolvedBy, CanResolve: db.MayResolve(c, viewer, mayComment, mayEdit)}
	if !c.Edited.IsZero() {
		out.Edited = stamp(c.Edited)
	}
	if !c.Resolved.IsZero() {
		out.Resolved = stamp(c.Resolved)
	}
	return out
}

// commentAnswer writes one comment with what the viewer may do with it, judged on its page.
func (h *handler) commentAnswer(w http.ResponseWriter, r *http.Request, status int, c db.Comment) {
	user, _ := account.From(r.Context())
	o, err := h.options.DB.Object(r.Context(), c.Page, user.ID)
	if h.failed(w, err, "comment") {
		return
	}
	writeJSON(w, status, commentOf(c, user.ID, h.options.DB.MayComment(r.Context(), o, user.ID), h.options.DB.MayEdit(r.Context(), o, user.ID)))
}

// apiComments lists the comments of a page, threads in the order they began, each with its replies.
func (h *handler) apiComments(w http.ResponseWriter, r *http.Request) {
	if h.options.DB == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "this needs the database"})
		return
	}
	ctx := r.Context()
	user, _ := account.From(ctx)
	comments, err := h.options.DB.Comments(ctx, r.PathValue("id"), user.ID)
	if h.failed(w, err, "comments") {
		return
	}
	o, err := h.options.DB.Object(ctx, r.PathValue("id"), user.ID)
	if h.failed(w, err, "comments") {
		return
	}
	mayComment, mayEdit := h.options.DB.MayComment(ctx, o, user.ID), h.options.DB.MayEdit(ctx, o, user.ID)
	out := apiComments{Comments: []apiComment{}, CanComment: mayComment}
	for _, c := range comments {
		out.Comments = append(out.Comments, commentOf(c, user.ID, mayComment, mayEdit))
	}
	writeJSON(w, http.StatusOK, out)
}

// apiAddComment puts a comment on a page or one of its blocks, or a reply under a comment.
func (h *handler) apiAddComment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Block  string `json:"block"`
		Body   string `json:"body"`
		Parent int64  `json:"parent"`
	}
	user, ok := h.change(w, r, 64<<10, &in)
	if !ok {
		return
	}
	c, err := h.options.DB.AddComment(r.Context(), r.PathValue("id"), user.ID, in.Block, in.Body, in.Parent)
	if h.failed(w, err, "new comment") {
		return
	}
	h.commentAnswer(w, r, http.StatusCreated, c)
}

// commentID reads the ID of a comment from the path; one that is no number is not found.
func commentID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return 0, false
	}
	return id, true
}

// apiChangeComment gives one's own comment new text, or resolves its thread or opens it again.
func (h *handler) apiChangeComment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Body     *string `json:"body"`
		Resolved *bool   `json:"resolved"`
	}
	user, ok := h.change(w, r, 64<<10, &in)
	if !ok {
		return
	}
	id, ok := commentID(w, r)
	if !ok {
		return
	}
	if (in.Body == nil) == (in.Resolved == nil) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "send either body or resolved"})
		return
	}
	var c db.Comment
	var err error
	if in.Body != nil {
		c, err = h.options.DB.EditComment(r.Context(), id, user.ID, *in.Body)
	} else {
		c, err = h.options.DB.ResolveComment(r.Context(), id, user.ID, *in.Resolved)
	}
	if h.failed(w, err, "comment") {
		return
	}
	h.commentAnswer(w, r, http.StatusOK, c)
}

// apiDeleteComment takes one's own comment away.
func (h *handler) apiDeleteComment(w http.ResponseWriter, r *http.Request) {
	user, ok := h.change(w, r, 0, nil)
	if !ok {
		return
	}
	id, ok := commentID(w, r)
	if !ok {
		return
	}
	if h.failed(w, h.options.DB.DeleteComment(r.Context(), id, user.ID), "delete comment") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
