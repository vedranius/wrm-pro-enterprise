// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ─── COLLABORATION ROOMS ─────────────────────────────
//
// Everyone who opens the same share joins its room over /ws/share/<token>. The server
// decides who a participant is (account name or guest name) and what they may do (their
// role in the share); clients cannot claim another identity.
//
// Features: presence, chat with history, file exchange, terminal sharing (each shared
// terminal is a "stream" that others can watch; watchers get a snapshot of the screen),
// remote keyboard control (granted by the sharer, can be requested), voice calls (WebRTC,
// the server only relays signalling), raised hands and moderation (roles, mute, kick, ban).
//
// Every participant has its own send queue and writer goroutine, so a slow network on one
// side never blocks the room.

const (
	roomMaxParticipants = 100
	roomHistoryMax      = 150
	roomHistoryMaxBytes = 24 << 20
	participantQueue    = 1024
)

type outMsg struct {
	data   []byte
	close  bool
	code   int
	reason string
}

type screenStream struct {
	Title    string          `json:"title"`
	Cols     int             `json:"cols"`
	Rows     int             `json:"rows"`
	watchers map[string]bool // viewer pid → true
}

type participant struct {
	pid       string
	ws        *websocket.Conn
	send      chan outMsg
	done      chan struct{}
	closeOnce sync.Once

	sa     *ShareAccess
	name   string
	ip     string
	joined time.Time

	streams     map[string]*screenStream
	voice       bool
	muted       bool
	deafened    bool
	serverMuted bool
	hand        bool

	rateWindow time.Time
	rateCount  int
	chatTimes  []time.Time
}

type controlGrant struct {
	Sharer string `json:"sharer"`
	SID    string `json:"sid"`
}

type room struct {
	mu        sync.Mutex
	shareID   int
	parts     map[string]*participant
	control   map[string]controlGrant // controller pid → grant
	history   []map[string]interface{}
	histBytes int
	seq       int
}

var rooms = struct {
	sync.Mutex
	m map[int]*room
}{m: map[int]*room{}}

func getRoom(shareID int) *room {
	rooms.Lock()
	defer rooms.Unlock()
	rm, ok := rooms.m[shareID]
	if !ok {
		rm = &room{shareID: shareID, parts: map[string]*participant{}, control: map[string]controlGrant{}}
		rm.loadHistory()
		rooms.m[shareID] = rm
	}
	return rm
}

func findRoom(shareID int) *room {
	rooms.Lock()
	defer rooms.Unlock()
	return rooms.m[shareID]
}

func dropRoomIfEmpty(rm *room) {
	rooms.Lock()
	defer rooms.Unlock()
	rm.mu.Lock()
	empty := len(rm.parts) == 0
	rm.mu.Unlock()
	if empty && rooms.m[rm.shareID] == rm {
		delete(rooms.m, rm.shareID)
	}
}

func (rm *room) loadHistory() {
	rows, err := db.Query(`SELECT id, ts, name, kind, text FROM (SELECT id, ts, name, kind, text FROM collab_messages WHERE share_id=? ORDER BY id DESC LIMIT 100) ORDER BY id`, rm.shareID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var ts, name, kind, text string
		rows.Scan(&id, &ts, &name, &kind, &text)
		m := map[string]interface{}{"type": kind, "id": id, "name": name, "ts": ts}
		if kind == "file" {
			m["file"] = text
		} else {
			m["text"] = text
		}
		rm.history = append(rm.history, m)
	}
}

// ─── SENDING ─────────────────────────────────────────

func (p *participant) enqueue(data []byte) {
	select {
	case p.send <- outMsg{data: data}:
	case <-p.done:
	default:
		log.Printf("collab: %s is too slow, disconnecting", p.name)
		p.shutdown(4008, "connection too slow")
	}
}

func (p *participant) sendJSON(v interface{}) { p.enqueue(jsonMarshal(v)) }

// closeWith sends a last message and closes the connection with a close code.
func (p *participant) closeWith(v interface{}, code int, reason string) {
	select {
	case p.send <- outMsg{data: jsonMarshal(v)}:
	default:
	}
	select {
	case p.send <- outMsg{close: true, code: code, reason: reason}:
	default:
		p.shutdown(code, reason)
	}
}

