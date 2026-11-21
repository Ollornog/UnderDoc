// SPDX-License-Identifier: MIT

package sheet

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// OpenDocument spreadsheets: content.xml holds every sheet. Beside each value a cell stores the text its program
// displayed (text:p), which UnderDoc shows as it is (ADR-31). Repeated rows and columns count as what they repeat.

type odsReader struct {
	*pkg
	wb           *Workbook
	hiddenTables map[string]bool // styles of tables that are not displayed
	cells        int
	// text counts the bytes of text of the cells, a repeated cell as often as it repeats (MaxText).
	text int
	// hidden counts the hidden rows and columns kept (MaxHidden).
	hidden int
}

func readODS(ctx context.Context, r io.ReaderAt, size int64) (*Workbook, error) {
	p, err := openPkg(ctx, r, size)
	if err != nil {
		return nil, err
	}
	if !p.has("content.xml") {
		return nil, fmt.Errorf("%w: no content", ErrUnreadable)
	}
	o := &odsReader{pkg: p, wb: &Workbook{Formats: []string{"General"}, App: "libreoffice"}, hiddenTables: map[string]bool{}}
	err = o.walk("content.xml", func(d *xml.Decoder, se xml.StartElement) error {
		switch se.Name.Local {
		case "style":
			if attr(se, "family") == "table" {
				return o.readTableStyle(d, attr(se, "name"))
			}
			return o.skip(d)
		case "table":
			if se.Name.Space != "table" {
				return nil
			}
			if len(o.wb.Sheets) >= MaxSheets {
				return &LimitError{What: "sheets", Limit: MaxSheets}
			}
			return o.readTable(d, se)
		case "named-range":
			o.addName(se, "")
		case "database-range":
			o.addTable(se)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Names and ranges on hidden sheets are not offered.
	names := o.wb.Names[:0]
	for _, n := range o.wb.Names {
		if _, visible := o.wb.Sheet(n.Sheet); visible {
			names = append(names, n)
		}
	}
	o.wb.Names = names
	tables := o.wb.Tables[:0]
	for _, t := range o.wb.Tables {
		if _, visible := o.wb.Sheet(t.Sheet); visible {
			tables = append(tables, t)
		}
	}
	o.wb.Tables = tables
	return o.wb, nil
}

func (o *odsReader) readTableStyle(d *xml.Decoder, name string) error {
	depth := 1
	for depth > 0 {
		tok, err := o.token(d)
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if t.Name.Local == "table-properties" && attr(t, "display") == "false" {
				o.hiddenTables[name] = true
			}
		case xml.EndElement:
			depth--
		}
	}
	return nil
}

// odsRange reads an address such as $Sheet1.$A$1:.$D$20 or $'My sheet'.A1.
func odsRange(s string) (string, Rect, bool) {
	s = strings.TrimSpace(s)
	if strings.ContainsAny(s, " ;") && !strings.Contains(s, "'") {
		return "", Rect{}, false
	}
	first, second, two := cutOutsideQuotes(s, ':')
	sheetName, a, ok := odsCell(first)
	if !ok {
		return "", Rect{}, false
	}
	rect, ok := ParseRect(a)
	if !ok {
		return "", Rect{}, false
	}
	if two {
		other, b, ok := odsCell(second)
		if !ok || (other != "" && other != sheetName) {
			return "", Rect{}, false
		}
		r2, c2, ok := ParseCell(b)
		if !ok {
			return "", Rect{}, false
		}
		rect = Rect{R1: min(rect.R1, r2), C1: min(rect.C1, c2), R2: max(rect.R2, r2), C2: max(rect.C2, c2)}
	}
	return sheetName, rect, sheetName != ""
}

func cutOutsideQuotes(s string, sep byte) (string, string, bool) {
	quoted := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\'':
			quoted = !quoted
		case s[i] == sep && !quoted:
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}

// odsCell splits $Sheet1.$A$1 into the sheet and the cell; .$D$20 has no sheet.
func odsCell(s string) (string, string, bool) {
	s = strings.TrimPrefix(s, "$")
	dot := strings.LastIndexByte(s, '.')
	if dot < 0 {
		return "", "", false
	}
	sheetName := s[:dot]
	if strings.HasPrefix(sheetName, "'") {
		if len(sheetName) < 2 || !strings.HasSuffix(sheetName, "'") {
			return "", "", false
		}
		sheetName = strings.ReplaceAll(sheetName[1:len(sheetName)-1], "''", "'")
	}
	return strings.TrimPrefix(sheetName, "$"), s[dot+1:], true
}

func (o *odsReader) addName(se xml.StartElement, scope string) {
	if len(o.wb.Names) >= MaxNames {
		return
	}
	sheetName, rect, ok := odsRange(attr(se, "cell-range-address"))
	if !ok || !rect.Valid() {
		return
	}
	o.wb.Names = append(o.wb.Names, Name{Name: clipRunes(attr(se, "name"), MaxNameRunes), Sheet: sheetName, Rect: rect, Scope: scope})
}

func (o *odsReader) addTable(se xml.StartElement) {
	name := attr(se, "name")
	if len(o.wb.Tables) >= MaxTables || name == "" || strings.HasPrefix(name, "__Anonymous_Sheet_DB__") {
		return
	}
	sheetName, rect, ok := odsRange(attr(se, "target-range-address"))
	if !ok || !rect.Valid() {
		return
	}
	header := 1
	if attr(se, "contains-header") == "false" {
		header = 0
	}
	o.wb.Tables = append(o.wb.Tables, Table{Name: clipRunes(name, MaxNameRunes), Sheet: sheetName, Rect: rect, Header: header})
}

// readTable reads one sheet: its columns, rows and cells.
func (o *odsReader) readTable(d *xml.Decoder, se xml.StartElement) error {
	sh := Sheet{Name: clipRunes(attr(se, "name"), MaxNameRunes), Hidden: o.hiddenTables[attr(se, "style-name")]}
	if sh.Hidden {
		o.wb.Sheets = append(o.wb.Sheets, sh)
		return o.skip(d)
	}
	row, col := 0, 0
	depth := 1
	for depth > 0 {
		tok, err := o.token(d)
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.EndElement:
			depth--
		case xml.StartElement:
			switch t.Name.Local {
			case "table-column":
				n := repeat(attr(t, "number-columns-repeated"))
				hidden := attr(t, "visibility") == "collapse" || attr(t, "visibility") == "filter"
				for range n {
					col++
					if col > MaxCols {
						break
					}
					if hidden {
						sh.HiddenCols = append(sh.HiddenCols, col)
					}
				}
				if err := o.skip(d); err != nil {
					return err
				}
			case "table-row":
				n := repeat(attr(t, "number-rows-repeated"))
				hidden := attr(t, "visibility") == "collapse" || attr(t, "visibility") == "filter"
				cells, merges, err := o.readRow(d)
				if err != nil {
					return err
				}
				text := 0
				for _, c := range cells {
					text += len(c.Value)
				}
				for range n {
					row++
					if row > MaxRows {
						return &LimitError{What: "rows", Limit: MaxRows}
					}
					if hidden {
						sh.HiddenRows = append(sh.HiddenRows, row)
					}
					if len(cells) > 0 {
						o.cells += len(cells)
						if o.cells > MaxCells {
							return &LimitError{What: "cells", Limit: MaxCells}
						}
						if o.text += text; o.text > MaxText {
							return &LimitError{What: textLimit, Limit: MaxText}
						}
						sh.Rows = append(sh.Rows, Row{N: row, Cells: cells})
					}
					for _, m := range merges {
						if len(sh.Merges) >= MaxMerges {
							return &LimitError{What: "merged areas", Limit: MaxMerges}
						}
						sh.Merges = append(sh.Merges, Rect{R1: row, C1: m.C1, R2: min(row+m.R2-1, MaxRows), C2: m.C2})
					}
					if len(cells) == 0 && len(merges) == 0 && !hidden {
						// An empty repeated row only moves on.
						row += n - 1
						break
					}
				}
			case "named-range":
				o.addName(t, sh.Name)
				if err := o.skip(d); err != nil {
					return err
				}
			case "table-header-rows", "table-rows", "table-row-group", "table-header-columns", "table-columns", "table-column-group", "named-expressions":
				depth++
			default:
				if err := o.skip(d); err != nil {
					return err
				}
			}
		}
	}
	if o.hidden += sh.trimHidden(); o.hidden > MaxHidden {
		return &LimitError{What: "hidden rows and columns", Limit: MaxHidden}
	}
	o.wb.Sheets = append(o.wb.Sheets, sh)
	return nil
}

func repeat(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 1
	}
	return min(n, MaxRows)
}

