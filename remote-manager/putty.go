// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

// ─── IMPORT FROM PuTTY ───────────────────────────────
//
// A Windows registry export (.reg, UTF-16 LE with a BOM or UTF-8) of
// HKEY_CURRENT_USER\Software\SimonTatham\PuTTY\Sessions. Session names are URL-encoded
// (My%20Server). "Default Settings" is not imported; its values are the defaults of the
// other sessions. SSH and Telnet sessions become connections; raw, rlogin and serial
// (a COM port of the Windows PC) are listed as skipped.
//
// Proxy settings (SOCKS4 / SOCKS5 / HTTP) become saved proxies, one per distinct proxy and
// reused when the user already has the same one, and are linked to the connections. A
// "SSH proxy" (PuTTY 0.77+) becomes the jump host. Every imported session is remembered
// with its proxy (putty_sessions), so mRemoteNG connections that name it (PuttySession)
// get the same proxy, whichever of the two files is imported first.

const puttySessionsKey = `\software\simontatham\putty\sessions\`

type puttySession struct {
	name string
	vals map[string]string // lower-case value name → string, or the number of a dword
}

// decodeRegFile returns the text of a .reg file: UTF-16 LE (with or without a BOM) or UTF-8.
func decodeRegFile(data []byte) string {
	if bytes.HasPrefix(data, []byte{0xFF, 0xFE}) || len(data) >= 2 && data[0] != 0 && data[1] == 0 {
		if bytes.HasPrefix(data, []byte{0xFF, 0xFE}) {
			data = data[2:]
		}
		u := make([]uint16, len(data)/2)
		for i := range u {
			u[i] = uint16(data[2*i]) | uint16(data[2*i+1])<<8
		}
		return string(utf16.Decode(u))
	}
	return string(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))
}

// regString reads a quoted .reg string value ("a\\b \"c\"").
func regString(v string) (string, bool) {
	if len(v) < 2 || v[0] != '"' || v[len(v)-1] != '"' {
		return "", false
	}
	var b strings.Builder
	for i := 1; i < len(v)-1; i++ {
		if v[i] == '\\' && i+1 < len(v)-1 {
			i++
		}
		b.WriteByte(v[i])
	}
	return b.String(), true
}

// parsePuttyReg returns the sessions of a .reg export in file order.
func parsePuttyReg(text string) ([]*puttySession, error) {
	var out []*puttySession
	var cur *puttySession
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	header := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "Windows Registry Editor") || line == "REGEDIT4" {
			header = true
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			cur = nil
			key := line[1 : len(line)-1]
			// ASCII-only lowering keeps the byte offsets of key (strings.ToLower replaces invalid
			// UTF-8 with a longer rune, and the index would then not fit key)
			i := strings.Index(asciiLower(key), puttySessionsKey)
			if strings.HasPrefix(key, "-") || i < 0 {
				continue
			}
			raw := key[i+len(puttySessionsKey):]
			if raw == "" || strings.Contains(raw, `\`) {
				continue
			}
			name, err := url.PathUnescape(raw)
			if err != nil {
				name = raw
			}
			cur = &puttySession{name: strings.TrimSpace(name), vals: map[string]string{}}
			out = append(out, cur)
			continue
		}
		if cur == nil || !strings.HasPrefix(line, `"`) {
			continue // values of other keys, binary continuation lines
		}
		eq := strings.Index(line, `"=`)
		if eq < 1 {
			continue
		}
		name, ok := regString(line[:eq+1])
		if !ok {
			continue
		}
		v := line[eq+2:]
		if s, ok := regString(v); ok {
			cur.vals[strings.ToLower(name)] = s
		} else if strings.HasPrefix(v, "dword:") {
			if n, err := strconv.ParseUint(strings.TrimPrefix(v, "dword:"), 16, 32); err == nil {
				cur.vals[strings.ToLower(name)] = strconv.FormatUint(n, 10)
			}
		}
	}
	if !header && len(out) == 0 {
		return nil, fmt.Errorf("not a registry export (.reg)")
	}
	if len(out) == 0 {
		return nil, fmt.Errorf(`no PuTTY sessions found (export HKEY_CURRENT_USER\Software\SimonTatham\PuTTY\Sessions)`)
	}
	return out, nil
}

