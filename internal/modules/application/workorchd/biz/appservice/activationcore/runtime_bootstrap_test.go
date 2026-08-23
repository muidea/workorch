package activationcore

import (
	"fmt"
	"strings"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func testAcceptanceSlicePolicy() *runtimecontract.RuntimeAcceptanceSlicePolicy {
	return &runtimecontract.RuntimeAcceptanceSlicePolicy{
		SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent,
		MaxWorkItems:  4, MaxFocuses: 16, MaxCandidateContextItems: 4,
		Groups: []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{
			ID: "slice-default", Title: "Default", Scope: "default", Priority: 100, MaxWorkItems: 1,
			Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true},
		}},
	}
}

// withTestDeterministicProof supplies the same explicit declaration a real
// profile/template must provide after C12.  It deliberately lives in tests:
// production activation must reject an undeclared proof instead of inventing
// a target, validation recipe, or accepted operation from Goal/DoD prose.
func withTestDeterministicProof(goal *workspacemodel.Goal, dod *workspacemodel.DoD, activation runtimecontract.RuntimeActivationContext) runtimecontract.RuntimeActivationContext {
	const profileID = "test-deterministic-proof"
	const recipeID = "test-deterministic-validation"
	// Target grounding is independent from semantic proof credit. Keep a
	// generic test-only fallback so fixtures that are intentionally focused on
	// a different rule remain structurally complete; exact declarations below
	// still own every recipe/check.
	for _, scope := range []string{"dodItem", "goalCoverage", "acceptanceDimension", "deliverable"} {
		activation.ExecutionContract.ProfileContracts = append(activation.ExecutionContract.ProfileContracts, runtimecontract.RuntimeProfileContract{
			ID: profileID + "-grounding-" + scope,
			EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
				ID: "*", Scope: scope, TargetRole: "test_target",
				TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
				TargetSelectorRef:      "file/test_deterministic_proof", TargetSelectorOwnerRef: profileID, TargetSelectorSubjectRef: scope,
				RequireTargetGrounding: true,
			}},
		})
	}
	// Semantic proof must be declared per CWI source. The fixture supplies only
	// recipe/check declarations, preserving any target/operation policy that a
	// test's profile intentionally declares.
	semantic := make(map[string]map[string]struct{})
	add := func(scope, id string) {
		if strings.TrimSpace(scope) == "" || strings.TrimSpace(id) == "" {
			return
		}
		if semantic[scope] == nil {
			semantic[scope] = map[string]struct{}{}
		}
		semantic[scope][id] = struct{}{}
	}
	if dod != nil {
		for idx, item := range dod.Items {
			if strings.TrimSpace(item) != "" {
				add("dodItem", fmt.Sprintf("dod-%03d", idx+1))
			}
		}
	}
	for _, profile := range activation.ExecutionContract.ProfileContracts {
		for _, dimension := range profile.AcceptanceDimensions {
			if dimension.Required {
				add("acceptanceDimension", dimension.ID)
			}
		}
		for _, deliverable := range profile.DeliverableExpectations {
			if deliverable.Required {
				add("deliverable", deliverable.ID)
			}
		}
	}
	for scope, ids := range semantic {
		requirements := make([]runtimecontract.RuntimeProfileEvidenceRequirement, 0, len(ids))
		for id := range ids {
			requirements = append(requirements, runtimecontract.RuntimeProfileEvidenceRequirement{
				ID: id, Scope: scope, RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
				AcceptedStepKinds:  []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
				ValidationRecipeID: recipeID, RequiredCheckRefs: []string{"test-deterministic-check/" + id},
			})
		}
		activation.ExecutionContract.ProfileContracts = append(activation.ExecutionContract.ProfileContracts, runtimecontract.RuntimeProfileContract{ID: profileID + "-" + scope, EvidenceRequirements: requirements})
	}
	activation.EvidencePolicies = append(activation.EvidencePolicies, runtimecontract.RuntimeEvidencePolicy{
		ID: recipeID, AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "true", RecipeDeclared: true,
	})
	return activation
}

func runtimeBootstrapWithTestProof(goal *workspacemodel.Goal, dod *workspacemodel.DoD, definitionRevision string, changeImpact *runtimecontract.RuntimeDefinitionChangeImpact, activation runtimecontract.RuntimeActivationContext) (*runtimecontract.RuntimeBootstrap, error) {
	return RuntimeBootstrap(goal, dod, definitionRevision, "startup-input-test", changeImpact, withTestDeterministicProof(goal, dod, activation))
}

