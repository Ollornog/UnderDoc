// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/library"
	"github.com/Ollornog/UnderDoc/internal/search"
	"github.com/Ollornog/UnderDoc/internal/search/meili"
	"github.com/Ollornog/UnderDoc/internal/search/searchtest"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

type searchSite struct {
	*site
	indexer *search.Indexer
	db      *db.DB
}

// withSearch is a site with a database and the search service set up as serve does it, over the given engine (nil: none).
func withSearch(t *testing.T, engine search.Engine) *searchSite {
	t.Helper()
	ctx := tenant.With(context.Background(), tenant.Main)
	database, err := db.Open(ctx, pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	behindGate(t, database, tenant.Main)
	s := &searchSite{db: database}
	options := Options{DB: database}
	s.site = newSite(t, options, sample)
	log := slog.New(slog.DiscardHandler)
	service := search.NewService(engine, TitleFallback(database, s.store), tenant.Main, log)
	options.Search = service
	handler, err := New(s.store, log, options)
	if err != nil {
		t.Fatal(err)
	}
	s.http = handler
	if engine != nil {
		s.indexer = search.NewIndexer(engine, database, tenant.Main, log, database.SearchSource(), s.store.SearchSource())
		if err := s.indexer.Prepare(ctx); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = engine.Drop(context.Background(), tenant.Main) })
		s.store.Watch(func(ids []string) {
			keys := make([]search.Key, len(ids))
			for i, id := range ids {
				keys[i] = search.Key{Source: library.SourceFolder, Key: id}
			}
			if err := database.Enqueue(ctx, keys); err != nil {
				t.Error(err)
			}
		})
	}
	return s
}

func (s *searchSite) drain() {
	s.t.Helper()
	if err := s.indexer.Drain(tenant.With(context.Background(), tenant.Main)); err != nil {
		s.t.Fatal(err)
	}
}

func (s *searchSite) find(query string, headers map[string]string) apiSearchResult {
	s.t.Helper()
	return decode[apiSearchResult](s.t, s.getWith("/api/search?"+query, merged(headers)))
}

func meiliEngine(t *testing.T) search.Engine {
	return meili.New(searchtest.URL(t, searchtest.EnvMeilisearch), "", searchtest.Prefix())
}

func addBlocks(t *testing.T, s *site, pageID string, bodies ...string) []string {
	t.Helper()
	var ids []string
	after := ""
	for _, body := range bodies {
		created := decode[apiBlock](t, s.send(http.MethodPost, "/api/pages/"+pageID+"/blocks", `{"after":"`+after+`","md":`+string(must(json.Marshal(body)))+`}`, sameSite))
		after = created.ID
		ids = append(ids, created.ID)
	}
	return ids
}

func TestSearchThroughTheService(t *testing.T) {
	s := withSearch(t, meiliEngine(t))
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Filiale"}`, sameSite))
	handbook := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kasse bedienen","parent":"`+team.ID+`"}`, sameSite))
	blocks := addBlocks(t, s.site, handbook.ID, "Die Kasse ist das wichtigste Werkzeug.", "## Kartenzahlung",
		"Karte vorhalten, PIN eingeben.", "## Fehler", "Bei E-1042 das Terminal neu starten.")
	state := decode[apiState](t, s.get("/api/state"))
	notes := state.Collections[0].ID
	private := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Notiz zur Kartenzahlung","parent":"`+notes+`"}`, sameSite))
	addBlocks(t, s.site, private.ID, "Nur für mich.")
	s.drain()

	result := s.find("q=Kartenzahlung", nil)
	if result.Engine != "meilisearch" || result.Notice != "" || len(result.Hits) != 2 {
		t.Fatalf("the service answers with both pages: %+v", result)
	}
	if hit := result.Hits[0]; hit.ID != private.ID {
		t.Errorf("the title before a heading: %+v", result.Hits)
	}
	if hit := result.Hits[1]; hit.ID != handbook.ID || hit.Anchor != blocks[1] || hit.Heading != "Kartenzahlung" || hit.Place != "Filiale" {
		t.Errorf("a hit opens at the block of its section: %+v", hit)
	}
	if got := s.find("q=Kartenzahlung", bob); len(got.Hits) != 1 || got.Hits[0].ID != handbook.ID {
		t.Errorf("bob never finds Anna's private notes: %+v", got.Hits)
	}
	code := s.find("q=E-1042", nil)
	if len(code.Hits) != 1 || code.Hits[0].Anchor != blocks[3] || !code.Hits[0].Code || !strings.Contains(code.Hits[0].Snippet, "<mark>E-1042</mark>") {
		t.Errorf("a code finds its section, marked: %+v", code.Hits)
	}
	if got := s.find("q=E-1043", nil); len(got.Hits) != 0 {
		t.Errorf("a code one digit off finds nothing: %+v", got.Hits)
	}
	// The folder is searched too.
	if got := s.find("q=router", nil); len(got.Hits) == 0 || got.Hits[0].ID != "plan" {
		t.Errorf("the pages of the folder are in the index: %+v", got.Hits)
	}
	// Filters, and the choices they offer.
	filtered := s.find("q=Kartenzahlung&collection="+team.ID, nil)
	if len(filtered.Hits) != 1 || filtered.Hits[0].ID != handbook.ID {
		t.Errorf("filter by collection: %+v", filtered.Hits)
	}
	var names []string
	for _, f := range result.Facets["collection"] {
		names = append(names, f.Label)
	}
	if !strings.Contains(strings.Join(names, ","), "Filiale") || !strings.Contains(strings.Join(names, ","), "My notes") {
		t.Errorf("the collection filter offers their names: %+v", result.Facets)
	}

	// A change reaches the index through the queue; the trash leaves it.
	s.send(http.MethodPut, "/api/pages/"+handbook.ID+"/blocks/"+blocks[4], `{"md":"Bei E-1050 Papier nachlegen."}`, sameSite)
	s.send(http.MethodDelete, "/api/objects/"+private.ID, "", sameSite)
	s.drain()
	if got := s.find("q=E-1042", nil); len(got.Hits) != 0 {
		t.Errorf("the old text is gone from the index: %+v", got.Hits)
	}
	if got := s.find("q=Kartenzahlung", nil); len(got.Hits) != 1 {
		t.Errorf("what is in the trash is not found: %+v", got.Hits)
	}
}

