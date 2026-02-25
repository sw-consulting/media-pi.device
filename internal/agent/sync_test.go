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
	"strings"
	"testing"
	"time"
)

func TestSetAndGetSyncSchedule(t *testing.T) {
	// Use temp file for testing
	tempDir := t.TempDir()
	oldPath := syncSchedulePath
	syncSchedulePath = filepath.Join(tempDir, "sync.schedule.json")
	defer func() { syncSchedulePath = oldPath }()

	times := []string{"03:00", "15:00", "21:30"}
	if err := SetSyncSchedule(times); err != nil {
		t.Fatalf("SetSyncSchedule failed: %v", err)
	}

	retrieved, err := GetSyncSchedule()
	if err != nil {
		t.Fatalf("GetSyncSchedule failed: %v", err)
	}

	if len(retrieved) != len(times) {
		t.Errorf("expected %d times, got %d", len(times), len(retrieved))
	}

	for i, expected := range times {
		if i >= len(retrieved) {
			break
		}
		if retrieved[i] != expected {
			t.Errorf("time[%d]: expected %s, got %s", i, expected, retrieved[i])
		}
	}
}

func TestGetSyncSchedule_NotExists(t *testing.T) {
	// Use temp file that doesn't exist
	tempDir := t.TempDir()
	oldPath := syncSchedulePath
	syncSchedulePath = filepath.Join(tempDir, "nonexistent.json")
	defer func() { syncSchedulePath = oldPath }()

	retrieved, err := GetSyncSchedule()
	if err != nil {
		t.Fatalf("GetSyncSchedule should not error on missing file: %v", err)
	}

	if len(retrieved) != 0 {
		t.Errorf("expected empty schedule, got %d items", len(retrieved))
	}
}

func TestCalculateNextSyncTime(t *testing.T) {
	now := time.Date(2026, 2, 25, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		schedule []string
		now      time.Time
		wantHour int
		wantMin  int
	}{
		{
			name:     "next sync today",
			schedule: []string{"15:00", "03:00"},
			now:      now,
			wantHour: 15,
			wantMin:  0,
		},
		{
			name:     "next sync tomorrow",
			schedule: []string{"09:00", "08:00"},
			now:      now,
			wantHour: 8,
			wantMin:  0,
		},
		{
			name:     "single time today",
			schedule: []string{"12:30"},
			now:      now,
			wantHour: 12,
			wantMin:  30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := calculateNextSyncTime(tt.now, tt.schedule)
			if next.IsZero() {
				t.Fatal("expected non-zero next sync time")
			}

			if next.Hour() != tt.wantHour || next.Minute() != tt.wantMin {
				t.Errorf("expected %02d:%02d, got %02d:%02d",
					tt.wantHour, tt.wantMin, next.Hour(), next.Minute())
			}

			if !next.After(tt.now) {
				t.Error("next sync time should be after now")
			}
		})
	}
}

func TestCalculateNextSyncTime_EmptySchedule(t *testing.T) {
	now := time.Now()
	next := calculateNextSyncTime(now, []string{})

	if !next.IsZero() {
		t.Errorf("expected zero time for empty schedule, got %v", next)
	}
}

