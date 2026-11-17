// SPDX-License-Identifier: MIT

package searchtest

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v4"

	"github.com/Ollornog/UnderDoc/internal/markdown"
	"github.com/Ollornog/UnderDoc/internal/search"
)

// The corpus: about 200 German pages of a made-up retail company (error catalogue, runbooks, handbook, minutes, IT
// documentation) and the questions asked of them, each with the page that should come first.
//
//go:embed corpus
var corpus embed.FS

// Question is one query of the corpus with the page, and for some the section, that should come first.
type Question struct {
	Q       string `json:"q"`
	Page    string `json:"page"`
	Heading string `json:"heading"`
	// Kind groups the questions: code, code-variant and code-absent are hard gates, the others are measured.
	Kind string `json:"kind"`
}

// Hard gates: every code has to be the first hit, and a code that is not in the corpus finds nothing at all.
var hardKinds = []string{"code", "code-variant", "code-absent"}

// Corpus returns the pages of the corpus as records, one list per page, and the questions.
func Corpus(t testing.TB) (map[string][]search.Doc, []Question) {
	t.Helper()
	pages := map[string][]search.Doc{}
	err := fs.WalkDir(corpus, "corpus", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		content, err := corpus.ReadFile(path)
		if err != nil {
			return err
		}
		key := strings.TrimSuffix(strings.TrimPrefix(path, "corpus/"), ".md")
		docs, err := pageDocs(key, content)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		pages[key] = docs
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := corpus.ReadFile("corpus/queries.json")
	if err != nil {
		t.Fatal(err)
	}
	var questions []Question
	if err := json.Unmarshal(raw, &questions); err != nil {
		t.Fatal(err)
	}
	return pages, questions
}

func pageDocs(key string, content []byte) ([]search.Doc, error) {
	front, body, found := markdown.SplitFrontMatter(content)
	if !found {
		return nil, fmt.Errorf("no front matter")
	}
	var meta struct {
		Title      string   `yaml:"title"`
		Collection string   `yaml:"collection"`
		Author     string   `yaml:"author"`
		Tags       []string `yaml:"tags"`
		Changed    string   `yaml:"changed"`
	}
	if err := yaml.Unmarshal(front, &meta); err != nil {
		return nil, err
	}
	changed, err := time.Parse(time.DateOnly, meta.Changed)
	if err != nil {
		return nil, err
	}
	base := search.Doc{Object: key, Type: search.TypePage, Title: meta.Title, Path: meta.Collection,
		Collection: strings.Split(key, "/")[0], CollectionName: meta.Collection, Author: meta.Author, AuthorName: meta.Author,
		Tags: meta.Tags, Changed: changed.Unix(), Readers: []string{search.Everyone}}
	return search.SectionDocs(base, search.Sections(search.PageParts(body, meta.Title))), nil
}

// LoadCorpus puts the whole corpus into the index of a tenant, as one batch the way the indexer sends it.
func LoadCorpus(t testing.TB, ctx context.Context, engine search.Engine, tenant string, pages map[string][]search.Doc) {
	t.Helper()
	var keys []search.Key
	var docs []search.Doc
	for page, list := range pages {
		key := search.Key{Source: "corpus", Key: page}
		keys = append(keys, key)
		for i := range list {
			doc := list[i]
			doc.Source, doc.Key, doc.ID = key.Source, key.Key, search.DocID(key, i)
			doc.Prepare()
			docs = append(docs, doc)
		}
	}
	if err := engine.Replace(ctx, tenant, keys, docs); err != nil {
		t.Fatal(err)
	}
}

// Score is the measurement of one kind of question.
type Score struct {
	Kind       string
	Questions  int
	At1, At5   int
	Reciprocal float64
	// Sections counts questions with a heading whose first hit of the right page opened at that heading.
	Sections, WithHeading int
}

// MRR is the mean reciprocal rank.
func (s Score) MRR() float64 {
	if s.Questions == 0 {
		return 0
	}
	return s.Reciprocal / float64(s.Questions)
}

// Measure asks every question of the corpus, fails the test for every code that is not the first hit, and returns the
// scores by kind and in total.
func Measure(t *testing.T, ctx context.Context, service *search.Service, questions []Question) []Score {
	t.Helper()
	byKind := map[string]*Score{}
	var order []string
	total := &Score{Kind: "all"}
	for _, question := range questions {
		score, known := byKind[question.Kind]
		if !known {
			score = &Score{Kind: question.Kind}
			byKind[question.Kind] = score
			order = append(order, question.Kind)
		}
		result, err := service.Search(ctx, search.Request{Text: question.Q, Account: "reader", Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if result.Notice != "" {
			t.Fatalf("%q: the engine did not answer: %s", question.Q, result.Notice)
		}
		rank, hit := 0, search.Hit{}
		for i, h := range result.Hits {
			if h.Object == question.Page {
				rank, hit = i+1, h
				break
			}
		}
		hard := slices.Contains(hardKinds, question.Kind)
		for _, s := range []*Score{score, total} {
			s.Questions++
			if question.Kind == "code-absent" {
				if len(result.Hits) == 0 {
					s.At1++
					s.At5++
					s.Reciprocal++
				}
				continue
			}
			if rank == 1 {
				s.At1++
			}
			if rank >= 1 && rank <= 5 {
				s.At5++
			}
			if rank > 0 {
				s.Reciprocal += 1 / float64(rank)
			}
			if question.Heading != "" {
				s.WithHeading++
				if rank > 0 && hit.Heading == question.Heading {
					s.Sections++
				}
			}
		}
		switch {
		case question.Kind == "code-absent" && len(result.Hits) > 0:
			t.Errorf("hard gate: %q is in no page, but found %s", question.Q, objectsOf(result.Hits))
		case hard && question.Kind != "code-absent" && rank != 1:
			t.Errorf("hard gate: %q must find %s first, found %s", question.Q, question.Page, objectsOf(result.Hits))
		case !hard && rank != 1 && testing.Verbose():
			t.Logf("%-10s %q: %s at %d, first %s", question.Kind, question.Q, question.Page, rank, objectsOf(result.Hits))
		}
	}
	scores := make([]Score, 0, len(order)+1)
	for _, kind := range order {
		scores = append(scores, *byKind[kind])
	}
	return append(scores, *total)
}

func objectsOf(hits []search.Hit) string {
	var out []string
	for i, h := range hits {
		if i == 5 {
			out = append(out, "…")
			break
		}
		out = append(out, h.Object)
	}
	return "[" + strings.Join(out, " ") + "]"
}

// Table writes scores as a Markdown table.
func Table(engine string, scores []Score) string {
	var b strings.Builder
	fmt.Fprintf(&b, "| %s | questions | hit@1 | hit@5 | MRR | section |\n|---|---|---|---|---|---|\n", engine)
	for _, s := range scores {
		section := "–"
		if s.WithHeading > 0 {
			section = fmt.Sprintf("%d/%d", s.Sections, s.WithHeading)
		}
		fmt.Fprintf(&b, "| %s | %d | %.0f %% | %.0f %% | %.3f | %s |\n", s.Kind, s.Questions,
			100*float64(s.At1)/float64(s.Questions), 100*float64(s.At5)/float64(s.Questions), s.MRR(), section)
	}
	return b.String()
}

// Report logs the table and appends it to the file named by UNDERDOC_SEARCH_REPORT, if set.
func Report(t *testing.T, engine string, scores []Score) {
	t.Helper()
	table := Table(engine, scores)
	t.Log("\n" + table)
	if path := os.Getenv(EnvReport); path != "" {
		file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = file.Close() }()
		if _, err := file.WriteString(table + "\n"); err != nil {
			t.Fatal(err)
		}
	}
}

// RunCorpus loads the corpus into an engine, measures it, and holds the result to the hard gates and to the floor of
// the measured kinds, so that ranking cannot get worse unnoticed.
func RunCorpus(t *testing.T, newEngine NewEngine, floor map[string]float64) {
	engine := newEngine(Prefix())
	ctx := Context(t)
	const tenant = "corpus"
	if _, err := engine.Prepare(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Drop(context.Background(), tenant) })
	pages, questions := Corpus(t)
	if len(pages) < 200 {
		t.Fatalf("the corpus has %d pages, at least 200 were promised", len(pages))
	}
	LoadCorpus(t, ctx, engine, tenant, pages)
	scores := Measure(t, ctx, search.NewService(engine, nil, tenant, Quiet()), questions)
	Report(t, engine.Name(), scores)
	for _, s := range scores {
		if want, ok := floor[s.Kind]; ok && s.MRR() < want {
			t.Errorf("%s: MRR %.3f fell below %.3f", s.Kind, s.MRR(), want)
		}
	}
}
