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
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

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
	handler, err := web.New(store, log, web.Options{DevUI: true})
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
	return b, server.URL
}

func TestEveryPageIsClean(t *testing.T) {
	b, base := setup(t)
	own, _ := url.Parse(base)
	for name, target := range map[string]string{
		"start":    "/d/start",
		"plan":     "/d/plan",
		"plain":    "/d/~ops/plain.md",
		"root":     "/f/",
		"folder":   "/f/ops",
		"search":   "/search?q=router",
		"nothing":  "/search?q=xyzzy",
		"problems": "/problems",
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
		if got := b.eval(`getComputedStyle(document.querySelector('aside.sidebar nav')).position`); got != "fixed" {
			t.Errorf("%s: sidebar position %q", name, got)
		}
		if got := b.eval(`document.querySelector('aside.sidebar').dataset.sidebarInitialized`); got != "true" {
			t.Errorf("%s: the sidebar script did not run", name)
		}
		if got := b.eval(`document.documentElement.scrollWidth > innerWidth`); got != "false" {
			t.Errorf("%s: the page scrolls sideways", name)
		}
		b.shot(name)
	}
}

func TestMarkdownCannotRunScripts(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/d/start")
	if got := b.eval(`String(window.injected)`); got != "undefined" {
		t.Errorf("a script from Markdown ran: %s", got)
	}
	if got := b.eval(`document.querySelectorAll('article script, article a[href^="javascript"], article img[src^="http"]').length`); got != "0" {
		t.Errorf("%s dangerous elements in the page", got)
	}
}

func TestCSPReallyBlocks(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/d/start")
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

func TestSidebarOnAPhone(t *testing.T) {
	b, base := setup(t)
	b.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": 390, "height": 844, "deviceScaleFactor": 1, "mobile": true}, nil)
	b.open(base + "/d/plan")
	if got := b.eval(`document.querySelector('aside.sidebar').getAttribute('aria-hidden')`); got != "true" {
		t.Errorf("the sidebar should start closed on a phone, aria-hidden=%s", got)
	}
	if got := b.eval(`document.documentElement.scrollWidth > innerWidth`); got != "false" {
		t.Error("the page scrolls sideways on a phone")
	}
	if got := b.eval(`getComputedStyle(document.querySelector('nav.toc')).display`); got != "none" {
		t.Errorf("the table of contents should be hidden on a phone, display=%s", got)
	}
	b.eval(`document.querySelector('[data-sidebar-toggle]').click()`)
	if got := b.eval(`document.querySelector('aside.sidebar').getAttribute('aria-hidden')`); got != "false" {
		t.Error("the button did not open the sidebar")
	}
	// The sidebar slides in, so wait for the end of the animation.
	time.Sleep(500 * time.Millisecond)
	if got := b.eval(`document.querySelector('aside.sidebar nav').getBoundingClientRect().left`); got != "0" {
		t.Errorf("the open sidebar is at x=%s", got)
	}
	b.shot("phone-sidebar")
	b.eval(`document.querySelector('[data-sidebar-toggle]').click()`)
	if got := b.eval(`document.querySelector('aside.sidebar').getAttribute('aria-hidden')`); got != "true" {
		t.Error("the button did not close the sidebar")
	}
}

func TestTreeFollowsThePage(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/d/plan")
	if got := b.eval(`[...document.querySelectorAll('aside details[open] > summary')].map(s => s.textContent.trim()).join()`); got != "ops,net" {
		t.Errorf("open folders: %s", got)
	}
	if got := b.eval(`document.querySelector('aside a[aria-current="page"]').textContent.trim()`); got != "Network plan" {
		t.Errorf("current page: %s", got)
	}
	if got := b.eval(`document.querySelector('.breadcrumb [aria-current="page"]').textContent.trim()`); got != "Network plan" {
		t.Errorf("breadcrumb: %s", got)
	}
}

func TestTableOfContentsJumps(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/d/plan")
	if got := b.eval(`getComputedStyle(document.querySelector('nav.toc')).display`); got != "flex" {
		t.Errorf("table of contents display=%s", got)
	}
	if got := b.eval(`[...document.querySelectorAll('nav.toc a')].map(a => document.getElementById(decodeURIComponent(a.hash.slice(1))) ? 'ok' : a.hash).join()`); got != "ok,ok" {
		t.Errorf("entries without a target: %s", got)
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
		b.open(base + "/d/plan")
		if got := b.eval(focused); got == "q" {
			t.Fatalf("%s: the field already has the focus, the test proves nothing", name)
		}
		press()
		if got := b.eval(focused); got != "q" {
			t.Errorf("%s: focus is on %q", name, got)
		}
	}
	// While typing, "/" stays a character.
	b.open(base + "/d/plan")
	b.eval(`document.querySelector('input[name="q"]').focus()`)
	b.call("Input.dispatchKeyEvent", map[string]any{"type": "keyDown", "key": "/", "code": "Slash", "windowsVirtualKeyCode": 191, "text": "/"}, nil)
	b.call("Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": "/", "code": "Slash", "windowsVirtualKeyCode": 191}, nil)
	if got := b.eval(`document.querySelector('input[name="q"]').value`); got != "/" {
		t.Errorf("typing a slash gave %q", got)
	}
}

func TestSearchFromTheField(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/d/start")
	b.eval(`(() => { const f = document.querySelector('form[role=search]'); f.q.value = 'router'; f.submit(); })()`)
	b.collect()
	if got := b.eval(`location.pathname + location.search`); got != "/search?q=router" {
		t.Errorf("landed on %s", got)
	}
	if got := b.eval(`document.querySelector('mark').textContent`); !strings.EqualFold(got, "router") {
		t.Errorf("marked %q", got)
	}
}

func TestDarkModeFollowsTheSystem(t *testing.T) {
	b, base := setup(t)
	b.call("Emulation.setEmulatedMedia", map[string]any{"features": []map[string]string{{"name": "prefers-color-scheme", "value": "dark"}}}, nil)
	b.open(base + "/d/plan")
	dark := b.eval(`getComputedStyle(document.body).backgroundColor`)
	if got := b.eval(`document.documentElement.classList.contains('dark')`); got != "true" {
		t.Error("the dark class is missing")
	}
	b.shot("dark")
	b.call("Emulation.setEmulatedMedia", map[string]any{"features": []map[string]string{{"name": "prefers-color-scheme", "value": "light"}}}, nil)
	b.open(base + "/d/plan")
	if light := b.eval(`getComputedStyle(document.body).backgroundColor`); light == dark {
		t.Errorf("light and dark look the same: %s", light)
	}
}
