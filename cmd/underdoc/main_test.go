// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ollornog/UnderDoc/internal/version"
)

func TestVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"version"}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut.String())
	}
	if got := strings.TrimSpace(out.String()); got != "underdoc "+version.Version {
		t.Errorf("output %q", got)
	}
}

func TestUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"nonsense"}, {"version", "extra"}, {"serve", "a", "b"}, {"probe", "extra"}} {
		var out, errOut bytes.Buffer
		if code := run(args, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "usage:") {
			t.Errorf("%v: exit %d, stderr %q", args, code, errOut.String())
		}
	}
}

func TestServeNeedsAFolder(t *testing.T) {
	t.Setenv("UNDERDOC_DIR", "")
	var out, errOut bytes.Buffer
	if code := run([]string{"serve"}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "no folder") {
		t.Errorf("exit %d, stderr %q", code, errOut.String())
	}
}

func TestServeRefusesAMissingFolder(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"serve", filepath.Join(t.TempDir(), "gone")}, &out, &errOut); code != 1 {
		t.Errorf("exit %d, stderr %q", code, errOut.String())
	}
}

func TestServeRefusesABadInterval(t *testing.T) {
	t.Setenv("UNDERDOC_SCAN_INTERVAL", "soon")
	var out, errOut bytes.Buffer
	if code := run([]string{"serve", t.TempDir()}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "UNDERDOC_SCAN_INTERVAL") {
		t.Errorf("exit %d, stderr %q", code, errOut.String())
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	return listener.Addr().String()
}

// The whole path: a real server on a real folder sees an edit without a restart and stops cleanly.
func TestServeEndToEnd(t *testing.T) {
	dir := t.TempDir()
	page := filepath.Join(dir, "a.md")
	if err := os.WriteFile(page, []byte("---\nid: a\n---\nfirst version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	addr, healthAddr := freeAddr(t), freeAddr(t)
	t.Setenv("UNDERDOC_ADDR", addr)
	t.Setenv("UNDERDOC_HEALTH_ADDR", healthAddr)
	t.Setenv("UNDERDOC_SCAN_INTERVAL", "20ms")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan error, 1)
	var out bytes.Buffer
	go func() { stopped <- serve(ctx, dir, &out) }()

	fetch := func(url string) string {
		response, err := http.Get(url) //nolint:gosec // local test server
		if err != nil {
			return ""
		}
		defer func() { _ = response.Body.Close() }()
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		return body.String()
	}
	waitFor := func(url, want string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if strings.Contains(fetch(url), want) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("%s never showed %q", url, want)
	}
	waitFor("http://"+healthAddr+"/healthz", "ok")
	waitFor("http://"+addr+"/d/a", "first version")
	if err := os.WriteFile(page, []byte("---\nid: a\n---\nsecond, longer version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitFor("http://"+addr+"/d/a", "second, longer version")

	t.Setenv("UNDERDOC_HEALTH_ADDR", healthAddr)
	var probeOut, probeErr bytes.Buffer
	if code := run([]string{"probe"}, &probeOut, &probeErr); code != 0 {
		t.Errorf("probe: exit %d, %s", code, probeErr.String())
	}

	cancel()
	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("stop: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the server does not stop")
	}
}

// There is no sign-in in the application, so without a decision to do otherwise it must not be reachable from outside.
func TestDefaultAddressesAreLoopback(t *testing.T) {
	for _, addr := range []string{defaultAddr, defaultHealthAddr} {
		host, _, err := net.SplitHostPort(addr)
		if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
			t.Errorf("default address %q", addr)
		}
	}
}
