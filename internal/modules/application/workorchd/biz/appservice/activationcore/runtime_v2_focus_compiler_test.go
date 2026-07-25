package activationcore

import (
	"fmt"
	"strings"
	"testing"

	"github.com/muidea/workorch/internal/pkg/common"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCompileRuntimeV2CoverageFocusesAppliesDodItemWildcard(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "software-dev",
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
					ID: "*", Scope: "dodItem",
					TargetRole: "deliverable_implementation", TargetDiscoverySources: []string{
						runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest,
						runtimecontract.RuntimeValidationDiscoverySourceProjectFiles,
					}, TargetSelectorRef: "file/unique_workspace_entry", RequireTargetGrounding: true,
					RequireDirectEvidence: true, RequireClaimEvidence: true, RequireExecutableBehaviorEvidence: true,
					AcceptedStepKinds: []string{"shell/command", "patch/apply", "mcp/tool_call"},
					RejectedStepKinds: []string{"file/read", "file/list", "mcp/resource_read", "mcp/prompt_get"},
				}},
			}},
		},
	}
	focuses, err := compileRuntimeV2CoverageFocuses(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一", "行为二"}},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(focuses) != 2 {
		t.Fatalf("focuses = %+v", focuses)
	}
	for _, focus := range focuses {
		if focus.AssessmentScope != "dodItem" || focus.SliceID == "" || !focus.RequireDirectEvidence || !focus.RequireClaimEvidence || !focus.RequireExecutableBehaviorEvidence {
			t.Fatalf("dod focus policy not applied: %+v", focus)
		}
		if len(focus.AcceptedStepKinds) != 3 || focus.AcceptedStepKinds[0] != "mcp/tool_call" || focus.AcceptedStepKinds[1] != "patch/apply" || focus.AcceptedStepKinds[2] != "shell/command" {
			t.Fatalf("accepted kinds = %v", focus.AcceptedStepKinds)
		}
		if len(focus.RejectedStepKinds) != 4 || focus.RejectedStepKinds[0] != "file/list" || focus.RejectedStepKinds[1] != "file/read" {
			t.Fatalf("rejected kinds = %v", focus.RejectedStepKinds)
		}
		// Run-level must_produce_state_change no longer forces every DoD focus to require state_change.
		if len(focus.RequiredEvidenceKinds) != 1 || focus.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindValidation {
			t.Fatalf("required evidence kinds = %v, want validation only", focus.RequiredEvidenceKinds)
		}
		if focus.TargetRole != "deliverable_implementation" || focus.TargetSelectorRef != "file/unique_workspace_entry" || !focus.RequireTargetGrounding ||
			len(focus.TargetRefs) != 0 || len(focus.TargetDiscoverySources) != 2 {
			t.Fatalf("dod focus target contract not compiled: %+v", focus)
		}
	}
}

func TestRuntimeV2BootstrapDefersTargetSelectionToFileOwner(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: testAcceptanceSlicePolicy(), StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "profile", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
					ID: "*", Scope: "dodItem", TargetRole: "deliverable_implementation",
					TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
					TargetSelectorRef:      "file/unique_workspace_entry", RequireTargetGrounding: true,
					AcceptedStepKinds: []string{"patch/apply"},
				}},
			}},
		},
	}
	bootstrap, err := RuntimeV2Bootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "根据当前交付物完成能力"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"已有交付物通过验收"}},
		"definition-1", nil, activation,
	)
	if err != nil {
		t.Fatalf("bootstrap failed before File owner target selection: %v", err)
	}
	if len(bootstrap.Focuses) != 1 || len(bootstrap.Focuses[0].TargetRefs) != 0 || bootstrap.Focuses[0].TargetSelectorRef != "file/unique_workspace_entry" {
		t.Fatalf("bootstrap target contract = %+v", bootstrap.Focuses)
	}
}

func TestRuntimeV2BootstrapSealsProfileProspectiveTargetIntoFocus(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(), ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID: "profile", DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{ID: "initial-deliverable", Required: true}}, EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
				ID: "initial-deliverable", Scope: "deliverable", RequireTargetGrounding: true, TargetRole: "deliverable",
				TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}, TargetSelectorRef: "file/workspace_deliverable", ProspectiveTargetRef: "initial-deliverable.txt",
				AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite},
			}},
		}}},
	}
	bootstrap, err := RuntimeV2Bootstrap(&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"}, &workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"initial deliverable"}}, "definition-1", nil, activation)
	if err != nil {
		t.Fatal(err)
	}
	var focus *runtimecontract.CoverageFocus
	for idx := range bootstrap.Focuses {
		if bootstrap.Focuses[idx].TargetRole == "deliverable" {
			focus = &bootstrap.Focuses[idx]
			break
		}
	}
	if focus == nil || focus.ProspectiveTargetRef != "initial-deliverable.txt" {
		t.Fatalf("bootstrap lost prospective target: %+v", bootstrap.Focuses)
	}
}

