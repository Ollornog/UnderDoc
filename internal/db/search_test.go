// SPDX-License-Identifier: MIT

package db

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/search"
)

func claimAll(t *testing.T, db *DB) []string {
	t.Helper()
	keys, err := db.Claim(inMain(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, k := range keys {
		if k.Source != SourceObjects {
			t.Errorf("unexpected source %q", k.Source)
		}
		out = append(out, k.Key)
	}
	slices.Sort(out)
	return out
}

func TestChangesQueueForTheSearch(t *testing.T) {
	ctx := inMain()
	db := open(t)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	folder, _ := db.CreateFolder(ctx, anna, "Runbooks", team.ID)
	page, _ := db.CreatePage(ctx, anna, "Restart", folder.ID)
	claimAll(t, db)
	if got := claimAll(t, db); len(got) != 0 {
		t.Fatalf("a claimed key leaves the queue: %v", got)
	}
	block, err := db.AddBlock(ctx, page.ID, anna.ID, "", "Text")
	if err != nil {
		t.Fatal(err)
	}
	if got := claimAll(t, db); !slices.Contains(got, page.ID) {
		t.Errorf("a new block queues its page: %v", got)
	}
	if _, err := db.EditBlock(ctx, page.ID, block.ID, anna.ID, "Other text"); err != nil {
		t.Fatal(err)
	}
	if got := claimAll(t, db); !slices.Contains(got, page.ID) {
		t.Errorf("a changed block queues its page: %v", got)
	}
	// A block changed by any other way than the functions above, which also touch the page, still queues its page.
	if _, err := db.pool.Exec(ctx, "UPDATE blocks SET body = 'by hand' WHERE page = $1", page.ID); err != nil {
		t.Fatal(err)
	}
	if got := claimAll(t, db); !slices.Equal(got, []string{page.ID}) {
		t.Errorf("a block changed by hand queues its page: %v", got)
	}
	if _, err := db.Rename(ctx, team.ID, anna.ID, "Operations"); err != nil {
		t.Fatal(err)
	}
	want := []string{team.ID, folder.ID, page.ID}
	slices.Sort(want)
	if got := claimAll(t, db); !slices.Equal(got, want) {
		t.Errorf("a new title shows in the path of everything below: %v, want %v", got, want)
	}
	if err := db.Delete(ctx, folder.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if got := claimAll(t, db); !slices.Contains(got, folder.ID) || !slices.Contains(got, page.ID) {
		t.Errorf("the trash queues what goes into it: %v", got)
	}
}

func TestQueueRetryWaits(t *testing.T) {
	ctx := inMain()
	db := open(t)
	claimAll(t, db) // the private notes of the accounts
	key := search.Key{Source: "object", Key: "x"}
	if err := db.Retry(ctx, []search.Key{key}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if got := claimAll(t, db); len(got) != 0 {
		t.Errorf("a key put back for later is not due yet: %v", got)
	}
	if err := db.Enqueue(ctx, []search.Key{key}); err != nil {
		t.Fatal(err)
	}
	if got := claimAll(t, db); !slices.Equal(got, []string{"x"}) {
		t.Errorf("a change makes a waiting key due at once: %v", got)
	}
	if v, err := db.Version(ctx, "meilisearch/default"); err != nil || v != "" {
		t.Errorf("no version before the first build: %q %v", v, err)
	}
	if err := db.SetVersion(ctx, "meilisearch/default", "2"); err != nil {
		t.Fatal(err)
	}
	if v, _ := db.Version(ctx, "meilisearch/default"); v != "2" {
		t.Errorf("the version is kept: %q", v)
	}
}

func TestSearchDocsOfAPage(t *testing.T) {
	ctx := inMain()
	db := open(t)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	folder, _ := db.CreateFolder(ctx, anna, "Kasse", team.ID)
	page, _ := db.CreatePage(ctx, anna, "Kasse bedienen", folder.ID)
	if _, err := db.SetTags(ctx, page.ID, anna.ID, []string{"kasse"}); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, body := range []string{"Einleitung zur Kasse.", "## Kartenzahlung", "Karte vorhalten.", "Code: ::secret{id=\"s3cr3t\"}",
		"## Storno", "Bon verwerfen."} {
		after := ""
		if len(ids) > 0 {
			after = ids[len(ids)-1]
		}
		b, err := db.AddBlock(ctx, page.ID, anna.ID, after, body)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, b.ID)
	}
	docs, err := db.SearchSource().Docs(ctx, page.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 3 {
		t.Fatalf("one record per section: %d", len(docs))
	}
	card := docs[1]
	if card.Heading != "Kartenzahlung" || card.Anchor != ids[1] || card.Object != page.ID {
		t.Errorf("the section opens at its heading's block: %+v", card)
	}
	if card.Path != "Team › Kasse" || card.Collection != team.ID || card.CollectionName != "Team" || card.AuthorName != anna.Name {
		t.Errorf("place and author: %+v", card)
	}
	if !slices.Equal(card.Readers, []string{search.Everyone}) || !slices.Equal(card.Tags, []string{"kasse"}) {
		t.Errorf("readers and tags: %v %v", card.Readers, card.Tags)
	}
	for _, d := range docs {
		if strings.Contains(d.Body, "s3cr3t") || strings.Contains(d.Body, "secret") {
			t.Errorf("a password reference never reaches the index (ADR-7): %q", d.Body)
		}
	}
	if docs[0].Heading != "" || docs[0].Anchor != ids[0] {
		t.Errorf("the text before the first heading is a section of its own: %+v", docs[0])
	}
}

func TestSearchDocsKnowWhoReads(t *testing.T) {
	ctx := inMain()
	db := open(t)
	notes, _ := db.Collections(ctx, anna.ID)
	private, _ := db.CreatePage(ctx, anna, "Mine", notes[0].ID)
	docs, err := db.SearchSource().Docs(ctx, private.ID)
	if err != nil || len(docs) != 1 {
		t.Fatalf("one record: %v %v", docs, err)
	}
	if !slices.Equal(docs[0].Readers, []string{search.ReaderAccount(anna.ID)}) {
		t.Errorf("only the owner reads what lies in private notes: %v", docs[0].Readers)
	}
	if err := db.Delete(ctx, private.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if docs, err := db.SearchSource().Docs(ctx, private.ID); err != nil || len(docs) != 0 {
		t.Errorf("the trash is not searched: %v %v", docs, err)
	}
	if docs, err := db.SearchSource().Docs(ctx, "0000000000"); err != nil || len(docs) != 0 {
		t.Errorf("what does not exist has no records: %v %v", docs, err)
	}
	board, _, err := db.AddNote(ctx, anna, "Rückruf Firma Keller wegen E-1042")
	if err != nil {
		t.Fatal(err)
	}
	if docs, err := db.SearchSource().Docs(ctx, board.ID); err != nil || len(docs) != 1 || !strings.Contains(docs[0].Body, "E-1042") ||
		!slices.Equal(docs[0].Readers, []string{search.ReaderAccount(anna.ID)}) {
		t.Errorf("the cards of the quick notes are found, by their owner only: %+v %v", docs, err)
	}
	// What lies in the view Files has rights of its own (ADR-28): it is not searched through the rules of the documents.
	files, err := db.CreateFolder(ctx, anna, "Kartenzahlung Dateien", notes[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.pool.Exec(ctx, "UPDATE objects SET area = 'files', private = false WHERE id = $1", files.ID); err != nil {
		t.Fatal(err)
	}
	if docs, err := db.SearchSource().Docs(ctx, files.ID); err != nil || len(docs) != 0 {
		t.Errorf("the view Files is not indexed as documents: %v %v", docs, err)
	}
	if hits, _ := db.SearchTitles(ctx, "Kartenzahlung Dateien", anna.ID, nil, 10); len(hits) != 0 {
		t.Errorf("nor found by its title: %v", hits)
	}
	keys, err := db.SearchSource().Keys(ctx)
	if err != nil || slices.Contains(keys, private.ID) || slices.Contains(keys, files.ID) || !slices.Contains(keys, notes[0].ID) {
		t.Errorf("the keys for a rebuild leave out the trash: %v %v", keys, err)
	}
}

func TestSearchTitles(t *testing.T) {
	ctx := inMain()
	db := open(t)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	folder, _ := db.CreateFolder(ctx, anna, "Kasse", team.ID)
	exact, _ := db.CreatePage(ctx, anna, "Kartenzahlung", folder.ID)
	_, _ = db.CreatePage(ctx, anna, "Störung der Kartenzahlung", folder.ID)
	_, _ = db.CreatePage(ctx, anna, "Kartenzahlung_alt 100%", folder.ID)
	notes, _ := db.Collections(ctx, anna.ID)
	_, _ = db.CreatePage(ctx, anna, "Kartenzahlung privat", notes[0].ID)
	hits, err := db.SearchTitles(ctx, "kartenzahlung", bob.ID, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, h := range hits {
		got = append(got, h.Object.Title)
	}
	if len(got) != 3 || got[0] != "Kartenzahlung" || slices.Contains(got, "Kartenzahlung privat") {
		t.Errorf("the whole title first, nothing private of others: %v", got)
	}
	if hits[0].Object.ID != exact.ID || hits[0].Path != "Team › Kasse" || hits[0].Top.ID != team.ID {
		t.Errorf("a hit knows its place: %+v", hits[0])
	}
	if hits, _ := db.SearchTitles(ctx, "100%", bob.ID, nil, 10); len(hits) != 1 {
		t.Errorf("percent and underscore are letters here, not patterns: %d", len(hits))
	}
	if hits, _ := db.SearchTitles(ctx, "_", bob.ID, nil, 10); len(hits) != 1 {
		t.Errorf("_ matches only itself: %d", len(hits))
	}
	if _, err := db.SetTags(ctx, exact.ID, anna.ID, []string{"kasse", "karte"}); err != nil {
		t.Fatal(err)
	}
	if hits, _ := db.SearchTitles(ctx, "", bob.ID, []string{"kasse", "karte"}, 10); len(hits) != 1 || hits[0].Object.ID != exact.ID {
		t.Errorf("without text, the tags list what carries all of them: %v", hits)
	}
	if hits, _ := db.SearchTitles(ctx, "Kartenzahlung", bob.ID, []string{"kasse", "rack"}, 10); len(hits) != 0 {
		t.Errorf("every tag must be there: %v", hits)
	}
}
