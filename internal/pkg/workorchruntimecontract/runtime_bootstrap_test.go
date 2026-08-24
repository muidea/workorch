package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeBootstrapCloneAndEqualityPreserveTypedContract(t *testing.T) {
	skills, skillBudget := sealedRuntimeSkillFixture(t, "skill-1", "typed instruction")
	binding := CoverageEvidenceBinding{
		ID: "binding/cwi-1", CoverageWorkItemID: "cwi-1", FocusID: "focus-1", DefinitionRevision: "definition-1",
		Proofs:     []CoverageEvidenceProof{{EvidenceKind: EvidenceKindStateChange, RequiredEffectScopes: []RuntimeEffectScope{RuntimeEffectWorkspaceDurable}, AllowedOperationKinds: []string{StepKindFileEdit}}},
		TargetRefs: []string{"target.txt"}, DeclarationRef: "test/bootstrap",
	}.Normalized()
	binding.Hash = StableCoverageEvidenceBindingHash(binding)
	bootstrap := RuntimeBootstrap{
		Definition: RuntimeDefinitionContext{StartupInputHash: "startup-input-test",
			Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod",
			CapabilityContract: []string{"file/edit", "shell/command"},
		},
		Activation: RuntimeActivationContext{ProviderSelection: ProviderInferenceSelection{Target: ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}, DialogLanguage: "zh-CN", Skills: skills, SkillContextBudget: skillBudget, ExecutionContract: RuntimeExecutionContract{
			StateChangePolicy:         RuntimeStateChangeAllowedButNotRequired,
			MaxRepairAttemptsPerFocus: 7, ValidationPolicies: []RuntimeValidationPolicy{{ID: "validation", Scope: "workspace_profile", AppliesToEvidenceKinds: []string{EvidenceKindValidation}, MaxValidationOnlyRetriesPerFocus: 2}},
		}},
		Focuses: []CoverageFocus{
			{ID: "focus-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange}, TargetRefs: []string{"target.txt"}, AcceptedStepKinds: []string{StepKindFileEdit}, CoverageEvidenceBindings: []CoverageEvidenceBinding{binding}},
			{ID: "focus-0", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-0"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange}, TargetRefs: []string{"target.txt"}, AcceptedStepKinds: []string{StepKindFileEdit}},
		},
		Slices: []RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1", "cwi-0"}, CompletionPolicy: RuntimeAcceptanceSliceCompletionAll, ResultPolicy: RuntimeAcceptanceSliceResultAllowed}},
		CoverageItems: []RuntimeCoverageItem{
			{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-001", Order: 1,
				PrerequisiteCoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-0"},
				ReopenDependencies:              []RuntimeCoverageReopenDependency{{CoverageWorkItemID: "cwi-0", TriggerEvidenceKinds: []string{EvidenceKindStateChange}}}},
			{ID: "cwi-0", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-000", Order: 0},
		},
		Requirements:      []RuntimeRequirement{{Kind: PrerequisiteKindCapability, OwnerModuleID: RuntimeModuleFile, CapabilityClass: RuntimeCapabilityClassStateChange, RequiredCapabilities: []string{"file/edit"}}},
		ConvergencePolicy: RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 7, MaxValidationOnlyRetriesPerFocus: 2, MaxAcceptanceSliceWorkItems: 4, MaxFocuses: 4, MaxCandidateContextItems: 4},
	}
	bootstrap.RecoveryPolicy = DefaultRuntimeRecoveryPolicy()
	bootstrap.ExecutionBudgetPolicy = RuntimeExecutionBudgetPolicyForRuntimePolicies(bootstrap.ConvergencePolicy, bootstrap.RecoveryPolicy)
	bootstrap.Focuses[0].CoverageEvidenceBindings = nil
	seed, err := CompileRuntimeDefinitionSeed(bootstrap.Definition, bootstrap.Activation, bootstrap.CoverageItems, bootstrap.Slices)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap.DefinitionSeed = seed
	cloned := bootstrap.Clone()
	if err := cloned.Validate(); err != nil {
		t.Fatal(err)
	}
	if !SameRuntimeBootstrap(bootstrap, cloned) {
		t.Fatalf("bootstrap clone drifted: source=%+v clone=%+v", bootstrap, cloned)
	}
	if len(cloned.CoverageItems[0].PrerequisiteCoverageWorkItemIDs) != 1 || cloned.CoverageItems[0].PrerequisiteCoverageWorkItemIDs[0] != "cwi-0" {
		t.Fatalf("bootstrap clone lost prerequisite identity: %+v", cloned.CoverageItems)
	}
	cloned.CoverageItems[0].PrerequisiteCoverageWorkItemIDs[0] = "cwi-mutated"
	if bootstrap.CoverageItems[0].PrerequisiteCoverageWorkItemIDs[0] != "cwi-0" {
		t.Fatal("bootstrap clone shares mutable prerequisite slice")
	}
	facts, err := bootstrap.InitialFacts("ws-1", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Activation.Skills) != 1 || facts.Activation.Skills[0].Instruction != "typed instruction" {
		t.Fatalf("initial facts lost activation: %+v", facts.Activation)
	}
	if len(facts.DefinitionFocuses) != 2 {
		t.Fatalf("initial facts definition focus count = %d", len(facts.DefinitionFocuses))
	}
	for _, declaration := range facts.DefinitionFocuses {
		if len(declaration.AcceptedStepKinds) != 0 || len(declaration.TargetRefs) != 0 || declaration.ValidationRecipeID != "" || len(declaration.RequiredCheckRefs) != 0 || len(declaration.CoverageEvidenceBindings) != 0 {
			t.Fatalf("definition focus retained generation-specific authority: %+v", declaration)
		}
	}
	facts.Focus.AcceptedStepKinds = []string{RuntimeBlockKindPatchApply}
	if len(facts.DefinitionFocuses[0].AcceptedStepKinds) != 0 {
		t.Fatal("semantic generation mutation changed immutable definition focus")
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
		Definition:        RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"file/write"}},
		Activation:        RuntimeActivationContext{ProviderSelection: ProviderInferenceSelection{Target: ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
		Focuses:           []CoverageFocus{{ID: "focus-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange}}},
		Slices:            []RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, CompletionPolicy: RuntimeAcceptanceSliceCompletionAll, ResultPolicy: RuntimeAcceptanceSliceResultAllowed}},
		CoverageItems:     []RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "deliverable", SourceID: "source", Order: 1, ReopenDependencies: []RuntimeCoverageReopenDependency{{CoverageWorkItemID: "cwi-missing", TriggerEvidenceKinds: []string{EvidenceKindStateChange}}}}},
		ConvergencePolicy: RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 1, MaxValidationOnlyRetriesPerFocus: 1, MaxAcceptanceSliceWorkItems: 1, MaxFocuses: 1, MaxCandidateContextItems: 1},
	}
	bootstrap.RecoveryPolicy = DefaultRuntimeRecoveryPolicy()
	bootstrap.ExecutionBudgetPolicy = RuntimeExecutionBudgetPolicyForRuntimePolicies(bootstrap.ConvergencePolicy, bootstrap.RecoveryPolicy)
	bootstrap.DefinitionSeed = RuntimeDefinitionSeed{
		SchemaVersion: RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: bootstrap.Definition.Revision,
		PolicyHash:    StableRuntimeCandidatePrefixHashWithExecutionContract(bootstrap.Definition, bootstrap.Activation.ExecutionContract),
		CoverageItems: bootstrap.CoverageItems, Slices: bootstrap.Slices,
	}
	bootstrap.DefinitionSeed.Hash = StableRuntimeDefinitionSeedHash(bootstrap.DefinitionSeed)
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

