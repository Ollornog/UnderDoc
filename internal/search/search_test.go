// SPDX-License-Identifier: MIT

package search

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestSectionsFollowHeadings(t *testing.T) {
	blocks := []Block{
		{"b1", "Intro text."},
		{"b2", "## Kartenzahlung"},
		{"b3", "Karte vorhalten.\n\n| Code | Bedeutung |\n|---|---|\n| E-1042 | Terminal |"},
		{"b4", "### PIN"},
		{"b5", "Vier Ziffern."},
		{"b6", "# Ende"},
	}
	sections := Sections(BlockParts(blocks))
	want := []Section{
		{Anchor: "b1", Text: "Intro text."},
		{Anchor: "b2", Heading: "Kartenzahlung", Level: 2, Text: "Karte vorhalten. Code Bedeutung E-1042 Terminal"},
		{Anchor: "b4", Heading: "PIN", Level: 3, Text: "Vier Ziffern."},
		{Anchor: "b6", Heading: "Ende", Level: 1},
	}
	if !slices.Equal(sections, want) {
		t.Errorf("one section per heading, tables as text:\n%+v\nwant\n%+v", sections, want)
	}
}

func TestSectionsOfAWholePage(t *testing.T) {
	page := []byte("# Kasse\n\nIntro.\n\n## Storno\n\nBon verwerfen.\n")
	sections := Sections(PageParts(page, "Kasse"))
	if len(sections) != 2 || sections[0].Anchor != "" || sections[1].Anchor != "h-storno" || sections[1].Heading != "Storno" {
		t.Errorf("the first heading repeating the title is left out, headings keep their anchors: %+v", sections)
	}
}

func TestLongSectionsAreSplit(t *testing.T) {
	word := strings.Repeat("wort ", 1000)
	sections := Sections(BlockParts([]Block{{"b1", "## Lang"}, {"b2", word}, {"b3", word}}))
	if len(sections) != 2 || sections[1].Heading != "Lang" || sections[1].Anchor != "b3" {
		t.Errorf("a long section goes on with its heading at the next block: %d %+v", len(sections), sections)
	}
}

func TestPasswordsNeverReachTheText(t *testing.T) {
	for _, body := range []string{`Zugang: ::secret{id="k7x9"} bitte nicht weitergeben`, `::secret{id="k7x9"}`,
		`::embed{ref="abc123"}`, `::secret[Router]{id="k7x9"}`} {
		for _, s := range Sections(BlockParts([]Block{{"b1", body}})) {
			if strings.Contains(s.Text, "k7x9") || strings.Contains(s.Text, "secret") || strings.Contains(s.Text, "abc123") {
				t.Errorf("%q left %q in the text (ADR-7)", body, s.Text)
			}
		}
	}
}

func TestPrepareFillsCodes(t *testing.T) {
	d := Doc{Title: "E-1042: Terminal", Heading: "Siehe HTTP 503", Body: "Auch ORA-00942 und markiert."}
	d.Prepare()
	if !slices.Equal(d.TitleCodes, []string{"e1042"}) || !slices.Equal(d.HeadingCodes, []string{"http503"}) {
		t.Errorf("codes by place: %v %v", d.TitleCodes, d.HeadingCodes)
	}
	if !slices.Equal(d.Codes, []string{"e1042", "http503", "ora00942"}) || !slices.Contains(d.CodeParts, "942") {
		t.Errorf("all codes and their parts: %v %v", d.Codes, d.CodeParts)
	}
	if strings.ContainsAny(d.Body, MarkStart+MarkEnd) || d.Tags == nil || d.Readers == nil {
		t.Errorf("no marker characters, no nil lists: %q %v %v", d.Body, d.Tags, d.Readers)
	}
}

func TestFragments(t *testing.T) {
	got := Fragments("a b cd")
	want := []Fragment{{Text: "a "}, {Text: "b", Match: true}, {Text: " c"}, {Text: "d", Match: true}}
	if !slices.Equal(got, want) {
		t.Errorf("%+v", got)
	}
}

