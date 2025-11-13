// Copyright (c) 2025 sw.consulting
// This file is a part of Media Pi device agent

package agent

import "os"

func init() {
	_ = os.Setenv("MEDIA_PI_AGENT_MOCK_DBUS", "1")
}
