package specifications

import (
	"fmt"
	"slices"

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
	CandidateItemRef            string                                        `json:"candidateItemRef"`
	SourceScope                 string                                        `json:"sourceScope"`
	SourceID                    string                                        `json:"sourceId"`
	DeclarationBindingMode      string                                        `json:"declarationBindingMode"`
	Declarations                []authoringVerificationDeclaration            `json:"declarations"`
	MergedRequiredEvidenceKinds []string                                      `json:"mergedRequiredEvidenceKinds"`
	MergedRequiredConsumers     []runtimecontract.RuntimeLineageAssertionRule `json:"mergedRequiredConsumers"`
}

const authoringSingleDeclarationBinding = "one_applicable_reference"

// This is source applicability, not semantic feasibility or verification credit.
// Reuse the receiving owner's after-image source resolver and merged-policy gate.
func authoringVerificationOptions(items []workspace.DefinitionCandidateItem, profiles []runtimecontract.RuntimeProfileContract) ([]authoringCandidateVerificationOptions, error) {
	result := make([]authoringCandidateVerificationOptions, 0, len(items))
	sources, valid := definitionadmission.SpecificationDoDSources(items)
	if !valid {
		return nil, fmt.Errorf("authoring verification options require exact candidate source order")
	}
	for _, item := range items {
		policy, err := runtimecontract.CompileRuntimeSourceEvidencePolicy("dodItem", sources[item.CandidateItemRef], profiles, nil)
		if err != nil {
			return nil, fmt.Errorf("authoring verification policy for %s: %w", item.CandidateItemRef, err)
		}
		options := authoringCandidateVerificationOptions{
			CandidateItemRef: item.CandidateItemRef, SourceScope: "dodItem", SourceID: sources[item.CandidateItemRef],
			DeclarationBindingMode: authoringSingleDeclarationBinding, Declarations: []authoringVerificationDeclaration{},
			MergedRequiredEvidenceKinds: append([]string{}, policy.RequiredEvidenceKinds...),
			MergedRequiredConsumers:     slices.Clone(policy.LineageAssertions),
		}
		if options.MergedRequiredConsumers == nil {
			options.MergedRequiredConsumers = []runtimecontract.RuntimeLineageAssertionRule{}
		}
		spec := workspace.GoalSpecification{Obligations: []workspace.GoalSpecificationObligation{{ID: "source", DoDItemRefs: []string{item.CandidateItemRef}}}}
		for _, profile := range profiles {
			for _, declaration := range profile.EvidenceRequirements {
				verification := workspace.GoalSpecificationVerification{ObligationID: "source", ProfileID: profile.ID, EvidenceRequirementID: declaration.ID, Scope: declaration.Scope}
				if definitionadmission.SpecificationVerificationApplies(spec, verification, items, profiles) {
					options.Declarations = append(options.Declarations, authoringVerificationDeclaration{ProfileID: profile.ID, EvidenceRequirementID: declaration.ID, Scope: declaration.Scope})
				}
			}
		}
		result = append(result, options)
	}
	return result, nil
}
