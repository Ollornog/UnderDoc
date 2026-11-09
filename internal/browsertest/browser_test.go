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
	// exited closes when Chrome ends; stderr keeps the end of what it wrote, so a silent Chrome can say why.
	exited  chan struct{}
	exitErr error
	stderr  *tail
}

// tail keeps the last bytes written to it.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (w *tail) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	if len(w.buf) > 4096 {
		w.buf = w.buf[len(w.buf)-4096:]
	}
	return len(p), nil
}

func (w *tail) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.buf)
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
	stderr := &tail{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	_ = toChrome.Close()
	_ = chromeWrites.Close()
	b := &browser{t: t, out: weWrite, waiting: map[int]chan message{}, events: make(chan message, 4096), exited: exited, stderr: stderr}
	go func() {
		b.exitErr = cmd.Wait()
		close(exited)
	}()
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
		<-exited
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
		b.t.Fatalf("%s: %v; %s", method, err, b.why())
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
		b.t.Fatalf("%s: no answer; %s", method, b.why())
	}
}

// why says whether Chrome still runs and what it wrote last, for a call it did not answer.
func (b *browser) why() string {
	state := "Chrome still runs"
	select {
	case <-b.exited:
		state = fmt.Sprintf("Chrome has ended (%v)", b.exitErr)
	default:
	}
	return state + "; the end of its stderr:\n" + b.stderr.String()
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
	return setupWith(t, web.Options{DevUI: true}, nil)
}

// setupWithDatabase starts the handler with a throwaway PostgreSQL behind it.
func setupWithDatabase(t *testing.T) (*browser, string) {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	return setupWith(t, web.Options{DevUI: true, DB: database}, nil)
}

