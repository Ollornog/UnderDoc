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
//	UNDERDOC_DEV_USER       a login that stands in for the gate, for development on one machine only
//	UNDERDOC_DATABASE_URL   PostgreSQL for accounts and what is made in the web (none: the folder is all there is)
//	UNDERDOC_TENANTS        "one" tenant; "many" waits for the gate to name the tenant (ADR-32)
//	UNDERDOC_STORE_DIR      the content store of the files, with the database (none: no files; docs/files.md)
//	UNDERDOC_FILES_MAX_SIZE, _QUOTA, _USER_QUOTA, _COLLECTION_QUOTA, _KEEP_ALL_DAYS, _KEEP_DAILY_DAYS, _KEEP_WEEKLY_DAYS
//	                        limits and how long versions are kept (docs/files.md)
//	UNDERDOC_SECRET_KEY     32 random bytes in base64 that seal secrets: passwords in the text, app passwords of Samba
//	                        (none: not offered)
//	UNDERDOC_SECRET_KEY_OLD the key before a rotation, until underdoc rekey ran (docs/secrets.md)
//	UNDERDOC_MODULES        the modules allowed, each on by default or with ":off" off by default (none: every module)
//	UNDERDOC_OFFICE_URL     the optional Office module for previews, a Gotenberg service (none: no Office previews)
//	UNDERDOC_TEXT_WAYS, _TEXT_MAX_SIZE, UNDERDOC_TESSERACT, UNDERDOC_AI_OCR_PAGES_USER, _TENANT
//	                        how the text of files is read for the search (docs/files.md)
//	UNDERDOC_SEARCH_*       the search service, see search.go and docs/search.md
//	UNDERDOC_GATE_*, UNDERDOC_ADMIN_ROLE
//	                        the sign-in gate and its client, see gate.go and docs/sign-in.md
//
// Export, import and tenants need the database (docs/export.md, docs/tenants.md). --tenant names the tenant, Main
// without it:
//
//	underdoc export [--tenant <id>] <dir>                          every collection as normal files, private notes under private/<login>/
//	underdoc import [--tenant <id>] --owner <login> <dir> <title>  a folder of Markdown files as a new collection of that account
//	underdoc import [--tenant <id>] --files --owner <login> <dir>  the files/ of an export, with UNDERDOC_STORE_DIR (docs/files.md)
//	underdoc tenant add <id> <name>                                a new, empty tenant, reached at <id>.<domain>
//	underdoc account [--tenant <id>] list                         every account, with the lasting ID of the gate it is bound to
//	underdoc account [--tenant <id>] bind <key|login> <remote-id> an account made before ADR-32 bound to its person in the gate
//	underdoc admin [--tenant <id>] grant|revoke <remote-id>        the emergency access: marks the account of that lasting ID as an
//	                                                               administrator, or takes the mark back
//	underdoc reindex [--tenant <id>]                               everything of a tenant queued for the search service again
//	underdoc rekey                                                 every sealed value of every tenant sealed again under
//	                                                               UNDERDOC_SECRET_KEY, opened with UNDERDOC_SECRET_KEY_OLD
//
// People sign in only through the gate in front, TinySesam, whose Go client checks what it passes on (ADR-32,
// docs/sign-in.md). UnderDoc binds to loopback unless told otherwise.
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
	"github.com/Ollornog/UnderDoc/internal/db"
	filetype "github.com/Ollornog/UnderDoc/internal/files"
	"github.com/Ollornog/UnderDoc/internal/folder"
	"github.com/Ollornog/UnderDoc/internal/library"
	"github.com/Ollornog/UnderDoc/internal/module"
	"github.com/Ollornog/UnderDoc/internal/share"
	"github.com/Ollornog/UnderDoc/internal/sheet"
	"github.com/Ollornog/UnderDoc/internal/store"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
	"github.com/Ollornog/UnderDoc/internal/version"
	"github.com/Ollornog/UnderDoc/internal/web"
)

