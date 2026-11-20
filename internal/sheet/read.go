// SPDX-License-Identifier: MIT

package sheet

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/Ollornog/UnderDoc/internal/files"
)

// Types the reader knows (ADR-31).
const (
	TypeXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	TypeXLSM = "application/vnd.ms-excel.sheet.macroEnabled.12"
	TypeODS  = "application/vnd.oasis.opendocument.spreadsheet"
	TypeCSV  = "text/csv"
	TypeTSV  = "text/tab-separated-values"
)

// Readable tells whether files of a type are read as tables.
func Readable(contentType string) bool {
	switch contentType {
	case TypeXLSX, TypeXLSM, TypeODS, TypeCSV, TypeTSV:
		return true
	}
	return false
}

// Read reads a spreadsheet of a type the reader knows. name is the name of the file, the name of the one sheet of a CSV.
// It stops when ctx ends; a panic of the reader becomes ErrUnreadable.
func Read(ctx context.Context, r io.ReaderAt, size int64, contentType, name string) (wb *Workbook, err error) {
	defer func() {
		if p := recover(); p != nil {
			wb, err = nil, fmt.Errorf("%w: the reader failed", ErrUnreadable)
		}
	}()
	switch contentType {
	case TypeXLSX, TypeXLSM:
		return readXLSX(ctx, r, size)
	case TypeODS:
		return readODS(ctx, r, size)
	case TypeCSV, TypeTSV:
		return readCSV(ctx, r, size, name, contentType == TypeTSV)
	}
	return nil, fmt.Errorf("%w: not a spreadsheet", ErrUnreadable)
}

// pkg is a zip package read with the limits of internal/files: a part is checked against its packed size before a byte of
// it is unpacked, and all parts read together stay below files.MaxUnpacked.
type pkg struct {
	ctx      context.Context
	parts    map[string]*zip.File // by lower-case name
	unpacked uint64
	tokens   int
}

func openPkg(ctx context.Context, r io.ReaderAt, size int64) (*pkg, error) {
	archive, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	if len(archive.File) > files.MaxParts {
		return nil, &LimitError{What: "parts in the archive", Limit: files.MaxParts}
	}
	p := &pkg{ctx: ctx, parts: map[string]*zip.File{}}
	for _, f := range archive.File {
		p.parts[strings.ToLower(strings.TrimPrefix(strings.ReplaceAll(f.Name, `\`, "/"), "/"))] = f
	}
	return p, nil
}

func (p *pkg) has(name string) bool { _, ok := p.parts[strings.ToLower(name)]; return ok }

// open opens a part, refusing one that would unpack to too much.
func (p *pkg) open(name string) (io.ReadCloser, error) {
	f, ok := p.parts[strings.ToLower(name)]
	if !ok {
		return nil, fmt.Errorf("%w: %s is missing", ErrUnreadable, name)
	}
	if f.UncompressedSize64 > files.MaxUnpacked || f.UncompressedSize64 > files.MaxRatio*max(f.CompressedSize64, 1024) {
		return nil, fmt.Errorf("%w: %s", files.ErrBomb, name)
	}
	p.unpacked += f.UncompressedSize64
	if p.unpacked > files.MaxUnpacked {
		return nil, files.ErrBomb
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	// The sizes in the directory of a zip can lie; the reader never gives more than they say.
	return struct {
		io.Reader
		io.Closer
	}{io.LimitReader(rc, int64(f.UncompressedSize64)), rc}, nil
}

// errDoctype is an XML part that declares a document type: no part of a spreadsheet needs one, and entities come with it.
var errDoctype = errors.New("a part declares a document type")

// walk reads the XML of a part token by token and hands every start element to fn, which may read on with the decoder.
// It refuses a document type and stops when the context ends.
func (p *pkg) walk(name string, fn func(d *xml.Decoder, se xml.StartElement) error) error {
	rc, err := p.open(name)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	return p.walkReader(rc, name, fn)
}

func (p *pkg) walkReader(r io.Reader, name string, fn func(d *xml.Decoder, se xml.StartElement) error) error {
	d := newDecoder(r)
	for {
		tok, err := p.token(d)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return wrapXML(name, err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			if err := fn(d, se); err != nil {
				return wrapXML(name, err)
			}
		}
	}
}

func wrapXML(name string, err error) error {
	var limit *LimitError
	if errors.As(err, &limit) || errors.Is(err, files.ErrBomb) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) ||
		errors.Is(err, ErrUnreadable) {
		return err
	}
	return fmt.Errorf("%w: %s: %v", ErrUnreadable, name, err)
}

// newDecoder reads XML strictly: Go's decoder reads no document type definition and expands no entity besides the five
// of XML, so nothing is fetched and nothing grows.
func newDecoder(r io.Reader) *xml.Decoder {
	d := xml.NewDecoder(r)
	d.Strict = true
	return d
}

// token reads the next token, refusing a document type, and looks at the context now and then.
func (p *pkg) token(d *xml.Decoder) (xml.Token, error) {
	tok, err := d.RawToken()
	if err != nil {
		return nil, err
	}
	if dir, ok := tok.(xml.Directive); ok && strings.HasPrefix(strings.ToUpper(strings.TrimSpace(string(dir))), "DOCTYPE") {
		return nil, errDoctype
	}
	p.tokens++
	if p.tokens%4096 == 0 {
		if err := p.ctx.Err(); err != nil {
			return nil, err
		}
	}
	return tok, nil
}

// text reads the character data of the element just opened, up to its end, at most limit runes kept.
func (p *pkg) text(d *xml.Decoder, limit int) (string, error) {
	var b strings.Builder
	depth := 1
	for depth > 0 {
		tok, err := p.token(d)
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if b.Len() < limit*4 {
				b.Write(t)
			}
		}
	}
	return clipRunes(b.String(), limit), nil
}

// skip reads past the element just opened.
func (p *pkg) skip(d *xml.Decoder) error {
	depth := 1
	for depth > 0 {
		tok, err := p.token(d)
		if err != nil {
			return err
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return nil
}

func clipRunes(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	n := 0
	for i := range s {
		if n == limit {
			return s[:i]
		}
		n++
	}
	return s
}

func attr(se xml.StartElement, local string) string {
	for _, a := range se.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// attrNS reads an attribute with its prefix, as RawToken leaves it: r:id, table:name.
func attrNS(se xml.StartElement, space, local string) string {
	for _, a := range se.Attr {
		if a.Name.Local == local && a.Name.Space == space {
			return a.Value
		}
	}
	return ""
}

// relsOf reads the relationships of a part: ID → target, resolved against the folder of the part, and ID → type.
func (p *pkg) relsOf(part string) (map[string]string, map[string]string, error) {
	dir, file := path.Split(part)
	name := path.Join(dir, "_rels", file+".rels")
	targets, types := map[string]string{}, map[string]string{}
	if !p.has(name) {
		return targets, types, nil
	}
	err := p.walk(name, func(_ *xml.Decoder, se xml.StartElement) error {
		if se.Name.Local != "Relationship" || attr(se, "TargetMode") == "External" {
			return nil
		}
		target := attr(se, "Target")
		if strings.HasPrefix(target, "/") {
			target = strings.TrimPrefix(target, "/")
		} else {
			target = path.Join(dir, target)
		}
		targets[attr(se, "Id")] = target
		types[attr(se, "Id")] = attr(se, "Type")
		return nil
	})
	return targets, types, err
}
