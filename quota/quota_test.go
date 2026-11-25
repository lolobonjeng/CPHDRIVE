package quota

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/afero"

	fberrors "github.com/filebrowser/filebrowser/v2/errors"
	"github.com/filebrowser/filebrowser/v2/users"
)

func testUser(t *testing.T, id uint, quota int64) *users.User {
	t.Helper()

	fs := afero.NewMemMapFs()
	if err := fs.MkdirAll("/docs", 0750); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/docs/a.txt", make([]byte, 40), 0640); err != nil {
		t.Fatal(err)
	}
	if err := afero.WriteFile(fs, "/b.txt", make([]byte, 60), 0640); err != nil {
		t.Fatal(err)
	}

	return &users.User{ID: id, Fs: fs, Quota: quota}
}

func TestUsedSumsRegularFiles(t *testing.T) {
	m := New()
	u := testUser(t, 1, 0)

	used, err := m.Used(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	if used != 100 {
		t.Fatalf("used = %d, want 100", used)
	}
}

func TestUsedIsCachedUntilInvalidated(t *testing.T) {
	m := New()
	u := testUser(t, 1, 0)

	if _, err := m.Used(context.Background(), u); err != nil {
		t.Fatal(err)
	}

	if err := afero.WriteFile(u.Fs, "/c.txt", make([]byte, 25), 0640); err != nil {
		t.Fatal(err)
	}

	used, err := m.Used(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	if used != 100 {
		t.Fatalf("cached used = %d, want 100", used)
	}

	m.Invalidate(u.ID)
	used, err = m.Used(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	if used != 125 {
		t.Fatalf("fresh used = %d, want 125", used)
	}
}

func TestReserveRefusesOverQuota(t *testing.T) {
	m := New()
	u := testUser(t, 1, 100)

	if _, _, err := m.Reserve(context.Background(), u, "", "a", 1); err == nil {
		t.Fatal("expected the quota to reject a write when it is already full")
	} else if !errors.Is(err, fberrors.ErrQuotaExceeded) {
		t.Fatalf("err = %v, want ErrQuotaExceeded", err)
	}
}

func TestReservationsCannotBothSpendTheSameBytes(t *testing.T) {
	m := New()
	u := testUser(t, 1, 150)

	if _, release, err := m.Reserve(context.Background(), u, "", "a", 40); err != nil {
		t.Fatalf("first reservation: %v", err)
	} else {
		defer release()
	}

	if _, _, err := m.Reserve(context.Background(), u, "", "b", 20); err == nil {
		t.Fatal("second reservation should not fit next to the first")
	}

	m.Release(1, "a")

	if _, release, err := m.Reserve(context.Background(), u, "", "b", 20); err != nil {
		t.Fatalf("reservation after release: %v", err)
	} else {
		release()
	}
}

func TestAddAdjustsCachedUsage(t *testing.T) {
	m := New()
	u := testUser(t, 1, 0)

	if _, err := m.Used(context.Background(), u); err != nil {
		t.Fatal(err)
	}

	m.Add(u.ID, 50)
	used, err := m.Used(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	if used != 150 {
		t.Fatalf("used = %d, want 150", used)
	}

	m.Add(u.ID, -1000)
	used, err = m.Used(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	if used != 0 {
		t.Fatalf("used = %d, want 0 after a negative adjustment", used)
	}
}

func TestUnknownSizeReservesTheRemainingAllowance(t *testing.T) {
	m := New()
	u := testUser(t, 1, 130)

	allowed, release, err := m.Reserve(context.Background(), u, "", "a", -1)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	if allowed != 30 {
		t.Fatalf("allowed = %d, want 30", allowed)
	}

	if _, _, err := m.Reserve(context.Background(), u, "", "b", 1); err == nil {
		t.Fatal("the unknown-size reservation should hold the whole allowance")
	}
}

func TestMovingBlocksWrites(t *testing.T) {
	m := New()
	u := testUser(t, 1, 0)

	m.SetMoving(u.ID, true)
	if _, _, err := m.Reserve(context.Background(), u, "", "a", 1); !errors.Is(err, fberrors.ErrUserMoving) {
		t.Fatalf("err = %v, want ErrUserMoving", err)
	}

	m.SetMoving(u.ID, false)
	if _, release, err := m.Reserve(context.Background(), u, "", "a", 1); err != nil {
		t.Fatalf("reservation after move: %v", err)
	} else {
		release()
	}
}
