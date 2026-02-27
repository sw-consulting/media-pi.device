// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFetchManifest(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse string
		statusCode     int
		wantErr        bool
		wantItems      int
	}{
		{
			name: "valid manifest",
			serverResponse: `{
				"items": [
					{"id": 1, "filename": "test.mp4", "fileSizeBytes": 1024, "sha256": "abc123"},
					{"id": 2, "filename": "sub/test2.mp4", "fileSizeBytes": 2048, "sha256": "def456"}
				]
			}`,
			statusCode: http.StatusOK,
			wantErr:    false,
			wantItems:  2,
		},
		{
			name:           "empty manifest",
			serverResponse: `{"items": []}`,
			statusCode:     http.StatusOK,
			wantErr:        false,
			wantItems:      0,
		},
		{
			name:           "server error",
			serverResponse: `error`,
			statusCode:     http.StatusInternalServerError,
			wantErr:        true,
		},
		{
			name:           "invalid json",
			serverResponse: `{invalid`,
			statusCode:     http.StatusOK,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/devicesync" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			config := Config{
				CoreAPIBase: server.URL,
				ServerKey:   "test-key",
			}

			manifest, err := fetchManifest(context.Background(), config)
			if (err != nil) != tt.wantErr {
				t.Errorf("fetchManifest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(manifest.Items) != tt.wantItems {
				t.Errorf("fetchManifest() got %d items, want %d", len(manifest.Items), tt.wantItems)
			}
		})
	}
}

func TestFetchManifest_WithAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authentication header
		deviceID := r.Header.Get("X-Device-Id")
		if deviceID != "test-key-123" {
			t.Errorf("expected X-Device-Id header 'test-key-123', got '%s'", deviceID)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items": []}`))
	}))
	defer server.Close()

	config := Config{
		CoreAPIBase: server.URL,
		ServerKey:   "test-key-123",
	}

	_, err := fetchManifest(context.Background(), config)
	if err != nil {
		t.Errorf("fetchManifest() error = %v", err)
	}
}

func TestDownloadFile(t *testing.T) {
	testContent := "test file content"
	testHash := "60f5237ed4049f0382661ef009d2bc42e48c3ceb3edb6600f7024e7ab3b838f3" // Real SHA256 of "test file content"

	tests := []struct {
		name           string
		item           ManifestItem
		serverResponse string
		statusCode     int
		wantErr        bool
	}{
		{
			name: "successful download",
			item: ManifestItem{
				ID:            1,
				Filename:      "test.txt",
				FileSizeBytes: int64(len(testContent)),
				SHA256:        testHash,
			},
			serverResponse: testContent,
			statusCode:     http.StatusOK,
			wantErr:        false,
		},
		{
			name: "size mismatch",
			item: ManifestItem{
				ID:            1,
				Filename:      "test.txt",
				FileSizeBytes: 999,
				SHA256:        testHash,
			},
			serverResponse: testContent,
			statusCode:     http.StatusOK,
			wantErr:        true,
		},
		{
			name: "hash mismatch",
			item: ManifestItem{
				ID:            1,
				Filename:      "test.txt",
				FileSizeBytes: int64(len(testContent)),
				SHA256:        "wrong_hash",
			},
			serverResponse: testContent,
			statusCode:     http.StatusOK,
			wantErr:        true,
		},
		{
			name: "server error",
			item: ManifestItem{
				ID:            1,
				Filename:      "test.txt",
				FileSizeBytes: int64(len(testContent)),
				SHA256:        testHash,
			},
			serverResponse: "error",
			statusCode:     http.StatusInternalServerError,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expectedPath := fmt.Sprintf("/api/devicesync/%d", tt.item.ID)
				if r.URL.Path != expectedPath {
					t.Errorf("unexpected path: %s, want %s", r.URL.Path, expectedPath)
				}
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			config := Config{
				CoreAPIBase: server.URL,
				ServerKey:   "test-key",
			}

			tmpDir := t.TempDir()
			destPath := filepath.Join(tmpDir, "test.txt")

			err := downloadFile(context.Background(), config, tt.item, destPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("downloadFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify file was created
				content, err := os.ReadFile(destPath)
				if err != nil {
					t.Errorf("failed to read downloaded file: %v", err)
					return
				}
				if string(content) != testContent {
					t.Errorf("downloaded content = %s, want %s", string(content), testContent)
				}
			}
		})
	}
}

