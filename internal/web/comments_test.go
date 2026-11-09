// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// Comments on a page and its blocks through the API: added, answered, changed, resolved and deleted, counted in /api/doc,
// and allowed as ADR-20 says.
func TestCommentsThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
	plan := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Plan","parent":"`+team.ID+`"}`, sameSite))
	blocks := "/api/pages/" + plan.ID + "/blocks"
	s.send(http.MethodPost, blocks, `{"md":"## Switches"}`, sameSite)
	s.send(http.MethodPost, blocks, `{"after":"b1","md":"Two in the rack."}`, sameSite)
	comments := "/api/pages/" + plan.ID + "/comments"
	asBob := merged(bob)
	asBob["Sec-Fetch-Site"] = "same-origin"

	if got := s.send(http.MethodPost, comments, `{"body":"x"}`, map[string]string{"Sec-Fetch-Site": "cross-site"}).Code; got != http.StatusForbidden {
		t.Errorf("a comment from another site = %d", got)
	}
	made := s.send(http.MethodPost, comments, `{"body":"Good plan.<script>alert(1)</script>"}`, sameSite)
	if made.Code != http.StatusCreated {
		t.Fatalf("comment %d %s", made.Code, made.Body.String())
	}
	whole := decode[apiComment](t, made)
	if whole.Block != "" || whole.Who != "Anna Berg" || !whole.Mine || !whole.CanResolve || whole.Body != "Good plan.<script>alert(1)</script>" || !strings.HasSuffix(whole.At, "Z") {
		t.Errorf("a comment on the whole page, kept as plain text, stamped in UTC: %+v", whole)
	}
	onBlock := decode[apiComment](t, s.send(http.MethodPost, comments, `{"block":"b2","body":"Which rack?"}`, asBob))
	if onBlock.Block != "b2" || onBlock.Who != "Bob" || !onBlock.Mine {
		t.Errorf("bob comments on a block: %+v", onBlock)
	}
	reply := decode[apiComment](t, s.send(http.MethodPost, comments, `{"body":"Rack 2.","parent":`+strconv.FormatInt(onBlock.ID, 10)+`}`, sameSite))
	if reply.Block != "b2" || reply.Parent != onBlock.ID || reply.CanResolve {
		t.Errorf("a reply, on the block of its thread, resolved only with it: %+v", reply)
	}
	for body, want := range map[string]int{
		`{"block":"b9","body":"x"}`:                                     http.StatusNotFound,
		`{"body":"   "}`:                                                http.StatusBadRequest,
		`{"body":"` + strings.Repeat("x", 10001) + `"}`:                 http.StatusBadRequest,
		`{"body":"x","parent":` + strconv.FormatInt(reply.ID, 10) + `}`: http.StatusBadRequest,
	} {
		if got := s.send(http.MethodPost, comments, body, sameSite).Code; got != want {
			t.Errorf("%.40s = %d, want %d", body, got, want)
		}
	}

	list := decode[apiComments](t, s.getWith(comments, asBob))
	if !list.CanComment || len(list.Comments) != 3 || list.Comments[0].ID != whole.ID || list.Comments[1].ID != onBlock.ID || list.Comments[2].ID != reply.ID {
		t.Fatalf("the list, threads with their replies: %+v", list)
	}
	if c := list.Comments[0]; c.Mine || c.CanResolve != true {
		t.Errorf("for bob, Anna's comment is not his, and he may resolve it while others may edit: %+v", c)
	}
	doc := page(t, s, plan.ID)
	if doc.Comments != 3 || !doc.CanComment || doc.Blocks[0].Comments != 0 || doc.Blocks[1].Comments != 2 {
		t.Errorf("open comments in /api/doc: %d %v %+v", doc.Comments, doc.CanComment, doc.Blocks)
	}

	id := "/api/comments/" + strconv.FormatInt(onBlock.ID, 10)
	if got := s.send(http.MethodPatch, id, `{"body":"Changed by Anna"}`, sameSite).Code; got != http.StatusForbidden {
		t.Errorf("Anna edits Bob's comment = %d", got)
	}
	edited := decode[apiComment](t, s.send(http.MethodPatch, id, `{"body":"Which rack, exactly?"}`, asBob))
	if edited.Body != "Which rack, exactly?" || edited.Edited == "" {
		t.Errorf("bob edits his own: %+v", edited)
	}
	if got := s.send(http.MethodPatch, id, `{"body":"x","resolved":true}`, asBob).Code; got != http.StatusBadRequest {
		t.Errorf("both at once = %d", got)
	}
	if got := s.send(http.MethodPatch, id, `{}`, asBob).Code; got != http.StatusBadRequest {
		t.Errorf("nothing = %d", got)
	}
	resolved := decode[apiComment](t, s.send(http.MethodPatch, id, `{"resolved":true}`, sameSite))
	if resolved.Resolved == "" || resolved.ResolvedBy != "Anna Berg" || !strings.HasSuffix(resolved.Resolved, "Z") {
		t.Errorf("the owner of the page resolves bob's thread: %+v", resolved)
	}
	if doc := page(t, s, plan.ID); doc.Comments != 1 || doc.Blocks[1].Comments != 0 {
		t.Errorf("a resolved thread counts no more: %d %+v", doc.Comments, doc.Blocks)
	}
	if got := decode[apiComment](t, s.send(http.MethodPatch, id, `{"resolved":false}`, asBob)); got.Resolved != "" {
		t.Errorf("opened again: %+v", got)
	}

	if got := s.send(http.MethodDelete, id, "", sameSite).Code; got != http.StatusForbidden {
		t.Errorf("Anna deletes Bob's comment = %d", got)
	}
	if got := s.send(http.MethodDelete, id, "", asBob).Code; got != http.StatusConflict {
		t.Errorf("a comment others have answered stays = %d", got)
	}
	if got := s.send(http.MethodDelete, "/api/comments/"+strconv.FormatInt(reply.ID, 10), "", sameSite).Code; got != http.StatusNoContent {
		t.Errorf("Anna deletes her reply = %d", got)
	}
	if got := s.send(http.MethodDelete, id, "", asBob).Code; got != http.StatusNoContent {
		t.Errorf("then bob deletes his = %d", got)
	}
	for _, bad := range []string{"/api/comments/abc", "/api/comments/0", "/api/comments/" + strconv.FormatInt(onBlock.ID, 10)} {
		if got := s.send(http.MethodDelete, bad, "", sameSite).Code; got != http.StatusNotFound {
			t.Errorf("DELETE %s = %d", bad, got)
		}
	}

	// A deleted block keeps its comments, marked as gone.
	gone := decode[apiComment](t, s.send(http.MethodPost, comments, `{"block":"b1","body":"Heading?"}`, sameSite))
	s.send(http.MethodDelete, blocks+"/b1", "", sameSite)
	if list := decode[apiComments](t, s.get(comments)); len(list.Comments) != 2 || list.Comments[1].ID != gone.ID || !list.Comments[1].Gone {
		t.Errorf("a comment on a block that is gone: %+v", list.Comments)
	}
	if doc := page(t, s, plan.ID); doc.Comments != 2 {
		t.Errorf("it still counts on the page: %d", doc.Comments)
	}

	// Read only: bob sees the comments and adds none.
	s.send(http.MethodPost, "/api/objects/"+team.ID+"/access", `{"access":"read"}`, sameSite)
	if list := decode[apiComments](t, s.getWith(comments, asBob)); list.CanComment || len(list.Comments) != 2 || list.Comments[0].CanResolve {
		t.Errorf("read only, as bob: %+v", list)
	}
	if doc := decode[apiDocument](t, s.getWith("/api/doc/"+plan.ID, asBob)); doc.CanComment || doc.Comments != 2 {
		t.Errorf("read only in /api/doc: %v %d", doc.CanComment, doc.Comments)
	}
	if got := s.send(http.MethodPost, comments, `{"body":"Bob's"}`, asBob).Code; got != http.StatusForbidden {
		t.Errorf("bob comments on a page set to read only = %d", got)
	}
	if got := s.send(http.MethodPost, comments, `{"body":"The owner always may"}`, sameSite).Code; got != http.StatusCreated {
		t.Errorf("the owner comments on a page set to read only = %d", got)
	}

	// A private page: its comments are not to be found for anyone else.
	notes := decode[apiState](t, s.get("/api/state")).Collections[0].ID
	diary := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Diary","parent":"`+notes+`"}`, sameSite))
	secret := decode[apiComment](t, s.send(http.MethodPost, "/api/pages/"+diary.ID+"/comments", `{"body":"Secret"}`, sameSite))
	if got := s.getWith("/api/pages/"+diary.ID+"/comments", asBob); got.Code != http.StatusNotFound || strings.Contains(got.Body.String(), "Secret") {
		t.Errorf("bob reads the comments of Anna's private page: %d", got.Code)
	}
	if got := s.send(http.MethodPatch, "/api/comments/"+strconv.FormatInt(secret.ID, 10), `{"resolved":true}`, asBob).Code; got != http.StatusNotFound {
		t.Errorf("bob resolves a comment of Anna's private page = %d", got)
	}
	if got := s.send(http.MethodPost, "/api/pages/"+diary.ID+"/comments", `{"body":"Mine"}`, asBob).Code; got != http.StatusNotFound {
		t.Errorf("bob comments on Anna's private page = %d", got)
	}
}
