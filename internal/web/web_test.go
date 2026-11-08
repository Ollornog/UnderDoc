// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/folder"
	"github.com/Ollornog/UnderDoc/internal/library"
	"github.com/Ollornog/UnderDoc/internal/markdown"
)

type site struct {
	t     *testing.T
	dir   string
	store *library.Store
	http  http.Handler
}

func newSite(t *testing.T, options Options, files map[string]string) *site {
	t.Helper()
	s := &site{t: t, dir: t.TempDir()}
	for name, content := range files {
		s.write(name, content)
	}
	log := slog.New(slog.DiscardHandler)
	s.store = library.NewStore(folder.New(s.dir), log)
	s.refresh()
	handler, err := New(s.store, log, options)
	if err != nil {
		t.Fatal(err)
	}
	s.http = handler
	return s
}

func (s *site) write(name, content string) {
	s.t.Helper()
	full := filepath.Join(s.dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

func (s *site) refresh() {
	s.t.Helper()
	if err := s.store.Refresh(); err != nil {
		s.t.Fatal(err)
	}
}

// get asks as Anna, signed in through the gate.
func (s *site) get(target string) *httptest.ResponseRecorder {
	s.t.Helper()
	return s.getWith(target, gateHeaders)
}

var gateHeaders = map[string]string{"Remote-User": "anna", "Remote-Id": "u-7", "Remote-Name": "Anna Berg", "Remote-Email": "anna@example.com", "Remote-Groups": "ops,underdoc-admins"}

func (s *site) getWith(target string, headers map[string]string) *httptest.ResponseRecorder {
	s.t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	s.http.ServeHTTP(recorder, request)
	return recorder
}

func (s *site) body(target string) string {
	s.t.Helper()
	recorder := s.get(target)
	if recorder.Code != http.StatusOK {
		s.t.Fatalf("GET %s = %d", target, recorder.Code)
	}
	return recorder.Body.String()
}

var sample = map[string]string{
	"README.md":          "---\nid: start\ntitle: Start page\n---\n# Welcome\n\nGo to [[plan]] or [[gone]].\n\n<script>alert(1)</script>\n",
	"ops/net/plan.md":    "---\nid: plan\ntitle: Network plan\n---\n# Network plan\n\n![Map](map.png)\n\nRouter in the tower. Back to [[start]].\n",
	"ops/net/map.png":    "\x89PNG\r\n\x1a\n",
	"ops/net/graph.svg":  `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`,
	"ops/no-id.md":       "No heading and no id.\n",
	"secret.txt":         "must not be served",
	".hidden/page.md":    "must not be served",
	".hidden/pic.png":    "must not be served",
	"ops/net/config.ini": "must not be served",
}

func TestPagesRender(t *testing.T) {
	s := newSite(t, Options{}, sample)
	cases := []struct{ target, want string }{
		{"/o/start", "Welcome"},
		{"/o/start", `href="/o/plan"`},
		{"/o/plan", `src="/files/ops/net/map.png"`},
		{"/o/plan", `aria-current="page"`},
		{"/o/plan", "Linked from"},
		{"/o/~ops/no-id.md", "its address changes when it moves"},
		{"/o/~ops/no-id.md", "no-id</h1>"},
		{"/f/", "ops"},
		{"/f/ops", "no-id"},
		{"/f/ops/net", "Network plan"},
		{"/search?q=router", "Network plan"},
		{"/search?q=router", "<mark"},
		{"/search?q=xyzzy", "Nothing found"},
		{"/search", "Type a word"},
		{"/problems", "[[gone]]: no such page"},
	}
	for _, c := range cases {
		if got := s.body(c.target); !strings.Contains(got, c.want) {
			t.Errorf("GET %s: %q missing", c.target, c.want)
		}
	}
}

func TestLinkToFolderOpensTheFolder(t *testing.T) {
	s := newSite(t, Options{}, map[string]string{
		"start.md":     "---\nid: start\n---\n[the folder](ops/)\n",
		"ops/inner.md": "---\nid: inner\ntitle: Inner page\n---\n",
	})
	if got := s.body("/o/start"); !strings.Contains(got, `href="/f/ops"`) {
		t.Errorf("no link to the folder in %s", got)
	}
	if got := s.body("/f/ops"); !strings.Contains(got, "Inner page") {
		t.Errorf("the folder does not list its page: %s", got)
	}
	if got := s.body("/problems"); strings.Contains(got, "ops/") {
		t.Errorf("the link is listed as broken: %s", got)
	}
}

func TestContentsNeedTwoEntries(t *testing.T) {
	s := newSite(t, Options{}, map[string]string{
		"one.md": "---\nid: one\n---\n## Only\n",
		"two.md": "---\nid: two\n---\n## First\n\n## Second <b>x</b>\n",
	})
	if got := s.body("/o/one"); strings.Contains(got, `class="toc`) {
		t.Error("a single heading needs no table of contents")
	}
	got := s.body("/o/two")
	if !strings.Contains(got, `href="#h-first" data-level="2"`) || !strings.Contains(got, `id="h-first"`) {
		t.Errorf("table of contents: %s", got)
	}
}

func TestRecentOnlyAtTheTop(t *testing.T) {
	s := newSite(t, Options{}, sample)
	if got := s.body("/f/"); !strings.Contains(got, "Recently changed") {
		t.Error("the root folder should list recent changes")
	}
	if got := s.body("/f/ops"); strings.Contains(got, "Recently changed") {
		t.Error("a subfolder should not")
	}
}

func TestOwnHeadingIsNotRepeated(t *testing.T) {
	s := newSite(t, Options{}, sample)
	if got := strings.Count(s.body("/o/plan"), "Network plan</h1>"); got != 1 {
		t.Errorf("heading appears %d times", got)
	}
}

func TestHomeRedirect(t *testing.T) {
	s := newSite(t, Options{}, sample)
	if got := s.get("/"); got.Code != http.StatusFound || got.Header().Get("Location") != "/o/start" {
		t.Errorf("%d %s", got.Code, got.Header().Get("Location"))
	}
	empty := newSite(t, Options{}, nil)
	if got := empty.get("/"); got.Header().Get("Location") != "/f/" {
		t.Errorf("empty: %d %s", got.Code, got.Header().Get("Location"))
	}
	if got := empty.body("/f/"); !strings.Contains(got, "No pages yet") {
		t.Errorf("empty library: %s", got)
	}
}

func TestSecurityHeadersOnEveryAnswer(t *testing.T) {
	s := newSite(t, Options{}, sample)
	for _, target := range []string{"/o/start", "/f/", "/search?q=a", "/problems", "/gone", "/o/gone", "/files/gone.png", "/assets/app/shell.js"} {
		headers := s.get(target).Header()
		if got := headers.Get("Content-Security-Policy"); got != CSP {
			t.Errorf("%s: CSP %q", target, got)
		}
		if headers.Get("X-Content-Type-Options") != "nosniff" || headers.Get("X-Frame-Options") != "DENY" {
			t.Errorf("%s: headers %v", target, headers)
		}
		if headers.Get("Strict-Transport-Security") != "" {
			t.Errorf("%s: HSTS belongs to the proxy", target)
		}
	}
	for _, unsafe := range []string{"unsafe-inline", "unsafe-eval", "*", "http:", "https:"} {
		if strings.Contains(CSP, unsafe) {
			t.Errorf("the CSP allows %s", unsafe)
		}
	}
	if got := s.get("/o/start").Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("content type %q", got)
	}
}

func TestReadOnlyMethods(t *testing.T) {
	s := newSite(t, Options{}, sample)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		for _, target := range []string{"/", "/o/start", "/files/ops/net/map.png", "/search"} {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(method, target, strings.NewReader("x"))
			request.Header.Set("Remote-User", "anna")
			s.http.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s = %d", method, target, recorder.Code)
			}
		}
	}
}

