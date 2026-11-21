// SPDX-License-Identifier: MIT

// Package sheet reads spreadsheets into tables (ADR-31): SpreadsheetML (.xlsx, .xlsm), OpenDocument (.ods) and CSV, read
// only, with the values their program stored (no formula is computed), number formats rendered in the reader's
// language, and hidden sheets, rows and columns left out. Every file is treated as an attack: the reader stops at limits
// of size, count and time, and gives a reason instead of failing.
package sheet

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Limits of a workbook, against files made to exhaust the reader (ADR-31).
const (
	MaxSheets    = 256
	MaxCells     = 1_000_000 // cells with content in a workbook
	MaxRows      = 1_048_576 // Excel's rows
	MaxCols      = 16_384    // Excel's columns
	MaxStrings   = 1_000_000 // shared strings
	MaxCellText  = 32_767    // characters in a cell, Excel's limit
	MaxMerges    = 100_000
	MaxNames     = 10_000
	MaxStyles    = 65_536
	MaxFormat    = 1_000 // custom number formats
	MaxTables    = 10_000
	MaxNameRunes = 255
	// MaxFormatCode is the longest format code that is read; Excel's limit is 255 characters.
	MaxFormatCode = 255
	// MaxGridCols is the most columns a grid shows, and the widest anchor of a range: a file with one cell far out to
	// the right must not make every view of it as wide as Excel's grid.
	MaxGridCols = 256
	// MaxArea is the most cells a rectangle written out whole (CSV) may cover, empty ones included.
	MaxArea = 1_000_000
)

// Kinds of a cell.
const (
	Number = 'n'
	Text   = 's'
	Bool   = 'b'
	Error  = 'e'
	// Uncomputed is a formula stored without its result.
	Uncomputed = 'f'
	// Shown is a cell whose text is what its program displayed, as OpenDocument stores it; ShownNumber the same for a
	// number, a date or a time.
	Shown       = 'd'
	ShownNumber = 'm'
)

// ErrUnreadable is a file that is no spreadsheet the reader knows, or a broken one.
var ErrUnreadable = errors.New("this file cannot be read as a table")

// LimitError is a workbook that passes a limit.
type LimitError struct {
	What  string
	Limit int
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("this file is too large to show as tables: more than %d %s", e.Limit, e.What)
}

// Workbook is what a spreadsheet holds that UnderDoc shows: its sheets, names and tables.
type Workbook struct {
	Sheets []Sheet `json:"sheets"`
	Names  []Name  `json:"names,omitempty"`
	Tables []Table `json:"tables,omitempty"`
	// Formats are the format codes, by index, that cells refer to; 0 is General.
	Formats  []string `json:"formats,omitempty"`
	Date1904 bool     `json:"date1904,omitempty"`
	// App is the program that saved the file: "excel", "libreoffice" or "".
	App string `json:"app,omitempty"`
}

// Sheet is one sheet. A hidden sheet keeps its name and number, never its cells.
type Sheet struct {
	Name string `json:"name"`
	// Number is the internal number of the sheet (sheetId of SpreadsheetML), 0 when the format has none.
	Number int   `json:"number,omitempty"`
	Hidden bool  `json:"hidden,omitempty"`
	Rows   []Row `json:"rows,omitempty"`
	// HiddenRows and HiddenCols list hidden rows and columns, 1-based, sorted.
	HiddenRows []int  `json:"hiddenRows,omitempty"`
	HiddenCols []int  `json:"hiddenCols,omitempty"`
	Merges     []Rect `json:"merges,omitempty"`
}

// Row is a row with cells, 1-based; rows without content are left out.
type Row struct {
	N     int    `json:"n"`
	Cells []Cell `json:"cells"`
}

// Cell is a cell with content, its column 1-based.
type Cell struct {
	Col  int  `json:"c"`
	Kind byte `json:"k"`
	// Value is the stored value: a number as written in the file, a text, "1"/"0", an error such as #DIV/0!, or what
	// the program displayed (Shown).
	Value  string `json:"v,omitempty"`
	Format int    `json:"f,omitempty"`
}

