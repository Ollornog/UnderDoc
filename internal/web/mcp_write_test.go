// SPDX-License-Identifier: MIT

package web

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/files"
)

// The tools that write (ADR-34), through the SDK's client as anna's agent.

var versionOf = regexp.MustCompile(`version: (\d+)`)

// readVersion reads a page through MCP and returns its answer and its version.
func readVersion(t *testing.T, s *site, headers map[string]string, page string) (string, int) {
	t.Helper()
	got, failed := call(t, mcpSession(t, s.http, headers), "read", map[string]any{"pages": []string{page}})
	m := versionOf.FindStringSubmatch(got)
	if failed || m == nil {
		t.Fatalf("read %s: %s", page, got)
	}
	var n int
	_, _ = fmt.Sscan(m[1], &n)
	return got, n
}

// TestMCPWrite: an agent writes a page against the version it read, a section, a passage, a dry run, the front matter;
// a write of a page changed since is refused with "page changed since you read it". Every version is an agent's, and on
// human documentation the changed blocks carry the hint of AI; every write is logged.
func TestMCPWrite(t *testing.T) {
	s, _ := adminSite(t)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kasse bedienen","parent":"`+team.ID+`","kind":"how-to"}`, sameSite))
	ids := addBlocks(t, s, page.ID, "So bedienst du die Kasse im Laden.", "## Anmelden", "1. Karte stecken.", "## Abmelden", "1. Karte ziehen.")
	session := mcpSession(t, s.http, annasAgent)
	raw, v := readVersion(t, s, annasAgent, page.ID)
	body := raw[strings.Index(raw, "---\n"):]
	edited := strings.Replace(body, "1. Karte stecken.", "1. Karte stecken, damit die Kasse dich kennt.", 1)
	edited = strings.Replace(edited, "<!-- block "+ids[3]+" -->\n## Abmelden\n\n<!-- block "+ids[4]+" -->\n1. Karte ziehen.\n",
		"<!-- block -->\n## Pause\n\n<!-- block -->\n1. Bildschirm sperren.\n", 1)

	got, failed := call(t, session, "write", map[string]any{"page": page.ID, "base": v, "markdown": edited, "dry_run": true})
	if failed || !strings.Contains(got, "Dry run, nothing saved") || !strings.Contains(got, "+1. Karte stecken, damit die Kasse dich kennt.") ||
		!strings.Contains(got, "-## Abmelden") {
		t.Errorf("dry run:\n%s", got)
	}
	if _, now := readVersion(t, s, annasAgent, page.ID); now != v {
		t.Errorf("a dry run saved: version %d → %d", v, now)
	}
	got, failed = call(t, session, "write", map[string]any{"page": page.ID, "base": v, "markdown": edited})
	if failed || !strings.Contains(got, "Saved underdoc:"+page.ID) || !strings.Contains(got, "2 added, 1 changed, 2 deleted") ||
		!strings.Contains(got, "The front matter was not written") || !strings.Contains(got, "Check:") {
		t.Errorf("write:\n%s", got)
	}
	// The same base again: the page changed since.
	if got, failed := call(t, session, "write", map[string]any{"page": page.ID, "base": v, "markdown": edited}); !failed ||
		!strings.Contains(got, "page changed since you read it") {
		t.Errorf("a write against an old version: %s", got)
	}
	if got, failed := call(t, session, "write", map[string]any{"page": page.ID, "markdown": edited}); !failed || !strings.Contains(got, "base") {
		t.Errorf("a write without a base: %s", got)
	}
	doc := decode[apiDocument](t, s.get("/api/doc/"+page.ID))
	marked := 0
	for _, b := range doc.Blocks {
		if b.AI != nil {
			marked++
		}
	}
	if len(doc.Blocks) != 5 || marked != 3 || len(doc.Removed) != 2 {
		t.Errorf("the hints of AI: %d blocks, %d marked, removed %+v", len(doc.Blocks), marked, doc.Removed)
	}
	for _, version := range decode[[]apiVersion](t, s.get("/api/pages/"+page.ID+"/versions"))[:5] {
		if !version.AI {
			t.Errorf("a version written through MCP is not an agent's: %+v", version)
		}
	}

	// A section, by the block that opens it; a marker of a block outside the section makes a new block there.
	_, v = readVersion(t, s, annasAgent, page.ID)
	got, failed = call(t, session, "write", map[string]any{"page": page.ID + "#" + ids[1] + ".section", "base": v,
		"markdown": "<!-- block " + ids[1] + " -->\n## Anmelden\n\n<!-- block " + ids[0] + " -->\nKarte bereithalten.\n\n<!-- block -->\n1. Karte stecken."})
	if failed || !strings.Contains(got, "2 added") {
		t.Errorf("write a section:\n%s", got)
	}
	doc = decode[apiDocument](t, s.get("/api/doc/"+page.ID))
	if doc.Blocks[0].ID != ids[0] || doc.Blocks[0].Markdown != "So bedienst du die Kasse im Laden." {
		t.Errorf("the block before the section changed: %+v", doc.Blocks[0])
	}

	// A passage.
	_, v = readVersion(t, s, annasAgent, page.ID)
	if got, failed := call(t, session, "replace", map[string]any{"page": page.ID, "base": v, "old": "Karte", "new": "Chip"}); !failed || !strings.Contains(got, "times") {
		t.Errorf("a passage that stands several times: %s", got)
	}
	if got, failed := call(t, session, "replace", map[string]any{"page": page.ID, "base": v, "old": "Bildschirm sperren", "new": "Bildschirm mit Windows+L sperren"}); failed ||
		!strings.Contains(got, "1 changed") || !strings.Contains(got, "+1. Bildschirm mit Windows+L sperren.") {
		t.Errorf("replace a passage:\n%s", got)
	}
	if got, failed := call(t, session, "replace", map[string]any{"page": page.ID, "base": v, "old": "Pause", "new": "Mittag"}); !failed || !strings.Contains(got, "page changed") {
		t.Errorf("replace against an old version: %s", got)
	}

	// The check: an error blocks, warnings come with their fix, a link to nowhere and a page too large are told.
	_, v = readVersion(t, s, annasAgent, page.ID)
	if got, failed := call(t, session, "write", map[string]any{"page": page.ID, "base": v,
		"markdown": "<!-- block -->\nSo bedienst du die Kasse.\n\n<!-- block -->\n---\nkind: how-to\ntitle: x\n---\nText"}); !failed || !strings.Contains(got, "front matter") {
		t.Errorf("front matter in a block: %s", got)
	}
	got, failed = call(t, session, "write", map[string]any{"page": page.ID, "base": v,
		"markdown": "So bedienst du die Kasse.\n\nSiehe [die Anleitung](/o/zzzzzzzzzz) und [mehr](underdoc:" + team.ID + ").\n\n" + strings.Repeat("Wort ", 6000)})
	if failed || strings.Count(got, "warning LINK") != 1 || !strings.Contains(got, "zzzzzzzzzz leads nowhere") ||
		!strings.Contains(got, "warning PAGE-SIZE") || !strings.Contains(got, "warning STEPS") || !strings.Contains(got, "Fix:") {
		t.Errorf("the check of a write:\n%s", got)
	}

	// The front matter.
	if got, failed := call(t, session, "set_front_matter", map[string]any{"page": page.ID, "title": "Kasse", "tags": []string{"Kasse", "laden"}}); failed ||
		!strings.Contains(got, `title "Kasse"`) || !strings.Contains(got, "[kasse laden]") {
		t.Errorf("set the front matter: %s", got)
	}
	// Anna wrote the page, so her agent may not take it out of the human documentation (ADR-33).
	if got, failed := call(t, session, "set_front_matter", map[string]any{"page": page.ID, "kind": "reference"}); !failed || !strings.Contains(got, "Not allowed") {
		t.Errorf("an agent hides its hints by the kind: %s", got)
	}
}

