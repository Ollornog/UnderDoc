// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// The text of files for the search (ADR-24): three ways, each switched on by the hoster, the tenant and the user in turn.
// The text belongs to the content of a version, a blob, and each way keeps its own, so the search knows where it came from.

// Ways says which ways of reading the text of files are on.
type Ways struct {
	Browser, Server, AI bool
}

// WaySettings is what the tenant or a user set; nil takes it from above.
type WaySettings struct {
	Browser, Server, AI *bool
}

// Sources of a text (files.TextBrowser and the others).
var textSources = map[string]bool{"browser": true, "server": true, "ocr": true, "ai": true}

// MaxFileText is the most text kept of one file, in bytes.
const MaxFileText = 1 << 20

// ErrText is a text that is not valid UTF-8, or a source that does not exist. A text that is too long is cut.
var ErrText = errors.New("a text is valid UTF-8 of at most 1 MiB from a known source")

// TextSettings returns what the tenant set ("" for account) or a user set.
func (db *DB) TextSettings(ctx context.Context, account string) (WaySettings, error) {
	var s WaySettings
	err := db.pool.QueryRow(ctx, "SELECT browser, server, ai FROM text_settings WHERE account = $1", account).Scan(&s.Browser, &s.Server, &s.AI)
	if errors.Is(err, pgx.ErrNoRows) {
		return WaySettings{}, nil
	}
	return s, err
}

// SetTextSettings keeps what the tenant ("" for account) or a user switched on or off.
func (db *DB) SetTextSettings(ctx context.Context, account string, s WaySettings) error {
	_, err := db.pool.Exec(ctx, `INSERT INTO text_settings (account, browser, server, ai) VALUES ($1, $2, $3, $4)
		ON CONFLICT (tenant, account) DO UPDATE SET browser = EXCLUDED.browser, server = EXCLUDED.server, ai = EXCLUDED.ai`, account, s.Browser, s.Server, s.AI)
	return err
}

// TextWays is what holds for a user: a way is on only where the hoster allows it, and the tenant and the user, where they
// set it, leave it on. Unset means on.
func (db *DB) TextWays(ctx context.Context, hoster Ways, user string) (Ways, error) {
	tenant, err := db.TextSettings(ctx, "")
	if err != nil {
		return Ways{}, err
	}
	own, err := db.TextSettings(ctx, user)
	if err != nil {
		return Ways{}, err
	}
	on := func(allowed bool, layers ...*bool) bool {
		for _, l := range layers {
			if l != nil && !*l {
				return false
			}
		}
		return allowed
	}
	return Ways{Browser: on(hoster.Browser, tenant.Browser, own.Browser), Server: on(hoster.Server, tenant.Server, own.Server),
		AI: on(hoster.AI, tenant.AI, own.AI)}, nil
}

// QueueText puts work for the server on the queue for a blob: one job per way, once.
func (db *DB) QueueText(ctx context.Context, sha, contentType, account string, ways ...string) error {
	for _, way := range ways {
		if _, err := db.pool.Exec(ctx, `INSERT INTO text_jobs (blob, way, account, content_type) VALUES ($1, $2, $3, $4)
			ON CONFLICT (tenant, blob, way) DO NOTHING`, sha, way, account, contentType); err != nil {
			return err
		}
	}
	return nil
}

// TextJob is work for the server.
type TextJob struct {
	Blob, Way, Account, ContentType string
	Tries                           int
}

// MaxTextTries is how often a job is tried before it stays failed.
const MaxTextTries = 3

// ClaimTextJob takes the next job that is due and marks it as tried, so another worker passes it by; a job whose worker
// stops is tried again later. None is ErrNotFound.
func (db *DB) ClaimTextJob(ctx context.Context, hold time.Duration) (TextJob, error) {
	var j TextJob
	err := db.pool.QueryRow(ctx, `UPDATE text_jobs SET tries = tries + 1, due = now() + $2 * interval '1 second'
		WHERE (blob, way) = (SELECT blob, way FROM text_jobs WHERE due <= now() AND tries < $1 ORDER BY due LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING blob, way, account, content_type, tries`, MaxTextTries, hold.Seconds()).Scan(&j.Blob, &j.Way, &j.Account, &j.ContentType, &j.Tries)
	if errors.Is(err, pgx.ErrNoRows) {
		return TextJob{}, ErrNotFound
	}
	return j, err
}

