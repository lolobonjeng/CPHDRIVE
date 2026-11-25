package users

import (
	"errors"
	"sync"
	"time"

	fberrors "github.com/filebrowser/filebrowser/v2/errors"
)

// StorageBackend is the interface to implement for a users storage.
type StorageBackend interface {
	GetBy(interface{}) (*User, error)
	GetByScope(scope string) (*User, error)
	Gets() ([]*User, error)
	Save(u *User) error
	Update(u *User, fields ...string) error
	DeleteByID(uint) error
	DeleteByUsername(string) error
	CountAdmins() (int, error)
}

type Store interface {
	Get(baseScope string, followExternalSymlinks bool, id interface{}) (user *User, err error)
	// GetByUsername returns the stored row without resolving its filesystem.
	// It exists for callers that must inspect a user whose disk is offline,
	// where Get fails before the row is usable.
	GetByUsername(username string) (*User, error)
	GetByScope(scope string) (*User, error)
	Gets(baseScope string, followExternalSymlinks bool) ([]*User, error)
	Update(user *User, fields ...string) error
	Save(user *User) error
	SaveProvisioned(user *User, derivedScope bool) error
	Delete(id interface{}) error
	LastUpdate(id uint) int64
}

// DiskProvider resolves a disk ID to the directory that holds user homes.
type DiskProvider interface {
	Path(id uint) (string, error)
}

// Storage is a users storage.
type Storage struct {
	back    StorageBackend
	updated map[uint]int64
	mux     sync.RWMutex
	disks   DiskProvider

	// provision serializes the scope-collision check and the save of newly
	// provisioned users, which must not interleave. See SaveProvisioned.
	provision sync.Mutex
}

// NewStorage creates a users storage from a backend.
func NewStorage(back StorageBackend) *Storage {
	return &Storage{
		back:    back,
		updated: map[uint]int64{},
	}
}

// SetDisks wires the disk registry used to resolve a user's home. It is set
// once at startup, before any request is served.
func (s *Storage) SetDisks(d DiskProvider) {
	s.disks = d
}

// scope resolves the base directory that holds this user's home. Users on the
// system disk keep using the server root; everyone else is rooted at their
// registered disk.
func (s *Storage) scope(base string, u *User) (string, error) {
	if u.DiskID == 0 {
		return base, nil
	}
	if s.disks == nil {
		return "", fberrors.ErrDiskUnavailable
	}
	return s.disks.Path(u.DiskID)
}

// Get allows you to get a user by its name or username. The provided
// id must be a string for username lookup or a uint for id lookup. If id
// is neither, a ErrInvalidDataType will be returned.
func (s *Storage) Get(baseScope string, followExternalSymlinks bool, id interface{}) (user *User, err error) {
	user, err = s.back.GetBy(id)
	if err != nil {
		return
	}

	scope, err := s.scope(baseScope, user)
	if err != nil {
		return nil, err
	}
	if err := user.Clean(scope, followExternalSymlinks); err != nil {
		return nil, err
	}
	return
}

// GetByUsername returns the stored user row, without setting up its
// filesystem. It is meant for cases that must still read the account while its
// disk is unavailable.
func (s *Storage) GetByUsername(username string) (*User, error) {
	return s.back.GetBy(username)
}

// GetByScope returns the first user whose scope matches the given one, or
// ErrNotExist if none does. The user is returned as stored, without setting up
// its filesystem, as it is meant for existence checks rather than serving.
func (s *Storage) GetByScope(scope string) (*User, error) {
	return s.back.GetByScope(scope)
}

// Gets gets a list of all users.
func (s *Storage) Gets(baseScope string, followExternalSymlinks bool) ([]*User, error) {
	users, err := s.back.Gets()
	if err != nil {
		return nil, err
	}

	for _, user := range users {
		scope, err := s.scope(baseScope, user)
		if err != nil {
			// A listing must still show the account when its disk is gone,
			// otherwise one unplugged drive hides every other user. The flag
			// lets the admin see which account needs attention.
			if errors.Is(err, fberrors.ErrDiskUnavailable) {
				user.DiskOffline = true
				continue
			}
			return nil, err
		}

		if err := user.Clean(scope, followExternalSymlinks); err != nil {
			return nil, err
		}
	}

	return users, err
}

// Update updates a user in the database.
func (s *Storage) Update(user *User, fields ...string) error {
	err := user.Clean("", false, fields...)
	if err != nil {
		return err
	}

	err = s.back.Update(user, fields...)
	if err != nil {
		return err
	}

	s.mux.Lock()
	s.updated[user.ID] = time.Now().Unix()
	s.mux.Unlock()
	return nil
}

// Save saves the user in a storage.
func (s *Storage) Save(user *User) error {
	if err := user.Clean("", false); err != nil {
		return err
	}

	return s.back.Save(user)
}

// SaveProvisioned saves a user that is being provisioned (via signup, proxy
// auth or hook auth). When its scope was derived from the username, it first
// rejects the save if another user already owns that scope, so that distinct
// usernames cannot silently share one home directory.
//
// The check and the save are held under a single lock. Performing them as two
// independent operations lets two concurrent provisioning requests both observe
// a free scope and both save, leaving two users sharing one home directory.
func (s *Storage) SaveProvisioned(user *User, derivedScope bool) error {
	if !derivedScope {
		return s.Save(user)
	}

	s.provision.Lock()
	defer s.provision.Unlock()

	switch _, err := s.back.GetByScope(user.Scope); {
	case err == nil:
		return fberrors.ErrExist
	case !errors.Is(err, fberrors.ErrNotExist):
		return err
	}

	return s.Save(user)
}

// Delete allows you to delete a user by its name or username. The provided
// id must be a string for username lookup or a uint for id lookup. If id
// is neither, a ErrInvalidDataType will be returned.
func (s *Storage) Delete(id interface{}) error {
	switch id := id.(type) {
	case string:
		user, err := s.back.GetBy(id)
		if err != nil {
			return err
		}
		if s.IsUniqueAdmin(user) {
			return fberrors.ErrRootUserDeletion
		}

		return s.back.DeleteByUsername(id)
	case uint:
		user, err := s.back.GetBy(id)
		if err != nil {
			return err
		}
		if s.IsUniqueAdmin(user) {
			return fberrors.ErrRootUserDeletion
		}

		return s.back.DeleteByID(id)
	default:
		return fberrors.ErrInvalidDataType
	}
}

// LastUpdate gets the timestamp for the last update of an user.
func (s *Storage) LastUpdate(id uint) int64 {
	s.mux.RLock()
	defer s.mux.RUnlock()
	if val, ok := s.updated[id]; ok {
		return val
	}
	return 0
}

func (s *Storage) IsUniqueAdmin(user *User) bool {
	if !user.Perm.Admin {
		return false
	}

	count, err := s.back.CountAdmins()
	if err != nil {
		return true
	}
	return count <= 1
}
