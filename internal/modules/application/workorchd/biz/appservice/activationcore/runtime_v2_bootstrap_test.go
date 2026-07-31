package activationcore

import (
	"strings"
	"testing"

	workspacemodel "github.com/muidea/workorch/internal/modules/blocks/workspace/pkg/models"
	"github.com/muidea/workorch/internal/pkg/common"
	runtimecontract "github.com/muidea/workorch/internal/pkg/workorchruntimecontract"
)

func testAcceptanceSlicePolicy() *runtimecontract.RuntimeAcceptanceSlicePolicy {
	return &runtimecontract.RuntimeAcceptanceSlicePolicy{
		SchemaVersion: runtimecontract.RuntimeAcceptanceSlicePolicySchemaV2,
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
func withTestDeterministicProof(activation runtimecontract.RuntimeActivationContext) runtimecontract.RuntimeActivationContext {
	const profileID = "test-deterministic-proof"
	const recipeID = "test-deterministic-validation"
	for _, scope := range []string{"dodItem", "goalCoverage", "acceptanceDimension", "deliverable"} {
		activation.ExecutionContract.ProfileContracts = append(activation.ExecutionContract.ProfileContracts, runtimecontract.RuntimeProfileContract{
			ID: profileID + "-" + scope,
			EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{{
				ID: "*", Scope: scope,
				RequiredEvidenceKinds:    []string{runtimecontract.EvidenceKindValidation},
				AcceptedStepKinds:        []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
				TargetRole:               "test_target",
				TargetDiscoverySources:   []string{runtimecontract.RuntimeValidationDiscoverySourceWorkspaceManifest},
				TargetSelectorRef:        "file/test_deterministic_proof",
				TargetSelectorOwnerRef:   profileID,
				TargetSelectorSubjectRef: scope,
				RequireTargetGrounding:   true,
				ValidationRecipeID:       recipeID,
				RequiredCheckRefs:        []string{"test-deterministic-check"},
			}},
		})
	}
	activation.EvidencePolicies = append(activation.EvidencePolicies, runtimecontract.RuntimeEvidencePolicy{
		ID: recipeID, AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "true", RecipeDeclared: true,
	})
	return activation
}

func runtimeV2BootstrapWithTestProof(goal *workspacemodel.Goal, dod *workspacemodel.DoD, definitionRevision string, changeImpact *runtimecontract.RuntimeDefinitionChangeImpact, activation runtimecontract.RuntimeActivationContext) (*runtimecontract.RuntimeBootstrap, error) {
	return RuntimeV2Bootstrap(goal, dod, definitionRevision, changeImpact, withTestDeterministicProof(activation))
}

// C12/T22: activation is the only place that compiles proof declarations.
// Missing declarations must terminate before a RuntimeBootstrap exists; they
// cannot reach a Run drive, provider, operation, or governance ticket where a
// later component might guess a CWI proof from Goal/DoD text.
func TestRuntimeV2BootstrapRejectsUndeclaredCoverageProofBeforeRuntimeExists(t *testing.T) {
	bootstrap, err := RuntimeV2Bootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver a capability"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"verify behavior"}},
		"definition-1", nil,
		runtimecontract.RuntimeActivationContext{
			ProviderTarget:    runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
			ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(), StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce},
		},
	)
	if bootstrap != nil || err == nil || !strings.Contains(err.Error(), "coverage_binding_unavailable") {
		t.Fatalf("undeclared proof bootstrap=%+v err=%v, want typed coverage_binding_unavailable", bootstrap, err)
	}
}

// C12/T22: a validation-capable diagnostic operation is not automatically a
// coverage proof. File read receipts have no owner-executed recipe/check
// observation, so activation must reject this profile before a Run, LLM call,
// operation, or ticket can exist.
func TestRuntimeV2BootstrapRejectsReadOnlyDiagnosticAsValidationCoverageProof(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
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
	bootstrap, err := RuntimeV2Bootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "inspect workspace"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"inspection is verified"}},
		"definition-1", nil, activation,
	)
	if bootstrap != nil || err == nil || !strings.Contains(err.Error(), "coverage_binding_unavailable") || !strings.Contains(err.Error(), "no deterministic validation proof operation") {
		t.Fatalf("read-only diagnostic proof bootstrap=%+v err=%v, want typed preflight rejection", bootstrap, err)
	}
}

