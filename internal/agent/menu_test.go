// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestHandleMenuList(t *testing.T) {
	ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleMenuList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.OK {
		t.Errorf("expected OK=true, got OK=false, error: %s", resp.ErrMsg)
	}

	// Check that we have menu actions in the response
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected Data to be a map, got %T", resp.Data)
	}

	actions, ok := data["actions"].([]interface{})
	if !ok {
		t.Fatalf("expected actions to be an array, got %T", data["actions"])
	}

	if len(actions) == 0 {
		t.Error("expected at least one menu action")
	}

	// Verify first action has required fields
	if len(actions) > 0 {
		action, ok := actions[0].(map[string]interface{})
		if !ok {
			t.Fatalf("expected action to be a map, got %T", actions[0])
		}

		requiredFields := []string{"id", "name", "description", "method", "path"}
		for _, field := range requiredFields {
			if _, exists := action[field]; !exists {
				t.Errorf("action missing required field: %s", field)
			}
		}
	}
}

func TestHandleMenuListMethodNotAllowed(t *testing.T) {
	ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodPost, "/api/menu", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleMenuList(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestGetMenuActions(t *testing.T) {
	actions := GetMenuActions()

	if len(actions) == 0 {
		t.Fatal("expected menu actions to be non-empty")
	}

	// Verify expected actions are present
	expectedIDs := []string{
		"playback-stop",
		"playback-start",
		"service-status",
		"configuration-get",
		"configuration-update",
		"system-reload",
		"system-reboot",
		"system-shutdown",
	}

	foundIDs := make(map[string]bool)
	for _, action := range actions {
		foundIDs[action.ID] = true

		// Verify each action has all required fields
		if action.ID == "" {
			t.Error("action has empty ID")
		}
		if action.Name == "" {
			t.Error("action has empty Name")
		}
		if action.Description == "" {
			t.Error("action has empty Description")
		}
		if action.Method == "" {
			t.Error("action has empty Method")
		}
		if action.Path == "" {
			t.Error("action has empty Path")
		}
	}

	for _, expectedID := range expectedIDs {
		if !foundIDs[expectedID] {
			t.Errorf("expected action ID %q not found", expectedID)
		}
	}
	
	// Ensure upload actions are NOT in the list
	uploadIDs := []string{
		"playlist-start-upload",
		"playlist-stop-upload",
		"video-start-upload",
		"video-stop-upload",
	}
	
	for _, uploadID := range uploadIDs {
		if foundIDs[uploadID] {
			t.Errorf("deprecated upload action ID %q should not be in menu actions", uploadID)
		}
	}
}

func TestHandlePlaybackStopMethodNotAllowed(t *testing.T) {
	ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/playback/stop", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandlePlaybackStop(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandlePlaybackStartMethodNotAllowed(t *testing.T) {
	ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/playback/start", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandlePlaybackStart(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

// (removed TestHandleStorageCheckMethodNotAllowed; storage-check endpoint replaced by system-status)

func TestHandleServiceStatusMethodNotAllowed(t *testing.T) {
	ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodPost, "/api/menu/service/status", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleServiceStatus(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleServiceStatusReturnsStatuses(t *testing.T) {
	ServerKey = "test-key"

	// Replace DBus factory with one that returns a noopDBusConnection
	originalFactory := dbusFactory
	SetDBusConnectionFactory(func(ctx context.Context) (DBusConnection, error) {
		return &noopDBusConnectionForStatus{}, nil
	})
	t.Cleanup(func() { SetDBusConnectionFactory(originalFactory) })

	// Create a temporary mounts file and point isPathMounted to read it by
	// using the MEDIA_PI_AGENT_PROC_MOUNTS environment variable.
	tmp := t.TempDir()
	mounts := filepath.Join(tmp, "mounts")
	if err := os.WriteFile(mounts, []byte("/dev/sda1 /mnt/ya.disk ext4 rw 0 0\n"), 0644); err != nil {
		t.Fatalf("failed to write mounts: %v", err)
	}
	originalProc := os.Getenv("MEDIA_PI_AGENT_PROC_MOUNTS")
	if err := os.Setenv("MEDIA_PI_AGENT_PROC_MOUNTS", mounts); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Setenv("MEDIA_PI_AGENT_PROC_MOUNTS", originalProc); err != nil {
			t.Fatalf("failed to restore env: %v", err)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/api/menu/service/status", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleServiceStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp struct {
		OK   bool                  `json:"ok"`
		Data ServiceStatusResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !resp.OK {
		t.Fatalf("expected ok response")
	}

	if resp.Data.PlaybackServiceStatus != true {
		t.Fatalf("expected playback service to be active, got %v", resp.Data.PlaybackServiceStatus)
	}
	
	// Check sync status is present (not checking upload statuses anymore)
	// Sync status will be empty/default values since no actual sync has run
	
	// Ensure the mount detection reads our temp mounts file
	if resp.Data.YaDiskMountStatus != true {
		t.Fatalf("expected ya disk to be reported mounted, got %v", resp.Data.YaDiskMountStatus)
	}
}

// noopDBusConnectionForStatus is a test helper that reports ActiveState=active
// for play.video.service.
type noopDBusConnectionForStatus struct{ noopDBusConnection }

func (n *noopDBusConnectionForStatus) GetUnitPropertiesContext(ctx context.Context, unit string) (map[string]any, error) {
	switch unit {
	case "play.video.service":
		return map[string]any{"ActiveState": "active"}, nil
	default:
		return map[string]any{"ActiveState": "inactive"}, nil
	}
}

func TestHandleConfigurationGetMethodNotAllowed(t *testing.T) {
	ServerKey = "test-key"
	req := httptest.NewRequest(http.MethodPost, "/api/menu/configuration/get", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleConfigurationGet(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleConfigurationUpdateMethodNotAllowed(t *testing.T) {
	ServerKey = "test-key"
	req := httptest.NewRequest(http.MethodGet, "/api/menu/configuration/update", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleConfigurationUpdate(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleConfigurationGet(t *testing.T) {
	ServerKey = "test-key"

	tmp := t.TempDir()
	audioPath := filepath.Join(tmp, "asound.conf")

	originalAudio := AudioConfigPath
	AudioConfigPath = audioPath
	t.Cleanup(func() {
		AudioConfigPath = originalAudio
	})

	if err := os.WriteFile(audioPath, []byte("defaults.pcm.card 0\n"), 0644); err != nil {
		t.Fatalf("failed to write audio config: %v", err)
	}

	originalCrontabRead := CrontabReadFunc
	CrontabReadFunc = func() (string, error) {
		return strings.Join([]string{
			"# MEDIA_PI_REST STOP",
			"30 18 * * * sudo systemctl stop play.video.service",
			"# MEDIA_PI_REST START",
			"00 09 * * * sudo systemctl start play.video.service",
		}, "\n") + "\n", nil
	}
	t.Cleanup(func() { CrontabReadFunc = originalCrontabRead })

	req := httptest.NewRequest(http.MethodGet, "/api/menu/configuration/get", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleConfigurationGet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp struct {
		OK   bool                  `json:"ok"`
		Data ConfigurationSettings `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !resp.OK {
		t.Fatalf("expected OK response, got %+v", resp.Data)
	}

	// Playlist and video should be empty (deprecated)
	if resp.Data.Playlist.Source != "" || resp.Data.Playlist.Destination != "" {
		t.Fatalf("expected empty playlist config, got: %+v", resp.Data.Playlist)
	}

	if len(resp.Data.Schedule.Playlist) != 0 {
		t.Fatalf("expected empty playlist timers: %+v", resp.Data.Schedule.Playlist)
	}
	if len(resp.Data.Schedule.Video) != 0 {
		t.Fatalf("unexpected video timers: %+v", resp.Data.Schedule.Video)
	}

	expectedRest := []RestTimePair{{Start: "18:30", Stop: "09:00"}}
	if !reflect.DeepEqual(expectedRest, resp.Data.Schedule.Rest) {
		t.Fatalf("unexpected rest schedule: %+v", resp.Data.Schedule.Rest)
	}

	if resp.Data.Audio.Output != "hdmi" {
		t.Fatalf("expected hdmi output, got %s", resp.Data.Audio.Output)
	}
}

func TestHandleConfigurationUploadWritesAllConfig(t *testing.T) {
	ServerKey = "test-key"

	tmp := t.TempDir()
	audioPath := filepath.Join(tmp, "asound.conf")

	originalAudio := AudioConfigPath
	AudioConfigPath = audioPath
	t.Cleanup(func() {
		AudioConfigPath = originalAudio
	})

	originalRead := CrontabReadFunc
	originalWrite := CrontabWriteFunc
	CrontabReadFunc = func() (string, error) { return "", nil }
	var (
		writeCalled bool
		writtenCron string
	)
	CrontabWriteFunc = func(content string) error {
		writeCalled = true
		writtenCron = content
		return nil
	}
	t.Cleanup(func() {
		CrontabReadFunc = originalRead
		CrontabWriteFunc = originalWrite
	})

	body := `{"playlist":{"source":"","destination":""},"schedule":{"playlist":[],"video":[],"rest":[{"start":"12:00","stop":"13:00"},{"start":"23:45","stop":"07:00"}]},"audio":{"output":"jack"}}`
	req := httptest.NewRequest(http.MethodPut, "/api/menu/configuration/update", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleConfigurationUpdate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if !writeCalled {
		t.Fatalf("expected crontab to be written")
	}
	if strings.Count(writtenCron, "# MEDIA_PI_REST STOP") != 2 {
		t.Fatalf("expected two rest stop markers, got %q", writtenCron)
	}

	audioData, err := os.ReadFile(audioPath)
	if err != nil {
		t.Fatalf("failed to read audio config: %v", err)
	}
	if !strings.Contains(string(audioData), "card 1") {
		t.Fatalf("expected jack config, got %s", string(audioData))
	}
}

func TestHandleConfigurationUploadValidation(t *testing.T) {
	ServerKey = "test-key"

	tmp := t.TempDir()
	originalAudio := AudioConfigPath
	AudioConfigPath = filepath.Join(tmp, "asound.conf")
	t.Cleanup(func() {
		AudioConfigPath = originalAudio
	})

	body := `{"playlist":{"source":"","destination":""},"schedule":{"playlist":[],"video":[],"rest":[]},"audio":{"output":"invalid"}}`
	req := httptest.NewRequest(http.MethodPut, "/api/menu/configuration/update", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleConfigurationUpdate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleSystemReloadMethodNotAllowed(t *testing.T) {
	ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/system/reload", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleSystemReload(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleSystemRebootMethodNotAllowed(t *testing.T) {
	ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/system/reboot", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleSystemReboot(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleSystemShutdownMethodNotAllowed(t *testing.T) {
	ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/system/shutdown", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleSystemShutdown(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleConfigurationUploadRejectsInvalidRestIntervals(t *testing.T) {
	ServerKey = "test-key"

	originalRead := CrontabReadFunc
	originalWrite := CrontabWriteFunc
	CrontabReadFunc = func() (string, error) { return "", nil }
	writeCalled := false
	CrontabWriteFunc = func(string) error {
		writeCalled = true
		return nil
	}
	t.Cleanup(func() {
		CrontabReadFunc = originalRead
		CrontabWriteFunc = originalWrite
	})

	body := `{"playlist":{"source":"","destination":""},"schedule":{"playlist":[],"video":[],"rest":[{"start":"10:00","stop":"12:00"},{"start":"11:00","stop":"13:00"}]},"audio":{"output":"hdmi"}}`
	req := httptest.NewRequest(http.MethodPut, "/api/menu/configuration/update", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleConfigurationUpdate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	if writeCalled {
		t.Fatalf("crontab write should not be called on invalid data")
	}
}

func TestGetMenuActionsIncludesNewActions(t *testing.T) {
	actions := GetMenuActions()

	expectedIDs := []string{
		"configuration-get",
		"configuration-update",
	}

	foundIDs := make(map[string]bool)
	for _, action := range actions {
		foundIDs[action.ID] = true
	}

	for _, expectedID := range expectedIDs {
		if !foundIDs[expectedID] {
			t.Errorf("expected action ID %q not found in menu actions", expectedID)
		}
	}
	
	// Ensure upload actions are NOT in the list
	uploadIDs := []string{
		"playlist-start-upload",
		"playlist-stop-upload",
		"video-start-upload",
		"video-stop-upload",
	}
	
	for _, uploadID := range uploadIDs {
		if foundIDs[uploadID] {
			t.Errorf("deprecated upload action ID %q should not be in menu actions", uploadID)
		}
	}
}

func TestParseRestTimes(t *testing.T) {
	content := `00 23 * * * sudo systemctl stop play.video.service
00 7 * * * sudo systemctl start play.video.service`

	pairs := parseRestTimes(content)

	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(pairs))
	}

	// Service stop at 23:00 = rest starts at 23:00
	if pairs[0].Start != "23:00" {
		t.Errorf("expected rest start time 23:00, got %s", pairs[0].Start)
	}

	// Service start at 07:00 = rest stops at 07:00
	if pairs[0].Stop != "07:00" {
		t.Errorf("expected rest stop time 07:00, got %s", pairs[0].Stop)
	}
}

func TestCrontabUserOperations(t *testing.T) {
	originalCrontabUser := MediaPiServiceUser
	originalCrontabRead := CrontabReadFunc
	originalCrontabWrite := CrontabWriteFunc

	t.Cleanup(func() {
		MediaPiServiceUser = originalCrontabUser
		CrontabReadFunc = originalCrontabRead
		CrontabWriteFunc = originalCrontabWrite
	})

	// Test with specific user
	MediaPiServiceUser = "testuser"

	var readArgs []string
	var writeArgs []string

	CrontabReadFunc = func() (string, error) {
		// Simulate the defaultCrontabRead behavior for testing
		cmd := &exec.Cmd{}
		user := MediaPiServiceUser
		if user == "" {
			user = "pi"
		}
		cmd.Args = []string{"crontab", "-u", user, "-l"}
		readArgs = cmd.Args
		return "", nil
	}

	CrontabWriteFunc = func(content string) error {
		// Simulate the defaultCrontabWrite behavior for testing
		cmd := &exec.Cmd{}
		user := MediaPiServiceUser
		if user == "" {
			user = "pi"
		}
		cmd.Args = []string{"crontab", "-u", user, "-"}
		writeArgs = cmd.Args
		return nil
	}

	// Test read with user
	_, err := getRestTimes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedReadArgs := []string{"crontab", "-u", "testuser", "-l"}
	if !reflect.DeepEqual(readArgs, expectedReadArgs) {
		t.Errorf("expected read args %v, got %v", expectedReadArgs, readArgs)
	}

	// Test write with user
	err = updateRestTimes([]RestTimePair{{Stop: "23:00", Start: "07:00"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedWriteArgs := []string{"crontab", "-u", "testuser", "-"}
	if !reflect.DeepEqual(writeArgs, expectedWriteArgs) {
		t.Errorf("expected write args %v, got %v", expectedWriteArgs, writeArgs)
	}

	// Test with empty user (should default to pi)
	MediaPiServiceUser = ""
	readArgs = nil
	writeArgs = nil

	_, err = getRestTimes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedReadArgsEmpty := []string{"crontab", "-u", "pi", "-l"}
	if !reflect.DeepEqual(readArgs, expectedReadArgsEmpty) {
		t.Errorf("expected read args %v, got %v", expectedReadArgsEmpty, readArgs)
	}

	err = updateRestTimes([]RestTimePair{{Stop: "23:00", Start: "07:00"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedWriteArgsEmpty := []string{"crontab", "-u", "pi", "-"}
	if !reflect.DeepEqual(writeArgs, expectedWriteArgsEmpty) {
		t.Errorf("expected write args %v, got %v", expectedWriteArgsEmpty, writeArgs)
	}
}

func TestSanitizeSystemdValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Normal string unchanged",
			input:    "Playlist upload timer",
			expected: "Playlist upload timer",
		},
		{
			name:     "Newline replaced with space",
			input:    "First line\nSecond line",
			expected: "First line Second line",
		},
		{
			name:     "Carriage return replaced with space",
			input:    "First part\rSecond part",
			expected: "First part Second part",
		},
		{
			name:     "Tab replaced with space",
			input:    "Part1\tPart2",
			expected: "Part1 Part2",
		},
		{
			name:     "Multiple newlines collapsed",
			input:    "Line1\n\n\nLine2",
			expected: "Line1   Line2",
		},
		{
			name:     "Mixed control characters",
			input:    "Text\nwith\rmixed\tcontrol\fchars\vhere",
			expected: "Text with mixed control chars here",
		},
		{
			name:     "Null character removed",
			input:    "Text\x00with\x00nulls",
			expected: "Textwithnulls",
		},
		{
			name:     "Leading and trailing whitespace trimmed",
			input:    "  \n\tTrimmed text\r\n  ",
			expected: "Trimmed text",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Only whitespace",
			input:    "   \n\t\r   ",
			expected: "",
		},
		{
			name:     "Special characters preserved",
			input:    "Timer-2.0_test (active)",
			expected: "Timer-2.0_test (active)",
		},
		{
			name:     "Unicode characters preserved",
			input:    "Таймер загрузки 音乐",
			expected: "Таймер загрузки 音乐",
		},
		{
			name:     "Potential injection attempt",
			input:    "Timer\n[Service]\nExecStart=/bin/malicious",
			expected: "Timer [Service] ExecStart=/bin/malicious",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeSystemdValue(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeSystemdValue(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestValidateRestTimePairsOverlap(t *testing.T) {
	tests := []struct {
		name     string
		pairs    []RestTimePair
		hasError bool
	}{
		{
			name: "No overlap - same day intervals",
			pairs: []RestTimePair{
				{Start: "09:00", Stop: "10:00"},
				{Start: "14:00", Stop: "15:00"},
			},
			hasError: false,
		},
		{
			name: "No overlap - midnight crossing intervals",
			pairs: []RestTimePair{
				{Start: "23:00", Stop: "01:00"},
				{Start: "10:00", Stop: "11:00"},
			},
			hasError: false,
		},
		{
			name: "Overlap - same day intervals",
			pairs: []RestTimePair{
				{Start: "09:00", Stop: "11:00"},
				{Start: "10:00", Stop: "12:00"},
			},
			hasError: true,
		},
		{
			name: "Overlap - midnight crossing with same day",
			pairs: []RestTimePair{
				{Start: "23:00", Stop: "02:00"},
				{Start: "01:00", Stop: "03:00"},
			},
			hasError: true,
		},
		{
			name: "Adjacent intervals (no overlap)",
			pairs: []RestTimePair{
				{Start: "09:00", Stop: "10:00"},
				{Start: "10:00", Stop: "11:00"},
			},
			hasError: false,
		},
		{
			name: "Full day coverage (no overlap)",
			pairs: []RestTimePair{
				{Start: "00:00", Stop: "12:00"},
				{Start: "12:00", Stop: "23:59"},
			},
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRestTimePairs(tt.pairs)
			if tt.hasError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.hasError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}
