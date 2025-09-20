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

	"gopkg.in/yaml.v3"
)

func TestIsAllowed(t *testing.T) {
	allowedUnits = map[string]struct{}{"a.service": {}, "b.service": {}}
	if err := isAllowed("a.service"); err != nil {
		t.Fatalf("expected allowed, got %v", err)
	}
	if err := isAllowed("c.service"); err == nil {
		t.Fatalf("expected error for c.service")
	}
}

func TestLoadConfigFrom(t *testing.T) {
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "agent.yaml")

	yaml := `allowed_units:
  - a.service
  - b.service
server_key: "test-key-123"
listen_addr: "0.0.0.0:8080"
`
	if err := os.WriteFile(p, []byte(yaml), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Store original values
	origAllowed := allowedUnits
	origKey := serverKey
	defer func() {
		allowedUnits = origAllowed
		serverKey = origKey
	}()

	if _, err := loadConfigFrom(p); err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := allowedUnits["a.service"]; !ok {
		t.Fatalf("a.service missing")
	}
	if _, ok := allowedUnits["b.service"]; !ok {
		t.Fatalf("b.service missing")
	}
	if serverKey != "test-key-123" {
		t.Fatalf("expected server_key 'test-key-123', got %q", serverKey)
	}
}

func TestGenerateServerKey(t *testing.T) {
	key, err := generateServerKey()
	if err != nil {
		t.Fatalf("generateServerKey failed: %v", err)
	}
	if len(key) != 64 { // 32 bytes = 64 hex chars
		t.Fatalf("expected key length 64, got %d", len(key))
	}
}

func TestHealthEndpoint(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.OK {
		t.Fatalf("expected ok=true, got %v", resp.OK)
	}

	// Check that response data contains required fields
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be map[string]interface{}, got %T", resp.Data)
	}

	if status, exists := data["status"]; !exists || status != "healthy" {
		t.Fatalf("expected status=healthy, got %v", status)
	}

	if version, exists := data["version"]; !exists {
		t.Fatalf("expected version field to be present")
	} else if versionStr, ok := version.(string); !ok || versionStr == "" {
		t.Fatalf("expected version to be non-empty string, got %v", version)
	}

	if _, exists := data["time"]; !exists {
		t.Fatalf("expected time field to be present")
	}
}

func TestAuthMiddleware(t *testing.T) {
	// Store original key
	origKey := serverKey
	defer func() { serverKey = origKey }()

	serverKey = "test-key-123"

	handler := authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("authorized")); err != nil {
			t.Errorf("Failed to write response: %v", err)
		}
	})

	// Test without auth header
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", w.Code)
	}

	// Test with wrong token
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w = httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong token, got %d", w.Code)
	}

	// Test with correct token
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer test-key-123")
	w = httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with correct token, got %d", w.Code)
	}
}

