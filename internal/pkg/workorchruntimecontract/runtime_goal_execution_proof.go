package workorchruntimecontract

import (
	"fmt"
	"slices"
	"strings"
)

func validateRuntimeExecutionProofMappingSource(mapping RuntimeExecutionProofMapping, consumer, source RuntimeDefinitionSource) error {
	if consumer.CandidateItemRef != mapping.ConsumerItemRef || source.CandidateItemRef != mapping.SourceItemRef ||
		source.SourceScope != "dodItem" || mapping.ConsumerItemRef == mapping.SourceItemRef || !runtimeSourceIdentity(mapping.RuleRef, 512) {
		return fmt.Errorf("execution proof mapping lacks exact original source identities")
	}
	switch mapping.ContractRef {
	case RuntimeCrossSourceExecutionProofV1:
		if consumer.SourceScope != "dodItem" || mapping.ConsumerGoalText != "" {
			return fmt.Errorf("v1 only accepts original DoD endpoints without Goal extensions")
		}
	case RuntimeGoalSourceExecutionProofV2:
		if consumer.SourceScope != "goal" || strings.TrimSpace(consumer.Text) == "" || len(consumer.Text) > 4096 || mapping.ConsumerGoalText != consumer.Text {
			return fmt.Errorf("Goal execution mapping requires bounded complete original Goal bytes")
		}
	default:
		return fmt.Errorf("unsupported execution proof source contract")
	}
	return nil
}

// Goal and goalCoverage are the original Goal/root declaration domains, not
// aliases for a DoD CWI. Only explicit v2 work/repair rules are projected.
func runtimeGoalExecutionRules(goal RuntimeDefinitionSource, profiles []RuntimeProfileContract) ([]RuntimeLineageAssertionRule, error) {
	var rules []RuntimeLineageAssertionRule
	for _, scope := range []string{"goal", "goalCoverage"} {
		policy, err := CompileRuntimeSourceEvidencePolicy(scope, goal.SourceID, profiles, nil)
		if err != nil {
			return nil, err
		}
		for _, rule := range policy.LineageAssertions {
			if rule.ExecutionSourceContractRef == "" {
				continue
			}
			if rule.ExecutionSourceContractRef != RuntimeGoalSourceExecutionProofV2 {
				return nil, fmt.Errorf("Goal/root execution source requires explicit v2")
			}
			rules, err = mergeRuntimeLineageAssertionRules(rules, []RuntimeLineageAssertionRule{rule})
			if err != nil {
				return nil, err
			}
		}
	}
	return rules, nil
}

// CompileRuntimeGoalExecutionAssertions does not create work or credit. Goal
// consumers are anchored only at the confirmed original execution source's
// ordinary acceptance CWI. No extra work/predecessor is inferred for Goal.
func CompileRuntimeGoalExecutionAssertions(inventory *RuntimeSourceObligationInventory, item RuntimeCoverageItem, profiles []RuntimeProfileContract) ([]RuntimeLineageAssertion, error) {
	if inventory == nil {
		return nil, nil
	}
	if err := ValidateRuntimeExecutionProofMappings(*inventory, profiles); err != nil {
		return nil, err
	}
	return compileRuntimeGoalExecutionAssertionsFromValidatedMappings(inventory, item, profiles)
}

// Only the same invocation's complete mapping gate may reach this compiler.
// Each item's original source text and every generated assertion still pass
// their independent gates; public and subsequent calls revalidate mappings.
func compileRuntimeGoalExecutionAssertionsFromValidatedMappings(inventory *RuntimeSourceObligationInventory, item RuntimeCoverageItem, profiles []RuntimeProfileContract) ([]RuntimeLineageAssertion, error) {
	if inventory == nil {
		return nil, nil
	}
	var ret []RuntimeLineageAssertion
	for _, mapping := range inventory.ExecutionProofMappings {
		if mapping.ContractRef != RuntimeGoalSourceExecutionProofV2 {
			continue
		}
		var goal, source RuntimeDefinitionSource
		for _, candidate := range inventory.Sources {
			if candidate.CandidateItemRef == mapping.ConsumerItemRef {
				goal = candidate
			}
			if candidate.CandidateItemRef == mapping.SourceItemRef {
				source = candidate
			}
		}
		if item.SourceScope != source.SourceScope || item.SourceID != source.SourceID {
			continue
		}
		if item.TargetText != strings.TrimSpace(source.Text) {
			return nil, fmt.Errorf("Goal execution source text drift")
		}
		rules, err := runtimeGoalExecutionRules(goal, profiles)
		if err != nil {
			return nil, err
		}
		for _, rule := range rules {
			if rule.Ref != mapping.RuleRef {
				continue
			}
			ret = append(ret, RuntimeLineageAssertion{Rule: rule, RuleHash: StableRuntimeLineageAssertionRuleHash(rule),
				DeclarationRef: CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID), CoverageWorkItemID: item.ID,
				SourceScope: goal.SourceScope, SourceID: goal.SourceID, SourceTextHash: RuntimeLineageSourceTextHash(goal.Text)})
		}
	}
	if err := ValidateRuntimeLineageAssertions(ret, item.ID, CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID)); err != nil {
		return nil, err
	}
	return ret, nil
}