// Rect is a rectangle of cells, 1-based and inclusive.
type Rect struct {
	R1 int `json:"r1"`
	C1 int `json:"c1"`
	R2 int `json:"r2"`
	C2 int `json:"c2"`
}

// Name is a defined name that points at one rectangle of a sheet.
type Name struct {
	Name  string `json:"name"`
	Sheet string `json:"sheet"`
	Rect  Rect   `json:"rect"`
	// Scope is the sheet a name of a sheet belongs to; "" for a name of the workbook.
	Scope string `json:"scope,omitempty"`
}

// Table is an Excel table (ListObject) or a database range of OpenDocument.
type Table struct {
	Name    string `json:"name"`
	Sheet   string `json:"sheet"`
	Rect    Rect   `json:"rect"`
	Header  int    `json:"header"` // rows of the head
	Totals  int    `json:"totals,omitempty"`
	Columns []string
}

// Sheet returns the visible sheet of the name.
func (w *Workbook) Sheet(name string) (*Sheet, bool) {
	for i := range w.Sheets {
		if w.Sheets[i].Name == name {
			return &w.Sheets[i], !w.Sheets[i].Hidden
		}
	}
	return nil, false
}

// Visible lists the names of the visible sheets.
func (w *Workbook) Visible() []string {
	var out []string
	for _, s := range w.Sheets {
		if !s.Hidden {
			out = append(out, s.Name)
		}
	}
	return out
}

// Format returns the code of a format index in a language: a built-in format that depends on the language, such as the
// short date, is resolved here.
func (w *Workbook) Format(i int, l Lang) string {
	if i <= 0 || i >= len(w.Formats) {
		return "General"
	}
	code := w.Formats[i]
	if strings.HasPrefix(code, builtInMark) {
		id, _ := strconv.Atoi(code[len(builtInMark):])
		code, _ = BuiltInCode(id, l)
	}
	return code
}

// Display is the text a cell shows in a language.
func (w *Workbook) Display(c Cell, l Lang) string {
	switch c.Kind {
	case Number:
		v, err := strconv.ParseFloat(c.Value, 64)
		if err != nil {
			return c.Value
		}
		return FormatNumber(v, w.Format(c.Format, l), l, w.Date1904)
	case Text:
		return FormatText(c.Value, w.Format(c.Format, l))
	case Bool:
		if c.Value == "1" || strings.EqualFold(c.Value, "true") {
			return l.True
		}
		return l.False
	}
	return c.Value
}

// Used is the rectangle from A1 to the last row and column with content; zero for an empty sheet.
func (s *Sheet) Used() Rect {
	var r Rect
	for _, row := range s.Rows {
		for _, c := range row.Cells {
			r.R2 = max(r.R2, row.N)
			r.C2 = max(r.C2, c.Col)
		}
	}
	for _, m := range s.Merges {
		if r.R2 > 0 {
			r.R2, r.C2 = max(r.R2, m.R2), max(r.C2, m.C2)
		}
	}
	if r.R2 == 0 {
		return Rect{}
	}
	r.R1, r.C1 = 1, 1
	return r
}

// --- Addresses

// ColName is the letters of a column: 1 is A, 27 is AA.
func ColName(c int) string {
	var b []byte
	for c > 0 {
		c--
		b = append([]byte{byte('A' + c%26)}, b...)
		c /= 26
	}
	return string(b)
}

// CellName is the address of a cell, such as B7.
func CellName(r, c int) string { return ColName(c) + strconv.Itoa(r) }

func (r Rect) String() string {
	if r.R1 == r.R2 && r.C1 == r.C2 {
		return CellName(r.R1, r.C1)
	}
	return CellName(r.R1, r.C1) + ":" + CellName(r.R2, r.C2)
}

