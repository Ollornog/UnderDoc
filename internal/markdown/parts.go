// SPDX-License-Identifier: MIT

package markdown

// Parts inside a paragraph (ADR-22): two point markers in the Markdown of a block, <!-- part tN --> and
// <!-- /part tN -->, give a few words of a paragraph an ID local to the block, addressed as <object>#<block>.<part>. The
// text stays where it is; the markers are anchored in the text, so typing around them moves nothing. Parts may overlap or
// nest, and a part may cut through formatting: the embedded part is cut out of the parsed paragraph, and formatting open
// at its edges is closed and opened again. A part never leaves its paragraph (or heading, list item, table cell).

import (
	"bytes"
	"errors"
	"html"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// PartID is the form of the ID of a part, local to its block.
var PartID = regexp.MustCompile(`^t[1-9][0-9]{0,8}$`)

// partMarker is one marker, as a whole raw HTML comment: the opening one may carry the name of a value (M-3).
var partMarker = regexp.MustCompile(`^<!-- (/?)part (t[1-9][0-9]{0,8})(?: name="([^"<>\n\\]{1,80})")? -->$`)

// anyPartMarker finds every marker in a text, also where it is no marker, such as in code.
var anyPartMarker = regexp.MustCompile(`<!-- /?part t[1-9][0-9]{0,8}(?: name="[^"<>\n\\]{1,80}")? -->`)

// partMarkerText finds markers in plain text, for the guard of paragraphs below.
var partMarkerText = regexp.MustCompile(`^[ \t]{0,3}<!-- /?part t[1-9]`)

// maxPartName is the longest a name of a value may be.
const maxPartName = 80

// Errors of marking a part.
var (
	ErrPartRange = errors.New("a part stays inside one paragraph, heading, list item or table cell: mark a block or a section for more")
	ErrPartEmpty = errors.New("there is no text to mark")
	ErrPartName  = errors.New("a name has at most 80 characters, without quotes, angle brackets or backslashes")
	ErrPartShape = errors.New("the marking would change how the paragraph reads")
	ErrNoPart    = errors.New("no such part")
)

// Part is one part of a block.
type Part struct {
	ID string
	// Name is set for a value (M-3), a part with a name.
	Name string
	// Open and Close are the byte offsets of the two markers in the Markdown of the block, each [start, end).
	Open, Close [2]int
}

// Text is the Markdown between the markers, as it stands in the block.
func (p Part) Text(src string) string { return src[p.Open[1]:p.Close[0]] }

// partGuard keeps a paragraph that starts with a part marker, or a line of one that does, a paragraph: CommonMark would
// take a line that starts with an HTML comment for a block of raw HTML. Every other line goes to the parser of HTML
// blocks as before.
type partGuard struct{ parser.BlockParser }

func (g partGuard) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	if line, _ := reader.PeekLine(); partMarkerText.Match(line) {
		return nil, parser.NoChildren
	}
	return g.BlockParser.Open(parent, reader, pc)
}

// blockParsers are the default block parsers of goldmark, with the parser of HTML blocks behind the guard.
func blockParsers() []util.PrioritizedValue {
	out := parser.DefaultBlockParsers()
	html := parser.NewHTMLBlockParser()
	for i, p := range out {
		if p.Value == html {
			out[i] = util.Prioritized(partGuard{html}, p.Priority)
		}
	}
	return out
}

// marker is one marker found in a parsed block.
type marker struct {
	closing  bool
	id, name string
	start    int // in the parsed source
	end      int
	node     *ast.RawHTML
	leaf     ast.Node
}

// parsed is a block of the database, parsed as RenderMarkdown parses it: with a newline in front, so that a block that
// starts with --- is not taken for front matter. Offsets in it are one more than in the block.
type parsed struct {
	source  []byte
	root    ast.Node
	markers []marker
}

