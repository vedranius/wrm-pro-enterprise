// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ─── ADDON PLATFORM v2 ───────────────────────────────
//
// apiVersion 2 keeps the v1 format (a folder with addon.json, installed as a
// ZIP) and adds what a real tool needs: themes users pick in Appearance,
// instances (two Grafana windows with two configurations), a small server-side
// store per instance, windows remembered per user, and a stricter sandbox. A
// manifest without apiVersion is v1 and behaves as before. Unknown fields in a
// v2 manifest refuse the install, so a typo never silently grants nothing.
//
// Personal addons: any user (policy addon_personal) installs addons that only
// they see. They cannot carry server code; what reaches the network or a secret is
// approved by an administrator (later versions of the platform).

const (
	addonStoreDefaultKB = 64
	addonStoreMaxKB     = 1024
	addonStoreMaxWrite  = 32 << 10
	addonMaxThemes      = 16
	addonMaxInstances   = 50
	addonWindowMaxBytes = 4 << 10
)

var (
	addonThemeIDPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	addonTokenPattern    = regexp.MustCompile(`^--[a-z0-9-]{1,40}$`)
	addonStoreKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	addonInlineScript    = regexp.MustCompile(`(?is)<script\b([^>]*)>(.*?)</script\s*>`)
	addonScriptSrc       = regexp.MustCompile(`(?i)\bsrc\s*=`)
)

// addonTheme is one colour theme an addon offers in Settings → Appearance:
// CSS variables (tokens) and/or a stylesheet. Nothing is applied globally; a
// user picks it.
type addonTheme struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	NameHR string            `json:"name_hr,omitempty"`
	Mode   string            `json:"mode"`
	Tokens map[string]string `json:"tokens,omitempty"`
	CSS    string            `json:"css,omitempty"`
}

type addonSize struct {
	W int `json:"w"`
	H int `json:"h"`
}

// addonStore asks for the server-side store: quotaKb per instance (64 KB by
// default, 1 MB at most) and shared: one set of values per instance instead of
// one per user.
type addonStore struct {
	QuotaKB int  `json:"quotaKb,omitempty"`
	Shared  bool `json:"shared,omitempty"`
}

func (s *addonStore) quota() int {
	if s == nil || s.QuotaKB <= 0 {
		return addonStoreDefaultKB
	}
	return s.QuotaKB
}

// ── errors with codes ────────────────────────────────

// addonError carries a stable code (E_LIMIT_SIZE, E_MANIFEST_FIELD, …) so an
// AI that wrote the addon can repair it instead of guessing from prose.
type addonError struct {
	Code string
	Msg  string
}

func (e *addonError) Error() string { return e.Msg }