func TestSnippetMarksCodesAndWords(t *testing.T) {
	text := strings.Repeat("Vorher. ", 20) + "Der Netzwerkdrucker meldet E-1042 und Drucker zwei auch."
	fragments := Snippet(text, []string{"e1042"}, []string{"drucker"})
	var marked []string
	for _, f := range fragments {
		if f.Match {
			marked = append(marked, f.Text)
		}
	}
	if !slices.Equal(marked, []string{"E-1042", "Drucker"}) || fragments[0].Text != "… " {
		t.Errorf("codes as a whole, words at their start, cut around the first: %+v", fragments)
	}
}

// fake is an engine in memory: records, and what it was asked.
type fake struct {
	docs    map[string]Doc
	fail    error
	queries []Query
}

func (f *fake) Name() string                                  { return "fake" }
func (f *fake) Prepare(context.Context, string) (bool, error) { return len(f.docs) == 0, f.fail }
func (f *fake) Drop(context.Context, string) error            { f.docs = nil; return nil }
func (f *fake) Replace(_ context.Context, _ string, keys []Key, docs []Doc) error {
	if f.fail != nil {
		return f.fail
	}
	if f.docs == nil {
		f.docs = map[string]Doc{}
	}
	for id, d := range f.docs {
		if slices.Contains(keys, Key{d.Source, d.Key}) {
			delete(f.docs, id)
		}
	}
	for _, d := range docs {
		f.docs[d.ID] = d
	}
	return nil
}

// Search knows only what the service asks for codes: the records with one of the codes in a field.
func (f *fake) Search(_ context.Context, _ string, q Query) (Answer, error) {
	f.queries = append(f.queries, q)
	if f.fail != nil {
		return Answer{}, f.fail
	}
	var answer Answer
	for _, d := range f.docs {
		match := q.Text != "" && strings.Contains(strings.ToLower(d.Title+" "+d.Body), strings.ToLower(q.Text))
		for _, c := range q.Codes {
			field := map[string][]string{FieldTitleCodes: d.TitleCodes, FieldHeadingCodes: d.HeadingCodes, FieldCodes: d.Codes, FieldCodeParts: d.CodeParts}[c.Field]
			if slices.ContainsFunc(c.Codes, func(code string) bool { return slices.Contains(field, code) }) {
				match = true
			}
		}
		if match {
			answer.Found = append(answer.Found, Found{Doc: d})
		}
	}
	slices.SortFunc(answer.Found, func(a, b Found) int { return strings.Compare(a.Doc.ID, b.Doc.ID) })
	return answer, nil
}

type fakeFallback struct{ asked string }

