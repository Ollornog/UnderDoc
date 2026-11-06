// SPDX-License-Identifier: MIT

package markdown

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Canonical classes of the design system. Tests check every one of them against the vendored stylesheet.
const (
	classH1         = "scroll-m-20 mb-4 text-4xl font-extrabold tracking-tight"
	classH2         = "scroll-m-20 border-b pb-2 text-3xl font-semibold tracking-tight first:mt-0"
	classH3         = "scroll-m-20 text-2xl font-semibold tracking-tight"
	classH4         = "scroll-m-20 text-xl font-semibold tracking-tight"
	classParagraph  = "leading-7"
	classList       = "my-2 ml-6 list-disc text-sm [&>li]:mt-1"
	classOrdered    = "my-2 ml-6 list-decimal text-sm [&>li]:mt-1"
	classQuote      = "text-muted-foreground mt-2 border-l-2 pl-6 italic"
	classCode       = "code-inline"
	classLink       = "font-medium underline underline-offset-4"
	classCodeBlock  = "code-block w-full"
	classTableBox   = "table-container my-4 w-full"
	classTable      = "table text-sm"
	classSeparator  = "separator"
	classBrokenItem = "badge"
)

// Classes lists the classes rendered pages use, for the check against the stylesheet.
func Classes() []string {
	all := strings.Join([]string{classH1, classH2, classH3, classH4, classParagraph, classList, classOrdered, classQuote,
		classCode, classLink, classCodeBlock, classTableBox, classTable, classSeparator, classBrokenItem}, " ")
	return strings.Fields(all)
}

// Target is what a link resolves to.
type Target struct {
	Href  string
	Title string
	// Document is the ID of the linked document, empty for files.
	Document string
}

// ErrAmbiguous marks a wiki link that fits several documents.
var ErrAmbiguous = errors.New("ambiguous")

// Resolver turns references in a page into URLs. It belongs to the page being rendered, since paths are relative to it.
type Resolver interface {
	// Wiki resolves the target of a [[wiki link]].
	Wiki(target string) (Target, error)
	// Link resolves a relative path of a normal link.
	Link(path string) (Target, bool)
	// Image resolves a relative path of an image.
	Image(path string) (Target, bool)
}

// Broken is a reference that could not be resolved. The page shows it, and the problem list names it.
type Broken struct {
	Reference string
	Reason    string
}

// Heading is one entry of the table of contents.
type Heading struct {
	Level  int
	Anchor string
	Text   string
}

// Rendered is the result of rendering a page.
type Rendered struct {
	HTML   string
	Links  []string
	Broken []Broken
	// Contents lists the headings of level 2 and 3.
	Contents []Heading
}

// Render resolves all references and renders the page. It changes the parsed page, so call it once.
func (p *Page) Render(resolver Resolver) (Rendered, error) {
	var result Rendered
	var nodes []ast.Node
	_ = ast.Walk(p.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			nodes = append(nodes, n)
		}
		return ast.WalkContinue, nil
	})
	for _, n := range nodes {
		switch v := n.(type) {
		case *WikiLink:
			result.resolveWiki(v, resolver)
		case *ast.Link:
			result.resolveLink(v, p.source, resolver)
		case *ast.Image:
			result.resolveImage(v, p.source, resolver)
		case *ast.Heading:
			v.SetAttributeString("class", headingClass(v.Level))
		case *ast.Paragraph:
			v.SetAttributeString("class", classParagraph)
		case *ast.List:
			if v.IsOrdered() {
				v.SetAttributeString("class", classOrdered)
			} else {
				v.SetAttributeString("class", classList)
			}
		case *ast.Blockquote:
			v.SetAttributeString("class", classQuote)
		case *ast.CodeSpan:
			v.SetAttributeString("class", classCode)
		case *ast.AutoLink:
			v.SetAttributeString("class", classLink)
		case *ast.ThematicBreak:
			v.SetAttributeString("class", classSeparator)
		case *extast.Table:
			v.SetAttributeString("class", classTable)
			box := &tableBox{}
			v.Parent().ReplaceChild(v.Parent(), v, box)
			box.AppendChild(box, v)
		}
	}
	for _, n := range nodes {
		if heading, ok := n.(*ast.Heading); ok && (heading.Level == 2 || heading.Level == 3) {
			if anchor, found := heading.AttributeString("id"); found {
				if id, isBytes := anchor.([]byte); isBytes {
					result.Contents = append(result.Contents, Heading{Level: heading.Level, Anchor: string(id), Text: nodeText(heading, p.source)})
				}
			}
		}
	}
	var out bytes.Buffer
	if err := engine.Renderer().Render(&out, p.source, p.root); err != nil {
		return Rendered{}, fmt.Errorf("render: %w", err)
	}
	result.HTML = out.String()
	return result, nil
}

