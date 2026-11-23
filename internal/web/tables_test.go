// SPDX-License-Identifier: MIT

package web

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/sheet"
)

// workbookSpec describes a hand-made workbook of one sheet: cells by address (a number or a text), a table, defined names, the
// program that saved it, and whether the sheet is hidden.
type workbookSpec struct {
	sheet  string
	number int
	cells  map[string]string
	table  [2]string // name, ref
	names  map[string]string
	app    string
	hidden bool
}

func (b workbookSpec) xlsx(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	write := func(name, body string) {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	write("_rels/.rels", `<Relationships><Relationship Id="rId1" Type="x/officeDocument" Target="xl/workbook.xml"/></Relationships>`)
	state := ""
	if b.hidden {
		state = ` state="hidden"`
	}
	var names strings.Builder
	for name, ref := range b.names {
		fmt.Fprintf(&names, `<definedName name="%s">%s</definedName>`, name, ref)
	}
	// A second, visible sheet, so a workbook keeps one visible sheet when the first is hidden.
	write("xl/workbook.xml", `<workbook xmlns:r="r"><sheets><sheet name="`+html.EscapeString(b.sheet)+`" sheetId="`+fmt.Sprint(b.number)+`" r:id="rId1"`+state+`/>`+
		`<sheet name="Notes" sheetId="99" r:id="rId2"/></sheets><definedNames>`+names.String()+`</definedNames></workbook>`)
	write("xl/_rels/workbook.xml.rels", `<Relationships><Relationship Id="rId1" Type="x/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="x/worksheet" Target="worksheets/sheet2.xml"/></Relationships>`)
	rows := map[int][]string{}
	for ref, v := range b.cells {
		r, _, _ := sheet.ParseCell(ref)
		cell := fmt.Sprintf(`<c r="%s" t="inlineStr"><is><t>%s</t></is></c>`, ref, html.EscapeString(v))
		if _, err := fmt.Sscanf(v, "%g", new(float64)); err == nil && !strings.ContainsAny(v, " ") {
			cell = fmt.Sprintf(`<c r="%s"><v>%s</v></c>`, ref, v)
		}
		rows[r] = append(rows[r], cell)
	}
	var data strings.Builder
	for _, r := range slices.Sorted(func(yield func(int) bool) {
		for r := range rows {
			if !yield(r) {
				return
			}
		}
	}) {
		slices.Sort(rows[r])
		fmt.Fprintf(&data, `<row r="%d">%s</row>`, r, strings.Join(rows[r], ""))
	}
	parts := ""
	if b.table[0] != "" {
		parts = `<tableParts><tablePart r:id="rT"/></tableParts>`
		write("xl/worksheets/_rels/sheet1.xml.rels", `<Relationships><Relationship Id="rT" Type="x/table" Target="../tables/table1.xml"/></Relationships>`)
		write("xl/tables/table1.xml", `<table displayName="`+b.table[0]+`" ref="`+b.table[1]+`" headerRowCount="1"/>`)
	}
	write("xl/worksheets/sheet1.xml", `<worksheet><sheetData>`+data.String()+`</sheetData>`+parts+`</worksheet>`)
	write("xl/worksheets/sheet2.xml", `<worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>note</t></is></c></row></sheetData></worksheet>`)
	app := "Microsoft Excel"
	if b.app != "" {
		app = b.app
	}
	write("docProps/app.xml", `<Properties><Application>`+app+`</Application></Properties>`)
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// bobsCollection makes a collection of the files for Bob, his alone, and returns its ID.
func bobsCollection(t *testing.T, s *filesSite) string {
	t.Helper()
	made := s.send(http.MethodPost, "/api/files/collections", `{"title":"Bobs"}`, plain)
	if made.Code != http.StatusCreated {
		t.Fatalf("collection = %d %s", made.Code, made.Body.String())
	}
	return decode[map[string]string](t, made)["id"]
}

// pageHTML is the HTML of every block of a page as a viewer sees it.
func pageHTML(t *testing.T, s *filesSite, page string, headers map[string]string) string {
	t.Helper()
	got := s.getWith("/api/doc/"+page, merged(headers))
	if got.Code != http.StatusOK {
		t.Fatalf("page: %d %s", got.Code, got.Body.String())
	}
	var out strings.Builder
	for _, b := range decode[apiDocument](t, got).Blocks {
		out.WriteString(b.HTML)
	}
	return out.String()
}

func chooseTable(t *testing.T, s *filesSite, file, body string, headers map[string]string, want int) apiTable {
	t.Helper()
	got := s.send(http.MethodPost, "/api/files/"+file+"/tables", body, merged2(sameSite, headers))
	if got.Code != want {
		t.Fatalf("choose %s: %d %s", body, got.Code, got.Body.String())
	}
	if want >= 300 {
		return apiTable{}
	}
	return decode[apiTable](t, got)
}

func appendBlocks(t *testing.T, s *filesSite, page string, headers map[string]string, blocks ...string) {
	t.Helper()
	after := ""
	for _, md := range blocks {
		body, _ := json.Marshal(map[string]string{"md": md, "after": after})
		got := s.send(http.MethodPost, "/api/pages/"+page+"/blocks", string(body), merged2(sameSite, headers))
		if got.Code != http.StatusCreated {
			t.Fatalf("block: %d %s", got.Code, got.Body.String())
		}
		after = decode[apiBlock](t, got).ID
	}
}

// TestTablesOfAWorkbookInAPage (ADR-31): an Excel table, a name of one cell, a range and a sheet of a workbook saved by
// LibreOffice are chosen, get IDs local to the file, and show live in a page; the file and each table know where they are
// used; a reader of the page who may not read the file sees neither cells nor the name of the file.
func TestTablesOfAWorkbookInAPage(t *testing.T) {
	s := withFiles(t)
	mine := s.root(nil, db.RootUser)
	file := decode[finished](t, s.put(nil, fmt.Sprintf(`"parent":%q`, mine.ID), "budget.xlsx", sheetFixture(t, "budget.xlsx"), 4096)).ID
	expenses := chooseTable(t, s, file, `{"kind":"table","name":"Expenses"}`, nil, http.StatusCreated)
	if expenses.ID != "x1" || expenses.Ref != file+"#x1" || expenses.Embed != `::embed{ref="`+file+`#x1"}` || !expenses.Head || expenses.Range != "A1:E5" {
		t.Fatalf("the table: %+v", expenses)
	}
	if again := chooseTable(t, s, file, `{"kind":"table","name":"Expenses"}`, nil, http.StatusOK); again.ID != "x1" {
		t.Errorf("the same table again: %+v", again)
	}
	vat := chooseTable(t, s, file, `{"kind":"name","name":"VAT"}`, nil, http.StatusCreated)
	rng := chooseTable(t, s, file, `{"kind":"range","sheet":"Budget","range":"A1:C3","head":true}`, nil, http.StatusCreated)
	odd := chooseTable(t, s, file, `{"kind":"sheet","sheet":"Odd <&> it's"}`, nil, http.StatusCreated)
	if vat.ID != "x2" || rng.ID != "x3" || odd.ID != "x4" {
		t.Errorf("IDs: %s %s %s", vat.ID, rng.ID, odd.ID)
	}
	for _, body := range []string{`{"kind":"sheet","sheet":"Secret"}`, `{"kind":"range","sheet":"Budget","range":"ZZ"}`, `{"kind":"name","name":"Nope"}`,
		`{"kind":"table","name":"Nope"}`, `{"kind":"cell","sheet":"Budget"}`} {
		chooseTable(t, s, file, body, nil, http.StatusBadRequest)
	}
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	report := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Report","parent":"`+team.ID+`"}`, sameSite)).ID
	appendBlocks(t, s, report, nil, "::embed{ref=\""+file+"#x1\"}", "VAT is :embed{ref=\""+file+"#x2\"} now.", "::embed{ref=\""+file+"#x3\"}",
		"::embed{ref=\""+file+"#x4\"}", "::embed{ref=\""+file+"#x9\"}", "Inline :embed{ref=\""+file+"#x1\"} fails.")
	got := pageHTML(t, s, report, nil)
	for _, want := range []string{`<figure class="editor-embed" data-ref="` + file + `#x1">`, `<th>Item</th>`, `1,234.50 €`, `href="/o/` + file + `#x1"`,
		`VAT is <span data-embed-part="` + file + `#x2"`, `>19%</span> now.`, `Odd &lt;&amp;&gt; it&#39;s`, `Längerer Text`, `Not to be found: ` + file + `#x9`, `Expenses in budget.xlsx is not a single value`} {
		if !strings.Contains(got, want) {
			t.Errorf("the page lacks %s:\n%s", want, got)
		}
	}
	if german := pageHTML(t, s, report, map[string]string{"Accept-Language": "de"}); !strings.Contains(german, "1.234,50 €") || !strings.Contains(german, "WAHR") {
		t.Errorf("in German: %s", german)
	}
	for _, not := range []string{"hidden row", "hidden column", "top secret"} {
		if strings.Contains(got, not) {
			t.Errorf("the page shows %q", not)
		}
	}
	doc := decode[apiDocument](t, s.get("/api/doc/"+report))
	var kinds []string
	for _, e := range doc.Embeds {
		kinds = append(kinds, e.Kind+":"+e.Name)
	}
	// The embedding of a table the file does not have is listed too, without a name, as a block that is gone is.
	if !slices.Equal(kinds, []string{"table:Expenses", "table:VAT", "table:Budget!A1:C3", "table:Odd <&> it's", "table:"}) {
		t.Errorf("the facts of the page: %v", kinds)
	}
	if f := decode[apiFile](t, s.get("/api/files/"+file)); !slices.Contains(f.UsedIn, "Report (table #x1)") || !slices.Contains(f.UsedIn, "Report (table #x2)") {
		t.Errorf("where the file is used: %v", f.UsedIn)
	}
	list := decode[[]apiTable](t, s.get("/api/files/"+file+"/tables"))
	if len(list) != 4 || list[0].Uses != 2 || list[1].Uses != 1 || list[0].State != "ok" {
		t.Errorf("the tables of the file: %+v", list)
	}
	// Carl reads the page but not the file: nothing of it, not even its name.
	carl := pageHTML(t, s, report, carlOf)
	if strings.Contains(carl, "Rent") || strings.Contains(carl, "budget") || strings.Contains(carl, "19%") || strings.Contains(carl, "Expenses") ||
		!strings.Contains(carl, "Not to be found: "+file+"#x1") {
		t.Errorf("Carl sees the file: %s", carl)
	}
	if got := s.getWith("/api/files/"+file+"/tables", merged(carlOf)); got.Code != http.StatusNotFound {
		t.Errorf("Carl lists the tables: %d", got.Code)
	}
	chooseTable(t, s, file, `{"kind":"table","name":"Expenses"}`, carlOf, http.StatusNotFound)
}

// TestTablesFollowNewVersions (ADR-31): rows inserted above, a sheet renamed in Excel, a first row changed, a table and
// a name gone, a sheet renamed by LibreOffice, a sheet hidden: each is followed or told, never shown quietly wrong.
func TestTablesFollowNewVersions(t *testing.T) {
	s := withFiles(t)
	s.send(http.MethodGet, "/api/state", "", sameSite) // Anna is an account of the tenant
	bobs := bobsCollection(t, s)
	first := workbookSpec{sheet: "Data", number: 1, cells: map[string]string{"A1": "Item", "B1": "Amount", "A2": "Rent", "B2": "1000"},
		table: [2]string{"Costs", "A1:B2"}, names: map[string]string{"Total": "Data!$B$2"}}
	file := decode[finished](t, s.put(plain, fmt.Sprintf(`"parent":%q`, bobs), "data.xlsx", first.xlsx(t), 4096)).ID
	// Bob lets Anna read his workbook.
	if got := s.send(http.MethodPut, "/api/objects/"+file+"/grants", `{"kind":"person","who":"1007","role":"read"}`, plain); got.Code != http.StatusNoContent {
		t.Fatalf("grant: %d %s", got.Code, got.Body.String())
	}
	for _, body := range []string{`{"kind":"range","sheet":"Data","range":"A1:B2","head":true}`, `{"kind":"table","name":"Costs"}`,
		`{"kind":"name","name":"Total"}`, `{"kind":"sheet","sheet":"Data"}`} {
		chooseTable(t, s, file, body, nil, http.StatusCreated)
	}
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	report := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Report","parent":"`+team.ID+`"}`, sameSite)).ID
	appendBlocks(t, s, report, nil, "::embed{ref=\""+file+"#x1\"}", "::embed{ref=\""+file+"#x2\"}", "Total :embed{ref=\""+file+"#x3\"}.",
		"::embed{ref=\""+file+"#x4\"}")
	version := func(b workbookSpec) {
		t.Helper()
		if got := s.put(plain, fmt.Sprintf(`"file":%q`, file), "data.xlsx", b.xlsx(t), 4096); got.Code != http.StatusOK && got.Code != http.StatusCreated {
			t.Fatalf("new version: %d %s", got.Code, got.Body.String())
		}
	}
	states := func() map[string]apiTable {
		out := map[string]apiTable{}
		for _, tb := range decode[[]apiTable](t, s.get("/api/files/"+file+"/tables")) {
			out[tb.ID] = tb
		}
		return out
	}

	// Two rows inserted above, and the sheet renamed in Excel: everything follows.
	second := workbookSpec{sheet: "Numbers", number: 1, cells: map[string]string{"A3": "Item", "B3": "Amount", "A4": "Rent", "B4": "1000", "A1": "Budget 2027"},
		table: [2]string{"Costs", "A3:B4"}, names: map[string]string{"Total": "Numbers!$B$4"}}
	version(second)
	got := pageHTML(t, s, report, nil)
	if strings.Count(got, "Rent") != 3 || !strings.Contains(got, `title="data.xlsx · Total">1000</span>.`) || !strings.Contains(got, "data-table-moved") ||
		!strings.Contains(got, "Followed from Data!A1:B2") {
		t.Errorf("after rows above and a rename: %s", got)
	}
	st := states()
	if st["x1"].State != "moved" || st["x1"].Range != "A3:B4" || st["x1"].Sheet != "Numbers" || st["x4"].State != "moved" || st["x4"].Moved != "Data" {
		t.Errorf("states: %+v", st)
	}

	// The first row of the range changed and is nowhere else: it shows, with a notice, and is a problem until kept.
	third := workbookSpec{sheet: "Numbers", number: 1, app: "LibreOffice/26.8", cells: map[string]string{"A3": "Thing", "B3": "Amount", "A4": "Rent", "B4": "1000"},
		table: [2]string{"Costs", "A3:B4"}, names: map[string]string{"Total": "Numbers!$B$4"}}
	version(third)
	got = pageHTML(t, s, report, nil)
	if !strings.Contains(got, "data-table-changed") || !strings.Contains(got, "Thing") {
		t.Errorf("a changed first row: %s", got)
	}
	problems := decode[apiProblems](t, s.get("/api/problems"))
	if !slices.ContainsFunc(problems.Database, func(p apiProblem) bool { return p.Ref == file+"#x1" && strings.Contains(p.Why, "first row changed") }) {
		t.Errorf("problems: %+v", problems.Database)
	}
	if got := s.send(http.MethodPost, "/api/files/"+file+"/tables/x1/keep", "", sameSite); got.Code != http.StatusForbidden {
		t.Errorf("Anna, who only reads the file, keeps the range: %d", got.Code)
	}
	if got := s.send(http.MethodPost, "/api/files/"+file+"/tables/x1/keep", "", plain); got.Code != http.StatusOK {
		t.Fatalf("Bob keeps the range: %d %s", got.Code, got.Body.String())
	}
	if got := pageHTML(t, s, report, nil); strings.Contains(got, "data-table-changed") {
		t.Errorf("kept, the notice stays: %s", got)
	}

	// The table and the name are gone, and LibreOffice renamed the sheet: each says so, nothing of the old content shows.
	fourth := workbookSpec{sheet: "Other", number: 1, app: "LibreOffice/26.8", cells: map[string]string{"A3": "Thing", "B3": "Amount", "A4": "Rent", "B4": "1000"}}
	version(fourth)
	got = pageHTML(t, s, report, nil)
	for _, want := range []string{"Table Costs is no longer in data.xlsx (version 4 of", "Name Total is no longer in data.xlsx",
		"Sheet Numbers is no longer in data.xlsx", "Range Numbers!A3:B4 is no longer in data.xlsx"} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %q: %s", want, got)
		}
	}
	if strings.Contains(got, "Rent") || strings.Contains(got, "1000") {
		t.Errorf("a lost table shows content: %s", got)
	}
	if st := states(); st["x2"].State != "lost" || !strings.Contains(st["x2"].Problem, "no longer") {
		t.Errorf("states: %+v", st)
	}

	// Hidden stays hidden: a sheet hidden in a new version shows nothing of it.
	fifth := fourth
	fifth.sheet, fifth.hidden = "Numbers", true
	version(fifth)
	if got := pageHTML(t, s, report, nil); !strings.Contains(got, "Sheet Numbers is hidden in data.xlsx") || strings.Contains(got, "Rent") {
		t.Errorf("a hidden sheet: %s", got)
	}
}

