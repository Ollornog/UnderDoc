// SPDX-License-Identifier: MIT

package share

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recording is what T-1 recorded of one save over a real Samba share (scripts/t1-libreoffice.sh, testdata/t1): the
// client, the operations smbd's audit saw in their order, and what was measured on disk.
type recording struct {
	Client   string `json:"client"`
	File     string `json:"file"`
	Measured string `json:"measured"`
	Steps    []step `json:"steps"`
	// NewInode tells that the saved file is a new file on disk, not the old one written again.
	NewInode bool `json:"new_inode"`
	// FileRightSurvived tells that an ACL set on the single file still held after the save.
	FileRightSurvived bool `json:"file_right_survived"`
	// HiddenWithoutRead tells that a user without read on the file did not see it in the listing of its folder.
	HiddenWithoutRead bool `json:"hidden_without_read"`
	// ShownAfterTheSave tells that the same user saw it after the save.
	ShownAfterTheSave bool `json:"shown_after_the_save"`
}

// step is one operation of smbd's audit, with paths relative to the library.
type step struct {
	Op   string `json:"op"`
	Path string `json:"path"`
	To   string `json:"to,omitempty"`
}

// replay does on disk what the steps did on the share, as far as it changes files: making, writing, truncating,
// renaming and deleting. Reads and the rest change nothing and are passed over.
func replay(t *testing.T, dir string, steps []step) {
	t.Helper()
	for i, s := range steps {
		full := filepath.Join(dir, filepath.FromSlash(s.Path))
		var err error
		switch s.Op {
		case "create_file", "openat":
			if _, statErr := os.Lstat(full); errors.Is(statErr, os.ErrNotExist) && !strings.HasSuffix(s.Path, "/") {
				// Only a step that made the file creates it; opening one that is there changes nothing.
				err = os.WriteFile(full, nil, 0o660)
			}
		case "pwrite", "pwrite_send":
			var file *os.File
			if file, err = os.OpenFile(full, os.O_WRONLY|os.O_APPEND, 0); err == nil {
				_, err = file.WriteString("written in step " + string(rune('a'+i%26)) + "\n")
				if cerr := file.Close(); err == nil {
					err = cerr
				}
			}
		case "ftruncate":
			err = os.Truncate(full, 0)
		case "renameat":
			err = os.Rename(full, filepath.Join(dir, filepath.FromSlash(s.To)))
		case "unlinkat":
			err = os.RemoveAll(full)
		case "mkdirat":
			err = os.Mkdir(full, 0o770)
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("step %d %+v: %v", i, s, err)
		}
	}
}

// The saves recorded in T-1 against real Samba, replayed on disk: each stays the same object with a new version by
// whoever saved it, carries its ID again, puts nothing into the trash and makes no object of a temporary file.
func TestRecordedSaves(t *testing.T) {
	names, err := filepath.Glob(filepath.Join("testdata", "t1", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("no recordings in testdata/t1: scripts/t1-libreoffice.sh writes them")
	}
	for _, name := range names {
		t.Run(filepath.Base(name), func(t *testing.T) {
			raw, err := os.ReadFile(name) // #nosec G304 -- a fixture of the repository
			if err != nil {
				t.Fatal(err)
			}
			var rec recording
			if err := json.Unmarshal(raw, &rec); err != nil {
				t.Fatal(err)
			}
			if len(rec.Steps) == 0 {
				t.Fatal("a recording without steps")
			}
			f := setup(t)
			file := f.upload(anna.ID, f.company.ID, rec.File, "made by LibreOffice")
			f.sync()
			replay(t, f.path(""), rec.Steps)
			f.h.wrote("anna", f.path(rec.File))
			f.sync()
			versions := f.versions(file.ID)
			if len(versions) != 2 || versions[0].Author != anna.ID {
				t.Fatalf("versions: %+v", versions)
			}
			if readID(f.path(rec.File)) != file.ID {
				t.Error("the saved file does not carry its ID")
			}
			if trash, _ := f.db.Trash(inMain(), anna.ID); len(trash) != 0 {
				t.Errorf("trash: %+v", trash)
			}
			if list, _ := f.db.FilesIn(inMain(), f.company.ID, anna.ID); len(list) != 1 {
				var titles []string
				for _, x := range list {
					titles = append(titles, x.Title)
				}
				t.Errorf("files after the save: %v", titles)
			}
		})
	}
}
