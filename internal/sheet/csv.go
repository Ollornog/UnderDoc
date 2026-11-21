// SPDX-License-Identifier: MIT

package sheet

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/Ollornog/UnderDoc/internal/files"
)

// CSV: one sheet named after the file, UTF-8 with or without a byte order mark, otherwise Windows-1252; the separator is
// found from the first line (comma, semicolon, tab). Every cell is text: CSV carries no formats.

func readCSV(ctx context.Context, r io.ReaderAt, size int64, name string, tabs bool) (*Workbook, error) {
	if size > files.MaxUnpacked {
		return nil, &LimitError{What: "bytes of text", Limit: files.MaxUnpacked}
	}
	content, err := io.ReadAll(io.NewSectionReader(r, 0, size))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	content = bytes.TrimPrefix(content, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(content) {
		content = fromWindows1252(content)
	}
	reader := csv.NewReader(bytes.NewReader(content))
	reader.Comma = separator(content, tabs)
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = true
	sh := Sheet{Name: csvTitle(name)}
	cells, text := 0, 0
	for row := 1; ; row++ {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnreadable, err)
		}
		if row > MaxRows {
			return nil, &LimitError{What: "rows", Limit: MaxRows}
		}
		if len(record) > MaxCols {
			return nil, &LimitError{What: "columns", Limit: MaxCols}
		}
		if row%4096 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		var out Row
		for i, field := range record {
			if field == "" {
				continue
			}
			cells++
			if cells > MaxCells {
				return nil, &LimitError{What: "cells", Limit: MaxCells}
			}
			value := clipRunes(field, MaxCellText)
			if text += len(value); text > MaxText {
				return nil, &LimitError{What: textLimit, Limit: MaxText}
			}
			out.Cells = append(out.Cells, Cell{Col: i + 1, Kind: Text, Value: value})
		}
		if len(out.Cells) > 0 {
			out.N = row
			sh.Rows = append(sh.Rows, out)
		}
	}
	return &Workbook{Sheets: []Sheet{sh}, Formats: []string{"General"}}, nil
}

// csvTitle is the name of the one sheet of a CSV: the name of its file without its extension.
func csvTitle(name string) string {
	title := strings.TrimSuffix(path.Base(name), path.Ext(name))
	if title == "" || title == "." || title == "/" {
		title = "Sheet1"
	}
	return clipRunes(title, MaxNameRunes)
}

// separator picks the separator that the first line, outside quotes, holds most of.
func separator(content []byte, tabs bool) rune {
	if tabs {
		return '\t'
	}
	counts := map[rune]int{}
	quoted := false
	for _, c := range string(content[:min(len(content), 64<<10)]) {
		if c == '"' {
			quoted = !quoted
			continue
		}
		if c == '\n' && !quoted {
			break
		}
		if !quoted && (c == ',' || c == ';' || c == '\t') {
			counts[c]++
		}
	}
	best := ','
	for _, c := range []rune{';', '\t'} {
		if counts[c] > counts[best] {
			best = c
		}
	}
	return best
}

// windows1252 are the characters of 0x80 to 0x9F in Windows-1252; the rest is Latin-1.
var windows1252 = [32]rune{'€', 0x81, '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', 0x8D, 'Ž', 0x8F,
	0x90, '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', 0x9D, 'ž', 'Ÿ'}

func fromWindows1252(b []byte) []byte {
	var out bytes.Buffer
	out.Grow(len(b) + len(b)/4)
	for _, c := range b {
		switch {
		case c < 0x80:
			out.WriteByte(c)
		case c < 0xA0:
			out.WriteRune(windows1252[c-0x80])
		default:
			out.WriteRune(rune(c))
		}
	}
	return out.Bytes()
}
