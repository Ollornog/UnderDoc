// SPDX-License-Identifier: MIT

package searchtest

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/search"
)

// NewEngine returns the engine under test, with indexes named after prefix.
type NewEngine func(prefix string) search.Engine

// Run runs every functional test against an engine.
func Run(t *testing.T, newEngine NewEngine) {
	engine := newEngine(Prefix())
	ctx := Context(t)
	tests := []struct {
		name string
		test func(*testing.T, context.Context, search.Engine, string)
	}{
		{"readers", testReaders},
		{"tenants", testTenants},
		{"replace", testReplace},
		{"filters", testFilters},
		{"one per object", testOnePerObject},
		{"codes exactly", testCodes},
		{"typing errors in words", testTypos},
		{"title before heading before text", testFieldOrder},
		{"marks", testMarks},
		{"exact words", testExact},
		{"prepare again", testPrepareAgain},
	}
	for i, tt := range tests {
		tenant := "suite" + string(rune('a'+i))
		t.Run(tt.name, func(t *testing.T) {
			if _, err := engine.Prepare(ctx, tenant); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = engine.Drop(context.Background(), tenant) })
			tt.test(t, ctx, engine, tenant)
		})
	}
}

func page(object, title string, sections ...string) []search.Doc {
	base := search.Doc{Object: object, Type: search.TypePage, Title: title, Readers: []string{search.Everyone},
		Collection: "c1", CollectionName: "Handbook", Author: "anna", AuthorName: "Anna", Changed: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC).Unix()}
	var secs []search.Section
	for i := 0; i+1 < len(sections); i += 2 {
		secs = append(secs, search.Section{Anchor: "b" + string(rune('1'+i/2)), Heading: sections[i], Text: sections[i+1]})
	}
	return search.SectionDocs(base, secs)
}

func objects(t *testing.T, ctx context.Context, service *search.Service, account, text string, filters search.Filters, groups ...string) []string {
	t.Helper()
	result, err := service.Search(ctx, search.Request{Text: text, Account: account, Groups: groups, Filters: filters, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if result.Notice != "" {
		t.Fatalf("the engine did not answer %q: %s", text, result.Notice)
	}
	var out []string
	for _, h := range result.Hits {
		out = append(out, h.Object)
	}
	return out
}

func serviceOf(engine search.Engine, tenant string) *search.Service {
	return search.NewService(engine, nil, tenant, Quiet())
}

func testReaders(t *testing.T, ctx context.Context, engine search.Engine, tenant string) {
	open := page("open", "Kassenhandbuch", "Kartenzahlung", "Die Kartenzahlung beginnt mit der Taste Karte.")
	private := page("private", "Meine Notizen zur Kartenzahlung", "", "Nur für mich: Kartenzahlung privat notiert.")
	for i := range private {
		private[i].Readers = []string{search.ReaderAccount("anna")}
	}
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "open"}, open)
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "private"}, private)
	// A page the group "board" reads (ADR-29): found by its members only.
	board := page("board", "Vorstand zur Kartenzahlung", "", "Beschluss: Kartenzahlung ab Januar.")
	for i := range board {
		board[i].Readers = []string{search.ReaderGroup("board")}
	}
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "board"}, board)
	service := serviceOf(engine, tenant)
	if got := objects(t, ctx, service, "carl", "Kartenzahlung", search.Filters{}, "board"); !sameSet(got, "open", "board") {
		t.Errorf("carl reads the open page and, through his group, the board's: %v", got)
	}
	if got := objects(t, ctx, service, "anna", "Kartenzahlung", search.Filters{}); !sameSet(got, "open", "private") {
		t.Errorf("anna reads both pages: %v", got)
	}
	if got := objects(t, ctx, service, "bob", "Kartenzahlung", search.Filters{}); !slices.Equal(got, []string{"open"}) {
		t.Errorf("bob must not find anna's private page: %v", got)
	}
	if got := objects(t, ctx, service, "bob", "", search.Filters{Types: []string{search.TypePage}}); slices.Contains(got, "private") {
		t.Errorf("not even a search by filter alone may find it: %v", got)
	}
	if _, err := engine.Search(ctx, tenant, search.Query{Text: "Kartenzahlung", Limit: 5}); err == nil {
		t.Error("a query without readers must be refused, not run without them")
	}
}

