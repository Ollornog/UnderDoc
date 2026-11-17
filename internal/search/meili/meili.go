// SPDX-License-Identifier: MIT

// Package meili is the search engine Meilisearch, the default of ADR-24. It speaks the HTTP API of Meilisearch directly,
// without a client library: the few calls it needs do not justify one more dependency.
package meili

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Ollornog/UnderDoc/internal/search"
)

// version changes whenever the shape of the records or the settings change in a way that needs every record again.
const version = "meilisearch-1"

// settings of every index, chosen by measuring the German corpus (docs/search.md). Title before headings before text
// (attributeRank), exact words before typing errors (typo), newer only to break a tie (changed:desc last). Title, headings
// and text are split into the parts of German compounds through the locale; title and heading stand a second time as
// written, where a typing error in a whole word is still found. Numbers never take a typing error, and codes are not
// searched as text at all but through their fields (search.Service).
var settings = map[string]any{
	"searchableAttributes": []string{"title", "title_words", "heading", "heading_words", "tags", "body"},
	"filterableAttributes": []string{"readers", "type", "collection", "author", "tags", "changed", "object", "source", "key",
		search.FieldCodes, search.FieldCodeParts, search.FieldTitleCodes, search.FieldHeadingCodes},
	"sortableAttributes": []string{"changed"},
	"rankingRules":       []string{"words", "typo", "proximity", "attributeRank", "sort", "wordPosition", "exactness", "changed:desc"},
	"stopWords":          search.StopWords,
	"typoTolerance": map[string]any{
		"enabled":             true,
		"disableOnNumbers":    true,
		"minWordSizeForTypos": map[string]int{"oneTypo": 5, "twoTypos": 9},
	},
	"localizedAttributes": []map[string]any{{"attributePatterns": []string{"title", "heading", "tags", "body"}, "locales": []string{"deu"}}},
	"pagination":          map[string]int{"maxTotalHits": search.MaxLimit},
	"faceting":            map[string]int{"maxValuesPerFacet": 100},
}

// Engine is one Meilisearch.
type Engine struct {
	url    string
	key    string
	prefix string
	client *http.Client
}

// New returns the engine at url. key is an API key with the rights to manage indexes and documents and to search, or
// empty for a Meilisearch without a master key (tests only). prefix starts the name of every index, so several
// installations can share one Meilisearch.
func New(url, key, prefix string) *Engine {
	return &Engine{url: strings.TrimRight(url, "/"), key: key, prefix: prefix, client: &http.Client{Timeout: 30 * time.Second}}
}

// Name implements search.Engine.
func (e *Engine) Name() string { return "meilisearch" }

// Version implements search.Versioned.
func (e *Engine) Version() string { return version }

func (e *Engine) index(tenant string) string { return search.IndexName(e.prefix, tenant) }

type apiError struct {
	Status  int
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *apiError) Error() string {
	return fmt.Sprintf("meilisearch: %d %s: %s", e.Status, e.Code, e.Message)
}

