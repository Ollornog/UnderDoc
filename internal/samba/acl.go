// SPDX-License-Identifier: MIT

package samba

import (
	"encoding/binary"
	"sort"
	"strings"
	"unicode/utf16"
)

// The extended attributes of POSIX ACLs on Linux, and the layout of their value (linux/posix_acl_xattr.h): a version,
// then entries of a tag, permissions and an ID, sorted by tag and then by ID.
const (
	XattrAccess  = "system.posix_acl_access"
	XattrDefault = "system.posix_acl_default"

	aclVersion  = 2
	tagUserObj  = 0x01
	tagUser     = 0x02
	tagGroupObj = 0x04
	tagMask     = 0x10
	tagOther    = 0x20
	undefinedID = 0xffffffff
)

// ACL returns the value of an ACL attribute: the owner and the named UID of UnderDoc get everything, group and others
// nothing, and each UID what access says, for a directory or a file. Passage on a file gives nothing. UnderDoc is named
// besides being the owner, because a file a Samba user makes belongs to that user until the helper hands it over, and
// UnderDoc must read it at once.
func ACL(access map[int]Access, dir bool, underdoc int) []byte {
	type entry struct {
		tag  uint16
		perm uint16
		id   uint32
	}
	const r, w, x = 4, 2, 1
	owner := uint16(r | w)
	if dir {
		owner |= x
	}
	entries := []entry{{tagUserObj, owner, undefinedID}}
	all := map[int]Access{}
	for uid, a := range access {
		all[uid] = a
	}
	all[underdoc] = Write
	access = all
	uids := make([]int, 0, len(access))
	for uid := range access {
		uids = append(uids, uid)
	}
	sort.Ints(uids)
	mask := uint16(0)
	for _, uid := range uids {
		var perm uint16
		switch access[uid] {
		case Pass:
			if dir {
				perm = x
			}
		case Read:
			perm = r
			if dir {
				perm |= x
			}
		case Write:
			perm = r | w
			if dir {
				perm |= x
			}
		}
		if perm == 0 {
			continue
		}
		mask |= perm
		entries = append(entries, entry{tagUser, perm, uint32(uid)})
	}
	entries = append(entries, entry{tagGroupObj, 0, undefinedID}, entry{tagMask, mask, undefinedID}, entry{tagOther, 0, undefinedID})
	out := make([]byte, 4+8*len(entries))
	binary.LittleEndian.PutUint32(out, aclVersion)
	for i, e := range entries {
		o := 4 + 8*i
		binary.LittleEndian.PutUint16(out[o:], e.tag)
		binary.LittleEndian.PutUint16(out[o+2:], e.perm)
		binary.LittleEndian.PutUint32(out[o+4:], e.id)
	}
	return out
}

// ParseACL reads the UIDs and permission bits (r=4, w=2, x=1) of the named users of an ACL attribute, for tests and
// for the scan that compares what is on disk.
func ParseACL(value []byte) map[int]int {
	out := map[int]int{}
	if len(value) < 4 || binary.LittleEndian.Uint32(value) != aclVersion {
		return out
	}
	for o := 4; o+8 <= len(value); o += 8 {
		if binary.LittleEndian.Uint16(value[o:]) == tagUser {
			out[int(binary.LittleEndian.Uint32(value[o+4:]))] = int(binary.LittleEndian.Uint16(value[o+2:]))
		}
	}
	return out
}

// NTHash is the hash Samba keeps of a password for NTLM: MD4 over its UTF-16LE form, as 32 upper-case hex digits.
func NTHash(password string) string {
	units := utf16.Encode([]rune(password))
	b := make([]byte, 2*len(units))
	for i, u := range units {
		binary.LittleEndian.PutUint16(b[2*i:], u)
	}
	sum := md4(b)
	const hexDigits = "0123456789ABCDEF"
	var s strings.Builder
	for _, c := range sum {
		s.WriteByte(hexDigits[c>>4])
		s.WriteByte(hexDigits[c&15])
	}
	return s.String()
}
