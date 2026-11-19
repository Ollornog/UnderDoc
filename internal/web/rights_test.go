// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/module"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// carlOf is carl, signed in through the gate with the group editors and nothing else; plain is bob with no group, so
// neither administers.
var (
	carlOf = map[string]string{"Remote-User": "carl", "Remote-Id": "u-carl", "Remote-Name": "Carl", "Remote-Groups": "editors", "Sec-Fetch-Site": "same-origin"}
	plain  = map[string]string{"Remote-User": "bob", "Remote-Id": "u-bob", "Remote-Name": "Bob", "Remote-Groups": "", "Sec-Fetch-Site": "same-origin"}
)

// TestRightsThroughTheAPI: roles to persons, groups and the tenant, breaking the inheritance, and embedding that bypasses
// nothing (ADR-29).
func TestRightsThroughTheAPI(t *testing.T) {
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	behindGate(t, database, tenant.Main)
	// Anna administers through the group of her gate headers.
	s := newSite(t, Options{DB: database, AdminGroup: "underdoc-admins"}, sample)
	// Everyone signs in once, so their accounts and groups are there.
	for _, who := range []map[string]string{plain, carlOf} {
		s.send(http.MethodGet, "/api/state", "", who)
	}
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	newPage := func(title string, blocks ...string) string {
		p := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"`+title+`","parent":"`+team.ID+`"}`, sameSite))
		after := ""
		for _, md := range blocks {
			body, _ := json.Marshal(map[string]string{"md": md, "after": after})
			after = decode[apiBlock](t, s.send(http.MethodPost, "/api/pages/"+p.ID+"/blocks", string(body), sameSite)).ID
		}
		return p.ID
	}
	salaries := newPage("Salaries", "The figures: 4711.")
	overview := newPage("Overview", "::embed{ref=\""+salaries+"\"}")
	put := func(target, body string, who map[string]string) int {
		return s.send(http.MethodPut, target, body, who).Code
	}
	// Salaries breaks away from Team and takes back what the tenant held.
	if got := put("/api/objects/"+salaries+"/inherit", `{"inherit":false}`, sameSite); got != http.StatusNoContent {
		t.Fatalf("break = %d", got)
	}
	if got := put("/api/objects/"+salaries+"/grants", `{"kind":"tenant","role":""}`, sameSite); got != http.StatusNoContent {
		t.Fatalf("take back = %d", got)
	}
	html := func(id string, who map[string]string) string {
		var out strings.Builder
		for _, b := range decode[apiDocument](t, s.send(http.MethodGet, "/api/doc/"+id, "", who)).Blocks {
			out.WriteString(b.HTML)
		}
		return out.String()
	}
	if got := s.send(http.MethodGet, "/api/doc/"+salaries, "", plain).Code; got != http.StatusNotFound {
		t.Errorf("bob does not see Salaries: %d", got)
	}
	if got := html(overview, plain); strings.Contains(got, "4711") || !strings.Contains(got, "Not to be found") {
		t.Errorf("embedding bypasses nothing: %s", got)
	}
	// The group editors, taken over from carl's sign-in, reads it; carl then sees it, also embedded.
	people := decode[struct {
		People []apiPerson `json:"people"`
		Groups []apiGroup  `json:"groups"`
	}](t, s.send(http.MethodGet, "/api/people", "", plain))
	editors := ""
	for _, g := range people.Groups {
		if g.Name == "editors" && g.Source == "synced" {
			editors = g.ID
		}
	}
	if editors == "" || len(people.People) < 3 {
		t.Fatalf("people and groups to pick from: %+v", people)
	}
	if got := put("/api/objects/"+salaries+"/grants", `{"kind":"group","who":"`+editors+`","role":"read"}`, sameSite); got != http.StatusNoContent {
		t.Fatalf("grant = %d", got)
	}
	if got := html(overview, carlOf); !strings.Contains(got, "4711") {
		t.Errorf("carl reads it through his group, also embedded: %s", got)
	}
	if got := html(overview, plain); strings.Contains(got, "4711") {
		t.Errorf("bob still does not: %s", got)
	}
	// Carl reads, so he edits nothing and manages nothing.
	if got := s.send(http.MethodPut, "/api/pages/"+salaries+"/blocks/b1", `{"md":"changed"}`, carlOf).Code; got != http.StatusForbidden {
		t.Errorf("carl edits = %d", got)
	}
	if got := put("/api/objects/"+salaries+"/grants", `{"kind":"person","who":"u-carl","role":"manage"}`, carlOf); got != http.StatusForbidden {
		t.Errorf("carl grants himself = %d", got)
	}
	rights := decode[apiRights](t, s.send(http.MethodGet, "/api/objects/"+salaries+"/rights", "", carlOf))
	if rights.Inherit || rights.Role != "read" || rights.Manage || len(rights.Own) == 0 || rights.OwnerName != "Anna Berg" {
		t.Errorf("carl reads the rights of what he sees: %+v", rights)
	}
	if got := s.send(http.MethodGet, "/api/objects/"+salaries+"/rights", "", plain).Code; got != http.StatusNotFound {
		t.Errorf("bob reads no rights of what he does not see: %d", got)
	}
	// Ownership goes to carl; anna keeps edit.
	if got := put("/api/objects/"+salaries+"/owner", `{"to":"u-carl"}`, sameSite); got != http.StatusNoContent {
		t.Fatalf("transfer = %d", got)
	}
	if rights := decode[apiRights](t, s.send(http.MethodGet, "/api/objects/"+salaries+"/rights", "", carlOf)); rights.Role != "owner" || !rights.Manage {
		t.Errorf("carl owns it: %+v", rights)
	}
	// Groups: only administrators make them; everyone else sees the groups they belong to.
	if got := s.send(http.MethodPost, "/api/groups", `{"name":"Board"}`, plain).Code; got != http.StatusForbidden {
		t.Errorf("bob makes a group = %d", got)
	}
	board := decode[apiGroup](t, s.send(http.MethodPost, "/api/groups", `{"name":"Board","description":"The board"}`, sameSite))
	if got := put("/api/groups/"+board.ID+"/members/u-bob", `{"member":true}`, sameSite); got != http.StatusNoContent {
		t.Fatalf("member = %d", got)
	}
	if got := put("/api/groups/"+editors+"/members/u-bob", `{"member":true}`, sameSite); got != http.StatusForbidden {
		t.Errorf("a group taken over can only be read = %d", got)
	}
	mine := decode[[]apiGroup](t, s.send(http.MethodGet, "/api/groups", "", plain))
	if len(mine) != 1 || mine[0].ID != board.ID || !mine[0].Mine {
		t.Errorf("bob sees his group only: %+v", mine)
	}
	if got := s.send(http.MethodGet, "/api/groups/"+editors+"/members", "", plain).Code; got != http.StatusNotFound {
		t.Errorf("bob reads the members of a group he is not in = %d", got)
	}
	if got := put("/api/objects/"+overview+"/grants", `{"kind":"group","who":"`+board.ID+`","role":"owner"}`, sameSite); got != http.StatusBadRequest {
		t.Errorf("owner is not granted = %d", got)
	}
	if got := s.send(http.MethodDelete, "/api/groups/"+board.ID, "", sameSite).Code; got != http.StatusNoContent {
		t.Errorf("delete = %d", got)
	}
}

// adminSite is a site where Anna administers through the group of her gate headers.
func adminSite(t *testing.T) (*site, *db.DB) {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	behindGate(t, database, tenant.Main)
	return newSite(t, Options{DB: database, AdminGroup: "underdoc-admins"}, sample), database
}

// TestCommentingWithoutEditing: the role comment adds comments, on the whole page and on one block, and changes nothing
// (ADR-29, ADR-18).
func TestCommentingWithoutEditing(t *testing.T) {
	s := withDatabase(t)
	s.send(http.MethodGet, "/api/state", "", plain)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	s.send(http.MethodPost, "/api/objects/"+team.ID+"/access", `{"access":"read"}`, sameSite)
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Plan","parent":"`+team.ID+`"}`, sameSite))
	s.send(http.MethodPost, "/api/pages/"+page.ID+"/blocks", `{"md":"The plan."}`, sameSite)
	if got := s.send(http.MethodPost, "/api/pages/"+page.ID+"/comments", `{"body":"before","block":"b1"}`, plain).Code; got != http.StatusForbidden {
		t.Fatalf("reading only, no comment: %d", got)
	}
	if got := s.send(http.MethodPut, "/api/objects/"+page.ID+"/grants", `{"kind":"person","who":"u-bob","role":"comment"}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("grant = %d", got)
	}
	doc := decode[apiDocument](t, s.send(http.MethodGet, "/api/doc/"+page.ID, "", plain))
	if !doc.CanComment || doc.CanEdit {
		t.Errorf("bob comments and does not edit: %+v", doc)
	}
	if got := s.send(http.MethodPost, "/api/pages/"+page.ID+"/comments", `{"body":"on the block","block":"b1"}`, plain).Code; got != http.StatusCreated {
		t.Errorf("a comment on a block = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/pages/"+page.ID+"/blocks/b1", `{"md":"changed"}`, plain).Code; got != http.StatusForbidden {
		t.Errorf("bob edits = %d", got)
	}
	if !slices.Contains(doc.SeenBy, "Bob (comment)") || !slices.Contains(doc.SeenBy, "everyone (read)") || !slices.Contains(doc.SeenBy, "Anna Berg (owner)") {
		t.Errorf("the facts name who sees it: %v", doc.SeenBy)
	}
}

// TestTheAccessCheck: what may X do and who sees Y, for administrators about anyone and for everyone about themselves.
func TestTheAccessCheck(t *testing.T) {
	s, _ := adminSite(t)
	s.send(http.MethodGet, "/api/state", "", plain)
	s.send(http.MethodGet, "/api/state", "", carlOf)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	s.send(http.MethodPut, "/api/objects/"+team.ID+"/grants", `{"kind":"person","who":"u-bob","role":"manage"}`, sameSite)
	check := decode[struct {
		Role    string      `json:"role"`
		Reasons []apiReason `json:"reasons"`
	}](t, s.get("/api/rights/check?object="+team.ID+"&account=u-bob"))
	if check.Role != "manage" || len(check.Reasons) != 2 {
		t.Errorf("bob manages Team, as a person, and edits as everyone: %+v", check)
	}
	held := decode[[]apiHeld](t, s.get("/api/rights/held?account=u-bob"))
	if len(held) != 2 {
		t.Errorf("what bob holds: %+v", held)
	}
	if got := s.send(http.MethodGet, "/api/rights/held?account=u-carl", "", plain).Code; got != http.StatusForbidden {
		t.Errorf("bob asks about carl = %d", got)
	}
	if got := s.send(http.MethodGet, "/api/rights/held", "", plain).Code; got != http.StatusOK {
		t.Errorf("bob asks about himself = %d", got)
	}
	if got := s.send(http.MethodGet, "/api/rights/check?object="+team.ID+"&account=u-carl", "", carlOf).Code; got != http.StatusOK {
		t.Errorf("carl asks about himself = %d", got)
	}
	if got := s.send(http.MethodGet, "/api/rights/check?object="+team.ID+"&account=u-bob", "", carlOf).Code; got != http.StatusForbidden {
		t.Errorf("carl, who only edits Team, asks about bob = %d", got)
	}
	if got := s.send(http.MethodGet, "/api/users", "", plain).Code; got != http.StatusForbidden {
		t.Errorf("bob lists the users = %d", got)
	}
}

// TestTakingBackAnAdministratorThroughTheAPI: taken back, a local administrator stops administering the organization of
// the time tracking at once (ADR-29).
func TestTakingBackAnAdministratorThroughTheAPI(t *testing.T) {
	s, database := adminSite(t)
	ctx := tenant.With(context.Background(), tenant.Main)
	if err := database.SetTenantModules(ctx, []string{module.Documents, module.Time}); err != nil {
		t.Fatal(err)
	}
	hash := []byte(strings.Repeat("d", 32))
	if err := database.CreateInvite(ctx, hash, db.Invite{Login: "dora", Name: "Dora", Admin: true, Expires: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	dora, err := database.AcceptInvite(ctx, hash, db.NewLocal{PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.MapTenant(ctx, dora.ID, true); err != nil {
		t.Fatal(err)
	}
	orgAdmin := func() bool {
		ms, err := database.TimeStore().Memberships(ctx, tenant.Main, db.Person(dora.ID))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range ms {
			if m.Role == "admin" && m.Until.IsZero() {
				return true
			}
		}
		return false
	}
	if !orgAdmin() {
		t.Fatal("dora administers the organization")
	}
	users := decode[[]apiAccount](t, s.get("/api/users"))
	if len(users) != 2 {
		t.Errorf("the users: %+v", users)
	}
	if got := s.send(http.MethodPut, "/api/users/"+dora.ID+"/admin", `{"admin":false}`, plain).Code; got != http.StatusForbidden {
		t.Errorf("bob takes it back = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/users/u-7/admin", `{"admin":false}`, sameSite).Code; got != http.StatusConflict {
		t.Errorf("anna is an administrator through her group = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/users/"+dora.ID+"/admin", `{"admin":false}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("take back = %d", got)
	}
	if orgAdmin() {
		t.Error("dora still administers the organization")
	}
}

