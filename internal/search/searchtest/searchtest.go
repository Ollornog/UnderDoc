// SPDX-License-Identifier: MIT

// Package searchtest holds what every search engine has to pass (ADR-24): the same functional tests, and the German test
// corpus with its questions, measured as hits at 1 and 5 and mean reciprocal rank. Codes are a hard gate: each one must be
// the first hit.
//
// The engines run in containers that scripts/search-engines.sh starts; their addresses come from the environment. A
// test without its engine fails: a skipped search test is not a green one.
package searchtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/search"
)

// Environment variables with the addresses of the engines under test.
const (
	EnvMeilisearch = "UNDERDOC_TEST_MEILISEARCH_URL"
	EnvOpenSearch  = "UNDERDOC_TEST_OPENSEARCH_URL"
	// EnvReport names a file the measurement of the corpus is appended to, as a row of Markdown.
	EnvReport = "UNDERDOC_SEARCH_REPORT"
)

// URL returns the address of an engine under test, or fails the test with how to start it.
func URL(t testing.TB, env string) string {
	t.Helper()
	url := os.Getenv(env)
	if url == "" {
		t.Fatalf("%s is not set: the search engines run in containers that scripts/check.sh starts "+
			"(scripts/search-engines.sh start prints the variables). A skipped search test is not a green one.", env)
	}
	return url
}

// Prefix returns a name for the indexes of one test run, so parallel runs against the same engine never meet.
func Prefix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("t%d%s", os.Getpid(), hex.EncodeToString(b))
}

// Quiet is a logger that writes nothing.
func Quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// Context gives a test a deadline.
func Context(t testing.TB) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

// Load puts docs of one key into the index of a tenant through the same steps the indexer takes.
func Load(t testing.TB, ctx context.Context, engine search.Engine, tenant string, key search.Key, docs []search.Doc) {
	t.Helper()
	for i := range docs {
		docs[i].Source, docs[i].Key = key.Source, key.Key
		docs[i].ID = search.DocID(key, i)
		docs[i].Prepare()
	}
	if err := engine.Replace(ctx, tenant, []search.Key{key}, docs); err != nil {
		t.Fatal(err)
	}
}
