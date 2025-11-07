// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package agent

// This package contains the core agent functionality extracted from main
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

	"github.com/coreos/go-systemd/v22/dbus"
	"gopkg.in/yaml.v3"
)

type Config struct {
	AllowedUnits []string `yaml:"allowed_units"`
	ServerKey    string   `yaml:"server_key,omitempty"`
	ListenAddr   string   `yaml:"listen_addr,omitempty"`
}

type APIResponse struct {
	OK     bool        `json:"ok"`
	ErrMsg string      `json:"errmsg,omitempty"`
	Data   interface{} `json:"data,omitempty"`
}

type UnitInfo struct {
	Unit   string      `json:"unit"`
	Active interface{} `json:"active,omitempty"`
	Sub    interface{} `json:"sub,omitempty"`
	Error  string      `json:"error,omitempty"`
}

type UnitActionRequest struct {
	Unit string `json:"unit"`
}

type UnitActionResponse struct {
	Unit   string `json:"unit"`
	Result string `json:"result,omitempty"`
}

var (
	AllowedUnits map[string]struct{}
	ServerKey    string
)

const DefaultListenAddr = "0.0.0.0:8081"

// Version can be set at build time with -ldflags
var Version = "unknown"

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

func DefaultConfig() Config {
	return Config{
		AllowedUnits: []string{},
		ListenAddr:   DefaultListenAddr,
	}
}

func LoadConfigFrom(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}

	AllowedUnits = make(map[string]struct{}, len(c.AllowedUnits))
	for _, u := range c.AllowedUnits {
		AllowedUnits[u] = struct{}{}
	}

	ServerKey = c.ServerKey
	if ServerKey == "" {
		return nil, fmt.Errorf("server_key is required in configuration")
	}

	return &c, nil
}

func GenerateServerKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

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

func IsAllowed(unit string) error {
	if _, ok := AllowedUnits[unit]; !ok {
		return fmt.Errorf("управление сервисом %q запрещено", unit)
	}
	return nil
}

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

func JSONResponse(w http.ResponseWriter, status int, response APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

func HandleListUnits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
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
