// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The MCP server (ADR-34, docs/mcp.md), through the SDK's own client, signed in as the gate's proxy passes an agent on:
// the same Remote headers as a browser, with Remote-Auth: oauth.

// annasAgent is a request of anna's agent through the gate (OAuth), bobsAgent one of bob's. Anna administers and holds
// the role to create spaces, which her agent never does (B-4).
var (
	annasAgent = merged2(gateHeaders, map[string]string{"Remote-Auth": "oauth", "Remote-Groups": "ops,underdoc-admin,underdoc-spaces"})
	bobsAgent  = merged2(plain, map[string]string{"Remote-Auth": "oauth"})
)

// headerTransport sends the gate's headers with every request, as the gate's proxy does.
type headerTransport struct{ headers map[string]string }

func (h headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for name, value := range h.headers {
		r.Header.Set(name, value)
	}
	return http.DefaultTransport.RoundTrip(r)
}

// mcpSession connects to the MCP server of a site as whoever the headers name.
func mcpSession(t *testing.T, handler http.Handler, headers map[string]string) *mcp.ClientSession {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: server.URL + MCPPath,
		HTTPClient: &http.Client{Transport: headerTransport{headers}}, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// call calls a tool and returns its text and whether it is an error.
func call(t *testing.T, session *mcp.ClientSession, tool string, args map[string]any) (string, bool) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	var b strings.Builder
	for _, c := range result.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String(), result.IsError
}

