// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/smtp"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // quiet hours use IANA time zones, also where the system has none
	"unicode"       // quiet hours use IANA time zones, also where the system has none
)

// ─── NOTIFICATIONS ───────────────────────────────────
//
// A general module that other features use to tell users about events outside the
// browser. Administrators configure the channels (an SMTP server, a Telegram bot, a Slack
// webhook, …); users choose which events they want to receive and through which channels.
// The in-app toasts and browser notifications are separate and stay as they are.
//
//   feature ──emit()──► notify_pending (per user, channel and address) ──dispatcher──► channel
//
// emit() only stores the message. The dispatcher sends what is waiting for one user and
// channel as one message (a digest): everything a status round finds is emitted within a
// moment, so one round gives one message. A channel sends to the same recipient at most
// once per notify_min_interval_seconds; what comes in meanwhile waits for the next digest.
// During a user's quiet hours messages wait and go out as one digest when they end.
// Pending messages are in the database, so they survive a restart. Failed deliveries are
// retried with a growing delay and dropped after notifyMaxAttempts.
//
// Channel secrets (bot tokens, webhook URLs, passwords) are encrypted at rest and never
// returned to the browser.

const (
	maxNotifyChannels   = 100
	maxPendingPerUser   = 500
	notifyMaxAttempts   = 6
	notifyHTTPTimeout   = 15 * time.Second
	notifyDigestMaxRows = 30 // lines in one digest; the rest is counted
)

// notifyDebounce is how long the dispatcher waits after the newest event of a group before it
// sends it, so that the events of one check round end up in one message.
var notifyDebounce = 3 * time.Second

// Event types users can subscribe to.
var notifyEvents = []string{"monitor.problem", "monitor.recovered", "status.down", "status.up", "credential.rotation_due", "credential.rotation_incomplete", "git.new_version", "git.drift", "git.unreachable", "git.functionality_problem", "git.functionality_recovered", "git.server_problem", "git.server_recovered",
	"git.deploy_started", "git.deploy_done", "git.deploy_failed", "git.restart_scheduled", "git.restart_done", "git.job_upcoming", "security.break_glass_login", "security.alert", "ai.approval"}

func validNotifyEvent(e string) bool {
	for _, x := range notifyEvents {
		if x == e {
			return true
		}
	}
	return false
}

// ─── CHANNEL KINDS ───────────────────────────────────

type notifyField struct {
	Name     string   `json:"name"`
	Secret   bool     `json:"secret,omitempty"`
	Required bool     `json:"required,omitempty"`
	Kind     string   `json:"kind,omitempty"` // "" (text) | url | int | enum
	Enum     []string `json:"enum,omitempty"`
	Default  string   `json:"default,omitempty"`
}

type notifyKind struct {
	Kind    string        `json:"kind"`
	Fields  []notifyField `json:"fields"`
	Address string        `json:"address,omitempty"` // the field a user may set to their own value
}

var notifyKinds = []notifyKind{
	{Kind: "email", Address: "to", Fields: []notifyField{
		{Name: "host", Required: true}, {Name: "port", Kind: "int", Default: "587"},
		{Name: "security", Kind: "enum", Enum: []string{"starttls", "tls", "none"}, Default: "starttls"},
		{Name: "username"}, {Name: "password", Secret: true},
		{Name: "from", Required: true}, {Name: "to", Required: true}}},
	{Kind: "telegram", Address: "chat_id", Fields: []notifyField{
		{Name: "bot_token", Secret: true, Required: true}, {Name: "chat_id", Required: true},
		{Name: "api_url", Kind: "url", Default: "https://api.telegram.org"}}},
	{Kind: "slack", Fields: []notifyField{{Name: "webhook_url", Secret: true, Required: true, Kind: "url"}}},
	{Kind: "teams", Fields: []notifyField{{Name: "webhook_url", Secret: true, Required: true, Kind: "url"}}},
	{Kind: "discord", Fields: []notifyField{{Name: "webhook_url", Secret: true, Required: true, Kind: "url"}}},
	{Kind: "ntfy", Address: "topic", Fields: []notifyField{
		{Name: "server", Kind: "url", Default: "https://ntfy.sh"}, {Name: "topic", Required: true},
		{Name: "access_token", Secret: true}}},
	{Kind: "gotify", Fields: []notifyField{
		{Name: "server", Kind: "url", Required: true}, {Name: "app_token", Secret: true, Required: true}}},
	{Kind: "pushover", Address: "user_key", Fields: []notifyField{
		{Name: "app_token", Secret: true, Required: true}, {Name: "user_key", Required: true},
		{Name: "api_url", Kind: "url", Default: "https://api.pushover.net"}}},
	{Kind: "webhook", Fields: []notifyField{
		{Name: "url", Kind: "url", Required: true}, {Name: "hmac_secret", Secret: true}}},
}

func notifyKindFor(kind string) (notifyKind, bool) {
	for _, k := range notifyKinds {
		if k.Kind == kind {
			return k, true
		}
	}
	return notifyKind{}, false
}

// ─── CHANNELS ────────────────────────────────────────

type notifyChannel struct {
	ID          int               `json:"id"`
	Name        string            `json:"name"`
	Kind        string            `json:"kind"`
	Enabled     bool              `json:"enabled"`
	UserAddress bool              `json:"user_address"` // users may set their own recipient
	Config      map[string]string `json:"config"`       // secret fields are empty in API answers
	SecretsSet  []string          `json:"secrets_set"`  // secret fields that have a value
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
	LastSentAt  string            `json:"last_sent_at"`
	LastError   string            `json:"last_error"`
	Subscribers int               `json:"subscribers"`
	cfg         map[string]string // decrypted configuration
}

const notifyChannelCols = `id,name,kind,config,enabled,user_address,created_at,updated_at,last_sent_at,last_error`

func scanNotifyChannel(sc interface{ Scan(...interface{}) error }) (notifyChannel, error) {
	var ch notifyChannel
	var raw string
	var en, ua int
	if err := sc.Scan(&ch.ID, &ch.Name, &ch.Kind, &raw, &en, &ua, &ch.CreatedAt, &ch.UpdatedAt, &ch.LastSentAt, &ch.LastError); err != nil {
		return ch, err
	}
	ch.Enabled, ch.UserAddress = en == 1, ua == 1
	stored := map[string]string{}
	json.Unmarshal([]byte(raw), &stored)
	ch.cfg, ch.Config, ch.SecretsSet = map[string]string{}, map[string]string{}, []string{}
	k, _ := notifyKindFor(ch.Kind)
	for _, f := range k.Fields {
		v := stored[f.Name]
		if f.Secret {
			v = decryptValue(v)
			if v != "" {
				ch.SecretsSet = append(ch.SecretsSet, f.Name)
			}
			ch.Config[f.Name] = ""
		} else {
			ch.Config[f.Name] = v
		}
		ch.cfg[f.Name] = v
	}
	return ch, nil
}

func loadNotifyChannel(id int) (notifyChannel, error) {
	ch, err := scanNotifyChannel(db.QueryRow(`SELECT `+notifyChannelCols+` FROM notify_channels WHERE id=?`, id))
	if err != nil {
		return ch, fmt.Errorf("channel not found")
	}
	return ch, nil
}

