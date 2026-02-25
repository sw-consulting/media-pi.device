// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mockHTTPClient implements HTTPClientInterface for testing
type mockHTTPClient struct {
	responses map[string]*http.Response
	requests  []*http.Request
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	m.requests = append(m.requests, req)
	
	// Return pre-configured response based on URL
	resp, ok := m.responses[req.URL.Path]
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader("not found")),
		}, nil
	}
	return resp, nil
}

func TestFetchManifest(t *testing.T) {
	// Save original values
	origHTTPClient := HTTPClient
	origCoreAPI := CoreAPIBase
	origAuthToken := DeviceAuthToken
	defer func() {
		HTTPClient = origHTTPClient
		CoreAPIBase = origCoreAPI
		DeviceAuthToken = origAuthToken
	}()

	// Set test values
	CoreAPIBase = "http://test.example.com"
	DeviceAuthToken = "test-token"

	manifest := []ManifestItem{
		{ID: "1", Filename: "video1.mp4", FileSizeBytes: 1024, SHA256: "abc123"},
		{ID: "2", Filename: "video2.mp4", FileSizeBytes: 2048, SHA256: "def456"},
	}
	manifestJSON, _ := json.Marshal(manifest)

	mockClient := &mockHTTPClient{
		responses: map[string]*http.Response{
			"/api/devicesync": {
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(string(manifestJSON))),
			},
		},
	}
	HTTPClient = mockClient

	ctx := context.Background()
	result, err := fetchManifest(ctx)
	if err != nil {
		t.Fatalf("fetchManifest failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result))
	}

	if result[0].Filename != "video1.mp4" {
		t.Errorf("expected video1.mp4, got %s", result[0].Filename)
	}

	// Verify authorization header
	if len(mockClient.requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(mockClient.requests))
	}
	authHeader := mockClient.requests[0].Header.Get("Authorization")
	if authHeader != "Bearer test-token" {
		t.Errorf("expected Bearer test-token, got %s", authHeader)
	}
}

func TestDownloadFileWithVerification(t *testing.T) {
	// Save original values
	origHTTPClient := HTTPClient
	origCoreAPI := CoreAPIBase
	origMediaDir := MediaDir
	defer func() {
		HTTPClient = origHTTPClient
		CoreAPIBase = origCoreAPI
		MediaDir = origMediaDir
	}()

	tmp := t.TempDir()
	MediaDir = tmp
	CoreAPIBase = "http://test.example.com"

	fileContent := "test video content"
	item := ManifestItem{
		ID:            "1",
		Filename:      "test.mp4",
		FileSizeBytes: int64(len(fileContent)),
		SHA256:        "b8e219804f9ca63b0cf1d1422176fa55838d065ac80f710e8aa1e985bf7c11fc", // SHA256 of "test video content"
	}

	mockClient := &mockHTTPClient{
		responses: map[string]*http.Response{
			"/api/devicesync/1": {
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(fileContent)),
			},
		},
	}
	HTTPClient = mockClient

	ctx := context.Background()
	err := downloadFile(ctx, item)
	if err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}

	// Verify file was created
	filePath := filepath.Join(tmp, "test.mp4")
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}

	if string(data) != fileContent {
		t.Errorf("expected %q, got %q", fileContent, string(data))
	}
}

func TestDownloadFileHashMismatch(t *testing.T) {
	// Save original values
	origHTTPClient := HTTPClient
	origCoreAPI := CoreAPIBase
	origMediaDir := MediaDir
	defer func() {
		HTTPClient = origHTTPClient
		CoreAPIBase = origCoreAPI
		MediaDir = origMediaDir
	}()

	tmp := t.TempDir()
	MediaDir = tmp
	CoreAPIBase = "http://test.example.com"

	fileContent := "test video content"
	item := ManifestItem{
		ID:            "1",
		Filename:      "test.mp4",
		FileSizeBytes: int64(len(fileContent)),
		SHA256:        "wronghash", // Intentionally wrong hash
	}

	mockClient := &mockHTTPClient{
		responses: map[string]*http.Response{
			"/api/devicesync/1": {
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(fileContent)),
			},
		},
	}
	HTTPClient = mockClient

	ctx := context.Background()
	err := downloadFile(ctx, item)
	if err == nil {
		t.Fatal("expected error due to hash mismatch, got nil")
	}

	if !strings.Contains(err.Error(), "hash mismatch") {
		t.Errorf("expected hash mismatch error, got: %v", err)
	}

	// Verify file was NOT created (due to hash mismatch)
	filePath := filepath.Join(tmp, "test.mp4")
	if _, err := os.Stat(filePath); err == nil {
		t.Error("file should not exist after hash mismatch")
	}
}

