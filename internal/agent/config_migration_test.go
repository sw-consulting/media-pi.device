// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrateConfigFromSystemd_AllSettings(t *testing.T) {
	// Set up temporary files for migration
	tmp := t.TempDir()
	servicePath := filepath.Join(tmp, "playlist.upload.service")
	playlistTimer := filepath.Join(tmp, "playlist.upload.timer")
	videoTimer := filepath.Join(tmp, "video.upload.timer")
	audioPath := filepath.Join(tmp, "asound.conf")

	originalServicePath := PlaylistServicePath
	originalPlaylist := PlaylistTimerPath
	originalVideo := VideoTimerPath
	originalAudio := AudioConfigPath
	PlaylistServicePath = servicePath
	PlaylistTimerPath = playlistTimer
	VideoTimerPath = videoTimer
	AudioConfigPath = audioPath
	t.Cleanup(func() {
		PlaylistServicePath = originalServicePath
		PlaylistTimerPath = originalPlaylist
		VideoTimerPath = originalVideo
		AudioConfigPath = originalAudio
	})

	// Create test files
	serviceContent := `[Unit]
Description = Rsync playlist upload service
[Service]
ExecStart = /usr/bin/rsync -czavP /mnt/test/src/ /mnt/test/dst/
[Install]
WantedBy = multi-user.target
`
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		t.Fatalf("failed to write service file: %v", err)
	}

	playlistContent := `[Unit]
Description = Playlist upload timer

[Timer]
OnCalendar=*-*-* 10:30:00
OnCalendar=*-*-* 14:45:00

[Install]
WantedBy=timers.target
`
	if err := os.WriteFile(playlistTimer, []byte(playlistContent), 0644); err != nil {
		t.Fatalf("failed to write playlist timer: %v", err)
	}

	videoContent := `[Unit]
Description = Video upload timer

[Timer]
OnCalendar=*-*-* 20:15:00

[Install]
WantedBy=timers.target
`
	if err := os.WriteFile(videoTimer, []byte(videoContent), 0644); err != nil {
		t.Fatalf("failed to write video timer: %v", err)
	}

	if err := os.WriteFile(audioPath, []byte("defaults.pcm.card 1\n"), 0644); err != nil {
		t.Fatalf("failed to write audio config: %v", err)
	}

	originalCrontabRead := CrontabReadFunc
	CrontabReadFunc = func() (string, error) {
		return strings.Join([]string{
			"# MEDIA_PI_REST STOP",
			"15 22 * * * sudo systemctl stop play.video.service",
			"# MEDIA_PI_REST START",
			"30 08 * * * sudo systemctl start play.video.service",
		}, "\n") + "\n", nil
	}
	t.Cleanup(func() { CrontabReadFunc = originalCrontabRead })

	// Create empty config and migrate
	cfg := Config{}
	needsSave := false
	err := migrateConfigFromSystemd(&cfg, &needsSave)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	if !needsSave {
		t.Fatalf("expected needsSave to be true")
	}

	// Verify migrated settings
	if cfg.Playlist.Source != "/mnt/test/src/" {
		t.Errorf("expected source /mnt/test/src/, got %s", cfg.Playlist.Source)
	}
	if cfg.Playlist.Destination != "/mnt/test/dst/" {
		t.Errorf("expected destination /mnt/test/dst/, got %s", cfg.Playlist.Destination)
	}

	expectedPlaylist := []string{"10:30", "14:45"}
	if len(cfg.Schedule.Playlist) != len(expectedPlaylist) {
		t.Errorf("expected %d playlist times, got %d", len(expectedPlaylist), len(cfg.Schedule.Playlist))
	}
	for i, expected := range expectedPlaylist {
		if i >= len(cfg.Schedule.Playlist) || cfg.Schedule.Playlist[i] != expected {
			t.Errorf("playlist time %d: expected %s, got %v", i, expected, cfg.Schedule.Playlist)
		}
	}

	expectedVideo := []string{"20:15"}
	if len(cfg.Schedule.Video) != len(expectedVideo) {
		t.Errorf("expected %d video times, got %d", len(expectedVideo), len(cfg.Schedule.Video))
	}
	if cfg.Schedule.Video[0] != expectedVideo[0] {
		t.Errorf("video time: expected %s, got %s", expectedVideo[0], cfg.Schedule.Video[0])
	}

	if len(cfg.Schedule.Rest) != 1 {
		t.Fatalf("expected 1 rest time pair, got %d", len(cfg.Schedule.Rest))
	}
	if cfg.Schedule.Rest[0].Start != "22:15" || cfg.Schedule.Rest[0].Stop != "08:30" {
		t.Errorf("rest time: expected 22:15-08:30, got %s-%s", cfg.Schedule.Rest[0].Start, cfg.Schedule.Rest[0].Stop)
	}

	if cfg.Audio.Output != "jack" {
		t.Errorf("expected audio output jack, got %s", cfg.Audio.Output)
	}
}

