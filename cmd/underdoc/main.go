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
//	UNDERDOC_TENANTS        "one" tenant, or "many" found by their host name, which needs the database ("one")
//	UNDERDOC_TENANT_DOMAIN  with many tenants, the domain of the hoster: <tenant>.<domain> leads to a tenant
//	UNDERDOC_STORE_DIR      the content store of the files, with the database (none: no files; docs/files.md)
//	UNDERDOC_FILES_MAX_SIZE, _QUOTA, _USER_QUOTA, _COLLECTION_QUOTA, _KEEP_ALL_DAYS, _KEEP_DAILY_DAYS, _KEEP_WEEKLY_DAYS
//	                        limits and how long versions are kept (docs/files.md)
//	UNDERDOC_GATE           "on" lets tenants trust the headers of a sign-in gate, "off" never; empty keeps it where it is
//	UNDERDOC_SECRET_KEY     32 random bytes in base64 that seal secrets: TOTP, the client secret of OIDC (none: not offered)
//	UNDERDOC_MODULES        the modules allowed, each on by default or with ":off" off by default (none: every module)
//	UNDERDOC_HOSTER_HOST    with many tenants, the host name of the hoster administration (none: no such page)
//
// Export, import and tenants need the database (docs/export.md, docs/tenants.md). --tenant names the tenant, Main
// without it:
//
//	underdoc export [--tenant <id>] <dir>                          every collection as normal files, private notes under private/<login>/
//	underdoc import [--tenant <id>] --owner <login> <dir> <title>  a folder of Markdown files as a new collection of that account
//	underdoc import [--tenant <id>] --files --owner <login> <dir>  the files/ of an export, with UNDERDOC_STORE_DIR (docs/files.md)
//	underdoc tenant add <id> <name>                                a new, empty tenant, reached at <id>.<domain>
//	underdoc invite [--tenant <id>] [--admin] <login> <name>       an invitation to a local account, as a link path
//
// Each tenant chooses how people sign in (docs/sign-in.md): passkeys, password and code, password, OIDC, or a sign-in gate
// in front that passes the user on in request headers. An instance that ran behind a gate keeps it. It binds to loopback
// unless told otherwise.
package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/auth"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/folder"
	"github.com/Ollornog/UnderDoc/internal/library"
	"github.com/Ollornog/UnderDoc/internal/module"
	"github.com/Ollornog/UnderDoc/internal/store"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
	"github.com/Ollornog/UnderDoc/internal/version"
	"github.com/Ollornog/UnderDoc/internal/web"
)

