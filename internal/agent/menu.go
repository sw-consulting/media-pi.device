// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// Configurable paths for timer and service files. Tests may override these to point to
// temporary locations to avoid requiring root file system access.
var (
	PlaylistTimerPath   = "/etc/systemd/system/playlist.upload.timer"
	VideoTimerPath      = "/etc/systemd/system/video.upload.timer"
	PlaylistServicePath = "/etc/systemd/system/playlist.upload.service"
)

// AudioConfigPath is the path to the ALSA config file controlling audio output.
// Tests may override this to point to a temp file.
var AudioConfigPath = "/etc/asound.conf"

// RebootAction and PowerOffAction are package-level hooks used to perform
// system reboot and power-off. Tests can replace these with stubs to avoid
// interacting with the real system.
var (
	RebootAction   = realReboot
	PowerOffAction = realPowerOff
)

// realReboot performs a reboot via systemd's D-Bus API (org.freedesktop.login1.Manager.Reboot).
func realReboot() error {
	// Fallback to invoking systemctl reboot. Tests should override RebootAction
	// to avoid actually rebooting the test host.
	cmd := exec.Command("systemctl", "reboot")
	return cmd.Run()
}

// realPowerOff performs a power-off via systemd's D-Bus API (org.freedesktop.login1.Manager.PowerOff).
func realPowerOff() error {
	// Fallback to invoking systemctl poweroff. Tests should override PowerOffAction
	// to avoid actually powering off the test host.
	cmd := exec.Command("systemctl", "poweroff")
	return cmd.Run()
}

const (
	restStopMarker   = "# MEDIA_PI_REST STOP"
	restStartMarker  = "# MEDIA_PI_REST START"
	restStopCommand  = "sudo systemctl stop play.video.service"
	restStartCommand = "sudo systemctl start play.video.service"
)

var (
	CrontabReadFunc  = defaultCrontabRead
	CrontabWriteFunc = defaultCrontabWrite
	cronParser       = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
)

// MenuActionResponse is returned after performing a menu action.
type MenuActionResponse struct {
	Action  string `json:"action"`
	Result  string `json:"result"`
	Message string `json:"message,omitempty"`
}

// MenuListResponse contains available menu actions.
type MenuListResponse struct {
	Actions []MenuAction `json:"actions"`
}

// MenuAction represents a single menu action.
type MenuAction struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Method      string `json:"method"`
	Path        string `json:"path"`
}

// GetMenuActions returns the list of all available menu actions.
func GetMenuActions() []MenuAction {
	return []MenuAction{
		{
			ID:          "playback-stop",
			Name:        "Остановить воспроизведение",
			Description: "Остановить сервис воспроизведения видео",
			Method:      "POST",
			Path:        "/api/menu/playback/stop",
		},
		{
			ID:          "playback-start",
			Name:        "Запустить воспроизведение",
			Description: "Запустить сервис воспроизведения видео",
			Method:      "POST",
			Path:        "/api/menu/playback/start",
		},
		{
			ID:          "service-status",
			Name:        "Статус сервисов",
			Description: "Получить статус сервисов",
			Method:      "GET",
			Path:        "/api/menu/service/status",
		},
		{
			ID:          "configuration-get",
			Name:        "Получить конфигурацию",
			Description: "Получить конфигурацию плейлиста, расписания и аудио",
			Method:      "GET",
			Path:        "/api/menu/configuration/get",
		},
		{
			ID:          "configuration-update",
			Name:        "Обновить конфигурацию",
			Description: "Обновить конфигурацию плейлиста, расписания и аудио",
			Method:      "PUT",
			Path:        "/api/menu/configuration/update",
		},
		{
			ID:          "playlist-start-upload",
			Name:        "Начать загрузку плейлиста",
			Description: "Запустить сервис загрузки плейлистов",
			Method:      "POST",
			Path:        "/api/menu/playlist/start-upload",
		},
		{
			ID:          "playlist-stop-upload",
			Name:        "Остановить загрузку плейлиста",
			Description: "Остановить сервис загрузки плейлистов",
			Method:      "POST",
			Path:        "/api/menu/playlist/stop-upload",
		},
		{
			ID:          "video-start-upload",
			Name:        "Начать загрузку видео",
			Description: "Запустить сервис загрузки видео",
			Method:      "POST",
			Path:        "/api/menu/video/start-upload",
		},
		{
			ID:          "video-stop-upload",
			Name:        "Остановить загрузку видео",
			Description: "Остановить сервис загрузки видео",
			Method:      "POST",
			Path:        "/api/menu/video/stop-upload",
		},
		{
			ID:          "system-reload",
			Name:        "Применить изменения",
			Description: "Перезагрузить конфигурацию systemd",
			Method:      "POST",
			Path:        "/api/menu/system/reload",
		},
		{
			ID:          "system-reboot",
			Name:        "Перезагрузка",
			Description: "Перезагрузить систему",
			Method:      "POST",
			Path:        "/api/menu/system/reboot",
		},
		{
			ID:          "system-shutdown",
			Name:        "Выключение",
			Description: "Остановить систему",
			Method:      "POST",
			Path:        "/api/menu/system/shutdown",
		},
	}
}

// PlaylistUploadConfig represents the relevant configuration extracted from
// playlist.upload.service.
type PlaylistUploadConfig struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

// ScheduleSettings represents playlist/video timer settings and optional rest periods.
type ScheduleSettings struct {
	Playlist []string       `json:"playlist"`
	Video    []string       `json:"video"`
	Rest     []RestTimePair `json:"rest,omitempty"`
}

// AudioSettings describes the selected audio output.
type AudioSettings struct {
	Output string `json:"output"`
}

