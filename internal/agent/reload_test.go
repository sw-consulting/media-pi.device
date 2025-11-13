// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent
package agent

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestReloadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "yaml")

	yamlData := `allowed_units:
  - reload.service
server_key: "reload-key-xyz"
listen_addr: "0.0.0.0:8081"
`
	if err := os.WriteFile(p, []byte(yamlData), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Load initial config through LoadConfigFrom via ConfigPath+Reload
	ConfigPath = p
	if err := ReloadConfig(); err != nil {
		t.Fatalf("ReloadConfig failed: %v", err)
	}
	if ServerKey != "reload-key-xyz" {
		t.Fatalf("expected server key to be reloaded, got %q", ServerKey)
	}
	if _, ok := AllowedUnits["reload.service"]; !ok {
		t.Fatalf("expected allowed unit reload.service after reload")
	}
}

func TestInternalReloadHandler(t *testing.T) {
	// prepare config
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "yaml")
	yamlData := `allowed_units:
  - handler.service
server_key: "handler-key-123"
listen_addr: "0.0.0.0:8081"
`
	if err := os.WriteFile(p, []byte(yamlData), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// set config path and initial fake key
	ConfigPath = p
	ServerKey = "handler-key-123"

	mux := http.NewServeMux()
	mux.HandleFunc("/internal/reload", AuthMiddleware(HandleReload))

	// call handler without auth
	req := httptest.NewRequest("POST", "/internal/reload", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", w.Code)
	}

	// call with correct auth
	req = httptest.NewRequest("POST", "/internal/reload", nil)
	req.Header.Set("Authorization", "Bearer handler-key-123")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on successful reload, got %d", w.Code)
	}
}
