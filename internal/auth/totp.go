// SPDX-License-Identifier: MIT

package auth

import (
	"bytes"
	"crypto/subtle"
	"encoding/base64"
	"image/png"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// TOTP after RFC 6238 as every authenticator app knows it: SHA-1, six digits, 30 seconds.
var totpOptions = totp.ValidateOpts{Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}

// NewTOTP is the setup of an authenticator app: the secret to keep, and the link and picture to scan.
type NewTOTP struct {
	// Secret is base32, as apps take it when typed in.
	Secret string
	URL    string
	// QR is the link as a PNG picture in a data: URL, which the content security policy allows.
	QR string
}

// GenerateTOTP makes a new secret for an account of an issuer, such as the name of the tenant.
func GenerateTOTP(issuer, accountName string) (NewTOTP, error) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: issuer, AccountName: accountName, SecretSize: 20})
	if err != nil {
		return NewTOTP{}, err
	}
	picture, err := key.Image(240, 240)
	if err != nil {
		return NewTOTP{}, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, picture); err != nil {
		return NewTOTP{}, err
	}
	return NewTOTP{Secret: key.Secret(), URL: key.URL(), QR: "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())}, nil
}

// CheckTOTP tells whether code is valid for the secret at now, one step of 30 seconds early or late allowed, and only
// for a step after last: a code that was used once does not work again. It returns the step to keep as the new last.
func CheckTOTP(secret, code string, now time.Time, last int64) (step int64, ok bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 {
		return last, false
	}
	current := now.Unix() / 30
	for _, s := range []int64{current - 1, current, current + 1} {
		if s <= last {
			continue
		}
		want, err := totp.GenerateCodeCustom(secret, time.Unix(s*30, 0), totpOptions)
		if err != nil {
			return last, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return s, true
		}
	}
	return last, false
}