// ConfigurationSettings aggregates playlist upload configuration, schedule and audio output.
type ConfigurationSettings struct {
	Playlist PlaylistUploadConfig `json:"playlist"`
	Schedule ScheduleSettings     `json:"schedule"`
	Audio    AudioSettings        `json:"audio"`
}

// ServiceStatusResponse describes the service status returned by the
// service-status endpoint.
type ServiceStatusResponse struct {
	PlaybackServiceStatus       bool `json:"playbackServiceStatus"`
	PlaylistUploadServiceStatus bool `json:"playlistUploadServiceStatus"`
	VideoUploadServiceStatus    bool `json:"videoUploadServiceStatus"`
	YaDiskMountStatus           bool `json:"yaDiskMountStatus"`
}

// HandleMenuList returns the list of available menu actions.
func HandleMenuList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Метод не разрешён",
		})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuListResponse{
			Actions: GetMenuActions(),
		},
	})
}

// HandlePlaybackStop stops the video playback service.
func HandlePlaybackStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Метод не разрешён",
		})
		return
	}

	conn, err := getDBusConnection(context.Background())
	if err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось подключиться к D-Bus: %v", err),
		})
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch := make(chan string, 1)
	_, err = conn.StopUnitContext(ctx, "play.video.service", "replace", ch)
	if err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось остановить воспроизведение: %v", err),
		})
		return
	}

	var result string
	select {
	case result = <-ch:
		// proceed as normal
	case <-ctx.Done():
		JSONResponse(w, http.StatusRequestTimeout, APIResponse{
			OK:     false,
			ErrMsg: "Таймаут остановки воспроизведения",
		})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "playback-stop",
			Result:  result,
			Message: "Воспроизведение остановлено",
		},
	})
}

// HandlePlaybackStart starts the video playback service.
func HandlePlaybackStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Метод не разрешён",
		})
		return
	}

	conn, err := getDBusConnection(context.Background())
	if err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось подключиться к D-Bus: %v", err),
		})
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch := make(chan string, 1)
	_, err = conn.StartUnitContext(ctx, "play.video.service", "replace", ch)
	if err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось запустить воспроизведение: %v", err),
		})
		return
	}

	var result string
	select {
	case result = <-ch:
		// Successfully received result from D-Bus
	case <-ctx.Done():
		JSONResponse(w, http.StatusRequestTimeout, APIResponse{
			OK:     false,
			ErrMsg: "Таймаут запуска воспроизведения",
		})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "playback-start",
			Result:  result,
			Message: "Воспроизведение запущено",
		},
	})
}

// isPathMounted checks whether the given path appears in /proc/mounts.
// It is small and testable; tests may override behavior by creating a
// temporary /proc/mounts-like file and setting os.Open to read from it
// indirectly via injection if necessary. For simplicity we read the real
// /proc/mounts which is fine for unit tests that don't rely on actual mounts.
func isPathMounted(path string) bool {
	// If a test-provided mounts file is set, prefer parsing it. This keeps
	// unit tests hermetic.
	if mountsPath := os.Getenv("MEDIA_PI_AGENT_PROC_MOUNTS"); mountsPath != "" {
		if f, err := os.Open(mountsPath); err == nil {
			defer func() { _ = f.Close() }()
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				fields := strings.Fields(scanner.Text())
				if len(fields) >= 2 && fields[1] == path {
					return true
				}
			}
		}
		return false
	}

	// Try POSIX device-id method: compare device IDs of the path and its
	// parent. If they differ, the path is a mount point.
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	parent := filepath.Clean(filepath.Join(path, ".."))
	pfi, err := os.Lstat(parent)
	if err != nil {
		return false
	}

	if ok, same := sameDevice(fi, pfi); ok {
		return !same
	}

	// Fallback: parse /proc/mounts if device-id check isn't available.
	if f, err := os.Open("/proc/mounts"); err == nil {
		defer func() { _ = f.Close() }()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) >= 2 && fields[1] == path {
				return true
			}
		}
	}
	return false
}

// HandleServiceStatus returns statuses for playback, playlist upload services
// and whether the Yandex disk mount point is mounted.
func HandleServiceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{OK: false, ErrMsg: "Метод не разрешён"})
		return
	}

	conn, err := getDBusConnection(context.Background())
	if err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{OK: false, ErrMsg: fmt.Sprintf("Не удалось подключиться к D-Bus: %v", err)})
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Helper to query ActiveState from unit properties.
	checkUnit := func(unit string) bool {
		props, err := conn.GetUnitPropertiesContext(ctx, unit)
		if err != nil {
			return false
		}
		if state, ok := props["ActiveState"].(string); ok {
			return state == "active"
		}
		return false
	}

	resp := ServiceStatusResponse{
		PlaybackServiceStatus:       checkUnit("play.video.service"),
		PlaylistUploadServiceStatus: checkUnit("playlist.upload.service"),
		VideoUploadServiceStatus:    checkUnit("video.upload.service"),
		YaDiskMountStatus:           isPathMounted("/mnt/ya.disk"),
	}

	JSONResponse(w, http.StatusOK, APIResponse{OK: true, Data: resp})
}