func TestRuntimeV2BootstrapSealsProfileBoundEmptyWorkspaceScopeIntoFocus(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(), ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID: "software-dev", DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{ID: "source_change", Required: true, EvidenceRole: "primary_state_change"}},
			EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
				ID: "source_change", Scope: "deliverable", EvidenceRole: "primary_state_change", RequireTargetGrounding: true, TargetRole: "deliverable_implementation",
				TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}, TargetSelectorRef: "file/workspace_deliverable",
				TargetSelectorOwnerRef: "software-dev", TargetSelectorSubjectRef: "workspace_deliverable_create_scope",
				AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite},
			}},
		}}},
	}
	bootstrap, err := RuntimeV2Bootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "create a minimal Go command"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"main.go exists"}},
		"definition-1", nil, activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	var focus *runtimecontract.CoverageFocus
	for idx := range bootstrap.Focuses {
		if bootstrap.Focuses[idx].TargetRole == "deliverable_implementation" {
			focus = &bootstrap.Focuses[idx]
			break
		}
	}
	if focus == nil || focus.TargetSelectorOwnerRef != "software-dev" || focus.TargetSelectorSubjectRef != "workspace_deliverable_create_scope" || focus.ProspectiveTargetRef != "" {
		t.Fatalf("bootstrap lost profile-bound empty workspace scope: %+v", bootstrap.Focuses)
	}
}

func TestCompileRuntimeV2CoverageFocusesPrependsMatchedBootstrapArtifactsInDependencyOrder(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
			SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2,
			MaxWorkItems:  4, MaxFocuses: 16, MaxCandidateContextItems: 4,
			Groups: []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{
				{ID: "primary", Title: "Primary", Scope: "primary", Priority: 10, MaxWorkItems: 2, Match: runtimecontract.RuntimeAcceptanceSliceMatch{EvidenceRoles: []string{"primary_state_change"}}},
				{ID: "remaining", Title: "Remaining", Scope: "remaining", Priority: 90, MaxWorkItems: 4, Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true}},
			},
		}, ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID: "software-dev",
			BootstrapRecipes: []runtimecontract.RuntimeBootstrapRecipe{{
				ID: "go-service", RequiredGoalTerms: []string{"gopls"}, RequiredDoDTerms: []string{"go test"},
				Artifacts: []runtimecontract.RuntimeBootstrapArtifact{
					{ID: "test", TargetRole: "test_source", TargetRef: "internal/service/main_test.go", AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}, DependsOnArtifactIDs: []string{"source"}},
					{ID: "module", TargetRole: "module_definition", TargetRef: "go.mod", AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}},
					{ID: "source", TargetRole: "implementation", TargetRef: "internal/service/main.go", AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}, DependsOnArtifactIDs: []string{"module"}},
				},
			}},
		}}},
	}
	focuses, err := compileRuntimeV2CoverageFocuses(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "build a gopls integration"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"go test ./... passes"}}, activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(focuses) != 4 {
		t.Fatalf("focus count = %d, want 4: %+v", len(focuses), focuses)
	}
	for idx, want := range []string{"go.mod", "internal/service/main.go", "internal/service/main_test.go"} {
		focus := focuses[idx]
		if focus.TargetSelectorRef != "file/profile_declared_candidates" || focus.ProspectiveTargetRef != want ||
			len(focus.TargetSelectorCandidateRefs) != 1 || focus.TargetSelectorCandidateRefs[0] != want ||
			len(focus.RequiredEvidenceKinds) != 1 || focus.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange ||
			len(focus.AcceptedStepKinds) != 1 || focus.AcceptedStepKinds[0] != runtimecontract.StepKindFileWrite || !focus.RequireTargetGrounding || focus.RequireAssessment {
			t.Fatalf("bootstrap focus %d = %+v", idx, focus)
		}
		if focus.SliceID != "slice-bootstrap-artifact-00"+fmt.Sprint(idx+1) {
			t.Fatalf("bootstrap focus %d was not kept ahead of ordinary acceptance groups: %+v", idx, focus)
		}
	}
	if focuses[3].AssessmentScope != "dodItem" {
		t.Fatalf("DoD focus was not retained after bootstrap sequence: %+v", focuses[3])
	}
}

func TestRuntimeV2BootstrapSealsProfileDeclaredSelectorBinding(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(), ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID: "profile-lsp", DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{ID: "implementation", Required: true}}, EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
				ID: "implementation", Scope: "deliverable", RequireTargetGrounding: true, TargetRole: "implementation",
				TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}, TargetSelectorRef: "file/profile_declared_candidates", TargetSelectorSubjectRef: "implementation", TargetSelectorCandidateRefs: []string{"internal/lsp/manager.go", "internal/lsp/server.go"},
				AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindPatchApply},
			}},
		}}},
	}
	bootstrap, err := RuntimeV2Bootstrap(&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"}, &workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"implementation"}}, "definition-1", nil, activation)
	if err != nil {
		t.Fatal(err)
	}
	var focus *runtimecontract.CoverageFocus
	for idx := range bootstrap.Focuses {
		if bootstrap.Focuses[idx].TargetSelectorRef == "file/profile_declared_candidates" {
			focus = &bootstrap.Focuses[idx]
			break
		}
	}
	if focus == nil || focus.TargetSelectorOwnerRef != "profile-lsp" || focus.TargetSelectorSubjectRef != "implementation" || !runtimecontract.SameRuntimeTargetSelectorCandidateRefs(focus.TargetSelectorCandidateRefs, []string{"internal/lsp/manager.go", "internal/lsp/server.go"}) {
		t.Fatalf("bootstrap lost profile selector binding: %+v", bootstrap.Focuses)
	}
}

