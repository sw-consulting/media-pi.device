// Copyright (C) 2025-2026 sw.consulting
// This file is a part of Media Pi device agent

package agent

import (
	"context"
	"testing"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
)

type startupPlaybackConn struct {
	startedUnits []string
}

func (s *startupPlaybackConn) Close() {}

func (s *startupPlaybackConn) ReloadContext(ctx context.Context) error { return nil }

func (s *startupPlaybackConn) StartUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
	s.startedUnits = append(s.startedUnits, name)
	if ch != nil {
		ch <- "done"
	}
	return 1, nil
}

func (s *startupPlaybackConn) StopUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
	return 0, nil
}

func (s *startupPlaybackConn) RestartUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
	return 0, nil
}

func (s *startupPlaybackConn) EnableUnitFilesContext(ctx context.Context, files []string, runtime, force bool) (bool, []dbus.EnableUnitFileChange, error) {
	return true, nil, nil
}

func (s *startupPlaybackConn) DisableUnitFilesContext(ctx context.Context, files []string, runtime bool) ([]dbus.DisableUnitFileChange, error) {
	return nil, nil
}

func (s *startupPlaybackConn) GetUnitPropertiesContext(ctx context.Context, unit string) (map[string]any, error) {
	return map[string]any{}, nil
}

func (s *startupPlaybackConn) RebootContext(ctx context.Context) error { return nil }

func (s *startupPlaybackConn) PowerOffContext(ctx context.Context) error { return nil }

func TestEnsurePlaybackStateOnStartup_StartsOutsideRestInterval(t *testing.T) {
	originalFactory := dbusFactory
	conn := &startupPlaybackConn{}
	SetDBusConnectionFactory(func(ctx context.Context) (DBusConnection, error) {
		return conn, nil
	})
	t.Cleanup(func() { SetDBusConnectionFactory(originalFactory) })

	configMutex.Lock()
	originalConfig := currentConfig
	currentConfig = &Config{
		Schedule: ScheduleConfig{
			Rest: []RestTimePairConfig{{Start: "23:00", Stop: "07:00"}},
		},
	}
	configMutex.Unlock()
	t.Cleanup(func() {
		configMutex.Lock()
		currentConfig = originalConfig
		configMutex.Unlock()
	})

	now := time.Date(2026, time.April, 29, 8, 30, 0, 0, time.Local)
	if err := ensurePlaybackStateOnStartupAt(now); err != nil {
		t.Fatalf("ensurePlaybackStateOnStartupAt returned error: %v", err)
	}

	if len(conn.startedUnits) != 1 {
		t.Fatalf("expected playback service to be started once, got %d calls", len(conn.startedUnits))
	}
	if conn.startedUnits[0] != "play.video.service" {
		t.Fatalf("expected play.video.service to be started, got %q", conn.startedUnits[0])
	}
}

func TestEnsurePlaybackStateOnStartup_SkipsWithinRestInterval(t *testing.T) {
	originalFactory := dbusFactory
	conn := &startupPlaybackConn{}
	SetDBusConnectionFactory(func(ctx context.Context) (DBusConnection, error) {
		return conn, nil
	})
	t.Cleanup(func() { SetDBusConnectionFactory(originalFactory) })

	configMutex.Lock()
	originalConfig := currentConfig
	currentConfig = &Config{
		Schedule: ScheduleConfig{
			Rest: []RestTimePairConfig{{Start: "23:00", Stop: "07:00"}},
		},
	}
	configMutex.Unlock()
	t.Cleanup(func() {
		configMutex.Lock()
		currentConfig = originalConfig
		configMutex.Unlock()
	})

	now := time.Date(2026, time.April, 29, 23, 30, 0, 0, time.Local)
	if err := ensurePlaybackStateOnStartupAt(now); err != nil {
		t.Fatalf("ensurePlaybackStateOnStartupAt returned error: %v", err)
	}

	if len(conn.startedUnits) != 0 {
		t.Fatalf("expected playback service start to be skipped during rest interval, got %d calls", len(conn.startedUnits))
	}
}