func testTenants(t *testing.T, ctx context.Context, engine search.Engine, tenant string) {
	other := tenant + "-other"
	if _, err := engine.Prepare(ctx, other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Drop(context.Background(), other) })
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "mine"}, page("mine", "Lieferantenliste Nord"))
	if got := objects(t, ctx, serviceOf(engine, other), "anna", "Lieferantenliste", search.Filters{}); len(got) != 0 {
		t.Errorf("another tenant must find nothing of this one: %v", got)
	}
	if got := objects(t, ctx, serviceOf(engine, tenant), "anna", "Lieferantenliste", search.Filters{}); !slices.Equal(got, []string{"mine"}) {
		t.Errorf("the tenant finds its own page: %v", got)
	}
}

func testReplace(t *testing.T, ctx context.Context, engine search.Engine, tenant string) {
	key := search.Key{Source: "object", Key: "p"}
	Load(t, ctx, engine, tenant, key, page("p", "Inventur", "Zählung", "Die Zählung beginnt im Lager Ost.", "Abschluss", "Abschluss mit Unterschrift."))
	service := serviceOf(engine, tenant)
	if got := objects(t, ctx, service, "anna", "Lager Ost", search.Filters{}); !slices.Equal(got, []string{"p"}) {
		t.Fatalf("the page is found: %v", got)
	}
	Load(t, ctx, engine, tenant, key, page("p", "Inventur", "Zählung", "Die Zählung beginnt im Lager West."))
	if got := objects(t, ctx, service, "anna", "Unterschrift", search.Filters{}); len(got) != 0 {
		t.Errorf("a section that is gone must not be found: %v", got)
	}
	if got := objects(t, ctx, service, "anna", "West", search.Filters{}); !slices.Equal(got, []string{"p"}) {
		t.Errorf("the new text is found: %v", got)
	}
	if err := engine.Replace(ctx, tenant, []search.Key{key}, nil); err != nil {
		t.Fatal(err)
	}
	if got := objects(t, ctx, service, "anna", "Inventur", search.Filters{}); len(got) != 0 {
		t.Errorf("a removed page must not be found: %v", got)
	}
}

