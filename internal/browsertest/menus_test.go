//go:build browser

package browsertest

import "testing"

// The menus, the toast and the to-dos of the workspace are C22's building blocks (T-12 step 4): every menu and popup hangs
// inside the .workspace, where the pack underdoc gives it the mockup's size; a toast that stays through a render is the
// same node, so it does not come in again with every redraw; a to-do is ticked by its checkbox, in nav3 and in the window;
// and the general tab stays where it is while it is dragged.
func TestMenusAndViewsFromC22(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := rackPage(b)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	inside := func(selector string) string {
		return b.eval(`(() => { const n = document.querySelector('` + selector + `'); return n ? String(!!n.closest('#root > .workspace')) : 'none'; })()`)
	}

	// Window and tabs: C22's context menu, inside the workspace.
	b.clickOn(`[data-act=winmenu]`, 1)
	if got := inside(`.context-menu > [data-cm-panel]`); got != "true" {
		t.Errorf("the menu of the window is C22's context menu inside the workspace: %s", got)
	}
	b.press("Escape", "Escape", 27, 0)
	// New of the top row: the popover of its button.
	b.clickOn(`[data-act="newmenu"]`, 1)
	if got := b.eval(`String(!!document.querySelector('#root > .workspace > .workspace-top .popover > [data-popover][data-menu="quick"]'))`); got != "true" {
		t.Errorf("New of the top row is the popover of its button: %s", got)
	}
	b.press("Escape", "Escape", 27, 0)

	// A toast hangs in the workspace and stays the same node through a render.
	b.eval(`toast('Kept through a render.')`)
	if got := b.eval(`(() => { const t = document.querySelector('.toaster'); if (!t || !t.closest('#root > .workspace')) return 'outside'; render(); return String(document.querySelector('.toaster') === t); })()`); got != "true" {
		t.Errorf("the toast is the same node after a render: %s", got)
	}

	// The general tab stays while it is dragged; only a tab of its own leaves its place for the stand-in.
	b.clickOn(`[data-act=winmenu]`, 1)
	b.clickOn(probe(b, `[data-cm-panel] button`, "Keep as a tab of its own"), 1)
	b.eval(`openDoc(SID['` + page + `']); render()`)
	if !b.waitFor(`!!document.querySelector('[data-win="0"] .workspace-tab[data-general]')`) {
		t.Fatalf("the general tab holds Rack: %s", b.eval(`document.querySelector('[data-win="0"] .workspace-tabs')?.innerText`))
	}
	b.dragTo(`[data-win="0"] .workspace-tab[data-general]`, `[data-win="0"] .workspace-main > section`, 0.5, func() {
		if got := b.eval(`[!!document.getElementById('ghost'), getComputedStyle(document.querySelector('[data-win="0"] .workspace-tab[data-general]')).display !== 'none'].join(' ')`); got != "true true" {
			t.Errorf("while the general tab is dragged it stays, with its label at the pointer: %q", got)
		}
	})

	// To-dos: the checkbox of an entry in nav3 ticks it, and so does the one in the window.
	b.clickOn(`[aria-label=Nav1] [data-act="view"][data-id="todo"]`, 1)
	box := `[aria-label=Nav3] input[data-act="tick"][data-l="mine"][data-id="0"]`
	if !b.waitFor(`!!document.querySelector('` + box + `') && !document.querySelector('` + box + `').checked`) {
		t.Fatalf("the first to-do of My to-dos is open: %s", b.eval(`document.querySelector('[aria-label=Nav3]')?.innerText`))
	}
	b.clickOn(box, 1)
	if !b.waitFor(`TL[0].items[0].done === true && document.querySelector('` + box + `').checked`) {
		t.Errorf("the checkbox in nav3 ticks the to-do: %s", b.eval(`JSON.stringify(TL[0].items[0])`))
	}
	b.clickOn(`[aria-label=Nav3] [data-sechead="mine"] a`, 1)
	inWindow := `[data-sc="0"] input[data-act="tick"][data-l="mine"][data-id="0"]`
	if !b.waitFor(`document.querySelector('` + inWindow + `')?.checked === true`) {
		t.Fatalf("the list opens in the window with the to-do ticked: %s", b.eval(`document.querySelector('[data-sc="0"]')?.innerHTML.slice(0, 300)`))
	}
	b.clickOn(inWindow, 1)
	if !b.waitFor(`TL[0].items[0].done === false && !document.querySelector('` + box + `').checked`) {
		t.Errorf("the checkbox in the window ticks it off again: %s", b.eval(`JSON.stringify(TL[0].items[0])`))
	}
	b.shot("todo-ticked")
}
