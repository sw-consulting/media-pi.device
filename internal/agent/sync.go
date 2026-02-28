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
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

var (
	syncStatusFilePath = "/var/lib/media-pi-agent/sync-status.json"
)

// ManifestItem represents a single file in the sync manifest.
type ManifestItem struct {
	ID            int64  `json:"id"`
	Filename      string `json:"filename"`
	FileSizeBytes int64  `json:"fileSizeBytes"`
	SHA256        string `json:"sha256"`
}

// Manifest represents the response from /api/devicesync endpoint.
type Manifest struct {
	Items []ManifestItem `json:"items"`
}

// SyncStatus represents the last sync operation status.
type SyncStatus struct {
	LastSyncTime time.Time `json:"lastSyncTime"`
	OK           bool      `json:"ok"`
	Error        string    `json:"error,omitempty"`
}

var (
	// syncStatus holds the last sync status in memory
	syncStatus     SyncStatus
	syncStatusLock sync.RWMutex

	// syncContext and syncCancel are used to cancel ongoing sync operations
	syncContext context.Context
	syncCancel  context.CancelFunc
	syncLock    sync.Mutex

	// syncReloadChan is used to signal the scheduler to reload the schedule
	syncReloadChan chan struct{}

	// scheduledSyncCallback is called after a successful scheduled sync
	scheduledSyncCallback     func()
	scheduledSyncCallbackLock sync.RWMutex

	// cronScheduler manages scheduled sync operations
	cronScheduler     *cron.Cron
	cronSchedulerLock sync.Mutex
)

func init() {
	syncContext, syncCancel = context.WithCancel(context.Background())
	syncReloadChan = make(chan struct{}, 1)
}

// GetSyncStatus returns the current sync status.
func GetSyncStatus() SyncStatus {
	syncStatusLock.RLock()
	defer syncStatusLock.RUnlock()
	return syncStatus
}

// setSyncStatus updates the sync status in memory and optionally persists to file.
func setSyncStatus(status SyncStatus) {
	syncStatusLock.Lock()
	syncStatus = status
	syncStatusLock.Unlock()

	// Try to persist to file (best effort)
	go func() {
		data, err := json.Marshal(status)
		if err != nil {
			log.Printf("Warning: Failed to marshal sync status: %v", err)
			return
		}

		dir := filepath.Dir(syncStatusFilePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("Warning: Failed to create sync status directory: %v", err)
			return
		}

		tmpPath := syncStatusFilePath + ".tmp"
		if err := os.WriteFile(tmpPath, data, 0644); err != nil {
			log.Printf("Warning: Failed to write sync status: %v", err)
			return
		}
		if err := os.Rename(tmpPath, syncStatusFilePath); err != nil {
			_ = os.Remove(tmpPath)
			log.Printf("Warning: Failed to rename sync status file: %v", err)
		}
	}()
}

// fetchManifest fetches the manifest from the core API.
func fetchManifest(ctx context.Context, config Config) (*Manifest, error) {
	url := config.CoreAPIBase + "/api/devicesync"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add device authentication header
	req.Header.Set("X-Device-Id", config.ServerKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	var manifest Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	return &manifest, nil
}

// downloadFile downloads a file from the core API and verifies its integrity.
func downloadFile(ctx context.Context, config Config, item ManifestItem, destPath string) error {
	url := fmt.Sprintf("%s/api/devicesync/%d", config.CoreAPIBase, item.ID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add device authentication header
	req.Header.Set("X-Device-Id", config.ServerKey)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	// Create temp file
	tmpPath := destPath + ".tmp"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}()

	// Download file while computing SHA256
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmpFile, hasher), resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Verify file size
	if written != item.FileSizeBytes {
		return fmt.Errorf("file size mismatch: expected %d, got %d", item.FileSizeBytes, written)
	}

	// Verify SHA256
	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if actualHash != item.SHA256 {
		return fmt.Errorf("SHA256 mismatch: expected %s, got %s", item.SHA256, actualHash)
	}

	// Close temp file before rename
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}

// verifyLocalFile checks if a local file matches the manifest item.
func verifyLocalFile(path string, item ManifestItem) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	// Check size first (fast check)
	if info.Size() != item.FileSizeBytes {
		return false, nil
	}

	// Compute SHA256
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return false, err
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	return actualHash == item.SHA256, nil
}

