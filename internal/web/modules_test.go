// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/module"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

func stateOf(t *testing.T, c *client) apiState {
	t.Helper()
	return decode[apiState](t, c.get("/api/state"))
}

func documents(states []apiModule) apiModule {
	for _, m := range states {
		if m.ID == module.Documents {
			return m
		}
	}
	return apiModule{}
}

// TestModulesSwitchOffWithoutLoss (ADR-25): the administrator switches Documents off, and its entries, addresses and
// API are gone for everyone, but nothing of it is deleted: switched on again, everything is back.
func TestModulesSwitchOffWithoutLoss(t *testing.T) {
	s, _ := dbSite(t, Options{})
	admin := newClientWith(t, s, gateHeaders)
	other := newClientWith(t, s, bob)
	team := admin.sharedCollection(`{"title":"Team"}`)
	plan := decode[apiNode](t, admin.post("/api/pages", `{"title":"Plan","parent":"`+team.ID+`"}`))
	admin.post("/api/pages/"+plan.ID+"/blocks", `{"md":"Kept while off."}`)
	if m := documents(stateOf(t, admin).Modules); !m.Allowed || !m.On || m.Hidden {
		t.Fatalf("on by default: %+v", m)
	}
	if got := other.get("/admin/modules"); got.Code != http.StatusNotFound {
		t.Errorf("not an administrator: %d", got.Code)
	}
	if got := other.form("/admin/modules", url.Values{}); got.Code != http.StatusNotFound {
		t.Errorf("not an administrator switches: %d", got.Code)
	}
	if got := admin.form("/admin/modules", url.Values{}); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "Nothing was deleted") {
		t.Fatalf("switch off: %d", got.Code)
	}
	for _, c := range []*client{admin, other} {
		st := stateOf(t, c)
		if len(st.Collections) != 0 || st.CanCreate || documents(st.Modules).On {
			t.Errorf("entries of Documents while off: %+v", st)
		}
	}
	for _, target := range []string{"/o/" + plan.ID, "/api/doc/" + plan.ID, "/api/trash", "/api/search?q=kept", "/o/start", "/f/ops", "/files/ops/net/map.png", "/d/" + plan.ID} {
		if got := admin.get(target); got.Code != http.StatusNotFound {
			t.Errorf("%s while off: %d", target, got.Code)
		}
	}
	if got := admin.post("/api/collections", `{"title":"New"}`); got.Code != http.StatusNotFound {
		t.Errorf("making while off: %d", got.Code)
	}
	for _, target := range []string{"/", "/profile", "/api/state"} {
		if got := admin.get(target); got.Code != http.StatusOK {
			t.Errorf("%s answers while off: %d", target, got.Code)
		}
	}
	if got := admin.form("/admin/modules", url.Values{"module": {module.Documents}}); got.Code != http.StatusOK {
		t.Fatalf("switch on: %d", got.Code)
	}
	doc := decode[apiDocument](t, admin.get("/api/doc/"+plan.ID))
	if len(doc.Blocks) != 1 || !strings.Contains(doc.Blocks[0].HTML, "Kept while off.") {
		t.Errorf("everything is back: %+v", doc)
	}
	if findNode(stateOf(t, other).Collections, team.ID) == nil {
		t.Error("the others see it again")
	}
}

// TestTheHosterDecidesWhatATenantMayUse: a module the hoster does not allow is off whatever the tenant switches, and
// what the instance does not allow is off for everyone.
func TestTheHosterDecidesWhatATenantMayUse(t *testing.T) {
	s, database := dbSite(t, Options{})
	admin := newClientWith(t, s, gateHeaders)
	if err := database.SetTenantLimits(context.Background(), tenant.Main, db.HosterLimits{Modules: []string{}}); err != nil {
		t.Fatal(err)
	}
	admin.form("/admin/modules", url.Values{"module": {module.Documents}})
	if m := documents(stateOf(t, admin).Modules); m.Allowed || m.On {
		t.Errorf("not allowed by the hoster: %+v", m)
	}
	if page := admin.get("/admin/modules").Body.String(); !strings.Contains(page, "Not offered by your hoster") {
		t.Error("the page says why")
	}
	if got := admin.get("/o/start"); got.Code != http.StatusNotFound {
		t.Errorf("an address of a module not allowed: %d", got.Code)
	}
	if err := database.SetTenantLimits(context.Background(), tenant.Main, db.HosterLimits{}); err != nil {
		t.Fatal(err)
	}
	none, err := module.ParsePolicy(",")
	if err != nil {
		t.Fatal(err)
	}
	strict := newClientWith(t, newSite(t, Options{DB: database, Modules: none}, sample), gateHeaders)
	if m := documents(stateOf(t, strict).Modules); m.Allowed || m.On {
		t.Errorf("not allowed by the instance: %+v", m)
	}
	if err := database.SetTenantModules(inMain(), []string{module.Documents}); err != nil {
		t.Fatal(err)
	}
	off, _ := module.ParsePolicy("documents:off")
	fresh := newClientWith(t, newSite(t, Options{DB: database, Modules: off}, sample), gateHeaders)
	if m := documents(stateOf(t, fresh).Modules); !m.Allowed || !m.On {
		t.Errorf("the switch of the tenant counts over the default of the instance: %+v", m)
	}
}

