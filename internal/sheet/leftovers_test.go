// SPDX-License-Identifier: MIT

package sheet

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// What the attack round of ADR-31 left open (B-2), as tests.

// TestTheCacheIsBoundedBySize: the cache counts what its workbooks take and drops the oldest past its size, whatever
// their count; the newest stays even when it alone is larger, and an entry kept again is counted once.
func TestTheCacheIsBoundedBySize(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dirBlobs{dir}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if c.maxBytes != CacheBytes || CacheBytes > 512<<20 {
		t.Fatalf("the cache may keep %d bytes", c.maxBytes)
	}
	// Each blob is a CSV of about 300 KiB of text; the cache may keep about 1 MiB.
	c.maxBytes = 1 << 20
	ctx := context.Background()
	for i := range 5 {
		var b strings.Builder
		for r := range 3000 {
			fmt.Fprintf(&b, "%d,%s\n", r, strings.Repeat(string(rune('a'+i)), 100))
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprint("blob", i)), []byte(b.String()), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Workbook(ctx, "t", fmt.Sprint("blob", i), TypeCSV, "x.csv"); err != nil {
			t.Fatal(err)
		}
	}
	c.mu.Lock()
	kept, held := slices.Clone(c.order), c.bytes
	sum := 0
	for _, k := range kept {
		sum += c.entries[k].size
	}
	c.mu.Unlock()
	if len(kept) >= 5 || held > c.maxBytes || held != sum || kept[len(kept)-1] != "t/blob4-csv" {
		t.Errorf("kept %q, %d bytes (%d counted), at most %d", kept, held, sum, c.maxBytes)
	}
	if held < 512<<10 {
		t.Errorf("the cache keeps less than it may: %d bytes in %q", held, kept)
	}
	// The newest stays, larger than all the cache may keep; kept again, it is counted once.
	c.maxBytes = 1000
	big := &Workbook{Sheets: []Sheet{{Name: "S", Rows: []Row{{N: 1, Cells: []Cell{{Col: 1, Kind: Text, Value: strings.Repeat("x", 5000)}}}}}}}
	c.put("big", cached{wb: big})
	c.put("big", cached{wb: big})
	if c.mu.Lock(); len(c.order) != 1 || c.order[0] != "big" || c.bytes != big.memory() {
		t.Errorf("after a large one: %q, %d bytes", c.order, c.bytes)
	}
	c.mu.Unlock()
	if big.memory() < 5000 {
		t.Errorf("the estimate leaves out the text: %d", big.memory())
	}
}

// worksheet is a workbook of one sheet "One" with the worksheet given whole.
func worksheet(t *testing.T, xml string) *Workbook {
	t.Helper()
	b := pack(t, part{name: "_rels/.rels", body: rels}, part{name: "xl/workbook.xml", body: book},
		part{name: "xl/_rels/workbook.xml.rels", body: bookRels}, part{name: "xl/sharedStrings.xml", body: noStr},
		part{name: "xl/worksheets/sheet1.xml", body: `<?xml version="1.0"?><worksheet>` + xml + `</worksheet>`})
	wb, err := read(t, b, TypeXLSX)
	if err != nil {
		t.Fatal(err)
	}
	return wb
}

func cellText(text string) string { return `<is><t>` + text + `</t></is>` }

// TestRowsHiddenByDefault: a sheet may hide every row by default (zeroHeight of sheetFormatPr, ECMA-376 Part 1,
// 18.3.1.81) and write out only the rows that are not hidden; a row written out is hidden only when it says so. A row of
// no height (customHeight with ht 0) is hidden as well. Without zeroHeight, rows not written out are no hidden rows.
func TestRowsHiddenByDefault(t *testing.T) {
	rows := `<sheetData><row r="1"><c r="A1" t="inlineStr">` + cellText("Head") + `</c></row>` +
		`<row r="2" hidden="1"><c r="A2" t="inlineStr">` + cellText("Hidden itself") + `</c></row>` +
		`<row r="5"><c r="A5" t="inlineStr">` + cellText("Shown") + `</c></row>` +
		`<row r="6" customHeight="1" ht="0"><c r="A6" t="inlineStr">` + cellText("No height") + `</c></row>` +
		`<row r="7" customHeight="1" ht="0.0"><c r="A7" t="inlineStr">` + cellText("No height either") + `</c></row>` +
		`<row r="8" customHeight="1" ht="12"><c r="A8" t="inlineStr">` + cellText("Low") + `</c></row></sheetData>`
	wb := worksheet(t, `<sheetFormatPr defaultRowHeight="15" zeroHeight="1"/>`+rows)
	s := &wb.Sheets[0]
	if want := []int{2, 3, 4, 6, 7}; !slices.Equal(s.HiddenRows, want) {
		t.Errorf("hidden by default: %v, want %v", s.HiddenRows, want)
	}
	g := wb.Grid(s, s.Used(), English, 0, 0)
	var shown []string
	for _, r := range g.Rows {
		shown = append(shown, r.Cells[0].Text)
	}
	if !slices.Equal(shown, []string{"Head", "Shown", "Low"}) {
		t.Errorf("the grid shows %q", shown)
	}
	if text := wb.SearchText(1000, 1<<20); len(text) != 1 || strings.Contains(text[0].Text, "Hidden") || strings.Contains(text[0].Text, "No height") {
		t.Errorf("the search finds %+v", text)
	}
	plain := worksheet(t, `<sheetFormatPr defaultRowHeight="15" zeroHeight="0"/>`+rows)
	if want := []int{2, 6, 7}; !slices.Equal(plain.Sheets[0].HiddenRows, want) {
		t.Errorf("without zeroHeight: %v, want %v", plain.Sheets[0].HiddenRows, want)
	}
}

