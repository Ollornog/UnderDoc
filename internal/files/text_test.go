// SPDX-License-Identifier: MIT

package files

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func zipOf(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, content := range parts {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write([]byte(content))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// textPDF is a PDF of one page with one line of text.
func textPDF(text string) []byte { return pagedPDF(text) }

// pagedPDF is a PDF with a page for each text, one line on each; an empty text is a page without text.
func pagedPDF(texts ...string) []byte {
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"}
	var kids []string
	for _, text := range texts {
		stream := ""
		if text != "" {
			stream = "BT /F1 24 Tf 20 60 Td (" + text + ") Tj ET"
		}
		page := len(objects) + 1
		kids = append(kids, fmt.Sprintf("%d 0 R", page))
		objects = append(objects,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 144] /Contents %d 0 R /Resources << /Font << /F1 3 0 R >> >> >>", page+1),
			fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(texts))
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	var offsets []int
	for i, o := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return []byte(b.String())
}

// TestPDFPages: the text of a PDF keeps its pages apart with PageBreak, an empty page included, so the n-th part is the
// n-th page (ADR-34).
func TestPDFPages(t *testing.T) {
	got, err := read(t, pagedPDF("first page", "", "third page"), "application/pdf")
	if err != nil {
		t.Fatal(err)
	}
	pages := strings.Split(got, PageBreak)
	if len(pages) != 3 || strings.TrimSpace(pages[0]) != "first page" || strings.TrimSpace(pages[1]) != "" || strings.TrimSpace(pages[2]) != "third page" {
		t.Errorf("the pages of the text: %q", pages)
	}
	if _, err := read(t, pagedPDF("", ""), "application/pdf"); !errors.Is(err, ErrNoText) {
		t.Errorf("a PDF of empty pages has no text: %v", err)
	}
}

func read(t *testing.T, content []byte, contentType string) (string, error) {
	t.Helper()
	return ServerText(context.Background(), bytes.NewReader(content), int64(len(content)), contentType)
}

func TestServerText(t *testing.T) {
	docx := zipOf(t, map[string]string{
		"word/document.xml": `<w:document xmlns:w="w"><w:body><w:p><w:r><w:t>Error E-1042</w:t></w:r></w:p><w:p><w:r><w:t>card payment</w:t></w:r></w:p></w:body></w:document>`,
		"word/styles.xml":   `<w:styles xmlns:w="w"><w:t>not text</w:t></w:styles>`,
	})
	xlsx := zipOf(t, map[string]string{
		"xl/sharedStrings.xml":     `<sst><si><t>Router</t></si><si><t>Switch</t></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row><c><v>42</v></c></row></sheetData></worksheet>`,
	})
	pptx := zipOf(t, map[string]string{"ppt/slides/slide10.xml": `<p:sld><a:t>ten</a:t></p:sld>`, "ppt/slides/slide2.xml": `<p:sld><a:t>two</a:t></p:sld>`})
	odt := zipOf(t, map[string]string{"content.xml": `<office:document-content><text:h>Title</text:h><text:p>Body</text:p></office:document-content>`})
	cases := []struct {
		content []byte
		ct      string
		want    string
	}{
		{docx, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "Error E-1042\ncard payment"},
		{xlsx, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "Router\nSwitch\n42"},
		{pptx, "application/vnd.openxmlformats-officedocument.presentationml.presentation", "twoten"},
		{odt, "application/vnd.oasis.opendocument.text", "Title\nBody"},
		{[]byte("plain text\xff"), "text/plain", "plain text"},
		{textPDF("ORA-00942 in the log"), "application/pdf", "ORA-00942 in the log"},
	}
	for _, c := range cases {
		got, err := read(t, c.content, c.ct)
		if err != nil || strings.Join(strings.Fields(got), " ") != strings.Join(strings.Fields(c.want), " ") {
			t.Errorf("%s: %q %v, want %q", c.ct, got, err, c.want)
		}
	}
	for _, c := range []struct {
		content []byte
		ct      string
	}{
		{[]byte("%PDF-1.4\n garbage"), "application/pdf"},
		{[]byte("\x89PNG"), "image/png"},
		{[]byte("PK not a zip"), "application/vnd.oasis.opendocument.text"},
		{[]byte("<html>"), "text/html"},
	} {
		if _, err := read(t, c.content, c.ct); !errors.Is(err, ErrNoText) {
			t.Errorf("%s %q: %v", c.ct, c.content, err)
		}
	}
	long, _ := read(t, bytes.Repeat([]byte("ä"), MaxText), "text/plain")
	if len(long) > MaxText || !strings.HasPrefix(long, "ää") {
		t.Errorf("a long text is %d bytes", len(long))
	}
}

// A zip bomb in the dress of an Office file: one part that packs a gigabyte of zeros into a few kilobytes. It is refused
// by its sizes before a byte is unpacked, and never fills the memory.
func TestZipBombsAreRefused(t *testing.T) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, _ := w.Create("word/document.xml")
	zeros := make([]byte, 1<<20)
	for range 200 {
		_, _ = f.Write(zeros)
	}
	_ = w.Close()
	if b.Len() > 1<<20 {
		t.Fatalf("the bomb is %d bytes packed", b.Len())
	}
	if _, err := read(t, b.Bytes(), "application/vnd.openxmlformats-officedocument.wordprocessingml.document"); !errors.Is(err, ErrBomb) {
		t.Fatalf("a bomb of 200 MiB: %v", err)
	}
	// A part far smaller than the limit of all parts, but packed far tighter than any document: refused by its ratio.
	b.Reset()
	w = zip.NewWriter(&b)
	f, _ = w.Create("word/document.xml")
	for range 50 {
		_, _ = f.Write(zeros)
	}
	_ = w.Close()
	if _, err := read(t, b.Bytes(), "application/vnd.openxmlformats-officedocument.wordprocessingml.document"); !errors.Is(err, ErrBomb) {
		t.Fatalf("a part of 50 MiB packed into %d bytes: %v", b.Len(), err)
	}
	many := map[string]string{}
	for i := range MaxParts + 1 {
		many[fmt.Sprintf("p%d", i)] = "x"
	}
	if _, err := read(t, zipOf(t, many), "application/vnd.oasis.opendocument.text"); !errors.Is(err, ErrBomb) {
		t.Fatalf("too many parts: %v", err)
	}
}
