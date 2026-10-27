// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// ─── AI: CLOUD AUTHENTICATION ────────────────────────
//
// AWS Signature Version 4 for Bedrock, the AWS event stream framing of ConverseStream,
// and Google service account tokens (JWT bearer grant) for Vertex AI. Small and
// dependency-free, like the rest of WRM.

// awsURIEncode encodes like AWS expects: every byte except A-Z a-z 0-9 - _ . ~.
func awsURIEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func hmacSHA256(key []byte, s string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(s))
	return h.Sum(nil)
}

// sigV4Sign signs a request (header authorization). The canonical URI encodes every path
// segment of the already escaped path again, as AWS requires for services other than S3.
func sigV4Sign(r *http.Request, body []byte, accessKey, secretKey, sessionToken, region, service string, now time.Time) error {
	if accessKey == "" || secretKey == "" {
		return errors.New("AWS credentials are missing")
	}
	amzDate := now.Format("20060102T150405Z")
	day := now.Format("20060102")
	sum := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(sum[:])
	r.Header.Set("X-Amz-Date", amzDate)
	if sessionToken != "" {
		r.Header.Set("X-Amz-Security-Token", sessionToken)
	}
	host := r.URL.Host
	r.Host = host
	var segs []string
	for _, s := range strings.Split(r.URL.EscapedPath(), "/") {
		segs = append(segs, awsURIEncode(s))
	}
	canonURI := strings.Join(segs, "/")
	if canonURI == "" {
		canonURI = "/"
	}
	// canonical query: sorted, encoded
	q := r.URL.Query()
	var qk []string
	for k := range q {
		qk = append(qk, k)
	}
	sort.Strings(qk)
	var qs []string
	for _, k := range qk {
		vals := append([]string{}, q[k]...)
		sort.Strings(vals)
		for _, v := range vals {
			qs = append(qs, awsURIEncode(k)+"="+awsURIEncode(v))
		}
	}
	headers := map[string]string{"host": host, "x-amz-date": amzDate}
	if ct := r.Header.Get("Content-Type"); ct != "" {
		headers["content-type"] = ct
	}
	if sessionToken != "" {
		headers["x-amz-security-token"] = sessionToken
	}
	var hk []string
	for k := range headers {
		hk = append(hk, k)
	}
	sort.Strings(hk)
	var ch strings.Builder
	for _, k := range hk {
		ch.WriteString(k + ":" + strings.TrimSpace(headers[k]) + "\n")
	}
	signed := strings.Join(hk, ";")
	canon := strings.Join([]string{r.Method, canonURI, strings.Join(qs, "&"), ch.String(), signed, payloadHash}, "\n")
	scope := day + "/" + region + "/" + service + "/aws4_request"
	cs := sha256.Sum256([]byte(canon))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(cs[:])
	k := hmacSHA256([]byte("AWS4"+secretKey), day)
	k = hmacSHA256(k, region)
	k = hmacSHA256(k, service)
	k = hmacSHA256(k, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(k, toSign))
	r.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+", SignedHeaders="+signed+", Signature="+sig)
	return nil
}

// readEventStream decodes application/vnd.amazon.eventstream messages:
// prelude (total length, headers length, CRC), headers, payload, message CRC.
func readEventStream(r io.Reader, fn func(msgType, eventType string, payload []byte) error) error {
	prelude := make([]byte, 12)
	for {
		if _, err := io.ReadFull(r, prelude); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		total := binary.BigEndian.Uint32(prelude[0:4])
		hlen := binary.BigEndian.Uint32(prelude[4:8])
		if crc32.ChecksumIEEE(prelude[0:8]) != binary.BigEndian.Uint32(prelude[8:12]) {
			return errors.New("event stream: prelude checksum mismatch")
		}
		if total < 16 || total > 16<<20 || hlen > total-16 {
			return errors.New("event stream: bad message length")
		}
		rest := make([]byte, total-12)
		if _, err := io.ReadFull(r, rest); err != nil {
			return err
		}
		crc := crc32.NewIEEE()
		crc.Write(prelude)
		crc.Write(rest[:len(rest)-4])
		if crc.Sum32() != binary.BigEndian.Uint32(rest[len(rest)-4:]) {
			return errors.New("event stream: message checksum mismatch")
		}
		hdrs := map[string]string{}
		h := rest[:hlen]
		for len(h) > 0 {
			nl := int(h[0])
			if len(h) < 1+nl+1 {
				return errors.New("event stream: bad header")
			}
			name := string(h[1 : 1+nl])
			typ := h[1+nl]
			h = h[2+nl:]
			var size int
			switch typ {
			case 0, 1:
				size = 0
			case 2:
				size = 1
			case 3:
				size = 2
			case 4:
				size = 4
			case 5, 8:
				size = 8
			case 9:
				size = 16
			case 6, 7:
				if len(h) < 2 {
					return errors.New("event stream: bad header")
				}
				size = int(binary.BigEndian.Uint16(h[:2]))
				h = h[2:]
			default:
				return errors.New("event stream: unknown header type")
			}
			if len(h) < size {
				return errors.New("event stream: bad header")
			}
			if typ == 7 {
				hdrs[name] = string(h[:size])
			}
			h = h[size:]
		}
		payload := rest[hlen : len(rest)-4]
		evt := hdrs[":event-type"]
		if hdrs[":message-type"] == "exception" {
			evt = hdrs[":exception-type"]
		}
		if err := fn(hdrs[":message-type"], evt, payload); err != nil {
			return err
		}
	}
}

