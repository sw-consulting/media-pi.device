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
