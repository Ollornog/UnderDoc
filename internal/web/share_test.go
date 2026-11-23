// SPDX-License-Identifier: MIT

package web

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// anonymous sends a request as someone without an account, from a page of the application.
func (s *site) anonymous(method, target string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	s.t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	s.http.ServeHTTP(recorder, request)
	return recorder
}

// pathOf is the path of an address.
func pathOf(t *testing.T, address string) string {
	t.Helper()
	u, err := url.Parse(address)
	if err != nil || !strings.HasPrefix(u.Path, "/s/") {
		t.Fatalf("not the address of a link: %q", address)
	}
	return u.Path
}

// TestAPageSharedByLink: the page shows frozen, only with the embeddings released when the link was made, also after the
// page changes; renewing freezes it again; a password keeps others out; revoked, the link leads nowhere (ADR-29).
func TestAPageSharedByLink(t *testing.T) {
	s := withDatabase(t)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	newPage := func(title string, blocks ...string) string {
		p := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"`+title+`","parent":"`+team.ID+`"}`, sameSite))
		after := ""
		for _, md := range blocks {
			body, _ := json.Marshal(map[string]string{"md": md, "after": after})
			after = decode[apiBlock](t, s.send(http.MethodPost, "/api/pages/"+p.ID+"/blocks", string(body), sameSite)).ID
		}
		return p.ID
	}
	prices := newPage("Prices", "Coffee costs 2 euros.")
	salaries := newPage("Salaries", "Anna earns 4711.")
	menu := newPage("Menu", "Our menu.", "::embed{ref=\""+prices+"\"}", "::embed{ref=\""+salaries+"\"}")
	can := decode[[]apiShareable](t, s.get("/api/objects/"+menu+"/shareables"))
	if len(can) != 2 {
		t.Fatalf("both embedded pages are listed for release: %+v", can)
	}
	made := s.send(http.MethodPost, "/api/objects/"+menu+"/links", `{"mode":"read","released":["`+prices+`","zzzzzzzzzz"]}`, sameSite)
	if made.Code != http.StatusCreated {
		t.Fatalf("make = %d %s", made.Code, made.Body.String())
	}
	link := decode[apiLink](t, made)
	path := pathOf(t, link.URL)
	if len(link.Released) != 1 || link.Released[0] != prices || link.Creator != "Anna Berg" || link.Password {
		t.Errorf("only what the page embeds is released: %+v", link)
	}
	page := s.anonymous(http.MethodGet, path, nil, nil)
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, "Our menu.") || !strings.Contains(body, "Coffee costs 2 euros.") {
		t.Fatalf("anyone with the link reads the page and what was released: %d %s", page.Code, body)
	}
	if strings.Contains(body, "4711") || !strings.Contains(body, "Not shared: "+salaries) {
		t.Errorf("what was not released is not shown: %s", body)
	}
	if page.Header().Get("Cache-Control") != "no-store" || !strings.Contains(page.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Errorf("headers: %v", page.Header())
	}
	// Frozen: a change of the page or of what it embeds does not show until the link is renewed.
	s.send(http.MethodPut, "/api/pages/"+prices+"/blocks/b1", `{"md":"Coffee costs 3 euros."}`, sameSite)
	if body := s.anonymous(http.MethodGet, path, nil, nil).Body.String(); !strings.Contains(body, "2 euros") || strings.Contains(body, "3 euros") {
		t.Errorf("the page stays as it was frozen: %s", body)
	}
	if got := s.send(http.MethodPost, "/api/links/"+link.ID+"/renew", `{"released":["`+prices+`","`+salaries+`"]}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("renew = %d", got)
	}
	if body := s.anonymous(http.MethodGet, path, nil, nil).Body.String(); !strings.Contains(body, "3 euros") || !strings.Contains(body, "4711") {
		t.Errorf("renewed with both released: %s", body)
	}
	// Someone who only reads the page makes no link, sees none and revokes none.
	if got := s.send(http.MethodPost, "/api/objects/"+menu+"/links", `{"mode":"read"}`, merged2(sameSite, plain)).Code; got != http.StatusForbidden {
		t.Errorf("bob makes a link = %d", got)
	}
	if got := decode[[]apiLink](t, s.send(http.MethodGet, "/api/links", "", plain)); len(got) != 0 {
		t.Errorf("bob sees links: %+v", got)
	}
	if got := s.send(http.MethodDelete, "/api/links/"+link.ID, "", plain).Code; got != http.StatusForbidden {
		t.Errorf("bob revokes = %d", got)
	}
	// A password, with the limit against guessing.
	secret := decode[apiLink](t, s.send(http.MethodPost, "/api/objects/"+menu+"/links", `{"mode":"read","password":"correct horse battery","expires":"2099-12-31"}`, sameSite))
	secretPath := pathOf(t, secret.URL)
	if body := s.anonymous(http.MethodGet, secretPath, nil, nil).Body.String(); strings.Contains(body, "Our menu") || !strings.Contains(body, "This link has a password") {
		t.Errorf("without the password, only the form: %s", body)
	}
	form := func(password string) *httptest.ResponseRecorder {
		return s.anonymous(http.MethodPost, secretPath+"/password", []byte("password="+url.QueryEscape(password)),
			map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	}
	if got := form("wrong").Code; got != http.StatusUnauthorized {
		t.Errorf("a wrong password = %d", got)
	}
	right := form("correct horse battery")
	cookies := right.Result().Cookies()
	if right.Code != http.StatusSeeOther || len(cookies) != 2 || cookies[0].Path != secretPath || !cookies[0].HttpOnly ||
		cookies[1].Path != "/link/"+strings.TrimPrefix(secretPath, "/s/") || cookies[1].Value != cookies[0].Value {
		t.Fatalf("the right password lets the browser in for this link, and where an account takes it: %d %+v", right.Code, cookies)
	}
	with := map[string]string{"Cookie": cookies[0].Name + "=" + cookies[0].Value}
	if body := s.anonymous(http.MethodGet, secretPath, nil, with).Body.String(); !strings.Contains(body, "Our menu") || !strings.Contains(body, "until 2099-12-31") {
		t.Errorf("with the cookie, the page: %s", body)
	}
	if body := s.anonymous(http.MethodGet, path, nil, with).Body.String(); !strings.Contains(body, "Our menu") {
		t.Errorf("the other link has no password: %s", body)
	}
	for range 10 {
		form("wrong")
	}
	if got := form("correct horse battery").Code; got != http.StatusTooManyRequests {
		t.Errorf("after many wrong tries, a wait = %d", got)
	}
	// Listed, then revoked.
	links := decode[[]apiLink](t, s.get("/api/links?object="+menu))
	if len(links) != 2 || links[0].URL != "" {
		t.Errorf("the links of the page, without their addresses: %+v", links)
	}
	if got := s.send(http.MethodDelete, "/api/links/"+link.ID, "", sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("revoke = %d", got)
	}
	if got := s.anonymous(http.MethodGet, path, nil, nil); got.Code != http.StatusNotFound || strings.Contains(got.Body.String(), "menu") {
		t.Errorf("a revoked link leads nowhere: %d", got.Code)
	}
}

