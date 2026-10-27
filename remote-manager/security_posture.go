// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ─── SECURITY POSTURE ────────────────────────────────
//
// Admin → Security grades this installation against the chosen level: one check per item,
// ok / warn / bad (or info), with the value found and what the level requires. The browser
// adds the explanation and the exact fix in the user's language, and exports the same data as
// a report for auditors (JSON, or printed to PDF).

type postureCheck struct {
	ID        string             `json:"id"`
	Kind      string             `json:"kind"` // rule (a level setting) or check
	Status    string             `json:"status"`
	Value     string             `json:"value,omitempty"`
	Required  string             `json:"required,omitempty"`
	Env       string             `json:"env,omitempty"`
	Count     int                `json:"count,omitempty"`
	Recommend bool               `json:"recommended,omitempty"` // Required is only a recommendation at this level
	Deviation *securityDeviation `json:"deviation,omitempty"`
}

func securityPosture(r *http.Request) map[string]interface{} {
	level := securityLevel()
	li := levelIndex(level)
	strict := li > 0 // Business or Enterprise: missing protections are errors, not hints
	bad := func(ok bool) string {
		if ok {
			return "ok"
		}
		if strict {
			return "bad"
		}
		return "warn"
	}
	devs := securityDeviations()
	checks := []postureCheck{}

	// HTTPS for this browser.
	tr := "http"
	if isHTTPS(r) {
		tr = "https"
	} else if requestFromLoopback(r) {
		tr = "localhost"
	}
	checks = append(checks, postureCheck{ID: "transport", Kind: "check", Status: bad(tr != "http"), Value: tr})

	// The level's settings.
	for _, rule := range levelRules {
		c := postureCheck{ID: rule.Key, Kind: "rule", Value: getSetting(rule.Key), Required: rule.Need[li]}
		switch {
		case c.Required == "" && rule.Rec != "":
			c.Required, c.Recommend = rule.Rec, true
			c.Status = map[bool]string{true: "ok", false: "warn"}[levelMeets(rule.Key, c.Value, rule.Rec)]
		case c.Required == "":
			c.Status = "info"
		case levelMeets(rule.Key, c.Value, c.Required):
			c.Status = "ok"
		case settingLocked(rule.Key):
			c.Status = "bad"
			_, c.Env = settingEnv(rule.Key)
		default:
			c.Status = "bad"
			if d, ok := devs[rule.Key]; ok {
				d := d
				c.Status, c.Deviation = "warn", &d
			}
		}
		checks = append(checks, c)
	}

	// Two-factor authentication actually set up.
	var admins, adminsNo2FA, usersNo2FA int
	db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN totp_enabled=0 THEN 1 ELSE 0 END),0) FROM users WHERE disabled=0 AND is_admin=1`).Scan(&admins, &adminsNo2FA)
	db.QueryRow(`SELECT COUNT(*) FROM users WHERE disabled=0 AND totp_enabled=0`).Scan(&usersNo2FA)
	checks = append(checks, postureCheck{ID: "admins_2fa", Kind: "check", Status: bad(adminsNo2FA == 0), Count: adminsNo2FA})
	if getSetting("require_2fa") == "all" || li > 0 {
		st := "ok"
		if usersNo2FA > 0 {
			st = "warn"
		}
		checks = append(checks, postureCheck{ID: "users_2fa", Kind: "check", Status: st, Count: usersNo2FA})
	}

	// The key that encrypts stored secrets.
	keyCheck := postureCheck{ID: "encryption_key", Kind: "check", Status: "ok", Value: "separate"}
	if os.Getenv("ENCRYPTION_KEY") == "" && os.Getenv("ENCRYPTION_KEY_FILE") == "" {
		keyCheck.Value = "next_to_database"
		keyCheck.Status = map[bool]string{true: "warn", false: "info"}[strict]
	}
	checks = append(checks, keyCheck)

	// Cross-origin requests.
	checks = append(checks, postureCheck{ID: "any_origin", Kind: "check", Status: map[bool]string{true: "bad", false: "ok"}[os.Getenv("WRM_ALLOW_ANY_ORIGIN") == "1"]})

	// Guest links that never expire.
	var openLinks int
	db.QueryRow(`SELECT COUNT(*) FROM share_links WHERE active=1 AND access_mode='link' AND expires_at=''`).Scan(&openLinks)
	linkSt := "ok"
	if openLinks > 0 {
		linkSt = "warn"
	}
	checks = append(checks, postureCheck{ID: "open_links", Kind: "check", Status: linkSt, Count: openLinks})

	// Signed updates and the current version.
	checks = append(checks, postureCheck{ID: "update_signature", Kind: "check", Status: map[bool]string{true: "ok", false: "warn"}[len(releaseTrustedKeys()) > 0], Count: len(releaseTrustedKeys())})
	ver := postureCheck{ID: "version", Kind: "check", Status: "ok", Value: AppVersion}
	if !settingBool("update_check") {
		ver.Status = "info"
	} else if info := lastUpdateInfo(); updateAvailable(info) {
		ver.Status, ver.Required = "warn", info.Latest
	}
	checks = append(checks, ver)

	// The audit log's hash chain.
	av := verifyAuditChain()
	checks = append(checks, postureCheck{ID: "audit_chain", Kind: "check", Status: map[bool]string{true: "ok", false: "bad"}[av.OK], Count: av.Checked, Value: strconv.Itoa(av.BrokenAt)})

	sum := map[string]int{"ok": 0, "warn": 0, "bad": 0, "info": 0}
	for _, c := range checks {
		sum[c.Status]++
	}
	devList := []securityDeviation{}
	for _, rule := range levelRules {
		if d, ok := devs[rule.Key]; ok {
			devList = append(devList, d)
		}
	}
	rules := []map[string]interface{}{}
	for _, rule := range levelRules {
		rules = append(rules, map[string]interface{}{"key": rule.Key, "need": rule.Need, "rec": rule.Rec})
	}
	_, levelEnv := settingEnv("security_level")
	host, _ := os.Hostname()
	return map[string]interface{}{
		"level": level, "level_locked": settingLocked("security_level"), "level_env": levelEnv, "levels": securityLevels,
		"rules": rules, "deviations": devList, "checks": checks, "summary": sum,
		"version": AppVersion, "host": strings.TrimSpace(host), "generated_at": time.Now().UTC().Format(time.RFC3339),
	}
}
