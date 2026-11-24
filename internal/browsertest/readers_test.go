//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import "testing"

// The mark of agents' writes after reading what fewer may read is switched per space in its rights dialog (T-38).
func TestTheAgentMarkIsSwitchedPerSpace(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.signInWith("anna", "Anna Berg", "ops,underdoc-admin")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
	b.clickOn(`[data-act="rights"]`, 1)
	if !b.waitFor(`document.getElementById('udrights')?.open && !!document.querySelector('#udrights select[data-agentmark]')`) {
		t.Fatalf("the space offers no switch of the mark: %s", b.eval(`document.getElementById('udrights')?.innerText`))
	}
	b.eval(`(() => { const s = document.querySelector('#udrights select[data-agentmark]'); s.value = 'off'; s.dispatchEvent(new Event('change', {bubbles: true})); })()`)
	b.eval(`window.mark = null; fetch('/api/objects/` + team + `/rights').then(r => r.json()).then(a => { window.mark = a.agentMark && a.agentMark.set; })`)
	if !b.waitFor(`window.mark === 'off' || (fetch('/api/objects/` + team + `/rights').then(r => r.json()).then(a => { window.mark = a.agentMark && a.agentMark.set; }), false)`) {
		t.Errorf("the switch did not reach the server: %s", b.eval(`String(window.mark)`))
	}
	if !b.waitFor(`(document.querySelector('#udrights [data-agent-mark]')?.innerText || '').includes('❌')`) {
		t.Errorf("switched off, the dialog does not say so: %s", b.eval(`document.querySelector('#udrights [data-agent-mark]')?.innerText`))
	}
}
