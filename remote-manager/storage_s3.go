// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ─── S3 CLIENT ────────────────────────────────────────────────────────────────────────────
//
// A small SigV4 client for the subset WRM needs: list, get (with Range), head, put, copy and
// delete. Deliberately no SDK: the server speaks the same protocol as every provider and the
// dependency list stays as short as the rest of the project. See docs/rfcs/0008-storages-s3.md.

// rctx is the context for work that is not tied to one HTTP request (preflight, status).
func rctx() context.Context { return context.Background() }

type s3Config struct {
	Endpoint      string
	Region        string
	Bucket        string
	Prefix        string
	AccessKey     string
	SecretKey     string
	CAFingerprint string // hex SHA-256 of the leaf certificate; "" verifies the chain normally
	SSE           string // "" | AES256 | aws:kms
	PathStyle     bool
}

type s3Client struct {
	cfg  s3Config
	base *url.URL
	http *http.Client
}

type s3Entry struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	ETag         string `json:"etag"`
	LastModified string `json:"last_modified"`
	StorageClass string `json:"storage_class,omitempty"`
}

type s3List struct {
	Prefixes  []string  `json:"prefixes"`
	Objects   []s3Entry `json:"objects"`
	Next      string    `json:"next,omitempty"`
	Truncated bool      `json:"truncated"`
}

type s3ObjectMeta struct {
	Size         int64
	ETag         string
	ContentType  string
	LastModified string
}

func newS3Client(cfg s3Config) (*s3Client, error) {
	u, err := url.Parse(strings.TrimRight(cfg.Endpoint, "/"))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("the endpoint address is not valid")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.CAFingerprint != "" {
		pin := strings.ToLower(cfg.CAFingerprint)
		// The certificate is pinned by its SHA-256 digest instead of a public CA, which is the
		// only way to use a self-signed MinIO without trusting every certificate on the host.
		// VerifyConnection also runs for a resumed session, so the pin cannot be bypassed by
		// session resumption; tickets are disabled as well.
		tlsCfg.InsecureSkipVerify = true // #nosec G402 -- replaced by the fingerprint check below
		tlsCfg.SessionTicketsDisabled = true
		tlsCfg.VerifyConnection = func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return fmt.Errorf("the server presented no certificate")
			}
			sum := sha256.Sum256(state.PeerCertificates[0].Raw)
			if hex.EncodeToString(sum[:]) != pin {
				return fmt.Errorf("the certificate fingerprint does not match the pinned one")
			}
			return nil
		}
	}
	tr := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSClientConfig:       tlsCfg,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       60 * time.Second,
		MaxIdleConns:          8,
	}
	return &s3Client{cfg: cfg, base: u, http: &http.Client{
		Transport: tr,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // a redirect to another host would leak the signature
		},
	}}, nil
}

// host is the Host header: the bucket is in front of the endpoint with virtual-host style.
func (c *s3Client) host() string {
	if c.cfg.PathStyle {
		return c.base.Host
	}
	return c.cfg.Bucket + "." + c.base.Host
}

// objectPath returns the decoded and the escaped path of an object (or of the bucket when key
// is empty). RawPath is set explicitly so the signed bytes are exactly the escaped ones, even
// for keys with spaces, '+' or other characters Go would encode differently.
func (c *s3Client) objectPath(key string) (string, string) {
	decoded := strings.TrimRight(c.base.Path, "/")
	escaped := decoded
	if c.cfg.PathStyle {
		decoded += "/" + c.cfg.Bucket
		escaped += "/" + c.cfg.Bucket
	}
	if key != "" {
		full := c.fullKey(key)
		decoded += "/" + full
		escaped += "/" + s3EscapePath(full)
	}
	if decoded == "" {
		decoded, escaped = "/", "/"
	}
	return decoded, escaped
}

// s3EscapePath encodes each path segment the way SigV4 expects, keeping the slashes.
func s3EscapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, part := range parts {
		parts[i] = s3Escape(part)
	}
	return strings.Join(parts, "/")
}

// s3Escape is RFC 3986 encoding with the AWS quirks (no '+' for spaces, '~' unescaped).
func s3Escape(s string) string { return awsURIEncode(s) }

