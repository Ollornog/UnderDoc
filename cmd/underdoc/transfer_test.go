// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Main(m)) }

func inMain() context.Context { return tenant.With(context.Background(), tenant.Main) }

func runCommand(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestTransferUsage(t *testing.T) {
	for _, args := range [][]string{{"export"}, {"export", "a", "b"}, {"import"}, {"import", "--owner", "anna", "dir"},
		{"import", "dir", "title"}, {"import", "--owner"}, {"import", "--bogus", "x", "dir", "title"}, {"import", "--owner", "anna", "a", "b", "c"},
		{"export", "--tenant"}, {"export", "--tenant", "Bad", "dir"}, {"import", "--tenant", "-x", "--owner", "anna", "d", "t"}, {"tenant", "add", "x"}} {
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
	database, err := db.Open(inMain(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if err := database.SeeAccount(inMain(), account.User{ID: "u-anna", Login: "anna", Name: "Anna"}); err != nil {
		t.Fatal(err)
	}
	return database, url
}

// The command line round trip: export, import into another database, export again, and the collection is the same.
func TestExportAndImport(t *testing.T) {
	ctx := inMain()
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
	// --archived brings the space in archived (T-34).
	code, stdout, _ = runCommand(t, "import", "--archived", "--owner", "anna", filepath.Join(out, "Team"), "Team archived")
	if code != 0 || !strings.Contains(stdout, ", archived)") {
		t.Errorf("an import into the archive: exit %d, %q", code, stdout)
	}
	if code, stdout, _ := runCommand(t, "import", "--owner", "anna", filepath.Join(out, "Team"), "Team plain"); code != 0 || strings.Contains(stdout, "archived") {
		t.Errorf("an import without --archived: exit %d, %q", code, stdout)
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

// Links between imported pages point at the pages they became: relative links to Markdown files and [[links]] by ID,
// file name or title; what finds nothing, and what stands in fenced code, stays as written. An embedding of an imported
// page follows it to its new ID.
func TestImportedLinksFollowThePages(t *testing.T) {
	ctx := inMain()
	store, url := database(t)
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a/one.md", "# One\n\nSee [two](../b/two.md#part), [[Two]], [[two|the second]], [[T-3#Part B]], [[missing]] and [web](https://example.com/x.md).\n\n```\n[[Two]]\n```\n\n::embed{ref=\"T-3\"}\n")
	write("b/two.md", "---\nid: T-3\n---\n# Two\n\n## Part B\n\ntext\n")
	t.Setenv("UNDERDOC_DATABASE_URL", url)
	if code, stdout, errOut := runCommand(t, "import", "--owner", "anna", dir, "Docs"); code != 0 {
		t.Fatalf("import: exit %d, %q %q", code, stdout, errOut)
	}
	var one, two string
	var walk func(id string)
	walk = func(id string) {
		kids, _ := store.Children(ctx, id, "u-anna")
		for _, k := range kids {
			switch k.Title {
			case "One":
				one = k.ID
			case "Two":
				two = k.ID
			}
			walk(k.ID)
		}
	}
	tops, _ := store.Collections(ctx, "u-anna")
	for _, c := range tops {
		if c.Title == "Docs" {
			walk(c.ID)
		}
	}
	if one == "" || two == "" || two == "T-3" {
		t.Fatalf("pages: one %q two %q", one, two)
	}
	blocks, _ := store.Blocks(ctx, one, "u-anna")
	var body strings.Builder
	for _, b := range blocks {
		body.WriteString(b.Body + "\n")
	}
	got := body.String()
	for _, want := range []string{"[two](/o/" + two + "#part)", "[Two](/o/" + two + ")", "[the second](/o/" + two + ")", "[Two](/o/" + two + "#h-part-b)",
		"[[missing]]", "[web](https://example.com/x.md)", "```\n[[Two]]\n```", `::embed{ref="` + two + `"}`} {
		if !strings.Contains(got, want) {
			t.Errorf("%q is not in:\n%s", want, got)
		}
	}
	if refs, _ := store.UsedIn(ctx, two, "u-anna"); len(refs) != 1 || refs[0].Page != one {
		t.Errorf("the embedding is kept as a reference: %+v", refs)
	}
}

// Tags come in with the import, from a list or a line with commas, are kept as the database keeps them, and go out again
// with the export; tags it cannot keep are named in the warnings.
func TestTagsComeAndGo(t *testing.T) {
	ctx := inMain()
	store, url := database(t)
	dir := t.TempDir()
	for name, content := range map[string]string{
		"one.md":             "---\ntags: [Network, rack]\n---\n# One\n\ntext\n",
		"two.md":             "---\ntags: vlan, Main-Site\n---\n# Two\n\ntext\n",
		"three.md":           "---\ntags: [two words]\n---\n# Three\n\ntext\n",
		"sub/.underdoc.yaml": "tags: [folder]\n",
		"sub/four.md":        "# Four\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("UNDERDOC_DATABASE_URL", url)
	code, _, errOut := runCommand(t, "import", "--owner", "anna", dir, "Tagged")
	if code != 0 || !strings.Contains(errOut, "three.md: tags left out") {
		t.Fatalf("import: exit %d, %q", code, errOut)
	}
	tops, _ := store.Collections(ctx, "u-anna")
	got := map[string][]string{}
	for _, c := range tops {
		if c.Title != "Tagged" {
			continue
		}
		kids, _ := store.Children(ctx, c.ID, "u-anna")
		for _, k := range kids {
			got[k.Title] = k.Tags
		}
	}
	if !slices.Equal(got["One"], []string{"network", "rack"}) || !slices.Equal(got["Two"], []string{"vlan", "main-site"}) || len(got["Three"]) != 0 || !slices.Equal(got["sub"], []string{"folder"}) {
		t.Errorf("tags in the database: %q", got)
	}
	out := filepath.Join(t.TempDir(), "export")
	if code, _, errOut := runCommand(t, "export", out); code != 0 {
		t.Fatalf("export: %q", errOut)
	}
	exported := files(t, out)
	if !strings.Contains(exported["Tagged/One.md"], "tags: [network, rack]\n") || !strings.Contains(exported["Tagged/sub/.underdoc.yaml"], "tags: [folder]\n") {
		t.Errorf("the export writes them:\n%s\n%s", exported["Tagged/One.md"], exported["Tagged/sub/.underdoc.yaml"])
	}
	if strings.Contains(exported["Tagged/Three.md"], "tags") {
		t.Errorf("no tags, no line:\n%s", exported["Tagged/Three.md"])
	}
}

// TestTransferPerTenant: export and import work on the tenant named by --tenant and nothing else, and tenant add makes
// a tenant to import into.
func TestTransferPerTenant(t *testing.T) {
	first, url := database(t)
	t.Setenv("UNDERDOC_DATABASE_URL", url)
	if code, stdout, errOut := runCommand(t, "tenant", "add", "acme", "Acme Ltd"); code != 0 || !strings.Contains(stdout, "tenant acme") {
		t.Fatalf("tenant add: %d %q %q", code, stdout, errOut)
	}
	if code, _, errOut := runCommand(t, "tenant", "add", "acme", "Again"); code != 1 || !strings.Contains(errOut, "taken") {
		t.Errorf("tenant add twice: %d %q", code, errOut)
	}
	anna := account.User{ID: "u-anna", Login: "anna"}
	acme := tenant.With(context.Background(), "acme")
	if err := first.SeeAccount(acme, anna); err != nil {
		t.Fatal(err)
	}
	if _, err := first.CreateCollection(inMain(), anna, "Main only", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := first.CreateCollection(acme, anna, "Acme only", ""); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "acme")
	if code, _, errOut := runCommand(t, "export", "--tenant", "acme", out); code != 0 {
		t.Fatalf("export acme: %q", errOut)
	}
	exported := files(t, out)
	if _, ok := exported["Acme only/.underdoc.yaml"]; !ok {
		t.Errorf("acme's collection is missing: %v", slices.Collect(maps.Keys(exported)))
	}
	for name := range exported {
		if strings.HasPrefix(name, "Main only") {
			t.Errorf("the export of acme holds %s of Main", name)
		}
	}
	if code, _, errOut := runCommand(t, "import", "--tenant", "acme", "--owner", "anna", filepath.Join(out, "Acme only"), "Back"); code != 0 {
		t.Fatalf("import into acme: %q", errOut)
	}
	if code, _, errOut := runCommand(t, "import", "--tenant", "nobody", "--owner", "anna", filepath.Join(out, "Acme only"), "Back"); code != 1 || !strings.Contains(errOut, "no such account") {
		t.Errorf("import into a tenant that does not exist: %d %q", code, errOut)
	}
	list, err := first.Collections(inMain(), anna.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range list {
		if c.Title == "Back" || c.Title == "Acme only" {
			t.Errorf("the import into acme landed in Main: %+v", c)
		}
	}
}

// TestEmergencyAccess: whoever runs the instance marks the account of a lasting ID of the gate as an administrator, also
// before its first sign-in, takes the mark back, and binds an account made before ADR-32 to its person (ADR-32). The web
// marks nobody.
func TestEmergencyAccess(t *testing.T) {
	first, url := database(t)
	t.Setenv("UNDERDOC_DATABASE_URL", url)
	code, stdout, errOut := runCommand(t, "admin", "grant", "77")
	if code != 0 || !strings.Contains(stdout, "lasting ID 77, is marked as an administrator") {
		t.Fatalf("%d %q %q", code, stdout, errOut)
	}
	a, err := first.AccountByID(inMain(), "77")
	if err != nil || !a.Admin || a.RemoteID != "77" {
		t.Errorf("the marked account: %+v %v", a, err)
	}
	if code, stdout, _ := runCommand(t, "account", "list"); code != 0 || !strings.Contains(stdout, "77") || !strings.Contains(stdout, "✓") ||
		!strings.Contains(stdout, "– not bound") {
		t.Errorf("the list shows the mark and anna, not bound: %d %q", code, stdout)
	}
	if code, _, _ := runCommand(t, "admin", "revoke", "77"); code != 0 {
		t.Errorf("revoke: %d", code)
	}
	if a, _ := first.AccountByID(inMain(), "77"); a.Admin {
		t.Error("the mark stays after revoke")
	}
	if code, _, errOut := runCommand(t, "admin", "grant", "anna"); code != 1 || !strings.Contains(errOut, "number") {
		t.Errorf("a login instead of the lasting ID: %d %q", code, errOut)
	}
	if code, stdout, errOut := runCommand(t, "account", "bind", "anna", "78"); code != 0 || !strings.Contains(stdout, "bound to the lasting ID 78") {
		t.Errorf("bind anna: %d %q %q", code, stdout, errOut)
	}
	if code, _, errOut := runCommand(t, "account", "bind", "anna", "79"); code != 1 || !strings.Contains(errOut, "bound already") {
		t.Errorf("anna bound to another ID: %d %q", code, errOut)
	}
	if code, _, errOut := runCommand(t, "account", "bind", "77", "78"); code != 1 || !strings.Contains(errOut, "bound already") {
		t.Errorf("an ID bound to another account: %d %q", code, errOut)
	}
	if code, _, _ := runCommand(t, "admin", "grant", "--tenant", "nobody", "80"); code == 0 {
		t.Errorf("an unknown tenant: %d", code)
	}
	for _, args := range [][]string{{"admin", "grant"}, {"admin", "make", "77"}, {"account", "bind", "anna"}, {"invite", "kim", "Kim"}} {
		if code, _, _ := runCommand(t, args...); code != 2 {
			t.Errorf("usage of %v: %d", args, code)
		}
	}
}
