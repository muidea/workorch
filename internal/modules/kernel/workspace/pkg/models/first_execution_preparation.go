package workspace

import (
	filepreparation "github.com/muidea/workorch/internal/modules/blocks/file/pkg/preparation"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

const FirstExecutionPreparationEvidenceVersion = "workorch-first-execution-preparation-evidence/v1"

// Classification only. An advanced epoch or rejected intent is not execution
// history. The owner independently validates the fence and original receipts.
func RequiresFirstExecutionPreparation(w *Workspace) bool {
	if w == nil || w.ActiveRunID != "" {
		return false
	}
	if w.PreparationFence != nil {
		for _, settled := range w.PreparationFence.Settled {
			if settled.Receipt.Admission.Reason == runtimecontract.RuntimeExecutionIntentReasonNewRunActivated && settled.Receipt.Admission.RunID != "" {
				return false
			}
		}
	}
	return true
}

// Complete File-owner observation used by the private reservation CAS. It is
// restricted historical material, not a caller credential or current lease.
type WorkspaceFirstExecutionPreparationEvidence struct {
	Version     string
	WorkspaceID common.WorkspaceID
	RequestID   string
	Files       filepreparation.Observation
}

func (e *WorkspaceFirstExecutionPreparationEvidence) Clone() *WorkspaceFirstExecutionPreparationEvidence {
	if e == nil {
		return nil
	}
	next := *e
	next.Files = e.Files.Clone()
	return &next
}
