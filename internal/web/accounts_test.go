// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/gateapi"
	"github.com/Ollornog/UnderDoc/internal/gateapi/gatetest"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// Accounts through the gate's account API, and the approvals of the gate (T-33), through the API. The gate is the
// stand-in of package gatetest here; the contract test of package gateapi holds it to a real TinySesam. Each test is
// also the attack round on the door it opens.

// The persons of these tests, besides anna (an administrator) and bob: carl becomes a group admin, dora holds the role of
// the delegated administrator.
var (
	carlHeaders = map[string]string{"Remote-Auth": "session", "Remote-Kind": "user", "Remote-User": "carl", "Remote-Id": "1003", "Remote-Name": "Carl",
		"Remote-Groups": "", "Sec-Fetch-Site": "same-origin"}
	doraHeaders = map[string]string{"Remote-Auth": "session", "Remote-Kind": "user", "Remote-User": "dora", "Remote-Id": "1004", "Remote-Name": "Dora",
		"Remote-Groups": "underdoc-delegate", "Sec-Fetch-Site": "same-origin"}
)

type accountsSite struct {
	*site
	gate   *gatetest.Gate
	db     *db.DB
	branch string // a group carl manages
	office string // a group nobody but administrators manage
}

// withAccounts is a site with the account API of the stand-in gate, and with its approvals for deleting for good where
// approvals is true.
func withAccounts(t *testing.T, approvals bool) *accountsSite {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	g := gatetest.New(t, "underdoc", []string{"underdoc-user"}, "underdoc-admin")
	client, err := gateapi.New(gateapi.Config{URL: g.URL, KeyFile: g.KeyFile()})
	if err != nil {
		t.Fatal(err)
	}
	s := &accountsSite{gate: g, db: database, site: newSite(t, Options{DB: database,
		GateClient: gateOf(t, account.GateConfig{ServiceAccounts: []string{"4242"}}),
		Gate:       &GateAPI{Client: client, Roles: []string{"underdoc-user"}, ApproverRole: "underdoc-admin", App: "underdoc", Approvals: approvals}}, sample)}
	for _, who := range []map[string]string{gateHeaders, plain, carlHeaders, doraHeaders} {
		if got := s.getWith("/api/state", who).Code; got != http.StatusOK {
			t.Fatalf("%s comes in: %d", who["Remote-User"], got)
		}
	}
	s.branch = decode[apiGroup](t, s.send(http.MethodPost, "/api/groups", `{"name":"Branch"}`, sameSite)).ID
	s.office = decode[apiGroup](t, s.send(http.MethodPost, "/api/groups", `{"name":"Office"}`, sameSite)).ID
	for _, step := range [][2]string{
		{"/api/groups/" + s.branch + "/admins/1003", `{"admin":true}`},
		{"/api/groups/" + s.office + "/members/1002", `{"member":true}`},
	} {
		if got := s.send(http.MethodPut, step[0], step[1], sameSite).Code; got != http.StatusNoContent {
			t.Fatalf("%s = %d", step[0], got)
		}
	}
	return s
}

// invite asks for an invitation into a group as someone.
func (s *accountsSite) invite(group, body string, who map[string]string) (int, map[string]any) {
	got := s.send(http.MethodPost, "/api/groups/"+group+"/invite", body, who)
	var out map[string]any
	_ = json.Unmarshal(got.Body.Bytes(), &out)
	return got.Code, out
}

func (s *accountsSite) gateCalls() int { return len(s.gate.Calls()) }

