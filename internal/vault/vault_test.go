// SPDX-License-Identifier: MIT

package vault

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

// testKey is a key for tests only: 32 bytes in base64.
const testKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func TestSealAndOpen(t *testing.T) {
	v, err := New(testKey)
	if err != nil || !v.Ready() {
		t.Fatal(err)
	}
	sealed, err := v.Seal("acme", "oidc", Secret("s3cret"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("s3cret")) {
		t.Error("the value shows in what is sealed")
	}
	again, _ := v.Seal("acme", "oidc", Secret("s3cret"))
	if bytes.Equal(sealed, again) {
		t.Error("the same value seals the same twice: the nonce does not change")
	}
	if got, err := v.Open("acme", "oidc", sealed); err != nil || string(got) != "s3cret" {
		t.Errorf("%q %v", got, err)
	}
	for _, c := range []struct{ tenant, name string }{{"main", "oidc"}, {"acme", "mail"}} {
		if _, err := v.Open(c.tenant, c.name, sealed); !errors.Is(err, ErrSealed) {
			t.Errorf("opened as %s/%s: %v", c.tenant, c.name, err)
		}
	}
	broken := bytes.Clone(sealed)
	broken[len(broken)-1] ^= 1
	if _, err := v.Open("acme", "oidc", broken); !errors.Is(err, ErrSealed) {
		t.Errorf("a changed value opens: %v", err)
	}
	if _, err := v.Open("acme", "oidc", []byte{1}); !errors.Is(err, ErrSealed) {
		t.Errorf("a short value: %v", err)
	}
	other, _ := New("ZmVkY2JhOTg3NjU0MzIxMGZlZGNiYTk4NzY1NDMyMTA=")
	if _, err := other.Open("acme", "oidc", sealed); !errors.Is(err, ErrSealed) {
		t.Errorf("another key opens: %v", err)
	}
}

func TestKeys(t *testing.T) {
	v, err := New("")
	if err != nil || v.Ready() {
		t.Fatalf("no key: %v", err)
	}
	if _, err := v.Seal("a", "b", Secret("x")); !errors.Is(err, ErrNoKey) {
		t.Errorf("seal without a key: %v", err)
	}
	if _, err := v.Open("a", "b", []byte("x")); !errors.Is(err, ErrNoKey) {
		t.Errorf("open without a key: %v", err)
	}
	var none *Vault
	if none.Ready() {
		t.Error("nil is ready")
	}
	for _, bad := range []string{"short", "MDEyMzQ1Njc4OWFiY2RlZg==", "!!!"} {
		if _, err := New(bad); !errors.Is(err, ErrKey) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestSecretNeverShows(t *testing.T) {
	s := Secret("s3cret")
	var log bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&log, nil))
	logger.Info("x", slog.Any("secret", s), slog.String("str", s.String()))
	logger.Info("y", "secret", s)
	text := slog.New(slog.NewTextHandler(&log, nil))
	text.Info("z", "secret", s)
	j, _ := json.Marshal(map[string]any{"s": s})
	all := log.String() + string(j) + fmt.Sprintf("%v %s %#v %+v", s, s, s, s)
	if strings.Contains(all, "s3cret") {
		t.Errorf("the secret shows: %s", all)
	}
	if !strings.Contains(all, "[hidden]") {
		t.Errorf("no mark: %s", all)
	}
}

// newKey makes a fresh key for a test.
func newKey(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// TestRotation: during a rotation the vault opens what the old key sealed, seals only with the new key, and Reseal moves
// a value to the new key once; afterwards the new key alone opens it (ADR-7).
func TestRotation(t *testing.T) {
	oldKey, newKeyText := newKey(t), newKey(t)
	before, _ := New(oldKey)
	sealed, err := before.Seal("acme", "page:7k2m9q4x1c:1", Secret("hunter2"))
	if err != nil {
		t.Fatal(err)
	}
	fresh, _ := New(newKeyText)
	if _, err := fresh.Open("acme", "page:7k2m9q4x1c:1", sealed); !errors.Is(err, ErrSealed) {
		t.Fatalf("the new key alone opens the old value: %v", err)
	}
	both, err := NewRotating(newKeyText, oldKey)
	if err != nil || !both.Rotating() {
		t.Fatal(err)
	}
	if got, err := both.Open("acme", "page:7k2m9q4x1c:1", sealed); err != nil || string(got) != "hunter2" {
		t.Fatalf("the rotating vault does not open the old value: %v", err)
	}
	if _, err := both.Open("acme", "page:7k2m9q4x1c:2", sealed); !errors.Is(err, ErrSealed) {
		t.Errorf("the old key opens under another name: %v", err)
	}
	again, err := both.Reseal("acme", "page:7k2m9q4x1c:1", sealed)
	if err != nil || again == nil {
		t.Fatalf("Reseal: %v", err)
	}
	if got, err := fresh.Open("acme", "page:7k2m9q4x1c:1", again); err != nil || string(got) != "hunter2" {
		t.Fatalf("the resealed value does not open with the new key: %v", err)
	}
	if more, err := both.Reseal("acme", "page:7k2m9q4x1c:1", again); err != nil || more != nil {
		t.Errorf("Reseal changed what the new key already opens: %v", err)
	}
	if _, err := both.Reseal("acme", "page:7k2m9q4x1c:1", []byte("not sealed by anyone, long enough")); !errors.Is(err, ErrSealed) {
		t.Errorf("Reseal of a value no key opens: %v", err)
	}
	if fresh.Rotating() {
		t.Error("a vault with one key says it rotates")
	}
	if _, err := NewRotating("", oldKey); !errors.Is(err, ErrOldOnly) {
		t.Errorf("an old key without a new one: %v", err)
	}
	if _, err := NewRotating(newKeyText, "short"); !errors.Is(err, ErrOldKey) {
		t.Errorf("a broken old key: %v", err)
	}
}