func loadNotifyChannels() []notifyChannel {
	out := []notifyChannel{}
	rows, err := db.Query(`SELECT ` + notifyChannelCols + ` FROM notify_channels ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		if ch, err := scanNotifyChannel(rows); err == nil {
			out = append(out, ch)
		}
	}
	return out
}

// get returns a configuration value or the field's default.
func (ch notifyChannel) get(name string) string {
	if v := strings.TrimSpace(ch.cfg[name]); v != "" {
		return v
	}
	k, _ := notifyKindFor(ch.Kind)
	for _, f := range k.Fields {
		if f.Name == name {
			return f.Default
		}
	}
	return ""
}

var notifyAddrRe = regexp.MustCompile(`^[^\s<>"]{1,200}$`)

// validateNotifyConfig checks a channel configuration (the decrypted values).
func validateNotifyConfig(kind string, cfg map[string]string, userAddress bool) error {
	k, ok := notifyKindFor(kind)
	if !ok {
		return fmt.Errorf("unknown channel type %q", kind)
	}
	for _, f := range k.Fields {
		v := strings.TrimSpace(cfg[f.Name])
		if len(v) > 2000 {
			return fmt.Errorf("%s: too long", f.Name)
		}
		if strings.ContainsAny(v, "\r\n") {
			return fmt.Errorf("%s: must be one line", f.Name)
		}
		if v == "" {
			if f.Required && !(userAddress && f.Name == k.Address) {
				return fmt.Errorf("%s is required", f.Name)
			}
			continue
		}
		switch f.Kind {
		case "url":
			u, err := url.Parse(v)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return fmt.Errorf("%s: enter an http:// or https:// address", f.Name)
			}
		case "int":
			if n, err := strconv.Atoi(v); err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("%s: 1 to 65535", f.Name)
			}
		case "enum":
			found := false
			for _, e := range f.Enum {
				found = found || e == v
			}
			if !found {
				return fmt.Errorf("%s: one of %s", f.Name, strings.Join(f.Enum, ", "))
			}
		}
	}
	if kind == "email" {
		for _, a := range append([]string{cfg["from"]}, splitAddrs(cfg["to"])...) {
			if strings.TrimSpace(a) != "" && !validEmail(a) {
				return fmt.Errorf("invalid e-mail address %q", a)
			}
		}
	}
	return nil
}

var emailRe = regexp.MustCompile(`^[^\s@<>",;]+@[^\s@<>",;]+$`)

func validEmail(a string) bool {
	if strings.IndexFunc(a, unicode.IsControl) >= 0 {
		return false // no CR / LF / TAB: the address goes into a mail header
	}
	a = strings.TrimSpace(a)
	if i := strings.LastIndexByte(a, '<'); i >= 0 && strings.HasSuffix(a, ">") {
		a = a[i+1 : len(a)-1]
	}
	return emailRe.MatchString(a)
}

func splitAddrs(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// validateNotifyAddress checks a recipient a user enters for a channel.
func validateNotifyAddress(ch notifyChannel, addr string) error {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil
	}
	if !ch.UserAddress {
		return fmt.Errorf("channel %q sends to a fixed recipient", ch.Name)
	}
	if ch.Kind == "email" {
		list := splitAddrs(addr)
		if len(list) == 0 || len(list) > 5 {
			return fmt.Errorf("1 to 5 e-mail addresses")
		}
		for _, a := range list {
			if !emailRe.MatchString(a) { // users enter bare addresses only (no display name)
				return fmt.Errorf("invalid e-mail address %q", a)
			}
		}
		return nil
	}
	if !notifyAddrRe.MatchString(addr) {
		return fmt.Errorf("invalid recipient")
	}
	return nil
}

// ─── MESSAGES ────────────────────────────────────────

type notifyMessage struct {
	Event string    `json:"event"`
	Title string    `json:"title"`
	Body  string    `json:"body"`
	Time  time.Time `json:"time"`
}

// notifyTexts holds the server side message texts (the user's language is stored with
// their preferences).
var notifyTexts = map[string]map[string]string{
	"en": {
		"status.down":                    "🔴 {name} is down",
		"status.down.body":               "{name} ({host}) does not answer: {error}.",
		"status.up":                      "🟢 {name} is up again",
		"status.up.body":                 "{name} ({host}) answers again{after}.",
		"status.up.after":                " after {dur}",
		"credential.rotation_due":        "⏰ Rotation due: {name}",
		"credential.rotation_due.body":   "The password of the credential {name} ({user}) was last changed on {date}. Its rotation interval is {days} days.",
		"credential.rotation_incomplete": "⚠ Rotation incomplete: {name}",
		"credential.rotation_incomplete.body": "The last rotation of the credential {name} ({user}) could not be completed: {status}. " +
			"Some servers have the new password (kept as pending in the vault). Fix them and save the right password.",
		"git.new_version":                  "🆕 {app}: {new}",
		"git.new_version.body":             "A new version of {app} is available: {old} → {new}.",
		"git.server_problem":               "⚠ Server services: {server}",
		"git.server_problem.body":          "Server service checks on {server} need attention: {error}.",
		"git.server_recovered":             "✓ Server services recovered: {server}",
		"git.server_recovered.body":        "Server service checks on {server} are healthy again.",
		"git.functionality_problem":        "⚠ Service check: {app} on {server}",
		"git.functionality_problem.body":   "Functionality of {app} in {path} on {server} needs attention: {error}.",
		"git.functionality_recovered":      "✓ Service recovered: {app} on {server}",
		"git.functionality_recovered.body": "Functionality checks for {app} in {path} on {server} are healthy again.",
		"git.drift":                        "✏ Drift: {app} on {server}",
		"git.drift.body":                   "Files of {app} in {path} on {server} were changed by hand. Review them in the Git workspace.",
		"git.unreachable":                  "⚠ Git check: {server} unreachable",
		"git.unreachable.body":             "The Git check could not log in to {server}: {error}.",
		"git.deploy_started":               "▶ Git {kind} started: {summary}",
		"git.deploy_started.body":          "Run {label}: {kind} of {summary}.",
		"git.deploy_done":                  "✅ Git {kind} done: {summary}",
		"git.deploy_done.body":             "Run {label} finished: {kind} of {summary}. Restart: {units}.",
		"git.deploy_failed":                "❌ Git {kind} failed: {summary}",
		"git.deploy_failed.body":           "Run {label} ({kind} of {summary}) did not succeed: {message}",
		"git.restart_scheduled":            "⏰ Restart scheduled at {time}: {summary}",
		"git.restart_scheduled.body":       "Run {label}: these units will be restarted at {time}: {units}.",
		"git.restart_done":                 "🔄 Restart {result}: {summary}",
		"git.restart_done.body":            "Run {label}: {units}. Health check: {health}. {message}",
		"git.job_upcoming":                 "⏳ Git {kind} at {time}: {summary}",
		"git.job_upcoming.body":            "Run {label} ({kind} of {summary}) starts at {time}. Cancel it in the Git workspace if it should not run.",
		"git.result.ok":                    "done",
		"git.result.failed":                "failed",
		"digest":                           "WRM: {n} notifications",
		"digest.more":                      "… and {n} more",
		"test":                             "WRM test message",
		"test.body":                        "This is a test message from WRM PRO (channel {channel}). If you can read it, the channel works.",
	},
	"hr": {
		"status.down":                    "🔴 {name} je nedostupan",
		"status.down.body":               "{name} ({host}) se ne javlja: {error}.",
		"status.up":                      "🟢 {name} je ponovno dostupan",
		"status.up.body":                 "{name} ({host}) se ponovno javlja{after}.",
		"status.up.after":                " nakon {dur}",
		"credential.rotation_due":        "⏰ Vrijeme za rotaciju: {name}",
		"credential.rotation_due.body":   "Lozinka vjerodajnice {name} ({user}) zadnji je put promijenjena {date}. Interval rotacije je {days} dana.",
		"credential.rotation_incomplete": "⚠ Rotacija nije dovršena: {name}",
		"credential.rotation_incomplete.body": "Zadnja rotacija vjerodajnice {name} ({user}) nije dovršena: {status}. " +
			"Neki serveri imaju novu lozinku (spremljena je kao nedovršena u trezoru). Popravite ih i spremite ispravnu lozinku.",
		"git.new_version":                  "🆕 {app}: {new}",
		"git.new_version.body":             "Dostupna je nova verzija servisa {app}: {old} → {new}.",
		"git.server_problem":               "⚠ Servisi servera: {server}",
		"git.server_problem.body":          "Provjere servisa na serveru {server} zahtijevaju pažnju: {error}.",
		"git.server_recovered":             "✓ Servisi servera oporavljeni: {server}",
		"git.server_recovered.body":        "Provjere servisa na serveru {server} ponovno su uspješne.",
		"git.functionality_problem":        "⚠ Provjera servisa: {app} na {server}",
		"git.functionality_problem.body":   "Funkcionalnost servisa {app} u {path} na {server} zahtijeva pažnju: {error}.",
		"git.functionality_recovered":      "✓ Servis oporavljen: {app} na {server}",
		"git.functionality_recovered.body": "Provjere funkcionalnosti servisa {app} u {path} na {server} ponovno su uspješne.",
		"git.drift":                        "✏ Odstupanje: {app} na {server}",
		"git.drift.body":                   "Datoteke servisa {app} u {path} na {server} ručno su promijenjene. Pregledajte ih u Git radnom prostoru.",
		"git.unreachable":                  "⚠ Git provjera: {server} nedostupan",
		"git.unreachable.body":             "Git provjera se ne može prijaviti na {server}: {error}.",
		"git.deploy_started":               "▶ Git {kind} pokrenuto: {summary}",
		"git.deploy_started.body":          "Izvođenje {label}: {kind} za {summary}.",
		"git.deploy_done":                  "✅ Git {kind} gotovo: {summary}",
		"git.deploy_done.body":             "Izvođenje {label} završeno: {kind} za {summary}. Ponovno pokretanje: {units}.",
		"git.deploy_failed":                "❌ Git {kind} nije uspjelo: {summary}",
		"git.deploy_failed.body":           "Izvođenje {label} ({kind} za {summary}) nije uspjelo: {message}",
		"git.restart_scheduled":            "⏰ Ponovno pokretanje zakazano u {time}: {summary}",
		"git.restart_scheduled.body":       "Izvođenje {label}: ove jedinice ponovno će se pokrenuti u {time}: {units}.",
		"git.restart_done":                 "🔄 Ponovno pokretanje {result}: {summary}",
		"git.restart_done.body":            "Izvođenje {label}: {units}. Provjera stanja: {health}. {message}",
		"git.job_upcoming":                 "⏳ Git {kind} u {time}: {summary}",
		"git.job_upcoming.body":            "Izvođenje {label} ({kind} za {summary}) počinje u {time}. Otkažite ga u Git radnom prostoru ako se ne smije izvesti.",
		"git.result.ok":                    "gotovo",
		"git.result.failed":                "nije uspjelo",
		"digest":                           "WRM: {n} obavijesti",
		"digest.more":                      "… i još {n}",
		"test":                             "WRM probna poruka",
		"test.body":                        "Ovo je probna poruka iz WRM PRO (kanal {channel}). Ako je vidite, kanal radi.",
	},
}

func notifyText(lang, key string, vars map[string]string) string {
	s, ok := notifyTexts[lang][key]
	if !ok {
		s = notifyTexts["en"][key]
	}
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

// ─── PREFERENCES ─────────────────────────────────────

type notifyPrefs struct {
	QuietEnabled bool   `json:"quiet_enabled"`
	QuietStart   string `json:"quiet_start"` // HH:MM
	QuietEnd     string `json:"quiet_end"`
	TZ           string `json:"tz"`   // IANA time zone of the user
	Lang         string `json:"lang"` // en | hr
}

func loadNotifyPrefs(userID int) notifyPrefs {
	p := notifyPrefs{QuietStart: "22:00", QuietEnd: "07:00", TZ: "UTC", Lang: "en"}
	var q int
	if db.QueryRow(`SELECT quiet_enabled, quiet_start, quiet_end, tz, lang FROM notify_prefs WHERE user_id=?`, userID).
		Scan(&q, &p.QuietStart, &p.QuietEnd, &p.TZ, &p.Lang) == nil {
		p.QuietEnabled = q == 1
	}
	return p
}

var hhmmRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func (p *notifyPrefs) validate() error {
	if p.Lang != "hr" {
		p.Lang = "en"
	}
	if p.TZ == "" {
		p.TZ = "UTC"
	}
	if _, err := time.LoadLocation(p.TZ); err != nil || len(p.TZ) > 64 {
		return fmt.Errorf("unknown time zone %q", p.TZ)
	}
	if !hhmmRe.MatchString(p.QuietStart) || !hhmmRe.MatchString(p.QuietEnd) {
		return fmt.Errorf("quiet hours: use HH:MM")
	}
	return nil
}

// inQuietHours reports whether t falls into the user's quiet hours.
func (p notifyPrefs) inQuietHours(t time.Time) bool {
	if !p.QuietEnabled || p.QuietStart == p.QuietEnd {
		return false
	}
	loc, err := time.LoadLocation(p.TZ)
	if err != nil {
		loc = time.UTC
	}
	lt := t.In(loc)
	now := lt.Hour()*60 + lt.Minute()
	mins := func(s string) int {
		h, _ := strconv.Atoi(s[:2])
		m, _ := strconv.Atoi(s[3:])
		return h*60 + m
	}
	start, end := mins(p.QuietStart), mins(p.QuietEnd)
	if start < end {
		return now >= start && now < end
	}
	return now >= start || now < end // over midnight
}

// ─── EMIT ────────────────────────────────────────────

// notifyUser queues an event for a user on every channel they subscribed it to. text
// builds the title and body in the user's language. It reports whether anything was queued.
func notifyUser(userID int, event string, text func(lang string) (string, string)) bool {
	if userID > 0 {
		// the in-app list (Workboard) keeps every event, subscribed or not
		t, b := text(loadNotifyPrefs(userID).Lang)
		recordUserEvent(userID, event, t, b)
	}
	if userID <= 0 || !settingBool("notifications_enabled") {
		return false
	}
	rows, err := db.Query(`SELECT s.channel_id, s.address FROM notify_subscriptions s JOIN notify_channels c ON c.id=s.channel_id
		WHERE s.user_id=? AND s.event=? AND c.enabled=1`, userID, event)
	if err != nil {
		return false
	}
	type sub struct {
		ch   int
		addr string
	}
	var subs []sub
	for rows.Next() {
		var s sub
		if rows.Scan(&s.ch, &s.addr) == nil {
			subs = append(subs, s)
		}
	}
	rows.Close()
	if len(subs) == 0 {
		return false
	}
	title, body := text(loadNotifyPrefs(userID).Lang)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, s := range subs {
		db.Exec(`INSERT INTO notify_pending (user_id, channel_id, address, event, title, body, created_at) VALUES (?,?,?,?,?,?,?)`,
			userID, s.ch, s.addr, event, truncateStr(title, 300), truncateStr(body, 2000), now)
	}
	// Bound what can pile up (e.g. a channel that has been failing for hours).
	db.Exec(`DELETE FROM notify_pending WHERE user_id=? AND id NOT IN (SELECT id FROM notify_pending WHERE user_id=? ORDER BY id DESC LIMIT ?)`,
		userID, userID, maxPendingPerUser)
	notifier.poke()
	return true
}

// ─── DISPATCHER ──────────────────────────────────────

type notifyGroupKey struct {
	user, channel int
	addr          string
}

type notifyDispatcher struct {
	mu       sync.Mutex // one dispatch at a time
	stateMu  sync.Mutex
	lastSent map[notifyGroupKey]time.Time
	retryAt  map[notifyGroupKey]time.Time
	attempts map[notifyGroupKey]int
	trigger  chan struct{}
}

var notifier = &notifyDispatcher{lastSent: map[notifyGroupKey]time.Time{}, retryAt: map[notifyGroupKey]time.Time{},
	attempts: map[notifyGroupKey]int{}, trigger: make(chan struct{}, 1)}

func (d *notifyDispatcher) poke() {
	select {
	case d.trigger <- struct{}{}:
	default:
	}
}

func (d *notifyDispatcher) run() {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
		case <-d.trigger:
			time.Sleep(notifyDebounce / 2)
		}
		if !settingBool("notifications_enabled") {
			// Turned off: nothing more is sent, also not what was waiting or being retried.
			db.Exec(`DELETE FROM notify_pending`)
			continue
		}
		d.dispatch(time.Now(), false)
	}
}

