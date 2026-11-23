// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/db"
	"github.com/Ollornog/UnderDoc/internal/db/pgtest"
	"github.com/Ollornog/UnderDoc/internal/samba"
	"github.com/Ollornog/UnderDoc/internal/share"
	"github.com/Ollornog/UnderDoc/internal/store"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
)

// stubCompanion stands in for the Samba companion: it keeps the state it was given and says what is open.
type stubCompanion struct {
	mu    sync.Mutex
	state samba.State
	open  []samba.Open
	who   []samba.Event
}

func (c *stubCompanion) Do(_ context.Context, r samba.Request) (samba.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch r.Op {
	case samba.OpApply:
		c.state = *r.State
	case samba.OpOpen:
		return samba.Response{OK: true, Open: c.open}, nil
	case samba.OpWho:
		return samba.Response{OK: true, Who: c.who, First: 1, Last: uint64(len(c.who))}, nil
	}
	return samba.Response{OK: true}, nil
}

// bobOnly is Bob without the groups and mail of the headers the tests send by default.
var bobOnly = merged2(bob, map[string]string{"Remote-Groups": "", "Remote-Email": ""})

type sambaSite struct {
	*site
	db        *db.DB
	share     *share.Manager
	companion *stubCompanion
	shareDir  string
}

func withSamba(t *testing.T) *sambaSite {
	t.Helper()
	database, err := db.Open(context.Background(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(database.Close)
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	v, err := vault.New(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	companion := &stubCompanion{}
	dir := t.TempDir()
	m, err := share.New(database, st, v, share.Config{Dir: dir, Helpers: map[string]share.Helper{tenant.Main: companion}, Server: "files"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	behindGate(t, database, tenant.Main)
	s := newSite(t, Options{DB: database, Store: st, Vault: v, Share: m}, sample)
	return &sambaSite{site: s, db: database, share: m, companion: companion, shareDir: dir}
}

func (s *sambaSite) sync() {
	s.t.Helper()
	if err := s.share.Sync(context.Background(), tenant.Main); err != nil {
		s.t.Fatal(err)
	}
}

// An account makes an app password in its profile: shown once, in groups to type, its NT hash given to the companion and
// sealed in the database, the password itself kept nowhere. A new one replaces it; removing it ends the Samba user.
func TestAppPassword(t *testing.T) {
	s := withSamba(t)
	got := decode[apiSamba](t, s.getWith("/api/samba", merged2(gateHeaders, bobOnly)))
	if !got.On || got.Has || got.Suggest != "bob" || got.Server != "files" {
		t.Fatalf("before: %+v", got)
	}
	made := s.send(http.MethodPost, "/api/samba/password", `{}`, merged2(bobOnly, sameSite))
	if made.Code != http.StatusOK || made.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("make: %d %s", made.Code, made.Body)
	}
	answer := decode[map[string]string](t, made)
	if answer["name"] != "bob" || !regexp.MustCompile(`^[a-z2-9]{5}(-[a-z2-9]{5}){3}$`).MatchString(answer["password"]) {
		t.Fatalf("answer: %+v", answer)
	}
	s.sync()
	if len(s.companion.state.Users) != 1 || s.companion.state.Users[0].NTHash != samba.NTHash(answer["password"]) {
		t.Fatalf("the companion has %+v", s.companion.state.Users)
	}
	// The database keeps the NT hash sealed only.
	kept, err := s.db.SambaAccountOf(tenant.With(context.Background(), tenant.Main), "1002")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(kept.Sealed), answer["password"]) || strings.Contains(strings.ToUpper(string(kept.Sealed)), samba.NTHash(answer["password"])) {
		t.Error("the database keeps the password or its hash unsealed")
	}
	// A new one replaces it, the name stays.
	again := decode[map[string]string](t, s.send(http.MethodPost, "/api/samba/password", `{"name":"other"}`, merged2(bobOnly, sameSite)))
	if again["name"] != "bob" || again["password"] == answer["password"] {
		t.Fatalf("again: %+v", again)
	}
	s.sync()
	if s.companion.state.Users[0].NTHash != samba.NTHash(again["password"]) {
		t.Error("the companion keeps the old password")
	}
	// Only from this application, and only for oneself unless an administrator.
	if got := s.send(http.MethodPost, "/api/samba/password", `{}`, merged2(gateHeaders, bobOnly)); got.Code != http.StatusForbidden {
		t.Errorf("from elsewhere: %d", got.Code)
	}
	if got := s.send(http.MethodPost, "/api/samba/password", `{}`, sameSite); got.Code != http.StatusOK {
		t.Fatalf("Anna makes hers: %d", got.Code)
	}
	if got := s.send(http.MethodDelete, "/api/samba/accounts/1007", ``, merged2(bobOnly, sameSite)); got.Code != http.StatusForbidden {
		t.Errorf("Bob removes Anna's: %d", got.Code)
	}
	if _, err := s.db.SambaAccountOf(tenant.With(context.Background(), tenant.Main), "1007"); err != nil {
		t.Errorf("Anna's Samba user after Bob's try: %v", err)
	}
	if got := s.send(http.MethodDelete, "/api/samba/password", ``, sameSite); got.Code != http.StatusNoContent {
		t.Fatalf("Anna removes hers: %d", got.Code)
	}
	if got := s.getWith("/api/samba/accounts", merged2(gateHeaders, bobOnly)); got.Code != http.StatusForbidden {
		t.Errorf("Bob lists the Samba users: %d", got.Code)
	}
	list := decode[map[string][]map[string]string](t, s.getWith("/api/samba/accounts", gateHeaders))
	if len(list["accounts"]) != 1 || list["accounts"][0]["name"] != "bob" {
		t.Fatalf("the administrator's list: %+v", list)
	}
	if got := s.send(http.MethodDelete, "/api/samba/accounts/1002", ``, sameSite); got.Code != http.StatusNoContent {
		t.Fatalf("the administrator removes Bob's: %d %s", got.Code, got.Body)
	}
	s.sync()
	if len(s.companion.state.Users) != 0 {
		t.Errorf("the companion keeps %+v", s.companion.state.Users)
	}
	// A name that does not fit is refused.
	if got := s.send(http.MethodPost, "/api/samba/password", `{"name":"Root User"}`, merged2(bobOnly, sameSite)); got.Code != http.StatusBadRequest {
		t.Errorf("a bad name: %d", got.Code)
	}
}

// Without a companion there is nothing of Samba: the profile says so, and the rest answers 404.
func TestNoSamba(t *testing.T) {
	s := withFiles(t)
	if got := decode[apiSamba](t, s.getWith("/api/samba", gateHeaders)); got.On {
		t.Errorf("Samba on without a companion: %+v", got)
	}
	if got := s.send(http.MethodPost, "/api/samba/password", `{}`, sameSite); got.Code != http.StatusNotFound {
		t.Errorf("an app password without a companion: %d", got.Code)
	}
}

// The facts of a file in a shared library name its path on the share and who has it open there; while it is open, the
// web answers 409 to a change and names who has it.
func TestFileOnTheShare(t *testing.T) {
	s := withSamba(t)
	s.send(http.MethodPost, "/api/samba/password", `{}`, sameSite)
	fs := &filesSite{site: s.site, db: s.db}
	company := fs.root(nil, db.RootTenant)
	f := decode[finished](t, fs.put(nil, fmt.Sprintf(`"parent":%q`, company.ID), "plan.txt", []byte("plan"), 1<<20)).ID
	s.sync()
	file := decode[apiFile](t, s.getWith("/api/files/"+f, gateHeaders))
	if file.Share == nil || file.Share.Path != `\\files\Company\plan.txt` {
		t.Fatalf("share: %+v", file.Share)
	}
	s.companion.mu.Lock()
	s.companion.open = []samba.Open{{Path: s.shareDir + "/main/" + company.ID + "/plan.txt", User: "anna", Write: true}}
	s.companion.mu.Unlock()
	got := s.send(http.MethodPatch, "/api/objects/"+f, `{"title":"other.txt"}`, sameSite)
	if got.Code != http.StatusConflict || !strings.Contains(got.Body.String(), "open over Samba by anna") {
		t.Fatalf("rename of an open file: %d %s", got.Code, got.Body)
	}
	file = decode[apiFile](t, s.getWith("/api/files/"+f, gateHeaders))
	if file.Share == nil || len(file.Share.OpenBy) != 1 || file.Share.OpenBy[0] != "anna" {
		t.Errorf("open by: %+v", file.Share)
	}
}

// A manager switches the share of a collection on and off; others may not.
func TestCollectionShare(t *testing.T) {
	s := withSamba(t)
	made := decode[apiNode](t, s.send(http.MethodPost, "/api/files/collections", `{"title":"Team"}`, sameSite))
	if got := s.send(http.MethodPut, "/api/files/roots/"+made.ID+"/samba", `{"on":true}`, merged2(bobOnly, sameSite)); got.Code == http.StatusNoContent {
		t.Error("Bob switched a share he does not manage")
	}
	if got := s.send(http.MethodPut, "/api/files/roots/"+made.ID+"/samba", `{"on":true}`, sameSite); got.Code != http.StatusNoContent {
		t.Fatalf("the owner switches it on: %d %s", got.Code, got.Body)
	}
	roots, err := s.db.SharedRoots(tenant.With(context.Background(), tenant.Main))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range roots {
		found = found || r.ID == made.ID
	}
	if !found {
		t.Errorf("the collection is not shared: %+v", roots)
	}
}

// A save over SMB that the quota turns away is told to the account that saved it, in the profile and when the workspace
// opens (ADR-30, 2026-11-20); others hear nothing of it.
func TestARefusedSaveIsToldInTheWeb(t *testing.T) {
	s := withSamba(t)
	s.send(http.MethodPost, "/api/samba/password", `{}`, sameSite)
	fs := &filesSite{site: s.site, db: s.db}
	company := fs.root(nil, db.RootTenant)
	decode[finished](t, fs.put(nil, fmt.Sprintf(`"parent":%q`, company.ID), "plan.txt", []byte("plan"), 1<<20))
	s.sync()
	if err := s.db.SetQuota(tenant.With(context.Background(), tenant.Main), company.ID, 10); err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(s.shareDir, tenant.Main, company.ID, "budget.xlsx")
	if err := os.WriteFile(full, []byte("far more than ten bytes"), 0o660); err != nil {
		t.Fatal(err)
	}
	s.companion.mu.Lock()
	s.companion.who = []samba.Event{{User: "anna", Op: "renameat", Path: full}}
	s.companion.mu.Unlock()
	s.sync()
	got := decode[apiSamba](t, s.getWith("/api/samba", gateHeaders))
	if len(got.Refused) != 1 || got.Refused[0].Library != "Company" || got.Refused[0].Path != "budget.xlsx" || got.Refused[0].Why != "the quota is full" {
		t.Fatalf("the profile: %+v", got.Refused)
	}
	state := decode[apiState](t, s.getWith("/api/state", gateHeaders))
	if len(state.ShareRefused) != 1 || state.ShareRefused[0].Path != "budget.xlsx" {
		t.Errorf("the workspace: %+v", state.ShareRefused)
	}
	if other := decode[apiState](t, s.getWith("/api/state", merged2(gateHeaders, bobOnly))); len(other.ShareRefused) != 0 {
		t.Errorf("bob is told: %+v", other.ShareRefused)
	}
}
