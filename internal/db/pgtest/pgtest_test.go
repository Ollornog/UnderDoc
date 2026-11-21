// SPDX-License-Identifier: MIT

package pgtest

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// childEnv makes the test binary a stand-in for a test run that gets killed: it starts the server, prints the folder
// and waits.
const childEnv = "UNDERDOC_PGTEST_CHILD"

func TestMain(m *testing.M) {
	if os.Getenv(childEnv) != "" {
		once.Do(start)
		if failure != nil {
			println("child:", failure.Error())
			os.Exit(1)
		}
		_, _ = os.Stdout.WriteString("cluster " + cluster + "\n")
		time.Sleep(time.Hour) // until it is killed
	}
	os.Exit(Main(m))
}

// postmaster returns the process number of the server of a folder, from data/postmaster.pid.
func postmaster(t *testing.T, folder string) int {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(folder, "data", "postmaster.pid"))
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.SplitN(string(content), "\n", 2)[0])
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func running(pid int) bool { return syscall.Kill(pid, 0) == nil }

// TestAKilledRunLeavesNothingBehind kills a test binary that started a server, the way a timeout or an aborted suite
// does, and shows that the next start stops that server and removes its folder, but leaves a running binary's server
// and folders of others alone.
func TestAKilledRunLeavesNothingBehind(t *testing.T) {
	bin, err := binaries()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	child := exec.Command(os.Args[0])
	child.Env = append(os.Environ(), childEnv+"=1", "TMPDIR="+dir)
	out, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	killed, server := false, 0
	defer func() {
		if !killed {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
		sweep(dir, bin)
		// Whatever the test leaves, never a running server, even when sweep fails.
		if server > 0 && running(server) {
			_ = syscall.Kill(server, syscall.SIGQUIT)
		}
	}()
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "cluster ") {
		t.Fatalf("the child did not start a server: %q, %v", line, err)
	}
	folder := strings.TrimSpace(strings.TrimPrefix(line, "cluster "))
	if filepath.Dir(folder) != dir {
		t.Fatalf("the server of the child is in %s, not under %s", folder, dir)
	}
	server = postmaster(t, folder)

	// Others: a folder of another program and a fresh folder without an owner (being set up) stay.
	foreign := filepath.Join(dir, "otherapp-pg-1")
	fresh := filepath.Join(dir, prefix+"fresh")
	for _, f := range []string{foreign, fresh} {
		if err := os.Mkdir(f, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * unowned)
	if err := os.Chtimes(foreign, old, old); err != nil {
		t.Fatal(err)
	}

	// While the child runs, its server is not touched.
	sweep(dir, bin)
	if _, err := os.Stat(folder); err != nil || !running(server) {
		t.Fatalf("the server of a running test binary was touched: %v, running %v", err, running(server))
	}

	if err := child.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	killed = true
	if !running(server) {
		t.Fatal("the server ended with the killed binary; this test shows nothing then")
	}

	sweep(dir, bin)
	deadline := time.Now().Add(30 * time.Second)
	for running(server) && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if running(server) {
		t.Errorf("the server %d of the killed binary still runs", server)
	}
	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		t.Errorf("the folder of the killed binary is still there: %v", err)
	}
	for _, f := range []string{foreign, fresh} {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("%s was removed: %v", f, err)
		}
	}
}

// TestAFolderWithoutOwnerGoesWhenOld covers the folders of versions before the owner file.
func TestAFolderWithoutOwnerGoesWhenOld(t *testing.T) {
	bin, err := binaries()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	stale := filepath.Join(dir, prefix+"stale")
	if err := os.Mkdir(stale, 0o700); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * unowned)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	sweep(dir, bin)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("an old folder without owner and server stayed: %v", err)
	}
}

// TestAReusedNumberIsNotTheOwner: the owner's number now belongs to another process (here this one, with another start
// time), so the folder is an orphan.
func TestAReusedNumberIsNotTheOwner(t *testing.T) {
	me := identity(os.Getpid())
	if !strings.Contains(me, " ") {
		t.Fatalf("no start time for this process: %q", me)
	}
	dir := t.TempDir()
	folder := filepath.Join(dir, prefix+"reused")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Lstat(folder)
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(folder, owner), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(strconv.Itoa(os.Getpid()) + " 1")
	if !orphaned(folder, info) {
		t.Error("a number with another start time was taken as the owner")
	}
	write(me)
	if orphaned(folder, info) {
		t.Error("the running owner was taken as gone")
	}
}
