// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Ollornog/UnderDoc/internal/files"
	"github.com/Ollornog/UnderDoc/internal/search"
	"github.com/Ollornog/UnderDoc/internal/sheet"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// SourceObjects is the name of the source of collections, folders and pages in the search queue. The triggers of
// migration 019 queue under this name.
const SourceObjects = "object"

// --- The queue (search.Queue)

// Claim implements search.Queue: the keys leave the queue, so two indexers never take the same.
func (db *DB) Claim(ctx context.Context, n int) ([]search.Key, error) {
	rows, err := db.pool.Query(ctx, `
		DELETE FROM search_queue WHERE (source, key) IN (
			SELECT source, key FROM search_queue WHERE due <= now() ORDER BY due LIMIT $1 FOR UPDATE SKIP LOCKED)
		RETURNING source, key`, n)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (search.Key, error) {
		var k search.Key
		err := row.Scan(&k.Source, &k.Key)
		return k, err
	})
}

// Retry implements search.Queue.
func (db *DB) Retry(ctx context.Context, keys []search.Key, after time.Duration) error {
	sources, names := split(keys)
	_, err := db.pool.Exec(ctx, `
		INSERT INTO search_queue (source, key, due) SELECT s, k, now() + $3 * interval '1 millisecond' FROM unnest($1::text[], $2::text[]) AS t(s, k)
		ON CONFLICT (tenant, source, key) DO NOTHING`, sources, names, after.Milliseconds())
	return err
}

// Enqueue implements search.Queue.
func (db *DB) Enqueue(ctx context.Context, keys []search.Key) error {
	if len(keys) == 0 {
		return nil
	}
	sources, names := split(keys)
	_, err := db.pool.Exec(ctx, `
		INSERT INTO search_queue (source, key) SELECT s, k FROM unnest($1::text[], $2::text[]) AS t(s, k)
		ON CONFLICT (tenant, source, key) DO UPDATE SET due = least(search_queue.due, now())`, sources, names)
	return err
}

func split(keys []search.Key) (sources, names []string) {
	for _, k := range keys {
		sources = append(sources, k.Source)
		names = append(names, k.Key)
	}
	return sources, names
}

// Version implements search.Queue.
func (db *DB) Version(ctx context.Context, index string) (string, error) {
	var version string
	err := db.pool.QueryRow(ctx, "SELECT version FROM search_indexes WHERE name = $1", index).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return version, err
}

// SetVersion implements search.Queue.
func (db *DB) SetVersion(ctx context.Context, index, version string) error {
	_, err := db.pool.Exec(ctx, `
		INSERT INTO search_indexes (name, version) VALUES ($1, $2)
		ON CONFLICT (tenant, name) DO UPDATE SET version = EXCLUDED.version, built = now()`, index, version)
	return err
}

// Queued returns how many keys wait in the queue, for the health of the search.
func (db *DB) Queued(ctx context.Context) (int, error) {
	var n int
	err := db.pool.QueryRow(ctx, "SELECT count(*) FROM search_queue").Scan(&n)
	return n, err
}

// --- The source of objects (search.Source)

// SearchSource returns the collections, folders and pages of the database as a source of the search.
func (db *DB) SearchSource() search.Source { return objectSource{db} }

type objectSource struct{ db *DB }

func (objectSource) Name() string { return SourceObjects }

// Version is raised whenever the records find more than before, so an index built before is filled again: 2 adds the text
// of files and of the attachments of pages (M-2), 3 readers as persons, groups and the tenant (ADR-29), 4 paths without
// the names of levels not every reader sees (ADR-29), 5 the cells of spreadsheets by sheet and range (ADR-31).
func (objectSource) Version() string { return "5" }

