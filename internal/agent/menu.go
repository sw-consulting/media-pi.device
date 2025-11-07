// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
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

	conn, err := dbus.NewWithContext(context.Background())
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

	conn, err := dbus.NewWithContext(context.Background())
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

	conn, err := dbus.NewWithContext(context.Background())
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

	config := " defaults.pcm.card 0 \ndefaults.ctl.card 0"
	cmd := exec.Command("sudo", "bash", "-c", fmt.Sprintf("echo -e '%s' > /etc/asound.conf", config))
	output, err := cmd.CombinedOutput()
	if err != nil {
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

	if len(output) > 0 {
		fmt.Printf("audio-hdmi output: %s\n", string(output))
	}
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

	config := " defaults.pcm.card 1 \ndefaults.ctl.card 1"
	cmd := exec.Command("sudo", "bash", "-c", fmt.Sprintf("echo -e '%s' > /etc/asound.conf", config))
	output, err := cmd.CombinedOutput()
	if err != nil {
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

	if len(output) > 0 {
		fmt.Printf("audio-jack output: %s\n", string(output))
	}
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

	conn, err := dbus.NewWithContext(context.Background())
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
