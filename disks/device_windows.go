//go:build windows

package disks

import "os"

// SameDevice reports whether two entries live on the same filesystem. Windows
// does not expose a device number through os.FileInfo, so every path is treated
// as being on the same volume and the walks keep their previous behaviour.
func SameDevice(_, _ os.FileInfo) bool { return true }
