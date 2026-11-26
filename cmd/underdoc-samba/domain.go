// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// A companion that is a member of an Active Directory domain (T-42, docs/samba.md): people sign in with their domain
// account, and the companion maps each domain account to the UID of its UnderDoc account by name (idmap_nss), from the
// users UnderDoc applies. A domain account without such a user, or whose name is a user of the system outside the range
// of Samba users (root, daemon, administrator, …), gets no UID and is turned away. The mode is chosen at setup, never
// mixed with app passwords: a state that brings an NT hash is refused (ErrMixed).

// domainConfig is the setup of the domain, from the environment.
type domainConfig struct {
	realm     string // the Kerberos realm, EXAMPLE.TEST
	workgroup string // the short name of the domain, EXAMPLE
	joinUser  string // the account that may join computers to the domain; needed only while not joined
	joinFile  string // the file that holds its password; the password never stands in the environment or a command line
	state     string // a volume: the machine account's secret (secrets.tdb) outlives the container
}

var (
	realmPattern    = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$`)
	workPattern     = regexp.MustCompile(`^[A-Za-z0-9-]{1,15}$`)
	joinUserPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
)

// readDomainConfig reads the domain from the environment; nil without UNDERDOC_SAMBA_DOMAIN_REALM.
func readDomainConfig() (*domainConfig, error) {
	// A password in the environment would stand in /proc/<pid>/environ of every process and in docker inspect.
	for _, name := range []string{"UNDERDOC_SAMBA_DOMAIN_JOIN_PASSWORD", "PASSWD", "PASSWD_FD"} {
		if _, set := os.LookupEnv(name); set {
			return nil, fmt.Errorf("%s is set: the password for the join comes only from a file (UNDERDOC_SAMBA_DOMAIN_JOIN_PASSWORD_FILE)", name)
		}
	}
	realm := os.Getenv("UNDERDOC_SAMBA_DOMAIN_REALM")
	if realm == "" {
		for _, name := range []string{"UNDERDOC_SAMBA_DOMAIN_WORKGROUP", "UNDERDOC_SAMBA_DOMAIN_JOIN_USER", "UNDERDOC_SAMBA_DOMAIN_JOIN_PASSWORD_FILE"} {
			if os.Getenv(name) != "" {
				return nil, fmt.Errorf("%s without UNDERDOC_SAMBA_DOMAIN_REALM: a domain needs its realm", name)
			}
		}
		return nil, nil
	}
	d := &domainConfig{
		realm:     strings.ToUpper(realm),
		workgroup: strings.ToUpper(os.Getenv("UNDERDOC_SAMBA_DOMAIN_WORKGROUP")),
		joinUser:  os.Getenv("UNDERDOC_SAMBA_DOMAIN_JOIN_USER"),
		joinFile:  os.Getenv("UNDERDOC_SAMBA_DOMAIN_JOIN_PASSWORD_FILE"),
		state:     env("UNDERDOC_SAMBA_DOMAIN_STATE", "/var/lib/underdoc-samba-domain"),
	}
	switch {
	case !realmPattern.MatchString(d.realm) || len(d.realm) > 253:
		return nil, fmt.Errorf("UNDERDOC_SAMBA_DOMAIN_REALM: the realm of the domain, such as EXAMPLE.TEST")
	case !workPattern.MatchString(d.workgroup):
		return nil, fmt.Errorf("UNDERDOC_SAMBA_DOMAIN_WORKGROUP: the short name of the domain, 1 to 15 letters, digits or '-'")
	case d.joinUser != "" && !joinUserPattern.MatchString(d.joinUser):
		// No '%' or '\': net would read user%password or DOMAIN\user out of it.
		return nil, fmt.Errorf("UNDERDOC_SAMBA_DOMAIN_JOIN_USER: an account name of letters, digits, '.', '_' or '-'")
	case d.joinFile != "" && !filepath.IsAbs(d.joinFile):
		return nil, fmt.Errorf("UNDERDOC_SAMBA_DOMAIN_JOIN_PASSWORD_FILE: an absolute path")
	case (d.joinUser == "") != (d.joinFile == ""):
		return nil, fmt.Errorf("UNDERDOC_SAMBA_DOMAIN_JOIN_USER and UNDERDOC_SAMBA_DOMAIN_JOIN_PASSWORD_FILE go together")
	case !filepath.IsAbs(d.state) || filepath.Clean(d.state) != d.state || d.state == "/":
		return nil, fmt.Errorf("UNDERDOC_SAMBA_DOMAIN_STATE: an absolute directory, a volume of its own")
	}
	return d, nil
}

// privateDir is where Samba keeps the secret of the machine account: on the volume, so a new container is still joined.
func (d *domainConfig) privateDir() string { return filepath.Join(d.state, "private") }

// domainGlobal is the part of [global] that makes the companion a member of the domain; it replaces the part of a
// standalone server.
func (h *helper) domainGlobal() string {
	d := h.cfg.domain
	return fmt.Sprintf(`	server role = member server
	security = ADS
	realm = %[1]s
	workgroup = %[2]s
	kerberos method = secrets only
	idmap config * : backend = tdb
	idmap config * : range = 3000000-3999999
	idmap config %[2]s : backend = nss
	idmap config %[2]s : range = %[3]d-%[4]d
	winbind use default domain = yes
	winbind enum users = no
	winbind enum groups = no
	winbind offline logon = no
	winbind nss info = template
	template shell = /usr/sbin/nologin
	template homedir = /nonexistent
`, d.realm, d.workgroup, h.cfg.uidMin, h.cfg.uidMax)
}

// krb5Conf is the Kerberos configuration of the companion: the realm, and its domain controllers found through DNS.
func (h *helper) krb5Conf() string {
	return fmt.Sprintf("[libdefaults]\n\tdefault_realm = %s\n\tdns_lookup_realm = false\n\tdns_lookup_kdc = true\n", h.cfg.domain.realm)
}

// net runs net of Samba with the configuration of the companion. Its output goes to the log with the password taken out,
// should it ever stand there.
func (h *helper) net(ctx context.Context, env []string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "net", append(args, "--configfile="+h.confPath())...) // #nosec G204 -- fixed words and checked names
	cmd.Env = append(os.Environ(), env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	text := strings.TrimSpace(out.String())
	if secret := h.joinPassword(); secret != "" {
		text = strings.ReplaceAll(text, secret, "[password]")
	}
	if text != "" {
		h.log.Info("net "+args[0]+" "+args[1], "output", text)
	}
	return err
}

// joinPassword reads the password of the join, only to keep it out of the log; "" without one.
func (h *helper) joinPassword() string {
	if h.cfg.domain.joinFile == "" {
		return ""
	}
	raw, err := os.ReadFile(h.cfg.domain.joinFile)
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(raw), "\r\n")
}

// joinDomain makes the companion a member: still joined from before (the secret on the volume), or joined now with the
// account and the password file of the setup. Either way the join is checked (net ads testjoin) before anything goes on.
func (h *helper) joinDomain(ctx context.Context) error {
	d := h.cfg.domain
	if err := os.MkdirAll(d.privateDir(), 0o700); err != nil {
		return fmt.Errorf("the state of the domain (%s): %w", d.state, err)
	}
	if err := h.net(ctx, nil, "ads", "testjoin"); err == nil {
		h.log.Info("member of the domain", "realm", d.realm, "joined", "before")
		return nil
	}
	if d.joinFile == "" {
		return errors.New("not a member of the domain " + d.realm + ", and no UNDERDOC_SAMBA_DOMAIN_JOIN_USER and _PASSWORD_FILE to join it")
	}
	info, err := os.Stat(d.joinFile)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("UNDERDOC_SAMBA_DOMAIN_JOIN_PASSWORD_FILE: a file that holds the password (%s)", d.joinFile)
	}
	// net reads the password from the file it names in PASSWD_FILE: neither the command line nor the environment carries
	// it. --no-dns-updates: the address of the container is not the address people reach, so the companion leaves the DNS
	// of the domain alone.
	if err := h.net(ctx, []string{"PASSWD_FILE=" + d.joinFile}, "ads", "join", "-U", d.joinUser, "--no-dns-updates"); err != nil {
		return fmt.Errorf("joining the domain %s failed: %w (see the log above)", d.realm, err)
	}
	if err := h.net(ctx, nil, "ads", "testjoin"); err != nil {
		return fmt.Errorf("joined the domain %s, but net ads testjoin fails: %w", d.realm, err)
	}
	h.log.Info("member of the domain", "realm", d.realm, "joined", "now")
	return nil
}

// startWinbindd starts winbindd in the foreground, as a child the helper watches, and waits until it answers.
func (h *helper) startWinbindd() error {
	cmd := exec.Command("winbindd", "--foreground", "--no-process-group", "--debug-stdout", "--configfile="+h.confPath()) // #nosec G204
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	h.mu.Lock()
	h.winbindd = cmd.Process
	h.mu.Unlock()
	go func() { _ = cmd.Wait() }()
	for range 60 {
		if exec.Command("wbinfo", "--ping-dc").Run() == nil { // #nosec G204
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return errors.New("winbindd does not reach a domain controller")
}

// flushCaches makes winbindd forget what it mapped: the idmap cache in gencache (net cache flush) and its own cache of
// the domain's accounts (smbcontrol winbindd reload-config drops what it keeps in memory).
func (h *helper) flushCaches() error {
	if out, err := exec.Command("net", "cache", "flush", "--configfile="+h.confPath()).CombinedOutput(); err != nil { // #nosec G204
		return fmt.Errorf("net cache flush: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("smbcontrol", "--configfile="+h.confPath(), "winbindd", "reload-config").CombinedOutput(); err != nil { // #nosec G204
		return fmt.Errorf("smbcontrol winbindd reload-config: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
