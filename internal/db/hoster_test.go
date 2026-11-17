// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/tenant"
)

func ptr[T any](v T) *T { return &v }

// TestLimitsHold: a tenant with a limit on users takes no new account past it, from the gate, OIDC or an invitation,
// while those it has keep signing in; a limit on storage stops every way that writes blocks, and shrinking always works.
func TestLimitsHold(t *testing.T) {
	db := open(t) // anna and bob in Main
	ctx := inMain()
	if err := db.SetTenantLimits(ctx, tenant.Main, HosterLimits{MaxUsers: ptr(2)}); err != nil {
		t.Fatal(err)
	}
	if err := db.SeeAccount(ctx, account.User{ID: "u-carl", Login: "carl", Name: "Carl"}); !errors.Is(err, ErrQuota) {
		t.Errorf("a third account through the gate: %v", err)
	}
	if err := db.SeeAccount(ctx, anna); err != nil {
		t.Errorf("a known account still signs in: %v", err)
	}
	hash := make([]byte, 32)
	if err := db.CreateInvite(ctx, hash, Invite{Login: "dora", Name: "Dora", Expires: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AcceptInvite(ctx, hash, NewLocal{PasswordHash: "x"}); !errors.Is(err, ErrQuota) {
		t.Errorf("a third account by invitation: %v", err)
	}
	if _, err := db.OpenInvite(ctx, hash); err != nil {
		t.Errorf("the refused invitation stays open for later: %v", err)
	}
	if err := db.SetTenantLimits(ctx, tenant.Main, HosterLimits{MaxUsers: ptr(3), MaxStorage: ptr(int64(40))}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AcceptInvite(ctx, hash, NewLocal{PasswordHash: "x"}); err != nil {
		t.Errorf("with room again: %v", err)
	}
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	p, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	if _, err := db.AddBlock(ctx, p.ID, anna.ID, "", strings.Repeat("a", 30)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddBlock(ctx, p.ID, anna.ID, "b1", strings.Repeat("b", 11)); !errors.Is(err, ErrQuota) {
		t.Errorf("a block past the limit: %v", err)
	}
	if _, err := db.EditBlock(ctx, p.ID, "b1", anna.ID, strings.Repeat("a", 41)); !errors.Is(err, ErrQuota) {
		t.Errorf("a longer block past the limit: %v", err)
	}
	if _, err := db.EditBlock(ctx, p.ID, "b1", anna.ID, "short"); err != nil {
		t.Errorf("shrinking: %v", err)
	}
	if _, err := db.AddBlock(ctx, p.ID, anna.ID, "b1", strings.Repeat("c", 30)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Copy(ctx, p.ID, anna.ID, team.ID, ""); !errors.Is(err, ErrQuota) {
		t.Errorf("a copy past the limit: %v", err)
	}
	if _, err := db.Import(ctx, "anna", "In", &Node{Object: Object{Kind: KindCollection, Title: "In"},
		Kids: []*Node{{Object: Object{Kind: KindPage, Title: "P"}, Blocks: []Block{{ID: "b1", Body: strings.Repeat("d", 10)}}}}}); !errors.Is(err, ErrQuota) {
		t.Errorf("an import past the limit: %v", err)
	}
	if kids, _ := db.Children(ctx, team.ID, anna.ID); len(kids) != 1 {
		t.Errorf("a refused copy leaves nothing behind: %v", titles(kids))
	}
	u, err := db.TenantUsage(context.Background(), tenant.Main)
	if err != nil || u.Users != 3 || u.Storage != 35 {
		t.Errorf("usage %+v %v", u, err)
	}
	// Another tenant has its own limits and its own usage.
	if _, err := db.CreateTenant(ctx, "acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	acme := tenant.With(context.Background(), "acme")
	if err := db.SeeAccount(acme, account.User{ID: "u-carl", Login: "carl", Name: "Carl"}); err != nil {
		t.Errorf("acme has no limit: %v", err)
	}
	if u, _ := db.TenantUsage(context.Background(), "acme"); u.Users != 1 || u.Storage != 0 {
		t.Errorf("acme's usage %+v", u)
	}
	if err := db.SetTenantLimits(ctx, tenant.Hoster, HosterLimits{}); !errors.Is(err, ErrReserved) {
		t.Errorf("limits of the hoster: %v", err)
	}
	if err := db.SetTenantLimits(ctx, "nobody", HosterLimits{}); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown tenant: %v", err)
	}
	if err := db.SetTenantLimits(ctx, "acme", HosterLimits{MaxUsers: ptr(-1)}); err == nil {
		t.Error("a negative limit")
	}
	l, err := db.TenantLimits(ctx, tenant.Main)
	if err != nil || *l.MaxUsers != 3 || *l.MaxStorage != 40 || l.Modules != nil {
		t.Errorf("%+v %v", l, err)
	}
}

func TestModulesOfTenantsAndUsers(t *testing.T) {
	db := open(t)
	ctx := inMain()
	allowed, on, err := db.TenantModules(ctx)
	if err != nil || allowed != nil || on != nil {
		t.Fatalf("nothing set: %v %v %v", allowed, on, err)
	}
	if err := db.SetTenantLimits(ctx, tenant.Main, HosterLimits{Modules: []string{}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetTenantModules(ctx, []string{"documents", "documents"}); err != nil {
		t.Fatal(err)
	}
	allowed, on, _ = db.TenantModules(ctx)
	if allowed == nil || len(allowed) != 0 || !slices.Equal(on, []string{"documents"}) {
		t.Errorf("%v %v", allowed, on)
	}
	if _, err := db.CreateTenant(ctx, "acme", "Acme"); err != nil {
		t.Fatal(err)
	}
	if _, on, _ := db.TenantModules(tenant.With(context.Background(), "acme")); on != nil {
		t.Errorf("another tenant's switches: %v", on)
	}
	if err := db.SetHiddenModules(ctx, anna.ID, []string{"documents"}); err != nil {
		t.Fatal(err)
	}
	if hidden, _ := db.HiddenModules(ctx, anna.ID); !slices.Equal(hidden, []string{"documents"}) {
		t.Errorf("%v", hidden)
	}
	if hidden, _ := db.HiddenModules(ctx, bob.ID); len(hidden) != 0 {
		t.Errorf("bob's: %v", hidden)
	}
	if err := db.SetHiddenModules(ctx, "nobody", nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("%v", err)
	}
}

// TestTheStorageLimitCountsFiles: the storage limit of the hoster counts pages and files together, and an upload past
// it is refused as one past a quota of the files is.
func TestTheStorageLimitCountsFiles(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.EnsureRoots(ctx, anna, true, ""); err != nil {
		t.Fatal(err)
	}
	mine := rootOfRole(t, db, anna.ID, RootUser)
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	p, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	if _, err := db.AddBlock(ctx, p.ID, anna.ID, "", strings.Repeat("a", 30)); err != nil {
		t.Fatal(err)
	}
	if err := db.SetTenantLimits(ctx, tenant.Main, HosterLimits{MaxStorage: ptr(int64(100))}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.StartUpload(ctx, anna.ID, UploadTarget{Parent: mine.ID}, "big", 71); !errors.Is(err, ErrQuota) {
		t.Errorf("an upload past the limit with the pages: %v", err)
	}
	upload(t, db, anna.ID, UploadTarget{Parent: mine.ID}, "fits", 70, sha(7))
	if _, err := db.AddBlock(ctx, p.ID, anna.ID, "b1", "x"); !errors.Is(err, ErrQuota) {
		t.Errorf("a block past the limit with the files: %v", err)
	}
	if u, _ := db.TenantUsage(context.Background(), tenant.Main); u.Storage != 100 {
		t.Errorf("usage %+v", u)
	}
}