type pendingRow struct {
	id      int
	msg     notifyMessage
	created time.Time
}

// dispatch sends the groups that are ready. force ignores the debounce and the rate limit
// (tests; quiet hours still apply).
func (d *notifyDispatcher) dispatch(now time.Time, force bool) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	rows, err := db.Query(`SELECT id, user_id, channel_id, address, event, title, body, created_at FROM notify_pending ORDER BY id`)
	if err != nil {
		return 0
	}
	groups := map[notifyGroupKey][]pendingRow{}
	var order []notifyGroupKey
	for rows.Next() {
		var p pendingRow
		var k notifyGroupKey
		var created string
		if rows.Scan(&p.id, &k.user, &k.channel, &k.addr, &p.msg.Event, &p.msg.Title, &p.msg.Body, &created) != nil {
			continue
		}
		p.created, _ = time.Parse(time.RFC3339Nano, created)
		p.msg.Time = p.created
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], p)
	}
	rows.Close()
	minGap := time.Duration(settingInt("notify_min_interval_seconds")) * time.Second
	sent := 0
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	var cntMu sync.Mutex
	prefs := map[int]notifyPrefs{}
	for _, k := range order {
		list := groups[k]
		if _, ok := prefs[k.user]; !ok {
			prefs[k.user] = loadNotifyPrefs(k.user)
		}
		p := prefs[k.user]
		if p.inQuietHours(now) {
			continue
		}
		d.stateMu.Lock()
		last, retry := d.lastSent[k], d.retryAt[k]
		d.stateMu.Unlock()
		if !force {
			if now.Sub(list[len(list)-1].created) < notifyDebounce || now.Sub(last) < minGap || now.Before(retry) {
				continue
			}
		}
		ch, err := loadNotifyChannel(k.channel)
		if err != nil || !ch.Enabled {
			db.Exec(`DELETE FROM notify_pending WHERE user_id=? AND channel_id=? AND address=?`, k.user, k.channel, k.addr)
			continue
		}
		wg.Add(1)
		go func(k notifyGroupKey, list []pendingRow, ch notifyChannel, lang string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			msgs := make([]notifyMessage, len(list))
			for i, p := range list {
				msgs[i] = p.msg
			}
			title, body := digestText(lang, msgs)
			err := sendNotification(ch, k.addr, title, body, msgs)
			ts := time.Now().UTC().Format(time.RFC3339)
			ids := make([]string, len(list))
			for i, p := range list {
				ids[i] = strconv.Itoa(p.id)
			}
			idList := strings.Join(ids, ",")
			d.stateMu.Lock()
			defer d.stateMu.Unlock()
			if err == nil {
				db.Exec(`DELETE FROM notify_pending WHERE id IN (` + idList + `)`)
				db.Exec(`UPDATE notify_channels SET last_sent_at=?, last_error='' WHERE id=?`, ts, ch.ID)
				d.lastSent[k] = time.Now()
				delete(d.retryAt, k)
				delete(d.attempts, k)
				cntMu.Lock()
				sent++
				cntMu.Unlock()
				return
			}
			db.Exec(`UPDATE notify_channels SET last_error=? WHERE id=?`, truncateStr(ts+": "+err.Error(), 500), ch.ID)
			d.attempts[k]++
			n := d.attempts[k]
			if n >= notifyMaxAttempts {
				log.Printf("Notifications: dropping %d message(s) for user %d on channel %q after %d attempts: %v", len(list), k.user, ch.Name, n, err)
				db.Exec(`DELETE FROM notify_pending WHERE id IN (` + idList + `)`)
				delete(d.attempts, k)
				delete(d.retryAt, k)
				return
			}
			backoff := time.Duration(1<<uint(n)) * 15 * time.Second
			if backoff > 30*time.Minute {
				backoff = 30 * time.Minute
			}
			d.retryAt[k] = time.Now().Add(backoff)
			log.Printf("Notifications: channel %q failed (attempt %d, retry in %s): %v", ch.Name, n, backoff, err)
		}(k, list, ch, p.Lang)
	}
	wg.Wait()
	return sent
}

