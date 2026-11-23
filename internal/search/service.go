// SPDX-License-Identifier: MIT

package search

import (
	"cmp"
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// Request is a search as a reader asks it.
type Request struct {
	Text string
	// Account is the reader, Groups the groups it belongs to and Direct those it is in directly. What it may not read is
	// never found (ADR-24, ADR-29, ADR-36).
	Account string
	Groups  []string
	Direct  []string
	// Agent tells that the reader is an agent (ADR-36): it finds nothing a deny on Agents keeps from it.
	Agent   bool
	Filters Filters
	Limit   int
	// Exact finds the words only as written and in this order, without typing errors (the strategy "exact words" of
	// ADR-16).
	Exact bool
}

// Hit is one result: an object and the section in it where the match is.
type Hit struct {
	Object         string
	Anchor         string
	Type           string
	Title          string
	Heading        string
	Path           string
	Collection     string
	CollectionName string
	Author         string
	AuthorName     string
	Tags           []string
	Changed        time.Time
	Snippet        []Fragment
	// Code is true when the hit came through a code of the query.
	Code bool
}

// Result is the answer to a request.
type Result struct {
	Hits   []Hit
	Facets Facets
	// Engine is meilisearch, opensearch or titles.
	Engine string
	// Notice says why only titles were searched; empty when the search service answered.
	Notice string
}

// Fallback finds objects by their title and tags in the database, for when no search service is set up or it does not
// answer. Every tag of the filter must be there; an empty text lists what has them.
type Fallback interface {
	Titles(ctx context.Context, text, account string, tags []string, limit int) ([]Hit, error)
}

// Notices of the fallback.
const (
	EngineTitles          = "titles"
	NoticeNoService       = "No search service is set up, so only titles are searched."
	NoticeServiceDown     = "The search service does not answer, so only titles are searched for now."
	maxTierRecords        = 200
	defaultLimit          = 20
	maxRequestLimit       = 100
	maxQueryLength        = 256
	maxQueryWords         = 16
	queryWordsForSnippets = 8
)

// Service answers searches through an engine, or through the fallback without one. Each tenant searches its own index
// (ADR-23): the tenant of the context, else the tenant the service was made with.
type Service struct {
	engine   Engine
	fallback Fallback
	tenant   string
	log      *slog.Logger
}

// NewService returns a service. engine may be nil: then only the fallback answers.
func NewService(engine Engine, fallback Fallback, tenant string, log *slog.Logger) *Service {
	return &Service{engine: engine, fallback: fallback, tenant: tenant, log: log}
}

// Engine returns the name of the engine, or titles without one.
func (s *Service) Engine() string {
	if s.engine == nil {
		return EngineTitles
	}
	return s.engine.Name()
}

// DropTenant removes the index of a tenant deleted for good. Without an engine there is none.
func (s *Service) DropTenant(ctx context.Context, tenantID string) error {
	if s == nil || s.engine == nil {
		return nil
	}
	return s.engine.Drop(ctx, tenantID)
}

// ParsedQuery is a query split into its codes and its other words.
type ParsedQuery struct {
	// Codes are the canonical codes as written in the query, Parts their parts.
	Codes []string
	Parts []string
	// Words is the rest of the query, for the full text.
	Words []string
}

// Parse splits a query. A code is never also a word: it must not be found through a typing error.
func Parse(text string) ParsedQuery {
	if len(text) > maxQueryLength {
		text = strings.ToValidUTF8(text[:maxQueryLength], "")
	}
	var q ParsedQuery
	rest := []byte(text)
	for _, token := range FindCodes(text) {
		q.Codes = appendNew(q.Codes, token.Code)
		for _, part := range token.Parts {
			q.Parts = appendNew(q.Parts, part)
		}
		for i := token.Start; i < token.End; i++ {
			rest[i] = ' '
		}
	}
	for _, word := range strings.FieldsFunc(string(rest), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' && r != '\''
	}) {
		word = strings.Trim(word, "-_'")
		if word != "" && len(q.Words) < maxQueryWords {
			q.Words = append(q.Words, word)
		}
	}
	return q
}

