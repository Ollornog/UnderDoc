// SPDX-License-Identifier: MIT

package sheet

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Files made to exhaust what comes after reading: a workbook small in memory can still be huge once its cells are
// written out, formatted, laid out in a grid or searched (attack round on ADR-31, 2026-11-21).

// allocated is how many bytes fn allocates.
func allocated(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

func odsOf(t testing.TB, tables string) []byte {
	t.Helper()
	return pack(t, part{name: "mimetype", body: TypeODS}, part{name: "content.xml", body: `<?xml version="1.0"?><office:document-content xmlns:office="o" xmlns:table="t" xmlns:text="x"><office:body><office:spreadsheet>` +
		tables + `</office:spreadsheet></office:body></office:document-content>`})
}

// TestTextSharedByManyCellsCounts: one long text stands in many cells for little in the file (a shared string of
// SpreadsheetML, a repeated cell of OpenDocument). Written out, cell by cell, it would be gigabytes: the text of the cells
// counts as often as it stands, and more than MaxText is refused.
func TestTextSharedByManyCellsCounts(t *testing.T) {
	long := strings.Repeat("x", MaxCellText)
	var data strings.Builder
	for r := 1; r <= 4000; r++ {
		fmt.Fprintf(&data, `<row r="%d"><c t="s"><v>0</v></c></row>`, r)
	}
	_, err := read(t, xlsx(t, data.String(), `<sst><si><t>`+long+`</t></si></sst>`), TypeXLSX)
	wantLimit(t, err, "bytes of text in cells")
	_, err = read(t, odsOf(t, `<table:table table:name="a"><table:table-row table:number-rows-repeated="61"><table:table-cell table:number-columns-repeated="16384" office:value-type="string"><text:p>`+
		long+`</text:p></table:table-cell></table:table-row></table:table>`), TypeODS)
	wantLimit(t, err, "bytes of text in cells")
	// A workbook of ordinary size reads.
	if _, err := read(t, xlsx(t, `<row r="1"><c t="s"><v>0</v></c><c t="s"><v>0</v></c></row>`, `<sst><si><t>`+long+`</t></si></sst>`), TypeXLSX); err != nil {
		t.Errorf("two long cells: %v", err)
	}
}

// TestATextFormatDoesNotMultiplyText: a format code of 255 @ shows a text 255 times; a cell shows at most MaxCellText
// characters, whatever its format.
func TestATextFormatDoesNotMultiplyText(t *testing.T) {
	out := FormatText(strings.Repeat("x", 1000), strings.Repeat("@", MaxFormatCode))
	if n := len([]rune(out)); n > MaxCellText {
		t.Errorf("a text format made %d characters of 1000", n)
	}
	if out := FormatText("ab", `@" "@`); out != "ab ab" {
		t.Errorf("a text twice: %q", out)
	}
}

// TestAGridOfAHugeMergeCostsAPage: a merged area as large as the sheet, laid out as a page of 500 rows, costs what the
// page costs, not what the area covers.
func TestAGridOfAHugeMergeCostsAPage(t *testing.T) {
	wb := &Workbook{Formats: []string{"General"}}
	s := &Sheet{Name: "S", Rows: []Row{{N: 1, Cells: []Cell{{Col: 1, Kind: Text, Value: "top"}}}},
		Merges: []Rect{{R1: 1, C1: 1, R2: 20000, C2: 256}}}
	var g Grid
	n := allocated(func() { g = wb.Grid(s, s.Used(), English, 0, 500) })
	if n > 16<<20 {
		t.Errorf("a page of a merge of 5 million cells allocated %d MiB", n>>20)
	}
	if len(g.Rows) != 500 || g.Rows[0].Cells[0].Text != "top" || g.Rows[0].Cells[0].ColSpan != 256 || g.Rows[0].Cells[0].RowSpan != 500 {
		t.Errorf("the merge on the page: %+v", g.Rows[0])
	}
	if len(g.Rows[1].Cells) != 0 {
		t.Errorf("covered cells show: %+v", g.Rows[1])
	}
	// The next page shows the rest of the merge as one empty cell of the page.
	next := wb.Grid(s, s.Used(), English, 500, 500)
	if len(next.Rows) != 500 || len(next.Rows[0].Cells) != 1 || next.Rows[0].Cells[0].Text != "" || next.Rows[0].Cells[0].RowSpan != 500 {
		t.Errorf("the merge on the next page: %+v", next.Rows[0])
	}
	// Many merges over the same cells, as no program writes them, cost no more than the cells of the page: 3000 of them
	// take about as long as one (a ratio, not a time, so a slow or busy machine does not decide).
	took := func(n int) time.Duration {
		s.Merges = nil
		for range n {
			s.Merges = append(s.Merges, Rect{R1: 1, C1: 1, R2: 500, C2: 256})
		}
		best := time.Duration(1 << 62)
		for range 3 {
			start := time.Now()
			wb.Grid(s, s.Used(), English, 0, 500)
			best = min(best, time.Since(start))
		}
		return best
	}
	if one, many := took(1), took(3000); many > 5*one {
		t.Errorf("3000 overlapping merges took %v, one %v", many, one)
	}
}

// TestTheTextOfAMergeShowsOnce: the text of a merge whose first cell is hidden shows in its first visible cell, on the
// page where the merge starts, and not again on the next page.
func TestTheTextOfAMergeShowsOnce(t *testing.T) {
	wb := &Workbook{Formats: []string{"General"}}
	s := &Sheet{Name: "S", HiddenCols: []int{1}, Merges: []Rect{{R1: 1, C1: 1, R2: 4, C2: 2}},
		Rows: []Row{{N: 1, Cells: []Cell{{Col: 1, Kind: Text, Value: "merged"}}}, {N: 4, Cells: []Cell{{Col: 3, Kind: Text, Value: "end"}}}}}
	g := wb.Grid(s, s.Used(), English, 0, 2)
	if c := g.Rows[0].Cells[0]; c.Col != 2 || c.Text != "merged" || c.RowSpan != 2 {
		t.Errorf("first page: %+v", g.Rows[0])
	}
	next := wb.Grid(s, s.Used(), English, 2, 2)
	if c := next.Rows[0].Cells[0]; c.Col != 2 || c.Text != "" || c.RowSpan != 2 {
		t.Errorf("next page: %+v", next.Rows[0])
	}
}

// TestAGridHoldsAtMostMaxGridText: a page of a grid stops at MaxGridText bytes of text and counts the rows it leaves out,
// so a page of long texts does not grow without end.
func TestAGridHoldsAtMostMaxGridText(t *testing.T) {
	wb := &Workbook{Formats: []string{"General"}}
	long := strings.Repeat("y", MaxCellText)
	s := &Sheet{Name: "S"}
	for r := 1; r <= 500; r++ {
		row := Row{N: r}
		for c := 1; c <= 20; c++ {
			row.Cells = append(row.Cells, Cell{Col: c, Kind: Text, Value: long})
		}
		s.Rows = append(s.Rows, row)
	}
	g := wb.Grid(s, s.Used(), English, 0, 500)
	text := 0
	for _, r := range g.Rows {
		for _, c := range r.Cells {
			text += len(c.Text)
		}
	}
	if text > MaxGridText+20*MaxCellText {
		t.Errorf("a page of %d bytes of text", text)
	}
	if len(g.Rows) == 0 || len(g.Rows)+g.More != 500 {
		t.Errorf("rows %d and more %d", len(g.Rows), g.More)
	}
}

// TestFindingAnAnchorIsLinear: looking for the first row of a range in a sheet where every cell looks like its start
// costs a pass over the cells, not a pass over the row for each of them.
func TestFindingAnAnchorIsLinear(t *testing.T) {
	wb := &Workbook{Formats: []string{"General"}}
	s := &Sheet{Name: "S"}
	for r := 1; r <= 20; r++ {
		row := Row{N: r}
		for c := 1; c <= 500; c++ {
			row.Cells = append(row.Cells, Cell{Col: c, Kind: Text, Value: "x"})
		}
		s.Rows = append(s.Rows, row)
	}
	anchor := make([]string, MaxGridCols)
	for i := range anchor {
		anchor[i] = "x"
	}
	anchor[len(anchor)-1] = "y"
	var found [][2]int
	n := allocated(func() { found = wb.FindAnchor(s, anchor) })
	if len(found) != 0 {
		t.Errorf("found %v", found)
	}
	if n > 8<<20 {
		t.Errorf("looking for an anchor in 10000 cells allocated %d MiB", n>>20)
	}
	s.Rows[7].Cells[499].Value = "y"
	if found := wb.FindAnchor(s, anchor); len(found) != 1 || found[0] != [2]int{8, 500 - MaxGridCols + 1} {
		t.Errorf("found %v", found)
	}
}

// TestAnchorTextsAreShort: the anchor of a range keeps at most MaxAnchorText characters of each cell, so choosing a range
// of long texts does not write megabytes for it, and a range is found again all the same.
func TestAnchorTextsAreShort(t *testing.T) {
	wb := &Workbook{Formats: []string{"General"}}
	long := strings.Repeat("z", MaxCellText)
	s := &Sheet{Name: "S", Rows: []Row{{N: 3, Cells: []Cell{{Col: 2, Kind: Text, Value: long}, {Col: 3, Kind: Text, Value: "b"}}}}}
	anchor := wb.Anchor(s, 3, 2, 3)
	if len([]rune(anchor[0])) > MaxAnchorText {
		t.Errorf("an anchor text of %d characters", len([]rune(anchor[0])))
	}
	if found := wb.FindAnchor(s, anchor); len(found) != 1 || found[0] != [2]int{3, 2} {
		t.Errorf("found %v", found)
	}
}

// TestHiddenRowsCostNoMoreThanTheSheet: rows hidden beyond the last row with content (LibreOffice writes them up to the
// end of the sheet) are not kept, a column hidden again and again is kept once, and more hidden rows and columns than
// MaxHidden in a workbook are refused.
func TestHiddenRowsCostNoMoreThanTheSheet(t *testing.T) {
	cell := `<table:table-cell office:value-type="string"><text:p>x</text:p></table:table-cell>`
	wb, err := read(t, odsOf(t, `<table:table table:name="a"><table:table-row>`+cell+`</table:table-row><table:table-row table:number-rows-repeated="1048575" table:visibility="collapse"/></table:table>`), TypeODS)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(wb.Sheets[0].HiddenRows); n != 0 {
		t.Errorf("%d hidden rows below the content kept", n)
	}
	var tables strings.Builder
	for i := range 3 {
		fmt.Fprintf(&tables, `<table:table table:name="s%d"><table:table-row table:number-rows-repeated="1048575" table:visibility="collapse"/><table:table-row>%s</table:table-row></table:table>`, i, cell)
	}
	_, err = read(t, odsOf(t, tables.String()), TypeODS)
	wantLimit(t, err, "hidden rows and columns")
	var cols strings.Builder
	cols.WriteString(`<?xml version="1.0"?><worksheet><cols>`)
	for range 500 {
		cols.WriteString(`<col min="1" max="16384" hidden="1"/>`)
	}
	cols.WriteString(`</cols><sheetData><row r="1"><c r="XFD1"><v>1</v></c></row></sheetData></worksheet>`)
	b := pack(t, part{name: "_rels/.rels", body: rels}, part{name: "xl/workbook.xml", body: book},
		part{name: "xl/_rels/workbook.xml.rels", body: bookRels}, part{name: "xl/sharedStrings.xml", body: noStr},
		part{name: "xl/worksheets/sheet1.xml", body: cols.String()})
	var got *Workbook
	n := allocated(func() { got, err = read(t, b, TypeXLSX) })
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sheets[0].HiddenCols) != MaxCols {
		t.Errorf("hidden columns: %d", len(got.Sheets[0].HiddenCols))
	}
	if n > 16<<20 {
		t.Errorf("500 times every column hidden allocated %d MiB", n>>20)
	}
}

