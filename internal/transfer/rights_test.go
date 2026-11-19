// SPDX-License-Identifier: MIT

package transfer

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// TestRolesGoOutAndComeBack: the role of the whole tenant and a broken inheritance survive an export and an import; a
// collection at the top that grants the tenant nothing says none, and an export from before ADR-29 means edit there.
func TestRolesGoOutAndComeBack(t *testing.T) {
	first := open(t)
	ctx := tenant.With(context.Background(), tenant.Main)
	team := must[db.Object](t)(first.CreateCollection(ctx, anna, "Team", ""))
	closed := must[db.Object](t)(first.CreateFolder(ctx, anna, "Closed", team.ID))
	must[db.Object](t)(first.SetAccess(ctx, closed.ID, db.Actor{ID: anna.ID}, "read"))
	shut := must[db.Object](t)(first.CreateCollection(ctx, anna, "Shut", ""))
	must[db.Object](t)(first.SetAccess(ctx, shut.ID, db.Actor{ID: anna.ID, Admin: true}, ""))
	if err := first.SetGrant(ctx, shut.ID, db.Actor{ID: anna.ID}, db.ToTenant, "", db.RoleNone); err != nil {
		t.Fatal(err)
	}
	exported := exportAll(t, first)
	if meta := string(exported["Team/Closed/.underdoc.yaml"].Data); !strings.Contains(meta, "access: read") || !strings.Contains(meta, "inherit: false") {
		t.Errorf("the folder says it breaks the inheritance:\n%s", meta)
	}
	if meta := string(exported["Shut/.underdoc.yaml"].Data); !strings.Contains(meta, "access: none") {
		t.Errorf("the collection says it grants the tenant nothing:\n%s", meta)
	}
	second := open(t)
	done := importDir(t, second, exported, "Team", "anna")
	kids := must[[]db.Object](t)(second.Children(ctx, done.Top.ID, anna.ID))
	if len(kids) != 1 || !kids[0].Breaks || kids[0].Access != "read" {
		t.Fatalf("the folder comes back breaking the inheritance: %+v", kids)
	}
	if got := must[string](t)(second.Role(ctx, kids[0].ID, bob.ID)); got != db.RoleRead {
		t.Errorf("bob reads it only: %q", got)
	}
	if done := importDir(t, second, exported, "Shut", "anna"); must[string](t)(second.Role(ctx, done.Top.ID, bob.ID)) != db.RoleNone {
		t.Errorf("Shut comes back shut")
	}
	old := fstest.MapFS{".underdoc.yaml": {Data: []byte("title: Old\n")}, "page.md": {Data: []byte("# Page\n\nText.\n")}}
	tree := must[*db.Node](t)(readOnly(old))
	made := must[db.Imported](t)(second.Import(ctx, "anna", "Old", tree))
	if got := must[string](t)(second.Role(ctx, made.Top.ID, bob.ID)); got != db.RoleEdit {
		t.Errorf("an export from before ADR-29 lets the tenant edit at the top: %q", got)
	}
}
