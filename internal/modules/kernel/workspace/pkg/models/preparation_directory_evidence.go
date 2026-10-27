package workspace

import (
	"encoding/json"
	"reflect"
	"time"

	filepreparation "github.com/muidea/workorch/internal/modules/blocks/file/pkg/preparation"
	"github.com/muidea/workorch/internal/pkg/common"
	"github.com/muidea/workorch/internal/pkg/fileobservation"
)

const PreparationDirectoryEvidenceVersion = "workorch-preparation-directory-evidence/v1"

// Historical material is separate from the public bounded operation record.
// Original bytes remain in owner storage. A checksum binds content; it does not
// authenticate an observation or grant readiness. Only private owner facts can
// introduce it via the original accepted -> blocked CAS.
type WorkspacePreparationDirectoryEvidence struct {
	// Only the original settlement can attach a File re-observation. Older
	// history remains absent; recovery must not manufacture a new window.
	RevalidationVersion       string                       `json:",omitempty"`
	Revalidation              *filepreparation.Observation `json:",omitempty"`
	Version                   string
	WorkspaceID               common.WorkspaceID
	RequestID, PayloadHash    string
	ObservedWorkspaceRevision uint64
	StartedAt, CompletedAt    time.Time
	Internal, Deliverable     fileobservation.Directory
	ContentHash               string
}

func (e WorkspacePreparationDirectoryEvidence) Clone() WorkspacePreparationDirectoryEvidence {
	e.Internal, e.Deliverable = e.Internal.Clone(), e.Deliverable.Clone()
	if e.Revalidation != nil {
		v := e.Revalidation.Clone()
		e.Revalidation = &v
	}
	return e
}

func (e WorkspacePreparationDirectoryEvidence) Hash() string {
	e.ContentHash = ""
	raw, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return fileobservation.Digest(raw)
}

func (e WorkspacePreparationDirectoryEvidence) Validate() error {
	if e.RevalidationVersion != "" && e.RevalidationVersion != filepreparation.Version || e.Revalidation != nil && (e.RevalidationVersion != filepreparation.Version || e.Revalidation.Validate() != nil || e.Revalidation.Scope.WorkspaceID != e.WorkspaceID || e.Revalidation.Scope.RequestID != e.RequestID || e.Revalidation.Scope.PayloadHash != e.PayloadHash || e.Revalidation.Scope.WorkspaceRevision != e.ObservedWorkspaceRevision || e.Revalidation.Scope.InternalDirectory != e.Internal.Path || e.Revalidation.Scope.DeliverableDirectory != e.Deliverable.Path || e.Revalidation.StartedAt.Before(e.CompletedAt)) {
		return common.ErrConflict
	}
	if e.Version != PreparationDirectoryEvidenceVersion || e.WorkspaceID == "" || !boundedPreparationIdentity(e.RequestID) || !boundedPreparationIdentity(e.PayloadHash) || e.ObservedWorkspaceRevision == 0 || e.StartedAt.IsZero() || e.CompletedAt.Before(e.StartedAt) || e.Internal.Validate() != nil || e.Deliverable.Validate() != nil || e.ContentHash != e.Hash() {
		return common.ErrConflict
	}
	return nil
}

func (e WorkspacePreparationDirectoryEvidence) BlockedReason() string {
	if !e.Internal.Complete || !e.Deliverable.Complete {
		return "directory_observation_incomplete"
	}
	if !e.Internal.Readable || !e.Internal.Writable || !e.Internal.Searchable || !e.Deliverable.Readable || !e.Deliverable.Writable || !e.Deliverable.Searchable {
		return "directory_permissions_denied"
	}
	return "directory_observed_remaining_checks"
}

func ClonePreparationDirectoryEvidence(items []WorkspacePreparationDirectoryEvidence) []WorkspacePreparationDirectoryEvidence {
	if items == nil {
		return nil
	}
	cloned := make([]WorkspacePreparationDirectoryEvidence, len(items))
	for i := range items {
		cloned[i] = items[i].Clone()
	}
	return cloned
}

