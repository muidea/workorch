package workspace

import (
	"reflect"
	"time"

	"github.com/muidea/workorch/internal/pkg/common"
)

// Bind the entire re-observed scope to the original archives, including order,
// empty sets and exact roots. A caller digest cannot replace raw equality.
func validatePreparationFileRevalidationSources(w *Workspace, a WorkspacePreparationAttempt, d WorkspacePreparationDirectoryEvidence) error {
	o := d.Revalidation
	if o == nil || o.Validate() != nil {
		return common.ErrConflict
	}
	r, c := 0, 0
	for _, e := range w.PreparationResourceEvidence {
		if e.ContentHash != a.ResourceEvidenceHash {
			continue
		}
		r++
		if e.RequestID != a.RequestID || e.PayloadHash != a.PayloadHash || e.WorkspaceID != w.ID || e.ObservedWorkspaceRevision != o.Scope.WorkspaceRevision || e.DeliverableDirectory != o.Scope.DeliverableDirectory || len(e.Resources) != len(o.Scope.ResourceRefs) || o.StartedAt.Before(e.CompletedAt) {
			return common.ErrConflict
		}
		for i, item := range e.Resources {
			if item.Observation.Ref != o.Scope.ResourceRefs[i] {
				return common.ErrConflict
			}
		}
	}
	for _, e := range w.PreparationManagedConfigEvidence {
		if e.Ref != a.ManagedConfigEvidenceRef {
			continue
		}
		c++
		if e.RequestID != a.RequestID || e.PayloadHash != a.PayloadHash || e.WorkspaceID != w.ID || e.ObservedWorkspaceRevision != o.Scope.WorkspaceRevision || e.InternalDirectory != o.Scope.InternalDirectory || len(e.Files) != len(o.Scope.Configs) || o.StartedAt.Before(e.CompletedAt) {
			return common.ErrConflict
		}
		for i, item := range e.Files {
			if item.Path != o.Scope.Configs[i].Path || item.Scope != o.Scope.Configs[i].Scope {
				return common.ErrConflict
			}
		}
	}
	if r != 1 || c != 1 {
		return common.ErrConflict
	}
	return nil
}

// This is a File-only age/current-known-drift check, never whole readiness.
// epoch=="" means the caller has no current File owner observation: fail closed.
// It never extends expiry, changes history, or performs filesystem I/O.
func PreparationFileRevalidationStale(w *Workspace, now time.Time, epoch string) (bool, error) {
	if w == nil || now.IsZero() {
		return false, common.ErrInvalidArgument
	}
	if w.StartupPreparation == nil || len(w.StartupPreparation.Attempts) == 0 {
		return false, nil
	}
	a := w.StartupPreparation.Attempts[len(w.StartupPreparation.Attempts)-1]
	if a.DirectoryEvidenceHash == "" {
		return false, nil
	}
	var original *WorkspacePreparationDirectoryEvidence
	for i := range w.PreparationDirectoryEvidence {
		if w.PreparationDirectoryEvidence[i].ContentHash == a.DirectoryEvidenceHash {
			if original != nil {
				return false, common.ErrConflict
			}
			original = &w.PreparationDirectoryEvidence[i]
		}
	}
	if original == nil || original.Validate() != nil {
		return false, common.ErrConflict
	}
	for _, d := range w.PreparationDirectoryEvidence {
		if d.ContentHash != a.DirectoryEvidenceHash {
			continue
		}
		if d.RevalidationVersion == "" {
			return false, nil
		} // no upgrade of old histories
		if d.Validate() != nil {
			return false, common.ErrConflict
		}
		o := d.Revalidation
		if o == nil {
			return true, nil
		}
		if validatePreparationFileRevalidationSources(w, a, d) != nil {
			return false, common.ErrConflict
		}
		if !reflect.DeepEqual(d.Internal, o.Internal) || !reflect.DeepEqual(d.Deliverable, o.Deliverable) {
			return true, nil
		}
		for _, e := range w.PreparationResourceEvidence {
			if e.ContentHash == a.ResourceEvidenceHash {
				for i, item := range e.Resources {
					if !reflect.DeepEqual(item.Observation, o.Resources[i]) {
						return true, nil
					}
				}
			}
		}
		for _, e := range w.PreparationManagedConfigEvidence {
			if e.Ref == a.ManagedConfigEvidenceRef {
				for i, item := range e.Files {
					if !reflect.DeepEqual(item.Observation, o.Configs[i]) {
						return true, nil
					}
				}
			}
		}
		return now.Before(o.ObservedAt) || !now.Before(o.ExpiresAt) || epoch != o.OwnerEpoch, nil
	}
	return false, nil
}

func PreparationFileObservationEpoch(w *Workspace) string {
	if w != nil && w.StartupPreparation != nil && len(w.StartupPreparation.Attempts) > 0 {
		a := w.StartupPreparation.Attempts[len(w.StartupPreparation.Attempts)-1]
		for _, d := range w.PreparationDirectoryEvidence {
			if d.ContentHash == a.DirectoryEvidenceHash && d.Revalidation != nil {
				return d.Revalidation.OwnerEpoch
			}
		}
	}
	return ""
}