func TestRuntimeBootstrapRejectsSharedCoverageFocus(t *testing.T) {
	bootstrap := RuntimeBootstrap{
		Definition: RuntimeDefinitionContext{StartupInputHash: "startup-input-test",
			Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod",
		},
		Activation: RuntimeActivationContext{ProviderSelection: ProviderInferenceSelection{Target: ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
		Focuses: []CoverageFocus{{
			ID: "focus-shared", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1", "cwi-2"},
			RequiredEvidenceKinds: []string{EvidenceKindStateChange},
		}},
		Slices: []RuntimeAcceptanceSlice{{
			ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1", "cwi-2"},
			CompletionPolicy: RuntimeAcceptanceSliceCompletionAll, ResultPolicy: RuntimeAcceptanceSliceResultAllowed,
		}},
		CoverageItems: []RuntimeCoverageItem{
			{ID: "cwi-1", SliceID: "slice-1", SourceScope: "test", SourceID: "cwi-1"},
			{ID: "cwi-2", SliceID: "slice-1", SourceScope: "test", SourceID: "cwi-2"},
		},
		ConvergencePolicy: RuntimeConvergencePolicy{MaxRepairAttemptsPerFocus: 1, MaxValidationOnlyRetriesPerFocus: 1, MaxAcceptanceSliceWorkItems: 2, MaxFocuses: 2, MaxCandidateContextItems: 2},
	}
	bootstrap.RecoveryPolicy = DefaultRuntimeRecoveryPolicy()
	bootstrap.ExecutionBudgetPolicy = RuntimeExecutionBudgetPolicyForRuntimePolicies(bootstrap.ConvergencePolicy, bootstrap.RecoveryPolicy)
	seed, err := CompileRuntimeDefinitionSeed(bootstrap.Definition, bootstrap.Activation, bootstrap.CoverageItems, bootstrap.Slices)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap.DefinitionSeed = seed
	if err := bootstrap.Validate(); err == nil {
		t.Fatalf("shared CWI focus was accepted: %+v", bootstrap)
	}
}