// digestText turns the waiting messages of one recipient into one title and body.
func digestText(lang string, msgs []notifyMessage) (string, string) {
	if len(msgs) == 1 {
		return msgs[0].Title, msgs[0].Body
	}
	var b strings.Builder
	for i, m := range msgs {
		if i == notifyDigestMaxRows {
			b.WriteString(notifyText(lang, "digest.more", map[string]string{"n": strconv.Itoa(len(msgs) - i)}) + "\n")
			break
		}
		b.WriteString("• " + m.Title)
		if m.Body != "" {
			b.WriteString(" — " + m.Body)
		}
		b.WriteString("\n")
	}
	return notifyText(lang, "digest", map[string]string{"n": strconv.Itoa(len(msgs))}), strings.TrimRight(b.String(), "\n")
}

// ─── SENDERS ─────────────────────────────────────────

var notifyHTTPClient = &http.Client{Timeout: notifyHTTPTimeout}

// sendNotification delivers one message through a channel. addr overrides the channel's
// recipient when the channel allows users to set their own.
func sendNotification(ch notifyChannel, addr, title, body string, msgs []notifyMessage) error {
	k, _ := notifyKindFor(ch.Kind)
	if addr = strings.TrimSpace(addr); addr != "" && ch.UserAddress && k.Address != "" {
		cfg := map[string]string{}
		for kk, v := range ch.cfg {
			cfg[kk] = v
		}
		cfg[k.Address] = addr
		ch.cfg = cfg
	}
	if k.Address != "" && ch.get(k.Address) == "" {
		return fmt.Errorf("no recipient: enter your %s in your notification settings", k.Address)
	}
	text := title
	if body != "" {
		text += "\n\n" + body
	}
	base := func(name string) string { return strings.TrimRight(ch.get(name), "/") }
	switch ch.Kind {
	case "email":
		return sendSMTP(ch, title, body)
	case "telegram":
		return postJSON(base("api_url")+"/bot"+ch.get("bot_token")+"/sendMessage", map[string]interface{}{
			"chat_id": ch.get("chat_id"), "text": truncateStr(text, 4000), "disable_web_page_preview": true}, nil, "telegram")
	case "slack":
		// Names and hosts come from users: no <!channel>, @here or links of their own.
		return postJSON(ch.get("webhook_url"), map[string]interface{}{"text": truncateStr(slackEscape(text), 30000)}, nil, "")
	case "discord":
		return postJSON(ch.get("webhook_url"), map[string]interface{}{"content": truncateStr(text, 1990),
			"allowed_mentions": map[string]interface{}{"parse": []string{}}}, nil, "")
	case "teams":
		blocks := []map[string]interface{}{{"type": "TextBlock", "text": title, "weight": "Bolder", "size": "Medium", "wrap": true}}
		if body != "" {
			blocks = append(blocks, map[string]interface{}{"type": "TextBlock", "text": truncateStr(body, 20000), "wrap": true})
		}
		return postJSON(ch.get("webhook_url"), map[string]interface{}{"type": "message", "attachments": []map[string]interface{}{{
			"contentType": "application/vnd.microsoft.card.adaptive",
			"content": map[string]interface{}{"$schema": "http://adaptivecards.io/schemas/adaptive-card.json", "type": "AdaptiveCard",
				"version": "1.4", "body": blocks}}}}, nil, "")
	case "ntfy":
		h := map[string]string{"Title": mime.QEncoding.Encode("utf-8", title), "Tags": "wrm"}
		if tok := ch.get("access_token"); tok != "" {
			h["Authorization"] = "Bearer " + tok
		}
		return postBody(base("server")+"/"+url.PathEscape(ch.get("topic")), "text/plain; charset=utf-8", []byte(truncateStr(body, 4000)), h)
	case "gotify":
		return postJSON(base("server")+"/message", map[string]interface{}{"title": title, "message": body, "priority": 5},
			map[string]string{"X-Gotify-Key": ch.get("app_token")}, "")
	case "pushover":
		form := url.Values{"token": {ch.get("app_token")}, "user": {ch.get("user_key")}, "title": {truncateStr(title, 250)}, "message": {truncateStr(body, 1024)}}
		if body == "" {
			form.Set("message", title)
		}
		return postBody(base("api_url")+"/1/messages.json", "application/x-www-form-urlencoded", []byte(form.Encode()), nil)
	case "webhook":
		payload, _ := json.Marshal(map[string]interface{}{"source": "wrm", "version": AppVersion, "sent_at": time.Now().UTC().Format(time.RFC3339),
			"title": title, "text": body, "events": msgs})
		h := map[string]string{}
		if sec := ch.get("hmac_secret"); sec != "" {
			m := hmac.New(sha256.New, []byte(sec))
			m.Write(payload)
			h["X-WRM-Signature"] = "sha256=" + hex.EncodeToString(m.Sum(nil))
		}
		return postBody(ch.get("url"), "application/json", payload, h)
	}
	return fmt.Errorf("unknown channel type %q", ch.Kind)
}

