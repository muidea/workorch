//go:build unix

package shell

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Pin each ancestor by descriptor: a concurrent rename/symlink swap cannot
// redirect the selector through another ancestor. O_NONBLOCK prevents a racing
// FIFO from blocking before the regular-file gate can reject it. OpenRoot still
// defines the exact Workspace-owner directory boundary.
func openShellDependencySelector(ctx context.Context, root *os.Root, ref string) (*os.File, error) {
	if root == nil || ref == "." || !filepath.IsLocal(ref) || filepath.ToSlash(filepath.Clean(ref)) != ref {
		return nil, os.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	directory, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { _ = directory.Close() }()
	parts := strings.Split(ref, "/")
	for index, part := range parts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
		if index != len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		fd, err := unix.Openat(int(directory.Fd()), part, flags, 0)
		if err != nil {
			return nil, &os.PathError{Op: "open", Path: ref, Err: err}
		}
		file := os.NewFile(uintptr(fd), part)
		if index == len(parts)-1 {
			return file, nil
		}
		if err := directory.Close(); err != nil {
			_ = file.Close()
			return nil, err
		}
		directory = file
	}
	return nil, os.ErrInvalid
}