func TestOnlyKnownImagesAreServed(t *testing.T) {
	s := newSite(t, Options{}, sample)
	image := s.get("/files/ops/net/map.png")
	if image.Code != http.StatusOK || image.Header().Get("Content-Type") != "image/png" {
		t.Errorf("image: %d %s", image.Code, image.Header().Get("Content-Type"))
	}
	for _, target := range []string{
		"/files/secret.txt", "/files/README.md", "/files/ops/net/config.ini", "/files/.hidden/page.md", "/files/.hidden/pic.png",
		"/files/../secret.txt", "/files/ops/../../secret.txt", "/files/%2e%2e/secret.txt", "/files/ops/net/gone.png", "/files/",
	} {
		recorder := s.get(target)
		if strings.Contains(recorder.Body.String(), "must not be served") {
			t.Errorf("%s leaked its content", target)
		}
		if recorder.Code == http.StatusOK {
			t.Errorf("%s = 200", target)
		}
	}
}

func TestSVGRunsSandboxed(t *testing.T) {
	s := newSite(t, Options{}, sample)
	answer := s.get("/files/ops/net/graph.svg")
	if answer.Code != http.StatusOK {
		t.Fatal(answer.Code)
	}
	csp := answer.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "sandbox") || !strings.Contains(csp, "default-src 'none'") || strings.Contains(csp, "script-src") {
		t.Errorf("CSP of an SVG: %q", csp)
	}
}