// DoD work units precede profile deliverables in a fresh activation. A profile
// that only declares a prospective target on its later source-change item
// therefore still deadlocks an empty workspace at the first direct-evidence
// DoD focus. The declaration must be compiled into that leading focus too;
// File remains the owner that verifies the path and reports the create fact.
func TestRuntimeV2BootstrapSealsProspectiveTargetIntoLeadingDoDFocus(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(), ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID: "profile", EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
				{
					ID: "*", Scope: "dodItem", RequireTargetGrounding: true, TargetRole: "deliverable_implementation",
					TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
					TargetSelectorRef:      "file/workspace_deliverable", ProspectiveTargetRef: "workorch-deliverable.md",
					AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindPatchApply},
				},
				{
					ID: "source_change", Scope: "deliverable", RequireTargetGrounding: true, TargetRole: "deliverable_implementation",
					TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
					TargetSelectorRef:      "file/workspace_deliverable", ProspectiveTargetRef: "workorch-deliverable.md",
					AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite, runtimecontract.RuntimeBlockKindPatchApply},
				},
			},
		}}},
	}
	bootstrap, err := RuntimeV2Bootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"first direct evidence"}},
		"definition-1", nil, activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(bootstrap.Focuses) == 0 {
		t.Fatal("bootstrap has no focuses")
	}
	leading := bootstrap.Focuses[0]
	if leading.AssessmentScope != "dodItem" || leading.TargetSelectorRef != "file/workspace_deliverable" ||
		leading.ProspectiveTargetRef != "workorch-deliverable.md" || !leading.RequireTargetGrounding {
		t.Fatalf("leading DoD focus lost profile creation declaration: %+v", leading)
	}
	if len(leading.AcceptedStepKinds) != 2 || leading.AcceptedStepKinds[0] != runtimecontract.StepKindFileWrite || leading.AcceptedStepKinds[1] != runtimecontract.RuntimeBlockKindPatchApply {
		t.Fatalf("leading DoD focus cannot admit the initial File create: %+v", leading.AcceptedStepKinds)
	}
	hasFileWriteCapability := false
	for _, kind := range bootstrap.Definition.CapabilityContract {
		if kind == runtimecontract.StepKindFileWrite {
			hasFileWriteCapability = true
			break
		}
	}
	if !hasFileWriteCapability {
		t.Fatalf("initial File create capability was not sealed into bootstrap: %+v", bootstrap.Definition.CapabilityContract)
	}
}

func TestResolveCoverageEvidencePolicyCompilesGroundedDirectEvidenceScopes(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "software-dev",
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
					{
						ID: "*", Scope: "dodItem", TargetRole: "deliverable_implementation",
						TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest, runtimecontract.RuntimeValidationDiscoverySourceProjectFiles},
						TargetSelectorRef:      "file/unique_workspace_entry", RequireTargetGrounding: true,
					},
					{
						ID: "*", Scope: "goalCoverage", TargetRole: "deliverable_implementation",
						TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest, runtimecontract.RuntimeValidationDiscoverySourceProjectFiles},
						TargetSelectorRef:      "file/unique_workspace_entry", RequireTargetGrounding: true,
					},
				},
			}},
		},
	}
	for _, scope := range []string{"dodItem", "goalCoverage"} {
		t.Run(scope, func(t *testing.T) {
			policy, err := resolveCoverageEvidencePolicy(scope, "any-item", activation, false)
			if err != nil {
				t.Fatal(err)
			}
			if policy.TargetRole != "deliverable_implementation" || policy.TargetSelectorRef != "file/unique_workspace_entry" || !policy.RequireTargetGrounding ||
				len(policy.TargetRefs) != 0 || len(policy.TargetDiscoverySources) != 2 {
				t.Fatalf("%s target policy = %+v", scope, policy)
			}
		})
	}
}

