// SPDX-License-Identifier: MIT

package samba

import (
	"encoding/hex"
	"testing"
)

// The NT hash is MD4 over UTF-16LE, as Samba keeps it: the known value of "password", and one with a character beyond
// Latin-1.
func TestNTHash(t *testing.T) {
	if got := NTHash("password"); got != "8846F7EAEE8FB117AD06BDD830B7586C" {
		t.Errorf("NTHash(password) = %s", got)
	}
	if NTHash("Grüße") == NTHash("Grusse") {
		t.Error("umlauts are lost")
	}
}

// MD4 as RFC 1320 gives it.
func TestMD4(t *testing.T) {
	for in, want := range map[string]string{"": "31d6cfe0d16ae931b73c59d7e0c089c0", "abc": "a448017aaf21d8525fc10ae87aa6729d",
		"12345678901234567890123456789012345678901234567890123456789012345678901234567890": "e33b4ddc9c38f2199c3e7b164fcc0536"} {
		sum := md4([]byte(in))
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("md4(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestNames(t *testing.T) {
	for _, ok := range []string{"anna", "a.b-c_d", "x1"} {
		if !ValidName(ok) {
			t.Errorf("%q refused", ok)
		}
	}
	for _, bad := range []string{"", "Anna", "1anna", "root", "nobody", "homes", "a b", "a/b", "abcdefghijklmnopqrstuvwxyz0123456"} {
		if ValidName(bad) {
			t.Errorf("%q taken", bad)
		}
	}
	for title, want := range map[string]string{"Company": "Company", "Verträge & Angebote": "Vertraege _ Angebote", "": "Files",
		"..": "Files", "Homes": "Files Homes", "a/b\\c": "a_b_c"} {
		if got := ShareName(title); got != want || !ValidShareName(got) {
			t.Errorf("ShareName(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestBelow(t *testing.T) {
	root := "/srv/share/main"
	for p, want := range map[string]bool{root: true, root + "/a/b": true, "/srv/share/mainx": false, "/srv/share/acme/a": false,
		root + "/../acme": false, "relative": false, root + "/a/": false} {
		if Below(root, p) != want {
			t.Errorf("Below(%q) = %v", p, !want)
		}
	}
}

// The helper turns away what the channel does not allow, before it does anything.
func TestRequestCheck(t *testing.T) {
	root := "/srv/share/main"
	hash := NTHash("x")
	good := Request{Op: OpApply, State: &State{Users: []User{{Name: "anna", UID: 20000, NTHash: hash, Home: root + "/abc"}},
		Shares: []Share{{Name: "Company", Path: root + "/c", Users: []string{"anna"}}}}}
	if err := good.Check(root, 20000, 29999); err != nil {
		t.Fatal(err)
	}
	room := Request{Op: OpRoom, Room: []Room{{Path: root + "/c", Free: 10, Total: 100}, {Path: root + "/abc", Total: 5}}}
	if err := room.Check(root, 20000, 29999); err != nil {
		t.Fatalf("the room: %v", err)
	}
	bad := []Request{
		{Op: "exec"},
		{Op: OpApply},
		{Op: OpApply, State: &State{Users: []User{{Name: "root", UID: 20000, NTHash: hash}}}},
		{Op: OpApply, State: &State{Users: []User{{Name: "anna", UID: 0, NTHash: hash}}}},
		{Op: OpApply, State: &State{Users: []User{{Name: "anna", UID: 20000, NTHash: "plain"}}}},
		{Op: OpApply, State: &State{Users: []User{{Name: "anna", UID: 20000, NTHash: hash, Home: "/etc"}}}},
		{Op: OpApply, State: &State{Shares: []Share{{Name: "Company", Path: "/srv/share/acme/c"}}}},
		{Op: OpApply, State: &State{Shares: []Share{{Name: "global", Path: root + "/c"}}}},
		{Op: OpApply, State: &State{Shares: []Share{{Name: "Company", Path: root + "/c", Users: []string{"ghost"}}}}},
		{Op: OpApply, State: &State{Users: []User{{Name: "anna", UID: 20000, NTHash: hash}, {Name: "bob", UID: 20000, NTHash: hash}}}},
		{Op: OpACL, ACL: []PathACL{{Path: "/srv/share/acme"}}},
		{Op: OpACL, ACL: []PathACL{{Path: root, Access: map[int]Access{0: Write}}}},
		{Op: OpACL, ACL: []PathACL{{Path: root, Access: map[int]Access{20000: "all"}}}},
		{Op: OpOpen, Paths: []string{"/etc/passwd"}},
		{Op: OpApply, State: &State{Shares: []Share{{Name: "Company", Path: root + "/c\n[evil]\nguest ok = yes"}}}},
		{Op: OpApply, State: &State{Shares: []Share{{Name: "Company", Path: root + "/c", Comment: "x\n[evil]"}}}},
		// A folder named "%U" would be the folder of each user's name.
		{Op: OpApply, State: &State{Shares: []Share{{Name: "Company", Path: root + "/c/%U"}}}},
		// The room: only libraries below the tenant's directory, each once, free within total.
		{Op: OpRoom, Room: []Room{{Path: "/etc", Total: 1}}},
		{Op: OpRoom, Room: []Room{{Path: root, Total: 1}}},
		{Op: OpRoom, Room: []Room{{Path: root + "/c", Total: 1}, {Path: root + "/c", Total: 2}}},
		{Op: OpRoom, Room: []Room{{Path: root + "/c", Free: 2, Total: 1}}},
		{Op: OpRoom, Room: []Room{{Path: root + "/c", Free: -1, Total: 1}}},
	}
	for i, r := range bad {
		if err := r.Check(root, 20000, 29999); err == nil {
			t.Errorf("request %d passed: %+v", i, r)
		}
	}
}

// An ACL names UnderDoc, each UID with its access, and passage only on directories; a parse gives the same back.
func TestACL(t *testing.T) {
	access := map[int]Access{20001: Read, 20002: Write, 20003: Pass}
	dir := ParseACL(ACL(access, true, 1000))
	if dir[1000] != 7 || dir[20001] != 5 || dir[20002] != 7 || dir[20003] != 1 {
		t.Errorf("directory: %v", dir)
	}
	file := ParseACL(ACL(access, false, 1000))
	if file[1000] != 6 || file[20001] != 4 || file[20002] != 6 {
		t.Errorf("file: %v", file)
	}
	if _, ok := file[20003]; ok {
		t.Error("passage gives something on a file")
	}
}

// A user prints without its hash.
func TestUserHidesItsHash(t *testing.T) {
	u := User{Name: "anna", UID: 20000, NTHash: NTHash("secret")}
	for _, s := range []string{u.String(), u.GoString()} {
		if len(s) == 0 || containsHash(s, u.NTHash) {
			t.Errorf("%q shows the hash", s)
		}
	}
}

func containsHash(s, hash string) bool {
	for i := 0; i+len(hash) <= len(s); i++ {
		if s[i:i+len(hash)] == hash {
			return true
		}
	}
	return false
}

// A share hides names only from users who reach it, and only names a veto list can hold (T-43).
func TestVetoIsChecked(t *testing.T) {
	user := User{Name: "bob", UID: 20001, NTHash: NTHash("x")}
	state := func(veto map[string][]string) Request {
		return Request{Op: OpApply, State: &State{Users: []User{user}, Shares: []Share{{Name: "Company", Path: "/srv/share/main/c", Users: []string{"bob"}, Veto: veto}}}}
	}
	if err := state(map[string][]string{"bob": {"HR"}}).Check("/srv/share/main", 20000, 29999); err != nil {
		t.Errorf("a plain name: %v", err)
	}
	for name, veto := range map[string]map[string][]string{
		"a user who does not reach the share": {"carl": {"HR"}},
		"a wildcard":                          {"bob": {"H*"}},
		"a variable":                          {"bob": {"%U"}},
		"a separator":                         {"bob": {"a/b"}},
		"a line break":                        {"bob": {"HR\nx"}},
	} {
		if err := state(veto).Check("/srv/share/main", 20000, 29999); err == nil {
			t.Errorf("%s passes", name)
		}
	}
}

// With a domain (T-42) a user is a domain account without a hash, whose name may be a number; a hash, a name of the
// system and an unknown mode are refused, and with app passwords a user without a hash is refused: never mixed.
func TestDomainStateIsChecked(t *testing.T) {
	root := "/srv/share/main"
	hash := NTHash("pw")
	check := func(mode string, users ...User) error {
		return Request{Op: OpApply, State: &State{Mode: mode, Users: users}}.Check(root, 20000, 29999)
	}
	if err := check(ModeDomain, User{Name: "dora", UID: 20001}, User{Name: "4711", UID: 20002}); err != nil {
		t.Fatalf("domain accounts: %v", err)
	}
	for _, c := range []struct {
		name  string
		mode  string
		user  User
		wants error
	}{
		{"a hash with a domain", ModeDomain, User{Name: "dora", UID: 20001, NTHash: hash}, ErrMixed},
		{"no hash with app passwords", ModeLocal, User{Name: "dora", UID: 20001}, ErrMixed},
		{"no hash as before T-42", "", User{Name: "dora", UID: 20001}, ErrMixed},
		{"a number with app passwords", ModeLocal, User{Name: "4711", UID: 20001, NTHash: hash}, ErrName},
		{"root", ModeDomain, User{Name: "root", UID: 20001}, ErrDomainName},
		{"administrator", ModeDomain, User{Name: "administrator", UID: 20001}, ErrDomainName},
		{"daemon", ModeDomain, User{Name: "daemon", UID: 20001}, ErrDomainName},
		{"upper case", ModeDomain, User{Name: "Dora", UID: 20001}, ErrDomainName},
		{"with the domain", ModeDomain, User{Name: `example\dora`, UID: 20001}, ErrDomainName},
		{"a UID of the system", ModeDomain, User{Name: "dora", UID: 0}, ErrUID},
	} {
		if err := check(c.mode, c.user); err != c.wants {
			t.Errorf("%s: %v, want %v", c.name, err, c.wants)
		}
	}
	if err := check("kerberos", User{Name: "dora", UID: 20001}); err == nil {
		t.Error("an unknown mode")
	}
}
