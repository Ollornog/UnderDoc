// SPDX-License-Identifier: MIT

package web

import (
	"cmp"
	"context"
	"html"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/document"
	"github.com/Ollornog/UnderDoc/internal/library"
	"github.com/Ollornog/UnderDoc/internal/search"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// apiSearchResult is the answer of /api/search: the hits, the values the filters offer, and which engine answered.
type apiSearchResult struct {
	// Engine is meilisearch, opensearch, titles (the fallback of ADR-24) or folder (no database, M-1).
	Engine string `json:"engine"`
	// Notice says why the search is limited, for the results page to show.
	Notice string                `json:"notice,omitempty"`
	Hits   []apiHit              `json:"hits"`
	Facets map[string][]apiFacet `json:"facets"`
}

// apiHit is one hit: the object to open, the block or heading to open at, and a snippet with the matches marked.
type apiHit struct {
	ID      string   `json:"id"`
	Anchor  string   `json:"anchor,omitempty"`
	Title   string   `json:"n"`
	Kind    string   `json:"t"`
	Heading string   `json:"heading,omitempty"`
	Place   string   `json:"place"`
	Snippet string   `json:"snippet"`
	By      string   `json:"by,omitempty"`
	Changed string   `json:"changed,omitempty"`
	Tags    []string `json:"tags"`
	// Code is true when the hit came through an error code of the query.
	Code bool `json:"code,omitempty"`
}

// apiFacet is one value a filter offers.
type apiFacet struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

const (
	maxQuickHits = 50
	maxFacets    = 30
)

// apiSearch searches what the reader may read: through the search service when there is one (ADR-24), else titles in the
// database, and without a database the full text of the folder (M-1).
func (h *handler) apiSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if h.options.Search == nil {
		writeJSON(w, http.StatusOK, h.folderSearch(h.library(r.Context()), query.Get("q")))
		return
	}
	user, _ := account.From(r.Context())
	limit, _ := strconv.Atoi(query.Get("limit"))
	request := search.Request{Text: query.Get("q"), Account: user.ID, Limit: min(cmp.Or(limit, 20), maxQuickHits), Exact: query.Get("exact") == "1",
		Filters: search.Filters{Types: query["type"], Collections: query["collection"], Authors: query["author"], Tags: query["tag"]}}
	if from, err := time.Parse(time.DateOnly, query.Get("from")); err == nil {
		request.Filters.From = from
	}
	if to, err := time.Parse(time.DateOnly, query.Get("to")); err == nil {
		request.Filters.To = to.Add(24*time.Hour - time.Second)
	}
	// The filter "AI changes to check" (ADR-33): the pages the database names, of what the reader sees.
	if query.Get("check") == "ai" && h.options.DB != nil {
		pages, err := h.options.DB.PagesToCheck(r.Context(), user.ID)
		if h.failed(w, err, "search") {
			return
		}
		if len(pages) == 0 {
			writeJSON(w, http.StatusOK, apiSearchResult{Engine: h.options.Search.Engine(), Hits: []apiHit{}, Facets: map[string][]apiFacet{}})
			return
		}
		request.Filters.Objects = pages
	}
	result, err := h.searchAs(r.Context(), request)
	if h.failed(w, err, "search") {
		return
	}
	answer := apiSearchResult{Engine: result.Engine, Notice: result.Notice, Hits: []apiHit{}, Facets: h.facets(r.Context(), result.Facets, user.ID)}
	for _, hit := range result.Hits {
		answer.Hits = append(answer.Hits, apiHit{ID: hit.Object, Anchor: hit.Anchor, Title: hit.Title, Kind: hit.Type,
			Heading: hit.Heading, Place: placeOf(hit), Snippet: markHTML(hit.Snippet), By: hit.AuthorName,
			Changed: changedOf(hit.Changed), Tags: nonEmpty(hit.Tags), Code: hit.Code})
	}
	writeJSON(w, http.StatusOK, answer)
}

