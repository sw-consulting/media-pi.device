// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package main

import (
	"log"
	"net/http"
	"os"

	"github.com/swconsulting/media-pi.device/internal/agent"
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

	cfg, err := agent.LoadConfigFrom("/etc/media-pi-agent/agent.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", agent.HandleHealth)
	mux.HandleFunc("/api/units", agent.AuthMiddleware(agent.HandleListUnits))
	mux.HandleFunc("/api/units/status", agent.AuthMiddleware(agent.HandleUnitStatus))
	mux.HandleFunc("/api/units/start", agent.AuthMiddleware(agent.HandleUnitAction("start")))
	mux.HandleFunc("/api/units/stop", agent.AuthMiddleware(agent.HandleUnitAction("stop")))
	mux.HandleFunc("/api/units/restart", agent.AuthMiddleware(agent.HandleUnitAction("restart")))
	mux.HandleFunc("/api/units/enable", agent.AuthMiddleware(agent.HandleUnitAction("enable")))
	mux.HandleFunc("/api/units/disable", agent.AuthMiddleware(agent.HandleUnitAction("disable")))

	listenAddr := cfg.ListenAddr
	if listenAddr == "" {
		listenAddr = agent.DefaultListenAddr
	}
	log.Printf("Starting Media Pi Agent REST service on %s", listenAddr)

	server := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
