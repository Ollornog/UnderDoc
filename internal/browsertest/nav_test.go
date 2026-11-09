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
	time.Sleep(150 * time.Millisecond) // the entries slide for 140 ms
	if check != nil {
		check()
	}
	b.mouse("mouseReleased", x, y, 1, 0)
}

// gapAt tells where the gap of a drag is: right below which entry, as the text or the mark of that entry.
const gapAt = `(() => { const g = document.querySelector('.dropgap'); if (!g) return 'none';
	const r = g.getBoundingClientRect(), up = [...document.querySelectorAll('#clist > :not(.dropgap), .nav3 .row, .nav3 .cheadrow')].filter(n => !n.closest('.dropgap')).filter(n => n.offsetParent && n.getBoundingClientRect().bottom <= r.top + 1 && r.top - n.getBoundingClientRect().bottom < 6).pop();
	return up ? (up.dataset.deco || up.dataset.favid || up.querySelector(':scope > .collrow')?.dataset.favid || up.textContent.trim()) + ' ' + Math.round(r.height) : 'nothing above ' + Math.round(r.top); })()`

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
	if got := bar("ops"); !strings.HasSuffix(got, " 3px 0px 0px 0px inset") {
		t.Errorf("kept open: a thick line on the left, got %q", got)
	}
	b.clickOn(`.nav2 [data-act="coll"][data-id="pro"]`, 1)
	b.shot("nav2-thin-and-thick")
	if got := b.eval(`getComputedStyle(document.querySelector('.nav3 .cheadrow .chead')).fontSize`); got != "13.5px" {
		t.Errorf("the name of a collection in nav3 reads as a heading: %q", got)
	}
	// Each open collection begins with a band of its own, with space above the second.
	if got := b.eval(`(() => { const h = [...document.querySelectorAll('.nav3 .cheadrow')]; if (h.length < 2) return 'only ' + h.length; const s = getComputedStyle(h[1]); return [s.backgroundColor !== 'rgba(0, 0, 0, 0)', parseFloat(s.marginTop) >= 16].join(' '); })()`); got != "true true" {
		t.Errorf("collections in nav3 are not set apart: %q", got)
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
	if got := b.eval(`[...document.querySelectorAll('#clist > *')].map(n => n.dataset.deco || n.dataset.favid || n.querySelector(':scope > .collrow')?.dataset.favid).slice(0, 8).join(' ')`); !strings.Contains(got, "ops line") {
		t.Fatalf("the line is below ops: %q", got)
	}
	// While dragging, the line leaves its place and a gap of its height opens below pro, drawn with a dashed border.
	h := b.eval(`Math.round(document.querySelector('#clist .deco').getBoundingClientRect().height)`)
	b.dragTo(`#clist .deco`, `#clist [data-favid="pro"]`, 0.95, func() {
		b.shot("nav2-gap")
		if got := b.eval(gapAt); got != "pro "+h {
			t.Errorf("the gap is not below pro: %q", got)
		}
		if got := b.eval(`(() => { const s = getComputedStyle(document.querySelector('.dropgap')); return s.outlineStyle + ' ' + s.outlineWidth; })()`); got != "dashed 1px" {
			t.Errorf("the place is not drawn: %q", got)
		}
		if got := b.eval(`[document.querySelector('#clist .deco[data-favid]').offsetParent === null, document.querySelectorAll('.dropgap .deco').length, !!document.getElementById('ghost')].join(' ')`); got != "true 1 false" {
			t.Errorf("the line itself moves through the list, no picture floats: %q", got)
		}
	})
	if got := b.eval(`[...document.querySelectorAll('#clist > *')].map(n => n.dataset.deco || n.dataset.favid || n.querySelector(':scope > .collrow')?.dataset.favid).join(' ')`); !strings.Contains(got, "pro line") {
		t.Errorf("the line moved below pro: %q", got)
	}
	b.open(base + "/o/plan")
	b.ready("Network plan")
	if got := b.eval(`[...document.querySelectorAll('#clist > *')].map(n => n.dataset.deco || n.dataset.favid || n.querySelector(':scope > .collrow')?.dataset.favid).join(' ')`); !strings.Contains(got, "pro line") {
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
	// Carried up before A, then Escape: B goes back where it was.
	ra := b.rect(a)
	b.mouse("mouseMoved", rb.X, ra.Top+(ra.Bottom-ra.Top)*0.1, 0, 0)
	time.Sleep(150 * time.Millisecond)
	if got := b.eval(gapAt); !strings.HasPrefix(got, "TeamNew ") {
		t.Errorf("B is held before A: %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
	b.mouse("mouseReleased", rb.X, ra.Top+(ra.Bottom-ra.Top)*0.1, 1, 0)
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
	if got := b.eval(`[...document.querySelectorAll('.dropgap .row')].map(n => n.textContent.trim()).join() + ' ' + !!document.getElementById('ghost')`); got != "C false" {
		t.Errorf("C is held in the list as it looks, nothing floats: %q", got)
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
	// Onto it, after waiting, and below it: the entry stays faint where it was, and nothing is framed.
	b.dragTo(`.nav3 .row[data-row="net"]`, `.nav3 .row[data-row="old"]`, 0.5, func() {
		time.Sleep(600 * time.Millisecond)
		if got := b.eval(`!!document.querySelector('.dropgap.idle') && !document.querySelector('.drop-into') && !document.querySelector('[data-row="old"]').closest('ul').querySelector('.dropgap')`); got != "true" {
			t.Errorf("onto an entry of the archive, which may only be read: %s", b.eval(gapAt))
		}
	})
	b.dragTo(`.nav3 .row[data-row="net"]`, `.nav3 .row[data-row="old"]`, 0.9, func() {
		if got := b.eval(`!!document.querySelector('.dropgap.idle') && !document.querySelector('[data-row="old"]').closest('ul').querySelector('.dropgap')`); got != "true" {
			t.Errorf("beside an entry of the archive: %s", b.eval(gapAt))
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
		// Up and down goes between entries; only waiting on the middle of A goes into it.
		if got := b.eval(`document.querySelectorAll('.nav3 .drop-into').length`); got != "0" {
			t.Errorf("at once, the middle of A is not yet onto it")
		}
		// Nothing answers the pointer while it drags, so nothing lights up under it.
		if got := b.eval(`getComputedStyle(document.querySelector(` + strconv.Quote(rowNamed(b, "A")) + `)).pointerEvents`); got != "none" {
			t.Errorf("an entry answers the pointer while dragging: %q", got)
		}
		time.Sleep(350 * time.Millisecond)
		b.shot("drop-onto")
		if got := b.eval(`[...document.querySelectorAll('.nav3 .drop-into')].map(n => n.textContent.trim()).join() + ' ' + getComputedStyle(document.querySelector('.dropgap')).opacity`); got != "A 0.4" {
			t.Errorf("after waiting, A is framed and C waits faint: %q", got)
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
		if got := b.eval(`[...document.querySelectorAll('.dropgap .row')].map(n => n.textContent.trim()).join(' ')`); got != "A C" {
			t.Errorf("both are held together: %q", got)
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
		if got := b.eval(`!!document.querySelector('[data-win="0"].zone-open') && !!document.querySelector('.dropgap.idle')`); got != "true" {
			t.Error("over the window it says it opens there")
		}
	})
	if got := b.eval(`[...document.querySelectorAll('[data-win="0"] .dtab')].map(n => n.textContent.trim()).filter(n => n === 'A' || n === 'C').join(' ')`); got != "A C" {
		t.Errorf("dropped on the window, both open as tabs: %s", b.eval(`document.querySelector('[data-win="0"] .tabrow').innerText.replace(/\s+/g, ' ')`))
	}
	if got := tops(); got != "B A C" {
		t.Errorf("opening moves nothing: %q", got)
	}
	// A tab still drags as a label; it takes the sizes of the workspace although it hangs outside it.
	b.dragTo(probe(b, `[data-win="0"] .dtab`, "C"), `[data-win="0"] .wbody`, 0.5, func() {
		if got := b.eval(`getComputedStyle(document.getElementById('ghost')).paddingLeft`); got == "0px" || got == "" {
			t.Errorf("the label of a dragged tab has no padding: %q", got)
		}
	})

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

// A plain click is where a range starts: Shift and click right after it takes the range, Ctrl and click adds to it.
func TestShiftAfterAPlainClick(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	click := func(sel string, mods int) {
		r := b.rect(sel)
		b.mouse("mouseMoved", r.X, r.Y, 0, 0)
		b.mouse("mousePressed", r.X, r.Y, 1, mods)
		b.mouse("mouseReleased", r.X, r.Y, 1, mods)
	}
	selected := func() string {
		return b.eval(`[...document.querySelectorAll('.nav3 [aria-selected="true"]')].map(n => n.textContent.trim()).join(' ')`)
	}
	click(rowNamed(b, "A"), 0)
	if got := selected(); got != "" {
		t.Errorf("a plain click selects nothing to see: %q", got)
	}
	click(rowNamed(b, "C"), 8)
	if got := selected(); got != "A B C" {
		t.Errorf("Shift right after a plain click takes the range: %q", got)
	}
	click(rowNamed(b, "B"), 0)
	click(rowNamed(b, "C"), 2)
	if got := selected(); got != "B C" {
		t.Errorf("Ctrl right after a plain click adds to it: %q", got)
	}
	// In nav2 too, where no open document stands in for the click.
	click(probe(b, `.nav2 [data-act="coll"]`, "My notes"), 0)
	click(probe(b, `.nav2 [data-act="coll"]`, "Home"), 8)
	if got := b.eval(`[...document.querySelectorAll('#clist [aria-selected="true"]')].map(n => n.textContent.trim()).join(', ')`); !strings.HasPrefix(got, "My notes, ") || !strings.HasSuffix(got, ", Home") {
		t.Errorf("Shift after a plain click in nav2: %q", got)
	}
}

// An entry dropped on a collection of nav2 goes into it, at once, wherever on it.
func TestIntoACollectionOfNav2(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.eval(`window.other = null; fetch('/api/collections', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: '{"title":"Other"}'}).then(r => r.json()).then(j => { window.other = j.id; })`)
	if !b.waitFor(`!!window.other`) {
		t.Fatal("no second collection")
	}
	other := b.eval(`window.other`)
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	target := probe(b, `.nav2 [data-act="coll"]`, "Other")
	b.dragTo(rowNamed(b, "A"), target, 0.2, func() {
		if got := b.eval(`(() => { const n = document.querySelector('#clist .drop-into'); return n ? n.textContent.trim() + ' ' + (getComputedStyle(n).boxShadow !== 'none') : 'none'; })()`); got != "Other true" {
			t.Errorf("Other is framed at once: %q", got)
		}
	})
	time.Sleep(500 * time.Millisecond)
	if got := b.eval(`[...document.querySelectorAll('.nav3 .cheadrow[data-cdrop="` + team + `"] + ul > li > .row')].map(n => n.textContent.trim()).join(' ')`); got != "B C" {
		t.Errorf("A left the team: %q", got)
	}
	b.eval(`window.kids = null; fetch('/api/state').then(r => r.json()).then(j => { window.kids = j.collections.find(c => c.id === '` + other + `').kids.map(k => k.n).join(' '); })`)
	if !b.waitFor(`window.kids !== null`) || b.eval(`window.kids`) != "A" {
		t.Errorf("the database has A in Other: %q", b.eval(`window.kids`))
	}
}

// Ctrl at the drop copies; the right click duplicates an entry and sorts the children of a parent; every entry of a
// menu has its icon.
func TestCopyDuplicateAndSort(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	tops := func() string {
		return b.eval(`[...document.querySelectorAll('.nav3 .cheadrow[data-cdrop="` + team + `"] + ul > li > .row')].map(n => n.dataset.deco || n.textContent.trim()).join(', ')`)
	}
	from, to := b.rect(rowNamed(b, "C")), b.rect(rowNamed(b, "A"))
	b.mouse("mouseMoved", from.X, from.Y, 0, 0)
	b.mouse("mousePressed", from.X, from.Y, 1, 0)
	y := to.Top + (to.Bottom-to.Top)*0.85
	for step := 1; step <= 8; step++ {
		b.mouse("mouseMoved", from.X, from.Y+(y-from.Y)*float64(step)/8, 0, 2)
	}
	time.Sleep(250 * time.Millisecond)
	if got := b.eval(`!!document.querySelector('.dropgap.copy')`); got != "true" {
		t.Error("with Ctrl held, the held entry says it is copied")
	}
	b.mouse("mouseReleased", from.X, y, 1, 2)
	if !b.waitFor(`[...document.querySelectorAll('.nav3 .row')].some(n => n.textContent.trim() === 'C (copy)')`) {
		t.Fatalf("Ctrl at the drop copies: %q", tops())
	}
	if got := tops(); got != "A, C (copy), B, C" {
		t.Errorf("the copy lands where it was dropped, the original stays: %q", got)
	}
	b.rightClickOn(rowNamed(b, "B"))
	if got := b.eval(`[...document.querySelectorAll('.ctxpop button')].every(n => n.querySelector('svg'))`); got != "true" {
		t.Errorf("an entry of the menu has no icon: %s", b.eval(`[...document.querySelectorAll('.ctxpop button')].filter(n => !n.querySelector('svg')).map(n => n.textContent).join()`))
	}
	b.clickOn(probe(b, `.ctxpop button`, "Duplicate"), 1)
	if !b.waitFor(`[...document.querySelectorAll('.nav3 .row')].some(n => n.textContent.trim() === 'B (copy)')`) {
		t.Fatalf("Duplicate: %q", tops())
	}
	b.rightClickOn(`.nav3 .cheadrow[data-cdrop="` + team + `"] .chead`)
	b.clickOn(probe(b, `.ctxpop button`, "Sort its children Z to A"), 1)
	if got := tops(); got != "C (copy), C, B (copy), B, A" {
		t.Errorf("sorted Z to A: %q", got)
	}
	// A line stays where it is: each group around it is sorted on its own.
	b.rightClickOn(rowNamed(b, "B (copy)"))
	b.clickOn(probe(b, `.ctxpop button`, "Line below it"), 1)
	if !b.waitFor(`document.querySelectorAll('.nav3 .row.deco').length === 1`) {
		t.Fatal("no line")
	}
	b.rightClickOn(`.nav3 .cheadrow[data-cdrop="` + team + `"] .chead`)
	b.clickOn(probe(b, `.ctxpop button`, "Sort its children A to Z"), 1)
	want := "B (copy), C, C (copy), line, A, B"
	if got := tops(); got != want {
		t.Errorf("sorted A to Z around the line: %q", got)
	}
	time.Sleep(700 * time.Millisecond)
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := tops(); got != want {
		t.Errorf("the database keeps the copies and the order: %q", got)
	}
}

// While a single window holds only the general tab, neither tabs nor the window are drawn; a button beside New holds
// what the tab offers, and a second tab brings the tabs back.
func TestOneWindowShowsNoTabs(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	if got := b.eval(`[document.querySelectorAll('.tabrow').length, document.querySelectorAll('.brandrow .winbtn').length].join(' ')`); got != "0 1" {
		t.Fatalf("no tabs, a window button: %q", got)
	}
	b.clickOn(`.brandrow .winbtn`, 1)
	if got := b.eval(`[...document.querySelectorAll('.ctxpop button')].every(n => n.querySelector('svg')) && !!document.querySelector('.ctxpop')`); got != "true" {
		t.Errorf("the window menu, with icons: %s", b.eval(`document.querySelector('.ctxpop')?.innerText`))
	}
	b.shot("window-menu")
	b.clickOn(probe(b, `.ctxpop button`, "Keep as a tab of its own"), 1)
	if got := b.eval(`[document.querySelectorAll('.tabrow').length, document.querySelectorAll('.brandrow .winbtn').length].join(' ')`); got != "1 0" {
		t.Errorf("a tab of its own brings the tabs back: %q", got)
	}
}

// A folder is made in a collection and named in place; a collection and a folder turn into each other. A collection inside
// another shows in nav2 only when asked to, as an entry that points at it.
func TestFoldersAndShowingInNav2(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.eval(`window.done = false; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		let parent = '` + team + `';
		for (const title of ['L1', 'L2']) { parent = (await post('/api/collections', {title, parent})).id; window[title] = parent; }
		await post('/api/pages', {title: 'Deep page', parent});
		window.done = true;
	})()`)
	if !b.waitFor(`window.done`) {
		t.Fatal("the tree for the test was not made")
	}
	l2 := b.eval(`window.L2`)
	b.open(base + "/o/" + team)
	b.ready("Team")
	inNav2 := func() string {
		return b.eval(`[...document.querySelectorAll('#clist > .collrow:not(.deco)')].map(n => n.textContent.trim() + (n.dataset.alias ? '*' : '')).join(', ')`)
	}
	if got := inNav2(); strings.Contains(got, "L1") || strings.Contains(got, "L2") {
		t.Errorf("collections inside others stay out of nav2 by themselves: %q", got)
	}
	if got := b.eval(`document.querySelectorAll('#clist [data-favid="` + team + `"] svg rect').length`); got != "4" {
		t.Errorf("a collection of the database has the icon of a collection, not of a folder: %s rects", got)
	}
	b.rightClickOn(rowNamed(b, "L2"))
	b.clickOn(probe(b, `.ctxpop button`, "Show in nav2"), 1)
	if !b.waitFor(`!!document.querySelector('#clist [data-alias][data-favid="` + l2 + `"]')`) {
		t.Fatalf("L2 is shown in nav2: %q", inNav2())
	}
	if got := b.eval(`getComputedStyle(document.querySelector('#clist [data-alias] .lbl')).fontStyle`); got != "italic" {
		t.Errorf("the entry only points at L2: %q", got)
	}
	b.shot("nav2-shown")
	b.clickOn(`#clist [data-alias] [data-act="coll"]`, 1)
	if !b.waitFor(`[...document.querySelectorAll('.nav3 .cheadrow .chead')].map(n => n.textContent.trim()).join() === 'L2'`) {
		t.Errorf("it opens in nav3 on its own: %s", b.eval(`document.querySelector('.nav3').innerText.replace(/\s+/g, ' ')`))
	}
	if !b.waitFor(`[...document.querySelectorAll('.nav3 .row')].some(n => n.textContent.trim() === 'Deep page')`) {
		t.Error("with its own entries")
	}
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := b.eval(`!!document.querySelector('#clist [data-alias][data-favid="` + l2 + `"]')`); got != "true" {
		t.Errorf("the browser keeps it in nav2: %q", inNav2())
	}
	if got := b.eval(`(() => { const deep = Object.keys(D).find(k => D[k].n === 'Deep page'); return PARENT[deep] === SID['` + l2 + `'] && COLL[SID['` + l2 + `']]?.id === SID['` + team + `']; })()`); got != "true" {
		t.Error("the entry in nav2 changed where the pages of L2 lie")
	}
	// It still lies in L1: the tree of Team holds it as before.
	if got := b.eval(`(() => { const n = [...document.querySelectorAll('.nav3 .row')].find(n => n.textContent.trim() === 'L2'); return !!n && !!n.closest('ul:not(.top)'); })()`); got != "true" {
		t.Errorf("L2 left L1: %s", b.eval(`document.querySelector('.nav3').innerText.replace(/\s+/g, ' ')`))
	}
	b.rightClickOn(`#clist [data-alias]`)
	b.clickOn(probe(b, `.ctxpop button`, "Remove from nav2"), 1)
	if got := b.eval(`document.querySelectorAll('#clist [data-alias]').length`); got != "0" {
		t.Errorf("removed from nav2: %q", inNav2())
	}
	b.clickOn(`.cheadrow[data-cdrop="`+team+`"] .newplus`, 1)
	b.typeText("folder")
	b.press("Enter", "Enter", 13, 0)
	if !b.waitFor(`document.activeElement?.dataset?.ren !== undefined && document.activeElement.value === 'New folder (1)'`) {
		t.Fatalf("the folder is named in place: %q", b.eval(`document.activeElement?.outerHTML.slice(0, 160)`))
	}
	b.typeText("Drawings")
	b.press("Enter", "Enter", 13, 0)
	kind := func(title string) string {
		b.eval(`window.kind = null; fetch('/api/state').then(r => r.json()).then(j => { const walk = l => l.forEach(n => { if (n.n === ` + strconv.Quote(title) + `) window.kind = n.t; walk(n.kids || []); }); walk(j.collections); })`)
		b.waitFor(`window.kind !== null`)
		return b.eval(`window.kind`)
	}
	time.Sleep(300 * time.Millisecond)
	if got := kind("Drawings"); got != "folder" {
		t.Errorf("the database has a folder: %q", got)
	}
	b.rightClickOn(rowNamed(b, "Drawings"))
	if got := b.eval(`[...document.querySelectorAll('.ctxpop button')].some(n => n.textContent.includes('Show in nav2'))`); got != "false" {
		t.Error("a folder offers to show in nav2")
	}
	b.clickOn(probe(b, `.ctxpop button`, "Turn into a collection"), 1)
	time.Sleep(300 * time.Millisecond)
	if got := kind("Drawings"); got != "collection" {
		t.Errorf("the database has a collection: %q", got)
	}
	b.rightClickOn(rowNamed(b, "Drawings"))
	b.clickOn(probe(b, `.ctxpop button`, "Show in nav2"), 1)
	if !b.waitFor(`[...document.querySelectorAll('#clist [data-alias]')].some(n => n.textContent.trim() === 'Drawings')`) {
		t.Fatal("Drawings is not shown in nav2")
	}
	b.rightClickOn(rowNamed(b, "Drawings"))
	b.clickOn(probe(b, `.ctxpop button`, "Turn into a folder"), 1)
	time.Sleep(300 * time.Millisecond)
	if got := kind("Drawings"); got != "folder" {
		t.Errorf("and back into a folder: %q", got)
	}
	if got := b.eval(`[...document.querySelectorAll('#clist [data-alias]')].some(n => n.textContent.trim() === 'Drawings')`); got != "false" {
		t.Error("a folder stays shown in nav2")
	}
	b.rightClickOn(`.nav2 [data-act="coll"][data-id="` + team + `"]`)
	if got := b.eval(`[...document.querySelectorAll('.ctxpop button')].some(n => n.textContent.includes('Turn into') || n.textContent.includes('Show in nav2'))`); got != "false" {
		t.Error("a collection at the top offers to become a folder or to be shown in nav2")
	}
}

// A right click menu stays inside the window, wherever the click is.
func TestMenusStayInTheWindow(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	inside := `(() => { const r = document.querySelector('.ctxpop').getBoundingClientRect(); return r.top >= 0 && r.left >= 0 && r.bottom <= innerHeight && r.right <= innerWidth; })()`
	h := b.eval(`innerHeight`)
	hf, _ := strconv.ParseFloat(h, 64)
	n2 := b.rect(`.nav2`)
	b.call("Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": n2.X, "y": hf - 6}, nil)
	for _, kind := range []string{"mousePressed", "mouseReleased"} {
		b.call("Input.dispatchMouseEvent", map[string]any{"type": kind, "x": n2.X, "y": hf - 6, "button": "right", "buttons": 2, "clickCount": 1}, nil)
	}
	if !b.waitFor(`!!document.querySelector('.ctxpop')`) {
		t.Fatal("no menu on the free part of nav2")
	}
	if got := b.eval(inside); got != "true" {
		t.Errorf("a menu opened at the bottom leaves the window: %s", b.eval(`JSON.stringify(document.querySelector('.ctxpop').getBoundingClientRect())`))
	}
	// A long one, in the corner: inside too, and it scrolls.
	b.eval(`S.ctx = {kind: 'win', x: innerWidth - 10, y: innerHeight - 10}; render()`)
	if got := b.eval(inside); got != "true" {
		t.Errorf("a menu in the corner leaves the window: %s", b.eval(`JSON.stringify(document.querySelector('.ctxpop').getBoundingClientRect())`))
	}
	if got := b.eval(`getComputedStyle(document.querySelector('.ctxpop')).overflowY`); got != "auto" {
		t.Errorf("a long menu scrolls: %q", got)
	}
	// New near the bottom opens above its place, inside the window.
	b.eval(`S.ctx = null; S.addIn = 'n2'; S.addQ = ''; S.addAt = {x: 40, y: innerHeight - 20, up: innerHeight - 40}; render()`)
	if got := b.eval(`(() => { const r = document.querySelector('.cmdpop').getBoundingClientRect(); return r.bottom <= innerHeight && r.top >= 0; })()`); got != "true" {
		t.Errorf("New leaves the window: %s", b.eval(`JSON.stringify(document.querySelector('.cmdpop').getBoundingClientRect())`))
	}
	// The menu of a tab at the right edge moves in.
	b.eval(`S.addIn = null; const w = S.wins[0]; w.tabs.push(newTab(w.tabs[0].d)); S.tmenu = {w: 0, t: 1, x: innerWidth}; render()`)
	if got := b.eval(`(() => { const r = document.querySelector('.tabctx').getBoundingClientRect(); return r.right <= innerWidth; })()`); got != "true" {
		t.Errorf("the menu of a tab leaves the window: %s", b.eval(`JSON.stringify(document.querySelector('.tabctx').getBoundingClientRect())`))
	}
}

// An entry dragged into an empty collection lands in it: below its heading, onto its heading, and onto an empty
// collection inside another.
func TestDragIntoAnEmptyCollection(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.eval(`window.ids = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		const empty = (await post('/api/collections', {title: 'Empty'})).id;
		const team = (await post('/api/collections', {title: 'Team'})).id;
		for (const title of ['A', 'B', 'C', 'D']) await post('/api/pages', {title, parent: team});
		const inner = (await post('/api/collections', {title: 'Inner', parent: team})).id;
		window.ids = [empty, team, inner];
	})()`)
	if !b.waitFor(`!!window.ids`) {
		t.Fatal("the collections for the test were not made")
	}
	var ids []string
	for _, k := range []string{"0", "1", "2"} {
		ids = append(ids, b.eval(`window.ids[`+k+`]`))
	}
	empty, team, inner := ids[0], ids[1], ids[2]
	b.open(base + "/o/" + team)
	b.ready("Team")
	b.clickOn(`.nav2 [data-act="coll"][data-id="`+team+`"]`, 2)
	b.clickOn(`.nav2 [data-act="coll"][data-id="`+empty+`"]`, 1)
	if !b.waitFor(`!!document.querySelector('.cheadrow[data-cdrop="` + empty + `"]') && !!document.querySelector('.cheadrow[data-cdrop="` + team + `"]')`) {
		t.Fatalf("both are open: %s", b.eval(`document.querySelector('.nav3').innerText.replace(/\s+/g, ' ')`))
	}
	kids := func(of string) string {
		return b.eval(`D[SID['` + of + `']].kids.map(k => D[k].n).join(' ')`)
	}
	b.dragTo(rowNamed(b, "A"), `.cheadrow[data-cdrop="`+empty+`"]`, 0.9, nil)
	if got := kids(empty); got != "A" {
		t.Errorf("below the heading of the empty collection: %q", got)
	}
	b.dragTo(rowNamed(b, "B"), `.cheadrow[data-cdrop="`+empty+`"]`, 0.5, func() { time.Sleep(400 * time.Millisecond) })
	if got := kids(empty); got != "A B" {
		t.Errorf("onto the heading, after waiting: %q", got)
	}
	b.dragTo(rowNamed(b, "C"), rowNamed(b, "Inner"), 0.5, func() {
		time.Sleep(400 * time.Millisecond)
		if got := b.eval(`[...document.querySelectorAll('.nav3 .drop-into')].map(n => n.textContent.trim()).join()`); got != "Inner" {
			t.Errorf("waiting on the middle of Inner frames it: %q", got)
		}
	})
	if got := kids(inner); got != "C" {
		t.Errorf("onto an empty collection inside Team, after waiting: %q", got)
	}
}

// The settings of a collection are in its overview: the right click opens it ready to rename, nav3 keeps its tree, and a
// collection one may only read shows no settings.
func TestSettingsInTheOverview(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.rightClickOn(`.nav2 [data-act="coll"][data-id="` + team + `"]`)
	b.clickOn(probe(b, `.ctxpop button`, "Settings and name"), 1)
	if !b.waitFor(`document.activeElement?.id === 'cname' && document.querySelector('.dtitle h1')?.textContent === 'Team'`) {
		t.Fatalf("the overview of Team opens with its name ready: %q", b.eval(`document.activeElement?.outerHTML.slice(0, 120)`))
	}
	if got := b.eval(`!!document.getElementById('tree') && [...document.querySelectorAll('article.doc h2')].some(n => n.textContent === 'Settings')`); got != "true" {
		t.Error("nav3 keeps its tree, and the settings are in the overview")
	}
	b.shot("settings-in-overview")
	b.typeText("Team space")
	b.press("Enter", "Enter", 13, 0)
	b.eval(`document.activeElement.blur()`)
	if !b.waitFor(`[...document.querySelectorAll('.nav2 .lbl')].some(n => n.textContent === 'Team space')`) {
		t.Errorf("the new name shows in nav2: %s", b.eval(`document.querySelector('.nav2').innerText.replace(/\\s+/g, ' ')`))
	}
	b.clickOn(`article.doc [data-act="cacc"][data-id="comment"]`, 1)
	if got := b.eval(`document.querySelector('article.doc [data-act="cacc"][data-id="comment"]').getAttribute('aria-pressed')`); got != "true" {
		t.Errorf("others may comment now: %q", got)
	}
	time.Sleep(400 * time.Millisecond)
	b.open(base + "/o/" + team)
	b.ready("Team space")
	if got := b.eval(`!!document.getElementById('cname')`); got != "true" {
		t.Error("the overview holds the settings")
	}
	// The archive of the demo may only be read: no settings there.
	b.open(base + "/dev/ui")
	b.ready("")
	b.eval(`openDoc('arc'); render()`)
	if !b.waitFor(`document.querySelector('.dtitle h1')?.textContent.startsWith('Archive')`) {
		t.Fatal("no archive")
	}
	if got := b.eval(`!!document.getElementById('cname')`); got != "false" {
		t.Error("a collection one may only read shows settings")
	}
}

// The settings of the workspace belong to the account: in a browser that kept nothing, nav2 comes back as it was left.
func TestSettingsFollowTheAccount(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.rightClickOn(`.nav2 [data-act="coll"][data-id="` + team + `"]`)
	b.clickOn(probe(b, `.ctxpop button`, "Line below it"), 1)
	b.clickOn(`.nav2 [data-act="coll"][data-id="`+team+`"]`, 2) // kept open
	b.eval(`S.s2 = 2; render()`)
	state := func() string {
		return b.eval(`[[...document.querySelectorAll('#clist > *')].map(n => n.dataset.deco || (n.dataset.favid === '` + team + `' ? 'Team' : '')).filter(Boolean).join(' '), !!document.querySelector('.nav2 [data-id="` + team + `"][data-pinned]'), S.s2].join(' | ')`)
	}
	before := state()
	if before != "Team line | true | 2" {
		t.Fatalf("the setup: %q", before)
	}
	if !b.waitFor(`sentSettings !== null && sentSettings === localStorage.getItem('ud.workspace')`) {
		t.Fatal("the settings did not go to the server")
	}
	b.eval(`localStorage.clear()`)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	if got := state(); got != before {
		t.Errorf("a browser that kept nothing gets them from the account: %q, want %q", got, before)
	}
}

// The owner sets what others may do, in the overview of a collection; the database keeps it, nav3 shows the lock, and
// someone else then may change nothing there.
func TestAccessSetByTheOwner(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	b.clickOn(`article.doc [data-act="cacc"][data-id="read"]`, 1)
	time.Sleep(400 * time.Millisecond)
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := b.eval(`[document.querySelector('article.doc [data-act="cacc"][aria-pressed="true"]')?.dataset.id, !!document.querySelector('.cheadrow[data-cdrop="` + team + `"] .acc')].join(' ')`); got != "read true" {
		t.Errorf("the database keeps it, nav3 shows the lock: %q", got)
	}
	// Someone else.
	b.call("Network.setExtraHTTPHeaders", map[string]any{"headers": map[string]string{"Remote-User": "bob", "Remote-Name": "Bob Kern", "Remote-Email": "bob@example.com", "Remote-Groups": "ops"}}, nil)
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := b.eval(`[!!document.getElementById('cname'), document.querySelector('.cheadrow[data-cdrop="` + team + `"] .newplus')?.disabled].join(' ')`); got != "false true" {
		t.Errorf("for someone else no settings, and New is off: %q", got)
	}
	// Back to edit: someone else may rename it, but not set what others may do.
	b.signIn(true)
	b.open(base + "/o/" + team)
	b.ready("Team")
	b.clickOn(`article.doc [data-act="cacc"][data-id="edit"]`, 1)
	time.Sleep(400 * time.Millisecond)
	b.call("Network.setExtraHTTPHeaders", map[string]any{"headers": map[string]string{"Remote-User": "bob", "Remote-Name": "Bob Kern", "Remote-Email": "bob@example.com", "Remote-Groups": "ops"}}, nil)
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := b.eval(`[!!document.getElementById('cname'), [...document.querySelectorAll('article.doc [data-act="cacc"]')].every(n => n.disabled)].join(' ')`); got != "true true" {
		t.Errorf("someone who may edit renames, but sets no level: %q", got)
	}
	b.clickOn(rowNamed(b, "A")+` [data-act="open"]`, 1)
	b.ready("A")
	b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
	if got := b.eval(`(() => { const l = [...document.querySelectorAll('[data-act="setacc"]')]; return l.length > 0 && l.every(n => n.disabled); })()`); got != "true" {
		t.Error("in the settings of a page, someone else sets no level either")
	}
	b.signIn(true)
}

// Moments come in UTC and show in the time zone of the device.
func TestTimesShowInTheTimeZoneOfTheDevice(t *testing.T) {
	b, base := setupWithDatabase(t)
	shown := func(zone string) string {
		b.call("Emulation.setTimezoneOverride", map[string]any{"timezoneId": zone}, nil)
		b.open(base + "/o/plan")
		b.ready("Network plan")
		return b.eval(`when('2026-11-09T05:54:00Z') + ' | ' + when('2026-07-01T22:30:00Z', false) + ' | ' + when('today')`)
	}
	if got := shown("Europe/Berlin"); got != "Nov 09 06:54 | Jul 02 | today" {
		t.Errorf("in Berlin: %q", got)
	}
	if got := shown("America/New_York"); got != "Nov 09 00:54 | Jul 01 | today" {
		t.Errorf("in New York: %q", got)
	}
	// A page of the database shows when it changed as a moment of this device, not as the stamp of the server.
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	b.clickOn(rowNamed(b, "A")+` [data-act="open"]`, 1)
	b.ready("A")
	b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
	if got := b.eval(`(() => { const t = document.querySelector('.fdrawer')?.innerText || ''; return /\d{4}-\d\d-\d\dT/.test(t) + ' ' + /(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec) \d\d \d\d:\d\d/.test(t); })()`); got != "false true" {
		t.Errorf("changed shows as a local moment: %s", b.eval(`document.querySelector('.fdrawer')?.innerText`))
	}
	b.call("Emulation.setTimezoneOverride", map[string]any{"timezoneId": ""}, nil)
}

// Private notes stay at the top: dragged onto a collection of nav2 or into nav3, they do not go.
func TestPrivateNotesStayAtTheTop(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	notes := probe(b, `.nav2 [data-act="coll"]`, "My notes")
	b.dragTo(notes, `.nav2 [data-act="coll"][data-id="`+team+`"]`, 0.5, func() {
		time.Sleep(400 * time.Millisecond)
		if got := b.eval(`document.querySelectorAll('.drop-into').length`); got != "0" {
			t.Error("Team offers to take the private notes")
		}
	})
	b.dragTo(probe(b, `.nav2 [data-act="coll"]`, "My notes"), rowNamed(b, "A"), 0.8, func() {
		if got := b.eval(`!!document.querySelector('.nav3 .dropgap:not(.idle)')`); got != "false" {
			t.Error("the private notes find a place in nav3")
		}
	})
	time.Sleep(300 * time.Millisecond)
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := b.eval(`[...document.querySelectorAll('#clist [data-act="coll"]')].some(n => n.textContent.trim() === 'My notes')`); got != "true" {
		t.Error("the private notes left nav2")
	}
}

// The address of a block, pasted alone into an empty block of another page, embeds it there: live, with a link back, and
// the source says where it is used.
func TestEmbedABlockByItsAddress(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.eval(`window.ids = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		const team = await post('/api/collections', {title: 'Team'});
		const plan = await post('/api/pages', {title: 'Plan', parent: team.id});
		const b1 = await post('/api/pages/' + plan.id + '/blocks', {md: '## Switches', after: ''});
		const b2 = await post('/api/pages/' + plan.id + '/blocks', {md: '| Port | VLAN |\n|---|---|\n| 1 | 10 |', after: b1.id});
		await post('/api/pages/' + plan.id + '/blocks', {md: '', after: b2.id});
		const other = await post('/api/pages', {title: 'Other', parent: team.id});
		await post('/api/pages/' + other.id + '/blocks', {md: '::embed{ref="' + plan.id + '#b3"}', after: ''});
		const rack = await post('/api/pages', {title: 'Rack', parent: team.id});
		await post('/api/pages/' + rack.id + '/blocks', {md: '', after: ''});
		window.ids = [plan.id, rack.id];
	})()`)
	if !b.waitFor(`!!window.ids`) {
		t.Fatal("the pages for the test were not made")
	}
	plan, rack := b.eval(`window.ids[0]`), b.eval(`window.ids[1]`)
	b.open(base + "/o/" + rack)
	b.ready("Rack")
	b.clickOn(`[data-act="edit"]`, 1)
	if !b.waitFor(`!!document.querySelector('textarea[data-blk="b1"]')`) {
		t.Fatal("no field to write in")
	}
	b.eval(`(() => { const x = document.querySelector('textarea[data-blk="b1"]'); x.focus(); const dt = new DataTransfer(); dt.setData('text/plain', location.origin + '/o/` + plan + `#b2');
		x.dispatchEvent(new ClipboardEvent('paste', {clipboardData: dt, bubbles: true, cancelable: true})); })()`)
	if got := b.eval(`document.querySelector('textarea[data-blk="b1"]').value`); got != `::embed{ref="`+plan+`#b2"}` {
		t.Fatalf("pasting the address alone makes it an embedding: %q", got)
	}
	b.press("Enter", "Enter", 13, 2) // Ctrl+Enter saves
	b.clickOn(`[data-act="edit"]`, 1)
	if !b.waitFor(`!!document.querySelector('article.doc figure.embed td')`) {
		t.Fatalf("the table of Plan shows in Rack: %s", b.eval(`document.querySelector('article.doc')?.innerHTML.slice(0, 300)`))
	}
	b.shot("embedded")
	if got := b.eval(`document.querySelector('article.doc figure.embed td:last-child').textContent + ' | ' + document.querySelector('article.doc figure.embed figcaption').textContent.trim()`); got != "10 | Plan · #b2" {
		t.Errorf("what shows and where it comes from: %q", got)
	}
	b.clickOn(`article.doc figure.embed figcaption a`, 1)
	b.ready("Plan")
	b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
	if got := b.eval(`document.querySelector('.fdrawer')?.innerText.includes('Rack (block #b2)')`); got != "true" {
		t.Errorf("Plan says where it is used: %s", b.eval(`document.querySelector('.fdrawer')?.innerText`))
	}
	// In editing, a block says where it is used, and an empty one in use asks before it goes.
	b.clickOn(`[data-act="edit"]`, 1)
	if !b.waitFor(`!!document.querySelector('textarea[data-blk="b3"]')`) {
		t.Fatal("Plan is not in editing")
	}
	if got := b.eval(`[...document.querySelectorAll('.bbar em')].map(n => n.textContent.trim()).join(' | ')`); got != "#b1 | Used in 1 · #b2 | Used in 1 · #b3" {
		t.Errorf("the bars: %q", got)
	}
	b.clickOn(`[data-act="blkdel"][data-id="b3"]`, 1)
	if got := b.eval(`[!!document.querySelector('textarea[data-blk="b3"]'), document.querySelector('[data-act="blkdel"][data-id="b3"]')?.textContent].join(' ')`); got != "true Delete? Yes" {
		t.Errorf("an empty block in use asks first: %q", got)
	}
}

// Editing is found: the pencil is always there, greyed with the reason where a page cannot be edited, and a double click on
// a block of a page one may edit opens editing at that block.
func TestEditingIsFound(t *testing.T) {
	b, base := setupWithDatabase(t)
	pencil := `(() => { const p = document.querySelector('[data-win="0"] [data-act="edit"]'); return p ? p.disabled + ' ' + p.title : 'none'; })()`
	b.open(base + "/o/plan")
	b.ready("Network plan")
	if got := b.eval(pencil); !strings.HasPrefix(got, "true From the folder source, read only") {
		t.Errorf("a page of the folder source: %q", got)
	}
	b.eval(`window.made = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		const team = await post('/api/collections', {title: 'Team'});
		const page = await post('/api/pages', {title: 'Notes', parent: team.id});
		const b1 = await post('/api/pages/' + page.id + '/blocks', {md: 'First', after: ''});
		await post('/api/pages/' + page.id + '/blocks', {md: 'Second', after: b1.id});
		window.made = page.id;
	})()`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the page for the test was not made")
	}
	b.open(base + "/o/" + b.eval(`window.made`))
	b.ready("Notes")
	if got := b.eval(pencil); got != "false Edit block by block (or double click a block)" {
		t.Errorf("a page of the database: %q", got)
	}
	b.clickOn(`article.doc [data-b="b2"]`, 2)
	if !b.waitFor(`document.activeElement?.dataset?.blk === 'b2'`) {
		t.Fatalf("a double click edits that block: %s", b.eval(`document.activeElement?.outerHTML.slice(0, 120)`))
	}
}