const (
	usage             = "usage: underdoc serve [folder] | underdoc export [--tenant <id>] <dir> | underdoc import [--tenant <id>] --owner <login> <dir> <title> | underdoc import [--tenant <id>] --files --owner <login> <dir> | underdoc tenant add <id> <name> | underdoc account [--tenant <id>] list | underdoc account [--tenant <id>] bind <key|login> <remote-id> | underdoc admin [--tenant <id>] grant|revoke <remote-id> | underdoc reindex [--tenant <id>] | underdoc rekey | underdoc probe | underdoc version"
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
	case args[0] == "reindex":
		ctx, rest, terr := tenantFlag(context.Background(), args[1:])
		if terr != nil || len(rest) != 0 {
			fmt.Fprintln(stderr, usage)
			return 2
		}
		err = reindex(ctx, stdout)
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
	case args[0] == "rekey" && len(args) == 1:
		err = rekey(context.Background(), stdout)
	case args[0] == "tenant" && len(args) == 4 && args[1] == "add":
		err = addTenant(context.Background(), args[2], args[3], stdout)
	case args[0] == "account" || args[0] == "admin":
		ctx, rest, terr := tenantFlag(context.Background(), args[1:])
		if terr != nil || !accountCommand(args[0], rest) {
			fmt.Fprintln(stderr, usage)
			return 2
		}
		err = accountsCommand(ctx, args[0], rest, stdout)
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
		DevUI:   os.Getenv("UNDERDOC_DEV_UI") == "1",
		Tenants: env("UNDERDOC_TENANTS", tenant.ModeOne),
	}
	if options.Tenants == tenant.ModeMany {
		// The gate does not name the tenant yet (its T-73): every person of the gate would come into every tenant.
		return fmt.Errorf("UNDERDOC_TENANTS=many waits for the sign-in gate to name the tenant (ADR-32); run one tenant")
	}
	if gate := os.Getenv("UNDERDOC_GATE"); gate == "off" {
		return fmt.Errorf("UNDERDOC_GATE=off: the gate is the only way in since ADR-32, so nobody could sign in; remove the setting")
	} else if gate != "" {
		log.Warn("UNDERDOC_GATE is gone with ADR-32: the gate is the only way in; remove the setting")
	}
	if options.Modules, err = module.ParsePolicy(os.Getenv("UNDERDOC_MODULES")); err != nil {
		return err
	}
	gateConfig, err := readGateConfig(log)
	if err != nil {
		return err
	}
	if options.GateClient, err = account.NewGate(gateConfig); err != nil {
		return fmt.Errorf("gate: %w", err)
	}
	keys, err := vault.NewRotating(os.Getenv(vault.KeyEnv), os.Getenv(vault.OldKeyEnv))
	if err != nil {
		return err
	}
	options.Vault = keys
	if !keys.Ready() {
		log.Info(vault.KeyEnv + " is not set: passwords in the text and Samba are not offered")
	}
	if keys.Rotating() {
		log.Warn(vault.OldKeyEnv + " is set: run underdoc rekey, then remove it")
	}
	searching, err := readSearchConfig()
	if err != nil {
		return err
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
		// The trash belongs to Documents: where that is off, its trash is not emptied (ADR-25). Signing in and the log
		// expire in every tenant (ADR-29).
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
			// One cache of spreadsheets for the web and the search, set before the indexers start (ADR-31).
			options.Sheets = sheet.NewCache(options.Store, log)
			database.SetSheets(options.Sheets)
			if office := os.Getenv("UNDERDOC_OFFICE_URL"); office != "" {
				options.Office = officeModule(office)
			}
			filesOn := web.ModuleOn(database, options.Modules, module.Files)
			go keepFiles(ctx, database, options.Store, files.Retention, time.Hour, filesOn, log)
			text, err := readTextConfig()
			if err != nil {
				return err
			}
			options.TextWays = text.Ways
			worker := textWorker{db: database, store: options.Store, config: text, on: filesOn, log: log}
			if text.Tesseract != "" {
				worker.ocr = filetype.Tesseract{Path: text.Tesseract}
				options.OCR = worker.ocr
			}
			if text.Ways.Server || text.Ways.AI {
				go worker.run(ctx, 5*time.Second)
			}
			sambaConfig, on, err := readSambaConfig()
			if err != nil {
				return err
			}
			if on {
				if options.Share, err = share.New(database, options.Store, keys, sambaConfig, log); err != nil {
					return err
				}
				options.Share.Start(ctx)
				log.Info("samba: the working copy runs", slog.String("dir", sambaConfig.Dir))
			}
		} else if os.Getenv("UNDERDOC_SAMBA") != "" {
			return fmt.Errorf("samba needs the content store (UNDERDOC_STORE_DIR)")
		}
		options.Search = startSearch(ctx, searching, database, lib, log)
	} else if searching.engine != "" {
		log.Warn("the search service needs the database (UNDERDOC_DATABASE_URL); the folder is searched in memory")
	}
	if login := os.Getenv("UNDERDOC_DEV_USER"); login != "" {
		// It stands for the account with the lasting ID 1 of a gate, an administrator.
		options.DevUser = &account.User{ID: "1", RemoteID: "1", Login: login, Name: login, Groups: []string{options.GateClient.AdminRole()}}
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
