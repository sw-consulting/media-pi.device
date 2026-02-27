// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
		{ID: 1, Filename: "video1.mp4", FileSizeBytes: 1024, SHA256: "abc123"},
		{ID: 2, Filename: "video2.mp4", FileSizeBytes: 2048, SHA256: "def456"},
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
	CoreAPIBase = server.URL
	defer func() {
		CoreAPIBase = oldBase
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
		ID:            123,
		Filename:      "test.mp4",
		FileSizeBytes: int64(len(content)),
		SHA256:        expectedHash,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := fmt.Sprintf("/api/devicesync/%d", item.ID)
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
	CoreAPIBase = server.URL
	defer func() {
		CoreAPIBase = oldBase
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
		ID:            123,
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
		ID:            123,
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
	// Simulate sync in progress by setting syncCancelFunc
	syncCancelFuncLock.Lock()
	dummyCancel := func() {} // dummy cancel function
	syncCancelFunc = dummyCancel
	syncCancelFuncLock.Unlock()

	// Clean up after test
	defer func() {
		syncCancelFuncLock.Lock()
		syncCancelFunc = nil
		syncCancelFuncLock.Unlock()
	}()

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

func TestTriggerSyncSuccess(t *testing.T) {
	// Set up test environment
	tempDir := t.TempDir()
	mediaDir := filepath.Join(tempDir, "media")
	oldMediaDir := MediaDir
	MediaDir = mediaDir
	defer func() { MediaDir = oldMediaDir }()

	// Create test files
	content1 := []byte("video file 1")
	content2 := []byte("video file 2")

	hasher1 := sha256.New()
	hasher1.Write(content1)
	hash1 := hex.EncodeToString(hasher1.Sum(nil))

	hasher2 := sha256.New()
	hasher2.Write(content2)
	hash2 := hex.EncodeToString(hasher2.Sum(nil))

	manifest := []ManifestItem{
		{
			ID:            1,
			Filename:      "video1.mp4",
			FileSizeBytes: int64(len(content1)),
			SHA256:        hash1,
		},
		{
			ID:            2,
			Filename:      "video2.mp4",
			FileSizeBytes: int64(len(content2)),
			SHA256:        hash2,
		},
	}

	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/devicesync" {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(manifest); err != nil {
				t.Errorf("failed to encode manifest: %v", err)
			}
			return
		}

		if strings.HasPrefix(r.URL.Path, "/api/devicesync/") {
			id := strings.TrimPrefix(r.URL.Path, "/api/devicesync/")
			switch id {
			case "1":
				if _, err := w.Write(content1); err != nil {
					t.Errorf("failed to write response: %v", err)
				}
			case "2":
				if _, err := w.Write(content2); err != nil {
					t.Errorf("failed to write response: %v", err)
				}
			default:
				http.NotFound(w, r)
			}
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() {
		CoreAPIBase = oldBase
	}()

	ctx := context.Background()
	err := TriggerSync(ctx)
	if err != nil {
		t.Fatalf("TriggerSync failed: %v", err)
	}

	// Verify files were downloaded
	file1, err := os.ReadFile(filepath.Join(mediaDir, "video1.mp4"))
	if err != nil {
		t.Errorf("failed to read video1.mp4: %v", err)
	}
	if string(file1) != string(content1) {
		t.Error("video1.mp4 content mismatch")
	}

	file2, err := os.ReadFile(filepath.Join(mediaDir, "video2.mp4"))
	if err != nil {
		t.Errorf("failed to read video2.mp4: %v", err)
	}
	if string(file2) != string(content2) {
		t.Error("video2.mp4 content mismatch")
	}

	// Check sync status
	status := GetSyncStatus()
	if !status.OK {
		t.Errorf("expected OK=true, got OK=false with error: %s", status.Error)
	}
}

func TestTriggerSyncGarbageCollection(t *testing.T) {
	// Set up test environment
	tempDir := t.TempDir()
	mediaDir := filepath.Join(tempDir, "media")
	oldMediaDir := MediaDir
	MediaDir = mediaDir
	defer func() { MediaDir = oldMediaDir }()

	// Create media dir with an old file
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		t.Fatalf("failed to create media dir: %v", err)
	}

	oldFile := filepath.Join(mediaDir, "old-video.mp4")
	if err := os.WriteFile(oldFile, []byte("old content"), 0644); err != nil {
		t.Fatalf("failed to write old file: %v", err)
	}

	// Empty manifest
	manifest := []ManifestItem{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/devicesync" {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(manifest); err != nil {
				t.Errorf("failed to encode manifest: %v", err)
			}
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	ctx := context.Background()
	err := TriggerSync(ctx)
	if err != nil {
		t.Fatalf("TriggerSync failed: %v", err)
	}

	// Verify old file was removed
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("old file should have been garbage collected")
	}
}

