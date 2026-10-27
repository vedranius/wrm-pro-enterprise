// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"crypto"
)

var testSAKey *rsa.PrivateKey

func testServiceAccountJSON(t *testing.T, tokenURI string) string {
	t.Helper()
	if testSAKey == nil {
		testSAKey, _ = rsa.GenerateKey(rand.Reader, 2048)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(testSAKey)
	b, _ := json.Marshal(map[string]string{"type": "service_account", "client_email": "wrm@demo-project.iam.gserviceaccount.com",
		"private_key": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), "token_uri": tokenURI})
	return string(b)
}

func testAIRequest() aiLLMRequest {
	return aiLLMRequest{System: "sys", Model: "m", MaxTokens: 100,
		Messages: []aiMsg{{Role: "user", Text: "hi"}, {Role: "assistant", Text: "checking", Calls: []aiToolCall{{ID: "t1", Name: "system_info", Input: json.RawMessage(`{}`)}}},
			{Role: "user", Results: []aiToolResult{{ID: "t1", Name: "system_info", Content: "Linux"}}}},
		Tools: []aiToolDef{{Name: "system_info", Description: "d", Schema: obj(map[string]interface{}{})}}}
}

// AWS SigV4 against the published "get-vanilla" test vector.
func TestSigV4Vector(t *testing.T) {
	r, _ := http.NewRequest("GET", "https://example.amazonaws.com/", nil)
	r.Header.Del("Content-Type")
	now, _ := time.Parse("20060102T150405Z", "20150830T123600Z")
	sigV4Sign(r, nil, "AKIDEXAMPLE", "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", "", "us-east-1", "service", now)
	want := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;x-amz-date, Signature=5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31"
	if got := r.Header.Get("Authorization"); got != want {
		t.Fatalf("signature\n got %s\nwant %s", got, want)
	}
}

