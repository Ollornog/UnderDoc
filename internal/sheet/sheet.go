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
	"sort"
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
	// MaxText is the most bytes of text the cells of a workbook hold together, a text counted as often as it stands: a
	// shared string or a repeated cell of a few bytes in the file must not become gigabytes once written out.
	MaxText = 100 << 20
	// MaxHidden is the most hidden rows and columns a workbook keeps, those beyond the last row and column with content
	// not counted.
	MaxHidden = 2 * MaxRows
	// MaxGridText is the most bytes of text one page of a grid holds; the rows beyond it are counted as more.
	MaxGridText = 8 << 20
	// MaxAnchorText is the most characters of a cell an anchor keeps.
	MaxAnchorText = 256
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

// memory is about how many bytes a workbook takes in memory: its texts, and what a cell, a row, a hidden row or column,
// a merged area, a name and a table take beside them (B-2). It is an estimate for the cache, not a measure.
func (w *Workbook) memory() int {
	const cell, row, number, rect, entry = 48, 32, 8, 32, 64
	n := 256
	for i := range w.Sheets {
		s := &w.Sheets[i]
		n += entry + len(s.Name) + rect*len(s.Merges) + number*(len(s.HiddenRows)+len(s.HiddenCols))
		for _, r := range s.Rows {
			n += row + cell*len(r.Cells)
			for _, c := range r.Cells {
				n += len(c.Value)
			}
		}
	}
	for _, x := range w.Names {
		n += entry + len(x.Name) + len(x.Sheet) + len(x.Scope)
	}
	for _, t := range w.Tables {
		n += entry + len(t.Name) + len(t.Sheet)
		for _, c := range t.Columns {
			n += 16 + len(c)
		}
	}
	for _, f := range w.Formats {
		n += 16 + len(f)
	}
	return n
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

// textLimit names MaxText in a LimitError.
const textLimit = "bytes of text in cells"

// trimHidden drops the hidden rows and columns beyond the last row and column with content, which change nothing that
// shows (LibreOffice writes the rows below the content hidden up to the end of the sheet), and returns how many are kept.
func (s *Sheet) trimHidden() int {
	used := s.Used()
	if i, _ := slices.BinarySearch(s.HiddenRows, used.R2+1); i < len(s.HiddenRows) {
		s.HiddenRows = slices.Clone(s.HiddenRows[:i])
	}
	if i, _ := slices.BinarySearch(s.HiddenCols, used.C2+1); i < len(s.HiddenCols) {
		s.HiddenCols = slices.Clone(s.HiddenCols[:i])
	}
	if len(s.HiddenRows) == 0 {
		s.HiddenRows = nil
	}
	if len(s.HiddenCols) == 0 {
		s.HiddenCols = nil
	}
	return len(s.HiddenRows) + len(s.HiddenCols)
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
// all), with text in the language. It costs what the page costs: merged areas are cut at the edges of the page, a cell
// is looked up once, and a page stops at MaxGridText bytes of text, the rows beyond counted as more.
func (w *Workbook) Grid(s *Sheet, rect Rect, l Lang, skip, limit int) Grid {
	g := Grid{Cols: []int{}}
	for c := rect.C1; c <= rect.C2; c++ {
		if hidden(s.HiddenCols, c) {
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
	// The visible rows of the page, in order.
	var page []int
	visible := 0
	for r := rect.R1; r <= rect.R2; r++ {
		if hidden(s.HiddenRows, r) {
			continue
		}
		switch {
		case visible < skip:
		case limit > 0 && visible >= skip+limit:
			g.More++
		default:
			page = append(page, r)
		}
		visible++
	}
	g.Rows = []GridRow{}
	if len(page) == 0 || len(g.Cols) == 0 {
		g.Rows = make([]GridRow, len(page))
		for i, r := range page {
			g.Rows[i] = GridRow{N: r, Cells: []GridCell{}}
		}
		return g
	}
	at := make(map[int]int, len(page)) // row → its place on the page
	for i, r := range page {
		at[r] = i
	}
	colAt := make(map[int]int, len(g.Cols))
	for j, c := range g.Cols {
		colAt[c] = j
	}
	// The cells of the page, each found once: cells[i*width+j] is the cell of page[i] and g.Cols[j].
	width := len(g.Cols)
	cells := make([]*Cell, len(page)*width)
	var byRow map[int][]Cell // every row by its number, made only when a merge needs a cell off the page
	for k := range s.Rows {
		i, ok := at[s.Rows[k].N]
		if !ok {
			continue
		}
		row := s.Rows[k].Cells
		for n := range row {
			if j, ok := colAt[row[n].Col]; ok && cells[i*width+j] == nil {
				cells[i*width+j] = &row[n]
			}
		}
	}
	type span struct{ rows, cols int }
	starts := map[int]span{}
	covered := map[int]bool{}
	// Merges are cut at the edges of the page; merges over cells already covered, as no program writes them, are left
	// out, and the work on merges stays within a few times the cells of the page.
	work := 4 * len(page) * width
	for _, m := range s.Merges {
		m = clip(m, rect)
		if m.R1 == 0 || work <= 0 {
			continue
		}
		i0, _ := slices.BinarySearch(page, m.R1)
		i1, _ := slices.BinarySearch(page, m.R2+1)
		j0, _ := slices.BinarySearch(g.Cols, m.C1)
		j1, _ := slices.BinarySearch(g.Cols, m.C2+1)
		if i0 >= i1 || j0 >= j1 {
			continue
		}
		start := i0*width + j0
		if _, taken := starts[start]; taken || covered[start] {
			continue
		}
		starts[start] = span{i1 - i0, j1 - j0}
		for i := i0; i < i1 && work > 0; i++ {
			for j := j0; j < j1; j++ {
				if i != i0 || j != j0 {
					covered[i*width+j] = true
				}
				work--
			}
		}
		// The text of a merge is in its first cell; it shows in the first visible cell of the merge, even when the first
		// cell is hidden, and only on the page where the merge starts to show.
		first, firstCol := page[i0], g.Cols[j0]
		if (first != m.R1 || firstCol != m.C1) && visibleBetween(s.HiddenRows, m.R1, first) == 0 && visibleBetween(s.HiddenCols, m.C1, firstCol) == 0 {
			if byRow == nil {
				byRow = make(map[int][]Cell, len(s.Rows))
				for _, r := range s.Rows {
					byRow[r.N] = r.Cells
				}
			}
			if c := cellAt(byRow[m.R1], m.C1); c != nil {
				cells[start] = c
			}
		}
	}
	text := 0
	for i, r := range page {
		if text > MaxGridText {
			g.More += len(page) - i
			break
		}
		row := GridRow{N: r, Cells: []GridCell{}}
		for j, c := range g.Cols {
			if covered[i*width+j] {
				continue
			}
			gc := GridCell{Col: c}
			if sp, ok := starts[i*width+j]; ok {
				if sp.rows > 1 {
					gc.RowSpan = sp.rows
				}
				if sp.cols > 1 {
					gc.ColSpan = sp.cols
				}
			}
			if cell := cells[i*width+j]; cell != nil {
				gc.Text, gc.Kind = w.Display(*cell, l), cell.Kind
				text += len(gc.Text)
			}
			row.Cells = append(row.Cells, gc)
		}
		g.Rows = append(g.Rows, row)
	}
	// A page cut short: no merge spans beyond its last row.
	for i := range g.Rows {
		for k := range g.Rows[i].Cells {
			if c := &g.Rows[i].Cells[k]; c.RowSpan > len(g.Rows)-i {
				c.RowSpan = len(g.Rows) - i
				if c.RowSpan == 1 {
					c.RowSpan = 0
				}
			}
		}
	}
	return g
}

// shownRows are the rows with content as the grid shows them: the text of a merged area whose first cell is hidden stands
// in the first visible cell of the area (Grid, and Excel with a merged cell partly hidden), so the search and a CSV find
// it where it shows (B-2). Without such an area they are the rows of the sheet.
func (s *Sheet) shownRows() []Row {
	shown := s.mergedShown()
	if len(shown) == 0 {
		return s.Rows
	}
	rows := slices.Clone(s.Rows)
	at := make(map[int]int, len(rows))
	for i, r := range rows {
		at[r.N] = i
	}
	byCol := func(a, b Cell) int { return a.Col - b.Col }
	for n, extra := range shown {
		// The text of the area comes first, so it wins over a cell of its own the file may keep under it.
		cells := slices.Clone(extra)
		if i, ok := at[n]; ok {
			cells = append(cells, rows[i].Cells...)
		}
		slices.SortStableFunc(cells, byCol)
		cells = slices.CompactFunc(cells, func(a, b Cell) bool { return a.Col == b.Col })
		if i, ok := at[n]; ok {
			rows[i].Cells = cells
		} else {
			rows = append(rows, Row{N: n, Cells: cells})
		}
	}
	slices.SortStableFunc(rows, func(a, b Row) int { return a.N - b.N })
	return rows
}

// mergedShown finds the merged areas whose first cell is hidden and holds something, by the row of their first visible
// cell: the cell, with the column it shows in.
func (s *Sheet) mergedShown() map[int][]Cell {
	var out, byRow map[int][]Cell
	for _, m := range s.Merges {
		if !hidden(s.HiddenRows, m.R1) && !hidden(s.HiddenCols, m.C1) {
			continue
		}
		r, c := firstVisible(s.HiddenRows, m.R1, m.R2), firstVisible(s.HiddenCols, m.C1, m.C2)
		if r == 0 || c == 0 {
			continue
		}
		if byRow == nil {
			byRow = make(map[int][]Cell, len(s.Rows))
			for _, row := range s.Rows {
				byRow[row.N] = row.Cells
			}
		}
		cell := cellAt(byRow[m.R1], m.C1)
		if cell == nil {
			continue
		}
		moved := *cell
		moved.Col = c
		if out == nil {
			out = map[int][]Cell{}
		}
		out[r] = append(out[r], moved)
	}
	return out
}

// firstVisible is the first row or column from a to b that is not in a sorted list of hidden ones, or 0. A run of hidden
// ones is skipped in one step: within it, the value less its place in the list stays the same.
func firstVisible(sorted []int, a, b int) int {
	i, found := slices.BinarySearch(sorted, a)
	if !found {
		return a
	}
	key := sorted[i] - i
	j := i + sort.Search(len(sorted)-i, func(k int) bool { return sorted[i+k]-(i+k) != key })
	if n := a + j - i; n <= b {
		return n
	}
	return 0
}

// hidden tells whether a row or column is in a sorted list of hidden ones.
func hidden(sorted []int, n int) bool {
	_, found := slices.BinarySearch(sorted, n)
	return found
}

// visibleBetween counts the rows or columns from a to b, b not included, that are not in a sorted list of hidden ones.
func visibleBetween(sorted []int, a, b int) int {
	if b <= a {
		return 0
	}
	lo, _ := slices.BinarySearch(sorted, a)
	hi, _ := slices.BinarySearch(sorted, b)
	return b - a - (hi - lo)
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

// Anchor returns the texts of one row of a rectangle in English, hidden columns included, at most MaxGridCols of them
// and at most MaxAnchorText characters of each: what a range is found again by when its file changes (ADR-31). It is
// never shown.
func (w *Workbook) Anchor(s *Sheet, row, c1, c2 int) []string {
	c2 = min(c2, c1+MaxGridCols-1) // at most as wide as a grid shows
	var cells []Cell
	for _, r := range s.Rows {
		if r.N == row {
			cells = r.Cells
			break
		}
	}
	out := make([]string, c2-c1+1)
	for _, c := range cells {
		if c.Col >= c1 && c.Col <= c2 {
			out[c.Col-c1] = w.anchorText(c)
		}
	}
	return out
}

func (w *Workbook) anchorText(c Cell) string { return clipRunes(w.Display(c, English), MaxAnchorText) }

// ClipAnchor cuts an anchor kept before anchors were short to what Anchor makes now: at most MaxGridCols texts of at
// most MaxAnchorText characters.
func ClipAnchor(anchor []string) []string {
	out := slices.Clone(anchor[:min(len(anchor), MaxGridCols)])
	for i := range out {
		out[i] = clipRunes(out[i], MaxAnchorText)
	}
	return out
}

// FindAnchor looks for the rows of a sheet that hold the anchor, as wide as it is, anywhere: the top left cell of each
// place, at most two (more than one is no answer). An anchor of empty texts is found nowhere. Each cell is shown once,
// and a place is given up at its first cell that differs, so a sheet where every cell looks like the start of the anchor
// costs a pass over its cells and the width of the anchor for each, no more.
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
	var texts []string // the texts of a row by column, filled when the row holds the anchor's first text
	for _, r := range s.Rows {
		filled := false
		for _, c := range r.Cells {
			start := c.Col - first
			if start < 1 || start+len(anchor)-1 > MaxCols {
				continue
			}
			if !filled {
				if w.anchorText(c) != anchor[first] {
					continue
				}
				if texts == nil {
					texts = make([]string, MaxCols+1)
				}
				for _, other := range r.Cells {
					texts[other.Col] = w.anchorText(other)
				}
				filled = true
			}
			if texts[c.Col] != anchor[first] {
				continue
			}
			same := true
			for i, a := range anchor {
				if texts[start+i] != a {
					same = false
					break
				}
			}
			if same {
				found = append(found, [2]int{r.N, start})
			}
		}
		if filled {
			for _, other := range r.Cells {
				texts[other.Col] = ""
			}
		}
		if len(found) > 1 {
			return found
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
		for _, row := range s.shownRows() {
			if hidden(s.HiddenRows, row.N) {
				continue
			}
			// The line of the row, given up as soon as it would pass the size of the whole text.
			var b strings.Builder
			for _, c := range row.Cells {
				if hidden(s.HiddenCols, c.Col) {
					continue
				}
				t := w.Display(c, English)
				if t == "" {
					continue
				}
				if b.Len() > 0 {
					b.WriteString(" · ")
				}
				if total+b.Len()+len(t) > maxBytes {
					flush()
					return out
				}
				b.WriteString(t)
			}
			if b.Len() == 0 {
				continue
			}
			line := b.String()
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
