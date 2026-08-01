package activationcore

import (
	"fmt"
	"strings"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func TestCompileRuntimeCoverageFocusesAppliesDodItemWildcard(t *testing.T) {
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
	focuses, err := compileRuntimeCoverageFocuses(
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

func TestRuntimeBootstrapDefersTargetSelectionToFileOwner(t *testing.T) {
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
	bootstrap, err := runtimeBootstrapWithTestProof(
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

func TestRuntimeBootstrapSealsProfileProspectiveTargetIntoFocus(t *testing.T) {
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
	bootstrap, err := runtimeBootstrapWithTestProof(&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"}, &workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"initial deliverable"}}, "definition-1", nil, activation)
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

func TestRuntimeBootstrapSealsProfileBoundEmptyWorkspaceScopeIntoFocus(t *testing.T) {
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
	bootstrap, err := runtimeBootstrapWithTestProof(
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

func TestCompileRuntimeCoverageFocusesPrependsMatchedBootstrapArtifactsInDependencyOrder(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
			SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent,
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
	focuses, err := compileRuntimeCoverageFocuses(
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

func TestRuntimeBootstrapRejectsBroadDoDWithSharedValidationProof(t *testing.T) {
	dodItems := make([]string, 25)
	for idx := range dodItems {
		dodItems[idx] = fmt.Sprintf("acceptance item %02d", idx+1)
	}
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
				SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent,
				MaxWorkItems:  4, MaxFocuses: runtimecontract.RuntimeAcceptanceFocusMaxCount, MaxCandidateContextItems: 4,
				Groups: []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{
					{ID: "primary", Title: "Primary state change", Scope: "primary", Priority: 10, MaxWorkItems: 2, Match: runtimecontract.RuntimeAcceptanceSliceMatch{EvidenceRoles: []string{"primary_state_change"}}},
					{ID: "validation", Title: "Validation", Scope: "validation", Priority: 20, MaxWorkItems: 4, Match: runtimecontract.RuntimeAcceptanceSliceMatch{MissingEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}}},
					{ID: "version", Title: "Version control", Scope: "version", Priority: 25, MaxWorkItems: 2, Match: runtimecontract.RuntimeAcceptanceSliceMatch{SourceRequirementIDs: []string{"version_control_snapshot"}}},
					{ID: "supplemental", Title: "Supplemental", Scope: "supplemental", Priority: 30, MaxWorkItems: 4, Match: runtimecontract.RuntimeAcceptanceSliceMatch{EvidenceRoles: []string{"supplemental_evidence"}}},
					{ID: "remaining", Title: "Remaining", Scope: "remaining", Priority: 90, MaxWorkItems: 4, Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true}},
				},
			},
			ProfileContracts: []runtimecontract.RuntimeProfileContract{{
				ID: "software-dev",
				BootstrapRecipes: []runtimecontract.RuntimeBootstrapRecipe{{
					ID: "go-lsp", RequiredGoalTerms: []string{"gopls"},
					Artifacts: []runtimecontract.RuntimeBootstrapArtifact{
						{ID: "module", TargetRole: "module", TargetRef: "go.mod", AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}},
						{ID: "implementation", TargetRole: "implementation", TargetRef: "internal/lsp/manager.go", AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}, DependsOnArtifactIDs: []string{"module"}},
						{ID: "test", TargetRole: "test", TargetRef: "internal/lsp/manager_test.go", AllowedOperationKinds: []string{runtimecontract.StepKindFileWrite}, DependsOnArtifactIDs: []string{"implementation"}},
					},
				}},
				DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{
					{ID: "source_change", Required: true, EvidenceRole: "primary_state_change"},
					{ID: "validation_report", Required: true, EvidenceRole: "supplemental_evidence"},
					{ID: "coverage_map", Required: true, EvidenceRole: "supplemental_evidence"},
					{ID: "version_control_snapshot", Required: true, EvidenceRole: "supplemental_evidence"},
				},
				EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
					{ID: "source_change", Scope: "deliverable", EvidenceRole: "primary_state_change", AcceptedStepKinds: []string{runtimecontract.StepKindFileWrite}},
					{ID: "validation_report", Scope: "deliverable", EvidenceRole: "supplemental_evidence", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}},
					{ID: "coverage_map", Scope: "deliverable", EvidenceRole: "supplemental_evidence", AcceptedStepKinds: []string{runtimecontract.StepKindMCPToolCall}},
					{ID: "version_control_snapshot", Scope: "deliverable", EvidenceRole: "supplemental_evidence", AcceptedStepKinds: []string{runtimecontract.StepKindGitWrite}, AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{Kind: runtimecontract.StepKindGitWrite, VariantID: "commit"}}},
					{ID: "*", Scope: "dodItem", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation}, AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession}},
				},
			}},
		},
	}
	bootstrap, err := RuntimeBootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "build a gopls service"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: dodItems},
		"definition-1", nil, activation,
	)
	if bootstrap != nil || err == nil || !strings.Contains(err.Error(), "requires a CWI-specific declaration") {
		t.Fatalf("broad shared validation bootstrap=%+v err=%v, want activation rejection", bootstrap, err)
	}
}

