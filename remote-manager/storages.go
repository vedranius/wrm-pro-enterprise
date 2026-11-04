// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── OBJECT STORAGE (Storages, S3) ────────────────────────────────────────────────────────
//
// A storage is an S3-compatible endpoint (Hetzner, DigitalOcean, AWS, MinIO, Backblaze) that
// belongs to a user. It is deliberately not a Connection: it never reaches the terminal, the
// tunnels, the jump hosts, the deploy code or the status monitor. The design lives in
// docs/rfcs/0008-storages-s3.md and the work in docs/ROADMAP-STORAGES.md.
//
// Capabilities are separate: reading is always allowed, saving an object needs allow_write,
// adding a new object needs allow_upload and deleting needs allow_delete. They are enforced
// here, on the server, never only in the UI.

const (
	storageDefaultMaxObjectMB = 16
	storageMaxObjectMB        = 128
	storageDefaultEditKB      = 2
	storageMaxEditKB          = 64
	storageMaxName            = 120
	storageMaxKey             = 1024
)

// Storage is one S3-compatible endpoint. Secret is the encrypted secret key: it is never
// serialised (json:"-") and never leaves the server.
type Storage struct {
	ID            int      `json:"id"`
	UserID        int      `json:"-"`
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	Endpoint      string   `json:"endpoint"`
	Region        string   `json:"region"`
	Bucket        string   `json:"bucket"`
	Prefix        string   `json:"prefix"`
	PathStyle     bool     `json:"path_style"`
	AccessKey     string   `json:"access_key"`
	Secret        string   `json:"-"`
	SecretSet     bool     `json:"secret_set"`
	CAFingerprint string   `json:"ca_fingerprint,omitempty"`
	SSE           string   `json:"sse,omitempty"`
	AllowWrite    bool     `json:"allow_write"`
	AllowDelete   bool     `json:"allow_delete"`
	AllowUpload   bool     `json:"allow_upload"`
	MaxObjectMB   int      `json:"max_object_mb"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
	Tags          []string `json:"tags,omitempty"`
	// Status is filled in by the background check (HeadBucket), never stored in the database.
	StatusOK  *bool  `json:"status_ok,omitempty"`
	StatusErr string `json:"status_err,omitempty"`
	StatusAt  string `json:"status_at,omitempty"`
}

// storageCapabilities answers what this user and this storage allow.
func (s Storage) capabilities() map[string]bool {
	return map[string]bool{
		"read":   true,
		"write":  s.AllowWrite,
		"upload": s.AllowUpload,
		"delete": s.AllowDelete,
	}
}

// storageSchema is created on start; see ensureTable in initDB.
const storageSchema = `CREATE TABLE IF NOT EXISTS storages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		kind TEXT NOT NULL DEFAULT 's3',
		endpoint TEXT NOT NULL DEFAULT '',
		region TEXT NOT NULL DEFAULT '',
		bucket TEXT NOT NULL DEFAULT '',
		prefix TEXT NOT NULL DEFAULT '',
		path_style INTEGER NOT NULL DEFAULT 1,
		access_key TEXT NOT NULL DEFAULT '',
		secret TEXT NOT NULL DEFAULT '',
		ca_fingerprint TEXT NOT NULL DEFAULT '',
		sse TEXT NOT NULL DEFAULT '',
		allow_write INTEGER NOT NULL DEFAULT 1,
		allow_delete INTEGER NOT NULL DEFAULT 0,
		allow_upload INTEGER NOT NULL DEFAULT 0,
		max_object_mb INTEGER NOT NULL DEFAULT 16,
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT '')`

const storageShareSchema = `CREATE TABLE IF NOT EXISTS storage_shares (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		storage_id INTEGER NOT NULL,
		token_hash TEXT NOT NULL UNIQUE,
		prefix TEXT NOT NULL DEFAULT '',
		can_write INTEGER NOT NULL DEFAULT 0,
		can_delete INTEGER NOT NULL DEFAULT 0,
		expires_at TEXT NOT NULL DEFAULT '',
		created_by INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL DEFAULT '')`

// loadStorage returns a storage the user may see: their own, or any when they are an
// administrator (the same rule connections follow).
func loadStorage(id, userID int) (Storage, error) {
	var s Storage
	var secret string
	var pathStyle, w, d, u int
	err := db.QueryRow(`SELECT id, user_id, name, kind, endpoint, region, bucket, prefix, path_style,
		access_key, secret, ca_fingerprint, sse, allow_write, allow_delete, allow_upload, max_object_mb,
		created_at, updated_at FROM storages WHERE id=?`, id).
		Scan(&s.ID, &s.UserID, &s.Name, &s.Kind, &s.Endpoint, &s.Region, &s.Bucket, &s.Prefix, &pathStyle,
			&s.AccessKey, &secret, &s.CAFingerprint, &s.SSE, &w, &d, &u, &s.MaxObjectMB, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return s, err
	}
	if s.UserID != userID && !isAdminUser(userID) {
		return s, fmt.Errorf("not found")
	}
	s.PathStyle, s.AllowWrite, s.AllowDelete, s.AllowUpload = pathStyle == 1, w == 1, d == 1, u == 1
	s.Secret, s.SecretSet = secret, secret != ""
	if s.MaxObjectMB <= 0 {
		s.MaxObjectMB = storageDefaultMaxObjectMB
	}
	return s, nil
}

func storagesForUser(userID int) []Storage {
	rows, err := db.Query(`SELECT id FROM storages WHERE user_id=? ORDER BY name COLLATE NOCASE`, userID)
	if err != nil {
		return []Storage{}
	}
	var ids []int
	for rows.Next() {
		var id int
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	out := make([]Storage, 0, len(ids))
	for _, id := range ids {
		if s, err := loadStorage(id, userID); err == nil {
			if ok, msg, at := storageStatusFor(id); at != "" {
				s.StatusOK, s.StatusErr, s.StatusAt = &ok, msg, at
			}
			out = append(out, s)
		}
	}
	return out
}

// storageStatus is the last HeadBucket result per storage. It is memory only: a restart
// simply shows "not checked yet".
var storageStatus = struct {
	sync.Mutex
	m map[int]storageStatusEntry
}{m: map[int]storageStatusEntry{}}

type storageStatusEntry struct {
	OK  bool
	Err string
	At  time.Time
}

func setStorageStatus(id int, err error) {
	e := storageStatusEntry{OK: err == nil, At: time.Now().UTC()}
	if err != nil {
		e.Err = err.Error()
	}
	storageStatus.Lock()
	storageStatus.m[id] = e
	storageStatus.Unlock()
}

func storageStatusFor(id int) (bool, string, string) {
	storageStatus.Lock()
	defer storageStatus.Unlock()
	e, ok := storageStatus.m[id]
	if !ok {
		return false, "", ""
	}
	return e.OK, e.Err, e.At.Format(time.RFC3339)
}

// refreshStorageStatus asks every storage for its bucket, so the list shows which one is
// reachable before a user opens it.
func refreshStorageStatus() {
	rows, err := db.Query(`SELECT id, user_id FROM storages`)
	if err != nil {
		return
	}
	type pair struct{ id, userID int }
	var all []pair
	for rows.Next() {
		var p pair
		if rows.Scan(&p.id, &p.userID) == nil {
			all = append(all, p)
		}
	}
	rows.Close()
	for i, p := range all {
		if i >= 200 {
			break
		}
		s, err := loadStorage(p.id, p.userID)
		if err != nil {
			continue
		}
		c, err := s.clientFor()
		if err != nil {
			setStorageStatus(s.ID, err)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		err = c.headBucket(ctx)
		cancel()
		setStorageStatus(s.ID, err)
	}
}

// storageStatusLoop keeps the status fresh without blocking a request.
func storageStatusLoop() {
	time.Sleep(45 * time.Second)
	for {
		if settingBool("storages_enabled") {
			refreshStorageStatus()
		}
		time.Sleep(5 * time.Minute)
	}
}

// normalizePrefix makes a prefix safe and canonical: no leading or trailing slash, no empty,
// "." or ".." segments. "" means the whole bucket and is refused separately.
func normalizePrefix(raw string) (string, error) {
	p := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	p = strings.Trim(p, "/")
	if p == "" {
		return "", nil
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("the prefix must not contain empty, '.' or '..' segments")
		}
	}
	return p, nil
}

// storageKeyPrefix is the prefix with its trailing slash, ready to prepend to a listing.
func storageKeyPrefix(s Storage) string {
	if s.Prefix == "" {
		return ""
	}
	return s.Prefix + "/"
}

// validStorageKey accepts a relative object key. It must stay inside the storage prefix, so
// absolute paths, backslashes, control characters and traversal are refused before any
// request is signed.
func validStorageKey(key string) error {
	if key == "" {
		return fmt.Errorf("the object key is empty")
	}
	if len(key) > storageMaxKey {
		return fmt.Errorf("the object key is longer than %d characters", storageMaxKey)
	}
	if strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		return fmt.Errorf("the object key must be a relative path")
	}
	if strings.ContainsAny(key, "\x00\n\r\t") {
		return fmt.Errorf("the object key contains a control character")
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("the object key must not contain empty, '.' or '..' segments")
		}
	}
	return nil
}

// validBucketName follows the S3 rules closely enough to catch typos before a request.
func validBucketName(b string) bool {
	if len(b) < 3 || len(b) > 63 {
		return false
	}
	if strings.HasPrefix(b, ".") || strings.HasSuffix(b, ".") || strings.Contains(b, "..") {
		return false
	}
	for _, r := range b {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func storageEndpoint(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return "", fmt.Errorf("enter the endpoint as https://host (no credentials in the address)")
	}
	if err := guardEgressURL(raw); err != nil {
		return "", err
	}
	return raw, nil
}

func storageFingerprint(raw string) (string, error) {
	f := strings.ToLower(strings.TrimSpace(raw))
	f = strings.TrimPrefix(f, "sha256:")
	f = strings.NewReplacer(":", "", " ", "").Replace(f)
	if f == "" {
		return "", nil
	}
	if len(f) != 64 || strings.Trim(f, "0123456789abcdef") != "" {
		return "", fmt.Errorf("the certificate fingerprint must be 64 hex characters (SHA-256)")
	}
	return f, nil
}

// validateStorage normalises and checks a storage from the browser. The secret is optional on
// update: an empty one keeps the stored key.
func validateStorage(s *Storage, userID int) error {
	s.Name = truncateStr(strings.TrimSpace(s.Name), storageMaxName)
	if s.Name == "" {
		return fmt.Errorf("give the storage a name")
	}
	if s.Kind == "" {
		s.Kind = "s3"
	}
	if s.Kind != "s3" {
		return fmt.Errorf("only S3-compatible storage is supported")
	}
	ep, err := storageEndpoint(s.Endpoint)
	if err != nil {
		return err
	}
	s.Endpoint = ep
	s.Region = truncateStr(strings.TrimSpace(s.Region), 64)
	if !validBucketName(strings.TrimSpace(s.Bucket)) {
		return fmt.Errorf("the bucket name is not valid (3-63 characters: a-z, 0-9, '-', '.')")
	}
	s.Bucket = strings.TrimSpace(s.Bucket)
	p, err := normalizePrefix(s.Prefix)
	if err != nil {
		return err
	}
	s.Prefix = p
	s.AccessKey = truncateStr(strings.TrimSpace(s.AccessKey), 256)
	if s.AccessKey == "" {
		return fmt.Errorf("enter the access key")
	}
	if s.Secret != "" && len(s.Secret) > 512 {
		return fmt.Errorf("the secret key is too long")
	}
	if s.CAFingerprint, err = storageFingerprint(s.CAFingerprint); err != nil {
		return err
	}
	if s.SSE != "" && s.SSE != "AES256" && s.SSE != "aws:kms" {
		return fmt.Errorf("the server-side encryption must be empty, AES256 or aws:kms")
	}
	if s.MaxObjectMB <= 0 {
		s.MaxObjectMB = storageDefaultMaxObjectMB
	}
	if s.MaxObjectMB > storageMaxObjectMB {
		return fmt.Errorf("at most %d MB per object", storageMaxObjectMB)
	}
	var n int
	db.QueryRow(`SELECT COUNT(1) FROM storages WHERE user_id=? AND name=? AND id<>?`, userID, s.Name, s.ID).Scan(&n)
	if n > 0 {
		return fmt.Errorf("a storage named %q already exists", s.Name)
	}
	return nil
}

func storageAuditDetails(s Storage) map[string]interface{} {
	return map[string]interface{}{
		"endpoint": s.Endpoint, "bucket": s.Bucket, "prefix": s.Prefix,
		"read": true, "write": s.AllowWrite, "upload": s.AllowUpload, "delete": s.AllowDelete,
		"path_style": s.PathStyle,
	}
}

// clientFor builds the S3 client of a storage from its decrypted secret.
func (s Storage) clientFor() (*s3Client, error) {
	return s.clientForSecret(decryptValue(s.Secret))
}

// clientForSecret builds the client from a plaintext secret; the secret is only held in
// memory for the lifetime of the request. A freshly entered storage is preflighted this way
// before it is encrypted and stored.
func (s Storage) clientForSecret(secret string) (*s3Client, error) {
	if secret == "" {
		return nil, fmt.Errorf("the secret key is missing; edit the storage and enter it again")
	}
	return newS3Client(s3Config{
		Endpoint: s.Endpoint, Region: s.Region, Bucket: s.Bucket, Prefix: s.Prefix,
		PathStyle: s.PathStyle, AccessKey: s.AccessKey, SecretKey: secret,
		CAFingerprint: s.CAFingerprint, SSE: s.SSE,
	})
}

// requireStorage authenticates the request, resolves {id} from the path and enforces a
// capability. It answers the request itself when anything is wrong.
func requireStorage(w http.ResponseWriter, r *http.Request, capability string) (Storage, int, bool) {
	var zero Storage
	userID, ok := requireAuth(w, r)
	if !ok {
		return zero, 0, false
	}
	if !settingBool("storages_enabled") {
		jsonError(w, "Storages are turned off by the administrator", http.StatusForbidden)
		return zero, 0, false
	}
	id, err := strconv.Atoi(strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/storages/"), "/"), "/")[0])
	if err != nil {
		jsonError(w, "Not found", http.StatusNotFound)
		return zero, 0, false
	}
	s, err := loadStorage(id, userID)
	if err != nil {
		jsonError(w, "Not found", http.StatusNotFound)
		return zero, 0, false
	}
	caps := s.capabilities()
	if !caps[capability] {
		auditLog(r, userID, "storage.denied", s.Name, map[string]interface{}{"capability": capability, "path": r.URL.Path})
		jsonError(w, capabilityMessage(capability), http.StatusForbidden)
		return zero, 0, false
	}
	if !mcpRateOK(fmt.Sprintf("storage:%d:%d", userID, s.ID), 120, time.Minute) {
		jsonError(w, "Too many storage operations; wait a moment", http.StatusTooManyRequests)
		return zero, 0, false
	}
	return s, userID, true
}

// storageExport is the on-the-wire shape of a storage in a configuration export. The secret is
// only present when the operator asked for a secrets export (password and step-up gated).
type storageExport struct {
	Storage
	Secret string `json:"secret,omitempty"`
}

// exportStorages lists the user's storages for the configuration export.
func exportStorages(userID int, withSecrets bool) []storageExport {
	list := storagesForUser(userID)
	out := make([]storageExport, 0, len(list))
	for _, s := range list {
		e := storageExport{Storage: s}
		e.SecretSet = s.SecretSet
		if withSecrets {
			e.Secret = decryptValue(s.Secret)
		}
		e.Secret = strings.TrimSpace(e.Secret)
		out = append(out, e)
	}
	return out
}

// importStorages adds the storages of an imported configuration. A storage whose name already
// exists is skipped, and nothing is preflighted: an import must work while the endpoint is
// temporarily unreachable.
func importStorages(userID int, in []storageExport) (added, skipped int) {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, e := range in {
		s := e.Storage
		s.Secret = e.Secret
		if err := validateStorage(&s, userID); err != nil {
			skipped++
			continue
		}
		var n int
		db.QueryRow(`SELECT COUNT(1) FROM storages WHERE user_id=? AND name=?`, userID, s.Name).Scan(&n)
		if n > 0 {
			skipped++
			continue
		}
		stored := ""
		if s.Secret != "" {
			stored = encryptValue(s.Secret)
		}
		if _, err := db.Exec(`INSERT INTO storages (user_id, name, kind, endpoint, region, bucket, prefix, path_style,
			access_key, secret, ca_fingerprint, sse, allow_write, allow_delete, allow_upload, max_object_mb, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			userID, s.Name, s.Kind, s.Endpoint, s.Region, s.Bucket, s.Prefix, boolInt(s.PathStyle),
			s.AccessKey, stored, s.CAFingerprint, s.SSE, boolInt(s.AllowWrite), boolInt(s.AllowDelete),
			boolInt(s.AllowUpload), s.MaxObjectMB, now, now); err != nil {
			skipped++
			continue
		}
		added++
	}
	return added, skipped
}

