// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/pkg/sftp"
)

func inventoryExtensions(filter string) (map[string]bool, error) {
	if filter == "" {
		filter = "ini,py,conf,json,yaml,yml,log,txt"
	}
	out := map[string]bool{}
	for _, x := range strings.Split(filter, ",") {
		x = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(x)), ".")
		if len(x) < 1 || len(x) > 16 || strings.ContainsAny(x, "/\\*?\x00\r\n ") {
			return nil, fmt.Errorf("use comma-separated file extensions")
		}
		out["."+x] = true
	}
	if len(out) > 20 {
		return nil, fmt.Errorf("at most 20 extensions")
	}
	return out, nil
}

func (s *aiSession) inventoryFiles(parent context.Context, in aiToolInput) (string, int, error) {
	ctx, cancel := context.WithTimeout(parent, time.Duration(aiClamp(settingInt("ai_command_timeout_seconds"), 60, 5, 3600))*time.Second)
	defer cancel()
	cl, err := s.sshClient()
	if err != nil {
		return "", 1, err
	}
	fs, err := sftp.NewClient(cl)
	if err != nil {
		return "", 1, err
	}
	defer fs.Close()
	stop := context.AfterFunc(ctx, func() { fs.Close() })
	defer stop()
	if err := sftpNoSymlinks(fs, in.Path); err != nil {
		return "", 1, err
	}
	root, err := fs.RealPath(in.Path)
	if err != nil {
		return "", 1, err
	}
	if why := aiReadDenied(root); why != "" {
		return "", 1, fmt.Errorf("sensitive root: %s", why)
	}
	ext, _ := inventoryExtensions(in.Filter)
	maxFiles := aiClamp(in.Limit, 500, 1, 3000)
	type directory struct {
		name  string
		depth int
	}
	queue := []directory{{root, 0}}
	var out strings.Builder
	files, visited, skipped, errorsN := 0, 0, 0, 0
	capBytes := int(aiReadMax())
	capped := false
	for len(queue) > 0 && files < maxFiles && visited < 10000 && out.Len() < capBytes {
		if err := ctx.Err(); err != nil {
			return out.String(), 1, err
		}
		d := queue[0]
		queue = queue[1:]
		entries, err := fs.ReadDir(d.name)
		if err != nil {
			errorsN++
			continue
		}
		for _, f := range entries {
			visited++
			if visited > 10000 || files >= maxFiles || out.Len() >= capBytes {
				capped = true
				break
			}
			p := path.Join(d.name, f.Name())
			if f.Mode()&0o170000 != 0 && !f.IsDir() && !f.Mode().IsRegular() {
				skipped++
				continue
			}
			if aiReadDenied(p) != "" {
				skipped++
				continue
			}
			if f.IsDir() {
				if d.depth < 12 {
					queue = append(queue, directory{p, d.depth + 1})
				} else {
					capped = true
				}
				continue
			}
			if !f.Mode().IsRegular() || !ext[strings.ToLower(path.Ext(p))] {
				continue
			}
			real, err := fs.RealPath(p)
			if err != nil || real != p || aiReadDenied(real) != "" {
				skipped++
				continue
			}
			files++
			if in.Grep == "" {
				fmt.Fprintf(&out, "%s (%d bytes)\n", p, f.Size())
				continue
			}
			if f.Size() > 256<<10 {
				fmt.Fprintf(&out, "%s: skipped content (over 256 KiB)\n", p)
				capped = true
				continue
			}
			file, err := fs.Open(p)
			if err != nil {
				errorsN++
				continue
			}
			b, err := io.ReadAll(io.LimitReader(file, 256<<10))
			file.Close()
			if err != nil {
				errorsN++
				continue
			}
			if strings.ContainsRune(string(b), '\x00') {
				skipped++
				continue
			}
			for i, line := range strings.Split(string(b), "\n") {
				if strings.Contains(line, in.Grep) {
					fmt.Fprintf(&out, "%s:%d: %s\n", p, i+1, redactText(truncateStr(line, 1000)))
					if out.Len() >= capBytes {
						capped = true
						break
					}
				}
			}
		}
	}
	if len(queue) > 0 || files >= maxFiles || visited >= 10000 {
		capped = true
	}
	text := out.String()
	if len(text) > capBytes {
		text = text[:capBytes]
		capped = true
	}
	return fmt.Sprintf("%s\n[files checked: %d; entries visited: %d; skipped sensitive/symlink/binary: %d; unreadable directories/files: %d; incomplete: %t; max depth: 12; max file content: 256 KiB]\n", text, files, visited, skipped, errorsN, capped || errorsN > 0), 0, nil
}
