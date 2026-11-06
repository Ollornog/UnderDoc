// SPDX-License-Identifier: MIT

// Package route is the one place that knows the URLs of the application.
package route

import (
	"net/url"
	"strings"
)

// Document returns the URL of a document. IDs from the folder source can contain slashes.
func Document(id string) string { return "/d/" + escapeSegments(id) }

// Folder returns the URL of a folder listing. The empty path is the root folder.
func Folder(path string) string { return "/f/" + escapeSegments(path) }

// File returns the URL of a file next to the pages.
func File(path string) string { return "/files/" + escapeSegments(path) }

// Search returns the URL of a search.
func Search(query string) string { return "/search?q=" + url.QueryEscape(query) }

func escapeSegments(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
