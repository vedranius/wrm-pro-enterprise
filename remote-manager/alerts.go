// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

// Security alert rules (v12.6.26).
//
// Every audit entry passes the rules below. A rule that matches writes a `security.alert`
// audit entry (so it reaches the SIEM like everything else) and tells every active
// administrator (Workboard inbox and the `security.alert` notification). Each rule has a
// setting, so it can be turned off or forced with WRM_ALERT_… variables:
//
//   alert_failed_signins (+ _minutes)  repeated failed sign-ins from one address or for one name
//   alert_break_glass                  a break-glass administrator signed in
//   alert_new_admin                    an account became an administrator
//   alert_secret_reveal                a stored secret was revealed or exported
//   alert_ai_full_access               a command WRM normally refuses was approved under AI full access
//   alert_audit_off                    the audit log or a SIEM export was turned off or removed
//
// The rules run on a separate goroutine fed by a buffered channel, so audit writes never
// wait for them.

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

type alertInput struct {
	Action, Username, IP, Target string
	Details                      map[string]interface{}
}

var alertQueue = make(chan alertInput, 2000)
var alertOnce sync.Once

// alertObserve is called by auditWrite for every entry it stored.
func alertObserve(action, username, ip, target, det string) {
	if strings.HasPrefix(action, "security.alert") {
		return
	}
	alertOnce.Do(func() { go alertWorker() })
	in := alertInput{Action: action, Username: username, IP: ip, Target: target}
	if det != "" && det[0] == '{' {
		json.Unmarshal([]byte(det), &in.Details)
	}
	select {
	case alertQueue <- in:
	default:
		log.Print("alerts: queue full, an audit entry was not checked")
	}
}

func alertWorker() {
	for in := range alertQueue {
		alertCheck(in)
	}
}

// failed sign-ins per key ("ip:…", "user:…") within the window
var alertFailures = struct {
	sync.Mutex
	hits  map[string][]time.Time
	fired map[string]time.Time
}{hits: map[string][]time.Time{}, fired: map[string]time.Time{}}

func detStr(d map[string]interface{}, k string) string {
	if d == nil {
		return ""
	}
	switch v := d[k].(type) {
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		return fmt.Sprint(v)
	}
	return ""
}

func alertCheck(in alertInput) {
	d := in.Details
	switch {
	case in.Action == "auth.login_failed" || in.Action == "auth.step_up_failed" || in.Action == "auth.mfa_failed" || in.Action == "mcp.auth_failed":
		limit := settingInt("alert_failed_signins")
		if limit <= 0 {
			return
		}
		window := time.Duration(settingInt("alert_failed_signins_minutes")) * time.Minute
		now := time.Now()
		keys := []string{}
		if in.IP != "" {
			keys = append(keys, "ip:"+in.IP)
		}
		if in.Username != "" {
			keys = append(keys, "user:"+strings.ToLower(in.Username))
		}
		for _, k := range keys {
			alertFailures.Lock()
			var keep []time.Time
			for _, t := range alertFailures.hits[k] {
				if now.Sub(t) < window {
					keep = append(keep, t)
				}
			}
			keep = append(keep, now)
			alertFailures.hits[k] = keep
			fire := len(keep) >= limit && now.Sub(alertFailures.fired[k]) >= window
			if fire {
				alertFailures.fired[k] = now
			}
			if len(alertFailures.hits) > 10000 { // forget old keys
				for kk, v := range alertFailures.hits {
					if len(v) == 0 || now.Sub(v[len(v)-1]) > window {
						delete(alertFailures.hits, kk)
					}
				}
			}
			alertFailures.Unlock()
			if fire {
				what := strings.TrimPrefix(strings.TrimPrefix(k, "ip:"), "user:")
				kind := map[bool]string{true: "address", false: "account"}[strings.HasPrefix(k, "ip:")]
				raiseAlert("failed_signins", in, fmt.Sprintf("%d failed sign-ins within %d minutes for the %s %s", len(keep), int(window.Minutes()), kind, what),
					fmt.Sprintf("%d neuspjelih prijava unutar %d minuta za %s %s", len(keep), int(window.Minutes()), map[string]string{"address": "adresu", "account": "račun"}[kind], what), true)
			}
		}
	case in.Action == "auth.break_glass_login":
		if settingBool("alert_break_glass") {
			// administrators already get security.break_glass_login; the alert is for the SIEM
			raiseAlert("break_glass", in, "Break-glass administrator "+in.Username+" signed in from "+in.IP,
				"Break-glass administrator "+in.Username+" prijavio se s adrese "+in.IP, false)
		}
	case alertMakesAdmin(in):
		if settingBool("alert_new_admin") {
			raiseAlert("new_admin", in, "Account "+in.Target+" became an administrator ("+in.Action+")",
				"Račun "+in.Target+" postao je administrator ("+in.Action+")", true)
		}
	case in.Action == "credential.revealed" || in.Action == "connection.secret_revealed" || in.Action == "ssh_key.exported" || (in.Action == "config.exported" && detStr(d, "with_secrets") == "true"):
		if settingBool("alert_secret_reveal") {
			what, hr := alertRevealText(in.Action)
			raiseAlert("secret_reveal", in, strings.TrimSpace(in.Username+" "+what+" "+in.Target), strings.TrimSpace(in.Username+" "+hr+" "+in.Target), true)
		}
	case in.Action == "ai.approved" && detStr(d, "rule") == "full_access":
		if settingBool("alert_ai_full_access") {
			raiseAlert("ai_full_access", in, "An AI command WRM normally refuses was approved under full access ("+detStr(d, "tool")+")",
				"Odobrena je AI naredba koju WRM inače odbija, uz puni pristup ("+detStr(d, "tool")+")", true)
		}
	case alertAuditOff(in):
		if settingBool("alert_audit_off") {
			raiseAlert("audit_off", in, "Audit or SIEM export was turned off or removed by "+in.Username+" ("+in.Action+")",
				"Audit ili izvoz u SIEM isključio je ili uklonio "+in.Username+" ("+in.Action+")", true)
		}
	}
}

