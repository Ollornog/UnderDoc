// SPDX-License-Identifier: MIT

package meili_test

import (
	"testing"

	"github.com/Ollornog/UnderDoc/internal/search"
	"github.com/Ollornog/UnderDoc/internal/search/meili"
	"github.com/Ollornog/UnderDoc/internal/search/searchtest"
)

func newEngine(t *testing.T) searchtest.NewEngine {
	url := searchtest.URL(t, searchtest.EnvMeilisearch)
	return func(prefix string) search.Engine { return meili.New(url, "", prefix) }
}

func TestEngine(t *testing.T) { searchtest.Run(t, newEngine(t)) }

// TestCorpus measures Meilisearch on the German corpus. The floor is what was measured when the engine came in, rounded
// down; docs/search.md has the full table.
func TestCorpus(t *testing.T) {
	searchtest.RunCorpus(t, newEngine(t), map[string]float64{
		"title": 0.95, "heading": 0.95, "compound": 0.9, "wordform": 0.75, "typo": 0.7, "phrase": 0.55, "all": 0.93})
}