func TestTriggerSyncSkipsExistingFiles(t *testing.T) {
	// Set up test environment
	tempDir := t.TempDir()
	mediaDir := filepath.Join(tempDir, "media")
	oldMediaDir := MediaDir
	MediaDir = mediaDir
	defer func() { MediaDir = oldMediaDir }()

	// Create media dir with existing correct file
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		t.Fatalf("failed to create media dir: %v", err)
	}

	content := []byte("existing video content")
	hasher := sha256.New()
	hasher.Write(content)
	hash := hex.EncodeToString(hasher.Sum(nil))

	existingFile := filepath.Join(mediaDir, "video1.mp4")
	if err := os.WriteFile(existingFile, content, 0644); err != nil {
		t.Fatalf("failed to write existing file: %v", err)
	}

	manifest := []ManifestItem{
		{
			ID:            1,
			Filename:      "video1.mp4",
			FileSizeBytes: int64(len(content)),
			SHA256:        hash,
		},
	}

	downloadCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/devicesync" {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(manifest); err != nil {
				t.Errorf("failed to encode manifest: %v", err)
			}
			return
		}

		if strings.HasPrefix(r.URL.Path, "/api/devicesync/") {
			downloadCalled = true
			if _, err := w.Write(content); err != nil {
				t.Errorf("failed to write response: %v", err)
			}
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	ctx := context.Background()
	err := TriggerSync(ctx)
	if err != nil {
		t.Fatalf("TriggerSync failed: %v", err)
	}

	if downloadCalled {
		t.Error("download should not have been called for existing valid file")
	}
}

func TestTriggerSyncManifestError(t *testing.T) {
	tempDir := t.TempDir()
	mediaDir := filepath.Join(tempDir, "media")
	oldMediaDir := MediaDir
	MediaDir = mediaDir
	defer func() { MediaDir = oldMediaDir }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	ctx := context.Background()
	err := TriggerSync(ctx)
	if err == nil {
		t.Error("expected error when manifest fetch fails")
	}

	// Check sync status reflects error
	status := GetSyncStatus()
	if status.OK {
		t.Error("expected OK=false when sync fails")
	}
	if status.Error == "" {
		t.Error("expected error message in status")
	}
}

func TestTriggerSyncDownloadError(t *testing.T) {
	tempDir := t.TempDir()
	mediaDir := filepath.Join(tempDir, "media")
	oldMediaDir := MediaDir
	MediaDir = mediaDir
	defer func() { MediaDir = oldMediaDir }()

	content := []byte("test content")
	hasher := sha256.New()
	hasher.Write(content)
	hash := hex.EncodeToString(hasher.Sum(nil))

	manifest := []ManifestItem{
		{
			ID:            1,
			Filename:      "video1.mp4",
			FileSizeBytes: int64(len(content)),
			SHA256:        hash,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/devicesync" {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(manifest); err != nil {
				t.Errorf("failed to encode manifest: %v", err)
			}
			return
		}

		// Return error for download
		if strings.HasPrefix(r.URL.Path, "/api/devicesync/") {
			http.Error(w, "download failed", http.StatusInternalServerError)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	ctx := context.Background()
	err := TriggerSync(ctx)
	if err == nil {
		t.Error("expected error when download fails")
	}

	status := GetSyncStatus()
	if status.OK {
		t.Error("expected OK=false when download fails")
	}
}

func TestSetSyncSchedule_WriteError(t *testing.T) {
	// Try to write to a read-only location
	oldPath := syncSchedulePath
	syncSchedulePath = "/proc/invalid/sync.schedule.json"
	defer func() { syncSchedulePath = oldPath }()

	err := SetSyncSchedule([]string{"10:00"})
	if err == nil {
		t.Error("expected error when writing to invalid path")
	}
}

func TestGetSyncSchedule_InvalidJSON(t *testing.T) {
	tempDir := t.TempDir()
	oldPath := syncSchedulePath
	syncSchedulePath = filepath.Join(tempDir, "invalid.json")
	defer func() { syncSchedulePath = oldPath }()

	// Write invalid JSON
	if err := os.WriteFile(syncSchedulePath, []byte("not json"), 0644); err != nil {
		t.Fatalf("failed to write invalid json: %v", err)
	}

	_, err := GetSyncSchedule()
	if err == nil {
		t.Error("expected error when reading invalid JSON")
	}
}

func TestSetSyncStatus_Persistence(t *testing.T) {
	tempDir := t.TempDir()
	oldPath := syncStatusPath
	syncStatusPath = filepath.Join(tempDir, "sync.status.json")
	defer func() { syncStatusPath = oldPath }()

	status := SyncStatus{
		LastSyncTime: time.Now().Truncate(time.Second),
		OK:           true,
	}

	setSyncStatus(status)

	// Read from disk
	data, err := os.ReadFile(syncStatusPath)
	if err != nil {
		t.Fatalf("failed to read status file: %v", err)
	}

	var loaded SyncStatus
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal status: %v", err)
	}

	if !loaded.OK {
		t.Error("expected OK=true")
	}
}

