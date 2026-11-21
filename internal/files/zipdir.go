// SPDX-License-Identifier: MIT

package files

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
)

// Go's zip reader reads the whole directory of an archive into memory before anyone can count its parts, and it reads
// headers until one is not a header, whatever the count at the end of the archive says. A file of a few hundred
// megabytes of empty directory entries would be millions of them in memory. ZipParts counts the headers of the directory
// first, with a small buffer, and stops past a limit.

const (
	zipEnd        = 0x06054b50 // end of central directory
	zip64Locator  = 0x07064b50
	zip64End      = 0x06064b50
	zipDirHeader  = 0x02014b50
	zipEndLen     = 22
	zipDirLen     = 46
	zipMaxComment = 0xffff
)

// ZipParts counts the entries of the central directory of a zip archive, at most limit+1: more than limit is too many.
// It reads where Go's reader would read, from the offset the end record gives and from where the directory ends before
// the end record (an archive with data in front). An archive it cannot make sense of counts 0: zip.NewReader refuses
// it then.
func ZipParts(r io.ReaderAt, size int64, limit int) int {
	end, at := zipEndRecord(r, size)
	if end == nil {
		return 0
	}
	dirSize := uint64(binary.LittleEndian.Uint32(end[12:16]))
	dirOffset := uint64(binary.LittleEndian.Uint32(end[16:20]))
	if at >= 20 {
		var loc [20]byte
		if _, err := r.ReadAt(loc[:], at-20); err == nil && binary.LittleEndian.Uint32(loc[0:4]) == zip64Locator {
			var rec [56]byte
			off := binary.LittleEndian.Uint64(loc[8:16])
			if off < uint64(size) {
				if _, err := r.ReadAt(rec[:], int64(off)); err == nil && binary.LittleEndian.Uint32(rec[0:4]) == zip64End {
					dirSize = binary.LittleEndian.Uint64(rec[40:48])
					dirOffset = binary.LittleEndian.Uint64(rec[48:56])
				}
			}
		}
	}
	n := countHeaders(r, size, dirOffset, limit)
	if dirSize <= uint64(at) {
		n = max(n, countHeaders(r, size, uint64(at)-dirSize, limit))
	}
	return n
}

// zipEndRecord finds the end record of a zip archive in its last bytes, and where it starts.
func zipEndRecord(r io.ReaderAt, size int64) ([]byte, int64) {
	n := min(size, zipEndLen+zipMaxComment)
	if n < zipEndLen {
		return nil, 0
	}
	tail := make([]byte, n)
	if _, err := r.ReadAt(tail, size-n); err != nil && err != io.EOF {
		return nil, 0
	}
	sig := binary.LittleEndian.AppendUint32(nil, zipEnd)
	for i := len(tail) - zipEndLen; i >= 0; i-- {
		if bytes.Equal(tail[i:i+4], sig) {
			return tail[i : i+zipEndLen], size - n + int64(i)
		}
	}
	return nil, 0
}

// countHeaders counts directory headers one after another from an offset, at most limit+1.
func countHeaders(r io.ReaderAt, size int64, offset uint64, limit int) int {
	if offset >= uint64(size) {
		return 0
	}
	b := bufio.NewReaderSize(io.NewSectionReader(r, int64(offset), size-int64(offset)), 64<<10)
	var h [zipDirLen]byte
	n := 0
	for n <= limit {
		if _, err := io.ReadFull(b, h[:]); err != nil || binary.LittleEndian.Uint32(h[0:4]) != zipDirHeader {
			return n
		}
		skip := int(binary.LittleEndian.Uint16(h[28:30])) + int(binary.LittleEndian.Uint16(h[30:32])) + int(binary.LittleEndian.Uint16(h[32:34]))
		if _, err := b.Discard(skip); err != nil {
			return n
		}
		n++
	}
	return n
}
