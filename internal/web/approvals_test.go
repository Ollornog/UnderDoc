// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/gateapi"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// Deleting for good with a signed approval of the gate (T-33, B-3, B-6): checked first, then confirmed in the gate,
// then checked again and the confirmation used, once, for this object, this person and this browser.

// trashed makes a page in a space of anna's and puts it into the trash.
func (s *accountsSite) trashed(space, title string) string {
	s.t.Helper()
	page := decode[apiNode](s.t, s.send(http.MethodPost, "/api/pages", `{"title":"`+title+`","parent":"`+space+`"}`, sameSite))
	if got := s.send(http.MethodDelete, "/api/objects/"+page.ID, "", sameSite).Code; got != http.StatusNoContent {
		s.t.Fatalf("to the trash = %d", got)
	}
	return page.ID
}

// asked sends the deletion and returns the approval the gate was asked for and the cookie of the browser.
func (s *accountsSite) asked(target, body string) (gateapiApproval, string) {
	s.t.Helper()
	got := s.send(http.MethodDelete, target, body, sameSite)
	if got.Code != http.StatusAccepted || !strings.Contains(got.Body.String(), "approveUrl") {
		s.t.Fatalf("delete for good asks no approval: %d %s", got.Code, got.Body.String())
	}
	var cookie string
	for _, c := range got.Result().Cookies() {
		if c.Name == approvalCookie {
			if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != approvalReturn {
				s.t.Errorf("the cookie of the approval: %+v", c)
			}
			cookie = c.Name + "=" + c.Value
		}
	}
	if cookie == "" {
		s.t.Fatal("no cookie for the way back")
	}
	last := s.gate.Last()
	return gateapiApproval{id: last.ID, nonce: last.Nonce, fields: last.Fields, typ: last.Type, subject: last.Subject, ret: last.ReturnURL}, cookie
}

type gateapiApproval struct {
	id, nonce, typ, subject, ret string
	fields                       map[string]any
}

// back is the browser coming back from the gate's page.
func (s *accountsSite) back(a gateapiApproval, query, cookie string, who map[string]string) *httptest.ResponseRecorder {
	headers := merged2(who, map[string]string{})
	if cookie != "" {
		headers["Cookie"] = cookie
	}
	return s.getWith(approvalReturn+"?approval="+url.QueryEscape(a.id)+"&state="+url.QueryEscape(a.nonce)+query, headers)
}

func (s *accountsSite) inTrash(id string) bool {
	return strings.Contains(s.send(http.MethodGet, "/api/trash", "", sameSite).Body.String(), id)
}

func TestPurgeWaitsForTheGate(t *testing.T) {
	s := withAccounts(t, true)
	space := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := s.trashed(space.ID, "Minutes 2026")
	a, cookie := s.asked("/api/trash/"+page, `{"confirm":"Minutes 2026"}`)
	if a.typ != "underdoc.object.purge" || a.subject != "1007" || a.fields["object"] != page || a.fields["title"] != "Minutes 2026" ||
		!strings.HasSuffix(a.ret, approvalReturn) || len(a.nonce) < 40 {
		t.Errorf("the gate was asked: %+v", a)
	}
	if !s.inTrash(page) {
		t.Fatal("it went before the confirmation")
	}
	// Attacks on the way back before and after the confirmation.
	s.gate.Confirm(a.id, "1007")
	for name, got := range map[string]*httptest.ResponseRecorder{
		"without the cookie":         s.back(a, "", "", gateHeaders),
		"with another state":         s.getWith(approvalReturn+"?approval="+a.id+"&state=other", merged2(gateHeaders, map[string]string{"Cookie": cookie})),
		"bob with anna's cookie":     s.back(a, "", cookie, plain),
		"anna's agent":               s.back(a, "", cookie, merged2(gateHeaders, map[string]string{"Remote-Auth": "oauth"})),
		"another approval":           s.getWith(approvalReturn+"?approval=apv_000999xxxxxxxx&state="+a.nonce, merged2(gateHeaders, map[string]string{"Cookie": cookie})),
		"no approval id of the gate": s.getWith(approvalReturn+"?approval=../users&state="+a.nonce, merged2(gateHeaders, map[string]string{"Cookie": cookie})),
		"a cookie of another browser": s.getWith(approvalReturn+"?approval="+a.id+"&state="+a.nonce,
			merged2(gateHeaders, map[string]string{"Cookie": approvalCookie + "=other"})),
		"cookie and state of another browser": s.getWith(approvalReturn+"?approval="+a.id+"&state=other",
			merged2(gateHeaders, map[string]string{"Cookie": approvalCookie + "=other"})),
	} {
		if got.Code < 400 {
			t.Errorf("%s = %d", name, got.Code)
		}
	}
	if !s.inTrash(page) {
		t.Fatal("an attack on the way back deleted it")
	}
	// The wait for a reset hands out nothing of a purge.
	if got := s.send(http.MethodPost, "/api/approvals/"+a.id+"/wait", "", sameSite).Code; got != http.StatusNotFound {
		t.Errorf("waiting on the approval of a purge = %d", got)
	}
	if got := s.back(a, "", cookie, gateHeaders); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "Deleted for good") {
		t.Fatalf("the way back = %d %s", got.Code, got.Body.String())
	}
	if s.inTrash(page) {
		t.Error("confirmed, it is still in the trash")
	}
	if got := s.back(a, "", cookie, gateHeaders).Code; got < 400 {
		t.Errorf("the way back twice = %d", got)
	}
	log := s.send(http.MethodGet, "/api/log?limit=20", "", sameSite).Body.String()
	if !strings.Contains(log, "confirmed in the gate") {
		t.Errorf("the log: %s", log)
	}
}