func (c *s3Client) sign(req *http.Request, payload []byte, payloadHash string) {
	now := time.Now().UTC()
	amzDate, date := now.Format("20060102T150405Z"), now.Format("20060102")
	if payloadHash == "" {
		payloadHash = sha256Hex(string(payload))
	}
	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	if c.cfg.SSE != "" && req.Method == http.MethodPut {
		req.Header.Set("x-amz-server-side-encryption", c.cfg.SSE)
	}

	canonical := req.Method + "\n" + req.URL.EscapedPath() + "\n" + canonicalQuery(req.URL.Query()) + "\n"
	signed := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	headers := map[string]string{
		"host":                 c.host(),
		"x-amz-content-sha256": payloadHash,
		"x-amz-date":           amzDate,
	}
	for _, h := range signed {
		canonical += h + ":" + headers[h] + "\n"
	}
	canonical += "\n" + strings.Join(signed, ";") + "\n" + payloadHash

	scope := date + "/" + c.cfg.Region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + sha256Hex(canonical)
	key := hmacSHA256([]byte("AWS4"+c.cfg.SecretKey), date)
	key = hmacSHA256(key, c.cfg.Region)
	key = hmacSHA256(key, "s3")
	key = hmacSHA256(key, "aws4_request")
	sig := hex.EncodeToString(hmacSHA256(key, stringToSign))

	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+c.cfg.AccessKey+"/"+scope+
		", SignedHeaders="+strings.Join(signed, ";")+", Signature="+sig)
	req.Host = c.host()
}

func canonicalQuery(v url.Values) string {
	if len(v) == 0 {
		return ""
	}
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		vals := append([]string(nil), v[k]...)
		sort.Strings(vals)
		for _, val := range vals {
			parts = append(parts, s3Escape(k)+"="+s3Escape(val))
		}
	}
	return strings.Join(parts, "&")
}

// s3Error is a provider error with the code kept for the caller.
type s3Error struct {
	Code    string
	Message string
	Status  int
}

func (e *s3Error) Error() string { return s3ErrorMessage(e) }

func s3ErrorMessage(e *s3Error) string {
	switch e.Code {
	case "NoSuchBucket":
		return "The bucket does not exist (check the name and the region)"
	case "AccessDenied":
		return "Access denied: the key may not allow this operation on the bucket"
	case "InvalidAccessKeyId":
		return "The access key is not valid for this endpoint"
	case "SignatureDoesNotMatch":
		return "The secret key or the region does not match the endpoint"
	case "NoSuchKey":
		return "The object does not exist any more"
	case "PreconditionFailed":
		return "The object changed in the meantime; reload it and try again"
	case "BucketRegionError", "AuthorizationHeaderMalformed":
		return "The endpoint is in another region; check the region field"
	}
	if e.Message != "" {
		return "The storage answered: " + e.Message
	}
	return "The storage answered with HTTP " + strconv.Itoa(e.Status)
}

// do signs and sends one request, with two retries for transient answers.
func (c *s3Client) do(ctx context.Context, method, key string, q url.Values, body []byte, hdr http.Header) (*http.Response, error) {
	if ctx == nil {
		ctx = rctx()
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 200 * time.Millisecond):
			}
		}
		u := *c.base
		u.Path, u.RawPath = c.objectPath(key)
		u.RawQuery = ""
		if q != nil {
			u.RawQuery = q.Encode()
		}
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		// #nosec G704 -- the endpoint is entered by an administrator and checked by guardEgressURL (metadata, link-local, egress policy)
		req, err := http.NewRequestWithContext(ctx, method, u.String(), rdr)
		if err != nil {
			return nil, err
		}
		if body != nil {
			req.ContentLength = int64(len(body))
		}
		for k, vs := range hdr {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
		c.sign(req, body, "")
		// #nosec G704 -- same endpoint, already validated when the storage was saved
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("cannot reach the storage: %w", err)
			if ctx.Err() != nil {
				return nil, lastErr
			}
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp, nil
		}
		e := readS3Error(resp)
		if resp.StatusCode >= 500 && resp.StatusCode != http.StatusNotImplemented {
			lastErr = e
			continue
		}
		return nil, e
	}
	return nil, lastErr
}

func readS3Error(resp *http.Response) *s3Error {
	e := &s3Error{Status: resp.StatusCode}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	resp.Body.Close()
	var parsed struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	if xml.Unmarshal(raw, &parsed) == nil && parsed.Code != "" {
		e.Code, e.Message = parsed.Code, parsed.Message
	} else {
		e.Message = strings.TrimSpace(string(raw))
	}
	if e.Code == "" {
		switch resp.StatusCode {
		case http.StatusForbidden:
			e.Code = "AccessDenied"
		case http.StatusNotFound:
			e.Code = "NoSuchKey"
		case http.StatusPreconditionFailed:
			e.Code = "PreconditionFailed"
		}
	}
	return e
}

// headBucket checks that the bucket exists and the key may read it.
func (c *s3Client) headBucket(ctx context.Context) error {
	resp, err := c.do(ctx, http.MethodHead, "", nil, nil, nil)
	if err != nil {
		if e, ok := err.(*s3Error); ok && e.Code == "NoSuchKey" {
			return &s3Error{Code: "NoSuchBucket", Status: e.Status}
		}
		return err
	}
	resp.Body.Close()
	return nil
}