func TestCompileRuntimeV2CoverageFocusesExactBeatsWildcard(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired,
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "software-dev",
				AcceptanceDimensions: []runtimecontract.RuntimeAcceptanceDimension{{
					ID: "runtime_entrypoint", Required: true, Description: "entrypoint works",
				}},
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
					{
						ID: "*", Scope: "acceptanceDimension",
						AcceptedStepKinds: []string{"shell/command"},
						RejectedStepKinds: []string{"file/read"},
					},
					{
						ID: "runtime_entrypoint", Scope: "acceptanceDimension",
						RequireExecutableBehaviorEvidence: true,
						AcceptedStepKinds:                 []string{"validation/managed_session", "shell/command"},
						RejectedStepKinds:                 []string{"file/read", "file/list"},
						Description:                       "exact entrypoint policy",
					},
				},
			}},
		},
	}
	focuses, err := compileRuntimeV2CoverageFocuses(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一"}},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(focuses) != 2 {
		t.Fatalf("focuses = %+v", focuses)
	}
	entrypoint := focuses[1]
	if entrypoint.AssessmentID != "runtime_entrypoint" {
		t.Fatalf("entrypoint focus = %+v", entrypoint)
	}
	// exact accepted kinds win (first non-empty highest priority), rejected kinds are union
	// Normalized() stably sorts accepted kinds; exact source still excludes wildcard-only values.
	if len(entrypoint.AcceptedStepKinds) != 2 || entrypoint.AcceptedStepKinds[0] != "shell/command" || entrypoint.AcceptedStepKinds[1] != "validation/managed_session" {
		t.Fatalf("exact accepted kinds = %v", entrypoint.AcceptedStepKinds)
	}
	if !entrypoint.RequireExecutableBehaviorEvidence {
		t.Fatalf("exact executable constraint missing: %+v", entrypoint)
	}
	if len(entrypoint.RejectedStepKinds) != 2 {
		t.Fatalf("rejected union = %v", entrypoint.RejectedStepKinds)
	}
}

func TestCompileRuntimeV2CoverageFocusesRejectsUnknownAcceptedKind(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "broken",
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
					ID: "*", Scope: "dodItem", AcceptedStepKinds: []string{"totally/unknown"},
				}},
			}},
		},
	}
	_, err := compileRuntimeV2CoverageFocuses(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一"}},
		activation,
	)
	if err == nil || !strings.Contains(err.Error(), "no runtime operation descriptor") {
		t.Fatalf("expected unknown kind failure, got %v", err)
	}
}

func TestCompileRuntimeV2CoverageFocusesUsesEvidencePolicyCoverageRequirements(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
			ID: "policy-1", AppliesTo: "shell/command",
			CoverageRequirements: []runtimecontract.RuntimeCoverageEvidenceRequirement{{
				ID: "*", Scope: "dod_item",
				RequiredEvidenceKinds:             []string{"validation"},
				RequireDirectEvidence:             true,
				RequireExecutableBehaviorEvidence: true,
				AcceptedStepKinds:                 []string{"shell/command", "mcp/tool_call"},
				RejectedStepKinds:                 []string{"file/read"},
			}},
		}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired,
		},
	}
	focuses, err := compileRuntimeV2CoverageFocuses(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一"}},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	focus := focuses[0]
	if !focus.RequireDirectEvidence || !focus.RequireExecutableBehaviorEvidence {
		t.Fatalf("evidence policy not applied: %+v", focus)
	}
	if len(focus.AcceptedStepKinds) != 2 || focus.AcceptedStepKinds[0] != "mcp/tool_call" || focus.AcceptedStepKinds[1] != "shell/command" {
		t.Fatalf("accepted = %v", focus.AcceptedStepKinds)
	}
	if len(focus.RejectedStepKinds) != 1 || focus.RejectedStepKinds[0] != "file/read" {
		t.Fatalf("rejected = %v", focus.RejectedStepKinds)
	}
}

func TestRuntimeV2BootstrapCompilesDodWildcardIntoFocuses(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2, MaxWorkItems: 4, MaxFocuses: 4, MaxCandidateContextItems: 4},
			StateChangePolicy:     runtimecontract.RuntimeStateChangeMustProduce,
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "software-dev",
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
					ID: "*", Scope: "dodItem",
					RequireDirectEvidence: true, RequireExecutableBehaviorEvidence: true,
					AcceptedStepKinds: []string{"shell/command", "mcp/tool_call"},
					RejectedStepKinds: []string{"file/read"},
				}},
			}},
		},
	}
	bootstrap, err := RuntimeV2Bootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "交付完整能力"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"实现行为一"}},
		"definition-1", nil, activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(bootstrap.Focuses) != 1 {
		t.Fatalf("focuses = %+v", bootstrap.Focuses)
	}
	focus := bootstrap.Focuses[0]
	if !focus.RequireDirectEvidence || !focus.RequireExecutableBehaviorEvidence || len(focus.RejectedStepKinds) == 0 {
		t.Fatalf("bootstrap did not compile dod wildcard policy: %+v", focus)
	}
}