// alertRevealText describes a secret reveal in English and Croatian.
func alertRevealText(action string) (string, string) {
	switch action {
	case "credential.revealed":
		return "revealed the stored credential", "otkrio je spremljenu vjerodajnicu"
	case "ssh_key.exported":
		return "exported the private SSH key", "izvezao je privatni SSH ključ"
	case "connection.secret_revealed":
		return "revealed what the connection logs in with:", "otkrio je s čime se veza prijavljuje:"
	}
	return "exported connections including their stored credentials", "izvezao je veze sa spremljenim vjerodajnicama"
}

func alertMakesAdmin(in alertInput) bool {
	d := in.Details
	switch in.Action {
	case "admin.user_created":
		return detStr(d, "admin") == "true" || detStr(d, "role") == userRoleAdmin
	case "admin.user_updated":
		return detStr(d, "role") == userRoleAdmin || detStr(d, "is_admin") == "true"
	case "oidc.account_created", "ldap.account_created":
		return detStr(d, "role") == userRoleAdmin
	}
	return strings.HasSuffix(in.Action, ".role_synced") && detStr(d, "to") == userRoleAdmin
}

func alertAuditOff(in alertInput) bool {
	switch in.Action {
	case "admin.settings":
		return detStr(in.Details, "audit_enabled") == "0"
	case "siem.destination_deleted":
		return true
	case "siem.destination_updated":
		return detStr(in.Details, "enabled") == "false"
	}
	return false
}

// raiseAlert audits the alert and tells the administrators.
func raiseAlert(rule string, in alertInput, summary, summaryHR string, notify bool) {
	auditLogAs(nil, 0, "system", "security.alert", rule, map[string]interface{}{"rule": rule, "summary": summary, "summary_hr": summaryHR,
		"source_action": in.Action, "user": in.Username, "ip": in.IP, "target": in.Target})
	if !notify {
		return
	}
	rows, err := db.Query(`SELECT id FROM users WHERE is_admin=1 AND disabled=0`)
	if err != nil {
		return
	}
	var ids []int
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		notifyUser(id, "security.alert", func(lang string) (string, string) {
			if lang == "hr" {
				return "Sigurnosna uzbuna", summaryHR
			}
			return "Security alert", summary
		})
	}
}

// recentAlerts for Admin → Security.
func recentAlerts(limit int) []map[string]interface{} {
	out := []map[string]interface{}{}
	rows, err := db.Query(`SELECT id, ts, target, details FROM audit_log WHERE action='security.alert' ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var ts, rule, det string
		rows.Scan(&id, &ts, &rule, &det)
		var d map[string]interface{}
		json.Unmarshal([]byte(det), &d)
		out = append(out, map[string]interface{}{"id": id, "ts": ts, "rule": rule, "summary": detStr(d, "summary"), "summary_hr": detStr(d, "summary_hr"), "user": detStr(d, "user"), "ip": detStr(d, "ip")})
	}
	return out
}
