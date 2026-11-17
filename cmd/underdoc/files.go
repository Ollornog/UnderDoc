// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Ollornog/UnderDoc/internal/db"
	filetype "github.com/Ollornog/UnderDoc/internal/files"
	"github.com/Ollornog/UnderDoc/internal/store"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// filesConfig is the configuration of the files (ADR-28), read from the environment.
type filesConfig struct {
	Dir       string
	Limits    db.Limits
	Retention db.Retention
}

// readFilesConfig reads the configuration of the files. Without UNDERDOC_STORE_DIR there are no files.
func readFilesConfig() (filesConfig, error) {
	c := filesConfig{Dir: os.Getenv("UNDERDOC_STORE_DIR")}
	sizes := []struct {
		name string
		into *int64
		def  string
	}{
		{"UNDERDOC_FILES_MAX_SIZE", &c.Limits.MaxFile, "2G"},
		{"UNDERDOC_FILES_QUOTA", &c.Limits.Tenant, "0"},
		{"UNDERDOC_FILES_USER_QUOTA", &c.Limits.User, "10G"},
		{"UNDERDOC_FILES_COLLECTION_QUOTA", &c.Limits.Collection, "0"},
	}
	for _, s := range sizes {
		n, err := parseSize(env(s.name, s.def))
		if err != nil {
			return filesConfig{}, fmt.Errorf("%s: %w", s.name, err)
		}
		*s.into = n
	}
	days := []struct {
		name string
		into *time.Duration
		def  string
	}{
		{"UNDERDOC_FILES_KEEP_ALL_DAYS", &c.Retention.KeepAll, "30"},
		{"UNDERDOC_FILES_KEEP_DAILY_DAYS", &c.Retention.KeepDaily, "90"},
		{"UNDERDOC_FILES_KEEP_WEEKLY_DAYS", &c.Retention.KeepWeekly, "0"},
	}
	for _, d := range days {
		n, err := strconv.Atoi(env(d.name, d.def))
		if err != nil || n < 0 || n > 100000 {
			return filesConfig{}, fmt.Errorf("%s: a number of days is expected, like 30", d.name)
		}
		*d.into = time.Duration(n) * 24 * time.Hour
	}
	return c, nil
}

// officeModule is the optional module that converts Office files for their preview (docs/files.md).
func officeModule(url string) filetype.Gotenberg {
	return filetype.Gotenberg{URL: url, Client: &http.Client{Timeout: 3 * time.Minute}}
}

// parseSize reads a size in bytes, with an optional K, M, G or T for powers of 1024; 0 means no limit.
func parseSize(s string) (int64, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	shift := 0
	if s != "" {
		switch s[len(s)-1] {
		case 'K':
			shift = 10
		case 'M':
			shift = 20
		case 'G':
			shift = 30
		case 'T':
			shift = 40
		}
		if shift > 0 {
			s = s[:len(s)-1]
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 || n > (1<<62)>>shift {
		return 0, fmt.Errorf("a size is expected, like 500M or 2G")
	}
	return n << shift, nil
}

// keepFiles keeps the store clean for every tenant, at once and then every so often until ctx ends: uploads that got
// nothing for a day go, versions are thinned, blobs nothing refers to leave the store, and blobs the database does not
// know go after a day.
func keepFiles(ctx context.Context, database *db.DB, s *store.Store, retention db.Retention, every time.Duration, on func(context.Context) bool, log *slog.Logger) {
	swept := map[string]time.Time{}
	round := func(ctx context.Context, t string) error {
		if _, err := database.ExpireUploads(ctx, time.Now().Add(-24*time.Hour), func(id string) error { return s.RemoveUpload(t, id) }); err != nil {
			return err
		}
		if _, err := database.ThinVersions(ctx, time.Now(), retention); err != nil {
			return err
		}
		if _, err := database.CollectBlobs(ctx, func(sha string) error { return s.Remove(t, sha) }); err != nil {
			return err
		}
		// The sweep walks the whole store of the tenant and holds back finishing uploads meanwhile, so it runs once a day.
		if time.Since(swept[t]) < 24*time.Hour {
			return nil
		}
		each := func(fn func(string, time.Time) error) error { return s.Blobs(t, fn) }
		_, err := database.SweepStore(ctx, each, func(sha string) error { return s.Remove(t, sha) }, 24*time.Hour)
		if err == nil {
			swept[t] = time.Now()
		}
		return err
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		tenants, err := database.TenantIDs(ctx)
		for _, t := range tenants {
			// Where Files is switched off, its job leaves everything as it is (ADR-25).
			if on != nil && !on(tenant.With(ctx, t)) {
				continue
			}
			if err := round(tenant.With(ctx, t), t); err != nil && ctx.Err() == nil {
				log.Error("keeping the files failed", slog.String("tenant", t), slog.String("error", err.Error()))
			}
		}
		if err != nil && ctx.Err() == nil {
			log.Error("keeping the files failed", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
