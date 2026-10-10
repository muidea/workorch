package specifications

import (
	"fmt"

	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/definitionconfiguration"
)

func ParseAuthoringMapping(payload []byte) (workspace.DefinitionAuthoringMapping, error) {
	return workspace.DecodeDefinitionAuthoringMapping(payload)
}

func ParseAuthoringProposal(payload []byte, derivation workspace.DefinitionDerivationRequest) (workspace.GoalSpecification, error) {
	mapping, err := ParseAuthoringMapping(payload)
	if err != nil {
		return workspace.GoalSpecification{}, err
	}
	proposal, err := workspace.CompileDefinitionAuthoringMapping(derivation, mapping)
	if err != nil {
		return workspace.GoalSpecification{}, err
	}
	items, err := derivation.Proposal.CandidateItems(derivation.Input, &derivation.Template)
	if err != nil {
		return workspace.GoalSpecification{}, err
	}
	bindings, err := definitionconfiguration.Compile(&workspace.Workspace{}, &derivation.Template)
	if err != nil {
		return workspace.GoalSpecification{}, fmt.Errorf("compile sealed template profiles: %w", err)
	}
	if err := definitionadmission.ValidateAuthoringVerificationApplicability(proposal, items, bindings.ProfileContracts); err != nil {
		return workspace.GoalSpecification{}, err
	}
	return proposal, nil
}

func ParseAuthoringReview(payload []byte, kind string, proposal workspace.GoalSpecification, derivation workspace.DefinitionDerivationRequest) (workspace.GoalSpecificationCompletenessReview, error) {
	if derivation.Validate() != nil || derivation.Proposal == nil {
		return workspace.GoalSpecificationCompletenessReview{}, fmt.Errorf("authoring review requires completed derivation")
	}
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