// TestMCPStructure: a new page with text, moving a page and a block, and the trash.
func TestMCPStructure(t *testing.T) {
	s, _ := adminSite(t)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	archive := decode[apiNode](t, s.send(http.MethodPost, "/api/folders", `{"title":"Archive","parent":"`+team.ID+`"}`, sameSite))
	session := mcpSession(t, s.http, annasAgent)
	got, failed := call(t, session, "create_page", map[string]any{"parent": "underdoc:" + team.ID, "title": "Drucker", "kind": "reference",
		"markdown": "Die Drucker im Laden.\n\n## Bondrucker\n\nPort 9100.\n\n## Etiketten\n\nZebra."})
	m := regexp.MustCompile(`Made underdoc:([0-9a-z]{10})`).FindStringSubmatch(got)
	if failed || m == nil || !strings.Contains(got, "3 added") {
		t.Fatalf("create a page:\n%s", got)
	}
	printers := m[1]
	if got, failed := call(t, session, "create_page", map[string]any{"parent": "", "title": "My space"}); !failed || !strings.Contains(got, "parent:") {
		t.Errorf("a space through MCP: %s", got)
	}
	if got, failed := call(t, session, "move", map[string]any{"object": printers, "to": archive.ID}); failed || !strings.Contains(got, "underdoc:"+printers+" stays") {
		t.Errorf("move a page: %s", got)
	}
	if got, failed := call(t, session, "move", map[string]any{"object": printers, "to": ""}); !failed || !strings.Contains(got, "to:") {
		t.Errorf("move to the top: %s", got)
	}
	other := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Etiketten","parent":"`+team.ID+`","kind":"reference"}`, sameSite))
	addBlocks(t, s, other.ID, "Etiketten drucken.")
	doc := decode[apiDocument](t, s.get("/api/doc/"+printers))
	last := doc.Blocks[len(doc.Blocks)-1].ID
	got, failed = call(t, session, "move", map[string]any{"object": printers + "#" + last, "to": other.ID})
	if failed || !strings.Contains(got, "the old address leads there") {
		t.Errorf("move a block: %s", got)
	}
	if got, _ := call(t, session, "read", map[string]any{"pages": []string{printers + "#" + last}}); !strings.Contains(got, "moved to underdoc:"+other.ID) {
		t.Errorf("read a moved block: %s", got)
	}
	if got, failed := call(t, session, "trash", map[string]any{"object": "underdoc:" + other.ID}); failed || !strings.Contains(got, "to the trash") {
		t.Errorf("trash: %s", got)
	}
	if got, failed := call(t, session, "read", map[string]any{"pages": []string{other.ID}}); !failed || got != errNotFound {
		t.Errorf("read what lies in the trash: %s", got)
	}
	trash := decode[[]apiTrashed](t, s.get("/api/trash"))
	if len(trash) != 1 || trash[0].ID != other.ID {
		t.Errorf("anna finds it in her trash to restore: %+v", trash)
	}
	// The log keeps what the agent did, as it keeps a person's changes (ADR-11): a change once an hour per object.
	log := decode[[]apiEntry](t, s.get("/api/log?object="+printers))
	if !slices.ContainsFunc(log, func(e apiEntry) bool { return e.Action == "change" && e.Detail == "made through mcp" }) {
		t.Errorf("the new page is not logged: %+v", log)
	}
	bobsPage := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Notes","parent":"`+team.ID+`"}`, sameSite))
	s.send(http.MethodGet, "/api/state", "", plain)
	if got, failed := call(t, mcpSession(t, s.http, bobsAgent), "write", map[string]any{"page": bobsPage.ID, "base": 0, "markdown": "Bob's agent was here."}); failed {
		t.Fatalf("bob's agent writes in Team: %s", got)
	}
	log = decode[[]apiEntry](t, s.get("/api/log?object="+bobsPage.ID))
	if !slices.ContainsFunc(log, func(e apiEntry) bool { return e.Actor == "1002" && e.Detail == "mcp write" }) {
		t.Errorf("the write of bob's agent is not logged: %+v", log)
	}
}

