//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"strconv"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
	"github.com/Ollornog/UnderDoc/internal/web"
)

// setupWithVault is setupWithDatabase with a vault, so passwords in the text can be made (ADR-7).
func setupWithVault(t *testing.T) (*browser, string) {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	if err := database.SetSignIn(tenant.With(context.Background(), tenant.Main), db.SignIn{Methods: []string{db.MethodGate}}); err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	keys, err := vault.New(base64.StdEncoding.EncodeToString(raw))
	if err != nil {
		t.Fatal(err)
	}
	return setupWith(t, web.Options{DevUI: true, DB: database, Vault: keys}, nil)
}

func randomPassword(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return "pw-" + base64.RawURLEncoding.EncodeToString(raw)
}

// answerAsk types a value into the dialog that asks for a password, and keeps it.
func (b *browser) answerAsk(value string) {
	b.t.Helper()
	if !b.waitFor(`document.getElementById('udsecret-ask')?.open === true && document.activeElement?.matches('[data-secret-input]')`) {
		b.t.Fatalf("the dialog that asks for the password did not open: %s", b.eval(`document.getElementById('udsecret-ask')?.outerHTML.slice(0, 200) || 'none'`))
	}
	if got := b.eval(`document.querySelector('[data-secret-input]').type`); got != "password" {
		b.t.Errorf("the value is typed into a field that shows it: %q", got)
	}
	b.typeText(value)
	b.typeKeys("\n")
}