// searchAs runs a search as the user of ctx, through the search service: with their groups, as an agent or a service
// account where they are one, and each hit checked once more against the one check of the database. The browser and the
// MCP access search through it alike (ADR-34).
func (h *handler) searchAs(ctx context.Context, request search.Request) (search.Result, error) {
	user, _ := account.From(ctx)
	request.Account = user.ID
	request.Agent = user.Agent()
	request.Service = user.Kind == account.KindService
	if h.options.DB != nil {
		// A service account finds through the groups whose allow holds for it only: never a role of the gate (B-3). The
		// check of each hit below holds every deny.
		groupsOf, directOf := h.options.DB.GroupsOf, h.options.DB.DirectGroupsOf
		if request.Service {
			groupsOf, directOf = h.options.DB.AllowGroupsOf, h.options.DB.AllowDirectGroupsOf
		}
		groups, err := groupsOf(ctx, user.ID)
		if err != nil {
			return search.Result{}, err
		}
		direct, err := directOf(ctx, user.ID)
		if err != nil {
			return search.Result{}, err
		}
		request.Groups, request.Direct = groups, direct
	}
	result, err := h.options.Search.Search(ctx, request)
	if err != nil {
		return search.Result{}, err
	}
	// The index may lag behind a change of rights for a moment: each hit is checked once more against the one check of
	// the database, so a deny holds at once (ADR-36).
	if h.options.DB != nil {
		kept := result.Hits[:0]
		for _, hit := range result.Hits {
			ok, err := h.options.DB.MayFind(ctx, hit.Object, user.ID)
			if err != nil {
				return search.Result{}, err
			}
			if ok {
				kept = append(kept, hit)
			}
		}
		result.Hits = kept
		// The counts beside the hits come from them, after the check: the index counts by the reader's tokens, which
		// leave out the denies of groups that drop out of them (archived ones, and for a service account everyone and
		// the roles of the gate), so its counts could tell of what the reader may not find (B-4).
		result.Facets = facetsOf(result.Hits)
	}
	return result, nil
}

// facetsOf counts hits by type, collection, author and tag, as the engine counts records.
func facetsOf(hits []search.Hit) search.Facets {
	facets := search.Facets{}
	bump := func(field, value string) {
		if value == "" {
			return
		}
		if facets[field] == nil {
			facets[field] = map[string]int{}
		}
		facets[field][value]++
	}
	for _, hit := range hits {
		bump(search.FacetType, hit.Type)
		bump(search.FacetCollection, hit.Collection)
		bump(search.FacetAuthor, hit.Author)
		for _, tag := range hit.Tags {
			bump(search.FacetTag, tag)
		}
	}
	return facets
}

func placeOf(hit search.Hit) string {
	if hit.Path != "" {
		return hit.Path
	}
	if hit.CollectionName != "" && hit.Object != hit.Collection {
		return hit.CollectionName
	}
	return ""
}

func changedOf(t time.Time) string {
	if t.Unix() <= 0 {
		return ""
	}
	return t.UTC().Format(time.DateOnly)
}

