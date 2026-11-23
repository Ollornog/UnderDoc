//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/EightPerDay/core"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/module"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/web"
)

// inFrame is the document of the time tracking's frame in the active window, frameWindow its window.
const (
	inFrame     = `document.querySelector('iframe[data-time-frame]:not([hidden])')?.contentDocument`
	frameWindow = `document.querySelector('iframe[data-time-frame]:not([hidden])')?.contentWindow`
)

// frameRect is where an element inside the frame of the time tracking is on the screen. It scrolls the element to the middle
// of the frame first, at once and not smoothly, so it stands clear of the tab bar EightPerDay keeps at the bottom.
func (b *browser) frameRect(selector string) box {
	b.t.Helper()
	if !b.waitFor(`!!` + inFrame + `?.querySelector(` + strconv.Quote(selector) + `)`) {
		b.t.Fatalf("%s is not in the frame: %s", selector, b.eval(inFrame+`?.body?.innerText.slice(0, 300)`))
	}
	raw := b.eval(`JSON.stringify((() => { const f = document.querySelector('iframe[data-time-frame]:not([hidden])'), o = f.getBoundingClientRect();
		const el = f.contentDocument.querySelector(` + strconv.Quote(selector) + `); el.scrollIntoView({block: 'center', behavior: 'instant'}); const r = el.getBoundingClientRect();
		return {X: o.left + r.left + r.width / 2, Y: o.top + r.top + r.height / 2}; })())`)
	var r box
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		b.t.Fatalf("%s: %s", selector, raw)
	}
	return r
}

// clickInFrame clicks an element of the time tracking with the real mouse.
func (b *browser) clickInFrame(selector string) {
	b.t.Helper()
	r := b.frameRect(selector)
	b.mouse("mouseMoved", r.X, r.Y, 0, 0)
	b.mouse("mousePressed", r.X, r.Y, 1, 0)
	b.mouse("mouseReleased", r.X, r.Y, 1, 0)
}

