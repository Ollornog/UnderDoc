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

func page(t *testing.T, s *site, docID string) apiDocument {
	t.Helper()
	return decode[apiDocument](t, s.get("/api/doc/"+docID))
}

func node(t *testing.T, nodes []apiNode, nodeID string) apiNode {
	t.Helper()
	for _, n := range nodes {
		if n.ID == nodeID {
			return n
		}
		if found := findNode(n.Kids, nodeID); found != nil {
			return *found
		}
	}
	t.Fatalf("%s is not in the tree", nodeID)
	return apiNode{}
}

func findNode(nodes []apiNode, nodeID string) *apiNode {
	for i := range nodes {
		if nodes[i].ID == nodeID {
			return &nodes[i]
		}
		if found := findNode(nodes[i].Kids, nodeID); found != nil {
			return found
		}
	}
	return nil
}

func TestPagesArrive(t *testing.T) {
	s := newSite(t, Options{}, sample)
	start := page(t, s, "start")
	if !strings.Contains(start.Intro, "Welcome") || !strings.Contains(start.Intro, `href="/o/plan"`) || strings.Contains(start.Intro, "<script") {
		t.Errorf("start: %s", start.Intro)
	}
	plan := page(t, s, "plan")
	if !strings.Contains(plan.Intro, `src="/files/ops/net/map.png"`) || strings.Contains(plan.Intro, "<h1") {
		t.Errorf("plan: %s", plan.Intro)
	}
	if plan.Place != "ops › net" || plan.Path != "ops/net/plan.md" || !plan.Stable || strings.Join(plan.UsedIn, ",") != "Start page" {
		t.Errorf("plan: %+v", plan)
	}
	if noID := page(t, s, "~ops/no-id.md"); noID.Title != "no-id" || noID.Stable {
		t.Errorf("a page without an id: %+v", noID)
	}
	state := decode[apiState](t, s.get("/api/state"))
	if state.Problems != 1 {
		t.Errorf("[[gone]] is the one problem, got %d", state.Problems)
	}
	if home := node(t, state.Collections, "f:"); home.Title != "Home" || len(home.Kids) != 1 || home.Kids[0].ID != "start" {
		t.Errorf("home: %+v", home)
	}
	if net := node(t, state.Collections, "f:ops/net"); len(net.Kids) != 1 || net.Kids[0].Title != "Network plan" || net.Kind != "folder" {
		t.Errorf("ops/net: %+v", net)
	}
	if got := decode[[]apiHit](t, s.get("/api/search?q=xyzzy")); len(got) != 0 {
		t.Errorf("nothing should be found: %+v", got)
	}
}

func TestLinkToFolderOpensTheFolder(t *testing.T) {
	s := newSite(t, Options{}, map[string]string{
		"start.md":     "---\nid: start\n---\n[the folder](ops/)\n",
		"ops/inner.md": "---\nid: inner\ntitle: Inner page\n---\n",
	})
	if got := page(t, s, "start"); !strings.Contains(got.Intro, `href="/f/ops"`) {
		t.Errorf("no link to the folder in %s", got.Intro)
	}
	state := decode[apiState](t, s.get("/api/state"))
	if ops := node(t, state.Collections, "f:ops"); len(ops.Kids) != 1 || ops.Kids[0].Title != "Inner page" {
		t.Errorf("the folder does not list its page: %+v", ops)
	}
	if state.Problems != 0 {
		t.Error("the link is counted as broken")
	}
}

func TestSectionsOfAPage(t *testing.T) {
	s := newSite(t, Options{}, map[string]string{
		"two.md": "---\nid: two\n---\nBefore.\n\n## First\n\nOne.\n\n## Second <b>x</b>\n\nTwo.\n",
	})
	two := page(t, s, "two")
	if !strings.Contains(two.Intro, "Before.") || len(two.Sections) != 2 {
		t.Fatalf("%+v", two)
	}
	if two.Sections[0][0] != "First" || !strings.Contains(two.Sections[0][1], "One.") || two.Sections[1][0] != "Second x" {
		t.Errorf("sections: %q", two.Sections)
	}
}

