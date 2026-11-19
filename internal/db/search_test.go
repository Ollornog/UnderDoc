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
	team, _ := db.createShared(ctx, anna, "Team")
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
	team, _ := db.createShared(ctx, anna, "Team")
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
	if !slices.Equal(card.Readers, []string{search.ReaderAccount(anna.ID), search.Everyone}) || !slices.Equal(card.Tags, []string{"kasse"}) {
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
	// The folders of the view Files are places, not content: only its files are found, by their text (below).
	files, err := db.CreateFolder(ctx, anna, "Kartenzahlung Dateien", notes[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.pool.Exec(ctx, "UPDATE objects SET area = 'files', private = false WHERE id = $1", files.ID); err != nil {
		t.Fatal(err)
	}
	if docs, err := db.SearchSource().Docs(ctx, files.ID); err != nil || len(docs) != 0 {
		t.Errorf("a folder of the view Files has no records: %v %v", docs, err)
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
	team, _ := db.createShared(ctx, anna, "Team")
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

func TestFilesAreFoundByTheirText(t *testing.T) {
	ctx := inMain()
	db := open(t)
	if err := db.EnsureRoots(ctx, anna, false, ""); err != nil {
		t.Fatal(err)
	}
	mine := rootOfRole(t, db, anna.ID, RootUser)
	claimAll(t, db)
	done := upload(t, db, anna.ID, UploadTarget{Parent: mine.ID}, "Wartung.pdf", 10, sha(41))
	if got := claimAll(t, db); !slices.Contains(got, done.File.ID) {
		t.Errorf("a new file queues itself: %v", got)
	}
	keys, err := db.SearchSource().Keys(ctx)
	if err != nil || !slices.Contains(keys, done.File.ID) || slices.Contains(keys, mine.ID) {
		t.Errorf("a rebuild takes the files, not their roots: %v %v", keys, err)
	}
	docs, err := db.SearchSource().Docs(ctx, done.File.ID)
	if err != nil || len(docs) != 1 || docs[0].Type != search.TypeFile || docs[0].Title != "Wartung.pdf" || docs[0].Body != "" {
		t.Fatalf("before its text is read a file is found by its name: %+v %v", docs, err)
	}
	if !slices.Equal(docs[0].Readers, []string{search.ReaderAccount(anna.ID)}) || docs[0].Collection != mine.ID {
		t.Errorf("the folder of a user is read by that user only: %+v", docs[0])
	}

	// The text read on the server reaches the index, and the file is queued when it arrives.
	if err := db.QueueText(ctx, sha(41), "application/pdf", anna.ID, "server"); err != nil {
		t.Fatal(err)
	}
	job, err := db.ClaimTextJob(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("Pumpe ", 1600)
	if err := db.FinishTextJob(ctx, job, "server", "Fehler E-2207 an der Schranke\n\nDanach "+long, nil); err != nil {
		t.Fatal(err)
	}
	if got := claimAll(t, db); !slices.Equal(got, []string{done.File.ID}) {
		t.Errorf("a text read later queues the file: %v", got)
	}
	docs, err = db.SearchSource().Docs(ctx, done.File.ID)
	if err != nil || len(docs) != 2 || !strings.Contains(docs[0].Body, "E-2207") || docs[0].Origin != "server" || docs[1].Rank != 1 {
		t.Fatalf("the text is found, split where it is long, and says where it came from: %d %v", len(docs), err)
	}
	// What the content says itself comes before what was seen in pixels.
	if err := db.SaveFileText(ctx, done.File.ID, anna.ID, "ocr", "Fehler E-9999 erkannt"); err != nil {
		t.Fatal(err)
	}
	if docs, _ := db.SearchSource().Docs(ctx, done.File.ID); strings.Contains(docs[0].Body, "E-9999") {
		t.Errorf("recognised text does not push out the text of the content: %q", docs[0].Body)
	}

	// A new version is the file as it is now: its text, not the old one.
	claimAll(t, db)
	upload(t, db, anna.ID, UploadTarget{File: done.File.ID}, "Wartung.pdf", 11, sha(42))
	if got := claimAll(t, db); !slices.Contains(got, done.File.ID) {
		t.Errorf("a new version queues the file: %v", got)
	}
	if docs, _ := db.SearchSource().Docs(ctx, done.File.ID); len(docs) != 1 || docs[0].Body != "" {
		t.Errorf("the text of an old version is not found: %+v", docs)
	}
	// A version made any other way than by an upload, which also touches the file, still queues it.
	if _, err := db.pool.Exec(ctx, `INSERT INTO file_versions (file, number, blob, size, name, content_type, author)
		SELECT file, number + 1, $2, size, name, content_type, author FROM file_versions WHERE file = $1 ORDER BY number DESC LIMIT 1`,
		done.File.ID, sha(41)); err != nil {
		t.Fatal(err)
	}
	if got := claimAll(t, db); !slices.Equal(got, []string{done.File.ID}) {
		t.Errorf("a version made by hand queues its file: %v", got)
	}
	if docs, _ := db.SearchSource().Docs(ctx, done.File.ID); !strings.Contains(docs[0].Body, "E-2207") {
		t.Errorf("back on the first content, its text is found again: %+v", docs)
	}
	if err := db.Delete(ctx, done.File.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if docs, err := db.SearchSource().Docs(ctx, done.File.ID); err != nil || len(docs) != 0 {
		t.Errorf("a file in the trash is not found: %v %v", docs, err)
	}

	// An attachment is found by its text and opens its page at the block that shows it; bob reads the page, so he finds it.
	team, _ := db.createShared(ctx, anna, "Team")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	db.AddBlock(ctx, page.ID, anna.ID, "", "Vorne")
	scan := upload(t, db, anna.ID, UploadTarget{Page: page.ID}, "scan.png", 12, sha(43))
	block, err := db.AddBlock(ctx, page.ID, anna.ID, "", "![scan]("+AttachmentPath(page.ID, scan.Attachment)+")")
	if err != nil {
		t.Fatal(err)
	}
	claimAll(t, db)
	if err := db.QueueText(ctx, sha(43), "image/png", anna.ID, "server"); err != nil {
		t.Fatal(err)
	}
	job, _ = db.ClaimTextJob(ctx, time.Minute)
	if err := db.FinishTextJob(ctx, job, "ocr", "Lieferschein 4711", nil); err != nil {
		t.Fatal(err)
	}
	if got := claimAll(t, db); !slices.Equal(got, []string{page.ID}) {
		t.Errorf("the text of an attachment queues its page: %v", got)
	}
	docs, err = db.SearchSource().Docs(ctx, page.ID)
	if err != nil {
		t.Fatal(err)
	}
	last := docs[len(docs)-1]
	if last.Type != search.TypeFile || last.Heading != "scan.png" || last.Anchor != block.ID || last.Object != page.ID ||
		!strings.Contains(last.Body, "4711") || last.Origin != "ocr" || !slices.Equal(last.Readers, []string{search.ReaderAccount(anna.ID), search.Everyone}) {
		t.Errorf("the attachment is a record of its page: %+v", last)
	}
	if docs[0].Type != search.TypePage {
		t.Errorf("the page itself comes first: %+v", docs[0])
	}
}

// TestSearchPathHidesLevelsNotEveryReaderSees: a page someone reads through a grant of its own is found without the
// titles of the folders above it that they may not see; those show as "…" (ADR-29). Where every reader sees the levels
// above, they are named.
func TestSearchPathHidesLevelsNotEveryReaderSees(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.SeeAccount(ctx, carl); err != nil {
		t.Fatal(err)
	}
	board, _ := db.createShared(ctx, anna, "Board")
	if err := db.SetGrant(ctx, board.ID, Actor{ID: anna.ID}, ToTenant, "", RoleNone); err != nil {
		t.Fatal(err)
	}
	folder, _ := db.CreateFolder(ctx, anna, "Layoffs 2027", board.ID)
	page, _ := db.CreatePage(ctx, anna, "Agenda", folder.ID)
	if err := db.SetGrant(ctx, page.ID, Actor{ID: anna.ID}, ToPerson, carl.ID, RoleRead); err != nil {
		t.Fatal(err)
	}
	notes, _ := db.CreatePage(ctx, anna, "Notes", folder.ID)
	doc := func(id string) search.Doc {
		t.Helper()
		docs, err := objectSource{db: db}.Docs(ctx, id)
		if err != nil || len(docs) == 0 {
			t.Fatalf("docs: %v %v", docs, err)
		}
		return docs[0]
	}
	if d := doc(page.ID); !slices.Contains(d.Readers, "account:"+carl.ID) || d.Path != "…" || d.CollectionName != "…" {
		t.Errorf("carl finds the page with the titles above it he may not see: path %q, collection %q, readers %v", d.Path, d.CollectionName, d.Readers)
	}
	if d := doc(notes.ID); d.Path != "Board › Layoffs 2027" || d.CollectionName != "Board" {
		t.Errorf("anna alone reads the notes and every level above: path %q, collection %q", d.Path, d.CollectionName)
	}
}