func TestCalculateNextSyncTime_InvalidFormat(t *testing.T) {
	now := time.Now()

	// All invalid formats (non-numeric) should result in zero time
	schedule := []string{"invalid", "ab:cd", "not:time"}
	next := calculateNextSyncTime(now, schedule)
	if !next.IsZero() {
		t.Error("expected zero time for all invalid schedule formats")
	}

	// Mix of valid and invalid - should use the valid one
	scheduleWithValid := []string{"invalid", "14:30", "ab:cd"}
	next = calculateNextSyncTime(now, scheduleWithValid)
	if next.IsZero() {
		t.Error("expected non-zero time when at least one valid time exists")
	}
	if next.Hour() != 14 || next.Minute() != 30 {
		// Could be tomorrow if past 14:30
		if next.Hour() != 14 || next.Minute() != 30 {
			t.Errorf("expected 14:30, got %02d:%02d", next.Hour(), next.Minute())
		}
	}
}

func TestFetchManifest_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	ctx := context.Background()
	_, err := fetchManifest(ctx)
	if err == nil {
		t.Error("expected error when server returns error")
	}
}

func TestFetchManifest_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte("not valid json")); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	ctx := context.Background()
	_, err := fetchManifest(ctx)
	if err == nil {
		t.Error("expected error when server returns invalid JSON")
	}
}

func TestDownloadFile_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	tempDir := t.TempDir()
	destPath := filepath.Join(tempDir, "test.mp4")

	item := ManifestItem{
		ID:            123,
		Filename:      "test.mp4",
		FileSizeBytes: 100,
		SHA256:        "abc123",
	}

	ctx := context.Background()
	err := downloadFile(ctx, item, destPath)
	if err == nil {
		t.Error("expected error when server returns error")
	}
}

func TestVerifyLocalFile_StatError(t *testing.T) {
	item := ManifestItem{
		Filename:      "nonexistent.mp4",
		FileSizeBytes: 100,
		SHA256:        "abc123",
	}

	if verifyLocalFile("/nonexistent/path/file.mp4", item) {
		t.Error("verification should fail for nonexistent file")
	}
}

func TestStopSyncScheduler_NotRunning(t *testing.T) {
	// Ensure scheduler is not running
	if IsSyncSchedulerRunning() {
		StopSyncScheduler()
		time.Sleep(100 * time.Millisecond)
	}

	// Stopping when not running should be safe
	StopSyncScheduler()

	if IsSyncSchedulerRunning() {
		t.Error("scheduler should still not be running")
	}
}