// TestTablesInReadLinks (ADR-31, ADR-29): a frozen page shows an embedded table only if its file was released for the
// link, and only while the maker of the link reads the file.
func TestTablesInReadLinks(t *testing.T) {
	s := withFiles(t)
	s.send(http.MethodGet, "/api/state", "", sameSite) // Anna is an account of the tenant
	bobs := bobsCollection(t, s)
	b := workbookSpec{sheet: "Data", number: 1, cells: map[string]string{"A1": "Item", "A2": "Salary 4711"}, names: map[string]string{"Pay": "Data!$A$2"}}
	file := decode[finished](t, s.put(plain, fmt.Sprintf(`"parent":%q`, bobs), "pay.xlsx", b.xlsx(t), 4096)).ID
	if got := s.send(http.MethodPut, "/api/objects/"+file+"/grants", `{"kind":"person","who":"1007","role":"read"}`, plain); got.Code != http.StatusNoContent {
		t.Fatalf("grant: %d", got.Code)
	}
	chooseTable(t, s, file, `{"kind":"sheet","sheet":"Data"}`, nil, http.StatusCreated)
	chooseTable(t, s, file, `{"kind":"name","name":"Pay"}`, nil, http.StatusCreated)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Menu","parent":"`+team.ID+`"}`, sameSite)).ID
	appendBlocks(t, s, page, nil, "::embed{ref=\""+file+"#x1\"}", "Pay is :embed{ref=\""+file+"#x2\"}.")
	shareables := decode[[]apiShareable](t, s.get("/api/objects/"+page+"/shareables"))
	if len(shareables) != 1 || shareables[0].ID != file {
		t.Fatalf("the file is to be released: %+v", shareables)
	}
	without := pathOf(t, decode[apiLink](t, s.send(http.MethodPost, "/api/objects/"+page+"/links", `{"mode":"read"}`, sameSite)).URL)
	if got := s.anonymous(http.MethodGet, without, nil, nil).Body.String(); strings.Contains(got, "4711") || !strings.Contains(got, "Not shared") {
		t.Errorf("a link that did not release the file: %s", got)
	}
	with := pathOf(t, decode[apiLink](t, s.send(http.MethodPost, "/api/objects/"+page+"/links", `{"mode":"read","released":["`+file+`"]}`, sameSite)).URL)
	if got := s.anonymous(http.MethodGet, with, nil, nil).Body.String(); strings.Count(got, "4711") != 2 {
		t.Errorf("a link that released the file: %s", got)
	}
	// Bob takes Anna's role away: the frozen tables of her link go.
	if got := s.send(http.MethodPut, "/api/objects/"+file+"/grants", `{"kind":"person","who":"1007","role":""}`, plain); got.Code != http.StatusNoContent {
		t.Fatalf("take: %d", got.Code)
	}
	if got := s.anonymous(http.MethodGet, with, nil, nil).Body.String(); strings.Contains(got, "4711") || strings.Count(got, "Not shared") != 2 {
		t.Errorf("the maker no longer reads the file: %s", got)
	}
}

