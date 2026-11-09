//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"strconv"
	"strings"
	"testing"
	"time"
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

// dragTo holds the mouse on the middle of one element and moves it in steps to a point of another: fraction 0 is its top
// edge, 1 its bottom. Before letting go it runs check, which sees the page while the pointer is still held.
func (b *browser) dragTo(from, to string, fraction float64, check func()) {
	b.t.Helper()
	b.dragToX(from, to, fraction, 0, check)
}

// dragToX is dragTo that ends dx pixels right of the middle of the target.
func (b *browser) dragToX(from, to string, fraction, dx float64, check func()) {
	b.t.Helper()
	a := b.rect(from)
	b.mouse("mouseMoved", a.X, a.Y, 0, 0)
	b.mouse("mousePressed", a.X, a.Y, 1, 0)
	// Lift first, as a hand does, then aim: lifting several entries changes where the others are.
	b.mouse("mouseMoved", a.X, a.Y+8, 0, 0)
	time.Sleep(200 * time.Millisecond)
	z := b.rect(to)
	x, y := z.X+dx, z.Top+(z.Bottom-z.Top)*fraction
	for step := 1; step <= 8; step++ {
		b.mouse("mouseMoved", a.X+(x-a.X)*float64(step)/8, a.Y+(y-a.Y)*float64(step)/8, 0, 0)
	}
	time.Sleep(250 * time.Millisecond) // the entries slide for 140 ms
	if check != nil {
		check()
	}
	b.mouse("mouseReleased", x, y, 1, 0)
}

