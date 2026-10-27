// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ─── SELF-UPDATE FROM GITHUB RELEASES ────────────────
//
// runUpdateChecker asks the GitHub releases API of update_repo (default this project) once a
// day for the latest release; administrators can check on demand. The request carries no
// user data: only the repository path and a fixed User-Agent. HTTP_PROXY / HTTPS_PROXY or the
// update_proxy setting are used when set.
//
// An update (policy self_update, administrators only, never in a container) downloads the
// release file for this OS / architecture next to the binary, verifies its SHA-256 against
// the release's SHA256SUMS.txt (refused when the file or its line is missing or different),
// runs it with -version, and replaces the binary atomically: Unix renames the new file over
// the old one (a hard link keeps the old one as <binary>.previous); Windows renames the running
// .exe to <binary>.old, puts the new file in place and, on the next start, keeps .old as
// .previous. Then WRM restarts (service manager or in place). <binary>.previous gives a
// one-click rollback. Every step is audited (system.update_*). The database is only touched
// by the new version's normal additive migrations.

const (
	defaultUpdateRepo = "vedranius/wrm-pro-enterprise"
	checksumsAsset    = "SHA256SUMS.txt"
	updateUserAgent   = "WRM-PRO-update-check"
	maxUpdateBytes    = 512 << 20
	updateEvery       = 24 * time.Hour
)

// githubAPIBase is the GitHub API (a variable for tests).
var githubAPIBase = "https://api.github.com"

var updateRepoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}$`)

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type releaseInfo struct {
	Tag        string         `json:"tag_name"`
	Name       string         `json:"name"`
	Body       string         `json:"body"`
	HTMLURL    string         `json:"html_url"`
	Published  string         `json:"published_at"`
	Draft      bool           `json:"draft"`
	Prerelease bool           `json:"prerelease"`
	Assets     []releaseAsset `json:"assets"`
}

// updateInfo is the result of the last check (kept in app_settings as _update_state).
type updateInfo struct {
	Repo      string `json:"repo"`
	CheckedAt string `json:"checked_at"`
	Latest    string `json:"latest"`
	Name      string `json:"name"`
	Notes     string `json:"notes"`
	URL       string `json:"url"`
	Published string `json:"published_at"`
	Error     string `json:"error"`
}

type updateProgressInfo struct {
	State   string `json:"state"` // idle | downloading | verifying | installing | restarting | failed
	Version string `json:"version,omitempty"`
	Done    int64  `json:"done,omitempty"`
	Total   int64  `json:"total,omitempty"`
	Error   string `json:"error,omitempty"`
}

var updateMu = struct {
	sync.Mutex
	info     updateInfo
	loaded   bool
	progress updateProgressInfo
	busy     bool
}{progress: updateProgressInfo{State: "idle"}}

func updateRepo() string {
	if r := strings.TrimSpace(getSetting("update_repo")); updateRepoRe.MatchString(r) {
		return r
	}
	return defaultUpdateRepo
}

func lastUpdateInfo() updateInfo {
	updateMu.Lock()
	defer updateMu.Unlock()
	if !updateMu.loaded {
		json.Unmarshal([]byte(getInternal("update_state")), &updateMu.info)
		updateMu.loaded = true
	}
	return updateMu.info
}

func storeUpdateInfo(info updateInfo) {
	updateMu.Lock()
	updateMu.info, updateMu.loaded = info, true
	updateMu.Unlock()
	b, _ := json.Marshal(info)
	setInternal("update_state", string(b))
}

func setUpdateProgress(p updateProgressInfo) {
	updateMu.Lock()
	updateMu.progress = p
	updateMu.Unlock()
}

func updateHTTPClient() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = http.ProxyFromEnvironment
	if p := strings.TrimSpace(getSetting("update_proxy")); p != "" {
		if u, err := url.Parse(p); err == nil && u.Host != "" {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Transport: tr}
}

func githubGet(ctx context.Context, rawURL string, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", updateUserAgent)
	req.Header.Set("Accept", accept)
	if strings.HasPrefix(rawURL, githubAPIBase) {
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	resp, err := updateHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil, fmt.Errorf("not found (HTTP 404)")
		case http.StatusForbidden, http.StatusTooManyRequests:
			return nil, fmt.Errorf("GitHub refused the request (HTTP %d, rate limit?); try again later", resp.StatusCode)
		}
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

// fetchRelease reads the latest release ("" tag) or the release of a tag.
func fetchRelease(ctx context.Context, repo, tag string) (*releaseInfo, error) {
	u := githubAPIBase + "/repos/" + repo + "/releases/latest"
	if tag != "" {
		u = githubAPIBase + "/repos/" + repo + "/releases/tags/" + url.PathEscape(tag)
	}
	resp, err := githubGet(ctx, u, "application/vnd.github+json")
	if err != nil {
		if tag == "" && strings.Contains(err.Error(), "404") {
			return nil, fmt.Errorf("%s has no published release", repo)
		}
		return nil, err
	}
	defer resp.Body.Close()
	var rel releaseInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		return nil, fmt.Errorf("unexpected answer from GitHub: %v", err)
	}
	if _, ok := parseVersion(rel.Tag); !ok {
		return nil, fmt.Errorf("the release tag %q is not a version", rel.Tag)
	}
	return &rel, nil
}

// checkForUpdate asks GitHub for the latest release and remembers the answer.
func checkForUpdate(ctx context.Context) updateInfo {
	repo := updateRepo()
	info := updateInfo{Repo: repo, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	rel, err := fetchRelease(ctx, repo, "")
	if err != nil {
		prev := lastUpdateInfo()
		if prev.Repo == repo {
			info.Latest, info.Name, info.Notes, info.URL, info.Published = prev.Latest, prev.Name, prev.Notes, prev.URL, prev.Published
		}
		info.Error = err.Error()
	} else {
		info.Latest, info.Name, info.URL, info.Published = rel.Tag, rel.Name, rel.HTMLURL, rel.Published
		info.Notes = truncateStr(rel.Body, 20000)
	}
	storeUpdateInfo(info)
	return info
}

// runUpdateChecker checks once a day while update_check is on (also across restarts).
func runUpdateChecker() {
	time.Sleep(30 * time.Second)
	for {
		if settingBool("update_check") {
			last, _ := time.Parse(time.RFC3339, lastUpdateInfo().CheckedAt)
			if time.Since(last) >= updateEvery || lastUpdateInfo().Repo != updateRepo() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				info := checkForUpdate(ctx)
				cancel()
				if info.Error != "" {
					log.Printf("Update check: %s", info.Error)
				} else if compareSemver(info.Latest, AppVersion) > 0 {
					log.Printf("Update available: %s (running %s)", info.Latest, AppVersion)
				}
			}
		}
		time.Sleep(time.Hour)
	}
}

func updateAvailable(info updateInfo) bool {
	if _, ok := parseVersion(AppVersion); !ok || info.Latest == "" {
		return false
	}
	return compareSemver(info.Latest, AppVersion) > 0
}

func selfUpdateAllowed(userID int) bool {
	return getSetting("self_update") == "admins" && isAdminUser(userID)
}

// dirWritable checks that WRM may create files next to its binary.
func dirWritable(dir string) bool {
	f, err := os.CreateTemp(dir, ".wrm-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

// previousBinary is <binary>.previous with its version (probed, cached for a minute).
var previousCache = struct {
	sync.Mutex
	at      time.Time
	version string
}{}

func previousBinaryPath() string {
	exe, err := selfExecutable()
	if err != nil {
		return ""
	}
	return exe + ".previous"
}

func previousBinaryVersion() string {
	previousCache.Lock()
	defer previousCache.Unlock()
	if time.Since(previousCache.at) < time.Minute {
		return previousCache.version
	}
	previousCache.at, previousCache.version = time.Now(), ""
	p := previousBinaryPath()
	if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
		return ""
	}
	makeExecutable(p)
	if v, err := probeBinaryVersion(p); err == nil {
		if _, ok := parseVersion(v); ok {
			previousCache.version = v
		}
	}
	return previousCache.version
}

func forgetPreviousBinary() {
	previousCache.Lock()
	previousCache.at = time.Time{}
	previousCache.Unlock()
}

// cleanupReplacedBinary runs at start: the Windows .old file of the last update becomes the
// rollback copy, and interrupted downloads are removed.
func cleanupReplacedBinary() {
	exe, err := selfExecutable()
	if err != nil {
		return
	}
	if _, err := os.Stat(exe + ".old"); err == nil {
		os.Remove(exe + ".previous")
		if os.Rename(exe+".old", exe+".previous") != nil {
			os.Remove(exe + ".old")
		}
	}
	if old, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), ".wrm-update-*")); len(old) > 0 {
		for _, f := range old {
			os.Remove(f)
		}
	}
}

func dockerImage() string {
	owner, _, _ := strings.Cut(updateRepo(), "/")
	return "ghcr.io/" + strings.ToLower(owner) + "/wrm-pro"
}

// updateBlocked says why this installation cannot replace itself ("" = it can).
func updateBlocked() string {
	if os.Getenv("WRM_APPIMAGE") == "1" {
		return "appimage"
	}
	if inContainer() {
		return "docker"
	}
	exe, err := selfExecutable()
	if err != nil {
		return "no_binary"
	}
	if !dirWritable(filepath.Dir(exe)) {
		return "not_writable"
	}
	return ""
}

func updateStatus(userID int) map[string]interface{} {
	info := lastUpdateInfo()
	out := map[string]interface{}{
		"current": AppVersion, "latest": info.Latest, "available": updateAvailable(info), "name": info.Name,
		"notes": info.Notes, "url": info.URL, "published_at": info.Published, "checked_at": info.CheckedAt,
		"check_enabled": settingBool("update_check"), "docker": inContainer(),
	}
	if !isAdminUser(userID) {
		return out
	}
	updateMu.Lock()
	out["progress"] = updateMu.progress
	updateMu.Unlock()
	blocked := updateBlocked()
	labels := platformLabels(runtime.GOOS, runtime.GOARCH, buildGOARM)
	out["repo"], out["error"], out["service"], out["platform"] = info.Repo, info.Error, serviceMode, labels[0]
	out["can_update"] = selfUpdateAllowed(userID) && blocked == ""
	out["policy"] = getSetting("self_update")
	out["blocked"] = blocked
	out["docker_image"] = dockerImage()
	out["signature_keys"] = len(releaseTrustedKeys())
	if blocked == "" {
		if nb := newerLocalBinary(); nb != nil {
			out["local"] = nb
		}
		if v := previousBinaryVersion(); v != "" {
			out["rollback"] = v
		}
	}
	return out
}

// GET  /api/update            status (everyone: versions and notes; administrators: more)
// POST /api/update/check      check now (administrators)
// POST /api/update/apply      {version}: download, verify, replace, restart (policy self_update)
// POST /api/update/rollback   back to <binary>.previous (policy self_update)
// POST /api/update/restart    {local: true}: restart, to the newest binary in the folder (policy self_update)
func apiUpdateHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/update"), "/")
	if action == "" {
		if r.Method != http.MethodGet {
			jsonError(w, "Method not allowed", 405)
			return
		}
		jsonOK(w, updateStatus(userID))
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	if !isAdminUser(userID) {
		jsonError(w, "Administrators only", 403)
		return
	}
	switch action {
	case "check":
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		info := checkForUpdate(ctx)
		cancel()
		auditLog(r, userID, "system.update_check", info.Repo, map[string]string{"latest": info.Latest, "current": AppVersion, "error": info.Error})
		jsonOK(w, updateStatus(userID))
		return
	}
	if !selfUpdateAllowed(userID) {
		jsonError(w, "Self-update is turned off by the policy self_update.", 403)
		return
	}
	if b := updateBlocked(); b != "" {
		msg := map[string]string{
			"appimage":     "Stop WRM and download the new AppImage from the release page. Your data stays in the same location.",
			"docker":       "WRM runs in a container: update the image (docker pull / docker compose) instead.",
			"not_writable": "WRM cannot write to the folder of its binary.",
		}[b]
		if msg == "" {
			msg = "This installation cannot update itself."
		}
		jsonError(w, msg, 409)
		return
	}
	var body struct {
		Version string `json:"version"`
		Local   bool   `json:"local"`
	}
	json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)
	// The background steps keep the client address and headers for the audit log.
	ar := &http.Request{RemoteAddr: r.RemoteAddr, Header: r.Header.Clone()}
	switch action {
	case "apply":
		if _, ok := parseVersion(body.Version); !ok {
			jsonError(w, "Unknown version", 400)
			return
		}
		if compareSemver(body.Version, AppVersion) <= 0 {
			jsonError(w, "That version is not newer than "+AppVersion+".", 400)
			return
		}
		if !startUpdateJob(func(ctx context.Context) error { return applyUpdate(ctx, ar, userID, body.Version) }) {
			jsonError(w, "An update is already running.", 409)
			return
		}
		jsonOK(w, map[string]interface{}{"ok": true})
	case "rollback":
		v := previousBinaryVersion()
		if v == "" {
			jsonError(w, "There is no previous binary to roll back to.", 409)
			return
		}
		if !startUpdateJob(func(ctx context.Context) error { return rollbackUpdate(ar, userID, v) }) {
			jsonError(w, "An update is already running.", 409)
			return
		}
		jsonOK(w, map[string]interface{}{"ok": true})
	case "restart":
		target, version := "", AppVersion
		if body.Local {
			nb := newerLocalBinary()
			if nb == nil {
				jsonError(w, "There is no newer binary in the folder.", 409)
				return
			}
			target, version = nb.Path, nb.Version
		}
		auditLog(r, userID, "system.update_restart", version, map[string]string{"from": AppVersion, "to": version, "file": filepath.Base(target), "service": serviceMode})
		setUpdateProgress(updateProgressInfo{State: "restarting", Version: version})
		go func() {
			time.Sleep(time.Second)
			requestRestart(target)
		}()
		jsonOK(w, map[string]interface{}{"ok": true})
	default:
		jsonError(w, "Not found", 404)
	}
}

// startUpdateJob runs one update / rollback at a time.
func startUpdateJob(job func(ctx context.Context) error) bool {
	updateMu.Lock()
	if updateMu.busy {
		updateMu.Unlock()
		return false
	}
	updateMu.busy = true
	updateMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		err := job(ctx)
		updateMu.Lock()
		updateMu.busy = false
		if err != nil {
			updateMu.progress.State, updateMu.progress.Error = "failed", err.Error()
		}
		updateMu.Unlock()
	}()
	return true
}

// pickAsset finds the release file for this platform (own label first).
func pickAsset(rel *releaseInfo, labels []string) *releaseAsset {
	for _, l := range labels {
		name := releaseAssetName(rel.Tag, l)
		for i := range rel.Assets {
			if rel.Assets[i].Name == name {
				return &rel.Assets[i]
			}
		}
	}
	return nil
}

// parseChecksums reads "<sha256>  <file>" lines (sha256sum format, optional '*').
func parseChecksums(r io.Reader) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 2 || len(f[0]) != 64 {
			continue
		}
		if _, err := hex.DecodeString(f[0]); err != nil {
			continue
		}
		out[strings.TrimPrefix(f[1], "*")] = strings.ToLower(f[0])
	}
	return out
}

type countingWriter struct {
	n      int64
	report func(int64)
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	if c.report != nil {
		c.report(c.n)
	}
	return len(p), nil
}

// checkReleaseSignature verifies the release's SHA256SUMS.txt.sig. With a trusted release
// key a missing or wrong signature refuses the update; without one it returns
// "not checked" and the update relies on the SHA-256 alone.
func checkReleaseSignature(ctx context.Context, rel *releaseInfo, sumsData []byte) (string, error) {
	keys := releaseTrustedKeys()
	if len(keys) == 0 {
		return "not checked (no release key)", nil
	}
	var sig *releaseAsset
	for i := range rel.Assets {
		if rel.Assets[i].Name == checksumsAsset+releaseSignatureExt {
			sig = &rel.Assets[i]
		}
	}
	if sig == nil {
		return "", fmt.Errorf("the release %s is not signed (no %s); update refused", rel.Tag, checksumsAsset+releaseSignatureExt)
	}
	resp, err := githubGet(ctx, sig.URL, "application/octet-stream")
	if err != nil {
		return "", fmt.Errorf("%s: %v", checksumsAsset+releaseSignatureExt, err)
	}
	sigData, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseSigFileSize+1))
	resp.Body.Close()
	if err != nil {
		return "", fmt.Errorf("%s: %v", checksumsAsset+releaseSignatureExt, err)
	}
	id, err := verifyReleaseSignature(checksumsAsset, sumsData, sigData, keys)
	if err != nil {
		return "", fmt.Errorf("the signature of release %s is not valid (%v); update refused", rel.Tag, err)
	}
	return "key " + id, nil
}

func applyUpdate(ctx context.Context, r *http.Request, userID int, version string) (err error) {
	exe, err := selfExecutable()
	if err != nil {
		return err
	}
	labels := platformLabels(runtime.GOOS, runtime.GOARCH, buildGOARM)
	audit := func(action string, details map[string]string) {
		details["from"], details["to"] = AppVersion, version
		auditLog(r, userID, action, version, details)
	}
	fail := func(reason string, e error) error {
		audit("system.update_failed", map[string]string{"step": reason, "error": e.Error()})
		log.Printf("Update to %s failed (%s): %v", version, reason, e)
		return e
	}
	setUpdateProgress(updateProgressInfo{State: "downloading", Version: version})
	audit("system.update_started", map[string]string{"platform": labels[0], "service": serviceMode})
	rel, err := fetchRelease(ctx, updateRepo(), version)
	if err != nil {
		return fail("release", fmt.Errorf("release %s: %v", version, err))
	}
	asset := pickAsset(rel, labels)
	if asset == nil {
		return fail("asset", fmt.Errorf("the release %s has no file for this platform (%s)", rel.Tag, releaseAssetName(rel.Tag, labels[0])))
	}
	var sums *releaseAsset
	for i := range rel.Assets {
		if rel.Assets[i].Name == checksumsAsset {
			sums = &rel.Assets[i]
		}
	}
	if sums == nil {
		return fail("checksums", fmt.Errorf("the release %s has no %s, so the download cannot be verified; update refused", rel.Tag, checksumsAsset))
	}
	resp, err := githubGet(ctx, sums.URL, "application/octet-stream")
	if err != nil {
		return fail("checksums", fmt.Errorf("%s: %v", checksumsAsset, err))
	}
	sumsData, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if err != nil {
		return fail("checksums", fmt.Errorf("%s: %v", checksumsAsset, err))
	}
	signedBy, err := checkReleaseSignature(ctx, rel, sumsData)
	if err != nil {
		return fail("signature", err)
	}
	want := parseChecksums(bytes.NewReader(sumsData))[asset.Name]
	if want == "" {
		return fail("checksums", fmt.Errorf("%s has no line for %s; update refused", checksumsAsset, asset.Name))
	}

	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".wrm-update-*"+binaryExt(runtime.GOOS))
	if err != nil {
		return fail("download", fmt.Errorf("cannot write to %s: %v", dir, err))
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		if !keep {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	resp, err = githubGet(ctx, asset.URL, "application/octet-stream")
	if err != nil {
		return fail("download", fmt.Errorf("%s: %v", asset.Name, err))
	}
	total := resp.ContentLength
	if total <= 0 {
		total = asset.Size
	}
	h := sha256.New()
	last := time.Now()
	cw := &countingWriter{report: func(n int64) {
		if time.Since(last) > 200*time.Millisecond {
			last = time.Now()
			setUpdateProgress(updateProgressInfo{State: "downloading", Version: version, Done: n, Total: total})
		}
	}}
	n, err := io.Copy(io.MultiWriter(tmp, h, cw), io.LimitReader(resp.Body, maxUpdateBytes+1))
	resp.Body.Close()
	if err == nil && n > maxUpdateBytes {
		err = errors.New("the file is too large")
	}
	if err == nil {
		err = tmp.Close()
	}
	if err != nil {
		return fail("download", fmt.Errorf("%s: %v", asset.Name, err))
	}
	audit("system.update_downloaded", map[string]string{"asset": asset.Name, "bytes": fmt.Sprint(n)})

	setUpdateProgress(updateProgressInfo{State: "verifying", Version: version, Done: n, Total: n})
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fail("verify", fmt.Errorf("SHA-256 of %s does not match %s (expected %s, got %s); update refused", asset.Name, checksumsAsset, want, got))
	}
	os.Chmod(tmpName, 0o755)
	answer, err := probeBinaryVersion(tmpName)
	if err != nil {
		return fail("verify", fmt.Errorf("the new binary does not run (-version): %v", err))
	}
	if !versionAnswerMatches(answer, rel.Tag) {
		return fail("verify", fmt.Errorf("the new binary answers -version with %q instead of %s", answer, rel.Tag))
	}
	audit("system.update_verified", map[string]string{"asset": asset.Name, "sha256": got, "signature": signedBy})

	setUpdateProgress(updateProgressInfo{State: "installing", Version: version})
	if err := replaceBinary(exe, tmpName); err != nil {
		return fail("install", err)
	}
	keep = true
	forgetPreviousBinary()
	forgetLocalBinary()
	audit("system.update_installed", map[string]string{"asset": asset.Name, "previous": filepath.Base(previousBinaryPath())})
	log.Printf("Updated to %s (%s); restarting", rel.Tag, asset.Name)
	setUpdateProgress(updateProgressInfo{State: "restarting", Version: version})
	audit("system.update_restart", map[string]string{"service": serviceMode})
	time.Sleep(500 * time.Millisecond)
	requestRestart("")
	return nil
}

// replaceBinary puts newFile in the place of exe and keeps the old binary for a rollback.
func replaceBinary(exe, newFile string) error {
	prev := exe + ".previous"
	if runtime.GOOS == "windows" {
		// A running .exe cannot be overwritten or deleted, but it can be renamed.
		old := exe + ".old"
		os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			return fmt.Errorf("rename the running binary: %v", err)
		}
		if err := os.Rename(newFile, exe); err != nil {
			os.Rename(old, exe)
			return fmt.Errorf("put the new binary in place: %v", err)
		}
		return nil
	}
	os.Remove(prev)
	if err := os.Link(exe, prev); err != nil {
		if err := copyFile(exe, prev); err != nil {
			return fmt.Errorf("keep the previous binary: %v", err)
		}
	}
	if err := os.Rename(newFile, exe); err != nil {
		return fmt.Errorf("put the new binary in place: %v", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	fi, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// rollbackUpdate swaps the binary with <binary>.previous and restarts.
func rollbackUpdate(r *http.Request, userID int, version string) error {
	exe, err := selfExecutable()
	if err != nil {
		return err
	}
	prev := exe + ".previous"
	setUpdateProgress(updateProgressInfo{State: "installing", Version: version})
	if runtime.GOOS == "windows" {
		old := exe + ".old"
		os.Remove(old)
		if err = os.Rename(exe, old); err == nil {
			if err = os.Rename(prev, exe); err != nil {
				os.Rename(old, exe)
			}
		}
	} else {
		tmp := filepath.Join(filepath.Dir(exe), ".wrm-update-rollback")
		os.Remove(tmp)
		if err = os.Rename(exe, tmp); err == nil {
			if err = os.Rename(prev, exe); err != nil {
				os.Rename(tmp, exe)
			} else {
				os.Rename(tmp, prev)
			}
		}
	}
	if err != nil {
		auditLog(r, userID, "system.update_failed", version, map[string]string{"step": "rollback", "error": err.Error(), "from": AppVersion, "to": version})
		return fmt.Errorf("rollback: %v", err)
	}
	forgetPreviousBinary()
	forgetLocalBinary()
	auditLog(r, userID, "system.update_rollback", version, map[string]string{"from": AppVersion, "to": version})
	log.Printf("Rolled back to %s; restarting", version)
	setUpdateProgress(updateProgressInfo{State: "restarting", Version: version})
	time.Sleep(500 * time.Millisecond)
	requestRestart("")
	return nil
}