// shutdown closes the connection without blocking the caller (it may hold the room lock).
func (p *participant) shutdown(code int, reason string) {
	p.closeOnce.Do(func() {
		close(p.done)
		go func() {
			p.ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, truncateStr(reason, 120)), time.Now().Add(2*time.Second))
			p.ws.Close()
		}()
	})
}

func (p *participant) writer() {
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case m := <-p.send:
			if m.close {
				p.shutdown(m.code, m.reason)
				return
			}
			p.ws.SetWriteDeadline(time.Now().Add(20 * time.Second))
			if err := p.ws.WriteMessage(websocket.TextMessage, m.data); err != nil {
				p.shutdown(1001, "write error")
				return
			}
		case <-ping.C:
			if err := p.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
				p.shutdown(1001, "ping failed")
				return
			}
		case <-p.done:
			return
		}
	}
}

func (rm *room) broadcastLocked(v interface{}, except *participant) {
	b := jsonMarshal(v)
	for _, p := range rm.parts {
		if p != except {
			p.enqueue(b)
		}
	}
}

func (rm *room) systemLocked(text string, except *participant) {
	rm.broadcastLocked(map[string]interface{}{"type": "system", "text": text, "ts": time.Now().UTC().Format(time.RFC3339)}, except)
}

// ─── PRESENCE ────────────────────────────────────────

type streamInfo struct {
	SID   string `json:"sid"`
	Title string `json:"title"`
	Cols  int    `json:"cols"`
	Rows  int    `json:"rows"`
}

func (p *participant) info(rm *room) map[string]interface{} {
	streams := []streamInfo{}
	for sid, s := range p.streams {
		streams = append(streams, streamInfo{SID: sid, Title: s.Title, Cols: s.Cols, Rows: s.Rows})
	}
	sort.Slice(streams, func(i, j int) bool { return streams[i].SID < streams[j].SID })
	var control interface{}
	if g, ok := rm.control[p.pid]; ok {
		control = g
	}
	return map[string]interface{}{
		"pid": p.pid, "name": p.name, "guest": p.sa.Guest, "member": p.sa.Member, "owner": p.sa.IsOwner,
		"role": p.sa.Role, "streams": streams, "voice": p.voice, "muted": p.muted || p.serverMuted,
		"deafened": p.deafened, "server_muted": p.serverMuted, "hand": p.hand, "control": control,
		"joined_at": p.joined.UTC().Format(time.RFC3339),
	}
}

func (rm *room) participantsLocked() []map[string]interface{} {
	list := make([]*participant, 0, len(rm.parts))
	for _, p := range rm.parts {
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool {
		if roleRank[list[i].sa.Role] != roleRank[list[j].sa.Role] {
			return roleRank[list[i].sa.Role] > roleRank[list[j].sa.Role]
		}
		return list[i].joined.Before(list[j].joined)
	})
	out := make([]map[string]interface{}, 0, len(list))
	for _, p := range list {
		out = append(out, p.info(rm))
	}
	return out
}

func (rm *room) presenceLocked() {
	rm.broadcastLocked(map[string]interface{}{"type": "participants", "participants": rm.participantsLocked()}, nil)
}

func (rm *room) uniqueNameLocked(base string) string {
	name := base
	for i := 2; ; i++ {
		taken := false
		for _, p := range rm.parts {
			if strings.EqualFold(p.name, name) {
				taken = true
				break
			}
		}
		if !taken {
			return name
		}
		name = fmt.Sprintf("%s (%d)", base, i)
	}
}

// ─── WEBSOCKET HANDLER ───────────────────────────────

type inMsg struct {
	Type     string          `json:"type"`
	Text     string          `json:"text"`
	Name     string          `json:"name"`
	Mime     string          `json:"mime"`
	Data     string          `json:"data"`
	Action   string          `json:"action"`
	SID      string          `json:"sid"`
	Title    string          `json:"title"`
	Cols     int             `json:"cols"`
	Rows     int             `json:"rows"`
	PID      string          `json:"pid"`
	To       string          `json:"to"`
	Role     string          `json:"role"`
	Muted    *bool           `json:"muted"`
	Deafened *bool           `json:"deafened"`
	Up       bool            `json:"up"`
	Signal   json.RawMessage `json:"signal"`
}

