// SPDX-License-Identifier: MIT

// Package opensearch is the search engine OpenSearch, the choice next to Meilisearch of ADR-24. It speaks the REST API of
// OpenSearch directly, without a client library, and is held to the same tests as Meilisearch (search/searchtest).
package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Ollornog/UnderDoc/internal/search"
)

// version changes whenever the mapping or the analysis changes: OpenSearch cannot change them in place, so the index is
// made anew and filled again from the queue.
const version = "opensearch-2"

// Engine is one OpenSearch.
type Engine struct {
	url            string
	user, password string
	prefix         string
	client         *http.Client
}

// New returns the engine at url. user and password are for basic authentication, both empty for an OpenSearch without
// the security plugin (tests only). prefix starts the name of every index.
func New(url, user, password, prefix string) *Engine {
	return &Engine{url: strings.TrimRight(url, "/"), user: user, password: password, prefix: prefix,
		client: &http.Client{Timeout: 30 * time.Second}}
}

// Name implements search.Engine.
func (e *Engine) Name() string { return "opensearch" }

// Version implements search.Versioned.
func (e *Engine) Version() string { return version }

func (e *Engine) index(tenant string) string { return search.IndexName(e.prefix, tenant) }

type apiError struct {
	Status int
	Body   string
}

func (a *apiError) Error() string { return fmt.Sprintf("opensearch: %d %s", a.Status, a.Body) }

