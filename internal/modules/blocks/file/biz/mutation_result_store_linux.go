//go:build linux

package file

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/muidea/workorch/internal/pkg/common"
	"golang.org/x/sys/unix"
)

func openMutationResultStore(root, key string, create bool) (*mutationResultStore, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || len(key) != 64 {
		return nil, common.ErrInvalidArgument
	}
	if raw, err := hex.DecodeString(key); err != nil || len(raw) != 32 {
		return nil, common.ErrInvalidArgument
	}
	var fd int
	var err error
	if create {
		fd, err = createPreparationDirectory(root)
		if err == nil {
			fd, err = createPreparationDirectoryAt(fd, "file-operation-results/"+key, true)
		}
	} else {
		fd, err = openAuthoringRoot(root)
		if err == nil {
			parent := fd
			fd, err = unix.Openat2(parent, "file-operation-results/"+key, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
			unix.Close(parent)
		}
	}
	if err != nil {
		return nil, err
	}
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil || st.Mode&0077 != 0 || st.Uid != uint32(os.Geteuid()) {
		unix.Close(fd)
		return nil, fmt.Errorf("File result directory is not restricted to its owner")
	}
	return &mutationResultStore{fd: fd}, nil
}
func (s *mutationResultStore) close() { unix.Close(s.fd) }
func (s *mutationResultStore) read(name string) ([]byte, error) {
	if filepath.Base(name) != name || name == "." || name == ".." {
		return nil, common.ErrInvalidArgument
	}
	fd, err := unix.Openat2(s.fd, name, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil {
		return nil, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Mode&0077 != 0 || st.Size > mutationResultMaxBytes {
		return nil, fmt.Errorf("invalid restricted File result material")
	}
	b, err := io.ReadAll(io.LimitReader(f, mutationResultMaxBytes+1))
	if len(b) > mutationResultMaxBytes {
		return nil, fmt.Errorf("File result exceeds byte boundary")
	}
	return b, err
}

// Immutable atomic publication. Existing bytes are never overwritten. Every
// caller must check the existing generation before interpreting it.
func (s *mutationResultStore) write(name string, b []byte) (bool, error) {
	if filepath.Base(name) != name || name == "." || name == ".." {
		return false, common.ErrInvalidArgument
	}
	if len(b) > mutationResultMaxBytes {
		return false, fmt.Errorf("File result exceeds byte boundary: %d bytes", len(b))
	}
	tmp := ".result-" + rand.Text()
	fd, err := unix.Openat(s.fd, tmp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return false, err
	}
	defer unix.Unlinkat(s.fd, tmp, 0)
	f := os.NewFile(uintptr(fd), tmp)
	n, err := f.Write(b)
	if err == nil && n != len(b) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	err = unix.Renameat2(s.fd, tmp, s.fd, name, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.EEXIST) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, unix.Fsync(s.fd)
}
