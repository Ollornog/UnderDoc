// SPDX-License-Identifier: MIT

package sheet

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func readFixture(t *testing.T, name, contentType string) *Workbook {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "libreoffice", name))
	if err != nil {
		t.Fatal(err)
	}
	wb, err := Read(context.Background(), bytes.NewReader(b), int64(len(b)), contentType, name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return wb
}

// oracle is what LibreOffice displays in each cell of the fixtures, by file, sheet and cell (scripts/sheets/make.py).
func oracle(t *testing.T, tag string) map[string]map[string]map[string]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "libreoffice", "display-"+tag+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]map[string]map[string]string
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// shown returns every cell of the visible sheets as it displays, by sheet and address; hidden rows and columns too.
func shown(wb *Workbook, l Lang) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, s := range wb.Sheets {
		if s.Hidden {
			continue
		}
		cells := map[string]string{}
		for _, r := range s.Rows {
			for _, c := range r.Cells {
				cells[CellName(r.N, c.Col)] = wb.Display(c, l)
			}
		}
		out[s.Name] = cells
	}
	return out
}

// TestFixturesShowAsLibreOfficeShows: every cell of the workbook saved by LibreOffice, as .xlsx and as .ods, shows the
// text LibreOffice itself displays, in English and, where its locale reaches the format, in German (ADR-31).
func TestFixturesShowAsLibreOfficeShows(t *testing.T) {
	for _, tag := range []string{"en", "de"} {
		l := English
		if tag == "de" {
			l = German
		}
		want := oracle(t, tag)
		english := oracle(t, "en")
		for _, f := range []struct{ name, contentType string }{{"budget.xlsx", TypeXLSX}, {"budget.ods", TypeODS}} {
			got := shown(readFixture(t, f.name, f.contentType), l)
			if len(got) != len(want[f.name]) {
				t.Errorf("%s %s: sheets %d, want %d", tag, f.name, len(got), len(want[f.name]))
			}
			for sheetName, cells := range want[f.name] {
				for cell, text := range cells {
					// LibreOffice keeps the language of a format of SpreadsheetML: in German it differs only where the
					// format names its language or the value is a boolean, and only there it is the oracle.
					if tag == "de" && english[f.name][sheetName][cell] == text {
						continue
					}
					if got[sheetName][cell] != text {
						t.Errorf("%s %s %s!%s: %q, want %q", tag, f.name, sheetName, cell, got[sheetName][cell], text)
					}
				}
				for cell, text := range got[sheetName] {
					if _, ok := cells[cell]; !ok {
						t.Errorf("%s %s %s!%s: %q, LibreOffice shows nothing", tag, f.name, sheetName, cell, text)
					}
				}
			}
		}
	}
}

// TestFixturesStructure: the sheets, the hidden ones, hidden rows and columns, merges, names, values and tables.
func TestFixturesStructure(t *testing.T) {
	for _, f := range []struct{ name, contentType string }{{"budget.xlsx", TypeXLSX}, {"budget.ods", TypeODS}} {
		wb := readFixture(t, f.name, f.contentType)
		if got := wb.Visible(); !slices.Equal(got, []string{"Budget", "Odd <&> it's"}) {
			t.Errorf("%s: visible sheets %q", f.name, got)
		}
		if len(wb.Sheets) != 3 || !wb.Sheets[1].Hidden || wb.Sheets[1].Name != "Secret" || len(wb.Sheets[1].Rows) != 0 {
			t.Errorf("%s: the hidden sheet keeps its cells: %+v", f.name, wb.Sheets[1])
		}
		budget, _ := wb.Sheet("Budget")
		if !slices.Contains(budget.HiddenRows, 7) || !slices.Contains(budget.HiddenCols, 7) {
			t.Errorf("%s: hidden row 7 %v, column G %v", f.name, budget.HiddenRows, budget.HiddenCols)
		}
		if !slices.Contains(budget.Merges, Rect{R1: 9, C1: 1, R2: 9, C2: 3}) {
			t.Errorf("%s: merges %+v", f.name, budget.Merges)
		}
		names := map[string]Name{}
		for _, n := range wb.Names {
			names[n.Name] = n
		}
		if n := names["Amounts"]; n.Sheet != "Budget" || n.Rect.String() != "B2:B5" {
			t.Errorf("%s: name Amounts %+v", f.name, n)
		}
		if n := names["VAT"]; n.Sheet != "Budget" || n.Rect.String() != "H1" {
			t.Errorf("%s: name VAT %+v", f.name, n)
		}
		if len(wb.Tables) != 1 || wb.Tables[0].Name != "Expenses" || wb.Tables[0].Rect.String() != "A1:E5" || wb.Tables[0].Header != 1 {
			t.Errorf("%s: tables %+v", f.name, wb.Tables)
		}
	}
	if wb := readFixture(t, "budget.xlsx", TypeXLSX); wb.App != "libreoffice" {
		t.Errorf("the program that saved it: %q", wb.App)
	}
}

// TestFixtureCSV: the CSV LibreOffice wrote (semicolons, UTF-8) reads as one sheet of text named after the file.
func TestFixtureCSV(t *testing.T) {
	wb := readFixture(t, "budget.csv", TypeCSV)
	if len(wb.Sheets) != 1 || wb.Sheets[0].Name != "budget" {
		t.Fatalf("%+v", wb.Sheets)
	}
	cells := shown(wb, English)["budget"]
	for cell, want := range map[string]string{"A1": "Item", "B2": "1,234.50 €", "A5": "Ümlaut ✓", "F9": "=cmd|' /C calc'!A0", "H1": "19%"} {
		if cells[cell] != want {
			t.Errorf("%s: %q, want %q", cell, cells[cell], want)
		}
	}
}
