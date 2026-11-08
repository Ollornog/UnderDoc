// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"strings"
	"testing"
)

func TestPagesThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
	if got := s.send(http.MethodPost, "/api/pages", `{"title":"Plan","parent":"`+team.ID+`"}`, map[string]string{"Sec-Fetch-Site": "cross-site"}).Code; got != http.StatusForbidden {
		t.Errorf("a page from another site = %d", got)
	}
	made := s.send(http.MethodPost, "/api/pages", `{"title":"Plan","parent":"`+team.ID+`"}`, sameSite)
	if made.Code != http.StatusCreated {
		t.Fatalf("page %d %s", made.Code, made.Body.String())
	}
	plan := decode[apiNode](t, made)
	if found := findNode(decode[apiState](t, s.get("/api/state")).Collections, plan.ID); found == nil || found.Kind != "page" {
		t.Errorf("the page is in the tree: %+v", found)
	}
	if got := s.get("/o/" + plan.ID); got.Code != http.StatusOK {
		t.Errorf("the page has its address: %d", got.Code)
	}
	empty := page(t, s, plan.ID)
	if !empty.CanEdit || len(empty.Blocks) != 0 || empty.Place != "Team" || empty.Owner != "Anna Berg" {
		t.Errorf("a new page: %+v", empty)
	}

	blocks := "/api/pages/" + plan.ID + "/blocks"
	first := decode[apiBlock](t, s.send(http.MethodPost, blocks, `{"md":"## Switches\n\nTwo in the rack."}`, sameSite))
	if first.ID != "b1" || !strings.Contains(first.HTML, "<h2") || !strings.Contains(first.HTML, "Two in the rack.") {
		t.Errorf("a block answers rendered: %+v", first)
	}
	second := decode[apiBlock](t, s.send(http.MethodPost, blocks, `{"after":"b1","md":"| Port | VLAN |\n|---|---|\n| 1 | 10 |"}`, sameSite))
	if second.ID != "b2" || !strings.Contains(second.HTML, "<table") {
		t.Errorf("a table block: %+v", second)
	}
	edited := s.send(http.MethodPut, blocks+"/b1", `{"md":"Three in the rack.<script>alert(1)</script>"}`, sameSite)
	if b := decode[apiBlock](t, edited); strings.Contains(b.HTML, "<script") || !strings.Contains(b.Markdown, "<script>") {
		t.Errorf("raw HTML is dropped from the view and kept in the source: %+v", b)
	}
	if got := s.send(http.MethodDelete, blocks+"/b2", "", sameSite).Code; got != http.StatusNoContent {
		t.Errorf("delete = %d", got)
	}
	if got := s.send(http.MethodDelete, blocks+"/b2", "", sameSite).Code; got != http.StatusNotFound {
		t.Errorf("delete twice = %d", got)
	}
	doc := page(t, s, plan.ID)
	if len(doc.Blocks) != 1 || doc.Blocks[0].ID != "b1" || !strings.Contains(doc.Blocks[0].HTML, "Three in the rack.") {
		t.Errorf("the page after the changes: %+v", doc.Blocks)
	}
	versions := decode[[]apiVersion](t, s.get("/api/pages/"+plan.ID+"/versions"))
	if len(versions) != 4 || !versions[0].Deleted || versions[0].Block != "b2" || versions[3].Block != "b1" || versions[3].Author != "Anna Berg" {
		t.Errorf("versions, newest first: %+v", versions)
	}
	if got := s.send(http.MethodPost, blocks, `{"md":"`+strings.Repeat("x", 100001)+`"}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("a block too long = %d", got)
	}
	if got := s.send(http.MethodPost, "/api/pages/"+team.ID+"/blocks", `{"md":"x"}`, sameSite).Code; got != http.StatusNotFound {
		t.Errorf("a block in a collection = %d", got)
	}
}

func TestPagesFollowAccess(t *testing.T) {
	s := withDatabase(t)
	state := decode[apiState](t, s.get("/api/state"))
	notes := state.Collections[0].ID
	diary := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Diary","parent":"`+notes+`"}`, sameSite))
	s.send(http.MethodPost, "/api/pages/"+diary.ID+"/blocks", `{"md":"Secret."}`, sameSite)
	asBob := merged(bob)
	asBob["Sec-Fetch-Site"] = "same-origin"
	for _, target := range []string{"/api/doc/" + diary.ID, "/api/pages/" + diary.ID + "/versions", "/o/" + diary.ID} {
		if got := s.getWith(target, asBob); got.Code != http.StatusNotFound || strings.Contains(got.Body.String(), "Secret") {
			t.Errorf("bob reads %s: %d", target, got.Code)
		}
	}
	if got := s.send(http.MethodPut, "/api/pages/"+diary.ID+"/blocks/b1", `{"md":"Mine."}`, asBob).Code; got != http.StatusNotFound {
		t.Errorf("bob writes into Anna's private page: %d", got)
	}
	if got := s.send(http.MethodPost, "/api/pages", `{"title":"Mine","parent":"`+notes+`"}`, asBob).Code; got != http.StatusNotFound {
		t.Errorf("bob makes a page in Anna's notes: %d", got)
	}

	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
	plan := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Plan","parent":"`+team.ID+`"}`, sameSite))
	s.send(http.MethodPost, "/api/pages/"+plan.ID+"/blocks", `{"md":"Text."}`, sameSite)
	if doc := decode[apiDocument](t, s.getWith("/api/doc/"+plan.ID, asBob)); !doc.CanEdit {
		t.Errorf("others may edit by default: %+v", doc)
	}
	if got := s.send(http.MethodPut, "/api/pages/"+plan.ID+"/blocks/b1", `{"md":"Bob was here."}`, asBob).Code; got != http.StatusOK {
		t.Errorf("bob edits a shared page: %d", got)
	}
}