func TestRuntimeV2BootstrapSealsRunLevelStateChangeRequirement(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2, MaxWorkItems: 4, MaxFocuses: 4, MaxCandidateContextItems: 4},
			StateChangePolicy:     runtimecontract.RuntimeStateChangeMustProduce,
		},
	}
	bootstrap, err := RuntimeV2Bootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "交付完整能力"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一", "行为二"}},
		"definition-1", nil, activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bootstrap.StateChangeRequirement.Required {
		t.Fatalf("state-change requirement = %+v", bootstrap.StateChangeRequirement)
	}
	if len(bootstrap.StateChangeRequirement.CoverageWorkItemIDs) != 2 {
		t.Fatalf("coverage ids = %v", bootstrap.StateChangeRequirement.CoverageWorkItemIDs)
	}
	// DoD focuses themselves must not be mechanically forced to state_change.
	for _, focus := range bootstrap.Focuses {
		for _, kind := range focus.RequiredEvidenceKinds {
			if kind == runtimecontract.EvidenceKindStateChange {
				t.Fatalf("DoD focus unexpectedly requires state_change: %+v", focus)
			}
		}
	}
	facts, err := bootstrap.InitialFacts("ws-1", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if !facts.StateChange.Required || facts.StateChange.Satisfied {
		t.Fatalf("initial ledger = %+v", facts.StateChange)
	}
}

func TestCompileRuntimeV2CoverageFocusesKeepsDeliverableStateChange(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "software-dev",
				DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{
					{ID: "source_change", Required: true, EvidenceRole: "primary_state_change", Description: "workspace changed"},
				},
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
					ID: "source_change", Scope: "deliverable", EvidenceRole: "primary_state_change",
					TargetRole: "implementation", TargetRefs: []string{"cmd/main.go"}, TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}, RequireTargetGrounding: true,
					AcceptedStepKinds: []string{"patch/apply"},
				}},
			}},
		},
	}
	focuses, err := compileRuntimeV2CoverageFocuses(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一"}},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(focuses) != 2 {
		t.Fatalf("focuses = %+v", focuses)
	}
	dod, deliverable := focuses[0], focuses[1]
	for _, kind := range dod.RequiredEvidenceKinds {
		if kind == runtimecontract.EvidenceKindStateChange {
			t.Fatalf("DoD focus must not auto-require state_change: %+v", dod)
		}
	}
	found := false
	for _, kind := range deliverable.RequiredEvidenceKinds {
		if kind == runtimecontract.EvidenceKindStateChange {
			found = true
		}
	}
	if !found || deliverable.EvidenceRole != "primary_state_change" || deliverable.TargetRole != "implementation" || !deliverable.RequireTargetGrounding ||
		len(deliverable.TargetRefs) != 1 || deliverable.TargetRefs[0] != "cmd/main.go" || len(deliverable.TargetDiscoverySources) != 1 {
		t.Fatalf("deliverable focus must keep explicit state_change: %+v", deliverable)
	}
}

func TestCompileRuntimeV2CoverageFocusesRejectsUngroundedRequiredTarget(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID: "profile", DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{ID: "source", Required: true, EvidenceRole: "primary_state_change"}},
			EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
				ID: "source", Scope: "deliverable", RequireTargetGrounding: true, TargetRole: "implementation", AcceptedStepKinds: []string{"patch/apply"},
			}},
		}}},
	}
	_, err := compileRuntimeV2CoverageFocuses(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"behavior"}}, activation,
	)
	if err == nil || !strings.Contains(err.Error(), "target grounding requires target refs or bounded discovery sources") {
		t.Fatalf("ungrounded required target was accepted: %v", err)
	}
}

func TestCompileRuntimeV2CoverageFocusesSplitsIncompatibleTargetBindingsWithinSliceGroup(t *testing.T) {
	for _, tc := range []struct {
		name           string
		firstRole      string
		secondRole     string
		firstSelector  string
		secondSelector string
		want           string
	}{
		{
			name: "target_role", firstRole: "implementation", secondRole: "generated_artifact",
			firstSelector: "file/unique_workspace_entry", secondSelector: "file/unique_workspace_entry",
			want: "target role",
		},
		{
			name: "target_selector", firstRole: "implementation", secondRole: "implementation",
			firstSelector: "file/unique_workspace_entry", secondSelector: "profile/semantic_owner_selector",
			want: "target selector",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			activation := runtimecontract.RuntimeActivationContext{
				ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
				ExecutionContract: runtimecontract.RuntimeExecutionContract{
					AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
						SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2,
						MaxWorkItems:  2, MaxFocuses: 4, MaxCandidateContextItems: 4,
						Groups: []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{
							ID: "combined", Title: "combined", Scope: "deliverable", Priority: 1, MaxWorkItems: 2,
							Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true},
						}},
					},
					ProfileContracts: []runtimecontract.RuntimeProfileContract{{
						ID: "profile",
						DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{
							{ID: "first", Required: true, EvidenceRole: "primary_state_change"},
							{ID: "second", Required: true, EvidenceRole: "primary_state_change"},
						},
						EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
							{ID: "first", Scope: "deliverable", EvidenceRole: "primary_state_change", TargetRole: tc.firstRole, TargetSelectorRef: tc.firstSelector, TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}, RequireTargetGrounding: true, AcceptedStepKinds: []string{"patch/apply"}},
							{ID: "second", Scope: "deliverable", EvidenceRole: "primary_state_change", TargetRole: tc.secondRole, TargetSelectorRef: tc.secondSelector, TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}, RequireTargetGrounding: true, AcceptedStepKinds: []string{"patch/apply"}},
						},
					}},
				},
			}
			compiled, err := compileRuntimeV2Coverage(
				&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
				&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1"},
				activation,
			)
			if err != nil || len(compiled.Focuses) != 2 || len(compiled.Slices) != 2 {
				t.Fatalf("compile incompatible %s binding = %+v err=%v, want two isolated focuses", tc.name, compiled, err)
			}
			if tc.name == "target_role" && (compiled.Focuses[0].TargetRole != tc.firstRole || compiled.Focuses[1].TargetRole != tc.secondRole) {
				t.Fatalf("compiled target roles = %+v, want %q then %q", compiled.Focuses, tc.firstRole, tc.secondRole)
			}
			if tc.name == "target_selector" && (compiled.Focuses[0].TargetSelectorRef != tc.firstSelector || compiled.Focuses[1].TargetSelectorRef != tc.secondSelector) {
				t.Fatalf("compiled target selectors = %+v, want %q then %q", compiled.Focuses, tc.firstSelector, tc.secondSelector)
			}
		})
	}
}