func TestDownloadFile_WithAuth(t *testing.T) {
	testContent := "test content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authentication header
		deviceID := r.Header.Get("X-Device-Id")
		if deviceID != "auth-key-456" {
			t.Errorf("expected X-Device-Id header 'auth-key-456', got '%s'", deviceID)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(testContent))
	}))
	defer server.Close()

	config := Config{
		CoreAPIBase: server.URL,
		ServerKey:   "auth-key-456",
	}

	item := ManifestItem{
		ID:            1,
		Filename:      "test.txt",
		FileSizeBytes: int64(len(testContent)),
		SHA256:        "6ae8a75555209fd6c44157c0aed8016e763ff435a19cf186f76863140143ff72", // real SHA256
	}

	tmpDir := t.TempDir()
	destPath := filepath.Join(tmpDir, "test.txt")

	err := downloadFile(context.Background(), config, item, destPath)
	if err != nil {
		t.Errorf("downloadFile() error = %v", err)
	}
}

func TestVerifyLocalFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "test content"
	_ = os.WriteFile(testFile, []byte(testContent), 0644)

	tests := []struct {
		name    string
		item    ManifestItem
		want    bool
		wantErr bool
	}{
		{
			name: "valid file",
			item: ManifestItem{
				Filename:      "test.txt",
				FileSizeBytes: int64(len(testContent)),
				SHA256:        "6ae8a75555209fd6c44157c0aed8016e763ff435a19cf186f76863140143ff72",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "size mismatch",
			item: ManifestItem{
				Filename:      "test.txt",
				FileSizeBytes: 999,
				SHA256:        "6ae8a75555209fd6c44157c0aed8016e763ff435a19cf186f76863140143ff72",
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "hash mismatch",
			item: ManifestItem{
				Filename:      "test.txt",
				FileSizeBytes: int64(len(testContent)),
				SHA256:        "wrong_hash",
			},
			want:    false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := verifyLocalFile(testFile, tt.item)
			if (err != nil) != tt.wantErr {
				t.Errorf("verifyLocalFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("verifyLocalFile() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVerifyLocalFile_MissingFile(t *testing.T) {
	item := ManifestItem{
		Filename:      "missing.txt",
		FileSizeBytes: 100,
		SHA256:        "abc",
	}

	got, err := verifyLocalFile("/nonexistent/missing.txt", item)
	if err != nil {
		t.Errorf("verifyLocalFile() unexpected error for missing file: %v", err)
	}
	if got != false {
		t.Errorf("verifyLocalFile() = %v, want false for missing file", got)
	}
}

func TestSyncFiles(t *testing.T) {
	tmpDir := t.TempDir()
	testContent := "file 1"

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/devicesync/1" {
			// Return file content
			_, _ = w.Write([]byte(testContent))
		}
	}))
	defer server.Close()

	config := Config{
		CoreAPIBase: server.URL,
		ServerKey:   "test-key",
		Playlist:    PlaylistConfig{Destination: filepath.Join(tmpDir, "playlist.m3u")},
	}

	manifest := &Manifest{
		Items: []ManifestItem{
			{
				ID:            1,
				Filename:      "file1.txt",
				FileSizeBytes: int64(len(testContent)),
				SHA256:        "83bf7fcd913e81d35f0d0e94ed1ec0611e8e3b4909c23b00ef9f076f205e67c6", // SHA256 of "file 1"
			},
		},
	}

	err := syncFiles(context.Background(), config, manifest)
	if err != nil {
		t.Errorf("syncFiles() error = %v", err)
	}

	// Verify file was created
	filePath := filepath.Join(tmpDir, "file1.txt")
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Errorf("failed to read synced file: %v", err)
	}
	if string(content) != testContent {
		t.Errorf("file content = %s, want %s", string(content), testContent)
	}
}

func TestSyncFiles_PreventPathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		filename string
	}{
		{"absolute path", "/etc/passwd"},
		{"parent directory", "../../../etc/passwd"},
		{"backslash path", "..\\..\\windows\\system32"},
		{"empty filename", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				CoreAPIBase: "http://example.com",
				Playlist:    PlaylistConfig{Destination: filepath.Join(tmpDir, "playlist.m3u")},
			}

			manifest := &Manifest{
				Items: []ManifestItem{
					{
						ID:            1,
						Filename:      tt.filename,
						FileSizeBytes: 10,
						SHA256:        "abc123",
					},
				},
			}

			// Should not error, but should skip the invalid file
			err := syncFiles(context.Background(), config, manifest)
			if err != nil {
				t.Errorf("syncFiles() should not error on invalid filename, got: %v", err)
			}

			// Verify no unexpected files were created in tmpDir
			entries, err := os.ReadDir(tmpDir)
			if err != nil {
				t.Errorf("failed to read tmpDir: %v", err)
			}
			if len(entries) > 0 {
				t.Errorf("syncFiles() created unexpected files: %v", entries)
			}
		})
	}
}

