// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

// ─── IMPORT FROM mRemoteNG AND OpenSSH ───────────────
//
// mRemoteNG (confCons.xml): containers become nested folders ("Parent / Child"), SSH and HTTP(S)
// connections are imported with their passwords, the "SSH tunnel" setting becomes the
// jump host. Other protocols (RDP, VNC, Telnet…) are listed as skipped. Passwords are
// decrypted with the mRemoteNG master password (default "mR3m"): AES-256-GCM with a
// PBKDF2-HMAC-SHA1 key (current versions) or AES-CBC (old versions).
//
// OpenSSH (~/.ssh/config): Host blocks become connections; HostName, User, Port,
// ProxyJump / ProxyCommand "ssh -W", LocalForward, RemoteForward, DynamicForward and
// IdentityFile are understood.
//
// PuTTY (.reg export of the saved sessions): see putty.go.

type importResult struct {
	Imported int            `json:"imported"`
	Updated  int            `json:"updated"`
	Folders  int            `json:"folders"`
	Tunnels  int            `json:"tunnels"`
	Jumps    int            `json:"jump_hosts"`
	Proxies  int            `json:"proxies"`     // saved proxies created (PuTTY)
	Linked   int            `json:"proxy_links"` // connections that got the proxy of their PuTTY session
	Skipped  []importIssue  `json:"skipped"`
	Notes    []importIssue  `json:"notes"`
	ByProto  map[string]int `json:"by_protocol"`
}

type importIssue struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// importItem is one connection to create.
type importItem struct {
	c        Connection
	folder   string
	jumpName string // resolved after all items exist
	tunnels  []tunnelDef
	extID    string      // id in the source inventory (NetBox): updated instead of duplicated
	extTags  []string    // tags that come from the source (replaced on update)
	proxy    *proxyInput // PuTTY: a proxy to create (or reuse) and link
	proxyOut *int        // PuTTY: the saved proxy, once resolved (0: could not be saved)
	puttyRef string      // mRemoteNG: the PuTTY session it names (PuttySession)
	puttyOwn string      // PuTTY: the name of the imported session
}

// importer collects folders and connections and writes them in one transaction.
type importer struct {
	userID  int
	res     importResult
	items   []*importItem
	folders map[string]int
	update  bool // update connections with the same ext_id
	proxies map[string]int
}

func newImporter(userID int) *importer {
	return &importer{userID: userID, folders: map[string]int{}, res: importResult{Skipped: []importIssue{}, Notes: []importIssue{}, ByProto: map[string]int{}}}
}

func (im *importer) skip(name, reason string) {
	if len(im.res.Skipped) < 500 {
		im.res.Skipped = append(im.res.Skipped, importIssue{name, reason})
	}
}

func (im *importer) note(name, reason string) {
	if len(im.res.Notes) < 500 {
		im.res.Notes = append(im.res.Notes, importIssue{name, reason})
	}
}

func (im *importer) folderID(name string) (*int, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	if id, ok := im.folders[name]; ok {
		return &id, nil
	}
	parts := strings.Split(name, " / ")
	if len(parts) > folderMaxDepth {
		return nil, fmt.Errorf("at most %d nested folders", folderMaxDepth)
	}
	folderHierarchyMu.Lock()
	defer folderHierarchyMu.Unlock()
	var parent *int
	for _, part := range parts {
		part = truncateStr(strings.TrimSpace(part), 120)
		if part == "" {
			continue
		}
		var id int
		if err := db.QueryRow(`SELECT id FROM folders WHERE name=? AND user_id=? AND parent_id IS ? ORDER BY id LIMIT 1`, part, im.userID, parent).Scan(&id); err != nil {
			res, err := db.Exec(`INSERT INTO folders(name,user_id,parent_id) VALUES(?,?,?)`, part, im.userID, parent)
			if err != nil {
				return nil, err
			}
			n, _ := res.LastInsertId()
			id = int(n)
			im.res.Folders++
		}
		parent = &id
	}
	if parent != nil {
		im.folders[name] = *parent
	}
	return parent, nil
}

