//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"strconv"
	"strings"
	"testing"
)

// rightClickOn opens the menu of an element with the real right mouse button.
func (b *browser) rightClickOn(selector string) {
	b.t.Helper()
	r := b.rect(selector)
	b.mouse("mouseMoved", r.X, r.Y, 0, 0)
	for _, kind := range []string{"mousePressed", "mouseReleased"} {
		b.call("Input.dispatchMouseEvent", map[string]any{"type": kind, "x": r.X, "y": r.Y, "button": "right", "buttons": 2, "clickCount": 1}, nil)
	}
}

// dragTo holds the mouse on one element and moves it in steps to a point of another: fraction 0 is its top edge, 1 its
// bottom. Before letting go it runs check, which sees the page while the pointer is still held.
func (b *browser) dragTo(from, to string, fraction float64, check func()) {
	b.t.Helper()
	a := b.rect(from)
	b.mouse("mouseMoved", a.X, a.Y, 0, 0)
	b.mouse("mousePressed", a.X, a.Y, 1, 0)
	z := b.rect(to)
	y := z.Top + (z.Bottom-z.Top)*fraction
	for step := 1; step <= 8; step++ {
		b.mouse("mouseMoved", a.X+(z.X-a.X)*float64(step)/8, a.Y+(y-a.Y)*float64(step)/8, 0, 0)
	}
	if check != nil {
		check()
	}
	b.mouse("mouseReleased", z.X, y, 1, 0)
}

// makeTeam makes a collection with pages A, B and C through the API, as a setup, and returns its ID.
func makeTeam(b *browser) string {
	b.t.Helper()
	b.eval(`window.made = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		const team = await post('/api/collections', {title: 'Team'});
		for (const title of ['A', 'B', 'C']) await post('/api/pages', {title, parent: team.id});
		window.made = team.id;
	})()`)
	if !b.waitFor(`!!window.made`) {
		b.t.Fatal("the collection for the test was not made")
	}
	return b.eval(`window.made`)
}

func TestNav2ShowsWhatIsKeptOpen(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	bar := func(id string) string {
		return b.eval(`getComputedStyle(document.querySelector('.nav2 [data-act="coll"][data-id="` + id + `"]')).boxShadow`)
	}
	b.clickOn(`.nav2 [data-act="coll"][data-id="ops"]`, 1)
	if got := bar("ops"); got != "none" {
		t.Errorf("open for now: no line on the left, got %q", got)
	}
	b.clickOn(`.nav2 [data-act="coll"][data-id="ops"]`, 2)
	if !b.waitFor(`!!document.querySelector('.nav2 [data-id="ops"][data-pinned]')`) {
		t.Fatal("a double click keeps it open")
	}
	if got := bar("ops"); got == "none" {
		t.Error("kept open: the line on the left is missing")
	}
	if got := b.eval(`document.querySelectorAll('.nav2 .star, .nav2 .pinmark, [data-act="star"]').length`); got != "0" {
		t.Errorf("no star and no pin any more: %s", got)
	}
	for s := 0; s <= 3; s++ {
		b.eval(`S.s2=` + strconv.Itoa(s) + `;render()`)
		if got := b.eval(`(() => { const h = document.querySelector('.n2head').getBoundingClientRect(), n = document.querySelector('.n2head .newplus').getBoundingClientRect(); return n.right <= h.right + 0.5 && n.left >= h.left - 0.5; })()`); got != "true" {
			t.Errorf("at step %d, New is cut off", s)
		}
	}
	if got := b.eval(`getComputedStyle(document.querySelector('.nav3 .cheadrow')).borderTopStyle`); got != "none" {
		t.Errorf("no line above the first collection of nav3: %q", got)
	}
}

