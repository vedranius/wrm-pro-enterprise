// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestTagsNormalizeAndBulk(t *testing.T) {
	if got := normalizeTags([]string{" Env:Prod ", "web, WEB", "rack: A 12", "bad<>tag", "", ":x:"}); strings.Join(got, ",") != "env:prod,web,rack:a-12,badtag,x" {
		t.Fatalf("normalize: %q", got)
	}
	if got := mergeTags([]string{"a", "site:old", "mine"}, []string{"site:old"}, []string{"site:new", "a"}); strings.Join(got, ",") != "a,mine,site:new" {
		t.Fatalf("merge: %q", got)
	}
	srv := newTestServer(t)
	u := newTestUser(t, srv, "tags-user", false)
	other := newTestUser(t, srv, "tags-other", false)
	var v connView
	if code := u.jsonDo("POST", "/api/connections", map[string]interface{}{"name": "web-01", "host": "10.0.0.1", "tags": []string{"Env:Prod", "web"}}, &v); code != 201 || strings.Join(v.Tags, ",") != "env:prod,web" {
		t.Fatalf("create with tags: %d %+v", code, v)
	}
	id2 := u.addConnection("db-01", "10.0.0.2")
	// PUT without tags keeps them, with tags replaces them
	u.jsonDo("PUT", fmt.Sprintf("/api/connections/%d", v.ID), map[string]interface{}{"name": "web-01", "host": "10.0.0.1"}, nil)
	u.jsonDo("GET", fmt.Sprintf("/api/connections/%d", v.ID), nil, &v)
	if len(v.Tags) != 2 {
		t.Fatalf("PUT without tags dropped them: %+v", v.Tags)
	}
	var res map[string]interface{}
	if code := u.jsonDo("POST", "/api/connections/bulk", map[string]interface{}{"action": "tag", "ids": []int{v.ID, id2}, "add": []string{"site:zg1", "Rack:A12"}, "remove": []string{"web"}}, &res); code != 200 || res["changed"] != float64(2) {
		t.Fatalf("bulk tag: %d %v", code, res)
	}
	u.jsonDo("GET", fmt.Sprintf("/api/connections/%d", id2), nil, &v)
	if strings.Join(v.Tags, ",") != "site:zg1,rack:a12" {
		t.Fatalf("bulk tags: %+v", v.Tags)
	}
	// other users' connections are not touched
	if other.jsonDo("POST", "/api/connections/bulk", map[string]interface{}{"action": "tag", "ids": []int{id2}, "add": []string{"pwned"}}, &res); res["changed"] != float64(0) {
		t.Fatalf("tagged someone else's connection: %v", res)
	}
	// export and import keep the tags
	r := u.do("GET", "/api/config/export", nil, "")
	var exp map[string]json.RawMessage
	json.NewDecoder(r.Body).Decode(&exp)
	r.Body.Close()
	if !strings.Contains(string(exp["connections"]), `"tags":["site:zg1","rack:a12"]`) {
		t.Fatalf("export without tags: %s", exp["connections"])
	}
	imp := newTestUser(t, srv, "tags-importer", false)
	if code := imp.jsonDo("POST", "/api/config/import", map[string]json.RawMessage{"connections": exp["connections"]}, &res); code != 200 {
		t.Fatalf("import: %d %v", code, res)
	}
	var list []connView
	imp.jsonDo("GET", "/api/connections", nil, &list)
	found := false
	for _, c := range list {
		if c.Name == "db-01" && strings.Join(c.Tags, ",") == "site:zg1,rack:a12" {
			found = true
		}
	}
	if !found {
		t.Fatalf("imported tags: %+v", list)
	}
}

