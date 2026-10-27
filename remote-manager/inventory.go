// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

// ─── TAGS & INVENTORY IMPORT ─────────────────────────
//
// Connections carry tags ("env:prod", "site:dc1", "rack:a12", "web"…), lower case, at most
// 30 per connection. Tags with a key ("env:", "site:", "rack:", "role:", "tenant:",
// "platform:") come from inventories; the UI filters by tags and shows env: as a badge.
//
// Inventories are imported from:
//   - CSV files (comma, semicolon, tab or | separated; UTF-8, UTF-16 or Windows-1250 as
//     saved by Excel in Central Europe) and Excel .xlsx workbooks (read with archive/zip +
//     encoding/xml, no extra dependency), with a column mapping the user checks in a preview;
//   - NetBox (devices and virtual machines through its REST API, with filters). Imported
//     NetBox objects remember their id (connections.ext_id), so a later import updates them
//     ("sync") instead of creating duplicates, and reports objects that disappeared.
//
// Imported connections can log in with a vault credential or a stored SSH key right away.

const (
	maxConnTags      = 30
	maxInventoryRows = 20000
	maxNetboxObjects = 10000
)

var tagStripRe = regexp.MustCompile(`[^\p{L}\p{N}_.:/@+#-]+`)
var tagColonRe = regexp.MustCompile(`\s*:\s*`)

func normalizeTag(s string) string {
	s = strings.ToLower(strings.Join(strings.Fields(tagColonRe.ReplaceAllString(s, ":")), "-"))
	s = strings.Trim(tagStripRe.ReplaceAllString(s, ""), ":-")
	return truncateStr(s, 64)
}

// normalizeTags cleans, de-duplicates and limits tags; entries may be comma separated.
func normalizeTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		for _, p := range strings.FieldsFunc(t, func(r rune) bool { return r == ',' || r == ';' || r == '|' }) {
			if n := normalizeTag(p); n != "" && !seen[n] {
				seen[n] = true
				out = append(out, n)
				if len(out) == maxConnTags {
					return out
				}
			}
		}
	}
	return out
}

func tagsString(t []string) string { return strings.Join(normalizeTags(t), ",") }

func parseTags(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}

// keyedTag returns "key:value" ("" for an empty value).
func keyedTag(key, val string) string {
	if v := normalizeTag(val); v != "" {
		return key + ":" + v
	}
	return ""
}

// mergeTags returns cur without the tags in drop, plus add.
func mergeTags(cur, drop, add []string) []string {
	d := map[string]bool{}
	for _, t := range drop {
		d[t] = true
	}
	keep := []string{}
	for _, t := range cur {
		if !d[t] {
			keep = append(keep, t)
		}
	}
	return normalizeTags(append(keep, add...))
}

// bulkTag adds and removes tags on the user's connections; returns how many changed.
func bulkTag(userID int, ids []int, add, remove []string) int {
	add, remove = normalizeTags(add), normalizeTags(remove)
	n := 0
	for _, id := range ids {
		var cur string
		if db.QueryRow(`SELECT COALESCE(tags,'') FROM connections WHERE id=? AND user_id=?`, id, userID).Scan(&cur) != nil {
			continue
		}
		next := tagsString(mergeTags(parseTags(cur), remove, add))
		if next != cur {
			db.Exec(`UPDATE connections SET tags=? WHERE id=? AND user_id=?`, next, id, userID)
			n++
		}
	}
	return n
}

// ── table files: CSV and XLSX ──

type tableFile struct {
	Sheets    []string `json:"sheets"`
	Sheet     string   `json:"sheet"`
	Encoding  string   `json:"encoding,omitempty"`
	Delimiter string   `json:"delimiter,omitempty"`
	rows      [][]string
}

func parseTableFile(name string, data []byte, sheet string) (*tableFile, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("the file is empty")
	}
	if len(data) > maxImportFile {
		return nil, fmt.Errorf("the file is too large (%s): the import limit is %s", formatMB(int64(len(data))), formatMB(maxImportFile))
	}
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return parseXLSX(data, sheet)
	}
	if bytes.HasPrefix(data, []byte{0xd0, 0xcf, 0x11, 0xe0}) || strings.HasSuffix(strings.ToLower(name), ".xls") {
		return nil, fmt.Errorf("old Excel files (.xls) are not supported: save the sheet as .xlsx or CSV")
	}
	return parseCSV(data)
}

// cp1250 maps the bytes 0x80–0xFF of Windows-1250 (Central European) to Unicode.
var cp1250 = []rune("€\u0081‚\u0083„…†‡\u0088‰Š‹ŚŤŽŹ" +
	"\u0090‘’“”•–—\u0098™š›śťžź" +
	" ˇ˘Ł¤Ą¦§¨©Ş«¬­®Ż" +
	"°±˛ł´µ¶·¸ąş»Ľ˝ľż" +
	"ŔÁÂĂÄĹĆÇČÉĘËĚÍÎĎ" +
	"ĐŃŇÓÔŐÖ×ŘŮÚŰÜÝŢß" +
	"ŕáâăäĺćçčéęëěíîď" +
	"đńňóôőö÷řůúűüýţ˙")

