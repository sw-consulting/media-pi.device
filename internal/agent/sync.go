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
	"sort"
	"strings"
	"sync"
	"time"
)

// ManifestItem represents a single video file in the sync manifest.
type ManifestItem struct {
	ID            string `json:"id"`
	Filename      string `json:"filename"`
	FileSizeBytes int64  `json:"fileSizeBytes"`
	SHA256        string `json:"sha256"`
}

// SyncStatus represents the current synchronization status.
type SyncStatus struct {
	LastSyncTime   time.Time `json:"lastSyncTime,omitempty"`
	LastSyncOK     bool      `json:"lastSyncOk"`
	LastSyncError  string    `json:"lastSyncError,omitempty"`
	SyncInProgress bool      `json:"syncInProgress"`
}

// SyncScheduleSettings represents the sync schedule configuration.
type SyncScheduleSettings struct {
	Times []string `json:"times"` // HH:MM format
}

var (
	// syncStatusMutex protects syncStatus
	syncStatusMutex sync.RWMutex
	syncStatus      = SyncStatus{}

	// syncInProgressMutex ensures only one sync runs at a time
	syncInProgressMutex sync.Mutex

	// syncScheduleMutex protects syncSchedule
	syncScheduleMutex sync.RWMutex
	syncSchedule      SyncScheduleSettings

	// syncSchedulePath is the path to the sync schedule configuration file
	syncSchedulePath = "/etc/systemd/system/sync.schedule.json"

	// syncStopChan is used to signal the scheduler to stop
	syncStopChan chan struct{}
	syncStopOnce sync.Once

	// HTTPClient can be overridden for testing
	HTTPClient HTTPClientInterface = &http.Client{
		Timeout: 5 * time.Minute,
	}
)

// HTTPClientInterface allows for mocking HTTP client in tests
type HTTPClientInterface interface {
	Do(req *http.Request) (*http.Response, error)
}

// StartSyncScheduler starts the background sync scheduler.
func StartSyncScheduler() {
	syncStopChan = make(chan struct{})
	
	// Load schedule from file if it exists
	if err := loadSyncSchedule(); err != nil {
		log.Printf("Warning: failed to load sync schedule: %v", err)
		// Continue with empty schedule
	}

	go func() {
		for {
			if !CurrentSyncConfig.Enabled {
				// Wait and check again
				select {
				case <-time.After(1 * time.Minute):
					continue
				case <-syncStopChan:
					return
				}
			}

			nextSync := calculateNextSyncTime()
			log.Printf("Next scheduled sync at: %s", nextSync.Format(time.RFC3339))

			select {
			case <-time.After(time.Until(nextSync)):
				log.Printf("Starting scheduled sync")
				if err := TriggerSync(); err != nil {
					log.Printf("Scheduled sync failed: %v", err)
				}
			case <-syncStopChan:
				log.Printf("Sync scheduler stopped")
				return
			}
		}
	}()
}

// StopSyncScheduler stops the background sync scheduler.
func StopSyncScheduler() {
	syncStopOnce.Do(func() {
		if syncStopChan != nil {
			close(syncStopChan)
		}
	})
}

// calculateNextSyncTime calculates when the next sync should occur based on schedule.
func calculateNextSyncTime() time.Time {
	syncScheduleMutex.RLock()
	times := syncSchedule.Times
	syncScheduleMutex.RUnlock()

	now := time.Now()
	
	// If no schedule is configured, use the interval
	if len(times) == 0 {
		return now.Add(time.Duration(CurrentSyncConfig.IntervalSeconds) * time.Second)
	}

	// Parse all scheduled times and find the next one
	var nextTime time.Time
	var nextDayTime time.Time
	
	for _, t := range times {
		parts := strings.Split(t, ":")
		if len(parts) != 2 {
			continue
		}
		
		var hour, minute int
		if _, err := fmt.Sscanf(t, "%d:%d", &hour, &minute); err != nil {
			continue
		}
		
		// Create time for today
		scheduledTime := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
		
		// If this time hasn't passed today, consider it
		if scheduledTime.After(now) {
			if nextTime.IsZero() || scheduledTime.Before(nextTime) {
				nextTime = scheduledTime
			}
		} else {
			// Consider this time for tomorrow
			tomorrowTime := scheduledTime.Add(24 * time.Hour)
			if nextDayTime.IsZero() || tomorrowTime.Before(nextDayTime) {
				nextDayTime = tomorrowTime
			}
		}
	}
	
	// Use today's next time if available, otherwise tomorrow's earliest
	if !nextTime.IsZero() {
		return nextTime
	}
	if !nextDayTime.IsZero() {
		return nextDayTime
	}
	
	// Fallback to interval-based scheduling
	return now.Add(time.Duration(CurrentSyncConfig.IntervalSeconds) * time.Second)
}

