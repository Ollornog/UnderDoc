// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Ollornog/UnderDoc/internal/samba"
)

// clearDomainEnv empties the settings of the domain for one test.
func clearDomainEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"UNDERDOC_SAMBA_DOMAIN_REALM", "UNDERDOC_SAMBA_DOMAIN_WORKGROUP", "UNDERDOC_SAMBA_DOMAIN_JOIN_USER",
		"UNDERDOC_SAMBA_DOMAIN_JOIN_PASSWORD_FILE", "UNDERDOC_SAMBA_DOMAIN_STATE"} {
		t.Setenv(name, "")
	}
}

// The setup of a domain (T-42): realm and short name, the account of the join with its password from a file only. A
// password in the environment, a name net would read a password or a domain out of, and half a setup are refused.
func TestDomainConfig(t *testing.T) {
	clearDomainEnv(t)
	if d, err := readDomainConfig(); d != nil || err != nil {
		t.Fatalf("without a realm: %+v %v", d, err)
	}
	t.Setenv("UNDERDOC_SAMBA_DOMAIN_REALM", "example.test")
	t.Setenv("UNDERDOC_SAMBA_DOMAIN_WORKGROUP", "example")
	t.Setenv("UNDERDOC_SAMBA_DOMAIN_JOIN_USER", "joiner")
	t.Setenv("UNDERDOC_SAMBA_DOMAIN_JOIN_PASSWORD_FILE", "/run/secrets/join")
	d, err := readDomainConfig()
	if err != nil || d.realm != "EXAMPLE.TEST" || d.workgroup != "EXAMPLE" || d.joinUser != "joiner" || d.state != "/var/lib/underdoc-samba-domain" {
		t.Fatalf("a domain: %+v %v", d, err)
	}
	for _, c := range []struct{ name, value, want string }{
		{"UNDERDOC_SAMBA_DOMAIN_JOIN_PASSWORD", "secret", "only from a file"},
		{"PASSWD", "secret", "only from a file"},
		{"UNDERDOC_SAMBA_DOMAIN_JOIN_USER", "joiner%secret", "UNDERDOC_SAMBA_DOMAIN_JOIN_USER"},
		{"UNDERDOC_SAMBA_DOMAIN_JOIN_USER", `EXAMPLE\joiner`, "UNDERDOC_SAMBA_DOMAIN_JOIN_USER"},
		{"UNDERDOC_SAMBA_DOMAIN_JOIN_USER", "", "go together"},
		{"UNDERDOC_SAMBA_DOMAIN_JOIN_PASSWORD_FILE", "join.txt", "an absolute path"},
		{"UNDERDOC_SAMBA_DOMAIN_REALM", "example", "UNDERDOC_SAMBA_DOMAIN_REALM"},
		{"UNDERDOC_SAMBA_DOMAIN_REALM", "example.test\n[global]", "UNDERDOC_SAMBA_DOMAIN_REALM"},
		{"UNDERDOC_SAMBA_DOMAIN_WORKGROUP", "", "UNDERDOC_SAMBA_DOMAIN_WORKGROUP"},
		{"UNDERDOC_SAMBA_DOMAIN_STATE", "/", "UNDERDOC_SAMBA_DOMAIN_STATE"},
	} {
		t.Run(c.name+"="+c.value, func(t *testing.T) {
			t.Setenv(c.name, c.value)
			if _, err := readDomainConfig(); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("%v, want %q", err, c.want)
			}
		})
	}
	// Without a realm, a part of the setup alone is a mistake, not nothing.
	t.Setenv("UNDERDOC_SAMBA_DOMAIN_REALM", "")
	if _, err := readDomainConfig(); err == nil {
		t.Error("a setup of a domain without its realm")
	}
}

func domainHelper(t *testing.T) *helper {
	t.Helper()
	run := t.TempDir()
	return &helper{log: slog.New(slog.DiscardHandler), cfg: config{run: run, owner: 1000, ownerGID: 1000, uidMin: 20000, uidMax: 29999, name: "FILES",
		domain: &domainConfig{realm: "EXAMPLE.TEST", workgroup: "EXAMPLE", state: filepath.Join(run, "domain")}}}
}