func setupWith(t *testing.T, options web.Options, extra map[string]string) (*browser, string) {
	t.Helper()
	dir := t.TempDir()
	picture, err := base64.StdEncoding.DecodeString(pixel)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"README.md": []byte("---\nid: start\ntitle: Start page\n---\n# Welcome\n\nSee [[plan]], [[missing]] and ![outside](https://example.com/a.png).\n\n" +
			"| Name | Value |\n|---|---:|\n| Height | 50 m |\n\n- [x] done\n- [ ] open\n\n> A quote.\n\n<script>window.injected = true</script>\n\n[bad](javascript:window.injected=true)\n"),
		"ops/net/plan.md": []byte("---\nid: plan\n---\n# Network plan\n\n![Map](map.png)\n\n## Devices\n\nRouter.\n\n## Übersicht\n\n```text\nrouter 192.0.2.1\n```\n\n[[start]]\n"),
		"ops/net/map.png": picture,
		"ops/plain.md":    []byte("No heading here.\n"),
	}
	for name, content := range extra {
		files[name] = []byte(content)
	}
	for name, content := range files {
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

// ready waits until the workspace has drawn itself and, on the address of a page, has loaded that page.
func (b *browser) ready(title string) {
	b.t.Helper()
	check := `!!document.querySelector('#root .dtitle, #root .n1top')`
	if title != "" {
		check += ` && document.querySelector('.dtitle h1')?.textContent === ` + strconv.Quote(title) + ` && !/^(Loading|This page could not)/.test(document.querySelector('article.doc')?.textContent.trim())`
	}
	if !b.waitFor(check) {
		b.t.Fatalf("the workspace did not show %q: %s", title, b.eval(`document.querySelector('.dtitle h1')?.textContent + ' | ' + location.pathname`))
	}
}

// probe marks the first element of a selector with exactly this text, so a test needs no ID of the workspace.
func probe(b *browser, selector, text string) string {
	b.t.Helper()
	b.eval(`document.querySelectorAll('[data-probe]').forEach(n => n.removeAttribute('data-probe')); [...document.querySelectorAll(` + strconv.Quote(selector) + `)].find(n => n.textContent.trim() === ` + strconv.Quote(text) + `)?.setAttribute('data-probe', '')`)
	return selector + "[data-probe]"
}

// clickOn clicks the middle of an element with the real mouse.
func (b *browser) clickOn(selector string, clicks int) {
	b.t.Helper()
	if b.eval(`!!document.querySelector(`+strconv.Quote(selector)+`)`) != "true" {
		b.t.Fatalf("%s is not on the page; nav2: %s; nav3: %s", selector, b.eval(`document.querySelector('.nav2')?.innerText.replace(/\s+/g, ' ')`), b.eval(`[...document.querySelectorAll('.nav3 [data-id]')].map(n => n.dataset.id).join(' ')`))
	}
	r := b.rect(selector)
	b.mouse("mouseMoved", r.X, r.Y, 0, 0)
	for n := 1; n <= clicks; n++ {
		b.mouse("mousePressed", r.X, r.Y, n, 0)
		b.mouse("mouseReleased", r.X, r.Y, n, 0)
	}
}

func TestEveryPageIsClean(t *testing.T) {
	b, base := setup(t)
	own, _ := url.Parse(base)
	for name, c := range map[string]struct{ target, title string }{
		"home":    {"/", "Start page"},
		"start":   {"/o/start", "Start page"},
		"plan":    {"/o/plan", "Network plan"},
		"plain":   {"/o/~ops/plain.md", "plain"},
		"folder":  {"/f/ops", ""},
		"profile": {"/profile", ""},
		"gallery": {"/dev/ui", ""},
	} {
		f := b.open(base + c.target)
		b.ready(c.title)
		if len(f.requests) < 4 {
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
		// An image may still be on its way under load: wait for all of them, then name any that failed.
		b.waitFor(`[...document.images].every(i => i.complete)`)
		if got := b.eval(`[...document.images].filter(i => !i.complete || !i.naturalWidth).map(i => i.src).join()`); got != "" {
			t.Errorf("%s: images did not load: %s", name, got)
		}
		if got := b.eval(`getComputedStyle(document.body).fontFamily`); !strings.Contains(got, "Inter") {
			t.Errorf("%s: the stylesheet did not apply, font is %q", name, got)
		}
		if got := b.eval(`document.fonts.check('15px Inter')`); got != "true" {
			t.Errorf("%s: the font did not load", name)
		}
		// The look of the mockup comes from C22 under its root class: the sizes of the approved mockup.
		if c.title == "" {
			b.shot(name)
			continue
		}
		if got := b.eval(`getComputedStyle(document.querySelector('.nav3 .row') || document.body).height + ' ' + getComputedStyle(document.querySelector('.nav3 .row .name') || document.body).fontSize`); got != "30px 14px" {
			t.Errorf("%s: a row of nav3 is %s, the mockup has 30px 14px", name, got)
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
	b.ready("Start page")
	if got := b.eval(`String(window.injected)`); got != "undefined" {
		t.Errorf("a script from Markdown ran: %s", got)
	}
	if got := b.eval(`document.querySelectorAll('article.doc table').length + ' ' + document.querySelectorAll('article script, article a[href^="javascript"], article img[src^="http"]').length`); got != "1 0" {
		t.Errorf("the table should show and nothing dangerous: %s", got)
	}
}

func TestCSPReallyBlocks(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/o/start")
	b.ready("Start page")
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

// Titles, descriptions, tags and folder names come from files and from people: the workspace shows them as text,
// wherever it shows them.
func TestNamesStayText(t *testing.T) {
	b, base := setupWith(t, web.Options{DevUI: true}, map[string]string{
		// The slash of </i> makes a folder inside a folder: the name of the outer one is <i class="x1">evil<.
		`<i class="x1">evil</i>/page.md`: "---\nid: evil\ntitle: '<b class=\"x2\">Bold</b>'\ndescription: '<u class=\"x3\">desc</u>'\ntags: ['<s class=\"x4\">tag</s>']\n---\nSome evil text.\n",
	})
	b.open(base + "/o/evil")
	b.ready(`<b class="x2">Bold</b>`)
	b.eval(`[...document.querySelectorAll('.nav2 .navitem')].find(n => n.textContent.includes('evil')).setAttribute('data-probe', '')`)
	b.clickOn(`.nav2 [data-probe]`, 1)
	b.clickOn(`.nav3 .chead`, 1)
	if !b.waitFor(`location.pathname === "/f/%3Ci%20class%3D%22x1%22%3Eevil%3C"`) {
		t.Errorf("the address of the folder: %s", b.eval(`location.pathname`))
	}
	b.clickOn(`[data-act="gear"]`, 1)
	b.eval(`(() => { const f = document.getElementById('search'); f.value = 'evil'; f.dispatchEvent(new Event('input', {bubbles: true})); })()`)
	time.Sleep(600 * time.Millisecond)
	if got := b.eval(`document.querySelectorAll('.x1, .x2, .x3, .x4').length`); got != "0" {
		t.Errorf("%s names became markup: %s", got, b.eval(`[...document.querySelectorAll('.x1, .x2, .x3, .x4')].map(n => n.parentElement.outerHTML.slice(0, 160)).join(' || ')`))
	}
	b.shot("names-stay-text")
}

// On a phone the workspace keeps the layout of the mockup for phones: nav1 at the side, the window readable (T-10).
func TestWorkspaceOnAPhone(t *testing.T) {
	b, base := setup(t)
	b.call("Emulation.setDeviceMetricsOverride", map[string]any{"width": 390, "height": 844, "deviceScaleFactor": 1, "mobile": true}, nil)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	if got := b.eval(`document.documentElement.scrollWidth > innerWidth`); got != "false" {
		t.Error("the page scrolls sideways on a phone")
	}
	if got := b.eval(`document.querySelector('article.doc').getBoundingClientRect().width > 250`); got != "true" {
		t.Error("the window is too narrow to read")
	}
	b.shot("phone")
}

// A click on an entry in nav3 opens it, the address follows; a double click unfolds a folder (ADR-17).
func TestClickOpensDoubleClickUnfolds(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.clickOn(probe(b, `.nav3 .name`, "plain"), 1)
	b.ready("plain")
	if got := b.eval(`location.pathname`); got != "/o/~ops/plain.md" {
		t.Errorf("the address did not follow: %s", got)
	}
	plan := `[...document.querySelectorAll('.nav3 .name')].some(n => n.textContent === 'Network plan')`
	if got := b.eval(plan); got != "true" {
		t.Fatal("the open page should show where it is")
	}
	b.clickOn(probe(b, `.nav3 .name`, "net"), 2)
	if !b.waitFor(`!` + plan) {
		t.Error("a double click should fold the folder")
	}
	b.clickOn(probe(b, `.nav3 .name`, "net"), 2)
	if !b.waitFor(plan) {
		t.Error("a second double click should unfold it again")
	}
	b.eval(`history.back()`)
	b.ready("plain")
}

// A link inside a page opens its target in the window, as a click in the list does.
func TestLinksInAPageStayInTheWorkspace(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/o/start")
	b.ready("Start page")
	b.eval(`window.sameLoad = true`)
	b.clickOn(`article.doc a[href="/o/plan"]`, 1)
	b.ready("Network plan")
	if got := b.eval(`location.pathname + ' ' + window.sameLoad`); got != "/o/plan true" {
		t.Errorf("the link left the workspace: %s", got)
	}
}

// The search field finds what the server knows, also pages that are not loaded yet.
func TestSearch(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/o/start")
	b.ready("Start page")
	b.press("/", "Slash", 191, 0)
	if got := b.eval(`document.activeElement.id`); got != "search" {
		t.Errorf("/ should focus the search: %q", got)
	}
	// Only the page that is not loaded yet has this word, so only the server can find it.
	b.call("Input.insertText", map[string]any{"text": "Übersicht"}, nil)
	if !b.waitFor(`document.getElementById('sres')?.textContent.includes('Network plan')`) {
		t.Fatalf("the server hit is missing: %s", b.eval(`document.querySelector('main, #root').textContent.slice(0, 300)`))
	}
	if got := b.eval(`!!document.querySelector('mark')`); got != "true" {
		t.Error("the hit should mark the word")
	}
	b.shot("search")
}

// The theme follows the system until it is set (ADR-16).
func TestDarkModeFollowsTheSystem(t *testing.T) {
	b, base := setup(t)
	b.call("Emulation.setEmulatedMedia", map[string]any{"features": []map[string]string{{"name": "prefers-color-scheme", "value": "dark"}}}, nil)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	if got := b.eval(`document.documentElement.classList.contains('dark') && getComputedStyle(document.body).backgroundColor !== 'rgb(255, 255, 255)'`); got != "true" {
		t.Error("the system asks for dark")
	}
	b.shot("dark")
	b.call("Emulation.setEmulatedMedia", map[string]any{"features": []map[string]string{{"name": "prefers-color-scheme", "value": "light"}}}, nil)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	if got := b.eval(`document.documentElement.classList.contains('dark')`); got != "false" {
		t.Error("the system asks for light")
	}
}

func TestMain(m *testing.M) { os.Exit(pgtest.Main(m)) }

// New in nav2 makes a collection in the database at once, numbered, and its name is written in place; a new name stays
// (M-28).
func TestNewCollectionByHand(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	if got := b.eval(`[...document.querySelectorAll('.nav2 .name, .nav2 button')].some(n => n.textContent.trim() === 'My notes')`); got != "true" {
		t.Fatal("the private notes of the account are missing")
	}
	newIn := func() {
		b.clickOn(`.nav2 [data-act="addmenu"][data-id="n2"]`, 1)
		if !b.waitFor(`document.activeElement?.dataset?.nfilter !== undefined`) {
			t.Fatalf("the filter of New did not get the focus: %q", b.eval(`document.activeElement?.outerHTML.slice(0, 120)`))
		}
		b.clickOn(probe(b, `.cmdpop [role="menuitem"]`, "Collection"), 1)
	}
	newIn()
	if !b.waitFor(`document.activeElement?.dataset?.ren !== undefined`) {
		t.Fatalf("the name is not written in place: %q", b.eval(`document.activeElement?.outerHTML.slice(0, 120)`))
	}
	if got := b.eval(`[document.activeElement.value, document.activeElement.selectionStart, document.activeElement.selectionEnd].join('|')`); got != "New collection (1)|0|18" {
		t.Errorf("the first new collection is numbered, its name selected: %q", got)
	}
	b.typeText("Opera")
	b.eval(`render()`) // something else draws: the name being written stays
	b.typeText("tions")
	b.press("Enter", "Enter", 13, 0)
	if !b.waitFor(`!document.querySelector('[data-ren]') && [...document.querySelectorAll('.nav2 .lbl')].some(n => n.textContent === 'Operations')`) {
		t.Fatalf("Enter keeps the name: %s", b.eval(`document.querySelector('.nav2').innerText.replace(/\s+/g, ' ')`))
	}
	b.rightClickOn(probe(b, `.nav2 [data-act="coll"]`, "Operations"))
	if got := b.eval(`[...document.querySelectorAll('.ctxpop button')].find(n => n.textContent.trim() === 'Delete')?.disabled`); got != "false" {
		t.Errorf("a collection just made is one's own: Delete is offered at once, got %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
	// Two more without a name: the free numbers in turn, and Escape keeps the name it got.
	for _, want := range []string{"New collection (1)", "New collection (2)"} {
		newIn()
		if !b.waitFor(`document.activeElement?.value === ` + strconv.Quote(want)) {
			t.Fatalf("want %q: %q", want, b.eval(`document.activeElement?.value`))
		}
		b.typeText("Not kept")
		b.press("Escape", "Escape", 27, 0)
		if !b.waitFor(`!document.querySelector('[data-ren]')`) {
			t.Fatal("Escape ends writing the name")
		}
	}
	b.shot("new-collection")
	time.Sleep(500 * time.Millisecond)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	if got := b.eval(`[...document.querySelectorAll('.nav2 .lbl')].map(n => n.textContent).filter(n => n === 'Operations' || n.startsWith('New collection')).sort().join(', ')`); got != "New collection (1), New collection (2), Operations" {
		t.Errorf("the database keeps them: %q", got)
	}
}

// New is one menu in groups, and typing filters it: arrows move the choice, Enter takes it.
func TestNewMenuFilters(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	visible := `[...document.querySelectorAll('.cmdpop [role="menuitem"]')].filter(n => n.offsetParent).map(n => n.textContent.trim()).join(', ')`
	b.clickOn(`.nav2 [data-act="addmenu"][data-id="n2"]`, 1)
	if got := b.eval(`[...document.querySelectorAll('.cmdpop [role="heading"]')].map(n => n.textContent).join(', ')`); got != "Make, Set apart, Bring in" {
		t.Errorf("the groups of New in nav2: %q", got)
	}
	b.shot("new-menu-nav2")
	if got := b.eval(visible); got != "Collection, Line, Space, Import, Link" {
		t.Errorf("what New in nav2 offers: %q", got)
	}
	if got := b.eval(`(() => { const s = getComputedStyle(document.activeElement); return document.activeElement.dataset.nfilter !== undefined && s.outlineStyle + ' ' + s.borderTopStyle; })()`); got != "none none" {
		t.Errorf("the filter has no box of its own: %q", got)
	}
	b.typeText("spa")
	if got := b.eval(visible); got != "Space" {
		t.Errorf("typing filters: %q", got)
	}
	before := b.eval(`document.querySelectorAll('#clist .deco[data-deco="space"]').length`)
	b.press("Enter", "Enter", 13, 0)
	if !b.waitFor(`!document.querySelector('.cmdpop') && document.querySelectorAll('#clist .deco[data-deco="space"]').length === ` + before + ` + 1 && document.querySelector('#clist').lastElementChild.dataset.deco === 'space'`) {
		t.Errorf("Enter adds a space at the end of nav2: %q", b.eval(`[...document.querySelectorAll('#clist > *')].map(n => n.dataset.deco || n.dataset.favid).join(' ')`))
	}
	b.clickOn(`.nav2 [data-act="addmenu"][data-id="n2"]`, 1)
	b.typeText("zzz")
	if got := b.eval(visible); got != "" {
		t.Errorf("nothing matches: %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
	if !b.waitFor(`!document.querySelector('.cmdpop')`) {
		t.Error("Escape closes New")
	}

	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	b.clickOn(`.cheadrow[data-cdrop="`+team+`"] .newplus`, 1)
	if got := b.eval(`[...document.querySelectorAll('.cmdpop [role="heading"]')].map(n => n.textContent).join(', ')`); got != "Tables, Databases, Records, Boards and lists, Media, Organize, Set apart, Bring in" {
		t.Errorf("New in a collection, in blocks: %q", got)
	}
	b.shot("new-menu-blocks")
	if got := b.eval(`(() => { const m = document.querySelector('.cmdpop [role="menu"]'); return [getComputedStyle(m).scrollbarWidth, m.scrollHeight > m.clientHeight].join(' '); })()`); got != "thin true" {
		t.Errorf("the long list of New shows its scrollbar: %q", got)
	}
	if got := b.eval(`[...document.querySelectorAll('.cmdpop [role="menuitem"]')].slice(0, 5).map(n => n.textContent.trim()).join(', ')`); got != "Page, Table, Excel file, Database, Database view" {
		t.Errorf("what is needed most comes first: %q", got)
	}
	b.typeText("lin")
	b.shot("new-menu-filtered")
	if got := b.eval(visible); got != "Line, Link" {
		t.Errorf("New in a collection, filtered: %q", got)
	}
	b.press("ArrowDown", "ArrowDown", 40, 0)
	if got := b.eval(`document.querySelector('.cmdpop .active')?.textContent.trim()`); got != "Line" {
		t.Errorf("the arrow skips what is not built: %q", got)
	}
	b.press("Enter", "Enter", 13, 0)
	if !b.waitFor(`document.querySelectorAll('.nav3 .row.deco').length === 1`) {
		t.Fatal("Enter adds a line to the collection")
	}
	if got := b.eval(`[...document.querySelectorAll('.nav3 .cheadrow[data-cdrop="` + team + `"] + ul > li > .row')].map(n => n.dataset.deco || n.textContent.trim()).join(' ')`); got != "A B C line" {
		t.Errorf("the line is at the end: %q", got)
	}
}

// typeText types into the element that has the focus, as a keyboard would.
func (b *browser) typeText(text string) {
	b.t.Helper()
	b.call("Input.insertText", map[string]any{"text": text}, nil)
}

func TestEditAPageBlockByBlock(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.eval(`window.made = null; fetch('/api/collections', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: '{"title":"Team"}'}).then(r => r.json()).then(j => { window.made = j.id; })`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the collection for the test was not made")
	}
	team := b.eval(`window.made`)
	b.open(base + "/o/" + team)
	b.ready("Team")

	b.clickOn(`.cheadrow[data-cdrop="`+team+`"] .newplus`, 1)
	b.clickOn(`.cmdpop [data-act="add"][data-id="page"]`, 1)
	if !b.waitFor(`document.activeElement?.dataset?.blk === 'b1'`) {
		t.Fatalf("a new page opens with its first block ready to write: %q", b.eval(`document.activeElement?.outerHTML.slice(0, 160)`))
	}
	page := b.eval(`location.pathname.slice(3)`)
	b.typeText("## Switches\n\nTwo in the rack.")
	b.press("Enter", "Enter", 13, 2) // Ctrl+Enter saves
	if !b.waitFor(`document.querySelector('.bbar em')?.textContent === 'Saved · #b1'`) {
		t.Fatalf("the block was not saved: %q", b.eval(`document.querySelector('.bbar')?.textContent`))
	}
	b.clickOn(probe(b, `.bbar .chip`, "Block below"), 1)
	if !b.waitFor(`document.activeElement?.dataset?.blk === 'b2'`) {
		t.Fatalf("the block below has the focus: %q", b.eval(`document.activeElement?.outerHTML.slice(0, 160)`))
	}
	b.typeText("| Port | VLAN |\n|---|---|\n| 1 | 10 |")
	b.shot("edit-blocks")
	b.clickOn(`[data-act="edit"]`, 1) // leaving editing saves the block being written
	if !b.waitFor(`!!document.querySelector('article.doc h2') && !!document.querySelector('article.doc table') && !document.querySelector('textarea[data-blk]')`) {
		t.Fatalf("the view shows the blocks: %s", b.eval(`document.querySelector('article.doc')?.innerHTML.slice(0, 300)`))
	}
	b.shot("view-blocks")
	if got := b.eval(`document.querySelector('article.doc .blk span')?.textContent`); got != "#b2" {
		t.Errorf("a table has the ID of its block, to embed it by: %q", got)
	}

	b.open(base + "/o/" + page)
	b.ready("New page")
	if got := b.eval(`document.querySelector('article.doc h2')?.textContent + ' | ' + document.querySelectorAll('article.doc td').length`); got != "Switches | 2" {
		t.Errorf("both blocks are kept: %q", got)
	}
	b.clickOn(`[data-act="gear"]`, 1)
	// Each block was made empty and then written: four versions, newest first.
	if !b.waitFor(`[...document.querySelectorAll('.fdrawer li')].map(li => (li.textContent.match(/#b\d+ \S+/) || [''])[0]).join(', ') === '#b2 |, #b2 (empty), #b1 ##, #b1 (empty)'`) {
		t.Errorf("the versions list both blocks: %s", b.eval(`document.querySelector('.fdrawer')?.innerText.replace(/\s+/g, ' ')`))
	}
	b.clickOn(`[data-act="gear"]`, 1)

	b.clickOn(`[data-act="edit"]`, 1)
	b.clickOn(`.bedit:first-child .bbar [data-act="blkdel"]`, 1)
	if got := b.eval(`document.querySelectorAll('textarea[data-blk]').length + ' ' + document.querySelector('.bedit:first-child [data-act="blkdel"]').textContent`); got != "2 Delete? Yes" {
		t.Fatalf("a block with text asks first: %q", got)
	}
	b.clickOn(`.bedit:first-child .bbar [data-act="blkdel"]`, 1)
	if !b.waitFor(`document.querySelectorAll('textarea[data-blk]').length === 1 && document.querySelector('textarea[data-blk]').dataset.blk === 'b2'`) {
		t.Errorf("the block is deleted: %s", b.eval(`[...document.querySelectorAll('textarea[data-blk]')].map(x => x.dataset.blk).join(' ')`))
	}
}

func TestBlockTextStaysText(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.eval(`window.made = null; (async () => {
		const json = {'Content-Type': 'application/json'};
		const team = await (await fetch('/api/collections', {method: 'POST', headers: json, body: '{"title":"Team"}'})).json();
		const page = await (await fetch('/api/pages', {method: 'POST', headers: json, body: JSON.stringify({title: 'Tricky', parent: team.id})})).json();
		await fetch('/api/pages/' + page.id + '/blocks', {method: 'POST', headers: json, body: JSON.stringify({md: '</textarea><b id="escaped">x</b> & <i>'})});
		window.made = page.id;
	})()`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the page for the test was not made")
	}
	b.open(base + "/o/" + b.eval(`window.made`))
	b.ready("Tricky")
	b.clickOn(`[data-act="edit"]`, 1)
	if !b.waitFor(`!!document.querySelector('textarea[data-blk]')`) {
		t.Fatal("no field to edit")
	}
	if got := b.eval(`(document.getElementById('escaped') ? 'markup ' : '') + document.querySelector('textarea[data-blk]').value`); got != `</textarea><b id="escaped">x</b> & <i>` {
		t.Errorf("the Markdown of a block is its text in the field, never markup: %q", got)
	}
}
