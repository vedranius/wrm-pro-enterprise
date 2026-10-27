// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// ─── AI: OPENCODE GO ─────────────────────────────────
//
// OpenCode Go (opencode.ai/console/go) is a subscription next to OpenCode Zen
// (the pay-as-you-go Console). Both use an OpenCode API key, but a Go key is
// only accepted by Go's endpoints:
//
//	chat      https://opencode.ai/inference/go/openai/v1/chat/completions
//	messages  https://opencode.ai/inference/go/anthropic/v1/messages
//	models    https://opencode.ai/inference/go/v1/models
//
// Sending a Go key to Zen makes Console answer "insufficient funds", because
// the key has no pay-as-you-go balance; that is why Go needs its own provider
// kind instead of reusing "opencode".

const (
	opencodeGoRoot          = "https://opencode.ai/inference/go"
	opencodeGoDefaultModel  = "kimi-k3"
	opencodeGoSessionHeader = "x-opencode-session"
)

// opencodeGoAPIs is the per-model API on the Go endpoint. It differs from Zen
// for some models with the same id (Qwen and MiniMax M3 are Messages on Go,
// Chat Completions on Zen), so the Zen catalog alone is not enough.
var opencodeGoAPIs = map[string]string{
	// Anthropic Messages
	"claude-haiku-5-5": "messages",
	"qwen3.8-max":      "messages",
	"qwen3.8-flash":    "messages",
	"qwen3.7-plus":     "messages",
	"minimax-m3":       "messages",
	// OpenAI Chat Completions
	"glm-5.3-flash":            "chat",
	"glm-5.3":                  "chat",
	"glm-5.2":                  "chat",
	"kimi-k3":                  "chat",
	"kimi-k2.7-code":           "chat",
	"longcat-2.0":              "chat",
	"deepseek-v4.1-flash":      "chat",
	"deepseek-v4-pro":          "chat",
	"mimo-v2.6-flash":          "chat",
	"mimo-v2.6-pro":            "chat",
	"mimo-v2.5":                "chat",
	"mimo-v2.5-pro":            "chat",
	"minimax-m3.1-flash":       "chat",
	"hy4-preview":              "chat",
	"hy3":                      "chat",
	"longcat-2.5-preview-free": "chat",
	"step-5-preview":           "chat",
	"beam-free":                "chat",
	// Responses / Google: the docs list them, WRM cannot speak those APIs yet.
	"grok-4.7":                   "responses",
	"gpt-6-luna":                 "responses",
	"gpt-5.6-luna":               "responses",
	"gemini-4-argon-contributor": "google",
	"muse-spark-1.3-contributor": "responses",
	"muse-spark-1.2-contributor": "responses",
}

// opencodeGoFallback is used when the Go model list cannot be fetched. It is
// the list from the Go documentation at the time of this release; unknown new
// models are still accepted when typed by hand.
var opencodeGoFallback = []string{
	"claude-haiku-5-5", "qwen3.8-max", "qwen3.8-flash", "qwen3.7-plus", "minimax-m3",
	"glm-5.3-flash", "glm-5.3", "glm-5.2", "kimi-k3", "kimi-k2.7-code", "longcat-2.0",
	"deepseek-v4.1-flash", "deepseek-v4-pro", "mimo-v2.6-flash", "mimo-v2.6-pro", "mimo-v2.5", "mimo-v2.5-pro",
	"minimax-m3.1-flash", "hy4-preview", "hy3", "longcat-2.5-preview-free", "step-5-preview", "beam-free",
}

func opencodeGoAPI(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if a, ok := opencodeGoAPIs[id]; ok {
		return a
	}
	return opencodeGuessAPI(id)
}

func opencodeGoSupported(id string) bool {
	a := opencodeGoAPI(id)
	return a == "messages" || a == "chat"
}

// opencodeGoRootFor accepts the Go root or one of its API bases and returns the
// root, so a custom base URL may be written in either form.
func opencodeGoRootFor(p aiProvider) string {
	b := strings.TrimRight(nonEmpty(p.BaseURL, opencodeGoRoot), "/")
	for _, suffix := range []string{"/openai/v1", "/anthropic/v1", "/openai", "/anthropic", "/v1"} {
		if strings.HasSuffix(b, suffix) {
			b = strings.TrimSuffix(b, suffix)
			break
		}
	}
	return b
}

func opencodeGoAPIBase(p aiProvider, api string) string {
	base := opencodeGoRootFor(p)
	switch api {
	case "messages":
		return base + "/anthropic/v1"
	case "responses":
		return base + "/openai/v1"
	case "google":
		return base + "/google/v1beta"
	}
	return base + "/openai/v1"
}

