// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/store"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/transfer"
)

// openDatabase opens the database that export and import work on.
func openDatabase(ctx context.Context) (*db.DB, error) {
	url := os.Getenv("UNDERDOC_DATABASE_URL")
	if url == "" {
		return nil, errors.New("no database: set UNDERDOC_DATABASE_URL")
	}
	database, err := db.Open(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	return database, nil
}

// export writes every collection of the tenant of ctx into dir, which must not exist yet or be empty, so nothing is
// ever overwritten. Private notes of every account go too: whoever runs this runs the server (docs/export.md).
func export(ctx context.Context, dir string, stdout, stderr io.Writer) error {
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("%s is not empty, export writes only into a new or empty directory", dir)
	}
	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()
	shared, private, err := database.Everything(ctx)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	files := 0
	warnings, err := transfer.Write(transfer.Tree{Shared: shared, Private: private}, func(name string, data []byte) error {
		if parent := path.Dir(name); parent != "." {
			if err := root.MkdirAll(parent, 0o755); err != nil {
				return err
			}
		}
		files++
		return root.WriteFile(name, data, 0o644)
	})
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintln(stderr, "warning:", w)
	}
	fmt.Fprintf(stdout, "exported %d collections and the private notes of %d accounts as %d files to %s\n", len(shared), accounts(private), files, dir)
	return exportFiles(ctx, database, root, stdout)
}

// exportFiles writes the view Files and the attachments of pages below files/, with their content from the store: a
// backup of the database and the store together (ADR-28). Without a store there is nothing to write.
func exportFiles(ctx context.Context, database *db.DB, root *os.Root, stdout io.Writer) error {
	config, err := readFilesConfig()
	if err != nil || config.Dir == "" {
		return err
	}
	s, err := store.Open(config.Dir)
	if err != nil {
		return err
	}
	t, _ := tenant.From(ctx)
	roots, attachments, err := database.AllFiles(ctx)
	if err != nil {
		return err
	}
	if err := root.Mkdir(transfer.FilesDir, 0o755); err != nil {
		return err
	}
	dir, err := root.OpenRoot(transfer.FilesDir)
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	done, err := transfer.WriteFiles(dir, roots, attachments, func(sha string) (io.ReadCloser, error) { return s.Open(t, sha) })
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "exported %d roots of the files with %d files and %d further versions and attachments to %s/\n", done.Roots, done.Files, done.Blobs, transfer.FilesDir)
	return nil
}

func hashOf(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// importFiles reads a files/ directory of an export back: the content into the store, checked against its hash, then the
// roots, files, versions and attachments into the database in one transaction (docs/files.md).
func importFiles(ctx context.Context, owner, dir string, stdout, stderr io.Writer) error {
	config, err := readFilesConfig()
	if err != nil {
		return err
	}
	if config.Dir == "" {
		return errors.New("no content store: set UNDERDOC_STORE_DIR")
	}
	s, err := store.Open(config.Dir)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	in, err := transfer.ReadFiles(root)
	if err != nil {
		return err
	}
	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()
	t, _ := tenant.From(ctx)
	// Every copy of every content is checked against its hash, so a backup changed by hand is found, not half taken.
	for _, sha := range slices.Sorted(maps.Keys(in.Sources)) {
		for i, name := range in.Sources[sha] {
			f, err := root.Open(name)
			if err != nil {
				return err
			}
			var got string
			if i == 0 && !s.Has(t, sha) {
				got, _, err = s.Put(t, f, math.MaxInt64-1)
				if err == nil && got != sha {
					_ = s.Remove(t, got)
				}
			} else {
				got, err = hashOf(f)
			}
			_ = f.Close()
			if err != nil {
				return err
			}
			if got != sha {
				return fmt.Errorf("%s: the content does not match its hash %s", name, sha)
			}
		}
	}
	done, err := database.ImportFiles(ctx, owner, in.Roots, in.Attachments, func(sha string) bool { return s.Has(t, sha) })
	if err != nil {
		return err
	}
	for _, w := range done.Warnings {
		fmt.Fprintln(stderr, "warning:", w)
	}
	for _, old := range slices.Sorted(maps.Keys(done.Renamed)) {
		fmt.Fprintf(stderr, "new id: %s was taken or not valid, it is %s now\n", old, done.Renamed[old])
	}
	fmt.Fprintf(stdout, "imported %d roots with %d objects, %d versions and %d attachments, %d got a new id\n",
		done.Roots, done.Objects, done.Versions, done.Attachments, len(done.Renamed))
	return nil
}

func accounts(private []db.Owned) int {
	seen := map[string]bool{}
	for _, o := range private {
		seen[o.Login] = true
	}
	return len(seen)
}

// importTree reads dir into a new collection of the account with the login owner, in the tenant of ctx. It never
// changes what exists. With archived, or where the export says so, the new space is archived (T-34).
func importTree(ctx context.Context, owner, dir, title string, archived bool, stdout, stderr io.Writer) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	tree, warnings, err := transfer.Read(root.FS())
	if err != nil {
		return err
	}
	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()
	tree.Archived = tree.Archived || archived
	done, err := database.Import(ctx, owner, title, tree)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Fprintln(stderr, "warning:", w)
	}
	for _, old := range slices.Sorted(maps.Keys(done.Renamed)) {
		fmt.Fprintf(stderr, "new id: %s was taken or not valid, it is %s now\n", old, done.Renamed[old])
	}
	state := ""
	if done.Top.Archived {
		state = ", archived"
	}
	fmt.Fprintf(stdout, "imported %d objects into %q (%s%s), %d got a new id\n", done.Objects, done.Top.Title, done.Top.ID, state, len(done.Renamed))
	return nil
}

// takeFlag takes --name <value> or --name=<value> from anywhere in the arguments and returns the rest. It reports false
// only for the flag without its value.
func takeFlag(args []string, name string) (value string, rest []string, ok bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--"+name || a == "-"+name:
			if i+1 >= len(args) {
				return "", nil, false
			}
			value = args[i+1]
			i++
		case strings.HasPrefix(a, "--"+name+"="), strings.HasPrefix(a, "-"+name+"="):
			_, value, _ = strings.Cut(a, "=")
		default:
			rest = append(rest, a)
		}
	}
	return value, rest, true
}

// tenantFlag takes --tenant <id> from the arguments, Main without it, and returns a context of that tenant.
func tenantFlag(ctx context.Context, args []string) (context.Context, []string, error) {
	id, rest, ok := takeFlag(args, "tenant")
	if !ok {
		return ctx, nil, errors.New("--tenant needs the ID of a tenant")
	}
	if id == "" {
		id = tenant.Main
	}
	if !tenant.Valid(id) {
		return ctx, nil, tenant.ErrID
	}
	return tenant.With(ctx, id), rest, nil
}

// ownerFlag takes --owner <login> or --owner=<login> from anywhere in the arguments and returns the rest.
func ownerFlag(args []string) (owner string, rest []string, ok bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--owner" || a == "-owner":
			if i+1 >= len(args) {
				return "", nil, false
			}
			owner = args[i+1]
			i++
		case strings.HasPrefix(a, "--owner="), strings.HasPrefix(a, "-owner="):
			_, owner, _ = strings.Cut(a, "=")
		case strings.HasPrefix(a, "-") && len(a) > 1:
			return "", nil, false
		default:
			rest = append(rest, a)
		}
	}
	return owner, rest, owner != ""
}
