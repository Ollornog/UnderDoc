// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/Ollornog/UnderDoc/internal/db"
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

// export writes every collection of the database into dir, which must not exist yet or be empty, so nothing is ever
// overwritten. Private notes of every account go too: whoever runs this runs the server (docs/export.md).
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
	return nil
}

func accounts(private []db.Owned) int {
	seen := map[string]bool{}
	for _, o := range private {
		seen[o.Login] = true
	}
	return len(seen)
}

// importTree reads dir into a new collection of the account with the login owner. It never changes what exists.
func importTree(ctx context.Context, owner, dir, title string, stdout, stderr io.Writer) error {
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
	fmt.Fprintf(stdout, "imported %d objects into %q (%s), %d got a new id\n", done.Objects, done.Top.Title, done.Top.ID, len(done.Renamed))
	return nil
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
