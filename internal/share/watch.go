// SPDX-License-Identifier: MIT

package share

import (
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

// watcher tells when something changed below a directory, through inotify: a watch per folder, added again after every
// pass, so new folders are watched too. It says only that something changed; the pass that follows looks at everything,
// so a missed or merged event costs nothing, and an overflow of the queue simply asks for that pass.
type watcher struct {
	fd      int
	file    *os.File
	changed func()
	mu      sync.Mutex
	watched map[string]int
	closed  bool
}

const watchMask = unix.IN_CREATE | unix.IN_DELETE | unix.IN_MOVED_FROM | unix.IN_MOVED_TO | unix.IN_CLOSE_WRITE |
	unix.IN_DELETE_SELF | unix.IN_MOVE_SELF

func newWatcher(changed func()) (*watcher, error) {
	// Non-blocking, so the runtime's poller reads it and closing the file ends the read.
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, err
	}
	w := &watcher{fd: fd, file: os.NewFile(uintptr(fd), "inotify"), changed: changed, watched: map[string]int{}}
	go w.read()
	return w, nil
}

// read reports every batch of events, until the watcher is closed.
func (w *watcher) read() {
	buf := make([]byte, 64<<10)
	for {
		n, err := w.file.Read(buf)
		if err != nil {
			return
		}
		if n <= 0 {
			return
		}
		w.mu.Lock()
		closed := w.closed
		w.mu.Unlock()
		if closed {
			return
		}
		w.changed()
	}
}

// watch adds a watch on every folder below dir, links not followed, and forgets watches of folders that are gone.
func (w *watcher) watch(dir string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	seen := map[string]bool{}
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if p != dir && ignored(d.Name()) {
			return filepath.SkipDir
		}
		seen[p] = true
		if _, ok := w.watched[p]; ok {
			return nil
		}
		wd, err := unix.InotifyAddWatch(w.fd, p, watchMask|unix.IN_DONT_FOLLOW)
		if err == nil {
			w.watched[p] = wd
		}
		return nil
	})
	for p, wd := range w.watched {
		if !seen[p] && (p == dir || len(p) > len(dir) && p[:len(dir)] == dir) {
			_, _ = unix.InotifyRmWatch(w.fd, uint32(wd)) // #nosec G115 -- a watch descriptor
			delete(w.watched, p)
		}
	}
}

func (w *watcher) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}
	w.closed = true
	_ = w.file.Close()
}
