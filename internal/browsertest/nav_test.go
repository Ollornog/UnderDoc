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
	const r = g.getBoundingClientRect(), up = [...document.querySelectorAll('#clist > :not(.dropgap), [aria-label=Nav3] li > div, [aria-label=Nav3] section > header')].filter(n => !n.closest('.dropgap')).filter(n => n.offsetParent && n.getBoundingClientRect().bottom <= r.top + 1 && r.top - n.getBoundingClientRect().bottom < 6).pop();
	return up ? (up.dataset.deco || up.dataset.favid || up.textContent.trim()) + ' ' + Math.round(r.height) : 'nothing above ' + Math.round(r.top); })()`

// makeTeam makes a collection with pages A, B and C through the API, as a setup, and returns its ID.
func makeTeam(b *browser) string {
	b.t.Helper()
	b.eval(`window.made = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		const team = await post('/api/collections', {title: 'Team'});
		// Shared with everyone to edit, as its maker does in the step Share with (ADR-29, 2026-11-19).
		await fetch('/api/objects/' + team.id + '/grants', {method: 'PUT', headers: json, body: JSON.stringify({kind: 'tenant', role: 'edit'})});
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
		return b.eval(`getComputedStyle(document.querySelector('[aria-label=Nav2] [data-act="coll"][data-id="` + id + `"]')).boxShadow`)
	}
	b.clickOn(`[aria-label=Nav2] [data-act="coll"][data-id="ops"]`, 1)
	// The line at the start: 1px while open for now, 3px when kept open (the pack's --workspace-open-bar and
	// --workspace-pinned-bar), a difference in shape, not in colour.
	if got := bar("ops"); !strings.Contains(got, " 1px 0px 0px 0px inset") {
		t.Errorf("open for now: a thin line on the left, got %q", got)
	}
	if got := b.eval(`document.querySelector('[aria-label=Nav2] [data-act="coll"][data-id="ops"]').getAttribute('aria-current')`); got != "page" {
		t.Errorf("open for now: shown as open: %q", got)
	}
	b.clickOn(`[aria-label=Nav2] [data-act="coll"][data-id="ops"]`, 2)
	if !b.waitFor(`!!document.querySelector('[aria-label=Nav2] [data-id="ops"][data-pinned]')`) {
		t.Fatal("a double click keeps it open")
	}
	if got := b.eval(`document.querySelector('[aria-label=Nav2] [data-act="coll"][data-id="ops"]').getAttribute('aria-current')`); got != "page" {
		t.Errorf("kept open: still open: %q", got)
	}
	if got := bar("ops"); !strings.Contains(got, " 3px 0px 0px 0px inset") {
		t.Errorf("kept open: a thick line on the left, got %q", got)
	}
	b.clickOn(`[aria-label=Nav2] [data-act="coll"][data-id="pro"]`, 1)
	b.shot("nav2-thin-and-thick")
	if got := b.eval(`getComputedStyle(document.querySelector('[aria-label=Nav3] section > header > a')).fontSize`); got != "13.5px" {
		t.Errorf("the name of a collection in nav3 reads as a heading: %q", got)
	}
	// Each open collection begins with a band of its own; the second is set apart by space and a line above it.
	if got := b.eval(`(() => { const h = [...document.querySelectorAll('[aria-label=Nav3] section > header')]; if (h.length < 2) return 'only ' + h.length; const s = getComputedStyle(h[1]), c = getComputedStyle(h[1].parentElement); return [s.backgroundColor !== 'rgba(0, 0, 0, 0)', parseFloat(c.marginTop) >= 8 && parseFloat(c.borderTopWidth) >= 1].join(' '); })()`); got != "true true" {
		t.Errorf("collections in nav3 are not set apart: %q", got)
	}
	if got := b.eval(`document.querySelectorAll('[aria-label=Nav2] .star, [aria-label=Nav2] .pinmark, [data-act="star"]').length`); got != "0" {
		t.Errorf("no star and no pin any more: %s", got)
	}
	for s := 0; s <= 3; s++ {
		b.eval(`S.s2=` + strconv.Itoa(s) + `;render()`)
		if got := b.eval(`(() => { const h = document.querySelector('[aria-label=Nav2] > header').getBoundingClientRect(), n = document.querySelector('[aria-label=Nav2] > header .btn').getBoundingClientRect(); return n.right <= h.right + 0.5 && n.left >= h.left - 0.5; })()`); got != "true" {
			t.Errorf("at step %d, New is cut off", s)
		}
	}
	if got := b.eval(`getComputedStyle(document.querySelector('[aria-label=Nav3] section')).borderTopWidth`); got != "0px" {
		t.Errorf("no line above the first collection of nav3: %q", got)
	}
}

