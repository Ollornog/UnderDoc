// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/search/opensearch"
	"github.com/Ollornog/UnderDoc/internal/search/searchtest"
)

// The archive of spaces and the preset Members through the API, MCP, the search and upload links (T-34): the attack
// round on every door.

var (
	annasKey      = merged2(sameSite, map[string]string{"Remote-Auth": "apikey"})
	annasPassword = merged2(sameSite, map[string]string{"Remote-Auth": "app-password"})
	annasOAuth    = merged2(sameSite, map[string]string{"Remote-Auth": "oauth"})
	// dora reads nothing of anna's spaces.
	doraOf = map[string]string{"Remote-Auth": "session", "Remote-Kind": "user", "Remote-User": "dora", "Remote-Id": "1009", "Remote-Name": "Dora",
		"Remote-Groups": "", "Sec-Fetch-Site": "same-origin"}
)

func archive(s *site, id string, on bool, headers map[string]string) int {
	body := `{"archived":false}`
	if on {
		body = `{"archived":true}`
	}
	return s.send(http.MethodPut, "/api/objects/"+id+"/archive", body, headers).Code
}

func hasNode(nodes []apiNode, id string) (apiNode, bool) {
	i := slices.IndexFunc(nodes, func(n apiNode) bool { return n.ID == id })
	if i < 0 {
		return apiNode{}, false
	}
	return nodes[i], true
}

