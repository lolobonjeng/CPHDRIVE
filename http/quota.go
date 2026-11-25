package fbhttp

import (
	"context"
	"io"
	"math"
	"os"
	"strconv"
	"sync/atomic"

	"github.com/spf13/afero"

	fberrors "github.com/filebrowser/filebrowser/v2/errors"
	"github.com/filebrowser/filebrowser/v2/quota"
)

var quotaKeySeq uint64

func nextQuotaKey() string {
	return "req:" + strconv.FormatUint(atomic.AddUint64(&quotaKeySeq, 1), 10)
}

// diskPath is the root of the disk that holds the current user's home.
func (d *data) diskPath() (string, error) {
	if d.user.DiskID == 0 {
		return d.server.Root, nil
	}
	if d.store.Disks == nil {
		return "", fberrors.ErrDiskUnavailable
	}
	return d.store.Disks.Path(d.user.DiskID)
}

func (d *data) diskRootFor(id uint) (string, error) {
	if id == 0 {
		return d.server.Root, nil
	}
	if d.store.Disks == nil {
		return "", fberrors.ErrDiskUnavailable
	}
	return d.store.Disks.Path(id)
}

type writeGuard struct {
	allowed int64
	oldSize int64
	release func()
}

// beginWrite checks the user's quota and the disk's free space, then holds the
// expected growth. size is the incoming body length; pass unknown when the
// client did not declare one.
func (d *data) beginWrite(ctx context.Context, path string, size int64, unknown bool) (*writeGuard, error) {
	diskPath, err := d.diskPath()
	if err != nil {
		return nil, err
	}

	oldSize := int64(0)
	if info, statErr := d.user.Fs.Stat(path); statErr == nil && !info.IsDir() {
		oldSize = info.Size()
	}

	delta := size - oldSize
	if unknown {
		delta = -1
	} else if delta < 0 {
		delta = 0
	}

	allowed, release, err := d.store.Quota.Reserve(ctx, d.user, diskPath, nextQuotaKey(), delta)
	if err != nil {
		return nil, err
	}

	return &writeGuard{allowed: allowed, oldSize: oldSize, release: release}, nil
}

// maxWrite is the largest body the caller may write. Overwriting truncates the
// old file first, so its bytes are available in addition to the allowance.
func (g *writeGuard) maxWrite() int64 {
	if g.allowed > math.MaxInt64-g.oldSize {
		return math.MaxInt64
	}
	return g.allowed + g.oldSize
}

func (g *writeGuard) finish(m *quota.Manager, userID uint, written int64) {
	g.release()
	m.Add(userID, written-g.oldSize)
}

func (g *writeGuard) cancel() {
	g.release()
}

// countingReader reports how many bytes a write actually consumed, so a body
// that lies about its length can be caught and rolled back.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// treeSize sums the regular files under root in a user's filesystem.
func treeSize(fs afero.Fs, root string) int64 {
	var total int64
	_ = afero.Walk(fs, root, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total
}
