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
	"github.com/Ollornog/UnderDoc/internal/markdown"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Main(m)) }

var (
	anna = account.User{ID: "u-anna", Login: "anna", Name: "Anna Berg"}
	bob  = account.User{ID: "u-bob", Login: "bob", Name: "Bob"}
)

func open(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(tenant.With(context.Background(), tenant.Main), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	for _, user := range []account.User{anna, bob} {
		if err := database.SeeAccount(tenant.With(context.Background(), tenant.Main), user); err != nil {
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
	shared, private, err := database.Everything(tenant.With(context.Background(), tenant.Main))
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
	done, err := database.Import(tenant.With(context.Background(), tenant.Main), login, tree.Title, tree)
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
	ctx := tenant.With(context.Background(), tenant.Main)
	first := open(t)
	team := must[db.Object](t)(first.CreateCollection(ctx, anna, "Team: ops/net", ""))
	plan := must[db.Object](t)(first.CreatePage(ctx, anna, "Plan", team.ID))
	b1 := must[db.Block](t)(first.AddBlock(ctx, plan.ID, anna.ID, "", "## Switches\n\nTwo of them."))
	b2 := must[db.Block](t)(first.AddBlock(ctx, plan.ID, anna.ID, b1.ID, "gone"))
	b3 := must[db.Block](t)(first.AddBlock(ctx, plan.ID, anna.ID, b1.ID, "```md\n# not a heading\n<!-- not a marker -->\n```"))
	b4 := must[db.Block](t)(first.AddBlock(ctx, plan.ID, anna.ID, b3.ID, "::embed{ref=\""+plan.ID+"#"+b1.ID+"\"}"))
	// Parts keep their markers and IDs (ADR-22), and what embeds them follows.
	parted := must[db.Block](t)(first.AddBlock(ctx, plan.ID, anna.ID, b4.ID, "<!-- part t1 -->Port 1<!-- /part t1 --> is **the <!-- part t2 name=\"uplink\" -->uplink** to the core<!-- /part t2 -->."))
	must[db.Block](t)(first.AddBlock(ctx, plan.ID, anna.ID, parted.ID, "See :embed{ref=\""+plan.ID+"#"+parted.ID+".t2\"}.\n\n::embed{ref=\""+plan.ID+"#"+b1.ID+".section\"}"))
	b5 := must[db.Block](t)(first.AddBlock(ctx, plan.ID, anna.ID, parted.ID, "last, deleted"))
	for _, b := range []db.Block{b2, b5} {
		if err := first.DeleteBlock(ctx, plan.ID, b.ID, anna.ID); err != nil {
			t.Fatal(err)
		}
	}
	full, side := "full", "side"
	must[db.Object](t)(first.SetView(ctx, plan.ID, anna.ID, &full, &side))
	if err := first.AlignBlock(ctx, plan.ID, b1.ID, anna.ID, "center"); err != nil {
		t.Fatal(err)
	}
	must[db.Object](t)(first.SetTemplate(ctx, plan.ID, anna.ID, true))
	kid := must[db.Object](t)(first.CreatePage(ctx, anna, "Plan", plan.ID))
	must[db.Block](t)(first.AddBlock(ctx, kid.ID, anna.ID, "", "Below the plan."))
	must[db.Object](t)(first.CreatePage(ctx, bob, "Grandchild", kid.ID))
	must[db.Object](t)(first.CreateDivider(ctx, anna, db.KindLine, team.ID, plan.ID))
	folder := must[db.Object](t)(first.CreateFolder(ctx, anna, "Plan", team.ID))
	must[db.Object](t)(first.SetAccess(ctx, folder.ID, db.Actor{ID: anna.ID}, "read"))
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
	board, _, err := first.AddNote(ctx, anna, "A card")
	if err != nil {
		t.Fatal(err)
	}
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
	if b := must[[]db.Block](t)(second.Blocks(ctx, plan.ID, anna.ID)); b[0].ID != b1.ID || b[0].Align != "center" || b[1].Align != "" {
		t.Errorf("the alignment of the blocks after the round trip: %+v", b)
	}
	if o := must[db.Object](t)(second.Object(ctx, board.ID, anna.ID)); o.Kind != db.KindNotes {
		t.Errorf("the board of quick notes after the round trip: %+v", o)
	}
	if o := must[db.Object](t)(second.Object(ctx, plan.ID, anna.ID)); o.Width != "full" || o.Contents != "side" || !o.Template {
		t.Errorf("the view of the plan after the round trip: %q %q", o.Width, o.Contents)
	}
	// What the files say is what the database has: the next block of the plan does not reuse the deleted b7.
	if b := must[db.Block](t)(second.AddBlock(ctx, plan.ID, anna.ID, "", "new")); b.ID != "b8" {
		t.Errorf("the next block after the round trip is %s", b.ID)
	}
	if got := ids(must[[]db.Block](t)(second.Blocks(ctx, plan.ID, anna.ID))); !slices.Equal(got[1:], ids(must[[]db.Block](t)(first.Blocks(ctx, plan.ID, anna.ID)))) {
		t.Errorf("the blocks of the plan: %q", got)
	}
	for _, b := range must[[]db.Block](t)(second.Blocks(ctx, plan.ID, anna.ID)) {
		if b.ID == parted.ID {
			if ps := markdown.Parts(b.Body); len(ps) != 2 || ps[0].ID != "t1" || ps[1].ID != "t2" || ps[1].Name != "uplink" {
				t.Errorf("the parts after the round trip: %+v", ps)
			}
		}
	}
	// Imported into the same database again, the plan is a copy with a new ID, and its embeddings, of parts and sections
	// too, follow it.
	copied := importDir(t, first, exported, "Team_ ops_net", "anna")
	newPlan := copied.Renamed[plan.ID]
	if newPlan == "" {
		t.Fatalf("the plan got no new ID: %+v", copied.Renamed)
	}
	var embeds string
	for _, b := range must[[]db.Block](t)(first.Blocks(ctx, newPlan, anna.ID)) {
		embeds += b.Body + "\n"
	}
	if !strings.Contains(embeds, `:embed{ref="`+newPlan+"#"+parted.ID+`.t2"}`) || !strings.Contains(embeds, `::embed{ref="`+newPlan+"#"+b1.ID+`.section"}`) || strings.Contains(embeds, plan.ID) {
		t.Errorf("the embeddings of the copy:\n%s", embeds)
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
