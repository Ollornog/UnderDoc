// SPDX-License-Identifier: MIT

// Command underdoc-samba is the helper of the Samba companion (ADR-30, docs/samba.md). It is the first process of its
// container: it starts and watches smbd, receives smbd's audit on a syslog socket of its own, and answers UnderDoc on a
// Unix socket, one JSON request per connection, from a closed list (package samba). It runs as root inside its container,
// because smbd switches to the UID of each user; UnderDoc never does.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/Ollornog/UnderDoc/internal/samba"
	"github.com/Ollornog/UnderDoc/internal/version"
)

// config is what the helper reads from its environment.
type config struct {
	root     string // the directory of the tenant, the same path as UnderDoc sees it
	socket   string // where UnderDoc reaches the helper
	owner    int    // the UID of UnderDoc: the only peer, and the owner of every path
	ownerGID int
	uidMin   int
	uidMax   int
	name     string // the NetBIOS name and the server string
	smbd     string
	run      string // where the helper keeps what it writes: users, passwords, shares
	audit    string // the syslog socket of smbd's audit
	verbose  bool   // audit every operation, for recordings (T-1)
}

func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func readConfig() (config, error) {
	c := config{
		root:    env("UNDERDOC_SAMBA_ROOT", ""),
		socket:  env("UNDERDOC_SAMBA_SOCKET", "/run/underdoc-samba/helper.sock"),
		name:    env("UNDERDOC_SAMBA_NAME", "UNDERDOC"),
		smbd:    env("UNDERDOC_SAMBA_SMBD", "/usr/sbin/smbd"),
		run:     env("UNDERDOC_SAMBA_RUN", "/run/underdoc-samba/state"),
		audit:   env("UNDERDOC_SAMBA_AUDIT_SOCKET", "/dev/log"),
		verbose: os.Getenv("UNDERDOC_SAMBA_AUDIT") == "all",
	}
	var err error
	if c.owner, err = strconv.Atoi(env("UNDERDOC_UID", "65532")); err != nil || c.owner < 1 {
		return c, fmt.Errorf("UNDERDOC_UID: a UID other than 0")
	}
	if c.ownerGID, err = strconv.Atoi(env("UNDERDOC_GID", strconv.Itoa(c.owner))); err != nil || c.ownerGID < 1 {
		return c, fmt.Errorf("UNDERDOC_GID: a GID other than 0")
	}
	lo, hi, ok := strings.Cut(env("UNDERDOC_SAMBA_UIDS", "20000-29999"), "-")
	if c.uidMin, err = strconv.Atoi(lo); err != nil || !ok {
		return c, fmt.Errorf("UNDERDOC_SAMBA_UIDS: a range like 20000-29999")
	}
	if c.uidMax, err = strconv.Atoi(hi); err != nil || c.uidMax < c.uidMin || c.uidMin < 1000 {
		return c, fmt.Errorf("UNDERDOC_SAMBA_UIDS: a range like 20000-29999, from 1000 on")
	}
	if c.owner >= c.uidMin && c.owner <= c.uidMax {
		return c, fmt.Errorf("UNDERDOC_UID lies in UNDERDOC_SAMBA_UIDS")
	}
	if c.root == "" || !filepath.IsAbs(c.root) || filepath.Clean(c.root) != c.root || c.root == "/" {
		return c, fmt.Errorf("UNDERDOC_SAMBA_ROOT: the absolute directory of the tenant")
	}
	if !regexpNetbios(c.name) {
		return c, fmt.Errorf("UNDERDOC_SAMBA_NAME: 1 to 15 letters, digits or '-'")
	}
	return c, nil
}

