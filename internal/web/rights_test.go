// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
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
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
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