// Fresh-load independently reconstructs the projection from original sealed
// Profile rules and confirmed inventory; coordinated item/hash edits are not authority.
func ValidateRuntimeGoalExecutionAssertionSeed(seed RuntimeDefinitionSeed, profiles []RuntimeProfileContract) error {
	if len(seed.CoverageItems) != 0 && seed.SourceObligations != nil {
		if err := ValidateRuntimeExecutionProofMappings(*seed.SourceObligations, profiles); err != nil {
			return err
		}
	}
	for _, item := range seed.CoverageItems {
		expected, err := compileRuntimeGoalExecutionAssertionsFromValidatedMappings(seed.SourceObligations, item, profiles)
		if err != nil {
			return err
		}
		if !slices.Equal(expected, item.GoalExecutionAssertions) {
			return fmt.Errorf("Goal execution assertions differ from confirmed source and Profile")
		}
	}
	return nil
}

func validateRuntimeGoalExecutionProjection(inventory RuntimeSourceObligationInventory, items []RuntimeCoverageItem) error {
	seen := map[string]int{}
	for _, item := range items {
		declaration := CanonicalRuntimeDefinitionEvidenceDeclarationRef(item.SourceScope, item.SourceID)
		if err := ValidateRuntimeLineageAssertions(item.GoalExecutionAssertions, item.ID, declaration); err != nil {
			return err
		}
		for _, assertion := range item.GoalExecutionAssertions {
			if assertion.Rule.ExecutionSourceContractRef != RuntimeGoalSourceExecutionProofV2 || assertion.SourceScope != "goal" {
				return fmt.Errorf("invalid Goal consumer projection")
			}
			matches := 0
			for _, mapping := range inventory.ExecutionProofMappings {
				if mapping.ContractRef != RuntimeGoalSourceExecutionProofV2 || mapping.RuleRef != assertion.Rule.Ref {
					continue
				}
				var goal, source RuntimeDefinitionSource
				for _, candidate := range inventory.Sources {
					if candidate.CandidateItemRef == mapping.ConsumerItemRef {
						goal = candidate
					}
					if candidate.CandidateItemRef == mapping.SourceItemRef {
						source = candidate
					}
				}
				if source.SourceScope == item.SourceScope && source.SourceID == item.SourceID && item.TargetText == strings.TrimSpace(source.Text) && goal.SourceID == assertion.SourceID && RuntimeLineageSourceTextHash(goal.Text) == assertion.SourceTextHash {
					matches++
					seen[mapping.ConsumerItemRef+"\x00"+mapping.RuleRef]++
				}
			}
			if matches != 1 {
				return fmt.Errorf("Goal projection lacks exact confirmed source")
			}
		}
	}
	for _, mapping := range inventory.ExecutionProofMappings {
		if mapping.ContractRef == RuntimeGoalSourceExecutionProofV2 && seen[mapping.ConsumerItemRef+"\x00"+mapping.RuleRef] != 1 {
			return fmt.Errorf("Goal execution mapping omitted/duplicated consumer")
		}
	}
	return nil
}

func runtimeGoalExecutionCheck(inventory RuntimeSourceObligationInventory, item RuntimeCoverageItem) *RuntimeSourceObligationCheck {
	var goal RuntimeDefinitionSource
	for _, source := range inventory.Sources {
		if source.SourceScope == "goal" {
			goal = source
		}
	}
	for _, mapping := range inventory.ExecutionProofMappings {
		if mapping.ContractRef != RuntimeGoalSourceExecutionProofV2 {
			continue
		}
		for _, source := range inventory.Sources {
			if source.CandidateItemRef != mapping.SourceItemRef || source.SourceScope != item.SourceScope || source.SourceID != item.SourceID {
				continue
			}
			// One ordinary full-Goal check, even when several all-of rules share it.
			identity := "goal-execution-" + RuntimeLineageSourceTextHash(goal.CandidateItemRef)
			value := RuntimeSourceObligationCheck{InventoryHash: inventory.Hash, ObligationID: identity, RequirementID: identity,
				VerificationID: identity, Criterion: goal.Text, Declaration: "Ordinary full Goal proof AND confirmed execution lineage consumers; no Goal work credit."}
			value.CheckRef = StableRuntimeSourceObligationCheckRef(inventory.DefinitionRevision, item.ID, value)
			return &value
		}
	}
	return nil
}
