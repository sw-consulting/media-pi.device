// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package agent

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleConfigurationUpdate_DualWrite(t *testing.T) {
	ServerKey = "test-key"

	tmp := t.TempDir()
	servicePath := filepath.Join(tmp, "playlist.upload.service")
	playlistTimer := filepath.Join(tmp, "playlist.upload.timer")
	videoTimer := filepath.Join(tmp, "video.upload.timer")
	audioPath := filepath.Join(tmp, "asound.conf")
	configPath := filepath.Join(tmp, "agent.yaml")

	originalServicePath := PlaylistServicePath
	originalPlaylist := PlaylistTimerPath
	originalVideo := VideoTimerPath
	originalAudio := AudioConfigPath
	originalConfigPath := ConfigPath
	PlaylistServicePath = servicePath
	PlaylistTimerPath = playlistTimer
	VideoTimerPath = videoTimer
	AudioConfigPath = audioPath
	ConfigPath = configPath
	t.Cleanup(func() {
		PlaylistServicePath = originalServicePath
		PlaylistTimerPath = originalPlaylist
		VideoTimerPath = originalVideo
		AudioConfigPath = originalAudio
		ConfigPath = originalConfigPath
	})

	// Create initial systemd service file
	serviceContent := `[Unit]
Description = Rsync playlist upload service
[Service]
ExecStart = /usr/bin/rsync -czavP /old/src/ /old/dst/
[Install]
WantedBy = multi-user.target
`
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		t.Fatalf("failed to write service file: %v", err)
	}

	// Set up test config
	testConfig := &Config{
		AllowedUnits:       []string{"test.service"},
		ServerKey:          "test-key",
		ListenAddr:         "0.0.0.0:8081",
		MediaPiServiceUser: "pi",
		Playlist: PlaylistConfig{
			Source:      "/old/src/",
			Destination: "/old/dst/",
		},
		Schedule: ScheduleConfig{
			Playlist: []string{"10:00"},
			Video:    []string{"20:00"},
		},
		Audio: AudioConfig{
			Output: "hdmi",
		},
	}

	configMutex.Lock()
	originalConfig := currentConfig
	currentConfig = testConfig
	configMutex.Unlock()
	t.Cleanup(func() {
		configMutex.Lock()
		currentConfig = originalConfig
		configMutex.Unlock()
	})

	originalCrontabWrite := CrontabWriteFunc
	originalCrontabRead := CrontabReadFunc
	CrontabWriteFunc = func(content string) error {
		return nil // Mock crontab write
	}
	CrontabReadFunc = func() (string, error) {
		return "", nil // Mock crontab read
	}
	t.Cleanup(func() {
		CrontabWriteFunc = originalCrontabWrite
		CrontabReadFunc = originalCrontabRead
	})

	// Make request to update configuration
	reqBody := ConfigurationSettings{
		Playlist: PlaylistUploadConfig{
			Source:      "/new/src/",
			Destination: "/new/dst/",
		},
		Schedule: ScheduleSettings{
			Playlist: []string{"11:30", "14:45"},
			Video:    []string{"21:00"},
			Rest:     []RestTimePair{{Start: "22:00", Stop: "08:00"}},
		},
		Audio: AudioSettings{
			Output: "jack",
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/menu/configuration/update", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleConfigurationUpdate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify systemd service file was updated
	serviceData, err := os.ReadFile(servicePath)
	if err != nil {
		t.Fatalf("failed to read service file: %v", err)
	}
	if !strings.Contains(string(serviceData), "/new/src/") || !strings.Contains(string(serviceData), "/new/dst/") {
		t.Errorf("service file not updated correctly: %s", string(serviceData))
	}

	// Verify timer files were updated
	playlistData, err := os.ReadFile(playlistTimer)
	if err != nil {
		t.Fatalf("failed to read playlist timer: %v", err)
	}
	if !strings.Contains(string(playlistData), "11:30:00") || !strings.Contains(string(playlistData), "14:45:00") {
		t.Errorf("playlist timer not updated correctly: %s", string(playlistData))
	}

	videoData, err := os.ReadFile(videoTimer)
	if err != nil {
		t.Fatalf("failed to read video timer: %v", err)
	}
	if !strings.Contains(string(videoData), "21:00:00") {
		t.Errorf("video timer not updated correctly: %s", string(videoData))
	}

	// Verify audio config was updated
	audioData, err := os.ReadFile(audioPath)
	if err != nil {
		t.Fatalf("failed to read audio config: %v", err)
	}
	if !strings.Contains(string(audioData), "card 1") {
		t.Errorf("audio config not updated correctly: %s", string(audioData))
	}

	// Verify agent config was updated
	cfg := GetCurrentConfig()
	if cfg.Playlist.Source != "/new/src/" {
		t.Errorf("config source not updated: %s", cfg.Playlist.Source)
	}
	if cfg.Playlist.Destination != "/new/dst/" {
		t.Errorf("config destination not updated: %s", cfg.Playlist.Destination)
	}
	if len(cfg.Schedule.Playlist) != 2 || cfg.Schedule.Playlist[0] != "11:30" {
		t.Errorf("config playlist schedule not updated: %v", cfg.Schedule.Playlist)
	}
	if len(cfg.Schedule.Video) != 1 || cfg.Schedule.Video[0] != "21:00" {
		t.Errorf("config video schedule not updated: %v", cfg.Schedule.Video)
	}
	if len(cfg.Schedule.Rest) != 1 || cfg.Schedule.Rest[0].Start != "22:00" {
		t.Errorf("config rest times not updated: %v", cfg.Schedule.Rest)
	}
	if cfg.Audio.Output != "jack" {
		t.Errorf("config audio not updated: %s", cfg.Audio.Output)
	}

	// Verify config file was written
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("config file was not written: %v", err)
	}
}

