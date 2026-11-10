package workspace

import (
	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
)

func authoringSuccessorDraft(w *workspacemodel.Workspace) bool {
	if w == nil || w.DefinitionAuthoring == nil || w.DefinitionAuthoringConfirmation != nil || len(w.DefinitionAuthoringHistory) == 0 || len(w.DefinitionAdmissionCandidates) != len(w.DefinitionAuthoringHistory) || w.StartupInput != nil || w.ActivationAdmission != nil || w.ActiveRunID != "" || w.ReadCommittedAuthoring() != nil {
		return false
	}
	if _, present, err := w.ReadCommittedActivation(); err != nil || present {
		return false
	}
	candidate := w.DefinitionAdmissionCandidates[len(w.DefinitionAdmissionCandidates)-1]
	outcome, err := w.ReadDefinitionAdmissionResult(candidate)
	return err == nil && outcome.Disposition == workspacemodel.DefinitionAdmissionClarificationRequired
}

// blockedAuthoringSuccessor permits only an unsealed, materialized authoring
// candidate to be revised. It never treats a pending or eligible startup as a
// draft, and an invalid history cannot open a new provider budget.
func blockedAuthoringSuccessor(w *workspacemodel.Workspace) bool {
	if w == nil || w.DefinitionAuthoring == nil || w.DefinitionAuthoringConfirmation == nil || w.DefinitionAuthoringConfirmation.Phase != workspacemodel.DefinitionAuthoringConfirmationMaterialized || w.StartupInput != nil || w.ActivationAdmission != nil || w.ActiveRunID != "" || len(w.DefinitionAdmissionCandidates) != len(w.DefinitionAuthoringHistory)+1 || w.ReadCommittedAuthoring() != nil {
		return false
	}
	if w.DefinitionAuthoringConfirmation.ReadCommitted(w.DefinitionAuthoring, w.ID) != nil {
		return false
	}
	candidate := w.DefinitionAdmissionCandidates[len(w.DefinitionAdmissionCandidates)-1]
	if candidate.Validate() != nil || candidate.AuthoringRef == nil || candidate.CandidateRevision != uint64(len(w.DefinitionAdmissionCandidates)) {
		return false
	}
	if len(w.DefinitionAuthoringHistory) > 0 {
		previous := w.DefinitionAuthoringHistory[len(w.DefinitionAuthoringHistory)-1]
		if w.DefinitionAuthoringConfirmation.ValidateSuccessor(previous.Confirmation) != nil || candidate.CandidateID != w.DefinitionAdmissionCandidates[0].CandidateID {
			return false
		}
	}
	ref, err := w.DefinitionAuthoringConfirmation.CommittedAdmissionRef(w.DefinitionAuthoring, w.ID)
	if err != nil || *candidate.AuthoringRef != ref {
		return false
	}
	if _, present, err := w.ReadCommittedActivation(); err != nil || present {
		return false
	}
	outcome, err := w.ReadDefinitionAdmissionResult(candidate)
	return err == nil && outcome.Disposition == workspacemodel.DefinitionAdmissionClarificationRequired
}