// Valid tells whether a rectangle lies within Excel's grid and is not upside down.
func (r Rect) Valid() bool {
	return r.R1 >= 1 && r.C1 >= 1 && r.R2 >= r.R1 && r.C2 >= r.C1 && r.R2 <= MaxRows && r.C2 <= MaxCols
}

// Contains tells whether a cell lies in the rectangle.
func (r Rect) Contains(row, col int) bool {
	return row >= r.R1 && row <= r.R2 && col >= r.C1 && col <= r.C2
}

// Cells is how many cells the rectangle covers.
func (r Rect) Cells() int { return (r.R2 - r.R1 + 1) * (r.C2 - r.C1 + 1) }

// ParseCell reads a cell address such as B7 or $B$7.
func ParseCell(s string) (row, col int, ok bool) {
	s = strings.ReplaceAll(s, "$", "")
	i := 0
	for i < len(s) && (s[i]|0x20) >= 'a' && (s[i]|0x20) <= 'z' {
		col = col*26 + int((s[i]|0x20)-'a'+1)
		if col > MaxCols {
			return 0, 0, false
		}
		i++
	}
	if i == 0 || i == len(s) || i > 3 {
		return 0, 0, false
	}
	row, err := strconv.Atoi(s[i:])
	if err != nil || row < 1 || row > MaxRows || s[i] == '+' || s[i] == '-' {
		return 0, 0, false
	}
	return row, col, true
}

// ParseRect reads a range such as A1:D20, $A$1:$D$20 or a single cell; the corners may come in any order.
func ParseRect(s string) (Rect, bool) {
	a, b, two := strings.Cut(s, ":")
	r1, c1, ok := ParseCell(a)
	if !ok {
		return Rect{}, false
	}
	r2, c2 := r1, c1
	if two {
		if r2, c2, ok = ParseCell(b); !ok {
			return Rect{}, false
		}
	}
	return Rect{R1: min(r1, r2), C1: min(c1, c2), R2: max(r1, r2), C2: max(c1, c2)}, true
}

// SplitRef splits a reference such as 'My sheet'!$A$1:$B$2 or Sheet1!A1 into its sheet and its range.
func SplitRef(ref string) (sheetName string, rect Rect, ok bool) {
	ref = strings.TrimSpace(ref)
	at := strings.LastIndexByte(ref, '!')
	if at <= 0 {
		return "", Rect{}, false
	}
	sheetName, rest := ref[:at], ref[at+1:]
	if strings.HasPrefix(sheetName, "'") {
		if len(sheetName) < 2 || !strings.HasSuffix(sheetName, "'") {
			return "", Rect{}, false
		}
		sheetName = strings.ReplaceAll(sheetName[1:len(sheetName)-1], "''", "'")
	} else if strings.ContainsAny(sheetName, " '[]") {
		return "", Rect{}, false
	}
	rect, ok = ParseRect(rest)
	return sheetName, rect, ok && sheetName != ""
}

// --- A grid of cells to show

// Grid is a rectangle of a sheet as it shows: visible rows and columns only, merged cells spanning, text in a language.
type Grid struct {
	Rows []GridRow `json:"rows"`
	// Cols are the visible columns, 1-based.
	Cols []int `json:"cols"`
	// More is how many visible rows follow beyond the limit asked for; MoreCols how many visible columns are left out
	// beyond MaxGridCols.
	More     int `json:"more,omitempty"`
	MoreCols int `json:"moreCols,omitempty"`
}

// GridRow is one visible row.
type GridRow struct {
	N     int        `json:"n"`
	Cells []GridCell `json:"cells"`
}

// GridCell is one cell that shows; a cell covered by a merge does not.
type GridCell struct {
	Col  int    `json:"c"`
	Text string `json:"t,omitempty"`
	Kind byte   `json:"k,omitempty"`
	// RowSpan and ColSpan count visible rows and columns.
	RowSpan int `json:"rs,omitempty"`
	ColSpan int `json:"cs,omitempty"`
}

