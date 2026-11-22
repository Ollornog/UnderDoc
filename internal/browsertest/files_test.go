//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/store"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/web"
)

// setupWithFiles starts the handler with a database and a content store; anna is an administrator.
func setupWithFiles(t *testing.T) (*browser, string, *db.DB, *store.Store) {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetSignIn(tenant.With(context.Background(), tenant.Main), db.SignIn{Methods: []string{db.MethodGate}}); err != nil {
		t.Fatal(err)
	}
	b, base := setupWith(t, web.Options{DevUI: true, DB: database, Store: s, AdminGroup: "ops", TextWays: db.Ways{Browser: true}}, nil)
	return b, base, database, s
}

// waitEvent reads the events of the browser until one of this method comes, and returns its parameters.
func (b *browser) waitEvent(method string) json.RawMessage {
	b.t.Helper()
	deadline := time.After(evalPatience)
	for {
		select {
		case m, open := <-b.events:
			if !open {
				b.t.Fatal("Chrome has ended")
			}
			if m.Method == method && m.SessionID == b.session {
				return m.Params
			}
		case <-deadline:
			b.t.Fatalf("no %s", method)
		}
	}
}

// choose clicks a button that opens the file chooser, as a hand would, and picks files in it.
func (b *browser) choose(selector string, files ...string) {
	b.t.Helper()
	b.call("Page.setInterceptFileChooserDialog", map[string]any{"enabled": true}, nil)
	b.clickOn(selector, 1)
	var opened struct{ BackendNodeID int }
	if err := json.Unmarshal(b.waitEvent("Page.fileChooserOpened"), &opened); err != nil {
		b.t.Fatal(err)
	}
	b.call("DOM.setFileInputFiles", map[string]any{"files": files, "backendNodeId": opened.BackendNodeID}, nil)
	b.call("Page.setInterceptFileChooserDialog", map[string]any{"enabled": false}, nil)
}

