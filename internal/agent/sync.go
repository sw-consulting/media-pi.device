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
)

// Configurable paths for sync files. Tests may override these to point to
// temporary locations.
var (
	syncSchedulePath = "/etc/systemd/system/sync.schedule.json"
	syncStatusPath   = "/var/lib/media-pi/sync.status.json"
)

// ManifestItem represents a single file entry from the backend manifest.
type ManifestItem struct {
	ID            string `json:"id"`
	Filename      string `json:"filename"`
	FileSizeBytes int64  `json:"fileSizeBytes"`
	SHA256        string `json:"sha256"`
}

// SyncSchedule stores the configured sync times.
type SyncSchedule struct {
	Times []string `json:"times"` // HH:MM format, e.g., ["03:00", "15:00"]
}

// SyncStatus tracks the last sync operation.
type SyncStatus struct {
	LastSyncTime time.Time `json:"lastSyncTime"`
	OK           bool      `json:"ok"`
	Error        string    `json:"error,omitempty"`
}

var (
	// syncInProgressMutex ensures only one sync runs at a time.
	syncInProgressMutex sync.Mutex

	// syncStatus holds the current sync status in memory.
	syncStatus     SyncStatus
	syncStatusLock sync.RWMutex

	// syncStopChan is used to signal the scheduler to stop.
	syncStopChan chan struct{}
	syncStopOnce sync.Once

	// syncSchedulerRunning tracks whether the scheduler is active.
	syncSchedulerRunning     bool
	syncSchedulerRunningLock sync.Mutex
)

// SetSyncSchedule saves the sync schedule to disk.
func SetSyncSchedule(times []string) error {
	schedule := SyncSchedule{Times: times}
	data, err := json.MarshalIndent(schedule, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal sync schedule: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(syncSchedulePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory for sync schedule: %w", err)
	}

	if err := os.WriteFile(syncSchedulePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write sync schedule: %w", err)
	}

	return nil
}

// GetSyncSchedule loads the sync schedule from disk.
func GetSyncSchedule() ([]string, error) {
	data, err := os.ReadFile(syncSchedulePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read sync schedule: %w", err)
	}

	var schedule SyncSchedule
	if err := json.Unmarshal(data, &schedule); err != nil {
		return nil, fmt.Errorf("failed to unmarshal sync schedule: %w", err)
	}

	return schedule.Times, nil
}

// GetSyncStatus returns the current sync status.
func GetSyncStatus() SyncStatus {
	syncStatusLock.RLock()
	defer syncStatusLock.RUnlock()
	return syncStatus
}

// setSyncStatus updates the sync status and persists it to disk.
func setSyncStatus(status SyncStatus) {
	syncStatusLock.Lock()
	syncStatus = status
	syncStatusLock.Unlock()

	// Persist to disk (best effort)
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		log.Printf("Warning: failed to marshal sync status: %v", err)
		return
	}

	if err := os.MkdirAll(filepath.Dir(syncStatusPath), 0755); err != nil {
		log.Printf("Warning: failed to create directory for sync status: %v", err)
		return
	}

	if err := os.WriteFile(syncStatusPath, data, 0644); err != nil {
		log.Printf("Warning: failed to write sync status: %v", err)
	}
}

// fetchManifest retrieves the file manifest from the backend API.
func fetchManifest(ctx context.Context) ([]ManifestItem, error) {
	if CoreAPIBase == "" {
		return nil, fmt.Errorf("core_api_base not configured")
	}

	url := CoreAPIBase + "/api/devicesync"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if DeviceAuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+DeviceAuthToken)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var manifest []ManifestItem
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	return manifest, nil
}

// downloadFile downloads a single file from the backend and verifies it.
func downloadFile(ctx context.Context, item ManifestItem, destPath string) error {
	if CoreAPIBase == "" {
		return fmt.Errorf("core_api_base not configured")
	}

	url := fmt.Sprintf("%s/api/devicesync/%s", CoreAPIBase, item.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}

	if DeviceAuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+DeviceAuthToken)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Create temporary file for atomic write
	tempPath := destPath + ".tmp"
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		tempFile.Close()
		os.Remove(tempPath) // Clean up on error
	}()

	// Download and compute hash simultaneously
	hasher := sha256.New()
	writer := io.MultiWriter(tempFile, hasher)

	bytesWritten, err := io.Copy(writer, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Verify file size
	if bytesWritten != item.FileSizeBytes {
		return fmt.Errorf("file size mismatch: expected %d, got %d", item.FileSizeBytes, bytesWritten)
	}

	// Verify SHA256
	computedHash := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(computedHash, item.SHA256) {
		return fmt.Errorf("SHA256 mismatch: expected %s, got %s", item.SHA256, computedHash)
	}

	// Atomic rename
	if err := os.Rename(tempPath, destPath); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// verifyLocalFile checks if a local file matches the manifest item.
func verifyLocalFile(path string, item ManifestItem) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	if info.Size() != item.FileSizeBytes {
		return false
	}

	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return false
	}

	computedHash := hex.EncodeToString(hasher.Sum(nil))
	return strings.EqualFold(computedHash, item.SHA256)
}

