// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
)

func TestMain(m *testing.M) { os.Exit(pgtest.Main(m)) }

func withDatabase(t *testing.T) *site {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	return newSite(t, Options{DB: database}, sample)
}

var sameSite = map[string]string{"Sec-Fetch-Site": "same-origin"}

var bob = map[string]string{"Remote-User": "bob", "Remote-Id": "u-bob", "Remote-Name": "Bob"}

func TestAccountAndPrivateNotes(t *testing.T) {
	s := withDatabase(t)
	state := decode[apiState](t, s.get("/api/state"))
	if len(state.Collections) == 0 || state.Collections[0].Title != "My notes" || !state.Collections[0].Private || !state.CanCreate {
		t.Fatalf("the first request makes the account with its private notes: %+v", state.Collections)
	}
	notes := state.Collections[0].ID
	if got := s.getWith("/o/"+notes, bob); got.Code != http.StatusNotFound {
		t.Errorf("bob opens Anna's notes: %d", got.Code)
	}
	if got := s.getWith("/api/doc/"+notes, bob); got.Code != http.StatusNotFound {
		t.Errorf("bob reads Anna's notes: %d", got.Code)
	}
	if strings.Contains(s.getWith("/api/state", bob).Body.String(), notes) {
		t.Error("bob sees Anna's notes in his list")
	}
	if got := s.get("/o/" + notes); got.Code != http.StatusOK {
		t.Errorf("Anna opens her notes: %d", got.Code)
	}
	if mine := page(t, s, notes); mine.Title != "My notes" || mine.Kind != "collection" {
		t.Errorf("%+v", mine)
	}
}

func TestSharedCollections(t *testing.T) {
	s := withDatabase(t)
	top := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":" Operations "}`, sameSite))
	if top.Title != "Operations" {
		t.Errorf("the title is trimmed: %q", top.Title)
	}
	if got := s.send(http.MethodPost, "/api/collections", `{"title":"x","parent":"aaaaaaaaaa"}`, sameSite).Code; got != http.StatusNotFound {
		t.Errorf("inside something unknown: %d", got)
	}
	for name, headers := range map[string]map[string]string{"bob": bob, "anna": nil} {
		state := decode[apiState](t, s.getWith("/api/state", merged(headers)))
		if shared := findNode(state.Collections, top.ID); shared == nil || shared.Owner != "Anna Berg" {
			t.Errorf("%s does not see the shared collection: %+v", name, state.Collections)
		}
	}
}

func merged(headers map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range gateHeaders {
		out[k] = v
	}
	for k, v := range headers {
		out[k] = v
	}
	return out
}
