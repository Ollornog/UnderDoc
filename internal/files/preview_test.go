// SPDX-License-Identifier: MIT

package files

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		img.Set(x, h/2, color.NRGBA{R: 200, A: 255})
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestThumbnail(t *testing.T) {
	out, err := Thumbnail(bytes.NewReader(pngOf(t, 2000, 500)), 256)
	if err != nil {
		t.Fatal(err)
	}
	got, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil || got.Width != 256 || got.Height != 64 {
		t.Fatalf("%+v %v", got, err)
	}
	// A small image stays its size.
	out, _ = Thumbnail(bytes.NewReader(pngOf(t, 40, 30)), 256)
	if got, _ := jpeg.DecodeConfig(bytes.NewReader(out)); got.Width != 40 || got.Height != 30 {
		t.Fatalf("%+v", got)
	}
	if _, err := Thumbnail(strings.NewReader("%PDF-1.7"), 256); !errors.Is(err, ErrNoPreview) {
		t.Fatalf("not an image: %v", err)
	}
}

// A decompression bomb says in its header that it is huge; it is refused before a pixel is decoded.
func TestThumbnailRefusesBombs(t *testing.T) {
	head := pngOf(t, 1, 1)
	// The IHDR chunk starts at byte 8: length, type, width, height, and a CRC over type and data.
	binary.BigEndian.PutUint32(head[16:], 100000)
	binary.BigEndian.PutUint32(head[20:], 100000)
	binary.BigEndian.PutUint32(head[29:], crc32.ChecksumIEEE(head[12:29]))
	if _, err := Thumbnail(bytes.NewReader(head), 256); !errors.Is(err, ErrNoPreview) || !strings.Contains(err.Error(), "100000x100000") {
		t.Fatalf("a bomb of 10 gigapixels: %v", err)
	}
}

func TestGotenberg(t *testing.T) {
	var name string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/forms/libreoffice/convert" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		part, err := multipart.NewReader(r.Body, params["boundary"]).NextPart()
		if err != nil || part.FormName() != "files" {
			http.Error(w, "no file", http.StatusBadRequest)
			return
		}
		name = part.FileName()
		body, _ := io.ReadAll(part)
		if string(body) != "PK-office" {
			http.Error(w, "wrong content", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte("%PDF-1.7 converted"))
	}))
	defer server.Close()
	g := Gotenberg{URL: server.URL + "/"}
	var out bytes.Buffer
	xlsx := "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	if err := g.ToPDF(context.Background(), strings.NewReader("PK-office"), xlsx, &out); err != nil || out.String() != "%PDF-1.7 converted" || name != "document.xlsx" {
		t.Fatalf("%q %q %v", out.String(), name, err)
	}
	if err := g.ToPDF(context.Background(), strings.NewReader("x"), "image/png", &out); !errors.Is(err, ErrNoPreview) {
		t.Fatalf("not an Office file: %v", err)
	}
	if err := (Gotenberg{URL: server.URL + "/nothing"}).ToPDF(context.Background(), strings.NewReader("PK-office"), xlsx, &out); err == nil {
		t.Fatal("a failing module passed")
	}
}