// readRow reads the cells of a row; merges carry their column span in C1..C2 and their row span in R2.
func (o *odsReader) readRow(d *xml.Decoder) ([]Cell, []Rect, error) {
	var cells []Cell
	var merges []Rect
	col := 0
	for {
		tok, err := o.token(d)
		if err != nil {
			return nil, nil, err
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if t.Name.Local == "table-row" {
				return cells, merges, nil
			}
		case xml.StartElement:
			if t.Name.Local != "table-cell" && t.Name.Local != "covered-table-cell" {
				if err := o.skip(d); err != nil {
					return nil, nil, err
				}
				continue
			}
			n := repeat(attr(t, "number-columns-repeated"))
			cell, ok, err := o.readCell(d, t)
			if err != nil {
				return nil, nil, err
			}
			rows, _ := strconv.Atoi(attr(t, "number-rows-spanned"))
			cols, _ := strconv.Atoi(attr(t, "number-columns-spanned"))
			for range n {
				col++
				if col > MaxCols {
					if ok {
						return nil, nil, &LimitError{What: "columns", Limit: MaxCols}
					}
					break
				}
				if ok && t.Name.Local == "table-cell" {
					if len(cells) >= MaxCells {
						return nil, nil, &LimitError{What: "cells", Limit: MaxCells}
					}
					cell.Col = col
					cells = append(cells, cell)
				}
				if rows > 1 || cols > 1 {
					merges = append(merges, Rect{C1: col, R2: max(rows, 1), C2: min(col+max(cols, 1)-1, MaxCols)})
				}
			}
		}
	}
}