func (f *fakeFallback) Titles(_ context.Context, text, _ string, _ []string, _ int) ([]Hit, error) {
	f.asked = text
	return []Hit{{Object: "t", Title: text}}, nil
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func load(engine Engine, objects map[string][2]string) {
	for object, tb := range objects {
		key := Key{"object", object}
		d := Doc{Object: object, Title: tb[0], Body: tb[1], Readers: []string{Everyone}, Source: "object", Key: object, ID: DocID(key, 0)}
		d.Prepare()
		_ = engine.Replace(context.Background(), "t", []Key{key}, []Doc{d})
	}
}

func TestCodesComeInTiers(t *testing.T) {
	engine := &fake{}
	load(engine, map[string][2]string{
		"mention": {"Runbook Kasse", "Bei E-1042 neu starten."},
		"catalog": {"E-1042: Terminal antwortet nicht", "Netzwerk prüfen."},
		"other":   {"KB5005565: Drucker meldet E-1042", "Treiber."},
		"part":    {"Liste", "Terminal 1042 im Lager."},
	})
	service := NewService(engine, nil, "t", quiet())
	result, err := service.Search(context.Background(), Request{Text: "E-1042", Account: "a"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, h := range result.Hits {
		got = append(got, h.Object)
	}
	want := []string{"catalog", "other", "mention", "part"}
	if !slices.Equal(got, want) {
		t.Errorf("title leading, title, text, then by a part: %v, want %v", got, want)
	}
	for _, q := range engine.queries {
		if !slices.Equal(q.Readers, Readers("a")) {
			t.Errorf("every query carries the readers: %+v", q)
		}
		if q.Text != "" {
			t.Errorf("a query of codes alone never searches them as text, where typing errors would match: %+v", q)
		}
	}
}

func TestFallbackWithoutEngine(t *testing.T) {
	fallback := &fakeFallback{}
	result, err := NewService(nil, fallback, "t", quiet()).Search(context.Background(), Request{Text: " Kasse ", Account: "a"})
	if err != nil || result.Engine != EngineTitles || result.Notice != NoticeNoService || fallback.asked != "Kasse" || len(result.Hits) != 1 {
		t.Errorf("without an engine the titles answer, and say so: %+v %v", result, err)
	}
	fallback = &fakeFallback{}
	result, err = NewService(&fake{fail: errors.New("down")}, fallback, "t", quiet()).Search(context.Background(), Request{Text: "Kasse", Account: "a"})
	if err != nil || result.Notice != NoticeServiceDown || len(result.Hits) != 1 {
		t.Errorf("an engine that fails hands over to the titles: %+v %v", result, err)
	}
}

// memoryQueue is a queue in memory.
type memoryQueue struct {
	keys     []Key
	retried  []Key
	versions map[string]string
}

func (q *memoryQueue) Claim(_ context.Context, n int) ([]Key, error) {
	n = min(n, len(q.keys))
	out := q.keys[:n]
	q.keys = q.keys[n:]
	return out, nil
}
func (q *memoryQueue) Retry(_ context.Context, keys []Key, _ time.Duration) error {
	q.retried = append(q.retried, keys...)
	return nil
}
func (q *memoryQueue) Enqueue(_ context.Context, keys []Key) error {
	q.keys = append(q.keys, keys...)
	return nil
}
func (q *memoryQueue) Version(_ context.Context, index string) (string, error) {
	return q.versions[index], nil
}
func (q *memoryQueue) SetVersion(_ context.Context, index, version string) error {
	if q.versions == nil {
		q.versions = map[string]string{}
	}
	q.versions[index] = version
	return nil
}

type mapSource map[string]string

func (m mapSource) Name() string { return "object" }
func (m mapSource) Keys(context.Context) ([]string, error) {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys, nil
}
func (m mapSource) Docs(_ context.Context, key string) ([]Doc, error) {
	body, ok := m[key]
	if !ok {
		return nil, nil
	}
	if body == "broken" {
		return nil, errors.New("cannot read")
	}
	return []Doc{{Object: key, Title: key, Body: body, Readers: []string{Everyone}}}, nil
}

func TestIndexerBuildsAndKeepsUp(t *testing.T) {
	ctx := context.Background()
	engine, queue := &fake{}, &memoryQueue{}
	source := mapSource{"a": "Erste Seite", "b": "Zweite Seite mit E-1042"}
	ix := NewIndexer(engine, queue, "t", quiet(), source)
	if err := ix.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	if len(queue.keys) != 2 || queue.versions["fake/t"] != "1" {
		t.Fatalf("a new index queues every key and records the version: %+v", queue)
	}
	if err := ix.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if len(engine.docs) != 2 {
		t.Fatalf("both pages are in the index: %v", engine.docs)
	}
	for _, d := range engine.docs {
		if d.Source != "object" || d.ID == "" || (d.Key == "b" && !slices.Contains(d.Codes, "e1042")) {
			t.Errorf("the indexer sets source, key, ID and codes: %+v", d)
		}
	}
	if err := ix.Prepare(ctx); err != nil || len(queue.keys) != 0 {
		t.Errorf("an index that is up to date is not built again: %v %v", queue.keys, err)
	}
	delete(source, "a")
	source["c"] = "broken"
	_ = queue.Enqueue(ctx, []Key{{"object", "a"}, {"object", "c"}, {"gone-source", "x"}})
	if err := ix.Drain(ctx); err == nil {
		t.Error("a key the source cannot read is an error")
	}
	if _, kept := engine.docs[DocID(Key{"object", "a"}, 0)]; kept {
		t.Error("a key without records leaves the index")
	}
	if !slices.Equal(queue.retried, []Key{{"object", "c"}}) {
		t.Errorf("only the failed key waits for another try: %v", queue.retried)
	}
	engine.fail = errors.New("down")
	_ = queue.Enqueue(ctx, []Key{{"object", "b"}})
	queue.retried = nil
	if err := ix.Drain(ctx); err == nil || !slices.Equal(queue.retried, []Key{{"object", "b"}}) {
		t.Errorf("when the engine fails, the keys go back into the queue: %v %v", queue.retried, err)
	}
}
