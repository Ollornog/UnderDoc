// SPDX-License-Identifier: MIT

package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Ollornog/EightPerDay/core"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/module"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

func timeModule(states []apiModule) apiModule {
	for _, m := range states {
		if m.ID == module.Time {
			return m
		}
	}
	return apiModule{}
}

// switchTime switches the module of the time tracking on or off as the administrator, Documents and Files staying on.
func switchTime(t *testing.T, admin *client, on bool) {
	t.Helper()
	values := url.Values{"module": {module.Documents, module.Files}}
	if on {
		values["module"] = append(values["module"], module.Time)
	}
	if got := admin.form("/admin/modules", values); got.Code != http.StatusOK {
		t.Fatalf("switch the time tracking: %d", got.Code)
	}
}

// book makes a private employer with the mode "weekly total" and books a week on it, through the interface of the
// time tracking, and returns the employer's ID.
func book(t *testing.T, c *client, name string) string {
	t.Helper()
	made := c.form("/time/employers", url.Values{"name": {name}, "kind": {"private"}, "mode": {"week_sum"}})
	if made.Code != http.StatusSeeOther || !strings.HasPrefix(made.Header().Get("Location"), "/time/") {
		t.Fatalf("make the employer: %d %s %s", made.Code, made.Header().Get("Location"), made.Body.String())
	}
	location, _ := url.Parse(made.Header().Get("Location"))
	employer := location.Query().Get("e")
	if got := c.form("/time/book/week", url.Values{"employer": {employer}, "week": {"2026-11-16"}, "hours": {"38,5"},
		"back": {"/week"}}); got.Code != http.StatusSeeOther {
		t.Fatalf("book the week: %d %s", got.Code, got.Body.String())
	}
	if got := c.form("/time/book/absence", url.Values{"employer": {employer}, "type": {core.AbsenceVacation},
		"from": {"2026-12-28"}, "to": {"2026-12-30"}, "back": {"/week"}}); got.Code != http.StatusSeeOther {
		t.Fatalf("book leave: %d %s", got.Code, got.Body.String())
	}
	return employer
}

// TestTimeTrackingIsAModule (ADR-25): off by default, every address of it is gone; the administrator switches it on and
// it runs below /time inside the workspace with UnderDoc's sign-in; off again, nothing is lost.
func TestTimeTrackingIsAModule(t *testing.T) {
	s, database := dbSite(t, Options{})
	admin := newClientWith(t, s, gateHeaders)
	if m := timeModule(stateOf(t, admin).Modules); !m.Allowed || m.On {
		t.Fatalf("allowed and off by default: %+v", m)
	}
	for _, target := range []string{"/time", "/time/", "/time/week", "/time/assets/app/app.js", "/api/time/absences?from=2026-01-01&to=2026-12-31"} {
		if got := admin.get(target); got.Code != http.StatusNotFound {
			t.Errorf("%s while off: %d", target, got.Code)
		}
	}
	if got := admin.form("/time/employers", url.Values{"name": {"x"}}); got.Code != http.StatusNotFound {
		t.Errorf("a change while off: %d", got.Code)
	}
	switchTime(t, admin, true)
	if m := timeModule(stateOf(t, admin).Modules); !m.On {
		t.Fatalf("switched on: %+v", m)
	}
	page := admin.get("/time/")
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `data-base="/time"`) {
		t.Fatalf("the time tracking: %d %s", page.Code, page.Body.String())
	}
	if csp := page.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Errorf("the workspace may not frame it: %q", csp)
	}
	if csp := admin.get("/").Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-src 'self'") {
		t.Errorf("the workspace may not show a frame of its own origin: %q", csp)
	}
	// No second sign-in: without UnderDoc's, nothing of it answers, and it has no sign-in of its own.
	stranger := newClient(t, s, "wiki.example")
	if got := stranger.get("/time/"); got.Code == http.StatusOK {
		t.Errorf("without signing in: %d", got.Code)
	}
	for _, target := range []string{"/time/login", "/time/setup", "/time/account", "/time/admin"} {
		if got := admin.get(target); got.Code != http.StatusNotFound {
			t.Errorf("%s: %d", target, got.Code)
		}
	}
	employer := book(t, admin, "Own work")
	entries, err := database.TimeStore().Entries(inMain(), core.EntryFilter{Owner: db.Person("1007"), EmployerID: employer})
	if err != nil || len(entries) != 1 || entries[0].Minutes != 38*60+30 {
		t.Fatalf("the week is Anna's: %+v %v", entries, err)
	}
	// Off again: gone, and back whole.
	switchTime(t, admin, false)
	if got := admin.get("/time/"); got.Code != http.StatusNotFound {
		t.Errorf("off again: %d", got.Code)
	}
	switchTime(t, admin, true)
	if got := admin.get("/time/?e=" + employer); !strings.Contains(got.Body.String(), "Own work") {
		t.Error("the employer is not back")
	}
	// Hidden is not off.
	if got := admin.form("/account/modules", url.Values{"hide": {module.Time}}); got.Code != http.StatusOK {
		t.Fatalf("hide: %d", got.Code)
	}
	if m := timeModule(stateOf(t, admin).Modules); !m.Hidden {
		t.Errorf("hidden: %+v", m)
	}
	if got := admin.get("/time/"); got.Code != http.StatusOK {
		t.Errorf("a hidden module answers all the same: %d", got.Code)
	}
}

