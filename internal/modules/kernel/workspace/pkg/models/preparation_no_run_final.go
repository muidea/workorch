package workspace

import (
	"github.com/muidea/workorch/internal/pkg/common"
)

const PreparationNoRunFinalVersion = "workorch-preparation-no-run-final-fence/v1"
const PreparationNoRunFinalReason = "run_history_empty_final_fence_observed"

// Exact Workspace preimage of the original settlement CAS. Stored only in the
// private Run-history archive. Not an activation permit or an M3 completion.
type PreparationNoRunFinalFence struct {
	Version           string
	WorkspaceRevision uint64
	ActiveRunID       common.RunID
	Fence             WorkspacePreparationFence
}

func (f PreparationNoRunFinalFence) Clone() PreparationNoRunFinalFence {
	f.Fence = *f.Fence.Clone()
	return f
}

func (f PreparationNoRunFinalFence) validFor(id common.WorkspaceID) bool {
	return f.Version == PreparationNoRunFinalVersion && f.WorkspaceRevision > 0 && f.ActiveRunID == "" && f.Fence.Validate(id) == nil && f.Fence.PreparationEpoch == 0 && f.Fence.Pending == nil && len(f.Fence.Settled) == 0
}

func validPreparationNoRunFinalShape(a WorkspacePreparationAttempt) bool {
	if a.NoRunEvaluationVersion != PreparationNoRunFinalVersion || a.State != WorkspacePreparationBlocked || a.EvaluationVersion != PreparationCheckEvaluationVersion || a.PreparationEpoch != 0 || !ValidPreparationRunHistoryRef(a.RunHistoryEvidenceRef) || !ValidPreparationSealRef(a.SealEvidenceRef) || a.ExpectedWorkspaceRevision > ^uint64(0)-2 || a.CommittedWorkspaceRevision != a.ExpectedWorkspaceRevision+2 {
		return false
	}
	count := 0
	for _, c := range a.Checks {
		if c.ID == "first_activation_no_run" || c.ID == "exact_seal" || c.ID == "persistence_integrity" {
			if c.State != "passed" || !ValidPreparationEvaluatedCheck(a, c) {
				return false
			}
			count++
		}
	}
	return count == 3
}

func preparationNoRunSealComplete(w *Workspace, a WorkspacePreparationAttempt) bool {
	if a.State != WorkspacePreparationBlocked || a.EvaluationVersion != PreparationCheckEvaluationVersion || a.SealEvidenceRef == "" || a.PreparationEpoch != 0 {
		return false
	}
	count := 0
	for _, c := range a.Checks {
		if c.ID == "exact_seal" || c.ID == "persistence_integrity" {
			if c.State != "passed" || !ValidPreparationEvaluatedCheck(a, c) {
				return false
			}
			count++
		}
	}
	if count != 2 || w == nil {
		return false
	}
	for _, seal := range w.PreparationSealEvidence {
		if seal.Ref == a.SealEvidenceRef && seal.validateFor(w, a) == nil {
			return true
		}
	}
	return false
}

func (e WorkspacePreparationRunHistoryEvidence) validFinalFor(w *Workspace, a WorkspacePreparationAttempt) bool {
	f := e.FinalFence
	return w != nil && f != nil && f.validFor(w.ID) && validPreparationNoRunFinalShape(a) && f.WorkspaceRevision == a.ExpectedWorkspaceRevision+1 && a.CommittedWorkspaceRevision == f.WorkspaceRevision+1 && f.Fence.CreationEpoch == a.CreationEpoch && e.Facts.WorkspaceRevision == f.WorkspaceRevision && e.Facts.BlockedReason() == "run_history_empty_fence_not_final" && preparationNoRunSealComplete(w, a)
}

// Only the original private settlement uses this pure compiler. The repository
// transition gate independently compares the saved preimage to its actual CAS
// preimage. Missing/stale/legacy/unknown inputs never obtain a final fence.
func CompilePreparationNoRunFinalFence(current, next *Workspace, a WorkspacePreparationAttempt) *PreparationNoRunFinalFence {
	if current == nil || next == nil || current.ID != next.ID || a.ExpectedWorkspaceRevision > ^uint64(0)-2 || current.Revision != a.ExpectedWorkspaceRevision+1 || current.Revision == ^uint64(0) || current.PreparationFence == nil || current.StartupPreparation == nil || current.StartupPreparation.Version != WorkspacePreparationWorkflowVersion || len(current.StartupPreparation.Attempts) == 0 || !preparationNoRunSealComplete(next, a) {
		return nil
	}
	original := current.StartupPreparation.Attempts[len(current.StartupPreparation.Attempts)-1]
	if original.State != WorkspacePreparationAccepted || original.RequestID != a.RequestID || original.PayloadHash != a.PayloadHash || original.Binding != a.Binding || original.CreationEpoch != a.CreationEpoch || original.PreparationEpoch != a.PreparationEpoch || original.CommittedWorkspaceRevision != current.Revision {
		return nil
	}
	f := PreparationNoRunFinalFence{Version: PreparationNoRunFinalVersion, WorkspaceRevision: current.Revision, ActiveRunID: current.ActiveRunID, Fence: *current.PreparationFence.Clone()}
	if !f.validFor(current.ID) || f.Fence.CreationEpoch != a.CreationEpoch {
		return nil
	}
	for _, e := range next.PreparationRunHistoryEvidence {
		if e.Ref == a.RunHistoryEvidenceRef && e.FinalFence == nil && e.Validate() == nil && e.RequestID == a.RequestID && e.PayloadHash == a.PayloadHash && e.Binding == a.Binding && e.CreationEpoch == a.CreationEpoch && e.PreparationEpoch == a.PreparationEpoch && e.Facts.Observation.History.WorkspaceID == current.ID && e.Facts.WorkspaceRevision == current.Revision && e.Facts.BlockedReason() == "run_history_empty_fence_not_final" {
			return &f
		}
	}
	return nil
}
