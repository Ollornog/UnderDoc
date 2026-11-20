// SPDX-License-Identifier: MIT

package web

import (
	"errors"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/route"
	"github.com/Ollornog/UnderDoc/internal/sheet"
)

// Tables of spreadsheets in pages (ADR-31): a sheet, a range, a defined name or an Excel table of a file gets an ID local
// to the file when it is chosen, <file>#x3, and ::embed{ref=…} shows it live from the current version, for each viewer.
// When the file has a new version, the table is looked for again: by its name, a sheet renamed in Excel by its number,
// a range by its first row. What cannot be followed says so; nothing shows quietly wrong.

// embedRows is how many rows an embedded table shows; the rest is a link to the file.
const embedRows = 200

// tableView is a table as found in the current version of its file.
type tableView struct {
	rec     db.FileTable
	title   string
	version db.FileVersion
	wb      *sheet.Workbook
	sheet   *sheet.Sheet
	rect    sheet.Rect
	head    bool
	// problem is why it cannot be shown: the file cannot be read, or the table is lost. Nothing of it shows then.
	problem string
}

// lostReasons are what Lost keeps; the sentence is made when it shows, with the current name of the file.
const (
	lostGone   = "gone"
	lostHidden = "hidden"
)

// tableOf finds a table of a file in the file's current version, for the viewer of the request: db.ErrNotFound when the
// viewer may not read the file or the file has no such table.
func (h *handler) tableOf(r *http.Request, fileID, id string) (tableView, error) {
	ctx := r.Context()
	user, _ := account.From(ctx)
	rec, err := h.options.DB.FileTableOf(ctx, fileID, id, user.ID)
	if err != nil {
		return tableView{}, err
	}
	f, err := h.options.DB.File(ctx, fileID, user.ID)
	if err != nil {
		return tableView{}, err
	}
	v, err := h.options.DB.FileVersion(ctx, fileID, user.ID, 0)
	if err != nil {
		return tableView{}, err
	}
	view := tableView{rec: rec, title: f.Title, version: v}
	if !sheet.Readable(v.ContentType) {
		view.problem = f.Title + " is no longer a spreadsheet"
		return view, nil
	}
	wb, err := h.workbook(ctx, h.tenant(r), v.Blob, v.ContentType, v.Name)
	var reason *sheetError
	switch {
	case errors.As(err, &reason):
		view.problem = f.Title + ": " + reason.reason
		return view, nil
	case err != nil:
		return tableView{}, err
	}
	view.wb = wb
	if rec.FoundBlob != v.Blob {
		next := locate(rec, wb, v.Blob)
		if err := h.options.DB.TableFound(ctx, rec.FoundBlob, next); err != nil {
			h.log.Warn("table found", slog.String("error", err.Error()))
		}
		view.rec = next
	}
	return view.place(), nil
}

// place sets the sheet and the rectangle of a table found in its workbook, or why it cannot be shown.
func (v tableView) place() tableView {
	rec := v.rec
	if rec.Lost != "" {
		what := map[string]string{db.TableSheet: "Sheet", db.TableRange: "Range", db.TableName: "Name", db.TableTable: "Table"}[rec.Kind]
		where := "is no longer in"
		if rec.Lost == lostHidden {
			where = "is hidden in"
		}
		v.problem = fmt.Sprintf("%s %s %s %s (version %d of %s)", what, rec.Label(), where, v.title, v.version.Number,
			v.version.At.UTC().Format("2 Jan 2006"))
		return v
	}
	s, visible := v.wb.Sheet(rec.Sheet)
	if !visible {
		v.problem = "Sheet " + rec.Sheet + " is no longer in " + v.title
		return v
	}
	v.sheet = s
	switch rec.Kind {
	case db.TableSheet:
		v.rect, v.head = s.Used(), rec.Head
	case db.TableRange:
		v.rect, _ = sheet.ParseRect(rec.Range)
		v.head = rec.Head
	case db.TableName:
		n, _ := findName(v.wb, rec)
		v.rect, v.head = n.Rect, rec.Head
	case db.TableTable:
		t, _ := findTable(v.wb, rec)
		v.rect, v.head = t.Rect, t.Header == 1
	}
	return v
}

func findName(wb *sheet.Workbook, rec db.FileTable) (sheet.Name, bool) {
	var found []sheet.Name
	for _, n := range wb.Names {
		if n.Name == rec.Name {
			found = append(found, n)
		}
	}
	if len(found) == 0 {
		return sheet.Name{}, false
	}
	// A name of the workbook and a name of a sheet may share their text: the one on the sheet it was on comes first.
	if i := slices.IndexFunc(found, func(n sheet.Name) bool { return n.Sheet == rec.Sheet }); i >= 0 {
		return found[i], true
	}
	return found[0], true
}