func writeTemp(t *testing.T, name string, content []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// openMyFiles opens the view Files at the user's own folder, once the roots have come from the server: until then nav2
// shows nothing in their place.
func (b *browser) openMyFiles() {
	b.t.Helper()
	b.clickOn(`[aria-label=Nav1] [data-act="view"][data-id="files"]`, 1)
	if !b.waitFor(`FS.on && !FS.pending`) {
		b.t.Fatal("the roots of the files did not come")
	}
	b.clickOn(probe(b, `[aria-label=Nav2] [data-act="sec"]`, "My files"), 1)
}

// filesSettled waits until nothing is left that draws the workspace again on its own: the uploads are done and their list
// has gone, no request of the view is under way, and no toast waits to go. Each of them redraws everything when it comes,
// which drops a mark put on an entry, swallows a click whose press and release it falls between, and (before the form
// kept it) put the first name back into a field being typed into. The toast of a restore went 2.6 s after it, the list
// of the uploads after a drop that came right after it 2.5 s after the upload: in CI the toast went last, while the test
// was already typing the name of a new folder (2026-11-17).
func (b *browser) filesSettled() {
	b.t.Helper()
	if !b.waitFor(`FS.ups.length === 0 && FS.busy === 0 && !S.toast && !Object.values(FS.dirs).some(d => d.loading)`) {
		b.t.Fatalf("the files did not settle: %s", b.eval(`JSON.stringify({ups: FS.ups, busy: FS.busy, toast: S.toast})`))
	}
}

const fileRows = `[...document.querySelectorAll('[aria-label=Nav3] [data-act="fopen"], [aria-label=Nav3] [data-act="fdir"]:not([data-toggle])')].map(n => n.textContent.trim()).join(', ')`

// The view Files with the real mouse: a root in nav2, an upload through the file chooser, the file in the window with its
// preview, a new version, a restore, files dropped from the desktop, and a new folder.
func TestFilesByHand(t *testing.T) {
	b, base, _, _ := setupWithFiles(t)
	b.open(base + "/")
	b.ready("")
	b.openMyFiles()
	if !b.waitFor(`!!document.querySelector('[aria-label=Nav3] [data-act="fup"]')`) {
		t.Fatalf("no upload in my files: %s", b.eval(`document.querySelector('[aria-label=Nav3]')?.innerText`))
	}
	if got := b.eval(`[...document.querySelectorAll('[aria-label=Nav2] [data-act="sec"] > span')].map(n => n.textContent).join(', ')`); got != "Company, My files, New collection, Add a source" {
		t.Errorf("the roots in nav2: %q", got)
	}
	picture, _ := base64.StdEncoding.DecodeString(pixel)
	b.choose(`[aria-label=Nav3] [data-act="fup"]`, writeTemp(t, "photo.png", picture))
	if !b.waitFor(fileRows + ` === 'photo.png'`) {
		t.Fatalf("the upload is not listed: %q", b.eval(fileRows))
	}
	b.filesSettled()
	b.clickOn(probe(b, `[aria-label=Nav3] [data-act="fopen"]`, "photo.png"), 1)
	b.ready("photo.png")
	if !b.waitFor(`(() => { const i = document.querySelector('article.typeset img'); return !!i && i.complete && i.naturalWidth === 1; })()`) {
		t.Fatalf("no preview of the image: %s", b.eval(`document.querySelector('article.typeset')?.innerHTML.slice(0, 400)`))
	}
	b.shot("file-in-window")
	// A new version through its button, then the first one back.
	b.filesSettled()
	b.choose(probe(b, `article.typeset button.chip`, "Upload a new version"), writeTemp(t, "other.png", append(picture, 0)))
	versions := `document.querySelectorAll('article.typeset table tr').length - 1`
	if !b.waitFor(versions + ` === 2`) {
		t.Fatalf("versions: %s", b.eval(versions))
	}
	b.filesSettled()
	b.clickOn(probe(b, `article.typeset [data-act="frestore"]`, "Restore"), 1)
	if !b.waitFor(versions + ` === 3`) {
		t.Fatalf("after the restore: %s", b.eval(versions))
	}
	// Files dropped from the desktop onto the list land in the folder shown.
	b.filesSettled()
	notes := writeTemp(t, "notes.txt", []byte("hello"))
	list := b.rect(`[aria-label=Nav3] #tree`)
	for _, kind := range []string{"dragEnter", "dragOver", "drop"} {
		b.call("Input.dispatchDragEvent", map[string]any{"type": kind, "x": list.X, "y": list.Y,
			"data": map[string]any{"items": []any{}, "files": []string{notes}, "dragOperationsMask": 1}}, nil)
	}
	if !b.waitFor(fileRows + ` === 'notes.txt, photo.png'`) {
		t.Fatalf("the dropped file is not listed: %q", b.eval(fileRows))
	}
	// A new folder is named in place and opens with its path.
	b.filesSettled()
	b.clickOn(`[aria-label=Nav3] [data-act="fnewdir"]`, 1)
	field := `document.querySelector('[aria-label=Nav3] form[data-fform="dir"] input')`
	b.clickOn(`[aria-label=Nav3] form[data-fform="dir"] input`, 3)
	if !b.waitFor(`document.activeElement === ` + field + ` && ` + field + `.selectionStart === 0 && ` + field + `.selectionEnd === ` + field + `.value.length`) {
		t.Fatalf("the name of the folder does not wait for the typing: %s", b.eval(`document.activeElement?.outerHTML.slice(0, 160)`))
	}
	b.typeText("Scans")
	// What is typed stays, with the focus, when the workspace draws itself again meanwhile.
	b.eval(`render()`)
	if got := b.eval(field + `.value + ' ' + (document.activeElement === ` + field + `)`); got != "Scans true" {
		t.Fatalf("the name typed after a redraw: %q", got)
	}
	b.clickOn(`[aria-label=Nav3] form[data-fform="dir"] button`, 1)
	if !b.waitFor(fileRows + ` === 'Scans, notes.txt, photo.png'`) {
		t.Fatalf("the new folder: %q", b.eval(fileRows))
	}
	b.clickOn(probe(b, `[aria-label=Nav3] li > div > button[data-act="fdir"]:not([data-toggle])`, "Scans"), 1)
	if !b.waitFor(`document.querySelector('[aria-label=Nav3] .hint')?.textContent.startsWith('My files › Scans')`) {
		t.Fatalf("the path: %q", b.eval(`document.querySelector('[aria-label=Nav3] .hint')?.textContent`))
	}
	b.shot("files-folder")
}

// A chunk that breaks on the way is sent again, and the server takes the upload from where it stands: the file that
// arrives is the file that was chosen.
func TestUploadResumesAfterABreak(t *testing.T) {
	b, base, database, s := setupWithFiles(t)
	b.open(base + "/")
	b.ready("")
	b.openMyFiles()
	big := make([]byte, 2*web.MaxChunk+12345)
	_, _ = rand.Read(big)
	sum := sha256.Sum256(big)
	path := writeTemp(t, "big.bin", big)
	b.call("Fetch.enable", map[string]any{"patterns": []any{map[string]any{"urlPattern": "*/api/uploads/*", "requestStage": "Request"}}}, nil)
	b.choose(`[aria-label=Nav3] [data-act="fup"]`, path)
	patches, broken, finished := 0, false, false
	for deadline := time.Now().Add(60 * time.Second); !finished; {
		if time.Now().After(deadline) {
			t.Fatalf("the upload did not finish: %d chunks sent", patches)
		}
		var paused struct {
			RequestID string
			Request   struct{ URL, Method string }
		}
		if err := json.Unmarshal(b.waitEvent("Fetch.requestPaused"), &paused); err != nil {
			t.Fatal(err)
		}
		if paused.Request.Method == "PATCH" {
			patches++
		}
		if paused.Request.Method == "PATCH" && patches == 2 && !broken {
			broken = true
			b.call("Fetch.failRequest", map[string]any{"requestId": paused.RequestID, "errorReason": "ConnectionReset"}, nil)
			continue
		}
		finished = strings.HasSuffix(paused.Request.URL, "/finish")
		b.call("Fetch.continueRequest", map[string]any{"requestId": paused.RequestID}, nil)
	}
	b.call("Fetch.disable", map[string]any{}, nil)
	if !b.waitFor(fileRows + ` === 'big.bin'`) {
		t.Fatalf("the upload is not listed: %q", b.eval(fileRows))
	}
	if patches != 4 {
		t.Errorf("%d chunks sent for three, one of them broken", patches)
	}
	roots, _ := database.FileRoots(tenant.With(context.Background(), tenant.Main), "anna")
	list, _ := database.FilesIn(tenant.With(context.Background(), tenant.Main), roots[1].ID, "anna")
	if len(list) != 1 || list[0].Blob != hex.EncodeToString(sum[:]) {
		t.Fatalf("the stored file is not the chosen one: %+v", list)
	}
	f, err := s.Open(store.DefaultTenant, list[0].Blob)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	got, _ := io.ReadAll(f)
	if !bytes.Equal(got, big) {
		t.Fatal("the content differs")
	}
	if left := b.eval(`Object.keys(localStorage).filter(k => k.startsWith('ud-upload:')).length`); left != "0" {
		t.Errorf("%s uploads are still kept for resuming", left)
	}
}

// While the roots of the files are on their way, nav2 shows none: not the made-up ones of the mockup, which a click
// would open instead (the cause of a red run under load, 2026-11-17).
func TestFilesWaitForTheirRoots(t *testing.T) {
	b, base, _, _ := setupWithFiles(t)
	b.call("Fetch.enable", map[string]any{"patterns": []any{map[string]any{"urlPattern": "*/api/files", "requestStage": "Request"}}}, nil)
	b.call("Page.navigate", map[string]any{"url": base + "/"}, nil)
	var held struct{ RequestID string }
	if err := json.Unmarshal(b.waitEvent("Fetch.requestPaused"), &held); err != nil {
		t.Fatal(err)
	}
	b.ready("")
	b.clickOn(`[aria-label=Nav1] [data-act="view"][data-id="files"]`, 1)
	if got := b.eval(`[...document.querySelectorAll('[aria-label=Nav2] [data-act="sec"] > span')].map(n => n.textContent).join(', ')`); got != "" {
		t.Errorf("before the roots came, nav2 shows %q", got)
	}
	b.call("Fetch.continueRequest", map[string]any{"requestId": held.RequestID}, nil)
	b.call("Fetch.disable", map[string]any{}, nil)
	if !b.waitFor(`[...document.querySelectorAll('[aria-label=Nav2] [data-act="sec"] > span')].map(n => n.textContent).join(', ') === 'Company, My files, New collection, Add a source'`) {
		t.Fatalf("the roots: %s", b.eval(`document.querySelector('[aria-label=Nav2]')?.innerText`))
	}
}

// minimalPDF is a PDF of one page with one line of text, with its cross-reference table.
func minimalPDF(text string) []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 144] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		"",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	stream := "BT /F1 24 Tf 20 60 Td (" + text + ") Tj ET"
	objects[3] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream)
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := []int{}
	for i, o := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return []byte(b.String())
}