func importPuTTY(userID int, data []byte, folder string) (*importResult, error) {
	sessions, err := parsePuttyReg(decodeRegFile(data))
	if err != nil {
		return nil, err
	}
	folder = strings.TrimSpace(folder)
	if folder == "" {
		folder = "PuTTY"
	}
	defaults := map[string]string{}
	for _, s := range sessions {
		if strings.EqualFold(s.name, "Default Settings") {
			defaults = s.vals
		}
	}
	im := newImporter(userID)
	for _, s := range sessions {
		if strings.EqualFold(s.name, "Default Settings") {
			continue
		}
		get := func(k string) string {
			if v, ok := s.vals[strings.ToLower(k)]; ok {
				return v
			}
			return defaults[strings.ToLower(k)]
		}
		im.puttySession(s.name, get, folder)
	}
	if len(im.items) == 0 && len(im.res.Skipped) == 0 {
		return nil, fmt.Errorf("no PuTTY sessions found (only Default Settings)")
	}
	if err := im.commit(); err != nil {
		return nil, err
	}
	// Remember each imported session's proxy and give it to connections that name the
	// session (mRemoteNG PuttySession) and have no proxy of their own.
	for _, it := range im.items {
		if it.puttyOwn == "" {
			continue
		}
		var pid *int
		if it.proxy != nil {
			if id := im.itemProxy(it); id > 0 { // also for a session skipped as already imported
				pid = &id
			}
		}
		db.Exec(`INSERT INTO putty_sessions (user_id, name, proxy_id) VALUES (?,?,?) ON CONFLICT(user_id, name) DO UPDATE SET proxy_id=excluded.proxy_id`,
			userID, it.puttyOwn, pid)
		if pid == nil {
			continue
		}
		res, err := db.Exec(`UPDATE connections SET proxy_id=? WHERE user_id=? AND putty_session=? COLLATE NOCASE AND proxy_id IS NULL AND protocol IN ('SSH','TELNET')`,
			*pid, userID, it.puttyOwn)
		if err == nil {
			n, _ := res.RowsAffected()
			im.res.Linked += int(n)
		}
	}
	if im.res.Proxies > 0 {
		hub.sendTo(userID, []byte(`{"type":"proxies_changed"}`))
	}
	if im.res.Linked > 0 {
		statusMon.poke()
	}
	return &im.res, nil
}

