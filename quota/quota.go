// Package quota enforces per-user storage limits and the real free space of
// the disk that holds a user's home.
package quota

import (
	"context"
	"fmt"
	"math"
	"os"
	"sync"
	"time"

	"github.com/spf13/afero"

	"github.com/filebrowser/filebrowser/v2/disks"
	fberrors "github.com/filebrowser/filebrowser/v2/errors"
	"github.com/filebrowser/filebrowser/v2/users"
)

const cacheTTL = 10 * time.Second

type entry struct {
	used int64
	at   time.Time
}

// Manager tracks how much each user stores and what is currently being
// written. The usage number is a cached directory walk; reservations cover the
// gap between the check and the end of the write, so two uploads cannot both
// spend the same free bytes.
type Manager struct {
	mu       sync.Mutex
	entries  map[uint]*entry
	reserved map[uint]map[string]int64
	moving   map[uint]bool
}

// New creates a quota manager.
func New() *Manager {
	return &Manager{
		entries:  map[uint]*entry{},
		reserved: map[uint]map[string]int64{},
		moving:   map[uint]bool{},
	}
}

// Used returns the bytes stored in the user's home. The value is cached for a
// short window because every write has to consult it.
func (m *Manager) Used(ctx context.Context, u *users.User) (int64, error) {
	if u.Fs == nil {
		return 0, fberrors.ErrDiskUnavailable
	}

	m.mu.Lock()
	if e, ok := m.entries[u.ID]; ok && time.Since(e.at) < cacheTTL {
		used := e.used
		m.mu.Unlock()
		return used, nil
	}
	m.mu.Unlock()

	used, err := walkUsed(ctx, u.Fs)
	if err != nil {
		return 0, err
	}

	m.mu.Lock()
	m.entries[u.ID] = &entry{used: used, at: time.Now()}
	m.mu.Unlock()

	return used, nil
}

// Reserve checks the user quota and the disk's free space, then holds delta
// bytes for key. Pass a negative delta when the size is unknown: the whole
// remaining allowance is held, and returned as the number of bytes the caller
// may write.
//
// The returned release function is idempotent and must always be called.
func (m *Manager) Reserve(ctx context.Context, u *users.User, diskPath, key string, delta int64) (int64, func(), error) {
	if m.IsMoving(u.ID) {
		return 0, nil, fberrors.ErrUserMoving
	}

	used, err := m.Used(ctx, u)
	if err != nil {
		return 0, nil, err
	}

	diskFree, err := freeSpace(ctx, diskPath)
	if err != nil {
		return 0, nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	reserved := int64(0)
	if rs, ok := m.reserved[u.ID]; ok {
		for _, v := range rs {
			reserved += v
		}
	}

	allowed := int64(math.MaxInt64)
	limitErr := error(fberrors.ErrDiskFull)

	if u.Quota > 0 {
		remaining := u.Quota - used - reserved
		if remaining < 0 {
			remaining = 0
		}
		allowed = remaining
		limitErr = fberrors.ErrQuotaExceeded
	}

	if diskFree < allowed {
		allowed = diskFree
		limitErr = fberrors.ErrDiskFull
	}

	want := delta
	if want < 0 {
		want = allowed
	}
	if want < 0 {
		want = 0
	}
	if want > allowed {
		return allowed, nil, fmt.Errorf("%w: need %d bytes, %d available", limitErr, want, allowed)
	}

	if m.reserved[u.ID] == nil {
		m.reserved[u.ID] = map[string]int64{}
	}
	m.reserved[u.ID][key] = want

	released := false
	release := func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if released {
			return
		}
		released = true
		if rs, ok := m.reserved[u.ID]; ok {
			delete(rs, key)
			if len(rs) == 0 {
				delete(m.reserved, u.ID)
			}
		}
	}

	return allowed, release, nil
}

// HasReservation reports whether key is already holding bytes for the user.
func (m *Manager) HasReservation(id uint, key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	rs, ok := m.reserved[id]
	if !ok {
		return false
	}
	_, ok = rs[key]
	return ok
}

// Release drops a named reservation, used when an upload is deleted or evicted.
func (m *Manager) Release(id uint, key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if rs, ok := m.reserved[id]; ok {
		delete(rs, key)
		if len(rs) == 0 {
			delete(m.reserved, id)
		}
	}
}

// Add adjusts the cached usage after a write, without waiting for the cache to
// expire. Negative values are clamped at zero.
func (m *Manager) Add(id uint, delta int64) {
	if delta == 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.entries[id]; ok {
		e.used += delta
		if e.used < 0 {
			e.used = 0
		}
	}
}

// Invalidate drops the cached usage so the next check walks the tree again.
func (m *Manager) Invalidate(id uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, id)
}

// SetMoving marks a user as being migrated. Writes are refused while the copy
// runs, so the destination cannot miss a file that arrived mid-move.
func (m *Manager) SetMoving(id uint, moving bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if moving {
		m.moving[id] = true
		return
	}
	delete(m.moving, id)
}

// IsMoving reports whether a migration is running for the user.
func (m *Manager) IsMoving(id uint) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.moving[id]
}

func freeSpace(ctx context.Context, path string) (int64, error) {
	if path == "" {
		return math.MaxInt64, nil
	}

	u, err := disks.Usage(ctx, path)
	if err != nil {
		return 0, err
	}

	free := int64(u.Free) - disks.SafetyReserve(u.Total)
	if free < 0 {
		free = 0
	}
	return free, nil
}

func walkUsed(ctx context.Context, fs afero.Fs) (int64, error) {
	var total int64

	err := afero.Walk(fs, "/", func(p string, info os.FileInfo, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			// An unreadable subdirectory should not hide the rest of the
			// account. Only a root that cannot be read is fatal.
			if p == "/" || p == "." {
				return err
			}
			return nil
		}
		if info == nil {
			return nil
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	return total, nil
}
