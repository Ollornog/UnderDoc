// SPDX-License-Identifier: MIT

package transfer

import (
	"context"
	"maps"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Main(m)) }

var (
	anna = account.User{ID: "u-anna", Login: "anna", Name: "Anna Berg"}
	bob  = account.User{ID: "u-bob", Login: "bob", Name: "Bob"}
)

func open(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	for _, user := range []account.User{anna, bob} {
		if err := database.SeeAccount(context.Background(), user); err != nil {
			t.Fatal(err)
		}
	}
	return database
}

func must[T any](t *testing.T) func(T, error) T {
	return func(v T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

// exportAll writes everything of a database into a map of files, as the command line does into a directory.
func exportAll(t *testing.T, database *db.DB) fstest.MapFS {
	t.Helper()
	shared, private, err := database.Everything(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{}
	warnings, err := Write(Tree{Shared: shared, Private: private}, func(name string, data []byte) error {
		files[name] = &fstest.MapFile{Data: data}
		return nil
	})
	if err != nil || len(warnings) > 0 {
		t.Fatalf("export: %v %q", err, warnings)
	}
	return files
}

func subtree(files fstest.MapFS, dir string) map[string]string {
	out := map[string]string{}
	for name, f := range files {
		if rest, ok := strings.CutPrefix(name, dir+"/"); ok {
			out[rest] = string(f.Data)
		}
	}
	return out
}

func importDir(t *testing.T, database *db.DB, files fstest.MapFS, dir, login string) db.Imported {
	t.Helper()
	sub, err := fsSub(files, dir)
	if err != nil {
		t.Fatal(err)
	}
	tree, warnings, err := Read(sub)
	if err != nil || len(warnings) > 0 {
		t.Fatalf("read %s: %v %q", dir, err, warnings)
	}
	done, err := database.Import(context.Background(), login, tree.Title, tree)
	if err != nil {
		t.Fatal(err)
	}
	return done
}

func fsSub(files fstest.MapFS, dir string) (fstest.MapFS, error) {
	out := fstest.MapFS{}
	for name, f := range subtree(files, dir) {
		out[name] = &fstest.MapFile{Data: []byte(f)}
	}
	return out, nil
}

// Export, then import into an empty database, then export again: the files are the same byte for byte, so the tree,
// the titles, the order with its lines and spaces, the blocks and every ID came back.
func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	first := open(t)
	team := must[db.Object](t)(first.CreateCollection(ctx, anna, "Team: ops/net", ""))
	plan := must[db.Object](t)(first.CreatePage(ctx, anna, "Plan", team.ID))
	b1 := must[db.Block](t)(first.AddBlock(ctx, plan.ID, anna.ID, "", "## Switches\n\nTwo of them."))
	b2 := must[db.Block](t)(first.AddBlock(ctx, plan.ID, anna.ID, b1.ID, "gone"))
	b3 := must[db.Block](t)(first.AddBlock(ctx, plan.ID, anna.ID, b1.ID, "```md\n# not a heading\n<!-- not a marker -->\n```"))
	b4 := must[db.Block](t)(first.AddBlock(ctx, plan.ID, anna.ID, b3.ID, "::embed{ref=\""+plan.ID+"#"+b1.ID+"\"}"))
	b5 := must[db.Block](t)(first.AddBlock(ctx, plan.ID, anna.ID, b4.ID, "last, deleted"))
	for _, b := range []db.Block{b2, b5} {
		if err := first.DeleteBlock(ctx, plan.ID, b.ID, anna.ID); err != nil {
			t.Fatal(err)
		}
	}
	kid := must[db.Object](t)(first.CreatePage(ctx, anna, "Plan", plan.ID))
	must[db.Block](t)(first.AddBlock(ctx, kid.ID, anna.ID, "", "Below the plan."))
	must[db.Object](t)(first.CreatePage(ctx, bob, "Grandchild", kid.ID))
	must[db.Object](t)(first.CreateDivider(ctx, anna, db.KindLine, team.ID, plan.ID))
	folder := must[db.Object](t)(first.CreateFolder(ctx, anna, "Plan", team.ID))
	must[db.Object](t)(first.SetAccess(ctx, folder.ID, anna.ID, "read"))
	must[db.Object](t)(first.CreateDivider(ctx, anna, db.KindSpace, team.ID, folder.ID))
	inner := must[db.Object](t)(first.CreateCollection(ctx, anna, "Inner", team.ID))
	must[db.Object](t)(first.CreatePage(ctx, anna, "CON", inner.ID))
	must[db.Object](t)(first.CreatePage(ctx, anna, "con", inner.ID))
	must[db.Object](t)(first.CreateCollection(ctx, bob, "private", ""))
	// Descriptions are made only by an import so far.
	handbook := fstest.MapFS{
		".underdoc.yaml": {Data: []byte("description: How we work\n")},
		"intro.md":       {Data: []byte("---\ndescription: |\n  Start here.\n  Two lines.\n---\n# Intro\n\nHello.\n")},
	}
	if _, err := first.Import(ctx, "anna", "Handbook", must[*db.Node](t)(readOnly(handbook))); err != nil {
		t.Fatal(err)
	}
	notes := must[[]db.Object](t)(first.Collections(ctx, anna.ID))[0]
	must[db.Object](t)(first.CreatePage(ctx, anna, "Secret", notes.ID))

	exported := exportAll(t, first)
	second := open(t)
	var tops []string
	for _, e := range must[[]string](t)(rootOrder(exported)) {
		tops = append(tops, e)
		importDir(t, second, exported, e, "anna")
	}
	if !slices.Equal(tops, []string{"Team_ ops_net", "private (2)", "Handbook"}) {
		t.Fatalf("collections at the top: %q", tops)
	}
	privateNotes := importDir(t, second, exported, "private/anna/My notes", "anna")
	if !privateNotes.Top.Private || privateNotes.Top.ID != notes.ID || len(privateNotes.Renamed) != 0 {
		t.Errorf("private notes stay private and keep their ID: %+v", privateNotes)
	}

	again := exportAll(t, second)
	for _, top := range tops {
		want, got := subtree(exported, top), subtree(again, top)
		if !maps.Equal(want, got) {
			for _, name := range slices.Sorted(maps.Keys(want)) {
				if want[name] != got[name] {
					t.Errorf("%s/%s:\n%s\nafter the round trip:\n%s", top, name, want[name], got[name])
				}
			}
			t.Errorf("%s: files %q, after the round trip %q", top, slices.Sorted(maps.Keys(want)), slices.Sorted(maps.Keys(got)))
		}
	}
	if string(exported[".underdoc.yaml"].Data) != string(again[".underdoc.yaml"].Data) {
		t.Errorf("the order at the top:\n%s\n%s", exported[".underdoc.yaml"].Data, again[".underdoc.yaml"].Data)
	}
	var restored map[string]string
	for name := range again {
		if strings.HasPrefix(name, "private/anna/") && strings.HasSuffix(name, "/Secret.md") {
			restored = subtree(again, path.Dir(name))
		}
	}
	if !maps.Equal(restored, subtree(exported, "private/anna/My notes")) {
		t.Errorf("the private notes after the round trip: %q", restored)
	}
	// What the files say is what the database has: the next block of the plan does not reuse the deleted b5.
	if b := must[db.Block](t)(second.AddBlock(ctx, plan.ID, anna.ID, "", "new")); b.ID != "b6" {
		t.Errorf("the next block after the round trip is %s", b.ID)
	}
	if got := ids(must[[]db.Block](t)(second.Blocks(ctx, plan.ID, anna.ID))); !slices.Equal(got[1:], ids(must[[]db.Block](t)(first.Blocks(ctx, plan.ID, anna.ID)))) {
		t.Errorf("the blocks of the plan: %q", got)
	}
}

func readOnly(fsys fstest.MapFS) (*db.Node, error) {
	tree, _, err := Read(fsys)
	return tree, err
}

func rootOrder(files fstest.MapFS) ([]string, error) {
	sub, err := fsSub(fstest.MapFS{"x/.underdoc.yaml": files[".underdoc.yaml"]}, "x")
	if err != nil {
		return nil, err
	}
	r := &reader{fsys: sub}
	var out []string
	for _, e := range r.meta(".").Order {
		out = append(out, e.Name)
	}
	return out, nil
}