// TestMCPWriteAttacks, attack: writing what the person may only read, in a space with no agents, beyond the limit a
// person set for their agents, a request too large, and a base given as text.
func TestMCPWriteAttacks(t *testing.T) {
	s, _ := adminSite(t)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	readOnly := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Handbook"}`, sameSite))
	s.send(http.MethodPut, "/api/objects/"+readOnly.ID+"/grants", `{"kind":"tenant","role":"read"}`, sameSite)
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Rules","parent":"`+readOnly.ID+`"}`, sameSite))
	addBlocks(t, s, page.ID, "Read only.")
	s.send(http.MethodGet, "/api/state", "", plain)
	bobs := mcpSession(t, s.http, bobsAgent)
	_, v := readVersion(t, s, bobsAgent, page.ID)
	for name, c := range map[string][2]any{
		"write":       {"write", map[string]any{"page": page.ID, "base": v, "markdown": "Bob was here."}},
		"replace":     {"replace", map[string]any{"page": page.ID, "base": v, "old": "Read", "new": "Write"}},
		"front":       {"set_front_matter", map[string]any{"page": page.ID, "title": "Bob's"}},
		"create":      {"create_page", map[string]any{"parent": readOnly.ID, "title": "Bob's page"}},
		"move":        {"move", map[string]any{"object": page.ID, "to": team.ID}},
		"trash":       {"trash", map[string]any{"object": page.ID}},
		"dry run too": {"write", map[string]any{"page": page.ID, "base": v, "markdown": "Bob was here.", "dry_run": true}},
	} {
		if got, failed := call(t, bobs, c[0].(string), c[1].(map[string]any)); !failed || !strings.Contains(got, "Not allowed") && !strings.Contains(got, "Not found") {
			t.Errorf("bob's agent may only read, %s: %s", name, got)
		}
	}
	if got := decode[apiDocument](t, s.get("/api/doc/"+page.ID)); len(got.Blocks) != 1 || got.Blocks[0].Markdown != "Read only." || got.Title != "Rules" {
		t.Errorf("the page changed: %+v", got)
	}
	// Anna limits her agents to reading.
	mine := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Mine","parent":"`+team.ID+`"}`, sameSite))
	if got := s.send(http.MethodPut, "/api/me/agents", `{"actions":["read"]}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("limit = %d", got)
	}
	annas := mcpSession(t, s.http, annasAgent)
	if got, failed := call(t, annas, "write", map[string]any{"page": mine.ID, "base": 0, "markdown": "Agent text."}); !failed || !strings.Contains(got, "Not allowed") {
		t.Errorf("anna's agent writes beyond her limit: %s", got)
	}
	s.send(http.MethodPut, "/api/me/agents", `{"actions":["read","write","create","comment","delete"]}`, sameSite)
	// No agents in Team.
	s.send(http.MethodPut, "/api/objects/"+team.ID+"/grants", `{"kind":"agents","actions":{"read":"deny"}}`, sameSite)
	if got, failed := call(t, annas, "write", map[string]any{"page": mine.ID, "base": 0, "markdown": "Agent text."}); !failed || got != errNotFound {
		t.Errorf("anna's agent writes in a space with no agents: %s", got)
	}
	// A base as text is no base, and a request beyond the limit is refused before any tool.
	if got := rawMCP(s.http, annasAgent, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write","arguments":{"page":"`+mine.ID+`","base":"latest","markdown":"x"}}}`); !strings.Contains(got.Body.String(), "base") {
		t.Errorf("a base as text: %d %s", got.Code, got.Body.String())
	}
	huge := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write","arguments":{"page":"` + mine.ID + `","base":0,"markdown":"` + strings.Repeat("x", 3<<20) + `"}}}`
	if got := rawMCP(s.http, annasAgent, huge); got.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("a request of 3 MB = %d", got.Code)
	}
}