// commit creates the connections, links jump hosts by name and adds the tunnels.
func (im *importer) commit() error {
	existing := map[string]int{} // name → id of the user's connections
	for _, c := range loadUserConnections(im.userID) {
		if _, ok := existing[strings.ToLower(c.Name)]; !ok {
			existing[strings.ToLower(c.Name)] = c.ID
		}
	}
	dup := map[string]bool{}
	for _, c := range loadUserConnections(im.userID) {
		dup[strings.ToLower(c.Name+"\x00"+c.Host+"\x00"+c.Protocol)] = true
	}
	type extConn struct {
		id            int
		tags, extTags string
	}
	byExt := map[string]extConn{}
	if rows, err := db.Query(`SELECT id, COALESCE(tags,''), COALESCE(ext_tags,''), ext_id FROM connections WHERE user_id=? AND ext_id<>''`, im.userID); err == nil {
		for rows.Next() {
			var e extConn
			var ext string
			rows.Scan(&e.id, &e.tags, &e.extTags, &ext)
			byExt[ext] = e
		}
		rows.Close()
	}
	created := map[string]int{}
	ids := make([]int, len(im.items))
	for i, it := range im.items {
		c := it.c
		if err := normalizeConnection(&c); err != nil {
			im.skip(c.Name, err.Error())
			continue
		}
		if e, ok := byExt[it.extID]; ok && it.extID != "" {
			if !im.update {
				im.skip(c.Name, "imported before (turn on \"update\" to refresh it)")
				continue
			}
			// Address, tags and folder follow the source; name and login stay as the user set them.
			tags := tagsString(mergeTags(parseTags(e.tags), parseTags(e.extTags), c.Tags))
			fid, err := im.folderID(it.folder)
			if err != nil {
				return err
			}
			db.Exec(`UPDATE connections SET host=?, tags=?, ext_tags=?, folder_id=COALESCE(?, folder_id) WHERE id=? AND user_id=?`,
				c.Host, tags, tagsString(it.extTags), fid, e.id, im.userID)
			im.res.Updated++
			statusMon.poke()
			continue
		}
		if err := checkAuthRefs(&c, im.userID); err != nil {
			im.note(c.Name, err.Error()+" — imported without a login")
			c.AuthMethod, c.KeyID, c.CredentialID = "PASSWORD", nil, nil
		}
		key := strings.ToLower(c.Name + "\x00" + c.Host + "\x00" + c.Protocol)
		if dup[key] {
			im.skip(c.Name, "already exists (same name, host and protocol)")
			continue
		}
		dup[key] = true
		if it.proxy != nil {
			if pid := im.itemProxy(it); pid > 0 {
				c.ProxyID = &pid
			}
		} else if it.puttyRef != "" {
			if pid := puttyProxy(im.userID, it.puttyRef); pid > 0 {
				c.ProxyID = &pid
				im.res.Linked++
			}
		}
		fid, err := im.folderID(it.folder)
		if err != nil {
			return err
		}
		c.FolderID = fid
		encryptConnectionSecrets(&c)
		opts := ""
		if len(c.Options) > 0 {
			opts = string(jsonMarshal(c.Options))
		}
		res, err := db.Exec(`INSERT INTO connections (name,protocol,host,username,auth_method,password,private_key,key_path,folder_id,user_id,web_path,options,key_id,credential_id,tags,ext_id,ext_tags,proxy_id,putty_session)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			c.Name, c.Protocol, c.Host, c.Username, c.AuthMethod, c.Password, c.PrivateKey, c.KeyPath, c.FolderID, im.userID, c.WebPath, opts,
			c.KeyID, c.CredentialID, tagsString(c.Tags), it.extID, tagsString(it.extTags), c.ProxyID, it.puttyRef)
		if err != nil {
			im.skip(c.Name, err.Error())
			continue
		}
		id64, _ := res.LastInsertId()
		ids[i] = int(id64)
		if _, ok := created[strings.ToLower(c.Name)]; !ok {
			created[strings.ToLower(c.Name)] = ids[i]
		}
		im.res.Imported++
		im.res.ByProto[c.Protocol]++
	}
	// jump hosts by name: imported connections first, then existing ones
	for i, it := range im.items {
		if ids[i] == 0 || it.jumpName == "" {
			continue
		}
		jid, ok := created[strings.ToLower(it.jumpName)]
		if !ok {
			jid, ok = existing[strings.ToLower(it.jumpName)]
		}
		if !ok {
			im.note(it.c.Name, fmt.Sprintf("jump host %q not found — set it in the connection", it.jumpName))
			continue
		}
		j := jid
		if err := validateJump(im.userID, ids[i], &j); err != nil {
			im.note(it.c.Name, fmt.Sprintf("jump host %q: %v", it.jumpName, err))
			continue
		}
		db.Exec(`UPDATE connections SET jump_conn_id=? WHERE id=?`, jid, ids[i])
		im.res.Jumps++
	}
	// tunnels
	now := time.Now().UTC().Format(time.RFC3339)
	for i, it := range im.items {
		if ids[i] == 0 {
			continue
		}
		for n, d := range it.tunnels {
			if err := validateTunnelDef(&d, im.userID); err != nil {
				im.note(it.c.Name, fmt.Sprintf("tunnel %s: %v", d.describe(), err))
				continue
			}
			db.Exec(`INSERT INTO connection_tunnels (conn_id, user_id, name, kind, bind_host, bind_port, target_host, target_port, open_scheme, open_path, start_mode, sort, created_at)
				VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, ids[i], im.userID, d.Name, d.Kind, d.BindHost, d.BindPort, d.TargetHost, d.TargetPort, d.OpenScheme, d.OpenPath, d.StartMode, n, now)
			im.res.Tunnels++
		}
	}
	return nil
}