func TestEmptyLibrary(t *testing.T) {
	s := newSite(t, Options{}, nil)
	if got := s.get("/"); got.Code != http.StatusOK {
		t.Errorf("/ = %d", got.Code)
	}
	if got := decode[apiState](t, s.get("/api/state")); len(got.Collections) != 0 {
		t.Errorf("collections of an empty library: %+v", got.Collections)
	}
}

func TestSecurityHeadersOnEveryAnswer(t *testing.T) {
	s := newSite(t, Options{}, sample)
	for _, target := range []string{"/", "/o/start", "/f/", "/api/state", "/api/doc/start", "/api/search?q=a", "/gone", "/o/gone", "/files/gone.png", "/assets/app/demo.js"} {
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
		for _, target := range []string{"/", "/o/start", "/files/ops/net/map.png", "/api/state", "/api/doc/start", "/api/search"} {
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

func TestUserTextStaysText(t *testing.T) {
	s := newSite(t, Options{}, map[string]string{
		"a.md":                 "---\ntitle: \"<script>alert(1)</script>\"\n---\ntext\n",
		"<b>folder/<i>page.md": "text [[gone<script>]]\n",
		"raw.md":               "<script>alert(2)</script>\n\n<img src=x onerror=alert(3)>\n",
	})
	for _, target := range []string{"/api/state", "/api/doc/~a.md", "/api/doc/~raw.md", "/api/search?q=text", "/api/doc/~%3Cb%3Efolder/%3Ci%3Epage.md"} {
		got := s.body(target)
		for _, forbidden := range []string{"<script>alert", "<b>folder", "<i>page", "<img src=x"} {
			if strings.Contains(got, forbidden) {
				t.Errorf("%s: %q arrives as markup", target, forbidden)
			}
		}
	}
	if got := page(t, s, "~a.md"); got.Title != "<script>alert(1)</script>" {
		t.Errorf("the title is data, the workspace escapes it: %q", got.Title)
	}
}

func TestChangesOnDiskAppear(t *testing.T) {
	s := newSite(t, Options{}, map[string]string{"a.md": "---\nid: a\n---\nold text\n"})
	if got := page(t, s, "a"); !strings.Contains(got.Intro, "old text") {
		t.Fatal(got)
	}
	s.write("a.md", "---\nid: a\n---\nnew and longer text\n")
	s.write("b.md", "---\nid: b\ntitle: B\n---\n[[a]]\n")
	s.refresh()
	got := page(t, s, "a")
	if !strings.Contains(got.Intro, "new and longer text") || strings.Contains(got.Intro, "old text") {
		t.Errorf("after the edit: %s", got.Intro)
	}
	if strings.Join(got.UsedIn, ",") != "B" {
		t.Errorf("the new backlink is missing: %v", got.UsedIn)
	}
	if hits := decode[[]apiHit](t, s.get("/api/search?q=longer")); len(hits) != 1 || hits[0].ID != "a" {
		t.Errorf("search did not see the change: %+v", hits)
	}
	for _, target := range []string{"/o/a", "/api/doc/a", "/api/state"} {
		if got := s.get(target).Header().Get("Cache-Control"); got != "no-cache" && got != "no-store" {
			t.Errorf("%s: a cached answer would hide the change: %q", target, got)
		}
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
	if got := page(t, s, "a"); got.Path != "new/deep/a.md" || len(got.UsedIn) != 1 {
		t.Errorf("after the move: %+v", got)
	}
	if !strings.Contains(page(t, s, "b").Intro, `href="/o/a"`) {
		t.Error("the link broke")
	}
	if s.get("/f/old").Code != http.StatusNotFound || s.get("/o/a").Code != http.StatusOK {
		t.Error("the old folder should be gone, the page should stay")
	}
}

func TestNotFoundIsAPage(t *testing.T) {
	s := newSite(t, Options{}, sample)
	for _, target := range []string{"/gone", "/o/gone", "/f/gone", "/files/gone.png"} {
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
	if got := newSite(t, Options{DevUI: true}, sample).body("/dev/ui"); !strings.Contains(got, `class="c22-app-workspace" data-demo>`) {
		t.Error("the gallery is the workspace with the content of the mockup")
	}
	if got := newSite(t, Options{DevUI: true}, sample).body("/"); strings.Contains(got, "data-demo") {
		t.Error("only the gallery is a demo")
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
	if len(used) < 20 {
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
	for _, target := range []string{"/", "/o/plan", "/f/ops", "/dev/ui"} {
		if found := inline.FindString(s.body(target)); found != "" {
			t.Errorf("%s: %q would need a weaker CSP", target, found)
		}
	}
	if found := inline.FindString(s.get("/gone").Body.String()); found != "" {
		t.Errorf("not found: %q", found)
	}
	// The workspace writes its markup in JavaScript: a style attribute there would be blocked, it uses data-style.
	script, err := assetFiles.ReadFile("assets/app/demo.js")
	if err != nil {
		t.Fatal(err)
	}
	if found := regexp.MustCompile(`\sstyle="|\son[a-z]+="`).Find(script); found != nil {
		t.Errorf("demo.js writes %q", found)
	}
}

func TestNothingFromThirdParties(t *testing.T) {
	s := newSite(t, Options{DevUI: true}, sample)
	foreign := regexp.MustCompile(`(?i)(?:src|<link[^>]+href)="(?:https?:)?//`)
	for _, target := range []string{"/", "/o/plan", "/dev/ui"} {
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
		"/assets/app/demo.js":                "javascript",
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
	request := httptest.NewRequest(http.MethodGet, "/api/search?q=private-word", nil)
	request.RemoteAddr = "192.0.2.7:1234"
	handler.ServeHTTP(httptest.NewRecorder(), request)
	got := lines.String()
	if strings.Contains(got, "private-word") || strings.Contains(got, "192.0.2.7") {
		t.Errorf("the log says too much: %s", got)
	}
	if !strings.Contains(got, `"path":"/api/search"`) {
		t.Errorf("the request is missing: %s", got)
	}
}

func TestNothingAnswersWithoutTheGate(t *testing.T) {
	s := newSite(t, Options{}, sample)
	for _, target := range []string{"/", "/o/start", "/f/", "/profile", "/api/state", "/api/doc/start", "/api/search?q=router", "/files/ops/net/map.png", "/gone"} {
		recorder := s.getWith(target, nil)
		if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "Not signed in") || strings.Contains(recorder.Body.String(), "Welcome") {
			t.Errorf("%s = %d", target, recorder.Code)
		}
	}
	if got := s.getWith("/assets/c22/css/c22-vega.css", nil).Code; got != http.StatusOK {
		t.Errorf("the page that says so needs its styles: %d", got)
	}
	dev := newSite(t, Options{DevUser: &account.User{Login: "dev", Name: "Dev"}}, sample)
	if got := decode[apiState](t, dev.getWith("/api/state", nil)); got.User.Name != "Dev" {
		t.Errorf("the development user stands in for the gate: %+v", got.User)
	}
}

func TestOldAddressesMove(t *testing.T) {
	s := newSite(t, Options{}, sample)
	if got := s.get("/d/plan"); got.Code != http.StatusMovedPermanently || got.Header().Get("Location") != "/o/plan" {
		t.Errorf("%d %s", got.Code, got.Header().Get("Location"))
	}
}

// Who is signed in, as the profile shows it; the administration only for administrators (ADR-16).
func TestAccount(t *testing.T) {
	s := newSite(t, Options{AdminGroup: "underdoc-admins"}, sample)
	anna := decode[apiState](t, s.get("/api/state")).User
	if anna.Name != "Anna Berg" || anna.Email != "anna@example.com" || anna.Initials != "AB" || !anna.Admin || strings.Join(anna.Groups, ",") != "ops,underdoc-admins" {
		t.Errorf("anna: %+v", anna)
	}
	if bob := decode[apiState](t, s.getWith("/api/state", map[string]string{"Remote-User": "bob"})).User; bob.Admin || bob.Login != "bob" {
		t.Errorf("only administrators see the administration: %+v", bob)
	}
}

func TestHeadOfAPage(t *testing.T) {
	s := newSite(t, Options{}, map[string]string{
		"ops/net/plan.md": "---\nid: plan\ntitle: Network plan\ndescription: Router and switches.\ntags: [network, main-site]\n---\n# Network plan\n\nText.\n",
	})
	plan := page(t, s, "plan")
	if plan.Title != "Network plan" || plan.Description != "Router and switches." || strings.Join(plan.Tags, ",") != "network,main-site" || strings.Contains(plan.Intro, "Network plan") {
		t.Errorf("%+v", plan)
	}
}
