package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeBootstrapCloneAndEqualityPreserveTypedContract(t *testing.T) {
	bootstrap := RuntimeBootstrap{
		SchemaVersion: RuntimeStateSchemaVersionCurrent,
		Definition: RuntimeDefinitionContext{
			Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod",
			CapabilityContract: []string{"file/edit", "shell/command"},
		},
		Activation: RuntimeActivationContext{ProviderTarget: ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, DialogLanguage: "zh-CN", Skills: []RuntimeSkillContext{{ID: "skill-1", Instruction: "typed instruction"}}, ExecutionContract: RuntimeExecutionContract{
			MaxRepairAttemptsPerFocus: 7, ValidationPolicies: []RuntimeValidationPolicy{{ID: "validation", Scope: "workspace_profile", AppliesToEvidenceKinds: []string{EvidenceKindValidation}, MaxValidationOnlyRetriesPerFocus: 2}},
		}},
		Focuses:           []CoverageFocus{{ID: "focus-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange}}},
		Slices:            []RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, CompletionPolicy: RuntimeAcceptanceSliceCompletionAll, ResultPolicy: RuntimeAcceptanceSliceResultAllowed}},
		CoverageItems:     []RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-001", Order: 1}},
		Requirements:      []RuntimeRequirement{{Kind: PrerequisiteKindCapability, OwnerModuleID: RuntimeModuleFile, CapabilityClass: RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{"file/edit"}}},
		ConvergencePolicy: RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 7, MaxValidationOnlyRetriesPerFocus: 2, MaxAcceptanceSliceWorkItems: 4},
	}
	cloned := bootstrap.Clone()
	if err := cloned.Validate(); err != nil {
		t.Fatal(err)
	}
	if !SameRuntimeBootstrap(bootstrap, cloned) {
		t.Fatalf("bootstrap clone drifted: source=%+v clone=%+v", bootstrap, cloned)
	}
	facts, err := bootstrap.InitialFacts("ws-1", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Activation.Skills) != 1 || facts.Activation.Skills[0].Instruction != "typed instruction" {
		t.Fatalf("initial facts lost activation: %+v", facts.Activation)
	}
	cloned.Focuses[0].ID = "focus-2"
	if SameRuntimeBootstrap(bootstrap, cloned) {
		t.Fatal("focus drift was not detected")
	}
	cloned = bootstrap.Clone()
	cloned.ConvergencePolicy.MaxValidationOnlyRetriesPerFocus = 3
	if SameRuntimeBootstrap(bootstrap, cloned) {
		t.Fatal("convergence policy drift was not detected")
	}
}