// gapAt tells where the gap of a drag is: right below which entry, as the text or the mark of that entry.
const gapAt = `(() => { const g = document.querySelector('.dropgap'); if (!g) return 'none';
	const r = g.getBoundingClientRect(), up = [...document.querySelectorAll('.nav2 .collrow, .nav3 .row, .nav3 .cheadrow')].filter(n => n.offsetParent && n.getBoundingClientRect().bottom <= r.top + 1 && r.top - n.getBoundingClientRect().bottom < 6).pop();
	return up ? (up.dataset.deco || up.dataset.favid || up.textContent.trim()) + ' ' + Math.round(r.height) : 'nothing above ' + Math.round(r.top); })()`

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
	if got := bar("ops"); !strings.HasSuffix(got, " 1px 0px 0px 0px inset") {
		t.Errorf("open for now: a thin line on the left, got %q", got)
	}
	b.clickOn(`.nav2 [data-act="coll"][data-id="ops"]`, 2)
	if !b.waitFor(`!!document.querySelector('.nav2 [data-id="ops"][data-pinned]')`) {
		t.Fatal("a double click keeps it open")
	}
	if got := bar("ops"); !strings.HasSuffix(got, " 4px 0px 0px 0px inset") {
		t.Errorf("kept open: a thick line on the left, got %q", got)
	}
	b.clickOn(`.nav2 [data-act="coll"][data-id="pro"]`, 1)
	b.shot("nav2-thin-and-thick")
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
	// While dragging, the line leaves its place and a gap of its height opens below pro, drawn with a dashed border.
	h := b.eval(`Math.round(document.querySelector('#clist .deco').getBoundingClientRect().height)`)
	b.dragTo(`#clist .deco`, `#clist [data-favid="pro"]`, 0.95, func() {
		b.shot("nav2-gap")
		if got := b.eval(gapAt); got != "pro "+h {
			t.Errorf("the gap is not below pro: %q", got)
		}
		if got := b.eval(`(() => { const s = getComputedStyle(document.querySelector('.dropgap')); return s.borderTopStyle + ' ' + s.borderTopWidth; })()`); got != "dashed 1px" {
			t.Errorf("the gap is not drawn: %q", got)
		}
		if got := b.eval(`[document.querySelectorAll('#clist .deco').length, document.querySelector('#clist .deco').offsetParent === null, document.getElementById('ghost')?.className].join(' ')`); got != "1 true ghost lift" {
			t.Errorf("the line is pulled out and the ghost carries it: %q", got)
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
		if got := b.eval(gapAt); !strings.HasPrefix(got, "TeamNew ") {
			t.Errorf("the gap shows that C lands before A, at the top of the team: %q", got)
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

// The others move apart live, a gap is the same wherever in it the dragged entry is, and moving it to the right puts it
// below the entry above it, for good.
func TestDragOpensAGapAndNests(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	a, c := rowNamed(b, "A"), rowNamed(b, "C")
	top := func(sel string) float64 { return b.rect(sel).Top }
	bTop := top(rowNamed(b, "B"))
	hC := b.eval(`Math.round(document.querySelector('` + strings.ReplaceAll(c, `"`, `\"`) + `').getBoundingClientRect().height)`)

	// Lifted, B leaves a gap where it was, between A and C, and nothing jumps; Escape puts it back.
	rb := b.rect(rowNamed(b, "B"))
	b.mouse("mouseMoved", rb.X, rb.Y, 0, 0)
	b.mouse("mousePressed", rb.X, rb.Y, 1, 0)
	b.mouse("mouseMoved", rb.X, rb.Y+8, 0, 0)
	time.Sleep(50 * time.Millisecond)
	if got := b.eval(gapAt); got != "A "+hC {
		t.Errorf("lifted, the gap stays where B was, below A: %q", got)
	}
	if got := b.eval(`document.getAnimations().length + ' ' + Math.round(document.querySelector(` + strconv.Quote(c) + `).getBoundingClientRect().top - ` + strconv.FormatFloat(top(c), 'f', 2, 64) + `)`); got != "0 0" {
		t.Errorf("lifting B makes nothing jump: animations and the move of C are %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
	b.mouse("mouseReleased", rb.X, rb.Y+8, 1, 0)
	if got := b.eval(`[document.querySelectorAll('.dropgap, #ghost').length, [...document.querySelectorAll('.nav3 .cheadrow[data-cdrop="` + team + `"] + ul > li > .row')].map(n => n.textContent.trim()).join(' ')].join(' ')`); got != "0 A B C" {
		t.Errorf("Escape puts B back: %q", got)
	}

	// By hand: C goes up between A and B. The lower half of A and the upper half of B give the one gap.
	r := b.rect(c)
	b.mouse("mouseMoved", r.X, r.Y, 0, 0)
	b.mouse("mousePressed", r.X, r.Y, 1, 0)
	to := func(sel string, fraction float64) {
		z := b.rect(sel)
		for step := 1; step <= 6; step++ {
			b.mouse("mouseMoved", r.X, r.Y+(z.Top+(z.Bottom-z.Top)*fraction-r.Y)*float64(step)/6, 0, 0)
		}
		r = box{X: r.X, Y: z.Top + (z.Bottom-z.Top)*fraction}
		time.Sleep(250 * time.Millisecond)
	}
	b.mouse("mouseMoved", r.X, r.Y+8, 0, 0)
	time.Sleep(50 * time.Millisecond)
	if got := b.eval(gapAt); got != "B "+hC {
		t.Errorf("lifted, the gap first stays where C was, below B: %q", got)
	}
	if got := b.eval(`(() => { const g = document.getElementById('ghost').getBoundingClientRect(); return g.height < ` + hC + ` * 1.5; })()`); got != "true" {
		t.Errorf("the ghost is not one entry high: %s", b.eval(`document.getElementById('ghost').getBoundingClientRect().height`))
	}
	to(a, 0.8)
	first := b.eval(gapAt)
	if first != "A "+hC {
		t.Errorf("the gap below A: %q", first)
	}
	if moved := top(rowNamed(b, "B")) - bTop; moved < 10 {
		t.Errorf("B did not move apart for the gap: %.1f", moved)
	}
	to(rowNamed(b, "B"), 0.2)
	if got := b.eval(gapAt); got != first {
		t.Errorf("the upper half of B gives another place than the lower half of A: %q, then %q", first, got)
	}
	b.mouse("mouseReleased", r.X, r.Y, 1, 0)
	if got := b.eval(`[...document.querySelectorAll('.nav3 .cheadrow[data-cdrop="` + team + `"] + ul > li > .row')].map(n => n.textContent.trim()).join(' ')`); got != "A C B" {
		t.Errorf("C lands between A and B: %q", got)
	}

	// Moved to the right by more than one step, C goes below A: the gap shows it indented.
	b.dragToX(c, a, 0.8, 30, func() {
		b.shot("nav3-gap-nested")
		if got := b.eval(`(() => { const g = document.getElementById('ghost').getBoundingClientRect(), n = document.querySelector(` + strconv.Quote(a) + `).getBoundingClientRect(); return Math.abs(g.width - n.width) < 2; })()`); got != "true" {
			t.Errorf("the ghost is not as wide as the entry it carries: %s", b.eval(`document.getElementById('ghost').getBoundingClientRect().width`))
		}
		if got := b.eval(`(() => { const g = document.querySelector('.dropgap'), n = document.querySelector('` + strings.ReplaceAll(a, `"`, `\"`) + `'); return g && g.getBoundingClientRect().left > n.getBoundingClientRect().left + 5; })()`); got != "true" {
			t.Errorf("the gap is not indented below A: %s", b.eval(gapAt))
		}
	})
	b.shot("nav3-nested")
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := b.eval(`[...document.querySelectorAll('.nav3 .cheadrow[data-cdrop="` + team + `"] + ul > li > .row')].map(n => n.textContent.trim()).join(' ')`); got != "A B" {
		t.Errorf("C left the top of the team: %q", got)
	}
	if !b.waitFor(`[...document.querySelectorAll('.nav3 .row')].some(n => n.textContent.trim() === 'C' && n.closest('ul:not(.top)'))`) {
		t.Errorf("the database keeps C below A: %s", b.eval(`document.querySelector('.nav3').innerText.replace(/\s+/g, ' ')`))
	}
}

// The private lists of nav2 sort the same way: the list leaves its place and lands in the gap.
func TestPrivateListsSortWithAGap(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/dev/ui")
	b.ready("")
	b.clickOn(`.strip [data-act="view"][data-id="todo"]`, 1)
	order := func() string {
		return b.eval(`[...document.querySelectorAll('.nav2 [data-secid]')].map(n => n.dataset.secid).join(' ')`)
	}
	if got := order(); got != "mine shop car" {
		t.Fatalf("the lists: %q", got)
	}
	b.dragTo(`.nav2 [data-secid="car"]`, `.nav2 [data-secid="mine"]`, 0.2, func() {
		if got := b.eval(`(() => { const g = document.querySelector('.nav2 .dropgap'), m = document.querySelector('.nav2 [data-secid="mine"]'); return !!g && g.getBoundingClientRect().bottom <= m.getBoundingClientRect().top + 1; })()`); got != "true" {
			t.Errorf("the gap is not above the first list")
		}
	})
	if got := order(); got != "car mine shop" {
		t.Errorf("after the drop: %q", got)
	}
	// Ctrl and click selects here too, and keeps nothing open.
	r := b.rect(`.nav2 [data-secid="shop"]`)
	b.mouse("mouseMoved", r.X, r.Y, 0, 0)
	b.mouse("mousePressed", r.X, r.Y, 1, 2)
	b.mouse("mouseReleased", r.X, r.Y, 1, 2)
	if got := b.eval(`[document.querySelector('.nav2 [data-secid="shop"]').getAttribute('aria-selected'), document.querySelectorAll('.nav2 [data-pinned]').length].join(' ')`); got != "true 0" {
		t.Errorf("Ctrl and click selects a list: %q", got)
	}
}

// Nothing lands where one may only read: no gap opens there, and a drop leaves everything where it was.
func TestNoGapWhereOneMayOnlyRead(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/dev/ui")
	b.ready("")
	b.eval(`S.view = 'workspace'; S.open = {ops: true, arc: true}; render()`)
	src := `.nav3 .row[data-row="hand"]`
	if b.eval(`!!document.querySelector('`+src+`') && !!document.querySelector('.nav3 .row[data-row="old"]')`) != "true" {
		t.Fatalf("the demo changed: %s", b.eval(`document.querySelector('.nav3').innerText.replace(/\s+/g, ' ')`))
	}
	b.dragTo(`.nav3 .row[data-row="net"]`, `.nav3 .row[data-row="old"]`, 0.5, func() {
		if got := b.eval(gapAt); got != "none" {
			t.Errorf("a gap opens in the archive, which may only be read: %q", got)
		}
	})
	// Below it, beside it, the same.
	b.dragTo(`.nav3 .row[data-row="net"]`, `.nav3 .row[data-row="old"]`, 0.9, func() {
		if got := b.eval(gapAt); got != "none" {
			t.Errorf("a gap opens beside an entry of the archive: %q", got)
		}
	})
	if got := b.eval(`D.arc.kids.join(' ')`); got != "old" {
		t.Errorf("the archive changed: %q", got)
	}
}

// The middle third of an entry puts the dragged one below it; level with the entry above it lands beside it, and further
// left it goes out towards the top.
func TestDropOntoAndAcross(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	kids := func(name string) string {
		return b.eval(`[...(document.querySelector(` + strconv.Quote(rowNamed(b, name)) + `)?.closest('li').querySelectorAll(':scope > ul > li > .row') || [])].map(n => n.textContent.trim()).join(' ')`)
	}
	tops := func() string {
		return b.eval(`[...document.querySelectorAll('.nav3 .cheadrow[data-cdrop="` + team + `"] + ul > li > .row')].map(n => n.textContent.trim()).join(' ')`)
	}
	b.dragTo(rowNamed(b, "C"), rowNamed(b, "A"), 0.5, func() {
		b.shot("drop-onto")
		if got := b.eval(`[document.querySelector('.dropgap')?.classList.contains('idle'), getComputedStyle(document.getElementById('ghost')).opacity < 0.5].join(' ')`); got != "true true" {
			t.Errorf("onto A, the gap waits unseen and the ghost lets A show through: %q", got)
		}
		if got := b.eval(`[...document.querySelectorAll('.nav3 .drop-into')].map(n => n.textContent.trim()).join()`); got != "A" {
			t.Errorf("the middle of A frames A: %q", got)
		}
	})
	if got := kids("A"); got != "C" {
		t.Fatalf("C is below A: %q", got)
	}
	b.dragTo(rowNamed(b, "B"), rowNamed(b, "C"), 0.8, func() {
		if got := b.eval(`document.querySelectorAll('.nav3 .drop-into').length`); got != "0" {
			t.Errorf("the lower part of C is beside it, not onto it")
		}
	})
	if got := kids("A"); got != "C B" {
		t.Fatalf("level with C, B lands beside it: %q", got)
	}
	b.dragToX(rowNamed(b, "B"), rowNamed(b, "C"), 0.8, -40, nil)
	if got := tops() + " | " + kids("A"); got != "A B | C" {
		t.Fatalf("further left, B goes out to the top: %q", got)
	}
	time.Sleep(500 * time.Millisecond)
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := tops() + " | " + kids("A"); got != "A B | C" {
		t.Errorf("the database keeps it: %q", got)
	}
}

// Ctrl and click selects, Shift and click a range; dragging one moves all of them, and on a window they open as tabs.
func TestSelectSeveralAndMove(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	ctrl := func(sel string, mods int) {
		r := b.rect(sel)
		b.mouse("mouseMoved", r.X, r.Y, 0, 0)
		b.mouse("mousePressed", r.X, r.Y, 1, mods)
		b.mouse("mouseReleased", r.X, r.Y, 1, mods)
	}
	selected := func() string {
		return b.eval(`[...document.querySelectorAll('[aria-selected="true"]')].map(n => n.textContent.trim() || n.dataset.deco).join(' ')`)
	}
	tops := func() string {
		return b.eval(`[...document.querySelectorAll('.nav3 .cheadrow[data-cdrop="` + team + `"] + ul > li > .row')].map(n => n.textContent.trim()).join(' ')`)
	}
	title := b.eval(`document.querySelector('.dtitle h1').textContent`)
	ctrl(rowNamed(b, "A"), 2)
	ctrl(rowNamed(b, "C"), 2)
	if got := selected(); got != "A C" {
		t.Fatalf("Ctrl and click selects: %q", got)
	}
	b.shot("selected")
	if got := b.eval(`(() => { const s = getComputedStyle(document.querySelector('.nav3 [aria-selected="true"]')); return s.outlineStyle + ' ' + s.outlineWidth; })()`); got != "solid 1px" {
		t.Errorf("a selected entry is framed: %q", got)
	}
	if got := b.eval(`document.querySelector('.dtitle h1').textContent`); got != title {
		t.Errorf("selecting opens nothing: %q", got)
	}
	b.dragTo(rowNamed(b, "A"), rowNamed(b, "B"), 0.8, func() {
		b.shot("select-drag")
		if got := b.eval(`document.querySelector('#ghost .count')?.textContent`); got != "2" {
			t.Errorf("the ghost says two: %q", got)
		}
	})
	if got := tops(); got != "B A C" {
		t.Fatalf("both move below B: %q", got)
	}
	time.Sleep(500 * time.Millisecond)
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := tops(); got != "B A C" {
		t.Errorf("the database keeps the order: %q", got)
	}
	ctrl(rowNamed(b, "B"), 2)
	ctrl(rowNamed(b, "C"), 8) // Shift
	if got := selected(); got != "B A C" {
		t.Fatalf("Shift and click takes the range: %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
	if got := selected(); got != "" {
		t.Errorf("Escape ends the selection: %q", got)
	}
	ctrl(rowNamed(b, "A"), 2)
	ctrl(rowNamed(b, "C"), 2)
	b.dragTo(rowNamed(b, "A"), `[data-win="0"] .wbody`, 0.5, func() {
		if got := b.eval(`!!document.querySelector('[data-win="0"].zone-open') && !document.querySelector('.dropgap')`); got != "true" {
			t.Error("over the window it says it opens there")
		}
	})
	if got := b.eval(`[...document.querySelectorAll('[data-win="0"] .dtab')].map(n => n.textContent.trim()).filter(n => n === 'A' || n === 'C').join(' ')`); got != "A C" {
		t.Errorf("dropped on the window, both open as tabs: %s", b.eval(`document.querySelector('[data-win="0"] .tabrow').innerText.replace(/\s+/g, ' ')`))
	}
	if got := tops(); got != "B A C" {
		t.Errorf("opening moves nothing: %q", got)
	}

	// In nav2, Ctrl and click selects and no longer keeps a collection open; the right click keeps the selection open.
	ctrl(`.nav2 [data-act="coll"][data-id="ops"]`, 2)
	ctrl(probe(b, `.nav2 [data-act="coll"]`, "Home"), 2)
	if got := b.eval(`[...document.querySelectorAll('#clist [aria-selected="true"]')].map(n => n.textContent.trim()).join(' ') + ' ' + document.querySelectorAll('.nav2 [data-pinned]').length`); got != "Home Operations (demo) 0" {
		t.Fatalf("nav2 selects with Ctrl and click, and pins nothing: %q", got)
	}
	b.rightClickOn(`#clist [aria-selected="true"]`)
	b.clickOn(probe(b, `.ctxpop button`, "Keep them open"), 1)
	if got := b.eval(`document.querySelectorAll('.nav2 [data-pinned]').length`); got != "2" {
		t.Errorf("the right click keeps both open: %s", got)
	}
}