func TestDividersInNav2(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.rightClickOn(`[aria-label=Nav2] [data-favid="ops"]`)
	b.clickOn(probe(b, `[data-cm-panel] button`, "Line below it"), 1)
	if got := b.eval(`[...document.querySelectorAll('#clist > *')].map(n => n.dataset.deco || n.dataset.favid).slice(0, 8).join(' ')`); !strings.Contains(got, "ops line") {
		t.Fatalf("the line is below ops: %q", got)
	}
	// While dragging, the line leaves its place and a gap of its height opens below pro, drawn with a dashed border.
	h := b.eval(`Math.round(document.querySelector('#clist [data-deco]').getBoundingClientRect().height)`)
	b.dragTo(`#clist [data-deco]`, `#clist [data-favid="pro"]`, 0.95, func() {
		b.shot("nav2-gap")
		if got := b.eval(gapAt); got != "pro "+h {
			t.Errorf("the gap is not below pro: %q", got)
		}
		if got := b.eval(`(() => { const s = getComputedStyle(document.querySelector('.dropgap')); return s.outlineStyle + ' ' + s.outlineWidth; })()`); got != "dashed 1px" {
			t.Errorf("the place is not drawn: %q", got)
		}
		if got := b.eval(`[document.querySelector('#clist [data-deco][data-favid]').offsetParent === null, document.querySelectorAll('.dropgap [data-deco]').length, !!document.getElementById('ghost')].join(' ')`); got != "true 1 false" {
			t.Errorf("the line itself moves through the list, no picture floats: %q", got)
		}
	})
	if got := b.eval(`[...document.querySelectorAll('#clist > *')].map(n => n.dataset.deco || n.dataset.favid).join(' ')`); !strings.Contains(got, "pro line") {
		t.Errorf("the line moved below pro: %q", got)
	}
	b.settingsSent()
	b.open(base + "/o/plan")
	b.ready("Network plan")
	if got := b.eval(`[...document.querySelectorAll('#clist > *')].map(n => n.dataset.deco || n.dataset.favid).join(' ')`); !strings.Contains(got, "pro line") {
		t.Errorf("the browser keeps the line: %q", got)
	}
	b.clickOn(`[aria-label=Nav2] [data-act="coll"][data-id="pro"]`, 2)
	b.shot("nav2-line-and-kept-open")
	b.rightClickOn(`#clist [data-deco]`)
	b.clickOn(probe(b, `[data-cm-panel] button`, "Remove"), 1)
	if got := b.eval(`document.querySelectorAll('#clist [data-deco]').length`); got != "0" {
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
		return b.eval(`[...document.querySelectorAll('[aria-label=Nav3] section > header[data-cdrop="` + team + `"] + ul > li > div')].map(n => n.dataset.deco || n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim()).join(' ')`)
	}
	if got := names(); got != "A B C" {
		t.Fatalf("the pages of the team: %q", got)
	}
	b.rightClickOn(probe(b, `[aria-label=Nav3] li > div`, "A"))
	b.clickOn(probe(b, `[data-cm-panel] button`, "Line below it"), 1)
	if !b.waitFor(`document.querySelectorAll('[aria-label=Nav3] li > div[data-deco]').length === 1`) {
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
	// The drop is written in the background; on a busy machine a reload right after it came too early (2026-11-17).
	if !b.waitFor(`(() => { fetch('/api/state').then(r => r.json()).then(s => { const find = (l) => { for (const n of l || []) { if (n.id === '` + team + `') return n; const k = find(n.kids); if (k) return k; } return null; }; const t = find(s.collections); window.order = t ? t.kids.map(k => k.t === 'line' ? 'line' : k.n).join(' ') : ''; }); return window.order === 'C A line B'; })()`) {
		t.Errorf("the server has the order of the drop: %q", b.eval(`window.order`))
	}
	b.open(base + "/o/" + team)
	b.ready("Team")
	if !b.waitFor(`document.querySelectorAll('[aria-label=Nav3] li > div[data-deco]').length === 1`) || names() != "C A line B" {
		t.Errorf("the database keeps the order and the line: %q", names())
	}
	b.shot("nav3-line")
	b.rightClickOn(`[aria-label=Nav3] li > div[data-deco]`)
	b.clickOn(probe(b, `[data-cm-panel] button`, "Remove"), 1)
	if !b.waitFor(`document.querySelectorAll('[aria-label=Nav3] li > div[data-deco]').length === 0`) {
		t.Error("the line was not removed")
	}
	if !b.waitFor(`(() => { fetch('/api/state').then(r => r.json()).then(s => { const find = (l) => { for (const n of l || []) { if (n.id === '` + team + `') return n; const k = find(n.kids); if (k) return k; } return null; }; const t = find(s.collections); window.order = t ? t.kids.map(k => k.t === 'line' ? 'line' : k.n).join(' ') : ''; }); return window.order === 'C A B'; })()`) {
		t.Errorf("the server removed the line: %q", b.eval(`window.order`))
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
	b.waitFor(`[...document.querySelectorAll('[aria-label=Nav3] li > div')].some(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim() === ` + strconv.Quote(name) + `)`)
	id := b.eval(`[...document.querySelectorAll('[aria-label=Nav3] li > div')].find(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim() === ` + strconv.Quote(name) + `)?.dataset.row`)
	return `[aria-label=Nav3] div[data-row="` + id + `"]`
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
	if got := b.eval(`[document.querySelectorAll('.dropgap, #ghost').length, [...document.querySelectorAll('[aria-label=Nav3] section > header[data-cdrop="` + team + `"] + ul > li > div')].map(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim()).join(' ')].join(' ')`); got != "0 A B C" {
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
	if got := b.eval(`[...document.querySelectorAll('.dropgap > div')].map(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim()).join() + ' ' + !!document.getElementById('ghost')`); got != "C false" {
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
	if got := b.eval(`[...document.querySelectorAll('[aria-label=Nav3] section > header[data-cdrop="` + team + `"] + ul > li > div')].map(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim()).join(' ')`); got != "A C B" {
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
	if got := b.eval(`[...document.querySelectorAll('[aria-label=Nav3] section > header[data-cdrop="` + team + `"] + ul > li > div')].map(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim()).join(' ')`); got != "A B" {
		t.Errorf("C left the top of the team: %q", got)
	}
	if !b.waitFor(`[...document.querySelectorAll('[aria-label=Nav3] li > div')].some(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim() === 'C' && n.closest('ul:not(.workspace-tree)'))`) {
		t.Errorf("the database keeps C below A: %s", b.eval(`document.querySelector('[aria-label=Nav3]').innerText.replace(/\s+/g, ' ')`))
	}
}

// The private lists of nav2 sort the same way: the list leaves its place and lands in the gap.
func TestPrivateListsSortWithAGap(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/dev/ui")
	b.ready("")
	b.clickOn(`[aria-label=Nav1] [data-act="view"][data-id="todo"]`, 1)
	order := func() string {
		return b.eval(`[...document.querySelectorAll('[aria-label=Nav2] [data-secid]')].map(n => n.dataset.secid).join(' ')`)
	}
	if got := order(); got != "mine shop car" {
		t.Fatalf("the lists: %q", got)
	}
	b.dragTo(`[aria-label=Nav2] [data-secid="car"]`, `[aria-label=Nav2] [data-secid="mine"]`, 0.2, func() {
		if got := b.eval(`(() => { const g = document.querySelector('[aria-label=Nav2] .dropgap'), m = document.querySelector('[aria-label=Nav2] [data-secid="mine"]'); return !!g && g.getBoundingClientRect().bottom <= m.getBoundingClientRect().top + 1; })()`); got != "true" {
			t.Errorf("the gap is not above the first list")
		}
	})
	if got := order(); got != "car mine shop" {
		t.Errorf("after the drop: %q", got)
	}
	// Ctrl and click selects here too, and keeps nothing open.
	r := b.rect(`[aria-label=Nav2] [data-secid="shop"]`)
	b.mouse("mouseMoved", r.X, r.Y, 0, 0)
	b.mouse("mousePressed", r.X, r.Y, 1, 2)
	b.mouse("mouseReleased", r.X, r.Y, 1, 2)
	if got := b.eval(`[document.querySelector('[aria-label=Nav2] [data-secid="shop"]').getAttribute('aria-selected'), document.querySelectorAll('[aria-label=Nav2] [data-pinned]').length].join(' ')`); got != "true 0" {
		t.Errorf("Ctrl and click selects a list: %q", got)
	}
}

// Nothing lands where one may only read: no gap opens there, and a drop leaves everything where it was.
func TestNoGapWhereOneMayOnlyRead(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/dev/ui")
	b.ready("")
	b.eval(`S.view = 'workspace'; S.open = {ops: true, arc: true}; render()`)
	src := `[aria-label=Nav3] div[data-row="hand"]`
	if b.eval(`!!document.querySelector('`+src+`') && !!document.querySelector('[aria-label=Nav3] div[data-row="old"]')`) != "true" {
		t.Fatalf("the demo changed: %s", b.eval(`document.querySelector('[aria-label=Nav3]').innerText.replace(/\s+/g, ' ')`))
	}
	// Onto it, after waiting, and below it: the entry stays faint where it was, and nothing is framed.
	b.dragTo(`[aria-label=Nav3] div[data-row="net"]`, `[aria-label=Nav3] div[data-row="old"]`, 0.5, func() {
		time.Sleep(600 * time.Millisecond)
		if got := b.eval(`!!document.querySelector('.dropgap.idle') && !document.querySelector('.drop-into') && !document.querySelector('[data-row="old"]').closest('ul').querySelector('.dropgap')`); got != "true" {
			t.Errorf("onto an entry of the archive, which may only be read: %s", b.eval(gapAt))
		}
	})
	b.dragTo(`[aria-label=Nav3] div[data-row="net"]`, `[aria-label=Nav3] div[data-row="old"]`, 0.9, func() {
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
		return b.eval(`[...(document.querySelector(` + strconv.Quote(rowNamed(b, name)) + `)?.closest('li').querySelectorAll(':scope > ul > li > div') || [])].map(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim()).join(' ')`)
	}
	tops := func() string {
		return b.eval(`[...document.querySelectorAll('[aria-label=Nav3] section > header[data-cdrop="` + team + `"] + ul > li > div')].map(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim()).join(' ')`)
	}
	b.dragTo(rowNamed(b, "C"), rowNamed(b, "A"), 0.5, func() {
		// Up and down goes between entries; only waiting on the middle of A goes into it.
		if got := b.eval(`document.querySelectorAll('[aria-label=Nav3] .drop-into').length`); got != "0" {
			t.Errorf("at once, the middle of A is not yet onto it")
		}
		// Nothing answers the pointer while it drags, so nothing lights up under it.
		if got := b.eval(`getComputedStyle(document.querySelector(` + strconv.Quote(rowNamed(b, "A")) + `)).pointerEvents`); got != "none" {
			t.Errorf("an entry answers the pointer while dragging: %q", got)
		}
		time.Sleep(350 * time.Millisecond)
		b.shot("drop-onto")
		if got := b.eval(`[...document.querySelectorAll('[aria-label=Nav3] .drop-into')].map(n => n.textContent.trim()).join() + ' ' + getComputedStyle(document.querySelector('.dropgap')).opacity`); got != "A 0.4" {
			t.Errorf("after waiting, A is framed and C waits faint: %q", got)
		}
	})
	if got := kids("A"); got != "C" {
		t.Fatalf("C is below A: %q", got)
	}
	b.dragTo(rowNamed(b, "B"), rowNamed(b, "C"), 0.8, func() {
		if got := b.eval(`document.querySelectorAll('[aria-label=Nav3] .drop-into').length`); got != "0" {
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

// TestOnlyWhoManagesMovesElsewhere: an entry the viewer may only edit is reordered in its place, but no gap opens for it
// in another place, since moving it there needs the role manage (ADR-29, 2026-11-19).
func TestOnlyWhoManagesMovesElsewhere(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	kids := func(name string) string {
		return b.eval(`[...(document.querySelector(` + strconv.Quote(rowNamed(b, name)) + `)?.closest('li').querySelectorAll(':scope > ul > li > div') || [])].map(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim()).join(' ')`)
	}
	tops := func() string {
		return b.eval(`[...document.querySelectorAll('[aria-label=Nav3] section > header[data-cdrop="` + team + `"] + ul > li > div')].map(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim()).join(' ')`)
	}
	// C is someone else's page to the workspace, which the viewer may edit.
	c := rowNamed(b, "C")
	b.eval(`(() => { const d = D[document.querySelector(` + strconv.Quote(c) + `).dataset.row]; d.role = 'edit'; d.mine = false; })()`)
	b.dragTo(c, rowNamed(b, "A"), 0.5, func() {
		time.Sleep(350 * time.Millisecond)
		if got := b.eval(`document.querySelectorAll('[aria-label=Nav3] .drop-into').length`); got != "0" {
			t.Errorf("A is framed to take C, which the viewer only edits")
		}
	})
	if got := kids("A"); got != "" {
		t.Errorf("C went into A: %q", got)
	}
	b.dragTo(c, rowNamed(b, "A"), 0.8, nil)
	if got := tops(); got != "A C B" {
		t.Errorf("within its place, C is reordered: %q", got)
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
		return b.eval(`[...document.querySelectorAll('[aria-selected="true"]')].map(n => (n.querySelector(':scope > button:not([data-toggle])') || n).textContent.trim() || n.dataset.deco).join(' ')`)
	}
	tops := func() string {
		return b.eval(`[...document.querySelectorAll('[aria-label=Nav3] section > header[data-cdrop="` + team + `"] + ul > li > div')].map(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim()).join(' ')`)
	}
	title := b.eval(`document.querySelector('.workspace-main > header > h1').textContent`)
	ctrl(rowNamed(b, "A"), 2)
	ctrl(rowNamed(b, "C"), 2)
	if got := selected(); got != "A C" {
		t.Fatalf("Ctrl and click selects: %q", got)
	}
	b.shot("selected")
	if got := b.eval(`(() => { const s = getComputedStyle(document.querySelector('[aria-label=Nav3] [aria-selected="true"]')); return s.outlineStyle + ' ' + s.outlineWidth; })()`); got != "solid 1px" {
		t.Errorf("a selected entry is framed: %q", got)
	}
	if got := b.eval(`document.querySelector('.workspace-main > header > h1').textContent`); got != title {
		t.Errorf("selecting opens nothing: %q", got)
	}
	b.dragTo(rowNamed(b, "A"), rowNamed(b, "B"), 0.8, func() {
		b.shot("select-drag")
		if got := b.eval(`[...document.querySelectorAll('.dropgap > div')].map(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim()).join(' ')`); got != "A C" {
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
	b.dragTo(rowNamed(b, "A"), `[data-win="0"] .workspace-main > section`, 0.5, func() {
		if got := b.eval(`!!document.querySelector('[data-win="0"] [data-drop-zone="here"]:not([hidden])') && !!document.querySelector('.dropgap.idle')`); got != "true" {
			t.Error("over the window it says it opens there")
		}
	})
	if got := b.eval(`[...document.querySelectorAll('[data-win="0"] .workspace-tab')].map(n => n.textContent.trim()).filter(n => n === 'A' || n === 'C').join(' ')`); got != "A C" {
		t.Errorf("dropped on the window, both open as tabs: %s", b.eval(`document.querySelector('[data-win="0"] .workspace-tabs').innerText.replace(/\s+/g, ' ')`))
	}
	if got := tops(); got != "B A C" {
		t.Errorf("opening moves nothing: %q", got)
	}
	// A tab still drags as a label; it takes the sizes of the workspace although it hangs outside it.
	b.dragTo(probe(b, `[data-win="0"] .workspace-tab`, "C"), `[data-win="0"] .workspace-main > section`, 0.5, func() {
		if got := b.eval(`getComputedStyle(document.getElementById('ghost')).paddingLeft`); got == "0px" || got == "" {
			t.Errorf("the label of a dragged tab has no padding: %q", got)
		}
	})

	// In nav2, Ctrl and click selects and no longer keeps a collection open; the right click keeps the selection open.
	ctrl(`[aria-label=Nav2] [data-act="coll"][data-id="ops"]`, 2)
	ctrl(probe(b, `[aria-label=Nav2] [data-act="coll"]`, "Home"), 2)
	if got := b.eval(`[...document.querySelectorAll('#clist [aria-selected="true"]')].map(n => (n.querySelector(':scope > button:not([data-toggle])') || n).textContent.trim()).join(' ') + ' ' + document.querySelectorAll('[aria-label=Nav2] [data-pinned]').length`); got != "Home Operations (demo) 0" {
		t.Fatalf("nav2 selects with Ctrl and click, and pins nothing: %q", got)
	}
	b.rightClickOn(`#clist [aria-selected="true"]`)
	b.clickOn(probe(b, `[data-cm-panel] button`, "Keep them open"), 1)
	if got := b.eval(`document.querySelectorAll('[aria-label=Nav2] [data-pinned]').length`); got != "2" {
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
	// A plain click opens the entry, whose page then loads and draws the workspace anew. A redraw between the press and
	// the release of the next click swallows that click, so the next one waits for the page (a red run under load,
	// 2026-11-17: the Ctrl click after a plain click went nowhere).
	click := func(sel string, mods int) {
		r := b.rect(sel)
		b.mouse("mouseMoved", r.X, r.Y, 0, 0)
		b.mouse("mousePressed", r.X, r.Y, 1, mods)
		b.mouse("mouseReleased", r.X, r.Y, 1, mods)
		if mods == 0 && !b.waitFor(`!Object.values(D).some(d => d && d.loading)`) {
			t.Fatalf("the page opened by a plain click on %s did not load", sel)
		}
	}
	selected := func() string {
		return b.eval(`[...document.querySelectorAll('[aria-label=Nav3] [aria-selected="true"]')].map(n => (n.querySelector(':scope > button:not([data-toggle])') || n).textContent.trim()).join(' ')`)
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
	click(probe(b, `[aria-label=Nav2] [data-act="coll"]`, "My notes"), 0)
	click(probe(b, `[aria-label=Nav2] [data-act="coll"]`, "Home"), 8)
	if got := b.eval(`[...document.querySelectorAll('#clist [aria-selected="true"]')].map(n => (n.querySelector(':scope > button:not([data-toggle])') || n).textContent.trim()).join(', ')`); !strings.HasPrefix(got, "My notes, ") || !strings.HasSuffix(got, ", Home") {
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
	target := probe(b, `[aria-label=Nav2] [data-act="coll"]`, "Other")
	b.dragTo(rowNamed(b, "A"), target, 0.2, func() {
		if got := b.eval(`(() => { const n = document.querySelector('#clist .drop-into'); return n ? n.textContent.trim() + ' ' + (getComputedStyle(n).boxShadow !== 'none') : 'none'; })()`); got != "Other true" {
			t.Errorf("Other is framed at once: %q", got)
		}
	})
	time.Sleep(500 * time.Millisecond)
	if got := b.eval(`[...document.querySelectorAll('[aria-label=Nav3] section > header[data-cdrop="` + team + `"] + ul > li > div')].map(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim()).join(' ')`); got != "B C" {
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
		return b.eval(`[...document.querySelectorAll('[aria-label=Nav3] section > header[data-cdrop="` + team + `"] + ul > li > div')].map(n => n.dataset.deco || n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim()).join(', ')`)
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
	if !b.waitFor(`[...document.querySelectorAll('[aria-label=Nav3] li > div')].some(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim() === 'C (copy)')`) {
		t.Fatalf("Ctrl at the drop copies: %q", tops())
	}
	if got := tops(); got != "A, C (copy), B, C" {
		t.Errorf("the copy lands where it was dropped, the original stays: %q", got)
	}
	b.rightClickOn(rowNamed(b, "B"))
	if got := b.eval(`[...document.querySelectorAll('[data-cm-panel] button')].every(n => n.querySelector('svg'))`); got != "true" {
		t.Errorf("an entry of the menu has no icon: %s", b.eval(`[...document.querySelectorAll('[data-cm-panel] button')].filter(n => !n.querySelector('svg')).map(n => n.textContent).join()`))
	}
	b.clickOn(probe(b, `[data-cm-panel] button`, "Duplicate"), 1)
	if !b.waitFor(`[...document.querySelectorAll('[aria-label=Nav3] li > div')].some(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim() === 'B (copy)')`) {
		t.Fatalf("Duplicate: %q", tops())
	}
	b.rightClickOn(`[aria-label=Nav3] section > header[data-cdrop="` + team + `"] > a`)
	b.clickOn(probe(b, `[data-cm-panel] button`, "Sort its children Z to A"), 1)
	if got := tops(); got != "C (copy), C, B (copy), B, A" {
		t.Errorf("sorted Z to A: %q", got)
	}
	// A line stays where it is: each group around it is sorted on its own.
	b.rightClickOn(rowNamed(b, "B (copy)"))
	b.clickOn(probe(b, `[data-cm-panel] button`, "Line below it"), 1)
	if !b.waitFor(`document.querySelectorAll('[aria-label=Nav3] li > div[data-deco]').length === 1`) {
		t.Fatal("no line")
	}
	b.rightClickOn(`[aria-label=Nav3] section > header[data-cdrop="` + team + `"] > a`)
	b.clickOn(probe(b, `[data-cm-panel] button`, "Sort its children A to Z"), 1)
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

// While a single window holds only the general tab, no tab row is drawn and the window takes its place; a button beside
// New holds what the tab offers, and a second tab brings the tabs back.
func TestOneWindowShowsNoTabs(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	if got := b.eval(`[document.querySelectorAll('.workspace-tabs, .workspace-tab').length, document.querySelectorAll('.workspace-top [data-act=winmenu]').length].join(' ')`); got != "0 1" {
		t.Fatalf("no tabs, a window button: %q", got)
	}
	b.clickOn(`[data-act=winmenu]`, 1)
	if got := b.eval(`[...document.querySelectorAll('[data-cm-panel] button')].every(n => n.querySelector('svg')) && !!document.querySelector('[data-cm-panel]')`); got != "true" {
		t.Errorf("the window menu, with icons: %s", b.eval(`document.querySelector('[data-cm-panel]')?.innerText`))
	}
	b.shot("window-menu")
	b.clickOn(probe(b, `[data-cm-panel] button`, "Keep as a tab of its own"), 1)
	if got := b.eval(`[document.querySelectorAll('.workspace-tabs').length, document.querySelectorAll('[data-act=winmenu]').length].join(' ')`); got != "1 0" {
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
		return b.eval(`[...document.querySelectorAll('#clist > [data-favid]:not([data-deco])')].map(n => n.textContent.trim() + (n.dataset.alias ? '*' : '')).join(', ')`)
	}
	if got := inNav2(); strings.Contains(got, "L1") || strings.Contains(got, "L2") {
		t.Errorf("collections inside others stay out of nav2 by themselves: %q", got)
	}
	if got := b.eval(`document.querySelectorAll('#clist [data-favid="` + team + `"] svg rect').length`); got != "4" {
		t.Errorf("a collection of the database has the icon of a collection, not of a folder: %s rects", got)
	}
	b.rightClickOn(rowNamed(b, "L2"))
	b.clickOn(probe(b, `[data-cm-panel] button`, "Show in nav2"), 1)
	if !b.waitFor(`!!document.querySelector('#clist [data-alias][data-favid="` + l2 + `"]')`) {
		t.Fatalf("L2 is shown in nav2: %q", inNav2())
	}
	if got := b.eval(`getComputedStyle(document.querySelector('#clist [data-alias] > span')).fontStyle`); got != "italic" {
		t.Errorf("the entry only points at L2: %q", got)
	}
	b.shot("nav2-shown")
	b.clickOn(`#clist [data-alias][data-act="coll"]`, 1)
	if !b.waitFor(`[...document.querySelectorAll('[aria-label=Nav3] section > header > a')].map(n => n.textContent.trim()).join() === 'L2'`) {
		t.Errorf("it opens in nav3 on its own: %s", b.eval(`document.querySelector('[aria-label=Nav3]').innerText.replace(/\s+/g, ' ')`))
	}
	if !b.waitFor(`[...document.querySelectorAll('[aria-label=Nav3] li > div')].some(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim() === 'Deep page')`) {
		t.Error("with its own entries")
	}
	b.settingsSent()
	b.open(base + "/o/" + team)
	b.ready("Team")
	if !b.waitFor(`!!document.querySelector('#clist [data-alias][data-favid="` + l2 + `"]')`) {
		t.Errorf("the browser keeps it in nav2: %q", inNav2())
	}
	if got := b.eval(`(() => { const deep = Object.keys(D).find(k => D[k].n === 'Deep page'); return PARENT[deep] === SID['` + l2 + `'] && COLL[SID['` + l2 + `']]?.id === SID['` + team + `']; })()`); got != "true" {
		t.Error("the entry in nav2 changed where the pages of L2 lie")
	}
	// It still lies in L1: the tree of Team holds it as before.
	if got := b.eval(`(() => { const n = [...document.querySelectorAll('[aria-label=Nav3] li > div')].find(n => n.querySelector(':scope > button:not([data-toggle])')?.textContent.trim() === 'L2'); return !!n && !!n.closest('ul:not(.workspace-tree)'); })()`); got != "true" {
		t.Errorf("L2 left L1: %s", b.eval(`document.querySelector('[aria-label=Nav3]').innerText.replace(/\s+/g, ' ')`))
	}
	b.rightClickOn(`#clist [data-alias]`)
	b.clickOn(probe(b, `[data-cm-panel] button`, "Remove from nav2"), 1)
	if got := b.eval(`document.querySelectorAll('#clist [data-alias]').length`); got != "0" {
		t.Errorf("removed from nav2: %q", inNav2())
	}
	b.clickOn(`section > header[data-cdrop="`+team+`"] .btn`, 1)
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
	if got := b.eval(`[...document.querySelectorAll('[data-cm-panel] button')].some(n => n.textContent.includes('Show in nav2'))`); got != "false" {
		t.Error("a folder offers to show in nav2")
	}
	b.clickOn(probe(b, `[data-cm-panel] button`, "Turn into a collection"), 1)
	time.Sleep(300 * time.Millisecond)
	if got := kind("Drawings"); got != "collection" {
		t.Errorf("the database has a collection: %q", got)
	}
	b.rightClickOn(rowNamed(b, "Drawings"))
	b.clickOn(probe(b, `[data-cm-panel] button`, "Show in nav2"), 1)
	if !b.waitFor(`[...document.querySelectorAll('#clist [data-alias]')].some(n => n.textContent.trim() === 'Drawings')`) {
		t.Fatal("Drawings is not shown in nav2")
	}
	b.rightClickOn(rowNamed(b, "Drawings"))
	b.clickOn(probe(b, `[data-cm-panel] button`, "Turn into a folder"), 1)
	time.Sleep(300 * time.Millisecond)
	if got := kind("Drawings"); got != "folder" {
		t.Errorf("and back into a folder: %q", got)
	}
	if got := b.eval(`[...document.querySelectorAll('#clist [data-alias]')].some(n => n.textContent.trim() === 'Drawings')`); got != "false" {
		t.Error("a folder stays shown in nav2")
	}
	b.rightClickOn(`[aria-label=Nav2] [data-act="coll"][data-id="` + team + `"]`)
	if got := b.eval(`[...document.querySelectorAll('[data-cm-panel] button')].some(n => n.textContent.includes('Turn into') || n.textContent.includes('Show in nav2'))`); got != "false" {
		t.Error("a collection at the top offers to become a folder or to be shown in nav2")
	}
}

// A right click menu stays inside the window, wherever the click is.
func TestMenusStayInTheWindow(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	inside := `(() => { const r = document.querySelector('[data-cm-panel]').getBoundingClientRect(); return r.top >= 0 && r.left >= 0 && r.bottom <= innerHeight && r.right <= innerWidth; })()`
	h := b.eval(`innerHeight`)
	hf, _ := strconv.ParseFloat(h, 64)
	n2 := b.rect(`[aria-label=Nav2]`)
	b.call("Input.dispatchMouseEvent", map[string]any{"type": "mouseMoved", "x": n2.X, "y": hf - 6}, nil)
	for _, kind := range []string{"mousePressed", "mouseReleased"} {
		b.call("Input.dispatchMouseEvent", map[string]any{"type": kind, "x": n2.X, "y": hf - 6, "button": "right", "buttons": 2, "clickCount": 1}, nil)
	}
	if !b.waitFor(`!!document.querySelector('[data-cm-panel]')`) {
		t.Fatal("no menu on the free part of nav2")
	}
	if got := b.eval(inside); got != "true" {
		t.Errorf("a menu opened at the bottom leaves the window: %s", b.eval(`JSON.stringify(document.querySelector('[data-cm-panel]').getBoundingClientRect())`))
	}
	// A long one, in the corner: inside too, and it scrolls.
	b.eval(`S.ctx = {kind: 'win', x: innerWidth - 10, y: innerHeight - 10}; render()`)
	if got := b.eval(inside); got != "true" {
		t.Errorf("a menu in the corner leaves the window: %s", b.eval(`JSON.stringify(document.querySelector('[data-cm-panel]').getBoundingClientRect())`))
	}
	if got := b.eval(`getComputedStyle(document.querySelector('[data-cm-panel]')).overflowY`); got != "auto" {
		t.Errorf("a long menu scrolls: %q", got)
	}
	// New near the bottom opens above its place, inside the window.
	b.eval(`S.ctx = null; S.addIn = 'n2'; S.addQ = ''; S.addAt = {x: 40, y: innerHeight - 20, up: innerHeight - 40}; render()`)
	if got := b.eval(`(() => { const r = document.querySelector('[data-menu="new"]').getBoundingClientRect(); return r.bottom <= innerHeight && r.top >= 0; })()`); got != "true" {
		t.Errorf("New leaves the window: %s", b.eval(`JSON.stringify(document.querySelector('[data-menu="new"]').getBoundingClientRect())`))
	}
	// The menu of a tab at the right edge moves in.
	b.eval(`S.addIn = null; const w = S.wins[0]; w.tabs.push(newTab(w.tabs[0].d)); S.tmenu = {w: 0, t: 1, x: innerWidth}; render()`)
	if got := b.eval(`(() => { const r = document.querySelector('[data-menu="tab"] > [data-cm-panel]').getBoundingClientRect(); return r.right <= innerWidth; })()`); got != "true" {
		t.Errorf("the menu of a tab leaves the window: %s", b.eval(`JSON.stringify(document.querySelector('[data-menu="tab"] > [data-cm-panel]').getBoundingClientRect())`))
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
	b.clickOn(`[aria-label=Nav2] [data-act="coll"][data-id="`+team+`"]`, 2)
	b.clickOn(`[aria-label=Nav2] [data-act="coll"][data-id="`+empty+`"]`, 1)
	if !b.waitFor(`!!document.querySelector('section > header[data-cdrop="` + empty + `"]') && !!document.querySelector('section > header[data-cdrop="` + team + `"]')`) {
		t.Fatalf("both are open: %s", b.eval(`document.querySelector('[aria-label=Nav3]').innerText.replace(/\s+/g, ' ')`))
	}
	kids := func(of string) string {
		return b.eval(`D[SID['` + of + `']].kids.map(k => D[k].n).join(' ')`)
	}
	b.dragTo(rowNamed(b, "A"), `section > header[data-cdrop="`+empty+`"]`, 0.9, nil)
	if got := kids(empty); got != "A" {
		t.Errorf("below the heading of the empty collection: %q", got)
	}
	b.dragTo(rowNamed(b, "B"), `section > header[data-cdrop="`+empty+`"]`, 0.5, func() { time.Sleep(400 * time.Millisecond) })
	if got := kids(empty); got != "A B" {
		t.Errorf("onto the heading, after waiting: %q", got)
	}
	b.dragTo(rowNamed(b, "C"), rowNamed(b, "Inner"), 0.5, func() {
		time.Sleep(400 * time.Millisecond)
		if got := b.eval(`[...document.querySelectorAll('[aria-label=Nav3] .drop-into')].map(n => n.textContent.trim()).join()`); got != "Inner" {
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
	b.rightClickOn(`[aria-label=Nav2] [data-act="coll"][data-id="` + team + `"]`)
	b.clickOn(probe(b, `[data-cm-panel] button`, "Settings and name"), 1)
	if !b.waitFor(`document.activeElement?.id === 'cname' && document.querySelector('.workspace-main > header > h1')?.textContent === 'Team'`) {
		t.Fatalf("the overview of Team opens with its name ready: %q", b.eval(`document.activeElement?.outerHTML.slice(0, 120)`))
	}
	if got := b.eval(`!!document.getElementById('tree') && [...document.querySelectorAll('article.typeset h2')].some(n => n.textContent === 'Settings')`); got != "true" {
		t.Error("nav3 keeps its tree, and the settings are in the overview")
	}
	b.shot("settings-in-overview")
	b.typeText("Team space")
	b.press("Enter", "Enter", 13, 0)
	b.eval(`document.activeElement.blur()`)
	if !b.waitFor(`[...document.querySelectorAll('[aria-label=Nav2] .workspace-item > span')].some(n => n.textContent === 'Team space')`) {
		t.Errorf("the new name shows in nav2: %s", b.eval(`document.querySelector('[aria-label=Nav2]').innerText.replace(/\\s+/g, ' ')`))
	}
	b.clickOn(`article.typeset [data-act="cacc"][data-id="comment"]`, 1)
	if got := b.eval(`document.querySelector('article.typeset [data-act="cacc"][data-id="comment"]').getAttribute('aria-pressed')`); got != "true" {
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
	if !b.waitFor(`document.querySelector('.workspace-main > header > h1')?.textContent.startsWith('Archive')`) {
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
	b.rightClickOn(`[aria-label=Nav2] [data-act="coll"][data-id="` + team + `"]`)
	b.clickOn(probe(b, `[data-cm-panel] button`, "Line below it"), 1)
	b.clickOn(`[aria-label=Nav2] [data-act="coll"][data-id="`+team+`"]`, 2) // kept open
	b.eval(`S.s2 = 2; render()`)
	state := func() string {
		return b.eval(`[[...document.querySelectorAll('#clist > *')].map(n => n.dataset.deco || (n.dataset.favid === '` + team + `' ? 'Team' : '')).filter(Boolean).join(' '), !!document.querySelector('[aria-label=Nav2] [data-id="` + team + `"][data-pinned]'), S.s2].join(' | ')`)
	}
	before := state()
	if before != "Team line | true | 2" {
		t.Fatalf("the setup: %q", before)
	}
	b.settingsSent()
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
	b.clickOn(`article.typeset [data-act="cacc"][data-id="read"]`, 1)
	time.Sleep(400 * time.Millisecond)
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := b.eval(`[document.querySelector('article.typeset [data-act="cacc"][aria-pressed="true"]')?.dataset.id, !!document.querySelector('section > header[data-cdrop="` + team + `"] [data-lock]')].join(' ')`); got != "read true" {
		t.Errorf("the database keeps it, nav3 shows the lock: %q", got)
	}
	// Someone else.
	b.call("Network.setExtraHTTPHeaders", map[string]any{"headers": map[string]string{"Remote-User": "bob", "Remote-Name": "Bob Kern", "Remote-Email": "bob@example.com", "Remote-Groups": "ops"}}, nil)
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := b.eval(`[!!document.getElementById('cname'), document.querySelector('section > header[data-cdrop="` + team + `"] .btn') === null].join(' ')`); got != "false true" {
		t.Errorf("for someone else no settings, and no New: %q", got)
	}
	// Back to edit: someone else may rename it, but not set what others may do.
	b.signIn(true)
	b.open(base + "/o/" + team)
	b.ready("Team")
	b.clickOn(`article.typeset [data-act="cacc"][data-id="edit"]`, 1)
	time.Sleep(400 * time.Millisecond)
	b.call("Network.setExtraHTTPHeaders", map[string]any{"headers": map[string]string{"Remote-User": "bob", "Remote-Name": "Bob Kern", "Remote-Email": "bob@example.com", "Remote-Groups": "ops"}}, nil)
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := b.eval(`[!!document.getElementById('cname'), [...document.querySelectorAll('article.typeset [data-act="cacc"]')].every(n => n.disabled)].join(' ')`); got != "true true" {
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
	if got := b.eval(`(() => { const t = document.querySelector('aside[data-drawer]')?.innerText || ''; return /\d{4}-\d\d-\d\dT/.test(t) + ' ' + /(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec) \d\d \d\d:\d\d/.test(t); })()`); got != "false true" {
		t.Errorf("changed shows as a local moment: %s", b.eval(`document.querySelector('aside[data-drawer]')?.innerText`))
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
	notes := probe(b, `[aria-label=Nav2] [data-act="coll"]`, "My notes")
	b.dragTo(notes, `[aria-label=Nav2] [data-act="coll"][data-id="`+team+`"]`, 0.5, func() {
		time.Sleep(400 * time.Millisecond)
		if got := b.eval(`document.querySelectorAll('.drop-into').length`); got != "0" {
			t.Error("Team offers to take the private notes")
		}
	})
	b.dragTo(probe(b, `[aria-label=Nav2] [data-act="coll"]`, "My notes"), rowNamed(b, "A"), 0.8, func() {
		if got := b.eval(`!!document.querySelector('[aria-label=Nav3] .dropgap:not(.idle)')`); got != "false" {
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
	if !b.waitFor(`document.activeElement?.classList.contains('cm-content')`) {
		t.Fatal("no editor to write in")
	}
	b.eval(`(() => { const dt = new DataTransfer(); dt.setData('text/plain', location.origin + '/o/` + plan + `#b2');
		document.querySelector('.cm-content').dispatchEvent(new ClipboardEvent('paste', {clipboardData: dt, bubbles: true, cancelable: true})); })()`)
	if got := b.eval(`EDS[at(0).d].ed.text()`); got != "<!-- block b1 -->\n::embed{ref=\""+plan+"#b2\"}" {
		t.Fatalf("pasting the address alone makes it an embedding: %q", got)
	}
	b.clickOn(`[data-act="edit"]`, 1) // leaving the editor saves it
	if !b.waitFor(`!!document.querySelector('article.typeset figure.editor-embed td')`) {
		t.Fatalf("the table of Plan shows in Rack: %s", b.eval(`document.querySelector('article.typeset')?.innerHTML.slice(0, 300)`))
	}
	b.shot("embedded")
	if got := b.eval(`document.querySelector('article.typeset figure.editor-embed td:last-child').textContent + ' | ' + document.querySelector('article.typeset figure.editor-embed figcaption').textContent.trim()`); got != "10 | Plan · #b2" {
		t.Errorf("what shows and where it comes from: %q", got)
	}
	b.clickOn(`article.typeset figure.editor-embed figcaption a`, 1)
	b.ready("Plan")
	b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
	if got := b.eval(`document.querySelector('aside[data-drawer]')?.innerText.includes('Rack (block #b2)')`); got != "true" {
		t.Errorf("Plan says where it is used: %s", b.eval(`document.querySelector('aside[data-drawer]')?.innerText`))
	}
	// In editing, the handle says where a block is used, and an empty one in use asks before it goes.
	b.clickOn(`[data-act="edit"]`, 1)
	if !b.waitFor(`!!document.querySelector('.cm-ud-marker[data-edb="b3"] .editor-handle > button')`) {
		t.Fatal("Plan is not in editing")
	}
	b.clickOn(`.cm-ud-marker[data-edb="b2"] .editor-handle > button`, 1)
	if got := b.eval(`document.querySelector('.editor-handle [role=menu] > [role=heading]')?.textContent`); got != "Used in Rack" {
		t.Errorf("the handle of b2: %q", got)
	}
	b.clickOn(`.workspace-main > header > h1`, 1) // closes the menu, which covers the handle below
	b.clickOn(`.cm-ud-marker[data-edb="b3"] .editor-handle > button`, 1)
	b.clickOn(`.editor-handle [role=menu] [data-act="eddel"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-ask="b3"]')`) {
		t.Errorf("an empty block in use asks first: %q", b.eval(`EDS[at(0).d].ed.text()`))
	}
}

// Editing is found: left of the title a large button says Reading or Editing and turns it, greyed with the reason where a
// page cannot be edited; the setting "open documents for editing" opens pages that way. In editing, the page is one
// editor with a bar above it (ADR-21).
func TestEditingIsFound(t *testing.T) {
	b, base := setupWithDatabase(t)
	status := `(() => { const p = document.querySelector('[data-win="0"] [data-mode]'); return p ? p.disabled + ' ' + p.getAttribute('aria-label') + ' | ' + p.title : 'none'; })()`
	b.open(base + "/o/plan")
	b.ready("Network plan")
	if got := b.eval(status); !strings.HasPrefix(got, "true Read only | Read only. From the folder source, read only") {
		t.Errorf("a page of the folder source: %q", got)
	}
	if got := b.eval(`(() => { const c = document.querySelector('[data-win="0"] [role=toolbar] [aria-label="Comments"]'); return (c ? c.disabled + ' ' + c.title : 'none') + ' | ' + document.querySelectorAll('article.typeset [data-comment]').length; })()`); got != "true No comments here: this page comes from the folder source, read only. Comments are kept on pages of the database. | 0" {
		t.Errorf("a page of the folder source takes no comments it would not keep: %q", got)
	}
	// New makes no page where it could not be written: in a collection of the folder source, Page says why.
	if got := b.eval(`(() => { try { const id = Object.keys(D).find(k => D[k].real && !D[k].db && D[k].t === 'coll'); openNew(id); render();
		const p = document.querySelector('[data-menu="new"] [data-act="add"][data-id="page"]'); return id + ' ' + (p ? p.disabled + ' ' + p.title : 'none'); } catch (e) { return 'error ' + e.message; } })()`); !strings.HasSuffix(got, "true The folder source is read only. Pages are made in collections of the database") {
		t.Errorf("Page in New of the folder source: %q", got)
	}
	b.eval(`S.addIn = null; render()`)
	if got := b.eval(`(() => { const id = Object.keys(D).find(k => D[k].real && !D[k].db && D[k].t === 'coll'); S.open[id] = true; render();
		const h = document.querySelector('section > header[data-cdrop="' + id + '"]'); return h ? String(!h.querySelector('header .btn')) : 'no head'; })()`); got != "true" {
		t.Errorf("a collection of the folder source shows no New in nav3: %q", got)
	}
	if got := b.eval(`(() => { const id = Object.keys(D).find(k => D[k].real && !D[k].db && D[k].t === 'page'); S.ctx = {kind: 'row', id, x: 20, y: 20}; render();
		const p = document.querySelector('[data-act="addchild"]'); return p ? p.disabled + ' ' + p.title : 'none'; })()`); got != "true The folder source is read only. Pages are made in collections of the database" {
		t.Errorf("New page below it, in the folder source: %q", got)
	}
	b.eval(`(() => { S.ctx = null; S.into = Object.keys(D).find(k => D[k].real && !D[k].db && D[k].t === 'coll'); S.newAt = 'bar'; render(); })()`)
	b.clickOn(`[data-act="newdoc"][data-id="page"]`, 1)
	if got := b.eval(`(document.querySelector('.toast')?.textContent || '') + ' | ' + (D[at(0).d]?.real ? 'kept' : 'made')`); got != "The folder source is read only. Pages are made in collections of the database. | kept" {
		t.Errorf("New in the head makes nothing in the folder source: %q", got)
	}
	b.eval(`window.made = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		const team = await post('/api/collections', {title: 'Team'});
		const page = await post('/api/pages', {title: 'Notes', parent: team.id});
		const b1 = await post('/api/pages/' + page.id + '/blocks', {md: 'First', after: ''});
		await post('/api/pages/' + page.id + '/blocks', {md: 'Second', after: b1.id});
		const other = await post('/api/pages', {title: 'Other', parent: team.id});
		await post('/api/pages/' + other.id + '/blocks', {md: 'x', after: ''});
		window.made = [page.id, other.id];
	})()`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the pages for the test were not made")
	}
	notes, other := b.eval(`window.made[0]`), b.eval(`window.made[1]`)
	b.open(base + "/o/" + notes)
	b.ready("Notes")
	if got := b.eval(status); got != "false Reading | Reading. Click to edit" {
		t.Errorf("a page of the database opens for reading: %q", got)
	}
	if got := b.eval(`(() => { const r = e => document.querySelector('[data-win="0"] ' + e).getBoundingClientRect(), s = r('[data-mode]'), p = r('.breadcrumb'), h = r('.workspace-main > header > h1');
		return [s.top <= p.top + 2, s.bottom >= h.bottom - 2, s.right <= Math.min(p.left, h.left), Math.abs(s.width - s.height) < 1].join(' '); })()`); got != "true true true true" {
		t.Errorf("the button is square, left of path and title, as tall as both: %q", got)
	}
	// The editor's bundle loads on demand (on the button's hover, or when the page is idle), so the editor comes a moment
	// after the click: the state is waited for, not read at once (it was read at once and lost the race under load).
	b.clickOn(`[data-win="0"] [data-mode]`, 1)
	editing := status + ` + ' ' + document.querySelectorAll('article.typeset .cm-editor').length + ' ' + document.querySelectorAll('[data-win="0"] .workspace-main > [data-edbar]').length`
	if !b.waitFor(`(` + editing + `) === "false Editing | Editing. Click to read 1 1"`) {
		t.Errorf("the button turns it to editing, one editor and one bar for the page: %q", b.eval(editing))
	}
	// What the editor and its bar do is tested in editor_test.go.
	b.clickOn(`[data-win="0"] [data-mode]`, 1)
	reading := status + ` + ' ' + document.querySelectorAll('.cm-editor').length`
	if !b.waitFor(`(` + reading + `) === "false Reading | Reading. Click to edit 0"`) {
		t.Errorf("and back to reading: %q", b.eval(reading))
	}
	// The setting opens documents for editing.
	b.clickOn(`[data-act="view"][data-id="profile"]`, 1)
	b.clickOn(`[data-act="sec"][data-id="docs"]`, 1)
	b.clickOn(`[data-act="openedit"][data-id="true"]`, 1)
	b.settingsSent()
	b.open(base + "/o/" + other)
	b.ready("Other")
	if got := b.eval(status); got != "false Editing | Editing. Click to read" {
		t.Errorf("with the setting, a page opens for editing: %q", got)
	}
}

// In reading, every block has a handle at its left; its menu copies the address, opens the block for editing, comments on
// it and says where it is used.
func TestTheHandleOfABlock(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.eval(`window.made = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		const team = await post('/api/collections', {title: 'Team'});
		const page = await post('/api/pages', {title: 'Plan', parent: team.id});
		const b1 = await post('/api/pages/' + page.id + '/blocks', {md: 'First', after: ''});
		await post('/api/pages/' + page.id + '/blocks', {md: 'Second', after: b1.id});
		const rack = await post('/api/pages', {title: 'Rack', parent: team.id});
		await post('/api/pages/' + rack.id + '/blocks', {md: '::embed{ref="' + page.id + '#b2"}', after: ''});
		await post('/api/objects/' + team.id + '/access', {access: 'comment'});
		window.made = page.id;
	})()`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the pages for the test were not made")
	}
	page := b.eval(`window.made`)
	b.open(base + "/o/" + page)
	b.ready("Plan")
	if got := b.eval(`document.querySelectorAll('article.typeset [data-block] > .editor-handle > button').length`); got != "2" {
		t.Fatalf("each block has a handle: %s", got)
	}
	b.clickOn(`article.typeset [data-block="b2"] > .editor-handle > button`, 1)
	if got := b.eval(`[...document.querySelectorAll('.editor-handle [role=menu] [role=menuitem]')].map(x => x.textContent.trim() + (x.disabled ? ' (off)' : '')).join(', ') + ' | ' + document.querySelector('.editor-handle [role=menu] > [role=heading]')?.textContent`); got != "Copy address#b2, Edit this block, Move to page, Comment | Used in Rack" {
		t.Errorf("the menu of b2: %q", got)
	}
	b.clickOn(`.editor-handle [role=menu] [data-act="blkedit"]`, 1)
	if !b.waitFor(editorAt("b2")) {
		t.Errorf("Edit this block opens the editor at b2: %s", b.eval(`document.activeElement?.outerHTML.slice(0, 100)`))
	}
	b.clickOn(`[data-win="0"] [data-mode]`, 1)
	b.clickOn(`article.typeset [data-block="b1"] > .editor-handle > button`, 1)
	b.shot("block-handle")
	b.clickOn(`.workspace-main > header > h1`, 1)
	if got := b.eval(`!!document.querySelector('.editor-handle [role=menu]')`); got != "false" {
		t.Errorf("a click elsewhere closes the menu")
	}
	// Someone who may only comment: no editing, but comments.
	b.call("Network.setExtraHTTPHeaders", map[string]any{"headers": map[string]string{"Remote-User": "bob", "Remote-Name": "Bob Kern", "Remote-Email": "bob@example.com", "Remote-Groups": "ops"}}, nil)
	b.open(base + "/o/" + page)
	b.ready("Plan")
	b.clickOn(`article.typeset [data-block="b1"] > .editor-handle > button`, 1)
	if got := b.eval(`[...document.querySelectorAll('.editor-handle [role=menu] [role=menuitem]')].map(x => x.textContent.trim() + (x.disabled ? ' (off)' : '')).join(', ')`); got != "Copy address#b1, Edit this block (off), Comment" {
		t.Errorf("the menu for someone who may comment: %q", got)
	}
	b.clickOn(`.editor-handle [role=menu] [data-act="bcmt"]`, 1)
	if !b.waitFor(`document.activeElement?.dataset?.addcmt === '0' && document.querySelector('[data-cfor] small')?.textContent === '#b1'`) {
		t.Errorf("Comment opens the comments of b1, ready to write: %s", b.eval(`document.querySelector('[data-cfor]')?.textContent`))
	}
}

// The right click on the free part of nav2 opens the list of problems: what does not resolve in the pages one sees, with a
// way to the page, and what the folder source ran into.
func TestTheListOfProblemsInTheWorkspace(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.eval(`window.made = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		const team = await post('/api/collections', {title: 'Team'});
		const page = await post('/api/pages', {title: 'Rack', parent: team.id});
		await post('/api/pages/' + page.id + '/blocks', {md: '::embed{ref="zzzzzzzzzz"}', after: ''});
		window.made = page.id;
	})()`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the page for the test was not made")
	}
	b.open(base + "/")
	b.ready("")
	n2 := b.rect("[aria-label=Nav2]")
	h := b.eval(`String(document.querySelector('[aria-label=Nav2]').getBoundingClientRect().bottom)`)
	y, _ := strconv.ParseFloat(h, 64)
	for _, kind := range []string{"mousePressed", "mouseReleased"} {
		b.call("Input.dispatchMouseEvent", map[string]any{"type": kind, "x": n2.X, "y": y - 6, "button": "right", "buttons": 2, "clickCount": 1}, nil)
	}
	if !b.waitFor(`!!document.querySelector('[data-cm-panel]')`) {
		t.Fatal("no menu on the free part of nav2")
	}
	b.clickOn(probe(b, `[data-cm-panel] button`, "Problems"), 1)
	if !b.waitFor(`document.querySelectorAll('tr[data-problem]').length === 1`) {
		t.Fatalf("the problem of Rack: %s", b.eval(`document.querySelector('article.typeset')?.innerText.slice(0, 300)`))
	}
	if got := b.eval(`[...document.querySelector('tr[data-problem]').cells].map(c => c.textContent.trim()).join(' / ') + ' | ' + document.querySelector('.workspace-main > header > h1').textContent + ' | ' + [...document.querySelectorAll('article.typeset table')].length + ':' + (document.querySelector('article.typeset h2:last-of-type')?.textContent || '')`); got != "Rack / #b1 / zzzzzzzzzz / embeds something that is not to be found | Problems | 2:Folder source" {
		t.Errorf("the list: %q", got)
	}
	b.shot("problems")
	b.clickOn(`tr[data-problem] [data-act="open"]`, 1)
	b.ready("Rack")
}

// Tags of a page of the database are added and taken off in the gear, kept by the server, shown in the head and found by
// the search filter.
func TestTagsOfAPage(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.eval(`window.made = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		const team = await post('/api/collections', {title: 'Team'});
		const page = await post('/api/pages', {title: 'Rack', parent: team.id});
		const other = await post('/api/pages', {title: 'Other', parent: team.id});
		await fetch('/api/objects/' + other.id + '/tags', {method: 'PUT', headers: json, body: JSON.stringify({tags: ['rack']})});
		await post('/api/objects/' + team.id + '/access', {access: 'read'});
		window.made = page.id;
	})()`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the page for the test was not made")
	}
	page := b.eval(`window.made`)
	b.open(base + "/o/" + page)
	b.ready("Rack")
	b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
	b.clickOn(`aside[data-drawer] [data-addtag]`, 1)
	b.typeText("Network")
	b.press("Enter", "Enter", 13, 0)
	if !b.waitFor(`[...document.querySelectorAll('aside[data-drawer] [data-act="tagsearch"]')].map(x => x.textContent).join(' ') === '#network'`) {
		t.Fatalf("the tag is added: %s", b.eval(`document.querySelector('aside[data-drawer]')?.innerText`))
	}
	b.clickOn(`aside[data-drawer] [data-addtag]`, 1)
	b.typeText("rack")
	b.press("Enter", "Enter", 13, 0)
	b.waitFor(`document.querySelectorAll('aside[data-drawer] [data-act="tagsearch"]').length === 2`)
	b.open(base + "/o/" + page)
	b.ready("Rack")
	if got := b.eval(`[...document.querySelectorAll('[data-win="0"] .workspace-main > header .badge[data-act="tagsearch"]')].map(x => x.textContent).join(' ')`); got != "#network #rack" {
		t.Errorf("the server keeps them, the head shows them: %q", got)
	}
	b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
	b.clickOn(`aside[data-drawer] [data-act="tagdel"][data-tag="network"]`, 1)
	if !b.waitFor(`[...document.querySelectorAll('aside[data-drawer] [data-act="tagsearch"]')].map(x => x.textContent).join(' ') === '#rack'`) {
		t.Errorf("the x takes it off: %s", b.eval(`document.querySelector('aside[data-drawer] [data-tags]')?.innerText`))
	}
	b.clickOn(`aside[data-drawer] [data-act="tagsearch"][data-tag="rack"]`, 1)
	if !b.waitFor(`['Rack', 'Other'].every(n => [...document.querySelectorAll('[aria-label=Nav3] [data-id]')].some(x => D[x.dataset.id]?.n === n))`) {
		t.Errorf("the tag finds the page, and one never opened: %s", b.eval(`document.querySelector('[aria-label=Nav3]')?.innerText.slice(0, 200)`))
	}
	// Someone who may only read sees the tags, but changes none.
	b.call("Network.setExtraHTTPHeaders", map[string]any{"headers": map[string]string{"Remote-User": "bob", "Remote-Name": "Bob Kern", "Remote-Email": "bob@example.com", "Remote-Groups": "ops"}}, nil)
	b.open(base + "/o/" + page)
	b.ready("Rack")
	b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
	if got := b.eval(`[document.querySelectorAll('aside[data-drawer] [data-act="tagsearch"]').length, !!document.querySelector('aside[data-drawer] [data-addtag]'), !!document.querySelector('aside[data-drawer] [data-act="tagdel"]'), document.querySelector('aside[data-drawer] [data-tags] + p')?.textContent].join(' | ')`); got != "1 | false | false | Only who may change this adds tags." {
		t.Errorf("for a reader: %q", got)
	}
}

// Deleting something that is embedded elsewhere names those pages in the question, before the second click deletes.
func TestDeletingSomethingInUse(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.eval(`window.made = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		const team = await post('/api/collections', {title: 'Team'});
		const plan = await post('/api/pages', {title: 'Plan', parent: team.id});
		await post('/api/pages/' + plan.id + '/blocks', {md: 'one', after: ''});
		const rack = await post('/api/pages', {title: 'Rack', parent: team.id});
		await post('/api/pages/' + rack.id + '/blocks', {md: '::embed{ref="' + plan.id + '#b1"}', after: ''});
		window.made = team.id;
	})()`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the pages for the test were not made")
	}
	b.open(base + "/o/" + b.eval(`window.made`))
	b.ready("Team")
	b.rightClickOn(rowNamed(b, "Plan"))
	b.clickOn(`[data-cm-panel] [data-act="delobj"]`, 1)
	if !b.waitFor(`document.querySelector('[data-cm-panel] [data-uses]')?.textContent === 'Embedded in Rack. You choose what happens there.'`) {
		t.Fatalf("the question names Rack: %s", b.eval(`document.querySelector('[data-cm-panel]')?.innerText`))
	}
	b.press("Escape", "Escape", 27, 0)
	b.rightClickOn(rowNamed(b, "Rack"))
	b.clickOn(`[data-cm-panel] [data-act="delobj"]`, 1)
	b.eval(`new Promise(r => setTimeout(r, 300))`)
	if got := b.eval(`document.querySelector('[data-cm-panel] [data-act="delobj"]')?.textContent + ' | ' + !!document.querySelector('[data-cm-panel] [data-uses]')`); got != "Delete? Yes | false" {
		t.Errorf("what is used nowhere just asks: %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
	// Deleting Plan goes through what of it is used, with nothing chosen in advance (ADR-22): a copy in Rack, then the
	// trash.
	b.rightClickOn(rowNamed(b, "Plan"))
	b.clickOn(`[data-cm-panel] [data-act="delobj"]`, 1)
	b.waitFor(`!!document.querySelector('[data-cm-panel] [data-uses]')`)
	b.clickOn(`[data-cm-panel] [data-act="delobj"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-ways] [data-wayitem]')`) {
		t.Fatalf("deleting a page in use asks for the ways")
	}
	if got := b.eval(`document.querySelector('[data-ways] [data-wayitem]').innerText.replace(/\s+/g, ' ')`); !strings.Contains(got, "Block #b1 in Plan") || !strings.Contains(got, "Used in Rack") {
		t.Errorf("the dialog: %q", got)
	}
	b.clickOn(`[data-ways] [data-act="way"][data-way="copies"]`, 1)
	b.clickOn(`[data-ways] [data-act="waysgo"]`, 1)
	if !b.waitFor(`/Plan is in the trash/.test(document.body.innerText)`) {
		t.Errorf("Plan went into the trash: %s", b.eval(`document.querySelector('.toast')?.textContent`))
	}
	b.eval(`window.got = null; fetch('/api/doc/' + D[Object.keys(D).find(k => plain(D[k].n) === 'Rack')].sid).then(r => r.json()).then(d => { window.got = d.blocks[0].md; })`)
	if !b.waitFor(`window.got === 'one'`) {
		t.Errorf("Rack has a copy: %q", b.eval(`window.got`))
	}
}