// Grid returns the visible part of a rectangle of the sheet, at most limit rows from the visible row skip on (limit 0:
// all), with text in the language.
func (w *Workbook) Grid(s *Sheet, rect Rect, l Lang, skip, limit int) Grid {
	hiddenRow := toSet(s.HiddenRows)
	hiddenCol := toSet(s.HiddenCols)
	g := Grid{Cols: []int{}}
	for c := rect.C1; c <= rect.C2; c++ {
		if hiddenCol[c] {
			continue
		}
		if len(g.Cols) == MaxGridCols {
			g.MoreCols++
			continue
		}
		g.Cols = append(g.Cols, c)
	}
	if g.MoreCols > 0 {
		rect.C2 = g.Cols[len(g.Cols)-1]
	}
	// The visible rows of the rectangle, as numbers; a sheet has at most a million, so this is a slice of ints at most.
	cells := map[int][]Cell{}
	for _, row := range s.Rows {
		if row.N >= rect.R1 && row.N <= rect.R2 {
			cells[row.N] = row.Cells
		}
	}
	type span struct{ rows, cols int }
	starts := map[[2]int]span{}
	covered := map[[2]int]bool{}
	for _, m := range s.Merges {
		m = clip(m, rect)
		if m.R1 == 0 {
			continue
		}
		// Spans count visible rows and columns; a merge whose first cell is hidden starts at its first visible cell.
		var vr, vc []int
		for r := m.R1; r <= m.R2; r++ {
			if !hiddenRow[r] {
				vr = append(vr, r)
			}
		}
		for c := m.C1; c <= m.C2; c++ {
			if !hiddenCol[c] {
				vc = append(vc, c)
			}
		}
		if len(vr) == 0 || len(vc) == 0 {
			continue
		}
		starts[[2]int{vr[0], vc[0]}] = span{len(vr), len(vc)}
		for _, r := range vr {
			for _, c := range vc {
				if r != vr[0] || c != vc[0] {
					covered[[2]int{r, c}] = true
				}
			}
		}
		// The text of a merge is in its first cell, even when that is hidden.
		if (vr[0] != m.R1 || vc[0] != m.C1) && cellAt(cells[m.R1], m.C1) != nil {
			first := *cellAt(cells[m.R1], m.C1)
			first.Col = vc[0]
			cells[vr[0]] = append(withoutCol(cells[vr[0]], vc[0]), first)
		}
	}
	visible := 0
	for r := rect.R1; r <= rect.R2; r++ {
		if hiddenRow[r] {
			continue
		}
		if visible < skip {
			visible++
			continue
		}
		if limit > 0 && visible >= skip+limit {
			g.More++
			continue
		}
		visible++
		row := GridRow{N: r, Cells: []GridCell{}}
		for _, c := range g.Cols {
			if covered[[2]int{r, c}] {
				continue
			}
			gc := GridCell{Col: c}
			if sp, ok := starts[[2]int{r, c}]; ok {
				if sp.rows > 1 {
					gc.RowSpan = sp.rows
				}
				if sp.cols > 1 {
					gc.ColSpan = sp.cols
				}
			}
			if cell := cellAt(cells[r], c); cell != nil {
				gc.Text, gc.Kind = w.Display(*cell, l), cell.Kind
			}
			row.Cells = append(row.Cells, gc)
		}
		g.Rows = append(g.Rows, row)
	}
	if g.Rows == nil {
		g.Rows = []GridRow{}
	}
	return g
}

func clip(m, r Rect) Rect {
	out := Rect{R1: max(m.R1, r.R1), C1: max(m.C1, r.C1), R2: min(m.R2, r.R2), C2: min(m.C2, r.C2)}
	if out.R1 > out.R2 || out.C1 > out.C2 {
		return Rect{}
	}
	return out
}

func cellAt(cells []Cell, col int) *Cell {
	for i := range cells {
		if cells[i].Col == col {
			return &cells[i]
		}
	}
	return nil
}

