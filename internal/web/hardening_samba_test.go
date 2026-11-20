// SPDX-License-Identifier: MIT

package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
)

// A new Samba user may not take the name of a share, in any case: the answer is 400 and says why; another name passes.
func TestASambaNameOfAShareIsRefused(t *testing.T) {
	s := withSamba(t)
	if got := s.send(http.MethodPost, "/api/samba/password", `{}`, sameSite); got.Code != http.StatusOK {
		t.Fatalf("Anna's app password: %d %s", got.Code, got.Body)
	}
	fs := &filesSite{site: s.site, db: s.db}
	fs.root(nil, db.RootTenant)
	s.sync()
	for _, name := range []string{"company", "COMPANY"} {
		got := s.send(http.MethodPost, "/api/samba/password", `{"name":"`+name+`"}`, merged2(bobOnly, sameSite))
		if got.Code != http.StatusBadRequest || !strings.Contains(got.Body.String(), "a share has this name") {
			t.Errorf("%s: %d %s", name, got.Code, got.Body)
		}
	}
	if got := s.send(http.MethodPost, "/api/samba/password", `{"name":"bobby"}`, merged2(bobOnly, sameSite)); got.Code != http.StatusOK {
		t.Errorf("another name: %d %s", got.Code, got.Body)
	}
}
