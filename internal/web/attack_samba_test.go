// SPDX-License-Identifier: MIT

package web

// Attack round on the Samba part of the web (ADR-30). Each test is red while the weakness it names is open and turns
// green with a correct fix.

import (
	"net/http"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
)

// Any account picks the name of its Samba user freely when its login does not fit, or simply asks for another. Bob asks
// for "company": the share of the tenant, which everyone has mapped as \\files\Company, is renamed to "Company 2" at the
// next pass to make room for him, so every mapped drive of the workspace breaks (Samba then looks for the [homes] of a
// user "company" and turns everyone else away). A name that is a share already must be refused, or the shares keep
// their names.
func TestAttackSambaNameTakesShareName(t *testing.T) {
	s := withSamba(t)
	// Anna, an administrator, has her app password; the folder of the tenant is shared to her.
	if got := s.send(http.MethodPost, "/api/samba/password", `{}`, sameSite); got.Code != http.StatusOK {
		t.Fatalf("Anna's app password: %d %s", got.Code, got.Body)
	}
	fs := &filesSite{site: s.site, db: s.db}
	fs.root(nil, db.RootTenant)
	s.sync()
	before := map[string]bool{}
	for _, sh := range s.companion.state.Shares {
		before[sh.Name] = true
	}
	if !before["Company"] {
		t.Fatalf("no share Company to begin with: %+v", s.companion.state.Shares)
	}
	made := s.send(http.MethodPost, "/api/samba/password", `{"name":"company"}`, merged2(bobOnly, sameSite))
	if made.Code != http.StatusOK {
		return // refusing the name is a correct answer
	}
	s.sync()
	for _, sh := range s.companion.state.Shares {
		if sh.Name == "Company" {
			return
		}
	}
	t.Errorf("Bob's Samba user \"company\" took the name of the share of the tenant: %+v", s.companion.state.Shares)
}
