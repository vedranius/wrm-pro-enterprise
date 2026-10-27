// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/pkg/sftp"
)

// ─── AI: FILE TRANSFERS (MCP scope "transfer") ───────
//
// transfer_file copies one file between two servers of the same AI connection: WRM opens
// SFTP on both SSH clients of the sessions, streams the file into a temporary name next to
// the destination, checks size and SHA-256 and renames it into place. The transfer is
// recorded like a server-to-server copy of the file manager (file_transfers).

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, errAIStopped
	}
	return c.r.Read(p)
}

func (s *aiSession) transferRun(ctx context.Context, pl *aiPlanned) (string, int, error) {
	src := pl.From
	rec := transferRec{Direction: "s2s", SrcConn: &src.conn, SrcPath: pl.FromPath, DstConn: &s.conn, DstPath: pl.Path, Status: "failed"}
	defer func() { logFileTransferAs(s.UserID, s.Username, s.ClientIP, s.ShareID, rec) }()
	fail := func(err error) (string, int, error) {
		rec.Error = err.Error()
		return "Transfer failed: " + err.Error(), 1, nil
	}
	srcCl, err := src.sshClient()
	if err != nil {
		return fail(fmt.Errorf("source: %v", err))
	}
	dstCl, err := s.sshClient()
	if err != nil {
		return fail(fmt.Errorf("destination: %v", err))
	}
	sf, err := sftp.NewClient(srcCl)
	if err != nil {
		return fail(fmt.Errorf("source SFTP: %v", err))
	}
	defer sf.Close()
	df, err := sftp.NewClient(dstCl)
	if err != nil {
		return fail(fmt.Errorf("destination SFTP: %v", err))
	}
	defer df.Close()
	in, err := sf.Open(pl.FromPath)
	if err != nil {
		return fail(fmt.Errorf("open %s: %v", pl.FromPath, err))
	}
	defer in.Close()
	tmp := path.Join(path.Dir(pl.Path), ".wrm-transfer-"+randomToken(6))
	out, err := df.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fail(fmt.Errorf("create in %s: %v", path.Dir(pl.Path), err))
	}
	max := int64(settingInt("ai_mcp_transfer_max_mb")) << 20
	hw := newHashingWriter(out)
	n, err := io.Copy(hw, io.LimitReader(ctxReader{ctx, in}, max+1))
	cerr := out.Close()
	if err == nil {
		err = cerr
	}
	if err == nil && n > max {
		err = fmt.Errorf("the file grew beyond the transfer limit")
	}
	if err == nil && n != pl.Size {
		err = fmt.Errorf("the source file changed during the transfer (%d bytes instead of %d)", n, pl.Size)
	}
	if err != nil {
		df.Remove(tmp)
		if err == errAIStopped {
			rec.Error = "stopped"
			return "", -1, errAIStopped
		}
		return fail(err)
	}
	if st, e := sf.Stat(pl.FromPath); e == nil {
		df.Chmod(tmp, st.Mode().Perm())
	}
	if err := df.PosixRename(tmp, pl.Path); err != nil {
		if err2 := df.Rename(tmp, pl.Path); err2 != nil {
			df.Remove(tmp)
			return fail(fmt.Errorf("rename to %s: %v", pl.Path, err))
		}
	}
	sum := hex.EncodeToString(hw.h.Sum(nil))
	rec.Size, rec.SHA256, rec.Status = n, sum, "ok"
	s.audit(nil, "ai.transfer", map[string]interface{}{"from_conn": src.ConnName, "from": pl.FromPath, "path": pl.Path, "bytes": n, "sha256": sum})
	return fmt.Sprintf("Copied %d bytes from %s:%s to %s:%s (SHA-256 %s).", n, src.ConnName, pl.FromPath, s.ConnName, pl.Path, sum), 0, nil
}
