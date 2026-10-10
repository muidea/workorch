package workorchruntimecontract

import "fmt"

// validateRuntimeCloseoutSourceObligations 反证完整来源清单，不用自洽 hash 掩盖删 B。
func validateRuntimeCloseoutSourceObligations(closeout RuntimeResultCloseout) error {
	if closeout.SourceObligations == nil {
		for _, outcome := range closeout.RequirementOutcomes {
			if len(outcome.SourceObligations) != 0 {
				return fmt.Errorf("runtime closeout source outcomes require sealed inventory")
			}
		}
		return nil
	}
	inventory := *closeout.SourceObligations
	if err := inventory.Validate(); err != nil {
		return err
	}
	if inventory.DefinitionRevision != closeout.DefinitionRevision || inventory.StartupInputHash != closeout.StartupInputHash {
		return fmt.Errorf("runtime closeout source inventory startup identity mismatch")
	}
	seen := make(map[string]bool)
	for _, outcome := range closeout.RequirementOutcomes {
		var item RuntimeCoverageItem
		item.ID = outcome.CoverageWorkItemID
		for _, source := range inventory.Sources {
			if outcome.RequirementRef == source.SourceScope+"/"+source.SourceID {
				item.SourceScope, item.SourceID, item.TargetText = source.SourceScope, source.SourceID, source.Text
			}
		}
		item = item.Normalized()
		checks, err := CompileRuntimeSourceObligationChecks(inventory, item)
		if err != nil {
			return err
		}
		actual := make([]RuntimeSourceObligationCheck, 0, len(outcome.SourceObligations))
		for _, source := range outcome.SourceObligations {
			actual = append(actual, source.Check)
		}
		if !sameRuntimeSourceObligationChecks(checks, actual) {
			return fmt.Errorf("runtime closeout coverage %q source outcomes omit or alter sealed obligation", item.ID)
		}
		if item.SourceScope == "dodItem" {
			if seen[item.SourceID] {
				return fmt.Errorf("runtime closeout duplicates source coverage")
			}
			seen[item.SourceID] = true
		}
	}
	for _, source := range inventory.Sources {
		if source.SourceScope == "dodItem" && !seen[source.SourceID] {
			return fmt.Errorf("runtime closeout omits sealed DoD source %q", source.SourceID)
		}
	}
	return nil
}

// runtimeTerminalSourceObligationOutcomes 只细分 SM-2 已裁决的当前 CWI 结论。
// 历史 assessment、旧 binding 或仅 A 的观察不能补发源义务 credit。
func runtimeTerminalSourceObligationOutcomes(source RuntimeStateRecord, coverage RuntimeCoverageItemFact, outcome RuntimeRequirementOutcome) []RuntimeSourceObligationOutcome {
	var checks []RuntimeSourceObligationCheck
	for _, item := range source.Facts.DefinitionSeed.CoverageItems {
		if item.ID == coverage.ID {
			checks = item.SourceObligationChecks
			break
		}
	}
	if len(checks) == 0 {
		return nil
	}
	binding, _ := runtimeTerminalCurrentCoverageBinding(source.Facts, coverage)
	proof, hasProof := CoverageEvidenceProofForKind(binding, EvidenceKindValidation)
	var ret []RuntimeSourceObligationOutcome
	for _, check := range checks {
		value := RuntimeSourceObligationOutcome{Check: check, CoverageState: RuntimeRequirementUncovered, AssessmentState: RuntimeRequirementUnassessed}
		if coverage.CurrentBindingRef != nil {
			ref := coverage.CurrentBindingRef.Clone()
			value.CurrentBindingRef = &ref
		}
		if outcome.AssessmentState == RuntimeRequirementUnsatisfied {
			value.AssessmentState = RuntimeRequirementUnsatisfied
			value.AssessmentReasonCode = outcome.AssessmentReasonCode
		}
		if outcome.CoverageState == RuntimeRequirementCovered && outcome.AssessmentState == RuntimeRequirementSatisfied && hasProof && containsString(proof.RequiredCheckRefs, check.CheckRef) {
			for _, evidence := range source.Facts.Evidence.Accepted {
				if evidence.Kind != EvidenceKindValidation || evidence.Validation == nil || assessmentEvidenceWeak(evidence) || !containsString(outcome.AcceptedEvidenceRefs, string(evidence.ID)) || !runtimeTerminalEvidenceMatchesCurrentProof(source, evidence, binding) {
					continue
				}
				if RuntimeValidationObservationSatisfiesCoverageProof(proof, *evidence.Validation) {
					value.AcceptedEvidenceRefs = append(value.AcceptedEvidenceRefs, string(evidence.ID))
				}
			}
			if len(value.AcceptedEvidenceRefs) != 0 {
				value.CoverageState = RuntimeRequirementCovered
				value.AssessmentState = RuntimeRequirementSatisfied
			} else {
				value.AssessmentState = RuntimeRequirementIndeterminate
				value.AssessmentReasonCode = "source_obligation_current_proof_missing"
			}
		}
		ret = append(ret, value.Normalized())
	}
	return ret
}