func shareWSHandler(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(strings.SplitN(strings.Trim(strings.TrimPrefix(r.URL.Path, "/ws/share/"), "/"), "/", 2)[0])
	sa, d := resolveShareToken(r, token)
	if d != nil {
		http.Error(w, d.Message, d.Code)
		return
	}
	rm := getRoom(sa.Share.ID)
	rm.mu.Lock()
	full := len(rm.parts) >= roomMaxParticipants
	rm.mu.Unlock()
	if full {
		http.Error(w, "The room is full", 503)
		return
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	maxFile := settingInt("chat_file_max_mb") << 20
	ws.SetReadLimit(int64(maxFile*4/3 + 256<<10))
	if maxFile < 1<<20 {
		ws.SetReadLimit(1 << 20)
	}

	p := &participant{
		pid: randomToken(9), ws: ws, send: make(chan outMsg, participantQueue), done: make(chan struct{}),
		sa: sa, ip: clientIP(r), joined: time.Now(), streams: map[string]*screenStream{},
	}
	base := sa.Name
	if sa.Guest {
		var n int
		db.QueryRow(`SELECT COUNT(1) FROM users WHERE username=? COLLATE NOCASE`, base).Scan(&n)
		if n > 0 {
			base += " (guest)"
		}
	}
	touchParticipant(sa, p.ip)
	go p.writer()

	rm.mu.Lock()
	p.name = rm.uniqueNameLocked(base)
	rm.parts[p.pid] = p
	hist := make([]map[string]interface{}, len(rm.history))
	copy(hist, rm.history)
	p.sendJSON(map[string]interface{}{
		"type": "welcome", "pid": p.pid, "name": p.name, "role": sa.Role, "perms": rolePerms(sa.Role),
		"guest": sa.Guest, "owner": sa.IsOwner, "chat_file_max_mb": settingInt("chat_file_max_mb"),
		"share":        map[string]interface{}{"id": sa.Share.ID, "name": sa.Share.Name},
		"participants": rm.participantsLocked(), "history": hist,
		"voice": map[string]interface{}{
			"enabled": settingBool("voice_enabled"), "max": settingInt("voice_max_participants"),
			"ice_servers": iceServersFor(r, p.pid),
		},
	})
	rm.presenceLocked()
	rm.systemLocked(p.name+" joined", p)
	rm.mu.Unlock()
	auditLogAs(r, sa.UserID, participantAuditName(sa), "collab.join", sa.Share.Name, map[string]interface{}{"share_id": sa.Share.ID, "role": sa.Role, "as": p.name})

	ws.SetReadDeadline(time.Now().Add(75 * time.Second))
	ws.SetPongHandler(func(string) error { ws.SetReadDeadline(time.Now().Add(75 * time.Second)); return nil })
	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			break
		}
		ws.SetReadDeadline(time.Now().Add(75 * time.Second))
		var m inMsg
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		rm.handle(p, &m, r)
	}

	rm.mu.Lock()
	rm.leaveLocked(p)
	rm.mu.Unlock()
	p.shutdown(1000, "bye")
	dropRoomIfEmpty(rm)
	auditLogAs(r, sa.UserID, participantAuditName(sa), "collab.leave", sa.Share.Name, map[string]interface{}{"share_id": sa.Share.ID, "minutes": int(time.Since(p.joined).Minutes())})
}

func participantAuditName(sa *ShareAccess) string {
	if sa.Guest {
		return sa.Name + " (guest)"
	}
	return sa.Username
}

func (rm *room) leaveLocked(p *participant) {
	if _, ok := rm.parts[p.pid]; !ok {
		return
	}
	delete(rm.parts, p.pid)
	rm.stopStreamsLocked(p, "")
	delete(rm.control, p.pid)
	for _, other := range rm.parts {
		for _, s := range other.streams {
			delete(s.watchers, p.pid)
		}
	}
	if p.voice {
		rm.broadcastLocked(map[string]interface{}{"type": "voice-left", "pid": p.pid}, nil)
	}
	rm.presenceLocked()
	rm.systemLocked(p.name+" left", nil)
}