// Bedrock Converse: SigV4 signing, the event stream, tool calls and usage.
func TestBedrockConverseStream(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.EscapedPath(), r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		ev := func(typ string, v interface{}) {
			p, _ := json.Marshal(v)
			w.Write(encodeEventStreamMessage(map[string]string{":event-type": typ, ":message-type": "event", ":content-type": "application/json"}, p))
		}
		ev("messageStart", map[string]string{"role": "assistant"})
		ev("contentBlockDelta", map[string]interface{}{"contentBlockIndex": 0, "delta": map[string]string{"text": "Look"}})
		ev("contentBlockDelta", map[string]interface{}{"contentBlockIndex": 0, "delta": map[string]string{"text": "ing."}})
		ev("contentBlockStart", map[string]interface{}{"contentBlockIndex": 1, "start": map[string]interface{}{"toolUse": map[string]string{"toolUseId": "tu1", "name": "journal"}}})
		ev("contentBlockDelta", map[string]interface{}{"contentBlockIndex": 1, "delta": map[string]interface{}{"toolUse": map[string]string{"input": `{"unit":`}}})
		ev("contentBlockDelta", map[string]interface{}{"contentBlockIndex": 1, "delta": map[string]interface{}{"toolUse": map[string]string{"input": `"nginx"}`}}})
		ev("messageStop", map[string]string{"stopReason": "tool_use"})
		ev("metadata", map[string]interface{}{"usage": map[string]int{"inputTokens": 11, "outputTokens": 7}})
	}))
	defer srv.Close()
	p := aiProvider{Kind: "bedrock", Region: "eu-central-1", BaseURL: srv.URL, APIMode: "converse",
		secret: aiProviderSecret{AccessKeyID: "AKIDEXAMPLE", SecretKey: "secret"}}
	req := testAIRequest()
	req.Model = "eu.anthropic.claude-opus-5-5:0"
	var streamed strings.Builder
	res, err := aiComplete(context.Background(), p, req, func(s string) { streamed.WriteString(s) })
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/model/eu.anthropic.claude-opus-5-5%3A0/converse-stream" {
		t.Fatalf("path %s", gotPath)
	}
	if !strings.HasPrefix(gotAuth, "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/") || !strings.Contains(gotAuth, "/eu-central-1/bedrock/aws4_request") {
		t.Fatalf("auth %s", gotAuth)
	}
	if streamed.String() != "Looking." || res.Text != "Looking." || len(res.Calls) != 1 || string(res.Calls[0].Input) != `{"unit":"nginx"}` || res.Usage.In != 11 {
		t.Fatalf("result %+v", res)
	}
	msgs := gotBody["messages"].([]interface{})
	if len(msgs) != 3 || !strings.Contains(mustJSON(msgs[2]), `"toolResult"`) || !strings.Contains(mustJSON(gotBody["toolConfig"]), "system_info") {
		t.Fatalf("body %v", gotBody)
	}
	// an exception in the stream is an error
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(encodeEventStreamMessage(map[string]string{":message-type": "exception", ":exception-type": "throttlingException"}, []byte(`{"message":"slow down"}`)))
	})
	if _, err := aiComplete(context.Background(), p, req, nil); err == nil || !strings.Contains(err.Error(), "slow down") {
		t.Fatalf("exception: %v", err)
	}
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// Vertex AI: the service account token exchange, Claude's rawPredict and the
// OpenAI-compatible endpoint.
func TestVertexProvider(t *testing.T) {
	tokenCalls := 0
	tok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		parts := strings.Split(r.Form.Get("assertion"), ".")
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" || len(parts) != 3 {
			w.WriteHeader(400)
			return
		}
		sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
		h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if rsa.VerifyPKCS1v15(&testSAKey.PublicKey, crypto.SHA256, h[:], sig) != nil {
			w.WriteHeader(401)
			return
		}
		tokenCalls++
		json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "ya29.fake-access-token-0123456789", "expires_in": 3600})
	}))
	defer tok.Close()
	llm := newFakeLLM(t, func(n int, body map[string]interface{}) fakeTurn { return fakeTurn{Text: "Hi from Vertex"} })
	p := aiProvider{Kind: "vertex", Region: "us-east5", Project: "demo-project", BaseURL: llm.srv.URL, APIMode: "anthropic",
		secret: aiProviderSecret{ServiceAccount: testServiceAccountJSON(t, tok.URL)}}
	req := testAIRequest()
	req.Model = "claude-opus-5-5"
	res, err := aiComplete(context.Background(), p, req, nil)
	if err != nil || res.Text != "Hi from Vertex" {
		t.Fatalf("%v %+v", err, res)
	}
	if llm.paths[0] != "/v1/projects/demo-project/locations/us-east5/publishers/anthropic/models/claude-opus-5-5:streamRawPredict" {
		t.Fatalf("path %s", llm.paths[0])
	}
	b := llm.body(0)
	if b["anthropic_version"] != "vertex-2023-10-16" || b["model"] != nil || llm.headers[0].Get("Authorization") != "Bearer ya29.fake-access-token-0123456789" {
		t.Fatalf("request %v %v", b, llm.headers[0])
	}
	if b["cache_control"] != nil || b["fallbacks"] != nil {
		t.Fatal("first-party-only fields sent to Vertex")
	}
	p.APIMode = "openai"
	req.Model = "google/gemini-model"
	if _, err := aiComplete(context.Background(), p, req, nil); err != nil {
		t.Fatal(err)
	}
	if llm.paths[1] != "/v1/projects/demo-project/locations/us-east5/endpoints/openapi/chat/completions" || tokenCalls != 1 {
		t.Fatalf("path %s, token calls %d", llm.paths[1], tokenCalls)
	}
}