// TestWritingAWideRangeIsLinear: a CSV of a range of wide rows looks each cell up once, not along its row. The same
// number of cells in rows 16 times as wide must not take much longer (looking along the row took 16 times as long); a
// ratio, not a time, so a slow or busy machine does not decide.
func TestWritingAWideRangeIsLinear(t *testing.T) {
	wb := &Workbook{Formats: []string{"General"}}
	sheetOf := func(rows, cols int) *Sheet {
		s := &Sheet{Name: "S"}
		for r := 1; r <= rows; r++ {
			row := Row{N: r}
			for c := cols; c >= 1; c-- { // the order of the file, which need not be the order of the columns
				row.Cells = append(row.Cells, Cell{Col: c, Kind: Number, Value: "1"})
			}
			s.Rows = append(s.Rows, row)
		}
		return s
	}
	took := func(s *Sheet) time.Duration {
		best := time.Duration(1 << 62)
		for range 3 {
			start := time.Now()
			if err := WriteCSV(io.Discard, wb, s, s.Used()); err != nil {
				t.Fatal(err)
			}
			best = min(best, time.Since(start))
		}
		return best
	}
	wide, narrow := took(sheetOf(40, MaxCols)), took(sheetOf(640, 1024))
	if wide > 4*narrow {
		t.Errorf("650000 cells in rows of 16384 took %v, in rows of 1024 %v", wide, narrow)
	}
}