func testFilters(t *testing.T, ctx context.Context, engine search.Engine, tenant string) {
	day := func(d int) int64 { return time.Date(2026, 1, d, 12, 0, 0, 0, time.UTC).Unix() }
	docs := []struct {
		object, typ, collection, author string
		tags                            []string
		changed                         int64
	}{
		{"a", search.TypePage, "c1", "anna", []string{"kasse", "netz"}, day(2)},
		{"b", search.TypePage, "c2", "bob", []string{"kasse"}, day(10)},
		{"c", search.TypeFolder, "c1", "bob", nil, day(20)},
	}
	for _, d := range docs {
		doc := search.Doc{Object: d.object, Type: d.typ, Title: "Kassennetz " + d.object, Collection: d.collection,
			Author: d.author, Tags: d.tags, Changed: d.changed, Readers: []string{search.Everyone}}
		Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: d.object}, []search.Doc{doc})
	}
	service := serviceOf(engine, tenant)
	cases := []struct {
		name    string
		filters search.Filters
		want    []string
	}{
		{"type", search.Filters{Types: []string{search.TypeFolder}}, []string{"c"}},
		{"collection", search.Filters{Collections: []string{"c1"}}, []string{"a", "c"}},
		{"author", search.Filters{Authors: []string{"bob"}}, []string{"b", "c"}},
		{"every tag", search.Filters{Tags: []string{"kasse", "netz"}}, []string{"a"}},
		{"from", search.Filters{From: time.Unix(day(5), 0)}, []string{"b", "c"}},
		{"to", search.Filters{To: time.Unix(day(15), 0)}, []string{"a", "b"}},
		{"together", search.Filters{Collections: []string{"c1"}, Authors: []string{"bob"}}, []string{"c"}},
	}
	for _, c := range cases {
		if got := objects(t, ctx, service, "anna", "Kassennetz", c.filters); !sameSet(got, c.want...) {
			t.Errorf("filter %s: got %v, want %v", c.name, got, c.want)
		}
	}
	// Without text, the filters alone list what fits, the newest first.
	if got := objects(t, ctx, service, "anna", "", search.Filters{Tags: []string{"kasse"}}); !slices.Equal(got, []string{"b", "a"}) {
		t.Errorf("filters without text, newest first: %v", got)
	}
	result, err := service.Search(ctx, search.Request{Text: "Kassennetz", Account: "anna", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Facets[search.FacetType][search.TypePage] == 0 || result.Facets[search.FacetAuthor]["bob"] == 0 ||
		result.Facets[search.FacetTag]["kasse"] == 0 || result.Facets[search.FacetCollection]["c2"] == 0 {
		t.Errorf("facets count types, authors, tags and collections: %v", result.Facets)
	}
}

func testOnePerObject(t *testing.T, ctx context.Context, engine search.Engine, tenant string) {
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "k"}, page("k", "Kasse bedienen",
		"Anmelden", "Mit der Kassenkarte anmelden.", "Kartenzahlung", "Kartenzahlung mit PIN.", "Storno", "Eine Kartenzahlung stornieren."))
	result, err := serviceOf(engine, tenant).Search(ctx, search.Request{Text: "Kartenzahlung", Account: "anna", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 {
		t.Fatalf("one hit per page, not one per section: %d", len(result.Hits))
	}
	if result.Hits[0].Heading != "Kartenzahlung" || result.Hits[0].Anchor != "b2" {
		t.Errorf("the hit opens at the section with the heading: %q #%s", result.Hits[0].Heading, result.Hits[0].Anchor)
	}
	// The engine keeps one record per object itself, so the sections of one page do not take the places of other pages.
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "many"}, page("many", "Kartenzahlung",
		"Ablauf", "Kartenzahlung Schritt eins.", "Fehler", "Kartenzahlung Fehler.", "Storno", "Kartenzahlung Storno.", "Belege", "Kartenzahlung Belege."))
	result, err = serviceOf(engine, tenant).Search(ctx, search.Request{Text: "Kartenzahlung", Account: "anna", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 2 {
		t.Errorf("two places, two pages: %d", len(result.Hits))
	}
	answer, err := engine.Search(ctx, tenant, search.Query{Text: "Kartenzahlung", Readers: search.Readers("anna"), Limit: 10, OnePerObject: true})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, f := range answer.Found {
		if seen[f.Doc.Object] {
			t.Errorf("the engine returns %s twice", f.Doc.Object)
		}
		seen[f.Doc.Object] = true
	}
}

func testCodes(t *testing.T, ctx context.Context, engine search.Engine, tenant string) {
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "e1042"}, page("e1042", "E-1042 Kartenterminal antwortet nicht",
		"Ursache", "Das Terminal ist nicht im Netz."))
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "e1043"}, page("e1043", "E-1043 Zeitüberschreitung",
		"Ursache", "Der Server antwortet zu spät, anders als bei E-1042."))
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "win"}, page("win", "Windows Update",
		"Zugriff verweigert", "Fehler 0x80070005 beim Update.", "Hinweis", "Nicht verwechseln mit HTTP 503."))
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "ora"}, page("ora", "Datenbankfehler",
		"ORA-00942", "Tabelle oder View existiert nicht."))
	service := serviceOf(engine, tenant)
	first := map[string]string{
		"E-1042": "e1042", "e1042": "e1042", "E-1043": "e1043", "0x80070005": "win", "80070005": "win",
		"ORA-00942": "ora", "ora-942": "ora", "00942": "ora", "HTTP 503": "win", "Kartenterminal E-1042": "e1042",
	}
	for query, want := range first {
		got := objects(t, ctx, service, "anna", query, search.Filters{})
		if len(got) == 0 || got[0] != want {
			t.Errorf("%q: first hit %v, want %s", query, got, want)
		}
	}
	// Never through a typing error: a code one digit off finds nothing.
	for _, query := range []string{"0x80070015", "E-1049", "ORA-00943", "HTTP 504"} {
		if got := objects(t, ctx, service, "anna", query, search.Filters{}); len(got) != 0 {
			t.Errorf("%q must find nothing, not a code that looks alike: %v", query, got)
		}
	}
}

