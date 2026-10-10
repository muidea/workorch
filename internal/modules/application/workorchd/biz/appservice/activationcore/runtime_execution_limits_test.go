package activationcore

import (
	"encoding/json"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/kernel/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestRuntimeBootstrapSealsExecutionLimitsAndRejectsRehashedBudgetDrift(t *testing.T) {
	limits := &runtimecontract.RuntimeExecutionLimits{ProtocolRef: runtimecontract.RuntimeExecutionLimitsV1, MaxProviderCalls: 12, MaxCapabilityOperations: 8}
	bootstrap, err := runtimeBootstrapWithTestProof(
		&workspacemodel.Goal{ID: "goal", WorkspaceID: "ws", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod", WorkspaceID: "ws", Items: []string{"validate"}}, "definition", nil,
		runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "provider", ModelID: "model"}},
			ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(), ExecutionLimits: limits}},
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	var restored runtimecontract.RuntimeBootstrap
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := runtimecontract.ValidateRuntimeExecutionLimitsBinding(restored.ExecutionBudgetPolicy, limits); err != nil {
		t.Fatal(err)
	}
	limits.MaxProviderCalls++
	if restored.Activation.ExecutionContract.ExecutionLimits.MaxProviderCalls != 12 || bootstrap.Activation.ExecutionContract.ExecutionLimits.MaxProviderCalls != 12 {
		t.Fatal("bootstrap retained mutable configuration")
	}
	for _, dimension := range []runtimecontract.RuntimeBudgetDimension{runtimecontract.RuntimeBudgetDimensionProviderCall, runtimecontract.RuntimeBudgetDimensionCapabilityOperation} {
		drifted := restored.Clone()
		for i := range drifted.ExecutionBudgetPolicy.Dimensions {
			if drifted.ExecutionBudgetPolicy.Dimensions[i].Dimension == dimension {
				drifted.ExecutionBudgetPolicy.Dimensions[i].Limit++
			}
		}
		drifted.ExecutionBudgetPolicy.PolicyHash = runtimecontract.StableRuntimeExecutionBudgetPolicyHash(drifted.ExecutionBudgetPolicy)
		if drifted.Validate() == nil {
			t.Fatal("bootstrap accepted rehashed policy drift")
		}
	}
}