func TestSyncFiles_GarbageCollection(t *testing.T) {
	tmpDir := t.TempDir()

	// Create old files that should be removed
	oldFile := filepath.Join(tmpDir, "old.txt")
	_ = os.WriteFile(oldFile, []byte("old"), 0644)

	// Create subdirectory with old file
	subDir := filepath.Join(tmpDir, "subdir")
	_ = os.MkdirAll(subDir, 0755)
	oldSubFile := filepath.Join(subDir, "old_sub.txt")
	_ = os.WriteFile(oldSubFile, []byte("old sub"), 0644)

	config := Config{
		CoreAPIBase: "http://example.com",
		Playlist:    PlaylistConfig{Destination: filepath.Join(tmpDir, "playlist.m3u")},
	}

	// Empty manifest - all files should be garbage collected
	manifest := &Manifest{
		Items: []ManifestItem{},
	}

	err := syncFiles(context.Background(), config, manifest)
	if err != nil {
		t.Errorf("syncFiles() error = %v", err)
	}

	// Verify old files were removed
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Errorf("old file should be removed: %s", oldFile)
	}
	if _, err := os.Stat(oldSubFile); !os.IsNotExist(err) {
		t.Errorf("old sub file should be removed: %s", oldSubFile)
	}
}

func TestSyncFiles_GarbageCollectsInvalidFilenames(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file that should be garbage collected
	garbageFile := filepath.Join(tmpDir, "garbage.txt")
	_ = os.WriteFile(garbageFile, []byte("garbage"), 0644)

	config := Config{
		CoreAPIBase: "http://example.com",
		Playlist:    PlaylistConfig{Destination: filepath.Join(tmpDir, "playlist.m3u")},
	}

	// Manifest with invalid filename
	manifest := &Manifest{
		Items: []ManifestItem{
			{
				ID:            1,
				Filename:      "../invalid.txt",
				FileSizeBytes: 10,
				SHA256:        "abc123",
			},
		},
	}

	// No error expected - invalid filename should be skipped
	err := syncFiles(context.Background(), config, manifest)
	if err != nil {
		t.Errorf("syncFiles() unexpected error: %v", err)
	}

	// Verify garbage file was removed (not protected by invalid filename in manifest)
	if _, err := os.Stat(garbageFile); !os.IsNotExist(err) {
		t.Errorf("garbage file should be removed: %s", garbageFile)
	}
}

func TestSyncFiles_WithSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test server that serves files
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/devicesync/1":
			_, _ = w.Write([]byte("content1"))
		case "/api/devicesync/2":
			_, _ = w.Write([]byte("content2"))
		}
	}))
	defer server.Close()

	config := Config{
		CoreAPIBase: server.URL,
		ServerKey:   "test-key",
		Playlist:    PlaylistConfig{Destination: filepath.Join(tmpDir, "playlist.m3u")},
	}

	manifest := &Manifest{
		Items: []ManifestItem{
			{
				ID:            1,
				Filename:      "dir1/file1.txt",
				FileSizeBytes: 8,
				SHA256:        "e3b5ad3b4f0f6b8a6c5b1e2f3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d",
			},
			{
				ID:            2,
				Filename:      "dir1/dir2/file2.txt",
				FileSizeBytes: 8,
				SHA256:        "f4c6be4c5f0f7c9b7d6c2f3e4d5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3",
			},
		},
	}

	err := syncFiles(context.Background(), config, manifest)
	// Even if downloads fail due to hash mismatch, the directory structure should be created
	// We just want to test that subdirectories are created

	// Check if subdirectories were created
	dir1 := filepath.Join(tmpDir, "dir1")
	dir2 := filepath.Join(tmpDir, "dir1", "dir2")

	if _, err := os.Stat(dir1); os.IsNotExist(err) {
		t.Errorf("subdirectory dir1 was not created")
	}
	if _, err := os.Stat(dir2); os.IsNotExist(err) {
		t.Errorf("subdirectory dir1/dir2 was not created")
	}

	// The actual file download will fail due to hash mismatch, but that's expected
	// We're testing directory creation, not the full download process
	_ = err // Ignore errors from hash mismatches
}

func TestGetSetSyncStatus(t *testing.T) {
	status := SyncStatus{
		LastSyncTime: time.Now(),
		OK:           true,
		Error:        "",
	}

	setSyncStatus(status)
	got := GetSyncStatus()

	if got.OK != status.OK {
		t.Errorf("GetSyncStatus().OK = %v, want %v", got.OK, status.OK)
	}
	if got.Error != status.Error {
		t.Errorf("GetSyncStatus().Error = %v, want %v", got.Error, status.Error)
	}
}

