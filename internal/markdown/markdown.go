// SPDX-License-Identifier: MIT

// Package markdown parses pages and renders them to sanitized HTML with the canonical classes of the design system.
//
// Raw HTML in a page is dropped, and only a short list of link schemes survives: Markdown files come from people, and a
// shared wiki must not turn them into scripts for every reader.
package markdown

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.yaml.in/yaml/v4"
)

var engine = goldmark.New(
	goldmark.WithExtensions(
		// Attributes instead of the default inline style, which a strict CSP would block.
		extension.NewTable(extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute)),
		extension.Strikethrough,
		extension.Linkify,
		extension.TaskList,
		extension.Footnote,
	),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
		parser.WithInlineParsers(util.Prioritized(wikiParser{}, 199)), // before the link parser at 200
	),
	goldmark.WithRendererOptions(renderer.WithNodeRenderers(util.Prioritized(nodeRenderer{}, 100))),
)

// Front is the front matter a page may start with. All other keys are ignored.
type Front struct {
	ID          string `yaml:"id"`
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Tags        Tags   `yaml:"tags"`
}

// Tags accepts a YAML list as well as one string with commas, since both are common in hand-written front matter.
type Tags []string

// UnmarshalYAML reads either form.
func (t *Tags) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		*t = nil
		for _, part := range strings.Split(node.Value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				*t = append(*t, part)
			}
		}
		return nil
	}
	var list []string
	if err := node.Decode(&list); err != nil {
		return err
	}
	*t = list
	return nil
}

// Page is a parsed page that has not been rendered yet.
type Page struct {
	Front Front
	// FrontError is set if the front matter is not valid YAML. The page itself still works.
	FrontError error
	// Heading is the text of the first top-level heading, if any.
	Heading string
	// StartsWithHeading is true if the body opens with that heading.
	StartsWithHeading bool
	source            []byte
	root              ast.Node
}

// Parse reads a page. It accepts what text editors produce: a byte order mark, CRLF line ends and broken encodings.
func Parse(content []byte) *Page {
	content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))
	content = bytes.ToValidUTF8(content, []byte("�"))
	page := &Page{}
	yamlPart, body, found := splitFrontMatter(content)
	if found {
		page.FrontError = yaml.Unmarshal(yamlPart, &page.Front)
		if page.FrontError != nil {
			page.Front = Front{}
		}
	}
	page.source = body
	ctx := parser.NewContext(parser.WithIDs(&headingIDs{seen: map[string]int{}}))
	page.root = engine.Parser().Parse(text.NewReader(body), parser.WithContext(ctx))
	page.findHeading()
	return page
}

func splitFrontMatter(content []byte) (yamlPart, body []byte, found bool) {
	const limit = 64 << 10
	first, rest, ok := bytes.Cut(content, []byte("\n"))
	if !ok || string(bytes.TrimRight(first, " \t\r")) != "---" {
		return nil, content, false
	}
	consumed := 0
	for consumed < len(rest) && consumed < limit {
		line, after, more := bytes.Cut(rest[consumed:], []byte("\n"))
		if string(bytes.TrimRight(line, " \t\r")) == "---" {
			return rest[:consumed], after, true
		}
		if !more {
			break
		}
		consumed += len(line) + 1
	}
	return nil, content, false
}

func (p *Page) findHeading() {
	_ = ast.Walk(p.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		heading, ok := n.(*ast.Heading)
		if !entering || !ok || heading.Level != 1 {
			return ast.WalkContinue, nil
		}
		var b strings.Builder
		collectText(heading, p.source, &b)
		p.Heading = strings.Join(strings.Fields(b.String()), " ")
		p.StartsWithHeading = n.Parent() == p.root && n.PreviousSibling() == nil
		return ast.WalkStop, nil
	})
}

// Text returns the plain text of the page on one line, without raw HTML.
func (p *Page) Text() string {
	var b strings.Builder
	collectText(p.root, p.source, &b)
	return strings.Join(strings.Fields(b.String()), " ")
}

func collectText(n ast.Node, source []byte, b *strings.Builder) {
	switch v := n.(type) {
	case *ast.Text:
		b.Write(v.Value(source))
		if v.SoftLineBreak() || v.HardLineBreak() {
			b.WriteByte(' ')
		}
	case *ast.String:
		b.Write(v.Value)
	case *ast.AutoLink:
		b.Write(v.Label(source))
	case *WikiLink:
		b.WriteString(v.label())
	case *ast.HTMLBlock, *ast.RawHTML:
		return
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		lines := n.Lines()
		for i := range lines.Len() {
			segment := lines.At(i)
			b.Write(segment.Value(source))
		}
	}
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		collectText(child, source, b)
	}
	if n.Type() == ast.TypeBlock {
		b.WriteByte('\n')
	}
}

// Slug returns the anchor of a heading with this text. It keeps letters of all languages, as “Übersicht” would otherwise
// lose its first letter. The prefix keeps headings from colliding with IDs of the application.
func Slug(heading string) string {
	var b strings.Builder
	b.WriteString("h-")
	dash := false
	for _, r := range strings.ToLower(heading) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if dash && b.Len() > 2 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		default:
			dash = true
		}
	}
	return b.String()
}

type headingIDs struct {
	seen map[string]int
}

func (h *headingIDs) Generate(value []byte, _ ast.NodeKind) []byte {
	slug := Slug(string(value))
	h.seen[slug]++
	if count := h.seen[slug]; count > 1 {
		return []byte(slug + "-" + strconv.Itoa(count))
	}
	return []byte(slug)
}

func (h *headingIDs) Put(value []byte) { h.seen[string(value)]++ }

var scheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

// linkScheme returns the lowercase scheme of a destination. Whatever has no scheme counts as a relative path, and those only
// work if they name a known file. So “java\tscript:”, which browsers would run, ends up as a broken reference.
func linkScheme(destination string) string {
	return strings.ToLower(strings.TrimSuffix(scheme.FindString(destination), ":"))
}