func TestUserTextIsEscaped(t *testing.T) {
	s := newSite(t, Options{}, map[string]string{
		"a.md":                 "---\ntitle: \"<script>alert(1)</script>\"\n---\ntext\n",
		"<b>folder/<i>page.md": "text [[gone<script>]]\n",
		"raw.md":               "<script>alert(2)</script>\n\n<img src=x onerror=alert(3)>\n",
	})
	targets := []string{"/o/~a.md", "/f/", "/search?q=" + `%3Cscript%3Ealert(1)%3C/script%3E`, "/search?q=text", "/problems", "/o/~raw.md", "/o/~%3Cb%3Efolder/%3Ci%3Epage.md"}
	for _, target := range targets {
		got := s.body(target)
		for _, forbidden := range []string{"<script>alert", "<b>folder", "<i>page", "<img src=x"} {
			if strings.Contains(got, forbidden) {
				t.Errorf("%s: %q not escaped", target, forbidden)
			}
		}
	}
}

func TestChangesOnDiskAppear(t *testing.T) {
	s := newSite(t, Options{}, map[string]string{"a.md": "---\nid: a\n---\nold text\n"})
	if got := s.body("/o/a"); !strings.Contains(got, "old text") {
		t.Fatal(got)
	}
	s.write("a.md", "---\nid: a\n---\nnew and longer text\n")
	s.write("b.md", "---\nid: b\n---\n[[a]]\n")
	s.refresh()
	if got := s.body("/o/a"); !strings.Contains(got, "new and longer text") || strings.Contains(got, "old text") {
		t.Errorf("after the edit: %s", got)
	}
	if got := s.body("/o/a"); !strings.Contains(got, "Linked from") {
		t.Error("the new backlink is missing")
	}
	if got := s.body("/search?q=longer"); !strings.Contains(got, `/o/a"`) {
		t.Error("search did not see the change")
	}
	if got := s.get("/o/a").Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("a cached page would hide the change: %q", got)
	}
}

func TestMovingAFileKeepsItsAddress(t *testing.T) {
	s := newSite(t, Options{}, map[string]string{
		"old/a.md": "---\nid: a\n---\n# A\n",
		"b.md":     "---\nid: b\n---\n[[a]]\n",
	})
	if err := os.MkdirAll(filepath.Join(s.dir, "new/deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(s.dir, "old/a.md"), filepath.Join(s.dir, "new/deep/a.md")); err != nil {
		t.Fatal(err)
	}
	s.refresh()
	got := s.body("/o/a")
	if !strings.Contains(got, "new/deep/a.md") || !strings.Contains(got, "Linked from") {
		t.Errorf("after the move: %s", got)
	}
	if !strings.Contains(s.body("/o/b"), `href="/o/a"`) {
		t.Error("the link broke")
	}
	if s.get("/f/old").Code != http.StatusNotFound {
		t.Error("the old folder should be gone")
	}
}

func TestNotFoundIsAPage(t *testing.T) {
	s := newSite(t, Options{}, sample)
	for _, target := range []string{"/gone", "/o/gone", "/f/gone"} {
		recorder := s.get(target)
		if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "Page not found") {
			t.Errorf("%s = %d", target, recorder.Code)
		}
	}
}