// TestSearchTextStopsEarly: the text for the search stops at its size while it is built, not after a row of long texts
// was joined whole.
func TestSearchTextStopsEarly(t *testing.T) {
	wb := &Workbook{Formats: []string{"General"}}
	long := strings.Repeat("w ", MaxCellText/2)
	row := Row{N: 1}
	for c := 1; c <= 4000; c++ {
		row.Cells = append(row.Cells, Cell{Col: c, Kind: Text, Value: long})
	}
	wb.Sheets = []Sheet{{Name: "S", Rows: []Row{row, {N: 2, Cells: []Cell{{Col: 1, Kind: Text, Value: "after"}}}}}}
	var out []TextSection
	n := allocated(func() { out = wb.SearchText(1500, 1<<20) })
	if n > 32<<20 {
		t.Errorf("the search text of a row of 128 MiB allocated %d MiB", n>>20)
	}
	total := 0
	for _, s := range out {
		total += len(s.Text)
	}
	if total > 1<<20 {
		t.Errorf("search text of %d bytes", total)
	}
}

// TestAZipDirectoryIsCountedBeforeItIsRead: a zip whose directory names a million parts is refused before the directory is
// read into memory.
func TestAZipDirectoryIsCountedBeforeItIsRead(t *testing.T) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for i := range 200_000 {
		if _, err := z.CreateHeader(&zip.FileHeader{Name: fmt.Sprint(i), Method: zip.Store}); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	n := allocated(func() { _, err = read(t, buf.Bytes(), TypeXLSX) })
	wantLimit(t, err, "parts in the archive")
	if n > 8<<20 {
		t.Errorf("a directory of 200000 parts allocated %d MiB before it was refused", n>>20)
	}
}

