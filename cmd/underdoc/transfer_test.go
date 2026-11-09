// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Main(m)) }

func runCommand(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestTransferUsage(t *testing.T) {
	for _, args := range [][]string{{"export"}, {"export", "a", "b"}, {"import"}, {"import", "--owner", "anna", "dir"},
		{"import", "dir", "title"}, {"import", "--owner"}, {"import", "--bogus", "x", "dir", "title"}, {"import", "--owner", "anna", "a", "b", "c"}} {
		if code, _, errOut := runCommand(t, args...); code != 2 || !strings.Contains(errOut, "usage:") {
			t.Errorf("%v: exit %d, stderr %q", args, code, errOut)
		}
	}
}

func TestOwnerFlag(t *testing.T) {
	for _, args := range [][]string{{"--owner", "anna", "d", "t"}, {"d", "--owner=anna", "t"}, {"d", "t", "-owner", "anna"}} {
		if owner, rest, ok := ownerFlag(args); !ok || owner != "anna" || strings.Join(rest, " ") != "d t" {
			t.Errorf("%v: %q %q %v", args, owner, rest, ok)
		}
	}
}

func TestTransferNeedsADatabase(t *testing.T) {
	t.Setenv("UNDERDOC_DATABASE_URL", "")
	if code, _, errOut := runCommand(t, "export", filepath.Join(t.TempDir(), "out")); code != 1 || !strings.Contains(errOut, "UNDERDOC_DATABASE_URL") {
		t.Errorf("export: exit %d, stderr %q", code, errOut)
	}
	if code, _, errOut := runCommand(t, "import", "--owner", "anna", t.TempDir(), "X"); code != 1 || !strings.Contains(errOut, "UNDERDOC_DATABASE_URL") {
		t.Errorf("import: exit %d, stderr %q", code, errOut)
	}
	if code, _, _ := runCommand(t, "import", "--owner", "anna", filepath.Join(t.TempDir(), "gone"), "X"); code != 1 {
		t.Errorf("import of a missing folder: exit %d", code)
	}
}

func files(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func database(t *testing.T) (*db.DB, string) {
	t.Helper()
	url := pgtest.URL(t)
	database, err := db.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if err := database.SeeAccount(context.Background(), account.User{ID: "u-anna", Login: "anna", Name: "Anna"}); err != nil {
		t.Fatal(err)
	}
	return database, url
}

// The command line round trip: export, import into another database, export again, and the collection is the same.
func TestExportAndImport(t *testing.T) {
	ctx := context.Background()
	first, url := database(t)
	anna := account.User{ID: "u-anna", Login: "anna"}
	team, _ := first.CreateCollection(ctx, anna, "Team", "")
	plan, _ := first.CreatePage(ctx, anna, "Plan", team.ID)
	if _, err := first.AddBlock(ctx, plan.ID, anna.ID, "", "## Switches"); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "export")
	t.Setenv("UNDERDOC_DATABASE_URL", url)
	code, stdout, errOut := runCommand(t, "export", out)
	if code != 0 || !strings.Contains(stdout, "exported 1 collections and the private notes of 1 accounts") {
		t.Fatalf("export: exit %d, %q %q", code, stdout, errOut)
	}
	exported := files(t, out)
	if page := exported["Team/Plan.md"]; !strings.Contains(page, "id: "+plan.ID) || !strings.Contains(page, "<!-- block b1 -->\n## Switches\n") {
		t.Errorf("the page:\n%s", page)
	}
	if _, ok := exported["private/anna/My notes/.underdoc.yaml"]; !ok {
		t.Errorf("the private notes: %q", exported)
	}
	if code, _, errOut := runCommand(t, "export", out); code != 1 || !strings.Contains(errOut, "not empty") {
		t.Errorf("a second export into the same folder: exit %d, %q", code, errOut)
	}
	if code, _, errOut := runCommand(t, "import", "--owner", "nobody", filepath.Join(out, "Team"), "Team"); code != 1 || !strings.Contains(errOut, "no such account") {
		t.Errorf("an unknown owner: exit %d, %q", code, errOut)
	}
	code, stdout, errOut = runCommand(t, "import", "--owner", "anna", filepath.Join(out, "Team"), "Team again")
	if code != 0 || !strings.Contains(stdout, `imported 2 objects into "Team again"`) || !strings.Contains(stdout, "2 got a new id") || !strings.Contains(errOut, plan.ID+" was taken") {
		t.Errorf("into the same database, every ID is taken: exit %d, %q %q", code, stdout, errOut)
	}

	_, other := database(t)
	t.Setenv("UNDERDOC_DATABASE_URL", other)
	code, stdout, errOut = runCommand(t, "import", filepath.Join(out, "Team"), "Team", "--owner", "anna")
	if code != 0 || !strings.Contains(stdout, "0 got a new id") || errOut != "" {
		t.Fatalf("into an empty database: exit %d, %q %q", code, stdout, errOut)
	}
	again := filepath.Join(t.TempDir(), "again")
	if code, _, errOut := runCommand(t, "export", again); code != 0 {
		t.Fatalf("export again: %q", errOut)
	}
	back := files(t, again)
	for name, want := range exported {
		if strings.HasPrefix(name, "Team/") && back[name] != want {
			t.Errorf("%s after the round trip:\n%s\nwant\n%s", name, back[name], want)
		}
	}
}