// ── mRemoteNG ──

type mrNode struct {
	Attrs []xml.Attr `xml:",any,attr"`
	Nodes []mrNode   `xml:"Node"`
}

type mrRoot struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Nodes   []mrNode   `xml:"Node"`
	Text    string     `xml:",chardata"`
}

// xmlAttrs maps attribute names to values; names are matched without regard to case
// (mRemoteNG versions differ, e.g. SSHTunnelConnectionName / SshTunnelConnectionName).
type attrMap map[string]string

func xmlAttrs(list []xml.Attr) attrMap {
	m := attrMap{}
	for _, a := range list {
		m[strings.ToLower(a.Name.Local)] = a.Value
	}
	return m
}

func (m attrMap) get(name string) string { return m[strings.ToLower(name)] }

// mrCrypto decrypts mRemoteNG secrets.
type mrCrypto struct {
	password   string
	iterations int
	gcm        bool
}

func (mc mrCrypto) decrypt(b64 string) (string, error) {
	b64 = strings.TrimSpace(b64)
	if b64 == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", fmt.Errorf("not base64")
	}
	if mc.gcm {
		if len(data) < 48 {
			return "", fmt.Errorf("too short")
		}
		salt, nonce, ct := data[:16], data[16:32], data[32:]
		key := pbkdf2.Key([]byte(mc.password), salt, mc.iterations, 32, sha1.New)
		block, err := aes.NewCipher(key)
		if err != nil {
			return "", err
		}
		aead, err := cipher.NewGCMWithNonceSize(block, 16)
		if err != nil {
			return "", err
		}
		pt, err := aead.Open(nil, nonce, ct, salt)
		if err != nil {
			return "", fmt.Errorf("wrong master password or damaged value")
		}
		return string(pt), nil
	}
	// Old mRemote/mRemoteNG: AES-CBC, key = MD5(password), IV = first 16 bytes.
	if len(data) < 32 || len(data)%16 != 0 {
		return "", fmt.Errorf("damaged value")
	}
	key := md5.Sum([]byte(mc.password))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	iv, ct := data[:16], data[16:]
	pt := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(pt, ct)
	pad := int(pt[len(pt)-1])
	if pad < 1 || pad > 16 || pad > len(pt) {
		return "", fmt.Errorf("wrong master password or damaged value")
	}
	for _, b := range pt[len(pt)-pad:] {
		if int(b) != pad {
			return "", fmt.Errorf("wrong master password or damaged value")
		}
	}
	return string(pt[:len(pt)-pad]), nil
}

var errNeedPassword = fmt.Errorf("this file is protected with a master password")

