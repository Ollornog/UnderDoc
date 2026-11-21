// SPDX-License-Identifier: MIT

package web

import (
	"bytes"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/sheet"
)

// Spreadsheets as tables (ADR-31): a file of a type internal/sheet reads shows its visible sheets, names and tables, and
// any rectangle of a sheet as a grid in the reader's language. Reading and keeping what was read is sheet.Cache's.

// gridRows is the most rows one answer of a grid holds.
const gridRows = 500

// fileWorkbook reads the current version of a file the viewer may read. A file that is no spreadsheet is ErrNotFound.
func (h *handler) fileWorkbook(r *http.Request, fileID string) (db.FileVersion, *sheet.Workbook, error) {
	user, _ := account.From(r.Context())
	v, err := h.options.DB.FileVersion(r.Context(), fileID, user.ID, 0)
	if err != nil {
		return v, nil, err
	}
	if !sheet.Readable(v.ContentType) {
		return v, nil, db.ErrNotFound
	}
	wb, err := h.options.Sheets.Workbook(r.Context(), h.tenant(r), v.Blob, v.ContentType, v.Name)
	return v, wb, err
}

type apiSheet struct {
	Name string `json:"name"`
	// Range is the used range, "" for an empty sheet.
	Range string `json:"range"`
	Rows  int    `json:"rows"`
	Cols  int    `json:"cols"`
}

type apiSheetName struct {
	Name  string `json:"name"`
	Sheet string `json:"sheet"`
	Range string `json:"range"`
	Scope string `json:"scope,omitempty"`
	Value bool   `json:"value,omitempty"`
}

type apiSheetTable struct {
	Name   string `json:"name"`
	Sheet  string `json:"sheet"`
	Range  string `json:"range"`
	Header int    `json:"header"`
}

// apiSheets lists the visible sheets of a spreadsheet, and the names and tables on them (ADR-31).
func (h *handler) apiSheets(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	v, wb, err := h.fileWorkbook(r, r.PathValue("id"))
	if h.sheetFailed(w, err) {
		return
	}
	if private, err := h.options.DB.Private(r.Context(), r.PathValue("id")); err == nil && !private {
		h.note(r, db.LogOpen, r.PathValue("id"), "")
	}
	out := struct {
		Name   string          `json:"name"`
		Sheets []apiSheet      `json:"sheets"`
		Names  []apiSheetName  `json:"names"`
		Tables []apiSheetTable `json:"tables"`
	}{Name: v.Name, Sheets: []apiSheet{}, Names: []apiSheetName{}, Tables: []apiSheetTable{}}
	for i := range wb.Sheets {
		s := &wb.Sheets[i]
		if s.Hidden {
			continue
		}
		used := s.Used()
		entry := apiSheet{Name: s.Name}
		if used.R2 > 0 {
			entry.Range, entry.Rows, entry.Cols = used.String(), used.R2-len(s.HiddenRows), used.C2
		}
		out.Sheets = append(out.Sheets, entry)
	}
	for _, n := range wb.Names {
		out.Names = append(out.Names, apiSheetName{Name: n.Name, Sheet: n.Sheet, Range: n.Rect.String(), Scope: n.Scope, Value: n.Rect.Cells() == 1})
	}
	for _, t := range wb.Tables {
		out.Tables = append(out.Tables, apiSheetTable{Name: t.Name, Sheet: t.Sheet, Range: t.Rect.String(), Header: t.Header})
	}
	writeJSON(w, http.StatusOK, out)
}

// sheetRect reads the sheet and the range of a request: the used range of the sheet without ?range=.
func sheetRect(r *http.Request, wb *sheet.Workbook) (*sheet.Sheet, sheet.Rect, string) {
	s, visible := wb.Sheet(r.URL.Query().Get("sheet"))
	if !visible {
		return nil, sheet.Rect{}, "no such sheet"
	}
	rect := s.Used()
	if text := r.URL.Query().Get("range"); text != "" {
		var ok bool
		if rect, ok = sheet.ParseRect(text); !ok {
			return nil, sheet.Rect{}, "range is a range of cells such as A1:D20"
		}
	}
	return s, rect, ""
}

// apiSheetGrid answers a rectangle of a visible sheet as it shows, in pages of rows: ?sheet=, ?range= (the used range
// without it), ?from= the first visible row to show, counted from 0, or ?at= a row whose page to show.
func (h *handler) apiSheetGrid(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	_, wb, err := h.fileWorkbook(r, r.PathValue("id"))
	if h.sheetFailed(w, err) {
		return
	}
	s, rect, problem := sheetRect(r, wb)
	if problem != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": problem})
		return
	}
	from, _ := strconv.Atoi(r.URL.Query().Get("from"))
	// ?at= a row: the page that holds the first visible row from it on, as a hit of the search opens it.
	if at, err := strconv.Atoi(r.URL.Query().Get("at")); err == nil && at > 0 {
		hidden := map[int]bool{}
		for _, n := range s.HiddenRows {
			hidden[n] = true
		}
		index := 0
		for n := rect.R1; n < min(at, rect.R2+1); n++ {
			if !hidden[n] {
				index++
			}
		}
		from = index / gridRows * gridRows
	}
	out := struct {
		Sheet string `json:"sheet"`
		Range string `json:"range"`
		From  int    `json:"from"`
		sheet.Grid
	}{Sheet: s.Name, From: max(from, 0)}
	if rect.R2 > 0 {
		out.Range = rect.String()
		out.Grid = wb.Grid(s, rect, sheet.LangOf(r.Header.Get("Accept-Language")), out.From, gridRows)
	} else {
		out.Grid = sheet.Grid{Rows: []sheet.GridRow{}, Cols: []int{}}
	}
	writeJSON(w, http.StatusOK, out)
}

// apiSheetCSV sends a rectangle of a visible sheet as CSV, without hidden rows and columns, and without a text that a
// spreadsheet program would read as a formula (sheet.Defuse).
func (h *handler) apiSheetCSV(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	v, wb, err := h.fileWorkbook(r, r.PathValue("id"))
	if h.sheetFailed(w, err) {
		return
	}
	s, rect, problem := sheetRect(r, wb)
	if problem != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": problem})
		return
	}
	var out bytes.Buffer
	if rect.R2 > 0 {
		var limit *sheet.LimitError
		if err := sheet.WriteCSV(&out, wb, s, rect); errors.As(err, &limit) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("the range is too large to download: more than %d cells; choose a smaller one", limit.Limit)})
			return
		} else if err != nil {
			h.failed(w, err, "csv")
			return
		}
	}
	name := strings.TrimSuffix(v.Name, path.Ext(v.Name)) + " - " + s.Name + ".csv"
	header := w.Header()
	header.Set("Content-Type", "text/csv; charset=utf-8")
	header.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	header.Set("Content-Security-Policy", fileCSP)
	header.Set("Cache-Control", "private, no-cache")
	_, _ = w.Write(out.Bytes())
}

// sheetFailed answers a file that is not there, no spreadsheet, or cannot be read; the reason of the last is a sentence.
func (h *handler) sheetFailed(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	var reason *sheet.Reason
	if errors.As(err, &reason) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": reason.Text})
		return true
	}
	return h.failed(w, err, "sheet")
}
