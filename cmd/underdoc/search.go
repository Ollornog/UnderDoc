// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/library"
	"github.com/Ollornog/UnderDoc/internal/search"
	"github.com/Ollornog/UnderDoc/internal/search/meili"
	"github.com/Ollornog/UnderDoc/internal/search/opensearch"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/web"
)

// Search settings (docs/search.md):
//
//	UNDERDOC_SEARCH_ENGINE    meilisearch, opensearch or none (meilisearch when a URL is set, none without)
//	UNDERDOC_SEARCH_URL       address of the search service, such as http://127.0.0.1:7700
//	UNDERDOC_SEARCH_KEY_FILE  a file holding the API key of Meilisearch or the password of OpenSearch
//	UNDERDOC_SEARCH_KEY       the same, from the environment, for a container that gets it from a .env; not both
//	UNDERDOC_SEARCH_USER      the user of OpenSearch for basic authentication (none: no authentication)
//	UNDERDOC_SEARCH_INDEX     what every index name starts with ("underdoc"), so installations can share a service
const defaultIndexPrefix = "underdoc"

// searchConfig is the search service as the environment sets it.
type searchConfig struct {
	engine, url, user, key, prefix string
}

func readSearchConfig() (searchConfig, error) {
	c := searchConfig{engine: strings.ToLower(os.Getenv("UNDERDOC_SEARCH_ENGINE")), url: os.Getenv("UNDERDOC_SEARCH_URL"),
		user: os.Getenv("UNDERDOC_SEARCH_USER"), prefix: env("UNDERDOC_SEARCH_INDEX", defaultIndexPrefix)}
	if c.engine == "" && c.url != "" {
		c.engine = "meilisearch"
	}
	switch c.engine {
	case "", "none":
		c.engine = ""
		return c, nil
	case "meilisearch", "opensearch":
	default:
		return c, fmt.Errorf("UNDERDOC_SEARCH_ENGINE: %q is not known, use meilisearch, opensearch or none", c.engine)
	}
	if c.url == "" {
		return c, fmt.Errorf("UNDERDOC_SEARCH_URL: the address of %s is missing", c.engine)
	}
	if !validPrefix(c.prefix) {
		return c, fmt.Errorf("UNDERDOC_SEARCH_INDEX: lower case letters, digits and dashes, 1 to 40")
	}
	path, inline := os.Getenv("UNDERDOC_SEARCH_KEY_FILE"), strings.TrimSpace(os.Getenv("UNDERDOC_SEARCH_KEY"))
	switch {
	case path != "" && inline != "":
		// Two places for one key: which one counts would be a guess.
		return c, fmt.Errorf("UNDERDOC_SEARCH_KEY and UNDERDOC_SEARCH_KEY_FILE are both set; set one")
	case path != "":
		raw, err := os.ReadFile(path)
		if err != nil {
			return c, fmt.Errorf("UNDERDOC_SEARCH_KEY_FILE: %w", err)
		}
		c.key = strings.TrimSpace(string(raw))
	default:
		c.key = inline
	}
	return c, nil
}

func validPrefix(s string) bool {
	if len(s) < 1 || len(s) > 40 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

func (c searchConfig) open() search.Engine {
	switch c.engine {
	case "meilisearch":
		return meili.New(c.url, c.key, c.prefix)
	case "opensearch":
		return opensearch.New(c.url, c.user, c.key, c.prefix)
	}
	return nil
}

// startSearch sets up the search of serve: the service with its fallback, and an indexer per tenant that feeds the
// search service from the queue of that tenant. Without a search service only titles are searched, and the queues wait.
func startSearch(ctx context.Context, config searchConfig, database *db.DB, store *library.Store, log *slog.Logger) *search.Service {
	engine := config.open()
	service := search.NewService(engine, web.TitleFallback(database, store), tenant.Main, log)
	if engine == nil {
		log.Info("no search service is set up, only titles are searched (UNDERDOC_SEARCH_URL)")
		return service
	}
	// The folder belongs to the instance, so only Main shows it and only Main's index has it (docs/tenants.md).
	inMain := context.WithoutCancel(tenant.With(ctx, tenant.Main))
	store.Watch(func(ids []string) {
		keys := make([]search.Key, len(ids))
		for i, id := range ids {
			keys[i] = search.Key{Source: library.SourceFolder, Key: id}
		}
		if err := database.Enqueue(inMain, keys); err != nil {
			log.Error("pages of the folder not queued for the search", slog.String("error", err.Error()))
		}
	})
	go runIndexers(ctx, engine, database, store, log)
	log.Info("search service set up", slog.String("engine", engine.Name()))
	return service
}

// runIndexers keeps one indexer running for each tenant, also for a tenant made later, each as its tenant.
func runIndexers(ctx context.Context, engine search.Engine, database *db.DB, store *library.Store, log *slog.Logger) {
	running := map[string]bool{}
	for {
		ids, err := database.TenantIDs(ctx)
		if err != nil && ctx.Err() == nil {
			log.Error("tenants not read for the search", slog.String("error", err.Error()))
		}
		for _, id := range ids {
			if running[id] || id == tenant.Hoster {
				continue
			}
			running[id] = true
			sources := []search.Source{database.SearchSource()}
			if id == tenant.Main {
				sources = append(sources, store.SearchSource())
			}
			go search.NewIndexer(engine, database, id, log, sources...).Run(tenant.With(ctx, id))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
	}
}

// reindex queues everything of a tenant for the search service again, for after a restore or when the index was lost.
func reindex(ctx context.Context, stdout io.Writer) error {
	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()
	keys, err := database.SearchSource().Keys(ctx)
	if err != nil {
		return err
	}
	queued := make([]search.Key, len(keys))
	for i, key := range keys {
		queued[i] = search.Key{Source: db.SourceObjects, Key: key}
	}
	if err := database.Enqueue(ctx, queued); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%d objects queued for the search; the pages of the folder are queued when serve starts\n", len(keys))
	return err
}
