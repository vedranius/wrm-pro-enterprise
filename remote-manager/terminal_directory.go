// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"path"
	"strings"
	"time"
	"unicode/utf16"

	"golang.org/x/crypto/ssh"
)

func terminalDirectoryCommand(directory string, windows bool) (string, error) {
	if directory == "" {
		return "", nil
	}
	if len(directory) > 2048 || strings.ContainsAny(directory, "\x00\r\n") {
		return "", fmt.Errorf("unsupported terminal directory")
	}
	if windows {
		if len(directory) > 3 && directory[0] == '/' && directory[2] == ':' {
			directory = directory[1:]
		}
		script := "Set-Location -LiteralPath '" + strings.ReplaceAll(directory, "'", "''") + "' -ErrorAction Stop"
		return powershellEncoded(script), nil
	}
	script := "cd -- " + shellQuote(directory) + ` && exec "${SHELL:-/bin/sh}" -l`
	return "sh -c " + shellQuote(script), nil
}
func terminalDirectoryStart(cl *ssh.Client, c Connection, directory string, logFiles ...string) (string, error) {
	file := ""
	if len(logFiles) > 0 {
		file = logFiles[0]
	}
	if directory == "" && file == "" {
		return "", nil
	}
	windows := c.Options["terminal_shell"] == "powershell"
	if c.Options["terminal_shell"] == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		out, err := monitorSSHCommand(ctx, cl, "cmd.exe /c ver")
		windows = err == nil && strings.Contains(strings.ToLower(out), "windows")
	}
	if file != "" {
		if len(file) > 2048 || strings.ContainsAny(file, "\x00\r\n") || !strings.HasPrefix(file, "/") {
			return "", fmt.Errorf("unsupported log path")
		}
		file = path.Clean(file)
		allowed := false
		// discovered or configured roots, plus /var/log when it is a scan path
		roots := append(monitorAvailableRoots(c), monitorAvailableRootsFrom(loadLogSettings(c.ID))...)
		for _, root := range roots {
			r := path.Clean(root.Path)
			if strings.HasPrefix(file, r+"/") {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", fmt.Errorf("log is outside configured monitoring roots")
		}
		if windows {
			if len(file) > 3 && file[0] == '/' && file[2] == ':' {
				file = file[1:]
			}
			return powershellEncoded("Set-Location -LiteralPath '" + strings.ReplaceAll(path.Dir(file), "'", "''") + "'; Get-Content -LiteralPath '" + strings.ReplaceAll(file, "'", "''") + "' -Tail 100 -Wait"), nil
		}
		// Ctrl+C stops tail only (the trap keeps this shell alive), then a login shell
		// stays open in the log folder.
		script := "cd -- " + shellQuote(path.Dir(file)) + " && { trap : INT; tail -n 100 -F -- " + shellQuote(file) + `; trap - INT; }; exec "${SHELL:-/bin/sh}" -l`
		return "sh -c " + shellQuote(script), nil
	}
	return terminalDirectoryCommand(directory, windows)
}
func powershellEncoded(script string) string {
	units := utf16.Encode([]rune(script))
	data := make([]byte, len(units)*2)
	for i, v := range units {
		binary.LittleEndian.PutUint16(data[i*2:], v)
	}
	return "powershell.exe -NoLogo -NoExit -EncodedCommand " + base64.StdEncoding.EncodeToString(data)
}
