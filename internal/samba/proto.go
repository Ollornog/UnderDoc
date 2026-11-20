// SPDX-License-Identifier: MIT

// Package samba is the channel between UnderDoc and its Samba companion (ADR-30): the requests the helper of the companion
// accepts, the rules for names and paths both sides check, and the encoding of POSIX ACLs. UnderDoc never runs as root;
// the helper runs smbd in a container of its own and takes a few defined requests over a Unix socket, one JSON request per
// connection.
package samba

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"
)

// The requests the helper accepts. Nothing else is ever run.
const (
	OpPing  = "ping"  // the helper and smbd are up
	OpApply = "apply" // the whole wanted state of users and shares
	OpACL   = "acl"   // ACLs for directories and the files directly in them
	OpOpen  = "open"  // the files open over SMB now
	OpWho   = "who"   // who wrote, made, renamed or deleted a path lately
	OpRoom  = "room"  // the room each library that a limit binds has left, for dfree command
)

// Request is one request to the helper.
type Request struct {
	Op    string    `json:"op"`
	State *State    `json:"state,omitempty"`
	ACL   []PathACL `json:"acl,omitempty"`
	// Paths limits open and who to these paths, absolute below the tenant's directory; empty means all.
	Paths []string `json:"paths,omitempty"`
	// Room is the whole list of libraries a limit binds (room); a library not in it has the room of the disk.
	Room []Room `json:"room,omitempty"`
}

// Room is what a library still takes: Free bytes of Total, by the quotas of UnderDoc (ADR-28, ADR-30). Its Path is the
// directory of the library; dfree command answers it for everything below.
type Room struct {
	Path  string `json:"path"`
	Free  int64  `json:"free"`
	Total int64  `json:"total"`
}

// State is everything the companion shall know: its users and its shares. Applying the same state twice changes nothing.
type State struct {
	Users  []User  `json:"users"`
	Shares []Share `json:"shares"`
}

// User is a Samba user: a Unix user of the companion with the NT hash of its app password, and the directory [homes]
// shows to it.
type User struct {
	Name   string `json:"name"`
	UID    int    `json:"uid"`
	NTHash string `json:"nt_hash"`
	Home   string `json:"home,omitempty"`
}

// GoString keeps the hash out of every %#v.
func (u User) GoString() string { return fmt.Sprintf("samba.User{Name:%q, UID:%d}", u.Name, u.UID) }

// String keeps the hash out of every %v.
func (u User) String() string { return u.GoString() }

// Share is a share of the companion: its name, the directory it shows, and the users that reach it.
type Share struct {
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Comment string   `json:"comment,omitempty"`
	Users   []string `json:"users"`
}

// Access is what an ACL grants one UID: passage only, reading, or writing.
type Access string

// The kinds of access.
const (
	Pass  Access = "pass"  // --x on a directory: reach what lies below, see nothing
	Read  Access = "read"  // r-x on a directory, r-- on a file
	Write Access = "write" // rwx on a directory, rw- on a file
)

// PathACL is the ACL of one directory and of the files directly in it. Access maps UIDs to what they may.
type PathACL struct {
	Path   string         `json:"path"`
	Access map[int]Access `json:"access"`
}

// Response is the answer of the helper.
type Response struct {
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
	Version string `json:"version,omitempty"`
	Samba   string `json:"samba,omitempty"`
	// Boot names this start of the helper; a new one means the companion forgot its users and shares.
	Boot string  `json:"boot,omitempty"`
	Open []Open  `json:"open,omitempty"`
	Who  []Event `json:"who,omitempty"`
	// First and Last number the oldest and the newest event the audit still keeps (who). A First beyond the Last a caller
	// saw before means events were pushed out of the audit in between: who changed something then is unknown.
	First uint64 `json:"first,omitempty"`
	Last  uint64 `json:"last,omitempty"`
}

// Open is a file open over SMB.
type Open struct {
	Path  string    `json:"path"`
	User  string    `json:"user"`
	Write bool      `json:"write"`
	Since time.Time `json:"since"`
}

// Event is an operation the audit of smbd saw: who did what to a path, and when.
type Event struct {
	Path string    `json:"path"`
	To   string    `json:"to,omitempty"`
	User string    `json:"user"`
	Op   string    `json:"op"`
	At   time.Time `json:"at"`
}

// Errors both sides tell apart.
var (
	ErrName = errors.New("a Samba user name has 1 to 32 characters a-z, 0-9, '.', '_' or '-', starts with a letter, and is not a user of the system")
	ErrPath = errors.New("a path of the share lies below the directory of the tenant, absolute and clean")
	ErrUID  = errors.New("a UID of a Samba user lies in the range of the companion")
	ErrHash = errors.New("an NT hash is 32 hexadecimal digits")
)

var (
	namePattern  = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,31}$`)
	hashPattern  = regexp.MustCompile(`^[0-9A-F]{32}$`)
	sharePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,79}$`)
)