func TestFetchManifest(t *testing.T) {
	manifest := []ManifestItem{
		{ID: "1", Filename: "video1.mp4", FileSizeBytes: 1024, SHA256: "abc123"},
		{ID: "2", Filename: "video2.mp4", FileSizeBytes: 2048, SHA256: "def456"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/devicesync" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}

		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(manifest); err != nil {
			t.Errorf("failed to encode manifest: %v", err)
		}
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	oldToken := DeviceAuthToken
	CoreAPIBase = server.URL
	DeviceAuthToken = "test-token"
	defer func() {
		CoreAPIBase = oldBase
		DeviceAuthToken = oldToken
	}()

	ctx := context.Background()
	result, err := fetchManifest(ctx)
	if err != nil {
		t.Fatalf("fetchManifest failed: %v", err)
	}

	if len(result) != len(manifest) {
		t.Errorf("expected %d items, got %d", len(manifest), len(result))
	}
}

func TestFetchManifest_NoConfig(t *testing.T) {
	oldBase := CoreAPIBase
	CoreAPIBase = ""
	defer func() { CoreAPIBase = oldBase }()

	ctx := context.Background()
	_, err := fetchManifest(ctx)
	if err == nil {
		t.Error("expected error when core_api_base not configured")
	}
}

func TestDownloadFile(t *testing.T) {
	content := []byte("test video content")
	hasher := sha256.New()
	hasher.Write(content)
	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	item := ManifestItem{
		ID:            "test-id",
		Filename:      "test.mp4",
		FileSizeBytes: int64(len(content)),
		SHA256:        expectedHash,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/api/devicesync/" + item.ID
		if r.URL.Path != expectedPath {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}

		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if _, err := w.Write(content); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	oldToken := DeviceAuthToken
	CoreAPIBase = server.URL
	DeviceAuthToken = "test-token"
	defer func() {
		CoreAPIBase = oldBase
		DeviceAuthToken = oldToken
	}()

	tempDir := t.TempDir()
	destPath := filepath.Join(tempDir, item.Filename)

	ctx := context.Background()
	if err := downloadFile(ctx, item, destPath); err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}

	// Verify file was created
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}

	if string(data) != string(content) {
		t.Errorf("content mismatch: expected %q, got %q", content, data)
	}
}

func TestDownloadFile_SizeMismatch(t *testing.T) {
	content := []byte("test content")
	hasher := sha256.New()
	hasher.Write(content)
	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	item := ManifestItem{
		ID:            "test-id",
		Filename:      "test.mp4",
		FileSizeBytes: 999, // Wrong size
		SHA256:        expectedHash,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(content); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	tempDir := t.TempDir()
	destPath := filepath.Join(tempDir, item.Filename)

	ctx := context.Background()
	err := downloadFile(ctx, item, destPath)
	if err == nil {
		t.Error("expected error for size mismatch")
	}
	if !strings.Contains(err.Error(), "size mismatch") {
		t.Errorf("expected size mismatch error, got: %v", err)
	}
}

func TestDownloadFile_HashMismatch(t *testing.T) {
	content := []byte("test content")

	item := ManifestItem{
		ID:            "test-id",
		Filename:      "test.mp4",
		FileSizeBytes: int64(len(content)),
		SHA256:        "wronghash123",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(content); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	tempDir := t.TempDir()
	destPath := filepath.Join(tempDir, item.Filename)

	ctx := context.Background()
	err := downloadFile(ctx, item, destPath)
	if err == nil {
		t.Error("expected error for hash mismatch")
	}
	if !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Errorf("expected SHA256 mismatch error, got: %v", err)
	}
}

func TestVerifyLocalFile(t *testing.T) {
	content := []byte("test content for verification")
	hasher := sha256.New()
	hasher.Write(content)
	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.mp4")

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	item := ManifestItem{
		Filename:      "test.mp4",
		FileSizeBytes: int64(len(content)),
		SHA256:        expectedHash,
	}

	if !verifyLocalFile(testFile, item) {
		t.Error("verification should pass for matching file")
	}

	// Test with wrong size
	itemWrongSize := item
	itemWrongSize.FileSizeBytes = 999
	if verifyLocalFile(testFile, itemWrongSize) {
		t.Error("verification should fail for size mismatch")
	}

	// Test with wrong hash
	itemWrongHash := item
	itemWrongHash.SHA256 = "wronghash"
	if verifyLocalFile(testFile, itemWrongHash) {
		t.Error("verification should fail for hash mismatch")
	}

	// Test with nonexistent file
	if verifyLocalFile(filepath.Join(tempDir, "nonexistent.mp4"), item) {
		t.Error("verification should fail for nonexistent file")
	}
}

func TestSyncStatus(t *testing.T) {
	// Test getting and setting sync status
	status := SyncStatus{
		LastSyncTime: time.Now(),
		OK:           true,
	}

	setSyncStatus(status)
	retrieved := GetSyncStatus()

	if !retrieved.OK {
		t.Error("expected OK=true")
	}

	if retrieved.LastSyncTime.IsZero() {
		t.Error("expected non-zero LastSyncTime")
	}

	// Test with error
	statusWithError := SyncStatus{
		LastSyncTime: time.Now(),
		OK:           false,
		Error:        "test error",
	}

	setSyncStatus(statusWithError)
	retrieved = GetSyncStatus()

	if retrieved.OK {
		t.Error("expected OK=false")
	}

	if retrieved.Error != "test error" {
		t.Errorf("expected error 'test error', got %q", retrieved.Error)
	}
}

func TestTriggerSync_AlreadyInProgress(t *testing.T) {
	// Lock the mutex to simulate sync in progress
	syncInProgressMutex.Lock()
	defer syncInProgressMutex.Unlock()

	ctx := context.Background()
	err := TriggerSync(ctx)
	if err == nil {
		t.Error("expected error when sync already in progress")
	}
	if !strings.Contains(err.Error(), "already in progress") {
		t.Errorf("expected 'already in progress' error, got: %v", err)
	}
}

func TestSyncSchedulerStartStop(t *testing.T) {
	// Set up temporary schedule file
	tempDir := t.TempDir()
	oldPath := syncSchedulePath
	syncSchedulePath = filepath.Join(tempDir, "sync.schedule.json")
	defer func() { syncSchedulePath = oldPath }()

	// Set empty schedule initially
	if err := SetSyncSchedule([]string{}); err != nil {
		t.Fatalf("SetSyncSchedule failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start scheduler
	StartSyncScheduler(ctx)

	// Wait a bit for scheduler to start
	time.Sleep(100 * time.Millisecond)

	if !IsSyncSchedulerRunning() {
		t.Error("scheduler should be running")
	}

	// Start again should be no-op
	StartSyncScheduler(ctx)

	// Stop scheduler
	StopSyncScheduler()

	// Wait for scheduler to stop
	time.Sleep(100 * time.Millisecond)

	if IsSyncSchedulerRunning() {
		t.Error("scheduler should be stopped")
	}
}

func TestSyncSchedulerWithContext(t *testing.T) {
	// Set up temporary schedule file
	tempDir := t.TempDir()
	oldPath := syncSchedulePath
	syncSchedulePath = filepath.Join(tempDir, "sync.schedule.json")
	defer func() { syncSchedulePath = oldPath }()

	if err := SetSyncSchedule([]string{}); err != nil {
		t.Fatalf("SetSyncSchedule failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	StartSyncScheduler(ctx)
	time.Sleep(50 * time.Millisecond)

	if !IsSyncSchedulerRunning() {
		t.Error("scheduler should be running")
	}

	// Cancel context should stop scheduler
	cancel()
	time.Sleep(100 * time.Millisecond)

	if IsSyncSchedulerRunning() {
		t.Error("scheduler should stop when context is cancelled")
	}
}
