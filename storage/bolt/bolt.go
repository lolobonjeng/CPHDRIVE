package bolt

import (
	"errors"
	"log"

	"github.com/asdine/storm/v3"

	"github.com/filebrowser/filebrowser/v2/auth"
	"github.com/filebrowser/filebrowser/v2/disks"
	fberrors "github.com/filebrowser/filebrowser/v2/errors"
	"github.com/filebrowser/filebrowser/v2/quota"
	"github.com/filebrowser/filebrowser/v2/settings"
	"github.com/filebrowser/filebrowser/v2/share"
	"github.com/filebrowser/filebrowser/v2/storage"
	"github.com/filebrowser/filebrowser/v2/users"
)

// NewStorage creates a storage.Storage based on Bolt DB.
func NewStorage(db *storm.DB) (*storage.Storage, error) {
	userStore := users.NewStorage(usersBackend{db: db})
	diskStore := disks.NewStorage(disksBackend{db: db})
	userStore.SetDisks(diskStore)
	shareStore := share.NewStorage(shareBackend{db: db})
	settingsStore := settings.NewStorage(settingsBackend{db: db})
	authStore := auth.NewStorage(authBackend{db: db}, userStore)

	err := save(db, "version", 3)
	if err != nil {
		return nil, err
	}

	if err := applyDefaultQuota(db); err != nil {
		return nil, err
	}

	return &storage.Storage{
		Auth:     authStore,
		Users:    userStore,
		Share:    shareStore,
		Settings: settingsStore,
		Disks:    diskStore,
		Quota:    quota.New(),
	}, nil
}

// applyDefaultQuota gives every non-admin user that has never had a quota the
// 100 GiB default exactly once. The marker is written even when no user needed
// the change, so an admin can later set a user back to unlimited (0) without
// the next restart silently re-applying the default.
func applyDefaultQuota(db *storm.DB) error {
	var applied bool
	err := get(db, "defaultQuotaV1", &applied)
	if err == nil && applied {
		return nil
	}
	if err != nil && !errors.Is(err, fberrors.ErrNotExist) {
		return err
	}

	back := usersBackend{db: db}
	all, err := back.Gets()
	if err != nil && !errors.Is(err, fberrors.ErrNotExist) {
		return err
	}

	changed := 0
	for _, u := range all {
		if u.Perm.Admin || u.Quota != 0 {
			continue
		}
		u.Quota = settings.DefaultQuota
		if err := back.Save(u); err != nil {
			return err
		}
		changed++
	}

	// The settings record predates this field too. Treat its zero value as
	// unset for the same one-time migration, so new users created from the
	// admin form start with the same 100 GiB default.
	settingsStore := settings.NewStorage(settingsBackend{db: db})
	if set, err := settingsStore.Get(); err == nil && set.Defaults.Quota == 0 {
		set.Defaults.Quota = settings.DefaultQuota
		if err := settingsStore.Save(set); err != nil {
			return err
		}
	}

	if err := save(db, "defaultQuotaV1", true); err != nil {
		return err
	}

	if changed > 0 {
		log.Printf("quota: applied the %d GiB default to %d existing users", settings.DefaultQuota>>30, changed)
	}
	return nil
}
