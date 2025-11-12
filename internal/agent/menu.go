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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// Configurable paths for timer files. Tests may override these to point to
// temporary locations to avoid requiring root file system access.
var (
	PlaylistTimerPath = "/etc/systemd/system/playlist.upload.timer"
	VideoTimerPath    = "/etc/systemd/system/video.upload.timer"
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
			ID:          "storage-check",
			Name:        "Проверка яндекс диска",
			Description: "Проверить статус Яндекс.Диска",
			Method:      "GET",
			Path:        "/api/menu/storage/check",
		},
		{
			ID:          "playlist-upload",
			Name:        "Загрузка плейлиста",
			Description: "Загрузить плейлист из удалённого источника",
			Method:      "POST",
			Path:        "/api/menu/playlist/upload",
		},
		{
			ID:          "playlist-select",
			Name:        "Выбор плейлиста",
			Description: "Обновить конфигурацию сервиса загрузки плейлистов",
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
			Description: "Установить расписание обновления плейлистов и видео",
			Method:      "PUT",
			Path:        "/api/menu/schedule/update",
		},
		{
			ID:          "audio-get",
			Name:        "Получить аудио-вывод",
			Description: "Получить текущую настройку вывода звука (hdmi|jack)",
			Method:      "GET",
			Path:        "/api/menu/audio/get",
		},
		{
			ID:          "audio-update",
			Name:        "Обновить аудио-вывод",
			Description: "Установить вывод звука (hdmi|jack)",
			Method:      "PUT",
			Path:        "/api/menu/audio/update",
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

	var result string
	select {
	case result = <-ch:
		// Successfully received result from D-Bus operation
	case <-ctx.Done():
		JSONResponse(w, http.StatusGatewayTimeout, APIResponse{
			OK:     false,
			ErrMsg: "Таймаут при загрузке плейлиста",
		})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "playlist-upload",
			Result:  result,
			Message: "Загрузка плейлиста",
		},
	})
}

// (removed deprecated handlers: audio-hdmi and audio-jack)

// AudioUpdateRequest represents {"output":"hdmi"|"jack"}
type AudioUpdateRequest struct {
	Output string `json:"output"`
}

// HandleAudioGet reads the `AudioConfigPath` file and returns the current
// output: "hdmi", "jack" or "unknown".
func HandleAudioGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{OK: false, ErrMsg: "Метод не разрешён"})
		return
	}

	data, err := os.ReadFile(AudioConfigPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			JSONResponse(w, http.StatusOK, APIResponse{OK: true, Data: "unknown"})
			return
		}
		JSONResponse(w, http.StatusInternalServerError, APIResponse{OK: false, ErrMsg: fmt.Sprintf("Не удалось прочитать конфиг: %v", err)})
		return
	}

	content := string(data)
	if strings.Contains(content, "card 0") {
		JSONResponse(w, http.StatusOK, APIResponse{OK: true, Data: "hdmi"})
		return
	}
	if strings.Contains(content, "card 1") {
		JSONResponse(w, http.StatusOK, APIResponse{OK: true, Data: "jack"})
		return
	}
	JSONResponse(w, http.StatusOK, APIResponse{OK: true, Data: "unknown"})
}

// HandleAudioUpdate sets the audio output by writing `AudioConfigPath`.
func HandleAudioUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{OK: false, ErrMsg: "Метод не разрешён"})
		return
	}

	var req AudioUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONResponse(w, http.StatusBadRequest, APIResponse{OK: false, ErrMsg: "Неверный JSON"})
		return
	}

	var config string
	switch strings.ToLower(strings.TrimSpace(req.Output)) {
	case "hdmi":
		config = "defaults.pcm.card 0\ndefaults.ctl.card 0\n"
	case "jack":
		config = "defaults.pcm.card 1\ndefaults.ctl.card 1\n"
	default:
		JSONResponse(w, http.StatusBadRequest, APIResponse{OK: false, ErrMsg: "output must be 'hdmi' or 'jack'"})
		return
	}

	if err := os.WriteFile(AudioConfigPath, []byte(config), 0644); err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{OK: false, ErrMsg: fmt.Sprintf("Не удалось записать конфиг: %v", err)})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{OK: true, Data: MenuActionResponse{Action: "audio-update", Result: "success", Message: req.Output}})
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
type RestTimePair struct {
	Stop  string `json:"stop"`
	Start string `json:"start"`
}

// FileContentRequest represents a request to update a file.
type FileContentRequest struct {
	Content string `json:"content"`
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

	restTimes, err := getRestTimes()
	if err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: fmt.Sprintf("Не удалось прочитать crontab: %v", err),
		})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: ScheduleResponse{
			Playlist: playlistTimers,
			Video:    videoTimers,
			Rest:     restTimes,
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

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: MenuActionResponse{
			Action:  "schedule-update",
			Result:  "success",
			Message: "Расписание обновлений обновлено",
		},
	})
}

func sanitizeRestPairs(raw []RestTimePair) ([]RestTimePair, error) {
	pairs := make([]RestTimePair, 0, len(raw))
	for _, pair := range raw {
		start := strings.TrimSpace(pair.Start)
		stop := strings.TrimSpace(pair.Stop)
		if start == "" || stop == "" {
			return nil, errors.New("для каждого интервала необходимо указать start и stop")
		}
		pairs = append(pairs, RestTimePair{Start: start, Stop: stop})
	}
	return pairs, nil
}