func decodeCP1250(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c < 0x80 {
			sb.WriteByte(c)
		} else {
			sb.WriteRune(cp1250[c-0x80])
		}
	}
	return sb.String()
}

func decodeUTF16(b []byte, bigEndian bool) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if bigEndian {
			u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
		} else {
			u = append(u, uint16(b[i+1])<<8|uint16(b[i]))
		}
	}
	return string(utf16.Decode(u))
}

func detectDelimiter(text string) rune {
	lines := []string{}
	for _, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
			if len(lines) == 5 {
				break
			}
		}
	}
	best, bestScore := ',', -1
	for _, d := range []rune{',', ';', '\t', '|'} {
		// The count outside quotes on the first line, if the next lines agree.
		count := func(l string) int {
			n, q := 0, false
			for _, r := range l {
				if r == '"' {
					q = !q
				} else if r == d && !q {
					n++
				}
			}
			return n
		}
		if len(lines) == 0 {
			break
		}
		first := count(lines[0])
		score := first * 10
		for _, l := range lines[1:] {
			if count(l) == first {
				score++
			}
		}
		if first > 0 && score > bestScore {
			best, bestScore = d, score
		}
	}
	return best
}

func parseCSV(data []byte) (*tableFile, error) {
	t := &tableFile{Sheets: []string{}, Encoding: "UTF-8"}
	var text string
	switch {
	case bytes.HasPrefix(data, []byte{0xff, 0xfe}):
		text, t.Encoding = decodeUTF16(data[2:], false), "UTF-16"
	case bytes.HasPrefix(data, []byte{0xfe, 0xff}):
		text, t.Encoding = decodeUTF16(data[2:], true), "UTF-16"
	default:
		data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
		if utf8.Valid(data) {
			text = string(data)
		} else {
			text, t.Encoding = decodeCP1250(data), "Windows-1250"
		}
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if strings.HasPrefix(text, "sep=") { // Excel's "sep=;" first line
		if i := strings.IndexByte(text, '\n'); i > 0 {
			text = text[i+1:]
		}
	}
	d := detectDelimiter(text)
	t.Delimiter = map[rune]string{',': ",", ';': ";", '\t': "tab", '|': "|"}[d]
	r := csv.NewReader(strings.NewReader(text))
	r.Comma, r.FieldsPerRecord, r.LazyQuotes, r.TrimLeadingSpace = d, -1, true, true
	for len(t.rows) <= maxInventoryRows {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("CSV line %d: %v", len(t.rows)+1, err)
		}
		t.rows = append(t.rows, rec)
	}
	return t, nil
}

func xlsxColumn(ref string) int {
	n := 0
	for _, r := range ref {
		if r >= 'A' && r <= 'Z' {
			n = n*26 + int(r-'A'+1)
		} else if r >= 'a' && r <= 'z' {
			n = n*26 + int(r-'a'+1)
		} else {
			break
		}
	}
	return n - 1
}

type xlsxText struct {
	T string `xml:"t"`
	R []struct {
		T string `xml:"t"`
	} `xml:"r"`
}

func (x xlsxText) String() string {
	if len(x.R) == 0 {
		return x.T
	}
	var sb strings.Builder
	sb.WriteString(x.T)
	for _, r := range x.R {
		sb.WriteString(r.T)
	}
	return sb.String()
}

