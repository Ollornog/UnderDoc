// SPDX-License-Identifier: MIT

package transfer

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/markdown"
)

// MaxPageBytes is the largest Markdown file that is read, as in the folder source.
const MaxPageBytes = 8 << 20

// maxBlockNumber keeps block IDs within the form the database accepts, b1 to b999999999.
const maxBlockNumber = 999999999

var (
	markerLine  = regexp.MustCompile(`^ {0,3}<!-- block (b[0-9]+) -->[ \t]*$`)
	headingLine = regexp.MustCompile(`^ {0,3}#{1,3}([ \t]|$)`)
	titleLine   = regexp.MustCompile(`^ {0,3}#[ \t]+(.*?)[ \t#]*$`)
	validBlock  = regexp.MustCompile(`^b[1-9][0-9]{0,8}$`)
)

// chunk is the text of one block before it is checked, with the ID of its marker if it had one.
type chunk struct {
	id    string
	lines []string
}

// page is a Markdown file as the import reads it.
type page struct {
	Meta     pageMeta
	Title    string
	Blocks   []db.Block
	Next     int
	Warnings []string
}

// normalize accepts what editors write: a byte order mark, CRLF line ends and broken encodings.
func normalize(content []byte) []byte {
	content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))
	content = bytes.ToValidUTF8(content, []byte("�"))
	return bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
}

// fences follows fenced code line by line, so that a marker or heading inside code stays text.
type fences struct {
	char  byte
	count int
}

// inside reports whether a line is inside fenced code, the opening and closing lines included.
func (f *fences) inside(line string) bool {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return f.count > 0
	}
	run := 0
	if trimmed != "" && (trimmed[0] == '`' || trimmed[0] == '~') {
		for run < len(trimmed) && trimmed[run] == trimmed[0] {
			run++
		}
	}
	switch {
	case f.count == 0 && run >= 3:
		f.char, f.count = trimmed[0], run
		return true
	case f.count > 0 && run >= f.count && trimmed[0] == f.char && strings.TrimSpace(trimmed[run:]) == "":
		f.count = 0
		return true
	}
	return f.count > 0
}

// parsePage reads one Markdown file. Blocks come from the markers the export writes; a file without markers is split
// before each heading of level 1 to 3 outside fenced code, and a heading of level 1 that opens it and only repeats the
// title is left out. Block IDs from markers are kept where valid and not used before in the file; every other block gets
// the next free number after the highest, and never before the next_block in the front matter.
func parsePage(content []byte, fileName string) page {
	content = normalize(content)
	var p page
	yamlPart, body, found := markdown.SplitFrontMatter(content)
	if found {
		if err := yaml.Unmarshal(yamlPart, &p.Meta); err != nil {
			p.Warnings = append(p.Warnings, "front matter is not valid YAML, it was left out")
			p.Meta = pageMeta{}
		}
	}
	p.Title = cleanTitle(p.Meta.Title)
	if p.Title == "" {
		p.Title = cleanTitle(markdown.Parse(body).Heading)
	}
	if p.Title == "" {
		p.Title = cleanTitle(strings.TrimSuffix(fileName, path.Ext(fileName)))
	}
	if p.Title == "" {
		p.Title = "Untitled"
	}

	chunks := []chunk{{}}
	marked := false
	lines := strings.Split(string(body), "\n")
	f := &fences{}
	for _, line := range lines {
		if f.inside(line) {
			chunks[len(chunks)-1].lines = append(chunks[len(chunks)-1].lines, line)
			continue
		}
		if m := markerLine.FindStringSubmatch(line); m != nil {
			chunks = append(chunks, chunk{id: m[1]})
			marked = true
			continue
		}
		chunks[len(chunks)-1].lines = append(chunks[len(chunks)-1].lines, line)
	}
	if !marked {
		chunks = splitHeadings(dropTitle(lines, p.Title))
	}
	for _, c := range chunks {
		text := trimBody(strings.Join(c.lines, "\n"))
		if text == "" && c.id == "" {
			continue
		}
		for j, piece := range fit(text, db.MaxBlock) {
			b := db.Block{Body: piece}
			if j == 0 {
				b.ID = c.id
			}
			p.Blocks = append(p.Blocks, b)
		}
	}
	p.numberBlocks()
	return p
}

