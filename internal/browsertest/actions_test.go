//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"testing"
)

// TestActionsInTheRightsDialog: the rights dialog shows presets first and the single actions on request (ADR-36). Anna
// grants bob a preset, opens his actions, denies him write and saves: his row is custom, with the denied action named,
// and the server keeps the entries. No agents denies read to Agents there. The access check names the deciding entry,
// also for an agent; the tab Rights lets each person narrow their own agents.
func TestActionsInTheRightsDialog(t *testing.T) {
	b, base := setupAsAdmin(t)
	b.signInWith("bob", "Bob Kern", "")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.signIn(true)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	b.clickOn(rowNamed(b, "A")+` [data-act="open"]`, 1)
	b.ready("A")
	page := b.eval(`location.pathname.split('/').pop()`)
	b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
	b.clickOn(`[data-act="rights"]`, 1)
	if !b.waitFor(`document.getElementById('udrights')?.open && !!document.querySelector('#udrights [data-rform="grant"]')`) {
		t.Fatalf("the dialog opens: %s", b.eval(`document.getElementById('udrights')?.innerText`))
	}
	b.submit(`#udrights [data-rform="grant"]`, map[string]string{"who": "person:bob", "role": "edit"})
	if !b.waitFor(`!!document.querySelector('#udrights [data-grant="person:bob"] [data-ract="actions"]')`) {
		t.Fatalf("bob is granted, with Actions: %s", b.eval(`document.getElementById('udrights').innerText`))
	}
	b.clickOn(`#udrights [data-grant="person:bob"] [data-ract="actions"]`, 1)
	form := `#udrights [data-actions-of="person:bob"] form[data-rform="actions"]`
	if !b.waitFor(`!!document.querySelector('` + form + `')`) {
		t.Fatalf("the actions open: %s", b.eval(`document.getElementById('udrights').innerText`))
	}
	if got := b.eval(`[...document.querySelectorAll('` + form + ` select[data-action]')].map(s => s.dataset.action + '=' + s.value).join(' ')`); got != "read=allow comment=allow create=allow write=allow delete=allow share= manage=" {
		t.Errorf("the preset edit as actions: %q", got)
	}
	b.submit(form, map[string]string{"write": "deny"})
	if !b.waitFor(`(document.querySelector('#udrights [data-grant="person:bob"] [data-summary]')?.textContent || '').includes('❌ Write')`) {
		t.Fatalf("bob's row names the deny: %s", b.eval(`document.querySelector('#udrights [data-grants]')?.innerText`))
	}
	if got := b.eval(`document.querySelector('#udrights [data-grant="person:bob"] select[data-rsel]').value`); got != "custom" {
		t.Errorf("a changed preset shows as custom: %q", got)
	}
	b.eval(`window.r = null; fetch('/api/objects/` + page + `/rights').then(r => r.json()).then(j => { window.r = JSON.stringify(j.own.filter(g => g.who === 'bob').map(g => g.role + ':' + g.actions.write)); }); ''`)
	if !b.waitFor(`window.r !== null`) || b.eval(`window.r`) != `["custom:deny"]` {
		t.Errorf("the server keeps the deny: %s", b.eval(`String(window.r)`))
	}
	b.shot("rights-actions")
	// No agents here.
	b.clickOn(`#udrights [data-ract="noagents"]`, 1)
	if !b.waitFor(`(document.querySelector('#udrights [data-agents]')?.innerText || '').includes('No agents here') && document.querySelector('#udrights [data-ract="noagents"]')?.textContent.includes('Let agents in')`) {
		t.Errorf("no agents: %s", b.eval(`document.querySelector('#udrights [data-agents]')?.innerText`))
	}
	b.clickOn(`#udrights [data-ract="close"]`, 1)
	// The access check, for bob and for an agent of anna.
	b.clickOn(`[data-act="view"][data-id="rights"]`, 1)
	b.clickOn(`[data-act="sec"][data-id="check"]`, 1)
	b.submit(`[data-rights="check"] [data-rform="who"]`, map[string]string{"object": "/o/" + page, "account": "bob"})
	if !b.waitFor(`(document.querySelector('[data-decisions] tr[data-decision="write"]')?.innerText || '').includes('❌ denied')`) {
		t.Fatalf("bob may not write: %s", b.eval(`document.querySelector('[aria-label=Nav3] aside.card')?.innerText`))
	}
	if got := b.eval(`document.querySelector('[data-decisions] tr[data-decision="write"]').innerText.replace(/\s+/g, ' ').trim()`); got != "Write ❌ denied deny for Bob Kern · on A" {
		t.Errorf("the deciding entry and where it is: %q", got)
	}
	b.eval(`(() => { const f = document.querySelector('[data-rights="check"] [data-rform="who"]'); f.elements.agent.checked = true; f.elements.account.value = 'anna'; })()`)
	b.submit(`[data-rights="check"] [data-rform="who"]`, map[string]string{"object": "/o/" + page})
	if !b.waitFor(`(document.querySelector('[data-check]')?.innerText || '').includes('As an agent') && (document.querySelector('[data-decisions] tr[data-decision="read"]')?.innerText || '').includes('denied to agents')`) {
		t.Errorf("anna's agent may not read where no agents are: %s", b.eval(`document.querySelector('[aria-label=Nav3] aside.card')?.innerText`))
	}
	b.shot("rights-check-actions")
	// My agents: anna lets hers only read.
	b.clickOn(`[data-act="sec"][data-id="agents"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-rights="agents"] [data-rform="agents"]')`) {
		t.Fatalf("my agents: %s", b.eval(`document.querySelector('[aria-label=Nav3] aside.card')?.innerText`))
	}
	b.eval(`document.querySelectorAll('[data-rform="agents"] input[name=act]').forEach(x => { x.checked = x.value === 'read'; })`)
	b.submit(`[data-rights="agents"] [data-rform="agents"]`, map[string]string{})
	// The form is sent a moment after the click, and a loaded host answers late: ask until the server keeps it.
	b.eval(`window.l = null; (async () => { for (let i = 0; i < 100; i++) { window.l = (await (await fetch('/api/me/agents')).json()).actions.join(','); if (window.l === 'read') break; await new Promise(r => setTimeout(r, 100)); } })(); ''`)
	if !b.waitFor(`window.l === 'read'`) {
		t.Errorf("anna's agents only read: %s", b.eval(`String(window.l)`))
	}
}

