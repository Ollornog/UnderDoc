// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/document"
	"github.com/Ollornog/UnderDoc/internal/id"
	"github.com/Ollornog/UnderDoc/internal/pagekind"
	"github.com/Ollornog/UnderDoc/internal/search"
)

// UnderDoc as an MCP server for its documents (ADR-34, T-21, docs/mcp.md). Agents reach it at MCPPath through the gate,
// which signs them in with OAuth and passes on the same Remote headers as for a browser (ADR-32, ADR-35): UnderDoc carries
// no OAuth code. Every call runs as the person behind the token, with the same check of rights as the browser, and always
// as an agent (account.User.MCP), so the ceiling of the group Agents holds (ADR-36).
//
// The server is stateless: each POST is a whole exchange, made for its request, so every tool sees the request's
// person, tenant and connection and nothing of another. A small core of tools, no resources and no prompts.

// MCPPath is where the MCP server answers. The operator lists https://<host>/mcp as an OAuth resource of the gate, whose
// tokens hold for this path and below it.
const MCPPath = "/mcp"

// Limits of the answers (ADR-34): compact, so an agent's context is not flooded.
const (
	// mcpMaxAnswer is the most text one answer carries; a page above it is answered with its outline.
	mcpMaxAnswer = 100 << 10
	// mcpMaxRows is the most rows of a list in an answer: hits, entries of a folder, pages that use something.
	mcpMaxRows = 20
	// mcpMaxPages is how many pages one read takes.
	mcpMaxPages = 5
	// mcpMaxDepth is how deep the tree of context goes, mcpDepth its depth unless asked, mcpMaxTree the most entries.
	mcpMaxDepth = 4
	mcpDepth    = 2
	mcpMaxTree  = 200
	// mcpMaxBody is the most a request to the MCP server may carry.
	mcpMaxBody = 2 << 20
	// mcpSearchDepth is how many hits a search counts at most for its total.
	mcpSearchDepth = 100
)

// The scheme of the addresses an agent names (ADR-34): underdoc:<id> for an object of the database, with #<block>,
// #<block>.<part> or #<block>.section, and #page=<n> for a page of a file; underdoc:folder/<id> for a page of the folder
// source. Other sources get schemes of their own later (T-31).
const (
	uriScheme = "underdoc:"
	uriFolder = "folder/"
)

// mcpInstructions are short and point to the rules page, the single source of the conventions (ADR-34). They stay well
// below the 4,096 characters clients take (TestMCPInstructionsAreShort).
const mcpInstructions = `UnderDoc is a wiki of pages and files. You act for the signed-in person, always as an agent: you may read, search, ` +
	`create, write, comment and move to the trash what they may. You never manage rights, make share links, delete for good ` +
	`or read passwords; a space set to "no agents" is closed to you.

Start with the tool context: it gives the tree, the person and the rules page of this instance. Read the rules page ` +
	`before you write; it is the one source of the conventions, and the check at every write enforces it.

Addresses: underdoc:<id> is an object, underdoc:<id>#b3 a block or the section a heading block opens, underdoc:<id>#b3.t2 ` +
	`a part, underdoc:<id>#page=2 a page of a PDF. Embed with ::embed{ref="<id>#b3"} or :embed{ref="<id>#b3.t2"}.

A page is Markdown in blocks, each after a marker line <!-- block b3 -->. read gives it with its front matter, markers ` +
	`and version.

Audiences: reference and agent-notes are the agent side, write them short and dense. How-to, explanation and tutorial ` +
	`are for people: an opening sentence, numbered steps with the why, values embedded from a reference, never copied. ` +
	`Your changes there show as "written by AI, not checked" until a person checks them. Structure kinds (overview, ` +
	`register, record, plan, source) hold structure.

Limits: a search answers at most 20 hits, lists at most 20 rows, the tree goes to a depth; a page at most 25 KB, a ` +
	`description at most 120 characters.

Work: move the work forward first, then ask whether and where to save it; do not save every intermediate state. Choose ` +
	`place, tags and names yourself and ask the person only about facts. Knowledge for everyone goes into the open with ` +
	`tags; spaces hold what is internal or sensitive.`

// mcpVersion is the version of UnderDoc the server names, from the build.
func mcpVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "devel"
}