func TestGalleryIsOffByDefault(t *testing.T) {
	if got := newSite(t, Options{}, sample).get("/dev/ui").Code; got != http.StatusNotFound {
		t.Errorf("/dev/ui without the switch = %d", got)
	}
	if got := newSite(t, Options{DevUI: true}, sample).body("/dev/ui"); !strings.Contains(got, "Components") {
		t.Error("the gallery is empty")
	}
}

// The design system only compiles classes it uses itself, and a missing class does nothing without any error. So every class
// in the templates and in rendered pages has to exist in the vendored stylesheet.
func TestEveryClassExistsInTheStylesheet(t *testing.T) {
	css, err := assetFiles.ReadFile("assets/c22/css/c22-vega.css")
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, match := range regexp.MustCompile(`\.((?:\\.|[\w-])+)`).FindAllStringSubmatch(string(css), -1) {
		known[strings.ReplaceAll(match[1], `\`, "")] = true
	}
	if len(known) < 500 {
		t.Fatalf("only %d classes found in the stylesheet, the check itself is broken", len(known))
	}
	for _, control := range []string{"made-up-class", "py-13"} {
		if known[control] {
			t.Fatalf("control class %q found, the check itself is broken", control)
		}
	}
	// The shell is a special path of this application until C22 takes it over (C22 M-2). Its classes count as known.
	shell, err := assetFiles.ReadFile("assets/app/shell.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range regexp.MustCompile(`\.(ud-[\w-]+)`).FindAllStringSubmatch(string(shell), -1) {
		known[match[1]] = true
	}
	used := map[string]string{}
	for _, class := range markdown.Classes() {
		used[class] = "package markdown"
	}
	attribute := regexp.MustCompile(`class="([^"{]*)"`)
	files, err := fs.Glob(templateFiles, "templates/*.html")
	if err != nil || len(files) == 0 {
		t.Fatal(files, err)
	}
	for _, file := range files {
		content, err := templateFiles.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range attribute.FindAllStringSubmatch(string(content), -1) {
			for _, class := range strings.Fields(match[1]) {
				used[class] = file
			}
		}
	}
	if len(used) < 40 {
		t.Fatalf("only %d classes in use, the check itself is broken", len(used))
	}
	for class, where := range used {
		if !known[class] {
			t.Errorf("%s: class %q is not in the stylesheet", where, class)
		}
	}
}

func TestPagesHaveNoInlineCode(t *testing.T) {
	s := newSite(t, Options{DevUI: true}, sample)
	inline := regexp.MustCompile(`(?i)\sstyle=|\son[a-z]+=|<style|<script(?:\s+defer)?>`)
	for _, target := range []string{"/o/start", "/o/plan", "/f/", "/search?q=router", "/problems", "/dev/ui"} {
		if found := inline.FindString(s.body(target)); found != "" {
			t.Errorf("%s: %q would need a weaker CSP", target, found)
		}
	}
}

func TestNothingFromThirdParties(t *testing.T) {
	s := newSite(t, Options{DevUI: true}, sample)
	foreign := regexp.MustCompile(`(?i)(?:src|<link[^>]+href)="(?:https?:)?//`)
	for _, target := range []string{"/o/start", "/o/plan", "/f/", "/dev/ui"} {
		if found := foreign.FindString(s.body(target)); found != "" {
			t.Errorf("%s: %q", target, found)
		}
	}
	css, err := assetFiles.ReadFile("assets/c22/css/c22-vega.css")
	if err != nil {
		t.Fatal(err)
	}
	if found := regexp.MustCompile(`url\(["']?(?:https?:)?//[^)]*`).Find(css); found != nil {
		t.Errorf("the stylesheet loads %s", found)
	}
}

func TestAssetsArrive(t *testing.T) {
	s := newSite(t, Options{}, sample)
	for target, contentType := range map[string]string{
		"/assets/c22/css/c22-vega.css":       "text/css",
		"/assets/c22/fonts/inter.woff2":      "font/woff2",
		"/assets/c22/js/basecoat.min.js":     "javascript",
		"/assets/c22/js/sidebar.min.js":      "javascript",
		"/assets/app/shell.js":               "javascript",
		"/assets/app/shell.css":              "css",
		"/assets/app/theme.js":               "javascript",
		"/assets/logo.svg":                   "image/svg+xml",
		"/favicon.ico":                       "image/",
		"/assets/c22/licenses/Inter-OFL.txt": "text/plain",
	} {
		recorder := s.get(target)
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Header().Get("Content-Type"), contentType) {
			t.Errorf("%s: %d %s", target, recorder.Code, recorder.Header().Get("Content-Type"))
		}
	}
}

func TestHealth(t *testing.T) {
	ready := false
	handler := Health(func() bool { return ready })
	ask := func() int {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, healthPath, nil))
		return recorder.Code
	}
	if ask() != http.StatusServiceUnavailable {
		t.Error("not ready should be 503")
	}
	ready = true
	if ask() != http.StatusOK {
		t.Error("ready should be 200")
	}
}