func TestRuntimeBootstrapSealsProfileDeclaredSelectorBinding(t *testing.T) {
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
	bootstrap, err := runtimeBootstrapWithTestProof(&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"}, &workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"implementation"}}, "definition-1", nil, activation)
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
func TestRuntimeBootstrapSealsProspectiveTargetIntoLeadingDoDFocus(t *testing.T) {
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
	bootstrap, err := runtimeBootstrapWithTestProof(
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
	hasFileWrite, hasPatchApply := false, false
	for _, kind := range leading.AcceptedStepKinds {
		hasFileWrite = hasFileWrite || kind == runtimecontract.StepKindFileWrite
		hasPatchApply = hasPatchApply || kind == runtimecontract.RuntimeBlockKindPatchApply
	}
	if !hasFileWrite || !hasPatchApply {
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

func TestCompileRuntimeCoverageFocusesExactBeatsWildcard(t *testing.T) {
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
	focuses, err := compileRuntimeCoverageFocuses(
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

func TestCompileRuntimeCoverageFocusesRejectsUnknownAcceptedKind(t *testing.T) {
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
	_, err := compileRuntimeCoverageFocuses(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一"}},
		activation,
	)
	if err == nil || !strings.Contains(err.Error(), "no runtime operation descriptor") {
		t.Fatalf("expected unknown kind failure, got %v", err)
	}
}

func TestCompileRuntimeCoverageFocusesUsesEvidencePolicyCoverageRequirements(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
			ID: "policy-1", AppliesTo: "shell/command",
			CoverageRequirements: []runtimecontract.RuntimeCoverageEvidenceRequirement{{
				ID: "*", Scope: "dod_item",
				TargetRole:                        "verification_target",
				TargetRefs:                        []string{"runtime-validation-source.txt"},
				RequireTargetGrounding:            true,
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
	focuses, err := compileRuntimeCoverageFocuses(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一"}},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	focus := focuses[0]
	if !focus.RequireDirectEvidence || !focus.RequireExecutableBehaviorEvidence || focus.TargetRole != "verification_target" ||
		len(focus.TargetRefs) != 1 || focus.TargetRefs[0] != "runtime-validation-source.txt" || !focus.RequireTargetGrounding {
		t.Fatalf("evidence policy not applied: %+v", focus)
	}
	if len(focus.AcceptedStepKinds) != 2 || focus.AcceptedStepKinds[0] != "mcp/tool_call" || focus.AcceptedStepKinds[1] != "shell/command" {
		t.Fatalf("accepted = %v", focus.AcceptedStepKinds)
	}
	if len(focus.RejectedStepKinds) != 1 || focus.RejectedStepKinds[0] != "file/read" {
		t.Fatalf("rejected = %v", focus.RejectedStepKinds)
	}
}

func TestRuntimeBootstrapCompilesDodWildcardIntoFocuses(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent, MaxWorkItems: 4, MaxFocuses: 4, MaxCandidateContextItems: 4},
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
	bootstrap, err := runtimeBootstrapWithTestProof(
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

func TestRuntimeBootstrapSealsRunLevelStateChangeRequirement(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			// Validation proof bindings are CWI-granular; keep this fixture's
			// independent DoD items in separate focuses while testing the run-level
			// state-change ledger.
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent, MaxWorkItems: 1, MaxFocuses: 4, MaxCandidateContextItems: 4},
			StateChangePolicy:     runtimecontract.RuntimeStateChangeMustProduce,
		},
	}
	bootstrap, err := runtimeBootstrapWithTestProof(
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

func TestCompileRuntimeCoverageFocusesKeepsDeliverableStateChange(t *testing.T) {
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
	focuses, err := compileRuntimeCoverageFocuses(
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

func TestCompileRuntimeCoverageFocusesRejectsUngroundedRequiredTarget(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID: "profile", DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{ID: "source", Required: true, EvidenceRole: "primary_state_change"}},
			EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
				ID: "source", Scope: "deliverable", RequireTargetGrounding: true, TargetRole: "implementation", AcceptedStepKinds: []string{"patch/apply"},
			}},
		}}},
	}
	_, err := compileRuntimeCoverageFocuses(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"behavior"}}, activation,
	)
	if err == nil || !strings.Contains(err.Error(), "target grounding requires target refs or bounded discovery sources") {
		t.Fatalf("ungrounded required target was accepted: %v", err)
	}
}

func TestCompileRuntimeCoverageFocusesSplitsIncompatibleTargetBindingsWithinSliceGroup(t *testing.T) {
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
						SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent,
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
			compiled, err := compileRuntimeCoverage(
				&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
				&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1"},
				activation,
			)
			if err != nil || len(compiled.Focuses) != 2 || len(compiled.Slices) != 1 {
				t.Fatalf("compile incompatible %s binding = %+v err=%v, want two isolated focuses in one presentation slice", tc.name, compiled, err)
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

func TestCompileRuntimeCoverageFocusesPreservesSharedTargetBindingWithinSlice(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
				SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent,
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
	compiled, err := compileRuntimeCoverage(
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

func TestCompileRuntimeCoverageCompilesProfileCoverageDependencies(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
				SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent, MaxWorkItems: 2, MaxFocuses: 4, MaxCandidateContextItems: 4,
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
	compiled, err := compileRuntimeCoverage(&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"}, &workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1"}, activation)
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

func TestCompileRuntimeCoverageRejectsUnresolvedCoverageDependency(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID: "profile", DeliverableExpectations: []runtimecontract.RuntimeDeliverableExpectation{{ID: "source", Required: true}},
			CoverageDependencies: []runtimecontract.RuntimeCoverageDependency{{ID: "missing-dependent", SourceScope: "deliverable", SourceRequirementID: "source", DependentScope: "deliverable", DependentRequirementID: "missing", TriggerEvidenceKinds: []string{runtimecontract.EvidenceKindStateChange}}},
		}}},
	}
	_, err := compileRuntimeCoverage(&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"}, &workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1"}, activation)
	if err == nil || !strings.Contains(err.Error(), "matched no coverage item") {
		t.Fatalf("unresolved dependency error = %v", err)
	}
}

func TestCompileRuntimeCoverageRejectsCyclicCoverageDependencies(t *testing.T) {
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
	_, err := compileRuntimeCoverage(&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"}, &workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1"}, activation)
	if err == nil || !strings.Contains(err.Error(), "contain a cycle") {
		t.Fatalf("cyclic dependency error = %v", err)
	}
}

func TestCompileRuntimeCoverageSlicesBoundLargeDoD(t *testing.T) {
	items := make([]string, 0, 10)
	for i := 1; i <= 10; i++ {
		items = append(items, "行为"+string(rune('A'+i-1)))
	}
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
				SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent,
				MaxWorkItems:  4, MaxFocuses: 16, MaxCandidateContextItems: 4,
				Groups: []runtimecontract.RuntimeAcceptanceSliceGroupPolicy{{
					ID: "slice-remaining-acceptance", Title: "Remaining", Scope: "remaining_acceptance", Priority: 90,
					MaxWorkItems: 4, Match: runtimecontract.RuntimeAcceptanceSliceMatch{All: true},
				}},
			},
		},
	}
	compiled, err := compileRuntimeCoverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: items},
		activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	// 10 items / max 4 => 3 presentation slices but 10 independent semantic
	// focuses.  A slice never lets one validation receipt close four CWI.
	if len(compiled.Focuses) != 10 || len(compiled.Slices) != 3 {
		t.Fatalf("focuses=%d slices=%d, want 10/3", len(compiled.Focuses), len(compiled.Slices))
	}
	if len(compiled.CoverageItems) != 10 {
		t.Fatalf("coverage items = %d, want 10 stable identities", len(compiled.CoverageItems))
	}
	if compiled.Focuses[0].SliceID != "slice-remaining-acceptance-001" || len(compiled.Focuses[0].CoverageWorkItemIDs) != 1 {
		t.Fatalf("first slice focus = %+v", compiled.Focuses[0])
	}
	if compiled.Focuses[9].SliceID != "slice-remaining-acceptance-003" || len(compiled.Focuses[9].CoverageWorkItemIDs) != 1 {
		t.Fatalf("last slice focus = %+v", compiled.Focuses[9])
	}
	// Item IDs are independent of slice packing.
	if compiled.CoverageItems[0].ID != "cwi-001" || compiled.CoverageItems[9].ID != "cwi-010" {
		t.Fatalf("coverage item ids = %+v", compiled.CoverageItems)
	}
	if compiled.CoverageItems[0].TargetText != "行为A" || compiled.CoverageItems[9].TargetText != "行为J" {
		t.Fatalf("coverage item claim text = %+v", compiled.CoverageItems)
	}
}

func TestCompileRuntimeCoverageRejectsInvalidSliceMax(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: &runtimecontract.RuntimeAcceptanceSlicePolicy{
				SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaCurrent,
				MaxWorkItems:  100, MaxFocuses: 4, MaxCandidateContextItems: 4,
			},
		},
	}
	_, err := compileRuntimeCoverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为一"}},
		activation,
	)
	if err == nil || !strings.Contains(err.Error(), "maxWorkItems") {
		t.Fatalf("expected maxWorkItems validation failure, got %v", err)
	}
}

func TestCompileRuntimeCoverageRejectsUnboundedFocusCount(t *testing.T) {
	items := make([]string, runtimecontract.RuntimeAcceptanceFocusMaxCount+1)
	for idx := range items {
		items[idx] = fmt.Sprintf("requirement-%03d", idx+1)
	}
	_, err := compileRuntimeCoverage(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: items},
		runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}},
	)
	if err == nil || !strings.Contains(err.Error(), "exceeding sealed policy limit") {
		t.Fatalf("expected focus-count guard failure, got %v", err)
	}
}
