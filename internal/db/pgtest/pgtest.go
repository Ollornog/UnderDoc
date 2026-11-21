// SPDX-License-Identifier: MIT

// Package pgtest starts a throwaway PostgreSQL for tests: initdb into a temporary folder, a server that listens only on
// a Unix socket there, and a fresh database per call. It needs the server binaries (initdb, pg_ctl, postgres), found
// through $PG_BIN or under /usr/lib/postgresql/<version>/bin. Without them the test fails: a skipped database test is
// not a green one.
//
// The server is stopped when the test binary ends through Main. A binary that ends otherwise (killed, a timeout, a
// panic) cannot stop it, so every folder names its test binary in a file "owner", and the next start stops the server
// of every folder whose binary is gone and removes the folder. Only folders of this package are touched: the prefix
// underdoc-pg-, the same user, and an owner that is not running.
package pgtest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	prefix = "underdoc-pg-"
	owner  = "owner" // in the folder: "<pid> <start time of the process>" of the test binary
	// A folder without an owner is either being set up right now or comes from a version before the owner file; it is
	// left alone until it is this old.
	unowned = time.Hour
)

var (
	once    sync.Once
	cluster string // the folder with data and socket
	failure error
	count   atomic.Int64
)

func binaries() (string, error) {
	if dir := os.Getenv("PG_BIN"); dir != "" {
		return dir, nil
	}
	found, _ := filepath.Glob("/usr/lib/postgresql/*/bin/initdb")
	if len(found) == 0 {
		return "", fmt.Errorf("no PostgreSQL server binaries: set PG_BIN or install them (the CI images have them)")
	}
	slices.Sort(found)
	return filepath.Dir(found[len(found)-1]), nil
}

func start() {
	bin, err := binaries()
	if err != nil {
		failure = err
		return
	}
	sweep(os.TempDir(), bin)
	// Not t.TempDir: the server outlives a single test. The folder goes when the test binary ends (Main below), or with
	// the next start after the binary was killed (sweep).
	cluster, err = os.MkdirTemp("", prefix)
	if err != nil {
		failure = err
		return
	}
	if failure = os.WriteFile(filepath.Join(cluster, owner), []byte(identity(os.Getpid())), 0o600); failure != nil {
		return
	}
	data := filepath.Join(cluster, "data")
	run := func(name string, args ...string) error {
		out, err := exec.Command(filepath.Join(bin, name), args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %v\n%s", name, err, out)
		}
		return nil
	}
	if failure = run("initdb", "-D", data, "-U", "underdoc", "-A", "trust", "--no-sync", "-E", "UTF8", "--locale=C"); failure != nil {
		return
	}
	options := fmt.Sprintf("-k %s -c listen_addresses='' -c fsync=off -c synchronous_commit=off -c full_page_writes=off", cluster)
	failure = run("pg_ctl", "-D", data, "-o", options, "-l", filepath.Join(cluster, "log"), "-w", "-t", "30", "start")
}

// identity returns "<pid> <start time>" of a process, or "" if it does not run. The start time tells a process from a
// later one that got the same number; where /proc cannot say it, the number alone counts.
func identity(pid int) string {
	if err := syscall.Kill(pid, 0); err != nil && err != syscall.EPERM {
		return ""
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return strconv.Itoa(pid)
	}
	// The name in parentheses may hold spaces; the fields after it start with the state (field 3), the start time is
	// field 22.
	fields := strings.Fields(string(stat[strings.LastIndexByte(string(stat), ')')+1:]))
	if len(fields) < 20 {
		return strconv.Itoa(pid)
	}
	return strconv.Itoa(pid) + " " + fields[19]
}

// orphaned tells whether a folder of this package belongs to a test binary that no longer runs.
func orphaned(folder string, info os.FileInfo) bool {
	content, err := os.ReadFile(filepath.Join(folder, owner))
	if err != nil {
		return os.IsNotExist(err) && time.Since(info.ModTime()) > unowned
	}
	recorded := strings.TrimSpace(string(content))
	pid, err := strconv.Atoi(strings.Fields(recorded + " x")[0])
	if err != nil || pid <= 0 {
		return time.Since(info.ModTime()) > unowned
	}
	now := identity(pid)
	if now == "" {
		return true
	}
	// Both with a start time: they must match. One without: the number still runs, so it is taken as alive.
	return strings.Contains(recorded, " ") && strings.Contains(now, " ") && now != recorded
}

// sweep stops the servers of the folders under dir whose test binary is gone and removes the folders. Only folders of
// this package and this user; a server that does not stop keeps its folder, so no server runs on deleted files.
func sweep(dir, bin string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), prefix) || !entry.IsDir() {
			continue
		}
		folder := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(folder)
		if err != nil || !info.IsDir() {
			continue
		}
		if st, ok := info.Sys().(*syscall.Stat_t); !ok || int(st.Uid) != os.Getuid() {
			continue
		}
		if !orphaned(folder, info) {
			continue
		}
		data := filepath.Join(folder, "data")
		pgctl := filepath.Join(bin, "pg_ctl")
		_ = exec.Command(pgctl, "-D", data, "-m", "immediate", "-w", "-t", "30", "stop").Run()
		// pg_ctl status: 3 is "no server running", 4 "no data directory"; anything else and the folder stays.
		if err := exec.Command(pgctl, "-D", data, "status").Run(); err != nil {
			if exit, ok := err.(*exec.ExitError); ok && (exit.ExitCode() == 3 || exit.ExitCode() == 4) {
				_ = os.RemoveAll(folder)
			}
		}
	}
}

// Main runs the tests of a package and stops the server afterwards. Call it from TestMain.
func Main(m *testing.M) int {
	code := m.Run()
	if cluster != "" {
		if bin, err := binaries(); err == nil {
			_ = exec.Command(filepath.Join(bin, "pg_ctl"), "-D", filepath.Join(cluster, "data"), "-m", "immediate", "-w", "stop").Run()
		}
		_ = os.RemoveAll(cluster)
	}
	return code
}

// URL returns the address of a new, empty database, only for this test.
func URL(t *testing.T) string {
	t.Helper()
	once.Do(start)
	if failure != nil {
		t.Fatal(failure)
	}
	name := fmt.Sprintf("t%d_%d", os.Getpid(), count.Add(1))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, address("postgres"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(context.Background()) }()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	return address(name)
}

func address(database string) string {
	return "postgres://underdoc@/" + database + "?host=" + strings.ReplaceAll(cluster, "/", "%2F")
}
