// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/store"
)

func pdfWith(content string) string {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 144] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	var offsets []int
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
	return b.String()
}

// fakeModel stands in for Tesseract or a model for OCR.
type fakeModel struct {
	text  string
	pages int
	calls int
}

func (f *fakeModel) Recognize(_ context.Context, in io.Reader, _ string) (string, int, error) {
	f.calls++
	_, _ = io.Copy(io.Discard, in)
	return f.text, f.pages, nil
}

// The queue of the server: a PDF with text is read in Go, a scanned PDF goes on to the model of the uploader's tenant, an
// image to Tesseract, and the files with a new text go into the queue of the search.
func TestTextWorker(t *testing.T) {
	ctx := inMain()
	d, _ := database(t)
	anna := account.User{ID: "u-anna", Login: "anna"}
	s, _ := store.Open(t.TempDir())
	if err := d.EnsureRoots(ctx, anna, false, ""); err != nil {
		t.Fatal(err)
	}
	roots, _ := d.FileRoots(ctx, anna.ID)
	put := func(name, content, contentType string, ways ...string) db.Finished {
		done := putFile(t, d, s, anna.ID, db.UploadTarget{Parent: roots[0].ID}, name, content)
		v, _ := d.FileVersion(ctx, done.File.ID, anna.ID, 0)
		if err := d.QueueText(ctx, v.Blob, contentType, anna.ID, ways...); err != nil {
			t.Fatal(err)
		}
		return done
	}
	text := put("log.pdf", pdfWith("BT /F1 12 Tf 20 60 Td (Error 0x80070005 on login) Tj ET"), "application/pdf", "server")
	scan := put("scan.pdf", pdfWith("0 0 m 100 100 l S"), "application/pdf", "server")
	photo := put("board.png", "\x89PNG fake", "image/png", "ocr")
	ocr, model := &fakeModel{text: "whiteboard notes", pages: 1}, &fakeModel{text: "scanned contract", pages: 3}
	if _, err := d.Claim(ctx, 1000); err != nil { // what the uploads queued
		t.Fatal(err)
	}
	w := textWorker{db: d, store: s, config: textConfig{Ways: db.Ways{Server: true, AI: true}, MaxSize: 1 << 20, AIUser: 10}, ocr: ocr, ai: model,
		log: slog.New(slog.DiscardHandler)}
	for range 10 {
		if did, err := w.once(ctx); err != nil {
			t.Fatal(err)
		} else if !did {
			break
		}
	}
	want := map[string]string{text.File.ID: "server:Error 0x80070005 on login", scan.File.ID: "ai:scanned contract", photo.File.ID: "ocr:whiteboard notes"}
	for id, w := range want {
		texts, err := d.FileTexts(ctx, id, anna.ID)
		if err != nil || len(texts) != 1 || texts[0].Source+":"+strings.TrimSpace(texts[0].Text) != w {
			t.Errorf("%s: %+v %v, want %s", id, texts, err, w)
		}
	}
	queued, err := d.Claim(ctx, 1000)
	var told []string
	for _, k := range queued {
		told = append(told, k.Key)
	}
	slices.Sort(told)
	files := []string{text.File.ID, scan.File.ID, photo.File.ID}
	slices.Sort(files)
	if err != nil || !slices.Equal(told, files) || model.calls != 1 || ocr.calls != 1 {
		t.Errorf("the search queued %v, want %v (%v); model %d, tesseract %d calls", told, files, err, model.calls, ocr.calls)
	}
	// The quota of the model counts every page it read.
	if err := d.UseAIPages(ctx, anna.ID, 7, 10, 0); err != nil {
		t.Errorf("3 of 10 pages used, 7 more should fit: %v", err)
	}
	if err := d.UseAIPages(ctx, anna.ID, 1, 10, 0); err == nil {
		t.Error("the eleventh page fit")
	}
}

func TestTextConfig(t *testing.T) {
	t.Setenv("UNDERDOC_TEXT_WAYS", "browser, ai")
	t.Setenv("UNDERDOC_AI_OCR_PAGES_USER", "50")
	c, err := readTextConfig()
	if err != nil || c.Ways != (db.Ways{Browser: true, AI: true}) || c.AIUser != 50 || c.MaxSize != 100<<20 {
		t.Fatalf("%+v %v", c, err)
	}
	t.Setenv("UNDERDOC_TEXT_WAYS", "telepathy")
	if _, err := readTextConfig(); err == nil {
		t.Fatal("an unknown way was taken")
	}
}