// readPlaylistUploadConfig parses the playlist upload service file and returns
// the configured source and destination paths.
func readPlaylistUploadConfig(path string) (PlaylistUploadConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PlaylistUploadConfig{}, err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "ExecStart=") && !strings.HasPrefix(trimmed, "ExecStart ") {
			continue
		}

		eqIdx := strings.Index(line, "=")
		if eqIdx == -1 {
			return PlaylistUploadConfig{}, fmt.Errorf("строка ExecStart не содержит '='")
		}

		commandWithComment := strings.TrimSpace(line[eqIdx+1:])
		if hashIdx := strings.Index(commandWithComment, "#"); hashIdx != -1 {
			commandWithComment = strings.TrimSpace(commandWithComment[:hashIdx])
		}
		fields := strings.Fields(commandWithComment)
		if len(fields) < 2 {
			return PlaylistUploadConfig{}, fmt.Errorf("строка ExecStart не содержит пути источника и назначения")
		}

		return PlaylistUploadConfig{
			Source:      fields[len(fields)-2],
			Destination: fields[len(fields)-1],
		}, nil
	}

	if err := scanner.Err(); err != nil {
		return PlaylistUploadConfig{}, err
	}

	return PlaylistUploadConfig{}, fmt.Errorf("строка ExecStart не найдена")
}

// writePlaylistUploadConfig updates the ExecStart line in the playlist upload
// service file, preserving other parts of the file intact while replacing the
// source and destination paths.
func writePlaylistUploadConfig(path, source, destination string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	updated := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "ExecStart=") && !strings.HasPrefix(trimmed, "ExecStart ") {
			continue
		}

		eqIdx := strings.Index(line, "=")
		if eqIdx == -1 {
			return fmt.Errorf("строка ExecStart не содержит '='")
		}

		commandWithComment := strings.TrimSpace(line[eqIdx+1:])
		comment := ""
		if hashIdx := strings.Index(commandWithComment, "#"); hashIdx != -1 {
			comment = strings.TrimSpace(commandWithComment[hashIdx:])
			commandWithComment = strings.TrimSpace(commandWithComment[:hashIdx])
		}
		fields := strings.Fields(commandWithComment)
		if len(fields) < 2 {
			return fmt.Errorf("строка ExecStart не содержит пути источника и назначения")
		}

		prefixFields := append([]string{}, fields[:len(fields)-2]...)
		prefixFields = append(prefixFields, source, destination)

		newCommand := strings.Join(prefixFields, " ")
		if comment != "" {
			newCommand = fmt.Sprintf("%s %s", newCommand, comment)
		}
		lhs := strings.TrimSpace(line[:eqIdx])
		lines[i] = fmt.Sprintf("%s = %s", lhs, newCommand)
		updated = true
		break
	}

	if !updated {
		return fmt.Errorf("строка ExecStart не найдена")
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

func readAudioSettings() (AudioSettings, error) {
	data, err := os.ReadFile(AudioConfigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AudioSettings{Output: "unknown"}, nil
		}
		return AudioSettings{}, fmt.Errorf("не удалось прочитать конфигурационный файл: %w", err)
	}

	content := string(data)
	switch {
	case strings.Contains(content, "card 0"):
		return AudioSettings{Output: "hdmi"}, nil
	case strings.Contains(content, "card 1"):
		return AudioSettings{Output: "jack"}, nil
	default:
		return AudioSettings{Output: "unknown"}, nil
	}
}

func validateAudioOutput(output string) error {
	reqOutput := strings.ToLower(strings.TrimSpace(output))
	if reqOutput != "hdmi" && reqOutput != "jack" {
		return fmt.Errorf("output должен быть 'hdmi' или 'jack'")
	}
	return nil
}

func writeAudioSettings(output string) error {
	if err := validateAudioOutput(output); err != nil {
		return err
	}

	reqOutput := strings.ToLower(strings.TrimSpace(output))
	var config string
	switch reqOutput {
	case "hdmi":
		config = "defaults.pcm.card 0\ndefaults.ctl.card 0\n"
	case "jack":
		config = "defaults.pcm.card 1\ndefaults.ctl.card 1\n"
	}

	return os.WriteFile(AudioConfigPath, []byte(config), 0644)
}

// HandleConfigurationGet aggregates playlist, schedule and audio configuration into a single response.
func HandleConfigurationGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{OK: false, ErrMsg: "Метод не разрешён"})
		return
	}

	// Read configuration from agent.yaml instead of systemd files
	cfg := GetCurrentConfig()

	// Convert RestTimePairConfig to RestTimePair for the response
	restPairs := make([]RestTimePair, len(cfg.Schedule.Rest))
	for i, r := range cfg.Schedule.Rest {
		restPairs[i] = RestTimePair(r)
	}

	JSONResponse(w, http.StatusOK, APIResponse{OK: true, Data: ConfigurationSettings{
		Playlist: PlaylistUploadConfig{
			Source:      cfg.Playlist.Source,
			Destination: cfg.Playlist.Destination,
		},
		Schedule: ScheduleSettings{
			Playlist: cfg.Schedule.Playlist,
			Video:    cfg.Schedule.Video,
			Rest:     restPairs,
		},
		Audio: AudioSettings{
			Output: cfg.Audio.Output,
		},
	}})
}