func TestParseCSVVariants(t *testing.T) {
	// Excel in Croatian: semicolons, Windows-1250, "sep=" line
	raw := []byte("sep=;\r\nNaziv;IP adresa;Korisnik;Lokacija;Okru\x9eenje;Opis\r\nposlu\x9eitelj-\xe8\xe6;10.1.0.5;root;Zagreb;prod;\"ra\xe8un; test\"\r\n")
	tf, err := parseTableFile("servers.csv", raw, "")
	if err != nil || tf.Encoding != "Windows-1250" || tf.Delimiter != ";" || len(tf.rows) != 2 {
		t.Fatalf("cp1250 csv: %v %+v", err, tf)
	}
	if tf.rows[1][0] != "poslužitelj-čć" || tf.rows[1][5] != "račun; test" {
		t.Fatalf("decoded: %q", tf.rows[1])
	}
	m := guessMapping(tf.rows[0])
	if m["name"] != 0 || m["host"] != 1 || m["username"] != 2 || m["site"] != 3 || m["env"] != 4 {
		t.Fatalf("mapping: %v", m)
	}
	// UTF-16 with tabs (Excel "Unicode text")
	u16 := []byte{0xff, 0xfe}
	for _, r := range "host\tuser\r\n10.0.0.9\tadmin\r\n" {
		u16 = append(u16, byte(r), byte(r>>8))
	}
	tf, err = parseTableFile("x.txt", u16, "")
	if err != nil || tf.Delimiter != "tab" || tf.rows[1][1] != "admin" {
		t.Fatalf("utf16: %v %+v", err, tf)
	}
	// plain comma CSV with quotes and a BOM
	tf, _ = parseTableFile("a.csv", []byte("\xef\xbb\xbfname,host\n\"a, b\",h1\n"), "")
	if tf.Delimiter != "," || tf.rows[1][0] != "a, b" || tf.rows[0][0] != "name" {
		t.Fatalf("comma csv: %+v", tf)
	}
	if _, err := parseTableFile("old.xls", []byte{0xd0, 0xcf, 0x11, 0xe0, 1, 2}, ""); err == nil || !strings.Contains(err.Error(), ".xlsx") {
		t.Fatalf("xls: %v", err)
	}
	// host field variants
	for in, want := range map[string]string{"10.0.0.5": "10.0.0.5", "root@10.0.0.5:2222": "10.0.0.5:2222", "10.0.0.5/24": "10.0.0.5",
		"fd00::1": "[fd00::1]", "https://idrac-01.mgmt/login": "idrac-01.mgmt"} {
		if h, _, _, _ := splitHostField(in); h != want {
			t.Errorf("splitHostField(%q) = %q, want %q", in, h, want)
		}
	}
}

