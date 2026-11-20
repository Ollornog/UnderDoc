// SPDX-License-Identifier: MIT

package markdown

import (
	"slices"
	"strings"
	"testing"
)

// TestASecretRendersAsItsSentinel: a reference to a password (ADR-7) becomes the sentinel the web replaces, alone on a
// line and inside a paragraph, and stays text in code.
func TestASecretRendersAsItsSentinel(t *testing.T) {
	const ref = `::secret{id="7k2m9q4x1c"}`
	for _, source := range []string{ref, "Root: " + ref + " (rotated yearly)", "- " + ref} {
		got := render(t, source).HTML
		if !strings.Contains(got, SecretSentinel("7k2m9q4x1c")) {
			t.Errorf("%q rendered without the sentinel: %s", source, got)
		}
		if strings.Contains(got, "::secret") {
			t.Errorf("%q left the directive in the page: %s", source, got)
		}
	}
	for _, source := range []string{"`" + ref + "`", "```\n" + ref + "\n```"} {
		if got := render(t, source).HTML; strings.Contains(got, "data-secret") {
			t.Errorf("a reference in code became a secret: %s", got)
		}
	}
	if got := render(t, `::secret{id="7K2M9Q4X1C"}`).HTML; strings.Contains(got, "data-secret") {
		t.Errorf("an ID that is not one became a secret: %s", got)
	}
	if !SecretSentinels.MatchString(SecretSentinel("7k2m9q4x1c")) {
		t.Error("the sentinel is not found again")
	}
}

// TestSecretIDs lists each referenced ID once.
func TestSecretIDs(t *testing.T) {
	got := SecretIDs(`a ::secret{id="7k2m9q4x1c"} b ::secret{id="8p3n5v2w6h"} ::secret{id="7k2m9q4x1c"} ::secret{id="nope"}`)
	if !slices.Equal(got, []string{"7k2m9q4x1c", "8p3n5v2w6h"}) {
		t.Errorf("SecretIDs = %v", got)
	}
}

// TestASecretIsNoTextOfTheOutline: the outline, which the search and the parts read, holds neither the directive nor
// the ID.
func TestASecretIsNoTextOfTheOutline(t *testing.T) {
	for _, piece := range Parse([]byte("# Router\n\nAdmin password: ::secret{id=\"7k2m9q4x1c\"} here")).Outline() {
		if strings.Contains(piece.Text, "7k2m9q4x1c") || strings.Contains(piece.Text, "secret{") {
			t.Errorf("the outline holds the reference: %q", piece.Text)
		}
	}
}
