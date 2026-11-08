//go:build browser

// SPDX-License-Identifier: MIT

// Package browsertest runs the real handler in a real headless Chrome. A missing Chrome fails the test: a skipped browser
// test is not a green one. It talks to Chrome over a pipe, without a websocket and without a third-party library.
//
//	go test -tags browser ./internal/browsertest/
//
// BROWSER_SHOTS=<folder> also saves a picture of every page.
package browsertest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/folder"
	"github.com/Ollornog/UnderDoc/internal/library"
	"github.com/Ollornog/UnderDoc/internal/web"
)

func findChrome(t *testing.T) string {
	t.Helper()
	for _, name := range []string{os.Getenv("CHROME"), "google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if name == "" {
			continue
		}
		if found, err := exec.LookPath(name); err == nil {
			return found
		}
	}
	t.Fatal("no Chrome or Chromium found (set CHROME=...): the browser test is required")
	return ""
}

type message struct {
	ID        int                       `json:"id,omitempty"`
	Method    string                    `json:"method,omitempty"`
	Params    json.RawMessage           `json:"params,omitempty"`
	Result    json.RawMessage           `json:"result,omitempty"`
	Error     *struct{ Message string } `json:"error,omitempty"`
	SessionID string                    `json:"sessionId,omitempty"`
}

type browser struct {
	t       *testing.T
	out     io.Writer
	mu      sync.Mutex
	next    int
	waiting map[int]chan message
	events  chan message
	session string
}

