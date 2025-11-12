// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Configurable paths for timer files. Tests may override these to point to
// temporary locations to avoid requiring root file system access.
var (
	PlaylistTimerPath = "/etc/systemd/system/playlist.upload.timer"
	VideoTimerPath    = "/etc/systemd/system/video.upload.timer"
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
			Description: "Stop video playback service",
			Method:      "POST",
			Path:        "/api/menu/playback/stop",
		},
		{
			ID:          "playback-start",
			Name:        "Запустить воспроизведение",
			Description: "Start video playback service",
			Method:      "POST",
			Path:        "/api/menu/playback/start",
		},
		{
			ID:          "storage-check",
			Name:        "Проверка яндекс диска",
			Description: "Check Yandex disk mount status",
			Method:      "GET",
			Path:        "/api/menu/storage/check",
		},
		{
			ID:          "playlist-upload",
			Name:        "Загрузка плейлиста",
			Description: "Upload playlist from remote source",
			Method:      "POST",
			Path:        "/api/menu/playlist/upload",
		},
		{
			ID:          "rest-time",
			Name:        "Задать время отдыха",
			Description: "Set rest time interval with crontab",
			Method:      "PUT",
			Path:        "/api/menu/schedule/rest-time",
		},
		{
			ID:          "playlist-select",
			Name:        "Выбор плейлиста",
			Description: "Update playlist upload service configuration",
			Method:      "PUT",
			Path:        "/api/menu/playlist/select",
		},
		{
			ID:          "schedule-get",
			Name:        "Получить расписание обновлений",
			Description: "Get playlist and video update timers",
			Method:      "GET",
			Path:        "/api/menu/schedule/get",
		},
		{
			ID:          "schedule-update",
			Name:        "Обновить расписание",
			Description: "Set playlist and video update timers",
			Method:      "PUT",
			Path:        "/api/menu/schedule/update",
		},
		{
			ID:          "audio-hdmi",
			Name:        "HDMI Audio",
			Description: "Configure HDMI audio output",
			Method:      "POST",
			Path:        "/api/menu/audio/hdmi",
		},
		{
			ID:          "audio-jack",
			Name:        "3.5 Jack Audio",
			Description: "Configure 3.5mm jack audio output",
			Method:      "POST",
			Path:        "/api/menu/audio/jack",
		},
		{
			ID:          "system-reload",
			Name:        "Применить изменения",
			Description: "Reload systemd daemon configuration",
			Method:      "POST",
			Path:        "/api/menu/system/reload",
		},
		{
			ID:          "system-reboot",
			Name:        "Перезагрузка",
			Description: "Reboot the system",
			Method:      "POST",
			Path:        "/api/menu/system/reboot",
		},
		{
			ID:          "system-shutdown",
			Name:        "Выключение",
			Description: "Shutdown the system",
			Method:      "POST",
			Path:        "/api/menu/system/shutdown",
		},
	}
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

	result := <-ch

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

	result := <-ch

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "playback-start",
			Result:  result,
			Message: "Воспроизведение запущено",
		},
	})
}

// HandleStorageCheck checks the Yandex disk mount status.
func HandleStorageCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Метод не разрешён",
		})
		return
	}

	cmd := exec.Command("ls", "-l", "/mnt/ya.disk")
	output, err := cmd.CombinedOutput()

	result := "success"
	message := strings.TrimSpace(string(output))

	if err != nil {
		result = "error"
		message = fmt.Sprintf("Ошибка проверки диска: %v", err)
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "storage-check",
			Result:  result,
			Message: message,
		},
	})
}

// HandlePlaylistUpload triggers playlist upload service.
func HandlePlaylistUpload(w http.ResponseWriter, r *http.Request) {
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
	_, err = conn.StartUnitContext(ctx, "playlist.upload.service", "replace", ch)
	if err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось загрузить плейлист: %v", err),
		})
		return
	}

	result := <-ch

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "playlist-upload",
			Result:  result,
			Message: "Загрузка плейлиста",
		},
	})
}

// HandleAudioHDMI configures HDMI audio output.
func HandleAudioHDMI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Метод не разрешён",
		})
		return
	}

	config := "defaults.pcm.card 0\ndefaults.ctl.card 0\n"
	
	if err := os.WriteFile("/etc/asound.conf", []byte(config), 0644); err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось настроить HDMI аудио: %v", err),
		})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "audio-hdmi",
			Result:  "success",
			Message: "HDMI",
		},
	})
}

// HandleAudioJack configures 3.5mm jack audio output.
func HandleAudioJack(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Метод не разрешён",
		})
		return
	}

	config := "defaults.pcm.card 1\ndefaults.ctl.card 1\n"
	
	if err := os.WriteFile("/etc/asound.conf", []byte(config), 0644); err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось настроить 3.5 Jack аудио: %v", err),
		})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "audio-jack",
			Result:  "success",
			Message: "3.5 Jack",
		},
	})
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

	// Execute reboot in a goroutine to allow response to be sent
	go func() {
		cmd := exec.Command("sudo", "reboot", "now")
		_ = cmd.Run()
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

	// Execute shutdown in a goroutine to allow response to be sent
	go func() {
		cmd := exec.Command("sudo", "shutdown", "now")
		_ = cmd.Run()
	}()
}