// TestMCPFiles: an agent lists the files and reads the text of a PDF page by page; bob's agent finds nothing of anna's
// own folder.
func TestMCPFiles(t *testing.T) {
	s := withFiles(t)
	mine := s.root(nil, "user")
	done := s.put(nil, `"parent":"`+mine.ID+`"`, "manual.pdf", []byte("%PDF-1.4 not really"), 1<<10)
	if done.Code != http.StatusCreated {
		t.Fatalf("upload = %d %s", done.Code, done.Body.String())
	}
	file := decode[struct {
		ID string `json:"id"`
	}](t, done).ID
	// The text as the reader of PDF on the server keeps it: the pages apart by a form feed.
	if err := s.db.SaveFileText(inMain(), file, "1007", files.TextServer, "Seite eins\fSeite zwei mit E-1042\f"); err != nil {
		t.Fatal(err)
	}
	session := mcpSession(t, s.http, annasAgent)
	if got, failed := call(t, session, "list_files", nil); failed || !strings.Contains(got, "underdoc:"+mine.ID) {
		t.Errorf("the roots: %s", got)
	}
	if got, failed := call(t, session, "list_files", map[string]any{"in": mine.ID}); failed || !strings.Contains(got, "[file] manual.pdf underdoc:"+file) ||
		!strings.HasSuffix(got, "1 of 1 shown.") {
		t.Errorf("the files of anna's folder: %s", got)
	}
	got, failed := call(t, session, "read_file_page", map[string]any{"file": "underdoc:" + file + "#page=2"})
	if failed || !strings.Contains(got, "page 2 of 3") || !strings.Contains(got, "Seite zwei mit E-1042") || strings.Contains(got, "Seite eins") {
		t.Errorf("page 2: %s", got)
	}
	if got, _ := call(t, session, "read_file_page", map[string]any{"file": "underdoc:" + file + "#page=3"}); !strings.Contains(got, "(no text on this page)") {
		t.Errorf("an empty page: %s", got)
	}
	if got, failed := call(t, session, "read_file_page", map[string]any{"file": "underdoc:" + file + "#page=9"}); !failed || !strings.Contains(got, "has 3 pages") {
		t.Errorf("a page beyond the end: %s", got)
	}
	s.send(http.MethodGet, "/api/state", "", plain)
	bobs := mcpSession(t, s.http, bobsAgent)
	for tool, args := range map[string]map[string]any{
		"list_files":     {"in": mine.ID},
		"read_file_page": {"file": "underdoc:" + file},
	} {
		if got, failed := call(t, bobs, tool, args); !failed || got != errNotFound {
			t.Errorf("bob's agent, %s in anna's folder: %s", tool, got)
		}
	}
}