// HandleConfigurationUpdate updates playlist upload paths, schedule timers and audio output together.
func HandleConfigurationUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{OK: false, ErrMsg: "Метод не разрешён"})
		return
	}

	var req ConfigurationSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONResponse(w, http.StatusBadRequest, APIResponse{OK: false, ErrMsg: "Неверный JSON в теле запроса"})
		return
	}

	playlistSource := strings.TrimSpace(req.Playlist.Source)
	playlistDestination := strings.TrimSpace(req.Playlist.Destination)
	if playlistSource == "" || playlistDestination == "" {
		JSONResponse(w, http.StatusBadRequest, APIResponse{OK: false, ErrMsg: "Поля source и destination обязательны"})
		return
	}

	if hasInvalidTimes(req.Schedule.Playlist, req.Schedule.Video) {
		JSONResponse(w, http.StatusBadRequest, APIResponse{OK: false, ErrMsg: "Неверный формат времени. Используйте HH:MM"})
		return
	}

	restPairs, err := sanitizeRestPairs(req.Schedule.Rest)
	if err != nil {
		JSONResponse(w, http.StatusBadRequest, APIResponse{OK: false, ErrMsg: err.Error()})
		return
	}

	if err := validateRestTimePairs(restPairs); err != nil {
		JSONResponse(w, http.StatusBadRequest, APIResponse{OK: false, ErrMsg: err.Error()})
		return
	}

	if err := validateAudioOutput(req.Audio.Output); err != nil {
		JSONResponse(w, http.StatusBadRequest, APIResponse{OK: false, ErrMsg: err.Error()})
		return
	}

	normalizedPlaylist, err := normalizeTimes(req.Schedule.Playlist)
	if err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{OK: false, ErrMsg: fmt.Sprintf("Неправильный формат таймера загрузки плейлиста: %v", err)})
		return
	}

	normalizedVideo, err := normalizeTimes(req.Schedule.Video)
	if err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{OK: false, ErrMsg: fmt.Sprintf("Неправильный формат таймера загрузки видео: %v", err)})
		return
	}

	if err := writePlaylistUploadConfig(PlaylistServicePath, playlistSource, playlistDestination); err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{OK: false, ErrMsg: fmt.Sprintf("Не удалось обновить конфигурацию: %v", err)})
		return
	}

	if err := writeTimerSchedule(PlaylistTimerPath, "Playlist upload timer", "playlist.upload.service", normalizedPlaylist); err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{OK: false, ErrMsg: fmt.Sprintf("Не удалось записать файл таймера плейлиста: %v", err)})
		return
	}

	if err := writeTimerSchedule(VideoTimerPath, "Video upload timer", "video.upload.service", normalizedVideo); err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{OK: false, ErrMsg: fmt.Sprintf("Не удалось записать файл таймера видео: %v", err)})
		return
	}

	if err := updateRestTimes(restPairs); err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{OK: false, ErrMsg: fmt.Sprintf("Не удалось обновить crontab: %v", err)})
		return
	}

	if err := writeAudioSettings(req.Audio.Output); err != nil {
		JSONResponse(w, http.StatusBadRequest, APIResponse{OK: false, ErrMsg: err.Error()})
		return
	}

	// Update configuration file with all settings
	// Note: We log but don't fail on config file update errors during migration period.
	// Systemd files remain the operational source of truth, and YAML file is being
	// introduced as the new configuration store. On next agent restart, the YAML will
	// be populated via migration if still missing.
	restConfigPairs := make([]RestTimePairConfig, len(restPairs))
	for i, p := range restPairs {
		restConfigPairs[i] = RestTimePairConfig(p)
	}
	
	if err := UpdateConfigSettings(
		PlaylistConfig{Source: playlistSource, Destination: playlistDestination},
		ScheduleConfig{Playlist: normalizedPlaylist, Video: normalizedVideo, Rest: restConfigPairs},
		AudioConfig{Output: req.Audio.Output},
	); err != nil {
		log.Printf("Warning: Failed to update config file: %v", err)
	}

	JSONResponse(w, http.StatusOK, APIResponse{OK: true, Data: MenuActionResponse{Action: "configuration-update", Result: "success", Message: "Конфигурация обновлена"}})
}

// HandleSystemReload reloads systemd daemon configuration.
func HandleSystemReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Метод не разрешён",
		})
		return
	}

	conn, err := getDBusConnection(context.Background())
	if err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось подключиться к D-Bus: %v", err),
		})
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = conn.ReloadContext(ctx)
	if err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось перезагрузить конфигурацию: %v", err),
		})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "system-reload",
			Result:  "success",
			Message: "Изменения применены",
		},
	})
}

// HandleSystemReboot reboots the system.
func HandleSystemReboot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Метод не разрешён",
		})
		return
	}

	// Note: We manually encode JSON here instead of using JSONResponse because
	// we need to ensure the response is fully sent to the client before the
	// reboot command executes. Using JSONResponse would work, but manually
	// encoding gives us more explicit control over the response lifecycle.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "system-reboot",
			Result:  "success",
			Message: "Перезагрузка...",
		},
	}); err != nil {
		fmt.Printf("Failed to encode JSON response: %v\n", err)
	}

	// Execute reboot in a goroutine to allow response to be sent. Use the
	// RebootAction hook so tests can override it and so we use the D-Bus
	// implementation by default.
	go func() {
		if err := RebootAction(); err != nil {
			fmt.Printf("Reboot action failed: %v\n", err)
		}
	}()
}

// HandleSystemShutdown shuts down the system.
func HandleSystemShutdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Метод не разрешён",
		})
		return
	}

	// Note: We manually encode JSON here instead of using JSONResponse because
	// we need to ensure the response is fully sent to the client before the
	// shutdown command executes. Using JSONResponse would work, but manually
	// encoding gives us more explicit control over the response lifecycle.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "system-shutdown",
			Result:  "success",
			Message: "Выключение...",
		},
	}); err != nil {
		fmt.Printf("Failed to encode JSON response: %v\n", err)
	}

	// Execute shutdown in a goroutine to allow response to be sent. Use the
	// PowerOffAction hook so tests can override it and so we use the D-Bus
	// implementation by default.
	go func() {
		if err := PowerOffAction(); err != nil {
			fmt.Printf("PowerOff action failed: %v\n", err)
		}
	}()
}

// RestTimePair describes a single rest-time interval with start and stop times.
// Start is when the rest period begins (service stops)
// Stop is when the rest period ends (service starts)
type RestTimePair struct {
	Start string `yaml:"start" json:"start"` // When rest begins (service stops)
	Stop  string `yaml:"stop" json:"stop"`   // When rest ends (service starts)
}