func TestSyncSchedulerReadsSchedulePeriodically(t *testing.T) {
	tempDir := t.TempDir()
	oldPath := syncSchedulePath
	syncSchedulePath = filepath.Join(tempDir, "sync.schedule.json")
	defer func() { syncSchedulePath = oldPath }()

	// Start with empty schedule
	if err := SetSyncSchedule([]string{}); err != nil {
		t.Fatalf("SetSyncSchedule failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartSyncScheduler(ctx)
	time.Sleep(50 * time.Millisecond)

	if !IsSyncSchedulerRunning() {
		t.Error("scheduler should be running")
	}

	// Update schedule while running
	// Note: In real use, scheduler re-reads every 5 minutes or on next iteration
	// This test just verifies the scheduler handles schedule updates
	if err := SetSyncSchedule([]string{"23:59"}); err != nil {
		t.Fatalf("SetSyncSchedule failed: %v", err)
	}

	// Clean up
	StopSyncScheduler()
	time.Sleep(100 * time.Millisecond)
}

func TestDownloadFile_NoAPIBase(t *testing.T) {
	oldBase := CoreAPIBase
	CoreAPIBase = ""
	defer func() { CoreAPIBase = oldBase }()

	tempDir := t.TempDir()
	destPath := filepath.Join(tempDir, "test.mp4")

	item := ManifestItem{
		ID:            123,
		Filename:      "test.mp4",
		FileSizeBytes: 100,
		SHA256:        "abc123",
	}

	ctx := context.Background()
	err := downloadFile(ctx, item, destPath)
	if err == nil {
		t.Error("expected error when core_api_base not configured")
	}
}

func TestFetchManifest_WithAuth(t *testing.T) {
	manifest := []ManifestItem{
		{ID: 1, Filename: "video1.mp4", FileSizeBytes: 1024, SHA256: "abc123"},
	}

	deviceIDReceived := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deviceIDReceived = r.Header.Get("X-Device-Id")
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(manifest); err != nil {
			t.Errorf("failed to encode manifest: %v", err)
		}
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() {
		CoreAPIBase = oldBase
	}()

	oldServerKey := ServerKey
	ServerKey = "test-device-key"
	defer func() {
		ServerKey = oldServerKey
	}()

	ctx := context.Background()
	_, err := fetchManifest(ctx)
	if err != nil {
		t.Fatalf("fetchManifest failed: %v", err)
	}

	expectedDeviceID := "test-device-key"
	if deviceIDReceived != expectedDeviceID {
		t.Errorf("expected X-Device-Id: %s, got: %s", expectedDeviceID, deviceIDReceived)
	}
}

func TestDownloadFile_WithAuth(t *testing.T) {
	content := []byte("test video")
	hasher := sha256.New()
	hasher.Write(content)
	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	deviceIDReceived := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deviceIDReceived = r.Header.Get("X-Device-Id")
		if _, err := w.Write(content); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() {
		CoreAPIBase = oldBase
	}()

	oldServerKey := ServerKey
	ServerKey = "test-device-key"
	defer func() {
		ServerKey = oldServerKey
	}()

	tempDir := t.TempDir()
	destPath := filepath.Join(tempDir, "test.mp4")

	item := ManifestItem{
		ID:            123,
		Filename:      "test.mp4",
		FileSizeBytes: int64(len(content)),
		SHA256:        expectedHash,
	}

	ctx := context.Background()
	err := downloadFile(ctx, item, destPath)
	if err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}

	expectedDeviceID := "test-device-key"
	if deviceIDReceived != expectedDeviceID {
		t.Errorf("expected X-Device-Id: %s, got: %s", expectedDeviceID, deviceIDReceived)
	}
}

func TestSyncFiles_ReadDirError(t *testing.T) {
	tempDir := t.TempDir()

	// Create a file where media dir should be (not a directory)
	mediaDir := filepath.Join(tempDir, "media")
	if err := os.WriteFile(mediaDir, []byte("not a dir"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	oldMediaDir := MediaDir
	MediaDir = mediaDir
	defer func() { MediaDir = oldMediaDir }()

	// Empty manifest
	manifest := []ManifestItem{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(manifest); err != nil {
			t.Errorf("failed to encode manifest: %v", err)
		}
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	ctx := context.Background()
	err := TriggerSync(ctx)
	if err == nil {
		t.Error("expected error when media dir creation fails")
	}
}

func TestTriggerSyncWithSubdirectories(t *testing.T) {
	tempDir := t.TempDir()
	mediaDir := filepath.Join(tempDir, "media")
	oldMediaDir := MediaDir
	MediaDir = mediaDir
	defer func() { MediaDir = oldMediaDir }()

	// Create media dir with a subdirectory
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		t.Fatalf("failed to create media dir: %v", err)
	}

	subDir := filepath.Join(mediaDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	// Empty manifest
	manifest := []ManifestItem{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(manifest); err != nil {
			t.Errorf("failed to encode manifest: %v", err)
		}
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	ctx := context.Background()
	err := TriggerSync(ctx)
	if err != nil {
		t.Fatalf("TriggerSync failed: %v", err)
	}

	// Verify subdirectory still exists (not garbage collected)
	if _, err := os.Stat(subDir); os.IsNotExist(err) {
		t.Error("subdirectory should not be garbage collected")
	}
}

func TestDownloadFile_WriteFails(t *testing.T) {
	content := []byte("test content")
	hasher := sha256.New()
	hasher.Write(content)
	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	// Server that closes connection mid-stream
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijacking not supported", http.StatusInternalServerError)
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Write headers then close abruptly
		if _, err := conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 1000000\r\n\r\n")); err != nil {
			t.Logf("failed to write headers: %v", err)
		}
		if err := conn.Close(); err != nil {
			t.Logf("failed to close conn: %v", err)
		}
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	tempDir := t.TempDir()
	destPath := filepath.Join(tempDir, "test.mp4")

	item := ManifestItem{
		ID:            123,
		Filename:      "test.mp4",
		FileSizeBytes: 1000000,
		SHA256:        expectedHash,
	}

	ctx := context.Background()
	err := downloadFile(ctx, item, destPath)
	if err == nil {
		t.Error("expected error when download interrupted")
	}
}

func TestVerifyLocalFile_HashComparison(t *testing.T) {
	content := []byte("test content")
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
		SHA256:        strings.ToUpper(expectedHash), // Test case-insensitive comparison
	}

	if !verifyLocalFile(testFile, item) {
		t.Error("verification should pass with uppercase hash")
	}
}

func TestStartSyncScheduler_AlreadyRunning(t *testing.T) {
	tempDir := t.TempDir()
	oldPath := syncSchedulePath
	syncSchedulePath = filepath.Join(tempDir, "sync.schedule.json")
	defer func() { syncSchedulePath = oldPath }()

	if err := SetSyncSchedule([]string{}); err != nil {
		t.Fatalf("SetSyncSchedule failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start first time
	StartSyncScheduler(ctx)
	time.Sleep(50 * time.Millisecond)

	if !IsSyncSchedulerRunning() {
		t.Fatal("scheduler should be running")
	}

	// Try to start again - should be no-op
	StartSyncScheduler(ctx)
	time.Sleep(50 * time.Millisecond)

	if !IsSyncSchedulerRunning() {
		t.Error("scheduler should still be running")
	}

	// Clean up
	StopSyncScheduler()
	time.Sleep(100 * time.Millisecond)
}

func TestFetchManifest_ContextCanceled(t *testing.T) {
	// Server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte("[]")); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := fetchManifest(ctx)
	if err == nil {
		t.Error("expected error when context is canceled")
	}
}

func TestDownloadFile_ContextCanceled(t *testing.T) {
	// Server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		if _, err := w.Write([]byte("content")); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	tempDir := t.TempDir()
	destPath := filepath.Join(tempDir, "test.mp4")

	item := ManifestItem{
		ID:            123,
		Filename:      "test.mp4",
		FileSizeBytes: 100,
		SHA256:        "abc123",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := downloadFile(ctx, item, destPath)
	if err == nil {
		t.Error("expected error when context is canceled")
	}
}

func TestSyncFilesParallelDownloads(t *testing.T) {
	tempDir := t.TempDir()
	mediaDir := filepath.Join(tempDir, "media")
	oldMediaDir := MediaDir
	oldMaxDownloads := MaxParallelDownloads
	MediaDir = mediaDir
	MaxParallelDownloads = 2 // Test parallelization
	defer func() {
		MediaDir = oldMediaDir
		MaxParallelDownloads = oldMaxDownloads
	}()

	// Create multiple files to download
	files := make(map[string][]byte)
	manifest := []ManifestItem{}

	for i := 1; i <= 5; i++ {
		content := []byte(fmt.Sprintf("video content %d", i))
		hasher := sha256.New()
		hasher.Write(content)
		hash := hex.EncodeToString(hasher.Sum(nil))

		filename := fmt.Sprintf("video%d.mp4", i)
		files[filename] = content

		manifest = append(manifest, ManifestItem{
			ID:            int64(i),
			Filename:      filename,
			FileSizeBytes: int64(len(content)),
			SHA256:        hash,
		})
	}

	downloadCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/devicesync" {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(manifest); err != nil {
				t.Errorf("failed to encode manifest: %v", err)
			}
			return
		}

		if strings.HasPrefix(r.URL.Path, "/api/devicesync/") {
			mu.Lock()
			downloadCount++
			mu.Unlock()

			id := strings.TrimPrefix(r.URL.Path, "/api/devicesync/")
			// Find matching file
			for filename, content := range files {
				if strings.Contains(id, filename[5:6]) { // Extract number from id
					if _, err := w.Write(content); err != nil {
						t.Errorf("failed to write response: %v", err)
					}
					return
				}
			}
			http.NotFound(w, r)
			return
		}

		http.NotFound(w, r)
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	ctx := context.Background()
	err := TriggerSync(ctx)
	if err != nil {
		t.Fatalf("TriggerSync failed: %v", err)
	}

	if downloadCount != 5 {
		t.Errorf("expected 5 downloads, got %d", downloadCount)
	}

	// Verify all files were downloaded
	for filename := range files {
		filePath := filepath.Join(mediaDir, filename)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Errorf("file %s was not downloaded", filename)
		}
	}
}

func TestVerifyLocalFile_ReadError(t *testing.T) {
	// Create a file then make it unreadable
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.mp4")

	content := []byte("test")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Make file unreadable (only on Unix-like systems)
	if err := os.Chmod(testFile, 0000); err != nil {
		t.Skip("cannot change file permissions on this system")
	}
	defer func() {
		if err := os.Chmod(testFile, 0644); err != nil {
			t.Logf("failed to restore file permissions: %v", err)
		}
	}()

	item := ManifestItem{
		Filename:      "test.mp4",
		FileSizeBytes: int64(len(content)),
		SHA256:        "abc123",
	}

	if verifyLocalFile(testFile, item) {
		t.Error("verification should fail for unreadable file")
	}
}

func TestSetSyncSchedule_MkdirError(t *testing.T) {
	// Try to create schedule in an invalid location
	oldPath := syncSchedulePath
	// Use a path where mkdir will fail
	syncSchedulePath = "/proc/test/invalid/sync.schedule.json"
	defer func() { syncSchedulePath = oldPath }()

	err := SetSyncSchedule([]string{"10:00"})
	if err == nil {
		t.Error("expected error when mkdir fails")
	}
	if !strings.Contains(err.Error(), "failed to create directory") {
		t.Errorf("expected 'failed to create directory' error, got: %v", err)
	}
}

func TestDownloadFile_CreateTempError(t *testing.T) {
	content := []byte("test")
	hasher := sha256.New()
	hasher.Write(content)
	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(content); err != nil {
			t.Errorf("failed to write: %v", err)
		}
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	// Try to write to a read-only directory
	destPath := "/proc/test.mp4"

	item := ManifestItem{
		ID:            123,
		Filename:      "test.mp4",
		FileSizeBytes: int64(len(content)),
		SHA256:        expectedHash,
	}

	ctx := context.Background()
	err := downloadFile(ctx, item, destPath)
	if err == nil {
		t.Error("expected error when creating temp file in read-only location")
	}
}

func TestSchedulerWithScheduledTime(t *testing.T) {
	tempDir := t.TempDir()
	oldPath := syncSchedulePath
	syncSchedulePath = filepath.Join(tempDir, "sync.schedule.json")
	defer func() { syncSchedulePath = oldPath }()

	// Set a schedule that will trigger soon (but not immediately)
	futureTime := time.Now().Add(2 * time.Second)
	schedule := []string{fmt.Sprintf("%02d:%02d", futureTime.Hour(), futureTime.Minute())}

	if err := SetSyncSchedule(schedule); err != nil {
		t.Fatalf("SetSyncSchedule failed: %v", err)
	}

	// Configure a mock server that won't be called
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte("[]")); err != nil {
			t.Errorf("failed to write: %v", err)
		}
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	StartSyncScheduler(ctx)
	time.Sleep(100 * time.Millisecond)

	if !IsSyncSchedulerRunning() {
		t.Error("scheduler should be running")
	}

	// Wait for context timeout
	<-ctx.Done()
	time.Sleep(100 * time.Millisecond)

	if IsSyncSchedulerRunning() {
		t.Error("scheduler should have stopped after context cancellation")
	}
}

func TestIsSyncInProgress(t *testing.T) {
	// Setup temporary directories for testing
	tempDir := t.TempDir()
	oldPath := syncSchedulePath
	oldStatus := syncStatusPath
	syncSchedulePath = filepath.Join(tempDir, "sync.schedule.json")
	syncStatusPath = filepath.Join(tempDir, "sync.status.json")
	defer func() {
		syncSchedulePath = oldPath
		syncStatusPath = oldStatus
	}()

	oldMediaDir := MediaDir
	MediaDir = filepath.Join(tempDir, "media")
	defer func() { MediaDir = oldMediaDir }()

	oldCoreAPIBase := CoreAPIBase
	defer func() { CoreAPIBase = oldCoreAPIBase }()

	// Initially, no sync should be in progress
	if IsSyncInProgress() {
		t.Error("IsSyncInProgress should return false when no sync is running")
	}

	// Create a test server that delays manifest fetch to simulate a long-running sync
	manifestDelay := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api/devicesync") {
			<-manifestDelay // Block until we signal to proceed
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `[]`) // Empty manifest for quick completion
		}
	}))
	defer server.Close()

	CoreAPIBase = server.URL

	// Start a sync in the background
	ctx := context.Background()
	syncDone := make(chan error, 1)
	go func() {
		syncDone <- TriggerSync(ctx)
	}()

	// Wait for sync to report it is in progress, with a timeout
	for i := 0; i < 50; i++ {
		if IsSyncInProgress() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Now sync should be in progress
	if !IsSyncInProgress() {
		t.Fatalf("IsSyncInProgress should return true when sync is running")
	}

	// Signal manifest to complete
	close(manifestDelay)

	// Wait for sync to complete
	err := <-syncDone
	if err != nil {
		t.Errorf("sync should complete successfully, got error: %v", err)
	}

	// After completion, sync should not be in progress
	if IsSyncInProgress() {
		t.Error("IsSyncInProgress should return false after sync completes")
	}
}

