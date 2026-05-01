// Copyright (C) 2025-2026 sw.consulting
// This file is a part of Media Pi device agent

package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

var (
	syncStatusFilePath   = "/var/media-pi/sync/sync-status.json"
	runScreenshotCapture = captureScreenshot
	runScreenshotCommand = func(inputPath, outputPath string) error {
		ffmpegPath, err := resolveFFmpegPath()
		if err != nil {
			return err
		}
		cmd := exec.Command(ffmpegPath, "-loglevel", "error", "-y", "-i", inputPath, "-frames:v", "1", outputPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("ffmpeg command failed: %w: %s", err, strings.TrimSpace(string(out)))
		}
		log.Printf("Debug: created device screenshot at %s", outputPath)
		return nil
	}
	screenshotNow = time.Now
)

// resolveFFmpegPath returns the path to the ffmpeg binary. It checks the
// FFMPEG_PATH environment variable first, then falls back to exec.LookPath.
func resolveFFmpegPath() (string, error) {
	if envFFmpegPath := strings.TrimSpace(os.Getenv("FFMPEG_PATH")); envFFmpegPath != "" {
		info, err := os.Stat(envFFmpegPath)
		if err != nil {
			return "", fmt.Errorf("FFMPEG_PATH %q is not accessible: %w", envFFmpegPath, err)
		}
		if info.IsDir() || info.Mode()&0o111 == 0 {
			return "", fmt.Errorf("FFMPEG_PATH %q is not an executable file", envFFmpegPath)
		}
		return envFFmpegPath, nil
	}
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("ffmpeg executable not found in PATH: %w", err)
	}
	return ffmpegPath, nil
}

// ManifestItem represents a single file in the sync manifest.
type ManifestItem struct {
	ID            int64  `json:"id"`
	Filename      string `json:"filename"`
	FileSizeBytes int64  `json:"fileSizeBytes"`
	SHA256        string `json:"sha256"`
}

// Manifest represents the response from /api/devicesync endpoint.
// The backend returns a JSON array of ManifestItem directly (IEnumerable<DeviceSyncManifestItem>),
// rather than an object with an "items" field.
type Manifest []ManifestItem

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

	// Tracking for running sync processes
	videoSyncRunning        bool
	videoSyncRunningLock    sync.RWMutex
	playlistSyncRunning     bool
	playlistSyncRunningLock sync.RWMutex
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

// IsVideoSyncRunning returns whether video sync is currently running.
func IsVideoSyncRunning() bool {
	videoSyncRunningLock.RLock()
	defer videoSyncRunningLock.RUnlock()
	return videoSyncRunning
}

// IsPlaylistSyncRunning returns whether playlist sync is currently running.
func IsPlaylistSyncRunning() bool {
	playlistSyncRunningLock.RLock()
	defer playlistSyncRunningLock.RUnlock()
	return playlistSyncRunning
}

// setVideoSyncRunning sets the video sync running state.
func setVideoSyncRunning(running bool) {
	videoSyncRunningLock.Lock()
	videoSyncRunning = running
	videoSyncRunningLock.Unlock()
}

