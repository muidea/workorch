//go:build linux

package file

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"os"
	"syscall"
)

func preparationResourceIdentity(info os.FileInfo, r *fileobservation.Resource) error {
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return common.ErrConflict
	}
	r.Device, r.Inode, r.Mode, r.UID, r.GID = uint64(s.Dev), s.Ino, s.Mode, s.Uid, s.Gid
	r.ModifiedSeconds, r.ModifiedNanos, r.ChangedSeconds, r.ChangedNanos = s.Mtim.Sec, s.Mtim.Nsec, s.Ctim.Sec, s.Ctim.Nsec
	return r.Validate()
}