// Attack: archiving and restoring by an agent, a key, an app password, a service account with the role of
// administrators, an editor, the holder of Members, another site; writing into the archive through the API and MCP; the
// search and the tree of MCP showing the archive unasked, or to someone who may not read it.
func TestArchiveThroughTheAPIAndMCP(t *testing.T) {
	s := withSearch(t, meiliEngine(t), gateOf(t, account.GateConfig{ServiceAccounts: []string{"4242"}}))
	s.send(http.MethodGet, "/api/state", "", plain)
	s.send(http.MethodGet, "/api/state", "", carlOf)
	s.send(http.MethodGet, "/api/state", "", doraOf)
	s.send(http.MethodGet, "/api/state", "", serviceWithRoles)
	team := s.sharedCollection(`{"title":"Filiale"}`, sameSite)
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kassenhandbuch","parent":"`+team.ID+`"}`, sameSite))
	addBlocks(t, s.site, page.ID, "Die Kartenzahlung beginnt mit der Taste Karte.")
	if got := s.send(http.MethodPut, "/api/objects/"+team.ID+"/grants", `{"kind":"person","who":"1003","role":"members"}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("Members for carl = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/objects/"+team.ID+"/grants", `{"kind":"person","who":"1009","actions":{"read":"deny"}}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("no read for dora = %d", got)
	}
	s.drain()
	if got := s.find("q=Kartenzahlung", nil); len(got.Hits) != 1 {
		t.Fatalf("found before: %+v", got.Hits)
	}

	for name, headers := range map[string]map[string]string{
		"an agent of the owner":        annasOAuth,
		"a key of the owner":           annasKey,
		"an app password of the owner": annasPassword,
		"a service account":            serviceWithRoles,
		"an editor":                    plain,
		"the holder of Members":        carlOf,
		"another site":                 {"Sec-Fetch-Site": "cross-site"},
		"no site at all":               {},
	} {
		if got := archive(s.site, team.ID, true, headers); got != http.StatusForbidden {
			t.Errorf("%s archives = %d", name, got)
		}
	}
	if got := s.send(http.MethodPut, "/api/objects/"+page.ID+"/archive", `{"archived":true}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("a page is archived = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/objects/"+team.ID+"/archive", `{}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("archiving without saying it = %d", got)
	}
	state := decode[apiState](t, s.get("/api/state"))
	if n, ok := hasNode(state.Collections, team.ID); !ok || !n.Archive || n.Archived {
		t.Errorf("the owner is not offered to archive: %+v", n)
	}
	if n, _ := hasNode(decode[apiState](t, s.getWith("/api/state", merged(carlOf))).Collections, team.ID); n.Archive {
		t.Error("the holder of Members is offered to archive")
	}
	if got := archive(s.site, team.ID, true, sameSite); got != http.StatusNoContent {
		t.Fatalf("the owner archives = %d", got)
	}

	// The tree and the search leave it out unless asked for, for whoever may read it, and never show it to dora.
	for name, headers := range map[string]map[string]string{"anna": sameSite, "bob": plain, "carl": carlOf} {
		if _, ok := hasNode(decode[apiState](t, s.getWith("/api/state", merged(headers))).Collections, team.ID); ok {
			t.Errorf("%s sees the archive in the tree", name)
		}
		if n, ok := hasNode(decode[apiState](t, s.getWith("/api/state?archive=1", merged(headers))).Collections, team.ID); !ok || !n.Archived {
			t.Errorf("%s does not see the archive when asking", name)
		}
		if got := s.find("q=Kartenzahlung", headers); len(got.Hits) != 0 || len(got.Facets["collection"]) != 0 {
			t.Errorf("%s finds the archive unasked: %+v %+v", name, got.Hits, got.Facets)
		}
		if got := s.find("q=Kartenzahlung&archive=1", headers); len(got.Hits) != 1 {
			t.Errorf("%s does not find the archive when asking: %+v", name, got.Hits)
		}
		if got := s.find("q=Kartenzahlung&collection="+team.ID, headers); len(got.Hits) != 1 {
			t.Errorf("%s does not find the archive when naming it: %+v", name, got.Hits)
		}
	}
	if _, ok := hasNode(decode[apiState](t, s.getWith("/api/state?archive=1", merged(doraOf))).Collections, team.ID); ok {
		t.Error("dora sees the archive")
	}
	if got := s.find("q=Kartenzahlung&archive=1", doraOf); len(got.Hits) != 0 || len(got.Facets["collection"]) != 0 || len(got.Facets["type"]) != 0 {
		t.Errorf("the search tells dora of the archive: %+v %+v", got.Hits, got.Facets)
	}
	doc := decode[apiDocument](t, s.getWith("/api/doc/"+page.ID, merged(plain)))
	if !doc.Archived || doc.CanEdit || doc.CanComment {
		t.Errorf("a page of the archive for bob: archived %v, edit %v, comment %v", doc.Archived, doc.CanEdit, doc.CanComment)
	}
	if top := decode[apiDocument](t, s.get("/api/doc/"+team.ID)); !top.Archived || !top.Archive {
		t.Errorf("the overview of the space: %+v", top)
	}

	// Writing through the API, for the owner too.
	for name, call := range map[string][3]string{
		"a new page":   {http.MethodPost, "/api/pages", `{"title":"Neu","parent":"` + team.ID + `"}`},
		"renaming":     {http.MethodPatch, "/api/objects/" + page.ID, `{"title":"Anders"}`},
		"a new block":  {http.MethodPost, "/api/pages/" + page.ID + "/blocks", `{"after":"","md":"Mehr."}`},
		"a comment":    {http.MethodPost, "/api/pages/" + page.ID + "/comments", `{"body":"Frage"}`},
		"the trash":    {http.MethodDelete, "/api/objects/" + page.ID, ``},
		"a share link": {http.MethodPost, "/api/objects/" + page.ID + "/links", `{"mode":"read"}`},
		"moving it":    {http.MethodPost, "/api/objects/" + team.ID + "/move", `{"parent":""}`},
	} {
		got := s.send(call[0], call[1], call[2], sameSite)
		if got.Code != http.StatusForbidden {
			t.Errorf("%s in the archive = %d %s", name, got.Code, got.Body.String())
		}
	}
	if got := s.send(http.MethodPost, "/api/pages", `{"title":"Neu","parent":"`+team.ID+`"}`, sameSite); !strings.Contains(got.Body.String(), "archived") {
		t.Errorf("the refusal does not say why: %s", got.Body.String())
	}

	// MCP: the tree and the search leave it out until asked for; writing is refused.
	session := mcpSession(t, s.http, annasAgent)
	if got, _ := call(t, session, "context", nil); strings.Contains(got, team.ID) || !strings.Contains(got, "1 archived spaces are left out") {
		t.Errorf("context shows the archive unasked:\n%s", got)
	}
	if got, _ := call(t, session, "context", map[string]any{"archived": true}); !strings.Contains(got, "Filiale underdoc:"+team.ID+" (archived, read only)") {
		t.Errorf("context with the archive:\n%s", got)
	}
	if got, _ := call(t, session, "search", map[string]any{"query": "Kartenzahlung"}); strings.Contains(got, page.ID) {
		t.Errorf("search finds the archive unasked: %s", got)
	}
	if got, _ := call(t, session, "search", map[string]any{"query": "Kartenzahlung", "archived": true}); !strings.Contains(got, page.ID) {
		t.Errorf("search does not find the archive when asked: %s", got)
	}
	if got, failed := call(t, session, "read", map[string]any{"pages": []string{"underdoc:" + page.ID}}); failed || !strings.Contains(got, "Kartenzahlung") {
		t.Errorf("an agent does not read the archive: %s", got)
	}
	for tool, args := range map[string]map[string]any{
		"create_page": {"parent": "underdoc:" + team.ID, "title": "Neu"},
		"trash":       {"object": "underdoc:" + page.ID},
	} {
		if got, failed := call(t, session, tool, args); !failed || !strings.Contains(got, "archived") {
			t.Errorf("%s in the archive: %s", tool, got)
		}
	}
	if dorasAgent := mcpSession(t, s.http, merged2(doraOf, map[string]string{"Remote-Auth": "oauth"})); dorasAgent != nil {
		if got, _ := call(t, dorasAgent, "context", map[string]any{"archived": true}); strings.Contains(got, team.ID) || strings.Contains(got, "archived spaces") {
			t.Errorf("dora's agent learns of the archive:\n%s", got)
		}
	}

	// Restoring: the same people as archiving.
	for name, headers := range map[string]map[string]string{"an agent": annasOAuth, "an editor": plain, "the holder of Members": carlOf} {
		if got := archive(s.site, team.ID, false, headers); got != http.StatusForbidden {
			t.Errorf("%s restores = %d", name, got)
		}
	}
	if got := archive(s.site, team.ID, false, sameSite); got != http.StatusNoContent {
		t.Fatalf("the owner restores = %d", got)
	}
	if got := s.find("q=Kartenzahlung", plain); len(got.Hits) != 1 {
		t.Errorf("restored, the search finds it again: %+v", got.Hits)
	}
	if got := s.send(http.MethodPost, "/api/pages", `{"title":"Neu","parent":"`+team.ID+`"}`, plain).Code; got != http.StatusCreated {
		t.Errorf("restored, bob writes again = %d", got)
	}
}