func TestProbe(t *testing.T) {
	ready := true
	server := httptest.NewServer(Health(func() bool { return ready }))
	defer server.Close()
	addr := server.Listener.Addr().String()
	if err := Probe(context.Background(), addr); err != nil {
		t.Errorf("healthy: %v", err)
	}
	ready = false
	if err := Probe(context.Background(), addr); err == nil {
		t.Error("an unhealthy server must fail the probe")
	}
	_, port, _ := net.SplitHostPort(addr)
	ready = true
	if err := Probe(context.Background(), ":"+port); err != nil {
		t.Errorf("all interfaces should be asked over loopback: %v", err)
	}
}

func TestAccessLogLeavesOutTheQuery(t *testing.T) {
	var lines strings.Builder
	log := slog.New(slog.NewJSONHandler(&lines, nil))
	store := library.NewStore(folder.New(t.TempDir()), log)
	if err := store.Refresh(); err != nil {
		t.Fatal(err)
	}
	handler, err := New(store, log, Options{})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/search?q=private-word", nil)
	request.RemoteAddr = "192.0.2.7:1234"
	handler.ServeHTTP(httptest.NewRecorder(), request)
	got := lines.String()
	if strings.Contains(got, "private-word") || strings.Contains(got, "192.0.2.7") {
		t.Errorf("the log says too much: %s", got)
	}
	if !strings.Contains(got, `"path":"/search"`) {
		t.Errorf("the request is missing: %s", got)
	}
}

func TestNothingAnswersWithoutTheGate(t *testing.T) {
	s := newSite(t, Options{}, sample)
	for _, target := range []string{"/", "/o/start", "/f/", "/search?q=router", "/profile", "/files/ops/net/map.png", "/gone"} {
		recorder := s.getWith(target, nil)
		if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "Not signed in") || strings.Contains(recorder.Body.String(), "Welcome") {
			t.Errorf("%s = %d", target, recorder.Code)
		}
	}
	if got := s.getWith("/assets/app/shell.css", nil).Code; got != http.StatusOK {
		t.Errorf("the page that says so needs its styles: %d", got)
	}
	dev := newSite(t, Options{DevUser: &account.User{Login: "dev", Name: "Dev"}}, sample)
	if got := dev.getWith("/o/start", nil); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "Profile: Dev") {
		t.Errorf("the development user stands in for the gate: %d", got.Code)
	}
}

func TestOldAddressesMove(t *testing.T) {
	s := newSite(t, Options{}, sample)
	if got := s.get("/d/plan"); got.Code != http.StatusMovedPermanently || got.Header().Get("Location") != "/o/plan" {
		t.Errorf("%d %s", got.Code, got.Header().Get("Location"))
	}
}