func start(t *testing.T) *browser {
	t.Helper()
	toChrome, weWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	fromChrome, chromeWrites, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	// Not t.TempDir: helper processes keep writing after the main process ends, which would fail its cleanup.
	profile, err := os.MkdirTemp("", "underdoc-browsertest-")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(findChrome(t), "--headless=new", "--remote-debugging-pipe", "--no-first-run",
		"--no-default-browser-check", "--disable-gpu", "--disable-extensions", "--disable-sync",
		"--disable-background-networking", "--disable-component-update", "--no-sandbox",
		"--user-data-dir="+profile, "--window-size=1280,900", "about:blank")
	cmd.ExtraFiles = []*os.File{toChrome, chromeWrites} // Chrome reads fd 3 and writes fd 4
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = toChrome.Close()
	_ = chromeWrites.Close()
	b := &browser{t: t, out: weWrite, waiting: map[int]chan message{}, events: make(chan message, 4096)}
	go func() {
		reader := bufio.NewReader(fromChrome)
		for {
			raw, err := reader.ReadBytes(0)
			if err != nil {
				close(b.events)
				return
			}
			var m message
			if json.Unmarshal(bytes.TrimSuffix(raw, []byte{0}), &m) != nil {
				continue
			}
			if m.ID != 0 {
				b.mu.Lock()
				answer := b.waiting[m.ID]
				delete(b.waiting, m.ID)
				b.mu.Unlock()
				if answer != nil {
					answer <- m
				}
				continue
			}
			select {
			case b.events <- m:
			default:
			}
		}
	}()
	t.Cleanup(func() {
		_ = weWrite.Close()
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // the whole group: renderer, GPU, crash handler
		_, _ = cmd.Process.Wait()
		for range 20 {
			if os.RemoveAll(profile) == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
	return b
}

func (b *browser) call(method string, params, result any) {
	b.t.Helper()
	b.mu.Lock()
	b.next++
	id := b.next
	answer := make(chan message, 1)
	b.waiting[id] = answer
	b.mu.Unlock()
	raw, _ := json.Marshal(params)
	m := message{ID: id, Method: method, Params: raw, SessionID: b.session}
	if method == "Target.createTarget" || method == "Target.attachToTarget" {
		m.SessionID = ""
	}
	encoded, _ := json.Marshal(m)
	if _, err := b.out.Write(append(encoded, 0)); err != nil {
		b.t.Fatalf("%s: %v", method, err)
	}
	select {
	case r := <-answer:
		if r.Error != nil {
			b.t.Fatalf("%s: %s", method, r.Error.Message)
		}
		if result != nil {
			if err := json.Unmarshal(r.Result, result); err != nil {
				b.t.Fatalf("%s: %v", method, err)
			}
		}
	case <-time.After(30 * time.Second):
		b.t.Fatalf("%s: no answer", method)
	}
}

type findings struct {
	requests, failed, csp, errors []string
}

// collect reads events until the page has loaded and the network has been quiet for a while.
func (b *browser) collect() findings {
	b.t.Helper()
	var f findings
	loaded := false
	quiet := time.NewTimer(10 * time.Second)
	deadline := time.After(20 * time.Second)
	for {
		select {
		case m, open := <-b.events:
			if !open {
				b.t.Fatal("Chrome has ended")
			}
			if m.SessionID != b.session {
				continue
			}
			switch m.Method {
			case "Page.loadEventFired":
				loaded = true
				quiet.Reset(500 * time.Millisecond)
			case "Network.requestWillBeSent":
				var p struct{ Request struct{ URL string } }
				_ = json.Unmarshal(m.Params, &p)
				f.requests = append(f.requests, p.Request.URL)
			case "Network.loadingFailed":
				var p struct{ ErrorText, BlockedReason, Type string }
				_ = json.Unmarshal(m.Params, &p)
				f.failed = append(f.failed, fmt.Sprintf("%s %s %s", p.Type, p.ErrorText, p.BlockedReason))
			case "Network.responseReceived":
				var p struct {
					Response struct {
						URL    string
						Status int
					}
				}
				_ = json.Unmarshal(m.Params, &p)
				if p.Response.Status >= 400 {
					f.failed = append(f.failed, fmt.Sprintf("%d %s", p.Response.Status, p.Response.URL))
				}
			case "Log.entryAdded":
				var p struct {
					Entry struct{ Source, Level, Text string }
				}
				_ = json.Unmarshal(m.Params, &p)
				if p.Entry.Source == "security" || strings.Contains(p.Entry.Text, "Content Security Policy") {
					f.csp = append(f.csp, p.Entry.Text)
				} else if p.Entry.Level == "error" {
					f.errors = append(f.errors, p.Entry.Source+": "+p.Entry.Text)
				}
			case "Runtime.exceptionThrown":
				f.errors = append(f.errors, "exception: "+string(m.Params))
			}
		case <-quiet.C:
			if loaded {
				return f
			}
		case <-deadline:
			b.t.Fatal("the page does not finish loading")
		}
	}
}

func (b *browser) open(address string) findings {
	b.t.Helper()
	b.call("Page.navigate", map[string]any{"url": address}, nil)
	return b.collect()
}

// eval runs an expression in the page and returns its value as a string.
func (b *browser) eval(expression string) string {
	b.t.Helper()
	var result struct {
		Result struct {
			Value any
		}
		ExceptionDetails *struct{ Text string }
	}
	b.call("Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true}, &result)
	if result.ExceptionDetails != nil {
		b.t.Fatalf("%s: %s", expression, result.ExceptionDetails.Text)
	}
	return fmt.Sprint(result.Result.Value)
}

func (b *browser) press(key, code string, keyCode, modifiers int) {
	b.t.Helper()
	for _, kind := range []string{"keyDown", "keyUp"} {
		params := map[string]any{"type": kind, "key": key, "code": code, "windowsVirtualKeyCode": keyCode, "modifiers": modifiers}
		b.call("Input.dispatchKeyEvent", params, nil)
	}
}

func (b *browser) shot(name string) {
	b.t.Helper()
	folder := os.Getenv("BROWSER_SHOTS")
	if folder == "" {
		return
	}
	var result struct{ Data string }
	b.call("Page.captureScreenshot", map[string]any{"format": "png"}, &result)
	picture, err := base64.StdEncoding.DecodeString(result.Data)
	if err != nil {
		b.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, name+".png"), picture, 0o644); err != nil {
		b.t.Fatal(err)
	}
}

// A 1x1 PNG.
const pixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

func setup(t *testing.T) (*browser, string) {
	t.Helper()
	return setupWith(t, web.Options{DevUI: true})
}

// setupWithDatabase starts the handler with a throwaway PostgreSQL behind it.
func setupWithDatabase(t *testing.T) (*browser, string) {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	return setupWith(t, web.Options{DevUI: true, DB: database})
}

func setupWith(t *testing.T, options web.Options) (*browser, string) {
	t.Helper()
	dir := t.TempDir()
	picture, err := base64.StdEncoding.DecodeString(pixel)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{
		"README.md": []byte("---\nid: start\ntitle: Start page\n---\n# Welcome\n\nSee [[plan]], [[missing]] and ![outside](https://example.com/a.png).\n\n" +
			"| Name | Value |\n|---|---:|\n| Height | 50 m |\n\n- [x] done\n- [ ] open\n\n> A quote.\n\n<script>window.injected = true</script>\n\n[bad](javascript:window.injected=true)\n"),
		"ops/net/plan.md": []byte("---\nid: plan\n---\n# Network plan\n\n![Map](map.png)\n\n## Devices\n\nRouter.\n\n## Übersicht\n\n```text\nrouter 192.0.2.1\n```\n\n[[start]]\n"),
		"ops/net/map.png": picture,
		"ops/plain.md":    []byte("No heading here.\n"),
	} {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	log := slog.New(slog.DiscardHandler)
	store := library.NewStore(folder.New(dir), log)
	if err := store.Refresh(); err != nil {
		t.Fatal(err)
	}
	handler, err := web.New(store, log, options)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	b := start(t)
	var target struct{ TargetID string }
	b.call("Target.createTarget", map[string]any{"url": "about:blank"}, &target)
	var session struct{ SessionID string }
	b.call("Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true}, &session)
	b.session = session.SessionID
	for _, domain := range []string{"Page.enable", "Network.enable", "Log.enable", "Runtime.enable"} {
		b.call(domain, map[string]any{}, nil)
	}
	// Without the cache every page loads its own assets, otherwise only the first would show an error.
	b.call("Network.setCacheDisabled", map[string]any{"cacheDisabled": true}, nil)
	b.signIn(true)
	return b, server.URL
}

// signIn sets the headers the gate in front would set, on every request of the browser.
func (b *browser) signIn(on bool) {
	b.t.Helper()
	headers := map[string]string{}
	if on {
		headers = map[string]string{"Remote-User": "anna", "Remote-Name": "Anna Berg", "Remote-Email": "anna@example.com", "Remote-Groups": "ops"}
	}
	b.call("Network.setExtraHTTPHeaders", map[string]any{"headers": headers}, nil)
}

type box struct{ X, Y, Left, Right, Top, Bottom, Width float64 }

// rect returns where an element is on the screen, for real pointer input.
func (b *browser) rect(selector string) box {
	b.t.Helper()
	raw := b.eval(`JSON.stringify((() => { const r = document.querySelector(` + strconv.Quote(selector) + `).getBoundingClientRect();
		return {X: r.left + r.width / 2, Y: r.top + r.height / 2, Left: r.left, Right: r.right, Top: r.top, Bottom: r.bottom, Width: r.width}; })())`)
	var r box
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		b.t.Fatalf("%s: %s", selector, raw)
	}
	return r
}

// mouse sends one real pointer event, as a hand on a mouse would.
func (b *browser) mouse(kind string, x, y float64, clicks int, modifiers int) {
	b.t.Helper()
	button, buttons := "none", 0
	if kind != "mouseMoved" {
		button = "left"
	}
	if kind == "mousePressed" {
		buttons = 1
	}
	b.call("Input.dispatchMouseEvent", map[string]any{"type": kind, "x": x, "y": y, "button": button, "buttons": buttons, "clickCount": clicks, "modifiers": modifiers}, nil)
}

func (b *browser) click(x, y float64, clicks int) {
	b.t.Helper()
	b.mouse("mouseMoved", x, y, 0, 0)
	b.mouse("mousePressed", x, y, clicks, 0)
	b.mouse("mouseReleased", x, y, clicks, 0)
}

func (b *browser) drag(from box, dx float64) {
	b.t.Helper()
	x, y := from.Right-2, from.Top+200
	b.mouse("mouseMoved", x, y, 0, 0)
	b.mouse("mousePressed", x, y, 1, 0)
	for step := 1.0; step <= 8; step++ {
		b.call("Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": x + dx*step/8, "y": y, "button": "left", "buttons": 1}, nil)
	}
	b.mouse("mouseReleased", x+dx, y, 1, 0)
}

// waitFor polls an expression until it is true, since a swap of the window loads in the background.
func (b *browser) waitFor(expression string) bool {
	b.t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if b.eval(expression) == "true" {
			return true
		}
	}
	return false
}

func TestEveryPageIsClean(t *testing.T) {
	b, base := setup(t)
	own, _ := url.Parse(base)
	for name, target := range map[string]string{
		"start":    "/o/start",
		"plan":     "/o/plan",
		"plain":    "/o/~ops/plain.md",
		"root":     "/f/",
		"folder":   "/f/ops",
		"search":   "/search?q=router",
		"nothing":  "/search?q=xyzzy",
		"tag":      "/search?tag=network",
		"problems": "/problems",
		"profile":  "/profile",
		"gallery":  "/dev/ui",
	} {
		f := b.open(base + target)
		if len(f.requests) < 5 {
			t.Errorf("%s: only %d requests, the collection is broken", name, len(f.requests))
		}
		for _, request := range f.requests {
			u, err := url.Parse(request)
			if err != nil || (u.Scheme != "data" && u.Host != own.Host) {
				t.Errorf("%s: request to a foreign origin: %s", name, request)
			}
		}
		for _, list := range [][]string{f.failed, f.csp, f.errors} {
			for _, entry := range list {
				t.Errorf("%s: %s", name, entry)
			}
		}
		if got := b.eval(`[...document.images].filter(i => !i.complete || !i.naturalWidth).map(i => i.src).join()`); got != "" {
			t.Errorf("%s: images did not load: %s", name, got)
		}
		if got := b.eval(`getComputedStyle(document.body).fontFamily`); !strings.Contains(got, "Inter") {
			t.Errorf("%s: the stylesheet did not apply, font is %q", name, got)
		}
		if got := b.eval(`document.fonts.check('16px Inter')`); got != "true" {
			t.Errorf("%s: the font did not load", name)
		}
		if got := b.eval(`getComputedStyle(document.querySelector('.workspace')).display`); got != "grid" {
			t.Errorf("%s: the shell stylesheet did not apply, display %q", name, got)
		}
		if got := b.eval(`[...document.querySelectorAll('.workspace-nav, .workspace-main')].map(n => n.getBoundingClientRect().width > 40).join()`); got != "true,true,true,true" {
			t.Errorf("%s: a column is missing: %s", name, got)
		}
		if got := b.eval(`document.documentElement.scrollWidth > innerWidth || document.body.scrollHeight > innerHeight`); got != "false" {
			t.Errorf("%s: the page itself scrolls, only the columns should", name)
		}
		b.shot(name)
	}
}

func TestNothingWithoutTheGate(t *testing.T) {
	b, base := setup(t)
	b.signIn(false)
	b.open(base + "/o/plan")
	if got := b.eval(`document.querySelector('h1').textContent`); got != "Not signed in" {
		t.Errorf("without the gate: %q", got)
	}
	if got := b.eval(`document.body.textContent.includes('Router')`); got != "false" {
		t.Error("content showed without the gate")
	}
	b.shot("signed-out")
}

func TestMarkdownCannotRunScripts(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/o/start")
	if got := b.eval(`String(window.injected)`); got != "undefined" {
		t.Errorf("a script from Markdown ran: %s", got)
	}
	if got := b.eval(`document.querySelectorAll('article script, article a[href^="javascript"], article img[src^="http"]').length`); got != "0" {
		t.Errorf("%s dangerous elements in the page", got)
	}
}

func TestCSPReallyBlocks(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/o/start")
	// The control for the clean pages above: an inline script injected here must be stopped and reported.
	b.eval(`(() => { const s = document.createElement('script'); s.textContent = 'window.ran = true'; document.head.append(s); })()`)
	if got := b.eval(`String(window.ran)`); got != "undefined" {
		t.Error("the CSP let an inline script run")
	}
	deadline := time.After(5 * time.Second)
	for {
		select {
		case m := <-b.events:
			if m.Method == "Log.entryAdded" && strings.Contains(string(m.Params), "Content Security Policy") {
				return
			}
		case <-deadline:
			t.Fatal("the violation was not reported, so the clean pages above prove nothing")
		}
	}
}

// On a phone, roughly until its own layout is settled (T-10): nav2 and nav3 open over the window from the active view.
func TestShellOnAPhone(t *testing.T) {
	b, base := setup(t)
	b.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": 390, "height": 844, "deviceScaleFactor": 1, "mobile": true}, nil)
	b.open(base + "/o/plan")
	visible := `[...document.querySelectorAll('.workspace-nav[data-level="2"], .workspace-nav[data-level="3"]')].map(n => getComputedStyle(n).display !== 'none').join()`
	if got := b.eval(visible); got != "false,false" {
		t.Errorf("nav2 and nav3 should start closed on a phone: %s", got)
	}
	if got := b.eval(`document.documentElement.scrollWidth > innerWidth`); got != "false" {
		t.Error("the page scrolls sideways on a phone")
	}
	if got := b.eval(`document.querySelector('.workspace-main').getBoundingClientRect().width > 250`); got != "true" {
		t.Error("the window is too narrow to read")
	}
	view := b.rect(`.workspace-nav[data-level="1"] [data-ud-workspace]`)
	b.click(view.X, view.Y, 1)
	if got := b.eval(visible); got != "true,true" {
		t.Errorf("the active view should open nav2 and nav3: %s", got)
	}
	b.shot("phone-open")
	b.click(view.X, view.Y, 1)
	if got := b.eval(visible); got != "false,false" {
		t.Errorf("a second tap closes them: %s", got)
	}
}

func TestTreeFollowsThePage(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/o/plan")
	if got := b.eval(`document.querySelector('.workspace-nav[data-level="2"] [aria-current]').textContent.trim()`); got != "ops" {
		t.Errorf("collection: %s", got)
	}
	if got := b.eval(`[...document.querySelectorAll('.workspace-nav[data-level="3"] [aria-expanded="true"]')].map(a => a.closest('li').querySelector('a').textContent.trim()).join()`); got != "net" {
		t.Errorf("open folders: %s", got)
	}
	if got := b.eval(`document.querySelector('.workspace-nav[data-level="3"] [aria-current="page"]').textContent.trim()`); got != "Network plan" {
		t.Errorf("current page: %s", got)
	}
	if got := b.eval(`[...document.querySelectorAll('.breadcrumb a')].map(a => a.textContent.trim()).join('/') + '|' + document.querySelector('.workspace-main h1').textContent`); got != "ops/net|Network plan" {
		t.Errorf("head: %s", got)
	}
}

// A click on a name opens the object and keeps the list as it is. A double click or the arrow shows or hides what is in
// it (ADR-16), with real pointer input.
func TestClickOpensDoubleClickUnfolds(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/o/plan")
	b.eval(`window.sameLoad = true`)
	plain := b.rect(`.workspace-nav[data-level="3"] a[href="/o/~ops/plain.md"]`)
	b.click(plain.X, plain.Y, 1)
	if !b.waitFor(`location.pathname === '/o/~ops/plain.md' && document.querySelector('.workspace-main h1').textContent === 'plain'`) {
		t.Fatalf("the click did not open the page: %s", b.eval(`location.pathname`))
	}
	if got := b.eval(`String(window.sameLoad) + document.querySelector('.workspace-nav[data-level="3"] [aria-current="page"]').textContent.trim() + document.title`); got != "trueplainplain · UnderDoc" {
		t.Errorf("the window should swap without a reload: %s", got)
	}
	hidden := `document.querySelector('.workspace-nav[data-level="3"] a[href="/f/ops/net"]').closest('li').querySelector('ul').hidden`
	if got := b.eval(hidden); got != "false" {
		t.Fatal("net should start open, it holds the page that was open")
	}
	net := b.rect(`.workspace-nav[data-level="3"] a[href="/f/ops/net"]`)
	b.mouse("mouseMoved", net.X, net.Y, 0, 0)
	for _, clicks := range []int{1, 2} {
		b.mouse("mousePressed", net.X, net.Y, clicks, 0)
		b.mouse("mouseReleased", net.X, net.Y, clicks, 0)
	}
	if !b.waitFor(hidden) {
		t.Error("the double click did not close the folder")
	}
	arrow := b.rect(`.workspace-nav[data-level="3"] a[href="/f/ops/net"]`)
	b.click(arrow.Left-10, arrow.Y, 1)
	if got := b.eval(hidden); got != "false" {
		t.Error("the arrow did not open it again")
	}
	b.eval(`history.back()`)
	if !b.waitFor(`location.pathname === '/f/ops/net' || location.pathname === '/o/~ops/plain.md' || location.pathname === '/o/plan'`) {
		t.Error("back did not work")
	}
}

// The right edge of each column can be dragged, nav1 and nav2 snap to four steps, nav3 moves freely, and the widths
// stay after a reload (ADR-16). The behavior is C22's workspace; this proves UnderDoc wires it up.
func TestColumnsSnapByDragging(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/o/plan")
	b.eval(`localStorage.clear()`)
	b.open(base + "/o/plan")
	const nav1, nav2, nav3 = `.workspace-nav[data-level="1"]`, `.workspace-nav[data-level="2"]`, `.workspace-nav[data-level="3"]`
	first, second := b.rect(nav1), b.rect(nav2)
	if first.Width != 50 || second.Width != 172 {
		t.Fatalf("nav1 starts at the icon step and nav2 beside it, as in the mockup: %v, %v", first.Width, second.Width)
	}
	b.drag(first, 70)
	if got := b.eval(`document.querySelector('` + nav1 + `').dataset.step + ':' + document.querySelector('` + nav1 + `').getBoundingClientRect().width`); got != "2:116" {
		t.Errorf("nav1 should snap to the third step: %s", got)
	}
	if moved := b.rect(nav2); moved.Left <= second.Left {
		t.Error("nav1 should push nav2")
	}
	list := b.rect(nav3)
	b.drag(list, 37)
	if got := b.rect(nav3).Width; got < list.Width+30 || got > list.Width+44 {
		t.Errorf("nav3 moves freely: %v to %v", list.Width, got)
	}
	b.shot("dragged")
	b.open(base + "/o/plan")
	if got := b.eval(`document.querySelector('` + nav1 + `').dataset.step`); got != "2" {
		t.Errorf("the width did not stay: %s", got)
	}
}

func TestTableOfContentsJumps(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/o/plan")
	marker := b.rect("#ud-toc-trigger")
	b.click(marker.X, marker.Y, 1)
	if !b.waitFor(`document.getElementById('ud-toc-trigger').getAttribute('aria-expanded') === 'true'`) {
		t.Fatal("the marker did not open")
	}
	if got := b.eval(`[...document.querySelectorAll('#ud-toc-menu a')].map(a => document.getElementById(decodeURIComponent(a.hash.slice(1))) ? 'ok' : a.hash).join()`); got != "ok,ok" {
		t.Errorf("entries without a target: %s", got)
	}
	b.press("Escape", "Escape", 27, 0)
	if !b.waitFor(`document.getElementById('ud-toc-trigger').getAttribute('aria-expanded') === 'false'`) {
		t.Error("Escape should close it")
	}
}

func TestGearOpensTheMetadata(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/o/plan")
	gear := b.rect(`[role=toolbar] [data-drawer-toggle="ud-drawer"]`)
	b.click(gear.X, gear.Y, 1)
	if got := b.eval(`!document.getElementById('ud-drawer').hidden && document.getElementById('ud-drawer').textContent.includes('/o/plan')`); got != "true" {
		t.Fatal("the gear did not open the metadata")
	}
	b.shot("gear")
	b.press("Escape", "Escape", 27, 0)
	if got := b.eval(`document.getElementById('ud-drawer').hidden`); got != "true" {
		t.Error("Escape should close it")
	}
}

func TestSearchShortcuts(t *testing.T) {
	const ctrl = 2
	b, base := setup(t)
	focused := `document.activeElement.name`
	for name, press := range map[string]func(){
		"slash":  func() { b.press("/", "Slash", 191, 0) },
		"ctrl+k": func() { b.press("k", "KeyK", 75, ctrl) },
	} {
		b.open(base + "/o/plan")
		if got := b.eval(focused); got == "q" {
			t.Fatalf("%s: the field already has the focus, the test proves nothing", name)
		}
		press()
		if got := b.eval(focused); got != "q" {
			t.Errorf("%s: focus is on %q", name, got)
		}
	}
	// While typing, "/" stays a character.
	b.open(base + "/o/plan")
	b.eval(`document.querySelector('input[name="q"]').focus()`)
	b.call("Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "/", "code": "Slash", "windowsVirtualKeyCode": 191, "text": "/"}, nil)
	b.call("Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "/", "code": "Slash", "windowsVirtualKeyCode": 191}, nil)
	if got := b.eval(`document.querySelector('input[name="q"]').value`); got != "/" {
		t.Errorf("typing a slash gave %q", got)
	}
}

func TestSearchFromTheField(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/o/start")
	b.eval(`(() => { const f = document.querySelector('form[role=search]'); f.q.value = 'router'; f.submit(); })()`)
	b.collect()
	if got := b.eval(`location.pathname + location.search`); got != "/search?q=router" {
		t.Errorf("landed on %s", got)
	}
	if got := b.eval(`document.querySelector('mark').textContent`); !strings.EqualFold(got, "router") {
		t.Errorf("marked %q", got)
	}
}

func TestDarkModeFollowsTheSystemOrTheSetting(t *testing.T) {
	b, base := setup(t)
	b.call("Emulation.setEmulatedMedia", map[string]any{"features": []map[string]string{{"name": "prefers-color-scheme", "value": "dark"}}}, nil)
	b.open(base + "/o/plan")
	b.eval(`localStorage.clear()`)
	b.open(base + "/o/plan")
	dark := b.eval(`getComputedStyle(document.body).backgroundColor`)
	if got := b.eval(`document.documentElement.classList.contains('dark')`); got != "true" {
		t.Error("the dark class is missing")
	}
	b.shot("dark")
	b.call("Emulation.setEmulatedMedia", map[string]any{"features": []map[string]string{{"name": "prefers-color-scheme", "value": "light"}}}, nil)
	b.open(base + "/o/plan")
	if light := b.eval(`getComputedStyle(document.body).backgroundColor`); light == dark {
		t.Errorf("light and dark look the same: %s", light)
	}
	// The setting in the profile wins over the system, and stays after a reload.
	b.open(base + "/profile?s=appearance")
	button := b.rect(`[data-ud-theme="dark"]`)
	b.click(button.X, button.Y, 1)
	if got := b.eval(`document.documentElement.classList.contains('dark') + ':' + document.querySelector('[data-ud-theme="dark"]').getAttribute('aria-pressed')`); got != "true:true" {
		t.Errorf("the setting did not apply: %s", got)
	}
	b.open(base + "/o/plan")
	if got := b.eval(`document.documentElement.classList.contains('dark')`); got != "true" {
		t.Error("the setting did not stay")
	}
}

// nav1 and nav2 open things in the columns, the window keeps what it shows (ADR-16): into the profile and back.
func TestColumnsChangeTheWindowStays(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/o/plan")
	b.eval(`window.sameLoad = true`)
	const nav1, nav2, nav3 = `.workspace-nav[data-level="1"]`, `.workspace-nav[data-level="2"]`, `.workspace-nav[data-level="3"]`
	window := `document.querySelector('.workspace-main h1').textContent + '|' + location.pathname`
	profile := b.rect(nav1 + ` a[href="/profile"]`)
	b.click(profile.X, profile.Y, 1)
	if !b.waitFor(`document.querySelector('` + nav2 + `').getAttribute('aria-label') === 'Profile'`) {
		t.Fatal("the profile did not open in nav2")
	}
	if got := b.eval(window + ` + '|' + window.sameLoad + '|' + !!document.querySelector('` + nav3 + ` [href="/.tinysesam/logout"]')`); got != "Network plan|/o/plan|true|true" {
		t.Errorf("the window should stay, the account show in nav3: %s", got)
	}
	appearance := b.rect(nav2 + ` a[href="/profile?s=appearance"]`)
	b.click(appearance.X, appearance.Y, 1)
	if !b.waitFor(`!!document.querySelector('` + nav3 + ` [data-ud-theme="dark"]')`) {
		t.Fatal("the section did not open in nav3")
	}
	b.shot("profile-view")
	workspace := b.rect(nav1 + ` a[data-ud-workspace]`)
	b.click(workspace.X, workspace.Y, 1)
	if !b.waitFor(`document.querySelector('` + nav2 + `').getAttribute('aria-label') === 'Collections'`) {
		t.Fatal("back to the workspace did not work")
	}
	if got := b.eval(`document.querySelector('` + nav3 + ` [aria-current="page"]').textContent.trim() + '|' + ` + window); got != "Network plan|Network plan|/o/plan" {
		t.Errorf("the workspace should show the collection of the window: %s", got)
	}
	home := b.rect(nav2 + ` a[title="Home"]`)
	b.click(home.X, home.Y, 1)
	if !b.waitFor(`document.querySelector('` + nav3 + ` section > header > a').textContent === 'Home'`) {
		t.Fatal("a collection in nav2 did not open in nav3")
	}
	if got := b.eval(window + ` + '|' + window.sameLoad`); got != "Network plan|/o/plan|true" {
		t.Errorf("a collection must not change the window: %s", got)
	}
}

// A right click on an entry in nav3 opens its menu, named after the entry; Open opens it in the window (ADR-16).
func TestRightClickOnAnEntry(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/o/plan")
	plain := b.rect(`.workspace-nav[data-level="3"] a[href="/o/~ops/plain.md"]`)
	b.mouse("mouseMoved", plain.X, plain.Y, 0, 0)
	b.call("Input.dispatchMouseEvent", map[string]any{"type": "mousePressed", "x": plain.X, "y": plain.Y, "button": "right", "buttons": 2, "clickCount": 1}, nil)
	b.call("Input.dispatchMouseEvent", map[string]any{"type": "mouseReleased", "x": plain.X, "y": plain.Y, "button": "right", "buttons": 0, "clickCount": 1}, nil)
	panel := `document.querySelector('.workspace-nav[data-level="3"] [data-cm-panel]')`
	if !b.waitFor(`!` + panel + `.hidden && ` + panel + `.querySelector('[role=heading]').textContent === 'plain'`) {
		t.Fatalf("the menu of the entry did not open: %s", b.eval(panel+`.outerHTML.slice(0, 200)`))
	}
	if got := b.eval(panel + `.querySelector('[aria-disabled="true"]') !== null`); got != "true" {
		t.Error("what is not built yet should be greyed out in the menu")
	}
	b.shot("row-menu")
	open := b.rect(`.workspace-nav[data-level="3"] [data-ud-row="open"]`)
	b.click(open.X, open.Y, 1)
	if !b.waitFor(`location.pathname === '/o/~ops/plain.md'`) {
		t.Error("Open in the menu did not open the entry")
	}
}

func TestMain(m *testing.M) { os.Exit(pgtest.Main(m)) }

// New collection in nav2, by mouse and keyboard: the popover, a name, Enter, and the new collection is open (M-28).
func TestNewCollectionByHand(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	const nav2 = `.workspace-nav[data-level="2"]`
	if got := b.eval(`!!document.querySelector('` + nav2 + ` [title="My notes, only you see it"]')`); got != "true" {
		t.Fatal("the private notes of the account are missing")
	}
	trigger := b.rect("#ud-new-top-trigger")
	b.click(trigger.X, trigger.Y, 1)
	if !b.waitFor(`document.getElementById('ud-new-top-popover').getAttribute('aria-hidden') === 'false'`) {
		t.Fatal("the form did not open")
	}
	// The popover fades and moves in. A click before it stands still could miss the field and close it again.
	if !b.waitFor(`(() => { const p = document.getElementById('ud-new-top-popover'), r = p.getBoundingClientRect();
		const still = window.lastBox === r.top + ',' + r.left; window.lastBox = r.top + ',' + r.left;
		return still && getComputedStyle(p).opacity === '1' && r.height > 0; })()`) {
		t.Fatal("the form did not come to rest")
	}
	field := b.rect("#ud-new-top-title")
	b.click(field.X, field.Y, 1)
	if got := b.eval(`document.activeElement.id`); got != "ud-new-top-title" {
		t.Fatalf("the name field did not get the focus: %q", got)
	}
	b.call("Input.insertText", map[string]any{"text": "Operations"}, nil)
	if got := b.eval(`document.getElementById('ud-new-top-title').value`); got != "Operations" {
		t.Fatalf("typed %q", got)
	}
	b.shot("new-collection")
	// A real Enter carries the character "\r", and only that submits a form.
	b.call("Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13, "text": "\r"}, nil)
	b.call("Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13}, nil)
	// Asking the page while the form is sent keeps the old page alive in Chrome, so wait a moment first.
	time.Sleep(500 * time.Millisecond)
	if !b.waitFor(`/^\/o\/[0-9a-z]{10}$/.test(location.pathname) && document.querySelector('.workspace-main h1')?.textContent === 'Operations'`) {
		t.Fatalf("the new collection did not open: %s", b.eval(`location.pathname`))
	}
	if got := b.eval(`document.querySelector('` + nav2 + ` [aria-current]')?.textContent.trim() + '|' + document.querySelector('.workspace-nav[data-level="3"] section > header > a').textContent`); got != "Operations|Operations" {
		t.Errorf("nav2 and nav3 should show it: %s", got)
	}
	b.shot("collection")
}