func TestRuntimeBootstrapAcceptsDefinitionSeedWithoutPrecompiledProof(t *testing.T) {
	bootstrap, err := RuntimeBootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver a capability"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"verify behavior"}},
		"definition-1", "startup-input-test", nil,
		runtimecontract.RuntimeActivationContext{
			ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
			ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(), StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce},
		},
	)
	if err != nil || bootstrap == nil {
		t.Fatalf("definition seed bootstrap=%+v err=%v", bootstrap, err)
	}
	if err := bootstrap.DefinitionSeed.Validate(); err != nil || len(bootstrap.Focuses) != 1 || len(bootstrap.Focuses[0].CoverageEvidenceBindings) != 0 {
		t.Fatalf("unsealed semantic bootstrap=%+v seedErr=%v", bootstrap, err)
	}
}

// C12/T22: a validation-capable diagnostic operation is not automatically a
// coverage proof. File read receipts have no owner-executed recipe/check
// observation, so activation must reject this profile before a Run, LLM call,
// operation, or ticket can exist.
func TestRuntimeBootstrapLeavesReadOnlyValidationProposalUnsealed(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: testAcceptanceSlicePolicy(),
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID:                      "invalid-read-proof",
				DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{ID: "read-proof", Required: true}},
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
					// The ordinary DoD focus has a valid proof so this test reaches
					// the deliberately invalid deliverable declaration below.
					{ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
						AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, TargetSelectorRef: "file/workspace",
						TargetSelectorOwnerRef: "invalid-read-proof", TargetSelectorSubjectRef: "dod-item",
						ValidationRecipeID: "valid-dod-recipe", RequiredCheckRefs: []string{"check/dod"}},
					{ID: "read-proof", Scope: "deliverable", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
						AcceptedStepKinds: []string{runtimecontract.StepKindFileRead}, TargetSelectorRef: "file/workspace",
						TargetSelectorOwnerRef: "invalid-read-proof", TargetSelectorSubjectRef: "read-proof",
						ValidationRecipeID: "declared-but-unverifiable", RequiredCheckRefs: []string{"check/read"}},
				},
			}},
		},
	}
	bootstrap, err := RuntimeBootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "inspect workspace"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"inspection is verified"}},
		"definition-1", "startup-input-test", nil, activation,
	)
	if err != nil || bootstrap == nil || len(bootstrap.Focuses) != 2 {
		t.Fatalf("read-only semantic seed bootstrap=%+v err=%v", bootstrap, err)
	}
	for _, focus := range bootstrap.Focuses {
		if len(focus.CoverageEvidenceBindings) != 0 {
			t.Fatalf("activation sealed owner proof: %+v", focus)
		}
	}
}

func TestRuntimeBootstrapKeepsSharedValidationDeclarationAsNonAuthoritativeSeed(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
			ID: "shared-validation", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "true", RecipeDeclared: true,
		}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
				SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent,
				MaxWorkItems:  2, MaxFocuses: 4, MaxCandidateContextItems: 4,
				Groups: []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{ID: "combined", Title: "combined", Scope: "combined", Priority: 1, MaxWorkItems: 2, Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true}}},
			},
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "profile",
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
					ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
					AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
					TargetRefs:        []string{"workspace/implementation"}, TargetRole: "implementation",
					ValidationRecipeID: "shared-validation", RequiredCheckRefs: []string{"check/generic"},
				}},
			}},
		},
	}
	bootstrap, err := RuntimeBootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"behavior one", "behavior two"}},
		"definition-1", "startup-input-test", nil, activation,
	)
	if err != nil || bootstrap == nil || len(bootstrap.DefinitionSeed.CoverageItems) != 2 {
		t.Fatalf("shared validation seed bootstrap=%+v err=%v", bootstrap, err)
	}
}

func TestRuntimeBootstrapDefersMissingSemanticFieldsToPlanning(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: testAcceptanceSlicePolicy(),
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "profile",
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
					ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
					AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
					TargetSelectorRef: "file/workspace", TargetSelectorOwnerRef: "profile", TargetSelectorSubjectRef: "dodItem",
				}},
			}},
		},
	}
	goal := &workspacemodel.Goal{ID: "goal", WorkspaceID: "workspace", Text: "goal"}
	dod := &workspacemodel.DoD{ID: "dod", WorkspaceID: "workspace", Items: []string{"first", "second"}}
	bootstrap, err := RuntimeBootstrap(goal, dod, "definition", "startup-input-test", nil, activation)
	if err != nil || bootstrap == nil || len(bootstrap.DefinitionSeed.CoverageItems) != 2 {
		t.Fatalf("semantic planning seed bootstrap=%+v err=%v", bootstrap, err)
	}
}

