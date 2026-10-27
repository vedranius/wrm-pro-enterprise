// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// ─── AI: OPENCODE ZEN ────────────────────────────────
//
// OpenCode Zen (opencode.ai) is a model gateway: one key, many models. Each model is served
// on the API its family speaks: Anthropic Messages (Claude, some Qwen and MiniMax models),
// OpenAI Chat Completions (most open models and every free model) or the OpenAI Responses
// and Google APIs (GPT, Grok, Gemini), which WRM does not speak yet.
//
// Which model uses which API, what it costs and whether it is free comes from the public
// models.dev catalog (provider "opencode"), the same source OpenCode itself uses. Without
// the catalog WRM falls back to Zen's own model list and a name rule.
//
// Without an API key Zen accepts the key "public" for its free models only (rate limited per
// IP address), exactly like OpenCode does when nobody is signed in. With a key (workspace at
// opencode.ai, pay as you go) every model is available.

const (
	opencodeDefaultBase = "https://opencode.ai/zen/v1"
	opencodePublicKey   = "public"
)

// opencodeCatalogURL is a variable so tests can point it at a fake server.
var opencodeCatalogURL = "https://models.dev/api.json"

type opencodeModel struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	API        string  `json:"api"` // messages | chat | responses | google
	Base       string  `json:"-"`   // the model's own endpoint base from the catalog
	Free       bool    `json:"free"`
	PriceIn    float64 `json:"price_in"`
	PriceOut   float64 `json:"price_out"`
	Tools      bool    `json:"tools"`
	Deprecated bool    `json:"deprecated"`
	Desc       string  `json:"description"`
	Reasoning  bool    `json:"reasoning"`
	Context    int     `json:"context"`
	DataUse    string  `json:"data_use"` // paid | zero_retention | trains | unknown (free models)
}

// What Zen's free models do with prompts, from the OpenCode Zen documentation: some may use
// them to improve their models during the free period, some keep nothing. Free models that are
// not listed are "unknown" and treated like the ones that train. The lists are prefixes.
var (
	opencodeTrainsPrefixes = []string{"big-pickle", "exo", "mimo", "ling-", "muse-spark", "nemotron"}
	opencodeZeroPrefixes   = []string{"space-bunny", "longcat", "step-"}
)

func opencodeDataUse(id string, free bool) string {
	if !free {
		return "paid"
	}
	id = strings.ToLower(id)
	for _, p := range opencodeZeroPrefixes {
		if strings.HasPrefix(id, p) {
			return "zero_retention"
		}
	}
	for _, p := range opencodeTrainsPrefixes {
		if strings.HasPrefix(id, p) {
			return "trains"
		}
	}
	return "unknown"
}

// mayTrain reports whether prompts sent to the model may be used for training (or the terms
// are unknown).
func (m opencodeModel) mayTrain() bool { return m.DataUse == "trains" || m.DataUse == "unknown" }

// supported reports whether WRM can talk to the model (the assistant needs tool calls).
func (m opencodeModel) supported() bool {
	return (m.API == "messages" || m.API == "chat") && m.Tools
}

var opencodeCatalog struct {
	sync.Mutex
	models  map[string]opencodeModel
	fetched time.Time
	err     error
}

// opencodeAPIFor maps the catalog's AI SDK package to the API WRM must speak.
func opencodeAPIFor(npm string) string {
	switch npm {
	case "@ai-sdk/anthropic":
		return "messages"
	case "@ai-sdk/openai":
		return "responses"
	case "@ai-sdk/google":
		return "google"
	}
	return "chat"
}

// opencodeGuessAPI is the fallback when the catalog is unreachable.
func opencodeGuessAPI(id string) string {
	id = strings.ToLower(id)
	switch {
	case strings.HasPrefix(id, "claude"):
		return "messages"
	case strings.HasPrefix(id, "gpt-"), strings.HasPrefix(id, "grok"), strings.HasPrefix(id, "muse"):
		return "responses"
	case strings.HasPrefix(id, "gemini"):
		return "google"
	}
	return "chat"
}

func opencodeGuessFree(id string) bool {
	id = strings.ToLower(id)
	return strings.HasSuffix(id, "-free") || strings.Contains(id, "-free-") || id == "big-pickle"
}