func (e *Engine) call(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, e.url+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if e.key != "" {
		request.Header.Set("Authorization", "Bearer "+e.key)
	}
	response, err := e.client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil {
		return err
	}
	if response.StatusCode >= 300 {
		failure := &apiError{Status: response.StatusCode}
		_ = json.Unmarshal(raw, failure)
		return failure
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

type task struct {
	UID    int64  `json:"taskUid"`
	Status string `json:"status"`
	Error  *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
}

// run calls the API for a change and waits until Meilisearch has done it.
func (e *Engine) run(ctx context.Context, method, path string, body any) error {
	var t task
	if err := e.call(ctx, method, path, body, &t); err != nil {
		return err
	}
	return e.wait(ctx, t.UID)
}

func (e *Engine) wait(ctx context.Context, uid int64) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	pause := 5 * time.Millisecond
	for {
		var t struct {
			Status string `json:"status"`
			Error  *struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		if err := e.call(ctx, http.MethodGet, fmt.Sprintf("/tasks/%d", uid), nil, &t); err != nil {
			return err
		}
		switch t.Status {
		case "succeeded":
			return nil
		case "failed", "canceled":
			if t.Error != nil {
				return fmt.Errorf("meilisearch task %d %s: %s (%s)", uid, t.Status, t.Error.Message, t.Error.Code)
			}
			return fmt.Errorf("meilisearch task %d %s", uid, t.Status)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("meilisearch task %d: %w", uid, ctx.Err())
		case <-time.After(pause):
		}
		pause = min(pause*2, 200*time.Millisecond)
	}
}

// Healthy reports whether Meilisearch answers.
func (e *Engine) Healthy(ctx context.Context) bool {
	return e.call(ctx, http.MethodGet, "/health", nil, nil) == nil
}

// Prepare implements search.Engine.
func (e *Engine) Prepare(ctx context.Context, tenant string) (bool, error) {
	uid := e.index(tenant)
	created := false
	err := e.call(ctx, http.MethodGet, "/indexes/"+uid, nil, nil)
	var api *apiError
	switch {
	case errors.As(err, &api) && api.Code == "index_not_found":
		if err := e.run(ctx, http.MethodPost, "/indexes", map[string]string{"uid": uid, "primaryKey": "id"}); err != nil {
			return false, err
		}
		created = true
	case err != nil:
		return false, err
	}
	return created, e.run(ctx, http.MethodPatch, "/indexes/"+uid+"/settings", settings)
}

// Replace implements search.Engine. Both steps are tasks of the same index, which Meilisearch does in their order.
func (e *Engine) Replace(ctx context.Context, tenant string, keys []search.Key, docs []search.Doc) error {
	uid := e.index(tenant)
	if len(keys) > 0 {
		var t task
		if err := e.call(ctx, http.MethodPost, "/indexes/"+uid+"/documents/delete", map[string]string{"filter": keysFilter(keys)}, &t); err != nil {
			return err
		}
		if len(docs) == 0 {
			return e.wait(ctx, t.UID)
		}
		if err := e.wait(ctx, t.UID); err != nil {
			return err
		}
	}
	if len(docs) == 0 {
		return nil
	}
	records := make([]record, len(docs))
	for i, d := range docs {
		records[i] = record{Doc: d, TitleWords: d.Title, HeadingWords: d.Heading}
	}
	return e.run(ctx, http.MethodPost, "/indexes/"+uid+"/documents?primaryKey=id", records)
}

// record is a doc as Meilisearch keeps it. Title and heading stand twice: once split into the parts of German compounds,
// and once as written, where a typing error in a whole word is still found (measured in docs/search.md).
type record struct {
	search.Doc
	TitleWords   string `json:"title_words,omitempty"`
	HeadingWords string `json:"heading_words,omitempty"`
}

func keysFilter(keys []search.Key) string {
	bySource := map[string][]string{}
	var order []string
	for _, k := range keys {
		if _, seen := bySource[k.Source]; !seen {
			order = append(order, k.Source)
		}
		bySource[k.Source] = append(bySource[k.Source], k.Key)
	}
	var parts []string
	for _, source := range order {
		parts = append(parts, fmt.Sprintf("(source = %s AND key IN %s)", quote(source), list(bySource[source])))
	}
	return strings.Join(parts, " OR ")
}

// quote writes a string for a filter of Meilisearch: in double quotes, with quotes and backslashes escaped.
func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func list(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = quote(v)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// Drop implements search.Engine.
func (e *Engine) Drop(ctx context.Context, tenant string) error {
	err := e.run(ctx, http.MethodDelete, "/indexes/"+e.index(tenant), nil)
	var api *apiError
	if errors.As(err, &api) && api.Code == "index_not_found" {
		return nil
	}
	return err
}

type searchRequest struct {
	Q                     string   `json:"q"`
	Filter                []string `json:"filter"`
	Limit                 int      `json:"limit"`
	AttributesToCrop      []string `json:"attributesToCrop,omitempty"`
	CropLength            int      `json:"cropLength,omitempty"`
	CropMarker            string   `json:"cropMarker,omitempty"`
	HighlightPreTag       string   `json:"highlightPreTag,omitempty"`
	HighlightPostTag      string   `json:"highlightPostTag,omitempty"`
	AttributesToHighlight []string `json:"attributesToHighlight,omitempty"`
	Distinct              string   `json:"distinct,omitempty"`
	Facets                []string `json:"facets,omitempty"`
	Sort                  []string `json:"sort,omitempty"`
	Locales               []string `json:"locales,omitempty"`
}

type searchResponse struct {
	Hits []struct {
		search.Doc
		Formatted *struct {
			Body string `json:"body"`
		} `json:"_formatted"`
	} `json:"hits"`
	EstimatedTotalHits int                       `json:"estimatedTotalHits"`
	FacetDistribution  map[string]map[string]int `json:"facetDistribution"`
}

// Search implements search.Engine.
func (e *Engine) Search(ctx context.Context, tenant string, q search.Query) (search.Answer, error) {
	if len(q.Readers) == 0 {
		return search.Answer{}, search.ErrNoReaders
	}
	request := searchRequest{Q: q.Text, Filter: filters(q), Limit: min(max(q.Limit, 1), search.MaxLimit), Locales: []string{"deu"}}
	if q.Phrase && strings.TrimSpace(q.Text) != "" {
		// Meilisearch finds a text in double quotes as a phrase, exactly and without typing errors.
		request.Q = `"` + strings.ReplaceAll(q.Text, `"`, " ") + `"`
	}
	if strings.TrimSpace(q.Text) != "" {
		request.AttributesToCrop = []string{"body"}
		request.CropLength = 36
		request.CropMarker = "…"
		request.AttributesToHighlight = []string{"body"}
		request.HighlightPreTag, request.HighlightPostTag = search.MarkStart, search.MarkEnd
	}
	if q.OnePerObject {
		request.Distinct = "object"
	}
	if q.Facets {
		request.Facets = []string{search.FacetType, search.FacetCollection, search.FacetAuthor, search.FacetTag}
	}
	if q.Newest {
		request.Sort = []string{"changed:desc"}
	}
	var response searchResponse
	if err := e.call(ctx, http.MethodPost, "/indexes/"+e.index(tenant)+"/search", request, &response); err != nil {
		return search.Answer{}, err
	}
	answer := search.Answer{Total: response.EstimatedTotalHits, Facets: search.Facets{}}
	for field, values := range response.FacetDistribution {
		answer.Facets[field] = values
	}
	for _, hit := range response.Hits {
		found := search.Found{Doc: hit.Doc}
		if hit.Formatted != nil && strings.Contains(hit.Formatted.Body, search.MarkStart) {
			found.Snippet = search.Fragments(hit.Formatted.Body)
		}
		answer.Found = append(answer.Found, found)
	}
	return answer, nil
}

// filters writes the readers, the filters of the results page and the codes as filters of Meilisearch, all of which must
// hold.
func filters(q search.Query) []string {
	out := []string{"readers IN " + list(q.Readers)}
	for _, f := range []struct {
		field  string
		values []string
	}{{"type", q.Filters.Types}, {"collection", q.Filters.Collections}, {"author", q.Filters.Authors}} {
		if len(f.values) > 0 {
			out = append(out, f.field+" IN "+list(f.values))
		}
	}
	// Tags: every chosen tag must be there.
	for _, tag := range q.Filters.Tags {
		out = append(out, "tags = "+quote(tag))
	}
	if !q.Filters.From.IsZero() {
		out = append(out, fmt.Sprintf("changed >= %d", q.Filters.From.Unix()))
	}
	if !q.Filters.To.IsZero() {
		out = append(out, fmt.Sprintf("changed <= %d", q.Filters.To.Unix()))
	}
	if len(q.Codes) > 0 {
		var either []string
		for _, c := range q.Codes {
			if len(c.Codes) > 0 {
				either = append(either, c.Field+" IN "+list(c.Codes))
			}
		}
		out = append(out, "("+strings.Join(either, " OR ")+")")
	}
	return out
}