func TestRuntimeBootstrapDoesNotClaimShellOwnerRecipeFeasibility(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: testAcceptanceSlicePolicy(),
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "profile",
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
					ID: "dod-001", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
					AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, TargetRefs: []string{"cmd/bridge/main.go"},
					ValidationRecipeID: "missing-shell-recipe", RequiredCheckRefs: []string{"bridge-starts"},
				}},
			}},
		},
	}
	goal := &workspacemodel.Goal{ID: "goal", WorkspaceID: "workspace", Text: "goal"}
	dod := &workspacemodel.DoD{ID: "dod", WorkspaceID: "workspace", Items: []string{"bridge starts"}}
	bootstrap, err := RuntimeBootstrap(goal, dod, "definition", "startup-input-test", nil, activation)
	if err != nil || bootstrap == nil || len(bootstrap.Focuses) != 1 || len(bootstrap.Focuses[0].CoverageEvidenceBindings) != 0 {
		t.Fatalf("bootstrap shell recipe seed=%+v err=%v", bootstrap, err)
	}
}

func TestRuntimeBootstrapDefersCWIValidationCheckToSemanticPlanning(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
			ID: "validation", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "true", RecipeDeclared: true,
		}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: testAcceptanceSlicePolicy(),
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "profile",
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
					ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
					AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
					TargetRefs:        []string{"workspace/implementation"}, TargetRole: "implementation",
					ValidationRecipeID: "validation",
				}},
			}},
		},
	}
	bootstrap, err := RuntimeBootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"behavior one"}},
		"definition-1", "startup-input-test", nil, activation,
	)
	if err != nil || bootstrap == nil || len(bootstrap.DefinitionSeed.CoverageItems) != 1 {
		t.Fatalf("missing CWI check seed bootstrap=%+v err=%v", bootstrap, err)
	}
}

func TestRuntimeBootstrapAllowsPerCWIVariationWithSharedRecipe(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
			ID: "shared-validation", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "true", RecipeDeclared: true,
		}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
				SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent,
				MaxWorkItems:  2, MaxFocuses: 4, MaxCandidateContextItems: 4,
				Groups: []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{ID: "combined", Title: "combined", Scope: "combined", Priority: 1, MaxWorkItems: 2, Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true}}},
			},
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "profile",
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
					{ID: "dod-001", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, TargetRefs: []string{"workspace/one"}, TargetRole: "implementation", ValidationRecipeID: "shared-validation", RequiredCheckRefs: []string{"check/one"}},
					{ID: "dod-002", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, TargetRefs: []string{"workspace/two"}, TargetRole: "implementation", ValidationRecipeID: "shared-validation", RequiredCheckRefs: []string{"check/two"}},
				},
			}},
		},
	}
	bootstrap, err := RuntimeBootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"behavior one", "behavior two"}},
		"definition-1", "startup-input-test", nil, activation,
	)
	if err != nil {
		t.Fatalf("per-CWI proof bootstrap: %v", err)
	}
	if len(bootstrap.Focuses) != 2 {
		t.Fatalf("per-CWI proof focus count = %d, want 2", len(bootstrap.Focuses))
	}
	for _, focus := range bootstrap.Focuses {
		if len(focus.CoverageWorkItemIDs) != 1 || len(focus.CoverageEvidenceBindings) != 0 {
			t.Fatalf("activation sealed semantic proof: %+v", focus)
		}
	}
}

func TestRuntimeBootstrapSeparatesFourValidationCWIsInsideOnePresentationSlice(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
			ID: "shared-validation", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "true", RecipeDeclared: true,
		}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
				SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent, MaxWorkItems: 4, MaxFocuses: 4, MaxCandidateContextItems: 4,
				Groups: []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{ID: "combined", Title: "combined", Scope: "combined", Priority: 1, MaxWorkItems: 4, Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true}}},
			},
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "profile",
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
					{ID: "dod-001", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, TargetRefs: []string{"workspace/one"}, ValidationRecipeID: "shared-validation", RequiredCheckRefs: []string{"check/one"}},
					{ID: "dod-002", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, TargetRefs: []string{"workspace/two"}, ValidationRecipeID: "shared-validation", RequiredCheckRefs: []string{"check/two"}},
					{ID: "dod-003", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, TargetRefs: []string{"workspace/three"}, ValidationRecipeID: "shared-validation", RequiredCheckRefs: []string{"check/three"}},
					{ID: "dod-004", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}, TargetRefs: []string{"workspace/four"}, ValidationRecipeID: "shared-validation", RequiredCheckRefs: []string{"check/four"}},
				},
			}},
		},
	}
	bootstrap, err := RuntimeBootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"one", "two", "three", "four"}},
		"definition-1", "startup-input-test", nil, activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(bootstrap.Slices) != 1 || len(bootstrap.Slices[0].CoverageWorkItemIDs) != 4 || len(bootstrap.Focuses) != 4 {
		t.Fatalf("presentation slices/focuses = %+v / %+v, want one four-item slice and four focuses", bootstrap.Slices, bootstrap.Focuses)
	}
	seen := map[common.CoverageWorkItemID]struct{}{}
	for _, focus := range bootstrap.Focuses {
		if focus.SliceID != bootstrap.Slices[0].ID || len(focus.CoverageWorkItemIDs) != 1 || len(focus.CoverageEvidenceBindings) != 0 {
			t.Fatalf("semantic focus = %+v, want one unsealed CWI", focus)
		}
		coverageID := focus.CoverageWorkItemIDs[0]
		if _, duplicate := seen[coverageID]; duplicate {
			t.Fatalf("CWI %s appears in more than one focus", coverageID)
		}
		seen[coverageID] = struct{}{}
	}
}

