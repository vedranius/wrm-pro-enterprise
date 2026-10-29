// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// ─── FILE MANAGER ─────────────────────────────────────

// fileRequest validates that the request may use the connection referenced by the "id"
// query parameter for perm (PermFilesRead or PermFilesWrite).
func fileRequest(w http.ResponseWriter, r *http.Request, perm string) (Connection, *connAccess, bool) {
	connID, _ := strconv.Atoi(r.URL.Query().Get("id"))
	acc, code, msg := authorizeConnection(r, connID, perm)
	if acc == nil {
		jsonError(w, msg, code)
		return Connection{}, nil, false
	}
	c, err := loadConnection(connID)
	if err != nil {
		jsonError(w, "Connection not found", 404)
		return Connection{}, nil, false
	}
	if isWeb(c) {
		jsonError(w, "This is a web interface connection; it has no file manager", 400)
		return Connection{}, nil, false
	}
	if isDesktopProtocol(c.Protocol) || c.Protocol == "SERIAL" {
		jsonError(w, "This is a remote desktop or serial connection; it has no file manager", 400)
		return Connection{}, nil, false
	}
	return c, acc, true
}

// auditFileOp records a change made through the file manager.
func auditFileOp(r *http.Request, acc *connAccess, c Connection, action, target string, extra map[string]interface{}) {
	uid, name := acc.actor()
	d := map[string]interface{}{"connection": c.Name, "host": c.Host}
	if acc.Share != nil {
		d["share"] = acc.Share.Share.Name
	}
	for k, v := range extra {
		d[k] = v
	}
	auditLogRef(r, uid, name, action, target, d, auditRef{ConnID: c.ID})
}

func requireMethod(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, m := range methods {
		if r.Method == m {
			return true
		}
	}
	jsonError(w, "Method not allowed", 405)
	return false
}

// dialFTP connects and logs in; FTPS uses explicit TLS (AUTH TLS). With a jump host the
// control and data connections go through the jump host's SSH connection. Always call
// the returned close function (it also closes the jump host connection).
func dialFTP(c Connection) (*ftp.ServerConn, func(), error) {
	if err := guardLoopbackTunnel(c); err != nil {
		return nil, nil, err
	}
	opts := []ftp.DialOption{ftp.DialWithTimeout(15 * time.Second)}
	ftps := strings.ToUpper(c.Protocol) == "FTPS"
	tlsCfg := ftpsTLSConfig(c) // one config for the control and the data connections
	if ftps {
		opts = append(opts, ftp.DialWithExplicitTLS(tlsCfg))
	}
	var via *targetRoute
	if needsRoute(c) {
		// Control and data connections go through the jump hosts and / or the proxy.
		var err error
		if via, err = openTargetRoute(c); err != nil {
			return nil, nil, err
		}
		opts = append(opts, ftp.DialWithDialFunc(routedFTPDial(via, hostOnly(c.Host), ftps, tlsCfg)))
	}
	closeVia := func() { via.Close() }
	if c.authErr != "" {
		closeVia()
		return nil, nil, errors.New(c.authErr)
	}
	fc, err := ftp.Dial(c.Host, opts...)
	if err != nil {
		closeVia()
		return nil, nil, fmt.Errorf("FTP: %w", err)
	}
	if err := fc.Login(c.Username, c.Password); err != nil {
		fc.Quit()
		closeVia()
		return nil, nil, fmt.Errorf("FTP login: %v", err)
	}
	return fc, func() { fc.Quit(); closeVia() }, nil
}

// ftpRouteHost stands for the FTP server in the control connection's RemoteAddr when it
// goes through a route: the library takes the EPSV data host from there, and through a
// proxy or an SSH channel that address is not the server's (TEST-NET-1, never a target).
var ftpRouteHost = net.IPv4(192, 0, 2, 255)

type ftpAddrConn struct {
	net.Conn
	raddr net.Addr
}

func (a *ftpAddrConn) RemoteAddr() net.Addr { return a.raddr }

