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
	b.clickOn(`.strip [data-act="view"][data-id="files"]`, 1)
	if !b.waitFor(`FS.on && !FS.pending`) {
		b.t.Fatal("the roots of the files did not come")
	}
	b.clickOn(probe(b, `.nav2 [data-act="sec"]`, "My files"), 1)
}

const fileRows = `[...document.querySelectorAll('.nav3 [data-act="fopen"], .nav3 [data-act="fdir"].name')].map(n => n.textContent.trim()).join(', ')`

// The view Files with the real mouse: a root in nav2, an upload through the file chooser, the file in the window with its
// preview, a new version, a restore, files dropped from the desktop, and a new folder.
func TestFilesByHand(t *testing.T) {
	b, base, _, _ := setupWithFiles(t)
	b.open(base + "/")
	b.ready("")
	b.openMyFiles()
	if !b.waitFor(`!!document.querySelector('.nav3 [data-act="fup"]')`) {
		t.Fatalf("no upload in my files: %s", b.eval(`document.querySelector('.nav3')?.innerText`))
	}
	if got := b.eval(`[...document.querySelectorAll('.nav2 [data-act="sec"] .lbl')].map(n => n.textContent).join(', ')`); got != "Company, My files, New collection, Add a source" {
		t.Errorf("the roots in nav2: %q", got)
	}
	picture, _ := base64.StdEncoding.DecodeString(pixel)
	b.choose(`.nav3 [data-act="fup"]`, writeTemp(t, "photo.png", picture))
	if !b.waitFor(fileRows + ` === 'photo.png'`) {
		t.Fatalf("the upload is not listed: %q", b.eval(fileRows))
	}
	b.clickOn(probe(b, `.nav3 [data-act="fopen"]`, "photo.png"), 1)
	b.ready("photo.png")
	if !b.waitFor(`(() => { const i = document.querySelector('article.doc img'); return !!i && i.complete && i.naturalWidth === 1; })()`) {
		t.Fatalf("no preview of the image: %s", b.eval(`document.querySelector('article.doc')?.innerHTML.slice(0, 400)`))
	}
	b.shot("file-in-window")
	// A new version through its button, then the first one back.
	b.choose(probe(b, `article.doc button.chip`, "Upload a new version"), writeTemp(t, "other.png", append(picture, 0)))
	versions := `document.querySelectorAll('article.doc table tr').length - 1`
	if !b.waitFor(versions + ` === 2`) {
		t.Fatalf("versions: %s", b.eval(versions))
	}
	b.clickOn(probe(b, `article.doc [data-act="frestore"]`, "Restore"), 1)
	if !b.waitFor(versions + ` === 3`) {
		t.Fatalf("after the restore: %s", b.eval(versions))
	}
	// Files dropped from the desktop onto the list land in the folder shown.
	notes := writeTemp(t, "notes.txt", []byte("hello"))
	list := b.rect(`.nav3 #tree`)
	for _, kind := range []string{"dragEnter", "dragOver", "drop"} {
		b.call("Input.dispatchDragEvent", map[string]any{"type": kind, "x": list.X, "y": list.Y,
			"data": map[string]any{"items": []any{}, "files": []string{notes}, "dragOperationsMask": 1}}, nil)
	}
	if !b.waitFor(fileRows + ` === 'notes.txt, photo.png'`) {
		t.Fatalf("the dropped file is not listed: %q", b.eval(fileRows))
	}
	// A new folder is named in place and opens with its path.
	b.clickOn(`.nav3 [data-act="fnewdir"]`, 1)
	b.clickOn(`.nav3 form[data-fform="dir"] input`, 3)
	b.typeText("Scans")
	b.clickOn(`.nav3 form[data-fform="dir"] button`, 1)
	if !b.waitFor(fileRows + ` === 'Scans, notes.txt, photo.png'`) {
		t.Fatalf("the new folder: %q", b.eval(fileRows))
	}
	b.clickOn(probe(b, `.nav3 .name[data-act="fdir"]`, "Scans"), 1)
	if !b.waitFor(`document.querySelector('.nav3 .hint')?.textContent.startsWith('My files › Scans')`) {
		t.Fatalf("the path: %q", b.eval(`document.querySelector('.nav3 .hint')?.textContent`))
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
	b.choose(`.nav3 [data-act="fup"]`, path)
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
	b.clickOn(`.strip [data-act="view"][data-id="files"]`, 1)
	if got := b.eval(`[...document.querySelectorAll('.nav2 [data-act="sec"] .lbl')].map(n => n.textContent).join(', ')`); got != "" {
		t.Errorf("before the roots came, nav2 shows %q", got)
	}
	b.call("Fetch.continueRequest", map[string]any{"requestId": held.RequestID}, nil)
	b.call("Fetch.disable", map[string]any{}, nil)
	if !b.waitFor(`[...document.querySelectorAll('.nav2 [data-act="sec"] .lbl')].map(n => n.textContent).join(', ') === 'Company, My files, New collection, Add a source'`) {
		t.Fatalf("the roots: %s", b.eval(`document.querySelector('.nav2')?.innerText`))
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
	b.choose(`.nav3 [data-act="fup"]`, writeTemp(t, "report.pdf", minimalPDF("Card payment")))
	b.clickOn(probe(b, `.nav3 [data-act="fopen"]`, "report.pdf"), 1)
	b.ready("report.pdf")
	drawn := `(() => { const c = document.querySelector('article.doc [data-pdf] canvas'); if (!c) return false;
		const d = c.getContext('2d').getImageData(0, 0, c.width, c.height).data;
		for (let i = 0; i < d.length; i += 4) if (d[i + 3] > 0 && d[i] < 128) return true; return false; })()`
	if !b.waitFor(drawn) {
		t.Fatalf("the page is not drawn: %s; %s", b.eval(`document.querySelector('article.doc [data-pdf]')?.outerHTML.slice(0, 300)`), b.eval(`JSON.stringify(PDFS)`))
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
	b.clickOn(`.cheadrow[data-cdrop="`+team+`"] .newplus`, 1)
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
	if !b.waitFor(`!document.querySelector('.cm-editor') && [...document.querySelectorAll('article.doc img')].some(i => i.complete && i.naturalWidth === 1)`) {
		t.Fatalf("reading does not show the image: %s", b.eval(`document.querySelector('article.doc')?.innerHTML.slice(0, 400)`))
	}
	b.clickOn(`[data-act="gear"]`, 1)
	b.clickOn(probe(b, `[data-act="fatt"]`, "Move to the files"), 1)
	if !b.waitFor(`[...document.querySelectorAll('article.doc img')].some(i => /^\/api\/files\/[0-9a-z]{10}\/content$/.test(i.getAttribute('src')) && i.complete && i.naturalWidth === 1)`) {
		t.Fatalf("the page does not show the file: %s", b.eval(`document.querySelector('article.doc')?.innerHTML.slice(0, 400)`))
	}
	b.openMyFiles()
	if !b.waitFor(fileRows + ` === 'rack photo.png'`) {
		t.Fatalf("the file is not in my files: %q", b.eval(fileRows))
	}
}