func TestRuntimeBootstrapDefinitionSeedCoversEveryCWIWithoutPreflightFailure(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy()},
	}
	bootstrap, err := RuntimeBootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"one", "two", "three", "four"}},
		"definition-1", "startup-input-test", nil, activation,
	)
	if err != nil || bootstrap == nil || len(bootstrap.DefinitionSeed.CoverageItems) != 4 {
		t.Fatalf("bootstrap=%+v err=%v", bootstrap, err)
	}
}

// C12/T22: a template evidence-policy declaration must preserve its bounded
// target through the typed payload/compiler path.  Dropping it would make a
// valid declaration indistinguishable from an undeclared proof and either
// fail activation or tempt a later runtime component to infer a target.
func TestRuntimeBootstrapDoesNotCarryEvidencePolicyBindingTarget(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
			ID: "source-present", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "true", RecipeDeclared: true,
			CoverageRequirements: []runtimecontract.RuntimeCoverageEvidenceRequirement{{
				ID: "dod-001", Scope: "dod_item", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
				AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
				TargetRole:        "verification_target", TargetRefs: []string{"runtime-validation-source.txt"}, RequireTargetGrounding: true,
				ValidationRecipeID: "source-present", RequiredCheckRefs: []string{"check/source-present"},
			}},
		}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: testAcceptanceSlicePolicy(),
			StateChangePolicy:     runtimecontract.RuntimeStateChangeAllowedButNotRequired,
		},
	}
	bootstrap, err := RuntimeBootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "verify source"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"source exists"}},
		"definition-1", "startup-input-test", nil, activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(bootstrap.Focuses) != 1 || len(bootstrap.Focuses[0].CoverageEvidenceBindings) != 0 {
		t.Fatalf("activation sealed coverage bindings = %+v", bootstrap.Focuses)
	}
	assertRuntimeBootstrapHasNoSemanticFocusAuthority(t, bootstrap.Focuses)
}

func TestRuntimeBootstrapBuildsOrderedDoDFocuses(t *testing.T) {
	bootstrap, err := runtimeBootstrapWithTestProof(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "交付完整能力"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"实现行为一", "验证行为二", "覆盖行为三"}},
		"definition-1",
		nil,
		runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}, ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(), StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(bootstrap.Focuses) != 3 || bootstrap.Focuses[0].ID != "focus-001" || bootstrap.Focuses[2].CoverageWorkItemIDs[0] != "cwi-003" {
		t.Fatalf("bootstrap focuses = %+v", bootstrap.Focuses)
	}
	if bootstrap.Definition.GoalDefinition != "交付完整能力" || bootstrap.Definition.DoDDefinition == "" || len(bootstrap.Requirements) != 9 {
		t.Fatalf("bootstrap = %+v", bootstrap)
	}
	if !containsBootstrapCapability(bootstrap.Requirements, runtimecontract.RuntimeModuleFile, runtimecontract.RuntimeCapabilityClassValidation, runtimecontract.StepKindFileRead) {
		t.Fatalf("bootstrap must provide a non-mutating file/read validation capability: %+v", bootstrap.Requirements)
	}
	if !containsBootstrapCapability(bootstrap.Requirements, runtimecontract.RuntimeModuleShell, runtimecontract.RuntimeCapabilityClassValidation, runtimecontract.RuntimeBlockKindManagedValidationSession) {
		t.Fatalf("bootstrap must provide managed validation capability: %+v", bootstrap.Requirements)
	}
	if containsBootstrapCapability(bootstrap.Requirements, runtimecontract.RuntimeModuleShell, runtimecontract.RuntimeCapabilityClassValidation, runtimecontract.StepKindShellCommand) {
		t.Fatalf("bootstrap must not seal side-effecting shell/command as default validation capability: %+v", bootstrap.Requirements)
	}
	if !containsBootstrapOperationAdmissibility(bootstrap.Requirements, runtimecontract.RuntimeModuleFile, runtimecontract.StepKindFileEdit, runtimecontract.StepKindFileRead) ||
		!containsBootstrapOperationAdmissibility(bootstrap.Requirements, runtimecontract.RuntimeModuleShell, runtimecontract.StepKindShellCommand, runtimecontract.RuntimeBlockKindManagedValidationSession) {
		t.Fatalf("bootstrap must seal owner operation-admissibility prerequisites: %+v", bootstrap.Requirements)
	}
	facts, err := bootstrap.InitialFacts(common.WorkspaceID("ws-1"), common.RunID("run-1"))
	if err != nil {
		t.Fatal(err)
	}
	if facts.Focus.ID != "focus-001" || len(facts.RemainingFocuses) != 2 || facts.EvidenceRevision != runtimecontract.InitialRuntimeEvidenceRevision {
		t.Fatalf("initial facts = %+v", facts)
	}
	if bootstrap.RecoveryPolicy.MaxPrerequisiteRefreshes != runtimecontract.DefaultMaxPrerequisiteRefreshes {
		t.Fatalf("activation did not seal stale prerequisite refresh budget: %+v", bootstrap.RecoveryPolicy)
	}
	for _, requirement := range bootstrap.Requirements {
		if requirement.Kind == runtimecontract.PrerequisiteKindValidationScratch {
			if requirement.Scope != runtimecontract.RuntimePrerequisiteScopeGlobal || requirement.FocusID != "" {
				t.Fatalf("activation validation scratch must be run-global: %+v", requirement)
			}
			return
		}
	}
	t.Fatal("activation bootstrap has no validation scratch prerequisite")
}