// Attack: everyone who may not invite into a group, and every way that is no person in the browser, reaches the gate
// not at all; a role off UnderDoc's allowlist, the approver's or an administrator's role included, neither.
func TestInvitingIsForGroupAdminsInTheBrowser(t *testing.T) {
	s := withAccounts(t, false)
	body := `{"login":"lena","name":"Lena M."}`
	for name, try := range map[string]struct {
		group string
		who   map[string]string
		want  int
	}{
		"bob, no admin":                        {s.branch, plain, http.StatusForbidden},
		"carl into a group he does not manage": {s.office, carlHeaders, http.StatusForbidden},
		"carl's agent":                         {s.branch, merged2(carlHeaders, map[string]string{"Remote-Auth": "oauth"}), http.StatusForbidden},
		"carl's key":                           {s.branch, merged2(carlHeaders, map[string]string{"Remote-Auth": "apikey"}), http.StatusForbidden},
		"carl's app password":                  {s.branch, merged2(carlHeaders, map[string]string{"Remote-Auth": "app-password"}), http.StatusForbidden},
		"an administrator's agent":             {s.branch, merged2(gateHeaders, map[string]string{"Remote-Auth": "oauth", "Sec-Fetch-Site": "same-origin"}), http.StatusForbidden},
		"a service account with every role":    {s.branch, serviceWithRoles, http.StatusForbidden},
		"another site":                         {s.branch, merged2(carlHeaders, map[string]string{"Sec-Fetch-Site": "cross-site"}), http.StatusForbidden},
	} {
		before := s.gateCalls()
		if code, out := s.invite(try.group, body, try.who); code != try.want {
			t.Errorf("%s invites = %d %v", name, code, out)
		}
		if s.gateCalls() != before {
			t.Errorf("%s reached the gate", name)
		}
	}
	for name, roles := range map[string]string{
		"a role off the allowlist": `["staff"]`,
		"the approver role":        `["underdoc-admin"]`,
		"the delegate's role":      `["underdoc-delegate"]`,
		"one good, one bad":        `["underdoc-user","underdoc-admin"]`,
	} {
		before := s.gateCalls()
		if code, out := s.invite(s.branch, `{"login":"otto","roles":`+roles+`}`, carlHeaders); code != http.StatusBadRequest {
			t.Errorf("%s = %d %v", name, code, out)
		}
		if s.gateCalls() != before {
			t.Errorf("%s reached the gate", name)
		}
	}
	// The tools of MCP manage no account.
	session := mcpSession(t, s.http, annasAgent)
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		if strings.Contains(tool.Name, "account") || strings.Contains(tool.Name, "invite") || strings.Contains(tool.Name, "lock") {
			t.Errorf("an MCP tool for accounts: %s", tool.Name)
		}
	}
}

// A group admin invites: the gate makes the account in UnderDoc's name for carl, UnderDoc binds a profile to the new
// lasting ID at once and puts it into the group; the link comes once, and the log says who acted.
func TestAGroupAdminInvites(t *testing.T) {
	s := withAccounts(t, false)
	code, out := s.invite(s.branch, `{"login":"lena","name":"Lena M.","email":"lena@example.org"}`, carlHeaders)
	if code != http.StatusOK || out["link"] == "" || out["account"] == "" {
		t.Fatalf("carl invites = %d %v", code, out)
	}
	calls := s.gate.Calls()
	last := calls[len(calls)-1]
	if last.Method != http.MethodPost || last.Path != "/auth/api/users" || last.Actor != "1003" {
		t.Errorf("the gate was asked: %+v", last)
	}
	lena := out["account"].(string)
	ctx := tenant.With(context.Background(), tenant.Main)
	a, err := s.db.AccountByID(ctx, lena)
	if err != nil || a.RemoteID == "" || a.Admin || a.Login != "lena" {
		t.Fatalf("the profile: %+v %v", a, err)
	}
	members := decode[struct {
		Members []apiPerson `json:"members"`
	}](t, s.send(http.MethodGet, "/api/groups/"+s.branch, "", carlHeaders))
	if len(members.Members) != 1 || members.Members[0].ID != lena {
		t.Errorf("lena is not in the group at once: %+v", members.Members)
	}
	// Her first request comes into that profile.
	lenaHeaders := map[string]string{"Remote-Auth": "session", "Remote-Kind": "user", "Remote-User": "lena", "Remote-Id": a.RemoteID, "Remote-Name": "Lena M.", "Remote-Groups": "underdoc-user"}
	if state := decode[apiState](t, s.getWith("/api/state", lenaHeaders)); state.User.ID != lena {
		t.Errorf("lena comes into another account: %s", state.User.ID)
	}
	log := s.send(http.MethodGet, "/api/log?limit=50", "", sameSite).Body.String()
	if !strings.Contains(log, `"invited"`) || !strings.Contains(log, "on behalf of 1003") || strings.Contains(log, "/auth/magic/") {
		t.Errorf("the log of the invitation: %s", log)
	}
	// An invitation under the login of someone in UnderDoc makes another account: bob is never bound by his name.
	code, out = s.invite(s.branch, `{"login":"bob"}`, carlHeaders)
	if code != http.StatusOK || out["account"] == "1002" {
		t.Fatalf("an invitation under bob's login = %d %v", code, out)
	}
	if b, _ := s.db.AccountByID(ctx, "1002"); b.RemoteID != "1002" {
		t.Errorf("bob's account was bound elsewhere: %+v", b)
	}
	// The delegated administrator invites into any group, also one nobody made them admin of.
	if code, out := s.invite(s.office, `{"login":"mira"}`, doraHeaders); code != http.StatusOK {
		t.Errorf("dora invites into the office = %d %v", code, out)
	}
	// Into an archived group nobody invites.
	if got := s.send(http.MethodDelete, "/api/groups/"+s.office, "", sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("archive = %d", got)
	}
	before := s.gateCalls()
	if code, _ := s.invite(s.office, `{"login":"otto"}`, sameSite); code != http.StatusConflict {
		t.Errorf("into an archived group = %d", code)
	}
	if s.gateCalls() != before {
		t.Error("an invitation into an archived group reached the gate")
	}
}