// rateOK allows short bursts (40 messages / 2 s) of control messages per participant.
func (p *participant) rateOK() bool {
	now := time.Now()
	if now.Sub(p.rateWindow) > 2*time.Second {
		p.rateWindow, p.rateCount = now, 0
	}
	p.rateCount++
	return p.rateCount <= 40
}

func (p *participant) chatOK() bool {
	now := time.Now()
	keep := p.chatTimes[:0]
	for _, t := range p.chatTimes {
		if now.Sub(t) < 10*time.Second {
			keep = append(keep, t)
		}
	}
	p.chatTimes = keep
	if len(p.chatTimes) >= 10 {
		return false
	}
	p.chatTimes = append(p.chatTimes, now)
	return true
}

func (rm *room) handle(p *participant, m *inMsg, r *http.Request) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if _, ok := rm.parts[p.pid]; !ok {
		return
	}
	switch m.Type {
	case "screen", "remote-input", "voice-signal", "ping":
	default:
		if !p.rateOK() {
			return
		}
	}
	errorTo := func(msg string) { p.sendJSON(map[string]string{"type": "error", "message": msg}) }
	switch m.Type {
	case "ping":
		p.sendJSON(map[string]string{"type": "pong"})

	case "chat":
		text := strings.TrimSpace(m.Text)
		if text == "" {
			return
		}
		if len(text) > 4000 {
			text = text[:4000]
		}
		if !p.chatOK() {
			errorTo("You are sending messages too fast")
			return
		}
		rm.seq++
		ts := time.Now().UTC().Format(time.RFC3339)
		res, err := db.Exec(`INSERT INTO collab_messages (share_id, ts, pkey, name, kind, text) VALUES (?,?,?,?,?,?)`, rm.shareID, ts, p.sa.PKey, p.name, "chat", text)
		id := int64(rm.seq)
		if err == nil {
			id, _ = res.LastInsertId()
		}
		msg := map[string]interface{}{"type": "chat", "id": id, "pid": p.pid, "name": p.name, "guest": p.sa.Guest, "role": p.sa.Role, "text": text, "ts": ts}
		rm.addHistoryLocked(msg, len(text))
		rm.broadcastLocked(msg, nil)

	case "file":
		max := settingInt("chat_file_max_mb") << 20
		if max == 0 {
			errorTo("File sharing in chat is disabled by the administrator")
			return
		}
		if len(m.Data) > max*4/3+8 || m.Data == "" {
			errorTo(fmt.Sprintf("File too large (max %d MB)", max>>20))
			return
		}
		if !p.chatOK() {
			errorTo("You are sending messages too fast")
			return
		}
		name := cleanDisplayName(m.Name)
		if name == "" {
			name = "file"
		}
		ts := time.Now().UTC().Format(time.RFC3339)
		db.Exec(`INSERT INTO collab_messages (share_id, ts, pkey, name, kind, text) VALUES (?,?,?,?,?,?)`, rm.shareID, ts, p.sa.PKey, p.name, "file", name)
		rm.seq++
		msg := map[string]interface{}{"type": "file", "id": rm.seq, "pid": p.pid, "name": p.name, "guest": p.sa.Guest, "file": name,
			"mime": truncateStr(m.Mime, 100), "size": len(m.Data) * 3 / 4, "data": m.Data, "ts": ts}
		rm.addHistoryLocked(msg, len(m.Data))
		rm.broadcastLocked(msg, nil)
		auditLogAs(r, p.sa.UserID, participantAuditName(p.sa), "collab.file_shared", name, map[string]interface{}{"share_id": rm.shareID, "bytes": len(m.Data) * 3 / 4})

	case "set-name":
		if !p.sa.Guest {
			return
		}
		name := cleanDisplayName(m.Name)
		if name == "" {
			return
		}
		old := p.name
		p.sa.Name = name
		db.Exec(`UPDATE share_participants SET name=? WHERE share_id=? AND pkey=?`, name, rm.shareID, p.sa.PKey)
		p.name = ""
		p.name = rm.uniqueNameLocked(name)
		p.sendJSON(map[string]string{"type": "renamed", "name": p.name})
		rm.presenceLocked()
		rm.systemLocked(old+" is now "+p.name, nil)

	case "screen":
		rm.handleScreenLocked(p, m, errorTo)

	case "watch", "unwatch":
		owner := rm.parts[m.PID]
		if owner == nil || owner == p {
			return
		}
		s := owner.streams[m.SID]
		if s == nil {
			if m.Type == "watch" {
				p.sendJSON(map[string]interface{}{"type": "screen", "action": "stop", "pid": m.PID, "sid": m.SID})
			}
			return
		}
		if m.Type == "watch" {
			s.watchers[p.pid] = true
			owner.sendJSON(map[string]interface{}{"type": "watch-request", "sid": m.SID, "viewer": p.pid, "name": p.name})
		} else {
			delete(s.watchers, p.pid)
			if g, ok := rm.control[p.pid]; ok && g.Sharer == owner.pid && g.SID == m.SID {
				rm.revokeControlLocked(p.pid, "")
			}
		}

	case "control-grant":
		target := rm.parts[m.PID]
		if target == nil || target == p || p.streams[m.SID] == nil {
			return
		}
		if !roleCan(target.sa.Role, PermControl) {
			errorTo(target.name + "'s role (" + target.sa.Role + ") cannot receive keyboard control")
			return
		}
		rm.control[target.pid] = controlGrant{Sharer: p.pid, SID: m.SID}
		target.sendJSON(map[string]interface{}{"type": "control", "granted": true, "sharer": p.pid, "sid": m.SID, "by": p.name})
		rm.presenceLocked()
		rm.systemLocked(p.name+" gave keyboard control to "+target.name, nil)
		auditLogAs(r, p.sa.UserID, participantAuditName(p.sa), "collab.control_granted", target.name, map[string]interface{}{"share_id": rm.shareID, "terminal": p.streams[m.SID].Title})

	case "control-revoke":
		if g, ok := rm.control[m.PID]; ok && g.Sharer == p.pid {
			rm.revokeControlLocked(m.PID, p.name)
		}

	case "control-release":
		if _, ok := rm.control[p.pid]; ok {
			rm.revokeControlLocked(p.pid, p.name)
		}

	case "control-request":
		sharer := rm.parts[m.PID]
		if sharer == nil || sharer == p || sharer.streams[m.SID] == nil {
			return
		}
		if !roleCan(p.sa.Role, PermControl) {
			errorTo("Your role cannot take keyboard control")
			return
		}
		sharer.sendJSON(map[string]interface{}{"type": "control-request", "from": p.pid, "name": p.name, "sid": m.SID, "title": sharer.streams[m.SID].Title})

	case "remote-input":
		g, ok := rm.control[p.pid]
		if !ok || len(m.Data) > 64<<10 {
			return
		}
		sharer := rm.parts[g.Sharer]
		if sharer == nil || sharer.streams[g.SID] == nil {
			delete(rm.control, p.pid)
			return
		}
		sharer.sendJSON(map[string]interface{}{"type": "remote-input", "from": p.pid, "sid": g.SID, "data": m.Data})

	case "voice-join":
		if !settingBool("voice_enabled") {
			errorTo("Voice calls are disabled by the administrator")
			return
		}
		if p.voice {
			return
		}
		n := 0
		peers := []string{}
		for _, o := range rm.parts {
			if o.voice {
				n++
				peers = append(peers, o.pid)
			}
		}
		if n >= settingInt("voice_max_participants") {
			errorTo(fmt.Sprintf("The voice call is full (max %d people)", settingInt("voice_max_participants")))
			return
		}
		p.voice = true
		p.muted = m.Muted != nil && *m.Muted
		p.deafened = m.Deafened != nil && *m.Deafened
		p.sendJSON(map[string]interface{}{"type": "voice-peers", "peers": peers})
		rm.broadcastLocked(map[string]interface{}{"type": "voice-joined", "pid": p.pid, "name": p.name}, p)
		rm.presenceLocked()

	case "voice-leave":
		if !p.voice {
			return
		}
		p.voice = false
		rm.broadcastLocked(map[string]interface{}{"type": "voice-left", "pid": p.pid}, nil)
		rm.presenceLocked()

	case "voice-signal":
		target := rm.parts[m.To]
		if target == nil || !p.voice || !target.voice || len(m.Signal) > 64<<10 || len(m.Signal) == 0 {
			return
		}
		target.enqueue(jsonMarshal(map[string]interface{}{"type": "voice-signal", "from": p.pid, "signal": m.Signal}))

	case "voice-state":
		if m.Muted != nil {
			p.muted = *m.Muted
		}
		if m.Deafened != nil {
			p.deafened = *m.Deafened
		}
		rm.presenceLocked()

	case "hand":
		if p.hand == m.Up {
			return
		}
		p.hand = m.Up
		rm.presenceLocked()
		if m.Up {
			rm.systemLocked("✋ "+p.name+" raised a hand", nil)
		}

	case "mod":
		rm.moderateLocked(p, m, r, errorTo)
	}
}