// The archive filter of the search holds in OpenSearch as in Meilisearch.
func TestArchiveIsLeftOutOfOpenSearch(t *testing.T) {
	s := withSearch(t, opensearch.New(searchtest.URL(t, searchtest.EnvOpenSearch), "", "", searchtest.Prefix()))
	team := s.sharedCollection(`{"title":"Filiale"}`, sameSite)
	other := s.sharedCollection(`{"title":"Zentrale"}`, sameSite)
	for _, parent := range []string{team.ID, other.ID} {
		page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Kassenhandbuch","parent":"`+parent+`"}`, sameSite))
		addBlocks(t, s.site, page.ID, "Die Kartenzahlung beginnt mit der Taste Karte.")
	}
	if got := archive(s.site, team.ID, true, sameSite); got != http.StatusNoContent {
		t.Fatalf("archive = %d", got)
	}
	s.drain()
	if got := s.find("q=Kartenzahlung", nil); len(got.Hits) != 1 || len(got.Facets["collection"]) != 1 || got.Facets["collection"][0].Value != other.ID {
		t.Errorf("OpenSearch shows the archive unasked: %+v %+v", got.Hits, got.Facets["collection"])
	}
	if got := s.find("q=Kartenzahlung&archive=1", nil); len(got.Hits) != 2 {
		t.Errorf("OpenSearch with the archive: %+v", got.Hits)
	}
}

// Attack: an upload into an archived collection of Files through an upload link made before, a new one, a share link
// to upload, and the list of the roots.
func TestArchiveOfFilesAndUploadLinks(t *testing.T) {
	s := withFiles(t)
	library := decode[apiNode](t, s.send(http.MethodPost, "/api/files/collections", `{"title":"Ablage"}`, sameSite))
	session := mcpSession(t, s.http, annasAgent)
	got, failed := call(t, session, "upload_link", map[string]any{"in": library.ID, "name": "spaet.txt", "size": 4})
	link := uploadLink.FindString(got)
	if failed || link == "" {
		t.Fatalf("upload_link: %s", got)
	}
	share := s.send(http.MethodPost, "/api/objects/"+library.ID+"/links", `{"mode":"upload"}`, sameSite)
	if share.Code != http.StatusCreated {
		t.Fatalf("an upload share link = %d %s", share.Code, share.Body.String())
	}
	token := decode[struct {
		URL string `json:"url"`
	}](t, share).URL
	sharePath := token[strings.Index(token, "/s/"):]
	if got := s.getWith(sharePath, nil); got.Code != http.StatusOK {
		t.Fatalf("the upload share link before the archive = %d", got.Code)
	}
	if got := archive(s.site, library.ID, true, sameSite); got != http.StatusNoContent {
		t.Fatalf("archive = %d", got)
	}
	if got := s.getWith(sharePath, nil); got.Code == http.StatusOK {
		t.Errorf("the upload share link answers in the archive: %d", got.Code)
	}
	if got := putLink(s, link, []byte("spat"), nil); got.Code != http.StatusForbidden {
		t.Errorf("an upload link made before the archive = %d %s", got.Code, got.Body.String())
	}
	if got, failed := call(t, session, "upload_link", map[string]any{"in": library.ID, "name": "x.txt", "size": 1}); !failed || !strings.Contains(got, "archived") {
		t.Errorf("a new upload link into the archive: %s", got)
	}
	roots := decode[rootsAnswer](t, s.get("/api/files"))
	if slices.ContainsFunc(roots.Roots, func(r apiRoot) bool { return r.ID == library.ID }) {
		t.Error("the archived collection is among the roots")
	}
	roots = decode[rootsAnswer](t, s.get("/api/files?archive=1"))
	if i := slices.IndexFunc(roots.Roots, func(r apiRoot) bool { return r.ID == library.ID }); i < 0 || !roots.Roots[i].Archived || roots.Roots[i].CanEdit || !roots.Roots[i].Archive {
		t.Errorf("the archive among the roots when asked: %+v", roots.Roots)
	}
	if got := s.send(http.MethodPost, "/api/uploads", `{"parent":"`+library.ID+`","name":"a.txt","size":1}`, sameSite).Code; got != http.StatusForbidden {
		t.Errorf("an upload in the browser into the archive = %d", got)
	}
}

