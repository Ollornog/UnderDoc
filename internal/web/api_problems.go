// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"regexp"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
)

// maxProblemBlocks is how many blocks the list of problems reads at most; beyond that it says there is more.
const maxProblemBlocks = 20000

var (
	fencedCode   = regexp.MustCompile("(?s)(?m)^[ \t]*(```|~~~).*?^[ \t]*(```|~~~)[ \t]*$")
	databaseLink = regexp.MustCompile(`\]\(/o/([0-9a-hjkmnp-tv-z]{10})[)/#?]`)
)

type apiProblem struct {
	Page  string `json:"page"`
	Title string `json:"title"`
	Block string `json:"block"`
	Ref   string `json:"ref"`
	Why   string `json:"why"`
}

type apiSourceProblem struct {
	Source  string `json:"source"`
	Message string `json:"message"`
}

type apiProblems struct {
	Database []apiProblem       `json:"database"`
	Source   []apiSourceProblem `json:"source"`
	More     bool               `json:"more"`
}

// apiProblems lists what does not resolve (M-3): in the pages of the database the viewer sees, embeddings and links to
// pages or blocks that are gone, never there or not to be seen by the viewer, and links of the Markdown that find
// nothing; and what the folder source ran into. A broken reference is a visible error, never a silent gap.
func (h *handler) apiProblems(w http.ResponseWriter, r *http.Request) {
	out := apiProblems{Database: []apiProblem{}, Source: []apiSourceProblem{}}
	for _, p := range h.store.Library().Problems() {
		out.Source = append(out.Source, apiSourceProblem{Source: p.Source, Message: p.Message})
	}
	if h.options.DB == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	ctx := r.Context()
	user, _ := account.From(ctx)
	blocks, err := h.options.DB.AllBlocks(ctx, user.ID, maxProblemBlocks+1)
	if h.failed(w, err, "problems") {
		return
	}
	if len(blocks) > maxProblemBlocks {
		blocks, out.More = blocks[:maxProblemBlocks], true
	}
	type target struct {
		title  string
		page   bool
		blocks map[string]bool
	}
	known := map[string]*target{}
	look := func(id string) *target {
		if t, ok := known[id]; ok {
			return t
		}
		var t *target
		if o, err := h.options.DB.Object(ctx, id, user.ID); err == nil {
			t = &target{title: o.Title, page: o.Kind == db.KindPage, blocks: map[string]bool{}}
			if t.page {
				bs, _ := h.options.DB.Blocks(ctx, id, user.ID)
				for _, b := range bs {
					t.blocks[b.ID] = true
				}
			}
		}
		known[id] = t
		return t
	}
	seen := map[[3]string]bool{}
	add := func(b db.PageBlock, ref, why string) {
		key := [3]string{b.Page, b.Block, ref}
		if seen[key] {
			return
		}
		seen[key] = true
		out.Database = append(out.Database, apiProblem{Page: b.Page, Title: b.Title, Block: b.Block, Ref: ref, Why: why})
	}
	for _, b := range blocks {
		text := fencedCode.ReplaceAllString(b.Body, "")
		for _, m := range db.EmbedLine.FindAllStringSubmatch(text, -1) {
			ref := m[1]
			if m[2] != "" {
				ref += "#" + m[2]
			}
			switch t := look(m[1]); {
			case t == nil:
				add(b, ref, "embeds something that is not to be found")
			case !t.page:
				add(b, ref, "embeds "+t.title+", which is not a page")
			case m[2] != "" && !t.blocks[m[2]]:
				add(b, ref, "embeds block #"+m[2]+", which is no longer in "+t.title)
			}
		}
		for _, m := range databaseLink.FindAllStringSubmatch(text, -1) {
			if look(m[1]) == nil {
				add(b, "/o/"+m[1], "links to a page that is not to be found")
			}
		}
		if rendered, err := h.store.Library().RenderMarkdown(text); err == nil {
			for _, broken := range rendered.Broken {
				add(b, broken.Reference, broken.Reason)
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}
