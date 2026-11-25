package disks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBrowseListsDirectoriesOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "alpha"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "beta"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".hidden"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}

	res, err := Browse(context.Background(), dir, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Entries) != 2 {
		t.Fatalf("entries = %d, want 2: %+v", len(res.Entries), res.Entries)
	}
	if res.Entries[0].Name != "alpha" || res.Entries[1].Name != "beta" {
		t.Fatalf("entries are not sorted: %+v", res.Entries)
	}
	if !res.Entries[0].Selectable {
		t.Fatalf("alpha should be selectable: %+v", res.Entries[0])
	}
}

func TestBrowseRefusesRelativePath(t *testing.T) {
	if _, err := Browse(context.Background(), "mnt/data", "", nil); err == nil {
		t.Fatal("a relative path should be refused")
	}
}

func TestBrowseMarksTheServedRoot(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0750); err != nil {
		t.Fatal(err)
	}

	res, err := Browse(context.Background(), root, root, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Selectable || res.Reason != ReasonServerRoot {
		t.Fatalf("served root: selectable=%v reason=%q", res.Selectable, res.Reason)
	}

	// A plain subdirectory of the served root is not a mount point, so it
	// cannot become a disk.
	if len(res.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(res.Entries))
	}
	if res.Entries[0].Selectable || res.Entries[0].Reason != ReasonNotMount {
		t.Fatalf("child: selectable=%v reason=%q", res.Entries[0].Selectable, res.Entries[0].Reason)
	}
}

func TestBrowseMarksRegisteredDisk(t *testing.T) {
	base := t.TempDir()
	registered := filepath.Join(base, "disk2")
	if err := os.Mkdir(registered, 0750); err != nil {
		t.Fatal(err)
	}

	res, err := Browse(context.Background(), base, "", []*Disk{{ID: 1, Name: "disk2", Path: registered}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(res.Entries))
	}
	if res.Entries[0].Selectable || res.Entries[0].Reason != ReasonRegistered {
		t.Fatalf("registered: selectable=%v reason=%q", res.Entries[0].Selectable, res.Entries[0].Reason)
	}
}

func TestBrowseParentStopsAtTheRoot(t *testing.T) {
	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)

	res, err := Browse(context.Background(), root, "", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Parent != "" {
		t.Fatalf("parent = %q, want empty", res.Parent)
	}
	if res.Selectable || res.Reason != ReasonRoot {
		t.Fatalf("root: selectable=%v reason=%q", res.Selectable, res.Reason)
	}
}
