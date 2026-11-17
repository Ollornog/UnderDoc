// SPDX-License-Identifier: MIT

package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

// The Office preview is an optional module (ADR-28): LibreOffice headless is far larger than UnderDoc, so it does not go
// into the image. A service of its own converts, Gotenberg by default, whose route for LibreOffice takes a file and
// answers with the PDF; UnderDoc talks to it over HTTP and keeps the result beside the blob.

// MaxPDF is the largest PDF the Office module may answer with.
const MaxPDF = 200 << 20

// officeExtension is the name LibreOffice needs to know what a file is.
var officeExtension = map[string]string{
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   ".docx",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ".xlsx",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
	"application/vnd.oasis.opendocument.text":                                   ".odt",
	"application/vnd.oasis.opendocument.spreadsheet":                            ".ods",
	"application/vnd.oasis.opendocument.presentation":                           ".odp",
	"application/msword":            ".doc",
	"application/vnd.ms-excel":      ".xls",
	"application/vnd.ms-powerpoint": ".ppt",
}

// Gotenberg converts through a Gotenberg service at URL, such as http://127.0.0.1:3000.
type Gotenberg struct {
	URL    string
	Client *http.Client
}

// ToPDF sends the file to the route of LibreOffice and writes the PDF it answers with.
func (g Gotenberg) ToPDF(ctx context.Context, in io.Reader, contentType string, out io.Writer) error {
	ext, ok := officeExtension[contentType]
	if !ok {
		return fmt.Errorf("%w: %s", ErrNoPreview, contentType)
	}
	body, writer := io.Pipe()
	form := multipart.NewWriter(writer)
	go func() {
		part, err := form.CreateFormFile("files", "document"+ext)
		if err == nil {
			_, err = io.Copy(part, in)
		}
		if err == nil {
			err = form.Close()
		}
		_ = writer.CloseWithError(err)
	}()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(g.URL, "/")+"/forms/libreoffice/convert", body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	client := g.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return errors.New("office module: " + response.Status)
	}
	n, err := io.Copy(out, io.LimitReader(response.Body, MaxPDF+1))
	if err == nil && n > MaxPDF {
		err = fmt.Errorf("office module: more than %d bytes", MaxPDF)
	}
	return err
}
