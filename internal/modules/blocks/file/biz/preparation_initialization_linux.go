//go:build linux

package file

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	"golang.org/x/sys/unix"
)

// Pin every directory without symlink traversal. Only infrastructure paths
// derived by Workspace are accepted; no shell or external SkillHub init runs.
func createPreparationDirectory(path string) (int, error) {
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	return createPreparationDirectoryAt(fd, strings.TrimPrefix(filepath.Clean(path), "/"), false)
}

// Takes ownership of fd. Descendants of a sealed root cannot cross mounts.
func createPreparationDirectoryAt(fd int, ref string, noXdev bool) (int, error) {
	resolve := uint64(unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS)
	if noXdev {
		resolve |= unix.RESOLVE_NO_XDEV
	}
	for _, part := range strings.Split(ref, "/") {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			unix.Close(fd)
			return -1, common.ErrInvalidArgument
		}
		next, e := unix.Openat2(fd, part, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: resolve})
		if errors.Is(e, unix.ENOENT) {
			if e = unix.Mkdirat(fd, part, 0700); e == nil || errors.Is(e, unix.EEXIST) {
				if e = unix.Fsync(fd); e != nil {
					unix.Close(fd)
					return -1, e
				}
				next, e = unix.Openat2(fd, part, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: resolve})
			}
		}
		unix.Close(fd)
		if e != nil {
			return -1, e
		}
		fd = next
	}
	return fd, nil
}
func validatePreparationInitializationRoots(roots ...string) error {
	for _, root := range roots {
		fd, e := openAuthoringRoot(root)
		if e != nil {
			return e
		}
		unix.Close(fd)
	}
	return nil
}
func createPreparationSkillDirectory(root string) error {
	fd, err := openAuthoringRoot(root)
	if err != nil {
		return err
	}
	// Turn the pinned O_PATH root into a directory descriptor supporting fsync.
	writable, e := unix.Openat2(fd, ".", &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
	unix.Close(fd)
	if e != nil {
		return e
	}
	dir, e := createPreparationDirectoryAt(writable, ".agents/skills", true)
	if e == nil {
		unix.Close(dir)
	}
	return e
}
func createPreparationConfig(root, ref string, raw []byte) error {
	if strings.Contains(root, "\x00") {
		return common.ErrInvalidArgument
	}
	rootFD, err := createPreparationDirectory(root)
	if err != nil {
		return err
	}
	fd, err := createPreparationDirectoryAt(rootFD, filepath.Dir(filepath.FromSlash(ref)), true)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	name := filepath.Base(ref)
	// Existing paths, even malformed files or symlinks, belong to the operator.
	var st unix.Stat_t
	if e := unix.Fstatat(fd, name, &st, unix.AT_SYMLINK_NOFOLLOW); e == nil {
		return nil
	} else if !errors.Is(e, unix.ENOENT) {
		return e
	}
	temp := ".workorch-init-" + rand.Text()
	out, e := unix.Openat(fd, temp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return e
	}
	defer unix.Unlinkat(fd, temp, 0)
	f := os.NewFile(uintptr(out), temp)
	n, e := f.Write(raw)
	if e == nil && n != len(raw) {
		e = common.ErrConflict
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	// Atomic no-replace publication leaves no extra hard link after a crash.
	e = unix.Renameat2(fd, temp, fd, name, unix.RENAME_NOREPLACE)
	if errors.Is(e, unix.EEXIST) {
		return nil
	}
	if e != nil {
		return e
	}
	return unix.Fsync(fd)
}