// C12/T22: a template evidence-policy declaration must preserve its bounded
// target through the typed payload/compiler path.  Dropping it would make a
// valid declaration indistinguishable from an undeclared proof and either
// fail activation or tempt a later runtime component to infer a target.
func TestRuntimeV2BootstrapCompilesEvidencePolicyBindingTarget(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		EvidencePolicies: []runtimecontract.RuntimeEvidencePolicy{{
			ID: "source-present", AppliesTo: runtimecontract.StepKindShellCommand, CommandName: "true", RecipeDeclared: true,
			CoverageRequirements: []runtimecontract.RuntimeCoverageEvidenceRequirement{{
				ID: "*", Scope: "dod_item", RequiredEvidenceKinds: []string{runtimecontract.EvidenceKindValidation},
				AcceptedStepKinds: []string{runtimecontract.RuntimeBlockKindManagedValidationSession},
				TargetRole:        "verification_target", TargetRefs: []string{"runtime-validation-source.txt"}, RequireTargetGrounding: true,
				ValidationRecipeID: "source-present",
			}},
		}},
		ExecutionContract: runtimecontract.RuntimeExecutionContract{
			AcceptanceSlicePolicy: testAcceptanceSlicePolicy(),
			StateChangePolicy:     runtimecontract.RuntimeStateChangeAllowedButNotRequired,
		},
	}
	bootstrap, err := RuntimeV2Bootstrap(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "verify source"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"source exists"}},
		"definition-1", nil, activation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(bootstrap.Focuses) != 1 || len(bootstrap.Focuses[0].CoverageEvidenceBindings) != 1 {
		t.Fatalf("coverage bindings = %+v", bootstrap.Focuses)
	}
	binding := bootstrap.Focuses[0].CoverageEvidenceBindings[0]
	if len(binding.TargetRefs) != 1 || binding.TargetRefs[0] != "runtime-validation-source.txt" || binding.TargetSelectorRef != "" {
		t.Fatalf("binding target = %+v", binding)
	}
}

func TestRuntimeV2BootstrapBuildsOrderedDoDFocuses(t *testing.T) {
	bootstrap, err := runtimeV2BootstrapWithTestProof(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "交付完整能力"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"实现行为一", "验证行为二", "覆盖行为三"}},
		"definition-1",
		nil,
		runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(), StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce}},
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

func TestRuntimeV2BootstrapAllowsValidationOnlyFocusWhenStateChangeIsOptional(t *testing.T) {
	bootstrap, err := runtimeV2BootstrapWithTestProof(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "完成只读核对"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"核对结果已验证"}},
		"definition-1",
		nil,
		runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(), StateChangePolicy: runtimecontract.RuntimeStateChangeAllowedButNotRequired}},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{runtimecontract.EvidenceKindValidation}
	if len(bootstrap.Focuses) != 1 || len(bootstrap.Focuses[0].RequiredEvidenceKinds) != 1 || bootstrap.Focuses[0].RequiredEvidenceKinds[0] != want[0] {
		t.Fatalf("optional state-change focus = %+v, want validation only", bootstrap.Focuses)
	}
}

func TestRuntimeV2BootstrapSealsConfiguredConvergencePolicy(t *testing.T) {
	bootstrap, err := runtimeV2BootstrapWithTestProof(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "deliver"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"validate"}},
		"definition-1",
		nil,
		runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(),
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
}