// numberBlocks keeps the IDs that are valid and not taken earlier in the page, and numbers the rest after them.
func (p *page) numberBlocks() {
	used := map[string]bool{}
	highest := 0
	for i, b := range p.Blocks {
		if b.ID == "" {
			continue
		}
		if !validBlock.MatchString(b.ID) || used[b.ID] {
			p.Warnings = append(p.Warnings, fmt.Sprintf("block %s is not valid or used twice, it got a new ID", b.ID))
			p.Blocks[i].ID = ""
			continue
		}
		used[b.ID] = true
		number, _ := strconv.Atoi(b.ID[1:])
		highest = max(highest, number)
	}
	p.Next = highest + 1
	if p.Meta.NextBlock > p.Next && p.Meta.NextBlock <= maxBlockNumber {
		p.Next = p.Meta.NextBlock
	}
	for i := range p.Blocks {
		if p.Blocks[i].ID == "" {
			p.Blocks[i].ID = "b" + strconv.Itoa(p.Next)
			p.Next++
		}
	}
}

// nextAfter is the number after the highest block ID.
func nextAfter(blocks []db.Block) int {
	highest := 0
	for _, b := range blocks {
		if number, err := strconv.Atoi(strings.TrimPrefix(b.ID, "b")); err == nil {
			highest = max(highest, number)
		}
	}
	return highest + 1
}

// dropTitle leaves out a first heading of level 1 that only repeats the title, which the page shows anyway.
func dropTitle(lines []string, title string) []string {
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if m := titleLine.FindStringSubmatch(line); m != nil && cleanTitle(m[1]) == title {
			return lines[i+1:]
		}
		break
	}
	return lines
}

// splitHeadings cuts lines before each heading of level 1 to 3 outside fenced code.
func splitHeadings(lines []string) []chunk {
	out := []chunk{{}}
	f := &fences{}
	for _, line := range lines {
		if !f.inside(line) && headingLine.MatchString(line) {
			out = append(out, chunk{})
		}
		out[len(out)-1].lines = append(out[len(out)-1].lines, line)
	}
	return out
}

// fit cuts a text that is longer than limit characters into pieces that are not: at blank lines where it can, else at
// line ends, else anywhere.
func fit(text string, limit int) []string {
	if len([]rune(text)) <= limit {
		return []string{text}
	}
	var out []string
	var current []string
	size := 0
	flush := func() {
		if len(current) > 0 {
			out = append(out, strings.Join(current, "\n\n"))
			current, size = nil, 0
		}
	}
	for _, part := range strings.Split(text, "\n\n") {
		n := len([]rune(part))
		if n > limit {
			flush()
			out = append(out, fitLines(part, limit)...)
			continue
		}
		if size > 0 && size+2+n > limit {
			flush()
		}
		if size > 0 {
			size += 2
		}
		current = append(current, part)
		size += n
	}
	flush()
	return out
}

func fitLines(text string, limit int) []string {
	var out []string
	var b strings.Builder
	size := 0
	for _, line := range strings.Split(text, "\n") {
		r := []rune(line)
		for len(r) > limit {
			if size > 0 {
				out, size = append(out, b.String()), 0
				b.Reset()
			}
			out, r = append(out, string(r[:limit])), r[limit:]
		}
		if size > 0 && size+1+len(r) > limit {
			out, size = append(out, b.String()), 0
			b.Reset()
		}
		if size > 0 {
			b.WriteByte('\n')
			size++
		}
		b.WriteString(string(r))
		size += len(r)
	}
	if size > 0 || len(out) == 0 {
		out = append(out, b.String())
	}
	return out
}

// cleanTitle makes a title the database takes: one line, at most 200 characters.
func cleanTitle(title string) string {
	r := []rune(strings.Join(strings.Fields(title), " "))
	if len(r) > 200 {
		r = r[:200]
	}
	return strings.TrimSpace(string(r))
}

func cleanAccess(access string) (string, bool) {
	switch access {
	case "", "edit", "comment", "read":
		return access, true
	}
	return "", false
}

type reader struct {
	fsys     fs.FS
	warnings []string
	// pages are the pages read, by path, so links between them can point at the pages after the import.
	pages []readPage
}

type readPage struct {
	file string
	node *db.Node
}

func (r *reader) warn(name, message string) {
	r.warnings = append(r.warnings, name+": "+message)
}