// A new sign-in link only before the first sign-in and only for an account that is not locked.
func TestANewSignInLink(t *testing.T) {
	s := withAccounts(t, false)
	_, out := s.invite(s.branch, `{"login":"lena"}`, carlHeaders)
	lena := out["account"].(string)
	got := s.send(http.MethodPost, "/api/accounts/"+lena+"/signin-link", "", carlHeaders)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "/auth/magic/") {
		t.Fatalf("a new link = %d %s", got.Code, got.Body.String())
	}
	if got := s.send(http.MethodPost, "/api/accounts/"+lena+"/signin-link", "", plain).Code; got != http.StatusForbidden {
		t.Errorf("bob asks for lena's link = %d", got)
	}
	a, _ := s.db.AccountByID(tenant.With(context.Background(), tenant.Main), lena)
	s.gate.SignIn(a.RemoteID)
	if got := s.send(http.MethodPost, "/api/accounts/"+lena+"/signin-link", "", carlHeaders); got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), "signed in already") {
		t.Errorf("a link after the first sign-in = %d %s", got.Code, got.Body.String())
	}
	info := decode[apiAccountView](t, s.send(http.MethodGet, "/api/accounts/"+lena, "", carlHeaders))
	if info.Link || !info.SignedIn || info.Gate != gateapi.StateActive {
		t.Errorf("after the first sign-in the workspace offers: %+v", info)
	}
}