// FinishTextJob ends a job: with its text kept, or failed with why; a failure is tried again later until MaxTextTries.
func (db *DB) FinishTextJob(ctx context.Context, j TextJob, source, text string, failure error) error {
	if failure != nil {
		_, err := db.pool.Exec(ctx, `UPDATE text_jobs SET failed = $3, due = now() + interval '10 minutes' * tries WHERE blob = $1 AND way = $2`,
			j.Blob, j.Way, failure.Error())
		return err
	}
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if text != "" {
			if err := saveText(ctx, tx, j.Blob, source, text, j.Account); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, "DELETE FROM text_jobs WHERE blob = $1 AND way = $2", j.Blob, j.Way)
		return err
	})
}

// saveText keeps a text of a blob from one source, replacing what that source said before.
func saveText(ctx context.Context, tx pgx.Tx, sha, source, text, author string) error {
	if !textSources[source] {
		return ErrText
	}
	text = cleanText(text)
	if len(text) > MaxFileText {
		// A rune cut in half at the end is dropped.
		text = strings.ToValidUTF8(text[:MaxFileText], "")
	}
	_, err := tx.Exec(ctx, `INSERT INTO file_texts (blob, source, text, author) VALUES ($1, $2, $3, nullif($4, ''))
		ON CONFLICT (tenant, blob, source) DO UPDATE SET text = EXCLUDED.text, author = EXCLUDED.author, at = now()`, sha, source, text, author)
	return err
}

// FileText is a text of a file and where it came from.
type FileText struct {
	Source string
	Text   string
	At     time.Time
}

// SaveFileText keeps a text of the current version of a file that the browser of a user read (ADR-24, the way of the
// browser). Who may change the file may give its text.
func (db *DB) SaveFileText(ctx context.Context, fileID, viewer, source, text string) error {
	if !textSources[source] || !utf8.ValidString(text) {
		return ErrText
	}
	f, err := db.File(ctx, fileID, viewer)
	if err != nil {
		return err
	}
	if err := db.mayDo(ctx, f.Object, viewer, ActWrite); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error { return saveText(ctx, tx, f.Blob, source, text, viewer) })
}

// FileTexts returns the texts of the current version of a file the viewer sees, the best first: what the content says
// itself before what was recognised in pixels.
func (db *DB) FileTexts(ctx context.Context, fileID, viewer string) ([]FileText, error) {
	f, err := db.File(ctx, fileID, viewer)
	if err != nil {
		return nil, err
	}
	rows, err := db.pool.Query(ctx, `SELECT source, text, at FROM file_texts WHERE blob = $1
		ORDER BY CASE source WHEN 'server' THEN 0 WHEN 'browser' THEN 1 WHEN 'ai' THEN 2 ELSE 3 END`, f.Blob)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (FileText, error) {
		var t FileText
		err := row.Scan(&t.Source, &t.Text, &t.At)
		return t, err
	})
}

// UseAIPages counts pages a model for OCR read for an account this month, refused with ErrQuota if that passes the
// quota of the user or of the tenant (0: none).
func (db *DB) UseAIPages(ctx context.Context, account string, pages, userQuota, tenantQuota int) error {
	month := time.Now().UTC().Format("2006-01") + "-01"
	return pgx.BeginFunc(ctx, db.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", lockQuota); err != nil {
			return err
		}
		var mine, all int
		if err := tx.QueryRow(ctx, `SELECT coalesce((SELECT pages FROM ai_ocr_usage WHERE account = $1 AND month = $2), 0),
			coalesce((SELECT sum(pages) FROM ai_ocr_usage WHERE month = $2), 0)`, account, month).Scan(&mine, &all); err != nil {
			return err
		}
		if (userQuota > 0 && mine+pages > userQuota) || (tenantQuota > 0 && all+pages > tenantQuota) {
			return ErrQuota
		}
		_, err := tx.Exec(ctx, `INSERT INTO ai_ocr_usage (account, month, pages) VALUES ($1, $2, $3)
			ON CONFLICT (tenant, account, month) DO UPDATE SET pages = ai_ocr_usage.pages + EXCLUDED.pages`, account, month, pages)
		return err
	})
}

// cleanText trims a text and drops what no search needs.
func cleanText(text string) string { return strings.TrimSpace(strings.ToValidUTF8(text, "")) }
