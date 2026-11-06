// SPDX-License-Identifier: MIT

// Package document holds what a document is, independent of where it appears in the folder tree (ADR-13).
package document

import (
	"regexp"
	"time"
)

// ID identifies a document. A stable ID comes from the document itself and survives a move. A path ID only exists until
// the document gets a stable one, so it changes when the file moves.
type ID string

var stableID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// ValidStableID reports whether s can be used as a stable ID.
func ValidStableID(s string) bool { return stableID.MatchString(s) }

// PathID returns the ID of a document without a stable ID. The prefix cannot occur in a stable ID.
func PathID(source string) ID { return ID("~" + source) }

// Heading is one entry of a table of contents.
type Heading struct {
	Level  int
	Anchor string
	Text   string
}

// Document is one readable document, already rendered.
type Document struct {
	ID ID
	// Title is the display name.
	Title string
	// Source says where the content came from. In the folder source it is the path inside the folder.
	Source string
	// Stable is true if the ID comes from the document itself.
	Stable   bool
	Modified time.Time
	// HTML is the rendered and sanitized body.
	HTML string
	// Text is the plain text of the body, on one line.
	Text string
	// StartsWithTitle is true if the body opens with its own top-level heading.
	StartsWithTitle bool
	// Contents lists the main headings of the body.
	Contents []Heading
	// Links are the other documents this one points to, and Backlinks the ones pointing here. Both are unique and sorted.
	Links     []ID
	Backlinks []ID
}