// A PDF shows in the window through pdf.js, served by UnderDoc itself and under its policy: the page is drawn into a
// canvas, and nothing is loaded from elsewhere.
func TestPDFPreview(t *testing.T) {
	b, base, _, _ := setupWithFiles(t)
	b.open(base + "/")
	b.ready("")
	b.openMyFiles()
	b.choose(`[aria-label=Nav3] [data-act="fup"]`, writeTemp(t, "report.pdf", minimalPDF("Card payment")))
	b.filesSettled()
	b.clickOn(probe(b, `[aria-label=Nav3] [data-act="fopen"]`, "report.pdf"), 1)
	b.ready("report.pdf")
	drawn := `(() => { const c = document.querySelector('article.typeset [data-pdf] canvas'); if (!c) return false;
		const d = c.getContext('2d').getImageData(0, 0, c.width, c.height).data;
		for (let i = 0; i < d.length; i += 4) if (d[i + 3] > 0 && d[i] < 128) return true; return false; })()`
	if !b.waitFor(drawn) {
		t.Fatalf("the page is not drawn: %s; %s", b.eval(`document.querySelector('article.typeset [data-pdf]')?.outerHTML.slice(0, 300)`), b.eval(`JSON.stringify(PDFS)`))
	}
	b.shot("pdf-preview")
	// The browser of the uploader read the text of the PDF for the search (ADR-24).
	textOf := `(() => { const id = D[Object.keys(D).find(k => D[k]?.n === 'report.pdf' && D[k].sid)].sid;
		const x = new XMLHttpRequest(); x.open('GET', '/api/files/' + id + '/text', false); x.send(); return x.responseText; })()`
	if !b.waitFor(`JSON.parse(` + textOf + `).some(t => t.source === 'browser' && t.text.includes('Card payment'))`) {
		t.Fatalf("the browser gave no text: %s", b.eval(textOf))
	}
	// It stays drawn when the window draws itself again.
	b.eval(`render()`)
	if !b.waitFor(drawn) {
		t.Fatal("the canvas is gone after a redraw")
	}
	if got := b.eval(`performance.getEntriesByType('resource').map(e => e.name).filter(n => !n.startsWith(location.origin)).join(' ')`); got != "" {
		t.Errorf("loaded from elsewhere: %s", got)
	}
}

