package fileobservation

import (
	"encoding/hex"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/muidea/workorch/internal/pkg/common"
)

const ResourceLimit = 128
const ResourceMaxBytes = 32 * 1024
const ResourceTotalBytes = 128 * 1024

// Resource preserves the complete bytes of a bounded, pinned regular-file
// read. Incomplete observations contain no partial content or invented version.
// Neither the content digest nor this point-in-time version grants currentness
// for a later operation.
type Resource struct {
	Ref                                                          string
	Complete                                                     bool
	Content                                                      []byte
	ContentBytes                                                 int
	ContentSHA256                                                string
	Device, Inode                                                uint64
	Mode, UID, GID                                               uint32
	ModifiedSeconds, ModifiedNanos, ChangedSeconds, ChangedNanos int64
	Diagnostic                                                   []byte
	DiagnosticBytes                                              int
	DiagnosticSHA256                                             string
	// Oversized diagnostics are omitted explicitly, never truncated or marked
	// complete. Their original byte count/digest remain historical metadata.
	DiagnosticNotSavedReason string
}

func ValidResourceRef(ref string) bool {
	return ref != "" && len(ref) <= 1024 && utf8.ValidString(ref) && !strings.ContainsRune(ref, 0)
}

func (r Resource) Clone() Resource {
	r.Content, r.Diagnostic = slices.Clone(r.Content), slices.Clone(r.Diagnostic)
	return r
}

func (r Resource) Validate() error {
	if !ValidResourceRef(r.Ref) || len(r.Content) > ResourceMaxBytes || len(r.Content) != r.ContentBytes || r.ContentSHA256 != Digest(r.Content) || len(r.Diagnostic) > DiagnosticLimit {
		return common.ErrConflict
	}
	if r.DiagnosticNotSavedReason == "" {
		if len(r.Diagnostic) != r.DiagnosticBytes || r.DiagnosticSHA256 != Digest(r.Diagnostic) {
			return common.ErrConflict
		}
	} else {
		digest, err := hex.DecodeString(r.DiagnosticSHA256)
		if r.Complete || r.DiagnosticNotSavedReason != "diagnostic_exceeds_byte_boundary" || len(r.Diagnostic) != 0 || r.DiagnosticBytes <= DiagnosticLimit || err != nil || len(digest) != 32 {
			return common.ErrConflict
		}
	}
	if r.Complete {
		if len(r.Diagnostic) != 0 || r.Inode == 0 || r.Mode&0170000 != 0100000 || r.ModifiedNanos < 0 || r.ModifiedNanos >= 1e9 || r.ChangedNanos < 0 || r.ChangedNanos >= 1e9 {
			return common.ErrConflict
		}
	} else if r.DiagnosticBytes == 0 || len(r.Content) != 0 || r.Device != 0 || r.Inode != 0 || r.Mode != 0 || r.UID != 0 || r.GID != 0 || r.ModifiedSeconds != 0 || r.ModifiedNanos != 0 || r.ChangedSeconds != 0 || r.ChangedNanos != 0 {
		return common.ErrConflict
	}
	return nil
}