// syncFiles synchronizes files from the manifest to the local media directory.
func syncFiles(ctx context.Context, config Config, manifest *Manifest) error {
	// Get media directory from playlist destination
	mediaDir := filepath.Dir(config.Playlist.Destination)
	if mediaDir == "" || mediaDir == "." {
		mediaDir = "/var/media-pi"
	}

	// Ensure media directory exists
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		return fmt.Errorf("failed to create media directory: %w", err)
	}

	// Track expected files for garbage collection
	expectedFiles := make(map[string]struct{})

	// Validate all filenames first to prevent path traversal and build expected files map
	for _, item := range manifest.Items {
		// Validate filename - prevent path traversal
		if item.Filename == "" || item.Filename[0] == '/' || item.Filename[0] == '\\' {
			log.Printf("Warning: Invalid filename '%s' for item %d, skipping", item.Filename, item.ID)
			continue
		}
		// Check for path traversal attempts
		if strings.Contains(item.Filename, "..") {
			log.Printf("Warning: Suspicious filename '%s' for item %d, skipping", item.Filename, item.ID)
			continue
		}
		// Normalize path separators for cross-platform compatibility (server uses forward slashes)
		normalizedPath := filepath.FromSlash(item.Filename)
		cleanPath := filepath.Clean(normalizedPath)
		if cleanPath != normalizedPath || filepath.IsAbs(cleanPath) {
			log.Printf("Warning: Suspicious filename '%s' for item %d, skipping", item.Filename, item.ID)
			continue
		}

		// Mark as expected
		fullPath := filepath.Join(mediaDir, item.Filename)
		expectedFiles[fullPath] = struct{}{}
	}

	// Download missing or outdated files
	var downloadErrors []string
	for _, item := range manifest.Items {
		// Skip invalid filenames (already validated above)
		if item.Filename == "" || item.Filename[0] == '/' || item.Filename[0] == '\\' || strings.Contains(item.Filename, "..") {
			continue
		}
		// Normalize path separators for cross-platform compatibility
		normalizedPath := filepath.FromSlash(item.Filename)
		cleanPath := filepath.Clean(normalizedPath)
		if cleanPath != normalizedPath || filepath.IsAbs(cleanPath) {
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fullPath := filepath.Join(mediaDir, item.Filename)

		// Ensure subdirectories exist
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			downloadErrors = append(downloadErrors, fmt.Sprintf("%s: %v", item.Filename, err))
			continue
		}

		// Check if file needs update
		needsUpdate := true
		if valid, err := verifyLocalFile(fullPath, item); err == nil && valid {
			needsUpdate = false
		}

		if needsUpdate {
			log.Printf("Downloading %s (ID: %d, size: %d bytes)", item.Filename, item.ID, item.FileSizeBytes)
			if err := downloadFile(ctx, config, item, fullPath); err != nil {
				downloadErrors = append(downloadErrors, fmt.Sprintf("%s: %v", item.Filename, err))
			}
		}
	}

	// Garbage collect files not in manifest
	// Protect playlist file from deletion by adding it to expectedFiles
	if config.Playlist.Destination != "" {
		expectedFiles[config.Playlist.Destination] = struct{}{}
	}
	
	if err := garbageCollect(mediaDir, expectedFiles); err != nil {
		log.Printf("Warning: Garbage collection errors: %v", err)
	}

	if len(downloadErrors) > 0 {
		return fmt.Errorf("download errors: %v", downloadErrors)
	}

	return nil
}

// garbageCollect removes files from the media directory that are not in the manifest.
func garbageCollect(mediaDir string, expectedFiles map[string]struct{}) error {
	var errors []string

	err := filepath.Walk(mediaDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Skip temp files
		if filepath.Ext(path) == ".tmp" {
			return nil
		}

		// Check if file is expected
		if _, expected := expectedFiles[path]; !expected {
			log.Printf("Garbage collecting: %s", path)
			if err := os.Remove(path); err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", path, err))
			}
		}

		return nil
	})

	if err != nil {
		errors = append(errors, fmt.Sprintf("walk error: %v", err))
	}

	if len(errors) > 0 {
		return fmt.Errorf("%v", errors)
	}

	return nil
}