func TestCompileRuntimeV2CoverageFocusesPreservesSharedTargetBindingWithinSlice(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
				SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2,
				MaxWorkItems:  2, MaxFocuses: 4, MaxCandidateContextItems: 4,
				Groups: []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{
					ID: "combined", Title: "combined", Scope: "deliverable", Priority: 1, MaxWorkItems: 2,
					Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true},
				}},
			},
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "profile",
				DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{
					{ID: "first", Required: true, EvidenceRole: "primary_state_change"},
					{ID: "second", Required: true, EvidenceRole: "primary_state_change"},
				},
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
					{ID: "first", Scope: "deliverable", EvidenceRole: "primary_state_change", TargetRole: "implementation", TargetSelectorRef: "file/unique_workspace_entry", TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}, RequireTargetGrounding: true, AcceptedStepKinds: []string{"patch/apply"}},
					{ID: "second", Scope: "deliverable", EvidenceRole: "primary_state_change", TargetRole: "implementation", TargetSelectorRef: "file/unique_workspace_entry", TargetDiscoverySources: []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest}, RequireTargetGrounding: true, AcceptedStepKinds: []string{"patch/apply"}},
				},
			}},
		},
	}
	compiled, err := compileRuntimeV2Coverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1"},
		activation,
	)
	if err != nil || len(compiled.Focuses) != 1 {
		t.Fatalf("compile shared binding = %+v err=%v", compiled, err)
	}
	focus := compiled.Focuses[0]
	if focus.TargetRole != "implementation" || focus.TargetSelectorRef != "file/unique_workspace_entry" || !focus.RequireTargetGrounding || len(focus.TargetDiscoverySources) != 1 {
		t.Fatalf("shared target binding was not preserved: %+v", focus)
	}
}

func TestCompileRuntimeV2CoverageCompilesProfileCoverageDependencies(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
				SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2, MaxWorkItems: 2, MaxFocuses: 4, MaxCandidateContextItems: 4,
				Groups: []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{ID: "deliverables", Title: "deliverables", Scope: "deliverable", Priority: 1, MaxWorkItems: 2, Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true}}},
			},
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID:                      "profile",
				DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{ID: "source", Required: true, EvidenceRole: "primary_state_change"}, {ID: "validation", Required: true, EvidenceRole: "supplemental_evidence"}},
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
					{ID: "source", Scope: "deliverable", EvidenceRole: "primary_state_change", AcceptedStepKinds: []string{"file/write"}},
					{ID: "validation", Scope: "deliverable", EvidenceRole: "supplemental_evidence", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{"validation/managed_session"}},
				},
				CoverageDependencies: []runtimecontract.RuntimeCoverageDependency{{ID: "source-invalidates-validation", SourceScope: "deliverable", SourceRequirementID: "source", DependentScope: "deliverable", DependentRequirementID: "validation", TriggerEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}}},
			}},
		},
	}
	compiled, err := compileRuntimeV2Coverage(&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"}, &workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1"}, activation)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Focuses) != 2 || len(compiled.CoverageItems) != 2 {
		t.Fatalf("compiled dependency coverage = %+v", compiled)
	}
	source := compiled.CoverageItems[0]
	if source.SourceID != "source" || len(source.ReopenDependencies) != 1 || source.ReopenDependencies[0].CoverageWorkItemID != compiled.CoverageItems[1].ID || len(source.ReopenDependencies[0].TriggerEvidenceKinds) != 1 || source.ReopenDependencies[0].TriggerEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange {
		t.Fatalf("source dependency = %+v; coverage=%+v", source, compiled.CoverageItems)
	}
}

