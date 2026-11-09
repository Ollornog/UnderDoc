// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
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
	if got := s.send(http.MethodDelete, "/api/objects/"+a.ID, "", sameSite).Code; got != http.StatusNotFound {
		t.Errorf("a page is not deleted this way = %d", got)
	}
	if got := s.send(http.MethodDelete, "/api/objects/"+line.ID, "", sameSite).Code; got != http.StatusNoContent {
		t.Errorf("delete the line = %d", got)
	}
	if got := kinds(); got != "page:B page:A " {
		t.Errorf("the line is gone: %q", got)
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
