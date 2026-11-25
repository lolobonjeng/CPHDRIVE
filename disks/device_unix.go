//go:build !windows

package disks

import (
	"os"
	"syscall"
)

// SameDevice reports whether two entries live on the same filesystem, which is
// what makes a directory a mount point of its own disk. Entries that do not
// expose a device number are treated as being on the same filesystem, so a walk
// never prunes something it cannot classify.
func SameDevice(a, b os.FileInfo) bool {
	if a == nil || b == nil {
		return true
	}

	sa, ok := a.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	sb, ok := b.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}

	return sa.Dev == sb.Dev
}
