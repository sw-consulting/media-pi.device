//go:build !windows
// +build !windows

package agent

import (
	"os"
	"syscall"
)

// sameDevice checks whether two FileInfo values refer to the same device.
// Returns (available, same) where available indicates whether the check
// could be performed on this platform.
func sameDevice(fi, pfi os.FileInfo) (bool, bool) {
	st, ok1 := fi.Sys().(*syscall.Stat_t)
	pst, ok2 := pfi.Sys().(*syscall.Stat_t)
	if !ok1 || !ok2 {
		return false, false
	}
	return true, st.Dev == pst.Dev
}