func findTable(wb *sheet.Workbook, rec db.FileTable) (sheet.Table, bool) {
	for _, t := range wb.Tables {
		if t.Name == rec.Name {
			return t, true
		}
	}
	return sheet.Table{}, false
}

// locate looks for a table in a new version of its file and says what came of it (ADR-31).
func locate(rec db.FileTable, wb *sheet.Workbook, blob string) db.FileTable {
	next := rec
	next.FoundBlob, next.FoundApp, next.MovedFrom, next.Changed, next.Lost = blob, wb.App, "", false, ""
	lost := func(why string) db.FileTable {
		next.Lost = why
		return next
	}
	switch rec.Kind {
	case db.TableName:
		n, ok := findName(wb, rec)
		if !ok {
			return lost(lostGone)
		}
		if _, visible := wb.Sheet(n.Sheet); !visible {
			return lost(lostHidden)
		}
		next.Sheet, next.Range = n.Sheet, n.Rect.String()
		return next
	case db.TableTable:
		t, ok := findTable(wb, rec)
		if !ok {
			return lost(lostGone)
		}
		if _, visible := wb.Sheet(t.Sheet); !visible {
			return lost(lostHidden)
		}
		next.Sheet, next.Range = t.Sheet, t.Rect.String()
		return next
	}
	// A sheet, or the sheet of a range: by its name, or, from Excel to Excel, by its number when its name is gone.
	s, visible := wb.Sheet(rec.Sheet)
	switch {
	case s != nil && !visible:
		return lost(lostHidden)
	case s == nil && rec.SheetNumber > 0 && rec.FoundApp == "excel" && wb.App == "excel":
		for i := range wb.Sheets {
			if wb.Sheets[i].Number == rec.SheetNumber && !wb.Sheets[i].Hidden {
				s = &wb.Sheets[i]
				next.MovedFrom, next.Sheet = rec.Sheet, s.Name
			}
		}
	}
	if s == nil {
		return lost(lostGone)
	}
	next.SheetNumber = s.Number
	if rec.Kind == db.TableSheet {
		return next
	}
	rect, ok := sheet.ParseRect(rec.Range)
	if !ok {
		return lost(lostGone)
	}
	current := wb.Anchor(s, rect.R1, rect.C1, rect.C2)
	if slices.Equal(current, rec.Anchor) || !slices.ContainsFunc(rec.Anchor, func(a string) bool { return a != "" }) {
		return next
	}
	found := wb.FindAnchor(s, rec.Anchor)
	if len(found) != 1 {
		next.Changed = true
		return next
	}
	moved := sheet.Rect{R1: found[0][0], C1: found[0][1], R2: found[0][0] + rect.R2 - rect.R1, C2: found[0][1] + rect.C2 - rect.C1}
	if !moved.Valid() {
		next.Changed = true
		return next
	}
	if next.MovedFrom == "" {
		next.MovedFrom = rec.Sheet + "!" + rec.Range
	} else {
		next.MovedFrom += "!" + rec.Range
	}
	next.Range = moved.String()
	return next
}

// embeddedTable renders a table of a file as an embedded block: its grid, at most embedRows rows, with what happened to
// it since it was chosen.
func (h *handler) embeddedTable(r *http.Request, o db.Object, a db.Address) (inner, label, why string) {
	view, err := h.tableOf(r, o.ID, a.Block)
	if err != nil {
		return "", "", "Not to be found: " + a.String()
	}
	if view.problem != "" {
		return "", "", view.problem
	}
	label = view.title + " · " + view.rec.Label()
	lang := sheet.LangOf(r.Header.Get("Accept-Language"))
	grid := view.wb.Grid(view.sheet, view.rect, lang, 0, embedRows)
	var out []byte
	if view.rec.Changed {
		out = append(out, `<p class="text-sm text-warning" data-table-changed>The first row of this range changed since it was chosen, and it was not found elsewhere: check that it still shows what it should.</p>`...)
	}
	if view.rec.MovedFrom != "" {
		out = append(out, `<p class="text-sm text-muted-foreground" data-table-moved>Followed from `+html.EscapeString(view.rec.MovedFrom)+` in version `+strconv.Itoa(view.version.Number)+`.</p>`...)
	}
	out = append(out, sheet.HTML(grid, view.head, "")...)
	if grid.More > 0 {
		out = append(out, `<p class="text-sm text-muted-foreground"><a href="`+html.EscapeString(route.Object(o.ID)+"#"+a.Block)+`">`+
			strconv.Itoa(grid.More)+` more rows in `+html.EscapeString(view.title)+`</a></p>`...)
	}
	return string(out), label, ""
}

