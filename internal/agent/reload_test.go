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
	// Check default media-pi service user is set
	if MediaPiServiceUser != "pi" {
		t.Fatalf("expected default media-pi service user 'pi', got %q", MediaPiServiceUser)
	}
}

func TestReloadConfigWithCrontabUser(t *testing.T) {
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "yaml")

	yamlData := `allowed_units:
  - reload.service
server_key: "reload-key-xyz"
listen_addr: "0.0.0.0:8081"
media_pi_service_user: "customuser"
`
	if err := os.WriteFile(p, []byte(yamlData), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Load initial config through LoadConfigFrom via ConfigPath+Reload
	ConfigPath = p
	if err := ReloadConfig(); err != nil {
		t.Fatalf("ReloadConfig failed: %v", err)
	}
	if MediaPiServiceUser != "customuser" {
		t.Fatalf("expected media-pi service user 'customuser', got %q", MediaPiServiceUser)
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

// TestSetupConfigUpgradeFrom06x verifies that SetupConfig properly applies
// defaults for new 0.7.0 parameters when upgrading from a 0.6.x config file
func TestSetupConfigUpgradeFrom06x(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "agent.yaml")

	// Simulate an old 0.6.x config file without the new parameters
	oldConfigData := `allowed_units:
  - old.service
server_key: "old-key-123"
listen_addr: "0.0.0.0:9999"
media_pi_service_user: "olduser"
`
	if err := os.WriteFile(configPath, []byte(oldConfigData), 0600); err != nil {
		t.Fatalf("failed to write old config: %v", err)
	}

	// Run SetupConfig - it should preserve existing values and add defaults for new params
	if err := SetupConfig(configPath); err != nil {
		t.Fatalf("SetupConfig failed: %v", err)
	}

	// Load and verify the upgraded config
	cfg, err := LoadConfigFrom(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFrom failed: %v", err)
	}

	// Check that old values were preserved
	if cfg.ListenAddr != "0.0.0.0:9999" {
		t.Errorf("expected ListenAddr to be preserved as '0.0.0.0:9999', got %q", cfg.ListenAddr)
	}
	if cfg.MediaPiServiceUser != "olduser" {
		t.Errorf("expected MediaPiServiceUser to be preserved as 'olduser', got %q", cfg.MediaPiServiceUser)
	}

	// Check that new parameters have defaults
	if cfg.MediaDir != "/var/lib/media-pi" {
		t.Errorf("expected MediaDir default '/var/lib/media-pi', got %q", cfg.MediaDir)
	}
	if cfg.MaxParallelDownloads != 3 {
		t.Errorf("expected MaxParallelDownloads default 3, got %d", cfg.MaxParallelDownloads)
	}
	if cfg.CoreAPIBase != "https://vezyn.fvds.ru" {
		t.Errorf("expected CoreAPIBase default 'https://vezyn.fvds.ru', got %q", cfg.CoreAPIBase)
	}

	// Check that server_key was regenerated (it will be different from old-key-123)
	if cfg.ServerKey == "old-key-123" {
		t.Error("expected server_key to be regenerated")
	}
	if cfg.ServerKey == "" {
		t.Error("expected server_key to be set")
	}
}

// TestSetupConfigFreshInstall verifies that SetupConfig creates a proper
// config with all defaults for a fresh installation
func TestSetupConfigFreshInstall(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "agent.yaml")

	// Run SetupConfig on non-existent file
	if err := SetupConfig(configPath); err != nil {
		t.Fatalf("SetupConfig failed: %v", err)
	}

	// Load and verify the config
	cfg, err := LoadConfigFrom(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFrom failed: %v", err)
	}

	// Check all defaults are set
	if cfg.ListenAddr != DefaultListenAddr {
		t.Errorf("expected ListenAddr default %q, got %q", DefaultListenAddr, cfg.ListenAddr)
	}
	if cfg.MediaPiServiceUser != "pi" {
		t.Errorf("expected MediaPiServiceUser default 'pi', got %q", cfg.MediaPiServiceUser)
	}
	if cfg.MediaDir != "/var/lib/media-pi" {
		t.Errorf("expected MediaDir default '/var/lib/media-pi', got %q", cfg.MediaDir)
	}
	if cfg.MaxParallelDownloads != 3 {
		t.Errorf("expected MaxParallelDownloads default 3, got %d", cfg.MaxParallelDownloads)
	}
	if cfg.CoreAPIBase != "https://vezyn.fvds.ru" {
		t.Errorf("expected CoreAPIBase default 'https://vezyn.fvds.ru', got %q", cfg.CoreAPIBase)
	}
	if cfg.ServerKey == "" {
		t.Error("expected server_key to be generated")
	}
}

// TestLoadConfigFromAppliesDefaults verifies that LoadConfigFrom applies
// defaults for missing optional parameters
func TestLoadConfigFromAppliesDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "agent.yaml")

	// Write minimal config without optional parameters
	minimalConfig := `allowed_units:
  - test.service
server_key: "test-key-456"
`
	if err := os.WriteFile(configPath, []byte(minimalConfig), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Load config
	cfg, err := LoadConfigFrom(configPath)
	if err != nil {
		t.Fatalf("LoadConfigFrom failed: %v", err)
	}

	// Verify defaults were applied
	if cfg.MediaPiServiceUser != "pi" {
		t.Errorf("expected MediaPiServiceUser default 'pi', got %q", cfg.MediaPiServiceUser)
	}
	if cfg.MediaDir != "/var/lib/media-pi" {
		t.Errorf("expected MediaDir default '/var/lib/media-pi', got %q", cfg.MediaDir)
	}
	if cfg.MaxParallelDownloads != 3 {
		t.Errorf("expected MaxParallelDownloads default 3, got %d", cfg.MaxParallelDownloads)
	}
}