func TestDividersInNav2(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.rightClickOn(`.nav2 [data-favid="ops"]`)
	b.clickOn(probe(b, `.ctxpop button`, "Line below it"), 1)
	if got := b.eval(`[...document.querySelectorAll('#clist > *')].map(n => n.dataset.deco || n.dataset.favid).slice(0, 8).join(' ')`); !strings.Contains(got, "ops line") {
		t.Fatalf("the line is below ops: %q", got)
	}
	// While dragging, a line shows where it lands.
	b.dragTo(`#clist .deco`, `#clist [data-favid="pro"]`, 0.95, func() {
		// Drawn, not only styled: shown, with content, two pixels high and in a colour.
		got := b.eval(`(() => { const n = document.querySelector('#clist .drop-after, #clist .drop-before'); if (!n) return 'none'; const l = getComputedStyle(n, '::before'); return [n.dataset.favid, l.display, l.content, l.height, l.backgroundColor].join(' '); })()`)
		if !strings.HasPrefix(got, "pro block \"\" 2px ") || strings.HasSuffix(got, "rgba(0, 0, 0, 0)") {
			t.Errorf("no line shows where it lands: %q", got)
		}
	})
	if got := b.eval(`[...document.querySelectorAll('#clist > *')].map(n => n.dataset.deco || n.dataset.favid).join(' ')`); !strings.Contains(got, "pro line") {
		t.Errorf("the line moved below pro: %q", got)
	}
	b.open(base + "/o/plan")
	b.ready("Network plan")
	if got := b.eval(`[...document.querySelectorAll('#clist > *')].map(n => n.dataset.deco || n.dataset.favid).join(' ')`); !strings.Contains(got, "pro line") {
		t.Errorf("the browser keeps the line: %q", got)
	}
	b.clickOn(`.nav2 [data-act="coll"][data-id="pro"]`, 2)
	b.shot("nav2-line-and-kept-open")
	b.rightClickOn(`#clist .deco`)
	b.clickOn(probe(b, `.ctxpop button`, "Remove"), 1)
	if got := b.eval(`document.querySelectorAll('#clist .deco').length`); got != "0" {
		t.Errorf("removed: %s", got)
	}
}

func TestDividersAndMovingInNav3(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	names := func() string {
		return b.eval(`[...document.querySelectorAll('.nav3 .cheadrow[data-cdrop="` + team + `"] + ul > li > .row')].map(n => n.dataset.deco || n.textContent.trim()).join(' ')`)
	}
	if got := names(); got != "A B C" {
		t.Fatalf("the pages of the team: %q", got)
	}
	b.rightClickOn(probe(b, `.nav3 .row`, "A"))
	b.clickOn(probe(b, `.ctxpop button`, "Line below it"), 1)
	if !b.waitFor(`document.querySelectorAll('.nav3 .row.deco').length === 1`) {
		t.Fatalf("no line: %q", names())
	}
	b.dragTo(rowNamed(b, "C"), rowNamed(b, "A"), 0.1, func() {
		if got := b.eval(`document.querySelector('.nav3 .drop-before')?.textContent.trim()`); got != "A" {
			t.Errorf("a line shows that C lands before A: %q", got)
		}
	})
	if got := names(); got != "C A line B" {
		t.Errorf("after the drop: %q", got)
	}
	b.open(base + "/o/" + team)
	b.ready("Team")
	if !b.waitFor(`document.querySelectorAll('.nav3 .row.deco').length === 1`) || names() != "C A line B" {
		t.Errorf("the database keeps the order and the line: %q", names())
	}
	b.shot("nav3-line")
	b.rightClickOn(`.nav3 .row.deco`)
	b.clickOn(probe(b, `.ctxpop button`, "Remove"), 1)
	if !b.waitFor(`document.querySelectorAll('.nav3 .row.deco').length === 0`) {
		t.Error("the line was not removed")
	}
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := names(); got != "C A B" {
		t.Errorf("removed for good: %q", got)
	}
}

// rowNamed is the selector of the entry of nav3 with this name, by its ID, so two of them can be used at once.
func rowNamed(b *browser, name string) string {
	b.t.Helper()
	id := b.eval(`[...document.querySelectorAll('.nav3 .row')].find(n => n.textContent.trim() === ` + strconv.Quote(name) + `)?.dataset.row`)
	return `.nav3 .row[data-row="` + id + `"]`
}
