package tests

import "os"

func init() {
	_ = os.Setenv("MEDIA_PI_AGENT_MOCK_DBUS", "1")
}