func opencodeGoFree(id string) bool {
	id = strings.ToLower(id)
	return strings.HasSuffix(id, "-free") || id == "longcat-2.5-preview-free" || id == "beam-free"
}

func opencodeGoDataUse(id string) string {
	id = strings.ToLower(id)
	switch {
	case strings.Contains(id, "contributor"):
		return "trains" // Gemini 4 Argon Contributor, Muse Spark Contributor
	case strings.HasPrefix(id, "claude"), strings.HasPrefix(id, "gpt"), strings.HasPrefix(id, "grok"):
		return "paid" // provider retains for 30 days, not used for training
	}
	return "zero_retention"
}

// opencodeGoModelInfo describes one Go model. Price and context come from the
// models.dev OpenCode catalog when it knows the id; the API and data use are Go's.
func opencodeGoModelInfo(ctx context.Context, id string) opencodeModel {
	m := opencodeModelInfo(ctx, id)
	m.ID, m.Name = id, nonEmpty(m.Name, id)
	m.API = opencodeGoAPI(id)
	m.Free = opencodeGoFree(id)
	m.Tools = true
	m.DataUse = opencodeGoDataUse(id)
	return m
}

func opencodeGoSession(id string) string {
	id = strings.TrimSpace(id)
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		}
		if b.Len() >= 64 {
			break
		}
	}
	return nonEmpty(b.String(), "oneshot")
}

func opencodeGoHeaders(key, session string) http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+key)
	h.Set("User-Agent", "WRM-PRO/"+AppVersion)
	h.Set(opencodeGoSessionHeader, "wrm-"+opencodeGoSession(session))
	return h
}

// opencodeGoComplete sends one turn to the Go subscription. A Go key never goes
// to Zen: that endpoint would charge the pay-as-you-go Console balance and answer
// "insufficient funds".
func opencodeGoComplete(ctx context.Context, p aiProvider, req aiLLMRequest, onText func(string)) (aiLLMResponse, error) {
	if p.secret.APIKey == "" {
		return aiLLMResponse{}, fmt.Errorf("add an OpenCode Go API key to this provider (OpenCode Console → Go)")
	}
	m := opencodeGoModelInfo(ctx, req.Model)
	if m.API != "messages" && m.API != "chat" {
		return aiLLMResponse{}, fmt.Errorf("%s uses the %s API on OpenCode Go, which WRM does not support yet; pick a Claude, Qwen, MiniMax M3, GLM, Kimi, DeepSeek, MiMo, LongCat, Step, Hy or Beam model", req.Model,
			map[string]string{"responses": "OpenAI Responses", "google": "Google Gemini"}[m.API])
	}
	base, key := opencodeGoAPIBase(p, m.API), p.secret.APIKey
	switch m.API {
	case "messages":
		return anthropicStream(ctx, p, base+"/messages", req, onText, func(h http.Header) {
			for k, v := range opencodeGoHeaders(key, req.SessionID) {
				h[k] = v
			}
			h.Set("x-api-key", key)
		}, false)
	default:
		return openaiStream(ctx, p, base+"/chat/completions", req, onText, func(h http.Header) {
			for k, v := range opencodeGoHeaders(key, req.SessionID) {
				h[k] = v
			}
		})
	}
}

// opencodeGoModelIDs asks Go for its model list. The endpoint is OpenAI-shaped.
func opencodeGoModelIDs(ctx context.Context, p aiProvider) ([]string, error) {
	q := p
	q.Kind = "openai_compatible"
	q.BaseURL = opencodeGoRootFor(p) + "/v1"
	return aiListModels(ctx, q)
}

// opencodeGoListModels lists the Go models WRM can use. A failure to fetch the
// list falls back to the documented set instead of hiding the provider.
func opencodeGoListModels(ctx context.Context, p aiProvider) ([]opencodeModel, error) {
	if p.secret.APIKey == "" {
		return nil, fmt.Errorf("add an OpenCode Go API key to this provider")
	}
	ids, err := opencodeGoModelIDs(ctx, p)
	if err != nil {
		ids = opencodeGoFallback
	}
	seen := map[string]bool{}
	out := []opencodeModel{}
	for _, id := range append(append([]string{}, ids...), opencodeGoFallback...) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] || !opencodeGoSupported(id) {
			continue
		}
		seen[id] = true
		out = append(out, opencodeGoModelInfo(ctx, id))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Free != out[j].Free {
			return out[i].Free
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > 500 {
		out = out[:500]
	}
	return out, nil
}
