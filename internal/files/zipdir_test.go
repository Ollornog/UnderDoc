// SPDX-License-Identifier: MIT

package files

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"runtime"
	"testing"
)

func zipOfParts(t *testing.T, n int) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for i := range n {
		if _, err := z.CreateHeader(&zip.FileHeader{Name: fmt.Sprint(i), Method: zip.Store}); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestZipPartsCountsTheDirectory: the headers of the directory are counted as Go's reader would read them: from the
// offset of the end record, behind data in front of the archive, and whatever the count of the end record says, which
// is 16 bits only and can lie.
func TestZipPartsCountsTheDirectory(t *testing.T) {
	plain := zipOfParts(t, 3000)
	if n := ZipParts(bytes.NewReader(plain), int64(len(plain)), MaxParts); n != MaxParts+1 {
		t.Errorf("3000 parts counted as %d", n)
	}
	if n := ZipParts(bytes.NewReader(plain), int64(len(plain)), 5000); n != 3000 {
		t.Errorf("3000 parts counted as %d with room", n)
	}
	front := append(bytes.Repeat([]byte("x"), 1000), plain...)
	if _, err := zip.NewReader(bytes.NewReader(front), int64(len(front))); err != nil {
		t.Fatalf("Go reads an archive with data in front: %v", err)
	}
	if n := ZipParts(bytes.NewReader(front), int64(len(front)), MaxParts); n != MaxParts+1 {
		t.Errorf("behind data in front: %d", n)
	}
	// 65536 + 5 parts, and a zip64 end record that says 5: Go reads them all, since it compares 16 bits.
	lying := zipOfParts(t, 65536+5)
	end := bytes.LastIndex(lying, []byte("PK\x06\x06"))
	binary.LittleEndian.PutUint64(lying[end+24:], 5)
	binary.LittleEndian.PutUint64(lying[end+32:], 5)
	if r, err := zip.NewReader(bytes.NewReader(lying), int64(len(lying))); err != nil || len(r.File) != 65536+5 {
		t.Fatalf("Go reads the lying archive: %v", err)
	}
	if n := ZipParts(bytes.NewReader(lying), int64(len(lying)), MaxParts); n != MaxParts+1 {
		t.Errorf("an end record that says 5: %d", n)
	}
	for _, broken := range [][]byte{nil, []byte("PK"), bytes.Repeat([]byte{0}, 100), plain[:len(plain)/2]} {
		if n := ZipParts(bytes.NewReader(broken), int64(len(broken)), MaxParts); n != 0 {
			t.Errorf("%d bytes of no archive: %d", len(broken), n)
		}
	}
}

// TestOfficeTextCountsPartsFirst: an Office file with a directory of 200000 parts is refused before Go's reader holds
// them all.
func TestOfficeTextCountsPartsFirst(t *testing.T) {
	b := zipOfParts(t, 200_000)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := read(t, b, "application/vnd.oasis.opendocument.text")
	runtime.ReadMemStats(&after)
	if err == nil {
		t.Fatal("read")
	}
	if n := after.TotalAlloc - before.TotalAlloc; n > 8<<20 {
		t.Errorf("200000 parts allocated %d MiB before they were refused", n>>20)
	}
}
