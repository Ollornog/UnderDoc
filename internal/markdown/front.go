// SPDX-License-Identifier: MIT

package markdown

// SplitFrontMatter separates the front matter of a page from its body, the same way Parse does, for readers that need
// keys of their own, such as the import (ADR-19). Without front matter, body is the whole content and found is false.
func SplitFrontMatter(content []byte) (yamlPart, body []byte, found bool) {
	return splitFrontMatter(content)
}
