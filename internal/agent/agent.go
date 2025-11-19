// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package agent

// Package agent contains the core agent functionality extracted from main
// so tests can live in a separate directory and import a stable API.

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the agent configuration file structure. It is loaded
// from YAML and contains the list of allowed systemd units, the server
// authentication key and the listen address for the HTTP API.
type Config struct {
	AllowedUnits       []string `yaml:"allowed_units"`
	ServerKey          string   `yaml:"server_key,omitempty"`
	ListenAddr         string   `yaml:"listen_addr,omitempty"`
	MediaPiServiceUser string   `yaml:"media_pi_service_user,omitempty"`
}

// APIResponse is the standard envelope used by HTTP handlers to return
// success or failure along with optional data.
type APIResponse struct {
	OK     bool        `json:"ok"`
	ErrMsg string      `json:"errmsg,omitempty"`
	Data   interface{} `json:"data,omitempty"`
}

// UnitInfo contains a brief set of properties about a systemd unit used in
// list and status responses. The fields mirror systemd properties and are
// encoded as JSON.
type UnitInfo struct {
	Unit   string      `json:"unit"`
	Active interface{} `json:"active,omitempty"`
	Sub    interface{} `json:"sub,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// UnitActionRequest is the JSON body used for unit control actions such as
// start/stop/restart.
type UnitActionRequest struct {
	Unit string `json:"unit"`
}

// UnitActionResponse is returned after performing a unit action.
type UnitActionResponse struct {
	Unit   string `json:"unit"`
	Result string `json:"result,omitempty"`
}

var (
	// AllowedUnits contains the set of unit names the agent is permitted
	// to operate on. It is populated by LoadConfigFrom.
	AllowedUnits map[string]struct{}

	// ServerKey is the Bearer token required to access authenticated API
	// endpoints. It is loaded from the configuration and may be rotated
	// by updating the config and reloading.
	ServerKey string

	// ConfigPath holds the path to the active configuration file. It must
	// be set by the caller (main) before calling ReloadConfig.
	ConfigPath string

	// MediaPiServiceUser is the username for crontab and systemd timer operations.
	// It defaults to "pi" and is loaded from the configuration.
	MediaPiServiceUser string
)

// DefaultListenAddr is used when the configuration does not specify a
// listen address for the HTTP API.
const DefaultListenAddr = "0.0.0.0:8081"

// Version can be set at build time with -ldflags
var Version = "unknown"

// GetVersion returns the version string for the running binary. When the
// Version variable is not set at build time it will try to derive a value
// from git tags; otherwise it returns "unknown".
func GetVersion() string {
	if Version != "unknown" {
		return Version
	}

	if cmd := exec.Command("git", "describe", "--tags", "--abbrev=0"); cmd != nil {
		if output, err := cmd.Output(); err == nil {
			if version := strings.TrimSpace(string(output)); version != "" {
				return version
			}
		}
	}

	return "unknown"
}

// DefaultConfig returns a reasonable default Config.
func DefaultConfig() Config {
	return Config{
		AllowedUnits:       []string{},
		ListenAddr:         DefaultListenAddr,
		MediaPiServiceUser: "pi",
	}
}

// LoadConfigFrom loads configuration from path and updates package-level
// state (AllowedUnits, ServerKey). It returns the parsed Config to the
// caller for further use.
func LoadConfigFrom(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}

	newAllowedUnits := make(map[string]struct{}, len(c.AllowedUnits))
	for _, u := range c.AllowedUnits {
		newAllowedUnits[u] = struct{}{}
	}

	if c.ServerKey == "" {
		return nil, fmt.Errorf("server_key is required in configuration")
	}

	// Set default media-pi service user if not specified
	if c.MediaPiServiceUser == "" {
		c.MediaPiServiceUser = "pi"
	}

	AllowedUnits = newAllowedUnits
	ServerKey = c.ServerKey
	MediaPiServiceUser = c.MediaPiServiceUser

	return &c, nil
}

// ReloadConfig reloads configuration from the previously set ConfigPath.
// Callers must set ConfigPath before invoking ReloadConfig (for example in
// main after the initial load). ReloadConfig updates package globals
// (AllowedUnits, ServerKey) by reusing LoadConfigFrom.
func ReloadConfig() error {
	if ConfigPath == "" {
		return fmt.Errorf("config path is not set")
	}
	cfg, err := LoadConfigFrom(ConfigPath)
	if err != nil {
		return err
	}
	// Optionally we could do something with cfg here in the future.
	_ = cfg
	return nil
}

// HandleReload is an authenticated HTTP handler that triggers a
// configuration reload. It accepts POST requests and returns 204 on
// success.
func HandleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{OK: false, ErrMsg: "method not allowed"})
		return
	}

	if err := ReloadConfig(); err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{OK: false, ErrMsg: err.Error()})
		return
	}

	// No content - reload successful
	w.WriteHeader(http.StatusNoContent)
}

// GenerateServerKey creates a random 32-byte server key encoded as
// hexadecimal. It is suitable for storing in the config file.
func GenerateServerKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// SetupConfig creates or updates the configuration file at configPath.
// It generates a new ServerKey and writes the file with secure
// permissions.
func SetupConfig(configPath string) error {
	config := DefaultConfig()
	existing := false

	data, err := os.ReadFile(configPath)
	switch {
	case err == nil:
		existing = true
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse existing config: %w", err)
		}
		if config.ServerKey != "" {
			fmt.Printf("Warning: configuration at %s already has a server_key; it will be overwritten\n", configPath)
		}
		if config.ListenAddr == "" {
			config.ListenAddr = DefaultListenAddr
		}
	case errors.Is(err, os.ErrNotExist):
		// use defaults
	default:
		return fmt.Errorf("failed to read existing config: %w", err)
	}

	key, err := GenerateServerKey()
	if err != nil {
		return fmt.Errorf("failed to generate server key: %w", err)
	}

	config.ServerKey = key

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err = yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	if existing {
		fmt.Printf("Configuration updated at %s\n", configPath)
	} else {
		fmt.Printf("Configuration created at %s\n", configPath)
	}
	fmt.Println("The key is saved in the agent configuration file, it will be used for API access")

	return nil
}

// IsAllowed returns nil when the provided unit is present in AllowedUnits
// and an error otherwise.
func IsAllowed(unit string) error {
	if _, ok := AllowedUnits[unit]; !ok {
		return fmt.Errorf("управление сервисом %q запрещено", unit)
	}
	return nil
}

// AuthMiddleware enforces Bearer token authentication using ServerKey and
// invokes the next handler when authentication succeeds.
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			JSONResponse(w, http.StatusUnauthorized, APIResponse{OK: false, ErrMsg: "Требуется заголовок Authorization"})
			return
		}

		if !strings.HasPrefix(auth, "Bearer ") {
			JSONResponse(w, http.StatusUnauthorized, APIResponse{OK: false, ErrMsg: "Требуется токен Bearer"})
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(ServerKey)) != 1 {
			JSONResponse(w, http.StatusUnauthorized, APIResponse{OK: false, ErrMsg: "Недействительный токен"})
			return
		}

		next(w, r)
	}
}

// JSONResponse writes an APIResponse as JSON with the provided HTTP status
// code and sets the Content-Type header.
func JSONResponse(w http.ResponseWriter, status int, response APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

// HandleListUnits returns state for all allowed units as JSON. It requires
// a GET request and authentication.
func HandleListUnits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
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

	var infos []UnitInfo
	for unit := range AllowedUnits {
		props, err := conn.GetUnitPropertiesContext(ctx, unit)
		if err != nil {
			infos = append(infos, UnitInfo{Unit: unit, Error: err.Error()})
			continue
		}
		infos = append(infos, UnitInfo{
			Unit:   unit,
			Active: props["ActiveState"],
			Sub:    props["SubState"],
		})
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK:   true,
		Data: infos,
	})
}

// HandleUnitStatus returns state for a single allowed unit. It requires a
// GET request and the "unit" query parameter.
func HandleUnitStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Метод не разрешён",
		})
		return
	}

	unit := r.URL.Query().Get("unit")
	if unit == "" {
		JSONResponse(w, http.StatusBadRequest, APIResponse{
			OK:     false,
			ErrMsg: "Требуется параметр unit",
		})
		return
	}

	if err := IsAllowed(unit); err != nil {
		JSONResponse(w, http.StatusForbidden, APIResponse{
			OK:     false,
			ErrMsg: err.Error(),
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

	props, err := conn.GetUnitPropertiesContext(ctx, unit)
	if err != nil {
		JSONResponse(w, http.StatusInternalServerError, APIResponse{
			OK:     false,
			ErrMsg: err.Error(),
		})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: UnitInfo{
			Unit:   unit,
			Active: props["ActiveState"],
			Sub:    props["SubState"],
		},
	})
}

// HandleUnitAction returns an HTTP handler which performs the specified
// action (start/stop/restart/enable/disable) on the unit provided in the
// request body.
func HandleUnitAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
				OK:     false,
				ErrMsg: "Метод запрещён",
			})
			return
		}

		var req UnitActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			JSONResponse(w, http.StatusBadRequest, APIResponse{
				OK:     false,
				ErrMsg: "Неверный JSON в теле запроса",
			})
			return
		}

		if req.Unit == "" {
			JSONResponse(w, http.StatusBadRequest, APIResponse{
				OK:     false,
				ErrMsg: "Поле unit обязательно",
			})
			return
		}

		if err := IsAllowed(req.Unit); err != nil {
			JSONResponse(w, http.StatusForbidden, APIResponse{
				OK:     false,
				ErrMsg: err.Error(),
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

		var result string
		var actionErr error

		switch action {
		case "start":
			ch := make(chan string, 1)
			_, actionErr = conn.StartUnitContext(ctx, req.Unit, "replace", ch)
			if actionErr == nil {
				result = <-ch
			}
		case "stop":
			ch := make(chan string, 1)
			_, actionErr = conn.StopUnitContext(ctx, req.Unit, "replace", ch)
			if actionErr == nil {
				result = <-ch
			}
		case "restart":
			ch := make(chan string, 1)
			_, actionErr = conn.RestartUnitContext(ctx, req.Unit, "replace", ch)
			if actionErr == nil {
				result = <-ch
			}
		case "enable":
			_, _, actionErr = conn.EnableUnitFilesContext(ctx, []string{req.Unit}, false, true)
			if actionErr == nil {
				result = "enabled"
			}
		case "disable":
			_, actionErr = conn.DisableUnitFilesContext(ctx, []string{req.Unit}, false)
			if actionErr == nil {
				result = "disabled"
			}
		default:
			JSONResponse(w, http.StatusBadRequest, APIResponse{
				OK:     false,
				ErrMsg: fmt.Sprintf("Неизвестное действие: %s", action),
			})
			return
		}

		if actionErr != nil {
			JSONResponse(w, http.StatusInternalServerError, APIResponse{
				OK:     false,
				ErrMsg: "Выполнение действия завершилось с ошибкой: " + actionErr.Error(),
			})
			return
		}

		JSONResponse(w, http.StatusOK, APIResponse{
			OK: true,
			Data: UnitActionResponse{
				Unit:   req.Unit,
				Result: result,
			},
		})
	}
}

// HandleHealth provides a simple healthcheck endpoint with version and
// timestamp information.
func HandleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		JSONResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:     false,
			ErrMsg: "Метод не разрешён",
		})
		return
	}

	JSONResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: map[string]string{
			"status":  "healthy",
			"version": GetVersion(),
			"time":    time.Now().UTC().Format(time.RFC3339),
		},
	})
}
