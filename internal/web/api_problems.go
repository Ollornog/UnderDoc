// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"regexp"
	"slices"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/markdown"
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
	for _, p := range h.library(r.Context()).Problems() {
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
		bodies map[string]string
		order  []string
	}
	known := map[string]*target{}
	look := func(id string) *target {
		if t, ok := known[id]; ok {
			return t
		}
		var t *target
		if o, err := h.options.DB.Object(ctx, id, user.ID); err == nil {
			t = &target{title: o.Title, page: o.Kind == db.KindPage, blocks: map[string]bool{}, bodies: map[string]string{}}
			if t.page {
				bs, _ := h.options.DB.Blocks(ctx, id, user.ID)
				for _, b := range bs {
					t.blocks[b.ID] = true
					t.bodies[b.ID] = b.Body
					t.order = append(t.order, b.ID)
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
		for _, r := range db.RefsIn(text) {
			a := db.Address{Object: r[0], Block: db.BlockOf(r[1])}
			if a.Block != r[1] {
				a.Sub = r[1][len(a.Block)+1:]
			}
			ref := a.String()
			switch t := look(a.Object); {
			case t == nil:
				add(b, ref, "embeds something that is not to be found")
			case !t.page:
				add(b, ref, "embeds "+t.title+", which is not a page")
			case a.Block != "" && !t.blocks[a.Block]:
				add(b, ref, "embeds block #"+a.Block+", which is no longer in "+t.title)
			case a.Sub == db.Section:
				at := slices.Index(t.order, a.Block)
				bodies := []string{}
				for _, id := range t.order[at:] {
					bodies = append(bodies, t.bodies[id])
				}
				if markdown.SectionEnd(bodies) == 0 {
					add(b, ref, "embeds the section of block #"+a.Block+", which opens no section in "+t.title)
				}
			case a.Sub != "":
				if _, ok := markdown.FindPart(t.bodies[a.Block], a.Sub); !ok {
					add(b, ref, "embeds part #"+a.Target()+", which is no longer in "+t.title)
				}
			}
		}
		for _, m := range databaseLink.FindAllStringSubmatch(text, -1) {
			if look(m[1]) == nil {
				add(b, "/o/"+m[1], "links to a page that is not to be found")
			}
		}
		if rendered, err := h.library(r.Context()).RenderMarkdown(text); err == nil {
			for _, broken := range rendered.Broken {
				add(b, broken.Reference, broken.Reason)
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}
