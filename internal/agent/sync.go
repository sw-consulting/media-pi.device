// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ManifestItem represents a single file in the sync manifest from the backend.
type ManifestItem struct {
	ID            string `json:"id"`
	Filename      string `json:"filename"`
	FileSizeBytes int64  `json:"fileSizeBytes"`
	SHA256        string `json:"sha256"`
}

// SyncStatus holds the current state of the last sync operation.
type SyncStatus struct {
	mu            sync.RWMutex
	LastSyncTime  time.Time `json:"lastSyncTime"`
	LastSyncOK    bool      `json:"lastSyncOk"`
	LastSyncError string    `json:"lastSyncError,omitempty"`
	InProgress    bool      `json:"inProgress"`
}

// syncState holds the current sync status for the agent.
var syncState = &SyncStatus{}

// GetSyncStatus returns a copy of the current sync status.
func GetSyncStatus() SyncStatus {
	syncState.mu.RLock()
	defer syncState.mu.RUnlock()
	return SyncStatus{
		LastSyncTime:  syncState.LastSyncTime,
		LastSyncOK:    syncState.LastSyncOK,
		LastSyncError: syncState.LastSyncError,
		InProgress:    syncState.InProgress,
	}
}

// setSyncStatus updates the sync status atomically.
func setSyncStatus(ok bool, err error) {
	syncState.mu.Lock()
	defer syncState.mu.Unlock()
	syncState.LastSyncTime = time.Now()
	syncState.LastSyncOK = ok
	if err != nil {
		syncState.LastSyncError = err.Error()
	} else {
		syncState.LastSyncError = ""
	}
}

// setSyncInProgress marks sync as in progress or completed.
func setSyncInProgress(inProgress bool) {
	syncState.mu.Lock()
	defer syncState.mu.Unlock()
	syncState.InProgress = inProgress
}

// SyncOnce performs a single synchronization cycle:
// 1. Fetches manifest from backend
// 2. Compares with local files
// 3. Downloads missing/outdated files
// 4. Garbage collects files not in manifest
func SyncOnce(ctx context.Context) error {
	if AgentConfig == nil {
		return fmt.Errorf("agent config not loaded")
	}

	if !AgentConfig.Sync.Enabled {
		return nil // Sync disabled, not an error
	}

	if AgentConfig.CoreAPIBase == "" {
		return fmt.Errorf("core_api_base not configured")
	}

	setSyncInProgress(true)
	defer setSyncInProgress(false)

	// Ensure media directory exists
	if err := os.MkdirAll(AgentConfig.MediaDir, 0755); err != nil {
		return fmt.Errorf("failed to create media directory: %w", err)
	}

	// Fetch manifest
	manifest, err := fetchManifest(ctx)
	if err != nil {
		setSyncStatus(false, err)
		return fmt.Errorf("failed to fetch manifest: %w", err)
	}

	// Build map of expected files
	expectedFiles := make(map[string]ManifestItem, len(manifest))
	for _, item := range manifest {
		expectedFiles[item.Filename] = item
	}

	// Check existing files
	localFiles, err := listLocalFiles(AgentConfig.MediaDir)
	if err != nil {
		setSyncStatus(false, err)
		return fmt.Errorf("failed to list local files: %w", err)
	}

	// Download missing or outdated files
	for _, item := range manifest {
		localPath := filepath.Join(AgentConfig.MediaDir, item.Filename)
		needsDownload := false

		if info, exists := localFiles[item.Filename]; exists {
			// File exists, check if it needs updating
			if info.Size() != item.FileSizeBytes {
				needsDownload = true
			} else {
				// Verify SHA256
				hash, err := computeSHA256(localPath)
				if err != nil || hash != item.SHA256 {
					needsDownload = true
				}
			}
		} else {
			// File doesn't exist
			needsDownload = true
		}

		if needsDownload {
			if err := downloadFile(ctx, item); err != nil {
				setSyncStatus(false, err)
				return fmt.Errorf("failed to download %s: %w", item.Filename, err)
			}
			log.Printf("Downloaded: %s (ID: %s)", item.Filename, item.ID)
		}
	}

	// Garbage collect files not in manifest
	for filename := range localFiles {
		if _, expected := expectedFiles[filename]; !expected {
			localPath := filepath.Join(AgentConfig.MediaDir, filename)
			if err := os.Remove(localPath); err != nil {
				log.Printf("Warning: failed to remove obsolete file %s: %v", filename, err)
			} else {
				log.Printf("Removed obsolete file: %s", filename)
			}
		}
	}

	setSyncStatus(true, nil)
	log.Printf("Sync completed successfully: %d files in manifest", len(manifest))
	return nil
}