// buildXLSX writes a minimal workbook: shared strings, inline strings and numbers.
func buildXLSX(t *testing.T, sheets map[string][][]string, order []string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, body string) {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	shared := []string{}
	idx := map[string]int{}
	si := func(s string) int {
		if i, ok := idx[s]; ok {
			return i
		}
		idx[s] = len(shared)
		shared = append(shared, s)
		return idx[s]
	}
	var wb, rels strings.Builder
	wb.WriteString(`<?xml version="1.0" encoding="UTF-8"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>`)
	rels.WriteString(`<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for n, name := range order {
		fmt.Fprintf(&wb, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, name, n+1, n+1)
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/s%d.xml"/>`, n+1, n+1)
		var ws strings.Builder
		ws.WriteString(`<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
		for r, row := range sheets[name] {
			fmt.Fprintf(&ws, `<row r="%d">`, r+1)
			for c, v := range row {
				ref := fmt.Sprintf("%c%d", 'A'+c, r+1)
				switch {
				case v == "":
				case strings.HasPrefix(v, "#"): // number
					fmt.Fprintf(&ws, `<c r="%s"><v>%s</v></c>`, ref, v[1:])
				case strings.HasPrefix(v, "~"): // inline string
					fmt.Fprintf(&ws, `<c r="%s" t="inlineStr"><is><t>%s</t></is></c>`, ref, v[1:])
				default:
					fmt.Fprintf(&ws, `<c r="%s" t="s"><v>%d</v></c>`, ref, si(v))
				}
			}
			ws.WriteString(`</row>`)
		}
		ws.WriteString(`</sheetData></worksheet>`)
		add(fmt.Sprintf("xl/worksheets/s%d.xml", n+1), ws.String())
	}
	wb.WriteString(`</sheets></workbook>`)
	rels.WriteString(`</Relationships>`)
	add("xl/workbook.xml", wb.String())
	add("xl/_rels/workbook.xml.rels", rels.String())
	var sst strings.Builder
	sst.WriteString(`<?xml version="1.0" encoding="UTF-8"?><sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	for _, s := range shared {
		fmt.Fprintf(&sst, `<si><t>%s</t></si>`, s)
	}
	sst.WriteString(`</sst>`)
	add("xl/sharedStrings.xml", sst.String())
	zw.Close()
	return buf.Bytes()
}

func TestXLSXInventoryImport(t *testing.T) {
	srv := newTestServer(t)
	u := newTestUser(t, srv, "inv-user", false)
	var cr credential
	u.jsonDo("POST", "/api/credentials", map[string]interface{}{"name": "root@zg1", "username": "root", "password": "pw-123456"}, &cr)
	data := buildXLSX(t, map[string][][]string{
		"Notes": {{"nothing here"}},
		"Servers": {
			{"Hostname", "IP", "Port", "Platform", "Site", "Rack", "Environment", "Tags", "Jump host"},
			{"web-01", "10.20.0.11", "", "Ubuntu 22.04", "zg1", "A12", "prod", "web, nginx", ""},
			{"win-dc01", "10.20.0.20", "", "Windows Server 2022", "zg1", "A13", "prod", "ad", ""},
			{"db-01", "~10.20.0.30", "#2222", "linux", "st1", "", "test", "", "web-01"},
			{"v6-host", "fd00::30", "", "", "st1", "", "", "", ""},
			{"idrac-web-01", "https://10.30.0.11/restgui", "", "", "zg1", "A12", "", "bmc", ""},
			{"", "", "", "", "", "", "", "", ""},
		},
	}, []string{"Notes", "Servers"})
	b64 := base64.StdEncoding.EncodeToString(data)
	var pr struct {
		Sheets  []string       `json:"sheets"`
		Sheet   string         `json:"sheet"`
		Headers []string       `json:"headers"`
		Rows    [][]string     `json:"rows"`
		Total   int            `json:"total"`
		Mapping map[string]int `json:"mapping"`
	}
	if code := u.jsonDo("POST", "/api/inventory/parse", map[string]interface{}{"file_name": "dc.xlsx", "data": b64}, &pr); code != 200 || pr.Sheet != "Notes" || len(pr.Sheets) != 2 {
		t.Fatalf("parse first sheet: %d %+v", code, pr)
	}
	if code := u.jsonDo("POST", "/api/inventory/parse", map[string]interface{}{"file_name": "dc.xlsx", "data": b64, "sheet": "Servers"}, &pr); code != 200 || pr.Total != 6 || pr.Rows[2][2] != "2222" || pr.Rows[2][1] != "10.20.0.30" {
		t.Fatalf("parse: %d %+v", code, pr)
	}
	want := map[string]int{"name": 0, "host": 1, "port": 2, "platform": 3, "site": 4, "rack": 5, "env": 6, "tags": 7, "jump": 8}
	for f, i := range want {
		if pr.Mapping[f] != i {
			t.Fatalf("mapping %s = %d, want %d (%v)", f, pr.Mapping[f], i, pr.Mapping)
		}
	}
	mapping := pr.Mapping
	var res importResult
	body := map[string]interface{}{"file_name": "dc.xlsx", "data": b64, "sheet": "Servers", "mapping": mapping,
		"options": map[string]interface{}{"folder_mode": "site", "tags": "imported", "auth": "credential", "credential_id": cr.ID}}
	if code := u.jsonDo("POST", "/api/inventory/import", body, &res); code != 200 || res.Imported != 5 || res.Folders != 2 || res.Jumps != 1 {
		t.Fatalf("import: %d %+v", code, res)
	}
	var list []connView
	u.jsonDo("GET", "/api/connections", nil, &list)
	by := map[string]connView{}
	for _, c := range list {
		by[c.Name] = c
	}
	if c := by["web-01"]; c.Protocol != "SSH" || c.AuthMethod != "CREDENTIAL" || c.CredName != "root@zg1" || strings.Join(c.Tags, ",") != "imported,web,nginx,env:prod,site:zg1,rack:a12,platform:ubuntu-22.04" {
		t.Fatalf("web-01: %+v", c)
	}
	if c := by["win-dc01"]; c.Protocol != "RDP" || c.AuthMethod != "CREDENTIAL" {
		t.Fatalf("windows → RDP: %+v", c)
	}
	if c := by["db-01"]; c.Host != "10.20.0.30:2222" || c.JumpID == nil || *c.JumpID != by["web-01"].ID {
		t.Fatalf("db-01: %+v", c)
	}
	if c := by["v6-host"]; c.Host != "[fd00::30]:22" {
		t.Fatalf("ipv6: %+v", c)
	}
	if c := by["idrac-web-01"]; c.Protocol != "HTTPS" || c.Host != "10.30.0.11" || c.WebPath != "/restgui" {
		t.Fatalf("url: %+v", c)
	}
	var folders []Folder
	u.jsonDo("GET", "/api/folders", nil, &folders)
	if len(folders) != 2 {
		t.Fatalf("folders: %+v", folders)
	}
	// importing the same file again creates nothing
	if u.jsonDo("POST", "/api/inventory/import", body, &res); res.Imported != 0 || len(res.Skipped) != 5 {
		t.Fatalf("re-import: %+v", res)
	}
	// a credential of somebody else cannot be used
	stranger := newTestUser(t, srv, "inv-stranger", false)
	if code := stranger.jsonDo("POST", "/api/inventory/import", body, &map[string]interface{}{}); code != 400 {
		t.Fatalf("foreign credential: %d", code)
	}
}

type fakeNetbox struct {
	mu      sync.Mutex
	devices []map[string]interface{}
	vms     []map[string]interface{}
	queries []string
}

func (f *fakeNetbox) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token nb-secret" {
			w.WriteHeader(403)
			w.Write([]byte(`{"detail":"Invalid token"}`))
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.queries = append(f.queries, r.URL.RawQuery)
		var all []map[string]interface{}
		switch r.URL.Path {
		case "/api/dcim/devices/":
			all = f.devices
		case "/api/virtualization/virtual-machines/":
			all = f.vms
		default:
			w.WriteHeader(404)
			return
		}
		// two objects per page, like NetBox with ?limit
		off := 0
		fmt.Sscanf(r.URL.Query().Get("offset"), "%d", &off)
		end := off + 2
		if end > len(all) {
			end = len(all)
		}
		page := map[string]interface{}{"count": len(all), "results": all[off:end], "next": nil}
		if end < len(all) {
			q := r.URL.Query()
			q.Set("offset", fmt.Sprint(end))
			page["next"] = "http://" + r.Host + r.URL.Path + "?" + q.Encode()
		}
		json.NewEncoder(w).Encode(page)
	}
}