func TestRuntimeV2BootstrapAppendsRequiredProfileFocusesWithTypedRequirements(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(),
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
				{ID: "runtime_entrypoint", Scope: "acceptanceDimension", AcceptedStepKinds: []string{"shell/command"}},
				{ID: "source_change", Scope: "deliverable", EvidenceRole: "primary_state_change", AcceptedStepKinds: []string{"patch/apply"}},
				{ID: "version_control_snapshot", Scope: "deliverable", AcceptedStepKinds: []string{"git/write"}, AcceptedOperationVariants: []runtimecontract.RuntimeOperationVariantRef{{Kind: "git/write", VariantID: "commit"}}},
			},
		}},
	}}
	bootstrap, err := runtimeV2BootstrapWithTestProof(
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
	entrypoint, sourceChange, coverageMap, snapshot := bootstrap.Focuses[1], bootstrap.Focuses[2], bootstrap.Focuses[3], bootstrap.Focuses[4]
	if entrypoint.AssessmentScope != "acceptanceDimension" || len(entrypoint.RequiredEvidenceKinds) != 1 || entrypoint.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindValidation ||
		sourceChange.EvidenceRole != "primary_state_change" || sourceChange.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange ||
		len(coverageMap.RequiredEvidenceKinds) != 1 || coverageMap.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindValidation || !coverageMap.RequireAssessment ||
		len(snapshot.AcceptedOperationVariants) != 1 || snapshot.AcceptedOperationVariants[0] != (runtimecontract.RuntimeOperationVariantRef{Kind: "git/write", VariantID: "commit"}) || snapshot.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange {
		t.Fatalf("typed profile focuses = %+v", bootstrap.Focuses)
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

func TestRuntimeV2BootstrapBuildsBoundedGitCapabilityStateFixture(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(),
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
	bootstrap, err := runtimeV2BootstrapWithTestProof(
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
	validation, gitFocus := bootstrap.Focuses[0], bootstrap.Focuses[1]
	if validation.AssessmentScope != "dodItem" || len(validation.RequiredEvidenceKinds) != 1 || validation.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindValidation ||
		gitFocus.AssessmentScope != "deliverable" || gitFocus.RequiredEvidenceKinds[0] != runtimecontract.EvidenceKindStateChange ||
		len(gitFocus.AcceptedOperationVariants) != 1 || gitFocus.AcceptedOperationVariants[0] != (runtimecontract.RuntimeOperationVariantRef{Kind: runtimecontract.StepKindGitWrite, VariantID: "commit"}) {
		t.Fatalf("bounded Git fixture focus contracts = %+v", bootstrap.Focuses)
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

func TestRuntimeV2BootstrapRejectsUnknownProfileAcceptedKind(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(),
		StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
		ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID: "broken-profile",
			EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
				{ID: "mystery", Scope: "acceptanceDimension", AcceptedStepKinds: []string{"totally/unknown"}},
			},
		}},
	}}
	_, err := runtimeV2BootstrapWithTestProof(
		&workspacemodel.Goal{ID: "goal-1", WorkspaceID: "ws-1", Text: "交付完整能力"},
		&workspacemodel.DoD{ID: "dod-1", WorkspaceID: "ws-1", Items: []string{"行为已验证"}},
		"definition-1", nil, activation,
	)
	if err == nil || !strings.Contains(err.Error(), "no runtime operation descriptor") {
		t.Fatalf("expected unknown accepted kind failure, got %v", err)
	}
}

func TestRuntimeV2BootstrapSoftwareDevProfileIncludesManagedValidation(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"}, ExecutionContract: runtimecontract.RuntimeExecutionContract{AcceptanceSlicePolicy: testAcceptanceSlicePolicy(),
		StateChangePolicy: runtimecontract.RuntimeStateChangeMustProduce,
		ProfileContracts: []runtimecontract.RuntimeProfileContract{{
			ID: "software-dev",
			EvidenceRequirements: []runtimecontract.RuntimeProfileEvidenceRequirement{
				{ID: "runtime_entrypoint", Scope: "acceptanceDimension", AcceptedStepKinds: []string{"shell/command", "mcp/tool_call", "validation/managed_session"}},
			},
		}},
	}}
	bootstrap, err := runtimeV2BootstrapWithTestProof(
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

func TestRuntimeV2BootstrapRegistersSelectedReadOnlyMCPToolForValidation(t *testing.T) {
	activation := runtimecontract.RuntimeActivationContext{
		ProviderTarget: runtimecontract.ProviderModelTarget{ProviderID: "ai-proxy", ModelID: "gpt-4o"},
		MCPCapabilities: []runtimecontract.RuntimeMCPCapabilityContext{{
			ID: "mcp-1:tool:repo.summary", ServerID: "mcp-1", Kind: "tool", Name: "repo.summary", Enabled: true,
			EffectScope: string(runtimecontract.RuntimeEffectNone), IdempotencyClass: string(runtimecontract.RuntimeOperationIdempotencySafeToReplay),
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
	bootstrap, err := runtimeV2BootstrapWithTestProof(
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
