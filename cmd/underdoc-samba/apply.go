// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Ollornog/UnderDoc/internal/samba"
)

// The files the helper writes, all below its run directory. The image links /etc/passwd and /etc/group there, so the
// users of the companion are exactly the base users and the Samba users of the last state.
func (h *helper) confPath() string { return filepath.Join(h.cfg.run, "smb.conf") }

// dfreeName is the name under which smbd runs this program for dfree command (room.go). smbd runs the setting as one
// path with the directory as its only argument, never splitting it into words, so the setting cannot be this program
// with an argument; a link in the directory of the helper names it instead.
const dfreeName = "dfree"

// linkDfree puts that link in place, to this program.
func (h *helper) linkDfree() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	link := filepath.Join(h.cfg.run, dfreeName)
	_ = os.Remove(link)
	return os.Symlink(exe, link)
}

// writeBase writes the configuration of smbd that never changes with the state; the shares come from an included file.
func (h *helper) writeBase() error {
	verbose := "create_file renameat unlinkat mkdirat linkat pwrite_send"
	if h.cfg.verbose {
		verbose = "all"
	}
	conf := fmt.Sprintf(`# Written by underdoc-samba (ADR-30) at every start; the shares follow in shares.conf.
[global]
	netbios name = %[1]s
	server string = %[1]s
	server role = standalone server
	workgroup = WORKGROUP
	security = user
	passdb backend = smbpasswd:%[2]s/smbpasswd
	map to guest = never
	restrict anonymous = 2
	server min protocol = SMB2_02
	ntlm auth = ntlmv2-only
	server signing = auto
	smb encrypt = if_required
	smb3 unix extensions = no
	ea support = no
	store dos attributes = no
	map archive = no
	map hidden = no
	map system = no
	map readonly = no
	nt acl support = no
	unix extensions = no
	wide links = no
	follow symlinks = no
	disable netbios = yes
	smb ports = 445
	load printers = no
	printing = bsd
	printcap name = /dev/null
	disable spoolss = yes
	access based share enum = yes
	hide unreadable = yes
	hide unwriteable files = no
	hide dot files = no
	dfree command = %[4]s
	dfree cache time = 5
	veto files = /.~underdoc-*/
	delete veto files = no
	create mask = 0660
	directory mask = 0770
	inherit acls = yes
	inherit permissions = no
	private dir = %[2]s/private
	lock directory = %[2]s/lock
	state directory = %[2]s/state
	cache directory = %[2]s/cache
	pid directory = %[2]s/pid
	ncalrpc dir = %[2]s/ncalrpc
	log level = 1
	vfs objects = full_audit
	full_audit:prefix = %%u
	full_audit:success = %[3]s
	full_audit:failure = none
	full_audit:facility = LOCAL5
	full_audit:priority = NOTICE
	full_audit:syslog = yes

[homes]
	comment = My files
	browseable = no
	read only = no
	valid users = %%S

include = %[2]s/shares.conf
`, h.cfg.name, h.cfg.run, verbose, filepath.Join(h.cfg.run, dfreeName))
	if err := h.linkDfree(); err != nil {
		return err
	}
	for _, d := range []string{"private", "lock", "state", "cache", "pid", "ncalrpc"} {
		if err := os.MkdirAll(filepath.Join(h.cfg.run, d), 0o700); err != nil {
			return err
		}
	}
	return writeFile(h.confPath(), []byte(conf), 0o644)
}

// writeFile writes a file through a temporary one and a rename, so smbd and the system never read half of it.
func writeFile(name string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(name), ".tmp-")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), name)
}

