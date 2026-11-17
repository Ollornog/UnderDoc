//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/folder"
	"github.com/Ollornog/UnderDoc/internal/library"
	"github.com/Ollornog/UnderDoc/internal/search"
	"github.com/Ollornog/UnderDoc/internal/search/meili"
	"github.com/Ollornog/UnderDoc/internal/search/searchtest"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/web"
)

// The search of M-12 in the browser (ADR-24): Ctrl+K, hits while typing, the jump to the section with the words marked,
// and the results page with its filters. Everything is typed and clicked with real keys and the real mouse, against a
// real Meilisearch fed by the indexer as serve feeds it.

func setupWithSearch(t *testing.T) (*browser, string) {
	t.Helper()
	ctx, cancel := context.WithCancel(tenant.With(context.Background(), tenant.Main))
	t.Cleanup(cancel)
	database, err := db.Open(ctx, pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	engine := meili.New(searchtest.URL(t, searchtest.EnvMeilisearch), "", searchtest.Prefix())
	t.Cleanup(func() { _ = engine.Drop(context.Background(), tenant.Main) })
	log := slog.New(slog.DiscardHandler)
	// The folder of setupWith is read again here only for the title fallback; the indexer takes the pages of the database.
	store := library.NewStore(folder.New(t.TempDir()), log)
	if err := store.Refresh(); err != nil {
		t.Fatal(err)
	}
	indexer := search.NewIndexer(engine, database, tenant.Main, log, database.SearchSource())
	indexer.Poll = 50 * time.Millisecond
	go indexer.Run(ctx)
	service := search.NewService(engine, web.TitleFallback(database, store), tenant.Main, log)
	return setupWith(t, web.Options{DB: database, Search: service}, nil)
}

// indexed waits until the search finds a word, so the indexer has done its work.
func (b *browser) indexed(word string) {
	b.t.Helper()
	b.eval(`window.found = false; (async () => { for (let i = 0; i < 200 && !window.found; i++) {
		const r = await (await fetch('/api/search?q=' + encodeURIComponent(` + strconv.Quote(word) + `))).json();
		window.found = r.engine === 'meilisearch' && r.hits.length > 0; if (!window.found) await new Promise(f => setTimeout(f, 50)); } })()`)
	if !b.waitFor(`window.found === true`) {
		b.t.Fatalf("%q did not reach the index", word)
	}
}

const ctrl = 2

func TestQuickSearchJumpsToTheSection(t *testing.T) {
	b, base := setupWithSearch(t)
	b.open(base + "/")
	b.ready("")
	filler := strings.Repeat("Ein Absatz, der nur Platz nimmt, damit die Seite scrollen muss. ", 30)
	blocks := []string{"Die Kasse ist das wichtigste Werkzeug.", filler, filler, "## Kartenzahlung", "Karte vorhalten und die PIN eingeben.", filler, filler,
		"## Störungen", "Bei E-1042 das Terminal neu starten und das Kabel prüfen."}
	page := makePage(b, "Kasse bedienen", blocks...)
	b.indexed("E-1042")
	b.open(base + "/")
	b.ready("")

	b.press("k", "KeyK", 75, ctrl)
	if !b.waitFor(`document.activeElement?.dataset.palq !== undefined`) {
		t.Fatalf("Ctrl+K opens the quick search with the focus in its field: %s | %s", b.eval(`document.activeElement?.tagName`), b.eval(`String(!!document.querySelector('.udpal')) + ' ' + (document.querySelector('.udpal')?.outerHTML.slice(0, 300))`))
	}
	b.typeText("E-1042")
	if !b.waitFor(`[...document.querySelectorAll('.udpal .hit mark')].some(m => m.textContent === 'E-1042')`) {
		t.Fatalf("hits show while typing, the code marked: %s", b.eval(`document.querySelector('.udpal')?.innerText`))
	}
	if got := b.eval(`document.querySelector('.udpal .hit small')?.textContent`); !strings.Contains(got, "Störungen") {
		t.Errorf("a hit names its place and section: %q", got)
	}
	b.shot("search-palette")
	// The first hit is chosen; the arrows move the choice and back.
	b.press("ArrowDown", "ArrowDown", 40, 0)
	b.press("ArrowUp", "ArrowUp", 38, 0)
	b.press("Enter", "Enter", 13, 0)
	if !b.waitFor(`location.pathname === '/o/` + page + `' && !!document.querySelector('article.doc mark.hitmark')`) {
		t.Fatalf("Enter opens the page with the code marked: %s %s %s", b.eval(`location.pathname`), b.eval(`JSON.stringify(S.wins.map(w => w.tabs.map(t => [t.d, t.hl])))`), b.eval(`document.querySelector('[data-art]')?.innerText.slice(-300)`))
	}
	if got := b.eval(`document.querySelector('mark.hitmark').closest('[data-b]')?.textContent.startsWith('Bei E-1042') + ' ' + document.querySelector('mark.hitmark').textContent`); got != "true E-1042" {
		t.Errorf("the mark sits in the section of the hit: %q", got)
	}
	inView := `(() => { const r = document.querySelector('mark.hitmark').getBoundingClientRect(); return r.top >= 0 && r.bottom <= innerHeight; })()`
	if !b.waitFor(inView) {
		t.Errorf("the page opens at the section, not at its top")
	}
	if got := b.eval(`!!document.querySelector('.udpal')`); got != "false" {
		t.Error("the palette closes when a hit opens")
	}
	b.shot("search-jump")

	// The palette also jumps to a page by its title, and Escape closes it.
	b.press("k", "KeyK", 75, ctrl)
	b.waitFor(`document.activeElement?.dataset.palq !== undefined`)
	b.typeText("Kasse bed")
	if !b.waitFor(`document.querySelector('.udpal [data-sact="open"]')?.textContent.includes('Kasse bedienen')`) {
		t.Fatalf("pages show by their title at once: %s", b.eval(`document.querySelector('.udpal')?.innerText`))
	}
	b.press("Escape", "Escape", 27, 0)
	if !b.waitFor(`!document.querySelector('.udpal')`) {
		t.Error("Escape closes the palette")
	}
}

func TestResultsPageWithFilters(t *testing.T) {
	b, base := setupWithSearch(t)
	b.open(base + "/")
	b.ready("")
	makePage(b, "Kartenzahlung", "Wie man mit Karte zahlt.")
	other := makePage(b, "Kasse bedienen", "## Kartenzahlung", "Karte vorhalten.")
	b.eval(`fetch('/api/objects/` + other + `/tags', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({tags: ['kasse']})})`)
	b.indexed("Kartenzahlung")
	b.eval(`window.tagged = false; (async () => { for (let i = 0; i < 200 && !window.tagged; i++) {
		const r = await (await fetch('/api/search?q=Kartenzahlung&tag=kasse')).json(); window.tagged = r.hits.length === 1;
		if (!window.tagged) await new Promise(f => setTimeout(f, 50)); } })()`)
	if !b.waitFor(`window.tagged === true`) {
		t.Fatal("the tag did not reach the index")
	}
	b.open(base + "/")
	b.ready("")
	b.press("/", "Slash", 191, 0)
	b.typeText("Kartenzahlung")
	if !b.waitFor(`document.querySelectorAll('#sres .hit').length === 2`) {
		t.Fatalf("both pages are hits: %s", b.eval(`document.getElementById('sres')?.innerText`))
	}
	if got := b.eval(`document.querySelector('#sres .hit').textContent`); !strings.HasPrefix(got, "Kartenzahlung") {
		t.Errorf("the page with the word as its title comes first: %q", got)
	}
	b.clickOn(`.spanel .filterbtn`, 1)
	b.clickOn(`.spanel .tagc[data-tag="kasse"]`, 1)
	if !b.waitFor(`document.querySelectorAll('#sres .hit').length === 1 && document.querySelector('#sres .hit').textContent.startsWith('Kasse bedienen')`) {
		t.Fatalf("a tag narrows the hits: %s", b.eval(`document.getElementById('sres')?.innerText`))
	}
	b.shot("search-results")
	b.clickOn(`#sres .hit`, 1)
	if !b.waitFor(`location.pathname === '/o/` + other + `' && document.querySelector('mark.hitmark')?.textContent === 'Kartenzahlung'`) {
		t.Errorf("a click opens the hit with the heading marked: %s %s %s", b.eval(`location.pathname`), b.eval(`JSON.stringify(S.wins.map(w => w.tabs.map(t => [t.d, t.hl])))`), b.eval(`document.querySelector('[data-art]')?.innerHTML.slice(0, 600)`))
	}
}
