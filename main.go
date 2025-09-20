// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package main

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
	OK    bool        `json:"ok"`
	Error string      `json:"error,omitempty"`
	Data  interface{} `json:"data,omitempty"`
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
	allowedUnits map[string]struct{}
	serverKey    string
)

const defaultListenAddr = "0.0.0.0:8080"

func defaultConfig() Config {
	return Config{
		AllowedUnits: []string{},
		ListenAddr:   defaultListenAddr,
	}
}

func loadConfig() (*Config, error) {
	return loadConfigFrom("/etc/media-pi-agent/agent.yaml")
}

func loadConfigFrom(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}

	allowedUnits = make(map[string]struct{}, len(c.AllowedUnits))
	for _, u := range c.AllowedUnits {
		allowedUnits[u] = struct{}{}
	}

	serverKey = c.ServerKey
	if serverKey == "" {
		return nil, fmt.Errorf("server_key is required in configuration")
	}

	return &c, nil
}

func generateServerKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func setupConfig(configPath string) error {
	config := defaultConfig()
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
			config.ListenAddr = defaultListenAddr
		}
	case errors.Is(err, os.ErrNotExist):
		// Use defaults defined above.
	default:
		return fmt.Errorf("failed to read existing config: %w", err)
	}

	key, err := generateServerKey()
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
	fmt.Printf("Server key: %s\n", key)
	fmt.Println("The key is saved in the agent configuration file, it will be used for API access")

	return nil
}

func isAllowed(unit string) error {
	if _, ok := allowedUnits[unit]; !ok {
		return fmt.Errorf("unit %q is not allowed", unit)
	}
	return nil
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "Bearer token required", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(serverKey)) != 1 {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}

func jsonResponse(w http.ResponseWriter, status int, response APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
	}
}

func handleListUnits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:    false,
			Error: "Method not allowed",
		})
		return
	}

	conn, err := dbus.NewWithContext(context.Background())
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, APIResponse{
			OK:    false,
			Error: fmt.Sprintf("dbus connection failed: %v", err),
		})
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var infos []UnitInfo
	for unit := range allowedUnits {
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

	jsonResponse(w, http.StatusOK, APIResponse{
		OK:   true,
		Data: infos,
	})
}

func handleUnitStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:    false,
			Error: "Method not allowed",
		})
		return
	}

	unit := r.URL.Query().Get("unit")
	if unit == "" {
		jsonResponse(w, http.StatusBadRequest, APIResponse{
			OK:    false,
			Error: "unit parameter is required",
		})
		return
	}

	if err := isAllowed(unit); err != nil {
		jsonResponse(w, http.StatusForbidden, APIResponse{
			OK:    false,
			Error: err.Error(),
		})
		return
	}

	conn, err := dbus.NewWithContext(context.Background())
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, APIResponse{
			OK:    false,
			Error: fmt.Sprintf("dbus connection failed: %v", err),
		})
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	props, err := conn.GetUnitPropertiesContext(ctx, unit)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, APIResponse{
			OK:    false,
			Error: err.Error(),
		})
		return
	}

	jsonResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: UnitInfo{
			Unit:   unit,
			Active: props["ActiveState"],
			Sub:    props["SubState"],
		},
	})
}

func handleUnitAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonResponse(w, http.StatusMethodNotAllowed, APIResponse{
				OK:    false,
				Error: "Method not allowed",
			})
			return
		}

		var req UnitActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonResponse(w, http.StatusBadRequest, APIResponse{
				OK:    false,
				Error: "Invalid JSON body",
			})
			return
		}

		if req.Unit == "" {
			jsonResponse(w, http.StatusBadRequest, APIResponse{
				OK:    false,
				Error: "unit field is required",
			})
			return
		}

		if err := isAllowed(req.Unit); err != nil {
			jsonResponse(w, http.StatusForbidden, APIResponse{
				OK:    false,
				Error: err.Error(),
			})
			return
		}

		conn, err := dbus.NewWithContext(context.Background())
		if err != nil {
			jsonResponse(w, http.StatusInternalServerError, APIResponse{
				OK:    false,
				Error: fmt.Sprintf("dbus connection failed: %v", err),
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
			jsonResponse(w, http.StatusBadRequest, APIResponse{
				OK:    false,
				Error: fmt.Sprintf("unknown action: %s", action),
			})
			return
		}

		if actionErr != nil {
			jsonResponse(w, http.StatusInternalServerError, APIResponse{
				OK:    false,
				Error: actionErr.Error(),
			})
			return
		}

		jsonResponse(w, http.StatusOK, APIResponse{
			OK: true,
			Data: UnitActionResponse{
				Unit:   req.Unit,
				Result: result,
			},
		})
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonResponse(w, http.StatusMethodNotAllowed, APIResponse{
			OK:    false,
			Error: "Method not allowed",
		})
		return
	}

	jsonResponse(w, http.StatusOK, APIResponse{
		OK: true,
		Data: map[string]string{
			"status": "healthy",
			"time":   time.Now().UTC().Format(time.RFC3339),
		},
	})
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if len(os.Args) > 1 && os.Args[1] == "setup" {
		configPath := "/etc/media-pi-agent/agent.yaml"
		if len(os.Args) > 2 {
			configPath = os.Args[2]
		}
		if err := setupConfig(configPath); err != nil {
			log.Fatalf("Setup failed: %v", err)
		}
		return
	}

	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	mux := http.NewServeMux()

	// Public health endpoint
	mux.HandleFunc("/health", handleHealth)

	// Protected API endpoints
	mux.HandleFunc("/api/units", authMiddleware(handleListUnits))
	mux.HandleFunc("/api/units/status", authMiddleware(handleUnitStatus))
	mux.HandleFunc("/api/units/start", authMiddleware(handleUnitAction("start")))
	mux.HandleFunc("/api/units/stop", authMiddleware(handleUnitAction("stop")))
	mux.HandleFunc("/api/units/restart", authMiddleware(handleUnitAction("restart")))
	mux.HandleFunc("/api/units/enable", authMiddleware(handleUnitAction("enable")))
	mux.HandleFunc("/api/units/disable", authMiddleware(handleUnitAction("disable")))

	listenAddr := config.ListenAddr
	if listenAddr == "" {
		listenAddr = defaultListenAddr
	}
	log.Printf("Starting Media Pi Agent REST service on %s", listenAddr)
	log.Printf("API endpoints:")
	log.Printf("  GET  /health - Health check (no auth)")
	log.Printf("  GET  /api/units - List all units")
	log.Printf("  GET  /api/units/status?unit=<name> - Get unit status")
	log.Printf("  POST /api/units/start - Start unit")
	log.Printf("  POST /api/units/stop - Stop unit")
	log.Printf("  POST /api/units/restart - Restart unit")
	log.Printf("  POST /api/units/enable - Enable unit")
	log.Printf("  POST /api/units/disable - Disable unit")

	server := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