const (
	usage             = "usage: underdoc serve [folder] | underdoc export [--tenant <id>] <dir> | underdoc import [--tenant <id>] --owner <login> <dir> <title> | underdoc import [--tenant <id>] --files --owner <login> <dir> | underdoc tenant add <id> <name> | underdoc invite [--tenant <id>] [--admin] <login> <name> | underdoc probe | underdoc version"
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
	case args[0] == "import" && slices.Contains(args, "--files"):
		ctx, rest, terr := tenantFlag(context.Background(), slices.DeleteFunc(slices.Clone(args[1:]), func(a string) bool { return a == "--files" }))
		owner, rest, ok := ownerFlag(rest)
		if terr != nil || !ok || len(rest) != 1 {
			fmt.Fprintln(stderr, usage)
			return 2
		}
		err = importFiles(ctx, owner, rest[0], stdout, stderr)
	case args[0] == "export":
		ctx, rest, terr := tenantFlag(context.Background(), args[1:])
		if terr != nil || len(rest) != 1 {
			fmt.Fprintln(stderr, usage)
			return 2
		}
		err = export(ctx, rest[0], stdout, stderr)
	case args[0] == "import":
		ctx, rest, terr := tenantFlag(context.Background(), args[1:])
		owner, rest, ok := ownerFlag(rest)
		if terr != nil || !ok || len(rest) != 2 {
			fmt.Fprintln(stderr, usage)
			return 2
		}
		err = importTree(ctx, owner, rest[0], rest[1], stdout, stderr)
	case args[0] == "tenant" && len(args) == 4 && args[1] == "add":
		err = addTenant(context.Background(), args[2], args[3], stdout)
	case args[0] == "invite":
		ctx, rest, terr := tenantFlag(context.Background(), args[1:])
		admin := false
		if i := slices.Index(rest, "--admin"); i >= 0 {
			admin, rest = true, slices.Delete(rest, i, i+1)
		}
		if terr != nil || len(rest) != 2 {
			fmt.Fprintln(stderr, usage)
			return 2
		}
		err = inviteAccount(ctx, rest[0], rest[1], admin, stdout)
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
	lib := library.NewStore(folder.New(dir), log)
	if err := lib.Refresh(); err != nil {
		return err
	}
	options := web.Options{
		DevUI:        os.Getenv("UNDERDOC_DEV_UI") == "1",
		AdminGroup:   env("UNDERDOC_ADMIN_GROUP", "underdoc-admins"),
		Tenants:      env("UNDERDOC_TENANTS", tenant.ModeOne),
		TenantDomain: os.Getenv("UNDERDOC_TENANT_DOMAIN"),
		Gate:         os.Getenv("UNDERDOC_GATE"),
		HosterHost:   os.Getenv("UNDERDOC_HOSTER_HOST"),
	}
	if options.Modules, err = module.ParsePolicy(os.Getenv("UNDERDOC_MODULES")); err != nil {
		return err
	}
	keys, err := vault.New(os.Getenv(vault.KeyEnv))
	if err != nil {
		return err
	}
	options.Vault = keys
	if !keys.Ready() {
		log.Info(vault.KeyEnv + " is not set: password and code, and the client secret of OIDC, are not offered")
	}
	if options.Tenants == tenant.ModeMany && options.TenantDomain == "" {
		log.Warn("UNDERDOC_TENANT_DOMAIN is not set: tenants are found by a domain of their own only")
	}
	healthy := lib.Healthy
	if url := os.Getenv("UNDERDOC_DATABASE_URL"); url != "" {
		database, err := db.Open(ctx, url)
		if err != nil {
			return fmt.Errorf("database: %w", err)
		}
		defer database.Close()
		options.DB = database
		healthy = func() bool { return lib.Healthy() && database.Healthy(context.Background()) }
		// The trash belongs to Documents: where that is off, its job does not run either (ADR-25).
		go database.KeepTrash(ctx, time.Hour, web.ModuleOn(database, options.Modules, module.Documents), func(err error) { log.Error("emptying the trash failed", slog.String("error", err.Error())) })
		files, err := readFilesConfig()
		if err != nil {
			return err
		}
		if files.Dir != "" {
			if options.Store, err = store.Open(files.Dir); err != nil {
				return fmt.Errorf("content store: %w", err)
			}
			database.SetLimits(files.Limits)
			go keepFiles(ctx, database, options.Store, files.Retention, time.Hour, web.ModuleOn(database, options.Modules, module.Files), log)
		}
	}
	if login := os.Getenv("UNDERDOC_DEV_USER"); login != "" {
		options.DevUser = &account.User{ID: login, Login: login, Name: login, Groups: []string{options.AdminGroup}}
		log.Warn("UNDERDOC_DEV_USER is set: every request counts as signed in, never use this behind a real gate", slog.String("user", login))
	}
	handler, err := web.New(lib, log, options)
	if err != nil {
		return err
	}
	go lib.Run(ctx, interval)
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

// addTenant makes a new, empty tenant.
func addTenant(ctx context.Context, tenantID, name string, stdout io.Writer) error {
	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()
	t, err := database.CreateTenant(ctx, tenantID, name)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "tenant %s (%s) is there\n", t.ID, t.Name)
	return nil
}

// inviteAccount makes an invitation to a local account and prints its path, valid for a week: the way to the first
// administrator of a tenant that has no way in yet.
func inviteAccount(ctx context.Context, login, name string, admin bool, stdout io.Writer) error {
	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()
	token, hash, err := auth.NewToken()
	if err != nil {
		return err
	}
	if err := database.CreateInvite(ctx, hash, db.Invite{Login: login, Name: name, Admin: admin, Expires: time.Now().Add(7 * 24 * time.Hour)}); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "open /invite/%s on the host of the tenant within a week; it works once\n", token)
	return nil
}
