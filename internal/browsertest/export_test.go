//go:build browser

// SPDX-License-Identifier: MIT

package browsertest

import (
	"archive/zip"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// The right click on a collection downloads it as a zip of normal files, as `underdoc export` writes them.
func TestExportACollectionAsFiles(t *testing.T) {
	b, base := setupWithDatabase(t)
	b.open(base + "/o/plan")
	b.ready("Network plan")
	team := makeTeam(b)
	b.open(base + "/o/" + team)
	b.ready("Team")
	downloads := t.TempDir()
	b.call("Page.setDownloadBehavior", map[string]any{"behavior": "allow", "downloadPath": downloads}, nil)
	b.rightClickOn(`.nav3 .cheadrow[data-cdrop="` + team + `"] .chead`)
	item := probe(b, `.ctxpop button`, "Export as files")
	if got := b.eval(`!!document.querySelector(` + "`" + item + "`" + `)?.querySelector('svg')`); got != "true" {
		t.Error("Export as files has no icon")
	}
	b.clickOn(item, 1)
	file := filepath.Join(downloads, "Team.zip")
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(file); err == nil {
			break
		}
		if time.Now().After(deadline) {
			entries, _ := os.ReadDir(downloads)
			t.Fatalf("no Team.zip was downloaded: %v", entries)
		}
		time.Sleep(50 * time.Millisecond)
	}
	var archive *zip.ReadCloser
	var err error
	for time.Now().Before(deadline) {
		if archive, err = zip.OpenReader(file); err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = archive.Close() }()
	var names []string
	for _, f := range archive.File {
		names = append(names, f.Name)
	}
	slices.Sort(names)
	if want := []string{".underdoc.yaml", "Team/.underdoc.yaml", "Team/A.md", "Team/B.md", "Team/C.md"}; !slices.Equal(names, want) {
		t.Errorf("the zip holds %q", names)
	}
	if got := b.eval(`location.pathname`); got != "/o/"+team {
		t.Errorf("the page stays where it was: %s", got)
	}
}
