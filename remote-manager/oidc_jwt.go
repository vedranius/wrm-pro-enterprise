// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ─── OIDC: discovery, keys and ID token verification ─
//
// Only what sign-in needs, with the standard library: the provider's discovery document, its
// signing keys (JWKS) and the verification of ID tokens signed with RS256/384/512, PS256/384/512
// or ES256/384/512. Unsigned tokens and shared-secret (HS*) signatures are refused.

type oidcDiscovery struct {
	Issuer                 string   `json:"issuer"`
	AuthorizationEndpoint  string   `json:"authorization_endpoint"`
	TokenEndpoint          string   `json:"token_endpoint"`
	JWKSURI                string   `json:"jwks_uri"`
	EndSessionEndpoint     string   `json:"end_session_endpoint"`
	ScopesSupported        []string `json:"scopes_supported"`
	ClaimsSupported        []string `json:"claims_supported"`
	IDTokenAlgs            []string `json:"id_token_signing_alg_values_supported"`
	TokenAuthMethods       []string `json:"token_endpoint_auth_methods_supported"`
	CodeChallengeMethods   []string `json:"code_challenge_methods_supported"`
	ResponseTypesSupported []string `json:"response_types_supported"`
}

type oidcJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

type oidcKey struct {
	kid string
	alg string
	pub crypto.PublicKey
}

// oidcHTTP talks to identity providers (tests replace it with a client that trusts their server).
var oidcHTTP = &http.Client{Timeout: 15 * time.Second}

var oidcCache = struct {
	sync.Mutex
	disc     map[string]oidcCachedDisc
	keys     map[string]oidcCachedKeys
	lastPull map[string]time.Time
}{disc: map[string]oidcCachedDisc{}, keys: map[string]oidcCachedKeys{}, lastPull: map[string]time.Time{}}

type oidcCachedDisc struct {
	d  *oidcDiscovery
	at time.Time
}

type oidcCachedKeys struct {
	keys []oidcKey
	at   time.Time
}

// oidcURLAllowed: identity provider addresses must use HTTPS (plain HTTP only on loopback, for a
// provider running on the WRM host itself).
func oidcURLAllowed(raw string) error {
	if err := guardEgressURL(raw); err != nil {
		return err
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%q is not a valid address", raw)
	}
	if u.Scheme == "https" {
		return nil
	}
	h := u.Hostname()
	if u.Scheme == "http" && (h == "localhost" || h == "127.0.0.1" || h == "::1") {
		return nil
	}
	return fmt.Errorf("%s must use https://", raw)
}

func oidcGetJSON(raw string, out interface{}) error {
	if err := oidcURLAllowed(raw); err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := oidcHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s answered HTTP %d", raw, resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s did not return JSON: %v", raw, err)
	}
	return nil
}

func oidcIssuerTrim(s string) string { return strings.TrimRight(strings.TrimSpace(s), "/") }

// oidcDiscover reads <issuer>/.well-known/openid-configuration (cached for an hour).
func oidcDiscover(issuer string, fresh bool) (*oidcDiscovery, error) {
	issuer = oidcIssuerTrim(issuer)
	oidcCache.Lock()
	c, ok := oidcCache.disc[issuer]
	oidcCache.Unlock()
	if ok && !fresh && time.Since(c.at) < time.Hour {
		return c.d, nil
	}
	var d oidcDiscovery
	if err := oidcGetJSON(issuer+"/.well-known/openid-configuration", &d); err != nil {
		return nil, err
	}
	if oidcIssuerTrim(d.Issuer) != issuer {
		return nil, fmt.Errorf("the provider calls itself %q, not %q: enter exactly the issuer it publishes", d.Issuer, issuer)
	}
	for name, v := range map[string]string{"authorization_endpoint": d.AuthorizationEndpoint, "token_endpoint": d.TokenEndpoint, "jwks_uri": d.JWKSURI} {
		if v == "" {
			return nil, fmt.Errorf("the discovery document has no %s", name)
		}
		if err := oidcURLAllowed(v); err != nil {
			return nil, err
		}
	}
	oidcCache.Lock()
	oidcCache.disc[issuer] = oidcCachedDisc{d: &d, at: time.Now()}
	oidcCache.Unlock()
	return &d, nil
}