// readCell reads a cell: the text its program displayed, and what kind of value it holds.
func (o *odsReader) readCell(d *xml.Decoder, se xml.StartElement) (Cell, bool, error) {
	valueType := attrNS(se, "office", "value-type")
	if valueType == "" {
		valueType = attr(se, "value-type")
	}
	calc := attrNS(se, "calcext", "value-type")
	formula := attr(se, "formula") != ""
	var b strings.Builder
	paragraphs := 0
	depth := 1
	for depth > 0 {
		tok, err := o.token(d)
		if err != nil {
			return Cell{}, false, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "p", "h":
				if paragraphs > 0 {
					b.WriteByte('\n')
				}
				paragraphs++
			case "s":
				if n := repeat(attr(t, "c")); b.Len() < MaxCellText {
					b.WriteString(strings.Repeat(" ", min(n, MaxCellText)))
				}
			case "tab":
				b.WriteByte('\t')
			case "line-break":
				b.WriteByte('\n')
			case "annotation":
				// A comment is not the cell's text.
				if err := o.skip(d); err != nil {
					return Cell{}, false, err
				}
				depth--
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if b.Len() < MaxCellText*4 {
				b.Write(t)
			}
		}
	}
	text := clipRunes(b.String(), MaxCellText)
	switch {
	case calc == "error":
		return Cell{Kind: Error, Value: text}, true, nil
	case valueType == "" && formula:
		return Cell{Kind: Uncomputed}, true, nil
	case valueType == "" && text == "":
		return Cell{}, false, nil
	case valueType == "string" || valueType == "":
		return Cell{Kind: Text, Value: text}, true, nil
	}
	if text == "" {
		text = attrNS(se, "office", "value")
		if text == "" {
			text = attrNS(se, "office", "date-value") + attrNS(se, "office", "time-value") + attrNS(se, "office", "boolean-value")
		}
	}
	if valueType == "boolean" {
		// A boolean reads in the reader's language, as it does from SpreadsheetML.
		v := "0"
		if attrNS(se, "office", "boolean-value") == "true" {
			v = "1"
		}
		return Cell{Kind: Bool, Value: v}, true, nil
	}
	return Cell{Kind: ShownNumber, Value: text}, true, nil
}