// routedFTPDial dials the control and the data connections of an FTP session over a route.
// With a dial function the library neither knows the server's address nor adds TLS to
// the data connections, so this does both.
func routedFTPDial(via *targetRoute, server string, ftps bool, cfg *tls.Config) func(network, address string) (net.Conn, error) {
	var ctrlDone atomic.Bool
	return func(network, address string) (net.Conn, error) {
		if !ctrlDone.Swap(true) { // the control connection (the library adds TLS after AUTH TLS)
			conn, err := via.Dial("tcp", address)
			if err != nil {
				return nil, err
			}
			return &ftpAddrConn{Conn: conn, raddr: &net.TCPAddr{IP: ftpRouteHost}}, nil
		}
		if h, p, err := net.SplitHostPort(address); err == nil && net.ParseIP(h).Equal(ftpRouteHost) {
			address = net.JoinHostPort(server, p) // EPSV: the data port on the FTP server
		}
		conn, err := via.Dial("tcp", address)
		if err != nil {
			return nil, err
		}
		if ftps {
			return tls.Client(conn, cfg), nil // PROT P: the data connections are TLS too
		}
		return conn, nil
	}
}

func withFTP(c Connection, fn func(*ftp.ServerConn) error) error {
	fc, done, err := dialFTP(c)
	if err != nil {
		return err
	}
	defer done()
	return fn(fc)
}

// fileOp runs fn against either an FTP or an SFTP connection.
func fileOp(c Connection, retry bool, sftpFn func(*sftp.Client) error, ftpFn func(*ftp.ServerConn) error) error {
	if isFTP(c) {
		return withFTP(c, ftpFn)
	}
	return withSFTP(c, retry, sftpFn)
}

func listRemoteFilesHandler(w http.ResponseWriter, r *http.Request) {
	c, _, ok := fileRequest(w, r, PermFilesRead)
	if !ok {
		return
	}
	p := r.URL.Query().Get("path")
	if p == "" {
		p = "."
	}
	list := make([]FileItem, 0)
	err := fileOp(c, true, func(sc *sftp.Client) error {
		list = list[:0]
		if p == "." || p == "~" || strings.HasPrefix(p, "~/") {
			rel := "" // a directory below the home directory (a bookmark such as ~/logs)
			if strings.HasPrefix(p, "~/") {
				rel = strings.TrimLeft(p[2:], "/")
			}
			if rp, err := sc.RealPath("."); err == nil {
				p = rp
			} else if wd, err := sc.Getwd(); err == nil {
				p = wd
			} else {
				p = "."
			}
			if rel != "" {
				p = joinRemote(p, rel)
			}
		}
		if rp, err := sc.RealPath(p); err == nil {
			p = rp // resolves "..", "." and duplicate slashes
		}
		files, err := sc.ReadDir(p)
		if err != nil {
			return fmt.Errorf("cannot read %s: %v", p, err)
		}
		for _, f := range files {
			item := FileItem{Name: f.Name(), IsDir: f.IsDir(), Size: f.Size(), Mode: f.Mode().String(), ModTime: f.ModTime().Unix()}
			if f.Mode()&os.ModeSymlink != 0 {
				item.IsLink = true
				if st, err := sc.Stat(joinRemote(p, f.Name())); err == nil {
					item.IsDir = st.IsDir()
					if !st.IsDir() {
						item.Size = st.Size()
					}
				}
			}
			list = append(list, item)
		}
		return nil
	}, func(fc *ftp.ServerConn) error {
		if p == "~" || strings.HasPrefix(p, "~/") {
			p = strings.TrimLeft(p[1:], "/") // the login directory, or a directory below it
			if p == "" {
				p = "."
			}
		}
		if p != "." && p != "" {
			if err := fc.ChangeDir(p); err != nil {
				return fmt.Errorf("cannot open %s: %v", p, err)
			}
		}
		p, _ = fc.CurrentDir()
		entries, err := fc.List(p)
		if err != nil {
			return fmt.Errorf("cannot list %s: %v", p, err)
		}
		for _, e := range entries {
			if e.Name == "." || e.Name == ".." {
				continue
			}
			list = append(list, FileItem{Name: e.Name, IsDir: e.Type == ftp.EntryTypeFolder, IsLink: e.Type == ftp.EntryTypeLink, Size: int64(e.Size), ModTime: e.Time.Unix()})
		}
		return nil
	})
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]interface{}{"path": p, "files": list})
}

