// SPDX-License-Identifier: MIT

package web

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/store"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.NRGBA{G: 255, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// fakeOffice stands in for the Office module: it answers with a PDF, or with what it is told to.
type fakeOffice struct {
	answer string
	calls  int
}

func (f *fakeOffice) ToPDF(_ context.Context, in io.Reader, _ string, out io.Writer) error {
	f.calls++
	_, _ = io.Copy(io.Discard, in)
	_, err := io.WriteString(out, f.answer)
	return err
}

func TestPreviews(t *testing.T) {
	s := withFiles(t)
	office := &fakeOffice{answer: "%PDF-1.7 from the office module"}
	s.site = newSite(t, Options{DB: s.db, Store: s.store, AdminGroup: "underdoc-admins", Office: office}, sample)
	mine := s.root(nil, db.RootUser)
	parent := fmt.Sprintf(`"parent":%q`, mine.ID)

	picture := decode[finished](t, s.put(nil, parent, "wide.png", testPNG(t, 2048, 512), 4096))
	file := decode[apiFile](t, s.get("/api/files/"+picture.ID))
	if file.Preview != "image" || file.Thumb != "/api/files/"+picture.ID+"/thumb?size=1024" {
		t.Fatalf("%+v", file)
	}
	for size, width := range map[string]int{"256": 256, "1024": 1024} {
		got := s.get("/api/files/" + picture.ID + "/thumb?size=" + size)
		config, err := jpeg.DecodeConfig(got.Body)
		if got.Code != http.StatusOK || err != nil || config.Width != width || got.Header().Get("Content-Type") != "image/jpeg" ||
			!strings.Contains(got.Header().Get("Content-Security-Policy"), "sandbox") {
			t.Errorf("size %s: %d %+v %v %v", size, got.Code, config, err, got.Header())
		}
	}
	// The preview is kept beside its blob.
	f, _ := s.db.File(tenant.With(context.Background(), tenant.Main), picture.ID, "u-7")
	kept, _ := s.store.DerivedPath(store.DefaultTenant, f.Blob+"-thumb-256.jpg")
	if _, err := os.Stat(kept); err != nil {
		t.Errorf("the preview is not kept: %v", err)
	}
	if got := s.get("/api/files/" + picture.ID + "/thumb?size=999"); got.Code != http.StatusBadRequest {
		t.Errorf("a size of its own: %d", got.Code)
	}
	if got := s.getWith("/api/files/"+picture.ID+"/thumb?size=256", merged(bob)); got.Code != http.StatusNotFound {
		t.Errorf("bob sees a preview of anna's file: %d", got.Code)
	}
	text := decode[finished](t, s.put(nil, parent, "notes.txt", []byte("plain"), 10))
	if got := s.get("/api/files/" + text.ID + "/thumb?size=256"); got.Code != http.StatusNotFound {
		t.Errorf("a preview of text: %d", got.Code)
	}

	pdf := decode[finished](t, s.put(nil, parent, "report.pdf", []byte("%PDF-1.7\nbody"), 10))
	if file := decode[apiFile](t, s.get("/api/files/"+pdf.ID)); file.Preview != "pdf" || file.PDF != "/api/files/"+pdf.ID+"/pdf" {
		t.Fatalf("%+v", file)
	}
	if got := s.get("/api/files/" + pdf.ID + "/pdf"); got.Body.String() != "%PDF-1.7\nbody" || got.Header().Get("Content-Type") != "application/octet-stream" {
		t.Errorf("pdf: %q %v", got.Body.String(), got.Header())
	}

	sheet := decode[finished](t, s.put(nil, parent, "sheet.xlsx", []byte("PK\x03\x04sheet"), 10))
	for range 2 {
		got := s.get("/api/files/" + sheet.ID + "/pdf")
		if got.Code != http.StatusOK || got.Body.String() != office.answer {
			t.Fatalf("office: %d %q", got.Code, got.Body.String())
		}
	}
	if office.calls != 1 {
		t.Errorf("the module converted %d times; the PDF is kept", office.calls)
	}
	office.answer = "<html>not a pdf"
	other := decode[finished](t, s.put(nil, parent, "other.docx", []byte("PK\x03\x04other"), 10))
	if got := s.get("/api/files/" + other.ID + "/pdf"); got.Code != http.StatusBadGateway {
		t.Errorf("a module that answers with something else: %d", got.Code)
	}

	// Without the module an Office file has no preview.
	plain := withFiles(t)
	root := plain.root(nil, db.RootUser)
	doc := decode[finished](t, plain.put(nil, fmt.Sprintf(`"parent":%q`, root.ID), "letter.docx", []byte("PK\x03\x04letter"), 10))
	if file := decode[apiFile](t, plain.get("/api/files/"+doc.ID)); file.Preview != "" {
		t.Errorf("preview without the module: %+v", file)
	}
	if got := plain.get("/api/files/" + doc.ID + "/pdf"); got.Code != http.StatusNotFound {
		t.Errorf("pdf without the module: %d", got.Code)
	}
}