// TriggerSync triggers an on-demand synchronization.
func TriggerSync() error {
	// Try to acquire lock without blocking
	if !syncInProgressMutex.TryLock() {
		return fmt.Errorf("sync already in progress")
	}
	defer syncInProgressMutex.Unlock()

	// Set sync in progress
	setSyncInProgress(true)
	defer setSyncInProgress(false)

	// Perform the actual sync
	err := performSync()

	// Update status
	syncStatusMutex.Lock()
	syncStatus.LastSyncTime = time.Now()
	syncStatus.LastSyncOK = (err == nil)
	if err != nil {
		syncStatus.LastSyncError = err.Error()
	} else {
		syncStatus.LastSyncError = ""
	}
	syncStatusMutex.Unlock()

	return err
}

// GetSyncStatus returns the current sync status.
func GetSyncStatus() SyncStatus {
	syncStatusMutex.RLock()
	defer syncStatusMutex.RUnlock()
	return syncStatus
}

// setSyncInProgress sets the sync in progress flag.
func setSyncInProgress(inProgress bool) {
	syncStatusMutex.Lock()
	syncStatus.SyncInProgress = inProgress
	syncStatusMutex.Unlock()
}

// performSync performs the actual synchronization logic.
func performSync() error {
	if CoreAPIBase == "" {
		return fmt.Errorf("core_api_base not configured")
	}

	ctx := context.Background()

	// Fetch manifest from backend
	manifest, err := fetchManifest(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch manifest: %w", err)
	}

	// Ensure media directory exists
	if err := os.MkdirAll(MediaDir, 0755); err != nil {
		return fmt.Errorf("failed to create media directory: %w", err)
	}

	// Compare and download files
	if err := syncFiles(ctx, manifest); err != nil {
		return fmt.Errorf("failed to sync files: %w", err)
	}

	// Garbage collect files not in manifest
	if err := garbageCollect(manifest); err != nil {
		return fmt.Errorf("failed to garbage collect: %w", err)
	}

	return nil
}

// fetchManifest fetches the list of files from the backend.
func fetchManifest(ctx context.Context) ([]ManifestItem, error) {
	url := fmt.Sprintf("%s/api/devicesync", CoreAPIBase)
	
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	// Add authentication if available
	if DeviceAuthToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", DeviceAuthToken))
	}

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("backend returned status %d: %s", resp.StatusCode, string(body))
	}

	var manifest []ManifestItem
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, err
	}

	return manifest, nil
}