func TestUnitActionRequest(t *testing.T) {
	// Store original values
	origAllowed := allowedUnits
	origKey := serverKey
	defer func() {
		allowedUnits = origAllowed
		serverKey = origKey
	}()

	allowedUnits = map[string]struct{}{"test.service": {}}
	serverKey = "test-key-123"

	// Test invalid JSON
	req := httptest.NewRequest("POST", "/api/units/start", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Authorization", "Bearer test-key-123")
	w := httptest.NewRecorder()

	handleUnitAction("start")(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}

	// Test missing unit field
	reqData := UnitActionRequest{Unit: ""}
	jsonData, _ := json.Marshal(reqData)
	req = httptest.NewRequest("POST", "/api/units/start", bytes.NewReader(jsonData))
	req.Header.Set("Authorization", "Bearer test-key-123")
	w = httptest.NewRecorder()

	handleUnitAction("start")(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing unit, got %d", w.Code)
	}

	// Test forbidden unit
	reqData = UnitActionRequest{Unit: "forbidden.service"}
	jsonData, _ = json.Marshal(reqData)
	req = httptest.NewRequest("POST", "/api/units/start", bytes.NewReader(jsonData))
	req.Header.Set("Authorization", "Bearer test-key-123")
	w = httptest.NewRecorder()

	handleUnitAction("start")(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for forbidden unit, got %d", w.Code)
	}
}

func TestSetupConfig(t *testing.T) {
	t.Run("creates configuration when missing", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "agent.yaml")
		if err := setupConfig(configPath); err != nil {
			t.Fatalf("setupConfig: %v", err)
		}

		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}

		var cfg Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("unmarshal config: %v", err)
		}

		if cfg.ServerKey == "" {
			t.Fatalf("expected server key to be generated")
		}
		if len(cfg.AllowedUnits) != 0 {
			t.Fatalf("expected no default allowed units, got %#v", cfg.AllowedUnits)
		}
		if cfg.ListenAddr != defaultListenAddr {
			t.Fatalf("expected listen addr %q, got %q", defaultListenAddr, cfg.ListenAddr)
		}
	})

	t.Run("keeps allowed units empty when not specified", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "agent.yaml")
		existingYAML := "listen_addr: \"127.0.0.1:9000\"\n"
		if err := os.WriteFile(configPath, []byte(existingYAML), 0600); err != nil {
			t.Fatalf("write existing config: %v", err)
		}

		if err := setupConfig(configPath); err != nil {
			t.Fatalf("setupConfig: %v", err)
		}

		updatedData, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read updated config: %v", err)
		}

		var updated Config
		if err := yaml.Unmarshal(updatedData, &updated); err != nil {
			t.Fatalf("unmarshal updated config: %v", err)
		}

		if len(updated.AllowedUnits) != 0 {
			t.Fatalf("expected allowed units to remain empty, got %#v", updated.AllowedUnits)
		}
		if updated.ListenAddr != "127.0.0.1:9000" {
			t.Fatalf("expected listen addr to be preserved, got %q", updated.ListenAddr)
		}
	})

	t.Run("updates existing configuration without server key", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "agent.yaml")
		existing := Config{
			AllowedUnits: []string{"custom.service"},
			ListenAddr:   "127.0.0.1:9000",
			ServerKey:    "",
		}
		data, err := yaml.Marshal(existing)
		if err != nil {
			t.Fatalf("marshal existing config: %v", err)
		}
		if err := os.WriteFile(configPath, data, 0600); err != nil {
			t.Fatalf("write existing config: %v", err)
		}

		if err := setupConfig(configPath); err != nil {
			t.Fatalf("setupConfig: %v", err)
		}

		updatedData, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read updated config: %v", err)
		}
		var updated Config
		if err := yaml.Unmarshal(updatedData, &updated); err != nil {
			t.Fatalf("unmarshal updated config: %v", err)
		}

		if updated.ServerKey == "" {
			t.Fatalf("expected server key to be generated")
		}
		if len(updated.AllowedUnits) != 1 || updated.AllowedUnits[0] != "custom.service" {
			t.Fatalf("expected allowed units to be preserved, got %#v", updated.AllowedUnits)
		}
		if updated.ListenAddr != "127.0.0.1:9000" {
			t.Fatalf("expected listen addr to be preserved, got %q", updated.ListenAddr)
		}
	})

	t.Run("overwrites existing server key with warning", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "agent.yaml")
		existing := Config{
			AllowedUnits: []string{"custom.service"},
			ListenAddr:   "127.0.0.1:9000",
			ServerKey:    "existing-key-123",
		}
		data, err := yaml.Marshal(existing)
		if err != nil {
			t.Fatalf("marshal existing config: %v", err)
		}
		if err := os.WriteFile(configPath, data, 0600); err != nil {
			t.Fatalf("write existing config: %v", err)
		}

		if err := setupConfig(configPath); err != nil {
			t.Fatalf("setupConfig should succeed and overwrite existing key: %v", err)
		}

		updatedData, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("read updated config: %v", err)
		}
		var updated Config
		if err := yaml.Unmarshal(updatedData, &updated); err != nil {
			t.Fatalf("unmarshal updated config: %v", err)
		}

		if updated.ServerKey == "" {
			t.Fatalf("expected server key to be generated")
		}
		if updated.ServerKey == "existing-key-123" {
			t.Fatalf("expected server key to be replaced, but it remained the same")
		}
		if len(updated.AllowedUnits) != 1 || updated.AllowedUnits[0] != "custom.service" {
			t.Fatalf("expected allowed units to be preserved, got %#v", updated.AllowedUnits)
		}
		if updated.ListenAddr != "127.0.0.1:9000" {
			t.Fatalf("expected listen addr to be preserved, got %q", updated.ListenAddr)
		}
	})
}