// PerformSync performs a complete sync operation.
func PerformSync(ctx context.Context) error {
	config := GetCurrentConfig()

	log.Println("Starting sync operation")
	startTime := time.Now()

	manifest, err := fetchManifest(ctx, config)
	if err != nil {
		setSyncStatus(SyncStatus{
			LastSyncTime: startTime,
			OK:           false,
			Error:        err.Error(),
		})
		return fmt.Errorf("failed to fetch manifest: %w", err)
	}

	log.Printf("Manifest fetched: %d items", len(manifest.Items))

	if err := syncFiles(ctx, config, manifest); err != nil {
		setSyncStatus(SyncStatus{
			LastSyncTime: startTime,
			OK:           false,
			Error:        err.Error(),
		})
		return fmt.Errorf("failed to sync files: %w", err)
	}

	setSyncStatus(SyncStatus{
		LastSyncTime: startTime,
		OK:           true,
		Error:        "",
	})

	log.Println("Sync completed successfully")
	return nil
}

// TriggerSync triggers an immediate sync operation.
// If callback is provided, it will be called after successful sync.
func TriggerSync(callback func()) error {
	syncLock.Lock()
	defer syncLock.Unlock()

	// Cancel any ongoing sync
	if syncCancel != nil {
		syncCancel()
	}

	// Create new context
	syncContext, syncCancel = context.WithCancel(context.Background())
	
	// Capture context before releasing lock
	ctx := syncContext

	// Perform sync in background
	go func() {
		if err := PerformSync(ctx); err != nil {
			log.Printf("Sync error: %v", err)
		} else if callback != nil {
			callback()
		}
	}()

	return nil
}

// TriggerSyncWithCallback is an alias for TriggerSync for compatibility.
func TriggerSyncWithCallback(callback func()) error {
	return TriggerSync(callback)
}

// TriggerPlaylistSync triggers playlist download and service restart.
// This downloads only the playlist file (not video files) and restarts the play service.
func TriggerPlaylistSync(callback func()) error {
	syncLock.Lock()
	defer syncLock.Unlock()

	// Cancel any ongoing sync
	if syncCancel != nil {
		syncCancel()
	}

	// Create new context
	syncContext, syncCancel = context.WithCancel(context.Background())
	
	// Capture context before releasing lock
	ctx := syncContext

	// Perform playlist sync in background
	go func() {
		log.Println("Starting playlist-only sync")
		if err := PerformPlaylistSync(ctx); err != nil {
			log.Printf("Playlist sync error: %v", err)
		} else if callback != nil {
			callback()
		}
	}()

	return nil
}

// StopSync cancels any ongoing sync operation.
func StopSync() error {
	syncLock.Lock()
	defer syncLock.Unlock()

	if syncCancel != nil {
		syncCancel()
	}

	return nil
}

// downloadPlaylist downloads the current playlist from the core API.
func downloadPlaylist(ctx context.Context, config Config) ([]byte, error) {
	url := config.CoreAPIBase + "/api/devicesync/playlist"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add device authentication header
	req.Header.Set("X-Device-Id", config.ServerKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download playlist: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNoContent {
		// No playlist to activate
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read playlist: %w", err)
	}

	return data, nil
}

// PerformPlaylistSync downloads the playlist and optionally saves it.
func PerformPlaylistSync(ctx context.Context) error {
	config := GetCurrentConfig()

	log.Println("Starting playlist sync")

	data, err := downloadPlaylist(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to download playlist: %w", err)
	}

	if data == nil {
		log.Println("No playlist to activate (HTTP 204)")
		return nil
	}

	// Save playlist to destination
	if config.Playlist.Destination != "" {
		destPath := config.Playlist.Destination
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create playlist directory: %w", err)
		}

		tmpPath := destPath + ".tmp"
		if err := os.WriteFile(tmpPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write playlist: %w", err)
		}
		if err := os.Rename(tmpPath, destPath); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("failed to rename playlist: %w", err)
		}

		log.Printf("Playlist saved to %s", destPath)
	}

	log.Println("Playlist sync completed successfully")
	return nil
}