// Locking holds in the gate and in UnderDoc: the locked person comes in through no way, MCP and a share link of theirs
// included. Nobody locks themselves, a group admin no administrator, the delegate no administrator.
func TestLockingHoldsEverywhere(t *testing.T) {
	s := withAccounts(t, false)
	_, out := s.invite(s.branch, `{"login":"lena"}`, carlHeaders)
	lena := out["account"].(string)
	ctx := tenant.With(context.Background(), tenant.Main)
	a, _ := s.db.AccountByID(ctx, lena)
	lenaHeaders := map[string]string{"Remote-Auth": "session", "Remote-Kind": "user", "Remote-User": "lena", "Remote-Id": a.RemoteID, "Remote-Name": "Lena", "Remote-Groups": "underdoc-user", "Sec-Fetch-Site": "same-origin"}
	// Lena shares a page of hers with a link before she is locked.
	space := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Plan","parent":"`+space.ID+`"}`, lenaHeaders))
	link := decode[map[string]any](t, s.send(http.MethodPost, "/api/objects/"+page.ID+"/links", `{"mode":"read"}`, lenaHeaders))
	shareURL, _ := link["url"].(string)
	share, _ := url.Parse(shareURL)
	if share == nil || share.Path == "" || s.getWith(share.Path, nil).Code != http.StatusOK {
		t.Fatalf("lena's link before the lock: %v", link)
	}
	for name, try := range map[string]struct {
		target string
		who    map[string]string
	}{
		"carl himself":                          {"1003", carlHeaders},
		"carl, an administrator":                {"1007", carlHeaders},
		"dora, an administrator":                {"1007", doraHeaders},
		"bob, lena":                             {lena, plain},
		"carl's agent, lena":                    {lena, merged2(carlHeaders, map[string]string{"Remote-Auth": "oauth"})},
		"a service account, lena":               {lena, serviceWithRoles},
		"carl, bob in another group":            {"1002", carlHeaders},
		"carl, the service account":             {"4242", carlHeaders},
		"an administrator, themself":            {"1007", sameSite},
		"an administrator, the service account": {"4242", sameSite},
		"an administrator's key, bob":           {"1002", merged2(gateHeaders, map[string]string{"Remote-Auth": "apikey", "Sec-Fetch-Site": "same-origin"})},
	} {
		if got := s.send(http.MethodPut, "/api/accounts/"+try.target+"/lock", `{"locked":true}`, try.who).Code; got != http.StatusForbidden && got != http.StatusNotFound {
			t.Errorf("%s locks = %d", name, got)
		}
	}
	got := decode[map[string]any](t, s.send(http.MethodPut, "/api/accounts/"+lena+"/lock", `{"locked":true}`, carlHeaders))
	if got["locked"] != true || got["gate"] != "done" {
		t.Fatalf("carl locks lena: %v", got)
	}
	if ga, _ := s.gate.Get(a.RemoteID); ga.State != gateapi.StateDisabled {
		t.Errorf("lena is not locked in the gate: %+v", ga)
	}
	for name, who := range map[string]map[string]string{
		"the workspace":        lenaHeaders,
		"a key of hers":        merged2(lenaHeaders, map[string]string{"Remote-Auth": "apikey"}),
		"an app password":      merged2(lenaHeaders, map[string]string{"Remote-Auth": "app-password"}),
		"her agent with OAuth": merged2(lenaHeaders, map[string]string{"Remote-Auth": "oauth"}),
	} {
		if got := s.getWith("/api/state", who); got.Code != http.StatusForbidden || !strings.Contains(got.Body.String(), "Locked") {
			t.Errorf("%s while locked = %d", name, got.Code)
		}
	}
	if got := s.send(http.MethodPost, MCPPath, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, merged2(lenaHeaders, map[string]string{"Remote-Auth": "oauth"})).Code; got != http.StatusForbidden {
		t.Errorf("MCP while locked = %d", got)
	}
	if got := s.getWith(share.Path, nil).Code; got != http.StatusNotFound {
		t.Errorf("lena's share link while she is locked = %d", got)
	}
	if got := s.send(http.MethodPost, "/api/accounts/"+lena+"/signin-link", "", carlHeaders).Code; got != http.StatusConflict {
		t.Errorf("a sign-in link for a locked account = %d", got)
	}
	// Unlocked, all of it comes back.
	if got := decode[map[string]any](t, s.send(http.MethodPut, "/api/accounts/"+lena+"/lock", `{"locked":false}`, carlHeaders)); got["locked"] != false {
		t.Fatalf("unlock: %v", got)
	}
	if got := s.getWith("/api/state", lenaHeaders).Code; got != http.StatusOK {
		t.Errorf("unlocked, lena comes in = %d", got)
	}
	if got := s.getWith(share.Path, nil).Code; got != http.StatusOK {
		t.Errorf("unlocked, her link = %d", got)
	}
	// A gate that does not lock keeps nobody open: the lock of UnderDoc holds, and no new sign-in link is given.
	s.gate.Refuse(http.StatusBadGateway, "")
	got = decode[map[string]any](t, s.send(http.MethodPut, "/api/accounts/"+lena+"/lock", `{"locked":true}`, carlHeaders))
	s.gate.Refuse(0, "")
	if got["locked"] != true || got["gate"] != "failed" {
		t.Errorf("locked while the gate fails: %v", got)
	}
	if got := s.send(http.MethodPost, "/api/accounts/"+lena+"/signin-link", "", carlHeaders).Code; got != http.StatusConflict {
		t.Errorf("a sign-in link for an account locked in UnderDoc only = %d", got)
	}
	if got := s.send(http.MethodPut, "/api/accounts/"+lena+"/lock", `{"locked":false}`, carlHeaders).Code; got != http.StatusOK {
		t.Fatalf("unlock = %d", got)
	}
	// An account the gate does not let UnderDoc manage is locked in UnderDoc all the same.
	got = decode[map[string]any](t, s.send(http.MethodPut, "/api/accounts/1002/lock", `{"locked":true}`, sameSite))
	if got["locked"] != true || got["gate"] != "unmanaged" {
		t.Errorf("anna locks bob, whom the gate's operator manages: %v", got)
	}
	if got := s.getWith("/api/state", plain).Code; got != http.StatusForbidden {
		t.Errorf("bob while locked in UnderDoc only = %d", got)
	}
	log := s.send(http.MethodGet, "/api/log?limit=50", "", sameSite).Body.String()
	if !strings.Contains(log, `"locked"`) || !strings.Contains(log, `"unlocked"`) || !strings.Contains(log, "on behalf of 1003") {
		t.Errorf("the log of locking: %s", log)
	}
}