func importMRemoteNG(userID int, data []byte, password string) (*importResult, error) {
	var root mrRoot
	if err := xml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("not a mRemoteNG connection file (confCons.xml): %v", err)
	}
	if root.XMLName.Local != "Connections" {
		return nil, fmt.Errorf("not a mRemoteNG connection file (root element %q)", root.XMLName.Local)
	}
	ra := xmlAttrs(root.Attrs)
	mc := mrCrypto{password: "mR3m", iterations: 1000, gcm: true}
	if n, err := strconv.Atoi(ra.get("KdfIterations")); err == nil && n > 0 && n <= 10000000 {
		mc.iterations = n
	}
	if ra.get("EncryptionEngine") == "" && ra.get("BlockCipherMode") == "" {
		mc.gcm = false
	}
	if mode := strings.ToUpper(ra.get("BlockCipherMode")); mode != "" && mode != "GCM" {
		return nil, fmt.Errorf("unsupported mRemoteNG encryption %s/%s — export without a custom encryption setting", ra.get("EncryptionEngine"), ra.get("BlockCipherMode"))
	}
	if password != "" {
		mc.password = password
	}
	// The "Protected" attribute tells whether the master password is right.
	if p := ra.get("Protected"); p != "" {
		if _, err := mc.decrypt(p); err != nil {
			if password == "" {
				return nil, errNeedPassword
			}
			return nil, fmt.Errorf("wrong master password")
		}
	}
	nodes := root.Nodes
	if strings.EqualFold(ra.get("FullFileEncryption"), "true") {
		inner, err := mc.decrypt(root.Text)
		if err != nil {
			return nil, fmt.Errorf("cannot decrypt the file: %v", err)
		}
		var wrap struct {
			Nodes []mrNode `xml:"Node"`
		}
		if err := xml.Unmarshal([]byte("<x>"+inner+"</x>"), &wrap); err != nil {
			return nil, fmt.Errorf("cannot read the decrypted file: %v", err)
		}
		nodes = wrap.Nodes
	}
	im := newImporter(userID)
	var walk func(list []mrNode, path []string, inherited map[string]string)
	walk = func(list []mrNode, path []string, inherited map[string]string) {
		for _, n := range list {
			a := xmlAttrs(n.Attrs)
			name := strings.TrimSpace(a.get("Name"))
			// effective values with mRemoteNG inheritance from the parent container
			eff := map[string]string{}
			for _, k := range []string{"Username", "Password", "Port", "SSHTunnelConnectionName", "Domain", "PuttySession"} {
				eff[k] = a.get(k)
				if strings.EqualFold(a.get("Inherit"+k), "true") {
					eff[k] = inherited[k]
				}
			}
			if strings.EqualFold(a.get("Type"), "Container") {
				walk(n.Nodes, append(append([]string{}, path...), name), eff)
				continue
			}
			im.mrConnection(mc, name, a, eff, strings.Join(path, " / "))
		}
	}
	walk(nodes, nil, map[string]string{})
	if err := im.commit(); err != nil {
		return nil, err
	}
	return &im.res, nil
}

