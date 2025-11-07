// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent
package tests

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/sw-consulting/media-pi.device/internal/agent"
)

func TestReloadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "agent.yaml")

	yamlData := `allowed_units:
  - reload.service
server_key: "reload-key-xyz"
listen_addr: "0.0.0.0:8081"
`
	if err := os.WriteFile(p, []byte(yamlData), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Load initial config through LoadConfigFrom via ConfigPath+Reload
	agent.ConfigPath = p
	if err := agent.ReloadConfig(); err != nil {
		t.Fatalf("ReloadConfig failed: %v", err)
	}
	if agent.ServerKey != "reload-key-xyz" {
		t.Fatalf("expected server key to be reloaded, got %q", agent.ServerKey)
	}
	if _, ok := agent.AllowedUnits["reload.service"]; !ok {
		t.Fatalf("expected allowed unit reload.service after reload")
	}
}

func TestInternalReloadHandler(t *testing.T) {
	// prepare config
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "agent.yaml")
	yamlData := `allowed_units:
  - handler.service
server_key: "handler-key-123"
listen_addr: "0.0.0.0:8081"
`
	if err := os.WriteFile(p, []byte(yamlData), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// set config path and initial fake key
	agent.ConfigPath = p
	agent.ServerKey = "handler-key-123"

	mux := http.NewServeMux()
	mux.HandleFunc("/internal/reload", agent.AuthMiddleware(agent.HandleReload))

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
