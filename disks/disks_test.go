package disks

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	fberrors "github.com/filebrowser/filebrowser/v2/errors"
)

func TestValidateRejectsRelativePath(t *testing.T) {
	err := Validate(&Disk{Path: "mnt/data"}, "", nil)
	if !errors.Is(err, fberrors.ErrDiskInvalid) {
		t.Fatalf("err = %v, want ErrDiskInvalid", err)
	}
}

func TestValidateRejectsMissingPath(t *testing.T) {
	err := Validate(&Disk{Path: filepath.Join(t.TempDir(), "nope")}, "", nil)
	if !errors.Is(err, fberrors.ErrDiskInvalid) {
		t.Fatalf("err = %v, want ErrDiskInvalid", err)
	}
}

func TestValidateRejectsServerRoot(t *testing.T) {
	root := t.TempDir()
	err := Validate(&Disk{Path: root}, root, nil)
	if !errors.Is(err, fberrors.ErrDiskInvalid) {
		t.Fatalf("err = %v, want ErrDiskInvalid", err)
	}
}

func TestValidateRejectsDiskInsideServedRoot(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "disk")
	if err := os.Mkdir(inside, 0750); err != nil {
		t.Fatal(err)
	}

	err := Validate(&Disk{Path: inside}, root, nil)
	if !errors.Is(err, fberrors.ErrDiskInvalid) {
		t.Fatalf("err = %v, want ErrDiskInvalid", err)
	}
}

func TestValidateRejectsNestedDisks(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	if err := os.Mkdir(child, 0750); err != nil {
		t.Fatal(err)
	}

	err := Validate(&Disk{Path: child}, "", []*Disk{{ID: 1, Name: "parent", Path: parent}})
	if !errors.Is(err, fberrors.ErrDiskInvalid) {
		t.Fatalf("err = %v, want ErrDiskInvalid", err)
	}
}

func TestValidateRejectsDuplicatePath(t *testing.T) {
	dir := t.TempDir()
	err := Validate(&Disk{Path: dir}, "", []*Disk{{ID: 1, Name: "one", Path: dir}})
	if !errors.Is(err, fberrors.ErrDiskInvalid) {
		t.Fatalf("err = %v, want ErrDiskInvalid", err)
	}
}

func TestValidateAcceptsAndNormalizes(t *testing.T) {
	dir := t.TempDir()
	d := &Disk{Path: dir + string(filepath.Separator)}

	if err := Validate(d, "", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resolved, err := filepath.EvalSymlinks(filepath.Clean(dir))
	if err != nil {
		t.Fatal(err)
	}
	if d.Path != filepath.Clean(resolved) {
		t.Fatalf("path = %q, want %q", d.Path, filepath.Clean(resolved))
	}
	if d.Name == "" {
		t.Fatal("name was not derived from the path")
	}
}

func TestSafetyReserveStaysBounded(t *testing.T) {
	if got := SafetyReserve(0); got != 256<<20 {
		t.Fatalf("reserve(0) = %d", got)
	}
	if got := SafetyReserve(500 << 30); got != 2<<30 {
		t.Fatalf("reserve(500GiB) = %d, want the 2GiB cap", got)
	}
}
