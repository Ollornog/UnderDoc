// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Ollornog/UnderDoc/internal/db"
	filetype "github.com/Ollornog/UnderDoc/internal/files"
	"github.com/Ollornog/UnderDoc/internal/store"
)

// textConfig is how the text of files is read for the search (ADR-24, docs/files.md), from the environment.
type textConfig struct {
	Ways db.Ways
	// MaxSize is the largest file the server reads.
	MaxSize int64
	// Tesseract is the program for OCR on the server, if any.
	Tesseract string
	// AIUser and AITenant are the pages a model may read per month for one user and for the tenant, 0 for no limit.
	AIUser, AITenant int
}

func readTextConfig() (textConfig, error) {
	var c textConfig
	for _, way := range strings.FieldsFunc(env("UNDERDOC_TEXT_WAYS", "browser,server"), func(r rune) bool { return r == ',' || r == ' ' }) {
		switch way {
		case "browser":
			c.Ways.Browser = true
		case "server":
			c.Ways.Server = true
		case "ai":
			c.Ways.AI = true
		case "none":
		default:
			return c, fmt.Errorf("UNDERDOC_TEXT_WAYS: %q is none of browser, server, ai", way)
		}
	}
	var err error
	if c.MaxSize, err = parseSize(env("UNDERDOC_TEXT_MAX_SIZE", "100M")); err != nil {
		return c, fmt.Errorf("UNDERDOC_TEXT_MAX_SIZE: %w", err)
	}
	c.Tesseract = os.Getenv("UNDERDOC_TESSERACT")
	for name, into := range map[string]*int{"UNDERDOC_AI_OCR_PAGES_USER": &c.AIUser, "UNDERDOC_AI_OCR_PAGES_TENANT": &c.AITenant} {
		n, err := strconv.Atoi(env(name, "0"))
		if err != nil || n < 0 {
			return c, fmt.Errorf("%s: a number of pages is expected", name)
		}
		*into = n
	}
	return c, nil
}

// textWorker reads the text of files in the queue, one at a time, each within a time limit, until ctx ends.
type textWorker struct {
	db      *db.DB
	store   *store.Store
	config  textConfig
	ocr, ai filetype.OCR
	changed func(ctx context.Context, fileIDs []string)
	// on tells whether the module Files is on for the tenant of ctx; nil for always.
	on  func(context.Context) bool
	log *slog.Logger
}

// run takes jobs until ctx ends, and looks again every poll when there is none.
func (t textWorker) run(ctx context.Context, poll time.Duration) {
	for ctx.Err() == nil {
		did, err := t.once(ctx)
		if err != nil && ctx.Err() == nil {
			t.log.Error("reading the text of a file failed", slog.String("error", err.Error()))
		}
		if !did {
			select {
			case <-ctx.Done():
			case <-time.After(poll):
			}
		}
	}
}

// once does the next job, if there is one, and reports whether there was.
func (t textWorker) once(ctx context.Context) (bool, error) {
	job, err := t.db.ClaimTextJob(ctx, 5*time.Minute)
	if errors.Is(err, db.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	work, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	source, text, failure := t.read(work, job)
	if errors.Is(failure, filetype.ErrNoText) {
		failure = nil
	}
	if err := t.db.FinishTextJob(ctx, job, source, text, failure); err != nil {
		return true, err
	}
	if text != "" && t.changed != nil {
		if ids, err := t.db.FilesOfBlob(ctx, job.Blob); err == nil && len(ids) > 0 {
			t.changed(ctx, ids)
		}
	}
	return true, failure
}

// read does one job. A PDF without text is a scan: it goes on to OCR, the ways the uploader has on.
func (t textWorker) read(ctx context.Context, job db.TextJob) (string, string, error) {
	tenant := store.DefaultTenant
	f, err := t.store.Open(tenant, job.Blob)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return "", "", err
	}
	if info.Size() > t.config.MaxSize {
		return "", "", fmt.Errorf("%w: larger than UNDERDOC_TEXT_MAX_SIZE", filetype.ErrNoText)
	}
	switch job.Way {
	case filetype.TextServer:
		text, err := filetype.ServerText(ctx, f, info.Size(), job.ContentType)
		if errors.Is(err, filetype.ErrNoText) && job.ContentType == "application/pdf" {
			err = errors.Join(err, t.scan(ctx, job))
		}
		return filetype.TextServer, text, err
	case filetype.TextOCR:
		if t.ocr == nil {
			return "", "", filetype.ErrNoText
		}
		text, _, err := t.ocr.Recognize(ctx, f, job.ContentType)
		return filetype.TextOCR, text, err
	case filetype.TextAI:
		if t.ai == nil {
			return "", "", filetype.ErrNoText
		}
		// A page is counted before, so a quota used up stops the model before it reads.
		if err := t.db.UseAIPages(ctx, job.Account, 1, t.config.AIUser, t.config.AITenant); err != nil {
			return "", "", err
		}
		text, pages, err := t.ai.Recognize(ctx, f, job.ContentType)
		if err == nil && pages > 1 {
			err = t.db.UseAIPages(ctx, job.Account, pages-1, 0, 0)
		}
		return filetype.TextAI, text, err
	}
	return "", "", fmt.Errorf("unknown way %q", job.Way)
}

// scan puts the OCR of a scanned PDF on the queue, by a model where the uploader has that way on.
func (t textWorker) scan(ctx context.Context, job db.TextJob) error {
	ways, err := t.db.TextWays(ctx, t.config.Ways, job.Account)
	if err != nil || !ways.AI || t.ai == nil {
		return err
	}
	return t.db.QueueText(ctx, job.Blob, job.ContentType, job.Account, filetype.TextAI)
}