// ScheduleResponse represents the current update schedule.
type ScheduleResponse struct {
	Playlist []string       `json:"playlist"`
	Video    []string       `json:"video"`
	Rest     []RestTimePair `json:"rest,omitempty"`
}

// ScheduleUpdateRequest represents the request to update timers for playlist and video uploads.
type ScheduleUpdateRequest struct {
	Playlist []string        `json:"playlist"`
	Video    []string        `json:"video"`
	Rest     *[]RestTimePair `json:"rest"`
}

// HandlePlaylistStartUpload triggers playlist sync (replaces old systemd upload service).
func HandlePlaylistStartUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Требуется метод POST",
		})
		return
	}

	// Trigger both file sync and playlist download with callback to restart video.play service
	err := TriggerSync(func() {
		log.Println("File sync completed, downloading playlist")
		// Download the playlist file
		if err := PerformPlaylistSync(context.Background()); err != nil {
			log.Printf("Warning: Failed to download playlist: %v", err)
		}
		log.Println("Playlist sync completed, restarting video.play service")
		if err := restartVideoPlayService(); err != nil {
			log.Printf("Warning: Failed to restart video.play service: %v", err)
		}
	})

	if err != nil {
		log.Printf("Failed to trigger playlist sync: %v", err)
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось запустить загрузку плейлиста: %v", err),
		})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "playlist-start-upload",
			Result:  "success",
			Message: "Загрузка плейлиста запущена",
		},
	})
}

// HandlePlaylistStopUpload stops ongoing playlist sync (replaces old systemd upload service).
func HandlePlaylistStopUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Требуется метод POST",
		})
		return
	}

	err := StopSync()
	if err != nil {
		log.Printf("Failed to stop playlist sync: %v", err)
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось остановить загрузку плейлиста: %v", err),
		})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "playlist-stop-upload",
			Result:  "success",
			Message: "Загрузка плейлиста остановлена",
		},
	})
}

// HandleVideoStartUpload triggers video sync (replaces old systemd upload service).
func HandleVideoStartUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Требуется метод POST",
		})
		return
	}

	// Trigger video sync without callback (don't restart video.play service)
	err := TriggerSync(nil)
	if err != nil {
		log.Printf("Failed to trigger video sync: %v", err)
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось запустить загрузку видео: %v", err),
		})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "video-start-upload",
			Result:  "success",
			Message: "Загрузка видео запущена",
		},
	})
}

// HandleVideoStopUpload stops ongoing video sync (replaces old systemd upload service).
func HandleVideoStopUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Требуется метод POST",
		})
		return
	}

	err := StopSync()
	if err != nil {
		log.Printf("Failed to stop video sync: %v", err)
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось остановить загрузку видео: %v", err),
		})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "video-stop-upload",
			Result:  "success",
			Message: "Загрузка видео остановлена",
		},
	})
}

// restartVideoPlayService restarts the video.play service via systemd.
func restartVideoPlayService() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := getDBusConnection(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to D-Bus: %w", err)
	}
	defer conn.Close()

	ch := make(chan string, 1)
	_, err = conn.RestartUnitContext(ctx, "video.play.service", "replace", ch)
	if err != nil {
		return fmt.Errorf("failed to restart video.play service: %w", err)
	}

	// Wait for result
	select {
	case result := <-ch:
		if result != "done" {
			return fmt.Errorf("restart failed with result: %s", result)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("restart timeout")
	}
}

// RestartVideoPlayServiceSimple restarts the video.play service using systemctl command.
// This is a simpler version that can be exported and used from main.
func RestartVideoPlayServiceSimple() error {
	cmd := exec.Command("systemctl", "restart", "video.play.service")
	return cmd.Run()
}

// HandleScheduleGet returns the configured update timers for playlist and video uploads.
func HandleScheduleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Метод не разрешён",
		})
		return
	}

	// Read schedule from agent.yaml instead of systemd files
	cfg := GetCurrentConfig()

	// Convert RestTimePairConfig to RestTimePair for the response
	restPairs := make([]RestTimePair, len(cfg.Schedule.Rest))
	for i, r := range cfg.Schedule.Rest {
		restPairs[i] = RestTimePair(r)
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: ScheduleResponse{
			Playlist: cfg.Schedule.Playlist,
			Video:    cfg.Schedule.Video,
			Rest:     restPairs,
		},
	})
}

