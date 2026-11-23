// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
)

// TestAnAdministratorTransfersWhatIsLeftBehind: a new collection is its owner's alone (ADR-29, 2026-11-19). When its
// owner is gone, an administrator finds it among what that account holds, sees its title and its rights but not its
// content, and transfers it to someone else; the log names who did it. Nobody else learns of the collection meanwhile.
func TestAnAdministratorTransfersWhatIsLeftBehind(t *testing.T) {
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	// Anna administers through the group of her gate headers; bob and carl do not.
	s := newSite(t, Options{DB: database}, sample)
	for _, who := range []map[string]string{plain, carlOf} {
		s.send(http.MethodGet, "/api/state", "", who)
	}
	drafts := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Drafts"}`, bobSpaces))
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Offer","parent":"`+drafts.ID+`"}`, plain))
	if got := s.send(http.MethodPost, "/api/pages/"+page.ID+"/blocks", `{"md":"The price: 4711."}`, plain).Code; got != http.StatusCreated {
		t.Fatalf("block = %d", got)
	}
	for name, who := range map[string]map[string]string{"carl": carlOf, "the administrator": sameSite} {
		state := decode[apiState](t, s.send(http.MethodGet, "/api/state", "", who))
		if slices.ContainsFunc(state.Collections, func(n apiNode) bool { return n.ID == drafts.ID }) {
			t.Errorf("%s sees bob's new collection: %+v", name, state.Collections)
		}
		if got := s.send(http.MethodGet, "/api/doc/"+page.ID, "", who).Code; got != http.StatusNotFound {
			t.Errorf("%s opens the page = %d", name, got)
		}
	}
	// The administrator asks what bob holds: the collection is there by its title.
	held := decode[[]apiHeld](t, s.send(http.MethodGet, "/api/rights/held?account=1002", "", sameSite))
	if !slices.ContainsFunc(held, func(h apiHeld) bool { return h.Object == drafts.ID && h.Via == "owner" && h.Title == "Drafts" }) {
		t.Fatalf("bob owns Drafts: %+v", held)
	}
	rights := decode[apiRights](t, s.send(http.MethodGet, "/api/objects/"+drafts.ID+"/rights", "", sameSite))
	if rights.Owner != "1002" || len(rights.Own) != 0 || !rights.Manage || rights.Role != "" {
		t.Errorf("the administrator manages its rights, holding no role: %+v", rights)
	}
	if got := s.send(http.MethodPut, "/api/objects/"+drafts.ID+"/owner", `{"to":"1003"}`, carlOf).Code; got != http.StatusNotFound {
		t.Errorf("carl takes it = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/objects/"+drafts.ID+"/owner", `{"to":"1003"}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("the administrator transfers it = %d", got)
	}
	if got := s.send(http.MethodGet, "/api/doc/"+page.ID, "", sameSite).Code; got != http.StatusNotFound {
		t.Errorf("transferring it shows the administrator nothing of it = %d", got)
	}
	if doc := s.send(http.MethodGet, "/api/doc/"+page.ID, "", carlOf); doc.Code != http.StatusOK || !strings.Contains(doc.Body.String(), "4711") {
		t.Errorf("carl owns it now and reads the page = %d", doc.Code)
	}
	entries := decode[[]apiEntry](t, s.send(http.MethodGet, "/api/log?object="+drafts.ID, "", sameSite))
	if !slices.ContainsFunc(entries, func(e apiEntry) bool {
		return e.Action == db.LogOwner && e.Actor == "1007" && e.Detail == "to 1003" && e.ObjectTitle == "Drafts"
	}) {
		t.Errorf("the log names the transfer: %+v", entries)
	}
}

// TestTheRetentionOfTheLog: everyone reads how long the log is kept; only administrators change it, within its bounds,
// and the change is logged with what it was (ADR-29, 2026-11-19).
func TestTheRetentionOfTheLog(t *testing.T) {
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	s := newSite(t, Options{DB: database}, sample)
	s.send(http.MethodGet, "/api/state", "", plain)
	if got := decode[apiKeep](t, s.send(http.MethodGet, "/api/log/keep", "", plain)); got.Days != 365 || got.Min != 30 || got.Max != 3650 {
		t.Errorf("a year by default: %+v", got)
	}
	if got := s.send(http.MethodPut, "/api/log/keep", `{"days":30}`, plain).Code; got != http.StatusForbidden {
		t.Errorf("bob sets it = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/log/keep", `{"days":10}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("ten days = %d", got)
	}
	if got := decode[apiKeep](t, s.send(http.MethodPut, "/api/log/keep", `{"days":730}`, sameSite)); got.Days != 730 {
		t.Errorf("two years: %+v", got)
	}
	if got := decode[apiKeep](t, s.send(http.MethodGet, "/api/log/keep", "", plain)); got.Days != 730 {
		t.Errorf("kept: %+v", got)
	}
	entries := decode[[]apiEntry](t, s.send(http.MethodGet, "/api/log", "", sameSite))
	if !slices.ContainsFunc(entries, func(e apiEntry) bool { return e.Action == db.LogKept && e.Detail == "365 days to 730 days" }) {
		t.Errorf("the change is logged: %+v", entries)
	}
}