// TestALinkToEdit: the address of a link is open at the gate, so nobody is signed in there: it leads to /link/<token>,
// behind the gate, where an account of the tenant takes the role and edits from then on. An agent never takes it, and a
// link to read gives nothing there (ADR-32, ADR-36).
func TestALinkToEdit(t *testing.T) {
	s := withDatabase(t)
	s.send(http.MethodGet, "/api/state", "", plain)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	s.send(http.MethodPost, "/api/objects/"+team.ID+"/access", `{"access":"read"}`, sameSite)
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Plan","parent":"`+team.ID+`"}`, sameSite))
	link := decode[apiLink](t, s.send(http.MethodPost, "/api/objects/"+team.ID+"/links", `{"mode":"edit"}`, sameSite))
	path := pathOf(t, link.URL)
	take := "/link/" + strings.TrimPrefix(path, "/s/")
	if got := s.send(http.MethodPost, "/api/pages/"+page.ID+"/blocks", `{"md":"bob was here"}`, plain).Code; got != http.StatusForbidden {
		t.Fatalf("before the link bob only reads: %d", got)
	}
	if got := s.anonymous(http.MethodGet, path, nil, nil); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "Sign in to edit") ||
		!strings.Contains(got.Body.String(), `href="`+take+`"`) {
		t.Errorf("without an account: %d %s", got.Code, got.Body.String())
	}
	if got := s.anonymous(http.MethodGet, take, nil, nil).Code; got != http.StatusUnauthorized {
		t.Errorf("taking it without the gate = %d", got)
	}
	if opened := s.send(http.MethodGet, path, "", plain); opened.Code != http.StatusOK {
		t.Errorf("the address of the link itself takes nothing: %d", opened.Code)
	}
	agent := merged2(plain, map[string]string{"Remote-Auth": "oauth"})
	if got := s.send(http.MethodGet, take, "", agent); got.Code != http.StatusSeeOther || got.Header().Get("Location") != path {
		t.Errorf("bob's agent takes the link: %d %v", got.Code, got.Header())
	}
	if got := s.send(http.MethodPost, "/api/pages/"+page.ID+"/blocks", `{"md":"bob was here"}`, plain).Code; got != http.StatusForbidden {
		t.Fatalf("after his agent, bob still reads: %d", got)
	}
	opened := s.send(http.MethodGet, take, "", plain)
	if opened.Code != http.StatusSeeOther || opened.Header().Get("Location") != "/o/"+team.ID {
		t.Fatalf("signed in, the link opens the workspace: %d %v", opened.Code, opened.Header())
	}
	if got := s.send(http.MethodPost, "/api/pages/"+page.ID+"/blocks", `{"md":"bob was here"}`, plain).Code; got != http.StatusCreated {
		t.Errorf("bob edits now: %d", got)
	}
	s.send(http.MethodDelete, "/api/links/"+link.ID, "", sameSite)
	if got := s.send(http.MethodPost, "/api/pages/"+page.ID+"/blocks", `{"md":"again"}`, plain).Code; got != http.StatusForbidden {
		t.Errorf("revoked, bob reads again: %d", got)
	}
}

