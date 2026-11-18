// SPDX-License-Identifier: MIT

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSearchConfig(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(keyFile, []byte("  the-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name              string
		env               map[string]string
		engine, key, user string
		fails             bool
	}{
		{"nothing set: titles only", nil, "", "", "", false},
		{"a URL means Meilisearch", map[string]string{"UNDERDOC_SEARCH_URL": "http://127.0.0.1:7700"}, "meilisearch", "", "", false},
		{"OpenSearch with user and password from a file", map[string]string{"UNDERDOC_SEARCH_ENGINE": "OpenSearch",
			"UNDERDOC_SEARCH_URL": "https://search:9200", "UNDERDOC_SEARCH_USER": "underdoc", "UNDERDOC_SEARCH_KEY_FILE": keyFile},
			"opensearch", "the-key", "underdoc", false},
		{"none switches it off", map[string]string{"UNDERDOC_SEARCH_ENGINE": "none", "UNDERDOC_SEARCH_URL": "http://x"}, "", "", "", false},
		{"an engine without a URL", map[string]string{"UNDERDOC_SEARCH_ENGINE": "opensearch"}, "", "", "", true},
		{"an unknown engine", map[string]string{"UNDERDOC_SEARCH_ENGINE": "solr", "UNDERDOC_SEARCH_URL": "http://x"}, "", "", "", true},
		{"the key from the environment", map[string]string{"UNDERDOC_SEARCH_URL": "http://x", "UNDERDOC_SEARCH_KEY": " env-key\n"},
			"meilisearch", "env-key", "", false},
		{"a key from both places", map[string]string{"UNDERDOC_SEARCH_URL": "http://x", "UNDERDOC_SEARCH_KEY": "env-key",
			"UNDERDOC_SEARCH_KEY_FILE": keyFile}, "", "", "", true},
		{"a key file that is not there", map[string]string{"UNDERDOC_SEARCH_URL": "http://x", "UNDERDOC_SEARCH_KEY_FILE": keyFile + ".gone"}, "", "", "", true},
		{"an index name engines refuse", map[string]string{"UNDERDOC_SEARCH_URL": "http://x", "UNDERDOC_SEARCH_INDEX": "Under Doc"}, "", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, name := range []string{"UNDERDOC_SEARCH_ENGINE", "UNDERDOC_SEARCH_URL", "UNDERDOC_SEARCH_USER", "UNDERDOC_SEARCH_KEY_FILE", "UNDERDOC_SEARCH_KEY", "UNDERDOC_SEARCH_INDEX"} {
				t.Setenv(name, c.env[name])
			}
			got, err := readSearchConfig()
			if (err != nil) != c.fails {
				t.Fatalf("error %v, want failure %v", err, c.fails)
			}
			if c.fails {
				return
			}
			if got.engine != c.engine || got.key != c.key || got.user != c.user || got.prefix != "underdoc" {
				t.Errorf("%+v", got)
			}
		})
	}
}
