//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestSpreadsheetInTheWorkspace (ADR-31): a workbook saved by LibreOffice opens as tables: its visible sheets as tabs, a
// head that stays while the table scrolls, hidden rows and columns left out, text from the file as text; a range marked
// with the mouse and with Shift and the arrows is embedded from the view, and the page shows it; an address of a hit of
// the search opens the view at its sheet. Nothing is loaded from elsewhere.
func TestSpreadsheetInTheWorkspace(t *testing.T) {
	b, base, _, _ := setupWithFiles(t)
	b.open(base + "/")
	b.ready("")
	b.openMyFiles()
	fixture, err := os.ReadFile(filepath.Join("..", "sheet", "testdata", "libreoffice", "budget.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	b.choose(`[aria-label=Nav3] [data-act="fup"]`, writeTemp(t, "budget.xlsx", fixture))
	b.filesSettled()
	b.clickOn(probe(b, `[aria-label=Nav3] [data-act="fopen"]`, "budget.xlsx"), 1)
	b.ready("budget.xlsx")
	grid := `article.typeset [data-sheet-grid]`
	if !b.waitFor(`!!document.querySelector('` + grid + ` td[data-r="2"][data-c="2"]')`) {
		t.Fatalf("no table view: %s", b.eval(`document.querySelector('article.typeset')?.innerText.slice(0, 600)`))
	}
	if got := b.eval(`[...document.querySelectorAll('article.typeset [data-sheet-tab]')].map(n => n.textContent + ':' + n.getAttribute('aria-pressed')).join(', ')`); got != "Budget:true, Odd <&> it's:false" {
		t.Errorf("the tabs: %q", got)
	}
	// Numbers read in the language of the browser (Accept-Language), German or English.
	money := "1,234.50 €"
	if b.eval(`navigator.language.toLowerCase().startsWith('de')`) == "true" {
		money = "1.234,50 €"
	}
	cell := func(r, c int) string {
		return b.eval(`document.querySelector('` + grid + ` td[data-r="` + strconv.Itoa(r) + `"][data-c="` + strconv.Itoa(c) + `"]')?.textContent ?? 'none'`)
	}
	for _, c := range []struct {
		r, col int
		want   string
	}{{1, 1, "Item"}, {2, 2, money}, {4, 1, "Coffee & <tea>"}, {6, 3, "#DIV/0!"}, {7, 1, "none"}, {2, 7, "none"}, {12, 6, "<script>alert(1)</script>"}} {
		if got := cell(c.r, c.col); got != c.want {
			t.Errorf("%d,%d: %q, want %q", c.r, c.col, got, c.want)
		}
	}
	if b.exists(grid + ` script`) {
		t.Error("text of the file became markup")
	}
	if got := b.eval(`document.querySelector('` + grid + ` td[data-r="9"][data-c="1"]').colSpan`); got != "3" {
		t.Errorf("the merged cells span %s", got)
	}
	if got := b.eval(`getComputedStyle(document.querySelector('` + grid + ` thead')).position + ' ' + getComputedStyle(document.querySelector('` + grid + ` tbody th')).position`); got != "sticky sticky" {
		t.Errorf("the head and the row numbers do not stay: %s", got)
	}
	b.shot("sheet-view")

	// A range by dragging: A1 to C3.
	from, to := b.rect(grid+` td[data-r="1"][data-c="1"]`), b.rect(grid+` td[data-r="3"][data-c="3"]`)
	b.mouse("mouseMoved", from.X, from.Y, 0, 0)
	b.mouse("mousePressed", from.X, from.Y, 1, 0)
	b.mouse("mouseMoved", (from.X+to.X)/2, (from.Y+to.Y)/2, 0, 0)
	b.mouse("mouseMoved", to.X, to.Y, 0, 0)
	b.mouse("mouseReleased", to.X, to.Y, 1, 0)
	embed := `document.querySelector('article.typeset [data-sheet-act="embed"]').textContent`
	if !b.waitFor(embed + ` === 'Embed A1:C3'`) {
		t.Fatalf("the dragged range: %s", b.eval(embed))
	}
	if got := b.eval(`document.querySelectorAll('` + grid + ` td[aria-selected="true"]').length`); got != "9" {
		t.Errorf("marked cells: %s", got)
	}
	// With the keyboard: a click on B2, then Shift and the arrows.
	b.clickOn(grid+` td[data-r="2"][data-c="2"]`, 1)
	b.press("ArrowDown", "ArrowDown", 40, 8)
	b.press("ArrowRight", "ArrowRight", 39, 8)
	if !b.waitFor(embed + ` === 'Embed B2:C3'`) {
		t.Fatalf("the range by the keyboard: %s", b.eval(embed))
	}
	b.clickOn(`article.typeset [data-sheet-act="embed"]`, 1)
	said := `document.querySelector('article.typeset [data-sheet-say]')?.textContent ?? ''`
	if !b.waitFor(`(` + said + `).includes('::embed{ref=')`) {
		t.Fatalf("the line to embed: %s", b.eval(said))
	}
	line := b.eval(`(` + said + `).match(/::embed\{ref="[^"]+"\}/)[0]`)
	if !b.waitFor(`!!document.querySelector('article.typeset [data-sheet-table="x1"]')`) {
		t.Fatalf("the embedded table is not listed: %s", b.eval(`document.querySelector('article.typeset')?.innerText.slice(-400)`))
	}
	// A page embeds it and shows it.
	page := b.eval(`(() => { const send = (m, u, body) => { const x = new XMLHttpRequest(); x.open(m, u, false); x.setRequestHeader('Content-Type', 'application/json'); x.send(JSON.stringify(body)); return JSON.parse(x.responseText); };
		const c = send('POST', '/api/collections', {title: 'Reports'}); const p = send('POST', '/api/pages', {title: 'Budget report', parent: c.id});
		send('POST', '/api/pages/' + p.id + '/blocks', {md: ` + strconv.Quote(line) + `}); return p.id; })()`)
	b.open(base + "/o/" + page)
	b.ready("Budget report")
	if !b.waitFor(`[...document.querySelectorAll('article.typeset figure.embed td')].some(td => td.textContent === '` + money + `')`) {
		t.Fatalf("the page does not show the table: %s", b.eval(`document.querySelector('article.typeset')?.innerHTML.slice(0, 800)`))
	}
	b.shot("sheet-in-page")

	// An address of a hit of the search opens the view at its sheet.
	file := b.eval(`(() => { const x = new XMLHttpRequest(); x.open('GET', '/api/doc/` + page + `', false); x.send(); return JSON.parse(x.responseText).embeds[0].ref.split('#')[0]; })()`)
	b.open(base + "/o/" + file + "#" + url.PathEscape("Odd <&> it's!A1:B2"))
	if !b.waitFor(`document.querySelector('article.typeset [data-sheet-tab][aria-pressed="true"]')?.textContent === "Odd <&> it's"`) {
		t.Fatalf("the hit does not open its sheet: %s", b.eval(`document.querySelector('article.typeset')?.innerText.slice(0, 400)`))
	}
	if !b.waitFor(`[...document.querySelectorAll('` + grid + ` td')].some(td => td.textContent.startsWith('Längerer Text'))`) {
		t.Error("the sheet of the hit shows no cells")
	}
	if got := b.eval(`performance.getEntriesByType('resource').map(e => e.name).filter(n => !n.startsWith(location.origin)).join(' ')`); got != "" {
		t.Errorf("loaded from elsewhere: %s", got)
	}
	if strings.Contains(b.eval(`document.documentElement.outerHTML`), "<script>alert(1)</script>") {
		t.Error("a script of the file is in the page")
	}
}