// TestNestedGroupsInTheTabRights: an administrator makes a group inside a group, puts a group from the sign-in inside it
// and makes bob its group admin; the list says where each group is (ADR-36).
func TestNestedGroupsInTheTabRights(t *testing.T) {
	b, base := setupAsAdmin(t)
	b.signInWith("bob", "Bob Kern", "")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.signIn(true)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.clickOn(`[data-act="view"][data-id="rights"]`, 1)
	b.submit(`[data-rights="groups"] form[data-rform="group"]:not([data-parent])`, map[string]string{"name": "Company", "description": ""})
	if !b.waitFor(`!!document.querySelector('[data-rights="groups"] [data-group="Company"]')`) {
		t.Fatalf("Company is made: %s", b.eval(`document.querySelector('[aria-label=Nav3] aside.card')?.innerText`))
	}
	b.clickOn(`[data-group="Company"] [data-ract="members"]`, 1)
	inside := `[data-group-detail="Company"] form[data-rform="group"][data-parent]`
	if !b.waitFor(`!!document.querySelector('` + inside + `')`) {
		t.Fatalf("Company opens with its forms: %s", b.eval(`document.querySelector('[aria-label=Nav3] aside.card')?.innerText`))
	}
	b.submit(inside, map[string]string{"name": "Sales"})
	if !b.waitFor(`(document.querySelector('[data-rights="groups"] [data-group="Sales"]')?.innerText || '').includes('inside Company') && !!document.querySelector('[data-group-detail="Company"] [data-subgroup="Sales"]')`) {
		t.Fatalf("Sales is inside Company: %s", b.eval(`document.querySelector('[aria-label=Nav3] aside.card')?.innerText`))
	}
	b.eval(`(() => { const s = document.querySelector('[data-group-detail="Company"] form[data-rform="subgroup"] select'); s.value = [...s.options].find(o => o.textContent.startsWith('ops')).value; })()`)
	b.submit(`[data-group-detail="Company"] form[data-rform="subgroup"]`, map[string]string{})
	if !b.waitFor(`!!document.querySelector('[data-group-detail="Company"] [data-subgroup="ops"]')`) {
		t.Errorf("ops from the sign-in is inside Company: %s", b.eval(`document.querySelector('[data-group-detail="Company"]')?.innerText`))
	}
	b.submit(`[data-group-detail="Company"] form[data-rform="groupadmin"]`, map[string]string{"account": "bob"})
	if !b.waitFor(`(document.querySelector('[data-group-detail="Company"] [data-admins]')?.innerText || '').includes('Bob Kern')`) {
		t.Errorf("bob is a group admin of Company: %s", b.eval(`document.querySelector('[data-group-detail="Company"]')?.innerText`))
	}
	b.shot("rights-groups-nested")
	// Bob, a group admin, sees Company and Sales and manages them.
	b.signInWith("bob", "Bob Kern", "")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.clickOn(`[data-act="view"][data-id="rights"]`, 1)
	if !b.waitFor(`(document.querySelector('[data-rights="groups"] [data-group="Sales"]')?.innerText || '').includes('you manage it')`) {
		t.Errorf("bob manages Sales through Company: %s", b.eval(`document.querySelector('[aria-label=Nav3] aside.card')?.innerText`))
	}
}