// reserved are names Samba or the container knows already, or that Samba treats specially.
var reserved = map[string]bool{
	"root": true, "daemon": true, "bin": true, "sys": true, "sync": true, "games": true, "man": true, "lp": true,
	"mail": true, "news": true, "uucp": true, "proxy": true, "www-data": true, "backup": true, "list": true, "irc": true,
	"gnats": true, "nobody": true, "nogroup": true, "systemd-network": true, "messagebus": true, "_apt": true,
	"underdoc": true, "admin": true, "administrator": true, "guest": true, "homes": true, "printers": true,
	"global": true, "ipc": true, "print": true,
}

// ValidName tells whether a Samba user name is allowed.
func ValidName(name string) bool { return namePattern.MatchString(name) && !reserved[name] }

// ValidShareName tells whether a share name is allowed: letters, digits, space, '.', '_' and '-', up to 80, and none of
// the sections Samba keeps for itself.
func ValidShareName(name string) bool {
	return sharePattern.MatchString(name) && !reserved[strings.ToLower(name)] && !strings.HasSuffix(name, " ")
}

// ShareName makes a share name of a title: what the rules do not take becomes '_', and an empty result is "Files".
func ShareName(title string) string {
	var b strings.Builder
	for _, r := range title {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == ' ', r == '.', r == '_', r == '-':
			b.WriteRune(r)
		case r == 'ä':
			b.WriteString("ae")
		case r == 'ö':
			b.WriteString("oe")
		case r == 'ü':
			b.WriteString("ue")
		case r == 'Ä':
			b.WriteString("Ae")
		case r == 'Ö':
			b.WriteString("Oe")
		case r == 'Ü':
			b.WriteString("Ue")
		case r == 'ß':
			b.WriteString("ss")
		default:
			b.WriteRune('_')
		}
	}
	name := strings.Trim(b.String(), " ._-")
	if len(name) > 80 {
		name = strings.TrimRight(name[:80], " ")
	}
	if name == "" || !ValidShareName(name) {
		name = "Files " + strings.Trim(name, " ")
		if !ValidShareName(name) {
			name = "Files"
		}
	}
	return name
}

// Below tells whether p is root itself or lies below it, both absolute and clean, and without control characters, which
// would end a line of smb.conf.
func Below(root, p string) bool {
	if !path.IsAbs(p) || path.Clean(p) != p || !path.IsAbs(root) || path.Clean(root) != root || strings.ContainsFunc(p, isControl) {
		return false
	}
	return p == root || strings.HasPrefix(p, strings.TrimSuffix(root, "/")+"/")
}

func isControl(r rune) bool { return r < ' ' || r == 0x7f }

// Check checks a request against the rules of the channel before the helper does anything: names, UIDs, hashes and paths.
func (r Request) Check(root string, uidMin, uidMax int) error {
	switch r.Op {
	case OpPing:
		return nil
	case OpApply:
		if r.State == nil {
			return fmt.Errorf("apply without a state")
		}
		seen := map[string]bool{}
		uids := map[int]bool{}
		for _, u := range r.State.Users {
			if !ValidName(u.Name) || seen[u.Name] {
				return ErrName
			}
			seen[u.Name] = true
			if u.UID < uidMin || u.UID > uidMax || uids[u.UID] {
				return ErrUID
			}
			uids[u.UID] = true
			if !hashPattern.MatchString(u.NTHash) {
				return ErrHash
			}
			if u.Home != "" && (!Below(root, u.Home) || u.Home == root) {
				return ErrPath
			}
		}
		shares := map[string]bool{}
		for _, s := range r.State.Shares {
			if !ValidShareName(s.Name) || shares[strings.ToLower(s.Name)] {
				return fmt.Errorf("share %q: not a valid or unique share name", s.Name)
			}
			shares[strings.ToLower(s.Name)] = true
			// smb.conf substitutes variables such as %U in a path: a path with '%' would be another one for each user.
			if !Below(root, s.Path) || s.Path == root || strings.ContainsRune(s.Path, '%') {
				return ErrPath
			}
			if strings.ContainsFunc(s.Comment, isControl) {
				return fmt.Errorf("share %q: a comment of one line", s.Name)
			}
			for _, u := range s.Users {
				if !seen[u] {
					return fmt.Errorf("share %q names a user that is not in the state", s.Name)
				}
			}
		}
		return nil
	case OpACL:
		for _, a := range r.ACL {
			if !Below(root, a.Path) {
				return ErrPath
			}
			for uid, access := range a.Access {
				if uid < uidMin || uid > uidMax {
					return ErrUID
				}
				if access != Pass && access != Read && access != Write {
					return fmt.Errorf("access %q", access)
				}
			}
		}
		return nil
	case OpRoom:
		seen := map[string]bool{}
		for _, room := range r.Room {
			if !Below(root, room.Path) || room.Path == root || seen[room.Path] {
				return ErrPath
			}
			seen[room.Path] = true
			if room.Free < 0 || room.Total < room.Free {
				return fmt.Errorf("room of %s: free and total do not fit", room.Path)
			}
		}
		return nil
	case OpOpen, OpWho:
		for _, p := range r.Paths {
			if !Below(root, p) {
				return ErrPath
			}
		}
		return nil
	}
	return fmt.Errorf("unknown request %q", r.Op)
}
