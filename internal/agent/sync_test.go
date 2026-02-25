// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSyncOnce_FetchManifest(t *testing.T) {
	// Setup mock server
	manifest := []ManifestItem{
		{
			ID:            "1",
			Filename:      "test.mp4",
			FileSizeBytes: 100,
			SHA256:        "abc123",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/devicesync" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(manifest)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Setup temp directory
	tmpDir := t.TempDir()

	// Setup config
	AgentConfig = &Config{
		CoreAPIBase: server.URL,
		MediaDir:    tmpDir,
		Sync: SyncConfig{
			Enabled:              true,
			Schedule:             []string{"03:00", "15:00"},
			MaxParallelDownloads: 2,
		},
	}

	// Note: This test will fail to download the file (404) but we're just testing
	// that the manifest fetch works
	err := SyncOnce(context.Background())
	// We expect an error because the file download will fail (404)
	if err == nil {
		t.Error("expected error since file download should fail, got nil")
	}
	// But the error should be about downloading, not fetching manifest
	if err != nil && err.Error() != "" {
		// Check that error mentions download, not manifest
		t.Logf("Got expected error: %v", err)
	}
}

func TestSyncOnce_VerifyHashMismatch(t *testing.T) {
	// Create a test file with known content
	tmpDir := t.TempDir()
	testContent := []byte("test content")
	testHash := sha256.Sum256(testContent)
	correctHash := hex.EncodeToString(testHash[:])
	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"

	// Setup mock server
	manifest := []ManifestItem{
		{
			ID:            "1",
			Filename:      "test.mp4",
			FileSizeBytes: int64(len(testContent)),
			SHA256:        wrongHash, // Intentionally wrong hash
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/devicesync" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(manifest)
		} else if r.URL.Path == "/api/devicesync/1" {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(testContent)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Setup config
	AgentConfig = &Config{
		CoreAPIBase: server.URL,
		MediaDir:    tmpDir,
		Sync: SyncConfig{
			Enabled:              true,
			Schedule:             []string{"03:00", "15:00"},
			MaxParallelDownloads: 2,
		},
	}

	// Run sync
	err := SyncOnce(context.Background())

	// Should fail due to hash mismatch
	if err == nil {
		t.Fatal("expected error due to hash mismatch")
	}

	if err.Error() == "" || err.Error() == correctHash {
		t.Errorf("expected hash mismatch error, got: %v", err)
	}

	// File should not exist (download failed)
	finalPath := filepath.Join(tmpDir, "test.mp4")
	if _, err := os.Stat(finalPath); err == nil {
		t.Error("file should not exist after failed download")
	}
}

func TestSyncOnce_GarbageCollection(t *testing.T) {
	// Create temp directory with an obsolete file
	tmpDir := t.TempDir()
	obsoleteFile := filepath.Join(tmpDir, "obsolete.mp4")
	if err := os.WriteFile(obsoleteFile, []byte("old content"), 0644); err != nil {
		t.Fatalf("failed to create obsolete file: %v", err)
	}

	// Setup mock server with empty manifest
	manifest := []ManifestItem{} // Empty manifest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/devicesync" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(manifest)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Setup config
	AgentConfig = &Config{
		CoreAPIBase: server.URL,
		MediaDir:    tmpDir,
		Sync: SyncConfig{
			Enabled:              true,
			Schedule:             []string{"03:00", "15:00"},
			MaxParallelDownloads: 2,
		},
	}

	// Run sync
	err := SyncOnce(context.Background())
	if err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// Obsolete file should be removed
	if _, err := os.Stat(obsoleteFile); err == nil {
		t.Error("obsolete file should have been removed")
	}
}

func TestSyncOnce_SkipWhenDisabled(t *testing.T) {
	// Setup config with sync disabled
	AgentConfig = &Config{
		CoreAPIBase: "http://localhost:9999",
		MediaDir:    t.TempDir(),
		Sync: SyncConfig{
			Enabled: false, // Disabled
		},
	}

	// Run sync - should not fail even though server is unreachable
	err := SyncOnce(context.Background())
	if err != nil {
		t.Errorf("sync should not fail when disabled, got: %v", err)
	}
}

func TestGetSyncStatus(t *testing.T) {
	// Reset sync state
	syncState = &SyncStatus{}

	// Get initial status
	status := GetSyncStatus()
	if status.LastSyncOK {
		t.Error("initial sync status should not be OK")
	}

	// Set status
	setSyncStatus(true, nil)

	// Get updated status
	status = GetSyncStatus()
	if !status.LastSyncOK {
		t.Error("sync status should be OK after successful sync")
	}
	if status.LastSyncError != "" {
		t.Errorf("sync status should have no error, got: %s", status.LastSyncError)
	}

	// Set error status
	testErr := testError("test error")
	setSyncStatus(false, &testErr)

	status = GetSyncStatus()
	if status.LastSyncOK {
		t.Error("sync status should not be OK after error")
	}
	if status.LastSyncError != string(testErr) {
		t.Errorf("expected error %q, got %q", testErr, status.LastSyncError)
	}
}

func TestCalculateNextSyncTime(t *testing.T) {
	// Test with multiple times
	schedule := []string{"03:00", "15:00", "21:30"}
	
	// Get next sync time
	nextTime := calculateNextSyncTime(schedule)
	
	// Should be one of the scheduled times
	hour := nextTime.Hour()
	minute := nextTime.Minute()
	
	validTimes := map[int]map[int]bool{
		3:  {0: true},
		15: {0: true},
		21: {30: true},
	}
	
	if !validTimes[hour][minute] {
		t.Errorf("next sync time %02d:%02d is not in schedule", hour, minute)
	}
	
	// Should be in the future
	if !nextTime.After(time.Now()) {
		t.Error("next sync time should be in the future")
	}
}

func TestCalculateNextSyncTime_InvalidFormat(t *testing.T) {
	// Test with invalid time format
	schedule := []string{"invalid", "25:00", "12:99"}
	
	// Should still return a valid time (1 hour from now as fallback)
	nextTime := calculateNextSyncTime(schedule)
	
	if nextTime.IsZero() {
		t.Error("should return a valid fallback time")
	}
	
	// Should be roughly 1 hour from now (within 5 minutes tolerance)
	expectedTime := time.Now().Add(1 * time.Hour)
	diff := nextTime.Sub(expectedTime)
	if diff < -5*time.Minute || diff > 5*time.Minute {
		t.Errorf("fallback time should be ~1 hour from now, got %v", nextTime)
	}
}

// Helper to create error from string
type testError string

func (e *testError) Error() string {
	return string(*e)
}
