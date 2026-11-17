// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestPasswords(t *testing.T) {
	hash, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("not Argon2id with the chosen parameters: %s", hash)
	}
	if again, _ := HashPassword("correct horse battery"); again == hash {
		t.Error("no salt")
	}
	if !VerifyPassword(hash, "correct horse battery") {
		t.Error("the right password")
	}
	for _, wrong := range []string{"correct horse batterY", "", "correct horse battery ", strings.Repeat("x", 300)} {
		if VerifyPassword(hash, wrong) {
			t.Errorf("%q passes", wrong)
		}
	}
	if VerifyPassword("", "correct horse battery") {
		t.Error("an account without a password")
	}
	for _, bad := range []string{"x", "$argon2i$v=19$m=1,t=1,p=1$c2FsdA$aGFzaA", "$argon2id$v=19$m=9999999,t=1,p=1$c2FsdA$aGFzaA",
		"$argon2id$v=19$m=8,t=1,p=0$c2FsdA$aGFzaA", "$argon2id$v=18$m=8,t=1,p=1$c2FsdA$aGFzaA"} {
		if VerifyPassword(bad, "x") {
			t.Errorf("%q passes", bad)
		}
	}
	for _, short := range []string{"", "eleven char", strings.Repeat("ä", 201)} {
		if _, err := HashPassword(short); err != ErrPasswordLength {
			t.Errorf("%q: %v", short, err)
		}
	}
}

func TestTOTP(t *testing.T) {
	setup, err := GenerateTOTP("Acme", "anna")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(setup.URL, "otpauth://totp/Acme:anna?") || !strings.HasPrefix(setup.QR, "data:image/png;base64,") || len(setup.Secret) < 32 {
		t.Errorf("%+v", setup)
	}
	now := time.Unix(1_800_000_000, 0)
	code, err := totp.GenerateCode(setup.Secret, now)
	if err != nil {
		t.Fatal(err)
	}
	step, ok := CheckTOTP(setup.Secret, code, now, 0)
	if !ok || step != now.Unix()/30 {
		t.Fatalf("the code of now: %d %v", step, ok)
	}
	if _, ok := CheckTOTP(setup.Secret, code, now, step); ok {
		t.Error("a code used once works again")
	}
	if _, ok := CheckTOTP(setup.Secret, code, now.Add(25*time.Second), 0); !ok {
		t.Error("a code from the step before")
	}
	if _, ok := CheckTOTP(setup.Secret, code, now.Add(90*time.Second), 0); ok {
		t.Error("a code from three steps before")
	}
	if _, ok := CheckTOTP(setup.Secret, " "+code[:3]+" "+code[3:], now, 0); !ok {
		t.Error("spaces in the code")
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef"} {
		if _, ok := CheckTOTP(setup.Secret, bad, now, 0); ok {
			t.Errorf("%q passes", bad)
		}
	}
}

func TestThrottle(t *testing.T) {
	clock := time.Unix(1_800_000_000, 0)
	th := NewThrottle()
	th.now = func() time.Time { return clock }
	for i := 0; i < freeFailures; i++ {
		if th.Wait("acme/anna") != 0 {
			t.Fatalf("waits after %d failures", i)
		}
		th.Failed("acme/anna")
	}
	if th.Wait("acme/anna") != 0 {
		t.Error("the free failures")
	}
	th.Failed("acme/anna")
	if got := th.Wait("acme/anna"); got != firstWait {
		t.Errorf("first wait %v", got)
	}
	if th.Wait("main/anna") != 0 || th.Wait("acme/bob") != 0 {
		t.Error("other keys wait too")
	}
	th.Failed("acme/anna")
	if got := th.Wait("acme/anna"); got != 2*firstWait {
		t.Errorf("second wait %v", got)
	}
	for range 40 {
		th.Failed("acme/anna")
	}
	if got := th.Wait("acme/anna"); got != longestWait {
		t.Errorf("longest wait %v", got)
	}
	clock = clock.Add(longestWait)
	if th.Wait("acme/anna") != 0 {
		t.Error("the wait ends")
	}
	th.Succeeded("acme/anna")
	th.Failed("acme/anna")
	if th.Wait("acme/anna") != 0 {
		t.Error("a success starts over")
	}
}

func TestTokens(t *testing.T) {
	token, hash, err := NewToken()
	if err != nil || len(token) != 43 || !bytes.Equal(hash, HashToken(token)) || len(hash) != 32 {
		t.Fatalf("%q %x %v", token, hash, err)
	}
	other, _, _ := NewToken()
	if other == token {
		t.Error("the same token twice")
	}
}