// inlineTable renders a table of one cell inside a paragraph, as a value (M-3): its text.
func (h *handler) inlineTable(r *http.Request, a db.Address) (inner, label, why string) {
	if unreleased(r.Context(), a.Object) {
		return "", "", "Not shared: " + a.String()
	}
	view, err := h.tableOf(r, a.Object, a.Block)
	if err != nil {
		return "", "", "Not to be found: " + a.String()
	}
	if view.problem != "" {
		return "", "", view.problem
	}
	if view.rect.Cells() != 1 {
		return "", "", view.rec.Label() + " in " + view.title + " is not a single value"
	}
	grid := view.wb.Grid(view.sheet, view.rect, sheet.LangOf(r.Header.Get("Accept-Language")), 0, 1)
	if len(grid.Rows) == 0 || len(grid.Rows[0].Cells) == 0 {
		return "", "", view.rec.Label() + " is hidden in " + view.title
	}
	return html.EscapeString(grid.Rows[0].Cells[0].Text), view.title + " · " + view.rec.Label(), ""
}

// --- API

type apiTable struct {
	ID    string `json:"id"`
	Ref   string `json:"ref"`
	URL   string `json:"url"`
	Embed string `json:"embed"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Sheet string `json:"sheet"`
	Range string `json:"range"`
	Head  bool   `json:"head"`
	// State is ok, moved, changed or lost; Problem the sentence when it cannot be shown.
	State   string `json:"state"`
	Moved   string `json:"moved,omitempty"`
	Problem string `json:"problem,omitempty"`
	Uses    int    `json:"uses"`
}

func tableOut(v tableView, uses int) apiTable {
	rec := v.rec
	out := apiTable{ID: rec.ID, Ref: rec.Ref(), URL: route.Object(rec.File) + "#" + rec.ID, Embed: `::embed{ref="` + rec.Ref() + `"}`,
		Kind: rec.Kind, Label: rec.Label(), Sheet: rec.Sheet, Range: rec.Range, Head: v.head, State: "ok", Moved: rec.MovedFrom, Uses: uses}
	if v.rect.R2 > 0 {
		out.Range = v.rect.String()
	}
	switch {
	case v.problem != "":
		out.State, out.Problem = "lost", v.problem
	case rec.Changed:
		out.State = "changed"
	case rec.MovedFrom != "":
		out.State = "moved"
	}
	return out
}

// apiChooseTable gives a table of a spreadsheet its ID (ADR-31): {"kind": "sheet"|"range"|"name"|"table", "sheet",
// "range", "name", "head"}, checked against the current version. Whoever reads the file chooses; the same table again
// gets the same ID.
func (h *handler) apiChooseTable(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	var in struct {
		Kind  string `json:"kind"`
		Sheet string `json:"sheet"`
		Range string `json:"range"`
		Name  string `json:"name"`
		Head  bool   `json:"head"`
	}
	user, ok := h.change(w, r, 8<<10, &in)
	if !ok {
		return
	}
	v, wb, err := h.fileWorkbook(r, r.PathValue("id"))
	if h.sheetFailed(w, err) {
		return
	}
	rec := db.FileTable{Kind: in.Kind, Sheet: in.Sheet, Head: in.Head, FoundBlob: v.Blob, FoundApp: wb.App}
	bad := func(why string) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": why})
	}
	switch in.Kind {
	case db.TableName:
		n, found := findName(wb, db.FileTable{Name: in.Name, Sheet: in.Sheet})
		if _, visible := wb.Sheet(n.Sheet); !found || !visible {
			bad("no such name on a visible sheet")
			return
		}
		rec.Name, rec.Sheet, rec.Range = n.Name, n.Sheet, n.Rect.String()
	case db.TableTable:
		t, found := findTable(wb, db.FileTable{Name: in.Name})
		if _, visible := wb.Sheet(t.Sheet); !found || !visible {
			bad("no such table on a visible sheet")
			return
		}
		rec.Name, rec.Sheet, rec.Range, rec.Head = t.Name, t.Sheet, t.Rect.String(), t.Header == 1
	case db.TableSheet, db.TableRange:
		s, visible := wb.Sheet(in.Sheet)
		if !visible {
			bad("no such sheet")
			return
		}
		rec.SheetNumber = s.Number
		if in.Kind == db.TableRange {
			rect, ok := sheet.ParseRect(in.Range)
			if !ok || !rect.Valid() {
				bad("range is a range of cells such as A1:D20")
				return
			}
			rec.Range, rec.Anchor = rect.String(), wb.Anchor(s, rect.R1, rect.C1, rect.C2)
		}
	default:
		bad(db.ErrTable.Error())
		return
	}
	made, fresh, err := h.options.DB.ChooseTable(r.Context(), r.PathValue("id"), user.ID, rec)
	if errors.Is(err, db.ErrTable) {
		bad(err.Error())
		return
	}
	if h.failed(w, err, "choose table") {
		return
	}
	view, err := h.tableOf(r, made.File, made.ID)
	if h.failed(w, err, "choose table") {
		return
	}
	status := http.StatusOK
	if fresh {
		status = http.StatusCreated
	}
	writeJSON(w, status, tableOut(view, 0))
}

// apiFileTables lists the tables of a file that have an ID, each as found in the current version, with how many places
// in pages the viewer sees embed it.
func (h *handler) apiFileTables(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	user, _ := account.From(r.Context())
	list, err := h.options.DB.FileTables(r.Context(), r.PathValue("id"), user.ID)
	if h.failed(w, err, "tables") {
		return
	}
	uses, err := h.options.DB.TableUses(r.Context(), r.PathValue("id"), user.ID)
	if h.failed(w, err, "tables") {
		return
	}
	out := []apiTable{}
	for _, t := range list {
		view, err := h.tableOf(r, t.File, t.ID)
		if h.failed(w, err, "tables") {
			return
		}
		out = append(out, tableOut(view, uses[t.ID]))
	}
	writeJSON(w, http.StatusOK, out)
}

// apiKeepTable keeps a range whose first row changed as it is now (ADR-31), for whoever may edit the file.
func (h *handler) apiKeepTable(w http.ResponseWriter, r *http.Request) {
	if !h.filesReady(w) {
		return
	}
	user, ok := h.change(w, r, 0, nil)
	if !ok {
		return
	}
	view, err := h.tableOf(r, r.PathValue("id"), r.PathValue("table"))
	if h.failed(w, err, "keep table") {
		return
	}
	if view.problem != "" || view.rec.Kind != db.TableRange {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "only a range that shows can be kept as it is"})
		return
	}
	anchor := view.wb.Anchor(view.sheet, view.rect.R1, view.rect.C1, view.rect.C2)
	if h.failed(w, h.options.DB.KeepTable(r.Context(), r.PathValue("id"), r.PathValue("table"), user.ID, anchor), "keep table") {
		return
	}
	view.rec.Changed, view.rec.MovedFrom = false, ""
	writeJSON(w, http.StatusOK, tableOut(view, 0))
}

// tableProblem says why an embedded table does not show, for the list of problems; "" when it shows.
func (h *handler) tableProblem(r *http.Request, a db.Address) string {
	if h.options.Store == nil {
		return "embeds a table, and files are off"
	}
	view, err := h.tableOf(r, a.Object, a.Block)
	switch {
	case err != nil:
		return "embeds a table that is not to be found"
	case view.problem != "":
		return "embeds a table that does not show: " + view.problem
	case view.rec.Changed:
		return "embeds a range whose first row changed: " + view.rec.Label() + " in " + view.title
	}
	return ""
}

var (
	frozenTable      = regexp.MustCompile(`(?s)<figure class="embed" data-ref="([0-9a-hjkmnp-tv-z]{10})#x[1-9][0-9]{0,8}">.*?</figure>`)
	frozenTableValue = regexp.MustCompile(`<span data-embed-part="([0-9a-hjkmnp-tv-z]{10})#x[1-9][0-9]{0,8}" title="[^"]*">[^<]*</span>`)
)

// frozenTables takes out of a frozen page the tables of files its maker no longer reads (ADR-31): they were released
// for the link and rendered when it was frozen, and show only as long as the maker still reads the file, the rule of
// attachments in links.
func (h *handler) frozenTables(r *http.Request, frozen, maker string) string {
	if !strings.Contains(frozen, "#x") {
		return frozen
	}
	reads := map[string]bool{}
	may := func(file string) bool {
		ok, seen := reads[file]
		if !seen {
			_, err := h.options.DB.File(r.Context(), file, maker)
			ok = err == nil
			reads[file] = ok
		}
		return ok
	}
	frozen = frozenTable.ReplaceAllStringFunc(frozen, func(figure string) string {
		if may(frozenTable.FindStringSubmatch(figure)[1]) {
			return figure
		}
		return `<div class="embed broken">Not shared</div>`
	})
	return frozenTableValue.ReplaceAllStringFunc(frozen, func(span string) string {
		if may(frozenTableValue.FindStringSubmatch(span)[1]) {
			return span
		}
		return `<span class="badge" data-variant="destructive">Not shared</span>`
	})
}