func (im *importer) puttySession(name string, get func(string) string, folder string) {
	name = truncateStr(name, 120)
	c := Connection{Name: name, AuthMethod: "PASSWORD", UserID: im.userID}
	proto := strings.ToLower(get("Protocol"))
	switch proto {
	case "", "ssh":
		c.Protocol = "SSH"
	case "telnet":
		c.Protocol = "TELNET"
	case "serial":
		im.skip(name, fmt.Sprintf("serial line %s is a port of the Windows PC — create a serial connection for a port of the WRM server", get("SerialLine")))
		return
	default:
		im.skip(name, fmt.Sprintf("PuTTY protocol %s is not supported by WRM (SSH, Telnet)", proto))
		return
	}
	host := strings.TrimSpace(get("HostName"))
	c.Username = strings.TrimSpace(get("UserName"))
	if i := strings.LastIndex(host, "@"); i >= 0 { // user@host wins over UserName, as in PuTTY
		c.Username, host = host[:i], host[i+1:]
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		im.skip(name, "no host name")
		return
	}
	def := map[string]int{"SSH": 22, "TELNET": 23}[c.Protocol]
	if port, _ := strconv.Atoi(get("PortNumber")); port > 0 && port != def {
		host = net.JoinHostPort(host, strconv.Itoa(port))
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	c.Host = host
	if kf := strings.TrimSpace(get("PublicKeyFile")); kf != "" && c.Protocol == "SSH" {
		im.note(name, "PuTTY key file "+kf+": convert it with PuTTYgen (Conversions → Export OpenSSH key), add it under Keys and choose it")
	}
	if c.Protocol == "SSH" && c.Username == "" {
		im.note(name, "no user name — set it before connecting")
	}
	item := &importItem{c: c, folder: folder, puttyOwn: truncateStr(name, 200)}
	ph := strings.TrimSpace(get("ProxyHost"))
	pport, _ := strconv.Atoi(get("ProxyPort"))
	switch get("ProxyMethod") {
	case "", "0":
	case "1", "2", "3":
		kind := map[string]string{"1": "socks4", "2": "socks5", "3": "http"}[get("ProxyMethod")]
		dns := get("ProxyDNS") // 0 no, 1 auto (remote for SOCKS5 and HTTP), 2 yes
		item.proxy = &proxyInput{Name: "PuTTY", Kind: kind, Host: ph, Port: pport, Username: get("ProxyUsername"), Password: get("ProxyPassword"),
			RemoteDNS: dns == "2" || dns != "0" && kind != "socks4"}
	case "6":
		item.jumpName = ph // PuTTY's "SSH proxy": a host or a saved session, linked by name
	default:
		im.note(name, "PuTTY proxy type (Telnet or local command) not imported — set a proxy or jump host in the connection")
	}
	im.items = append(im.items, item)
}

// itemProxy resolves the proxy of an imported session once.
func (im *importer) itemProxy(it *importItem) int {
	if it.proxyOut == nil {
		id := im.proxyID(it.c.Name, it.proxy)
		it.proxyOut = &id
	}
	return *it.proxyOut
}

func proxyKey(in *proxyInput) string {
	return strings.ToLower(fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%t", in.Kind, in.Host, in.Port, in.Username, in.RemoteDNS)) + "\x00" + in.Password
}

// proxyID returns the saved proxy for a PuTTY proxy setting: one created by this import,
// an identical proxy the user already owns, or a new one. 0 when it cannot be saved.
func (im *importer) proxyID(conn string, in *proxyInput) int {
	if !proxiesAllowed(im.userID) {
		im.note(conn, "proxy "+in.Host+" not imported (defining proxies is not allowed for this account)")
		return 0
	}
	if err := normalizeProxyInput(in, im.userID); err != nil {
		im.note(conn, "proxy not imported: "+err.Error())
		return 0
	}
	key := proxyKey(in)
	if id, ok := im.proxies[key]; ok {
		return id
	}
	if im.proxies == nil {
		im.proxies = map[string]int{}
	}
	if rows, err := db.Query(`SELECT `+proxyCols+` FROM proxies WHERE owner_id=? AND kind=? AND host=? COLLATE NOCASE AND port=? AND username=? AND credential_id IS NULL`,
		im.userID, in.Kind, in.Host, in.Port, in.Username); err == nil {
		for rows.Next() {
			if p, err := scanProxy(rows); err == nil && p.RemoteDNS == in.RemoteDNS && decryptValue(p.password) == in.Password {
				im.proxies[key] = p.ID
				break
			}
		}
		rows.Close()
		if id := im.proxies[key]; id > 0 {
			return id
		}
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM proxies WHERE owner_id=?`, im.userID).Scan(&n)
	if n >= maxProxiesPerUser {
		im.note(conn, fmt.Sprintf("proxy not imported: at most %d proxies per user", maxProxiesPerUser))
		return 0
	}
	base := "PuTTY " + in.Kind + " " + net.JoinHostPort(in.Host, strconv.Itoa(in.Port))
	if in.Username != "" {
		base += " (" + in.Username + ")"
	}
	in.Name = truncateStr(base, 110)
	for i := 2; ; i++ {
		var taken int
		db.QueryRow(`SELECT COUNT(*) FROM proxies WHERE owner_id=? AND name=?`, im.userID, in.Name).Scan(&taken)
		if taken == 0 {
			break
		}
		in.Name = truncateStr(base, 110) + " " + strconv.Itoa(i)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO proxies (owner_id,name,kind,host,port,username,password,remote_dns,description,shared_all,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,0,?,?)`, im.userID, in.Name, in.Kind, in.Host, in.Port, in.Username, encryptValue(in.Password),
		boolInt(in.RemoteDNS), "Imported from PuTTY", now, now)
	if err != nil {
		im.note(conn, "proxy not imported: "+err.Error())
		return 0
	}
	id64, _ := res.LastInsertId()
	im.proxies[key] = int(id64)
	im.res.Proxies++
	return int(id64)
}

// puttyProxy is the proxy of an imported PuTTY session, if the user may still use it.
func puttyProxy(userID int, session string) int {
	var pid *int
	if db.QueryRow(`SELECT proxy_id FROM putty_sessions WHERE user_id=? AND name=?`, userID, session).Scan(&pid) != nil || pid == nil {
		return 0
	}
	if p, err := loadProxy(*pid); err != nil || !proxyAccessible(p, userID) {
		return 0
	}
	return *pid
}

// asciiLower lowers A-Z only, so the result has the same length as s.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
