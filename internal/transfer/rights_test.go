// SPDX-License-Identifier: MIT

package transfer

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// TestRolesGoOutAndComeBack: the role of the whole tenant and a broken inheritance survive an export and an import; a
// collection at the top that grants the tenant nothing says none, and an export from before ADR-29 means edit there.
func TestRolesGoOutAndComeBack(t *testing.T) {
	first := open(t)
	ctx := tenant.With(context.Background(), tenant.Main)
	team := shared(t, first, anna, "Team")
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

// TestEntriesOfPersonsAndGroupsGoOutAndComeBack: the entries of persons, groups, Agents and a tenant beyond its preset
// go out with names another instance knows (login, name of the group) and come back on the same objects (T-15); an
// entry whose holder the other instance does not know is left out and named, so nobody there gets more than written.
func TestEntriesOfPersonsAndGroupsGoOutAndComeBack(t *testing.T) {
	first := open(t)
	ctx := tenant.With(context.Background(), tenant.Main)
	carl := account.User{ID: "u-carl", Login: "carl", Name: "Carl"}
	if err := first.SeeAccount(ctx, carl); err != nil {
		t.Fatal(err)
	}
	admin := db.Actor{ID: anna.ID, Admin: true}
	sales := must[db.Group](t)(first.CreateGroup(ctx, admin, "Sales", "", ""))
	team := shared(t, first, anna, "Team")
	closed := must[db.Object](t)(first.CreateFolder(ctx, anna, "Closed", team.ID))
	page := must[db.Object](t)(first.CreatePage(ctx, anna, "Prices", closed.ID))
	owner := db.Actor{ID: anna.ID}
	for _, e := range []struct {
		object, kind, who string
		actions           map[string]string
		subgroups         bool
	}{
		{closed.ID, db.ToPerson, bob.ID, map[string]string{db.ActRead: db.Allow, db.ActComment: db.Allow, db.ActWrite: db.Deny}, true},
		{closed.ID, db.ToGroup, sales.ID, map[string]string{db.ActRead: db.Allow}, false},
		{closed.ID, db.ToPerson, carl.ID, map[string]string{db.ActRead: db.Deny}, true},
		{page.ID, db.ToTenant, "", map[string]string{db.ActRead: db.Allow, db.ActDelete: db.Deny}, true},
		{team.ID, db.ToAgents, "", map[string]string{db.ActWrite: db.Deny}, true},
	} {
		if err := first.SetEntries(ctx, e.object, owner, e.kind, e.who, e.actions, e.subgroups); err != nil {
			t.Fatalf("%s %s: %v", e.kind, e.who, err)
		}
	}
	exported := exportAll(t, first)
	meta := string(exported["Team/Closed/.underdoc.yaml"].Data)
	for _, want := range []string{"person: bob", "allow: [read, comment]", "deny: [write]", "group: Sales", "subgroups: false", "person: carl"} {
		if !strings.Contains(meta, want) {
			t.Errorf("the folder lacks %q:\n%s", want, meta)
		}
	}
	if top := string(exported["Team/.underdoc.yaml"].Data); !strings.Contains(top, "agents: true") || strings.Contains(top, "everyone: true") {
		t.Errorf("the top: Agents yes, the tenant as access only:\n%s", top)
	}
	if prices := string(exported["Team/Closed/Prices.md"].Data); !strings.Contains(prices, "everyone: true") || !strings.Contains(prices, "deny: [delete]") {
		t.Errorf("a tenant beyond its preset goes out as entries:\n%s", prices)
	}

	// Another instance knows bob and a group Sales, not carl.
	second := open(t)
	must[db.Group](t)(second.CreateGroup(ctx, admin, "Sales", "", ""))
	done := importDir(t, second, exported, "Team", "anna")
	if len(done.Skipped) != 1 || !strings.Contains(done.Skipped[0], `person "carl"`) {
		t.Errorf("the entries left out: %q", done.Skipped)
	}
	kids := must[[]db.Object](t)(second.Children(ctx, done.Top.ID, anna.ID))
	got := must[db.Rights](t)(second.RightsOf(ctx, kids[0].ID, owner))
	var own []string
	for _, g := range got.Own {
		own = append(own, fmt.Sprintf("%s %s %v %v", g.Kind, g.Name, g.Actions, g.Subgroups))
	}
	slices.Sort(own)
	if text := strings.Join(own, " | "); !strings.Contains(text, "person Bob map[comment:allow read:allow write:deny] true") ||
		!strings.Contains(text, "group Sales map[read:allow] false") || strings.Contains(text, "Carl") {
		t.Errorf("the entries of Closed came back as:\n%s", text)
	}
	if ok, _ := second.May(ctx, kids[0].ID, bob.ID, db.ActWrite); ok {
		t.Error("bob writes Closed, the deny was lost")
	}
	prices := must[[]db.Object](t)(second.Children(ctx, kids[0].ID, anna.ID))[0]
	if ok, _ := second.May(ctx, prices.ID, bob.ID, db.ActDelete); ok {
		t.Error("the tenant's deny of delete on Prices was lost")
	}
	if d := must[db.Decision](t)(second.Decide(ctx, done.Top.ID, anna.ID, db.ActWrite, true)); d.Allowed {
		t.Errorf("an agent of anna writes Team, the deny on Agents was lost: %+v", d)
	}
	// An import into a private collection takes no entries.
	notes := fstest.MapFS{".underdoc.yaml": {Data: []byte("title: Mine\nprivate: true\nrights:\n  - person: bob\n    allow: [read]\n")}}
	tree := must[*db.Node](t)(readOnly(notes))
	mine := must[db.Imported](t)(second.Import(ctx, "anna", "Mine", tree))
	if ok, _ := second.May(ctx, mine.Top.ID, bob.ID, db.ActRead); ok || len(mine.Skipped) != 1 {
		t.Errorf("a private import takes entries: %v %q", ok, mine.Skipped)
	}
	// Agents are only ever narrowed: an allow for them, or an action there is not, is left out and named.
	odd := fstest.MapFS{".underdoc.yaml": {Data: []byte("title: Odd\nrights:\n  - agents: true\n    allow: [write]\n  - person: bob\n    allow: [fly]\n  - group: Nobody\n    allow: [read]\n")}}
	tree = must[*db.Node](t)(readOnly(odd))
	strange := must[db.Imported](t)(second.Import(ctx, "anna", "Odd", tree))
	if len(strange.Skipped) != 3 || !strings.Contains(strings.Join(strange.Skipped, " "), "Agents") {
		t.Errorf("odd entries: %q", strange.Skipped)
	}
	if g := must[db.Rights](t)(second.RightsOf(ctx, strange.Top.ID, db.Actor{ID: anna.ID})); slices.ContainsFunc(g.Own, func(x db.Grant) bool { return x.Kind == db.ToAgents }) {
		t.Errorf("Agents got an entry: %+v", g.Own)
	}
	// An entry that names no holder, or two, is left out by the reader with a warning.
	two := fstest.MapFS{"page.md": {Data: []byte("---\nrights:\n  - person: bob\n    group: Sales\n    allow: [read]\n  - allow: [read]\n---\n# Page\n")}}
	if _, warnings, err := Read(two); err != nil || len(warnings) != 2 {
		t.Errorf("entries of two holders and of none: %v %q", err, warnings)
	}
}