func ValidatePreparationEvidencePersistence(w *Workspace) error {
	if err := validatePreparationAssessmentPersistence(w); err != nil {
		return err
	}
	if err := validatePreparationRunHistoryEvidencePersistence(w); err != nil {
		return err
	}
	if err := validatePreparationSealEvidencePersistence(w); err != nil {
		return err
	}
	if err := validatePreparationMCPRegistryEvidencePersistence(w); err != nil {
		return err
	}
	if err := validatePreparationCheckEvaluationPersistence(w); err != nil {
		return err
	}
	if err := validatePreparationMemoryInventoryEvidencePersistence(w); err != nil {
		return err
	}
	if err := validatePreparationProviderCacheEvidencePersistence(w); err != nil {
		return err
	}
	if err := ValidatePreparationSkillHubEffectsPersistence(w); err != nil {
		return err
	}
	if err := validatePreparationSkillRegistryEvidencePersistence(w); err != nil {
		return err
	}
	if err := validatePreparationSkillSelectionPersistence(w); err != nil {
		return err
	}
	if err := validatePreparationMCPSelectionPersistence(w); err != nil {
		return err
	}
	if err := validatePreparationEmptySelectionPersistence(w); err != nil {
		return err
	}
	if w == nil {
		return common.ErrInvalidArgument
	}
	if err := validatePreparationSoulEvidencePersistence(w); err != nil {
		return err
	}
	if err := validatePreparationResourceEvidencePersistence(w); err != nil {
		return err
	}
	if err := validatePreparationManagedConfigEvidencePersistence(w); err != nil {
		return err
	}
	if len(w.PreparationDirectoryEvidence) > WorkspacePreparationAttemptLimit {
		return common.ErrConflict
	}
	if len(w.PreparationDirectoryEvidence) != 0 && (w.StartupPreparation == nil || w.StartupPreparation.Version != WorkspacePreparationWorkflowVersion || w.StartupPreparation.Validate(w.ID) != nil) {
		return common.ErrConflict
	}
	refs := make(map[string]WorkspacePreparationAttempt)
	if w.StartupPreparation != nil {
		for _, a := range w.StartupPreparation.Attempts {
			if a.DirectoryEvidenceHash != "" {
				if _, duplicate := refs[a.DirectoryEvidenceHash]; duplicate {
					return common.ErrConflict
				}
				refs[a.DirectoryEvidenceHash] = a
			}
		}
	}
	if len(refs) != len(w.PreparationDirectoryEvidence) {
		return common.ErrConflict
	}
	for _, e := range w.PreparationDirectoryEvidence {
		a, ok := refs[e.ContentHash]
		if !ok || e.Validate() != nil || e.WorkspaceID != w.ID || e.RequestID != a.RequestID || e.PayloadHash != a.PayloadHash || e.ObservedWorkspaceRevision != a.ExpectedWorkspaceRevision+1 || e.StartedAt.Before(a.StartedAt) || e.CompletedAt.After(a.CompletedAt) || a.State != WorkspacePreparationBlocked {
			return common.ErrConflict
		}
		if e.Revalidation != nil && (e.Revalidation.ObservedAt.After(a.CompletedAt) || validatePreparationFileRevalidationSources(w, a, e) != nil) {
			return common.ErrConflict
		}
		delete(refs, e.ContentHash)
	}
	return nil
}

func validatePreparationEvidenceTransition(current, next *Workspace, cas bool) error {
	if err := validatePreparationRunHistoryEvidenceTransition(current, next, cas); err != nil {
		return err
	}
	if err := validatePreparationSealEvidenceTransition(current, next, cas); err != nil {
		return err
	}
	if err := validatePreparationMCPRegistryEvidenceTransition(current, next, cas); err != nil {
		return err
	}
	if err := validatePreparationMemoryInventoryEvidenceTransition(current, next, cas); err != nil {
		return err
	}
	if err := validatePreparationProviderCacheEvidenceTransition(current, next, cas); err != nil {
		return err
	}
	if err := validatePreparationSkillHubEffectsTransition(current, next, cas); err != nil {
		return err
	}
	if err := validatePreparationSkillRegistryEvidenceTransition(current, next, cas); err != nil {
		return err
	}
	if err := validatePreparationSoulEvidenceTransition(current, next, cas); err != nil {
		return err
	}
	if err := validatePreparationManagedConfigEvidenceTransition(current, next, cas); err != nil {
		return err
	}
	if err := validatePreparationResourceEvidenceTransition(current, next, cas); err != nil {
		return err
	}
	if ValidatePreparationEvidencePersistence(next) != nil {
		return common.ErrConflict
	}
	if current == nil {
		if len(next.PreparationDirectoryEvidence) != 0 {
			return common.ErrConflict
		}
		return nil
	}
	if ValidatePreparationEvidencePersistence(current) != nil {
		return common.ErrConflict
	}
	before, after := current.PreparationDirectoryEvidence, next.PreparationDirectoryEvidence
	if reflect.DeepEqual(before, after) {
		return nil
	}
	if !cas || len(after) != len(before)+1 || len(before) != 0 && !reflect.DeepEqual(before, after[:len(before)]) || current.StartupPreparation == nil || next.StartupPreparation == nil || len(current.StartupPreparation.Attempts) == 0 || len(next.StartupPreparation.Attempts) == 0 {
		return common.ErrConflict
	}
	a := current.StartupPreparation.Attempts[len(current.StartupPreparation.Attempts)-1]
	b := next.StartupPreparation.Attempts[len(next.StartupPreparation.Attempts)-1]
	if a.State != WorkspacePreparationAccepted && a.State != WorkspacePreparationUnknown || b.State != WorkspacePreparationBlocked || b.RequestID != a.RequestID || a.DirectoryEvidenceHash != "" || b.DirectoryEvidenceHash != after[len(after)-1].ContentHash {
		return common.ErrConflict
	}
	return nil
}
