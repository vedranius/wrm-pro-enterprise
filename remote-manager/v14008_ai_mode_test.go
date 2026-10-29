// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"sync"
	"testing"
)

// TestModeAndAutoReturnsACopy covers the remaining half of AI-02: reading s.Mode and s.Auto
// separately (as the OpenCode start did) raced with setModeInternal replacing the pointer.
func TestModeAndAutoReturnsACopy(t *testing.T) {
	s := &aiSession{Mode: aiModeAuto, Auto: &aiAutoLimits{Used: 3, MaxActions: 10}}

	mode, auto := s.modeAndAuto()
	if mode != aiModeAuto || auto == nil || auto.Used != 3 || auto.MaxActions != 10 {
		t.Fatalf("mode=%q auto=%+v", mode, auto)
	}
	auto.Used = 99
	if s.Auto.Used != 3 {
		t.Fatal("modeAndAuto handed out the live limits")
	}

	if mode, auto := (&aiSession{Mode: aiModeAsk}).modeAndAuto(); mode != aiModeAsk || auto != nil {
		t.Fatalf("no limits: mode=%q auto=%+v", mode, auto)
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			s.mu.Lock()
			if i%2 == 0 {
				s.Mode, s.Auto = aiModeAsk, nil
			} else {
				s.Mode, s.Auto = aiModeAuto, &aiAutoLimits{Used: i}
			}
			s.mu.Unlock()
		}(i)
		go func() {
			defer wg.Done()
			if m, a := s.modeAndAuto(); m == "" {
				t.Error("empty mode")
			} else if a != nil && a.Used < 0 {
				t.Error("negative budget")
			}
		}()
	}
	wg.Wait()
}
