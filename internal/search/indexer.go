// SPDX-License-Identifier: MIT

package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

// Source says what there is to index. Pages of the database are one source and pages of the folder another; tables,
// lists, databases and files of later milestones each add theirs (M-2, M-4, M-6) and nothing else changes.
type Source interface {
	// Name is unique among sources and stays the same: the queue and the index keep it.
	Name() string
	// Docs returns everything to index for a key, with Source and Key set. None means the key is gone: its records are
	// removed. Docs must never carry a password (ADR-7).
	Docs(ctx context.Context, key string) ([]Doc, error)
	// Keys lists every key there is, to build the index anew.
	Keys(ctx context.Context) ([]string, error)
}

// Queue is the list of keys whose records are out of date, kept in the database so that nothing is lost when the search
// service is down or the application restarts.
type Queue interface {
	// Claim takes up to n keys that are due off the queue.
	Claim(ctx context.Context, n int) ([]Key, error)
	// Retry puts keys back, due after the given time. A key queued again meanwhile keeps its earlier time.
	Retry(ctx context.Context, keys []Key, after time.Duration) error
	// Enqueue adds keys.
	Enqueue(ctx context.Context, keys []Key) error
	// Version returns the version an index was last built with, and SetVersion records it.
	Version(ctx context.Context, index string) (string, error)
	SetVersion(ctx context.Context, index, version string) error
}

// Versioned is an engine whose settings carry a version, or a source whose records do: when it changes, the index is
// built anew. A source raises its version when it finds what it did not find before, such as the text of files.
type Versioned interface {
	Version() string
}

// Indexer keeps the index of a tenant up to date from the queue.
type Indexer struct {
	engine  Engine
	queue   Queue
	sources map[string]Source
	order   []string
	tenant  string
	log     *slog.Logger
	// Poll is how often the queue is looked at when it was empty.
	Poll time.Duration
	// Batch is how many keys are handled at once.
	Batch int
}

// NewIndexer returns an indexer for one tenant.
func NewIndexer(engine Engine, queue Queue, tenant string, log *slog.Logger, sources ...Source) *Indexer {
	ix := &Indexer{engine: engine, queue: queue, sources: map[string]Source{}, tenant: tenant, log: log, Poll: time.Second, Batch: 100}
	for _, s := range sources {
		ix.sources[s.Name()] = s
		ix.order = append(ix.order, s.Name())
	}
	return ix
}

func (ix *Indexer) versionKey() string { return ix.engine.Name() + "/" + ix.tenant }

// version is the version of the engine's settings, followed by the version of each source that has one.
func (ix *Indexer) version() string {
	version := "1"
	if v, ok := ix.engine.(Versioned); ok {
		version = v.Version()
	}
	for _, name := range ix.order {
		if v, ok := ix.sources[name].(Versioned); ok {
			version += "+" + name + "-" + v.Version()
		}
	}
	return version
}

// Prepare readies the index. When it is new, or was built by another version of the settings, every key of every source
// is queued.
func (ix *Indexer) Prepare(ctx context.Context) error {
	rebuild, err := ix.engine.Prepare(ctx, ix.tenant)
	if err != nil {
		return fmt.Errorf("prepare the index: %w", err)
	}
	built, err := ix.queue.Version(ctx, ix.versionKey())
	if err != nil {
		return err
	}
	if !rebuild && built == ix.version() {
		return nil
	}
	if err := ix.Rebuild(ctx); err != nil {
		return err
	}
	return ix.queue.SetVersion(ctx, ix.versionKey(), ix.version())
}

// Rebuild queues every key of every source.
func (ix *Indexer) Rebuild(ctx context.Context) error {
	for _, name := range ix.order {
		keys, err := ix.sources[name].Keys(ctx)
		if err != nil {
			return fmt.Errorf("keys of %s: %w", name, err)
		}
		batch := make([]Key, 0, len(keys))
		for _, key := range keys {
			batch = append(batch, Key{Source: name, Key: key})
		}
		if err := ix.queue.Enqueue(ctx, batch); err != nil {
			return err
		}
	}
	ix.log.Info("search index queued for a rebuild", slog.String("engine", ix.engine.Name()), slog.String("tenant", ix.tenant))
	return nil
}

// Run prepares the index and then works the queue until the context ends. While the search service is down it waits
// longer each time, up to a minute, and logs only when that starts and ends.
func (ix *Indexer) Run(ctx context.Context) {
	wait := ix.Poll
	failing := false
	prepared := false
	for {
		var err error
		worked := false
		if !prepared {
			err = ix.Prepare(ctx)
			prepared = err == nil
		}
		if prepared {
			worked, err = ix.Step(ctx)
		}
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			if !failing {
				ix.log.Error("search index not updated, trying again", slog.String("engine", ix.engine.Name()), slog.String("error", err.Error()))
			}
			failing = true
			wait = min(max(wait*2, ix.Poll), time.Minute)
		case failing:
			ix.log.Info("search index updated again", slog.String("engine", ix.engine.Name()))
			failing = false
			wait = ix.Poll
		}
		if worked && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// Step handles one batch of the queue. It reports whether there was anything to do.
func (ix *Indexer) Step(ctx context.Context) (bool, error) {
	keys, err := ix.queue.Claim(ctx, ix.Batch)
	if err != nil || len(keys) == 0 {
		return false, err
	}
	var docs []Doc
	var done, failed []Key
	var failure error
	for _, key := range keys {
		source, known := ix.sources[key.Source]
		if !known {
			// A source that is gone, such as one a later version dropped: its records go.
			done = append(done, key)
			continue
		}
		found, err := source.Docs(ctx, key.Key)
		if err != nil {
			failed = append(failed, key)
			failure = errors.Join(failure, fmt.Errorf("%s %s: %w", key.Source, key.Key, err))
			continue
		}
		for i := range found {
			found[i].Source, found[i].Key = key.Source, key.Key
			found[i].ID = DocID(key, i)
			found[i].Prepare()
		}
		docs = append(docs, found...)
		done = append(done, key)
	}
	if len(done) > 0 {
		if err := ix.engine.Replace(ctx, ix.tenant, done, docs); err != nil {
			failed = append(failed, done...)
			failure = errors.Join(failure, err)
		}
	}
	if len(failed) > 0 {
		if err := ix.queue.Retry(context.WithoutCancel(ctx), failed, 5*time.Second); err != nil {
			failure = errors.Join(failure, err)
		}
	}
	return true, failure
}

// Drain works the queue until it is empty, for tests and the command line.
func (ix *Indexer) Drain(ctx context.Context) error {
	for {
		worked, err := ix.Step(ctx)
		if err != nil {
			return err
		}
		if !worked {
			return nil
		}
	}
}

// DocID gives a record an ID engines accept: letters, digits, dash and underscore, the same for the same section.
func DocID(key Key, n int) string {
	sum := sha256.Sum256([]byte(key.Source + "\x00" + key.Key))
	return key.Source + "-" + hex.EncodeToString(sum[:12]) + "-" + strconv.Itoa(n)
}
