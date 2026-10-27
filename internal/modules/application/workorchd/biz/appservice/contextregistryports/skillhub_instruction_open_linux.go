package contextregistryports

import (
	"golang.org/x/sys/unix"
	"os"
)

// O_NONBLOCK prevents a raced FIFO replacement from waiting for a writer.
// os.Root contains symlink traversal; the caller also checks file identity and
// rejects non-regular files before reading. No write/create/truncate flags.
func openPreparationInstructionFile(root *os.Root, path string) (*os.File, error) {
	return root.OpenFile(path, os.O_RDONLY|unix.O_NONBLOCK, 0)
}