func testTypos(t *testing.T, ctx context.Context, engine search.Engine, tenant string) {
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "z"}, page("z", "Zertifikat erneuern",
		"Vorgehen", "Das Zertifikat des Webservers läuft jährlich ab."))
	service := serviceOf(engine, tenant)
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "w"}, page("w", "Hof", "Parken", "Der Wagen steht im Hof."))
	// One letter off in a word of five: no part of it is left to match, only a typing error finds it.
	if got := objects(t, ctx, service, "anna", "Wafen", search.Filters{}); !slices.Equal(got, []string{"w"}) {
		t.Errorf("%q finds the page: %v", "Wafen", got)
	}
	for _, query := range []string{"Zertifkat", "Zertifikat erneuern", "Webservers"} {
		if got := objects(t, ctx, service, "anna", query, search.Filters{}); !slices.Equal(got, []string{"z"}) {
			t.Errorf("%q finds the page: %v", query, got)
		}
	}
}

func testFieldOrder(t *testing.T, ctx context.Context, engine search.Engine, tenant string) {
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "text"}, page("text", "Protokoll März",
		"Punkte", "Die Lieferkette wurde besprochen, auch die Lieferkette im Lager und die Lieferkette der Filialen."))
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "heading"}, page("heading", "Einkauf",
		"Lieferkette", "Wer bestellt bei wem."))
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "title"}, page("title", "Lieferkette",
		"Überblick", "Vom Hersteller bis zum Regal."))
	got := objects(t, ctx, serviceOf(engine, tenant), "anna", "Lieferkette", search.Filters{})
	if !slices.Equal(got, []string{"title", "heading", "text"}) {
		t.Errorf("title, then heading, then text, however often the word stands in the text: %v", got)
	}
}

func testMarks(t *testing.T, ctx context.Context, engine search.Engine, tenant string) {
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "m"}, page("m", "Netzwerk",
		"Drucker", "Der Netzwerkdrucker im Lager steht hinter der Tür <script>."))
	result, err := serviceOf(engine, tenant).Search(ctx, search.Request{Text: "Lager", Account: "anna", Limit: 5})
	if err != nil || len(result.Hits) != 1 {
		t.Fatalf("one hit: %v %v", result.Hits, err)
	}
	var marked, all strings.Builder
	for _, f := range result.Hits[0].Snippet {
		all.WriteString(f.Text)
		if f.Match {
			marked.WriteString(f.Text)
		}
	}
	if marked.String() != "Lager" || !strings.Contains(all.String(), "<script>") {
		t.Errorf("the snippet marks the match and keeps the text as text: %q in %q", marked.String(), all.String())
	}
}

func testExact(t *testing.T, ctx context.Context, engine search.Engine, tenant string) {
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "a"}, page("a", "Kasse", "Storno", "Die Kartenzahlung stornieren geht nur am selben Tag."))
	Load(t, ctx, engine, tenant, search.Key{Source: "object", Key: "b"}, page("b", "Retoure", "Ablauf", "Stornieren einer Retoure, danach die Kartenzahlung erstatten."))
	service := serviceOf(engine, tenant)
	exact := func(text string) []string {
		result, err := service.Search(ctx, search.Request{Text: text, Account: "anna", Limit: 10, Exact: true})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, h := range result.Hits {
			out = append(out, h.Object)
		}
		return out
	}
	if got := exact("Kartenzahlung stornieren"); !slices.Equal(got, []string{"a"}) {
		t.Errorf("exact words in their order: %v", got)
	}
	if got := exact("Kartenzahlnug"); len(got) != 0 {
		t.Errorf("exact words take no typing error: %v", got)
	}
	if got := objects(t, ctx, service, "anna", "Kartenzahlung stornieren", search.Filters{}); !sameSet(got, "a", "b") {
		t.Errorf("the full text finds both: %v", got)
	}
}

func testPrepareAgain(t *testing.T, ctx context.Context, engine search.Engine, tenant string) {
	rebuild, err := engine.Prepare(ctx, tenant)
	if err != nil || rebuild {
		t.Errorf("preparing an index that is ready changes nothing: rebuild=%v %v", rebuild, err)
	}
	if err := engine.Drop(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	rebuild, err = engine.Prepare(ctx, tenant)
	if err != nil || !rebuild {
		t.Errorf("a missing index has to be built anew: rebuild=%v %v", rebuild, err)
	}
}

func sameSet(got []string, want ...string) bool {
	return len(got) == len(want) && !slices.ContainsFunc(want, func(w string) bool { return !slices.Contains(got, w) })
}
