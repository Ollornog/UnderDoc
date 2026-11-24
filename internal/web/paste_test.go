// SPDX-License-Identifier: MIT

package web

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Where a pasted text came from (T-41): the version of the block it lands in names the blocks it was copied from, each
// with the version it has, only those the writer sees; whoever reads the versions sees the addresses of the pages they
// see, with titles, and nothing of the others. A copy stays a copy: nothing else changes.
func TestAPasteNotesWhereItCameFrom(t *testing.T) {
	s := withDatabase(t)
	asBob := merged(bob)
	asBob["Sec-Fetch-Site"] = "same-origin"
	s.getWith("/api/state", asBob) // bob has an account
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	mine := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Anna's"}`, sameSite))
	newPage := func(parent, title string, blocks ...string) string {
		t.Helper()
		p := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"`+title+`","parent":"`+parent+`"}`, sameSite))
		after := ""
		for _, md := range blocks {
			body, _ := json.Marshal(map[string]string{"md": md, "after": after})
			after = decode[apiBlock](t, s.send(http.MethodPost, "/api/pages/"+p.ID+"/blocks", string(body), sameSite)).ID
		}
		return p.ID
	}
	private := newPage(mine.ID, "Private notes", "The core switch sits in rack B.")
	s.send(http.MethodPut, "/api/pages/"+private+"/blocks/b1", `{"md":"The core switch sits in rack B, top."}`, sameSite)
	shared := newPage(team.ID, "Shared", "Rack plan.", "Gone soon.")
	s.send(http.MethodDelete, "/api/pages/"+shared+"/blocks/b2", "", sameSite)
	target := newPage(team.ID, "Target", "Notes.")
	versions := func(page string, as map[string]string) []apiVersion {
		t.Helper()
		var out []apiVersion
		if as["Remote-User"] == "" {
			as = gateHeaders
		}
		if err := json.Unmarshal(s.getWith("/api/pages/"+page+"/versions", as).Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	edit := func(as map[string]string, body string) string {
		t.Helper()
		if got := s.send(http.MethodPut, "/api/pages/"+target+"/blocks/b1", body, as); got.Code != http.StatusOK {
			t.Fatalf("edit = %d %s", got.Code, got.Body.String())
		}
		return versions(target, as)[0].Origin
	}

	// Anna pastes from her private page and from the shared one: both are named, each with its version.
	got := edit(sameSite, `{"md":"Notes. The core switch sits in rack B, top. Rack plan.","copied":["`+private+`#b1","`+shared+`#b1","`+private+`#b1"]}`)
	if want := "copy of " + private + "#b1, version 2; " + shared + "#b1, version 1"; got != want {
		t.Errorf("the origin for Anna:\n%s\nwant\n%s", got, want)
	}
	if v := versions(target, sameSite)[0]; len(v.Places) != 2 || v.Places[0].Title != "Private notes" || v.Places[1].Title != "Shared" {
		t.Errorf("the places Anna sees: %+v", v.Places)
	}
	// Bob may read the versions of Target, not Anna's private page: its address is not told, the shared one is.
	if v := versions(target, asBob)[0]; v.Origin != "copy of a page you do not see, version 2; "+shared+"#b1, version 1" || len(v.Places) != 1 || v.Places[0].Ref != shared+"#b1" {
		t.Errorf("the origin for Bob: %+v", v)
	}

	// What the writer does not see, what is not there and what is no block names nothing.
	for name, copied := range map[string]string{
		"a page Bob does not see": `["` + private + `#b1"]`,
		"a block that is not":     `["` + shared + `#b9"]`,
		"a block that went":       `["` + shared + `#b2"]`,
		"a part":                  `["` + shared + `#b1.t1"]`,
		"a whole page":            `["` + shared + `"]`,
		"no address":              `["<script>"]`,
		"an object that is not":   `["aaaaaaaaaa#b1"]`,
	} {
		if got := edit(asBob, `{"md":"Bob writes `+name+`.","copied":`+copied+`}`); got != "" {
			t.Errorf("%s: origin %q", name, got)
		}
	}
	// Pasting from anywhere else sends nothing, and the version has no origin.
	if got := edit(sameSite, `{"md":"From elsewhere."}`); got != "" {
		t.Errorf("without a copy: origin %q", got)
	}
	// A new block takes its origin as well; the source is unchanged, tied to nothing.
	body, _ := json.Marshal(map[string]any{"md": "Rack plan.", "after": "b1", "copied": []string{shared + "#b1"}})
	added := decode[apiBlock](t, s.send(http.MethodPost, "/api/pages/"+target+"/blocks", string(body), sameSite))
	if v := versions(target, sameSite)[0]; v.Block != added.ID || v.Origin != "copy of "+shared+"#b1, version 1" {
		t.Errorf("the origin of a new block: %+v", v)
	}
	if got := decode[apiDocument](t, s.get("/api/doc/"+shared)).Blocks[0].UsedIn; len(got) != 0 {
		t.Errorf("a copy ties the source to it: %q", got)
	}
}
