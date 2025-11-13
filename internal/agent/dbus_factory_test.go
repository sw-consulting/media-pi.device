package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coreos/go-systemd/v22/dbus"
)

// fakeConn implements agent.DBusConnection for tests. Only ReloadContext is
// observed; other methods return zero values.
type fakeConn struct {
	calledReload bool
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
	return 0, nil
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

	// Inject factory that returns our fake connection
	SetDBusConnectionFactory(func(ctx context.Context) (DBusConnection, error) {
		return fake, nil
	})
	// restore default factory after test
	defer SetDBusConnectionFactory(nil)

	req := httptest.NewRequest(http.MethodPost, "/api/menu/system/reload", nil)
	rr := httptest.NewRecorder()

	HandleSystemReload(rr, req)

	if rr.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", rr.Result().StatusCode)
	}

	if !fake.calledReload {
		t.Fatalf("expected ReloadContext to be called on injected DBus connection")
	}
}
