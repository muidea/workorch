//go:build linux

package file

import (
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
	"golang.org/x/sys/unix"
)

// No mkdir, write probe, chmod or repair. Resolve every directory component
// without symlinks, pin the descriptor, check effective credentials (ACL-aware),
// then reopen the path and compare the full observed identity/version.
func observePreparationDirectory(path string) fileobservation.Directory {
	fd, err := openAuthoringRoot(path)
	if err != nil {
		return failedPreparationDirectory(path, err)
	}
	defer unix.Close(fd)
	var before unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil {
		return failedPreparationDirectory(path, err)
	}
	permissions := [3]bool{}
	for i, mode := range []uint32{unix.R_OK, unix.W_OK, unix.X_OK} {
		err := unix.Faccessat2(fd, "", mode, unix.AT_EMPTY_PATH|unix.AT_EACCESS)
		if err != nil && err != unix.EACCES && err != unix.EPERM {
			return failedPreparationDirectory(path, err)
		}
		permissions[i] = err == nil
	}
	reopened, err := openAuthoringRoot(path)
	if err != nil {
		return failedPreparationDirectory(path, err)
	}
	defer unix.Close(reopened)
	var after, pinned unix.Stat_t
	if err := unix.Fstat(reopened, &after); err != nil {
		return failedPreparationDirectory(path, err)
	}
	if err := unix.Fstat(fd, &pinned); err != nil {
		return failedPreparationDirectory(path, err)
	}
	if !samePreparationDirectoryStat(before, after) || !samePreparationDirectoryStat(before, pinned) {
		return failedPreparationDirectory(path, common.ErrConflict)
	}
	return fileobservation.Directory{Path: path, Complete: true, Device: uint64(before.Dev), Inode: before.Ino, Mode: before.Mode, UID: before.Uid, GID: before.Gid,
		ModifiedSeconds: before.Mtim.Sec, ModifiedNanos: before.Mtim.Nsec, ChangedSeconds: before.Ctim.Sec, ChangedNanos: before.Ctim.Nsec,
		Readable: permissions[0], Writable: permissions[1], Searchable: permissions[2], DiagnosticSHA256: fileobservation.Digest(nil)}
}

func samePreparationDirectoryStat(a, b unix.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Mode == b.Mode && a.Uid == b.Uid && a.Gid == b.Gid && a.Mtim == b.Mtim && a.Ctim == b.Ctim
}