func b64url(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

func (k oidcJWK) publicKey() (crypto.PublicKey, error) {
	switch k.Kty {
	case "RSA":
		n, err1 := b64url(k.N)
		e, err2 := b64url(k.E)
		if err1 != nil || err2 != nil || len(n) < 256 || len(e) == 0 || len(e) > 4 {
			return nil, errors.New("bad RSA key (keys under 2048 bits are refused)")
		}
		ei := 0
		for _, b := range e {
			ei = ei<<8 | int(b)
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: ei}, nil
	case "EC":
		var curve elliptic.Curve
		switch k.Crv {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		case "P-521":
			curve = elliptic.P521()
		default:
			return nil, fmt.Errorf("unsupported curve %q", k.Crv)
		}
		x, err1 := b64url(k.X)
		y, err2 := b64url(k.Y)
		if err1 != nil || err2 != nil {
			return nil, errors.New("bad EC key")
		}
		size := (curve.Params().BitSize + 7) / 8
		if len(x) > size || len(y) > size {
			return nil, errors.New("bad EC key")
		}
		point := make([]byte, 1+2*size)
		point[0] = 4
		copy(point[1+size-len(x):1+size], x)
		copy(point[1+2*size-len(y):], y)
		pub, err := ecdsa.ParseUncompressedPublicKey(curve, point)
		if err != nil {
			return nil, errors.New("EC key is not on its curve")
		}
		return pub, nil
	}
	return nil, fmt.Errorf("unsupported key type %q", k.Kty)
}

// oidcKeys returns the provider's signing keys; fresh pulls them again (at most once a minute),
// for a token signed with a key WRM has not seen yet (key rotation).
func oidcKeys(jwksURI string, fresh bool) ([]oidcKey, error) {
	oidcCache.Lock()
	c, ok := oidcCache.keys[jwksURI]
	recent := time.Since(oidcCache.lastPull[jwksURI]) < time.Minute
	oidcCache.Unlock()
	if ok && (!fresh || recent) && time.Since(c.at) < 6*time.Hour {
		return c.keys, nil
	}
	var set struct {
		Keys []oidcJWK `json:"keys"`
	}
	oidcCache.Lock()
	oidcCache.lastPull[jwksURI] = time.Now()
	oidcCache.Unlock()
	if err := oidcGetJSON(jwksURI, &set); err != nil {
		return nil, err
	}
	var keys []oidcKey
	for _, k := range set.Keys {
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		pub, err := k.publicKey()
		if err != nil {
			continue
		}
		keys = append(keys, oidcKey{kid: k.Kid, alg: k.Alg, pub: pub})
	}
	if len(keys) == 0 {
		return nil, errors.New("the provider publishes no usable signing keys")
	}
	oidcCache.Lock()
	oidcCache.keys[jwksURI] = oidcCachedKeys{keys: keys, at: time.Now()}
	oidcCache.Unlock()
	return keys, nil
}

func oidcHashFor(alg string) (crypto.Hash, func() hash.Hash, bool) {
	switch alg[2:] {
	case "256":
		return crypto.SHA256, sha256.New, true
	case "384":
		return crypto.SHA384, sha512.New384, true
	case "512":
		return crypto.SHA512, sha512.New, true
	}
	return 0, nil, false
}

func oidcVerifySig(alg string, pub crypto.PublicKey, signed, sig []byte) error {
	if len(alg) != 5 {
		return fmt.Errorf("unsupported signature algorithm %q", alg)
	}
	h, newHash, ok := oidcHashFor(alg)
	if !ok {
		return fmt.Errorf("unsupported signature algorithm %q", alg)
	}
	hh := newHash()
	hh.Write(signed)
	digest := hh.Sum(nil)
	switch alg[:2] {
	case "RS":
		k, ok := pub.(*rsa.PublicKey)
		if !ok {
			return errors.New("key type does not match the algorithm")
		}
		return rsa.VerifyPKCS1v15(k, h, digest, sig)
	case "PS":
		k, ok := pub.(*rsa.PublicKey)
		if !ok {
			return errors.New("key type does not match the algorithm")
		}
		return rsa.VerifyPSS(k, h, digest, sig, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash})
	case "ES":
		k, ok := pub.(*ecdsa.PublicKey)
		if !ok || len(sig)%2 != 0 {
			return errors.New("key type does not match the algorithm")
		}
		half := len(sig) / 2
		if !ecdsa.Verify(k, digest, new(big.Int).SetBytes(sig[:half]), new(big.Int).SetBytes(sig[half:])) {
			return errors.New("signature does not match")
		}
		return nil
	}
	return fmt.Errorf("unsupported signature algorithm %q", alg)
}