// sharedCollection makes a collection at the top and grants the whole tenant edit on it, as its owner does in the step
// "Share with" of the dialog: what a new collection at the top did by itself before 2026-11-19 (ADR-29). The tests
// written before that keep their meaning through it.
func (s *site) sharedCollection(body string, headers map[string]string) apiNode {
	s.t.Helper()
	made := decode[apiNode](s.t, s.send(http.MethodPost, "/api/collections", body, headers))
	if got := s.send(http.MethodPut, "/api/objects/"+made.ID+"/grants", `{"kind":"tenant","role":"edit"}`, headers); got.Code != http.StatusNoContent {
		s.t.Fatalf("sharing %s with the tenant: %d %s", made.Title, got.Code, got.Body.String())
	}
	return made
}

// sharedCollection is the same for a client of a tenant.
func (c *client) sharedCollection(body string) apiNode {
	c.t.Helper()
	made := decode[apiNode](c.t, c.post("/api/collections", body))
	if got := c.do(http.MethodPut, "/api/objects/"+made.ID+"/grants", "application/json", `{"kind":"tenant","role":"edit"}`); got.Code != http.StatusNoContent {
		c.t.Fatalf("sharing %s with the tenant: %d %s", made.Title, got.Code, got.Body.String())
	}
	return made
}