// setPlaylistSyncRunning sets the playlist sync running state.
func setPlaylistSyncRunning(running bool) {
	playlistSyncRunningLock.Lock()
	playlistSyncRunning = running
	playlistSyncRunningLock.Unlock()
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
	// Get media directory from playlist destination (destination is a folder)
	mediaDir := config.Playlist.Destination
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
	for _, item := range *manifest {
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
	for _, item := range *manifest {
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
		playlistPath := filepath.Join(config.Playlist.Destination, "playlist.m3u")
		expectedFiles[playlistPath] = struct{}{}
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

// PerformSync performs a video sync operation.
func PerformSync(ctx context.Context) error {
	config := GetCurrentConfig()

	log.Println("Starting video sync")
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

	log.Printf("Manifest fetched: %d items", len(*manifest))

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
// Returns an error if prerequisites are not met (e.g., missing configuration).
func TriggerSync(callback func()) error {
	// Validate prerequisites before spawning async task
	config := GetCurrentConfig()
	if config.CoreAPIBase == "" {
		return fmt.Errorf("core_api_base not configured")
	}
	if config.ServerKey == "" {
		return fmt.Errorf("server_key not configured")
	}
	if config.Playlist.Destination == "" {
		return fmt.Errorf("playlist destination not configured")
	}

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
		setVideoSyncRunning(true)
		defer setVideoSyncRunning(false)
		if err := PerformSync(ctx); err != nil {
			log.Printf("Sync error: %v", err)
		} else if callback != nil {
			callback()
		}
	}()

	return nil
}

// TriggerPlaylistSync triggers playlist download and service restart.
// This downloads only the playlist file (not video files) and restarts the play service.
// Returns an error if prerequisites are not met (e.g., missing configuration).
func TriggerPlaylistSync(callback func()) error {
	// Validate prerequisites before spawning async task
	config := GetCurrentConfig()
	if config.CoreAPIBase == "" {
		return fmt.Errorf("core_api_base not configured")
	}
	if config.ServerKey == "" {
		return fmt.Errorf("server_key not configured")
	}
	if config.Playlist.Destination == "" {
		return fmt.Errorf("playlist destination not configured")
	}

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
		setPlaylistSyncRunning(true)
		defer setPlaylistSyncRunning(false)
		log.Println("Starting playlist sync")
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

	data, err := downloadPlaylist(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to download playlist: %w", err)
	}

	if data == nil {
		log.Println("No playlist to activate (HTTP 204)")
		return nil
	}

	// Save playlist to destination (destination is a folder, append filename)
	if config.Playlist.Destination != "" {
		destPath := filepath.Join(config.Playlist.Destination, "playlist.m3u")
		if err := os.MkdirAll(config.Playlist.Destination, 0755); err != nil {
			return fmt.Errorf("failed to create playlist directory: %w", err)
		}

		tmpPath := destPath + ".tmp"
		if err := os.WriteFile(tmpPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write playlist: %w", err)
		}
		// Remove destination file if it exists before rename (atomic replacement)
		_ = os.Remove(destPath)
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

	triggerStartupScreenshotCapture()

	// Start scheduler goroutine
	go schedulerLoop()

	return nil
}

func triggerStartupScreenshotCapture() {
	cfg := GetCurrentConfig()
	if cfg.Screenshot.IntervalMinutes <= 0 {
		return
	}

	go func() {
		if err := runScreenshotCapture(); err != nil {
			log.Printf("Failed to capture startup screenshot: %v", err)
		}
	}()
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

				// Route through shared sync trigger to serialize with manual sync operations.
				callback := getScheduledSyncCallback()
				if err := TriggerPlaylistSync(callback); err != nil {
					log.Printf("Failed to trigger scheduled playlist sync: %v", err)
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

				// Route through shared sync trigger to serialize with other sync operations.
				if err := TriggerSync(nil); err != nil {
					log.Printf("Failed to trigger scheduled video sync: %v", err)
				}
				// Note: No restart after video sync - only playlist sync restarts service
			})
			cronSchedulerLock.Unlock()
			if err != nil {
				log.Printf("Warning: Failed to schedule video sync at %s: %v", timeStr, err)
			}
		}

		// Add periodic screenshot capture task.
		// SkipIfStillRunning ensures at most one ffmpeg process runs at a time:
		// if a capture is still in progress when the next tick fires, that tick is dropped.
		if config.Screenshot.IntervalMinutes > 0 {
			cronSpec := fmt.Sprintf("@every %dm", config.Screenshot.IntervalMinutes)
			screenshotJob := cron.NewChain(
				cron.SkipIfStillRunning(cron.DiscardLogger),
			).Then(cron.FuncJob(func() {
				if err := runScreenshotCapture(); err != nil {
					log.Printf("Failed to capture screenshot: %v", err)
				}
			}))
			cronSchedulerLock.Lock()
			_, err := cronScheduler.AddJob(cronSpec, screenshotJob)
			cronSchedulerLock.Unlock()
			if err != nil {
				log.Printf("Warning: Failed to schedule screenshot capture every %d minutes: %v", config.Screenshot.IntervalMinutes, err)
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

func captureScreenshot() error {
	cfg := GetCurrentConfig()
	if cfg.Screenshot.IntervalMinutes <= 0 {
		return nil
	}
	pathTemplate := strings.TrimSpace(cfg.Screenshot.PathTemplate)
	if pathTemplate == "" {
		return fmt.Errorf("screenshot path template not configured")
	}
	input := strings.TrimSpace(cfg.Screenshot.Input)
	if input == "" {
		return fmt.Errorf("screenshot input is not configured")
	}
	outputPath := uniqueOutputPath(renderScreenshotOutputPath(pathTemplate, screenshotNow()))
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("create screenshot output directory %q: %w", outputDir, err)
	}

	if err := resendPendingScreenshots(context.Background(), cfg, outputDir, outputPath); err != nil {
		log.Printf("Warning: pending screenshot resend encountered errors: %v", err)
	}

	if err := runScreenshotCommand(input, outputPath); err != nil {
		return err
	}

	if err := uploadScreenshot(context.Background(), cfg, outputPath); err != nil {
		return fmt.Errorf("upload screenshot %q: %w", outputPath, err)
	}

	if err := os.Remove(outputPath); err != nil {
		return fmt.Errorf("delete uploaded screenshot %q: %w", outputPath, err)
	}

	log.Printf("Debug: uploaded and removed screenshot %s", outputPath)
	return nil
}

func captureScreenshotFileOnly() (string, error) {
	cfg := GetCurrentConfig()
	pathTemplate := strings.TrimSpace(cfg.Screenshot.PathTemplate)
	if pathTemplate == "" {
		return "", fmt.Errorf("screenshot path template not configured")
	}
	input := strings.TrimSpace(cfg.Screenshot.Input)
	if input == "" {
		return "", fmt.Errorf("screenshot input is not configured")
	}

	outputPath := uniqueOutputPath(renderScreenshotOutputPath(pathTemplate, screenshotNow()))
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", fmt.Errorf("create screenshot output directory %q: %w", outputDir, err)
	}
	if err := runScreenshotCommand(input, outputPath); err != nil {
		return "", err
	}

	return outputPath, nil
}

func resendPendingScreenshots(ctx context.Context, config Config, screenshotDir, currentCapturePath string) error {
	limit := config.Screenshot.ResendLimit
	if limit <= 0 {
		limit = DefaultScreenshotResendLimit
	}

	pending, err := listPendingScreenshotFiles(screenshotDir, currentCapturePath)
	if err != nil {
		return fmt.Errorf("list pending screenshots: %w", err)
	}

	if len(pending) > limit {
		pending = pending[:limit]
	}

	var resendErrors []string
	for _, path := range pending {
		if err := uploadScreenshot(ctx, config, path); err != nil {
			resendErrors = append(resendErrors, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		if err := os.Remove(path); err != nil {
			resendErrors = append(resendErrors, fmt.Sprintf("%s: delete failed: %v", path, err))
			continue
		}
		log.Printf("Debug: resent and removed pending screenshot %s", path)
	}

	if len(resendErrors) > 0 {
		return fmt.Errorf("%v", resendErrors)
	}

	return nil
}

func listPendingScreenshotFiles(screenshotDir, currentCapturePath string) ([]string, error) {
	entries, err := os.ReadDir(screenshotDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	currentName := filepath.Base(currentCapturePath)
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if entry.Name() == currentName {
			continue
		}
		files = append(files, filepath.Join(screenshotDir, entry.Name()))
	}

	sort.Strings(files)
	return files, nil
}

func uploadScreenshot(ctx context.Context, config Config, screenshotPath string) error {
	if strings.TrimSpace(config.CoreAPIBase) == "" {
		return fmt.Errorf("core_api_base not configured")
	}
	if strings.TrimSpace(config.ServerKey) == "" {
		return fmt.Errorf("server_key not configured")
	}

	file, err := os.Open(screenshotPath)
	if err != nil {
		return fmt.Errorf("open screenshot: %w", err)
	}
	defer func() { _ = file.Close() }()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(screenshotPath))
	if err != nil {
		return fmt.Errorf("create multipart file part: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("write multipart file part: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finalize multipart body: %w", err)
	}

	url := strings.TrimRight(config.CoreAPIBase, "/") + "/api/devicesync/screenshot"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Device-Id", config.ServerKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post screenshot: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func renderScreenshotOutputPath(pathTemplate string, now time.Time) string {
	const dateToken = "$(date +%F_%H-%M-%S)"
	return strings.ReplaceAll(pathTemplate, dateToken, now.Format("2006-01-02_15-04-05"))
}

// uniqueOutputPath returns path unchanged if no file exists at that location.
// When a file already exists, it appends an incrementing counter before the
// file extension (e.g. "cam_2026-01-02_09-00-00_1.jpg") until it finds a
// path that does not exist, preventing an existing pending screenshot from
// being silently overwritten.
func uniqueOutputPath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s_%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
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

// SignalSchedulerReload signals the scheduler to reload its configuration.
func SignalSchedulerReload() {
	select {
	case syncReloadChan <- struct{}{}:
	default:
	}
}