// putLink sends a file to a one-time upload link as a program does: no sign-in, no browser.
func putLink(s *filesSite, link string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	u, err := url.Parse(link)
	if err != nil {
		panic(err)
	}
	request := httptest.NewRequest(http.MethodPut, u.Path, bytes.NewReader(body))
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	s.http.ServeHTTP(recorder, request)
	return recorder
}

var uploadLink = regexp.MustCompile(`https?://[^/\s]+/s/u/[A-Za-z0-9_-]{43}`)

// TestMCPUploadLink: a file reaches UnderDoc through a one-time link, never inside a message. Attack: the link used
// twice, from a browser of another site, with another size, for a place the person may not write, and after the person
// closed the place to agents; a link of bob's agent into anna's folder.
func TestMCPUploadLink(t *testing.T) {
	s := withFiles(t)
	mine := s.root(nil, "user")
	session := mcpSession(t, s.http, annasAgent)
	ask := func(name string, size int) string {
		t.Helper()
		got, failed := call(t, session, "upload_link", map[string]any{"in": mine.ID, "name": name, "size": size})
		link := uploadLink.FindString(got)
		if failed || link == "" {
			t.Fatalf("upload_link: %s", got)
		}
		return link
	}
	content := []byte("Inventur am Freitag.\n")
	link := ask("inventur.txt", len(content))
	if got := putLink(s, link, content, map[string]string{"Sec-Fetch-Site": "cross-site"}); got.Code != http.StatusForbidden {
		t.Errorf("from a browser of another site = %d", got.Code)
	}
	if got := putLink(s, link, content, nil); got.Code != http.StatusCreated || !strings.Contains(got.Body.String(), `"name":"inventur.txt"`) {
		t.Fatalf("the upload = %d %s", got.Code, got.Body.String())
	}
	if got := putLink(s, link, content, nil); got.Code != http.StatusNotFound {
		t.Errorf("the link a second time = %d", got.Code)
	}
	if got, _ := call(t, session, "list_files", map[string]any{"in": mine.ID}); !strings.Contains(got, "inventur.txt") {
		t.Errorf("the file is not there: %s", got)
	}
	link = ask("other.txt", 5)
	if got := putLink(s, link, []byte("more than five"), nil); got.Code != http.StatusBadRequest {
		t.Errorf("more than the size = %d", got.Code)
	}
	if got := putLink(s, link, []byte("12345"), nil); got.Code != http.StatusNotFound {
		t.Errorf("the link after a failed upload = %d", got.Code)
	}
	// The rights hold again when the file is made: anna lets her agents only read after the link was made.
	link = ask("late.txt", 4)
	if got := s.send(http.MethodPut, "/api/me/agents", `{"actions":["read"]}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("limit = %d", got)
	}
	if got := putLink(s, link, []byte("late"), nil); got.Code != http.StatusForbidden {
		t.Errorf("a link of an agent that may only read since = %d %s", got.Code, got.Body.String())
	}
	if got, failed := call(t, session, "upload_link", map[string]any{"in": mine.ID, "name": "x.txt", "size": 1}); !failed || !strings.Contains(got, "Not allowed") {
		t.Errorf("a link of an agent that may only read: %s", got)
	}
	s.send(http.MethodGet, "/api/state", "", plain)
	bobs := mcpSession(t, s.http, bobsAgent)
	if got, failed := call(t, bobs, "upload_link", map[string]any{"in": mine.ID, "name": "x.txt", "size": 1}); !failed || got != errNotFound {
		t.Errorf("bob's agent, a link into anna's folder: %s", got)
	}
	if got := putLink(s, "/s/u/"+strings.Repeat("A", 43), []byte("x"), nil); got.Code != http.StatusNotFound {
		t.Errorf("a made-up link = %d", got.Code)
	}
}