// The configuration of a member: ADS, the domain's accounts mapped by name (idmap_nss) into the range of Samba users
// only, the secret of the machine account on the volume, and Kerberos from the file the helper writes.
func TestDomainConfiguration(t *testing.T) {
	t.Setenv("KRB5_CONFIG", "")
	h := domainHelper(t)
	if err := h.writeBase(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(h.confPath())
	if err != nil {
		t.Fatal(err)
	}
	conf := string(raw)
	for _, want := range []string{"\tsecurity = ADS\n", "\trealm = EXAMPLE.TEST\n", "\tworkgroup = EXAMPLE\n",
		"\tidmap config EXAMPLE : backend = nss\n", "\tidmap config EXAMPLE : range = 20000-29999\n",
		"\twinbind use default domain = yes\n", "\tprivate dir = " + filepath.Join(h.cfg.run, "domain", "private") + "\n",
		"\tntlm auth = ntlmv2-only\n", "\tmap to guest = never\n"} {
		if !strings.Contains(conf, want) {
			t.Errorf("missing %q in\n%s", want, conf)
		}
	}
	for _, not := range []string{"security = user", "standalone", "backend = tdb\n\tidmap config EXAMPLE : range = 3000000"} {
		if strings.Contains(conf, not) {
			t.Errorf("%q in\n%s", not, conf)
		}
	}
	if got := os.Getenv("KRB5_CONFIG"); got != filepath.Join(h.cfg.run, "krb5.conf") {
		t.Errorf("KRB5_CONFIG %q", got)
	}
	// Without a domain, as before.
	local := &helper{cfg: config{run: t.TempDir(), owner: 1000, ownerGID: 1000, name: "FILES"}}
	if err := local.writeBase(); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(local.confPath())
	if !strings.Contains(string(raw), "\tsecurity = user\n") || strings.Contains(string(raw), "idmap") {
		t.Errorf("without a domain:\n%s", raw)
	}
}

// A companion takes only a state of its own mode: a member of a domain refuses app passwords, a companion with app
// passwords refuses domain accounts, and nothing is written then (T-42).
func TestDomainStateOfTheOtherModeIsRefused(t *testing.T) {
	h := domainHelper(t)
	local := samba.State{Users: []samba.User{{Name: "dora", UID: 20001, NTHash: samba.NTHash("x")}}}
	if err := h.apply(local); !errors.Is(err, samba.ErrMixed) {
		t.Fatalf("app passwords on a member of a domain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(h.cfg.run, "passwd")); !os.IsNotExist(err) {
		t.Error("the refused state was written")
	}
	domain := samba.State{Mode: samba.ModeDomain, Users: []samba.User{{Name: "dora", UID: 20001}}}
	plain := &helper{cfg: config{run: t.TempDir(), owner: 1000, ownerGID: 1000}}
	if err := plain.apply(domain); !errors.Is(err, samba.ErrMixed) {
		t.Fatalf("domain accounts on a companion with app passwords: %v", err)
	}
	// net and smbcontrol stand in, and say what they were asked.
	bin, calls := t.TempDir(), filepath.Join(t.TempDir(), "calls")
	for _, name := range []string{"net", "smbcontrol"} {
		script := "#!/bin/sh\necho \"" + name + " $*\" >> " + calls + "\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil { // #nosec G306 -- a test program
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	if err := h.apply(domain); err != nil {
		t.Fatalf("domain accounts on a member: %v", err)
	}
	// The users changed: winbindd forgets what it mapped. The same state again changes nothing, and flushes nothing.
	asked, _ := os.ReadFile(calls)
	if !strings.Contains(string(asked), "net cache flush") || !strings.Contains(string(asked), "winbindd reload-config") {
		t.Fatalf("the caches stay: %q", asked)
	}
	if err := os.Remove(calls); err != nil {
		t.Fatal(err)
	}
	if err := h.apply(domain); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(calls); !os.IsNotExist(err) {
		t.Error("the same state flushed the caches")
	}
	// The UID passes to another account: flushed.
	if err := h.apply(samba.State{Mode: samba.ModeDomain, Users: []samba.User{{Name: "eve", UID: 20001}}}); err != nil {
		t.Fatal(err)
	}
	if asked, _ := os.ReadFile(calls); !strings.Contains(string(asked), "net cache flush") {
		t.Error("a UID that passed to another account left the caches")
	}
	passwd, _ := os.ReadFile(filepath.Join(h.cfg.run, "passwd"))
	smbpasswd, _ := os.ReadFile(filepath.Join(h.cfg.run, "smbpasswd"))
	if !strings.Contains(string(passwd), "\neve:x:20001:20001:") || len(smbpasswd) != 0 {
		t.Errorf("passwd %q, smbpasswd %q", passwd, smbpasswd)
	}
}

// What net says goes to the log without the password of the join, should it ever stand there (T-42).
func TestDomainPasswordStaysOutOfTheLog(t *testing.T) {
	bin := t.TempDir()
	secret := "Synthetic-" + strings.Repeat("x", 12)
	file := filepath.Join(t.TempDir(), "join")
	if err := os.WriteFile(file, []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A net that tells everything it got, the password included.
	script := "#!/bin/sh\necho \"args: $*\"\necho \"env: $PASSWD_FILE\"\ncat \"$PASSWD_FILE\"\n"
	if err := os.WriteFile(filepath.Join(bin, "net"), []byte(script), 0o755); err != nil { // #nosec G306 -- a test program
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	var out bytes.Buffer
	h := domainHelper(t)
	h.log = slog.New(slog.NewTextHandler(&out, nil))
	h.cfg.domain.joinUser, h.cfg.domain.joinFile = "joiner", file
	if err := h.net(context.Background(), []string{"PASSWD_FILE=" + file}, "ads", "join", "-U", "joiner"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), secret) || !strings.Contains(out.String(), "[password]") {
		t.Errorf("the log: %s", out.String())
	}
	if !strings.Contains(out.String(), "args: ads join -U joiner --configfile=") {
		t.Errorf("the command line: %s", out.String())
	}
}
