package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeBootstrapCloneAndEqualityPreserveTypedContract(t *testing.T) {
	binding := CoverageEvidenceBinding{
		ID: "binding/cwi-1", CoverageWorkItemID: "cwi-1", FocusID: "focus-1", DefinitionRevision: "definition-1",
		Proofs:     []CoverageEvidenceProof{{EvidenceKind: EvidenceKindStateChange, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, AllowedOperationKinds: []string{StepKindFileEdit}}},
		TargetRefs: []string{"target.txt"}, DeclarationRef: "test/bootstrap",
	}.Normalized()
	binding.Hash = StableCoverageEvidenceBindingHash(binding)
	bootstrap := RuntimeBootstrap{
		SchemaVersion: RuntimeStateSchemaVersionCurrent,
		Definition: RuntimeDefinitionContext{
			Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod",
			CapabilityContract: []string{"file/edit", "shell/command"},
		},
		Activation: RuntimeActivationContext{ProviderTarget: ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, DialogLanguage: "zh-CN", Skills: []RuntimeSkillContext{{ID: "skill-1", Instruction: "typed instruction"}}, ExecutionContract: RuntimeExecutionContract{
			MaxRepairAttemptsPerFocus: 7, ValidationPolicies: []RuntimeValidationPolicy{{ID: "validation", Scope: "workspace_profile", AppliesToEvidenceKinds: []string{EvidenceKindValidation}, MaxValidationOnlyRetriesPerFocus: 2}},
		}},
		Focuses:           []CoverageFocus{{ID: "focus-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange}, TargetRefs: []string{"target.txt"}, AcceptedStepKinds: []string{StepKindFileEdit}, CoverageEvidenceBindings: []CoverageEvidenceBinding{binding}}},
		Slices:            []RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, CompletionPolicy: RuntimeAcceptanceSliceCompletionAll, ResultPolicy: RuntimeAcceptanceSliceResultAllowed}},
		CoverageItems:     []RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-001", Order: 1}},
		Requirements:      []RuntimeRequirement{{Kind: PrerequisiteKindCapability, OwnerModuleID: RuntimeModuleFile, CapabilityClass: RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{"file/edit"}}},
		ConvergencePolicy: RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 7, MaxValidationOnlyRetriesPerFocus: 2, MaxAcceptanceSliceWorkItems: 4, MaxFocuses: 4, MaxCandidateContextItems: 4},
	}
	contract, err := CompileRuntimeCWIReconciliationContract(bootstrap.Definition.Revision, bootstrap.Focuses)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap.ReconciliationContract = contract
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

func TestRuntimeBootstrapRejectsUnknownCoverageReopenDependency(t *testing.T) {
	bootstrap := RuntimeBootstrap{
		SchemaVersion:     RuntimeStateSchemaVersionCurrent,
		Definition:        RuntimeDefinitionContext{Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"file/write"}},
		Activation:        RuntimeActivationContext{ProviderTarget: ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		Focuses:           []CoverageFocus{{ID: "focus-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange}}},
		Slices:            []RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, CompletionPolicy: RuntimeAcceptanceSliceCompletionAll, ResultPolicy: RuntimeAcceptanceSliceResultAllowed}},
		CoverageItems:     []RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "deliverable", SourceID: "source", Order: 1, ReopenDependencies: []RuntimeCoverageReopenDependency{{CoverageWorkItemID: "cwi-missing", TriggerEvidenceKinds: []string{EvidenceKindStateChange}}}}},
		ConvergencePolicy: RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 1, MaxValidationOnlyRetriesPerFocus: 1, MaxAcceptanceSliceWorkItems: 1, MaxFocuses: 1, MaxCandidateContextItems: 1},
	}
	if err := bootstrap.Validate(); err == nil {
		t.Fatal("bootstrap accepted a coverage dependency with unknown target")
	}
}

func TestRuntimeBootstrapRequirementScopeIsPartOfIdentityAndEquality(t *testing.T) {
	global := RuntimeRequirement{Kind: PrerequisiteKindValidationScratch, OwnerModuleID: RuntimeModuleOrchestrator}
	focus := RuntimeRequirement{Kind: PrerequisiteKindValidationScratch, Scope: RuntimePrerequisiteScopeFocus, FocusID: "focus-1", OwnerModuleID: RuntimeModuleOrchestrator}
	normalized := normalizeRuntimeRequirements([]RuntimeRequirement{global, focus})
	if len(normalized) != 2 || normalized[0].Scope != RuntimePrerequisiteScopeGlobal || normalized[0].FocusID != "" || normalized[1].Scope != RuntimePrerequisiteScopeFocus || normalized[1].FocusID != "focus-1" {
		t.Fatalf("normalized prerequisite scopes = %+v", normalized)
	}
	if sameRuntimeRequirement(global, focus) {
		t.Fatal("global and focus prerequisite requirements compare equal")
	}
}