// base reads the users and groups the image brings, from the copies the image keeps (/etc/passwd.base, /etc/group.base).
func base(name string) []string {
	raw, err := os.ReadFile(name) // #nosec G304 -- fixed paths of the image
	if err != nil {
		return nil
	}
	var lines []string
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// writeState writes users, passwords and shares of a state. It changes nothing else; apply reloads smbd afterwards.
func (h *helper) writeState(s samba.State) error {
	users := append([]samba.User(nil), s.Users...)
	sort.Slice(users, func(i, j int) bool { return users[i].Name < users[j].Name })
	passwd := base("/etc/passwd.base")
	group := base("/etc/group.base")
	passwd = append(passwd, fmt.Sprintf("underdoc:x:%d:%d:UnderDoc:/nonexistent:/usr/sbin/nologin", h.cfg.owner, h.cfg.ownerGID))
	group = append(group, fmt.Sprintf("underdoc:x:%d:", h.cfg.ownerGID))
	var smbpasswd bytes.Buffer
	lct := fmt.Sprintf("LCT-%08X", time.Now().Unix())
	for _, u := range users {
		home := u.Home
		if home == "" {
			home = "/nonexistent"
		}
		passwd = append(passwd, fmt.Sprintf("%s:x:%d:%d:Samba user:%s:/usr/sbin/nologin", u.Name, u.UID, u.UID, home))
		group = append(group, fmt.Sprintf("%s:x:%d:", u.Name, u.UID))
		fmt.Fprintf(&smbpasswd, "%s:%d:XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX:%s:[U          ]:%s:\n", u.Name, u.UID, u.NTHash, lct)
	}
	if err := writeFile(filepath.Join(h.cfg.run, "passwd"), []byte(strings.Join(passwd, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(h.cfg.run, "group"), []byte(strings.Join(group, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(h.cfg.run, "smbpasswd"), smbpasswd.Bytes(), 0o600); err != nil {
		return err
	}
	var shares strings.Builder
	shares.WriteString("# Written by underdoc-samba from the state UnderDoc applied (ADR-30).\n")
	list := append([]samba.Share(nil), s.Shares...)
	sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })
	// The folders a user only passes through are hidden from that user in the share above them (T-43): each user of a
	// share reads a file of their own, named after them (%U), with the veto list of the share.
	// A new directory each time, and the one before kept, so smbd never reads an include that is gone (a missing one
	// would hide nothing).
	veto := filepath.Join(h.cfg.run, "veto."+strconv.FormatInt(time.Now().UnixNano(), 10))
	for i, sh := range list {
		if len(sh.Users) == 0 {
			continue
		}
		fmt.Fprintf(&shares, "\n[%s]\n\tpath = %s\n\tread only = no\n\tbrowseable = yes\n\tvalid users = %s\n",
			sh.Name, sh.Path, strings.Join(sh.Users, " "))
		// Until the file of the user is read, the share hides every folder it hides from anyone: a user whose file
		// smbd does not find sees less, never more.
		var all []string
		for _, names := range sh.Veto {
			all = append(all, names...)
		}
		sort.Strings(all)
		fmt.Fprintf(&shares, "\t%s", vetoLine(slices.Compact(all)))
		dir := filepath.Join(veto, strconv.Itoa(i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		for _, u := range sh.Users {
			if err := os.WriteFile(filepath.Join(dir, u+".conf"), []byte(vetoLine(sh.Veto[u])), 0o644); err != nil {
				return err
			}
		}
		// %U is the account the session signed in as (smbd sets it after the sign-in; %u is not known yet when the
		// include is read).
		fmt.Fprintf(&shares, "\tinclude = %s\n", filepath.Join(veto, strconv.Itoa(i), "%U.conf"))
		if sh.Comment != "" {
			fmt.Fprintf(&shares, "\tcomment = %s\n", strings.Map(func(r rune) rune {
				if r < ' ' || r == '\\' || r == '%' {
					return ' '
				}
				return r
			}, sh.Comment))
		}
	}
	if err := writeFile(filepath.Join(h.cfg.run, "shares.conf"), []byte(shares.String()), 0o644); err != nil {
		return err
	}
	old, err := filepath.Glob(filepath.Join(h.cfg.run, "veto.*"))
	if err != nil {
		return err
	}
	sort.Strings(old)
	for len(old) > 2 {
		if err := os.RemoveAll(old[0]); err != nil {
			return err
		}
		old = old[1:]
	}
	return nil
}

// baseVeto is the veto list of every share: the temporary files of a save (global section of smb.conf). A share that
// sets its own list replaces it, so each list starts with it.
const baseVeto = "/.~underdoc-*/"

// vetoLine is the line of a user's file for a share: the base list and the folders hidden from them there (T-43).
func vetoLine(names []string) string {
	var b strings.Builder
	b.WriteString("veto files = " + baseVeto)
	for _, n := range names {
		if samba.ValidVeto(n) {
			b.WriteString(n + "/")
		}
	}
	return b.String() + "\n"
}

// apply makes the companion hold the state: it writes users, passwords and shares, closes the sessions of users that are
// gone or whose password changed, and has smbd read its configuration again.
func (h *helper) apply(s samba.State) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	before := map[string]samba.User{}
	for _, u := range h.state.Users {
		before[u.Name] = u
	}
	after := map[string]bool{}
	var end []string
	for _, u := range s.Users {
		after[u.Name] = true
		if old, ok := before[u.Name]; ok && (old.NTHash != u.NTHash || old.UID != u.UID) {
			end = append(end, u.Name)
		}
	}
	for name := range before {
		if !after[name] {
			end = append(end, name)
		}
	}
	if err := h.writeState(s); err != nil {
		return err
	}
	h.state = s
	if h.smbd != nil {
		_ = h.smbd.Signal(syscall.SIGHUP)
	}
	if len(end) > 0 {
		h.endSessions(end)
	}
	return nil
}

// endSessions ends the processes of smbd that serve the named users, so a removed user or an old password stops at once.
func (h *helper) endSessions(users []string) {
	out, err := exec.Command("smbstatus", "--processes", "--json", "--configfile="+h.confPath()).Output() // #nosec G204
	if err != nil {
		h.log.Warn("smbstatus", "err", err)
		return
	}
	for _, pid := range sessionPIDs(out, users) {
		if h.smbd != nil && pid == h.smbd.Pid {
			continue
		}
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
}