func containsBootstrapOperationAdmissibility(requirements []runtimecontract.RuntimeRequirement, owner string, kinds ...string) bool {
	for _, requirement := range requirements {
		if requirement.Kind != runtimecontract.PrerequisiteKindOperationAdmissibility || requirement.OwnerModuleID != owner || len(requirement.RequiredCapabilities) != len(kinds) {
			continue
		}
		matches := true
		for idx, kind := range kinds {
			if requirement.RequiredCapabilities[idx] != kind {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func containsBootstrapCapability(requirements []runtimecontract.RuntimeRequirement, owner string, class runtimecontract.RuntimeCapabilityClass, kind string) bool {
	for _, requirement := range requirements {
		if requirement.Kind == runtimecontract.PrerequisiteKindCapability && requirement.OwnerModuleID == owner && requirement.CapabilityClass == class &&
			len(requirement.RequiredCapabilities) == 1 && requirement.RequiredCapabilities[0] == kind {
			return true
		}
	}
	return false
}

func TestRuntimeBootstrapAllowsValidationOnlyFocusWhenStateChangeIsOptional(t *testing.T) {
	bootstrap, err := runtimeBootstrapWithTestProof(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "完成只读核对"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"核对结果已验证"}},
		"definition-1",
		nil,
		runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}, ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(), StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired}},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{runtimecontract.EvidenceKindValidation}
	if len(bootstrap.Focuses) != 1 || len(bootstrap.Focuses[0].RequiredEvidenceKinds) != 1 || bootstrap.Focuses[0].RequiredEvidenceKinds[0] != want[0] {
		t.Fatalf("optional state-change focus = %+v, want validation only", bootstrap.Focuses)
	}
}

func TestRuntimeBootstrapSealsConfiguredConvergencePolicy(t *testing.T) {
	bootstrap, err := runtimeBootstrapWithTestProof(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"validate"}},
		"definition-1",
		nil,
		runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}, ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(),
			MaxRepairAttemptsPerFocus: 7,
			ValidationPolicies: []runtimecontract.RuntimeValidationPolicy{{
				ID: "validation", Scope: "workspace_profile", AppliesToEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, MaxValidationOnlyRetriesPerFocus: 2,
			}},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrap.ConvergencePolicy.MaxRepairAttemptsPerFocus != 7 || bootstrap.ConvergencePolicy.MaxValidationOnlyRetriesPerFocus != 2 {
		t.Fatalf("convergence policy = %+v", bootstrap.ConvergencePolicy)
	}
	repairBudget, found := runtimecontract.RuntimeBudgetDimensionPolicyFor(bootstrap.ExecutionBudgetPolicy, runtimecontract.RuntimeBudgetDimensionSameFocusRepair)
	if !found || repairBudget.Limit != 7 {
		t.Fatalf("same-focus execution budget = %+v found=%v", repairBudget, found)
	}
	recoveryBudget, found := runtimecontract.RuntimeBudgetDimensionPolicyFor(bootstrap.ExecutionBudgetPolicy, runtimecontract.RuntimeBudgetDimensionOperationRecovery)
	if !found || recoveryBudget.Limit != int64(bootstrap.RecoveryPolicy.MaxOperationRecoveryAttempts) {
		t.Fatalf("operation-recovery execution budget = %+v found=%v", recoveryBudget, found)
	}
}

