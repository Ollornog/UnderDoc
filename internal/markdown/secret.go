// SPDX-License-Identifier: MIT

package markdown

import (
	"html"
	"regexp"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// A password in the text (ADR-7) is a reference, ::secret{id="<id>"}, anywhere in a block, also inside a paragraph. The
// value lives sealed in the database; the text never holds it.

// SecretRef matches a reference in Markdown, with the ID of the secret.
var SecretRef = regexp.MustCompile(`::secret\{id="([0-9a-hjkmnp-tv-z]{10})"\}`)

// SecretIDs returns the IDs a text refers to, each once, in their order. Code is not told apart: whoever asks wants
// every reference that could be shown, and one in code counts too.
func SecretIDs(body string) []string {
	var ids []string
	seen := map[string]bool{}
	for _, m := range SecretRef.FindAllStringSubmatch(body, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			ids = append(ids, m[1])
		}
	}
	return ids
}

// SecretSentinel is how a reference shows in rendered HTML. On its own it says the password is not available here, which
// is what it is wherever nothing puts the secret in its place: a page of the folder source, a Markdown file, a frozen
// page. The web replaces it for a page of the database, by what the viewer may do (internal/web). Raw HTML of a page is
// dropped, so nothing typed can look like it.
func SecretSentinel(id string) string {
	return `<span class="badge" data-variant="muted" data-secret="` + html.EscapeString(id) + `">Password not available here</span>`
}

// SecretSentinels finds them in rendered HTML: the whole sentinel and the ID.
var SecretSentinels = regexp.MustCompile(`<span class="badge" data-variant="muted" data-secret="([0-9a-hjkmnp-tv-z]{10})">Password not available here</span>`)

type secretNode struct {
	ast.BaseInline
	id string
}

var kindSecret = ast.NewNodeKind("Secret")

func (n *secretNode) Kind() ast.NodeKind { return kindSecret }

func (n *secretNode) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

type secretParser struct{}

func (secretParser) Trigger() []byte { return []byte{':'} }

func (secretParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
	m := SecretRef.FindSubmatchIndex(line)
	if m == nil || m[0] != 0 {
		return nil
	}
	block.Advance(m[1])
	return &secretNode{id: string(line[m[2]:m[3]])}
}

type secretRenderer struct{}

func (secretRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindSecret, func(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			_, _ = w.WriteString(SecretSentinel(n.(*secretNode).id))
		}
		return ast.WalkContinue, nil
	})
}