type listBucketResult struct {
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
	Contents              []struct {
		Key          string `xml:"Key"`
		LastModified string `xml:"LastModified"`
		ETag         string `xml:"ETag"`
		Size         int64  `xml:"Size"`
		StorageClass string `xml:"StorageClass"`
	} `xml:"Contents"`
	CommonPrefixes []struct {
		Prefix string `xml:"Prefix"`
	} `xml:"CommonPrefixes"`
}

// list returns one page of a prefix listing; relPrefix is relative to the storage prefix.
func (c *s3Client) list(ctx context.Context, relPrefix, cursor string, max int) (s3List, error) {
	q := url.Values{"list-type": []string{"2"}, "delimiter": []string{"/"}}
	if p := c.fullKey(relPrefix); p != "" {
		q.Set("prefix", p)
	}
	if cursor != "" {
		q.Set("continuation-token", cursor)
	}
	if max > 0 {
		q.Set("max-keys", strconv.Itoa(max))
	}
	resp, err := c.do(ctx, http.MethodGet, "", q, nil, nil)
	if err != nil {
		return s3List{}, err
	}
	defer resp.Body.Close()
	var parsed listBucketResult
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&parsed); err != nil {
		return s3List{}, fmt.Errorf("the listing could not be read: %w", err)
	}
	out := s3List{Next: parsed.NextContinuationToken, Truncated: parsed.IsTruncated, Prefixes: []string{}, Objects: []s3Entry{}}
	base := storageKeyPrefix(Storage{Prefix: c.cfg.Prefix})
	for _, p := range parsed.CommonPrefixes {
		out.Prefixes = append(out.Prefixes, strings.TrimPrefix(p.Prefix, base))
	}
	for _, o := range parsed.Contents {
		key := strings.TrimPrefix(o.Key, base)
		if key == "" {
			continue // the folder marker itself
		}
		out.Objects = append(out.Objects, s3Entry{
			Key: key, Size: o.Size, ETag: strings.Trim(o.ETag, `"`),
			LastModified: o.LastModified, StorageClass: o.StorageClass,
		})
	}
	return out, nil
}

// fullKey places a key inside the storage prefix; every request goes through it, so a storage
// configured as "team-a/" can never address anything else in the bucket.
func (c *s3Client) fullKey(key string) string {
	return storageKeyPrefix(Storage{Prefix: c.cfg.Prefix}) + key
}

// get streams an object; rangeHeader (e.g. "bytes=0-1023") makes it a partial read.
func (c *s3Client) get(ctx context.Context, key, rangeHeader string) (*http.Response, error) {
	h := http.Header{}
	if rangeHeader != "" {
		h.Set("Range", rangeHeader)
	}
	return c.do(ctx, http.MethodGet, key, nil, nil, h)
}

func (c *s3Client) head(ctx context.Context, key string) (s3ObjectMeta, error) {
	resp, err := c.do(ctx, http.MethodHead, key, nil, nil, nil)
	if err != nil {
		return s3ObjectMeta{}, err
	}
	resp.Body.Close()
	size, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	return s3ObjectMeta{
		Size: size, ETag: strings.Trim(resp.Header.Get("ETag"), `"`),
		ContentType: resp.Header.Get("Content-Type"), LastModified: resp.Header.Get("Last-Modified"),
	}, nil
}

// put stores one object. ifMatch makes it a conditional write ("" = unconditional).
func (c *s3Client) put(ctx context.Context, key string, body []byte, contentType, ifMatch string) (string, error) {
	h := http.Header{}
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	if ifMatch != "" {
		h.Set("If-Match", `"`+strings.Trim(ifMatch, `"`)+`"`)
	}
	resp, err := c.do(ctx, http.MethodPut, key, nil, body, h)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	return strings.Trim(resp.Header.Get("ETag"), `"`), nil
}

// copy copies one object inside the same bucket.
func (c *s3Client) copy(ctx context.Context, from, to string) error {
	h := http.Header{}
	h.Set("x-amz-copy-source", "/"+c.cfg.Bucket+"/"+s3EscapePath(c.fullKey(from)))
	resp, err := c.do(ctx, http.MethodPut, to, nil, nil, h)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// S3 reports a failed copy inside a 200 answer.
	var parsed struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	if xml.Unmarshal(raw, &parsed) == nil && parsed.Code != "" {
		return &s3Error{Code: parsed.Code, Message: parsed.Message, Status: resp.StatusCode}
	}
	return nil
}

func (c *s3Client) delete(ctx context.Context, key string) error {
	resp, err := c.do(ctx, http.MethodDelete, key, nil, nil, nil)
	if err != nil {
		if e, ok := err.(*s3Error); ok && e.Code == "NoSuchKey" {
			return nil // already gone
		}
		return err
	}
	resp.Body.Close()
	return nil
}
