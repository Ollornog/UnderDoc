// SPDX-License-Identifier: MIT

// Command underdoc serves a folder of Markdown files as a read-only wiki.
//
// Configuration comes from the environment, defaults in brackets:
//
//	UNDERDOC_DIR            the folder (or the argument of serve)
//	UNDERDOC_ADDR           address of the wiki ("127.0.0.1:8080")
//	UNDERDOC_HEALTH_ADDR    address of the health endpoint ("127.0.0.1:8081")
//	UNDERDOC_SCAN_INTERVAL  how often the folder is checked for changes ("2s")
//	UNDERDOC_DEV_UI         "1" turns on the component gallery at /dev/ui
//	UNDERDOC_ADMIN_GROUP    the group of the gate whose members see the administration ("underdoc-admins")
//	UNDERDOC_DEV_USER       a login that stands in for the gate, for development on one machine only
//	UNDERDOC_DATABASE_URL   PostgreSQL for accounts and what is made in the web (none: the folder is all there is)
//
// There is no sign-in in the application: it must only be reachable through a sign-in gate, which passes the user on in
// request headers. That is why it binds to loopback unless told otherwise, and answers nothing without those headers.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/folder"
	"github.com/Ollornog/UnderDoc/internal/library"
	"github.com/Ollornog/UnderDoc/internal/version"
	"github.com/Ollornog/UnderDoc/internal/web"
)

const (
	usage             = "usage: underdoc serve [folder] | underdoc probe | underdoc version"
	defaultAddr       = "127.0.0.1:8080"
	defaultHealthAddr = "127.0.0.1:8081"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the command and returns the process exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	var err error
	switch {
	case args[0] == "version" && len(args) == 1:
		fmt.Fprintln(stdout, "underdoc", version.Version)
	case args[0] == "probe" && len(args) == 1:
		err = web.Probe(context.Background(), env("UNDERDOC_HEALTH_ADDR", defaultHealthAddr))
	case args[0] == "serve" && len(args) <= 2:
		dir := os.Getenv("UNDERDOC_DIR")
		if len(args) == 2 {
			dir = args[1]
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
		defer stop()
		err = serve(ctx, dir, stdout)
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "underdoc:", err)
		return 1
	}
	return 0
}

func serve(ctx context.Context, dir string, stdout io.Writer) error {
	if dir == "" {
		return fmt.Errorf("no folder: set UNDERDOC_DIR or pass it to serve")
	}
	interval, err := time.ParseDuration(env("UNDERDOC_SCAN_INTERVAL", "2s"))
	if err != nil || interval <= 0 {
		return fmt.Errorf("UNDERDOC_SCAN_INTERVAL: a positive duration is expected, like 2s")
	}
	log := slog.New(slog.NewJSONHandler(stdout, nil))
	store := library.NewStore(folder.New(dir), log)
	if err := store.Refresh(); err != nil {
		return err
	}
	options := web.Options{DevUI: os.Getenv("UNDERDOC_DEV_UI") == "1", AdminGroup: env("UNDERDOC_ADMIN_GROUP", "underdoc-admins")}
	healthy := store.Healthy
	if url := os.Getenv("UNDERDOC_DATABASE_URL"); url != "" {
		database, err := db.Open(ctx, url)
		if err != nil {
			return fmt.Errorf("database: %w", err)
		}
		defer database.Close()
		options.DB = database
		healthy = func() bool { return store.Healthy() && database.Healthy(context.Background()) }
		go database.KeepTrash(ctx, time.Hour, func(err error) { log.Error("emptying the trash failed", slog.String("error", err.Error())) })
	}
	if login := os.Getenv("UNDERDOC_DEV_USER"); login != "" {
		options.DevUser = &account.User{ID: login, Login: login, Name: login, Groups: []string{options.AdminGroup}}
		log.Warn("UNDERDOC_DEV_USER is set: every request counts as signed in, never use this behind a real gate", slog.String("user", login))
	}
	handler, err := web.New(store, log, options)
	if err != nil {
		return err
	}
	go store.Run(ctx, interval)
	log.Info("start", slog.String("version", version.Version))
	return web.Serve(ctx, log,
		web.NewServer(env("UNDERDOC_ADDR", defaultAddr), handler, log),
		web.NewServer(env("UNDERDOC_HEALTH_ADDR", defaultHealthAddr), web.Health(healthy), log))
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