// TestTimeTrackingPrivateAndManaged: what a person tracks privately only they see; the tenant is an organization whose
// administrators are UnderDoc's, and what a member tracks for it its reviewers see by EightPerDay's rules, never the
// private part.
func TestTimeTrackingPrivateAndManaged(t *testing.T) {
	s, database := dbSite(t, Options{})
	admin := newClientWith(t, s, gateHeaders)
	other := newClientWith(t, s, bob)
	switchTime(t, admin, true)
	book(t, other, "Bobs own")
	// Anna, who administers the tenant, sees nothing of Bob's private time tracking.
	for _, target := range []string{"/time/", "/time/week", "/time/reports", "/time/leave"} {
		if got := admin.get(target); strings.Contains(got.Body.String(), "Bobs own") {
			t.Errorf("%s: the administrator sees Bob's private employer", target)
		}
	}
	// The organization of the tenant: Anna administers it, Bob does not even find it.
	if got := admin.get("/time/orgs/" + tenant.Main); got.Code != http.StatusOK {
		t.Fatalf("the administrator's organization: %d", got.Code)
	}
	if got := other.get("/time/orgs/" + tenant.Main); got.Code != http.StatusNotFound {
		t.Errorf("Bob opens the organization: %d", got.Code)
	}
	// Anna takes Bob in; his managed employer belongs to the organization.
	if got := admin.form("/time/orgs/"+tenant.Main+"/member", url.Values{"person": {"1002"}, "role": {"member"}, "records": {"on"},
		"back": {"/orgs/" + tenant.Main}}); got.Code != http.StatusSeeOther {
		t.Fatalf("take Bob in: %d %s", got.Code, got.Body.String())
	}
	managed, err := database.TimeStore().Managed(inMain(), tenant.Main, core.MustDate("2026-01-01"), core.MustDate("2026-12-31"))
	if err != nil || len(managed.Catalog.Employers) != 1 || managed.Catalog.Employers[0].Owner != "1002" {
		t.Fatalf("the organization's employers: %+v %v", managed.Catalog.Employers, err)
	}
	org := admin.get("/time/orgs/" + tenant.Main).Body.String()
	if !strings.Contains(org, "Bob") || strings.Contains(org, "Bobs own") {
		t.Error("the organization's page names its member and nothing private of him")
	}
	// Bob has both, his own and the organization's employer, and books on each.
	cat, err := database.TimeStore().Catalog(inMain(), "1002")
	if err != nil || len(cat.Employers) != 2 {
		t.Fatalf("Bob's employers: %+v %v", cat.Employers, err)
	}
	managedID := managed.Catalog.Employers[0].ID
	// The sheet has a column for each of his employers (EightPerDay M-6).
	if got := other.get("/time/"); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "Bobs own") ||
		!strings.Contains(got.Body.String(), `data-col="e-`+managedID+`"`) {
		t.Errorf("Bob's sheet lacks one of his employers: %d", got.Code)
	}
}

// TestTimeTrackingAbsencesForTheCalendar: the absences of the user over their employers, as JSON for a calendar, only
// their own.
func TestTimeTrackingAbsencesForTheCalendar(t *testing.T) {
	s, _ := dbSite(t, Options{})
	admin := newClientWith(t, s, gateHeaders)
	other := newClientWith(t, s, bob)
	switchTime(t, admin, true)
	book(t, admin, "Own work")
	type answer struct {
		Absences []core.CalendarAbsence `json:"absences"`
	}
	read := func(c *client, query string) (int, answer) {
		got := c.get("/api/time/absences?" + query)
		var a answer
		_ = json.Unmarshal(got.Body.Bytes(), &a)
		return got.Code, a
	}
	code, mine := read(admin, "from=2026-12-01&to=2026-12-31")
	if code != http.StatusOK || len(mine.Absences) != 1 || mine.Absences[0].Type != core.AbsenceVacation ||
		mine.Absences[0].Employer != "Own work" || !mine.Absences[0].Leave {
		t.Fatalf("Anna's leave: %d %+v", code, mine)
	}
	if _, theirs := read(other, "from=2026-12-01&to=2026-12-31"); len(theirs.Absences) != 0 {
		t.Errorf("Bob reads Anna's leave: %+v", theirs)
	}
	if _, none := read(admin, "from=2027-01-01&to=2027-01-31"); len(none.Absences) != 0 {
		t.Errorf("outside the range: %+v", none)
	}
	for _, query := range []string{"", "from=2026-12-31&to=2026-12-01", "from=x&to=2026-12-01", "from=2020-01-01&to=2026-12-31"} {
		if code, _ := read(admin, query); code != http.StatusBadRequest {
			t.Errorf("%q: %d", query, code)
		}
	}
}

