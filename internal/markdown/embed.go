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

// An embedding inside a paragraph (ADR-22): :embed{ref="<object>#<block>.<part>"} shows a part of another paragraph in
// the flow of this one, as a value of M-3 does. Its line form, ::embed{…} on a line of its own, embeds blocks, sections
// and parts as blocks; that is handled before Markdown (internal/web).

// InlineEmbed matches the inline form; only parts, and values of spreadsheets (#x<n>, ADR-31), are embedded inside a
// paragraph.
var InlineEmbed = regexp.MustCompile(`:embed\{ref="([0-9a-hjkmnp-tv-z]{10})#(?:(b[1-9][0-9]{0,8})\.(t[1-9][0-9]{0,8})|(x[1-9][0-9]{0,8}))"\}`)

// InlineTarget is the target within the object of a match of InlineEmbed: b3.t2, or x4.
func InlineTarget(m []string) string {
	if m[4] != "" {
		return m[4]
	}
	return m[2] + "." + m[3]
}

// EmbedSentinel is how an inline embedding shows in rendered HTML, for the web to put the part in its place. Raw HTML
// of a page is dropped, so nothing typed can look like it.
func EmbedSentinel(ref string) string {
	return `<span data-embed-inline="` + html.EscapeString(ref) + `"></span>`
}

// EmbedSentinels finds them in rendered HTML: the whole sentinel and the address.
var EmbedSentinels = regexp.MustCompile(`<span data-embed-inline="([0-9a-hjkmnp-tv-z]{10}#(?:b[1-9][0-9]{0,8}\.t[1-9][0-9]{0,8}|x[1-9][0-9]{0,8}))"></span>`)

// inlineEmbed is the node of an inline embedding.
type inlineEmbed struct {
	ast.BaseInline
	ref string
}

var kindInlineEmbed = ast.NewNodeKind("InlineEmbed")

func (n *inlineEmbed) Kind() ast.NodeKind { return kindInlineEmbed }

func (n *inlineEmbed) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

type embedParser struct{}

func (embedParser) Trigger() []byte { return []byte{':'} }

func (embedParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
	m := InlineEmbed.FindSubmatchIndex(line)
	if m == nil || m[0] != 0 {
		return nil
	}
	block.Advance(m[1])
	if m[8] >= 0 {
		return &inlineEmbed{ref: string(line[m[2]:m[3]]) + "#" + string(line[m[8]:m[9]])}
	}
	return &inlineEmbed{ref: string(line[m[2]:m[3]]) + "#" + string(line[m[4]:m[5]]) + "." + string(line[m[6]:m[7]])}
}

type embedRenderer struct{}

func (embedRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindInlineEmbed, func(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			_, _ = w.WriteString(EmbedSentinel(n.(*inlineEmbed).ref))
		}
		return ast.WalkContinue, nil
	})
}