// Attack: a confirmation is used up only by a purge that goes. Restored meanwhile, the purge is refused on the way back,
// and the confirmation is not used; rejected in the gate, nothing goes.
func TestARefusedPurgeUsesNoConfirmation(t *testing.T) {
	s := withAccounts(t, true)
	space := s.sharedCollection(`{"title":"Team"}`, sameSite)
	page := s.trashed(space.ID, "Plan")
	a, cookie := s.asked("/api/trash/"+page, `{"confirm":"Plan"}`)
	s.gate.Confirm(a.id, "1007")
	if got := s.send(http.MethodPost, "/api/trash/"+page+"/restore", "", sameSite).Code; got != http.StatusOK && got != http.StatusNoContent {
		t.Fatalf("restore = %d", got)
	}
	if got := s.back(a, "", cookie, gateHeaders); got.Code != http.StatusNotFound {
		t.Errorf("the way back after the restore = %d %s", got.Code, got.Body.String())
	}
	ctx := tenant.With(context.Background(), tenant.Main)
	if first, err := s.db.RememberJTI(ctx, a.id, time.Now().Add(time.Minute).Unix()); err != nil || !first {
		t.Errorf("a refused purge used the confirmation: %v %v", first, err)
	}
	// Rejected: nothing goes.
	other := s.trashed(space.ID, "Draft")
	b, cookie := s.asked("/api/trash/"+other, `{"confirm":"Draft"}`)
	s.gate.Reject(b.id)
	if got := s.back(b, "&result=rejected", cookie, gateHeaders); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "Nothing deleted") {
		t.Errorf("rejected = %d %s", got.Code, got.Body.String())
	}
	if !s.inTrash(other) {
		t.Error("a rejected confirmation deleted it")
	}
	// Agents and wrong names never reach the gate.
	before := s.gateCalls()
	if got := s.send(http.MethodDelete, "/api/trash/"+other, `{"confirm":"Draft"}`, merged2(gateHeaders, map[string]string{"Remote-Auth": "oauth", "Sec-Fetch-Site": "same-origin"})).Code; got != http.StatusForbidden {
		t.Errorf("an agent purges = %d", got)
	}
	if got := s.send(http.MethodDelete, "/api/trash/"+other, `{"confirm":"Draf"}`, sameSite).Code; got != http.StatusBadRequest {
		t.Errorf("a wrong name = %d", got)
	}
	if s.gateCalls() != before {
		t.Error("a refused purge reached the gate")
	}
}

