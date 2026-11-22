// SPDX-License-Identifier: MIT

//go:build office

package files

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTheOfficeModuleConverts sends a real spreadsheet through a real Gotenberg the way the preview does, and reads the
// text of the PDF it answers with. scripts/check-services.sh starts the Gotenberg of deploy/compose.yml and names it in
// UNDERDOC_TEST_OFFICE_URL; without it this test fails, because a skipped test is not a green one.
func TestTheOfficeModuleConverts(t *testing.T) {
	url := os.Getenv("UNDERDOC_TEST_OFFICE_URL")
	if url == "" {
		t.Fatal("UNDERDOC_TEST_OFFICE_URL is not set: run scripts/check-services.sh")
	}
	in, err := os.ReadFile(filepath.Join("..", "sheet", "testdata", "libreoffice", "budget.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var out bytes.Buffer
	xlsx := "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	if err := (Gotenberg{URL: url}).ToPDF(ctx, bytes.NewReader(in), xlsx, &out); err != nil {
		t.Fatalf("the office module does not convert: %v", err)
	}
	if !bytes.HasPrefix(out.Bytes(), []byte("%PDF-")) {
		t.Fatalf("the answer is not a PDF: %.40q", out.String())
	}
	text, err := ServerText(ctx, bytes.NewReader(out.Bytes()), int64(out.Len()), "application/pdf")
	if err != nil {
		t.Fatalf("the PDF has no text: %v", err)
	}
	for _, want := range []string{"Rent", "Coffee"} {
		if !strings.Contains(text, want) {
			t.Errorf("the PDF does not show %q: %.200q", want, text)
		}
	}
}
