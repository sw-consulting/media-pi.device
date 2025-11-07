// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
		"playlist-update-time",
		"video-update-time",
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

func TestHandlePlaylistUpdateTimeMethodNotAllowed(t *testing.T) {
	agent.ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/schedule/playlist-update", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandlePlaylistUpdateTime(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleVideoUpdateTimeMethodNotAllowed(t *testing.T) {
	agent.ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/schedule/video-update", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	agent.HandleVideoUpdateTime(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandlePlaylistUpdateTimeValidation(t *testing.T) {
	agent.ServerKey = "test-key"
	// override timer path so handler can write without root
	tmp := t.TempDir()
	tmpPlaylist := filepath.Join(tmp, "playlist.timer")
	agent.PlaylistTimerPath = tmpPlaylist

	// invalid time
	req := httptest.NewRequest(http.MethodPut, "/api/menu/schedule/playlist-update", strings.NewReader(`{"time":"25:00"}`))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()
	agent.HandlePlaylistUpdateTime(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid time, got %d", w.Code)
	}

	// valid time
	req = httptest.NewRequest(http.MethodPut, "/api/menu/schedule/playlist-update", strings.NewReader(`{"time":"08:30"}`))
	req.Header.Set("Authorization", "Bearer test-key")
	w = httptest.NewRecorder()
	agent.HandlePlaylistUpdateTime(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid time, got %d", w.Code)
	}
}

func TestHandleVideoUpdateTimeValidation(t *testing.T) {
	agent.ServerKey = "test-key"
	// override timer path so handler can write without root
	tmp := t.TempDir()
	tmpVideo := filepath.Join(tmp, "video.timer")
	agent.VideoTimerPath = tmpVideo

	// invalid time
	req := httptest.NewRequest(http.MethodPut, "/api/menu/schedule/video-update", strings.NewReader(`{"time":"99:99"}`))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()
	agent.HandleVideoUpdateTime(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid time, got %d", w.Code)
	}

	// valid time
	req = httptest.NewRequest(http.MethodPut, "/api/menu/schedule/video-update", strings.NewReader(`{"time":"23:59"}`))
	req.Header.Set("Authorization", "Bearer test-key")
	w = httptest.NewRecorder()
	agent.HandleVideoUpdateTime(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid time, got %d", w.Code)
	}
}

func TestGetMenuActionsIncludesNewActions(t *testing.T) {
	actions := agent.GetMenuActions()

	expectedIDs := []string{
		"rest-time",
		"playlist-select",
		"playlist-update-time",
		"video-update-time",
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
