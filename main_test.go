// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAllowed(t *testing.T) {
	allow := map[string]struct{}{"a.service": {}, "b.service": {}}
	if err := allowed(allow, "a.service"); err != nil {
		t.Fatalf("expected allowed, got %v", err)
	}
	if err := allowed(allow, "c.service"); err == nil {
		t.Fatalf("expected error for c.service")
	}
}

func TestLoadConfigFrom(t *testing.T) {
	tmpDir := t.TempDir()
	p := filepath.Join(tmpDir, "agent.yaml")

	yaml := "allowed_units:\n  - a.service\n  - b.service\n"
	if err := os.WriteFile(p, []byte(yaml), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	m, err := loadConfigFrom(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, ok := m["a.service"]; !ok {
		t.Fatalf("a.service missing")
	}
	if _, ok := m["b.service"]; !ok {
		t.Fatalf("b.service missing")
	}
}
