// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
		"storage-check",
		"playlist-get",
		"playlist-update",
		"playlist-start-upload",
		"playlist-stop-upload",
		"schedule-get",
		"schedule-update",
		"audio-get",
		"audio-update",
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

func TestHandleStorageCheckMethodNotAllowed(t *testing.T) {
	ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodPost, "/api/menu/storage/check", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleStorageCheck(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandlePlaylistGetMethodNotAllowed(t *testing.T) {
	ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodPost, "/api/menu/playlist/get", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandlePlaylistGet(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandlePlaylistUpdateMethodNotAllowed(t *testing.T) {
	ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/playlist/update", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandlePlaylistUpdate(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleAudioGetMethodNotAllowed(t *testing.T) {
	ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodPost, "/api/menu/audio/get", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleAudioGet(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleAudioUpdateMethodNotAllowed(t *testing.T) {
	ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/audio/update", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleAudioUpdate(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandlePlaylistGet(t *testing.T) {
	ServerKey = "test-key"

	tmp := t.TempDir()
	servicePath := filepath.Join(tmp, "playlist.upload.service")

	originalServicePath := PlaylistServicePath
	PlaylistServicePath = servicePath
	t.Cleanup(func() { PlaylistServicePath = originalServicePath })

	content := `[Unit]
Description = Rsync playlist upload service
[Service]
ExecStart = /usr/bin/rsync -czavP /mnt/src/playlist/ /mnt/dst/playlist/ # nightly sync
[Install]
WantedBy = multi-user.target
`

	if err := os.WriteFile(servicePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write service file: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/menu/playlist/get", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandlePlaylistGet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.OK {
		t.Fatalf("expected OK response, got error: %s", resp.ErrMsg)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected response data to be a map, got %T", resp.Data)
	}

	if got := data["source"]; got != "/mnt/src/playlist/" {
		t.Fatalf("unexpected source, got %v", got)
	}
	if got := data["destination"]; got != "/mnt/dst/playlist/" {
		t.Fatalf("unexpected destination, got %v", got)
	}
}

func TestHandlePlaylistUpdate(t *testing.T) {
	ServerKey = "test-key"

	tmp := t.TempDir()
	servicePath := filepath.Join(tmp, "playlist.upload.service")

	originalServicePath := PlaylistServicePath
	PlaylistServicePath = servicePath
	t.Cleanup(func() { PlaylistServicePath = originalServicePath })

	content := `[Unit]
Description = Rsync playlist upload service
[Service]
ExecStart = /usr/bin/rsync -czavP /mnt/src/playlist/ /mnt/dst/playlist/ # nightly sync
ExecStartPost = /home/pi/videoplay.sh
[Install]
WantedBy = multi-user.target
`

	if err := os.WriteFile(servicePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write service file: %v", err)
	}

	body := strings.NewReader(`{"source":"/mnt/ya.disk/playlist/test/","destination":"/mnt/usb/playlist/"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/menu/playlist/update", body)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandlePlaylistUpdate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	updated, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("failed to read updated service file: %v", err)
	}

	if !strings.Contains(string(updated), "ExecStart = /usr/bin/rsync -czavP /mnt/ya.disk/playlist/test/ /mnt/usb/playlist/ # nightly sync") {
		t.Fatalf("updated service file does not contain new paths:\n%s", string(updated))
	}

	if !strings.Contains(string(updated), "# nightly sync") {
		t.Fatalf("updated service file lost inline comment:\n%s", string(updated))
	}
}

func TestHandleAudioGetAndUpdate(t *testing.T) {
	ServerKey = "test-key"

	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "asound.conf")

	original := AudioConfigPath
	AudioConfigPath = cfg
	t.Cleanup(func() { AudioConfigPath = original })

	// Initially file does not exist -> unknown
	req := httptest.NewRequest(http.MethodGet, "/api/menu/audio/get", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()
	HandleAudioGet(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data != "unknown" {
		t.Fatalf("expected unknown, got %#v", resp.Data)
	}

	// Update to hdmi
	body := strings.NewReader(`{"output":"hdmi"}`)
	req = httptest.NewRequest(http.MethodPut, "/api/menu/audio/update", body)
	req.Header.Set("Authorization", "Bearer test-key")
	w = httptest.NewRecorder()
	HandleAudioUpdate(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on update, got %d", w.Code)
	}

	// Verify get returns hdmi
	req = httptest.NewRequest(http.MethodGet, "/api/menu/audio/get", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w = httptest.NewRecorder()
	HandleAudioGet(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data != "hdmi" {
		t.Fatalf("expected hdmi, got %#v", resp.Data)
	}

	// Update to jack
	body = strings.NewReader(`{"output":"jack"}`)
	req = httptest.NewRequest(http.MethodPut, "/api/menu/audio/update", body)
	req.Header.Set("Authorization", "Bearer test-key")
	w = httptest.NewRecorder()
	HandleAudioUpdate(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on update, got %d", w.Code)
	}

	// Verify get returns jack
	req = httptest.NewRequest(http.MethodGet, "/api/menu/audio/get", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w = httptest.NewRecorder()
	HandleAudioGet(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data != "jack" {
		t.Fatalf("expected jack, got %#v", resp.Data)
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

func TestHandlePlaylistStartStopMethodNotAllowed(t *testing.T) {
	ServerKey = "test-key"

	// start-upload expects POST
	req := httptest.NewRequest(http.MethodGet, "/api/menu/playlist/start-upload", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()
	HandlePlaylistStartUpload(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405 for start-upload, got %d", w.Code)
	}

	// stop-upload expects POST
	req2 := httptest.NewRequest(http.MethodGet, "/api/menu/playlist/stop-upload", nil)
	req2.Header.Set("Authorization", "Bearer test-key")
	w2 := httptest.NewRecorder()
	HandlePlaylistStopUpload(w2, req2)
	if w2.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405 for stop-upload, got %d", w2.Code)
	}
}

func TestHandleScheduleGetMethodNotAllowed(t *testing.T) {
	ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodPost, "/api/menu/schedule/get", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleScheduleGet(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleScheduleUpdateMethodNotAllowed(t *testing.T) {
	ServerKey = "test-key"

	req := httptest.NewRequest(http.MethodGet, "/api/menu/schedule/update", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleScheduleUpdate(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleScheduleGetReturnsTimers(t *testing.T) {
	ServerKey = "test-key"

	tmp := t.TempDir()
	playlistTimer := filepath.Join(tmp, "playlist.upload.timer")
	videoTimer := filepath.Join(tmp, "video.upload.timer")

	originalPlaylist := PlaylistTimerPath
	originalVideo := VideoTimerPath
	PlaylistTimerPath = playlistTimer
	VideoTimerPath = videoTimer
	t.Cleanup(func() {
		PlaylistTimerPath = originalPlaylist
		VideoTimerPath = originalVideo
	})

	originalCrontabRead := CrontabReadFunc
	CrontabReadFunc = func() (string, error) {
		return "", nil
	}
	t.Cleanup(func() {
		CrontabReadFunc = originalCrontabRead
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

	HandleScheduleGet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		OK   bool             `json:"ok"`
		Data ScheduleResponse `json:"data"`
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

	if len(resp.Data.Rest) != 0 {
		t.Fatalf("expected empty rest timers, got %#v", resp.Data.Rest)
	}
}

func TestHandleScheduleGetIncludesRestTimes(t *testing.T) {
	ServerKey = "test-key"

	tmp := t.TempDir()
	playlistTimer := filepath.Join(tmp, "playlist.upload.timer")
	videoTimer := filepath.Join(tmp, "video.upload.timer")

	originalPlaylist := PlaylistTimerPath
	originalVideo := VideoTimerPath
	PlaylistTimerPath = playlistTimer
	VideoTimerPath = videoTimer
	t.Cleanup(func() {
		PlaylistTimerPath = originalPlaylist
		VideoTimerPath = originalVideo
	})

	playlistContent := ` [Unit]
Description = Playlist upload timer

[Timer]
OnCalendar=--* 12:32:00

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

	originalCrontabRead := CrontabReadFunc
	CrontabReadFunc = func() (string, error) {
		return strings.Join([]string{
			"# MEDIA_PI_REST STOP",
			"30 18 * * * sudo systemctl stop play.video.service",
			"# MEDIA_PI_REST START",
			"00 09 * * * sudo systemctl start play.video.service",
		}, "\n") + "\n", nil
	}
	t.Cleanup(func() {
		CrontabReadFunc = originalCrontabRead
	})

	req := httptest.NewRequest(http.MethodGet, "/api/menu/schedule/get", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleScheduleGet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		OK   bool             `json:"ok"`
		Data ScheduleResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !resp.OK {
		t.Fatalf("expected ok response")
	}

	expectedRest := []RestTimePair{{Stop: "18:30", Start: "09:00"}}
	if !reflect.DeepEqual(expectedRest, resp.Data.Rest) {
		t.Fatalf("unexpected rest schedule: %#v", resp.Data.Rest)
	}
}

func TestHandleScheduleUpdateValidation(t *testing.T) {
	ServerKey = "test-key"

	tmp := t.TempDir()
	playlistTimer := filepath.Join(tmp, "playlist.upload.timer")
	videoTimer := filepath.Join(tmp, "video.upload.timer")

	originalPlaylist := PlaylistTimerPath
	originalVideo := VideoTimerPath
	PlaylistTimerPath = playlistTimer
	VideoTimerPath = videoTimer
	t.Cleanup(func() {
		PlaylistTimerPath = originalPlaylist
		VideoTimerPath = originalVideo
	})

	req := httptest.NewRequest(http.MethodPut, "/api/menu/schedule/update", strings.NewReader(`{"playlist":["25:00"],"video":["08:00"]}`))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleScheduleUpdate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleScheduleUpdateWritesTimersAndRest(t *testing.T) {
	ServerKey = "test-key"

	tmp := t.TempDir()
	playlistTimer := filepath.Join(tmp, "playlist.upload.timer")
	videoTimer := filepath.Join(tmp, "video.upload.timer")

	originalPlaylist := PlaylistTimerPath
	originalVideo := VideoTimerPath
	PlaylistTimerPath = playlistTimer
	VideoTimerPath = videoTimer
	t.Cleanup(func() {
		PlaylistTimerPath = originalPlaylist
		VideoTimerPath = originalVideo
	})

	originalRead := CrontabReadFunc
	originalWrite := CrontabWriteFunc
	t.Cleanup(func() {
		CrontabReadFunc = originalRead
		CrontabWriteFunc = originalWrite
	})

	existing := strings.Join([]string{
		"5 4 * * * /usr/bin/echo 'hello'",
		"# MEDIA_PI_REST STOP",
		"15 20 * * * sudo systemctl stop play.video.service",
		"# MEDIA_PI_REST START",
		"45 21 * * * sudo systemctl start play.video.service",
	}, "\n") + "\n"

	CrontabReadFunc = func() (string, error) {
		return existing, nil
	}

	var (
		writeCalled bool
		writtenCron string
	)
	CrontabWriteFunc = func(content string) error {
		writeCalled = true
		writtenCron = content
		return nil
	}

	body := `{"playlist":["6:05","16:28"],"video":["22:22"],"rest":[{"start":"13:00","stop":"12:00"},{"start":"07:00","stop":"23:45"}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/menu/schedule/update", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleScheduleUpdate(w, req)

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

	if !writeCalled {
		t.Fatalf("expected crontab to be written")
	}

	if strings.Contains(writtenCron, "15 20 * * * sudo systemctl stop play.video.service") {
		t.Fatalf("expected old rest stop entry to be removed, got %q", writtenCron)
	}

	if strings.Count(writtenCron, "# MEDIA_PI_REST STOP") != 2 {
		t.Fatalf("expected two rest stop markers, got %q", writtenCron)
	}

	if !strings.Contains(writtenCron, "00 12 * * * sudo systemctl stop play.video.service") {
		t.Fatalf("expected stop entry for 12:00, got %q", writtenCron)
	}

	if !strings.Contains(writtenCron, "00 13 * * * sudo systemctl start play.video.service") {
		t.Fatalf("expected start entry for 13:00, got %q", writtenCron)
	}

	if !strings.Contains(writtenCron, "45 23 * * * sudo systemctl stop play.video.service") {
		t.Fatalf("expected stop entry for 23:45, got %q", writtenCron)
	}

	if !strings.Contains(writtenCron, "00 07 * * * sudo systemctl start play.video.service") {
		t.Fatalf("expected start entry for 07:00, got %q", writtenCron)
	}
}

func TestHandleScheduleUpdateClearsRest(t *testing.T) {
	ServerKey = "test-key"

	tmp := t.TempDir()
	playlistTimer := filepath.Join(tmp, "playlist.upload.timer")
	videoTimer := filepath.Join(tmp, "video.upload.timer")

	originalPlaylist := PlaylistTimerPath
	originalVideo := VideoTimerPath
	PlaylistTimerPath = playlistTimer
	VideoTimerPath = videoTimer
	t.Cleanup(func() {
		PlaylistTimerPath = originalPlaylist
		VideoTimerPath = originalVideo
	})

	originalRead := CrontabReadFunc
	originalWrite := CrontabWriteFunc
	t.Cleanup(func() {
		CrontabReadFunc = originalRead
		CrontabWriteFunc = originalWrite
	})

	existing := strings.Join([]string{
		"# MEDIA_PI_REST STOP",
		"15 20 * * * sudo systemctl stop play.video.service",
		"# MEDIA_PI_REST START",
		"45 21 * * * sudo systemctl start play.video.service",
	}, "\n") + "\n"

	CrontabReadFunc = func() (string, error) {
		return existing, nil
	}

	var (
		writeCalled bool
		writtenCron string
	)
	CrontabWriteFunc = func(content string) error {
		writeCalled = true
		writtenCron = content
		return nil
	}

	body := `{"playlist":["6:05"],"video":["22:22"],"rest":[]}`
	req := httptest.NewRequest(http.MethodPut, "/api/menu/schedule/update", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleScheduleUpdate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	if !writeCalled {
		t.Fatalf("expected crontab to be written")
	}

	if strings.Contains(writtenCron, "# MEDIA_PI_REST STOP") || strings.Contains(writtenCron, "# MEDIA_PI_REST START") {
		t.Fatalf("expected rest markers to be removed, got %q", writtenCron)
	}
}

func TestHandleScheduleUpdateRejectsInvalidRestIntervals(t *testing.T) {
	ServerKey = "test-key"

	tmp := t.TempDir()
	playlistTimer := filepath.Join(tmp, "playlist.upload.timer")
	videoTimer := filepath.Join(tmp, "video.upload.timer")

	originalPlaylist := PlaylistTimerPath
	originalVideo := VideoTimerPath
	PlaylistTimerPath = playlistTimer
	VideoTimerPath = videoTimer
	t.Cleanup(func() {
		PlaylistTimerPath = originalPlaylist
		VideoTimerPath = originalVideo
	})

	originalRead := CrontabReadFunc
	originalWrite := CrontabWriteFunc
	t.Cleanup(func() {
		CrontabReadFunc = originalRead
		CrontabWriteFunc = originalWrite
	})

	CrontabReadFunc = func() (string, error) {
		return "", nil
	}

	writeCalled := false
	CrontabWriteFunc = func(string) error {
		writeCalled = true
		return nil
	}

	body := `{"playlist":["6:05"],"video":["22:22"],"rest":[{"start":"12:00","stop":"10:00"},{"start":"13:00","stop":"11:00"}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/menu/schedule/update", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleScheduleUpdate(w, req)

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
		"playlist-get",
		"playlist-update",
		"playlist-start-upload",
		"playlist-stop-upload",
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