func (im *importer) mrConnection(mc mrCrypto, name string, a attrMap, eff map[string]string, folder string) {
	proto := strings.ToUpper(strings.TrimSpace(a.get("Protocol")))
	host := strings.TrimSpace(a.get("Hostname"))
	if name == "" {
		name = host
	}
	c := Connection{Name: name, Username: eff["Username"], AuthMethod: "PASSWORD", UserID: im.userID}
	port, _ := strconv.Atoi(eff["Port"])
	switch proto {
	case "SSH2", "SSH1", "SSH":
		c.Protocol = "SSH"
	case "HTTP", "HTTPS":
		c.Protocol = proto
		if strings.Contains(host, "://") {
			if u, err := url.Parse(host); err == nil && u.Host != "" {
				c.Protocol = strings.ToUpper(u.Scheme)
				host = u.Host
				c.WebPath = u.EscapedPath()
				if u.RawQuery != "" {
					c.WebPath += "?" + u.RawQuery
				}
			}
		}
	case "RDP":
		c.Protocol = "RDP"
		c.Options = map[string]string{"ignore_cert": "true"}
		if d := strings.TrimSpace(eff["Domain"]); d != "" {
			c.Options["domain"] = d
		}
		if strings.EqualFold(a.get("UseConsoleSession"), "true") {
			c.Options["console"] = "true"
		}
		if cd := map[string]string{"Colors256": "8", "Colors15Bit": "16", "Colors16Bit": "16", "Colors24Bit": "24", "Colors32Bit": "32"}[a.get("Colors")]; cd != "" {
			c.Options["color_depth"] = cd
		}
		if gw := strings.TrimSpace(a.get("RDGatewayHostname")); gw != "" && !strings.EqualFold(a.get("RDGatewayUsageMethod"), "Never") {
			c.Options["gateway_host"] = gw
			if u := strings.TrimSpace(a.get("RDGatewayUsername")); u != "" {
				c.Options["gateway_user"] = u
			}
			if d := strings.TrimSpace(a.get("RDGatewayDomain")); d != "" {
				c.Options["gateway_domain"] = d
			}
		}
		if strings.EqualFold(a.get("RedirectSound"), "DoNotPlay") {
			c.Options["audio"] = "false"
		}
	case "VNC":
		c.Protocol = "VNC"
		c.Options = map[string]string{}
		if strings.EqualFold(a.get("VNCViewOnly"), "true") {
			c.Options["read_only"] = "true"
		}
	case "TELNET":
		c.Protocol = "TELNET"
	default:
		im.skip(name, fmt.Sprintf("protocol %s is not supported by WRM (SSH, SFTP, RDP, VNC, Telnet, HTTP/HTTPS)", a.get("Protocol")))
		return
	}
	if host == "" {
		im.skip(name, "no host name")
		return
	}
	if port > 0 && !strings.Contains(strings.Trim(host, "[]"), ":") {
		def := map[string]int{"SSH": 22, "HTTP": 80, "HTTPS": 443, "RDP": 3389, "VNC": 5900, "TELNET": 23}[c.Protocol]
		if port != def {
			host = net.JoinHostPort(host, strconv.Itoa(port))
		}
	}
	c.Host = host
	if pw := eff["Password"]; pw != "" {
		plain, err := mc.decrypt(pw)
		if err != nil {
			im.note(name, "password not imported: "+err.Error())
		} else {
			c.Password = plain
		}
	}
	if c.Protocol == "SSH" && c.Username == "" {
		im.note(name, "no user name — set it before connecting")
	}
	item := &importItem{c: c, folder: folder, jumpName: strings.TrimSpace(eff["SSHTunnelConnectionName"])}
	if ps := strings.TrimSpace(eff["PuttySession"]); (c.Protocol == "SSH" || c.Protocol == "TELNET") && ps != "" && !strings.EqualFold(ps, "Default Settings") {
		item.puttyRef = truncateStr(ps, 200)
	}
	im.items = append(im.items, item)
}

// ── OpenSSH config ──

type sshHostBlock struct {
	patterns []string
	opts     [][2]string
}

var proxyCmdJump = regexp.MustCompile(`^ssh\s+(?:.*\s)?-W\s+\S+\s+(\S+)\s*$|^ssh\s+(\S+)\s+(?:.*\s)?-W\s+\S+\s*$`)