func postJSON(u string, v interface{}, headers map[string]string, redact string) error {
	b, _ := json.Marshal(v)
	err := postBody(u, "application/json", b, headers)
	if err != nil && redact == "telegram" {
		// The bot token is part of the URL: never show it in an error.
		if i := strings.Index(u, "/bot"); i >= 0 {
			err = fmt.Errorf("%s", strings.ReplaceAll(err.Error(), u[i:], "/bot•••"))
		}
	}
	return err
}

func postBody(u, contentType string, body []byte, headers map[string]string) error {
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("invalid address")
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "WRM-PRO/"+AppVersion)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := notifyHTTPClient.Do(req)
	if err != nil {
		// url.Error repeats the URL, which may hold a secret (webhook URLs, bot tokens).
		if ue, ok := err.(*url.Error); ok {
			err = ue.Err
		}
		return fmt.Errorf("%s: %v", req.URL.Host, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := strings.TrimSpace(string(data))
		var j struct {
			Description string `json:"description"`
			Message     string `json:"message"`
			Error       string `json:"error"`
			Errors      []string
		}
		if json.Unmarshal(data, &j) == nil {
			for _, s := range append([]string{j.Description, j.Message, j.Error}, j.Errors...) {
				if s != "" {
					msg = s
					break
				}
			}
		}
		return fmt.Errorf("%s answered HTTP %d %s", req.URL.Host, resp.StatusCode, truncateStr(msg, 200))
	}
	return nil
}