// Search answers a request. Codes come first, in tiers: in a title, in a heading, anywhere as a whole, by a part. The
// full text follows. Each object shows once, with its best section.
func (s *Service) Search(ctx context.Context, r Request) (Result, error) {
	limit := r.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	limit = min(limit, maxRequestLimit)
	if s.engine == nil {
		return s.titles(ctx, r, limit, NoticeNoService)
	}
	result, err := s.searchEngine(ctx, s.tenantOf(ctx), r, limit)
	if err != nil {
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		s.log.Warn("search service failed, searching titles", slog.String("engine", s.engine.Name()), slog.String("error", err.Error()))
		return s.titles(ctx, r, limit, NoticeServiceDown)
	}
	return result, nil
}

func (s *Service) titles(ctx context.Context, r Request, limit int, notice string) (Result, error) {
	result := Result{Engine: EngineTitles, Notice: notice, Hits: []Hit{}, Facets: Facets{}}
	if (strings.TrimSpace(r.Text) == "" && len(r.Filters.Tags) == 0) || s.fallback == nil {
		return result, nil
	}
	hits, err := s.fallback.Titles(ctx, strings.TrimSpace(r.Text), r.Account, r.Filters.Tags, limit)
	if err != nil {
		return Result{}, err
	}
	for _, hit := range hits {
		if r.Filters.Objects == nil || slices.Contains(r.Filters.Objects, hit.Object) {
			result.Hits = append(result.Hits, hit)
		}
	}
	return result, nil
}

type tier struct {
	conditions []CodeCondition
	// located tells where in the record a code of the query sits, for ranking within the tier.
	located bool
}

func (s *Service) tenantOf(ctx context.Context) string {
	if t, ok := tenant.From(ctx); ok {
		return t
	}
	return s.tenant
}

func (s *Service) searchEngine(ctx context.Context, tenantID string, r Request, limit int) (Result, error) {
	q := Parse(r.Text)
	readers := ReadersOf(r.Account, r.Groups, r.Direct)
	result := Result{Engine: s.engine.Name(), Hits: []Hit{}, Facets: Facets{}}
	seen := map[string]bool{}
	add := func(found Found, code bool) {
		if seen[found.Doc.Object] || len(result.Hits) >= limit {
			return
		}
		seen[found.Doc.Object] = true
		result.Hits = append(result.Hits, hitOf(found, code))
	}
	if len(q.Codes) > 0 {
		weak := slices.Concat(q.Codes, q.Parts)
		tiers := []tier{
			{conditions: []CodeCondition{{FieldTitleCodes, q.Codes}}},
			{conditions: []CodeCondition{{FieldHeadingCodes, q.Codes}}},
			{conditions: []CodeCondition{{FieldCodes, q.Codes}}},
			{conditions: []CodeCondition{{FieldCodeParts, weak}, {FieldCodes, q.Parts}}, located: true},
		}
		for _, t := range tiers {
			if len(t.conditions[len(t.conditions)-1].Codes) == 0 {
				t.conditions = t.conditions[:len(t.conditions)-1]
			}
			answer, err := s.engine.Search(ctx, tenantID, Query{Readers: readers, Agent: r.Agent, Filters: r.Filters, Codes: t.conditions,
				Limit: maxTierRecords, Newest: true})
			if err != nil {
				return Result{}, err
			}
			found := answer.Found
			rankTier(found, q, t.located)
			for _, f := range found {
				f.Snippet = Snippet(f.Doc.Body, weak, q.Words)
				if !seen[f.Doc.Object] {
					count(result.Facets, f.Doc)
				}
				add(f, true)
			}
		}
	}
	text := r.Text
	if len(q.Codes) > 0 {
		text = strings.Join(q.Words, " ")
	}
	if len(q.Codes) > 0 && text == "" {
		return result, nil
	}
	answer, err := s.engine.Search(ctx, tenantID, Query{Text: text, Readers: readers, Agent: r.Agent, Filters: r.Filters, Limit: limit + len(seen),
		OnePerObject: true, Facets: true, Newest: strings.TrimSpace(text) == "", Phrase: r.Exact})
	if err != nil {
		return Result{}, err
	}
	for field, values := range answer.Facets {
		for value, n := range values {
			if result.Facets[field] == nil {
				result.Facets[field] = map[string]int{}
			}
			result.Facets[field][value] += n
		}
	}
	words := q.Words
	if len(words) > queryWordsForSnippets {
		words = words[:queryWordsForSnippets]
	}
	for _, f := range answer.Found {
		if len(f.Snippet) == 0 {
			f.Snippet = Snippet(f.Doc.Body, nil, words)
		}
		add(f, false)
	}
	return result, nil
}