func withoutCol(cells []Cell, col int) []Cell {
	out := cells[:0:0]
	for _, c := range cells {
		if c.Col != col {
			out = append(out, c)
		}
	}
	return out
}

func toSet(xs []int) map[int]bool {
	m := make(map[int]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// Anchor returns the texts of one row of a rectangle in English, hidden columns included, at most MaxGridCols of them: what a range is found again by
// when its file changes (ADR-31). It is never shown.
func (w *Workbook) Anchor(s *Sheet, row, c1, c2 int) []string {
	c2 = min(c2, c1+MaxGridCols-1) // at most as wide as a grid shows
	var cells []Cell
	for _, r := range s.Rows {
		if r.N == row {
			cells = r.Cells
			break
		}
	}
	out := make([]string, 0, c2-c1+1)
	for c := c1; c <= c2; c++ {
		text := ""
		if cell := cellAt(cells, c); cell != nil {
			text = w.Display(*cell, English)
		}
		out = append(out, text)
	}
	return out
}

// FindAnchor looks for the rows of a sheet that hold the anchor, as wide as it is, anywhere: the top left cell of each
// place, at most two (more than one is no answer). An anchor of empty texts is found nowhere.
func (w *Workbook) FindAnchor(s *Sheet, anchor []string) [][2]int {
	first := -1
	for i, a := range anchor {
		if a != "" {
			first = i
			break
		}
	}
	if first < 0 {
		return nil
	}
	var found [][2]int
	for _, r := range s.Rows {
		for _, c := range r.Cells {
			start := c.Col - first
			if start < 1 || start+len(anchor)-1 > MaxCols || w.Display(c, English) != anchor[first] {
				continue
			}
			if slices.Equal(w.Anchor(s, r.N, start, start+len(anchor)-1), anchor) {
				found = append(found, [2]int{r.N, start})
				if len(found) > 1 {
					return found
				}
			}
		}
	}
	return found
}

// TextSection is a run of rows of a visible sheet as text for the search (ADR-31): the sheet, the range it covers, and
// its cells in English, a row a line.
type TextSection struct {
	Sheet, Range, Text string
}

// SearchText returns the visible cells of the visible sheets as sections of at most maxWords words, at most maxBytes of
// text in all; hidden sheets, rows and columns give nothing. It goes through the cells there are, never through the
// empty cells of a rectangle, so a lone cell far out costs nothing.
func (w *Workbook) SearchText(maxWords, maxBytes int) []TextSection {
	var out []TextSection
	total := 0
	for i := range w.Sheets {
		s := &w.Sheets[i]
		if s.Hidden || len(s.Rows) == 0 {
			continue
		}
		hiddenRow, hiddenCol := toSet(s.HiddenRows), toSet(s.HiddenCols)
		used := s.Used()
		var text strings.Builder
		words, first, last := 0, 0, 0
		flush := func() {
			if text.Len() > 0 {
				out = append(out, TextSection{Sheet: s.Name, Range: Rect{R1: first, C1: used.C1, R2: last, C2: used.C2}.String(), Text: text.String()})
			}
			text.Reset()
			words = 0
		}
		for _, row := range s.Rows {
			if hiddenRow[row.N] {
				continue
			}
			var cells []string
			for _, c := range row.Cells {
				if hiddenCol[c.Col] {
					continue
				}
				if t := w.Display(c, English); t != "" {
					cells = append(cells, t)
				}
			}
			if len(cells) == 0 {
				continue
			}
			line := strings.Join(cells, " · ")
			if total+len(line) > maxBytes {
				flush()
				return out
			}
			if words > 0 && words+len(strings.Fields(line)) > maxWords {
				flush()
			}
			if words == 0 {
				first = row.N
			}
			last = row.N
			text.WriteString(line + "\n")
			words += len(strings.Fields(line))
			total += len(line) + 1
		}
		flush()
	}
	return out
}
