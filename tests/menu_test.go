// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sw-consulting/media-pi.device/internal/agent"
)

func TestHandleMenuList(t *testing.T) {
	agent.ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandleMenuList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp agent.APIResponse
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
	agent.ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodPost, "/api/menu", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandleMenuList(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestGetMenuActions(t *testing.T) {
	actions := agent.GetMenuActions()

	if len(actions) == 0 {
		t.Fatal("expected menu actions to be non-empty")
	}

	// Verify expected actions are present
	expectedIDs := []string{
		"playback-stop",
		"playback-start",
		"storage-check",
		"playlist-upload",
		"rest-time",
		"playlist-select",
		"schedule-get",
		"schedule-update",
		"audio-hdmi",
		"audio-jack",
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
}

func TestHandlePlaybackStopMethodNotAllowed(t *testing.T) {
	agent.ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/playback/stop", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandlePlaybackStop(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandlePlaybackStartMethodNotAllowed(t *testing.T) {
	agent.ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/playback/start", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandlePlaybackStart(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleStorageCheckMethodNotAllowed(t *testing.T) {
	agent.ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodPost, "/api/menu/storage/check", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandleStorageCheck(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandlePlaylistUploadMethodNotAllowed(t *testing.T) {
	agent.ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/playlist/upload", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandlePlaylistUpload(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleAudioHDMIMethodNotAllowed(t *testing.T) {
	agent.ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/audio/hdmi", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandleAudioHDMI(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleAudioJackMethodNotAllowed(t *testing.T) {
	agent.ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/audio/jack", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandleAudioJack(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleSystemReloadMethodNotAllowed(t *testing.T) {
	agent.ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/system/reload", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandleSystemReload(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleSystemRebootMethodNotAllowed(t *testing.T) {
	agent.ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/system/reboot", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandleSystemReboot(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleSystemShutdownMethodNotAllowed(t *testing.T) {
	agent.ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/system/shutdown", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandleSystemShutdown(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleSetRestTimeMethodNotAllowed(t *testing.T) {
	agent.ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/schedule/rest-time", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandleSetRestTime(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandlePlaylistSelectMethodNotAllowed(t *testing.T) {
	agent.ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/playlist/select", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandlePlaylistSelect(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleScheduleGetMethodNotAllowed(t *testing.T) {
	agent.ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodPost, "/api/menu/schedule/get", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandleScheduleGet(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleScheduleUpdateMethodNotAllowed(t *testing.T) {
	agent.ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/schedule/update", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandleScheduleUpdate(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleScheduleGetReturnsTimers(t *testing.T) {
	agent.ServerKey = "test-key"

	tmp := t.TempDir()
	playlistTimer := filepath.Join(tmp, "playlist.upload.timer")
	videoTimer := filepath.Join(tmp, "video.upload.timer")

	originalPlaylist := agent.PlaylistTimerPath
	originalVideo := agent.VideoTimerPath
	agent.PlaylistTimerPath = playlistTimer
	agent.VideoTimerPath = videoTimer
	t.Cleanup(func() {
		agent.PlaylistTimerPath = originalPlaylist
		agent.VideoTimerPath = originalVideo
	})

	playlistContent := ` [Unit]
Description = Playlist upload timer

[Timer]
OnCalendar=--* 12:32:00
OnCalendar=--* 16:28:00

[Install]
WantedBy=timers.target
`
	if err := os.WriteFile(playlistTimer, []byte(playlistContent), 0644); err != nil {
		t.Fatalf("failed to write playlist timer: %v", err)
	}

	videoContent := `[Unit]
Description = Video upload timer

[Timer]
OnCalendar=--* 22:22:00

[Install]
WantedBy=timers.target
`
	if err := os.WriteFile(videoTimer, []byte(videoContent), 0644); err != nil {
		t.Fatalf("failed to write video timer: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/menu/schedule/get", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandleScheduleGet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		OK   bool                   `json:"ok"`
		Data agent.ScheduleResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !resp.OK {
		t.Fatalf("expected ok response")
	}

	expectedPlaylist := []string{"12:32", "16:28"}
	if !reflect.DeepEqual(expectedPlaylist, resp.Data.Playlist) {
		t.Fatalf("unexpected playlist timers: %#v", resp.Data.Playlist)
	}

	expectedVideo := []string{"22:22"}
	if !reflect.DeepEqual(expectedVideo, resp.Data.Video) {
		t.Fatalf("unexpected video timers: %#v", resp.Data.Video)
	}
}

func TestHandleScheduleUpdateValidation(t *testing.T) {
	agent.ServerKey = "test-key"

	tmp := t.TempDir()
	playlistTimer := filepath.Join(tmp, "playlist.upload.timer")
	videoTimer := filepath.Join(tmp, "video.upload.timer")

	originalPlaylist := agent.PlaylistTimerPath
	originalVideo := agent.VideoTimerPath
	agent.PlaylistTimerPath = playlistTimer
	agent.VideoTimerPath = videoTimer
	t.Cleanup(func() {
		agent.PlaylistTimerPath = originalPlaylist
		agent.VideoTimerPath = originalVideo
	})

	req := httptest.NewRequest(http.MethodPut, "/api/menu/schedule/update", strings.NewReader(`{"playlist":["25:00"],"video":["08:00"]}`))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandleScheduleUpdate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleScheduleUpdateWritesTimers(t *testing.T) {
	agent.ServerKey = "test-key"

	tmp := t.TempDir()
	playlistTimer := filepath.Join(tmp, "playlist.upload.timer")
	videoTimer := filepath.Join(tmp, "video.upload.timer")

	originalPlaylist := agent.PlaylistTimerPath
	originalVideo := agent.VideoTimerPath
	agent.PlaylistTimerPath = playlistTimer
	agent.VideoTimerPath = videoTimer
	t.Cleanup(func() {
		agent.PlaylistTimerPath = originalPlaylist
		agent.VideoTimerPath = originalVideo
	})

	body := `{"playlist":["6:05","16:28"],"video":["22:22"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/menu/schedule/update", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandleScheduleUpdate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	playlistData, err := os.ReadFile(playlistTimer)
	if err != nil {
		t.Fatalf("failed to read playlist timer: %v", err)
	}

	if !strings.Contains(string(playlistData), "OnCalendar=--* 06:05:00") {
		t.Fatalf("expected normalized playlist time in file, got %s", string(playlistData))
	}

	videoData, err := os.ReadFile(videoTimer)
	if err != nil {
		t.Fatalf("failed to read video timer: %v", err)
	}

	if !strings.Contains(string(videoData), "OnCalendar=--* 22:22:00") {
		t.Fatalf("expected video time in file, got %s", string(videoData))
	}
}

func TestGetMenuActionsIncludesNewActions(t *testing.T) {
	actions := agent.GetMenuActions()

	expectedIDs := []string{
		"rest-time",
		"playlist-select",
		"schedule-get",
		"schedule-update",
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
}
