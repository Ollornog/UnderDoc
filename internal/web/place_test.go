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
