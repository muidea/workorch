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
	if record == nil || confirmation.Phase != DefinitionAuthoringConfirmationMaterialized || confirmation.Validate(record, workspaceID) != nil {
		return DefinitionCandidateItem{}, nil, GoalSpecification{}, fmt.Errorf("authoring candidate requires exact materialized confirmation")
	}
	derivation, reviewed, err := record.ConfirmableSpecification(confirmation.DerivationPermitID, confirmation.ProposalHash)
	if err != nil || reviewed.AdmissionHash() != confirmation.SpecificationHash {
		return DefinitionCandidateItem{}, nil, GoalSpecification{}, fmt.Errorf("authoring candidate review no longer matches confirmation")
	}
	items, err := derivation.Proposal.CandidateItems(derivation.Input, &derivation.Template)
	if err != nil || len(items) != len(confirmation.DoD.Items) {
		return DefinitionCandidateItem{}, nil, GoalSpecification{}, fmt.Errorf("authoring candidate lost its derived items")
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
		return DefinitionCandidateItem{}, nil, GoalSpecification{}, err
	}
	return goal, items, reviewed, nil
}

func ConfirmedAuthoringAdmissionRef(record *DefinitionAuthoringRecord, confirmation DefinitionAuthoringConfirmation, workspaceID common.WorkspaceID) (DefinitionAuthoringAdmissionRef, error) {
	if _, _, _, err := ConfirmedAuthoringCandidateInputs(record, confirmation, workspaceID); err != nil {
		return DefinitionAuthoringAdmissionRef{}, err
	}
	return DefinitionAuthoringAdmissionRef{
		InputHash:          confirmation.InputHash,
		DerivationPermitID: confirmation.DerivationPermitID,
		ProposalHash:       confirmation.ProposalHash,
		SpecificationHash:  confirmation.SpecificationHash,
	}, nil
}
