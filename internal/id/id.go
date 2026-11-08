// SPDX-License-Identifier: MIT

// Package id makes the IDs of objects and blocks (ADR-18): short, random, never derived from a place, so a move or a
// rename never changes them.
package id

import (
	"crypto/rand"
	"regexp"
)

// The alphabet of Crockford's base32 without the letters that look like digits, all lower case: an ID can be read out
// loud and typed without mixing up 0 and o or 1 and l.
const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// Length of an object ID: 10 characters of 5 bits are 50 bits, enough to draw at random without a central counter.
const Length = 10

var valid = regexp.MustCompile(`^[0-9a-hjkmnp-tv-z]{10}$`)

// New returns a fresh object ID.
func New() string {
	b := make([]byte, Length)
	if _, err := rand.Read(b); err != nil {
		panic("id: no randomness: " + err.Error()) // crypto/rand does not fail on supported systems
	}
	for i := range b {
		b[i] = alphabet[b[i]&31]
	}
	return string(b)
}

// Valid reports whether s has the form of an ID that New makes.
func Valid(s string) bool { return valid.MatchString(s) }