func importSSHConfig(userID int, text, folder string) (*importResult, error) {
	var blocks []*sshHostBlock
	var cur *sshHostBlock
	inMatch := false
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val := splitSSHOption(line)
		if key == "" {
			continue
		}
		switch strings.ToLower(key) {
		case "host":
			cur = &sshHostBlock{patterns: strings.Fields(val)}
			blocks = append(blocks, cur)
			inMatch = false
			continue
		case "match":
			inMatch = true
			cur = nil
			continue
		}
		if inMatch {
			continue
		}
		if cur == nil { // options before the first Host apply to every host
			cur = &sshHostBlock{patterns: []string{"*"}}
			blocks = append(blocks, cur)
		}
		cur.opts = append(cur.opts, [2]string{strings.ToLower(key), val})
	}
	isPattern := func(p string) bool { return strings.ContainsAny(p, "*?!") }
	// first value wins (OpenSSH semantics); wildcard blocks "*" provide defaults
	lookup := func(alias string, b *sshHostBlock, key string) []string {
		var vals []string
		for i, bb := range append([]*sshHostBlock{b}, blocks...) {
			if i > 0 && bb == b {
				continue // own block was read first
			}
			if bb != b {
				match := false
				for _, p := range bb.patterns {
					if ok, _ := filepath.Match(p, alias); ok && isPattern(p) {
						match = true
					}
				}
				if !match {
					continue
				}
			}
			for _, o := range bb.opts {
				if o[0] == key {
					vals = append(vals, o[1])
				}
			}
			if len(vals) > 0 && key != "localforward" && key != "remoteforward" && key != "dynamicforward" {
				return vals[:1]
			}
		}
		return vals
	}
	if folder = strings.TrimSpace(folder); folder == "" {
		folder = "SSH config"
	}
	im := newImporter(userID)
	known := map[string]bool{}
	for _, b := range blocks {
		for _, p := range b.patterns {
			if !isPattern(p) {
				known[strings.ToLower(p)] = true
			}
		}
	}
	implicit := map[string]bool{}
	addImplicit := func(spec string) string { // "user@host:port" used in ProxyJump without a Host block
		if known[strings.ToLower(spec)] || implicit[strings.ToLower(spec)] {
			return spec
		}
		implicit[strings.ToLower(spec)] = true
		user, hostport := "", spec
		if i := strings.LastIndex(spec, "@"); i >= 0 {
			user, hostport = spec[:i], spec[i+1:]
		}
		c := Connection{Name: spec, Protocol: "SSH", Host: hostport, Username: user, AuthMethod: "PASSWORD", UserID: userID}
		im.items = append(im.items, &importItem{c: c, folder: folder})
		im.note(spec, "created for a ProxyJump — set its password or key")
		return spec
	}
	allowKeys := serverKeysAllowed(userID)
	for _, b := range blocks {
		alias := ""
		for _, p := range b.patterns {
			if !isPattern(p) {
				alias = p
				break
			}
		}
		if alias == "" {
			continue // only wildcard patterns: defaults, not a connection
		}
		get := func(k string) string {
			if v := lookup(alias, b, k); len(v) > 0 {
				return strings.Trim(v[0], `"`)
			}
			return ""
		}
		host := get("hostname")
		if host == "" {
			host = alias
		}
		host = strings.ReplaceAll(host, "%h", alias)
		if port := get("port"); port != "" && port != "22" {
			host = net.JoinHostPort(strings.Trim(host, "[]"), port)
		}
		c := Connection{Name: alias, Protocol: "SSH", Host: host, Username: get("user"), AuthMethod: "PASSWORD", UserID: userID}
		if id := get("identityfile"); id != "" && !strings.EqualFold(id, "none") {
			if allowKeys {
				c.AuthMethod, c.KeyPath = "KEY_FILE", expandHome(id)
			} else {
				im.note(alias, "IdentityFile "+id+" not used (key files on the WRM server are for administrators) — add the key or a password")
			}
		}
		item := &importItem{c: c, folder: folder}
		if pj := get("proxyjump"); pj != "" && !strings.EqualFold(pj, "none") {
			hops := strings.Split(pj, ",")
			for i := range hops {
				hops[i] = strings.TrimSpace(hops[i])
			}
			item.jumpName = addImplicit(hops[len(hops)-1])
			if len(hops) > 1 {
				im.note(alias, "ProxyJump chain "+pj+": the hops are linked if they are separate Host entries; check the jump host of each")
			}
		} else if pc := get("proxycommand"); pc != "" && !strings.EqualFold(pc, "none") {
			if m := proxyCmdJump.FindStringSubmatch(pc); m != nil {
				j := m[1]
				if j == "" {
					j = m[2]
				}
				item.jumpName = addImplicit(j)
			} else {
				im.note(alias, "ProxyCommand not imported: "+pc)
			}
		}
		for _, v := range lookup(alias, b, "localforward") {
			if d, ok := parseForward("local", v); ok {
				item.tunnels = append(item.tunnels, d)
			} else {
				im.note(alias, "LocalForward not understood: "+v)
			}
		}
		for _, v := range lookup(alias, b, "remoteforward") {
			if d, ok := parseForward("remote", v); ok {
				item.tunnels = append(item.tunnels, d)
			} else {
				im.note(alias, "RemoteForward not understood: "+v)
			}
		}
		for _, v := range lookup(alias, b, "dynamicforward") {
			if d, ok := parseForward("dynamic", v); ok {
				item.tunnels = append(item.tunnels, d)
			} else {
				im.note(alias, "DynamicForward not understood: "+v)
			}
		}
		im.items = append(im.items, item)
	}
	if len(im.items) == 0 {
		return nil, fmt.Errorf("no Host entries found")
	}
	if err := im.commit(); err != nil {
		return nil, err
	}
	return &im.res, nil
}