func TestRuntimeBootstrapAppendsRequiredProfileFocusesWithTypedRequirements(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}, ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(),
		StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
		ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID:                   "software-dev",
			AcceptanceDimensions: []runtimecontract.RuntimeAcceptanceDimension{{ID: "runtime_entrypoint", Required: true, Description: "entrypoint works"}},
			DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{
				{ID: "source_change", Required: true, EvidenceRole: "primary_state_change", Description: "workspace changed"},
				{ID: "coverage_map", Required: true, EvidenceRole: "supplemental_evidence", Description: "coverage is mapped"},
				{ID: "version_control_snapshot", Required: true, EvidenceRole: "supplemental_evidence", Description: "snapshot exists"},
			},
			EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
				{ID: "runtime_entrypoint", Scope: "acceptanceDimension", AcceptedStepKinds: []string{"shell/command", runtimecontract.RuntimeBlockKindManagedValidationSession}, RequiredOwnerFacts: []runtimecontract.RuntimeOwnerFactRequirement{{
					OwnerModuleID: runtimecontract.RuntimeModuleShell, FactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface,
					OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession,
				}}, OwnerFactConvergences: []runtimecontract.RuntimeOwnerFactConvergence{{
					Requirement:        runtimecontract.RuntimeOwnerFactRequirement{OwnerModuleID: runtimecontract.RuntimeModuleShell, FactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession},
					RequirePreexisting: true,
				}}},
				{ID: "source_change", Scope: "deliverable", EvidenceRole: "primary_state_change", AcceptedStepKinds: []string{"patch/apply"}},
				{ID: "version_control_snapshot", Scope: "deliverable", AcceptedStepKinds: []string{"git/write"}, AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{Kind: "git/write", VariantID: "commit"}}},
			},
		}},
	}}
	bootstrap, err := runtimeBootstrapWithTestProof(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "交付完整能力"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为已验证"}},
		"definition-1", nil, activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(bootstrap.Focuses) != 5 {
		t.Fatalf("profile focuses = %+v", bootstrap.Focuses)
	}
	assertRuntimeBootstrapHasNoSemanticFocusAuthority(t, bootstrap.Focuses)
	compiled, err := compileRuntimeCoverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "交付完整能力"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为已验证"}},
		withTestDeterministicProof(
			&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "交付完整能力"},
			&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为已验证"}}, activation),
	)
	if err != nil || len(compiled.Focuses) != 5 {
		t.Fatalf("compiled profile focuses=%+v err=%v", compiled.Focuses, err)
	}
	entrypoint, sourceChange, coverageMap, snapshot := compiled.Focuses[1], compiled.Focuses[2], compiled.Focuses[3], compiled.Focuses[4]
	if entrypoint.AssessmentScope != "acceptanceDimension" || len(entrypoint.RequiredEvidenceKinds) != 1 || entrypoint.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindValidation ||
		len(entrypoint.RequiredOwnerFacts) != 1 || entrypoint.RequiredOwnerFacts[0] != (runtimecontract.RuntimeOwnerFactRequirement{OwnerModuleID: runtimecontract.RuntimeModuleShell, FactKind: runtimecontract.RuntimeOwnerFactSelectableValidationSurface, OperationKind: runtimecontract.RuntimeBlockKindManagedValidationSession}) ||
		sourceChange.EvidenceRole != "primary_state_change" || sourceChange.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange ||
		len(coverageMap.RequiredEvidenceKinds) != 1 || coverageMap.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindValidation || !coverageMap.RequireAssessment ||
		len(snapshot.AcceptedOperationVariants) != 1 || snapshot.AcceptedOperationVariants[0] != (runtimecontract.RuntimeOperationVariantRef{Kind: "git/write", VariantID: "commit"}) || snapshot.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange {
		t.Fatalf("typed profile focuses = %+v", compiled.Focuses)
	}
	foundGit := false
	for _, requirement := range bootstrap.Requirements {
		if requirement.OwnerModuleID == runtimecontract.RuntimeModuleGit && requirement.CapabilityClass == runtimecontract.RuntimeCapabilityClassStateChange &&
			len(requirement.RequiredCapabilities) == 1 && requirement.RequiredCapabilities[0] == "git/write" {
			foundGit = true
		}
	}
	if !foundGit {
		t.Fatalf("profile capability requirements = %+v, want git/write state-change", bootstrap.Requirements)
	}
}

