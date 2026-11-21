// SPDX-License-Identifier: MIT

package web

import (
	"archive/zip"
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/sheet"
)

func sheetFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "sheet", "testdata", "libreoffice", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type sheetsAnswer struct {
	Name   string          `json:"name"`
	Sheets []apiSheet      `json:"sheets"`
	Names  []apiSheetName  `json:"names"`
	Tables []apiSheetTable `json:"tables"`
}

type gridAnswer struct {
	Sheet string `json:"sheet"`
	Range string `json:"range"`
	From  int    `json:"from"`
	sheet.Grid
}

// TestSpreadsheetAsTables (ADR-31): an uploaded workbook lists its visible sheets, names and tables, shows any range in
// the reader's language without hidden rows and columns, and downloads as CSV without formulas; a hidden sheet is not
// there, and whoever may not read the file gets nothing of it.
func TestSpreadsheetAsTables(t *testing.T) {
	s := withFiles(t)
	mine := s.root(nil, db.RootUser)
	parent := fmt.Sprintf(`"parent":%q`, mine.ID)
	up := decode[finished](t, s.put(nil, parent, "budget.xlsx", sheetFixture(t, "budget.xlsx"), 4096))
	file := decode[apiFile](t, s.get("/api/files/"+up.ID))
	if file.Sheets != "/api/files/"+up.ID+"/sheets" {
		t.Fatalf("the file names its tables: %+v", file)
	}
	list := decode[sheetsAnswer](t, s.get(file.Sheets))
	if len(list.Sheets) != 2 || list.Sheets[0].Name != "Budget" || list.Sheets[1].Name != "Odd <&> it's" {
		t.Fatalf("the visible sheets: %+v", list.Sheets)
	}
	if len(list.Tables) != 1 || list.Tables[0].Name != "Expenses" || list.Tables[0].Range != "A1:E5" {
		t.Errorf("tables: %+v", list.Tables)
	}
	values := map[string]bool{}
	for _, n := range list.Names {
		values[n.Name] = n.Value
	}
	if v, ok := values["VAT"]; !ok || !v || values["Amounts"] {
		t.Errorf("names: %+v", list.Names)
	}
	for _, target := range []string{"/sheets/grid?sheet=Secret", "/sheets/csv?sheet=Secret"} {
		if got := s.get("/api/files/" + up.ID + target); got.Code != http.StatusBadRequest {
			t.Errorf("the hidden sheet through %s: %d", target, got.Code)
		}
	}
	german := map[string]string{"Accept-Language": "de-DE,de;q=0.9"}
	grid := decode[gridAnswer](t, s.getWith("/api/files/"+up.ID+"/sheets/grid?sheet=Budget&range=A1:H9", merged(german)))
	cells := map[string]sheet.GridCell{}
	for _, r := range grid.Rows {
		for _, c := range r.Cells {
			cells[sheet.CellName(r.N, c.Col)] = c
		}
	}
	for cell, want := range map[string]string{"B2": "1.234,50 €", "E2": "WAHR", "C6": "#DIV/0!", "D6": "36:00", "A4": "Coffee & <tea>"} {
		if cells[cell].Text != want {
			t.Errorf("%s: %q, want %q", cell, cells[cell].Text, want)
		}
	}
	if _, ok := cells["A7"]; ok {
		t.Error("the hidden row shows")
	}
	if _, ok := cells["G2"]; ok {
		t.Error("the hidden column shows")
	}
	if m := cells["A9"]; m.ColSpan != 3 {
		t.Errorf("the merged cells: %+v", m)
	}
	english := decode[gridAnswer](t, s.get("/api/files/"+up.ID+"/sheets/grid?sheet=Budget&range=B2"))
	if english.Rows[0].Cells[0].Text != "1,234.50 €" {
		t.Errorf("in English: %+v", english.Rows)
	}
	csv := s.get("/api/files/" + up.ID + "/sheets/csv?sheet=Budget&range=F9:F12")
	if csv.Code != http.StatusOK || csv.Header().Get("Content-Type") != "text/csv; charset=utf-8" ||
		!strings.HasPrefix(csv.Header().Get("Content-Disposition"), "attachment") || csv.Header().Get("Content-Security-Policy") != fileCSP {
		t.Fatalf("the CSV: %d %v", csv.Code, csv.Header())
	}
	if got := csv.Body.String(); got != "'=cmd|' /C calc'!A0\n'@SUM(1+1)\n'-2+3\n<script>alert(1)</script>\n" {
		t.Errorf("the CSV starts formulas: %q", got)
	}
	// Bob reads nothing of Anna's file.
	for _, target := range []string{"/sheets", "/sheets/grid?sheet=Budget", "/sheets/csv?sheet=Budget"} {
		if got := s.getWith("/api/files/"+up.ID+target, merged(plain)); got.Code != http.StatusNotFound || strings.Contains(got.Body.String(), "Budget") {
			t.Errorf("Bob through %s: %d %s", target, got.Code, got.Body.String())
		}
	}
	// A file that is no spreadsheet has no tables.
	text := decode[finished](t, s.put(nil, parent, "notes.txt", []byte("plain"), 4096))
	if got := s.get("/api/files/" + text.ID + "/sheets"); got.Code != http.StatusNotFound {
		t.Errorf("a text file: %d", got.Code)
	}
}

