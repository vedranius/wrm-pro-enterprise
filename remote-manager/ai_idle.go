// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// A quiet provider stream must not retain an assistant turn indefinitely. Closing
// the response body interrupts Read without retrying the request or any tool.
type aiIdleBody struct {
	io.ReadCloser
	mu              sync.Mutex
	timer           *time.Timer
	idle            time.Duration
	lastRead        time.Time
	cancel          context.CancelFunc
	expired, closed bool
}

func newAIIdleBody(body io.ReadCloser, idle time.Duration, cancel ...context.CancelFunc) *aiIdleBody {
	b := &aiIdleBody{ReadCloser: body, idle: idle, lastRead: time.Now(), cancel: func() {}}
	if len(cancel) > 0 {
		b.cancel = cancel[0]
	}
	b.timer = time.AfterFunc(idle, func() {
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			return
		}
		if remaining := b.idle - time.Since(b.lastRead); remaining > 0 {
			b.timer.Reset(remaining)
			b.mu.Unlock()
			return
		}
		b.expired = true
		b.mu.Unlock()
		b.cancel()
		body.Close()
	})
	return b
}

func (b *aiIdleBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.expired {
		return n, fmt.Errorf("provider stream interrupted: no response data for %s", b.idle)
	}
	if n > 0 && !b.closed {
		b.lastRead = time.Now()
		b.timer.Reset(b.idle)
	}
	return n, err
}

func (b *aiIdleBody) Close() error {
	b.mu.Lock()
	b.closed = true
	b.timer.Stop()
	b.mu.Unlock()
	b.cancel()
	return b.ReadCloser.Close()
}