func count(facets Facets, doc Doc) {
	bump := func(field, value string) {
		if value == "" {
			return
		}
		if facets[field] == nil {
			facets[field] = map[string]int{}
		}
		facets[field][value]++
	}
	bump(FacetType, doc.Type)
	bump(FacetCollection, doc.Collection)
	bump(FacetAuthor, doc.Author)
	for _, tag := range doc.Tags {
		bump(FacetTag, tag)
	}
}

func hitOf(f Found, code bool) Hit {
	d := f.Doc
	return Hit{Object: d.Object, Anchor: d.Anchor, Type: d.Type, Title: d.Title, Heading: d.Heading, Path: d.Path,
		Collection: d.Collection, CollectionName: d.CollectionName, Author: d.Author, AuthorName: d.AuthorName,
		Tags: d.Tags, Changed: time.Unix(d.Changed, 0), Snippet: f.Snippet, Code: code}
}

// rankTier orders the records of one code tier: where the code sits (only for the tier of parts, the others hold it in
// one place already), then how many other words of the query are in title and heading, then in the text, then how often
// the code is named, then the newer, then the order of the sections.
func rankTier(found []Found, q ParsedQuery, located bool) {
	type key struct{ place, lead, head, body, mentions int }
	keys := make([]key, len(found))
	all := slices.Concat(q.Codes, q.Parts)
	for i := range found {
		d := found[i].Doc
		k := key{}
		if located {
			switch {
			case hasCode(d.Title, all):
				k.place = 2
			case hasCode(d.Heading, all):
				k.place = 1
			}
		}
		// A catalogue names the code first: "E-1042: Terminal antwortet nicht" is about E-1042, "KB5005565: Drucker meldet
		// 0x0000011b" is not about 0x0000011b.
		if leads(d.Title, all) || leads(d.Heading, all) {
			k.lead = 1
		}
		head, body := foldSameLength(d.Title+" "+d.Heading), foldSameLength(d.Body)
		for _, word := range q.Words {
			word = foldSameLength(word)
			if containsWord(head, word) {
				k.head++
			}
			if containsWord(body, word) {
				k.body++
			}
		}
		for _, token := range FindCodes(d.Body) {
			if slices.Contains(all, token.Code) || slices.ContainsFunc(token.Parts, func(p string) bool { return slices.Contains(all, p) }) {
				k.mentions++
			}
		}
		keys[i] = k
	}
	order := make([]int, len(found))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		ka, kb := keys[a], keys[b]
		da, db := found[a].Doc, found[b].Doc
		return cmp.Or(cmp.Compare(kb.place, ka.place), cmp.Compare(kb.lead, ka.lead), cmp.Compare(kb.head, ka.head), cmp.Compare(kb.body, ka.body),
			cmp.Compare(kb.mentions, ka.mentions), cmp.Compare(db.Changed, da.Changed), cmp.Compare(da.Object, db.Object),
			cmp.Compare(da.Rank, db.Rank))
	})
	sorted := make([]Found, len(found))
	for i, j := range order {
		sorted[i] = found[j]
	}
	copy(found, sorted)
}

func hasCode(text string, codes []string) bool {
	for _, token := range FindCodes(text) {
		if slices.Contains(codes, token.Code) || slices.ContainsFunc(token.Parts, func(p string) bool { return slices.Contains(codes, p) }) {
			return true
		}
	}
	return false
}

// leads tells whether a text starts with one of the codes.
func leads(text string, codes []string) bool {
	tokens := FindCodes(text)
	if len(tokens) == 0 || strings.TrimSpace(text[:tokens[0].Start]) != "" {
		return false
	}
	first := tokens[0]
	return slices.Contains(codes, first.Code) || slices.ContainsFunc(first.Parts, func(p string) bool { return slices.Contains(codes, p) })
}

// containsWord tells whether a word starts somewhere in a folded text.
func containsWord(text, word string) bool {
	for from := 0; word != ""; {
		at := strings.Index(text[from:], word)
		if at < 0 {
			return false
		}
		if wordStart(text, from+at) {
			return true
		}
		from += at + len(word)
	}
	return false
}
