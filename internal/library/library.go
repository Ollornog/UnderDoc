// SPDX-License-Identifier: MIT

// Package library turns the pages of a source into an immutable snapshot: documents, the separate folder tree, links in
// both directions, the search index and a list of everything that is broken.
package library

import (
	"cmp"
	"errors"
	"fmt"
	"html"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Ollornog/UnderDoc/internal/document"
	"github.com/Ollornog/UnderDoc/internal/markdown"
	"github.com/Ollornog/UnderDoc/internal/route"
	"github.com/Ollornog/UnderDoc/internal/tree"
)

// Page is one page as a source delivers it.
type Page struct {
	// Source is the slash-separated path inside the source.
	Source   string
	Content  []byte
	Modified time.Time
}

// Input is everything a source delivers.
type Input struct {
	Pages []Page
	// Images are the paths of the images next to the pages.
	Images []string
	// Problems are what the source itself ran into.
	Problems []Problem
}

// Problem is something a person should fix. The page keeps working as far as it can.
type Problem struct {
	Source  string
	Message string
}

// Library is one complete snapshot. It never changes, so requests can read it without locks.
type Library struct {
	documents map[document.ID]*document.Document
	tree      *tree.Tree
	images    map[string]bool
	problems  []Problem
	index     []indexEntry
	byPath    map[string]*document.Document
	byName    map[string][]*document.Document
}

// Build creates a snapshot from the input of a source.
func Build(in Input) *Library {
	lib := &Library{
		documents: map[document.ID]*document.Document{},
		tree:      tree.New(),
		images:    map[string]bool{},
		byPath:    map[string]*document.Document{},
		byName:    map[string][]*document.Document{},
		problems:  slices.Clone(in.Problems),
	}
	for _, image := range in.Images {
		lib.images[image] = true
	}
	pages := slices.SortedFunc(slices.Values(in.Pages), func(a, b Page) int { return cmp.Compare(a.Source, b.Source) })
	parsed := make(map[document.ID]*markdown.Page, len(pages))
	for _, page := range pages {
		doc, parsedPage := lib.identify(page)
		lib.documents[doc.ID] = doc
		parsed[doc.ID] = parsedPage
		lib.tree.Place(doc.ID, path.Dir(page.Source))
		lib.byPath[pathKey(page.Source)] = doc
		lib.addName(doc.Title, doc)
		lib.addName(stem(page.Source), doc)
	}
	lib.tree.Sort(func(id document.ID) string { return lib.documents[id].Title })
	for _, page := range pages {
		lib.render(lib.byPath[pathKey(page.Source)], parsed)
	}
	lib.linkBack()
	lib.buildIndex()
	slices.SortStableFunc(lib.problems, func(a, b Problem) int { return cmp.Compare(a.Source, b.Source) })
	return lib
}

func (lib *Library) identify(page Page) (*document.Document, *markdown.Page) {
	parsed := parse(page.Content)
	doc := &document.Document{Source: page.Source, Modified: page.Modified, Title: stem(page.Source)}
	if parsed.FrontError != nil {
		lib.report(page.Source, "front matter is not valid YAML: "+firstLine(parsed.FrontError.Error()))
	}
	if parsed.Heading != "" {
		doc.Title = parsed.Heading
	}
	if parsed.Front.Title != "" {
		doc.Title = parsed.Front.Title
	}
	doc.StartsWithTitle = parsed.StartsWithHeading && doc.Title == parsed.Heading
	doc.Description = strings.TrimSpace(parsed.Front.Description)
	doc.Tags = cleanTags(parsed.Front.Tags)
	doc.ID = document.PathID(page.Source)
	switch id := parsed.Front.ID; {
	case id == "":
	case !document.ValidStableID(id):
		lib.report(page.Source, fmt.Sprintf("id %q is not usable: letters, digits, dot, dash and underscore only", id))
	default:
		if other, taken := lib.documents[document.ID(id)]; taken {
			lib.report(page.Source, fmt.Sprintf("id %q is already used by %s, so this page got a path as its ID", id, other.Source))
			break
		}
		doc.ID = document.ID(id)
		doc.Stable = true
	}
	return doc, parsed
}