func listRemoteRecursiveHandler(w http.ResponseWriter, r *http.Request) {
	c, _, ok := fileRequest(w, r, PermFilesRead)
	if !ok {
		return
	}
	p := r.URL.Query().Get("path")
	if p == "" {
		p = "."
	}
	if isFTP(c) {
		jsonError(w, "Not supported for FTP", 400)
		return
	}
	type recursiveFile struct {
		Path string `json:"path"`
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	const maxFiles = 200000
	var files []recursiveFile
	notFound := false
	err := withSFTP(c, true, func(sc *sftp.Client) error {
		files = files[:0]
		info, err := sc.Stat(p)
		if err != nil {
			notFound = true
			return fmt.Errorf("path not found: %v", err)
		}
		if !info.IsDir() {
			files = append(files, recursiveFile{Path: p, Name: path.Base(p), Size: info.Size()})
			return nil
		}
		var walk func(dir string) error
		walk = func(dir string) error {
			entries, err := sc.ReadDir(dir)
			if err != nil {
				if isConnLostErr(err) {
					return err
				}
				return nil
			}
			for _, e := range entries {
				if len(files) >= maxFiles || r.Context().Err() != nil {
					return nil
				}
				full := joinRemote(dir, e.Name())
				if e.IsDir() {
					if err := walk(full); err != nil {
						return err
					}
				} else {
					files = append(files, recursiveFile{Path: full, Name: e.Name(), Size: e.Size()})
				}
			}
			return nil
		}
		return walk(p)
	})
	if err != nil {
		code := 500
		if notFound {
			code = 404
		}
		jsonError(w, err.Error(), code)
		return
	}
	var totalSize int64
	for _, f := range files {
		totalSize += f.Size
	}
	jsonOK(w, map[string]interface{}{"files": files, "count": len(files), "totalSize": totalSize})
}

// errStreamStarted wraps errors that happen after bytes were sent to the browser, so the
// pool never retries (which would corrupt the download).
type errStreamStarted struct{ err error }

func (e errStreamStarted) Error() string { return e.err.Error() }

func downloadRemoteFileHandler(w http.ResponseWriter, r *http.Request) {
	c, acc, ok := fileRequest(w, r, PermFilesRead)
	if !ok {
		return
	}
	remotePath := r.URL.Query().Get("path")
	inline := r.URL.Query().Get("inline") == "1"
	disposition := attachmentHeader(path.Base(remotePath))
	if inline {
		disposition = "inline"
	}
	started := false
	var copyErr error
	hw := newHashingWriter(w)
	defer func() {
		if !started {
			return
		}
		st, msg := "ok", ""
		if copyErr != nil || r.Context().Err() != nil {
			st, msg = "failed", "download interrupted"
		}
		logFileTransfer(r, acc, transferRec{Direction: "download", SrcConn: &c, SrcPath: remotePath, Size: hw.n, SHA256: hw.Sum(), Status: st, Error: msg})
		action := "file.download"
		if inline {
			action = "file.open"
		}
		auditFileOp(r, acc, c, action, remotePath, map[string]interface{}{"bytes": hw.n, "sha256": hw.Sum(), "status": st})
	}()
	err := fileOp(c, true, func(sc *sftp.Client) error {
		info, err := sc.Stat(remotePath)
		if err != nil {
			return fmt.Errorf("file not found: %v", err)
		}
		if info.IsDir() {
			return errors.New("cannot download a directory (use ZIP download)")
		}
		file, err := sc.Open(remotePath)
		if err != nil {
			return err
		}
		defer file.Close()
		w.Header().Set("Content-Disposition", disposition)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
		started = true
		if _, err := io.Copy(hw, io.LimitReader(file, info.Size())); err != nil {
			copyErr = err
			return errStreamStarted{err}
		}
		return nil
	}, func(fc *ftp.ServerConn) error {
		if size, err := fc.FileSize(remotePath); err == nil && size >= 0 {
			w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		}
		resp, err := fc.Retr(remotePath)
		if err != nil {
			w.Header().Del("Content-Length")
			return err
		}
		defer resp.Close()
		w.Header().Set("Content-Disposition", disposition)
		w.Header().Set("Content-Type", "application/octet-stream")
		started = true
		if _, err = io.Copy(hw, resp); err != nil {
			copyErr = err
		}
		return err
	})
	if err != nil {
		if started {
			log.Printf("download %s aborted: %v", remotePath, err)
			return
		}
		jsonError(w, err.Error(), 500)
	}
}

func downloadRemoteDirHandler(w http.ResponseWriter, r *http.Request) {
	c, acc, ok := fileRequest(w, r, PermFilesRead)
	if !ok {
		return
	}
	if isFTP(c) {
		jsonError(w, "ZIP download is not supported for FTP", 400)
		return
	}
	remotePath := r.URL.Query().Get("path")
	dirName := path.Base(strings.TrimSuffix(remotePath, "/"))
	if dirName == "" || dirName == "/" || dirName == "." {
		dirName = "root"
	}
	started := false
	err := withSFTP(c, true, func(sc *sftp.Client) error {
		info, err := sc.Stat(remotePath)
		if err != nil {
			return fmt.Errorf("folder not found: %v", err)
		}
		if !info.IsDir() {
			return errors.New("not a directory")
		}
		w.Header().Set("Content-Disposition", attachmentHeader(dirName+".zip"))
		w.Header().Set("Content-Type", "application/zip")
		started = true
		zw := zip.NewWriter(w)
		st := &zipStats{acc: acc, conn: &c}
		err = zipRemoteDir(r, sc, zw, remotePath, dirName, st)
		auditFileOp(r, acc, c, "file.download_zip", remotePath, map[string]interface{}{"files": st.files, "bytes": st.bytes, "complete": err == nil})
		if err != nil {
			zw.Close()
			return errStreamStarted{err}
		}
		return zw.Close()
	})
	if err != nil {
		if started {
			log.Printf("zip %s aborted: %v", remotePath, err)
			return
		}
		jsonError(w, err.Error(), 500)
	}
}

// zipStats records each file put into a ZIP download in the transfer log.
type zipStats struct {
	acc   *connAccess
	conn  *Connection
	files int
	bytes int64
}

func zipRemoteDir(r *http.Request, sc *sftp.Client, zw *zip.Writer, remotePath, baseName string, st *zipStats) error {
	entries, err := sc.ReadDir(remotePath)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		zw.Create(baseName + "/")
	}
	for _, entry := range entries {
		if err := r.Context().Err(); err != nil {
			return err
		}
		fullRemote := joinRemote(remotePath, entry.Name())
		zipPath := baseName + "/" + entry.Name()
		if entry.IsDir() {
			if err := zipRemoteDir(r, sc, zw, fullRemote, zipPath, st); err != nil {
				if r.Context().Err() != nil || isConnLostErr(err) {
					return err
				}
				log.Printf("zip subdir %s: %v", fullRemote, err)
			}
			continue
		}
		if !entry.Mode().IsRegular() {
			continue
		}
		f, err := sc.Open(fullRemote)
		if err != nil {
			continue
		}
		hdr := &zip.FileHeader{Name: zipPath, Method: zip.Deflate}
		hdr.Modified = entry.ModTime()
		fw, err := zw.CreateHeader(hdr)
		if err != nil {
			f.Close()
			return err
		}
		hw := newHashingWriter(fw)
		_, err = io.Copy(hw, f)
		f.Close()
		status, msg := "ok", ""
		if err != nil {
			status, msg = "failed", err.Error()
		}
		logFileTransfer(r, st.acc, transferRec{Direction: "download", SrcConn: st.conn, SrcPath: fullRemote, Size: hw.n, SHA256: hw.Sum(), Status: status, Error: msg})
		st.files++
		st.bytes += hw.n
		if err != nil {
			return err
		}
	}
	return nil
}

