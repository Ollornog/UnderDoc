// SPDX-License-Identifier: MIT

// Package route is the one place that knows the URLs of the application.
package route

import (
	"net/url"
	"strings"
)

// Object returns the one address of an object, whatever its kind (ADR-18). IDs from the folder source can contain slashes.
func Object(id string) string { return "/o/" + escapeSegments(id) }

// Document returns the URL of a document, which is its object address.
func Document(id string) string { return Object(id) }

// Profile returns the URL of the profile with the account and the settings.
func Profile() string { return "/profile" }

// Tag returns the URL that lists everything carrying a tag.
func Tag(tag string) string { return "/search?tag=" + url.QueryEscape(tag) }

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