// TestSpreadsheetThatCannotBeRead: a zip bomb named .xlsx, and a broken file, answer why in a sentence, never an error of
// the server, and the reason is kept so the file is not read again.
func TestSpreadsheetThatCannotBeRead(t *testing.T) {
	s := withFiles(t)
	mine := s.root(nil, db.RootUser)
	parent := fmt.Sprintf(`"parent":%q`, mine.ID)
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	w, _ := z.Create("xl/worksheets/sheet1.xml")
	_, _ = w.Write(bytes.Repeat([]byte(" "), 8<<20))
	w, _ = z.Create("xl/workbook.xml")
	_, _ = w.Write([]byte(`<workbook><sheets><sheet name="a" r:id="rId1"/></sheets></workbook>`))
	w, _ = z.Create("xl/_rels/workbook.xml.rels")
	_, _ = w.Write([]byte(`<Relationships><Relationship Id="rId1" Type="x/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`))
	_ = z.Close()
	for name, content := range map[string][]byte{"bomb.xlsx": buf.Bytes(), "broken.ods": append([]byte("PK\x03\x04"), bytes.Repeat([]byte("x"), 100)...)} {
		up := decode[finished](t, s.put(nil, parent, name, content, 1<<20))
		got := s.get("/api/files/" + up.ID + "/sheets")
		if got.Code != http.StatusUnprocessableEntity || !strings.Contains(got.Body.String(), "cannot be") {
			t.Errorf("%s: %d %s", name, got.Code, got.Body.String())
		}
		v, err := s.db.FileVersion(inMain(), up.ID, "u-7", 0)
		if err != nil {
			t.Fatal(err)
		}
		if path, _ := s.store.DerivedPath("main", v.Blob+sheetFailed); !fileExists(path) {
			t.Errorf("%s: the reason is not kept", name)
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// TestALoneCellFarOutThroughTheAPI: the grid of a workbook whose used range is Excel's whole grid answers one page of
// MaxGridCols columns, and its CSV is refused with a sentence.
func TestALoneCellFarOutThroughTheAPI(t *testing.T) {
	s := withFiles(t)
	mine := s.root(nil, db.RootUser)
	far := workbookOf(t, `<row r="1"><c r="A1"><v>1</v></c></row><row r="1048576"><c r="XFD1048576"><v>2</v></c></row>`)
	up := decode[finished](t, s.put(nil, fmt.Sprintf(`"parent":%q`, mine.ID), "far.xlsx", far, 4096))
	grid := decode[gridAnswer](t, s.get("/api/files/"+up.ID+"/sheets/grid?sheet=One"))
	if len(grid.Cols) != sheet.MaxGridCols || grid.MoreCols == 0 || len(grid.Rows) != gridRows {
		t.Errorf("the grid: %d columns, %d more, %d rows", len(grid.Cols), grid.MoreCols, len(grid.Rows))
	}
	if got := s.get("/api/files/" + up.ID + "/sheets/csv?sheet=One"); got.Code != http.StatusBadRequest || !strings.Contains(got.Body.String(), "too large") {
		t.Errorf("the CSV of the whole grid: %d %s", got.Code, got.Body.String())
	}
	if got := s.get("/api/files/" + up.ID + "/sheets/csv?sheet=One&range=A1:B2"); got.Code != http.StatusOK || got.Body.String() != "1,\n,\n" {
		t.Errorf("a small range: %d %q", got.Code, got.Body.String())
	}
}

// workbookOf is a workbook of one sheet "One" with the rows given, as SpreadsheetML.
func workbookOf(t *testing.T, rows string) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"_rels/.rels":                `<Relationships><Relationship Id="rId1" Type="x/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<workbook><sheets><sheet name="One" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Type="x/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   `<worksheet><sheetData>` + rows + `</sheetData></worksheet>`,
	} {
		w, _ := z.Create(name)
		_, _ = w.Write([]byte(body))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
