//go:build !android

// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pion/logging"
	"github.com/pion/turn/v4"
)

// ─── BUILT-IN TURN RELAY FOR VOICE CALLS ─────────────
//
// Voice calls are peer-to-peer WebRTC (low latency, end-to-end DTLS-SRTP encryption).
// When two browsers cannot reach each other directly (strict NAT, corporate firewall),
// audio is relayed through TURN. WRM contains a TURN server (UDP + TCP, port 3478 by
// default) so calls work without extra software. It only accepts short-lived credentials
// that WRM hands to participants of a room, and by default refuses to relay to private,
// loopback and link-local addresses (no access to internal networks through the relay).
//
// Admin → Voice: turn_enabled, turn_port, turn_public_ip (when WRM is behind NAT),
// turn_host (name browsers use to reach it), turn_relay_ports, turn_allow_private.
// External STUN/TURN servers (e.g. coturn on 443/TLS) can be added in ice_servers.

type turnState struct {
	mu        sync.Mutex
	server    *turn.Server
	port      int
	relayIP   net.IP
	err       string
	secret    string
	startedAt time.Time
}

var turnSrv = &turnState{}

func turnSecret() string {
	if turnSrv.secret == "" {
		turnSrv.secret = hex.EncodeToString([]byte(signValue("turn-secret")))[:48]
	}
	return turnSrv.secret
}

// outboundIP finds the address of the interface used for outgoing traffic.
func outboundIP() net.IP {
	c, err := net.Dial("udp4", "192.0.2.1:9") // TEST-NET address, no packet is sent
	if err != nil {
		return net.IPv4(127, 0, 0, 1)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP
}

func localIPs() map[string]bool {
	out := map[string]bool{}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				out[ipn.IP.String()] = true
			}
		}
	}
	return out
}

func startTURN() {
	turnSrv.mu.Lock()
	defer turnSrv.mu.Unlock()
	turnSrv.err = ""
	if !settingBool("turn_enabled") || !settingBool("voice_enabled") {
		return
	}
	port := settingInt("turn_port")
	relayIP := net.ParseIP(getSetting("turn_public_ip"))
	if relayIP == nil {
		relayIP = outboundIP()
	}
	relayIP = relayIP.To4()
	if relayIP == nil {
		turnSrv.err = "turn_public_ip must be an IPv4 address"
		return
	}
	minPort, maxPort, err := parsePortRange(getSetting("turn_relay_ports"))
	if err != nil {
		turnSrv.err = err.Error()
		return
	}
	udp, err := net.ListenPacket("udp4", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		turnSrv.err = fmt.Sprintf("cannot listen on UDP %d: %v", port, err)
		log.Printf("TURN: %s — voice calls will only work peer-to-peer", turnSrv.err)
		return
	}
	tcp, err := net.Listen("tcp4", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		udp.Close()
		turnSrv.err = fmt.Sprintf("cannot listen on TCP %d: %v", port, err)
		log.Printf("TURN: %s — voice calls will only work peer-to-peer", turnSrv.err)
		return
	}
	allowPrivate := settingBool("turn_allow_private")
	own := localIPs()
	own[relayIP.String()] = true
	permission := func(clientAddr net.Addr, peer net.IP) bool {
		if own[peer.String()] {
			return true // relay ↔ relay on this server
		}
		if peer.IsLoopback() || peer.IsUnspecified() || peer.IsMulticast() || peer.IsLinkLocalUnicast() || peer.IsLinkLocalMulticast() {
			return false
		}
		if !allowPrivate && (peer.IsPrivate() || isCGNAT(peer)) {
			return false
		}
		return true
	}
	gen := func() turn.RelayAddressGenerator {
		return &turn.RelayAddressGeneratorPortRange{RelayAddress: relayIP, Address: "0.0.0.0", MinPort: minPort, MaxPort: maxPort}
	}
	lf := logging.NewDefaultLoggerFactory()
	lf.DefaultLogLevel = logging.LogLevelError
	quota := map[string]int{}
	var quotaMu sync.Mutex
	srv, err := turn.NewServer(turn.ServerConfig{
		Realm:         "wrm",
		LoggerFactory: lf,
		AuthHandler:   turn.LongTermTURNRESTAuthHandler(turnSecret(), lf.NewLogger("turn")),
		QuotaHandler: func(username, realm string, srcAddr net.Addr) bool {
			quotaMu.Lock()
			defer quotaMu.Unlock()
			if len(quota) > 10000 {
				quota = map[string]int{}
			}
			quota[username]++
			return quota[username] <= 200
		},
		PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: udp, RelayAddressGenerator: gen(), PermissionHandler: permission}},
		ListenerConfigs:   []turn.ListenerConfig{{Listener: tcp, RelayAddressGenerator: gen(), PermissionHandler: permission}},
	})
	if err != nil {
		udp.Close()
		tcp.Close()
		turnSrv.err = err.Error()
		log.Printf("TURN: %v", err)
		return
	}
	turnSrv.server, turnSrv.port, turnSrv.relayIP, turnSrv.startedAt = srv, port, relayIP, time.Now()
	log.Printf("TURN relay for voice calls on UDP/TCP %d (relay address %s, ports %d-%d)", port, relayIP, minPort, maxPort)
}

func isCGNAT(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 100 && v4[1]&0xc0 == 64
}

func stopTURN() {
	turnSrv.mu.Lock()
	defer turnSrv.mu.Unlock()
	if turnSrv.server != nil {
		turnSrv.server.Close()
		turnSrv.server = nil
	}
}

func restartTURN() {
	stopTURN()
	startTURN()
}

func turnStatus() map[string]interface{} {
	turnSrv.mu.Lock()
	defer turnSrv.mu.Unlock()
	st := map[string]interface{}{"enabled": settingBool("turn_enabled"), "running": turnSrv.server != nil, "error": turnSrv.err}
	if turnSrv.server != nil {
		st["port"] = turnSrv.port
		st["relay_ip"] = turnSrv.relayIP.String()
		st["allocations"] = turnSrv.server.AllocationCount()
		st["since"] = turnSrv.startedAt.UTC().Format(time.RFC3339)
	}
	return st
}

// iceServersFor returns the STUN/TURN configuration for one participant: the configured
// servers plus the built-in relay with credentials valid for 12 hours.
func iceServersFor(r *http.Request, pid string) []map[string]interface{} {
	out := []map[string]interface{}{}
	var configured []map[string]interface{}
	if json.Unmarshal([]byte(getSetting("ice_servers")), &configured) == nil {
		out = append(out, configured...)
	}
	turnSrv.mu.Lock()
	running, port := turnSrv.server != nil, turnSrv.port
	turnSrv.mu.Unlock()
	if running {
		host := strings.TrimSpace(getSetting("turn_host"))
		if host == "" {
			host = hostOnly(r.Host)
			if trustProxy && r.Header.Get("X-Forwarded-Host") != "" {
				host = hostOnly(strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Host"), ",")[0]))
			}
		}
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		user, pass, err := turn.GenerateLongTermTURNRESTCredentials(turnSecret(), pid, 12*time.Hour)
		if err == nil {
			p := strconv.Itoa(port)
			out = append(out, map[string]interface{}{
				"urls":       []string{"stun:" + host + ":" + p, "turn:" + host + ":" + p + "?transport=udp", "turn:" + host + ":" + p + "?transport=tcp"},
				"username":   user,
				"credential": pass,
			})
		}
	}
	return out
}
