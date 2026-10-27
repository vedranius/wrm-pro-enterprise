// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// The optional client companion runs on the user's computer. It never listens on
// a port, never changes browser security, and sends no GitLab token to WRM.
func runGitLabRelayClient(workerURL, gitURL, keyFile, gitTokenFile, caFile string, out io.Writer, liveMode ...bool) int {
	fail := func(msg string) int { fmt.Fprintln(out, msg); return 1 }
	worker, err := url.Parse(workerURL)
	if err != nil || worker.User != nil || worker.Fragment != "" || worker.Path != "/gitlab-relay" || worker.Query().Get("job") == "" || !oneOf(worker.Scheme, "http", "https") {
		return fail("Use the exact relay job URL from WRM Git settings")
	}
	if worker.Scheme == "http" && !oneOf(worker.Hostname(), "localhost", "127.0.0.1", "::1") {
		return fail("The WRM relay URL must use HTTPS outside loopback")
	}
	api, err := gitAPIBase("gitlab", gitURL)
	if err != nil {
		return fail("Invalid GitLab URL")
	}
	base, _ := url.Parse(api)
	if base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return fail("GitLab URL cannot contain credentials, query or fragment")
	}
	readSecret := func(file, env string) (string, error) {
		if file != "" {
			b, err := os.ReadFile(file)
			if err != nil {
				return "", err
			}
			if len(b) > 4096 {
				return "", fmt.Errorf("secret file too large")
			}
			return strings.TrimSpace(string(b)), nil
		}
		return strings.TrimSpace(os.Getenv(env)), nil
	}
	key, err := readSecret(keyFile, "WRM_RELAY_KEY")
	if err != nil || key == "" {
		return fail("Read the relay key from -relay-key-file or WRM_RELAY_KEY")
	}
	token, err := readSecret(gitTokenFile, "WRM_GITLAB_TOKEN")
	if err != nil || token == "" {
		return fail("Read your GitLab access token from -gitlab-token-file or WRM_GITLAB_TOKEN")
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	defer tr.CloseIdleConnections()
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return fail("Cannot read CA file")
		}
		roots, _ := x509.SystemCertPool()
		if roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(pem) {
			return fail("Invalid CA file")
		}
		tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	}
	client := &http.Client{Timeout: 60 * time.Second, Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	workerCall := func(method string, body interface{}, result interface{}) error {
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(jsonMarshal(body))
		}
		req, err := http.NewRequest(method, workerURL, reader)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("WRM relay connection unavailable; check network and certificate")
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("WRM relay answered HTTP %d; job may have expired or permissions changed", resp.StatusCode)
		}
		if result != nil {
			return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(result)
		}
		return nil
	}
	live := len(liveMode) > 0 && liveMode[0]
	published := false
	until := time.Now().Add(30 * time.Minute)
	for time.Now().Before(until) {
		var state struct {
			State    string   `json:"state"`
			Error    string   `json:"error"`
			Projects []string `json:"projects"`
			Requests []struct {
				ID  int    `json:"id"`
				URL string `json:"url"`
			} `json:"requests"`
			Completed int `json:"completed"`
		}
		if err := workerCall("GET", nil, &state); err != nil {
			if published {
				fmt.Fprintln(out, "Client relay ended; the last complete server snapshot is retained: "+err.Error())
				return 0
			}
			return fail(err.Error())
		}
		if state.State == "cached" {
			if !published {
				fmt.Fprintf(out, "Complete snapshot published (%d responses). GitLab token remained on this computer.\n", state.Completed)
				published = true
			}
			if !live {
				workerCall("DELETE", nil, nil)
				return 0
			}
		}
		if state.State == "failed" {
			return fail("Synchronization failed; previous snapshot retained: " + state.Error)
		}
		for _, task := range state.Requests {
			u, err := url.Parse(task.URL)
			if err != nil {
				return fail("Invalid task URL")
			}
			rel, err := relayRequestKey(api, u)
			if err != nil {
				return fail("WRM requested a different GitLab origin; refusing")
			}
			allowed := false
			for _, project := range state.Projects {
				prefix := "/projects/" + url.PathEscape(project)
				if strings.SplitN(rel, "?", 2)[0] == prefix || strings.HasPrefix(rel, prefix+"/repository/") {
					allowed = true
					break
				}
			}
			if !allowed {
				return fail("WRM requested an endpoint outside the selected catalog repositories")
			}
			req, _ := http.NewRequest("GET", task.URL, nil)
			req.Header.Set("PRIVATE-TOKEN", token)
			req.Header.Set("Accept", "application/json")
			res := gitRelayResponse{Status: 502, Type: "application/json", Body: []byte(`{"message":"Client cannot reach GitLab; check network and CA certificates"}`)}
			resp, err := client.Do(req)
			if err == nil {
				body, e := io.ReadAll(io.LimitReader(resp.Body, relayMaxBody+1))
				resp.Body.Close()
				if e != nil || len(body) > relayMaxBody {
					return fail("GitLab response exceeds 8 MiB; previous snapshot retained")
				}
				if resp.StatusCode/100 != 2 {
					body = jsonMarshal(map[string]string{"message": fmt.Sprintf("Client GitLab answered HTTP %d; check token read_api scope, repository access, expiry and API limits", resp.StatusCode)})
				} else if bytes.Contains(body, []byte(token)) {
					body = jsonMarshal(map[string]string{"message": "GitLab response contained client authorization material; refusing to forward it"})
					resp.StatusCode = 422
				}
				res = gitRelayResponse{Status: resp.StatusCode, Body: body, Next: resp.Header.Get("X-Next-Page"), Type: resp.Header.Get("Content-Type")}
			}
			body := map[string]interface{}{"request_id": task.ID, "status": res.Status, "body": res.Body, "next": res.Next, "type": res.Type}
			if err := workerCall("POST", body, nil); err != nil {
				return fail(err.Error())
			}
		}
		if len(state.Requests) == 0 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	if published {
		fmt.Fprintln(out, "Client relay session expired; complete cached snapshot remains active.")
		return 0
	}
	return fail("Client synchronization timed out; previous complete snapshot remains active")
}