func TestDownloadPlaylist(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		serverResponse string
		wantNil        bool
		wantErr        bool
	}{
		{
			name:           "playlist available",
			statusCode:     http.StatusOK,
			serverResponse: "playlist content",
			wantNil:        false,
			wantErr:        false,
		},
		{
			name:           "no playlist",
			statusCode:     http.StatusNoContent,
			serverResponse: "",
			wantNil:        true,
			wantErr:        false,
		},
		{
			name:           "server error",
			statusCode:     http.StatusInternalServerError,
			serverResponse: "error",
			wantNil:        false,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/devicesync/playlist" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				w.WriteHeader(tt.statusCode)
				if tt.serverResponse != "" {
					_, _ = w.Write([]byte(tt.serverResponse))
				}
			}))
			defer server.Close()

			config := Config{
				CoreAPIBase: server.URL,
				ServerKey:   "test-key",
			}

			data, err := downloadPlaylist(context.Background(), config)
			if (err != nil) != tt.wantErr {
				t.Errorf("downloadPlaylist() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantNil && data != nil {
				t.Errorf("downloadPlaylist() expected nil, got data")
			}
			if !tt.wantNil && !tt.wantErr && data == nil {
				t.Errorf("downloadPlaylist() expected data, got nil")
			}
		})
	}
}

func TestPerformPlaylistSync(t *testing.T) {
	tmpDir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test playlist"))
	}))
	defer server.Close()

	// Test with destination set
	destPath := filepath.Join(tmpDir, "playlist.txt")
	config := Config{
		CoreAPIBase: server.URL,
		ServerKey:   "test-key",
		Playlist: PlaylistConfig{
			Destination: destPath,
		},
	}

	// Store config for the test
	configMutex.Lock()
	currentConfig = &config
	configMutex.Unlock()

	err := PerformPlaylistSync(context.Background())
	if err != nil {
		t.Errorf("PerformPlaylistSync() error = %v", err)
	}

	// Verify playlist was saved
	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Errorf("failed to read playlist: %v", err)
	}
	if string(content) != "test playlist" {
		t.Errorf("playlist content = %s, want 'test playlist'", string(content))
	}
}

func TestTriggerSync_StopSync(t *testing.T) {
	// Mock config
	config := Config{
		CoreAPIBase: "http://example.com",
		Playlist:    PlaylistConfig{Destination: filepath.Join(t.TempDir(), "playlist.m3u")},
	}
	configMutex.Lock()
	currentConfig = &config
	configMutex.Unlock()

	// Create a mock server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(Manifest{Items: []ManifestItem{}})
	}))
	defer server.Close()

	config.CoreAPIBase = server.URL
	configMutex.Lock()
	currentConfig = &config
	configMutex.Unlock()

	// Trigger sync
	err := TriggerSync(nil)
	if err != nil {
		t.Errorf("TriggerSync() error = %v", err)
	}

	// Give it a moment to start
	time.Sleep(10 * time.Millisecond)

	// Stop sync
	err = StopSync()
	if err != nil {
		t.Errorf("StopSync() error = %v", err)
	}
}

func TestGarbageCollect(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")
	tmpFile := filepath.Join(tmpDir, "temp.tmp")

	_ = os.WriteFile(file1, []byte("content1"), 0644)
	_ = os.WriteFile(file2, []byte("content2"), 0644)
	_ = os.WriteFile(tmpFile, []byte("temp"), 0644)

	// Only file1 is expected
	expectedFiles := map[string]struct{}{
		file1: {},
	}

	err := garbageCollect(tmpDir, expectedFiles)
	if err != nil {
		t.Errorf("garbageCollect() error = %v", err)
	}

	// Verify file1 still exists
	if _, err := os.Stat(file1); os.IsNotExist(err) {
		t.Errorf("expected file was removed: %s", file1)
	}

	// Verify file2 was removed
	if _, err := os.Stat(file2); !os.IsNotExist(err) {
		t.Errorf("unexpected file was not removed: %s", file2)
	}

	// Verify tmp file was not removed (should be skipped)
	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Errorf("tmp file should not be removed: %s", tmpFile)
	}
}

func TestContextCancellation(t *testing.T) {
	config := Config{
		CoreAPIBase: "http://example.com",
		Playlist:    PlaylistConfig{Destination: filepath.Join(t.TempDir(), "playlist.m3u")},
	}

	// Create context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	manifest := &Manifest{
		Items: []ManifestItem{
			{ID: 1, Filename: "test.txt", FileSizeBytes: 100, SHA256: "abc"},
		},
	}

	err := syncFiles(ctx, config, manifest)
	if err != context.Canceled {
		t.Errorf("syncFiles() should return context.Canceled, got: %v", err)
	}
}