// Azure OpenAI (deployment + api-version and the v1 API) and OpenAI.
func TestAzureAndOpenAIProviders(t *testing.T) {
	llm := newFakeLLM(t, func(n int, body map[string]interface{}) fakeTurn {
		return fakeTurn{Text: "ok", Calls: []fakeCall{{"journal", `{"unit":"nginx","lines":50}`}}}
	})
	p := aiProvider{Kind: "azure", BaseURL: llm.srv.URL, Deployment: "gpt-deploy", APIVersion: "2024-10-21", secret: aiProviderSecret{APIKey: "azure-key-123"}}
	req := testAIRequest()
	req.Model = "gpt-deploy"
	res, err := aiComplete(context.Background(), p, req, nil)
	if err != nil || len(res.Calls) != 1 || string(res.Calls[0].Input) != `{"unit":"nginx","lines":50}` {
		t.Fatalf("%v %+v", err, res)
	}
	u, _ := url.Parse(llm.paths[0])
	if u.Path != "/openai/deployments/gpt-deploy/chat/completions" || u.Query().Get("api-version") != "2024-10-21" || llm.headers[0].Get("api-key") != "azure-key-123" {
		t.Fatalf("azure request %s %v", llm.paths[0], llm.headers[0])
	}
	msgs := llm.body(0)["messages"].([]interface{})
	if asMap(msgs[2])["tool_calls"] == nil || asMap(msgs[3])["role"] != "tool" || asMap(msgs[3])["tool_call_id"] != "t1" {
		t.Fatalf("messages %v", msgs)
	}
	p.APIVersion = ""
	aiComplete(context.Background(), p, req, nil)
	if llm.paths[1] != "/openai/v1/chat/completions" {
		t.Fatalf("v1 path %s", llm.paths[1])
	}
	o := aiProvider{Kind: "openai", BaseURL: llm.srv.URL + "/v1", secret: aiProviderSecret{APIKey: "sk-openai-test"}}
	aiComplete(context.Background(), o, req, nil)
	if llm.headers[2].Get("Authorization") != "Bearer sk-openai-test" || llm.body(2)["max_completion_tokens"] == nil {
		t.Fatalf("openai request %v %v", llm.headers[2], llm.body(2))
	}
	// errors: the provider's message, without the key
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"Incorrect API key provided: sk-openai-test","type":"invalid_request_error"}}`))
	}))
	defer bad.Close()
	o.BaseURL = bad.URL
	_, err = aiComplete(context.Background(), o, req, nil)
	if err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "sk-openai-test") {
		t.Fatalf("error %v", err)
	}
}

// Anthropic first-party extras, refusal and cancellation.
func TestAnthropicRequestShape(t *testing.T) {
	llm := newFakeLLM(t, func(n int, body map[string]interface{}) fakeTurn {
		if n == 1 {
			return fakeTurn{Text: "I can't help with that.", Stop: "refusal"}
		}
		return fakeTurn{Text: "fine"}
	})
	p := aiProvider{Kind: "anthropic", Fallback: true, secret: aiProviderSecret{APIKey: "k"}}
	// first-party fields are only sent without a base URL; use a transport that redirects to the fake
	old := aiHTTPClient
	aiHTTPClient = &http.Client{Transport: redirectTransport{to: llm.srv.URL}}
	defer func() { aiHTTPClient = old }()
	req := testAIRequest()
	req.Model = "claude-opus-5-5"
	if _, err := aiComplete(context.Background(), p, req, nil); err != nil {
		t.Fatal(err)
	}
	b := llm.body(0)
	if b["fallbacks"] != "default" || llm.headers[0].Get("anthropic-beta") != "server-side-fallback-2026-07-01" || b["cache_control"] == nil {
		t.Fatalf("first-party request %v %v", b, llm.headers[0])
	}
	tools := b["tools"].([]interface{})
	if asMap(tools[0])["eager_input_streaming"] != true {
		t.Fatal("eager input streaming not set")
	}
	res, _ := aiComplete(context.Background(), p, req, nil)
	if !res.Refusal {
		t.Fatal("refusal not detected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := aiComplete(ctx, p, req, nil); err == nil {
		t.Fatal("cancelled request succeeded")
	}
	if c := aiCost(aiProvider{Kind: "anthropic"}, "claude-opus-5-5", aiUsage{In: 1_000_000, Out: 1_000_000}); c != 24 {
		t.Fatalf("cost %v", c)
	}
}

type redirectTransport struct{ to string }

func (rt redirectTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	u, _ := url.Parse(rt.to)
	r.URL.Scheme, r.URL.Host = u.Scheme, u.Host
	return http.DefaultTransport.RoundTrip(r)
}
