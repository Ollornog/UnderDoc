// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"slices"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/markdown"
	"github.com/Ollornog/UnderDoc/internal/route"
)

// Parts, sections and the places they are used (ADR-22). Offsets come from the browser, in UTF-16 code units of the
// block's Markdown, and go back the same way.

type apiPart struct {
	ID string `json:"id"`
	// Name is set for a value (M-3).
	Name string `json:"name,omitempty"`
	// UsedIn names the pages that embed this part.
	UsedIn []string `json:"usedIn"`
}

// parts lists the parts of a block with where each is used.
func parts(page string, b db.Block, used []db.Ref) []apiPart {
	out := []apiPart{}
	for _, p := range markdown.Parts(b.Body) {
		part := apiPart{ID: p.ID, Name: p.Name, UsedIn: []string{}}
		for _, u := range used {
			if u.Target == page && u.TargetBlock == b.ID+"."+p.ID && !slices.Contains(part.UsedIn, u.Title) {
				part.UsedIn = append(part.UsedIn, u.Title)
			}
		}
		out = append(out, part)
	}
	return out
}

type apiSelection struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Name  string `json:"name"`
}

// apiMarkPart marks the selected stretch of a block as a part, so it can be embedded (ADR-22): its edges snap outwards at
// atomic elements, and it stays in its paragraph. It answers the block's new Markdown, the part, its address and where its
// text now lies.
func (h *handler) apiMarkPart(w http.ResponseWriter, r *http.Request) {
	var in apiSelection
	user, ok := h.change(w, r, 1<<10, &in)
	if !ok {
		return
	}
	page, block := r.PathValue("id"), r.PathValue("block")
	body, ok := h.blockBody(w, r, page, block)
	if !ok {
		return
	}
	b, part, err := h.options.DB.MarkPart(r.Context(), page, block, user.ID, markdown.ByteOffset(body, in.Start), markdown.ByteOffset(body, in.End), in.Name)
	if h.failed(w, err, "mark part") {
		return
	}
	address := db.Address{Object: page, Block: block, Sub: part.ID}
	writeJSON(w, http.StatusCreated, map[string]any{
		"part": part.ID, "name": part.Name, "ref": address.String(), "url": route.Object(page) + "#" + address.Target(),
		"md": b.Body, "start": markdown.UnitOffset(b.Body, part.Open[1]), "end": markdown.UnitOffset(b.Body, part.Close[0]),
	})
}

// apiExcerpt answers the Markdown of a selected stretch of a block as a copy (ADR-22): plain text with no tie, with
// formatting open at its edges closed and opened again, without markers of parts. Who sees the page may copy from it.
func (h *handler) apiExcerpt(w http.ResponseWriter, r *http.Request) {
	var in apiSelection
	if _, ok := h.change(w, r, 1<<10, &in); !ok {
		return
	}
	page, block := r.PathValue("id"), r.PathValue("block")
	body, ok := h.blockBody(w, r, page, block)
	if !ok {
		return
	}
	id := markdown.NextPartID(body, 1)
	marked, _, err := markdown.MarkPart(body, markdown.ByteOffset(body, in.Start), markdown.ByteOffset(body, in.End), id, "")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	md, err := markdown.PartMarkdown(marked, id)
	if h.failed(w, err, "excerpt") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"md": markdown.StripParts(md), "from": db.Address{Object: page, Block: block}.String()})
}

// blockBody reads the Markdown of one block the viewer sees, or answers the request.
func (h *handler) blockBody(w http.ResponseWriter, r *http.Request, page, block string) (string, bool) {
	user, _ := account.From(r.Context())
	blocks, err := h.options.DB.Blocks(r.Context(), page, user.ID)
	if h.failed(w, err, "blocks") {
		return "", false
	}
	for _, b := range blocks {
		if b.ID == block {
			return b.Body, true
		}
	}
	h.failed(w, db.ErrNotFound, "blocks")
	return "", false
}

type apiUse struct {
	Page    string `json:"page"`
	Title   string `json:"title"`
	Block   string `json:"block"`
	CanEdit bool   `json:"canEdit"`
}

// usesOf answers where an address is embedded, place by place, for GET /api/objects/{id}/uses?ref=<block>[.<part>].
func (h *handler) usesOf(w http.ResponseWriter, r *http.Request, object, target string) {
	user, _ := account.From(r.Context())
	a, ok := db.ParseAddress(object + "#" + target)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not an address"})
		return
	}
	uses, hidden, err := h.options.DB.UsesOf(r.Context(), a, user.ID)
	if h.failed(w, err, "uses") {
		return
	}
	out := []apiUse{}
	for _, u := range uses {
		out = append(out, apiUse{Page: u.Page, Title: u.Title, Block: u.Block, CanEdit: u.CanEdit})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ref": a.String(), "uses": out, "hidden": hidden})
}