// enter presses Enter on the focused element, as a keyboard does, so a form sends itself.
func (b *browser) enter() {
	b.t.Helper()
	b.call("Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13, "text": "\r"}, nil)
	b.call("Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13}, nil)
}

// An administrator switches the time tracking on, opens Time in the private area of the workspace, and records a week
// in the list of EightPerDay inside the window, by mouse and keyboard, signed in once. Another tenant sees nothing of
// it (ADR-23, ADR-25, docs/time-tracking.md).
func TestTimeTrackingInTheWorkspace(t *testing.T) {
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	main := tenant.With(context.Background(), tenant.Main)
	acme := tenant.With(context.Background(), "acme")
	if _, err := database.CreateTenant(main, "acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	// Chrome reaches the test server by its address, which leads to Main; acme is asked from here, by its host name.
	if err := database.AddTenantHost(main, "main", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	for _, ctx := range []context.Context{main, acme} {
		if err := database.SetSignIn(ctx, db.SignIn{Methods: []string{db.MethodGate}}); err != nil {
			t.Fatal(err)
		}
	}
	b, base := setupWith(t, web.Options{DB: database, Tenants: tenant.ModeMany, TenantDomain: "localhost", AdminGroup: "ops"}, nil)

	// Off by default: no entry in the workspace, no address.
	b.open(base + "/")
	b.ready("")
	if b.eval(`!!document.querySelector('[data-act="view"][data-id="time"]')`) != "false" {
		t.Fatal("Time is in the workspace while its module is off")
	}
	b.open(base + "/admin/modules")
	b.pressOn(`#mod-` + module.Time)
	b.pressOn(`form[action="/admin/modules"] button[type="submit"]`)
	if !b.waitFor(`(document.querySelector('[data-test="notice"]')?.textContent || '').includes('Nothing was deleted')`) {
		t.Fatal("the module was not switched on")
	}

	f := b.open(base + "/")
	b.ready("")
	clean(t, "workspace", f)
	b.clickOn(`[data-act="view"][data-id="time"]`, 1)
	b.clickOn(`[data-act="open"][data-id="epd:/"]`, 1)
	if !b.waitFor(`!!` + inFrame + `?.querySelector('[data-employer-form]')`) {
		t.Fatalf("the time tracking does not show in the window: %s", b.eval(`document.querySelector('[data-win]')?.innerText.slice(0, 300)`))
	}
	// No second sign-in: the frame is signed in already, and offers no sign-in or account of its own.
	if got := b.eval(inFrame + `.querySelector('a[href$="/account"], a[href$="/login"]') === null && ` + inFrame + `.documentElement.dataset.themeKey === 'ud.theme'`); got != "true" {
		t.Error("the frame has a sign-in or an account of its own, or not the theme of the workspace")
	}

	// A new employer with a schedule of eight hours a day, then the week of today on the sheet: each field saves itself
	// (EightPerDay M-6), in the table or, in a narrow frame, the journal, whichever the sheet chose.
	b.clickInFrame(`#new-name`)
	b.typeText("Acme work")
	b.clickInFrame(`#new-mode-schedule`)
	b.clickInFrame(`#new-hours`)
	b.typeText("8:00")
	b.clickInFrame(`[data-employer-form] button[type="submit"]`)
	berlin, _ := time.LoadLocation("Europe/Berlin")
	today := core.DateIn(time.Now(), berlin)
	monday := today.StartOfWeek(time.Monday)
	if !b.waitFor(`!!` + inFrame + `?.querySelector('[data-sheet] [data-cell][data-date="` + monday.String() + `"]')`) {
		t.Fatalf("the sheet does not show the week: %s", b.eval(inFrame+`?.body?.innerText.slice(0, 300)`))
	}
	for i := range 5 {
		day := monday.AddDays(i).String()
		field := `[data-cell][data-date="` + day + `"]`
		value := strconv.Itoa(7 + i%3)
		b.eval(inFrame + `.querySelector('` + field + `').scrollIntoView({block: 'center', behavior: 'instant'}); 1`)
		b.clickInFrame(field)
		b.typeText(value)
		b.enter()
		if !b.waitFor(inFrame + `?.querySelector('` + field + `')?.value === '` + value + `'`) {
			t.Fatalf("%s does not show what was typed: %s", day, b.eval(inFrame+`?.body?.innerText.slice(0, 300)`))
		}
	}
	b.shot("time-tracking")
	// The workspace around it stayed: its nav1 is there, and a redraw of the workspace keeps the frame where it was.
	before := b.eval(inFrame + `.location.href`)
	b.clickOn(`[data-act="view"][data-id="time"]`, 1)
	b.clickOn(`[data-act="view"][data-id="time"]`, 1)
	if after := b.eval(inFrame + `.location.href`); after != before {
		t.Errorf("a redraw of the workspace reloaded the frame: %s, was %s", after, before)
	}

	// Each field saved itself: the week arrives in the database, five entries.
	var entries []core.Entry
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		entries, err = database.TimeStore().Entries(main, core.EntryFilter{Owner: db.Person("anna"), From: monday, To: monday.AddDays(6)})
		if (err == nil && len(entries) == 5) || time.Now().After(deadline) {
			break
		}
	}
	if err != nil || len(entries) != 5 {
		t.Fatalf("the week in the database: %+v %v", entries, err)
	}

	// acme, the module on there too, sees nothing of it: neither its pages nor the absences nor its database.
	if err := database.SetTenantModules(acme, []string{module.Documents, module.Files, module.Time}); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/time/", "/time/week", "/time/all", "/time/reports", "/api/time/absences?from=2026-01-01&to=2026-12-31"} {
		request, err := http.NewRequest(http.MethodGet, base+target, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Host = "acme.localhost"
		request.Header.Set("Remote-Auth", "session")
		request.Header.Set("Remote-User", "anna")
		request.Header.Set("Remote-Groups", "ops")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || strings.Contains(string(body), "Acme work") {
			t.Errorf("acme %s: %d, shows the employer of Main: %v", target, response.StatusCode, strings.Contains(string(body), "Acme work"))
		}
	}
	if theirs, err := database.TimeStore().Entries(acme, core.EntryFilter{Owner: db.Person("anna")}); err != nil || len(theirs) != 0 {
		t.Errorf("acme's database holds the week of Main: %+v %v", theirs, err)
	}
}
