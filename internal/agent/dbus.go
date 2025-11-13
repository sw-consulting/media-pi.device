// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package agent

import (
	"context"
	"os"
	"sync"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/coreos/go-systemd/v22/login1"
)

// DBusConnection represents the subset of systemd D-Bus functionality used by the agent.
type DBusConnection interface {
	Close()
	ReloadContext(ctx context.Context) error
	StartUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error)
	StopUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error)
	RestartUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error)
	EnableUnitFilesContext(ctx context.Context, files []string, runtime, force bool) (bool, []dbus.EnableUnitFileChange, error)
	DisableUnitFilesContext(ctx context.Context, files []string, runtime bool) ([]dbus.DisableUnitFileChange, error)
	GetUnitPropertiesContext(ctx context.Context, unit string) (map[string]any, error)
	RebootContext(ctx context.Context) error
	PowerOffContext(ctx context.Context) error
}

// DBusConnectionFactory creates a DBusConnection using the provided context.
type DBusConnectionFactory func(ctx context.Context) (DBusConnection, error)

var (
	dbusFactoryMu sync.RWMutex
	dbusFactory   DBusConnectionFactory = defaultDBusConnectionFactory
)

func defaultDBusConnectionFactory(ctx context.Context) (DBusConnection, error) {
	if os.Getenv("MEDIA_PI_AGENT_MOCK_DBUS") == "1" {
		return &noopDBusConnection{}, nil
	}
	// Create the systemd manager connection
	sysconn, err := dbus.NewWithContext(ctx)
	if err != nil {
		return nil, err
	}

	// Create a login1 connection for power operations. If this fails, close
	// the systemd connection and return the error.
	loginConn, lerr := login1.New()
	if lerr != nil {
		sysconn.Close()
		return nil, lerr
	}

	return &realDBusConnection{sys: sysconn, login: loginConn}, nil
}

// SetDBusConnectionFactory overrides the factory used to create D-Bus connections.
// Passing nil restores the default factory.
func SetDBusConnectionFactory(factory DBusConnectionFactory) {
	dbusFactoryMu.Lock()
	defer dbusFactoryMu.Unlock()

	if factory == nil {
		dbusFactory = defaultDBusConnectionFactory
		return
	}
	dbusFactory = factory
}

func getDBusConnection(ctx context.Context) (DBusConnection, error) {
	dbusFactoryMu.RLock()
	factory := dbusFactory
	dbusFactoryMu.RUnlock()
	return factory(ctx)
}

// noopDBusConnection is a minimal in-memory stub used for tests.
type noopDBusConnection struct{}

func (n *noopDBusConnection) Close() {}

func (n *noopDBusConnection) ReloadContext(ctx context.Context) error {
	return nil
}

func (n *noopDBusConnection) StartUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
	if ch != nil {
		select {
		case ch <- "done":
		default:
		}
	}
	return 1, nil
}

func (n *noopDBusConnection) StopUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
	if ch != nil {
		select {
		case ch <- "done":
		default:
		}
	}
	return 1, nil
}

func (n *noopDBusConnection) RestartUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
	if ch != nil {
		select {
		case ch <- "done":
		default:
		}
	}
	return 1, nil
}

func (n *noopDBusConnection) EnableUnitFilesContext(ctx context.Context, files []string, runtime, force bool) (bool, []dbus.EnableUnitFileChange, error) {
	return true, nil, nil
}

func (n *noopDBusConnection) DisableUnitFilesContext(ctx context.Context, files []string, runtime bool) ([]dbus.DisableUnitFileChange, error) {
	return nil, nil
}

func (n *noopDBusConnection) GetUnitPropertiesContext(ctx context.Context, unit string) (map[string]any, error) {
	return map[string]any{
		"ActiveState": "inactive",
		"SubState":    "dead",
	}, nil
}

func (n *noopDBusConnection) RebootContext(ctx context.Context) error {
	return nil
}

func (n *noopDBusConnection) PowerOffContext(ctx context.Context) error {
	return nil
}

// Define small interfaces for the concrete systemd/login1 connections so
// tests can inject fakes. These mirror only the methods used by the agent.
type systemdConn interface {
	Close()
	ReloadContext(ctx context.Context) error
	StartUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error)
	StopUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error)
	RestartUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error)
	EnableUnitFilesContext(ctx context.Context, files []string, runtime, force bool) (bool, []dbus.EnableUnitFileChange, error)
	DisableUnitFilesContext(ctx context.Context, files []string, runtime bool) ([]dbus.DisableUnitFileChange, error)
	GetUnitPropertiesContext(ctx context.Context, unit string) (map[string]any, error)
}

type login1Conn interface {
	Close()
	Reboot(askForAuth bool)
	PowerOff(askForAuth bool)
}

// realDBusConnection is a thin adapter that implements DBusConnection by
// delegating calls to the underlying systemd dbus connection and the login1
// connection for power/reboot operations.
type realDBusConnection struct {
	sys   systemdConn
	login login1Conn
}

func (r *realDBusConnection) Close() {
	if r.login != nil {
		r.login.Close()
	}
	if r.sys != nil {
		r.sys.Close()
	}
}

func (r *realDBusConnection) ReloadContext(ctx context.Context) error {
	return r.sys.ReloadContext(ctx)
}

func (r *realDBusConnection) StartUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
	return r.sys.StartUnitContext(ctx, name, mode, ch)
}

func (r *realDBusConnection) StopUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
	return r.sys.StopUnitContext(ctx, name, mode, ch)
}

func (r *realDBusConnection) RestartUnitContext(ctx context.Context, name, mode string, ch chan<- string) (int, error) {
	return r.sys.RestartUnitContext(ctx, name, mode, ch)
}

func (r *realDBusConnection) EnableUnitFilesContext(ctx context.Context, files []string, runtime, force bool) (bool, []dbus.EnableUnitFileChange, error) {
	return r.sys.EnableUnitFilesContext(ctx, files, runtime, force)
}

func (r *realDBusConnection) DisableUnitFilesContext(ctx context.Context, files []string, runtime bool) ([]dbus.DisableUnitFileChange, error) {
	return r.sys.DisableUnitFilesContext(ctx, files, runtime)
}

func (r *realDBusConnection) GetUnitPropertiesContext(ctx context.Context, unit string) (map[string]any, error) {
	return r.sys.GetUnitPropertiesContext(ctx, unit)
}

// RebootContext delegates to login1.Conn.Reboot. login1's API does not
// accept a context, so we call it directly. The askForAuth flag is set to
// false to match the previous behavior of calling reboot without prompting.
func (r *realDBusConnection) RebootContext(ctx context.Context) error {
	if r.login == nil {
		return nil
	}
	// login1.Conn only exposes non-context methods; call directly.
	r.login.Reboot(false)
	return nil
}

// PowerOffContext delegates to login1.Conn.PowerOff.
func (r *realDBusConnection) PowerOffContext(ctx context.Context) error {
	if r.login == nil {
		return nil
	}
	r.login.PowerOff(false)
	return nil
}