func TestRuntimeBootstrapBuildsBoundedGitCapabilityStateFixture(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}, ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(),
		StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
		ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID: "bounded-git",
			DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{
				ID: "version_control_snapshot", Required: true, EvidenceRole: "primary_state_change", Description: "local checkpoint",
			}},
			EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
				ID: "version_control_snapshot", Scope: "deliverable", EvidenceRole: "primary_state_change",
				RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}, AcceptedStepKinds: []string{runtimecontract.StepKindGitWrite},
				AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindGitWrite, VariantID: "commit"}},
				CapabilityStateRequirements: []runtimecontract.RuntimeProfileCapabilityStateRequirement{{
					OwnerModuleID: runtimecontract.RuntimeModuleGit, CapabilityStateKind: "repository_state", SubjectRef: "workspace_deliverable",
				}},
			}},
		}},
	}}
	bootstrap, err := runtimeBootstrapWithTestProof(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "create a local checkpoint and validate fixture content"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"fixture content is validated"}},
		"definition-1", nil, activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(bootstrap.Focuses) != 2 || len(bootstrap.Slices) != 2 || len(bootstrap.Focuses[0].CoverageWorkItemIDs) != 1 || len(bootstrap.Focuses[1].CoverageWorkItemIDs) != 1 {
		t.Fatalf("bounded Git fixture focuses/slices = %+v / %+v", bootstrap.Focuses, bootstrap.Slices)
	}
	assertRuntimeBootstrapHasNoSemanticFocusAuthority(t, bootstrap.Focuses)
	compiled, err := compileRuntimeCoverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "create a local checkpoint and validate fixture content"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"fixture content is validated"}},
		withTestDeterministicProof(
			&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "create a local checkpoint and validate fixture content"},
			&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"fixture content is validated"}}, activation),
	)
	if err != nil || len(compiled.Focuses) != 2 {
		t.Fatalf("compiled Git focuses=%+v err=%v", compiled.Focuses, err)
	}
	validation, gitFocus := compiled.Focuses[0], compiled.Focuses[1]
	if validation.AssessmentScope != "dodItem" || len(validation.RequiredEvidenceKinds) != 1 || validation.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindValidation ||
		gitFocus.AssessmentScope != "deliverable" || gitFocus.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange ||
		len(gitFocus.AcceptedOperationVariants) != 1 || gitFocus.AcceptedOperationVariants[0] != (runtimecontract.RuntimeOperationVariantRef{Kind: runtimecontract.StepKindGitWrite, VariantID: "commit"}) {
		t.Fatalf("bounded Git fixture focus contracts = %+v", compiled.Focuses)
	}
	foundStateRequirement := false
	for _, requirement := range bootstrap.Requirements {
		if requirement.Kind == runtimecontract.PrerequisiteKindCapabilityState && requirement.OwnerModuleID == runtimecontract.RuntimeModuleGit &&
			requirement.CapabilityStateKind == "repository_state" && requirement.SubjectRef == "workspace_deliverable" {
			foundStateRequirement = true
		}
	}
	if !foundStateRequirement {
		t.Fatalf("bounded Git fixture lacks capability-state prerequisite: %+v", bootstrap.Requirements)
	}
}

func assertRuntimeBootstrapHasNoSemanticFocusAuthority(t *testing.T, focuses []runtimecontract.CoverageFocus) {
	t.Helper()
	for _, focus := range focuses {
		if len(focus.CoverageEvidenceBindings) != 0 || focus.EvidenceDeclarationRef != "" || len(focus.TargetRefs) != 0 ||
			focus.TargetRole != "" || len(focus.TargetDiscoverySources) != 0 || focus.TargetSelectorRef != "" ||
			focus.TargetSelectorOwnerRef != "" || focus.TargetSelectorSubjectRef != "" || len(focus.TargetSelectorCandidateRefs) != 0 ||
			focus.ProspectiveTargetRef != "" || focus.RequireTargetGrounding || len(focus.AcceptedStepKinds) != 0 ||
			len(focus.AcceptedOperationVariants) != 0 || len(focus.RejectedStepKinds) != 0 || focus.RequireDirectEvidence ||
			focus.RequireClaimEvidence || focus.RequireExecutableBehaviorEvidence || focus.RequireCoverageTargets ||
			focus.ValidationRecipeID != "" || len(focus.RequiredCheckRefs) != 0 || len(focus.MetricConstraints) != 0 || len(focus.ArtifactRefs) != 0 {
			t.Fatalf("bootstrap focus carries provisional semantic authority: %+v", focus)
		}
	}
}

