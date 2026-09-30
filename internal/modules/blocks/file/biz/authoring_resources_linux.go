//go:build linux

package file

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/muidea/workorch/internal/pkg/common"
)

// openAuthoringResource confines the lookup to the opened workspace root.
// In particular, a mount or symlink introduced between path validation and
// openat2 cannot change the file that is read into the creation-time fact.
func openAuthoringResource(root, ref string) (*os.File, error) {
	rootFD, err := openAuthoringRoot(root)
	if err != nil {
		return nil, err
	}
	defer unix.Close(rootFD)
	var rootInfo unix.Stat_t
	if err := unix.Fstat(rootFD, &rootInfo); err != nil {
		return nil, err
	}
	fd, err := unix.Openat2(rootFD, ref, &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NONBLOCK | unix.O_NOFOLLOW,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV,
	})
	if err != nil {
		return nil, err
	}
	var info unix.Stat_t
	if err := unix.Fstat(fd, &info); err != nil {
		unix.Close(fd)
		return nil, err
	}
	if info.Mode&unix.S_IFMT != unix.S_IFREG || info.Nlink != 1 || info.Dev != rootInfo.Dev {
		unix.Close(fd)
		return nil, fmt.Errorf("%w: authoring resource must be a single-link regular file on the workspace filesystem", common.ErrInvalidArgument)
	}
	var rootMount, fileMount unix.Statx_t
	if unix.Statx(rootFD, "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &rootMount) != nil ||
		unix.Statx(fd, "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &fileMount) != nil ||
		rootMount.Mask&unix.STATX_MNT_ID == 0 || fileMount.Mask&unix.STATX_MNT_ID == 0 || rootMount.Mnt_id != fileMount.Mnt_id {
		unix.Close(fd)
		return nil, fmt.Errorf("%w: authoring resource mount cannot be verified", common.ErrInvalidArgument)
	}
	return os.NewFile(uintptr(fd), ref), nil
}

func openAuthoringRoot(root string) (int, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return -1, err
	}
	absolute = filepath.Clean(absolute)
	if !filepath.IsAbs(absolute) {
		return -1, common.ErrInvalidArgument
	}
	parts := strings.Split(strings.TrimPrefix(absolute, string(filepath.Separator)), string(filepath.Separator))
	fd, err := unix.Open(string(filepath.Separator), unix.O_PATH|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, part := range parts {
		if part == "" {
			continue
		}
		next, openErr := unix.Openat2(fd, part, &unix.OpenHow{
			Flags:   unix.O_PATH | unix.O_DIRECTORY | unix.O_CLOEXEC,
			Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS,
		})
		if openErr != nil {
			unix.Close(fd)
			return -1, openErr
		}
		unix.Close(fd)
		fd = next
	}
	return fd, nil
}

func statAuthoringResource(root, ref string) (os.FileInfo, error) {
	file, err := openAuthoringResource(root, ref)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return file.Stat()
}

func sameAuthoringFileVersion(before, after os.FileInfo) bool {
	first, firstOK := before.Sys().(*syscall.Stat_t)
	second, secondOK := after.Sys().(*syscall.Stat_t)
	return firstOK && secondOK && before.Size() == after.Size() && first.Ctim == second.Ctim && first.Mtim == second.Mtim && first.Nlink == 1 && second.Nlink == 1
}
