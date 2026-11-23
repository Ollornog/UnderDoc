// SPDX-License-Identifier: MIT

package db

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/account"
	"github.com/Ollornog/UnderDoc/internal/tenant"
	"github.com/Ollornog/UnderDoc/internal/vault"
)

// fakeSeal stands in for the vault in the tests of the database, which never opens a value: it keeps the name a value is
// bound to beside it, so a test sees which version came back.
func fakeSeal(value string) Seal {
	return func(name string) ([]byte, error) { return []byte(name + "=" + value), nil }
}

// randomValue makes a value for a test, so no password is written down anywhere.
func randomValue(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// secretPage makes a collection of anna that the tenant reads, with a page of anna in it, and carl as a third account.
func secretPage(t *testing.T, db *DB) (Object, Object) {
	t.Helper()
	ctx := inMain()
	if err := db.SeeAccount(ctx, carl); err != nil {
		t.Fatal(err)
	}
	ops, err := db.CreateCollection(ctx, anna, "Ops", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetGrant(ctx, ops.ID, Actor{ID: anna.ID}, ToTenant, "", RoleRead); err != nil {
		t.Fatal(err)
	}
	page, err := db.CreatePage(ctx, anna, "Router", ops.ID)
	if err != nil {
		t.Fatal(err)
	}
	return ops, page
}

func reveal(db *DB, secretID, viewer string) (string, error) {
	sealed, _, err := db.RevealPageSecret(inMain(), secretID, viewer, 0, false)
	return string(sealed), err
}

// TestWhoRevealsASecret: whoever reads the page reveals its secret by default; someone without a role, a role lent by a
// share link, and a reader left out of a restriction do not, and get the same answer as for an ID that names nothing.
// Only who manages the page restricts, and there is only narrowing (ADR-7).
func TestWhoRevealsASecret(t *testing.T) {
	db := open(t)
	ctx := inMain()
	dave := account.User{ID: "u-dave", Login: "dave", Name: "Dave"}
	if err := db.SeeAccount(ctx, dave); err != nil {
		t.Fatal(err)
	}
	_, page := secretPage(t, db)
	value := randomValue(t)
	if _, err := db.CreatePageSecret(ctx, page.ID, carl.ID, fakeSeal(value)); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("a reader made a secret: %v", err)
	}
	secretID, err := db.CreatePageSecret(ctx, page.ID, anna.ID, fakeSeal(value))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := reveal(db, secretID, carl.ID); err != nil || got != SecretSealName(secretID, 1)+"="+value {
		t.Fatalf("a reader of the page does not reveal it: %q %v", got, err)
	}
	// dave reads nothing of Ops: he loses the read of the tenant by a broken inheritance on the page.
	if err := db.SetInherit(ctx, page.ID, Actor{ID: anna.ID}, false); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGrant(ctx, page.ID, Actor{ID: anna.ID}, ToTenant, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGrant(ctx, page.ID, Actor{ID: anna.ID}, ToPerson, carl.ID, RoleRead); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGrant(ctx, page.ID, Actor{ID: anna.ID}, ToPerson, bob.ID, RoleRead); err != nil {
		t.Fatal(err)
	}
	if _, err := reveal(db, secretID, dave.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("someone without a role on the page revealed it: %v", err)
	}
	if _, err := reveal(db, "zzzzzzzzzz", carl.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("an ID without a secret: %v", err)
	}
	// A link to edit the page, opened by dave, lends him edit; it lends no secret.
	link, _, err := db.CreateLink(ctx, page.ID, Actor{ID: anna.ID}, NewLink{Mode: "edit"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UseLink(ctx, link.ID, dave.ID); err != nil {
		t.Fatal(err)
	}
	if got := role(t, db, page.ID, dave.ID); got != RoleEdit {
		t.Fatalf("the link lends dave %q, the test needs edit", got)
	}
	if _, err := reveal(db, secretID, dave.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a role lent by a link revealed it: %v", err)
	}
	// Restricted to bob: carl reads the page and no longer reveals; neither does anna, who owns it, until she adds herself.
	if err := db.SetPageSecretReaders(ctx, secretID, carl.ID, true, []SecretReader{{Kind: ToPerson, Who: carl.ID}}); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("a reader restricted it: %v", err)
	}
	if err := db.SetPageSecretReaders(ctx, secretID, anna.ID, true, []SecretReader{{Kind: ToPerson, Who: bob.ID}}); err != nil {
		t.Fatal(err)
	}
	for viewer, want := range map[string]bool{bob.ID: true, carl.ID: false, anna.ID: false, dave.ID: false} {
		if _, err := reveal(db, secretID, viewer); (err == nil) != want {
			t.Errorf("restricted to bob, %s reveals: %v", viewer, err)
		}
	}
	// A reader also has to read the page: dave named as a reader still reads nothing.
	ops2 := []SecretReader{{Kind: ToPerson, Who: anna.ID}, {Kind: ToPerson, Who: dave.ID}}
	if err := db.SetPageSecretReaders(ctx, secretID, anna.ID, true, ops2); err != nil {
		t.Fatal(err)
	}
	if _, err := reveal(db, secretID, dave.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a named reader without a role on the page revealed it: %v", err)
	}
	if _, err := reveal(db, secretID, anna.ID); err != nil {
		t.Errorf("anna added herself and does not reveal it: %v", err)
	}
	// A group counts: anna's group from the sign-in.
	group, err := db.CreateGroup(ctx, Actor{ID: anna.ID, Admin: true}, "Network", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetMember(ctx, Actor{ID: anna.ID, Admin: true}, group.ID, carl.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPageSecretReaders(ctx, secretID, anna.ID, true, []SecretReader{{Kind: ToGroup, Who: group.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := reveal(db, secretID, carl.ID); err != nil {
		t.Errorf("a member of a reading group does not reveal it: %v", err)
	}
	if err := db.DeleteGroup(ctx, Actor{ID: anna.ID, Admin: true}, group.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reveal(db, secretID, carl.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("the group is gone and its member still reveals it: %v", err)
	}
	if err := db.SetPageSecretReaders(ctx, secretID, anna.ID, true, []SecretReader{{Kind: ToPerson, Who: "u-nobody"}}); !errors.Is(err, ErrReader) {
		t.Errorf("an unknown reader: %v", err)
	}
	// Back to whoever reads the page.
	if err := db.SetPageSecretReaders(ctx, secretID, anna.ID, false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := reveal(db, secretID, carl.ID); err != nil {
		t.Errorf("no longer restricted, a reader does not reveal it: %v", err)
	}
}

// TestVersionsOfASecret: every value is a version with who and when; an older one is revealed and restored only with
// edit on the page, and a restore makes a new version. Deleting takes every version.
func TestVersionsOfASecret(t *testing.T) {
	db := open(t)
	ctx := inMain()
	_, page := secretPage(t, db)
	first, second := randomValue(t), randomValue(t)
	secretID, err := db.CreatePageSecret(ctx, page.ID, anna.ID, fakeSeal(first))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ChangePageSecret(ctx, secretID, carl.ID, fakeSeal(second)); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("a reader changed it: %v", err)
	}
	if err := db.ChangePageSecret(ctx, secretID, anna.ID, fakeSeal(second)); err != nil {
		t.Fatal(err)
	}
	if got, _ := reveal(db, secretID, carl.ID); got != SecretSealName(secretID, 2)+"="+second {
		t.Errorf("the newest version: %q", got)
	}
	if _, _, err := db.RevealPageSecret(ctx, secretID, carl.ID, 1, false); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("a reader revealed an older version: %v", err)
	}
	if sealed, name, err := db.RevealPageSecret(ctx, secretID, anna.ID, 1, false); err != nil || string(sealed) != SecretSealName(secretID, 1)+"="+first || name != SecretSealName(secretID, 1) {
		t.Errorf("version 1: %q %q %v", sealed, name, err)
	}
	err = db.RestorePageSecret(ctx, secretID, anna.ID, 1, func(oldName string, old []byte, newName string) ([]byte, error) {
		value := strings.TrimPrefix(string(old), oldName+"=")
		return []byte(newName + "=" + value), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := reveal(db, secretID, carl.ID); got != SecretSealName(secretID, 3)+"="+first {
		t.Errorf("after restoring version 1: %q", got)
	}
	s, err := db.PageSecretOf(ctx, secretID, anna.ID)
	if err != nil || len(s.Versions) != 3 || s.Versions[0].Origin != "restored from version 1" || s.Versions[2].Name != anna.Name {
		t.Errorf("versions: %+v %v", s.Versions, err)
	}
	if s, err := db.PageSecretOf(ctx, secretID, carl.ID); err != nil || len(s.Versions) != 0 || !s.May || s.MayChange {
		t.Errorf("a reader sees versions or may change: %+v %v", s, err)
	}
	if err := db.DeletePageSecret(ctx, secretID, carl.ID); !errors.Is(err, ErrNotAllowed) {
		t.Errorf("a reader deleted it: %v", err)
	}
	if err := db.DeletePageSecret(ctx, secretID, anna.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM page_secret_versions WHERE secret = $1", secretID).Scan(&n); err != nil || n != 0 {
		t.Errorf("%d versions left after deleting: %v", n, err)
	}
}

// TestASecretGoesWithItsPage: in the trash it reveals nothing, restored it does again, and purged it is gone; a copy of
// the page keeps the reference but not the secret, which stays with its page.
func TestASecretGoesWithItsPage(t *testing.T) {
	db := open(t)
	ctx := inMain()
	ops, page := secretPage(t, db)
	secretID, err := db.CreatePageSecret(ctx, page.ID, anna.ID, fakeSeal(randomValue(t)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddBlock(ctx, page.ID, anna.ID, "", "Admin: ::secret{id=\""+secretID+"\"}"); err != nil {
		t.Fatal(err)
	}
	copied, err := db.Copy(ctx, page.ID, anna.ID, ops.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	states, err := db.SecretStates(ctx, anna.ID, []string{secretID, "zzzzzzzzzz"})
	if err != nil || len(states) != 1 || states[secretID].Page != page.ID || !states[secretID].May {
		t.Errorf("states: %+v %v", states, err)
	}
	var n int
	if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM page_secrets WHERE page = $1", copied.ID).Scan(&n); err != nil || n != 0 {
		t.Errorf("the copy got %d secrets of its own: %v", n, err)
	}
	if list, _ := db.PageSecrets(ctx, page.ID, carl.ID); len(list) != 1 || !list[0].Referenced || !list[0].May {
		t.Errorf("the secrets of the page: %+v", list)
	}
	if err := db.Delete(ctx, page.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reveal(db, secretID, anna.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a secret in the trash was revealed: %v", err)
	}
	if _, err := db.Restore(ctx, page.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reveal(db, secretID, anna.ID); err != nil {
		t.Errorf("restored, it does not reveal: %v", err)
	}
	if err := db.Delete(ctx, page.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Purge(ctx, page.ID, anna.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.pool.QueryRow(ctx, "SELECT count(*) FROM page_secret_versions WHERE secret = $1", secretID).Scan(&n); err != nil || n != 0 {
		t.Errorf("purged, %d versions are left: %v", n, err)
	}
}

// TestMovingABlockTakesItsSecrets: moving a block to another page takes the secrets of its page along, if the mover may
// reveal them; otherwise the block stays.
func TestMovingABlockTakesItsSecrets(t *testing.T) {
	db := open(t)
	ctx := inMain()
	ops, page := secretPage(t, db)
	other, err := db.CreatePage(ctx, anna, "Firewall", ops.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetGrant(ctx, ops.ID, Actor{ID: anna.ID}, ToPerson, bob.ID, RoleEdit); err != nil {
		t.Fatal(err)
	}
	secretID, err := db.CreatePageSecret(ctx, page.ID, anna.ID, fakeSeal(randomValue(t)))
	if err != nil {
		t.Fatal(err)
	}
	b, err := db.AddBlock(ctx, page.ID, anna.ID, "", "Admin: ::secret{id=\""+secretID+"\"}")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetPageSecretReaders(ctx, secretID, anna.ID, true, []SecretReader{{Kind: ToPerson, Who: anna.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.MoveBlockTo(ctx, page.ID, b.ID, bob.ID, other.ID, ""); !errors.Is(err, ErrSecretHeld) {
		t.Fatalf("bob, who may not reveal it, moved it: %v", err)
	}
	if blocks, _ := db.Blocks(ctx, page.ID, anna.ID); len(blocks) != 1 {
		t.Errorf("the refused move took the block: %d left", len(blocks))
	}
	if _, err := db.MoveBlockTo(ctx, page.ID, b.ID, anna.ID, other.ID, ""); err != nil {
		t.Fatal(err)
	}
	states, _ := db.SecretStates(ctx, anna.ID, []string{secretID})
	if states[secretID].Page != other.ID || !states[secretID].May {
		t.Errorf("the secret did not move with its block: %+v", states)
	}
	if s, err := db.PageSecretOf(ctx, secretID, anna.ID); err != nil || !s.Restricted || len(s.Readers) != 1 {
		t.Errorf("the restriction did not move along: %+v %v", s, err)
	}
}

// TestTheLogOfSecrets: making, revealing, copying, changing, restricting and deleting each leave an entry on the page,
// with the ID and never the value; nothing of a private page is logged.
func TestTheLogOfSecrets(t *testing.T) {
	db := open(t)
	ctx := inMain()
	_, page := secretPage(t, db)
	value := randomValue(t)
	secretID, err := db.CreatePageSecret(ctx, page.ID, anna.ID, fakeSeal(value))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reveal(db, secretID, carl.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.RevealPageSecret(ctx, secretID, carl.ID, 0, true); err != nil {
		t.Fatal(err)
	}
	if err := db.ChangePageSecret(ctx, secretID, anna.ID, fakeSeal(value+"2")); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPageSecretReaders(ctx, secretID, anna.ID, true, []SecretReader{{Kind: ToPerson, Who: anna.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeletePageSecret(ctx, secretID, anna.ID); err != nil {
		t.Fatal(err)
	}
	entries, err := db.LogEntries(ctx, Actor{ID: anna.ID, Admin: true}, LogFilter{Object: page.ID})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, e := range entries {
		if strings.Contains(e.Detail, value) {
			t.Errorf("the log holds the value: %+v", e)
		}
		if strings.HasPrefix(e.Action, "secret ") {
			seen[e.Action] = e.Actor
			if !strings.Contains(e.Detail, secretID) {
				t.Errorf("an entry without the ID of the secret: %+v", e)
			}
		}
	}
	for action, actor := range map[string]string{LogSecretMade: anna.ID, LogSecretRevealed: carl.ID, LogSecretCopied: carl.ID,
		LogSecretChanged: anna.ID, LogSecretReaders: anna.ID, LogSecretDeleted: anna.ID} {
		if seen[action] != actor {
			t.Errorf("%q logged for %q, want %q", action, seen[action], actor)
		}
	}
	// A private page: nothing is logged.
	notes, err := db.Collections(ctx, anna.ID)
	if err != nil {
		t.Fatal(err)
	}
	var mine Object
	for _, c := range notes {
		if c.Private {
			mine = c
		}
	}
	note, err := db.CreatePage(ctx, anna, "Mine", mine.ID)
	if err != nil {
		t.Fatal(err)
	}
	private, err := db.CreatePageSecret(ctx, note.ID, anna.ID, fakeSeal(value))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reveal(db, private, anna.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reveal(db, private, bob.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("someone else revealed a private secret: %v", err)
	}
	if entries, _ := db.LogEntries(ctx, Actor{ID: anna.ID, Admin: true}, LogFilter{Object: note.ID}); len(entries) != 0 {
		t.Errorf("a private secret was logged: %+v", entries)
	}
}

// TestSecretsKeepTenantsApart: an ID of another tenant names nothing.
func TestSecretsKeepTenantsApart(t *testing.T) {
	db := open(t)
	ctx := inMain()
	_, page := secretPage(t, db)
	secretID, err := db.CreatePageSecret(ctx, page.ID, anna.ID, fakeSeal(randomValue(t)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateTenant(ctx, "acme", "Acme Ltd"); err != nil {
		t.Fatal(err)
	}
	acme := tenant.With(context.Background(), "acme")
	if err := db.SeeAccount(acme, anna); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.RevealPageSecret(acme, secretID, anna.ID, 0, false); !errors.Is(err, ErrNotFound) {
		t.Errorf("another tenant revealed it: %v", err)
	}
	if states, err := db.SecretStates(acme, anna.ID, []string{secretID}); err != nil || len(states) != 0 {
		t.Errorf("another tenant sees it: %+v %v", states, err)
	}
}

// TestRekey: every sealed value of a tenant opens with the new key after a rotation, the passwords of pages with every
// version and the secrets of other systems; a second run changes nothing, and a value no key opens stops it with
// nothing changed.
func TestRekey(t *testing.T) {
	db := open(t)
	ctx := inMain()
	key := func() string {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			t.Fatal(err)
		}
		return base64.StdEncoding.EncodeToString(raw)
	}
	oldKey, newKey := key(), key()
	before, _ := vault.New(oldKey)
	seal := func(value string) Seal {
		return func(name string) ([]byte, error) { return before.Seal(tenant.Main, name, vault.Secret(value)) }
	}
	_, page := secretPage(t, db)
	first, second, other := randomValue(t), randomValue(t), randomValue(t)
	secretID, err := db.CreatePageSecret(ctx, page.ID, anna.ID, seal(first))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ChangePageSecret(ctx, secretID, anna.ID, seal(second)); err != nil {
		t.Fatal(err)
	}
	sealedOther, _ := before.Seal(tenant.Main, "source.token", vault.Secret(other))
	if err := db.PutSealedSecret(ctx, "source.token", sealedOther); err != nil {
		t.Fatal(err)
	}
	both, err := vault.NewRotating(newKey, oldKey)
	if err != nil {
		t.Fatal(err)
	}
	reseal := func(name string, sealed []byte) ([]byte, error) { return both.Reseal(tenant.Main, name, sealed) }
	if n, err := db.Rekey(ctx, tenant.Main, reseal); err != nil || n != 3 {
		t.Fatalf("rekey: %d %v", n, err)
	}
	if n, err := db.Rekey(ctx, tenant.Main, reseal); err != nil || n != 0 {
		t.Errorf("a second run changed %d: %v", n, err)
	}
	after, _ := vault.New(newKey)
	for number, want := range map[int]string{1: first, 2: second} {
		sealed, name, err := db.RevealPageSecret(ctx, secretID, anna.ID, number, false)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := after.Open(tenant.Main, name, sealed); err != nil || string(got) != want {
			t.Errorf("version %d does not open with the new key: %v", number, err)
		}
	}
	gotOther, _ := db.SealedSecret(ctx, "source.token")
	if got, err := after.Open(tenant.Main, "source.token", gotOther); err != nil || string(got) != other {
		t.Errorf("the secret of another system does not open with the new key: %v", err)
	}
	// A value sealed by a third key opens with neither: nothing changes.
	stranger, _ := vault.New(key())
	foreign, _ := stranger.Seal(tenant.Main, "mail", vault.Secret(randomValue(t)))
	if err := db.PutSealedSecret(ctx, "mail", foreign); err != nil {
		t.Fatal(err)
	}
	if err := db.ChangePageSecret(ctx, secretID, anna.ID, seal(randomValue(t))); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Rekey(ctx, tenant.Main, reseal); err == nil || strings.Contains(err.Error(), "mail=") {
		t.Errorf("a value no key opens: %v", err)
	}
	sealed, name, _ := db.RevealPageSecret(ctx, secretID, anna.ID, 0, false)
	if _, err := after.Open(tenant.Main, name, sealed); !errors.Is(err, vault.ErrSealed) {
		t.Errorf("the failed run changed a value: %v", err)
	}
}

// TestASecretStaysOutOfTheSearch: the records of a page for the search hold neither the reference nor the ID of its
// secret, while the text around it is found.
func TestASecretStaysOutOfTheSearch(t *testing.T) {
	db := open(t)
	ctx := inMain()
	_, page := secretPage(t, db)
	secretID, err := db.CreatePageSecret(ctx, page.ID, anna.ID, fakeSeal(randomValue(t)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddBlock(ctx, page.ID, anna.ID, "", "Router admin: ::secret{id=\""+secretID+"\"} rotated yearly"); err != nil {
		t.Fatal(err)
	}
	docs, err := db.SearchSource().Docs(ctx, page.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range docs {
		raw, _ := json.Marshal(d)
		if strings.Contains(string(raw), secretID) || strings.Contains(string(raw), "secret{") {
			t.Errorf("a record of the search holds the secret: %s", raw)
		}
		found = found || strings.Contains(string(raw), "rotated yearly")
	}
	if !found {
		t.Errorf("the text around the secret is not in the search: %+v", docs)
	}
}

// TestASealedValueBelongsToItsVersion: a sealed value copied to another version of the same secret, or to another
// secret, does not open, since the vault binds it to the secret and the number of its version (ADR-7).
func TestASealedValueBelongsToItsVersion(t *testing.T) {
	db := open(t)
	ctx := inMain()
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	keys, _ := vault.New(base64.StdEncoding.EncodeToString(raw))
	seal := func(value string) Seal {
		return func(name string) ([]byte, error) { return keys.Seal(tenant.Main, name, vault.Secret(value)) }
	}
	_, page := secretPage(t, db)
	first, err := db.CreatePageSecret(ctx, page.ID, anna.ID, seal(randomValue(t)))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ChangePageSecret(ctx, first, anna.ID, seal(randomValue(t))); err != nil {
		t.Fatal(err)
	}
	other, err := db.CreatePageSecret(ctx, page.ID, anna.ID, seal(randomValue(t)))
	if err != nil {
		t.Fatal(err)
	}
	// Version 1 of the first secret copied over its version 2, and over the only version of the other.
	for _, target := range []struct {
		secret string
		number int
	}{{first, 2}, {other, 1}} {
		if _, err := db.pool.Exec(ctx, `UPDATE page_secret_versions SET sealed = (SELECT sealed FROM page_secret_versions WHERE secret = $1 AND number = 1)
			WHERE secret = $2 AND number = $3`, first, target.secret, target.number); err != nil {
			t.Fatal(err)
		}
		sealed, name, err := db.RevealPageSecret(ctx, target.secret, anna.ID, target.number, false)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := keys.Open(tenant.Main, name, sealed); !errors.Is(err, vault.ErrSealed) {
			t.Errorf("a value copied to %s version %d opens: %v", target.secret, target.number, err)
		}
	}
}

// TestRevealsCountedAtOnce: many reveals of one account at the same moment, each of another secret, stay within the
// limit of a minute: the log counts them one after another (ADR-7).
func TestRevealsCountedAtOnce(t *testing.T) {
	db := open(t)
	ctx := inMain()
	_, page := secretPage(t, db)
	const many = 3 * RevealsPerWindow
	ids := make([]string, many)
	for i := range ids {
		var err error
		if ids[i], err = db.CreatePageSecret(ctx, page.ID, anna.ID, fakeSeal("value")); err != nil {
			t.Fatal(err)
		}
	}
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		revealed int
	)
	start := make(chan struct{})
	for _, secretID := range ids {
		wg.Go(func() {
			<-start
			_, err := reveal(db, secretID, carl.ID)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				revealed++
			case !errors.Is(err, ErrTooManyReveals):
				t.Errorf("reveal: %v", err)
			}
		})
	}
	close(start)
	wg.Wait()
	if revealed != RevealsPerWindow {
		t.Errorf("%d of %d reveals at once went through, want %d", revealed, many, RevealsPerWindow)
	}
}
