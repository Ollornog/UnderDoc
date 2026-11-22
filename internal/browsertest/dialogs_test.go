//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import "testing"

// c22Spy records every change c22.js makes to an element (an attribute, hidden, a custom property), told apart by the
// script in the stack: where the element is (inside the workspace, elsewhere on the page, already detached by a redraw of
// the app) and what changed.
const c22Spy = `(() => {
	if (window.c22Touch) { window.c22Touch.length = 0; return 'reset'; }
	window.c22Touch = [];
	const fromC22 = () => /\/assets\/c22\/js\/c22\.js/.test(new Error().stack || '');
	const where = (n) => !n || !n.closest ? 'style' : n.closest('.workspace') ? (n.isConnected ? 'workspace' : 'detached') : n.isConnected ? 'page' : 'detached';
	for (const name of ['setAttribute', 'removeAttribute', 'toggleAttribute']) {
		const was = Element.prototype[name];
		Element.prototype[name] = function (...a) { if (fromC22()) window.c22Touch.push(where(this) + ' ' + name + ' ' + a[0]); return was.apply(this, a); };
	}
	const prop = CSSStyleDeclaration.prototype.setProperty;
	CSSStyleDeclaration.prototype.setProperty = function (...a) { if (fromC22()) window.c22Touch.push('style setProperty ' + a[0]); return prop.apply(this, a); };
	const hidden = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'hidden');
	Object.defineProperty(HTMLElement.prototype, 'hidden', { configurable: true, get: hidden.get,
		set(v) { if (fromC22()) window.c22Touch.push(where(this) + ' hidden'); hidden.set.call(this, v); } });
	return 'on';
})()`

// TestC22JSLeavesTheWorkspaceToTheApp: the workspace loads c22.js for the building blocks that need it (the steps of a
// dialog), and .workspace[data-behaviour="app"] keeps it out of the workspace, which UnderDoc drives itself: dragging a
// grip, opening and closing a list, a click, a double click and a right click on a name, the current entry of nav1 — each
// is handled once, by the app, and c22.js changes nothing there. The same spy sees c22.js work in the dialog, so it is not
// blind. A filter bar UnderDoc draws cannot be toggled by c22.js either (T-12).
func TestC22JSLeavesTheWorkspaceToTheApp(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	if got := b.eval(`typeof window.C22?.init + ' ' + document.querySelector('#root > .workspace')?.dataset.behaviour`); got != "function app" {
		t.Fatalf("c22.js is loaded and the workspace is the app's: %q", got)
	}
	if got := b.eval(c22Spy); got != "on" {
		t.Fatal(got)
	}
	drag := func(selector string, dx float64) {
		r := b.rect(selector)
		b.mouse("mouseMoved", r.X, r.Y, 0, 0)
		b.mouse("mousePressed", r.X, r.Y, 1, 0)
		b.mouse("mouseMoved", r.X+dx/2, r.Y, 0, 0)
		b.mouse("mouseMoved", r.X+dx, r.Y, 0, 0)
		b.mouse("mouseReleased", r.X+dx, r.Y, 1, 0)
	}
	width3 := `Math.round(document.querySelector('#root > .workspace > .workspace-nav[data-level="3"]').getBoundingClientRect().width)`
	before := b.eval(width3)
	drag(`#root > .workspace > .workspace-nav[data-level="3"] > [data-grip]`, 60)
	if got := b.eval(width3); got == before {
		t.Errorf("the grip of nav3 moved nothing: %s", got)
	}
	step1 := `document.querySelector('#root > .workspace > .workspace-nav[data-level="1"]').dataset.step`
	was := b.eval(step1)
	drag(`#root > .workspace > .workspace-nav[data-level="1"] > [data-grip]`, 120)
	if got := b.eval(step1); got == was {
		t.Errorf("the grip of nav1 took no step: %s", got)
	}
	// A list opened and closed by its arrow: once per click.
	arrow := `[aria-label=Nav3] li > div > button[data-toggle]`
	open := b.eval(`document.querySelector('` + arrow + `').getAttribute('aria-expanded')`)
	b.clickOn(arrow, 1)
	if !b.waitFor(`document.querySelector('` + arrow + `').getAttribute('aria-expanded') !== '` + open + `'`) {
		t.Errorf("the arrow did not toggle its list")
	}
	b.clickOn(arrow, 1)
	if !b.waitFor(`document.querySelector('` + arrow + `').getAttribute('aria-expanded') === '` + open + `'`) {
		t.Errorf("the arrow did not toggle its list back")
	}
	name := probe(b, `[aria-label=Nav3] li > div > button:not([data-toggle])`, "Network plan")
	b.clickOn(name, 1)
	b.clickOn(probe(b, `[aria-label=Nav3] li > div > button:not([data-toggle])`, "Network plan"), 2)
	b.rightClickOn(probe(b, `[aria-label=Nav3] li > div > button:not([data-toggle])`, "Network plan"))
	b.press("Escape", "Escape", 27, 0)
	current := `[aria-label=Nav1] .workspace-item[aria-current="page"]`
	b.clickOn(current, 1)
	if !b.waitFor(`document.querySelector('#root > .workspace').hasAttribute('data-nav-hidden')`) {
		t.Errorf("the current entry of nav1 did not hide nav2 and nav3")
	}
	b.clickOn(current, 1)
	if !b.waitFor(`!document.querySelector('#root > .workspace').hasAttribute('data-nav-hidden')`) {
		t.Errorf("the current entry of nav1 did not show nav2 and nav3 again")
	}
	if got := b.eval(`JSON.stringify(window.c22Touch)`); got != "[]" {
		t.Errorf("c22.js changed the workspace UnderDoc drives: %s", got)
	}

	// The spy is not blind: in the dialog of a new collection c22.js moves the steps.
	b.clickOn(`[aria-label=Nav2] [data-act="addmenu"][data-id="n2"]`, 1)
	b.clickOn(probe(b, `[data-menu="new"] [role="menuitem"]`, "Collection"), 1)
	if !b.waitFor(`document.getElementById('udrights')?.open`) {
		t.Fatal("the dialog of a new collection did not open")
	}
	b.clickOn(`#udrights [data-step-next]`, 1)
	if got := b.eval(`window.c22Touch.some(x => x === 'page setAttribute aria-current') + ' ' + window.c22Touch.every(x => x.startsWith('page '))`); got != "true true" {
		t.Errorf("c22.js steps the dialog, outside the workspace: %s", b.eval(`JSON.stringify(window.c22Touch)`))
	}
	b.press("Escape", "Escape", 27, 0)
}