// TestPasswordsInTheEditorAndWhileReading: the bar and the slash menu ask for a password in a field that hides it and
// write only its reference; the editor and the page show a mask, Reveal shows the value in place and masks it again,
// Copy puts it on the clipboard without showing it; the value is never in the text or in the page until revealed; a reader
// left out sees "hidden", and the settings of the page list its passwords (ADR-7).
func TestPasswordsInTheEditorAndWhileReading(t *testing.T) {
	b, base := setupWithVault(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.eval(`window.made = null; (async () => {
		const json = {'Content-Type': 'application/json'};
		const p = await (await fetch('/api/pages', {method: 'POST', headers: json, body: JSON.stringify({title: 'Router', parent: '` + team + `'})})).json();
		await fetch('/api/pages/' + p.id + '/blocks', {method: 'POST', headers: json, body: JSON.stringify({md: 'Admin: ', after: ''})});
		window.made = p.id;
	})()`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the page for the test was not made")
	}
	page := b.eval(`window.made`)
	b.open(base + "/o/" + page)
	b.ready("Router")
	b.eval(`window.navigator.clipboard.writeText = async (t) => { window.clip = t; };`)
	b.clickOn(`[data-act="edit"]`, 1)
	if !b.waitFor(`!!document.querySelector('.cm-content') && !!document.querySelector('[data-act="edsecret"]')`) {
		t.Fatal("the editor did not open with the button for a password")
	}
	// The bar: the cursor at the end of "Admin: ", then the button.
	b.eval(`(() => { const e = EDS[at(0).d].ed; e.focus(); const v = e.view; v.dispatch({selection: {anchor: v.state.doc.toString().indexOf('Admin: ') + 7}}); })()`)
	first := randomPassword(t)
	b.clickOn(`[data-act="edsecret"]`, 1)
	b.answerAsk(first)
	if !b.waitFor(`/Admin: ::secret\{id="[0-9a-hjkmnp-tv-z]{10}"\}/.test(EDS[at(0).d]?.ed.text() || '') && document.querySelector('[data-edstatus]')?.textContent === 'Saved'`) {
		t.Fatalf("the reference was not written and saved: %q | %s", b.eval(`EDS[at(0).d]?.ed.text()`), b.eval(`S.toast + ' | ' + document.getElementById('udsecret-ask')?.open + ' | ' + document.activeElement?.outerHTML.slice(0, 100)`))
	}
	if got := b.eval(`(EDS[at(0).d].ed.text().includes(` + strconv.Quote(first) + `) || document.body.innerHTML.includes(` + strconv.Quote(first) + `)) + ''`); got != "false" {
		t.Fatal("the value is in the text or the page")
	}
	// The slash menu, on a new empty line: the cursor leaves the first reference, which becomes a mask.
	b.eval(`(() => { const e = EDS[at(0).d].ed, v = e.view; e.focus(); v.dispatch({selection: {anchor: v.state.doc.length}}); })()`)
	b.typeKeys("\n\n/Passw")
	if !b.waitFor(`[...document.querySelectorAll('.bslash [role=option]')].some(o => o.textContent.includes('Password'))`) {
		t.Fatalf("the slash menu offers no password: %s", b.eval(`document.querySelector('.bslash')?.textContent || 'none'`))
	}
	b.press("Enter", "Enter", 13, 0)
	second := randomPassword(t)
	b.answerAsk(second)
	if !b.waitFor(`(EDS[at(0).d]?.ed.text().match(/::secret\{id="[0-9a-hjkmnp-tv-z]{10}"\}/g) || []).length === 2 && document.querySelector('[data-edstatus]')?.textContent === 'Saved'`) {
		t.Fatalf("the slash menu wrote no second reference: %q", b.eval(`EDS[at(0).d]?.ed.text()`))
	}
	b.eval(`(() => { const e = EDS[at(0).d].ed, v = e.view; v.dispatch({selection: {anchor: v.state.doc.length}}); })()`)
	b.typeKeys("\n\nEnd.")
	if !b.waitFor(`document.querySelectorAll('.cm-content [data-ed-secret] .secret-key').length === 2`) {
		t.Fatalf("the editor shows no masks: %s", b.eval(`document.querySelector('.cm-content')?.innerText`))
	}
	if got := b.eval(`document.querySelector('.cm-content [data-ed-secret] .secret-key').textContent`); got != "••••••••" {
		t.Errorf("the mask in the editor: %q", got)
	}
	b.clickOn(`.cm-content [data-ed-secret] [data-ed-sact="show"]`, 1)
	if !b.waitFor(`document.querySelector('.cm-content [data-ed-secret] .secret-key').textContent === ` + strconv.Quote(first)) {
		t.Errorf("Reveal in the editor did not show the value: %q", b.eval(`document.querySelector('.cm-content [data-ed-secret] .secret-key').textContent`))
	}
	b.clickOn(`.cm-content [data-ed-secret] [data-ed-sact="show"]`, 1)
	if !b.waitFor(`document.querySelector('.cm-content [data-ed-secret] .secret-key').textContent === '••••••••'`) {
		t.Error("Hide in the editor did not mask it again")
	}
	b.clickOn(`.cm-content [data-ed-secret] [data-ed-sact="copy"]`, 1)
	if !b.waitFor(`window.clip === ` + strconv.Quote(first)) {
		t.Errorf("Copy in the editor: %q", b.eval(`String(window.clip)`))
	}
	if got := b.eval(`document.querySelector('.cm-content').innerText.includes(` + strconv.Quote(first) + `) + ''`); got != "false" {
		t.Error("Copy showed the value")
	}
	b.shot("secrets-editor")

	// Reading: the masks the server drew.
	b.clickOn(`[data-act="edit"]`, 1)
	if !b.waitFor(`document.querySelectorAll('article.doc [data-secret-state="ok"]').length === 2 && !document.querySelector('.cm-editor')`) {
		t.Fatalf("reading shows no masks: %s", b.eval(`document.querySelector('article.doc')?.innerHTML.slice(0, 400)`))
	}
	b.eval(`window.clip = null`)
	b.clickOn(`article.doc [data-secret-state="ok"] [data-pwact="show"]`, 1)
	if !b.waitFor(`document.querySelector('article.doc [data-secret-value]').textContent === ` + strconv.Quote(first)) {
		t.Errorf("Reveal while reading: %q", b.eval(`document.querySelector('article.doc [data-secret-value]').textContent`))
	}
	b.shot("secrets-revealed")
	b.clickOn(`article.doc [data-secret-state="ok"] [data-pwact="show"]`, 1)
	if !b.waitFor(`document.querySelector('article.doc [data-secret-value]').textContent === '••••••••'`) {
		t.Error("Hide while reading did not mask it again")
	}
	b.clickOn(`article.doc [data-secret-state="ok"] [data-pwact="copy"]`, 1)
	if !b.waitFor(`window.clip === ` + strconv.Quote(first)) {
		t.Errorf("Copy while reading: %q", b.eval(`String(window.clip)`))
	}

	// The settings list both passwords, with versions and readers.
	b.clickOn(`[data-act="gear"]`, 1)
	b.clickOn(`[data-act="secrets"]`, 1)
	if !b.waitFor(`document.getElementById('udsecrets')?.open && document.querySelectorAll('#udsecrets [data-secret-row]').length === 2`) {
		t.Fatalf("the passwords of the page are not listed: %s", b.eval(`document.getElementById('udsecrets')?.innerText || 'none'`))
	}
	b.clickOn(`#udsecrets [data-secret-row] [data-pwact="more"]`, 1)
	if !b.waitFor(`!!document.querySelector('#udsecrets [data-secret-detail] [data-secret-version="1"]') && !!document.querySelector('#udsecrets [data-secret-readers]')`) {
		t.Errorf("versions and readers: %s", b.eval(`document.getElementById('udsecrets')?.innerText`))
	}
	if got := b.eval(`document.getElementById('udsecrets').innerHTML.includes(` + strconv.Quote(first) + `) + ''`); got != "false" {
		t.Error("the dialog shows the value")
	}
	b.shot("secrets-settings")
	// Restricted to anna alone: bob reads the page and sees "hidden".
	b.eval(`(() => { const f = document.querySelector('#udsecrets [data-secret-readers]'); f.querySelector('[data-secret-restricted]').checked = true;
		for (const o of f.querySelectorAll('[data-secret-who]')) o.checked = o.parentElement.textContent.trim() === 'Anna Berg'; })()`)
	b.clickOn(`#udsecrets [data-secret-readers] [data-pwact="readers"]`, 1)
	if !b.waitFor(`/restricted/.test(document.querySelector('#udsecrets [data-secret-row]')?.textContent || '')`) {
		t.Fatalf("the restriction was not kept: %s", b.eval(`document.getElementById('udsecrets')?.innerText`))
	}
	b.clickOn(`#udsecrets [data-pwact="close"]`, 1)
	b.signInWith("bob", "Bob", "")
	b.open(base + "/o/" + page)
	b.ready("Router")
	if got := b.eval(`document.querySelectorAll('article.doc [data-secret-state="hidden"]').length + ' ' + document.querySelectorAll('article.doc [data-secret-state="ok"]').length`); got != "1 1" {
		t.Errorf("bob sees one hidden and one he may reveal: %q", got)
	}
	b.signIn(true)
}
