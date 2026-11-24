// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/gateapi/gatetest"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// The review of B-6, T-33 and T-38 (B-7), through the API. Each test is the attack round on the door it closes.

// Finding 2: the admin of one group put any account into their group by its key, then made a new sign-in link for it and
// signed in as that person, or locked them everywhere. A group admin now manages only the persons in none but their
// groups or whom they invited; a new sign-in link is only for the inviter, and their lock takes the person out of their
// groups, nothing more.
func TestGroupAdminsTakeOverNobody(t *testing.T) {
	s := withAccounts(t, false)
	ctx := tenant.With(context.Background(), tenant.Main)
	// mira is an account the gate lets UnderDoc manage, prepared before her first sign-in, in the office only.
	miraID := s.gate.Add(gatetest.Account{Username: "mira", Roles: []string{"underdoc-user"}, Managed: true})
	mira, _, err := s.db.AddAccount(ctx, miraID, "mira", "Mira", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := s.send(http.MethodPut, "/api/groups/"+s.office+"/members/"+mira.ID, `{"member":true}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("mira into the office = %d", got)
	}
	// carl takes mira into his branch by her key.
	if got := s.send(http.MethodPut, "/api/groups/"+s.branch+"/members/"+mira.ID, `{"member":true}`, carlHeaders).Code; got != http.StatusNoContent {
		t.Fatalf("carl takes mira into his branch = %d", got)
	}
	before := s.gateCalls()
	for name, try := range map[string][2]string{
		"looks at her account":   {http.MethodGet, "/api/accounts/" + mira.ID},
		"makes a sign-in link":   {http.MethodPost, "/api/accounts/" + mira.ID + "/signin-link"},
		"locks her":              {http.MethodPut, "/api/accounts/" + mira.ID + "/lock"},
		"resets her access":      {http.MethodPost, "/api/accounts/" + mira.ID + "/reset"},
		"makes a link for bob":   {http.MethodPost, "/api/accounts/1002/signin-link"},
		"locks bob in the gate":  {http.MethodPut, "/api/accounts/1002/lock"},
		"makes a link for anna":  {http.MethodPost, "/api/accounts/1007/signin-link"},
		"makes a link for dora":  {http.MethodPost, "/api/accounts/1004/signin-link"},
		"locks dora, a delegate": {http.MethodPut, "/api/accounts/1004/lock"},
	} {
		body := ""
		if try[0] == http.MethodPut {
			body = `{"locked":true}`
		}
		if got := s.send(try[0], try[1], body, carlHeaders).Code; got != http.StatusForbidden {
			t.Errorf("carl %s, of a person also in another group = %d", name, got)
		}
	}
	// The gate is asked what roles an account holds, never to act.
	for _, c := range s.gate.Calls()[before:] {
		if c.Method != http.MethodGet {
			t.Errorf("a refused request reached the gate: %+v", c)
		}
	}
	if a, _ := s.db.AccountByID(ctx, mira.ID); a.Locked {
		t.Error("carl locked mira")
	}
	// Out of the office, mira is in none but carl's group: he sees her and takes her out of it, and no more.
	if got := s.send(http.MethodPut, "/api/groups/"+s.office+"/members/"+mira.ID, `{"member":false}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("mira out of the office = %d", got)
	}
	info := decode[apiAccountView](t, s.send(http.MethodGet, "/api/accounts/"+mira.ID, "", carlHeaders))
	if info.Link || info.Lock || !info.Remove || info.Reset {
		t.Errorf("carl is offered for mira: %+v", info)
	}
	if got := s.send(http.MethodPost, "/api/accounts/"+mira.ID+"/signin-link", "", carlHeaders); got.Code != http.StatusForbidden || strings.Contains(got.Body.String(), "/auth/magic/") {
		t.Errorf("carl makes a sign-in link for mira, whom he did not invite = %d", got.Code)
	}
	got := decode[map[string]any](t, s.send(http.MethodPut, "/api/accounts/"+mira.ID+"/lock", `{"locked":true}`, carlHeaders))
	if got["locked"] != false || got["removed"] != float64(1) || got["gate"] != "untouched" {
		t.Errorf("carl's lock of mira: %v", got)
	}
	if a, _ := s.db.AccountByID(ctx, mira.ID); a.Locked {
		t.Error("carl's lock locked mira in UnderDoc")
	}
	if ga, _ := s.gate.Get(miraID); ga.State != "active" {
		t.Errorf("carl's lock locked mira in the gate: %+v", ga)
	}
	if got := s.send(http.MethodPut, "/api/accounts/"+mira.ID+"/lock", `{"locked":false}`, carlHeaders).Code; got != http.StatusForbidden {
		t.Errorf("carl unlocks = %d", got)
	}
	// Whom carl invited is his: he gives her a new link before her first sign-in, and his lock still only removes.
	_, out := s.invite(s.branch, `{"login":"lena"}`, carlHeaders)
	lena := out["account"].(string)
	if got := s.send(http.MethodPut, "/api/groups/"+s.office+"/members/"+lena, `{"member":true}`, sameSite).Code; got != http.StatusNoContent {
		t.Fatalf("lena into the office = %d", got)
	}
	if info := decode[apiAccountView](t, s.send(http.MethodGet, "/api/accounts/"+lena, "", carlHeaders)); !info.Link || info.Lock || !info.Remove {
		t.Errorf("carl is offered for lena, whom he invited: %+v", info)
	}
	if got := s.send(http.MethodPost, "/api/accounts/"+lena+"/signin-link", "", carlHeaders); got.Code != http.StatusOK {
		t.Errorf("carl's new link for lena = %d %s", got.Code, got.Body.String())
	}
	if got := decode[map[string]any](t, s.send(http.MethodPut, "/api/accounts/"+lena+"/lock", `{"locked":true}`, carlHeaders)); got["locked"] != false || got["removed"] != float64(1) {
		t.Errorf("carl's lock of lena: %v", got)
	}
	if groups, _ := s.db.DirectGroupsOf(ctx, lena); len(groups) != 1 || groups[0] != s.office {
		t.Errorf("lena after carl's lock is in %v", groups)
	}
	// Still his, as he invited her, but there is nothing for him to unlock.
	if got := s.send(http.MethodPut, "/api/accounts/"+lena+"/lock", `{"locked":false}`, carlHeaders).Code; got != http.StatusForbidden {
		t.Errorf("carl unlocks lena = %d", got)
	}
}

// Finding 9: whether an account is an administrator's came from the roles of its last sign-in. An account made by add or
// adopt, or promoted in the gate since, looked like nobody's: the delegate locked or gave links to an administrator.
// The gate's roles now count where it lets UnderDoc see the account.
func TestRolesComeFromTheGate(t *testing.T) {
	s := withAccounts(t, false)
	ctx := tenant.With(context.Background(), tenant.Main)
	bossID := s.gate.Add(gatetest.Account{Username: "boss", Roles: []string{"underdoc-admin"}, Managed: true})
	boss, _, err := s.db.AddAccount(ctx, bossID, "boss", "Boss", "")
	if err != nil {
		t.Fatal(err)
	}
	deputyID := s.gate.Add(gatetest.Account{Username: "deputy", Roles: []string{"underdoc-delegate"}, Managed: true})
	deputy, _, err := s.db.AddAccount(ctx, deputyID, "deputy", "Deputy", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{boss.ID, deputy.ID} {
		for name, try := range map[string][2]string{
			"looks at":        {http.MethodGet, "/api/accounts/" + target},
			"makes a link":    {http.MethodPost, "/api/accounts/" + target + "/signin-link"},
			"locks":           {http.MethodPut, "/api/accounts/" + target + "/lock"},
			"unlocks":         {http.MethodPut, "/api/accounts/" + target + "/lock"},
			"invited, a link": {http.MethodPost, "/api/accounts/" + target + "/signin-link"},
		} {
			body := ""
			if try[0] == http.MethodPut {
				body = `{"locked":` + map[bool]string{true: "false", false: "true"}[name == "unlocks"] + `}`
			}
			if got := s.send(try[0], try[1], body, doraHeaders).Code; got != http.StatusForbidden {
				t.Errorf("dora %s %s, privileged in the gate = %d", name, target, got)
			}
		}
	}
	if a, _ := s.db.AccountByID(ctx, boss.ID); a.Locked {
		t.Error("the delegate locked an administrator")
	}
	// The administrator manages them.
	if got := s.send(http.MethodGet, "/api/accounts/"+boss.ID, "", sameSite).Code; got != http.StatusOK {
		t.Errorf("anna looks at the boss = %d", got)
	}
	// A gate that does not answer: nobody but an administrator acts, except a lock, which must work while it is away.
	plainID := s.gate.Add(gatetest.Account{Username: "pia", Roles: []string{"underdoc-user"}, Managed: true})
	pia, _, _ := s.db.AddAccount(ctx, plainID, "pia", "Pia", "")
	s.gate.Refuse(http.StatusBadGateway, "")
	if got := s.send(http.MethodPost, "/api/accounts/"+pia.ID+"/signin-link", "", doraHeaders).Code; got != http.StatusForbidden {
		t.Errorf("dora makes a link while the gate does not answer = %d", got)
	}
	got := decode[map[string]any](t, s.send(http.MethodPut, "/api/accounts/"+pia.ID+"/lock", `{"locked":true}`, doraHeaders))
	s.gate.Refuse(0, "")
	if got["locked"] != true {
		t.Errorf("dora locks pia while the gate does not answer: %v", got)
	}
}