// Tagged returns the documents that carry a tag, sorted by title.
func (lib *Library) Tagged(tag string) []*document.Document {
	var out []*document.Document
	for _, doc := range lib.documents {
		if slices.Contains(doc.Tags, tag) {
			out = append(out, doc)
		}
	}
	slices.SortFunc(out, func(a, b *document.Document) int {
		return cmp.Or(cmp.Compare(a.Title, b.Title), cmp.Compare(a.ID, b.ID))
	})
	return out
}

// cleanTags makes tags comparable: lower case, without a leading #, each once.
func cleanTags(tags []string) []string {
	var out []string
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(tag), "#"))
		if tag != "" && !slices.Contains(out, tag) {
			out = append(out, tag)
		}
	}
	return out
}

// parse guards against a page that crashes the parser: one bad file must not stop the whole wiki.
func parse(content []byte) (page *markdown.Page) {
	defer func() {
		if recover() != nil {
			page = markdown.Parse([]byte("The parser failed on this page."))
			page.FrontError = errors.New("the parser failed on this page")
		}
	}()
	return markdown.Parse(content)
}

func (lib *Library) render(doc *document.Document, parsed map[document.ID]*markdown.Page) {
	page := parsed[doc.ID]
	doc.Text = page.Text()
	result, err := safeRender(page, &resolver{lib: lib, doc: doc})
	if err != nil {
		lib.report(doc.Source, "could not be rendered: "+err.Error())
		doc.HTML = "<pre>" + html.EscapeString(doc.Text) + "</pre>"
		return
	}
	doc.HTML = result.HTML
	for _, heading := range result.Contents {
		doc.Contents = append(doc.Contents, document.Heading(heading))
	}
	for _, broken := range result.Broken {
		lib.report(doc.Source, broken.Reference+": "+broken.Reason)
	}
	for _, link := range result.Links {
		if id := document.ID(link); id != doc.ID && !slices.Contains(doc.Links, id) {
			doc.Links = append(doc.Links, id)
		}
	}
	slices.Sort(doc.Links)
}

// RenderMarkdown renders Markdown that does not live in the folder source, such as a block of a page in the database
// (ADR-19). Links resolve as from a page at the root of the folder source. A leading line of dashes stays a rule: a block
// has no front matter.
func (lib *Library) RenderMarkdown(source string) (markdown.Rendered, error) {
	return safeRender(markdown.Parse([]byte("\n"+source)), &resolver{lib: lib, doc: &document.Document{Source: "block.md"}, database: true})
}

func safeRender(page *markdown.Page, res markdown.Resolver) (result markdown.Rendered, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	return page.Render(res)
}

func (lib *Library) linkBack() {
	for _, doc := range lib.documents {
		for _, target := range doc.Links {
			other := lib.documents[target]
			other.Backlinks = append(other.Backlinks, doc.ID)
		}
	}
	for _, doc := range lib.documents {
		slices.SortFunc(doc.Backlinks, func(a, b document.ID) int {
			return cmp.Or(cmp.Compare(strings.ToLower(lib.documents[a].Title), strings.ToLower(lib.documents[b].Title)), cmp.Compare(a, b))
		})
	}
}

func (lib *Library) report(source, message string) {
	lib.problems = append(lib.problems, Problem{Source: source, Message: message})
}

func (lib *Library) addName(name string, doc *document.Document) {
	key := strings.ToLower(name)
	if !slices.Contains(lib.byName[key], doc) {
		lib.byName[key] = append(lib.byName[key], doc)
	}
}

// Document returns a document by its ID.
func (lib *Library) Document(id document.ID) (*document.Document, bool) {
	doc, ok := lib.documents[id]
	return doc, ok
}

// Tree returns the folder tree.
func (lib *Library) Tree() *tree.Tree { return lib.tree }

// HasImage reports whether a path is a known image.
func (lib *Library) HasImage(imagePath string) bool { return lib.images[imagePath] }

// Problems lists everything broken, ordered by file.
func (lib *Library) Problems() []Problem { return lib.problems }

// Len returns the number of documents.
func (lib *Library) Len() int { return len(lib.documents) }

// Recent returns the most recently changed documents.
func (lib *Library) Recent(limit int) []*document.Document {
	docs := slices.SortedFunc(maps.Values(lib.documents), func(a, b *document.Document) int {
		return cmp.Or(b.Modified.Compare(a.Modified), cmp.Compare(a.Source, b.Source))
	})
	return docs[:min(limit, len(docs))]
}