func parseBlock(src string) *parsed {
	p := &parsed{source: []byte("\n" + src)}
	ctx := parser.NewContext(parser.WithIDs(&headingIDs{seen: map[string]int{}}))
	p.root = engine.Parser().Parse(text.NewReader(p.source), parser.WithContext(ctx))
	_ = ast.Walk(p.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		raw, ok := n.(*ast.RawHTML)
		if !entering || !ok || raw.Segments.Len() == 0 {
			return ast.WalkContinue, nil
		}
		start, end := raw.Segments.At(0).Start, raw.Segments.At(raw.Segments.Len()-1).Stop
		m := partMarker.FindStringSubmatch(string(p.source[start:end]))
		if m != nil {
			p.markers = append(p.markers, marker{closing: m[1] == "/", id: m[2], name: m[3], start: start, end: end, node: raw, leaf: leafOf(raw)})
		}
		return ast.WalkContinue, nil
	})
	return p
}

// leafOf returns the block that holds an inline node: a paragraph, a heading, the text of a list item or a table cell.
func leafOf(n ast.Node) ast.Node {
	p := n.Parent()
	for p != nil && p.Type() != ast.TypeBlock {
		p = p.Parent()
	}
	return p
}

// pairs finds the parts: an opening marker with the first closing marker of the same ID after it in the same leaf. A
// second opening marker of an ID already taken counts as nothing, as does a marker without its partner.
func (p *parsed) pairs() []Part {
	var out []Part
	taken := map[string]bool{}
	for i, open := range p.markers {
		if open.closing || taken[open.id] {
			continue
		}
		for _, closing := range p.markers[i+1:] {
			if closing.closing && closing.id == open.id && closing.leaf == open.leaf {
				taken[open.id] = true
				out = append(out, Part{ID: open.id, Name: open.name, Open: [2]int{open.start - 1, open.end - 1}, Close: [2]int{closing.start - 1, closing.end - 1}})
				break
			}
		}
	}
	return out
}

// Parts returns the parts of a block, in the order of their opening markers.
func Parts(src string) []Part {
	if !strings.Contains(src, "<!-- ") {
		return nil
	}
	return parseBlock(src).pairs()
}

// FindPart returns one part of a block.
func FindPart(src, id string) (Part, bool) {
	for _, p := range Parts(src) {
		if p.ID == id {
			return p, true
		}
	}
	return Part{}, false
}

// NextPartID returns an ID for a new part of a block: one more than the highest any marker in it names, and never
// below at.
func NextPartID(src string, at int) string {
	highest := max(at, 1) - 1
	for _, m := range regexp.MustCompile(`<!-- /?part t([1-9][0-9]{0,8})`).FindAllStringSubmatch(src, -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && n > highest {
			highest = n
		}
	}
	return "t" + strconv.Itoa(highest+1)
}

// StripParts takes every marker out of a block's Markdown, for a copy, which keeps no tie to anything.
func StripParts(src string) string {
	if !strings.Contains(src, "<!-- ") {
		return src
	}
	p := parseBlock(src)
	var out strings.Builder
	last := 1
	for _, m := range p.markers {
		out.Write(p.source[last:m.start])
		last = m.end
	}
	out.Write(p.source[last:])
	return out.String()
}

// PartMarkdown returns the Markdown of a part on its own, without markers: formatting open at its edges is closed and
// opened again, so `**very <!-- part t2 -->important** customers<!-- /part t2 -->` gives `**important** customers`. When
// that cannot be written so that it reads exactly as the part reads in its paragraph, the part comes as its plain text.
func PartMarkdown(src, id string) (string, error) {
	p := parseBlock(src)
	var open, closing *marker
	for _, part := range p.pairs() {
		if part.ID == id {
			for i := range p.markers {
				if p.markers[i].start == part.Open[0]+1 {
					open = &p.markers[i]
				}
				if p.markers[i].start == part.Close[0]+1 {
					closing = &p.markers[i]
				}
			}
		}
	}
	if open == nil || closing == nil {
		return "", ErrNoPart
	}
	want, ok := cutPart(renderPlain(p.source, p.root), id)
	if !ok {
		return "", ErrNoPart
	}
	if md, ok := p.excerpt(open, closing); ok && sameInline(renderInline(md), want) {
		return md, nil
	}
	return plainPart(want), nil
}

