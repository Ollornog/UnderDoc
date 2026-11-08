// SPDX-License-Identifier: MIT

// Package pgtest starts a throwaway PostgreSQL for tests: initdb into a temporary folder, a server that listens only on
// a Unix socket there, and a fresh database per call. It needs the server binaries (initdb, pg_ctl, postgres), found
// through $PG_BIN or under /usr/lib/postgresql/<version>/bin. Without them the test fails: a skipped database test is
// not a green one.
package pgtest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
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
	// Not t.TempDir: the server outlives a single test. The folder goes when the test binary ends (Main below).
	cluster, err = os.MkdirTemp("", "underdoc-pg-")
	if err != nil {
		failure = err
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
