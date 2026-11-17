// SPDX-License-Identifier: MIT

package files

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif" // decoders, registered by their import
	"image/jpeg"
	_ "image/png"
	"io"

	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// Previews (ADR-28): a small picture of an image, made on the server with Go alone, so the image of UnderDoc needs no
// libraries of the system. A preview is made once, when it is first asked for, and kept in the store beside its blob.

// MaxPixels is the largest image a preview is made of. An image says its size in its header, so one that would unpack to
// gigabytes from a few bytes (a decompression bomb) is refused before a single pixel is decoded.
const MaxPixels = 50_000_000

// ThumbSizes are the edges a preview may have; others are refused, so a request cannot fill the store with sizes.
var ThumbSizes = []int{256, 1024}

// ErrNoPreview is content that has no preview: not an image, or too large.
var ErrNoPreview = errors.New("no preview of this content")

// Thumbnail reads an image and returns a JPEG whose longer edge is at most size, never larger than the image itself.
func Thumbnail(r io.ReadSeeker, size int) ([]byte, error) {
	config, _, err := image.DecodeConfig(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoPreview, err)
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > MaxPixels {
		return nil, fmt.Errorf("%w: %dx%d pixels", ErrNoPreview, config.Width, config.Height)
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	src, _, err := image.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoPreview, err)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > size || h > size {
		if w >= h {
			w, h = size, max(1, h*size/w)
		} else {
			w, h = max(1, w*size/h), size
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	// Transparent parts show white, as JPEG has no transparency.
	draw.Draw(dst, dst.Bounds(), image.White, image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: 82}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// OfficeConverter turns an Office file into a PDF for its preview. LibreOffice headless does it as an optional module
// next to UnderDoc; without one, Office files have no preview.
type OfficeConverter interface {
	// ToPDF reads an Office file and writes the PDF of it.
	ToPDF(ctx context.Context, in io.Reader, contentType string, out io.Writer) error
}

// IsOffice tells whether a type is one an OfficeConverter takes.
func IsOffice(contentType string) bool {
	switch contentType {
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"application/vnd.oasis.opendocument.text", "application/vnd.oasis.opendocument.spreadsheet",
		"application/vnd.oasis.opendocument.presentation", "application/msword", "application/vnd.ms-excel",
		"application/vnd.ms-powerpoint":
		return true
	}
	return false
}
