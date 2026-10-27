package specifications

import (
	"github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/definitionadmission"
	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type authoringVerificationDeclaration struct {
	ProfileID             string `json:"profileId"`
	EvidenceRequirementID string `json:"evidenceRequirementId"`
	Scope                 string `json:"scope"`
}

type authoringCandidateVerificationOptions struct {
	CandidateItemRef string                             `json:"candidateItemRef"`
	Declarations     []authoringVerificationDeclaration `json:"declarations"`
}

// This is source applicability, not semantic feasibility or verification credit.
// Reuse the receiving owner's after-image source resolver and merged-policy gate.
func authoringVerificationOptions(items []workspace.DefinitionCandidateItem, profiles []runtimecontract.RuntimeProfileContract) []authoringCandidateVerificationOptions {
	result := make([]authoringCandidateVerificationOptions, 0, len(items))
	for _, item := range items {
		options := authoringCandidateVerificationOptions{CandidateItemRef: item.CandidateItemRef, Declarations: []authoringVerificationDeclaration{}}
		spec := workspace.GoalSpecification{Obligations: []workspace.GoalSpecificationObligation{{ID: "source", DoDItemRefs: []string{item.CandidateItemRef}}}}
		for _, profile := range profiles {
			for _, declaration := range profile.EvidenceRequirements {
				verification := workspace.GoalSpecificationVerification{ObligationID: "source", ProfileID: profile.ID, EvidenceRequirementID: declaration.ID, Scope: declaration.Scope}
				if definitionadmission.SpecificationVerificationApplies(spec, verification, items, profiles) {
					options.Declarations = append(options.Declarations, authoringVerificationDeclaration{profile.ID, declaration.ID, declaration.Scope})
				}
			}
		}
		result = append(result, options)
	}
	return result
}
