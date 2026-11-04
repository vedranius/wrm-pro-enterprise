// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// ─── S3 stub ──────────────────────────────────────────────────────────────────────────────
//
// A tiny S3 endpoint that verifies the SigV4 signature with its own canonicalisation, so a
// broken signer (wrong encoding, wrong header set, wrong payload hash) fails the test instead
// of "working" against itself.

type s3Stub struct {
	t         *testing.T
	mu        sync.Mutex
	objects   map[string][]byte
	etags     map[string]string
	bucket    string
	accessKey string
	secret    string
	region    string
	lastError string
}

func newS3Stub(t *testing.T, bucket, accessKey, secret string) *s3Stub {
	return &s3Stub{t: t, objects: map[string][]byte{}, etags: map[string]string{},
		bucket: bucket, accessKey: accessKey, secret: secret, region: "us-east-1"}
}

func (s *s3Stub) put(key string, body []byte) string {
	sum := sha256.Sum256(body)
	etag := hex.EncodeToString(sum[:8])
	s.mu.Lock()
	s.objects[key] = append([]byte(nil), body...)
	s.etags[key] = etag
	s.mu.Unlock()
	return etag
}

func (s *s3Stub) fail(w http.ResponseWriter, status int, code, msg string) {
	body, _ := xml.Marshal(struct {
		XMLName xml.Name `xml:"Error"`
		Code    string   `xml:"Code"`
		Message string   `xml:"Message"`
	}{Code: code, Message: msg})
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	w.Write(body)
}

// canonicalQueryFor rebuilds the canonical query string the way SigV4 defines it.
func canonicalQueryFor(v url.Values) string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []string
	for _, k := range keys {
		vals := append([]string(nil), v[k]...)
		sort.Strings(vals)
		for _, val := range vals {
			out = append(out, awsURIEncode(k)+"="+awsURIEncode(val))
		}
	}
	return strings.Join(out, "&")
}

// verifySignature checks the Authorization header of the request against its own canonical
// request. It deliberately does not reuse s3Client.sign.
func (s *s3Stub) verifySignature(r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	const prefix = "AWS4-HMAC-SHA256 "
	if !strings.HasPrefix(auth, prefix) {
		s.lastError = "missing Authorization"
		return false
	}
	fields := map[string]string{}
	for _, part := range strings.Split(strings.TrimPrefix(auth, prefix), ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 {
			fields[kv[0]] = kv[1]
		}
	}
	cred := strings.Split(fields["Credential"], "/")
	if len(cred) != 5 || cred[0] != s.accessKey {
		s.lastError = "bad credential " + fields["Credential"]
		return false
	}
	date, region, service, term := cred[1], cred[2], cred[3], cred[4]
	if region != s.region || service != "s3" || term != "aws4_request" {
		s.lastError = "bad scope " + fields["Credential"]
		return false
	}
	payloadHash := r.Header.Get("X-Amz-Content-Sha256")
	if payloadHash == "" {
		s.lastError = "missing x-amz-content-sha256"
		return false
	}
	signed := strings.Split(fields["SignedHeaders"], ";")
	var canon strings.Builder
	canon.WriteString(r.Method + "\n" + r.URL.EscapedPath() + "\n" + canonicalQueryFor(r.URL.Query()) + "\n")
	for _, h := range signed {
		v := r.Header.Get(h)
		if h == "host" {
			v = r.Host
		}
		canon.WriteString(strings.ToLower(h) + ":" + strings.TrimSpace(v) + "\n")
	}
	canon.WriteString("\n" + fields["SignedHeaders"] + "\n" + payloadHash)
	scope := date + "/" + region + "/" + service + "/" + term
	stringToSign := "AWS4-HMAC-SHA256\n" + r.Header.Get("X-Amz-Date") + "\n" + scope + "\n" + sha256Hex(canon.String())
	key := hmacSHA256([]byte("AWS4"+s.secret), date)
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, service)
	key = hmacSHA256(key, term)
	want := hex.EncodeToString(hmacSHA256(key, stringToSign))
	if want != fields["Signature"] {
		s.lastError = "signature mismatch"
		return false
	}
	return true
}