// syncFiles downloads missing or outdated files.
func syncFiles(ctx context.Context, manifest []ManifestItem) error {
	// Create a semaphore to limit parallel downloads
	sem := make(chan struct{}, CurrentSyncConfig.MaxParallelDownloads)
	errChan := make(chan error, len(manifest))
	var wg sync.WaitGroup

	for _, item := range manifest {
		wg.Add(1)
		go func(item ManifestItem) {
			defer wg.Done()
			
			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Check if file needs to be downloaded
			needsDownload, err := checkFileNeedsDownload(item)
			if err != nil {
				errChan <- fmt.Errorf("failed to check file %s: %w", item.Filename, err)
				return
			}

			if !needsDownload {
				log.Printf("File %s is up to date", item.Filename)
				return
			}

			// Download the file
			if err := downloadFile(ctx, item); err != nil {
				errChan <- fmt.Errorf("failed to download file %s: %w", item.Filename, err)
				return
			}

			log.Printf("Successfully downloaded %s", item.Filename)
		}(item)
	}

	wg.Wait()
	close(errChan)

	// Collect any errors
	var errs []string
	for err := range errChan {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf("sync errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

// checkFileNeedsDownload checks if a file needs to be downloaded.
func checkFileNeedsDownload(item ManifestItem) (bool, error) {
	filePath := filepath.Join(MediaDir, item.Filename)

	// Check if file exists
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	// Check file size
	if info.Size() != item.FileSizeBytes {
		return true, nil
	}

	// Check SHA256
	hash, err := calculateFileSHA256(filePath)
	if err != nil {
		return false, err
	}

	if hash != item.SHA256 {
		return true, nil
	}

	return false, nil
}

// calculateFileSHA256 calculates the SHA256 hash of a file.
func calculateFileSHA256(path string) (string, error) {
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

// downloadFile downloads a file from the backend with atomic write.
func downloadFile(ctx context.Context, item ManifestItem) error {
	url := fmt.Sprintf("%s/api/devicesync/%s", CoreAPIBase, item.ID)
	
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	// Add authentication if available
	if DeviceAuthToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", DeviceAuthToken))
	}

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("backend returned status %d: %s", resp.StatusCode, string(body))
	}

	// Write to temporary file
	tmpPath := filepath.Join(MediaDir, fmt.Sprintf(".%s.tmp", item.Filename))
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer func() {
		tmpFile.Close()
		os.Remove(tmpPath) // Clean up temp file if we don't rename it
	}()

	// Download with hash calculation
	h := sha256.New()
	tee := io.TeeReader(resp.Body, h)
	
	written, err := io.Copy(tmpFile, tee)
	if err != nil {
		return err
	}

	// Verify size
	if written != item.FileSizeBytes {
		return fmt.Errorf("size mismatch: expected %d bytes, got %d", item.FileSizeBytes, written)
	}

	// Verify hash
	hash := hex.EncodeToString(h.Sum(nil))
	if hash != item.SHA256 {
		return fmt.Errorf("hash mismatch: expected %s, got %s", item.SHA256, hash)
	}

	// Close temp file before rename
	if err := tmpFile.Close(); err != nil {
		return err
	}

	// Atomic rename
	finalPath := filepath.Join(MediaDir, item.Filename)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return err
	}

	return nil
}

// garbageCollect removes files from media directory that are not in the manifest.
func garbageCollect(manifest []ManifestItem) error {
	// Build set of valid filenames
	validFiles := make(map[string]struct{})
	for _, item := range manifest {
		validFiles[item.Filename] = struct{}{}
	}

	// List all files in media directory
	entries, err := os.ReadDir(MediaDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filename := entry.Name()
		
		// Skip temporary files
		if strings.HasPrefix(filename, ".") && strings.HasSuffix(filename, ".tmp") {
			continue
		}

		// Check if file is in manifest
		if _, ok := validFiles[filename]; !ok {
			filePath := filepath.Join(MediaDir, filename)
			log.Printf("Garbage collecting file: %s", filename)
			if err := os.Remove(filePath); err != nil {
				log.Printf("Warning: failed to remove %s: %v", filename, err)
			}
		}
	}

	return nil
}

// loadSyncSchedule loads the sync schedule from file.
func loadSyncSchedule() error {
	data, err := os.ReadFile(syncSchedulePath)
	if os.IsNotExist(err) {
		// No schedule file is OK, use defaults
		return nil
	}
	if err != nil {
		return err
	}

	var schedule SyncScheduleSettings
	if err := json.Unmarshal(data, &schedule); err != nil {
		return err
	}

	syncScheduleMutex.Lock()
	syncSchedule = schedule
	syncScheduleMutex.Unlock()

	return nil
}

// saveSyncSchedule saves the sync schedule to file.
func saveSyncSchedule(schedule SyncScheduleSettings) error {
	data, err := json.Marshal(schedule)
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(syncSchedulePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	if err := os.WriteFile(syncSchedulePath, data, 0644); err != nil {
		return err
	}

	syncScheduleMutex.Lock()
	syncSchedule = schedule
	syncScheduleMutex.Unlock()

	return nil
}

// GetSyncSchedule returns the current sync schedule.
func GetSyncSchedule() SyncScheduleSettings {
	syncScheduleMutex.RLock()
	defer syncScheduleMutex.RUnlock()
	
	// Return a copy
	times := make([]string, len(syncSchedule.Times))
	copy(times, syncSchedule.Times)
	
	return SyncScheduleSettings{Times: times}
}

// SetSyncSchedule updates the sync schedule.
func SetSyncSchedule(times []string) error {
	// Validate and normalize times
	normalized := make([]string, 0, len(times))
	for _, t := range times {
		parts := strings.Split(t, ":")
		if len(parts) != 2 {
			return fmt.Errorf("invalid time format: %s (expected HH:MM)", t)
		}
		
		var hour, minute int
		if _, err := fmt.Sscanf(t, "%d:%d", &hour, &minute); err != nil {
			return fmt.Errorf("invalid time format: %s (expected HH:MM)", t)
		}
		
		if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
			return fmt.Errorf("invalid time values: %s", t)
		}
		
		normalized = append(normalized, fmt.Sprintf("%02d:%02d", hour, minute))
	}
	
	// Sort times for consistency
	sort.Strings(normalized)
	
	return saveSyncSchedule(SyncScheduleSettings{Times: normalized})
}
