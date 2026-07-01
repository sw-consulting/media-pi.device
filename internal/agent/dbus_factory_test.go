// Copyright (C) 2025-2026 sw.consulting
// This file is a part of Media Pi device agent
package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
)

// fakeConn implements agent.DBusConnection for tests. ReloadContext and
// RestartUnitContext are observed; other methods return zero values.
type fakeConn struct {
	calledReload  bool
	calledRestart bool
}

func (f *fakeConn) Close()                                  {}
func (f *fakeConn) ReloadContext(ctx context.Context) error { f.calledReload = true; return nil }
func (f *fakeConn) StartUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
	return 0, nil
}
func (f *fakeConn) StopUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
	return 0, nil
}
func (f *fakeConn) RestartUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
	f.calledRestart = true
	if ch != nil {
		ch <- "done"
	}
	return 1, nil
}
func (f *fakeConn) EnableUnitFilesContext(ctx context.Context, files []string, runtime, force bool) (bool, []dbus.EnableUnitFileChange, error) {
	return true, nil, nil
}
func (f *fakeConn) DisableUnitFilesContext(ctx context.Context, files []string, runtime bool) ([]dbus.DisableUnitFileChange, error) {
	return nil, nil
}
func (f *fakeConn) GetUnitPropertiesContext(ctx context.Context, unit string) (map[string]any, error) {
	return map[string]any{}, nil
}
func (f *fakeConn) RebootContext(ctx context.Context) error   { return nil }
func (f *fakeConn) PowerOffContext(ctx context.Context) error { return nil }

func TestHandleSystemReload_UsesInjectedDBusFactory(t *testing.T) {
	fake := &fakeConn{}

	SetDBusConnectionFactory(func(ctx context.Context) (DBusConnection, error) {
		return fake, nil
	})

	configMutex.Lock()
	originalConfig := currentConfig
	currentConfig = &Config{}
	configMutex.Unlock()

	t.Cleanup(func() {
		SetDBusConnectionFactory(nil)
		configMutex.Lock()
		currentConfig = originalConfig
		configMutex.Unlock()
		cancelScheduledPlaylistPhotoCaptures()
	})

	req := httptest.NewRequest(http.MethodPost, "/api/menu/system/reload", nil)
	rr := httptest.NewRecorder()

	HandleSystemReload(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rr.Result().StatusCode)
	}

	if !fake.calledReload {
		t.Fatalf("expected ReloadContext to be called on injected DBus connection")
	}
	if !fake.calledRestart {
		t.Fatalf("expected RestartUnitContext to be called on injected DBus connection")
	}
}

func TestHandleSystemReloadUsesNormalTimeoutForDBusConnection(t *testing.T) {
	originalFactory := dbusFactory
	fake := &fakeConn{}
	var connectionTimeouts []time.Duration

	SetDBusConnectionFactory(func(ctx context.Context) (DBusConnection, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatalf("expected D-Bus connection context to have a deadline")
		}
		connectionTimeouts = append(connectionTimeouts, time.Until(deadline))
		return fake, nil
	})

	configMutex.Lock()
	originalConfig := currentConfig
	currentConfig = &Config{}
	configMutex.Unlock()

	originalDBusTimeout := dbusOperationTimeout
	originalPlaybackTimeout := playbackServiceOperationTimeout
	originalActiveCheckTimeout := playbackServiceActiveCheckTimeout
	dbusOperationTimeout = 50 * time.Millisecond
	playbackServiceOperationTimeout = time.Second
	playbackServiceActiveCheckTimeout = time.Second

	t.Cleanup(func() {
		SetDBusConnectionFactory(originalFactory)
		dbusOperationTimeout = originalDBusTimeout
		playbackServiceOperationTimeout = originalPlaybackTimeout
		playbackServiceActiveCheckTimeout = originalActiveCheckTimeout
		configMutex.Lock()
		currentConfig = originalConfig
		configMutex.Unlock()
		cancelScheduledPlaylistPhotoCaptures()
	})

	req := httptest.NewRequest(http.MethodPost, "/api/menu/system/reload", nil)
	rr := httptest.NewRecorder()

	HandleSystemReload(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rr.Result().StatusCode)
	}
	if len(connectionTimeouts) < 2 {
		t.Fatalf("expected reload and playback restart D-Bus connections, got %d", len(connectionTimeouts))
	}

	reloadTimeout := connectionTimeouts[0]
	playbackTimeout := connectionTimeouts[1]
	if reloadTimeout > dbusOperationTimeout+50*time.Millisecond {
		t.Fatalf("expected reload D-Bus connection to use normal timeout %s, got %s", dbusOperationTimeout, reloadTimeout)
	}
	if playbackTimeout <= dbusOperationTimeout+50*time.Millisecond {
		t.Fatalf("expected playback restart D-Bus connection to use longer playback timeout, got %s", playbackTimeout)
	}
}