func TestCompileRuntimeV2CoverageRejectsUnresolvedCoverageDependency(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID: "profile", DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{ID: "source", Required: true}},
			CoverageDependencies: []runtimecontract.RuntimeCoverageDependency{{ID: "missing-dependent", SourceScope: "deliverable", SourceRequirementID: "source", DependentScope: "deliverable", DependentRequirementID: "missing", TriggerEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}}},
		}}},
	}
	_, err := compileRuntimeV2Coverage(&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"}, &workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1"}, activation)
	if err == nil || !strings.Contains(err.Error(), "matched no coverage item") {
		t.Fatalf("unresolved dependency error = %v", err)
	}
}

func TestCompileRuntimeV2CoverageRejectsCyclicCoverageDependencies(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID: "profile", DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{ID: "first", Required: true}, {ID: "second", Required: true}},
			CoverageDependencies: []runtimecontract.RuntimeCoverageDependency{
				{ID: "first-refreshes-second", SourceScope: "deliverable", SourceRequirementID: "first", DependentScope: "deliverable", DependentRequirementID: "second", TriggerEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}},
				{ID: "second-refreshes-first", SourceScope: "deliverable", SourceRequirementID: "second", DependentScope: "deliverable", DependentRequirementID: "first", TriggerEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}},
			},
		}}},
	}
	_, err := compileRuntimeV2Coverage(&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"}, &workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1"}, activation)
	if err == nil || !strings.Contains(err.Error(), "contain a cycle") {
		t.Fatalf("cyclic dependency error = %v", err)
	}
}

func TestCompileRuntimeV2CoverageSlicesBoundLargeDoD(t *testing.T) {
	items := make([]string, 0, 10)
	for i := 1; i <= 10; i++ {
		items = append(items, "行为"+string(rune('A'+i-1)))
	}
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
				SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2,
				MaxWorkItems:  4, MaxFocuses: 4, MaxCandidateContextItems: 4,
				Groups: []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{
					ID: "slice-remaining-acceptance", Title: "Remaining", Scope: "remaining_acceptance", Priority: 90,
					MaxWorkItems: 4, Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true},
				}},
			},
		},
	}
	compiled, err := compileRuntimeV2Coverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: items},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	// 10 items / max 4 => 3 focuses/slices, not 10 assessment rounds.
	if len(compiled.Focuses) != 3 || len(compiled.Slices) != 3 {
		t.Fatalf("focuses=%d slices=%d, want 3/3", len(compiled.Focuses), len(compiled.Slices))
	}
	if len(compiled.CoverageItems) != 10 {
		t.Fatalf("coverage items = %d, want 10 stable identities", len(compiled.CoverageItems))
	}
	if compiled.Focuses[0].SliceID != "slice-remaining-acceptance-001" || len(compiled.Focuses[0].CoverageWorkItemIDs) != 4 {
		t.Fatalf("first slice focus = %+v", compiled.Focuses[0])
	}
	if compiled.Focuses[2].SliceID != "slice-remaining-acceptance-003" || len(compiled.Focuses[2].CoverageWorkItemIDs) != 2 {
		t.Fatalf("last slice focus = %+v", compiled.Focuses[2])
	}
	// Item IDs are independent of slice packing.
	if compiled.CoverageItems[0].ID != "cwi-001" || compiled.CoverageItems[9].ID != "cwi-010" {
		t.Fatalf("coverage item ids = %+v", compiled.CoverageItems)
	}
	if compiled.CoverageItems[0].TargetText != "行为A" || compiled.CoverageItems[9].TargetText != "行为J" {
		t.Fatalf("coverage item claim text = %+v", compiled.CoverageItems)
	}
}

func TestCompileRuntimeV2CoverageRejectsInvalidSliceMax(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
				SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2,
				MaxWorkItems:  100, MaxFocuses: 4, MaxCandidateContextItems: 4,
			},
		},
	}
	_, err := compileRuntimeV2Coverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一"}},
		activation,
	)
	if err == nil || !strings.Contains(err.Error(), "maxWorkItems") {
		t.Fatalf("expected maxWorkItems validation failure, got %v", err)
	}
}

func TestCompileRuntimeV2CoverageRejectsUnboundedFocusCount(t *testing.T) {
	items := make([]string, runtimecontract.RuntimeAcceptanceFocusMaxCount+1)
	for idx := range items {
		items[idx] = fmt.Sprintf("requirement-%03d", idx+1)
	}
	_, err := compileRuntimeV2Coverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: items},
		runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
	)
	if err == nil || !strings.Contains(err.Error(), "exceeding sealed policy limit") {
		t.Fatalf("expected focus-count guard failure, got %v", err)
	}
}