func parseXLSX(data []byte, want string) (*tableFile, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("not a valid .xlsx file: %v", err)
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[strings.TrimPrefix(f.Name, "/")] = f
	}
	read := func(name string) ([]byte, error) {
		f := files[name]
		if f == nil {
			return nil, fmt.Errorf("%s is missing", name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(io.LimitReader(rc, 64<<20))
	}
	var wb struct {
		Sheets []struct {
			Name string `xml:"name,attr"`
			RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sheets>sheet"`
	}
	b, err := read("xl/workbook.xml")
	if err != nil {
		return nil, fmt.Errorf("not an Excel workbook: %v", err)
	}
	if err := xml.Unmarshal(b, &wb); err != nil || len(wb.Sheets) == 0 {
		return nil, fmt.Errorf("the workbook has no sheets")
	}
	var rels struct {
		Rels []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if b, err := read("xl/_rels/workbook.xml.rels"); err == nil {
		xml.Unmarshal(b, &rels)
	}
	target := map[string]string{}
	for _, r := range rels.Rels {
		p := r.Target
		if strings.HasPrefix(p, "/") {
			p = strings.TrimPrefix(p, "/")
		} else {
			p = "xl/" + p
		}
		target[r.ID] = p
	}
	t := &tableFile{}
	idx := 0
	for i, s := range wb.Sheets {
		t.Sheets = append(t.Sheets, s.Name)
		if want != "" && s.Name == want {
			idx = i
		}
	}
	t.Sheet = wb.Sheets[idx].Name
	sheetPath := target[wb.Sheets[idx].RID]
	if sheetPath == "" {
		sheetPath = fmt.Sprintf("xl/worksheets/sheet%d.xml", idx+1)
	}
	var shared []string
	if b, err := read("xl/sharedStrings.xml"); err == nil {
		var sst struct {
			SI []xlsxText `xml:"si"`
		}
		xml.Unmarshal(b, &sst)
		for _, si := range sst.SI {
			shared = append(shared, si.String())
		}
	}
	b, err = read(sheetPath)
	if err != nil {
		return nil, fmt.Errorf("sheet %q: %v", t.Sheet, err)
	}
	var ws struct {
		Rows []struct {
			R int `xml:"r,attr"`
			C []struct {
				R  string   `xml:"r,attr"`
				T  string   `xml:"t,attr"`
				V  string   `xml:"v"`
				IS xlsxText `xml:"is"`
			} `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(b, &ws); err != nil {
		return nil, fmt.Errorf("sheet %q: %v", t.Sheet, err)
	}
	for i, row := range ws.Rows {
		rn := row.R - 1
		if rn < 0 {
			rn = i
		}
		if rn >= maxInventoryRows {
			break
		}
		for len(t.rows) <= rn {
			t.rows = append(t.rows, []string{})
		}
		cells := t.rows[rn]
		for j, c := range row.C {
			col := j
			if c.R != "" {
				col = xlsxColumn(c.R)
			}
			if col < 0 || col >= 200 {
				continue
			}
			v := c.V
			switch c.T {
			case "s":
				if n, err := strconv.Atoi(strings.TrimSpace(c.V)); err == nil && n >= 0 && n < len(shared) {
					v = shared[n]
				}
			case "inlineStr":
				v = c.IS.String()
			case "b":
				v = map[string]string{"1": "TRUE", "0": "FALSE"}[c.V]
			case "e":
				v = ""
			default:
				if f, err := strconv.ParseFloat(c.V, 64); err == nil && f == float64(int64(f)) && !strings.ContainsAny(c.V, "eE") {
					v = strconv.FormatInt(int64(f), 10)
				}
			}
			for len(cells) <= col {
				cells = append(cells, "")
			}
			cells[col] = v
		}
		t.rows[rn] = cells
	}
	return t, nil
}

// ── column mapping ──

var inventoryFields = []string{"name", "host", "port", "protocol", "username", "password", "folder", "tags", "env", "site", "rack", "role", "platform", "jump", "web_path"}

var fieldSynonyms = map[string][]string{
	"name":     {"name", "naziv", "ime", "server", "servername", "device", "devicename", "uredaj", "vm", "label", "title", "connection", "veza", "hostname"},
	"host":     {"ip", "ipaddress", "ipadresa", "ipv4", "primaryip", "primaryipv4", "mgmtip", "address", "adresa", "host", "hostname", "fqdn", "dns", "url", "target"},
	"port":     {"port"},
	"protocol": {"protocol", "protokol", "type", "tip", "service", "servis", "connectiontype"},
	"username": {"user", "username", "login", "korisnik", "korisnickoime", "account", "racun", "userid"},
	"password": {"password", "pass", "lozinka", "pwd", "secret", "zaporka"},
	"folder":   {"folder", "mapa", "group", "grupa", "directory", "kategorija", "category", "container", "path"},
	"tags":     {"tags", "tag", "oznake", "labels"},
	"env":      {"env", "environment", "okruzenje", "stage"},
	"site":     {"site", "location", "lokacija", "datacenter", "dc", "lokacijadc", "podatkovnicentar"},
	"rack":     {"rack", "ormar", "regal"},
	"role":     {"role", "uloga", "function", "funkcija", "devicerole"},
	"platform": {"platform", "os", "operatingsystem", "sustav", "platforma", "operativnisustav"},
	"jump":     {"jump", "jumphost", "bastion", "proxy", "via", "proxyjump", "gateway"},
	"web_path": {"webpath", "urlpath"},
}

var foldRepl = strings.NewReplacer("č", "c", "ć", "c", "š", "s", "ž", "z", "đ", "d", "dž", "dz")
var nonAlnumRe = regexp.MustCompile(`[^a-z0-9]+`)

func foldHeader(h string) string {
	return nonAlnumRe.ReplaceAllString(foldRepl.Replace(strings.ToLower(strings.TrimSpace(h))), "")
}

// guessMapping maps fields to column indexes from the header row. Synonyms are tried in
// order, so "IP" wins over "Hostname" for the host and "Hostname" then names the connection.
func guessMapping(headers []string) map[string]int {
	m := map[string]int{}
	used := map[int]bool{}
	folded := make([]string, len(headers))
	for i, h := range headers {
		folded[i] = foldHeader(h)
	}
	// The address first: a "Hostname" column is the name only when there is an IP column.
	order := append([]string{"host"}, inventoryFields...)
	for _, f := range order {
		if _, done := m[f]; done {
			continue
		}
	syn:
		for _, s := range fieldSynonyms[f] {
			for i, fh := range folded {
				if !used[i] && fh == s {
					m[f] = i
					used[i] = true
					break syn
				}
			}
		}
	}
	return m
}

type inventoryOptions struct {
	DefaultProtocol string `json:"default_protocol"`
	FolderMode      string `json:"folder_mode"` // column | site | role | tenant | fixed | none
	Folder          string `json:"folder"`
	Tags            string `json:"tags"`     // added to every connection
	Username        string `json:"username"` // when the file has none
	Auth            string `json:"auth"`     // "" | credential | key
	CredentialID    int    `json:"credential_id"`
	KeyID           int    `json:"key_id"`
	Update          bool   `json:"update"` // NetBox: update connections imported before
}

func (o *inventoryOptions) normalize(userID int) error {
	o.DefaultProtocol = strings.ToUpper(strings.TrimSpace(o.DefaultProtocol))
	if !validProtocols[o.DefaultProtocol] {
		o.DefaultProtocol = "SSH"
	}
	switch o.Auth {
	case "credential":
		cr, err := loadCredential(o.CredentialID)
		if err != nil || !credentialAccessible(cr, userID) {
			return fmt.Errorf("credential not found")
		}
	case "key":
		if k, err := loadSSHKey(o.KeyID, userID); err != nil || !k.HasPrivate {
			return fmt.Errorf("SSH key not found")
		}
	default:
		o.Auth = ""
	}
	o.Username = truncateStr(strings.TrimSpace(o.Username), 120)
	return nil
}

// applyLogin sets the login chosen for imported connections without one of their own.
func (o *inventoryOptions) applyLogin(c *Connection) {
	if c.Password != "" {
		return
	}
	switch o.Auth {
	case "credential":
		id := o.CredentialID
		c.AuthMethod, c.CredentialID = "CREDENTIAL", &id
	case "key":
		if isSSHProtocol(*c) {
			id := o.KeyID
			c.AuthMethod, c.KeyID = "KEY_REF", &id
		}
	}
}

var protocolAliases = map[string]string{"ssh": "SSH", "sftp": "SFTP", "scp": "SFTP", "rdp": "RDP", "remotedesktop": "RDP", "mstsc": "RDP", "vnc": "VNC",
	"telnet": "TELNET", "ftp": "FTP", "ftps": "FTPS", "http": "HTTP", "https": "HTTPS", "web": "HTTPS", "ssh2": "SSH", "putty": "SSH"}

// splitHostField understands "10.0.0.5", "10.0.0.5:2222", "user@host", "10.0.0.5/24",
// "[fd00::1]:22" and URLs like https://idrac-01/ or ssh://root@host:22.
func splitHostField(v string) (host, user, proto, path string) {
	v = strings.TrimSpace(v)
	if i := strings.Index(v, "://"); i > 0 {
		if u, err := url.Parse(v); err == nil && u.Host != "" {
			proto = protocolAliases[strings.ToLower(u.Scheme)]
			if u.User != nil {
				user = u.User.Username()
			}
			if u.Path != "" && u.Path != "/" {
				path = u.Path
			}
			return u.Host, user, proto, path
		}
	}
	if i := strings.LastIndex(v, "@"); i > 0 {
		user, v = v[:i], v[i+1:]
	}
	if i := strings.Index(v, "/"); i > 0 {
		if ip := net.ParseIP(v[:i]); ip != nil {
			v = v[:i]
		}
	}
	if ip := net.ParseIP(v); ip != nil && ip.To4() == nil {
		v = "[" + v + "]"
	}
	return v, user, proto, path
}

// itemFromRow turns a table row into a connection to import (nil for an empty row).
func itemFromRow(row []string, m map[string]int, o *inventoryOptions) *importItem {
	get := func(f string) string {
		if i, ok := m[f]; ok && i >= 0 && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	name := get("name")
	host, user, proto, path := splitHostField(get("host"))
	if host == "" {
		if name == "" {
			return nil
		}
		host, user, proto, path = splitHostField(name)
	}
	if p := protocolAliases[foldHeader(get("protocol"))]; p != "" {
		proto = p
	}
	platform := get("platform")
	if proto == "" {
		if strings.Contains(strings.ToLower(platform), "windows") {
			proto = "RDP"
		} else {
			proto = o.DefaultProtocol
		}
	}
	if _, _, err := net.SplitHostPort(host); err != nil { // no port in the host field
		if p, err := strconv.Atoi(get("port")); err == nil && p > 0 && p < 65536 {
			host = net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(p))
		} else if strings.HasPrefix(host, "[") {
			host = net.JoinHostPort(strings.Trim(host, "[]"), defaultPortFor(proto))
		}
	}
	if name == "" {
		name = hostOnly(host)
	}
	if u := get("username"); u != "" {
		user = u
	}
	if user == "" {
		user = o.Username
	}
	c := Connection{Name: name, Host: host, Username: user, Protocol: proto, AuthMethod: "PASSWORD", Password: get("password"), WebPath: firstNonEmpty(get("web_path"), path)}
	tags := append([]string{o.Tags, get("tags")}, keyedTag("env", get("env")), keyedTag("site", get("site")), keyedTag("rack", get("rack")),
		keyedTag("role", get("role")), keyedTag("platform", platform))
	c.Tags = normalizeTags(tags)
	o.applyLogin(&c)
	folder := ""
	switch o.FolderMode {
	case "column":
		folder = get("folder")
	case "site":
		folder = get("site")
	case "role":
		folder = get("role")
	case "fixed":
		folder = o.Folder
	}
	return &importItem{c: c, folder: folder, jumpName: get("jump")}
}

// ── NetBox ──

type netboxQuery struct {
	URL      string `json:"url"`
	Token    string `json:"token"`
	Insecure bool   `json:"insecure"`
	Devices  bool   `json:"devices"`
	VMs      bool   `json:"vms"`
	Site     string `json:"site"`
	Role     string `json:"role"`
	Tenant   string `json:"tenant"`
	Tag      string `json:"tag"`
	Status   string `json:"status"`
	EnvField string `json:"env_field"` // custom field with the environment
	HostFrom string `json:"host_from"` // ip | name
	Remember bool   `json:"remember"`
}

type netboxRow struct {
	ExtID    string   `json:"ext_id"`
	Kind     string   `json:"kind"` // device | vm
	Name     string   `json:"name"`
	Host     string   `json:"host"`
	Site     string   `json:"site"`
	Rack     string   `json:"rack"`
	Role     string   `json:"role"`
	Tenant   string   `json:"tenant"`
	Platform string   `json:"platform"`
	Status   string   `json:"status"`
	Env      string   `json:"env"`
	Cluster  string   `json:"cluster,omitempty"`
	Tags     []string `json:"tags"`
	Protocol string   `json:"protocol"`
	ConnID   int      `json:"conn_id,omitempty"`
	ConnName string   `json:"conn_name,omitempty"`
	Problem  string   `json:"problem,omitempty"`
}

type nbRef struct {
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	Display string `json:"display"`
	Address string `json:"address"`
	Value   string `json:"value"`
	Label   string `json:"label"`
}

type nbObject struct {
	ID           int                    `json:"id"`
	Name         *string                `json:"name"`
	Display      string                 `json:"display"`
	PrimaryIP4   *nbRef                 `json:"primary_ip4"`
	PrimaryIP6   *nbRef                 `json:"primary_ip6"`
	PrimaryIP    *nbRef                 `json:"primary_ip"`
	Site         *nbRef                 `json:"site"`
	Rack         *nbRef                 `json:"rack"`
	Role         *nbRef                 `json:"role"`
	DeviceRole   *nbRef                 `json:"device_role"`
	Tenant       *nbRef                 `json:"tenant"`
	Platform     *nbRef                 `json:"platform"`
	Status       *nbRef                 `json:"status"`
	Cluster      *nbRef                 `json:"cluster"`
	Tags         []nbRef                `json:"tags"`
	CustomFields map[string]interface{} `json:"custom_fields"`
}

func (r *nbRef) name() string {
	if r == nil {
		return ""
	}
	return firstNonEmpty(r.Name, r.Display, r.Label, r.Value)
}

func (r *nbRef) slug() string {
	if r == nil {
		return ""
	}
	return firstNonEmpty(r.Slug, r.Value, r.Name, r.Display)
}

func (r *nbRef) ip() string {
	if r == nil || r.Address == "" {
		return ""
	}
	a := r.Address
	if i := strings.IndexByte(a, '/'); i > 0 {
		a = a[:i]
	}
	return a
}

// netboxBase validates the NetBox address and returns it without a trailing slash.
func netboxBase(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw != "" && !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("enter the NetBox address, e.g. https://netbox.example.com")
	}
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/api")
	u.RawQuery, u.Fragment = "", ""
	return u, nil
}

func netboxClient(insecure bool, base *url.URL) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: insecure} //nolint:gosec // user's choice for self-signed NetBox
	return &http.Client{Timeout: 30 * time.Second, Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Host != base.Host || len(via) > 3 {
			return fmt.Errorf("NetBox redirected to another server (%s)", req.URL.Host)
		}
		return nil
	}}
}

func netboxExtPrefix(base *url.URL) string { return "netbox:" + strings.ToLower(base.Host) + ":" }

// netboxFetch reads devices and virtual machines and turns them into rows.
func netboxFetch(q netboxQuery) ([]netboxRow, *url.URL, error) {
	base, err := netboxBase(q.URL)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(q.Token) == "" {
		return nil, nil, fmt.Errorf("an API token is required (NetBox → your profile → API tokens; read access is enough)")
	}
	cl := netboxClient(q.Insecure, base)
	params := url.Values{"limit": {"500"}}
	add := func(key, vals string) {
		for _, v := range strings.FieldsFunc(vals, func(r rune) bool { return r == ',' || r == ' ' }) {
			params.Add(key, v)
		}
	}
	add("site", q.Site)
	add("tenant", q.Tenant)
	add("tag", q.Tag)
	add("status", firstNonEmpty(q.Status, "active"))
	if q.Status == "any" {
		params.Del("status")
	}
	rows := []netboxRow{}
	fetch := func(path, kind string) error {
		p := params
		if q.Role != "" {
			p = url.Values{}
			for k, v := range params {
				p[k] = v
			}
			for _, v := range strings.FieldsFunc(q.Role, func(r rune) bool { return r == ',' || r == ' ' }) {
				p.Add("role", v)
			}
		}
		next := base.String() + path + "?" + p.Encode()
		for next != "" {
			nu, err := url.Parse(next)
			if err != nil || nu.Host != base.Host {
				return fmt.Errorf("NetBox returned a link to another server")
			}
			req, _ := http.NewRequest("GET", next, nil)
			req.Header.Set("Authorization", "Token "+strings.TrimSpace(q.Token))
			req.Header.Set("Accept", "application/json")
			resp, err := cl.Do(req)
			if err != nil {
				return fmt.Errorf("NetBox: %v", err)
			}
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
			resp.Body.Close()
			switch {
			case resp.StatusCode == 401 || resp.StatusCode == 403:
				return fmt.Errorf("NetBox refused the token (%d)", resp.StatusCode)
			case resp.StatusCode == 404:
				return fmt.Errorf("NetBox API not found at %s (wrong address?)", base.String())
			case resp.StatusCode != 200:
				return fmt.Errorf("NetBox answered %d: %s", resp.StatusCode, truncateStr(strings.TrimSpace(string(body)), 200))
			}
			var page struct {
				Count   int        `json:"count"`
				Next    *string    `json:"next"`
				Results []nbObject `json:"results"`
			}
			if err := json.Unmarshal(body, &page); err != nil {
				return fmt.Errorf("NetBox answered something that is not its API (%v)", err)
			}
			for _, o := range page.Results {
				rows = append(rows, netboxRowFrom(o, kind, base, q))
				if len(rows) > maxNetboxObjects {
					return fmt.Errorf("more than %d objects: narrow the import with filters", maxNetboxObjects)
				}
			}
			next = ""
			if page.Next != nil {
				next = *page.Next
			}
		}
		return nil
	}
	if !q.Devices && !q.VMs {
		q.Devices = true
	}
	if q.Devices {
		if err := fetch("/api/dcim/devices/", "device"); err != nil {
			return nil, base, err
		}
	}
	if q.VMs {
		if err := fetch("/api/virtualization/virtual-machines/", "vm"); err != nil {
			return nil, base, err
		}
	}
	return rows, base, nil
}

func netboxRowFrom(o nbObject, kind string, base *url.URL, q netboxQuery) netboxRow {
	name := o.Display
	if o.Name != nil && *o.Name != "" {
		name = *o.Name
	}
	role := o.Role
	if role == nil {
		role = o.DeviceRole
	}
	r := netboxRow{ExtID: fmt.Sprintf("%s%s:%d", netboxExtPrefix(base), kind, o.ID), Kind: kind, Name: name,
		Site: o.Site.slug(), Rack: o.Rack.name(), Role: role.slug(), Tenant: o.Tenant.slug(), Platform: o.Platform.slug(),
		Status: o.Status.slug(), Cluster: o.Cluster.name(), Tags: []string{}}
	for _, t := range o.Tags {
		r.Tags = append(r.Tags, t.slug())
	}
	if q.EnvField != "" {
		if v, ok := o.CustomFields[q.EnvField]; ok && v != nil {
			switch x := v.(type) {
			case string:
				r.Env = x
			case map[string]interface{}:
				r.Env = fmt.Sprint(firstNonEmpty(fmt.Sprint(x["value"]), fmt.Sprint(x["label"])))
			default:
				r.Env = fmt.Sprint(x)
			}
		}
	}
	ip := firstNonEmpty(o.PrimaryIP4.ip(), o.PrimaryIP.ip(), o.PrimaryIP6.ip())
	if q.HostFrom == "name" {
		r.Host = name
	} else if ip != "" {
		r.Host = ip
		if p := net.ParseIP(ip); p != nil && p.To4() == nil {
			r.Host = net.JoinHostPort(ip, "22")
		}
	} else if strings.Contains(name, ".") {
		r.Host = name
	} else {
		r.Problem = "no primary IP address"
	}
	r.Protocol = "SSH"
	if strings.Contains(strings.ToLower(o.Platform.name()+" "+o.Platform.slug()), "windows") {
		r.Protocol = "RDP"
	}
	return r
}

// netboxTags are the tags WRM manages for a NetBox object.
func (r netboxRow) netboxTags() []string {
	t := []string{keyedTag("site", r.Site), keyedTag("rack", r.Rack), keyedTag("role", r.Role), keyedTag("tenant", r.Tenant),
		keyedTag("platform", r.Platform), keyedTag("env", r.Env), keyedTag("cluster", r.Cluster)}
	return normalizeTags(append(t, r.Tags...))
}

func (r netboxRow) folder(mode, fixed string) string {
	switch mode {
	case "site":
		return r.Site
	case "role":
		return r.Role
	case "tenant":
		return r.Tenant
	case "fixed":
		return fixed
	}
	return ""
}

// ── saved inventory sources (NetBox address and token per user) ──

func loadNetboxSource(userID int) (netboxQuery, bool) {
	var q netboxQuery
	var token, opts string
	if db.QueryRow(`SELECT url, token, options FROM inventory_sources WHERE user_id=? AND kind='netbox'`, userID).Scan(&q.URL, &token, &opts) != nil {
		return q, false
	}
	json.Unmarshal([]byte(opts), &q)
	q.Token = decryptValue(token)
	return q, true
}

func saveNetboxSource(userID int, q netboxQuery) {
	token := q.Token
	q.Token = ""
	db.Exec(`DELETE FROM inventory_sources WHERE user_id=? AND kind='netbox'`, userID)
	db.Exec(`INSERT INTO inventory_sources (user_id, kind, url, token, options, last_sync_at) VALUES (?,?,?,?,?,?)`,
		userID, "netbox", q.URL, encryptValue(token), string(jsonMarshal(q)), time.Now().UTC().Format(time.RFC3339))
}

// ── API ──

// POST /api/inventory/parse            {file_name, data (base64), sheet} → columns, preview, mapping guess
// POST /api/inventory/import           {file_name, data, sheet, header, mapping, options} → import result
// GET  /api/inventory/netbox           saved NetBox address and filters (never the token)
// POST /api/inventory/netbox/preview   {query} → rows, already imported, missing
// POST /api/inventory/netbox/import    {query, rows, options} → import result
func apiInventoryHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireAuth(w, r)
	if !ok {
		return
	}
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/inventory"), "/")
	if action == "netbox" && r.Method == http.MethodGet {
		q, ok := loadNetboxSource(userID)
		has := q.Token != ""
		q.Token = ""
		jsonOK(w, map[string]interface{}{"saved": ok, "query": q, "has_token": has})
		return
	}
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	var in struct {
		FileName string           `json:"file_name"`
		Data     string           `json:"data"`
		Sheet    string           `json:"sheet"`
		Header   *bool            `json:"header"`
		Mapping  map[string]int   `json:"mapping"`
		Options  inventoryOptions `json:"options"`
		Query    netboxQuery      `json:"query"`
		Rows     []netboxRow      `json:"rows"`
	}
	if !decodeImportJSON(w, r, &in) {
		return
	}
	switch action {
	case "parse", "import":
		data, err := base64.StdEncoding.DecodeString(in.Data)
		if err != nil {
			jsonError(w, "Bad file data", 400)
			return
		}
		if len(data) > maxImportFile {
			importTooLarge(w, int64(len(data)))
			return
		}
		tf, err := parseTableFile(in.FileName, data, in.Sheet)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		header := in.Header == nil || *in.Header
		var headers []string
		body := tf.rows
		if len(body) > 0 && header {
			headers, body = body[0], body[1:]
		}
		cols := 0
		for _, row := range tf.rows {
			if len(row) > cols {
				cols = len(row)
			}
		}
		if action == "parse" {
			if headers == nil {
				headers = make([]string, cols)
				for i := range headers {
					headers[i] = fmt.Sprintf("%c", 'A'+i%26)
				}
			}
			preview := body
			if len(preview) > 25 {
				preview = preview[:25]
			}
			jsonOK(w, map[string]interface{}{"sheets": tf.Sheets, "sheet": tf.Sheet, "encoding": tf.Encoding, "delimiter": tf.Delimiter,
				"headers": headers, "columns": cols, "rows": preview, "total": len(body), "mapping": guessMapping(headers), "fields": inventoryFields})
			return
		}
		if err := in.Options.normalize(userID); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		if _, ok := in.Mapping["host"]; !ok {
			if _, ok := in.Mapping["name"]; !ok {
				jsonError(w, "Map at least the host (or name) column", 400)
				return
			}
		}
		im := newImporter(userID)
		for i, row := range body {
			it := itemFromRow(row, in.Mapping, &in.Options)
			if it == nil {
				if strings.TrimSpace(strings.Join(row, "")) != "" {
					im.skip(fmt.Sprintf("row %d", i+1+boolInt(header)), "no host")
				}
				continue
			}
			im.items = append(im.items, it)
		}
		if err := im.commit(); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		auditLog(r, userID, "inventory.imported", truncateStr(in.FileName, 120), map[string]interface{}{"format": map[bool]string{true: "xlsx", false: "csv"}[len(tf.Sheets) > 0],
			"connections": im.res.Imported, "folders": im.res.Folders, "skipped": len(im.res.Skipped)})
		broadcastSessionUpdate(userID)
		jsonOK(w, im.res)
	case "netbox/preview", "netbox/import":
		q := in.Query
		if saved, ok := loadNetboxSource(userID); ok && strings.TrimSpace(q.Token) == "" && strings.TrimRight(saved.URL, "/") == strings.TrimRight(q.URL, "/") {
			q.Token = saved.Token
		}
		base, err := netboxBase(q.URL)
		if err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		prefix := netboxExtPrefix(base)
		existing := map[string][2]interface{}{}
		if rows, err := db.Query(`SELECT id, name, ext_id FROM connections WHERE user_id=? AND ext_id LIKE ?`, userID, prefix+"%"); err == nil {
			for rows.Next() {
				var id int
				var n, e string
				rows.Scan(&id, &n, &e)
				existing[e] = [2]interface{}{id, n}
			}
			rows.Close()
		}
		if action == "netbox/preview" {
			rows, _, err := netboxFetch(q)
			if err != nil {
				jsonError(w, err.Error(), 502)
				return
			}
			seen := map[string]bool{}
			for i := range rows {
				seen[rows[i].ExtID] = true
				if e, ok := existing[rows[i].ExtID]; ok {
					rows[i].ConnID, rows[i].ConnName = e[0].(int), e[1].(string)
				}
			}
			missing := []map[string]interface{}{}
			for ext, e := range existing {
				if !seen[ext] {
					missing = append(missing, map[string]interface{}{"conn_id": e[0], "name": e[1], "ext_id": ext})
				}
			}
			sort.Slice(missing, func(i, j int) bool { return fmt.Sprint(missing[i]["name"]) < fmt.Sprint(missing[j]["name"]) })
			if q.Remember {
				saveNetboxSource(userID, q)
			}
			jsonOK(w, map[string]interface{}{"rows": rows, "count": len(rows), "missing": missing, "partial_filters": q.Site != "" || q.Role != "" || q.Tenant != "" || q.Tag != ""})
			return
		}
		if err := in.Options.normalize(userID); err != nil {
			jsonError(w, err.Error(), 400)
			return
		}
		im := newImporter(userID)
		im.update = in.Options.Update
		for _, row := range in.Rows {
			if !strings.HasPrefix(row.ExtID, prefix) {
				im.skip(row.Name, "not from this NetBox")
				continue
			}
			if row.Host == "" {
				im.skip(row.Name, firstNonEmpty(row.Problem, "no address"))
				continue
			}
			proto := strings.ToUpper(row.Protocol)
			if !validProtocols[proto] {
				proto = in.Options.DefaultProtocol
			}
			c := Connection{Name: truncateStr(row.Name, 120), Host: row.Host, Protocol: proto, Username: in.Options.Username, AuthMethod: "PASSWORD"}
			c.Tags = normalizeTags(append(row.netboxTags(), in.Options.Tags))
			in.Options.applyLogin(&c)
			im.items = append(im.items, &importItem{c: c, folder: row.folder(in.Options.FolderMode, in.Options.Folder), extID: row.ExtID, extTags: row.netboxTags()})
		}
		if err := im.commit(); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		if q.Remember {
			saveNetboxSource(userID, q)
		}
		auditLog(r, userID, "inventory.netbox_imported", base.Host, map[string]interface{}{"connections": im.res.Imported, "updated": im.res.Updated,
			"folders": im.res.Folders, "skipped": len(im.res.Skipped)})
		broadcastSessionUpdate(userID)
		jsonOK(w, im.res)
	default:
		jsonError(w, "Not found", 404)
	}
}