// TestUsersHideModules: a user hides Documents for themselves; the others keep it, and what is hidden still answers.
func TestUsersHideModules(t *testing.T) {
	s, _ := dbSite(t, Options{})
	anna := newClientWith(t, s, gateHeaders)
	other := newClientWith(t, s, bob)
	team := anna.sharedCollection(`{"title":"Team"}`)
	if got := anna.form("/account/modules", url.Values{"hide": {module.Documents, "nonsense"}}); got.Code != http.StatusOK {
		t.Fatalf("hide: %d", got.Code)
	}
	st := stateOf(t, anna)
	if m := documents(st.Modules); !m.On || !m.Hidden || len(st.Collections) != 0 {
		t.Errorf("hidden for anna: %+v %d", m, len(st.Collections))
	}
	if got := anna.get("/api/doc/" + team.ID); got.Code != http.StatusOK {
		t.Errorf("hidden is not off: %d", got.Code)
	}
	if findNode(stateOf(t, other).Collections, team.ID) == nil {
		t.Error("bob still sees it")
	}
	if page := anna.get("/account/modules").Body.String(); !strings.Contains(page, `value="documents" checked`) {
		t.Error("the page shows what is hidden")
	}
	anna.form("/account/modules", url.Values{})
	if findNode(stateOf(t, anna).Collections, team.ID) == nil {
		t.Error("shown again")
	}
}

// TestTheTrashJobFollowsDocuments: where Documents is off, its job leaves the trash as it is.
func TestTheTrashJobFollowsDocuments(t *testing.T) {
	s, database := dbSite(t, Options{})
	anna := newClientWith(t, s, gateHeaders)
	team := anna.sharedCollection(`{"title":"Team"}`)
	anna.do(http.MethodDelete, "/api/objects/"+team.ID, "", "")
	policy, _ := module.ParsePolicy("")
	on := ModuleOn(database, policy, module.Documents)
	if err := database.SetTenantModules(inMain(), []string{}); err != nil {
		t.Fatal(err)
	}
	if gone, err := database.ExpireAllTrash(context.Background(), time.Now().Add(time.Hour), on); err != nil || gone != 0 {
		t.Errorf("off: %d gone, %v", gone, err)
	}
	if err := database.SetTenantModules(inMain(), []string{module.Documents}); err != nil {
		t.Fatal(err)
	}
	if gone, err := database.ExpireAllTrash(context.Background(), time.Now().Add(time.Hour), on); err != nil || gone != 1 {
		t.Errorf("on: %d gone, %v", gone, err)
	}
}

// TestFilesSwitchOffWithoutLoss: the module Files switched off has no entry and no address, its uploads are refused,
// and switched on again its roots are all there.
func TestFilesSwitchOffWithoutLoss(t *testing.T) {
	f := withFiles(t)
	anna := newClientWith(t, f.site, gateHeaders)
	if st := stateOf(t, anna); !st.Files {
		t.Fatal("Files is on by default")
	}
	before := anna.get("/api/files")
	if before.Code != http.StatusOK {
		t.Fatalf("roots: %d", before.Code)
	}
	if err := f.db.SetTenantModules(inMain(), []string{module.Documents}); err != nil {
		t.Fatal(err)
	}
	if st := stateOf(t, anna); st.Files {
		t.Error("an entry of Files while off")
	}
	for _, target := range []string{"/api/files", "/api/uploads/x"} {
		if got := anna.get(target); got.Code != http.StatusNotFound {
			t.Errorf("%s while off: %d", target, got.Code)
		}
	}
	if got := anna.post("/api/uploads", `{"root":"x","name":"a.txt","size":1}`); got.Code != http.StatusNotFound {
		t.Errorf("an upload while off: %d", got.Code)
	}
	if got := anna.get("/api/state"); got.Code != http.StatusOK {
		t.Errorf("the workspace answers: %d", got.Code)
	}
	if err := f.db.SetTenantModules(inMain(), []string{module.Documents, module.Files}); err != nil {
		t.Fatal(err)
	}
	if after := anna.get("/api/files"); after.Code != http.StatusOK || after.Body.String() != before.Body.String() {
		t.Errorf("the roots are back unchanged: %d", after.Code)
	}
}