func addonErrf(code, format string, args ...interface{}) error {
	return &addonError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

var addonErrorHints = map[string]string{
	"E_MANIFEST":        "Fix addon.json as the message says; Settings → Addons → AI addon lists every field.",
	"E_MANIFEST_FIELD":  "apiVersion 2 rejects unknown fields: remove the field or correct its spelling.",
	"E_API_VERSION":     "Use apiVersion 2 (or leave it out for a v1 addon).",
	"E_MISSING_FILE":    "Add the file to the ZIP or remove it from addon.json.",
	"E_LIMIT_SIZE":      "Make the payload smaller.",
	"E_LIMIT_QUOTA":     "Delete keys you no longer need or ask for a larger store.quotaKb (1024 at most).",
	"E_LIMIT_COUNT":     "Remove instances that are no longer used.",
	"E_STORE_KEY":       "Keys are 1-64 characters: letters, digits, dot, dash, underscore; not starting with wrm.",
	"E_STORE_VALUE":     "Store JSON values only.",
	"E_STORE_READONLY":  "Shared values of a shared instance are written by an administrator.",
	"E_CAPABILITY":      "Declare the capability in addon.json (and apiVersion 2).",
	"E_DISABLED":        "Enable the addon in Settings → Addons.",
	"E_INTEGRITY":       "Install the addon again from its ZIP.",
	"E_DOWNGRADE":       "Install with allow_downgrade=1 if the older version is intended.",
	"E_PERSONAL_OFF":    "Ask an administrator to allow personal addons (addon_personal).",
	"E_PERSONAL_SERVER": "Remove the server block, or ask an administrator to install the addon for the organisation.",
	"E_NOT_FOUND":       "Reload the addon list; the instance or addon no longer exists.",
	"E_HTTP_REQUEST":    "Send an absolute http(s) URL, a JSON request and, for bytes, a base64 body with encoding: 'base64'.",
	"E_HTTP_ORIGIN":     "Add the site to http.origins in addon.json (exact scheme://host[:port]).",
	"E_HTTP_PATH":       "Add the path to http.paths (exact, or a prefix ending in *).",
	"E_HTTP_METHOD":     "Add the method to http.methods.",
	"E_HTTP_UNAPPROVED": "An administrator approves the addon's sites in Settings → Addons.",
	"E_HTTP_SCHEME":     "Use https, or ask an administrator to approve the site for the private network.",
	"E_HTTP_BLOCKED":    "The site resolves to a private or local address; an administrator can approve it for the private network.",
	"E_HTTP_REDIRECT":   "The site redirected too often or to a site or path that is not allowed; request the final URL.",
	"E_HTTP_SECRET":     "Name a secret listed in http.secrets.",
	"E_SECRET_MISSING":  "Enter the value in Settings → Addons → Instances → Secrets.",
	"E_HTTP_TIMEOUT":    "The site did not answer within 10 seconds; try again later or ask for less.",
	"E_HTTP_UPSTREAM":   "The connection to the site failed; check its address and certificate.",
	"E_HTTP_OFF":        "Ask an administrator to allow addons with web access (addon_http).",
	"E_LIMIT_RATE":      "Send fewer requests: 120 a minute and 20 at once per instance.",
}

// addonFail answers with {error, code, hint} for coded errors.
func addonFail(w http.ResponseWriter, err error, status int) {
	var ae *addonError
	if !errors.As(err, &ae) {
		jsonError(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": ae.Msg, "code": ae.Code, "hint": addonErrorHints[ae.Code]})
}

// ── manifest ─────────────────────────────────────────

// addonDecodeManifest reads addon.json: leniently for v1 (as before), strictly
// for apiVersion 2.
func addonDecodeManifest(raw []byte) (*addonManifest, error) {
	var peek map[string]json.RawMessage
	if err := json.Unmarshal(raw, &peek); err != nil {
		return nil, addonErrf("E_MANIFEST", "addon.json is not valid JSON: %v", err)
	}
	var m addonManifest
	v := 1
	if rv, ok := peek["apiVersion"]; ok {
		if json.Unmarshal(rv, &v) != nil || v < 1 {
			return nil, addonErrf("E_API_VERSION", "apiVersion must be 1 or 2")
		}
		if v > 2 {
			return nil, addonErrf("E_API_VERSION", "apiVersion %d is newer than this WRM supports (2)", v)
		}
	}
	if v < 2 {
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, addonErrf("E_MANIFEST", "addon.json is not valid JSON: %v", err)
		}
		return &m, nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "unknown field") {
			return nil, addonErrf("E_MANIFEST_FIELD", "addon.json: %s (not part of apiVersion 2 in this WRM)", strings.TrimPrefix(msg, "json: "))
		}
		return nil, addonErrf("E_MANIFEST", "addon.json is not valid: %v", err)
	}
	if dec.More() {
		return nil, addonErrf("E_MANIFEST", "addon.json holds more than one JSON value")
	}
	return &m, nil
}

func addonValidateUIv2(ui *addonUI) error {
	if len(ui.Title) > 80 {
		return fmt.Errorf("ui.title is longer than 80 characters")
	}
	for name, sz := range map[string]*addonSize{"ui.defaultSize": ui.DefaultSize, "ui.minSize": ui.MinSize} {
		if sz != nil && (sz.W < 160 || sz.W > 4000 || sz.H < 120 || sz.H > 4000) {
			return fmt.Errorf("%s must be 160-4000 wide and 120-4000 high", name)
		}
	}
	if ui.DefaultSize != nil && ui.MinSize != nil && (ui.MinSize.W > ui.DefaultSize.W || ui.MinSize.H > ui.DefaultSize.H) {
		return fmt.Errorf("ui.minSize is larger than ui.defaultSize")
	}
	if (ui.DefaultSize != nil || ui.MinSize != nil) && ui.Panel == "" {
		return fmt.Errorf("ui.defaultSize and ui.minSize need ui.panel")
	}
	if len(ui.Themes) > addonMaxThemes {
		return fmt.Errorf("at most %d themes are allowed", addonMaxThemes)
	}
	ids := map[string]bool{}
	for i, th := range ui.Themes {
		where := fmt.Sprintf("ui.themes[%d]", i)
		if !addonThemeIDPattern.MatchString(th.ID) {
			return fmt.Errorf("%s.id must be 1-32 characters: lowercase letters, digits and dash", where)
		}
		if ids[th.ID] {
			return fmt.Errorf("%s.id %q is used twice", where, th.ID)
		}
		ids[th.ID] = true
		if n := strings.TrimSpace(th.Name); n == "" || len(n) > 60 || len(th.NameHR) > 60 {
			return fmt.Errorf("%s needs a name of 1-60 characters", where)
		}
		if th.Mode != "light" && th.Mode != "dark" {
			return fmt.Errorf("%s.mode must be light or dark", where)
		}
		if len(th.Tokens) == 0 && th.CSS == "" {
			return fmt.Errorf("%s declares neither tokens nor css", where)
		}
		if len(th.Tokens) > 200 {
			return fmt.Errorf("%s has more than 200 tokens", where)
		}
		for k, v := range th.Tokens {
			if !addonTokenPattern.MatchString(k) {
				return fmt.Errorf("%s token %q must be a CSS variable such as --bg", where, k)
			}
			if !addonSafeTokenValue(v) {
				return fmt.Errorf("%s token %s has a value that is not allowed", where, k)
			}
		}
		if th.CSS != "" && (!addonSafeRel(th.CSS) || !strings.HasSuffix(strings.ToLower(th.CSS), ".css")) {
			return fmt.Errorf("%s.css must be a relative .css file", where)
		}
	}
	return nil
}

// addonSafeTokenValue keeps a token a plain value (a colour, a length, a font
// list): nothing that could close the declaration or load something.
func addonSafeTokenValue(v string) bool {
	if v == "" || len(v) > 200 {
		return false
	}
	low := strings.ToLower(v)
	if strings.ContainsAny(v, ";{}<>\\\x00\r\n") {
		return false
	}
	for _, bad := range []string{"url(", "expression", "@import", "javascript:", "image-set(", "image("} {
		if strings.Contains(low, bad) {
			return false
		}
	}
	return true
}

func addonValidateV2(m *addonManifest, seen map[string]bool) error {
	if m.APIVersion < 2 {
		if seen["store"] || seen["notify"] || m.Store != nil {
			return fmt.Errorf("the store and notify capabilities need \"apiVersion\": 2")
		}
		return nil
	}
	if m.Store != nil && !seen["store"] {
		return fmt.Errorf("a store block requires the store capability")
	}
	if m.Store != nil && (m.Store.QuotaKB < 0 || m.Store.QuotaKB > addonStoreMaxKB) {
		return fmt.Errorf("store.quotaKb must be 1-%d", addonStoreMaxKB)
	}
	if (seen["store"] || seen["notify"]) && (m.UI == nil || m.UI.Panel == "") {
		return fmt.Errorf("the store and notify capabilities are used from a panel: declare ui.panel")
	}
	return nil
}

// ── permissions ──────────────────────────────────────

// addonPermissions is the permission sheet of a manifest as stable atoms. The
// interface turns them into sentences; approving the sheet is enabling the
// addon, and an update is compared atom by atom.
func addonPermissions(m *addonManifest) []string {
	out := []string{}
	if m.UI != nil {
		if m.UI.Panel != "" {
			out = append(out, "panel")
		}
		n := len(m.UI.Themes)
		if m.UI.Theme != "" {
			n++
		}
		if n > 0 {
			out = append(out, "themes:"+strconv.Itoa(n))
		}
	}
	has := map[string]bool{}
	for _, c := range m.Capabilities {
		has[c] = true
	}
	if has["store"] {
		a := "store:" + strconv.Itoa(m.Store.quota())
		if m.Store != nil && m.Store.Shared {
			a += ":shared"
		}
		out = append(out, a)
	}
	if has["notify"] {
		out = append(out, "notify")
	}
	out = append(out, addonHTTPPermissions(m.HTTP)...)
	if m.Server != nil {
		out = append(out, "server:"+strings.Join(m.Server.Exports, ","))
	}
	return out
}

// addonPermsAdded lists the atoms of next that base does not already cover:
// a smaller store, fewer themes or fewer exports need no new approval.
func addonPermsAdded(next, base []string) []string {
	added := []string{}
	for _, a := range next {
		if !addonPermCovered(a, base) {
			added = append(added, a)
		}
	}
	return added
}

func addonPermCovered(a string, base []string) bool {
	kind, val, _ := strings.Cut(a, ":")
	for _, b := range base {
		if a == b {
			return true
		}
		bk, bv, _ := strings.Cut(b, ":")
		if bk != kind {
			continue
		}
		switch kind {
		case "themes":
			return true
		case "store":
			an, ashared, _ := strings.Cut(val, ":")
			bn, bshared, _ := strings.Cut(bv, ":")
			ai, _ := strconv.Atoi(an)
			bi, _ := strconv.Atoi(bn)
			if ai <= bi && ashared == bshared {
				return true
			}
		case "http.methods", "http.paths":
			if addonHTTPPermCovered(kind, val, bv) {
				return true
			}
		case "server":
			ok := true
			have := map[string]bool{}
			for _, e := range strings.Split(bv, ",") {
				have[e] = true
			}
			for _, e := range strings.Split(val, ",") {
				ok = ok && have[e]
			}
			if ok {
				return true
			}
		}
	}
	return false
}

// addonVersionLess compares dotted versions (1.10.0 > 1.9.3); a part that is
// not a number is compared as text.
func addonVersionLess(a, b string) bool {
	pa := strings.Split(strings.TrimPrefix(a, "v"), ".")
	pb := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y string
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x == y {
			continue
		}
		xi, ex := strconv.Atoi(x)
		yi, ey := strconv.Atoi(y)
		if ex == nil && ey == nil {
			return xi < yi
		}
		if x == "" || y == "" {
			return x == ""
		}
		return x < y
	}
	return false
}