// An image dropped from the desktop into the editor of a page becomes an attachment of the page, on a line of its own that
// shows it; "Move to the files" makes it a file in the user's own folder, and the page shows the file from then on.
func TestImagesInTheEditor(t *testing.T) {
	b, base, _, _ := setupWithFiles(t)
	b.open(base + "/")
	b.ready("")
	b.eval(`window.made = null; fetch('/api/collections', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: '{"title":"Team"}'}).then(r => r.json()).then(j => { window.made = j.id; })`)
	if !b.waitFor(`!!window.made`) {
		t.Fatal("the collection for the test was not made")
	}
	team := b.eval(`window.made`)
	b.open(base + "/o/" + team)
	b.ready("Team")
	b.clickOn(`section > header[data-cdrop="`+team+`"] .btn`, 1)
	b.clickOn(`.cmdpop [data-act="add"][data-id="page"]`, 1)
	if !b.waitFor(`document.activeElement?.classList.contains('cm-content')`) {
		t.Fatal("the new page is not in the editor")
	}
	b.typeKeys("Rack photo:")
	picture, _ := base64.StdEncoding.DecodeString(pixel)
	photo := writeTemp(t, "rack photo.png", picture)
	r := b.rect(`.cm-content`)
	for _, kind := range []string{"dragEnter", "dragOver", "drop"} {
		b.call("Input.dispatchDragEvent", map[string]any{"type": kind, "x": r.X, "y": r.Top + 8,
			"data": map[string]any{"items": []any{}, "files": []string{photo}, "dragOperationsMask": 1}}, nil)
	}
	if !b.waitFor(`/!\[rack photo\]\(\/api\/pages\/[0-9a-z]{10}\/attachments\/a1\)/.test(EDS[at(0).d]?.ed.text()) && document.querySelector('[data-edstatus]')?.textContent === 'Saved'`) {
		t.Fatalf("the image is not in the page: %q", b.eval(`EDS[at(0).d]?.ed.text()`))
	}
	if !b.waitFor(`[...document.querySelectorAll('.cm-content img.cm-lp-image')].some(i => i.complete && i.naturalWidth === 1)`) {
		t.Fatalf("the editor does not show the image: %s", b.eval(`document.querySelector('.cm-content')?.innerHTML.slice(0, 400)`))
	}
	b.shot("image-in-editor")
	b.clickOn(`[data-act="edit"]`, 1)
	if !b.waitFor(`!document.querySelector('.cm-editor') && [...document.querySelectorAll('article.typeset img')].some(i => i.complete && i.naturalWidth === 1)`) {
		t.Fatalf("reading does not show the image: %s", b.eval(`document.querySelector('article.typeset')?.innerHTML.slice(0, 400)`))
	}
	b.clickOn(`[data-act="gear"]`, 1)
	// The facts of the page stand below the window's fold: scrolled into view as a hand would, once the attachments are
	// there and nothing draws the drawer again.
	if !b.waitFor(`D[at(0).d].atts?.length === 1 && !D[at(0).d].loading && (() => { const x = document.querySelector('[data-act="fatt"]');
		if (!x) return false; x.scrollIntoView({block: 'center'}); const r = x.getBoundingClientRect();
		return document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2) === x; })()`) {
		t.Fatal("Move to the files is not in reach")
	}
	// The drawer of the facts may move while it fills in under load; the button is taken by the keyboard, focused and
	// pressed in one go, which a moving layout cannot misdirect. A redraw between the two keeps the focus on the button
	// (before, the Enter went nowhere then: a red run under load, 2026-11-17).
	b.eval(`document.querySelector('[data-act="fatt"]').focus()`)
	b.eval(`render()`)
	if got := b.eval(`document.activeElement?.dataset.act`); got != "fatt" {
		t.Fatalf("after a redraw the focus is on %q, not on Move to the files", got)
	}
	for _, kind := range []string{"keyDown", "keyUp"} {
		b.call("Input.dispatchKeyEvent", map[string]any{"type": kind, "key": "Enter", "code": "Enter", "windowsVirtualKeyCode": 13, "text": "\r"}, nil)
	}
	if !b.waitFor(`[...document.querySelectorAll('article.typeset img')].some(i => /^\/api\/files\/[0-9a-z]{10}\/content$/.test(i.getAttribute('src')) && i.complete && i.naturalWidth === 1)`) {
		t.Fatalf("the page does not show the file: %s; %s", b.eval(`document.querySelector('article.typeset')?.innerHTML.slice(0, 400)`), b.eval(`(() => { const sid = D[at(0).d].sid, x = new XMLHttpRequest(); x.open('GET', '/api/pages/' + sid + '/attachments', false); x.send(); const y = new XMLHttpRequest(); y.open('GET', '/api/doc/' + sid, false); y.send(); return x.responseText + ' || ' + y.responseText.slice(0, 300) + ' || ' + JSON.stringify(D[at(0).d].atts) })()`))
	}
	b.openMyFiles()
	if !b.waitFor(fileRows + ` === 'rack photo.png'`) {
		t.Fatalf("the file is not in my files: %q", b.eval(fileRows))
	}
}