func TestHandleScheduleUpdate_DualWrite(t *testing.T) {
	ServerKey = "test-key"

	tmp := t.TempDir()
	playlistTimer := filepath.Join(tmp, "playlist.upload.timer")
	videoTimer := filepath.Join(tmp, "video.upload.timer")
	configPath := filepath.Join(tmp, "agent.yaml")

	originalPlaylist := PlaylistTimerPath
	originalVideo := VideoTimerPath
	originalConfigPath := ConfigPath
	PlaylistTimerPath = playlistTimer
	VideoTimerPath = videoTimer
	ConfigPath = configPath
	t.Cleanup(func() {
		PlaylistTimerPath = originalPlaylist
		VideoTimerPath = originalVideo
		ConfigPath = originalConfigPath
	})

	// Set up test config
	testConfig := &Config{
		AllowedUnits:       []string{"test.service"},
		ServerKey:          "test-key",
		ListenAddr:         "0.0.0.0:8081",
		MediaPiServiceUser: "pi",
		Playlist: PlaylistConfig{
			Source:      "/test/src/",
			Destination: "/test/dst/",
		},
		Schedule: ScheduleConfig{
			Playlist: []string{"10:00"},
			Video:    []string{"20:00"},
			Rest:     []RestTimePairConfig{},
		},
		Audio: AudioConfig{
			Output: "hdmi",
		},
	}

	configMutex.Lock()
	originalConfig := currentConfig
	currentConfig = testConfig
	configMutex.Unlock()
	t.Cleanup(func() {
		configMutex.Lock()
		currentConfig = originalConfig
		configMutex.Unlock()
	})

	originalCrontabWrite := CrontabWriteFunc
	originalCrontabRead := CrontabReadFunc
	CrontabWriteFunc = func(content string) error {
		return nil // Mock crontab write
	}
	CrontabReadFunc = func() (string, error) {
		return "", nil // Mock crontab read
	}
	t.Cleanup(func() {
		CrontabWriteFunc = originalCrontabWrite
		CrontabReadFunc = originalCrontabRead
	})

	// Make request to update schedule
	restPairs := []RestTimePair{{Start: "23:00", Stop: "07:00"}}
	reqBody := ScheduleUpdateRequest{
		Playlist: []string{"12:15", "18:30"},
		Video:    []string{"22:45"},
		Rest:     &restPairs,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/menu/schedule/update", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleScheduleUpdate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify timer files were updated
	playlistData, err := os.ReadFile(playlistTimer)
	if err != nil {
		t.Fatalf("failed to read playlist timer: %v", err)
	}
	if !strings.Contains(string(playlistData), "12:15:00") || !strings.Contains(string(playlistData), "18:30:00") {
		t.Errorf("playlist timer not updated correctly: %s", string(playlistData))
	}

	videoData, err := os.ReadFile(videoTimer)
	if err != nil {
		t.Fatalf("failed to read video timer: %v", err)
	}
	if !strings.Contains(string(videoData), "22:45:00") {
		t.Errorf("video timer not updated correctly: %s", string(videoData))
	}

	// Verify agent config was updated
	cfg := GetCurrentConfig()
	if len(cfg.Schedule.Playlist) != 2 || cfg.Schedule.Playlist[0] != "12:15" || cfg.Schedule.Playlist[1] != "18:30" {
		t.Errorf("config playlist schedule not updated: %v", cfg.Schedule.Playlist)
	}
	if len(cfg.Schedule.Video) != 1 || cfg.Schedule.Video[0] != "22:45" {
		t.Errorf("config video schedule not updated: %v", cfg.Schedule.Video)
	}
	if len(cfg.Schedule.Rest) != 1 || cfg.Schedule.Rest[0].Start != "23:00" || cfg.Schedule.Rest[0].Stop != "07:00" {
		t.Errorf("config rest times not updated: %v", cfg.Schedule.Rest)
	}

	// Verify playlist config was preserved
	if cfg.Playlist.Source != "/test/src/" {
		t.Errorf("playlist config was not preserved: %s", cfg.Playlist.Source)
	}

	// Verify config file was written
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("config file was not written: %v", err)
	}
}