// cleanRelPath validates a client-supplied relative path (for folder uploads).
func cleanRelPath(rel string) (string, error) {
	rel = strings.ReplaceAll(rel, "\\", "/")
	rel = strings.Trim(rel, "/")
	if rel == "" {
		return "", nil
	}
	for _, part := range strings.Split(rel, "/") {
		if part == ".." || part == "." || part == "" {
			return "", fmt.Errorf("invalid relative path %q", rel)
		}
	}
	return rel, nil
}

type uploadResult struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | conflict | error
	Size   int64  `json:"size"`
	Error  string `json:"error,omitempty"`
}

// uploadRemoteFileHandler streams multipart uploads straight to the remote server
// (no buffering of the whole file in RAM). Parameters can come from the query string or
// from form fields that precede the file parts. A "relpath" field before a file part
// uploads that file into a sub-directory (folder upload / drag & drop of folders).
func uploadRemoteFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "Method not allowed", 405)
		return
	}
	mr, err := r.MultipartReader()
	if err != nil {
		jsonError(w, "Upload error: "+err.Error(), 400)
		return
	}
	q := r.URL.Query()
	params := map[string]string{"id": q.Get("id"), "path": q.Get("path"), "overwrite": q.Get("overwrite"), "share_token": q.Get("share_token")}
	var (
		c        Connection
		acc      *connAccess
		haveConn bool
		results  []uploadResult
		relDir   string
		editIDs  []int64
	)
	maxBytes := int64(settingInt("max_upload_mb")) << 20
	ensureConn := func() bool {
		if haveConn {
			return true
		}
		connID, _ := strconv.Atoi(params["id"])
		if params["share_token"] != "" && q.Get("share_token") == "" {
			qq := r.URL.Query()
			qq.Set("share_token", params["share_token"])
			r.URL.RawQuery = qq.Encode()
		}
		a, code, msg := authorizeConnection(r, connID, PermFilesWrite)
		if a == nil {
			jsonError(w, msg, code)
			return false
		}
		acc = a
		var err error
		if c, err = loadConnection(connID); err != nil {
			jsonError(w, "Connection not found", 404)
			return false
		}
		haveConn = true
		return true
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			if !haveConn {
				jsonError(w, "Upload error: "+err.Error(), 400)
				return
			}
			results = append(results, uploadResult{Status: "error", Error: err.Error()})
			break
		}
		name := part.FormName()
		if part.FileName() == "" {
			val, _ := io.ReadAll(io.LimitReader(part, 64*1024))
			if name == "relpath" {
				relDir = string(val)
			} else {
				params[name] = string(val)
			}
			continue
		}
		if !ensureConn() {
			return
		}
		fileName := path.Base(strings.ReplaceAll(part.FileName(), "\\", "/"))
		rel, err := cleanRelPath(relDir)
		relDir = ""
		res := uploadResult{Name: fileName}
		if err != nil {
			res.Status, res.Error = "error", err.Error()
			results = append(results, res)
			io.Copy(io.Discard, part)
			continue
		}
		baseDir := params["path"]
		if baseDir == "" {
			baseDir = "."
		}
		targetDir := baseDir
		if rel != "" {
			dir := path.Dir(rel)
			if dir != "." {
				targetDir = joinRemote(baseDir, dir)
			}
			res.Name = rel
		}
		fullPath := joinRemote(targetDir, fileName)
		overwrite := params["overwrite"] == "true" || params["overwrite"] == "1"
		// a save from the file editor keeps the diff of the change (file edit history, #129)
		edit := q.Get("edit") == "1" && settingBool("file_edit_history")
		var src io.Reader = part
		var tee *editTee
		var oldData []byte
		var oldBig, oldExists bool
		var oldSize int64
		if edit {
			tee = &editTee{max: fileEditMax()}
			src = io.TeeReader(part, tee)
		}
		cr := &countingReader{r: src, max: maxBytes, h: sha256.New()}
		opErr := fileOp(c, false, func(sc *sftp.Client) error {
			if targetDir != baseDir {
				if err := sc.MkdirAll(targetDir); err != nil {
					return fmt.Errorf("mkdir %s: %v", targetDir, err)
				}
			}
			if !overwrite {
				if _, err := sc.Stat(fullPath); err == nil {
					return errConflict
				}
			}
			if edit {
				if f, err := sc.Open(fullPath); err == nil {
					if st, err := f.Stat(); err == nil {
						oldSize = st.Size()
					}
					oldData, oldBig, _ = cappedRead(f, tee.max)
					oldExists = true
					f.Close()
				}
			}
			dst, err := sc.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
			if err != nil {
				return err
			}
			if _, err := dst.ReadFrom(cr); err != nil {
				dst.Close()
				if errors.Is(err, errTooLarge) {
					sc.Remove(fullPath)
				}
				return err
			}
			return dst.Close()
		}, func(fc *ftp.ServerConn) error {
			if targetDir != baseDir {
				ftpMkdirAll(fc, targetDir)
			}
			if !overwrite {
				if _, err := fc.FileSize(fullPath); err == nil {
					return errConflict
				}
			}
			if edit {
				if resp, err := fc.Retr(fullPath); err == nil {
					oldData, oldBig, _ = cappedRead(resp, tee.max)
					resp.Close()
					oldExists = true
					if n, err := fc.FileSize(fullPath); err == nil {
						oldSize = n
					}
				}
			}
			err := fc.Stor(fullPath, cr)
			if errors.Is(err, errTooLarge) || cr.tooBig {
				fc.Delete(fullPath)
				return errTooLarge
			}
			return err
		})
		res.Size = cr.n
		switch {
		case opErr == nil:
			res.Status = "ok"
			if edit {
				uid, name := acc.actor()
				rec := fileEditRec{UserID: uid, User: name, IP: clientIP(r), Source: "editor", Conn: c, Path: fullPath, Created: !oldExists,
					Old: oldData, OldBig: oldBig, OldSize: oldSize, New: tee.buf.Bytes(), NewBig: tee.big, NewSize: cr.n}
				if acc.Share != nil {
					rec.ShareID, rec.Session = acc.Share.Share.ID, "share: "+acc.Share.Share.Name
				}
				if id := recordFileEdit(rec); id > 0 {
					editIDs = append(editIDs, id)
				}
			}
		case errors.Is(opErr, errConflict):
			res.Status = "conflict"
		default:
			res.Status, res.Error = "error", opErr.Error()
		}
		if res.Status != "conflict" {
			st := map[string]string{"ok": "ok"}[res.Status]
			if st == "" {
				st = "failed"
			}
			logFileTransfer(r, acc, transferRec{Direction: "upload", DstConn: &c, DstPath: fullPath, Size: cr.n,
				SHA256: hex.EncodeToString(cr.h.Sum(nil)), Status: st, Error: res.Error})
		}
		io.Copy(io.Discard, part) // drain whatever was not consumed (conflict/error)
		results = append(results, res)
	}
	if !haveConn {
		jsonError(w, "No file received", 400)
		return
	}
	okFiles, bytes := 0, int64(0)
	for _, res := range results {
		if res.Status == "ok" {
			okFiles++
			bytes += res.Size
		}
	}
	if okFiles > 0 {
		d := map[string]interface{}{"files": okFiles, "bytes": bytes}
		if len(editIDs) > 0 {
			d["file_edits"] = editIDs
		}
		auditFileOp(r, acc, c, "file.upload", params["path"], d)
	}
	status := http.StatusOK
	allConflict := len(results) > 0
	for _, res := range results {
		if res.Status != "conflict" {
			allConflict = false
		}
	}
	if allConflict {
		status = http.StatusConflict
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := map[string]interface{}{"results": results}
	if len(results) == 1 && results[0].Status == "conflict" {
		body["conflict"] = results[0].Name
	}
	w.Write(jsonMarshal(body))
}

