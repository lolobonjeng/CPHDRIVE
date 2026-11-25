package bolt

import (
	"errors"

	"github.com/asdine/storm/v3"

	"github.com/filebrowser/filebrowser/v2/disks"
	fberrors "github.com/filebrowser/filebrowser/v2/errors"
)

type disksBackend struct {
	db *storm.DB
}

func (st disksBackend) GetByID(id uint) (*disks.Disk, error) {
	disk := &disks.Disk{}
	err := st.db.One("ID", id, disk)
	if err != nil {
		if errors.Is(err, storm.ErrNotFound) {
			return nil, fberrors.ErrNotExist
		}
		return nil, err
	}
	return disk, nil
}

func (st disksBackend) Gets() ([]*disks.Disk, error) {
	var all []*disks.Disk
	err := st.db.All(&all)
	if errors.Is(err, storm.ErrNotFound) {
		return []*disks.Disk{}, nil
	}
	if err != nil {
		return nil, err
	}
	if all == nil {
		all = []*disks.Disk{}
	}
	return all, nil
}

func (st disksBackend) Save(disk *disks.Disk) error {
	err := st.db.Save(disk)
	if errors.Is(err, storm.ErrAlreadyExists) {
		return fberrors.ErrExist
	}
	return err
}

func (st disksBackend) Update(disk *disks.Disk) error {
	err := st.db.Update(disk)
	if errors.Is(err, storm.ErrNotFound) {
		return fberrors.ErrNotExist
	}
	return err
}

func (st disksBackend) DeleteByID(id uint) error {
	err := st.db.DeleteStruct(&disks.Disk{ID: id})
	if errors.Is(err, storm.ErrNotFound) {
		return fberrors.ErrNotExist
	}
	return err
}