func TestRuntimeBootstrapRejectsUnknownProfileAcceptedKind(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}, ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(),
		StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
		ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID: "broken-profile",
			EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
				{ID: "mystery", Scope: "acceptanceDimension", AcceptedStepKinds: []string{"totally/unknown"}},
			},
		}},
	}}
	_, err := runtimeBootstrapWithTestProof(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "交付完整能力"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为已验证"}},
		"definition-1", nil, activation,
	)
	if err == nil || !strings.Contains(err.Error(), "no runtime operation descriptor") {
		t.Fatalf("expected unknown accepted kind failure, got %v", err)
	}
}

func TestRuntimeBootstrapSoftwareDevProfileIncludesManagedValidation(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}}, ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(),
		StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
		ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID: "software-dev",
			EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
				{ID: "runtime_entrypoint", Scope: "acceptanceDimension", AcceptedStepKinds: []string{"shell/command", "mcp/tool_call", "validation/managed_session"}},
			},
		}},
	}}
	bootstrap, err := runtimeBootstrapWithTestProof(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "交付完整能力"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为已验证"}},
		"definition-1", nil, activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	foundManaged := false
	foundShellCommand := false
	for _, kind := range bootstrap.Definition.CapabilityContract {
		if kind == runtimecontract.RuntimeBlockKindManagedValidationSession {
			foundManaged = true
		}
		if kind == runtimecontract.StepKindShellCommand {
			foundShellCommand = true
		}
	}
	if !foundManaged || !foundShellCommand {
		t.Fatalf("capability contract = %v, want managed_session and shell/command", bootstrap.Definition.CapabilityContract)
	}
	if !containsBootstrapCapability(bootstrap.Requirements, runtimecontract.RuntimeModuleShell, runtimecontract.RuntimeCapabilityClassValidation, runtimecontract.RuntimeBlockKindManagedValidationSession) {
		t.Fatalf("requirements missing managed validation: %+v", bootstrap.Requirements)
	}
}

func TestRuntimeBootstrapRegistersSelectedReadOnlyMCPToolForValidation(t *testing.T) {
	serverID, toolName := "mcp-1", "repo.summary"
	activation := runtimecontract.RuntimeActivationContext{
		ProviderSelection: runtimecontract.ProviderInferenceSelection{Target: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
		MCPCapabilities: []runtimecontract.RuntimeMCPCapabilityContext{{
			ID: "mcp-1:tool:repo.summary", ServerID: common.MCPServerID(serverID), Kind: "tool", Name: toolName, Enabled: true,
			EffectScope: string(runtimecontract.RuntimeEffectNone), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencySafeToReplay),
		}},
		OperationPolicies: []runtimecontract.RuntimeOperationPolicyFact{{
			ID: "operation-policy/mcp-1:tool:repo.summary", OwnerModuleID: runtimecontract.RuntimeModuleMCP,
			OperationKind: runtimecontract.StepKindMCPToolCall, SubjectRef: "mcp-1:tool:repo.summary",
			ParameterMatches: []runtimecontract.RuntimeOperationParameterMatch{
				{Name: "serverId", String: &serverID}, {Name: "tool", String: &toolName},
			},
			EffectScope: runtimecontract.RuntimeEffectNone, IdempotencyClass: runtimecontract.RuntimeOperationIdempotencySafeToReplay,
		}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: testAcceptanceSlicePolicy(),
			StateChangePolicy:     runtimecontract.RuntimeStateChangeAllowedButNotRequired,
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "mcp-validation",
				DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{
					ID: "repository_summary", Required: true, EvidenceRole: "supplemental_evidence",
				}},
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
					ID: "repository_summary", Scope: "deliverable", EvidenceRole: "supplemental_evidence",
					RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.StepKindMCPToolCall},
				}},
			}},
		},
	}
	bootstrap, err := runtimeBootstrapWithTestProof(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "obtain a bounded repository summary"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"repository summary is validated"}},
		"definition-1", nil, activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !containsBootstrapCapability(bootstrap.Requirements, runtimecontract.RuntimeModuleMCP, runtimecontract.RuntimeCapabilityClassValidation, runtimecontract.StepKindMCPToolCall) {
		t.Fatalf("selected read-only MCP tool must be acquired as validation capability: %+v", bootstrap.Requirements)
	}
	if containsBootstrapCapability(bootstrap.Requirements, runtimecontract.RuntimeModuleMCP, runtimecontract.RuntimeCapabilityClassStateChange, runtimecontract.StepKindMCPToolCall) {
		t.Fatalf("selected read-only MCP tool must not be acquired as state-change capability: %+v", bootstrap.Requirements)
	}
}
