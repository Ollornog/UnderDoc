// SPDX-License-Identifier: MIT

package sheet

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/files"
)

// part is one part of a hand-made package; stored parts are not compressed.
type part struct {
	name, body string
	stored     bool
}

func pack(t testing.TB, parts ...part) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, p := range parts {
		method := zip.Deflate
		if p.stored {
			method = zip.Store
		}
		w, err := z.CreateHeader(&zip.FileHeader{Name: p.name, Method: method})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(p.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const (
	rels     = `<?xml version="1.0"?><Relationships><Relationship Id="rId1" Type="x/officeDocument" Target="xl/workbook.xml"/></Relationships>`
	bookRels = `<?xml version="1.0"?><Relationships><Relationship Id="rId1" Type="x/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="x/sharedStrings" Target="sharedStrings.xml"/></Relationships>`
	book     = `<?xml version="1.0"?><workbook><sheets><sheet name="One" sheetId="7" r:id="rId1"/></sheets></workbook>`
	noStr    = `<?xml version="1.0"?><sst></sst>`
)

func sheetXML(data string) string {
	return `<?xml version="1.0"?><worksheet><sheetData>` + data + `</sheetData></worksheet>`
}

// xlsx is a workbook of one sheet "One" with the data and shared strings given.
func xlsx(t testing.TB, data, shared string, extra ...part) []byte {
	t.Helper()
	parts := []part{{name: "_rels/.rels", body: rels}, {name: "xl/workbook.xml", body: book},
		{name: "xl/_rels/workbook.xml.rels", body: bookRels}, {name: "xl/sharedStrings.xml", body: shared},
		{name: "xl/worksheets/sheet1.xml", body: sheetXML(data)}}
	return pack(t, append(parts, extra...)...)
}

func read(t testing.TB, b []byte, contentType string) (*Workbook, error) {
	t.Helper()
	return Read(context.Background(), bytes.NewReader(b), int64(len(b)), contentType, "x")
}

func wantLimit(t *testing.T, err error, what string) {
	t.Helper()
	var limit *LimitError
	if !errors.As(err, &limit) || limit.What != what {
		t.Fatalf("want the limit of %s, got %v", what, err)
	}
}

// TestZipBombsAreRefused: a part that would unpack to far more than it is, too many parts, and too much unpacked in all
// are refused before they are unpacked; the macros of an .xlsm are never opened, so a bomb there costs nothing.
func TestZipBombsAreRefused(t *testing.T) {
	t.Parallel()
	bomb := strings.Repeat(" ", 4<<20) // 4 MiB of spaces pack to a few KiB, a ratio far beyond 200
	if _, err := read(t, xlsx(t, `<row r="1"><c r="A1"><v>1</v></c></row>`+"<!--"+bomb+"-->", noStr), TypeXLSX); !errors.Is(err, files.ErrBomb) {
		t.Errorf("a bomb in a sheet: %v", err)
	}
	if _, err := read(t, xlsx(t, ``, `<sst>`+bomb+`</sst>`), TypeXLSX); !errors.Is(err, files.ErrBomb) {
		t.Errorf("a bomb in the shared strings: %v", err)
	}
	var many []part
	for i := range files.MaxParts + 1 {
		many = append(many, part{name: fmt.Sprintf("x/%d", i), body: "x"})
	}
	_, err := read(t, xlsx(t, ``, noStr, many...), TypeXLSX)
	wantLimit(t, err, "parts in the archive")
	// Four stored parts of 26 MiB each: none is a bomb on its own, together they pass the limit.
	huge := strings.Repeat("x", 26<<20)
	stored := []part{{name: "_rels/.rels", body: rels}, {name: "xl/_rels/workbook.xml.rels", body: `<?xml version="1.0"?><Relationships>` +
		`<Relationship Id="rId1" Type="x/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="x/worksheet" Target="worksheets/sheet2.xml"/>` +
		`<Relationship Id="rId3" Type="x/worksheet" Target="worksheets/sheet3.xml"/><Relationship Id="rId4" Type="x/worksheet" Target="worksheets/sheet4.xml"/></Relationships>`},
		{name: "xl/workbook.xml", body: `<workbook><sheets><sheet name="a" r:id="rId1"/><sheet name="b" r:id="rId2"/><sheet name="c" r:id="rId3"/><sheet name="d" r:id="rId4"/></sheets></workbook>`}}
	for i := 1; i <= 4; i++ {
		stored = append(stored, part{name: fmt.Sprintf("xl/worksheets/sheet%d.xml", i), body: sheetXML("<!--" + huge + "-->"), stored: true})
	}
	if _, err := read(t, pack(t, stored...), TypeXLSX); !errors.Is(err, files.ErrBomb) {
		t.Errorf("too much unpacked in all: %v", err)
	}
	// The macros of an .xlsm: a bomb that is never opened.
	wb, err := read(t, xlsx(t, `<row r="1"><c r="A1"><v>5</v></c></row>`, noStr, part{name: "xl/vbaProject.bin", body: bomb}), TypeXLSM)
	if err != nil || len(wb.Sheets[0].Rows) != 1 {
		t.Errorf("an .xlsm with macros: %v", err)
	}
}

// TestEntitiesAreRefused: a document type, the way to entities, is refused in every part, and an entity that is not one
// of the five of XML is an error, never expanded.
func TestEntitiesAreRefused(t *testing.T) {
	laughs := `<?xml version="1.0"?><!DOCTYPE sst [<!ENTITY a "aaaaaaaaaa"><!ENTITY b "&a;&a;&a;&a;&a;&a;&a;&a;&a;&a;"><!ENTITY c "&b;&b;&b;&b;&b;&b;&b;&b;&b;&b;">]><sst><si><t>&c;</t></si></sst>`
	external := `<?xml version="1.0"?><!DOCTYPE sst [<!ENTITY x SYSTEM "file:///etc/passwd">]><sst><si><t>&x;</t></si></sst>`
	for name, shared := range map[string]string{"laughs": laughs, "external": external} {
		if _, err := read(t, xlsx(t, ``, shared), TypeXLSX); !errors.Is(err, ErrUnreadable) || !strings.Contains(err.Error(), "document type") {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := read(t, xlsx(t, ``, `<sst><si><t>&undeclared;</t></si></sst>`), TypeXLSX); !errors.Is(err, ErrUnreadable) {
		t.Errorf("an undeclared entity: %v", err)
	}
	ods := pack(t, part{name: "mimetype", body: TypeODS}, part{name: "content.xml", body: `<?xml version="1.0"?><!DOCTYPE x [<!ENTITY e "x">]><office:document-content/>`})
	if _, err := read(t, ods, TypeODS); !errors.Is(err, ErrUnreadable) {
		t.Errorf("a document type in OpenDocument: %v", err)
	}
	wb, err := read(t, xlsx(t, `<row r="1"><c r="A1" t="s"><v>0</v></c></row>`, `<sst><si><t>a &lt;b&gt; &amp; &#x263A;</t></si></sst>`), TypeXLSX)
	if err != nil || wb.Sheets[0].Rows[0].Cells[0].Value != "a <b> & ☺" {
		t.Errorf("the five entities and character references: %+v %v", wb, err)
	}
}

// TestHugeSheetsAreRefused: rows and columns beyond Excel's grid, more cells than the limit (also through repeated rows
// and columns of OpenDocument, a file of a few hundred bytes), too many sheets.
func TestHugeSheetsAreRefused(t *testing.T) {
	t.Parallel()
	_, err := read(t, xlsx(t, `<row r="1048577"><c><v>1</v></c></row>`, noStr), TypeXLSX)
	wantLimit(t, err, "rows")
	_, err = read(t, xlsx(t, `<row r="1"><c r="XFE1"><v>1</v></c></row>`, noStr), TypeXLSX)
	wantLimit(t, err, "columns")
	var data strings.Builder
	for r := 1; r <= MaxCells/100+1; r++ {
		fmt.Fprintf(&data, `<row r="%d">`, r)
		for range 100 {
			data.WriteString(`<c><v>1</v></c>`)
		}
		data.WriteString(`</row>`)
	}
	big := pack(t, part{name: "_rels/.rels", body: rels}, part{name: "xl/workbook.xml", body: book},
		part{name: "xl/_rels/workbook.xml.rels", body: bookRels}, part{name: "xl/sharedStrings.xml", body: noStr},
		part{name: "xl/worksheets/sheet1.xml", body: sheetXML(data.String()), stored: true})
	_, err = read(t, big, TypeXLSX)
	wantLimit(t, err, "cells")
	odsBomb := func(table string) []byte {
		return pack(t, part{name: "mimetype", body: TypeODS}, part{name: "content.xml", body: `<?xml version="1.0"?><office:document-content xmlns:office="o" xmlns:table="t" xmlns:text="x"><office:body><office:spreadsheet>` +
			table + `</office:spreadsheet></office:body></office:document-content>`})
	}
	cell := `<table:table-cell office:value-type="string"><text:p>x</text:p></table:table-cell>`
	_, err = read(t, odsBomb(`<table:table table:name="a"><table:table-row table:number-rows-repeated="1048576">`+cell+`</table:table-row></table:table>`), TypeODS)
	wantLimit(t, err, "cells")
	_, err = read(t, odsBomb(`<table:table table:name="a"><table:table-row table:number-rows-repeated="1000"><table:table-cell table:number-columns-repeated="16384" office:value-type="string"><text:p>x</text:p></table:table-cell></table:table-row></table:table>`), TypeODS)
	wantLimit(t, err, "cells")
	_, err = read(t, odsBomb(`<table:table table:name="a"><table:table-row table:number-rows-repeated="1048576"/><table:table-row>`+cell+`</table:table-row></table:table>`), TypeODS)
	wantLimit(t, err, "rows")
	// Empty repeated rows, as LibreOffice writes them up to the end of the sheet, only move on.
	wb, err := read(t, odsBomb(`<table:table table:name="a"><table:table-row>`+cell+`</table:table-row><table:table-row table:number-rows-repeated="1048575"/></table:table>`), TypeODS)
	if err != nil || len(wb.Sheets[0].Rows) != 1 {
		t.Errorf("empty repeated rows: %v", err)
	}
	var sheets strings.Builder
	for i := range MaxSheets + 1 {
		fmt.Fprintf(&sheets, `<sheet name="s%d" sheetId="%d" r:id="r%d"/>`, i, i+1, i)
	}
	_, err = read(t, pack(t, part{name: "_rels/.rels", body: rels}, part{name: "xl/workbook.xml", body: `<workbook><sheets>` + sheets.String() + `</sheets></workbook>`}), TypeXLSX)
	wantLimit(t, err, "sheets")
}

// TestReadingStopsInTime: the reader looks at its context, so a workbook read past its deadline stops.
func TestReadingStopsInTime(t *testing.T) {
	var data strings.Builder
	for r := 1; r <= 5000; r++ {
		fmt.Fprintf(&data, `<row r="%d"><c><v>1</v></c><c><v>2</v></c></row>`, r)
	}
	b := xlsx(t, data.String(), noStr)
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	if _, err := Read(ctx, bytes.NewReader(b), int64(len(b)), TypeXLSX, "x"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("past the deadline: %v", err)
	}
}

// TestBrokenFilesGiveAReason: not a zip, a zip without a workbook, broken XML, an unknown type: a reason, never a panic.
func TestBrokenFilesGiveAReason(t *testing.T) {
	for name, b := range map[string][]byte{
		"not a zip":   []byte("PK\x03\x04 broken"),
		"no workbook": pack(t, part{name: "a.txt", body: "x"}),
		"broken xml":  xlsx(t, `<row r="1"><c><v>1</c></row>`, noStr),
		"no content":  pack(t, part{name: "mimetype", body: TypeODS}),
	} {
		contentType := TypeXLSX
		if name == "no content" {
			contentType = TypeODS
		}
		if _, err := read(t, b, contentType); !errors.Is(err, ErrUnreadable) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := read(t, []byte("x"), "application/pdf"); !errors.Is(err, ErrUnreadable) {
		t.Errorf("a PDF: %v", err)
	}
}

// TestCellsOfEveryKind: shared, inline and formula strings, booleans, errors, a formula without its result, a date of
// type d, escapes of OOXML, phonetic guides left out, the number of a sheet, cells without an address.
func TestCellsOfEveryKind(t *testing.T) {
	data := `<row r="2"><c r="A2" t="s"><v>0</v></c><c r="B2" t="inlineStr"><is><t>inline</t></is></c><c r="C2" t="str"><f>A2</f><v>formula</v></c>` +
		`<c r="D2" t="b"><v>1</v></c><c r="E2" t="e"><f>1/0</f><v>#DIV/0!</v></c><c r="F2"><f>1+1</f></c><c r="G2" t="d"><v>2023-03-15T18:00:00</v></c>` +
		`<c r="H2" s="0"/></row><row><c><v>3</v></c><c t="s"><v>1</v></c><c t="s"><v>99</v></c></row>`
	shared := `<sst><si><r><t>rich </t></r><r><t>text_x000D_</t></r><rPh><t>phonetic</t></rPh></si><si><t>second</t></si></sst>`
	wb, err := read(t, xlsx(t, data, shared), TypeXLSX)
	if err != nil {
		t.Fatal(err)
	}
	if wb.Sheets[0].Number != 7 {
		t.Errorf("the number of the sheet: %d", wb.Sheets[0].Number)
	}
	rows := wb.Sheets[0].Rows
	got := map[string]string{}
	for _, r := range rows {
		for _, c := range r.Cells {
			got[CellName(r.N, c.Col)] = string(rune(c.Kind)) + ":" + wb.Display(c, German)
		}
	}
	want := map[string]string{"A2": "s:rich text\r", "B2": "s:inline", "C2": "s:formula", "D2": "b:WAHR", "E2": "e:#DIV/0!",
		"F2": "f:", "G2": "n:45000,75", "A3": "n:3", "B3": "s:second"}
	for cell, w := range want {
		if got[cell] != w {
			t.Errorf("%s: %q, want %q", cell, got[cell], w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("cells: %v", got)
	}
}

// TestGridShowsWhatIsVisible: hidden rows and columns are left out, merges span visible rows and columns only, a merge
// cut at the edge of a range, and the rows beyond a limit are counted.
func TestGridShowsWhatIsVisible(t *testing.T) {
	wb := &Workbook{Formats: []string{"General"}}
	s := Sheet{Name: "S", HiddenRows: []int{3}, HiddenCols: []int{2}, Merges: []Rect{{R1: 1, C1: 1, R2: 4, C2: 3}, {R1: 6, C1: 3, R2: 7, C2: 5}}}
	for r := 1; r <= 8; r++ {
		row := Row{N: r}
		for c := 1; c <= 5; c++ {
			row.Cells = append(row.Cells, Cell{Col: c, Kind: Text, Value: CellName(r, c)})
		}
		s.Rows = append(s.Rows, row)
	}
	g := wb.Grid(&s, Rect{R1: 1, C1: 1, R2: 7, C2: 4}, English, 0, 4)
	if fmt.Sprint(g.Cols) != "[1 3 4]" {
		t.Errorf("columns: %v", g.Cols)
	}
	if len(g.Rows) != 4 || g.Rows[0].N != 1 || g.Rows[2].N != 4 || g.More != 2 {
		t.Fatalf("rows: %+v", g)
	}
	first := g.Rows[0].Cells[0]
	if first.Text != "A1" || first.RowSpan != 3 || first.ColSpan != 2 {
		t.Errorf("the merge spans visible rows and columns: %+v", first)
	}
	if len(g.Rows[1].Cells) != 1 || g.Rows[1].Cells[0].Col != 4 {
		t.Errorf("covered cells show: %+v", g.Rows[1])
	}
	rest := wb.Grid(&s, Rect{R1: 1, C1: 1, R2: 7, C2: 4}, English, 4, 10)
	if len(rest.Rows) != 2 || rest.Rows[0].N != 6 || rest.Rows[0].Cells[1].ColSpan != 2 || rest.Rows[0].Cells[1].RowSpan != 2 {
		t.Errorf("the merge cut at the edge: %+v", rest.Rows)
	}
	html := HTML(g, true, `<b>"x"</b>`)
	if !strings.Contains(html, `<th rowspan="3" colspan="2">A1</th>`) || !strings.Contains(html, `<caption>&lt;b&gt;&#34;x&#34;&lt;/b&gt;</caption>`) {
		t.Errorf("html: %s", html)
	}
}

// TestOutputIsEscapedAndDefused: text from a file never becomes markup, and a CSV of it never starts a formula.
func TestOutputIsEscapedAndDefused(t *testing.T) {
	wb := &Workbook{Formats: []string{"General", `"<i>"0`}}
	s := Sheet{Name: "S", Rows: []Row{{N: 1, Cells: []Cell{
		{Col: 1, Kind: Text, Value: `<script>alert(1)</script>`}, {Col: 2, Kind: Text, Value: `=cmd|' /C calc'!A0`},
		{Col: 3, Kind: Text, Value: "@SUM(1+1)"}, {Col: 4, Kind: Text, Value: "+1"}, {Col: 5, Kind: Text, Value: "-2+3"},
		{Col: 6, Kind: Text, Value: "\t=1"}, {Col: 7, Kind: Number, Value: "-5"}, {Col: 8, Kind: Number, Value: "7", Format: 1},
		{Col: 9, Kind: ShownNumber, Value: "-89.90"}, {Col: 10, Kind: Text, Value: "a\nb"}}}}}
	g := wb.Grid(&s, Rect{R1: 1, C1: 1, R2: 1, C2: 10}, English, 0, 0)
	out := HTML(g, false, "")
	for _, bad := range []string{"<script>", "<i>"} {
		if strings.Contains(out, bad) {
			t.Errorf("markup from the file: %s", out)
		}
	}
	if !strings.Contains(out, "&lt;i&gt;7") || !strings.Contains(out, "a<br>b") {
		t.Errorf("the format's literal or the line break: %s", out)
	}
	var b bytes.Buffer
	if err := WriteCSV(&b, wb, &s, Rect{R1: 1, C1: 1, R2: 1, C2: 10}); err != nil {
		t.Fatal(err)
	}
	// +1 and -89.90 are plain numbers, which start no formula.
	want := `<script>alert(1)</script>,'=cmd|' /C calc'!A0,'@SUM(1+1),+1,'-2+3,'` + "\t" + `=1,-5,7,-89.90,"a` + "\n" + `b"` + "\n"
	if b.String() != want {
		t.Errorf("csv:\n%q\nwant\n%q", b.String(), want)
	}
}

// TestALoneCellFarOutCostsNothing: a file of two cells, A1 and XFD1048576, has the whole of Excel's grid as its used
// range. A page of its grid stays MaxGridCols wide and counts the rest, and writing the whole range out is refused, so
// neither builds billions of empty cells.
func TestALoneCellFarOutCostsNothing(t *testing.T) {
	wb, err := read(t, xlsx(t, `<row r="1"><c r="A1"><v>1</v></c></row><row r="1048576"><c r="XFD1048576"><v>2</v></c></row>`, noStr), TypeXLSX)
	if err != nil {
		t.Fatal(err)
	}
	s := &wb.Sheets[0]
	used := s.Used()
	if used.String() != "A1:XFD1048576" {
		t.Fatalf("used: %s", used)
	}
	start := time.Now()
	g := wb.Grid(s, used, English, 0, 500)
	if len(g.Cols) != MaxGridCols || g.MoreCols != MaxCols-MaxGridCols || len(g.Rows) != 500 || g.More != MaxRows-500 {
		t.Errorf("grid: %d columns, %d more, %d rows, %d more", len(g.Cols), g.MoreCols, len(g.Rows), g.More)
	}
	for _, r := range g.Rows {
		if len(r.Cells) > MaxGridCols {
			t.Fatalf("row %d: %d cells", r.N, len(r.Cells))
		}
	}
	var b bytes.Buffer
	err = WriteCSV(&b, wb, s, used)
	wantLimit(t, err, "cells in the range written out")
	if b.Len() > 0 {
		t.Errorf("a refused CSV wrote %d bytes", b.Len())
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s", d)
	}
}

// TestTheAnchorOfAWideRangeIsNarrow: the anchor of a range as wide as Excel's grid holds MaxGridCols texts, and is found
// again by them.
func TestTheAnchorOfAWideRangeIsNarrow(t *testing.T) {
	wb, err := read(t, xlsx(t, `<row r="3"><c r="A3" t="inlineStr"><is><t>Head</t></is></c></row>`, noStr), TypeXLSX)
	if err != nil {
		t.Fatal(err)
	}
	s := &wb.Sheets[0]
	anchor := wb.Anchor(s, 3, 1, MaxCols)
	if len(anchor) != MaxGridCols || anchor[0] != "Head" {
		t.Fatalf("anchor: %d %q", len(anchor), anchor[0])
	}
	if found := wb.FindAnchor(s, anchor); len(found) != 1 || found[0] != [2]int{3, 1} {
		t.Errorf("found: %v", found)
	}
}

// TestSearchTextOfALoneCellFarOut: the text for the search goes through the cells there are, so a lone cell at
// XFD1048576 gives two short lines, quickly.
func TestSearchTextOfALoneCellFarOut(t *testing.T) {
	wb, err := read(t, xlsx(t, `<row r="1"><c r="A1"><v>1</v></c></row><row r="1048576"><c r="XFD1048576"><v>2</v></c></row>`, noStr), TypeXLSX)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	got := wb.SearchText(1500, 1<<20)
	if len(got) != 1 || got[0].Text != "1\n2\n" || got[0].Range != "A1:XFD1048576" {
		t.Errorf("%+v", got)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %s", d)
	}
}
