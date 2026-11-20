// SPDX-License-Identifier: MIT

package search

import (
	"strings"
	"testing"
)

// TestStripDirectivesTakesThePasswordReference: the layer of the search on its own (ADR-7): a reference to a password,
// alone or inside a sentence, leaves no ID in the text to index, and the words around it stay. The parser of the
// Markdown keeps it out of the outline too (markdown.TestASecretIsNoTextOfTheOutline); each layer holds alone.
func TestStripDirectivesTakesThePasswordReference(t *testing.T) {
	for _, text := range []string{`::secret{id="7k2m9q4x1c"}`, `Router admin: ::secret{id="7k2m9q4x1c"} rotated yearly`} {
		got := StripDirectives(text)
		if strings.Contains(got, "7k2m9q4x1c") || strings.Contains(got, "secret{") {
			t.Errorf("StripDirectives(%q) = %q", text, got)
		}
		if strings.Contains(text, "rotated") && got != "Router admin: rotated yearly" {
			t.Errorf("the words around it: %q", got)
		}
	}
}
