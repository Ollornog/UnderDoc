// SPDX-License-Identifier: MIT

package library

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"time"
)

// Source delivers the content of a library. The folder on disk is the first one (ADR-13).
type Source interface {
	// Scan returns the content, or nil if nothing has changed since the given state. It also returns the new state.
	Scan(since string) (in *Input, state string, err error)
	// OpenImage opens an image for reading.
	OpenImage(imagePath string) (io.ReadSeekCloser, time.Time, error)
}

// Store holds the current snapshot and replaces it when the source changes.
type Store struct {
	source  Source
	log     *slog.Logger
	current atomic.Pointer[Library]
	healthy atomic.Bool
	state   string
}

// NewStore returns an empty store. Refresh fills it.
func NewStore(source Source, log *slog.Logger) *Store {
	return &Store{source: source, log: log}
}

// Library returns the current snapshot, or nil before the first successful refresh.
func (s *Store) Library() *Library { return s.current.Load() }

// Healthy reports whether a snapshot exists and the last look at the source worked.
func (s *Store) Healthy() bool { return s.current.Load() != nil && s.healthy.Load() }

// OpenImage opens an image of the source.
func (s *Store) OpenImage(imagePath string) (io.ReadSeekCloser, time.Time, error) {
	return s.source.OpenImage(imagePath)
}

// Refresh looks at the source and builds a new snapshot if something has changed. On failure the last good snapshot stays.
func (s *Store) Refresh() error {
	in, state, err := s.source.Scan(s.state)
	if err != nil {
		s.healthy.Store(false)
		return fmt.Errorf("scan: %w", err)
	}
	s.healthy.Store(true)
	if in == nil {
		return nil
	}
	lib := Build(*in)
	s.current.Store(lib)
	s.state = state
	s.log.Info("library loaded", slog.Int("documents", lib.Len()), slog.Int("problems", len(lib.Problems())))
	return nil
}

// Run refreshes on every tick until the context ends. It only logs when the source starts or stops failing.
func (s *Store) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	failing := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := s.Refresh()
			switch {
			case err != nil && !failing:
				s.log.Error("source failing, keeping the last good state", slog.String("error", err.Error()))
			case err == nil && failing:
				s.log.Info("source works again")
			}
			failing = err != nil
		}
	}
}