// TestTimeTrackingLockedDayIsTold (EightPerDay T-18): the details of a day in a week handed in are locked in the core,
// in UnderDoc's store too; the interface tells the member so (a sentence, never an error of the server), and the store
// refuses the record itself with core.ErrLocked.
func TestTimeTrackingLockedDayIsTold(t *testing.T) {
	s, database := dbSite(t, Options{})
	admin := newClientWith(t, s, gateHeaders)
	other := newClientWith(t, s, bob)
	switchTime(t, admin, true)
	other.get("/time/") // Bob is known to the time tracking once he opened it.
	org := "/time/orgs/" + tenant.Main
	if got := admin.form(org+"/template", url.Values{"name": {"Full time"}, "mode": {"plan"}, "hours-1": {"8"},
		"hours-2": {"8"}, "hours-3": {"8"}, "hours-4": {"8"}, "hours-5": {"8"}, "leave_days": {"30"}, "basis": {"week"},
		"back": {"/"}}); got.Code != http.StatusSeeOther {
		t.Fatalf("the template: %d %s", got.Code, got.Body.String())
	}
	if got := admin.form(org+"/features", url.Values{"approval": {"on"}, "confirm": {"on"}, "agreement": {"BV 2026"},
		"back": {"/"}}); got.Code != http.StatusSeeOther {
		t.Fatalf("the approval: %d %s", got.Code, got.Body.String())
	}
	if got := admin.form(org+"/member", url.Values{"person": {"1002"}, "role": {"member"}, "template": {"Full time"},
		"from": {"2026-11-01"}, "records": {"on"}, "back": {"/"}}); got.Code != http.StatusSeeOther {
		t.Fatalf("take Bob in: %d %s", got.Code, got.Body.String())
	}
	managed, err := database.TimeStore().Managed(inMain(), tenant.Main, core.MustDate("2026-01-01"), core.MustDate("2026-12-31"))
	if err != nil || len(managed.Catalog.Employers) != 1 {
		t.Fatalf("the organization's employer: %+v %v", managed.Catalog.Employers, err)
	}
	employer := managed.Catalog.Employers[0].ID
	monday := core.MustDate("2026-11-16")
	day := func(place string) url.Values {
		return url.Values{"employer": {employer}, "date": {monday.String()}, "place": {place}, "reason": {"why"},
			"back": {"/day?e=" + employer + "&d=" + monday.String()}}
	}
	if got := other.form("/time/book/dayinfo", day("Berlin")); got.Code != http.StatusSeeOther {
		t.Fatalf("the details of the day: %d %s", got.Code, got.Body.String())
	}
	if got := other.form("/time/approval/submit", url.Values{"employer": {employer}, "kind": {"period"},
		"from": {monday.String()}, "to": {monday.AddDays(6).String()}, "back": {"/"}}); got.Code != http.StatusSeeOther {
		t.Fatalf("hand in the week: %d %s", got.Code, got.Body.String())
	}
	got := other.form("/time/book/dayinfo", day("Hamburg"))
	if got.Code != http.StatusBadRequest || !strings.Contains(got.Body.String(), "cannot be changed") {
		t.Errorf("a locked day: %d %s", got.Code, got.Body.String())
	}
	cat, err := database.TimeStore().Catalog(inMain(), "1002")
	if err != nil {
		t.Fatal(err)
	}
	var info core.DayInfo
	for _, i := range cat.DayInfos {
		if i.ID == core.DayInfoID(employer, monday) {
			info = i
		}
	}
	if info.Place != "Berlin" {
		t.Fatalf("the details changed: %+v", cat.DayInfos)
	}
	info.Place = "Hamburg"
	if err := database.TimeStore().Put(inMain(), info); !errors.Is(err, core.ErrLocked) {
		t.Errorf("the store writes a locked day: %v", err)
	}
}

// sendForm posts a form with the headers given, as Anna through the gate.
func sendForm(s *site, target string, values url.Values, headers map[string]string) *httptest.ResponseRecorder {
	s.t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for name, value := range gateHeaders {
		request.Header.Set(name, value)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	if host := request.Header.Get("Host"); host != "" {
		request.Host = host
	}
	s.http.ServeHTTP(recorder, request)
	return recorder
}

// bookIn makes a private employer and books a week on it in the tenant of the headers, and returns the employer's ID.
func bookIn(t *testing.T, s *site, headers map[string]string, name string) string {
	t.Helper()
	made := sendForm(s, "/time/employers", url.Values{"name": {name}, "kind": {"private"}, "mode": {"week_sum"}}, headers)
	location, err := url.Parse(made.Header().Get("Location"))
	if made.Code != http.StatusSeeOther || err != nil || location.Query().Get("e") == "" {
		t.Fatalf("make the employer: %d %s", made.Code, made.Body.String())
	}
	employer := location.Query().Get("e")
	if got := sendForm(s, "/time/book/week", url.Values{"employer": {employer}, "week": {"2026-11-16"}, "hours": {"38,5"},
		"back": {"/week"}}, headers); got.Code != http.StatusSeeOther {
		t.Fatalf("book the week: %d %s", got.Code, got.Body.String())
	}
	return employer
}