func validateRestTimePairs(pairs []RestTimePair) error {
	if len(pairs) == 0 {
		return nil
	}

	type parsedPair struct {
		index int
		start int
		stop  int
	}

	parsed := make([]parsedPair, len(pairs))
	occupied := make([]bool, 24*60)

	for i, pair := range pairs {
		if !isValidTimeFormat(pair.Start) || !isValidTimeFormat(pair.Stop) {
			return errors.New("неверный формат времени. Используйте HH:MM")
		}

		startHour, startMinute, err := parseTimeValue(pair.Start)
		if err != nil {
			return err
		}
		stopHour, stopMinute, err := parseTimeValue(pair.Stop)
		if err != nil {
			return err
		}

		startMin := startHour*60 + startMinute
		stopMin := stopHour*60 + stopMinute
		if startMin == stopMin {
			return errors.New("интервал отдыха не может иметь нулевую длительность")
		}

		if err := markRestInterval(occupied, stopMin, startMin); err != nil {
			return err
		}

		parsed[i] = parsedPair{index: i, start: startMin, stop: stopMin}
	}

	sort.SliceStable(parsed, func(i, j int) bool {
		if parsed[i].stop == parsed[j].stop {
			return parsed[i].index < parsed[j].index
		}
		return parsed[i].stop < parsed[j].stop
	})

	const day = 24 * 60
	prevStop := -1
	prevEnd := -1

	for _, pair := range parsed {
		timelineStop := pair.stop
		timelineEnd := pair.start

		if prevStop == -1 {
			if timelineEnd <= timelineStop {
				timelineEnd += day
			}
			prevStop = timelineStop
			prevEnd = timelineEnd
			continue
		}

		for timelineStop <= prevStop {
			timelineStop += day
		}

		if timelineStop <= prevEnd {
			return errors.New("интервалы отдыха не должны пересекаться")
		}

		for timelineEnd <= timelineStop {
			timelineEnd += day
		}

		prevStop = timelineStop
		prevEnd = timelineEnd
	}

	firstStop := parsed[0].stop
	if prevStop != -1 {
		nextCycleStart := firstStop + day
		if prevEnd >= nextCycleStart {
			return errors.New("интервалы отдыха не должны пересекаться через границу суток")
		}
	}

	return nil
}

func markRestInterval(occupied []bool, stopMin, startMin int) error {
	day := len(occupied)
	if day == 0 {
		return nil
	}

	minute := stopMin % day
	startTarget := startMin % day
	steps := 0

	for {
		if occupied[minute] {
			return errors.New("интервалы отдыха не должны пересекаться")
		}
		occupied[minute] = true
		minute = (minute + 1) % day
		steps++
		if minute == startTarget {
			break
		}
		if steps >= day {
			return errors.New("интервал отдыха не может занимать целые сутки")
		}
	}

	return nil
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
					pairs = append(pairs, RestTimePair{Stop: timeValue})
					i++
				}
			}
		case restStartMarker:
			if i+1 < len(lines) {
				if timeValue, err := parseCronCommandTime(lines[i+1], restStartCommand); err == nil {
					if len(pairs) == 0 || pairs[len(pairs)-1].Start != "" {
						pairs = append(pairs, RestTimePair{Start: timeValue})
					} else {
						pairs[len(pairs)-1].Start = timeValue
					}
					i++
				}
			}
		default:
			if isRestCommandLine(lines[i], restStopCommand) {
				if timeValue, err := parseCronCommandTime(lines[i], restStopCommand); err == nil {
					pairs = append(pairs, RestTimePair{Stop: timeValue})
				}
			} else if isRestCommandLine(lines[i], restStartCommand) {
				if timeValue, err := parseCronCommandTime(lines[i], restStartCommand); err == nil {
					if len(pairs) == 0 || pairs[len(pairs)-1].Start != "" {
						pairs = append(pairs, RestTimePair{Start: timeValue})
					} else {
						pairs[len(pairs)-1].Start = timeValue
					}
				}
			}
		}
	}

	cleaned := make([]RestTimePair, 0, len(pairs))
	for _, pair := range pairs {
		if pair.Start == "" || pair.Stop == "" {
			continue
		}
		cleaned = append(cleaned, pair)
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
		startHour, startMinute, err := parseTimeValue(pair.Start)
		if err != nil {
			return nil, err
		}
		stopHour, stopMinute, err := parseTimeValue(pair.Stop)
		if err != nil {
			return nil, err
		}
		if idx > 0 {
			entries = append(entries, "")
		}
		entries = append(entries, restStopMarker)
		entries = append(entries, fmt.Sprintf("%02d %02d * * * %s", stopMinute, stopHour, restStopCommand))
		entries = append(entries, restStartMarker)
		entries = append(entries, fmt.Sprintf("%02d %02d * * * %s", startMinute, startHour, restStartCommand))
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
	cmd := exec.Command("crontab", "-l")
	output, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.ToLower(string(output))
		if text == "" {
			text = strings.ToLower(err.Error())
		}
		if strings.Contains(text, "no crontab for") {
			return "", nil
		}
		return "", fmt.Errorf("crontab -l: %w: %s", err, string(output))
	}
	return string(output), nil
}

func defaultCrontabWrite(content string) error {
	cmd := exec.Command("crontab", "-")
	cmd.Stdin = strings.NewReader(content)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("crontab -: %w: %s", err, string(output))
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
	builder.WriteString(fmt.Sprintf("Description = %s\n\n", sanitizedDescription))
	builder.WriteString("[Timer]\n")
	for _, t := range times {
		builder.WriteString(fmt.Sprintf("OnCalendar=--* %s:00\n", t))
	}
	builder.WriteString(fmt.Sprintf("Unit=%s\n", sanitizedUnit))
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
