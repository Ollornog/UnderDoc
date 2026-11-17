// SPDX-License-Identifier: MIT

// Package auth holds the pieces of signing in that need no database and no HTTP (ADR-23): passwords with Argon2id, TOTP
// codes, the limit against guessing and the tokens of sessions.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters after the OWASP cheat sheet (19 MiB, two passes, one lane): strong against guessing, and small
// enough that many sign-ins at once do not exhaust the memory of the server.
const (
	argonMemory  = 19 * 1024
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
)

// Lengths of a password, in characters. The upper bound keeps hashing cheap for the server.
const (
	MinPassword = 12
	MaxPassword = 200
)

// Errors about passwords.
var (
	ErrPasswordLength = fmt.Errorf("a password has %d to %d characters", MinPassword, MaxPassword)
	ErrHash           = errors.New("not a password hash of UnderDoc")
)

// CheckPassword tells whether a new password may be used.
func CheckPassword(password string) error {
	if n := utf8.RuneCountInString(password); n < MinPassword || n > MaxPassword {
		return ErrPasswordLength
	}
	return nil
}

// HashPassword returns the password as an Argon2id hash in the usual text form, with a random salt.
func HashPassword(password string) (string, error) {
	if err := CheckPassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// dummyHash is checked when there is no account, so a missing account takes as long as a wrong password.
var dummyHash, _ = HashPassword("not a password of anyone at all")

// VerifyPassword tells whether a password matches a hash. An empty hash, as for an account without a password, takes
// the same time and never matches.
func VerifyPassword(hash, password string) bool {
	if hash == "" {
		_, _ = verify(dummyHash, password)
		return false
	}
	ok, err := verify(hash, password)
	return err == nil && ok
}

func verify(hash, password string) (bool, error) {
	parts := strings.Split(hash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrHash
	}
	var version int
	var memory, passes uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, ErrHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &passes, &threads); err != nil || memory > 1<<20 || passes > 10 || threads == 0 {
		return false, ErrHash
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return false, ErrHash
	}
	want, err := enc.DecodeString(parts[5])
	if err != nil || len(want) == 0 || len(want) > 64 {
		return false, ErrHash
	}
	if utf8.RuneCountInString(password) > MaxPassword {
		return false, nil
	}
	got := argon2.IDKey([]byte(password), salt, passes, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// NewToken returns a random token for a cookie or a link, and the hash that the database keeps of it, so a copy of the
// database lets nobody in.
func NewToken() (token string, hash []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	return token, HashToken(token), nil
}

// HashToken returns the hash the database keeps of a token.
func HashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
