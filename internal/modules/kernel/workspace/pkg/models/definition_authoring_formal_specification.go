package workspace

import (
	"fmt"

	"github.com/muidea/workorch/internal/pkg/common"
)

// ConfirmedAuthoringCandidateInputs keeps the item references and six reviews
// exactly as they were reviewed. The formal Goal/DoD revisions are bound as
// source identities; the startup contract maps candidate refs to sealed item
// refs only after admission. A change of references here would create a new
// specification hash that the original reviewers never judged.
func ConfirmedAuthoringCandidateInputs(record *DefinitionAuthoringRecord, confirmation DefinitionAuthoringConfirmation, workspaceID common.WorkspaceID) (DefinitionCandidateItem, []DefinitionCandidateItem, GoalSpecification, error) {
	goal, items, reviewed, _, err := ConfirmedAuthoringAdmissionSource(record, confirmation, workspaceID)
	return goal, items, reviewed, err
}

// ConfirmedAuthoringAdmissionSource 在同一次完整来源校验中派生正式输入和准入引用。
// 返回值只含数据；不暴露可跨调用复用的校验凭据。
func ConfirmedAuthoringAdmissionSourceView(record *DefinitionAuthoringRecord, confirmation DefinitionAuthoringConfirmation, workspaceID common.WorkspaceID) (DefinitionCandidateItem, []DefinitionCandidateItem, GoalSpecification, DefinitionAuthoringAdmissionRef, error) {
	return confirmedAuthoringAdmissionSourceMode(record, confirmation, workspaceID, true)
}
func ConfirmedAuthoringAdmissionSource(record *DefinitionAuthoringRecord, confirmation DefinitionAuthoringConfirmation, workspaceID common.WorkspaceID) (DefinitionCandidateItem, []DefinitionCandidateItem, GoalSpecification, DefinitionAuthoringAdmissionRef, error) {
	return confirmedAuthoringAdmissionSourceMode(record, confirmation, workspaceID, false)
}
func confirmedAuthoringAdmissionSourceMode(record *DefinitionAuthoringRecord, confirmation DefinitionAuthoringConfirmation, workspaceID common.WorkspaceID, view bool) (DefinitionCandidateItem, []DefinitionCandidateItem, GoalSpecification, DefinitionAuthoringAdmissionRef, error) {
	if record == nil || confirmation.Phase != DefinitionAuthoringConfirmationMaterialized {
		return DefinitionCandidateItem{}, nil, GoalSpecification{}, DefinitionAuthoringAdmissionRef{}, fmt.Errorf("authoring candidate requires exact materialized confirmation")
	}
	reviewed, items, err := confirmation.validatedSpecificationMode(record, workspaceID, view)
	if err != nil {
		return DefinitionCandidateItem{}, nil, GoalSpecification{}, DefinitionAuthoringAdmissionRef{}, err
	}
	goal := DefinitionCandidateItem{
		CandidateItemRef: DefinitionAuthoringGoalItemRef,
		Text:             confirmation.Goal.Text,
		SourceIdentity:   "goal:" + string(confirmation.Goal.ID) + "@" + confirmation.Goal.DefinitionRevision,
	}
	for index := range items {
		items[index].Text = confirmation.DoD.Items[index]
		items[index].SourceIdentity = "dod:" + string(confirmation.DoD.ID) + "@" + confirmation.DoD.DefinitionRevision
	}
	if err := reviewed.Validate(goal, items); err != nil {
		return DefinitionCandidateItem{}, nil, GoalSpecification{}, DefinitionAuthoringAdmissionRef{}, err
	}
	ref := DefinitionAuthoringAdmissionRef{
		InputHash:          confirmation.InputHash,
		DerivationPermitID: confirmation.DerivationPermitID,
		ProposalHash:       confirmation.ProposalHash,
		SpecificationHash:  confirmation.SpecificationHash,
	}
	return goal, items, reviewed, ref, nil
}

func ConfirmedAuthoringAdmissionRef(record *DefinitionAuthoringRecord, confirmation DefinitionAuthoringConfirmation, workspaceID common.WorkspaceID) (DefinitionAuthoringAdmissionRef, error) {
	_, _, _, ref, err := ConfirmedAuthoringAdmissionSource(record, confirmation, workspaceID)
	return ref, err
}