// An image uploaded is read in the same browser with Tesseract.js (ADR-24): the words in it become the text of the file,
// and everything comes from UnderDoc itself, the language data too. The image is drawn by the browser, so its letters are
// what a scan of a printed page looks like.
func TestTextOfAnImageInTheBrowser(t *testing.T) {
	b, base, _, _ := setupWithFiles(t)
	b.open(base + "/")
	b.ready("")
	drawn := b.eval(`(() => { const c = document.createElement('canvas'); c.width = 900; c.height = 200;
		const g = c.getContext('2d'); g.fillStyle = '#fff'; g.fillRect(0, 0, c.width, c.height); g.fillStyle = '#000';
		g.font = '48px sans-serif'; g.fillText('Lieferschein 4711', 30, 80); g.fillText('Größe und Gewicht', 30, 160);
		return c.toDataURL('image/png').split(',')[1]; })()`)
	png, err := base64.StdEncoding.DecodeString(drawn)
	if err != nil {
		t.Fatal(err)
	}
	b.openMyFiles()
	b.choose(`[aria-label=Nav3] [data-act="fup"]`, writeTemp(t, "lieferschein.png", png))
	b.filesSettled()
	b.clickOn(probe(b, `[aria-label=Nav3] [data-act="fopen"]`, "lieferschein.png"), 1)
	b.ready("lieferschein.png")
	textOf := `(() => { const id = D[Object.keys(D).find(k => D[k]?.n === 'lieferschein.png' && D[k].sid)].sid;
		const x = new XMLHttpRequest(); x.open('GET', '/api/files/' + id + '/text', false); x.send(); return x.responseText; })()`
	read := `JSON.parse(` + textOf + `).some(t => t.source === 'browser' && t.text.includes('Lieferschein 4711') && t.text.includes('Größe'))`
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline) && b.eval(read) != "true"; {
		time.Sleep(200 * time.Millisecond)
	}
	if b.eval(read) != "true" {
		t.Fatalf("the browser read no text, or the wrong one: %s", b.eval(textOf))
	}
	if got := b.eval(`performance.getEntriesByType('resource').map(e => e.name).filter(n => !n.startsWith(location.origin)).join(' ')`); got != "" {
		t.Errorf("loaded from elsewhere: %s", got)
	}
}

