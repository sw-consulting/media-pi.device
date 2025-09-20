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
