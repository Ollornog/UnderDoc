// SPDX-License-Identifier: MIT

package share

import (
	"crypto/rand"
	"encoding/base64"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
)

// TestNoPasswordOnTheShare: a password in the text of a page (ADR-7) never reaches the working copy: pages are not on the
// share, and nothing of a secret is written there, neither its value nor its reference, while the files beside it are.
func TestNoPasswordOnTheShare(t *testing.T) {
	f := setup(t)
	ctx := inMain()
	raw := make([]byte, 18)
	_, _ = rand.Read(raw)
	value := "pw-" + base64.RawURLEncoding.EncodeToString(raw)
	team, err := f.db.CreateCollection(ctx, anna, "Team", "")
	if err != nil {
		t.Fatal(err)
	}
	page, err := f.db.CreatePage(ctx, anna, "Router", team.ID)
	if err != nil {
		t.Fatal(err)
	}
	secretID, err := f.db.CreatePageSecret(ctx, page.ID, anna.ID, func(name string) ([]byte, error) {
		return f.m.vault.Seal(tenant.Main, name, vault.Secret(value))
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.AddBlock(ctx, page.ID, anna.ID, "", "Router admin: ::secret{id=\""+secretID+"\"}"); err != nil {
		t.Fatal(err)
	}
	f.upload(anna.ID, f.company.ID, "notes.txt", "a file beside it")
	f.sync()
	files := 0
	err = filepath.WalkDir(f.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		files++
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, leak := range []string{value, secretID, "Router admin"} {
			if strings.Contains(string(content), leak) || strings.Contains(path, leak) {
				t.Errorf("%s holds %q", path, leak)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("nothing on the share at all: the test proves nothing")
	}
}