func splitSSHOption(line string) (string, string) {
	i := strings.IndexAny(line, " \t=")
	if i < 0 {
		return line, ""
	}
	key := line[:i]
	val := strings.TrimLeft(line[i:], " \t")
	val = strings.TrimSpace(strings.TrimPrefix(val, "="))
	return key, val
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") || p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}

// parseForward reads "[bind:]port host:hostport" (local/remote) or "[bind:]port" (dynamic).
func parseForward(kind, v string) (tunnelDef, bool) {
	f := strings.Fields(v)
	if len(f) == 0 {
		return tunnelDef{}, false
	}
	d := tunnelDef{Kind: kind, BindHost: "127.0.0.1", StartMode: "connect"}
	bind := f[0]
	if strings.HasPrefix(bind, "[") || strings.Count(bind, ":") == 1 {
		h, p, err := net.SplitHostPort(bind)
		if err != nil {
			return d, false
		}
		if h == "*" || h == "" {
			h = "0.0.0.0"
		}
		d.BindHost, bind = h, p
	}
	port, err := strconv.Atoi(bind)
	if err != nil {
		return d, false
	}
	d.BindPort = port
	if kind == "dynamic" {
		return d, len(f) == 1
	}
	if len(f) != 2 {
		return d, false
	}
	h, p, err := net.SplitHostPort(f[1])
	if err != nil {
		return d, false
	}
	tp, err := strconv.Atoi(p)
	if err != nil {
		return d, false
	}
	d.TargetHost, d.TargetPort = h, tp
	if kind == "local" && (tp == 443 || tp == 8443) {
		d.OpenScheme = "https"
	} else if kind == "local" && (tp == 80 || tp == 8080 || tp == 3000 || tp == 9090) {
		d.OpenScheme = "http"
	}
	return d, true
}

// ── API ──

// POST /api/config/import/mremoteng   {"xml": "...", "password": "optional master password"}
// POST /api/config/import/sshconfig   {"text": "...", "folder": "optional folder name"}
// POST /api/config/import/putty       {"data": "base64 of the .reg file", "folder": "optional folder name"}
func apiImportExternalHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	var in struct {
		XML      string `json:"xml"`
		Text     string `json:"text"`
		Password string `json:"password"`
		Folder   string `json:"folder"`
		Data     string `json:"data"` // PuTTY: the .reg file, base64 (UTF-16 or UTF-8)
	}
	if !decodeImportJSON(w, r, &in) {
		return
	}
	if n := int64(len(in.XML) + len(in.Text) + base64.StdEncoding.DecodedLen(len(in.Data))); n > maxImportFile {
		importTooLarge(w, n)
		return
	}
	var res *importResult
	var err error
	source := strings.TrimPrefix(r.URL.Path, "/api/config/import/")
	switch source {
	case "mremoteng":
		res, err = importMRemoteNG(userID, []byte(in.XML), in.Password)
		if err == errNeedPassword {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error(), "need_password": true})
			return
		}
	case "sshconfig":
		res, err = importSSHConfig(userID, in.Text, in.Folder)
	case "putty":
		raw, derr := base64.StdEncoding.DecodeString(in.Data)
		if derr != nil {
			raw = []byte(in.Text)
		}
		res, err = importPuTTY(userID, raw, in.Folder)
	default:
		jsonError(w, "Not found", 404)
		return
	}
	if err != nil {
		jsonError(w, err.Error(), 400)
		return
	}
	auditLog(r, userID, "config.imported", source, map[string]interface{}{"connections": res.Imported, "folders": res.Folders,
		"tunnels": res.Tunnels, "jump_hosts": res.Jumps, "proxies": res.Proxies, "proxy_links": res.Linked, "skipped": len(res.Skipped)})
	broadcastSessionUpdate(userID)
	jsonOK(w, res)
}