func TestHandleScheduleUpdate_PreservesRestWhenOmitted(t *testing.T) {
	ServerKey = "test-key"

	tmp := t.TempDir()
	playlistTimer := filepath.Join(tmp, "playlist.upload.timer")
	videoTimer := filepath.Join(tmp, "video.upload.timer")
	configPath := filepath.Join(tmp, "agent.yaml")

	originalPlaylist := PlaylistTimerPath
	originalVideo := VideoTimerPath
	originalConfigPath := ConfigPath
	PlaylistTimerPath = playlistTimer
	VideoTimerPath = videoTimer
	ConfigPath = configPath
	t.Cleanup(func() {
		PlaylistTimerPath = originalPlaylist
		VideoTimerPath = originalVideo
		ConfigPath = originalConfigPath
	})

	// Set up test config with existing rest times
	existingRestTimes := []RestTimePairConfig{
		{Start: "20:00", Stop: "08:00"},
		{Start: "12:00", Stop: "13:00"},
	}
	testConfig := &Config{
		AllowedUnits:       []string{"test.service"},
		ServerKey:          "test-key",
		ListenAddr:         "0.0.0.0:8081",
		MediaPiServiceUser: "pi",
		Playlist: PlaylistConfig{
			Source:      "/test/src/",
			Destination: "/test/dst/",
		},
		Schedule: ScheduleConfig{
			Playlist: []string{"10:00"},
			Video:    []string{"20:00"},
			Rest:     existingRestTimes,
		},
		Audio: AudioConfig{
			Output: "hdmi",
		},
	}

	configMutex.Lock()
	originalConfig := currentConfig
	currentConfig = testConfig
	configMutex.Unlock()
	t.Cleanup(func() {
		configMutex.Lock()
		currentConfig = originalConfig
		configMutex.Unlock()
	})

	// Make request WITHOUT Rest field (omitted, not empty array)
	reqBody := ScheduleUpdateRequest{
		Playlist: []string{"11:00", "17:00"},
		Video:    []string{"23:00"},
		Rest:     nil, // Explicitly omitted
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/menu/schedule/update", bytes.NewReader(body))
	req.Header.Set("Authorization", "******")
	w := httptest.NewRecorder()

	HandleScheduleUpdate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify agent config preserved existing rest times
	cfg := GetCurrentConfig()
	if len(cfg.Schedule.Rest) != 2 {
		t.Errorf("expected 2 rest time pairs preserved, got %d: %v", len(cfg.Schedule.Rest), cfg.Schedule.Rest)
	}
	if len(cfg.Schedule.Rest) > 0 && (cfg.Schedule.Rest[0].Start != "20:00" || cfg.Schedule.Rest[0].Stop != "08:00") {
		t.Errorf("first rest time not preserved: got %s-%s", cfg.Schedule.Rest[0].Start, cfg.Schedule.Rest[0].Stop)
	}
	if len(cfg.Schedule.Rest) > 1 && (cfg.Schedule.Rest[1].Start != "12:00" || cfg.Schedule.Rest[1].Stop != "13:00") {
		t.Errorf("second rest time not preserved: got %s-%s", cfg.Schedule.Rest[1].Start, cfg.Schedule.Rest[1].Stop)
	}

	// Verify playlist and video schedules were updated
	if len(cfg.Schedule.Playlist) != 2 || cfg.Schedule.Playlist[0] != "11:00" || cfg.Schedule.Playlist[1] != "17:00" {
		t.Errorf("config playlist schedule not updated: %v", cfg.Schedule.Playlist)
	}
	if len(cfg.Schedule.Video) != 1 || cfg.Schedule.Video[0] != "23:00" {
		t.Errorf("config video schedule not updated: %v", cfg.Schedule.Video)
	}
}

func TestHandleScheduleGet_ReadsFromConfig(t *testing.T) {
	ServerKey = "test-key"

	// Set up test config
	testConfig := &Config{
		AllowedUnits:       []string{"test.service"},
		ServerKey:          "test-key",
		ListenAddr:         "0.0.0.0:8081",
		MediaPiServiceUser: "pi",
		Schedule: ScheduleConfig{
			Playlist: []string{"09:00", "15:00"},
			Video:    []string{"21:30"},
			Rest: []RestTimePairConfig{
				{Start: "20:00", Stop: "08:00"},
			},
		},
	}

	configMutex.Lock()
	originalConfig := currentConfig
	currentConfig = testConfig
	configMutex.Unlock()
	t.Cleanup(func() {
		configMutex.Lock()
		currentConfig = originalConfig
		configMutex.Unlock()
	})

	req := httptest.NewRequest(http.MethodGet, "/api/menu/schedule/get", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	w := httptest.NewRecorder()

	HandleScheduleGet(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp struct {
		OK   bool             `json:"ok"`
		Data ScheduleResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if !resp.OK {
		t.Fatalf("expected OK response")
	}

	// Verify schedule was read from config
	if len(resp.Data.Playlist) != 2 || resp.Data.Playlist[0] != "09:00" || resp.Data.Playlist[1] != "15:00" {
		t.Errorf("unexpected playlist schedule: %v", resp.Data.Playlist)
	}
	if len(resp.Data.Video) != 1 || resp.Data.Video[0] != "21:30" {
		t.Errorf("unexpected video schedule: %v", resp.Data.Video)
	}
	if len(resp.Data.Rest) != 1 || resp.Data.Rest[0].Start != "20:00" || resp.Data.Rest[0].Stop != "08:00" {
		t.Errorf("unexpected rest times: %v", resp.Data.Rest)
	}
}
