// SPDX-License-Identifier: MIT

package web

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/files"
	"github.com/Ollornog/UnderDoc/internal/sheet"
)

// Spreadsheets as tables (ADR-31): a file of a type internal/sheet reads shows its visible sheets, names and tables, and
// any rectangle of a sheet as a grid in the reader's language. What was read is kept beside the blob (derived) and in a
// small cache in memory; at most two workbooks are read at once, each for at most sheetTime.

const (
	// sheetTime is how long reading one workbook may take.
	sheetTime = 30 * time.Second
	// sheetDerived is the name of what was read, beside the blob; its number moves when the model changes.
	sheetDerived = "-sheet-1.json.gz"
	// sheetFailed keeps why a blob could not be read, so a broken or too large file is not read again on every view.
	sheetFailed = "-sheet-1.err"
	// gridRows is the most rows one answer of a grid holds.
	gridRows = 500
)

var (
	// sheetReaders holds a place for each workbook being read.
	sheetReaders = make(chan struct{}, 2)
	// sheetCache keeps the workbooks read last, by tenant and blob.
	sheetCache = &workbookCache{max: 8}
)

// workbookCache is a small cache of workbooks in memory, the oldest out first. A failure that may pass, such as the end
// of the time, is kept for a while only.
type workbookCache struct {
	mu      sync.Mutex
	max     int
	order   []string
	entries map[string]cachedWorkbook
}

type cachedWorkbook struct {
	wb    *sheet.Workbook
	err   error
	until time.Time
}

func (c *workbookCache) get(key string) (cachedWorkbook, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if ok && !e.until.IsZero() && time.Now().After(e.until) {
		delete(c.entries, key)
		return cachedWorkbook{}, false
	}
	return e, ok
}

func (c *workbookCache) put(key string, e cachedWorkbook) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]cachedWorkbook{}
	}
	if _, ok := c.entries[key]; !ok {
		c.order = append(c.order, key)
	}
	c.entries[key] = e
	for len(c.order) > c.max {
		delete(c.entries, c.order[0])
		c.order = c.order[1:]
	}
}

// workbook returns the workbook of a blob: from memory, from what was read before, or read now.
func (h *handler) workbook(ctx context.Context, tenant, sha, contentType, name string) (*sheet.Workbook, error) {
	key := tenant + "/" + sha
	if e, ok := sheetCache.get(key); ok {
		return e.wb, e.err
	}
	wb, err := h.readWorkbook(ctx, tenant, sha, contentType, name)
	switch {
	case err == nil:
		sheetCache.put(key, cachedWorkbook{wb: wb})
	case errors.Is(err, context.DeadlineExceeded):
		err = &sheetError{"this file takes too long to read as tables"}
		sheetCache.put(key, cachedWorkbook{err: err, until: time.Now().Add(10 * time.Minute)})
	case errors.Is(err, context.Canceled):
	default:
		sheetCache.put(key, cachedWorkbook{err: err})
	}
	return wb, err
}

func (h *handler) readWorkbook(ctx context.Context, tenant, sha, contentType, name string) (*sheet.Workbook, error) {
	done, err := h.options.Store.DerivedPath(tenant, sha+sheetDerived)
	if err != nil {
		return nil, err
	}
	failed, err := h.options.Store.DerivedPath(tenant, sha+sheetFailed)
	if err != nil {
		return nil, err
	}
	if wb, err := loadWorkbook(done); err == nil {
		return wb, nil
	}
	if reason, err := os.ReadFile(failed); err == nil {
		return nil, &sheetError{string(reason)}
	}
	select {
	case sheetReaders <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-sheetReaders }()
	f, err := h.options.Store.Open(tenant, sha)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	// The time of reading runs on its own: a viewer who leaves does not end it, so the next one finds it done.
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sheetTime)
	defer cancel()
	wb, err := sheet.Read(readCtx, f, info.Size(), contentType, name)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		reason := sheetReason(err)
		if werr := h.options.Store.WriteDerived(tenant, sha+sheetFailed, []byte(reason)); werr != nil {
			h.log.Warn("sheet reason not kept", slog.String("error", werr.Error()))
		}
		return nil, &sheetError{reason}
	}
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	if err := json.NewEncoder(z).Encode(wb); err == nil && z.Close() == nil {
		if err := h.options.Store.WriteDerived(tenant, sha+sheetDerived, buf.Bytes()); err != nil {
			h.log.Warn("sheet not kept", slog.String("error", err.Error()))
		}
	}
	return wb, nil
}

// sheetError is why a file cannot be shown as tables, a sentence for a person.
type sheetError struct{ reason string }

func (e *sheetError) Error() string { return e.reason }

// sheetReason is the sentence a person reads when a file cannot be shown as tables.
func sheetReason(err error) string {
	var limit *sheet.LimitError
	switch {
	case errors.As(err, &limit):
		return limit.Error()
	case errors.Is(err, files.ErrBomb):
		return "this file cannot be shown as tables: it unpacks to far more than it is"
	}
	return sheet.ErrUnreadable.Error()
}

func loadWorkbook(path string) (*sheet.Workbook, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	z, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	var wb sheet.Workbook
	if err := json.NewDecoder(io.LimitReader(z, 1<<30)).Decode(&wb); err != nil {
		return nil, err
	}
	return &wb, nil
}

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
	wb, err := h.workbook(r.Context(), h.tenant(r), v.Blob, v.ContentType, v.Name)
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
// without it), ?from= the first visible row to show, counted from 0.
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
	var reason *sheetError
	if errors.As(err, &reason) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": reason.reason})
		return true
	}
	return h.failed(w, err, "sheet")
}