// TestAMergeWithItsFirstCellHiddenIsOneThing: the text of a merged area whose first cell is hidden shows in its first
// visible cell (as Excel shows a merged cell partly hidden), and the search and a CSV find it there too, instead of
// leaving it out as hidden. A cell of its own the file keeps under the area does not win. An area hidden whole shows
// nothing anywhere.
func TestAMergeWithItsFirstCellHiddenIsOneThing(t *testing.T) {
	wb := worksheet(t, `<cols><col min="3" max="3" hidden="1"/></cols><sheetData>`+
		`<row r="1" hidden="1"><c r="A1" t="inlineStr">`+cellText("Total")+`</c><c r="C1" t="inlineStr">`+cellText("Gone")+`</c></row>`+
		`<row r="2"><c r="A2" t="inlineStr">`+cellText("stale")+`</c></row>`+
		`<row r="4"><c r="C4" t="inlineStr">`+cellText("Region")+`</c></row>`+
		`<row r="6"><c r="F6" t="inlineStr">`+cellText("Last")+`</c></row></sheetData>`+
		`<mergeCells><mergeCell ref="A1:B3"/><mergeCell ref="C4:D5"/><mergeCell ref="C1:C1"/><mergeCell ref="C1:C2"/></mergeCells>`)
	s := &wb.Sheets[0]
	all := Rect{R1: 1, C1: 1, R2: 6, C2: 6}
	g := wb.Grid(s, all, English, 0, 0)
	grid := map[string]string{}
	for _, r := range g.Rows {
		for _, c := range r.Cells {
			if c.Text != "" {
				grid[CellName(r.N, c.Col)] = c.Text
			}
		}
	}
	if grid["A2"] != "Total" || grid["D4"] != "Region" || grid["F6"] != "Last" || len(grid) != 3 {
		t.Errorf("the grid shows %v", grid)
	}
	var b bytes.Buffer
	if err := WriteCSV(&b, wb, s, all); err != nil {
		t.Fatal(err)
	}
	// Row 1 and column C are hidden: rows 2 to 6, columns A, B, D, E, F.
	want := `"Total","","","",""` + "\n" + `"","","","",""` + "\n" + `"","","Region","",""` + "\n" + `"","","","",""` + "\n" + `"","","","","Last"` + "\n"
	if b.String() != want {
		t.Errorf("csv:\n%s\nwant\n%s", b.String(), want)
	}
	var found []string
	for _, x := range wb.SearchText(1000, 1<<20) {
		found = append(found, x.Text)
	}
	if text := strings.Join(found, ""); text != "Total\nRegion\nLast\n" {
		t.Errorf("the search finds %q", text)
	}
	// A range that starts below the first visible cell of the area: neither the grid nor the CSV shows its text.
	part := Rect{R1: 3, C1: 1, R2: 3, C2: 2}
	if g := wb.Grid(s, part, English, 0, 0); g.Rows[0].Cells[0].Text != "" {
		t.Errorf("the grid of a part: %+v", g.Rows)
	}
	b.Reset()
	if err := WriteCSV(&b, wb, s, part); err != nil || b.String() != `"",""`+"\n" {
		t.Errorf("the csv of a part: %q %v", b.String(), err)
	}
	if s.Rows[0].Cells[0].Value != "Total" || len(s.Rows) != 4 {
		t.Errorf("the sheet itself changed: %+v", s.Rows)
	}
}

// TestFirstVisibleSkipsARunAtOnce: the first visible row of an area is found in one step over a run of hidden ones.
func TestFirstVisibleSkipsARunAtOnce(t *testing.T) {
	hiddenRows := []int{3, 4, 5, 8, 9}
	for _, c := range []struct{ a, b, want int }{{1, 9, 1}, {3, 9, 6}, {3, 5, 0}, {8, 9, 0}, {8, 10, 10}, {4, 7, 6}, {6, 6, 6}} {
		if got := firstVisible(hiddenRows, c.a, c.b); got != c.want {
			t.Errorf("first visible of %d to %d = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestLookAlikesDoNotStartAFormula: a text that starts with a full-width = + - @, or with white space before one of
// them, gets the apostrophe too; LibreOffice reads both as text, Excel may not. Plain numbers and texts stay.
func TestLookAlikesDoNotStartAFormula(t *testing.T) {
	for in, want := range map[string]string{
		"＝1+1":               "'＝1+1",
		"＋cmd|' /C calc'!A0": "'＋cmd|' /C calc'!A0",
		"－2":                 "'－2",
		"＠SUM(1)":            "'＠SUM(1)",
		" =1+1":              "' =1+1",
		" =1":                "' =1",
		"　＝1":                "'　＝1",
		" -5":                "' -5",
		"\n@x":               "'\n@x",
		"\tplain":            "'\tplain",
		"=1":                 "'=1",
		"-89.90":             "-89.90",
		"+1":                 "+1",
		"Net = 1":            "Net = 1",
		"  plain":            "  plain",
		"   ":                "   ",
		"":                   "",
	} {
		if got := Defuse(in); got != want {
			t.Errorf("Defuse(%q) = %q, want %q", in, got, want)
		}
	}
}