// Read reads a tree of files into a node for db.Import: an export of UnderDoc, or any folder of Markdown files like the
// folder source. The root directory becomes the collection; directories become folders, or collections if their MetaFile
// says so; a directory beside a page with the same name holds the children of that page. Entries starting with a dot
// and anything that is not a regular file or directory are left out, other files are named in the warnings.
func Read(fsys fs.FS) (*db.Node, []string, error) {
	r := &reader{fsys: fsys}
	root, err := r.dir(".", "")
	if err != nil {
		return nil, nil, err
	}
	root.Kind = db.KindCollection
	r.relink()
	return root, r.warnings, nil
}

var (
	fileLink = regexp.MustCompile(`\]\(<?([^()\s<>]+?\.md|[^()<>]+?\.md)>?(#[^)\s]*)?\)`)
	wikiLink = regexp.MustCompile(`\[\[([^\]|#]*)(#[^\]|]*)?(?:\|([^\]]*))?\]\]`)
)

// relink turns the links between the pages read into links to the objects they become: a relative link to a Markdown file
// and a link [[page]] by ID, path, file name or title. Each page gets a key, which db.Import replaces with its new ID. A
// link that finds no page, and anything in fenced code, stays as written; the list of problems names what stays broken.
func (r *reader) relink() {
	byPath, byID, byName := map[string]*readPage{}, map[string]*readPage{}, map[string][]*readPage{}
	for i := range r.pages {
		p := &r.pages[i]
		p.node.Key = fmt.Sprintf("import-%d", i+1)
		byPath[strings.ToLower(p.file)] = p
		if p.node.ID != "" {
			byID[p.node.ID] = p
		}
		base := strings.ToLower(strings.TrimSuffix(path.Base(p.file), path.Ext(p.file)))
		byName[base] = append(byName[base], p)
		if title := strings.ToLower(p.node.Title); title != base {
			byName[title] = append(byName[title], p)
		}
	}
	find := func(from, target string) *readPage {
		target = strings.TrimSpace(target)
		if p, ok := byID[target]; ok {
			return p
		}
		for _, candidate := range []string{path.Join(path.Dir(from), target), path.Clean(strings.TrimPrefix(target, "/"))} {
			for _, ext := range []string{"", ".md"} {
				if p, ok := byPath[strings.ToLower(candidate+ext)]; ok {
					return p
				}
			}
		}
		if ps := byName[strings.ToLower(target)]; len(ps) == 1 {
			return ps[0]
		}
		return nil
	}
	for _, p := range r.pages {
		for i, b := range p.node.Blocks {
			var f fences
			lines := strings.Split(b.Body, "\n")
			for k, line := range lines {
				if f.inside(line) {
					continue
				}
				line = fileLink.ReplaceAllStringFunc(line, func(m string) string {
					parts := fileLink.FindStringSubmatch(m)
					target, err := url.PathUnescape(parts[1])
					if err != nil || strings.Contains(target, "://") {
						return m
					}
					if to := find(p.file, target); to != nil {
						return "](/o/" + to.node.Key + parts[2] + ")"
					}
					return m
				})
				line = wikiLink.ReplaceAllStringFunc(line, func(m string) string {
					parts := wikiLink.FindStringSubmatch(m)
					if parts[1] == "" {
						return m
					}
					to := find(p.file, parts[1])
					if to == nil {
						return m
					}
					label := strings.TrimSpace(parts[3])
					if label == "" {
						label = to.node.Title
					}
					anchor := ""
					if parts[2] != "" {
						anchor = "#" + markdown.Slug(parts[2][1:])
					}
					return "[" + label + "](/o/" + to.node.Key + anchor + ")"
				})
				lines[k] = line
			}
			p.node.Blocks[i].Body = strings.Join(lines, "\n")
		}
	}
}

func (r *reader) meta(dir string) dirMeta {
	var m dirMeta
	data, err := fs.ReadFile(r.fsys, path.Join(dir, MetaFile))
	if err != nil {
		return m
	}
	if err := yaml.Unmarshal(data, &m); err != nil {
		r.warn(path.Join(dir, MetaFile), "not valid YAML, it was left out")
		return dirMeta{}
	}
	return m
}

