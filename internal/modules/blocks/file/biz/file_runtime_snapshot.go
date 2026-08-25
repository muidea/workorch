package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/muidea/workorch/internal/pkg/common"
	runtimemodel "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func (a *RuntimeAdapter) AcquireTargetSnapshot(ctx context.Context, workspaceDirectory, targetRef string, maxBytes int) (runtimemodel.TargetSnapshot, error) {
	result, err := a.ReadFile(ctx, FileReadRequest{
		WorkspaceDirectory: workspaceDirectory,
		Path:               targetRef,
		MaxBytes:           int64(maxBytes),
	})
	if err != nil {
		return runtimemodel.TargetSnapshot{}, err
	}
	snapshot := runtimemodel.TargetSnapshot{
		TargetRef:   result.TargetRef,
		Content:     result.Content,
		ContentHash: result.ContentHash,
		Size:        result.Size,
		LineCount:   int(result.LineCount),
	}
	switch {
	case !result.Exists:
		snapshot.ContentState = runtimemodel.SnapshotContentStateMissing
	case result.Binary:
		snapshot.ContentState = runtimemodel.SnapshotContentStateBinary
	case result.Redacted:
		snapshot.ContentState = runtimemodel.SnapshotContentStateRedacted
	case result.Truncated:
		snapshot.ContentState = runtimemodel.SnapshotContentStateTruncated
	case result.ContentComplete:
		snapshot.ContentState = runtimemodel.SnapshotContentStateComplete
	}
	if !snapshot.Complete() {
		return snapshot, fmt.Errorf("%w: target snapshot %q is not complete (%s)", common.ErrInvalidArgument, snapshot.TargetRef, snapshot.ContentState)
	}
	return snapshot, nil
}

type workspaceManifestObservation struct {
	manifest   runtimemodel.WorkspaceManifest
	entryCount int
}

func (a *RuntimeAdapter) AcquireWorkspaceManifest(workspaceDirectory string) (runtimemodel.WorkspaceManifest, error) {
	observation, err := a.acquireWorkspaceManifestObservation(workspaceDirectory)
	return observation.manifest, err
}

// acquireWorkspaceManifestObservation keeps the File owner's bounded list
// cardinality alongside the file-only manifest. Directories and safe internal
// symlinks consume the same ListFiles bound as files, so their canonical
// topology identity is part of Revision even though only regular files enter
// TargetRefs. A prospective create can then bind both target visibility and
// capacity to one exact owner observation.
func (a *RuntimeAdapter) acquireWorkspaceManifestObservation(workspaceDirectory string) (workspaceManifestObservation, error) {
	result, err := a.ListFiles(FileListRequest{
		WorkspaceDirectory: workspaceDirectory,
		Path:               ".",
		Recursive:          true,
	})
	if err != nil {
		return workspaceManifestObservation{}, err
	}
	if result.Truncated {
		return workspaceManifestObservation{}, fmt.Errorf("%w: workspace manifest exceeds bounded file listing", common.ErrInvalidArgument)
	}
	targetRefs := make([]string, 0, len(result.Entries))
	h := sha256.New()
	for _, entry := range result.Entries {
		ref := strings.TrimSpace(entry.Path)
		kind := strings.TrimSpace(entry.Kind)
		if ref == "" || kind == "" {
			continue
		}
		// Path + kind bind every entry that consumes the bounded File inventory.
		// Directory mtimes are deliberately excluded: the canonical child entry
		// set already represents topology and remains stable when an idempotent
		// file write merely touches its parent directory metadata.
		_, _ = fmt.Fprintf(h, "%d:%s|%d:%s|", len(ref), ref, len(kind), kind)
		switch kind {
		case "file":
			targetRefs = append(targetRefs, ref)
			_, _ = fmt.Fprintf(h, "%s|%d|%s;", entry.ContentHash, entry.Size, entry.ModifiedAt)
		case "symlink":
			// ListFiles resolves safe internal aliases to their current target ref.
			// Size/mtime additionally invalidate an alias replacement whose target
			// happens to normalize to the same path.
			_, _ = fmt.Fprintf(h, "%d|%s;", entry.Size, entry.ModifiedAt)
		default:
			_, _ = fmt.Fprint(h, ";")
		}
	}
	return workspaceManifestObservation{
		manifest: runtimemodel.WorkspaceManifest{
			Revision:   hex.EncodeToString(h.Sum(nil)),
			TargetRefs: targetRefs,
		},
		entryCount: len(result.Entries),
	}, nil
}