func nonEmpty(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

// markHTML writes a snippet as HTML: every piece escaped, the matches in mark.
func markHTML(fragments []search.Fragment) string {
	var b strings.Builder
	for _, f := range fragments {
		if f.Match {
			b.WriteString("<mark>" + html.EscapeString(f.Text) + "</mark>")
		} else {
			b.WriteString(html.EscapeString(f.Text))
		}
	}
	return b.String()
}

// facets turns the counts of the engine into the choices of the filters, with names for collections and authors.
func (h *handler) facets(ctx context.Context, counts search.Facets, viewer string) map[string][]apiFacet {
	out := map[string][]apiFacet{}
	for _, field := range []string{search.FacetType, search.FacetCollection, search.FacetAuthor, search.FacetTag} {
		list := []apiFacet{}
		for value, n := range counts[field] {
			label := value
			switch field {
			case search.FacetCollection:
				label = h.collectionName(ctx, value, viewer)
			case search.FacetAuthor:
				label = h.options.DB.AccountName(ctx, value)
			}
			if label == "" {
				continue
			}
			list = append(list, apiFacet{Value: value, Label: label, Count: n})
		}
		slices.SortFunc(list, func(a, b apiFacet) int { return cmp.Or(cmp.Compare(b.Count, a.Count), cmp.Compare(a.Label, b.Label)) })
		if len(list) > maxFacets {
			list = list[:maxFacets]
		}
		out[field] = list
	}
	return out
}

func (h *handler) collectionName(ctx context.Context, id, viewer string) string {
	if id == library.FolderCollection {
		return library.FolderLabel
	}
	o, err := h.options.DB.Object(ctx, id, viewer)
	if err != nil {
		return ""
	}
	return o.Title
}

// folderSearch is the search of M-1: the full text of the folder, in memory, for an installation without a database.
func (h *handler) folderSearch(lib *library.Library, q string) apiSearchResult {
	result := apiSearchResult{Engine: "folder", Hits: []apiHit{}, Facets: map[string][]apiFacet{}}
	for _, hit := range lib.Search(q, maxSearchResults) {
		fragments := make([]search.Fragment, len(hit.Snippet))
		for i, f := range hit.Snippet {
			fragments[i] = search.Fragment{Text: f.Text, Match: f.Match}
		}
		result.Hits = append(result.Hits, apiHit{ID: string(hit.Document.ID), Title: hit.Document.Title, Kind: search.TypePage,
			Place: lib.Place(hit.Document.ID), Snippet: markHTML(fragments), Tags: nonEmpty(hit.Document.Tags)})
	}
	return result
}

// TitleFallback is the search without a search service (ADR-24): titles of the database and of the folder.
func TitleFallback(database *db.DB, store *library.Store) search.Fallback {
	return titleFallback{database, store}
}

type titleFallback struct {
	db    *db.DB
	store *library.Store
}

func (f titleFallback) Titles(ctx context.Context, text, viewer string, tags []string, limit int) ([]search.Hit, error) {
	found, err := f.db.SearchTitles(ctx, text, viewer, tags, limit)
	if err != nil {
		return nil, err
	}
	hits := make([]search.Hit, 0, len(found))
	for _, t := range found {
		hits = append(hits, search.Hit{Object: t.Object.ID, Type: t.Object.Kind, Title: t.Object.Title, Path: t.Path,
			Collection: t.Top.ID, CollectionName: t.Top.Title, Tags: t.Object.Tags, Changed: t.Object.Changed,
			AuthorName: f.db.AccountName(ctx, t.Object.Owner)})
	}
	// The folder belongs to the instance: only Main shows it (docs/tenants.md).
	if f.store == nil {
		return hits, nil
	}
	lib := f.store.Library()
	if id, _ := tenant.From(ctx); lib == nil || id != tenant.Main {
		return hits, nil
	}
	folded := strings.ToLower(text)
	var docs []*document.Document
	for _, doc := range lib.Documents() {
		if strings.Contains(strings.ToLower(doc.Title), folded) && !slices.ContainsFunc(tags, func(t string) bool { return !slices.Contains(doc.Tags, t) }) {
			docs = append(docs, doc)
		}
	}
	slices.SortFunc(docs, func(a, b *document.Document) int {
		return cmp.Or(-cmp.Compare(boolInt(strings.EqualFold(a.Title, text)), boolInt(strings.EqualFold(b.Title, text))),
			cmp.Compare(a.Title, b.Title), cmp.Compare(a.ID, b.ID))
	})
	for _, doc := range docs {
		if len(hits) >= limit {
			break
		}
		hits = append(hits, search.Hit{Object: string(doc.ID), Type: search.TypePage, Title: doc.Title, Path: lib.Place(doc.ID),
			Collection: library.FolderCollection, CollectionName: library.FolderLabel, Tags: doc.Tags, Changed: doc.Modified})
	}
	return hits, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