// Home returns the start page: index.md or README.md in the root folder.
func (lib *Library) Home() (*document.Document, bool) {
	for _, name := range []string{"index", "readme"} {
		for _, id := range lib.tree.Root().Documents {
			if doc := lib.documents[id]; strings.EqualFold(stem(doc.Source), name) {
				return doc, true
			}
		}
	}
	return nil, false
}

type resolver struct {
	lib *Library
	doc *document.Document
	// database is set for blocks of pages in the database: a link to /o/<id> may name an object of the database, which
	// the folder source does not know; it stays as written, and the list of problems checks it against the database.
	database bool
}

// databaseLink is the address of an object, as a block of the database links to it: /o/<id>, maybe with a name after it.
var databaseLink = regexp.MustCompile(`^/o/[0-9a-hjkmnp-tv-z]{10}(/[^/?#]*)?$`)

// Wiki resolves in this order: stable ID, path relative to the page, path from the root, then title or file name.
func (r *resolver) Wiki(target string) (markdown.Target, error) {
	if doc, ok := r.lib.documents[document.ID(target)]; ok {
		return documentTarget(doc), nil
	}
	if doc, ok := r.byPath(target); ok {
		return documentTarget(doc), nil
	}
	matches := r.lib.byName[strings.ToLower(target)]
	switch len(matches) {
	case 0:
		return markdown.Target{}, errors.New("not found")
	case 1:
		return documentTarget(matches[0]), nil
	}
	sources := make([]string, len(matches))
	for i, doc := range matches {
		sources[i] = doc.Source
	}
	slices.Sort(sources)
	return markdown.Target{}, fmt.Errorf("%w, fits %s", markdown.ErrAmbiguous, strings.Join(sources, ", "))
}

func (r *resolver) byPath(target string) (*document.Document, bool) {
	if doc, ok := r.lib.byPath[pathKey(r.relative(target))]; ok {
		return doc, true
	}
	doc, ok := r.lib.byPath[pathKey(path.Clean(strings.TrimPrefix(target, "/")))]
	return doc, ok
}

// relative resolves a path against the folder of the page. A path leaving the source finds nothing, as all lookups go
// through the scanned paths and never to the disk.
func (r *resolver) relative(target string) string {
	if strings.HasPrefix(target, "/") {
		return path.Clean(strings.TrimPrefix(target, "/"))
	}
	return path.Join(path.Dir(r.doc.Source), target)
}

// Link resolves a normal relative link, to a page, an image or a folder.
func (r *resolver) Link(linkPath string) (markdown.Target, bool) {
	if r.database && strings.HasPrefix(linkPath, "/o/") {
		if doc, ok := r.lib.documents[document.ID(strings.SplitN(linkPath[3:], "/", 2)[0])]; ok {
			return documentTarget(doc), true
		}
		if databaseLink.MatchString(linkPath) {
			return markdown.Target{Href: linkPath}, true
		}
	}
	relative := r.relative(linkPath)
	if doc, ok := r.lib.byPath[pathKey(relative)]; ok && isMarkdownPath(relative) {
		return documentTarget(doc), true
	}
	if r.lib.images[relative] {
		return markdown.Target{Href: route.File(relative)}, true
	}
	if relative == "." { // the root folder has the empty path
		relative = ""
	}
	if folder, ok := r.lib.tree.Folder(relative); ok && linkPath != "" {
		return markdown.Target{Href: route.Folder(folder.Path)}, true
	}
	return markdown.Target{}, false
}

// Image resolves the path of an image.
func (r *resolver) Image(imagePath string) (markdown.Target, bool) {
	relative := r.relative(imagePath)
	if !r.lib.images[relative] {
		return markdown.Target{}, false
	}
	return markdown.Target{Href: route.File(relative)}, true
}

func documentTarget(doc *document.Document) markdown.Target {
	return markdown.Target{Href: route.Document(string(doc.ID)), Title: doc.Title, Document: string(doc.ID)}
}

func isMarkdownPath(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".md", ".markdown":
		return true
	}
	return false
}

// pathKey makes paths comparable: without the Markdown extension and without case.
func pathKey(p string) string {
	if isMarkdownPath(p) {
		p = strings.TrimSuffix(p, path.Ext(p))
	}
	return strings.ToLower(p)
}

func stem(p string) string {
	base := path.Base(p)
	return strings.TrimSuffix(base, path.Ext(base))
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
