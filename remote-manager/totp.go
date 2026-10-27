// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"rsc.io/qr"
)

// ─── TOTP (RFC 6238) ─────────────────────────────────
// Works with Google/Microsoft Authenticator, 1Password, Bitwarden, FreeOTP, …:
// SHA-1, 6 digits, 30-second steps, ±1 step tolerance, replay protection per user.

const totpPeriod = 30

func newTOTPSecret() string {
	b := make([]byte, 20)
	rand.Read(b)
	return strings.TrimRight(base32.StdEncoding.EncodeToString(b), "=")
}

func totpCodeAt(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := (uint32(sum[off])&0x7f)<<24 | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])
	return fmt.Sprintf("%06d", v%1000000)
}

func decodeTOTPSecret(s string) ([]byte, error) {
	s = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	if m := len(s) % 8; m != 0 {
		s += strings.Repeat("=", 8-m)
	}
	return base32.StdEncoding.DecodeString(s)
}

// totpVerify checks a code and returns the matched time step. Codes from steps <= lastStep
// are rejected so an intercepted code cannot be used twice.
func totpVerify(secretB32, code string, lastStep int64) (bool, int64) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 {
		return false, 0
	}
	secret, err := decodeTOTPSecret(secretB32)
	if err != nil {
		return false, 0
	}
	now := time.Now().Unix() / totpPeriod
	for _, d := range []int64{0, -1, 1} {
		step := now + d
		if step <= lastStep {
			continue
		}
		if hmac.Equal([]byte(totpCodeAt(secret, step)), []byte(code)) {
			return true, step
		}
	}
	return false, 0
}

func totpURI(username, secret string) string {
	issuer := "Web Remote Manager"
	label := url.PathEscape(issuer + ":" + username)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", "6")
	q.Set("period", "30")
	return "otpauth://totp/" + label + "?" + q.Encode()
}

func qrDataURL(text string) string {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return ""
	}
	code.Scale = 6
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(code.PNG())
}

// ─── RECOVERY CODES ──────────────────────────────────

func newRecoveryCodes() (plain []string, hashedJSON string) {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	hashes := []string{}
	for i := 0; i < 10; i++ {
		b := make([]byte, 10)
		rand.Read(b)
		var sb strings.Builder
		for j, x := range b {
			if j == 5 {
				sb.WriteByte('-')
			}
			sb.WriteByte(alphabet[int(x)%len(alphabet)])
		}
		c := sb.String()
		plain = append(plain, c)
		hashes = append(hashes, sha256Hex("wrm-recovery:"+c))
	}
	j, _ := json.Marshal(hashes)
	return plain, string(j)
}

// useRecoveryCode consumes a recovery code; it returns the remaining codes JSON.
func useRecoveryCode(storedJSON, code string) (bool, string) {
	code = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(code), " ", ""))
	if len(code) == 10 {
		code = code[:5] + "-" + code[5:]
	}
	var hashes []string
	if json.Unmarshal([]byte(storedJSON), &hashes) != nil {
		return false, storedJSON
	}
	h := sha256Hex("wrm-recovery:" + code)
	for i, x := range hashes {
		if hmac.Equal([]byte(x), []byte(h)) {
			hashes = append(hashes[:i], hashes[i+1:]...)
			j, _ := json.Marshal(hashes)
			return true, string(j)
		}
	}
	return false, storedJSON
}

func recoveryCodesLeft(storedJSON string) int {
	var hashes []string
	json.Unmarshal([]byte(storedJSON), &hashes)
	return len(hashes)
}