// oidcClaims are the ID token's claims as decoded JSON.
type oidcClaims map[string]interface{}

func (c oidcClaims) str(k string) string {
	if v, ok := c[k].(string); ok {
		return v
	}
	return ""
}

func (c oidcClaims) num(k string) int64 {
	if v, ok := c[k].(float64); ok {
		return int64(v)
	}
	return 0
}

// list returns a claim as a list of strings (a single string counts as one element).
func (c oidcClaims) list(k string) []string {
	switch v := c[k].(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, x := range v {
			switch y := x.(type) {
			case string:
				out = append(out, y)
			case float64:
				out = append(out, fmt.Sprint(y))
			}
		}
		return out
	}
	return nil
}

// oidcVerifyIDToken checks signature, issuer, audience, lifetime and (when given) the nonce.
func oidcVerifyIDToken(raw string, d *oidcDiscovery, clientID, nonce string) (oidcClaims, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, errors.New("the ID token is not a signed JWT")
	}
	hdrB, err := b64url(parts[0])
	if err != nil {
		return nil, errors.New("the ID token header is not valid")
	}
	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
		Typ string `json:"typ"`
	}
	if json.Unmarshal(hdrB, &hdr) != nil {
		return nil, errors.New("the ID token header is not valid")
	}
	if hdr.Alg == "" || hdr.Alg == "none" || strings.HasPrefix(hdr.Alg, "HS") {
		return nil, fmt.Errorf("ID tokens signed with %q are refused", hdr.Alg)
	}
	sig, err := b64url(parts[2])
	if err != nil {
		return nil, errors.New("the ID token signature is not valid")
	}
	signed := []byte(parts[0] + "." + parts[1])
	verify := func(keys []oidcKey) (bool, error) {
		var lastErr error
		found := false
		for _, k := range keys {
			if hdr.Kid != "" && k.kid != hdr.Kid {
				continue
			}
			if k.alg != "" && k.alg != hdr.Alg {
				continue
			}
			found = true
			if lastErr = oidcVerifySig(hdr.Alg, k.pub, signed, sig); lastErr == nil {
				return true, nil
			}
		}
		return found, lastErr
	}
	keys, err := oidcKeys(d.JWKSURI, false)
	if err != nil {
		return nil, err
	}
	found, verr := verify(keys)
	if !found {
		if keys, err = oidcKeys(d.JWKSURI, true); err != nil {
			return nil, err
		}
		found, verr = verify(keys)
	}
	if !found {
		return nil, errors.New("the ID token is signed with a key the provider does not publish")
	}
	if verr != nil {
		return nil, errors.New("the ID token signature does not match the provider's key")
	}
	payload, err := b64url(parts[1])
	if err != nil {
		return nil, errors.New("the ID token payload is not valid")
	}
	var c oidcClaims
	if json.Unmarshal(payload, &c) != nil {
		return nil, errors.New("the ID token payload is not valid")
	}
	if c.str("iss") != d.Issuer {
		return nil, fmt.Errorf("the ID token was issued by %q, expected %q", c.str("iss"), d.Issuer)
	}
	aud := c.list("aud")
	audOK := false
	for _, a := range aud {
		if a == clientID {
			audOK = true
		}
	}
	if !audOK {
		return nil, errors.New("the ID token is not meant for this WRM (audience does not match the client ID)")
	}
	if len(aud) > 1 && c.str("azp") != "" && c.str("azp") != clientID {
		return nil, errors.New("the ID token was requested by another client (azp)")
	}
	now := time.Now().Unix()
	const skew = 120
	if exp := c.num("exp"); exp == 0 || now > exp+skew {
		return nil, errors.New("the ID token has expired: check the clock of the WRM host")
	}
	if iat := c.num("iat"); iat > now+skew {
		return nil, errors.New("the ID token was issued in the future: check the clock of the WRM host")
	}
	if nbf := c.num("nbf"); nbf > now+skew {
		return nil, errors.New("the ID token is not valid yet: check the clock of the WRM host")
	}
	if nonce != "" && c.str("nonce") != nonce {
		return nil, errors.New("the ID token does not belong to this sign-in (nonce)")
	}
	if c.str("sub") == "" {
		return nil, errors.New("the ID token has no subject (sub)")
	}
	return c, nil
}