func TestGarbageCollect(t *testing.T) {
	// Save original MediaDir
	origMediaDir := MediaDir
	defer func() {
		MediaDir = origMediaDir
	}()

	tmp := t.TempDir()
	MediaDir = tmp

	// Create some test files
	validFile := filepath.Join(tmp, "valid.mp4")
	obsoleteFile := filepath.Join(tmp, "obsolete.mp4")
	tempFile := filepath.Join(tmp, ".temp.mp4.tmp")

	for _, f := range []string{validFile, obsoleteFile, tempFile} {
		if err := os.WriteFile(f, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	// Define manifest with only one valid file
	manifest := []ManifestItem{
		{ID: "1", Filename: "valid.mp4", FileSizeBytes: 4, SHA256: "test"},
	}

	err := garbageCollect(manifest)
	if err != nil {
		t.Fatalf("garbageCollect failed: %v", err)
	}

	// Valid file should still exist
	if _, err := os.Stat(validFile); os.IsNotExist(err) {
		t.Error("valid file was incorrectly deleted")
	}

	// Obsolete file should be removed
	if _, err := os.Stat(obsoleteFile); !os.IsNotExist(err) {
		t.Error("obsolete file was not deleted")
	}

	// Temp file should still exist (not garbage collected)
	if _, err := os.Stat(tempFile); os.IsNotExist(err) {
		t.Error("temp file should not be deleted")
	}
}

func TestCheckFileNeedsDownload(t *testing.T) {
	// Save original MediaDir
	origMediaDir := MediaDir
	defer func() {
		MediaDir = origMediaDir
	}()

	tmp := t.TempDir()
	MediaDir = tmp

	// Test case 1: File doesn't exist
	item := ManifestItem{
		ID:            "1",
		Filename:      "missing.mp4",
		FileSizeBytes: 100,
		SHA256:        "abc123",
	}

	needsDownload, err := checkFileNeedsDownload(item)
	if err != nil {
		t.Fatalf("checkFileNeedsDownload failed: %v", err)
	}
	if !needsDownload {
		t.Error("expected needsDownload=true for missing file")
	}

	// Test case 2: File exists with correct size and hash
	fileContent := "test content"
	correctHash := "6ae8a75555209fd6c44157c0aed8016e763ff435a19cf186f76863140143ff72" // SHA256 of "test content"
	filePath := filepath.Join(tmp, "exists.mp4")
	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	item = ManifestItem{
		ID:            "2",
		Filename:      "exists.mp4",
		FileSizeBytes: int64(len(fileContent)),
		SHA256:        correctHash,
	}

	needsDownload, err = checkFileNeedsDownload(item)
	if err != nil {
		t.Fatalf("checkFileNeedsDownload failed: %v", err)
	}
	if needsDownload {
		t.Error("expected needsDownload=false for correct file")
	}

	// Test case 3: File exists with wrong size
	item.FileSizeBytes = 999
	needsDownload, err = checkFileNeedsDownload(item)
	if err != nil {
		t.Fatalf("checkFileNeedsDownload failed: %v", err)
	}
	if !needsDownload {
		t.Error("expected needsDownload=true for wrong size")
	}

	// Test case 4: File exists with wrong hash
	item.FileSizeBytes = int64(len(fileContent))
	item.SHA256 = "wronghash"
	needsDownload, err = checkFileNeedsDownload(item)
	if err != nil {
		t.Fatalf("checkFileNeedsDownload failed: %v", err)
	}
	if !needsDownload {
		t.Error("expected needsDownload=true for wrong hash")
	}
}

func TestTriggerSyncPreventsParallel(t *testing.T) {
	// Save original values
	origHTTPClient := HTTPClient
	origCoreAPI := CoreAPIBase
	origMediaDir := MediaDir
	defer func() {
		HTTPClient = origHTTPClient
		CoreAPIBase = origCoreAPI
		MediaDir = origMediaDir
	}()

	tmp := t.TempDir()
	MediaDir = tmp
	CoreAPIBase = "http://test.example.com"

	// Set up a mock client that returns empty manifest
	mockClient := &mockHTTPClient{
		responses: map[string]*http.Response{
			"/api/devicesync": {
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("[]")),
			},
		},
	}
	HTTPClient = mockClient

	// Lock the sync mutex
	syncInProgressMutex.Lock()

	// Try to trigger sync while locked
	err := TriggerSync()
	if err == nil {
		t.Fatal("expected error when sync is already in progress")
	}
	if !strings.Contains(err.Error(), "already in progress") {
		t.Errorf("expected 'already in progress' error, got: %v", err)
	}

	// Unlock and try again
	syncInProgressMutex.Unlock()
	err = TriggerSync()
	if err != nil {
		t.Fatalf("TriggerSync failed: %v", err)
	}

	// Check sync status was updated
	status := GetSyncStatus()
	if status.LastSyncTime.IsZero() {
		t.Error("expected LastSyncTime to be set")
	}
	if !status.LastSyncOK {
		t.Error("expected LastSyncOK to be true")
	}
}

func TestSyncScheduleOperations(t *testing.T) {
	// Save original path
	origPath := syncSchedulePath
	defer func() {
		syncSchedulePath = origPath
	}()

	tmp := t.TempDir()
	syncSchedulePath = filepath.Join(tmp, "sync.schedule.json")

	// Test SetSyncSchedule
	times := []string{"10:30", "14:45", "20:00"}
	err := SetSyncSchedule(times)
	if err != nil {
		t.Fatalf("SetSyncSchedule failed: %v", err)
	}

	// Test GetSyncSchedule
	schedule := GetSyncSchedule()
	if len(schedule.Times) != 3 {
		t.Fatalf("expected 3 times, got %d", len(schedule.Times))
	}

	// Times should be normalized and sorted
	expectedTimes := []string{"10:30", "14:45", "20:00"}
	for i, expected := range expectedTimes {
		if schedule.Times[i] != expected {
			t.Errorf("time %d: expected %s, got %s", i, expected, schedule.Times[i])
		}
	}

	// Test invalid time format
	err = SetSyncSchedule([]string{"25:00"})
	if err == nil {
		t.Error("expected error for invalid time")
	}

	err = SetSyncSchedule([]string{"10:99"})
	if err == nil {
		t.Error("expected error for invalid minutes")
	}
}

func TestCalculateNextSyncTime(t *testing.T) {
	// Save original values
	origSchedulePath := syncSchedulePath
	origSyncConfig := CurrentSyncConfig
	origSchedule := GetSyncSchedule() // Save the original schedule
	
	defer func() {
		syncSchedulePath = origSchedulePath
		CurrentSyncConfig = origSyncConfig
		// Restore original schedule
		if len(origSchedule.Times) > 0 {
			SetSyncSchedule(origSchedule.Times)
		} else {
			syncScheduleMutex.Lock()
			syncSchedule = SyncScheduleSettings{}
			syncScheduleMutex.Unlock()
		}
	}()

	// Clear any existing schedule first
	syncScheduleMutex.Lock()
	syncSchedule = SyncScheduleSettings{}
	syncScheduleMutex.Unlock()

	tmp := t.TempDir()
	syncSchedulePath = filepath.Join(tmp, "sync.schedule.json")

	// Test with no schedule - should return zero time (no automatic sync)
	CurrentSyncConfig = SyncConfig{
		Enabled:         true,
		IntervalSeconds: 300,
	}
	
	nextTime := calculateNextSyncTime()

	// Should return zero time when no schedule is configured
	if !nextTime.IsZero() {
		t.Errorf("expected zero time with no schedule, got %v", nextTime)
	}

	// Test with schedule - use a time that's definitely in the future today
	now := time.Now()
	// Add 2 hours to current time to ensure it's in the future
	futureTime := now.Add(2 * time.Hour)
	scheduleTime := fmt.Sprintf("%02d:%02d", futureTime.Hour(), futureTime.Minute())
	
	err := SetSyncSchedule([]string{scheduleTime})
	if err != nil {
		t.Fatalf("SetSyncSchedule failed: %v", err)
	}

	nextTime = calculateNextSyncTime()
	
	// Next time should be roughly 2 hours from now (within 5 minutes tolerance)
	expectedMin := now.Add(115 * time.Minute)
	expectedMax := now.Add(125 * time.Minute)
	
	if nextTime.Before(expectedMin) || nextTime.After(expectedMax) {
		t.Errorf("scheduled next sync time %v not in expected range [%v, %v]", nextTime, expectedMin, expectedMax)
	}
}

func TestHandleSyncStart(t *testing.T) {
	// Save original values
	origHTTPClient := HTTPClient
	origCoreAPI := CoreAPIBase
	origMediaDir := MediaDir
	origServerKey := ServerKey
	defer func() {
		HTTPClient = origHTTPClient
		CoreAPIBase = origCoreAPI
		MediaDir = origMediaDir
		ServerKey = origServerKey
	}()

	tmp := t.TempDir()
	MediaDir = tmp
	CoreAPIBase = "http://test.example.com"
	ServerKey = "test-key"

	// Set up a mock client that returns empty manifest
	mockClient := &mockHTTPClient{
		responses: map[string]*http.Response{
			"/api/devicesync": {
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("[]")),
			},
		},
	}
	HTTPClient = mockClient

	req := httptest.NewRequest(http.MethodPost, "/api/menu/sync/start", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleSyncStart(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !resp.OK {
		t.Errorf("expected OK response, got: %+v", resp)
	}
}

func TestHandleSyncScheduleGetAndUpdate(t *testing.T) {
	// Save original values
	origSchedulePath := syncSchedulePath
	origServerKey := ServerKey
	defer func() {
		syncSchedulePath = origSchedulePath
		ServerKey = origServerKey
	}()

	tmp := t.TempDir()
	syncSchedulePath = filepath.Join(tmp, "sync.schedule.json")
	ServerKey = "test-key"

	// Set initial schedule
	times := []string{"10:00", "15:00"}
	if err := SetSyncSchedule(times); err != nil {
		t.Fatalf("SetSyncSchedule failed: %v", err)
	}

	// Test GET
	req := httptest.NewRequest(http.MethodGet, "/api/menu/sync/schedule/get", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleSyncScheduleGet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var getResp struct {
		OK   bool                 `json:"ok"`
		Data SyncScheduleSettings `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !getResp.OK {
		t.Error("expected OK response")
	}
	if len(getResp.Data.Times) != 2 {
		t.Errorf("expected 2 times, got %d", len(getResp.Data.Times))
	}

	// Test UPDATE
	updateBody := `{"times": ["12:30", "18:45"]}`
	req2 := httptest.NewRequest(http.MethodPut, "/api/menu/sync/schedule/update", strings.NewReader(updateBody))
	req2.Header.Set("Authorization", "Bearer test-key")
	w2 := httptest.NewRecorder()

	HandleSyncScheduleUpdate(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w2.Code, w2.Body.String())
	}

	// Verify schedule was updated
	schedule := GetSyncSchedule()
	if len(schedule.Times) != 2 {
		t.Errorf("expected 2 times, got %d", len(schedule.Times))
	}
	if schedule.Times[0] != "12:30" || schedule.Times[1] != "18:45" {
		t.Errorf("unexpected schedule: %v", schedule.Times)
	}
}
