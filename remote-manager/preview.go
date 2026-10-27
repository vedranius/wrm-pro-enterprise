// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/jlaffaye/ftp"
	"github.com/pkg/sftp"
)

const (
	remotePreviewTextMax   = 256 << 10
	remotePreviewSampleMax = 8 << 10
)

var errPreviewNotText = errors.New("file has no text preview")

type remotePreviewInfo struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	MIME      string `json:"mime"`
	Lang      string `json:"lang,omitempty"`
	Size      int64  `json:"size"`
	Text      string `json:"text,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// Known binary files keep the old 415 answer even when their first bytes happen
// to look like text. Everything else is a text candidate and is checked by
// content, so scripts, configs and extensionless documents need no allowlist.
var remotePreviewBinaryExtensions = map[string]bool{
	".7z": true, ".a": true, ".aab": true, ".accdb": true, ".apk": true, ".appimage": true,
	".appx": true, ".appxbundle": true, ".avro": true, ".azw": true, ".azw3": true,
	".bin": true, ".bz2": true, ".cab": true, ".class": true, ".core": true, ".cpl": true,
	".crash": true, ".crdownload": true, ".db": true, ".deb": true, ".dex": true,
	".dll": true, ".dmg": true, ".doc": true, ".docx": true, ".download": true,
	".drv": true, ".dylib": true, ".ear": true, ".eot": true, ".epub": true, ".exe": true,
	".feather": true, ".flatpak": true, ".gif": true, ".gz": true, ".h5": true, ".hdf5": true,
	".icns": true, ".ico": true, ".img": true, ".ipa": true, ".iso": true, ".jar": true,
	".jks": true, ".jpg": true, ".jpeg": true, ".keystore": true, ".lz": true, ".lzma": true,
	".mdb": true, ".mobi": true, ".msi": true, ".msix": true, ".msixbundle": true,
	".msp": true, ".npy": true, ".npz": true, ".o": true, ".obj": true, ".ocx": true,
	".odp": true, ".ods": true, ".odt": true, ".orc": true, ".otf": true, ".ova": true,
	".parquet": true, ".p12": true, ".part": true, ".pdb": true, ".pdf": true,
	".pfx": true, ".pickle": true, ".pkl": true, ".png": true, ".ppt": true, ".pptx": true,
	".psd": true, ".pyc": true, ".pyd": true, ".pyo": true, ".qcow2": true, ".rar": true,
	".raw": true, ".rpm": true, ".scr": true, ".sketch": true, ".snap": true, ".so": true,
	".sqlite": true, ".sqlite3": true, ".sst": true, ".swf": true, ".swo": true, ".swp": true,
	".sys": true, ".tar": true, ".tgz": true, ".torrent": true, ".ttf": true, ".vhd": true,
	".vhdx": true, ".vmdk": true, ".wasm": true, ".war": true, ".woff": true, ".woff2": true,
	".xls": true, ".xlsx": true, ".xz": true, ".zip": true, ".zst": true,
	".mp4": true, ".m4v": true, ".webm": true, ".ogv": true, ".mov": true, ".mkv": true,
	".mp3": true, ".m4a": true, ".aac": true, ".ogg": true, ".oga": true, ".wav": true,
	".flac": true, ".webp": true, ".avif": true, ".bmp": true,
}

var remotePreviewLanguageByExt = map[string]string{
	".json": "json", ".jsonc": "json", ".json5": "json", ".jsonl": "json", ".ndjson": "json",
	".geojson": "json", ".har": "json", ".webmanifest": "json", ".ipynb": "json",
	".yaml": "yaml", ".yml": "yaml", ".toml": "toml",
	".ini": "ini", ".conf": "ini", ".cfg": "ini", ".cnf": "ini", ".properties": "properties",
	".service": "ini", ".socket": "ini", ".timer": "ini", ".target": "ini", ".mount": "ini",
	".automount": "ini", ".path": "ini", ".slice": "ini", ".scope": "ini", ".swap": "ini",
	".device": "ini", ".nspawn": "ini", ".desktop": "ini", ".directory": "ini", ".reg": "ini",
	".xml": "markup", ".xsd": "markup", ".xsl": "markup", ".xslt": "markup", ".svg": "markup",
	".smil": "markup", ".smi": "markup", ".html": "markup", ".htm": "markup", ".xhtml": "markup",
	".rss": "markup", ".atom": "markup", ".plist": "markup", ".pom": "markup", ".wsdl": "markup",
	".dtd": "markup", ".xaml": "markup", ".csproj": "markup", ".vbproj": "markup",
	".fsproj": "markup", ".props": "markup", ".targets": "markup", ".nuspec": "markup",
	".storyboard": "markup", ".xib": "markup", ".resx": "markup", ".kml": "markup",
	".gpx": "markup", ".opml": "markup",
	".sh": "bash", ".bash": "bash", ".zsh": "bash", ".ksh": "bash", ".fish": "bash",
	".command": "bash", ".awk": "bash", ".sed": "bash",
	".py": "python", ".pyw": "python", ".pyi": "python", ".pyx": "python", ".pxd": "python",
	".js": "javascript", ".mjs": "javascript", ".cjs": "javascript", ".jsx": "javascript",
	".ts": "typescript", ".tsx": "typescript", ".mts": "typescript", ".cts": "typescript",
	".css": "css", ".scss": "scss", ".sass": "scss", ".less": "less",
	".go": "go", ".rs": "rust", ".rb": "ruby", ".rake": "ruby", ".gemspec": "ruby",
	".php": "php", ".phtml": "php", ".java": "java", ".kt": "java", ".kts": "java",
	".cs": "csharp", ".c": "c", ".h": "c", ".cc": "cpp", ".cpp": "cpp", ".cxx": "cpp",
	".hpp": "cpp", ".hh": "cpp", ".hxx": "cpp", ".m": "clike", ".mm": "clike",
	".swift": "clike", ".scala": "clike", ".groovy": "clike", ".gradle": "clike",
	".dart": "clike", ".proto": "clike", ".thrift": "clike", ".sol": "clike",
	".md": "markdown", ".markdown": "markdown", ".mdown": "markdown", ".mkd": "markdown",
	".mdx": "markdown", ".diff": "diff", ".patch": "diff",
	".ps1": "powershell", ".psm1": "powershell", ".psd1": "powershell", ".ps1xml": "powershell",
	".pssc": "powershell", ".bat": "batch", ".cmd": "batch", ".dockerfile": "docker",
	".mk": "makefile", ".make": "makefile", ".nginx": "nginx", ".apacheconf": "apacheconf",
	".htaccess": "apacheconf", ".sql": "sql", ".psql": "sql", ".plsql": "sql",
	".mysql": "sql", ".pgsql": "sql",
}

var remotePreviewLanguageByName = map[string]string{
	"dockerfile": "docker", "containerfile": "docker", "makefile": "makefile",
	"gnumakefile": "makefile", "nginx.conf": "nginx", "httpd.conf": "apacheconf",
	"apache2.conf": "apacheconf", ".htaccess": "apacheconf", "php.ini": "ini",
	"my.cnf": "ini", "redis.conf": "ini", "vagrantfile": "ruby", "gemfile": "ruby",
	"rakefile": "ruby", "podfile": "ruby", "brewfile": "ruby", "jenkinsfile": "clike",
	".bashrc": "bash", ".bash_profile": "bash", ".bash_login": "bash", ".bash_logout": "bash",
	".profile": "bash", ".zshrc": "bash", ".zprofile": "bash", ".zshenv": "bash",
	".zlogin": "bash", ".zlogout": "bash", ".kshrc": "bash", ".cshrc": "bash",
	".login": "bash", ".logout": "bash", ".xinitrc": "bash", ".xprofile": "bash",
	".tmux.conf": "ini", ".gitconfig": "ini", ".editorconfig": "ini", ".npmrc": "ini",
	".yarnrc": "ini", ".gitmodules": "ini",
}

// The MIME allowlist is independent of OS MIME databases. Active documents are shown
// as escaped text; SVG/HTML/JavaScript never execute in WRM's authenticated origin.
func remotePreviewType(name string) (string, string) {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".mp4", ".m4v":
		return "video", "video/mp4"
	case ".webm":
		return "video", "video/webm"
	case ".ogv":
		return "video", "video/ogg"
	case ".mov":
		return "video", "video/quicktime"
	case ".mkv":
		return "video", "video/x-matroska"
	case ".mp3":
		return "audio", "audio/mpeg"
	case ".m4a", ".aac":
		if ext == ".aac" {
			return "audio", "audio/aac"
		}
		return "audio", "audio/mp4"
	case ".ogg", ".oga":
		return "audio", "audio/ogg"
	case ".wav":
		return "audio", "audio/wav"
	case ".flac":
		return "audio", "audio/flac"
	case ".png":
		return "image", "image/png"
	case ".jpg", ".jpeg":
		return "image", "image/jpeg"
	case ".gif":
		return "image", "image/gif"
	case ".webp":
		return "image", "image/webp"
	case ".avif":
		return "image", "image/avif"
	case ".bmp":
		return "image", "image/bmp"
	case ".pdf":
		return "pdf", "application/pdf"
	}
	if remotePreviewBinaryExtensions[ext] {
		return "", ""
	}
	return "sniff", "text/plain; charset=utf-8"
}

// remotePreviewIsText accepts UTF-8, UTF-16 with a BOM and single-byte text,
// while rejecting NUL bytes, common binary signatures and control-heavy data.
func remotePreviewIsText(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	if len(b) >= 2 && ((b[0] == 0xFF && b[1] == 0xFE) || (b[0] == 0xFE && b[1] == 0xFF)) {
		return true
	}
	for _, magic := range [][]byte{
		{0x7F, 'E', 'L', 'F'}, {0x89, 'P', 'N', 'G'}, {0xFF, 0xD8, 0xFF},
		[]byte("GIF87a"), []byte("GIF89a"), []byte("%PDF-"), []byte("PK\x03\x04"),
		[]byte("PK\x05\x06"), []byte("PK\x07\x08"), {0x1F, 0x8B}, []byte("BZh"),
		[]byte("7z\xBC\xAF\x27\x1C"), []byte("Rar!\x1A\x07"), {0xFD, '7', 'z', 'X', 'Z', 0},
		{0x28, 0xB5, 0x2F, 0xFD}, {0xCA, 0xFE, 0xBA, 0xBE}, {0xFE, 0xED, 0xFA, 0xCE},
		{0xCE, 0xFA, 0xED, 0xFE}, {0xCF, 0xFA, 0xED, 0xFE}, {0xD0, 0xCF, 0x11, 0xE0},
		[]byte("RIFF"), []byte("OggS"), []byte("fLaC"), []byte("ID3"),
		{0x1A, 0x45, 0xDF, 0xA3},
	} {
		if bytes.HasPrefix(b, magic) {
			return false
		}
	}
	if bytes.IndexByte(b, 0) >= 0 {
		order, ok := remotePreviewUTF16Order(b)
		if !ok {
			return false
		}
		return remotePreviewIsText([]byte(remotePreviewUTF16(b, order)))
	}
	controls := 0
	for _, c := range b {
		if c == 0x7F || (c < 0x20 && c != '\t' && c != '\n' && c != '\r' && c != '\f' && c != '\b' && c != 0x1B) {
			controls++
		}
	}
	if controls*100 > len(b)*2 {
		return false
	}
	if utf8.Valid(b) {
		return true
	}
	printable := 0
	for _, c := range b {
		if c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\b' || c == 0x1B ||
			(c >= 0x20 && c < 0x7F) || c >= 0xA0 {
			printable++
		}
	}
	return printable*100 >= len(b)*98
}

func remotePreviewUTF16Order(b []byte) (binary.ByteOrder, bool) {
	if len(b) >= 2 {
		switch {
		case b[0] == 0xFF && b[1] == 0xFE:
			return binary.LittleEndian, true
		case b[0] == 0xFE && b[1] == 0xFF:
			return binary.BigEndian, true
		}
	}
	n := len(b)
	if n > 512 {
		n = 512
	}
	evenZero, oddZero := 0, 0
	for i := 0; i < n; i++ {
		if b[i] != 0 {
			continue
		}
		if i%2 == 0 {
			evenZero++
		} else {
			oddZero++
		}
	}
	pairs := n / 2
	if pairs < 2 || n-evenZero-oddZero < n/4 {
		return nil, false
	}
	switch {
	case oddZero*10 >= pairs*9 && evenZero*10 <= pairs:
		return binary.LittleEndian, true
	case evenZero*10 >= pairs*9 && oddZero*10 <= pairs:
		return binary.BigEndian, true
	}
	return nil, false
}

func remotePreviewDecodeText(b []byte) string {
	if len(b) >= 2 {
		switch {
		case b[0] == 0xFF && b[1] == 0xFE:
			return remotePreviewUTF16(b[2:], binary.LittleEndian)
		case b[0] == 0xFE && b[1] == 0xFF:
			return remotePreviewUTF16(b[2:], binary.BigEndian)
		case len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF:
			b = b[3:]
		}
	}
	if order, ok := remotePreviewUTF16Order(b); ok {
		return remotePreviewUTF16(b, order)
	}
	return strings.ToValidUTF8(string(b), "�")
}

func remotePreviewUTF16(b []byte, order binary.ByteOrder) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = order.Uint16(b[i*2:])
	}
	return string(utf16.Decode(u))
}

// remotePreviewLanguage returns a Prism language id for the file name, falling
// back to the first bytes for extensionless scripts and XML/JSON documents.
func remotePreviewLanguage(name string, sample []byte) string {
	base := strings.ToLower(path.Base(name))
	if lang, ok := remotePreviewLanguageByName[base]; ok {
		return lang
	}
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return "bash"
	}
	if base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") || base == "containerfile" || strings.HasPrefix(base, "containerfile.") {
		return "docker"
	}
	if base == "makefile" || strings.HasPrefix(base, "makefile.") {
		return "makefile"
	}
	ext := strings.ToLower(path.Ext(base))
	if lang, ok := remotePreviewLanguageByExt[ext]; ok {
		return lang
	}
	if strings.HasSuffix(base, ".conf") {
		switch {
		case strings.Contains(base, "nginx"):
			return "nginx"
		case strings.Contains(base, "apache") || strings.Contains(base, "httpd"):
			return "apacheconf"
		default:
			return "ini"
		}
	}
	if len(sample) == 0 {
		return ""
	}
	text := strings.TrimSpace(remotePreviewDecodeText(sample))
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "#!") {
		line := text
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line = line[:i]
		}
		line = strings.ToLower(line)
		switch {
		case strings.Contains(line, "python"):
			return "python"
		case strings.Contains(line, "node") || strings.Contains(line, "deno") || strings.Contains(line, "bun"):
			return "javascript"
		case strings.Contains(line, "bash") || strings.Contains(line, "zsh") || strings.Contains(line, "ksh") || strings.Contains(line, "/sh"):
			return "bash"
		case strings.Contains(line, "ruby"):
			return "ruby"
		case strings.Contains(line, "php"):
			return "php"
		case strings.Contains(line, "pwsh") || strings.Contains(line, "powershell"):
			return "powershell"
		}
	}
	if (strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[")) && json.Valid([]byte(text)) {
		return "json"
	}
	if strings.HasPrefix(text, "<?xml") || strings.HasPrefix(text, "<!DOCTYPE") || strings.HasPrefix(text, "<!--") {
		return "markup"
	}
	if len(text) > 1 && text[0] == '<' {
		for i := 1; i < len(text) && i < 64; i++ {
			c := text[i]
			if c == '/' || c == '!' || c == '?' {
				continue
			}
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				return "markup"
			}
			break
		}
	}
	if strings.HasPrefix(text, "---") {
		return "yaml"
	}
	for i, line := range strings.Split(text, "\n") {
		if i >= 20 {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.Contains(line, "]") {
			return "ini"
		}
		break
	}
	return ""
}

type previewWriter struct {
	http.ResponseWriter
	bytes  int64
	status int
}

func (w *previewWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}
func (w *previewWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	n, e := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, e
}

func previewRemoteFileHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet, http.MethodHead) {
		return
	}
	c, acc, ok := fileRequest(w, r, PermFilesRead)
	if !ok {
		return
	}
	rel := r.URL.Query().Get("path")
	if rel == "" || strings.ContainsAny(rel, "\x00\n\r") {
		jsonError(w, "Invalid file path", 400)
		return
	}
	kind, mime := remotePreviewType(rel)
	if kind == "" {
		jsonError(w, "This file type has no embedded preview; download it to open it", 415)
		return
	}
	sniff := kind == "sniff"
	if sniff {
		kind = "text"
	}
	meta := r.URL.Query().Get("metadata") == "1"
	text := r.URL.Query().Get("text") == "1"
	if text && kind != "text" {
		jsonError(w, "Not a text preview", 400)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	info := remotePreviewInfo{Name: path.Base(rel), Kind: kind, MIME: mime, Lang: remotePreviewLanguage(rel, nil)}
	pw := &previewWriter{ResponseWriter: w}
	started := false
	serve := func(reader io.ReadSeeker, size int64, mtime time.Time) error {
		info.Size = size
		var sample []byte
		if sniff {
			sample = make([]byte, remotePreviewSampleMax)
			n, err := io.ReadFull(reader, sample)
			if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
				return err
			}
			sample = sample[:n]
			if !remotePreviewIsText(sample) {
				return errPreviewNotText
			}
			info.Lang = remotePreviewLanguage(rel, sample)
		}
		if meta {
			jsonOK(pw, info)
			started = true
			return nil
		}
		if text {
			var src io.Reader = reader
			if sniff {
				src = io.MultiReader(bytes.NewReader(sample), reader)
			}
			b, err := io.ReadAll(io.LimitReader(src, remotePreviewTextMax+1))
			if err != nil {
				return err
			}
			if len(b) > remotePreviewTextMax {
				b = b[:remotePreviewTextMax]
				info.Truncated = true
			}
			info.Text = remotePreviewDecodeText(b)
			jsonOK(pw, info)
			started = true
			return nil
		}
		if sniff {
			if _, err := reader.Seek(0, io.SeekStart); err != nil {
				return err
			}
		}
		pw.Header().Set("Content-Type", mime)
		pw.Header().Set("Content-Disposition", "inline")
		pw.Header().Set("X-Content-Type-Options", "nosniff")
		pw.Header().Set("Cache-Control", "private, no-store")
		if kind == "pdf" {
			pw.Header().Set("Content-Security-Policy", "default-src 'none'; object-src 'none'; base-uri 'none'; frame-ancestors 'self'")
		} else {
			pw.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		}
		pw.Header().Set("Accept-Ranges", "bytes")
		started = true
		http.ServeContent(pw, r, info.Name, mtime, reader)
		return nil
	}
	err := fileOp(c, true, func(sc *sftp.Client) error {
		st, err := sc.Stat(rel)
		if err != nil {
			return err
		}
		if !st.Mode().IsRegular() {
			return fmt.Errorf("preview requires a regular file")
		}
		if meta && !sniff {
			return serve(nil, st.Size(), st.ModTime())
		}
		file, err := sc.Open(rel)
		if err != nil {
			return err
		}
		defer file.Close()
		stop := context.AfterFunc(r.Context(), func() { file.Close() })
		defer stop()
		return serve(file, st.Size(), st.ModTime())
	}, func(fc *ftp.ServerConn) error {
		size, err := fc.FileSize(rel)
		if err != nil || size < 0 {
			return fmt.Errorf("FTP server must provide file size for streaming previews")
		}
		if meta && !sniff {
			return serve(nil, size, time.Time{})
		}
		reader := &ftpPreviewReader{client: fc, name: rel, size: size}
		defer reader.Close()
		return serve(reader, size, time.Time{})
	})
	if err != nil && !started {
		if errors.Is(err, errPreviewNotText) {
			jsonError(w, "This file is not text; download it to open it", 415)
			return
		}
		jsonError(w, "Cannot preview file: "+redactText(err.Error()), 502)
		return
	}
	if started {
		status := "ok"
		if err != nil || r.Context().Err() != nil || pw.status >= 400 {
			status = "failed"
		}
		auditFileOp(r, acc, c, "file.preview", rel, map[string]interface{}{"bytes": pw.bytes, "status": status, "range": truncateStr(r.Header.Get("Range"), 100), "metadata": meta, "text": text})
	}
}

// Seek starts the next FTP transfer at REST offset; no discarded download prefix is
// needed to seek a large file. Used only by the request goroutine.
type ftpPreviewReader struct {
	client    *ftp.ServerConn
	name      string
	size, pos int64
	response  *ftp.Response
}

func (f *ftpPreviewReader) Read(b []byte) (int, error) {
	if f.pos >= f.size {
		return 0, io.EOF
	}
	if f.response == nil {
		resp, err := f.client.RetrFrom(f.name, uint64(f.pos))
		if err != nil {
			return 0, err
		}
		f.response = resp
	}
	if int64(len(b)) > f.size-f.pos {
		b = b[:f.size-f.pos]
	}
	n, err := f.response.Read(b)
	f.pos += int64(n)
	return n, err
}
func (f *ftpPreviewReader) Seek(offset int64, whence int) (int64, error) {
	p := offset
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		p += f.pos
	case io.SeekEnd:
		p += f.size
	default:
		return 0, fmt.Errorf("invalid seek")
	}
	if p < 0 {
		return 0, fmt.Errorf("negative seek")
	}
	if p != f.pos {
		if err := f.Close(); err != nil {
			return 0, err
		}
		f.pos = p
	}
	return p, nil
}
func (f *ftpPreviewReader) Close() error {
	if f.response != nil {
		r := f.response
		f.response = nil
		return r.Close()
	}
	return nil
}
