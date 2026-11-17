// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/db"
)

func TestTextOfFiles(t *testing.T) {
	s := withFiles(t)
	s.site = newSite(t, Options{DB: s.db, Store: s.store, AdminGroup: "underdoc-admins", TextWays: db.Ways{Browser: true, Server: true}}, sample)
	mine := s.root(nil, db.RootUser)
	done := decode[finished](t, s.put(nil, fmt.Sprintf(`"parent":%q`, mine.ID), "notes.md", []byte("# Card payment"), 10))
	// The upload put the reading on the queue of the server.
	job, err := s.db.ClaimTextJob(context.Background(), time.Minute)
	if err != nil || job.Way != "server" || job.ContentType != "text/markdown" {
		t.Fatalf("%+v %v", job, err)
	}
	if got := s.send(http.MethodPut, "/api/files/"+done.ID+"/text", `{"text":"from the browser"}`, sameSite); got.Code != http.StatusNoContent {
		t.Fatalf("the browser's text: %d %s", got.Code, got.Body)
	}
	texts := decode[[]map[string]string](t, s.get("/api/files/"+done.ID+"/text"))
	if len(texts) != 1 || texts[0]["source"] != "browser" || texts[0]["text"] != "from the browser" {
		t.Fatalf("%+v", texts)
	}
	bobStaff := merged2(bob, map[string]string{"Remote-Groups": "staff"})
	if got := s.send(http.MethodPut, "/api/files/"+done.ID+"/text", `{"text":"x"}`, merged2(sameSite, bobStaff)); got.Code != http.StatusNotFound {
		t.Errorf("bob gave a text to anna's file: %d", got.Code)
	}
	// The tenant switches the browser off: only administrators may, and then nobody's browser gives text.
	if got := s.send(http.MethodPut, "/api/files/text-settings", `{"scope":"tenant","browser":false}`, merged2(sameSite, bobStaff)); got.Code != http.StatusForbidden {
		t.Errorf("bob set the ways of the tenant: %d", got.Code)
	}
	if got := s.send(http.MethodPut, "/api/files/text-settings", `{"scope":"tenant","browser":false}`, sameSite); got.Code != http.StatusNoContent {
		t.Fatalf("anna sets the ways of the tenant: %d %s", got.Code, got.Body)
	}
	if got := s.send(http.MethodPut, "/api/files/"+done.ID+"/text", `{"text":"again"}`, sameSite); got.Code != http.StatusForbidden {
		t.Errorf("the browser gave text while its way is off: %d", got.Code)
	}
	settings := decode[map[string]any](t, s.get("/api/files/text-settings"))
	if ways := settings["ways"].(map[string]any); ways["browser"] != false || ways["server"] != true || ways["ai"] != false {
		t.Errorf("%+v", settings)
	}
	// A user switches the server off for their own uploads.
	if got := s.send(http.MethodPut, "/api/files/text-settings", `{"scope":"mine","server":false}`, sameSite); got.Code != http.StatusNoContent {
		t.Fatalf("%d", got.Code)
	}
	decode[finished](t, s.put(nil, fmt.Sprintf(`"parent":%q`, mine.ID), "more.txt", []byte("more"), 10))
	if job, err := s.db.ClaimTextJob(context.Background(), time.Minute); err == nil {
		t.Errorf("a job for a user who switched the server off: %+v", job)
	}
}
