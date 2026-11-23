//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/web"
)

// signInWith sets the headers of the gate for another person with groups.
func (b *browser) signInWith(login, name, groups string) {
	b.t.Helper()
	b.call("Network.setExtraHTTPHeaders", map[string]any{"headers": map[string]string{"Remote-Auth": "session", "Remote-User": login, "Remote-Name": name, "Remote-Groups": groups}}, nil)
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
	newHere := `[document.querySelector('section > header[data-cdrop="` + team + `"] .btn') !== null, !!document.querySelector('section > header[data-cdrop="` + team + `"] [data-lock]')].join(' ')`
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

// TestARefusalSaysWhy: when the server refuses a move or a deletion for a reason of the rights, the toast says that
// reason, not only that others may not change it (ADR-29). Anna's page A holds a page of bob: she may neither take A into
// her notes nor delete B, below which bob's page breaks the inheritance and shuts her out.
func TestARefusalSaysWhy(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	sid := func(title string) string {
		return b.eval(`(() => { const k = Object.keys(D).find(k => D[k].sid && D[k].n === ` + strconv.Quote(title) + `); return D[k].sid; })()`)
	}
	b.open(base + "/o/" + team)
	b.ready("Team")
	rowNamed(b, "B")
	pageA, pageB := sid("A"), sid("B")
	b.signInWith("bob", "Bob Kern", "")
	b.open(base + "/o/" + team)
	b.ready("Team")
	b.eval(`window.done = null; (async () => {
		const json = {'Content-Type': 'application/json'}, post = async (u, v) => (await fetch(u, {method: 'POST', headers: json, body: JSON.stringify(v)})).json();
		await post('/api/pages', {title: 'Bob in A', parent: '` + pageA + `'});
		const mine = await post('/api/pages', {title: 'Bob in B', parent: '` + pageB + `'});
		await fetch('/api/objects/' + mine.id + '/inherit', {method: 'PUT', headers: json, body: JSON.stringify({inherit: false})});
		// Broken, the page holds what held from above; bob takes all of it back, anna's role included.
		const rights = await (await fetch('/api/objects/' + mine.id + '/rights')).json();
		let r = {status: 204};
		for (const g of rights.own) if (r.status === 204) r = await fetch('/api/objects/' + mine.id + '/grants', {method: 'PUT', headers: json, body: JSON.stringify({kind: g.kind, who: g.who, role: ''})});
		window.done = rights.own.length > 0 ? r.status : 'nothing held';
	})()`)
	if !b.waitFor(`window.done === 204`) {
		t.Fatalf("bob's pages were not made: %s", b.eval(`String(window.done)`))
	}
	b.signIn(true)
	b.open(base + "/o/" + team)
	b.ready("Team")
	rowNamed(b, "A")
	b.eval(`window.notes = null; fetch('/api/state').then(r => r.json()).then(s => { const n = s.collections.find(c => c.private);
		D.refusalNotes = {sid: n.id, n: n.n}; window.notes = n.id; })`)
	if !b.waitFor(`!!window.notes`) {
		t.Fatal("no private notes")
	}
	key := func(title string) string {
		return b.eval(`Object.keys(D).find(k => D[k].sid && D[k].n === ` + strconv.Quote(title) + `)`)
	}
	b.eval(`moveObject(` + strconv.Quote(key("A")) + `, 'refusalNotes', '', () => {})`)
	if !b.waitFor(`(document.querySelector('.toast')?.textContent || '') === 'something in it belongs to someone else, and only what is all yours goes into something private'`) {
		t.Errorf("the toast of a refused move: %q", b.eval(`document.querySelector('.toast')?.textContent`))
	}
	b.eval(`deleteObject(` + strconv.Quote(key("B")) + `)`)
	if !b.waitFor(`(document.querySelector('.toast')?.textContent || '') === 'something below it is not open to you, so it cannot go with it; its owner moves it out first'`) {
		t.Errorf("the toast of a refused deletion: %q", b.eval(`document.querySelector('.toast')?.textContent`))
	}
}

// setupAsAdmin is setupWithDatabase where the group ops administers the tenant, as Anna's sign-in names it.
func setupAsAdmin(t *testing.T) (*browser, string) {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if err := database.SetSignIn(tenant.With(context.Background(), tenant.Main), db.SignIn{Methods: []string{db.MethodGate}}); err != nil {
		t.Fatal(err)
	}
	return setupWith(t, web.Options{DevUI: true, DB: database, AdminGroup: "ops"}, nil)
}

// submit fills a form of the rights by script, as a person would by hand, and sends it.
// pick chooses in a C22 combobox as a person does: a click into its field, a few letters typed, which leave only
// matching options visible (one of another group hides), then a click on the option. The hidden field takes its value.
func (b *browser) pick(combobox, typed, value string) {
	b.t.Helper()
	if !b.waitFor(`document.querySelector(` + strconv.Quote(combobox) + `)?.dataset.comboboxInitialized === 'true'`) {
		b.t.Fatalf("%s is not a combobox Basecoat took over: %s", combobox, b.eval(`String(document.querySelector(`+strconv.Quote(combobox)+`)?.outerHTML).slice(0, 300)`))
	}
	b.clickOn(combobox+` > input[role=combobox]`, 1)
	b.eval(`(() => { const i = document.querySelector(` + strconv.Quote(combobox+` > input[role=combobox]`) + `); i.select(); })()`)
	b.typeText(typed)
	option := combobox + ` [role=option][data-value=` + strconv.Quote(value) + `]`
	if !b.waitFor(`document.querySelector(` + strconv.Quote(combobox+` [data-popover]`) + `).getAttribute('aria-hidden') === 'false' && document.querySelector(` + strconv.Quote(option) + `)?.getAttribute('aria-hidden') === 'false' &&
		[...document.querySelectorAll(` + strconv.Quote(combobox+` [role=option]`) + `)].some(o => o.getAttribute('aria-hidden') === 'true')`) {
		b.t.Fatalf("typing %q into %s does not filter to %s: %s", typed, combobox, value, b.eval(`[...document.querySelectorAll(`+strconv.Quote(combobox+` [role=option]`)+`)].map(o => o.dataset.value + ':' + o.getAttribute('aria-hidden')).join(' ')`))
	}
	// Between two groups that show one line, none at the top or the bottom: C22 hides the lines of groups without a hit.
	lines := `[...document.querySelector(` + strconv.Quote(combobox+` [role=listbox]`) + `).children].filter(e => getComputedStyle(e).display !== 'none').map(e => e.getAttribute('role') === 'separator' ? '-' : 'x').join('')`
	if got := b.eval(lines); got == "" || got[0] == '-' || got[len(got)-1] == '-' || strings.Contains(got, "--") {
		b.t.Errorf("typing %q into %s leaves lines without a group between them (x a group or an option, - a line): %q", typed, combobox, got)
	}
	// The list fades in; the click waits until it stands still and the option is what lies under the pointer.
	if !b.waitFor(`(() => { const o = document.querySelector(` + strconv.Quote(option) + `); if (!o || document.getAnimations().some(a => a.playState === 'running')) return false;
		const r = o.getBoundingClientRect(), e = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2); return !!e && o.contains(e); })()`) {
		b.t.Fatalf("%s lies under something else: %s", option, b.eval(`(() => { const o = document.querySelector(`+strconv.Quote(option)+`), r = o.getBoundingClientRect(), e = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2); return e ? e.outerHTML.slice(0, 160) : 'nothing'; })()`))
	}
	b.shot("combobox-open")
	b.clickOn(option, 1)
	if !b.waitFor(`document.querySelector(` + strconv.Quote(combobox+` > input[type=hidden]`) + `).value === ` + strconv.Quote(value)) {
		b.t.Fatalf("%s did not take %s: %q", combobox, value, b.eval(`document.querySelector(`+strconv.Quote(combobox+` > input[type=hidden]`)+`).value`))
	}
	// The list closes with a fade over what lies below it (the role, Add): the next click waits until it is gone.
	if !b.waitFor(`document.querySelector(` + strconv.Quote(combobox+` [data-popover]`) + `).getAttribute('aria-hidden') === 'true' && !document.getAnimations().some(a => a.playState === 'running')`) {
		b.t.Fatalf("the list of %s does not close", combobox)
	}
}

func (b *browser) submit(form string, values map[string]string) {
	b.t.Helper()
	b.exists(form)
	script := `(() => { const f = document.querySelector(` + strconv.Quote(form) + `);`
	for name, value := range values {
		script += `f.elements[` + strconv.Quote(name) + `].value = ` + strconv.Quote(value) + `;`
	}
	b.eval(script + ` f.requestSubmit(); })()`)
}

// TestTheRightsDialog: from the settings of a page, its owner grants a person a role, breaks the inheritance and makes a
// link; the facts panel names who sees the page; the link shows the page to anyone (ADR-29).
func TestTheRightsDialog(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.signInWith("bob", "Bob Kern", "")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.signIn(true)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	// Everyone reads the whole collection: on the page that is a role from above.
	b.eval(`(() => { const x = new XMLHttpRequest(); x.open('PUT', '/api/objects/` + team + `/grants', false); x.setRequestHeader('Content-Type', 'application/json'); x.send(JSON.stringify({kind: 'tenant', who: '', role: 'read'})); return x.status; })()`)
	b.clickOn(rowNamed(b, "A")+` [data-act="open"]`, 1)
	b.ready("A")
	b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
	b.clickOn(`[data-act="rights"]`, 1)
	if !b.waitFor(`document.getElementById('udrights')?.open && !!document.querySelector('#udrights [data-rform="grant"]')`) {
		t.Fatalf("the dialog opens with the means to grant: %s", b.eval(`document.getElementById('udrights')?.innerText`))
	}
	// From C22's rights block: owner and inheritance are entries with their action, a role from above names its origin and
	// the role as one split badge, each grant has the icon of its kind (T-12).
	if got := b.eval(`[...document.querySelectorAll('#udrights .item-group > .item h4')].map(n => n.textContent).join(' | ') + ' / ' +
		[...document.querySelectorAll('#udrights [data-inherited] li')].map(n => n.querySelector('svg')?.dataset.iconLu + ' ' + n.innerText.replace(/\s+/g, ' ').trim() + ' ' + [...n.querySelectorAll('.badge-split > span')].map(x => x.textContent).join('|')).join(', ')`); got != "Owner | Takes the roles from above | Agents / user Anna Berg from above manage from above|manage, globe Everyone from above read from above|read" {
		t.Errorf("the parts of the rights dialog: %q", got)
	}
	b.submit(`#udrights [data-rform="grant"]`, map[string]string{"who": "person:bob", "role": "comment"})
	if !b.waitFor(`!!document.querySelector('#udrights [data-grant="person:bob"]')`) {
		t.Fatalf("bob is granted: %s", b.eval(`document.getElementById('udrights').innerText`))
	}
	if got := b.eval(`(() => { const r = document.querySelector('#udrights [data-grant="person:bob"]'); return r.querySelector('td svg')?.dataset.iconLu + ' ' + r.querySelector('[data-ract="ungrant"]')?.getAttribute('aria-label'); })()`); got != "user Take back: Bob Kern" {
		t.Errorf("a grant has the icon of its kind and a named take-back: %q", got)
	}
	b.clickOn(`#udrights [data-ract="inherit"]`, 1)
	if !b.waitFor(`document.getElementById('udrights').innerText.includes('Takes no roles from above')`) {
		t.Errorf("the inheritance breaks: %s", b.eval(`document.getElementById('udrights').innerText`))
	}
	b.submit(`#udrights [data-rform="link"]`, map[string]string{"mode": "read"})
	if !b.waitFor(`!!document.querySelector('#udrights [data-made-link] input')?.value.includes('/s/')`) {
		t.Fatalf("a link is made and shown once: %s", b.eval(`document.getElementById('udrights').innerText`))
	}
	link := b.eval(`document.querySelector('#udrights [data-made-link] input').value`)
	b.shot("rights-dialog")
	if got := b.eval(`document.querySelectorAll('#udrights [data-links] tr[data-link]').length`); got != "1" {
		t.Errorf("the link is listed: %s", got)
	}
	b.clickOn(`#udrights [data-ract="close"]`, 1)
	// The facts panel names who sees the page.
	b.open(base + "/o/" + team)
	b.ready("Team")
	b.clickOn(rowNamed(b, "A")+` [data-act="open"]`, 1)
	b.ready("A")
	b.clickOn(`[data-win="0"] [data-act="gear"]`, 1)
	if !b.waitFor(`(document.querySelector('[data-seen-by]')?.textContent || '').includes('Bob Kern (comment)')`) {
		t.Errorf("who sees it: %s", b.eval(`document.querySelector('[data-seen-by]')?.textContent`))
	}
	b.open(link)
	if !b.waitFor(`document.querySelector('[data-test="share-title"]')?.textContent === 'A'`) {
		t.Errorf("the link shows the page: %s", b.eval(`document.body.innerText.slice(0, 300)`))
	}
	b.shot("share-page")
}

// TestTheTabRights: an administrator makes a group in the tab Rights, takes a member in, sees the users, and asks who
// sees a page (ADR-29).
func TestTheTabRights(t *testing.T) {
	b, base := setupAsAdmin(t)
	b.signInWith("bob", "Bob Kern", "")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.signIn(true)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.clickOn(`[data-act="view"][data-id="rights"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-rights="groups"] [data-group="ops"]')`) {
		t.Fatalf("the groups, with ops from the sign-in: %s", b.eval(`document.querySelector('[aria-label=Nav3] aside.card')?.innerText`))
	}
	b.submit(`[data-rights="groups"] [data-rform="group"]`, map[string]string{"name": "Board", "description": "The board"})
	if !b.waitFor(`!!document.querySelector('[data-rights="groups"] [data-group="Board"]')`) {
		t.Fatalf("the group is made: %s", b.eval(`document.querySelector('[aria-label=Nav3] aside.card')?.innerText`))
	}
	b.clickOn(`[data-group="Board"] [data-ract="members"]`, 1)
	b.submit(`[data-rights="groups"] [data-rform="member"]`, map[string]string{"account": "bob"})
	if !b.waitFor(`(document.querySelector('[data-rights="groups"]')?.innerText || '').includes('Bob Kern')`) {
		t.Errorf("bob is a member: %s", b.eval(`document.querySelector('[aria-label=Nav3] aside.card')?.innerText`))
	}
	b.clickOn(`[data-act="sec"][data-id="users"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-rights="users"] [data-user="bob"]')`) {
		t.Errorf("the users: %s", b.eval(`document.querySelector('[aria-label=Nav3] aside.card')?.innerText`))
	}
	b.clickOn(`[data-act="sec"][data-id="check"]`, 1)
	b.submit(`[data-rights="check"] [data-rform="who"]`, map[string]string{"object": "/o/" + team, "account": "bob"})
	if !b.waitFor(`(document.querySelector('[data-who]')?.innerText || '').includes('Everyone') && (document.querySelector('[data-check]')?.innerText || '').includes('Role: edit')`) {
		t.Errorf("who sees Team, and what bob may do: %s", b.eval(`document.querySelector('[aria-label=Nav3] aside.card')?.innerText`))
	}
	b.shot("rights-tab")
	// What bob left behind: a collection of his own, which the administrator transfers without seeing what is in it.
	b.signInWith("bob", "Bob Kern", "")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.eval(`window.left = null; fetch('/api/collections', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: '{"title":"Left behind"}'}).then(r => r.json()).then(j => { window.left = j.id; })`)
	if !b.waitFor(`!!window.left`) {
		t.Fatal("bob's collection was not made")
	}
	left := b.eval(`window.left`)
	b.signIn(true)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.clickOn(`[data-act="view"][data-id="rights"]`, 1)
	b.clickOn(`[data-act="sec"][data-id="users"]`, 1)
	b.clickOn(`[data-rights="users"] [data-user="bob"] [data-ract="heldby"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-held-row="` + left + `"] [data-rform="transfer"]')`) {
		t.Fatalf("what bob owns, with Transfer: %s", b.eval(`document.querySelector('[aria-label=Nav3] aside.card')?.innerText`))
	}
	// Nobody is chosen before the administrator chooses, and bob, whose things these are, is not offered: sent as it is,
	// the form gives nothing away.
	transfer := `[data-held-row="` + left + `"] [data-rform="transfer"]`
	if got := b.eval(`(() => { const s = document.querySelector('` + transfer + ` select'); return s.value + '|' + [...s.options].filter(o => !o.disabled).map(o => o.value).join(','); })()`); got != "|anna" {
		t.Errorf("the transfer chooses nobody and offers everyone but bob: %q", got)
	}
	b.eval(`window.confirm = () => true; window.asked = 0; document.querySelector('` + transfer + `').requestSubmit(); document.querySelector('` + transfer + `').dispatchEvent(new Event('submit', {bubbles: true, cancelable: true}))`)
	b.eval(`window.r = null; fetch('/api/rights/held?account=bob').then(r => r.json()).then(j => { window.r = JSON.stringify(j.filter(x => x.object === '` + left + `').map(x => x.via)); }); ''`)
	if !b.waitFor(`window.r !== null`) || b.eval(`window.r`) != `["owner"]` {
		t.Errorf("sent without a choice, bob still owns it: %s", b.eval(`String(window.r)`))
	}
	b.submit(transfer, map[string]string{"to": "anna"})
	// Bob keeps edit, as a former owner does.
	if !b.waitFor(`!document.querySelector('[data-held-row="` + left + `"] [data-rform="transfer"]') && (document.querySelector('[data-held-row="` + left + `"]')?.innerText || '').includes('edit')`) {
		t.Errorf("transferred, bob only edits it: %s", b.eval(`document.querySelector('[aria-label=Nav3] aside.card')?.innerText`))
	}
	// The log names what was done, the group made among it.
	b.clickOn(`[data-act="sec"][data-id="log"]`, 1)
	if !b.waitFor(`!!document.querySelector('[data-rights="log"] ol.timeline[data-size="sm"][data-log] > li[data-entry="group"] > header > time') && (document.querySelector('[data-rights="log"]')?.innerText || '').includes('Board')`) {
		t.Errorf("the log: %s", b.eval(`document.querySelector('[aria-label=Nav3] aside.card')?.innerText`))
	}
	b.shot("rights-log")
}

// TestANewCollectionAsksWithWhomToShare: the dialog of a new collection at the top has the step Share with, through
// the same roles as the rights dialog. Anna shares hers with bob to comment and makes it; bob comments, carl sees
// nothing, and its overview says that nobody else may do anything with it (ADR-29, 2026-11-19).
func TestANewCollectionAsksWithWhomToShare(t *testing.T) {
	b, base := setupWithDatabase(t)
	for _, who := range [][2]string{{"bob", "Bob Kern"}, {"carl", "Carl"}} {
		b.signInWith(who[0], who[1], "")
		b.open(base + "/o/plan")
		b.ready("Network plan")
	}
	b.signIn(true)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	b.clickOn(`[aria-label=Nav2] [data-act="addmenu"][data-id="n2"]`, 1)
	b.clickOn(probe(b, `[data-menu="new"] [role="menuitem"]`, "Collection"), 1)
	if !b.waitFor(`document.getElementById('udrights')?.open && !!document.querySelector('#udrights [data-share-step] [role=option][data-value="person:bob"]')`) {
		t.Fatalf("the step Share with lists the people: %s", b.eval(`document.getElementById('udrights')?.innerText`))
	}
	// C22's form in steps: the name first, Next leads to Share with; adding someone draws the list again and keeps the step.
	step := `[...document.querySelectorAll('#udrights [data-step]')].map(n => n.hidden ? '-' : '+').join('') + ' ' + document.querySelector('#udrights ol.steps > li[aria-current="step"]')?.textContent + ' ' +
		['back', 'next', 'done'].map(k => document.querySelector('#udrights [data-step-' + k + ']').hidden ? '-' : '+').join('')`
	if got := b.eval(step); got != "+- Name -+-" {
		t.Errorf("the dialog starts at the name: %q", got)
	}
	b.clickOn(`#udrights [data-step-next]`, 1)
	if got := b.eval(step); got != "-+ Share with +-+" {
		t.Errorf("Next leads to Share with: %q", got)
	}
	// Whom is C22's grouped combobox (Basecoat's core and combobox alone): typing filters across the groups, a click picks.
	b.pick(`#udrights [data-share-step] .combobox`, "Kern", "person:bob")
	b.eval(`document.querySelector('#udrights [data-share-step] select[name="role"]').value = 'comment'`)
	b.clickOn(`#udrights [data-ract="newshare"]`, 1)
	if !b.waitFor(`!!document.querySelector('#udrights [data-new-share="person:bob"]') && !document.querySelector('#udrights [data-only-me]')`) {
		t.Fatalf("bob is in the list: %s", b.eval(`document.getElementById('udrights').innerText`))
	}
	if got := b.eval(step); got != "-+ Share with +-+" {
		t.Errorf("adding someone keeps the step: %q", got)
	}
	// Added twice, the same one is listed once, with the role chosen last.
	b.pick(`#udrights [data-share-step] .combobox`, "every", "tenant:")
	b.clickOn(`#udrights [data-ract="newshare"]`, 1)
	if !b.waitFor(`document.querySelectorAll('#udrights [data-new-shares] tr').length === 2`) {
		t.Fatalf("everyone is in the list: %s", b.eval(`document.getElementById('udrights').innerText`))
	}
	b.clickOn(`#udrights [data-new-share="tenant:"] [data-ract="newunshare"]`, 1)
	if !b.waitFor(`document.querySelectorAll('#udrights [data-new-shares] tr').length === 1`) {
		t.Fatalf("everyone is taken out: %s", b.eval(`document.getElementById('udrights').innerText`))
	}
	b.eval(`(() => { const f = document.querySelector('#udrights [data-new-title]'); f.value = 'Board'; f.dispatchEvent(new Event('input', {bubbles: true})); })()`)
	b.shot("new-collection-share")
	b.clickOn(`#udrights footer button[type="submit"]`, 1)
	if !b.waitFor(`!document.getElementById('udrights').open && [...document.querySelectorAll('[aria-label=Nav2] .workspace-item > span')].some(n => n.textContent === 'Board')`) {
		t.Fatalf("Board is made: %s", b.eval(`document.getElementById('udrights')?.innerText + ' / ' + document.querySelector('[aria-label=Nav2]').innerText.replace(/\s+/g, ' ')`))
	}
	board := b.eval(`(() => { const n = [...document.querySelectorAll('[aria-label=Nav2] [data-act="coll"]')].find(n => n.textContent.includes('Board')); return D[n.dataset.id].sid; })()`)
	b.eval(`window.r = null; fetch('/api/objects/` + board + `/rights').then(r => r.json()).then(j => { window.r = JSON.stringify(j.own.map(g => g.kind + ':' + g.who + ':' + g.role)); }); ''`)
	if !b.waitFor(`window.r === '["person:bob:comment"]'`) {
		t.Errorf("only bob is granted, to comment: %s", b.eval(`String(window.r)`))
	}
	// Its overview: nobody else, that is, nobody in the workspace without a role of their own.
	b.open(base + "/o/" + board)
	b.ready("Board")
	if got := b.eval(`document.querySelector('[data-act="cacc"][data-id="none"]')?.getAttribute('aria-pressed')`); got != "true" {
		t.Errorf("the overview says nobody else: %q", got)
	}
	b.signInWith("bob", "Bob Kern", "")
	b.open(base + "/o/" + board)
	b.ready("Board")
	b.signInWith("carl", "Carl", "")
	b.open(base + "/o/plan")
	b.ready("Network plan")
	if got := b.eval(`[...document.querySelectorAll('[aria-label=Nav2] .workspace-item > span')].some(n => n.textContent === 'Board')`); got != "false" {
		t.Errorf("carl sees Board")
	}
	b.eval(`window.c = null; fetch('/api/objects/` + board + `/rights').then(r => { window.c = r.status; }); ''`)
	if !b.waitFor(`window.c === 404`) {
		t.Errorf("carl reads the rights of Board: %s", b.eval(`String(window.c)`))
	}
	b.signIn(true)
}
