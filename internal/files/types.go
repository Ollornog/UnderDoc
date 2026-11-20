// SPDX-License-Identifier: MIT

// Package files knows what a file is from its content (ADR-28): its type for storing and serving, previews and its text
// for the search. It never trusts what a browser says a file is: the type comes from the first bytes, and the name only
// tells apart what the bytes cannot, such as the kinds of Office files, which are all zip archives.
package files

import (
	"mime"
	"net/http"
	"path"
	"strings"
)

// Sniff is how many bytes Detect looks at.
const Sniff = 512

// byExtension refines what the content alone leaves open: plain text, zip archives and unknown binary data.
var byExtension = map[string]string{
	".md":       "text/markdown",
	".markdown": "text/markdown",
	".csv":      "text/csv",
	".tsv":      "text/tab-separated-values",
	".json":     "application/json",
	".yaml":     "application/yaml",
	".yml":      "application/yaml",
	".log":      "text/plain",
	".txt":      "text/plain",
	".svg":      "image/svg+xml",
	".html":     "text/html",
	".htm":      "text/html",
	".xml":      "text/xml",
	".js":       "text/javascript",
	".css":      "text/css",
	".docx":     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xlsx":     "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".xlsm":     "application/vnd.ms-excel.sheet.macroEnabled.12",
	".pptx":     "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".odt":      "application/vnd.oasis.opendocument.text",
	".ods":      "application/vnd.oasis.opendocument.spreadsheet",
	".odp":      "application/vnd.oasis.opendocument.presentation",
	".doc":      "application/msword",
	".xls":      "application/vnd.ms-excel",
	".ppt":      "application/vnd.ms-powerpoint",
	".heic":     "image/heic",
	".tif":      "image/tiff",
	".tiff":     "image/tiff",
}

// Detect returns the type of a file from its first bytes (at most Sniff) and its name. The content decides where it
// knows: a PNG named report.pdf is a PNG. The name only refines text, zip archives and unknown data, and a name never
// turns content into an image, a PDF or anything else that is shown inline.
func Detect(name string, head []byte) string {
	sniffed, _, _ := mime.ParseMediaType(http.DetectContentType(head))
	ext := strings.ToLower(path.Ext(name))
	byName := byExtension[ext]
	switch sniffed {
	case "text/plain":
		if strings.HasPrefix(byName, "text/") || byName == "application/json" || byName == "application/yaml" || byName == "image/svg+xml" {
			return byName
		}
		return "text/plain"
	case "application/zip":
		if strings.HasPrefix(byName, "application/vnd.") {
			return byName
		}
		return sniffed
	case "application/octet-stream":
		if byName != "" && !Inline(byName) {
			return byName
		}
		return sniffed
	case "text/xml":
		// An SVG starts like any XML document; it stays a document that can carry scripts, never an image to show.
		if ext == ".svg" {
			return "image/svg+xml"
		}
	}
	return sniffed
}

// inline are the types the browser may show at the address of the application: raster images, plain text, sound and
// video. Everything else is downloaded, above all what can carry scripts, such as HTML, SVG and XML.
var inline = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
	"image/avif": true,
	"image/bmp":  true,
	"text/plain": true,
	"video/mp4":  true,
	"video/webm": true,
	"audio/mpeg": true,
	"audio/ogg":  true,
	"audio/wave": true,
	"audio/webm": true,
}

// Inline tells whether a type may be shown in the browser rather than downloaded.
func Inline(contentType string) bool { return inline[contentType] }

// IsImage tells whether a type is a raster image a preview can be made of.
func IsImage(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp":
		return true
	}
	return false
}
