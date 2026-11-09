// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"strings"
	"testing"
)

func TestDividersAndMovingThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
	a := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"A","parent":"`+team.ID+`"}`, sameSite))
	b := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"B","parent":"`+team.ID+`"}`, sameSite))
	if got := s.send(http.MethodPost, "/api/dividers", `{"kind":"line","parent":"`+team.ID+`","after":"`+a.ID+`"}`, map[string]string{"Sec-Fetch-Site": "cross-site"}).Code; got != http.StatusForbidden {
		t.Errorf("a line from another site = %d", got)
	}
	line := decode[apiNode](t, s.send(http.MethodPost, "/api/dividers", `{"kind":"line","parent":"`+team.ID+`","after":"`+a.ID+`"}`, sameSite))
	if line.Kind != "line" {
		t.Fatalf("line %+v", line)
	}
	kinds := func() string {
		var out string
		for _, k := range node(t, decode[apiState](t, s.get("/api/state")).Collections, team.ID).Kids {
			out += k.Kind + ":" + k.Title + " "
		}
		return out
	}
	if got := kinds(); got != "page:A line:line page:B " {
		t.Errorf("the line is in the tree between A and B: %q", got)
	}
	if got := s.send(http.MethodPost, "/api/objects/"+b.ID+"/move", `{"parent":"`+team.ID+`"}`, sameSite).Code; got != http.StatusOK {
		t.Errorf("move = %d", got)
	}
	if got := kinds(); got != "page:B page:A line:line " {
		t.Errorf("B moved first: %q", got)
	}
	if got := s.send(http.MethodPost, "/api/objects/"+a.ID+"/move", `{"parent":""}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("a page to the top = %d", got)
	}
	if got := s.send(http.MethodDelete, "/api/objects/"+line.ID, "", sameSite).Code; got != http.StatusNoContent {
		t.Errorf("delete the line = %d", got)
	}
	if got := kinds(); got != "page:B page:A " {
		t.Errorf("the line is gone: %q", got)
	}
	if trash := decode[[]apiTrashed](t, s.get("/api/trash")); len(trash) != 0 {
		t.Errorf("a line goes for good, not into the trash: %+v", trash)
	}
}

func TestTrashThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
	a := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"A","parent":"`+team.ID+`"}`, sameSite))
	s.send(http.MethodPost, "/api/pages", `{"title":"Kid","parent":"`+a.ID+`"}`, sameSite)
	s.send(http.MethodPost, "/api/pages", `{"title":"B","parent":"`+team.ID+`"}`, sameSite)
	titles := func() string {
		var out string
		for _, k := range node(t, decode[apiState](t, s.get("/api/state")).Collections, team.ID).Kids {
			out += k.Title + " "
		}
		return out
	}
	crossSite := map[string]string{"Sec-Fetch-Site": "cross-site"}
	bobHere := map[string]string{"Sec-Fetch-Site": "same-origin"}
	for k, v := range bob {
		bobHere[k] = v
	}
	if got := s.send(http.MethodDelete, "/api/objects/"+a.ID, "", crossSite).Code; got != http.StatusForbidden {
		t.Errorf("a delete from another site = %d", got)
	}
	if got := s.send(http.MethodDelete, "/api/objects/"+team.ID, "", bobHere).Code; got != http.StatusForbidden {
		t.Errorf("bob deletes Anna's collection at the top = %d", got)
	}
	if got := s.send(http.MethodDelete, "/api/objects/"+a.ID, "", sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("delete A = %d", got)
	}
	if got := titles(); got != "B " {
		t.Errorf("A is gone from the tree: %q", got)
	}
	if got := s.get("/api/doc/" + a.ID).Code; got != http.StatusNotFound {
		t.Errorf("A still opens: %d", got)
	}
	trash := decode[[]apiTrashed](t, s.get("/api/trash"))
	if len(trash) != 1 || trash[0].ID != a.ID || trash[0].Place != "Team" || trash[0].Who != "Anna Berg" || trash[0].At == "" {
		t.Fatalf("the trash: %+v", trash)
	}
	if got := decode[[]apiTrashed](t, s.getWith("/api/trash", merged(bob))); len(got) != 0 {
		t.Errorf("bob sees Anna's trash: %+v", got)
	}
	if got := s.send(http.MethodPost, "/api/trash/"+a.ID+"/restore", "", crossSite).Code; got != http.StatusForbidden {
		t.Errorf("a restore from another site = %d", got)
	}
	rec := s.send(http.MethodPost, "/api/trash/"+a.ID+"/restore", "", sameSite)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore = %d %s", rec.Code, rec.Body)
	}
	back := decode[struct {
		Parent string  `json:"parent"`
		Node   apiNode `json:"node"`
	}](t, rec)
	if back.Parent != team.ID || back.Node.ID != a.ID || len(back.Node.Kids) != 1 || back.Node.Kids[0].Title != "Kid" || !back.Node.Mine {
		t.Errorf("the answer names the place and holds A with its child: %+v", back)
	}
	if got := titles(); got != "B A " {
		t.Errorf("A is back at the end: %q", got)
	}
	// Under a parent in the trash, the answer says what to do.
	s.send(http.MethodDelete, "/api/objects/"+a.ID, "", sameSite)
	s.send(http.MethodDelete, "/api/objects/"+team.ID, "", sameSite)
	rec = s.send(http.MethodPost, "/api/trash/"+a.ID+"/restore", "", sameSite)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "restore that first") {
		t.Errorf("restore under a deleted parent = %d %s", rec.Code, rec.Body)
	}
	if trash := decode[[]apiTrashed](t, s.get("/api/trash")); len(trash) != 2 || !trash[1].PlaceDeleted {
		t.Errorf("the list says that its place is in the trash: %+v", trash)
	}
	if got := s.send(http.MethodDelete, "/api/trash/"+team.ID, "", crossSite).Code; got != http.StatusForbidden {
		t.Errorf("a purge from another site = %d", got)
	}
	if got := s.send(http.MethodDelete, "/api/trash/"+team.ID, "", bobHere).Code; got != http.StatusNotFound {
		t.Errorf("bob purges Anna's collection = %d", got)
	}
	if got := s.send(http.MethodDelete, "/api/trash/"+team.ID, "", sameSite).Code; got != http.StatusNoContent {
		t.Errorf("purge = %d", got)
	}
	if trash := decode[[]apiTrashed](t, s.get("/api/trash")); len(trash) != 0 {
		t.Errorf("the trash is empty, also of A that lay in it: %+v", trash)
	}
}

func TestCopyThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
	a := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"A","parent":"`+team.ID+`"}`, sameSite))
	s.send(http.MethodPost, "/api/pages", `{"title":"Kid","parent":"`+a.ID+`"}`, sameSite)
	if got := s.send(http.MethodPost, "/api/objects/"+a.ID+"/copy", `{"parent":"`+team.ID+`","after":"`+a.ID+`"}`, map[string]string{"Sec-Fetch-Site": "cross-site"}).Code; got != http.StatusForbidden {
		t.Errorf("a copy from another site = %d", got)
	}
	rec := s.send(http.MethodPost, "/api/objects/"+a.ID+"/copy", `{"parent":"`+team.ID+`","after":"`+a.ID+`"}`, sameSite)
	if rec.Code != http.StatusCreated {
		t.Fatalf("copy = %d %s", rec.Code, rec.Body)
	}
	made := decode[apiNode](t, rec)
	if made.Title != "A (copy)" || len(made.Kids) != 1 || made.Kids[0].Title != "Kid" || made.ID == a.ID {
		t.Errorf("the answer holds the copy and its child: %+v", made)
	}
	if got := s.send(http.MethodPost, "/api/objects/"+a.ID+"/copy", `{"parent":""}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("a page to the top = %d", got)
	}
}

func TestFoldersThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
	rec := s.send(http.MethodPost, "/api/folders", `{"title":"Drawings","parent":"`+team.ID+`"}`, sameSite)
	if rec.Code != http.StatusCreated {
		t.Fatalf("folder = %d %s", rec.Code, rec.Body)
	}
	folder := decode[apiNode](t, rec)
	if folder.Kind != "folder" {
		t.Errorf("the kind: %+v", folder)
	}
	if got := s.send(http.MethodPost, "/api/folders", `{"title":"Loose","parent":""}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("a folder at the top = %d", got)
	}
	if got := s.send(http.MethodPost, "/api/objects/"+folder.ID+"/kind", `{"kind":"collection"}`, map[string]string{"Sec-Fetch-Site": "cross-site"}).Code; got != http.StatusForbidden {
		t.Errorf("converting from another site = %d", got)
	}
	if got := decode[apiNode](t, s.send(http.MethodPost, "/api/objects/"+folder.ID+"/kind", `{"kind":"collection"}`, sameSite)); got.Kind != "collection" {
		t.Errorf("turned into a collection: %+v", got)
	}
	if got := s.send(http.MethodPost, "/api/objects/"+team.ID+"/kind", `{"kind":"folder"}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("a collection at the top into a folder = %d", got)
	}
}

func TestSettingsThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	if got := string(decode[apiState](t, s.get("/api/state")).Settings); got != "{}" {
		t.Errorf("no settings yet: %q", got)
	}
	if got := s.send(http.MethodPut, "/api/settings", `{"s2":1}`, map[string]string{"Sec-Fetch-Site": "cross-site"}).Code; got != http.StatusForbidden {
		t.Errorf("settings from another site = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/settings", `{"s2":1,"n2":["a"]}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("saving = %d", got)
	}
	if got := string(decode[apiState](t, s.get("/api/state")).Settings); got != `{"n2":["a"],"s2":1}` {
		t.Errorf("the state carries them: %q", got)
	}
	for _, bad := range []string{`[1]`, `{"s2":`, `"x"`} {
		if got := s.send(http.MethodPut, "/api/settings", bad, sameSite).Code; got != http.StatusBadRequest {
			t.Errorf("%s = %d", bad, got)
		}
	}
}

func TestAccessThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
	if got := s.send(http.MethodPost, "/api/objects/"+team.ID+"/access", `{"access":"read"}`, map[string]string{"Sec-Fetch-Site": "cross-site"}).Code; got != http.StatusForbidden {
		t.Errorf("from another site = %d", got)
	}
	if got := decode[apiNode](t, s.send(http.MethodPost, "/api/objects/"+team.ID+"/access", `{"access":"read"}`, sameSite)); got.Access != "read" {
		t.Errorf("read only: %+v", got)
	}
	if got := node(t, decode[apiState](t, s.get("/api/state")).Collections, team.ID).Access; got != "read" {
		t.Errorf("the state carries it: %q", got)
	}
	if got := s.send(http.MethodPost, "/api/objects/"+team.ID+"/access", `{"access":"owner"}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("no such level = %d", got)
	}
}
