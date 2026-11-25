package fbhttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"github.com/filebrowser/filebrowser/v2/disks"
	fberrors "github.com/filebrowser/filebrowser/v2/errors"
)

// usageBudget bounds how long the users table waits for its usage numbers. A
// user whose home is slow to walk comes back without a number instead of
// blocking the whole page.
const usageBudget = 20 * time.Second

type diskInfo struct {
	ID            uint      `json:"id"`
	Name          string    `json:"name"`
	Path          string    `json:"path"`
	CreatedAt     time.Time `json:"createdAt"`
	Virtual       bool      `json:"virtual"`
	Online        bool      `json:"online"`
	Total         uint64    `json:"total"`
	Used          uint64    `json:"used"`
	Free          uint64    `json:"free"`
	UserCount     int       `json:"userCount"`
	SafetyReserve uint64    `json:"safetyReserve"`
}

func buildDiskInfo(ctx context.Context, d *disks.Disk, virtual bool, userCount int) diskInfo {
	info := diskInfo{
		ID:        d.ID,
		Name:      d.Name,
		Path:      d.Path,
		CreatedAt: d.CreatedAt,
		Virtual:   virtual,
		UserCount: userCount,
	}

	if _, err := os.Stat(d.Path); err != nil {
		return info
	}

	usage, err := disks.Usage(ctx, d.Path)
	if err != nil {
		return info
	}

	info.Online = true
	info.Total = usage.Total
	info.Used = usage.Used
	info.Free = usage.Free
	info.SafetyReserve = uint64(disks.SafetyReserve(usage.Total))
	return info
}

func diskUserCounts(d *data) (map[uint]int, error) {
	all, err := d.store.Users.Gets(d.server.Root, d.server.FollowExternalSymlinks)
	if err != nil {
		return nil, err
	}

	counts := map[uint]int{}
	for _, u := range all {
		counts[u.DiskID]++
	}
	return counts, nil
}

var disksGetHandler = withAdmin(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	stored, err := d.store.Disks.Gets()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	counts, err := diskUserCounts(d)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	out := []diskInfo{
		buildDiskInfo(r.Context(), &disks.Disk{ID: disks.SystemDiskID, Name: "System", Path: d.server.Root}, true, counts[disks.SystemDiskID]),
	}
	for _, disk := range stored {
		out = append(out, buildDiskInfo(r.Context(), disk, false, counts[disk.ID]))
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return renderJSON(w, r, out)
})

var diskPostHandler = withAdmin(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	var body struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return http.StatusBadRequest, err
	}

	existing, err := d.store.Disks.Gets()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	disk := &disks.Disk{Name: body.Name, Path: body.Path, CreatedAt: time.Now()}
	if err := disks.Validate(r.Context(), disk, d.server.Root, existing); err != nil {
		return http.StatusBadRequest, err
	}

	if err := d.store.Disks.Save(disk); err != nil {
		return http.StatusInternalServerError, err
	}

	return renderJSON(w, r, buildDiskInfo(r.Context(), disk, false, 0))
})

var diskPutHandler = withAdmin(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	id, err := parseDiskID(r)
	if err != nil {
		return http.StatusBadRequest, err
	}

	disk, err := d.store.Disks.Get(id)
	if errors.Is(err, fberrors.ErrNotExist) {
		return http.StatusNotFound, err
	}
	if err != nil {
		return http.StatusInternalServerError, err
	}

	var body struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return http.StatusBadRequest, err
	}

	existing, err := d.store.Disks.Gets()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	updated := *disk
	updated.Name = body.Name
	updated.Path = body.Path
	if err := disks.Validate(r.Context(), &updated, d.server.Root, existing); err != nil {
		return http.StatusBadRequest, err
	}

	if updated.Path != disk.Path {
		counts, err := diskUserCounts(d)
		if err != nil {
			return http.StatusInternalServerError, err
		}
		if counts[id] > 0 {
			return http.StatusConflict, fmt.Errorf("%w: move its %d users first", fberrors.ErrDiskInUse, counts[id])
		}
	}

	if err := d.store.Disks.Update(&updated); err != nil {
		return http.StatusInternalServerError, err
	}

	return renderJSON(w, r, buildDiskInfo(r.Context(), &updated, false, 0))
})

var diskDeleteHandler = withAdmin(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	id, err := parseDiskID(r)
	if err != nil {
		return http.StatusBadRequest, err
	}
	if id == disks.SystemDiskID {
		return http.StatusBadRequest, fmt.Errorf("%w: the system disk cannot be removed", fberrors.ErrDiskInvalid)
	}

	counts, err := diskUserCounts(d)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if counts[id] > 0 {
		return http.StatusConflict, fmt.Errorf("%w: %d users are still on it", fberrors.ErrDiskInUse, counts[id])
	}

	if err := d.store.Disks.Delete(id); err != nil {
		if errors.Is(err, fberrors.ErrNotExist) {
			return http.StatusNotFound, err
		}
		return http.StatusInternalServerError, err
	}

	return http.StatusNoContent, nil
})

