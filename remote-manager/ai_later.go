// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strconv"
	"sync"
	"time"
)

// Approvals kept for later outlive their AI session: an MCP app often closes its session
// right after WRM stopped waiting. The user can still approve such a request in WRM (same
// URL, the ended session's id), and the approval then covers the same call once in any new
// session of that user on the same connection.
var aiLaterStore = struct {
	sync.Mutex
	open     map[string]*aiLaterOpen // approval id → kept request of an ended session
	approved map[string]time.Time    // user, connection, call key → valid until
}{open: map[string]*aiLaterOpen{}, approved: map[string]time.Time{}}

type aiLaterOpen struct {
	s *aiSession
	a *aiApproval
}

func aiLaterKey(userID, connID int, key string) string {
	return strconv.Itoa(userID) + "\x00" + strconv.Itoa(connID) + "\x00" + key
}

// aiKeepLater moves an ending session's requests kept for later, and its later approvals
// the AI did not use yet, to the user's store; s.mu is held.
func (s *aiSession) aiKeepLater(now time.Time) {
	aiLaterStore.Lock()
	defer aiLaterStore.Unlock()
	for _, a := range s.pending {
		if a.Later && now.Before(a.Expires) {
			a.RunNow = false // the session has ended: an approval lets a new session run it once
			aiLaterStore.open[a.ID] = &aiLaterOpen{s: s, a: a}
		}
	}
	for k, until := range s.approvedLater {
		if now.Before(until) {
			aiLaterStore.approved[aiLaterKey(s.UserID, s.ConnID, k)] = until
		}
	}
}

// aiTakeStoredLater uses up an approval given after the session that asked had ended.
func aiTakeStoredLater(userID, connID int, key string) bool {
	aiLaterStore.Lock()
	defer aiLaterStore.Unlock()
	k := aiLaterKey(userID, connID, key)
	until, ok := aiLaterStore.approved[k]
	delete(aiLaterStore.approved, k)
	return ok && time.Now().Before(until)
}

// aiResolveStoredLater decides a kept request of an ended session (sessionID from the URL).
// found is false when there is no such request for this user.
func aiResolveStoredLater(sessionID int64, approvalID string, userID int, approve bool) (found bool, err error) {
	aiLaterStore.Lock()
	o := aiLaterStore.open[approvalID]
	if o == nil || o.s.ID != sessionID || o.s.UserID != userID {
		aiLaterStore.Unlock()
		return false, nil
	}
	delete(aiLaterStore.open, approvalID)
	if time.Now().After(o.a.Expires) {
		aiLaterStore.Unlock()
		return true, fmt.Errorf("the request expired; ask the AI to repeat it")
	}
	outcome := "denied"
	if approve {
		outcome = "approved"
		aiLaterStore.approved[aiLaterKey(o.s.UserID, o.s.ConnID, o.a.key)] = time.Now().Add(aiLaterTTL)
	}
	aiLaterStore.Unlock()
	d := map[string]interface{}{"approval": o.a.ID, "tool": o.a.Tool, "outcome": outcome, "by": usernameOf(userID), "later": true, "session_ended": true}
	o.s.audit(nil, map[bool]string{true: "ai.approved", false: "ai.denied"}[approve], d)
	return true, nil
}

// aiStoredLaterFor lists the user's kept requests of ended sessions (top bar, live panel).
func aiStoredLaterFor(userID int) []map[string]interface{} {
	aiLaterStore.Lock()
	defer aiLaterStore.Unlock()
	out := []map[string]interface{}{}
	now := time.Now()
	for _, o := range aiLaterStore.open {
		if o.s.UserID == userID && now.Before(o.a.Expires) {
			out = append(out, map[string]interface{}{"session": o.s.ID, "connection": o.s.ConnName, "host": o.s.Host, "client": o.s.Model, "approval": o.a, "session_ended": true})
		}
	}
	return out
}

// aiLaterJanitor drops expired entries.
func aiLaterJanitor(now time.Time) {
	aiLaterStore.Lock()
	defer aiLaterStore.Unlock()
	for id, o := range aiLaterStore.open {
		if now.After(o.a.Expires) {
			delete(aiLaterStore.open, id)
		}
	}
	for k, until := range aiLaterStore.approved {
		if now.After(until) {
			delete(aiLaterStore.approved, k)
		}
	}
}

func parseID(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
