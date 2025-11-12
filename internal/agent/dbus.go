package agent

import (
	"context"
	"os"
	"sync"

	"github.com/coreos/go-systemd/v22/dbus"
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
	return dbus.NewWithContext(ctx)
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
