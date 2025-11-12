//go:build !windows

// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package tests

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func pickFreePort(t *testing.T) string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to pick port: %v", err)
	}
	defer func() {
		if cerr := ln.Close(); cerr != nil {
			t.Logf("warning: failed to close listener: %v", cerr)
		}
	}()
	addr := ln.Addr().String()
	parts := strings.Split(addr, ":")
	return parts[len(parts)-1]
}

func waitForHTTP(t *testing.T, url string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			if rcerr := resp.Body.Close(); rcerr != nil {
				t.Logf("warning: failed to close response body: %v", rcerr)
			}
			if resp.StatusCode == 200 {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", url)
}

func TestSighupReloadIntegration(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "media-pi-agent-test")

	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = filepath.Join("..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, string(out))
	}

	port := pickFreePort(t)
	listen := fmt.Sprintf("127.0.0.1:%s", port)

	cfgPath := filepath.Join(tmp, "agent.yaml")
	initialKey := "initial-key-000"
	newKey := "new-key-999"
	cfg := fmt.Sprintf("allowed_units:\n  - foo.service\nserver_key: \"%s\"\nlisten_addr: \"%s\"\n", initialKey, listen)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatalf("write cfg: %v", err)
	}

	// Start the agent process with MEDIA_PI_AGENT_CONFIG pointing to our config
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"MEDIA_PI_AGENT_CONFIG="+cfgPath,
		"MEDIA_PI_AGENT_MOCK_DBUS=1",
	)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start agent: %v", err)
	}
	// ensure process is cleaned up
	defer func() {
		_ = cmd.Process.Kill()
		if werr := cmd.Wait(); werr != nil {
			t.Logf("warning: waiting for agent process: %v", werr)
		}
	}()

	// read logs to keep pipes from blocking
	go func() {
		s := bufio.NewScanner(stdout)
		for s.Scan() {
			t.Logf("agent: %s", s.Text())
		}
	}()
	go func() {
		s := bufio.NewScanner(stderr)
		for s.Scan() {
			t.Logf("agent-err: %s", s.Text())
		}
	}()

	healthURL := fmt.Sprintf("http://%s/health", listen)
	waitForHTTP(t, healthURL, 5*time.Second)

	// protected endpoint - list units (requires auth)
	listURL := fmt.Sprintf("http://%s/api/units", listen)

	// initial key should work
	req, _ := http.NewRequestWithContext(context.Background(), "GET", listURL, nil)
	req.Header.Set("Authorization", "Bearer "+initialKey)
	cl := &http.Client{Timeout: 2 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 with initial key, got %d", resp.StatusCode)
	}
	if rcerr := resp.Body.Close(); rcerr != nil {
		t.Logf("warning: failed to close response body: %v", rcerr)
	}

	// Now update config with new key
	cfg2 := fmt.Sprintf("allowed_units:\n  - foo.service\nserver_key: \"%s\"\nlisten_addr: \"%s\"\n", newKey, listen)
	if err := os.WriteFile(cfgPath, []byte(cfg2), 0644); err != nil {
		t.Fatalf("write cfg2: %v", err)
	}

	// send SIGHUP to process
	if err := cmd.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatalf("failed to signal process: %v", err)
	}

	// Wait briefly to let reload happen
	time.Sleep(500 * time.Millisecond)

	// old key should now be rejected
	req, _ = http.NewRequestWithContext(context.Background(), "GET", listURL, nil)
	req.Header.Set("Authorization", "Bearer "+initialKey)
	resp, err = cl.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode == 200 {
		// attempt to parse body for debugging
		var body any
		_ = json.NewDecoder(resp.Body).Decode(&body)
		if rcerr := resp.Body.Close(); rcerr != nil {
			t.Logf("warning: failed to close response body: %v", rcerr)
		}
		t.Fatalf("expected old key to be rejected after reload, but got 200; body=%v", body)
	}
	if rcerr := resp.Body.Close(); rcerr != nil {
		t.Logf("warning: failed to close response body: %v", rcerr)
	}

	// new key should work
	req, _ = http.NewRequestWithContext(context.Background(), "GET", listURL, nil)
	req.Header.Set("Authorization", "Bearer "+newKey)
	resp, err = cl.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 with new key, got %d", resp.StatusCode)
	}
	if rcerr := resp.Body.Close(); rcerr != nil {
		t.Logf("warning: failed to close response body: %v", rcerr)
	}
}
