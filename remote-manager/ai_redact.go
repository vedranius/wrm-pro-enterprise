// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"regexp"
	"strings"
)

// ─── AI: REDACTION ───────────────────────────────────
//
// redactSecrets removes secrets from text before it leaves WRM (to a model provider, when
// the ai_redact_output policy is on) and always before it is stored in the AI transcript
// and the audit log. It uses the key names of the audit redaction (password, secret,
// token, credential, private key, passphrase …) for key = value / key: value pairs, plus
// the common formats of private keys, cloud and SaaS tokens, JWTs, Authorization headers,
// passwords in URLs and password hashes from shadow files.

const aiRedacted = "[REDACTED]"

var aiSecretKeyWords = append(append([]string{}, secretKeys...),
	"api_key", "apikey", "api-key", "access_key", "accesskey", "secret_key", "client_secret", "auth_token", "access_token",
	"refresh_token", "session_token", "private-key", "pass", "pwd_hash", "sas", "connectionstring", "account_key", "accountkey")

var aiRedactRules = func() []struct {
	re   *regexp.Regexp
	repl string
} {
	keys := []string{}
	for _, k := range aiSecretKeyWords {
		keys = append(keys, regexp.QuoteMeta(k))
	}
	keyRe := `(?i)\b((?:[A-Za-z0-9_.-]*[_.-])?(?:` + strings.Join(keys, "|") + `)(?:[_.-][A-Za-z0-9_.-]*)?)`
	return []struct {
		re   *regexp.Regexp
		repl string
	}{
		// PEM / OpenSSH private keys, also cut off at the end of the output
		{regexp.MustCompile(`-----BEGIN ([A-Z0-9 ]*)PRIVATE KEY( BLOCK)?-----[\s\S]*?(-----END ([A-Z0-9 ]*)PRIVATE KEY( BLOCK)?-----|$)`), "-----BEGIN ${1}PRIVATE KEY----- " + aiRedacted + " -----END ${1}PRIVATE KEY-----"},
		{regexp.MustCompile(`(?s)PuTTY-User-Key-File-[0-9]+:.*?(Private-MAC:[^\n]*|$)`), "PuTTY-User-Key-File " + aiRedacted},
		// Authorization headers
		{regexp.MustCompile(`(?i)\b(authorization|proxy-authorization)(\s*[:=]\s*)(bearer|basic|token|digest|negotiate)?\s*[^\s"',;]+`), "${1}${2}${3} " + aiRedacted},
		{regexp.MustCompile(`(?i)\b(bearer)\s+[A-Za-z0-9\-._~+/]{16,}=*`), "${1} " + aiRedacted},
		// passwords in URLs (scheme://user:password@host)
		{regexp.MustCompile(`\b([a-zA-Z][a-zA-Z0-9+.-]{1,20}://[^/\s:@'"]+):([^@\s/'"]+)@`), "${1}:" + aiRedacted + "@"},
		// well-known token formats
		{regexp.MustCompile(`\b(AKIA|ASIA|AGPA|AIDA|AROA|ANPA|ANVA|AIPA)[A-Z0-9]{16}\b`), aiRedacted},
		{regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{30,}\b`), aiRedacted},
		{regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,}\b`), aiRedacted},
		{regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}\b`), aiRedacted},
		{regexp.MustCompile(`\bgl(ptt|dt|rt|cbt|ft|imt|oas|sa)-[A-Za-z0-9_-]{20,}\b`), aiRedacted},
		{regexp.MustCompile(`\bxox[abposre]-[A-Za-z0-9-]{10,}`), aiRedacted},
		{regexp.MustCompile(`\bsk-(ant-)?[A-Za-z0-9_-]{20,}`), aiRedacted},
		{regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`), aiRedacted},
		{regexp.MustCompile(`\bya29\.[0-9A-Za-z_-]{20,}`), aiRedacted},
		{regexp.MustCompile(`\b[rsp]k_(live|test)_[A-Za-z0-9]{16,}\b`), aiRedacted},
		{regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`), aiRedacted},
		{regexp.MustCompile(`\bpypi-[A-Za-z0-9_-]{50,}`), aiRedacted},
		{regexp.MustCompile(`\bhv[sbr]\.[A-Za-z0-9_-]{20,}`), aiRedacted},
		{regexp.MustCompile(`\bdop_v1_[a-f0-9]{64}\b`), aiRedacted},
		{regexp.MustCompile(`\bSG\.[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{16,}`), aiRedacted},
		{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`), aiRedacted},
		{regexp.MustCompile(`(?i)\b(AccountKey|SharedAccessKey|sig)=([^;&\s"']+)`), "${1}=" + aiRedacted},
		// password hashes (shadow lines and bare crypt hashes)
		{regexp.MustCompile(`(?m)^([a-z_][a-z0-9_.-]*\$?):(\$(1|2[abxy]?|5|6|7|y|gy|sha1|md5)\$[^:\s]+|[!*]?[A-Za-z0-9./]{13,}):`), "${1}:" + aiRedacted + ":"},
		{regexp.MustCompile(`\$(2[abxy]|5|6|y|gy|argon2id?)\$[A-Za-z0-9./$=,+-]{20,}`), aiRedacted},
		// Quoted assignments can contain spaces; redact the complete value first.
		{regexp.MustCompile(keyRe + `(\s*[:=]\s*|"\s*:\s*|'\s*:\s*)(")(?:\\.|[^"\\])*(")`), "${1}${2}${3}" + aiRedacted + "${4}"},
		{regexp.MustCompile(keyRe + `(\s*[:=]\s*|"\s*:\s*|'\s*:\s*)(')(?:\\.|[^'\\])*(')`), "${1}${2}${3}" + aiRedacted + "${4}"},
		// key = value, key: value, "key": "value", --key value
		{regexp.MustCompile(keyRe + `(\s*[:=]\s*|"\s*:\s*"?|'\s*:\s*'?)(["']?)([^\s"',;}{]+)`), "${1}${2}${3}" + aiRedacted},
		{regexp.MustCompile(`(?i)(--?(?:password|passwd|pass|token|secret|api-key|apikey|client-secret|access-key|secret-key)(?:[= ]))("?)([^\s"']+)`), "${1}${2}" + aiRedacted},
	}
}()

// redactSecrets returns the text with secrets replaced and the number of replacements.
func redactSecrets(s string) (string, int) {
	if s == "" {
		return s, 0
	}
	n := 0
	for _, r := range aiRedactRules {
		s = r.re.ReplaceAllStringFunc(s, func(m string) string {
			out := r.re.ReplaceAllString(m, r.repl)
			if out != m {
				n++
			}
			return out
		})
	}
	return s, n
}

func redactText(s string) string {
	out, _ := redactSecrets(s)
	return out
}