// Attack: the holder of Members gives edit or manage, gives delete to the whole tenant, restores the inheritance,
// deletes, through the API.
func TestMembersThroughTheAPI(t *testing.T) {
	s := withDatabase(t)
	s.send(http.MethodGet, "/api/state", "", plain)
	s.send(http.MethodGet, "/api/state", "", carlOf)
	team := decode[apiNode](t, s.send(http.MethodPost, "/api/collections", `{"title":"Team"}`, sameSite))
	folder := decode[apiNode](t, s.send(http.MethodPost, "/api/folders", `{"title":"Plans","parent":"`+team.ID+`"}`, sameSite))
	if got := s.send(http.MethodPut, "/api/objects/"+team.ID+"/grants", `{"kind":"person","who":"1003","role":"members"}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("Members for carl = %d", got)
	}
	rights := decode[apiRights](t, s.getWith("/api/objects/"+team.ID+"/rights", merged(carlOf)))
	if !rights.Manage || slices.Contains(rights.Actions, "delete") {
		t.Errorf("carl's actions: %+v", rights.Actions)
	}
	for _, body := range []string{`{"kind":"person","who":"1002","role":"read"}`, `{"kind":"person","who":"1002","role":"members"}`, `{"kind":"person","who":"1002","role":""}`} {
		if got := s.send(http.MethodPut, "/api/objects/"+team.ID+"/grants", body, carlOf).Code; got != http.StatusNoContent {
			t.Errorf("Members %s = %d", body, got)
		}
	}
	for name, call := range map[string][3]string{
		"edit for bob":       {http.MethodPut, "/api/objects/" + team.ID + "/grants", `{"kind":"person","who":"1002","role":"edit"}`},
		"manage for bob":     {http.MethodPut, "/api/objects/" + team.ID + "/grants", `{"kind":"person","who":"1002","role":"manage"}`},
		"delete for himself": {http.MethodPut, "/api/objects/" + team.ID + "/grants", `{"kind":"person","who":"1003","actions":{"read":"allow","manage":"allow","delete":"allow"}}`},
		"edit for everyone":  {http.MethodPost, "/api/objects/" + folder.ID + "/access", `{"access":"edit"}`},
		"the trash":          {http.MethodDelete, "/api/objects/" + folder.ID, ``},
		"the archive":        {http.MethodPut, "/api/objects/" + team.ID + "/archive", `{"archived":true}`},
	} {
		if got := s.send(call[0], call[1], call[2], carlOf); got.Code != http.StatusForbidden {
			t.Errorf("Members: %s = %d %s", name, got.Code, got.Body.String())
		}
	}
	if got := s.send(http.MethodPut, "/api/objects/"+folder.ID+"/inherit", `{"inherit":false}`, carlOf).Code; got != http.StatusNoContent {
		t.Errorf("Members breaks the inheritance = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/objects/"+folder.ID+"/inherit", `{"inherit":true}`, carlOf).Code; got != http.StatusForbidden {
		t.Errorf("Members restores the inheritance = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/objects/"+team.ID+"/owner", `{"to":"1003"}`, annasOAuth).Code; got != http.StatusForbidden {
		t.Errorf("an agent of the owner transfers = %d", got)
	}
}