// TestTheFilterBarOfASavedSearchIsNotToggled: the values of a saved search are shown pressed and cannot be changed
// before M-8; c22.js, which toggles the buttons of a filter bar, leaves them as they are.
func TestTheFilterBarOfASavedSearchIsNotToggled(t *testing.T) {
	b, base := setup(t)
	b.open(base + "/dev/ui")
	b.eval(`openDoc('ss1'); render()`)
	button := `.workspace-main .filter-bar button`
	if !b.waitFor(`!!document.querySelector('` + button + `')`) {
		t.Fatal("no filter bar")
	}
	b.clickOn(button, 1)
	if got := b.eval(`[...document.querySelectorAll('` + button + `')].every(n => n.getAttribute('aria-pressed') === 'true' && n.disabled)`); got != "true" {
		t.Errorf("a value of the saved search changed: %s", b.eval(`document.querySelector('.filter-bar').outerHTML.slice(0, 300)`))
	}
}

// TestBasecoatTakesOnlyTheCombobox: the workspace loads Basecoat's core and its combobox alone (C22's recipe for an app
// that draws its own menus), so its observer takes over the grouped combobox of the rights dialogs and nothing else: the
// menus, popovers and context menus UnderDoc draws open and closed itself are never Basecoat's (T-12).
func TestBasecoatTakesOnlyTheCombobox(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	taken := `[...document.querySelectorAll('[data-basecoat-component]')].map(n => n.dataset.basecoatComponent).sort().join(' ')`
	// The menu More of the window, a .dropdown-menu, drawn open by the app; a context menu on a name.
	b.clickOn(`[data-win="0"] [data-act="menu"]`, 1)
	more := `document.querySelector('[data-win="0"] [data-act="menu"]')?.closest('.dropdown-menu')`
	if !b.waitFor(more + `?.querySelector('[data-popover]')?.getAttribute('aria-hidden') === 'false'`) {
		t.Fatalf("the menu More did not open: %s", b.eval(`String(`+more+`?.outerHTML).slice(0, 300)`))
	}
	if got := b.eval(`typeof window.basecoat?.register + ' ' + (document.querySelectorAll('.dropdown-menu').length > 0)`); got != "function true" {
		t.Errorf("Basecoat is loaded and the page has menus of the app: %q", got)
	}
	if got := b.eval(taken); got != "" {
		t.Errorf("Basecoat took over a part the app draws: %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
	b.rightClickOn(probe(b, `[aria-label=Nav3] li > div > button:not([data-toggle])`, "Network plan"))
	if !b.waitFor(`!!document.querySelector('[data-cm-panel]')`) {
		t.Fatal("no context menu")
	}
	if got := b.eval(taken); got != "" {
		t.Errorf("Basecoat took over the context menu: %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
	// The combobox of the dialog of a new collection is Basecoat's, and only it.
	b.clickOn(`[aria-label=Nav2] [data-act="addmenu"][data-id="n2"]`, 1)
	b.clickOn(probe(b, `[data-menu="new"] [role="menuitem"]`, "Collection"), 1)
	if !b.waitFor(`document.getElementById('udrights')?.open && document.querySelector('#udrights .combobox')?.dataset.comboboxInitialized === 'true'`) {
		t.Fatalf("the combobox of the dialog was not taken over: %s", b.eval(`String(document.querySelector('#udrights .combobox')?.outerHTML).slice(0, 300)`))
	}
	if got := b.eval(taken); got != "combobox" {
		t.Errorf("Basecoat takes over the combobox alone: %q", got)
	}
	b.press("Escape", "Escape", 27, 0)
}