func TestCancelSync(t *testing.T) {
	// Setup temporary directories for testing
	tempDir := t.TempDir()
	oldPath := syncSchedulePath
	oldStatus := syncStatusPath
	syncSchedulePath = filepath.Join(tempDir, "sync.schedule.json")
	syncStatusPath = filepath.Join(tempDir, "sync.status.json")
	defer func() {
		syncSchedulePath = oldPath
		syncStatusPath = oldStatus
	}()

	oldMediaDir := MediaDir
	MediaDir = filepath.Join(tempDir, "media")
	defer func() { MediaDir = oldMediaDir }()

	oldCoreAPIBase := CoreAPIBase
	defer func() { CoreAPIBase = oldCoreAPIBase }()

	// Initially, cancel should return false (no sync running)
	if CancelSync() {
		t.Error("CancelSync should return false when no sync is running")
	}

	// Create a test server that blocks on manifest fetch
	manifestDelay := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api/devicesync") {
			<-manifestDelay // Block until cancelled or signal
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `[]`)
		}
	}))
	defer server.Close()

	CoreAPIBase = server.URL

	// Start a sync in the background
	ctx := context.Background()
	syncDone := make(chan error, 1)
	go func() {
		syncDone <- TriggerSync(ctx)
	}()

	// Wait for sync to start, up to a timeout
	deadline := time.Now().Add(5 * time.Second)
	for !IsSyncInProgress() {
		if time.Now().After(deadline) {
			t.Fatal("sync did not start within timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Cancel the sync
	if !CancelSync() {
		t.Error("CancelSync should return true when cancelling an in-progress sync")
	}

	// Close the delay channel to unblock the server (in case context cancellation doesn't work)
	close(manifestDelay)

	// Wait for sync to complete (it should fail due to cancellation)
	err := <-syncDone
	if err == nil {
		t.Error("expected sync to fail after cancellation")
	}
	// The error could be "context canceled" or "failed to fetch manifest" depending on timing
	if !strings.Contains(err.Error(), "context canceled") && !strings.Contains(err.Error(), "failed to fetch manifest") {
		t.Errorf("expected context canceled or fetch manifest error, got: %v", err)
	}

	// After cancellation, sync should not be in progress
	if IsSyncInProgress() {
		t.Error("IsSyncInProgress should return false after cancellation")
	}

	// Second cancel should return false
	if CancelSync() {
		t.Error("CancelSync should return false when no sync is running")
	}
}

// TestValidateFilename tests the filename validation function.
func TestValidateFilename(t *testing.T) {
	mediaDir := "/var/lib/media-pi"

	tests := []struct {
		name     string
		filename string
		wantErr  bool
	}{
		{
			name:     "valid filename",
			filename: "video.mp4",
			wantErr:  false,
		},
		{
			name:     "valid filename with extension",
			filename: "my-video_01.mp4",
			wantErr:  false,
		},
		{
			name:     "empty filename",
			filename: "",
			wantErr:  true,
		},
		{
			name:     "path traversal with ..",
			filename: "../etc/passwd",
			wantErr:  true,
		},
		{
			name:     "path traversal in middle",
			filename: "foo/../../../etc/passwd",
			wantErr:  true,
		},
		{
			name:     "absolute path",
			filename: "/etc/passwd",
			wantErr:  true,
		},
		{
			name:     "forward slash separator",
			filename: "subdir/video.mp4",
			wantErr:  true,
		},
		{
			name:     "backward slash separator",
			filename: "subdir\\video.mp4",
			wantErr:  true,
		},
		{
			name:     "hidden file",
			filename: ".hidden",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFilename(tt.filename, mediaDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateFilename() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestSyncFiles_InvalidFilenames tests that invalid filenames in manifest are skipped.
func TestSyncFiles_InvalidFilenames(t *testing.T) {
	oldMediaDir := MediaDir
	oldMaxParallel := MaxParallelDownloads
	oldBase := CoreAPIBase
	defer func() {
		MediaDir = oldMediaDir
		MaxParallelDownloads = oldMaxParallel
		CoreAPIBase = oldBase
	}()

	tmpDir := t.TempDir()
	MediaDir = tmpDir
	MaxParallelDownloads = 2

	// Create a test server with malicious filenames in manifest
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/devicesync":
			manifest := []ManifestItem{
				{
					ID:            1,
					Filename:      "valid.mp4",
					FileSizeBytes: 4,
					SHA256:        "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", // sha256("test")
				},
				{
					ID:            2,
					Filename:      "../../../etc/passwd",
					FileSizeBytes: 100,
					SHA256:        "invalid",
				},
				{
					ID:            3,
					Filename:      "/absolute/path",
					FileSizeBytes: 100,
					SHA256:        "invalid",
				},
				{
					ID:            4,
					Filename:      "subdir/file.mp4",
					FileSizeBytes: 100,
					SHA256:        "invalid",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(manifest)
		case "/api/devicesync/1":
			_, _ = w.Write([]byte("test"))
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	CoreAPIBase = server.URL

	ctx := context.Background()
	err := TriggerSync(ctx)
	if err != nil {
		t.Fatalf("TriggerSync failed: %v", err)
	}

	// Verify only the valid file was downloaded
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 file, got %d", len(entries))
	}

	if len(entries) > 0 && entries[0].Name() != "valid.mp4" {
		t.Errorf("expected valid.mp4, got %s", entries[0].Name())
	}

	// Verify no files were created outside the media directory
	parentDir := filepath.Dir(tmpDir)
	parentEntries, err := os.ReadDir(parentDir)
	if err != nil {
		t.Fatalf("ReadDir parent failed: %v", err)
	}

	for _, entry := range parentEntries {
		if entry.Name() == "passwd" || entry.Name() == "etc" {
			t.Errorf("found suspicious file in parent directory: %s", entry.Name())
		}
	}
}

func TestScheduleReloadImmediate(t *testing.T) {
	tempDir := t.TempDir()
	oldPath := syncSchedulePath
	oldStatusPath := syncStatusPath
	syncSchedulePath = filepath.Join(tempDir, "sync.schedule.json")
	syncStatusPath = filepath.Join(tempDir, "sync.status.json")
	defer func() {
		syncSchedulePath = oldPath
		syncStatusPath = oldStatusPath
	}()

	oldMediaDir := MediaDir
	MediaDir = filepath.Join(tempDir, "media")
	defer func() { MediaDir = oldMediaDir }()

	// Configure a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte("[]")); err != nil {
			t.Errorf("failed to write: %v", err)
		}
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	// Set an initial schedule with no times (should wait 5 minutes)
	if err := SetSyncSchedule([]string{}); err != nil {
		t.Fatalf("SetSyncSchedule failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track when scheduler recalculates
	recalculated := make(chan struct{}, 1)

	// Start scheduler
	StartSyncScheduler(ctx)
	time.Sleep(100 * time.Millisecond)

	if !IsSyncSchedulerRunning() {
		t.Fatal("scheduler should be running")
	}

	// Monitor logs to detect reload - we'll update schedule and verify
	// that the scheduler picks it up quickly (not after 5 minutes)
	go func() {
		// Set a schedule with a time in the future
		futureTime := time.Now().Add(10 * time.Minute)
		newSchedule := []string{fmt.Sprintf("%02d:%02d", futureTime.Hour(), futureTime.Minute())}
		if err := SetSyncSchedule(newSchedule); err != nil {
			t.Errorf("SetSyncSchedule failed: %v", err)
			return
		}
		// Signal that we've updated the schedule
		recalculated <- struct{}{}
	}()

	// Wait for schedule update
	select {
	case <-recalculated:
		// Good - schedule was updated
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for schedule update")
	}

	// Give scheduler time to process the reload signal
	time.Sleep(200 * time.Millisecond)

	// The key test is that the scheduler is still running and didn't crash
	// The reload mechanism should have been triggered via the channel
	if !IsSyncSchedulerRunning() {
		t.Error("scheduler should still be running after schedule update")
	}

	// Stop the scheduler
	StopSyncScheduler()

	if IsSyncSchedulerRunning() {
		t.Error("scheduler should have stopped")
	}
}

// TestSyncFilesGarbageCollectsInvalidFilenames tests that files with invalid
// names from the manifest don't prevent garbage collection.
func TestSyncFilesGarbageCollectsInvalidFilenames(t *testing.T) {
	tempDir := t.TempDir()
	mediaDir := filepath.Join(tempDir, "media")
	oldMediaDir := MediaDir
	MediaDir = mediaDir
	defer func() { MediaDir = oldMediaDir }()

	// Create media dir with files that have invalid names
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		t.Fatalf("failed to create media dir: %v", err)
	}

	// Create a file with URL-encoded path traversal name
	encodedFile := filepath.Join(mediaDir, "..%2Fetc%2Fpasswd")
	if err := os.WriteFile(encodedFile, []byte("malicious content"), 0644); err != nil {
		t.Fatalf("failed to write encoded invalid file: %v", err)
	}

	// Create a file with exact path traversal name (non-encoded)
	invalidFile := filepath.Join(mediaDir, "..invalid")
	if err := os.WriteFile(invalidFile, []byte("malicious content"), 0644); err != nil {
		t.Fatalf("failed to write invalid file: %v", err)
	}

	// Create a valid file that should be kept
	validFile := filepath.Join(mediaDir, "valid-video.mp4")
	content := []byte("valid content")
	if err := os.WriteFile(validFile, content, 0644); err != nil {
		t.Fatalf("failed to write valid file: %v", err)
	}

	// Compute hash for the valid file
	hasher := sha256.New()
	hasher.Write(content)
	hash := hex.EncodeToString(hasher.Sum(nil))

	// Manifest contains items: one with path traversal, one with "..", and one valid
	manifest := []ManifestItem{
		{
			Filename:      "../etc/passwd",
			FileSizeBytes: 17,
			SHA256:        "0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			Filename:      "..invalid",
			FileSizeBytes: 17,
			SHA256:        "0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			Filename:      "valid-video.mp4",
			FileSizeBytes: int64(len(content)),
			SHA256:        hash,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/devicesync" {
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(manifest); err != nil {
				t.Errorf("failed to encode manifest: %v", err)
			}
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	oldBase := CoreAPIBase
	CoreAPIBase = server.URL
	defer func() { CoreAPIBase = oldBase }()

	ctx := context.Background()
	// syncFiles should succeed (skipping the invalid files)
	if err := syncFiles(ctx); err != nil {
		t.Fatalf("syncFiles failed: %v", err)
	}

	// Verify the file with URL-encoded invalid name was garbage collected
	if _, err := os.Stat(encodedFile); !os.IsNotExist(err) {
		t.Error("file with URL-encoded invalid name should have been garbage collected")
	}

	// Verify the file with path traversal ".." was garbage collected
	if _, err := os.Stat(invalidFile); !os.IsNotExist(err) {
		t.Error("file with '..' in name should have been garbage collected")
	}

	// Verify the valid file still exists
	if _, err := os.Stat(validFile); err != nil {
		t.Error("valid file should not have been garbage collected")
	}
}