func nbDevice(id int, name, ip, site, rack, role, platform string, tags ...string) map[string]interface{} {
	d := map[string]interface{}{"id": id, "name": name, "display": name, "site": map[string]string{"name": strings.ToUpper(site), "slug": site},
		"rack": nil, "role": map[string]string{"name": role, "slug": role}, "tenant": nil, "status": map[string]string{"value": "active", "label": "Active"},
		"custom_fields": map[string]interface{}{"environment": "prod"}}
	if ip != "" {
		d["primary_ip4"] = map[string]string{"address": ip + "/24"}
	}
	if rack != "" {
		d["rack"] = map[string]string{"name": rack}
	}
	if platform != "" {
		d["platform"] = map[string]string{"name": platform, "slug": strings.ToLower(strings.ReplaceAll(platform, " ", "-"))}
	}
	tl := []map[string]string{}
	for _, t := range tags {
		tl = append(tl, map[string]string{"name": t, "slug": t})
	}
	d["tags"] = tl
	return d
}

func TestNetboxImportAndSync(t *testing.T) {
	nb := &fakeNetbox{
		devices: []map[string]interface{}{
			nbDevice(1, "sw-core-1", "10.0.0.1", "zg1", "A01", "switch", "", "network"),
			nbDevice(2, "web-01", "10.0.1.11", "zg1", "A12", "server", "Ubuntu"),
			nbDevice(3, "dc-01", "10.0.1.20", "st1", "B02", "server", "Windows Server"),
			nbDevice(4, "pdu-1", "", "st1", "B02", "pdu", ""),
		},
		vms: []map[string]interface{}{nbDevice(10, "app-vm", "10.0.2.5", "zg1", "", "vm", "Debian")},
	}
	nbSrv := httptest.NewServer(nb.handler(t))
	defer nbSrv.Close()
	srv := newTestServer(t)
	u := newTestUser(t, srv, "nb-user", false)
	q := map[string]interface{}{"url": nbSrv.URL, "token": "wrong", "devices": true, "vms": true, "env_field": "environment"}
	var e map[string]interface{}
	if code := u.jsonDo("POST", "/api/inventory/netbox/preview", map[string]interface{}{"query": q}, &e); code != 502 || !strings.Contains(fmt.Sprint(e["error"]), "token") {
		t.Fatalf("wrong token: %d %v", code, e)
	}
	q["token"], q["remember"], q["site"] = "nb-secret", true, "zg1, st1"
	var pv struct {
		Rows    []netboxRow              `json:"rows"`
		Missing []map[string]interface{} `json:"missing"`
	}
	if code := u.jsonDo("POST", "/api/inventory/netbox/preview", map[string]interface{}{"query": q}, &pv); code != 200 || len(pv.Rows) != 5 {
		t.Fatalf("preview: %d %+v", code, pv)
	}
	if !strings.Contains(nb.queries[0], "site=zg1") || !strings.Contains(nb.queries[0], "site=st1") || !strings.Contains(nb.queries[0], "status=active") {
		t.Fatalf("filters not sent: %v", nb.queries)
	}
	rowsByName := map[string]netboxRow{}
	for _, r := range pv.Rows {
		rowsByName[r.Name] = r
	}
	if r := rowsByName["dc-01"]; r.Protocol != "RDP" || r.Env != "prod" || r.Rack != "B02" {
		t.Fatalf("dc-01 row: %+v", r)
	}
	if r := rowsByName["pdu-1"]; r.Host != "" || r.Problem == "" {
		t.Fatalf("no IP: %+v", r)
	}
	if r := rowsByName["app-vm"]; r.Kind != "vm" || !strings.Contains(r.ExtID, ":vm:10") {
		t.Fatalf("vm row: %+v", r)
	}
	// the token is remembered, never returned
	var src map[string]interface{}
	u.jsonDo("GET", "/api/inventory/netbox", nil, &src)
	if src["has_token"] != true || strings.Contains(fmt.Sprint(src), "nb-secret") {
		t.Fatalf("saved source: %v", src)
	}
	var raw string
	db.QueryRow(`SELECT token FROM inventory_sources WHERE user_id=?`, u.userID).Scan(&raw)
	if raw == "" || strings.Contains(raw, "nb-secret") {
		t.Fatalf("token not encrypted")
	}
	var res importResult
	opts := map[string]interface{}{"folder_mode": "site", "username": "admin"}
	if code := u.jsonDo("POST", "/api/inventory/netbox/import", map[string]interface{}{"query": map[string]interface{}{"url": nbSrv.URL}, "rows": pv.Rows, "options": opts}, &res); code != 200 || res.Imported != 4 || len(res.Skipped) != 1 {
		t.Fatalf("import: %d %+v", code, res)
	}
	var list []connView
	u.jsonDo("GET", "/api/connections", nil, &list)
	by := map[string]connView{}
	for _, c := range list {
		by[c.Name] = c
	}
	if c := by["web-01"]; c.Host != "10.0.1.11" || c.Username != "admin" || c.Source != "netbox" || strings.Join(c.Tags, ",") != "site:zg1,rack:a12,role:server,platform:ubuntu,env:prod" {
		t.Fatalf("web-01: %+v", c)
	}
	// the user adds a tag of their own; NetBox changes; sync
	u.jsonDo("POST", "/api/connections/bulk", map[string]interface{}{"action": "tag", "ids": []int{by["web-01"].ID}, "add": []string{"mine"}}, nil)
	nb.mu.Lock()
	nb.devices[1] = nbDevice(2, "web-01", "10.0.1.99", "zg1", "A14", "server", "Ubuntu")
	nb.devices = append(nb.devices[:0], nb.devices[1:]...) // sw-core-1 is gone
	nb.mu.Unlock()
	if code := u.jsonDo("POST", "/api/inventory/netbox/preview", map[string]interface{}{"query": map[string]interface{}{"url": nbSrv.URL, "devices": true, "vms": true, "env_field": "environment"}}, &pv); code != 200 {
		t.Fatalf("preview with the saved token: %d", code)
	}
	if len(pv.Missing) != 1 || pv.Missing[0]["name"] != "sw-core-1" {
		t.Fatalf("missing: %+v", pv.Missing)
	}
	for _, r := range pv.Rows {
		if r.Name == "web-01" && r.ConnID != by["web-01"].ID {
			t.Fatalf("existing not recognised: %+v", r)
		}
	}
	if u.jsonDo("POST", "/api/inventory/netbox/import", map[string]interface{}{"query": map[string]interface{}{"url": nbSrv.URL}, "rows": pv.Rows, "options": opts}, &res); res.Imported != 0 || res.Updated != 0 {
		t.Fatalf("without update: %+v", res)
	}
	opts["update"] = true
	if u.jsonDo("POST", "/api/inventory/netbox/import", map[string]interface{}{"query": map[string]interface{}{"url": nbSrv.URL}, "rows": pv.Rows, "options": opts}, &res); res.Updated != 3 || res.Imported != 0 {
		t.Fatalf("sync: %+v", res)
	}
	var c connView
	u.jsonDo("GET", fmt.Sprintf("/api/connections/%d", by["web-01"].ID), nil, &c)
	if c.Host != "10.0.1.99" || strings.Join(c.Tags, ",") != "mine,site:zg1,rack:a14,role:server,platform:ubuntu,env:prod" {
		t.Fatalf("after sync: %+v", c)
	}
	// rows of another NetBox are refused
	fake := []netboxRow{{ExtID: "netbox:evil.example:device:1", Name: "x", Host: "1.2.3.4"}}
	if u.jsonDo("POST", "/api/inventory/netbox/import", map[string]interface{}{"query": map[string]interface{}{"url": nbSrv.URL}, "rows": fake, "options": opts}, &res); res.Imported != 0 || len(res.Skipped) != 1 {
		t.Fatalf("foreign rows: %+v", res)
	}
}
