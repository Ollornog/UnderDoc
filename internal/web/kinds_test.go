// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/pagekind"
)

func codesOf(found []pagekind.Finding) []string {
	var out []string
	for _, f := range found {
		out = append(out, f.Code)
	}
	return out
}

// TestPageKindsThroughTheAPI: a page gets its kind when made and later, the answer of every write names the warnings
// with their fixes, an error refuses the write, the page says its kind and side, and agents read the raw page (T-20).
func TestPageKindsThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	s.send(http.MethodGet, "/api/state", "", plain)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	ref := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Network","parent":"`+team.ID+`","kind":"reference"}`, sameSite))
	addBlocks(t, s, ref.ID, "## Backup\n\nThe backup host is backup.example.com.")
	if got := s.send(http.MethodPost, "/api/pages", `{"title":"Odd","parent":"`+team.ID+`","kind":"manual"}`, sameSite); got.Code != http.StatusBadRequest {
		t.Errorf("an unknown kind = %d", got.Code)
	}
	guide := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Restore","parent":"`+team.ID+`","template":"builtin:howto"}`, sameSite))
	if p := page(t, s, guide.ID); p.PageKind != pagekind.HowTo || p.Audience != pagekind.AudiencePeople {
		t.Errorf("a page from the template How-to is a how-to for people: %q %q", p.PageKind, p.Audience)
	}
	blocks := "/api/pages/" + guide.ID + "/blocks"
	added := decode[apiBlock](t, s.send(http.MethodPost, blocks, `{"after":"b4","md":"Files come from backup.example.com."}`, sameSite))
	if got := codesOf(added.Findings); !slices.Contains(got, pagekind.CodeCopy) {
		t.Errorf("the answer of a write names the copied value: %v", got)
	}
	for _, f := range added.Findings {
		if f.Code == pagekind.CodeCopy && (f.Block != added.ID || !strings.HasPrefix(f.Source, ref.ID+"#") || !strings.Contains(f.Fix, "Embed instead of copying")) {
			t.Errorf("COPY with its block, source and fix: %+v", f)
		}
	}
	refused := s.send(http.MethodPut, blocks+"/b1", `{"md":"---\ntitle: Restore\nkind: how-to\n---\nText."}`, sameSite)
	answer := decode[struct {
		Error    string             `json:"error"`
		Findings []pagekind.Finding `json:"findings"`
	}](t, refused)
	if refused.Code != http.StatusBadRequest || len(answer.Findings) != 1 || answer.Findings[0].Code != pagekind.CodeFrontMatter ||
		answer.Findings[0].Level != pagekind.Error || !strings.Contains(answer.Error, "front matter") {
		t.Errorf("front matter is refused with its finding: %d %s", refused.Code, refused.Body.String())
	}
	if got := page(t, s, guide.ID).Blocks[0].Markdown; got != "What this does, in one sentence." {
		t.Errorf("the refused text was not written: %q", got)
	}

	set := s.send(http.MethodPut, "/api/pages/"+guide.ID+"/kind", `{"kind":"Explanation"}`, sameSite)
	kind := decode[struct {
		Kind     string             `json:"kind"`
		Audience string             `json:"audience"`
		Findings []pagekind.Finding `json:"findings"`
	}](t, set)
	if set.Code != http.StatusOK || kind.Kind != pagekind.Explanation || kind.Audience != pagekind.AudiencePeople || kind.Findings == nil {
		t.Errorf("set the kind: %d %s", set.Code, set.Body.String())
	}
	if got := s.send(http.MethodPut, "/api/pages/"+guide.ID+"/kind", `{"kind":"notes"}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("notes is no kind = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/pages/"+team.ID+"/kind", `{"kind":"how-to"}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("a collection takes no kind = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/pages/"+guide.ID+"/kind", `{"kind":"how-to"}`, map[string]string{"Sec-Fetch-Site": "cross-site"}).Code; got != http.StatusForbidden {
		t.Errorf("from another site = %d", got)
	}

	// The raw page: front matter with the kind, and the marker of each block.
	raw := s.get("/api/pages/" + guide.ID + "/raw")
	if raw.Code != http.StatusOK || !strings.HasPrefix(raw.Header().Get("Content-Type"), "text/markdown") ||
		!strings.Contains(raw.Body.String(), "\nkind: explanation\n") || !strings.Contains(raw.Body.String(), "<!-- block b1 -->") {
		t.Errorf("the raw page: %d %s", raw.Code, raw.Body.String())
	}
	// People see neither: the page answers its description and blocks, without front matter or markers.
	for _, b := range page(t, s, guide.ID).Blocks {
		if strings.Contains(b.HTML, "kind:") || strings.Contains(b.HTML, "<!--") {
			t.Errorf("a block for people holds machine parts: %s", b.HTML)
		}
	}

	// A reader sees the kind, not the findings, and sets no kind.
	s.send(http.MethodPost, "/api/objects/"+guide.ID+"/access", `{"access":"read"}`, sameSite)
	s.send(http.MethodPut, "/api/objects/"+guide.ID+"/inherit", `{"inherit":false}`, sameSite)
	s.send(http.MethodPut, "/api/objects/"+guide.ID+"/grants", `{"kind":"tenant","role":"read"}`, sameSite)
	read := decode[apiDocument](t, s.getWith("/api/doc/"+guide.ID, plain))
	if read.CanEdit || read.PageKind != pagekind.Explanation || read.Findings != nil {
		t.Errorf("a reader: canEdit %v kind %q findings %v", read.CanEdit, read.PageKind, read.Findings)
	}
	if got := s.send(http.MethodPut, "/api/pages/"+guide.ID+"/kind", `{"kind":"how-to"}`, plain).Code; got != http.StatusForbidden {
		t.Errorf("a reader sets the kind = %d", got)
	}
	if got := s.getWith("/api/pages/"+guide.ID+"/raw", plain).Code; got != http.StatusOK {
		t.Errorf("a reader reads the raw page = %d", got)
	}
	mine := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Mine"}`, sameSite))
	hidden := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Hidden","parent":"`+mine.ID+`"}`, sameSite))
	if got := s.getWith("/api/pages/"+hidden.ID+"/raw", plain).Code; got != http.StatusNotFound {
		t.Errorf("the raw page of a page one does not see = %d", got)
	}
	if got := s.get("/api/pages/" + team.ID + "/raw").Code; got != http.StatusNotFound {
		t.Errorf("a collection has no raw page = %d", got)
	}

	// The list of problems names the pages that break their rules.
	problems := decode[apiProblems](t, s.get("/api/problems"))
	at := slices.IndexFunc(problems.Rules, func(p apiRuleProblem) bool { return p.Page == guide.ID })
	if at < 0 || problems.Rules[at].Kind != pagekind.Explanation || !slices.Contains(codesOf(problems.Rules[at].Findings), pagekind.CodeCopy) {
		t.Errorf("the problems name the explanation with its copy: %+v", problems.Rules)
	}
	templates := decode[[]apiTemplate](t, s.get("/api/templates"))
	if templates[1].ID != "builtin:howto" || templates[1].Kind != pagekind.HowTo {
		t.Errorf("a template names its kind: %+v", templates[1])
	}
}