func addonPersonalAllowed(userID int) bool {
	v := getSetting("addon_personal")
	return v != "off" && policyAllows(userID, v)
}

// ── integrity and sandbox ────────────────────────────

// addonHashTree pins the SHA-256 of every file of an installed addon.
func addonHashTree(dir string) (map[string]string, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	out := map[string]string{}
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		f, err := root.Open(p)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return err
		}
		out[p] = hex.EncodeToString(h.Sum(nil))
		return nil
	})
	return out, err
}

// addonPanelCSP is the policy of an apiVersion 2 panel: sandboxed (opaque
// origin), no network of its own (connect-src 'none': requests go through the
// host), its own files only, and inline scripts allowed by hash so a single-file
// addon works while injected script does not.
func addonPanelCSP(html []byte) string {
	hashes := []string{}
	seen := map[string]bool{}
	for _, m := range addonInlineScript.FindAllSubmatch(html, 64) {
		if addonScriptSrc.Match(m[1]) {
			continue
		}
		sum := sha256.Sum256(m[2])
		h := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
		if !seen[h] {
			seen[h] = true
			hashes = append(hashes, h)
		}
	}
	script := "script-src 'self'"
	if len(hashes) > 0 {
		script += " " + strings.Join(hashes, " ")
	}
	return strings.Join([]string{
		"sandbox allow-scripts",
		"default-src 'none'",
		script,
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: blob:",
		"font-src 'self' data:",
		"media-src 'self' data: blob:",
		"connect-src 'none'",
		"form-action 'none'",
		"base-uri 'none'",
		"frame-ancestors 'self'",
	}, "; ")
}