// syncFiles performs the actual file synchronization.
func syncFiles(ctx context.Context) error {
	// Ensure media directory exists
	if err := os.MkdirAll(MediaDir, 0755); err != nil {
		return fmt.Errorf("failed to create media directory: %w", err)
	}

	// Fetch manifest from backend
	manifest, err := fetchManifest(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch manifest: %w", err)
	}

	log.Printf("Fetched manifest with %d items", len(manifest))

	// Build map of expected files
	expectedFiles := make(map[string]ManifestItem)
	for _, item := range manifest {
		expectedFiles[item.Filename] = item
	}

	// Collect files that need to be downloaded
	var toDownload []ManifestItem
	for _, item := range manifest {
		destPath := filepath.Join(MediaDir, item.Filename)
		if !verifyLocalFile(destPath, item) {
			toDownload = append(toDownload, item)
		}
	}

	log.Printf("Found %d files to download", len(toDownload))

	// Download files with parallelization
	if len(toDownload) > 0 {
		sem := make(chan struct{}, MaxParallelDownloads)
		var wg sync.WaitGroup
		var errMutex sync.Mutex
		var firstErr error

		for _, item := range toDownload {
			wg.Add(1)
			go func(item ManifestItem) {
				defer wg.Done()

				sem <- struct{}{} // Acquire semaphore
				defer func() { <-sem }()

				destPath := filepath.Join(MediaDir, item.Filename)
				log.Printf("Downloading %s...", item.Filename)

				if err := downloadFile(ctx, item, destPath); err != nil {
					log.Printf("Error downloading %s: %v", item.Filename, err)
					errMutex.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMutex.Unlock()
				} else {
					log.Printf("Successfully downloaded %s", item.Filename)
				}
			}(item)
		}

		wg.Wait()

		if firstErr != nil {
			return fmt.Errorf("download errors occurred: %w", firstErr)
		}
	}

	// Garbage collect files not in manifest
	entries, err := os.ReadDir(MediaDir)
	if err != nil {
		return fmt.Errorf("failed to read media directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		if _, expected := expectedFiles[filename]; !expected {
			path := filepath.Join(MediaDir, filename)
			log.Printf("Removing file not in manifest: %s", filename)
			if err := os.Remove(path); err != nil {
				log.Printf("Warning: failed to remove %s: %v", filename, err)
			}
		}
	}

	return nil
}

// TriggerSync manually triggers a sync operation.
func TriggerSync(ctx context.Context) error {
	if !syncInProgressMutex.TryLock() {
		return fmt.Errorf("sync already in progress")
	}
	defer syncInProgressMutex.Unlock()

	log.Println("Starting sync...")

	startTime := time.Now()
	err := syncFiles(ctx)

	status := SyncStatus{
		LastSyncTime: startTime,
		OK:           err == nil,
	}
	if err != nil {
		status.Error = err.Error()
		log.Printf("Sync failed: %v", err)
	} else {
		log.Println("Sync completed successfully")
	}

	setSyncStatus(status)
	return err
}

// calculateNextSyncTime computes the next scheduled sync time based on the schedule.
func calculateNextSyncTime(now time.Time, schedule []string) time.Time {
	if len(schedule) == 0 {
		return time.Time{} // Zero time indicates no schedule
	}

	var nextTime time.Time
	for _, timeStr := range schedule {
		parts := strings.Split(timeStr, ":")
		if len(parts) != 2 {
			continue
		}

		var hour, minute int
		if _, err := fmt.Sscanf(timeStr, "%d:%d", &hour, &minute); err != nil {
			continue
		}

		// Calculate today's occurrence
		scheduledTime := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())

		// If it's in the past today, try tomorrow
		if scheduledTime.Before(now) || scheduledTime.Equal(now) {
			scheduledTime = scheduledTime.Add(24 * time.Hour)
		}

		// Keep the earliest upcoming time
		if nextTime.IsZero() || scheduledTime.Before(nextTime) {
			nextTime = scheduledTime
		}
	}

	return nextTime
}

// StartSyncScheduler starts the sync scheduler loop.
func StartSyncScheduler(ctx context.Context) {
	syncSchedulerRunningLock.Lock()
	if syncSchedulerRunning {
		syncSchedulerRunningLock.Unlock()
		return
	}
	syncSchedulerRunning = true
	syncStopChan = make(chan struct{})
	syncSchedulerRunningLock.Unlock()

	go func() {
		log.Println("Sync scheduler started")
		defer func() {
			syncSchedulerRunningLock.Lock()
			syncSchedulerRunning = false
			syncSchedulerRunningLock.Unlock()
			log.Println("Sync scheduler stopped")
		}()

		for {
			schedule, err := GetSyncSchedule()
			if err != nil {
				log.Printf("Error reading sync schedule: %v", err)
			}

			nextSync := calculateNextSyncTime(time.Now(), schedule)
			if nextSync.IsZero() {
				// No schedule configured, wait indefinitely
				select {
				case <-syncStopChan:
					return
				case <-ctx.Done():
					return
				}
			}

			waitDuration := time.Until(nextSync)
			log.Printf("Next sync scheduled at %s (in %v)", nextSync.Format(time.RFC3339), waitDuration)

			timer := time.NewTimer(waitDuration)
			select {
			case <-timer.C:
				log.Println("Scheduled sync triggered")
				if err := TriggerSync(ctx); err != nil {
					log.Printf("Scheduled sync error: %v", err)
				}
			case <-syncStopChan:
				timer.Stop()
				return
			case <-ctx.Done():
				timer.Stop()
				return
			}
		}
	}()
}

// StopSyncScheduler stops the sync scheduler.
func StopSyncScheduler() {
	syncSchedulerRunningLock.Lock()
	running := syncSchedulerRunning
	syncSchedulerRunningLock.Unlock()

	if running {
		syncStopOnce.Do(func() {
			close(syncStopChan)
		})
		// Reset the sync.Once for next start
		syncStopOnce = sync.Once{}
	}
}

// IsSyncSchedulerRunning returns whether the scheduler is currently active.
func IsSyncSchedulerRunning() bool {
	syncSchedulerRunningLock.Lock()
	defer syncSchedulerRunningLock.Unlock()
	return syncSchedulerRunning
}