// Keys lists every object there is to find, outside the trash: the documents, and the files of the view Files.
func (s objectSource) Keys(ctx context.Context) ([]string, error) {
	rows, err := s.db.pool.Query(ctx, `SELECT id FROM objects WHERE deleted_at IS NULL
		AND ((area = 'docs' AND kind IN ('collection', 'folder', 'page', 'notes')) OR (area = 'files' AND kind = 'file')) ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// Docs returns the records of one object: one per section of a page, one for a folder or collection. An object that is
// gone, in the trash or a divider has none, so its records leave the index.
func (s objectSource) Docs(ctx context.Context, objectID string) ([]search.Doc, error) {
	// The object and everything above it, nearest first.
	rows, err := s.db.pool.Query(ctx, `
		WITH RECURSIVE up AS (
			SELECT id, parent, kind, title, owner, private, deleted_at, changed, tags, area, 0 AS depth FROM objects WHERE id = $1
			UNION ALL SELECT p.id, p.parent, p.kind, p.title, p.owner, p.private, p.deleted_at, p.changed, p.tags, p.area, up.depth + 1
			FROM objects p JOIN up ON p.id = up.parent
		)
		SELECT up.id, up.kind, up.title, up.owner, coalesce(a.name, up.owner), up.private, up.deleted_at IS NOT NULL, up.changed, up.tags, up.area
		FROM up LEFT JOIN accounts a ON a.id = up.owner ORDER BY depth`, objectID)
	if err != nil {
		return nil, err
	}
	type level struct {
		id, kind, title, owner, ownerName, area string
		private, deleted                        bool
		changed                                 time.Time
		tags                                    []string
	}
	chain, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (level, error) {
		var l level
		err := row.Scan(&l.id, &l.kind, &l.title, &l.owner, &l.ownerName, &l.private, &l.deleted, &l.changed, &l.tags, &l.area)
		return l, err
	})
	if err != nil || len(chain) == 0 {
		return nil, err
	}
	self := chain[0]
	// Of the view Files only files are found, by their text; its folders and roots are places, not content. They follow
	// the same rule of who may read them as the documents (ADR-20) until the rights of M-5 come.
	if self.deleted || IsDivider(self.kind) || (self.area != "docs" && self.kind != KindFile) {
		return nil, nil
	}
	// Who may read it: the tokens of the persons, groups and the tenant that hold a role on it (ADR-29), the same rule
	// as the condition visible.
	var readers []string
	if err := s.db.pool.QueryRow(ctx, "SELECT object_readers($1)", objectID).Scan(&readers); err != nil {
		return nil, err
	}
	readers = nonNil(readers)
	// The path names only the levels above that everyone who finds the record may see; the others show as "…", once for
	// each run of them (ADR-29). A page shared on its own does not give away the folders it lies in.
	shown := make([]bool, len(chain))
	for i := 1; i < len(chain); i++ {
		var above []string
		if err := s.db.pool.QueryRow(ctx, "SELECT object_readers($1)", chain[i].id).Scan(&above); err != nil {
			return nil, err
		}
		shown[i] = coversReaders(above, readers)
	}
	top := chain[len(chain)-1]
	topName := top.title
	if len(chain) > 1 && !shown[len(chain)-1] {
		topName = hiddenLevel
	}
	var path []string
	for i := len(chain) - 1; i > 0; i-- {
		switch {
		case shown[i]:
			path = append(path, chain[i].title)
		case len(path) == 0 || path[len(path)-1] != hiddenLevel:
			path = append(path, hiddenLevel)
		}
	}
	base := search.Doc{Object: self.id, Type: self.kind, Title: self.title, Tags: self.tags, Path: strings.Join(path, " › "),
		Collection: top.id, CollectionName: topName, Author: self.owner, AuthorName: self.ownerName,
		Changed: self.changed.Unix(), Readers: readers}
	if self.kind == KindFile {
		base.Type = search.TypeFile
		var blob, contentType, name string
		err := s.db.pool.QueryRow(ctx, `SELECT blob, content_type, name FROM file_versions WHERE file = $1 ORDER BY number DESC LIMIT 1`,
			objectID).Scan(&blob, &contentType, &name)
		if errors.Is(err, pgx.ErrNoRows) {
			return []search.Doc{base}, nil
		}
		if err != nil {
			return nil, err
		}
		if docs, ok := s.sheetDocs(ctx, base, blob, contentType, name); ok {
			return docs, nil
		}
		text, origin, err := s.bestText(ctx, blob)
		if err != nil {
			return nil, err
		}
		base.Origin = origin
		return search.SectionDocs(base, search.Sections(search.TextParts(text, ""))), nil
	}
	// A page, and the board of quick notes, whose cards are its blocks (M-8), are found by their text.
	if self.kind != KindPage && self.kind != KindNotes {
		return []search.Doc{base}, nil
	}
	blockRows, err := s.db.pool.Query(ctx, `SELECT id, body FROM blocks WHERE page = $1 ORDER BY position`, objectID)
	if err != nil {
		return nil, err
	}
	blocks, err := pgx.CollectRows(blockRows, func(row pgx.CollectableRow) (search.Block, error) {
		var b search.Block
		err := row.Scan(&b.ID, &b.Body)
		return b, err
	})
	if err != nil {
		return nil, err
	}
	docs := search.SectionDocs(base, search.Sections(search.BlockParts(blocks)))
	if self.kind != KindPage {
		return docs, nil
	}
	// The attachments of a page, such as a pasted scan, are found by their text too, and open the page. They come after
	// the sections of the page, each under its own name.
	attachments, err := s.db.pool.Query(ctx, `SELECT id, name, blob FROM attachments WHERE page = $1 ORDER BY at, id`, objectID)
	if err != nil {
		return nil, err
	}
	type attachment struct{ id, name, blob string }
	list, err := pgx.CollectRows(attachments, func(row pgx.CollectableRow) (attachment, error) {
		var a attachment
		err := row.Scan(&a.id, &a.name, &a.blob)
		return a, err
	})
	if err != nil {
		return nil, err
	}
	for _, a := range list {
		text, origin, err := s.bestText(ctx, a.blob)
		if err != nil {
			return nil, err
		}
		if text == "" {
			continue
		}
		// A hit opens the block that shows the attachment, else the top of the page.
		var anchor string
		err = s.db.pool.QueryRow(ctx, `SELECT id FROM blocks WHERE page = $1 AND strpos(body, $2) > 0 ORDER BY position LIMIT 1`,
			objectID, AttachmentPath(objectID, a.id)).Scan(&anchor)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		file := base
		file.Type, file.Origin = search.TypeFile, origin
		for _, section := range search.Sections(search.TextParts(text, anchor)) {
			file.Heading, file.Anchor, file.Body, file.Rank = a.name, section.Anchor, section.Text, len(docs)
			docs = append(docs, file)
		}
	}
	return docs, nil
}

// sheetDocs returns the records of a spreadsheet the reader reads (ADR-31): the cells of its visible sheets, a record for
// each run of rows, headed by its sheet; a hit opens the table view at <sheet>!<range>. It reports false for a file the
// search finds by its text: no reader set, another type, or a file that cannot be read as tables.
func (s objectSource) sheetDocs(ctx context.Context, base search.Doc, blob, contentType, name string) ([]search.Doc, bool) {
	if s.db.sheets == nil || !sheet.Readable(contentType) {
		return nil, false
	}
	t, _ := tenant.From(ctx)
	wb, err := s.db.sheets.Workbook(ctx, t, blob, contentType, name)
	if err != nil {
		return nil, false
	}
	base.Origin = files.TextServer
	sections := wb.SearchText(search.MaxSectionWords, files.MaxText)
	if len(sections) == 0 {
		return []search.Doc{base}, true
	}
	docs := make([]search.Doc, len(sections))
	for i, section := range sections {
		doc := base
		doc.Anchor, doc.Heading, doc.Body, doc.Rank = section.Sheet+"!"+section.Range, section.Sheet, section.Text, i
		docs[i] = doc
	}
	return docs, true
}

// bestText returns the text of a blob the search takes, and where it came from: what the content says itself (server,
// browser) before what was recognised in pixels (ai, ocr), as FileTexts orders them. None is "".
func (s objectSource) bestText(ctx context.Context, blob string) (string, string, error) {
	var text, origin string
	err := s.db.pool.QueryRow(ctx, `SELECT text, source FROM file_texts WHERE blob = $1 AND text <> ''
		ORDER BY CASE source WHEN 'server' THEN 0 WHEN 'browser' THEN 1 WHEN 'ai' THEN 2 ELSE 3 END LIMIT 1`, blob).Scan(&text, &origin)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	return text, origin, err
}

// --- The fallback without a search service

// TitleHit is an object whose title fits a search, with the titles above it.
type TitleHit struct {
	Object Object
	Path   string
	Top    Object
}

// SearchTitles finds the collections, folders and pages the viewer may see whose title contains the text and that carry
// every one of the tags, the closest first: the whole title, then a title that starts with it, then the rest, the newer
// before the older. Without text, it lists what carries the tags.
func (db *DB) SearchTitles(ctx context.Context, text, viewer string, tags []string, limit int) ([]TitleHit, error) {
	text = strings.TrimSpace(text)
	if text == "" && len(tags) == 0 {
		return nil, nil
	}
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(text)
	found, err := db.list(ctx, `SELECT `+columns+` FROM objects o
		WHERE o.title ILIKE $2 AND o.tags @> $6 AND o.kind IN ('collection', 'folder', 'page', 'notes') AND o.area = 'docs' AND `+alive+` AND `+visible+`
		ORDER BY lower(o.title) = lower($3) DESC, o.title ILIKE $4 DESC, o.changed DESC, o.id LIMIT $5`,
		viewer, "%"+escaped+"%", text, escaped+"%", limit, nonNil(tags))
	if err != nil {
		return nil, err
	}
	hits := make([]TitleHit, 0, len(found))
	for _, o := range found {
		hit := TitleHit{Object: o, Top: o}
		var names []string
		for parent := o.Parent; parent != ""; {
			above, err := db.Object(ctx, parent, viewer)
			if err != nil {
				break
			}
			names = append([]string{above.Title}, names...)
			hit.Top, parent = above, above.Parent
		}
		hit.Path = strings.Join(names, " › ")
		hits = append(hits, hit)
	}
	return hits, nil
}

// hiddenLevel stands in a path for levels above a record that not everyone who finds it may see.
const hiddenLevel = "…"

// coversReaders tells whether every reader of a record also reads a level above it: the tenant reads it, or each token of
// the record is a reader there too.
func coversReaders(above, record []string) bool {
	if slices.Contains(above, "all") {
		return true
	}
	for _, r := range record {
		if !slices.Contains(above, r) {
			return false
		}
	}
	return true
}
