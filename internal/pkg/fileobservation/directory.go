// Package fileobservation contains detached filesystem values only. It has no
// owner, topic, I/O handle, preparation verdict or execution authority.
package fileobservation

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

// Directory is a point-in-time identity/permission observation, not a promise
// that future filesystem operations will succeed. Diagnostic bytes are original
// owner errors, not public display text. A failed observation is never complete.
type Directory struct {
	Path                                                         string
	Complete                                                     bool
	Device, Inode                                                uint64
	Mode, UID, GID                                               uint32
	ModifiedSeconds, ModifiedNanos, ChangedSeconds, ChangedNanos int64
	Readable, Writable, Searchable                               bool
	Diagnostic                                                   []byte
	DiagnosticBytes                                              int
	DiagnosticSHA256                                             string
}

const DiagnosticLimit = 4096

func (d Directory) Clone() Directory {
	d.Diagnostic = slices.Clone(d.Diagnostic)
	return d
}

func (d Directory) Validate() error {
	if !ValidPath(d.Path) || len(d.Diagnostic) > DiagnosticLimit || len(d.Diagnostic) != d.DiagnosticBytes || d.DiagnosticSHA256 != Digest(d.Diagnostic) {
		return common.ErrConflict
	}
	if d.Complete {
		if len(d.Diagnostic) != 0 || d.Inode == 0 || d.Mode&0170000 != 0040000 || d.ModifiedNanos < 0 || d.ModifiedNanos >= 1e9 || d.ChangedNanos < 0 || d.ChangedNanos >= 1e9 {
			return common.ErrConflict
		}
	} else if len(d.Diagnostic) == 0 || d.Device != 0 || d.Inode != 0 || d.Mode != 0 || d.UID != 0 || d.GID != 0 || d.ModifiedSeconds != 0 || d.ModifiedNanos != 0 || d.ChangedSeconds != 0 || d.ChangedNanos != 0 || d.Readable || d.Writable || d.Searchable {
		return common.ErrConflict
	}
	return nil
}

func ValidPath(path string) bool {
	return len(path) > 1 && len(path) <= 4096 && utf8.ValidString(path) && filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsRune(path, 0)
}

func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
