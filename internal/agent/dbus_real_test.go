package agent

import (
    "context"
    "testing"

    "github.com/coreos/go-systemd/v22/dbus"
)

// fakeSystemd captures calls made by realDBusConnection and allows assertions.
type fakeSystemd struct{
    closed bool
    started []string
    stopped []string
}

func (f *fakeSystemd) Close(){ f.closed = true }
func (f *fakeSystemd) ReloadContext(ctx context.Context) error { return nil }
func (f *fakeSystemd) StartUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
    f.started = append(f.started, name)
    if ch!=nil { ch <- "ok" }
    return 1, nil
}
func (f *fakeSystemd) StopUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
    f.stopped = append(f.stopped, name)
    if ch!=nil { ch <- "ok" }
    return 1, nil
}
func (f *fakeSystemd) RestartUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) { return 1, nil }
func (f *fakeSystemd) EnableUnitFilesContext(ctx context.Context, files []string, runtime, force bool) (bool, []dbus.EnableUnitFileChange, error) { return true, nil, nil }
func (f *fakeSystemd) DisableUnitFilesContext(ctx context.Context, files []string, runtime bool) ([]dbus.DisableUnitFileChange, error) { return nil, nil }
func (f *fakeSystemd) GetUnitPropertiesContext(ctx context.Context, unit string) (map[string]any, error) { return map[string]any{"ActiveState":"inactive"}, nil }

type fakeLogin1 struct{
    rebooted bool
    poweredOff bool
    closed bool
}

func (l *fakeLogin1) Close(){ l.closed = true }
func (l *fakeLogin1) Reboot(askForAuth bool){ l.rebooted = true }
func (l *fakeLogin1) PowerOff(askForAuth bool){ l.poweredOff = true }

func TestRealDBusConnectionDelegates(t *testing.T) {
    sys := &fakeSystemd{}
    login := &fakeLogin1{}
    r := &realDBusConnection{sys: sys, login: login}

    // Start unit
    ch := make(chan string,1)
    if _, err := r.StartUnitContext(context.Background(), "foo.service", "replace", ch); err!=nil {
        t.Fatalf("StartUnitContext failed: %v", err)
    }
    if len(sys.started)!=1 || sys.started[0] != "foo.service" { t.Fatalf("expected start called for foo.service, got %v", sys.started) }

    // Stop unit
    if _, err := r.StopUnitContext(context.Background(), "foo.service", "replace", ch); err!=nil {
        t.Fatalf("StopUnitContext failed: %v", err)
    }
    if len(sys.stopped)!=1 || sys.stopped[0] != "foo.service" { t.Fatalf("expected stop called for foo.service, got %v", sys.stopped) }

    // Reboot/PowerOff delegation
    if err := r.RebootContext(context.Background()); err!=nil { t.Fatalf("RebootContext error: %v", err) }
    if !login.rebooted { t.Fatalf("expected login.Reboot to be called") }

    if err := r.PowerOffContext(context.Background()); err!=nil { t.Fatalf("PowerOffContext error: %v", err) }
    if !login.poweredOff { t.Fatalf("expected login.PowerOff to be called") }

    // Close should close both
    r.Close()
    if !sys.closed { t.Fatalf("expected systemd Close called") }
    if !login.closed { t.Fatalf("expected login Close called") }
}