// parseOpencodeCatalog reads the "opencode" provider of a models.dev api.json document.
func parseOpencodeCatalog(b []byte) (map[string]opencodeModel, error) {
	var doc map[string]struct {
		NPM    string `json:"npm"`
		API    string `json:"api"`
		Models map[string]struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			ToolCall    bool   `json:"tool_call"`
			Reasoning   bool   `json:"reasoning"`
			Status      string `json:"status"`
			Limit       struct {
				Context int `json:"context"`
			} `json:"limit"`
			Provider struct {
				NPM string `json:"npm"`
				API string `json:"api"`
			} `json:"provider"`
			Cost *struct {
				Input  float64 `json:"input"`
				Output float64 `json:"output"`
			} `json:"cost"`
		} `json:"models"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("the models.dev catalog is not valid JSON")
	}
	prov, ok := doc["opencode"]
	if !ok || len(prov.Models) == 0 {
		return nil, fmt.Errorf("the models.dev catalog has no OpenCode models")
	}
	out := map[string]opencodeModel{}
	for key, m := range prov.Models {
		id := nonEmpty(m.ID, key)
		if id == "" || len(id) > 200 || strings.ContainsAny(id, " \t\"'<>,") {
			continue
		}
		npm := nonEmpty(m.Provider.NPM, prov.NPM)
		om := opencodeModel{ID: id, Name: nonEmpty(m.Name, id), API: opencodeAPIFor(npm), Base: strings.TrimRight(nonEmpty(m.Provider.API, prov.API), "/"),
			Tools: m.ToolCall, Deprecated: m.Status == "deprecated", Desc: truncateStr(m.Description, 200), Reasoning: m.Reasoning, Context: m.Limit.Context}
		if m.Cost != nil {
			om.PriceIn, om.PriceOut = m.Cost.Input, m.Cost.Output
			om.Free = m.Cost.Input == 0 && m.Cost.Output == 0
		} else {
			om.Free = opencodeGuessFree(id)
		}
		om.DataUse = opencodeDataUse(id, om.Free)
		out[id] = om
	}
	return out, nil
}

// opencodeModels returns the catalog, fetched at most every six hours (a failure is
// retried after five minutes). The error is set only when no catalog was ever loaded.
func opencodeModels(ctx context.Context) (map[string]opencodeModel, error) {
	c := &opencodeCatalog
	c.Lock()
	defer c.Unlock()
	fresh := 6 * time.Hour
	if c.models == nil {
		fresh = 5 * time.Minute
	}
	if !c.fetched.IsZero() && time.Since(c.fetched) < fresh {
		return c.models, c.err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	models, err := func() (map[string]opencodeModel, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, opencodeCatalogURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "WRM-PRO/"+AppVersion)
		resp, err := aiHTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("cannot reach models.dev: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return nil, fmt.Errorf("models.dev answered HTTP %d", resp.StatusCode)
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		if err != nil {
			return nil, err
		}
		return parseOpencodeCatalog(b)
	}()
	c.fetched = time.Now()
	if err == nil {
		c.models, c.err = models, nil
	} else if c.models == nil {
		c.err = err
	}
	return c.models, c.err
}

// opencodeModelInfo describes one model, from the catalog when it lists the model.
func opencodeModelInfo(ctx context.Context, id string) opencodeModel {
	if cat, _ := opencodeModels(ctx); cat != nil {
		if m, ok := cat[id]; ok {
			return m
		}
	}
	free := opencodeGuessFree(id)
	return opencodeModel{ID: id, Name: id, API: opencodeGuessAPI(id), Free: free, Tools: true, DataUse: opencodeDataUse(id, free)}
}

// opencodeKey is the key sent to Zen: the configured one, or "public" (free models only).
func opencodeKey(p aiProvider) string {
	if p.secret.APIKey != "" {
		return p.secret.APIKey
	}
	return opencodePublicKey
}

// opencodeEndpoint returns the base URL for a model: the provider's override, else the
// model's own endpoint from the catalog, else Zen's default.
func opencodeEndpoint(p aiProvider, m opencodeModel) string {
	if p.BaseURL != "" {
		return p.BaseURL
	}
	if m.Base != "" {
		return m.Base
	}
	return opencodeDefaultBase
}

func opencodeComplete(ctx context.Context, p aiProvider, req aiLLMRequest, onText func(string)) (aiLLMResponse, error) {
	m := opencodeModelInfo(ctx, req.Model)
	if p.secret.APIKey == "" && !m.Free {
		return aiLLMResponse{}, fmt.Errorf("%s is not a free OpenCode Zen model: add an OpenCode Zen API key to the provider, or pick a free model", req.Model)
	}
	key := opencodeKey(p)
	base := opencodeEndpoint(p, m)
	switch m.API {
	case "messages":
		return anthropicStream(ctx, p, base+"/messages", req, onText, func(h http.Header) {
			h.Set("x-api-key", key)
			h.Set("Authorization", "Bearer "+key)
		}, false)
	case "chat":
		return openaiStream(ctx, p, base+"/chat/completions", req, onText, func(h http.Header) {
			h.Set("Authorization", "Bearer "+key)
		})
	}
	return aiLLMResponse{}, fmt.Errorf("%s uses the %s API, which WRM does not support yet; pick a Claude model or a model served on chat completions", req.Model, map[string]string{"responses": "OpenAI Responses", "google": "Google Gemini"}[m.API])
}

// opencodeListModels lists the models WRM can use: with a key every supported model, without
// one only the free ones. Deprecated models are left out unless nothing else is free.
func opencodeListModels(ctx context.Context, p aiProvider) ([]opencodeModel, error) {
	cat, catErr := opencodeModels(ctx)
	var all []opencodeModel
	if cat != nil {
		for _, m := range cat {
			all = append(all, m)
		}
	} else {
		// fallback: Zen's own list, classified by name
		q := p
		q.Kind = "openai_compatible"
		q.BaseURL = nonEmpty(p.BaseURL, opencodeDefaultBase)
		q.secret.APIKey = opencodeKey(p)
		ids, err := aiListModels(ctx, q)
		if err != nil {
			return nil, fmt.Errorf("%v (and %v)", err, catErr)
		}
		for _, id := range ids {
			free := opencodeGuessFree(id)
			all = append(all, opencodeModel{ID: id, Name: id, API: opencodeGuessAPI(id), Free: free, Tools: true, DataUse: opencodeDataUse(id, free)})
		}
	}
	pick := func(withDeprecated bool) []opencodeModel {
		var out []opencodeModel
		for _, m := range all {
			if !m.supported() || (p.secret.APIKey == "" && !m.Free) || (m.Deprecated && !withDeprecated) {
				continue
			}
			out = append(out, m)
		}
		return out
	}
	out := pick(false)
	if len(out) == 0 {
		out = pick(true)
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

// opencodePrice returns the catalog price of a model (USD per million tokens).
func opencodePrice(model string) (float64, float64, bool) {
	c := &opencodeCatalog
	c.Lock()
	defer c.Unlock()
	if m, ok := c.models[model]; ok {
		return m.PriceIn, m.PriceOut, true
	}
	return 0, 0, false
}