// s3ErrorStatus maps a provider error to the answer the browser should see, so a stale etag
// is a 409 the editor can explain and a missing key is a 404.
func s3ErrorStatus(err error) int {
	if e, ok := err.(*s3Error); ok {
		switch e.Code {
		case "PreconditionFailed":
			return http.StatusConflict
		case "NoSuchKey":
			return http.StatusNotFound
		case "NoSuchBucket":
			return http.StatusNotFound
		case "AccessDenied", "InvalidAccessKeyId", "SignatureDoesNotMatch":
			return http.StatusForbidden
		}
	}
	return http.StatusBadGateway
}

func capabilityMessage(capability string) string {
	switch capability {
	case "write":
		return "This storage does not allow changes"
	case "upload":
		return "This storage does not allow new files"
	case "delete":
		return "This storage does not allow deleting files"
	}
	return "Not allowed"
}

// apiStoragesHandler serves /api/storages and /api/storages/{id}[/...].
func apiStoragesHandler(w http.ResponseWriter, r *http.Request) {
	if !settingBool("storages_enabled") {
		if _, ok := requireAuth(w, r); !ok {
			return
		}
		jsonError(w, "Storages are turned off by the administrator", http.StatusForbidden)
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/storages"), "/")
	if rest == "" {
		apiStoragesRoot(w, r)
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 1 {
		apiStorageByID(w, r)
		return
	}
	switch parts[1] {
	case "test":
		apiStorageTest(w, r)
	case "objects":
		apiStorageObjects(w, r)
	case "object":
		apiStorageObject(w, r)
	case "dir":
		apiStorageDir(w, r)
	case "move":
		apiStorageMove(w, r)
	case "text":
		apiStorageText(w, r)
	default:
		jsonError(w, "Not found", http.StatusNotFound)
	}
}

func apiStoragesRoot(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		jsonOK(w, storagesForUser(userID))
	case http.MethodPost:
		var in struct {
			Storage
			Secret string `json:"secret"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
			jsonError(w, "Invalid request", http.StatusBadRequest)
			return
		}
		s := in.Storage
		s.Secret = in.Secret
		if err := validateStorage(&s, userID); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if s.Secret == "" {
			jsonError(w, "enter the secret key", http.StatusBadRequest)
			return
		}
		if err := storagePreflight(s, s.Secret); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		res, err := db.Exec(`INSERT INTO storages (user_id, name, kind, endpoint, region, bucket, prefix, path_style,
			access_key, secret, ca_fingerprint, sse, allow_write, allow_delete, allow_upload, max_object_mb, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			userID, s.Name, s.Kind, s.Endpoint, s.Region, s.Bucket, s.Prefix, boolInt(s.PathStyle),
			s.AccessKey, encryptValue(s.Secret), s.CAFingerprint, s.SSE, boolInt(s.AllowWrite), boolInt(s.AllowDelete),
			boolInt(s.AllowUpload), s.MaxObjectMB, now, now)
		if err != nil {
			jsonError(w, "Server error", http.StatusInternalServerError)
			return
		}
		id, _ := res.LastInsertId()
		s.ID, s.UserID, s.SecretSet = int(id), userID, true
		s.Secret = ""
		s.CreatedAt, s.UpdatedAt = now, now
		auditLog(r, userID, "storage.created", s.Name, storageAuditDetails(s))
		jsonOK(w, s)
	default:
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func apiStorageByID(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	id, err := strconv.Atoi(strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/storages/"), "/"), "/")[0])
	if err != nil {
		jsonError(w, "Not found", http.StatusNotFound)
		return
	}
	s, err := loadStorage(id, userID)
	if err != nil {
		jsonError(w, "Not found", http.StatusNotFound)
		return
	}
	if s.UserID != userID && !isAdminUser(userID) {
		jsonError(w, "Not found", http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		jsonOK(w, s)
	case http.MethodPut:
		var in struct {
			Storage
			Secret string `json:"secret"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
			jsonError(w, "Invalid request", http.StatusBadRequest)
			return
		}
		upd := in.Storage
		upd.ID = s.ID
		if err := validateStorage(&upd, s.UserID); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		plain := in.Secret
		if plain == "" {
			plain = decryptValue(s.Secret)
		}
		if err := storagePreflight(Storage{Endpoint: upd.Endpoint, Region: upd.Region, Bucket: upd.Bucket,
			PathStyle: upd.PathStyle, AccessKey: upd.AccessKey, CAFingerprint: upd.CAFingerprint}, plain); err != nil {
			jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if in.Secret != "" {
			s.Secret = encryptValue(in.Secret)
			s.SecretSet = true
		}
		now := time.Now().UTC().Format(time.RFC3339)
		_, err := db.Exec(`UPDATE storages SET name=?, endpoint=?, region=?, bucket=?, prefix=?, path_style=?,
			access_key=?, secret=?, ca_fingerprint=?, sse=?, allow_write=?, allow_delete=?, allow_upload=?,
			max_object_mb=?, updated_at=? WHERE id=?`,
			upd.Name, upd.Endpoint, upd.Region, upd.Bucket, upd.Prefix, boolInt(upd.PathStyle), upd.AccessKey, s.Secret,
			upd.CAFingerprint, upd.SSE, boolInt(upd.AllowWrite), boolInt(upd.AllowDelete), boolInt(upd.AllowUpload),
			upd.MaxObjectMB, now, s.ID)
		if err != nil {
			jsonError(w, "Server error", http.StatusInternalServerError)
			return
		}
		upd.UserID, upd.Secret, upd.SecretSet = s.UserID, "", s.SecretSet
		upd.CreatedAt, upd.UpdatedAt = s.CreatedAt, now
		auditLog(r, userID, "storage.updated", upd.Name, storageAuditDetails(upd))
		jsonOK(w, upd)
	case http.MethodDelete:
		if s.UserID != userID && !isAdminUser(userID) {
			jsonError(w, "Not found", http.StatusNotFound)
			return
		}
		db.Exec(`DELETE FROM storage_shares WHERE storage_id=?`, s.ID)
		if _, err := db.Exec(`DELETE FROM storages WHERE id=?`, s.ID); err != nil {
			jsonError(w, "Server error", http.StatusInternalServerError)
			return
		}
		auditLog(r, userID, "storage.deleted", s.Name, storageAuditDetails(s))
		jsonOK(w, map[string]bool{"ok": true})
	default:
		jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// apiStorageTest re-checks the credentials and the bucket without saving.
func apiStorageTest(w http.ResponseWriter, r *http.Request) {
	s, userID, ok := requireStorage(w, r, "read")
	if !ok {
		return
	}
	out := map[string]interface{}{"ok": true}
	if s.CAFingerprint != "" {
		out["tls_fingerprint"] = s.CAFingerprint
	}
	if err := storagePreflight(s, s.Secret); err != nil {
		setStorageStatus(s.ID, err)
		auditLog(r, userID, "storage.test_failed", s.Name, map[string]interface{}{"error": err.Error()})
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	setStorageStatus(s.ID, nil)
	jsonOK(w, out)
}

// storagePreflight proves the endpoint, the bucket and the keys before anything is stored: a
// storage that cannot list its bucket would only fail later, inside the file window.
func storagePreflight(s Storage, secret string) error {
	c, err := s.clientForSecret(secret)
	if err != nil {
		return err
	}
	if err := c.headBucket(rctx()); err != nil {
		return err
	}
	if _, err := c.list(rctx(), "", "", 1); err != nil {
		return err
	}
	return nil
}