func regexpNetbios(s string) bool {
	if len(s) < 1 || len(s) > 15 {
		return false
	}
	for _, r := range s {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

// helper is the running helper: its configuration, smbd, the audit and the state last applied.
type helper struct {
	cfg   config
	log   *slog.Logger
	audit *ring

	boot  string     // names this start, so UnderDoc knows to apply its state again
	mu    sync.Mutex // one change at a time
	smbd  *os.Process
	state samba.State
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if len(os.Args) > 1 && os.Args[1] == "probe" {
		os.Exit(probe())
	}
	cfg, err := readConfig()
	if err != nil {
		log.Error("configuration", "err", err)
		os.Exit(2)
	}
	var b [8]byte
	_, _ = rand.Read(b[:])
	h := &helper{cfg: cfg, log: log, audit: newRing(4096), boot: hex.EncodeToString(b[:])}
	if err := h.run(); err != nil {
		log.Error("helper", "err", err)
		os.Exit(1)
	}
}

// probe is the health check of the container: the helper answers a ping and smbd is up.
func probe() int {
	c := samba.Client{Socket: env("UNDERDOC_SAMBA_SOCKET", "/run/underdoc-samba/helper.sock"), Timeout: 3 * time.Second}
	if _, err := c.Do(context.Background(), samba.Request{Op: samba.OpPing}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func (h *helper) run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	if err := os.MkdirAll(h.cfg.run, 0o700); err != nil {
		return err
	}
	if err := h.writeBase(); err != nil {
		return err
	}
	if err := h.writeState(samba.State{}); err != nil {
		return err
	}
	if err := h.listenAudit(ctx); err != nil {
		return err
	}
	if err := h.startSmbd(); err != nil {
		return err
	}
	go h.watchSmbd(ctx)
	ln, err := h.listen()
	if err != nil {
		return err
	}
	h.log.Info("ready", "socket", h.cfg.socket, "root", h.cfg.root, "version", version.Version)
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			if errors.Is(err, net.ErrClosed) {
				break
			}
			continue
		}
		go h.serve(conn.(*net.UnixConn))
	}
	h.mu.Lock()
	if h.smbd != nil {
		_ = h.smbd.Signal(syscall.SIGTERM)
	}
	h.mu.Unlock()
	return nil
}

// listen makes the socket for UnderDoc: mode 0600 and owned by UnderDoc's UID, in a directory only it can enter.
func (h *helper) listen() (*net.UnixListener, error) {
	dir := filepath.Dir(h.cfg.socket)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chown(dir, h.cfg.owner, h.cfg.ownerGID); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	_ = os.Remove(h.cfg.socket)
	old := unix.Umask(0o177)
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: h.cfg.socket, Net: "unix"})
	unix.Umask(old)
	if err != nil {
		return nil, err
	}
	if err := os.Chown(h.cfg.socket, h.cfg.owner, h.cfg.ownerGID); err != nil {
		_ = ln.Close()
		return nil, err
	}
	return ln, nil
}

// serve answers one request. A peer that is not UnderDoc's UID, or root for the probe, is turned away unanswered.
func (h *helper) serve(conn *net.UnixConn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Minute))
	raw, err := conn.SyscallConn()
	if err != nil {
		return
	}
	var cred *unix.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) { cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) }); err != nil || credErr != nil {
		return
	}
	var req samba.Request
	if err := json.NewDecoder(io.LimitReader(conn, 64<<20)).Decode(&req); err != nil {
		h.answer(conn, samba.Response{Error: "a request is one JSON object"})
		return
	}
	if int(cred.Uid) != h.cfg.owner && !(cred.Uid == 0 && req.Op == samba.OpPing) {
		h.log.Warn("a peer that is not UnderDoc", "uid", cred.Uid)
		return
	}
	if err := req.Check(h.cfg.root, h.cfg.uidMin, h.cfg.uidMax); err != nil {
		h.answer(conn, samba.Response{Error: err.Error()})
		return
	}
	resp, err := h.handle(req)
	if err != nil {
		h.log.Warn("request", "op", req.Op, "err", err)
		resp = samba.Response{Error: err.Error()}
	} else {
		resp.OK = true
	}
	h.answer(conn, resp)
}

func (h *helper) answer(conn net.Conn, r samba.Response) { _ = json.NewEncoder(conn).Encode(r) }

func (h *helper) handle(req samba.Request) (samba.Response, error) {
	switch req.Op {
	case samba.OpPing:
		h.mu.Lock()
		up := h.smbd != nil && h.smbd.Signal(syscall.Signal(0)) == nil
		h.mu.Unlock()
		if !up {
			return samba.Response{}, fmt.Errorf("smbd is not running")
		}
		return samba.Response{Version: version.Version, Samba: smbdVersion(h.cfg.smbd), Boot: h.boot}, nil
	case samba.OpApply:
		return samba.Response{}, h.apply(*req.State)
	case samba.OpACL:
		return samba.Response{}, h.applyACL(req.ACL)
	case samba.OpOpen:
		open, err := h.openFiles(req.Paths)
		return samba.Response{Open: open}, err
	case samba.OpWho:
		return samba.Response{Who: h.audit.find(req.Paths)}, nil
	}
	return samba.Response{}, fmt.Errorf("unknown request")
}

func smbdVersion(smbd string) string {
	out, err := exec.Command(smbd, "--version").Output() // #nosec G204 -- the path comes from the configuration
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// startSmbd starts smbd in the foreground, as a child the helper watches.
func (h *helper) startSmbd() error {
	cmd := exec.Command(h.cfg.smbd, "--foreground", "--no-process-group", "--debug-stdout", "--configfile="+h.confPath()) // #nosec G204
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	h.mu.Lock()
	h.smbd = cmd.Process
	h.mu.Unlock()
	go func() { _ = cmd.Wait() }()
	return nil
}

// watchSmbd starts smbd again when it ends, at most once every few seconds.
func (h *helper) watchSmbd(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
		h.mu.Lock()
		p := h.smbd
		h.mu.Unlock()
		if p != nil && p.Signal(syscall.Signal(0)) == nil {
			continue
		}
		h.log.Warn("smbd ended, starting it again")
		if err := h.startSmbd(); err != nil {
			h.log.Error("smbd", "err", err)
		}
	}
}