// ── instances, store, windows ────────────────────────

func initAddonV2Schema() {
	ensureTable("addons_personal", `CREATE TABLE IF NOT EXISTS addons_personal (
		owner_id INTEGER NOT NULL,
		id TEXT NOT NULL,
		version TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 0,
		manifest TEXT NOT NULL DEFAULT '',
		source TEXT NOT NULL DEFAULT '',
		checksum TEXT NOT NULL DEFAULT '',
		installed_at TEXT NOT NULL DEFAULT '',
		installed_by TEXT NOT NULL DEFAULT '',
		files TEXT NOT NULL DEFAULT '',
		approved TEXT NOT NULL DEFAULT '',
		pending TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (owner_id, id)
	)`)
	// One instance is one configuration of an addon: owner_id 0 is shared by
	// everybody who sees the addon (only organisation addons have those).
	ensureTable("addon_instances", `CREATE TABLE IF NOT EXISTS addon_instances (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		addon_owner INTEGER NOT NULL DEFAULT 0,
		addon_id TEXT NOT NULL,
		owner_id INTEGER NOT NULL DEFAULT 0,
		name TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		created_by TEXT NOT NULL DEFAULT ''
	)`)
	mustExec(`CREATE INDEX IF NOT EXISTS idx_addon_instances_addon ON addon_instances(addon_owner, addon_id)`)
	restoreAddonInstances()
	ensureTable("addon_store", `CREATE TABLE IF NOT EXISTS addon_store (
		instance_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL DEFAULT 0,
		key TEXT NOT NULL,
		value TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (instance_id, user_id, key)
	)`)
	ensureTable("addon_windows", `CREATE TABLE IF NOT EXISTS addon_windows (
		user_id INTEGER NOT NULL,
		instance_id INTEGER NOT NULL,
		state TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (user_id, instance_id)
	)`)
	initAddonHTTPSchema()
}

