// SPDX-License-Identifier: MIT

package web

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func unzip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range archive.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, _ := io.ReadAll(r)
		_ = r.Close()
		out[f.Name] = string(content)
	}
	return out
}

func TestExportAsZip(t *testing.T) {
	s := withDatabase(t)
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team: ops"}`, sameSite))
	plan := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Plan","parent":"`+team.ID+`"}`, sameSite))
	s.send(http.MethodPost, "/api/pages/"+plan.ID+"/blocks", `{"md":"## Switches"}`, sameSite)
	line := decode[apiNode](t, s.send(http.MethodPost, "/api/dividers", `{"kind":"line","parent":"`+team.ID+`","after":"`+plan.ID+`"}`, sameSite))

	got := s.getWith("/api/objects/"+team.ID+"/export", bob)
	if got.Code != http.StatusOK || got.Header().Get("Content-Type") != "application/zip" ||
		got.Header().Get("Content-Disposition") != `attachment; filename="Team_ ops.zip"` {
		t.Fatalf("export = %d %v", got.Code, got.Header())
	}
	files := unzip(t, got.Body.Bytes())
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{".underdoc.yaml", "Team_ ops/.underdoc.yaml", "Team_ ops/Plan.md"}) {
		t.Errorf("files: %q", names)
	}
	if page := files["Team_ ops/Plan.md"]; !strings.Contains(page, "id: "+plan.ID) || !strings.Contains(page, "<!-- block b1 -->\n## Switches\n") {
		t.Errorf("the page:\n%s", page)
	}
	if !strings.Contains(files["Team_ ops/.underdoc.yaml"], "line: "+line.ID) {
		t.Errorf("the order keeps the line:\n%s", files["Team_ ops/.underdoc.yaml"])
	}
	if page := s.get("/api/objects/" + plan.ID + "/export"); page.Code != http.StatusOK || unzip(t, page.Body.Bytes())["Plan.md"] == "" {
		t.Errorf("a page alone = %d", page.Code)
	}
	notes := decode[apiState](t, s.get("/api/state")).Collections[0]
	if got := s.getWith("/api/objects/"+notes.ID+"/export", bob).Code; got != http.StatusNotFound {
		t.Errorf("Anna's notes for Bob = %d", got)
	}
	if got := s.get("/api/objects/" + line.ID + "/export").Code; got != http.StatusNotFound {
		t.Errorf("a line = %d", got)
	}
	if got := newSite(t, Options{}, sample).get("/api/objects/" + team.ID + "/export").Code; got != http.StatusNotImplemented {
		t.Errorf("without the database = %d", got)
	}
}