// TestAnAnchorKeptLongIsComparedShort: an anchor kept before anchors were cut to sheet.MaxAnchorText characters still
// finds its range, rather than turning every range of long texts into one whose first row changed.
func TestAnAnchorKeptLongIsComparedShort(t *testing.T) {
	long := strings.Repeat("q", sheet.MaxCellText)
	wb := &sheet.Workbook{Formats: []string{"General"}, Sheets: []sheet.Sheet{{Name: "S", Rows: []sheet.Row{{N: 2, Cells: []sheet.Cell{
		{Col: 1, Kind: sheet.Text, Value: long}, {Col: 2, Kind: sheet.Text, Value: "b"}}}}}}}
	rec := db.FileTable{Kind: db.TableRange, Sheet: "S", Range: "A2:B3", Anchor: []string{long, "b"}, FoundBlob: "old"}
	next := locate(rec, wb, "new")
	if next.Changed || next.Lost != "" || next.Range != "A2:B3" || next.MovedFrom != "" {
		t.Errorf("a long anchor: %+v", next)
	}
}

// A page frozen for a link before T-12 step 3 holds the old markup of an embedded table: it is taken out as surely as
// one frozen since, when its maker no longer reads the file.
func TestFrozenTablesKnowTheMarkupOfBefore(t *testing.T) {
	for _, figure := range []string{
		`<figure class="embed" data-ref="mqk47jxx57#x1"><div class="embedbody"><table><tr><td>4711</td></tr></table></div><figcaption>pay.xlsx</figcaption></figure>`,
		`<figure class="editor-embed" data-ref="mqk47jxx57#x1"><div class="table-container"><table><tr><td>4711</td></tr></table></div><figcaption>pay.xlsx</figcaption></figure>`,
	} {
		if got := frozenTable.FindString("<p>Pay:</p>" + figure + "<p>End.</p>"); got != figure {
			t.Errorf("the frozen table is not found whole in %s: %q", figure, got)
		}
	}
}