func (s *s3Stub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.verifySignature(r) {
		s.fail(w, http.StatusForbidden, "SignatureDoesNotMatch", s.lastError)
		return
	}
	path := strings.TrimPrefix(r.URL.EscapedPath(), "/")
	bucket, key := path, ""
	if i := strings.Index(path, "/"); i >= 0 {
		bucket, key = path[:i], path[i+1:]
	}
	if bucket != s.bucket {
		s.fail(w, http.StatusNotFound, "NoSuchBucket", "no such bucket")
		return
	}
	if key != "" {
		if k, err := url.PathUnescape(key); err == nil {
			key = k
		}
	}
	switch {
	case key == "" && r.Method == http.MethodHead:
		w.WriteHeader(http.StatusOK)
	case key == "" && r.Method == http.MethodGet:
		s.list(w, r)
	case key == "" && r.Method == http.MethodPut:
		// create bucket: not needed
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodPut && r.Header.Get("X-Amz-Copy-Source") != "":
		s.copyObject(w, r, key)
	case r.Method == http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		if want := strings.Trim(r.Header.Get("If-Match"), `"`); want != "" && s.etags[key] != want {
			s.mu.Unlock()
			s.fail(w, http.StatusPreconditionFailed, "PreconditionFailed", "the object changed")
			return
		}
		etag := s.putLocked(key, body)
		s.mu.Unlock()
		w.Header().Set("ETag", `"`+etag+`"`)
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet || r.Method == http.MethodHead:
		s.mu.Lock()
		body, ok := s.objects[key]
		etag := s.etags[key]
		s.mu.Unlock()
		if !ok {
			s.fail(w, http.StatusNotFound, "NoSuchKey", "no such key")
			return
		}
		w.Header().Set("ETag", `"`+etag+`"`)
		w.Header().Set("Content-Type", storageContentType("", key))
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.WriteHeader(http.StatusOK)
			return
		}
		if rh := r.Header.Get("Range"); rh != "" {
			start, end, ok := parseTestRange(rh, len(body))
			if !ok {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
			w.Header().Set("Content-Length", strconv.Itoa(end-start+1))
			w.WriteHeader(http.StatusPartialContent)
			w.Write(body[start : end+1])
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Write(body)
	case r.Method == http.MethodDelete:
		s.mu.Lock()
		delete(s.objects, key)
		delete(s.etags, key)
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		s.fail(w, http.StatusMethodNotAllowed, "MethodNotAllowed", r.Method)
	}
}

func (s *s3Stub) putLocked(key string, body []byte) string {
	sum := sha256.Sum256(body)
	etag := hex.EncodeToString(sum[:8])
	s.objects[key] = append([]byte(nil), body...)
	s.etags[key] = etag
	return etag
}

func (s *s3Stub) copyObject(w http.ResponseWriter, r *http.Request, to string) {
	src := strings.TrimPrefix(r.Header.Get("X-Amz-Copy-Source"), "/")
	if i := strings.Index(src, "/"); i >= 0 {
		src = src[i+1:]
	}
	if k, err := url.PathUnescape(src); err == nil {
		src = k
	}
	s.mu.Lock()
	body, ok := s.objects[src]
	if ok {
		s.putLocked(to, body)
	}
	s.mu.Unlock()
	if !ok {
		s.fail(w, http.StatusNotFound, "NoSuchKey", "no such key")
		return
	}
	w.Write([]byte(`<CopyObjectResult><ETag>"x"</ETag></CopyObjectResult>`))
}

func (s *s3Stub) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	prefix := q.Get("prefix")
	delimiter := q.Get("delimiter")
	s.mu.Lock()
	keys := make([]string, 0, len(s.objects))
	for k := range s.objects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	seen := map[string]bool{}
	type content struct {
		Key  string `xml:"Key"`
		Size int    `xml:"Size"`
		ETag string `xml:"ETag"`
	}
	var contents []content
	var prefixes []string
	for _, k := range keys {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		rest := strings.TrimPrefix(k, prefix)
		if delimiter != "" {
			if i := strings.Index(rest, delimiter); i >= 0 {
				p := prefix + rest[:i+1]
				if !seen[p] {
					seen[p] = true
					prefixes = append(prefixes, p)
				}
				continue
			}
		}
		contents = append(contents, content{Key: k, Size: len(s.objects[k]), ETag: `"` + s.etags[k] + `"`})
	}
	s.mu.Unlock()
	type result struct {
		XMLName        xml.Name `xml:"ListBucketResult"`
		Name           string   `xml:"Name"`
		Prefix         string   `xml:"Prefix"`
		IsTruncated    bool     `xml:"IsTruncated"`
		Contents       []content
		CommonPrefixes []struct {
			Prefix string `xml:"Prefix"`
		} `xml:"CommonPrefixes"`
	}
	out := result{Name: s.bucket, Prefix: prefix, Contents: contents}
	for _, p := range prefixes {
		out.CommonPrefixes = append(out.CommonPrefixes, struct {
			Prefix string `xml:"Prefix"`
		}{Prefix: p})
	}
	body, _ := xml.Marshal(out)
	w.Header().Set("Content-Type", "application/xml")
	w.Write(body)
}

func parseTestRange(h string, size int) (int, int, bool) {
	h = strings.TrimPrefix(h, "bytes=")
	parts := strings.SplitN(h, "-", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	end := size - 1
	if parts[1] != "" {
		if v, err := strconv.Atoi(parts[1]); err == nil {
			end = v
		}
	}
	if start < 0 || start >= size || end >= size || end < start {
		return 0, 0, false
	}
	return start, end, true
}