// excerpt writes the part with the formatting open at its edges closed and opened again.
func (p *parsed) excerpt(open, closing *marker) (string, bool) {
	var prefix, suffix []string
	for n := open.node.Parent(); n != open.leaf; n = n.Parent() {
		opener, _, ok := p.delimiters(n)
		if !ok {
			return "", false
		}
		prefix = append([]string{opener}, prefix...)
	}
	for n := closing.node.Parent(); n != closing.leaf; n = n.Parent() {
		_, closer, ok := p.delimiters(n)
		if !ok {
			return "", false
		}
		suffix = append(suffix, closer)
	}
	// The text between the markers, line by line as the leaf holds it: without the > of a quote or the indent of a list
	// item on the lines after the first, and without other markers.
	var body strings.Builder
	write := func(a, b int) {
		last := a
		for _, m := range p.markers {
			if m.start >= a && m.end <= b {
				body.Write(p.source[last:m.start])
				last = m.end
			}
		}
		body.Write(p.source[last:b])
	}
	leafLines := open.leaf.Lines()
	if leafLines.Len() == 0 {
		write(open.end, closing.start)
	}
	for i := range leafLines.Len() {
		seg := leafLines.At(i)
		if a, b := max(seg.Start, open.end), min(seg.Stop, closing.start); a < b {
			write(a, b)
		}
	}
	text := strings.TrimSpace(body.String())
	// A backslash at the end was a hard line break before the newline that the trim took.
	if n := len(text) - len(strings.TrimRight(text, `\`)); n%2 == 1 {
		text = strings.TrimSpace(text[:len(text)-1])
	}
	if text == "" {
		return "", false
	}
	// Each line may now start differently than in the paragraph: the first starts at the part, and a line the cut ends
	// early may be left as a lone # or -.
	lines := strings.Split(strings.Join(prefix, "")+text+strings.Join(suffix, ""), "\n")
	for i, line := range lines {
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		lines[i] = line[:indent] + escapeLineStart(line[indent:])
	}
	return strings.Join(lines, "\n"), true
}

// delimiters returns how an inline node of formatting opens and closes in Markdown.
func (p *parsed) delimiters(n ast.Node) (opener, closer string, ok bool) {
	switch v := n.(type) {
	case *ast.Emphasis:
		if v.Pos() < 0 || v.Pos() >= len(p.source) {
			return "", "", false
		}
		d := strings.Repeat(string(p.source[v.Pos()]), v.Level)
		return d, d, true
	case *extast.Strikethrough:
		if v.Pos() < 0 {
			return "", "", false
		}
		k := 0
		for i := v.Pos(); i < len(p.source) && p.source[i] == '~' && k < 2; i++ {
			k++
		}
		if k == 0 {
			return "", "", false
		}
		d := strings.Repeat("~", k)
		return d, d, true
	case *ast.Link:
		closer := "](" + linkDestination(string(v.Destination))
		if len(v.Title) > 0 {
			closer += ` "` + strings.ReplaceAll(strings.ReplaceAll(string(v.Title), `\`, `\\`), `"`, `\"`) + `"`
		}
		return "[", closer + ")", true
	}
	return "", "", false
}

func linkDestination(d string) string {
	if d == "" {
		return "<>"
	}
	if strings.ContainsAny(d, " ()<>\n\\") {
		r := strings.NewReplacer(`\`, `\\`, "<", `\<`, ">", `\>`, "\n", " ")
		return "<" + r.Replace(d) + ">"
	}
	return d
}

var (
	blockStart  = regexp.MustCompile(`^(?:[#>+=|]|-|\*(?:[ \t]|$)|[0-9]{1,9}[.)](?:[ \t]|$)|~~~|` + "```" + `)`)
	orderedLead = regexp.MustCompile(`^([0-9]{1,9})([.)])`)
)

// escapeLineStart keeps a part from turning into a heading, a list, a quote or a fence when it stands on a line of its
// own, as it does when it is embedded or copied: in its paragraph it started in the middle of a line.
func escapeLineStart(md string) string {
	if !blockStart.MatchString(md) {
		return md
	}
	if m := orderedLead.FindStringSubmatch(md); m != nil {
		return m[1] + `\` + md[len(m[1]):]
	}
	return `\` + md
}

// plainPart is the text of a part's HTML on one line, as Markdown.
func plainPart(rendered string) string {
	return escapeText(strings.Join(strings.Fields(htmlText(rendered)), " "))
}

// escapeText writes plain text as Markdown that reads as exactly that text.
func escapeText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 128 && strings.ContainsRune("\\`*_{}[]()#+-.!<>|~:&\"'=$", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// Rendering for the comparisons: goldmark's HTML, with the markers as empty spans.

func renderPlain(source []byte, root ast.Node) string {
	var out bytes.Buffer
	if err := engine.Renderer().Render(&out, source, root); err != nil {
		return ""
	}
	return out.String()
}

// renderInline renders Markdown that is one paragraph and returns what is inside the paragraph.
func renderInline(md string) string {
	p := parseBlock(md)
	if p.root.ChildCount() != 1 || p.root.FirstChild().Kind() != ast.KindParagraph {
		return "\x00not one paragraph"
	}
	out := strings.TrimSuffix(renderPlain(p.source, p.root), "\n")
	if !strings.HasPrefix(out, "<p>") || !strings.HasSuffix(out, "</p>") {
		return "\x00not one paragraph"
	}
	return out[3 : len(out)-4]
}

var (
	tagOrText     = regexp.MustCompile(`<[^>]*>`)
	leadingSpace  = regexp.MustCompile(`^((?:<[^/!][^>]*>)*)(?:\s|<br>)+`)
	trailingSpace = regexp.MustCompile(`(?:\s|<br>)+((?:</[^>]+>)*)$`)
)

// sameInline tells whether two pieces of inline HTML read the same, without the white space at their edges, which a part
// written on its own drops.
func sameInline(a, b string) bool {
	trim := func(s string) string {
		return trailingSpace.ReplaceAllString(leadingSpace.ReplaceAllString(s, "$1"), "$1")
	}
	return trim(a) == trim(b)
}

// htmlText is the text of a piece of HTML.
func htmlText(s string) string { return html.UnescapeString(tagOrText.ReplaceAllString(s, "")) }

// Sentinels: how a marker shows in rendered HTML. Raw HTML of a page is dropped, so nothing typed can look like them.
func openSentinel(id string) string  { return `<span data-part-open="` + id + `"></span>` }
func closeSentinel(id string) string { return `<span data-part-close="` + id + `"></span>` }

var (
	anySentinel = regexp.MustCompile(`<span data-part-(?:open|close)="t[0-9]+"></span>`)
	htmlTag     = regexp.MustCompile(`^<(/?)([a-zA-Z][a-zA-Z0-9]*)\b[^>]*?(/?)>`)
	blockTags   = map[string]bool{"p": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
		"li": true, "td": true, "th": true, "ul": true, "ol": true, "blockquote": true, "pre": true, "table": true,
		"thead": true, "tbody": true, "tr": true, "div": true, "figure": true, "figcaption": true, "section": true,
		"dl": true, "dt": true, "dd": true}
	voidTags = map[string]bool{"br": true, "img": true, "hr": true, "input": true, "wbr": true}
)

// StripPartSentinels takes the markers out of rendered HTML, for an embedding of a whole block.
func StripPartSentinels(s string) string {
	if !strings.Contains(s, "data-part-") {
		return s
	}
	return anySentinel.ReplaceAllString(s, "")
}

// CutPart cuts a part out of the rendered HTML of its block, with the inline elements open at its edges closed and opened
// again, so it shows exactly as in its paragraph, without the paragraph. It reports false if the part is not there.
func CutPart(rendered, id string) (string, bool) { return cutPart(rendered, id) }

type openTag struct {
	name, tag string
	serial    int
}

func cutPart(rendered, id string) (string, bool) {
	from := strings.Index(rendered, openSentinel(id))
	if from < 0 {
		return "", false
	}
	to := strings.Index(rendered[from:], closeSentinel(id))
	if to < 0 {
		return "", false
	}
	to += from
	var stack []openTag
	serial := 0
	var atOpen []openTag
	leaf := -1
	for i := 0; i < len(rendered); {
		if i == from {
			atOpen = slices.Clone(stack)
			leaf = innermostBlock(stack)
			if leaf < 0 {
				return "", false
			}
		}
		if i == to {
			break
		}
		if rendered[i] != '<' {
			i++
			continue
		}
		if s := anySentinel.FindStringIndex(rendered[i:]); s != nil && s[0] == 0 {
			i += s[1]
			continue
		}
		m := htmlTag.FindStringSubmatch(rendered[i:])
		if m == nil {
			i++
			continue
		}
		name := strings.ToLower(m[2])
		switch {
		case m[1] == "/":
			if k := lastOpen(stack, name); k >= 0 {
				stack = stack[:k]
			}
		case m[3] == "/" || voidTags[name]:
		default:
			serial++
			stack = append(stack, openTag{name: name, tag: m[0], serial: serial})
		}
		i += len(m[0])
	}
	// The paragraph open at the start must still be the one open at the end: a part never leaves it.
	if leaf < 0 || len(stack) <= leaf || stack[leaf].serial != atOpen[leaf].serial {
		return "", false
	}
	var out strings.Builder
	for _, t := range atOpen[leaf+1:] {
		out.WriteString(t.tag)
	}
	out.WriteString(anySentinel.ReplaceAllString(rendered[from+len(openSentinel(id)):to], ""))
	for k := len(stack) - 1; k > leaf; k-- {
		out.WriteString("</" + stack[k].name + ">")
	}
	return out.String(), true
}

func innermostBlock(stack []openTag) int {
	for k := len(stack) - 1; k >= 0; k-- {
		if blockTags[stack[k].name] {
			return k
		}
	}
	return -1
}

func lastOpen(stack []openTag, name string) int {
	for k := len(stack) - 1; k >= 0; k-- {
		if stack[k].name == name {
			return k
		}
	}
	return -1
}

// Marking a part.

// span is a stretch of the parsed source, [a, b).
type span struct{ a, b int }

// textSpans returns where text may be cut in a leaf: inside its text, never inside inline code, a formula, an image, an
// autolink, a wiki link, raw HTML or the address of a link. atoms are those elements, the edges a boundary snaps to.
func (p *parsed) textSpans(leaf ast.Node) (texts, atoms, tails []span) {
	var walk func(n ast.Node, atomic bool)
	walk = func(n ast.Node, atomic bool) {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch v := c.(type) {
			case *ast.Text:
				if !atomic {
					texts = append(texts, span{v.Segment.Start, v.Segment.Stop})
				}
			case *ast.String:
			case *ast.Emphasis, *extast.Strikethrough:
				walk(c, atomic)
			case *ast.Link:
				walk(c, atomic)
				if end := p.linkEnd(c); end > 0 && !atomic {
					if inner := p.lastTextEnd(c); inner > 0 {
						atoms = append(atoms, span{inner, end})
						tails = append(tails, span{inner, end})
					}
				}
			default:
				if a, b, ok := p.nodeSpan(c); ok && !atomic {
					atoms = append(atoms, span{a, b})
				}
				walk(c, true)
			}
		}
	}
	walk(leaf, false)
	return texts, atoms, tails
}

// nodeSpan returns where an atomic inline node lies in the source.
func (p *parsed) nodeSpan(n ast.Node) (int, int, bool) {
	switch v := n.(type) {
	case *ast.RawHTML:
		if v.Segments.Len() == 0 {
			return 0, 0, false
		}
		return v.Segments.At(0).Start, v.Segments.At(v.Segments.Len() - 1).Stop, true
	case *ast.CodeSpan:
		start := v.Pos()
		if start < 0 {
			return 0, 0, false
		}
		run := 0
		for i := start; i < len(p.source) && p.source[i] == '`'; i++ {
			run++
		}
		end := start + run
		for c := v.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				end = max(end, t.Segment.Stop)
			}
		}
		closer := bytes.Index(p.source[end:], bytes.Repeat([]byte("`"), run))
		if closer < 0 {
			return 0, 0, false
		}
		return start, end + closer + run, true
	case *ast.AutoLink:
		start := v.Pos()
		if start < 0 {
			return 0, 0, false
		}
		label := v.Label(p.source)
		if start < len(p.source) && p.source[start] == '<' {
			end := bytes.IndexByte(p.source[start:], '>')
			if end < 0 {
				return 0, 0, false
			}
			return start, start + end + 1, true
		}
		return start, start + len(label), true
	case *ast.Image:
		start := v.Pos()
		if start < 0 {
			return 0, 0, false
		}
		end := p.linkEnd(v)
		if end < 0 {
			return 0, 0, false
		}
		return start, end, true
	}
	start := n.Pos()
	if start < 0 {
		return 0, 0, false
	}
	// Wiki links, footnote references and anything else: up to the next sibling, or to where the next text starts.
	if next := n.NextSibling(); next != nil {
		if a := p.startOf(next); a > start {
			return start, a, true
		}
	}
	if s := bytes.IndexAny(p.source[start:], "]}"); s >= 0 {
		end := start + s + 1
		if end < len(p.source) && p.source[end] == ']' {
			end++
		}
		return start, end, true
	}
	return 0, 0, false
}

func (p *parsed) startOf(n ast.Node) int {
	if t, ok := n.(*ast.Text); ok {
		return t.Segment.Start
	}
	if r, ok := n.(*ast.RawHTML); ok && r.Segments.Len() > 0 {
		return r.Segments.At(0).Start
	}
	return n.Pos()
}

// lastTextEnd is where the last text inside a node ends.
func (p *parsed) lastTextEnd(n ast.Node) int {
	end := -1
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if t, ok := c.(*ast.Text); ok && entering {
			end = max(end, t.Segment.Stop)
		}
		if r, ok := c.(*ast.RawHTML); ok && entering && r.Segments.Len() > 0 {
			end = max(end, r.Segments.At(r.Segments.Len()-1).Stop)
		}
		return ast.WalkContinue, nil
	})
	return end
}

// linkEnd finds the end of a link or image in the source: after its text, `](destination "title")`, `][label]`, `[]` or
// nothing for a shortcut reference.
func (p *parsed) linkEnd(n ast.Node) int {
	from := p.lastTextEnd(n)
	if from < 0 {
		from = n.Pos() + 1
		if _, ok := n.(*ast.Image); ok {
			from++
		}
	}
	s := p.source
	i := bytes.IndexByte(s[from:], ']')
	if i < 0 {
		return -1
	}
	i += from + 1
	if i < len(s) && s[i] == '(' {
		depth, quote := 0, byte(0)
		for j := i + 1; j < len(s); j++ {
			c := s[j]
			switch {
			case c == '\\':
				j++
			case quote != 0:
				if c == quote {
					quote = 0
				}
			case c == '"' || c == '\'':
				quote = c
			case c == '<':
				if k := bytes.IndexByte(s[j:], '>'); k >= 0 {
					j += k
				}
			case c == '(':
				depth++
			case c == ')':
				if depth == 0 {
					return j + 1
				}
				depth--
			}
		}
		return -1
	}
	if i < len(s) && s[i] == '[' {
		if k := bytes.IndexByte(s[i:], ']'); k >= 0 {
			return i + k + 1
		}
	}
	return i
}

// leafAt returns the leaf block whose text holds the source offset, and the stretch of its content.
func (p *parsed) leafAt(offset int) (ast.Node, span, bool) {
	var found ast.Node
	var stretch span
	_ = ast.Walk(p.root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering || n.Type() != ast.TypeBlock || n.Lines().Len() == 0 {
			return ast.WalkContinue, nil
		}
		switch n.Kind() {
		case ast.KindParagraph, ast.KindTextBlock, ast.KindHeading, extast.KindTableCell:
		default:
			return ast.WalkContinue, nil
		}
		lines := n.Lines()
		a, b := lines.At(0).Start, lines.At(lines.Len()-1).Stop
		if n.Kind() == extast.KindTableCell {
			a, b = cellStretch(n, p)
		}
		if offset >= a && offset <= b {
			found, stretch = n, span{a, b}
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found, stretch, found != nil
}

// cellStretch is where the content of a table cell lies: goldmark gives a cell no lines of its own text, so it is the
// stretch of its inline nodes.
func cellStretch(cell ast.Node, p *parsed) (int, int) {
	texts, atoms, _ := p.textSpans(cell)
	a, b := -1, -1
	for _, s := range append(texts, atoms...) {
		if a < 0 || s.a < a {
			a = s.a
		}
		b = max(b, s.b)
	}
	return a, b
}

// MarkPart puts the markers of a new part round the Markdown between start and end, byte offsets into the block. The
// edges snap outwards to the edges of atomic elements, inline code, a formula, an image, an autolink, a wiki link, raw
// HTML or the address of a link, and the part stays in its paragraph. The markers go where the paragraph reads exactly as
// before. It returns the new Markdown and the part.
func MarkPart(src string, start, end int, id, name string) (string, Part, error) {
	if !PartID.MatchString(id) {
		return "", Part{}, ErrNoPart
	}
	if len(name) > maxPartName || strings.ContainsAny(name, "\"<>\n\\") {
		return "", Part{}, ErrPartName
	}
	if start < 0 || end > len(src) || start >= end {
		return "", Part{}, ErrPartEmpty
	}
	for start < end && strings.ContainsRune(" \t\r\n", rune(src[start])) {
		start++
	}
	for end > start && strings.ContainsRune(" \t\r\n", rune(src[end-1])) {
		end--
	}
	if start >= end {
		return "", Part{}, ErrPartEmpty
	}
	p := parseBlock(src)
	a, b := start+1, end+1
	leaf, stretch, ok := p.leafAt(a)
	if other, _, found := p.leafAt(b); !ok || !found || other != leaf {
		return "", Part{}, ErrPartRange
	}
	// Text inside an atomic element is no text to cut, so snapping takes the whole element in.
	texts, _, tails := p.textSpans(leaf)
	a, b = max(a, stretch.a), min(b, stretch.b)
	a, aOK := p.snap(a, texts, -1, stretch)
	b, bOK := p.snap(b, texts, 1, stretch)
	if !aOK || !bOK {
		return "", Part{}, ErrPartRange
	}
	if other, _, ok := p.leafAt(b); !ok || other != leaf {
		return "", Part{}, ErrPartRange
	}
	if strings.TrimSpace(string(p.source[a:b])) == "" {
		return "", Part{}, ErrPartEmpty
	}
	want := renderPlain(p.source, p.root)
	opening := "<!-- part " + id
	if name != "" {
		opening += ` name="` + name + `"`
	}
	opening += " -->"
	closing := "<!-- /part " + id + " -->"
	starts, ends := p.alternatives(a, texts, tails, stretch), p.alternatives(b, texts, tails, stretch)
	// The start goes after a run of delimiters and the end before one where it can, so no formatting opens or closes
	// round nothing at an edge of the part.
	slices.SortFunc(starts, func(x, y int) int { return y - x })
	slices.Sort(ends)
	for _, s := range starts {
		for _, e := range ends {
			if s >= e {
				continue
			}
			marked := string(p.source[1:s]) + opening + string(p.source[s:e]) + closing + string(p.source[e:])
			q := parseBlock(marked)
			got := renderPlain(q.source, q.root)
			if StripPartSentinels(got) != StripPartSentinels(want) {
				continue
			}
			if cut, ok := cutPart(got, id); !ok || strings.TrimSpace(htmlText(cut)) == "" {
				return "", Part{}, ErrPartEmpty
			}
			if part, ok := FindPart(marked, id); ok {
				return marked, part, nil
			}
		}
	}
	return "", Part{}, ErrPartShape
}

// snap moves an offset to the nearest place where text may be cut, backwards for the start (step -1) and forwards for the
// end, so that the part grows to take in what it would otherwise cut.
func (p *parsed) snap(at int, texts []span, step int, stretch span) (int, bool) {
	for x := at; x >= stretch.a && x <= stretch.b; x += step {
		if p.cuttable(x, texts) || x == stretch.a || x == stretch.b {
			return x, true
		}
	}
	return 0, false
}

// cuttable tells whether text may be cut at an offset: inside or at the edge of text, not inside a character, an escape
// or an entity.
func (p *parsed) cuttable(x int, texts []span) bool {
	inText := false
	for _, t := range texts {
		if x >= t.a && x <= t.b {
			inText = true
			break
		}
	}
	if !inText {
		return false
	}
	if x < len(p.source) && !utf8.RuneStart(p.source[x]) {
		return false
	}
	if x > 0 && x < len(p.source) && p.source[x-1] == '\\' && isPunct(p.source[x]) {
		run := 0
		for k := x - 1; k >= 0 && p.source[k] == '\\'; k-- {
			run++
		}
		if run%2 == 1 {
			return false
		}
	}
	if amp := bytes.LastIndexByte(p.source[max(0, x-32):x], '&'); amp >= 0 {
		amp += max(0, x-32)
		if entity.Match(p.source[amp:]) && amp+len(entity.Find(p.source[amp:])) > x {
			return false
		}
	}
	return true
}

var entity = regexp.MustCompile(`^&(?:#[0-9]{1,7}|#[xX][0-9a-fA-F]{1,6}|[a-zA-Z][a-zA-Z0-9]{1,31});`)

func isPunct(c byte) bool {
	return c < 128 && strings.IndexByte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", c) >= 0
}

// alternatives are the places a marker may go for a boundary at x: x itself, and the places where the same text starts
// or ends across runs of formatting delimiters next to it (**, _, ~~, the [ of a link) and, going on, the rest of a link
// after its text.
func (p *parsed) alternatives(x int, texts, tails []span, stretch span) []int {
	out := []int{x}
	add := func(y int) {
		if p.cuttable(y, texts) && !slices.Contains(out, y) {
			out = append(out, y)
		}
	}
	for y := x; y < stretch.b; {
		if k := slices.IndexFunc(tails, func(t span) bool { return y >= t.a && y < t.b }); k >= 0 && p.source[y] == ']' {
			y = tails[k].b
		} else if strings.IndexByte("*_~", p.source[y]) >= 0 {
			y++
		} else {
			break
		}
		add(y)
	}
	for y := x; y > stretch.a && strings.IndexByte("*_~[", p.source[y-1]) >= 0; {
		y--
		add(y)
	}
	return out
}

// ByteOffset turns an offset in UTF-16 code units, as the browser counts, into a byte offset of s. Past the end it is the
// end.
func ByteOffset(s string, units int) int {
	n := 0
	for i, r := range s {
		if n >= units {
			return i
		}
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return len(s)
}

// UnitOffset turns a byte offset of s into UTF-16 code units, as the browser counts.
func UnitOffset(s string, bytesAt int) int {
	n := 0
	for i, r := range s {
		if i >= bytesAt {
			break
		}
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// Sections (ADR-22): a heading and everything up to the next heading of the same or a higher level. In the database a
// section starts with a block whose Markdown opens with a heading and takes the blocks after it up to a block that opens
// with such a heading.

// OpeningHeading returns the level and the text of the heading a block opens with, or 0 if it opens with none.
func OpeningHeading(src string) (int, string) {
	if src == "" {
		return 0, ""
	}
	p := parseBlock(src)
	h, ok := p.root.FirstChild().(*ast.Heading)
	if !ok {
		return 0, ""
	}
	var b strings.Builder
	collectText(h, p.source, &b)
	return h.Level, strings.Join(strings.Fields(b.String()), " ")
}

// SectionEnd returns how many blocks a section takes, starting at the first of bodies, which must open with a heading;
// 0 if it does not.
func SectionEnd(bodies []string) int {
	if len(bodies) == 0 {
		return 0
	}
	level, _ := OpeningHeading(bodies[0])
	if level == 0 {
		return 0
	}
	n := 1
	for ; n < len(bodies); n++ {
		if l, _ := OpeningHeading(bodies[n]); l > 0 && l <= level {
			break
		}
	}
	return n
}