// HandleScheduleUpdate updates the playlist and video timer configurations.
func HandleScheduleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Метод не разрешён",
		})
		return
	}

	var req ScheduleUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONResponse(w, http.StatusBadRequest, APIResponse{
			OK:     false,
			ErrMsg: "Неверный JSON в теле запроса",
		})
		return
	}

	if hasInvalidTimes(req.Playlist, req.Video) {
		JSONResponse(w, http.StatusBadRequest, APIResponse{
			OK:     false,
			ErrMsg: "Неверный формат времени. Используйте HH:MM",
		})
		return
	}

	var restPairs []RestTimePair
	if req.Rest != nil {
		pairs, err := sanitizeRestPairs(*req.Rest)
		if err != nil {
			JSONResponse(w, http.StatusBadRequest, APIResponse{
				OK:     false,
				ErrMsg: err.Error(),
			})
			return
		}

		if err := validateRestTimePairs(pairs); err != nil {
			JSONResponse(w, http.StatusBadRequest, APIResponse{
				OK:     false,
				ErrMsg: err.Error(),
			})
			return
		}

		restPairs = pairs
	}

	normalizedPlaylist, err := normalizeTimes(req.Playlist)
	if err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось обработать время плейлиста: %v", err),
		})
		return
	}

	normalizedVideo, err := normalizeTimes(req.Video)
	if err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось обработать время видео: %v", err),
		})
		return
	}

	if err := writeTimerSchedule(PlaylistTimerPath, "Playlist upload timer", "playlist.upload.service", normalizedPlaylist); err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось записать файл таймера плейлиста: %v", err),
		})
		return
	}

	if err := writeTimerSchedule(VideoTimerPath, "Video upload timer", "video.upload.service", normalizedVideo); err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось записать файл таймера видео: %v", err),
		})
		return
	}

	if req.Rest != nil {
		if err := updateRestTimes(restPairs); err != nil {
			JSONResponse(w, http.StatusInternalServerError, APIResponse{
				OK:     false,
				ErrMsg: fmt.Sprintf("Не удалось обновить crontab: %v", err),
			})
			return
		}
	}

	// Update configuration file with schedule settings
	// Note: We log but don't fail on config file update errors during migration period.
	// Systemd files remain the operational source of truth, and YAML file is being
	// introduced as the new configuration store. On next agent restart, the YAML will
	// be populated via migration if still missing.
	cfg := GetCurrentConfig()
	
	// Preserve existing rest configuration if not provided in request
	var restConfigPairs []RestTimePairConfig
	if req.Rest != nil {
		restConfigPairs = make([]RestTimePairConfig, len(restPairs))
		for i, p := range restPairs {
			restConfigPairs[i] = RestTimePairConfig(p)
		}
	} else {
		// Keep existing rest configuration when not provided
		restConfigPairs = cfg.Schedule.Rest
	}

	if err := UpdateConfigSettings(
		cfg.Playlist, // Keep existing playlist config
		ScheduleConfig{Playlist: normalizedPlaylist, Video: normalizedVideo, Rest: restConfigPairs},
		cfg.Audio, // Keep existing audio config
	); err != nil {
		log.Printf("Warning: Failed to update config file: %v", err)
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "schedule-update",
			Result:  "success",
			Message: "Расписание обновлено",
		},
	})
}

func sanitizeRestPairs(raw []RestTimePair) ([]RestTimePair, error) {
	pairs := make([]RestTimePair, 0, len(raw))
	for _, pair := range raw {
		start := strings.TrimSpace(pair.Start)
		stop := strings.TrimSpace(pair.Stop)
		if start == "" || stop == "" {
			return nil, errors.New("для каждого интервала нерабочего времени необходимо указать начало и конец")
		}
		pairs = append(pairs, RestTimePair{Start: start, Stop: stop})
	}
	return pairs, nil
}

func validateRestTimePairs(pairs []RestTimePair) error {
	if len(pairs) == 0 {
		return nil
	}

	// Validate time format and convert to minutes since midnight
	type restInterval struct {
		start int // minutes since midnight when rest starts
		stop  int // minutes since midnight when rest stops
		index int // original index for stable sorting
	}

	intervals := make([]restInterval, 0, len(pairs))

	for i, pair := range pairs {
		if !isValidTimeFormat(pair.Start) || !isValidTimeFormat(pair.Stop) {
			return errors.New("неверный формат времени. Используйте HH:MM")
		}

		startHour, startMinute, err := parseTimeValue(pair.Start)
		if err != nil {
			return fmt.Errorf("ошибка в времени начала нерабочего времени: %v", err)
		}
		stopHour, stopMinute, err := parseTimeValue(pair.Stop)
		if err != nil {
			return fmt.Errorf("ошибка в времени окончания нерабочего времени: %v", err)
		}

		startMin := startHour*60 + startMinute
		stopMin := stopHour*60 + stopMinute

		if startMin == stopMin {
			return errors.New("интервал нерабочего времени не может иметь нулевую длительность")
		}

		intervals = append(intervals, restInterval{
			start: startMin,
			stop:  stopMin,
			index: i,
		})
	}

	// Sort intervals by start time, then by original index for stability
	sort.SliceStable(intervals, func(i, j int) bool {
		if intervals[i].start == intervals[j].start {
			return intervals[i].index < intervals[j].index
		}
		return intervals[i].start < intervals[j].start
	})

	// Check for overlaps
	for i := 0; i < len(intervals); i++ {
		current := intervals[i]

		// Normalize intervals that cross midnight
		currentStart := current.start
		currentStop := current.stop
		if currentStop <= currentStart {
			currentStop += 24 * 60 // Add 24 hours
		}

		// Check against all other intervals
		for j := i + 1; j < len(intervals); j++ {
			other := intervals[j]

			// Check multiple scenarios for the other interval
			for dayOffset := 0; dayOffset < 2; dayOffset++ {
				otherStart := other.start + dayOffset*24*60
				otherStop := other.stop + dayOffset*24*60

				if otherStop <= otherStart {
					otherStop += 24 * 60
				}

				// Check if intervals overlap
				if intervalsOverlap(currentStart, currentStop, otherStart, otherStop) {
					return errors.New("интервалы нерабочего времени не должны пересекаться")
				}
			}
		}

		// Also check if current interval overlaps with next day occurrence of earlier intervals
		for j := 0; j < i; j++ {
			other := intervals[j]

			// Check if current interval overlaps with other's next day occurrence
			otherNextDayStart := other.start + 24*60
			otherNextDayStop := other.stop + 24*60
			if otherNextDayStop <= otherNextDayStart {
				otherNextDayStop += 24 * 60
			}

			if intervalsOverlap(currentStart, currentStop, otherNextDayStart, otherNextDayStop) {
				return errors.New("интервалы нерабочего времени не должны пересекаться через границу суток")
			}
		}
	}

	return nil
}

func intervalsOverlap(start1, stop1, start2, stop2 int) bool {
	return start1 < stop2 && start2 < stop1
}

