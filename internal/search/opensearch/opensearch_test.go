// SPDX-License-Identifier: MIT

package opensearch_test

import (
	"testing"

	"github.com/Ollornog/UnderDoc/internal/search"
	"github.com/Ollornog/UnderDoc/internal/search/opensearch"
	"github.com/Ollornog/UnderDoc/internal/search/searchtest"
)

func newEngine(t *testing.T) searchtest.NewEngine {
	url := searchtest.URL(t, searchtest.EnvOpenSearch)
	return func(prefix string) search.Engine { return opensearch.New(url, "", "", prefix) }
}

// TestEngine holds OpenSearch to the same tests as Meilisearch.
func TestEngine(t *testing.T) { searchtest.Run(t, newEngine(t)) }

// TestCorpus measures OpenSearch on the German corpus. The floor is what was measured when the engine came in, rounded
// down; docs/search.md has the full table.
func TestCorpus(t *testing.T) {
	searchtest.RunCorpus(t, newEngine(t), map[string]float64{
		"title": 0.95, "heading": 0.9, "compound": 0.8, "wordform": 0.9, "typo": 0.85, "typo-short": 0.95, "phrase": 0.6, "all": 0.94})
}