// sendSMTP sends a plain text e-mail (STARTTLS, implicit TLS or plain; AUTH only over TLS
// or to localhost, as net/smtp enforces).
func sendSMTP(ch notifyChannel, subject, body string) error {
	host := ch.get("host")
	port := ch.get("port")
	addr := net.JoinHostPort(host, port)
	from := ch.get("from")
	to := splitAddrs(ch.get("to"))
	if len(to) == 0 {
		return fmt.Errorf("no recipient")
	}
	tlsCfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	var conn net.Conn
	var err error
	dialer := &net.Dialer{Timeout: notifyHTTPTimeout}
	if ch.get("security") == "tls" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	} else {
		conn, err = dialer.Dial("tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("SMTP server %s: %v", addr, err)
	}
	conn.SetDeadline(time.Now().Add(2 * notifyHTTPTimeout))
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("SMTP server %s: %v", addr, err)
	}
	defer c.Close()
	if ch.get("security") == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("SMTP server %s does not offer STARTTLS (choose another security setting)", addr)
		}
		if err := c.StartTLS(tlsCfg); err != nil {
			return fmt.Errorf("STARTTLS: %v", err)
		}
	}
	if user := ch.get("username"); user != "" {
		// PLAIN, or LOGIN for servers that offer only that (e.g. Exchange receive connectors).
		var auth smtp.Auth = smtp.PlainAuth("", user, ch.get("password"), host)
		if ok, mechs := c.Extension("AUTH"); ok {
			m := " " + strings.ToUpper(mechs) + " "
			if !strings.Contains(m, " PLAIN ") && strings.Contains(m, " LOGIN ") {
				auth = &smtpLoginAuth{user: user, pass: ch.get("password"), host: host}
			}
		}
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("SMTP login: %v", err)
		}
	}
	if err := c.Mail(bareAddr(from)); err != nil {
		return fmt.Errorf("SMTP MAIL FROM: %v", err)
	}
	for _, a := range to {
		if err := c.Rcpt(bareAddr(a)); err != nil {
			return fmt.Errorf("SMTP RCPT TO %s: %v", a, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA: %v", err)
	}
	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%d.wrm@%s>\r\nMIME-Version: 1.0\r\n"+
		"Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\nAuto-Submitted: auto-generated\r\n\r\n",
		from, strings.Join(bareAddrs(to), ", "), mime.QEncoding.Encode("utf-8", subject), time.Now().Format(time.RFC1123Z), time.Now().UnixNano(), hostOnly(host))
	qp := quotedprintable.NewWriter(&msg)
	qp.Write([]byte(strings.ReplaceAll(body, "\n", "\r\n")))
	qp.Close()
	if _, err := w.Write(msg.Bytes()); err != nil {
		return fmt.Errorf("SMTP: %v", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("SMTP: %v", err)
	}
	return c.Quit()
}

// smtpLoginAuth implements AUTH LOGIN, with the same rule as smtp.PlainAuth: the password
// only goes over TLS or to the local machine.
type smtpLoginAuth struct{ user, pass, host string }

func (a *smtpLoginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS && a.host != "localhost" && a.host != "127.0.0.1" && a.host != "::1" {
		return "", nil, fmt.Errorf("unencrypted connection")
	}
	if server.Name != a.host {
		return "", nil, fmt.Errorf("wrong host name")
	}
	return "LOGIN", nil, nil
}

func (a *smtpLoginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	if strings.Contains(strings.ToLower(string(fromServer)), "user") {
		return []byte(a.user), nil
	}
	return []byte(a.pass), nil
}

func bareAddrs(list []string) []string {
	out := make([]string, len(list))
	for i, a := range list {
		out[i] = bareAddr(a)
	}
	return out
}

// slackEscape escapes the characters Slack (and Mattermost / Rocket.Chat) use for links and
// mentions, and breaks @channel / @here / @all / @everyone.
func slackEscape(s string) string {
	s = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
	return slackMentionRe.ReplaceAllString(s, "@\u200b$1")
}

var slackMentionRe = regexp.MustCompile(`(?i)@(channel|here|all|everyone)\b`)

func bareAddr(a string) string {
	a = strings.TrimSpace(a)
	if i := strings.LastIndexByte(a, '<'); i >= 0 && strings.HasSuffix(a, ">") {
		return a[i+1 : len(a)-1]
	}
	return a
}

// ─── EVENT SOURCES ───────────────────────────────────

// notifyStatusChange is called by the status monitor for a connection that went down or
// came back (only its owner is told).
func notifyStatusChange(userID int, name, host, state, errText string, downFor time.Duration) {
	event := "status." + state
	notifyUser(userID, event, func(lang string) (string, string) {
		vars := map[string]string{"name": name, "host": host, "error": errText}
		if state == "up" {
			vars["after"] = ""
			if downFor > 0 {
				vars["after"] = notifyText(lang, "status.up.after", map[string]string{"dur": shortDuration(downFor)})
			}
		}
		if vars["error"] == "" {
			vars["error"] = "?"
		}
		return notifyText(lang, event, vars), notifyText(lang, event+".body", vars)
	})
}

func shortDuration(d time.Duration) string {
	d = d.Round(time.Second)
	switch {
	case d < time.Minute:
		return strconv.Itoa(int(d.Seconds())) + " s"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + " min"
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h %d min", int(d.Hours()), int(d.Minutes())%60)
	}
	return strconv.Itoa(int(d.Hours()/24)) + " d"
}

func getNotifyState(key string) string {
	var v string
	db.QueryRow(`SELECT value FROM notify_state WHERE key=?`, key).Scan(&v)
	return v
}

func setNotifyState(key, value string) {
	db.Exec(`INSERT INTO notify_state (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
}

var (
	credReminderMu     sync.Mutex
	credDueRemindEvery = 7 * 24 * time.Hour
	credIncRemindEvery = 24 * time.Hour
)

// checkCredentialReminders tells owners about credentials whose rotation is due (again
// every 7 days while it stays due) or incomplete (again every day).
func checkCredentialReminders() {
	credReminderMu.Lock()
	defer credReminderMu.Unlock()
	rows, err := db.Query(`SELECT ` + credentialCols + ` FROM credentials`)
	if err != nil {
		return
	}
	var list []credential
	for rows.Next() {
		if c, err := scanCredential(rows); err == nil {
			list = append(list, c)
		}
	}
	rows.Close()
	now := time.Now().UTC()
	// state: "<marker>|<last reminder>"; a new marker (a new rotation) starts over. The state
	// is written only when a reminder was queued, so subscribing later still gets one.
	due := func(key, marker string, every time.Duration) bool {
		st := getNotifyState(key)
		if i := strings.LastIndexByte(st, '|'); i >= 0 && st[:i] == marker {
			if t, err := time.Parse(time.RFC3339, st[i+1:]); err == nil && now.Sub(t) < every {
				return false
			}
		}
		return true
	}
	sent := func(key, marker string) { setNotifyState(key, marker+"|"+now.Format(time.RFC3339)) }
	for _, c := range list {
		dueKey, incKey := "cred_due:"+strconv.Itoa(c.ID), "cred_incomplete:"+strconv.Itoa(c.ID)
		vars := func(lang string) map[string]string {
			last := c.RotatedAt
			if last == "" {
				last = c.CreatedAt
			}
			if t, err := time.Parse(time.RFC3339, last); err == nil {
				last = t.Format("2006-01-02")
			}
			st := c.RotationStatus
			if st == "" {
				st = "?"
			}
			return map[string]string{"name": c.Name, "user": c.Username, "date": last, "days": strconv.Itoa(c.RotateDays), "status": truncateStr(st, 300)}
		}
		if c.RotationDue && !c.Incomplete {
			if due(dueKey, c.RotatedAt, credDueRemindEvery) && notifyUser(c.OwnerID, "credential.rotation_due", func(lang string) (string, string) {
				v := vars(lang)
				return notifyText(lang, "credential.rotation_due", v), notifyText(lang, "credential.rotation_due.body", v)
			}) {
				sent(dueKey, c.RotatedAt)
			}
		} else {
			db.Exec(`DELETE FROM notify_state WHERE key=?`, dueKey)
		}
		if c.Incomplete {
			if due(incKey, c.RotationStatus, credIncRemindEvery) && notifyUser(c.OwnerID, "credential.rotation_incomplete", func(lang string) (string, string) {
				v := vars(lang)
				return notifyText(lang, "credential.rotation_incomplete", v), notifyText(lang, "credential.rotation_incomplete.body", v)
			}) {
				sent(incKey, c.RotationStatus)
			}
		} else {
			db.Exec(`DELETE FROM notify_state WHERE key=?`, incKey)
		}
	}
}

func runCredentialReminders() {
	time.Sleep(30 * time.Second)
	for {
		if settingBool("notifications_enabled") {
			checkCredentialReminders()
		}
		time.Sleep(time.Hour)
	}
}

// ─── API ─────────────────────────────────────────────

// GET    /api/admin/notify/channels            channels (no secrets) and channel types
// POST   /api/admin/notify/channels            create
// PUT    /api/admin/notify/channels/{id}       change (empty secret fields keep the stored value)
// DELETE /api/admin/notify/channels/{id}
// POST   /api/admin/notify/channels/{id}/test  {address} send a test message now
func apiAdminNotifyHandler(w http.ResponseWriter, r *http.Request) {
	adminID, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/notify/channels"), "/")
	if rest == "" {
		switch r.Method {
		case http.MethodGet:
			list := loadNotifyChannels()
			for i := range list {
				db.QueryRow(`SELECT COUNT(DISTINCT user_id) FROM notify_subscriptions WHERE channel_id=?`, list[i].ID).Scan(&list[i].Subscribers)
			}
			jsonOK(w, map[string]interface{}{"channels": list, "kinds": notifyKinds, "events": notifyEvents, "enabled": settingBool("notifications_enabled")})
		case http.MethodPost:
			saveNotifyChannel(w, r, adminID, nil)
		default:
			jsonError(w, "Method not allowed", 405)
		}
		return
	}
	parts := strings.SplitN(rest, "/", 2)
	id, err := strconv.Atoi(parts[0])
	if err != nil {
		jsonError(w, "Bad ID", 400)
		return
	}
	ch, err := loadNotifyChannel(id)
	if err != nil {
		jsonError(w, "Not found", 404)
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	switch {
	case action == "" && r.Method == http.MethodPut:
		saveNotifyChannel(w, r, adminID, &ch)
	case action == "" && r.Method == http.MethodDelete:
		db.Exec(`DELETE FROM notify_subscriptions WHERE channel_id=?`, id)
		db.Exec(`DELETE FROM notify_pending WHERE channel_id=?`, id)
		db.Exec(`DELETE FROM notify_channels WHERE id=?`, id)
		auditLog(r, adminID, "admin.notify_channel_deleted", ch.Name, map[string]interface{}{"channel_id": id, "kind": ch.Kind})
		jsonOK(w, map[string]bool{"ok": true})
	case action == "test" && r.Method == http.MethodPost:
		var in struct {
			Address string `json:"address"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		if err := validateNotifyAddress(ch, in.Address); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		err := sendTestNotification(ch, in.Address, loadNotifyPrefs(adminID).Lang)
		details := map[string]interface{}{"channel_id": id, "kind": ch.Kind, "ok": err == nil}
		if err != nil {
			details["error"] = err.Error()
		}
		auditLog(r, adminID, "admin.notify_channel_tested", ch.Name, details)
		if err != nil {
			db.Exec(`UPDATE notify_channels SET last_error=? WHERE id=?`, truncateStr(time.Now().UTC().Format(time.RFC3339)+": "+err.Error(), 500), id)
			jsonError(w, err.Error(), 502)
			return
		}
		db.Exec(`UPDATE notify_channels SET last_sent_at=?, last_error='' WHERE id=?`, time.Now().UTC().Format(time.RFC3339), id)
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

func sendTestNotification(ch notifyChannel, addr, lang string) error {
	title := notifyText(lang, "test", nil)
	body := notifyText(lang, "test.body", map[string]string{"channel": ch.Name})
	return sendNotification(ch, addr, title, body, []notifyMessage{{Event: "test", Title: title, Body: body, Time: time.Now().UTC()}})
}

func saveNotifyChannel(w http.ResponseWriter, r *http.Request, adminID int, cur *notifyChannel) {
	var in struct {
		Name        string            `json:"name"`
		Kind        string            `json:"kind"`
		Enabled     *bool             `json:"enabled"`
		UserAddress bool              `json:"user_address"`
		Config      map[string]string `json:"config"`
		Clear       []string          `json:"clear"` // secret fields to remove
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	name := truncateStr(strings.TrimSpace(in.Name), 120)
	if name == "" {
		jsonError(w, "Name is required", 400)
		return
	}
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	if cur != nil && kind == "" {
		kind = cur.Kind
	}
	k, ok := notifyKindFor(kind)
	if !ok {
		jsonError(w, "Unknown channel type", 400)
		return
	}
	if k.Address == "" {
		in.UserAddress = false
	}
	cfg := map[string]string{}
	changed := []string{}
	clear := map[string]bool{}
	for _, c := range in.Clear {
		clear[c] = true
	}
	for _, f := range k.Fields {
		v := strings.TrimSpace(in.Config[f.Name])
		old := ""
		if cur != nil && cur.Kind == kind {
			old = cur.cfg[f.Name]
		}
		if f.Secret && v == "" && !clear[f.Name] {
			v = old // the browser never gets secrets: empty keeps them
		}
		if v != old {
			changed = append(changed, f.Name)
		}
		cfg[f.Name] = v
	}
	if err := validateNotifyConfig(kind, cfg, in.UserAddress); err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	stored := map[string]string{}
	for _, f := range k.Fields {
		if f.Secret {
			if cfg[f.Name] != "" {
				stored[f.Name] = encryptValue(cfg[f.Name])
			}
		} else if cfg[f.Name] != "" {
			stored[f.Name] = cfg[f.Name]
		}
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	} else if cur != nil {
		enabled = cur.Enabled
	}
	now := time.Now().UTC().Format(time.RFC3339)
	raw := string(jsonMarshal(stored))
	var id int
	if cur == nil {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM notify_channels`).Scan(&n)
		if n >= maxNotifyChannels {
			jsonError(w, fmt.Sprintf("At most %d channels", maxNotifyChannels), 400)
			return
		}
		res, err := db.Exec(`INSERT INTO notify_channels (name, kind, config, enabled, user_address, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
			name, kind, raw, boolInt(enabled), boolInt(in.UserAddress), now, now)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		nid, _ := res.LastInsertId()
		id = int(nid)
		auditLog(r, adminID, "admin.notify_channel_created", name, map[string]interface{}{"channel_id": id, "kind": kind, "enabled": enabled, "user_address": in.UserAddress})
	} else {
		id = cur.ID
		if _, err := db.Exec(`UPDATE notify_channels SET name=?, kind=?, config=?, enabled=?, user_address=?, updated_at=? WHERE id=?`,
			name, kind, raw, boolInt(enabled), boolInt(in.UserAddress), now, id); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		for f, same := range map[string]bool{"name": name == cur.Name, "kind": kind == cur.Kind, "enabled": enabled == cur.Enabled, "user_address": in.UserAddress == cur.UserAddress} {
			if !same {
				changed = append(changed, f)
			}
		}
		sort.Strings(changed)
		auditLog(r, adminID, "admin.notify_channel_updated", name, map[string]interface{}{"channel_id": id, "kind": kind, "changed": changed})
		if !in.UserAddress {
			db.Exec(`UPDATE notify_subscriptions SET address='' WHERE channel_id=?`, id)
		}
	}
	ch, _ := loadNotifyChannel(id)
	jsonOK(w, ch)
}

type notifySubscription struct {
	Event     string `json:"event"`
	ChannelID int    `json:"channel_id"`
	Address   string `json:"address"`
}

var notifyTestLimiter = struct {
	sync.Mutex
	last map[int]time.Time
}{last: map[int]time.Time{}}

// GET  /api/notify        channels I can use, my subscriptions and quiet hours
// PUT  /api/notify        {subscriptions: [...], prefs: {...}}
// POST /api/notify/test   {channel_id, address} a test message to me
func apiNotifyHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	switch {
	case r.URL.Path == "/api/notify" && r.Method == http.MethodGet:
		chans := []map[string]interface{}{}
		for _, ch := range loadNotifyChannels() {
			if !ch.Enabled {
				continue
			}
			k, _ := notifyKindFor(ch.Kind)
			addrField := ""
			if ch.UserAddress {
				addrField = k.Address
			}
			chans = append(chans, map[string]interface{}{"id": ch.ID, "name": ch.Name, "kind": ch.Kind, "user_address": ch.UserAddress, "address_field": addrField})
		}
		subs := []notifySubscription{}
		if rows, err := db.Query(`SELECT event, channel_id, address FROM notify_subscriptions WHERE user_id=? ORDER BY event, channel_id`, userID); err == nil {
			for rows.Next() {
				var s notifySubscription
				if rows.Scan(&s.Event, &s.ChannelID, &s.Address) == nil {
					subs = append(subs, s)
				}
			}
			rows.Close()
		}
		var pending int
		db.QueryRow(`SELECT COUNT(*) FROM notify_pending WHERE user_id=?`, userID).Scan(&pending)
		jsonOK(w, map[string]interface{}{"enabled": settingBool("notifications_enabled"), "events": notifyEvents, "channels": chans,
			"subscriptions": subs, "prefs": loadNotifyPrefs(userID), "pending": pending})
	case r.URL.Path == "/api/notify" && r.Method == http.MethodPut:
		var in struct {
			Subscriptions []notifySubscription `json:"subscriptions"`
			Prefs         *notifyPrefs         `json:"prefs"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in) != nil {
			jsonError(w, "Bad JSON", 400)
			return
		}
		if len(in.Subscriptions) > 200 {
			jsonError(w, "Too many subscriptions", 400)
			return
		}
		chans := map[int]notifyChannel{}
		for _, ch := range loadNotifyChannels() {
			if ch.Enabled {
				chans[ch.ID] = ch
			}
		}
		seen := map[string]bool{}
		var subs []notifySubscription
		for _, s := range in.Subscriptions {
			ch, ok := chans[s.ChannelID]
			if !ok || !validNotifyEvent(s.Event) {
				jsonError(w, "Unknown channel or event", 400)
				return
			}
			s.Address = strings.TrimSpace(s.Address)
			if err := validateNotifyAddress(ch, s.Address); err != nil {
				jsonError(w, ch.Name+": "+err.Error(), 400)
				return
			}
			if k, _ := notifyKindFor(ch.Kind); k.Address != "" && s.Address == "" && ch.get(k.Address) == "" {
				jsonError(w, ch.Name+": enter your "+k.Address+" (this channel has no default recipient)", 400)
				return
			}
			key := s.Event + "\x00" + strconv.Itoa(s.ChannelID)
			if !seen[key] {
				seen[key] = true
				subs = append(subs, s)
			}
		}
		if in.Prefs != nil {
			if err := in.Prefs.validate(); err != nil {
				jsonError(w, err.Error(), 400)
				return
			}
		}
		tx, err := db.Begin()
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		defer tx.Rollback()
		// Only the channels offered (enabled) are replaced: subscriptions to a channel that is
		// disabled for a while stay; those of deleted channels go.
		for id := range chans {
			tx.Exec(`DELETE FROM notify_subscriptions WHERE user_id=? AND channel_id=?`, userID, id)
		}
		tx.Exec(`DELETE FROM notify_subscriptions WHERE user_id=? AND channel_id NOT IN (SELECT id FROM notify_channels)`, userID)
		for _, s := range subs {
			tx.Exec(`INSERT INTO notify_subscriptions (user_id, event, channel_id, address) VALUES (?,?,?,?)`, userID, s.Event, s.ChannelID, s.Address)
		}
		if in.Prefs != nil {
			p := in.Prefs
			tx.Exec(`INSERT INTO notify_prefs (user_id, quiet_enabled, quiet_start, quiet_end, tz, lang) VALUES (?,?,?,?,?,?)
				ON CONFLICT(user_id) DO UPDATE SET quiet_enabled=excluded.quiet_enabled, quiet_start=excluded.quiet_start,
				quiet_end=excluded.quiet_end, tz=excluded.tz, lang=excluded.lang`, userID, boolInt(p.QuietEnabled), p.QuietStart, p.QuietEnd, p.TZ, p.Lang)
		}
		if err := tx.Commit(); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		events := []string{}
		for _, s := range subs {
			events = append(events, s.Event+"→"+chans[s.ChannelID].Name)
		}
		details := map[string]interface{}{"subscriptions": events}
		if in.Prefs != nil {
			details["quiet_hours"] = map[bool]string{true: in.Prefs.QuietStart + "–" + in.Prefs.QuietEnd + " " + in.Prefs.TZ, false: "off"}[in.Prefs.QuietEnabled]
		}
		auditLog(r, userID, "notify.settings_changed", "", details)
		jsonOK(w, map[string]bool{"ok": true})
	case r.URL.Path == "/api/notify/test" && r.Method == http.MethodPost:
		var in struct {
			ChannelID int    `json:"channel_id"`
			Address   string `json:"address"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		ch, err := loadNotifyChannel(in.ChannelID)
		if err != nil || !ch.Enabled {
			jsonError(w, "Channel not found", 404)
			return
		}
		if err := validateNotifyAddress(ch, in.Address); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		notifyTestLimiter.Lock()
		if time.Since(notifyTestLimiter.last[userID]) < 10*time.Second {
			notifyTestLimiter.Unlock()
			jsonError(w, "Please wait a moment before sending another test", 429)
			return
		}
		notifyTestLimiter.last[userID] = time.Now()
		notifyTestLimiter.Unlock()
		err = sendTestNotification(ch, in.Address, loadNotifyPrefs(userID).Lang)
		details := map[string]interface{}{"channel_id": ch.ID, "ok": err == nil}
		if err != nil {
			details["error"] = err.Error()
		}
		auditLog(r, userID, "notify.tested", ch.Name, details)
		if err != nil {
			jsonError(w, err.Error(), 502)
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Not found", 404)
	}
}

// notifySummary for the admin overview.
func notifySummary() map[string]int {
	var channels, enabled, subs, pending int
	db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(enabled),0) FROM notify_channels`).Scan(&channels, &enabled)
	db.QueryRow(`SELECT COUNT(DISTINCT user_id) FROM notify_subscriptions`).Scan(&subs)
	db.QueryRow(`SELECT COUNT(*) FROM notify_pending`).Scan(&pending)
	return map[string]int{"channels": channels, "enabled": enabled, "subscribers": subs, "pending": pending}
}