// Emptying the trash: one confirmation, bound to its cutoff, so that nothing deleted after it goes.
func TestEmptyingTheTrashWaitsForOneConfirmation(t *testing.T) {
	s := withAccounts(t, true)
	space := s.sharedCollection(`{"title":"Team"}`, sameSite)
	first, second := s.trashed(space.ID, "One"), s.trashed(space.ID, "Two")
	a, cookie := s.asked("/api/trash", `{"confirm":"Trash"}`)
	if a.typ != "underdoc.trash.empty" || a.fields["items"] != int64(2) || a.fields["cutoff"] == nil {
		t.Errorf("the gate was asked: %+v", a)
	}
	late := s.trashed(space.ID, "Late")
	s.gate.Confirm(a.id, "1007")
	if got := s.back(a, "", cookie, gateHeaders); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "2 of your trash") {
		t.Fatalf("the way back = %d %s", got.Code, got.Body.String())
	}
	if s.inTrash(first) || s.inTrash(second) || !s.inTrash(late) {
		t.Errorf("after emptying: one %v, two %v, late %v", s.inTrash(first), s.inTrash(second), s.inTrash(late))
	}
	calls := 0
	for _, c := range s.gate.Calls() {
		if c.Path == "/auth/api/approvals" {
			calls++
		}
	}
	if calls != 1 {
		t.Errorf("emptying asked %d times", calls)
	}
}

// The approver itself: a confirmation for another object, another action or used before does not hold.
func TestTheApproverChecksTheConfirmation(t *testing.T) {
	s := withAccounts(t, true)
	client, err := gateapi.New(gateapi.Config{URL: s.gate.URL, KeyFile: s.gate.KeyFile()})
	if err != nil {
		t.Fatal(err)
	}
	approver, err := newApprover(&GateAPI{Client: client, App: "underdoc"}, s.db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := tenant.With(context.Background(), tenant.Main)
	fields := map[string]any{"object": "7k2m9q4x1c", "title": "Plan"}
	with := func(c confirmation) context.Context { return context.WithValue(ctx, confirmationKey{}, c) }
	token := s.gate.Sign("1007", "underdoc.object.purge", fields, "nonce", "apv_direct0001")
	if err := approver.Approve(ctx, db.ApprovePurge, "7k2m9q4x1c"); !errors.Is(err, db.ErrApproval) {
		t.Errorf("no confirmation: %v", err)
	}
	for name, c := range map[string]confirmation{
		"another object":  {token: token, nonce: "nonce", subject: "1007", action: db.ApprovePurge, object: "0000000000", fields: fields},
		"another person":  {token: token, nonce: "nonce", subject: "1002", action: db.ApprovePurge, object: "7k2m9q4x1c", fields: fields},
		"another browser": {token: token, nonce: "other", subject: "1007", action: db.ApprovePurge, object: "7k2m9q4x1c", fields: fields},
		"another action":  {token: token, nonce: "nonce", subject: "1007", action: db.ApproveEmptyTrash, object: "7k2m9q4x1c", fields: fields},
		"other fields":    {token: token, nonce: "nonce", subject: "1007", action: db.ApprovePurge, object: "7k2m9q4x1c", fields: map[string]any{"object": "7k2m9q4x1c", "title": "Other"}},
	} {
		if err := approver.Approve(with(c), c.action, "7k2m9q4x1c"); err == nil {
			t.Errorf("%s holds", name)
		}
	}
	right := confirmation{token: token, nonce: "nonce", subject: "1007", action: db.ApprovePurge, object: "7k2m9q4x1c", fields: fields}
	if err := approver.Approve(with(right), db.ApprovePurge, "7k2m9q4x1c"); err != nil {
		t.Fatalf("the right confirmation: %v", err)
	}
	if err := approver.Approve(with(right), db.ApprovePurge, "7k2m9q4x1c"); !errors.Is(err, errApprovalRefused) {
		t.Errorf("used twice: %v", err)
	}
	// Signed for emptying, bound to its cutoff.
	cutoff := time.Now().Unix()
	empty := map[string]any{"items": int64(2), "cutoff": cutoff * 1e6}
	token = s.gate.Sign("1007", "underdoc.trash.empty", empty, "n2", "apv_direct0002")
	c := confirmation{token: token, nonce: "n2", subject: "1007", action: db.ApproveEmptyTrash, object: strconv.FormatInt(cutoff, 10), fields: empty}
	if err := approver.Approve(with(c), db.ApproveEmptyTrash, strconv.FormatInt(cutoff+1, 10)); err == nil {
		t.Error("another cutoff holds")
	}
	if err := approver.Approve(with(c), db.ApproveEmptyTrash, strconv.FormatInt(cutoff, 10)); err != nil {
		t.Errorf("emptying: %v", err)
	}
}