// rawMCP posts one JSON-RPC message to the endpoint as a program would, without the SDK's client.
func rawMCP(handler http.Handler, headers map[string]string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, MCPPath, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

const listTools = `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`

// TestMCPInstructionsAreShort: the instructions fit the limit clients take, about 4,096 characters (ADR-34).
func TestMCPInstructionsAreShort(t *testing.T) {
	if n := len(mcpInstructions); n >= 4000 {
		t.Errorf("the instructions have %d characters", n)
	}
}

// TestMCPReadAndSearch: an agent starts with context, reads pages, a section and where something is used, and searches
// sections of pages and the pages of a PDF together, one line per hit with the total.
func TestMCPReadAndSearch(t *testing.T) {
	s := withSearch(t, meiliEngine(t))
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kassen","parent":"`+team.ID+`","kind":"reference"}`, sameSite))
	blocks := addBlocks(t, s.site, page.ID, "Die Kassen im Laden.", "## Kartenzahlung", "Fehler E-1042 heißt Terminal neu starten.", "## Bargeld", "Wechselgeld im Tresor.")
	guide := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kasse bedienen","parent":"`+team.ID+`"}`, sameSite))
	addBlocks(t, s.site, guide.ID, "So bedienst du die Kasse.", `::embed{ref="`+page.ID+`#`+blocks[1]+`.section"}`)
	s.drain()

	session := mcpSession(t, s.http, annasAgent)
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	if !slices.Equal(names, []string{"context", "create_page", "move", "read", "replace", "search", "set_front_matter", "trash", "used_in", "write"}) {
		t.Errorf("the tools: %v", names)
	}
	if got := session.InitializeResult().Instructions; !strings.Contains(got, "rules page") {
		t.Errorf("the instructions: %q", got)
	}

	got, failed := call(t, session, "context", nil)
	if failed || !strings.Contains(got, "You act for Anna Berg (anna)") || !strings.Contains(got, "make no space at the top") || !strings.Contains(got, "- [collection] Team underdoc:"+team.ID) ||
		!strings.Contains(got, "  - [page] Kassen underdoc:"+page.ID+" (reference)") || !strings.Contains(got, "Rules page: none yet") {
		t.Errorf("context:\n%s", got)
	}
	rules := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Rules for agents","parent":"`+team.ID+`","kind":"agent-notes"}`, sameSite))
	s.send(http.MethodPut, "/api/objects/"+rules.ID+"/tags", `{"tags":["rules"]}`, sameSite)
	if got, _ := call(t, session, "context", map[string]any{"depth": 1}); !strings.Contains(got, `Rules page: underdoc:`+rules.ID+` "Rules for agents"`) ||
		strings.Contains(got, "[page] Kassen") || !strings.Contains(got, "· 3 inside") {
		t.Errorf("context with the rules page, depth 1:\n%s", got)
	}

	got, failed = call(t, session, "read", map[string]any{"pages": []string{"underdoc:" + page.ID}})
	if failed || !strings.Contains(got, "page: underdoc:"+page.ID+" · version: ") || !strings.Contains(got, "audience: agents") ||
		!strings.Contains(got, "<!-- block "+blocks[2]+" -->\nFehler E-1042") || !strings.Contains(got, "title: Kassen") ||
		!strings.Contains(got, `used in: "Kasse bedienen" underdoc:`+guide.ID) {
		t.Errorf("read the page:\n%s", got)
	}
	got, _ = call(t, session, "read", map[string]any{"pages": []string{"/o/" + page.ID + "#" + blocks[1]}})
	if !strings.Contains(got, "section underdoc:"+page.ID+"#"+blocks[1]) || !strings.Contains(got, "E-1042") || strings.Contains(got, "Wechselgeld") {
		t.Errorf("read a section by a link of the workspace:\n%s", got)
	}
	got, _ = call(t, session, "used_in", map[string]any{"address": "underdoc:" + page.ID + "#" + blocks[1]})
	if !strings.Contains(got, `"Kasse bedienen" underdoc:`+guide.ID) {
		t.Errorf("used in:\n%s", got)
	}
	if got, _ := call(t, session, "used_in", map[string]any{"address": "underdoc:" + page.ID + "#" + blocks[3]}); !strings.Contains(got, "embedded nowhere") {
		t.Errorf("used in for a block nobody embeds:\n%s", got)
	}

	got, failed = call(t, session, "search", map[string]any{"query": "Wechselgeld"})
	if failed || !strings.Contains(got, "1. [page] Kassen › Bargeld underdoc:"+page.ID+"#"+blocks[3]+" — ") || !strings.HasSuffix(got, "1 of 1 found.") {
		t.Errorf("search a section:\n%s", got)
	}
	if got, _ := call(t, session, "search", map[string]any{"query": "E-1042"}); !strings.Contains(got, "underdoc:"+page.ID+"#"+blocks[1]) {
		t.Errorf("search a code:\n%s", got)
	}
	if got, failed := call(t, session, "search", map[string]any{"query": "Kasse", "types": []string{"table"}}); !failed || !strings.Contains(got, "types:") {
		t.Errorf("an unknown type: %v %s", failed, got)
	}
}

// TestMCPIsAlwaysAnAgent, attack: a browser session sent to the endpoint, an administrator, and a request without its
// kind of account. Through MCP the person is always an agent: a space with no agents stays closed, also in the search,
// even to its owner's session; an administrator's agent administers nothing; a request without Remote-Kind is refused.
func TestMCPIsAlwaysAnAgent(t *testing.T) {
	s := withSearch(t, meiliEngine(t))
	team := s.sharedCollection(`{"title":"Personal"}`, sameSite)
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Salaries","parent":"`+team.ID+`"}`, sameSite))
	addBlocks(t, s.site, page.ID, "Gehaltsband Zebrafink.")
	s.drain()
	if got := s.send(http.MethodPut, "/api/objects/"+team.ID+"/grants", `{"kind":"agents","actions":{"read":"deny"}}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("no agents: %d", got)
	}
	// The browser's own session, its cookie sent to the MCP endpoint, is an agent there.
	browser := mcpSession(t, s.http, gateHeaders)
	if got, failed := call(t, browser, "read", map[string]any{"pages": []string{page.ID}}); !failed || !strings.Contains(got, "Not found") {
		t.Errorf("a session through MCP reads in a space with no agents: %s", got)
	}
	// The index has not learnt the deny yet: the check of each hit holds it at once.
	if got, _ := call(t, browser, "search", map[string]any{"query": "Zebrafink"}); strings.Contains(got, page.ID) || !strings.HasSuffix(got, "0 of 0 found.") {
		t.Errorf("a session through MCP finds what no agent may: %s", got)
	}
	if got, _ := call(t, browser, "context", nil); strings.Contains(got, team.ID) {
		t.Errorf("the tree of a session through MCP shows the space with no agents:\n%s", got)
	}
	if got := s.send(http.MethodGet, "/api/doc/"+page.ID, "", sameSite).Code; got != http.StatusOK {
		t.Errorf("anna in the browser = %d", got)
	}
	// A request without the kind of account is refused before any tool, as the browser's is (ADR-32, B-3).
	noKind := merged2(annasAgent, map[string]string{})
	delete(noKind, "Remote-Kind")
	if got := rawMCP(s.http, noKind, listTools); got.Code != http.StatusForbidden {
		t.Errorf("without Remote-Kind = %d", got.Code)
	}
	// Nobody signed in.
	if got := rawMCP(s.http, map[string]string{}, listTools); got.Code != http.StatusUnauthorized {
		t.Errorf("without the gate = %d", got.Code)
	}
}

// TestMCPRefusesOtherSites, attack: the gate signs in with a cookie too, so a page of another site could post to the
// endpoint in the person's name. A browser's cross-site request is refused; a program, which sends no Sec-Fetch-Site,
// passes.
func TestMCPRefusesOtherSites(t *testing.T) {
	s, _ := adminSite(t)
	if got := rawMCP(s.http, merged2(gateHeaders, map[string]string{"Sec-Fetch-Site": "cross-site"}), listTools); got.Code != http.StatusForbidden {
		t.Errorf("a cross-site request = %d %s", got.Code, got.Body.String())
	}
	if got := rawMCP(s.http, merged2(gateHeaders, map[string]string{"Origin": "https://evil.example"}), listTools); got.Code != http.StatusForbidden {
		t.Errorf("a request of another origin = %d %s", got.Code, got.Body.String())
	}
	if got := rawMCP(s.http, annasAgent, listTools); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"name":"read"`) {
		t.Errorf("a program = %d %s", got.Code, got.Body.String())
	}
}

// TestMCPOtherSpacesAndAddresses, attack: bob's agent asks for anna's private notes and a space shared with nobody, and
// names addresses that try paths, schemes and tables. Nothing is found, no address reaches a disk, and "not found" never
// tells whether something exists.
func TestMCPOtherSpacesAndAddresses(t *testing.T) {
	s, _ := adminSite(t)
	state := decode[apiState](t, s.get("/api/state"))
	notes := state.Collections[0]
	private := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Diary","parent":"`+notes.ID+`"}`, sameSite))
	addBlocks(t, s, private.ID, "Private text.")
	space := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Board"}`, sameSite))
	board := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Minutes","parent":"`+space.ID+`"}`, sameSite))
	s.send(http.MethodGet, "/api/state", "", plain)
	bobs := mcpSession(t, s.http, bobsAgent)
	for _, address := range []string{private.ID, "underdoc:" + board.ID, "underdoc:" + board.ID + "#b1", "zzzzzzzzzz"} {
		if got, failed := call(t, bobs, "read", map[string]any{"pages": []string{address}}); !failed || got != errNotFound {
			t.Errorf("bob's agent reads %s: %s", address, got)
		}
	}
	if got, _ := call(t, bobs, "context", map[string]any{"under": "underdoc:" + space.ID}); !strings.Contains(got, errNotFound) {
		t.Errorf("bob's agent lists anna's space: %s", got)
	}
	for _, address := range []string{"../../etc/passwd", "underdoc:../../etc/passwd", "file:///etc/passwd", "underdoc:" + board.ID + "#x1",
		"underdoc:" + board.ID + "/../" + private.ID, "underdoc:folder/", strings.Repeat("a", 600)} {
		if got, failed := call(t, bobs, "read", map[string]any{"pages": []string{address}}); !failed || !strings.Contains(got, "not an address") {
			t.Errorf("read %q: %s", address, got)
		}
	}
	if got, failed := call(t, bobs, "read", map[string]any{"pages": []string{"underdoc:folder/../../etc/passwd"}}); !failed || got != errNotFound {
		t.Errorf("a path as a page of the folder source: %s", got)
	}
	if got, failed := call(t, bobs, "read", map[string]any{"pages": []string{"a", "b", "c", "d", "e", "f"}}); !failed || !strings.Contains(got, "1 to 5") {
		t.Errorf("six pages at once: %s", got)
	}
}

// TestMCPLargePagesComeAsOutline, attack: a page larger than one answer comes as its outline with the address of each
// section, never whole.
func TestMCPLargePagesComeAsOutline(t *testing.T) {
	s, _ := adminSite(t)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Big","parent":"`+team.ID+`"}`, sameSite))
	long := strings.Repeat("word ", 12000)
	blocks := addBlocks(t, s, page.ID, "## One", long, "## Two", long, "## Three")
	session := mcpSession(t, s.http, annasAgent)
	got, failed := call(t, session, "read", map[string]any{"pages": []string{page.ID}})
	if failed || len(got) > 4<<10 || !strings.Contains(got, "more than one answer takes") || !strings.Contains(got, "- Two underdoc:"+page.ID+"#"+blocks[2]) {
		t.Errorf("a large page (%d bytes):\n%.600s", len(got), got)
	}
	if got, _ := call(t, session, "read", map[string]any{"pages": []string{page.ID + "#" + blocks[2]}}); !strings.Contains(got, "section underdoc:") || len(got) < 60000 {
		t.Errorf("its section (%d bytes)", len(got))
	}
}
