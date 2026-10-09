package workorchruntimecontract

import (
	"fmt"
	"slices"

	"github.com/muidea/workorch/internal/pkg/common"
)

type RuntimeDeliveryLineageState string

const (
	RuntimeDeliveryLineageNotApplicable RuntimeDeliveryLineageState = "not_applicable"
	RuntimeDeliveryLineageDeferred      RuntimeDeliveryLineageState = "deferred"
	RuntimeDeliveryLineagePending       RuntimeDeliveryLineageState = "pending"
	RuntimeDeliveryLineageSatisfied     RuntimeDeliveryLineageState = "satisfied"
)

// RuntimeDeliveryLineageBinding 是封存来源声明的只读投影，不是 executable binding 或 coverage credit。
type RuntimeDeliveryLineageBinding struct {
	CoverageWorkItemID common.CoverageWorkItemID
	DeclarationRef     string
	Assertions         []RuntimeLineageAssertion
}

// RuntimeDeliveryLineageBindingsFromState 从完整封存 CWI 编译交付声明，不要求先生成 Result。
func RuntimeDeliveryLineageBindingsFromState(source RuntimeStateRecord) ([]RuntimeDeliveryLineageBinding, error) {
	if err := ValidateRuntimeDefinitionLineageAssertions(source.Facts.DefinitionFocuses, source.Facts.Activation.ExecutionContract.ProfileContracts); err != nil {
		return nil, err
	}
	declared := false
	for _, profile := range source.Facts.Activation.ExecutionContract.ProfileContracts {
		for _, requirement := range profile.EvidenceRequirements {
			for _, rule := range requirement.LineageAssertions {
				declared = declared || rule.Kind == RuntimeLineageDeliveryAcknowledgements
			}
		}
	}
	if !declared {
		return nil, nil
	}
	if len(source.Facts.DefinitionSeed.CoverageItems) == 0 {
		return nil, fmt.Errorf("delivery lineage declaration requires sealed CWI inventory")
	}
	var bindings []RuntimeDeliveryLineageBinding
	seen := make(map[common.CoverageWorkItemID]bool)
	for _, item := range source.Facts.DefinitionSeed.CoverageItems {
		if seen[item.ID] || item.ID == "" {
			return nil, fmt.Errorf("delivery lineage requires unique original CWI")
		}
		seen[item.ID] = true
		binding, err := runtimeDeliveryLineageBinding(item, source.Facts.Activation.ExecutionContract.ProfileContracts)
		if err != nil {
			return nil, err
		}
		if len(binding.Assertions) != 0 {
			bindings = append(bindings, binding)
		}
	}
	return bindings, nil
}

func runtimeDeliveryLineageBinding(item RuntimeCoverageItem, profiles []RuntimeProfileContract) (RuntimeDeliveryLineageBinding, error) {
	policy, err := CompileRuntimeSourceEvidencePolicy(item.SourceScope, item.SourceID, profiles, nil)
	if err != nil {
		return RuntimeDeliveryLineageBinding{}, err
	}
	var rules []RuntimeLineageAssertionRule
	for _, rule := range policy.LineageAssertions {
		if rule.Kind == RuntimeLineageDeliveryAcknowledgements {
			rules = append(rules, rule)
		}
	}
	assertions, err := CompileRuntimeLineageAssertions(rules, policy.SemanticProofDeclarationRef, item)
	if err != nil {
		return RuntimeDeliveryLineageBinding{}, err
	}
	return RuntimeDeliveryLineageBinding{CoverageWorkItemID: item.ID, DeclarationRef: policy.SemanticProofDeclarationRef, Assertions: assertions}, nil
}

// EvaluateRuntimeDeliveryLineageAssertionFromState 不调用 aggregate/state Validate，也不改变终态或材料。
func EvaluateRuntimeDeliveryLineageAssertionFromState(source RuntimeStateRecord, binding RuntimeDeliveryLineageBinding) (RuntimeDeliveryLineageState, error) {
	if err := ValidateRuntimeDefinitionLineageAssertions(source.Facts.DefinitionFocuses, source.Facts.Activation.ExecutionContract.ProfileContracts); err != nil {
		return "", err
	}
	var item RuntimeCoverageItem
	matches := 0
	for _, candidate := range source.Facts.DefinitionSeed.CoverageItems {
		if candidate.ID == binding.CoverageWorkItemID {
			item, matches = candidate, matches+1
		}
	}
	if matches != 1 {
		return "", fmt.Errorf("delivery lineage requires unique original CWI")
	}
	expected, err := runtimeDeliveryLineageBinding(item, source.Facts.Activation.ExecutionContract.ProfileContracts)
	if err != nil {
		return "", err
	}
	if expected.DeclarationRef != binding.DeclarationRef || !slices.Equal(expected.Assertions, binding.Assertions) {
		return "", fmt.Errorf("delivery lineage differs from sealed source")
	}
	if len(expected.Assertions) == 0 {
		return RuntimeDeliveryLineageNotApplicable, nil
	}
	if err := ValidateRuntimeTerminalMaterialSeal(source.TerminalMaterialSeal, source); err != nil {
		return "", err
	}
	_, terminal, err := DeriveRuntimeTerminalOutcome(source.Decision, source.Termination)
	if err != nil {
		return "", err
	}
	if !terminal {
		return RuntimeDeliveryLineageDeferred, nil
	}
	if source.TerminalMaterialSeal == nil || len(source.DeliveryAcknowledgements) != 3 {
		return RuntimeDeliveryLineagePending, nil
	}
	return RuntimeDeliveryLineageSatisfied, nil
}