var diskDetectHandler = withAdmin(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	stored, err := d.store.Disks.Gets()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	return renderJSON(w, r, disks.Detect(r.Context(), d.server.Root, stored))
})

var diskBrowseHandler = withAdmin(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	stored, err := d.store.Disks.Gets()
	if err != nil {
		return http.StatusInternalServerError, err
	}

	res, err := disks.Browse(r.Context(), r.URL.Query().Get("path"), d.server.Root, stored)
	if err != nil {
		return http.StatusBadRequest, err
	}

	return renderJSON(w, r, res)
})

type userUsage struct {
	ID      uint  `json:"id"`
	Used    int64 `json:"used"`
	Quota   int64 `json:"quota"`
	DiskID  uint  `json:"diskID"`
	Offline bool  `json:"offline"`
	// Unknown is set when the walk could not finish inside the budget, so the
	// client shows a dash instead of a misleading zero.
	Unknown bool `json:"unknown"`
}

var usersUsageHandler = withAdmin(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	all, err := d.store.Users.Gets(d.server.Root, d.server.FollowExternalSymlinks)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	// Usage is a directory walk per user. The budget keeps one slow home from
	// holding the whole table behind a spinner; a user whose walk does not
	// finish simply comes back without a number.
	ctx, cancel := context.WithTimeout(r.Context(), usageBudget)
	defer cancel()

	out := make([]userUsage, 0, len(all))
	for _, u := range all {
		entry := userUsage{ID: u.ID, Quota: u.Quota, DiskID: u.DiskID}
		if u.DiskOffline || u.Fs == nil {
			entry.Offline = true
			out = append(out, entry)
			continue
		}

		if ctx.Err() != nil {
			entry.Unknown = true
			out = append(out, entry)
			continue
		}

		used, err := d.store.Quota.Used(ctx, u)
		if err == nil {
			entry.Used = used
		} else {
			entry.Unknown = true
		}
		out = append(out, entry)
	}

	return renderJSON(w, r, out)
})

type moveJob struct {
	ID       string    `json:"id"`
	UserID   uint      `json:"userID"`
	Username string    `json:"username"`
	FromDisk uint      `json:"fromDisk"`
	ToDisk   uint      `json:"toDisk"`
	State    string    `json:"state"`
	Total    int64     `json:"total"`
	Copied   int64     `json:"copied"`
	Error    string    `json:"error,omitempty"`
	Started  time.Time `json:"started"`
	Finished time.Time `json:"finished"`
}

type moveRegistry struct {
	mu     sync.Mutex
	jobs   map[string]*moveJob
	active map[uint]string
}

var moves = &moveRegistry{
	jobs:   map[string]*moveJob{},
	active: map[uint]string{},
}

func (r *moveRegistry) create(job *moveJob) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, busy := r.active[job.UserID]; busy {
		return false
	}
	r.jobs[job.ID] = job
	r.active[job.UserID] = job.ID
	return true
}

func (r *moveRegistry) progress(id string, copied int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if job, ok := r.jobs[id]; ok {
		job.Copied = copied
	}
}

func (r *moveRegistry) succeed(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.jobs[id]
	if !ok {
		return
	}
	job.State = "done"
	job.Copied = job.Total
	job.Finished = time.Now()
	delete(r.active, job.UserID)
	r.prune()
}

func (r *moveRegistry) fail(id string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.jobs[id]
	if !ok {
		return
	}
	job.State = "failed"
	job.Error = err.Error()
	job.Finished = time.Now()
	delete(r.active, job.UserID)
	r.prune()
}

func (r *moveRegistry) get(id string) *moveJob {
	r.mu.Lock()
	defer r.mu.Unlock()

	job, ok := r.jobs[id]
	if !ok {
		return nil
	}
	copied := *job
	return &copied
}

// prune drops finished jobs after ten minutes so the map does not grow for the
// lifetime of the process.
func (r *moveRegistry) prune() {
	cutoff := time.Now().Add(-10 * time.Minute)
	for id, job := range r.jobs {
		if !job.Finished.IsZero() && job.Finished.Before(cutoff) {
			delete(r.jobs, id)
		}
	}
}