// mcpHandler is the endpoint: a request from a browser of another site is refused (the gate signs in with a cookie
// too), the person of the request becomes an agent, and the SDK's stateless handler answers it with a server made for
// this request.
func (h *handler) mcpHandler() http.Handler {
	cache := mcp.NewSchemaCache()
	streamable := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server { return h.mcpServer(r, cache) },
		&mcp.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
			Logger:       h.log,
			// UnderDoc is reached through the gate's proxy, never directly: the Host is the gate's, and the address it
			// connects from is the proxy's. The check against DNS rebinding is for servers on a desktop.
			DisableLocalhostProtection: true,
			MaxRequestBodyBytes:        mcpMaxBody,
		})
	protection := http.NewCrossOriginProtection()
	return protection.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.options.DB == nil {
			writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "the MCP server needs the database"})
			return
		}
		user, ok := account.From(r.Context())
		if !ok || user.ID == "" {
			h.notSignedIn(w)
			return
		}
		// Every call through MCP is an agent's (ADR-34, ADR-36): the connection of each request learns it from the
		// context, and an agent administers nothing.
		user.MCP, user.Admin, user.AdminRole = true, false, false
		streamable.ServeHTTP(w, r.WithContext(account.With(r.Context(), user)))
	}))
}

// mcpServer makes the server of one request with the tools that apply to it: those of a module only while the module
// is on (ADR-25).
func (h *handler) mcpServer(r *http.Request, cache *mcp.SchemaCache) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "underdoc", Title: "UnderDoc", Version: mcpVersion()},
		&mcp.ServerOptions{Instructions: mcpInstructions, SchemaCache: cache, Logger: h.log})
	t := &mcpTools{h: h, r: r}
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}
	mcp.AddTool(server, &mcp.Tool{Name: "context", Annotations: readOnly,
		Description: "Start here. The topic tree to a depth with the address of each entry, the rules page of this instance, " +
			"who you act for with what an agent may do, and open hints."}, t.context)
	mcp.AddTool(server, &mcp.Tool{Name: "read", Annotations: readOnly,
		Description: "Read one or several pages (up to 5) as Markdown with front matter, block markers and the version to " +
			"write against, or one section or block (underdoc:<id>#b3). Also a page of the folder source."}, t.read)
	mcp.AddTool(server, &mcp.Tool{Name: "used_in", Annotations: readOnly,
		Description: "Where a page, block, section or part is embedded: the pages that would change with it."}, t.usedIn)
	mcp.AddTool(server, &mcp.Tool{Name: "search", Annotations: readOnly,
		Description: "One search over pages and the text of files and PDFs, ranked together: one line per hit with its " +
			"address and section, a short excerpt, at most 20 hits and the total at the end. Error codes are found exactly."}, t.search)
	return server
}

// mcpTools are the tools of one request.
type mcpTools struct {
	h *handler
	r *http.Request
}

func (t *mcpTools) user() account.User {
	user, _ := account.From(t.r.Context())
	return user
}

