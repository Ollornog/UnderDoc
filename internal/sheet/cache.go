// SPDX-License-Identifier: MIT

package sheet

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/Ollornog/UnderDoc/internal/files"
)

// What was read of a blob is kept beside it (derived, ADR-28) and in a small cache in memory, so a page with embeddings,
// the table view and the search do not read the file again (ADR-31). At most two workbooks are read at once, each for at
// most ReadTime.

const (
	// ReadTime is how long reading one workbook may take.
	ReadTime = 30 * time.Second
	// Derived is the name of what was read, after the hash of the blob; its number moves when the model changes.
	Derived = "-sheet-1.json.gz"
	// Failed keeps why a blob could not be read, so a broken or too large file is not read again on every view.
	Failed = "-sheet-1.err"
)

// Reason is why a file cannot be shown as tables, a sentence for a person.
type Reason struct{ Text string }

func (r *Reason) Error() string { return r.Text }

// Blobs is the content store as the cache needs it (internal/store).
type Blobs interface {
	Open(tenant, sha string) (*os.File, error)
	DerivedPath(tenant, name string) (string, error)
	WriteDerived(tenant, name string, content []byte) error
}

// Cache reads workbooks of blobs and keeps them.
type Cache struct {
	blobs   Blobs
	log     *slog.Logger
	readers chan struct{}
	mu      sync.Mutex
	max     int
	order   []string
	entries map[string]cached
}

type cached struct {
	wb    *Workbook
	err   error
	until time.Time
}

// NewCache returns a cache over a content store that keeps the last eight workbooks in memory.
func NewCache(blobs Blobs, log *slog.Logger) *Cache {
	return &Cache{blobs: blobs, log: log, readers: make(chan struct{}, 2), max: 8, entries: map[string]cached{}}
}

func (c *Cache) get(key string) (cached, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if ok && !e.until.IsZero() && time.Now().After(e.until) {
		delete(c.entries, key)
		return cached{}, false
	}
	return e, ok
}

func (c *Cache) put(key string, e cached) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[key]; !ok {
		c.order = append(c.order, key)
	}
	c.entries[key] = e
	for len(c.order) > c.max {
		delete(c.entries, c.order[0])
		c.order = c.order[1:]
	}
}

// Workbook returns the workbook of a blob: from memory, from what was read before, or read now. A file that cannot be read
// gives a *Reason; a reason that may pass, the end of the time, is kept for ten minutes only.
func (c *Cache) Workbook(ctx context.Context, tenant, sha, contentType, name string) (*Workbook, error) {
	key := tenant + "/" + sha
	if e, ok := c.get(key); ok {
		return e.wb, e.err
	}
	wb, err := c.read(ctx, tenant, sha, contentType, name)
	switch {
	case err == nil:
		c.put(key, cached{wb: wb})
	case errors.Is(err, context.DeadlineExceeded):
		err = &Reason{"this file takes too long to read as tables"}
		c.put(key, cached{err: err, until: time.Now().Add(10 * time.Minute)})
	case errors.As(err, new(*Reason)):
		c.put(key, cached{err: err})
	}
	return wb, err
}

func (c *Cache) read(ctx context.Context, tenant, sha, contentType, name string) (*Workbook, error) {
	done, err := c.blobs.DerivedPath(tenant, sha+Derived)
	if err != nil {
		return nil, err
	}
	failed, err := c.blobs.DerivedPath(tenant, sha+Failed)
	if err != nil {
		return nil, err
	}
	if wb, err := load(done); err == nil {
		return wb, nil
	}
	if reason, err := os.ReadFile(failed); err == nil {
		return nil, &Reason{string(reason)}
	}
	select {
	case c.readers <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-c.readers }()
	f, err := c.blobs.Open(tenant, sha)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	// The time of reading runs on its own: a viewer who leaves does not end it, so the next one finds it done.
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ReadTime)
	defer cancel()
	wb, err := Read(readCtx, f, info.Size(), contentType, name)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		reason := reasonOf(err)
		if werr := c.blobs.WriteDerived(tenant, sha+Failed, []byte(reason)); werr != nil {
			c.log.Warn("sheet reason not kept", slog.String("error", werr.Error()))
		}
		return nil, &Reason{reason}
	}
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	if err := json.NewEncoder(z).Encode(wb); err == nil && z.Close() == nil {
		if err := c.blobs.WriteDerived(tenant, sha+Derived, buf.Bytes()); err != nil {
			c.log.Warn("sheet not kept", slog.String("error", err.Error()))
		}
	}
	return wb, nil
}

// reasonOf is the sentence a person reads when a file cannot be shown as tables.
func reasonOf(err error) string {
	var limit *LimitError
	switch {
	case errors.As(err, &limit):
		return limit.Error()
	case errors.Is(err, files.ErrBomb):
		return "this file cannot be shown as tables: it unpacks to far more than it is"
	}
	return ErrUnreadable.Error()
}

func load(path string) (*Workbook, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	z, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	var wb Workbook
	if err := json.NewDecoder(io.LimitReader(z, 1<<30)).Decode(&wb); err != nil {
		return nil, err
	}
	return &wb, nil
}