var userMoveHandler = withAdmin(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	id, err := getUserID(r)
	if err != nil {
		return http.StatusBadRequest, err
	}

	var body struct {
		DiskID uint `json:"diskID"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return http.StatusBadRequest, err
	}

	if _, err := d.diskRootFor(body.DiskID); err != nil {
		return http.StatusBadRequest, err
	}

	u, err := d.store.Users.Get(d.server.Root, d.server.FollowExternalSymlinks, id)
	if err != nil {
		return errToStatus(err), err
	}

	if u.DiskID == body.DiskID {
		return http.StatusBadRequest, fmt.Errorf("the user is already on that disk")
	}

	scope := filepath.Clean("/" + u.Scope)
	if scope == "/" {
		return http.StatusBadRequest, fmt.Errorf("%w: a user rooted at the server root cannot be moved", fberrors.ErrDiskInvalid)
	}

	src := u.FullPath("/")
	dstRoot, err := d.diskRootFor(body.DiskID)
	if err != nil {
		return http.StatusBadRequest, err
	}
	dst := filepath.Join(dstRoot, filepath.FromSlash(scope))

	if src == dst {
		return http.StatusBadRequest, fmt.Errorf("the source and destination are the same directory")
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if !srcInfo.IsDir() {
		return http.StatusBadRequest, fmt.Errorf("the user's home is not a directory")
	}

	if info, statErr := os.Stat(dst); statErr == nil {
		if !info.IsDir() {
			return http.StatusConflict, fmt.Errorf("the destination exists and is not a directory")
		}
		entries, readErr := os.ReadDir(dst)
		if readErr != nil {
			return http.StatusInternalServerError, readErr
		}
		if len(entries) > 0 {
			return http.StatusConflict, fmt.Errorf("the destination already has files: %s", dst)
		}
	}

	total, err := disks.DirSize(src)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	usage, err := disks.Usage(r.Context(), dstRoot)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	need := total + disks.SafetyReserve(usage.Total)
	if need < 0 || uint64(need) > usage.Free {
		return http.StatusInsufficientStorage, fmt.Errorf(
			"%w: need %d bytes, %d free on the destination", fberrors.ErrDiskFull, need, usage.Free)
	}

	job := &moveJob{
		ID:       fmt.Sprintf("%d-%d", u.ID, time.Now().UnixNano()),
		UserID:   u.ID,
		Username: u.Username,
		FromDisk: u.DiskID,
		ToDisk:   body.DiskID,
		State:    "running",
		Total:    total,
		Started:  time.Now(),
	}

	if !moves.create(job) {
		return http.StatusConflict, fberrors.ErrUserMoving
	}

	d.store.Quota.SetMoving(u.ID, true)
	go runUserMove(d, job, src, dst, dstRoot)

	return renderJSON(w, r, map[string]string{"id": job.ID})
})

var jobGetHandler = withAdmin(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	job := moves.get(mux.Vars(r)["id"])
	if job == nil {
		return http.StatusNotFound, fberrors.ErrNotExist
	}
	return renderJSON(w, r, job)
})

func runUserMove(d *data, job *moveJob, src, dst, dstRoot string) {
	ctx := context.Background()

	_, statErr := os.Stat(dst)
	dstExisted := statErr == nil

	err := disks.MoveTree(ctx, src, dst, func(p disks.MoveProgress) {
		moves.progress(job.ID, p.Copied)
	}, func() error {
		usage, err := disks.Usage(ctx, dstRoot)
		if err != nil {
			return err
		}
		if usage.Free < uint64(disks.SafetyReserve(usage.Total))+(64<<20) {
			return fberrors.ErrDiskFull
		}
		return nil
	})

	if err == nil {
		// Switch the row only after every file is verified at the destination.
		user, getErr := d.store.Users.Get(d.server.Root, d.server.FollowExternalSymlinks, job.UserID)
		if getErr != nil {
			err = getErr
		} else {
			user.DiskID = job.ToDisk
			if updateErr := d.store.Users.Update(user, "DiskID"); updateErr != nil {
				err = updateErr
			}
		}
	}

	d.store.Quota.SetMoving(job.UserID, false)
	d.store.Quota.Invalidate(job.UserID)

	if err != nil {
		if !dstExisted {
			_ = os.RemoveAll(dst)
		}
		moves.fail(job.ID, err)
		log.Printf("disk move: user %d failed: %v", job.UserID, err)
		return
	}

	if rmErr := os.RemoveAll(src); rmErr != nil {
		log.Printf("disk move: user %d moved, but the old home could not be removed: %v", job.UserID, rmErr)
	}

	moves.succeed(job.ID)
	log.Printf("disk move: user %d moved from disk %d to disk %d", job.UserID, job.FromDisk, job.ToDisk)
}

func parseDiskID(r *http.Request) (uint, error) {
	id, err := strconv.ParseUint(mux.Vars(r)["id"], 10, 0)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}
