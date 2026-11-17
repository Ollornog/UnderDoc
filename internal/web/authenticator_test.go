// SPDX-License-Identifier: MIT

package web

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

// authenticator is a passkey in software for tests: one key pair per credential, no attestation, and a counter. It
// answers the options the server sends as a browser and a security key would together.
type authenticator struct {
	t       *testing.T
	keys    map[string]*ecdsa.PrivateKey
	handles map[string][]byte
	counter uint32
	// synced keeps the counter at zero, as passkeys synced between devices do.
	synced bool
}

func newAuthenticator(t *testing.T) *authenticator {
	return &authenticator{t: t, keys: map[string]*ecdsa.PrivateKey{}, handles: map[string][]byte{}}
}

var b64 = base64.RawURLEncoding

type creationOptions struct {
	PublicKey struct {
		Challenge string `json:"challenge"`
		RP        struct {
			ID string `json:"id"`
		} `json:"rp"`
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	} `json:"publicKey"`
}

type requestOptions struct {
	PublicKey struct {
		Challenge string `json:"challenge"`
		RPID      string `json:"rpId"`
	} `json:"publicKey"`
}

func (a *authenticator) clientData(kind, challenge, origin string) []byte {
	raw, _ := json.Marshal(map[string]any{"type": kind, "challenge": challenge, "origin": origin, "crossOrigin": false})
	return raw
}

func flagsAndCount(rpID string, flags byte, counter uint32) []byte {
	hash := sha256.Sum256([]byte(rpID))
	out := append(hash[:], flags)
	return binary.BigEndian.AppendUint32(out, counter)
}

// create makes a new credential for the options, as navigator.credentials.create would, and returns its ID and the
// JSON the browser posts.
func (a *authenticator) create(options []byte, origin string) (string, string) {
	a.t.Helper()
	var o creationOptions
	if err := json.Unmarshal(options, &o); err != nil {
		a.t.Fatalf("%v: %s", err, options)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		a.t.Fatal(err)
	}
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	pub, err := key.PublicKey.Bytes() // 0x04 | x | y
	if err != nil {
		a.t.Fatal(err)
	}
	cose, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: pub[1:33], -3: pub[33:65]})
	if err != nil {
		a.t.Fatal(err)
	}
	authData := flagsAndCount(o.PublicKey.RP.ID, 0x45, a.counter) // user present, user verified, attested data
	authData = append(authData, make([]byte, 16)...)
	authData = binary.BigEndian.AppendUint16(authData, uint16(len(id)))
	authData = append(append(authData, id...), cose...)
	attestation, err := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": authData})
	if err != nil {
		a.t.Fatal(err)
	}
	credID := b64.EncodeToString(id)
	a.keys[credID] = key
	handle, err := b64.DecodeString(o.PublicKey.User.ID)
	if err != nil {
		a.t.Fatal(err)
	}
	a.handles[credID] = handle
	body, _ := json.Marshal(map[string]any{"id": credID, "rawId": credID, "type": "public-key", "response": map[string]any{
		"clientDataJSON":    b64.EncodeToString(a.clientData("webauthn.create", o.PublicKey.Challenge, origin)),
		"attestationObject": b64.EncodeToString(attestation),
	}})
	return credID, string(body)
}

// get signs the challenge of the options with a credential, as navigator.credentials.get would, and returns the JSON
// the browser posts. The counter goes up with every use unless a test sets it.
func (a *authenticator) get(options []byte, credID, origin string) string {
	a.t.Helper()
	var o requestOptions
	if err := json.Unmarshal(options, &o); err != nil {
		a.t.Fatalf("%v: %s", err, options)
	}
	key := a.keys[credID]
	if key == nil {
		a.t.Fatalf("no credential %s", credID)
	}
	if !a.synced {
		a.counter++
	}
	authData := flagsAndCount(o.PublicKey.RPID, 0x05, a.counter)
	client := a.clientData("webauthn.get", o.PublicKey.Challenge, origin)
	digest := sha256.Sum256(client)
	signed := sha256.Sum256(append(append([]byte{}, authData...), digest[:]...))
	signature, err := ecdsa.SignASN1(rand.Reader, key, signed[:])
	if err != nil {
		a.t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"id": credID, "rawId": credID, "type": "public-key", "response": map[string]any{
		"clientDataJSON":    b64.EncodeToString(client),
		"authenticatorData": b64.EncodeToString(authData),
		"signature":         b64.EncodeToString(signature),
		"userHandle":        b64.EncodeToString(a.handles[credID]),
	}})
	return string(body)
}