// encodeEventStreamMessage builds one event stream message (used by the tests' fake Bedrock).
func encodeEventStreamMessage(headers map[string]string, payload []byte) []byte {
	var hb []byte
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := headers[k]
		hb = append(hb, byte(len(k)))
		hb = append(hb, k...)
		hb = append(hb, 7)
		hb = binary.BigEndian.AppendUint16(hb, uint16(len(v)))
		hb = append(hb, v...)
	}
	total := uint32(12 + len(hb) + len(payload) + 4)
	msg := binary.BigEndian.AppendUint32(nil, total)
	msg = binary.BigEndian.AppendUint32(msg, uint32(len(hb)))
	msg = binary.BigEndian.AppendUint32(msg, crc32.ChecksumIEEE(msg[:8]))
	msg = append(msg, hb...)
	msg = append(msg, payload...)
	return binary.BigEndian.AppendUint32(msg, crc32.ChecksumIEEE(msg))
}

// ─── Google service accounts ─────────────────────────

type googleServiceAccount struct {
	Type        string `json:"type"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
	key         *rsa.PrivateKey
}

func parseServiceAccount(s string) (*googleServiceAccount, error) {
	var sa googleServiceAccount
	if err := json.Unmarshal([]byte(s), &sa); err != nil {
		return nil, fmt.Errorf("the service account key is not valid JSON")
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, fmt.Errorf("the service account key needs client_email and private_key")
	}
	if sa.TokenURI == "" {
		sa.TokenURI = "https://oauth2.googleapis.com/token"
	}
	if u, err := url.Parse(sa.TokenURI); err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("the service account token_uri is not a URL")
	}
	blk, _ := pem.Decode([]byte(sa.PrivateKey))
	if blk == nil {
		return nil, fmt.Errorf("the service account private key is not PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		if k2, err2 := x509.ParsePKCS1PrivateKey(blk.Bytes); err2 == nil {
			k = k2
		} else {
			return nil, fmt.Errorf("the service account private key cannot be read")
		}
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("the service account private key is not RSA")
	}
	sa.key = rk
	return &sa, nil
}

var vertexTokens = struct {
	sync.Mutex
	m map[string]struct {
		tok string
		exp time.Time
	}
}{m: map[string]struct {
	tok string
	exp time.Time
}{}}

// vertexToken returns a cached OAuth access token for the provider's service account.
func vertexToken(ctx context.Context, p aiProvider) (string, error) {
	sa, err := parseServiceAccount(p.secret.ServiceAccount)
	if err != nil {
		return "", err
	}
	key := sha256Hex(p.secret.ServiceAccount)
	vertexTokens.Lock()
	if t, ok := vertexTokens.m[key]; ok && time.Until(t.exp) > time.Minute {
		vertexTokens.Unlock()
		return t.tok, nil
	}
	vertexTokens.Unlock()
	now := time.Now()
	enc := base64.RawURLEncoding
	hdr := enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]interface{}{
		"iss": sa.ClientEmail, "scope": "https://www.googleapis.com/auth/cloud-platform", "aud": sa.TokenURI,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	unsigned := hdr + "." + enc.EncodeToString(claims)
	h := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, sa.key, crypto.SHA256, h[:])
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {unsigned + "." + enc.EncodeToString(sig)}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, sa.TokenURI, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := aiHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot get a Google access token: %v", err)
	}
	defer resp.Body.Close()
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		Desc        string `json:"error_description"`
	}
	json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&tr)
	if resp.StatusCode != 200 || tr.AccessToken == "" {
		return "", fmt.Errorf("Google refused the service account (HTTP %d): %s %s", resp.StatusCode, tr.Error, truncateStr(tr.Desc, 200))
	}
	if tr.ExpiresIn <= 0 {
		tr.ExpiresIn = 3600
	}
	vertexTokens.Lock()
	vertexTokens.m[key] = struct {
		tok string
		exp time.Time
	}{tr.AccessToken, now.Add(time.Duration(tr.ExpiresIn) * time.Second)}
	vertexTokens.Unlock()
	return tr.AccessToken, nil
}
