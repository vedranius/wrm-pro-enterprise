// Copyright 2026 vedranius and WRM contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

type fakeGuacdDocker struct {
	mu        sync.Mutex
	calls     [][]string
	container bool
	running   bool
	ready     bool
	serverOS  string
}

func (f *fakeGuacdDocker) command(_ context.Context, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string(nil), args...))
	switch args[0] {
	case "version":
		if f.serverOS != "" {
			return []byte(f.serverOS + "\n"), nil
		}
		return []byte("linux\n"), nil
	case "inspect":
		if !f.container {
			return []byte("Error: No such object: " + guacdManagedContainer), errors.New("exit status 1")
		}
		return []byte(fmt.Sprintf("%t|%s", f.running, guacdManagedImage)), nil
	case "start":
		f.running, f.ready = true, true
		return []byte(guacdManagedContainer + "\n"), nil
	case "run":
		f.container, f.running, f.ready = true, true, true
		return []byte("container-id\n"), nil
	}
	return nil, fmt.Errorf("unexpected docker command %q", args[0])
}

func (f *fakeGuacdDocker) called(name string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if len(c) > 0 && c[0] == name {
			return c
		}
	}
	return nil
}

func stubGuacd(t *testing.T) *fakeGuacdDocker {
	t.Helper()
	f := &fakeGuacdDocker{}
	oldGOOS, oldCLI, oldReady, oldCmd := guacdGOOS, guacdDockerCLI, guacdReady, guacdDockerCommand
	oldValues := map[string]string{}
	for _, k := range []string{"desktop_enabled", "guacd_auto", "guacd_address"} {
		settingsStore.RLock()
		oldValues[k] = settingsStore.values[k]
		settingsStore.RUnlock()
	}
	guacdGOOS = "windows"
	guacdDockerCLI = func() string { return "docker" }
	guacdReady = func() error {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.ready {
			return nil
		}
		return errors.New("not reachable")
	}
	guacdDockerCommand = f.command
	guacdSetupGate = make(chan struct{}, 1)
	guacdManaged.Lock()
	guacdManaged.starting, guacdManaged.lastErr = false, ""
	guacdManaged.Unlock()
	set := func(k, v string) {
		settingsStore.Lock()
		settingsStore.values[k] = v
		settingsStore.Unlock()
	}
	set("desktop_enabled", "1")
	set("guacd_auto", "1")
	set("guacd_address", "127.0.0.1:4822")
	t.Cleanup(func() {
		guacdGOOS, guacdDockerCLI, guacdReady, guacdDockerCommand = oldGOOS, oldCLI, oldReady, oldCmd
		guacdSetupGate = make(chan struct{}, 1)
		guacdManaged.Lock()
		guacdManaged.starting, guacdManaged.lastErr = false, ""
		guacdManaged.Unlock()
		for k, v := range oldValues {
			settingsStore.Lock()
			if v == "" {
				delete(settingsStore.values, k)
			} else {
				settingsStore.values[k] = v
			}
			settingsStore.Unlock()
		}
	})
	return f
}

func TestGuacdLocalTarget(t *testing.T) {
	settingsStore.RLock()
	old, had := settingsStore.values["guacd_address"]
	settingsStore.RUnlock()
	t.Cleanup(func() {
		settingsStore.Lock()
		if had {
			settingsStore.values["guacd_address"] = old
		} else {
			delete(settingsStore.values, "guacd_address")
		}
		settingsStore.Unlock()
	})
	tests := []struct {
		addr string
		port string
		ok   bool
	}{
		{"127.0.0.1:4822", "4822", true},
		{"localhost:4822", "4822", true},
		{"[::1]:4822", "4822", true},
		{"127.0.0.1:4900", "4900", true},
		{"10.0.0.5:4822", "", false},
		{"127.0.0.1", "", false},
		{"127.0.0.1:0", "", false},
	}
	for _, tt := range tests {
		settingsStore.Lock()
		settingsStore.values["guacd_address"] = tt.addr
		settingsStore.Unlock()
		port, ok := guacdLocalTarget()
		if port != tt.port || ok != tt.ok {
			t.Fatalf("guacdLocalTarget(%q) = %q, %v; want %q, %v", tt.addr, port, ok, tt.port, tt.ok)
		}
	}
}

func TestGuacdAutoStartsContainer(t *testing.T) {
	f := stubGuacd(t)
	if err := guacdEnsure(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	run := f.called("run")
	if len(run) == 0 {
		t.Fatal("docker run was not called")
	}
	joined := strings.Join(run, " ")
	for _, want := range []string{"--name " + guacdManagedContainer, "--restart unless-stopped", "-p 127.0.0.1:4822:4822", guacdManagedImage} {
		if !strings.Contains(joined, want) {
			t.Fatalf("docker run lacks %q: %s", want, joined)
		}
	}
}

func TestGuacdWindowsContainersIsRejected(t *testing.T) {
	f := stubGuacd(t)
	f.serverOS = "windows"
	if err := guacdEnsure(context.Background(), false); !errors.Is(err, errGuacdWindowsContainers) {
		t.Fatalf("guacdEnsure = %v; want errGuacdWindowsContainers", err)
	}
	if f.called("run") != nil {
		t.Fatal("docker run was called with Windows containers")
	}
	if got := guacdFailureMessage(errGuacdWindowsContainers); !strings.Contains(got, "Linux containers") {
		t.Fatalf("failure message = %q", got)
	}
}

func TestGuacdStartsExistingContainer(t *testing.T) {
	f := stubGuacd(t)
	f.container, f.running = true, false
	if err := guacdEnsure(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if f.called("start") == nil {
		t.Fatal("docker start was not called")
	}
	if f.called("run") != nil {
		t.Fatal("docker run was called for an existing container")
	}
}

func TestGuacdAutoOffDoesNotTouchDocker(t *testing.T) {
	f := stubGuacd(t)
	settingsStore.Lock()
	settingsStore.values["guacd_auto"] = "0"
	settingsStore.Unlock()
	if err := guacdEnsure(context.Background(), false); !errors.Is(err, errGuacdAutoOff) {
		t.Fatalf("guacdEnsure = %v; want errGuacdAutoOff", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("Docker was called with guacd_auto off: %v", f.calls)
	}
}

func TestGuacdRemoteAddressIsNotManaged(t *testing.T) {
	f := stubGuacd(t)
	settingsStore.Lock()
	settingsStore.values["guacd_address"] = "10.0.0.5:4822"
	settingsStore.Unlock()
	if err := guacdEnsure(context.Background(), false); !errors.Is(err, errGuacdNotLocal) {
		t.Fatalf("guacdEnsure = %v; want errGuacdNotLocal", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("Docker was called for a remote guacd: %v", f.calls)
	}
}

func TestGuacdFailureMessages(t *testing.T) {
	f := stubGuacd(t)
	_ = f
	cases := []struct {
		err  error
		want string
	}{
		{errGuacdNoDocker, "Install Docker Desktop"},
		{errGuacdDockerDown, "not running"},
		{errGuacdWindowsContainers, "Linux containers"},
		{context.DeadlineExceeded, "starting guacd automatically"},
	}
	for _, c := range cases {
		if got := guacdFailureMessage(c.err); !strings.Contains(got, c.want) {
			t.Fatalf("guacdFailureMessage(%v) = %q; want %q", c.err, got, c.want)
		}
	}
}
