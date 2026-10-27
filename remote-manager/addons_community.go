// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ─── COMMUNITY LIBRARY ───────────────────────────────
//
// The index is a JSON file in the public repository "WRM Pro - community public
// addons". WRM reads it, shows every entry with its trust level and refuses to
// install anything whose SHA-256 does not match the index. The download goes
// through the same archive checks as an uploaded ZIP; the index cannot point at
// an arbitrary host, so it cannot turn WRM into a request forwarder.

var (
	addonCommunityIndexURL = "https://raw.githubusercontent.com/vedranius/wrm-pro-community-addons/main/index.json"
	// tests point this at a local server
	addonCommunityIndex = addonCommunityIndexURL

	addonCommunityTrust = map[string]string{
		"official":   "written by the WRM author",
		"reviewed":   "checked by a maintainer",
		"community":  "published by the community, checked for format only",
		"unverified": "not reviewed; inspect the archive yourself",
	}
	addonCommunitySHA       = regexp.MustCompile(`^[a-f0-9]{64}$`)
	addonCommunityFetchTime = 10 * time.Minute
	addonCommunityCache     struct {
		sync.Mutex
		at   time.Time
		list []communityAddon
	}
)

type communityAddon struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description,omitempty"`
	Author       string   `json:"author,omitempty"`
	License      string   `json:"license,omitempty"`
	Homepage     string   `json:"homepage,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Trust        string   `json:"trust"`
	URL          string   `json:"url"`
	SHA256       string   `json:"sha256"`
}

// addonCommunityAllowedHost keeps the index from pointing anywhere: HTTPS to a
// GitHub host, or plain HTTP to loopback so the test suite can serve an index.
func addonCommunityAllowedHost(u *url.URL) bool {
	if u == nil || u.User != nil || u.Fragment != "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	loop := host == "127.0.0.1" || host == "localhost" || host == "::1"
	if loop {
		return u.Scheme == "http" || u.Scheme == "https"
	}
	if u.Scheme != "https" {
		return false
	}
	switch host {
	case "github.com", "raw.githubusercontent.com", "objects.githubusercontent.com", "codeload.github.com":
		return true
	}
	return false
}

func addonCommunityHTTP() *http.Client {
	return &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if !addonCommunityAllowedHost(req.URL) {
				return fmt.Errorf("the index redirected to an unsupported host")
			}
			return nil
		},
	}
}

func addonCommunityValid(a communityAddon) bool {
	if !addonIDPattern.MatchString(a.ID) || strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.Version) == "" {
		return false
	}
	if _, ok := addonCommunityTrust[a.Trust]; !ok {
		return false
	}
	if !addonCommunitySHA.MatchString(strings.ToLower(a.SHA256)) {
		return false
	}
	u, err := url.Parse(a.URL)
	if err != nil || !addonCommunityAllowedHost(u) {
		return false
	}
	for _, c := range a.Capabilities {
		if _, ok := addonCapabilities[c]; !ok {
			return false
		}
	}
	return true
}

func addonCommunityList() ([]communityAddon, error) {
	addonCommunityCache.Lock()
	defer addonCommunityCache.Unlock()
	if len(addonCommunityCache.list) > 0 && time.Since(addonCommunityCache.at) < addonCommunityFetchTime {
		return addonCommunityCache.list, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addonCommunityIndex, nil)
	if err != nil {
		return nil, fmt.Errorf("the community index address is invalid")
	}
	res, err := addonCommunityHTTP().Do(req)
	if err != nil {
		return nil, fmt.Errorf("the community index cannot be reached")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the community index answered HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("the community index cannot be read")
	}
	var idx struct {
		Addons []communityAddon `json:"addons"`
	}
	if json.Unmarshal(raw, &idx) != nil {
		return nil, fmt.Errorf("the community index is not valid JSON")
	}
	list := []communityAddon{}
	for _, a := range idx.Addons {
		if !addonCommunityValid(a) {
			continue // one bad entry must not hide the rest
		}
		a.SHA256 = strings.ToLower(a.SHA256)
		list = append(list, a)
	}
	addonCommunityCache.at, addonCommunityCache.list = time.Now(), list
	return list, nil
}

// GET /api/addons/community — everyone signed in sees what is available.
func addonCommunityListHandler(w http.ResponseWriter, r *http.Request) {
	list, err := addonCommunityList()
	if err != nil {
		jsonError(w, err.Error(), 502)
		return
	}
	jsonOK(w, map[string]interface{}{"addons": list, "trust": addonCommunityTrust, "index": addonCommunityIndex})
}

// POST /api/addons/community/install {"id": "..."} — administrator only.
func addonCommunityInstallHandler(w http.ResponseWriter, r *http.Request, adminID int) {
	var p struct {
		ID string `json:"id"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&p) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	out, status, err := addonCommunityInstall(r.Context(), r, adminID, p.ID)
	if err != nil {
		jsonError(w, err.Error(), status)
		return
	}
	jsonOK(w, map[string]interface{}{"id": out.ID, "name": out.Name, "version": out.Version, "enabled": out.Enabled, "checksum": out.Checksum})
}

// addonCommunityInstall downloads one index entry and installs it through the
// same archive checks as an upload. The AI tool calls it without a request.
func addonCommunityInstall(ctx context.Context, r *http.Request, adminID int, id string) (addonInstallOutcome, int, error) {
	var out addonInstallOutcome
	list, err := addonCommunityList()
	if err != nil {
		return out, 502, err
	}
	var chosen *communityAddon
	for i := range list {
		if list[i].ID == id {
			chosen = &list[i]
			break
		}
	}
	if chosen == nil {
		return out, 404, fmt.Errorf("this addon is not in the community index")
	}
	dctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(dctx, http.MethodGet, chosen.URL, nil)
	if err != nil {
		return out, 400, fmt.Errorf("the addon download address is invalid")
	}
	res, err := addonCommunityHTTP().Do(req)
	if err != nil {
		return out, 502, fmt.Errorf("the addon cannot be downloaded")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return out, 502, fmt.Errorf("the addon download answered HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, addonMaxArchive+1))
	if err != nil || len(body) == 0 {
		return out, 502, fmt.Errorf("the addon cannot be downloaded")
	}
	if len(body) > addonMaxArchive {
		return out, 413, fmt.Errorf("the archive is larger than 20 MB")
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != chosen.SHA256 {
		return out, 409, fmt.Errorf("the downloaded archive does not match the checksum in the index; nothing was installed")
	}
	return addonInstallArchive(ctx, r, adminID, body, "community:"+chosen.ID+"@"+chosen.Trust)
}
