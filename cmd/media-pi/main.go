// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

// Package main implements the media-pi-agent CLI & HTTP service. The
// binary supports a `setup` command which writes a configuration file and
// exits, and otherwise runs an HTTP API that controls allowed systemd
// units. Configuration is read from `/etc/media-pi-agent/agent.yaml` by
// default; tests can override that path with the `MEDIA_PI_AGENT_CONFIG`
// environment variable.
package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sw-consulting/media-pi.device/internal/agent"
)

// server timeouts protect the HTTP server from slowloris-style attacks and
// hung connections. Values are conservative for embedded devices.
const (
	serverReadTimeout  = 15 * time.Second
	serverWriteTimeout = 15 * time.Second
	serverIdleTimeout  = 60 * time.Second
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if len(os.Args) > 1 && os.Args[1] == "setup" {
		configPath := "/etc/media-pi-agent/agent.yaml"
		if len(os.Args) > 2 {
			configPath = os.Args[2]
		}
		if err := agent.SetupConfig(configPath); err != nil {
			log.Fatalf("Setup failed: %v", err)
		}
		return
	}

	// Allow tests and packaging to override the config path via environment
	// variable so integration tests can run without needing /etc access.
	configPath := os.Getenv("MEDIA_PI_AGENT_CONFIG")
	if configPath == "" {
		configPath = "/etc/media-pi-agent/agent.yaml"
	}

	cfg, err := agent.LoadConfigFrom(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	// Make the loaded config path available to the agent package for reloads
	agent.ConfigPath = configPath

	mux := http.NewServeMux()
	mux.HandleFunc("/health", agent.HandleHealth)
	// internal authenticated reload endpoint - used by setup scripts or ExecReload
	mux.HandleFunc("/internal/reload", agent.AuthMiddleware(agent.HandleReload))
	mux.HandleFunc("/api/units", agent.AuthMiddleware(agent.HandleListUnits))
	mux.HandleFunc("/api/units/status", agent.AuthMiddleware(agent.HandleUnitStatus))
	mux.HandleFunc("/api/units/start", agent.AuthMiddleware(agent.HandleUnitAction("start")))
	mux.HandleFunc("/api/units/stop", agent.AuthMiddleware(agent.HandleUnitAction("stop")))
	mux.HandleFunc("/api/units/restart", agent.AuthMiddleware(agent.HandleUnitAction("restart")))
	mux.HandleFunc("/api/units/enable", agent.AuthMiddleware(agent.HandleUnitAction("enable")))
	mux.HandleFunc("/api/units/disable", agent.AuthMiddleware(agent.HandleUnitAction("disable")))

	// Menu endpoints
	mux.HandleFunc("/api/menu", agent.AuthMiddleware(agent.HandleMenuList))
	mux.HandleFunc("/api/menu/playback/stop", agent.AuthMiddleware(agent.HandlePlaybackStop))
	mux.HandleFunc("/api/menu/playback/start", agent.AuthMiddleware(agent.HandlePlaybackStart))
	mux.HandleFunc("/api/menu/service/status", agent.AuthMiddleware(agent.HandleServiceStatus))
	mux.HandleFunc("/api/menu/configuration/get", agent.AuthMiddleware(agent.HandleConfigurationGet))
	mux.HandleFunc("/api/menu/configuration/update", agent.AuthMiddleware(agent.HandleConfigurationUpdate))
	mux.HandleFunc("/api/menu/playlist/start-upload", agent.AuthMiddleware(agent.HandlePlaylistStartUpload))
	mux.HandleFunc("/api/menu/playlist/stop-upload", agent.AuthMiddleware(agent.HandlePlaylistStopUpload))
	mux.HandleFunc("/api/menu/video/start-upload", agent.AuthMiddleware(agent.HandleVideoStartUpload))
	mux.HandleFunc("/api/menu/video/stop-upload", agent.AuthMiddleware(agent.HandleVideoStopUpload))
	mux.HandleFunc("/api/menu/sync/start", agent.AuthMiddleware(agent.HandleSyncStart))
	mux.HandleFunc("/api/menu/sync/schedule/get", agent.AuthMiddleware(agent.HandleSyncScheduleGet))
	mux.HandleFunc("/api/menu/sync/schedule/update", agent.AuthMiddleware(agent.HandleSyncScheduleUpdate))
	mux.HandleFunc("/api/menu/system/reload", agent.AuthMiddleware(agent.HandleSystemReload))
	mux.HandleFunc("/api/menu/system/reboot", agent.AuthMiddleware(agent.HandleSystemReboot))
	mux.HandleFunc("/api/menu/system/shutdown", agent.AuthMiddleware(agent.HandleSystemShutdown))

	listenAddr := cfg.ListenAddr
	if listenAddr == "" {
		listenAddr = agent.DefaultListenAddr
	}
	log.Printf("Starting Media Pi Agent REST service on %s", listenAddr)

	// Start sync scheduler if enabled
	if agent.CurrentSyncConfig.Enabled {
		log.Printf("Starting sync scheduler")
		agent.StartSyncScheduler()
		defer agent.StopSyncScheduler()
	}

	server := &http.Server{
		Addr:         listenAddr,
		Handler:      mux,
		ReadTimeout:  serverReadTimeout,
		WriteTimeout: serverWriteTimeout,
		IdleTimeout:  serverIdleTimeout,
	}

	// Handle SIGHUP to reload configuration without restarting the process.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGHUP)
	go func() {
		for range sigs {
			log.Printf("received SIGHUP, reloading configuration")
			if err := agent.ReloadConfig(); err != nil {
				log.Printf("reload failed: %v", err)
			} else {
				log.Printf("configuration reloaded")
			}
		}
	}()
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
