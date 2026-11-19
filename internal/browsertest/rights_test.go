//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import "testing"

// signInWith sets the headers of the gate for another person with groups.
func (b *browser) signInWith(login, name, groups string) {
	b.t.Helper()
	b.call("Network.setExtraHTTPHeaders", map[string]any{"headers": map[string]string{"Remote-User": login, "Remote-Name": name, "Remote-Groups": groups}}, nil)
}

// TestTheRoleDecidesWhatIsGreyed: what the viewer may do comes from their role (ADR-29), not from the level for all
// others. Team is read only for the tenant, but the group ops edits it: carl, in ops, may add to it and sees the lock
// that tells what others may do; bob, in no group, may not add.
func TestTheRoleDecidesWhatIsGreyed(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.eval(`window.done = null; (async () => {
		const json = {'Content-Type': 'application/json'};
		await fetch('/api/objects/` + team + `/access', {method: 'POST', headers: json, body: JSON.stringify({access: 'read'})});
		const people = await (await fetch('/api/people')).json();
		const ops = people.groups.find(g => g.name === 'ops');
		const r = await fetch('/api/objects/` + team + `/grants', {method: 'PUT', headers: json, body: JSON.stringify({kind: 'group', who: ops.id, role: 'edit'})});
		window.done = r.status;
	})()`)
	if !b.waitFor(`window.done === 204`) {
		t.Fatalf("the group ops was not granted edit: %s", b.eval(`String(window.done)`))
	}
	newHere := `[document.querySelector('.cheadrow[data-cdrop="` + team + `"] .newplus') !== null, !!document.querySelector('.cheadrow[data-cdrop="` + team + `"] .acc')].join(' ')`
	b.signInWith("carl", "Carl", "ops")
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := b.eval(newHere); got != "true true" {
		t.Errorf("carl edits through ops, and the lock says what others may do: %q", got)
	}
	b.signInWith("bob", "Bob", "")
	b.open(base + "/o/" + team)
	b.ready("Team")
	if got := b.eval(newHere); got != "false true" {
		t.Errorf("bob only reads: %q", got)
	}
	b.signIn(true)
}
