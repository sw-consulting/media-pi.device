//go:build test

// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
	agent "github.com/sw-consulting/media-pi.device/internal/agent"
)

// fakePlaylistConn records whether StartUnitContext/StopUnitContext were called.
type fakePlaylistConn struct {
	startCalled bool
	stopCalled  bool
	startUnit   string
	stopUnit    string
}

func (f *fakePlaylistConn) Close()                                  {}
func (f *fakePlaylistConn) ReloadContext(ctx context.Context) error { return nil }
func (f *fakePlaylistConn) StartUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
	f.startCalled = true
	f.startUnit = name
	if ch != nil {
		select {
		case ch <- "done":
		default:
		}
	}
	return 1, nil
}
func (f *fakePlaylistConn) StopUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
	f.stopCalled = true
	f.stopUnit = name
	if ch != nil {
		select {
		case ch <- "done":
		default:
		}
	}
	return 1, nil
}
func (f *fakePlaylistConn) RestartUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
	return 0, nil
}
func (f *fakePlaylistConn) EnableUnitFilesContext(ctx context.Context, files []string, runtime, force bool) (bool, []dbus.EnableUnitFileChange, error) {
	return true, nil, nil
}
func (f *fakePlaylistConn) DisableUnitFilesContext(ctx context.Context, files []string, runtime bool) ([]dbus.DisableUnitFileChange, error) {
	return nil, nil
}
func (f *fakePlaylistConn) GetUnitPropertiesContext(ctx context.Context, unit string) (map[string]any, error) {
	return map[string]any{}, nil
}
func (f *fakePlaylistConn) RebootContext(ctx context.Context) error   { return nil }
func (f *fakePlaylistConn) PowerOffContext(ctx context.Context) error { return nil }

func TestUploadServiceActions_CallsDBus(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		handler      func(http.ResponseWriter, *http.Request)
		expectStart  bool
		expectStop   bool
		expectedUnit string
	}{
		{
			name:         "playlist start",
			path:         "/api/menu/playlist/start-upload",
			handler:      agent.HandlePlaylistStartUpload,
			expectStart:  true,
			expectedUnit: "playlist.upload.service",
		},
		{
			name:         "playlist stop",
			path:         "/api/menu/playlist/stop-upload",
			handler:      agent.HandlePlaylistStopUpload,
			expectStop:   true,
			expectedUnit: "playlist.upload.service",
		},
		{
			name:         "video start",
			path:         "/api/menu/video/start-upload",
			handler:      agent.HandleVideoStartUpload,
			expectStart:  true,
			expectedUnit: "video.upload.service",
		},
		{
			name:         "video stop",
			path:         "/api/menu/video/stop-upload",
			handler:      agent.HandleVideoStopUpload,
			expectStop:   true,
			expectedUnit: "video.upload.service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakePlaylistConn{}
			agent.SetDBusConnectionFactory(func(ctx context.Context) (agent.DBusConnection, error) {
				return fake, nil
			})
			t.Cleanup(func() { agent.SetDBusConnectionFactory(nil) })

			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			rr := httptest.NewRecorder()

			tt := tt // capture range variable
			tt.handler(rr, req)

			if rr.Result().StatusCode != http.StatusOK {
				t.Fatalf("expected 200 OK, got %d", rr.Result().StatusCode)
			}

			time.Sleep(10 * time.Millisecond)

			if tt.expectStart {
				if !fake.startCalled {
					t.Fatalf("expected StartUnitContext to be called on DBus connection")
				}
				if fake.startUnit != tt.expectedUnit {
					t.Fatalf("expected start unit %s, got %s", tt.expectedUnit, fake.startUnit)
				}
			}

			if tt.expectStop {
				if !fake.stopCalled {
					t.Fatalf("expected StopUnitContext to be called on DBus connection")
				}
				if fake.stopUnit != tt.expectedUnit {
					t.Fatalf("expected stop unit %s, got %s", tt.expectedUnit, fake.stopUnit)
				}
			}
		})
	}
}
