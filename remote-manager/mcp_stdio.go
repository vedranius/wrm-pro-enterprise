// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
)

// ─── MCP: STDIO BRIDGE ───────────────────────────────
//
//   wrm -mcp-stdio -url https://wrm.example.com/mcp -token wrm_pat_…
//
// For MCP clients that only start local servers (stdio): the bridge reads newline-delimited
// JSON-RPC messages from stdin, POSTs each one to WRM's /mcp endpoint with the token, and
// writes every message of the answer (JSON or an event stream, including WRM's elicitation
// requests) to stdout, one per line. The client's answers to those requests come back on
// stdin and are POSTed like any other message. Requests run concurrently (a tools/call can
// wait for an approval), the initialize request first. Nothing but protocol messages is
// written to stdout; diagnostics go to stderr. The token can also come from WRM_MCP_TOKEN or
// -token-file, the URL from WRM_MCP_URL; -ca-file trusts a private CA or a self-signed
// certificate of the WRM server.

type mcpBridge struct {
	url     string
	token   string
	client  *http.Client
	out     io.Writer
	outMu   sync.Mutex
	mu      sync.Mutex
	session string
	version string
	wg      sync.WaitGroup
}

func runMCPStdio(rawURL, token, tokenFile, caFile string, in io.Reader, out io.Writer) int {
	if rawURL == "" {
		rawURL = os.Getenv("WRM_MCP_URL")
	}
	if token == "" && tokenFile != "" {
		b, err := os.ReadFile(tokenFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "wrm -mcp-stdio: %v\n", err)
			return 2
		}
		token = strings.TrimSpace(string(b))
	}
	if token == "" {
		token = os.Getenv("WRM_MCP_TOKEN")
	}
	if rawURL == "" || token == "" {
		fmt.Fprintln(os.Stderr, "wrm -mcp-stdio: -url (or WRM_MCP_URL) and -token (or -token-file, WRM_MCP_TOKEN) are required")
		return 2
	}
	if !strings.HasSuffix(strings.TrimRight(rawURL, "/"), "/mcp") {
		rawURL = strings.TrimRight(rawURL, "/") + "/mcp"
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "wrm -mcp-stdio: %v\n", err)
			return 2
		}
		pool, _ := x509.SystemCertPool()
		if pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			fmt.Fprintln(os.Stderr, "wrm -mcp-stdio: no certificate found in -ca-file")
			return 2
		}
		tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	b := &mcpBridge{url: rawURL, token: token, client: &http.Client{Transport: tr}, out: out}
	return b.run(in)
}

func (b *mcpBridge) run(in io.Reader) int {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64<<10), mcpMaxBody)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		msg := append([]byte(nil), line...)
		var head struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(msg, &head) != nil {
			b.write(jsonMarshal(rpcErr(nil, -32700, "parse error")))
			continue
		}
		if head.Method == "initialize" {
			b.wg.Wait() // nothing else may run before the session exists
			b.forward(msg, head.ID, head.Method)
			continue
		}
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			b.forward(msg, head.ID, head.Method)
		}()
	}
	b.wg.Wait()
	b.mu.Lock()
	sid := b.session
	b.mu.Unlock()
	if sid != "" {
		req, _ := http.NewRequest(http.MethodDelete, b.url, nil)
		b.headers(req)
		if resp, err := b.client.Do(req); err == nil {
			resp.Body.Close()
		}
	}
	return 0
}

func (b *mcpBridge) headers(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", "wrm-mcp-stdio/"+AppVersion)
	b.mu.Lock()
	if b.session != "" {
		req.Header.Set("Mcp-Session-Id", b.session)
	}
	if b.version != "" {
		req.Header.Set("MCP-Protocol-Version", b.version)
	}
	b.mu.Unlock()
}

func (b *mcpBridge) write(msg []byte) {
	b.outMu.Lock()
	defer b.outMu.Unlock()
	b.out.Write(append(bytes.TrimSpace(msg), '\n'))
}

// forward POSTs one message and copies the answer to stdout.
func (b *mcpBridge) forward(msg []byte, id json.RawMessage, method string) {
	isRequest := len(id) > 0 && method != ""
	fail := func(text string) {
		fmt.Fprintln(os.Stderr, "wrm -mcp-stdio:", text)
		if isRequest {
			b.write(jsonMarshal(rpcErr(id, -32000, text)))
		}
	}
	req, err := http.NewRequest(http.MethodPost, b.url, bytes.NewReader(msg))
	if err != nil {
		fail(err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	b.headers(req)
	resp, err := b.client.Do(req)
	if err != nil {
		fail("WRM is not reachable: " + err.Error())
		return
	}
	defer resp.Body.Close()
	if method == "initialize" && resp.StatusCode == 200 {
		b.mu.Lock()
		b.session = resp.Header.Get("Mcp-Session-Id")
		b.mu.Unlock()
	}
	ct := resp.Header.Get("Content-Type")
	switch {
	case resp.StatusCode == 202:
		return
	case strings.HasPrefix(ct, "text/event-stream"):
		b.copyEvents(resp.Body)
	case strings.HasPrefix(ct, "application/json"):
		body, _ := io.ReadAll(io.LimitReader(resp.Body, mcpMaxBody))
		if resp.StatusCode != 200 {
			var e struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			json.Unmarshal(body, &e)
			fail(fmt.Sprintf("WRM answered HTTP %d: %s", resp.StatusCode, nonEmpty(e.Error.Message, strings.TrimSpace(string(body)))))
			return
		}
		if method == "initialize" {
			var r struct {
				Result struct {
					ProtocolVersion string `json:"protocolVersion"`
				} `json:"result"`
			}
			if json.Unmarshal(body, &r) == nil {
				b.mu.Lock()
				b.version = r.Result.ProtocolVersion
				b.mu.Unlock()
			}
		}
		b.write(body)
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		fail(fmt.Sprintf("WRM answered HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
	}
}

// copyEvents writes the data of every server-sent event to stdout.
func (b *mcpBridge) copyEvents(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 2*mcpMaxBody)
	var data []string
	flush := func() {
		if len(data) > 0 {
			b.write([]byte(strings.Join(data, "\n")))
			data = nil
		}
	}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	flush()
}