// TestEveryCSVFieldIsQuoted: a spreadsheet program that splits a CSV at another separator than the comma (the semicolon,
// as German Excel and LibreOffice do) starts a new field after it; "x;=1+1" written bare became the formula =1+1 there
// (LibreOffice 26.8, checked by hand). Every field is quoted, so it stays one field whatever the separator.
func TestEveryCSVFieldIsQuoted(t *testing.T) {
	wb := &Workbook{Formats: []string{"General"}}
	s := &Sheet{Name: "S", Rows: []Row{{N: 1, Cells: []Cell{{Col: 1, Kind: Text, Value: "x;=1+1"}, {Col: 2, Kind: Number, Value: "5"},
		{Col: 4, Kind: Text, Value: `a"b`}, {Col: 5, Kind: Text, Value: "c\t@SUM(1)"}}}}}
	var b bytes.Buffer
	if err := WriteCSV(&b, wb, s, Rect{R1: 1, C1: 1, R2: 1, C2: 5}); err != nil {
		t.Fatal(err)
	}
	if want := `"x;=1+1","5","","a""b","c` + "\t" + `@SUM(1)"` + "\n"; b.String() != want {
		t.Errorf("csv:\n%q\nwant\n%q", b.String(), want)
	}
}

// TestASheetHiddenByACommonStyleStaysHidden: OpenDocument may hide a sheet by a style of styles.xml as well as by an
// automatic style of content.xml.
func TestASheetHiddenByACommonStyleStaysHidden(t *testing.T) {
	content := `<?xml version="1.0"?><office:document-content xmlns:office="o" xmlns:table="t" xmlns:text="x" xmlns:style="s"><office:body><office:spreadsheet>` +
		`<table:table table:name="Secret" table:style-name="Hide"><table:table-row><table:table-cell office:value-type="string"><text:p>salary</text:p></table:table-cell></table:table-row></table:table>` +
		`<table:table table:name="Open"><table:table-row><table:table-cell office:value-type="string"><text:p>ok</text:p></table:table-cell></table:table-row></table:table>` +
		`</office:spreadsheet></office:body></office:document-content>`
	styles := `<?xml version="1.0"?><office:document-styles xmlns:office="o" xmlns:style="s"><office:styles><style:style style:name="Hide" style:family="table"><style:table-properties table:display="false"/></style:style></office:styles></office:document-styles>`
	wb, err := read(t, pack(t, part{name: "mimetype", body: TypeODS}, part{name: "styles.xml", body: styles}, part{name: "content.xml", body: content}), TypeODS)
	if err != nil {
		t.Fatal(err)
	}
	if s, visible := wb.Sheet("Secret"); visible || s == nil || len(s.Rows) != 0 {
		t.Errorf("the sheet hidden by a common style: %+v", s)
	}
	if fmt.Sprint(wb.Visible()) != "[Open]" {
		t.Errorf("visible: %v", wb.Visible())
	}
}