func TestCompileRuntimeV2CoverageRehydratesUnresolvedContinuation(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
		},
		Continuation: runtimecontract.RuntimeContinuationContext{
			RejectedResult: &runtimecontract.RuntimeRejectedResultContinuation{
				PredecessorRunID: "run-old", ResultID: "result-old", ResultState: common.ResultStateRejected,
				DefinitionRevision: "definition-1",
				Disposition:        runtimecontract.AssessmentDispositionRepairableGap,
				SatisfiedDoDItems: []runtimecontract.RuntimeContinuationItem{{
					CoverageWorkItemID: "cwi-001", State: "satisfied",
				}},
				UnsatisfiedDoDItems: []runtimecontract.RuntimeContinuationItem{{
					CoverageWorkItemID: "cwi-002", State: "unsatisfied",
				}, {
					CoverageWorkItemID: "cwi-003", State: "unsatisfied",
				}},
				CoverageGaps: []runtimecontract.RuntimeContinuationCoverageGap{{
					Scope: "coverage", CoverageWorkItemIDs: []common.CoverageWorkItemID{"cwi-002", "cwi-003"},
					MissingEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange},
				}},
			},
		},
	}
	compiled, err := compileRuntimeV2Coverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一", "行为二", "行为三"}},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.CoverageItems) != 2 {
		t.Fatalf("coverage items = %+v, want only unresolved cwi-002/cwi-003", compiled.CoverageItems)
	}
	// IDs must remain original definition IDs, not renumbered.
	ids := []string{string(compiled.CoverageItems[0].ID), string(compiled.CoverageItems[1].ID)}
	if ids[0] != "cwi-002" || ids[1] != "cwi-003" {
		t.Fatalf("coverage ids = %v, want [cwi-002 cwi-003]", ids)
	}
	if len(compiled.Focuses) == 0 {
		t.Fatal("expected focuses for unresolved items")
	}
}

func TestCompileRuntimeV2CoverageContinuationRestoresDependentCoverage(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID: "profile", DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{ID: "source", Required: true, EvidenceRole: "primary_state_change"}, {ID: "validation", Required: true, EvidenceRole: "supplemental_evidence"}},
			CoverageDependencies: []runtimecontract.RuntimeCoverageDependency{{ID: "source-refreshes-validation", SourceScope: "deliverable", SourceRequirementID: "source", DependentScope: "deliverable", DependentRequirementID: "validation", TriggerEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}}},
		}}},
		Continuation: runtimecontract.RuntimeContinuationContext{RejectedResult: &runtimecontract.RuntimeRejectedResultContinuation{
			PredecessorRunID: "run-old", ResultID: "result-old", ResultState: common.ResultStateRejected,
			UnsatisfiedDoDItems: []runtimecontract.RuntimeContinuationItem{{CoverageWorkItemID: "cwi-001", State: "unsatisfied"}},
		}},
	}
	compiled, err := compileRuntimeV2Coverage(&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"}, &workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1"}, activation)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.CoverageItems) != 2 || compiled.CoverageItems[0].SourceID != "source" || compiled.CoverageItems[1].SourceID != "validation" || len(compiled.CoverageItems[0].ReopenDependencies) != 1 || compiled.CoverageItems[0].ReopenDependencies[0].CoverageWorkItemID != compiled.CoverageItems[1].ID {
		t.Fatalf("continuation dependency closure = %+v", compiled)
	}
}

func TestCompileRuntimeV2CoverageGapAnalysisWhenContinuationLacksStructuredGaps(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		Continuation: runtimecontract.RuntimeContinuationContext{
			RejectedResult: &runtimecontract.RuntimeRejectedResultContinuation{
				PredecessorRunID: "run-old", ResultID: "result-old", ResultState: common.ResultStateRejected,
				Summary: "legacy summary only",
			},
		},
	}
	compiled, err := compileRuntimeV2Coverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一", "行为二"}},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Focuses) != 1 || compiled.Focuses[0].AssessmentScope != "continuation_gap_analysis" {
		t.Fatalf("focuses = %+v, want single continuation_gap_analysis focus", compiled.Focuses)
	}
	if !compiled.Focuses[0].RequireAssessment {
		t.Fatalf("gap-analysis focus must require assessment: %+v", compiled.Focuses[0])
	}
}

func TestCompileRuntimeV2CoverageGapAnalysisOnDefinitionChangeSupersede(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		Continuation: runtimecontract.RuntimeContinuationContext{
			DefinitionChangeSupersedesPrior: true,
			RejectedResult: &runtimecontract.RuntimeRejectedResultContinuation{
				PredecessorRunID: "run-old", ResultID: "result-old", ResultState: common.ResultStateRejected,
				UnsatisfiedDoDItems: []runtimecontract.RuntimeContinuationItem{{CoverageWorkItemID: "cwi-002", State: "unsatisfied"}},
			},
		},
	}
	compiled, err := compileRuntimeV2Coverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一", "行为二"}},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Focuses) != 1 || compiled.Focuses[0].AssessmentScope != "continuation_gap_analysis" {
		t.Fatalf("focuses = %+v, want gap-analysis on definition change", compiled.Focuses)
	}
}