func TestMigrateConfigFromSystemd_SkipsExistingSettings(t *testing.T) {
	// Set up temporary files
	tmp := t.TempDir()
	servicePath := filepath.Join(tmp, "playlist.upload.service")

	originalServicePath := PlaylistServicePath
	PlaylistServicePath = servicePath
	t.Cleanup(func() {
		PlaylistServicePath = originalServicePath
	})

	serviceContent := `[Unit]
Description = Rsync playlist upload service
[Service]
ExecStart = /usr/bin/rsync -czavP /mnt/old/src/ /mnt/old/dst/
[Install]
WantedBy = multi-user.target
`
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		t.Fatalf("failed to write service file: %v", err)
	}

	// Create config with existing settings
	cfg := Config{
		Playlist: PlaylistConfig{
			Source:      "/mnt/existing/src/",
			Destination: "/mnt/existing/dst/",
		},
		Schedule: ScheduleConfig{
			Playlist: []string{"11:11"},
			Video:    []string{"22:22"},
			Rest:     []RestTimePairConfig{{Start: "23:00", Stop: "07:00"}},
		},
		Audio: AudioConfig{
			Output: "hdmi",
		},
	}

	needsSave := false
	err := migrateConfigFromSystemd(&cfg, &needsSave)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// needsSave should be false because all settings already exist
	if needsSave {
		t.Errorf("expected needsSave to be false when settings exist")
	}

	// Verify settings were not overwritten
	if cfg.Playlist.Source != "/mnt/existing/src/" {
		t.Errorf("playlist source was overwritten: %s", cfg.Playlist.Source)
	}
	if cfg.Playlist.Destination != "/mnt/existing/dst/" {
		t.Errorf("playlist destination was overwritten: %s", cfg.Playlist.Destination)
	}
	if len(cfg.Schedule.Playlist) != 1 || cfg.Schedule.Playlist[0] != "11:11" {
		t.Errorf("playlist schedule was overwritten: %v", cfg.Schedule.Playlist)
	}
}

func TestMigrateConfigFromSystemd_HandlesErrors(t *testing.T) {
	// Set up with non-existent files
	tmp := t.TempDir()
	servicePath := filepath.Join(tmp, "nonexistent.service")

	originalServicePath := PlaylistServicePath
	PlaylistServicePath = servicePath
	t.Cleanup(func() {
		PlaylistServicePath = originalServicePath
	})

	cfg := Config{}
	needsSave := false
	err := migrateConfigFromSystemd(&cfg, &needsSave)
	
	// Should return error but not crash
	if err == nil {
		t.Errorf("expected error when files don't exist")
	}

	// Settings should remain empty
	if cfg.Playlist.Source != "" {
		t.Errorf("expected empty source, got %s", cfg.Playlist.Source)
	}
}

func TestGetCurrentConfig(t *testing.T) {
	// Set up test config
	testConfig := &Config{
		AllowedUnits:       []string{"test.service"},
		ServerKey:          "test-key",
		Playlist:           PlaylistConfig{Source: "/src", Destination: "/dst"},
		Schedule:           ScheduleConfig{Playlist: []string{"10:00"}},
		Audio:              AudioConfig{Output: "hdmi"},
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

	// Get config and verify it's a copy
	cfg := GetCurrentConfig()
	
	if cfg.ServerKey != "test-key" {
		t.Errorf("expected server key test-key, got %s", cfg.ServerKey)
	}
	if cfg.Playlist.Source != "/src" {
		t.Errorf("expected source /src, got %s", cfg.Playlist.Source)
	}

	// Modify the returned config
	cfg.Playlist.Source = "/modified"

	// Original should be unchanged
	if currentConfig.Playlist.Source == "/modified" {
		t.Errorf("GetCurrentConfig did not return a copy - original was modified")
	}
}

func TestUpdateConfigSettings(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "test-config.yaml")

	// Create initial config
	initialConfig := Config{
		AllowedUnits:       []string{"test.service"},
		ServerKey:          "test-key",
		ListenAddr:         "0.0.0.0:8081",
		MediaPiServiceUser: "pi",
	}

	configMutex.Lock()
	originalConfig := currentConfig
	originalPath := ConfigPath
	currentConfig = &initialConfig
	ConfigPath = configPath
	configMutex.Unlock()

	t.Cleanup(func() {
		configMutex.Lock()
		currentConfig = originalConfig
		ConfigPath = originalPath
		configMutex.Unlock()
	})

	// Update settings
	err := UpdateConfigSettings(
		PlaylistConfig{Source: "/new/src", Destination: "/new/dst"},
		ScheduleConfig{Playlist: []string{"12:00"}, Video: []string{"18:00"}},
		AudioConfig{Output: "analog"},
	)
	if err != nil {
		t.Fatalf("UpdateConfigSettings failed: %v", err)
	}

	// Verify in-memory config was updated
	cfg := GetCurrentConfig()
	if cfg.Playlist.Source != "/new/src" {
		t.Errorf("expected source /new/src, got %s", cfg.Playlist.Source)
	}
	if cfg.Audio.Output != "analog" {
		t.Errorf("expected audio analog, got %s", cfg.Audio.Output)
	}

	// Verify file was created and can be loaded
	loadedCfg, err := LoadConfigFrom(configPath)
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}
	if loadedCfg.Playlist.Destination != "/new/dst" {
		t.Errorf("saved config has wrong destination: %s", loadedCfg.Playlist.Destination)
	}
	if len(loadedCfg.Schedule.Video) != 1 || loadedCfg.Schedule.Video[0] != "18:00" {
		t.Errorf("saved config has wrong video schedule: %v", loadedCfg.Schedule.Video)
	}
}

func TestUpdateConfigSettings_NoCurrentConfig(t *testing.T) {
	configMutex.Lock()
	originalConfig := currentConfig
	currentConfig = nil
	configMutex.Unlock()

	t.Cleanup(func() {
		configMutex.Lock()
		currentConfig = originalConfig
		configMutex.Unlock()
	})

	err := UpdateConfigSettings(
		PlaylistConfig{},
		ScheduleConfig{},
		AudioConfig{},
	)

	if err == nil {
		t.Errorf("expected error when currentConfig is nil")
	}
}
