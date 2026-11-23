// SPDX-License-Identifier: MIT

package search

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/Ollornog/UnderDoc/internal/files"
	"github.com/Ollornog/UnderDoc/internal/markdown"
)

// Section is a heading and what follows it up to the next heading, as plain text (ADR-22). The text before the first
// heading is a section without a heading.
type Section struct {
	// Anchor is where a hit opens: the block of the heading in a page of the database, the heading's anchor in a page of
	// the folder.
	Anchor  string
	Heading string
	Level   int
	Text    string
}

// Part is one element of a page with the anchor a hit on it opens.
type Part struct {
	Anchor string
	Piece  markdown.Piece
}

// maxSectionWords splits a long section: engines index only so many words of one field (Meilisearch about 65,000
// positions), and a hit should open near its place.
const maxSectionWords = 1500

// MaxSectionWords is how many words a section of other sources holds at most, such as a run of rows of a sheet.
const MaxSectionWords = maxSectionWords

// directive matches the directives of ADR-7 and ADR-18, such as ::secret{id="…"} and ::embed{ref="…"}. Their attributes
// are addresses and IDs, never text to find, and a secret's ID has no business in an index.
var directive = regexp.MustCompile(`::[A-Za-z][A-Za-z0-9_-]*(?:\[[^\]\n]*\])?(?:\{[^}\n]*\})?`)

// StripDirectives removes directives from a text.
func StripDirectives(text string) string {
	if !strings.Contains(text, "::") {
		return text
	}
	return strings.Join(strings.Fields(directive.ReplaceAllString(text, " ")), " ")
}

// BlockParts parses the blocks of a page of the database (ADR-19) into parts, each with the ID of its block.
func BlockParts(blocks []Block) []Part {
	var parts []Part
	for _, block := range blocks {
		for _, piece := range markdown.Parse([]byte(block.Body)).Outline() {
			parts = append(parts, Part{Anchor: block.ID, Piece: piece})
		}
	}
	return parts
}

// TextParts turns plain text, such as the text taken out of a file (M-2), into parts: one per paragraph, a paragraph
// ending at an empty line, all with the same anchor. The text has no headings, so Sections only splits it where it is long.
func TextParts(text, anchor string) []Part {
	var parts []Part
	for _, paragraph := range paragraphBreak.Split(text, -1) {
		if line := strings.Join(strings.Fields(paragraph), " "); line != "" {
			parts = append(parts, Part{Anchor: anchor, Piece: markdown.Piece{Kind: "paragraph", Text: line}})
		}
	}
	return parts
}

var paragraphBreak = regexp.MustCompile(`\n[ \t\r\f\v]*\n`)

// PageBreak separates the pages in the text taken out of a document such as a PDF (files.PageBreak): each page becomes
// sections of its own, so a hit names its page (ADR-34).
const PageBreak = files.PageBreak

// PageAnchor is the anchor of a page of a document, the fragment PDF viewers open at.
func PageAnchor(n int) string { return "page=" + strconv.Itoa(n) }

// FileSections turns the text taken out of a file into sections. A text with page breaks gives each page its own
// sections, headed with the page number after heading and opening at that page, or at anchor if one is given (the block
// that shows an attachment); so does a long text without them, in the pseudo pages an agent reads (files.Pages, B-5). A
// short text without them is one run of paragraphs under heading. Empty pages give none.
func FileSections(text, anchor, heading string) []Section {
	pages := files.Pages(text)
	if len(pages) == 1 {
		sections := Sections(TextParts(text, anchor))
		for i := range sections {
			sections[i].Heading = heading
		}
		return sections
	}
	var out []Section
	for i, page := range pages {
		at := anchor
		if at == "" {
			at = PageAnchor(i + 1)
		}
		label := "Page " + strconv.Itoa(i+1)
		if heading != "" {
			label = heading + " · page " + strconv.Itoa(i+1)
		}
		for _, section := range Sections(TextParts(page, at)) {
			section.Heading = label
			out = append(out, section)
		}
	}
	return out
}

// Block is a block of a page of the database: its ID and its Markdown.
type Block struct {
	ID   string
	Body string
}

// PageParts parses a whole Markdown page into parts, each heading with its own anchor. A first heading that only repeats
// the title is left out.
func PageParts(source []byte, title string) []Part {
	return PieceParts(markdown.Parse(source).Outline(), title)
}

// PieceParts turns the outline of a whole page into parts, as PageParts does.
func PieceParts(pieces []markdown.Piece, title string) []Part {
	var parts []Part
	for i, piece := range pieces {
		if i == 0 && piece.Level == 1 && piece.Text == title {
			continue
		}
		parts = append(parts, Part{Anchor: piece.Anchor, Piece: piece})
	}
	return parts
}

// Sections groups parts into sections: one starts at every heading. A section longer than maxSectionWords goes on in
// another section with the same heading, opened at its first part.
func Sections(parts []Part) []Section {
	var out []Section
	var current *Section
	words := 0
	flush := func() {
		if current != nil && (current.Heading != "" || current.Text != "") {
			out = append(out, *current)
		}
		current = nil
	}
	for _, part := range parts {
		text := StripDirectives(part.Piece.Text)
		if part.Piece.Level > 0 {
			flush()
			current = &Section{Anchor: part.Anchor, Heading: text, Level: part.Piece.Level}
			words = 0
			continue
		}
		if text == "" {
			continue
		}
		n := len(strings.Fields(text))
		if current != nil && words > 0 && words+n > maxSectionWords {
			heading, level, anchor := current.Heading, current.Level, part.Anchor
			if anchor == "" {
				anchor = current.Anchor
			}
			flush()
			current = &Section{Anchor: anchor, Heading: heading, Level: level}
			words = 0
		}
		if current == nil {
			current = &Section{Anchor: part.Anchor}
		}
		if current.Text != "" {
			current.Text += " "
		}
		current.Text += text
		words += n
	}
	flush()
	return out
}

// SectionDocs makes one record per section from a record that holds what they share. Without sections the object is one
// record with its title only.
func SectionDocs(base Doc, sections []Section) []Doc {
	if len(sections) == 0 {
		return []Doc{base}
	}
	docs := make([]Doc, len(sections))
	for i, section := range sections {
		doc := base
		doc.Anchor, doc.Heading, doc.Body, doc.Rank = section.Anchor, section.Heading, section.Text, i
		docs[i] = doc
	}
	return docs
}