func headingClass(level int) string {
	switch level {
	case 1:
		return classH1
	case 2:
		return classH2
	case 3:
		return classH3
	default:
		return classH4
	}
}

func (r *Rendered) fail(reference, reason string) {
	r.Broken = append(r.Broken, Broken{Reference: reference, Reason: reason})
}

func (r *Rendered) resolveWiki(link *WikiLink, resolver Resolver) {
	if link.Target == "" { // [[#Heading]] points into the same page
		link.href = "#" + Slug(link.Anchor)
		return
	}
	target, err := resolver.Wiki(link.Target)
	if err != nil {
		reason := "no such page"
		if errors.Is(err, ErrAmbiguous) {
			reason = err.Error()
		}
		link.broken = reason
		r.fail(link.source(), reason)
		return
	}
	link.href = target.Href
	if link.Anchor != "" {
		link.href += "#" + Slug(link.Anchor)
	}
	if link.Label == "" {
		link.Label = target.Title
	}
	r.Links = append(r.Links, target.Document)
}

func (r *Rendered) resolveLink(link *ast.Link, source []byte, resolver Resolver) {
	destination := string(link.Destination)
	link.SetAttributeString("class", classLink)
	if destination == "" || strings.HasPrefix(destination, "#") {
		return
	}
	switch linkScheme(destination) {
	case "http", "https", "mailto", "tel":
		return
	case "":
	default:
		replaceWithBroken(link, nodeText(link, source), "link type not allowed")
		r.fail(destination, "link type not allowed")
		return
	}
	if strings.HasPrefix(destination, "//") { // another host, without a scheme
		return
	}
	localPath, suffix := splitLocal(destination)
	target, ok := resolver.Link(localPath)
	if !ok {
		replaceWithBroken(link, nodeText(link, source), "no such page or file")
		r.fail(destination, "no such page or file")
		return
	}
	link.Destination = []byte(target.Href + suffix)
	if target.Document != "" {
		r.Links = append(r.Links, target.Document)
	}
}

func (r *Rendered) resolveImage(image *ast.Image, source []byte, resolver Resolver) {
	destination := string(image.Destination)
	alt := nodeText(image, source)
	switch linkScheme(destination) {
	case "http", "https":
		r.externalImage(image, destination, alt)
		return
	case "data": // the renderer lets only harmless image types through
		return
	case "":
	default:
		replaceWithBroken(image, alt, "image type not allowed")
		r.fail(destination, "image type not allowed")
		return
	}
	if strings.HasPrefix(destination, "//") {
		r.externalImage(image, destination, alt)
		return
	}
	localPath, _ := splitLocal(destination)
	target, ok := resolver.Image(localPath)
	if !ok {
		replaceWithBroken(image, alt, "no such image")
		r.fail(destination, "no such image")
		return
	}
	image.Destination = []byte(target.Href)
}

// externalImage turns an image from another host into a normal link. Pages never load anything from third parties,
// because every view would tell them who read what, and when.
func (r *Rendered) externalImage(image *ast.Image, destination, alt string) {
	if alt == "" {
		alt = destination
	}
	link := ast.NewLink()
	link.Destination = []byte(destination)
	link.SetAttributeString("class", classLink)
	link.AppendChild(link, ast.NewString([]byte(alt)))
	image.Parent().ReplaceChild(image.Parent(), image, link)
}

func splitLocal(destination string) (localPath, suffix string) {
	localPath = destination
	if i := strings.IndexAny(localPath, "?#"); i >= 0 {
		localPath, suffix = localPath[:i], localPath[i:]
	}
	if unescaped, err := url.PathUnescape(localPath); err == nil {
		localPath = unescaped
	}
	return localPath, suffix
}

func replaceWithBroken(n ast.Node, label, reason string) {
	n.Parent().ReplaceChild(n.Parent(), n, &brokenRef{text: label, reason: reason})
}

