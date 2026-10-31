package workspace

import (
	"fmt"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
	"strings"
)

func validateGoalEvidencePurposeMappings(mappings []GoalEvidencePurposeMapping, dod []DefinitionCandidateItem) error {
	if len(mappings) > 512 {
		return fmt.Errorf("evidence purpose mappings exceed bound")
	}
	refs := map[string]int{}
	for _, item := range dod {
		refs[item.CandidateItemRef]++
	}
	seen := map[string]bool{}
	for _, mapping := range mappings {
		validHistory := mapping.ContractRef == runtimecontract.RuntimeCrossCWIEvidencePurpose && refs[mapping.HistoricalItemRef] == 1 && mapping.HistoricalProfileID == "" && mapping.HistoricalDeliverableID == "" && mapping.HistoricalProfileHash == "" && mapping.HistoricalText == ""
		if mapping.ContractRef == runtimecontract.RuntimeProfileDeliverableEvidencePurpose {
			validHistory = refs[mapping.HistoricalItemRef] == 0 && runtimecontract.ValidateRuntimeProfileEvidencePurposeMappingDeclaration(runtimecontract.RuntimeEvidencePurposeMapping(mapping)) == nil
		}
		if !validHistory || refs[mapping.AcceptanceItemRef] != 1 ||
			mapping.HistoricalItemRef == mapping.AcceptanceItemRef || seen[mapping.HistoricalItemRef] ||
			strings.TrimSpace(mapping.HistoricalItemRef) != mapping.HistoricalItemRef || strings.TrimSpace(mapping.AcceptanceItemRef) != mapping.AcceptanceItemRef ||
			len(mapping.HistoricalItemRef) > 1024 || len(mapping.AcceptanceItemRef) > 1024 || strings.ContainsRune(mapping.HistoricalItemRef, 0) || strings.ContainsRune(mapping.AcceptanceItemRef, 0) {
			return fmt.Errorf("evidence purpose mapping requires unique distinct original DoD endpoints")
		}
		seen[mapping.HistoricalItemRef] = true
	}
	for _, mapping := range mappings {
		if seen[mapping.AcceptanceItemRef] {
			return fmt.Errorf("evidence purpose acceptance source cannot itself defer acceptance")
		}
	}
	return nil
}