var errConflict = errors.New("file exists")
var errTooLarge = errors.New("file exceeds the maximum upload size set by the administrator")

// countingReader counts bytes and stops with errTooLarge after max bytes (0 = unlimited).
type countingReader struct {
	r      io.Reader
	n      int64
	max    int64
	tooBig bool
	h      hash.Hash // optional: checksum of everything read
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.h != nil && n > 0 {
		c.h.Write(p[:n])
	}
	if c.max > 0 && c.n > c.max {
		c.tooBig = true
		return n, errTooLarge
	}
	return n, err
}

func ftpMkdirAll(fc *ftp.ServerConn, dir string) {
	cur := ""
	if strings.HasPrefix(dir, "/") {
		cur = "/"
	}
	for _, part := range strings.Split(strings.Trim(dir, "/"), "/") {
		if part == "" {
			continue
		}
		if cur == "" || cur == "/" {
			cur += part
		} else {
			cur += "/" + part
		}
		fc.MakeDir(cur) // errors ignored: the directory usually already exists
	}
}

func mkdirRemoteHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	c, acc, ok := fileRequest(w, r, PermFilesWrite)
	if !ok {
		return
	}
	remotePath := r.URL.Query().Get("path")
	err := fileOp(c, true, func(sc *sftp.Client) error {
		return sc.MkdirAll(remotePath)
	}, func(fc *ftp.ServerConn) error {
		return fc.MakeDir(remotePath)
	})
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	auditFileOp(r, acc, c, "file.mkdir", remotePath, nil)
	jsonOK(w, map[string]interface{}{"ok": true, "status": "ok"})
}