// text is the answer of a tool: plain text, compact.
func text(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// refusal is an answer that says what went wrong and how to go on. Errors of a tool are answers to the agent, not
// failures of the protocol, so the agent reads them and corrects itself.
func refusal(s string) *mcp.CallToolResult {
	result := text(s)
	result.IsError = true
	return result
}

// errNotFound hides whether something exists that the person may not see, as the browser does (ADR-29).
const errNotFound = "Not found: no such object, or the person you act for may not see it."

// failure turns an error of the database into an answer the agent can act on; an unexpected one is logged and named
// only as an internal error.
func (t *mcpTools) failure(err error, what string) *mcp.CallToolResult {
	var rule *db.RuleError
	switch {
	case errors.Is(err, db.ErrNotFound):
		return refusal(errNotFound)
	case errors.As(err, &rule):
		return refusal(err.Error() + "\n" + findingLines(rule.Findings))
	case errors.Is(err, db.ErrNotAllowed), errors.Is(err, db.ErrPrivate), errors.Is(err, db.ErrGroup), errors.Is(err, db.ErrSecretHeld):
		return refusal("Not allowed: " + err.Error())
	case errors.Is(err, db.ErrTitle), errors.Is(err, db.ErrKind), errors.Is(err, db.ErrBlock), errors.Is(err, db.ErrPlace),
		errors.Is(err, db.ErrTooMany), errors.Is(err, db.ErrTag), errors.Is(err, db.ErrQuota):
		return refusal(err.Error())
	}
	t.h.log.Error("mcp "+what+" failed", slog.String("error", err.Error()))
	return refusal("Internal error while trying to " + what + ".")
}

// mcpRef is what a tool names: an address of the database, a page of a file, or a page of the folder source.
type mcpRef struct {
	Address db.Address
	// FilePage is the page of a file, from #page=<n>; 0 for none.
	FilePage int
	// Folder is the ID of a page of the folder source.
	Folder document.ID
}

// uri is the address of an object of the database as the tools name it.
func uri(a db.Address) string { return uriScheme + a.String() }

// folderURI is the address of a page of the folder source.
func folderURI(docID string) string { return uriScheme + uriFolder + docID }

// errRef is an address the tools do not read.
var errRef = errors.New(`not an address: use underdoc:<id>, underdoc:<id>#b3, underdoc:<id>#b3.t2, underdoc:<id>#page=2 or underdoc:folder/<id>`)

// parseRef reads an address as an agent writes it: underdoc:<address>, the bare address, or a link of the workspace
// (/o/<id>#<anchor>, also with scheme and host). Nothing in it reaches a path on a disk: an object is found by its ID in
// the database, a page of the folder source by its ID in the library.
func parseRef(s string) (mcpRef, error) {
	s = strings.TrimSpace(s)
	if len(s) > 512 {
		return mcpRef{}, errRef
	}
	if rest, ok := strings.CutPrefix(s, uriScheme); ok {
		s = rest
	} else if u, err := url.Parse(s); err == nil && strings.HasPrefix(u.Path, "/o/") {
		s = strings.TrimPrefix(u.Path, "/o/")
		if u.Fragment != "" {
			s += "#" + u.Fragment
		}
	}
	if rest, ok := strings.CutPrefix(s, uriFolder); ok {
		if rest == "" || strings.ContainsAny(rest, "\x00\r\n#") {
			return mcpRef{}, errRef
		}
		return mcpRef{Folder: document.ID(rest)}, nil
	}
	if object, page, ok := strings.Cut(s, "#page="); ok {
		n, err := strconv.Atoi(page)
		if !id.Valid(object) || err != nil || n < 1 || n > 100000 {
			return mcpRef{}, errRef
		}
		return mcpRef{Address: db.Address{Object: object}, FilePage: n}, nil
	}
	a, ok := db.ParseAddress(s)
	if !ok || db.IsTableID(a.Block) {
		return mcpRef{}, errRef
	}
	return mcpRef{Address: a}, nil
}

// clip cuts an answer at the limit of an answer, on a line, and says so.
func clip(s string) string {
	if len(s) <= mcpMaxAnswer {
		return s
	}
	cut := strings.LastIndex(s[:mcpMaxAnswer], "\n")
	if cut < 0 {
		cut = mcpMaxAnswer
	}
	return s[:cut] + fmt.Sprintf("\n… cut at %d KB; ask for less.\n", mcpMaxAnswer>>10)
}

// oneLine puts text on one line, at most n characters.
func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// searchTypes are the types a search may be narrowed to.
var searchTypes = []string{search.TypePage, search.TypeFile, search.TypeFolder, search.TypeCollection}

// mcpContext is the context of a tool run in the request's context, so the connection to the database learns the
// tenant and that an agent acts (db.Open, PrepareConn).
func (t *mcpTools) ctx() context.Context { return t.r.Context() }

// findingLines writes the findings of the check, one per line: errors block, warnings come with their fix (ADR-33).
func findingLines(found []pagekind.Finding) string {
	if len(found) == 0 {
		return "Check: no findings."
	}
	var b strings.Builder
	b.WriteString("Check:\n")
	for i, f := range found {
		if i == mcpMaxRows {
			fmt.Fprintf(&b, "… %d findings in all.\n", len(found))
			break
		}
		where := ""
		if f.Block != "" {
			where = " #" + f.Block
		}
		fmt.Fprintf(&b, "- %s %s%s: %s Fix: %s\n", f.Level, f.Code, where, f.Message, f.Fix)
	}
	return strings.TrimRight(b.String(), "\n")
}
