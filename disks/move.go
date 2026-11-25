package disks

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// MoveProgress is a snapshot of a user move.
type MoveProgress struct {
	Total  int64
	Copied int64
	Path   string
}

// MoveTree copies src into dst, verifies every entry, and leaves src untouched.
// The caller switches the user over only after this returns nil, so a failure
// at any point leaves the original home intact.
//
// dst must not exist or must be an empty directory. freeCheck, when set, is
// called periodically during the copy and should return an error if the target
// disk is about to run out of room.
func MoveTree(ctx context.Context, src, dst string, progress func(MoveProgress), freeCheck func() error) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("source is not a directory: %s", src)
	}

	if info, statErr := os.Stat(dst); statErr == nil {
		if !info.IsDir() {
			return fmt.Errorf("destination exists and is not a directory: %s", dst)
		}
		entries, readErr := os.ReadDir(dst)
		if readErr != nil {
			return readErr
		}
		if len(entries) > 0 {
			return fmt.Errorf("destination is not empty: %s", dst)
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}

	if err := os.MkdirAll(dst, srcInfo.Mode().Perm()); err != nil {
		return err
	}

	total, err := DirSize(src)
	if err != nil {
		return err
	}

	var copied int64
	var lastCheck int64

	err = walkSameDevice(src, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		rel, relErr := filepath.Rel(src, p)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)

		switch {
		case info.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			link, linkErr := os.Readlink(p)
			if linkErr != nil {
				return linkErr
			}
			return os.Symlink(link, target)
		case !info.Mode().IsRegular():
			// Sockets, devices and pipes have no content to copy.
			return nil
		}

		if copied-lastCheck >= 64<<20 && freeCheck != nil {
			if err := freeCheck(); err != nil {
				return err
			}
			lastCheck = copied
		}

		if err := copyFile(p, target, info); err != nil {
			return err
		}

		copied += info.Size()
		if progress != nil {
			progress(MoveProgress{Total: total, Copied: copied, Path: rel})
		}
		return nil
	})
	if err != nil {
		return err
	}

	return VerifyTree(src, dst)
}

// VerifyTree checks that every source entry exists at the destination with the
// same type and, for regular files, the same size. It skips the same entries
// MoveTree skipped, so a mount inside the home does not fail the check.
func VerifyTree(src, dst string) error {
	return walkSameDevice(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(src, p)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)

		targetInfo, statErr := os.Lstat(target)
		if statErr != nil {
			return fmt.Errorf("verify %s: %w", rel, statErr)
		}

		if info.IsDir() != targetInfo.IsDir() {
			return fmt.Errorf("verify %s: type mismatch", rel)
		}
		if info.Mode().IsRegular() && info.Size() != targetInfo.Size() {
			return fmt.Errorf("verify %s: size %d != %d", rel, targetInfo.Size(), info.Size())
		}

		return nil
	})
}

func copyFile(src, dst string, info os.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	return os.Chtimes(dst, info.ModTime(), info.ModTime())
}