func remotePermissionErr(err error) bool {
	if err == nil {
		return false
	}
	e := strings.ToLower(err.Error())
	return strings.Contains(e, "permission denied") || strings.Contains(e, "operation not permitted") || strings.Contains(e, "access denied")
}

func chmodRemoteHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	c, acc, ok := fileRequest(w, r, PermFilesWrite)
	if !ok {
		return
	}
	var in struct {
		Path         string `json:"path"`
		Mode         string `json:"mode"`
		Recursive    bool   `json:"recursive"`
		Sudo         bool   `json:"sudo"`
		SudoPassword string `json:"sudo_password"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		jsonError(w, "Bad JSON", 400)
		return
	}
	in.Path = strings.TrimSpace(in.Path)
	if in.Path == "" || in.Path == "." || in.Path == ".." || strings.Trim(in.Path, "/") == "" || strings.ContainsRune(in.Path, 0) {
		jsonError(w, "Invalid path", 400)
		return
	}
	if len(in.Mode) < 3 || len(in.Mode) > 4 {
		jsonError(w, "Mode must be three or four octal digits", 400)
		return
	}
	for _, ch := range in.Mode {
		if ch < '0' || ch > '7' {
			jsonError(w, "Mode must be three or four octal digits", 400)
			return
		}
	}
	mode, _ := strconv.ParseUint(in.Mode, 8, 32)
	perm := os.FileMode(mode & 0o777)
	if mode&0o4000 != 0 {
		perm |= os.ModeSetuid
	}
	if mode&0o2000 != 0 {
		perm |= os.ModeSetgid
	}
	if mode&0o1000 != 0 {
		perm |= os.ModeSticky
	}
	useSudo := in.Sudo || in.SudoPassword != ""
	if !useSudo && in.Recursive {
		if isFTP(c) {
			jsonError(w, "Recursive permissions need an SSH/SFTP connection", 400)
			return
		}
		cmd := "chmod -R " + in.Mode + " -- " + shellQuote(in.Path)
		err := withSSHClient(c, func(cl *ssh.Client) error {
			_, e := runRemoteScript(cl, cmd, "", 60*time.Second)
			return e
		})
		if err != nil {
			if remotePermissionErr(err) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error(), "need_sudo": true})
				return
			}
			jsonError(w, err.Error(), 500)
			return
		}
		auditFileOp(r, acc, c, "file.chmod", in.Path, map[string]interface{}{"mode": in.Mode, "recursive": true, "sudo": false})
		jsonOK(w, map[string]interface{}{"ok": true, "status": "ok"})
		return
	}
	if useSudo {
		if isFTP(c) {
			jsonError(w, "sudo is only available on SSH/SFTP connections", 400)
			return
		}
		flag := ""
		if in.Recursive {
			flag = "-R "
		}
		cmd := "sudo -S -p '' chmod " + flag + in.Mode + " -- " + shellQuote(in.Path)
		stdin := in.SudoPassword + "\n"
		if in.SudoPassword == "" {
			cmd = "sudo -n -p '' chmod " + flag + in.Mode + " -- " + shellQuote(in.Path)
			stdin = ""
		}
		err := withSSHClient(c, func(cl *ssh.Client) error {
			_, e := runRemoteScript(cl, cmd, stdin, 60*time.Second)
			return e
		})
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		auditFileOp(r, acc, c, "file.chmod", in.Path, map[string]interface{}{"mode": in.Mode, "recursive": in.Recursive, "sudo": true})
		jsonOK(w, map[string]interface{}{"ok": true, "status": "ok"})
		return
	}
	err := fileOp(c, true, func(sc *sftp.Client) error {
		return sc.Chmod(in.Path, perm)
	}, func(fc *ftp.ServerConn) error {
		return errors.New("changing permissions is not supported on FTP connections")
	})
	if err != nil {
		if remotePermissionErr(err) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]interface{}{"error": err.Error(), "need_sudo": true})
			return
		}
		jsonError(w, err.Error(), 500)
		return
	}
	auditFileOp(r, acc, c, "file.chmod", in.Path, map[string]interface{}{"mode": in.Mode, "recursive": in.Recursive, "sudo": false})
	jsonOK(w, map[string]interface{}{"ok": true, "status": "ok"})
}

func deleteRemoteHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodDelete, http.MethodPost) {
		return
	}
	c, acc, ok := fileRequest(w, r, PermFilesWrite)
	if !ok {
		return
	}
	remotePath := r.URL.Query().Get("path")
	isDir := r.URL.Query().Get("dir") == "1"
	if p := strings.TrimRight(remotePath, "/"); p == "" || p == "." || p == ".." {
		jsonError(w, "Refusing to delete this path", 400)
		return
	}
	err := fileOp(c, true, func(sc *sftp.Client) error {
		if isDir {
			return removeRemoteDirSFTP(sc, remotePath)
		}
		return sc.Remove(remotePath)
	}, func(fc *ftp.ServerConn) error {
		if isDir {
			return fc.RemoveDirRecur(remotePath)
		}
		return fc.Delete(remotePath)
	})
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	auditFileOp(r, acc, c, "file.delete", remotePath, map[string]interface{}{"dir": isDir})
	jsonOK(w, map[string]interface{}{"ok": true, "status": "ok"})
}

func renameRemoteHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	oldPath := r.URL.Query().Get("old")
	newPath := r.URL.Query().Get("new")
	if oldPath == "" || newPath == "" {
		jsonError(w, "Missing old/new path", 400)
		return
	}
	c, acc, ok := fileRequest(w, r, PermFilesWrite)
	if !ok {
		return
	}
	err := fileOp(c, true, func(sc *sftp.Client) error {
		if _, err := sc.Lstat(newPath); err == nil {
			return fmt.Errorf("%s already exists", path.Base(newPath))
		}
		if err := sc.PosixRename(oldPath, newPath); err != nil {
			return sc.Rename(oldPath, newPath)
		}
		return nil
	}, func(fc *ftp.ServerConn) error {
		return fc.Rename(oldPath, newPath)
	})
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	auditFileOp(r, acc, c, "file.rename", oldPath, map[string]interface{}{"to": newPath})
	jsonOK(w, map[string]interface{}{"ok": true, "status": "ok"})
}

func removeRemoteDirSFTP(sc *sftp.Client, p string) error {
	entries, err := sc.ReadDir(p)
	if err != nil {
		return err
	}
	for _, e := range entries {
		full := joinRemote(p, e.Name())
		if e.IsDir() {
			if err := removeRemoteDirSFTP(sc, full); err != nil {
				return err
			}
		} else if err := sc.Remove(full); err != nil {
			return err
		}
	}
	return sc.RemoveDirectory(p)
}
