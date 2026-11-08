// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
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

func (s *site) post(target string, form url.Values, headers map[string]string) *httptest.ResponseRecorder {
	s.t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for name, value := range gateHeaders {
		request.Header.Set(name, value)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	s.http.ServeHTTP(recorder, request)
	return recorder
}

var sameSite = map[string]string{"Sec-Fetch-Site": "same-origin"}

func TestAccountAndPrivateNotes(t *testing.T) {
	s := withDatabase(t)
	body := s.body("/read/o/plan")
	if !strings.Contains(body, `title="My notes, only you see it"`) {
		t.Error("the first request makes the account with its private notes")
	}
	if strings.Contains(body, `title="New collection: not built yet`) || !strings.Contains(body, `action="/collections"`) {
		t.Error("with a database, New collection works")
	}
	notes := regexp.MustCompile(`href="/o/([0-9a-z]{10})" title="My notes`).FindStringSubmatch(body)
	if notes == nil {
		t.Fatal("no link to the notes")
	}
	if got := s.getWith("/read/o/"+notes[1], map[string]string{"Remote-User": "bob", "Remote-Id": "u-bob"}); got.Code != http.StatusNotFound {
		t.Errorf("bob opened anna's notes: %d", got.Code)
	}
	if mine := s.body("/read/o/" + notes[1]); !strings.Contains(mine, "Only you") || !strings.Contains(mine, "Anna Berg") {
		t.Error("the page of the notes says who sees them")
	}
}

func TestCreateCollections(t *testing.T) {
	s := withDatabase(t)
	made := s.post("/collections", url.Values{"title": {" Operations "}}, sameSite)
	place := made.Header().Get("Location")
	if made.Code != http.StatusSeeOther || !regexp.MustCompile(`^/o/[0-9a-z]{10}$`).MatchString(place) {
		t.Fatalf("%d %q", made.Code, place)
	}
	page := s.body("/read" + place)
	for _, want := range []string{"<h1>Operations</h1>", "<dd>Collection</dd>", "<dd>Anna Berg</dd>", "can edit (inherited)", "Everyone signed in", `name="parent" value="` + strings.TrimPrefix(place, "/o/") + `"`} {
		if !strings.Contains(page, want) {
			t.Errorf("%q missing", want)
		}
	}
	inner := s.post("/collections", url.Values{"title": {"Network"}, "parent": {strings.TrimPrefix(place, "/o/")}}, sameSite)
	if inner.Code != http.StatusSeeOther {
		t.Fatalf("inside: %d", inner.Code)
	}
	nested := s.body("/read" + inner.Header().Get("Location"))
	if !strings.Contains(nested, `<a href="`+place+`">Operations</a>`) || !strings.Contains(nested, `aria-current="page"><svg`) {
		t.Error("the inner collection shows its way and its place in nav3")
	}
	if !strings.Contains(s.body("/read"+place), "<span>Network</span><small>Collection</small>") {
		t.Error("the outer collection lists it")
	}
	if !strings.Contains(s.getWith("/read/o/plan", map[string]string{"Remote-User": "bob", "Remote-Id": "u-bob"}).Body.String(), `title="Operations"`) {
		t.Error("bob sees the shared collection")
	}
}

// A change only from a page of this application: the gate signs in with a cookie, so another site could post otherwise.
func TestCreateOnlyFromHere(t *testing.T) {
	s := withDatabase(t)
	for name, headers := range map[string]map[string]string{
		"cross-site":       {"Sec-Fetch-Site": "cross-site"},
		"same-site":        {"Sec-Fetch-Site": "same-site"},
		"no sign of where": {},
		"foreign origin":   {"Origin": "https://evil.example"},
	} {
		if got := s.post("/collections", url.Values{"title": {"x"}}, headers).Code; got != http.StatusForbidden {
			t.Errorf("%s: %d", name, got)
		}
	}
	if got := s.post("/collections", url.Values{"title": {"x"}}, map[string]string{"Origin": "http://example.com"}).Code; got != http.StatusSeeOther {
		t.Errorf("an old browser with the own origin: %d", got)
	}
	for _, title := range []string{"", "   "} {
		if got := s.post("/collections", url.Values{"title": {title}}, sameSite).Code; got != http.StatusBadRequest {
			t.Errorf("title %q: %d", title, got)
		}
	}
	if got := s.post("/collections", url.Values{"title": {"x"}, "parent": {"0000000000"}}, sameSite).Code; got != http.StatusNotFound {
		t.Errorf("into nothing: %d", got)
	}
}

func TestCreateNeedsTheDatabase(t *testing.T) {
	s := newSite(t, Options{}, sample)
	if got := s.post("/collections", url.Values{"title": {"x"}}, sameSite).Code; got != http.StatusNotImplemented {
		t.Errorf("%d", got)
	}
}