func updateRestTimes(pairs []RestTimePair) error {
	content, err := CrontabReadFunc()
	if err != nil {
		return err
	}

	lines := splitCrontabLines(content)
	lines = filterOutRestEntries(lines)

	restEntries, err := buildRestCronEntries(pairs)
	if err != nil {
		return err
	}

	if len(restEntries) > 0 {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
			lines = append(lines, "")
		}
		lines = append(lines, restEntries...)
	} else {
		lines = trimTrailingEmptyLines(lines)
	}

	return CrontabWriteFunc(joinCrontabLines(lines))
}

func getRestTimes() ([]RestTimePair, error) {
	content, err := CrontabReadFunc()
	if err != nil {
		return nil, err
	}
	return parseRestTimes(content), nil
}

func parseRestTimes(content string) []RestTimePair {
	lines := splitCrontabLines(content)
	pairs := make([]RestTimePair, 0)

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		switch trimmed {
		case restStopMarker:
			if i+1 < len(lines) {
				if timeValue, err := parseCronCommandTime(lines[i+1], restStopCommand); err == nil {
					// Service stop = rest start
					pairs = append(pairs, RestTimePair{Start: timeValue})
					i++
				}
			}
		case restStartMarker:
			if i+1 < len(lines) {
				if timeValue, err := parseCronCommandTime(lines[i+1], restStartCommand); err == nil {
					// Service start = rest stop
					if len(pairs) == 0 || pairs[len(pairs)-1].Stop != "" {
						pairs = append(pairs, RestTimePair{Stop: timeValue})
					} else {
						pairs[len(pairs)-1].Stop = timeValue
					}
					i++
				}
			}
		default:
			if isRestCommandLine(lines[i], restStopCommand) {
				if timeValue, err := parseCronCommandTime(lines[i], restStopCommand); err == nil {
					// Service stop = rest start
					pairs = append(pairs, RestTimePair{Start: timeValue})
				}
			} else if isRestCommandLine(lines[i], restStartCommand) {
				if timeValue, err := parseCronCommandTime(lines[i], restStartCommand); err == nil {
					// Service start = rest stop
					if len(pairs) == 0 || pairs[len(pairs)-1].Stop != "" {
						pairs = append(pairs, RestTimePair{Stop: timeValue})
					} else {
						pairs[len(pairs)-1].Stop = timeValue
					}
				}
			}
		}
	}

	// Filter out incomplete pairs
	cleaned := make([]RestTimePair, 0, len(pairs))
	for _, pair := range pairs {
		if pair.Start != "" && pair.Stop != "" {
			cleaned = append(cleaned, pair)
		}
	}
	return cleaned
}

func splitCrontabLines(content string) []string {
	if content == "" {
		return nil
	}
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func joinCrontabLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func trimTrailingEmptyLines(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func filterOutRestEntries(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	result := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		switch trimmed {
		case restStopMarker:
			if i+1 < len(lines) && isRestCommandLine(lines[i+1], restStopCommand) {
				i++
			}
			continue
		case restStartMarker:
			if i+1 < len(lines) && isRestCommandLine(lines[i+1], restStartCommand) {
				i++
			}
			continue
		}
		if isRestCommandLine(lines[i], restStopCommand) || isRestCommandLine(lines[i], restStartCommand) {
			continue
		}
		result = append(result, lines[i])
	}
	return result
}

func buildRestCronEntries(pairs []RestTimePair) ([]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	entries := make([]string, 0, len(pairs)*5)

	for idx, pair := range pairs {
		// Parse rest start time (when service should stop)
		startHour, startMinute, err := parseTimeValue(pair.Start)
		if err != nil {
			return nil, fmt.Errorf("ошибка в времени начала отдыха: %v", err)
		}

		// Parse rest stop time (when service should start)
		stopHour, stopMinute, err := parseTimeValue(pair.Stop)
		if err != nil {
			return nil, fmt.Errorf("ошибка в времени окончания отдыха: %v", err)
		}

		if idx > 0 {
			entries = append(entries, "")
		}

		// Add service stop entry (rest begins)
		entries = append(entries, restStopMarker)
		entries = append(entries, fmt.Sprintf("%02d %02d * * * %s", startMinute, startHour, restStopCommand))

		// Add service start entry (rest ends)
		entries = append(entries, restStartMarker)
		entries = append(entries, fmt.Sprintf("%02d %02d * * * %s", stopMinute, stopHour, restStartCommand))
	}
	return entries, nil
}

func parseTimeValue(value string) (int, int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("неверный формат времени: %s", value)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("неверный формат часа: %s", value)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("неверный формат минут: %s", value)
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("время вне диапазона: %s", value)
	}
	return hour, minute, nil
}

func parseCronCommandTime(line, expectedCommand string) (string, error) {
	expr, command, err := splitCronLine(line)
	if err != nil {
		return "", err
	}
	if command != expectedCommand {
		return "", fmt.Errorf("неожиданная команда: %s", command)
	}
	if _, err := cronParser.Parse(expr); err != nil {
		return "", err
	}
	fields := strings.Fields(expr)
	if len(fields) < 2 {
		return "", fmt.Errorf("недостаточно полей в cron: %s", expr)
	}
	minute, err := strconv.Atoi(fields[0])
	if err != nil {
		return "", err
	}
	hour, err := strconv.Atoi(fields[1])
	if err != nil {
		return "", err
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return "", fmt.Errorf("время вне диапазона в cron: %s", expr)
	}
	return fmt.Sprintf("%02d:%02d", hour, minute), nil
}

func isRestCommandLine(line, expectedCommand string) bool {
	_, command, err := splitCronLine(strings.TrimSpace(line))
	if err != nil {
		return false
	}
	return command == expectedCommand
}

