//go:build integration

package integration

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
}

func (f *fakePlaylistConn) Close()                                  {}
func (f *fakePlaylistConn) ReloadContext(ctx context.Context) error { return nil }
func (f *fakePlaylistConn) StartUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
	f.startCalled = true
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

func TestPlaylistStartUpload_CallsDBusStart(t *testing.T) {
	fake := &fakePlaylistConn{}
	agent.SetDBusConnectionFactory(func(ctx context.Context) (agent.DBusConnection, error) {
		return fake, nil
	})
	defer agent.SetDBusConnectionFactory(nil)

	req := httptest.NewRequest(http.MethodPost, "/api/menu/playlist/start-upload", nil)
	rr := httptest.NewRecorder()

	agent.HandlePlaylistStartUpload(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Result().StatusCode)
	}

	// Give goroutine-channel a short moment (handlers are synchronous but use channel)
	time.Sleep(10 * time.Millisecond)

	if !fake.startCalled {
		t.Fatalf("expected StartUnitContext to be called on DBus connection")
	}
}

func TestPlaylistStopUpload_CallsDBusStop(t *testing.T) {
	fake := &fakePlaylistConn{}
	agent.SetDBusConnectionFactory(func(ctx context.Context) (agent.DBusConnection, error) {
		return fake, nil
	})
	defer agent.SetDBusConnectionFactory(nil)

	req := httptest.NewRequest(http.MethodPost, "/api/menu/playlist/stop-upload", nil)
	rr := httptest.NewRecorder()

	agent.HandlePlaylistStopUpload(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Result().StatusCode)
	}

	time.Sleep(10 * time.Millisecond)

	if !fake.stopCalled {
		t.Fatalf("expected StopUnitContext to be called on DBus connection")
	}
}
