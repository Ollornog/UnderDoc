// SPDX-License-Identifier: MIT

package markdown

import (
	"strings"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
)

// Piece is one element at the top of a page as plain text: a heading, a paragraph, a list, a table, a quote or code. The
// search cuts pages into sections from these (ADR-22, ADR-24).
type Piece struct {
	// Level is 1 to 6 for a heading and 0 for everything else.
	Level int
	// Anchor is the ID a heading gets in the rendered page, so a hit can open there.
	Anchor string
	// Kind is heading, paragraph, list, table, quote, code or other.
	Kind string
	// Text is the plain text on one line. Cells of a table and items of a list are kept apart by a space.
	Text string
}

// Outline lists the top-level elements of the page in their order. Call it before Render, which changes the tree.
func (p *Page) Outline() []Piece {
	var pieces []Piece
	for n := p.root.FirstChild(); n != nil; n = n.NextSibling() {
		var b strings.Builder
		collectText(n, p.source, &b)
		piece := Piece{Kind: kindOf(n), Text: strings.Join(strings.Fields(b.String()), " ")}
		if heading, ok := n.(*ast.Heading); ok {
			piece.Level = heading.Level
			if anchor, found := heading.AttributeString("id"); found {
				if id, isBytes := anchor.([]byte); isBytes {
					piece.Anchor = string(id)
				}
			}
		}
		if piece.Text != "" || piece.Level > 0 {
			pieces = append(pieces, piece)
		}
	}
	return pieces
}

func kindOf(n ast.Node) string {
	switch n.(type) {
	case *ast.Heading:
		return "heading"
	case *ast.Paragraph, *ast.TextBlock:
		return "paragraph"
	case *ast.List:
		return "list"
	case *extast.Table:
		return "table"
	case *ast.Blockquote:
		return "quote"
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		return "code"
	}
	return "other"
}