func splitCronLine(line string) (string, string, error) {
	fields := strings.Fields(line)
	if len(fields) < 6 {
		return "", "", fmt.Errorf("невалидная строка cron: %s", line)
	}
	expr := strings.Join(fields[:5], " ")
	command := strings.Join(fields[5:], " ")
	return expr, command, nil
}

func defaultCrontabRead() (string, error) {
	cmd := exec.Command("crontab", "-u", MediaPiServiceUser, "-l")
	output, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.ToLower(string(output))
		if text == "" {
			text = strings.ToLower(err.Error())
		}
		if strings.Contains(text, "no crontab for") {
			return "", nil
		}
		return "", fmt.Errorf("crontab %s: %w: %s", strings.Join(cmd.Args[1:], " "), err, string(output))
	}
	return string(output), nil
}

func defaultCrontabWrite(content string) error {
	cmd := exec.Command("crontab", "-u", MediaPiServiceUser, "-")
	cmd.Stdin = strings.NewReader(content)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("crontab %s: %w: %s", strings.Join(cmd.Args[1:], " "), err, string(output))
	}
	return nil
}

func hasInvalidTimes(lists ...[]string) bool {
	for _, list := range lists {
		for _, t := range list {
			if !isValidTimeFormat(t) {
				return true
			}
		}
	}
	return false
}

func normalizeTimes(times []string) ([]string, error) {
	normalized := make([]string, 0, len(times))
	for _, t := range times {
		hour, minute, err := parseHourMinute(t)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, fmt.Sprintf("%02d:%02d", hour, minute))
	}
	return normalized, nil
}

func parseHourMinute(timeStr string) (int, int, error) {
	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("неверный формат")
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}

	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}

	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("значение вне диапазона")
	}

	return hour, minute, nil
}

func readTimerSchedule(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()

	var timers []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "OnCalendar=") {
			if timeStr, ok := extractTimeFromOnCalendar(line); ok {
				timers = append(timers, timeStr)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return timers, nil
}

func extractTimeFromOnCalendar(line string) (string, bool) {
	value := strings.TrimSpace(strings.TrimPrefix(line, "OnCalendar="))
	if value == "" {
		return "", false
	}

	for _, prefix := range []string{"--*", "*-*-*"} {
		if after, ok := strings.CutPrefix(value, prefix); ok {
			value = strings.TrimSpace(after)
		}
	}

	if value == "" {
		return "", false
	}

	fields := strings.Fields(value)
	if len(fields) > 0 {
		value = fields[len(fields)-1]
	}

	parts := strings.Split(value, ":")
	if len(parts) < 2 {
		return "", false
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return "", false
	}

	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", false
	}

	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return "", false
	}

	return fmt.Sprintf("%02d:%02d", hour, minute), true
}

// SanitizeSystemdValue sanitizes a string value for use in systemd unit files.
// It removes or replaces characters that could break the unit file format or
// be used for injection attacks, particularly newlines and carriage returns.
func SanitizeSystemdValue(value string) string {
	// Replace newlines, carriage returns, and other control characters with spaces
	// to prevent breaking the unit file format or injection attacks
	result := strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t', '\f', '\v':
			return ' ' // Replace control characters with space
		case '\x00': // Null character
			return -1 // Remove null characters
		default:
			if r < 32 && r != ' ' {
				return ' ' // Replace other control characters with space
			}
			return r
		}
	}, value)

	// Trim leading/trailing whitespace that might have been introduced
	return strings.TrimSpace(result)
}

func writeTimerSchedule(filePath, description, unit string, times []string) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}

	// Sanitize description and unit to prevent injection attacks
	sanitizedDescription := SanitizeSystemdValue(description)
	sanitizedUnit := SanitizeSystemdValue(unit)

	var builder strings.Builder
	builder.WriteString("[Unit]\n")
	fmt.Fprintf(&builder, "Description=%s\n\n", sanitizedDescription)
	builder.WriteString("[Timer]\n")
	for _, t := range times {
		fmt.Fprintf(&builder, "OnCalendar=*-*-* %s:00\n", t)
	}
	fmt.Fprintf(&builder, "Unit=%s\n", sanitizedUnit)
	builder.WriteString("Persistent=true\n\n")
	builder.WriteString("[Install]\n")
	builder.WriteString("WantedBy=timers.target\n")

	return os.WriteFile(filePath, []byte(builder.String()), 0644)
}

// isValidTimeFormat checks if a string is in HH:MM format.
func isValidTimeFormat(timeStr string) bool {
	_, _, err := parseHourMinute(timeStr)
	return err == nil
}

// Migration helper functions - these wrappers allow agent.go to call menu.go functions
// during configuration migration without creating circular dependencies.

func readPlaylistUploadConfigForMigration() (PlaylistUploadConfig, error) {
	return readPlaylistUploadConfig(PlaylistServicePath)
}

func playlistTimerPathForMigration() string {
	return PlaylistTimerPath
}

func videoTimerPathForMigration() string {
	return VideoTimerPath
}

func readTimerScheduleForMigration(filePath string) ([]string, error) {
	return readTimerSchedule(filePath)
}

func getRestTimesForMigration() ([]RestTimePairConfig, error) {
	pairs, err := getRestTimes()
	if err != nil {
		return nil, err
	}
	// Convert from RestTimePair to RestTimePairConfig
	result := make([]RestTimePairConfig, len(pairs))
	for i, p := range pairs {
		result[i] = RestTimePairConfig(p)
	}
	return result, nil
}

func readAudioSettingsForMigration() (AudioSettings, error) {
	return readAudioSettings()
}