// fetchManifest retrieves the list of files from the backend.
func fetchManifest(ctx context.Context) ([]ManifestItem, error) {
	url := fmt.Sprintf("%s/api/devicesync", AgentConfig.CoreAPIBase)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	// Add authentication if device auth token is configured
	if AgentConfig.DeviceAuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+AgentConfig.DeviceAuthToken)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var manifest []ManifestItem
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, err
	}

	return manifest, nil
}

// listLocalFiles returns a map of filename -> file info for files in the media directory.
func listLocalFiles(dir string) (map[string]os.FileInfo, error) {
	files := make(map[string]os.FileInfo)
	
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return files, nil // Directory doesn't exist yet, return empty map
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip directories
		}
		info, err := entry.Info()
		if err != nil {
			log.Printf("Warning: failed to get info for %s: %v", entry.Name(), err)
			continue
		}
		files[entry.Name()] = info
	}

	return files, nil
}

// computeSHA256 calculates the SHA256 hash of a file.
func computeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// downloadFile downloads a file from the backend and writes it atomically.
func downloadFile(ctx context.Context, item ManifestItem) error {
	url := fmt.Sprintf("%s/api/devicesync/%s", AgentConfig.CoreAPIBase, item.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	// Add authentication if device auth token is configured
	if AgentConfig.DeviceAuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+AgentConfig.DeviceAuthToken)
	}

	client := &http.Client{Timeout: 300 * time.Second} // 5 minutes for large files
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	// Write to temporary file
	tmpPath := filepath.Join(AgentConfig.MediaDir, "."+item.Filename+".tmp")
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath) // Clean up temp file on error

	// Copy and compute hash simultaneously
	h := sha256.New()
	writer := io.MultiWriter(tmpFile, h)
	
	bytesWritten, err := io.Copy(writer, resp.Body)
	tmpFile.Close()
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Verify size
	if bytesWritten != item.FileSizeBytes {
		return fmt.Errorf("size mismatch: got %d bytes, expected %d", bytesWritten, item.FileSizeBytes)
	}

	// Verify hash
	actualHash := hex.EncodeToString(h.Sum(nil))
	if actualHash != item.SHA256 {
		return fmt.Errorf("hash mismatch: got %s, expected %s", actualHash, item.SHA256)
	}

	// Atomically rename temp file to final location
	finalPath := filepath.Join(AgentConfig.MediaDir, item.Filename)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}

// StartSyncScheduler starts a background goroutine that periodically runs SyncOnce.
// It should be called once at agent startup if sync is enabled.
func StartSyncScheduler(ctx context.Context) {
	if AgentConfig == nil || !AgentConfig.Sync.Enabled {
		log.Printf("Sync scheduler not started: sync disabled or config not loaded")
		return
	}

	interval := time.Duration(AgentConfig.Sync.IntervalSeconds) * time.Second
	log.Printf("Starting sync scheduler with interval: %v", interval)

	go func() {
		// Run initial sync after a short delay to allow agent to fully start
		time.Sleep(10 * time.Second)
		
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Printf("Sync scheduler stopped")
				return
			case <-ticker.C:
				// Check if a sync is already in progress
				syncState.mu.RLock()
				inProgress := syncState.InProgress
				syncState.mu.RUnlock()
				
				if inProgress {
					log.Printf("Skipping sync: previous sync still in progress")
					continue
				}

				// Run sync with timeout
				syncCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				if err := SyncOnce(syncCtx); err != nil {
					log.Printf("Sync error: %v", err)
				}
				cancel()
			}
		}
	}()
}
