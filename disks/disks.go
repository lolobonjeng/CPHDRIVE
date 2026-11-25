// Package disks manages the physical storage locations a user's home can live
// on. Disk 0 is virtual: it is the server root and always exists, so a database
// that predates this feature keeps working without a migration.
package disks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	gopsutil "github.com/shirou/gopsutil/v4/disk"

	fberrors "github.com/filebrowser/filebrowser/v2/errors"
)

// SystemDiskID is the virtual disk that points at the server root.
const SystemDiskID uint = 0

// Disk is a registered storage location.
type Disk struct {
	ID        uint      `storm:"id,increment" json:"id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
}

// UsageInfo is the capacity of the filesystem that holds a path.
type UsageInfo struct {
	Total uint64
	Used  uint64
	Free  uint64
}

// Backend is the persistence contract for disks.
type Backend interface {
	GetByID(uint) (*Disk, error)
	Gets() ([]*Disk, error)
	Save(*Disk) error
	Update(*Disk) error
	DeleteByID(uint) error
}

// Storage is a disks storage.
type Storage struct {
	back Backend
}

// NewStorage creates a disks storage from a backend.
func NewStorage(back Backend) *Storage {
	return &Storage{back: back}
}

// Get returns a disk by ID.
func (s *Storage) Get(id uint) (*Disk, error) {
	return s.back.GetByID(id)
}

// Gets returns every registered disk.
func (s *Storage) Gets() ([]*Disk, error) {
	return s.back.Gets()
}

// Save stores a new disk.
func (s *Storage) Save(d *Disk) error {
	return s.back.Save(d)
}

// Update stores an existing disk.
func (s *Storage) Update(d *Disk) error {
	return s.back.Update(d)
}

// Delete removes a disk.
func (s *Storage) Delete(id uint) error {
	return s.back.DeleteByID(id)
}

// Path resolves a disk ID to its root, refusing a disk whose mount is gone.
// It implements the users.DiskProvider contract.
func (s *Storage) Path(id uint) (string, error) {
	d, err := s.back.GetByID(id)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(d.Path)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: %s", fberrors.ErrDiskUnavailable, d.Path)
	}

	return d.Path, nil
}

// Validate normalizes a disk and refuses paths that would make quotas or
// scopes ambiguous. It mutates Name and Path to their normalized form.
func Validate(ctx context.Context, d *Disk, serverRoot string, others []*Disk) error {
	d.Name = strings.TrimSpace(d.Name)
	d.Path = strings.TrimSpace(d.Path)

	if d.Path == "" {
		return fmt.Errorf("%w: path is empty", fberrors.ErrDiskInvalid)
	}
	if !filepath.IsAbs(d.Path) {
		return fmt.Errorf("%w: path must be absolute", fberrors.ErrDiskInvalid)
	}

	resolved, err := filepath.EvalSymlinks(filepath.Clean(d.Path))
	if err != nil {
		return fmt.Errorf("%w: %v", fberrors.ErrDiskInvalid, err)
	}
	resolved = filepath.Clean(resolved)

	if resolved == string(filepath.Separator) {
		return fmt.Errorf("%w: the filesystem root cannot be a disk", fberrors.ErrDiskInvalid)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return fmt.Errorf("%w: %v", fberrors.ErrDiskInvalid, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: path is not a directory", fberrors.ErrDiskInvalid)
	}

	probe, err := os.CreateTemp(resolved, ".cphdrive-write-test-*")
	if err != nil {
		return fmt.Errorf("%w: directory is not writable: %v", fberrors.ErrDiskInvalid, err)
	}
	probeName := probe.Name()
	_ = probe.Close()
	_ = os.Remove(probeName)

	if d.Name == "" {
		d.Name = filepath.Base(resolved)
	}

	if serverRoot != "" {
		root := filepath.Clean(serverRoot)
		if r, evalErr := filepath.EvalSymlinks(root); evalErr == nil {
			root = filepath.Clean(r)
		}

		if samePath(root, resolved) {
			return fmt.Errorf("%w: the server root is the system disk", fberrors.ErrDiskInvalid)
		}
		if isSubpath(resolved, root) {
			return fmt.Errorf("%w: the served root is inside this disk", fberrors.ErrDiskInvalid)
		}
		// A mount point inside the served root is a separate device, so it can
		// be a disk of its own: the external drive mounted at /root/External is
		// the common case. A plain subdirectory cannot, because its bytes are
		// already counted by the system disk and one tenant's home would sit
		// inside another tenant's view.
		if isSubpath(root, resolved) && !IsMountPoint(ctx, resolved) {
			return fmt.Errorf("%w: %s is inside the served root but is not a mount point", fberrors.ErrDiskInvalid, resolved)
		}
	}

	for _, other := range others {
		if other.ID == d.ID {
			continue
		}

		otherPath := filepath.Clean(other.Path)
		if r, evalErr := filepath.EvalSymlinks(otherPath); evalErr == nil {
			otherPath = filepath.Clean(r)
		}

		switch {
		case samePath(otherPath, resolved):
			return fmt.Errorf("%w: %s is already registered as %q", fberrors.ErrDiskInvalid, resolved, other.Name)
		case isSubpath(otherPath, resolved):
			return fmt.Errorf("%w: %s is inside disk %q", fberrors.ErrDiskInvalid, resolved, other.Name)
		case isSubpath(resolved, otherPath):
			return fmt.Errorf("%w: %s contains disk %q", fberrors.ErrDiskInvalid, resolved, other.Name)
		}
	}

	d.Path = resolved
	return nil
}

// Candidate is a mounted filesystem that could become a disk.
type Candidate struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	FSType string `json:"fstype"`
	Total  uint64 `json:"total"`
	Free   uint64 `json:"free"`
}

var skipFSTypes = map[string]struct{}{
	"autofs": {}, "binfmt_misc": {}, "bpf": {}, "cgroup": {}, "cgroup2": {},
	"configfs": {}, "debugfs": {}, "devpts": {}, "devtmpfs": {}, "fusectl": {},
	"hugetlbfs": {}, "mqueue": {}, "nsfs": {}, "overlay": {}, "proc": {},
	"pstore": {}, "ramfs": {}, "rpc_pipefs": {}, "securityfs": {}, "squashfs": {},
	"sysfs": {}, "tmpfs": {}, "tracefs": {},
}

var skipMountPrefixes = []string{
	"/proc", "/sys", "/dev", "/run", "/snap", "/boot", "/var/lib/docker",
}

// Detect lists mounted filesystems that are not already registered and that
// could safely hold a user's home. It is best-effort: a platform that does not
// expose partitions simply returns an empty list.
func Detect(ctx context.Context, serverRoot string, existing []*Disk) []Candidate {
	mounts, err := Mounts(ctx)
	if err != nil {
		return []Candidate{}
	}

	root := filepath.Clean(serverRoot)
	if r, evalErr := filepath.EvalSymlinks(root); evalErr == nil {
		root = filepath.Clean(r)
	}

	seen := map[string]struct{}{}
	out := make([]Candidate, 0, len(mounts))

	for _, m := range mounts {
		if _, skip := skipFSTypes[strings.ToLower(m.FSType)]; skip {
			continue
		}
		if m.ReadOnly {
			continue
		}
		if hasSkippedPrefix(m.Path) {
			continue
		}

		resolved, evalErr := filepath.EvalSymlinks(m.Path)
		if evalErr != nil {
			continue
		}
		resolved = filepath.Clean(resolved)

		if _, dup := seen[resolved]; dup {
			continue
		}

		// A mount that holds the served root, or is the served root itself,
		// cannot be a disk: its bytes would overlap the system disk. A mount
		// inside the served root can, because it is a different device.
		if serverRoot != "" && (samePath(root, resolved) || isSubpath(resolved, root)) {
			continue
		}
		if conflictsWithDisk(resolved, existing) {
			continue
		}

		info, statErr := os.Stat(resolved)
		if statErr != nil || !info.IsDir() {
			continue
		}

		u, usageErr := Usage(ctx, resolved)
		if usageErr != nil {
			continue
		}

		seen[resolved] = struct{}{}
		name := filepath.Base(resolved)
		if name == "." || name == string(filepath.Separator) {
			name = resolved
		}
		out = append(out, Candidate{
			Name:   name,
			Path:   resolved,
			FSType: m.FSType,
			Total:  u.Total,
			Free:   u.Free,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Usage returns the capacity of the filesystem that holds path.
func Usage(ctx context.Context, path string) (UsageInfo, error) {
	u, err := gopsutil.UsageWithContext(ctx, path)
	if err != nil {
		return UsageInfo{}, err
	}

	return UsageInfo{Total: u.Total, Used: u.Used, Free: u.Free}, nil
}

// SafetyReserve is the slice of a disk the app refuses to fill, so the
// operating system keeps room to write logs and finish its own work.
func SafetyReserve(total uint64) int64 {
	reserve := int64(total / 100) // 1%
	if reserve < 256<<20 {
		reserve = 256 << 20
	}
	if reserve > 2<<30 {
		reserve = 2 << 30
	}
	return reserve
}

// DirSize sums the regular files under root. It does not cross into another
// filesystem, so a mount that happens to sit inside root is not counted.
func DirSize(root string) (int64, error) {
	var total int64
	err := walkSameDevice(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func mountWritable(opts []string) bool {
	for _, opt := range opts {
		if strings.TrimSpace(opt) == "ro" {
			return false
		}
	}
	return true
}

func hasSkippedPrefix(mount string) bool {
	for _, prefix := range skipMountPrefixes {
		if mount == prefix || isSubpath(prefix, mount) {
			return true
		}
	}
	return false
}

func conflictsWithDisk(path string, existing []*Disk) bool {
	for _, d := range existing {
		other := filepath.Clean(d.Path)
		if r, err := filepath.EvalSymlinks(other); err == nil {
			other = filepath.Clean(r)
		}
		if samePath(other, path) || isSubpath(other, path) || isSubpath(path, other) {
			return true
		}
	}
	return false
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// isSubpath reports whether child is at or below parent.
func isSubpath(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// IsNotExist reports whether err means a disk row is missing.
func IsNotExist(err error) bool {
	return errors.Is(err, fberrors.ErrNotExist)
}