// The profile view: sections in nav2, the chosen one in nav3, the administration set apart for administrators (ADR-16).
func TestProfile(t *testing.T) {
	s := newSite(t, Options{AdminGroup: "underdoc-admins"}, sample)
	body := s.body("/profile")
	for _, want := range []string{
		`<nav class="ud-nav2" aria-label="Profile">`, `href="/profile?s=appearance"`, "Administration",
		`title="Modules: not built yet`, `title="Language: not built yet`,
		"Anna Berg", "anna@example.com", ">ops<", `href="/.tinysesam/logout"`, `>AB<`,
		`<a class="ud-navitem" href="/profile?s=account" title="Account" data-ud-columns aria-current="true">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("%q missing", want)
		}
	}
	if strings.Contains(body, `class="ud-tree"`) {
		t.Error("the profile replaces the collections in nav2 and nav3")
	}
	if got := s.body("/profile?s=appearance"); !strings.Contains(got, `data-ud-theme="dark"`) || strings.Contains(got, "/.tinysesam/logout") {
		t.Error("appearance shows the theme, not the account")
	}
	if !strings.Contains(s.body("/profile?s=navigation"), "data-ud-reset-columns") {
		t.Error("navigation shows the columns")
	}
	if !strings.Contains(s.body("/profile?s=modules"), "Sign out") {
		t.Error("a section that is not built falls back to the account")
	}
	other := s.getWith("/profile?s=modules", map[string]string{"Remote-User": "bob"}).Body.String()
	if strings.Contains(other, "Administration") || !strings.Contains(other, "bob") {
		t.Error("only administrators see the administration")
	}
}

func TestHeadAndCollections(t *testing.T) {
	files := map[string]string{
		"README.md":       "# Start\n",
		"ops/net/plan.md": "---\nid: plan\ntitle: Network plan\ndescription: Router and switches.\ntags: [network, main-site]\n---\n# Network plan\n\nText.\n",
		"ops/hand.md":     "---\ntags: network\n---\n# Handbook\n",
		"projects/x.md":   "# X\n",
	}
	s := newSite(t, Options{}, files)
	body := s.body("/o/plan")
	for _, want := range []string{
		`<h1 class="ud-title">Network plan</h1>`, `<p class="ud-desc">Router and switches.</p>`,
		`href="/search?tag=network"`, ">#main-site<",
		`<a class="ud-navitem" href="/f/ops" title="ops" data-ud-columns aria-current="true">`, `href="/f/projects"`, `title="Home"`,
		`<a href="/f/ops" class="ud-chead" title="Open ops">ops</a>`, `aria-current="page"><svg`,
		`<code>/o/plan</code>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("%q missing", want)
		}
	}
	if strings.Count(body, "Network plan</h1>") != 1 {
		t.Error("the title stands once, in the head")
	}
	if strings.Contains(body, `href="/o/~projects/x.md"`) {
		t.Error("nav3 shows only the open collection")
	}
	tagged := s.body("/search?tag=%23Network")
	if !strings.Contains(tagged, `<h1 class="ud-title">#network</h1>`) || !strings.Contains(tagged, "Handbook") || !strings.Contains(tagged, "Network plan") {
		t.Error("a tag lists what carries it")
	}
}

// Built later means greyed out (ADR-17): in sight, but nothing to click.
func TestLaterIsGreyedOut(t *testing.T) {
	body := newSite(t, Options{}, sample).body("/o/plan")
	for _, label := range []string{"Files", "Resources", "News", "Mail", "To-do", "Calendar", "Time", "Vacation", "Notes"} {
		if !regexp.MustCompile(`<span class="ud-navitem" aria-disabled="true" title="` + label + `: not built yet`).MatchString(body) {
			t.Errorf("%s is not greyed out", label)
		}
	}
	if strings.Count(body, `aria-disabled="true" title="New: `)+strings.Count(body, `aria-disabled="true" title="New collection: `) != 3 {
		t.Error("New in the top row, in nav2 and in nav3 waits for the database")
	}
}

// The shell is a special path until C22 takes it over, so it may only use the tokens of C22, never values of its own.
func TestShellUsesOnlyTokens(t *testing.T) {
	css, err := assetFiles.ReadFile("assets/app/shell.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, pattern := range []string{`#[0-9a-fA-F]{3,8}\b`, `\b(rgb|rgba|hsl|hsla|oklch|oklab|color-mix)\(`, `font-size:\s*[0-9]`, `font-weight:\s*[0-9]`, `border-radius:\s*[0-9.]+(px|rem)`, `[:\s](white|black)\s*[;}]`} {
		if found := regexp.MustCompile(pattern).FindString(string(css)); found != "" {
			t.Errorf("shell.css: %q is a value of its own, use a C22 token", found)
		}
	}
	if !strings.Contains(string(css), "var(--sidebar)") {
		t.Fatal("the check itself is broken")
	}
}
