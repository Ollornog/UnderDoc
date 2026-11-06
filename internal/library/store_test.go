// SPDX-License-Identifier: MIT

package library

import (
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"testing"
	"time"
)

type fakeSource struct {
	in    *Input
	state string
	err   error
	scans []string
}

func (f *fakeSource) Scan(since string) (*Input, string, error) {
	f.scans = append(f.scans, since)
	if f.err != nil {
		return nil, "", f.err
	}
	if since == f.state {
		return nil, f.state, nil
	}
	return f.in, f.state, nil
}

func (f *fakeSource) OpenImage(string) (io.ReadSeekCloser, time.Time, error) {
	return nil, time.Time{}, fs.ErrNotExist
}

func TestStoreKeepsTheLastGoodState(t *testing.T) {
	source := &fakeSource{in: &Input{Pages: []Page{page("a.md", "a")}}, state: "one"}
	store := NewStore(source, slog.New(slog.DiscardHandler))
	if store.Healthy() || store.Library() != nil {
		t.Fatal("an empty store must not be healthy")
	}
	if err := store.Refresh(); err != nil {
		t.Fatal(err)
	}
	first := store.Library()
	if first == nil || first.Len() != 1 || !store.Healthy() {
		t.Fatalf("after the first refresh: %+v", first)
	}

	if err := store.Refresh(); err != nil {
		t.Fatal(err)
	}
	if store.Library() != first {
		t.Error("nothing changed, so the snapshot must stay")
	}

	source.err = errors.New("disk gone")
	if err := store.Refresh(); err == nil {
		t.Fatal("the failure must be reported")
	}
	if store.Library() != first {
		t.Error("the last good state must stay")
	}
	if store.Healthy() {
		t.Error("a failing source must show in the health endpoint")
	}

	source.err = nil
	source.in = &Input{Pages: []Page{page("a.md", "a"), page("b.md", "b")}}
	source.state = "two"
	if err := store.Refresh(); err != nil {
		t.Fatal(err)
	}
	if store.Library().Len() != 2 || !store.Healthy() {
		t.Errorf("recovery failed: %d documents", store.Library().Len())
	}
	if last := source.scans[len(source.scans)-1]; last != "one" {
		t.Errorf("the failed scan must not lose the state, got %q", last)
	}
}
