// SPDX-License-Identifier: MIT

package files

import "testing"

func TestDetectTrustsTheContent(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	cases := []struct {
		name    string
		head    []byte
		want    string
		showsUp bool
	}{
		{"photo.png", png, "image/png", true},
		{"photo.pdf", png, "image/png", true},
		{"page.png", []byte("<html><body>hi"), "text/html", false},
		{"notes.md", []byte("# Title\n"), "text/markdown", false},
		{"notes.png", []byte("# Title\n"), "text/plain", true},
		{"data.csv", []byte("a,b\n1,2\n"), "text/csv", false},
		{"logo.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), "image/svg+xml", false},
		{"logo.svg", []byte(`<?xml version="1.0"?><svg/>`), "image/svg+xml", false},
		{"sheet.xlsx", []byte("PK\x03\x04rest"), "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", false},
		{"archive.png", []byte("PK\x03\x04rest"), "application/zip", false},
		{"report.pdf", []byte("%PDF-1.7"), "application/pdf", false},
		{"old.doc", []byte{0xd0, 0xcf, 0x11, 0xe0, 0x01}, "application/msword", false},
		{"blob.png", []byte{0x00, 0x01, 0x02, 0x03}, "application/octet-stream", false},
	}
	for _, c := range cases {
		got := Detect(c.name, c.head)
		if got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
		if Inline(got) != c.showsUp {
			t.Errorf("%s as %s: inline %v", c.name, got, Inline(got))
		}
	}
}