// The delegated administrator manages accounts and archives groups, and nothing else of an administrator.
func TestTheDelegatedAdministrator(t *testing.T) {
	s := withAccounts(t, false)
	if state := decode[apiState](t, s.getWith("/api/state", doraHeaders)); !state.User.Delegate || state.User.Admin || !state.User.Accounts || state.User.Reset {
		t.Errorf("dora's state: %+v", state.User)
	}
	if got := s.send(http.MethodPut, "/api/accounts/1002/lock", `{"locked":true}`, doraHeaders).Code; got != http.StatusOK {
		t.Errorf("dora locks bob = %d", got)
	}
	if got := s.send(http.MethodGet, "/api/users", "", doraHeaders).Code; got != http.StatusOK {
		t.Errorf("dora lists the users = %d", got)
	}
	if got := s.send(http.MethodGet, "/api/groups/"+s.office, "", doraHeaders); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"1002"`) {
		t.Errorf("dora reads the members of a group = %d", got.Code)
	}
	if got := s.send(http.MethodDelete, "/api/groups/"+s.branch, "", doraHeaders).Code; got != http.StatusNoContent {
		t.Errorf("dora archives a group = %d", got)
	}
	if got := s.send(http.MethodPost, "/api/groups/"+s.branch+"/restore", "", doraHeaders).Code; got != http.StatusNoContent {
		t.Errorf("dora restores a group = %d", got)
	}
	team := s.sharedCollection(`{"title":"Team"}`, sameSite)
	rules := decode[apiNode](t, s.send(http.MethodPost, "/api/pages", `{"title":"Rules","parent":"`+team.ID+`","kind":"agent-notes"}`, sameSite))
	for name, call := range map[string][3]string{
		"the log's keeping":    {http.MethodPut, "/api/log/keep", `{"days":30}`},
		"the rules page":       {http.MethodPut, "/api/rules-page", `{"page":"` + rules.ID + `"}`},
		"an administrator":     {http.MethodPut, "/api/users/1007/admin", `{"admin":false}`},
		"a top-level group":    {http.MethodPost, "/api/groups", `{"name":"Mine"}`},
		"rights on a space":    {http.MethodPut, "/api/objects/" + team.ID + "/grants", `{"kind":"person","who":"1004","role":"manage"}`},
		"locking anna":         {http.MethodPut, "/api/accounts/1007/lock", `{"locked":true}`},
		"resetting an account": {http.MethodPost, "/api/accounts/1002/reset", ""},
	} {
		if got := s.send(call[0], call[1], call[2], doraHeaders).Code; got < 400 {
			t.Errorf("dora changes %s = %d", name, got)
		}
	}
	if got := s.send(http.MethodPost, "/admin/modules", "", doraHeaders).Code; got < 400 {
		t.Errorf("dora changes the modules = %d", got)
	}
	// Her agent is no delegate.
	agent := merged2(doraHeaders, map[string]string{"Remote-Auth": "oauth"})
	if got := s.send(http.MethodDelete, "/api/groups/"+s.branch, "", agent).Code; got != http.StatusForbidden {
		t.Errorf("dora's agent archives a group = %d", got)
	}
	if state := decode[apiState](t, s.getWith("/api/state", agent)); state.User.Delegate || state.User.Accounts {
		t.Errorf("dora's agent is offered the accounts: %+v", state.User)
	}
	// A service account with the delegate's role is none.
	service := merged2(serviceWithRoles, map[string]string{"Remote-Groups": "underdoc-delegate"})
	if got := s.send(http.MethodDelete, "/api/groups/"+s.branch, "", service).Code; got != http.StatusForbidden {
		t.Errorf("a service account with the delegate's role archives a group = %d", got)
	}
}

// Without the account API nothing of it is offered, and nothing answers.
func TestWithoutTheAccountAPI(t *testing.T) {
	s := withDatabase(t)
	s.getWith("/api/state", plain)
	if state := decode[apiState](t, s.get("/api/state")); state.User.Accounts || state.User.Reset {
		t.Errorf("the workspace offers accounts: %+v", state.User)
	}
	group := decode[apiGroup](t, s.send(http.MethodPost, "/api/groups", `{"name":"Branch"}`, sameSite)).ID
	for name, call := range map[string][3]string{
		"invite": {http.MethodPost, "/api/groups/" + group + "/invite", `{"login":"lena"}`},
		"lock":   {http.MethodPut, "/api/accounts/1002/lock", `{"locked":true}`},
		"link":   {http.MethodPost, "/api/accounts/1002/signin-link", ""},
		"reset":  {http.MethodPost, "/api/accounts/1002/reset", ""},
		"read":   {http.MethodGet, "/api/accounts/1002", ""},
	} {
		if got := s.send(call[0], call[1], call[2], sameSite).Code; got != http.StatusNotFound {
			t.Errorf("%s without the account API = %d", name, got)
		}
	}
	if got := s.getWith(approvalReturn+"?approval=apv_x1234567&state=s", gateHeaders).Code; got != http.StatusNotFound {
		t.Errorf("the way back from the gate without approvals = %d", got)
	}
}

// Resetting access: only a holder of the gate's approver role asks, confirms it in the gate as themselves, and gets the
// link once; nobody else picks it up.
func TestResettingAccess(t *testing.T) {
	s := withAccounts(t, false)
	_, out := s.invite(s.branch, `{"login":"lena"}`, carlHeaders)
	lena := out["account"].(string)
	a, _ := s.db.AccountByID(tenant.With(context.Background(), tenant.Main), lena)
	s.gate.SignIn(a.RemoteID)
	before := s.gateCalls()
	if got := s.send(http.MethodPost, "/api/accounts/"+lena+"/reset", "", carlHeaders).Code; got != http.StatusForbidden {
		t.Errorf("carl, no approver, asks = %d", got)
	}
	if s.gateCalls() != before {
		t.Error("a reset by someone without the approver role reached the gate")
	}
	asked := decode[map[string]any](t, s.send(http.MethodPost, "/api/accounts/"+lena+"/reset", "", sameSite))
	id, _ := asked["approval"].(string)
	if !gateapi.ApprovalID(id) || asked["approveUrl"] == "" || asked["matchCode"] == "" {
		t.Fatalf("anna asks: %v", asked)
	}
	if apv, _ := s.gate.Asked(id); apv.Subject != "1007" || apv.Type != gateapi.ResetAccess || apv.Fields["account"] != a.RemoteID {
		t.Errorf("the gate was asked: %+v", apv)
	}
	if got := decode[map[string]any](t, s.send(http.MethodPost, "/api/approvals/"+id+"/wait", "", sameSite)); got["state"] != "pending" {
		t.Errorf("before the confirmation: %v", got)
	}
	s.gate.Confirm(id, "1007")
	if got := s.send(http.MethodPost, "/api/approvals/"+id+"/wait", "", carlHeaders).Code; got != http.StatusNotFound {
		t.Errorf("carl picks up anna's reset = %d", got)
	}
	got := decode[map[string]any](t, s.send(http.MethodPost, "/api/approvals/"+id+"/wait", "", sameSite))
	if got["state"] != "ok" || !strings.Contains(got["link"].(string), "/auth/magic/") {
		t.Fatalf("after the confirmation: %v", got)
	}
	if got := s.send(http.MethodPost, "/api/approvals/"+id+"/wait", "", sameSite).Code; got != http.StatusNotFound {
		t.Errorf("the link a second time = %d", got)
	}
	log := s.send(http.MethodGet, "/api/log?limit=50", "", sameSite).Body.String()
	if !strings.Contains(log, `"access reset"`) || strings.Contains(log, "/auth/magic/") {
		t.Errorf("the log of the reset: %s", log)
	}
}
