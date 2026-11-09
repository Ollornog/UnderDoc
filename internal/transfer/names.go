// SPDX-License-Identifier: MIT

package transfer

import (
	"strconv"
	"strings"
	"unicode"
)

// maxStem is the longest a file name gets before its extension, in characters: well inside the 255 bytes that Linux and
// Windows allow, also with a counter and ".md" added, and short enough for paths on Windows.
const maxStem = 80

// reserved are names Windows refuses for a file, with or without an extension.
var reserved = map[string]bool{"con": true, "prn": true, "aux": true, "nul": true}

func init() {
	for i := 1; i <= 9; i++ {
		reserved["com"+strconv.Itoa(i)] = true
		reserved["lpt"+strconv.Itoa(i)] = true
	}
}

// safeStem turns a title into a file name without extension that Linux and Windows both accept: no separators or
// characters Windows forbids, no control characters, no leading dot (which would hide it and make the folder source
// skip it), no trailing dot or space, no device name, at most maxStem characters. The name is only a label: the ID in
// the file is the identity.
func safeStem(title string) string {
	var b strings.Builder
	for _, r := range strings.Join(strings.Fields(title), " ") {
		switch {
		case strings.ContainsRune(`<>:"/\|?*`, r), unicode.IsControl(r), r == unicode.ReplacementChar:
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	stem := []rune(b.String())
	if len(stem) > maxStem {
		stem = stem[:maxStem]
	}
	name := strings.TrimRight(string(stem), ". ")
	if strings.HasPrefix(name, ".") {
		name = "_" + name[1:]
	}
	if base, _, _ := strings.Cut(strings.ToLower(name), "."); reserved[strings.TrimSpace(base)] {
		name = "_" + name
	}
	if name == "" {
		name = "untitled"
	}
	return name
}

// namer hands out names that are unique within one folder, without regard to case, since Windows and macOS do not tell
// "Plan" from "plan". A page takes "<stem>.md" and, when it has children, "<stem>" for their folder, so every stem is
// kept for both forms.
type namer struct{ used map[string]bool }

func newNamer(reservedStems ...string) *namer {
	n := &namer{used: map[string]bool{}}
	for _, stem := range reservedStems {
		n.keep(stem)
	}
	return n
}

func (n *namer) keep(stem string) {
	key := strings.ToLower(stem)
	n.used[key], n.used[key+".md"] = true, true
}

func (n *namer) free(stem string) bool {
	key := strings.ToLower(stem)
	return !n.used[key] && !n.used[key+".md"]
}

// take returns a free stem for a title: its safe form, or that form with " (2)", " (3)" and so on.
func (n *namer) take(title string) string {
	base := safeStem(title)
	stem := base
	for i := 2; !n.free(stem); i++ {
		stem = base + " (" + strconv.Itoa(i) + ")"
	}
	n.keep(stem)
	return stem
}
