package specifications

import (
	"fmt"

	workspace "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

type authoringRequiredEvidencePurpose struct {
	Source                     runtimecontract.RuntimeEvidencePurposeMapping `json:"source"`
	EligibleAcceptanceItemRefs []string                                      `json:"eligibleAcceptanceItemRefs"`
}

func authoringRequiredEvidencePurposes(items []workspace.DefinitionCandidateItem, profiles []runtimecontract.RuntimeProfileContract) ([]authoringRequiredEvidencePurpose, error) {
	required, err := runtimecontract.RequiredRuntimeEvidencePurposes(profiles)
	if err != nil {
		return nil, err
	}
	sources, ok := workspace.SpecificationDoDSources(items)
	if !ok {
		return nil, fmt.Errorf("purpose catalog requires exact original sources")
	}
	var result []authoringRequiredEvidencePurpose
	for _, source := range required {
		row := authoringRequiredEvidencePurpose{Source: source, EligibleAcceptanceItemRefs: []string{}}
		for _, item := range items {
			// Full historical/current declaration qualification, without claiming readiness.
			m := source
			m.AcceptanceItemRef = item.CandidateItemRef
			// Other required sources are not selected here. They remain explicitly unchosen.
			if runtimecontract.RuntimeEvidencePurposeAcceptanceEligible(m, profiles, sources[item.CandidateItemRef]) {
				row.EligibleAcceptanceItemRefs = append(row.EligibleAcceptanceItemRefs, item.CandidateItemRef)
			}
		}
		result = append(result, row)
	}
	return result, nil
}
