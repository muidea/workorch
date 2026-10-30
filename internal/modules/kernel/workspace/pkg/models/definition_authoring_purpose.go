package workspace

import (
	"fmt"

	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

// Compile only machine fields from the exact sealed derivation. The model and
// human choose the semantic edge; neither can supply a replacement snapshot.
func compileAuthoringEvidencePurposes(d DefinitionDerivationRequest, m DefinitionAuthoringMapping, items []DefinitionCandidateItem) ([]GoalEvidencePurposeMapping, error) {
	profiles, err := AuthoringProfileContracts(d)
	if err != nil {
		return nil, err
	}
	required, err := runtimecontract.RequiredRuntimeEvidencePurposes(profiles)
	if err != nil {
		return nil, err
	}
	catalog := map[string]runtimecontract.RuntimeEvidencePurposeMapping{}
	for _, r := range required {
		catalog[r.HistoricalItemRef] = r
	}
	result := append([]GoalEvidencePurposeMapping(nil), m.EvidencePurposeMappings...)
	seen := map[string]bool{}
	for _, selection := range m.ProfileAcceptanceSelections {
		source, ok := catalog[selection.SourceRef]
		if d.WireContractRef != DefinitionAuthoringOwnerBoundV3 || !ok || seen[selection.SourceRef] {
			return nil, fmt.Errorf("required_evidence_purpose_selection: unknown, duplicate or unsupported source %q", selection.SourceRef)
		}
		seen[selection.SourceRef] = true
		source.AcceptanceItemRef = selection.AcceptanceItemRef
		result = append(result, GoalEvidencePurposeMapping(source))
	}
	// A v3 required source must use the minimal selection, never full model metadata.
	if d.WireContractRef == DefinitionAuthoringOwnerBoundV3 {
		for _, r := range required {
			if !seen[r.HistoricalItemRef] {
				return nil, fmt.Errorf("required_evidence_purpose_missing: %s requires semantic selection", r.HistoricalItemRef)
			}
		}
	}
	sources, ok := SpecificationDoDSources(items)
	if !ok {
		return nil, fmt.Errorf("purpose selection requires exact original source order")
	}
	inventory := runtimecontract.RuntimeSourceObligationInventory{}
	for _, item := range items {
		inventory.Sources = append(inventory.Sources, runtimecontract.RuntimeDefinitionSource{CandidateItemRef: item.CandidateItemRef, SourceScope: "dodItem", SourceID: sources[item.CandidateItemRef], Text: item.Text})
	}
	for _, r := range result {
		inventory.EvidencePurposeMappings = append(inventory.EvidencePurposeMappings, runtimecontract.RuntimeEvidencePurposeMapping(r))
	}
	if err := runtimecontract.ValidateRuntimeEvidencePurposeMappings(inventory, profiles); err != nil {
		return nil, err
	}
	return result, nil
}
