package disks

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	fberrors "github.com/filebrowser/filebrowser/v2/errors"
)

// Reason codes explain why a directory cannot become a disk. The client
// translates them, so the wording stays in one place.
const (
	ReasonRoot       = "root"
	ReasonServerRoot = "server-root"
	ReasonNotMount   = "not-mount"
	ReasonRegistered = "registered"
	ReasonNested     = "nested"
)

// BrowseEntry is one directory in the disk path picker.
type BrowseEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Mount      bool   `json:"mount"`
	ReadOnly   bool   `json:"readOnly"`
	Selectable bool   `json:"selectable"`
	Reason     string `json:"reason,omitempty"`
	Total      uint64 `json:"total,omitempty"`
	Free       uint64 `json:"free,omitempty"`
}

// BrowseResult is one level of the picker.
type BrowseResult struct {
	Path       string        `json:"path"`
	Parent     string        `json:"parent"`
	Selectable bool          `json:"selectable"`
	Reason     string        `json:"reason,omitempty"`
	Entries    []BrowseEntry `json:"entries"`
}

// Browse lists the directories under path for the admin's disk picker. It
// returns directories only, never file contents, and skips dot-directories.
//
// The listing is read-only. Writability is still verified by Validate when the
// admin submits, so a read-only mount is reported but not probed.
func Browse(ctx context.Context, path, serverRoot string, existing []*Disk) (*BrowseResult, error) {
	if strings.TrimSpace(path) == "" {
		// Start at the root of the volume the process runs on: "/" on Unix,
		// "C:\" on Windows.
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", fberrors.ErrDiskInvalid, err)
		}
		path = filepath.VolumeName(wd) + string(filepath.Separator)
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("%w: path must be absolute", fberrors.ErrDiskInvalid)
	}

	path = filepath.Clean(path)

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", fberrors.ErrDiskInvalid, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: path is not a directory", fberrors.ErrDiskInvalid)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", fberrors.ErrDiskInvalid, err)
	}

	mounts, _ := Mounts(ctx)

	root := ""
	if serverRoot != "" {
		root = filepath.Clean(serverRoot)
		if r, evalErr := filepath.EvalSymlinks(root); evalErr == nil {
			root = filepath.Clean(r)
		}
	}

	res := &BrowseResult{Path: path, Parent: parentOf(path), Entries: []BrowseEntry{}}
	res.Selectable, res.Reason = selectable(path, root, existing, mounts)

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}

		child := filepath.Join(path, e.Name())
		entry := BrowseEntry{Name: e.Name(), Path: child}

		if m, ok := mountAt(mounts, child); ok {
			entry.Mount = true
			entry.ReadOnly = m.ReadOnly
			if !m.ReadOnly {
				if u, usageErr := Usage(ctx, child); usageErr == nil {
					entry.Total = u.Total
					entry.Free = u.Free
				}
			}
		}

		entry.Selectable, entry.Reason = selectable(child, root, existing, mounts)
		res.Entries = append(res.Entries, entry)
	}

	sort.Slice(res.Entries, func(i, j int) bool {
		return strings.ToLower(res.Entries[i].Name) < strings.ToLower(res.Entries[j].Name)
	})

	return res, nil
}

// selectable reports whether path may become a disk, and why not when it may
// not. It mirrors Validate without the write probe.
func selectable(path, serverRoot string, existing []*Disk, mounts []Mount) (bool, string) {
	// filepath.Dir reports the path itself for a filesystem root, which covers
	// "/" on Unix and "C:\" on Windows.
	if filepath.Dir(path) == path {
		return false, ReasonRoot
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false, ReasonNotMount
	}
	resolved = filepath.Clean(resolved)

	if serverRoot != "" {
		if samePath(serverRoot, resolved) || isSubpath(resolved, serverRoot) {
			return false, ReasonServerRoot
		}
		if isSubpath(serverRoot, resolved) {
			if _, ok := mountAt(mounts, resolved); !ok {
				return false, ReasonNotMount
			}
		}
	}

	for _, d := range existing {
		other := filepath.Clean(d.Path)
		if r, evalErr := filepath.EvalSymlinks(other); evalErr == nil {
			other = filepath.Clean(r)
		}

		if samePath(other, resolved) {
			return false, ReasonRegistered
		}
		if isSubpath(other, resolved) || isSubpath(resolved, other) {
			return false, ReasonNested
		}
	}

	return true, ""
}

func parentOf(path string) string {
	parent := filepath.Dir(path)
	if parent == path {
		return ""
	}
	return parent
}
