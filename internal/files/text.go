// SPDX-License-Identifier: MIT

package files

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

// The text of files for the search (ADR-24). On the server, Go reads the text of PDF, Office and text files itself; the
// browser of the uploader and models for OCR are the other two ways, each one switched on its own (docs/files.md).

// Where a text came from: the search keeps it beside the text.
const (
	TextBrowser = "browser" // pdf.js or Tesseract.js in the browser of the uploader
	TextServer  = "server"  // Go on the server, from the content itself
	TextOCR     = "ocr"     // Tesseract on the server
	TextAI      = "ai"      // a model for OCR (M-13)
)

// Limits of the reading on the server, against files made to exhaust it.
const (
	// MaxText is the most text kept of one file.
	MaxText = 1 << 20
	// MaxUnpacked is the most an Office file may unpack to, all its parts together.
	MaxUnpacked = 100 << 20
	// MaxRatio is how much larger than packed a part may be: a zip bomb is far beyond it.
	MaxRatio = 200
	// MaxParts is the most parts of an Office file that are read.
	MaxParts = 2000
	// MaxPages is the most pages of a PDF that are read.
	MaxPages = 2000
)

// ErrNoText is content whose text cannot be read on the server: an image, a scan without text, an unknown type.
var ErrNoText = errors.New("no text to read in this content")

// ErrBomb is an archive that would unpack to far more than it is.
var ErrBomb = errors.New("the archive unpacks to too much")

// HasServerText tells whether Go on the server reads text of this type.
func HasServerText(contentType string) bool {
	return isPlain(contentType) || contentType == "application/pdf" || officeKind(contentType) != ""
}

func isPlain(contentType string) bool {
	return strings.HasPrefix(contentType, "text/") && contentType != "text/html" && contentType != "text/xml" ||
		contentType == "application/json" || contentType == "application/yaml"
}

func officeKind(contentType string) string {
	switch contentType {
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "docx"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return "xlsx"
	case "application/vnd.openxmlformats-officedocument.presentationml.presentation":
		return "pptx"
	case "application/vnd.oasis.opendocument.text", "application/vnd.oasis.opendocument.spreadsheet", "application/vnd.oasis.opendocument.presentation":
		return "odf"
	}
	return ""
}

// ServerText reads the text of a file on the server, at most MaxText bytes of it. It stops when ctx ends.
func ServerText(ctx context.Context, r io.ReaderAt, size int64, contentType string) (text string, err error) {
	switch {
	case isPlain(contentType):
		b, err := io.ReadAll(io.NewSectionReader(r, 0, min(size, MaxText)))
		if err != nil {
			return "", err
		}
		return clip(strings.ToValidUTF8(string(b), "")), nil
	case contentType == "application/pdf":
		return pdfText(ctx, r, size)
	}
	if kind := officeKind(contentType); kind != "" {
		return officeText(ctx, r, size, kind)
	}
	return "", ErrNoText
}

// pdfText reads the text of each page. The reader of PDF panics on some broken files, which here is only a failure.
func pdfText(ctx context.Context, r io.ReaderAt, size int64) (text string, err error) {
	defer func() {
		if p := recover(); p != nil {
			text, err = "", fmt.Errorf("%w: a broken PDF (%v)", ErrNoText, p)
		}
	}()
	doc, err := pdf.NewReader(r, size)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNoText, err)
	}
	var b strings.Builder
	for n := 1; n <= min(doc.NumPage(), MaxPages) && b.Len() < MaxText; n++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		page := doc.Page(n)
		if page.V.IsNull() {
			continue
		}
		t, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		b.WriteString(t)
		b.WriteString("\n")
	}
	text = clip(strings.TrimSpace(b.String()))
	if text == "" {
		return "", fmt.Errorf("%w: the PDF has no text, perhaps a scan", ErrNoText)
	}
	return text, nil
}

// officeParts names the parts of an Office file that hold its text, in the order they are read.
var officeParts = map[string]*regexp.Regexp{
	"docx": regexp.MustCompile(`^word/(document|header\d*|footer\d*|footnotes|endnotes)\.xml$`),
	"xlsx": regexp.MustCompile(`^xl/(sharedStrings|worksheets/sheet\d+)\.xml$`),
	"pptx": regexp.MustCompile(`^ppt/(slides/slide\d+|notesSlides/notesSlide\d+)\.xml$`),
	"odf":  regexp.MustCompile(`^content\.xml$`),
}