func (r *reader) dir(dir, name string) (*db.Node, error) {
	m := r.meta(dir)
	n := &db.Node{Object: db.Object{ID: m.ID, Kind: db.KindFolder, Title: cleanTitle(m.Title), Description: strings.TrimSpace(m.Description), Private: m.Private}}
	if m.Kind == db.KindCollection {
		n.Kind = db.KindCollection
	}
	if n.Title == "" {
		n.Title = cleanTitle(name)
	}
	if n.Title == "" {
		n.Title = "Untitled"
	}
	if access, ok := cleanAccess(m.Access); ok {
		n.Access = access
	} else {
		r.warn(path.Join(dir, MetaFile), fmt.Sprintf("access %q is not edit, comment or read, it was left out", m.Access))
	}
	kids, err := r.children(dir, m.Order)
	n.Kids = kids
	return n, err
}

func (r *reader) children(dir string, order []entry) ([]*db.Node, error) {
	entries, err := fs.ReadDir(r.fsys, dir)
	if err != nil {
		return nil, err
	}
	dirs := map[string]fs.DirEntry{}
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs[strings.ToLower(e.Name())] = e
		}
	}
	byName := map[string]*db.Node{}
	var names []string
	for _, e := range entries {
		name := e.Name()
		full := path.Join(dir, name)
		switch {
		case strings.HasPrefix(name, "."):
			continue
		case e.Type().IsRegular() && isMarkdown(name):
			n, err := r.page(full, name)
			if err != nil {
				return nil, err
			}
			stem := strings.ToLower(strings.TrimSuffix(name, path.Ext(name)))
			if d, ok := dirs[stem]; ok && r.meta(path.Join(dir, d.Name())).Kind == "" {
				delete(dirs, stem)
				sub := path.Join(dir, d.Name())
				if n.Kids, err = r.children(sub, r.meta(sub).Order); err != nil {
					return nil, err
				}
			}
			byName[name] = n
			names = append(names, name)
		case e.IsDir(), e.Type().IsRegular():
		default:
			r.warn(full, "left out, only regular files and directories are read")
		}
	}
	for _, e := range entries {
		name := e.Name()
		switch {
		case strings.HasPrefix(name, "."):
		case e.IsDir():
			if _, ok := dirs[strings.ToLower(name)]; !ok {
				continue // holds the children of a page
			}
			n, err := r.dir(path.Join(dir, name), name)
			if err != nil {
				return nil, err
			}
			byName[name] = n
			names = append(names, name)
		case e.Type().IsRegular() && !isMarkdown(name):
			r.warn(path.Join(dir, name), "left out, only Markdown files are imported for now")
		}
	}
	var out []*db.Node
	taken := map[string]bool{}
	for _, e := range order {
		switch {
		case e.Line != "":
			out = append(out, &db.Node{Object: db.Object{ID: e.Line, Kind: db.KindLine, Title: db.KindLine}})
		case e.Space != "":
			out = append(out, &db.Node{Object: db.Object{ID: e.Space, Kind: db.KindSpace, Title: db.KindSpace}})
		case byName[e.Name] != nil && !taken[e.Name]:
			out = append(out, byName[e.Name])
			taken[e.Name] = true
		}
	}
	slices.SortFunc(names, func(a, b string) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a), strings.ToLower(b)), cmp.Compare(a, b))
	})
	for _, name := range names {
		if !taken[name] {
			out = append(out, byName[name])
		}
	}
	return out, nil
}

func (r *reader) page(file, name string) (*db.Node, error) {
	f, err := r.fsys.Open(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	content, err := io.ReadAll(io.LimitReader(f, MaxPageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(content) > MaxPageBytes {
		r.warn(file, fmt.Sprintf("larger than %d MiB, only the start was read", MaxPageBytes>>20))
		content = content[:MaxPageBytes]
	}
	p := parsePage(content, name)
	for _, w := range p.Warnings {
		r.warn(file, w)
	}
	n := &db.Node{Object: db.Object{ID: p.Meta.ID, Kind: db.KindPage, Title: p.Title, Description: strings.TrimSpace(p.Meta.Description)}, Blocks: p.Blocks, NextBlock: p.Next}
	r.pages = append(r.pages, readPage{file: file, node: n})
	if access, ok := cleanAccess(p.Meta.Access); ok {
		n.Access = access
	} else {
		r.warn(file, fmt.Sprintf("access %q is not edit, comment or read, it was left out", p.Meta.Access))
	}
	return n, nil
}

func isMarkdown(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".md", ".markdown":
		return true
	}
	return false
}
