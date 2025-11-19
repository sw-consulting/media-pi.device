//go:build windows
// +build windows

package agent

import "os"

// On Windows we cannot rely on POSIX device ids. Provide a stub that
// indicates the check is not available.
func sameDevice(fi, pfi os.FileInfo) (bool, bool) {
	return false, false
}
