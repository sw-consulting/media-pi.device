// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/sw-consulting/media-pi.device/internal/agent"
	"gopkg.in/yaml.v3"
)

func TestIsAllowed(t *testing.T) {
	agent.AllowedUnits = map[string]struct{}{"a.service": {}, "b.service": {}}
	if err := agent.IsAllowed("a.service"); err != nil {
		t.Fatalf("expected allowed, got %v", err)
	}
	if err := agent.IsAllowed("c.service"); err == nil {
		t.Fatalf("expected error for c.service")
	}
}

func TestLoadConfigFrom(t *testing.T) {
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "agent.yaml")

	yamlData := `allowed_units:
  - a.service
  - b.service
server_key: "test-key-123"
listen_addr: "0.0.0.0:8081"
`
	if err := os.WriteFile(p, []byte(yamlData), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Store original values
	origAllowed := agent.AllowedUnits
	origKey := agent.ServerKey
	defer func() {
		agent.AllowedUnits = origAllowed
		agent.ServerKey = origKey
	}()

	if _, err := agent.LoadConfigFrom(p); err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := agent.AllowedUnits["a.service"]; !ok {
		t.Fatalf("a.service missing")
	}
	if _, ok := agent.AllowedUnits["b.service"]; !ok {
		t.Fatalf("b.service missing")
	}
	if agent.ServerKey != "test-key-123" {
		t.Fatalf("expected server_key 'test-key-123', got %q", agent.ServerKey)
	}
}

func TestGenerateServerKey(t *testing.T) {
	key, err := agent.GenerateServerKey()
	if err != nil {
		t.Fatalf("generateServerKey failed: %v", err)
	}
	if len(key) != 64 {
		t.Fatalf("expected key length 64, got %d", len(key))
	}
}

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	agent.HandleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp agent.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.OK {
		t.Fatalf("expected ok=true, got %v", resp.OK)
	}
}

func TestAuthMiddleware(t *testing.T) {
	origKey := agent.ServerKey
	defer func() { agent.ServerKey = origKey }()

	agent.ServerKey = "test-key-123"

	handler := agent.AuthMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("authorized")); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", w.Code)
	}

	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w = httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong token, got %d", w.Code)
	}

	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer test-key-123")
	w = httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with correct token, got %d", w.Code)
	}
}

func TestUnitActionRequest(t *testing.T) {
	origAllowed := agent.AllowedUnits
	origKey := agent.ServerKey
	defer func() {
		agent.AllowedUnits = origAllowed
		agent.ServerKey = origKey
	}()

	agent.AllowedUnits = map[string]struct{}{"test.service": {}}
	agent.ServerKey = "test-key-123"

	req := httptest.NewRequest("POST", "/api/units/start", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Authorization", "Bearer test-key-123")
	w := httptest.NewRecorder()

	agent.HandleUnitAction("start")(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}

	reqData := agent.UnitActionRequest{Unit: ""}
	jsonData, _ := json.Marshal(reqData)
	req = httptest.NewRequest("POST", "/api/units/start", bytes.NewReader(jsonData))
	req.Header.Set("Authorization", "Bearer test-key-123")
	w = httptest.NewRecorder()

	agent.HandleUnitAction("start")(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing unit, got %d", w.Code)
	}

	reqData = agent.UnitActionRequest{Unit: "forbidden.service"}
	jsonData, _ = json.Marshal(reqData)
	req = httptest.NewRequest("POST", "/api/units/start", bytes.NewReader(jsonData))
	req.Header.Set("Authorization", "Bearer test-key-123")
	w = httptest.NewRecorder()

	agent.HandleUnitAction("start")(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for forbidden unit, got %d", w.Code)
	}
}

func TestSetupConfig(t *testing.T) {
	t.Run("creates configuration when missing", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "agent.yaml")
		if err := agent.SetupConfig(configPath); err != nil {
			t.Fatalf("setupConfig: %v", err)
		}

		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}

		var cfg agent.Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("unmarshal config: %v", err)
		}

		if cfg.ServerKey == "" {
			t.Fatalf("expected server key to be generated")
		}
		if len(cfg.AllowedUnits) != 0 {
			t.Fatalf("expected no default allowed units, got %#v", cfg.AllowedUnits)
		}
		if cfg.ListenAddr != agent.DefaultListenAddr {
			t.Fatalf("expected listen addr %q, got %q", agent.DefaultListenAddr, cfg.ListenAddr)
		}
	})
}
