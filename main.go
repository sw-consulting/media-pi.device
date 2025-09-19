// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
	"gopkg.in/yaml.v3"
)

type Config struct {
	AllowedUnits []string `yaml:"allowed_units"`
}

func loadConfig() (map[string]struct{}, error) {
	return loadConfigFrom("/etc/media-pi-agent/agent.yaml")
}

// loadConfigFrom читает конфигурацию из указанного пути (useful for tests)
func loadConfigFrom(path string) (map[string]struct{}, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	allow := make(map[string]struct{}, len(c.AllowedUnits))
	for _, u := range c.AllowedUnits {
		allow[u] = struct{}{}
	}
	return allow, nil
}

func allowed(allow map[string]struct{}, unit string) error {
	if _, ok := allow[unit]; !ok {
		return fmt.Errorf("unit %q is not allowed", unit)
	}
	return nil
}

func fatalJSON(err error) {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"ok":    false,
		"error": err.Error(),
	})
	os.Exit(1)
}

func main() {
	log.SetFlags(0)

	if len(os.Args) < 2 {
		fmt.Println(`Usage: media-pi-agent <list|status|start|stop|restart|enable|disable> [unit]`)
		os.Exit(2)
	}

	cmd := os.Args[1]
	allow, err := loadConfig()
	if err != nil {
		fatalJSON(fmt.Errorf("config: %w", err))
	}

	conn, err := dbus.NewWithContext(context.Background())
	if err != nil {
		fatalJSON(fmt.Errorf("dbus: %w", err))
	}
	defer conn.Close()

	switch cmd {
	case "list":
		// For each allowed unit, gather the same status information as the "status" command
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		type unitInfo struct {
			Unit   string      `json:"unit"`
			Active interface{} `json:"active,omitempty"`
			Sub    interface{} `json:"sub,omitempty"`
			Error  string      `json:"error,omitempty"`
		}

		var infos []unitInfo
		for u := range allow {
			props, err := conn.GetUnitPropertiesContext(ctx, u)
			if err != nil {
				infos = append(infos, unitInfo{Unit: u, Error: err.Error()})
				continue
			}
			infos = append(infos, unitInfo{Unit: u, Active: props["ActiveState"], Sub: props["SubState"]})
		}

		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
			"ok":    true,
			"units": infos,
		})
	case "status", "start", "stop", "restart", "enable", "disable":
		if len(os.Args) < 3 {
			fatalJSON(errors.New("missing unit"))
		}
		unit := os.Args[2]
		if err := allowed(allow, unit); err != nil {
			fatalJSON(err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		switch cmd {
		case "status":
			props, err := conn.GetUnitPropertiesContext(ctx, unit)
			if err != nil {
				fatalJSON(err)
			}
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
				"ok":     true,
				"unit":   unit,
				"active": props["ActiveState"],
				"sub":    props["SubState"],
			})
		case "start":
			ch := make(chan string, 1)
			_, err = conn.StartUnitContext(ctx, unit, "replace", ch)
			if err != nil {
				fatalJSON(err)
			}
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": true, "unit": unit, "result": <-ch})
		case "stop":
			ch := make(chan string, 1)
			_, err = conn.StopUnitContext(ctx, unit, "replace", ch)
			if err != nil {
				fatalJSON(err)
			}
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": true, "unit": unit, "result": <-ch})
		case "restart":
			ch := make(chan string, 1)
			_, err = conn.RestartUnitContext(ctx, unit, "replace", ch)
			if err != nil {
				fatalJSON(err)
			}
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": true, "unit": unit, "result": <-ch})
		case "enable":
			_, _, err = conn.EnableUnitFilesContext(ctx, []string{unit}, false, true)
			if err != nil {
				fatalJSON(err)
			}
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": true, "unit": unit, "enabled": true})
		case "disable":
			_, err = conn.DisableUnitFilesContext(ctx, []string{unit}, false)
			if err != nil {
				fatalJSON(err)
			}
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": true, "unit": unit, "enabled": false})
		}
	default:
		fatalJSON(fmt.Errorf("unknown command %q", cmd))
	}
}