// restoreAddonInstances undoes the v13.9.0-v13.14.0 start-up bug that set addon_instances
// aside as addon_instances_old_<time> on every start (addon_store and addon_windows kept
// pointing at the old instance ids). When addon_instances is empty, the rows of the newest
// non-empty copy are put back with their ids; the copies themselves are kept.
func restoreAddonInstances() {
	var n int
	if db.QueryRow(`SELECT COUNT(*) FROM addon_instances`).Scan(&n) != nil || n > 0 {
		return
	}
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'addon\_instances\_old\_%' ESCAPE '\' ORDER BY name DESC`)
	if err != nil {
		return
	}
	var olds []string
	for rows.Next() {
		var t string
		if rows.Scan(&t) == nil {
			olds = append(olds, t)
		}
	}
	rows.Close()
	const cols = `id, addon_owner, addon_id, owner_id, name, created_at, created_by`
	for _, t := range olds {
		q := `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
		var c int
		if db.QueryRow(`SELECT COUNT(*) FROM `+q).Scan(&c) != nil || c == 0 {
			continue // empty, or not the layout this version knows
		}
		res, err := db.Exec(`INSERT INTO addon_instances (` + cols + `) SELECT ` + cols + ` FROM ` + q)
		if err != nil {
			log.Printf("Addon instances: restoring from %s failed: %v", t, err)
			return
		}
		got, _ := res.RowsAffected()
		log.Printf("Addon instances: restored %d instance(s) from %s (set aside by an earlier start)", got, t)
		return
	}
}

type addonInstance struct {
	ID      int             `json:"id"`
	Name    string          `json:"name"`
	Shared  bool            `json:"shared"`
	OwnerID int             `json:"owner_id,omitempty"`
	Owner   string          `json:"owner,omitempty"`
	Mine    bool            `json:"mine"`
	Window  json.RawMessage `json:"window,omitempty"`
	Created string          `json:"created_at"`
}

// addonVisible tells whether the user may use the addon at all.
func addonVisible(rec addonRecord, userID int, admin bool) bool {
	if rec.Personal {
		return rec.OwnerID == userID
	}
	return rec.Enabled || admin
}

func addonInstanceList(ref addonRef, userID int, all bool) []addonInstance {
	rows, err := db.Query(`SELECT i.id, i.name, i.owner_id, i.created_at, COALESCE(w.state, '')
		FROM addon_instances i LEFT JOIN addon_windows w ON w.instance_id=i.id AND w.user_id=?
		WHERE i.addon_owner=? AND i.addon_id=? AND (i.owner_id=0 OR i.owner_id=? OR ?=1) ORDER BY i.id`,
		userID, ref.Owner, ref.ID, userID, boolToInt(all))
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []addonInstance{}
	for rows.Next() {
		var in addonInstance
		var state string
		if rows.Scan(&in.ID, &in.Name, &in.OwnerID, &in.Created, &state) != nil {
			continue
		}
		in.Shared = in.OwnerID == 0
		in.Mine = in.OwnerID == userID
		if !in.Shared && !in.Mine {
			in.Owner = usernameOf(in.OwnerID)
		}
		if state != "" {
			in.Window = json.RawMessage(state)
		}
		out = append(out, in)
	}
	return out
}

func addonInstanceGet(ref addonRef, iid int) (addonInstance, bool) {
	var in addonInstance
	err := db.QueryRow(`SELECT id, name, owner_id, created_at FROM addon_instances WHERE id=? AND addon_owner=? AND addon_id=?`,
		iid, ref.Owner, ref.ID).Scan(&in.ID, &in.Name, &in.OwnerID, &in.Created)
	if err != nil {
		return in, false
	}
	in.Shared = in.OwnerID == 0
	return in, true
}

func addonForgetInstances(ref addonRef) {
	rows, err := db.Query(`SELECT id FROM addon_instances WHERE addon_owner=? AND addon_id=?`, ref.Owner, ref.ID)
	if err != nil {
		return
	}
	ids := []int{}
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		addonForgetInstance(id)
	}
}

func addonForgetInstance(iid int) {
	db.Exec(`DELETE FROM addon_store WHERE instance_id=?`, iid)
	db.Exec(`DELETE FROM addon_windows WHERE instance_id=?`, iid)
	db.Exec(`DELETE FROM addon_secrets WHERE instance_id=?`, iid)
	db.Exec(`DELETE FROM addon_instances WHERE id=?`, iid)
}

// addonStoreScope is whose values a request reads and writes: the instance
// owner's for a personal instance; for a shared instance one set for everybody
// (store.shared, written by an administrator) or one set per user.
func addonStoreScope(rec addonRecord, in addonInstance, userID int, admin bool) (scope int, writable bool) {
	if !in.Shared {
		return in.OwnerID, in.OwnerID == userID
	}
	if rec.Store != nil && rec.Store.Shared {
		return 0, admin
	}
	return userID, true
}

func addonStoreValues(iid, scope int) (map[string]json.RawMessage, int) {
	out := map[string]json.RawMessage{}
	used := 0
	rows, err := db.Query(`SELECT key, value FROM addon_store WHERE instance_id=? AND user_id=? ORDER BY key`, iid, scope)
	if err != nil {
		return out, 0
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil {
			out[k] = json.RawMessage(v)
			used += len(k) + len(v)
		}
	}
	return out, used
}

func addonHasCap(rec addonRecord, c string) bool {
	for _, x := range rec.Capabilities {
		if x == c {
			return true
		}
	}
	return false
}

// addonInstancesHandler serves /api/addons/{key}/instances[/{id}[/store[/{key}]|/window|/export|/import]].
func addonInstancesHandler(w http.ResponseWriter, r *http.Request, userID int, admin bool, ref addonRef, rest []string) {
	rec, ok := addonGetRef(ref)
	if !ok || !addonVisible(rec, userID, admin) {
		addonFail(w, addonErrf("E_NOT_FOUND", "Addon not found"), 404)
		return
	}
	if rec.APIVersion < 2 || rec.UI == nil || rec.UI.Panel == "" {
		addonFail(w, addonErrf("E_CAPABILITY", "Instances are for apiVersion 2 addons with a panel"), 400)
		return
	}
	if len(rest) == 0 {
		switch r.Method {
		case http.MethodGet:
			jsonOK(w, map[string]interface{}{"instances": addonInstanceList(ref, userID, admin && r.URL.Query().Get("all") == "1")})
		case http.MethodPost:
			addonInstanceCreate(w, r, userID, admin, ref, rec)
		default:
			jsonError(w, "Method not allowed", 405)
		}
		return
	}
	iid, err := strconv.Atoi(rest[0])
	in, found := addonInstanceGet(ref, iid)
	// Another user's instance is private: an administrator may only remove it.
	if err != nil || !found || (!in.Shared && in.OwnerID != userID && !(admin && len(rest) == 1 && r.Method == http.MethodDelete)) {
		addonFail(w, addonErrf("E_NOT_FOUND", "Instance not found"), 404)
		return
	}
	in.Mine = in.OwnerID == userID
	manage := in.Mine || (in.Shared && admin) || (admin && r.Method == http.MethodDelete)
	if len(rest) == 1 {
		switch r.Method {
		case http.MethodPatch:
			if !manage {
				jsonError(w, "Only the owner or an administrator changes this instance", 403)
				return
			}
			var req struct {
				Name string `json:"name"`
			}
			if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req) != nil {
				jsonError(w, "Invalid request", 400)
				return
			}
			name := strings.TrimSpace(req.Name)
			if name == "" || len(name) > 40 {
				jsonError(w, "A name of 1-40 characters is required", 400)
				return
			}
			db.Exec(`UPDATE addon_instances SET name=? WHERE id=?`, name, in.ID)
			jsonOK(w, map[string]bool{"ok": true})
		case http.MethodDelete:
			if !manage {
				jsonError(w, "Only the owner or an administrator removes this instance", 403)
				return
			}
			addonForgetInstance(in.ID)
			auditLog(r, userID, "addon.instance_removed", ref.Key(), map[string]interface{}{"instance": in.ID, "name": in.Name})
			jsonOK(w, map[string]bool{"ok": true})
		default:
			jsonError(w, "Method not allowed", 405)
		}
		return
	}
	switch rest[1] {
	case "window":
		addonWindowSave(w, r, userID, in)
		return
	case "secrets":
		addonSecretsHandler(w, r, userID, admin, rec, in, rest[2:])
		return
	case "http":
		addonHTTPHandler(w, r, userID, admin, rec, in, rest[2:])
		return
	case "store", "export", "import":
	default:
		jsonError(w, "Not found", 404)
		return
	}
	if !rec.Enabled {
		addonFail(w, addonErrf("E_DISABLED", "The addon is disabled"), 409)
		return
	}
	if !addonHasCap(rec, "store") {
		addonFail(w, addonErrf("E_CAPABILITY", "The addon does not declare the store capability"), 403)
		return
	}
	scope, writable := addonStoreScope(rec, in, userID, admin)
	quota := rec.Store.quota() << 10
	switch rest[1] {
	case "store":
		addonStoreHandler(w, r, in, scope, writable, quota, rest[2:])
	case "export":
		if r.Method != http.MethodGet {
			jsonError(w, "Method not allowed", 405)
			return
		}
		values, _ := addonStoreValues(in.ID, scope)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%d.json"`, rec.ID, in.ID))
		jsonOK(w, map[string]interface{}{"format": "wrm-addon-config", "addon": rec.ID, "version": rec.Version,
			"instance": in.Name, "exported_at": time.Now().UTC().Format(time.RFC3339), "values": values})
	case "import":
		if r.Method != http.MethodPost {
			jsonError(w, "Method not allowed", 405)
			return
		}
		if !writable {
			addonFail(w, addonErrf("E_STORE_READONLY", "These values are written by an administrator"), 403)
			return
		}
		var req struct {
			Format string                     `json:"format"`
			Addon  string                     `json:"addon"`
			Values map[string]json.RawMessage `json:"values"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, int64(quota)+64<<10)).Decode(&req); err != nil || req.Format != "wrm-addon-config" {
			jsonError(w, "Not a WRM addon configuration file", 400)
			return
		}
		if req.Addon != rec.ID {
			jsonError(w, fmt.Sprintf("This file belongs to the addon %q", req.Addon), 400)
			return
		}
		total := 0
		for k, v := range req.Values {
			if err := addonStoreCheck(k, v); err != nil {
				addonFail(w, err, 400)
				return
			}
			total += len(k) + len(v)
		}
		if total > quota {
			addonFail(w, addonErrf("E_LIMIT_QUOTA", "The file holds %d bytes; the store allows %d", total, quota), 413)
			return
		}
		tx, err := db.Begin()
		if err != nil {
			jsonError(w, "Cannot save", 500)
			return
		}
		tx.Exec(`DELETE FROM addon_store WHERE instance_id=? AND user_id=?`, in.ID, scope)
		now := time.Now().UTC().Format(time.RFC3339)
		for k, v := range req.Values {
			tx.Exec(`INSERT INTO addon_store (instance_id, user_id, key, value, updated_at) VALUES (?,?,?,?,?)`, in.ID, scope, k, string(v), now)
		}
		if tx.Commit() != nil {
			jsonError(w, "Cannot save", 500)
			return
		}
		auditLog(r, userID, "addon.config_imported", ref.Key(), map[string]interface{}{"instance": in.ID, "keys": len(req.Values)})
		jsonOK(w, map[string]interface{}{"ok": true, "keys": len(req.Values)})
	}
}

func addonInstanceCreate(w http.ResponseWriter, r *http.Request, userID int, admin bool, ref addonRef, rec addonRecord) {
	var req struct {
		Name   string `json:"name"`
		Shared bool   `json:"shared"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req) != nil {
		jsonError(w, "Invalid request", 400)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 40 {
		jsonError(w, "A name of 1-40 characters is required", 400)
		return
	}
	if !rec.Enabled {
		addonFail(w, addonErrf("E_DISABLED", "The addon is disabled"), 409)
		return
	}
	owner := userID
	if req.Shared {
		if rec.Personal || !admin {
			jsonError(w, "Only an administrator creates shared instances of an organisation addon", 403)
			return
		}
		owner = 0
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM addon_instances WHERE addon_owner=? AND addon_id=? AND owner_id=?`, ref.Owner, ref.ID, owner).Scan(&n)
	if n >= addonMaxInstances {
		addonFail(w, addonErrf("E_LIMIT_COUNT", "At most %d instances of one addon", addonMaxInstances), 409)
		return
	}
	res, err := db.Exec(`INSERT INTO addon_instances (addon_owner, addon_id, owner_id, name, created_at, created_by) VALUES (?,?,?,?,?,?)`,
		ref.Owner, ref.ID, owner, name, time.Now().UTC().Format(time.RFC3339), usernameOf(userID))
	if err != nil {
		jsonError(w, "Cannot save", 500)
		return
	}
	id, _ := res.LastInsertId()
	auditLog(r, userID, "addon.instance_created", ref.Key(), map[string]interface{}{"instance": id, "name": name, "shared": owner == 0})
	jsonOK(w, map[string]interface{}{"ok": true, "id": id})
}

func addonStoreCheck(key string, value json.RawMessage) error {
	if !addonStoreKeyPattern.MatchString(key) || strings.HasPrefix(strings.ToLower(key), "wrm.") {
		return addonErrf("E_STORE_KEY", "Invalid key %q", key)
	}
	if len(value) > addonStoreMaxWrite {
		return addonErrf("E_LIMIT_SIZE", "A value is larger than 32 KB")
	}
	if !json.Valid(value) {
		return addonErrf("E_STORE_VALUE", "The value of %q is not JSON", key)
	}
	return nil
}

func addonStoreHandler(w http.ResponseWriter, r *http.Request, in addonInstance, scope int, writable bool, quota int, rest []string) {
	if len(rest) == 0 {
		if r.Method != http.MethodGet {
			jsonError(w, "Method not allowed", 405)
			return
		}
		values, used := addonStoreValues(in.ID, scope)
		jsonOK(w, map[string]interface{}{"values": values, "used": used, "quota": quota, "writable": writable})
		return
	}
	key := rest[0]
	if len(rest) != 1 || !addonStoreKeyPattern.MatchString(key) || strings.HasPrefix(strings.ToLower(key), "wrm.") {
		addonFail(w, addonErrf("E_STORE_KEY", "Invalid key"), 400)
		return
	}
	switch r.Method {
	case http.MethodGet:
		var v string
		err := db.QueryRow(`SELECT value FROM addon_store WHERE instance_id=? AND user_id=? AND key=?`, in.ID, scope, key).Scan(&v)
		if err == sql.ErrNoRows {
			jsonOK(w, map[string]interface{}{"key": key, "value": nil})
			return
		}
		jsonOK(w, map[string]interface{}{"key": key, "value": json.RawMessage(v)})
	case http.MethodPut:
		if !writable {
			addonFail(w, addonErrf("E_STORE_READONLY", "These values are written by an administrator"), 403)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, addonStoreMaxWrite+1))
		if err != nil {
			jsonError(w, "Invalid request", 400)
			return
		}
		if err := addonStoreCheck(key, body); err != nil {
			addonFail(w, err, 400)
			return
		}
		var used, mine int
		db.QueryRow(`SELECT COALESCE(SUM(LENGTH(key)+LENGTH(value)),0) FROM addon_store WHERE instance_id=? AND user_id=?`, in.ID, scope).Scan(&used)
		db.QueryRow(`SELECT COALESCE(LENGTH(key)+LENGTH(value),0) FROM addon_store WHERE instance_id=? AND user_id=? AND key=?`, in.ID, scope, key).Scan(&mine)
		if used-mine+len(key)+len(body) > quota {
			addonFail(w, addonErrf("E_LIMIT_QUOTA", "The store of this instance is full (%d of %d bytes)", used, quota), 413)
			return
		}
		if _, err := db.Exec(`INSERT INTO addon_store (instance_id, user_id, key, value, updated_at) VALUES (?,?,?,?,?)
			ON CONFLICT(instance_id, user_id, key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
			in.ID, scope, key, string(body), time.Now().UTC().Format(time.RFC3339)); err != nil {
			jsonError(w, "Cannot save", 500)
			return
		}
		jsonOK(w, map[string]interface{}{"ok": true, "used": used - mine + len(key) + len(body)})
	case http.MethodDelete:
		if !writable {
			addonFail(w, addonErrf("E_STORE_READONLY", "These values are written by an administrator"), 403)
			return
		}
		db.Exec(`DELETE FROM addon_store WHERE instance_id=? AND user_id=? AND key=?`, in.ID, scope, key)
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}

// addonWindowSave remembers where the user keeps the window of an instance
// ({open, l, t, w, h, min, snap}: fractions of the canvas), so it comes back
// after a reload, a restart or on another device.
func addonWindowSave(w http.ResponseWriter, r *http.Request, userID int, in addonInstance) {
	switch r.Method {
	case http.MethodPut:
		body, err := io.ReadAll(io.LimitReader(r.Body, addonWindowMaxBytes+1))
		var obj map[string]interface{}
		if err != nil || len(body) > addonWindowMaxBytes || json.Unmarshal(body, &obj) != nil || obj == nil {
			jsonError(w, "The window state must be a JSON object of 4 KB at most", 400)
			return
		}
		if _, err := db.Exec(`INSERT INTO addon_windows (user_id, instance_id, state, updated_at) VALUES (?,?,?,?)
			ON CONFLICT(user_id, instance_id) DO UPDATE SET state=excluded.state, updated_at=excluded.updated_at`,
			userID, in.ID, string(body), time.Now().UTC().Format(time.RFC3339)); err != nil {
			jsonError(w, "Cannot save", 500)
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
	case http.MethodDelete:
		db.Exec(`DELETE FROM addon_windows WHERE user_id=? AND instance_id=?`, userID, in.ID)
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Method not allowed", 405)
	}
}