func (e *Engine) call(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	contentType := "application/json"
	switch b := body.(type) {
	case nil:
	case []byte:
		reader, contentType = bytes.NewReader(b), "application/x-ndjson"
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, e.url+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", contentType)
	if e.user != "" {
		request.SetBasicAuth(e.user, e.password)
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
		return &apiError{Status: response.StatusCode, Body: strings.TrimSpace(string(raw[:min(len(raw), 500)]))}
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// mapping of every index, chosen by measuring the German corpus (docs/search.md). German text goes through a light stemmer and the German normalisation; title, heading and text
// have a subfield of word parts (n-grams of four and five letters), so a part of a compound finds the whole
// ("Warteschlange" finds "Druckerwarteschlange"), and one of the words as written, unstemmed, where typing errors are
// looked for: a stemmer cuts "Wagen" to a stem too short for one. Codes, readers and filters are keywords, matched exactly.
func mapping() map[string]any {
	keyword := map[string]any{"type": "keyword"}
	stored := map[string]any{"type": "keyword", "index": false}
	text := func() map[string]any {
		return map[string]any{"type": "text", "analyzer": "de", "fields": map[string]any{
			"parts": map[string]any{"type": "text", "analyzer": "de_parts"},
			"plain": map[string]any{"type": "text", "analyzer": "de_plain"}}}
	}
	return map[string]any{
		"settings": map[string]any{
			"number_of_shards": 1, "number_of_replicas": 0, "max_ngram_diff": 1,
			"analysis": map[string]any{
				"filter": map[string]any{
					"de_stop":  map[string]any{"type": "stop", "stopwords": search.StopWords},
					"de_stem":  map[string]any{"type": "stemmer", "language": "light_german"},
					"de_parts": map[string]any{"type": "ngram", "min_gram": 4, "max_gram": 5},
				},
				"analyzer": map[string]any{
					"de":       map[string]any{"tokenizer": "standard", "filter": []string{"lowercase", "de_stop", "german_normalization", "de_stem"}},
					"de_parts": map[string]any{"tokenizer": "standard", "filter": []string{"lowercase", "german_normalization", "de_parts"}},
					"de_plain": map[string]any{"tokenizer": "standard", "filter": []string{"lowercase", "german_normalization"}},
				},
			},
		},
		"mappings": map[string]any{
			"_meta":   map[string]any{"version": version},
			"dynamic": "strict",
			"properties": map[string]any{
				"id": keyword, "source": keyword, "key": keyword, "object": keyword, "anchor": stored, "rank": map[string]any{"type": "integer"},
				"type": keyword, "title": text(), "heading": text(), "body": text(),
				"tags":  map[string]any{"type": "keyword", "fields": map[string]any{"text": map[string]any{"type": "text", "analyzer": "de"}}},
				"codes": keyword, "code_parts": keyword, "title_codes": keyword, "heading_codes": keyword,
				"path": stored, "collection": keyword, "collection_name": stored, "author": keyword, "author_name": stored,
				"changed": map[string]any{"type": "long"}, "readers": keyword, "origin": keyword,
				"l0a": keyword, "l0d": keyword, "l1a": keyword, "l1d": keyword, "l2a": keyword, "l2d": keyword,
				"l3a": keyword, "l3d": keyword, "l4a": keyword, "l4d": keyword, "l5a": keyword, "l5d": keyword,
				"no_agents": map[string]any{"type": "boolean"},
			},
		},
	}
}

// Healthy reports whether OpenSearch answers.
func (e *Engine) Healthy(ctx context.Context) bool {
	return e.call(ctx, http.MethodGet, "/_cluster/health", nil, nil) == nil
}

// Prepare implements search.Engine. An index of another version is dropped and made anew.
func (e *Engine) Prepare(ctx context.Context, tenant string) (bool, error) {
	name := e.index(tenant)
	var current map[string]struct {
		Mappings struct {
			Meta struct {
				Version string `json:"version"`
			} `json:"_meta"`
		} `json:"mappings"`
	}
	err := e.call(ctx, http.MethodGet, "/"+name+"/_mapping", nil, &current)
	var api *apiError
	switch {
	case err == nil && current[name].Mappings.Meta.Version == version:
		return false, nil
	case err == nil:
		if err := e.Drop(ctx, tenant); err != nil {
			return false, err
		}
	case asAPI(err, &api) && api.Status == http.StatusNotFound:
	default:
		return false, err
	}
	return true, e.call(ctx, http.MethodPut, "/"+name, mapping(), nil)
}

func asAPI(err error, target **apiError) bool {
	a, ok := err.(*apiError)
	if ok {
		*target = a
	}
	return ok
}

// Replace implements search.Engine.
func (e *Engine) Replace(ctx context.Context, tenant string, keys []search.Key, docs []search.Doc) error {
	name := e.index(tenant)
	if len(keys) > 0 {
		var either []any
		bySource := map[string][]string{}
		for _, k := range keys {
			bySource[k.Source] = append(bySource[k.Source], k.Key)
		}
		for source, list := range bySource {
			either = append(either, map[string]any{"bool": map[string]any{"filter": []any{
				map[string]any{"term": map[string]any{"source": source}}, map[string]any{"terms": map[string]any{"key": list}}}}})
		}
		query := map[string]any{"query": map[string]any{"bool": map[string]any{"should": either, "minimum_should_match": 1}}}
		if err := e.call(ctx, http.MethodPost, "/"+name+"/_delete_by_query?refresh=true&conflicts=proceed", query, nil); err != nil {
			return err
		}
	}
	if len(docs) == 0 {
		return nil
	}
	var bulk bytes.Buffer
	encoder := json.NewEncoder(&bulk)
	for _, d := range docs {
		if err := encoder.Encode(map[string]any{"index": map[string]any{"_id": d.ID}}); err != nil {
			return err
		}
		if err := encoder.Encode(d); err != nil {
			return err
		}
	}
	var answer struct {
		Errors bool `json:"errors"`
		Items  []map[string]struct {
			Error json.RawMessage `json:"error"`
		} `json:"items"`
	}
	if err := e.call(ctx, http.MethodPost, "/"+name+"/_bulk?refresh=true", bulk.Bytes(), &answer); err != nil {
		return err
	}
	if answer.Errors {
		for _, item := range answer.Items {
			for _, result := range item {
				if len(result.Error) > 0 {
					return fmt.Errorf("opensearch: %s", result.Error)
				}
			}
		}
	}
	return nil
}

// Drop implements search.Engine.
func (e *Engine) Drop(ctx context.Context, tenant string) error {
	err := e.call(ctx, http.MethodDelete, "/"+e.index(tenant), nil, nil)
	var api *apiError
	if asAPI(err, &api) && api.Status == http.StatusNotFound {
		return nil
	}
	return err
}

// Search implements search.Engine. The weights follow the ranking of ADR-24: title before heading before tags before
// text, all words before some, exact words before typing errors, and newer only to break a tie.
func (e *Engine) Search(ctx context.Context, tenant string, q search.Query) (search.Answer, error) {
	if len(q.Readers) == 0 {
		return search.Answer{}, search.ErrNoReaders
	}
	body := map[string]any{"size": min(max(q.Limit, 1), search.MaxLimit), "track_total_hits": true}
	filter := filters(q)
	text := strings.TrimSpace(q.Text)
	switch {
	case text == "":
		body["query"] = map[string]any{"bool": map[string]any{"filter": filter}}
	case q.Phrase:
		body["query"] = map[string]any{"bool": map[string]any{"filter": filter, "must": map[string]any{"multi_match": map[string]any{
			"query": text, "type": "phrase", "fields": []string{"title^8", "heading^4", "tags.text^3", "body"}}}}}
	default:
		fields := []string{"title^8", "heading^4", "tags.text^3", "body"}
		should := []any{
			// Every word, as written (after stemming): the strongest.
			map[string]any{"multi_match": map[string]any{"query": text, "type": "best_fields", "fields": fields, "operator": "and", "boost": 4}},
			// Some of the words.
			map[string]any{"multi_match": map[string]any{"query": text, "type": "best_fields", "fields": fields, "boost": 2}},
			// With a typing error: one in four letters and more, two in nine and more, as Meilisearch does.
			// On the words as written, since a stem can be too short for one ("Wafen" for "Wagen").
			map[string]any{"multi_match": map[string]any{"query": text, "type": "best_fields", "fields": []string{"title.plain^8", "heading.plain^4", "body.plain"},
				"fuzziness": "AUTO:4,9", "prefix_length": 1, "boost": 0.5}},
			// The last word as the start of a word, while typing.
			map[string]any{"multi_match": map[string]any{"query": text, "type": "bool_prefix", "fields": []string{"title^8", "heading^4", "body"}}},
			// Parts of words, for compounds.
			map[string]any{"multi_match": map[string]any{"query": text, "type": "best_fields",
				"fields": []string{"title.parts^8", "heading.parts^4", "body.parts"}, "minimum_should_match": "60%", "boost": 0.5}},
		}
		body["query"] = map[string]any{"bool": map[string]any{"filter": filter, "should": should, "minimum_should_match": 1}}
		body["highlight"] = map[string]any{"pre_tags": []string{search.MarkStart}, "post_tags": []string{search.MarkEnd},
			"fields": map[string]any{"body": map[string]any{"fragment_size": 220, "number_of_fragments": 1}}}
	}
	if q.Newest || text == "" {
		body["sort"] = []any{map[string]any{"changed": "desc"}, map[string]any{"rank": "asc"}}
	} else {
		body["sort"] = []any{"_score", map[string]any{"changed": "desc"}, map[string]any{"rank": "asc"}}
	}
	if q.OnePerObject {
		body["collapse"] = map[string]any{"field": "object"}
	}
	if q.Facets {
		aggs := map[string]any{}
		for _, field := range []string{search.FacetType, search.FacetCollection, search.FacetAuthor, search.FacetTag} {
			aggs[field] = map[string]any{"terms": map[string]any{"field": field, "size": 100}}
		}
		body["aggs"] = aggs
	}
	var response struct {
		Hits struct {
			Total struct {
				Value int `json:"value"`
			} `json:"total"`
			Hits []struct {
				Source    search.Doc          `json:"_source"`
				Highlight map[string][]string `json:"highlight"`
			} `json:"hits"`
		} `json:"hits"`
		Aggregations map[string]struct {
			Buckets []struct {
				Key   string `json:"key"`
				Count int    `json:"doc_count"`
			} `json:"buckets"`
		} `json:"aggregations"`
	}
	if err := e.call(ctx, http.MethodPost, "/"+e.index(tenant)+"/_search", body, &response); err != nil {
		return search.Answer{}, err
	}
	answer := search.Answer{Total: response.Hits.Total.Value, Facets: search.Facets{}}
	for field, agg := range response.Aggregations {
		values := map[string]int{}
		for _, b := range agg.Buckets {
			values[b.Key] = b.Count
		}
		answer.Facets[field] = values
	}
	for _, hit := range response.Hits.Hits {
		found := search.Found{Doc: hit.Source}
		if marked := hit.Highlight["body"]; len(marked) > 0 && strings.Contains(marked[0], search.MarkStart) {
			found.Snippet = search.Fragments(marked[0])
		}
		answer.Found = append(answer.Found, found)
	}
	return answer, nil
}

// access is the filter of who reads (ADR-36): one of the tokens that always find the record, or, from the nearest level
// up, the first level that names a token of the reader allows one and denies none.
func access(readers []string) any {
	terms := func(field string) any { return map[string]any{"terms": map[string]any{field: readers}} }
	var expr any
	for i := search.MaxLevels - 1; i >= 0; i-- {
		a, d := terms(search.LevelField(i, false)), terms(search.LevelField(i, true))
		here := map[string]any{"bool": map[string]any{"filter": []any{a}, "must_not": []any{d}}}
		if expr == nil {
			expr = here
			continue
		}
		further := map[string]any{"bool": map[string]any{"filter": []any{expr}, "must_not": []any{a, d}}}
		expr = map[string]any{"bool": map[string]any{"should": []any{here, further}, "minimum_should_match": 1}}
	}
	return map[string]any{"bool": map[string]any{"should": []any{terms("readers"), expr}, "minimum_should_match": 1}}
}

// filters turns the readers, the filters of the results page and the codes into filters of OpenSearch, all of which must
// hold.
func filters(q search.Query) []any {
	out := []any{access(q.Readers)}
	if q.Agent {
		out = append(out, map[string]any{"bool": map[string]any{"must_not": []any{map[string]any{"term": map[string]any{"no_agents": true}}}}})
	}
	for _, f := range []struct {
		field  string
		values []string
	}{{"type", q.Filters.Types}, {"collection", q.Filters.Collections}, {"author", q.Filters.Authors}, {"object", q.Filters.Objects}} {
		if len(f.values) > 0 {
			out = append(out, map[string]any{"terms": map[string]any{f.field: f.values}})
		}
	}
	for _, tag := range q.Filters.Tags {
		out = append(out, map[string]any{"term": map[string]any{"tags": tag}})
	}
	changed := map[string]any{}
	if !q.Filters.From.IsZero() {
		changed["gte"] = q.Filters.From.Unix()
	}
	if !q.Filters.To.IsZero() {
		changed["lte"] = q.Filters.To.Unix()
	}
	if len(changed) > 0 {
		out = append(out, map[string]any{"range": map[string]any{"changed": changed}})
	}
	if len(q.Codes) > 0 {
		var either []any
		for _, c := range q.Codes {
			if len(c.Codes) > 0 {
				either = append(either, map[string]any{"terms": map[string]any{c.Field: c.Codes}})
			}
		}
		out = append(out, map[string]any{"bool": map[string]any{"should": either, "minimum_should_match": 1}})
	}
	return out
}
