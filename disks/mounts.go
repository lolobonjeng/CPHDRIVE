package disks

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	gopsutil "github.com/shirou/gopsutil/v4/disk"
)

// Mount is one entry of the operating system's mount table.
type Mount struct {
	Path     string
	Device   string
	FSType   string
	ReadOnly bool
}

// Mounts returns the mount table. Entries without an absolute mount point are
// dropped; callers decide which filesystem types are interesting.
func Mounts(ctx context.Context) ([]Mount, error) {
	parts, err := gopsutil.PartitionsWithContext(ctx, true)
	if err != nil {
		return nil, err
	}

	out := make([]Mount, 0, len(parts))
	for _, p := range parts {
		path := strings.TrimSpace(p.Mountpoint)
		if path == "" || !filepath.IsAbs(path) {
			continue
		}
		out = append(out, Mount{
			Path:     filepath.Clean(path),
			Device:   p.Device,
			FSType:   p.Fstype,
			ReadOnly: !mountWritable(p.Opts),
		})
	}

	return out, nil
}

// mountAt returns the mount entry that backs path, if path is a mount point
// itself. A path inside a mount is not a mount point.
func mountAt(mounts []Mount, path string) (Mount, bool) {
	for _, m := range mounts {
		if m.Path == path {
			return m, true
		}
	}
	return Mount{}, false
}

// IsMountPoint reports whether path is the mount point of its own filesystem.
// A disk may live inside the served root only when it is one: the bytes then
// belong to a different device, so quota accounting and the safety reserve stay
// honest.
func IsMountPoint(ctx context.Context, path string) bool {
	if runtime.GOOS == "windows" {
		return false
	}

	mounts, err := Mounts(ctx)
	if err != nil {
		return false
	}
	_, ok := mountAt(mounts, filepath.Clean(path))
	return ok
}

// walkSameDevice walks root and skips every directory that belongs to a
// different filesystem, so a walk never crosses into another disk. It is the
// equivalent of find -xdev.
func walkSameDevice(root string, fn filepath.WalkFunc) error {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}

	return filepath.Walk(root, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr == nil && p != root && info != nil && info.IsDir() && !SameDevice(rootInfo, info) {
			return filepath.SkipDir
		}
		return fn(p, info, walkErr)
	})
}
