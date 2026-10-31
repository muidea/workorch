package workorchruntimecontract

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRuntimeExecutionLimitsValidationIdentityAndClone(t *testing.T) {
	valid := RuntimeExecutionLimits{ProtocolRef: RuntimeExecutionLimitsV1, MaxProviderCalls: 6, MaxCapabilityOperations: 4}
	contract := RuntimeExecutionContract{StateChangePolicy: RuntimeStateChangeAllowedButNotRequired, ExecutionLimits: &valid}
	if err := contract.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*RuntimeExecutionLimits)
	}{
		{"protocol", func(v *RuntimeExecutionLimits) { v.ProtocolRef = "runtime-execution-limits/v2" }},
		{"blank protocol", func(v *RuntimeExecutionLimits) { v.ProtocolRef = "" }},
		{"spaced protocol", func(v *RuntimeExecutionLimits) { v.ProtocolRef += " " }},
		{"zero calls", func(v *RuntimeExecutionLimits) { v.MaxProviderCalls = 0 }},
		{"negative operations", func(v *RuntimeExecutionLimits) { v.MaxCapabilityOperations = -1 }},
		{"raise calls", func(v *RuntimeExecutionLimits) { v.MaxProviderCalls = 4097 }},
		{"raise operations", func(v *RuntimeExecutionLimits) { v.MaxCapabilityOperations = 8193 }},
		{"negative proof batches", func(v *RuntimeExecutionLimits) { v.MaxProofBatchesPerCWI = -1 }},
		{"negative durable dispatches", func(v *RuntimeExecutionLimits) { v.MaxDurableDispatchesPerCWI = -1 }},
		{"raise durable dispatches", func(v *RuntimeExecutionLimits) { v.MaxDurableDispatchesPerCWI = 8193 }},
		{"raise proof batches", func(v *RuntimeExecutionLimits) { v.MaxProofBatchesPerCWI = RuntimeExecutionProofBatchesPerCWIMaxV1 + 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := contract.Normalized()
			tc.edit(changed.ExecutionLimits)
			if err := changed.Validate(); err == nil {
				t.Fatal("invalid limits accepted")
			}
			if SameRuntimeExecutionContract(contract, changed) || StableRuntimeCandidatePrefixHashWithExecutionContract(RuntimeDefinitionContext{}, contract) == StableRuntimeCandidatePrefixHashWithExecutionContract(RuntimeDefinitionContext{}, changed) {
				t.Fatal("limits omitted from sealed identity")
			}
			if *contract.ExecutionLimits != valid {
				t.Fatal("clone aliases original")
			}
		})
	}
	raw := cloneRuntimeExecutionContractRaw(contract)
	raw.ExecutionLimits.MaxProviderCalls++
	if *contract.ExecutionLimits != valid {
		t.Fatal("raw clone aliases original")
	}
	encoded, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	var restored RuntimeExecutionContract
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if !SameRuntimeExecutionContract(contract, restored) || restored.Validate() != nil {
		t.Fatal("same-version reload lost limits")
	}
}

func TestRuntimeExecutionLimitsCompileOnlyDeclaredDimensionsAndRejectPolicyDrift(t *testing.T) {
	limits := RuntimeExecutionLimits{ProtocolRef: RuntimeExecutionLimitsV1, MaxProviderCalls: 3, MaxCapabilityOperations: 2}
	convergence, recovery := DefaultRuntimeConvergencePolicy(), DefaultRuntimeRecoveryPolicy()
	baseline := RuntimeExecutionBudgetPolicyForRuntimePolicies(convergence, recovery)
	policy, err := RuntimeExecutionBudgetPolicyWithLimits(convergence, recovery, &limits)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRuntimeExecutionLimitsBinding(policy, &limits); err != nil {
		t.Fatal(err)
	}
	for _, item := range policy.Dimensions {
		original, _ := RuntimeBudgetDimensionPolicyFor(baseline, item.Dimension)
		switch item.Dimension {
		case RuntimeBudgetDimensionProviderCall:
			original.Limit = 3
		case RuntimeBudgetDimensionCapabilityOperation:
			original.Limit = 2
		}
		if item != original {
			t.Fatalf("unrelated policy changed: %+v / %+v", item, original)
		}
	}
	for _, dimension := range []RuntimeBudgetDimension{RuntimeBudgetDimensionProviderCall, RuntimeBudgetDimensionCapabilityOperation} {
		changed := policy.Clone()
		for i := range changed.Dimensions {
			if changed.Dimensions[i].Dimension == dimension {
				changed.Dimensions[i].Limit++
			}
		}
		changed.PolicyHash = StableRuntimeExecutionBudgetPolicyHash(changed)
		if changed.Validate() != nil || ValidateRuntimeExecutionLimitsBinding(changed, &limits) == nil {
			t.Fatal("validly rehashed policy bypassed sealed limit")
		}
	}
	unchanged, err := RuntimeExecutionBudgetPolicyWithLimits(convergence, recovery, nil)
	if err != nil || !reflect.DeepEqual(baseline, unchanged) {
		t.Fatal("undeclared limits changed current default policy")
	}
}
