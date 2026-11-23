//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"strings"
	"testing"
)

// asAgent sets the headers of the gate for an agent of anna: she signed in with OAuth, not in a session (ADR-36).
func (b *browser) asAgent() {
	b.t.Helper()
	b.call("Network.setExtraHTTPHeaders", map[string]any{"headers": map[string]string{"Remote-Auth": "oauth", "Remote-User": "anna", "Remote-Id": "1007", "Remote-Name": "Anna Berg", "Remote-Email": "anna@example.com", "Remote-Groups": "ops"}}, nil)
}

// agentWrites changes a block as anna's agent, through the API, and signs anna in again.
func (b *browser) agentWrites(page, block, md string) {
	b.t.Helper()
	b.asAgent()
	b.eval(`window.agentDone = null; fetch('/api/pages/` + page + `/blocks/` + block + `', {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({md: ` + jsQuote(md) + `})}).then(r => { window.agentDone = r.status; })`)
	if !b.waitFor(`window.agentDone === 200`) {
		b.t.Fatalf("the agent did not write: %s", b.eval(`String(window.agentDone)`))
	}
	b.signIn(true)
}

func jsQuote(s string) string {
	q, _ := jsonList([]string{s})
	return strings.TrimSuffix(strings.TrimPrefix(q, "["), "]")
}

// A block an agent changed carries "written by AI, not checked" for readers; a person opens the change as a diff and
// checks it, and the page says who checked it; a person's own edit of a block ends its hint (ADR-33, T-20).
func TestAIChangesAreCheckedOneByOne(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	page := makePage(b, "Restore", "This guide brings files back.", "1. Open the backup.")
	b.agentWrites(page, "b2", "1. Open the backup of last night.")

	// An agent in the browser sees the hint, but has nothing to check with.
	b.asAgent()
	b.open(base + "/o/" + page)
	b.ready("Restore")
	if !b.exists(`[data-ai-hint="b2"]`) || b.eval(`document.querySelectorAll('[data-ud-review]').length`) != "0" {
		t.Errorf("an agent sees the hint without Review: %s", b.eval(`document.querySelector('[data-ai-hint]')?.outerHTML`))
	}
	b.signIn(true)
	b.open(base + "/o/" + page)
	b.ready("Restore")
	b.watch()
	if got := b.eval(`[...document.querySelectorAll('[data-ai-hint]')].map(x => x.dataset.aiHint + ':' + x.querySelector('.badge').textContent).join(',')`); got != "b2:✦ Written by AI, not checked" {
		t.Fatalf("the hint stands on the block the agent changed, and only there: %q", got)
	}
	b.clickOn(`[data-ai-hint="b2"] [data-ud-review]`, 1)
	if !b.waitFor(`!!document.querySelector('#udreview[open] [data-diff="came"]')`) {
		t.Fatalf("the diff did not open: %s", b.eval(`document.getElementById('udreview')?.innerText`))
	}
	if got := b.eval(`[...document.querySelectorAll('#udreview [data-diff]')].map(x => x.dataset.diff + ' ' + x.textContent).join(' | ')`); got != "went − 1. Open the backup. | came + 1. Open the backup of last night." {
		t.Errorf("the diff shows what went and what came: %q", got)
	}
	b.shot("ai-review")
	b.clickOn(`#udreview [data-review-act="check"]`, 1)
	if !b.waitFor(`!document.querySelector('[data-ai-hint]') && /^Checked by Anna Berg on /.test(document.querySelector('[data-checked]')?.textContent || '')`) {
		t.Fatalf("after the check the hint is gone and the page says who checked it: %s | %s", b.eval(`document.querySelector('[data-kind-head]')?.textContent`), b.eval(`window.writes.join(', ')`))
	}
	if !strings.Contains(b.eval(`window.writes.join(', ')`), "/blocks/b2/check") {
		t.Errorf("the check went to the server: %s", b.eval(`window.writes.join(', ')`))
	}

	// The agent changes it again, and anna edits the block herself: its hint goes.
	b.agentWrites(page, "b2", "1. Open the backup of tonight.")
	b.openEditing(base, page, "Restore")
	b.lineWith("1. Open the backup of tonight.")
	b.typeText(" Then wait.")
	if !b.waitFor(`window.written.some(w => w.endsWith('/blocks/b2'))`) {
		t.Fatalf("the edit was not saved: %s", b.eval(`window.writes.join(', ')`))
	}
	b.clickOn(`[data-win="0"] [data-mode]`, 1)
	if !b.waitFor(`!document.querySelector('.cm-editor') && !!document.querySelector('[data-block="b2"]')`) {
		t.Fatal("the page did not turn to reading")
	}
	if b.exists(`[data-ai-hint]`) {
		t.Errorf("anna's own edit ends the hint")
	}
	b.open(base + "/o/" + page)
	b.ready("Restore")
	if got := b.eval(`document.querySelectorAll('[data-ai-hint]').length`); got != "0" {
		t.Errorf("no hint after a reload either: %s", got)
	}
	b.clean("ai-review")
}

// The results page filters for "AI changes to check": only the pages with hints nobody checked (ADR-33).
func TestSearchForAIChangesToCheck(t *testing.T) {
	b, base := setupWithSearch(t)
	b.open(base + "/")
	b.ready("")
	makePage(b, "Kasse schließen", "Die Kasse wird abends geschlossen.")
	open := makePage(b, "Kasse öffnen", "Die Kasse wird morgens geöffnet.")
	b.agentWrites(open, "b1", "Die Kasse wird morgens um acht geöffnet.")
	b.indexed("acht")
	b.open(base + "/")
	b.ready("")
	b.press("/", "Slash", 191, 0)
	b.typeText("Kasse")
	if !b.waitFor(`document.querySelectorAll('#sres [data-sact="go"]').length === 2`) {
		t.Fatalf("both pages are hits: %s", b.eval(`document.getElementById('sres')?.innerText`))
	}
	b.clickOn(`[aria-label=Nav3] [data-act="filters"]`, 1)
	b.clickOn(`[aria-label=Nav3] [data-sact="aicheck"]`, 1)
	if !b.waitFor(`document.querySelectorAll('#sres [data-sact="go"]').length === 1 && document.querySelector('#sres [data-sact="go"]').dataset.sid === '` + open + `'`) {
		t.Fatalf("the filter keeps the page with the hint: %s", b.eval(`document.getElementById('sres')?.innerText`))
	}
	if got := b.eval(`document.querySelector('[aria-label=Nav3] [data-sact="aicheck"]').getAttribute('aria-pressed')`); got != "true" {
		t.Errorf("the filter shows that it is on: %s", got)
	}
}