func (rm *room) addHistoryLocked(msg map[string]interface{}, size int) {
	rm.history = append(rm.history, msg)
	rm.histBytes += size
	for len(rm.history) > roomHistoryMax || (rm.histBytes > roomHistoryMaxBytes && len(rm.history) > 1) {
		old := rm.history[0]
		if d, ok := old["data"].(string); ok {
			rm.histBytes -= len(d)
		} else if t, ok := old["text"].(string); ok {
			rm.histBytes -= len(t)
		}
		rm.history = rm.history[1:]
	}
}

// ─── TERMINAL SHARING ────────────────────────────────

// validStreamID: stream ids are chosen by the sharing browser; only simple ids are allowed.
func validStreamID(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (rm *room) handleScreenLocked(p *participant, m *inMsg, errorTo func(string)) {
	sid := m.SID
	if !validStreamID(sid) {
		return
	}
	switch m.Action {
	case "start":
		if !roleCan(p.sa.Role, PermShareScreen) {
			errorTo("Your role cannot share terminals")
			return
		}
		if len(p.streams) >= 8 {
			return
		}
		title := cleanDisplayName(m.Title)
		if title == "" {
			title = "Terminal"
		}
		p.streams[sid] = &screenStream{Title: title, Cols: clampInt(m.Cols, 2, 1000), Rows: clampInt(m.Rows, 1, 500), watchers: map[string]bool{}}
		rm.presenceLocked()
		rm.systemLocked("📡 "+p.name+" is sharing "+title, nil)
		rm.broadcastLocked(map[string]interface{}{"type": "screen", "action": "start", "pid": p.pid, "sid": sid, "title": title, "name": p.name}, p)
	case "stop":
		if p.streams[sid] != nil {
			rm.stopStreamsLocked(p, sid)
			rm.presenceLocked()
		}
	case "data", "resize":
		s := p.streams[sid]
		if s == nil {
			return
		}
		msg := map[string]interface{}{"type": "screen", "action": m.Action, "pid": p.pid, "sid": sid}
		if m.Action == "data" {
			if len(m.Data) > 2<<20 {
				return
			}
			msg["data"] = m.Data
		} else {
			s.Cols, s.Rows = clampInt(m.Cols, 2, 1000), clampInt(m.Rows, 1, 500)
			msg["cols"], msg["rows"] = s.Cols, s.Rows
		}
		b := jsonMarshal(msg)
		for w := range s.watchers {
			if v := rm.parts[w]; v != nil {
				v.enqueue(b)
			}
		}
	case "snapshot":
		s := p.streams[sid]
		v := rm.parts[m.To]
		if s == nil || v == nil || !s.watchers[m.To] || len(m.Data) > 8<<20 {
			return
		}
		v.sendJSON(map[string]interface{}{"type": "screen", "action": "snapshot", "pid": p.pid, "sid": sid, "data": m.Data, "cols": s.Cols, "rows": s.Rows, "title": s.Title})
	}
}

// stopStreamsLocked ends one stream (or all when sid is empty) and the control given for it.
func (rm *room) stopStreamsLocked(p *participant, sid string) {
	for id, s := range p.streams {
		if sid != "" && id != sid {
			continue
		}
		for c, g := range rm.control {
			if g.Sharer == p.pid && g.SID == id {
				rm.revokeControlLocked(c, "")
			}
		}
		delete(p.streams, id)
		rm.broadcastLocked(map[string]interface{}{"type": "screen", "action": "stop", "pid": p.pid, "sid": id, "title": s.Title, "name": p.name}, p)
	}
}

func (rm *room) revokeControlLocked(controller, by string) {
	g, ok := rm.control[controller]
	if !ok {
		return
	}
	delete(rm.control, controller)
	if c := rm.parts[controller]; c != nil {
		c.sendJSON(map[string]interface{}{"type": "control", "granted": false, "sharer": g.Sharer, "sid": g.SID, "by": by})
	}
	if s := rm.parts[g.Sharer]; s != nil {
		s.sendJSON(map[string]interface{}{"type": "control-ended", "pid": controller, "sid": g.SID})
	}
	rm.presenceLocked()
}

// ─── MODERATION ──────────────────────────────────────

func (rm *room) moderateLocked(p *participant, m *inMsg, r *http.Request, errorTo func(string)) {
	target := rm.parts[m.PID]
	if target == nil {
		return
	}
	self := target == p
	if !self && (!roleCan(p.sa.Role, PermModerate) || roleRank[p.sa.Role] <= roleRank[target.sa.Role]) {
		errorTo("You cannot moderate " + target.name)
		return
	}
	actor := participantAuditName(p.sa)
	audit := func(action string, extra map[string]interface{}) {
		d := map[string]interface{}{"share_id": rm.shareID, "target": target.name}
		for k, v := range extra {
			d[k] = v
		}
		auditLogAs(r, p.sa.UserID, actor, "collab."+action, target.name, d)
	}
	switch m.Action {
	case "lower-hand":
		target.hand = false
		rm.presenceLocked()
	case "mute":
		if self {
			return
		}
		target.serverMuted = true
		target.sendJSON(map[string]interface{}{"type": "force-mute", "muted": true, "by": p.name})
		rm.presenceLocked()
		rm.systemLocked("🔇 "+p.name+" muted "+target.name, nil)
		audit("mute", nil)
	case "unmute":
		target.serverMuted = false
		target.sendJSON(map[string]interface{}{"type": "force-mute", "muted": false, "by": p.name})
		rm.presenceLocked()
		audit("unmute", nil)
	case "stop-share":
		if len(target.streams) > 0 {
			target.sendJSON(map[string]interface{}{"type": "force-stop-share", "by": p.name})
			rm.stopStreamsLocked(target, "")
			rm.presenceLocked()
			audit("stop_share", nil)
		}
	case "kick", "ban":
		if self {
			return
		}
		if m.Action == "ban" {
			setParticipantBanned(rm.shareID, target.sa.PKey, true)
			shareID, pkey := rm.shareID, target.sa.PKey
			go killTerminals(func(t *termSession) bool { return t.ShareID == shareID && t.PKey == pkey }, "You were removed from this share")
		}
		verb := map[string]string{"kick": "removed", "ban": "banned"}[m.Action]
		target.closeWith(map[string]interface{}{"type": "kicked", "by": p.name, "ban": m.Action == "ban",
			"message": fmt.Sprintf("%s %s you from the room", p.name, verb)}, 4010, verb)
		rm.leaveLocked(target)
		rm.systemLocked(target.name+" was "+verb+" by "+p.name, nil)
		audit(m.Action, nil)
	case "set-role":
		if self || !validAssignableRole(m.Role, roleRank[p.sa.Role]-1) {
			errorTo("You cannot give that role")
			return
		}
		setParticipantRole(rm.shareID, target.sa.PKey, m.Role)
		audit("set_role", map[string]interface{}{"role": m.Role, "previous": target.sa.Role})
		go revalidateShare(rm.shareID)
	}
}

// ─── REVOCATION & ADMIN HELPERS ──────────────────────

func revalidateRoom(shareID int, check func(accessCtx) (*ShareAccess, *shareDenied)) {
	rm := findRoom(shareID)
	if rm == nil {
		return
	}
	rm.mu.Lock()
	defer rm.mu.Unlock()
	changed := false
	for _, p := range rm.parts {
		sa, d := check(p.sa.Ctx)
		if d != nil {
			p.closeWith(map[string]interface{}{"type": "kicked", "message": d.Message, "reason": d.Reason}, 4011, d.Reason)
			rm.leaveLocked(p)
			changed = true
			continue
		}
		if sa.Role != p.sa.Role {
			p.sa.Role = sa.Role
			p.sendJSON(map[string]interface{}{"type": "role", "role": sa.Role, "perms": rolePerms(sa.Role)})
			if !roleCan(sa.Role, PermShareScreen) && len(p.streams) > 0 {
				p.sendJSON(map[string]interface{}{"type": "force-stop-share"})
				rm.stopStreamsLocked(p, "")
			}
			if _, ok := rm.control[p.pid]; ok && !roleCan(sa.Role, PermControl) {
				rm.revokeControlLocked(p.pid, "")
			}
			rm.systemLocked(p.name+" is now "+sa.Role, nil)
			changed = true
		}
	}
	if changed {
		rm.presenceLocked()
	}
}

func closeRoom(shareID int, reason string) {
	rm := findRoom(shareID)
	if rm == nil {
		return
	}
	rm.mu.Lock()
	for _, p := range rm.parts {
		p.closeWith(map[string]interface{}{"type": "closed", "message": reason}, 4012, "closed")
		delete(rm.parts, p.pid)
	}
	rm.mu.Unlock()
	dropRoomIfEmpty(rm)
}

func kickUserFromRooms(userID int, reason string) int {
	n := 0
	rooms.Lock()
	list := make([]*room, 0, len(rooms.m))
	for _, rm := range rooms.m {
		list = append(list, rm)
	}
	rooms.Unlock()
	for _, rm := range list {
		rm.mu.Lock()
		for _, p := range rm.parts {
			if p.sa.UserID == userID {
				p.closeWith(map[string]interface{}{"type": "kicked", "message": reason}, 4011, "revoked")
				rm.leaveLocked(p)
				n++
			}
		}
		rm.mu.Unlock()
	}
	return n
}

func roomOnlineCount(shareID int) int {
	rm := findRoom(shareID)
	if rm == nil {
		return 0
	}
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return len(rm.parts)
}

func roomOnlinePKeys(shareID int) map[string]bool {
	out := map[string]bool{}
	rm := findRoom(shareID)
	if rm == nil {
		return out
	}
	rm.mu.Lock()
	defer rm.mu.Unlock()
	for _, p := range rm.parts {
		out[p.sa.PKey] = true
	}
	return out
}

func roomsSummary() []map[string]interface{} {
	rooms.Lock()
	list := make([]*room, 0, len(rooms.m))
	for _, rm := range rooms.m {
		list = append(list, rm)
	}
	rooms.Unlock()
	out := []map[string]interface{}{}
	for _, rm := range list {
		var name, owner string
		db.QueryRow(`SELECT s.name, u.username FROM share_links s JOIN users u ON u.id=s.owner_id WHERE s.id=?`, rm.shareID).Scan(&name, &owner)
		rm.mu.Lock()
		people := []map[string]interface{}{}
		voice, streams := 0, 0
		for _, p := range rm.parts {
			if p.voice {
				voice++
			}
			streams += len(p.streams)
			people = append(people, map[string]interface{}{"name": p.name, "role": p.sa.Role, "guest": p.sa.Guest, "ip": p.ip, "voice": p.voice, "since": p.joined.UTC().Format(time.RFC3339)})
		}
		rm.mu.Unlock()
		out = append(out, map[string]interface{}{"share_id": rm.shareID, "name": name, "owner": owner, "participants": people, "voice": voice, "streams": streams})
	}
	return out
}

func userInRooms(userID int) bool {
	rooms.Lock()
	list := make([]*room, 0, len(rooms.m))
	for _, rm := range rooms.m {
		list = append(list, rm)
	}
	rooms.Unlock()
	for _, rm := range list {
		rm.mu.Lock()
		for _, p := range rm.parts {
			if p.sa.UserID == userID {
				rm.mu.Unlock()
				return true
			}
		}
		rm.mu.Unlock()
	}
	return false
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