func TestSearchWithoutService(t *testing.T) {
	s := withSearch(t, nil)
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Filiale"}`, sameSite))
	decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kartenzahlung","parent":"`+team.ID+`"}`, sameSite))
	result := s.find("q=kartenzahlung", nil)
	if result.Engine != search.EngineTitles || result.Notice != search.NoticeNoService || len(result.Hits) != 1 || result.Hits[0].Place != "Filiale" {
		t.Errorf("without a search service, titles are searched, and the answer says so: %+v", result)
	}
	if got := s.find("q=network", nil); len(got.Hits) != 1 || got.Hits[0].ID != "plan" {
		t.Errorf("titles of the folder too: %+v", got.Hits)
	}
	if got := s.find("tag=nothing-has-it", nil); len(got.Hits) != 0 || got.Notice != search.NoticeNoService {
		t.Errorf("a tag alone lists what carries it: %+v", got)
	}
}

func TestSearchServiceDown(t *testing.T) {
	// Nothing listens there: the request is refused at once.
	s := withSearch(t, nil)
	down := meili.New("http://127.0.0.1:1", "", "down")
	options := Options{DB: s.db, Search: search.NewService(down, TitleFallback(s.db, s.store), tenant.Main, slog.New(slog.DiscardHandler))}
	handler, err := New(s.store, slog.New(slog.DiscardHandler), options)
	if err != nil {
		t.Fatal(err)
	}
	s.http = handler
	result := s.find("q=network+plan", nil)
	if result.Engine != search.EngineTitles || result.Notice != search.NoticeServiceDown || len(result.Hits) != 1 {
		t.Errorf("a search service that does not answer falls back to titles: %+v", result)
	}
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

// TestEveryTenantSearchesItsOwnIndex: each tenant has a queue and an index of its own (ADR-23), and a search finds only
// what its tenant has, also through a code, which no filter of readers could keep apart.
func TestEveryTenantSearchesItsOwnIndex(t *testing.T) {
	inMain := tenant.With(context.Background(), tenant.Main)
	database, err := db.Open(inMain, pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if _, err := database.CreateTenant(inMain, "acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	inAcme := tenant.With(context.Background(), "acme")
	engine := meiliEngine(t)
	log := slog.New(slog.DiscardHandler)
	user := account.User{ID: "u-anna", Login: "anna", Name: "Anna"}
	for _, ctx := range []context.Context{inMain, inAcme} {
		id, _ := tenant.From(ctx)
		t.Cleanup(func() { _ = engine.Drop(context.Background(), id) })
		if err := database.SeeAccount(ctx, user); err != nil {
			t.Fatal(err)
		}
		team, err := database.CreateCollection(ctx, user, "Team "+id, "")
		if err != nil {
			t.Fatal(err)
		}
		page, err := database.CreatePage(ctx, user, "Kasse "+id, team.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := database.AddBlock(ctx, page.ID, user.ID, "", "Fehler E-1042 bei "+id); err != nil {
			t.Fatal(err)
		}
		indexer := search.NewIndexer(engine, database, id, log, database.SearchSource())
		if err := indexer.Prepare(ctx); err != nil {
			t.Fatal(err)
		}
		if err := indexer.Drain(ctx); err != nil {
			t.Fatal(err)
		}
	}
	service := search.NewService(engine, TitleFallback(database, nil), tenant.Main, log)
	for _, ctx := range []context.Context{inMain, inAcme} {
		id, _ := tenant.From(ctx)
		for _, q := range []string{"E-1042", "Kasse"} {
			result, err := service.Search(ctx, search.Request{Text: q, Account: user.ID, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Hits) != 1 || result.Hits[0].Title != "Kasse "+id {
				t.Errorf("%s searching %q finds only its own page: %+v", id, q, result.Hits)
			}
		}
	}
}