// officeText reads the text of an Office file: a zip archive of XML parts. Every part is checked against its packed size
// before a byte is unpacked, and the unpacking stops at the limits, so a zip bomb costs nothing.
func officeText(ctx context.Context, r io.ReaderAt, size int64, kind string) (string, error) {
	// The directory is counted before Go's reader reads all of it into memory.
	if n := ZipParts(r, size, MaxParts); n > MaxParts {
		return "", fmt.Errorf("%w: more than %d parts", ErrBomb, MaxParts)
	}
	archive, err := zip.NewReader(r, size)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNoText, err)
	}
	if len(archive.File) > MaxParts {
		return "", fmt.Errorf("%w: %d parts", ErrBomb, len(archive.File))
	}
	parts := slices.Clone(archive.File)
	slices.SortStableFunc(parts, func(a, b *zip.File) int { return naturalCompare(a.Name, b.Name) })
	var out strings.Builder
	var unpacked uint64
	for _, f := range parts {
		if !officeParts[kind].MatchString(f.Name) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if f.UncompressedSize64 > MaxUnpacked || f.UncompressedSize64 > MaxRatio*max(f.CompressedSize64, 1024) {
			return "", fmt.Errorf("%w: %s", ErrBomb, f.Name)
		}
		unpacked += f.UncompressedSize64
		if unpacked > MaxUnpacked {
			return "", ErrBomb
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrNoText, err)
		}
		// The sizes in the directory of a zip can lie; the reader never gives more than they say, and this never more
		// than the limit.
		err = xmlText(io.LimitReader(rc, int64(f.UncompressedSize64)), &out)
		_ = rc.Close()
		if err != nil {
			return "", fmt.Errorf("%w: %s: %v", ErrNoText, f.Name, err)
		}
		if out.Len() >= MaxText {
			break
		}
	}
	text := clip(strings.TrimSpace(out.String()))
	if text == "" {
		return "", fmt.Errorf("%w: the file holds no text", ErrNoText)
	}
	return text, nil
}

// paragraphs are the elements after which a line ends, in the XML of OOXML and OpenDocument.
var paragraphs = map[string]bool{"p": true, "h": true, "tr": true, "row": true, "si": true, "br": true}

// xmlText writes the character data of an XML part, a line after each paragraph, row and shared string.
func xmlText(r io.Reader, out *strings.Builder) error {
	d := xml.NewDecoder(r)
	d.Strict = false
	for out.Len() < MaxText {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.CharData:
			out.Write(t)
		case xml.EndElement:
			if paragraphs[t.Name.Local] {
				out.WriteString("\n")
			} else if t.Name.Local == "c" || t.Name.Local == "tab" || t.Name.Local == "table-cell" {
				out.WriteString("\t")
			}
		}
	}
	return nil
}

var digits = regexp.MustCompile(`\d+`)

// naturalCompare puts slide2.xml before slide10.xml.
func naturalCompare(a, b string) int {
	da, db := path.Dir(a), path.Dir(b)
	if da != db {
		return strings.Compare(da, db)
	}
	na, nb := digits.FindString(path.Base(a)), digits.FindString(path.Base(b))
	if len(na) != len(nb) {
		return len(na) - len(nb)
	}
	return strings.Compare(a, b)
}

// clip cuts a text at MaxText bytes, on a rune.
func clip(text string) string {
	if len(text) <= MaxText {
		return text
	}
	text = text[:MaxText]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text
}

// OCR reads the text of an image or a scanned PDF. Tesseract on the server and a model for OCR (M-13) are two of them;
// both are optional, and without them a scan is read only in the browser.
type OCR interface {
	// Recognize returns the text of the content and how many pages it read, which counts against the quotas of a model.
	Recognize(ctx context.Context, content io.Reader, contentType string) (text string, pages int, err error)
}

// IsScan tells whether a type may hold text only as pixels: an image or a PDF.
func IsScan(contentType string) bool { return IsImage(contentType) || contentType == "application/pdf" }

// Tesseract reads the text of images with the program tesseract on the server, an optional part of the way of the server
// (UNDERDOC_TESSERACT): the image of UnderDoc does not bring it.
type Tesseract struct {
	// Path is the program, Langs the languages it reads, such as "deu+eng".
	Path, Langs string
}

// Recognize runs tesseract on one image, reading it from stdin and its text from stdout.
func (t Tesseract) Recognize(ctx context.Context, content io.Reader, contentType string) (string, int, error) {
	if !IsImage(contentType) {
		return "", 0, ErrNoText
	}
	langs := t.Langs
	if langs == "" {
		langs = "deu+eng"
	}
	cmd := exec.CommandContext(ctx, t.Path, "stdin", "stdout", "-l", langs)
	cmd.Stdin = content
	var out limitedBuffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", 0, fmt.Errorf("tesseract: %w", err)
	}
	return clip(strings.ToValidUTF8(out.String(), "")), 1, nil
}

// limitedBuffer keeps at most MaxText bytes and drops the rest.
type limitedBuffer struct{ strings.Builder }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := MaxText - b.Len(); room > 0 {
		b.Builder.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}
