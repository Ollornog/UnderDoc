// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"net/http"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/samba"
	"github.com/Ollornog/UnderDoc/internal/share"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

// With a domain (T-42) nobody makes an app password: the profile offers the domain account of the setup's name, the
// Samba user has no hash, and the companion gets the state of a domain.
func TestSambaDomainAccount(t *testing.T) {
	s := withSambaConfig(t, share.Config{Domain: map[string]bool{tenant.Main: true}})
	got := decode[apiSamba](t, s.getWith("/api/samba", merged2(gateHeaders, bobOnly)))
	if !got.On || !got.Domain || got.Has || got.Suggest != "bob" {
		t.Fatalf("before: %+v", got)
	}
	if got := s.send(http.MethodPost, "/api/samba/password", `{}`, merged2(bobOnly, sameSite)); got.Code != http.StatusConflict {
		t.Fatalf("an app password with a domain: %d %s", got.Code, got.Body)
	}
	// Only from this application.
	if got := s.send(http.MethodPost, "/api/samba/domain", `{}`, merged2(gateHeaders, bobOnly)); got.Code != http.StatusForbidden {
		t.Errorf("from elsewhere: %d", got.Code)
	}
	made := s.send(http.MethodPost, "/api/samba/domain", `{"name":"anna"}`, merged2(bobOnly, sameSite))
	if made.Code != http.StatusOK {
		t.Fatalf("the domain account: %d %s", made.Code, made.Body)
	}
	// The name is the setup's, never one the account chooses.
	if answer := decode[map[string]string](t, made); answer["name"] != "bob" {
		t.Fatalf("answer: %+v", answer)
	}
	s.sync()
	st := s.companion.state
	if st.Mode != samba.ModeDomain || len(st.Users) != 1 || st.Users[0].Name != "bob" || st.Users[0].NTHash != "" || st.Users[0].UID < 20000 {
		t.Fatalf("the companion has %+v", st)
	}
	kept, err := s.db.SambaAccountOf(tenant.With(context.Background(), tenant.Main), "1002")
	if err != nil || kept.Sealed != nil {
		t.Fatalf("kept: %+v %v", kept, err)
	}
	got = decode[apiSamba](t, s.getWith("/api/samba", merged2(gateHeaders, bobOnly)))
	if !got.Has || got.Name != "bob" || !got.Domain {
		t.Fatalf("after: %+v", got)
	}
	// Stopping SMB removes the Samba user, as removing an app password does.
	if got := s.send(http.MethodDelete, "/api/samba/password", ``, merged2(bobOnly, sameSite)); got.Code != http.StatusNoContent {
		t.Fatalf("stop: %d", got.Code)
	}
	s.sync()
	if len(s.companion.state.Users) != 0 {
		t.Errorf("the companion keeps %+v", s.companion.state.Users)
	}
}

// With the setup's names from the gate (Remote-Id), the domain account is the number the gate gives the account.
func TestSambaDomainAccountByRemoteID(t *testing.T) {
	s := withSambaConfig(t, share.Config{Domain: map[string]bool{tenant.Main: true}, DomainNames: share.DomainByRemoteID})
	if got := decode[apiSamba](t, s.getWith("/api/samba", merged2(gateHeaders, bobOnly))); got.Suggest != "1002" {
		t.Fatalf("suggest: %+v", got)
	}
	made := decode[map[string]string](t, s.send(http.MethodPost, "/api/samba/domain", `{}`, merged2(bobOnly, sameSite)))
	if made["name"] != "1002" {
		t.Fatalf("made: %+v", made)
	}
}

// Without a domain the share takes app passwords only: a domain account is refused.
func TestSambaNoDomainAccountWithAppPasswords(t *testing.T) {
	s := withSamba(t)
	if got := s.send(http.MethodPost, "/api/samba/domain", `{}`, merged2(bobOnly, sameSite)); got.Code != http.StatusConflict {
		t.Fatalf("a domain account without a domain: %d %s", got.Code, got.Body)
	}
	if got := decode[apiSamba](t, s.getWith("/api/samba", merged2(gateHeaders, bobOnly))); got.Domain {
		t.Errorf("the profile tells a domain: %+v", got)
	}
}
