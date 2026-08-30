package workorchruntimecontract

import (
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"
)

func TestRuntimeBootstrapCloneAndEqualityPreserveTypedContract(t *testing.T) {
	skills, skillBudget := sealedRuntimeSkillFixture(t, "skill-1", "typed instruction")
	bootstrap := RuntimeBootstrap{
		Definition: RuntimeDefinitionContext{StartupInputHash: "startup-input-test",
			Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod",
			CapabilityContract: []string{"file/edit", "shell/command"},
		},
		Activation: RuntimeActivationContext{ProviderSelection: ProviderInferenceSelection{Target: ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}, DialogLanguage: "zh-CN", Skills: skills, SkillContextBudget: skillBudget, ExecutionContract: RuntimeExecutionContract{
			StateChangePolicy:         RuntimeStateChangeAllowedButNotRequired,
			MaxRepairAttemptsPerFocus: 7, ValidationPolicies: []RuntimeValidationPolicy{{ID: "validation", Scope: "workspace_profile", AppliesToEvidenceKinds: []string{EvidenceKindValidation}, MaxValidationOnlyRetriesPerFocus: 2}},
		}},
		DefinitionFocuses: []RuntimeDefinitionFocus{
			{ID: "focus-1", SliceID: "slice-1", AssessmentScope: "dodItem", AssessmentID: "dod-001", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange}},
			{ID: "focus-0", SliceID: "slice-1", AssessmentScope: "dodItem", AssessmentID: "dod-000", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-0"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange}},
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
	invalidAllOfPrerequisite := cloneRuntimeFactsRaw(facts)
	invalidAllOfPrerequisite.Prerequisites.Pending = append(invalidAllOfPrerequisite.Prerequisites.Pending, RuntimePrerequisite{
		ID: "prerequisite-invalid-all-of", Kind: PrerequisiteKindTargetSelection, OwnerModuleID: RuntimeModuleFile,
		ProspectiveTargetAllOfConstraintRefs: []string{"workspace-relative-path"},
	})
	if err := invalidAllOfPrerequisite.Validate(); err == nil {
		t.Fatal("runtime facts accepted a prospective target all-of prerequisite without a complete selector")
	}
	invalidAllOfFocus := cloneRuntimeFactsRaw(facts)
	invalidAllOfFocus.Focus.ProspectiveTargetAllOfConstraintRefs = []string{"workspace-relative-path"}
	if err := invalidAllOfFocus.Validate(); err == nil {
		t.Fatal("runtime facts accepted a prospective target all-of focus without grounding and a complete selector")
	}
	facts.Focus.AcceptedStepKinds = []string{RuntimeBlockKindPatchApply}
	if facts.DefinitionFocuses[0].ID != "focus-1" {
		t.Fatal("semantic generation mutation changed immutable definition focus")
	}
	cloned.DefinitionFocuses[0].ID = "focus-2"
	if SameRuntimeBootstrap(bootstrap, cloned) {
		t.Fatal("focus drift was not detected")
	}
	cloned = bootstrap.Clone()
	cloned.ConvergencePolicy.MaxValidationOnlyRetriesPerFocus = 3
	if SameRuntimeBootstrap(bootstrap, cloned) {
		t.Fatal("convergence policy drift was not detected")
	}
	presealedTarget := bootstrap.Clone()
	presealedTarget.Requirements = append(presealedTarget.Requirements, RuntimeRequirement{
		Kind: PrerequisiteKindTargetSelection, OwnerModuleID: RuntimeModuleFile,
		TargetSelectorRef: "file/workspace-relative-create", ProspectiveTargetRef: "presealed-target.txt",
	})
	if err := presealedTarget.Validate(); err == nil {
		t.Fatal("bootstrap requirement accepted a pre-seal prospective target")
	}
	invalidAllOfBootstrap := bootstrap.Clone()
	invalidAllOfBootstrap.Requirements = append(invalidAllOfBootstrap.Requirements, RuntimeRequirement{
		Kind: PrerequisiteKindTargetSelection, OwnerModuleID: RuntimeModuleFile,
		ProspectiveTargetAllOfConstraintRefs: []string{"workspace-relative-path"},
	})
	if err := invalidAllOfBootstrap.Validate(); err == nil {
		t.Fatal("bootstrap requirement accepted a prospective target all-of declaration without a complete selector")
	}
	retiredFailureTarget := bootstrap.Clone()
	retiredFailureTarget.Requirements = append(retiredFailureTarget.Requirements, RuntimeRequirement{Kind: PrerequisiteKind("failure_target")})
	if err := retiredFailureTarget.Validate(); err == nil {
		t.Fatal("bootstrap accepted the retired failure_target prerequisite")
	}
}

func TestRuntimeBootstrapRejectsUnknownCoverageReopenDependency(t *testing.T) {
	bootstrap := RuntimeBootstrap{
		Definition:        RuntimeDefinitionContext{StartupInputHash: "startup-input-test", Revision: "definition-1", GoalID: "goal-1", GoalDefinition: "goal", DoDID: "dod-1", DoDDefinition: "dod", CapabilityContract: []string{"file/write"}},
		Activation:        RuntimeActivationContext{ProviderSelection: ProviderInferenceSelection{Target: ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}},
		DefinitionFocuses: []RuntimeDefinitionFocus{{ID: "focus-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindStateChange}}},
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
		DefinitionFocuses: []RuntimeDefinitionFocus{{
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

func TestRuntimeDefinitionFocusPreservesDeclarationWithoutGenerationAuthority(t *testing.T) {
	seed := RuntimeDefinitionSeed{
		SchemaVersion:      RuntimeDefinitionSeedSchemaCurrent,
		DefinitionRevision: "definition-1",
		PolicyHash:         "policy-1",
		CoverageItems:      []RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1"}},
		Slices:             []RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, CompletionPolicy: RuntimeAcceptanceSliceCompletionAll, ResultPolicy: RuntimeAcceptanceSliceResultAllowed}},
	}.Normalized()
	seed.Hash = StableRuntimeDefinitionSeedHash(seed)

	compiled := CompileRuntimeDefinitionFocuses([]CoverageFocus{{
		ID: "focus-1", SliceID: "slice-1", AssessmentScope: "dodItem", AssessmentID: "dod-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"},
		RequiredEvidenceKinds: []string{EvidenceKindValidation}, TargetRole: "verification_target",
		TargetRefs: []string{"declared-but-not-owner-resolved.txt"}, TargetSelectorRef: "file/workspace-deliverable",
		TargetSelectorOwnerRef: "profile/test", TargetSelectorSubjectRef: "verification_target",
		TargetSelectorCandidateRefs: []string{"activation-sealed-validation.sh"}, ProspectiveTargetAllOfConstraintRefs: []string{"workspace-relative-path", "new-target-requires-owner-selector"}, RequireTargetGrounding: true,
		AcceptedStepKinds: []string{RuntimeBlockKindManagedValidationSession}, ValidationRecipeID: "recipe-1",
		RequiredCheckRefs: []string{"check-1"},
	}})
	if err := ValidateRuntimeDefinitionFocuses(seed, compiled); err != nil {
		t.Fatalf("immutable declaration rejected: %v", err)
	}
	if len(compiled) != 1 || compiled[0].TargetRole != "verification_target" || !compiled[0].RequireTargetGrounding ||
		!sameStrings(compiled[0].TargetSelectorCandidateRefs, []string{"activation-sealed-validation.sh"}) ||
		!SameRuntimeProspectiveTargetAllOfConstraintRefs(compiled[0].ProspectiveTargetAllOfConstraintRefs, []string{"new-target-requires-owner-selector", "workspace-relative-path"}) {
		t.Fatalf("definition declaration lost grounding obligation: %+v", compiled)
	}
	for name, mutate := range map[string]func(*RuntimeDefinitionFocus){
		"missing grounding":        func(value *RuntimeDefinitionFocus) { value.RequireTargetGrounding = false },
		"missing selector ref":     func(value *RuntimeDefinitionFocus) { value.TargetSelectorRef = "" },
		"missing selector owner":   func(value *RuntimeDefinitionFocus) { value.TargetSelectorOwnerRef = "" },
		"missing selector subject": func(value *RuntimeDefinitionFocus) { value.TargetSelectorSubjectRef = "" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := compiled[0].Clone()
			mutate(&invalid)
			if err := ValidateRuntimeDefinitionFocuses(seed, []RuntimeDefinitionFocus{invalid}); err == nil {
				t.Fatalf("definition focus accepted selector candidates without grounding and a complete selector: %+v", invalid)
			}
		})
	}
	seedFocus := compiled[0].RuntimeFocusSeed()
	if len(seedFocus.TargetRefs) != 0 || len(seedFocus.ProspectiveTargetAllOfConstraintRefs) != 0 || len(seedFocus.ProspectiveTargetRefs) != 0 || len(seedFocus.AcceptedStepKinds) != 0 || seedFocus.ValidationRecipeID != "" || len(seedFocus.RequiredCheckRefs) != 0 {
		t.Fatalf("initial generation queue contains pre-sealed authority: %+v", seedFocus)
	}
}

func TestRuntimeDefinitionFocusRejectsAcquisitionSelectorDrift(t *testing.T) {
	requirement := RuntimeOwnerFactRequirement{OwnerModuleID: RuntimeModuleShell, FactKind: RuntimeOwnerFactSelectableValidationSurface, OperationKind: RuntimeBlockKindManagedValidationSession}
	convergence := RuntimeOwnerFactConvergence{
		Requirement: requirement, MissingTrigger: RuntimeOwnerFactMissingTrigger,
		ProspectiveSatisfactionContractRef: "shell/managed-validation/workspace-manifest-v1",
		AcquisitionOwnerModuleID:           RuntimeModuleFile, AcquisitionOperationKind: StepKindFileWrite,
		AcquisitionTargetPolicy: RuntimeOperationTargetPolicyProspectiveOwnerTarget, AcquisitionTargetSelectorRef: "file/new-target",
		TargetSelectorRef: "file/workspace-deliverable", TargetSelectorOwnerRef: "profile-a", TargetSelectorSubjectRef: "validation",
		ReobserveOwnerModuleID: RuntimeModuleShell, ReobserveFactKind: RuntimeOwnerFactSelectableValidationSurface, MaxAcquisitionAttempts: 1,
	}
	seed := RuntimeDefinitionSeed{
		SchemaVersion: RuntimeDefinitionSeedSchemaCurrent, DefinitionRevision: "definition-1", PolicyHash: "policy-1",
		CoverageItems: []RuntimeCoverageItem{{ID: "cwi-1", SliceID: "slice-1", SourceScope: "dodItem", SourceID: "dod-1"}},
		Slices:        []RuntimeAcceptanceSlice{{ID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, CompletionPolicy: RuntimeAcceptanceSliceCompletionAll, ResultPolicy: RuntimeAcceptanceSliceResultAllowed}},
	}.Normalized()
	seed.Hash = StableRuntimeDefinitionSeedHash(seed)
	focus := RuntimeDefinitionFocus{
		ID: "focus-1", SliceID: "slice-1", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-1"}, RequiredEvidenceKinds: []string{EvidenceKindValidation},
		TargetSelectorRef: "file/other", TargetSelectorOwnerRef: "profile-a", TargetSelectorSubjectRef: "validation",
		RequiredOwnerFacts: []RuntimeOwnerFactRequirement{requirement}, OwnerFactConvergences: []RuntimeOwnerFactConvergence{convergence},
	}
	if err := ValidateRuntimeDefinitionFocuses(seed, []RuntimeDefinitionFocus{focus}); err == nil {
		t.Fatal("definition/acquisition selector drift was accepted before Run activation")
	}
}