// TestFilesSharedByLink: a link to a folder lists its files and folders and sends their current versions, nothing outside
// it; a link to upload only takes files in under new names and shows nothing.
func TestFilesSharedByLink(t *testing.T) {
	s := withFiles(t)
	tenantRoot := s.root(nil, "tenant")
	folder := decode[apiNode](t, s.send(http.MethodPost, "/api/folders", `{"title":"Offers","parent":"`+tenantRoot.ID+`"}`, sameSite))
	if got := s.put(nil, `"parent":"`+folder.ID+`"`, "offer.txt", []byte("offer one"), 4); got.Code != http.StatusCreated {
		t.Fatalf("upload = %d %s", got.Code, got.Body.String())
	}
	outside := decode[finished](t, s.put(nil, `"parent":"`+tenantRoot.ID+`"`, "outside.txt", []byte("not shared"), 4))
	read := decode[apiLink](t, s.send(http.MethodPost, "/api/objects/"+folder.ID+"/links", `{"mode":"read"}`, sameSite))
	path := pathOf(t, read.URL)
	list := s.anonymous(http.MethodGet, path, nil, nil).Body.String()
	if !strings.Contains(list, "offer.txt") || strings.Contains(list, "outside.txt") {
		t.Fatalf("the folder lists its files only: %s", list)
	}
	start := strings.Index(list, path+"/f/")
	href := list[start : start+len(path)+3+10]
	if got := s.anonymous(http.MethodGet, href, nil, nil); got.Code != http.StatusOK || got.Body.String() != "offer one" {
		t.Errorf("the file comes: %d %q", got.Code, got.Body.String())
	}
	if got := s.anonymous(http.MethodGet, path+"/f/"+outside.ID, nil, nil); got.Code != http.StatusNotFound {
		t.Errorf("a file outside the folder: %d", got.Code)
	}
	// Upload only.
	drop := decode[apiLink](t, s.send(http.MethodPost, "/api/objects/"+folder.ID+"/links", `{"mode":"upload"}`, sameSite))
	dropPath := pathOf(t, drop.URL)
	if body := s.anonymous(http.MethodGet, dropPath, nil, nil).Body.String(); strings.Contains(body, "offer.txt") || !strings.Contains(body, "Send a file") {
		t.Errorf("upload only shows nothing of the folder: %s", body)
	}
	send := func(name, content string) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		part, _ := w.CreateFormFile("file", name)
		_, _ = part.Write([]byte(content))
		_ = w.Close()
		return s.anonymous(http.MethodPost, dropPath+"/upload", buf.Bytes(), map[string]string{"Content-Type": w.FormDataContentType()})
	}
	if got := send("offer.txt", "replaced?"); got.Code != http.StatusSeeOther {
		t.Fatalf("send = %d %s", got.Code, got.Body.String())
	}
	if got := s.anonymous(http.MethodGet, href, nil, nil).Body.String(); got != "offer one" {
		t.Errorf("an upload never replaces what is there: %q", got)
	}
	if list := s.anonymous(http.MethodGet, path, nil, nil).Body.String(); !strings.Contains(list, "offer (2).txt") {
		t.Errorf("it arrives under a new name: %s", list)
	}
	if got := s.anonymous(http.MethodGet, dropPath+"/f/"+outside.ID, nil, nil).Code; got != http.StatusNotFound {
		t.Errorf("upload only sends no file: %d", got)
	}
	if got := s.anonymous(http.MethodPost, path+"/upload", nil, nil).Code; got != http.StatusNotFound {
		t.Errorf("a link to read takes no upload: %d", got)
	}
	if got := s.send(http.MethodPost, "/api/objects/"+outside.ID+"/links", `{"mode":"upload"}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("upload only into a file = %d", got)
	}
}

// TestTokensStayOutOfTheLog: the log names the address of a link and of an invitation without its token.
func TestTokensStayOutOfTheLog(t *testing.T) {
	var out bytes.Buffer
	handler := accessLog(slog.New(slog.NewTextHandler(&out, nil)), http.NotFoundHandler())
	for _, target := range []string{"/s/secret-token-1/f/abc", "/s/secret-token-2", "/invite/secret-token-3"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, target, nil))
	}
	if log := out.String(); strings.Contains(log, "secret-token") || !strings.Contains(log, "/s/…/f/abc") {
		t.Errorf("the log: %s", log)
	}
}

// TestALinkShowsOnlyWhatItsMakerSees: a link to read a folder of the files reaches a folder below it that breaks the
// inheritance and shuts out whoever made the link. A link never shows more than its maker sees now, checked for each
// object (ADR-29): that folder and its files are not found through it, nor listed, while the rest of the folder is.
func TestALinkShowsOnlyWhatItsMakerSees(t *testing.T) {
	s := withFiles(t)
	for _, who := range []map[string]string{plain, carlOf} {
		s.send(http.MethodGet, "/api/state", "", who)
	}
	// Bob makes a collection of the files and shares it with the tenant; carl a folder in it that only he may see.
	made := s.send(http.MethodPost, "/api/files/collections", `{"title":"Projects"}`, plain)
	if made.Code != http.StatusCreated {
		t.Fatalf("collection = %d %s", made.Code, made.Body.String())
	}
	projects := decode[map[string]string](t, made)["id"]
	if got := s.send(http.MethodPut, "/api/objects/"+projects+"/grants", `{"kind":"tenant","role":"edit"}`, plain); got.Code != http.StatusNoContent {
		t.Fatalf("bob shares Projects with the tenant: %d %s", got.Code, got.Body.String())
	}
	hr := decode[apiNode](t, s.send(http.MethodPost, "/api/folders", `{"title":"HR","parent":"`+projects+`"}`, carlOf))
	if got := s.send(http.MethodPut, "/api/objects/"+hr.ID+"/inherit", `{"inherit":false}`, carlOf).Code; got != http.StatusNoContent {
		t.Fatalf("break = %d", got)
	}
	for _, take := range []string{`{"kind":"person","who":"1002","role":""}`, `{"kind":"tenant","role":""}`} {
		if got := s.send(http.MethodPut, "/api/objects/"+hr.ID+"/grants", take, carlOf); got.Code != http.StatusNoContent {
			t.Fatalf("take back %s = %d %s", take, got.Code, got.Body.String())
		}
	}
	check := decode[map[string]any](t, s.send(http.MethodGet, "/api/rights/check?object="+hr.ID, "", plain))
	if check["role"] != "" {
		t.Fatalf("bob holds nothing on HR: %v", check)
	}
	up := s.put(carlOf, `"parent":"`+hr.ID+`"`, "salaries.txt", []byte("carl earns 9000"), 64)
	if up.Code != http.StatusCreated {
		t.Fatalf("upload = %d %s", up.Code, up.Body.String())
	}
	file := decode[finished](t, up)
	plan := s.put(plain, `"parent":"`+projects+`"`, "plan.txt", []byte("the plan"), 64)
	if plan.Code != http.StatusCreated {
		t.Fatalf("upload = %d %s", plan.Code, plan.Body.String())
	}
	// Bob shares his collection to read.
	link := s.send(http.MethodPost, "/api/objects/"+projects+"/links", `{"mode":"read"}`, plain)
	if link.Code != http.StatusCreated {
		t.Fatalf("link = %d %s", link.Code, link.Body.String())
	}
	path := pathOf(t, decode[apiLink](t, link).URL)
	// A folder is listed as ">HR/<"; the bare letters may stand in the random token of the link (a run for #92, 2026-11-19).
	if got := s.anonymous(http.MethodGet, path, nil, nil); got.Code != http.StatusOK || strings.Contains(got.Body.String(), ">HR/<") ||
		!strings.Contains(got.Body.String(), "plan.txt") {
		t.Errorf("the link lists the folder its maker may not see, or not the plan: %d %s", got.Code, got.Body.String())
	}
	if got := s.anonymous(http.MethodGet, path+"?in="+hr.ID, nil, nil); got.Code != http.StatusNotFound || strings.Contains(got.Body.String(), "salaries.txt") {
		t.Errorf("the link opens the folder its maker may not see: %d %s", got.Code, got.Body.String())
	}
	if got := s.anonymous(http.MethodGet, path+"/f/"+file.ID, nil, nil); got.Code != http.StatusNotFound || got.Body.String() == "carl earns 9000" {
		t.Errorf("the link sends a file its maker may not see: %d %q", got.Code, got.Body.String())
	}
	if got := s.anonymous(http.MethodGet, path+"/f/"+decode[finished](t, plan).ID, nil, nil); got.Code != http.StatusOK || got.Body.String() != "the plan" {
		t.Errorf("the link does not send the plan: %d %q", got.Code, got.Body.String())
	}
}

// TestAFrozenPageShowsItsAttachments: a link to read a page shows the images pasted into it, and into the pages it
// released, through the address of the link, for anyone with the link; an attachment the frozen page does not show, of
// a page not released or added after the freeze, is not found through it. A link into the workspace becomes plain
// text, since whoever has the link has no way in there; one to a heading of the page itself stays (ADR-29).
func TestAFrozenPageShowsItsAttachments(t *testing.T) {
	s := withFiles(t)
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	newPage := func(title string) string {
		return decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"`+title+`","parent":"`+team.ID+`"}`, sameSite)).ID
	}
	attach := func(page, name, content string) string {
		got := s.put(nil, `"page":"`+page+`"`, name, []byte(content), 64)
		if got.Code != http.StatusCreated {
			t.Fatalf("attachment = %d %s", got.Code, got.Body.String())
		}
		return decode[finished](t, got).URL
	}
	block := func(page, md string) {
		body, _ := json.Marshal(map[string]string{"md": md})
		if got := s.send(http.MethodPost, "/api/pages/"+page+"/blocks", string(body), sameSite); got.Code != http.StatusCreated {
			t.Fatalf("block = %d %s", got.Code, got.Body.String())
		}
	}
	png := "\x89PNG\r\n\x1a\nmenu"
	prices, salaries, menu := newPage("Prices"), newPage("Salaries"), newPage("Menu")
	ownImage, pricesImage, salariesImage := attach(menu, "menu.png", png), attach(prices, "prices.png", "\x89PNG\r\n\x1a\nprices"), attach(salaries, "salaries.png", "\x89PNG\r\n\x1a\n4711")
	block(prices, "![prices]("+pricesImage+")")
	block(salaries, "![salaries]("+salariesImage+")")
	block(menu, "## Our menu\n\n![menu]("+ownImage+")\n\nSee [the prices](/o/"+prices+"), [the top](/o/"+menu+"#our-menu) and [elsewhere](https://example.org/).")
	block(menu, "::embed{ref=\""+prices+"\"}")
	block(menu, "::embed{ref=\""+salaries+"\"}")
	link := decode[apiLink](t, s.send(http.MethodPost, "/api/objects/"+menu+"/links", `{"mode":"read","released":["`+prices+`"]}`, sameSite))
	path := pathOf(t, link.URL)
	later := attach(menu, "later.png", "\x89PNG\r\n\x1a\nlater")
	page := s.anonymous(http.MethodGet, path, nil, nil).Body.String()
	for _, want := range []string{`src="` + path + `/p/` + menu + `/attachments/`, `src="` + path + `/p/` + prices + `/attachments/`, `href="#h-our-menu"`, `href="https://example.org/"`, `<a data-not-shared title="Not part of this link">the prices</a>`} {
		if !strings.Contains(page, want) {
			t.Errorf("the frozen page lacks %s: %s", want, page)
		}
	}
	for _, not := range []string{"/api/pages/", `href="/o/`, salaries + "/attachments"} {
		if strings.Contains(page, not) {
			t.Errorf("the frozen page holds %s: %s", not, page)
		}
	}
	through := func(address string) *httptest.ResponseRecorder {
		return s.anonymous(http.MethodGet, path+"/p/"+strings.TrimPrefix(address, "/api/pages/"), nil, nil)
	}
	if got := through(ownImage); got.Code != http.StatusOK || got.Body.String() != png {
		t.Errorf("the image of the page through the link: %d %q", got.Code, got.Body.String())
	}
	if got := through(pricesImage); got.Code != http.StatusOK || !strings.HasSuffix(got.Body.String(), "prices") {
		t.Errorf("the image of a released page through the link: %d %q", got.Code, got.Body.String())
	}
	for name, address := range map[string]string{"of a page not released": salariesImage, "added after the freeze": later} {
		if got := through(address); got.Code != http.StatusNotFound {
			t.Errorf("an attachment %s: %d %q", name, got.Code, got.Body.String())
		}
	}
	if got := s.anonymous(http.MethodGet, ownImage, nil, nil).Code; got == http.StatusOK {
		t.Errorf("the address of the workspace sends the image without an account")
	}
	// A released page that its maker no longer sees sends nothing through the link, though the frozen page shows it.
	board := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Board","parent":"`+team.ID+`"}`, plain)).ID
	boardImage := decode[finished](t, s.put(plain, `"page":"`+board+`"`, "board.png", []byte("\x89PNG\r\n\x1a\nboard"), 64)).URL
	if got := s.send(http.MethodPost, "/api/pages/"+board+"/blocks", `{"md":"![board](`+boardImage+`)"}`, plain); got.Code != http.StatusCreated {
		t.Fatalf("block = %d %s", got.Code, got.Body.String())
	}
	block(menu, "::embed{ref=\""+board+"\"}")
	both := decode[apiLink](t, s.send(http.MethodPost, "/api/objects/"+menu+"/links", `{"mode":"read","released":["`+board+`"]}`, sameSite))
	bothPath := pathOf(t, both.URL)
	viaBoth := bothPath + "/p/" + strings.TrimPrefix(boardImage, "/api/pages/")
	if got := s.anonymous(http.MethodGet, viaBoth, nil, nil).Code; got != http.StatusOK {
		t.Fatalf("the image of bob's released page: %d", got)
	}
	s.send(http.MethodPut, "/api/objects/"+board+"/inherit", `{"inherit":false}`, plain)
	// Broken, the page holds what held from above, anna's role included; bob takes all of it back.
	held := decode[struct {
		Own []struct{ Kind, Who string } `json:"own"`
	}](t, s.send(http.MethodGet, "/api/objects/"+board+"/rights", "", plain))
	for _, g := range held.Own {
		take, _ := json.Marshal(map[string]string{"kind": g.Kind, "who": g.Who, "role": ""})
		if got := s.send(http.MethodPut, "/api/objects/"+board+"/grants", string(take), plain); got.Code != http.StatusNoContent {
			t.Fatalf("take back %s = %d %s", take, got.Code, got.Body.String())
		}
	}
	if check := decode[map[string]any](t, s.send(http.MethodGet, "/api/rights/check?object="+board, "", sameSite)); check["role"] != nil && check["role"] != "" {
		t.Fatalf("anna still holds a role on Board: %v", check)
	}
	if got := s.anonymous(http.MethodGet, viaBoth, nil, nil); got.Code != http.StatusNotFound {
		t.Errorf("the image of a page its maker no longer sees: %d %q", got.Code, got.Body.String())
	}
	s.send(http.MethodDelete, "/api/links/"+link.ID, "", sameSite)
	if got := through(ownImage).Code; got != http.StatusNotFound {
		t.Errorf("revoked, the link sends the image: %d", got)
	}
}
