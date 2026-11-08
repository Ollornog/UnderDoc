// SPDX-License-Identifier: MIT

package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func (s *site) send(method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	s.t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	for name, value := range gateHeaders {
		request.Header.Set(name, value)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	s.http.ServeHTTP(recorder, request)
	return recorder
}

func decode[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(recorder.Body.Bytes(), &out); err != nil {
		t.Fatalf("%d %s: %v", recorder.Code, recorder.Body.String(), err)
	}
	return out
}

func TestWorkspaceAnswersEveryAddress(t *testing.T) {
	s := newSite(t, Options{}, sample)
	for _, target := range []string{"/", "/o/plan", "/o/~ops/no-id.md", "/f/", "/f/ops/net", "/profile"} {
		body := s.body(target)
		if !strings.Contains(body, `<div id="root" class="c22-app-workspace">`) || !strings.Contains(body, "/assets/app/demo.js") {
			t.Errorf("%s does not load the workspace", target)
		}
	}
	for _, target := range []string{"/o/gone", "/f/gone", "/o/aaaaaaaaaa"} {
		if got := s.get(target).Code; got != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", target, got)
		}
	}
}

func TestStateAndDocument(t *testing.T) {
	s := newSite(t, Options{}, sample)
	state := decode[apiState](t, s.get("/api/state"))
	if state.User.Name != "Anna Berg" || state.User.Initials != "AB" || state.CanCreate {
		t.Errorf("user %+v, canCreate %v", state.User, state.CanCreate)
	}
	var names []string
	for _, c := range state.Collections {
		names = append(names, c.Title)
	}
	if strings.Join(names, ",") != "Home,ops" {
		t.Errorf("collections %v", names)
	}
	doc := decode[apiDocument](t, s.get("/api/doc/plan"))
	if doc.Title != "Network plan" || !strings.Contains(doc.Intro, "Router in the tower") || strings.Contains(doc.Intro, "<h1") {
		t.Errorf("document %+v", doc)
	}
	if got := s.get("/api/doc/gone").Code; got != http.StatusNotFound {
		t.Errorf("unknown page = %d", got)
	}
	hits := decode[[]apiHit](t, s.get("/api/search?q=router"))
	if len(hits) != 1 || hits[0].ID != "plan" || !strings.Contains(hits[0].Snippet, "<mark>") {
		t.Errorf("search %+v", hits)
	}
}

func TestCollectionsThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	if got := s.send(http.MethodPost, "/api/collections", `{"title":"Operations"}`, map[string]string{"Sec-Fetch-Site": "cross-site"}).Code; got != http.StatusForbidden {
		t.Errorf("from another site = %d", got)
	}
	made := s.send(http.MethodPost, "/api/collections", `{"title":"Operations"}`, sameSite)
	if made.Code != http.StatusCreated {
		t.Fatalf("made %d %s", made.Code, made.Body.String())
	}
	top := decode[apiNode](t, made)
	inner := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Network","parent":"`+top.ID+`"}`, sameSite))
	state := decode[apiState](t, s.get("/api/state"))
	if !state.CanCreate || state.Collections[0].Title != "My notes" || !state.Collections[0].Private {
		t.Fatalf("state %+v", state.Collections)
	}
	var found bool
	for _, c := range state.Collections {
		if c.ID == top.ID && c.Owner == "Anna Berg" && len(c.Kids) == 1 && c.Kids[0].ID == inner.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("the new tree is missing: %+v", state.Collections)
	}
	if got := s.send(http.MethodPatch, "/api/objects/"+inner.ID, `{"title":"  "}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("empty title = %d", got)
	}
	renamed := decode[apiNode](t, s.send(http.MethodPatch, "/api/objects/"+inner.ID, `{"title":"Network gear"}`, sameSite))
	if renamed.Title != "Network gear" {
		t.Errorf("renamed %+v", renamed)
	}
	if !strings.Contains(s.get("/api/state").Body.String(), `"Network gear"`) {
		t.Error("the new name is not kept")
	}
	bob := map[string]string{"Remote-User": "bob", "Remote-Id": "u-bob", "Remote-Name": "Bob", "Sec-Fetch-Site": "same-origin"}
	notes := state.Collections[0].ID
	if got := s.send(http.MethodPatch, "/api/objects/"+notes, `{"title":"Mine"}`, bob).Code; got != http.StatusNotFound {
		t.Errorf("bob renames Anna's private notes: %d", got)
	}
}