// TestANewCollectionOfFilesAsksWithWhomToShare: New collection in the view Files takes its name, then the same dialog
// as in the documents asks with whom to share it; made with nobody, it is its maker's alone (ADR-29, 2026-11-19).
func TestANewCollectionOfFilesAsksWithWhomToShare(t *testing.T) {
	b, base, _, _ := setupWithFiles(t)
	b.open(base + "/")
	b.ready("")
	b.clickOn(`[aria-label=Nav1] [data-act="view"][data-id="files"]`, 1)
	if !b.waitFor(`FS.on && !FS.pending`) {
		t.Fatal("the roots of the files did not come")
	}
	b.clickOn(probe(b, `[aria-label=Nav2] [data-act="sec"]`, "New collection"), 1)
	b.submit(`[aria-label=Nav3] form[data-fform="coll"]`, map[string]string{"title": "Scans"})
	if !b.waitFor(`document.getElementById('udrights')?.open && document.querySelector('#udrights [data-new-title]')?.value === 'Scans' && !!document.querySelector('#udrights [data-only-me]')`) {
		t.Fatalf("the dialog asks with whom to share: %s", b.eval(`document.getElementById('udrights')?.innerText`))
	}
	b.clickOn(`#udrights footer button[type="submit"]`, 1)
	if !b.waitFor(`!document.getElementById('udrights').open && [...document.querySelectorAll('[aria-label=Nav2] [data-act="sec"] > span')].some(n => n.textContent === 'Scans')`) {
		t.Fatalf("Scans is a root now: %s", b.eval(`document.querySelector('[aria-label=Nav2]')?.innerText`))
	}
	b.eval(`window.r = null; fetch('/api/objects/' + S.sec.files + '/rights').then(r => r.json()).then(j => { window.r = j.own.length + ' ' + j.ownerName; }); ''`)
	if !b.waitFor(`window.r === '0 Anna Berg'`) {
		t.Errorf("nothing granted, anna owns it: %s", b.eval(`String(window.r)`))
	}
}
