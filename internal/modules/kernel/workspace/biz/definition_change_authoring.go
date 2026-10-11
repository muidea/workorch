package workspace

import (
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
)

// 新 reserve 必须显式选中输入；completion 只按原持久 permit 选择已接纳的 ledger。
func authoringMutationRecord(w *workspace.Workspace, inputHash, permitID string, specification bool) (*workspace.DefinitionAuthoringRecord, bool, error) {
	if w == nil || w.DefinitionAuthoring == nil || w.DefinitionSpecificationLedger != nil {
		return nil, false, common.ErrConflict
	}
	if w.ActiveRunID != "" && permitID == "" {
		return nil, false, common.ErrConflict
	}
	candidate := w.DefinitionChangeCandidate
	if candidate != nil {
		if candidate.Validate(w) != nil {
			return nil, false, common.ErrConflict
		}
		selected := inputHash != "" && inputHash == candidate.Record.InputHash
		if permitID != "" {
			if specification {
				for _, request := range candidate.Record.SpecificationRequests {
					if request.PermitID == permitID {
						for _, derivation := range candidate.Record.DerivationRequests {
							selected = selected || derivation.PermitID == request.DerivationPermitID && derivation.Input.InputHash() == candidate.Record.InputHash
						}
					}
				}
			} else {
				for _, request := range candidate.Record.DerivationRequests {
					selected = selected || request.PermitID == permitID && request.Input.InputHash() == candidate.Record.InputHash
				}
			}
		}
		if selected {
			return &candidate.Record, true, nil
		}
		// 独立候选存在时不再接纳原输入的新工作，旧 exact completion 仍可读回。
		if permitID == "" {
			return nil, false, common.ErrConflict
		}
	}
	if w.StartupInput != nil || w.ActivationAdmission != nil || w.DefinitionAuthoringConfirmation != nil || len(w.DefinitionAdmissionCandidates) > 0 && !authoringSuccessorDraft(w) {
		return nil, false, common.ErrConflict
	}
	if inputHash != "" && inputHash != w.DefinitionAuthoring.InputHash {
		return nil, false, common.ErrConflict
	}
	return w.DefinitionAuthoring, false, nil
}

func withAuthoringMutationRecord(w *workspace.Workspace, record workspace.DefinitionAuthoringRecord, candidate bool) *workspace.Workspace {
	next := *w
	if candidate {
		change := w.DefinitionChangeCandidate.Clone()
		change.Record = record
		next.DefinitionChangeCandidate = &change
	} else {
		next.DefinitionAuthoring = &record
	}
	return &next
}
