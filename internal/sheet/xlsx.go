// SPDX-License-Identifier: MIT

package sheet

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// SpreadsheetML (ECMA-376): a zip of XML parts. The reader follows the relationships from the package to the workbook,
// its sheets, shared strings, styles and tables, and streams each part. Macros (vbaProject.bin), links to other
// workbooks, drawings and comments are never opened.

type xlsxSheet struct {
	name, state, rel string
	number           int
}

type xlsxReader struct {
	*pkg
	wb      *Workbook
	strings []string
	// styles maps the index of a cell style to an index of wb.Formats.
	styles []int
	cells  int
}

func readXLSX(ctx context.Context, r io.ReaderAt, size int64) (*Workbook, error) {
	p, err := openPkg(ctx, r, size)
	if err != nil {
		return nil, err
	}
	x := &xlsxReader{pkg: p, wb: &Workbook{Formats: []string{"General"}}}
	return x.read()
}

func (x *xlsxReader) read() (*Workbook, error) {
	// The workbook: where the package says it is, else where it usually is.
	book := "xl/workbook.xml"
	if targets, types, err := x.relsOf(""); err == nil {
		for id, t := range types {
			if strings.HasSuffix(t, "/officeDocument") {
				book = targets[id]
			}
		}
	} else {
		return nil, err
	}
	if !x.has(book) {
		return nil, fmt.Errorf("%w: no workbook", ErrUnreadable)
	}
	targets, types, err := x.relsOf(book)
	if err != nil {
		return nil, err
	}
	var sheets []xlsxSheet
	type definedName struct{ name, ref, local string }
	var names []definedName
	err = x.walk(book, func(d *xml.Decoder, se xml.StartElement) error {
		switch se.Name.Local {
		case "workbookPr":
			x.wb.Date1904 = attr(se, "date1904") == "1" || attr(se, "date1904") == "true"
		case "sheet":
			if len(sheets) >= MaxSheets {
				return &LimitError{What: "sheets", Limit: MaxSheets}
			}
			n, _ := strconv.Atoi(attr(se, "sheetId"))
			sheets = append(sheets, xlsxSheet{name: clipRunes(attr(se, "name"), MaxNameRunes), state: attr(se, "state"), rel: attr(se, "id"), number: n})
		case "definedName":
			if len(names) >= MaxNames {
				return &LimitError{What: "defined names", Limit: MaxNames}
			}
			name, local, hidden := attr(se, "name"), attr(se, "localSheetId"), attr(se, "hidden")
			ref, err := x.text(d, 8192)
			if err != nil {
				return err
			}
			if hidden != "1" && hidden != "true" && !strings.HasPrefix(name, "_xlnm.") && !strings.HasPrefix(name, "_") {
				names = append(names, definedName{clipRunes(name, MaxNameRunes), ref, local})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for id, t := range types {
		switch {
		case strings.HasSuffix(t, "/sharedStrings"):
			if err := x.readStrings(targets[id]); err != nil {
				return nil, err
			}
		case strings.HasSuffix(t, "/styles"):
			if err := x.readStyles(targets[id]); err != nil {
				return nil, err
			}
		}
	}
	x.wb.App = x.app()
	for _, s := range sheets {
		hidden := s.state == "hidden" || s.state == "veryHidden"
		sh := Sheet{Name: s.name, Number: s.number, Hidden: hidden}
		target, ok := targets[s.rel]
		if !hidden && ok && x.has(target) {
			if err := x.readSheet(target, &sh); err != nil {
				return nil, err
			}
		}
		x.wb.Sheets = append(x.wb.Sheets, sh)
	}
	for _, n := range names {
		sheetName, rect, ok := SplitRef(n.ref)
		if !ok || !rect.Valid() || strings.ContainsAny(n.ref, ",[") {
			continue
		}
		name := Name{Name: n.name, Sheet: sheetName, Rect: rect}
		if n.local != "" {
			i, err := strconv.Atoi(n.local)
			if err != nil || i < 0 || i >= len(sheets) {
				continue
			}
			name.Scope = sheets[i].name
		}
		if _, visible := x.wb.Sheet(sheetName); visible {
			x.wb.Names = append(x.wb.Names, name)
		}
	}
	return x.wb, nil
}

// app tells which program saved the file, from docProps/app.xml.
func (x *xlsxReader) app() string {
	if !x.has("docProps/app.xml") {
		return ""
	}
	var app string
	_ = x.walk("docProps/app.xml", func(d *xml.Decoder, se xml.StartElement) error {
		if se.Name.Local == "Application" {
			text, err := x.text(d, 200)
			app = text
			return err
		}
		return nil
	})
	switch {
	case strings.HasPrefix(app, "Microsoft"):
		return "excel"
	case strings.Contains(strings.ToLower(app), "libreoffice"):
		return "libreoffice"
	}
	return ""
}

// readStrings reads the shared strings: the text of each si, its runs joined, without phonetic guides.
func (x *xlsxReader) readStrings(name string) error {
	return x.walk(name, func(d *xml.Decoder, se xml.StartElement) error {
		if se.Name.Local != "si" {
			return nil
		}
		if len(x.strings) >= MaxStrings {
			return &LimitError{What: "shared strings", Limit: MaxStrings}
		}
		text, err := x.richText(d)
		if err != nil {
			return err
		}
		x.strings = append(x.strings, text)
		return nil
	})
}

// richText reads the text of an si or is element: every t, except those of a phonetic guide (rPh).
func (x *xlsxReader) richText(d *xml.Decoder) (string, error) {
	var b strings.Builder
	depth, inT, phonetic := 1, 0, 0
	for depth > 0 {
		tok, err := x.token(d)
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "t":
				inT++
			case "rPh":
				phonetic++
			}
		case xml.EndElement:
			depth--
			switch t.Name.Local {
			case "t":
				inT--
			case "rPh":
				phonetic--
			}
		case xml.CharData:
			if inT > 0 && phonetic == 0 && b.Len() < MaxCellText*4 {
				b.Write(t)
			}
		}
	}
	return clipRunes(unescapeX(b.String()), MaxCellText), nil
}

// unescapeX turns the escapes of OOXML for characters XML cannot carry, _x000D_, into the characters.
func unescapeX(s string) string {
	if !strings.Contains(s, "_x") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if i+7 <= len(s) && s[i] == '_' && s[i+1] == 'x' && s[i+6] == '_' {
			if v, err := strconv.ParseUint(s[i+2:i+6], 16, 16); err == nil {
				if v == '_' {
					b.WriteByte('_')
				} else if v >= 0x20 || v == '\t' || v == '\n' || v == '\r' {
					b.WriteRune(rune(v))
				}
				i += 6
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// readStyles reads the number format of each cell style.
func (x *xlsxReader) readStyles(name string) error {
	custom := map[int]string{}
	inXfs := false
	return x.walk(name, func(d *xml.Decoder, se xml.StartElement) error {
		switch se.Name.Local {
		case "numFmt":
			if len(custom) >= MaxFormat {
				return &LimitError{What: "number formats", Limit: MaxFormat}
			}
			id, err := strconv.Atoi(attr(se, "numFmtId"))
			if err == nil {
				custom[id] = clipRunes(attr(se, "formatCode"), MaxFormatCode)
			}
		case "cellXfs":
			inXfs = true
		case "cellStyleXfs", "dxfs":
			inXfs = false
			return x.skip(d)
		case "xf":
			if !inXfs {
				return nil
			}
			if len(x.styles) >= MaxStyles {
				return &LimitError{What: "cell styles", Limit: MaxStyles}
			}
			id, _ := strconv.Atoi(attr(se, "numFmtId"))
			x.styles = append(x.styles, x.formatIndex(id, custom))
			return x.skip(d)
		}
		return nil
	})
}

// builtInMark marks a built-in format whose code depends on the language, resolved when it shows (Workbook.Format).
const builtInMark = "\x00"

// formatIndex returns the index in wb.Formats of a format ID, adding its code once.
func (x *xlsxReader) formatIndex(id int, custom map[int]string) int {
	code, ok := custom[id]
	if !ok {
		switch {
		case id == 14 || id == 22 || (id >= 5 && id <= 8):
			code = builtInMark + strconv.Itoa(id)
		default:
			code, ok = builtIn[id]
			if !ok {
				code = "General"
			}
		}
	}
	if code == "General" || code == "" {
		return 0
	}
	if i := slices.Index(x.wb.Formats, code); i >= 0 {
		return i
	}
	x.wb.Formats = append(x.wb.Formats, code)
	return len(x.wb.Formats) - 1
}

// readSheet streams a worksheet: hidden columns, rows with their cells, merged areas, and the tables it holds.
func (x *xlsxReader) readSheet(name string, sh *Sheet) error {
	targets, _, err := x.relsOf(name)
	if err != nil {
		return err
	}
	var tableRels []string
	row := 0
	err = x.walk(name, func(d *xml.Decoder, se xml.StartElement) error {
		switch se.Name.Local {
		case "col":
			lo, err1 := strconv.Atoi(attr(se, "min"))
			hi, err2 := strconv.Atoi(attr(se, "max"))
			if err1 == nil && err2 == nil && truthy(attr(se, "hidden")) {
				for c := max(lo, 1); c <= min(hi, MaxCols); c++ {
					sh.HiddenCols = append(sh.HiddenCols, c)
				}
			}
		case "row":
			if r, err := strconv.Atoi(attr(se, "r")); err == nil {
				row = r
			} else {
				row++
			}
			if row < 1 || row > MaxRows {
				return &LimitError{What: "rows", Limit: MaxRows}
			}
			if truthy(attr(se, "hidden")) {
				sh.HiddenRows = append(sh.HiddenRows, row)
			}
			return x.readRow(d, sh, row)
		case "mergeCell":
			if len(sh.Merges) >= MaxMerges {
				return &LimitError{What: "merged areas", Limit: MaxMerges}
			}
			if rect, ok := ParseRect(attr(se, "ref")); ok && rect.Cells() > 1 {
				sh.Merges = append(sh.Merges, rect)
			}
		case "tablePart":
			tableRels = append(tableRels, attr(se, "id"))
		case "sheetPr", "dimension", "sheetViews", "sheetFormatPr", "cols", "sheetData", "mergeCells", "tableParts", "worksheet":
		default:
			// Everything else of a sheet (conditional formats, drawings, pictures, controls) is not read.
			return x.skip(d)
		}
		return nil
	})
	if err != nil {
		return err
	}
	slices.Sort(sh.HiddenRows)
	sh.HiddenRows = slices.Compact(sh.HiddenRows)
	slices.Sort(sh.HiddenCols)
	sh.HiddenCols = slices.Compact(sh.HiddenCols)
	for _, rel := range tableRels {
		target, ok := targets[rel]
		if !ok || !x.has(target) {
			continue
		}
		if err := x.readTable(target, sh.Name); err != nil {
			return err
		}
	}
	return nil
}

func truthy(s string) bool { return s == "1" || s == "true" }

// readRow reads the cells of a row that was just opened.
func (x *xlsxReader) readRow(d *xml.Decoder, sh *Sheet, row int) error {
	r := Row{N: row}
	col := 0
	for {
		tok, err := x.token(d)
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.EndElement:
			if t.Name.Local == "row" {
				if len(r.Cells) > 0 {
					sh.Rows = append(sh.Rows, r)
				}
				return nil
			}
		case xml.StartElement:
			if t.Name.Local != "c" {
				if err := x.skip(d); err != nil {
					return err
				}
				continue
			}
			if ref := attr(t, "r"); ref != "" {
				cr, cc, ok := ParseCell(ref)
				if !ok {
					return &LimitError{What: "columns", Limit: MaxCols}
				}
				_ = cr // the row of the row element counts; a cell that names another row is read into this one
				col = cc
			} else {
				col++
			}
			if col < 1 || col > MaxCols {
				return &LimitError{What: "columns", Limit: MaxCols}
			}
			cell, ok, err := x.readCell(d, t, col)
			if err != nil {
				return err
			}
			if ok {
				x.cells++
				if x.cells > MaxCells {
					return &LimitError{What: "cells", Limit: MaxCells}
				}
				r.Cells = append(r.Cells, cell)
			}
		}
	}
}

// readCell reads a cell element: its stored value by its type, and whether it has content at all.
func (x *xlsxReader) readCell(d *xml.Decoder, se xml.StartElement, col int) (Cell, bool, error) {
	cell := Cell{Col: col}
	if s, err := strconv.Atoi(attr(se, "s")); err == nil && s >= 0 && s < len(x.styles) {
		cell.Format = x.styles[s]
	}
	typ := attr(se, "t")
	var value string
	hasValue, hasFormula := false, false
	depth := 1
	for depth > 0 {
		tok, err := x.token(d)
		if err != nil {
			return cell, false, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "v":
				v, err := x.text(d, MaxCellText)
				if err != nil {
					return cell, false, err
				}
				value, hasValue = v, v != ""
			case "f":
				hasFormula = true
				if err := x.skip(d); err != nil {
					return cell, false, err
				}
			case "is":
				v, err := x.richText(d)
				if err != nil {
					return cell, false, err
				}
				value, hasValue = v, true
			default:
				depth++
			}
		case xml.EndElement:
			depth--
		}
	}
	switch {
	case !hasValue && hasFormula:
		cell.Kind = Uncomputed
		return cell, true, nil
	case !hasValue:
		return cell, false, nil
	}
	switch typ {
	case "s":
		i, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || i < 0 || i >= len(x.strings) {
			return cell, false, nil
		}
		cell.Kind, cell.Value = Text, x.strings[i]
	case "str", "inlineStr":
		cell.Kind, cell.Value = Text, unescapeX(value)
	case "b":
		cell.Kind, cell.Value = Bool, strings.TrimSpace(value)
	case "e":
		cell.Kind, cell.Value = Error, strings.TrimSpace(value)
	case "d":
		serial, ok := isoSerial(strings.TrimSpace(value), x.wb.Date1904)
		if !ok {
			cell.Kind, cell.Value = Text, value
			break
		}
		cell.Kind, cell.Value = Number, strconv.FormatFloat(serial, 'g', -1, 64)
	default:
		v := strings.TrimSpace(value)
		if f, err := strconv.ParseFloat(v, 64); err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			cell.Kind, cell.Value = Text, value
		} else {
			cell.Kind, cell.Value = Number, v
		}
	}
	return cell, true, nil
}

// isoSerial turns a date of ISO 8601, as a cell of type d stores it, into a serial number of the date system.
func isoSerial(s string, date1904 bool) (float64, bool) {
	var t time.Time
	var err error
	for _, layout := range []string{"2006-01-02T15:04:05.999999999Z07:00", "2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err = time.Parse(layout, s); err == nil {
			break
		}
	}
	if err != nil {
		return 0, false
	}
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	if date1904 {
		base = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	days := t.Sub(base).Hours() / 24
	if !date1904 && days < 61 {
		days-- // before 1 March 1900, Excel counts its leap day of 1900 not yet
	}
	return days, days >= 0
}

// readTable reads an Excel table of a sheet.
func (x *xlsxReader) readTable(name, sheetName string) error {
	if len(x.wb.Tables) >= MaxTables {
		return &LimitError{What: "tables", Limit: MaxTables}
	}
	var t Table
	err := x.walk(name, func(d *xml.Decoder, se xml.StartElement) error {
		switch se.Name.Local {
		case "table":
			t.Name = clipRunes(attr(se, "displayName"), MaxNameRunes)
			if t.Name == "" {
				t.Name = clipRunes(attr(se, "name"), MaxNameRunes)
			}
			t.Rect, _ = ParseRect(attr(se, "ref"))
			t.Header = 1
			if h := attr(se, "headerRowCount"); h != "" {
				t.Header, _ = strconv.Atoi(h)
			}
			t.Totals, _ = strconv.Atoi(attr(se, "totalsRowCount"))
		case "tableColumn":
			if len(t.Columns) < MaxCols {
				t.Columns = append(t.Columns, clipRunes(attr(se, "name"), MaxNameRunes))
			}
			return x.skip(d)
		case "autoFilter", "sortState", "tableStyleInfo", "extLst":
			return x.skip(d)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if t.Name != "" && t.Rect.Valid() && t.Header >= 0 && t.Header <= 1 && t.Totals >= 0 {
		t.Sheet = sheetName
		x.wb.Tables = append(x.wb.Tables, t)
	}
	return nil
}