// StartScheduler starts the sync scheduler.
// Schedule is read from config (agent.yaml) - no separate schedule file needed.
func StartScheduler() error {
	// Initialize cron scheduler
	cronSchedulerLock.Lock()
	cronScheduler = cron.New()
	cronSchedulerLock.Unlock()

	// Start scheduler goroutine
	go schedulerLoop()

	return nil
}

// schedulerLoop manages scheduled sync operations.
// It reads playlist and video schedules from config and schedules them separately:
// - Playlist schedule: downloads playlist only and restarts play service
// - Video schedule: downloads video files only (no playlist, no restart)
func schedulerLoop() {
	for {
		config := GetCurrentConfig()

		// Stop existing scheduler with lock protection
		cronSchedulerLock.Lock()
		if cronScheduler != nil {
			cronScheduler.Stop()
			cronScheduler = cron.New()
		}
		cronSchedulerLock.Unlock()

		// Add scheduled playlist sync tasks (playlist only + restart service)
		for _, timeStr := range config.Schedule.Playlist {
			timeStr := timeStr // capture loop variable
			parts := strings.Split(timeStr, ":")
			if len(parts) != 2 {
				log.Printf("Warning: Invalid playlist time format '%s', expected HH:MM", timeStr)
				continue
			}
			cronSpec := fmt.Sprintf("%s %s * * *", parts[1], parts[0])
			cronSchedulerLock.Lock()
			_, err := cronScheduler.AddFunc(cronSpec, func() {
				log.Printf("Running scheduled playlist sync at %s", timeStr)
				if err := PerformPlaylistSync(context.Background()); err != nil {
					log.Printf("Scheduled playlist sync error: %v", err)
				} else {
					// Restart play service after successful playlist sync
					callback := getScheduledSyncCallback()
					if callback != nil {
						callback()
					}
				}
			})
			cronSchedulerLock.Unlock()
			if err != nil {
				log.Printf("Warning: Failed to schedule playlist sync at %s: %v", timeStr, err)
			}
		}

		// Add scheduled video sync tasks (video files only, no restart)
		for _, timeStr := range config.Schedule.Video {
			timeStr := timeStr // capture loop variable
			parts := strings.Split(timeStr, ":")
			if len(parts) != 2 {
				log.Printf("Warning: Invalid video time format '%s', expected HH:MM", timeStr)
				continue
			}
			cronSpec := fmt.Sprintf("%s %s * * *", parts[1], parts[0])
			cronSchedulerLock.Lock()
			_, err := cronScheduler.AddFunc(cronSpec, func() {
				log.Printf("Running scheduled video sync at %s", timeStr)
				if err := PerformSync(context.Background()); err != nil {
					log.Printf("Scheduled video sync error: %v", err)
				}
				// Note: No restart after video sync - only playlist sync restarts service
			})
			cronSchedulerLock.Unlock()
			if err != nil {
				log.Printf("Warning: Failed to schedule video sync at %s: %v", timeStr, err)
			}
		}

		// Start scheduler with lock protection
		cronSchedulerLock.Lock()
		cronScheduler.Start()
		cronSchedulerLock.Unlock()

		// Wait for reload signal
		<-syncReloadChan
		log.Println("Reloading sync schedule")
	}
}

// SetScheduledSyncCallback sets the callback to be called after successful scheduled syncs.
func SetScheduledSyncCallback(callback func()) {
	scheduledSyncCallbackLock.Lock()
	defer scheduledSyncCallbackLock.Unlock()
	scheduledSyncCallback = callback
}

// getScheduledSyncCallback returns the current scheduled sync callback.
func getScheduledSyncCallback() func() {
	scheduledSyncCallbackLock.RLock()
	defer scheduledSyncCallbackLock.RUnlock()
	return scheduledSyncCallback
}

// StopScheduler stops the sync scheduler.
func StopScheduler() {
	cronSchedulerLock.Lock()
	defer cronSchedulerLock.Unlock()
	if cronScheduler != nil {
		cronScheduler.Stop()
	}
}

// SignalSchedulerReload signals the scheduler to reload its configuration.
func SignalSchedulerReload() {
	syncLock.Lock()
	defer syncLock.Unlock()
	select {
	case syncReloadChan <- struct{}{}:
	default:
	}
}
