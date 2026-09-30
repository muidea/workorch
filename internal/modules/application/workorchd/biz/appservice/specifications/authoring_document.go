package specifications

import (
	"fmt"

	workspace "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
)

func ParseAuthoringProposal(payload []byte, derivation workspace.DefinitionDerivationRequest) (workspace.GoalSpecification, error) {
	if derivation.Validate() != nil || derivation.Proposal == nil {
		return workspace.GoalSpecification{}, fmt.Errorf("authoring proposal requires completed derivation")
	}
	goal := workspace.DefinitionCandidateItem{CandidateItemRef: workspace.DefinitionAuthoringGoalItemRef, Text: derivation.Input.GoalText, SourceIdentity: derivation.Input.InputHash()}
	items, err := derivation.Proposal.CandidateItems(derivation.Input, &derivation.Template)
	if err != nil {
		return workspace.GoalSpecification{}, err
	}
	proposal, err := ParseProposal(payload, goal, items)
	if err != nil {
		return workspace.GoalSpecification{}, err
	}
	if err := workspace.ValidateAuthoringSpecification(derivation.Input, &derivation.Template, *derivation.Proposal, derivation.Resources, proposal); err != nil {
		return workspace.GoalSpecification{}, err
	}
	return proposal, nil
}

func ParseAuthoringReview(payload []byte, kind string, proposal workspace.GoalSpecification, derivation workspace.DefinitionDerivationRequest) (workspace.GoalSpecificationCompletenessReview, error) {
	review, err := ParseReview(payload, kind, proposal)
	if err != nil {
		return workspace.GoalSpecificationCompletenessReview{}, err
	}
	items, err := derivation.Proposal.CandidateItems(derivation.Input, &derivation.Template)
	if err != nil {
		return workspace.GoalSpecificationCompletenessReview{}, err
	}
	seed := proposal.Clone()
	seed.Reviews = nil
	for _, allowed := range ReviewKinds() {
		entry := workspace.GoalSpecificationCompletenessReview{Kind: allowed, Status: workspace.GoalReviewUnknown, SpecificationHash: proposal.ContentHash()}
		if allowed == kind {
			entry = review
		}
		seed.Reviews = append(seed.Reviews, entry)
	}
	goal := workspace.DefinitionCandidateItem{CandidateItemRef: workspace.DefinitionAuthoringGoalItemRef, Text: derivation.Input.GoalText, SourceIdentity: derivation.Input.InputHash()}
	if err := seed.Validate(goal, items); err != nil {
		return workspace.GoalSpecificationCompletenessReview{}, err
	}
	return review, nil
}