func nodeText(n ast.Node, source []byte) string {
	var b strings.Builder
	collectText(n, source, &b)
	return strings.Join(strings.Fields(b.String()), " ")
}

// WikiLink is [[target]], [[target|label]] or [[target#heading]].
type WikiLink struct {
	ast.BaseInline
	Target string
	Anchor string
	Label  string
	href   string
	broken string
}

var kindWikiLink = ast.NewNodeKind("WikiLink")

// Kind implements ast.Node.
func (n *WikiLink) Kind() ast.NodeKind { return kindWikiLink }

// Dump implements ast.Node.
func (n *WikiLink) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

func (n *WikiLink) label() string {
	if n.Label != "" {
		return n.Label
	}
	if n.Target != "" {
		return n.Target
	}
	return n.Anchor
}

func (n *WikiLink) source() string {
	if n.Anchor != "" {
		return "[[" + n.Target + "#" + n.Anchor + "]]"
	}
	return "[[" + n.Target + "]]"
}

type wikiParser struct{}

func (wikiParser) Trigger() []byte { return []byte{'['} }

func (wikiParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
	if !bytes.HasPrefix(line, []byte("[[")) {
		return nil
	}
	end := bytes.Index(line, []byte("]]"))
	if end < 3 {
		return nil
	}
	inner := string(line[2:end])
	if strings.ContainsAny(inner, "[\n") {
		return nil
	}
	block.Advance(end + 2)
	target, label, _ := strings.Cut(inner, "|")
	target = strings.TrimSuffix(target, `\`) // a pipe inside a table cell has to be escaped
	target, anchor, _ := strings.Cut(target, "#")
	return &WikiLink{Target: strings.TrimSpace(target), Anchor: strings.TrimSpace(anchor), Label: strings.TrimSpace(label)}
}

// brokenRef shows a reference that does not work, as a visible error instead of a silent gap.
type brokenRef struct {
	ast.BaseInline
	text   string
	reason string
}

var kindBrokenRef = ast.NewNodeKind("BrokenReference")

func (n *brokenRef) Kind() ast.NodeKind { return kindBrokenRef }

func (n *brokenRef) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

// tableBox wraps a table in the scrolling container of the design system.
type tableBox struct{ ast.BaseBlock }

var kindTableBox = ast.NewNodeKind("TableBox")

func (n *tableBox) Kind() ast.NodeKind { return kindTableBox }

func (n *tableBox) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

type nodeRenderer struct{}

func (nodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindWikiLink, renderWikiLink)
	reg.Register(kindBrokenRef, renderBrokenRef)
	reg.Register(kindTableBox, renderTableBox)
	reg.Register(ast.KindFencedCodeBlock, renderCodeBlock)
	reg.Register(ast.KindCodeBlock, renderCodeBlock)
}

func renderWikiLink(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	link := n.(*WikiLink)
	if link.broken != "" {
		writeBroken(w, link.label(), link.broken)
		return ast.WalkContinue, nil
	}
	_, _ = fmt.Fprintf(w, `<a class="%s" href="%s">%s</a>`, classLink, html.EscapeString(link.href), html.EscapeString(link.label()))
	return ast.WalkContinue, nil
}

func renderBrokenRef(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		ref := n.(*brokenRef)
		writeBroken(w, ref.text, ref.reason)
	}
	return ast.WalkContinue, nil
}

func writeBroken(w util.BufWriter, label, reason string) {
	_, _ = fmt.Fprintf(w, `<span class="%s" data-variant="destructive" title="%s">%s: %s</span>`, classBrokenItem,
		html.EscapeString(reason), html.EscapeString(reason), html.EscapeString(label))
}

func renderTableBox(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = fmt.Fprintf(w, `<div class="%s">`, classTableBox)
	} else {
		_, _ = w.WriteString("</div>\n")
	}
	return ast.WalkContinue, nil
}

func renderCodeBlock(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	_, _ = fmt.Fprintf(w, `<pre class="%s"><code>`, classCodeBlock)
	lines := n.Lines()
	for i := range lines.Len() {
		segment := lines.At(i)
		_, _ = w.WriteString(html.EscapeString(string(segment.Value(source))))
	}
	_, _ = w.WriteString("</code></pre>\n")
	return ast.WalkContinue, nil
}
