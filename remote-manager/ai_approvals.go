// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// ─── AI: APPROVALS KEPT FOR LATER, AND WHY A DECISION FAILED ─
//
// Over MCP the AI app often stops waiting before anybody answers (its own tool timeout, or
// nobody looked). The request then stays open in WRM. With ai_mcp_later_approval = run (the
// default) approving it runs the call at once, the user sees the output in the live view, and
// when the AI repeats the same call it gets that output instead of running it a second time.
// With "repeat" an approval only lets the AI repeat the same call once.
//
// A decision on a request that is no longer waiting says what happened to it (approved or
// denied by whom, in WRM or in the AI app, or closed) instead of a bare error.

// aiViaMCPDeclined: the AI app closed WRM's approval question without an answer.
const aiViaMCPDeclined = "mcp_declined"

func aiLaterRunsNow() bool { return getSetting("ai_mcp_later_approval") != "repeat" }

// aiLaterRun is a call WRM ran when the user approved it after the AI stopped waiting.
type aiLaterRun struct {
	at   time.Time
	by   string
	done chan struct{}
	res  aiToolResult
}

// runLater runs an approved request kept for later, in the background.
func (s *aiSession) runLater(a *aiApproval, by string) {
	lr := &aiLaterRun{at: time.Now(), by: by, done: make(chan struct{})}
	s.mu.Lock()
	if s.laterRuns == nil {
		s.laterRuns = map[string]*aiLaterRun{}
	}
	s.laterRuns[a.key] = lr
	ctx := s.ctx
	s.mu.Unlock()
	pl := *a.pl
	pl.Approved = true
	pl.ForceRedact = pl.ForceRedact || pl.Rule == "sensitive_read"
	s.audit(nil, "ai.later_run", map[string]interface{}{"approval": a.ID, "tool": a.Tool, "by": by})
	go func() {
		defer close(lr.done)
		// the mode or the policies may have changed since the request was made: check again
		s.recheckMode()
		s.mu.Lock()
		mode, home := s.Mode, s.home
		s.mu.Unlock()
		dec := aiEvaluate(mode, nil, pl.Act, home, time.Now())
		if s.fullAccess() {
			dec = aiFullAccessDecision(dec, mode)
		}
		if dec.Action == aiDeny || (mode == aiModeReadOnly && (pl.Write || pl.Transfer)) {
			why := nonEmpty(dec.Reason, "the session is read-only now")
			s.audit(nil, "ai.tool_denied", map[string]interface{}{"tool": a.Tool, "rule": nonEmpty(dec.Rule, "mode"), "reason": why, "later_run": true})
			lr.res = aiToolResult{ID: a.call.ID, Name: a.call.Name, IsError: true, Content: "Denied by WRM when it was to run after the approval: " + why + "."}
			return
		}
		lr.res = s.run(ctx, a.call, &pl)
	}()
}

// takeLaterRun hands out (once) the run of an approved later request for the same call.
func (s *aiSession) takeLaterRun(key string) *aiLaterRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	lr := s.laterRuns[key]
	if lr != nil {
		delete(s.laterRuns, key)
	}
	return lr
}

// laterRunResult answers a repeated call with the output of the run the user approved.
func (s *aiSession) laterRunResult(ctx context.Context, call aiToolCall, lr *aiLaterRun) aiToolResult {
	select {
	case <-lr.done:
	case <-ctx.Done():
		return aiToolResult{ID: call.ID, Name: call.Name, IsError: true, Content: "Not finished yet: the approved call is still running in WRM. Call the tool again with the same arguments in a moment to get its output."}
	}
	when := lr.at.UTC().Format("15:04:05 UTC")
	s.audit(nil, "ai.approved", map[string]interface{}{"tool": call.Name, "via": "later_run", "by": lr.by})
	s.emit("approval_resolved", map[string]interface{}{"call_id": call.ID, "outcome": "approved", "via": "later_run", "by": lr.by})
	code := 0
	if lr.res.IsError {
		code = 1
	}
	s.emit("tool_result", map[string]interface{}{"call_id": call.ID, "tool": call.Name, "exit_code": code, "later_run": true, "ran_at": lr.at.UTC().Format(time.RFC3339)})
	s.transcript("tool_result", "WRM", "(the output of the run approved by "+lr.by+" at "+when+" was handed to the AI)", map[string]interface{}{"call_id": call.ID, "tool": call.Name, "later_run": true})
	return aiToolResult{ID: call.ID, Name: call.Name, IsError: lr.res.IsError,
		Content: "[" + lr.by + " approved this call in WRM at " + when + ", after you had stopped waiting, and WRM ran it then. This is its output; it was not run again.]\n" + lr.res.Content}
}

// ── a decision on a request that no longer waits ─────

type aiNotPendingErr struct {
	Info map[string]interface{}
}

func (e *aiNotPendingErr) Error() string {
	i := e.Info
	outcome, _ := i["outcome"].(string)
	by, _ := i["by"].(string)
	where := " in WRM"
	if v, _ := i["via"].(string); strings.HasPrefix(v, "mcp") {
		where = " in the AI app"
	}
	if by != "" && by != "WRM" {
		by = " by " + by
	} else {
		by = ""
	}
	switch outcome {
	case "approved", "denied":
		return "This request was already " + outcome + by + where + "."
	case "timeout", "cancelled":
		return "The AI stopped waiting for this request and it was closed. Ask the AI to repeat it."
	}
	return "This request is no longer waiting (the AI session ended or the request expired). Ask the AI to repeat it."
}

// aiNotPending looks up the last decision on an approval in the stored transcript.
func aiNotPending(sessionID int64, approvalID string) error {
	info := map[string]interface{}{"approval": approvalID, "outcome": ""}
	var actor, ts, meta string
	if db.QueryRow(`SELECT actor, ts, meta FROM ai_messages WHERE session_id=? AND kind='decision'
		AND CASE WHEN json_valid(meta) THEN json_extract(meta, '$.approval') END = ? ORDER BY seq DESC, id DESC LIMIT 1`, sessionID, approvalID).Scan(&actor, &ts, &meta) == nil {
		var m map[string]interface{}
		if json.Unmarshal([]byte(meta), &m) == nil {
			for _, k := range []string{"outcome", "via", "later", "run_now", "kept_for_later"} {
				if v, ok := m[k]; ok {
					info[k] = v
				}
			}
		}
		info["by"] = actor
		info["at"] = ts
	}
	return &aiNotPendingErr{Info: info}
}

// aiDecisionError answers a failed decision: 409 with what happened to the request.
func aiDecisionError(w http.ResponseWriter, err error) {
	var np *aiNotPendingErr
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(409)
	out := map[string]interface{}{"error": err.Error()}
	if errors.As(err, &np) {
		out["decided"] = np.Info
	}
	json.NewEncoder(w).Encode(out)
}