// RestTimeRequest represents the request to set rest time.
type RestTimeRequest struct {
	StopTime  string `json:"stop_time"`  // Format: "HH:MM"
	StartTime string `json:"start_time"` // Format: "HH:MM"
}

// FileContentRequest represents a request to update a file.
type FileContentRequest struct {
	Content string `json:"content"`
}

// ScheduleResponse represents the current update schedule.
type ScheduleResponse struct {
	Playlist []string `json:"playlist"`
	Video    []string `json:"video"`
}

// ScheduleUpdateRequest represents the request to update timers for playlist and video uploads.
type ScheduleUpdateRequest struct {
	Playlist []string `json:"playlist"`
	Video    []string `json:"video"`
}

// HandleSetRestTime sets the rest time interval using crontab.
func HandleSetRestTime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Метод не разрешён",
		})
		return
	}

	var req RestTimeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONResponse(w, http.StatusBadRequest, APIResponse{
			OK:     false,
			ErrMsg: "Неверный JSON в теле запроса",
		})
		return
	}

	// Validate time format (HH:MM)
	if !isValidTimeFormat(req.StopTime) || !isValidTimeFormat(req.StartTime) {
		JSONResponse(w, http.StatusBadRequest, APIResponse{
			OK:     false,
			ErrMsg: "Неверный формат времени. Используйте HH:MM",
		})
		return
	}

	// Parse times
	stopParts := strings.Split(req.StopTime, ":")
	startParts := strings.Split(req.StartTime, ":")

	// Create crontab entries
	crontabContent := fmt.Sprintf("%s %s * * * sudo systemctl stop play.video.service\n%s %s * * * sudo systemctl start play.video.service\n",
		stopParts[1], stopParts[0], startParts[1], startParts[0])

	// Write to temporary file
	tmpFile, err := os.CreateTemp("", "crontab-*")
	if err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось создать временный файл: %v", err),
		})
		return
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			// Log the error but do not fail the request because cleanup
			// failure is non-fatal.
			fmt.Printf("warning: failed to remove temp file %s: %v\n", tmpFile.Name(), err)
		}
	}()

	if _, err := tmpFile.WriteString(crontabContent); err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось записать crontab: %v", err),
		})
		return
	}
	if err := tmpFile.Close(); err != nil {
		fmt.Printf("warning: failed to close temp file %s: %v\n", tmpFile.Name(), err)
	}

	// Install crontab
	cmd := exec.Command("crontab", tmpFile.Name())
	if output, err := cmd.CombinedOutput(); err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось установить crontab: %v, %s", err, string(output)),
		})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "rest-time",
			Result:  "success",
			Message: "Время отдыха задано",
		},
	})
}

// HandlePlaylistSelect updates the playlist upload service configuration.
func HandlePlaylistSelect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Метод не разрешён",
		})
		return
	}

	var req FileContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONResponse(w, http.StatusBadRequest, APIResponse{
			OK:     false,
			ErrMsg: "Неверный JSON в теле запроса",
		})
		return
	}

	filePath := "/etc/systemd/system/playlist.upload.service"
	if err := os.WriteFile(filePath, []byte(req.Content), 0644); err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось записать файл: %v", err),
		})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "playlist-select",
			Result:  "success",
			Message: "Выбор плейлиста",
		},
	})
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

	playlistTimers, err := readTimerSchedule(PlaylistTimerPath)
	if err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось прочитать файл таймера плейлиста: %v", err),
		})
		return
	}

	videoTimers, err := readTimerSchedule(VideoTimerPath)
	if err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось прочитать файл таймера видео: %v", err),
		})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: ScheduleResponse{
			Playlist: playlistTimers,
			Video:    videoTimers,
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

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "schedule-update",
			Result:  "success",
			Message: "Расписание обновлений обновлено",
		},
	})
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

	if strings.HasPrefix(value, "--*") {
		value = strings.TrimSpace(strings.TrimPrefix(value, "--*"))
	}

	if value == "" {
		return "", false
	}

	fields := strings.Fields(value)
	if len(fields) > 0 {
		value = fields[0]
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

func writeTimerSchedule(filePath, description, unit string, times []string) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}

	var builder strings.Builder
	builder.WriteString("[Unit]\n")
	builder.WriteString(fmt.Sprintf("Description = %s\n\n", description))
	builder.WriteString("[Timer]\n")
	for _, t := range times {
		builder.WriteString(fmt.Sprintf("OnCalendar=--* %s:00\n", t))
	}
	builder.WriteString(fmt.Sprintf("Unit=%s\n", unit))
	builder.WriteString("Persistent=true\n")
	builder.WriteString("User=pi\n\n")
	builder.WriteString("[Install]\n")
	builder.WriteString("WantedBy=timers.target\n")

	return os.WriteFile(filePath, []byte(builder.String()), 0644)
}

// isValidTimeFormat checks if a string is in HH:MM format.
func isValidTimeFormat(timeStr string) bool {
	_, _, err := parseHourMinute(timeStr)
	return err == nil
}
