// SPDX-License-Identifier: MIT

package db

import (
	"errors"
	"testing"
	"time"
)

// TestShareLinks: only who manages makes a link, never on something private; the token is kept as a hash and leads to
// the link while it lives; a link to comment or edit gives its role to the accounts that open it, until it is revoked
// or expires (ADR-29).
func TestShareLinks(t *testing.T) {
	db := open(t)
	ctx := inMain()
	if err := db.SeeAccount(ctx, carl); err != nil {
		t.Fatal(err)
	}
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	page, _ := db.CreatePage(ctx, anna, "Plan", team.ID)
	if err := db.SetGrant(ctx, team.ID, Actor{ID: anna.ID}, ToTenant, "", RoleRead); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.CreateLink(ctx, page.ID, Actor{ID: bob.ID}, NewLink{Mode: LinkRead}); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob reads, he makes no link: %v", err)
	}
	if _, _, err := db.CreateLink(ctx, page.ID, Actor{ID: bob.ID, Admin: true}, NewLink{Mode: LinkRead}); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("an administrator manages rights, links are made by who manages the object: %v", err)
	}
	if _, _, err := db.CreateLink(ctx, page.ID, Actor{ID: anna.ID}, NewLink{Mode: LinkUpload}); !errors.Is(err, ErrLink) {
		t.Errorf("upload only fits a folder of the files: %v", err)
	}
	if _, _, err := db.CreateLink(ctx, page.ID, Actor{ID: anna.ID}, NewLink{Mode: LinkRead, Expires: time.Now().Add(-time.Hour)}); !errors.Is(err, ErrLink) {
		t.Errorf("an expiry in the past: %v", err)
	}
	if _, _, err := db.CreateLink(ctx, team.ID, Actor{ID: anna.ID}, NewLink{Mode: LinkRead}); !errors.Is(err, ErrLink) {
		t.Errorf("a collection of the documents is read by accounts, not through a link: %v", err)
	}
	notes, _ := db.Collections(ctx, anna.ID)
	if _, _, err := db.CreateLink(ctx, notes[0].ID, Actor{ID: anna.ID}, NewLink{Mode: LinkRead}); !errors.Is(err, ErrPrivate) {
		t.Errorf("private notes get no link: %v", err)
	}
	read, token, err := db.CreateLink(ctx, page.ID, Actor{ID: anna.ID}, NewLink{Mode: LinkRead, Frozen: "<p>frozen</p>", Released: []string{}})
	if err != nil || len(token) < 40 || read.Frozen != "<p>frozen</p>" || read.FrozenAt.IsZero() || read.Creator != anna.Name {
		t.Fatalf("a link to read: %+v %q %v", read, token, err)
	}
	var stored []byte
	if err := db.pool.QueryRow(ctx, "SELECT token FROM share_links WHERE id = $1", read.ID).Scan(&stored); err != nil || string(stored) == token {
		t.Errorf("only the hash of the token is kept: %v", err)
	}
	if l, err := db.LinkByToken(ctx, token); err != nil || l.ID != read.ID {
		t.Errorf("the token leads to the link: %+v %v", l, err)
	}
	if _, err := db.LinkByToken(ctx, token+"x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("another token leads nowhere: %v", err)
	}
	// To edit: carl opens it signed in and edits from then on.
	edit, editToken, err := db.CreateLink(ctx, team.ID, Actor{ID: anna.ID}, NewLink{Mode: LinkEdit, Expires: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if got := role(t, db, page.ID, carl.ID); got != RoleRead {
		t.Fatalf("before: %q", got)
	}
	if err := db.UseLink(ctx, edit.ID, carl.ID); err != nil {
		t.Fatal(err)
	}
	if got := role(t, db, page.ID, carl.ID); got != RoleEdit {
		t.Errorf("carl edits below the link: %q", got)
	}
	if err := db.UseLink(ctx, read.ID, bob.ID); err != nil || role(t, db, page.ID, bob.ID) != RoleRead {
		t.Errorf("a link to read gives no role: %v", err)
	}
	// Listing: anna made both; bob sees none; an administrator sees all.
	if links, err := db.Links(ctx, Actor{ID: anna.ID}, ""); err != nil || len(links) != 2 {
		t.Errorf("anna's links: %d %v", len(links), err)
	}
	if links, _ := db.Links(ctx, Actor{ID: bob.ID}, ""); len(links) != 0 {
		t.Errorf("bob sees no link: %+v", links)
	}
	if links, _ := db.Links(ctx, Actor{ID: bob.ID, Admin: true}, page.ID); len(links) != 1 || links[0].ID != read.ID {
		t.Errorf("an administrator sees every link: %+v", links)
	}
	if _, err := db.RevokeLink(ctx, edit.ID, Actor{ID: bob.ID}); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("bob revokes: %v", err)
	}
	// Expired, the role goes as well.
	if _, err := db.pool.Exec(ctx, "UPDATE share_links SET expires = now() - interval '1 second' WHERE id = $1", edit.ID); err != nil {
		t.Fatal(err)
	}
	if got := role(t, db, page.ID, carl.ID); got != RoleRead {
		t.Errorf("an expired link gives nothing: %q", got)
	}
	if _, err := db.LinkByToken(ctx, editToken); !errors.Is(err, ErrNotFound) {
		t.Errorf("an expired link leads nowhere: %v", err)
	}
	if _, err := db.pool.Exec(ctx, "UPDATE share_links SET expires = NULL WHERE id = $1", edit.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.RevokeLink(ctx, edit.ID, Actor{ID: carl.ID, Admin: true}); err != nil {
		t.Fatal(err)
	}
	if got := role(t, db, page.ID, carl.ID); got != RoleRead {
		t.Errorf("a revoked link gives nothing: %q", got)
	}
	// A link lives while its maker manages its object: bob, who managed Team, made one, then lost that.
	if err := db.SetGrant(ctx, team.ID, Actor{ID: anna.ID}, ToPerson, bob.ID, RoleManage); err != nil {
		t.Fatal(err)
	}
	bobs, bobsToken, err := db.CreateLink(ctx, team.ID, Actor{ID: bob.ID}, NewLink{Mode: LinkEdit})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UseLink(ctx, bobs.ID, carl.ID); err != nil || role(t, db, page.ID, carl.ID) != RoleEdit {
		t.Fatalf("carl edits through bob's link: %v", err)
	}
	if err := db.SetGrant(ctx, team.ID, Actor{ID: anna.ID}, ToPerson, bob.ID, RoleNone); err != nil {
		t.Fatal(err)
	}
	if _, err := db.LinkByToken(ctx, bobsToken); !errors.Is(err, ErrNotFound) {
		t.Errorf("bob's link leads nowhere once he no longer manages Team: %v", err)
	}
	if got := role(t, db, page.ID, carl.ID); got != RoleRead {
		t.Errorf("nor does it lend carl a role: %q", got)
	}
	// Passwords: a pass of one link opens no other.
	pass, err := db.GivePass(ctx, read.ID)
	if err != nil || !db.HasPass(ctx, read.ID, pass) || db.HasPass(ctx, read.ID, pass+"x") || db.HasPass(ctx, read.ID, "") {
		t.Errorf("passes: %v", err)
	}
	// In the trash, the link leads nowhere.
	if err := db.Delete(ctx, page.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.LinkByToken(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Errorf("a link to something in the trash: %v", err)
	}
}

func TestFreeNames(t *testing.T) {
	db := open(t)
	ctx := inMain()
	team, _ := db.CreateCollection(ctx, anna, "Team", "")
	for _, title := range []string{"plan.pdf", "Plan (2).pdf"} {
		if _, err := db.CreatePage(ctx, anna, title, team.ID); err != nil {
			t.Fatal(err)
		}
	}
	for name, want := range map[string]string{"report.pdf": "report.pdf", "PLAN.pdf": "PLAN (3).pdf", "notes": "notes"} {
		if got, err := db.FreeName(ctx, team.ID, name); err != nil || got != want {
			t.Errorf("%s: %q %v, want %q", name, got, err, want)
		}
	}
}
